package main

import "testing"

func TestAtari(t *testing.T) {
	// The font: ATASCII 'A' is the ROM's internal 33, the heart internal 64; the top half inverse.
	f := fontByID("atari")
	for r := 0; r < 8; r++ {
		if f.bits[0x41*16+r*2] != atariFontStd[33*8+r] || f.bits[0x00*16+r*2] != atariFontStd[64*8+r] ||
			f.bits[0xC1*16+r*2] != ^atariFontStd[33*8+r] {
			t.Fatal("font layout")
		}
	}
	// Clear, text, EOL, a control character shown through ESC, cursor right.
	d := parseAta([]byte{0x7D, 'H', 'I', 0x9B, 0x1B, 0x1C, 0x1F, 0x1F, 0x91, 0x9B}, 40)
	if cellCh(d.get(0, 0)) != 'H' || cellCh(d.get(0, 1)) != 0x1C || cellCh(d.get(3, 1)) != 0x91 {
		t.Fatalf("parse: %x %x %x", d.get(0, 0), d.get(0, 1), d.get(3, 1))
	}
	if d.bg != atariBg || d.ink != atariInk {
		t.Fatal("colours")
	}
	// Every code but EOL survives writing and reading.
	src := newAtariDoc()
	src.ensureRows(8)
	for c := 0; c < 256; c++ {
		if c != 0x9B {
			src.set(c%40, c/40, makeCell(c, 0, 0, 0))
		}
	}
	back := parseAta(writeAta(src, true), 40)
	for y := 0; y < 8; y++ {
		for x := 0; x < 40; x++ {
			if cellCh(back.get(x, y)) != cellCh(src.get(x, y)) {
				t.Fatalf("round trip %d,%d: %x != %x", x, y, back.get(x, y), src.get(x, y))
			}
		}
	}
	if !looksAtascii([]byte("HELLO\x9bWORLD\x9b\x9b")) || looksAtascii([]byte("\x9b1;33mHi\x9b0m\nthere\n")) {
		t.Fatal("telling ATASCII from Amiga ANSI")
	}
	anim := []byte{0x7D}
	for i := 0; i < 20; i++ {
		anim = append(anim, 0x1E, 0xA0, 0x1D)
	}
	if !looksAtascii(anim) || looksAtascii([]byte("\x1b[1;31mred\x1b[0m\r\n")) {
		t.Fatal("ATASCII animation")
	}
	if atariFromRune('a') != 'a' || atariFromRune('♥') != 0 || atariFromRune('`') != -1 {
		t.Fatal("typing")
	}
	// Mirror images in the Atari font, by pixels.
	if f.flipped(0x11, false) != 0x05 || f.flipped(0x11, true) != 0x1A || f.flipped(0x06, false) != 0x07 {
		t.Fatalf("mirror: %x %x %x", f.flipped(0x11, false), f.flipped(0x11, true), f.flipped(0x06, false))
	}
}

func TestAtariWidth(t *testing.T) {
	// A 40-wide line ended by EOL means 40.
	line := make([]byte, 40)
	for i := range line {
		line[i] = 'X'
	}
	if w := atariWidth(append(line, 0x9B)); w != 40 {
		t.Fatalf("40-wide line: %d", w)
	}
	// A stream with no EOL drawn on 38 columns: a block that stacks every 38 bytes.
	var b []byte
	for y := 0; y < 10; y++ {
		row := make([]byte, 38)
		for x := range row {
			row[x] = ' '
		}
		row[5], row[6], row[20] = 0xA0, 0xA0, 0x19
		b = append(b, row...)
	}
	if w := atariWidth(b); w != 38 {
		t.Fatalf("38-wide stream: %d", w)
	}
}
