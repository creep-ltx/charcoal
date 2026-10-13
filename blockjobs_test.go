package main

import "testing"

func TestFillSelection(t *testing.T) {
	a := testApp(t, "pc")
	a.brush, a.fg, a.bg = 0xB1, 4, 1
	a.sel = &Sel{3, 2, 1, 1}
	editorKey("F", false, false, true) // Alt+F
	want := makeCell(0xB1, 4, 1, 0)
	for y := 0; y < 4; y++ {
		for x := 0; x < 5; x++ {
			in := x >= 1 && x <= 3 && y >= 1 && y <= 2
			if got := a.doc.get(x, y); (got == want) != in {
				t.Errorf("cell %d,%d = %x (inside %v)", x, y, got, in)
			}
		}
	}
	if len(a.undo) != 1 {
		t.Errorf("%d undo steps", len(a.undo))
	}
	a.sel = nil
	editorKey("F", false, false, true)
	if a.tool != "fill" {
		t.Errorf("Alt+F with nothing selected: tool %s", a.tool)
	}
}

func TestMoveSelection(t *testing.T) {
	a := testApp(t, "pc")
	put(a, 2, 1, "ab")
	put(a, 2, 2, "cd")
	a.sel = &Sel{2, 1, 3, 2}
	a.liftSel(3, 2) // held at its bottom right cell
	if cellCh(a.doc.get(2, 1)) != ' ' || a.float == nil {
		t.Fatal("the block was not lifted")
	}
	a.hover = &Hover{x: 10, y: 5}
	a.dropMoved()
	if rowText(a, 4)[9:11] != "ab" || rowText(a, 5)[9:11] != "cd" {
		t.Errorf("moved block: %q / %q", rowText(a, 4)[8:12], rowText(a, 5)[8:12])
	}
	if a.selRect() != (Rect{9, 4, 2, 2}) {
		t.Errorf("selection after the move %+v", a.selRect())
	}
	if len(a.undo) != 1 {
		t.Errorf("a move took %d undo steps", len(a.undo))
	}
	a.doUndo()
	if rowText(a, 1)[2:4] != "ab" || rowText(a, 4)[9:11] != "  " {
		t.Error("undo did not put it back")
	}
	// Esc while moving puts it back.
	a.sel = &Sel{2, 1, 3, 2}
	a.liftSel(2, 1)
	editorKey("Escape", false, false, false)
	if a.float != nil || rowText(a, 1)[2:4] != "ab" {
		t.Error("Esc did not put the block back")
	}
	// A drop near the edge stays on the page.
	a.sel = &Sel{2, 1, 3, 2}
	a.liftSel(2, 1)
	a.hover = &Hover{x: 79, y: 0}
	a.dropMoved()
	if rowText(a, 0)[78:80] != "ab" {
		t.Errorf("drop at the edge: %q", rowText(a, 0)[76:80])
	}
}

func TestCrop(t *testing.T) {
	a := testApp(t, "pc")
	put(a, 5, 3, "xyz")
	a.sel = &Sel{5, 3, 14, 4}
	editorKey("K", true, false, false)
	if a.doc.cols != 10 || a.doc.rows != 2 || rowText(a, 0)[:3] != "xyz" {
		t.Errorf("crop: %dx%d %q", a.doc.cols, a.doc.rows, rowText(a, 0))
	}
	a.doUndo()
	if a.doc.cols != 80 || cellCh(a.doc.get(5, 3)) != 'x' {
		t.Error("undo did not bring the page back")
	}
	b := testApp(t, "c64")
	b.sel = &Sel{0, 0, 3, 3}
	b.cropToSel()
	if b.doc.cols != 40 || b.message == "" {
		t.Errorf("C64 crop: %d cols, message %q", b.doc.cols, b.message)
	}
}

func TestFlipBrush(t *testing.T) {
	a := testApp(t, "pc")
	a.brush = '/'
	editorKey("H", false, false, true)
	if a.brush != '\\' {
		t.Errorf("Alt+H on the brush: %c", a.brush)
	}
	a.brush = 0xDF // upper half block
	editorKey("V", false, false, true)
	if a.brush != 0xDC {
		t.Errorf("Alt+V on the brush: %x", a.brush)
	}
}

func TestToolKeys(t *testing.T) {
	a := testApp(t, "amiga")
	for k, want := range map[string]string{"T": "type", "D": "draw", "K": "colour", "L": "fill", "P": "pick", "S": "select"} {
		editorKey(k, false, false, true)
		if a.tool != want {
			t.Errorf("Alt+%s: tool %s, want %s", k, a.tool, want)
		}
	}
	editorKey("B", false, false, true)
	if a.tool == "half" {
		t.Error("Alt+B gave the Half tool on an Amiga page")
	}
	a = testApp(t, "pc")
	editorKey("B", false, false, true)
	if a.tool != "half" {
		t.Errorf("Alt+B on a PC page: %s", a.tool)
	}
}
