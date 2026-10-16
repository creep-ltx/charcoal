package main

// Layers together with frames (docs/layers-design.md §9), as Playscii has them:
// the layer list (names, shown / hidden, locked, black shows below) is the
// page's, the same for every frame; each frame has its own picture of every
// layer. A frame that is not current keeps its layers' pictures in
// Frame.layers and, in Frame.doc, the picture they make together: so the strip,
// the onion skin, playing, GIF/APNG and every writer of frames, which read
// Frame.doc, see each frame as it is seen without knowing about layers. Frames
// that are not current are never changed in place (as before), so the picture
// they make is made again only when the frame is left or a layer of every frame
// changes (new, delete, move, merge, show / hide, black shows below).
//
// The current frame's layers are the tab's (Tab.layers, the live one Tab.doc);
// its Frame.layers and Frame.doc are stale, as Frame.doc always was.

import "slices"

// frameLayerDocs are frame i's layer pictures, bottom first (the current
// frame's: the tab's layers, the live one included).
func (t *Tab) frameLayerDocs(i int) []*Doc {
	if t.layers == nil {
		return []*Doc{t.frameDoc(i)}
	}
	if t.frames == nil || i == t.cur {
		docs := make([]*Doc, len(t.layers))
		for k := range docs {
			docs[k] = t.layerDoc(k)
		}
		return docs
	}
	return t.frames[i].layers
}

// storeFrame puts the current frame's layers into its Frame, with the picture
// they make (the live layer as a copy: it goes on being drawn on until the
// frame is left).
func (a *App) storeFrame() {
	docs := make([]*Doc, len(a.layers))
	for k := range a.layers {
		if k != a.lay {
			docs[k] = a.layers[k].doc
			continue
		}
		d := shrinkLayer(a.doc, k)
		if d == a.doc {
			d = a.doc.clone()
		}
		docs[k] = d
	}
	a.frames[a.cur].layers = docs
	a.frames[a.cur].doc = flatten(a.layers, a.lay, a.doc)
}

// loadFrame makes frame i's layers the tab's, its current layer live.
func (a *App) loadFrame(i int) {
	f := a.frames[i]
	for k := range a.layers {
		a.layers[k].doc = f.layers[k]
	}
	a.lay = min(a.lay, len(a.layers)-1)
	a.liveLayer(a.lay, f.doc.rows, f.doc)
}

// flattenFrame is the picture a frame's layer pictures make, as tall as the
// frame was (rows) and with its fields (of was).
func flattenFrame(meta []Layer, docs []*Doc, was *Doc) *Doc {
	ls := slices.Clone(meta)
	rows := was.rows
	for k := range ls {
		ls[k].doc = docs[k]
		rows = max(rows, docs[k].rows)
	}
	ref := &Doc{cols: docs[0].cols, rows: rows}
	layerFields(ref, was)
	return flatten(ls, -1, ref)
}

// everyFrameLayers changes the layer pictures of every frame but the current
// one with fn (given a copy of the list, bottom first: a page of one layer gives
// the frame's picture). Called before the current frame's own change, so
// a.layers is the list as it was.
func (a *App) everyFrameLayers(fn func(docs []*Doc) []*Doc) {
	for i := range a.frames {
		if i == a.cur {
			continue
		}
		f := &a.frames[i]
		docs := []*Doc{f.doc}
		if a.layers != nil {
			docs = slices.Clone(f.layers)
		}
		h := 0
		for _, d := range docs {
			h = max(h, d.rows)
		}
		docs = fn(docs)
		if len(docs) > 0 && docs[0].rows < h { // the bottom layer is the frame's height (a short one may have come down)
			docs[0] = docs[0].clone()
			docs[0].ensureRows(h)
		}
		f.layers = docs
	}
}

// settleFrames makes every other frame's picture again from its layers after a
// change of the layer list (a.layers as it is now): a frame left with one layer
// has it as its picture.
func (a *App) settleFrames() {
	for i := range a.frames {
		f := &a.frames[i]
		if i == a.cur {
			if a.layers == nil {
				f.layers = nil
			}
			continue
		}
		if f.layers == nil {
			continue
		}
		if a.layers == nil || len(f.layers) == 1 {
			d := f.layers[0]
			if d.rows < f.doc.rows || !sameFields(d, f.doc) { // never changed in place: undo steps share it
				d = d.clone()
				d.ensureRows(f.doc.rows)
				layerFields(d, f.doc)
			}
			f.doc, f.layers = d, nil
			continue
		}
		f.doc = flattenFrame(a.layers, f.layers, f.doc)
	}
}

// emptyLayerLike is an empty layer picture for a frame whose picture is d.
func emptyLayerLike(d *Doc) *Doc {
	n := newDoc(d.cols, 1)
	layerFields(n, d)
	return n
}

// allCells counts every picture a page keeps: every frame's every layer, and
// the pictures frames of layers make.
func (t *Tab) allCells() int {
	if t.frames == nil {
		return t.layerCells()
	}
	n := 0
	for i := range t.frames {
		if i == t.cur {
			n += t.layerCells()
			continue
		}
		f := t.frames[i]
		n += f.doc.cols * f.doc.rows
		for _, d := range f.layers {
			n += d.cols * d.rows
		}
	}
	return n
}

// wholeSnap is an undo step of the page's structure: the frame list and the
// layer list both (either may be nil), the live layer cloned.
func (a *App) wholeSnap(extra int) Snap {
	s := a.snapshot()
	s.whole, s.lwhole = true, true
	if a.frames != nil {
		s.frames = slices.Clone(a.frames)
		if a.layers == nil {
			s.frames[a.cur].doc = s.doc
		}
		s.cost += 8 * len(s.frames)
	}
	s.layers = a.cloneLayers(s.doc)
	s.cost += 8*len(s.layers) + extra
	return s
}

// otherFramesCells counts the cells of the frames that are not current, their layers too.
func (t *Tab) otherFramesCells() int {
	if t.frames == nil {
		return 0
	}
	return t.allCells() - t.layerCells()
}

// sameFields: two pictures have the same page fields (layerFields).
func sameFields(x, y *Doc) bool {
	return x.bg == y.bg && x.border == y.border && x.ink == y.ink && x.glyph0 == y.glyph0 && x.uni == y.uni && x.fullest == y.fullest
}

// keepLayersHint says how to keep a page of frames of layers.
func (a *App) keepLayersHint() string {
	if layersKeptBy("psci", a.mode) {
		return "save as .psci to keep them, or merge the layers"
	}
	return "merge the layers first to take it for saved"
}
