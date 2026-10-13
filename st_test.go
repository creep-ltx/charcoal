package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TOS throws away most control codes, takes 11 and 12 as line feeds, and keeps
// ESC Y on the screen (EmuTOS bios/vt52.c; audit #19).
func TestVT52Controls(t *testing.T) {
	r := stResByID("stlow")
	d := parseVT52([]byte("A\x01\x02\x05\x06B\x0e\x1a\x1c\x1fC\x0bD\x0cE\x00\x7fF"), r, 0)
	for _, p := range []struct {
		x, y int
		c    int
	}{{0, 0, 'A'}, {1, 0, 'B'}, {2, 0, 'C'}, {3, 1, 'D'}, {4, 2, 'E'}, {5, 2, 0x7F}, {6, 2, 'F'}} {
		if got := cellCh(d.get(p.x, p.y)); got != p.c {
			t.Fatalf("%d,%d: %x, want %x", p.x, p.y, got, p.c)
		}
	}
	// ESC Y past the bottom: the last row of the screen, not row 223.
	d = parseVT52([]byte("\x1bY\xff\x25X"), r, 0)
	if d.rows != 25 || cellCh(d.get(5, 24)) != 'X' {
		t.Fatalf("ESC Y: %d rows", d.rows)
	}
}

// Saved .vt files end without CR LF (it would scroll the ST's screen), yet every
// row comes back, empty ones at the end included (audit #10); glyphs an ST cannot
// print become spaces without moving the rest of the row.
func TestVT52Height(t *testing.T) {
	r := stResByID("stmed")
	for _, tc := range []struct {
		rows   int
		filled []int
		full   int
	}{
		{3, []int{0}, -1}, {5, []int{0, 2}, -1}, {4, []int{3}, -1}, {25, []int{0, 24}, -1},
		{2, []int{}, -1}, {1, []int{}, -1}, {3, []int{0}, 2}, {3, []int{0}, 1},
	} {
		d := newDoc(80, tc.rows)
		t0 := newSTTerm(d, r)
		fillCells(d.cells, t0.blankCell())
		for _, y := range tc.filled {
			d.set(0, y, makeCell('A'+y, 1, 0, 0))
			d.set(9, y, makeCell(0xDF, 2, 0, 0))
		}
		if tc.full >= 0 {
			for x := 0; x < 80; x++ {
				d.set(x, tc.full, makeCell('#', 3, 0, 0))
			}
		}
		out := writeVT52(d, r)
		if strings.HasSuffix(string(out), "\n") {
			t.Fatalf("%+v: ends with a line feed", tc)
		}
		back := parseVT52(out, r, 0)
		if back.rows != tc.rows {
			t.Fatalf("%+v: %d rows came back: %q", tc, back.rows, out)
		}
		for i := range d.cells[:80*tc.rows] {
			if cellCh(back.cells[i]) != cellCh(d.cells[i]) {
				t.Fatalf("%+v: cell %d: %x != %x", tc, i, back.cells[i], d.cells[i])
			}
		}
	}
	d := newDoc(80, 1)
	d.set(0, 0, makeCell('A', 3, 0, 0))
	d.set(1, 0, makeCell(0x01, 3, 0, 0)) // an arrow glyph: TOS would throw it away
	d.set(2, 0, makeCell('B', 3, 0, 0))
	back := parseVT52(writeVT52(d, r), r, 0)
	if cellCh(back.get(1, 0)) != ' ' || cellCh(back.get(2, 0)) != 'B' || stUnprintable(d) != 1 {
		t.Fatalf("unprintable glyph: %x %x", back.get(1, 0), back.get(2, 0))
	}
	for _, set := range stFkeys {
		for _, c := range set {
			if c < 0x20 {
				t.Fatalf("F-key %x cannot be printed on an ST", c)
			}
		}
	}
}

// An EmuTOS font download shows the real error, and a right copy on disk is not fetched again.
func TestDownloadSTFonts(t *testing.T) {
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		http.NotFound(w, r)
	}))
	defer srv.Close()
	old := stFontURL
	stFontURL = func(file string) string { return srv.URL + "/" + file }
	defer func() { stFontURL = old }()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	err := downloadSTFonts()
	if err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("error: %v", err)
	}
	src := "/tmp/am/st/pin_fnt_st_8x8.c"
	b, err := os.ReadFile(src)
	if err != nil {
		t.Skip("no EmuTOS font here")
	}
	os.MkdirAll(filepath.Dir(stFontPath("x")), 0o755)
	os.WriteFile(stFontPath("fnt_st_8x8.c"), b, 0o644)
	hits = 0
	downloadSTFonts()
	if hits != 1 { // only the 8x16 font was asked for
		t.Fatalf("%d requests", hits)
	}
}

func TestVT52(t *testing.T) {
	r := stResByID("stlow")
	// Clear, red on white, position row 2 col 5, reverse, text.
	b := []byte("\x1bE\x1bb1Hi\x1bY\"%X\x1bpR\x1bq")
	d := parseVT52(b, r, 0)
	if d.cols != 40 || cellCh(d.get(0, 0)) != 'H' || cellFg(d.get(0, 0)) != 1 || cellCh(d.get(5, 2)) != 'X' {
		t.Fatalf("parse: %x %x", d.get(0, 0), d.get(5, 2))
	}
	if v := d.get(6, 2); cellCh(v) != 'R' || cellFg(v) != 0 || cellBg(v) != 1 { // reversed: red background
		t.Fatalf("reverse: %x", v)
	}
	back := parseVT52(writeVT52(d, r), r, 0)
	for y := 0; y < 3; y++ {
		for x := 0; x < 40; x++ {
			a, c := d.get(x, y), back.get(x, y)
			if a&255 == 32 && c&255 == 32 && cellBg(a) == cellBg(c) {
				continue
			}
			if a != c {
				t.Fatalf("round trip %d,%d: %x became %x", x, y, a, c)
			}
		}
	}
	if !isVT52(b) || isVT52([]byte("\x1b[1;31mred\x1bb")) {
		t.Fatal("sniffing")
	}
}

func TestSTFont(t *testing.T) {
	src := "/tmp/am/st/pin_fnt_st_8x16.c"
	if _, err := os.Stat(src); err != nil {
		t.Skip("no EmuTOS font here")
	}
	dir := t.TempDir()
	os.Setenv("XDG_DATA_HOME", dir)
	defer os.Unsetenv("XDG_DATA_HOME")
	os.MkdirAll(filepath.Join(dir, "charcoal"), 0o755)
	for _, f := range []string{"fnt_st_8x8.c", "fnt_st_8x16.c"} {
		b, _ := os.ReadFile("/tmp/am/st/pin_" + f)
		os.WriteFile(stFontPath(f), b, 0o644)
	}
	if !loadSTFonts() {
		t.Fatal("fonts not read")
	}
	f := fontByID("st8x16")
	ink := 0
	for r := 0; r < 16; r++ {
		for v := f.bits['A'*16+r]; v != 0; v &= v - 1 {
			ink++
		}
	}
	if ink < 12 || f.bits[' '*16+8] != 0 {
		t.Fatalf("'A' has %d pixels", ink)
	}
}
