package main

import (
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// uniFDurDir is where the durdraw files and durdraw's own ANSI exports of them are.
func uniFDurDirs() []string {
	dirs := []string{"testdata/durdraw"}
	if d := os.Getenv("CHARCOAL_DURDRAW"); d != "" {
		dirs = append(dirs, d)
	}
	return dirs
}

// TestUniDurdrawMatchesItsExport: every .dur opens as durdraw itself shows it:
// its first frame cell for cell as the ANSI durdraw --export-ansi wrote for it
// (the reference files were made by durdraw 0.31.0; in 256-colour mode durdraw
// writes every background black, so only 16-colour files compare backgrounds).
func TestUniDurdrawMatchesItsExport(t *testing.T) {
	n := 0
	for _, dir := range uniFDurDirs() {
		files, _ := filepath.Glob(filepath.Join(dir, "*.dur"))
		for _, f := range files {
			ref, err := os.ReadFile(strings.TrimSuffix(f, ".dur") + ".ansi")
			if err != nil || len(ref) < 30 {
				continue
			}
			b, _ := os.ReadFile(f)
			art, err := parseDur(b)
			if err != nil {
				t.Errorf("%s: %v", f, err)
				continue
			}
			want, _ := parseUniArt(ref, 0)
			got := art.docs[0]
			bad := 0
			var first string
			for y := 0; y < want.rows && y < got.rows; y++ {
				for x := 0; x < want.cols && x < got.cols; x++ {
					w, g := want.get(x, y), got.get(x, y)
					wf, wb := uniColoursOf(w)
					gf, gb := uniColoursOf(g)
					wc, gc := uniCode(w), uniCode(g)
					if art.colours256 {
						wb, gb = 0, 0
					}
					if wc == 32 && gc == 32 { // a space shows no foreground
						wf, gf = 0, 0
					}
					if wc != gc || wf != gf || wb != gb {
						bad++
						if first == "" {
							first = fmt.Sprintf("(%d,%d) %q %d/%d, durdraw %q %d/%d", x, y, uniRune(gc), gf, gb, uniRune(wc), wf, wb)
						}
					}
				}
			}
			if bad > 0 {
				t.Errorf("%s: %d cells differ from durdraw's export, first %s", filepath.Base(f), bad, first)
			}
			n++
		}
	}
	t.Logf("%d durdraw files compared", n)
}

// ---------------------------------------------------------------- the cell

func TestUniCodes(t *testing.T) {
	for _, r := range []rune{' ', 'A', 'é', '█', '▀', '⣿', '╳', 0x1FB00, 0x1FB3B, 0x1FBFF, 0x1CC00, 0x1CD00, 0x1CFFF, 0xFFFD} {
		c, ok := uniCodeOf(r)
		if !ok || uniRune(c) != r {
			t.Errorf("%U: code %X back %U", r, c, uniRune(c))
		}
		for _, p := range []int{0, 7, 255, 256, 1023, 1024, 65535} {
			v := uniCell(c, p)
			if uniCode(v) != c || uniPair(v) != p {
				t.Errorf("%U style %d: cell %08X gives %X %d", r, p, v, uniCode(v), uniPair(v))
			}
		}
	}
	for _, r := range []rune{0xD800, 0x1F600, 0x10FFFF} {
		if _, ok := uniCodeOf(r); ok {
			t.Errorf("%U: a cell cannot hold it", r)
		}
	}
	// A Unicode cell of an ASCII character in the 16 colours is the PC cell.
	for ch := 32; ch < 127; ch++ {
		if uniCell(ch, pairOf(9, 4)) != makeCell(ch, 9, 4, 0) {
			t.Fatalf("%q is not the PC cell", ch)
		}
	}
	// Italic and underline are part of the style.
	v := uniCell('x', styleOf(9, 4, underline))
	if fg, bg := uniColoursOf(v); fg != 9 || bg != 4 || uniFlagsOf(v) != underline || uniPair(v) < 256 {
		t.Errorf("underlined: %d/%d %X", fg, bg, uniFlagsOf(v))
	}
}

func TestUniPairs(t *testing.T) {
	if p := pairOf(3, 12); p != 3|12<<4 {
		t.Errorf("pairOf(3, 12) = %d", p)
	}
	p1, p2 := pairOf(196, rgbFlag|0x102030), pairOf(196, rgbFlag|0x102030)
	if p1 < 256 || p1 != p2 {
		t.Errorf("pairs %d %d", p1, p2)
	}
	if f, b := pairColours(p1); f != 196 || b != rgbFlag|0x102030 {
		t.Errorf("pair %d is %d/%X", p1, f, b)
	}
	if c := rgbColour(xterm256(200)); c != 200 {
		t.Errorf("xterm's 200 as RGB is %d", c)
	}
	if colourName(rgbFlag|0x0A0B0C) != "#0A0B0C" || colourName(42) != "42" {
		t.Errorf("names %s %s", colourName(rgbFlag|0x0A0B0C), colourName(42))
	}
}

// Every PC cell converts to Unicode and back unchanged (character 0 is a space).
func TestUniPCConversion(t *testing.T) {
	for ch := 0; ch < 256; ch++ {
		for _, at := range []int{0x07, 0x1F, 0xF4, 0x8C} {
			v := makeCell(ch, at&15, at>>4, underline*uint32(ch&1))
			u := pcToUni(v, false)
			if uniRune(uniCode(u)) != byteRune(ch, false) {
				t.Fatalf("%02X: %U", ch, uniRune(uniCode(u)))
			}
			back, exact := uniToPC(u, false, &vga16)
			want := v
			if ch == 0 {
				want = makeCell(32, at&15, at>>4, 0)
			}
			if back != want || !exact {
				t.Fatalf("%02X attr %02X: back %08X, want %08X", ch, at, back, want)
			}
		}
	}
	// Amiga: Latin-1.
	for _, ch := range []int{'a', 0xA9, 0xFF} {
		if back, _ := uniToPC(pcToUni(makeCell(ch, 3, 0, 0), true), true, &vga16); back != makeCell(ch, 3, 0, 0) {
			t.Errorf("Amiga %02X back %08X", ch, back)
		}
	}
	// What a PC page lacks: '?' and the nearest of the 16.
	v, exact := uniToPC(uniCell('⣿', pairOf(196, 21)), false, &vga16)
	if exact || cellCh(v) != '?' || cellFg(v) != 1 || cellBg(v) != 4 { // FF0000 is nearer AA0000 than FF5555
		t.Errorf("braille in 196 on 21: %08X exact %v", v, exact)
	}
}

// ---------------------------------------------------------------- pages for the tests

// uniFRunes are characters the tests draw with: Unifont has them all.
var uniFRunes = []rune("AZaz09 .,;:!?#@é߀█▀▄▌▐░▒▓■□▲►◆●○◢◣◤◥─│┌┐└┘├┤┬┴┼═║╔╗╚╝╠╣╦╩╬╭╮╯╰╱╲╳⠁⠃⠇⡇⣿⢸☺♥♦♣♠★→←↑↓∞≈≠≤≥αβγΩЖЯ\U0001FB00\U0001FB3B\U0001FB8F\U0001CD00")

// uniFColour is a random colour: one of the 16, of the 256, or 24-bit (one of
// 40, so the tests leave room in the one style table for the rest).
func uniFColour(r *rand.Rand, rgb bool) int {
	switch k := r.Intn(4); {
	case k == 0:
		return r.Intn(16)
	case k == 1 || !rgb:
		return 16 + r.Intn(240)
	}
	return rgbColour(uint32(r.Intn(40)*0x060504 + 0x010101))
}

// uniFPage is a random Unicode page; colour gives each cell's colours.
func uniFPage(r *rand.Rand, cols, rows int, colour func() (int, int), flags bool) *Doc {
	d := newDoc(cols, rows)
	d.uni = true
	for i := range d.cells {
		if r.Intn(5) == 0 {
			continue // some blank cells
		}
		code, _ := uniCodeOf(uniFRunes[r.Intn(len(uniFRunes))])
		fg, bg := colour()
		var fl uint32
		if flags && r.Intn(8) == 0 {
			fl = italic
		}
		if flags && r.Intn(8) == 0 {
			fl |= underline
		}
		d.cells[i] = uniCell(code, styleOf(fg, bg, fl))
	}
	return d
}

// uniFSame compares two pages cell for cell as they look: a space's
// foreground is not seen, nor are blank cells' colours told apart.
func uniFSame(t *testing.T, name string, got, want *Doc) {
	t.Helper()
	if got.cols != want.cols || got.rows != want.rows {
		t.Fatalf("%s: %dx%d, want %dx%d", name, got.cols, got.rows, want.cols, want.rows)
	}
	for y := 0; y < want.rows; y++ {
		for x := 0; x < want.cols; x++ {
			g, w := got.get(x, y), want.get(x, y)
			if uniFLook(g) != uniFLook(w) {
				gf, gb := uniColoursOf(g)
				wf, wb := uniColoursOf(w)
				t.Fatalf("%s: cell %d,%d is %q %s/%s %X, want %q %s/%s %X", name, x, y, uniRune(uniCode(g)), colourName(gf), colourName(gb), uniFlagsOf(g),
					uniRune(uniCode(w)), colourName(wf), colourName(wb), uniFlagsOf(w))
			}
		}
	}
}

// uniFLook is what a cell shows: character, colours, italic, underline.
func uniFLook(v uint32) string {
	fg, bg, fl := styleParts(uniPair(v))
	code := uniCode(v)
	if code == 0 {
		code = 32
	}
	if code == 32 && fl&underline == 0 {
		fg, fl = -1, 0
	}
	if uniBlank(v) {
		return "blank"
	}
	return fmt.Sprintf("%X %d %d %X", code, fg, bg, fl)
}

// uniFApp is a test App with a Unicode page.
func uniFApp(t *testing.T) *App {
	a := testApp(t, "uni")
	a.applyMode()
	return a
}

// uniFOpen opens bytes as a file in a fresh tab, as Open would.
func uniFOpen(a *App, b []byte, name string) {
	a.focusTab(a.newTab("pc"))
	a.loadBytes(b, name, "", "auto")
	a.applyMode()
}

// ---------------------------------------------------------------- UTF-8 ANSI

// Random pages in every kind of colour, italic and underline: written as UTF-8
// ANSI and read back, the same page.
func TestUniAnsiRoundTrip(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for i := 0; i < 40; i++ {
		cols := 80 + r.Intn(60)
		d := uniFPage(r, cols, 1+r.Intn(30), func() (int, int) { return uniFColour(r, true), uniFColour(r, true) }, true)
		// The reader finds the width in the longest line: make one row reach the edge.
		d.cells[cols-1] = uniCell('X', 7)
		got, _ := parseUniArt(writeUniAnsi(d), 0)
		uniFSame(t, fmt.Sprint("page ", i), got, d)
	}
}

// Saved and opened again through the editor: auto-detected as a Unicode page,
// the width kept (a SAUCE record when it is not the longest line's), the height kept.
func TestUniAnsiSaveOpen(t *testing.T) {
	a := uniFApp(t)
	r := rand.New(rand.NewSource(2))
	for _, cols := range []int{80, 40, 132} {
		a.focusTab(a.newTab("uni"))
		a.applyMode()
		d := uniFPage(r, cols, 12, func() (int, int) { return uniFColour(r, true), uniFColour(r, true) }, false)
		d.cells = append(d.cells, make([]uint32, cols*3)...)
		fillCells(d.cells[cols*12:], blank) // empty rows at the end are part of the page
		d.rows = 15
		a.doc = d
		path := filepath.Join(t.TempDir(), "a.ans")
		if !a.writeOut(path, "uniansi") {
			t.Fatal(a.message)
		}
		b, _ := os.ReadFile(path)
		uniFOpen(a, b, "a.ans")
		if !a.uni() || a.srcKind != "uniansi" || a.saveFmt != "uniansi" {
			t.Fatalf("width %d: opened as %s/%s/%s", cols, a.mode, a.srcKind, a.saveFmt)
		}
		uniFSame(t, fmt.Sprint("width ", cols), a.doc, d)
	}
}

// durdraw's own UTF-8 ANSI (indyz-kali.utf8.ans) opens as a Unicode page and saves back the same.
func TestUniRealUTF8Ansi(t *testing.T) {
	b, err := os.ReadFile("testdata/durdraw/indyz-kali.utf8.ans")
	if err != nil {
		t.Skip(err)
	}
	a := uniFApp(t)
	uniFOpen(a, b, "indyz-kali.utf8.ans")
	if !a.uni() || a.doc.cols != 80 || a.doc.rows != 31 {
		t.Fatalf("opened as %s %dx%d", a.mode, a.doc.cols, a.doc.rows)
	}
	if v := a.doc.get(0, 0); uniRune(uniCode(v)) != '▓' {
		t.Errorf("first cell %q", uniRune(uniCode(v)))
	}
	want := a.doc.clone()
	out, err := a.encode("uniansi")
	if err != nil {
		t.Fatal(err)
	}
	got, _ := parseUniArt(out, 0)
	uniFSame(t, "kali", got, want)
}

// A UTF-8 file opens on a Unicode page only when it needs one.
func TestUniDetect(t *testing.T) {
	for _, c := range []struct {
		text string
		uni  bool
	}{
		{"plain ascii\n", false},
		{"\x1b[31m█▓▒░ ☺ ╔═╗\x1b[0m\n", false}, // all CP437: a PC page, as before
		{"Hääl ÿ\n", false},
		{"⣿⣿ braille\n", true},
		{"\x1b[38;5;196mred\n", true},
		{"\x1b[38;5;9mbright red is one of the 16\n", false},
		{"\x1b[1;48;2;10;20;30mrgb\n", true},
		{"🬀 sextant\n", true},
	} {
		if got := needsUni([]byte(c.text)); got != c.uni {
			t.Errorf("%q: needsUni %v", c.text, got)
		}
		a := uniFApp(t)
		uniFOpen(a, []byte(c.text), "x.txt")
		if a.uni() != c.uni {
			t.Errorf("%q opened as %s", c.text, a.mode)
		}
	}
	// Open as Unicode: a CP437 ANSI as its Unicode characters.
	a := uniFApp(t)
	a.focusTab(a.newTab("pc"))
	a.loadBytes([]byte("\x1b[1;34m\xdb\xdf\xb0"), "x.ans", "", "uni")
	a.applyMode()
	if !a.uni() || uniRune(uniCode(a.doc.get(1, 0))) != '▀' {
		t.Errorf("open as Unicode: %s %q", a.mode, uniRune(uniCode(a.doc.get(1, 0))))
	}
	if fg, _ := uniColoursOf(a.doc.get(0, 0)); fg != 12 {
		t.Errorf("bright blue is %d", fg)
	}
}

// The terminal codes for colours: 90-97/100-107, 38/48;5, 38/48;2, reverse, conceal.
func TestUniAnsiColours(t *testing.T) {
	for _, c := range []struct {
		in     string
		fg, bg int
	}{
		{"\x1b[38;5;196;48;5;21mx", 196, 21},
		{"\x1b[38;2;1;2;3mx", rgbFlag | 0x010203, 0},
		{"\x1b[38;2;255;0;0mx", 196, 0}, // exactly xterm's 196
		{"\x1b[93;104mx", 11, 12},
		{"\x1b[1;31;38;5;100mx", 100, 0},
		{"\x1b[38;5;100;39mx", 7, 0},
		{"\x1b[7;38;5;100;48;5;200mx", 200, 100},
		{"\x1b[38;5;100;48;5;200m\x1b[Kx", 100, 200},
		{"\x1b[48;5;200m\x1b[2J\x1b[Hx", 7, 200},
	} {
		d, _ := parseUniArt([]byte(c.in), 0)
		if fg, bg := uniColoursOf(d.get(0, 0)); fg != c.fg || bg != c.bg {
			t.Errorf("%q: %s/%s, want %s/%s", c.in, colourName(fg), colourName(bg), colourName(c.fg), colourName(c.bg))
		}
	}
	// Erasing with a 256-colour background paints it.
	d, _ := parseUniArt([]byte("\x1b[48;5;200m\x1b[2J\x1b[Hx\r\n"), 0)
	if _, bg := uniColoursOf(d.get(5, 3)); bg != 200 {
		t.Errorf("cleared with %d", bg)
	}
}

// ---------------------------------------------------------------- durdraw

func TestUniDurRoundTrip(t *testing.T) {
	r := rand.New(rand.NewSource(3))
	for _, k := range []string{"16", "256", "rgb"} {
		var docs []*Doc
		var holds []int
		colour := func() (int, int) {
			switch k {
			case "16":
				return r.Intn(16), []int{0, 1, 2, 3, 4, 5, 6, 7, 9, 10, 11, 12, 13, 14, 15}[r.Intn(15)]
			case "256":
				bg := uniFColour(r, false)
				if bg == 8 { // durdraw's 256-colour files have no dark grey background: 240 is written
					bg = 240
				}
				return uniFColour(r, false), bg
			}
			return uniFColour(r, true), uniFColour(r, true)
		}
		for i := 0; i < 4; i++ {
			docs = append(docs, uniFPage(r, 50, 20, colour, false))
			holds = append(holds, []int{100, 100, 250, 1500}[i])
		}
		out, rgb := writeDur(docs, holds, "Title", "Artist")
		art, err := parseDur(out)
		if err != nil {
			t.Fatal(err)
		}
		if (k == "rgb") != (rgb > 0) || art.colours256 != (k != "16") || art.name != "Title" || art.artist != "Artist" {
			t.Errorf("%s: rgb %d, 256 %v, %q %q", k, rgb, art.colours256, art.name, art.artist)
		}
		if !slices.Equal(art.holds, holds) {
			t.Errorf("%s: holds %v", k, art.holds)
		}
		for i := range docs {
			want := docs[i]
			if k == "rgb" { // the nearest of the 256
				want = want.clone()
				for j, v := range want.cells {
					fg, bg := uniColoursOf(v)
					if fg&rgbFlag != 0 {
						fg = nearest256(colourRGB(fg, &vga16))
					}
					if bg&rgbFlag != 0 {
						bg = nearest256(colourRGB(bg, &vga16))
					}
					if bg == 8 {
						bg = 240
					}
					want.cells[j] = uniWithPair(v, pairOf(fg, bg))
				}
			}
			uniFSame(t, fmt.Sprint(k, " frame ", i), art.docs[i], want)
		}
	}
}

// durdraw's files open, save and open again the same, frames and holds included.
func TestUniDurFilesSaveBack(t *testing.T) {
	files, _ := filepath.Glob("testdata/durdraw/*.dur")
	if len(files) == 0 {
		t.Skip("no durdraw files")
	}
	a := uniFApp(t)
	for _, f := range files {
		b, _ := os.ReadFile(f)
		uniFOpen(a, b, filepath.Base(f))
		if !a.uni() || a.srcKind != "dur" {
			t.Fatalf("%s: opened as %s %s (%s)", f, a.mode, a.srcKind, a.message)
		}
		art, _ := parseDur(b)
		if a.nFrames() != len(art.docs) {
			t.Fatalf("%s: %d frames, want %d", f, a.nFrames(), len(art.docs))
		}
		path := filepath.Join(t.TempDir(), "x.dur")
		if !a.writeOut(path, "dur") {
			t.Fatal(a.message)
		}
		out, _ := os.ReadFile(path)
		again, err := parseDur(out)
		if err != nil {
			t.Fatal(err)
		}
		for i, d := range art.docs {
			uniFSame(t, fmt.Sprintf("%s frame %d", f, i), again.docs[i], d)
			if again.holds[i] != art.holds[i] {
				t.Errorf("%s frame %d: hold %d, want %d", f, i, again.holds[i], art.holds[i])
			}
		}
	}
}

// ---------------------------------------------------------------- mIRC

func TestUniMircRoundTrip(t *testing.T) {
	r := rand.New(rand.NewSource(4))
	for i := 0; i < 30; i++ {
		d := uniFPage(r, 20+r.Intn(60), 1+r.Intn(20), func() (int, int) { return mircColour(r.Intn(99)), mircColour(r.Intn(99)) }, true)
		// Digits and commas right after colour codes.
		for j := 0; j < 20; j++ {
			d.cells[r.Intn(len(d.cells))] = uniCell(int("0123456789,"[r.Intn(11)]), pairOf(mircColour(r.Intn(99)), mircColour(r.Intn(99))))
		}
		out, _, approx := writeMirc(d, &vga16)
		if approx != 0 {
			t.Fatalf("page %d: %d colours not mIRC's", i, approx)
		}
		got := parseMirc(out)
		want := d
		if got.cols < d.cols { // blank cells at the end of every row are not written
			want = d.withCols(got.cols)
		}
		uniFSame(t, fmt.Sprint("page ", i), got, want)
	}
}

func TestUniMircParse(t *testing.T) {
	m := func(n int) int { return mircColour(n) }
	for _, c := range []struct {
		in     string
		x      int
		ch     rune
		fg, bg int
		fl     uint32
	}{
		{"\x034,2x", 0, 'x', m(4), m(2), 0},
		{"\x0304,02x", 0, 'x', m(4), m(2), 0},
		{"\x03045", 0, '5', m(4), 0, 0},         // two digits at most: the 5 is text
		{"\x034,x", 0, ',', m(4), 0, 0},         // a comma not followed by a digit is text
		{"\x034,2a\x03b", 1, 'b', 7, 0, 0},      // ^C alone: the default colours
		{"\x0350,88x", 0, 'x', m(50), m(88), 0}, // extended colours
		{"\x0399,99x", 0, 'x', 7, 0, 0},         // 99: the default
		{"\x04FF8000,000010x", 0, 'x', rgbColour(0xFF8000), rgbColour(0x000010), 0},
		{"\x1dx\x1d\x1fy", 0, 'x', 7, 0, italic},
		{"\x1dx\x1d\x1fy", 1, 'y', 7, 0, underline},
		{"\x034,2\x16x", 0, 'x', m(2), m(4), 0}, // reverse
		{"\x034,2\x1d\x0fx", 0, 'x', 7, 0, 0},   // ^O: plain
		{"\x02b\x02", 0, 'b', 7, 0, 0},          // bold: no colour
		{"é ⣿", 2, '⣿', 7, 0, 0},
	} {
		d := parseMirc([]byte(c.in))
		v := d.get(c.x, 0)
		fg, bg, fl := styleParts(uniPair(v))
		if uniRune(uniCode(v)) != c.ch || fg != c.fg || bg != c.bg || fl != c.fl {
			t.Errorf("%q cell %d: %q %s/%s %X, want %q %s/%s %X", c.in, c.x, uniRune(uniCode(v)), colourName(fg), colourName(bg), fl,
				c.ch, colourName(c.fg), colourName(c.bg), c.fl)
		}
	}
	// mIRC's 16 are the page's 16, each its own.
	seen := map[int]bool{}
	for n := 0; n < 16; n++ {
		seen[mircToAnsi[n]] = true
		if k, ok := mircNumber(mircColour(n), &vga16); !ok || k != n {
			t.Errorf("mIRC %d comes back as %d", n, k)
		}
	}
	if len(seen) != 16 {
		t.Errorf("mircToAnsi is not one to one")
	}
	// Old Latin-1 text.
	if d := parseMirc([]byte("\x034caf\xe9")); uniRune(uniCode(d.get(3, 0))) != 'é' {
		t.Errorf("Latin-1: %q", uniRune(uniCode(d.get(3, 0))))
	}
}

// A PC page saves as mIRC text with its CP437 characters in Unicode and its colours exact.
func TestUniMircFromPC(t *testing.T) {
	a := testApp(t, "pc")
	a.applyMode()
	a.ice = true
	r := rand.New(rand.NewSource(5))
	for i := range a.doc.cells[:80*10] {
		a.doc.cells[i] = makeCell(1+r.Intn(255), r.Intn(16), r.Intn(16), 0)
	}
	a.doc.cells[79] = makeCell('X', 7, 0, 0)
	out, err := a.encode("mirc")
	if err != nil {
		t.Fatal(err)
	}
	got := parseMirc(out)
	want := docToUni(a.doc, false)
	want.rows = got.rows
	uniFSame(t, "pc page", got, want)
	if !strings.Contains(a.message, "longest line") {
		t.Errorf("an 80-column page in every colour is too long for IRC, said %q", a.message)
	}
	// A short line: no warning.
	a.doc = newDoc(20, 2)
	a.doc.cells[0] = makeCell(0xDB, 4, 1, 0)
	a.message = ""
	out, _ = a.encode("mirc")
	if a.message != "" || string(out) != "\x0302,5█\n\n" { // blue: mIRC's 2, dark red: its 5
		t.Errorf("%q, said %q", out, a.message)
	}
}

// ---------------------------------------------------------------- editing

// A PC page becomes a Unicode page in one step, exactly; undo brings the PC page back, redo the Unicode one.
func TestUniConvertUndo(t *testing.T) {
	a := testApp(t, "pc")
	a.applyMode()
	for i, c := range []int{0xDB, 0xB0, 'A', 0x01, 0xC9, 0xFE} {
		a.doc.set(i, 0, makeCell(c, i+9, i, 0))
	}
	a.doc.set(0, 1, makeCell('u', 7, 0, underline))
	a.dirty, a.path, a.name = true, "/x/art.ans", "art.ans"
	pc := a.doc.clone()
	a.setMode("uni")
	if !a.uni() || a.saveFmt != "uniansi" || a.path != "" || a.name != "art.utf8.ans" {
		t.Fatalf("after converting: %s %s %q %q", a.mode, a.saveFmt, a.path, a.name)
	}
	want := docToUni(pc, false)
	uniFSame(t, "converted", a.doc, want)
	if r := uniRune(uniCode(a.doc.get(0, 0))); r != '█' {
		t.Errorf("0xDB is %q", r)
	}
	if uniFlagsOf(a.doc.get(0, 1)) != underline {
		t.Errorf("underline lost")
	}
	a.doUndo()
	if !a.pc() || fmt.Sprint(a.doc.cells[:pc.cols*pc.rows]) != fmt.Sprint(pc.cells[:pc.cols*pc.rows]) {
		t.Fatalf("undo: %s", a.mode)
	}
	a.doRedo()
	if !a.uni() {
		t.Fatalf("redo: %s", a.mode)
	}
	uniFSame(t, "redone", a.doc, want)
	// Back to PC: what CP437 lacks is '?', colours the nearest; undoable too.
	a.doc.set(10, 0, uniCell('⣿', pairOf(196, 21)))
	a.setMode("pc")
	if !a.pc() || cellCh(a.doc.get(10, 0)) != '?' || a.doc.get(0, 0) != pc.get(0, 0) || a.doc.get(0, 1) != pc.get(0, 1) {
		t.Errorf("back to PC: %s %08X", a.mode, a.doc.get(10, 0))
	}
	a.doUndo()
	if !a.uni() || uniRune(uniCode(a.doc.get(10, 0))) != '⣿' {
		t.Errorf("undoing the way back: %s", a.mode)
	}
	// An empty page just changes type; Convert to Unicode is the same command.
	a.focusTab(a.newTab("pc"))
	runAction("convert_unicode")
	if !a.uni() || a.dirty || a.saveFmt != "uniansi" || !a.doc.uni {
		t.Errorf("empty page: %s dirty %v %s", a.mode, a.dirty, a.saveFmt)
	}
	// Other machines' pages come over in their colours.
	a.focusTab(a.newTab("atari"))
	a.applyMode()
	a.typeByte('A')
	a.setMode("uni")
	if !a.uni() || uniRune(uniCode(a.doc.get(0, 0))) != 'A' {
		t.Errorf("Atari page: %s %q", a.mode, uniRune(uniCode(a.doc.get(0, 0))))
	}
}

// runAction does what a key bound to the action would.
func runAction(id string) { actionByID[id].run() }

func TestUniTyping(t *testing.T) {
	a := uniFApp(t)
	textInput("a⣿é🬀╳")
	want := []rune("a⣿é🬀╳")
	for i, r := range want {
		if got := uniRune(uniCode(a.doc.get(i, 0))); got != r {
			t.Errorf("cell %d: %q, want %q", i, got, r)
		}
	}
	textInput("😀") // no cell holds it
	if !strings.Contains(a.message, "not") || uniCode(a.doc.get(5, 0)) == 0x1F600 {
		t.Errorf("an emoji: %q", a.message)
	}
	// The brush in any colour.
	a.setUniFg(201)
	a.setUniBg(rgbColour(0x123456))
	a.ital = true
	textInput("Z")
	fg, bg, fl := styleParts(uniPair(a.doc.get(a.cx-1, 0)))
	if fg != 201 || bg != rgbFlag|0x123456 || fl != italic {
		t.Errorf("Z in %s/%s %X", colourName(fg), colourName(bg), fl)
	}
	// One of the 16 again (a click on a swatch).
	a.setFg(3)
	a.ital = false
	if a.uniFg() != 3 || a.uniBg() != rgbFlag|0x123456 {
		t.Errorf("after setFg: %d %d", a.uniFg(), a.uniBg())
	}
	// F-keys type Unicode.
	a.fset = 0
	a.typeByte(a.fkeys["uni"][0][3])
	if uniRune(uniCode(a.doc.get(a.cx-1, 0))) != '█' {
		t.Errorf("F4 typed %q", uniRune(uniCode(a.doc.get(a.cx-1, 0))))
	}
	// Pick takes it all back.
	a.pick(1, 0)
	if uniRune(a.brush) != '⣿' || a.ufgOn {
		t.Errorf("picked %q fg %d", uniRune(a.brush), a.uniFg())
	}
	a.pick(5, 0)
	if a.uniFg() != 201 || a.uniBg() != rgbFlag|0x123456 || !a.ital {
		t.Errorf("picked %d/%d", a.uniFg(), a.uniBg())
	}
	// The text that goes to the clipboard is the characters.
	if got := docText(a.doc, a.runeOf, 0, 0, 6, 1)[0]; got != "a⣿é🬀╳Z" {
		t.Errorf("text %q", got)
	}
}

func TestUniHalfAndShapes(t *testing.T) {
	a := uniFApp(t)
	a.setUniFg(208)
	a.setUniBg(rgbColour(0x00FF80))
	a.uniPaintHalf(0, 0, a.halfFg()) // top half of cell 0,0
	if v := a.doc.get(0, 0); uniRune(uniCode(v)) != '▀' {
		t.Fatalf("top half: %q", uniRune(uniCode(v)))
	} else if fg, bg := uniColoursOf(v); fg != 208 || bg != 0 {
		t.Errorf("top half %d/%d", fg, bg)
	}
	a.uniPaintHalf(0, 1, a.halfBg()) // bottom half in the background colour
	if fg, bg := uniColoursOf(a.doc.get(0, 0)); fg != 208 || bg != rgbFlag|0x00FF80 {
		t.Errorf("both halves %s/%s", colourName(fg), colourName(bg))
	}
	a.uniPaintHalf(0, 0, rgbFlag|0x00FF80) // both the same: a space on that colour
	if v := a.doc.get(0, 0); uniCode(v) != ' ' {
		t.Errorf("one colour: %q", uniRune(uniCode(v)))
	} else if _, bg := uniColoursOf(v); bg != rgbFlag|0x00FF80 {
		t.Errorf("one colour bg %s", colourName(bg))
	}
	// Boxes in Unicode lines.
	a.shapeStyle = "round"
	if box := a.boxChars(); string([]rune{rune(box[2]), rune(box[5])}) != "╭╯" {
		t.Errorf("round box %v", box)
	}
	// The shade brush steps through ░▒▓█.
	v, ok := a.shadeCell(blank, true)
	if !ok || uniRune(uniCode(v)) != '░' {
		t.Errorf("shade: %q", uniRune(uniCode(v)))
	}
	v, _ = a.shadeCell(v, true)
	if uniRune(uniCode(v)) != '▒' {
		t.Errorf("shade 2: %q", uniRune(uniCode(v)))
	}
}

func TestUniFlips(t *testing.T) {
	a := uniFApp(t)
	for _, c := range []struct {
		r    rune
		k    int
		want rune
	}{
		{'▌', flipH, '▐'}, {'▀', flipV, '▄'}, {'╔', flipH, '╗'}, {'╔', flipV, '╚'}, {'/', flipH, '\\'},
		{'◢', flipH, '◣'}, {'─', turnCW, '│'}, {'┌', turnCW, '┐'}, {'A', flipH, 'A'}, {'⢇', flipV, '⡎'}, // dots 1238 -> 2347
		{'▘', flipH, '▝'}, {'⡀', flipH, '⢀'}, {'⠁', flipV, '⡀'}, {'\U0001FB00', flipH, '\U0001FB01'}, {'\U0001FB00', flipV, '\U0001FB0F'},
		{'\U0001FB13', flipH, '\U0001FB26'}, {'\U0001FB14', flipH, '\U0001FB27'}, // SEXTANT-35 -> 46, 235 -> 146
	} {
		code, _ := uniCodeOf(c.r)
		if got := uniRune(uniMirror(code, c.k)); got != c.want {
			t.Errorf("%q turned %d: %q, want %q", c.r, c.k, got, c.want)
		}
	}
	// Braille and sextants: flipping twice gives each back.
	for r := rune(0x2800); r <= 0x28FF; r++ {
		for _, k := range []int{flipH, flipV} {
			if m, _ := uniDotsTurned(r, k); func() rune { b, _ := uniDotsTurned(m, k); return b }() != r {
				t.Errorf("%q flipped twice", r)
			}
		}
	}
	for r := rune(0x1FB00); r <= 0x1FB3B; r++ {
		for _, k := range []int{flipH, flipV} {
			c, _ := uniCodeOf(r)
			if uniRune(uniMirror(uniMirror(c, k), k)) != r {
				t.Errorf("%U flipped twice", r)
			}
		}
	}
	// Invert swaps the colours and keeps the rest.
	v := uniCell('x', styleOf(200, rgbFlag|0x010203, underline))
	w := a.invertCell(v)
	if fg, bg, fl := styleParts(uniPair(w)); fg != rgbFlag|0x010203 || bg != 200 || fl != underline || uniCode(w) != 'x' {
		t.Errorf("inverted %s/%s %X", colourName(fg), colourName(bg), fl)
	}
	// Mirror a selection.
	a.doc.set(0, 0, uniCell('▌', pairOf(100, 0)))
	a.sel = &Sel{0, 0, 1, 0}
	a.blockOp(flipH)
	if uniRune(uniCode(a.doc.get(1, 0))) != '▐' {
		t.Errorf("mirrored block: %q", uniRune(uniCode(a.doc.get(1, 0))))
	}
}

// A block copied on a PC page and pasted on a Unicode page keeps its characters, and back.
func TestUniPasteBetweenPages(t *testing.T) {
	a := testApp(t, "pc")
	a.applyMode()
	a.doc.set(0, 0, makeCell(0xDB, 12, 1, 0))
	a.clip = a.doc.copyRect(0, 0, 1, 1)
	uniTab := a.newTab("uni")
	a.focusTab(uniTab)
	a.applyMode()
	a.startPaste(a.clip)
	a.stamp(2, 0, false)
	if v := a.doc.get(2, 0); uniRune(uniCode(v)) != '█' {
		t.Errorf("pasted on Unicode: %q", uniRune(uniCode(v)))
	}
	a.doc.set(3, 0, uniCell('▄', pairOf(196, 4)))
	a.clip = a.doc.copyRect(3, 0, 1, 1)
	a.focusTab(a.tabs[0])
	a.applyMode()
	a.startPaste(a.clip)
	a.stamp(5, 0, false)
	if v := a.doc.get(5, 0); cellCh(v) != 0xDC || cellBg(v) != 4 {
		t.Errorf("pasted on PC: %08X", v)
	}
}

func TestUniAutosave(t *testing.T) {
	a := uniFApp(t)
	r := rand.New(rand.NewSource(6))
	a.doc = uniFPage(r, 60, 10, func() (int, int) { return uniFColour(r, true), uniFColour(r, true) }, true)
	a.dirty, a.gen = true, nextGen()
	want := a.doc.clone()
	lastAutosave = time.Now()
	a.autosaveTick(time.Now().Add(3 * time.Minute))
	if a.Tab.autoID == "" {
		t.Fatal("not autosaved")
	}
	// Another run numbers the styles differently: make some first.
	for i := 0; i < 50; i++ {
		styleOf(16+i, 17, italic)
	}
	b := restart(t, "pc")
	if !offerAutosaves() {
		t.Fatal("no offer")
	}
	b.dialog.(*AutosaveReq).reopen()
	if !b.uni() {
		t.Fatalf("came back as %s", b.mode)
	}
	uniFSame(t, "autosave", b.doc, want)
}

// The table of styles lets go of what no page uses, and when it is full new
// colours take the nearest.
func TestUniStyleTable(t *testing.T) {
	uniStyles.Lock()
	oldList, oldIndex, oldFull := uniStyles.list, uniStyles.index, uniStyles.full
	uniStyles.list, uniStyles.index, uniStyles.full = nil, nil, false
	uniStyles.Unlock()
	defer func() {
		uniStyles.Lock()
		uniStyles.list, uniStyles.index, uniStyles.full = oldList, oldIndex, oldFull
		uniStyles.Unlock()
	}()
	a := uniFApp(t)
	live := uniCell('L', styleOf(rgbFlag|0xABCDEF, 200, underline))
	a.doc.set(0, 0, live)
	a.pushUndo()
	a.doc.set(1, 0, uniCell('M', pairOf(rgbFlag|0x111111, 17)))
	a.clip = &Block{w: 1, h: 1, cells: []uint32{uniCell('C', pairOf(99, rgbFlag|0x222222))}, uni: true}
	for i := 0; uniPair(live) > 0 && i < (uniPairMax-256)*3/4; i++ {
		pairOf(rgbFlag|i, 1)
	}
	if freed := a.uniCompact(); freed < (uniPairMax-256)*3/4-10 {
		t.Fatalf("freed %d", freed)
	}
	for _, c := range []struct {
		v      uint32
		fg, bg int
		fl     uint32
	}{
		{a.doc.get(0, 0), rgbFlag | 0xABCDEF, 200, underline},
		{a.undo[0].doc.get(0, 0), rgbFlag | 0xABCDEF, 200, underline},
		{a.doc.get(1, 0), rgbFlag | 0x111111, 17, 0},
		{a.clip.cells[0], 99, rgbFlag | 0x222222, 0},
	} {
		if fg, bg, fl := styleParts(uniPair(c.v)); fg != c.fg || bg != c.bg || fl != c.fl {
			t.Errorf("after compacting: %s/%s %X, want %s/%s %X", colourName(fg), colourName(bg), fl, colourName(c.fg), colourName(c.bg), c.fl)
		}
	}
	// Full: the nearest.
	for i := 0; len(uniStyles.list) < uniPairMax-256; i++ {
		pairOf(rgbFlag|(i*7919%0xFFFFFF), 2)
	}
	if fg, bg, fl := styleParts(styleOf(rgbFlag|0xABCDEE, 200, underline)); fg != rgbFlag|0xABCDEF || bg != 200 || fl != underline {
		t.Errorf("nearest of #ABCDEE/200 underlined: %s/%s %X", colourName(fg), colourName(bg), fl)
	}
	if uniFullNote() == "" {
		t.Error("no note when full")
	}
}

// ---------------------------------------------------------------- exports and the rest

func TestUniHTML(t *testing.T) {
	a := uniFApp(t)
	a.doc.set(0, 0, uniCell('⣿', styleOf(196, rgbFlag|0x102030, underline)))
	a.doc.set(1, 0, uniCell('<', pairOf(7, 0)))
	out, err := a.encode("html")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"⣿", "color:#ff0000", "background:#102030", "text-decoration:underline", "&lt;", `charset="utf-8"`} {
		if !strings.Contains(string(out), want) {
			t.Errorf("no %q in the page", want)
		}
	}
	if txt, _ := a.encode("utf8"); string(txt) != "⣿<\n"+strings.Repeat("\n", a.doc.rows-1) {
		t.Errorf("UTF-8 text %q", txt)
	}
	if _, err := a.encode("ansi"); err == nil {
		t.Error("a Unicode page saved as CP437 ANSI")
	}
}

// The cells are drawn in their colours: the 16 from the palette, the 256 and 24-bit as they are.
func TestUniDrawing(t *testing.T) {
	a := uniFApp(t)
	a.paletteFor["uni"] = "vga"
	a.applyMode()
	fb := &FB{}
	fb.resize(16, 16)
	fb.clip = Rect{0, 0, 16, 16}
	for _, c := range []struct {
		v      uint32
		fg, bg uint32
	}{
		{uniCell('█', pairOf(196, 0)), 0xFFFF0000, 0},
		{uniCell(' ', pairOf(7, rgbFlag|0x123456)), 0, 0xFF123456},
		{uniCell('█', pairOf(9, 0)), 0xFFFF5555, 0},
		{uniCell(0xD800+0x3A, pairOf(rgbFlag|0x0000FE, 0)), 0xFF0000FE, 0}, // 🬻: the lower blocks at least
	} {
		a.drawCell(fb, 0, 0, 1, c.v)
		px := fb.px[15*fb.w+3]
		want := c.fg
		if want == 0 {
			want = c.bg
		}
		if px != want {
			t.Errorf("cell %08X: pixel %08X, want %08X", c.v, px, want)
		}
	}
	// A character no font has: a box, not nothing.
	a.drawCell(fb, 0, 0, 1, uniCell(0x4E00, pairOf(15, 0)))
	if fb.px[2*fb.w+1] != 0xFFFFFFFF {
		t.Error("no box for a character without a glyph")
	}
	// The PNG export draws the same.
	if out, err := a.encode("png"); err != nil || len(out) < 100 {
		t.Errorf("png: %v", err)
	}
}

// Animate on a Unicode page: every pattern makes frames that end on the picture.
func TestUniAnimate(t *testing.T) {
	a := uniFApp(t)
	r := rand.New(rand.NewSource(7))
	a.doc = uniFPage(r, 40, 12, func() (int, int) { return uniFColour(r, true), uniFColour(r, true) }, true)
	a.applyMode()
	d := a.doc.clone()
	n := d.cols * d.rows
	for pat := 0; pat < nPatterns; pat++ {
		s := animSpec{pat: pat, n: 40, hold: 40, seed: 3}
		docs, _, _ := a.transitionFrames(d, s, maxPageCells)
		if len(docs) == 0 {
			continue
		}
		last := docs[len(docs)-1]
		if !slices.Equal(last.cells[:n], d.cells[:n]) && !strings.HasSuffix(patterns[pat].id, "out") {
			t.Errorf("%s: does not end on the picture", patterns[pat].id)
		}
		for i, f := range docs {
			for c := 0; c < n; c++ {
				if p := uniPair(f.cells[c]); p >= 256 {
					uniStyles.RLock()
					bad := p-256 >= len(uniStyles.list)
					uniStyles.RUnlock()
					if bad {
						t.Fatalf("%s frame %d: style %d was never made", patterns[pat].id, i, p)
					}
				}
			}
		}
	}
}

// Saving a page as .dur, .irc and UTF-8 ANSI keeps it as the page's own file (frames too for .dur).
func TestUniOwnFiles(t *testing.T) {
	a := uniFApp(t)
	dir := t.TempDir()
	for _, f := range []struct{ fmt, name string }{{"dur", "a.dur"}, {"mirc", "a.irc"}, {"uniansi", "a.utf8.ans"}} {
		a.focusTab(a.newTab("uni"))
		a.applyMode()
		a.typeByte('▀')
		if f.fmt == "dur" {
			a.newFrame()
			a.typeByte('▄')
		}
		p := filepath.Join(dir, f.name)
		if !a.writeOut(p, f.fmt) {
			t.Fatal(a.message)
		}
		if a.dirty || a.srcKind != f.fmt {
			t.Errorf("%s: dirty %v kind %q", f.fmt, a.dirty, a.srcKind)
		}
		b, _ := os.ReadFile(p)
		if bytes.Contains(b, []byte("SAUCE")) {
			t.Errorf("%s: a SAUCE record on a new Unicode page", f.name)
		}
		a.focusTab(a.newTab("pc"))
		a.loadBytes(b, f.name, p, "auto")
		a.applyMode()
		if !a.uni() || a.srcKind != f.fmt {
			t.Errorf("%s opened as %s %s", f.name, a.mode, a.srcKind)
		}
		if f.fmt == "dur" && a.nFrames() != 2 {
			t.Errorf("%s: %d frames", f.name, a.nFrames())
		}
	}
}

// A PC page converted to Unicode looks the same, pixel for pixel ("VGA + Unifont").
func TestUniConvertedLooksTheSame(t *testing.T) {
	b, err := os.ReadFile("testdata/alove/bs-alove.bin")
	if err != nil {
		t.Skip(err)
	}
	a := testApp(t, "pc")
	a.loadBytes(b, "bs-alove.bin", "", "auto")
	a.paletteFor["pc"] = "vga"
	a.applyMode()
	a.blink = true
	before := a.renderDoc(a.doc, 1)
	a.setMode("uni")
	a.applyMode()
	after := a.renderDoc(a.doc, 1)
	if before.w != after.w || before.h != after.h {
		t.Fatalf("%dx%d, then %dx%d", before.w, before.h, after.w, after.h)
	}
	for i := range before.px {
		if before.px[i] != after.px[i] {
			t.Fatalf("pixel %d,%d: %08X, then %08X", i%before.w, i/before.w, before.px[i], after.px[i])
		}
	}
}

// A PC page without iCE colours blinks; converted, those cells stand still on their dark background.
func TestUniConvertBlink(t *testing.T) {
	a := testApp(t, "pc")
	a.applyMode()
	a.ice = false
	a.doc.set(0, 0, makeCell('B', 14, 9, 0))
	a.dirty = true
	a.setMode("uni")
	if fg, bg := uniColoursOf(a.doc.get(0, 0)); fg != 14 || bg != 1 || !strings.Contains(a.message, "blinking") {
		t.Errorf("blinking cell: %d/%d, said %q", fg, bg, a.message)
	}
}

// The W field works on every Unicode file: an opened .dur and .irc get the new width too.
func TestUniWidth(t *testing.T) {
	b, err := os.ReadFile("testdata/durdraw/cm-eye.dur")
	if err != nil {
		t.Skip(err)
	}
	a := uniFApp(t)
	uniFOpen(a, b, "cm-eye.dur")
	a.setCols(100)
	if a.doc.cols != 100 || a.frameDoc(1).cols != 100 {
		t.Errorf("width %d, frame 2 %d", a.doc.cols, a.frameDoc(1).cols)
	}
	uniFOpen(a, []byte("\x034,2ab\n"), "x.irc")
	a.setCols(10)
	if a.doc.cols != 10 || uniCode(a.doc.get(1, 0)) != 'b' {
		t.Errorf("irc width %d", a.doc.cols)
	}
	// Crop works on a Unicode page.
	a.sel = &Sel{1, 0, 1, 0}
	a.cropToSel()
	if a.doc.cols != 1 || uniCode(a.doc.get(0, 0)) != 'b' {
		t.Errorf("cropped: %d %q", a.doc.cols, uniRune(uniCode(a.doc.get(0, 0))))
	}
}
