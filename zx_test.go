package main

import "testing"

func TestZX(t *testing.T) {
	d := newZXDoc()
	d.set(0, 0, zxCell('H', 2, 6, true, false))
	d.set(31, 23, zxCell(133, 1, 7, false, true)) // a block graphic, flashing
	d.set(5, 9, zxCell(0x60, 0, 7, false, false)) // £
	scr := writeSCR(d)
	if len(scr) != 6912 {
		t.Fatal(len(scr))
	}
	back, approx := parseSCR(scr)
	if approx != 0 {
		t.Fatalf("%d cells not matched", approx)
	}
	for _, p := range [][2]int{{0, 0}, {31, 23}, {5, 9}, {10, 10}} {
		if a, b := d.get(p[0], p[1]), back.get(p[0], p[1]); a != b {
			// a space can come back as a full block with the colours swapped; both look the same
			t.Errorf("cell %v: %08x became %08x", p, a, b)
		}
	}
	if zxFromRune('£') != 0x60 || zxRune(0x60) != '£' || zxRune(143) != '█' {
		t.Fatal("characters")
	}
}
