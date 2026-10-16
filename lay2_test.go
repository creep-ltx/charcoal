package main

import (
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
