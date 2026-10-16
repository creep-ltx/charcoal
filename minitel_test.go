package main

import (
	"bytes"
	"crypto/sha1"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// mtFRandom is a page of random cells a Minitel can show: mosaics, letters
// (G0 and the extras) and spaces with zone attributes, no big letters.
func mtFRandom(r *rand.Rand) *Doc {
	d := newMTDoc()
	for i := range d.cells {
		fg, bg := r.Intn(8), r.Intn(8)
		switch r.Intn(4) {
		case 0:
			d.cells[i] = mtBlank
		case 1:
			var f uint32 = mtG1
			if r.Intn(3) == 0 {
				f |= mtLine
			}
			if r.Intn(5) == 0 {
				f |= mtBlink
			}
			d.cells[i] = makeCell(mtCode(r.Intn(64)), fg, bg, f)
		case 2:
			c := 0x21 + r.Intn(0x5F)
			if r.Intn(6) == 0 {
				c = 0x80 + r.Intn(len(mtExtra))
			}
			var f uint32
			if r.Intn(4) == 0 {
				f |= mtInv
			}
			if r.Intn(5) == 0 {
				f |= mtBlink
			}
			d.cells[i] = makeCell(c, fg, 0, f)
		default:
			var f uint32
			if r.Intn(3) == 0 {
				f |= mtLine
			}
			if r.Intn(4) == 0 {
				f |= mtMask
			}
			if r.Intn(4) == 0 {
				f |= mtInv
			}
			d.cells[i] = makeCell(0x20, fg, bg, f)
		}
	}
	return d
}

// mtFDiff says where two settled pages differ.
func mtFDiff(want, got *Doc) string {
	var out []string
	for y := 0; y < 25; y++ {
		for x := 0; x < 40; x++ {
			if a, b := want.get(x, y), got.get(x, y); a != b {
				out = append(out, fmt.Sprintf("(%d,%d) want %08X got %08X", x, y, a, b))
			}
		}
	}
	if len(out) > 6 {
		out = append(out[:6], fmt.Sprintf("... %d cells", len(out)))
	}
	return strings.Join(out, "; ")
}

func TestMinitelFont(t *testing.T) {
	h := sha1.New()
	for _, g := range mtG0Glyphs {
		h.Write(g[:])
	}
	for _, g := range mtExtraGlyphs {
		h.Write(g[:])
	}
	if len(mtExtraGlyphs) != len(mtExtra) {
		t.Fatalf("%d extra glyphs for %d extras", len(mtExtraGlyphs), len(mtExtra))
	}
	// 'A' in the EF9345's shape: 5 dots wide, a row down from the top.
	if g := mtG0Glyphs['A'-0x20]; g[1] != 0x38 || g[5] != 0x7C || g[0] != 0 {
		t.Errorf("A: % X", g)
	}
	t.Logf("glyphs sha1 %x", h.Sum(nil))
	for c := 0x20; c < 0x80+len(mtExtra); c++ {
		if c != 0x83 && (c >= 0x80 || c != 0x5E && c != 0x60 && c < 0x7B) { // ↑ is G0's 0x5E and G2's too
			if r := mtRune(c); mtFromRune(r) != c {
				t.Errorf("code %02X -> %q -> %02X", c, r, mtFromRune(r))
			}
		}
	}
	for b := 1; b < 63; b++ { // 0 and 63 are G0's space and full block
		if got := mtFromRune(sextantRune(b)); got != mtBrushMosaic+b {
			t.Errorf("sextant %d -> %q -> %X", b, sextantRune(b), got)
		}
	}
}

func TestMinitelStreamBasics(t *testing.T) {
	// FF, US to row 2 column 5, red, "Hé", SO, blue background, a full block, REP 3.
	s := []byte("\x0c\x1f\x42\x45\x1b\x41H\x19\x42e\x0e\x1b\x54\x7f\x12\x43")
	d := parseVDT(s)[0]
	if c := d.get(4, 2); cellCh(c) != 'H' || cellFg(c) != 1 || mtMosaic(c) {
		t.Errorf("H: %08X", c)
	}
	if c := d.get(5, 2); mtRune(cellCh(c)) != 'é' {
		t.Errorf("é: %08X", c)
	}
	for x := 6; x < 10; x++ {
		if c := d.get(x, 2); !mtMosaic(c) || mtBits(cellCh(c)) != 63 || cellBg(c) != 4 || cellFg(c) != 1 {
			t.Errorf("block %d: %08X", x, c)
		}
	}
	if d.get(10, 2) != mtBlank {
		t.Errorf("after the run: %08X", d.get(10, 2))
	}
	// A zone: blue background from a space on, the letters after it on blue, underlined.
	d = parseVDT([]byte("\x0c\x1b\x54\x1b\x5a AB\x1b\x50\x1b\x59 C"))[0]
	row := mtDecode(d, 1)
	if !row[0].delim || row[0].bg != 4 || row[0].line {
		t.Errorf("delimiter: %+v", row[0])
	}
	if row[1].bg != 4 || !row[1].line || row[2].bg != 4 {
		t.Errorf("A B: %+v %+v", row[1], row[2])
	}
	if row[4].bg != 0 || row[4].line {
		t.Errorf("C: %+v", row[4])
	}
	// Double height from row 3 up: the top half on row 2.
	d = parseVDT([]byte("\x0c\x1f\x43\x41\x1b\x4dX"))[0]
	if c := d.get(0, 3); c&mtDH == 0 || c&mtTop != 0 || cellCh(c) != 'X' {
		t.Errorf("bottom: %08X", c)
	}
	if c := d.get(0, 2); c&(mtDH|mtTop) != mtDH|mtTop {
		t.Errorf("top: %08X", c)
	}
	// The status row, and LF back to where the cursor was.
	d = parseVDT([]byte("\x0c\x1f\x45\x41AB\x1f\x40\x41STATUS\x0aC"))[0]
	if cellCh(d.get(0, 0)) != 'S' || cellCh(d.get(2, 5)) != 'C' {
		t.Errorf("status: %c %c", cellCh(d.get(0, 0)), cellCh(d.get(2, 5)))
	}
	// Two screens: a clear between.
	if n := len(parseVDT([]byte("\x0cONE\x0cTWO"))); n != 2 {
		t.Errorf("%d screens", n)
	}
}

func TestMinitelRoundTripRandom(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for i := 0; i < 200; i++ {
		d := mtFRandom(r)
		want := mtFull(d)
		b := mtWrite(d)
		got := parseVDT(b)
		if len(got) != 1 {
			t.Fatalf("page %d: %d screens", i, len(got))
		}
		if diff := mtFDiff(want, got[0]); diff != "" {
			t.Fatalf("page %d: %s", i, diff)
		}
	}
}

func TestMinitelBigLetters(t *testing.T) {
	d := newMTDoc()
	put := func(x, y int, c int, f uint32) {
		v := makeCell(c, 3, 0, f)
		d.set(x, y, v)
		if f&mtDW != 0 {
			d.set(x+1, y, v|mtRight)
		}
		if f&mtDH != 0 {
			d.set(x, y-1, v|mtTop)
			if f&mtDW != 0 {
				d.set(x+1, y-1, v|mtRight|mtTop)
			}
		}
	}
	put(2, 5, 'W', mtDW)
	put(6, 5, 'H', mtDH)
	put(8, 9, 'S', mtDW|mtDH)
	put(12, 9, 'T', mtDW|mtDH)
	d.set(20, 24, makeCell('Z', 2, 0, mtDH)) // a bottom half with no top above it
	want := mtFull(d)
	got := parseVDT(mtWrite(d))[0]
	if diff := mtFDiff(want, got); diff != "" {
		t.Fatal(diff)
	}
}

// TestMinitelRealFiles reads every .vdt under CHARCOAL_VDT_FILES (MiniPavi's,
// MiEdit's and others' pages) and writes each again: the same screen comes back.
func TestMinitelRealFiles(t *testing.T) {
	dir := os.Getenv("CHARCOAL_VDT_FILES")
	if dir == "" {
		t.Skip("CHARCOAL_VDT_FILES not set")
	}
	n, bad := 0, 0
	filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.EqualFold(filepath.Ext(p), ".vdt") {
			return nil
		}
		b, _ := os.ReadFile(p)
		if len(b) == 0 {
			return nil
		}
		pages := parseVDT(b)
		n++
		for i, pg := range pages {
			out := mtWrite(pg)
			if dir := os.Getenv("CHARCOAL_VDT_OUT"); dir != "" && i == len(pages)-1 { // CharEdit's stream, for VDT2BMP
				os.WriteFile(filepath.Join(dir, filepath.Base(p)), out, 0o644)
			}
			got := parseVDT(out)
			if diff := mtFDiff(mtFull(pg), got[len(got)-1]); diff != "" || len(got) != 1 {
				bad++
				t.Errorf("%s screen %d (%d bytes -> %d): %s", filepath.Base(p), i, len(b), len(out), diff)
			}
		}
		if !isVDT(b, "page") {
			t.Logf("%s: not seen as Minitel without its name", filepath.Base(p))
		}
		return nil
	})
	t.Logf("%d files, %d screens wrong", n, bad)
}

// mtFRender draws a page as a Minitel's 320 x 250 screen in pal (flashing
// cells in their on phase, concealed ones hidden).
func mtFRender(d *Doc, pal []uint32) []uint32 {
	px := make([]uint32, 320*250)
	for y := 0; y < 25; y++ {
		row := mtDecode(d, y)
		for x := 0; x < 40; x++ {
			s := row[x]
			for dy := 0; dy < mtH; dy++ {
				for dx := 0; dx < mtW; dx++ {
					c := pal[s.bg]
					if !s.mask && mtDots(s, dx, dy) {
						c = pal[s.fg]
					}
					px[(y*mtH+dy)*320+x*mtW+dx] = c & 0xFFFFFF
				}
			}
		}
	}
	return px
}

// mtFReadBMP reads a 24-bit BMP of 320 x 250 (VDT2BMP's).
func mtFReadBMP(b []byte) []uint32 {
	if len(b) < 54+320*250*3 {
		return nil
	}
	off := int(b[10]) | int(b[11])<<8
	px := make([]uint32, 320*250)
	for y := 0; y < 250; y++ {
		row := off + (249-y)*320*3
		for x := 0; x < 320; x++ {
			i := row + x*3
			px[y*320+x] = uint32(b[i+2])<<16 | uint32(b[i+1])<<8 | uint32(b[i])
		}
	}
	return px
}

// TestMinitelLooksRight compares every page under CHARCOAL_VDT_REF (x.vdt
// with x.bmp beside it, made by VDT2BMP: github.com/jfdelnero/minitel) cell by
// cell with CharEdit's picture of it. The letters' shapes differ (VDT2BMP has
// the EF9345's own dots), so a letter cell only has to have the same colours.
func TestMinitelLooksRight(t *testing.T) {
	dir := os.Getenv("CHARCOAL_VDT_REF")
	if dir == "" {
		t.Skip("CHARCOAL_VDT_REF not set")
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*.vdt"))
	pal := minitelPalette.colors[:8]
	total, bad := 0, 0
	for _, f := range files {
		b, _ := os.ReadFile(f)
		ref, _ := os.ReadFile(strings.TrimSuffix(f, ".vdt") + ".bmp")
		want := mtFReadBMP(ref)
		if want == nil || len(b) == 0 {
			continue
		}
		pages := parseVDT(b)
		d := pages[len(pages)-1]
		got := mtFRender(d, pal)
		var wrong []string
		if out := os.Getenv("CHARCOAL_VDT_DUMP"); out != "" { // CharEdit's picture beside VDT2BMP's
			img := image.NewRGBA(image.Rect(0, 0, 640, 250))
			for i, c := range got {
				img.Set(i%320, i/320, color.RGBA{uint8(c >> 16), uint8(c >> 8), uint8(c), 255})
				w := want[i]
				img.Set(320+i%320, i/320, color.RGBA{uint8(w >> 16), uint8(w >> 8), uint8(w), 255})
			}
			var buf bytes.Buffer
			png.Encode(&buf, img)
			os.WriteFile(filepath.Join(out, strings.TrimSuffix(filepath.Base(f), ".vdt")+".png"), buf.Bytes(), 0o644)
		}
		for cy := 0; cy < 25; cy++ {
			row := mtDecode(d, cy)
			for cx := 0; cx < 40; cx++ {
				if cx == 38 && cy == 0 { // the Minitel's own sign (F: off line, C: connected)
					continue
				}
				same, gc, wc := true, map[uint32]bool{}, map[uint32]bool{}
				for dy := 0; dy < mtH; dy++ {
					for dx := 0; dx < mtW; dx++ {
						i := (cy*mtH+dy)*320 + cx*mtW + dx
						gc[got[i]], wc[want[i]] = true, true
						if got[i] != want[i] {
							same = false
						}
					}
				}
				if !same && (row[cx].mosaic || len(gc) == 1 || len(wc) == 1 || len(gc) != len(wc)) {
					ok := !row[cx].mosaic && len(gc) == len(wc)
					for c := range gc {
						ok = ok && wc[c]
					}
					if !ok {
						wrong = append(wrong, fmt.Sprintf("(%d,%d)", cx, cy))
					}
				}
			}
		}
		total++
		if len(wrong) > 0 {
			bad++
			if len(wrong) > 8 {
				wrong = append(wrong[:8], fmt.Sprintf("... %d cells", len(wrong)))
			}
			t.Errorf("%s: %s", filepath.Base(f), strings.Join(wrong, " "))
		}
	}
	t.Logf("%d pages, %d differ", total, bad)
}

// FuzzVideotex: the Minitel and Viewdata readers never crash, and whatever a
// Minitel stream shows, CharEdit's stream of it shows the same.
func FuzzVideotex(f *testing.F) {
	f.Add([]byte("\x0c\x1f\x42\x45\x1b\x41H\x19\x42e\x0e\x1b\x54\x7f\x12\x43"))
	f.Add([]byte("\x0c\x1b\x54\x1b\x5a AB\x1b\x50\x1b\x59 C\x1f\x43\x41\x1b\x4fX\x1b[2J\x18"))
	r := rand.New(rand.NewSource(3))
	f.Add(mtWrite(mtFRandom(r)))
	f.Fuzz(func(t *testing.T, b []byte) {
		pages := parseVDT(b)
		p := pages[len(pages)-1]
		if back := parseVDT(mtWrite(p)); len(back) != 1 || mtFDiff(mtFull(p), back[0]) != "" {
			t.Fatalf("written again: %s", mtFDiff(mtFull(p), back[len(back)-1]))
		}
		parseVDStream(b)
		parseTelstar(b)
	})
}

func TestMinitelEditor(t *testing.T) {
	a := testApp(t, "mt")
	if a.doc.cols != 40 || a.doc.rows != 25 || a.saveFmt != "vdt" || a.cellW() != 8 || a.cellH() != 10 || a.nColours(false) != 8 || a.cy != 1 {
		t.Fatalf("new page: %dx%d %s, cursor row %d", a.doc.cols, a.doc.rows, a.saveFmt, a.cy)
	}
	if a.pal[3] != minitelGreyPalette.colors[3] {
		t.Fatal("the grey levels are the default")
	}
	// A blue zone: a space with a blue background, then letters on it.
	a.fg, a.bg = 3, 4
	for _, r := range " Hé" {
		a.typeByte(a.charToByte(r))
	}
	row := mtDecode(a.doc, 1)
	if !row[0].delim || row[1].bg != 4 || row[1].fg != 3 || mtRune(row[2].code) != 'é' {
		t.Fatalf("zone: %+v %+v", row[0], row[1])
	}
	// Big letters: the cursor stays on their bottom row.
	a.cx, a.cy = 5, 4
	a.mtSize = 3
	a.typeByte('W')
	if a.cx != 7 || a.cy != 4 || a.doc.get(6, 3)&(mtTop|mtRight|mtDW|mtDH) != mtTop|mtRight|mtDW|mtDH {
		t.Fatalf("double size: cursor %d,%d top right %08X", a.cx, a.cy, a.doc.get(6, 3))
	}
	a.mtSize = 0
	// The Sixel tool: blocks on, in the colour, on the background the cell showed.
	a.fg = 2
	a.paintSixel(20*2+1, 10*3+2, true)
	if v := a.doc.get(20, 10); !mtMosaic(v) || mtBits(cellCh(v)) != 32 || cellFg(v) != 2 || cellBg(v) != 0 {
		t.Fatalf("sixel: %08X", v)
	}
	a.paintSixel(20*2+1, 10*3+2, false)
	if mtBits(cellCh(a.doc.get(20, 10))) != 0 {
		t.Fatal("sixel off")
	}
	// The brush from the character table: a mosaic code types a mosaic.
	a.cx, a.cy = 0, 12
	a.typeByte(mtBrushMosaic + 63)
	if v := a.doc.get(0, 12); !mtMosaic(v) || mtBits(cellCh(v)) != 63 {
		t.Fatalf("mosaic brush: %08X", v)
	}
	a.pick(0, 12)
	if a.brush != mtBrushMosaic+63 {
		t.Fatalf("pick: %X", a.brush)
	}
	// Flipping a block of mosaics mirrors their blocks.
	a.sel = &Sel{0, 12, 0, 12}
	a.blockOp(flipH)
	a.sel = nil
	a.setRows(30)
	a.setCols(50)
	if a.doc.rows != 25 || a.doc.cols != 40 {
		t.Fatal("a Minitel screen keeps its size")
	}
	want := mtFull(a.doc)
	p := filepath.Join(t.TempDir(), "art.vdt")
	if !a.writeOut(p, "vdt") {
		t.Fatal(a.message)
	}
	b := testApp(t, "amiga")
	b.openPath(p, "auto")
	if !b.mt() || b.saveFmt != "vdt" {
		t.Fatalf("opened as %s (%s)", b.mode, b.saveFmt)
	}
	if diff := mtFDiff(want, b.doc); diff != "" {
		t.Fatal(diff)
	}
	// Frames: one screen after another, each starting with FF.
	b.newFrame()
	b.doc.set(3, 3, makeCell('Z', 7, 0, 0))
	if !b.writeOut(p, b.saveFmt) {
		t.Fatal(b.message)
	}
	c := testApp(t, "amiga")
	c.openPath(p, "auto")
	if c.nFrames() != 2 || cellCh(c.frameDoc(1).get(3, 3)) != 'Z' {
		t.Fatalf("%d frames", c.nFrames())
	}
	// A PNG of the page is the Minitel's 320 x 250 (twice: 640 x 500).
	if !c.writeOut(filepath.Join(t.TempDir(), "art.png"), "png") {
		t.Fatal(c.message)
	}
}

func TestViewdataEditor(t *testing.T) {
	a := testApp(t, "vd")
	if !a.tt() || a.doc.rows != 24 || a.saveFmt != "vdcpt" || a.maxRows() != 24 {
		t.Fatalf("new page: %d rows %s", a.doc.rows, a.saveFmt)
	}
	a.typeByte(0x01)
	for _, r := range "Prestel" {
		a.typeByte(a.charToByte(r))
	}
	a.cx, a.cy = 0, 23
	a.typeByte(0x12)
	a.typeByte(0x7F)
	want := a.doc.clone()
	dir := t.TempDir()
	for _, f := range []struct{ fmtID, name string }{{"vdcpt", "a.cpt"}, {"vdbin", "a.bin"}, {"vdtel", "300a.json"}, {"tturl", "a.url"}} {
		p := filepath.Join(dir, f.name)
		if !a.writeOut(p, f.fmtID) {
			t.Fatal(a.message)
		}
		b := testApp(t, "amiga")
		b.openPath(p, "auto")
		wantMode := "vd"
		if f.fmtID == "tturl" {
			wantMode = "tt"
		}
		if b.mode != wantMode {
			t.Fatalf("%s opened as %s", f.name, b.mode)
		}
		for y := 0; y < 24; y++ {
			for x := 0; x < 40; x++ {
				if cellCh(b.doc.get(x, y))&0x7F != cellCh(want.get(x, y))&0x7F {
					t.Fatalf("%s: %d,%d", f.name, x, y)
				}
			}
		}
	}
	// A teletext file opened as Viewdata keeps 24 rows.
	b := testApp(t, "amiga")
	b.openPath(filepath.Join(dir, "a.url"), "vd")
	if !b.vd() || b.doc.rows != 24 {
		t.Fatalf("as Viewdata: %s, %d rows", b.mode, b.doc.rows)
	}
}

// TestVideotexPlay: Play sends the page's stream to a Minitel (or a Viewdata
// terminal) and shows what it draws; at its end the page is as it was.
func TestVideotexPlay(t *testing.T) {
	for _, mode := range []string{"mt", "vd"} {
		a := testApp(t, mode)
		a.fg, a.bg = 2, 5
		for _, r := range " Play it" {
			a.typeByte(a.charToByte(r))
		}
		want := a.doc.clone()
		a.play()
		if a.playing == nil {
			t.Fatalf("%s: %s", mode, a.message)
		}
		a.playing.stepLine(time.Now().Add(time.Hour), 1200)
		got := a.doc
		if mode == "mt" {
			if diff := mtFDiff(mtFull(want), got); diff != "" {
				t.Fatalf("mt: %s", diff)
			}
		} else {
			for i := range want.cells {
				if cellCh(want.cells[i]) != cellCh(got.cells[i]) {
					t.Fatalf("vd: cell %d", i)
				}
			}
		}
		a.stopPlay(false)
		for i := range want.cells {
			if a.doc.cells[i] != want.cells[i] {
				t.Fatalf("%s: the page is not back", mode)
			}
		}
	}
}

// TestVideotexAutosaveAndPicture: an autosave brings a Minitel or Viewdata page
// back as it was; a Minitel picture is the screen's 320 x 250, a Viewdata one 40 x 24 teletext cells.
func TestVideotexAutosaveAndPicture(t *testing.T) {
	for _, mode := range []string{"mt", "vd"} {
		a := testApp(t, mode)
		a.fg, a.bg = 1, 6
		for _, r := range " Saved" {
			a.typeByte(a.charToByte(r))
		}
		a.dirty = true
		if err := writeAutosave(a.Tab); err != nil {
			t.Fatal(err)
		}
		b := restart(t, "pc")
		files := leftAutosaves()
		if len(files) != 1 || b.restoreAutosave(files[0]) != nil {
			t.Fatalf("%s: autosave", mode)
		}
		if b.mode != mode || b.doc.rows != a.doc.rows {
			t.Fatalf("autosave: %s, %d rows", b.mode, b.doc.rows)
		}
		for i := range a.doc.cells[:a.doc.rows*40] {
			if a.doc.cells[i] != b.doc.cells[i] {
				t.Fatalf("%s: cell %d", mode, i)
			}
		}
		pngb, err := a.encode("png")
		if err != nil {
			t.Fatal(err)
		}
		img, err := png.Decode(bytes.NewReader(pngb))
		if err != nil {
			t.Fatal(err)
		}
		want := map[string][2]int{"mt": {320, 250}, "vd": {480, 480}}[mode] // 12x20 teletext cells
		if w, h := img.Bounds().Dx(), img.Bounds().Dy(); w != want[0] || h != want[1] {
			t.Fatalf("%s picture %dx%d", mode, w, h)
		}
	}
}
