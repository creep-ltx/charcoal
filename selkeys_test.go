package main

import "testing"

func TestShiftArrowsSelect(t *testing.T) {
	a := testApp(t, "pc")
	a.tool = "draw" // the Select tool need not be on
	a.cx, a.cy = 2, 3
	editorKey("Right", false, true, false)
	editorKey("Right", false, true, false)
	editorKey("Down", false, true, false)
	if a.sel == nil || a.selRect() != (Rect{2, 3, 3, 2}) {
		t.Fatalf("selection %+v", a.sel)
	}
	editorKey("Home", false, true, false)
	if r := a.selRect(); r != (Rect{0, 3, 3, 2}) {
		t.Errorf("Shift+Home: %+v", r)
	}
	editorKey("Escape", false, false, false)
	if a.sel != nil {
		t.Error("Esc did not let go")
	}
	// After a plain move a new selection starts at the cursor.
	editorKey("Right", false, true, false)
	editorKey("Left", false, false, false)
	editorKey("Left", false, false, false)
	editorKey("Up", false, true, false)
	if r := a.selRect(); r != (Rect{a.cx, a.cy, 1, 2}) {
		t.Errorf("new selection %+v, cursor %d,%d", r, a.cx, a.cy)
	}
}
