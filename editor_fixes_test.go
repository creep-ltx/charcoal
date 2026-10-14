package main

import (
	"bytes"
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

// A .c or .petmate file with several screens opens as one page of frames;
// saving it unchanged gives the same bytes, an edit of one frame keeps the
// others, and Petmate's fields CharEdit does not know are kept.
func TestScreensAsFrames(t *testing.T) {
	withFonts(t, false)
	s1, s2, s3 := newDoc(40, 25), newDoc(40, 25), newDoc(40, 25)
	s1.set(0, 0, makeCell(1, 3, 0, 0))
	s2.set(5, 5, makeCell(2, 4, 0, 0))
	s3.set(7, 7, makeCell(3, 5, 0, 0))
	pmSrc := []byte(`{"version":4,"screens":[0,1,2],"framebufs":[` +
		`{"width":40,"height":25,"backgroundColor":6,"borderColor":14,"charset":"upper","name":"one","zoom":{"zoomLevel":2},"framebuf":[]},` +
		`{"width":40,"height":25,"backgroundColor":6,"borderColor":14,"charset":"lower","name":"two","framebuf":[]},` +
		`{"width":40,"height":25,"backgroundColor":6,"borderColor":14,"charset":"upper","name":"three","framebuf":[]}],` +
		`"customFonts":{"x":{"name":"mine"}},"somethingNew":[1,2]}`)
	pmSrc = writePetmateFrames(pmSrc, []PetScreen{{doc: s1, fb: 0}, {doc: s2, fb: 1, lower: 1}, {doc: s3, fb: 2}})
	for _, c := range []struct {
		ext  string
		data []byte
	}{{".c", writeMarqFrames([]*Doc{s1, s2, s3}, false, "c64")}, {".petmate", pmSrc}} {
		kind := map[string]string{".c": "marq", ".petmate": "petmate"}[c.ext]
		a := testApp(t, "pc")
		a.hasC64 = true
		p := filepath.Join(t.TempDir(), "three"+c.ext)
		if err := os.WriteFile(p, c.data, 0o644); err != nil {
			t.Fatal(err)
		}
		a.openPath(p, "auto")
		if len(a.tabs) != 1 || a.nFrames() != 3 || a.cur != 0 {
			t.Fatalf("%s: %d tabs, %d frames", c.ext, len(a.tabs), a.nFrames())
		}
		a.save(false)
		if b, _ := os.ReadFile(p); !bytes.Equal(b, c.data) {
			t.Fatalf("%s: an unchanged save changed the file", c.ext)
		}
		a.goFrame(1)
		if kind == "petmate" && !a.lower() {
			t.Fatal("frame 2 is lower case in the file")
		}
		a.pushUndo()
		a.doc.set(9, 9, makeCell(7, 1, 0, 0))
		a.save(false)
		if a.dirty {
			t.Fatalf("%s: %s", c.ext, a.message)
		}
		b, _ := os.ReadFile(p)
		scr, err := a.readScreens(b, kind)
		if err != nil || len(scr) != 3 {
			t.Fatalf("%s: %d screens back, %v", c.ext, len(scr), err)
		}
		if scr[0].doc.get(0, 0) != s1.get(0, 0) || scr[1].doc.get(5, 5) != s2.get(5, 5) || scr[2].doc.get(7, 7) != s3.get(7, 7) {
			t.Fatalf("%s: a screen lost its cells", c.ext)
		}
		if scr[1].doc.get(9, 9) != makeCell(7, 1, 0, 0) || scr[0].doc.get(9, 9) == makeCell(7, 1, 0, 0) {
			t.Fatalf("%s: the edit is not where it was made", c.ext)
		}
		if kind == "petmate" {
			if scr[1].lower != 1 || scr[0].lower != 0 || scr[2].name != "three" {
				t.Fatalf("petmate: cases %d %d, name %q", scr[0].lower, scr[1].lower, scr[2].name)
			}
			for _, keep := range []string{`"customFonts":{"x":{"name":"mine"}}`, `"somethingNew":[1,2]`, `"zoom":{"zoomLevel":2}`, `"name":"two"`} {
				if !bytes.Contains(b, []byte(keep)) {
					t.Fatalf("petmate lost %s", keep)
				}
			}
		}
		// A new frame and a move, saved: four screens in the new order.
		a.newFrame()
		a.moveFrame(-1)
		a.save(false)
		b, _ = os.ReadFile(p)
		scr, _ = a.readScreens(b, kind)
		if len(scr) != 4 || scr[2].doc.get(9, 9) != makeCell(7, 1, 0, 0) || scr[1].doc.inkCells() != 0 {
			t.Fatalf("%s: after a new frame: %d screens", c.ext, len(scr))
		}
		if kind == "petmate" && (scr[2].name != "two" || scr[3].name != "three") {
			t.Fatalf("petmate: names %q %q", scr[2].name, scr[3].name)
		}
	}
}

// An empty page whose cells have a paper colour (a Spectrum's white, an ST's)
// is still empty: the page type buttons give it the new type's size.
func TestEmptyPaperPageTakesNewType(t *testing.T) {
	withFonts(t, true)
	for _, from := range []string{"zx", "st"} {
		a := testApp(t, from)
		if !a.doc.empty() || !a.pristine() {
			t.Fatalf("%s: a new page is not empty", from)
		}
		a.setMode("pc")
		if a.doc.cols != 80 || a.doc.rows != 25 {
			t.Fatalf("%s to PC: %dx%d", from, a.doc.cols, a.doc.rows)
		}
		a = testApp(t, from)
		a.pushUndo()
		a.doc.set(1, 1, a.cell('x'))
		if a.doc.empty() {
			t.Fatalf("%s: a drawn page counts as empty", from)
		}
	}
}
