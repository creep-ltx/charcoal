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
