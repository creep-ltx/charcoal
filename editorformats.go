package main

// The files of other text-art editors, so a page can go to the program an artist
// or a friend uses and come back: iCEDraw (.idf), ArtWorx (.adf), TundraDraw
// (.tnd), IcyDraw (.icy), REXPaint (.xp), Playscii (.psci), Marq's PETSCII editor
// (.pet, and its .bas and .s exports) and Petmate's JSON export (.json).
//
// Each writer was checked against the reader of the program it is for (or the
// readers that show its files: ansilove, IcyDraw); the traps are in the comments
// and in ~/Projects/Knowledge/amiga/ansi-art-real-world-traps.md.

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"math"
	"strings"

	"github.com/klauspost/compress/zstd"
)

// ---------------------------------------------------------------- shared

// font4096 is a PC font as 256 glyphs of 16 rows (an 8-row font doubled, as CharEdit keeps it).
func font4096(f *Font) []byte {
	out := make([]byte, 4096)
	if f == nil {
		f = fontByID("vga")
	}
	if f != nil {
		copy(out, f.bits)
	}
	return out
}

// sixBit is a colour as three 0-63 values, as VGA's DAC takes them.
func sixBit(c uint32) []byte {
	return []byte{byte(c>>16&255) >> 2, byte(c>>8&255) >> 2, byte(c&255) >> 2}
}

// shownBg is the background a cell shows: on a page without iCE colours a
// background of 8-15 is blinking text on 0-7.
func shownBg(v uint32, ice bool) int {
	if ice {
		return cellBg(v)
	}
	return cellBg(v) & 7
}

// rgbCell is a cell of a format that stores colours, not palette numbers.
type rgbCell struct {
	ch     byte
	fg, bg uint32
}

// rgbColours numbers the 24-bit colours of cells: own is their palette when they
// use 16 colours or fewer, else nil and each is the nearest of the VGA's.
//
// A colour that is one of the VGA's keeps the VGA's number (the rest take the free
// numbers in the order they first appear), so a page CharEdit wrote with the VGA
// colours comes back with the same colour numbers even when the file it came from
// was read with other numbers (a .tnd of more than 16 colours, matched to the VGA).
func rgbColours(cells []rgbCell) (pick func(uint32) int, own *[16]uint32) {
	vga := paletteByID("vga").colors
	var seen []uint32
	index := map[uint32]int{}
	for _, c := range cells {
		for _, col := range []uint32{c.fg & 0xFFFFFF, c.bg & 0xFFFFFF} {
			if _, ok := index[col]; !ok && len(seen) <= 16 {
				index[col] = -1
				seen = append(seen, col)
			}
		}
	}
	if len(seen) > 16 {
		return func(col uint32) int { return nearestOf(col, vga) }, nil
	}
	var pal [16]uint32
	taken := [16]bool{}
	for _, col := range seen {
		for i, v := range vga {
			if v&0xFFFFFF == col && !taken[i] {
				index[col], taken[i] = i, true
				break
			}
		}
	}
	free := 0
	for _, col := range seen {
		if index[col] >= 0 {
			continue
		}
		for taken[free] {
			free++
		}
		index[col], taken[free] = free, true
	}
	for i := range pal {
		pal[i] = 0xFF000000 | vga[i]
	}
	for col, i := range index {
		pal[i] = 0xFF000000 | col
	}
	return func(col uint32) int { return index[col&0xFFFFFF] }, &pal
}

// rgbDoc makes a page from cells with 24-bit colours (rgbColours); the palette is
// the VGA's when they use more than 16.
func rgbDoc(cols, rows int, cells []rgbCell) (*Doc, *[16]uint32) {
	pick, pal := rgbColours(cells)
	if pal == nil {
		vga := paletteByID("vga").colors
		pal = &vga
	}
	d := newDoc(cols, max(1, rows))
	for i, c := range cells {
		if i >= len(d.cells) {
			break
		}
		d.cells[i] = makeCell(int(c.ch), pick(c.fg), pick(c.bg), 0)
	}
	return d, pal
}

// ---------------------------------------------------------------- iCEDraw

// writeIDF gives an iCEDraw 1.4 file: x1, y1, x2, y2, the cells (character,
// attribute) with runs of the same cell as 01 00 count 00 char attr, then the
// font (8x16) and the palette (16 colours of 0-63, in BIOS order).
//
// As IcyDraw writes it: runs stay within a row, a run is used for 4 or more cells,
// and a character 1 always goes as a run (ansilove takes any 1 for the start of
// one). Counts stop at 255: ansilove and CharEdit read only the low byte.
func writeIDF(d *Doc, pal [16]uint32, f *Font) []byte {
	cols, rows := d.cols, max(1, d.rows)
	out := []byte("\x041.4")
	for _, v := range []int{0, 0, cols - 1, rows - 1} {
		out = append(out, byte(v), byte(v>>8))
	}
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; {
			v := d.get(x, y)
			ch, at := byte(cellCh(v)), cellAttr(v)
			n := 1
			for x+n < cols && n < 255 {
				w := d.get(x+n, y)
				if byte(cellCh(w)) != ch || cellAttr(w) != at {
					break
				}
				n++
			}
			if n > 3 || ch == 1 {
				out = append(out, 1, 0, byte(n), 0, ch, at)
			} else {
				n = 1
				out = append(out, ch, at)
			}
			x += n
		}
	}
	out = append(out, font4096(f)...)
	for _, c := range biosPalette(pal) {
		out = append(out, sixBit(c)...)
	}
	return out
}

// ---------------------------------------------------------------- ArtWorx

// egaPalette64 is the EGA's 64 colours (rgbRGB bits), 0-63 each.
func egaPalette64() []byte {
	out := make([]byte, 0, 192)
	for i := 0; i < 64; i++ {
		lvl := func(hi, lo int) byte { return byte(42*(i>>hi&1) + 21*(i>>lo&1)) }
		out = append(out, lvl(2, 5), lvl(1, 4), lvl(0, 3))
	}
	return out
}

// writeADF gives an ArtWorx file: version 1, the EGA's 64 colours with the 16 the
// text uses at their places (adfColours), the font (8x16), then 80-column cells.
func writeADF(d *Doc, pal [16]uint32, f *Font) ([]byte, error) {
	if d.cols != 80 {
		return nil, fmt.Errorf("an ArtWorx file is 80 columns wide (this page is %d)", d.cols)
	}
	out := []byte{1}
	ega := egaPalette64()
	for i, c := range biosPalette(pal) {
		copy(ega[adfColours[i]*3:], sixBit(c))
	}
	out = append(append(out, ega...), font4096(f)...)
	for y := 0; y < max(1, d.rows); y++ {
		for x := 0; x < 80; x++ {
			v := d.get(x, y)
			out = append(out, byte(cellCh(v)), cellAttr(v))
		}
	}
	return out, nil
}

// ---------------------------------------------------------------- TundraDraw

// writeTundra gives a TundraDraw file: "\x18TUNDRA24", then the cells of 80-column
// rows; a cell whose colours change is 2 (foreground), 4 (background) or 6 (both),
// its character, and 24-bit colours as 4 bytes (0, r, g, b).
//
// The first cell always says both colours: ansilove starts at black on black,
// IcyDraw and CharEdit at grey on black. Characters 1-6 always go inside a colour
// command (IcyDraw takes 1-6 as commands, ansilove 1, 2, 4 and 6); ansilove does
// not draw characters 1, 2, 4 and 6 even there (its reader's limit).
func writeTundra(d *Doc, pal [16]uint32, ice bool) ([]byte, error) {
	if d.cols != 80 {
		return nil, fmt.Errorf("a TundraDraw file is 80 columns wide (this page is %d)", d.cols)
	}
	out := append([]byte{24}, "TUNDRA24"...)
	fg, bg := uint32(1<<31), uint32(1<<31) // none yet
	col := func(c uint32) []byte { return []byte{0, byte(c >> 16), byte(c >> 8), byte(c)} }
	for y := 0; y < max(1, d.rows); y++ {
		for x := 0; x < 80; x++ {
			v := d.get(x, y)
			ch := byte(cellCh(v))
			f, b := pal[cellFg(v)]&0xFFFFFF, pal[shownBg(v, ice)]&0xFFFFFF
			cmd := byte(0)
			if f != fg || (ch >= 1 && ch <= 6 && b == bg) {
				cmd |= 2
			}
			if b != bg {
				cmd |= 4
			}
			if cmd == 0 {
				out = append(out, ch)
			} else {
				out = append(out, cmd, ch)
			}
			if cmd&2 != 0 {
				out = append(out, col(f)...)
			}
			if cmd&4 != 0 {
				out = append(out, col(b)...)
			}
			fg, bg = f, b
		}
	}
	return out, nil
}

// ---------------------------------------------------------------- REXPaint

// xpTransparent is REXPaint's background of an empty cell on a layer above the first.
const xpTransparent = 0xFF00FF

// writeXP gives a REXPaint image: gzip of version -1, one layer, its width and
// height, then the cells column by column: glyph (int32, CP437), foreground and
// background as r, g, b.
func writeXP(d *Doc, pal [16]uint32, ice bool) []byte {
	rows := max(1, d.rows)
	var raw bytes.Buffer
	le := func(v int32) { binary.Write(&raw, binary.LittleEndian, v) }
	le(-1)
	le(1)
	le(int32(d.cols))
	le(int32(rows))
	for x := 0; x < d.cols; x++ {
		for y := 0; y < rows; y++ {
			v := d.get(x, y)
			le(int32(cellCh(v)))
			f, b := pal[cellFg(v)], pal[shownBg(v, ice)]
			raw.Write([]byte{byte(f >> 16), byte(f >> 8), byte(f), byte(b >> 16), byte(b >> 8), byte(b)})
		}
	}
	var out bytes.Buffer
	zw := gzip.NewWriter(&out)
	zw.Write(raw.Bytes())
	zw.Close()
	return out.Bytes()
}

func isXP(name string, b []byte) bool {
	return strings.HasSuffix(strings.ToLower(name), ".xp") && len(b) > 2 && b[0] == 0x1f && b[1] == 0x8b
}

// parseXP reads a REXPaint image; its layers are put together (a cell of a layer
// above shows unless its background is REXPaint's transparent pink).
func parseXP(b []byte) (*Doc, XBinInfo, error) {
	var info XBinInfo
	zr, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return nil, info, fmt.Errorf("not a REXPaint image: %v", err)
	}
	raw, err := io.ReadAll(io.LimitReader(zr, 64<<20)) // 6.7M cells at most
	if err != nil && len(raw) == 0 {
		return nil, info, fmt.Errorf("the REXPaint image is damaged: %v", err)
	}
	i32 := func(o int) int {
		if o+4 > len(raw) {
			return -1 << 30
		}
		return int(int32(binary.LittleEndian.Uint32(raw[o:])))
	}
	n := i32(4)
	if n < 1 || n > 16 {
		return nil, info, fmt.Errorf("not a REXPaint image (%d layers)", n)
	}
	type layer struct {
		w, h int
		data []byte
	}
	var layers []layer
	o, w, h := 8, 0, 0
	for k := 0; k < n; k++ {
		lw, lh := i32(o), i32(o+4)
		if lw < 1 || lh < 1 || lw > 4096 || lh > 4096 || lw*lh > maxPageCells {
			return nil, info, fmt.Errorf("a REXPaint layer of %dx%d", lw, lh)
		}
		o += 8
		size := lw * lh * 10
		if o+size > len(raw) {
			return nil, info, fmt.Errorf("the REXPaint image is cut short")
		}
		layers = append(layers, layer{lw, lh, raw[o : o+size]})
		o += size
		w, h = max(w, lw), max(h, lh)
	}
	h = min(h, maxRowsFor(w))
	cells := make([]rgbCell, w*h)
	for i := range cells {
		cells[i] = rgbCell{' ', 0xAAAAAA, 0}
	}
	for k, l := range layers {
		for i := 0; i+10 <= len(l.data); i += 10 {
			x, y := i/10/l.h, i/10%l.h
			if y >= h {
				continue
			}
			c := l.data[i:]
			fg := uint32(c[4])<<16 | uint32(c[5])<<8 | uint32(c[6])
			bg := uint32(c[7])<<16 | uint32(c[8])<<8 | uint32(c[9])
			if bg == xpTransparent {
				if k > 0 {
					continue
				}
				bg = 0
			}
			cells[y*w+x] = rgbCell{byte(min(255, binary.LittleEndian.Uint32(c))), fg, bg}
		}
	}
	d, pal := rgbDoc(w, h, cells)
	info.pal, info.ice, info.cellH = pal, true, 16
	return d, info, nil
}

// ---------------------------------------------------------------- IcyDraw

// IcyDraw's own file (icy_tools, crates/icy_engine/src/formats/io/icy_draw.rs):
// a PNG (a picture of the first 80 rows) whose "icYD" chunks hold records:
// version 1, keyword (u16 length + text), data (u32 length + bytes). The first is
// ICED (version, compression 0 none / 2 zstd, buffer type, iCE mode, font mode,
// width, height, font size); then PALETTE (text, "ICE Palette"), FONT (slot, name,
// PSF2), LAYER (title, mode, colour, flags, offset, size, cells), SAUCE, END.
// Files from IcyDraw 0.4 and before (version 0) keep base64 records in tEXt/zTXt
// chunks instead; CharEdit reads both and writes version 1, uncompressed.

const (
	icyCP437     = 1 // buffer types
	icyBlink     = 1 // iCE modes: 0 unlimited, 1 blink, 2 iCE
	icyIce       = 2
	icyBold      = 1 << 0 // attribute bits
	icyBlinkAttr = 1 << 3
	icyInvisible = 1 << 15
)

func icyRecord(keyword string, data []byte) []byte {
	out := []byte{1, byte(len(keyword)), byte(len(keyword) >> 8)}
	out = append(out, keyword...)
	out = binary.LittleEndian.AppendUint32(out, uint32(len(data)))
	return append(out, data...)
}

func icyString(s string) []byte {
	return append(binary.LittleEndian.AppendUint32(nil, uint32(len(s))), s...)
}

// writeIcy gives an IcyDraw document of one layer, with the page's font and
// palette; preview is the picture the PNG shows (nil: one transparent pixel).
//
// No SAUCE record goes in: IcyDraw refuses the whole file when it cannot read one.
func writeIcy(d *Doc, pal [16]uint32, f *Font, ice bool, preview image.Image) []byte {
	rows := max(1, d.rows)
	if f == nil {
		f = fontByID("vga")
	}
	head := binary.LittleEndian.AppendUint16(nil, 1)
	head = append(head, 0, 0, 0) // no compression, reserved
	head = binary.LittleEndian.AppendUint16(head, icyCP437)
	head = append(head, map[bool]byte{true: icyIce, false: icyBlink}[ice], 2) // font mode: single
	head = binary.LittleEndian.AppendUint32(head, uint32(d.cols))
	head = binary.LittleEndian.AppendUint32(head, uint32(rows))
	head = append(head, 8, byte(f.rows()))
	recs := [][]byte{icyRecord("ICED", head)}
	var p strings.Builder
	p.WriteString("ICE Palette\n#Palette Name: \n#Author: \n#Description: \n#Colors: 16\n")
	for _, c := range biosPalette(pal) {
		fmt.Fprintf(&p, "%02x%02x%02x\n", c>>16&255, c>>8&255, c&255)
	}
	recs = append(recs, icyRecord("PALETTE", []byte(p.String())))
	font := []byte{0}
	font = append(font, icyString(f.name)...)
	h := f.rows()
	for _, v := range []uint32{0x864AB572, 0, 32, 0, 256, uint32(h), uint32(h), 8} {
		font = binary.LittleEndian.AppendUint32(font, v)
	}
	for c := 0; c < 256; c++ {
		for r := 0; r < h; r++ {
			font = append(font, f.row(c, r))
		}
	}
	recs = append(recs, icyRecord("FONT", font))
	layer := icyString("Background")
	layer = append(layer, 0, 0, 0, 0, 0)               // mode normal, no colour
	layer = binary.LittleEndian.AppendUint32(layer, 1) // visible
	layer = append(layer, 0, 0, 0, 0, 0, 0, 0, 0)      // offset
	layer = binary.LittleEndian.AppendUint32(layer, uint32(d.cols))
	layer = binary.LittleEndian.AppendUint32(layer, uint32(rows))
	for y := 0; y < rows; y++ {
		for x := 0; x < d.cols; x++ {
			v := d.get(x, y)
			at := cellAttr(v)
			fg, bg, flags := at&15, at>>4, uint16(0)
			if !ice && bg > 7 {
				bg, flags = bg&7, icyBlinkAttr
			}
			layer = binary.LittleEndian.AppendUint32(layer, uint32(cellCh(v)))
			layer = append(layer, 1+fg, 1+bg, 0, byte(flags), byte(flags>>8))
		}
	}
	recs = append(recs, icyRecord("LAYER", layer), icyRecord("END", nil))

	if preview == nil {
		preview = image.NewNRGBA(image.Rect(0, 0, 1, 1))
	}
	var pic bytes.Buffer
	png.Encode(&pic, preview)
	pb := pic.Bytes() // signature (8) and IHDR (25) first: the records go right after them
	var out bytes.Buffer
	out.Write(pb[:33])
	for _, r := range recs {
		pngChunk(&out, "icYD", r)
	}
	out.Write(pb[33:])
	return out.Bytes()
}

// isIcyDraw: is it an IcyDraw file (a PNG whose first chunk after IHDR is a record)?
func isIcyDraw(b []byte) bool {
	if len(b) < 45 || string(b[:8]) != "\x89PNG\r\n\x1a\n" || string(b[12:16]) != "IHDR" {
		return false
	}
	switch string(b[37:41]) {
	case "icYD":
		return true
	case "tEXt", "zTXt":
		return string(b[41:45]) == "ICED"
	}
	return false
}

// icyColour is a cell colour of an IcyDraw file: a palette number, an xterm number or RGB.
type icyColour struct {
	kind byte // 'p' palette, 'x' xterm 256, 'r' RGB, 't' transparent
	v    uint32
}

type icyCell struct {
	ch      rune
	fg, bg  icyColour
	attr    uint16
	visible bool
}

type icyLayer struct {
	mode       byte // 0 normal, 1 characters only, 2 colours only
	visible    bool
	alpha      bool
	group      bool
	x, y, w, h int
	cells      []icyCell
}

type icyDoc struct {
	version int
	buffer  int
	ice     int
	w, h    int
	fontH   int
	pal     []uint32
	font    *Font
	layers  []*icyLayer
	sauce   []byte
	skipped []string
}

// icyMaxCells: CharEdit opens IcyDraw pages (and layers) of up to 4M cells (80 x 52428):
// a page is put together from its layers in memory, and a packed file can say any size.
const icyMaxCells = 4 << 20

func icyLimit(w, h int) bool {
	return w > 0 && h > 0 && w <= 65535 && h <= maxPageRows && w*h <= icyMaxCells
}

func icyColourV1(b []byte) (icyColour, []byte, error) {
	if len(b) == 0 {
		return icyColour{}, nil, errors.New("cut short")
	}
	switch t := b[0]; {
	case t == 0:
		return icyColour{kind: 't'}, b[1:], nil
	case t <= 16:
		return icyColour{'p', uint32(t - 1)}, b[1:], nil
	case t == 17 && len(b) >= 2:
		return icyColour{'x', uint32(b[1])}, b[2:], nil
	case t == 18 && len(b) >= 4:
		return icyColour{'r', uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])}, b[4:], nil
	case t == 19 && len(b) >= 2:
		return icyColour{'p', uint32(b[1])}, b[2:], nil
	}
	return icyColour{}, nil, fmt.Errorf("a colour of kind %d", b[0])
}

// icyColourV0 reads a colour of a version 0 file (ext: which of fg/bg are RGB or xterm).
func icyColourV0(v uint32, ext byte, fg bool) icyColour {
	rgbFlag, extFlag := byte(2), byte(8)
	if fg {
		rgbFlag, extFlag = 1, 4
	}
	switch {
	case v == 0x80000000:
		return icyColour{kind: 't'}
	case ext&rgbFlag != 0:
		return icyColour{'r', v & 0xFFFFFF}
	case ext&extFlag != 0:
		return icyColour{'x', v & 255}
	}
	return icyColour{'p', v & 255}
}

func icyStr(b []byte) (string, []byte, error) {
	if len(b) < 4 {
		return "", nil, errors.New("cut short")
	}
	n := binary.LittleEndian.Uint32(b)
	if uint64(n) > uint64(len(b)-4) {
		return "", nil, errors.New("cut short")
	}
	return string(b[4 : 4+n]), b[4+n:], nil
}

// icyFont reads an IcyDraw font record's font: PSF1/PSF2, or 256 glyphs of raw rows.
func icyFont(name string, b []byte) *Font {
	if isPSF1(b) || isPSF2(b) {
		if data, h, n, _, err := readPSF(b); err == nil && h >= 6 && h <= 16 {
			f := newFont(ownID, name, "pc", data, h, n)
			f.format = "icy"
			return f
		}
		return nil
	}
	if len(b) > 0 && len(b)%256 == 0 && len(b)/256 >= 6 && len(b)/256 <= 16 {
		f := newFont(ownID, name, "pc", b, len(b)/256, 256)
		f.format = "icy"
		return f
	}
	return nil
}

func icyPalette(text []byte) []uint32 {
	var out []uint32
	for i, line := range strings.Split(string(text), "\n") {
		line = strings.TrimSpace(line)
		if i == 0 || line == "" || strings.HasPrefix(line, "#") || len(line) < 6 {
			continue
		}
		var r, g, b uint32
		if _, err := fmt.Sscanf(line[:6], "%02x%02x%02x", &r, &g, &b); err == nil {
			out = append(out, r<<16|g<<8|b)
		}
		if len(out) >= 256 {
			break
		}
	}
	return out
}

// parseIcyDraw reads an IcyDraw file (version 1, or 0 from IcyDraw 0.4 and before).
func parseIcyDraw(b []byte) (*icyDoc, error) {
	doc := &icyDoc{version: -1, fontH: 16}
	var recs []struct {
		key  string
		data []byte
	}
	v0, unpacked := false, 0
	types, datas, err := pngChunks(b)
	if err != nil {
		return nil, err
	}
	for ci, typ := range types {
		data := datas[ci]
		switch typ {
		case "icYD":
			if len(data) < 7 || data[0] != 1 {
				return nil, fmt.Errorf("an IcyDraw record of an unknown version")
			}
			n := int(binary.LittleEndian.Uint16(data[1:]))
			if 3+n+4 > len(data) {
				return nil, fmt.Errorf("an IcyDraw record is cut short")
			}
			key := string(data[3 : 3+n])
			m := int(binary.LittleEndian.Uint32(data[3+n:]))
			if m < 0 || m > len(data)-3-n-4 {
				return nil, fmt.Errorf("an IcyDraw record is cut short")
			}
			recs = append(recs, struct {
				key  string
				data []byte
			}{key, data[3+n+4 : 3+n+4+m]})
		case "tEXt", "zTXt":
			k := bytes.IndexByte(data, 0)
			if k < 0 {
				continue
			}
			key, text := string(data[:k]), data[k+1:]
			if typ == "zTXt" {
				if len(text) < 1 {
					continue
				}
				zr, err := zlib.NewReader(bytes.NewReader(text[1:]))
				if err != nil {
					continue
				}
				if text, err = io.ReadAll(io.LimitReader(zr, int64(64<<20-unpacked))); err != nil {
					continue
				}
				if unpacked += len(text); unpacked >= 64<<20 {
					return nil, fmt.Errorf("an IcyDraw file of more than 64 MB unpacked")
				}
			}
			raw, err := base64.StdEncoding.DecodeString(string(text))
			if err != nil {
				continue
			}
			v0 = true
			recs = append(recs, struct {
				key  string
				data []byte
			}{key, raw})
		}
		if typ == "IEND" {
			break
		}
	}
	if len(recs) == 0 || recs[0].key != "ICED" {
		return nil, fmt.Errorf("not an IcyDraw file")
	}
	h := recs[0].data
	if len(h) < 19 {
		return nil, fmt.Errorf("the IcyDraw header is cut short")
	}
	doc.version = int(binary.LittleEndian.Uint16(h))
	compression := byte(0)
	if v0 || doc.version == 0 {
		if doc.version != 0 {
			return nil, fmt.Errorf("an IcyDraw file of version %d in text chunks", doc.version)
		}
		doc.buffer = int(binary.LittleEndian.Uint16(h[6:]))
		doc.ice = int(h[8])
		doc.w, doc.h = int(binary.LittleEndian.Uint32(h[11:])), int(binary.LittleEndian.Uint32(h[15:]))
	} else {
		if doc.version < 1 || doc.version > 5 {
			return nil, fmt.Errorf("an IcyDraw file of version %d (CharEdit reads 0-5)", doc.version)
		}
		compression = h[2]
		doc.buffer = int(binary.LittleEndian.Uint16(h[5:]))
		doc.ice = int(h[7])
		doc.w, doc.h = int(binary.LittleEndian.Uint32(h[9:])), int(binary.LittleEndian.Uint32(h[13:]))
		doc.fontH = int(h[18])
	}
	if !icyLimit(doc.w, doc.h) {
		return nil, fmt.Errorf("an IcyDraw page of %dx%d", doc.w, doc.h)
	}
	if doc.buffer != icyCP437 && doc.buffer != 0 {
		what := map[int]string{2: "PETSCII", 3: "ATASCII", 4: "Viewdata", 5: "Atari ST"}[doc.buffer]
		return nil, fmt.Errorf("an IcyDraw %s page: CharEdit reads IcyDraw's PC (CP437) pages only", what)
	}
	var zdec *zstd.Decoder
	if compression == 2 {
		zdec, err = zstd.NewReader(nil, zstd.WithDecoderMaxMemory(64<<20), zstd.WithDecoderConcurrency(1))
		if err != nil {
			return nil, err
		}
		defer zdec.Close()
	} else if compression != 0 {
		return nil, fmt.Errorf("IcyDraw compression %d is unknown", compression)
	}
	layerAt := map[int]*icyLayer{} // version 0: LAYER_n and its continuations LAYER_n~k
	resume := map[int]int{}
	cells := 0 // in all layers
	tooMany := func(l *icyLayer) error {
		if cells += l.w * l.h; cells > icyMaxCells {
			return fmt.Errorf("an IcyDraw file of more than %d cells in its layers", icyMaxCells)
		}
		return nil
	}
	for _, r := range recs[1:] {
		data := r.data
		if zdec != nil && r.key != "END" && r.key != "SIXEL" {
			if data, err = zdec.DecodeAll(r.data, nil); err != nil {
				return nil, fmt.Errorf("an IcyDraw record cannot be unpacked: %v", err)
			}
			if unpacked += len(data); unpacked > 256<<20 {
				return nil, fmt.Errorf("an IcyDraw file of more than 256 MB unpacked")
			}
		}
		switch {
		case r.key == "END":
			return doc, nil
		case r.key == "PALETTE":
			doc.pal = icyPalette(data)
		case r.key == "SAUCE":
			doc.sauce = data
		case r.key == "FONT" && !v0:
			if len(data) > 1 && data[0] == 0 && doc.font == nil {
				if name, rest, err := icyStr(data[1:]); err == nil {
					doc.font = icyFont(name, rest)
				}
			}
		case strings.HasPrefix(r.key, "FONT_") && v0:
			if r.key == "FONT_0" {
				if name, rest, err := icyStr(data); err == nil {
					doc.font = icyFont(name, rest)
					if doc.font != nil {
						doc.fontH = doc.font.rows()
					}
				}
			}
		case r.key == "LAYER" && !v0:
			l, err := icyLayerV1(data, icyMaxCells-cells)
			if err != nil {
				return nil, err
			}
			if err := tooMany(l); err != nil {
				return nil, err
			}
			doc.layers = append(doc.layers, l)
		case r.key == "GROUP" && !v0:
			if len(doc.layers) > 0 && len(data) == 18 && binary.LittleEndian.Uint64(data[9:]) != 0 {
				doc.layers[len(doc.layers)-1].group = true // a group's marker, not cells
			}
		case strings.HasPrefix(r.key, "LAYER_") && v0:
			var n, k int
			if c, _ := fmt.Sscanf(r.key, "LAYER_%d~%d", &n, &k); c == 2 {
				if l := layerAt[n]; l != nil {
					resume[n] = icyCellsV0(l, data, resume[n])
				}
				continue
			}
			if _, err := fmt.Sscanf(r.key, "LAYER_%d", &n); err != nil || n != len(layerAt) {
				return nil, fmt.Errorf("an IcyDraw layer out of order (%s)", r.key)
			}
			l, rest, err := icyLayerV0(data, icyMaxCells-cells)
			if err != nil {
				return nil, err
			}
			layerAt[n] = l
			if l != nil {
				if err := tooMany(l); err != nil {
					return nil, err
				}
				resume[n] = icyCellsV0(l, rest, 0)
				doc.layers = append(doc.layers, l)
			} else {
				layerAt[n] = &icyLayer{} // a picture layer: skipped
				doc.skipped = append(doc.skipped, "a picture layer")
			}
		case r.key == "SIXEL":
			doc.skipped = append(doc.skipped, "a picture layer")
		case r.key == "EFFECTS":
			doc.skipped = append(doc.skipped, "layer effects")
		}
	}
	return doc, nil
}

func icyLayerV1(b []byte, room int) (*icyLayer, error) {
	_, b, err := icyStr(b)
	if err != nil || len(b) < 25 {
		return nil, fmt.Errorf("an IcyDraw layer is cut short")
	}
	l := &icyLayer{mode: b[0]}
	flags := binary.LittleEndian.Uint32(b[5:])
	l.visible, l.alpha = flags&1 != 0, flags&8 != 0
	l.x, l.y = int(int32(binary.LittleEndian.Uint32(b[9:]))), int(int32(binary.LittleEndian.Uint32(b[13:])))
	l.w, l.h = int(int32(binary.LittleEndian.Uint32(b[17:]))), int(int32(binary.LittleEndian.Uint32(b[21:])))
	b = b[25:]
	if !icyLimit(l.w, l.h) || l.w*l.h > room || len(b) < 9*l.w*l.h {
		return nil, fmt.Errorf("an IcyDraw layer of %dx%d with %d bytes", l.w, l.h, len(b))
	}
	l.cells = make([]icyCell, l.w*l.h)
	for i := range l.cells {
		if len(b) < 4 {
			return nil, fmt.Errorf("an IcyDraw layer is cut short")
		}
		c := icyCell{ch: rune(binary.LittleEndian.Uint32(b))}
		if c.fg, b, err = icyColourV1(b[4:]); err == nil {
			c.bg, b, err = icyColourV1(b)
		}
		if err != nil || len(b) < 3 {
			return nil, fmt.Errorf("an IcyDraw layer is cut short")
		}
		c.attr = binary.LittleEndian.Uint16(b[1:])
		c.visible = c.attr&icyInvisible == 0
		b = b[3:]
		l.cells[i] = c
	}
	return l, nil
}

// icyLayerV0 reads a version 0 layer's head; nil for a picture layer.
func icyLayerV0(b []byte, room int) (*icyLayer, []byte, error) {
	_, b, err := icyStr(b)
	if err != nil || len(b) < 1+4+1+4+4+1+8+8+2+8 {
		return nil, nil, fmt.Errorf("an IcyDraw layer is cut short")
	}
	if b[0] == 1 {
		return nil, nil, nil
	}
	l := &icyLayer{mode: b[5]}
	flags := binary.LittleEndian.Uint32(b[10:])
	l.visible, l.alpha = flags&1 != 0, flags&8 != 0
	l.x, l.y = int(int32(binary.LittleEndian.Uint32(b[15:]))), int(int32(binary.LittleEndian.Uint32(b[19:])))
	l.w, l.h = int(int32(binary.LittleEndian.Uint32(b[23:]))), int(int32(binary.LittleEndian.Uint32(b[27:])))
	if !icyLimit(l.w, l.h) || l.w*l.h > room {
		return nil, nil, fmt.Errorf("an IcyDraw layer of %dx%d", l.w, l.h)
	}
	l.cells = make([]icyCell, l.w*l.h)
	return l, b[1+4+1+4+4+1+8+8+2+8:], nil
}

// icyCellsV0 reads version 0 rows into l from row y on; it gives the row to go on from.
func icyCellsV0(l *icyLayer, b []byte, y int) int {
	o := 0
	for ; y < l.h && o < len(b); y++ {
		for x := 0; x < l.w; x++ {
			if o+2 > len(b) {
				return l.h
			}
			raw := binary.LittleEndian.Uint16(b[o:])
			o += 2
			if raw == 0xC000 {
				break
			}
			short := raw&(0x0800|0x4000) != 0
			attr := raw &^ (0x0800 | 0x4000)
			if attr == 0x8000 {
				continue
			}
			c := icyCell{attr: attr, visible: true}
			if short {
				if o+4 > len(b) {
					return l.h
				}
				c.ch, c.fg, c.bg = rune(b[o]), icyColour{'p', uint32(b[o+1])}, icyColour{'p', uint32(b[o+2])}
				o += 4
			} else {
				if o+14 > len(b) {
					return l.h
				}
				ext := b[o+13]
				c.ch = rune(binary.LittleEndian.Uint32(b[o:]))
				c.fg = icyColourV0(binary.LittleEndian.Uint32(b[o+4:]), ext, true)
				c.bg = icyColourV0(binary.LittleEndian.Uint32(b[o+8:]), ext, false)
				o += 14
			}
			l.cells[y*l.w+x] = c
		}
	}
	return y
}

// icyPage puts an IcyDraw file's visible layers together (later layers on top,
// as IcyDraw shows them) into one page; colours outside the first 16 go to the
// nearest of them.
func icyPage(doc *icyDoc) (*Doc, XBinInfo, string) {
	var info XBinInfo
	pal := doc.pal
	if len(pal) < 16 {
		vga := biosPalette(paletteByID("vga").colors)
		for i := len(pal); i < 16; i++ {
			pal = append(pal, vga[i]&0xFFFFFF)
		}
	}
	var p16 [16]uint32
	for i := range p16 {
		p16[i] = 0xFF000000 | pal[i]
	}
	cellPal := biosPalette(p16)
	info.pal = &cellPal
	info.ice = doc.ice != icyBlink
	info.font, info.cellH = doc.font, doc.fontH
	if info.font != nil {
		info.cellH = info.font.rows()
	}
	// colour gives a cell colour (ANSI order) of an IcyDraw colour.
	colour := func(c icyColour) int {
		var rgbv uint32
		switch c.kind {
		case 'p':
			if c.v < 16 {
				return biosANSI[c.v]
			}
			if int(c.v) < len(pal) {
				rgbv = pal[c.v]
			}
		case 'x':
			rgbv = xterm256(int(c.v))
		case 'r':
			rgbv = c.v
		}
		return nearestOf(rgbv, cellPal)
	}
	d := newDoc(doc.w, doc.h)
	type merged struct {
		ch     rune
		fg, bg icyColour
		seen   bool
	}
	cells := make([]merged, doc.w*doc.h)
	for _, l := range doc.layers {
		if l == nil || !l.visible || l.group || l.w == 0 {
			continue
		}
		for ly := 0; ly < l.h; ly++ {
			y := ly + l.y
			if y < 0 || y >= doc.h {
				continue
			}
			for lx := 0; lx < l.w; lx++ {
				x := lx + l.x
				if x < 0 || x >= doc.w {
					continue
				}
				c, m := l.cells[ly*l.w+lx], &cells[y*doc.w+x]
				switch l.mode {
				case 1: // characters only
					if c.visible && c.ch != ' ' && c.ch != 0 {
						m.ch = c.ch
					}
				case 2: // colours only
					if c.visible {
						m.fg, m.bg = c.fg, c.bg
					}
				default:
					under := *m
					if c.visible {
						m.ch, m.fg, m.bg, m.seen = c.ch, c.fg, c.bg, true
						if c.attr&icyBold != 0 && c.fg.kind == 'p' && c.fg.v < 8 {
							m.fg.v += 8
						}
						if c.attr&icyBlinkAttr != 0 && c.bg.kind == 'p' && c.bg.v < 8 && !info.ice {
							m.bg.v += 8
						}
					}
					if m.fg.kind == 't' {
						m.fg = under.fg
					}
					if m.bg.kind == 't' {
						m.bg = under.bg
					}
				}
			}
		}
	}
	for i, m := range cells {
		if !m.seen {
			continue
		}
		ch := m.ch
		if doc.buffer == 0 && ch > 255 { // a Unicode page: CP437 where it can be
			if c, ok := cp437Rev[ch]; ok {
				ch = rune(c)
			} else {
				ch = '?'
			}
		}
		fg, bg := 7, 0
		if m.fg.kind != 't' && m.fg.kind != 0 {
			fg = colour(m.fg)
		}
		if m.bg.kind != 't' && m.bg.kind != 0 {
			bg = colour(m.bg)
		}
		d.cells[i] = makeCell(int(ch&255), fg, bg, 0)
	}
	note := ""
	if n := len(doc.layers); n > 1 {
		note = fmt.Sprintf("its %d layers are put together into one page", n)
	}
	if len(doc.skipped) > 0 {
		if note != "" {
			note += "; "
		}
		note += "left out: " + strings.Join(uniq(doc.skipped), ", ")
	}
	return d, info, note
}

func uniq(s []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, v := range s {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

func parseIcy(b []byte) (*Doc, XBinInfo, error) {
	doc, err := parseIcyDraw(b)
	if err != nil {
		return nil, XBinInfo{}, err
	}
	d, info, _ := icyPage(doc)
	return d, info, nil
}

// ---------------------------------------------------------------- Marq's PETSCII editor: .pet

// writeMarqPet gives the .pet file Marq's PETSCII editor writes for a C64 screen
// (E): width, height, border, background, 1 for the lower case set, then the
// screen codes and the colours. Petmate's .pet export says 0 for lower case and 1
// for upper: the other way round.
func writeMarqPet(d *Doc, lower bool) []byte {
	w, h := min(255, d.cols), min(255, max(1, d.rows))
	out := []byte{byte(w), byte(h), byte(d.borderColour()), byte(d.bg), map[bool]byte{true: 1}[lower]}
	for part := 0; part < 2; part++ {
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				v := d.get(x, y)
				if part == 0 {
					out = append(out, byte(cellCh(v)))
				} else {
					out = append(out, byte(cellFg(v)))
				}
			}
		}
	}
	return out
}

func isMarqPet(name string, b []byte) bool {
	return strings.HasSuffix(strings.ToLower(name), ".pet") && len(b) >= 7 && b[0] > 0 && b[1] > 0 &&
		len(b) == 5+2*int(b[0])*int(b[1]) && b[4] <= 1
}

func parseMarqPet(b []byte) (*Doc, int, error) {
	if len(b) < 5 || len(b) != 5+2*int(b[0])*int(b[1]) || b[0] == 0 || b[1] == 0 {
		return nil, -1, fmt.Errorf("not a .pet screen")
	}
	w, h := int(b[0]), int(b[1])
	d := newDoc(w, h)
	d.border, d.bg = int(b[2]&15), int(b[3]&15)
	for i := 0; i < w*h; i++ {
		d.cells[i] = makeCell(int(b[5+i]), int(b[5+w*h+i]&15), 0, 0)
	}
	return d, int(b[4] & 1), nil
}

// ---------------------------------------------------------------- Marq's .bas and .s

// writeMarqBas gives the BASIC listing Marq's PETSCII editor exports (b) for a C64
// screen of 40x25: petcat makes a program of it.
func writeMarqBas(d *Doc, lower bool) []byte {
	var s strings.Builder
	s.WriteString("10 rem petcat -text -w2 -o export.prg export.bas\n20 print chr$(147)\n")
	fmt.Fprintf(&s, "30 poke 53280,%d\n", d.borderColour())
	if lower {
		s.WriteString("35 poke 53272,23\n")
	}
	fmt.Fprintf(&s, "40 poke 53281,%d\n", d.bg)
	s.WriteString("50 for i=0 to 999:read a:poke 1024+i,a:read a:poke 55296+i,a:next\n60 goto 60\n")
	chars, cols := c64Screen(d)
	line := 70
	for i := 0; i < 1000; i += 4 {
		fmt.Fprintf(&s, "%d data ", line)
		for j := 0; j < 4; j++ {
			fmt.Fprintf(&s, "%d,%d", chars[i+j], cols[i+j])
			if j != 3 {
				s.WriteString(",")
			}
		}
		s.WriteString("\n")
		line += 10
	}
	fmt.Fprintf(&s, "%d end\n", line)
	return []byte(s.String())
}

// marqAsmCode is Marq's copy loop for the self-contained ACME source (m_c64.pde, WTFPL).
const marqAsmCode = "\n\tldx\t#0\n\tldy\t#250\nkopy:\n\tlda\timg,x\n\tsta\t$400,x\n\tlda\timg+250,x\n\tsta\t$400+250,x\n" +
	"\tlda\timg+500,x\n\tsta\t$400+500,x\n\tlda\timg+750,x\n\tsta\t$400+750,x\n\n\tlda\timg+1000,x\n\tsta\t$d800,x\n" +
	"\tlda\timg+1250,x\n\tsta\t$d800+250,x\n\tlda\timg+1500,x\n\tsta\t$d800+500,x\n\tlda\timg+1750,x\n\tsta\t$d800+750,x\n\n" +
	"\tinx\n\tdey\n\tbne\tkopy\n\n\tlda\t#27\n\tsta\t$d011\n\njumi:\tjmp jumi\n"

// writeMarqAsm gives the ACME source Marq's PETSCII editor exports: a program that
// shows the screen (A) for 40x25, else the data alone (a): border, background,
// characters and colours.
func writeMarqAsm(d *Doc, lower bool) []byte {
	var s strings.Builder
	w, h := d.cols, max(1, d.rows)
	whole := w == 40 && h == 25
	if whole {
		s.WriteString("; Compile with: acme -o export.prg -f cbm export.s\n\n\t*=$801\n\t!byte $b,$08,$ef,$0,$9e,$32,$30,$36,$31,$0,$0,$0\n\n\tlda\t#11\n\tsta\t$d011\n")
		if lower {
			s.WriteString("\tlda\t#23\n\tsta\t$d018\n")
		}
		fmt.Fprintf(&s, "\tlda\t#%d\n\tsta\t$d020\n\tlda\t#%d\n\tsta\t$d021\n", d.borderColour(), d.bg)
		s.WriteString(marqAsmCode + "\nimg:\n")
	} else {
		fmt.Fprintf(&s, "; Border, bg, chars, colors\nimg:\n\t!byte %d,%d\n", d.borderColour(), d.bg)
	}
	for part, name := range []string{"; Character data\n", "; Color data\n"} {
		s.WriteString(name)
		for y := 0; y < h; y++ {
			s.WriteString("\t!byte ")
			for x := 0; x < w; x++ {
				v := d.get(x, y)
				if part == 0 {
					fmt.Fprint(&s, cellCh(v))
				} else {
					fmt.Fprint(&s, cellFg(v))
				}
				if x != w-1 {
					s.WriteString(",")
				}
			}
			s.WriteString("\n")
		}
	}
	return []byte(s.String())
}

// ---------------------------------------------------------------- Petmate's JSON export

type petmateJSONScreen struct {
	Width           int    `json:"width"`
	Height          int    `json:"height"`
	BackgroundColor int    `json:"backgroundColor"`
	BorderColor     int    `json:"borderColor"`
	Charset         string `json:"charset"`
	Name            string `json:"name,omitempty"`
	Screencodes     []int  `json:"screencodes"`
	Colors          []int  `json:"colors"`
}

type petmateJSONFile struct {
	Version   int                 `json:"version"`
	Framebufs []petmateJSONScreen `json:"framebufs"`
	Charsets  map[string]any      `json:"charsets"`
}

// writePetmateJSON gives the JSON Petmate exports (src/utils/exporters/json.ts), every screen.
func writePetmateJSON(scr []PetScreen) []byte {
	f := petmateJSONFile{Version: 1, Charsets: map[string]any{}}
	for _, s := range scr {
		d := s.doc
		j := petmateJSONScreen{Width: d.cols, Height: max(1, d.rows), BackgroundColor: d.bg, BorderColor: d.borderColour(),
			Charset: map[bool]string{true: "lower", false: "upper"}[s.lower == 1], Name: s.name}
		for i := 0; i < j.Width*j.Height; i++ {
			v := d.get(i%j.Width, i/j.Width)
			j.Screencodes = append(j.Screencodes, cellCh(v))
			j.Colors = append(j.Colors, cellFg(v))
		}
		f.Framebufs = append(f.Framebufs, j)
	}
	out, _ := json.Marshal(f)
	return out
}

func isPetmateJSON(b []byte) bool {
	t := bytes.TrimSpace(b[:min(len(b), 4096)])
	return len(t) > 0 && t[0] == '{' && bytes.Contains(t, []byte(`"framebufs"`)) && bytes.Contains(b, []byte(`"screencodes"`))
}

func parsePetmateJSON(b []byte) ([]PetScreen, error) {
	var f petmateJSONFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("not Petmate's JSON: %v", err)
	}
	var out []PetScreen
	for _, j := range f.Framebufs {
		if j.Width < 1 || j.Height < 1 || j.Width > 1000 || j.Height > maxRowsFor(j.Width) || j.Width*j.Height > len(j.Screencodes) {
			return nil, fmt.Errorf("a Petmate screen of %dx%d", j.Width, j.Height)
		}
		d := newDoc(j.Width, j.Height)
		d.bg, d.border = j.BackgroundColor&15, j.BorderColor&15
		for i := range d.cells {
			ch, col := 32, 14
			if i < len(j.Screencodes) {
				ch = j.Screencodes[i] & 255
			}
			if i < len(j.Colors) {
				col = j.Colors[i] & 15
			}
			d.cells[i] = makeCell(ch, col, 0, 0)
		}
		lower := map[string]int{"upper": 0, "lower": 1}[j.Charset]
		out = append(out, PetScreen{doc: d, lower: lower, name: j.Name, fb: -1})
		if len(out) >= 1000 {
			break
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no screens in the JSON")
	}
	return out, nil
}

// ---------------------------------------------------------------- Playscii

// Playscii (JP LeBreton, MIT) saves JSON: width, height, charset and palette by
// name (its charsets/ and palettes/ folders), camera, and frames (delay in seconds)
// of layers (z, visible, name) of tiles (char, fg, bg, xform) row by row. A
// charset's index is the place of the glyph in its picture; a palette's index 0 is
// transparent, 1.. are the picture's colours in the order they first appear.
// Which index is which glyph was found by comparing Playscii's pictures with the
// ROM fonts (tools: see the knowledge note).

// psciPalettes are Playscii's palettes CharEdit knows (1.. in order).
var psciPalettes = map[string][]uint32{
	"ansi":         {0x000000, 0xAA0000, 0x00AA00, 0xAA5500, 0x0000AA, 0xAA00AA, 0x00AAAA, 0xAAAAAA, 0x555555, 0xFF5555, 0x55FF55, 0xFFFF55, 0x5555FF, 0xFF55FF, 0x55FFFF, 0xFFFFFF},
	"ega":          {0x000000, 0x0000AA, 0x00AA00, 0x00AAAA, 0xAA0000, 0xAA00AA, 0xAA5500, 0xAAAAAA, 0x555555, 0x5555FF, 0x55FF55, 0x55FFFF, 0xFF5555, 0xFF55FF, 0xFFFF55, 0xFFFFFF},
	"c64_original": {0x0C1013, 0xC1C4C2, 0x8B3F2F, 0x57A9BC, 0x8B53B0, 0x398942, 0x393BA3, 0xDFC468, 0x8B551B, 0x632E00, 0xBC6F5F, 0x3F4443, 0x727472, 0x89D691, 0x7B78E1, 0xA7A7A9},
	"c64_pepto":    {0x0C1013, 0xC1C4C2, 0x68372B, 0x70A4B2, 0x6F3D86, 0x588D43, 0x352879, 0xB8C76F, 0x6F4F25, 0x433900, 0x9A6759, 0x444444, 0x6C6C6C, 0x9AD284, 0x6C5EB5, 0x959595},
	"c64_edscii":   {0x0C1013, 0x8B3F2F, 0x8B53B0, 0x393BA3, 0x398942, 0xDFC468, 0x3F4443, 0xA7A7A9, 0x632E00, 0x8B551B, 0xBC6F5F, 0x7B78E1, 0x89D691, 0x57A9BC, 0x727472, 0xC1C4C2},
	"atari": {0x000000, 0x444400, 0x702800, 0x841800, 0x880000, 0x78005C, 0x480078, 0x140084, 0x000088, 0x00187C, 0x002C5C, 0x00402C, 0x003C00, 0x143800, 0x2C3000, 0x442800,
		0x404040, 0x646410, 0x844414, 0x983418, 0x9C2020, 0x8C2074, 0x602090, 0x302098, 0x1C209C, 0x1C3890, 0x1C4C78, 0x1C5C48, 0x205C20, 0x345C1C, 0x4C501C, 0x644818,
		0x6C6C6C, 0x848424, 0x985C28, 0xAC5030, 0xB03C3C, 0xA03C88, 0x783CA4, 0x4C3CAC, 0x3840B0, 0x3854A8, 0x386890, 0x387C64, 0x407C40, 0x507C38, 0x687034, 0x846830,
		0x909090, 0xA0A034, 0xAC783C, 0xC06848, 0xC05858, 0xB0589C, 0x8C58B8, 0x6858C0, 0x505CC0, 0x5070BC, 0x5084AC, 0x509C80, 0x5C9C5C, 0x6C9850, 0x848C4C, 0xA08444,
		0xB0B0B0, 0xB8B840, 0xBC8C4C, 0xD0805C, 0xD07070, 0xC070B0, 0xA070CC, 0x7C70D0, 0x6874D0, 0x6888CC, 0x689CC0, 0x68B494, 0x74B474, 0x84B468, 0x9CA864, 0xB89C58,
		0xC8C8C8, 0xD0D050, 0xCCA05C, 0xE09470, 0xE08888, 0xD084C0, 0xB484DC, 0x9488E0, 0x7C8CE0, 0x7C9CDC, 0x7CB4D4, 0x7CD0AC, 0x8CD08C, 0x9CCC7C, 0xB4C078, 0xD0B46C,
		0xDCDCDC, 0xE8E85C, 0xDCB468, 0xECA880, 0xECA0A0, 0xDC9CD0, 0xC49CEC, 0xA8A0EC, 0x90A4EC, 0x90B4EC, 0x90CCE8, 0x90E4C0, 0xA4E4A4, 0xB4E490, 0xCCD488, 0xE8CC7C,
		0xECECEC, 0xFCFC68, 0xE9CC7C, 0xFCBC94, 0xFCB4B4, 0xECB0E0, 0xD4B0FC, 0xBCB4FC, 0xA4B8FC, 0xA4C8FC, 0xA4E0FC, 0xA4FCD4, 0xB8FCB8, 0xC8FCA4, 0xE0EC9C, 0xFCE08C},
}

// psciC64Sets: the C64 screen code (upper set, lower set) of each glyph of Playscii's
// c64_petscii, c64_edscii and jpetscii charsets, found by comparing their pictures with the
// C64's character ROM (codes 128-255 are the reversed glyphs); -1-n is not in that set, n the nearest.
var psciC64Sets = map[string][2][]int{
	"c64_petscii": {
		{32, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 0, 33, 34, 35, 36, 37, 38, 39, 40, 41, 42, 43, 44, 45, 46, 47, 48, 49, 50, 51, 52, 53, 54, 55, 56, 57, 58, 59, 60, 61, 62, 63, 112, 114, 110, 82, 70, 64, 32, 68, 69, 85, 73, 87, 81, 79, 119, 80, 107, 91, 115, 78, 77, 86, 121, 120, 230, 74, 75, 108, 123, 101, 160, 103, 109, 113, 125, 95, 105, 118, 117, 104, 92, 127, 98, 124, 126, 76, 111, 122, 32, 84, 71, 66, 32, 72, 89, 32, 99, 100, 94, 97, 88, 118, 90, 65, -54, -55, -13, -95, -32, -44, -88, -2, -10, -90, -13, -10, -2, -2, -88, -88, -88, -85, -62, -50, -16, -23, -95, -48, -88, -48, 102, 83, -103, -231, 32, 32},
		{32, 65, 66, 67, 68, 69, 70, 71, 72, 73, 74, 75, 76, 77, 78, 79, 80, 81, 82, 83, 84, 85, 86, 87, 88, 89, 90, 27, 28, 29, 30, 31, 0, 33, 34, 35, 36, 37, 38, 39, 40, 41, 42, 43, 44, 45, 46, 47, 48, 49, 50, 51, 52, 53, 54, 55, 56, 57, 58, 59, 60, 61, 62, 63, 112, 114, 110, -122, -33, 64, 32, -46, -121, -113, -111, -16, -6, -102, 119, -104, 107, 91, 115, -48, -128, -89, 121, 120, 94, -110, -126, 108, 123, 101, 160, 103, 109, 113, 125, -252, -237, 118, 117, 104, 92, 127, 98, 124, 126, -102, 111, -104, 32, -77, -28, 93, 32, -30, -11, 32, 99, 100, -5, 97, -10, 118, -31, -32, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 102, -250, 105, 95, 32, 32},
	},
	"jpetscii": { // a PETSCII-like set of its own: half its glyphs are the ROM's, the rest shown as the nearest
		{32, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 29, -41, -42, 28, 34, -54, -55, -13, -95, -32, -44, -88, -2, -10, -90, -13, -10, -2, -2, -88, -88, -88, -85, -62, -50, -16, -23, -95, -48, -88, -48, 65, 88, 90, 83, 94, 49, 50, 51, 52, 53, 54, 55, 56, 57, 48, 45, 61, 100, 43, 58, 59, 33, 0, 35, 36, 37, -33, 38, 42, 40, 41, 60, 47, 62, 44, 46, 63, 112, 114, 110, 79, 119, 80, -109, 248, -124, -79, 69, -78, 85, -33, 73, 64, 107, 91, 115, 101, -172, 103, 245, 81, 246, 84, -161, 89, -45, -44, -34, 66, 109, 113, 125, 76, 111, 122, -125, 249, -127, -78, 82, -48, 74, -33, 75, -33, -208, 227, -209, 30, -234, -224, 78, 77, -65, -65, -21, -83, -50, -50, -33, -33, -161, 87, -161, -67, 95, 105, -33, -33, -90, -85, -50, -26, -26, -26, -33, -35, -205, 228, -251, -67, 31, -65, -48, 86, 102, 104, -93, -103, -105, -93, -35, -25, -102, -120, -120, -104, -205, -98, -112, -99, -178, -2, 108, 98, 123, -59, -33, -210, -112, -104, -102, -109, -104, -102, -120, -84, -11, -240, 225, 255, 97, -33, -33, -50, -86, -74, -109, -124, -33, -33, -6, -66, -24, -62, 124, 226, 126, -210, -33, -161, -75, -76, -125, -127, -40, -33, -47, -2, -67, -56, -11, -130, -161, -33, -91, 160},
		{32, 65, 66, 67, 68, 69, 70, 71, 72, 73, 74, 75, 76, 77, 78, 79, 80, 81, 82, 83, 84, 85, 86, 87, 88, 89, 90, 27, 29, -41, -42, 28, 34, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, -32, -10, -31, -250, -5, 49, 50, 51, 52, 53, 54, 55, 56, 57, 48, 45, 61, 100, 43, 58, 59, 33, 0, 35, 36, 37, -33, 38, 42, 40, 41, 60, 47, 62, 44, 46, 63, 112, 114, 110, -102, 119, -104, -109, 248, -124, -48, -121, -9, -113, -33, -111, 64, 107, 91, 115, 101, -153, 103, 245, -6, 246, -77, -161, -11, -45, -25, -34, 93, 109, 113, 125, -102, 111, -104, -125, 249, -127, -12, -122, -48, -110, -33, -126, -33, -230, 227, -232, 30, -255, -253, -48, -128, -65, -65, -56, -99, -50, -50, -33, -33, -161, -16, -161, -94, -252, -237, -33, -33, -119, -98, -50, -90, -7, -31, -33, -35, -230, 228, -232, -94, 31, -65, -25, -89, 102, 104, -93, 105, -105, -106, -35, -25, -102, -120, -120, -104, -226, -98, -112, -99, -153, -66, 108, 98, 123, -59, -23, -33, -112, -104, -102, -109, -104, -102, -120, -227, -75, -240, 225, 255, 97, -33, -33, -50, -113, -111, -109, -124, -33, -33, -70, -4, -24, -62, 124, 226, 126, -59, -33, -161, -110, -126, -125, -127, -40, -33, -23, -66, -94, -56, -75, -131, -161, -33, -152, 160},
	},
	"c64_edscii": {
		{32, 33, 34, 35, 36, 37, 38, 39, 40, 41, 42, 43, 44, 45, 46, 47, 48, 49, 50, 51, 52, 53, 54, 55, 56, 57, 58, 59, 60, 61, 62, 63, 0, -54, -55, -13, -95, -32, -44, -88, -2, -10, -90, -13, -10, -2, -2, -88, -88, -88, -85, -62, -50, -16, -23, -95, -48, -88, -48, 27, 28, 29, -48, 94, 160, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 65, 83, 88, 90, 81, 98, 121, 111, 100, 82, 70, 64, 68, 69, 120, 119, 99, 124, 123, 126, 108, 97, 84, 72, 112, 110, 109, 125, 85, 73, 74, 75, 105, 95, 77, 78, 86, 117, 71, 89, 127, 230, 104, 92, 102, 79, 76, 80, 122, 113, 107, 114, 115, 101, 66, 103, 118, 30, 31, -103, -231, 91, 87, -78, 32, 32, 32, 32, 32},
		{32, 33, 34, 35, 36, 37, 38, 39, 40, 41, 42, 43, 44, 45, 46, 47, 48, 49, 50, 51, 52, 53, 54, 55, 56, 57, 58, 59, 60, 61, 62, 63, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 122, -5, 160, 65, 66, 67, 68, 69, 70, 71, 72, 73, 74, 75, 76, 77, 78, 79, 80, 81, 82, 83, 84, 85, 86, 87, 88, 89, 90, -32, -250, -10, -31, -6, 98, 121, 111, 100, -122, -33, 64, -46, -121, 120, 119, 99, 124, 123, 126, 108, 97, -77, -30, 112, 110, 109, 125, -113, -111, -110, -126, -237, -252, -128, -48, -89, 117, -28, -11, 127, 94, 104, 92, 102, -102, -102, -104, -104, 113, 107, 114, 115, 101, 93, 103, 118, 30, 31, 105, 95, 91, -16, -25, 32, 32, 32, 32, 32},
	},
}

// psciC64Index: the glyph of Playscii's c64_real_shifted (upper set) and
// c64_real_unshifted (lower set) charsets for a screen code 0-127: they are in
// PETSCII order (32-63, 64-95, 96-127, 160-191), so codes 0-31 and 32-63 swap.
func psciC64Index(sc int) int {
	if sc < 64 {
		return sc ^ 32
	}
	return sc
}

// psciPETIndex: commodore_pet has the PET's screen codes in order but for 0 and 32.
func psciPETIndex(sc int) int {
	switch sc {
	case 0:
		return 32
	case 32:
		return 0
	}
	return sc
}

// psciAmigaIndex: amiga_topaz holds 32-127, 192-223, 160-191, 224-255 in that order (-1: not there).
func psciAmigaIndex(c int) int {
	switch {
	case c >= 32 && c < 128:
		return c - 32
	case c >= 192 && c < 224:
		return c - 96
	case c >= 160 && c < 192:
		return c - 32
	case c >= 224:
		return c - 64
	}
	return -1
}

func psciAmigaChar(i int) int {
	switch {
	case i < 96:
		return i + 32
	case i < 128:
		return i + 96
	case i < 160:
		return i + 32
	case i < 192:
		return i + 64
	}
	return 32
}

type psciTile struct {
	Char  float64 `json:"char"`
	Fg    float64 `json:"fg"`
	Bg    float64 `json:"bg"`
	Xform float64 `json:"xform"`
}

type psciLayer struct {
	Z       float64    `json:"z"`
	Visible *float64   `json:"visible,omitempty"`
	Name    string     `json:"name"`
	Tiles   []psciTile `json:"tiles"`
}

type psciFrame struct {
	Delay  float64     `json:"delay"`
	Layers []psciLayer `json:"layers"`
}

type psciFile struct {
	ActiveFrame   int         `json:"active_frame"`
	ActiveLayer   int         `json:"active_layer"`
	Camera        [3]float64  `json:"camera"`
	Charset       string      `json:"charset"`
	Frames        []psciFrame `json:"frames"`
	Height        int         `json:"height"`
	Palette       string      `json:"palette"`
	SelectedBg    int         `json:"selected_bg_color"`
	SelectedChar  int         `json:"selected_char"`
	SelectedFg    int         `json:"selected_fg_color"`
	SelectedXform int         `json:"selected_xform"`
	Width         int         `json:"width"`
}

// psciPage is how a page type goes to Playscii and back.
type psciPage struct {
	mode, machine string
	lower         bool
	pal           [16]uint32 // the page's colours (cell order)
	ice           bool
}

// psciNearest is the Playscii colour (1..) nearest to c.
// psciGTIA is an Atari colour register value as a colour of Playscii's atari palette, and back.
func psciGTIA(v int) int { return v&0x0E/2*16 + v>>4&15 + 1 }

func gtiaOfPsci(i int) int { return (i-1)&15<<4 | (i-1)>>4&7*2 }

func psciNearest(c uint32, pal []uint32) int {
	best, bd := 0, math.MaxInt
	for i, p := range pal {
		dr, dg, db := int(c>>16&255)-int(p>>16&255), int(c>>8&255)-int(p>>8&255), int(c&255)-int(p&255)
		if d := dr*dr + dg*dg + db*db; d < bd {
			best, bd = i, d
		}
	}
	return best + 1
}

// writePsci gives a Playscii file of the frames (one layer each); missed counts
// the cells whose character Playscii's charset has not (written as spaces).
func writePsci(docs []*Doc, holds []int, p psciPage) (out []byte, missed int) {
	if p.mode == "cpc" {
		return writePsciCPC(docs, holds, a.cpcSet)
	}
	if p.mode == "msx" {
		return writePsciMSX(docs, holds, a.msxSet), 0
	}
	w, h := 1, 1
	for _, d := range docs {
		w, h = max(w, d.cols), max(h, d.rows)
	}
	f := psciFile{Width: w, Height: h, Camera: [3]float64{float64(w) / 2, -float64(h) / 2, math.Max(2.5, float64(max(w, h))*0.9)},
		SelectedChar: 1, SelectedFg: 8, SelectedBg: 1}
	tile := func(d *Doc, x, y int) psciTile {
		v := d.get(x, y)
		ch := cellCh(v)
		switch p.mode {
		case "c64":
			fg, bg := cellFg(v)+1, d.bg+1
			if p.machine == "pet" || p.machine == "pet80" {
				fg = 2 // the PET's one colour: white on black in Playscii's C64 palette
			}
			if ch >= 128 { // reversed: the colours swap
				fg, bg = bg, fg
			}
			idx := psciC64Index(ch & 127)
			if (p.machine == "pet" || p.machine == "pet80") && !p.lower {
				idx = psciPETIndex(ch & 127)
			}
			return psciTile{Char: float64(idx), Fg: float64(fg), Bg: float64(bg)}
		case "atari":
			// Playscii's atari palette is GTIA's 128 colours, 16 hues to a row, a row a luminance.
			fg, bg := psciGTIA(atariText(d)), psciGTIA(d.bg)
			if ch >= 128 {
				fg, bg = bg, fg
			}
			return psciTile{Char: float64(ch & 127), Fg: float64(fg), Bg: float64(bg)}
		case "amiga":
			idx := psciAmigaIndex(ch)
			if idx < 0 {
				idx, missed = 0, missed+1
			}
			return psciTile{Char: float64(idx), Fg: float64(cellFg(v) + 1), Bg: float64(shownBg(v, p.ice) + 1)}
		}
		return psciTile{Char: float64(ch), Fg: float64(cellFg(v) + 1), Bg: float64(shownBg(v, p.ice) + 1)}
	}
	switch p.mode {
	case "c64":
		f.Palette = "c64_original" // Playscii's default; C64 colour order
		f.Charset = map[bool]string{true: "c64_real_unshifted", false: "c64_real_shifted"}[p.lower]
		if (p.machine == "pet" || p.machine == "pet80") && !p.lower {
			f.Charset = "commodore_pet"
		}
	case "atari":
		f.Charset, f.Palette = "atari", "atari"
	case "amiga":
		f.Charset, f.Palette = "amiga_topaz", "ansi"
	default:
		f.Charset, f.Palette = "dos", "ansi"
	}
	for i, d := range docs {
		hold := 100
		if i < len(holds) && holds[i] > 0 {
			hold = holds[i]
		}
		l := psciLayer{Name: "Layer 1", Tiles: make([]psciTile, 0, w*h)}
		one := 1.0
		l.Visible = &one
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				l.Tiles = append(l.Tiles, tile(d, x, y))
			}
		}
		f.Frames = append(f.Frames, psciFrame{Delay: float64(hold) / 1000, Layers: []psciLayer{l}})
	}
	out, _ = json.MarshalIndent(f, "", " ")
	return out, missed
}

func isPsci(b []byte) bool {
	t := bytes.TrimSpace(b[:min(len(b), 4096)])
	return len(t) > 0 && t[0] == '{' && bytes.Contains(b, []byte(`"charset"`)) && bytes.Contains(b, []byte(`"frames"`)) && bytes.Contains(b, []byte(`"tiles"`))
}

// psciArt is a Playscii file read: its frames as pages of one type.
type psciArt struct {
	mode    string // "pc", "amiga", "c64" or "atari"
	machine string // a Commodore page's machine
	lower   bool
	docs    []*Doc
	holds   []int
	note    string
	cpc     *cpcSetup // a CPC page's screen (cpc.go)
	msx     *msxSetup // an MSX page's screen (msx.go)
}

// parsePsci reads a Playscii file; layers are put together (by z, visible only).
func parsePsci(b []byte) (*psciArt, error) {
	var f psciFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("not a Playscii file: %v", err)
	}
	w, h := f.Width, f.Height
	// A tile in the file takes 20 bytes or more: a picture larger than the file
	// could hold is no Playscii file.
	if w < 1 || h < 1 || w > 4096 || h > maxRowsFor(w) || w*h > len(b)/16 || len(f.Frames) == 0 {
		return nil, fmt.Errorf("a Playscii picture of %dx%d", w, h)
	}
	if isCPCCharset(f.Charset) { // Logiker's Amstrad ASCII Exporter's charsets (cpc.go)
		return psciCPC(&f), nil
	}
	if f.Charset == "msx" { // msx.go
		return psciMSX(&f), nil
	}
	art := &psciArt{}
	var glyph func(i int) (ch int, ok bool) // the page's character for a Playscii glyph
	set, unknownCharset := 0, ""
	switch cs := f.Charset; cs {
	case "dos", "dos40":
		art.mode = "pc"
		glyph = func(i int) (int, bool) { return i & 255, i < 256 }
	case "amiga_topaz":
		art.mode = "amiga"
		glyph = func(i int) (int, bool) { return psciAmigaChar(i), i < 192 }
	case "atari":
		art.mode = "atari"
		glyph = func(i int) (int, bool) { return i & 127, i < 128 }
	case "c64_real_shifted", "c64_real_unshifted":
		art.mode, art.machine, art.lower = "c64", "c64", cs == "c64_real_unshifted"
		glyph = func(i int) (int, bool) { return psciC64Index(i & 127), i < 128 }
	case "commodore_pet":
		art.mode, art.machine = "c64", "pet"
		glyph = func(i int) (int, bool) { return psciPETIndex(i & 127), i < 128 }
	case "c64_petscii", "c64_edscii", "jpetscii":
		art.mode, art.machine = "c64", "c64"
		t := psciC64Sets[cs]
		// The set most of the used glyphs are in.
		var in [2]int
		for _, fr := range f.Frames {
			for _, l := range fr.Layers {
				for _, tl := range l.Tiles {
					if i := int(tl.Char); i >= 0 && i < len(t[0]) {
						for s := 0; s < 2; s++ {
							if t[s][i] >= 0 {
								in[s]++
							}
						}
					}
				}
			}
		}
		if in[1] > in[0] {
			set, art.lower = 1, true
		}
		glyph = func(i int) (int, bool) {
			if i < 0 || i >= len(t[set]) {
				return 32, false
			}
			if v := t[set][i]; v >= 0 {
				return v, true
			} else {
				return -1 - v, false
			}
		}
	default:
		// A charset of a machine CharEdit has no page for: opened as a PC page with
		// the glyph numbers as characters, never refused.
		art.mode, unknownCharset = "pc", cs
		glyph = func(i int) (int, bool) { return i & 255, i < 256 }
	}
	// colour: a Playscii colour (1..) as the page's colour number.
	pal, known := psciPalettes[f.Palette]
	pageCols := map[string][16]uint32{"pc": paletteByID("vga").colors, "amiga": paletteByID("vga").colors, "atari": atariPalette.colors}
	colour := func(i int) int {
		if i <= 0 {
			return 0
		}
		switch {
		case !known:
			return (i - 1) & 15
		case art.mode == "c64" && (f.Palette == "c64_original" || f.Palette == "c64_pepto"):
			return (i - 1) & 15
		case art.mode == "c64":
			if i <= len(pal) {
				return nearestOf(pal[i-1], psciCols(psciPalettes["c64_original"]))
			}
			return 0
		}
		if i > len(pal) {
			return 0
		}
		if art.mode == "atari" && f.Palette == "atari" { // a register value (gtia.go)
			return gtiaOfPsci(i)
		}
		if art.mode == "atari" {
			return gtiaNearest(pal[i-1], &gtiaPAL)
		}
		return nearestOf(pal[i-1], pageCols[art.mode])
	}
	missed := 0
	if len(f.Frames) > 10000 {
		f.Frames = f.Frames[:10000]
	}
	cells := 0
	for _, fr := range f.Frames {
		if cells += w * h; cells > maxPageCells {
			break
		}
		type tile struct{ ch, fg, bg int }
		top := make([]tile, w*h)
		for i := range top {
			top[i] = tile{-1, 0, 0}
		}
		layers := append([]psciLayer{}, fr.Layers...)
		// by z, lowest first (stable: the file's order for the same z)
		for i := 1; i < len(layers); i++ {
			for j := i; j > 0 && layers[j].Z < layers[j-1].Z; j-- {
				layers[j], layers[j-1] = layers[j-1], layers[j]
			}
		}
		for _, l := range layers {
			if l.Visible != nil && *l.Visible == 0 {
				continue
			}
			for i, tl := range l.Tiles {
				if i >= w*h {
					break
				}
				ch, fg, bg := int(tl.Char), int(tl.Fg), int(tl.Bg)
				m := &top[i]
				switch {
				case bg > 0: // the tile covers what is under it
					if fg <= 0 {
						ch = -1
					}
					*m = tile{ch, fg, bg}
				case fg > 0 && ch >= 0:
					if c, _ := glyph(ch); c != 32 || m.ch < 0 {
						m.ch, m.fg = ch, fg
					}
				}
			}
		}
		var d *Doc
		switch art.mode {
		case "atari":
			d = newDoc(w, h)
			d.bg, d.ink, d.border, d.glyph0 = atariBg, atariInk, 0, true
		default:
			d = newDoc(w, h)
		}
		if art.mode == "c64" || art.mode == "atari" {
			// One background for the screen: the commonest; a cell with it as its
			// character colour and another background is a reversed one.
			count := map[int]int{}
			for _, t := range top {
				count[t.bg]++
			}
			bg, n := 0, -1
			for c, k := range count {
				if k > n || k == n && c < bg {
					bg, n = c, k
				}
			}
			d.bg, d.border = colour(bg), colour(bg)
			inks := map[int]int{}
			for i, t := range top {
				ch, ok := 32, true
				if t.ch >= 0 {
					ch, ok = glyph(t.ch)
				}
				if !ok {
					missed++
				}
				fg := t.fg
				if t.bg != bg && t.bg > 0 && (t.fg == bg || t.ch < 0 || ch == 32) {
					ch |= 128
					fg = t.bg
				}
				if fg <= 0 {
					fg = bg
				}
				inks[fg]++
				d.cells[i] = makeCell(ch, colour(fg), 0, 0)
			}
			if art.mode == "atari" {
				ink, n := 0, -1
				for c, k := range inks {
					if c != bg && (k > n || k == n && c < ink) {
						ink, n = c, k
					}
				}
				d.ink = colour(ink)
			}
		} else {
			for i, t := range top {
				ch, ok := 32, true
				if t.ch >= 0 && t.fg > 0 {
					ch, ok = glyph(t.ch)
				}
				if !ok {
					missed++
				}
				d.cells[i] = makeCell(ch, colour(t.fg), colour(t.bg), 0)
			}
		}
		if art.mode == "c64" {
			d.glyph0 = true
		}
		if art.machine == "pet" { // one colour on a PET's screen
			d.bg, d.border = 0, 0
			for i, v := range d.cells {
				d.cells[i] = makeCell(cellCh(v), 1, 0, 0)
			}
		}
		art.docs = append(art.docs, d)
		art.holds = append(art.holds, int(math.Round(math.Min(math.Max(fr.Delay, 0), 3600)*1000)))
	}
	var notes []string
	if unknownCharset != "" {
		notes = append(notes, fmt.Sprintf("Playscii's charset %q has no page here: opened as a PC page, the characters by number", unknownCharset))
	}
	if n := len(f.Frames[0].Layers); n > 1 {
		notes = append(notes, fmt.Sprintf("its %d layers are put together", n))
	}
	if missed > 0 {
		notes = append(notes, fmt.Sprintf("%d cells show the nearest character", missed))
	}
	if !known {
		notes = append(notes, fmt.Sprintf("palette %q is unknown: colours by number", f.Palette))
	}
	art.note = strings.Join(notes, "; ")
	return art, nil
}

func psciCols(p []uint32) [16]uint32 {
	var out [16]uint32
	for i := range out {
		if i < len(p) {
			out[i] = p[i]
		}
	}
	return out
}

// ---------------------------------------------------------------- in CharEdit

// editorOwn are the formats of other editors CharEdit reads back: saving in one
// keeps the page's file in it (Ctrl+S writes it again), as for CharEdit's own.
var editorOwn = map[string]bool{"idf": true, "adf": true, "tnd": true, "icy": true, "xp": true, "psci": true, "pet": true, "pmjson": true}

// editorCarriesFont: the format holds the page's font.
func editorCarriesFont(fmtID string) bool { return fmtID == "idf" || fmtID == "adf" || fmtID == "icy" }

func init() {
	framesFormats["psci"], framesFormats["pmjson"] = true, true
}

// editorKind is which other editor's file b is ("" none of them).
func editorKind(name string, b []byte) string {
	switch {
	case isIDF(b):
		return "idf"
	case isTundra(b):
		return "tnd"
	case strings.HasSuffix(strings.ToLower(name), ".adf"):
		return "adf"
	case isIcyDraw(b):
		return "icy"
	case isXP(name, b):
		return "xp"
	case isPsci(b):
		return "psci"
	case isMarqPet(name, b):
		return "pet"
	case isPetmateJSON(b):
		return "pmjson"
	}
	return ""
}

// parseEditorPC reads the PC page formats of other editors.
func parseEditorPC(kind string, b []byte) (*Doc, XBinInfo, string, error) {
	switch kind {
	case "icy":
		doc, err := parseIcyDraw(b)
		if err != nil {
			return nil, XBinInfo{}, "", err
		}
		d, info, note := icyPage(doc)
		return d, info, note, nil
	case "xp":
		d, info, err := parseXP(b)
		return d, info, "", err
	}
	d, info, err := parseOwn(kind, b)
	return d, info, "", err
}

// loadEditor opens a file of another editor into the current tab; false if b is none.
func (a *App) loadEditor(b []byte, name, path, as string) bool {
	kind := editorKind(name, b)
	if kind == "" {
		return false
	}
	note := ""
	switch kind {
	case "idf", "adf", "tnd", "icy", "xp":
		if as != "auto" && as != "pc" {
			return false
		}
		_, info, n, err := parseEditorPC(kind, b)
		if err != nil {
			a.msg("Could not read " + name + ": " + err.Error())
			return true
		}
		note = n
		a.srcKind, a.ownFont, a.ownLower, a.ownPal = "", nil, nil, nil
		a.mode, a.ice = "pc", info.ice
		if info.font != nil {
			a.ownFont, a.fontFor["pc"] = info.font, ownID
		}
		if info.pal != nil {
			a.ownPal, a.paletteFor["pc"] = info.pal, "file"
		}
	case "psci":
		art, err := parsePsci(b)
		if err != nil {
			if as != "auto" {
				return false
			}
			a.msg("Could not read " + name + ": " + err.Error())
			return true
		}
		if as != "auto" && as != art.mode {
			return false
		}
		if art.mode == "c64" && !a.hasC64 {
			t := a.Tab
			openC64Req(func() {
				if a.backTo(t) {
					a.loadBytes(b, name, path, as)
				}
			})
			return true
		}
		note = art.note
		a.srcKind, a.ownFont, a.ownLower, a.ownPal = "", nil, nil, nil
		a.mode, a.ice = art.mode, true
		if art.mode == "c64" {
			a.screenMachine(art.machine)
		}
	case "pet", "pmjson":
		if as != "auto" && as != "c64" {
			return false
		}
		if _, err := a.editorScreens(kind, b); err != nil {
			a.msg("Could not read " + name + ": " + err.Error())
			return true
		}
		if !a.hasC64 {
			t := a.Tab
			openC64Req(func() {
				if a.backTo(t) {
					a.loadBytes(b, name, path, as)
				}
			})
			return true
		}
		a.srcKind, a.ownFont, a.ownLower, a.ownPal = "", nil, nil, nil
		a.mode = "c64"
		a.screenMachine("c64")
	}
	a.srcKind = kind
	a.finishLoad(b, name, path, nil, kind)
	// The SAUCE record: after an iCEDraw, ArtWorx or TundraDraw file, in an IcyDraw
	// file's SAUCE record. Saving again keeps it (without comment lines in the first three).
	sauceOf := b
	if kind == "icy" {
		sauceOf = nil
		if doc, err := parseIcyDraw(b); err == nil {
			sauceOf = doc.sauce
		}
	}
	if s, _ := readSauce(sauceOf); s != nil && (kind == "idf" || kind == "adf" || kind == "tnd" || kind == "icy") {
		a.sauce = s
		a.info = Info{writeSauce: true, title: s.title, author: s.author, group: s.group, notes: s.notes}
	}
	switch {
	case note != "":
		a.msg(name + ": " + note)
	case a.nFrames() > 1:
		a.msg(fmt.Sprintf("%s: %d frames (%s and %s go through them)", name, a.nFrames(), keyName("frame_previous"), keyName("frame_next")))
	}
	return true
}

func (a *App) editorScreens(kind string, b []byte) ([]PetScreen, error) {
	if kind == "pet" {
		d, lower, err := parseMarqPet(b)
		if err != nil {
			return nil, err
		}
		return []PetScreen{{doc: d, lower: lower, fb: -1}}, nil
	}
	return parsePetmateJSON(b)
}

// reparseEditor reads the page again from its file of another editor; false if it is none.
func (a *App) reparseEditor() bool {
	switch a.srcKind {
	case "icy", "xp":
		if d, _, _, err := parseEditorPC(a.srcKind, a.source); err == nil {
			a.doc = d
		}
	case "psci":
		art, err := parsePsci(a.source)
		if err != nil {
			break
		}
		if len(art.docs) == 1 {
			a.doc = art.docs[0]
		} else {
			a.frames = make([]Frame, len(art.docs))
			for i, d := range art.docs {
				a.frames[i] = Frame{doc: d, hold: art.holds[i], lower: -1, src: -1}
			}
			a.doc = a.frames[0].doc.clone()
		}
		if art.mode == "c64" {
			a.setCase(art.lower)
		}
		if art.cpc != nil {
			a.cpcSet = *art.cpc
		}
		if art.msx != nil {
			a.msxSet = *art.msx
			a.msxBrushColours()
		}
	case "pet", "pmjson":
		scr, err := a.editorScreens(a.srcKind, a.source)
		if err != nil {
			break
		}
		if len(scr) == 1 {
			a.doc = scr[0].doc
			a.setCase(scr[0].lower == 1)
			break
		}
		a.frames = make([]Frame, len(scr))
		for i, s := range scr {
			a.frames[i] = Frame{doc: s.doc, lower: s.lower, name: s.name, src: -1}
		}
		a.doc = a.frames[0].doc.clone()
		a.showCase()
	default:
		return false
	}
	a.cx, a.cy, a.sel, a.float = 0, 0, nil, nil
	return true
}

// editorSauce is the SAUCE record for an iCEDraw, ArtWorx or TundraDraw file when
// the page keeps one: without comment lines, as ansilove finds the font and
// palette of a file with them in the wrong place.
func (a *App) editorSauce(d *Doc, fileType, artLen int) []byte {
	if !a.info.writeSauce {
		return nil
	}
	return makeSauce(SauceInfo{title: a.info.title, author: a.info.author, group: a.info.group, cols: d.cols, rows: max(1, d.rows),
		font: a.font().sauce, fileType: fileType, flags: map[bool]int{true: 1}[a.ice]}, artLen)
}

// encodeEditor writes the page in another editor's format; ok false if fmtID is none of them.
func (a *App) encodeEditor(fmtID string, d *Doc) (out []byte, ok bool, err error) {
	ok = true
	switch fmtID {
	case "idf":
		if d.rows > 200 {
			a.msg(fmt.Sprintf("Saved %d rows: iCEDraw itself shows 200 (ansilove and IcyDraw show them all)", d.rows))
		}
		out = writeIDF(d, a.pal, a.font())
		out = append(out, a.editorSauce(d, 1, len(out))...)
	case "adf":
		if out, err = writeADF(d, a.pal, a.font()); err == nil {
			out = append(out, a.editorSauce(d, 1, len(out))...)
		}
	case "tnd":
		if out, err = writeTundra(d, a.pal, a.ice); err == nil {
			out = append(out, a.editorSauce(d, 8, len(out))...)
		}
	case "icy":
		top := d
		if d.rows > 80 { // IcyDraw's picture shows the first 80 rows
			top = &Doc{cols: d.cols, rows: 80, cells: d.cells[:80*d.cols], bg: d.bg, border: d.border, ink: d.ink, glyph0: d.glyph0}
		}
		fb := a.renderDoc(top, 1)
		img := image.NewNRGBA(image.Rect(0, 0, fb.w, fb.h))
		for i, p := range fb.px {
			img.Pix[i*4], img.Pix[i*4+1], img.Pix[i*4+2], img.Pix[i*4+3] = byte(p>>16), byte(p>>8), byte(p), 255
		}
		out = writeIcy(d, a.pal, a.font(), a.ice, img)
	case "xp":
		out = writeXP(d, a.pal, a.ice)
	case "psci":
		a.keepCase()
		docs := a.allDocs()
		holds := make([]int, len(docs))
		for i := range holds {
			holds[i] = holdDefault(a.mode)
			if a.frames != nil && a.frames[i].hold > 0 {
				holds[i] = a.frames[i].hold
			}
		}
		var missed int
		out, missed = writePsci(docs, holds, psciPage{mode: a.mode, machine: a.machine, lower: a.lower(), pal: a.pal, ice: a.ice})
		if missed > 0 {
			a.msg(fmt.Sprintf("%d cells have characters Playscii's charset has not: written as spaces", missed))
		}
	case "pet":
		if d.cols > 255 || d.rows > 255 {
			a.msg("A .pet holds at most 255x255 cells: the rest is not saved")
		}
		out = writeMarqPet(d, a.lower())
	case "marqbas", "marqasm":
		if fmtID == "marqbas" && (d.cols != 40 || d.usedRows() > 25) {
			a.msg("Only the first 40x25 cells fit the BASIC listing (a C64 screen)")
		}
		if fmtID == "marqbas" {
			out = writeMarqBas(d, a.lower())
		} else {
			out = writeMarqAsm(d, a.lower())
		}
	case "pmjson":
		out = writePetmateJSON(a.petScreens())
	default:
		ok = false
	}
	return out, ok, err
}
