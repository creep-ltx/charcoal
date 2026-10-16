package main

import (
	"bytes"
	"fmt"
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

// wideFApp is a Unicode page of the given rows, cursor at 0,0.
func wideFApp(t *testing.T, cols int, rows ...string) *App {
	t.Helper()
	a := testApp(t, "uni")
	a.doc = wideFDoc(t, cols, rows...)
	a.cx, a.cy = 0, 0
	return a
}

func TestWideTyping(t *testing.T) {
	a := wideFApp(t, 6, "      ", "      ")
	textInput("a中b")
	if r := wideFRow(a.doc, 0); r != "a中|b  " || a.cx != 4 {
		t.Fatalf("typed %q, cursor %d", r, a.cx)
	}
	textInput("かx") // か does not fit in the last two? it does: 4,5; x goes to the next row
	if r0, r1 := wideFRow(a.doc, 0), wideFRow(a.doc, 1); r0 != "a中|bか|" || r1 != "x     " {
		t.Fatalf("rows %q %q", r0, r1)
	}
	a.cx, a.cy = 5, 1
	textInput("字") // the last column: to the next row
	if r := wideFRow(a.doc, 2); r != "字|    " || a.cy != 2 {
		t.Errorf("wrapped %q", r)
	}
	// Overwriting either half leaves a space in the other.
	a.cx, a.cy = 1, 0
	textInput("y")
	if r := wideFRow(a.doc, 0); r != "ay bか|" {
		t.Errorf("left half overwritten: %q", r)
	}
	a.doc.set(5, 0, uniCell('z', 7)) // (a click of the Draw tool on a right half goes through setCell)
	a.doc.set(5, 0, uniContOf(uniCell('か', 7)))
	a.setCell(5, 0, uniCell('z', 7))
	if r := wideFRow(a.doc, 0); r != "ay b z" {
		t.Errorf("right half overwritten: %q", r)
	}
	wideFCheck(t, a.doc, "typing")
	// Insert mode makes room for both cells.
	a = wideFApp(t, 6, "ab中c")
	a.insert = true
	a.cx = 1
	textInput("字")
	if r := wideFRow(a.doc, 0); r != "a字|b中|" { // (c falls off the end)
		t.Errorf("inserted %q", r)
	}
	wideFCheck(t, a.doc, "insert")
}

func TestWideCursorDelete(t *testing.T) {
	a := wideFApp(t, 8, "a中b字c")
	editorKey("Right", false, false, false)
	editorKey("Right", false, false, false)
	if a.cx != 3 {
		t.Errorf("Right went to %d, want past the right half (3)", a.cx)
	}
	a.cx = 2
	a.moved()
	if a.cx != 1 {
		t.Errorf("on a right half the cursor goes to the character: %d", a.cx)
	}
	editorKey("Delete", false, false, false)
	if r := wideFRow(a.doc, 0); r != "ab字|c   " {
		t.Errorf("deleted %q", r)
	}
	a.cx = 4
	a.insert = true
	editorKey("Backspace", false, false, false)
	if r := wideFRow(a.doc, 0); r != "abc     " || a.cx != 2 {
		t.Errorf("backspaced %q, cursor %d", r, a.cx)
	}
	a = wideFApp(t, 8, "a中b")
	a.cx = 3
	editorKey("Backspace", false, false, false)
	if r := wideFRow(a.doc, 0); r != "a  b    " || a.cx != 1 {
		t.Errorf("backspace over a wide character: %q, cursor %d", r, a.cx)
	}
}

func TestWideFillsAndLines(t *testing.T) {
	code, _ := uniCodeOf('中')
	for _, c := range []struct {
		name string
		do   func(a *App)
		want string
	}{
		{"fill selection", func(a *App) { a.sel = &Sel{1, 0, 5, 0}; a.fillSel() }, ".中|中|..."},
		{"left to right", func(a *App) {
			for x := 0; x < 7; x++ {
				a.setCell(x, 0, a.brushCell())
			}
		}, "中|中|中|中|"},
		{"right to left", func(a *App) {
			for x := 6; x >= 0; x-- {
				a.setCell(x, 0, a.brushCell())
			}
		}, "中|中|中|中|"},
		{"flood fill", func(a *App) { a.floodFill(3, 0) }, "中|中|中|中|"},
	} {
		a := wideFApp(t, 8, "........")
		a.brush = code
		c.do(a)
		if r := wideFRow(a.doc, 0); r != c.want {
			t.Errorf("%s: %q, want %q", c.name, r, c.want)
		}
		wideFCheck(t, a.doc, c.name)
	}
}

func TestWideBlocks(t *testing.T) {
	a := wideFApp(t, 8, "a中b字c", "xyzwvuts")
	// A selection that cuts a wide character holds all of it.
	a.sel = &Sel{2, 0, 4, 1}
	if r := a.selRect(); r.x != 1 || r.w != 5 {
		t.Errorf("selection %+v", r)
	}
	a.copySel(false)
	if a.clipText != "中b字\nyzwvu" {
		t.Errorf("text %q", a.clipText)
	}
	a.blockOp(flipH)
	if r := wideFRow(a.doc, 0); r != "a字|b中|c " {
		t.Errorf("mirrored %q", r)
	}
	wideFCheck(t, a.doc, "mirror")
	a.blockOp(turnCW)
	if !strings.Contains(a.message, "cannot be turned") || wideFRow(a.doc, 0) != "a字|b中|c " {
		t.Errorf("turned: %q", a.message)
	}
	// Stamping a block keeps its wide characters whole and cuts those it lands on.
	a.cx, a.cy = 0, 1
	a.startPaste(a.textBlock("漢x"))
	a.stamp(2, 1, false)
	if r := wideFRow(a.doc, 1); r != "xu漢|xyts" { // (row 1 was mirrored too)
		t.Errorf("stamped %q", r)
	}
	a.startPaste(a.textBlock("q"))
	a.stamp(3, 1, false)
	if r := wideFRow(a.doc, 1); r != "xu qxyts" {
		t.Errorf("stamped over a right half %q", r)
	}
	wideFCheck(t, a.doc, "stamp")
	// Picking a right half picks the character.
	a.pick(2, 0)
	if want, _ := uniCodeOf('字'); a.brush != want {
		t.Errorf("picked %X", a.brush)
	}
}

func TestWideColumns(t *testing.T) {
	a := wideFApp(t, 8, "a中b字c")
	a.cx = 2
	a.deleteColumn()
	if r := wideFRow(a.doc, 0); r != "a b字|c  " {
		t.Errorf("column deleted: %q", r)
	}
	a = wideFApp(t, 8, "a中b字c")
	a.cx = 2
	a.insertColumn()
	if r := wideFRow(a.doc, 0); r != "a   b字|c" {
		t.Errorf("column inserted: %q", r)
	}
	wideFCheck(t, a.doc, "insert column")
	a = wideFApp(t, 8, "a中b字c")
	a.setCols(8)
	a.doc = a.doc.withCols(5)
	if r := wideFRow(a.doc, 0); r != "a中|b " {
		t.Errorf("narrower: %q", r)
	}
	wideFCheck(t, a.doc, "W")
}

func TestWideLayersAndConversions(t *testing.T) {
	a := wideFApp(t, 6, "中字ab")
	a.newLayer(false)
	a.setCell(1, 0, uniCell('x', 7)) // on the layer above: covers 中's right half
	seen := a.seen()
	if r := wideFRow(seen, 0); r != " x字|ab" {
		t.Errorf("flattened %q", r)
	}
	wideFCheck(t, seen, "flatten")
	a.mergeDown()
	if r := wideFRow(a.doc, 0); r != " x字|ab" {
		t.Errorf("merged %q", r)
	}
	wideFCheck(t, a.doc, "merge down")
	// Back to a PC page: a question mark and a space.
	d, changed := docFromUni(wideFDoc(t, 4, "中a"), false, &vga16)
	if changed != 1 || cellCh(d.get(0, 0)) != '?' || cellCh(d.get(1, 0)) != ' ' || cellCh(d.get(2, 0)) != 'a' {
		t.Errorf("to PC: %d changed, %v", changed, d.cells[:3])
	}
	// An old autosave's wide character in one cell gets the blank after it.
	old := newDoc(4, 1)
	old.uni = true
	old.cells[0], old.cells[2] = uniCell('⭐', 7), uniCell('☕', 7)
	old.cells[3] = uniCell('x', 7)
	wideMendOld(old)
	if r := wideFRow(old, 0); r != "⭐|☕x" {
		t.Errorf("mended %q", r)
	}
	// Transitions never leave half a character.
	b := testApp(t, "uni")
	from, to := wideFDoc(t, 8, "中中中中", "字字字字"), wideFDoc(t, 8, "abcdefgh", "かかかか")
	for _, pat := range []int{patMatrix, patDecrypt, patDrop, patAssemble, patShine, patShadeIn, patMelt, patPowerOn} {
		for i, f := range b.effectFrames(from, to, pat, 1, 0) {
			if f != nil {
				wideFCheck(t, f, fmt.Sprintf("effect %d frame %d", pat, i))
			}
		}
	}
}

func TestWideDur(t *testing.T) {
	d := wideFDoc(t, 10, "a中b字c", "かなかなかな")
	d.set(1, 0, uniCell(uniCode(d.get(1, 0)), pairOf(3, 1)))
	d.set(2, 0, uniContOf(d.get(1, 0)))
	out, _ := writeDur([]*Doc{d}, []int{100}, "", "")
	art, err := parseDur(out)
	if err != nil {
		t.Fatal(err)
	}
	wideFSame(t, ".dur", art.docs[0], d)
	// A line as durdraw keeps it, a character per cell: a wide character moves the
	// rest on (as durdraw's export shows it); what goes past the page is dropped.
	js := `{"DurMovie":{"formatVersion":8,"colorFormat":"16","columns":4,"lines":1,"frames":[{"contents":["中ab "],"colorMap":[[[3,0],[4,0],[5,0],[2,0]]]}]}}`
	art, err = parseDur([]byte(js))
	if err != nil {
		t.Fatal(err)
	}
	g := art.docs[0]
	if r := wideFRow(g, 0); r != "中|ab" {
		t.Errorf("read %q", r)
	}
	if fa, _ := uniColoursOf(g.get(2, 0)); fa != durFg(4, false) {
		t.Errorf("a has colour %d: colours go with the characters", fa)
	}
	if f0, _ := uniColoursOf(g.get(1, 0)); f0 != durFg(3, false) {
		t.Errorf("the right half has colour %d", f0)
	}
}

func TestWideCharacterTable(t *testing.T) {
	a := testApp(t, "uni")
	for _, name := range []string{"CJK symbols, kana (wide)", "Fullwidth forms (wide)", "CJK ideographs (wide)", "Hangul syllables (wide)"} {
		found := false
		for _, b := range uniBlocks {
			if b.name == name {
				found = true
				shown := 0
				for c := b.base; c < b.base+256; c++ {
					if a.uniHas(c) {
						shown++
					}
				}
				if shown < 64 {
					t.Errorf("%s: %d characters shown", name, shown)
				}
			}
		}
		if !found {
			t.Errorf("no block %q", name)
		}
	}
	// The brush from the table types a wide character.
	a.doc = wideFDoc(t, 6, "")
	a.cx, a.cy = 0, 0
	a.brush, _ = uniCodeOf('ア')
	a.typeByte(a.brush)
	if r := wideFRow(a.doc, 0); r != "ア|    " || a.cx != 2 {
		t.Errorf("typed %q", r)
	}
}
