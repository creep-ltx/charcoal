package main

import (
	"os"
	"testing"
)

// The same art in five formats (from ansilove's tests) must give the same page.
func TestFiveFormats(t *testing.T) {
	os.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a = &App{tool: "draw", fg: 7, brush: 0x2F, charHover: -1, layout: "one"}
	loadPrefs()
	type page struct {
		d   *Doc
		pal [16]uint32
	}
	pages := map[string]page{}
	for _, e := range []string{"xb", "bin", "pcb", "adf", "tnd"} {
		b, err := os.ReadFile("testdata/alove/bs-alove." + e)
		if err != nil {
			t.Fatal(err)
		}
		a.tabs, a.Tab = nil, nil
		a.focusTab(a.newTab("pc"))
		a.loadBytes(b, "bs-alove."+e, "", "auto")
		a.ice = true
		a.setPalette()
		pages[e] = page{a.doc, a.pal}
	}
	near := func(x, y uint32) bool {
		for _, sh := range []uint{0, 8, 16} {
			d := int(x>>sh&255) - int(y>>sh&255)
			if d > 8 || d < -8 {
				return false
			}
		}
		return true
	}
	ref := pages["xb"]
	for _, e := range []string{"bin", "pcb", "adf", "tnd"} {
		p, bad := pages[e], 0
		for y := 0; y < ref.d.usedRows(); y++ {
			for x := 0; x < ref.d.cols; x++ {
				r, v := ref.d.get(x, y), p.d.get(x, y)
				rc, vc := cellCh(r)|32*b2i(cellCh(r) == 0), cellCh(v)|32*b2i(cellCh(v) == 0)
				if rc != vc {
					if e == "tnd" && (rc == 1 || rc == 2 || rc == 4 || rc == 6) { // TundraDraw cannot store these: they are its commands
						continue
					}
					if e == "tnd" && y == 58 { // the TundraDraw file has this row's two blocks one row lower
						continue
					}
					t.Logf("%s %d,%d: %02x vs %02x", e, x, y, rc, vc)
					bad++
					continue
				}
				if e == "pcb" && cellAttr(r) == 0 {
					continue // the file writes black on black as @X00, which keeps the colour on PCBoard (pcformats.go)
				}
				if (rc != 32 && !near(ref.pal[cellFg(r)], p.pal[cellFg(v)])) || !near(ref.pal[cellBg(r)], p.pal[cellBg(v)]) {
					bad++
				}
			}
		}
		if bad > 0 {
			t.Errorf("%s: %d cells differ from the XBin", e, bad)
		}
	}
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}
