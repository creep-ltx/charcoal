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
	{id: "custom", name: "Custom"},
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
	term  *Term
	saved Snap
	last  time.Time
}

type App struct {
	doc                  *Doc
	mode                 string // "amiga" or "pc"
	fontFor, paletteFor  map[string]string
	zoom                 int
	ch8, smear, ice      bool
	tool                 string
	fg, bg, brush        int
	ital, under, insert  bool
	cx, cy               int
	path, name           string
	dirty                bool
	source               []byte // the file as read (or last saved), to read again with other rules
	sourceBin            bool
	sauce                *Sauce
	saveFmt              string
	forceCols            int
	info                 Info
	sel                  *Sel
	selecting            bool
	float, clip          *Block
	clipText             string
	undo, redo           []Snap
	fset, factive        int
	fkeys                map[string][][]int
	hover                *Hover
	last                 *Hover
	button               int
	playing              *Play
	blink                bool
	lastDir              string
	baud                 int
	message              string
	messageAt            time.Time
	scrollX, scrollY     int
	page                 Rect // where the page view was laid out
	custom               [16]uint32
	pal                  [16]uint32
	dialog               Dialog
	charHover            int
	quit                 bool
	bare                 bool // gadgets hidden: only the page and the status line
	needLayoutScrollDown bool
}

var a *App

func (a *App) amiga() bool { return a.mode == "amiga" }
func (a *App) font() *Font {
	if f := fontByID(a.fontFor[a.mode]); f != nil && f.amiga == a.amiga() {
		return f
	}
	if a.amiga() {
		return fonts[1]
	}
	return fonts[len(fonts)-1]
}
func (a *App) cellH() int {
	if a.ch8 && a.amiga() {
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
func (a *App) brushCell() uint32 { return makeCell(a.brush, a.fg, a.bg, a.styleFlags()) }

func (a *App) setPalette() {
	p := paletteByID(a.paletteFor[a.mode])
	if p.id == "custom" {
		a.pal = a.custom
	} else {
		a.pal = p.colors
	}
}

// printable: is this byte a glyph on the current page?
func (a *App) printable(c int) bool {
	if a.amiga() {
		return !(c < 0x20 || (c >= 0x7F && c < 0xA0))
	}
	return c != 0
}

func (a *App) charToByte(r rune) int {
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

func (a *App) msg(s string) {
	a.message = s
	a.messageAt = time.Now()
}

// ---------------------------------------------------------------- undo

func (a *App) snapshot() Snap { return Snap{a.doc.clone(), a.cx, a.cy} }
func (a *App) pushUndo() {
	a.undo = append(a.undo, a.snapshot())
	if len(a.undo) > 300 {
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
	a.setCell(a.cx, a.cy, makeCell(b, a.fg, a.bg, a.styleFlags()))
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
	a.brush, a.fg, a.bg = cellCh(v), cellFg(v), cellBg(v)
	a.ital, a.under = v&italic != 0, v&underline != 0
}

func (a *App) setTool(t string) {
	if t == "half" && a.amiga() {
		t = "draw"
	}
	a.tool = t
	if t != "select" && t != "type" && !a.selecting {
		a.sel = nil
	}
}

func (a *App) setBg(i int) {
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
	a.clipText = strings.Join(docText(a.doc, a.amiga(), r.x, r.y, r.w, r.h), "\n")
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
			b.cells[y*w+x] = makeCell(c, a.fg, a.bg, a.styleFlags())
			x++
		}
	}
	return b
}

// ---------------------------------------------------------------- loading

func (a *App) reparse() {
	d, _, fr := parseArt(a.source, ParseOpts{amiga: a.amiga(), binary: a.sourceBin, forceCols: a.forceCols})
	a.doc = d
	a.doc.ensureRows(max(d.rows, 25))
	if fr >= 0 {
		for _, f := range fonts {
			if f.syncterm == fr && f.amiga == a.amiga() {
				a.fontFor[a.mode] = f.id
			}
		}
	}
	a.cx, a.cy, a.sel, a.float = 0, 0, nil, nil
}

var textExt = regexp.MustCompile(`(?i)\.(txt|asc|nfo|diz)$`)

// loadBytes takes a file's bytes; as is "auto", "amiga" or "pc".
func (a *App) loadBytes(b []byte, name, path, as string) {
	sauce, _ := readSauce(b)
	binary := strings.HasSuffix(strings.ToLower(name), ".bin") || (sauce != nil && sauce.dataType == 5)
	utf := sauce == nil && !binary && fromUtf8(b, true) != nil
	var isAmiga bool
	sfont := ""
	if sauce != nil {
		sfont = strings.ToLower(sauce.font)
	}
	switch {
	case as != "auto":
		isAmiga = as == "amiga"
	case strings.HasPrefix(sfont, "amiga"):
		isAmiga = true
	case sauce != nil && regexp.MustCompile(`^(ibm|cp437|vga|ega|mda)`).MatchString(sfont):
		isAmiga = false
	case binary:
		isAmiga = false
	case utf:
		isAmiga = !utf8LooksPc(b)
	default:
		isAmiga = guessAmiga(b)
	}
	a.mode = "pc"
	if isAmiga {
		a.mode = "amiga"
	}
	src := b
	if utf {
		src = fromUtf8(b, isAmiga)
		a.msg("Read as UTF-8 text")
	}
	a.ice = false
	if sauce != nil {
		for _, f := range fonts {
			if strings.EqualFold(f.sauce, sauce.font) && f.amiga == isAmiga {
				a.fontFor[a.mode] = f.id
			}
		}
		a.ice = sauce.flags&1 != 0
	}
	a.source, a.sourceBin, a.sauce, a.forceCols = src, binary, sauce, 0
	a.info = Info{writeSauce: sauce != nil}
	if sauce != nil {
		a.info.title, a.info.author, a.info.group = sauce.title, sauce.author, sauce.group
	}
	a.path, a.name = path, name
	head := b[:min(len(b), 4096)]
	switch {
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
}

func (a *App) applyMode() {
	if a.tool == "half" && a.amiga() {
		a.setTool("draw")
	}
	if a.fset >= len(a.fkeys[a.mode]) {
		a.fset = 0
	}
	a.setPalette()
}

func (a *App) setMode(m string) {
	if m == a.mode {
		return
	}
	a.mode = m
	if a.source != nil && !a.dirty {
		a.reparse()
	} else if a.dirty {
		a.msg("Shown with the " + map[bool]string{true: "Amiga", false: "PC"}[a.amiga()] + " font and colours; the bytes are unchanged")
	}
	a.applyMode()
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

func (a *App) newDoc() {
	a.doc = newDoc(80, 25)
	a.source, a.sauce, a.path, a.name, a.saveFmt = nil, nil, "", "untitled.ans", "ansi"
	a.info = Info{writeSauce: true}
	a.undo, a.redo = nil, nil
	a.cx, a.cy, a.sel, a.float = 0, 0, nil, nil
	a.dirty = false
	a.scrollX, a.scrollY = 0, 0
}

func (a *App) openPath(path, as string) {
	b, err := os.ReadFile(path)
	if err != nil {
		a.msg("Could not open: " + err.Error())
		return
	}
	a.loadBytes(b, filepath.Base(path), path, as)
	a.lastDir = filepath.Dir(path)
	savePrefs()
	if a.message == "" || time.Since(a.messageAt) > time.Second {
		a.msg("Opened " + path)
	}
}

// ---------------------------------------------------------------- saving

var formats = []struct{ id, name, ext string }{
	{"ansi", "ANSI (.ans)", ".ans"},
	{"text", "Plain text, no colours (.asc)", ".asc"},
	{"utf8", "UTF-8 text, no colours (.txt)", ".txt"},
	{"bin", "PC binary (.bin)", ".bin"},
	{"png", "Picture (.png)", ".png"},
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
	if am {
		nl = "\n"
	}
	switch fmtID {
	case "ansi":
		return withSauce(writeAnsi(d, am, true, info.clear), 1), nil
	case "text":
		return withSauce(writeAnsi(d, am, false, false), 0), nil
	case "utf8":
		return []byte(strings.Join(docText(d, am, 0, 0, 0, 0), nl) + nl), nil
	case "bin":
		return writeBin(d), nil
	case "png":
		H := a.cellH()
		rows := max(1, min(d.usedRows(), 32000/H))
		fb := &FB{}
		fb.resize(d.cols*8, rows*H)
		a.paintCells(fb, 0, 0, 1, func(x, y int) uint32 { return d.get(x, y) }, d.cols, rows)
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
	f, err := os.CreateTemp(dir, ".amiansi-")
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
	if fmtID == "ansi" || fmtID == "text" || fmtID == "bin" { // the page's own file; png and utf8 are exports
		a.path, a.name, a.saveFmt = path, filepath.Base(path), fmtID
		a.source, a.sourceBin, a.forceCols = data, fmtID == "bin", 0
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
		b = writeAnsi(a.doc, a.amiga(), true, false)
	}
	_, end := readSauce(b)
	b = b[:end]
	d := newDoc(a.doc.cols, 25)
	a.playing = &Play{bytes: b, term: newTerm(d, a.amiga()), saved: a.snapshot(), last: time.Now()}
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
	a.cx, a.cy = min(p.term.x, a.doc.cols-1), p.term.y
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
