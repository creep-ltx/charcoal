package main

// The 8x16 bitmap fonts: 256 glyphs of 16 bytes (bit 7 = the left pixel),
// indexed by the byte: Latin-1 for the Amiga fonts, CP437 for the IBM VGA font.
// The Amiga fonts are TrueSchool's raw conversions (8x8 with every row doubled);
// IBM_VGA_8x16.raw was rendered from Px437_IBM_VGA_8x16.ttf by the old web tools.

import (
	"embed"
	"math/bits"
	"strings"
)

//go:embed fonts/*.raw fonts/*.f08 fonts/*.f14 fonts/*.f16
var fontFiles embed.FS

type Font struct {
	id, name string
	kind     string // the page it belongs to: "amiga", "pc" or "c64"
	sauce    string // SAUCE font name (TInfoS)
	syncterm int    // SyncTERM font number, -1 if none
	bits     []byte
	mirror   [3][]int // the character that looks flipped left-right, upside down, turned (flipH, flipV, turnCW)
	h        int      // rows of the glyphs if not the kind's own (rows()): 8 (kept doubled) or 9-16
	src      string   // the file it came from
	format   string   // the kind of file it came from ("psf2", "64c", "petmate:custom_1" ...), see fontfile.go
}

var fonts = func() []*Font {
	list := []struct {
		id, name, file, sauce string
		sync, h               int
	}{
		{"topaz1", "Topaz 1 (A500)", "Topaz_a500_v1.0.raw", "Amiga Topaz 1", 42, 0},
		{"topaz1p", "Topaz 1+ (A500)", "TopazPlus_a500_v1.0.raw", "Amiga Topaz 1+", 40, 0},
		{"topaz2", "Topaz 2 (A1200)", "Topaz_a1200_v1.0.raw", "Amiga Topaz 2", -1, 0},
		{"topaz2p", "Topaz 2+ (A1200)", "TopazPlus_a1200_v1.0.raw", "Amiga Topaz 2+", -1, 0},
		{"mk", "MicroKnight", "MicroKnight_v1.0.raw", "Amiga MicroKnight", 41, 0},
		{"mkp", "MicroKnight+", "MicroKnightPlus_v1.0.raw", "Amiga MicroKnight+", 39, 0},
		{"noodle", "P0T-NOoDLE", "P0T-NOoDLE_v1.0.raw", "Amiga P0T-NOoDLE", 37, 0},
		{"mosoul", "mO'sOul", "mOsOul_v1.0.raw", "Amiga mOsOul", 38, 0},
		{"vga", "IBM VGA (CP437)", "IBM_VGA_8x16.raw", "IBM VGA", 0, 0},
		// VileR's Ultimate Oldschool PC Font Pack, drawn by tools/pcfonts.py.
		{"ega", "IBM EGA 8x14", "IBM_EGA_8x14.f14", "IBM EGA", -1, 14},
		{"vga50", "IBM VGA 8x8 (80x50)", "IBM_VGA_8x8.f08", "IBM VGA50", -1, 8},
		{"vga850", "IBM VGA CP850", "IBM_VGA_8x16_850.f16", "IBM VGA 850", -1, 16},
		{"vga866", "IBM VGA CP866", "IBM_VGA_8x16_866.f16", "IBM VGA 866", -1, 16},
	}
	var out []*Font
	for _, f := range list {
		b, err := fontFiles.ReadFile("fonts/" + f.file)
		if err != nil || (f.h == 0 && len(b) != 4096) || (f.h > 0 && len(b) != 256*f.h) {
			panic("font " + f.file)
		}
		kind := "amiga"
		if strings.HasPrefix(f.file, "IBM") {
			kind = "pc"
		}
		font := &Font{id: f.id, name: f.name, kind: kind, sauce: f.sauce, syncterm: f.sync, bits: b}
		if f.h > 0 {
			font = newFont(f.id, f.name, kind, b, f.h, 256)
			font.sauce, font.syncterm = f.sauce, f.sync
		}
		out = append(out, font)
	}
	return out
}()

// sauceAliases are other SAUCE font names for the built-in fonts (the SAUCE
// standard's "IBM VGA 437" is plain "IBM VGA"; EGA43 is the same 8x8 ROM font as VGA50).
var sauceAliases = map[string]string{"ibm vga 437": "IBM VGA", "ibm ega 437": "IBM EGA", "ibm vga50 437": "IBM VGA50",
	"ibm ega43": "IBM VGA50", "ibm ega43 437": "IBM VGA50"}

// fontBySauce is the page type's font a SAUCE font name means, nil if none.
func fontBySauce(name, mode string) *Font {
	name = strings.TrimSpace(name)
	if al, ok := sauceAliases[strings.ToLower(name)]; ok {
		name = al
	}
	for _, f := range fonts {
		if f.sauce != "" && strings.EqualFold(f.sauce, name) && f.kind == mode {
			return f
		}
	}
	// Names of fonts CharEdit lacks (Moebius has a set per code page, and the 19-row
	// IBM VGA25G): the same size in CP437, so the page keeps its shape.
	l := strings.ToLower(name)
	for _, p := range []struct{ prefix, to string }{{"ibm vga50 ", "IBM VGA50"}, {"ibm ega43 ", "IBM VGA50"},
		{"ibm ega ", "IBM EGA"}, {"ibm vga25g", "IBM VGA"}, {"ibm vga ", "IBM VGA"}} {
		if strings.HasPrefix(l, p.prefix) && !strings.EqualFold(name, p.to) {
			return fontBySauce(p.to, mode)
		}
	}
	return nil
}

func fontByID(id string) *Font {
	for _, f := range fonts {
		if f.id == id {
			return f
		}
	}
	return nil
}

// The ways a block is turned over.
const (
	flipH  = iota // left-right
	flipV         // upside down
	turnCW        // a quarter turn clockwise
)

// flipped gives the character that looks like c mirrored (vertical: upside down);
// c itself if none is close.
func (f *Font) flipped(c int, vertical bool) int {
	if vertical {
		return f.mapped(c, flipV)
	}
	return f.mapped(c, flipH)
}

// turned gives the character that looks like c turned a quarter clockwise.
func (f *Font) turned(c int) int { return f.mapped(c, turnCW) }

func (f *Font) mapped(c, k int) int {
	if f.mirror[k] == nil {
		f.mirror[k] = f.mirrorTable(k)
	}
	return f.mirror[k][c&255]
}

// mirrorTable: the Commodore sets use Marq's hand-made tables (cbmturn.go); the
// rest the known pairs, then the glyph whose pixels match best. Letters and
// digits only change into an exact mirror image (b and d), never into a
// look-alike (C into O); nothing turns into an empty cell.
func (f *Font) mirrorTable(k int) []int {
	t := make([]int, 256)
	if f.kind == "c64" {
		tabs := [3][2]*[256]uint8{{&cbmFlipH, &cbmFlipHLower}, {&cbmFlipV, &cbmFlipVLower}, {&cbmTurn, &cbmTurnLower}}
		lower := 0
		if strings.HasSuffix(f.id, "lower") {
			lower = 1
		}
		for c := range t {
			t[c] = int(tabs[k][lower][c])
		}
		return t
	}
	glyph := func(c int) [16]byte {
		var g [16]byte
		copy(g[:], f.bits[c*16:c*16+16])
		return g
	}
	// Turning needs square cells: fonts of 8 rows (stored doubled) can be turned
	// pixel by pixel, the tall PC fonts only by the known pairs.
	square := true
	for i := 0; i < len(f.bits); i += 2 {
		if f.bits[i] != f.bits[i+1] {
			square = false
			break
		}
	}
	transform := func(g [16]byte) [16]byte {
		var o [16]byte
		switch k {
		case flipV:
			for r := 0; r < 16; r++ {
				o[r] = g[15-r]
			}
		case flipH:
			for r := 0; r < 16; r++ {
				v := g[r]
				var m byte
				for b := 0; b < 8; b++ {
					if v&(1<<b) != 0 {
						m |= 0x80 >> b
					}
				}
				o[r] = m
			}
		case turnCW: // pixel (x, y) of 8x8 goes to (7-y, x); bit 7 is x 0
			for y := 0; y < 8; y++ {
				for x := 0; x < 8; x++ {
					if g[y*2]&(0x80>>x) != 0 {
						o[x*2] |= 0x80 >> (7 - y)
					}
				}
			}
			for r := 0; r < 16; r += 2 {
				o[r+1] = o[r]
			}
		}
		return o
	}
	ink := func(g [16]byte) int {
		n := 0
		for _, v := range g {
			n += bits.OnesCount8(v)
		}
		return n
	}
	diff := func(x, y [16]byte) int {
		n := 0
		for r := range x {
			n += bits.OnesCount8(x[r] ^ y[r])
		}
		return n
	}
	// Line characters sit a pixel off the middle of an 8-pixel cell, so their
	// mirror images match only one pixel over: try the flipped glyph moved by one.
	moved := func(g [16]byte, dx, dy int) [16]byte {
		var o [16]byte
		for r := range g {
			if src := r - dy; src >= 0 && src < 16 {
				o[r] = g[src]
				if dx > 0 {
					o[r] >>= dx
				} else if dx < 0 {
					o[r] <<= -dx
				}
			}
		}
		return o
	}
	var shifts [][2]int
	switch k {
	case flipH:
		shifts = [][2]int{{-1, 0}, {1, 0}}
	case flipV:
		shifts = [][2]int{{0, -2}, {0, -1}, {0, 1}, {0, 2}} // rows: one row of an 8-row font is two here
	default:
		shifts = [][2]int{{-1, 0}, {1, 0}, {0, -2}, {0, 2}}
	}
	near := func(want, g [16]byte) int {
		d := diff(want, g)
		for _, s := range shifts {
			if e := diff(moved(want, s[0], s[1]), g) + 1; e < d { // a moved match counts a little worse
				d = e
			}
		}
		return d
	}
	known := map[int]int{}
	for _, c := range symmetric[f.kind][k] {
		known[c] = c
	}
	for _, p := range mirrorPairs[f.kind][k] {
		if k == turnCW { // a cycle: each turns into the next
			for i, c := range p {
				known[c] = p[(i+1)%len(p)]
			}
			continue
		}
		known[p[0]], known[p[1]] = p[1], p[0]
	}
	half := 256
	if f.kind == "atari" {
		half = 128 // 128-255 are 0-127 inverted
	}
	for c := 0; c < half; c++ {
		if m, ok := known[c]; ok {
			t[c] = m
			continue
		}
		t[c] = c
		if (k == turnCW && !square) || f.kind == "tt" {
			continue
		}
		want := transform(glyph(c))
		if near(want, glyph(c)) <= 1 { // looks the same turned over
			continue
		}
		best, bestD := c, 1<<30
		for o := 0; o < half; o++ {
			if d := near(want, glyph(o)); d < bestD {
				best, bestD = o, d
			}
		}
		// Rows are doubled, so one pixel off is a difference of 2; allow a little,
		// but only an exact match for letters, digits and signs.
		limit := 6
		if f.isText(c) || f.isText(best) {
			limit = 1
		}
		if bestD > limit || (ink(glyph(best)) == 0) != (ink(glyph(c)) == 0) {
			best = c
		}
		t[c] = best
	}
	if k != turnCW { // a flip undone by flipping again: keep only pairs that agree
		for c := 0; c < half; c++ {
			if t[t[c]] != c {
				t[c] = c
			}
		}
	}
	for c := half; c < 256; c++ {
		t[c] = t[c-half] + half
	}
	return t
}

// isText: is c a letter, digit or sign in this font (as opposed to a graphics character)?
func (f *Font) isText(c int) bool {
	switch f.kind {
	case "atari":
		c &= 127
		return c >= 0x21 && c <= 0x5F || c >= 0x61 && c <= 0x7A
	case "pc", "st":
		return c >= 0x21 && c <= 0x7E || c >= 0x80 && c <= 0xA8 || c >= 0xE0 && c <= 0xEF
	case "amiga":
		return c >= 0x21 && c <= 0x7E || c >= 0xA1
	}
	return c >= 0x21 && c <= 0x7E
}

// mirrorPairs are the pairs everyone knows, used before comparing pixels
// (the fonts' line characters are not always exact mirror images). For turnCW
// each entry is a cycle: every character turns into the next.
var asciiPairs = [3][][]int{
	flipH:  {{'(', ')'}, {'[', ']'}, {'{', '}'}, {'<', '>'}, {'/', '\\'}},
	flipV:  {{'/', '\\'}, {'^', 'v'}},
	turnCW: {{'-', '|'}, {'/', '\\'}, {'^', '>', 'v', '<'}},
}

// symmetric are characters that look the same turned over, though their pixels
// sit a little off the middle (CP437's lines and blocks).
var symmetric = map[string][3][]int{
	"pc": {
		flipH:  {0xB0, 0xB1, 0xB2, 0xB3, 0xBA, 0xC1, 0xC2, 0xC4, 0xC5, 0xCA, 0xCB, 0xCD, 0xCE, 0xCF, 0xD0, 0xD1, 0xD2, 0xD7, 0xD8, 0xDB, 0xDC, 0xDF},
		flipV:  {0xB0, 0xB1, 0xB2, 0xB3, 0xB4, 0xB5, 0xB6, 0xB9, 0xBA, 0xC3, 0xC4, 0xC5, 0xC6, 0xC7, 0xCC, 0xCD, 0xCE, 0xD7, 0xD8, 0xDB, 0xDD, 0xDE},
		turnCW: {0xB0, 0xB1, 0xB2, 0xC5, 0xCE, 0xDB},
	},
}

var mirrorPairs = map[string][3][][]int{
	"amiga": {
		flipH:  append([][]int{{0xAB, 0xBB}}, asciiPairs[flipH]...),
		flipV:  append([][]int{{'_', 0xAF}}, asciiPairs[flipV]...),
		turnCW: asciiPairs[turnCW],
	},
	"pc": {
		flipH: append([][]int{{0xAE, 0xAF}, {0x10, 0x11}, {0x1A, 0x1B},
			{0xDA, 0xBF}, {0xC0, 0xD9}, {0xC3, 0xB4}, {0xC9, 0xBB}, {0xC8, 0xBC}, {0xCC, 0xB9}, {0xD5, 0xB8},
			{0xD4, 0xBE}, {0xD6, 0xB7}, {0xD3, 0xBD}, {0xC6, 0xB5}, {0xC7, 0xB6}, {0xDD, 0xDE}}, asciiPairs[flipH]...),
		flipV: append([][]int{{0x1E, 0x1F}, {0x18, 0x19}, {0xDC, 0xDF}, {0xDA, 0xC0}, {0xBF, 0xD9}, {0xC2, 0xC1},
			{0xC9, 0xC8}, {0xBB, 0xBC}, {0xCB, 0xCA}, {0xD5, 0xD4}, {0xB8, 0xBE}, {0xD6, 0xD3}, {0xB7, 0xBD},
			{0xD1, 0xCF}, {0xD2, 0xD0}}, asciiPairs[flipV]...),
		// CP437's lines, corners, double lines, half blocks, arrows and triangles.
		turnCW: append([][]int{{0xC4, 0xB3}, {0xCD, 0xBA}, {0xD8, 0xD7}, {0x12, 0x1D},
			{0xDA, 0xBF, 0xD9, 0xC0}, {0xC3, 0xC2, 0xB4, 0xC1}, {0xC9, 0xBB, 0xBC, 0xC8}, {0xCC, 0xCB, 0xB9, 0xCA},
			{0xD5, 0xB7, 0xBE, 0xD3}, {0xD6, 0xB8, 0xBD, 0xD4}, {0xC6, 0xD2, 0xB5, 0xD0}, {0xC7, 0xD1, 0xB6, 0xCF},
			{0xDF, 0xDE, 0xDC, 0xDD}, {0x18, 0x1A, 0x19, 0x1B}, {0x1E, 0x10, 0x1F, 0x11}}, asciiPairs[turnCW]...),
	},
	"st":    asciiPairs,
	"atari": asciiPairs,
	"zx":    asciiPairs,
	"tt":    asciiPairs,
}
