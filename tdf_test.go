package main

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// tdfFFont is a font for the tests: glyphs as rows of CP437 bytes; colour fonts give
// each cell's attribute in attrs (one byte per cell, the same shape as the rows).
type tdfFFont struct {
	name    string
	kind    int
	spacing int
	glyphs  map[rune][]string
	attrs   map[rune][]string
	widths  map[rune]int // the width byte when not the longest row
	heights map[rune]int // the height byte when not the rows without '&'
}

// tdfFFile writes fonts as a .tdf file, the way TheDraw's TDFONTS.TDF has them.
func tdfFFile(fonts ...tdfFFont) []byte {
	out := []byte{19}
	out = append(out, "TheDraw FONTS file"...)
	out = append(out, 0x1A)
	for _, f := range fonts {
		out = binary.LittleEndian.AppendUint32(out, 0xFF00AA55)
		name := make([]byte, 12)
		copy(name, f.name)
		out = append(out, byte(min(12, len(f.name))))
		out = append(out, name...)
		out = append(out, 0, 0, 0, 0, byte(f.kind), byte(f.spacing))
		var blk []byte
		tab := make([]uint16, tdfChars)
		for i := range tab {
			c := rune(tdfFirst + i)
			rows, ok := f.glyphs[c]
			if !ok {
				tab[i] = 0xFFFF
				continue
			}
			tab[i] = uint16(len(blk))
			w, h := 0, 0
			for _, r := range rows {
				w = max(w, len(strings.TrimSuffix(r, "&")))
				if !strings.HasSuffix(r, "&") {
					h++
				}
			}
			if v, ok := f.widths[c]; ok {
				w = v
			}
			if v, ok := f.heights[c]; ok {
				h = v
			}
			blk = append(blk, byte(w), byte(h))
			for y, r := range rows {
				if y > 0 {
					blk = append(blk, 13)
				}
				for x := 0; x < len(r); x++ {
					blk = append(blk, r[x])
					if f.kind == tdfColour && r[x] != '&' {
						blk = append(blk, f.attrs[c][y][x])
					}
				}
			}
			blk = append(blk, 0)
		}
		out = binary.LittleEndian.AppendUint16(out, uint16(len(blk)))
		for _, v := range tab {
			out = binary.LittleEndian.AppendUint16(out, v)
		}
		out = append(out, blk...)
	}
	return append(out, 0)
}

// The test fonts: a block font with a solid space and a descender, an outline font
// as TheDraw's own "Outline" draws a box, a colour font with a space on black
// (left out) and one on red (drawn).
func tdfFBlock() tdfFFont {
	return tdfFFont{name: "TestBlock", kind: tdfBlock, spacing: 2 /* a gap of 1 */, glyphs: map[rune][]string{
		'A': {"\xdc\xdc\xdc", "\xdb \xdb", "\xdb\xff\xdb"},
		'B': {"\xdb\xdb", "\xdb\xdf"},
		'Q': {"\xdb\xdb\xdb", "\xdb \xdb", "\xdf\xdf\xdb", "  \xdf&"},
		'i': {"\xdb"},
	}}
}

func tdfFOutline() tdfFFont {
	return tdfFFont{name: "TestOutline", kind: tdfOutline, spacing: 0, glyphs: map[rune][]string{
		'O': {"EAAF", "C@OD", "KBBL"},
		'x': {"M N", "P Q"},
	}}
}

func tdfFColour() tdfFFont {
	return tdfFFont{name: "TestColour", kind: tdfColour, spacing: 2,
		glyphs: map[rune][]string{'C': {"\xdb ", "  \xff"}},
		attrs:  map[rune][]string{'C': {"\x0c\x00", "\x40\x07\x1e"}}}
}

func tdfFFonts(t *testing.T) []*tdfFont {
	t.Helper()
	fs, err := parseTDF(tdfFFile(tdfFBlock(), tdfFOutline(), tdfFColour()), "test.tdf")
	if err != nil || len(fs) != 3 {
		t.Fatalf("%v %d", err, len(fs))
	}
	return fs
}

// tdfFLib makes the test fonts the library and the block font the one in use.
func tdfFLib(t *testing.T, use int) []*tdfFont {
	fs := tdfFFonts(t)
	tdfLib.fonts, tdfLib.loaded, tdfLib.fits = fs, true, map[*tdfFont]bool{}
	tdfChoose(fs[use])
	tdfFClean(t)
	return fs
}

// tdfFClean puts back what the Logo tool's tests change outside the App: the library,
// the run, the focused field and the window's picture.
func tdfFClean(t *testing.T) {
	fb, focus := ui.fb, ui.focus
	t.Cleanup(func() {
		tdfLib.fonts, tdfLib.loaded, tdfLib.fits = nil, false, nil
		logo.tab, logo.gens = nil, nil
		ui.fb, ui.focus, ui.drag = fb, focus, ""
	})
}

func tdfFRows(p *tdfPic) []string {
	var out []string
	for y := 0; y < p.h; y++ {
		var sb strings.Builder
		for x := 0; x < p.w; x++ {
			c := p.at(x, y)
			switch c.kind {
			case tdfNone:
				sb.WriteByte('.')
			default:
				sb.WriteByte(c.ch)
			}
		}
		out = append(out, sb.String())
	}
	return out
}

func TestTDFParse(t *testing.T) {
	fs := tdfFFonts(t)
	for i, want := range []struct {
		name          string
		kind, spacing int
	}{{"TestBlock", tdfBlock, 2}, {"TestOutline", tdfOutline, 0}, {"TestColour", tdfColour, 2}} {
		f := fs[i]
		if f.name != want.name || f.kind != want.kind || f.spacing != want.spacing || f.index != i || f.file != "test.tdf" {
			t.Errorf("font %d: %+v", i, f)
		}
	}
	if !fs[0].has('A') || fs[0].has('C') || fs[0].has(' ') || fs[0].has('~'+1) {
		t.Error("has")
	}
	if fs[0].resolve('a') != 'A' || fs[0].resolve('I') != 'i' || fs[0].resolve('z') != -1 {
		t.Error("the other case")
	}
	g := fs[0].glyph('Q')
	if g.w != 3 || g.h != 3 || len(g.rows) != 4 || !g.desc[3] || g.body() != 3 {
		t.Errorf("descender: %+v", g)
	}
	if fs[0].lineHeight() != 3 {
		t.Errorf("line height %d", fs[0].lineHeight())
	}
	// Wrong files.
	for _, b := range [][]byte{nil, []byte("TheDraw"), []byte("\x13TheDraw FONTS fileX\x00")} {
		if _, err := parseTDF(b, ""); err == nil {
			t.Errorf("%q read", b)
		}
	}
	// Two fonts, the second cut short: the first is kept.
	b := tdfFFile(tdfFBlock(), tdfFOutline())
	if fs, err := parseTDF(b[:len(b)-30], ""); err != nil || len(fs) != 1 {
		t.Errorf("cut: %v %d", err, len(fs))
	}
	// An unknown type, and nothing after the header.
	bad := tdfFFile(tdfFFont{name: "X", kind: 3, glyphs: map[rune][]string{'A': {"x"}}})
	if _, err := parseTDF(bad, ""); err == nil {
		t.Error("type 3 read")
	}
	if _, err := parseTDF(tdfFFile(), ""); err == nil {
		t.Error("no fonts read")
	}
	// An offset past the block is no glyph.
	b = tdfFFile(tdfFBlock())
	binary.LittleEndian.PutUint16(b[20+4+1+12+4+4+2*('B'-tdfFirst):], 0x7000)
	if fs, err := parseTDF(b, ""); err != nil || fs[0].has('B') || fs[0].glyph('B') != nil {
		t.Error("offset past the block")
	}
	// The same font twice is one font in the library.
	if fs1, _ := parseTDF(tdfFFile(tdfFBlock()), "a"); fs1[0].sum != fs[0].sum {
		t.Error("sum")
	}
}

func TestTDFRender(t *testing.T) {
	fs := tdfFFonts(t)
	// Block: the box filled with spaces in the brush's colours, the solid space a
	// space, a blank left as the fill, the descender only where it draws.
	got := tdfFRows(fs[0].render('Q', 0))
	want := []string{"\xdb\xdb\xdb", "\xdb \xdb", "\xdf\xdf\xdb", "..\xdf"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("Q: %q", got)
	}
	if p := fs[0].render('A', 0); p.at(1, 2) != (tdfCell{kind: tdfInk, ch: ' '}) || p.at(1, 1) != (tdfCell{kind: tdfInk, ch: ' '}) {
		t.Error("A's blanks")
	}
	// Outline, every style: A-N from TheDraw's table, @ O P blanks, others as they are.
	for s := range tdfOutlines {
		p := fs[1].render('O', s)
		o := tdfOutlines[s]
		want := []string{string([]byte{o[4], o[0], o[0], o[5]}), string([]byte{o[2], ' ', ' ', o[3]}), string([]byte{o[10], o[1], o[1], o[11]})}
		if got := tdfFRows(p); strings.Join(got, "|") != strings.Join(want, "|") {
			t.Errorf("style %d: %q want %q", s, got, want)
		}
		if got := tdfFRows(fs[1].render('x', s)); got[0] != string([]byte{o[12], ' ', o[13]}) || got[1] != "  Q" {
			t.Errorf("style %d x: %q", s, got)
		}
	}
	if tdfOutlines[0][0] != 0xC4 || tdfOutlines[3][4] != 0xD5 || tdfOutlines[18][2] != 0xDE {
		t.Error("the table")
	}
	// Colour: own colours, a space on black left out, a space on red drawn, 0xFF a space.
	p := fs[2].render('C', 0)
	for _, c := range []struct {
		x, y int
		want tdfCell
	}{{0, 0, tdfCell{tdfOwn, 0xDB, 0x0C}}, {1, 0, tdfCell{}}, {0, 1, tdfCell{tdfOwn, ' ', 0x40}}, {1, 1, tdfCell{}}, {2, 1, tdfCell{tdfOwn, ' ', 0x1E}}} {
		if got := p.at(c.x, c.y); got != c.want {
			t.Errorf("colour %d,%d: %+v", c.x, c.y, got)
		}
	}
	// A line: spacing between glyphs, a space, letters it has not, the other case.
	l := fs[0].layout("ab q?", 0, -1)
	if string(l.missing) != "?" {
		t.Errorf("missing %q", string(l.missing))
	}
	sw := fs[0].spaceWidth() // A 3, B 2, Q 3, i 1: 9/4 = 2 -> 1
	if sw != 1 {
		t.Errorf("space %d", sw)
	}
	if w := 3 + 1 + 2 + sw + 1 + 3; l.pic.w != w || l.pic.h != 4 {
		t.Errorf("line %dx%d want %d", l.pic.w, l.pic.h, w)
	}
	if l2 := fs[0].layout("AB", 0, 3); l2.pic.w != 3+3+2 {
		t.Errorf("own spacing: %d", l2.pic.w)
	}
}

// Typing with the Logo tool: a glyph a step, Backspace and Ctrl+Z take them back,
// Return starts the next line of letters at the column the run began.
func TestTDFTypeUndo(t *testing.T) {
	testApp(t, "pc")
	a.setRows(30)
	fs := tdfFLib(t, 0)
	a.fg, a.bg = 14, 1
	a.toolKey("logo")
	if a.tool != "logo" {
		t.Fatalf("tool %s: %s", a.tool, a.message)
	}
	a.cx, a.cy = 5, 2
	steps := len(a.undo)
	textInput("AB")
	if len(a.undo) != steps+2 {
		t.Fatalf("%d steps", len(a.undo)-steps)
	}
	if v := a.doc.get(5, 2); cellCh(v) != 0xDC || cellFg(v) != 14 || cellBg(v) != 1 {
		t.Errorf("A: %x", v)
	}
	if v := a.doc.get(6, 3); cellCh(v) != ' ' || cellBg(v) != 1 {
		t.Errorf("A's inside: %x", v)
	}
	if v := a.doc.get(8, 2); !isBlank(v) {
		t.Errorf("the spacing column was drawn: %x", v)
	}
	if v := a.doc.get(10, 3); cellCh(v) != 0xDF {
		t.Errorf("B: %x", v)
	}
	if a.cx != 5+3+1+2+1 || a.cy != 2 {
		t.Errorf("cursor %d,%d", a.cx, a.cy)
	}
	editorKey("Backspace", false, false, false)
	if !isBlank(a.doc.get(10, 3)) || cellCh(a.doc.get(5, 2)) != 0xDC || a.cx != 9 || len(a.undo) != steps+1 {
		t.Errorf("Backspace: cursor %d, B %x", a.cx, a.doc.get(10, 3))
	}
	textInput("B")
	runAction("undo")
	if !isBlank(a.doc.get(10, 3)) || a.cx != 9 {
		t.Error("Ctrl+Z")
	}
	// Return: under the letters, at the run's first column.
	editorKey("Return", false, false, false)
	if a.cx != 5 || a.cy != 2+fs[0].lineHeight() {
		t.Errorf("Return: %d,%d", a.cx, a.cy)
	}
	textInput("q")
	if cellCh(a.doc.get(5, 5)) != 0xDB || cellCh(a.doc.get(7, 8)) != 0xDF {
		t.Error("the second line")
	}
	// Backspace with no glyph of this run to take back is the normal Backspace.
	a.cx, a.cy = 40, 20
	a.doc.set(39, 20, makeCell('x', 7, 0, 0))
	editorKey("Backspace", false, false, false)
	if cellCh(a.doc.get(39, 20)) != ' ' {
		t.Error("plain Backspace")
	}
	// A letter that does not fit goes to the next line of letters.
	a.cx, a.cy = 77, 10
	logo.tab = nil
	textInput("AA")
	if cellCh(a.doc.get(77, 10)) != 0xDC || cellCh(a.doc.get(77, 13)) != 0xDC {
		t.Error("wrap")
	}
	// A click starts a new run there.
	a.logoClick(3, 20)
	textInput("i")
	editorKey("Return", false, false, false)
	if a.cx != 3 {
		t.Errorf("after a click Return goes to %d", a.cx)
	}
	// A character the font lacks is said, not typed.
	before := len(a.undo)
	textInput("%")
	if len(a.undo) != before || !strings.Contains(a.message, "has no") {
		t.Errorf("missing: %s", a.message)
	}
}

// Colour fonts keep their colours (the DOS attribute's BIOS order turned into the page's).
func TestTDFColourCells(t *testing.T) {
	testApp(t, "pc")
	tdfFLib(t, 2)
	a.setTool("logo")
	a.cx, a.cy = 0, 0
	a.doc.set(1, 0, makeCell('z', 2, 0, 0))
	textInput("C")
	if v := a.doc.get(0, 0); cellCh(v) != 0xDB || cellFg(v) != 9 || cellBg(v) != 0 { // 0x0C: light red
		t.Errorf("C: %x", v)
	}
	if cellCh(a.doc.get(1, 0)) != 'z' {
		t.Error("a space on black was drawn")
	}
	if v := a.doc.get(0, 1); cellBg(v) != 1 { // 0x40: red background (BIOS 4 = ANSI 1)
		t.Errorf("red space: %x", v)
	}
	if v := a.doc.get(2, 1); cellCh(v) != ' ' || cellFg(v) != 11 || cellBg(v) != 4 { // 0x1E: yellow on blue
		t.Errorf("solid space: %x", v)
	}
	if a.cx != 3+1 { // spacing byte 2: a gap of 1 (tdfFont.gap)
		t.Errorf("cursor %d", a.cx)
	}
}

// Amiga pages show Latin-1: a font drawn with blocks is refused, letters it has are typed.
func TestTDFAmigaAndUnicode(t *testing.T) {
	testApp(t, "amiga")
	fs := tdfFLib(t, 0)
	asc := tdfFFile(tdfFFont{name: "Ascii", kind: tdfBlock, glyphs: map[rune][]string{'A': {"/\\", "/~\\"}, 'B': {"\xe9"}}})
	af, err := parseTDF(asc, "")
	if err != nil {
		t.Fatal(err)
	}
	a.setTool("logo")
	pure, _ := parseTDF(tdfFFile(tdfFFont{name: "Pure", kind: tdfBlock, glyphs: map[rune][]string{'A': {"/\\"}}}), "")
	if a.tdfFits(fs[0]) || a.tdfFits(af[0]) || !a.tdfFits(pure[0]) {
		t.Error("fits")
	}
	textInput("A")
	if !strings.Contains(a.message, "Amiga") || !isBlank(a.doc.get(0, 0)) {
		t.Errorf("not refused: %s", a.message)
	}
	tdfLib.fonts = append(tdfLib.fonts, af[0])
	tdfChoose(af[0])
	textInput("AB")
	if cellCh(a.doc.get(0, 0)) != '/' || cellCh(a.doc.get(1, 1)) != '~' {
		t.Error("ASCII glyph on the Amiga page")
	}
	if !strings.Contains(a.message, "Amiga") { // CP437 0xE9 is Θ, not in Latin-1
		t.Errorf("Θ: %s", a.message)
	}

	// A Unicode page takes every CP437 character.
	testApp(t, "uni")
	tdfFLib(t, 0)
	a.setTool("logo")
	a.fg, a.bg = 12, 0
	textInput("A")
	if v := a.doc.get(0, 0); uniCode(v) != 0x2584 || a.message != "" && strings.Contains(a.message, "Amiga") {
		t.Errorf("Unicode cell %x (%x)", v, uniCode(v))
	}
	tdfFLib(t, 2)
	a.cx, a.cy = 0, 5
	textInput("C")
	if v := a.doc.get(0, 5); uniCode(v) != 0x2588 {
		t.Errorf("colour on Unicode %x", uniCode(v))
	}
	// Pages of other machines have no Logo tool.
	testApp(t, "c64")
	tdfFLib(t, 0)
	a.toolKey("logo")
	if a.tool == "logo" {
		t.Error("Logo tool on a C64 page")
	}
}

// The line requester lays the line out and hands it over as a block to stamp.
func TestTDFLineStamp(t *testing.T) {
	testApp(t, "pc")
	tdfFLib(t, 0)
	openTDFLine()
	d, ok := a.dialog.(*TDFLineReq)
	if !ok {
		t.Fatalf("%T", a.dialog)
	}
	d.text.set("AB")
	d.stamp()
	if a.dialog != nil || a.float == nil || a.float.w != 3+1+2 || a.float.h != 3 {
		t.Fatalf("block %+v", a.float)
	}
	a.stamp(10, 4, false)
	if cellCh(a.doc.get(10, 4)) != 0xDC || cellCh(a.doc.get(15, 5)) != 0xDF {
		t.Error("stamped")
	}
	if prefs.TDF.Text != "AB" {
		t.Error("the text is not kept")
	}
}

// Fonts from folders, .zip files and a file opened: each font once, sorted by name.
func TestTDFLibrary(t *testing.T) {
	testApp(t, "pc")
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "one.tdf"), tdfFFile(tdfFBlock(), tdfFColour()), 0o644)
	os.MkdirAll(filepath.Join(dir, "sub"), 0o755)
	os.WriteFile(filepath.Join(dir, "sub", "TWO.TDF"), tdfFFile(tdfFOutline(), tdfFBlock()), 0o644)
	os.WriteFile(filepath.Join(dir, "sub", "bad.tdf"), []byte("not a font"), 0o644)
	var zb bytes.Buffer
	zw := zip.NewWriter(&zb)
	w, _ := zw.Create("fonts/three.tdf")
	w.Write(tdfFFile(tdfFFont{name: "Zipped", kind: tdfBlock, glyphs: map[rune][]string{'A': {"x"}}}))
	zw.Close()
	os.WriteFile(filepath.Join(dir, "pack.zip"), zb.Bytes(), 0o644)
	tdfFClean(t)
	tdfReload()
	if len(tdfLib.fonts) != 0 {
		t.Fatal("fonts from nowhere")
	}
	if n := addTDFPath(dir); n != 4 {
		t.Errorf("%d fonts added", n)
	}
	var names []string
	for _, f := range tdfLib.fonts {
		names = append(names, f.name)
	}
	if strings.Join(names, ",") != "TestBlock,TestColour,TestOutline,Zipped" {
		t.Errorf("fonts %v", names)
	}
	_, bad := tdfScan([]string{dir})
	if len(bad) != 1 || !strings.HasSuffix(bad[0], "bad.tdf") {
		t.Errorf("bad %v", bad)
	}
	// The prefs keep the folder: a new start finds the fonts again.
	tdfLib.loaded = false
	if f := tdfNow(); f == nil || len(tdfLib.fonts) != 4 {
		t.Error("not found again")
	}
	// Opening a .tdf adds it and opens the list.
	other := filepath.Join(t.TempDir(), "x.tdf")
	os.WriteFile(other, tdfFFile(tdfFFont{name: "Opened", kind: tdfOutline, glyphs: map[rune][]string{'A': {"EF"}}}), 0o644)
	a.openPath(other, "auto")
	if _, ok := a.dialog.(*TDFReq); !ok || tdfNow().name != "Opened" || a.tool != "logo" {
		t.Errorf("open: %T %s", a.dialog, a.message)
	}
	// The picker finds by name, kind and file.
	d := a.dialog.(*TDFReq)
	d.find.set("colour")
	d.filter()
	if len(d.list) != 1 || d.list[0].name != "TestColour" {
		t.Errorf("find colour: %d", len(d.list))
	}
	d.find.set("two.tdf")
	d.filter()
	if len(d.list) != 1 || d.list[0].name != "TestOutline" { // its block font is one.tdf's, listed once
		t.Errorf("find by file: %d", len(d.list))
	}
	d.find.set("")
	d.filter()
	d.sel = 1
	d.use()
	if tdfNow() != d.list[1] || a.dialog != nil {
		t.Error("use")
	}
}

// The download: the zip's .tdf files land in the data folder and are read.
func TestTDFDownload(t *testing.T) {
	testApp(t, "pc")
	tdfFClean(t)
	var zb bytes.Buffer
	zw := zip.NewWriter(&zb)
	w, _ := zw.Create("TDFONTSALL.TDF")
	w.Write(tdfFFile(tdfFBlock(), tdfFOutline(), tdfFColour()))
	w, _ = zw.Create("readme.txt")
	w.Write([]byte("hello"))
	zw.Close()
	served := zb.Bytes()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write(served) }))
	defer srv.Close()
	old := tdfZipURL
	tdfZipURL = srv.URL + "/TDFONTSALL.ZIP"
	t.Cleanup(func() { tdfZipURL = old })
	// The tool on a page with no fonts asks.
	a.toolKey("logo")
	req, ok := a.dialog.(*FontReq)
	if !ok || !strings.Contains(req.why, "Roy/SAC") {
		t.Fatalf("no question: %T", a.dialog)
	}
	if err := req.fetch(); err != nil {
		t.Fatal(err)
	}
	msg, err := req.load()
	if err != nil || !strings.Contains(msg, "3 TheDraw fonts") {
		t.Fatalf("%s %v", msg, err)
	}
	if _, err := os.Stat(filepath.Join(tdfDir(), "TDFONTSALL.TDF")); err != nil {
		t.Error(err)
	}
	// Not a zip, or a zip without fonts: refused.
	served = []byte("<html>")
	if err := fetchTDF(); err == nil {
		t.Error("not a zip")
	}
	zb.Reset()
	zw = zip.NewWriter(&zb)
	w, _ = zw.Create("a.txt")
	w.Write([]byte("x"))
	zw.Close()
	served = zb.Bytes()
	if err := fetchTDF(); err == nil {
		t.Error("no fonts")
	}
}

// The picker and the line requester draw (headless) and their previews show the fonts.
func TestTDFDrawRequesters(t *testing.T) {
	testApp(t, "pc")
	tdfFLib(t, 1)
	ui.fb = &FB{}
	ui.fb.resize(1200, 900)
	ui.fb.clip = Rect{0, 0, 1200, 900}
	openTDFPicker(nil)
	d := a.dialog.(*TDFReq)
	d.draw(ui)
	if len(d.thumbs) != 3 {
		t.Errorf("%d previews", len(d.thumbs))
	}
	a.dialog = nil
	openTDFLine()
	a.dialog.draw(ui)
	a.setTool("logo")
	a.drawToolOptions(ui, 0, 0, 300)
}

// Real fonts: Roy/SAC's collection (CHARCOAL_TDF, a .tdf or a folder; by default
// ~/.cache/charcoal-tdf/roysac) and TheDraw 4.63's own TDFONTS.TDF (CHARCOAL_THEDRAW).
func TestTDFRealFiles(t *testing.T) {
	h, _ := os.UserHomeDir()
	path := os.Getenv("CHARCOAL_TDF")
	if path == "" {
		path = filepath.Join(h, ".cache/charcoal-tdf/roysac")
	}
	td := os.Getenv("CHARCOAL_THEDRAW")
	if td == "" {
		td = filepath.Join(h, ".cache/charcoal-others/thedraw/td463")
	}
	if _, err := os.Stat(path); err != nil {
		t.Skip("no fonts in", path)
	}
	fonts, bad := tdfScan([]string{path})
	if len(bad) > 0 || len(fonts) < 1000 {
		t.Fatalf("%d fonts, bad %v", len(fonts), bad)
	}
	kinds := map[int]int{}
	glyphs := 0
	for _, f := range fonts {
		kinds[f.kind]++
		for c := rune(tdfFirst); c < tdfFirst+tdfChars; c++ {
			for s := 0; s < map[bool]int{true: 19, false: 1}[f.kind == tdfOutline]; s++ {
				p := f.render(c, s)
				if p == nil {
					continue
				}
				glyphs++
				if p.w > 60 || p.h > 20 {
					t.Errorf("%s %q: %dx%d", f.name, c, p.w, p.h)
				}
			}
		}
		if f.lineHeight() < 1 || f.lineHeight() > 12 {
			t.Errorf("%s: line height %d", f.name, f.lineHeight())
		}
	}
	t.Logf("%d fonts (outline %d, block %d, colour %d), %d glyphs", len(fonts), kinds[0], kinds[1], kinds[2], glyphs)

	b, err := os.ReadFile(filepath.Join(td, "TDFONTS.TDF"))
	if err != nil {
		t.Skip("no TDFONTS.TDF in", td)
	}
	fs, err := parseTDF(b, "TDFONTS.TDF")
	if err != nil || len(fs) != 11 || fs[0].name != "Outline" || fs[0].kind != tdfOutline || fs[2].name != "ThickOutline" {
		t.Fatalf("%v %d %v", err, len(fs), fs[0].name)
	}
	// TheDraw's own Outline 'Q' has a descender line: the height byte leaves it out.
	q := fs[0].glyph('Q')
	if q.h != 6 || len(q.rows) != 7 || !q.desc[6] || fs[0].lineHeight() != 6 {
		t.Errorf("Q: h %d rows %d", q.h, len(q.rows))
	}
	// Its 'Q' in style 1 (TheDraw's first): single lines, the solid spaces blank.
	got := tdfFRows(fs[0].render('Q', 0))
	if got[0] != "\xda\xc4\xc4\xc4\xc4\xc4\xc4\xbf" || got[2] != "\xb3 \xb3  \xb3 \xb3" || got[6] != "    \xc0\xc4\xd9." {
		t.Errorf("Q: %q", got)
	}
}
