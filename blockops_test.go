package main

import "testing"

// Flipping twice gives the character back, in every font; turning four times too
// in the PC font (whose turns are all known pairs and cycles).
func TestMirrorTablesUndo(t *testing.T) {
	loadC64Fonts()
	for _, f := range fonts {
		if f.id == "c64lower" || f.id == "vic20lower" || f.id == "petlower" || f.id == "c128lower" {
			continue // Marq's lower-case tables have three one-way entries; kept as he made them
		}
		for _, k := range []int{flipH, flipV} {
			for c := 0; c < 256; c++ {
				if got := f.mapped(f.mapped(c, k), k); got != c {
					t.Errorf("%s: %d flipped twice (%d) gives %d", f.id, c, k, got)
				}
			}
		}
	}
	vga := fontByID("vga")
	for c := 0; c < 256; c++ {
		if got := vga.turned(vga.turned(vga.turned(vga.turned(c)))); got != c {
			t.Errorf("vga: %d turned four times gives %d", c, got)
		}
	}
	for _, p := range [][2]int{{0xDA, 0xBF}, {0xC3, 0xC2}, {0xDF, 0xDE}, {0xC4, 0xB3}, {'-', '|'}, {'^', '>'}, {'A', 'A'}, {0xB1, 0xB1}} {
		if got := vga.turned(p[0]); got != p[1] {
			t.Errorf("vga: %#x turns into %#x, want %#x", p[0], got, p[1])
		}
	}
	for _, p := range [][2]int{{0xB4, 0xB4}, {0xB3, 0xB3}, {0xDC, 0xDF}, {'C', 'C'}, {'B', 'B'}} {
		if got := vga.flipped(p[0], true); got != p[1] {
			t.Errorf("vga: %#x upside down is %#x, want %#x", p[0], got, p[1])
		}
	}
	topaz := fontByID("topaz1p")
	for _, c := range "BCGNO0123456789" {
		if got := topaz.flipped(int(c), false); got != int(c) {
			t.Errorf("topaz: %q mirrored is %q: letters and digits only change into exact mirror images", c, rune(got))
		}
	}
}

func TestBlockOps(t *testing.T) {
	a := testApp(t, "pc")
	a.ice = false
	// A 3 wide, 2 high block: ┌─A / │▀B in colours.
	b := &Block{w: 3, h: 2, cells: []uint32{
		makeCell(0xDA, 1, 2, 0), makeCell(0xC4, 3, 0, 0), makeCell('A', 4, 0, 0),
		makeCell(0xB3, 5, 0, 0), makeCell(0xDF, 6, 0, 0), makeCell('B', 15, 1, 0),
	}}
	tb := a.transformBlock(b, turnCW)
	if tb.w != 2 || tb.h != 3 {
		t.Fatalf("turned 3x2 is %dx%d", tb.w, tb.h)
	}
	// Turning clockwise: the bottom-left cell goes to the top-left.
	want := []int{0xC4, 0xBF, 0xDE, 0xB3, 'B', 'A'}
	for i, c := range want {
		if got := cellCh(tb.cells[i]); got != c {
			t.Errorf("turned cell %d is %#x, want %#x", i, got, c)
		}
	}
	if cellFg(tb.cells[0]) != 5 {
		t.Errorf("colours do not move with their characters")
	}
	for _, op := range []int{flipH, flipV} {
		back := a.transformBlock(a.transformBlock(b, op), op)
		for i := range b.cells {
			if back.cells[i] != b.cells[i] {
				t.Errorf("op %d twice changed cell %d", op, i)
			}
		}
	}
	inv := a.transformBlock(b, invertBlock)
	if v := inv.cells[0]; cellFg(v) != 2 || cellBg(v) != 1 {
		t.Errorf("invert gave fg %d bg %d", cellFg(v), cellBg(v))
	}
	if v := inv.cells[5]; cellFg(v) != 1 || cellBg(v) != 7 { // no iCE: a bright foreground becomes a dark background
		t.Errorf("invert without iCE gave fg %d bg %d", cellFg(v), cellBg(v))
	}

	// A selection that turns into another shape is lifted to be put down.
	a.doc.set(0, 0, makeCell('x', 7, 0, 0))
	a.sel = &Sel{0, 0, 2, 0}
	a.blockOp(turnCW)
	if a.float == nil || a.float.w != 1 || a.float.h != 3 || cellCh(a.doc.get(0, 0)) != 32 || !a.dirty {
		t.Errorf("a turned 3x1 selection should float as 1x3 and leave blanks")
	}
	a.blockOp(flipV) // the floating block turns over in place
	if cellCh(a.float.cells[2]) != 'x' {
		t.Errorf("flipping the floating block did not move its cells")
	}

	c := testApp(t, "atari")
	ib := c.transformBlock(&Block{w: 1, h: 1, cells: []uint32{makeCell('A', 0, 0, 0)}}, invertBlock)
	if cellCh(ib.cells[0]) != 'A'|0x80 {
		t.Errorf("invert on an Atari page should reverse the character")
	}
}

func TestTeletextBlockOps(t *testing.T) {
	for c := 0x20; c < 0x80; c++ {
		if c >= 0x40 && c < 0x60 {
			continue
		}
		for _, v := range []bool{false, true} {
			if got := ttSixelFlip(ttSixelFlip(c, v), v); got != c {
				t.Errorf("sixel %#x flipped twice is %#x", c, got)
			}
		}
	}
	if got := ttSixelFlip(0x21, false); got != 0x22 { // top-left becomes top-right
		t.Errorf("top-left sixel mirrored is %#x", got)
	}
	if got := ttSixelFlip(0x21, true); got != 0x30 { // top-left becomes bottom-left
		t.Errorf("top-left sixel upside down is %#x", got)
	}
	codes := &Block{w: 2, h: 1, cells: []uint32{makeCell(0x17, 0, 0, 0), makeCell(0x21, 0, 0, 0)}}
	if ttBlockOpRefused(codes, flipH) == "" || ttBlockOpRefused(codes, flipV) != "" || ttBlockOpRefused(codes, turnCW) == "" {
		t.Errorf("teletext: codes block left-right flips and turns, but not upside down")
	}
}
