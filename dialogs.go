package main

// The requesters: open / save (a file list), info (SAUCE), colour, help, and yes / no.

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Dialog interface {
	draw(u *UI)
	key(k string, ctrl, shift bool) bool // false: not used (Esc and Return have defaults)
}

func closeDialog() {
	a.dialog = nil
	ui.focus = nil
	ui.drag = ""
}

// centre gives a w x h box in the middle of the window, shrunk to fit.
// colAfter is where a gadget goes after its label(s): at least min, else past the widest label.
func colAfter(min int, labels ...string) int {
	w := min
	for _, l := range labels {
		w = max(w, textW(l)+10)
	}
	return w
}

// settingsCol is where Settings' gadgets start after their labels.
func settingsCol() int {
	return colAfter(120, "Window size", "Keys like", "My BBS", "Theme", "Interface font")
}

func centre(w, h int) Rect {
	if lineH > 16 { // the sizes are for 16-pixel lines: a bigger interface font gets more room
		w, h = w*lineH/16, h*lineH/16
	}
	w, h = min(w, ui.fb.w-20), min(h, ui.fb.h-20)
	return Rect{(ui.fb.w - w) / 2, (ui.fb.h - h) / 2, w, h}
}

// buttonRow lays out buttons from the right edge of r's bottom line; it returns the clicked one or -1.
func buttonRow(u *UI, r Rect, labels ...string) int {
	x := r.x + r.w
	hit := -1
	for i := len(labels) - 1; i >= 0; i-- {
		w := max(80, buttonW(labels[i]))
		x -= w
		if u.button(Rect{x, r.y + r.h - gadH, w, gadH}, labels[i], false, "") != nil {
			hit = i
		}
		x -= 8
	}
	return hit
}

// ---------------------------------------------------------------- yes / no

// Confirm asks before something that cannot be undone. Return never does it:
// Return saves when there is a Save choice and cancels otherwise; Esc cancels.
type Confirm struct {
	text   string
	button string // the yes button, "Throw away" if empty
	yes    func()
	save   func() // when set, a Save button as well (and the default)
}

func (d *Confirm) draw(u *UI) {
	in := u.window(centre(500, 150), "Charcoal")
	lines := wrap(d.text, in.w)
	for i, l := range lines {
		u.fb.text(in.x, in.y+i*lineH, l, cText)
	}
	yes := d.button
	if yes == "" {
		yes = "Throw away"
	}
	labels := []string{yes, "Cancel"}
	if d.save != nil {
		labels = []string{yes, "Cancel", "Save"}
	}
	switch buttonRow(u, in, labels...) {
	case 0:
		closeDialog()
		d.yes()
	case 1:
		closeDialog()
	case 2:
		closeDialog()
		d.save()
	}
}

func (d *Confirm) key(k string, ctrl, shift bool) bool {
	switch k {
	case "Return", "Keypad Enter", "S":
		closeDialog()
		if d.save != nil {
			d.save()
		}
	case "Escape", "N":
		closeDialog()
	default:
		return false
	}
	return true
}

// ---------------------------------------------------------------- the file requester

type entry struct {
	name string
	dir  bool
	size int64
	full string // the drives list: the drive's own path
}

type FileReq struct {
	save    bool
	dir     string // "" while the drives are listed
	entries []entry
	sel     int
	top     int
	path    Field
	choice  int // open: auto / Amiga / PC; save: the format
	err     string
	lastClk time.Time
	lastSel int
	selPath string            // the path field as the selection set it: Return then acts on the selection
	after   func()            // save: what to do once the page is saved
	pick    func(path string) // a requester for something else (a font): takes the path; no format choice
	title   string            // and its title
	note    string            // and the line where the format choice would be
	back    Dialog            // and the requester to go back to afterwards
	pickDir bool              // pick takes a folder: the one listed (Animate a folder, batch_ui.go)
}

var openAs = []string{"auto", "amiga", "pc", "atari", "st", "zx", "tt", "c64",
	"pcb", "avt", "pipe", "ctrla", "wwiv", "wildcat", "celerity"}
var openAsNames = []string{"detect", "Amiga", "PC", "Atari ATASCII", "Atari ST VT52", "ZX Spectrum", "Teletext", "C64 PETSCII",
	"PC: PCBoard @X", "PC: Avatar", "PC: pipe codes |07", "PC: Synchronet Ctrl-A", "PC: WWIV hearts", "PC: Wildcat! @XX@", "PC: Celerity |x"}

func openOpenRequester() {
	d := &FileReq{sel: -1}
	d.listFirst(startDir(a.lastDir))
	d.path.enter = d.enter
	a.dialog = d
	ui.focus = &d.path
}

func openSaveRequester() {
	d := &FileReq{save: true, sel: -1}
	dir := a.lastDir
	if a.path != "" {
		dir = filepath.Dir(a.path)
	}
	d.listFirst(startDir(dir))
	d.path.set(filepath.Join(d.dir, a.name))
	for i, f := range a.modeFormats() {
		if f.id == a.saveFmt {
			d.choice = i
		}
	}
	d.path.enter = d.enter
	a.dialog = d
	ui.focus = &d.path
}

// startDir is where a requester opens: the given folder, else the current one;
// a program started from the Mac's Finder runs in /, so the home folder then.
func startDir(dir string) string {
	if dir == "" {
		dir, _ = os.Getwd()
	}
	if dir == "" || filepath.Dir(dir) == dir {
		if h := homeDir(); h != "" {
			return h
		}
	}
	return dir
}

// listFirst lists the first of dir, the home folder and the first drive that can be listed
// (a remembered folder may be gone, a NAS unmounted); why dir could not be is shown.
func (d *FileReq) listFirst(dir string) {
	why := ""
	for _, p := range append([]string{dir, homeDir()}, fileRoots()...) {
		if p == "" {
			continue
		}
		d.list(p)
		if d.err == "" {
			break
		}
		if why == "" {
			why = d.err
		}
	}
	if why != "" && d.dir != "" {
		d.err = why
	}
}

func (d *FileReq) list(dir string) {
	dir = filepath.Clean(expandHome(dir))
	ents, err := os.ReadDir(dir)
	if err != nil {
		d.err = err.Error()
		return
	}
	d.err = ""
	d.dir = dir
	d.entries = []entry{{name: "..", dir: true}}
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		isDir := info.IsDir()
		if info.Mode()&os.ModeSymlink != 0 {
			if st, err := os.Stat(filepath.Join(dir, e.Name())); err == nil {
				isDir = st.IsDir()
			}
		}
		d.entries = append(d.entries, entry{name: e.Name(), dir: isDir, size: info.Size()})
	}
	rest := d.entries[1:]
	sort.Slice(rest, func(i, j int) bool {
		if rest[i].dir != rest[j].dir {
			return rest[i].dir
		}
		return strings.ToLower(rest[i].name) < strings.ToLower(rest[j].name)
	})
	d.sel, d.top, d.selPath = -1, 0, ""
	if !d.save {
		d.path.set(withPathEnd(dir))
	} else {
		d.path.set(filepath.Join(dir, filepath.Base(d.path.String())))
	}
}

// listRoots lists the drives (Windows) or the disks: / and the mounted ones.
func (d *FileReq) listRoots() {
	d.err, d.dir, d.entries = "", "", nil
	for _, r := range fileRoots() {
		d.entries = append(d.entries, entry{name: r, dir: true, full: r})
	}
	d.sel, d.top, d.selPath = -1, 0, ""
	if !d.save {
		d.path.set("")
	} else {
		d.path.set(filepath.Base(d.path.String()))
	}
}

// parent lists the folder above; above a drive's top, the drives.
func (d *FileReq) parent() {
	if d.dir == "" {
		return
	}
	if up := filepath.Dir(d.dir); up != d.dir {
		d.list(up)
	} else {
		d.listRoots()
	}
}

// rootsLabel names the drives button.
func rootsLabel() string {
	if runtime.GOOS == "windows" {
		return "Drives"
	}
	return "Disks"
}

func expandHome(p string) string {
	if p == "~" || (len(p) > 1 && p[0] == '~' && os.IsPathSeparator(p[1])) {
		return homeDir() + p[1:]
	}
	return p
}

func (d *FileReq) activate(i int) {
	if i < 0 || i >= len(d.entries) {
		return
	}
	e := d.entries[i]
	if e.dir {
		if e.full != "" {
			d.list(e.full)
		} else if e.name == ".." {
			d.parent()
		} else {
			d.list(filepath.Join(d.dir, e.name))
		}
		return
	}
	d.path.set(filepath.Join(d.dir, e.name))
	d.ok()
}

// enter is Return: on a selected entry nobody has typed over since, it opens
// it (a drawer is entered); otherwise the typed path is used.
func (d *FileReq) enter() {
	if d.sel >= 0 && d.sel < len(d.entries) && d.path.String() == d.selPath && (d.entries[d.sel].dir || !d.save) {
		d.activate(d.sel)
		return
	}
	d.ok()
}

func (d *FileReq) ok() {
	p := strings.TrimSpace(expandHome(d.path.String()))
	if p == "" {
		return
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(d.dir, p)
	}
	if st, err := os.Stat(p); err == nil && st.IsDir() {
		if d.pickDir && filepath.Clean(p) == d.dir {
			closeDialog()
			d.pick(p)
			return
		}
		d.list(p)
		return
	}
	if hasPathEnd(d.path.String()) {
		d.err = "No such drawer: " + p
		return
	}
	if d.pick != nil {
		closeDialog()
		d.pick(p)
		return
	}
	if d.save {
		// A typed file ending picks the format, unless the chosen one already
		// fits it (.png normal or twice the size, .txt plain or UTF-8).
		ext := strings.ToLower(filepath.Ext(p))
		formats := a.modeFormats()
		fits := func(f Format) bool {
			return f.ext == ext || (ext == ".txt" && f.id == "text") || (ext == ".ansi" && f.id == "ansi") || (ext == ".htm" && f.id == "html")
		}
		if !fits(formats[d.choice]) {
			for i, f := range formats {
				if fits(f) && !(ext == ".txt" && f.id == "text") { // a new .txt is UTF-8, as text is today
					d.choice = i
					break
				}
			}
		}
		id, after := formats[d.choice].id, d.after
		write := func() {
			if a.writeOut(p, id) && after != nil && !a.dirty {
				after()
			}
		}
		closeDialog()
		if _, err := os.Stat(p); err == nil && filepath.Clean(p) != filepath.Clean(a.path) {
			a.dialog = &Confirm{text: "\"" + filepath.Base(p) + "\" is already there. Replace it?", button: "Replace", yes: write}
			return
		}
		write()
		return
	}
	closeDialog()
	if d.pick != nil {
		d.pick(p)
		return
	}
	a.openPath(p, openAs[d.choice])
}

// recentPopup lists the recent files under r; picking one opens it.
func (d *FileReq) recentPopup(u *UI, r Rect) {
	recent := append([]string(nil), prefs.Recent...)
	items := make([]string, len(recent))
	w := r.w
	for i, p := range recent {
		items[i] = shortPath(p)
		w = max(w, textW(items[i])+40)
	}
	h := len(items)*(lineH+2) + 4
	py := r.y + r.h
	if py+h > u.fb.h {
		py = max(0, r.y-h)
	}
	px := min(r.x, max(0, u.fb.w-w))
	u.popup = &Popup{id: "recent", r: Rect{px, py, w, h}, items: items, cur: -1, pick: func(i int) {
		as := openAs[d.choice]
		closeDialog()
		a.openPath(recent[i], as)
	}}
}

func (d *FileReq) setFormat(i int) {
	d.choice = i
	p := d.path.String()
	ext := filepath.Ext(p)
	if ext != "" && !strings.Contains(ext, "/") {
		p = strings.TrimSuffix(p, ext)
	}
	d.path.set(p + a.modeFormats()[i].ext)
}

func (d *FileReq) draw(u *UI) {
	fb := u.fb
	title := "Open"
	if d.save {
		title = "Save as"
	}
	if d.title != "" {
		title = d.title
	}
	in := u.window(centre(720, ui.fb.h*4/5), title)
	if d.dir == "" {
		fb.text(in.x, in.y, rootsLabel(), cText)
	} else {
		fb.text(in.x, in.y, "Drawer: "+d.dir, cText)
	}
	listR := Rect{in.x, in.y + lineH + 6, in.w - 18, in.h - 3*gadH - lineH - 36}
	fb.fill(listR, cGrey)
	fb.bevel(listR, true)
	rows := max(1, (listR.h-4)/lineH)
	if w := u.takeWheel(listR); w != 0 {
		d.top += w * 3
	}
	d.top = u.scrollbar("filelist", Rect{listR.x + listR.w, listR.y, 18, listR.h}, true, d.top, rows, len(d.entries))
	old := fb.clip
	fb.clip = listR.inset(2)
	for i := d.top; i < min(len(d.entries), d.top+rows); i++ {
		e := d.entries[i]
		r := Rect{listR.x + 2, listR.y + 2 + (i-d.top)*lineH, listR.w - 4, lineH}
		col := uint32(cText)
		if e.dir {
			col = cHi
		}
		if i == d.sel {
			fb.fill(r, cBlue)
		}
		name := e.name
		if e.dir {
			fb.text(r.x+4, r.y, name, col)
			fb.text(r.x+r.w-textW("Drawer")-6, r.y, "Drawer", col)
		} else {
			fb.text(r.x+4, r.y, name, col)
			s := fmt.Sprint(e.size)
			fb.text(r.x+r.w-textW(s)-6, r.y, s, col)
		}
		if c := u.take(r); c != nil {
			double := i == d.lastSel && time.Since(d.lastClk) < 400*time.Millisecond
			d.sel, d.lastSel, d.lastClk = i, i, time.Now()
			if double || e.dir {
				d.activate(i)
				fb.clip = old
				return
			}
			d.path.set(filepath.Join(d.dir, e.name))
			d.selPath = d.path.String()
		}
	}
	fb.clip = old
	y := listR.y + listR.h + 8
	fb.text(in.x, y+3, "File", cText)
	u.field(Rect{in.x + colAfter(48, "File"), y, in.w - colAfter(48, "File"), gadH}, &d.path, "Type a path; Enter opens it (a drawer lists it)")
	y += gadH + 8
	if d.pick != nil {
		fb.text(in.x, y+3, d.note, cText)
	} else if d.save {
		fb.text(in.x, y+3, "Format", cText)
		names := []string{}
		for _, f := range a.modeFormats() {
			names = append(names, f.name)
		}
		u.choice("fmt", Rect{in.x + colAfter(64, "Format"), y, choiceW(names), gadH}, names, d.choice, "", d.setFormat)
	} else {
		fb.text(in.x, y+3, "Open as", cText)
		var names []string
		var shown []int // openAs indexes on offer (C64 only with its ROMs)
		cur := 0
		for i, n := range openAsNames {
			if (openAs[i] == "c64" || openAs[i] == "imageseq") && !a.hasC64 {
				continue
			}
			if i == d.choice {
				cur = len(shown)
			}
			names, shown = append(names, n), append(shown, i)
		}
		u.choice("as", Rect{in.x + colAfter(72, "Open as"), y, choiceW(names), gadH}, names, cur, "", func(i int) { d.choice = shown[i] })
		bx := in.x + 72 + choiceW(names) + 16
		if len(prefs.Recent) > 0 {
			r := Rect{bx, y, buttonW("Recent files"), gadH}
			if u.button(r, "Recent files", false, "The last files opened or saved ("+keyName("open_recent")+" opens the newest again)") != nil {
				d.recentPopup(u, r)
			}
			bx += r.w + 8
		}
		if u.button(Rect{bx, y, buttonW("Animate a folder..."), gadH}, "Animate a folder...", false,
			"Every screen of a folder animated for your BBS at once ("+keyName("animate_folder")+")") != nil {
			dir := d.dir
			closeDialog()
			openAnimateFolder()
			if b, ok := a.dialog.(*BatchReq); ok && dir != "" {
				b.folder.set(dir)
			}
			return
		}
	}
	if d.err != "" {
		fb.text(in.x, in.y+in.h-gadH-lineH-6, d.err, cHi)
	}
	label := "Open"
	if d.save {
		label = "Save"
	}
	if d.pickDir {
		label = "Choose"
	}
	switch buttonRow(u, in, "Parent", rootsLabel(), "Home", label, "Cancel") {
	case 0:
		d.parent()
	case 1:
		d.listRoots()
	case 2:
		d.list(homeDir())
	case 3:
		d.enter()
	case 4:
		d.cancel()
	}
}

// cancel closes the requester (a folder requester goes back to the one that asked).
func (d *FileReq) cancel() {
	closeDialog()
	if d.pickDir && d.back != nil {
		a.dialog = d.back
	}
}

func (d *FileReq) key(k string, ctrl, shift bool) bool {
	rows := max(1, (ui.fb.h*4/5-3*gadH-lineH-80)/lineH)
	move := func(n int) {
		if len(d.entries) == 0 {
			return
		}
		d.sel = max(0, min(len(d.entries)-1, d.sel+n))
		if d.sel < d.top {
			d.top = d.sel
		}
		if d.sel >= d.top+rows {
			d.top = d.sel - rows + 1
		}
		if e := d.entries[d.sel]; !e.dir {
			d.path.set(filepath.Join(d.dir, e.name))
		} else if e.full != "" && !d.save {
			d.path.set(e.full)
		} else if !d.save {
			d.path.set(withPathEnd(d.dir))
		}
		d.selPath = d.path.String()
	}
	switch k {
	case "Up":
		move(-1)
	case "Down":
		move(1)
	case "PageUp":
		move(-rows)
	case "PageDown":
		move(rows)
	case "Return", "Keypad Enter":
		d.enter()
	case "Escape":
		d.cancel()
	default:
		return false
	}
	return true
}

// ---------------------------------------------------------------- info (SAUCE)

type InfoReq struct {
	title, author, group Field
	sauce, clear         bool
}

func openInfo() {
	d := &InfoReq{title: Field{max: 35}, author: Field{max: 20}, group: Field{max: 20}, sauce: a.info.writeSauce, clear: a.info.clear}
	d.title.set(a.info.title)
	d.author.set(a.info.author)
	d.group.set(a.info.group)
	d.title.enter, d.author.enter, d.group.enter = d.ok, d.ok, d.ok
	a.dialog = d
	ui.focus = &d.title
}

func (d *InfoReq) ok() {
	a.info = Info{title: d.title.String(), author: d.author.String(), group: d.group.String(), writeSauce: d.sauce, clear: d.clear, notes: a.info.notes}
	closeDialog()
}

func (d *InfoReq) draw(u *UI) {
	fb := u.fb
	in := u.window(centre(640, 340), "Info")
	y := in.y
	for _, f := range []struct {
		label string
		fl    *Field
	}{{"Title", &d.title}, {"Author", &d.author}, {"Group", &d.group}} {
		fb.text(in.x, y+3, f.label, cText)
		u.field(Rect{in.x + 72, y, digitsW(f.fl.max+2) + 8, gadH}, f.fl, "")
		y += gadH + 6
	}
	y += 4
	d.sauce, _ = u.check(in.x, y, "Write a SAUCE record when saving (width, font, title)", d.sauce, "")
	y += gadH + 4
	d.clear, _ = u.check(in.x, y, "Start the file with clear screen (form feed on Amiga, ESC[2J on PC)", d.clear, "")
	y += gadH + 10
	for _, l := range wrap(describeSauce(a.sauce), in.w) {
		fb.text(in.x, y, l, cText)
		y += lineH
	}
	switch buttonRow(u, in, "OK", "Cancel") {
	case 0:
		d.ok()
	case 1:
		closeDialog()
	}
}

func (d *InfoReq) key(k string, ctrl, shift bool) bool {
	switch k {
	case "Tab":
		order := []*Field{&d.title, &d.author, &d.group}
		for i, f := range order {
			if ui.focus == f {
				n := i + 1
				if shift {
					n = i + 2
				}
				ui.focus = order[n%3]
				return true
			}
		}
		ui.focus = order[0]
	case "Escape":
		closeDialog()
	case "Return", "Keypad Enter":
		d.ok()
	default:
		return false
	}
	return true
}

// ---------------------------------------------------------------- colour

type ColourReq struct {
	i       int
	rgb     [3]int
	orig    [16]uint32
	origPal string
}

func openColour(i int) {
	c := a.pal[i]
	a.dialog = &ColourReq{i: i, rgb: [3]int{int(c >> 16 & 255), int(c >> 8 & 255), int(c & 255)}, orig: a.custom, origPal: a.paletteFor[a.mode]}
}

func (d *ColourReq) apply() {
	if a.paletteFor[a.mode] != "custom" {
		a.custom = a.pal
		a.paletteFor[a.mode] = "custom"
	}
	a.custom[d.i] = 0xFF000000 | uint32(d.rgb[0])<<16 | uint32(d.rgb[1])<<8 | uint32(d.rgb[2])
	a.setPalette()
}

func (d *ColourReq) cancel() {
	a.custom = d.orig
	a.paletteFor[a.mode] = d.origPal
	a.setPalette()
	closeDialog()
}

func (d *ColourReq) draw(u *UI) {
	fb := u.fb
	in := u.window(centre(460, 220), fmt.Sprintf("Colour %d", d.i))
	before := d.rgb
	for k, name := range []string{"Red", "Green", "Blue"} {
		y := in.y + k*(gadH+8)
		fb.text(in.x, y+3, name, cText)
		d.rgb[k] = u.slider(fmt.Sprint("rgb", k), Rect{in.x + 56, y, in.w - 56 - 100, gadH}, d.rgb[k])
		fb.text(in.x+in.w-90, y+3, fmt.Sprintf("%3d", d.rgb[k]), cText)
	}
	sw := Rect{in.x + in.w - 50, in.y, 50, 3*gadH + 16}
	if before != d.rgb {
		d.apply()
	}
	fb.fill(sw, 0xFF000000|uint32(d.rgb[0])<<16|uint32(d.rgb[1])<<8|uint32(d.rgb[2]))
	fb.frame(sw, cEdgeDk)
	switch buttonRow(u, in, "OK", "Cancel") {
	case 0:
		savePrefs()
		closeDialog()
	case 1:
		d.cancel()
	}
}

func (d *ColourReq) key(k string, ctrl, shift bool) bool {
	switch k {
	case "Escape":
		d.cancel()
	case "Return", "Keypad Enter":
		savePrefs()
		closeDialog()
	default:
		return false
	}
	return true
}

// ---------------------------------------------------------------- help

type HelpReq struct{ top int }

func openHelp() { a.dialog = &HelpReq{} }

// helpText is the help; its list of keys is made from the key table.
func helpText() string {
	return `START SCREEN
Started without files, CharEdit shows a new page for each machine (Amiga, PC, Atari 8-bit, Atari ST, C64, PET, VIC-20, C128, ZX Spectrum, teletext, Amstrad CPC), each in its own font and colours with its size, the last 10 files and Open. Click one, or arrows / Tab and Enter. Typing, a tool or Esc starts on a page of the last used type instead; dropping a file opens it. New pages (+, ` + keyName("new_page") + `) start there too; ` + keyName("start_screen") + ` brings it back. "Show this at start" off: CharEdit starts on an empty page of the last used type.

MOUSE
Draw: left paints the brush (character + colours), right erases, Shift+click draws a line from the last point.
Colour: left sets the foreground, right the background (on a C64 page: the foreground only).
Fill: left fills equal cells with the brush.
Shape: drag a line, box or ellipse (the choice under the brush), in the brush, in box-drawing lines (PC single or double, PETSCII and Atari lines) or in half blocks (PC; sixels on teletext). Shift at the press: filled (or the other way with Filled on). Right button: erases (half blocks: the background colour). One undo step a shape.
Under the brush, while drawing: Shade (left steps a cell up the shades (on PC pages from empty through the three shade blocks to the full block), right down; prefs.json "shades" can give each page type its own, e.g. "shades": {"amiga": [32, 46, 58, 43, 35]}), Colour only / Char only (paint just the colours or just the characters), and the mirror (what is drawn is mirrored around the page's centre, characters as their mirror twins).
Half (PC): left paints half-cell pixels in the foreground, right in the background colour. Sixel (teletext): left sets, right clears a graphics pixel.
Select: drag a block; drag inside a selected block to move it (Esc while moving puts it back). While pasting, the block follows the mouse: left click stamps it, Shift+click stamps it without its blank cells, right click or Esc stops.
Middle click or Pick: take a cell's character and colours.
Ctrl+drag moves the reference picture (any tool).
Characters strip: click = brush (types it with Type). Right click = put on the active F-key.
Colours: left = foreground, right = background (PETSCII and Atari: the screen colour; middle click: the border), Shift+click = change the colour.
Pop-up gadgets: click opens the list, right click steps to the next item.
Wheel scrolls the page, Shift+wheel sideways, Ctrl+wheel zooms around the mouse.
Tabs: click to show, x or middle click closes, + opens a new page. With two pages on show, click a page to work on it.

KEYS
The page has a height (H) as well as a width: it ends where the dark area starts, and every row is saved, empty rows at the bottom too. Enter or Down past the last row adds a row; H sets the height (fewer rows cuts, Undo brings them back).
Typing writes at the cursor in the brush colours (any tool). Arrows, Home, End, PgUp, PgDn, Enter, Tab, Backspace, Delete (clears a selected block), Insert (insert / overwrite), Esc (lets go of a block). F1-F10 type the F-key set.
Every key below can be changed in ` + shownDir(configDir()) + `keys (the file says how; delete it to get these back).` + macKeysNote() + `
` + keysHelp() + "\n" + `A selected block, or one being pasted, can be turned over with the buttons under the colours or the keys above: mirror (left-right), upside down, turn a quarter clockwise, invert (reversed characters on PETSCII and Atari pages, the two colours swapped elsewhere). Characters become their mirrored or turned twins: / and \, the box lines and corners, half blocks, arrows; on PETSCII pages Marq's PETSCII editor's tables. A selection that turns into another shape is lifted off the page to be put down. Copy a block, paste it, turn it, stamp it as often as you like.
"Sync" scrolls two pages together. Dropping files on the window opens them. Keys in the W and H fields stay in the field until Enter, Esc or Tab.
PAGE TYPES: AMIGA, PC, ATARI, ATARI ST, PETSCII, ZX SPECTRUM, TELETEXT, AMSTRAD CPC, MINITEL, VIEWDATA
Amiga pages are Latin-1, read 0x9B as CSI, clear on form feed, keep italic and underline, save with LF line ends. PC pages are CP437 in the VGA font and save with CRLF. Atari pages are ATASCII, the Atari 8-bit computers' text screen: inverse characters (Rev), one background, one text colour for the whole screen (left click a colour) and a border (middle click); .ata files, with EOL line ends and ESC before control characters. Width 38 (the screen editor's margins) or 40 (terminals) is guessed from the file and can be switched; the page is never wider. The font is AltirraOS's, free to pass on. PETSCII pages are exactly as wide as the machine's screen (the Machine gadget sets it): PET 40 or 80 columns (one colour: green, amber or white screen), VIC-20 (22 x 23, 8 text colours), C64 (40 x 25), C128 (40 columns, or 80 on the VDC with its RGBI colours). Each has its two character sets (the Font gadget: upper case + graphics, or lower + upper case), reverse characters, one screen colour; they read and write .seq files, the stream a C64 BBS sends. Switching keeps the bytes and only changes how they are shown; an unchanged file is read again with the other page's rules.
Atari ST pages are the ST's VT52 terminal in one of its resolutions (the Resolution gadget): low 40 x 25 in 16 colours, medium 80 x 25 in 4, high 80 x 25 in black and white with taller characters. Each cell has its own colour and background (ESC b, ESC c); files are the VT52 stream (.vt). The font is EmuTOS's, downloaded on first use.

ZX Spectrum pages are its 32 x 24 screen: a character per cell with ink (left click), paper (right click), bright (colours 8-15; ink and paper share it) and Flash; middle click sets the border. Codes 128-143 are the Spectrum's block graphics, 144-164 its user-defined graphics. Saved as .scr, the 6912-byte screen every Spectrum emulator loads; a .scr that is a drawing opens with each cell as the nearest character.

Teletext pages are 40 x 25 cells of 7-bit codes as on a BBC Micro in Mode 7 or a TV teletext page. Codes 0-31 are attributes that act from their cell to the right, as the teletext chip reads a row: text or graphics colour (F-key sets 1 and 2), double height, flash, conceal, separated, hold, new background. Codes shows their names in their cells. The Sixel tool paints the 2x3 graphics pixels of a cell (the row needs a graphics colour to its left). Files: .tti, .ep1, edit.tf links (.url; a text holding an edit.tf link opens too) and BBC Mode 7 dumps (1000-byte .bin). The characters are Bedstead's (public domain).

Unicode pages (Uni) are for terminals and IRC: any width, any character the built-in Unifont draws (blocks and half blocks, sextants and octants, Braille, box drawing, shapes, arrows, symbols, Latin, Greek, Cyrillic) in the 16 colours, xterm's 256 (the grid under the 16) or any 24-bit colour (the 24-bit... button, or ` + keyName("uni_colour") + `). The character table shows one block at a time (< and >, the wheel, ` + keyName("uni_block_previous") + ` / ` + keyName("uni_block_next") + `); typing takes any character it has. Half paints half blocks in any colours. "VGA + Unifont" draws CP437's characters in the VGA font. Choosing Unicode for a PC or Amiga page converts it exactly (` + keyName("undo") + ` undoes it); back to PC, characters CP437 lacks become ? and colours the nearest of 16. Files: UTF-8 ANSI (.ans, a SAUCE record only when the width needs one), durdraw .dur (frames), mIRC colour codes (.irc; PC pages can save these too; a warning when a line is too long for IRC), HTML, PNG, GIF.

Amstrad CPC pages are the CPC's text screen in one of its modes (the Mode gadget): mode 1, 40 x 25 with 4 inks (the AMSCII compos' main category), mode 0, 20 x 25 with 16 inks, mode 2, 80 x 25 with 2 inks. A cell has a character (all 256 of the ROM's, 0-31 too), a pen and a paper; the inks are the colours, each one of the CPC's 27. In Colours, the inks: left click the pen, right click the paper; under them the 27 colours: left click gives the pen's ink that colour, right click the paper's ink, middle click makes it the border. Saved as .bin (an AMSDOS file that runs on a CPC: RUN"NAME.BIN", or dropped on an emulator), as a .dsk disk image with the .bin on it, or as Playscii .psci for Logiker's Amstrad ASCII Exporter. The exporter's .bin files and disks with them open too. The font is the CPC's ROM: Amstrad has kindly given its permission for its copyrighted material to be redistributed but Amstrad retains its copyright.

Minitel pages (MT) are the Minitel's 40 x 25 screen, row 0 its status row. A cell is a letter (the G0 set, the G2 signs and the accented letters é è à ç ... ) in a colour, or a mosaic of 2 x 3 blocks (the characters from 0xC0 in the table, the Sixel tool) in a colour on a background. A letter has no background of its own: it shows the background, underline and concealing of its zone, which a space starts (a space typed with another background starts a new zone) or a mosaic. Flash, Inv (inverse), Line (underline; separated blocks on mosaics), Mask (conceal) and the size (1x, Wide, Tall, 2x: a big letter is written from its bottom row) go with what you type; Reveal shows concealed cells. The palette is the Minitel 1's grey levels or colour. Saved as .vdt, the stream a Minitel is sent (MiniPavi, MiEdit), each frame a screen of its own; .vdt files open as the screen they draw (screens a clear ends become frames). Play sends the stream to a Minitel on screen. The letters are Frédéric Bisson's Minitel font (public domain).

Viewdata pages (VD) are teletext pages of 24 rows, as Prestel and today's Viewdata boards (Telstar, CCl4) send them: the same codes, attributes and Sixel tool. Saved as a stream (.cpt: CS, rows, attributes as ESC and a letter), a 960-byte frame (.bin) or a Telstar frame (.json; one read from Telstar keeps its other fields), and as teletext .tti or an edit.tf link. A .cpt with several pages (a .car carousel) opens as frames; Play sends the stream to a Viewdata terminal on screen.
SAVE AS
ANSI, plain text, UTF-8 text, PC .bin, PCBoard .pcb, Avatar .avt, BBS colour codes (pipe codes |07 for Mystic and Renegade, Synchronet Ctrl-A .msg, WWIV .msg, Wildcat! .bbs, Celerity), ATASCII .ata, Atari ST .vt, ZX Spectrum .scr, Amstrad CPC .bin and .dsk, teletext .tti, .ep1 and edit.tf links (.url), XBin (.xb, with the font and palette inside, so it looks the same in any viewer), PETSCII .seq, a C64 program (.prg: LOAD and RUN it on a C64 or in VICE, a key goes back to BASIC; with the page's own character set, multicolour or ECM when it has them), a C64 disk with that program (.d64), the C array of Marq's PETSCII editor and Petmate (.c), a Petmate workspace (.petmate), pictures (.png, normal or twice the size; C64 pages with their border), a web page (.html) and an animation of the frames (.gif, or APNG .png). Typing a file ending picks the format. Saving under the name of another file that is already there asks first.
For other editors (at the end of the list, only for the pages they fit): iCEDraw .idf, ArtWorx .adf (80 columns) and TundraDraw .tnd (80 columns, 24-bit colours, no font) for PC and Amiga pages; IcyDraw .icy (font, palette and iCE inside; opens in IcyDraw as the page); REXPaint .xp (PC); Playscii .psci (PC, Amiga, PETSCII, Atari, Amstrad CPC; every frame with its hold; Playscii's own charsets and palettes by name, so an own font stays with CharEdit); and for a C64: the PETSCII editor's .pet, its BASIC listing (.bas, for petcat) and ACME source (.s), and Petmate's JSON export (.json, every screen). The ones that open again (.idf, .adf, .tnd, .icy, .xp, .psci, .pet, .json) become the page's file: Save writes the same format again.
OPENING
Everything above that holds characters opens again: ANSI, text, .bin, XBin, .pcb, .avt, BBS colour codes (found by their bytes, also mixed with ANSI; Open as picks one; a board's code that cannot be shown stays as text and is saved back), .ata (also as .txt, recognised by its bytes), .vt, .scr, CPC .bin and .dsk, .tti, .ep1, edit.tf links, .seq, .c, .petmate, and C64 programs saved by CharEdit; also iCEDraw .idf, ArtWorx .adf, TundraDraw .tnd, IcyDraw .icy (PC pages; its layers are put together into one page, picture layers and layer effects are left out), REXPaint .xp (layers put together), Playscii .psci (charsets dos, amiga_topaz, atari, the C64 ones, commodore_pet and the Amstrad exporter's; layers put together, frames kept), the PETSCII editor's .pet and Petmate's JSON export. A .c, .petmate, .psci or Petmate .json with several screens and a .tti carousel open as frames. Web pages do not open; a picture opened or dropped can be the page's reference (below).
REFERENCE PICTURE
A picture (PNG, JPEG, GIF, BMP, WebP) to trace over: the Ref gadget, ` + keyName("ref_open") + `, or drop it on the window. It shows in the page's empty cells (or faintly over everything), and can be faded, hidden, shown in greys, moved (` + keyName("ref_move") + `: drag or arrows, Shift a pixel, + and - size it; or Ctrl+drag) and sized (fit width, fit page, 1:1, bigger, smaller). It is only shown: never turned into characters, never saved or exported with the art. CharEdit remembers it with the art file (in prefs.json) and in the autosave.
FRAMES
A page can hold several frames: ` + keyName("frame_new") + ` adds an empty one, ` + keyName("frame_duplicate") + ` copies the frame, ` + keyName("frame_previous") + ` / ` + keyName("frame_next") + ` go through them. The strip under the page numbers them (click to go, drag to move) and has each frame's hold time (how long it shows when played) and New, Copy, Delete, Onion (the frame before, faint, under the empty cells), Play, Loop and the speed. Width and crop change every frame; each frame has its own height. Undo goes back through all of it, to the frame each step changed.
Saved as .c, .petmate or .tti every frame is kept (a .tti keeps each subpage's other lines; its hold is the CT line). ANSI, .seq, .ata and .vt save the frames as one stream with a clear screen between them; their speed is the modem's, so the hold times are not kept (in an .ans, My BBS's pauses keep them: see BBS pauses). A file that clears the screen between pictures opens as one picture and says so: ` + keyName("frames_strip") + ` shows the strip with Split (at each clear, or at each cursor home). Formats of one picture (.scr, .xb, .png ...) write the frame on show, and the page stays unsaved. Save as .gif or APNG for an animation.
ANIMATION TEMPLATES
` + keyName("animate") + ` (TheDraw's Easy Animation) puts a picture on the screen cell by cell in a pattern: TheDraw's Top, Left, Clock, Circles, Checkerboard, Angle, Gate, Pyramid, Squares, Scramble, Door, Diamond, Wiggle and Wiggle out, with Reverse (the whole order backwards) and Flip (rows mirrored top to bottom), and Charcoal's own Fade in / out (the colours come up out of the paper), Typewriter, Scroll in, Matrix rain, Decrypt, Spiral, Drop, Assemble, Blinds, Shine, Shade in, Melt in and Power on. A preview plays it; the effects' random parts come from a seed kept with the page (Shuffle or S picks another). The frame effects (Matrix rain to Power on but Spiral and Blinds) saved as the file write a delta stream: each frame's changed cells only. Make "the draw order of the saved file": saving as .ans, .seq, .ata or .vt writes the picture in that order (a clear screen, then each cell with a cursor move where it does not follow the one before; only cells that show something), so any terminal plays it at the modem's speed; Play shows it at the speed in the bar; the order stays with the page (status line "Draws:") until changed or cleared. Or make frames: one every N cells (or every N ms at the modem's speed), each held so long; Erase makes the picture go away instead (TheDraw's Fadeout). Teletext and Spectrum pages make frames only.
Size and cost: an animated file is paced by the caller's line (each byte is 10 bits), so Detail (D) has three levels: Small (for modems: a draw order puts down runs of up to 8 cells together and costs about a tenth more than the plain file; the frame effects take fewer frames, held longer, with little or no glyph churn, short trails and short flights), Medium (the default) and Large (the full look). The lists mark each template light, medium or heavy on this page at that level (one, two, three bars: up to 1.5x, up to 4x, more than 4x the plain file). Under them: the saved file's size and how long it plays at 2400, 9600, 14400, 33600 and 57600 bps and over telnet (bytes x 10 / bps plus the board's pauses; telnet only the pauses), with a warning past the time set in Settings (30 s) at the caller's speed. The preview plays the saved file as a caller at that speed sees it, with the board's pauses; Play does the same. The caller's speed is the speed in the bar.
BBS pauses: over telnet with no speed limit an animated file arrives at once and the effect flashes past. Settings' My BBS (default none) puts that board's own pause codes into every animated .ans you save (a draw order, a frame effect, or the strip's frames with their holds); Animate can choose another board, or none, for one page, and tick boxes save one copy per board beside the file (name-amiexpress.ans ...). A draw order pauses N ms every N cells, a frame effect after each frame. Ami-Express (/X): ~NS| first, ~<n>w| (n/50 s); Mystic: |PO first, |DE (half a second); PCBoard: @POFF@ first, @DELAY:n@ (n/10 s); Synchronet: Ctrl-A , ; . (0.1, 0.5, 2 s); Tempest: ~DELAY(n) (n/50 s, from its manual). Mystic, PCBoard and Synchronet files also get the board's own colour codes (|nn, @Xnn, Ctrl-A) instead of ANSI colours, unless a colour has no code there (blink on Mystic: the colours stay ANSI). Each board's code character in the art is written the board's way. None is the safe choice for art packs, 16colo.rs and several boards: another board's codes show as junk text. C64 .seq files: Image BBS (£Sx: the next character x/10 s later; a £ in the art as ££; lines of 80 bytes, a RETURN at the end) and C*Base (Ctrl-P: about 1/30 s; a £ or @ in the art gets a reverse code after it). Not offered: Image's £W (whole seconds, and no wait at all in the 2.0 source), Color 64 (£v speed of unknown units only), C-Net, DMBBS, Centipede (their codes are not known here). ATASCII and VT52 files get none (no board's codes known). A .seq with Image BBS's £ codes (£C colours, £F clear, £R reverse, ££) opens with them (or Open as: C64: Image BBS £ codes); its other codes stay as text and Save writes them back.
Animate a folder (` + keyName("animate_folder") + `, or the Open requester's button): every screen of a folder (and its subfolders, if ticked) animated for your board at once, without opening each, into a new folder (the folder's name + "-animated"; the names kept; files of other kinds such as .bin, .xb, .pcb are written as .ans). The originals are never touched, unless "Replace the originals" is ticked: then each is kept as .bak. The effect: one for all, or Random, a different one for each file, from all or the ticked ones, never the same twice in a row; Reverse and Flip at random; the seed is shown and can be typed, so a run can be repeated exactly. The pace and the board as in Animate (My BBS, None, or a board; copies for other boards). Versions of each screen: N animations of every screen, each with its own effect, named the way the board picks one at random: Ami-Express 001.NAME.TXT ... with NAME.TXT holding ~<n>SR_<screens folder>NAME|; Mystic name.ans, name.an1 ... name.an8; Synchronet name.ans, name1.ans ... (shown at random by a menu name with * in it); PCBoard and Tempest have no random screens known, so NAME-2 ... are just numbered. Files that are not art, teletext and Spectrum pages, files that are already animations and files of a kind the board has no codes for (.seq on a PC board, .ans on a C64 board, .ata and .vt on all) are skipped and the report says why. Detail as in Animate (Medium by default); Effects: any weight, light only or light and medium (weighed on each screen); a budget: at most N KB a screen and/or plays in under N s at a speed. With a budget each version takes an effect that fits: the chosen level, then lower ones, then another effect; a screen nothing fits is skipped with the reason. A progress bar with Cancel, then the report (each file, the effect and seed, each output's size and play time at the speed, what was skipped), saved as animate-report.txt in the output folder. On the command line: charedit --animate FOLDER (charedit --help for the options).
The Commodore character sets are the machines' ROMs and do not come with the program: the first run offers to download them from the VICE emulator project (each checked by its SHA-1), and so does Settings.

FONTS
The Font gadget picks the page's font: on PC pages IBM VGA, EGA 8x14, VGA 8x8 (80 x 50) and the code pages 850 and 866 (VileR's Ultimate Oldschool PC Font Pack); a SAUCE font name (IBM VGA, IBM EGA, IBM VGA50, IBM VGA 850 ...) chooses it when a file opens and is written when it is saved. "Load a font..." (` + keyName("font_load") + `, or drop the file on the window) takes a font of your own for the page: PC raw fonts (.fnt .f08 .f14 .f16, 256 characters of 8, 14 or 16 rows), PSF, an XBin's font; C64, VIC-20 and PET character sets (.64c, .bin: 2048 bytes a set, 4096 both); Atari 8-bit .fnt (1024 bytes); Spectrum sets (768 bytes) and UDGs (168 bytes). The font is named after its file. XBin and Petmate files keep the font inside; for every other format CharEdit remembers the font with the file (the status line says so) and so do the autosaves.
The font editor (` + keyName("font_editor") + `, or "Edit the font..."): the brush character's pixels in a grid; left click or drag sets them (a set pixel clicked clears), right clears; Invert (I), Flip H (H), Flip V (V), Clear (Delete), Move (Shift+arrows), Copy and Paste (Ctrl+C, Ctrl+V), Undo and Redo (Ctrl+Z, Ctrl+Y); arrows or a click in the character map pick another character. The page shows every change at once. Editing a built-in font makes a copy of it, the page's own. Save font... writes it as a file: the name's ending says the kind (.psf, .f08/.f14/.f16, .64c, .fnt, .ch8, .udg). On a C64 page in multicolour with a brush colour of 8-15 it draws pixel pairs: the Pen (keys 1-3) is MC 1, MC 2 or the colour, right click the screen colour; in ECM it edits glyphs 0-63.

COMING FROM MOEBIUS, PABLODRAW, THEDRAW OR ACIDDRAW
Settings > Keys like puts CharEdit's actions on that editor's keys (the keys file gets a "preset = moebius" line; your own lines still win; the file before is kept as keys.bak). The KEYS list above shows the keys in use. All of them: F-key sets on Alt+F1-F10 (set 4 on Ctrl+F4: Alt+F4 closes windows). Moebius: Ctrl+F 9-pixel font, Ctrl+, / Ctrl+. previous / next set, Ctrl+Tab reference picture, Alt+B select, Ctrl+Alt+M mirror. PabloDraw: Alt+P / Alt+B draw, Alt+L / R / O line, box, ellipse, Alt+I pick, Alt+E select, Ctrl+PgUp / PgDn top / bottom. TheDraw: Alt+L load, Alt+S save, Alt+X quit, Alt+H help, Alt+R undo, Alt+I / Alt+Y insert / delete a row, Alt+P fill, Alt+- colour, Alt+B block, Ctrl+Up foreground +1. ACiDDraw: the same but Ctrl+I / Ctrl+Y for rows. An action the other editor has not got keeps CharEdit's key unless that key went to another action; then it has none until you give it one in the keys file.
Not the same: those editors' block keys (M, C, F, R, X, Y ... while a block is up) and Moebius's tool letters are plain letters, which type here: use the block buttons or Alt+H / V / R. Moebius files open as Moebius shows them: its width (1 column and up), its height (the empty rows at the end), iCE, 9-pixel letter spacing (` + keyName("ninepx") + `), its font name, SAUCE text in UTF-8; saving keeps all of it. Characters 0, 7, 8 and 9 (• ◘ ○) are drawn in a file with SAUCE and saved as they were; on new pages they are saved as spaces, since a BBS terminal beeps, backs up and tabs on them.

SETTINGS
Kept in ` + shownDir(configDir()) + `prefs.json (a broken one is put aside as prefs.json.bad); the keys in ` + shownDir(configDir()) + `keys. Settings: the window size (1x to 4x for big screens), keys like another editor's (above), keeping the old version of a file as name.bak when a save writes over it (on unless switched off), and the downloads, which run while you work on.

SAFETY
Every two minutes, pages with changes are kept aside (in ` + shownDir(autosaveDir()) + `). If CharEdit stops without saving them (a crash, the power), the next start offers to open them again in tabs. Saving a page or closing it on purpose lets go of its copy. The Open requester's "Recent files" lists the last 10 files opened or saved.`
}

func (d *HelpReq) draw(u *UI) {
	fb := u.fb
	in := u.window(centre(900, ui.fb.h*5/6), "CharEdit "+version+" - keys and mouse")
	lines := wrap(helpText(), in.w-digitsW(3))
	textR := Rect{in.x, in.y, in.w - 20, in.h - gadH - 10}
	rows := textR.h / lineH
	if w := u.takeWheel(textR); w != 0 {
		d.top += w * 3
	}
	d.top = u.scrollbar("help", Rect{textR.x + textR.w + 2, textR.y, 18, textR.h}, true, d.top, rows, len(lines))
	for i := d.top; i < min(len(lines), d.top+rows); i++ {
		col := uint32(cText)
		if l := lines[i]; l != "" && l == strings.ToUpper(l) {
			col = cHi
		}
		fb.text(textR.x, textR.y+(i-d.top)*lineH, lines[i], col)
	}
	if buttonRow(u, in, "OK") == 0 {
		closeDialog()
	}
}

func (d *HelpReq) key(k string, ctrl, shift bool) bool {
	switch k {
	case "Escape", "Return", "Keypad Enter":
		closeDialog()
	case "Down":
		d.top++
	case "Up":
		d.top = max(0, d.top-1)
	case "PageDown":
		d.top += 20
	case "PageUp":
		d.top = max(0, d.top-20)
	default:
		return false
	}
	return true
}

// ---------------------------------------------------------------- fonts to download

func c64Why() string {
	return "PETSCII pages need the Commodore character sets (PET, VIC-20, C64, C128). They are the machines' " +
		"own ROMs, so they do not come with this program, but the VICE emulator project hands them out. Download them " +
		"from VICE now? (18 KB, each checked before it is used, kept in " + shownDir(appDataDir()) + ".) You can also do this " +
		"later under Settings."
}

func stWhy() string {
	return "Atari ST pages need the ST's font. Atari's own is in the TOS ROM; EmuTOS, the free TOS (GPL), has " +
		"its own version of it. Download it from EmuTOS now? (three small files, checked before they are used, kept in " +
		shownDir(appDataDir()) + ".) You can also do this later under Settings."
}

// FontReq asks to download fonts that may not come with the program.
type FontReq struct {
	title, why string
	what       string                 // what is downloading, for the status line
	fetch      func() error           // the network part: runs in the background
	load       func() (string, error) // takes the files in (main thread); the message once they are here
	firstRun   bool                   // the C64 question asked on the first run
	after      func()                 // what to do once they are here
	err        string
}

func openC64Req(after func()) {
	a.dialog = &FontReq{title: "Commodore character sets", why: c64Why(), firstRun: true, after: after,
		what: "the Commodore character sets", fetch: fetchC64ROMs, load: loadC64Downloaded}
}

func openSTReq(after func()) {
	a.dialog = &FontReq{title: "Atari ST font", why: stWhy(), after: after,
		what: "the Atari ST font", fetch: fetchSTFonts, load: loadSTDownloaded}
}

func (d *FontReq) answered() {
	if d.firstRun {
		prefs.C64Asked = true
		savePrefs()
	}
}

// download starts the download and closes the requester; the window goes on
// working meanwhile. If it fails, the requester comes back with the reason.
func (d *FontReq) download() {
	ok := startDownload(d.what, d.fetch, func(err error) {
		done, lerr := d.load() // what did come is taken in even when something failed
		if err == nil {
			err = lerr
		}
		if err != nil {
			d.err = "Could not download: " + err.Error()
			if a.dialog == nil {
				a.dialog = d
			} else {
				a.msg(d.err)
			}
			return
		}
		d.answered()
		a.msg(done)
		if d.after != nil {
			d.after()
		}
	})
	if ok {
		closeDialog()
	}
}

func (d *FontReq) draw(u *UI) {
	in := u.window(centre(600, 260), d.title)
	y := in.y
	for _, l := range wrap(d.why, in.w) {
		u.fb.text(in.x, y, l, cText)
		y += lineH
	}
	if d.err != "" {
		for _, l := range wrap(d.err, in.w) {
			u.fb.text(in.x, y+6, l, cOrange)
			y += lineH
		}
	}
	switch buttonRow(u, in, "Download", "Not now") {
	case 0:
		d.download()
	case 1:
		d.answered()
		closeDialog()
	}
}

func (d *FontReq) key(k string, ctrl, shift bool) bool {
	switch k {
	case "Return", "Keypad Enter":
		d.download()
	case "Escape":
		d.answered()
		closeDialog()
	default:
		return false
	}
	return true
}

// ---------------------------------------------------------------- downloads in the background

// download is the one download running, if any. The network part runs in a
// goroutine; what it got is taken in on the main thread, which alone touches
// SDL and the font list.
var download struct {
	what   string // shown in the status line while it runs; "" when none runs
	done   chan error
	finish func(error)
}

// startDownload runs fetch in the background and finish on the main thread
// once it is done. It says no while another download runs.
func startDownload(what string, fetch func() error, finish func(error)) bool {
	if download.what != "" {
		a.msg("Wait: still downloading " + download.what)
		return false
	}
	ch := make(chan error, 1)
	download.what, download.done, download.finish = what, ch, finish
	go func() { ch <- fetch() }()
	a.msg("Downloading " + what + " ...")
	return true
}

// pollDownload finishes a download that is done; it tells whether one was.
func pollDownload() bool {
	if download.what == "" {
		return false
	}
	select {
	case err := <-download.done:
		finish := download.finish
		download.what, download.done, download.finish = "", nil, nil
		finish(err)
		return true
	default:
		return false
	}
}

// fetchC64ROMs gets the Commodore character ROMs in the background; only the
// C64's must come (loadC64Downloaded names any others that did not).
func fetchC64ROMs() error {
	_, first := fetchChargen()
	if cbmROMs[0].onDisk() == nil {
		return first
	}
	return nil
}

// loadC64Downloaded takes the downloaded ROMs in and names any that did not come.
func loadC64Downloaded() (string, error) {
	ok := loadC64Fonts()
	a.hasC64 = hasROM("c64") // also after a failed or partial download: what came is used
	if !ok {
		return "", fmt.Errorf("saved the character sets but could not read them back")
	}
	var missing []string
	for _, r := range cbmROMs {
		if !hasROM(r.family) {
			missing = append(missing, r.name)
		}
	}
	if len(missing) > 0 {
		return "The C64 character sets are here, but not these: " + strings.Join(missing, ", ") + " (try again under Settings)", nil
	}
	return "The Commodore character sets are here: PETSCII pages work now", nil
}

func loadSTDownloaded() (string, error) {
	if !loadSTFonts() {
		return "", fmt.Errorf("saved the EmuTOS fonts but could not read them")
	}
	return "The ST font is here: Atari ST pages work now", nil
}

// ---------------------------------------------------------------- settings

type SettingsReq struct {
	err  string
	warn Field // Animate's warning, seconds (animcost.go)
}

func openSettings() {
	d := &SettingsReq{warn: Field{max: 4}}
	d.warn.set(strconv.Itoa(animWarnMs() / 1000))
	d.warn.enter = func() { ui.focus = nil }
	a.dialog = d
}

func (d *SettingsReq) draw(u *UI) {
	fb := u.fb
	in := u.window(centre(660, 520+gadH+8+2*(gadH+8)+lineH+16), "Settings")
	y := in.y
	y = d.drawAppearance(u, in, y)
	fb.text(in.x, y+3, "Window size", cText)
	sizes := []string{"1x (normal)", "2x (big screens)", "3x", "4x"} // as many as main allows (maxUIScale)
	u.choice("uiscale", Rect{in.x + settingsCol(), y, choiceW(sizes), gadH}, sizes, scale-1, "", func(i int) {
		scale = i + 1
		savePrefs()
	})
	y += gadH + 8
	y = drawKeysLike(u, in, y) // keypresets.go
	if v, _ := u.check(in.x, y, "Keep the old version as name.bak when saving over a file", !prefs.NoBackup, ""); v == prefs.NoBackup {
		prefs.NoBackup = !v
		savePrefs()
	}
	y += gadH + 8
	// My BBS: the board whose pause codes animated files get (bbspause.go).
	fb.text(in.x, y+3, "My BBS", cText)
	boards := []string{"None (plays at the modem's speed)"}
	cur := 0
	for i, b := range bbsBoards {
		boards = append(boards, b.name)
		if b.id == prefs.MyBBS {
			cur = i + 1
		}
	}
	u.choice("mybbs", Rect{in.x + settingsCol(), y, choiceW(boards), gadH}, boards, cur,
		"Animated files you save (draw orders, frame effects, frames) get this board's pause codes, so they play at their pace over telnet too. None is the safe choice for art packs, 16colo.rs and several boards: another board's codes show as junk text. Animate can choose for one page.", func(i int) {
			prefs.MyBBS = ""
			if i > 0 {
				prefs.MyBBS = bbsBoards[i-1].id
			}
			savePrefs()
		})
	y += gadH + 8
	x := fb.text(in.x, y+3, "Animate warns when a file plays longer than", cText) + 8
	u.field(Rect{x, y, digitsW(4) + 16, gadH}, &d.warn, "Seconds at the caller's speed chosen in Animate (the speed in the bar)")
	fb.text(x+digitsW(4)+24, y+3, "s", cText)
	if v, err := strconv.Atoi(d.warn.String()); err == nil && v > 0 && v != prefs.AnimWarn && v <= 9999 {
		prefs.AnimWarn = v
		savePrefs()
	}
	y += gadH + 4
	y += 12
	fb.text(in.x, y, "Commodore character sets (PETSCII)", cHi)
	y += lineH + 4
	var have, missing []string
	for _, r := range cbmROMs {
		if hasROM(r.family) {
			have = append(have, r.name)
		} else {
			missing = append(missing, r.name)
		}
	}
	status := "Here: " + strings.Join(have, ", ") + "."
	if len(have) == 0 {
		status = "Not here yet: PETSCII pages need them."
	} else if len(missing) > 0 {
		status += " Missing: " + strings.Join(missing, ", ") + "."
	}
	for _, l := range wrap(status, in.w) {
		fb.text(in.x, y, l, cText)
		y += lineH
	}
	label := "Download from VICE"
	if len(missing) == 0 {
		label = "Download again"
	}
	if download.what != "" {
		label = "Downloading ..."
	}
	if u.button(Rect{in.x, y + 6, buttonW(label), gadH}, label, false, "") != nil {
		d.err = ""
		startDownload("the Commodore character sets", fetchC64ROMs, func(err error) {
			done, lerr := loadC64Downloaded() // what did come is taken in even when something failed
			if err == nil {
				err = lerr
			}
			if err != nil {
				d.err = "Could not download: " + err.Error()
				a.msg(d.err)
				return
			}
			prefs.C64Asked = true
			savePrefs()
			a.msg(done)
		})
	}
	y += gadH + 18
	fb.text(in.x, y, "Atari ST font (EmuTOS)", cHi)
	y += lineH + 4
	stLabel := "Download from EmuTOS"
	if hasSTFont() {
		fb.text(in.x, y, "Here.", cText)
		stLabel = "Download again"
	} else {
		fb.text(in.x, y, "Not here yet: Atari ST pages need it.", cText)
	}
	if download.what != "" {
		stLabel = "Downloading ..."
	}
	if u.button(Rect{in.x, y + lineH + 4, buttonW(stLabel), gadH}, stLabel, false, "") != nil {
		d.err = ""
		startDownload("the Atari ST font", fetchSTFonts, func(err error) {
			done, lerr := loadSTDownloaded() // what did come is taken in even when something failed
			if err == nil {
				err = lerr
			}
			if err != nil {
				d.err = "Could not download: " + err.Error()
				a.msg(d.err)
				return
			}
			a.msg(done)
		})
	}
	if d.err != "" {
		for i, l := range wrap(d.err, in.w) {
			fb.text(in.x, y+lineH+gadH+12+i*lineH, l, cOrange)
		}
	}
	if buttonRow(u, in, "OK") == 0 {
		closeDialog()
	}
}

func (d *SettingsReq) key(k string, ctrl, shift bool) bool {
	switch k {
	case "Escape", "Return", "Keypad Enter":
		closeDialog()
	default:
		return false
	}
	return true
}

// drawAppearance draws Settings' rows for the interface's look and returns the y under them.
func (d *SettingsReq) drawAppearance(u *UI, in Rect, y int) int {
	fb := u.fb
	col2 := in.x + in.w/2
	fb.text(in.x, y+3, "Theme", cText)
	names := themeNames()
	u.choice("theme", Rect{in.x + settingsCol(), y, choiceW(names), gadH}, names, max(0, slices.Index(names, theme.Name)),
		"The interface's colours (also in the View menu)", func(i int) { setTheme(names[i]) })
	fb.text(col2, y+3, "Layout", cText)
	layouts := []string{"Studio", "Focus"}
	u.choice("uilayout", Rect{col2 + colAfter(70, "Layout", "Size"), y, choiceW(layouts), gadH}, layouts, map[bool]int{false: 1}[a.studio()],
		"Studio: the tools on the left, a panel on the right. Focus: the page as wide as the window, the gadgets under it", func(i int) {
			a.setUILayout([]string{"studio", "focus"}[i])
		})
	y += gadH + 8
	fb.text(in.x, y+3, "Interface font", cText)
	var fonts []string
	cur := 0
	for i, f := range uiFontChoices {
		name := f.name
		if f.id == "file" && prefs.UIFont == "file" {
			name = filepath.Base(prefs.UIFontFile)
		}
		fonts = append(fonts, name)
		if f.id == prefs.UIFont {
			cur = i
		}
	}
	u.choice("uifont", Rect{in.x + settingsCol(), y, choiceW(fonts), gadH}, fonts, cur,
		"The font of the menus, gadgets and requesters (the art keeps its own)", func(i int) {
			id := uiFontChoices[i].id
			if id == "file" {
				openUIFontRequester(d)
				return
			}
			prefs.UIFont = id
			applyUIFont()
			savePrefs()
		})
	if prefs.UIFont != "topaz" {
		fb.text(col2, y+3, "Size", cText)
		var sizes []string
		scur := 0
		size := prefs.UIFontSize
		if size == 0 {
			size = defaultUIFontSize
		}
		for i, s := range uiFontSizes {
			sizes = append(sizes, strconv.Itoa(s)+" pixels")
			if s == size {
				scur = i
			}
		}
		u.choice("uifontsize", Rect{col2 + colAfter(70, "Layout", "Size"), y, choiceW(sizes), gadH}, sizes, scur, "The interface font's size", func(i int) {
			prefs.UIFontSize = uiFontSizes[i]
			applyUIFont()
			savePrefs()
		})
	}
	y += gadH + 8
	fb.fill(Rect{in.x, y, in.w, 1}, cEdgeDk)
	return y + 10
}

// openUIFontRequester asks for a .ttf or .otf to be the interface font; back is Settings.
func openUIFontRequester(back Dialog) {
	d := &FileReq{sel: -1, title: "Choose the interface font", note: "A TrueType or OpenType font: .ttf, .otf or .ttc", back: back}
	d.listFirst(startDir(a.lastDir))
	d.path.enter = d.enter
	d.pick = func(p string) {
		prefs.UIFont, prefs.UIFontFile = "file", p
		applyUIFont()
		savePrefs()
		a.dialog = back
	}
	a.dialog = d
	ui.focus = &d.path
}
