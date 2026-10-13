package main

import (
	"strings"
	"testing"
	"time"
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
	// Swedish text in CP437 is PC, though plain otherwise.
	if guessAmiga([]byte("vi kanske inte \x84r b\x84st\nmen 2'a\n")) {
		t.Fatal("CP437 letters")
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

// Hostile numbers (audit #1): no crash, no hang, no huge page, nothing lost off the right edge.
func TestHostileCSI(t *testing.T) {
	for _, s := range []string{
		"\x1b[99999999Bx",
		"\x1b[1000000000000000000Xx",
		"\x1b[99999999999999999999999999999999Lx",
		"\x1b[99999999;99999999Hx",
		strings.Repeat("\x1b[9999B", 200) + "x",
		strings.Repeat("\x1b[9999L", 500),
	} {
		d := parse(s, false)
		if d.rows > maxRowsFor(d.cols) || len(d.cells) < d.rows*d.cols {
			t.Fatalf("%q: page %dx%d", s, d.cols, d.rows)
		}
	}
	// A huge CSI C stops at the last column instead of wrapping around.
	if d := parse("\x1b[9223372036854775807Cx", false); cellCh(d.get(79, 0)) != 'x' {
		t.Fatal("huge C")
	}
	// CSI X erases to the line end, not past it.
	if d := parse("abc\r\ndef\x1b[1;2H\x1b[99999X", false); row(d, 0) != "a" || row(d, 1) != "def" {
		t.Fatalf("X: %q %q", row(d, 0), row(d, 1))
	}
	if csiParam("123") != 123 || csiParam("99999999999999999999") != maxParam || csiParam("1:2") != noParam || csiParam("") != noParam {
		t.Fatal("csiParam")
	}
	// Writes past the largest page are ignored; the page stops there.
	d := newDoc(80, 1)
	d.set(0, maxRowsFor(80)+5, makeCell('x', 7, 0, 0))
	d.ensureRows(1 << 40)
	if d.rows != maxRowsFor(80) {
		t.Fatalf("rows %d", d.rows)
	}
	// Rows cut off and grown again come back blank.
	d = newDoc(10, 3)
	d.set(0, 2, makeCell('x', 7, 0, 0))
	d.rows = 1
	d.ensureRows(3)
	if d.get(0, 2) != blank {
		t.Fatal("stale cell")
	}
}

// The largest page, then cleared or shifted again and again: still fast.
func TestLargestPageRepeated(t *testing.T) {
	for _, s := range []string{
		strings.Repeat("\x1b[9999B", 10) + "x" + strings.Repeat("\x1b[2J", 300),
		strings.Repeat("\x1b[9999B", 10) + "x" + strings.Repeat("\x1b[1;1H\x1b[9999L", 300),
	} {
		start := time.Now()
		parse(s, false)
		if el := time.Since(start); el > time.Second {
			t.Fatalf("took %v", el)
		}
	}
}

// Insert / delete line move one screen at most (audit #25): fast on a long page.
func TestShiftRowsBounded(t *testing.T) {
	d := parse(strings.Repeat("x\n", 30000)+"\x1b[1;1H"+strings.Repeat("\x1b[L\x1b[M", 20000), false)
	if d.rows < 30000 || row(d, 29999) != "x" {
		t.Fatalf("rows %d", d.rows)
	}
	// On a short page it grows; on a long one the screen's last row falls off.
	d = parse("a\nb\nc\x1b[1;1H\x1b[2L", false)
	if row(d, 0) != "" || row(d, 2) != "a" || row(d, 4) != "c" {
		t.Fatal("insert 2")
	}
	d = parse(strings.Repeat("y\n", 40)+"\x1b[1;1H\x1b[L", false)
	if row(d, 0) != "" || row(d, 24) != "y" || row(d, 25) != "y" || d.rows != 40 {
		t.Fatalf("screen insert: %d rows", d.rows)
	}
}

// SAUCE (audit #23, the notes, CP437 text).
func TestSauceDetails(t *testing.T) {
	art := []byte("hello")
	rec := makeSauce(SauceInfo{title: "Grüß ░", notes: []string{"first", "second ▓"}, cols: 80, rows: 1}, len(art))
	file := append(append([]byte{}, art...), rec...)
	s, end := readSauce(file)
	if s == nil || end != len(art) || s.title != "Grüß ░" || len(s.notes) != 2 || s.notes[1] != "second ▓" {
		t.Fatalf("sauce %+v end %d", s, end)
	}
	if sauceStart(file) != len(art)+1 || dataEnd(file) != len(art) {
		t.Fatal("sauceStart / dataEnd")
	}
	// A .bin ending in 0x00 / 0x1A cells keeps them, and a FileSize past the data is not trusted.
	bin := []byte{'A', 0x07, 0x1A, 0x1A, 0, 0}
	file = append(append([]byte{}, bin...), makeSauce(SauceInfo{cols: 3, binaryText: true}, 999999)...)
	d, _, _ := parseArt(file, ParseOpts{binary: true})
	if d.cols != 4 || d.rows != 1 || cellCh(d.get(1, 0)) != 0x1A || cellCh(d.get(2, 0)) != 0 {
		t.Fatalf("bin tail: %dx%d", d.cols, d.rows)
	}
}

// .bin edge cases (audit #14, #15, #16).
func TestBinEdges(t *testing.T) {
	// 160 wide, no SAUCE, 0x1A attributes: the width comes from the whole file.
	b := make([]byte, 160*3*2)
	for i := 0; i < len(b); i += 2 {
		b[i], b[i+1] = byte('A'+i/2%160%26), 0x1A
	}
	if d, _, _ := parseArt(b, ParseOpts{binary: true}); d.cols != 160 || d.rows != 3 {
		t.Fatalf("width %d rows %d", d.cols, d.rows)
	}
	// Two rows stay two rows (160 wide: what a .bin without SAUCE is taken to be).
	d := newDoc(160, 2)
	d.set(0, 1, makeCell('x', 7, 0, 0))
	if back, _, _ := parseArt(writeBin(d), ParseOpts{binary: true}); back.rows != 2 {
		t.Fatalf("rows %d", back.rows)
	}
	// An odd width gets a blank column and reopens with it.
	d = newDoc(81, 2)
	d.set(80, 1, makeCell('z', 7, 0, 0))
	art := writeBin(d)
	back, _, _ := parseArt(append(art, makeSauce(SauceInfo{cols: 81, binaryText: true}, len(art))...), ParseOpts{binary: true})
	if back.cols != 82 || back.rows != 2 || cellCh(back.get(80, 1)) != 'z' {
		t.Fatalf("odd width: %dx%d", back.cols, back.rows)
	}
	if binSaveError(newDoc(510, 1)) != nil || binSaveError(newDoc(512, 1)) == nil {
		t.Fatal("binSaveError")
	}
}
