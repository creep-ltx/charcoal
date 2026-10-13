package main

import (
	"os"
	"path/filepath"
	"testing"
)

// testApp is an editor with one empty page of the given type, no window. Its
// settings and autosaves go to folders of the test's own, never the user's
// prefs.json or data folder; the keys are the defaults.
func testApp(t *testing.T, mode string) *App {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	setKeys(defaultKeys())
	prefs = Prefs{}
	a = &App{tool: "draw", fg: 7, brush: 0x2F, charHover: -1, layout: "one", defZoom: 2,
		defFont: map[string]string{}, defPalette: map[string]string{}, fkeys: defaultFkeys(), baud: 14400}
	a.focusTab(a.newTab(mode))
	return a
}

func TestValidFkeys(t *testing.T) {
	ok := [][]int{{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}}
	for _, c := range []struct {
		sets [][]int
		want bool
	}{
		{ok, true},
		{nil, false},
		{[][]int{{1, 2, 3}}, false},
		{[][]int{{1, 2, 3, 4, 5, 6, 7, 8, 9, 256}}, false},
		{[][]int{{-1, 2, 3, 4, 5, 6, 7, 8, 9, 10}}, false},
	} {
		if got := validFkeys(c.sets); got != c.want {
			t.Errorf("validFkeys(%v) = %v", c.sets, got)
		}
	}
}

func TestPageHeightLimits(t *testing.T) {
	for _, c := range []struct {
		mode string
		max  int
	}{{"zx", 24}, {"tt", 25}} {
		a := testApp(t, c.mode)
		for i := 0; i < 40; i++ {
			a.cy++
			a.moved()
		}
		a.setCell(0, 30, a.cell('x'))
		a.setRows(100)
		if a.doc.rows != c.max || a.cy != c.max-1 {
			t.Errorf("%s: %d rows, cursor on %d", c.mode, a.doc.rows, a.cy)
		}
	}
	a := testApp(t, "pc")
	a.setRows(1 << 30)
	if a.doc.rows != maxRowsFor(a.doc.cols) {
		t.Errorf("pc: setRows gave %d rows, the limit is %d", a.doc.rows, maxRowsFor(a.doc.cols))
	}
}

func TestUndoBackToSavedIsClean(t *testing.T) {
	a := testApp(t, "pc")
	a.typeByte('a')
	a.typeByte('b')
	a.typeByte('c')
	if !a.dirty || len(a.undo) != 1 {
		t.Fatalf("after typing: dirty %v, %d undo steps (want one)", a.dirty, len(a.undo))
	}
	a.doUndo()
	if a.dirty {
		t.Error("undoing back to the unchanged page left it marked changed")
	}
	a.doRedo()
	if !a.dirty {
		t.Error("redo did not mark it changed")
	}
	a.markSaved()
	a.typeByte('d') // typing after a save is a new step
	a.doUndo()
	if a.dirty {
		t.Error("undo back to the saved page left it marked changed")
	}
}

func TestPlaySavesThePage(t *testing.T) {
	a := testApp(t, "pc")
	for _, c := range "hello" {
		a.typeByte(int(c))
	}
	want, _ := a.encode("ansi")
	a.play()
	a.playStep()
	dir := t.TempDir()
	p := filepath.Join(dir, "x.ans")
	if !a.writeOut(p, "ansi") {
		t.Fatal(a.message)
	}
	got, _ := os.ReadFile(p)
	if string(got) != string(want) || a.playing != nil {
		t.Errorf("saved during Play: %q, want %q", got, want)
	}
}

func TestWriteAtomicFollowsSymlink(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "real.ans")
	link := filepath.Join(dir, "link.ans")
	os.WriteFile(real, []byte("old"), 0o644)
	if err := os.Symlink(real, link); err != nil {
		t.Skip(err)
	}
	if err := writeAtomic(link, []byte("new")); err != nil {
		t.Fatal(err)
	}
	if st, _ := os.Lstat(link); st.Mode()&os.ModeSymlink == 0 {
		t.Error("the link was replaced by a file")
	}
	if b, _ := os.ReadFile(real); string(b) != "new" {
		t.Errorf("the file the link points at holds %q", b)
	}
}

func TestCloseTabNeverShowsOneTabTwice(t *testing.T) {
	a := testApp(t, "pc")
	t1 := a.Tab
	a.setLayout("cols")
	t2 := a.shown[1]
	a.closeTab(t1)
	if a.layout != "one" || a.Tab != t2 {
		t.Errorf("layout %s, shown %v %v", a.layout, a.shown[0], a.shown[1])
	}
}
