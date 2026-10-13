package main

import "testing"

func TestTeletext(t *testing.T) {
	d := newTTDoc()
	row := []int{0x01, 'H', 'i', 0x12, 0x35, 0x1E, 0x13, 0x7F, 0x0D, 'B'}
	for x, c := range row {
		d.set(x, 2, makeCell(c, 0, 0, 0))
	}
	// edit.tf, .tti and .ep1 give the same codes back.
	e, _ := parseEditTF([]byte("https://edit.tf/#" + editTFHash(d, false)))
	ti, ep := parseTTI(writeTTI(d)), parseEP1(writeEP1(d))
	for i := range d.cells[:1000] {
		for name, o := range map[string]*Doc{"edit.tf": e, "tti": ti, "ep1": ep} {
			if cellCh(o.cells[i]) != cellCh(d.cells[i]) {
				t.Fatalf("%s cell %d: %x != %x", name, i, cellCh(o.cells[i]), cellCh(d.cells[i]))
			}
		}
	}
	r := ttDecode(d, 2, false)
	if r[1].fg != 1 || r[1].mosaic || r[1].code != 'H' { // red text, set-after
		t.Fatalf("text colour: %+v", r[1])
	}
	if r[0].fg != 7 { // the colour code's own cell is still white (set-after)
		t.Fatal("set-after")
	}
	if !r[4].mosaic || r[4].fg != 2 { // green graphics
		t.Fatalf("graphics: %+v", r[4])
	}
	if !r[6].mosaic || r[6].code != 0x35 { // hold: the yellow-graphics code shows the held sixels
		t.Fatalf("hold: %+v", r[6])
	}
	if r[9].dh != 1 || r[9].code != 'B' { // double height after 0x0D
		t.Fatalf("double: %+v", r[9])
	}
	under := ttDecode(d, 3, false)
	if under[9].dh != 2 || under[9].code != 'B' {
		t.Fatalf("bottom half: %+v", under[9])
	}
	// The A has its point, the rounding fills the diagonal.
	if !ttGlyph('A', 4, 0) || ttGlyph('A', 0, 0) {
		t.Fatal("glyph A")
	}
}

// Mode 7 dumps of any length (audit #2), and 1024-byte ones (all of screen memory).
func TestMode7Lengths(t *testing.T) {
	for _, n := range []int{0, 1, 999, 1000, 1024, 5000} {
		b := make([]byte, n)
		for i := range b {
			b[i] = 'A'
		}
		d := parseMode7(b)
		if d.cols != 40 || d.rows != 25 {
			t.Fatalf("%d bytes: %dx%d", n, d.cols, d.rows)
		}
		if n > 0 && cellCh(d.get(0, 0)) != 'A' {
			t.Fatalf("%d bytes: first cell %x", n, d.get(0, 0))
		}
		if n < 1000 && cellCh(d.get(39, 24)) != 0x20 {
			t.Fatalf("%d bytes: not padded with spaces", n)
		}
	}
	dump := make([]byte, 1024)
	for i := range dump {
		dump[i] = 0x20
		if i%40 == 0 && i < 1000 {
			dump[i] = 0x81 // red text, top bit set as in BBC screen memory
		}
	}
	if !isMode7(dump, "page.bin") || !isMode7(dump[:1000], "PAGE.M7") || isMode7(dump[:1010], "page.bin") ||
		isMode7(dump, "page.txt") {
		t.Fatal("isMode7 lengths and names")
	}
}

// edit.tf keeps its whole meta digit: the character set as well as black foreground.
func TestEditTFMeta(t *testing.T) {
	d := newTTDoc()
	d.set(0, 0, makeCell('Q', 0, 0, 0))
	for _, meta := range []int{0, 2, 7, 8, 9, 15} {
		url := writeEditTFURLMeta(d, meta)
		back, got := parseEditTFMeta(url)
		if back == nil || got != meta || cellCh(back.get(0, 0)) != 'Q' {
			t.Fatalf("meta %x came back as %x", meta, got)
		}
		if _, black := parseEditTF(url); black != (meta&8 != 0) {
			t.Fatalf("meta %x: black %v", meta, black)
		}
	}
}

// TTI: ESC is a code+0x40 escape only before 0x40-0x5F; any other ESC is the code 0x1B itself.
func TestTTIEsc(t *testing.T) {
	d := parseTTI([]byte("OL,1,\x1bA\x1b!B\x1b\r\n"))
	for x, c := range []int{0x01, 0x1B, '!', 'B', 0x1B} {
		if got := cellCh(d.get(x, 1)); got != c {
			t.Fatalf("cell %d: %x, want %x", x, got, c)
		}
	}
}
