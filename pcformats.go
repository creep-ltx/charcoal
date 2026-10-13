package main

// The PC BBS formats besides ANSI: PCBoard (@X codes), Avatar (^V codes),
// iCEDraw (.idf), ArtWorx (.adf) and TundraDraw (.tnd). The first two open
// and save; the last three open (with their own font or colours) and save as XBin.

import (
	"bytes"
	"fmt"
	"strconv"
)

// ---------------------------------------------------------------- PCBoard

func isPCBoard(b []byte) bool {
	n := 0
	for i := 0; i+3 < len(b); i++ {
		if b[i] == '@' && (b[i+1] == 'X' || b[i+1] == 'x') && isHex(b[i+2]) && isHex(b[i+3]) {
			n++
		}
	}
	return n >= 3
}

func isHex(c byte) bool { return c >= '0' && c <= '9' || c >= 'A' && c <= 'F' || c >= 'a' && c <= 'f' }

func hexVal(c byte) int {
	v, _ := strconv.ParseUint(string(c), 16, 8)
	return int(v)
}

// parsePCBoard reads PCBoard's @X colour codes, @CLS@ and @POS:n@; the rest prints as on an 80-column screen.
func parsePCBoard(b []byte, cols int) *Doc {
	d := newDoc(cols, 25)
	x, y, attr := 0, 0, 0x07
	put := func(c byte) {
		d.set(x, y, attrCell(c, byte(attr)))
		if x++; x >= cols {
			x, y = 0, y+1
		}
	}
	for i := 0; i < len(b); i++ {
		c := b[i]
		switch {
		case c == '@' && i+3 < len(b) && (b[i+1] == 'X' || b[i+1] == 'x') && isHex(b[i+2]) && isHex(b[i+3]):
			attr = hexVal(b[i+2])<<4 | hexVal(b[i+3])
			i += 3
		case c == '@' && bytes.HasPrefix(b[i:], []byte("@CLS@")):
			fillCells(d.cells[:d.rows*d.cols], blank)
			x, y = 0, 0
			i += 4
		case c == '@' && bytes.HasPrefix(b[i:], []byte("@POS:")):
			j := i + 5
			for j < len(b) && b[j] >= '0' && b[j] <= '9' {
				j++
			}
			if j < len(b) && b[j] == '@' {
				if n, err := strconv.Atoi(string(b[i+5 : j])); err == nil {
					x = max(0, min(cols-1, n-1))
				}
				i = j
			} else {
				put(c)
			}
		case c == '\r':
			x = 0
		case c == '\n':
			x, y = 0, y+1
		case c == '\t':
			x = min(cols-1, (x&^7)+8)
		case c == 0x1A:
			i = len(b)
		default:
			put(c)
		}
	}
	d.fit(cursorRows(x, y))
	return d
}

// writePCBoard gives the page with @X codes where the colours change.
func writePCBoard(d *Doc, clear bool) []byte {
	var out []byte
	if clear {
		out = append(out, "@CLS@"...)
	}
	attr := -1
	for y := 0; y < d.rows; y++ {
		last := d.lastCol(y)
		for x := 0; x <= last; x++ {
			v := d.get(x, y)
			if a := int(cellAttr(v)); a != attr {
				attr = a
				out = append(out, fmt.Sprintf("@X%02X", a)...)
			}
			c := byte(cellCh(v))
			switch c { // the bytes parsePCBoard acts on
			case 0, 0x09, 0x0A, 0x0D, 0x1A:
				c = ' '
			}
			out = append(out, c)
			if c == '@' && x < last {
				// "@X1F", "@CLS@" or "@POS:" in the art would be read as a code: a colour
				// code (the same colour again) after the @ keeps them apart.
				if n := byte(cellCh(d.get(x+1, y))); n == 'X' || n == 'x' || n == 'C' || n == 'P' {
					attr = -1
				}
			}
		}
		if last < d.cols-1 {
			out = append(out, "\r\n"...)
		}
	}
	return append(out, "@X07"...)
}

// safeByte: bytes a terminal would act on are written as spaces.
func safeByte(c byte) byte {
	switch c {
	case 0, 0x07, 0x08, 0x09, 0x0A, 0x0C, 0x0D, 0x16, 0x19, 0x1A, 0x1B:
		return ' '
	}
	return c
}

// ---------------------------------------------------------------- Avatar

func isAvatar(b []byte) bool {
	return bytes.Count(b, []byte{0x16, 0x01}) >= 3
}

// parseAvatar reads Avatar/0: ^V^A attr, ^V^B blink, ^V^C-^F cursor, ^V^G clear to the
// end of the line, ^V^H row col, ^Y char count (repeat), ^L clear screen.
func parseAvatar(b []byte, cols int) *Doc {
	d := newDoc(cols, 25)
	x, y, attr := 0, 0, 0x03
	put := func(c byte) {
		d.set(x, y, attrCell(c, byte(attr)))
		if x++; x >= cols {
			x, y = 0, y+1
		}
	}
	for i := 0; i < len(b); i++ {
		c := b[i]
		switch c {
		case 0x16:
			if i+1 >= len(b) {
				break
			}
			i++
			switch b[i] {
			case 0x01:
				if i+1 < len(b) {
					i++
					attr = int(b[i] & 0x7F)
				}
			case 0x02:
				attr |= 0x80
			case 0x03:
				y = max(0, y-1)
			case 0x04:
				y++
			case 0x05:
				x = max(0, x-1)
			case 0x06:
				x = min(cols-1, x+1)
			case 0x07:
				for k := x; k < cols; k++ {
					d.set(k, y, attrCell(32, byte(attr)))
				}
			case 0x08:
				if i+2 < len(b) {
					y, x = max(0, int(b[i+1])-1), max(0, min(cols-1, int(b[i+2])-1))
					i += 2
				}
			}
		case 0x19:
			if i+2 < len(b) {
				ch, n := b[i+1], int(b[i+2])
				for k := 0; k < n; k++ {
					put(ch)
				}
				i += 2
			}
		case 0x0C:
			fillCells(d.cells[:d.rows*d.cols], blank)
			x, y, attr = 0, 0, 0x03
		case '\r':
			x = 0
		case '\n':
			x, y = 0, y+1
		case 0x1A:
			i = len(b)
		default:
			put(c)
		}
	}
	d.fit(cursorRows(x, y))
	return d
}

// writeAvatar gives the page with ^V^A where the colours change and ^Y for runs.
func writeAvatar(d *Doc, clear bool) []byte {
	var out []byte
	if clear {
		out = append(out, 0x0C)
	}
	attr := -1
	for y := 0; y < d.rows; y++ {
		last := d.lastCol(y)
		for x := 0; x <= last; {
			v := d.get(x, y)
			if a := int(cellAttr(v)); a != attr {
				// ^V^A takes 7 bits (and turns blink off); blink / a bright background is ^V^B.
				attr = a
				out = append(out, 0x16, 0x01, byte(a&0x7F))
				if a&0x80 != 0 {
					out = append(out, 0x16, 0x02)
				}
			}
			n := 1
			for x+n <= last && d.get(x+n, y) == v && n < 255 {
				n++
			}
			ch := safeByte(byte(cellCh(v)))
			if n >= 4 {
				out = append(out, 0x19, ch, byte(n))
			} else {
				for k := 0; k < n; k++ {
					out = append(out, ch)
				}
			}
			x += n
		}
		if last < d.cols-1 {
			out = append(out, "\r\n"...)
		}
	}
	return out
}

// ---------------------------------------------------------------- iCEDraw

func isIDF(b []byte) bool { return len(b) > 12+4096+48 && bytes.HasPrefix(b, []byte("\x041.4")) }

// parseIDF reads an iCEDraw file: the size, cells packed in runs, then its font and palette.
func parseIDF(b []byte) (*Doc, XBinInfo, error) {
	var info XBinInfo
	if !isIDF(b) {
		return nil, info, fmt.Errorf("not an iCEDraw file")
	}
	b = b[:dataEnd(b)] // the font and palette end before a SAUCE record
	if len(b) < 12+4096+48 {
		return nil, info, fmt.Errorf("the iCEDraw file is cut short")
	}
	u16 := func(i int) int { return int(b[i]) | int(b[i+1])<<8 }
	w := u16(8) - u16(4) + 1 // x2 - x1 + 1
	if w < 1 || w > 1000 {
		return nil, info, fmt.Errorf("an iCEDraw file %d columns wide", w)
	}
	end := len(b) - 4096 - 48
	limit := 2 * w * maxRowsFor(w) // bytes of cells on the largest page
	var cells []byte
	for i := 12; i+1 < end && len(cells) < limit; {
		if b[i] == 1 && b[i+1] == 0 && i+5 < end { // a run: count, then the cell
			n := u16(i+2) & 255
			for k := 0; k < n; k++ {
				cells = append(cells, b[i+4], b[i+5])
			}
			i += 6
			continue
		}
		cells = append(cells, b[i], b[i+1])
		i += 2
	}
	n := min(len(cells)/2, limit/2)
	d := newDoc(w, max(1, (n+w-1)/w))
	for k := 0; k < n; k++ {
		d.cells[k] = attrCell(cells[2*k], cells[2*k+1])
	}
	info.font = &Font{id: "xbin", name: "The file's own font", kind: "pc", syncterm: -1, bits: append([]byte{}, b[end:end+4096]...)}
	info.pal = sixBitPalette(b[end+4096 : end+4096+48])
	info.ice, info.cellH = true, 16
	return d, info, nil
}

// sixBitPalette reads 16 colours of 0-63 in BIOS order, put in the cells' order.
func sixBitPalette(p []byte) *[16]uint32 {
	var pal [16]uint32
	for i := range pal {
		r, g, bl := uint32(p[i*3]&63), uint32(p[i*3+1]&63), uint32(p[i*3+2]&63)
		pal[i] = 0xFF000000 | (r<<2|r>>4)<<16 | (g<<2|g>>4)<<8 | (bl<<2 | bl>>4)
	}
	pal = biosPalette(pal)
	return &pal
}

// ---------------------------------------------------------------- ArtWorx

// adfColours: which of the EGA's 64 colours the 16 text colours are.
var adfColours = [16]int{0, 1, 2, 3, 4, 5, 20, 7, 56, 57, 58, 59, 60, 61, 62, 63}

// parseADF reads an ArtWorx file: version, a 64-colour palette, a font, then 80-column cells.
func parseADF(b []byte) (*Doc, XBinInfo, error) {
	var info XBinInfo
	b = b[:dataEnd(b)] // the cells end before a SAUCE record
	if len(b) < 1+192+4096 {
		return nil, info, fmt.Errorf("not an ArtWorx file")
	}
	var p48 []byte
	for _, c := range adfColours {
		p48 = append(p48, b[1+c*3:1+c*3+3]...)
	}
	info.pal = sixBitPalette(p48)
	info.font = &Font{id: "xbin", name: "The file's own font", kind: "pc", syncterm: -1, bits: append([]byte{}, b[193:193+4096]...)}
	info.ice, info.cellH = true, 16
	art := b[193+4096:]
	n := min(len(art)/2, 80*maxRowsFor(80))
	d := newDoc(80, max(1, (n+79)/80))
	for k := 0; k < n; k++ {
		d.cells[k] = attrCell(art[2*k], art[2*k+1])
	}
	return d, info, nil
}

// ---------------------------------------------------------------- TundraDraw

func isTundra(b []byte) bool { return len(b) > 9 && b[0] == 24 && string(b[1:9]) == "TUNDRA24" }

// parseTundra reads a TundraDraw file. Its colours are 24-bit; a file with 16 or
// fewer gets them as its palette, else each is matched to the nearest VGA colour.
func parseTundra(b []byte) (*Doc, XBinInfo, error) {
	var info XBinInfo
	if !isTundra(b) {
		return nil, info, fmt.Errorf("not a TundraDraw file")
	}
	b = b[:max(9, dataEnd(b))] // the drawing ends before a SAUCE record
	type cell struct {
		ch     byte
		fg, bg uint32
	}
	var cells []cell
	cols := 80
	x, y, fg, bg := 0, 0, uint32(0xAAAAAA), uint32(0)
	be32 := func(i int) uint32 { return uint32(b[i])<<24 | uint32(b[i+1])<<16 | uint32(b[i+2])<<8 | uint32(b[i+3]) }
	maxRows := maxRowsFor(cols)
	put := func(c byte) {
		if y >= maxRows {
			return // past the largest page
		}
		for len(cells) <= y*cols+x {
			cells = append(cells, cell{' ', 0xAAAAAA, 0})
		}
		cells[y*cols+x] = cell{c, fg, bg}
		if x++; x >= cols {
			x, y = 0, y+1
		}
	}
	for i := 9; i < len(b); i++ {
		switch b[i] {
		case 1:
			if i+8 < len(b) {
				y, x = int(be32(i+1)), int(be32(i+5))
				if y < 0 || y >= maxRows || x < 0 || x >= cols {
					y, x = 0, 0
				}
				i += 8
			}
		case 2:
			if i+5 < len(b) {
				fg = be32(i+2) & 0xFFFFFF
				put(b[i+1])
				i += 5
			}
		case 4:
			if i+5 < len(b) {
				bg = be32(i+2) & 0xFFFFFF
				put(b[i+1])
				i += 5
			}
		case 6:
			if i+9 < len(b) {
				fg, bg = be32(i+2)&0xFFFFFF, be32(i+6)&0xFFFFFF
				put(b[i+1])
				i += 9
			}
		default:
			put(b[i])
		}
	}
	// The colours: their own palette if they fit in 16, else the nearest of the VGA's.
	index := map[uint32]int{}
	for _, c := range cells {
		for _, col := range []uint32{c.fg, c.bg} {
			if _, ok := index[col]; !ok && len(index) <= 16 {
				index[col] = len(index)
			}
		}
	}
	var pal [16]uint32
	own := len(index) <= 16
	if own {
		for col, i := range index {
			pal[i] = 0xFF000000 | col
		}
		info.pal = &pal
	}
	vga := paletteByID("vga").colors
	pick := func(col uint32) int {
		if own {
			return index[col]
		}
		return nearestOf(col, vga)
	}
	rows := max(1, (len(cells)+cols-1)/cols)
	d := newDoc(cols, rows)
	for i, c := range cells {
		d.cells[i] = makeCell(int(c.ch), pick(c.fg), pick(c.bg), 0)
	}
	info.ice, info.cellH = true, 16
	return d, info, nil
}

func nearestOf(col uint32, pal [16]uint32) int {
	best, bd := 0, 1<<62
	for i, p := range pal {
		dr, dg, db := int(col>>16&255)-int(p>>16&255), int(col>>8&255)-int(p>>8&255), int(col&255)-int(p&255)
		if d := dr*dr + dg*dg + db*db; d < bd {
			best, bd = i, d
		}
	}
	return best
}

// parseOwn reads the formats that bring their own font or colours.
func parseOwn(kind string, b []byte) (*Doc, XBinInfo, error) {
	switch kind {
	case "idf":
		return parseIDF(b)
	case "adf":
		return parseADF(b)
	case "tnd":
		return parseTundra(b)
	}
	return parseXBin(b)
}
