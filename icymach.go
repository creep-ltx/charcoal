package main

// IcyDraw's pages for other machines: PETSCII (C64, C128 40 and 80 columns,
// VIC-20, PET 40 and 80, C16/Plus/4), ATASCII and the Atari ST's VT52 screen.
// IcyDraw 0.6 keeps them in the same .icy file as its PC pages (editorformats.go),
// with the ICED header's buffer type (2 PETSCII, 3 ATASCII, 5 Atari ST) and a
// SCREEN record: the machine and mode, the screen colour and the border.
//
// What a cell holds there (icy_tools, crates/icy_engine/src/formats/io/seq.rs,
// atascii.rs, vt52.rs, text_buffer/screen_metadata.rs; checked with files its
// engine wrote and read):
//   - PETSCII: the character is the screen code, 128-255 reversed; the colour is
//     the machine's colour number: the C64's order, the VIC-20's 8, the PET's 0/1,
//     the C16's luminance x 16 + hue (palette entries 16-127, so the file is ICED
//     version 2), the C128's 80 columns the VDC's RGBI order. A blinking cell is
//     the TED's flash; on the VDC font 1 is the lower case set, per cell. Every
//     cell's background is the screen colour.
//   - ATASCII: the character is the ATASCII code, 128-255 inverse; a background
//     other than 0 also asks for inverse (IcyDraw's ATASCII export). The screen's
//     two colours are palette entries 0 (background) and 7 (text).
//   - Atari ST: the pens, as the VT52 screen's (low 16, medium 4, high 2).
// The font records hold the machine's character set (the ROM's; a PETSCII page
// whose set differs opens with it as its own font).

import (
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"strings"
)

const (
	icyPETSCII = 2 // buffer types
	icyATASCII = 3
	icyAtariST = 5
)

// icyMachineBuffer: does CharEdit open this buffer type on a machine's page?
func icyMachineBuffer(t int) bool { return t == icyPETSCII || t == icyATASCII || t == icyAtariST }

// icyScreen is an IcyDraw file's SCREEN record.
type icyScreen struct {
	machine, mode int // IcyDraw's numbers (icyMachines); 0: none
	bg, border    int
	hasBg         bool
	hasBorder     bool
}

// icyMachines are IcyDraw's SCREEN machine numbers of the Commodore machines, as
// CharEdit's machine ids (IcyDraw's C16 is CharEdit's Plus/4 page). 8 is the
// Atari ST, 9 the Atari 8-bit, 10 Viewdata, 11 BBC Mode 7.
var icyMachines = map[int]string{1: "c64", 2: "c128", 3: "vic20", 4: "pet", 5: "plus4", 6: "pet80", 7: "c128vdc"}

func icyMachineNumber(id string) int {
	for n, m := range icyMachines {
		if m == id {
			return n
		}
	}
	return 1
}

// parseIcyScreen reads a SCREEN record: version 1, then fields of tag, length 4 and value
// (1 machine and mode as two u16, 2 border, 3 screen colour as u32). IcyDraw refuses a file
// with a field it does not know or one twice; so does CharEdit.
func parseIcyScreen(b []byte) (icyScreen, error) {
	var s icyScreen
	if len(b) < 2 || binary.LittleEndian.Uint16(b) != 1 {
		return s, errors.New("an IcyDraw SCREEN record of an unknown version")
	}
	b = b[2:]
	seen := 0
	for len(b) > 0 {
		if len(b) < 8 {
			return s, errors.New("an IcyDraw SCREEN record is cut short")
		}
		tag, n := int(binary.LittleEndian.Uint16(b)), int(binary.LittleEndian.Uint16(b[2:]))
		if tag < 1 || tag > 3 || n != 4 || seen&(1<<tag) != 0 {
			return s, fmt.Errorf("an IcyDraw SCREEN field %d of %d bytes", tag, n)
		}
		seen |= 1 << tag
		v := b[4:8]
		switch tag {
		case 1:
			s.machine, s.mode = int(binary.LittleEndian.Uint16(v)), int(binary.LittleEndian.Uint16(v[2:]))
			if s.machine < 1 || s.machine > 11 || s.mode > 2 {
				return s, fmt.Errorf("an IcyDraw screen of machine %d, mode %d", s.machine, s.mode)
			}
		case 2:
			s.border, s.hasBorder = int(binary.LittleEndian.Uint32(v)&255), true
		case 3:
			s.bg, s.hasBg = int(binary.LittleEndian.Uint32(v)&255), true
		}
		b = b[8:]
	}
	return s, nil
}

// icyFontRec is a font record kept as it is: its name and glyph rows.
type icyFontRec struct {
	name string
	bits []byte // n glyphs of h rows
	h, n int
}

// icyKeepFont keeps fonts 0 and 1 of a FONT record (PSF, 8 pixels wide).
func icyKeepFont(doc *icyDoc, data []byte) {
	if len(data) < 2 || data[0] > 1 || doc.fonts[data[0]].bits != nil {
		return
	}
	name, rest, err := icyStr(data[1:])
	if err != nil {
		return
	}
	bits, h, n, _, err := readPSF(rest)
	if err != nil || h < 1 || h > 32 {
		return
	}
	doc.fonts[data[0]] = icyFontRec{name, bits, h, n}
}

// glyph8 is glyph c of the font in 8 rows (a font of 16 rows has every row twice).
func (f icyFontRec) glyph8(c int) []byte {
	out := make([]byte, 8)
	if c >= f.n {
		return out
	}
	for r := range out {
		src := r
		if f.h == 16 {
			src = r * 2
		}
		if src < f.h {
			out[r] = f.bits[c*f.h+src]
		}
	}
	return out
}

// icyPetsciiOf is the Commodore machine and case of an IcyDraw PETSCII page: its SCREEN
// record, else the name of its font (as IcyDraw's petscii_charset), else the C64's upper case.
func icyPetsciiOf(doc *icyDoc) (string, bool) {
	if id, ok := icyMachines[doc.screen.machine]; ok {
		return id, doc.screen.mode == 1
	}
	name := strings.ToLower(doc.fonts[0].name)
	lower := strings.Contains(name, "unshifted") || strings.Contains(name, "lower")
	for _, p := range []struct{ prefix, id string }{{"c128 vdc", "c128vdc"}, {"c128", "c128"}, {"pet 80", "pet80"},
		{"vic-20", "vic20"}, {"pet", "pet"}, {"c16", "plus4"}} {
		if strings.HasPrefix(name, p.prefix) {
			return p.id, lower
		}
	}
	return "c64", lower
}

// icyStartColours are the text and screen colours IcyDraw gives a new page of the
// machine: the screen colour of a file from before the SCREEN record.
var icyStartColours = map[string][2]int{"c64": {14, 6}, "c128": {13, 11}, "vic20": {6, 1}, "pet": {1, 0},
	"pet80": {1, 0}, "c128vdc": {15, 0}, "plus4": {0, 0x71}}

// vdcOfC64 is the VDC's RGBI colour the C128 shows in 80 columns for each C64 colour
// number (IcyDraw's VDC_COLORS_OF_C64; CharEdit's 80-column page keeps the C64's numbers).
var vdcOfC64 = [16]int{0, 15, 8, 7, 11, 4, 2, 13, 10, 12, 9, 1, 6, 5, 3, 14}

func c64OfVDC(v int) int {
	for c, x := range vdcOfC64 {
		if x == v&15 {
			return c
		}
	}
	return 1
}

// icyMach is an IcyDraw page of another machine as CharEdit opens it.
type icyMach struct {
	mode    string // "c64", "atari" or "st"
	machine string // a PETSCII page's machine
	lower   bool
	stres   string
	doc     *Doc
	font    []byte // a PETSCII page's character set when it is not the machine's ROM: 256 x 8 rows
	notes   []string
}

// icyNearest is the colour of pal nearest to an IcyDraw colour that is no palette number.
func icyNearest(c icyColour, filePal []uint32, pal []uint32) int {
	var rgb uint32
	switch c.kind {
	case 'x':
		rgb = xterm256(int(c.v))
	case 'r':
		rgb = c.v
	case 'p':
		if int(c.v) < len(filePal) {
			rgb = filePal[c.v]
		}
	}
	best, bd := 0, 1<<62
	for i, p := range pal {
		dr, dg, db := int(rgb>>16&255)-int(p>>16&255), int(rgb>>8&255)-int(p>>8&255), int(rgb&255)-int(p&255)
		if d := dr*dr + dg*dg + db*db; d < bd {
			best, bd = i, d
		}
	}
	return best
}

// icyMachinePage reads an IcyDraw page of another machine (nil for a PC page). gtia is the
// Atari colour table the screen's two colours are matched to.
func icyMachinePage(doc *icyDoc, gtia *[256]uint32) *icyMach {
	if !icyMachineBuffer(doc.buffer) {
		return nil
	}
	cells := icyMerge(doc, false)
	m := &icyMach{}
	switch doc.buffer {
	case icyPETSCII:
		icyPetsciiPage(doc, cells, m)
	case icyATASCII:
		icyAtasciiPage(doc, cells, m, gtia)
	case icyAtariST:
		icyStPage(doc, cells, m)
	}
	if n := len(doc.layers); n > 1 {
		m.notes = append(m.notes, fmt.Sprintf("its %d layers are put together into one page", n))
	}
	if len(doc.skipped) > 0 {
		m.notes = append(m.notes, "left out: "+strings.Join(uniq(doc.skipped), ", "))
	}
	return m
}

func icyPetsciiPage(doc *icyDoc, cells []icyMerged, m *icyMach) {
	m.mode = "c64"
	m.machine, m.lower = icyPetsciiOf(doc)
	mach := machineByID(m.machine)
	ted, vdc := m.machine == "plus4", m.machine == "c128vdc"
	start := icyStartColours[m.machine]
	// The colour of a cell's colour in the machine's numbers.
	machPal := doc.pal
	colour := func(c icyColour, none int) int {
		switch {
		case c.kind == 0 || c.kind == 't':
			return none
		case c.kind == 'p' && (ted && c.v < 128 || c.v < 16):
			return int(c.v)
		}
		if len(machPal) == 0 {
			return none
		}
		return icyNearest(c, doc.pal, machPal)
	}
	bg := start[1]
	switch {
	case doc.screen.hasBg:
		bg = doc.screen.bg
	default: // a file from before SCREEN: the first visible cell's background (IcyDraw's migration)
		for _, c := range cells {
			if c.seen && c.bg.kind != 't' && c.bg.kind != 0 {
				bg = colour(c.bg, bg)
				break
			}
		}
	}
	cols, rows := mach.cols, max(1, doc.h)
	if doc.w > cols {
		m.notes = append(m.notes, fmt.Sprintf("%d columns: the %s's screen shows %d", doc.w, mach.name, cols))
	}
	d := newDoc(cols, rows)
	d.glyph0 = true
	blankAttr := start[0]
	switch {
	case ted:
		d.bg, d.border = bg&127, bg&127
		if doc.screen.hasBorder {
			d.border = doc.screen.border & 127
		}
		blankAttr = 0
	case vdc:
		d.bg, d.border = c64OfVDC(bg), -1
	case mach.mono:
		d.bg, d.border = 0, 0
	default:
		d.bg, d.border = bg&15, -1
		if doc.screen.hasBorder {
			d.border = doc.screen.border & 15
		}
	}
	// The C128's 80 columns pick the set per cell; CharEdit's page shows one: the one most cells use.
	var sets [2]int
	if vdc {
		for _, c := range cells {
			if c.seen && c.ch != ' ' {
				sets[c.page&1]++
			}
		}
		m.lower = sets[1] > sets[0]
	}
	odd, other := 0, 0
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			v := makeCell(32, blankAttr&15, blankAttr>>4&15, 0)
			if mach.mono {
				v = makeCell(32, 1, 0, 0)
			}
			if x < doc.w && y < doc.h {
				c := cells[y*doc.w+x]
				if c.seen {
					ch := int(c.ch)
					if ch < 0 || ch > 255 {
						ch, odd = 32, odd+1
					}
					fg := colour(c.fg, start[0])
					switch {
					case ted:
						attr := fg & 127
						if c.attr&icyBlinkAttr != 0 {
							attr |= 128
						}
						v = tedCell(ch, attr)
					case vdc:
						if (c.page&1 == 1) != m.lower && ch != 32 {
							other++
						}
						v = makeCell(ch, c64OfVDC(fg), 0, 0)
					case mach.mono:
						v = makeCell(ch, 1, 0, 0)
					default:
						v = makeCell(ch, fg&15, 0, 0)
					}
				}
			}
			d.cells[y*cols+x] = v
		}
	}
	m.doc = d
	if other > 0 {
		m.notes = append(m.notes, fmt.Sprintf("%d cells in the other character set show in this one", other))
	}
	if odd > 0 {
		m.notes = append(m.notes, fmt.Sprintf("%d cells hold no screen code: spaces", odd))
	}
	// The character set: the file's own when it is not the machine's ROM.
	f := doc.fonts[0]
	if vdc && m.lower {
		f = doc.fonts[1]
	}
	if f.bits != nil && (f.h == 8 || f.h == 16) && f.n >= 256 {
		set := make([]byte, 0, 256*8)
		for c := 0; c < 256; c++ {
			set = append(set, f.glyph8(c)...)
		}
		rom := fontByID(mach.family + map[bool]string{true: "lower", false: "upper"}[m.lower])
		if rom == nil || !sameGlyphs8(rom, set) {
			m.font = set
		}
	}
}

// sameGlyphs8: does the font hold these 256 glyphs of 8 rows?
func sameGlyphs8(f *Font, set []byte) bool {
	for c := 0; c < 256; c++ {
		for r := 0; r < 8; r++ {
			if f.row(c, r) != set[c*8+r] {
				return false
			}
		}
	}
	return true
}

// icyAtariPal is IcyDraw's ATASCII palette: entry 0 the screen, 7 the text (the rest unused).
var icyAtariPal = [16]uint32{0x095183, 0x0000AA, 0x00AA00, 0x00AAAA, 0xAA0000, 0xAA00AA, 0xAA5500, 0x65B7E9,
	0x555555, 0x5555FF, 0x55FF55, 0x55FFFF, 0xFF5555, 0xFF55FF, 0xFFFF55, 0xFFFFFF}

func icyAtasciiPage(doc *icyDoc, cells []icyMerged, m *icyMach, gtia *[256]uint32) {
	m.mode = "atari"
	pal := doc.pal
	if len(pal) < 8 {
		pal = icyAtariPal[:]
	}
	cols, rows := 40, max(1, doc.h)
	if doc.w > cols {
		m.notes = append(m.notes, fmt.Sprintf("an XEP80 page of %d columns: the Atari's screen shows the first 40", doc.w))
	}
	d := newDoc(cols, rows)
	d.bg, d.border, d.glyph0 = gtiaNearest(pal[0], gtia), atariBorder, true
	d.ink = gtiaNearest(pal[7], gtia)
	for y := 0; y < rows && y < doc.h; y++ {
		for x := 0; x < cols && x < doc.w; x++ {
			c := cells[y*doc.w+x]
			if !c.seen {
				continue
			}
			ch := int(c.ch) & 255
			if c.bg.kind == 'p' && c.bg.v > 0 || c.bg.kind == 'r' || c.bg.kind == 'x' { // IcyDraw's export: inverse
				ch |= 128
			}
			d.cells[y*cols+x] = makeCell(ch, 7, 0, 0)
		}
	}
	m.doc = d
}

var icySTRes = []string{"stlow", "stmed", "sthigh"}

func icyStPage(doc *icyDoc, cells []icyMerged, m *icyMach) {
	m.mode = "st"
	switch {
	case doc.screen.machine == 8:
		m.stres = icySTRes[doc.screen.mode]
	case doc.w <= 40:
		m.stres = "stlow"
	case len(doc.pal) > 0 && len(doc.pal) <= 2:
		m.stres = "sthigh"
	default:
		m.stres = "stmed"
	}
	r := stResByID(m.stres)
	resPal := paletteByID(r.palette).colors
	pens := make([]uint32, r.colours)
	for i := range pens {
		pens[i] = resPal[i] & 0xFFFFFF
	}
	pen := func(c icyColour, none int) int {
		switch {
		case c.kind == 0 || c.kind == 't':
			return none
		case c.kind == 'p' && int(c.v) < r.colours:
			return int(c.v)
		}
		return icyNearest(c, doc.pal, pens)
	}
	cols, rows := r.cols, max(1, doc.h)
	if doc.w > cols {
		m.notes = append(m.notes, fmt.Sprintf("%d columns: the ST's screen shows %d", doc.w, cols))
	}
	d := newSTDoc(r)
	d.cols, d.rows, d.cells = cols, rows, make([]uint32, cols*rows)
	fillCells(d.cells, makeCell(32, r.fg, r.bg, 0))
	for y := 0; y < rows && y < doc.h; y++ {
		for x := 0; x < cols && x < doc.w; x++ {
			c := cells[y*doc.w+x]
			if c.seen {
				d.cells[y*cols+x] = makeCell(int(c.ch)&255, pen(c.fg, r.fg), pen(c.bg, r.bg), 0)
			}
		}
	}
	m.doc = d
}

// ---------------------------------------------------------------- writing

// icyOut is an IcyDraw document to write: one layer of cells.
type icyOut struct {
	buffer       int
	ice, fontMod byte
	w, h         int
	fontH        int
	pal          []uint32
	fonts        []icyFontRec // slot = place in the list
	screen       []byte       // the SCREEN record (nil: none)
	// cell gives a cell: its character, colours (palette numbers), font and attributes.
	cell func(x, y int) (ch uint32, fg, bg int, page byte, attr uint16)
}

func icyColourBytes(b []byte, c int) []byte {
	if c < 16 {
		return append(b, byte(1+c))
	}
	return append(b, 19, byte(c))
}

// encode writes the document: a PNG of preview (nil: one transparent pixel) with the records.
func (o *icyOut) encode(preview image.Image) []byte {
	type rec struct {
		key  string
		data []byte
	}
	var layer []byte
	layer = append(layer, icyString("Background")...)
	layer = append(layer, 0, 0, 0, 0, 0)               // mode normal, no colour
	layer = binary.LittleEndian.AppendUint32(layer, 1) // visible
	layer = append(layer, 0, 0, 0, 0, 0, 0, 0, 0)      // offset
	layer = binary.LittleEndian.AppendUint32(layer, uint32(o.w))
	layer = binary.LittleEndian.AppendUint32(layer, uint32(o.h))
	wide := false // a colour past 15: ICED version 2 (IcyDraw's custom palette)
	for y := 0; y < o.h; y++ {
		for x := 0; x < o.w; x++ {
			ch, fg, bg, page, attr := o.cell(x, y)
			wide = wide || fg > 15 || bg > 15
			layer = binary.LittleEndian.AppendUint32(layer, ch)
			layer = icyColourBytes(layer, fg)
			layer = icyColourBytes(layer, bg)
			layer = append(layer, page, byte(attr), byte(attr>>8))
		}
	}
	version := uint16(1)
	if wide {
		version = 2
	}
	head := binary.LittleEndian.AppendUint16(nil, version)
	head = append(head, 0, 0, 0) // no compression, reserved
	head = binary.LittleEndian.AppendUint16(head, uint16(o.buffer))
	head = append(head, o.ice, o.fontMod)
	head = binary.LittleEndian.AppendUint32(head, uint32(o.w))
	head = binary.LittleEndian.AppendUint32(head, uint32(o.h))
	head = append(head, 8, byte(o.fontH))
	recs := []rec{{"ICED", head}}
	var p strings.Builder
	fmt.Fprintf(&p, "ICE Palette\n#Palette Name: \n#Author: \n#Description: \n#Colors: %d\n", len(o.pal))
	for _, c := range o.pal {
		fmt.Fprintf(&p, "%02x%02x%02x\n", c>>16&255, c>>8&255, c&255)
	}
	recs = append(recs, rec{"PALETTE", []byte(p.String())})
	if o.screen != nil {
		recs = append(recs, rec{"SCREEN", o.screen})
	}
	for slot, f := range o.fonts {
		font := []byte{byte(slot)}
		font = append(font, icyString(f.name)...)
		for _, v := range []uint32{0x864AB572, 0, 32, 0, 256, uint32(f.h), uint32(f.h), 8} {
			font = binary.LittleEndian.AppendUint32(font, v)
		}
		font = append(font, f.bits[:256*f.h]...)
		recs = append(recs, rec{"FONT", font})
	}
	recs = append(recs, rec{"LAYER", layer}, rec{"END", nil})
	var raw [][]byte
	for _, r := range recs {
		raw = append(raw, icyRecord(r.key, r.data))
	}
	return icyPNG(raw, preview)
}

// icyScreenRecord is a SCREEN record: the machine and mode, and the border and screen colour (-1: none).
func icyScreenRecord(machine, mode, border, bg int) []byte {
	out := binary.LittleEndian.AppendUint16(nil, 1)
	field := func(tag int, v uint32) {
		out = binary.LittleEndian.AppendUint16(out, uint16(tag))
		out = binary.LittleEndian.AppendUint16(out, 4)
		out = binary.LittleEndian.AppendUint32(out, v)
	}
	field(1, uint32(machine)|uint32(mode)<<16)
	if border >= 0 {
		field(2, uint32(border))
	}
	if bg >= 0 {
		field(3, uint32(bg))
	}
	return out
}

// icyFontOf is a font as an IcyDraw font record of h rows (8, or 16: an 8-row font's rows twice).
func icyFontOf(name string, f *Font, h int) icyFontRec {
	bits := make([]byte, 0, 256*h)
	src := f.rows()
	for c := 0; c < 256; c++ {
		for r := 0; r < h; r++ {
			switch {
			case src == h:
				bits = append(bits, f.row(c, r))
			case src == 8 && h == 16:
				bits = append(bits, f.row(c, r/2))
			default: // 16 rows to 8: every other row
				bits = append(bits, f.row(c, r*src/h))
			}
		}
	}
	return icyFontRec{name: name, bits: bits, h: h, n: 256}
}

// icyCBMNames are the names IcyDraw gives the Commodore character sets (seq.rs petscii_font).
var icyCBMNames = map[string]string{"c64": "C64", "c128": "C128", "vic20": "VIC-20", "pet": "PET", "plus4": "C16",
	"pet80": "PET 80", "c128vdc": "C128 VDC"}

// icyCaseFont is the character set of the page's machine in the upper or lower case (the page's own when it has one).
func (a *App) icyCaseFont(lower bool) *Font {
	if a.ownInUse() != nil {
		if !lower && a.ownFont != nil {
			return a.ownFont
		}
		if lower && a.ownLower != nil {
			return a.ownLower
		}
	}
	if a.lower() == lower {
		return a.font()
	}
	if f := fontByID(a.mach().family + map[bool]string{true: "lower", false: "upper"}[lower]); f != nil {
		return f
	}
	return a.font()
}

// writeIcyMachine writes a PETSCII, Atari or ST page as IcyDraw's page of that machine; ok
// false on other pages.
func (a *App) writeIcyMachine(d *Doc, preview image.Image) (out []byte, ok bool) {
	o := &icyOut{fontMod: 1, w: d.cols, h: max(1, d.rows), fontH: 8}
	switch {
	case a.c64():
		mach := a.mach()
		ted, vdc := a.ted(), a.machine == "c128vdc"
		lower := a.lower()
		o.buffer = icyPETSCII
		if vdc || a.machine == "pet80" {
			o.fontH = 16
		}
		name := icyCBMNames[a.machine]
		cs := map[bool]string{true: "lower", false: "upper"}
		if vdc { // both sets, font 1 the lower case; the page shows one of them
			o.fonts = []icyFontRec{icyFontOf(name+" PETSCII upper", a.icyCaseFont(false), 16), icyFontOf(name+" PETSCII lower", a.icyCaseFont(true), 16)}
		} else {
			o.fonts = []icyFontRec{icyFontOf(name+" PETSCII "+cs[lower], a.font(), o.fontH)}
		}
		bg, border := d.bg&15, d.border
		switch {
		case ted:
			bg = d.bg & 127
			if border < 0 {
				border = bg
			}
			border &= 127
			for c := 0; c < 128; c++ {
				o.pal = append(o.pal, a.tedRGB(c)&0xFFFFFF)
			}
		case vdc:
			bg, border = vdcOfC64[d.bg&15], -1
			o.pal = make([]uint32, 16)
			for c := 0; c < 16; c++ {
				o.pal[vdcOfC64[c]] = a.pal[c] & 0xFFFFFF
			}
		case mach.mono:
			bg, border = 0, -1
			o.pal = []uint32{a.pal[0] & 0xFFFFFF, a.pal[1] & 0xFFFFFF}
		default:
			if border < 0 {
				border = bg
			}
			border &= 15
			for _, c := range a.pal {
				o.pal = append(o.pal, c&0xFFFFFF)
			}
		}
		mode := 0
		if lower && !vdc {
			mode = 1
		}
		o.screen = icyScreenRecord(icyMachineNumber(a.machine), mode, border, bg)
		o.cell = func(x, y int) (uint32, int, int, byte, uint16) {
			v := d.get(x, y)
			ch := uint32(cellCh(v))
			switch {
			case ted:
				at := tedAttr(v)
				return ch, at & 127, bg, 0, uint16(at>>7) * icyBlinkAttr
			case vdc:
				return ch, vdcOfC64[cellFg(v)], bg, map[bool]byte{true: 1}[lower], 0
			case mach.mono:
				return ch, 1, 0, 0, 0
			}
			return ch, cellFg(v), bg, 0, 0
		}
	case a.atari():
		o.buffer = icyATASCII
		o.pal = append([]uint32(nil), icyAtariPal[:]...)
		o.pal[0], o.pal[7] = a.gtia()[d.bg&0xFE]&0xFFFFFF, a.gtia()[atariText(d)&0xFE]&0xFFFFFF
		o.screen = icyScreenRecord(9, 0, -1, -1)
		o.fonts = []icyFontRec{icyFontOf("Atari ATASCII", a.font(), 8)}
		o.cell = func(x, y int) (uint32, int, int, byte, uint16) {
			return uint32(cellCh(d.get(x, y))), 7, 0, 0, 0
		}
	case a.st():
		r := a.res()
		o.buffer = icyAtariST
		for i := 0; i < r.colours; i++ {
			o.pal = append(o.pal, a.pal[i]&0xFFFFFF)
		}
		name, h := "Atari ST 8x8", 8
		switch r.id {
		case "stmed":
			name, h = "Atari ST 8x8 medium", 16
		case "sthigh":
			name, h = "Atari ST 8x16", 16
		}
		o.fontH = h
		o.fonts = []icyFontRec{icyFontOf(name, a.font(), h)}
		o.screen = icyScreenRecord(8, map[string]int{"stlow": 0, "stmed": 1, "sthigh": 2}[r.id], -1, -1)
		o.cell = func(x, y int) (uint32, int, int, byte, uint16) {
			v := d.get(x, y)
			return uint32(cellCh(v)), cellFg(v) % r.colours, cellBg(v) % r.colours, 0, 0
		}
	default:
		return nil, false
	}
	return o.encode(preview), true
}

// ---------------------------------------------------------------- in CharEdit

func init() {
	for i, f := range formats {
		if f.id == "icy" {
			formats[i].modes += " c64 atari st"
		}
	}
}

// icyLoad states: how loadEditor goes on with an IcyDraw file.
const (
	icyPC      = iota // a PC page: read as before
	icyGoOn           // a machine's page, set up: finish loading
	icyStop           // handled (a message, or a download asked first)
	icyNotThis        // Open as another page type: not this reader
)

// icyMachineLoad sets the tab up for an IcyDraw file of another machine.
func (a *App) icyMachineLoad(b []byte, name, path, as string) (state int, note string) {
	doc, err := parseIcyDraw(b)
	if err != nil || !icyMachineBuffer(doc.buffer) {
		return icyPC, ""
	}
	m := icyMachinePage(doc, a.gtia())
	if as != "auto" && as != m.mode {
		return icyNotThis, ""
	}
	again := func() func() {
		t := a.Tab
		return func() {
			if a.backTo(t) {
				a.loadBytes(b, name, path, as)
			}
		}
	}
	switch {
	case m.mode == "c64" && !a.hasC64:
		openC64Req(again())
		return icyStop, ""
	case m.mode == "c64" && !hasROM(machineByID(m.machine).family):
		a.msg("Could not open " + name + ": the " + machineByID(m.machine).name + "'s character set is not here (Settings downloads it)")
		return icyStop, ""
	case m.mode == "st" && !hasSTFont():
		openSTReq(again())
		return icyStop, ""
	}
	a.srcKind, a.ownFont, a.ownLower, a.ownPal = "", nil, nil, nil
	a.mode, a.ice = m.mode, true
	switch m.mode {
	case "c64":
		a.screenMachine(m.machine)
	case "st":
		a.stres = m.stres
	}
	return icyGoOn, strings.Join(m.notes, "; ")
}

// reparseIcyMachine reads the page again from its IcyDraw file of another machine;
// false if the file is a PC page or the page is of another type now.
func (a *App) reparseIcyMachine() bool {
	doc, err := parseIcyDraw(a.source)
	if err != nil {
		return false
	}
	m := icyMachinePage(doc, a.gtia())
	if m == nil || m.mode != a.mode {
		return false
	}
	d := m.doc
	switch m.mode {
	case "c64":
		if a.machine != m.machine { // the page went to another machine: its width
			d = d.withCols(a.mach().cols)
		}
		a.setCase(m.lower)
	case "st":
		if a.stres != m.stres {
			d = d.withCols(a.res().cols)
		}
	}
	a.doc = d
	return true
}

// icyAfterLoad gives a PETSCII page from IcyDraw the file's character set when it is not the ROM's.
func (a *App) icyAfterLoad() {
	doc, err := parseIcyDraw(a.source)
	if err != nil || !a.c64() {
		return
	}
	m := icyMachinePage(doc, a.gtia())
	if m == nil || m.font == nil {
		return
	}
	set := newFont(ownID, strings.TrimSuffix(a.name, ".icy")+" (IcyDraw's set)", "c64", m.font, 8, 256)
	set.format = "icy"
	if m.lower {
		up := fontByID(a.mach().family + "upper")
		if up == nil {
			return
		}
		a.useOwnFont(up.clone(), set, false)
		a.fontFor["c64"] = ownLowerID
		return
	}
	a.useOwnFont(set, nil, false)
}
