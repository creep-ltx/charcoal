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
	lines := wrap(d.text, in.w/charW)
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
	selPath string // the path field as the selection set it: Return then acts on the selection
	after   func() // save: what to do once the page is saved
}

var openAs = []string{"auto", "amiga", "pc", "atari", "st", "zx", "tt", "c64"}
var openAsNames = []string{"detect", "Amiga", "PC", "Atari ATASCII", "Atari ST VT52", "ZX Spectrum", "Teletext", "C64 PETSCII"}

func openOpenRequester() {
	d := &FileReq{sel: -1}
	dir := a.lastDir
	if dir == "" {
		dir, _ = os.Getwd()
	}
	d.listFirst(dir)
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
	if dir == "" {
		dir, _ = os.Getwd()
	}
	d.listFirst(dir)
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

// listFirst lists the first of dir, the home folder and / that can be listed
// (a remembered folder may be gone, a NAS unmounted); why dir could not be is shown.
func (d *FileReq) listFirst(dir string) {
	home, _ := os.UserHomeDir()
	why := ""
	for _, p := range []string{dir, home, "/"} {
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
		d.entries = append(d.entries, entry{e.Name(), isDir, info.Size()})
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
	if i < 0 || i >= len(d.entries) {
		return
	}
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
			d.selPath = d.path.String()
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
			names = names[:7]
		}
		u.choice("as", Rect{in.x + 72, y, choiceW(names), gadH}, names, d.choice, "", func(i int) { d.choice = i })
		if len(prefs.Recent) > 0 {
			r := Rect{in.x + 72 + choiceW(names) + 16, y, buttonW("Recent files"), gadH}
			if u.button(r, "Recent files", false, "The last files opened or saved ("+keyName("open_recent")+" opens the newest again)") != nil {
				d.recentPopup(u, r)
			}
		}
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
		d.enter()
	case 3:
		closeDialog()
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
		} else if !d.save {
			d.path.set(d.dir + string(filepath.Separator))
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

// helpText is the help; its list of keys is made from the key table.
func helpText() string {
	return `MOUSE
Draw: left paints the brush (character + colours), right erases, Shift+click draws a line from the last point.
Colour: left sets the foreground, right the background (on a C64 page: the foreground only).
Fill: left fills equal cells with the brush.
Half (PC): left paints half-cell pixels in the foreground, right in the background colour. Sixel (teletext): left sets, right clears a graphics pixel.
Select: drag a block; drag inside a selected block to move it (Esc while moving puts it back). While pasting, the block follows the mouse: left click stamps it, Shift+click stamps it without its blank cells, right click or Esc stops.
Middle click or Pick: take a cell's character and colours.
Characters strip: click = brush (types it with Type). Right click = put on the active F-key.
Colours: left = foreground, right = background (PETSCII and Atari: the screen colour; middle click: the border), Shift+click = change the colour.
Pop-up gadgets: click opens the list, right click steps to the next item.
Wheel scrolls the page, Shift+wheel sideways, Ctrl+wheel zooms around the mouse.
Tabs: click to show, x or middle click closes, + opens a new page. With two pages on show, click a page to work on it.

KEYS
The page has a height (H) as well as a width: it ends where the dark area starts, and every row is saved, empty rows at the bottom too. Enter or Down past the last row adds a row; H sets the height (fewer rows cuts, Undo brings them back).
Typing writes at the cursor in the brush colours (any tool). Arrows, Home, End, PgUp, PgDn, Enter, Tab, Backspace, Delete (clears a selected block), Insert (insert / overwrite), Esc (lets go of a block). F1-F10 type the F-key set.
Every key below can be changed in ~/.config/charcoal/keys (the file says how; delete it to get these back).
` + keysHelp() + "\n" + `A selected block, or one being pasted, can be turned over with the buttons under the colours or the keys above: mirror (left-right), upside down, turn a quarter clockwise, invert (reversed characters on PETSCII and Atari pages, the two colours swapped elsewhere). Characters become their mirrored or turned twins: / and \, the box lines and corners, half blocks, arrows; on PETSCII pages Marq's PETSCII editor's tables. A selection that turns into another shape is lifted off the page to be put down. Copy a block, paste it, turn it, stamp it as often as you like.
"Sync" scrolls two pages together. Dropping files on the window opens them. Keys in the W and H fields stay in the field until Enter, Esc or Tab.
PAGE TYPES: AMIGA, PC, ATARI, ATARI ST, PETSCII, ZX SPECTRUM, TELETEXT
Amiga pages are Latin-1, read 0x9B as CSI, clear on form feed, keep italic and underline, save with LF line ends. PC pages are CP437 in the VGA font and save with CRLF. Atari pages are ATASCII, the Atari 8-bit computers' text screen: inverse characters (Rev), one background, one text colour for the whole screen (left click a colour) and a border (middle click); .ata files, with EOL line ends and ESC before control characters. Width 38 (the screen editor's margins) or 40 (terminals) is guessed from the file and can be switched; the page is never wider. The font is AltirraOS's, free to pass on. PETSCII pages are exactly as wide as the machine's screen (the Machine gadget sets it): PET 40 or 80 columns (one colour: green, amber or white screen), VIC-20 (22 x 23, 8 text colours), C64 (40 x 25), C128 (40 columns, or 80 on the VDC with its RGBI colours). Each has its two character sets (the Font gadget: upper case + graphics, or lower + upper case), reverse characters, one screen colour; they read and write .seq files, the stream a C64 BBS sends. Switching keeps the bytes and only changes how they are shown; an unchanged file is read again with the other page's rules.
Atari ST pages are the ST's VT52 terminal in one of its resolutions (the Resolution gadget): low 40 x 25 in 16 colours, medium 80 x 25 in 4, high 80 x 25 in black and white with taller characters. Each cell has its own colour and background (ESC b, ESC c); files are the VT52 stream (.vt). The font is EmuTOS's, downloaded on first use.

ZX Spectrum pages are its 32 x 24 screen: a character per cell with ink (left click), paper (right click), bright (colours 8-15; ink and paper share it) and Flash; middle click sets the border. Codes 128-143 are the Spectrum's block graphics, 144-164 its user-defined graphics. Saved as .scr, the 6912-byte screen every Spectrum emulator loads; a .scr that is a drawing opens with each cell as the nearest character.

Teletext pages are 40 x 25 cells of 7-bit codes as on a BBC Micro in Mode 7 or a TV teletext page. Codes 0-31 are attributes that act from their cell to the right, as the teletext chip reads a row: text or graphics colour (F-key sets 1 and 2), double height, flash, conceal, separated, hold, new background. Codes shows their names in their cells. The Sixel tool paints the 2x3 graphics pixels of a cell (the row needs a graphics colour to its left). Files: .tti, .ep1, edit.tf links (.url; a text holding an edit.tf link opens too) and BBC Mode 7 dumps (1000-byte .bin). The characters are Bedstead's (public domain).

SAVE AS
ANSI, plain text, UTF-8 text, PC .bin, PCBoard .pcb, Avatar .avt, ATASCII .ata, Atari ST .vt, ZX Spectrum .scr, teletext .tti, .ep1 and edit.tf links (.url), XBin (.xb, with the font and palette inside, so it looks the same in any viewer), PETSCII .seq, a C64 program (.prg: LOAD and RUN it on a C64 or in VICE, a key goes back to BASIC), the C array of Marq's PETSCII editor and Petmate (.c), a Petmate workspace (.petmate), pictures (.png, normal or twice the size; C64 pages with their border) and a web page (.html). Typing a file ending picks the format. Saving under the name of another file that is already there asks first.
OPENING
Everything above that holds characters opens again: ANSI, text, .bin, XBin, .pcb, .avt, .ata (also as .txt, recognised by its bytes), .vt, .scr, .tti, .ep1, edit.tf links, .seq, .c, .petmate, and C64 programs saved by CharEdit; also iCEDraw .idf, ArtWorx .adf and TundraDraw .tnd. A .c or .petmate with several screens opens each screen in a tab of its own. Pictures and web pages do not open.
The Commodore character sets are the machines' ROMs and do not come with the program: the first run offers to download them from the VICE emulator project (each checked by its SHA-1), and so does Settings.

SETTINGS
Kept in ~/.config/charcoal/prefs.json (a broken one is put aside as prefs.json.bad); the keys in ~/.config/charcoal/keys. Settings: the window size (1x to 4x for big screens), keeping the old version of a file as name.bak when a save writes over it (on unless switched off), and the downloads, which run while you work on.

SAFETY
Every two minutes, pages with changes are kept aside (in ~/.local/share/charcoal/autosave/). If CharEdit stops without saving them (a crash, the power), the next start offers to open them again in tabs. Saving a page or closing it on purpose lets go of its copy. The Open requester's "Recent files" lists the last 10 files opened or saved.`
}

func (d *HelpReq) draw(u *UI) {
	fb := u.fb
	in := u.window(centre(900, ui.fb.h*5/6), "CharEdit - keys and mouse")
	lines := wrap(helpText(), in.w/charW-3)
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

const c64Why = "PETSCII pages need the Commodore character sets (PET, VIC-20, C64, C128). They are the machines' " +
	"own ROMs, so they do not come with this program, but the VICE emulator project hands them out. Download them " +
	"from VICE now? (18 KB, each checked before it is used, kept in ~/.local/share/charcoal/.) You can also do this " +
	"later under Settings."

const stWhy = "Atari ST pages need the ST's font. Atari's own is in the TOS ROM; EmuTOS, the free TOS (GPL), has " +
	"its own version of it. Download it from EmuTOS now? (two small files, checked before they are used, kept in " +
	"~/.local/share/charcoal/.) You can also do this later under Settings."

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
	a.dialog = &FontReq{title: "Commodore character sets", why: c64Why, firstRun: true, after: after,
		what: "the Commodore character sets", fetch: fetchC64ROMs, load: loadC64Downloaded}
}

func openSTReq(after func()) {
	a.dialog = &FontReq{title: "Atari ST font", why: stWhy, after: after,
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
	for _, l := range wrap(d.why, in.w/charW) {
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

type SettingsReq struct{ err string }

func openSettings() { a.dialog = &SettingsReq{} }

func (d *SettingsReq) draw(u *UI) {
	fb := u.fb
	in := u.window(centre(660, 410), "Settings")
	y := in.y
	fb.text(in.x, y+3, "Window size", cText)
	sizes := []string{"1x (normal)", "2x (big screens)", "3x", "4x"} // as many as main allows (maxUIScale)
	u.choice("uiscale", Rect{in.x + 120, y, choiceW(sizes), gadH}, sizes, scale-1, "", func(i int) {
		scale = i + 1
		savePrefs()
	})
	y += gadH + 8
	if v, _ := u.check(in.x, y, "Keep the old version as name.bak when saving over a file", !prefs.NoBackup, ""); v == prefs.NoBackup {
		prefs.NoBackup = !v
		savePrefs()
	}
	y += gadH + 16
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
	for _, l := range wrap(status, in.w/charW) {
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
		for i, l := range wrap(d.err, in.w/charW) {
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
