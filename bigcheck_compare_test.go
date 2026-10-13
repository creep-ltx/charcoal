//go:build bigcheck

package main

import (
	"fmt"
	"image"
	_ "image/png"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func nearest(r, g, b uint32) int {
	best, bd := 0, uint32(1<<31)
	for i, c := range c64Palette.colors {
		cr, cg, cb := c>>16&255, c>>8&255, c&255
		d := (cr-r)*(cr-r) + (cg-g)*(cg-g) + (cb-b)*(cb-b)
		if int32(cr) < int32(r) {
			d = (r-cr)*(r-cr) + (cg-g)*(cg-g) + (cb-b)*(cb-b)
		}
		dr := int(cr) - int(r)
		dg := int(cg) - int(g)
		db := int(cb) - int(b)
		d = uint32(dr*dr + dg*dg + db*db)
		if d < bd {
			best, bd = i, d
		}
	}
	return best
}

func TestBigCheckCompare(t *testing.T) {
	os.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a = &App{tool: "draw", fg: 7, brush: 0x2F, charHover: -1, layout: "one"}
	a.hasC64 = loadC64Fonts()
	loadPrefs()
	pngs, _ := filepath.Glob("/tmp/bigcheck/nas-png/*.png")
	type res struct {
		name   string
		shape  int
		colour int
		cells  int
	}
	var rs []res
	perfect := 0
	for _, pp := range pngs {
		base := strings.TrimSuffix(filepath.Base(pp), ".png")
		var src string
		for _, ext := range []string{".c", ".seq", ".prg"} {
			if _, err := os.Stat("/tmp/bigcheck/files/nas-petscii/" + base + ext); err == nil {
				src = "/tmp/bigcheck/files/nas-petscii/" + base + ext
			}
		}
		if src == "" || strings.HasSuffix(src, ".seq") {
			continue
		}
		f, _ := os.Open(pp)
		img, _, err := image.Decode(f)
		f.Close()
		if err != nil {
			continue
		}
		b, _ := os.ReadFile(src)
		a.tabs, a.Tab = nil, nil
		a.focusTab(a.newTab("c64"))
		a.loadBytes(b, filepath.Base(src), src, "auto")
		if a.mode != "c64" || a.doc.cols != 40 {
			continue
		}
		scale := img.Bounds().Dx() / 320
		font := a.font()
		r := res{name: base}
		for cy := 0; cy < 25; cy++ {
			for cx := 0; cx < 40; cx++ {
				v := a.doc.get(cx, cy)
				ch, fg, bg := cellCh(v), cellFg(v), a.doc.bg
				shapeBad, colBad := false, false
				for py := 0; py < 8; py++ {
					bits := font.bits[ch*16+py*2]
					for px := 0; px < 8; px++ {
						want := bg
						if bits&(0x80>>px) != 0 {
							want = fg
						}
						R, G, B, _ := img.At((cx*8+px)*scale, (cy*8+py)*scale).RGBA()
						got := nearest(R>>8, G>>8, B>>8)
						if got != want {
							// shape: is the pixel ink in theirs where ours is ink?
							if (got == bg) != (want == bg) {
								shapeBad = true
							} else {
								colBad = true
							}
						}
					}
				}
				r.cells++
				if shapeBad {
					r.shape++
				} else if colBad {
					r.colour++
				}
			}
		}
		if r.shape == 0 && r.colour == 0 {
			perfect++
		}
		rs = append(rs, r)
	}
	sort.Slice(rs, func(i, j int) bool { return rs[i].shape+rs[i].colour > rs[j].shape+rs[j].colour })
	out := fmt.Sprintf("%d compared, %d identical\n", len(rs), perfect)
	for _, r := range rs[:min(25, len(rs))] {
		out += fmt.Sprintf("%-50s shape %4d colour %4d of %d cells\n", r.name, r.shape, r.colour, r.cells)
	}
	os.WriteFile("/tmp/bigcheck/compare.txt", []byte(out), 0o644)
}
