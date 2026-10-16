package main

// Tests of TheDraw .td writing and the TheDraw font editor (fed.go). Helpers are
// prefixed fedF.

import (
	"bytes"
	"math/rand"
	"os"
	"path/filepath"
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
