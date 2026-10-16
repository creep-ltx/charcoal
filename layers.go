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
	"strings"
)

// Layer is one picture of a page of several on top of one another.
type Layer struct {
	doc    *Doc // stale while the layer is the current one: Tab.doc is it then
	name   string
	hidden bool
	locked bool       // no tool changes it (IcyDraw's edit lock)
	clear  int        // which cells let the layers below show: 0 blank cells, 1 also a background 0
	keep   *layerKeep // what the file's layer had that CharEdit does not use, written back unchanged
}

// layerKeep is what a layer's file had that CharEdit keeps but does not use.
type layerKeep struct {
	IcyFlags  uint32  `json:"icy_flags,omitempty"`  // IcyDraw's layer flags (position and alpha locks and the like; visible and edit lock are the layer's own)
	IcyColour [4]byte `json:"icy_colour,omitempty"` // IcyDraw's layer colour (RGBA)
	PsciZ     float64 `json:"psci_z,omitempty"`     // Playscii's z
}

const (
	icyVisible  = 1 // IcyDraw's layer flags (icy_engine formats/io/icy_draw.rs: IS_VISIBLE, EDIT_LOCK)
	icyEditLock = 4
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
	if t.layers == nil || i == t.lay && !t.seeing {
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
	wideNormal(out) // a wide character half covered by a layer above: a space, as a terminal would show it (wide.go)
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

// asSeen puts the page as it is seen in the tab's doc until the function it
// gives is called: code that writes, exports or checks the whole page reads
// a.doc, and so gets the layers put together (`defer a.asSeen()()`). The live
// current layer waits in its place in the list meanwhile, so the layered writers
// still find every layer. A page of one layer is left as it is.
func (t *Tab) asSeen() func() {
	if t.layers == nil || t.seeing {
		return func() {}
	}
	live, kept := t.doc, t.layers[t.lay].doc
	t.layers[t.lay].doc = live
	t.doc, t.seeing = flatten(t.layers, t.lay, live), true
	return func() {
		t.doc, t.seeing = live, false
		if t.lay < len(t.layers) {
			t.layers[t.lay].doc = kept
		}
	}
}

// shownCell is the cell seen at x, y.
func (t *Tab) shownCell(x, y int) uint32 {
	v, _ := t.shownAt(x, y, -1)
	return v
}

// layersNoFrames is why a layered page gets no frames (layers and frames together: later).
const layersNoFrames = "Merge the layers first (Page > Merge all layers): frames are for pages of one layer"

// ---------------------------------------------------------------- working on layers

// layerMode: pages of this type can have layers (see layered).
func layerMode(m string) bool {
	switch m {
	case "pc", "amiga", "uni", "c64", "atari", "st":
		return true
	}
	return false
}

// layerRefused says why the page cannot get another layer ("" it can).
func (a *App) layerRefused() string {
	switch {
	case !a.layered():
		return "Layers are for PC, Amiga, Unicode, PETSCII, Atari and Atari ST pages"
	case a.frames != nil:
		return "Layers are for pages of one picture: this page has frames"
	case collab != nil && collab.tab == a.Tab:
		return "Layers cannot be shared (Moebius pages are one picture): end the drawing session first"
	case a.nLayers() >= maxLayers:
		return fmt.Sprintf("No more layers: %d is the most", maxLayers)
	case a.layerCells()+a.doc.cols*a.doc.rows > maxPageCells:
		return "No more layers: all of them together would be larger than the largest page"
	}
	return ""
}

// ensureLayers makes the one picture of a page a layer list of one.
func (t *Tab) ensureLayers() {
	if t.layers == nil {
		t.layers, t.lay = []Layer{{doc: t.doc}}, 0
	}
}

// oneLayer: a list of one layer is no list (a page of one layer is its doc).
func (t *Tab) oneLayer() {
	if t.layers != nil && len(t.layers) == 1 {
		t.layers, t.lay = nil, 0
	}
}

// shrinkLayer is a layer's picture as it waits while not current: a layer above
// the bottom one without its blank rows at the end (they are clear anyway), so a
// short layer over a tall picture costs little. The bottom layer is kept whole:
// its blank cells are seen.
func shrinkLayer(d *Doc, i int) *Doc {
	u := max(1, d.usedRows())
	if i == 0 || u*2 > d.rows {
		return d
	}
	c := &Doc{cols: d.cols, rows: u, cells: make([]uint32, u*d.cols)}
	layerFields(c, d)
	copy(c.cells, d.cells[:u*d.cols])
	return c
}

// leaveLayer puts the live current layer back into the list.
func (a *App) leaveLayer() {
	a.layers[a.lay].doc = shrinkLayer(a.doc, a.lay)
}

// liveLayer makes a clone of layer i the doc, as tall as the page (rows) and
// with the page's fields of from.
func (a *App) liveLayer(i, rows int, from *Doc) {
	a.lay = i
	d := a.layers[i].doc.clone()
	d.ensureRows(rows)
	layerFields(d, from)
	a.doc = d
}

// goLayer makes layer i the one worked on.
func (a *App) goLayer(i int) {
	if a.layers == nil || i < 0 || i >= len(a.layers) {
		return
	}
	a.endPlay()
	if i != a.lay {
		cur := a.doc
		a.leaveLayer()
		a.liveLayer(i, cur.rows, cur)
	}
	a.float, a.moving, a.inRun = nil, false, false
	a.layerMsg()
}

func (a *App) layerMsg() {
	if a.layers == nil {
		a.msg("One layer")
		return
	}
	s := fmt.Sprintf("Layer %d of %d: %s", a.lay+1, len(a.layers), a.layerName(a.lay))
	if a.layers[a.lay].hidden {
		s += " (hidden)"
	}
	if a.layers[a.lay].locked {
		s += " (locked)"
	}
	a.msg(s)
}

// pushLayersUndo keeps the whole layer list as an undo step; extra counts the
// cells of pictures the step lets go of.
func (a *App) pushLayersUndo(extra int) {
	s := a.snapshot()
	s.lwhole = true
	s.layers = a.cloneLayers(s.doc)
	s.cost += 8*len(s.layers) + extra
	a.pushSnap(s)
}

// otherLayerCells counts the cells of the layers that are not current.
func (t *Tab) otherLayerCells() int {
	n := 0
	for i, l := range t.layers {
		if i != t.lay {
			n += l.doc.cols * l.doc.rows
		}
	}
	return n
}

// newLayer puts an empty layer above the current one and goes to it; copy: a
// copy of the current one.
func (a *App) newLayer(copied bool) {
	a.endPlay()
	if why := a.layerRefused(); why != "" {
		a.msg(why)
		return
	}
	a.pushLayersUndo(0)
	a.ensureLayers()
	cur := a.doc
	var n *Doc
	name := ""
	if copied {
		n = cur.clone()
		name = a.layerName(a.lay) + " copy"
	} else {
		n = newDoc(cur.cols, cur.rows)
		layerFields(n, cur)
	}
	a.leaveLayer()
	l := Layer{doc: n, name: name}
	if copied {
		l.clear = a.layers[a.lay].clear
	}
	a.layers = slices.Insert(a.layers, a.lay+1, l)
	a.lay++
	a.doc = n.clone()
	a.float, a.moving, a.inRun = nil, false, false
	a.layerMsg()
}

// deleteLayer takes the current layer out; the one below (or above) is worked on.
func (a *App) deleteLayer() {
	a.endPlay()
	if a.layers == nil {
		a.msg("The page has one layer")
		return
	}
	if a.layers[a.lay].locked {
		a.msg("Unlock the layer first: a locked layer is not changed")
		return
	}
	cur := a.doc
	a.pushLayersUndo(cur.cols * cur.rows)
	a.layers = slices.Delete(a.layers, a.lay, a.lay+1)
	a.liveLayer(max(0, a.lay-1), cur.rows, cur)
	a.oneLayer()
	a.float, a.moving, a.inRun = nil, false, false
	a.layerMsg()
}

// moveLayer moves the current layer d places up (towards the top) or down.
func (a *App) moveLayer(d int) {
	a.endPlay()
	if a.layers == nil {
		return
	}
	to := a.lay + d
	if to < 0 || to >= len(a.layers) {
		return
	}
	a.pushLayersUndo(0)
	a.layers[a.lay], a.layers[to] = a.layers[to], a.layers[a.lay]
	a.lay = to
	a.layerMsg()
}

// mergeDown puts the current layer onto the one below, as one layer.
func (a *App) mergeDown() {
	a.endPlay()
	switch {
	case a.layers == nil || a.lay == 0:
		a.msg("There is no layer below to merge into")
		return
	case a.layers[a.lay].hidden || a.layers[a.lay-1].hidden:
		a.msg("Show both layers first: a hidden layer is not merged")
		return
	case a.layers[a.lay].locked || a.layers[a.lay-1].locked:
		a.msg("Unlock both layers first: a locked layer is not changed")
		return
	}
	cur := a.doc
	a.pushLayersUndo(cur.cols * cur.rows)
	below := a.layers[a.lay-1].doc.clone()
	below.ensureRows(cur.rows)
	clear := a.layers[a.lay].clear
	for y := range min(cur.rows, below.rows) {
		for x := range min(cur.cols, below.cols) {
			k := y*below.cols + x
			if v, ok := mergeCell(cur, below.cells[k], cur.cells[y*cur.cols+x], clear); ok {
				below.cells[k] = v
			}
		}
	}
	layerFields(below, cur)
	wideNormal(below) // a wide character half covered (wide.go)
	a.layers = slices.Delete(a.layers, a.lay, a.lay+1)
	a.lay--
	a.layers[a.lay].doc = below
	a.doc = below.clone()
	a.oneLayer()
	a.float, a.moving, a.inRun = nil, false, false
	a.layerMsg()
}

// mergeAll makes the page one layer: what is seen. Hidden layers are let go.
func (a *App) mergeAll() {
	a.endPlay()
	if a.layers == nil {
		a.msg("The page has one layer")
		return
	}
	hidden := 0
	for _, l := range a.layers {
		if l.hidden {
			hidden++
		}
	}
	a.pushLayersUndo(a.otherLayerCells())
	a.doc = a.seen()
	a.layers, a.lay = nil, 0
	a.float, a.moving, a.inRun = nil, false, false
	switch hidden {
	case 0:
		a.msg("The layers are one now")
	default:
		a.msg(fmt.Sprintf("The layers are one now; %d hidden layers were left out (%s brings them back)", hidden, keyName("undo")))
	}
}

// toggleLayer shows or hides layer i.
func (a *App) toggleLayer(i int) {
	if a.layers == nil || i < 0 || i >= len(a.layers) {
		a.msg("The page has one layer")
		return
	}
	a.pushLayersUndo(0)
	a.layers[i].hidden = !a.layers[i].hidden
	a.msg(fmt.Sprintf("%s %s", a.layerName(i), map[bool]string{true: "hidden", false: "shown"}[a.layers[i].hidden]))
}

// toggleBlackClear: on the current layer, a background 0 shows the background below (or not).
func (a *App) toggleBlackClear() {
	switch {
	case a.layers == nil:
		a.msg("The page has one layer")
		return
	case a.lay == 0:
		a.msg("The bottom layer covers everything: nothing is below it")
		return
	case !a.blackClears():
		a.msg("This page's cells have no background of their own")
		return
	}
	a.pushLayersUndo(0)
	l := &a.layers[a.lay]
	l.clear = layerBlack - l.clear
	a.msg(map[int]string{layerNormal: "Black is black on this layer", layerBlack: "Black shows the background below on this layer"}[l.clear])
}

// renameLayer gives layer i a name.
func (a *App) renameLayer(i int, name string) {
	if a.layers == nil || i < 0 || i >= len(a.layers) || a.layers[i].name == name {
		return
	}
	a.pushLayersUndo(0)
	a.layers[i].name = name
}

// cycleOthers shows the layers that are not current as they are, dim, or not at all.
func (a *App) cycleOthers() {
	a.others = (a.others + 1) % 3
	a.msg("Other layers: " + []string{"shown", "dim", "hidden"}[a.others])
}

// withEveryLayer replaces every layer's picture with fn of it, as one undo step
// (a page's layers are as wide as the page).
func (a *App) withEveryLayer(fn func(*Doc) *Doc) {
	a.pushLayersUndo(a.otherLayerCells())
	for i := range a.layers {
		if i != a.lay {
			a.layers[i].doc = fn(a.layers[i].doc)
		}
	}
	a.doc = fn(a.doc)
}

// everyLayerDo runs op (an edit of a.doc that pushes its own undo step: delete
// a row, insert a column ...) on every layer, as one undo step. Widths that
// came out different are made the widest.
func (a *App) everyLayerDo(op func()) {
	a.pushLayersUndo(a.otherLayerCells())
	a.layering = true
	defer func() { a.layering = false }()
	live, cx, cy := a.doc, a.cx, a.cy
	for i := range a.layers {
		if i == a.lay {
			continue
		}
		a.doc = a.layers[i].doc.clone()
		op()
		a.layers[i].doc = a.doc
		a.cx, a.cy = cx, cy
	}
	a.doc = live
	op()
	w := a.doc.cols
	for _, l := range a.layers {
		w = max(w, l.doc.cols)
	}
	for i := range a.layers {
		if i != a.lay && a.layers[i].doc.cols != w {
			a.layers[i].doc = a.layers[i].doc.withCols(w)
		}
	}
	if a.doc.cols != w {
		a.doc = a.doc.withCols(w)
	}
}

// layersMaxRows is the most rows the current layer may have so that all layers
// together stay under maxPageCells.
func (a *App) layersMaxRows() int {
	return max(1, (maxPageCells-a.otherLayerCells())/max(1, a.doc.cols))
}

// ---------------------------------------------------------------- drawing

// drawLayersRow draws row y of a page of layers, columns x0-x1, at (ox, py):
// each cell as it is seen; with the other layers dim, a cell that comes from
// another layer than the current one faint; with them hidden, only the current one.
func (a *App) drawLayersRow(fb *FB, ox, py, colPx, rowPx, z, x0, x1, y int) {
	only := -1
	if a.others == 2 {
		only = a.lay
	}
	for x := x0; x < x1; x++ {
		v, from := a.shownAt(x, y, only)
		px := ox + x*colPx
		if a.uni() { // a wide character's halves, cut where another layer covers one (as flatten does)
			prev, next := blank, blank
			if x > 0 {
				prev, _ = a.shownAt(x-1, y, only)
			}
			if x+1 < a.doc.cols {
				next, _ = a.shownAt(x+1, y, only)
			}
			if uniIsLead(v) && !uniIsCont(next) {
				v = uniSpaceOf(v)
			}
			a.drawUniAt(fb, px, py, z, v, prev, next)
		} else {
			a.drawCellXY(fb, px, py, z, x, y, v)
		}
		if a.others == 1 && from != a.lay {
			fb.blend(Rect{px, py, colPx, rowPx}, a.layerPaper(v), 150)
		}
	}
}

// layerPaper is the colour a faint cell fades towards: the page's paper.
func (a *App) layerPaper(v uint32) uint32 {
	switch {
	case a.atari():
		return a.atariRGB(a.doc.bg)
	case a.screen():
		return a.screenRGB(a.doc.bg)
	case a.st():
		return a.pal[cellBg(v)&15]
	}
	return uint32(cPaper)
}

// copySeen copies the selection as it is seen (every visible layer), not only
// the current layer's cells.
func (a *App) copySeen() {
	if a.sel == nil {
		a.msg("Select a block first (Select tool)")
		return
	}
	r := a.selRect()
	p := a.seen()
	a.clip = p.copyRect(r.x, r.y, r.w, r.h)
	a.clipText = strings.Join(docText(p, a.runeOf, r.x, r.y, r.w, r.h), "\n")
	setClipboard(a.clipText)
	a.msg(fmt.Sprintf("Copied %dx%d as it is seen", r.w, r.h))
}
