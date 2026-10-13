package main

import (
	"os"
	"path/filepath"
	"testing"
)

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
