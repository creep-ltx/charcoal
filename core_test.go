package main

import (
	"strings"
	"testing"
)

func row(d *Doc, y int) string {
	var sb strings.Builder
	for x := 0; x < d.cols; x++ {
		sb.WriteByte(byte(cellCh(d.get(x, y))))
	}
	return strings.TrimRight(sb.String(), " ")
}

func parse(s string, amiga bool) *Doc {
	d, _, _ := parseArt([]byte(s), ParseOpts{amiga: amiga})
	return d
}

func TestCore(t *testing.T) {
	// Amiga 8-bit CSI, form feed, LF-only lines, bold.
	d := parse("junk\x0c\x9b1;33mHi\x9b0m\nthere\x9b3;4mX", true)
	if row(d, 0) != "Hi" || cellFg(d.get(0, 0)) != 11 || row(d, 1) != "thereX" {
		t.Fatalf("amiga: %q %q", row(d, 0), row(d, 1))
	}
	if v := d.get(5, 1); v&italic == 0 || v&underline == 0 {
		t.Fatal("italic/underline")
	}
	// On a PC page 0x9B is a glyph.
	if cellCh(parse("a\x9bb", false).get(1, 0)) != 0x9b {
		t.Fatal("pc 0x9b")
	}
	// Positioning, CRLF, wrap at once at 80 columns.
	d = parse("\x1b[2J\x1b[3;5HX\r\n"+strings.Repeat("y", 80)+"z", false)
	if cellCh(d.get(4, 2)) != 'X' || row(d, 4) != "z" {
		t.Fatal("positioning")
	}
	// Round trip.
	d = newDoc(80, 3)
	d.set(0, 0, makeCell(65, 12, 1, 0))
	d.set(79, 0, makeCell(66, 3, 9, 0))
	for x := 0; x < 80; x++ {
		d.set(x, 1, makeCell(0xB0, 7, 0, 0))
	}
	d.set(2, 2, makeCell(67, 7, 0, italic))
	for _, amiga := range []bool{true, false} {
		back, _, _ := parseArt(writeAnsi(d, amiga, true, false), ParseOpts{amiga: amiga})
		for y := 0; y < 3; y++ {
			for x := 0; x < 80; x++ {
				if back.get(x, y) != d.get(x, y) {
					t.Fatalf("round trip %v %d,%d", amiga, x, y)
				}
			}
		}
	}
	// SAUCE.
	art := writeAnsi(d, false, true, false)
	file := append(append([]byte{}, art...), makeSauce(SauceInfo{title: "Test", author: "me", cols: 132, rows: 3, font: "Amiga Topaz 1+", fileType: 1}, len(art))...)
	s, end := readSauce(file)
	if end != len(art) || s.title != "Test" || s.font != "Amiga Topaz 1+" {
		t.Fatalf("sauce %d %+v", end, s)
	}
	if d2, _, _ := parseArt(file, ParseOpts{amiga: true}); d2.cols != 132 {
		t.Fatal("sauce width")
	}
	// Insert / delete lines.
	d = parse("a\nb\nc\x1b[2;1H\x1b[L", true)
	if row(d, 0) != "a" || row(d, 1) != "" || row(d, 2) != "b" || row(d, 3) != "c" {
		t.Fatal("insert line")
	}
	d = parse("a\nb\nc\x1b[1;1H\x1b[M", true)
	if row(d, 0) != "b" || row(d, 1) != "c" {
		t.Fatal("delete line")
	}
	// SyncTERM font switch.
	if _, _, f := parseArt([]byte("\x1b[0;41 D"), ParseOpts{amiga: true}); f != 41 {
		t.Fatal("font request", f)
	}
	// Guess.
	if !guessAmiga([]byte("x\n\xaf\xaf\xb8\ny\nz\nw\nv\n")) || guessAmiga([]byte("\xdb\xdb\xb0\xb1\r\n\xdc\xdf\r\n")) {
		t.Fatal("guess")
	}
	// UTF-8 art.
	if b := fromUtf8([]byte("█▀ x"), false); string(b) != "\xdb\xdf x" {
		t.Fatalf("utf8 %q", b)
	}
	// .bin
	d = newDoc(80, 1)
	d.set(0, 0, makeCell(0xDB, 4, 1, 0))
	b2, _, _ := parseArt(writeBin(d), ParseOpts{binary: true})
	if b2.get(0, 0) != d.get(0, 0) {
		t.Fatal("bin")
	}
}
