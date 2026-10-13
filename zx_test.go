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

// Any length of file opens as a screen without crashing (audit #2): short files are padded.
func TestSCRAnyLength(t *testing.T) {
	full := writeSCR(newZXDoc())
	for _, n := range []int{0, 1, 100, 6143, 6144, 6911, 6912, 6913, 20000} {
		b := make([]byte, n)
		copy(b, full)
		d, _ := parseSCR(b)
		if d.cols != 32 || d.rows != 24 {
			t.Fatalf("%d bytes: %dx%d", n, d.cols, d.rows)
		}
	}
	// Extra bytes are ignored: the same page as the 6912 alone.
	d := newZXDoc()
	d.set(3, 4, zxCell('Z', 1, 6, false, false))
	scr := writeSCR(d)
	a, _ := parseSCR(scr)
	b, _ := parseSCR(append(append([]byte{}, scr...), 1, 2, 3))
	for i := range a.cells[:768] {
		if a.cells[i] != b.cells[i] {
			t.Fatalf("cell %d differs with extra bytes", i)
		}
	}
	if cellCh(a.get(3, 4)) != 'Z' || a.border != 7 {
		t.Fatalf("cell %x, border %d", a.get(3, 4), a.border)
	}
}
