package main

import (
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
