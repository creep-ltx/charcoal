package main

// Layers (docs/layers-design.md): a page may be several pictures on top of one
// another. The tab's doc is always the layer being worked on, so every tool
// draws on it unchanged; the others wait in Tab.layers and are never changed in
// place, so undo steps may share them (as frames). nil is one layer, the doc:
// such a page is, in every byte, a page without layers.
//
// A cell of a layer above the bottom one lets what is below show when it is
// blank (Doc.blankAt: the empty cell of the page type, the same idea as Shift+click
// stamping). A layer whose clear is 1 also lets the background below show under
// its cells of background 0 ("black shows below"), for letters over a picture.

import (
	"fmt"
	"slices"
)

// Layer is one picture of a page of several on top of one another.
type Layer struct {
	doc    *Doc // stale while the layer is the current one: Tab.doc is it then
	name   string
	hidden bool
	clear  int        // which cells let the layers below show: 0 blank cells, 1 also a background 0
	keep   *layerKeep // what the file's layer had that CharEdit does not use, written back unchanged
}

// layerKeep is what a layer's file had that CharEdit keeps but does not use.
type layerKeep struct {
	IcyFlags  uint32  `json:"icy_flags,omitempty"`  // IcyDraw's layer flags (lock bits and the like)
	IcyColour [4]byte `json:"icy_colour,omitempty"` // IcyDraw's layer colour (RGBA)
	PsciZ     float64 `json:"psci_z,omitempty"`     // Playscii's z
}

const (
	maxLayers   = 64
	layerNormal = 0 // blank cells show what is below
	layerBlack  = 1 // also a background 0 shows the background below
)

// layered: the page type can have layers. Not teletext, Viewdata and Minitel (an
// attribute acts on the rest of its row), not yet the machines whose empty cell is
// not blank (Spectrum, CPC, MSX, MZ-700), not pages with a picture under the cells.
func (a *App) layered() bool {
	switch a.mode {
	case "pc", "amiga", "uni", "c64", "atari", "st":
		return !a.igsPage() && !a.skyPage()
	}
	return false
}

// blackClears: the page's cells have backgrounds of their own, so "black shows
// below" means something.
func (a *App) blackClears() bool {
	switch a.mode {
	case "pc", "amiga", "uni", "st":
		return true
	}
	return false
}

func (t *Tab) nLayers() int {
	if t.layers == nil {
		return 1
	}
	return len(t.layers)
}

// layerDoc is layer i's picture (the live one for the current layer).
func (t *Tab) layerDoc(i int) *Doc {
	if t.layers == nil || i == t.lay {
		return t.doc
	}
	return t.layers[i].doc
}

// layerCells counts the cells of all layers (the live current one included).
func (t *Tab) layerCells() int {
	n := 0
	for i := range t.nLayers() {
		d := t.layerDoc(i)
		n += d.cols * d.rows
	}
	return n
}

// layerFields gives dst the page's fields (screen colour, border ...) of src: they
// live on the current layer, the other layers' copies are stale.
func layerFields(dst, src *Doc) {
	dst.bg, dst.border, dst.ink, dst.glyph0, dst.uni, dst.fullest = src.bg, src.border, src.ink, src.glyph0, src.uni, src.fullest
}

// mergeCell puts v (a cell of a layer above, clear its mode) over under; ok is
// false when v lets under show unchanged. page is the page (for blankAt).
func mergeCell(page *Doc, under, v uint32, clear int) (uint32, bool) {
	if page.blankAt(v) {
		return under, false
	}
	if clear != layerBlack {
		return v, true
	}
	if page.uni {
		fg, bg, fl := styleParts(uniPair(v))
		if bg != 0 {
			return v, true
		}
		_, ubg, _ := styleParts(uniPair(under))
		return uniWithPair(v, styleOf(fg, ubg, fl)), true
	}
	if cellBg(v) != 0 {
		return v, true
	}
	return v&^0xF000 | under&0xF000, true
}

// flatten is the page as it is seen and saved: the visible layers put together,
// the bottom one covering everything (a hidden bottom layer leaves blank cells).
// d is the live current layer (layer cur).
func flatten(ls []Layer, cur int, d *Doc) *Doc {
	out := newDoc(d.cols, d.rows)
	layerFields(out, d)
	for i, l := range ls {
		if l.hidden {
			continue
		}
		ld := l.doc
		if i == cur {
			ld = d
		}
		rows := min(ld.rows, out.rows)
		if i == 0 {
			for y := range rows {
				w := min(ld.cols, out.cols)
				copy(out.cells[y*out.cols:y*out.cols+w], ld.cells[y*ld.cols:y*ld.cols+w])
			}
			continue
		}
		for y := range rows {
			for x := range min(ld.cols, out.cols) {
				k := y*out.cols + x
				if v, ok := mergeCell(out, out.cells[k], ld.cells[y*ld.cols+x], l.clear); ok {
					out.cells[k] = v
				}
			}
		}
	}
	return out
}

// seen is the page as it is seen and saved: the doc itself (the same pointer)
// when it has one layer, else its layers put together. Everything that writes,
// exports, checks or animates the whole page reads it through here.
func (t *Tab) seen() *Doc {
	if t.layers == nil {
		return t.doc
	}
	return flatten(t.layers, t.lay, t.doc)
}

// shownAt is the cell seen at x, y and the layer it comes from (-1: none, a blank
// cell of a hidden bottom layer). only >= 0 shows only that layer.
func (t *Tab) shownAt(x, y, only int) (uint32, int) {
	if t.layers == nil {
		return t.doc.get(x, y), 0
	}
	v, from := blank, -1
	for i, l := range t.layers {
		if only >= 0 && i != only || only < 0 && l.hidden {
			continue
		}
		c := t.layerDoc(i).get(x, y)
		if i == 0 {
			v, from = c, 0
			continue
		}
		if m, ok := mergeCell(t.doc, v, c, l.clear); ok {
			v, from = m, i
		}
	}
	return v, from
}

// layerName is the name a layer shows (its own, else "Layer n").
func (t *Tab) layerName(i int) string {
	if t.layers != nil && i < len(t.layers) && t.layers[i].name != "" {
		return t.layers[i].name
	}
	return fmt.Sprintf("Layer %d", i+1)
}

// cloneLayers is a copy of the list for an undo step, the current layer's doc
// being d (a clone of the live one).
func (t *Tab) cloneLayers(d *Doc) []Layer {
	if t.layers == nil {
		return nil
	}
	ls := slices.Clone(t.layers)
	ls[t.lay].doc = d
	return ls
}
