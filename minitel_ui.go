package main

// The Minitel and Viewdata pages in the editor: opening and saving their
// files, typing (big letters), picking, the Sixel tool on mosaics, and the
// toggles for the attributes new cells get. The pages themselves are in
// minitel.go and viewdata.go.

import (
	"fmt"
	"path/filepath"
	"strings"
)

// mtFkeys are the Minitel page's F-key sets: mosaics (brush codes 0xC0 + blocks), then signs.
var mtFkeys = [][]int{
	{0xFF, 0xD5, 0xEA, 0xC3, 0xF0, 0xCC, 0xC0, 0xC1, 0xC2, 0xFC},
	{0xC4, 0xC8, 0xD0, 0xE0, 0xC5, 0xCA, 0xD4, 0xE8, 0xF5, 0xFA},
	{0x7F, 0x60, 0x7B, 0x7C, 0x7D, 0x7E, 0x5F, 0x2D, 0x2B, 0x2A},
	{0x80, 0x81, 0x86, 0x87, 0x88, 0x89, 0x8A, 0x8B, 0x82, 0x84},
	{0x90, 0x93, 0x94, 0x95, 0x8F, 0x91, 0x99, 0x9A, 0x9D, 0x8D},
}

// mtSizeFlags are the cell flags of the typing size.
func (a *App) mtSizeFlags() uint32 {
	return [4]uint32{0, mtDW, mtDH, mtDW | mtDH}[a.mtSize&3]
}

// mtType types brush code b at the cursor: a mosaic takes a cell, a letter
// one, two or four (the cursor is on its bottom row, as on the Minitel).
func (a *App) mtType(b int) {
	v := a.cell(b)
	if mtMosaic(v) || a.cy == 0 { // no big letters on the status row
		a.setCell(a.cx, a.cy, v)
		a.cx++
		return
	}
	f := a.mtSizeFlags()
	if a.cx >= a.doc.cols-1 {
		f &^= mtDW // at the last column a wide letter is a normal one
	}
	if a.cy < 2 {
		f &^= mtDH // nor tall on row 1
	}
	v |= f
	a.setCell(a.cx, a.cy, v)
	if f&mtDW != 0 {
		a.setCell(a.cx+1, a.cy, v|mtRight)
	}
	if f&mtDH != 0 {
		a.setCell(a.cx, a.cy-1, v|mtTop)
		if f&mtDW != 0 {
			a.setCell(a.cx+1, a.cy-1, v|mtRight|mtTop)
		}
	}
	a.cx++
	if f&mtDW != 0 {
		a.cx++
	}
	if f != 0 && !a.mtSizeShown {
		a.mtSizeShown = true
		a.msg("A big letter is written from its bottom row, as on the Minitel: the cursor stays on it")
	}
}

// mtPick takes a cell's character, colours and attributes for the brush.
func (a *App) mtPick(v uint32) {
	a.brush = mtBrushOf(v)
	a.fg = cellFg(v) & 7
	if mtMosaic(v) || cellCh(v) == 0x20 {
		a.bg = cellBg(v) & 7
	}
	a.mtAttr = v & (mtBlink | mtInv | mtLine | mtMask)
	if !mtMosaic(v) {
		a.mtSize = map[uint32]int{0: 0, mtDW: 1, mtDH: 2, mtDW | mtDH: 3}[v&(mtDW|mtDH)]
	}
}

// mtPaintSixel sets (on) or clears one of a cell's 2x3 blocks; gx, gy count
// blocks. A letter under it becomes a mosaic on the background it showed.
func (a *App) mtPaintSixel(gx, gy int, on bool) {
	x, y := gx/2, gy/3
	if x < 0 || y < 0 || x >= a.doc.cols || y >= a.doc.rows {
		return
	}
	bit := (gy%3)*2 + gx%2
	old := a.doc.get(x, y)
	bits, fg, bg := 0, a.fg, mtDecode(a.doc, y)[x].bg
	if mtMosaic(old) {
		bits, fg, bg = mtBits(cellCh(old)), cellFg(old)&7, cellBg(old)&7
	} else if old&mtInv != 0 {
		bg = cellFg(old) & 7
	}
	if on {
		bits |= 1 << bit
		fg = a.fg
	} else {
		bits &^= 1 << bit
	}
	a.doc.set(x, y, makeCell(mtCode(bits), fg, bg, mtG1|a.mtAttr&(mtLine|mtBlink)))
}

// mtBlockOpRefused says why a Minitel block cannot be turned over this way, or "".
func mtBlockOpRefused(b *Block, op int) string {
	if op != turnCW {
		return ""
	}
	for _, v := range b.cells {
		if mtMosaic(v) && mtBits(cellCh(v)) != 0 && mtBits(cellCh(v)) != 63 {
			return "Minitel mosaics cannot be turned: a cell's blocks are 2 wide and 3 high"
		}
	}
	return ""
}

// vdKinds are the Viewdata files' kinds (srcKind and save format alike).
var vdKinds = map[string]bool{"vdcpt": true, "vdbin": true, "vdtel": true}

// loadVideotex opens a Minitel .vdt or a Viewdata file into the current tab;
// false if b is neither.
func (a *App) loadVideotex(b []byte, name, path, as string) bool {
	lname := strings.ToLower(name)
	ext := filepath.Ext(lname)
	tele := isTTI(b) || isEditTF(b) || isEP1(b) || isMode7(b, name)
	kind := ""
	switch {
	case as == "mt":
		kind = "vdt"
	case as == "auto" && (ext == ".vdt" || ext == "" && isVDT(b, name)):
		kind = "vdt"
	case (as == "auto" || as == "vd") && isTelstar(b):
		kind = "vdtel"
	case (as == "auto" || as == "vd") && isVDBin(b, name):
		kind = "vdbin"
	case as == "auto" && (ext == ".cpt" || ext == ".car"):
		kind = "vdcpt"
	case as == "vd" && !tele:
		kind = "vdcpt"
	default:
		return false
	}
	a.srcKind, a.ownFont, a.ownLower, a.ownPal = kind, nil, nil, nil
	a.mode = map[bool]string{true: "mt", false: "vd"}[kind == "vdt"]
	a.fg, a.bg = 7, 0
	a.finishLoad(b, name, path, nil, kind)
	return true
}

// reparseVideotex reads the page again from its file; false if it is not a
// Minitel or Viewdata page read from one.
func (a *App) reparseVideotex() bool {
	if a.source == nil || !a.mt() && !(a.tt() && vdKinds[a.srcKind]) {
		return false
	}
	var docs []*Doc
	note := ""
	switch {
	case a.mt():
		docs = parseVDT(a.source)
		if len(docs) > 1 {
			note = mtNote(a.name, len(docs))
		}
	case a.srcKind == "vdbin":
		docs = []*Doc{parseVDBin(a.source)}
	case a.srcKind == "vdtel":
		d, n := parseTelstar(a.source)
		docs = []*Doc{d}
		if n != "" {
			note = a.name + ": " + n
		}
	default:
		docs = parseVDStream(a.source)
		if len(docs) > 1 {
			note = fmt.Sprintf("%s: %d frames (each CS starts one)", a.name, len(docs))
		}
	}
	if !a.vd() && !a.mt() { // a Viewdata file shown as a teletext page: 25 rows
		for _, d := range docs {
			d.ensureRows(25)
			for i := 24 * 40; i < 25*40; i++ {
				d.cells[i] = makeCell(0x20, 0, 0, 0)
			}
		}
	}
	a.doc = docs[0].clone()
	if len(docs) > 1 {
		a.frames = make([]Frame, len(docs))
		for i, d := range docs {
			a.frames[i] = Frame{doc: d, lower: -1, src: -1}
		}
	}
	a.cx, a.cy, a.sel, a.float = 0, 0, nil, nil
	if note != "" {
		a.msg(note)
	}
	return true
}

// vdCut makes a teletext page read as Viewdata 24 rows.
func (a *App) vdCut() {
	if a.doc.rows > vdRows {
		a.doc.rows = vdRows
	}
	for i := range a.frames {
		if a.frames[i].doc.rows > vdRows {
			a.frames[i].doc.rows = vdRows
		}
	}
}

// encodeVideotex writes the Minitel and Viewdata pages' own formats; ok false for other formats.
func (a *App) encodeVideotex(fmtID string, d *Doc) (out []byte, ok bool, err error) {
	docs := []*Doc{d}
	if a.nFrames() > 1 {
		docs = a.allDocs()
	}
	switch fmtID {
	case "vdt":
		for _, f := range docs {
			out = append(out, mtWrite(f)...)
		}
		return out, true, nil
	case "vdcpt":
		for _, f := range docs {
			out = append(out, writeVDStream(f, true)...)
		}
		return out, true, nil
	case "vdbin":
		return writeVDBin(d), true, nil
	case "vdtel":
		name := a.name
		if cpcTarget != "" { // the file being written (writeOut)
			name = filepath.Base(cpcTarget)
		}
		var old []byte
		if a.srcKind == "vdtel" {
			old = a.source
		}
		out, err = writeTelstar(d, old, name)
		return out, true, err
	}
	return nil, false, nil
}

// drawMTToggles draws the Minitel's attribute toggles with toggle (panel.go).
func (a *App) drawMTToggles(toggle func(label string, on bool, hint string) bool) {
	flag := func(label string, f uint32, hint string) {
		if toggle(label, a.mtAttr&f != 0, hint) {
			a.mtAttr |= f
		} else {
			a.mtAttr &^= f
		}
	}
	flag("Flash", mtBlink, "Flash: new cells blink (ESC H)")
	flag("Inv", mtInv, "Inverse: new letters swap colour and background (ESC ])")
	flag("Line", mtLine, "Underline for a zone of letters (a space starts it), separated blocks for mosaics (ESC Z)")
	flag("Mask", mtMask, "Conceal: a space typed with it hides the letters after it until the next zone (ESC X)")
	sizes := []string{"1x", "Wide", "Tall", "2x"}
	if toggle(sizes[a.mtSize&3], a.mtSize != 0, "The size letters are typed in: normal, double width, double height, double size (click for the next)") != (a.mtSize != 0) {
		a.mtSize = (a.mtSize + 1) & 3
	}
	a.showCodes = toggle("Reveal", a.showCodes, "Show concealed cells")
}

// mtPlayer shows a Minitel stream as it comes: the Minitel's screen, settled
// into the page shown after every piece.
type mtPlayer struct {
	t *mtTerm
	d *Doc
}

func (p *mtPlayer) write(b []byte) {
	p.t.feed(b)
	copy(p.d.cells, p.t.page().cells)
}

func (p *mtPlayer) cursor() (int, int) { return p.t.x, p.t.y }

// vdPlayer does the same for a Viewdata terminal.
type vdPlayer struct {
	s *vdScreen
	d *Doc
}

func (p *vdPlayer) write(b []byte) {
	p.s.feed(b)
	copy(p.d.cells, p.s.d.cells)
}

func (p *vdPlayer) cursor() (int, int) { return p.s.x, p.s.y }

// videotexPlayer is the page Play shows and the terminal that draws it.
func (a *App) videotexPlayer() (*Doc, emulator) {
	if a.mt() {
		d := newMTDoc()
		return d, &mtPlayer{t: newMTTerm(), d: d}
	}
	d := newVDDoc()
	return d, &vdPlayer{s: newVDScreen(), d: d}
}

// videotexPlayBytes is what Play sends on a Minitel or Viewdata page: the file
// as read, or the page (every frame) written as a stream; nil on other pages.
func (a *App) videotexPlayBytes() []byte {
	switch {
	case a.mt() && a.source != nil && !a.dirty && a.srcKind == "vdt",
		a.vd() && a.source != nil && !a.dirty && a.srcKind == "vdcpt":
		return a.source
	case a.mt() || a.vd():
		out, _, _ := a.encodeVideotex(map[bool]string{true: "vdt", false: "vdcpt"}[a.mt()], a.doc)
		return out
	}
	return nil
}
