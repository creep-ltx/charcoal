package main

import "testing"

func TestSeq(t *testing.T) {
	// Clear, white "HI", reverse red block, RETURN, cyan "A" in column 0.
	d, lower := parseSeq([]byte{0x93, 0x05, 'H', 'I', 0x12, 0x1C, 0x20, 0x0D, 0x9F, 'A'}, 40)
	if lower != -1 || cellCh(d.get(0, 0)) != 8 || cellCh(d.get(1, 0)) != 9 || cellFg(d.get(0, 0)) != 1 {
		t.Fatalf("text: %x %x", d.get(0, 0), d.get(1, 0))
	}
	if cellCh(d.get(2, 0)) != 0xA0 || cellFg(d.get(2, 0)) != 2 {
		t.Fatalf("reverse: %x", d.get(2, 0))
	}
	if cellCh(d.get(0, 1)) != 1 || cellFg(d.get(0, 1)) != 3 {
		t.Fatalf("after RETURN (reverse off): %x", d.get(0, 1))
	}
	// Round trip through the writer, every screen code once.
	src := newDoc(40, 8)
	for c := 0; c < 256; c++ {
		src.set(c%40, c/40, makeCell(c, c%16, 0, 0))
	}
	back, _ := parseSeq(writeSeq(src, true, true), 40)
	for y := 0; y < 8; y++ {
		for x := 0; x < 40; x++ {
			v, w := src.get(x, y), back.get(x, y)
			if cellCh(v) == 0x20 { // a space carries no colour
				v, w = v&255, w&255
			}
			if v != w {
				t.Fatalf("round trip %d,%d: %x != %x", x, y, w, v)
			}
		}
	}
	// Typing.
	if petFromRune('a', false) != 1 || petFromRune('A', true) != 0x41 || petFromRune('£', false) != 0x1C {
		t.Fatal("typing")
	}
}

func TestMirror(t *testing.T) {
	vga := fontByID("vga")
	for _, p := range [][3]int{{0xC9, 0xBB, 0}, {0xDA, 0xBF, 0}, {0xDD, 0xDE, 0}, {'(', ')', 0}, {'/', '\\', 0},
		{0xDC, 0xDF, 1}, {0xDA, 0xC0, 1}, {0xC9, 0xC8, 1}} {
		if got := vga.flipped(p[0], p[2] == 1); got != p[1] {
			t.Errorf("flip %02X vertical=%d: got %02X, want %02X", p[0], p[2], got, p[1])
		}
	}
	for _, c := range []int{'A', 'H', 'O', 0xDB, ' '} {
		if got := vga.flipped(c, false); got != c {
			t.Errorf("%02X should stay, got %02X", c, got)
		}
	}
}

func TestMirrorC64(t *testing.T) {
	if !loadC64Fonts() {
		t.Skip("no C64 character ROM here")
	}
	f := fontByID("c64upper")
	for _, p := range [][3]int{{0x70, 0x6E, 0}, {0x6D, 0x7D, 0}, {0x55, 0x49, 0}, {0x4D, 0x4E, 0}, {0x6B, 0x73, 0},
		{0x70, 0x6D, 1}, {0x72, 0x71, 1}, {0x55, 0x4A, 1}} {
		if got := f.flipped(p[0], p[2] == 1); got != p[1] {
			t.Errorf("C64 flip %02X vertical=%d: got %02X, want %02X", p[0], p[2], got, p[1])
		}
	}
}
