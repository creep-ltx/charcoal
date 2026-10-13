package main

// The editor's state and everything it does to the page.

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

type Palette struct {
	id, name string
	colors   [16]uint32
}

var bright = []uint32{0x555555, 0xff5555, 0x55ff55, 0xffff55, 0x5555ff, 0xff55ff, 0x55ffff, 0xffffff}

func pal(id, name string, c ...uint32) *Palette {
	p := &Palette{id: id, name: name}
	c = append(c, bright...)
	for i := range p.colors {
		p.colors[i] = 0xFF000000 | c[i]
	}
	return p
}

var palettes = []*Palette{
	pal("amiga", "Amiga ANSI", 0x000000, 0xaa0000, 0x00aa00, 0xaaaa00, 0x0000aa, 0xaa00aa, 0x00aaaa, 0xaaaaaa),
	pal("vga", "PC VGA", 0x000000, 0xaa0000, 0x00aa00, 0xaa5500, 0x0000aa, 0xaa00aa, 0x00aaaa, 0xaaaaaa),
	pal("wb", "Workbench 2/3", 0x000000, 0xbb0000, 0x00bb00, 0xeeaa00, 0x6688bb, 0xbb66bb, 0x55aaaa, 0xaaaaaa,
		0x666666, 0xff6666, 0x66ff66, 0xffee55, 0x88aaff, 0xff88ff, 0x88ffff, 0xffffff),
	c64Palette,
	atariPalette,
	{id: "custom", name: "Custom"},
}

func init() { // the other Commodore palettes go after the C64's
	var out []*Palette
	for _, p := range palettes {
		out = append(out, p)
		if p.id == "c64" {
			out = append(out, cbmPalettes...)
		}
	}
	palettes = out
}

func paletteByID(id string) *Palette {
	for _, p := range palettes {
		if p.id == id {
			return p
		}
	}
	return palettes[0]
}

// The F-key sets (bytes). Amiga sets are Latin-1, PC sets CP437.
func defaultFkeys() map[string][][]int {
	return map[string][][]int{
		"amiga": {
			{0x5F, 0xAF, 0x2F, 0x5C, 0x7C, 0x3A, 0x2E, 0xB8, 0xB7, 0x27},
			{0xB4, 0x60, 0xA8, 0xB0, 0xA6, 0x2D, 0x2C, 0x3B, 0x28, 0x29},
			{0xAB, 0xBB, 0x3C, 0x3E, 0x3D, 0x2B, 0x2A, 0x5E, 0x7E, 0x22},
			{0xD7, 0xF7, 0xA4, 0xF8, 0xD8, 0xDF, 0xB5, 0xB6, 0xA7, 0xA9},
			{0x5B, 0x5D, 0x7B, 0x7D, 0x23, 0x40, 0x24, 0x25, 0x26, 0x21},
		},
		"pc": {
			{0xB0, 0xB1, 0xB2, 0xDB, 0xDF, 0xDC, 0xDD, 0xDE, 0xFE, 0xFA},
			{0xDA, 0xBF, 0xC0, 0xD9, 0xC4, 0xB3, 0xC3, 0xB4, 0xC2, 0xC1},
			{0xC9, 0xBB, 0xC8, 0xBC, 0xCD, 0xBA, 0xCC, 0xB9, 0xCB, 0xCA},
			{0xC5, 0xCE, 0xD5, 0xB8, 0xD4, 0xBE, 0xD6, 0xB7, 0xD3, 0xBD},
			{0xF9, 0xF8, 0xFD, 0xF7, 0xFB, 0xEF, 0xA9, 0xAA, 0x10, 0x11},
		},
		"c64":   c64Fkeys,
		"atari": atariFkeys,
	}
}

var bauds = []int{2400, 9600, 14400, 28800, 57600, 115200}

type Info struct {
	title, author, group string
	writeSauce, clear    bool
}

type Sel struct{ x0, y0, x1, y1 int }
type Hover struct {
	x, y, half int
}
type Snap struct {
	doc    *Doc
	cx, cy int
}
type Play struct {
	bytes []byte
	pos   int
	term  emulator
	saved Snap
	last  time.Time
}

// Tab is one open page with everything that belongs to it.
type Tab struct {
	doc                 *Doc
	mode                string // "amiga", "pc" or "c64"
	fontFor, paletteFor map[string]string
	zoom                int
	ch8, smear, ice     bool
	cx, cy              int
	path, name          string
	dirty               bool
	source              []byte // the file as read (or last saved), to read again with other rules
	sourceBin           bool
	sauce               *Sauce
	saveFmt             string
	forceCols           int
	info                Info
	sel                 *Sel
	selecting           bool
	float               *Block
	undo, redo          []Snap
	hover, last         *Hover
	playing             *Play
	scrollX, scrollY    int
	page                Rect // where the page view was laid out
	pal                 [16]uint32
	srcKind             string      // "xbin", "marq", "petmate" or "prg" for files that are not a stream of codes
	srcIndex            int         // which screen of a file that holds several
	ownFont             *Font       // a font that came in the file (XBin)
	ownPal              *[16]uint32 // a palette that came in the file (XBin)
	machine             string      // the Commodore machine of a PETSCII page
}

// App is the editor: the tab being worked on (its fields read as a's own),
// the other tabs, and what all tabs share (tool, brush, colours, F-keys).
type App struct {
	*Tab
	tabs          []*Tab
	layout        string // "one", "cols" (side by side) or "rows" (top and bottom)
	shown         [2]*Tab
	syncScroll    bool
	grid          bool
	tool          string
	fg, bg, brush int
	ital, under   bool
	rev, insert   bool
	clip          *Block
	clipText      string
	fset, factive int
	fkeys         map[string][][]int
	button        int
	blink         bool
	lastDir       string
	baud          int
	message       string
	messageAt     time.Time
	custom        [16]uint32
	dialog        Dialog
	charHover     int
	quit          bool
	bare          bool // gadgets hidden: only the page and the status line
	hasC64        bool
	defFont       map[string]string // the fonts and palettes a new tab starts with
	defPalette    map[string]string
	defZoom       int
	defMachine    string
	ch8Default    bool
	smearDefault  bool
}

var a *App

func (a *App) amiga() bool { return a.mode == "amiga" }
func (a *App) pc() bool    { return a.mode == "pc" }
func (a *App) c64() bool   { return a.mode == "c64" }
func (a *App) atari() bool { return a.mode == "atari" }

// screen: a machine's text screen (C64, Atari): 8x8 characters, one background
// colour for the whole screen, a border, inverse characters in the top half.
func (a *App) screen() bool { return a.c64() || a.atari() }

func (a *App) font() *Font {
	if a.ownFont != nil && a.fontFor[a.mode] == "xbin" {
		return a.ownFont
	}
	if a.c64() {
		fam := a.mach().family
		if f := fontByID(a.fontFor["c64"]); f != nil && strings.HasPrefix(f.id, fam) {
			return f
		}
		if f := fontByID(fam + "upper"); f != nil {
			return f
		}
	}
	if f := fontByID(a.fontFor[a.mode]); f != nil && f.kind == a.mode {
		return f
	}
	for _, f := range fonts {
		if f.kind == a.mode && f.id != "topaz1" {
			return f
		}
	}
	return fonts[1]
}

// lower: a C64 page in the lower + upper case set.
func (a *App) lower() bool { return strings.HasSuffix(a.font().id, "lower") }

// mach is the Commodore machine of a PETSCII page.
func (a *App) mach() Machine { return machineByID(a.machine) }

// setCase picks the upper or lower case set of the page's machine.
func (a *App) setCase(lower bool) {
	a.fontFor["c64"] = a.mach().family + map[bool]string{true: "lower", false: "upper"}[lower]
}

func (a *App) cellH() int {
	if a.screen() || (a.ch8 && a.amiga()) || (a.font() == a.ownFont && a.ownFont != nil && a.ownFont.h8) {
		return 8
	}
	return 16
}

func (a *App) styleFlags() uint32 {
	if !a.amiga() {
		return 0
	}
	var f uint32
	if a.ital {
		f |= italic
	}
	if a.under {
		f |= underline
	}
	return f
}

// cell makes a cell of the page in the brush's colours.
func (a *App) cell(ch int) uint32 {
	if a.screen() {
		if a.rev {
			ch |= 0x80
		}
		if a.atari() {
			return makeCell(ch, 0, 0, 0) // one text colour for the whole screen
		}
		return makeCell(ch, a.fg, 0, 0)
	}
	return makeCell(ch, a.fg, a.bg, a.styleFlags())
}

func (a *App) brushCell() uint32 {
	if a.atari() {
		return makeCell(a.brush, 0, 0, 0)
	}
	if a.c64() {
		return makeCell(a.brush, a.fg, 0, 0)
	}
	return makeCell(a.brush, a.fg, a.bg, a.styleFlags())
}

// previewFg is a colour that shows on the page's background, for the character table.
func (a *App) previewFg() int {
	if a.atari() {
		return a.doc.ink
	}
	if !a.c64() {
		return 7
	}
	if a.doc.bg == 15 || a.doc.bg == 1 || a.doc.bg == 7 || a.doc.bg == 13 {
		return 0
	}
	return 15
}

func (a *App) setPalette() {
	if a.ownPal != nil && a.paletteFor[a.mode] == "file" {
		a.pal = *a.ownPal
		return
	}
	p := paletteByID(a.paletteFor[a.mode])
	if a.c64() && !strings.Contains(" c64 colodore vic20 vdc petgreen petamber petwhite custom ", " "+p.id+" ") {
		p = paletteByID(a.mach().palette)
	}
	if a.atari() && p.id != "custom" && p.id != "atari" {
		p = atariPalette
	}
	if p.id == "custom" {
		a.pal = a.custom
	} else {
		a.pal = p.colors
	}
}

// printable: is this byte a glyph on the current page?
func (a *App) printable(c int) bool {
	if a.screen() {
		return true
	}
	if a.amiga() {
		return !(c < 0x20 || (c >= 0x7F && c < 0xA0))
	}
	return c != 0
}

func (a *App) charToByte(r rune) int {
	if a.c64() {
		return petFromRune(r, a.lower())
	}
	if a.atari() {
		return atariFromRune(r)
	}
	if a.amiga() {
		if r <= 0xFF && a.printable(int(r)) {
			return int(r)
		}
		return -1
	}
	if r >= 0x20 && r < 0x7F {
		return int(r)
	}
	if b, ok := cp437Rev[r]; ok {
		return int(b)
	}
	return -1
}

// runeOf is the character a byte of the page shows, as Unicode.
func (a *App) runeOf(c int) rune {
	if a.c64() {
		return petRune(c, a.lower())
	}
	if a.atari() {
		return atariRune(c)
	}
	return byteRune(c, a.amiga())
}

func (a *App) msg(s string) {
	a.message = s
	a.messageAt = time.Now()
}

// ---------------------------------------------------------------- undo

func (a *App) snapshot() Snap { return Snap{a.doc.clone(), a.cx, a.cy} }

// undoBudget caps the cells all undo steps of a page may keep (64 MB): tall art
// keeps fewer steps instead of eating the memory.
const undoBudget = 16 << 20

func (a *App) pushUndo() {
	a.undo = append(a.undo, a.snapshot())
	total := 0
	for _, s := range a.undo {
		total += len(s.doc.cells)
	}
	for len(a.undo) > 1 && (len(a.undo) > 300 || total > undoBudget) {
		total -= len(a.undo[0].doc.cells)
		a.undo = a.undo[1:]
	}
	a.redo = a.redo[:0]
	a.dirty = true
}
func (a *App) restore(s Snap) { a.doc, a.cx, a.cy = s.doc, s.cx, s.cy }
func (a *App) doUndo() {
	if len(a.undo) == 0 {
		a.msg("Nothing to undo")
		return
	}
	a.redo = append(a.redo, a.snapshot())
	a.restore(a.undo[len(a.undo)-1])
	a.undo = a.undo[:len(a.undo)-1]
	a.dirty = true
}
func (a *App) doRedo() {
	if len(a.redo) == 0 {
		a.msg("Nothing to redo")
		return
	}
	a.undo = append(a.undo, a.snapshot())
	a.restore(a.redo[len(a.redo)-1])
	a.redo = a.redo[:len(a.redo)-1]
	a.dirty = true
}

// ---------------------------------------------------------------- editing

func (a *App) setCell(x, y int, v uint32) {
	if x < 0 || x >= a.doc.cols || y < 0 {
		return
	}
	a.doc.set(x, y, v)
}

func (a *App) typeByte(b int) {
	a.pushUndo()
	d := a.doc
	if a.cx >= d.cols {
		a.cx, a.cy = 0, a.cy+1
	}
	if a.insert {
		for x := d.cols - 1; x > a.cx; x-- {
			d.set(x, a.cy, d.get(x-1, a.cy))
		}
	}
	a.setCell(a.cx, a.cy, a.cell(b))
	a.cx++
	if a.cx >= d.cols {
		a.cx, a.cy = 0, a.cy+1
	}
	a.moved()
}

func (a *App) moved() {
	a.cx = max(0, min(a.doc.cols-1, a.cx))
	a.cy = max(0, a.cy)
	if a.cy >= a.doc.rows-1 {
		a.doc.ensureRows(a.cy + 2)
	}
	a.blink = true
	a.ensureVisible(a.cx, a.cy)
}

func (a *App) ensureVisible(x, y int) {
	H, W := a.cellH()*a.zoom, 8*a.zoom
	top, left := y*H, x*W
	if top < a.scrollY {
		a.scrollY = max(0, top-H)
	} else if top+H > a.scrollY+a.page.h {
		a.scrollY = top + 2*H - a.page.h
	}
	if left < a.scrollX {
		a.scrollX = max(0, left-W)
	} else if left+W > a.scrollX+a.page.w {
		a.scrollX = left + 2*W - a.page.w
	}
}

// deleteAt removes the cell and pulls the rest of the row left.
func (a *App) deleteAt(x, y int) {
	d := a.doc
	for i := x; i < d.cols-1; i++ {
		d.set(i, y, d.get(i+1, y))
	}
	d.set(d.cols-1, y, blank)
}

func line(x0, y0, x1, y1 int, fn func(x, y int)) {
	dx, dy := abs(x1-x0), -abs(y1-y0)
	sx, sy := 1, 1
	if x0 > x1 {
		sx = -1
	}
	if y0 > y1 {
		sy = -1
	}
	err := dx + dy
	for {
		fn(x0, y0)
		if x0 == x1 && y0 == y1 {
			return
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func (a *App) paintAt(x, y, button int) {
	old := a.doc.get(x, y)
	switch a.tool {
	case "draw":
		if button == 3 {
			a.setCell(x, y, blank)
		} else {
			a.setCell(x, y, a.brushCell())
		}
	case "colour":
		if (button == 3 && a.c64()) || a.atari() {
			return // a C64 cell has no background of its own, an Atari cell no colour at all
		}
		if button == 3 {
			a.setCell(x, y, old&^0xF000|uint32(a.bg)<<12)
		} else {
			a.setCell(x, y, old&^0xF00|uint32(a.fg)<<8)
		}
	}
}

// paintHalf: a cell holds two pixels, top and bottom.
func (a *App) paintHalf(x, hy, colour int) {
	y, bottom := hy>>1, hy&1
	v := a.doc.get(x, y)
	ch, fg, bg := cellCh(v), cellFg(v), cellBg(v)
	var top, bot int
	switch ch {
	case 0xDB:
		top, bot = fg, fg
	case 0xDF:
		top, bot = fg, bg
	case 0xDC:
		top, bot = bg, fg
	default:
		top, bot = bg, bg
	}
	if bottom == 1 {
		bot = colour
	} else {
		top = colour
	}
	lim := 7
	if a.ice {
		lim = 15
	}
	var cell uint32
	switch {
	case top == bot && top <= lim:
		cell = makeCell(32, 7, top, 0)
	case top == bot:
		cell = makeCell(0xDB, top, 0, 0)
	case bot <= lim:
		cell = makeCell(0xDF, top, bot, 0)
	case top <= lim:
		cell = makeCell(0xDC, bot, top, 0)
	default:
		cell = makeCell(0xDF, top, bot&7, 0)
	}
	a.setCell(x, y, cell)
}

func (a *App) floodFill(x, y int) {
	d := a.doc
	target, want := d.get(x, y), a.brushCell()
	if target == want || y >= d.rows {
		return
	}
	stack := [][2]int{{x, y}}
	for len(stack) > 0 {
		p := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		px, py := p[0], p[1]
		if px < 0 || py < 0 || px >= d.cols || py >= d.rows || d.get(px, py) != target {
			continue
		}
		d.set(px, py, want)
		stack = append(stack, [2]int{px + 1, py}, [2]int{px - 1, py}, [2]int{px, py + 1}, [2]int{px, py - 1})
	}
}

func (a *App) pick(x, y int) {
	v := a.doc.get(x, y)
	a.brush = cellCh(v)
	if !a.atari() {
		a.fg = cellFg(v)
	}
	if !a.screen() {
		a.bg = cellBg(v)
	}
	a.ital, a.under = v&italic != 0, v&underline != 0
}

func (a *App) setTool(t string) {
	if t == "half" && !a.pc() {
		t = "draw"
	}
	a.tool = t
	if t != "select" && t != "type" && !a.selecting {
		a.sel = nil
	}
}

// setFg sets the brush's colour; on an Atari page, the text colour of the whole screen.
func (a *App) setFg(i int) {
	if a.atari() {
		if a.doc.ink != i {
			a.pushUndo()
			a.doc.ink = i
		}
		return
	}
	a.fg = i
}

// fgNow is the colour typing gives: the brush's, or an Atari screen's text colour.
func (a *App) fgNow() int {
	if a.atari() {
		return a.doc.ink
	}
	return a.fg
}

func (a *App) setBg(i int) {
	if a.screen() { // one background for the whole screen
		if a.doc.bg != i {
			a.pushUndo()
			a.doc.bg = i
		}
		return
	}
	a.bg = i
	if i > 7 && !a.ice {
		a.msg("Backgrounds 8-15 need iCE colours (otherwise they blink on PC)")
	}
}

func (a *App) setFset(n int) {
	sets := a.fkeys[a.mode]
	a.fset = (n%len(sets) + len(sets)) % len(sets)
}

// ---------------------------------------------------------------- selection and clipboard

func (a *App) selRect() Rect {
	s := a.sel
	x, y := min(s.x0, s.x1), min(s.y0, s.y1)
	return Rect{x, y, abs(s.x1-s.x0) + 1, abs(s.y1-s.y0) + 1}
}

func (a *App) copySel(cut bool) {
	if a.sel == nil {
		a.msg("Select a block first (Select tool)")
		return
	}
	r := a.selRect()
	a.clip = a.doc.copyRect(r.x, r.y, r.w, r.h)
	a.clipText = strings.Join(docText(a.doc, a.runeOf, r.x, r.y, r.w, r.h), "\n")
	setClipboard(a.clipText)
	if cut {
		a.clearSel()
	}
	verb := "Copied"
	if cut {
		verb = "Cut"
	}
	a.msg(fmt.Sprintf("%s %dx%d", verb, r.w, r.h))
}

func (a *App) clearSel() {
	r := a.selRect()
	a.pushUndo()
	for y := r.y; y < r.y+r.h; y++ {
		for x := r.x; x < r.x+r.w; x++ {
			a.doc.set(x, y, blank)
		}
	}
}

func (a *App) paste() {
	text := getClipboard()
	if a.clip != nil && (text == a.clipText || text == "") {
		a.startPaste(a.clip)
	} else if text != "" {
		a.startPaste(a.textBlock(text))
	} else {
		a.msg("The clipboard is empty")
	}
}

func (a *App) startPaste(b *Block) {
	a.float = b
	a.sel = nil
	if a.hover == nil {
		a.hover = &Hover{x: a.cx, y: a.cy}
	}
	a.msg("Click to stamp the block (Shift: without blanks), Esc to stop")
}

func (a *App) stamp(x0, y0 int, transparent bool) {
	f := a.float
	a.pushUndo()
	for y := 0; y < f.h; y++ {
		for x := 0; x < f.w; x++ {
			v := f.cells[y*f.w+x]
			if transparent && isBlank(v) {
				continue
			}
			if x0+x < a.doc.cols {
				a.doc.set(x0+x, y0+y, v)
			}
		}
	}
}

func (a *App) textBlock(text string) *Block {
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	text = strings.TrimSuffix(text, "\n")
	lines := strings.Split(text, "\n")
	w := 1
	for _, l := range lines {
		w = max(w, utf8.RuneCountInString(l))
	}
	w = min(w, a.doc.cols)
	b := &Block{w: w, h: len(lines), cells: make([]uint32, w*len(lines))}
	fillCells(b.cells, blank)
	for y, l := range lines {
		x := 0
		for _, r := range l {
			if x >= w {
				break
			}
			c := 32
			if r != '\t' {
				c = a.charToByte(r)
			}
			if c < 0 {
				c = '?'
			}
			b.cells[y*w+x] = a.cell(c)
			x++
		}
	}
	return b
}

// ---------------------------------------------------------------- loading

func (a *App) reparse() {
	switch a.srcKind {
	case "xbin":
		if d, _, err := parseXBin(a.source); err == nil {
			a.doc = d
		}
		a.doc.ensureRows(max(a.doc.rows, 25))
		a.cx, a.cy, a.sel, a.float = 0, 0, nil, nil
		return
	case "marq", "petmate", "prg":
		if scr, err := a.screens(a.source, a.srcKind); err == nil && a.srcIndex < len(scr) {
			a.doc = scr[a.srcIndex].doc
			if l := scr[a.srcIndex].lower; l >= 0 {
				a.setCase(l == 1)
			}
		}
		a.doc.ensureRows(max(a.doc.rows, 25))
		a.cx, a.cy, a.sel, a.float = 0, 0, nil, nil
		return
	}
	if a.atari() {
		a.doc = parseAta(a.source, a.forceCols)
	} else if a.c64() {
		cols := a.forceCols
		if cols == 0 {
			cols = a.mach().cols
		}
		d, lower := parseSeq(a.source, cols)
		a.doc = d
		if lower >= 0 {
			a.setCase(lower == 1)
		}
	} else {
		d, _, fr := parseArt(a.source, ParseOpts{amiga: a.amiga(), binary: a.sourceBin, forceCols: a.forceCols})
		a.doc = d
		if fr >= 0 {
			for _, f := range fonts {
				if f.syncterm == fr && f.kind == a.mode {
					a.fontFor[a.mode] = f.id
				}
			}
		}
	}
	a.doc.ensureRows(max(a.doc.rows, 25))
	a.cx, a.cy, a.sel, a.float = 0, 0, nil, nil
}

var textExt = regexp.MustCompile(`(?i)\.(txt|asc|nfo|diz)$`)
var seqExt = regexp.MustCompile(`(?i)\.(seq|pet)$`)

// loadBytes reads a file into the current tab; as is "auto", "amiga", "pc" or "c64".
func (a *App) loadBytes(b []byte, name, path, as string) {
	a.srcKind, a.ownFont, a.ownPal = "", nil, nil
	if what := notArt(b); what != "" && as == "auto" {
		a.msg("Could not open " + name + ": it is " + what + ", not art")
		return
	}
	if isXBin(b) && as == "auto" {
		_, info, err := parseXBin(b)
		if err != nil {
			a.msg("Could not read the XBin: " + err.Error())
			return
		}
		a.mode, a.srcKind, a.ice = "pc", "xbin", info.ice
		if info.font != nil {
			info.font.h8 = info.cellH == 8
			a.ownFont, a.fontFor["pc"] = info.font, "xbin"
		}
		if info.pal != nil {
			a.ownPal, a.paletteFor["pc"] = info.pal, "file"
		}
		a.finishLoad(b, name, path, nil, "xbin")
		return
	}
	kind := ""
	switch {
	case isPetmate(b):
		kind = "petmate"
	case isMarq(b):
		kind = "marq"
	case strings.HasSuffix(strings.ToLower(name), ".prg") && isOurPrg(b):
		kind = "prg"
	case strings.HasSuffix(strings.ToLower(name), ".prg"):
		a.msg("Only C64 programs saved by CharEdit can be opened")
		return
	}
	if kind != "" && (as == "auto" || as == "c64") {
		if !a.hasC64 {
			t := a.Tab
			openC64Req(func() { a.focusTab(t); a.loadBytes(b, name, path, as) })
			return
		}
		scr, err := a.screens(b, kind)
		if err != nil {
			a.msg("Could not read " + name + ": " + err.Error())
			return
		}
		a.mode, a.srcKind, a.srcIndex = "c64", kind, 0
		own := map[string]string{"petmate": "petmate", "marq": "marq", "prg": "prg"}[kind]
		a.finishLoad(b, name, path, nil, own)
		first := a.Tab
		for i := 1; i < len(scr); i++ { // every further screen in a tab of its own
			t := a.newTab("c64")
			a.Tab = t
			a.mode, a.srcKind, a.srcIndex = "c64", kind, i
			label := scr[i].name
			if label == "" {
				label = fmt.Sprintf("screen %d", i+1)
			}
			a.finishLoad(b, name+" ("+label+")", "", nil, own)
		}
		a.Tab = first
		a.applyMode()
		if len(scr) > 1 {
			a.msg(fmt.Sprintf("%s has %d screens: each is in a tab of its own", name, len(scr)))
		}
		return
	}
	sauce, _ := readSauce(b)
	binary := strings.HasSuffix(strings.ToLower(name), ".bin") || (sauce != nil && sauce.dataType == 5)
	utf := sauce == nil && !binary && fromUtf8(b, true) != nil
	sfont := ""
	if sauce != nil {
		sfont = strings.ToLower(sauce.font)
	}
	mode := ""
	switch {
	case as != "auto":
		mode = as
	case seqExt.MatchString(name) && sauce == nil:
		mode = "c64"
	case strings.HasSuffix(strings.ToLower(name), ".ata") && sauce == nil:
		mode = "atari"
	case sauce == nil && !utf && looksAtascii(b):
		mode = "atari"
	case strings.HasPrefix(sfont, "amiga"):
		mode = "amiga"
	case sauce != nil && regexp.MustCompile(`^(ibm|cp437|vga|ega|mda)`).MatchString(sfont):
		mode = "pc"
	case binary:
		mode = "pc"
	case utf:
		mode = map[bool]string{true: "pc", false: "amiga"}[utf8LooksPc(b)]
	default:
		mode = map[bool]string{true: "amiga", false: "pc"}[guessAmiga(b)]
	}
	if mode == "c64" && !a.hasC64 {
		// Shown as PC for now; with the character set it is read again as PETSCII.
		t := a.Tab
		openC64Req(func() {
			a.focusTab(t)
			a.forceCols = 0
			a.setMode("c64")
		})
		mode = "pc"
	}
	a.mode = mode
	src := b
	if utf && mode != "c64" && mode != "atari" {
		src = fromUtf8(b, mode == "amiga")
		a.msg("Read as UTF-8 text")
	}
	a.ice = false
	if sauce != nil {
		for _, f := range fonts {
			if strings.EqualFold(f.sauce, sauce.font) && f.kind == mode {
				a.fontFor[mode] = f.id
			}
		}
		a.ice = sauce.flags&1 != 0
	}
	a.source, a.sourceBin, a.sauce, a.forceCols = src, binary && mode == "pc", sauce, 0
	a.info = Info{writeSauce: sauce != nil}
	if sauce != nil {
		a.info.title, a.info.author, a.info.group = sauce.title, sauce.author, sauce.group
	}
	a.path, a.name = path, name
	head := b[:min(len(b), 4096)]
	switch {
	case mode == "c64":
		a.saveFmt = "seq"
	case mode == "atari":
		a.saveFmt = "ata"
	case binary:
		a.saveFmt = "bin"
	case textExt.MatchString(name) && !bytes.ContainsAny(head, "\x1b\x9b"):
		a.saveFmt = "text"
	default:
		a.saveFmt = "ansi"
	}
	a.undo, a.redo = nil, nil
	a.reparse()
	a.applyMode()
	a.dirty = false
	a.scrollX, a.scrollY = 0, 0
	if a.doc.fullest {
		a.msg("An animation that ends nearly empty: showing its fullest picture (Play shows it all)")
	}
}

// notArt names files that are surely not art (archives, programs, pictures), or "".
func notArt(b []byte) string {
	for _, k := range []struct{ magic, what string }{
		{"PK\x03\x04", "a ZIP archive"}, {"Rar!", "a RAR archive"}, {"\x1f\x8b", "a gzip archive"}, {"7z\xbc\xaf", "a 7-Zip archive"},
		{"MZ", "a DOS or Windows program"}, {"\x7fELF", "a Linux program"}, {"\x89PNG", "a PNG picture"}, {"GIF8", "a GIF picture"},
		{"\xff\xd8\xff", "a JPEG picture"}, {"%PDF", "a PDF"},
	} {
		if bytes.HasPrefix(b, []byte(k.magic)) {
			return k.what
		}
	}
	return ""
}

// screensCache keeps the last file of screens read, so that the tabs of a file
// with many screens do not each read all of it again.
var screensCache struct {
	data *byte
	n    int
	kind string
	scr  []PetScreen
}

// screens reads a file that holds whole C64 screens.
func (a *App) screens(b []byte, kind string) ([]PetScreen, error) {
	if len(b) > 0 && screensCache.data == &b[0] && screensCache.n == len(b) && screensCache.kind == kind {
		out := make([]PetScreen, len(screensCache.scr))
		for i, s := range screensCache.scr { // each tab gets a page of its own
			out[i] = PetScreen{doc: s.doc.clone(), lower: s.lower, name: s.name}
		}
		return out, nil
	}
	scr, err := a.readScreens(b, kind)
	if err == nil && len(b) > 0 {
		screensCache.data, screensCache.n, screensCache.kind, screensCache.scr = &b[0], len(b), kind, scr
		out := make([]PetScreen, len(scr))
		for i, s := range scr {
			out[i] = PetScreen{doc: s.doc.clone(), lower: s.lower, name: s.name}
		}
		return out, nil
	}
	return scr, err
}

func (a *App) readScreens(b []byte, kind string) ([]PetScreen, error) {
	switch kind {
	case "petmate":
		return parsePetmate(b)
	case "prg":
		d, lower, err := parsePrg(b)
		if err != nil {
			return nil, err
		}
		return []PetScreen{{doc: d, lower: lower}}, nil
	}
	return parseMarq(b)
}

// finishLoad is the end of loading for files that are not streams of codes.
func (a *App) finishLoad(b []byte, name, path string, sauce *Sauce, fmtID string) {
	a.source, a.sourceBin, a.sauce, a.forceCols = b, false, sauce, 0
	a.info = Info{}
	a.path, a.name, a.saveFmt = path, name, fmtID
	a.undo, a.redo = nil, nil
	a.reparse()
	a.applyMode()
	a.dirty = false
	a.scrollX, a.scrollY = 0, 0
}

func (a *App) applyMode() {
	a.doc.glyph0 = a.screen()
	if a.tool == "half" && !a.pc() {
		a.setTool("draw")
	}
	if a.fset >= len(a.fkeys[a.mode]) {
		a.fset = 0
	}
	a.setPalette()
}

var modeNames = map[string]string{"amiga": "Amiga", "pc": "PC", "c64": "PETSCII", "atari": "Atari"}

func (a *App) setMode(m string) {
	if m == a.mode {
		return
	}
	if m == "c64" && !a.hasC64 {
		openC64Req(func() { a.setMode("c64") })
		return
	}
	was := a.mode
	a.mode = m
	switch {
	case a.source != nil && !a.dirty:
		a.reparse()
	case a.source == nil && !a.dirty && a.doc.usedRows() == 0:
		// An empty page takes the new machine's width and colours.
		switch {
		case m == "c64":
			a.doc = newDoc(a.mach().cols, a.mach().rows)
		case m == "atari":
			a.doc = newAtariDoc()
		case was == "c64" || was == "atari":
			a.doc = newDoc(80, 25)
		}
		fmtID, ext := map[string]string{"c64": "seq", "atari": "ata"}[m], ""
		if fmtID == "" {
			fmtID = "ansi"
		}
		ext = "." + map[string]string{"ansi": "ans", "seq": "seq", "ata": "ata"}[fmtID]
		a.saveFmt = fmtID
		if a.path == "" {
			a.name = "untitled" + ext
		}
	case a.dirty:
		a.msg("Shown with the " + modeNames[m] + " font and colours; the bytes are unchanged")
	}
	a.applyMode()
	savePrefs()
}

// setMachine makes a PETSCII page one for another Commodore machine: its
// character set, palette and width.
func (a *App) setMachine(id string) {
	m := machineByID(id)
	if !hasROM(m.family) {
		openC64Req(func() { a.setMachine(id) })
		return
	}
	lower := a.lower()
	a.machine, a.defMachine = id, id
	a.setCase(lower)
	a.paletteFor["c64"] = m.palette
	a.setPalette()
	switch {
	case a.source != nil && !a.dirty && (a.srcKind == "" || a.srcKind == "seq"):
		a.forceCols = m.cols
		a.reparse()
		a.setCase(lower)
	case a.source == nil && !a.dirty && a.doc.usedRows() == 0:
		a.doc = newDoc(m.cols, m.rows)
		a.applyMode()
	case a.doc.cols != m.cols:
		a.msg(fmt.Sprintf("The page stays %d columns wide; the %s has %d (W changes it)", a.doc.cols, m.name, m.cols))
	}
	savePrefs()
}

func (a *App) setCols(n int) {
	n = max(8, min(1000, n))
	if n == a.doc.cols {
		return
	}
	if a.source != nil && !a.dirty {
		a.forceCols = n
		a.reparse()
	} else {
		a.pushUndo()
		a.doc = a.doc.withCols(n)
	}
	a.cx = min(a.cx, n-1)
}

// newAtariDoc is an empty Atari screen in the Atari's own start-up colours.
func newAtariDoc() *Doc {
	d := newDoc(40, 24)
	d.bg, d.ink, d.border = atariBg, atariInk, 0
	return d
}

// ---------------------------------------------------------------- tabs

func (a *App) newTab(mode string) *Tab {
	t := &Tab{mode: mode, zoom: a.defZoom, fontFor: map[string]string{}, paletteFor: map[string]string{},
		info: Info{writeSauce: true}, saveFmt: "ansi", name: "untitled.ans"}
	for k, v := range a.defFont {
		t.fontFor[k] = v
	}
	for k, v := range a.defPalette {
		t.paletteFor[k] = v
	}
	if t.zoom == 0 {
		t.zoom = 2
	}
	t.ch8, t.smear = a.ch8Default, a.smearDefault
	t.doc = newDoc(80, 25)
	t.machine = a.defMachine
	if mode == "c64" {
		m := machineByID(t.machine)
		t.doc = newDoc(m.cols, m.rows)
		t.saveFmt, t.name = "seq", "untitled.seq"
	}
	if mode == "atari" {
		t.doc = newAtariDoc()
		t.saveFmt, t.name = "ata", "untitled.ata"
	}
	a.tabs = append(a.tabs, t)
	return t
}

// pristine: an untitled tab nobody has touched, which an opened file may take over.
func (t *Tab) pristine() bool {
	return t.path == "" && !t.dirty && t.source == nil && t.doc.usedRows() == 0 && len(t.undo) == 0
}

// focusTab makes t the tab being worked on, and shows it in the active pane.
func (a *App) focusTab(t *Tab) {
	if a.Tab == t {
		return
	}
	if a.Tab != nil {
		a.Tab.hover = nil
	}
	pane := 0
	if a.layout != "one" && a.shown[1] == a.Tab {
		pane = 1
	}
	if a.layout != "one" && a.shown[1-pane] == t {
		pane = 1 - pane // already on show in the other pane: just go there
	}
	a.shown[pane] = t
	a.Tab = t
	a.applyMode()
}

func (a *App) tabIndex(t *Tab) int {
	for i, o := range a.tabs {
		if o == t {
			return i
		}
	}
	return -1
}

func (a *App) cycleTab(n int) {
	i := a.tabIndex(a.Tab)
	a.focusTab(a.tabs[((i+n)%len(a.tabs)+len(a.tabs))%len(a.tabs)])
}

func (a *App) addTab() {
	a.focusTab(a.newTab(a.mode))
}

func (a *App) closeTab(t *Tab) {
	if t.dirty {
		a.dialog = &Confirm{text: "\"" + t.name + "\" has changes that are not saved. Close it and throw them away?",
			button: "Throw away", yes: func() { t.dirty = false; a.closeTab(t) }}
		return
	}
	i := a.tabIndex(t)
	a.tabs = append(a.tabs[:i], a.tabs[i+1:]...)
	if len(a.tabs) == 0 {
		a.newTab(t.mode)
	}
	next := a.tabs[min(i, len(a.tabs)-1)]
	for p := range a.shown {
		if a.shown[p] == t {
			a.shown[p] = next
		}
	}
	if a.Tab == t {
		a.Tab = nil
		a.focusTab(next)
	}
}

// setLayout shows one page, or two side by side ("cols") or one over the other ("rows").
func (a *App) setLayout(l string) {
	a.layout = l
	if l == "one" {
		a.shown = [2]*Tab{a.Tab, nil}
		return
	}
	a.shown[0] = a.Tab
	if a.shown[1] == nil || a.shown[1] == a.Tab || a.tabIndex(a.shown[1]) < 0 {
		a.shown[1] = nil
		for _, t := range a.tabs { // the next tab, or a new one
			if t != a.Tab {
				a.shown[1] = t
				break
			}
		}
		if a.shown[1] == nil {
			cur := a.Tab
			a.shown[1] = a.newTab(a.mode)
			a.Tab = cur
		}
	}
}

func (a *App) openPath(path, as string) {
	b, err := os.ReadFile(path)
	if err != nil {
		a.msg("Could not open: " + err.Error())
		return
	}
	for _, t := range a.tabs { // already open: go there
		if t.path == path && as == "auto" {
			a.focusTab(t)
			return
		}
	}
	if !a.pristine() {
		a.focusTab(a.newTab(a.mode))
	}
	a.loadBytes(b, filepath.Base(path), path, as)
	a.lastDir = filepath.Dir(path)
	savePrefs()
	if a.message == "" || time.Since(a.messageAt) > time.Second {
		a.msg("Opened " + path)
	}
}

func (a *App) anyDirty() int {
	n := 0
	for _, t := range a.tabs {
		if t.dirty {
			n++
		}
	}
	return n
}

// ---------------------------------------------------------------- flipping

// flip mirrors the floating block, or else the selection, left-right or upside down;
// the characters are swapped for their mirror images where the font has one.
func (a *App) flip(vertical bool) {
	f := a.font()
	mirror := func(b *Block) {
		out := make([]uint32, len(b.cells))
		for y := 0; y < b.h; y++ {
			for x := 0; x < b.w; x++ {
				sx, sy := b.w-1-x, y
				if vertical {
					sx, sy = x, b.h-1-y
				}
				v := b.cells[sy*b.w+sx]
				c := f.flipped(cellCh(v), vertical)
				out[y*b.w+x] = v&^255 | uint32(c)
			}
		}
		b.cells = out
	}
	switch {
	case a.float != nil:
		mirror(a.float)
	case a.sel != nil:
		r := a.selRect()
		b := a.doc.copyRect(r.x, r.y, r.w, r.h)
		mirror(b)
		a.pushUndo()
		for y := 0; y < b.h; y++ {
			for x := 0; x < b.w; x++ {
				a.doc.set(r.x+x, r.y+y, b.cells[y*b.w+x])
			}
		}
	default:
		a.msg("Select a block (Select tool) or paste one first, then flip it")
		return
	}
	a.msg(map[bool]string{false: "Flipped left-right", true: "Flipped upside down"}[vertical])
}

// ---------------------------------------------------------------- saving

type Format struct{ id, name, ext, modes string }

var formats = []Format{
	{"ansi", "ANSI (.ans)", ".ans", "amiga pc"},
	{"text", "Plain text, no colours (.asc)", ".asc", "amiga pc"},
	{"seq", "PETSCII stream (.seq)", ".seq", "c64"},
	{"ata", "ATASCII (.ata)", ".ata", "atari"},
	{"utf8", "UTF-8 text, no colours (.txt)", ".txt", "amiga pc c64 atari"},
	{"bin", "PC binary (.bin)", ".bin", "pc"},
	{"xbin", "XBin, font and palette inside (.xb)", ".xb", "amiga pc"},
	{"prg", "C64 program that shows it (.prg)", ".prg", "c64 (C64)"},
	{"marq", "C array: PETSCII editor, Petmate (.c)", ".c", "c64"},
	{"petmate", "Petmate workspace (.petmate)", ".petmate", "c64"},
	{"png", "Picture (.png)", ".png", "amiga pc c64 atari"},
	{"png2", "Picture, twice the size (.png)", ".png", "amiga pc c64 atari"},
	{"html", "Web page (.html)", ".html", "amiga pc c64 atari"},
}

// modeFormats are the formats the current page can be saved in.
func (a *App) modeFormats() []Format {
	var out []Format
	for _, f := range formats {
		if f.modes == "c64 (C64)" && (!a.c64() || a.machine != "c64") {
			continue // a C64 program runs on a C64 only
		}
		if strings.Contains(f.modes, a.mode) {
			out = append(out, f)
		}
	}
	return out
}

func (a *App) encode(fmtID string) ([]byte, error) {
	d, am, info := a.doc, a.amiga(), a.info
	withSauce := func(art []byte, fileType int) []byte {
		if !info.writeSauce {
			return art
		}
		flags := 0
		if a.ice {
			flags = 1
		}
		return append(art, makeSauce(SauceInfo{title: info.title, author: info.author, group: info.group,
			cols: d.cols, rows: max(1, d.usedRows()), font: a.font().sauce, fileType: fileType, flags: flags}, len(art))...)
	}
	nl := "\r\n"
	if !a.pc() {
		nl = "\n"
	}
	if (fmtID == "ansi" || fmtID == "text") && d.cols != 80 && !info.writeSauce {
		// Only the record tells other programs the width: without it the art falls apart.
		info.writeSauce, a.info.writeSauce = true, true
		a.msg(fmt.Sprintf("Saved with a SAUCE record: it is the only way to keep a width of %d", d.cols))
	}
	if limit := map[string]int{"seq": a.mach().cols, "ata": 40}[fmtID]; limit > 0 && d.cols > limit {
		a.msg(fmt.Sprintf("Only the first %d columns fit this format; the rest is not saved", limit))
		d = d.withCols(limit)
	}
	switch fmtID {
	case "ansi":
		return withSauce(writeAnsi(d, am, true, info.clear, info.writeSauce), 1), nil
	case "text":
		return withSauce(writeAnsi(d, am, false, false, info.writeSauce), 0), nil
	case "seq":
		return writeSeq(d, a.lower(), info.clear), nil
	case "ata":
		return writeAta(d, info.clear), nil
	case "utf8":
		return []byte(strings.Join(docText(d, a.runeOf, 0, 0, 0, 0), nl) + nl), nil
	case "bin":
		art := writeBin(d)
		if !info.writeSauce && d.cols == 160 {
			return art, nil // 160 is what a .bin without a record is taken to be
		}
		return append(art, makeSauce(SauceInfo{title: info.title, author: info.author, group: info.group,
			cols: d.cols, font: a.font().sauce, binaryText: true, flags: map[bool]int{true: 1}[a.ice]}, len(art))...), nil
	case "xbin":
		return writeXBin(d, a.pal, a.font(), a.cellH(), a.ice), nil
	case "prg", "marq":
		if d.cols != 40 || d.usedRows() > 25 {
			a.msg("Only the first 40x25 cells fit a C64 screen")
		}
		if fmtID == "prg" {
			return writePrg(d, a.lower()), nil
		}
		return writeMarq(d, a.lower()), nil
	case "petmate":
		return writePetmate(d, a.lower(), strings.TrimSuffix(a.name, filepath.Ext(a.name))), nil
	case "html":
		return a.writeHTML(), nil
	case "png", "png2":
		H, z := a.cellH(), 1
		if fmtID == "png2" {
			z = 2
		}
		rows := max(1, min(d.usedRows(), 32000/(H*z)))
		if a.screen() {
			rows = max(rows, map[bool]int{true: 25, false: 24}[a.c64()])
		}
		border := 0
		fb := &FB{}
		if a.screen() { // with the machine's border around it
			border = 32 * z
			fb.resize(d.cols*8*z+2*border, rows*H*z+2*border)
			fb.fill(Rect{0, 0, fb.w, fb.h}, a.pal[d.borderColour()])
		} else {
			fb.resize(d.cols*8*z, rows*H*z)
		}
		a.paintCells(fb, border, border, z, func(x, y int) uint32 { return d.get(x, y) }, d.cols, rows)
		img := image.NewRGBA(image.Rect(0, 0, fb.w, fb.h))
		for i, p := range fb.px {
			img.Pix[i*4], img.Pix[i*4+1], img.Pix[i*4+2], img.Pix[i*4+3] = byte(p>>16), byte(p>>8), byte(p), 255
		}
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	}
	return nil, fmt.Errorf("unknown format %s", fmtID)
}

func writeAtomic(p string, data []byte) error {
	dir := filepath.Dir(p)
	f, err := os.CreateTemp(dir, ".charcoal-")
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, err = f.Write(data)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	mode := os.FileMode(0o644)
	if st, e := os.Stat(p); e == nil {
		mode = st.Mode().Perm()
	}
	if err == nil {
		err = os.Chmod(tmp, mode)
	}
	if err == nil {
		err = os.Rename(tmp, p)
	}
	if err != nil {
		os.Remove(tmp)
	}
	return err
}

func (a *App) writeOut(path, fmtID string) bool {
	if st, err := os.Stat(filepath.Dir(path)); err != nil || !st.IsDir() {
		a.msg("Could not save: there is no drawer " + filepath.Dir(path))
		return false
	}
	data, err := a.encode(fmtID)
	if err == nil {
		err = writeAtomic(path, data)
	}
	if err != nil {
		a.msg("Could not save: " + err.Error())
		return false
	}
	if strings.Contains(" ansi text bin seq ata xbin marq petmate prg ", " "+fmtID+" ") { // the page's own file; the others are exports
		a.path, a.name, a.saveFmt = path, filepath.Base(path), fmtID
		a.source, a.sourceBin, a.forceCols = data, fmtID == "bin", 0
		a.srcKind = map[string]string{"xbin": "xbin", "marq": "marq", "petmate": "petmate", "prg": "prg"}[fmtID]
		a.srcIndex = 0
		a.dirty = false
	}
	a.lastDir = filepath.Dir(path)
	savePrefs()
	a.msg(fmt.Sprintf("Saved %s (%d bytes)", path, len(data)))
	return true
}

func (a *App) save(as bool) {
	if as || a.path == "" {
		openSaveRequester()
		return
	}
	a.writeOut(a.path, a.saveFmt)
}

// ---------------------------------------------------------------- playback at modem speed

type emulator interface {
	write([]byte)
	cursor() (int, int)
}

func (a *App) play() {
	if a.playing != nil {
		a.stopPlay(true)
		return
	}
	if a.source != nil && a.sourceBin && !a.dirty {
		a.msg("A .bin file has no codes to play")
		return
	}
	b := a.source
	if b == nil || a.dirty {
		if a.c64() {
			b = writeSeq(a.doc, a.lower(), false)
		} else if a.atari() {
			b = writeAta(a.doc, false)
		} else {
			b = writeAnsi(a.doc, a.amiga(), true, false)
		}
	}
	_, end := readSauce(b)
	b = b[:end]
	d := newDoc(a.doc.cols, 25)
	d.bg, d.ink, d.border = a.doc.bg, a.doc.ink, a.doc.border
	var em emulator = newTerm(d, a.amiga())
	if a.c64() {
		em = newPetTerm(d)
	}
	if a.atari() {
		em = newAtariTerm(d)
	}
	a.playing = &Play{bytes: b, term: em, saved: a.snapshot(), last: time.Now()}
	a.doc, a.sel, a.float = d, nil, nil
	a.scrollY = 0
}

func (a *App) playStep() {
	p := a.playing
	now := time.Now()
	n := max(1, int(now.Sub(p.last).Seconds()*float64(a.baud)/10+0.5))
	p.last = now
	end := min(len(p.bytes), p.pos+n)
	p.term.write(p.bytes[p.pos:end])
	p.pos = end
	x, y := p.term.cursor()
	a.cx, a.cy = min(x, a.doc.cols-1), y
	a.doc.ensureRows(a.cy + 1)
	a.ensureVisible(a.cx, a.cy)
	if p.pos >= len(p.bytes) {
		a.stopPlay(false)
	}
}

func (a *App) stopPlay(skip bool) {
	p := a.playing
	if skip {
		p.term.write(p.bytes[p.pos:])
	}
	a.playing = nil
	// Playing does not change the page: put back the page as it was.
	a.restore(p.saved)
	a.msg(fmt.Sprintf("Played %d bytes", len(p.bytes)))
}
