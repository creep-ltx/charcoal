package main

// The requesters: open / save (a file list), info (SAUCE), colour, help, and yes / no.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
func centre(w, h int) Rect {
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

type Confirm struct {
	text   string
	button string // the yes button, "Throw away" if empty
	yes    func()
}

func (d *Confirm) draw(u *UI) {
	in := u.window(centre(460, 140), "Charcoal")
	lines := wrap(d.text, in.w/charW)
	for i, l := range lines {
		u.fb.text(in.x, in.y+i*lineH, l, cText)
	}
	yes := d.button
	if yes == "" {
		yes = "Throw away"
	}
	switch buttonRow(u, in, yes, "Cancel") {
	case 0:
		closeDialog()
		d.yes()
	case 1:
		closeDialog()
	}
}

func (d *Confirm) key(k string, ctrl, shift bool) bool {
	switch k {
	case "Return", "Keypad Enter", "Y":
		closeDialog()
		d.yes()
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
}

type FileReq struct {
	save    bool
	dir     string
	entries []entry
	sel     int
	top     int
	path    Field
	choice  int // open: auto / Amiga / PC; save: the format
	err     string
	lastClk time.Time
	lastSel int
}

var openAs = []string{"auto", "amiga", "pc", "c64"}
var openAsNames = []string{"detect", "Amiga", "PC", "C64 PETSCII"}

func openOpenRequester() {
	d := &FileReq{sel: -1}
	dir := a.lastDir
	if dir == "" {
		dir, _ = os.Getwd()
	}
	d.list(dir)
	d.path.enter = d.ok
	a.dialog = d
	ui.focus = &d.path
}

func openSaveRequester() {
	d := &FileReq{save: true, sel: -1}
	dir := a.lastDir
	if a.path != "" {
		dir = filepath.Dir(a.path)
	}
	if dir == "" {
		dir, _ = os.Getwd()
	}
	d.list(dir)
	d.path.set(filepath.Join(dir, a.name))
	for i, f := range a.modeFormats() {
		if f.id == a.saveFmt {
			d.choice = i
		}
	}
	d.path.enter = d.ok
	a.dialog = d
	ui.focus = &d.path
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
		d.entries = append(d.entries, entry{e.Name(), isDir, info.Size()})
	}
	rest := d.entries[1:]
	sort.Slice(rest, func(i, j int) bool {
		if rest[i].dir != rest[j].dir {
			return rest[i].dir
		}
		return strings.ToLower(rest[i].name) < strings.ToLower(rest[j].name)
	})
	d.sel, d.top = -1, 0
	if !d.save {
		d.path.set(dir + string(filepath.Separator))
	} else {
		d.path.set(filepath.Join(dir, filepath.Base(d.path.String())))
	}
}

func expandHome(p string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		h, _ := os.UserHomeDir()
		return h + p[1:]
	}
	return p
}

func (d *FileReq) activate(i int) {
	e := d.entries[i]
	if e.dir {
		if e.name == ".." {
			d.list(filepath.Dir(d.dir))
		} else {
			d.list(filepath.Join(d.dir, e.name))
		}
		return
	}
	d.path.set(filepath.Join(d.dir, e.name))
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
		d.list(p)
		return
	}
	if strings.HasSuffix(d.path.String(), "/") {
		d.err = "No such drawer: " + p
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
		closeDialog()
		a.writeOut(p, formats[d.choice].id)
		return
	}
	closeDialog()
	a.openPath(p, openAs[d.choice])
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
	in := u.window(centre(720, ui.fb.h*4/5), title)
	fb.text(in.x, in.y, "Drawer: "+d.dir, cText)
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
		}
	}
	fb.clip = old
	y := listR.y + listR.h + 8
	fb.text(in.x, y+3, "File", cText)
	u.field(Rect{in.x + 48, y, in.w - 48, gadH}, &d.path, "Type a path; Enter opens it (a drawer lists it)")
	y += gadH + 8
	if d.save {
		fb.text(in.x, y+3, "Format", cText)
		names := []string{}
		for _, f := range a.modeFormats() {
			names = append(names, f.name)
		}
		u.choice("fmt", Rect{in.x + 64, y, choiceW(names), gadH}, names, d.choice, "", d.setFormat)
	} else {
		fb.text(in.x, y+3, "Open as", cText)
		names := openAsNames
		if !a.hasC64 {
			names = names[:3]
		}
		u.choice("as", Rect{in.x + 72, y, choiceW(names), gadH}, names, d.choice, "", func(i int) { d.choice = i })
	}
	if d.err != "" {
		fb.text(in.x, in.y+in.h-gadH-lineH-6, d.err, cHi)
	}
	label := "Open"
	if d.save {
		label = "Save"
	}
	switch buttonRow(u, in, "Parent", "Home", label, "Cancel") {
	case 0:
		d.list(filepath.Dir(d.dir))
	case 1:
		h, _ := os.UserHomeDir()
		d.list(h)
	case 2:
		d.ok()
	case 3:
		closeDialog()
	}
}

func (d *FileReq) key(k string, ctrl, shift bool) bool {
	rows := max(1, (ui.fb.h*4/5-3*gadH-lineH-80)/lineH)
	move := func(n int) {
		d.sel = max(0, min(len(d.entries)-1, d.sel+n))
		if d.sel < d.top {
			d.top = d.sel
		}
		if d.sel >= d.top+rows {
			d.top = d.sel - rows + 1
		}
		if e := d.entries[d.sel]; !e.dir {
			d.path.set(filepath.Join(d.dir, e.name))
		} else if !d.save {
			d.path.set(d.dir + string(filepath.Separator))
		}
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
		if d.sel >= 0 && (d.entries[d.sel].dir || !d.save) && (d.sel == 0 || d.entries[d.sel].dir || strings.HasSuffix(d.path.String(), d.entries[d.sel].name)) {
			d.activate(d.sel)
		} else {
			d.ok()
		}
	case "Escape":
		closeDialog()
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
	a.info = Info{title: d.title.String(), author: d.author.String(), group: d.group.String(), writeSauce: d.sauce, clear: d.clear}
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
		u.field(Rect{in.x + 72, y, (f.fl.max+2)*charW + 8, gadH}, f.fl, "")
		y += gadH + 6
	}
	y += 4
	d.sauce, _ = u.check(in.x, y, "Write a SAUCE record when saving (width, font, title)", d.sauce, "")
	y += gadH + 4
	d.clear, _ = u.check(in.x, y, "Start the file with clear screen (form feed on Amiga, ESC[2J on PC)", d.clear, "")
	y += gadH + 10
	for _, l := range wrap(describeSauce(a.sauce), in.w/charW) {
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

const helpText = `MOUSE
Draw: left paints the brush (character + colours), right erases, Shift+click draws a line from the last point.
Colour: left sets the foreground, right the background (on a C64 page: the foreground only).
Fill: left fills equal cells with the brush.
Half (PC): left paints half-cell pixels in the foreground, right in the background colour.
Select: drag a block. While pasting, the block follows the mouse: left click stamps it, Shift+click stamps it without its blank cells, right click or Esc stops.
Middle click or Pick: take a cell's character and colours.
Characters strip: click = brush (types it with Type). Right click = put on the active F-key.
Colours: left = foreground, right = background (C64: the screen colour; middle click: the border), Shift+click = change the colour.
Pop-up gadgets: click opens the list, right click steps to the next item.
Wheel scrolls the page, Shift+wheel sideways, Ctrl+wheel zooms around the mouse.
Tabs: click to show, x or middle click closes, + opens a new page. With two pages on show, click a page to work on it.

KEYS
Typing writes at the cursor in the brush colours (any tool). Arrows, Home, End, PgUp, PgDn, Enter, Tab, Backspace, Delete, Insert.
F1-F10 type the F-key set, Ctrl+Up/Down change set.
Ctrl+Left/Right foreground -/+, Ctrl+Shift+Left/Right background -/+.
Ctrl+Z undo, Ctrl+Y redo, Ctrl+S save, Ctrl+Shift+S save as, Ctrl+O open, Ctrl+Q quit.
Ctrl+T (or Ctrl+N) new page, Ctrl+W close page, Ctrl+Tab / Ctrl+PgDn next page, Ctrl+Shift+Tab / Ctrl+PgUp previous.
Ctrl+L one page / side by side / top and bottom. "Sync" scrolls both together.
Ctrl+A select all, Ctrl+C / X / V copy, cut, paste (text from other programs too), Del clears the block, Esc lets go.
Alt+H flips the block (or the pasted block) left-right, Alt+V upside down; characters turn into their mirror images.
Ctrl+G grid. Ctrl+Plus / Ctrl+Minus zoom, Ctrl+0 fits the page's width. Ctrl+R reverse (C64).
Ctrl+H hides all gadgets (only the art and the status line), Ctrl+H again brings them back.
Dropping files on the window opens them.

AMIGA, PC AND C64
Amiga pages are Latin-1, read 0x9B as CSI, clear on form feed, keep italic and underline, save with LF line ends. PC pages are CP437 in the VGA font and save with CRLF. C64 pages are PETSCII: 40 columns, the C64's two character sets (the Font gadget), reverse characters, one screen colour; they read and write .seq files, the stream a C64 BBS sends. Switching keeps the bytes and only changes how they are shown; an unchanged file is read again with the other page's rules.
SAVE AS
ANSI, plain text, UTF-8 text, PC .bin, XBin (.xb, with the font and palette inside, so it looks the same in any viewer), PETSCII .seq, a C64 program (.prg: LOAD and RUN it on a C64 or in VICE, a key goes back to BASIC), the C array of Marq's PETSCII editor and Petmate (.c), pictures (.png, normal or twice the size; C64 pages with their border) and a web page (.html). Typing a file ending picks the format. XBin and .c files open again.
The C64 character set is the C64's ROM and does not come with the program: the first run offers to download it from the VICE emulator project (checked by its SHA-1), and so does Settings. A chargen.bin of your own (4096 bytes) in ~/.local/share/charcoal/ works too.

SETTINGS
Kept in ~/.config/charcoal/prefs.json. "uiscale": 2 there doubles the whole window for big screens.`

func (d *HelpReq) draw(u *UI) {
	fb := u.fb
	in := u.window(centre(900, ui.fb.h*5/6), "CharEdit - keys and mouse")
	lines := wrap(helpText, in.w/charW-3)
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

// ---------------------------------------------------------------- the C64 character set

const c64Why = "C64 pages (PETSCII) need the C64's character set. It is the C64's own ROM, so it does not come " +
	"with this program, but the VICE emulator project hands it out. Download it from VICE now? (4 KB, checked " +
	"before it is used, kept in ~/.local/share/charcoal/.) You can also do this later under Settings."

type C64Req struct {
	after func() // what to do once the character set is here
	err   string
}

func openC64Req(after func()) {
	a.dialog = &C64Req{after: after}
}

func (d *C64Req) answered() {
	prefs.C64Asked = true
	savePrefs()
}

func (d *C64Req) download() {
	if err := downloadChargen(); err != nil {
		d.err = "Could not download: " + err.Error()
		return
	}
	a.hasC64 = true
	d.answered()
	closeDialog()
	a.msg("The C64 character set is here: C64 pages work now")
	if d.after != nil {
		d.after()
	}
}

func (d *C64Req) draw(u *UI) {
	in := u.window(centre(560, 240), "C64 character set")
	y := in.y
	for _, l := range wrap(c64Why, in.w/charW) {
		u.fb.text(in.x, y, l, cText)
		y += lineH
	}
	if d.err != "" {
		for _, l := range wrap(d.err, in.w/charW) {
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

func (d *C64Req) key(k string, ctrl, shift bool) bool {
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

// ---------------------------------------------------------------- settings

type SettingsReq struct{ err string }

func openSettings() { a.dialog = &SettingsReq{} }

func (d *SettingsReq) draw(u *UI) {
	fb := u.fb
	in := u.window(centre(600, 260), "Settings")
	y := in.y
	fb.text(in.x, y+3, "Window size", cText)
	sizes := []string{"1x (normal)", "2x (big screens)", "3x"}
	u.choice("uiscale", Rect{in.x + 120, y, choiceW(sizes), gadH}, sizes, scale-1, "", func(i int) {
		scale = i + 1
		savePrefs()
	})
	y += gadH + 16
	fb.text(in.x, y, "C64 character set", cHi)
	y += lineH + 4
	status := "Not here yet: C64 pages need it."
	if a.hasC64 {
		status = "Here. C64 pages work."
		for _, p := range c64Paths() {
			if st, err := os.Stat(p); err == nil && st.Size() == 4096 {
				status = "Here: " + p
				break
			}
		}
	}
	for _, l := range wrap(status, in.w/charW) {
		fb.text(in.x, y, l, cText)
		y += lineH
	}
	label := "Download from VICE"
	if a.hasC64 {
		label = "Download again"
	}
	if u.button(Rect{in.x, y + 6, buttonW(label), gadH}, label, false, "") != nil {
		if err := downloadChargen(); err != nil {
			d.err = "Could not download: " + err.Error()
		} else {
			a.hasC64, d.err = true, ""
			prefs.C64Asked = true
			savePrefs()
			a.msg("The C64 character set is here: C64 pages work now")
		}
	}
	if d.err != "" {
		for i, l := range wrap(d.err, in.w/charW) {
			fb.text(in.x, y+gadH+14+i*lineH, l, cOrange)
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
