package main

import (
	"bytes"
	"strings"
	"testing"
)

// wideFDoc makes a Unicode page from rows of text: each wide character takes
// its cell and the next (a lead and its right half).
func wideFDoc(t *testing.T, cols int, rows ...string) *Doc {
	t.Helper()
	d := newDoc(cols, len(rows))
	d.uni = true
	for y, s := range rows {
		x := 0
		for _, r := range s {
			code, ok := uniCodeOf(r)
			if !ok {
				t.Fatalf("no cell holds %q", r)
			}
			v := uniCell(code, 7)
			d.set(x, y, v)
			x++
			if uniWideRune(r) {
				d.set(x, y, uniContOf(v))
				x++
			}
		}
	}
	return d
}

// wideFRow is a row as text: a right half shows as "|", so a test sees where the halves are.
func wideFRow(d *Doc, y int) string {
	var s []rune
	for x := 0; x < d.cols; x++ {
		v := d.get(x, y)
		if uniIsCont(v) {
			s = append(s, '|')
			continue
		}
		s = append(s, uniRune(uniCode(v)))
	}
	return string(s)
}

// wideFCheck fails when the page breaks the rule: every wide character followed by its right half.
func wideFCheck(t *testing.T, d *Doc, what string) {
	t.Helper()
	if x, y := wideOK(d); x >= 0 {
		t.Fatalf("%s: half a wide character at %d,%d: %q", what, x, y, wideFRow(d, y))
	}
}

func TestWideTable(t *testing.T) {
	for _, c := range []struct {
		r    rune
		wide bool
	}{
		{'a', false}, {'é', false}, {'═', false}, {'▀', false}, {'█', false}, {'░', false}, // ambiguous or narrow: art stays narrow
		{'ｱ', false}, {'❤', false}, {0x1FB00, false}, {0x1CD00, false},
		{'中', true}, {'ア', true}, {'가', true}, {'Ａ', true}, {0x3000, true}, {'😀', true}, {'⌚', true}, {'✅', true},
		{'☰', true}, {'⚊', true}, {0x4DC0, true}, // wide since Unicode 16: our table, not Go's
		{0x20000, true}, {0x1100, true}, {0x10FFFF, false}, {0x30000, true},
	} {
		if got := uniWideRune(c.r); got != c.wide {
			t.Errorf("uniWideRune(%U) = %v, want %v", c.r, got, c.wide)
		}
	}
	if uniWideVersion != "17.0.0" {
		t.Errorf("table of Unicode %s", uniWideVersion)
	}
	for i := 1; i < len(uniWideRanges); i++ {
		if uniWideRanges[i][0] <= uniWideRanges[i-1][1]+1 || uniWideRanges[i][0] > uniWideRanges[i][1] {
			t.Fatalf("ranges out of order at %d", i)
		}
	}
}

func TestWideNormal(t *testing.T) {
	d := wideFDoc(t, 8, "a中b", "中中中中")
	wideFCheck(t, d, "made")
	if got := wideFRow(d, 0); got != "a中|b    " {
		t.Fatalf("row %q", got)
	}
	// Cut the second row's pairs: a lone right half and a lone character become spaces.
	d.set(0, 1, uniCell('x', 7))
	d.set(7, 1, uniCell('y', 7))
	if n := wideNormal(d); n != 2 {
		t.Errorf("normalised %d cells", n)
	}
	if got := wideFRow(d, 1); got != "x 中|中| y" {
		t.Errorf("row %q", got)
	}
	wideFCheck(t, d, "normalised")
	// A page without wide characters is left alone.
	p := wideFDoc(t, 4, "abcd")
	if wideNormal(p) != 0 || wideIn(p) {
		t.Error("a narrow page changed")
	}
}

// wideFPixels gives the fg/bg pattern of row r of a rendered picture, cells x0..x1, as a bit string.
func wideFPixels(fb *FB, r, w int, fgc uint32) string {
	s := make([]byte, w)
	for x := 0; x < w; x++ {
		s[x] = '.'
		if fb.px[r*fb.w+x] == fgc {
			s[x] = '#'
		}
	}
	return string(s)
}

func TestWideDrawn(t *testing.T) {
	a := testApp(t, "uni")
	a.fontFor["uni"] = "univga"
	d := wideFDoc(t, 4, "中a")
	fb := a.renderDoc(d, 1)
	g, ok := uniWideGlyph('中')
	if !ok {
		t.Fatal("Unifont has no 中")
	}
	fgc := 0xFF000000 | colourRGB(7, &a.pal)
	for r := 0; r < 16; r++ {
		want := make([]byte, 16)
		for x := 0; x < 16; x++ {
			want[x] = '.'
			if g[r]&(0x8000>>x) != 0 {
				want[x] = '#'
			}
		}
		if got := wideFPixels(fb, r, 16, fgc); got != string(want) {
			t.Fatalf("row %d: %s, want %s", r, got, want)
		}
	}
	// A lone character (an old autosave) is squeezed into its cell; a lone right half is a space.
	lone := newDoc(2, 1)
	lone.uni = true
	lone.cells[0] = uniCell('中', 7)
	lone.cells[1] = uniCell(' ', 7)
	fb = a.renderDoc(lone, 1)
	small, _ := uniWideSmall('中')
	for r := 0; r < 16; r++ {
		want := make([]byte, 16)
		for x := 0; x < 16; x++ {
			want[x] = '.'
			if x < 8 && small[r]&(0x80>>x) != 0 {
				want[x] = '#'
			}
		}
		if got := wideFPixels(fb, r, 16, fgc); got != string(want) {
			t.Fatalf("lone row %d: %s, want %s", r, got, want)
		}
	}
	lone.cells[0] = uniContOf(uniCell('中', 7))
	if fb = a.renderDoc(lone, 1); wideFPixels(fb, 8, 16, fgc) != "................" {
		t.Error("a lone right half shows something")
	}
	// The character table can show kanji (squeezed), so they can be typed.
	if !a.uniHas(0x4E2D) {
		t.Error("中 cannot be shown")
	}
}

func wideFSame(t *testing.T, what string, got, want *Doc) {
	t.Helper()
	if got.cols != want.cols {
		t.Fatalf("%s: %d columns, want %d", what, got.cols, want.cols)
	}
	for y := 0; y < want.rows; y++ {
		if g, w := wideFRow(got, y), wideFRow(want, y); g != w {
			t.Fatalf("%s: row %d %q, want %q", what, y, g, w)
		}
		for x := 0; x < want.cols; x++ {
			if g, w := got.get(x, y), want.get(x, y); g != w {
				t.Fatalf("%s: cell %d,%d %08X, want %08X", what, x, y, g, w)
			}
		}
	}
	wideFCheck(t, got, what)
}

func TestWideAnsi(t *testing.T) {
	d := wideFDoc(t, 10, "ab中cかな", "漢字 x 가", "ｱ═Ａ")
	d.set(2, 0, uniCell('中', pairOf(196, 4)))
	d.set(3, 0, uniContOf(d.get(2, 0)))
	b := writeUniAnsi(d)
	if bytes.Contains(b, []byte("中 ")) || !bytes.Contains(b, []byte("漢字 x 가")) {
		t.Errorf("written %q", b)
	}
	got, _ := parseUniArt(b, d.cols)
	wideFSame(t, "UTF-8 ANSI", got, d)

	// A wide character does not fit in the last column: it goes to the next row.
	got, _ = parseUniArt([]byte("abc中\n"), 4)
	if r0, r1 := wideFRow(got, 0), wideFRow(got, 1); r0 != "abc " || r1 != "中|  " {
		t.Errorf("wrapped %q %q", r0, r1)
	}
	// Writing over half of one leaves a space in the other half (as a terminal).
	got, _ = parseUniArt([]byte("中中中\r\x1b[1Cx\x1b[1Cy"), 8)
	if r := wideFRow(got, 0); r != " x y中|  " {
		t.Errorf("overwritten %q", r)
	}
	// The width of a line with wide characters is its columns.
	if got, _ = parseUniArt([]byte(strings.Repeat("中", 50)+"\n"), 0); got.cols != 100 {
		t.Errorf("%d columns", got.cols)
	}
}

// A file whose tool counted a kanji as one column pads its line with blanks to
// the others' length (MC_FRESHER-RX-78.ANS, Mistigris 2025-08): the blanks past
// the page are dropped, the kanji stand where a terminal shows them.
func TestWidePaddedLine(t *testing.T) {
	art := strings.Repeat("#", 90) + "\n" +
		"  \x1b[3;38;5;12m機\x1b[0m  \x1b[3;38;5;12m動\x1b[0m" + strings.Repeat(" ", 84) + "\n" +
		strings.Repeat("#", 90) + "\n"
	d, _ := parseUniArt([]byte(art), 0)
	if d.cols != 90 || d.rows != 3 {
		t.Fatalf("%dx%d", d.cols, d.rows)
	}
	if r := wideFRow(d, 1); r != "  機|  動|"+strings.Repeat(" ", 82) {
		t.Errorf("row %q (%d), want %q", r, len([]rune(r)), "  機|  動|"+strings.Repeat(" ", 82))
	}
	if r := wideFRow(d, 2); r != strings.Repeat("#", 90) {
		t.Errorf("next row %q", r)
	}
	// Blanks past the page that something follows wrap, as before.
	d, _ = parseUniArt([]byte("中"+strings.Repeat("a", 3)+"   b\n"), 6)
	if r := wideFRow(d, 1); r != "  b   " {
		t.Errorf("wrapped %q", r)
	}
}

func TestWideMircTextHTML(t *testing.T) {
	a := testApp(t, "uni")
	d := wideFDoc(t, 6, "a中b", "かな!")
	b, _, _ := writeMirc(d, &a.pal)
	got := parseMirc(b)
	got = got.withCols(6)
	wideFSame(t, "mIRC", got, d)
	if lines := docText(d, a.runeOf, 0, 0, 0, 0); lines[0] != "a中b" || lines[1] != "かな!" {
		t.Errorf("text %q", lines)
	}
	h := string(writeUniHTML(d, &a.pal, "x"))
	if !strings.Contains(h, `<span class="w">中</span>b`) || !strings.Contains(h, ".w{display:inline-block;width:2ch") {
		t.Errorf("HTML %s", h)
	}
	if h := string(writeUniHTML(wideFDoc(t, 3, "abc"), &a.pal, "x")); strings.Contains(h, ".w{") {
		t.Error("a page without wide characters has the wide style")
	}
}
