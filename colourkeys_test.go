package main

import "testing"

func TestColourKeys(t *testing.T) {
	a := testApp(t, "pc")
	editorKey("Down", true, false, false) // Ctrl+Down: foreground +1
	if a.fg != 8 {
		t.Errorf("Ctrl+Down: fg %d, want 8", a.fg)
	}
	editorKey("Up", true, false, false)
	editorKey("Up", true, false, false)
	if a.fg != 6 {
		t.Errorf("Ctrl+Up twice: fg %d, want 6", a.fg)
	}
	editorKey("Left", true, false, false) // background -1 wraps
	if a.bg != 15 {
		t.Errorf("Ctrl+Left: bg %d, want 15", a.bg)
	}
	editorKey("3", true, false, false)
	if a.fg != 3 {
		t.Errorf("Ctrl+3: fg %d", a.fg)
	}
	editorKey("3", true, false, false)
	if a.fg != 11 {
		t.Errorf("Ctrl+3 again: fg %d, want the bright 11", a.fg)
	}
	editorKey("3", true, false, false)
	if a.fg != 3 {
		t.Errorf("Ctrl+3 a third time: fg %d, want 3", a.fg)
	}
	editorKey("4", false, false, true) // Alt+4
	if a.bg != 4 {
		t.Errorf("Alt+4: bg %d", a.bg)
	}
	editorKey("X", true, true, false) // Ctrl+Shift+X swaps
	if a.fg != 4 || a.bg != 3 {
		t.Errorf("swap: %d on %d", a.fg, a.bg)
	}
	editorKey("D", true, false, false)
	if a.fg != 7 || a.bg != 0 {
		t.Errorf("Ctrl+D: %d on %d", a.fg, a.bg)
	}
	a.doc.set(2, 1, makeCell('Q', 12, 5, 0))
	a.cx, a.cy = 2, 1
	editorKey("U", false, false, true) // Alt+U takes the cell
	if a.fg != 12 || a.bg != 5 || a.brush != 'Q' {
		t.Errorf("Alt+U: %d on %d, brush %c", a.fg, a.bg, a.brush)
	}
	editorKey("E", true, false, false)
	if !a.ice {
		t.Error("Ctrl+E did not switch iCE on")
	}
}

func TestFkeySetKeys(t *testing.T) {
	a := testApp(t, "pc")
	editorKey("F3", true, false, false)
	if a.fset != 2 {
		t.Errorf("Ctrl+F3: set %d", a.fset)
	}
	editorKey("Up", true, true, false)
	if a.fset != 3 {
		t.Errorf("Ctrl+Shift+Up: set %d", a.fset)
	}
	editorKey("Down", true, true, false)
	editorKey("Down", true, true, false)
	if a.fset != 1 {
		t.Errorf("Ctrl+Shift+Down twice: set %d", a.fset)
	}
	editorKey("F10", true, false, false) // PC pages have five sets
	if a.fset != 1 || a.message == "" {
		t.Errorf("Ctrl+F10 on five sets: set %d, message %q", a.fset, a.message)
	}
}

func TestColourKeysRespectPages(t *testing.T) {
	a := testApp(t, "st")
	a.setSTRes("stmed") // 4 colours
	a.fg = 3
	editorKey("Down", true, false, false)
	if a.fg != 0 {
		t.Errorf("ST medium: fg +1 from 3 gave %d, want 0", a.fg)
	}
	editorKey("2", true, false, false)
	editorKey("2", true, false, false) // no bright twin among 4 colours
	if a.fg != 2 {
		t.Errorf("ST medium: Ctrl+2 twice gave %d", a.fg)
	}
	editorKey("5", true, false, false)
	if a.fg != 2 {
		t.Errorf("ST medium: Ctrl+5 gave %d", a.fg)
	}

	a = testApp(t, "atari")
	editorKey("3", true, false, false) // luminance 6, the register's hue kept (gtia.go)
	if a.doc.ink != 0xC6 || len(a.undo) != 1 {
		t.Errorf("Atari: Ctrl+3 gave ink %d, %d undo steps", a.doc.ink, len(a.undo))
	}

	a = testApp(t, "tt")
	editorKey("3", true, false, false)
	if a.fg != 7 || a.message == "" {
		t.Errorf("teletext: Ctrl+3 changed fg to %d (message %q)", a.fg, a.message)
	}

	a = testApp(t, "zx")
	editorKey("D", true, false, false)
	if a.fg != 0 || a.bg != 7 {
		t.Errorf("ZX: Ctrl+D gave %d on %d", a.fg, a.bg)
	}
}
