package main

// Tests of TheDraw .td writing and the TheDraw font editor (fed.go). Helpers are
// prefixed fedF.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// fedFPage is a random PC page: every character (0-255), colours, blink, runs.
func fedFPage(rng *rand.Rand, w, h int) *Doc {
	d := newDoc(w, h)
	for i := range d.cells {
		switch rng.Intn(5) {
		case 0:
			continue // blank
		case 1:
			if i > 0 {
				d.cells[i] = d.cells[i-1] // runs
				continue
			}
		}
		d.cells[i] = attrCell(byte(rng.Intn(256)), byte(rng.Intn(256)))
	}
	return d
}

// fedFPadded is d as a TheDraw page of w x h: blank where d has no cell.
func fedFPadded(d *Doc, w, h int) *Doc {
	p := newDoc(w, h)
	for y := 0; y < min(h, d.rows); y++ {
		for x := 0; x < min(w, d.cols); x++ {
			p.set(x, y, d.get(x, y)&0xFFFF)
		}
	}
	return p
}

func fedFSame(t *testing.T, what string, got, want *Doc) {
	t.Helper()
	if got.cols != want.cols || got.rows != want.rows {
		t.Fatalf("%s: %dx%d, want %dx%d", what, got.cols, got.rows, want.cols, want.rows)
	}
	for i := 0; i < want.cols*want.rows; i++ {
		if got.cells[i] != want.cells[i] {
			t.Fatalf("%s: cell %d,%d is %08x, want %08x", what, i%want.cols, i/want.cols, got.cells[i], want.cells[i])
		}
	}
}

// Pages of every size and content come back from writeTD cell for cell (padded
// to TheDraw's width of 80 or 160 and the tallest page's height).
func TestFedTDRoundTrip(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	for n := 0; n < 200; n++ {
		pages := make([]*Doc, 1+rng.Intn(tdMaxPages))
		for i := range pages {
			pages[i] = fedFPage(rng, 1+rng.Intn(160), 1+rng.Intn(60))
		}
		b, err := writeTD(pages, rng.Intn(len(pages)))
		if err != nil {
			t.Fatal(err)
		}
		got, err := parseTD(b)
		if err != nil || len(got) != len(pages) {
			t.Fatalf("read back: %v, %d pages of %d", err, len(got), len(pages))
		}
		w, h := tdSize(pages)
		for i := range pages {
			fedFSame(t, "page", got[i], fedFPadded(pages[i], w, h))
		}
	}
	// Too busy: control characters in every colour, in a 160x100 page.
	d := newDoc(160, 100)
	for i := range d.cells {
		d.cells[i] = attrCell(byte(i%31+1), byte(i*37))
	}
	if _, err := writeTD([]*Doc{d}, 0); err == nil {
		t.Fatal("a page over 64 KB crunched was written")
	}
}

// The header: TheDraw's magic and version, the size twice, the page layers in use.
func TestFedTDHeader(t *testing.T) {
	b, _ := writeTD([]*Doc{newDoc(40, 30), newDoc(80, 10), newDoc(80, 2)}, 1)
	if !isTD(b) || string(b[22:27]) != "\x044.60" {
		t.Fatalf("magic/version % x", b[:27])
	}
	le := func(o int) int { return int(b[o]) | int(b[o+1])<<8 }
	if le(39) != 80 || le(41) != 30 || le(340) != 80 || le(342) != 30 || le(31) != 2 {
		t.Fatalf("size %d x %d / %d x %d, page %d", le(39), le(41), le(340), le(342), le(31))
	}
	if !bytes.Equal(b[55:63], []byte{2, 2, 2, 1, 1, 1, 1, 1}) {
		t.Fatalf("page layers % x", b[55:63])
	}
}

// TheDraw 4.63's own files read, written and read again give the same pages.
func TestFedTDTheDrawFiles(t *testing.T) {
	dir := os.Getenv("CHARCOAL_THEDRAW")
	if dir == "" {
		h, _ := os.UserHomeDir()
		dir = filepath.Join(h, ".cache/charcoal-others/thedraw/td463")
	}
	for _, name := range []string{"DEMO.TD", "SHADING.TD"} {
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Skip("no TheDraw files in", dir)
		}
		pages, _ := parseTD(b)
		out, err := writeTD(pages, 0)
		if err != nil {
			t.Fatal(err)
		}
		again, err := parseTD(out)
		if err != nil || len(again) != len(pages) {
			t.Fatalf("%s: %v", name, err)
		}
		for i := range pages {
			fedFSame(t, name, again[i], pages[i])
		}
		if len(out) > len(b)+40 {
			t.Errorf("%s: %d bytes, TheDraw's %d", name, len(out), len(b))
		}
	}
}

// CHARCOAL_FED_TDOUT=dir writes .td files for TheDraw itself to load (tools: the
// knowledge note on TheDraw in QEMU).
func TestFedTDSamples(t *testing.T) {
	dir := os.Getenv("CHARCOAL_FED_TDOUT")
	if dir == "" {
		t.Skip("CHARCOAL_FED_TDOUT not set")
	}
	rng := rand.New(rand.NewSource(1))
	write := func(name string, pages []*Doc) {
		b, err := writeTD(pages, 0)
		if err != nil {
			t.Fatal(err)
		}
		os.WriteFile(filepath.Join(dir, name), b, 0o644)
	}
	write("RAND25.TD", []*Doc{fedFPage(rng, 80, 25)})
	write("RAND50.TD", []*Doc{fedFPage(rng, 80, 50)})
	write("RAND30.TD", []*Doc{fedFPage(rng, 80, 30)})
	write("WIDE.TD", []*Doc{fedFPage(rng, 160, 25)})
	write("PAGES3.TD", []*Doc{fedFPage(rng, 80, 25), fedFPage(rng, 80, 25), fedFPage(rng, 80, 25)})
	// Fonts for TheDraw's TDFONTS: '!' made of lines of text, 'A' a box, 'Q' with a descender.
	for _, kind := range []int{tdfBlock, tdfColour} {
		var glyphs [tdfChars][]byte
		for i, rows := range map[rune][]string{'!': {"CharEdit", "font", "!!!"}, 'A': {"\xdb\xdf\xdb", "\xdb\xdc\xdb", "\xdb \xdb"}, 'Q': {"\xdb\xdf\xdb", "\xdb \xdb", "\xdf\xdf\xdb", "  \xdf"}} {
			d := newDoc(8, len(rows))
			for y, r := range rows {
				for x := 0; x < len(r); x++ {
					d.set(x, y, attrCell(r[x], byte(0x1E+y*0x11)))
				}
			}
			desc := make([]bool, len(rows))
			desc[len(rows)-1] = i == 'Q'
			glyphs[i-tdfFirst], _, _ = encodeTDFGlyph(kind, d, desc)
		}
		font, _ := encodeTDFFont(fmt.Sprintf("Fed %s", tdfKindNames[kind]), kind, 1, &glyphs)
		os.WriteFile(filepath.Join(dir, fmt.Sprintf("FED%d.TDF", kind)), tdfFileBytes(font), 0o644)
	}
	// TheDraw's TDFONTS.TDF with its second font (BigOutline) renamed and its 'A' changed, saved back into the pack.
	h, _ := os.UserHomeDir()
	if b, err := os.ReadFile(filepath.Join(h, ".cache/charcoal-others/thedraw/td463/TDFONTS.TDF")); err == nil {
		testApp(t, "pc")
		tdfFClean(t)
		pack := filepath.Join(dir, "PACK.TDF")
		os.WriteFile(pack, b, 0o644)
		fs, _ := parseTDF(b, pack)
		a.fedOpenFont(fs[1])
		fedFDraw(map[[2]int]uint32{{1, 1}: attrCell('@', 7), {2, 1}: attrCell('@', 7)})
		fieldFedName.set("BigEdited")
		fieldFedName.enter()
		a.save(false)
	}
}

// CHARCOAL_FED_TDBIN="file.td file.bin": TheDraw loaded file.td and saved it as a
// .bin (80 columns); its rows must be the .td's first ones, cell for cell.
func TestFedTDTheDrawBin(t *testing.T) {
	spec := strings.Fields(os.Getenv("CHARCOAL_FED_TDBIN"))
	if len(spec) < 2 {
		t.Skip("CHARCOAL_FED_TDBIN not set")
	}
	td, _ := os.ReadFile(spec[0])
	bin, _ := os.ReadFile(spec[1])
	pages, err := parseTD(td)
	if err != nil {
		t.Fatal(err)
	}
	page := 0
	if len(spec) > 2 {
		page, _ = strconv.Atoi(spec[2])
	}
	p := pages[page]
	rows := len(bin) / 160 // TheDraw's worksheet as set up: 80 columns here
	t.Logf("%s page %d: %dx%d, TheDraw's .bin 80x%d", spec[0], page+1, p.cols, p.rows, rows)
	for y := 0; y < min(rows, p.rows); y++ {
		for x := 0; x < 80; x++ {
			v, i := p.get(x, y), 2*(y*80+x)
			if bin[i] != byte(cellCh(v)) || bin[i+1] != cellAttr(v) {
				t.Fatalf("cell %d,%d: TheDraw %02x %02x, CharEdit %02x %02x", x, y, bin[i], bin[i+1], cellCh(v), cellAttr(v))
			}
		}
	}
}

func TestFedTDDumpBin(t *testing.T) {
	spec := strings.Fields(os.Getenv("CHARCOAL_FED_TDDUMP"))
	if len(spec) != 2 {
		t.Skip()
	}
	td, _ := os.ReadFile(spec[0])
	pages, _ := parseTD(td)
	os.WriteFile(spec[1], writeBin(pages[0]), 0o644)
}

// ---------------------------------------------------------------- the font editor

// fedFGlyphPage is a random letter page for a font of kind: every character, the
// colours, descender rows at the bottom.
func fedFGlyphPage(rng *rand.Rand, kind int) (*Doc, []bool) {
	d := newDoc(1+rng.Intn(30), 1+rng.Intn(12))
	for i := range d.cells {
		if rng.Intn(4) == 0 {
			continue
		}
		at := byte(rng.Intn(256))
		if kind != tdfColour {
			at = 0x07
		}
		d.cells[i] = attrCell(byte(rng.Intn(256)), at)
	}
	desc := make([]bool, d.rows)
	for y := d.rows - rng.Intn(min(3, d.rows)); y < d.rows; y++ {
		if y > 0 {
			desc[y] = true
		}
	}
	for y := 0; y < d.rows; y++ { // an '&' at a block letter's right edge cannot be kept (encodeTDFGlyph)
		if v := d.get(d.cols-1, y); kind != tdfColour && !desc[y] && cellCh(v) == '&' {
			d.set(d.cols-1, y, attrCell('x', 7))
		}
	}
	return d, desc
}

// fedFLook is a letter cell as the font draws it: transparent cells (a space in a
// block font, a space on black in a colour font) all alike, character 0 as the
// solid space it is written as, 13 and the '&' TheDraw cannot keep as the writer's.
func fedFLook(kind int, v uint32) uint32 {
	c, at := byte(cellCh(v)), cellAttr(v)
	if kind != tdfColour {
		at = 0x07
	}
	switch {
	case c == ' ' && (kind != tdfColour || at&0xF0 == 0):
		return 0
	case c == 0 || c == 13 && kind == tdfColour || c == '&' && kind == tdfColour && (at == 0 || at == 13):
		c = 0xFF
	case c == 13:
		return 0
	}
	return uint32(c) | uint32(at)<<8 | 1<<24
}

// Letters written by encodeTDFGlyph read back in tdf.go as drawn: every cell,
// the width, the lines, the descenders; whole fonts of every kind.
func TestFedGlyphRoundTrip(t *testing.T) {
	rng := rand.New(rand.NewSource(3))
	for _, kind := range []int{tdfOutline, tdfBlock, tdfColour} {
		for n := 0; n < 300; n++ {
			var glyphs [tdfChars][]byte
			pages := map[int]*Doc{}
			descs := map[int][]bool{}
			for k := 0; k < 1+rng.Intn(6); k++ {
				i := rng.Intn(tdfChars)
				d, desc := fedFGlyphPage(rng, kind)
				g, _, err := encodeTDFGlyph(kind, d, desc)
				if err != nil {
					t.Fatal(err)
				}
				glyphs[i], pages[i], descs[i] = g, d, desc
			}
			font, err := encodeTDFFont("Test font", kind, 2, &glyphs)
			if err != nil {
				t.Fatal(err)
			}
			fs, err := parseTDF(tdfFileBytes(font), "x.tdf")
			if err != nil || len(fs) != 1 || fs[0].name != "Test font" || fs[0].kind != kind || fs[0].spacing != 2 {
				t.Fatalf("read back: %v %d %q %d %d", err, len(fs), fs[0].name, fs[0].kind, fs[0].spacing)
			}
			f := fs[0]
			for i, d := range pages {
				g := f.glyph(rune(tdfFirst + i))
				if g == nil {
					t.Fatalf("kind %d %q: gone", kind, fedLetter(i))
				}
				if g.width() != d.cols || len(g.rows) != d.rows || !slices.Equal(g.desc, descs[i]) || g.h != g.body() {
					t.Fatalf("kind %d %q: %dx%d desc %v (h %d), want %dx%d %v", kind, fedLetter(i), g.width(), len(g.rows), g.desc, g.h, d.cols, d.rows, descs[i])
				}
				back, _ := fedGlyphDoc(kind, g)
				for y := 0; y < d.rows; y++ {
					for x := 0; x < d.cols; x++ {
						if w, got := fedFLook(kind, d.get(x, y)), fedFLook(kind, back.get(x, y)); w != got {
							t.Fatalf("kind %d %q cell %d,%d: %06x, want %06x", kind, fedLetter(i), x, y, got, w)
						}
					}
				}
				if !bytes.Equal(f.rawGlyph(i), glyphs[i]) {
					t.Fatalf("rawGlyph %q", fedLetter(i))
				}
			}
		}
	}
}

// fedFRealFonts are TheDraw's own fonts and Roy/SAC's collection, when here.
func fedFRealFonts(t *testing.T) []string {
	h, _ := os.UserHomeDir()
	var files []string
	td := os.Getenv("CHARCOAL_THEDRAW")
	if td == "" {
		td = filepath.Join(h, ".cache/charcoal-others/thedraw/td463")
	}
	if _, err := os.Stat(filepath.Join(td, "TDFONTS.TDF")); err == nil {
		files = append(files, filepath.Join(td, "TDFONTS.TDF"))
	}
	dir := os.Getenv("CHARCOAL_TDF")
	if dir == "" {
		dir = filepath.Join(h, ".cache/charcoal-tdf/roysac")
	}
	filepath.WalkDir(dir, func(p string, e os.DirEntry, err error) error {
		if err == nil && !e.IsDir() && strings.EqualFold(filepath.Ext(p), ".tdf") {
			files = append(files, p)
		}
		return nil
	})
	if len(files) == 0 {
		t.Skip("no real TheDraw fonts here")
	}
	return files
}

// Every real font opened in the editor: unchanged it is written back byte for
// byte (each font on its own); every letter written anew draws the same.
func TestFedRealFonts(t *testing.T) {
	fonts, glyphs := 0, 0
	for _, file := range fedFRealFonts(t) {
		b, _ := os.ReadFile(file)
		fs, err := parseTDF(b, file)
		if err != nil {
			t.Fatal(file, err)
		}
		for _, f := range fs {
			fonts++
			ff := newFedFont(f, "", 0, 0)
			var raw, fresh [tdfChars][]byte
			for i := range ff.slots {
				s := ff.slots[i]
				if !s.defined {
					continue
				}
				raw[i] = s.raw
				g, _, err := encodeTDFGlyph(f.kind, s.doc, s.desc)
				if err != nil {
					t.Fatalf("%s %q: %v", f.name, fedLetter(i), err)
				}
				fresh[i] = g
			}
			out, err := encodeTDFFont(ff.name, ff.kind, ff.spacing, &raw)
			if err != nil {
				t.Fatal(err)
			}
			again, err := parseTDF(tdfFileBytes(out), "")
			if err != nil || again[0].kind != f.kind || again[0].spacing != f.spacing || again[0].name != f.name && f.name != "(no name)" {
				t.Fatalf("%s %q: unchanged font written differently (%v) %q %d %d", file, f.name, err, again[0].name, again[0].kind, again[0].spacing)
			}
			for i := range raw {
				if !bytes.Equal(again[0].rawGlyph(i), f.rawGlyph(i)) {
					t.Fatalf("%s %s %q: unchanged letter written differently", file, f.name, fedLetter(i))
				}
			}
			out, _ = encodeTDFFont(ff.name, ff.kind, ff.spacing, &fresh)
			nf, err := parseTDF(tdfFileBytes(out), "")
			if err != nil {
				t.Fatal(err)
			}
			for i := range ff.slots {
				if !ff.slots[i].defined {
					continue
				}
				glyphs++
				c := rune(tdfFirst + i)
				for _, style := range []int{0, 16} {
					p, q := f.render(c, style), nf[0].render(c, style)
					if p == nil || q == nil || p.w != q.w || p.h != q.h || !slices.Equal(p.cells, q.cells) {
						t.Fatalf("%s %s %q style %d: drawn differently after writing", filepath.Base(file), f.name, c, style)
					}
				}
				if og, ng := f.glyph(c), nf[0].glyph(c); og.body() != ng.body() || !slices.Equal(og.desc, ng.desc) {
					t.Fatalf("%s %q: descenders", f.name, c)
				}
			}
		}
	}
	t.Logf("%d fonts, %d letters", fonts, glyphs)
}

// fedFParts is a glyph as IcyDraw's retrofont reads it (tdf.rs decode_glyph): one
// word per part, N line, E descender mark, S skip, H hard blank, F fill, O hole,
// P outline letter, C character, A colour cell.
func fedFParts(kind int, g *tdfGlyph) string {
	uc := func(c byte) rune { // retrofont's CP437 table keeps these as control characters
		if c == 0 || c == 8 || c == 9 || c == 10 || c == 13 || c == 26 || c == 27 || c == 127 {
			return rune(c)
		}
		return cp437[c]
	}
	var out []string
	for y, row := range g.rows {
		if y > 0 {
			out = append(out, "N")
		}
		for _, c := range row {
			switch {
			case c.ch == ' ':
				out = append(out, "S")
			case c.ch == 0xFF && kind != tdfOutline:
				out = append(out, "H")
			case kind == tdfColour:
				out = append(out, fmt.Sprintf("A%d:%d:%d:%d", uc(c.ch), c.attr&15, c.attr>>4&7, c.attr>>7))
			case kind == tdfOutline && c.ch == '@':
				out = append(out, "F")
			case kind == tdfOutline && c.ch == 'O':
				out = append(out, "O")
			case kind == tdfOutline && c.ch >= 'A' && c.ch <= 'R':
				out = append(out, fmt.Sprintf("P%d", c.ch))
			default:
				out = append(out, fmt.Sprintf("C%d", uc(c.ch)))
			}
		}
		if g.desc[y] {
			out = append(out, "E")
		}
	}
	return strings.Join(out, " ")
}

// CHARCOAL_FED_ICY (a command: IcyDraw's font library dumping a .tdf, the
// cctdf example of the knowledge note) and CHARCOAL_ICYDIR (a folder it can read):
// fonts CharEdit writes read the same in IcyDraw, letter for letter.
func TestFedIcyDrawReads(t *testing.T) {
	cmdline, dir := os.Getenv("CHARCOAL_FED_ICY"), os.Getenv("CHARCOAL_ICYDIR")
	if cmdline == "" || dir == "" {
		t.Skip("CHARCOAL_FED_ICY / CHARCOAL_ICYDIR not set")
	}
	rng := rand.New(rand.NewSource(11))
	checked := 0
	for _, kind := range []int{tdfOutline, tdfBlock, tdfColour} {
		for n := 0; n < 20; n++ {
			var glyphs [tdfChars][]byte
			for i := range glyphs {
				if rng.Intn(3) > 0 {
					continue
				}
				d, desc := fedFGlyphPage(rng, kind)
				for k, v := range d.cells { // retrofont reads every '&' as the descender mark
					if cellCh(v) == '&' {
						d.cells[k] = attrCell('%', cellAttr(v))
					}
				}
				glyphs[i], _, _ = encodeTDFGlyph(kind, d, desc)
			}
			font, err := encodeTDFFont(fmt.Sprintf("Font %d-%d", kind, n), kind, n%5, &glyphs)
			if err != nil {
				t.Fatal(err)
			}
			b := tdfFileBytes(font)
			p := filepath.Join(dir, "fed-check.tdf")
			if err := os.WriteFile(p, b, 0o644); err != nil {
				t.Fatal(err)
			}
			args := append(strings.Fields(cmdline), p)
			out, err := exec.Command(args[0], args[1:]...).Output()
			if err != nil {
				t.Fatalf("%v: %v", args, err)
			}
			f := tdfFParse(t, b)
			want := []string{fmt.Sprintf("FONT %q %d %d", f.name, f.kind, f.spacing)}
			for i := range glyphs {
				if g := f.glyph(rune(tdfFirst + i)); g != nil {
					want = append(want, strings.TrimSpace(fmt.Sprintf("GLYPH %d %d %d %s", tdfFirst+i, g.w, g.h, fedFParts(kind, g))))
					checked++
				}
			}
			got := strings.Split(strings.TrimSpace(string(out)), "\n")
			if len(got) != len(want) {
				t.Fatalf("kind %d: IcyDraw reads %d lines, want %d:\n%s", kind, len(got), len(want), out)
			}
			for i := range want {
				if got[i] != want[i] {
					t.Fatalf("kind %d:\nIcyDraw  %s\nCharEdit %s", kind, got[i], want[i])
				}
			}
		}
	}
	t.Logf("%d letters read the same", checked)
}

func tdfFParse(t *testing.T, b []byte) *tdfFont {
	fs, err := parseTDF(b, "")
	if err != nil || len(fs) != 1 {
		t.Fatal(err)
	}
	return fs[0]
}

// fedFDraw puts cells on the page as one undo step, as a tool does.
func fedFDraw(cells map[[2]int]uint32) {
	a.pushUndo()
	for p, v := range cells {
		a.doc.set(p[0], p[1], v)
	}
}

func fedFNew(t *testing.T, kind int, gap string) {
	t.Helper()
	openFedNew()
	d := a.dialog.(*FedNewReq)
	d.name.set("My font")
	d.kind = kind
	d.spacing.set(gap)
	d.make()
	if a.fed() == nil || a.saveFmt != "tdf" || a.doc.cols != fedNewW || a.doc.rows != fedNewH {
		t.Fatalf("no font tab: %s", a.message)
	}
}

// A new font: letters drawn, each with its own undo, saved, read back by tdf.go,
// opened again in the editor and saved unchanged byte for byte.
func TestFedNewFontSaveOpen(t *testing.T) {
	testApp(t, "pc")
	tdfFClean(t)
	fedFNew(t, tdfColour, "1")
	ff := a.fed()
	if fedLetter(ff.cur) != "A" || ff.spacing != 2 || ff.gap() != 1 {
		t.Fatalf("letter %s spacing %d", fedLetter(ff.cur), ff.spacing)
	}
	fedFDraw(map[[2]int]uint32{{0, 0}: attrCell(0xDB, 0x1E), {1, 1}: attrCell('x', 0x4F)})
	aUndo := len(a.undo)
	a.fedGo('B' - tdfFirst)
	if len(a.undo) != 0 || a.doc.cols != fedNewW || !docBlank(a.doc) {
		t.Fatalf("B: %d undo steps, %dx%d", len(a.undo), a.doc.cols, a.doc.rows)
	}
	a.setCols(5)
	a.setRows(3)
	fedFDraw(map[[2]int]uint32{{4, 2}: attrCell('B', 0x0C)})
	a.fedSetDesc(1)
	a.doUndo() // B's drawing back, not A's
	if cellCh(a.doc.get(4, 2)) == 'B' {
		t.Fatal("undo on B")
	}
	a.doRedo()
	a.fedGo('A' - tdfFirst)
	if len(a.undo) != aUndo || cellCh(a.doc.get(0, 0)) != 0xDB {
		t.Fatalf("A came back with %d steps", len(a.undo))
	}
	a.fedGo('C' - tdfFirst) // looked at, left blank: not in the font
	a.fedGo('A' - tdfFirst)

	path := filepath.Join(t.TempDir(), "my.tdf")
	if !a.writeOut(path, "tdf") || a.dirty || a.path != path {
		t.Fatalf("save: %s", a.message)
	}
	b, _ := os.ReadFile(path)
	f := tdfFParse(t, b)
	if f.name != "My font" || f.kind != tdfColour || f.gap() != 1 || f.has('C') || !f.has('A') || !f.has('B') {
		t.Fatalf("read back %+v", f)
	}
	gb := f.glyph('B')
	if gb.width() != 5 || len(gb.rows) != 3 || !gb.desc[2] || gb.h != 2 || gb.rows[2][4].ch != 'B' || gb.rows[2][4].attr != 0x0C {
		t.Fatalf("B: %+v", gb)
	}
	if !strings.Contains(a.message, "2 letters") {
		t.Errorf("message %q", a.message)
	}
	// Saved again unchanged: the same bytes.
	a.save(false)
	if again, _ := os.ReadFile(path); !bytes.Equal(again, b) {
		t.Fatal("saved again differently")
	}
	// Opened from the library: in place (a file of one font), each letter as drawn.
	f.file = path
	a.fedOpenFont(f)
	if a.path != path || a.fed() == ff || a.dirty {
		t.Fatalf("opened: path %q dirty %v", a.path, a.dirty)
	}
	if cellAttr(a.doc.get(1, 1)) != 0x4F || cellCh(a.doc.get(1, 1)) != 'x' {
		t.Fatalf("A as opened: %x", a.doc.get(1, 1))
	}
	a.save(false)
	if again, _ := os.ReadFile(path); !bytes.Equal(again, b) {
		t.Fatal("opened and saved: different bytes")
	}
}

// Delete, Copy to, the name and spacing fields, a font from a file of several fonts.
func TestFedEditing(t *testing.T) {
	testApp(t, "pc")
	tdfFClean(t)
	fedFNew(t, tdfBlock, "0")
	ff := a.fed()
	fedFDraw(map[[2]int]uint32{{0, 0}: attrCell('A', 7)})
	ff.copying = true
	a.fedGo('a' - tdfFirst)
	if fedLetter(ff.cur) != "a" || !ff.slots[ff.cur].defined || cellCh(a.doc.get(0, 0)) != 'A' {
		t.Fatalf("copy: %s", a.message)
	}
	ff.copying = true
	a.fedGo('A' - tdfFirst) // A is in the font: refused
	if fedLetter(ff.cur) != "a" || !strings.Contains(a.message, "delete it first") {
		t.Fatalf("copy over: %s", a.message)
	}
	a.fedDelete()
	if ff.slots[ff.cur].defined || !docBlank(a.doc) {
		t.Fatal("delete")
	}
	a.doUndo()
	a.fedGo('A' - tdfFirst)
	if !ff.slots['a'-tdfFirst].defined {
		t.Fatal("undoing the delete did not bring a back")
	}
	a.fedGo('a' - tdfFirst)
	a.fedDelete()
	a.fedGo('A' - tdfFirst)
	fieldFedName.set("Renamed font long")
	fieldFedName.enter()
	fieldFedSpacing.set("3")
	fieldFedSpacing.enter()
	path := filepath.Join(t.TempDir(), "x.tdf")
	a.writeOut(path, "tdf")
	b, _ := os.ReadFile(path)
	f := tdfFParse(t, b)
	if f.name != "Renamed font" || f.gap() != 3 || f.has('a') || !f.has('A') {
		t.Fatalf("%q gap %d a %v", f.name, f.gap(), f.has('a'))
	}
	// Not on a PC page any more: refused.
	a.mode = "c64"
	a.fedGo(5)
	if !strings.Contains(a.message, "PC page") {
		t.Fatalf("other page: %s", a.message)
	}
	if _, err := a.encode("tdf"); err == nil {
		t.Fatal("encoded off a PC page")
	}
}

// A font of a pack of three edited and saved: back into the pack, the other two
// byte for byte in their order, no 0 at the end (TDFONTS refuses one); from a
// .zip or a file CharEdit may not write: a file of its own.
func TestFedPack(t *testing.T) {
	testApp(t, "pc")
	tdfFClean(t)
	one, two, three := fedFFontBytes(t, "One"), fedFFontBytes(t, "Two"), fedFFontBytes(t, "Three")
	pack := append(tdfFileBytes(one, two, three), 0) // as IcyDraw writes it: the 0 goes
	path := filepath.Join(t.TempDir(), "pack.tdf")
	os.WriteFile(path, pack, 0o644)
	fs, _ := parseTDF(pack, path)
	a.fedOpenFont(fs[1])
	ff := a.fed()
	if a.path != path || ff.place != "font 2 of 3" || !strings.Contains(a.message, "back into pack.tdf") {
		t.Fatalf("path %q place %q: %s", a.path, ff.place, a.message)
	}
	fedFDraw(map[[2]int]uint32{{0, 0}: attrCell('Z', 7)})
	fieldFedName.set("Two edited")
	fieldFedName.enter()
	a.save(false)
	if a.dirty || !strings.Contains(a.message, "font 2 of 3") {
		t.Fatalf("save: %s", a.message)
	}
	b, _ := os.ReadFile(path)
	got, err := parseTDF(b, path)
	if err != nil || len(got) != 3 || got[1].name != "Two edited" || got[1].glyph('A').rows[0][0].ch != 'Z' {
		t.Fatalf("read back: %v %d", err, len(got))
	}
	if !bytes.Equal(b[got[0].off:got[0].end], one) || !bytes.Equal(b[got[2].off:got[2].end], three) || got[0].off != 20 {
		t.Fatal("the other fonts changed")
	}
	if got[2].end != len(b) {
		t.Fatalf("%d bytes after the last font", len(b)-got[2].end)
	}
	// Saved again unchanged: the same bytes.
	a.save(false)
	if again, _ := os.ReadFile(path); !bytes.Equal(again, b) {
		t.Fatal("saved again differently")
	}
	// An autosave of it comes back into the pack.
	if err := writeAutosave(a.Tab); err != nil {
		t.Fatal(err)
	}
	id := a.autoID
	js, _ := os.ReadFile(filepath.Join(autosaveDir(), id+".json"))
	var m autosaveMeta
	json.Unmarshal(js, &m)
	a.focusTab(a.newTab("pc"))
	if err := a.restoreAutosave(autosaveFile{id, m}); err != nil || a.fed() == nil || a.fed().place != "font 2 of 3" || a.path != path {
		t.Fatalf("restored: %v", err)
	}
	a.save(false)
	if again, _ := os.ReadFile(path); !bytes.Equal(again, b) {
		t.Fatal("restored and saved: different bytes")
	}
	// A read-only pack: a file of its own.
	os.Chmod(path, 0o444)
	fs, _ = parseTDF(b, path)
	a.fedOpenFont(fs[2])
	if os.Getuid() != 0 && (a.path != "" || a.name != "Three.tdf" || a.fed().before != nil) {
		t.Fatalf("read-only: path %q name %q", a.path, a.name)
	}
	// From a .zip: a file of its own.
	fs[0].file = filepath.Join(t.TempDir(), "fonts.zip")
	a.fedOpenFont(fs[0])
	if a.path != "" || a.name != "One.tdf" {
		t.Fatalf("zip: path %q name %q", a.path, a.name)
	}
}

// Two tabs edit fonts 1 and 3 of one pack: saved in either order, both edits are in
// the file. A pack changed under a tab (its font gone from its place) is left alone:
// the font goes to a file of its own.
func TestFedPackTwoTabs(t *testing.T) {
	for _, order := range [][2]int{{0, 1}, {1, 0}} {
		testApp(t, "pc")
		tdfFClean(t)
		path := filepath.Join(t.TempDir(), "pack.tdf")
		os.WriteFile(path, tdfFileBytes(fedFFontBytes(t, "One"), fedFFontBytes(t, "Two"), fedFFontBytes(t, "Three")), 0o644)
		b, _ := os.ReadFile(path)
		fs, _ := parseTDF(b, path)
		var tabs [2]*Tab
		for k, i := range []int{0, 2} {
			a.fedOpenFont(fs[i])
			tabs[k] = a.Tab
			fedFDraw(map[[2]int]uint32{{0, 0}: attrCell(byte('X'+k), 7)})
			fieldFedName.set(fs[i].name + " ed")
			fieldFedName.enter()
		}
		for _, k := range order {
			a.focusTab(tabs[k])
			a.save(false)
			if a.dirty || a.path != path {
				t.Fatalf("tab %d: %s", k, a.message)
			}
		}
		b, _ = os.ReadFile(path)
		got, err := parseTDF(b, path)
		if err != nil || len(got) != 3 || got[0].name != "One ed" || got[1].name != "Two" || got[2].name != "Three ed" ||
			got[0].glyph('A').rows[0][0].ch != 'X' || got[2].glyph('A').rows[0][0].ch != 'Y' || got[2].end != len(b) {
			t.Fatalf("order %v: %v %+v", order, err, got)
		}
		// Saved again (after the other tab's save): still both.
		a.focusTab(tabs[order[0]])
		a.pushUndo()
		a.save(false)
		if again, _ := os.ReadFile(path); !bytes.Equal(again, b) {
			t.Fatalf("order %v: saved again differently", order)
		}
	}
	// The pack changed: the font at place 3 is gone.
	path := filepath.Join(t.TempDir(), "pack.tdf")
	os.WriteFile(path, tdfFileBytes(fedFFontBytes(t, "One"), fedFFontBytes(t, "Two"), fedFFontBytes(t, "Three")), 0o644)
	b, _ := os.ReadFile(path)
	fs, _ := parseTDF(b, path)
	a.fedOpenFont(fs[2])
	fedFDraw(map[[2]int]uint32{{0, 0}: attrCell('Q', 7)})
	two := tdfFileBytes(fedFFontBytes(t, "One"), fedFFontBytes(t, "Two"))
	os.WriteFile(path, two, 0o644)
	a.save(false)
	if after, _ := os.ReadFile(path); !bytes.Equal(after, two) {
		t.Fatal("the changed pack was written over")
	}
	own := filepath.Join(filepath.Dir(path), "Three.tdf")
	ob, err := os.ReadFile(own)
	if err != nil || a.path != own || a.dirty || !strings.Contains(a.message, "saved as Three.tdf") {
		t.Fatalf("own file: %v %q %s", err, a.path, a.message)
	}
	if f := tdfFParse(t, ob); f.name != "Three" || f.glyph('A').rows[0][0].ch != 'Q' {
		t.Fatal("own file's font")
	}
}

func fedFFontBytes(t *testing.T, name string) []byte {
	var glyphs [tdfChars][]byte
	glyphs['A'-tdfFirst], _, _ = encodeTDFGlyph(tdfBlock, newDoc(2, 2), nil)
	b, err := encodeTDFFont(name, tdfBlock, 1, &glyphs)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// An autosave of a font tab comes back as the font, the letter on the page as it was.
func TestFedAutosave(t *testing.T) {
	testApp(t, "pc")
	tdfFClean(t)
	fedFNew(t, tdfBlock, "1")
	fedFDraw(map[[2]int]uint32{{0, 0}: attrCell('A', 7)})
	a.fedGo('B' - tdfFirst)
	fedFDraw(map[[2]int]uint32{{1, 1}: attrCell('B', 7)})
	if err := writeAutosave(a.Tab); err != nil {
		t.Fatal(err)
	}
	id := a.autoID
	js, _ := os.ReadFile(filepath.Join(autosaveDir(), id+".json"))
	var m autosaveMeta
	if err := json.Unmarshal(js, &m); err != nil {
		t.Fatal(err)
	}
	a.focusTab(a.newTab("pc"))
	if err := a.restoreAutosave(autosaveFile{id, m}); err != nil {
		t.Fatal(err)
	}
	ff := a.fed()
	if ff == nil || fedLetter(ff.cur) != "B" || cellCh(a.doc.get(1, 1)) != 'B' || !a.dirty || a.saveFmt != "tdf" {
		t.Fatalf("restored: %v %s", ff != nil, a.message)
	}
	a.fedGo('A' - tdfFirst)
	if cellCh(a.doc.get(0, 0)) != 'A' {
		t.Fatal("A lost")
	}
	if _, err := os.Stat(filepath.Join(autosaveDir(), id+".tdfed")); err == nil {
		t.Fatal("the old autosave's font is left")
	}
}

// The letter strip draws, takes clicks, and the Edit button of the fonts list opens the editor.
func TestFedStripAndPicker(t *testing.T) {
	testApp(t, "pc")
	fs := tdfFLib(t, 0)
	ui.fb = &FB{}
	ui.fb.resize(1400, 900)
	ui.fb.clip = Rect{0, 0, 1400, 900}
	openFedEdit()
	d, ok := a.dialog.(*TDFReq)
	if !ok || !d.editing {
		t.Fatal("no fonts list for editing")
	}
	d.draw(ui)
	d.sel = 0
	d.use() // Return: Edit
	ff := a.fed()
	if ff == nil || ff.name != fs[0].name || a.dialog != nil {
		t.Fatalf("not editing: %s", a.message)
	}
	r := Rect{0, 600, 1000, fedStripH()}
	a.drawFedStrip(ui, r)
	if ff.preview.fb == nil {
		t.Error("no preview")
	}
	// A click on the second row's first box: 'P'.
	boxW := max(textW("W"), textW("@")) + 6
	ui.click = &Click{x: 6 + boxW/2, y: 600 + 3 + gadH + 3 + 2, button: 1}
	a.drawFedStrip(ui, r)
	if fedLetter(ff.cur) != "P" {
		t.Fatalf("clicked to %s", fedLetter(ff.cur))
	}
	_ = boxW
	a.drawFedStrip(ui, Rect{0, 600, 300, fedStripH()}) // narrow
}

// TheDraw's SHADING.TD opened in CharEdit, saved with Ctrl+S as a .td again: the
// same three page layers, read back as frames.
func TestFedTDOpenSave(t *testing.T) {
	dir := os.Getenv("CHARCOAL_THEDRAW")
	if dir == "" {
		h, _ := os.UserHomeDir()
		dir = filepath.Join(h, ".cache/charcoal-others/thedraw/td463")
	}
	b, err := os.ReadFile(filepath.Join(dir, "SHADING.TD"))
	if err != nil {
		t.Skip("no TheDraw files in", dir)
	}
	testApp(t, "pc")
	path := filepath.Join(t.TempDir(), "SHADING.TD")
	os.WriteFile(path, b, 0o644)
	a.openPath(path, "auto")
	if a.nFrames() != 3 || a.saveFmt != "td" || a.path != path {
		t.Fatalf("%d frames, %s, %q", a.nFrames(), a.saveFmt, a.path)
	}
	want, _ := parseTD(b)
	a.goFrame(1)
	a.pushUndo()
	a.doc.set(0, 0, attrCell('X', 0x1F))
	want[1].set(0, 0, attrCell('X', 0x1F))
	a.save(false)
	if a.dirty || !strings.Contains(a.message, "80x25 TheDraw page") {
		t.Fatalf("save: %s", a.message)
	}
	out, _ := os.ReadFile(path)
	got, err := parseTD(out)
	if err != nil || len(got) != 3 {
		t.Fatal(err)
	}
	for i := range want {
		fedFSame(t, "SHADING.TD", got[i], want[i])
	}
	if out[31] != 2 { // the page layer shown: the one being worked on
		t.Errorf("page %d", out[31])
	}
}
