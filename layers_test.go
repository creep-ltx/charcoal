package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// layF2 gives the app a page of two layers: base at the bottom, top over it, top current.
func layF2(a *App, base, top *Doc) {
	a.layers = []Layer{{doc: base}, {doc: top}}
	a.lay = 1
	a.doc = top.clone()
}

func TestLayFlattenPC(t *testing.T) {
	a := testApp(t, "pc")
	base := newDoc(4, 2)
	for i := range base.cells {
		base.cells[i] = makeCell('#', 2, 1, 0)
	}
	top := newDoc(4, 2)
	top.set(1, 0, makeCell('A', 15, 4, 0)) // covers
	top.set(2, 0, makeCell('B', 14, 0, 0)) // letter on black: covers (normal layer)
	top.set(3, 0, makeCell(219, 0, 0, 0))  // a black full block covers
	layF2(a, base, top)
	p := a.seen()
	want := []uint32{makeCell('#', 2, 1, 0), makeCell('A', 15, 4, 0), makeCell('B', 14, 0, 0), makeCell(219, 0, 0, 0)}
	for x, w := range want {
		if g := p.get(x, 0); g != w {
			t.Errorf("x %d: %08x, want %08x", x, g, w)
		}
	}
	// black shows below
	a.layers[1].clear = layerBlack
	p = a.seen()
	if g := p.get(2, 0); g != makeCell('B', 14, 1, 0) {
		t.Errorf("black shows below: %08x", g)
	}
	if g := p.get(1, 0); g != makeCell('A', 15, 4, 0) {
		t.Errorf("own background kept: %08x", g)
	}
	// hidden top layer
	a.layers[1].clear = 0
	a.layers[1].hidden = true
	if g := a.seen().get(1, 0); g != makeCell('#', 2, 1, 0) {
		t.Errorf("hidden layer shows: %08x", g)
	}
	// hidden bottom layer: blank under the top
	a.layers[1].hidden, a.layers[0].hidden = false, true
	p = a.seen()
	if g := p.get(0, 0); g != blank {
		t.Errorf("hidden bottom: %08x", g)
	}
	if g := p.get(1, 0); g != makeCell('A', 15, 4, 0) {
		t.Errorf("top over hidden bottom: %08x", g)
	}
	// shownAt agrees with seen everywhere
	a.layers[0].hidden = false
	a.layers[1].clear = layerBlack
	p = a.seen()
	for y := range 2 {
		for x := range 4 {
			if v, _ := a.shownAt(x, y, -1); v != p.get(x, y) {
				t.Errorf("shownAt(%d,%d) %08x, seen %08x", x, y, v, p.get(x, y))
			}
		}
	}
	if _, from := a.shownAt(0, 0, -1); from != 0 {
		t.Errorf("from %d", from)
	}
	if _, from := a.shownAt(1, 0, -1); from != 1 {
		t.Errorf("from %d", from)
	}
}

func TestLayOneLayerIsTheDoc(t *testing.T) {
	a := testApp(t, "pc")
	if a.seen() != a.doc {
		t.Fatal("a page of one layer must be its doc")
	}
	if a.nLayers() != 1 || a.layerCells() != a.doc.cols*a.doc.rows {
		t.Fatal("one layer counts")
	}
}

func TestLayShortLayer(t *testing.T) {
	a := testApp(t, "pc")
	base := newDoc(3, 5)
	for i := range base.cells {
		base.cells[i] = makeCell('x', 7, 0, 0)
	}
	top := newDoc(3, 1)
	top.set(0, 0, makeCell('T', 7, 0, 0))
	a.layers = []Layer{{doc: top}, {doc: base}} // a short layer at the bottom, the tall one current
	a.layers[0].doc, a.layers[1].doc = base, top
	a.lay = 0
	a.doc = base.clone()
	p := a.seen()
	if p.rows != 5 || p.get(0, 0) != makeCell('T', 7, 0, 0) || p.get(0, 4) != makeCell('x', 7, 0, 0) {
		t.Fatalf("short layer: %dx%d %08x %08x", p.cols, p.rows, p.get(0, 0), p.get(0, 4))
	}
}

func TestLayPageTypes(t *testing.T) {
	for _, c := range []struct {
		mode  string
		ok    bool
		black bool
	}{{"pc", true, true}, {"amiga", true, true}, {"uni", true, true}, {"c64", true, false}, {"atari", true, false}, {"st", true, true},
		{"tt", false, false}, {"vd", false, false}, {"mt", false, false}, {"zx", false, false}, {"cpc", false, false}, {"msx", false, false}, {"mz", false, false}} {
		a := testApp(t, c.mode)
		if a.layered() != c.ok || a.blackClears() != c.black {
			t.Errorf("%s: layered %v black %v", c.mode, a.layered(), a.blackClears())
		}
	}
}

func TestLayFlattenGlyph0(t *testing.T) {
	// PETSCII and Atari: character 0 is a glyph, so it covers; a space does not.
	a := testApp(t, "c64")
	base := a.doc.clone()
	base.set(0, 0, makeCell(1, 1, 0, 0))
	base.set(1, 0, makeCell(2, 1, 0, 0))
	top := a.blankLike(base)
	top.set(0, 0, makeCell(0, 3, 0, 0))
	layF2(a, base, top)
	p := a.seen()
	if cellCh(p.get(0, 0)) != 0 || cellCh(p.get(1, 0)) != 2 {
		t.Fatalf("glyph 0: %08x %08x", p.get(0, 0), p.get(1, 0))
	}
	if p.bg != top.bg || !p.glyph0 {
		t.Fatal("page fields from the current layer")
	}
}

func TestLayFlattenUni(t *testing.T) {
	a := testApp(t, "uni")
	base := a.doc.clone()
	base.set(0, 0, uniCell('x', styleOf(200, 100, 0)))
	top := a.doc.clone()
	fillCells(top.cells, uniCell(' ', 7))
	top.set(0, 0, uniCell('é', styleOf(150, 0, italic)))
	layF2(a, base, top)
	if g := a.seen().get(0, 0); g != uniCell('é', styleOf(150, 0, italic)) {
		t.Fatalf("normal: %08x", g)
	}
	a.layers[1].clear = layerBlack
	g := a.seen().get(0, 0)
	fg, bg, fl := styleParts(uniPair(g))
	if uniCode(g) != 'é' || fg != 150 || bg != 100 || fl != italic {
		t.Fatalf("black shows below: %c %d %d %x", uniCode(g), fg, bg, fl)
	}
}

// layFArt draws something on the app's page: a few characters in a few colours.
func layFArt(a *App, seed int) {
	d := a.doc
	for y := range min(d.rows, 6) {
		for x := range min(d.cols, 40) {
			if (x*7+y*3+seed)%4 == 0 {
				continue
			}
			ch, fg, bg := 'A'+(x+y+seed)%26, 1+(x+seed)%7, (y+seed)%3
			if a.c64() || a.atari() {
				ch, bg = 1+(x+y+seed)%26, 0 // the letters' screen codes; one background for the screen
			}
			if a.uni() {
				d.set(x, y, uniCell(int(ch), styleOf(fg, bg, 0)))
			} else {
				d.set(x, y, makeCell(int(ch), fg, bg, 0))
			}
		}
	}
}

// layFSplit makes the app's page two layers that look the same as it does: the
// left half at the bottom, the right half on top (its left half blank).
func layFSplit(a *App) {
	whole := a.doc
	base, top := whole.clone(), a.blankLike(whole)
	top.ensureRows(whole.rows)
	top.rows = whole.rows
	for y := range whole.rows {
		for x := whole.cols / 2; x < whole.cols; x++ {
			if v := whole.get(x, y); !whole.blankAt(v) {
				top.set(x, y, v)
				base.set(x, y, makeCell('?', 3, 2, 0))
			}
		}
	}
	layerFields(top, whole)
	layF2(a, base, top)
}

// Saving a page of two layers gives the bytes its flattened page of one layer gives, in every format.
func TestLaySavesAsSeen(t *testing.T) {
	for _, mode := range []string{"pc", "amiga", "uni", "c64", "atari", "st"} {
		a := testApp(t, mode)
		layFArt(a, len(mode))
		flat := a.doc.clone()
		layFSplit(a)
		if !slices.Equal(a.seen().cells, flat.cells) {
			t.Fatalf("%s: the two layers do not look like the page", mode)
		}
		for _, f := range a.modeFormats() {
			if layersKeptBy(f.id, mode) {
				continue // they keep the layers (TestLayFormatsKeepLayers)
			}
			got, err1 := a.encode(f.id)
			live := a.doc
			ls, lay := a.layers, a.lay
			a.layers, a.lay, a.doc = nil, 0, flat.clone()
			want, err2 := a.encode(f.id)
			a.layers, a.lay, a.doc = ls, lay, live
			if (err1 == nil) != (err2 == nil) || !bytes.Equal(got, want) {
				t.Errorf("%s %s: layered %d bytes (%v), flat %d bytes (%v)", mode, f.id, len(got), err1, len(want), err2)
			}
			if a.doc != live || a.seeing || a.layers[a.lay].doc == live {
				t.Fatalf("%s %s: asSeen did not put the layer back", mode, f.id)
			}
		}
	}
}

func TestLayRefusals(t *testing.T) {
	a := testApp(t, "pc")
	layFArt(a, 1)
	layFSplit(a)
	a.newFrame()
	if a.frames != nil || !strings.Contains(a.message, "Merge the layers") {
		t.Fatalf("a new frame on a layered page: %v %q", a.frames != nil, a.message)
	}
	if err := a.collabHost(0, false, "x", ""); err == nil || !strings.Contains(err.Error(), "layers") {
		t.Fatalf("hosting a layered page: %v", err)
	}
}

func TestLayStagesSeeLayers(t *testing.T) {
	a := testApp(t, "pc")
	layFArt(a, 2)
	layFSplit(a)
	before := a.seen()
	a.pushUndo() // step 0 -> 1: the log keeps the page before the edit
	a.doc.set(0, 0, makeCell('Z', 1, 0, 0))
	a.pushUndo()
	if a.stages == nil || len(a.stages.Pics) == 0 {
		t.Fatal("no stage recorded")
	}
	if got := (stageCand{pic: &a.stages.Pics[0]}).page(); got == nil || !slices.Equal(got.cells, before.cells) {
		t.Fatal("the recorded stage is not the page as seen")
	}
	cands := a.stageCands()
	if last := cands[len(cands)-1].page(); !slices.Equal(last.cells, a.seen().cells) {
		t.Fatal("the stage now is not the page as seen")
	}
	if first := cands[0].page(); !slices.Equal(first.cells, before.cells) {
		t.Fatal("an undo step's stage is not its page as seen")
	}
}

func TestLayEditAndUndo(t *testing.T) {
	a := testApp(t, "pc")
	a.doc.set(0, 0, makeCell('a', 1, 2, 0))
	a.pushUndo()
	a.newLayer(false)
	if a.nLayers() != 2 || a.lay != 1 || a.doc.rows != 25 {
		t.Fatalf("new layer: %d layers, at %d, %d rows", a.nLayers(), a.lay, a.doc.rows)
	}
	a.pushUndo()
	a.doc.set(1, 0, makeCell('b', 3, 4, 0))
	if p := a.seen(); p.get(0, 0) != makeCell('a', 1, 2, 0) || p.get(1, 0) != makeCell('b', 3, 4, 0) {
		t.Fatal("both layers seen")
	}
	a.goLayer(0)
	a.pushUndo()
	a.doc.set(2, 0, makeCell('c', 5, 0, 0))
	a.goLayer(1)
	// undo the edit of layer 0: goes there
	a.doUndo()
	if a.lay != 0 || a.doc.get(2, 0) == makeCell('c', 5, 0, 0) {
		t.Fatalf("undo of layer 0's edit: at %d, %08x", a.lay, a.doc.get(2, 0))
	}
	if a.layers[1].doc.get(1, 0) != makeCell('b', 3, 4, 0) {
		t.Fatal("layer 1 lost its cell")
	}
	a.doRedo()
	if a.lay != 0 || a.doc.get(2, 0) != makeCell('c', 5, 0, 0) {
		t.Fatal("redo")
	}
	// undo everything back to one layer
	for len(a.undo) > 0 {
		a.doUndo()
	}
	if a.layers != nil || a.doc.get(0, 0) != makeCell('a', 1, 2, 0) {
		t.Fatalf("undone to the start: layers %v", a.layers != nil)
	}
	for len(a.redo) > 0 {
		a.doRedo()
	}
	if a.nLayers() != 2 || a.seen().get(1, 0) != makeCell('b', 3, 4, 0) || a.seen().get(2, 0) != makeCell('c', 5, 0, 0) {
		t.Fatal("redone")
	}
}

func TestLayStructure(t *testing.T) {
	a := testApp(t, "pc")
	a.doc.set(0, 0, makeCell('a', 1, 0, 0))
	a.newLayer(false)
	a.doc.set(1, 1, makeCell('b', 2, 0, 0))
	a.newLayer(true) // a copy of layer 2
	if a.nLayers() != 3 || a.layerName(2) != "Layer 2 copy" || a.doc.get(1, 1) != makeCell('b', 2, 0, 0) {
		t.Fatalf("copy: %d %q", a.nLayers(), a.layerName(2))
	}
	a.doc.set(1, 1, makeCell('c', 3, 0, 0))
	a.moveLayer(-1)
	if a.lay != 1 || a.seen().get(1, 1) != makeCell('b', 2, 0, 0) {
		t.Fatalf("move down: at %d, %08x", a.lay, a.seen().get(1, 1))
	}
	a.toggleLayer(2)
	if a.seen().get(1, 1) != makeCell('c', 3, 0, 0) {
		t.Fatal("hide the top")
	}
	a.toggleLayer(2)
	a.goLayer(2)
	a.mergeDown()
	if a.nLayers() != 2 || a.lay != 1 || a.doc.get(1, 1) != makeCell('b', 2, 0, 0) {
		t.Fatalf("merge down: %d at %d %08x", a.nLayers(), a.lay, a.doc.get(1, 1))
	}
	// width, height, rows and columns go to every layer
	a.setCols(60)
	if a.doc.cols != 60 || a.layers[0].doc.cols != 60 {
		t.Fatal("width of every layer")
	}
	a.cx, a.cy = 0, 0
	a.insertRow()
	if a.seen().get(0, 1) != makeCell('a', 1, 0, 0) || a.seen().get(1, 2) != makeCell('b', 2, 0, 0) {
		t.Fatal("insert row in every layer")
	}
	a.doUndo()
	if a.seen().get(0, 0) != makeCell('a', 1, 0, 0) || a.seen().get(1, 1) != makeCell('b', 2, 0, 0) {
		t.Fatal("insert row: one undo step")
	}
	a.insertColumn()
	if a.seen().get(1, 0) != makeCell('a', 1, 0, 0) || a.seen().get(2, 1) != makeCell('b', 2, 0, 0) {
		t.Fatal("insert column in every layer")
	}
	a.setRows(1)
	if a.doc.rows != 1 || a.layers[0].doc.rows != 1 {
		t.Fatal("height of every layer")
	}
	a.doUndo()
	if a.doc.rows != 25 || a.seen().get(2, 1) != makeCell('b', 2, 0, 0) {
		t.Fatal("height undone")
	}
	a.deleteLayer()
	if a.layers != nil || a.doc.get(1, 0) != makeCell('a', 1, 0, 0) {
		t.Fatal("deleting down to one layer: a page of one layer")
	}
	a.doUndo()
	a.mergeAll()
	if a.layers != nil || a.doc.get(2, 1) != makeCell('b', 2, 0, 0) {
		t.Fatal("merge all")
	}
}

func TestLayLimits(t *testing.T) {
	a := testApp(t, "pc")
	a.setRows(maxRowsFor(80)) // 80 x 100000: 8M cells
	a.newLayer(false)
	if a.nLayers() != 2 {
		t.Fatalf("a second full layer fits: %q", a.message)
	}
	a.newLayer(false)
	if a.nLayers() != 2 || !strings.Contains(a.message, "larger than the largest page") {
		t.Fatalf("a third: %d %q", a.nLayers(), a.message)
	}
	b := testApp(t, "pc")
	for range maxLayers + 3 {
		b.newLayer(false)
	}
	if b.nLayers() != maxLayers {
		t.Fatalf("%d layers", b.nLayers())
	}
	c := testApp(t, "pc")
	c.setCols(200)
	c.setRows(5)
	c.newLayer(false)
	c.goLayer(0)
	if c.maxRows() != (maxPageCells-200*1)/200 { // the empty layer above waits as one row
		t.Fatalf("maxRows %d", c.maxRows())
	}
	c.setMode("tt")
	if c.mode != "pc" || !strings.Contains(c.message, "Merge the layers") {
		t.Fatal("a teletext page has one layer")
	}
	d := testApp(t, "tt")
	d.newLayer(false)
	if d.layers != nil {
		t.Fatal("teletext layers")
	}
}

func TestLayShortLayersCostLittle(t *testing.T) {
	a := testApp(t, "pc")
	a.setRows(1000)
	a.newLayer(false)
	a.doc.set(0, 0, makeCell('x', 1, 0, 0))
	a.goLayer(0)
	if r := a.layers[1].doc.rows; r != 1 {
		t.Fatalf("a short layer keeps %d rows", r)
	}
	a.goLayer(1)
	if a.doc.rows != 1000 || a.seen().rows != 1000 {
		t.Fatal("the current layer is as tall as the page")
	}
}

func TestLayUniStylesKept(t *testing.T) {
	a := testApp(t, "uni")
	p := styleOf(201, 77, italic)
	a.doc.set(0, 0, uniCell('q', p))
	a.newLayer(false)
	a.goLayer(0)
	a.goLayer(1)
	a.toggleLayer(0)
	// fill the style table so that uniCompact runs
	for i := 0; layFStyleCount() < (uniPairMax-256)*3/4; i++ {
		styleOf(16+i%240, 16+i/240%240, uint32(i/(240*240))*italic)
	}
	if a.uniCompact() == 0 {
		t.Fatal("nothing compacted")
	}
	fg, bg, fl := styleParts(uniPair(a.layers[0].doc.get(0, 0)))
	if fg != 201 || bg != 77 || fl != italic {
		t.Fatalf("a hidden layer's style: %d %d %x", fg, bg, fl)
	}
}

func layFStyleCount() int {
	uniStyles.RLock()
	defer uniStyles.RUnlock()
	return len(uniStyles.list)
}

// layFDraw draws the page into a fresh picture and gives its pixels.
func layFDraw(a *App) []uint32 {
	u := &UI{fb: &FB{}}
	u.fb.resize(500, 300)
	a.page = Rect{0, 0, 480, 280}
	a.blink, a.start = false, false
	a.drawPage(u, "0")
	return slices.Clone(u.fb.px)
}

// A page of layers is drawn as its flattened page of one layer is; dim and hidden others differ.
func TestLayDrawn(t *testing.T) {
	for _, mode := range []string{"pc", "amiga", "uni", "atari", "st"} { // (PETSCII needs the ROMs, which CI has not)
		a := testApp(t, mode)
		layFArt(a, 3)
		flat := layFDraw(a)
		layFSplit(a)
		a.layers[1].clear = 0
		if got := layFDraw(a); !slices.Equal(got, flat) {
			t.Errorf("%s: two layers are not drawn as the page", mode)
		}
		a.others = 1
		dim := layFDraw(a)
		a.others = 2
		only := layFDraw(a)
		a.others = 0
		if slices.Equal(dim, flat) || slices.Equal(only, flat) || slices.Equal(dim, only) {
			t.Errorf("%s: dim / hidden others look the same as shown", mode)
		}
	}
}

func TestLayPickAndCopySeen(t *testing.T) {
	a := testApp(t, "pc")
	a.doc.set(0, 0, makeCell('a', 5, 3, 0))
	a.newLayer(false)
	a.doc.set(1, 0, makeCell('b', 6, 0, 0))
	a.pick(0, 0)
	if a.brush != 'a' || a.fg != 5 {
		t.Fatalf("pick what is seen: %c %d", a.brush, a.fg)
	}
	a.sel = &Sel{0, 0, 1, 0}
	a.copySel(false)
	if a.clip.cells[0] != blank || a.clip.cells[1] != makeCell('b', 6, 0, 0) {
		t.Fatal("Copy copies the current layer")
	}
	a.copySeen()
	if a.clip.cells[0] != makeCell('a', 5, 3, 0) || a.clip.cells[1] != makeCell('b', 6, 0, 0) {
		t.Fatal("Copy what is seen")
	}
}

// The panel's layers: New on a page of one layer, then a row a layer; a click on a row works on it.
func TestLayPanel(t *testing.T) {
	a := testApp(t, "pc")
	u := &UI{fb: &FB{}}
	u.fb.resize(400, 400)
	u.fb.clip = Rect{0, 0, 400, 400}
	x, w := 10, 300
	u.click = &Click{x: x + w - 5, y: 15, button: 1}
	a.drawLayers(u, x, 10, w)
	if a.nLayers() != 2 || a.lay != 1 {
		t.Fatalf("New: %d layers", a.nLayers())
	}
	// rows: the top layer (1) first, then layer 0
	u.click = &Click{x: x + checkW("") + 20, y: 10 + gadH + 2 + 5, button: 1}
	a.drawLayers(u, x, 10, w)
	if a.lay != 0 {
		t.Fatalf("a click on the second row: layer %d", a.lay)
	}
	u.click = &Click{x: x + 5, y: 10 + 5, button: 1} // the top row's box: hide layer 1
	a.drawLayers(u, x, 10, w)
	if !a.layers[1].hidden {
		t.Fatal("the box hides the layer")
	}
	u.click = nil
	if y := a.drawLayers(u, x, 10, w); y <= 10+2*gadH {
		t.Fatalf("the rows and buttons end at %d", y)
	}
}

// layFStack makes a page of three layers: art at the bottom, a short layer, a hidden one with black shows below.
func layFStack(a *App) {
	layFArt(a, 4)
	a.newLayer(false)
	a.doc.set(3, 0, makeCell('M', 14, 0, 0))
	a.renameLayer(1, "Letters")
	a.newLayer(false)
	a.doc.set(5, 1, makeCell('H', 9, 0, 0))
	a.toggleBlackClear()
	a.toggleLayer(2)
	a.goLayer(1)
}

// layFSame: two tabs have the same layers.
func layFSame(t *testing.T, got, want *Tab) {
	t.Helper()
	if got.nLayers() != want.nLayers() || got.lay != want.lay {
		t.Fatalf("%d layers at %d, want %d at %d", got.nLayers(), got.lay, want.nLayers(), want.lay)
	}
	for i := range want.nLayers() {
		g, w := got.layers[i], want.layers[i]
		if g.name != w.name || g.hidden != w.hidden || g.clear != w.clear {
			t.Errorf("layer %d: %+v, want %+v", i, g, w)
		}
		gd, wd := got.layerDoc(i), want.layerDoc(i)
		for y := range max(gd.rows, wd.rows) {
			for x := range wd.cols {
				if gd.get(x, y) != wd.get(x, y) && !(wd.blankAt(gd.get(x, y)) && wd.blankAt(wd.get(x, y))) {
					t.Fatalf("layer %d cell %d,%d: %08x, want %08x", i, x, y, gd.get(x, y), wd.get(x, y))
				}
			}
		}
	}
	if !sameLook(got.seen(), want.seen()) {
		t.Fatal("the pages do not look the same")
	}
}

func TestLayAutosave(t *testing.T) {
	for _, mode := range []string{"pc", "uni", "c64"} {
		a := testApp(t, mode)
		layFStack(a)
		a.path, a.name = "/nowhere/art.ans", "art.ans"
		if err := writeAutosave(a.Tab); err != nil {
			t.Fatal(err)
		}
		want := a.Tab
		b := restart(t, mode)
		files := leftAutosaves()
		if len(files) != 1 {
			t.Fatalf("%d autosaves", len(files))
		}
		if err := b.restoreAutosave(files[0]); err != nil {
			t.Fatal(err)
		}
		layFSame(t, b.Tab, want)
		removeAutosave(b.autoID)
	}
}

func TestLaySideFile(t *testing.T) {
	a := testApp(t, "pc")
	layFStack(a)
	want := a.Tab
	p := filepath.Join(t.TempDir(), "art.ans")
	if !a.writeOut(p, "ansi") || a.dirty {
		t.Fatalf("saved: dirty %v %q", a.dirty, a.message)
	}
	if !strings.Contains(a.message, "3 layers put together") || !strings.Contains(a.message, "1 hidden") {
		t.Errorf("message %q", a.message)
	}
	b := restart(t, "pc")
	b.openPath(p, "auto")
	layFSame(t, b.Tab, want)
	// the file changed elsewhere: it opens as one layer
	data, _ := os.ReadFile(p)
	os.WriteFile(p, append([]byte("x"), data...), 0o644)
	c := restart(t, "pc")
	c.openPath(p, "auto")
	if c.layers != nil {
		t.Fatal("a changed file got its layers back")
	}
	// CharView shows the file as it is
	os.WriteFile(p, data, 0o644)
	viewer = &Viewer{}
	v := restart(t, "pc")
	v.openPath(p, "auto")
	viewer = nil
	if v.layers != nil {
		t.Fatal("CharView took the layers")
	}
	// saved again with one layer: the side file goes
	os.WriteFile(p, data, 0o644)
	d := restart(t, "pc")
	d.openPath(p, "auto")
	d.mergeAll()
	d.writeOut(p, "ansi")
	e := restart(t, "pc")
	e.openPath(p, "auto")
	if e.layers != nil {
		t.Fatal("a page saved as one layer got layers back")
	}
}

// layFFormat saves the app's page of layers as fmtID and opens it again in a new app; it gives the new app.
func layFFormat(t *testing.T, a *App, fmtID, ext string) *App {
	t.Helper()
	p := filepath.Join(t.TempDir(), "art"+ext)
	if !a.writeOut(p, fmtID) {
		t.Fatalf("%s: %q", fmtID, a.message)
	}
	b := restart(t, a.mode)
	os.RemoveAll(layersDir()) // the format itself must keep them
	b.openPath(p, "auto")
	return b
}

func TestLayFormatsKeepLayers(t *testing.T) {
	for _, c := range []struct{ mode, fmtID, ext string }{{"pc", "icy", ".icy"}, {"pc", "xp", ".xp"}, {"pc", "psci", ".psci"}, {"amiga", "psci", ".psci"}} { // (IcyDraw files open as PC pages)
		a := testApp(t, c.mode)
		layFArt(a, 5)
		a.newLayer(false)
		a.doc.set(3, 0, makeCell('M', 14, 0, 0))
		a.doc.set(4, 0, makeCell('N', 14, 5, 0))
		a.renameLayer(1, "Letters")
		if c.fmtID != "xp" {
			a.newLayer(false)
			a.doc.set(5, 1, makeCell('H', 9, 0, 0))
			a.toggleBlackClear()
			a.newLayer(false)
			a.doc.set(6, 2, makeCell('Q', 9, 0, 0))
			a.toggleLayer(3)
		}
		a.goLayer(1)
		want := a.Tab
		b := layFFormat(t, a, c.fmtID, c.ext)
		if b.mode != c.mode {
			t.Fatalf("%s %s: opened as %s", c.mode, c.fmtID, b.mode)
		}
		if b.nLayers() != want.nLayers() {
			t.Fatalf("%s %s: %d layers, want %d (%q)", c.mode, c.fmtID, b.nLayers(), want.nLayers(), b.message)
		}
		for i := range want.layers {
			g, w := b.layers[i], want.layers[i]
			if g.hidden != w.hidden || g.clear != w.clear || (c.fmtID != "xp" && b.layerName(i) != want.layerName(i)) {
				t.Errorf("%s %s layer %d: hidden %v clear %d name %q, want %v %d %q", c.mode, c.fmtID, i, g.hidden, g.clear, b.layerName(i), w.hidden, w.clear, want.layerName(i))
			}
		}
		if !sameLook(b.seen(), want.seen()) {
			t.Errorf("%s %s: the page does not look the same", c.mode, c.fmtID)
		}
	}
}

// REXPaint keeps no hidden layers and no see-through backgrounds: left out, and the colour below.
func TestLayXPLeavesOut(t *testing.T) {
	a := testApp(t, "pc")
	a.doc.set(0, 0, makeCell('a', 1, 4, 0))
	a.newLayer(false)
	a.doc.set(0, 0, makeCell('b', 2, 0, 0))
	a.toggleBlackClear()
	a.newLayer(false)
	a.doc.set(1, 0, makeCell('c', 3, 0, 0))
	a.toggleLayer(2)
	seen := a.seen()
	b := layFFormat(t, a, "xp", ".xp")
	if b.nLayers() != 2 || b.seen().get(0, 0) != makeCell('b', 2, 4, 0) || !sameLook(b.seen(), seen) {
		t.Fatalf("%d layers, %08x", b.nLayers(), b.seen().get(0, 0))
	}
}

// A file whose layers CharEdit cannot show as they are opens as one layer, looking as before.
func TestLayFileFallsBack(t *testing.T) {
	a := testApp(t, "pc")
	a.doc.set(0, 0, makeCell('a', 1, 4, 0))
	a.newLayer(false)
	a.doc.set(0, 0, makeCell('b', 2, 0, 0))
	data, _ := a.encode("icy")
	doc, err := parseIcyDraw(data)
	if err != nil {
		t.Fatal(err)
	}
	// a visible black space on the top layer covers what is below in IcyDraw
	doc.layers[1].cells[0] = icyCell{ch: ' ', fg: icyColour{'p', 7}, bg: icyColour{'p', 0}, visible: true}
	flat, _, _ := icyPage(doc)
	if icyFileLayers(doc, flat) != nil {
		t.Fatal("kept layers that do not look the same")
	}
	// the real IcyDraw file of one layer: one layer
	b := testApp(t, "pc")
	b.openPath("testdata/icydraw/icydraw-alove.icy", "auto")
	if b.layers != nil || b.mode != "pc" {
		t.Fatal("a file of one layer")
	}
	// CharView never keeps layers
	p := filepath.Join(t.TempDir(), "two.icy")
	os.WriteFile(p, data, 0o644)
	viewer = &Viewer{}
	defer func() { viewer = nil }()
	c := testApp(t, "pc")
	c.openPath(p, "auto")
	if c.layers != nil {
		t.Fatal("CharView kept layers")
	}
}

// An Atari page saved as ATASCII: put together, the layers kept beside the file.
func TestLayAtariSideFile(t *testing.T) {
	if layersKeptBy("psci", "c64") || !layersKeptBy("psci", "pc") {
		t.Fatal("Playscii keeps the layers of PC pages, not of PETSCII pages")
	}
	a := testApp(t, "atari")
	layFArt(a, 6)
	a.newLayer(false)
	a.doc.set(3, 0, makeCell(13, 0, 0, 0))
	want := a.Tab
	p := filepath.Join(t.TempDir(), "art.ata")
	if !a.writeOut(p, "ata") {
		t.Fatal(a.message)
	}
	b := restart(t, "atari")
	b.openPath(p, "auto")
	layFSame(t, b.Tab, want)
}

// layFPage is an app (no test's: the fuzz target's seeds) with a PC page of two layers.
func layFPage() *App {
	keep := a
	defer func() { a = keep }()
	a = &App{tool: "draw", fg: 7, brush: 0x2F, charHover: -1, layout: "one", defZoom: 2,
		defFont: map[string]string{}, defPalette: map[string]string{}, fkeys: defaultFkeys(), baud: 14400}
	a.focusTab(a.newTab("pc"))
	a.doc.set(0, 0, makeCell('a', 1, 4, 0))
	a.newLayer(false)
	a.doc.set(1, 0, makeCell('b', 2, 0, 0))
	a.doc.set(2, 0, makeCell('c', 3, 0, 0))
	a.toggleBlackClear()
	return a
}

// CHARCOAL_LAYER_FILES=f1,f2,...: real files' layers (kept or not) and their look.
func TestLayRealFiles(t *testing.T) {
	list := os.Getenv("CHARCOAL_LAYER_FILES")
	if list == "" {
		t.Skip("CHARCOAL_LAYER_FILES not set")
	}
	for _, p := range strings.Split(list, ",") {
		a := testApp(t, "pc")
		a.openPath(p, "auto")
		t.Logf("%s: %s page, %d layers, %q", filepath.Base(p), a.mode, a.nLayers(), a.message)
	}
}

// IcyDraw's engine reads CharEdit's layered .icy as the page is seen (ccicy dump: the cells as IcyDraw
// puts its layers together). CHARCOAL_ICYMACH / CHARCOAL_ICYDIR as for TestOthIcyExternal.
func TestLayIcyDrawReads(t *testing.T) {
	cmd, dir := os.Getenv("CHARCOAL_ICYMACH"), os.Getenv("CHARCOAL_ICYDIR")
	if cmd == "" || dir == "" {
		t.Skip("CHARCOAL_ICYMACH / CHARCOAL_ICYDIR not set")
	}
	os.MkdirAll(dir, 0o755)
	a := testApp(t, "pc")
	layFArt(a, 7)
	a.newLayer(false)
	for x := range 30 {
		a.doc.set(x, 2, makeCell('A'+x%26, 14, 0, 0)) // letters on black over the art: black covers
	}
	a.newLayer(false)
	for x := range 30 {
		a.doc.set(x, 3, makeCell('a'+x%26, 11, 0, 0)) // black shows below
		a.doc.set(x, 4, makeCell('#', 12, 1, 0))
	}
	a.toggleBlackClear()
	a.newLayer(false)
	a.doc.set(0, 0, makeCell('Z', 15, 4, 0)) // hidden
	a.toggleLayer(3)
	path := filepath.Join(dir, "lay-stack.icy")
	if !a.writeOut(path, "icy") {
		t.Fatal(a.message)
	}
	args := append(strings.Fields(cmd), path)
	out, err := exec.Command(args[0], args[1:]...).Output()
	if err != nil {
		t.Fatal(err)
	}
	dump := othFReadDump(t, string(out))
	p := a.seen()
	if dump.w != p.cols || dump.h != p.rows {
		t.Fatalf("IcyDraw reads %dx%d", dump.w, dump.h)
	}
	bad := 0
	for i, c := range dump.cells {
		v := p.cells[i]
		at := cellAttr(v)
		want := [3]string{fmt.Sprintf("%x", cellCh(v)), fmt.Sprintf("p%d", at&15), fmt.Sprintf("p%d", at>>4)}
		if c[0] != want[0] || c[1] != want[1] || c[2] != want[2] {
			if bad++; bad < 5 {
				t.Errorf("cell %d,%d: IcyDraw %v, CharEdit %v", i%p.cols, i/p.cols, c[:3], want)
			}
		}
	}
	if bad > 0 {
		t.Fatalf("%d cells differ", bad)
	}
}
