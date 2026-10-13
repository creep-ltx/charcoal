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
