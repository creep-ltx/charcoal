package main

import (
	"os"
	"strings"
	"testing"
)

// Marq's .c files name their machine; a PET's hold the characters only.
func TestMarqMachines(t *testing.T) {
	for _, c := range []struct {
		file, machine string
		w, h          int
	}{{"testdata/marq/pet-ukkeli.c", "pet", 40, 25}, {"testdata/marq/vic20-iso.c", "vic20", 22, 23}} {
		b, err := os.ReadFile(c.file)
		if err != nil {
			t.Fatal(err)
		}
		scr, err := parseMarq(b)
		if err != nil || len(scr) != 1 {
			t.Fatalf("%s: %v", c.file, err)
		}
		d := scr[0].doc
		if scr[0].machine != c.machine || d.cols != c.w || d.rows != c.h {
			t.Errorf("%s: machine %q %dx%d", c.file, scr[0].machine, d.cols, d.rows)
		}
		ink := 0
		for _, v := range d.cells {
			if cellCh(v) != 32 {
				ink++
			}
		}
		if ink < 50 {
			t.Errorf("%s: only %d drawn cells", c.file, ink)
		}
		// Saved again for the same machine, it reads back the same.
		out := writeMarq(d, false, c.machine)
		if c.machine == "pet" && strings.Contains(string(out), "border,bg") {
			t.Errorf("a PET picture should be saved without colours")
		}
		back, err := parseMarq(out)
		if err != nil || back[0].machine != c.machine {
			t.Fatalf("%s: read back %v", c.file, err)
		}
		for i := range d.cells {
			if cellCh(back[0].doc.cells[i]) != cellCh(d.cells[i]) {
				t.Fatalf("%s: cell %d changed", c.file, i)
			}
		}
	}
}
