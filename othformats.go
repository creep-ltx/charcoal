package main

// More of the DOS editors' own files (the others are in editorformats.go):
//
//   - PabloDraw's own format (.pd; PabloDraw 3, Source/Pablo/Formats/Character/Types/Pablo.cs):
//     the font (a .NET string: the id of one of PabloDraw's fonts, or empty and the font
//     itself: width, height, count as int16 and the glyphs' rows), the palette (int16 count,
//     then r g b), then a TundraDraw body without its header (1 = position, 2/4/6 = colour
//     change as 0 r g b, characters 0-6 never stored). The width is the SAUCE record's (80
//     without one).
//   - TheDraw's own format (.TD, TheDraw 4.x; read only): a 367-byte setup (the width at 39,
//     the height at 41) and then each page layer as a uint16 length and the page "crunched"
//     as TheDraw's UNCRUNCH.PAS shows (0-15 text colour, 16-23 background, 24 next line,
//     25 n: n+1 spaces, 26 n c: n+1 times c, 27 blink on/off, 32-255 a character). The
//     pages open as frames. Layout after abydos' thedraw.c (Magnus Bergman), checked on
//     TheDraw 4.63's DEMO.TD and SHADING.TD.
//   - BASIC BSAVE screen dumps (.BSV; TheDraw, ACiDDraw): 0xFD, segment, offset and length
//     (uint16 each), then the screen memory: character and attribute, 80 columns.

import (
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
)

func init() {
	formats = append(formats, Format{"pd", "For other editors: PabloDraw (.pd)", ".pd", "amiga pc"},
		Format{"bsv", "For other editors: BASIC BSAVE screen (.bsv)", ".bsv", "pc"},
		Format{"td", "For other editors: TheDraw (.td)", ".td", "pc"})
	editorOwn["pd"], editorOwn["bsv"], editorOwn["td"] = true, true, true
	framesFormats["td"] = true // the frames are TheDraw's page layers
}

// ---------------------------------------------------------------- BSAVE

// isBSV: a BSAVE file of the text screen (named .bsv, segment B800 or B000).
func isBSV(name string, b []byte) bool {
	if !strings.HasSuffix(strings.ToLower(name), ".bsv") || len(b) < 7 || b[0] != 0xFD {
		return false
	}
	seg := binary.LittleEndian.Uint16(b[1:])
	return seg == 0xB800 || seg == 0xB000
}

// parseBSV reads a BSAVE screen: the bytes go where they were saved from (80 columns).
func parseBSV(b []byte) (*Doc, error) {
	if len(b) < 7 || b[0] != 0xFD {
		return nil, errors.New("not a BSAVE file")
	}
	off, n := int(binary.LittleEndian.Uint16(b[3:])), int(binary.LittleEndian.Uint16(b[5:]))
	data := b[7:]
	data = data[:min(len(data), n)&^1]
	start := off / 2
	cells := len(data) / 2
	rows := max(1, (start+cells+79)/80)
	d := newDoc(80, rows)
	for k := 0; k < cells; k++ {
		d.cells[start+k] = attrCell(data[2*k], data[2*k+1])
	}
	return d, nil
}

// writeBSV is a BSAVE screen dump of an 80-column page (at most 409 rows: the length is 16 bits).
func writeBSV(d *Doc) []byte {
	page := d
	if d.cols != 80 {
		page = d.withCols(80)
	}
	page = &Doc{cols: 80, rows: min(max(1, page.rows), 409), cells: page.cells}
	body := writeBin(page)
	out := []byte{0xFD, 0x00, 0xB8, 0, 0}
	out = binary.LittleEndian.AppendUint16(out, uint16(len(body)))
	return append(out, body...)
}

// ---------------------------------------------------------------- TheDraw .TD

func isTD(b []byte) bool {
	return len(b) > 18 && b[0] == 0x15 && string(b[1:18]) == "TheDraw Save File"
}

// parseTD reads the page layers of a TheDraw 4.x save file.
func parseTD(b []byte) ([]*Doc, error) {
	if !isTD(b) || len(b) < 369 {
		return nil, errors.New("not a TheDraw save file")
	}
	if b[22] < 1 || b[23] != '4' {
		return nil, fmt.Errorf("a save file of TheDraw %q: CharEdit reads those of TheDraw 4", strings.TrimSpace(string(b[23:23+min(int(b[22]), 4)])))
	}
	w, h := int(binary.LittleEndian.Uint16(b[39:])), int(binary.LittleEndian.Uint16(b[41:]))
	if w < 1 || w > 255 || h < 1 || h > 1000 {
		return nil, fmt.Errorf("a TheDraw page of %dx%d", w, h)
	}
	var pages []*Doc
	for o := 367; o+2 <= len(b) && len(pages) < 64 && (len(pages)+1)*w*h <= maxPageCells; {
		n := int(binary.LittleEndian.Uint16(b[o:]))
		if o+2+n > len(b) {
			break
		}
		d, ok := uncrunchTD(b[o+2:o+2+n], w, h)
		if !ok {
			break
		}
		pages = append(pages, d)
		o += 2 + n
	}
	if len(pages) == 0 {
		return nil, errors.New("a TheDraw save file without a page")
	}
	return pages, nil
}

// uncrunchTD draws crunched data on a w x h page from its top left (UNCRUNCH.PAS); false for
// a code TheDraw does not use (28-31).
func uncrunchTD(src []byte, w, h int) (*Doc, bool) {
	d := newDoc(w, h)
	fillCells(d.cells, attrCell(32, 0x07))
	attr, pos, line := byte(0), 0, 0
	put := func(c byte) {
		if pos >= 0 && pos < w*h {
			d.cells[pos] = attrCell(c, attr)
		}
		pos++
	}
	for i := 0; i < len(src); i++ {
		c := src[i]
		switch {
		case c >= 32:
			put(c)
		case c < 16:
			attr = attr&0xF0 | c
		case c < 24:
			attr = attr&0x8F | (c-16)<<4
		case c == 24:
			line += w
			pos = line
		case c == 25 || c == 26:
			if i+1 >= len(src) || c == 26 && i+2 >= len(src) {
				return d, true
			}
			n, ch := int(src[i+1])+1, byte(32)
			i++
			if c == 26 {
				ch = src[i+1]
				i++
			}
			for k := 0; k < n; k++ {
				put(ch)
			}
		case c == 27:
			attr ^= 0x80
		default:
			return nil, false
		}
	}
	return d, true
}

// tdMaxPages is how many page layers TheDraw keeps (its manual: "8 page layers").
const tdMaxPages = 8

// tdHeader is the 367-byte setup TheDraw 4.60 writes before the pages (its
// settings as it keeps them in memory: the fields CharEdit fills are known from
// TheDraw's own files; the rest as TheDraw 4.63's DEMO_IMG.TD and SHADING.TD have
// them, the junk it leaves in unused bytes as zeros). Bytes 55-62 are the eight
// page layers: 2 holds a page, 1 an unused one.
func tdHeader(w, h, pages, cur int) []byte {
	b := make([]byte, 367)
	copy(b, "\x15TheDraw Save File.\r\n\x1a\x044.60")
	put := func(o, v int) { binary.LittleEndian.PutUint16(b[o:], uint16(v)) }
	put(27, 1)     // the cursor's column (from 1)
	put(29, 1)     // its row
	put(31, cur+1) // the page layer shown (from 1)
	put(33, 8)
	put(35, 1)
	put(37, 1)
	put(39, w)
	put(41, h)
	put(49, 0x0E60)
	for i := 0; i < tdMaxPages; i++ {
		b[55+i] = 1
		if i < pages {
			b[55+i] = 2
		}
	}
	ruler := func(o int) { // the tab ruler: a mark every 10 columns
		b[o] = 80
		for i := 1; i <= 80; i++ {
			b[o+i] = 0xC4
			if i%10 == 0 {
				b[o+i] = 0xFE
			}
		}
	}
	ruler(63)
	b[144], b[145], b[153], b[162] = 0x07, 0x0F, 0xD4, 1
	ruler(164)
	put(340, w)
	put(342, h)
	put(344, 1)
	put(346, 1)
	return b
}

// crunchTD is a page in TheDraw's crunched form (UNCRUNCH.PAS, the reverse of
// uncrunchTD): colour codes where the attribute changes, runs of spaces and of
// one character, a line code (24) per row. Every cell is written, blank ones too:
// TheDraw clears its worksheet to its own colours (white on black as set up), not
// to grey, so a cell left out would come back another colour (*verified* in
// TheDraw 4.63).
func crunchTD(d *Doc, w, h int) []byte {
	var out []byte
	attr := byte(0)
	for y := 0; y < h; y++ {
		if y > 0 {
			out = append(out, 24)
		}
		end := w
		for x := 0; x < end; {
			v := d.get(x, y)
			c, at := byte(cellCh(v)), cellAttr(v)
			n := 1
			for x+n < end && n < 256 && d.get(x+n, y)&0xFFFF == v&0xFFFF {
				n++
			}
			if at&0x0F != attr&0x0F {
				out = append(out, at&0x0F)
			}
			if at&0x70 != attr&0x70 {
				out = append(out, 16+(at>>4)&7)
			}
			if at&0x80 != attr&0x80 {
				out = append(out, 27)
			}
			attr = at
			switch {
			case c == ' ' && n >= 3:
				out = append(out, 25, byte(n-1))
			case c < 32 || n >= 4:
				out = append(out, 26, byte(n-1), c)
			default:
				for k := 0; k < n; k++ {
					out = append(out, c)
				}
			}
			x += n
		}
	}
	return out
}

// tdSize is the page size a .td is written at: TheDraw's worksheets are 80 or 160
// columns wide (its manual, "Worksheet width"); the height is the tallest page's.
func tdSize(docs []*Doc) (w, h int) {
	for _, d := range docs {
		w, h = max(w, d.cols), max(h, d.rows)
	}
	return map[bool]int{true: 80, false: 160}[w <= 80], max(1, h)
}

// writeTD writes pages (at most 8) as a TheDraw 4.60 save file; cur is the page
// shown. A page that crunches to more than 64 KB cannot be stored (its length is 16 bits).
func writeTD(docs []*Doc, cur int) ([]byte, error) {
	docs = docs[:min(len(docs), tdMaxPages)]
	w, h := tdSize(docs)
	out := tdHeader(w, h, len(docs), max(0, min(cur, len(docs)-1)))
	for i, d := range docs {
		c := crunchTD(d, w, h)
		if len(c) > 0xFFFF {
			return nil, fmt.Errorf("page %d is too busy for a TheDraw file (%d bytes crunched, 65535 at most)", i+1, len(c))
		}
		out = binary.LittleEndian.AppendUint16(out, uint16(len(c)))
		out = append(out, c...)
	}
	return out, nil
}

// ---------------------------------------------------------------- PabloDraw .pd

func isPD(name string) bool { return strings.HasSuffix(strings.ToLower(name), ".pd") }

// pdFonts are PabloDraw's own font ids and CharEdit's fonts for them.
var pdFonts = map[string]string{
	"CP 437 8x16": "vga", "CP 437 8x8": "vga50", "CP 437 8x14": "ega", "CP 850 8x16": "vga850", "CP 866 8x16": "vga866",
	"amiga-topaz-500": "topaz1", "amiga-topaz-plus-500": "topaz1p", "amiga-topaz-1200": "topaz2",
	"amiga-topaz-plus-1200": "topaz2p", "amiga-microknight": "mk", "amiga-microknight+": "mkp",
	"amiga-mOsOul": "mosoul", "amiga-pot-noodle": "noodle",
}

// pdFontID is PabloDraw's id of a CharEdit font ("" for one it has not).
func pdFontID(id string) string {
	for k, v := range pdFonts {
		if v == id {
			return k
		}
	}
	return ""
}

// netString reads a .NET BinaryWriter string: its length in 7-bit groups, then UTF-8.
func netString(b []byte) (string, []byte, error) {
	n, shift := 0, 0
	for i := 0; i < len(b) && i < 5; i++ {
		n |= int(b[i]&0x7F) << shift
		if b[i] < 0x80 {
			if n > len(b)-i-1 {
				break
			}
			return string(b[i+1 : i+1+n]), b[i+1+n:], nil
		}
		shift += 7
	}
	return "", nil, errors.New("cut short")
}

func appendNetString(out []byte, s string) []byte {
	n := len(s)
	for n >= 0x80 {
		out = append(out, byte(n)|0x80)
		n >>= 7
	}
	return append(append(out, byte(n)), s...)
}

// pdPage is a PabloDraw file read.
type pdPage struct {
	doc    *Doc
	info   XBinInfo
	fontID string // PabloDraw's font id ("" for the file's own font)
	mode   string // "pc", or "amiga" for an Amiga font
	note   string
}

// parsePD reads a PabloDraw file; the width is the SAUCE record's.
func parsePD(b []byte) (*pdPage, error) {
	p := &pdPage{mode: "pc"}
	cols := 80
	if s, _ := readSauce(b); s != nil && s.tinfo1 > 0 && s.tinfo1 <= 4096 {
		cols = s.tinfo1
	}
	b = b[:dataEnd(b)]
	id, b, err := netString(b)
	if err != nil {
		return nil, errors.New("not a PabloDraw file: " + err.Error())
	}
	p.fontID = id
	if id == "" { // the font itself
		if len(b) < 6 {
			return nil, errors.New("a PabloDraw font cut short")
		}
		fw, fh, n := int(int16(binary.LittleEndian.Uint16(b))), int(int16(binary.LittleEndian.Uint16(b[2:]))), int(int16(binary.LittleEndian.Uint16(b[4:])))
		per := fh * ((fw + 7) / 8)
		if fw < 1 || fw > 16 || fh < 1 || fh > 32 || n < 1 || n > 4096 || len(b) < 6+n*per {
			return nil, fmt.Errorf("a PabloDraw font of %d %dx%d glyphs", n, fw, fh)
		}
		if fw <= 9 && fh >= 6 && fh <= 16 {
			data := make([]byte, 0, min(n, 256)*fh)
			for c := 0; c < min(n, 256); c++ {
				for r := 0; r < fh; r++ {
					data = append(data, b[6+c*per+r*((fw+7)/8)])
				}
			}
			f := newFont(ownID, "PabloDraw's font", "pc", data, fh, min(n, 256))
			f.format = "pd"
			p.info.font = f
		} else {
			p.note = fmt.Sprintf("its font of %dx%d pixels does not fit a cell here", fw, fh)
		}
		b = b[6+n*per:]
	} else if f, ok := pdFonts[id]; ok {
		p.info.font = fontByID(f)
		if strings.HasPrefix(id, "amiga-") {
			p.mode = "amiga"
		}
	} else {
		p.note = fmt.Sprintf("PabloDraw's font %q is shown as the VGA's", id)
	}
	if len(b) < 2 {
		return nil, errors.New("a PabloDraw palette cut short")
	}
	n := int(int16(binary.LittleEndian.Uint16(b)))
	if n < 0 || n > 256 || len(b) < 2+3*n {
		return nil, fmt.Errorf("a PabloDraw palette of %d colours", n)
	}
	var filePal []uint32
	for i := 0; i < n; i++ {
		filePal = append(filePal, uint32(b[2+3*i])<<16|uint32(b[3+3*i])<<8|uint32(b[4+3*i]))
	}
	body := b[2+3*n:]
	// The page's colours: the file's first 16 (PabloDraw's attribute order is the BIOS's).
	var bios [16]uint32
	vga := biosPalette(paletteByID("vga").colors)
	for i := range bios {
		bios[i] = vga[i]
		if i < len(filePal) {
			bios[i] = 0xFF000000 | filePal[i]
		}
	}
	cellPal := biosPalette(bios)
	p.info.pal, p.info.ice = &cellPal, true
	colour := func(rgb uint32) int { return nearestOf(rgb, cellPal) }
	// The body: TundraDraw's commands (PabloDraw's Tundra.Load).
	maxRows := maxRowsFor(cols)
	var cells []uint32
	x, y, fg, bg := 0, 0, 0, 0 // PabloDraw starts at attribute 0 (black on black)
	put := func(c byte) {
		if x >= cols {
			x, y = 0, y+1
		}
		if y >= maxRows {
			return
		}
		for len(cells) <= y*cols+x {
			cells = append(cells, blank)
		}
		cells[y*cols+x] = makeCell(int(c), fg, bg, 0)
		x++
	}
	be32 := func(i int) int { return int(int32(binary.BigEndian.Uint32(body[i:]))) }
	rgb := func(i int) uint32 { return uint32(body[i+1])<<16 | uint32(body[i+2])<<8 | uint32(body[i+3]) }
	for i := 0; i < len(body); i++ {
		switch c := body[i]; {
		case c == 1:
			if i+8 >= len(body) {
				i = len(body)
				break
			}
			y, x = be32(i+1), be32(i+5)
			if y < 0 || y >= maxRows || x < 0 || x >= cols {
				y, x = 0, 0
			}
			i += 8
		case c >= 2 && c <= 6:
			need := 1 + 4*map[bool]int{true: 1}[c&2 != 0] + 4*map[bool]int{true: 1}[c&4 != 0]
			if i+need >= len(body) {
				i = len(body)
				break
			}
			ch, k := body[i+1], i+2
			if c&2 != 0 {
				fg, k = colour(rgb(k)), k+4
			}
			if c&4 != 0 {
				bg, k = colour(rgb(k)), k+4
			}
			put(ch)
			i = k - 1
		default:
			put(c)
		}
	}
	rows := max(1, (len(cells)+cols-1)/cols)
	d := newDoc(cols, rows)
	copy(d.cells, cells)
	p.doc = d
	if f := p.info.font; f != nil {
		p.info.cellH = f.rows()
	}
	return p, nil
}

// writePD writes a page as PabloDraw's own file (the SAUCE record, needed for a width other
// than 80, is the caller's). missed counts characters 0-6, which the format cannot hold
// (PabloDraw skips them; they are written as nothing, so they read back as blanks).
func writePD(d *Doc, pal [16]uint32, f *Font, ice bool) (out []byte, missed int) {
	if id := pdFontID(f.id); id != "" {
		out = appendNetString(out, id)
	} else {
		out = appendNetString(out, "")
		h := f.rows()
		out = binary.LittleEndian.AppendUint16(out, 8)
		out = binary.LittleEndian.AppendUint16(out, uint16(h))
		out = binary.LittleEndian.AppendUint16(out, 256)
		for c := 0; c < 256; c++ {
			for r := 0; r < h; r++ {
				out = append(out, f.row(c, r))
			}
		}
	}
	bios := biosPalette(pal)
	out = binary.LittleEndian.AppendUint16(out, 16)
	for _, c := range bios {
		out = append(out, byte(c>>16), byte(c>>8), byte(c))
	}
	// TundraDraw's body as PabloDraw writes it: rows to the last that is not blank, a position
	// after characters 0-6, colours where they change (a space's text colour never).
	endy := -1
	for y := 0; y < d.rows; y++ {
		for x := 0; x < d.cols; x++ {
			if v := d.get(x, y); cellCh(v) != 32 && cellCh(v) != 0 || shownBg(v, ice) != 0 {
				endy = y
			}
		}
	}
	fg, bg, blank := 0, 0, true // PabloDraw starts at attribute 0
	put := func(c int) { out = append(out, 0, byte(bios[c]>>16), byte(bios[c]>>8), byte(bios[c])) }
	for y := 0; y <= endy; y++ {
		for x := 0; x < d.cols; x++ {
			v := d.get(x, y)
			ch := cellCh(v)
			if ch <= 6 {
				if ch != 0 {
					missed++
				}
				blank = true
				continue
			}
			if blank {
				out = append(out, 1)
				out = binary.BigEndian.AppendUint32(out, uint32(y))
				out = binary.BigEndian.AppendUint32(out, uint32(x))
				blank = false
			}
			f, b := biosANSI[cellFg(v)], biosANSI[shownBg(v, ice)]
			cmd := byte(0)
			if ch != 32 && f != fg {
				cmd |= 2
			}
			if b != bg {
				cmd |= 4
			}
			if cmd != 0 {
				out = append(out, cmd)
			}
			out = append(out, byte(ch))
			if cmd&2 != 0 {
				put(f)
				fg = f
			}
			if cmd&4 != 0 {
				put(b)
				bg = b
			}
		}
	}
	return out, missed
}

// ---------------------------------------------------------------- in CharEdit

// othLoad opens a .pd, .TD or .bsv into the tab (state as icyMachineLoad's).
func (a *App) othLoad(kind string, b []byte, name, as string) (state int, note string) {
	switch kind {
	case "pd":
		p, err := parsePD(b)
		if err != nil {
			a.msg("Could not read " + name + ": " + err.Error())
			return icyStop, ""
		}
		if as != "auto" && as != p.mode {
			return icyNotThis, ""
		}
		a.mode, a.ice = p.mode, p.info.ice
		if f := p.info.font; f != nil && f.id == ownID {
			a.ownFont, a.fontFor[p.mode] = f, ownID
		} else if f != nil {
			a.fontFor[p.mode] = f.id
		}
		if p.mode == "pc" {
			a.ownPal, a.paletteFor["pc"] = p.info.pal, "file"
		}
		return icyGoOn, p.note
	case "td", "bsv":
		if as != "auto" && as != "pc" {
			return icyNotThis, ""
		}
		if kind == "td" {
			if _, err := parseTD(b); err != nil {
				a.msg("Could not read " + name + ": " + err.Error())
				return icyStop, ""
			}
		}
		a.mode, a.ice = "pc", false
		return icyGoOn, ""
	}
	return icyPC, ""
}

// saveNote is what the last encoding has to say after "Saved file": writeOut
// adds it to its message (takeSaveNote) and clears it.
var saveNote string

func takeSaveNote() string {
	s := saveNote
	saveNote = ""
	return s
}

// othAfterLoad: a .TD's page layers are frames; Ctrl+S writes the .TD again.
func (a *App) othAfterLoad(kind string) {
	if kind != "td" {
		return
	}
	a.saveFmt = "td"
	if n := a.nFrames(); n > 1 {
		a.msg(fmt.Sprintf("%s: TheDraw's %d page layers as frames (%s and %s go through them)", a.name, n, keyName("frame_previous"), keyName("frame_next")))
	}
}

// othReparse reads the page again from its .pd, .TD or .bsv; false for other files.
func (a *App) othReparse() bool {
	switch a.srcKind {
	case "pd":
		if p, err := parsePD(a.source); err == nil {
			a.doc = p.doc
		}
	case "bsv":
		if d, err := parseBSV(a.source); err == nil {
			a.doc = d
		}
	case "tdf": // a font tab (fed.go): the letter on the page stays
	case "td":
		pages, err := parseTD(a.source)
		if err != nil {
			break
		}
		a.frames = nil
		if len(pages) > 1 {
			a.frames = make([]Frame, len(pages))
			for i, d := range pages {
				a.frames[i] = Frame{doc: d, lower: -1, src: -1, name: fmt.Sprintf("page %d", i+1)}
			}
		}
		a.doc = pages[0].clone()
	default:
		return false
	}
	return true
}

// othEncode writes a .pd or .bsv; ok false for other formats.
func (a *App) othEncode(fmtID string, d *Doc) (out []byte, ok bool) {
	switch fmtID {
	case "pd":
		var missed int
		out, missed = writePD(d, a.pal, a.font(), a.ice)
		if missed > 0 {
			a.msg(fmt.Sprintf("%d cells hold characters 1-6, which a PabloDraw file cannot: they are left out", missed))
		}
		if a.info.writeSauce || d.cols != 80 {
			out = append(out, makeSauce(SauceInfo{title: a.info.title, author: a.info.author, group: a.info.group, cols: d.cols,
				rows: max(1, d.rows), font: a.font().sauce, fileType: 8, flags: map[bool]int{true: 1}[a.ice]}, len(out))...)
		}
		return out, true
	case "td":
		docs := a.allDocs()
		w, h := tdSize(docs)
		var why []string
		if len(docs) > tdMaxPages {
			why = append(why, fmt.Sprintf("only the first %d frames (TheDraw has %d page layers)", tdMaxPages, tdMaxPages))
			docs = docs[:tdMaxPages]
		}
		if w > 160 {
			why = append(why, "only the first 160 columns")
		}
		out, err := writeTD(docs, a.cur)
		if err != nil {
			a.msg("Could not save: " + err.Error())
			return nil, true
		}
		saveNote = fmt.Sprintf(" as a %dx%d TheDraw page", w, h)
		if len(why) > 0 {
			saveNote += ", " + strings.Join(why, " and ")
		}
		return out, true
	case "tdf": // the font editor (fed.go)
		out, err := a.fedEncode()
		if err != nil {
			a.msg("Could not save: " + err.Error())
			return nil, true
		}
		return out, true
	case "bsv":
		if d.cols != 80 || d.rows > 409 {
			a.msg("A BSAVE screen is 80 columns and at most 409 rows: the rest is not saved")
		}
		return writeBSV(d), true
	}
	return nil, false
}
