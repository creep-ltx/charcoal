package main

import (
	"crypto/sha256"
	"fmt"
	"testing"
)

// The PNG export gives the same picture as before renderDoc was factored out of it.
func TestPNGExportUnchanged(t *testing.T) {
	withFonts(t, true)
	want := map[string]string{
		"pcpng":     "72785a228194a965",
		"pcpng2":    "313fcf055261d0c1",
		"amigapng":  "64749839035ab8e5",
		"amigapng2": "55e7f32684df95c2",
		"c64png":    "5028ddc5d9d10f57",
		"c64png2":   "1ad74a2c9afc0660",
		"ataripng":  "fe351378bf116171",
		"ataripng2": "b8105619930065ec",
		"zxpng":     "68c7f5a2b0fbefba",
		"zxpng2":    "000e5e69fb6ea821",
		"ttpng":     "79f413ae6c9de7ef",
		"ttpng2":    "484a7bd76d867517",
		"stpng":     "f208dab0c61d4143",
		"stpng2":    "720633d1f0230125",
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
			got := fmt.Sprintf("%x", sha256.Sum256(b))[:16]
			if w, ok := want[mode+f]; ok && w != got {
				t.Errorf("%s %s: %s, want %s", mode, f, got, w)
			}
		}
	}
}
