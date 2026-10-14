package main

// ZX Spectrum pages: the 32x24 screen, a character per 8x8 cell with its ink,
// paper, bright and flash. Saved as .scr (6912 bytes: the bitmap in the
// Spectrum's order, then the attributes), which every Spectrum emulator loads.
// Opening a .scr matches each cell's pixels to the font; a cell that is a
// drawing, not a character, gets the nearest one (and a message says how many).
//
// The font is the 48K ROM's (fonts/zx-spectrum-NOTICE.txt); codes 128-143 are
// the block graphics, made from their bits; 144-164 the UDGs, A-U at start-up.

import _ "embed"

//go:embed fonts/zx-spectrum-48-charset.font
var zxROMFont []byte

const flashFlag = 1 << 18 // a cell that flashes (ink and paper swap twice a second)

var zxFont = func() *Font {
	bits := make([]byte, 4096)
	for c := 0; c < 256; c++ {
		for r := 0; r < 8; r++ {
			var v byte
			switch {
			case c >= 32 && c < 128:
				v = zxROMFont[(c-32)*8+r]
			case c >= 128 && c < 144: // 2x2 blocks: bit 0 top right, 1 top left, 2 bottom right, 3 bottom left
				q := c - 128
				if r < 4 {
					v = byte(0xF0*(q>>1&1)) | byte(0x0F*(q&1))
				} else {
					v = byte(0xF0*(q>>3&1)) | byte(0x0F*(q>>2&1))
				}
			case c >= 144 && c < 165: // the UDGs start as copies of A-U
				v = zxROMFont[('A'+c-144-32)*8+r]
			}
			bits[c*16+r*2], bits[c*16+r*2+1] = v, v
		}
	}
	return &Font{id: "zx", name: "ZX Spectrum", kind: "zx", syncterm: -1, bits: bits}
}()

func init() { fonts = append(fonts, zxFont) }

var zxPalette = pal("zx", "ZX Spectrum", 0x000000, 0x0000D7, 0xD70000, 0xD700D7, 0x00D700, 0x00D7D7, 0xD7D700, 0xD7D7D7,
	0x000000, 0x0000FF, 0xFF0000, 0xFF00FF, 0x00FF00, 0x00FFFF, 0xFFFF00, 0xFFFFFF)

var zxFkeys = [][]int{
	{128, 129, 130, 131, 132, 133, 134, 135, 136, 137},
	{138, 139, 140, 141, 142, 143, '#', '*', '+', '.'},
	{'-', '=', '|', '/', '\\', '_', '(', ')', '<', '>'},
	{'[', ']', '{', '}', ':', ';', '^', '~', 0x60, 0x7F},
	{144, 145, 146, 147, 148, 149, 150, 151, 152, 153},
}

// zxCell makes a Spectrum cell: ink and paper share their brightness.
func zxCell(ch, ink, paper int, bright, flash bool) uint32 {
	b := 0
	if bright {
		b = 8
	}
	v := makeCell(ch, ink&7|b, paper&7|b, 0)
	if flash {
		v |= flashFlag
	}
	return v
}

func newZXDoc() *Doc {
	d := newDoc(32, 24)
	fillCells(d.cells, zxCell(32, 0, 7, false, false)) // black ink on white paper, as at start-up
	d.border = 7
	return d
}

// scrAddr is where pixel row y (0-191), byte column x (0-31) is in the Spectrum's bitmap.
func scrAddr(x, y int) int { return (y&0xC0)<<5 | (y&7)<<8 | (y&0x38)<<2 | x }

func writeSCR(d *Doc) []byte { return writeSCRFont(d, zxFont) }

// writeSCRFont writes a screen drawn in the page's font (its own, or the ROM's).
func writeSCRFont(d *Doc, f *Font) []byte {
	if f == nil || f.kind != "zx" {
		f = zxFont
	}
	out := make([]byte, 6912)
	for cy := 0; cy < 24; cy++ {
		for cx := 0; cx < 32; cx++ {
			v := d.get(cx, cy)
			ch := cellCh(v)
			for r := 0; r < 8; r++ {
				out[scrAddr(cx, cy*8+r)] = f.bits[ch*16+r*2]
			}
			attr := byte(cellFg(v)&7) | byte(cellBg(v)&7)<<3
			if cellFg(v) > 7 || cellBg(v) > 7 {
				attr |= 0x40
			}
			if v&flashFlag != 0 {
				attr |= 0x80
			}
			out[6144+cy*32+cx] = attr
		}
	}
	return out
}

// parseSCR reads a screen: each cell's pixels matched to a character (as is, or
// with ink and paper swapped); it tells how many cells were only near a character.
// Any file can be opened as a screen: a short one is read as if padded with
// zeros, bytes past 6912 are ignored.
func parseSCR(b []byte) (*Doc, int) { return parseSCRFont(b, zxFont) }

// parseSCRFont matches the cells to the page's font (its own, or the ROM's).
func parseSCRFont(b []byte, font *Font) (*Doc, int) {
	if font == nil || font.kind != "zx" {
		font = zxFont
	}
	if len(b) < 6912 {
		b = append(append(make([]byte, 0, 6912), b...), make([]byte, 6912-len(b))...)
	}
	d := newZXDoc()
	approx := 0
	for cy := 0; cy < 24; cy++ {
		for cx := 0; cx < 32; cx++ {
			var g [8]byte
			for r := 0; r < 8; r++ {
				g[r] = b[scrAddr(cx, cy*8+r)]
			}
			attr := b[6144+cy*32+cx]
			ink, paper := int(attr&7), int(attr>>3&7)
			bright, flash := attr&0x40 != 0, attr&0x80 != 0
			best, bestD, swap := 32, 65, false
			for c := 32; c < 165; c++ {
				dn, di := 0, 0
				for r := 0; r < 8; r++ {
					f := font.bits[c*16+r*2]
					dn += popcount(g[r] ^ f)
					di += popcount(g[r] ^ ^f)
				}
				if dn < bestD {
					best, bestD, swap = c, dn, false
				}
				if di < bestD {
					best, bestD, swap = c, di, true
				}
				if bestD == 0 {
					break
				}
			}
			if bestD > 0 {
				approx++
			}
			if swap {
				ink, paper = paper, ink
			}
			d.set(cx, cy, zxCell(best, ink, paper, bright, flash))
		}
	}
	return d, approx
}

func popcount(v byte) int {
	n := 0
	for ; v != 0; v &= v - 1 {
		n++
	}
	return n
}

// zxFromRune: the Spectrum's set is ASCII but for £ (0x60), © (0x7F) and ↑ (0x5E).
func zxFromRune(r rune) int {
	switch r {
	case '£':
		return 0x60
	case '©':
		return 0x7F
	case '↑':
		return 0x5E
	case '`':
		return -1
	}
	if r >= 32 && r < 127 {
		return int(r)
	}
	return -1
}

func zxRune(c int) rune {
	switch {
	case c == 0x60:
		return '£'
	case c == 0x7F:
		return '©'
	case c == 0x5E:
		return '↑'
	case c >= 32 && c < 127:
		return rune(c)
	case c >= 128 && c < 144:
		return []rune(" ▝▘▀▗▐▚▜▖▞▌▛▄▟▙█")[c-128]
	case c >= 144 && c < 165:
		return rune('A' + c - 144)
	}
	return ' '
}
