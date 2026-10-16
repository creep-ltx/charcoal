package main

import (
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// lay2FPress presses mouse button 1 on cell x, y of the page (as a click does).
func lay2FPress(a *App, x, y int) {
	a.page = Rect{0, 0, 4000, 4000}
	a.scrollX, a.scrollY = 0, 0
	a.zoom = 1
	a.pagePress(&Click{x: x*a.cellW() + 1, y: y*a.cellH() + 1, button: 1})
	a.pageRelease()
	a.lockCheck()
}

// lay2FTwo gives a PC page of two layers, the top one current: 'a' at 0,0 below.
func lay2FTwo(t *testing.T) *App {
	a := testApp(t, "pc")
	a.doc.set(0, 0, makeCell('a', 1, 2, 0))
	a.newLayer(false)
	return a
}

func TestLay2LockStopsTools(t *testing.T) {
	a := lay2FTwo(t)
	a.toggleLock(1)
	if !a.layerLocked() {
		t.Fatal("not locked")
	}
	steps := len(a.undo)
	// the draw tool
	a.setTool("draw")
	lay2FPress(a, 3, 2)
	if a.doc.get(3, 2) != blank || len(a.undo) != steps || !strings.Contains(a.message, "locked") {
		t.Fatalf("draw on a locked layer: %08x, %d steps, %q", a.doc.get(3, 2), len(a.undo)-steps, a.message)
	}
	// typing
	a.cx, a.cy = 0, 1
	a.typeByte('x')
	if a.doc.get(0, 1) != blank {
		t.Fatal("typed on a locked layer")
	}
	// an edit that reaches pushUndo some other way (Delete of a selection, a fill
	// of a block): its cells are put back after the event, no undo step is left
	a.doc.set(5, 5, makeCell('q', 3, 0, 0)) // (as if it were there before)
	before := a.doc.clone()
	a.sel = &Sel{0, 0, 9, 9}
	a.fillSel()
	a.lockCheck()
	if a.doc.get(2, 2) != before.get(2, 2) || a.doc.get(5, 5) != makeCell('q', 3, 0, 0) || len(a.undo) != steps {
		t.Fatalf("a block fill changed the locked layer: %08x, %d steps", a.doc.get(2, 2), len(a.undo)-steps)
	}
	a.sel = nil
	// a stamp of a block
	a.float = &Block{w: 1, h: 1, cells: []uint32{makeCell('z', 4, 0, 0)}}
	lay2FPress(a, 1, 1)
	if a.doc.get(1, 1) != blank || a.float == nil {
		t.Fatal("stamped on a locked layer (the block should stay)")
	}
	a.float = nil
	// the layer below is not locked: it takes edits
	a.goLayer(0)
	lay2FPress(a, 3, 2)
	if a.doc.get(3, 2) == blank {
		t.Fatal("the unlocked layer took no edit")
	}
	// unlocked again: the top layer takes edits
	a.goLayer(1)
	a.toggleLock(1)
	lay2FPress(a, 4, 2)
	if a.doc.get(4, 2) == blank {
		t.Fatal("an unlocked layer took no edit")
	}
	// the lock is an undo step
	a.toggleLock(1)
	a.doUndo()
	if a.layerLocked() {
		t.Fatal("undo of the lock")
	}
	a.doRedo()
	if !a.layerLocked() {
		t.Fatal("redo of the lock")
	}
}

func TestLay2LockKeepsPageChanges(t *testing.T) {
	a := testApp(t, "c64")
	a.newLayer(false)
	a.toggleLock(1)
	steps := len(a.undo)
	bg := a.doc.bg
	a.setBg((bg + 3) % 16) // the screen colour is the page's, not the layer's
	a.lockCheck()
	if a.doc.bg == bg || len(a.undo) != steps+1 {
		t.Fatalf("screen colour on a locked layer: %d (was %d), %d steps", a.doc.bg, bg, len(a.undo)-steps)
	}
	a.doUndo()
	if a.doc.bg != bg {
		t.Fatal("undo of the screen colour")
	}
}

func TestLay2LockRefusals(t *testing.T) {
	a := lay2FTwo(t)
	a.doc.set(2, 0, makeCell('b', 1, 0, 0))
	a.toggleLock(1)
	a.mergeDown()
	if a.nLayers() != 2 || !strings.Contains(a.message, "Unlock") {
		t.Fatalf("merge down of a locked layer: %d layers, %q", a.nLayers(), a.message)
	}
	a.deleteLayer()
	if a.nLayers() != 2 {
		t.Fatal("deleted a locked layer")
	}
	// the page's structure is every layer's, locked ones too
	a.setCols(60)
	if a.doc.cols != 60 || a.layers[0].doc.cols != 60 {
		t.Fatalf("width with a locked layer: %d, %d", a.doc.cols, a.layers[0].doc.cols)
	}
	// moving, hiding and renaming a locked layer are no edits of its cells
	a.moveLayer(-1)
	if a.lay != 0 || !a.layers[0].locked {
		t.Fatal("moving a locked layer")
	}
	a.renameLayer(0, "Fixed")
	a.toggleLayer(0)
	if a.layers[0].name != "Fixed" || !a.layers[0].hidden {
		t.Fatal("renaming / hiding a locked layer")
	}
}

func TestLay2LockKept(t *testing.T) {
	// autosave and the side file of a flat file
	a := lay2FTwo(t)
	a.doc.set(1, 1, makeCell('L', 3, 0, 0))
	a.toggleLock(0)
	a.path, a.name = "/nowhere/art.ans", "art.ans"
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
	if b.nLayers() != 2 || !b.layers[0].locked || b.layers[1].locked {
		t.Fatal("autosave lost the lock")
	}
	removeAutosave(b.autoID)
	p := filepath.Join(t.TempDir(), "art.ans")
	if !a.writeOut(p, "ansi") {
		t.Fatal(a.message)
	}
	c := restart(t, "pc")
	c.openPath(p, "auto")
	if c.nLayers() != 2 || !c.layers[0].locked {
		t.Fatal("the side file lost the lock")
	}
}

func TestLay2LockIcyDraw(t *testing.T) {
	a := lay2FTwo(t)
	a.doc.set(1, 1, makeCell('L', 3, 0, 0))
	a.toggleLock(1)
	b := layFFormat(t, a, "icy", ".icy")
	if b.nLayers() != 2 || b.layers[0].locked || !b.layers[1].locked {
		t.Fatalf("IcyDraw lock: %d layers", b.nLayers())
	}
	// the flags as written: visible and the edit lock; IcyDraw's other bits kept
	ic, err := parseIcyDraw(b.source)
	if err != nil {
		t.Fatal(err)
	}
	if f := ic.layers[1].flags; f != icyVisible|icyEditLock {
		t.Fatalf("flags %b", f)
	}
	b.layers[1].keep.IcyFlags |= 2 | 16 // position and alpha locks: kept as they are
	b.toggleLock(1)
	c := layFFormat(t, b, "icy", ".icy")
	ic, _ = parseIcyDraw(c.source)
	if f := ic.layers[1].flags; f != icyVisible|2|16 || c.layers[1].locked {
		t.Fatalf("unlocked flags %b", f)
	}
}

// ---------------------------------------------------------------- layers with frames

// lay2FFrames gives a page (mode) of two frames of three layers: art at the
// bottom, letters on the middle one (another in each frame), a hidden top one.
func lay2FFrames(t *testing.T, mode string) *App {
	t.Helper()
	a := testApp(t, mode)
	layFArt(a, 3)
	a.dupFrame()
	layFArt(a, 6)
	a.goFrame(0)
	a.newLayer(false)
	if a.nLayers() != 2 || a.frames[1].layers == nil || len(a.frames[1].layers) != 2 {
		t.Fatalf("a new layer on a page of frames: %d layers, frame 2 has %d", a.nLayers(), len(a.frames[1].layers))
	}
	ch := 'M'
	if a.c64() || a.atari() {
		ch = 13
	}
	cell := func(c rune, fg int) uint32 {
		if a.uni() {
			return uniCell(int(c), styleOf(fg, 0, 0))
		}
		return makeCell(int(c), fg, 0, 0)
	}
	a.pushUndo()
	a.doc.set(2, 1, cell(ch, 7))
	a.goFrame(1)
	if a.lay != 1 || !a.doc.blankAt(a.doc.get(2, 1)) {
		t.Fatalf("frame 2's new layer is not empty: layer %d, %08x", a.lay, a.doc.get(2, 1))
	}
	a.pushUndo()
	a.doc.set(3, 1, cell(ch+1, 5))
	a.newLayer(false)
	a.doc.set(0, 0, cell(ch+2, 4))
	a.toggleLayer(2)
	a.goLayer(1)
	return a
}

// lay2FFlat is a page of the same frames, one layer each, as a's frames are seen.
func lay2FFlat(t *testing.T, a *App) *App {
	b := testApp(t, a.mode)
	b.frames = make([]Frame, a.nFrames())
	for i := range b.frames {
		b.frames[i] = Frame{doc: a.frameDoc(i).clone(), hold: a.holdOf(i), lower: -1, src: -1}
	}
	b.cur = a.cur
	b.doc = b.frames[b.cur].doc.clone()
	return b
}

func TestLay2FramesOfLayers(t *testing.T) {
	a := lay2FFrames(t, "pc")
	f0, f1 := a.frameDoc(0), a.frameDoc(1)
	if f0.get(2, 1) != makeCell('M', 7, 0, 0) || f1.get(3, 1) != makeCell('N', 5, 0, 0) || f1.get(2, 1) == makeCell('M', 7, 0, 0) {
		t.Fatal("each frame's own letters")
	}
	if f1.get(0, 0) == makeCell('O', 4, 0, 0) {
		t.Fatal("a hidden layer is seen")
	}
	// the frame that is not current keeps the picture its layers make
	if !sameLook(a.frames[0].doc, flatten(layersWith(a.layers, a.frames[0].layers), -1, a.frames[0].doc)) {
		t.Fatal("frame 1's picture is not its layers put together")
	}
	// showing the hidden layer shows it in every frame
	a.toggleLayer(2)
	if a.frameDoc(1).get(0, 0) != makeCell('O', 4, 0, 0) {
		t.Fatal("shown layer")
	}
	a.goFrame(0)
	if a.seen().get(0, 0) == makeCell('O', 4, 0, 0) {
		t.Fatal("frame 1's top layer has frame 2's cell")
	}
	// the strip's onion and playing read Frame.doc: as seen
	a.goFrame(1)
	if !sameLook(a.frames[0].doc, f0) {
		t.Fatal("the onion's frame before is not as seen")
	}
	s1 := a.seen().clone()
	a.playFrames()
	if a.playing == nil || !sameLook(a.playing.frames[0], f0) || !sameLook(a.playing.frames[1], s1) {
		t.Fatal("playing does not show the frames as seen")
	}
	a.stopPlay(false)
	if a.cur != 1 || a.lay != 1 || a.layers == nil {
		t.Fatal("after playing")
	}
}

// layersWith is the layer list ls with the pictures docs.
func layersWith(ls []Layer, docs []*Doc) []Layer {
	out := make([]Layer, len(ls))
	for k := range ls {
		out[k] = ls[k]
		out[k].doc = docs[k]
	}
	return out
}

func TestLay2FramesWriteAsSeen(t *testing.T) {
	for _, c := range []struct{ mode, fmtID string }{{"pc", "ansi"}, {"pc", "gif"}, {"pc", "apng"}, {"c64", "seq"}, {"c64", "marq"}, {"c64", "petmate"}, {"atari", "ata"}, {"st", "vt52"}, {"uni", "uniansi"}, {"uni", "dur"}} {
		a := lay2FFrames(t, c.mode)
		b := lay2FFlat(t, a)
		got, err := a.encodeFor(c.fmtID, "", nil)
		if err != nil {
			t.Fatalf("%s %s: %v", c.mode, c.fmtID, err)
		}
		want, _ := b.encodeFor(c.fmtID, "", nil)
		if !bytesEq(got, want) {
			t.Errorf("%s %s: frames of layers write %d bytes, their pictures %d", c.mode, c.fmtID, len(got), len(want))
		}
	}
}

func bytesEq(x, y []byte) bool { return string(x) == string(y) }

func TestLay2FramesUndo(t *testing.T) {
	a := testApp(t, "pc")
	layFArt(a, 1)
	a.dupFrame()
	before := a.frameDoc(0).clone()
	a.newLayer(false)
	a.pushUndo()
	a.doc.set(5, 5, makeCell('Z', 2, 0, 0))
	a.goFrame(0)
	// undoing frame 2's edit goes to frame 2's layer
	a.doUndo()
	if a.cur != 1 || a.lay != 1 || a.doc.get(5, 5) == makeCell('Z', 2, 0, 0) {
		t.Fatalf("undo of frame 2's edit: frame %d layer %d", a.cur, a.lay)
	}
	a.doRedo()
	if a.cur != 1 || a.doc.get(5, 5) != makeCell('Z', 2, 0, 0) {
		t.Fatal("redo")
	}
	// undoing the new layer takes it out of every frame
	a.doUndo()
	a.doUndo()
	if a.layers != nil || a.frames[0].layers != nil || !sameLook(a.frameDoc(0), before) {
		t.Fatalf("undo of the new layer: layers %v", a.layers != nil)
	}
	a.doRedo()
	if a.nLayers() != 2 || len(a.frames[1-a.cur].layers) != 2 {
		t.Fatal("redo of the new layer")
	}
	// a new frame on a page of layers has every layer; deleting it and undoing
	a.newFrame()
	if a.nFrames() != 3 || a.nLayers() != 2 || a.cur != 2 {
		t.Fatalf("new frame: %d frames at %d", a.nFrames(), a.cur)
	}
	a.doc.set(1, 1, makeCell('Y', 3, 0, 0))
	a.goFrame(0)
	if len(a.frames[2].layers) != 2 || a.frames[2].doc.get(1, 1) != makeCell('Y', 3, 0, 0) {
		t.Fatal("the new frame's layers")
	}
	a.goFrame(2)
	a.deleteFrame()
	if a.nFrames() != 2 || a.nLayers() != 2 {
		t.Fatal("delete frame")
	}
	a.doUndo()
	if a.nFrames() != 3 {
		t.Fatal("undo of delete frame")
	}
}

func TestLay2FramesLayerOps(t *testing.T) {
	a := lay2FFrames(t, "pc")
	seen1 := a.frameDoc(1).clone()
	// move the middle layer up in every frame: under the (shown) top layer it was over
	a.toggleLayer(2)
	a.moveLayer(1)
	if a.lay != 2 || a.layers[2].name != "" || len(a.frames[0].layers) != 3 {
		t.Fatal("move")
	}
	if a.frames[0].doc.get(2, 1) != makeCell('M', 7, 0, 0) {
		t.Fatal("frame 1 after the move")
	}
	a.doUndo()
	a.doUndo()
	if !sameLook(a.frameDoc(1), seen1) {
		t.Fatal("undo of the move")
	}
	// merge down in every frame
	a.goLayer(1)
	a.mergeDown()
	if a.nLayers() != 2 || len(a.frames[0].layers) != 2 || !sameLook(a.frameDoc(0), a.frames[0].doc) {
		t.Fatal("merge down")
	}
	if a.frames[0].layers[0].get(2, 1) != makeCell('M', 7, 0, 0) {
		t.Fatal("frame 1's letters merged down")
	}
	// delete a layer: one left, every frame is one picture again
	a.goLayer(1)
	a.deleteLayer()
	if a.layers != nil || a.frames[0].layers != nil || a.frames[0].doc.get(2, 1) != makeCell('M', 7, 0, 0) {
		t.Fatal("delete down to one layer")
	}
	// merge all: every frame as it was seen
	b := lay2FFrames(t, "pc")
	s0, s1 := b.frameDoc(0).clone(), b.frameDoc(1).clone()
	b.mergeAll()
	if b.layers != nil || b.frames[0].layers != nil || !sameLook(b.frameDoc(0), s0) || !sameLook(b.frameDoc(1), s1) {
		t.Fatal("merge all")
	}
	// the width is every frame's every layer's
	c := lay2FFrames(t, "pc")
	c.setCols(100)
	for i := range c.frames {
		for k, d := range c.frameLayerDocs(i) {
			if d.cols != 100 {
				t.Fatalf("frame %d layer %d: %d columns", i, k, d.cols)
			}
		}
	}
	c.doUndo()
	if c.frames[0].layers[0].cols != 80 || c.doc.cols != 80 {
		t.Fatal("undo of the width")
	}
}

func TestLay2FramesKept(t *testing.T) {
	for _, mode := range []string{"pc", "uni", "c64"} {
		a := lay2FFrames(t, mode)
		a.toggleLock(0)
		a.path, a.name = "/nowhere/art.ans", "art.ans"
		if err := writeAutosave(a.Tab); err != nil {
			t.Fatal(err)
		}
		b := restart(t, mode)
		files := leftAutosaves()
		if len(files) != 1 {
			t.Fatalf("%d autosaves", len(files))
		}
		if err := b.restoreAutosave(files[0]); err != nil {
			t.Fatal(err)
		}
		removeAutosave(b.autoID)
		if b.nFrames() != 2 || b.nLayers() != 3 || b.cur != a.cur || b.lay != a.lay || !b.layers[0].locked {
			t.Fatalf("%s: %d frames %d layers", mode, b.nFrames(), b.nLayers())
		}
		for i := range 2 {
			if !sameLook(b.frameDoc(i), a.frameDoc(i)) {
				t.Fatalf("%s: frame %d", mode, i)
			}
		}
	}
	// Playscii keeps every frame's layers
	a := lay2FFrames(t, "pc")
	b := layFFormat(t, a, "psci", ".psci")
	if b.nFrames() != 2 || b.nLayers() != 3 {
		t.Fatalf("psci: %d frames, %d layers (%q)", b.nFrames(), b.nLayers(), b.message)
	}
	for i := range 2 {
		if !sameLook(b.frameDoc(i), a.frameDoc(i)) {
			t.Fatalf("psci: frame %d", i)
		}
	}
	if !b.layers[2].hidden {
		t.Fatal("psci: the hidden layer")
	}
	// a stream keeps the frames, not their layers: the page stays unsaved
	c := lay2FFrames(t, "pc")
	p := filepath.Join(t.TempDir(), "art.ans")
	if !c.writeOut(p, "ansi") || !c.dirty || !strings.Contains(c.message, "not their layers") {
		t.Fatalf("ansi: dirty %v %q", c.dirty, c.message)
	}
}

// ---------------------------------------------------------------- Spectrum, CPC, MSX, MZ-700

// lay2FMachineCell is a cell of character ch in colours fg on bg for the page.
func lay2FMachineCell(a *App, ch, fg, bg int) uint32 {
	switch {
	case a.zx():
		return zxCell(ch, fg, bg, false, false)
	case a.mz():
		return mzCell(ch, fg, bg)
	}
	return makeCell(ch, fg, bg, 0)
}

func TestLay2Machines(t *testing.T) {
	for _, c := range []struct{ mode, fmtID, ext string }{{"zx", "scr", ".scr"}, {"cpc", "cpcbin", ".bin"}, {"msx", "msxrom", ".rom"}, {"mz", "mzf", ".mzf"}} {
		a := testApp(t, c.mode)
		if !a.layered() {
			t.Fatalf("%s: no layers", c.mode)
		}
		a.doc.set(1, 1, lay2FMachineCell(a, 'A', 2, 3))
		a.doc.set(2, 1, lay2FMachineCell(a, 'B', 3, 2))
		one := a.doc.clone()
		flat, _ := a.encodeFor(c.fmtID, "", nil)
		a.newLayer(false)
		if !sameLook(a.seen(), one) {
			t.Fatalf("%s: a new layer changes the page", c.mode)
		}
		got, _ := a.encodeFor(c.fmtID, "", nil)
		if !bytesEq(got, flat) {
			t.Fatalf("%s: an empty layer changes the file", c.mode)
		}
		// drawn on the layer: seen over the one below; erased (right button): what is below shows again
		a.setTool("draw")
		a.brush = 'X'
		lay2FPress(a, 1, 1)
		if v := a.seen().get(1, 1); cellCh(v) != 'X' {
			t.Fatalf("%s: drawn on the layer: %08x", c.mode, v)
		}
		a.pushUndo()
		a.paintAt(1, 1, 3)
		if a.seen().get(1, 1) != one.get(1, 1) {
			t.Fatalf("%s: erased on the layer: %08x, want %08x", c.mode, a.seen().get(1, 1), one.get(1, 1))
		}
		// a row put in on every layer: the upper layer's new row is empty, the bottom's the machine's own
		a.cy = 0
		a.insertRow()
		if a.seen().get(1, 2) != one.get(1, 1) || a.seen().get(1, 0) != a.layers[0].doc.get(1, 0) {
			t.Fatalf("%s: a row put in", c.mode)
		}
		a.doUndo()
		// the machine's own file is the page as it is seen
		a.doc.set(5, 5, lay2FMachineCell(a, 'Q', 1, 0))
		b := testApp(t, c.mode)
		b.doc = a.seen().clone()
		want, _ := b.encodeFor(c.fmtID, "", nil)
		got, _ = a.encodeFor(c.fmtID, "", nil)
		if !bytesEq(got, want) {
			t.Fatalf("%s: the file is not the page as seen", c.mode)
		}
		// kept beside the file
		p := filepath.Join(t.TempDir(), "art"+c.ext)
		if !a.writeOut(p, c.fmtID) {
			t.Fatalf("%s: %q", c.mode, a.message)
		}
		r := restart(t, c.mode)
		r.openPath(p, "auto")
		if r.mode != c.mode || r.nLayers() != 2 || !sameLook(r.seen(), a.seen()) {
			t.Fatalf("%s: opened again: %s, %d layers (%q)", c.mode, r.mode, r.nLayers(), r.message)
		}
		// a page of layers keeps its machine
		a.setMode("pc")
		if a.mode != c.mode || !strings.Contains(a.message, "Merge the layers") {
			t.Fatalf("%s: changed to PC with layers: %s", c.mode, a.mode)
		}
	}
}

func TestLay2MachineClearCells(t *testing.T) {
	a := testApp(t, "zx")
	a.doc.set(0, 0, zxCell('A', 1, 2, false, false))
	a.newLayer(false)
	a.doc.set(0, 0, zxCell(32, 1, 7, false, false)) // white paper: covers, as on the screen
	if a.seen().get(0, 0) != zxCell(32, 1, 7, false, false) {
		t.Fatal("zx: white paper does not cover")
	}
	a.doc.set(0, 0, zxCell(32, 6, 0, false, true)) // a flashing space shows its ink
	if a.seen().get(0, 0)&flashFlag == 0 {
		t.Fatal("zx: a flashing space is clear")
	}
	a.doc.set(0, 0, zxCell(32, 6, 0, true, false)) // bright black: covers
	if a.seen().get(0, 0) == zxCell('A', 1, 2, false, false) {
		t.Fatal("zx: bright black is clear")
	}
	a.toggleBlackClear()
	if a.layers[1].clear != layerNormal {
		t.Fatal("zx: black shows below (ink and paper share their brightness)")
	}
	m := testApp(t, "mz")
	m.doc.set(0, 0, mzCell('A', 1, 2))
	m.newLayer(false)
	m.doc.set(0, 0, mzCell(256+32, 7, 0)) // the second set's 32 is a glyph
	if m.seen().get(0, 0) != mzCell(256+32, 7, 0) {
		t.Fatal("mz: the second set's 32 is clear")
	}
	m.doc.set(0, 0, mzCell('Z', 5, 0))
	m.toggleBlackClear()
	if m.seen().get(0, 0) != mzCell('Z', 5, 2) {
		t.Fatalf("mz: black shows below: %08x", m.seen().get(0, 0))
	}
	c := testApp(t, "cpc")
	c.doc.set(0, 0, makeCell('A', 1, 3, 0))
	c.newLayer(false)
	c.doc.set(0, 0, makeCell('Z', 2, 0, 0))
	c.toggleBlackClear()
	if c.seen().get(0, 0) != makeCell('Z', 2, 3, 0) {
		t.Fatal("cpc: black shows below")
	}
}

// ---------------------------------------------------------------- .icy layers of other machines

func TestLay2IcyMachines(t *testing.T) {
	withFonts(t, true)
	r := rand.New(rand.NewSource(7))
	for _, p := range []othFPage{{"c64", "c64", "", false}, {"c64", "plus4", "", true}, {"c64", "c128vdc", "", false}, {"c64", "vic20", "", false}, {"atari", "", "", false}, {"st", "", "stlow", false}, {"st", "", "stmed", false}} {
		a := othFApp(t, p)
		othFRandom(r, a, a.doc.rows)
		a.newLayer(false)
		for i := range 30 {
			x, y := r.Intn(a.doc.cols), r.Intn(a.doc.rows)
			var v uint32
			switch {
			case a.ted():
				v = tedCell(1+i, r.Intn(256))
			case a.st():
				v = makeCell(65+i, 1+r.Intn(3), 0, 0)
			case a.c64() && a.mach().mono:
				v = makeCell(1+i, 1, 0, 0)
			case a.c64():
				v = makeCell(1+i, r.Intn(a.mach().colours), 0, 0)
			default:
				v = makeCell(1+i, 7, 0, 0)
			}
			a.doc.set(x, y, v)
		}
		a.renameLayer(1, "Letters")
		a.toggleLock(1)
		if a.st() {
			a.toggleBlackClear() // written with the backgrounds as seen
		}
		a.newLayer(false)
		a.doc.set(0, 0, makeCell(5, map[bool]int{true: 7, false: 1}[a.atari()], 0, 0)) // (an Atari cell's colour is the screen's)
		a.toggleLayer(2)
		want := a.Tab
		path := filepath.Join(t.TempDir(), "art.icy")
		if !a.writeOut(path, "icy") {
			t.Fatalf("%v: %s", p, a.message)
		}
		if _, err := os.Stat(layersSideFile(path)); err == nil {
			t.Fatalf("%v: a side file for an .icy", p)
		}
		b, _ := os.ReadFile(path)
		a.focusTab(a.newTab("pc"))
		a.hasC64 = true
		a.loadBytes(b, "art.icy", path, "auto")
		if a.mode != p.mode || a.nLayers() != 3 {
			t.Fatalf("%v: opened as %s with %d layers: %q", p, a.mode, a.nLayers(), a.message)
		}
		if a.layerName(1) != "Letters" || !a.layers[1].locked || !a.layers[2].hidden || a.layers[0].locked {
			t.Fatalf("%v: names, locks, hidden: %+v", p, a.layers)
		}
		if !sameLook(a.seen(), want.seen()) {
			t.Fatalf("%v: the page does not look the same", p)
		}
		a.toggleLayer(2)
		want.layers[2].hidden = false
		if !sameLook(a.seen(), want.seen()) {
			t.Fatalf("%v: the hidden layer", p)
		}
	}
}

// IcyDraw's engine reads CharEdit's machine pages of layers as they are seen
// (the same command as TestOthIcyExternal):
//
//	CHARCOAL_ICYMACH="$HOME/.cache/charcoal-others/sandbox.sh target/release/examples/ccicy dump" \
//	CHARCOAL_ICYDIR=$HOME/.cache/charcoal-others/out go test -run Lay2IcyExternal .
func TestLay2IcyExternal(t *testing.T) {
	cmd, dir := os.Getenv("CHARCOAL_ICYMACH"), os.Getenv("CHARCOAL_ICYDIR")
	if cmd == "" || dir == "" {
		t.Skip("CHARCOAL_ICYMACH / CHARCOAL_ICYDIR not set")
	}
	os.MkdirAll(dir, 0o755)
	withFonts(t, true)
	r := rand.New(rand.NewSource(44))
	for _, p := range othFPages {
		a := othFApp(t, p)
		othFRandom(r, a, a.doc.rows)
		a.newLayer(false)
		for i := range 40 {
			v := makeCell(1+i, 1, 0, 0)
			switch {
			case a.ted():
				v = tedCell(1+i, r.Intn(256))
			case a.st():
				v = makeCell(65+i%26, 1+r.Intn(min(3, a.res().colours-1)), r.Intn(2), 0) // (pens the resolution has)
			case a.atari():
				v = makeCell(1+i, 7, 0, 0)
			}
			a.doc.set(r.Intn(a.doc.cols), r.Intn(a.doc.rows), v)
		}
		if a.st() {
			a.toggleBlackClear()
		}
		a.toggleLock(1)
		path := filepath.Join(dir, "lay2-"+strings.ReplaceAll(p.String(), "/", "-")+".icy")
		if !a.writeOut(path, "icy") {
			t.Fatal(a.message)
		}
		args := append(strings.Fields(cmd), path)
		out, err := exec.Command(args[0], args[1:]...).Output()
		if err != nil {
			t.Fatalf("%v: %v", p, err)
		}
		dump := othFReadDump(t, string(out))
		b, _ := os.ReadFile(path)
		doc, _ := parseIcyDraw(b)
		m := icyMachinePage(doc, a.gtia())
		othFCompare(t, p.String(), m, dump)
		seen := a.seen()
		for i, v := range seen.cells[:seen.cols*seen.rows] {
			if p.mode != "atari" && m.doc.cells[i] != v || p.mode == "atari" && cellCh(m.doc.cells[i]) != cellCh(v) {
				t.Fatalf("%v: cell %d: IcyDraw's page %08x, seen %08x", p, i, m.doc.cells[i], v)
			}
		}
		t.Logf("%v: IcyDraw reads %d layers as seen", p, len(doc.layers))
	}
}
