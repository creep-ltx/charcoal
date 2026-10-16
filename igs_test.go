package main

import (
	"bytes"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// igsFFonts loads the ST fonts (8x8, 8x16 and 6x6) for a test from CHARCOAL_ST_FONTS or the real data
// folder's downloads; false (and the test skipped by the caller) when they are not there.
func igsFFonts(t *testing.T) bool {
	t.Helper()
	old, old6, oldOK := fonts, igsFont6, igsFont6OK
	t.Cleanup(func() { fonts, igsFont6, igsFont6OK = old, old6, oldOK }) // other tests draw without them
	if readIGSFont6(); hasSTFont() && igsFont6OK {
		return true
	}
	dirs := []string{os.Getenv("CHARCOAL_ST_FONTS")}
	if h, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(h, ".local/share/charcoal"))
	}
	data := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	for _, f := range stFontFiles {
		for _, d := range dirs {
			if b, err := os.ReadFile(filepath.Join(d, "emutos-"+f.file)); d != "" && err == nil && sha1Is(b, f.sha1) {
				os.MkdirAll(filepath.Dir(stFontPath(f.file)), 0o755)
				os.WriteFile(stFontPath(f.file), b, 0o644)
				break
			}
		}
	}
	loadSTFonts()
	return hasSTFont() && igsFont6OK
}

// igsFRunAll runs every item of an IGS file, as a terminal shows it at the end (IcyDraw's tests do).
func igsFRunAll(b []byte, res int) *igsVM {
	vm := newIgsVM(res)
	for _, it := range igsParse(b) {
		if it.cmd != nil {
			vm.run(it.cmd)
		} else {
			vm.vt52(b[it.start:it.end])
		}
	}
	return vm
}

// igsFCompare gives the share of pixels of vm's screen that have the colour of the PNG's (scaled to it).
func igsFCompare(vm *igsVM, img image.Image) float64 {
	bnd := img.Bounds()
	same, n := 0, 0
	for y := 0; y < bnd.Dy(); y++ {
		for x := 0; x < bnd.Dx(); x++ {
			px, py := x*vm.w/bnd.Dx(), y*vm.h/bnd.Dy()
			c := vm.pal[vm.pix[py*vm.w+px]&15]
			r, g, b, _ := img.At(bnd.Min.X+x, bnd.Min.Y+y).RGBA()
			if uint32(r>>8) == c>>16&255 && uint32(g>>8) == c>>8&255 && uint32(b>>8) == c&255 {
				same++
			}
			n++
		}
	}
	return float64(same) / float64(max(1, n))
}

// IcyDraw's IGS test pictures (CHARCOAL_IGS_ICY = icy_tools' crates folder): each file run as IcyDraw
// runs it, compared pixel by pixel with IcyDraw's own PNG of it.
func TestIGSIcyRenders(t *testing.T) {
	dir := os.Getenv("CHARCOAL_IGS_ICY")
	if dir == "" {
		t.Skip("set CHARCOAL_IGS_ICY to icy_tools/crates")
	}
	if !igsFFonts(t) {
		t.Skip("no ST fonts")
	}
	res := map[string]int{"lowres": igsLow, "midres": igsMedium, "gof": igsMedium, "ignite01": igsLow, "palette": igsLow,
		"text_effect": igsLow, "highres": igsHigh}
	for sub, r := range res {
		files, _ := filepath.Glob(filepath.Join(dir, "icy_engine/tests/output/igs", sub, "*"))
		for _, f := range files {
			ext := strings.ToLower(filepath.Ext(f))
			if ext == ".skip" && strings.HasSuffix(strings.ToLower(f), ".ig.skip") {
				f = strings.TrimSuffix(f, ".skip")
				ext = ".ig"
			}
			if ext != ".ig" {
				continue
			}
			pngf := strings.TrimSuffix(f, filepath.Ext(f)) + ".png"
			if _, err := os.Stat(pngf); err != nil {
				continue
			}
			pf, err := os.Open(pngf)
			if err != nil {
				continue
			}
			img, err := png.Decode(pf)
			pf.Close()
			if err != nil {
				continue
			}
			b, err := os.ReadFile(f)
			if err != nil {
				b, _ = os.ReadFile(f + ".skip")
			}
			vm := igsFRunAll(b, r)
			t.Logf("%-12s %-22s %6.2f%%", sub, filepath.Base(f), 100*igsFCompare(vm, img))
		}
	}
}

// IcyDraw's test pictures kept in testdata/igs/icydraw: CharEdit draws each pixel for pixel as IcyDraw
// does (font_sizes needs the ST fonts).
func TestIGSIcyDrawPictures(t *testing.T) {
	fontsOK := igsFFonts(t)
	files, _ := filepath.Glob("testdata/igs/icydraw/*.ig")
	if len(files) < 10 {
		t.Fatalf("%d test files", len(files))
	}
	for _, f := range files {
		if strings.Contains(f, "font_sizes") && !fontsOK {
			continue
		}
		b, _ := os.ReadFile(f)
		pf, err := os.Open(strings.TrimSuffix(f, ".ig") + ".png")
		if err != nil {
			t.Fatal(err)
		}
		img, err := png.Decode(pf)
		pf.Close()
		if err != nil {
			t.Fatal(err)
		}
		res := igsLow
		if strings.Contains(f, "tie") {
			res = igsMedium
		}
		if got := igsFCompare(igsFRunAll(b, res), img); got != 1 {
			t.Errorf("%s: %.2f%% of the pixels as IcyDraw draws them", f, 100*got)
		}
	}
}

func TestIGSParse(t *testing.T) {
	kinds := func(b string) string {
		var s []string
		for _, it := range igsParse([]byte(b)) {
			switch {
			case it.cmd != nil:
				s = append(s, string(it.cmd.letter))
			case it.wipe:
				s = append(s, "E")
			default:
				s = append(s, strconv.Quote(b[it.start:it.end]))
			}
		}
		return strings.Join(s, " ")
	}
	for _, c := range []struct{ in, want string }{
		{"G#L>1,2,3,4:L>5,6,7,8:\r\nHi", `L L "Hi"`},
		{"G#p>24,19:We all", `p "We all"`},          // a letter not followed by '>' is text (the spec's example)
		{"G#t>1: \r\nG#s>0:", `t s`},                // a space and the line's end after a chain are not shown
		{"ab\x1bEcd", `"ab" E "cd"`},                // ESC E is an item of its own
		{"\x1bYG#x", `"\x1bYG#x"`},                  // G# inside an escape is not a command
		{"G#W>1,2,Hi there@C>1,2:", `W C`},          // W's text up to '@', then the chain goes on
		{"G#B 0,0,10,10,0\r\nG#Z>1,1,2,2:", `B Z`},  // a command without ':' ends after its values
		{"G#&>0,9,3,0,L,4,0,x,9,y:X>2,0,5:", `& X`}, // a loop, then the chain
	} {
		if got := kinds(c.in); got != c.want {
			t.Errorf("%q: %s, want %s", c.in, got, c.want)
		}
	}
	items := igsParse([]byte("G#L>10,48,\r\n1\r\n5\r\n2\r\n,\r\n48:")) // IG 2.18: values over lines
	if c := items[0].cmd; c.num(2) != 152 || c.num(3) != 48 {
		t.Fatalf("values %v", c.vals)
	}
	items = igsParse([]byte("G#W>10,20,Received Email\r\n will be deleted@"))
	if got := string(items[0].cmd.text); got != "Received Email will be deleted" {
		t.Fatalf("W text %q", got)
	}
	// A chain gang: the index of the command, then its values; ')' or the letter separates them.
	items = igsParse([]byte("G#&>0,636,4,0,>CL@,16,0)1,3:1)319,99,x,0:0C1,2:1L319,99,+2,0:"))
	l := items[0].cmd.loop
	n := 0
	for _, p := range l.params {
		if p.kind != ':' {
			n++
		}
	}
	if !l.gang || string(l.target) != "CL" || l.count != 16 || n != 16 || l.params[len(l.params)-2].kind != '+' {
		t.Fatalf("chain gang %+v", l)
	}
	// W@: one text per step.
	items = igsParse([]byte("G#&>20,60,20,0,W@2,0,x,One@Two@Three@:C>1,1:"))
	if l := items[0].cmd.loop; len(l.texts) != 3 || string(l.texts[2]) != "Three" || items[1].cmd.letter != 'C' {
		t.Fatalf("W@ loop %+v, %d items", items[0].cmd.loop, len(items))
	}
}

func TestIGSDraws(t *testing.T) {
	px := func(vm *igsVM, x, y int) uint8 { return vm.pix[y*vm.w+x] }
	vm := igsFRunAll([]byte("G#C>1,2:L>0,5,20,5:"), igsLow) // pen 2 is register 1
	if px(vm, 0, 5) != 1 || px(vm, 20, 5) != 1 || px(vm, 21, 5) != 0 || px(vm, 5, 4) != 0 {
		t.Fatal("line")
	}
	vm = igsFRunAll([]byte("G#&>0,10,5,0,L,4,0,x,100,x:"), igsLow) // three lines, y = x
	for _, y := range []int{0, 5, 10} {
		if px(vm, 50, y) != 15 {
			t.Fatalf("loop line at %d", y)
		}
	}
	vm = igsFRunAll([]byte("G#C>2,3:A>1,1,0:B>10,10,30,20,0:C>2,4:F>0,0:"), igsLow) // a box, then fill around it
	if px(vm, 20, 15) != 2 || px(vm, 0, 0) != 4 || px(vm, 319, 199) != 4 {
		t.Fatalf("box and fill: %d %d", px(vm, 20, 15), px(vm, 0, 0))
	}
	vm = igsFRunAll([]byte("G#S>1,7,0,7:X>1,2,1911:X>12,1,7,0,0,0:"), igsLow) // pen 1 is register 15
	if vm.pal[15] != stRGB(7, 0, 7) || vm.pal[2] != 0xFFEEEEEE || vm.pal[4] != stWord(7) || vm.pal[5] != 0xFF000000 {
		t.Fatalf("palette %x", vm.pal)
	}
	if stWord(0xFFF) != 0xFFFFFFFF || stWord(0x777) != 0xFFEEEEEE || stWord(0x888) != 0xFF111111 {
		t.Fatal("STE levels")
	}
	// X 11: the ST's screen memory after the command, shown.
	mem := make([]byte, 32000)
	mem[0], mem[1] = 0x80, 0x00 // plane 0 of the first 16 pixels: pixel 0 has colour 1
	mem[2] = 0x80               // plane 1: pixel 0 has colour 3
	b := append(append([]byte("G#X>11,1,0,"), mem...), "G#C>1,2:L>0,1,0,1:"...)
	vm = igsFRunAll(b, igsLow)
	if px(vm, 0, 0) != 3 || px(vm, 1, 0) != 0 || px(vm, 0, 1) != 1 {
		t.Fatalf("blit memory %d %d %d", px(vm, 0, 0), px(vm, 1, 0), px(vm, 0, 1))
	}
	// A user fill pattern (X 7) used by A 4.
	pat := strings.Repeat("XXXXXXXX--------@\r\n", 16)
	vm = igsFRunAll([]byte("G#X>7,2,\r\n"+pat+":C>2,2:A>4,2,0:Z>0,0,15,0:"), igsLow)
	if px(vm, 0, 0) != 1 || px(vm, 7, 0) != 1 || px(vm, 8, 0) != 0 {
		t.Fatal("user pattern")
	}
	// A hostile loop stops at the work budget, quickly.
	start := time.Now()
	igsFRunAll([]byte("G#&>0,32000,1,0,O,3,160,100,9999:"), igsLow)
	if el := time.Since(start); el > 3*time.Second {
		t.Fatalf("took %v", el)
	}
}

func TestIGSResolution(t *testing.T) {
	for _, c := range []struct {
		in   string
		want int
	}{
		{"G#L>0,0,319,199:", igsLow},
		{"G#L>0,0,639,199:", igsMedium},
		{"G#L>0,0,639,399:", igsHigh},
		{"G#R>2,0:L>0,0,5,5:", igsHigh},
		{"G#f>3,0,0,500,10,0,20:", igsMedium},
	} {
		if got := readIGS([]byte(c.in), -1).res; got != c.want {
			t.Errorf("%s: resolution %d", c.in, got)
		}
	}
	if l := readIGS([]byte("G#L>0,0,319,199:"), igsHigh); l.res != igsHigh || l.w != 640 || l.h != 400 {
		t.Fatal("chosen resolution")
	}
}

// igsFSample: a drawing, then text over it, a palette change, a sound and a pause, then the wipe.
const igsFSample = "G#I>0:R>0,1:s>5:k>0:\r\nG#C>2,2:A>1,1,0:B>10,10,100,60,0:\r\nG#C>1,4:L>0,199,319,0:\r\n" +
	"\x1bY%#Hello\r\nG#S>2,7,7,0:\r\nG#b>3:t>10:\r\nG#s>4:I>0:\r\n"

func TestIGSSplit(t *testing.T) {
	l := readIGS([]byte(igsFSample), -1)
	if l.res != igsLow || l.pix[30*320+50] != 1 || l.pix[199*320] != 4 {
		t.Fatalf("picture: res %d, %d %d", l.res, l.pix[30*320+50], l.pix[199*320])
	}
	if v := l.cells.get(3, 5); cellCh(v) != 'H' || cellFg(v) != 15 || l.cells.rows != 25 {
		t.Fatalf("text cell %x, %d rows", v, l.cells.rows)
	}
	if !isBlank(l.cells.get(0, 0)) {
		t.Fatal("cells not printed must show the picture")
	}
	if got := string(bytes.Join(l.kept, []byte("|"))); got != "G#S>2,7,7,0:|G#b>3:|G#t>10:" {
		t.Fatalf("kept %q", got)
	}
	if string(l.closing) != "G#s>4:I>0:\r\n" || !strings.HasSuffix(string(l.head), "L>0,199,319,0:") {
		t.Fatalf("closing %q, head ends %q", l.closing, l.head[max(0, len(l.head)-20):])
	}
	if l.pal[1] != stRGB(7, 7, 0) { // the palette after the S (as colourful as before it: the later wins)
		t.Fatalf("palette %x", l.pal)
	}
	// A fade to black after the drawing: the picture is shown before it.
	l = readIGS([]byte("G#C>2,2:B>0,0,100,100,0:\r\nG#S>2,0,0,0:S>0,0,0,0:q>50:"), -1)
	if l.pal[1] != igsDesktop[1] || l.pal[0] != igsDesktop[0] {
		t.Fatalf("fade: %x", l.pal)
	}
	// Drawn black, then revealed: shown revealed.
	l = readIGS([]byte("G#S>2,0,0,0:C>2,2:B>0,0,100,100,0:\r\nG#S>2,7,0,0:"), -1)
	if l.pal[1] != stRGB(7, 0, 0) {
		t.Fatalf("reveal: %x", l.pal)
	}
	// Text cleared and drawn again after the drawing: the clear is part of the drawing (the page is blank).
	l = readIGS([]byte("G#B>0,0,50,50,0:\x1bEMenu"), -1)
	if l.pix[10*320+10] != 0 || cellCh(l.cells.get(0, 0)) != 'M' {
		t.Fatal("ESC E before text")
	}
}

func TestIGSRoundTrip(t *testing.T) {
	a := testApp(t, "st")
	withFonts(t, true)
	src := []byte(igsFSample)
	a.loadBytes(src, "x.ig", "", "auto")
	if !a.igsPage() || a.saveFmt != "igs" || a.stres != "stlow" || cellCh(a.doc.get(3, 5)) != 'H' || !a.fixedHeight() {
		t.Fatalf("opened: igs %v, %s, %s", a.igsPage(), a.saveFmt, a.stres)
	}
	if out, _ := a.encode("igs"); !bytes.Equal(out, src) {
		t.Fatalf("unchanged page saved as %q", out)
	}
	// The picture shows through empty cells, the text's cells are drawn over it.
	fb := a.renderDoc(a.doc, 1)
	if fb.px[30*fb.w+50] != a.pal[1] || a.pal[1] != stRGB(7, 7, 0) {
		t.Fatalf("picture pixel %x", fb.px[30*fb.w+50])
	}
	a.doc.set(10, 20, makeCell('Z', 2, 4, 0))
	a.doc.set(3, 5, blank) // erased: the picture again
	a.doc.set(39, 24, makeCell('!', 1, 0, 0))
	fb = a.renderDoc(a.doc, 1)
	if fb.px[(20*8+3)*fb.w+10*8+3] != a.pal[4] { // the fake font's glyphs are empty: the cell's paper
		t.Fatal("cell over the picture")
	}
	out, _ := a.encode("igs")
	if !bytes.HasPrefix(out, []byte(igsFSample[:strings.Index(igsFSample, "\r\n\x1b")]+"\r\x1bq")) ||
		!bytes.HasSuffix(out, []byte("G#S>2,7,7,0:\rG#b>3:\rG#t>10:\rG#s>4:I>0:\r\n")) {
		t.Fatalf("saved %q", out)
	}
	b := testApp(t, "st")
	b.loadBytes(out, "y.ig", "", "auto")
	if !b.igsPage() || !igsSameCells(a.doc, b.doc) || !bytes.Equal(a.igs.pix, b.igs.pix) || a.igs.pal != b.igs.pal {
		t.Fatalf("read back: %v %v", b.igsPage(), igsSameCells(a.doc, b.doc))
	}
	if again, _ := b.encode("igs"); !bytes.Equal(again, out) {
		t.Fatal("saving the read-back page changed it")
	}
	// Another resolution: the picture is drawn again in it, typed cells stay, saving keeps the drawing; a
	// file that sets its own (R) keeps it.
	b.setSTRes("stmed")
	if b.stres != "stlow" || !b.igsPage() {
		t.Fatal("the file's own resolution")
	}
	m := testApp(t, "st")
	m.loadBytes([]byte("G#C>1,2:L>0,0,300,100:"), "m.ig", "", "auto")
	m.doc.set(10, 20, makeCell('Z', 2, 4, 0))
	m.dirty = true
	m.setSTRes("stmed")
	if !m.igsPage() || m.doc.cols != 80 || m.igs.w != 640 || cellCh(m.doc.get(10, 20)) != 'Z' {
		t.Fatalf("medium: %v %d", m.igsPage(), m.doc.cols)
	}
	if med, _ := m.encode("igs"); !bytes.HasPrefix(med, m.igs.head) || !bytes.Contains(med, []byte("Z")) {
		t.Fatal("medium save")
	}
	// Saved as .vt the cells go (the picture cannot); a plain ST page saved as .ig is its VT52.
	if vt, _ := b.encode("vt52"); !bytes.Contains(vt, []byte("Z")) {
		t.Fatal("vt52")
	}
	c := testApp(t, "st")
	c.doc.set(1, 1, makeCell('Q', 3, 0, 0))
	plain, _ := c.encode("igs")
	d := readIGS(plain, igsResOf(c.stres))
	if cellCh(d.cells.get(1, 1)) != 'Q' || len(d.head) != 0 {
		t.Fatalf("plain page as .ig: %q", plain)
	}
}

func TestIGSDetect(t *testing.T) {
	for _, c := range []struct {
		in, name string
		want     bool
	}{
		{igsFSample, "x.txt", true},
		{"G#L>1,2,3,4:", "x.ig", true},
		{"G#L>1,2,3,4:", "x.txt", false},
		{"\x1bEHello \x1bb1world", "x.vt", false},
		{"The key of G# minor, G# major", "x.txt", false},
		{"\x1b[1mG#L>1,2,3,4:G#L>1,2,3,4:", "x.ans", false},
	} {
		if got := isIGS([]byte(c.in), c.name); got != c.want {
			t.Errorf("%q %s: %v", c.in, c.name, got)
		}
	}
}

// Real IGS files (CHARCOAL_IGS_FILES: a folder, e.g. icy_tools/crates/icy_parser_core/benches/igs_data):
// each opens as an IGS page, saves unchanged byte for byte, and a line typed over it comes back.
func TestIGSRealFiles(t *testing.T) {
	dir := os.Getenv("CHARCOAL_IGS_FILES")
	if dir == "" {
		t.Skip("set CHARCOAL_IGS_FILES")
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*"))
	n := 0
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil || !isIGS(src, filepath.Base(f)) {
			continue
		}
		n++
		a := testApp(t, "st")
		a.loadBytes(src, filepath.Base(f), "", "auto")
		if !a.igsPage() {
			t.Errorf("%s: not an IGS page", f)
			continue
		}
		if out, _ := a.encode("igs"); !bytes.Equal(out, src) {
			t.Errorf("%s: unchanged save differs", f)
		}
		for x, ch := range []byte("CharEdit") {
			a.doc.set(2+x, 1, makeCell(int(ch), 1, 0, 0))
		}
		out, _ := a.encode("igs")
		l := readIGS(out, a.igs.res)
		if !igsSameCells(l.cells, a.doc) || !bytes.Equal(l.pix, a.igs.pix) || l.pal != a.igs.pal {
			t.Errorf("%s: typed text does not come back the same", f)
		}
		t.Logf("%-16s %s, %d kept, closing %d bytes, work %d", filepath.Base(f),
			igsResIDs[a.igs.res], len(a.igs.kept), len(a.igs.closing), igsFRunAll(src, a.igs.res).work)
	}
	if n == 0 {
		t.Fatal("no IGS files there")
	}
}

// FuzzIGS: the IGS reader on its own (FuzzParsers stalls on the slow readers): never a crash, never
// more than a second, and a page with a cell typed on it reads back as written.
//
//	systemd-run --user --scope -q -p MemoryMax=4G go test -run XXX -fuzz FuzzIGS -fuzztime 60s -parallel 2 .
func FuzzIGS(f *testing.F) {
	f.Add([]byte(igsFSample))
	f.Add([]byte("G#&>0,636,4,0,>CL@,16,0)1,3:1)319,99,x,0:0C1,2:1L319,99,+2,0:&>20,60,20,0,W@2,0,x,a@b@c@:"))
	f.Add([]byte("G#X>7,1,\r\n" + strings.Repeat("XX--XX--XX--XX--@", 16) + ":A>4,1,1:Z>r,R,9999,9999:G>0,6,0,0,99,99,5,5:"))
	f.Add([]byte("G#X>11,1,3,\xff\x00\xff\x00G#T>2,1,41:T>2,1,50:J>1,1,500,9,0,3600:E>31,20,1:W>9,9,Hi@F>5,5:"))
	f.Add([]byte("G#W>3,3,cut short"))
	f.Add([]byte("G#R>1,0:\x1bYA!x\x1bb3G#c>0,2:p>3,4:Hi\x1bEG#V>99,99,50,0,270:Y>9,9,99,9,10,20:U>1,1,99,99,1:q>3:s>2:"))
	f.Fuzz(func(t *testing.T, b []byte) {
		start := time.Now()
		l := readIGS(b, -1)
		if el := time.Since(start); el > time.Second {
			t.Fatalf("took %v", el)
		}
		d := l.cells.clone()
		d.set(1, 1, makeCell('Z', 1, 2, 0))
		back := readIGS(writeIGS(l, d), l.res)
		if !igsSameCells(back.cells, d) || !bytes.Equal(back.pix, l.pix) {
			t.Fatalf("does not read back")
		}
	})
}
