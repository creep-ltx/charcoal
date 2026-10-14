package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func cellAt(a *App, i, x, y int) int { return cellCh(a.frameDoc(i).get(x, y)) }

// Edits in two frames are undone one by one, each going to its frame, and redone.
func TestFrameUndoAcrossFrames(t *testing.T) {
	a := testApp(t, "pc")
	a.newFrame() // frame 2
	a.goFrame(0)
	a.cx, a.cy = 0, 0
	a.typeByte('A')
	a.goFrame(1)
	a.cx, a.cy = 0, 0
	a.typeByte('B')
	if cellAt(a, 0, 0, 0) != 'A' || cellAt(a, 1, 0, 0) != 'B' {
		t.Fatal("edits not in their frames")
	}
	a.doUndo()
	if a.cur != 1 || cellAt(a, 1, 0, 0) != ' ' || cellAt(a, 0, 0, 0) != 'A' {
		t.Fatalf("first undo: cur %d", a.cur)
	}
	a.doUndo()
	if a.cur != 0 || cellAt(a, 0, 0, 0) != ' ' {
		t.Fatalf("second undo: cur %d, %c", a.cur, cellAt(a, 0, 0, 0))
	}
	a.doRedo()
	if a.cur != 0 || cellAt(a, 0, 0, 0) != 'A' || cellAt(a, 1, 0, 0) != ' ' {
		t.Fatalf("redo: cur %d", a.cur)
	}
	a.doRedo()
	if a.cur != 1 || cellAt(a, 1, 0, 0) != 'B' || cellAt(a, 0, 0, 0) != 'A' {
		t.Fatalf("second redo: cur %d", a.cur)
	}
	a.doUndo() // B
	a.doUndo() // A
	a.doUndo() // the new frame
	if a.nFrames() != 1 || a.dirty {
		t.Fatalf("all undone: %d frames, dirty %v", a.nFrames(), a.dirty)
	}
}

// A copied frame is a picture of its own; undoing a delete gives the frame back as it was.
func TestFrameDocsNotShared(t *testing.T) {
	a := testApp(t, "pc")
	a.cx, a.cy = 0, 0
	a.typeByte('X')
	a.dupFrame()
	if a.cur != 1 || cellAt(a, 1, 0, 0) != 'X' {
		t.Fatal("the copy is not the same picture")
	}
	a.cx, a.cy = 0, 0
	a.typeByte('Y')
	if cellAt(a, 0, 0, 0) != 'X' || cellAt(a, 1, 0, 0) != 'Y' {
		t.Fatal("editing the copy changed the original")
	}
	a.goFrame(0)
	a.cx, a.cy = 1, 0
	a.typeByte('Z')
	a.deleteFrame()
	if a.nFrames() != 1 || cellAt(a, 0, 0, 0) != 'Y' {
		t.Fatalf("after delete: %d frames, %c", a.nFrames(), cellAt(a, 0, 0, 0))
	}
	a.doUndo()
	if a.nFrames() != 2 || a.cur != 0 || cellAt(a, 0, 1, 0) != 'Z' || cellAt(a, 1, 0, 0) != 'Y' {
		t.Fatalf("undo delete: %d frames, cur %d", a.nFrames(), a.cur)
	}
	// Moving and undoing the move.
	a.moveFrame(1)
	if a.cur != 1 || cellAt(a, 1, 1, 0) != 'Z' || cellAt(a, 0, 0, 0) != 'Y' {
		t.Fatal("move")
	}
	a.doUndo()
	if a.cur != 0 || cellAt(a, 0, 1, 0) != 'Z' {
		t.Fatal("undo move")
	}
}

// Many frames and steps stay under the undo budget.
func TestFrameUndoBudget(t *testing.T) {
	a := testApp(t, "pc")
	a.setCols(1000)
	a.setRows(1000) // a million cells a frame
	for i := 0; i < 12; i++ {
		a.dupFrame()
	}
	for i := 0; i < 30; i++ {
		a.goFrame(i % a.nFrames())
		a.pushUndo()
		a.doc.set(i, 0, a.cell('q'))
	}
	total := 0
	for _, s := range a.undo {
		total += s.cost
	}
	if total > undoBudget {
		t.Fatalf("undo keeps %d cells", total)
	}
	if a.frameCells() > maxPageCells {
		t.Fatalf("frames hold %d cells", a.frameCells())
	}
	// Past the largest page no frame is added.
	for i := 0; i < 10; i++ {
		a.dupFrame()
	}
	if a.frameCells() > maxPageCells {
		t.Fatalf("frames grew to %d cells", a.frameCells())
	}
}

// The width is every frame's; the height is each frame's own.
func TestFramesWidthAllFrames(t *testing.T) {
	a := testApp(t, "pc")
	a.newFrame()
	a.setRows(40)
	a.setCols(60)
	for i := 0; i < 2; i++ {
		if a.frameDoc(i).cols != 60 {
			t.Fatalf("frame %d is %d wide", i, a.frameDoc(i).cols)
		}
	}
	if a.frameDoc(0).rows != 25 || a.frameDoc(1).rows != 40 {
		t.Fatalf("heights %d %d", a.frameDoc(0).rows, a.frameDoc(1).rows)
	}
	a.doUndo()
	if a.frameDoc(0).cols != 80 || a.frameDoc(1).cols != 80 {
		t.Fatal("undo of the width")
	}
	// Crop crops every frame.
	a.sel = &Sel{2, 2, 11, 6}
	a.cropToSel()
	if a.frameDoc(0).cols != 10 || a.frameDoc(1).cols != 10 || a.frameDoc(0).rows != 5 {
		t.Fatalf("crop: %dx%d", a.frameDoc(0).cols, a.frameDoc(0).rows)
	}
}

// A page of several frames saved as a one-picture file writes the current
// frame as an export: the page stays unsaved.
func TestOnePictureFormatIsExport(t *testing.T) {
	a := testApp(t, "zx")
	a.newFrame()
	p := filepath.Join(t.TempDir(), "two.scr")
	if !a.writeOut(p, "scr") {
		t.Fatal(a.message)
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatal(err)
	}
	if !a.dirty || a.path == p || !strings.Contains(a.message, "frame 2 written") {
		t.Fatalf("dirty %v path %q: %s", a.dirty, a.path, a.message)
	}
}

// Hold times step through the list, teletext by whole seconds.
func TestFrameHold(t *testing.T) {
	a := testApp(t, "pc")
	if a.holdOf(0) != 100 {
		t.Fatal(a.holdOf(0))
	}
	a.stepHold(1)
	if a.holdOf(0) != 150 {
		t.Fatal(a.holdOf(0))
	}
	a.stepHold(-1)
	a.stepHold(-1)
	if a.holdOf(0) != 80 {
		t.Fatal(a.holdOf(0))
	}
	a.doUndo()
	if a.holdOf(0) != 100 {
		t.Fatal("undo hold", a.holdOf(0))
	}
	b := testApp(t, "tt")
	b.stepHold(1)
	if b.holdOf(0) != 9000 {
		t.Fatal(b.holdOf(0))
	}
}

// The strip shows while a page has several frames, unless switched off; Ctrl+Shift+F switches it.
func TestStripShown(t *testing.T) {
	a := testApp(t, "pc")
	if a.stripShown() {
		t.Fatal("shown for one frame")
	}
	a.toggleStrip()
	if !a.stripShown() {
		t.Fatal("not shown when switched on")
	}
	a.toggleStrip()
	a.newFrame()
	if a.stripShown() {
		t.Fatal("switched off, shown again")
	}
	a.strip = 0
	if !a.stripShown() {
		t.Fatal("not shown for two frames")
	}
	a.bare = true
	if a.stripShown() {
		t.Fatal("shown with the gadgets hidden")
	}
}

// The onion skin draws the frame before at about a quarter under an empty cell, and nothing over ink.
func TestOnionPixels(t *testing.T) {
	a := testApp(t, "pc")
	a.doc.set(0, 0, makeCell(0xDB, 15, 0, 0)) // a white full block
	a.doc.set(1, 0, makeCell(0xDB, 15, 0, 0))
	a.newFrame()
	a.doc.set(1, 0, makeCell(0xDB, 4, 0, 0)) // the new frame's own ink in cell 1
	u := &UI{fb: &FB{}}
	u.fb.resize(400, 200)
	a.page = Rect{0, 0, 300, 150}
	a.blink = false
	px := func() (uint32, uint32) {
		u.fb.fill(Rect{0, 0, 400, 200}, 0)
		a.drawPage(u, "0")
		z := a.zoom
		return u.fb.px[(4*z)*400+4*z] & 0xFFFFFF, u.fb.px[(4*z)*400+a.cellW()*z+4*z] & 0xFFFFFF
	}
	if c0, _ := px(); c0 != 0 {
		t.Fatalf("onion off: %06x", c0)
	}
	a.onion = true
	c0, c1 := px()
	if r := c0 >> 16; r < 0x30 || r > 0x50 || c0 != r*0x10101 {
		t.Fatalf("onion: %06x, want a quarter white", c0)
	}
	if c1 != a.pal[4]&0xFFFFFF {
		t.Fatalf("onion over ink: %06x", c1)
	}
	a.goFrame(0)
	if c0, _ := px(); c0 != 0xFFFFFF {
		t.Fatalf("first frame: %06x", c0)
	}
}
