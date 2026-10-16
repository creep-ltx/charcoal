package main

// Checks against the real programs' readers, run only when they are there:
//
//	CHARCOAL_ICYDUMP="/tmp/cc-icy/sandbox.sh target/release/examples/ccdump" go test -run External .
//	CHARCOAL_ANSILOVE=1 go test -run External .
//	CHARCOAL_PLAYSCII="/tmp/cc-icy/sandbox.sh python3 -I /tmp/cc-icy/scripts/psciload.py /tmp/cc-icy/psci" go test -run External .
//
// (CHARCOAL_ICYDIR: a folder the sandbox can read, for the files written.)
//
// ccdump is a small program on IcyDraw's engine (icy_tools, crates/icy_engine)
// that prints the cells of a file as IcyDraw reads them; see the knowledge note
// the knowledge base (amiga/text-art-editor-formats.md) for how it was built (in a
// sandbox without network).

import (
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

type icyDumpCell struct {
	ch     int
	fg, bg string
	attr   int
}

// icyDump runs ccdump on a file: its size, palette, font and cells.
func icyDump(t *testing.T, cmd, path string) (w, h int, ice string, pal []uint32, font []byte, cells []icyDumpCell) {
	t.Helper()
	args := append(strings.Fields(cmd), path)
	out, err := exec.Command(args[0], args[1:]...).Output()
	if err != nil {
		t.Fatalf("ccdump %s: %v\n%s", path, err, out)
	}
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		switch f[0] {
		case "SIZE":
			w, _ = strconv.Atoi(f[1])
			h, _ = strconv.Atoi(f[2])
			ice = f[3]
		case "PALETTE":
			for _, c := range f[1:] {
				v, _ := strconv.ParseUint(c, 16, 32)
				pal = append(pal, uint32(v))
			}
		case "FONT":
			for i := 0; i+1 < len(f[1]); i += 2 {
				v, _ := strconv.ParseUint(f[1][i:i+2], 16, 8)
				font = append(font, byte(v))
			}
		case "ROW":
			for _, c := range f[1:] {
				p := strings.Split(c, ":")
				ch, _ := strconv.ParseInt(p[0], 16, 32)
				at, _ := strconv.ParseInt(p[3], 16, 32)
				cells = append(cells, icyDumpCell{int(ch), p[1], p[2], int(at)})
			}
		case "ERROR":
			t.Fatalf("IcyDraw cannot read %s: %s", path, line)
		}
	}
	return
}

// icyRGB is the colour of a dumped cell colour.
func icyRGB(c string, pal []uint32) uint32 {
	switch c[0] {
	case 'P':
		n, _ := strconv.Atoi(c[1:])
		if n < len(pal) {
			return pal[n]
		}
	case 'R':
		v, _ := strconv.ParseUint(c[1:], 16, 32)
		return uint32(v)
	}
	return 1 << 31
}

func TestExternalIcyDrawReads(t *testing.T) {
	cmd := os.Getenv("CHARCOAL_ICYDUMP")
	if cmd == "" {
		t.Skip("CHARCOAL_ICYDUMP not set")
	}
	dir := os.Getenv("CHARCOAL_ICYDIR") // a folder the sandbox can read
	if dir == "" {
		dir = t.TempDir()
	}
	r := rand.New(rand.NewSource(11))
	for _, ice := range []bool{true, false} {
		d, pal, f := edPage(r, 80, 12), edPalette(r), edFont(r)
		files := map[string][]byte{"idf": writeIDF(d, pal, f), "icy": writeIcy(d, pal, f, ice, nil), "xp": writeXP(d, pal, ice)}
		files["adf"], _ = writeADF(d, pal, f)
		files["tnd"], _ = writeTundra(d, pal, ice)
		for ext, b := range files {
			p := filepath.Join(dir, fmt.Sprintf("page-%v.%s", ice, ext))
			if err := os.WriteFile(p, b, 0o644); err != nil {
				t.Fatal(err)
			}
			w, h, mode, ipal, font, cells := icyDump(t, cmd, p)
			if w != 80 || h != 12 || len(cells) != 80*12 {
				t.Fatalf("%s: IcyDraw reads %dx%d, %d cells", p, w, h, len(cells))
			}
			if ext == "idf" || ext == "adf" || ext == "icy" {
				for i := range f.bits {
					if font[i] != f.bits[i] {
						t.Fatalf("%s: font byte %d", p, i)
					}
				}
			}
			if ext == "icy" && (mode == "Ice") != ice {
				t.Fatalf("%s: iCE mode %s", p, mode)
			}
			for i, v := range d.cells {
				c := cells[i]
				// The cell as the page shows it; an iCE format shows backgrounds 8-15 as colours.
				shownIce := ice || ext == "idf" || ext == "adf"
				fg, bg := pal[cellFg(v)]&0xFFFFFF, pal[shownBg(v, shownIce)]&0xFFFFFF
				gotBg := icyRGB(c.bg, ipal)
				if ext == "icy" && !ice && cellBg(v) > 7 && c.attr&icyBlinkAttr == 0 {
					t.Fatalf("%s cell %d: not blinking", p, i)
				}
				if c.ch != cellCh(v) || icyRGB(c.fg, ipal) != fg || gotBg != bg {
					t.Fatalf("%s cell %d,%d: %02x %06x/%06x, IcyDraw reads %+v (%06x/%06x)", p, i%80, i/80, cellCh(v), fg, bg, c, icyRGB(c.fg, ipal), gotBg)
				}
			}
		}
	}
}

// ansilove draws the iCEDraw, ArtWorx and TundraDraw files as CharEdit draws the page.
func TestExternalAnsilove(t *testing.T) {
	if os.Getenv("CHARCOAL_ANSILOVE") == "" {
		t.Skip("CHARCOAL_ANSILOVE not set")
	}
	a := testApp(t, "pc")
	a.ice = true
	r := rand.New(rand.NewSource(12))
	d, pal := edPage(r, 80, 10), edPalette(r)
	for i, v := range d.cells { // ansilove draws neither 1, 2, 4 nor 6 from a .tnd (its reader)
		if c := cellCh(v); c == 1 || c == 2 || c == 4 || c == 6 {
			d.cells[i] = makeCell(c+0x40, cellFg(v), cellBg(v), 0)
		}
	}
	f := fontByID("vga")
	dir := t.TempDir()
	for _, ext := range []string{"idf", "adf", "tnd", "idf+sauce"} {
		var b []byte
		switch ext {
		case "idf":
			b = writeIDF(d, pal, f)
		case "idf+sauce": // ansilove takes 129 bytes off the end: a record without comments
			b = writeIDF(d, pal, f)
			a.info = Info{writeSauce: true, title: "t"}
			b = append(b, a.editorSauce(d, 1, len(b))...)
			ext = "idf"
		case "adf":
			b, _ = writeADF(d, pal, f)
		case "tnd":
			b, _ = writeTundra(d, pal, true)
		}
		p := filepath.Join(dir, "a."+ext)
		os.WriteFile(p, b, 0o644)
		if out, err := exec.Command("ansilove", "-o", p+".png", p).CombinedOutput(); err != nil {
			t.Fatalf("ansilove %s: %v %s", ext, err, out)
		}
		fp, err := os.Open(p + ".png")
		if err != nil {
			t.Fatal(err)
		}
		img, err := png.Decode(fp)
		fp.Close()
		if err != nil {
			t.Fatal(err)
		}
		a.doc, a.pal = d, pal
		fb := a.renderDoc(d, 1)
		if img.Bounds() != image.Rect(0, 0, fb.w, fb.h) {
			t.Fatalf("%s: ansilove draws %v, CharEdit %dx%d", ext, img.Bounds(), fb.w, fb.h)
		}
		bad := 0
		for y := 0; y < fb.h; y++ {
			for x := 0; x < fb.w; x++ {
				R, G, B, _ := img.At(x, y).RGBA()
				got := (R>>8)<<16 | (G>>8)<<8 | B>>8
				if ext == "tnd" {
					// A .tnd has no font: ansilove draws with its own VGA font, whose
					// glyphs differ from CharEdit's in places. The colours must be the cell's.
					v := d.get(x/8, y/16)
					if got != pal[cellFg(v)]&0xFFFFFF && got != pal[cellBg(v)]&0xFFFFFF {
						bad++
					}
					continue
				}
				if got != fb.px[y*fb.w+x]&0xFFFFFF {
					bad++
				}
			}
		}
		if bad > 0 {
			t.Errorf("%s: %d pixels differ from CharEdit's picture", ext, bad)
		}
	}
}

// Playscii's own loader (art.py ArtFromDisk, with its editor parts stubbed) reads
// the frames, delays and tiles as written.
func TestExternalPlayscii(t *testing.T) {
	cmd := os.Getenv("CHARCOAL_PLAYSCII")
	if cmd == "" {
		t.Skip("CHARCOAL_PLAYSCII not set")
	}
	dir := os.Getenv("CHARCOAL_ICYDIR")
	if dir == "" {
		dir = t.TempDir()
	}
	r := rand.New(rand.NewSource(13))
	for _, page := range []psciPage{{mode: "pc", ice: true}, {mode: "c64", machine: "c64", lower: true}, {mode: "atari"}, {mode: "amiga", ice: true}} {
		page.pal = map[string][16]uint32{"atari": atariPalette.colors}[page.mode]
		if page.mode != "atari" {
			page.pal = paletteByID("vga").colors
		}
		var docs []*Doc
		for k := 0; k < 2; k++ {
			d := edPage(r, 13, 4)
			d.bg, d.ink = 6, 3
			docs = append(docs, d)
		}
		out, _ := writePsci(docs, []int{100, 1500}, page)
		var f psciFile
		json.Unmarshal(out, &f)
		p := filepath.Join(dir, "page-"+page.mode+".psci")
		os.WriteFile(p, out, 0o644)
		args := append(strings.Fields(cmd), p)
		got, err := exec.Command(args[0], args[1:]...).CombinedOutput()
		if err != nil {
			t.Fatalf("playscii %s: %v\n%s", page.mode, err, got)
		}
		lines := strings.Split(strings.TrimSpace(string(got)), "\n")
		if want := fmt.Sprintf("FILE %s True 13 4 2 1 %s %s [0.1, 1.5]", p, f.Charset, f.Palette); lines[0] != want {
			t.Fatalf("playscii reads %q, want %q", lines[0], want)
		}
		for k := 0; k < 2; k++ {
			var arr [3][]int
			json.Unmarshal([]byte(strings.SplitN(lines[1+k], " ", 4)[3]), &arr)
			for i, tl := range f.Frames[k].Layers[0].Tiles {
				if arr[0][i] != int(tl.Char) || arr[1][i] != int(tl.Fg) || arr[2][i] != int(tl.Bg) {
					t.Fatalf("%s frame %d tile %d: playscii reads %d/%d/%d, written %v", page.mode, k, i, arr[0][i], arr[1][i], arr[2][i], tl)
				}
			}
		}
	}
}
