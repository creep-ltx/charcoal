package main

// IGS files on an Atari ST page (docs/igs-design.md): the drawing is a picture under the cells, the
// VT52 text after it is the cells, and saving keeps every byte of the drawing.

import (
	"fmt"
	"strings"
)

func init() {
	formats = append(formats, Format{"igs", "Atari ST IGS (.ig)", ".ig", "st"})
	editorOwn["igs"] = true
	openAs = append(openAs, "igs")
	openAsNames = append(openAsNames, "Atari ST IGS")
}

// An IGS file read: its picture, the cells of its text part, and the bytes kept to write it back.
type igsLayer struct {
	src     []byte
	res     int // the resolution the picture is in (igsLow ...)
	w, h    int
	pix     []uint8    // colour registers
	pal     [16]uint32 // the registers as shown
	head    []byte     // the drawing: written back as it is
	kept    [][]byte   // the text part's other commands (palette, sound, pauses ...), each "G#...:"
	closing []byte     // what comes after the picture is over (a clear, a reset ...), as it is
	cells   *Doc       // the text part's cells as read
	note    string     // what is not shown or not editable
}

// igsPage: is the page an IGS file's (a picture under its cells)?
func (a *App) igsPage() bool {
	return a.st() && a.igs != nil && a.igs.res == igsResOf(a.stres)
}

// igsGuessRes is the resolution a file draws for: its own R command before any drawing, else the
// coordinates it draws at, loops' steps included (x past 319: medium, y past 199: high), else low.
func igsGuessRes(items []igsItem) int {
	vm := newIgsVM(igsHigh)
	vm.probe = &[2]int{}
	for _, it := range items {
		c := it.cmd
		if c == nil {
			continue
		}
		if c.letter == 'R' && vm.probe[0] == 0 && vm.probe[1] == 0 {
			if r := c.num(0); r >= 0 && r <= 2 {
				return r
			}
		}
		if igsDraws(c) {
			vm.run(c)
		}
		if vm.busy() {
			break
		}
	}
	switch maxX, maxY := vm.probe[0], vm.probe[1]; {
	case maxY > 199 && maxX < 1000:
		return igsHigh
	case maxX > 319:
		return igsMedium
	}
	return igsLow
}

// readIGS reads an IGS file in resolution res (-1: the file's own or guessed).
func readIGS(b []byte, res int) *igsLayer {
	items := igsParse(b)
	if res < 0 {
		res = igsGuessRes(items)
	}
	wipes := func(it igsItem) bool { return it.wipe || (it.cmd != nil && igsWipes(it.cmd)) }
	lastVis := -1 // the last item that shows something
	for i, it := range items {
		if (it.cmd != nil && igsDraws(it.cmd)) || (it.cmd == nil && it.visible) {
			lastVis = i
		}
	}
	headEnd := -1 // the last item that changes pixels (or wipes them) before that
	for i := 0; i <= lastVis; i++ {
		if it := items[i]; (it.cmd != nil && igsDraws(it.cmd)) || wipes(it) {
			headEnd = i
		}
	}
	closeAt := len(items) // the first wipe after the last thing shown
	for i := lastVis + 1; i < len(items); i++ {
		if wipes(items[i]) {
			closeAt = i
			break
		}
	}
	l := &igsLayer{src: b}
	vm := newIgsVM(res)
	headBytes := 0
	for i := 0; i <= headEnd; i++ {
		it := items[i]
		if it.cmd != nil {
			vm.run(it.cmd)
		} else {
			vm.vt52(b[it.start:it.end])
		}
		headBytes = it.end
	}
	l.head = b[:headBytes]
	// A command cut short by the file's end is closed, so what is written after it is not read as its own:
	// a bitmap (X 11) gets the zeros it was read with, a text (W) its '@'.
	if c := igsLastCmd(items, headEnd); c != nil && c.end == len(b) && !c.chained {
		switch {
		case c.letter == 'X' && c.num(0) == 11 && (c.num(1) == 1 || c.num(1) == 2) && len(c.vals) == 3:
			want := map[bool]int{true: 4000, false: 32000}[c.num(2) >= 1 && c.num(2) <= 8]
			if c.text == nil { // the file ended right after the values: a separator, then the bytes
				want++
			}
			if len(c.text) < want {
				l.head = append(append([]byte(nil), l.head...), make([]byte, want-len(c.text))...)
			}
		case c.letter == 'W' && len(c.vals) == 2:
			l.head = append(append([]byte(nil), l.head...), '@')
		}
	}
	l.res, l.w, l.h = vm.res, vm.w, vm.h
	r := stResByID(igsResIDs[vm.res])

	// The text part: VT52 into cells, from where the drawing left the cursor and colours.
	d := newDoc(r.cols, r.rows)
	fillCells(d.cells, makeCell(32, r.fg, 0, 0))
	t := &STTerm{doc: d, x: vm.tx, y: vm.ty, fg: vm.tfg, bg: vm.tbg, rev: vm.trev, wrap: vm.twrap,
		sx: vm.tsx, sy: vm.tsy, rows: r.rows}
	pals := [][16]uint32{vm.pal}
	sounds := 0
	for i := headEnd + 1; i < closeAt; i++ {
		it := items[i]
		c := it.cmd
		switch {
		case c == nil:
			t.write(b[it.start:it.end])
		case igsTextCmd(c):
			igsTextOnCells(t, c.letter, vm.args(c))
		default:
			l.kept = append(l.kept, []byte("G#"+string(b[c.letterAt:c.bodyEnd])+":"))
			if igsPaletteCmd(c) {
				vm.run(c)
				pals = append(pals, vm.pal)
			}
			if strings.IndexByte("bnN", c.letter) >= 0 {
				sounds++
			}
		}
	}
	if d.rows > r.rows { // text past the screen's last row: the ST would scroll; the page keeps the screen
		d.rows = r.rows
	}
	d.ensureRows(r.rows)
	l.cells = d
	l.pix = vm.pix
	l.pal = igsBestPalette(pals, vm.pix, d)
	if closeAt < len(items) {
		it := items[closeAt]
		if it.cmd != nil && !it.cmd.own {
			l.closing = append([]byte("G#"), b[it.cmd.letterAt:]...)
		} else {
			l.closing = b[it.start:]
		}
	}
	var notes []string
	if sounds > 0 {
		notes = append(notes, fmt.Sprintf("%d sound%s not played", sounds, map[bool]string{true: "s"}[sounds > 1]))
	}
	if len(l.closing) > 0 {
		notes = append(notes, "its closing clear not shown")
	}
	if vm.busy() {
		notes = append(notes, "too much to draw: not all shown")
	}
	l.note = strings.Join(notes, "; ")
	return l
}

func igsLastCmd(items []igsItem, i int) *igsCmd {
	if i < 0 {
		return nil
	}
	return items[i].cmd
}

// igsBestPalette picks, of the palettes the text part goes through (a fade out, a reveal, colour
// cycling), the one in which the picture shows the most different colours, of those the brightest (a
// fade to black is darker at each step, a reveal brighter); a later one wins a tie (colour cycling).
func igsBestPalette(pals [][16]uint32, pix []uint8, d *Doc) [16]uint32 {
	var used [16]int
	for _, p := range pix {
		used[p&15]++
	}
	for _, v := range d.cells[:d.rows*d.cols] {
		if !isBlank(v) {
			used[cellFg(v)]++
			used[cellBg(v)]++
		}
	}
	best, bestN, bestLight := pals[0], -1, -1
	for _, p := range pals {
		seen, light := map[uint32]bool{}, 0
		for r, n := range used {
			if n > 0 {
				seen[p[r]] = true
				light += n * int(p[r]>>16&255+p[r]>>8&255+p[r]&255)
			}
		}
		if len(seen) > bestN || len(seen) == bestN && light >= bestLight {
			best, bestN, bestLight = p, len(seen), light
		}
	}
	return best
}

// igsTextOnCells runs IG's VT52 commands (c p m l d i r v w) on the page's VT52 terminal.
func igsTextOnCells(t *STTerm, letter byte, a []int) {
	cols, rows := t.doc.cols, t.rows
	switch letter {
	case 'c':
		if igsAt(a, 0) == 0 {
			t.bg = igsAt(a, 1) & 15
		} else {
			t.fg = igsAt(a, 1) & 15
		}
	case 'p':
		t.x, t.y = max(0, min(cols-1, igsAt(a, 0))), max(0, min(rows-1, igsAt(a, 1)))
	case 'm':
		n := max(0, min(igsAt(a, 1), 255))
		switch igsAt(a, 0) {
		case 0:
			t.x, t.y = 0, 0
		case 1:
			t.y = max(0, t.y-n)
		case 2:
			t.y = min(rows-1, t.y+n)
		case 3:
			t.x = min(cols-1, t.x+n)
		case 4:
			t.x = max(0, t.x-n)
		}
	case 'l':
		x0, x1 := 0, cols-1
		switch igsAt(a, 0) {
		case 0:
			t.x = 0
		case 1:
			x1 = t.x
		case 2:
			x0 = t.x
		}
		for x := x0; x <= x1; x++ {
			t.doc.set(x, t.y, t.blankCell())
		}
	case 'd', 'i':
		n, up := max(0, min(igsAt(a, 0), rows)), false
		if letter == 'i' {
			n, up = max(0, min(igsAt(a, 1), rows)), igsAt(a, 0) == 0
		}
		for k := 0; k < n; k++ {
			if up && t.y > 0 {
				t.y--
				continue
			}
			sh := &Term{doc: t.doc, y: t.y, screenH: rows}
			sh.shiftRows(map[bool]int{true: -1, false: 1}[letter == 'd'])
		}
	case 'r':
		if igsAt(a, 0) == 0 {
			t.sx, t.sy = t.x, t.y
		} else {
			t.x, t.y = t.sx, t.sy
		}
	case 'v':
		t.rev = igsAt(a, 0) != 0
	case 'w':
		t.wrap = igsAt(a, 0) != 0
	}
}

// igsSameCells: do two pages of cells look the same (every empty cell is the picture)?
func igsSameCells(a, b *Doc) bool {
	if a.cols != b.cols {
		return false
	}
	for y := 0; y < max(a.rows, b.rows); y++ {
		for x := 0; x < a.cols; x++ {
			u, v := a.get(x, y), b.get(x, y)
			if u != v && !(isBlank(u) && isBlank(v)) {
				return false
			}
		}
	}
	return true
}

// writeIGS writes an IGS page: the file as it was if its text is unchanged; else the drawing, the
// cells as VT52 (each run of cells that are not empty placed by ESC Y, the pens set where they change),
// the kept commands and the closing. Without a picture (a plain ST page) it is the cells' VT52.
func writeIGS(l *igsLayer, d *Doc) []byte {
	if l != nil && igsSameCells(l.cells, d) {
		return l.src
	}
	var out []byte
	if l != nil && len(l.head) > 0 {
		out = append(append(out, l.head...), '\r') // the CR ends the drawing's chain of commands
	}
	fg, bg, started := -1, -1, false
	for y := 0; y < d.rows && y < 25; y++ {
		run := false
		for x := 0; x < d.cols; x++ {
			v := d.get(x, y)
			if isBlank(v) {
				run = false
				continue
			}
			if !started {
				out = append(out, 0x1B, 'q', 0x1B, 'w') // reverse off, wrap off
				started = true
			}
			if !run || (cellCh(v) == '#' && x > 0 && cellCh(d.get(x-1, y)) == 'G') { // "G#" in text would be IGS
				out = append(out, 0x1B, 'Y', byte(32+y), byte(32+x))
				run = true
			}
			if f := cellFg(v); f != fg {
				fg = f
				out = append(out, 0x1B, 'b', byte('0'+f))
			}
			if b := cellBg(v); b != bg {
				bg = b
				out = append(out, 0x1B, 'c', byte('0'+b))
			}
			out = append(out, stSafeByte(byte(cellCh(v))))
		}
	}
	if l == nil {
		return out
	}
	for _, k := range l.kept {
		out = append(append(out, k...), '\r')
	}
	return append(out, l.closing...)
}

// ---------------------------------------------------------------- the App

// loadIGS opens an IGS file into the current tab; false if b is not one.
func (a *App) loadIGS(b []byte, name, path, as string) bool {
	if as != "igs" && (as != "auto" && as != "st" || !isIGS(b, name)) {
		return false
	}
	a.srcKind, a.ownFont, a.ownLower, a.ownPal = "igs", nil, nil, nil
	a.mode = "st"
	if readIGSFont6(); !hasSTFont() || !igsFont6OK { // IGS's small text needs the 6x6 font too
		t := a.Tab
		openSTReq(func() {
			if a.backTo(t) {
				a.reparse()
				a.applyMode()
			}
		})
	}
	a.finishLoad(b, name, path, nil, "igs")
	return true
}

// reparseIGS reads the page again from its IGS file; false if it has none. A resolution chosen on the
// page (setSTRes) is kept, else the file's own is taken.
func (a *App) reparseIGS() bool {
	if a.srcKind != "igs" || a.source == nil {
		a.igs = nil
		return false
	}
	res := -1
	if a.forceCols != 0 {
		res = igsResOf(a.stres)
	}
	l := readIGS(a.source, res)
	a.igs = l
	a.mode, a.stres = "st", igsResIDs[l.res]
	a.doc = l.cells.clone()
	a.setPalette()
	a.fg, a.bg = stResByID(a.stres).fg, 0
	a.cx, a.cy, a.sel, a.float = 0, 0, nil, nil
	note := ""
	if l.note != "" {
		note = " (" + l.note + ")"
	}
	if viewer != nil {
		a.msg(a.name + ": IGS drawing" + note)
	} else {
		a.msg(a.name + ": IGS drawing, kept as it is; what you type goes over it" + note)
	}
	return true
}

// igsSetRes draws an IGS page's picture again in another resolution; the cells typed so far stay (cut or
// widened to the new width). False on other pages.
func (a *App) igsSetRes(id string) bool {
	if !a.st() || a.igs == nil || a.srcKind != "igs" || a.source == nil {
		return false
	}
	l := readIGS(a.source, igsResOf(id))
	if l.res != igsResOf(id) {
		a.msg(a.name + " switches to " + stResByID(igsResIDs[l.res]).name + " itself")
	}
	id = igsResIDs[l.res]
	r := stResByID(id)
	a.stres, a.defSTRes = id, id
	if a.dirty {
		a.doc = a.doc.withCols(r.cols)
		a.doc.rows = min(a.doc.rows, r.rows)
		a.doc.ensureRows(r.rows)
	} else {
		a.doc = l.cells.clone()
	}
	a.igs = l
	a.forceCols = r.cols
	a.cx, a.sel = min(a.cx, r.cols-1), nil
	a.setPalette()
	savePrefs()
	return true
}

// encodeIGS writes the page as IGS; ok false for other formats.
func (a *App) encodeIGS(fmtID string, d *Doc) ([]byte, bool) {
	if fmtID != "igs" {
		return nil, false
	}
	if a.st() && a.igs != nil {
		return writeIGS(a.igs, d), true
	}
	return writeIGS(nil, d), true
}

// drawCellXY draws the page's cell (x, y): an empty cell of an IGS page shows the picture under it.
func (a *App) drawCellXY(fb *FB, px, py, z, x, y int, v uint32) {
	if a.drawSkyCell(fb, px, py, z, x, y, v) { // a SkyPix page (skypix.go)
		return
	}
	l := a.igs
	if l == nil || !isBlank(v) || !a.igsPage() || x < 0 || y < 0 || (x+1)*8 > l.w || (y+1)*a.cellH() > l.h {
		a.drawCell(fb, px, py, z, v)
		return
	}
	H, cl := a.cellH(), fb.clip
	inside := px >= cl.x && py >= cl.y && px+8*z <= cl.x+cl.w && py+H*z <= cl.y+cl.h
	for r := 0; r < H; r++ {
		row := l.pix[(y*H+r)*l.w+x*8:]
		for b := 0; b < 8; b++ {
			c := a.pal[row[b]&15]
			if !inside {
				fb.fill(Rect{px + b*z, py + r*z, z, z}, c)
				continue
			}
			for yy := 0; yy < z; yy++ {
				o := (py+r*z+yy)*fb.w + px + b*z
				for xx := 0; xx < z; xx++ {
					fb.px[o+xx] = c
				}
			}
		}
	}
}
