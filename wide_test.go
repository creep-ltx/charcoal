package main

import (
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
