package main

// Files written by Moebius's own save code (tools/moebius/gen.js runs its
// app/libtextmode encoders) must open as exactly the page Moebius reads back
// from them: testdata/moebius/<file>.json is that page, from Moebius's decoder.

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

type moebiusRef struct {
	Columns, Rows     int
	Cells             string // base64: a character and a BIOS attribute byte per cell
	Ice, Ninepx, Utf8 bool
	Font              string
	Title, Author     string
	Group, Comments   string
	Palette           []int // XBin: 16 x RGB, 0-63
	FontHeight        int
	FontBytes         string // XBin: base64
	Nosauce           bool   // saved without a SAUCE record (or exported as UTF-8)
}

func readMoebiusRef(t *testing.T, name string) moebiusRef {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata/moebius", name+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var r moebiusRef
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatal(name, err)
	}
	return r
}

func moebiusFiles(t *testing.T) []string {
	b, err := os.ReadFile("testdata/moebius/INDEX")
	if err != nil {
		t.Fatal(err)
	}
	return strings.Fields(string(b))
}

// biosToAnsi turns a BIOS colour (1 blue, 4 red) into CharEdit's ANSI order (1 red, 4 blue).
func biosToAnsi(c int) int { return [8]int{0, 4, 2, 6, 1, 5, 3, 7}[c&7] | c&8 }

// moebiusDiffs compares the page with Moebius's; it returns the differences, a few named.
func moebiusDiffs(d *Doc, r moebiusRef) (int, []string) {
	cells, _ := base64.StdEncoding.DecodeString(r.Cells)
	var named []string
	bad := 0
	note := func(s string) {
		bad++
		if len(named) < 6 {
			named = append(named, s)
		}
	}
	// Without a record the file cannot say its height, and Moebius makes such a
	// page 25 rows at least: rows it has past ours must be empty. And there
	// CharEdit takes 0x00, 0x07, 0x08 and 0x09 for what a terminal does with them
	// (nothing, a beep, a step back, a tab: plain text and NFO files use tabs),
	// so rows that hold those are not compared.
	skip := map[int]bool{}
	rows := r.Rows
	if r.Nosauce {
		rows = d.rows
		for i := 0; i < len(cells)/2; i++ {
			if c := cells[2*i]; !r.Utf8 && (c == 0 || c == 7 || c == 8 || c == 9) {
				skip[i/r.Columns] = true
			}
			if i/r.Columns >= d.rows && (cells[2*i] != 32 || cells[2*i+1]&0xF0 != 0) {
				note("Moebius has more rows, not empty")
				break
			}
		}
		if len(skip) > 0 { // the rows after one of those are where Moebius's are: do not compare them either
			first := r.Rows
			for y := range skip {
				first = min(first, y)
			}
			for y := first; y < r.Rows; y++ {
				skip[y] = true
			}
		}
	}
	if d.cols != r.Columns || d.rows != rows || d.rows > r.Rows {
		note("size " + itoa(d.cols) + "x" + itoa(d.rows) + ", Moebius " + itoa(r.Columns) + "x" + itoa(r.Rows))
	}
	for i := 0; i < min(len(cells)/2, rows*r.Columns); i++ {
		x, y := i%r.Columns, i/r.Columns
		if skip[y] {
			continue
		}
		code, attr := int(cells[2*i]), int(cells[2*i+1])
		fg, bg := biosToAnsi(attr&15), biosToAnsi(attr>>4)
		v := d.get(x, y)
		if x >= d.cols || y >= d.rows {
			v = blank
		}
		gc, gf, gb := cellCh(v), cellFg(v), cellBg(v)
		if r.Utf8 && (code == 32 || code == 255) && gc == code { // a blank's foreground is not written
			gf = fg
		}
		if gc != code || gf != fg || gb != bg || v&(italic|underline) != 0 {
			note("cell " + itoa(x) + "," + itoa(y) + ": " + hex2(gc) + " " + itoa(gf) + "/" + itoa(gb) +
				", Moebius " + hex2(code) + " " + itoa(fg) + "/" + itoa(bg))
		}
	}
	return bad, named
}

func itoa(n int) string { return strconv.Itoa(n) }
func hex2(n int) string { return fmt.Sprintf("%02X", n) }

func TestMoebiusFilesOpen(t *testing.T) {
	for _, name := range moebiusFiles(t) {
		t.Run(name, func(t *testing.T) {
			b, err := os.ReadFile(filepath.Join("testdata/moebius", name))
			if err != nil {
				t.Fatal(err)
			}
			r := readMoebiusRef(t, name)
			a := testApp(t, "pc")
			a.loadBytes(b, name, "", "auto")
			if bad, named := moebiusDiffs(a.doc, r); bad > 0 {
				t.Errorf("%d differences: %s", bad, strings.Join(named, "; "))
			}
			checkMoebiusMeta(t, a, r, name)
		})
	}
}

func checkMoebiusMeta(t *testing.T, a *App, r moebiusRef, name string) {
	t.Helper()
	if !r.Utf8 && a.ice != r.Ice {
		t.Errorf("iCE %v, Moebius %v", a.ice, r.Ice)
	}
	if r.Utf8 {
		return
	}
	if a.ninePx() != r.Ninepx {
		t.Errorf("9-pixel cells %v, Moebius %v", a.ninePx(), r.Ninepx)
	}
	if r.FontBytes == "" && fontBySauce(r.Font, a.mode) != nil && !strings.EqualFold(a.font().sauce, r.Font) &&
		!strings.EqualFold(fontBySauce(r.Font, a.mode).sauce, a.font().sauce) {
		t.Errorf("font %q, Moebius %q", a.font().sauce, r.Font)
	}
	if strings.HasPrefix(r.Font, "Amiga") && !a.amiga() {
		t.Errorf("an Amiga font (%s) but a %s page", r.Font, a.mode)
	}
	if a.info.title != r.Title || a.info.author != r.Author || a.info.group != r.Group {
		t.Errorf("SAUCE %q %q %q, Moebius %q %q %q", a.info.title, a.info.author, a.info.group, r.Title, r.Author, r.Group)
	}
	if got := strings.Join(a.info.notes, "\n"); got != r.Comments {
		t.Errorf("comments %q, Moebius %q", got, r.Comments)
	}
	if len(r.Palette) == 48 {
		for i := 0; i < 16; i++ {
			c := r.Palette[biosToAnsiIndex(i)*3:]
			s := func(v int) uint32 { return uint32(v<<2 | v>>4) }
			want := 0xFF000000 | s(c[0])<<16 | s(c[1])<<8 | s(c[2])
			if a.pal[i] != want {
				t.Errorf("palette %d: %08x, Moebius %08x", i, a.pal[i], want)
				break
			}
		}
	}
	if r.FontBytes != "" {
		want, _ := base64.StdEncoding.DecodeString(r.FontBytes)
		f := a.font()
		if got := f.native(); !bytes.Equal(got, want) {
			t.Errorf("font: %d rows (%d bytes), Moebius %d rows", f.rows(), len(got), r.FontHeight)
		}
	}
}

// biosToAnsiIndex: the BIOS palette entry that is ANSI colour i.
func biosToAnsiIndex(i int) int { return biosToAnsi(i) }

// TestMoebiusSaveBack saves every corpus file again in its own format into
// $CHARCOAL_MOEBIUS_SAVE, for tools/moebius/check.js to read with Moebius's decoder.
func TestMoebiusSaveBack(t *testing.T) {
	dir := os.Getenv("CHARCOAL_MOEBIUS_SAVE")
	if dir == "" {
		t.Skip("CHARCOAL_MOEBIUS_SAVE not set")
	}
	for _, name := range moebiusFiles(t) {
		b, err := os.ReadFile(filepath.Join("testdata/moebius", name))
		if err != nil {
			t.Fatal(err)
		}
		a := testApp(t, "pc")
		a.loadBytes(b, name, "", "auto")
		out, err := a.encode(a.saveFmt)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if err := os.WriteFile(filepath.Join(dir, name), out, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// Saving a Moebius file again keeps its page, its SAUCE text, letter spacing,
// aspect ratio and font name (◙ ♪ ← become spaces; ○ ◘ • and character 0 stay
// only when the file had them so).
func TestMoebiusRoundTrip(t *testing.T) {
	for _, name := range moebiusFiles(t) {
		b, err := os.ReadFile(filepath.Join("testdata/moebius", name))
		if err != nil {
			t.Fatal(err)
		}
		a := testApp(t, "pc")
		a.loadBytes(b, name, "", "auto")
		first := a.doc.clone()
		inSauce, _ := readSauce(b)
		out, err := a.encode(a.saveFmt)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		a2 := testApp(t, "pc")
		as := "auto"
		if inSauce == nil {
			as = a.mode // no record to say: every byte 0-255 on one page reads as Amiga (its CSI 0x9B)
		}
		a2.loadBytes(out, name, "", as)
		if first.cols != a2.doc.cols || first.rows != a2.doc.rows {
			t.Errorf("%s: %dx%d became %dx%d", name, first.cols, first.rows, a2.doc.cols, a2.doc.rows)
			continue
		}
		for i, v := range first.cells[:first.cols*first.rows] {
			if c := cellCh(v); a.saveFmt == "ansi" && (c == 10 || c == 13 || c == 27 || !a.rawCtl && (c == 0 || c == 7 || c == 8 || c == 9)) {
				v = v&^255 | 32
			}
			if w := a2.doc.cells[i]; w != v && !(cellCh(v) == 32 && cellCh(w) == 32 && cellBg(v) == cellBg(w)) {
				t.Errorf("%s: cell %d,%d %08x became %08x (%s %s, then %s)", name, i%first.cols, i/first.cols, v, w, a.saveFmt, a.mode, a2.mode)
				break
			}
		}
		outSauce, _ := readSauce(out)
		if inSauce == nil || outSauce == nil || inSauce.dataType == 6 {
			continue
		}
		if outSauce.flags != inSauce.flags || outSauce.font != inSauce.font || outSauce.title != inSauce.title ||
			strings.Join(outSauce.notes, "|") != strings.Join(inSauce.notes, "|") {
			t.Errorf("%s: SAUCE flags %02x font %q title %q notes %q, was %02x %q %q %q", name, outSauce.flags, outSauce.font,
				outSauce.title, outSauce.notes, inSauce.flags, inSauce.font, inSauce.title, inSauce.notes)
		}
	}
}

// 9-pixel letter spacing: the page is 9 pixels a cell wide, the line characters
// 0xC0-0xDF fill the ninth column, the others leave it empty; the key toggles it.
func TestNinePixelCells(t *testing.T) {
	a := testApp(t, "pc")
	b, _ := os.ReadFile("testdata/moebius/ninepx.ans")
	a.loadBytes(b, "ninepx.ans", "", "auto")
	if a.cellW() != 9 {
		t.Fatalf("cell width %d", a.cellW())
	}
	a.doc.set(0, 0, makeCell(0xC4, 15, 0, 0)) // ─
	a.doc.set(1, 0, makeCell(0xDB, 15, 0, 0)) // █
	a.doc.set(2, 0, makeCell(0xB3, 15, 0, 0)) // │ (0xB3: not in the range)
	fb := &FB{}
	fb.resize(27, a.cellH())
	fb.clip = Rect{0, 0, 27, a.cellH()}
	for x := 0; x < 3; x++ {
		a.drawCell(fb, x*9, 0, 1, a.doc.get(x, 0))
	}
	ink := a.pal[15]
	at := func(x, y int) bool { return fb.px[y*fb.w+x] == ink }
	line := -1 // the row of ─'s line
	for y := 0; y < a.cellH(); y++ {
		if at(0, y) {
			line = y
		}
	}
	if line < 0 || !at(8, line) || !at(17, 3) || at(26, 3) || !at(21, 3) {
		t.Errorf("ninth columns: ─ row %d, █ %v, │ %v", line, at(17, 3), at(26, 3))
	}
	a.toggleNinePx()
	if a.cellW() != 8 || a.sauceFlags&6 != 2 {
		t.Errorf("toggled: width %d flags %x", a.cellW(), a.sauceFlags)
	}
}
