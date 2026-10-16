package main

import (
	"bytes"
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
		for x := range min(d.cols, 20) {
			if (x*7+y*3+seed)%4 == 0 {
				continue
			}
			ch, fg, bg := 'A'+(x+y+seed)%26, 1+(x+seed)%7, (y+seed)%3
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
	a.blink = false
	a.drawPage(u, "0")
	return slices.Clone(u.fb.px)
}

// A page of layers is drawn as its flattened page of one layer is; dim and hidden others differ.
func TestLayDrawn(t *testing.T) {
	for _, mode := range []string{"pc", "c64", "uni", "atari", "st"} {
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
