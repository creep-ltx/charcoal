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
	amiga    bool
	sauce    string // SAUCE font name (TInfoS)
	syncterm int    // SyncTERM font number, -1 if none
	bits     []byte
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
		out = append(out, &Font{id: f.id, name: f.name, amiga: f.id != "vga", sauce: f.sauce, syncterm: f.sync, bits: b})
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
