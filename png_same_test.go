package main

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"image/png"
	"testing"
)

// The PNG export gives the same pixels (not bytes: zlib differs between Go versions) as before renderDoc was factored out of it.
func TestPNGExportUnchanged(t *testing.T) {
	withFonts(t, true)
	want := map[string]string{
		"pcpng":     "27173c14d4f5e8f7",
		"pcpng2":    "bd7026d698156537",
		"amigapng":  "cd9a1a654f9c0d96",
		"amigapng2": "7fc4461b34e0fe5c",
		"c64png":    "5028ddc5d9d10f57",
		"c64png2":   "1ad74a2c9afc0660",
		"ataripng":  "e7777fec6d995ff2", // GTIA register colours since 2026-10-16 (gtia.go)
		"ataripng2": "6ff248e515ca4e7b",
		"zxpng":     "7c4ceb4be0ac6498",
		"zxpng2":    "4745b6d35ab871a2",
		"ttpng":     "15fcee4a4128e027",
		"ttpng2":    "40ad4b9eb643e626",
		"stpng":     "4e18b744559e9e57",
		"stpng2":    "bd96e6b8485b2f66",
	}
	for _, mode := range []string{"pc", "amiga", "atari", "zx", "tt", "st"} {
		a := testApp(t, mode)
		a.hasC64 = true
		for i := 0; i < 20; i++ {
			a.doc.set(i, i%5, makeCell(65+i, i%8, (i/3)%8, 0))
		}
		a.doc.bg, a.doc.border = 6, 14
		for _, f := range []string{"png", "png2"} {
			b, err := a.encode(f)
			if err != nil {
				t.Fatal(err)
			}
			img, err := png.Decode(bytes.NewReader(b))
			if err != nil {
				t.Fatal(err)
			}
			h := sha256.New()
			r := img.Bounds()
			for y := r.Min.Y; y < r.Max.Y; y++ {
				for x := r.Min.X; x < r.Max.X; x++ {
					cr, cg, cb, _ := img.At(x, y).RGBA()
					h.Write([]byte{byte(cr >> 8), byte(cg >> 8), byte(cb >> 8)})
				}
			}
			got := fmt.Sprintf("%x", h.Sum(nil))[:16]
			if w, ok := want[mode+f]; ok && w != got {
				t.Errorf("%s %s: %s, want %s", mode, f, got, w)
			}
		}
	}
}
