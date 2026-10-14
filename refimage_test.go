package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/image/bmp"
)

// testPicture writes a w x h picture of one colour (its left half another) and gives its path.
func testPicture(t *testing.T, name string, w, h int, left, right color.NRGBA) string {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := right
			if x < w/2 {
				c = left
			}
			img.SetNRGBA(x, y, c)
		}
	}
	var b bytes.Buffer
	var err error
	if filepath.Ext(name) == ".bmp" {
		err = bmp.Encode(&b, img)
	} else {
		err = png.Encode(&b, img)
	}
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

var (
	red  = color.NRGBA{255, 0, 0, 255}
	blue = color.NRGBA{0, 0, 255, 255}
)

func TestRefLoad(t *testing.T) {
	for _, name := range []string{"a.png", "a.bmp"} {
		p := testPicture(t, name, 8, 4, red, blue)
		b, _ := os.ReadFile(p)
		if !isPicture(b) || !pictureFile(p) {
			t.Errorf("%s is not taken for a picture", name)
		}
		r, err := loadRefImage(p)
		if err != nil {
			t.Fatal(err)
		}
		if r.w != 8 || r.h != 4 || r.px[0] != 0xFFFF0000 || r.px[7] != 0xFF0000FF {
			t.Errorf("%s: %dx%d %08x %08x", name, r.w, r.h, r.px[0], r.px[7])
		}
	}
	if isPicture([]byte("\x1b[0mHello")) || isPicture([]byte("BM")) {
		t.Error("art taken for a picture")
	}
	if _, err := loadRefImage(filepath.Join(t.TempDir(), "none.png")); err == nil {
		t.Error("no error for a missing picture")
	}
	bad := filepath.Join(t.TempDir(), "bad.png")
	os.WriteFile(bad, []byte("\x89PNG broken"), 0o644)
	if _, err := loadRefImage(bad); err == nil {
		t.Error("no error for a broken picture")
	}
}

// TestRefRemembered: the reference goes with the art file in prefs.json, and comes back when it opens.
func TestRefRemembered(t *testing.T) {
	a := testApp(t, "pc")
	pic := testPicture(t, "ref.png", 64, 32, red, blue)
	art := filepath.Join(t.TempDir(), "art.ans")
	os.WriteFile(art, []byte("Hi\r\n"), 0o644)
	a.openPath(art, "auto")
	a.openRef(pic)
	if a.ref == nil || a.ref.scale != float64(a.doc.cols*8)/64 {
		t.Fatalf("not fitted to the width: %+v", a.ref)
	}
	a.refFade(-1)
	a.refOver()
	a.ref.x, a.ref.y = 5, -7
	a.endRefMove()
	saved, ok := readPrefsFile()
	if !ok {
		t.Fatal("prefs.json not written")
	}
	p := saved.Refs[art]
	if p == nil || p.Path != pic || p.X != 5 || p.Y != -7 || !p.Over || p.Opacity != 40 {
		t.Fatalf("kept: %+v", p)
	}

	// Another editor opens the art file: its reference is back, as it was.
	b := restart(t, "pc")
	prefs = saved
	b.openPath(art, "auto")
	if b.ref == nil || b.ref.path != pic || b.ref.x != 5 || b.ref.y != -7 || !b.ref.over || b.ref.opacity != 40 {
		t.Fatalf("not back: %+v", b.ref)
	}
	// Taken away: forgotten for the file.
	b.refRemove()
	if saved, _ := readPrefsFile(); saved.Refs[art] != nil {
		t.Fatal("a reference taken away is still kept")
	}
	// Save as under another name: the reference goes with the new name.
	b.openRef(pic)
	other := filepath.Join(t.TempDir(), "other.ans")
	if !b.writeOut(other, "ansi") {
		t.Fatal("not saved")
	}
	if saved, _ := readPrefsFile(); saved.Refs[other] == nil {
		t.Fatal("the reference is not kept for the new name")
	}
	// A picture whose file is gone: no reference, a message, no crash.
	os.Remove(pic)
	c := restart(t, "pc")
	prefs, _ = readPrefsFile()
	c.openPath(other, "auto")
	if c.ref != nil {
		t.Fatal("a reference without its picture")
	}
}

func TestRefOldestForgotten(t *testing.T) {
	a := testApp(t, "pc")
	pic := testPicture(t, "ref.png", 4, 4, red, blue)
	a.openRef(pic)
	prefs.Refs = map[string]*RefPref{}
	for i := 0; i < maxRefPrefs; i++ {
		prefs.Refs[filepath.Join("/old", string(rune('a'+i%26)), time.Duration(i).String())] = &RefPref{Path: pic, Used: int64(i + 1)}
	}
	a.path = "/new/art.ans"
	a.rememberRef()
	if len(prefs.Refs) != maxRefPrefs || prefs.Refs["/new/art.ans"] == nil || prefs.Refs[filepath.Join("/old", "a", time.Duration(0).String())] != nil {
		t.Fatalf("%d kept", len(prefs.Refs))
	}
}

func TestRefAutosave(t *testing.T) {
	a := testApp(t, "pc")
	pic := testPicture(t, "ref.png", 16, 16, red, blue)
	a.typeByte('A')
	a.openRef(pic)
	a.ref.x, a.ref.scale, a.ref.greyOn, a.ref.hidden = 12, 0.5, true, true
	if err := writeAutosave(a.Tab); err != nil {
		t.Fatal(err)
	}
	b := restart(t, "pc")
	files := leftAutosaves()
	if len(files) != 1 {
		t.Fatalf("%d autosaves", len(files))
	}
	if err := b.restoreAutosave(files[0]); err != nil {
		t.Fatal(err)
	}
	r := b.ref
	if r == nil || r.path != pic || r.x != 12 || r.scale != 0.5 || !r.greyOn || !r.hidden {
		t.Fatalf("the autosave's reference: %+v", r)
	}
}

// TestRefNeverExported: what is saved or exported is the same with a reference shown over everything.
func TestRefNeverExported(t *testing.T) {
	a := testApp(t, "pc")
	for _, c := range "Art" {
		a.typeByte(int(c))
	}
	formats := []string{"ansi", "bin", "xbin", "png", "html", "text"}
	want := map[string][]byte{}
	for _, f := range formats {
		b, err := a.encode(f)
		if err != nil {
			t.Fatal(f, err)
		}
		want[f] = b
	}
	a.openRef(testPicture(t, "ref.png", 32, 32, red, blue))
	a.ref.over, a.ref.opacity = true, 100
	for _, f := range formats {
		if b, _ := a.encode(f); !bytes.Equal(b, want[f]) {
			t.Errorf("%s changes with a reference", f)
		}
	}
	if fb := a.renderDoc(a.doc, 1); fb.px[0] != cPaper && fb.px[0] != 0xFF000000 {
		t.Errorf("the rendered page has the reference: %08x", fb.px[0])
	}
}

// TestRefDrawn: the reference shows in the empty cells only (or, over, in all of them, faint).
func TestRefDrawn(t *testing.T) {
	a := testApp(t, "pc")
	a.zoom = 1
	a.doc.set(1, 0, makeCell('#', 7, 0, 0))
	a.openRef(testPicture(t, "ref.png", a.doc.cols*8, a.doc.rows*16, red, red))
	a.ref.opacity = 100
	fb := &FB{}
	fb.resize(32, 16)
	fb.fill(Rect{0, 0, 32, 16}, cPaper)
	a.drawRef(fb, 0, 0, 0, 4, 0, 1)
	if fb.px[0] != 0xFFFF0000 || fb.px[16+4] != 0xFFFF0000 {
		t.Errorf("empty cells: %08x %08x", fb.px[0], fb.px[20])
	}
	if fb.px[8] != cPaper || fb.px[15*32+15] != cPaper {
		t.Error("the reference over a cell that is drawn")
	}
	a.ref.over, a.ref.opacity = true, 50
	fb.fill(Rect{0, 0, 32, 16}, cPaper)
	a.drawRef(fb, 0, 0, 0, 4, 0, 1)
	if p := fb.px[8]; p>>16&255 < 100 || p>>16&255 > 150 {
		t.Errorf("over at 50%%: %08x", p)
	}
	a.ref.hidden = true
	fb.fill(Rect{0, 0, 32, 16}, cPaper)
	a.drawRef(fb, 0, 0, 0, 4, 0, 1)
	if fb.px[0] != cPaper {
		t.Error("a hidden reference is drawn")
	}
}

// TestDroppedPicture: a picture opened or dropped asks about the reference, and opens no page.
func TestDroppedPicture(t *testing.T) {
	a := testApp(t, "pc")
	pic := testPicture(t, "ref.png", 8, 8, red, blue)
	a.openPath(pic, "auto")
	d, ok := a.dialog.(*Confirm)
	if !ok || len(a.tabs) != 1 {
		t.Fatalf("dialog %T, %d tabs", a.dialog, len(a.tabs))
	}
	closeDialog()
	d.yes()
	if a.ref == nil || a.ref.path != pic || a.dirty {
		t.Fatal("the picture is not the reference, or the page changed")
	}
}

func TestRefKeys(t *testing.T) {
	a := testApp(t, "pc")
	runKey(keysNow["ref_show"][0]) // no reference: a message, nothing else
	a.openRef(testPicture(t, "ref.png", 8, 8, red, blue))
	runKey(keysNow["ref_show"][0])
	if !a.ref.hidden {
		t.Error("ref_show")
	}
	runKey(keysNow["ref_move"][0])
	x := a.ref.x
	editorKey("Right", false, false, false)
	editorKey("Right", false, true, false)
	if !a.refMove || a.ref.x != x+9 || a.ref.hidden {
		t.Errorf("moving: %v %v", a.refMove, a.ref.x-x)
	}
	if a.sel != nil {
		t.Error("Shift+Right selected while moving the reference")
	}
	s := a.ref.scale
	editorKey("=", false, false, false)
	if a.ref.scale <= s {
		t.Error("+ in the move mode")
	}
	editorKey("Escape", false, false, false)
	if a.refMove {
		t.Error("Esc does not end moving")
	}
}
