package main

// ATASCII: the Atari 8-bit computers' text screen. A cell's byte is the
// ATASCII code (128-255 inverse); the screen has one background, one text
// colour (Doc.ink) and a border, as in GRAPHICS 0. Files are .ata: the bytes
// sent to the screen editor, with EOL (0x9B) between lines and ESC (0x1B)
// before a control character that should show as its glyph.
//
// The font is AltirraOS's (the Altirra emulator's free replacement for the
// Atari OS), see fonts/AltirraOS-LICENSE.txt.

import (
	"bytes"
	_ "embed"
)

//go:embed fonts/AltirraOS_atari.font
var atariFontStd []byte

//go:embed fonts/AltirraOS_atari_intl.font
var atariFontIntl []byte

// atariFont lays the ROM's 128 glyphs (in the Atari's internal order) out by
// ATASCII code, the top 128 inverse, rows doubled.
func atariFont(id, name string, rom []byte) *Font {
	bits := make([]byte, 4096)
	for c := 0; c < 256; c++ {
		a := c & 0x7F
		internal := a
		switch {
		case a < 32:
			internal = a + 64
		case a < 96:
			internal = a - 32
		}
		for r := 0; r < 8; r++ {
			v := rom[internal*8+r]
			if c >= 128 {
				v = ^v
			}
			bits[c*16+r*2], bits[c*16+r*2+1] = v, v
		}
	}
	return &Font{id: id, name: name, kind: "atari", syncterm: -1, bits: bits}
}

func init() {
	fonts = append(fonts,
		atariFont("atari", "Atari (AltirraOS)", atariFontStd),
		atariFont("atarixl", "Atari XL international (AltirraOS)", atariFontIntl))
}

// An Atari has 128 colours; these are 16 of them, close to the NTSC ones.
var atariPalette = pal("atari", "Atari", 0x000000, 0xFFFFFF, 0x1C3F9E, 0x8EB4F0, 0x555555, 0xAAAAAA, 0xA83224, 0xE07A3A,
	0xD6C840, 0x3E9A3A, 0x9AE07A, 0x2E8C8C, 0x7A3AA8, 0xD07AD0, 0x6B4A1E, 0xE8D8A8)

// The Atari's own start-up screen: blue background, light blue text, black border.
const atariBg, atariInk = 2, 3

var atariFkeys = [][]int{
	{0xA0, 0x19, 0x99, 0x15, 0x95, 0x0D, 0x0E, 0x16, 0x02, 0x82},
	{0x11, 0x12, 0x05, 0x7C, 0x1A, 0x03, 0x01, 0x04, 0x17, 0x18},
	{0x13, 0x06, 0x07, 0x08, 0x0A, 0x88, 0x8A, 0x09, 0x0B, 0x0C},
	{0x0F, 0x00, 0x10, 0x60, 0x7B, 0x14, 0x1C, 0x1D, 0x1E, 0x1F},
	{0x7E, 0x7F, 0x8F, 0x89, 0x8B, 0x8C, 0x94, 0x92, 0x91, 0x93},
}

// atariControl: bytes the screen editor acts on instead of showing (shown after ESC).
func atariControl(c byte) bool {
	switch c {
	case 0x1B, 0x1C, 0x1D, 0x1E, 0x1F, 0x7D, 0x7E, 0x7F, 0x9B, 0x9C, 0x9D, 0x9E, 0x9F, 0xFD, 0xFE, 0xFF:
		return true
	}
	return false
}

// AtariTerm prints ATASCII into a Doc like the screen editor (40 columns, no margins),
// with the page endless downwards.
type AtariTerm struct {
	beforeClear func()
	doc         *Doc
	x, y        int
	esc         bool
}

func newAtariTerm(d *Doc) *AtariTerm { return &AtariTerm{doc: d} }

func (t *AtariTerm) cursor() (int, int) { return t.x, t.y }

func (t *AtariTerm) put(c byte) {
	d := t.doc
	d.set(t.x, t.y, makeCell(int(c), 0, 0, 0))
	if t.x++; t.x >= d.cols {
		t.x, t.y = 0, t.y+1
	}
}

func (t *AtariTerm) write(b []byte) {
	d := t.doc
	for _, c := range b {
		if t.esc {
			t.esc = false
			t.put(c)
			continue
		}
		switch c {
		case 0x1B:
			t.esc = true
		case 0x9B: // EOL
			t.x, t.y = 0, t.y+1
		case 0x7D: // clear the screen
			if t.beforeClear != nil {
				t.beforeClear()
			}
			fillCells(d.cells[:d.rows*d.cols], blank)
			t.x, t.y = 0, 0
		case 0x1C:
			t.y = max(0, t.y-1)
		case 0x1D:
			t.y++
		case 0x1E:
			if t.x--; t.x < 0 {
				t.x = d.cols - 1
			}
		case 0x1F:
			if t.x++; t.x >= d.cols {
				t.x = 0
			}
		case 0x7E: // backspace: back and blank
			if t.x > 0 {
				t.x--
				d.set(t.x, t.y, blank)
			}
		case 0x7F: // tab
			t.x = min(d.cols-1, (t.x&^7)+8)
		case 0x9C: // delete line
			sh := &Term{doc: d, y: t.y}
			sh.shiftRows(-1)
		case 0x9D: // insert line
			sh := &Term{doc: d, y: t.y}
			sh.shiftRows(1)
		case 0xFE: // delete character
			for x := t.x; x < d.cols-1; x++ {
				d.set(x, t.y, d.get(x+1, t.y))
			}
			d.set(d.cols-1, t.y, blank)
		case 0xFF: // insert character
			for x := d.cols - 1; x > t.x; x-- {
				d.set(x, t.y, d.get(x-1, t.y))
			}
			d.set(t.x, t.y, blank)
		case 0x9E, 0x9F, 0xFD: // tab stops, the bell
		default:
			t.put(c)
		}
	}
}

func parseAta(b []byte, cols int) *Doc {
	if cols <= 0 {
		cols = atariWidth(b)
	}
	d := newDoc(cols, 24)
	d.bg, d.ink, d.border, d.glyph0 = atariBg, atariInk, 0, true
	t := newAtariTerm(d)
	var f fullest
	t.beforeClear = func() { f.look(t.doc) }
	writeWatching(t, b, &f)
	d = f.pick(t.doc)
	d.trim(1)
	return d
}

// atariWidth guesses the width a file was made for. The screen editor leaves a
// margin of two, so text made in it is 38 wide; terminals use all 40. A line
// longer than 38 settles it; otherwise the width where the rows line up best
// (the same character stacked over itself) wins.
func atariWidth(b []byte) int {
	run := 0
	for _, c := range b {
		switch c {
		case 0x9B:
			if run > 38 && run <= 40 { // a line of 39 or 40 ended by EOL
				return 40
			}
			run = 0
		case 0x7D:
			run = 0
		default:
			run++
		}
	}
	score := func(w int) int {
		d := newDoc(w, 24)
		newAtariTerm(d).write(b)
		n := 0
		for y := 0; y+1 < d.rows; y++ {
			for x := 0; x < w; x++ {
				if v := d.get(x, y); !d.blankAt(v) && cellCh(v) == cellCh(d.get(x, y+1)) {
					n++ // the same character stacked: a drawn shape
				}
			}
		}
		return n
	}
	if score(38) > score(40) {
		return 38
	}
	return 40
}

// writeAta gives the page as ATASCII: EOL after a row that does not fill the
// width, ESC before characters the editor would act on.
func writeAta(d *Doc, clear bool) []byte {
	var out []byte
	if clear {
		out = append(out, 0x7D)
	}
	rows := d.usedRows()
	for y := 0; y < rows; y++ {
		last := d.lastCol(y)
		for x := 0; x <= last; x++ {
			c := byte(cellCh(d.get(x, y)))
			if c == 0x9B { // the EOL glyph cannot be sent, even after ESC
				c = 0x20
			}
			if atariControl(c) {
				out = append(out, 0x1B)
			}
			out = append(out, c)
		}
		if last < d.cols-1 {
			out = append(out, 0x9B)
		}
	}
	return out
}

// atariFromRune is the ATASCII code a typed character gives, -1 if none.
func atariFromRune(r rune) int {
	switch {
	case r >= 0x20 && r <= 0x5F, r >= 0x61 && r <= 0x7A, r == '|':
		return int(r)
	}
	for c, u := range atariRunes {
		if u == r {
			return c
		}
	}
	return -1
}

// atariRunes are Unicode stand-ins for the ATASCII graphics, for copying as text.
var atariRunes = map[int]rune{
	0x00: '♥', 0x01: '├', 0x02: '▊', 0x03: '┘', 0x04: '┤', 0x05: '┐', 0x06: '╱', 0x07: '╲', 0x08: '◢', 0x09: '▗',
	0x0A: '◣', 0x0B: '▝', 0x0C: '▘', 0x0D: '▔', 0x0E: '▁', 0x0F: '▖', 0x10: '♣', 0x11: '┌', 0x12: '─', 0x13: '┼',
	0x14: '•', 0x15: '▄', 0x16: '▎', 0x17: '┬', 0x18: '┴', 0x19: '▌', 0x1A: '└', 0x1B: '␛', 0x1C: '↑', 0x1D: '↓',
	0x1E: '←', 0x1F: '→', 0x60: '♦', 0x7B: '♠', 0x7D: '↰', 0x7E: '◀', 0x7F: '▶',
}

func atariRune(c int) rune {
	inv, a := c&0x80 != 0, c&0x7F
	if inv && a == 0x20 {
		return '█'
	}
	if r, ok := atariRunes[a]; ok {
		return r
	}
	if a >= 0x20 {
		return rune(a)
	}
	return '?'
}

// looksAtascii: lines end in EOL (0x9B), which the Amiga uses as CSI followed
// by digits and a letter, and there are fewer LFs than EOLs (in ATASCII LF is a
// graphic); or, for animations, many of the Atari's cursor moves and no ANSI.
func looksAtascii(b []byte) bool {
	b = bytes.TrimRight(b, "\x00") // zeros after the end: the rest of a disk sector
	if bytes.HasPrefix(b, []byte("PK\x03\x04")) || bytes.HasPrefix(b, []byte("MZ")) || bytes.Count(b, []byte{0})*20 > len(b) {
		return false // an archive, a program, or other binary data
	}
	eol, csi, lf, moves, ansi := 0, 0, 0, 0, 0
	var dir [4]int
	for i, c := range b {
		switch c {
		case 0x1C, 0x1D, 0x1E, 0x1F: // the Atari's cursor moves, used by animations
			moves++
			dir[c-0x1C]++
		case 0x1B:
			if i+1 < len(b) && b[i+1] == '[' {
				ansi++
			}
		case 0x0A: // a graphic in ATASCII, a line end elsewhere
			lf++
		case 0x9B:
			j := i + 1
			for j < len(b) && ((b[j] >= '0' && b[j] <= '9') || b[j] == ';') {
				j++
			}
			if j > i+1 && j < len(b) && ((b[j] >= 'A' && b[j] <= 'Z') || (b[j] >= 'a' && b[j] <= 'z') || b[j] == '@') {
				csi++
			} else {
				eol++
			}
		}
	}
	ways := 0 // an animation moves the cursor more than one way
	for _, n := range dir {
		if n*10 >= moves {
			ways++
		}
	}
	if moves >= 16 && ways >= 2 && ansi*4 < moves && lf < moves {
		return true
	}
	return eol >= 2 && eol > 4*csi && lf < eol
}
