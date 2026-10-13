package main

// The 8x16 bitmap fonts: 256 glyphs of 16 bytes (bit 7 = the left pixel),
// indexed by the byte: Latin-1 for the Amiga fonts, CP437 for the IBM VGA font.
// The Amiga fonts are TrueSchool's raw conversions (8x8 with every row doubled);
// IBM_VGA_8x16.raw was rendered from Px437_IBM_VGA_8x16.ttf by the old web tools.

import "embed"

//go:embed fonts/*.raw
var fontFiles embed.FS

type Font struct {
	id, name string
	kind     string // the page it belongs to: "amiga", "pc" or "c64"
	sauce    string // SAUCE font name (TInfoS)
	syncterm int    // SyncTERM font number, -1 if none
	bits     []byte
	mirror   [2][]int // the character that looks flipped left-right [0] and upside down [1]
}

var fonts = func() []*Font {
	list := []struct {
		id, name, file, sauce string
		sync                  int
	}{
		{"topaz1", "Topaz 1 (A500)", "Topaz_a500_v1.0.raw", "Amiga Topaz 1", 42},
		{"topaz1p", "Topaz 1+ (A500)", "TopazPlus_a500_v1.0.raw", "Amiga Topaz 1+", 40},
		{"topaz2", "Topaz 2 (A1200)", "Topaz_a1200_v1.0.raw", "Amiga Topaz 2", -1},
		{"topaz2p", "Topaz 2+ (A1200)", "TopazPlus_a1200_v1.0.raw", "Amiga Topaz 2+", -1},
		{"mk", "MicroKnight", "MicroKnight_v1.0.raw", "Amiga MicroKnight", 41},
		{"mkp", "MicroKnight+", "MicroKnightPlus_v1.0.raw", "Amiga MicroKnight+", 39},
		{"noodle", "P0T-NOoDLE", "P0T-NOoDLE_v1.0.raw", "Amiga P0T-NOoDLE", 37},
		{"mosoul", "mO'sOul", "mOsOul_v1.0.raw", "Amiga mOsOul", 38},
		{"vga", "IBM VGA (CP437)", "IBM_VGA_8x16.raw", "IBM VGA", 0},
	}
	var out []*Font
	for _, f := range list {
		b, err := fontFiles.ReadFile("fonts/" + f.file)
		if err != nil || len(b) != 4096 {
			panic("font " + f.file)
		}
		out = append(out, &Font{id: f.id, name: f.name, kind: map[bool]string{true: "pc", false: "amiga"}[f.id == "vga"], sauce: f.sauce, syncterm: f.sync, bits: b})
	}
	return out
}()

func fontByID(id string) *Font {
	for _, f := range fonts {
		if f.id == id {
			return f
		}
	}
	return nil
}

// flipped gives the character that looks like c mirrored (vertical: upside down),
// found by comparing the glyphs' pixels; c itself if none is close.
func (f *Font) flipped(c int, vertical bool) int {
	k := 0
	if vertical {
		k = 1
	}
	if f.mirror[k] == nil {
		f.mirror[k] = f.mirrorTable(vertical)
	}
	return f.mirror[k][c&255]
}

func (f *Font) mirrorTable(vertical bool) []int {
	glyph := func(c int) [16]byte {
		var g [16]byte
		copy(g[:], f.bits[c*16:c*16+16])
		return g
	}
	flip := func(g [16]byte) [16]byte {
		var o [16]byte
		for r := 0; r < 16; r++ {
			if vertical {
				o[r] = g[15-r]
			} else {
				v := g[r]
				var m byte
				for b := 0; b < 8; b++ {
					if v&(1<<b) != 0 {
						m |= 0x80 >> b
					}
				}
				o[r] = m
			}
		}
		return o
	}
	diff := func(x, y [16]byte) int {
		n := 0
		for r := range x {
			v := x[r] ^ y[r]
			for ; v != 0; v &= v - 1 {
				n++
			}
		}
		return n
	}
	// Line characters sit a pixel off the middle of an 8-pixel cell, so their
	// mirror images match only one pixel over: try the flipped glyph moved by one.
	moved := func(g [16]byte, s int) [16]byte {
		var o [16]byte
		for r := range g {
			switch {
			case vertical && s != 0:
				if src := r - s; src >= 0 && src < 16 {
					o[r] = g[src]
				}
			case s > 0:
				o[r] = g[r] >> 1
			case s < 0:
				o[r] = g[r] << 1
			default:
				o[r] = g[r]
			}
		}
		return o
	}
	near := func(want [16]byte, g [16]byte) int {
		d := diff(want, g)
		shifts := []int{-1, 1}
		if vertical {
			shifts = []int{-2, -1, 1, 2} // rows: one row of an 8-row font is two here
		}
		for _, s := range shifts {
			if e := diff(moved(want, s), g) + 1; e < d { // a moved match counts a little worse
				d = e
			}
		}
		return d
	}
	t := make([]int, 256)
	known := map[int]int{}
	for _, p := range mirrorPairs[f.kind][vertical] {
		known[p[0]], known[p[1]] = p[1], p[0]
	}
	for c := 0; c < 256; c++ {
		if m, ok := known[c]; ok {
			t[c] = m
			continue
		}
		want := flip(glyph(c))
		best, bestD := c, near(want, glyph(c))
		if bestD <= 1 { // looks the same flipped
			t[c] = c
			continue
		}
		for o := 0; o < 256; o++ {
			if d := near(want, glyph(o)); d < bestD {
				best, bestD = o, d
			}
		}
		// Rows are doubled, so one pixel off is a difference of 2; allow a little.
		if bestD > 6 {
			best = c
		}
		t[c] = best
	}
	return t
}

// mirrorPairs are the pairs everyone knows, used before comparing pixels
// (the fonts' line characters are not always exact mirror images).
var mirrorPairs = map[string]map[bool][][2]int{
	"amiga": {
		false: {{'(', ')'}, {'[', ']'}, {'{', '}'}, {'<', '>'}, {'/', '\\'}, {0xAB, 0xBB}},
		true:  {{'/', '\\'}, {'_', 0xAF}, {'^', 'v'}},
	},
	"pc": {
		false: {{'(', ')'}, {'[', ']'}, {'{', '}'}, {'<', '>'}, {'/', '\\'}, {0xAE, 0xAF}, {0x10, 0x11}, {0x1A, 0x1B},
			{0xDA, 0xBF}, {0xC0, 0xD9}, {0xC3, 0xB4}, {0xC9, 0xBB}, {0xC8, 0xBC}, {0xCC, 0xB9}, {0xD5, 0xB8},
			{0xD4, 0xBE}, {0xD6, 0xB7}, {0xD3, 0xBD}, {0xC6, 0xB5}, {0xC7, 0xB6}, {0xDD, 0xDE}},
		true: {{'/', '\\'}, {0x1E, 0x1F}, {0x18, 0x19}, {0xDC, 0xDF}, {0xDA, 0xC0}, {0xBF, 0xD9}, {0xC2, 0xC1},
			{0xC9, 0xC8}, {0xBB, 0xBC}, {0xCB, 0xCA}, {0xD5, 0xD4}, {0xB8, 0xBE}, {0xD6, 0xD3}, {0xB7, 0xBD},
			{0xD1, 0xCF}, {0xD2, 0xD0}},
	},
}
