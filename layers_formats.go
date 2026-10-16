package main

// The files that keep a page's layers: IcyDraw .icy, REXPaint .xp and Playscii
// .psci (one frame). Reading keeps the layers only when CharEdit's layers show
// them exactly as the file's own editor does (the page put together from them
// looks as the format's reader gives it); otherwise the page opens as one
// layer, as before. Opening a file never changes what it looks like.

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"image"
	"slices"
)

// layersKeptBy: format fmtID keeps the layers of a page of type mode. Playscii's
// PETSCII and Atari pages have one background for the screen, so a layer of
// theirs does not read back on its own: they are put together (and kept beside
// the file).
func layersKeptBy(fmtID, mode string) bool {
	switch fmtID {
	case "icy": // (IcyDraw's PETSCII, ATASCII and ST pages: icymach.go)
		return mode == "pc" || mode == "amiga" || mode == "c64" || mode == "atari" || mode == "st"
	case "xp":
		return mode == "pc"
	case "psci":
		return mode == "pc" || mode == "amiga" || mode == "st"
	}
	return false
}

// fileLayers gives the page its file's layers (a.doc is the file's page as read),
// when they can be kept: not in CharView, on a page type with layers.
func (a *App) fileLayers() {
	if viewer != nil || !a.layered() || a.source == nil {
		return
	}
	if a.frames != nil {
		if a.srcKind == "psci" && layersKeptBy("psci", a.mode) {
			a.psciFramesLayers()
		}
		return
	}
	var ls []Layer
	switch a.srcKind {
	case "icy":
		if doc, err := parseIcyDraw(a.source); err == nil {
			ls = icyFileLayers(doc, a.doc, a.icyLayerPage(doc))
		}
	case "xp":
		ls = xpFileLayers(a.source, a.doc)
	case "psci":
		if layersKeptBy("psci", a.mode) {
			ls = psciFileLayers(a.source, 0, a.doc)
		}
	}
	if ls == nil {
		return
	}
	top := len(ls) - 1
	a.layers, a.lay = ls, top
	a.doc = ls[top].doc.clone()
	a.doc.ensureRows(ls[0].doc.rows)
}

// layersKept checks layers read from a file: as many as CharEdit has, one width,
// and put together they look as the file's page (flat) does. The page fields
// come from flat.
func layersKept(ls []Layer, flat *Doc) []Layer {
	if len(ls) < 2 || len(ls) > maxLayers {
		return nil
	}
	cells := 0
	for _, l := range ls {
		if l.doc == nil || l.doc.cols != flat.cols {
			return nil
		}
		layerFields(l.doc, flat)
		cells += l.doc.cols * l.doc.rows
	}
	if cells > maxPageCells {
		return nil
	}
	top := len(ls) - 1
	cur := ls[top].doc.clone()
	cur.ensureRows(flat.rows)
	if cur.rows != flat.rows || !sameLook(flatten(ls, top, cur), flat) {
		return nil
	}
	return ls
}

// layersOut are the page's layers to write, each with the page's fields (page
// is the page put together), nil for a page of one layer. Called while the page
// is seen (asSeen): every layer is in the list.
func (t *Tab) layersOut(page *Doc) []Layer {
	if t.layers == nil {
		return nil
	}
	ls := make([]Layer, len(t.layers))
	for i, l := range t.layers {
		d := *t.layerDoc(i) // the cells are only read
		layerFields(&d, page)
		l.doc = &d
		ls[i] = l
	}
	return ls
}

// ---------------------------------------------------------------- IcyDraw

// icyFileLayers are an IcyDraw file's layers as CharEdit's (nil: they cannot be
// kept): each layer is read as the file's only one, a cell IcyDraw leaves
// invisible is empty, a layer whose cells have a transparent background is one
// where black shows below.
func icyFileLayers(doc *icyDoc, flat *Doc, page func(*icyDoc) *Doc) []Layer {
	if len(doc.layers) < 2 || len(doc.layers) > maxLayers || len(doc.skipped) > 0 {
		return nil
	}
	ls := make([]Layer, len(doc.layers))
	for i, l := range doc.layers {
		if l == nil || l.group || l.mode != 0 {
			return nil
		}
		one, lc := *doc, *l
		lc.visible = true
		one.layers = []*icyLayer{&lc}
		d := page(&one)
		if d == nil || d.cols != flat.cols {
			return nil
		}
		clear := layerNormal
		for _, c := range l.cells {
			if i > 0 && c.visible && c.bg.kind == 't' {
				clear = layerBlack
			}
		}
		ls[i] = Layer{doc: d, name: l.title, hidden: !l.visible, locked: l.flags&icyEditLock != 0, clear: clear, keep: &layerKeep{IcyFlags: l.flags, IcyColour: l.colour}}
	}
	return layersKept(ls, flat)
}

// writeIcyLayers gives an IcyDraw document of every layer of t (page: them put
// together, for the picture): an empty cell of a layer above the bottom one is
// invisible, a black background on a layer where black shows below transparent.
func writeIcyLayers(t *Tab, page *Doc, pal [16]uint32, f *Font, ice bool, preview image.Image) []byte {
	if f == nil {
		f = fontByID("vga")
	}
	o := &icyOut{buffer: icyCP437, ice: map[bool]byte{true: icyIce, false: icyBlink}[ice], fontMod: 2,
		w: page.cols, h: max(1, page.rows), fontH: f.rows()}
	for _, c := range biosPalette(pal) {
		o.pal = append(o.pal, c&0xFFFFFF)
	}
	o.fonts = []icyFontRec{icyFontOf(f.name, f, f.rows())}
	ls := t.layersOut(page)
	for i, l := range ls {
		d, clear, bottom := l.doc, l.clear, i == 0
		var below *Doc
		if !bottom && clear == layerBlack {
			below = flatten(ls[:i], 0, ls[0].doc)
		}
		ol := icyOutLayer{title: t.layerName(i), flags: icyLayerFlags(l)}
		if l.keep != nil {
			ol.colour = l.keep.IcyColour
		}
		ol.cell = func(x, y int) (uint32, int, int, byte, uint16) {
			v := d.get(x, y)
			if !bottom && d.blankAt(v) {
				return 32, 7, 0, 0, icyInvisible
			}
			at := cellAttr(v)
			fg, bg, flags := int(at&15), int(at>>4), uint16(0)
			if !ice && bg > 7 {
				bg, flags = bg&7, icyBlinkAttr
			}
			if below != nil && cellBg(v) == 0 {
				// Transparent: IcyDraw shows the colour of the lower half of the cell
				// below (its half-block rule); where that is not the cell's background,
				// the background is written as it is seen.
				u := below.get(x, y)
				ubg := int(cellAttr(u) >> 4)
				if !ice && ubg > 7 {
					ubg &= 7
				}
				if bg = ubg; icyLowerColour(u) == ubg {
					bg = -1
				}
			}
			return uint32(cellCh(v)), fg, bg, 0, flags
		}
		o.layers = append(o.layers, ol)
	}
	return o.encode(preview)
}

// ---------------------------------------------------------------- REXPaint

// xpFileLayers are a REXPaint image's layers as CharEdit's (nil: they cannot be
// kept): a cell of REXPaint's transparent pink on a layer above the bottom one is empty.
func xpFileLayers(b []byte, flat *Doc) []Layer {
	layers, w, h, err := xpRaw(b)
	if err != nil || len(layers) < 2 {
		return nil
	}
	pick, pal := rgbColours(xpMerge(layers, w, h)) // the colours as the page has them (parseXP)
	if pal == nil {
		vga := paletteByID("vga").colors
		pal = &vga
	}
	colour := func(col uint32) int {
		if i := pick(col); i >= 0 && i < 16 && pal[i]&0xFFFFFF == col {
			return i
		}
		return nearestOf(col, *pal)
	}
	ls := make([]Layer, len(layers))
	for k, l := range layers {
		d := newDoc(w, h)
		if k == 0 {
			fillCells(d.cells, makeCell(' ', colour(0xAAAAAA), colour(0), 0))
		}
		for i := 0; (i+1)*10 <= len(l.data); i++ {
			x, y, c, clear := l.cell(i)
			if y >= h {
				continue
			}
			if clear {
				if k > 0 {
					continue
				}
				c.bg = 0
			}
			d.cells[y*w+x] = makeCell(int(c.ch), colour(c.fg), colour(c.bg), 0)
		}
		ls[k] = Layer{doc: d}
	}
	return layersKept(ls, flat)
}

// writeXPLayers gives a REXPaint image of the page's visible layers (REXPaint
// keeps no hidden ones, and shows 9 at most: the lowest are merged). An empty
// cell of a layer above the bottom one gets the transparent pink; a black
// background where black shows below gets the background below it (REXPaint has
// no see-through background).
func (a *App) writeXPLayers(page *Doc) []byte {
	var vis []Layer
	hidden, baked := 0, 0
	for _, l := range a.layersOut(page) {
		if l.hidden {
			hidden++
			continue
		}
		vis = append(vis, l)
	}
	if len(vis) == 0 {
		return writeXP(page, a.pal, a.ice)
	}
	if n := len(vis); n > 9 { // the lowest put together as the first
		low := flatten(vis[:n-8], 0, vis[0].doc)
		vis = append([]Layer{{doc: low}}, vis[n-8:]...)
	}
	rows := max(1, page.rows)
	var raw bytes.Buffer
	le := func(v int32) { binary.Write(&raw, binary.LittleEndian, v) }
	le(-1)
	le(int32(len(vis)))
	for k, l := range vis {
		var below *Doc
		if k > 0 && l.clear == layerBlack {
			below = flatten(vis[:k], 0, vis[0].doc)
		}
		le(int32(page.cols))
		le(int32(rows))
		for x := 0; x < page.cols; x++ {
			for y := 0; y < rows; y++ {
				v := l.doc.get(x, y)
				if k > 0 && page.blankAt(v) {
					le(0)
					raw.Write([]byte{0, 0, 0, 0xFF, 0, 0xFF})
					continue
				}
				if below != nil && cellBg(v) == 0 {
					v = v&^0xF000 | below.get(x, y)&0xF000
					baked++
				}
				le(int32(cellCh(v)))
				f, bg := a.pal[cellFg(v)], a.pal[shownBg(v, a.ice)]
				raw.Write([]byte{byte(f >> 16), byte(f >> 8), byte(f), byte(bg >> 16), byte(bg >> 8), byte(bg)})
			}
		}
	}
	var out bytes.Buffer
	zw := gzip.NewWriter(&out)
	zw.Write(raw.Bytes())
	zw.Close()
	var notes []string
	if hidden > 0 {
		notes = append(notes, fmt.Sprintf("%d hidden layers left out (REXPaint keeps none)", hidden))
	}
	if baked > 0 {
		notes = append(notes, fmt.Sprintf("%d black backgrounds got the colour below (REXPaint has no see-through background)", baked))
	}
	if len(notes) > 0 {
		a.msg("REXPaint: " + joinNotes(notes))
	}
	return out.Bytes()
}

func joinNotes(n []string) string {
	s := ""
	for i, v := range n {
		if i > 0 {
			s += "; "
		}
		s += v
	}
	return s
}

// ---------------------------------------------------------------- Playscii

// psciFileLayers are the layers of frame fi of a Playscii file as CharEdit's
// (nil: they cannot be kept): each layer is read as the file's only one (by
// z, as Playscii draws them); a layer with see-through backgrounds under its
// characters is one where black shows below.
func psciFileLayers(b []byte, fi int, flat *Doc) []Layer {
	var f psciFile
	if err := json.Unmarshal(b, &f); err != nil || fi >= len(f.Frames) {
		return nil
	}
	layers := append([]psciLayer{}, f.Frames[fi].Layers...)
	if len(layers) < 2 || len(layers) > maxLayers {
		return nil
	}
	for i := 1; i < len(layers); i++ { // by z, lowest first (stable), as parsePsci
		for j := i; j > 0 && layers[j].Z < layers[j-1].Z; j-- {
			layers[j], layers[j-1] = layers[j-1], layers[j]
		}
	}
	ls := make([]Layer, len(layers))
	one := 1.0
	for i, l := range layers {
		g := f
		lc := l
		lc.Visible = &one
		g.Frames = []psciFrame{{Delay: f.Frames[fi].Delay, Layers: []psciLayer{lc}}}
		js, err := json.Marshal(&g)
		if err != nil {
			return nil
		}
		art, err := parsePsci(js)
		if err != nil || len(art.docs) != 1 || len(art.need) > 0 {
			return nil
		}
		clear := layerNormal
		for _, t := range l.Tiles {
			if i > 0 && t.Bg <= 0 && t.Fg > 0 {
				clear = layerBlack
			}
		}
		ls[i] = Layer{doc: art.docs[0], name: l.Name, hidden: l.Visible != nil && *l.Visible == 0, clear: clear, keep: &layerKeep{PsciZ: l.Z}}
	}
	return layersKept(ls, flat)
}

// psciLayersOut are the Playscii layers of a page of layers: an empty cell of a
// layer above the bottom one is fully transparent, a black background where
// black shows below Playscii's transparent 0.
func psciLayersOut(ls []Layer, page *Doc, w, h int, tile func(*Doc, int, int) psciTile) []psciLayer {
	var out []psciLayer
	for i, l := range ls {
		name := l.name
		if name == "" {
			name = fmt.Sprintf("Layer %d", i+1)
		}
		pl := psciLayer{Z: float64(i), Name: name, Tiles: make([]psciTile, 0, w*h)}
		vis := 1.0
		if l.hidden {
			vis = 0
		}
		pl.Visible = &vis
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				v := l.doc.get(x, y)
				if i > 0 && page.blankAt(v) {
					pl.Tiles = append(pl.Tiles, psciTile{})
					continue
				}
				t := tile(l.doc, x, y)
				if i > 0 && l.clear == layerBlack && cellBg(v) == 0 && !page.uni {
					t.Bg = 0
				}
				pl.Tiles = append(pl.Tiles, t)
			}
		}
		out = append(out, pl)
	}
	return out
}

// psciLayersOf are the layers of every frame (docs: the frames as seen) a
// Playscii file of the page keeps (nil: they are put together).
func (a *App) psciLayersOf(docs []*Doc) [][]Layer {
	if !layersKeptBy("psci", a.mode) || a.layers == nil {
		return nil
	}
	out := make([][]Layer, len(docs))
	for i, page := range docs {
		ld := a.frameLayerDocs(i)
		ls := make([]Layer, len(a.layers))
		for k, l := range a.layers {
			d := *ld[k] // the cells are only read
			layerFields(&d, page)
			l.doc = &d
			ls[k] = l
		}
		out[i] = ls
	}
	return out
}

// icyLowerColour is the colour IcyDraw gives a transparent background over cell
// v (palette number, BIOS order): the colour of the lower half of v (icy_engine
// paint/half_block.rs, HalfBlock::from_char): a space's background, a lower half
// block's or full block's foreground, an upper half's background, else the
// foreground when both colours are one, else 0.
func icyLowerColour(v uint32) int {
	at := cellAttr(v)
	fg, bg := int(at&15), int(at>>4)
	switch cellCh(v) {
	case 0, 32, 255:
		return bg
	case 219, 220:
		return fg
	case 223:
		return bg
	}
	if fg == bg {
		return fg
	}
	return 0
}

// psciFramesLayers gives a page of frames read from a Playscii file every
// frame's layers (Playscii's frames all have the same layers), when they show
// every frame as it is; else the frames stay as they are, put together.
func (a *App) psciFramesLayers() {
	n := len(a.frames)
	per := make([][]Layer, n)
	for i := range n {
		if per[i] = psciFileLayers(a.source, i, a.frameDoc(i)); per[i] == nil || len(per[i]) != len(per[0]) {
			return
		}
	}
	// One layer list for every frame: a layer lets black show below if it does in any frame.
	meta := slices.Clone(per[0])
	for _, ls := range per[1:] {
		for k, l := range ls {
			if l.name != meta[k].name || l.hidden != meta[k].hidden {
				return
			}
			meta[k].clear = max(meta[k].clear, l.clear)
		}
	}
	cells := 0
	for i, ls := range per {
		for k := range ls {
			ls[k].clear = meta[k].clear
			cells += 2 * ls[k].doc.cols * ls[k].doc.rows
		}
		if layersKept(ls, a.frameDoc(i)) == nil {
			return
		}
	}
	if cells > maxPageCells {
		return
	}
	for i, ls := range per {
		docs := make([]*Doc, len(ls))
		for k := range ls {
			docs[k] = ls[k].doc
		}
		if i == a.cur {
			continue
		}
		a.frames[i].layers = docs
		a.frames[i].doc = flattenFrame(meta, docs, a.frames[i].doc)
	}
	top := len(meta) - 1
	a.layers, a.lay = meta, top
	for k := range a.layers {
		a.layers[k].doc = per[a.cur][k].doc
	}
	a.liveLayer(top, a.doc.rows, a.doc)
}

// icyLayerFlags are IcyDraw's flags of a layer: the file's other bits kept;
// visible and the edit lock are the layer's.
func icyLayerFlags(l Layer) uint32 {
	flags := uint32(0)
	if l.keep != nil {
		flags = l.keep.IcyFlags &^ (icyVisible | icyEditLock)
	}
	if !l.hidden {
		flags |= icyVisible
	}
	if l.locked {
		flags |= icyEditLock
	}
	return flags
}

// icyLayerPage is how one layer of an IcyDraw file reads on this page: as a PC
// page, or as the page of the file's machine (as reparseIcyMachine reads it).
func (a *App) icyLayerPage(doc *icyDoc) func(*icyDoc) *Doc {
	if !icyMachineBuffer(doc.buffer) {
		return icyPCPage
	}
	return icyMachLayerPage(a.gtia(), a.mode, a.doc.cols)
}

// icyPCPage is an IcyDraw document read as a PC page.
func icyPCPage(doc *icyDoc) *Doc {
	d, _, _ := icyPage(doc)
	return d
}

// icyMachLayerPage reads an IcyDraw document as a page of type mode, cols wide
// (nil: the file is of another machine).
func icyMachLayerPage(gtia *[256]uint32, mode string, cols int) func(*icyDoc) *Doc {
	return func(one *icyDoc) *Doc {
		m := icyMachinePage(one, gtia)
		if m == nil || m.mode != mode {
			return nil
		}
		if m.doc.cols != cols {
			return m.doc.withCols(cols)
		}
		return m.doc
	}
}

// icyMachineLayers are the IcyDraw layers of a PETSCII, Atari or ST page of
// layers (page: them put together), cellOf giving a picture's cells as the
// machine's page: an empty cell of a layer above the bottom one is invisible;
// on a layer where black shows below (ST pages) the background is written as it
// is seen (IcyDraw's transparent background takes another colour: layers_formats.go).
func icyMachineLayers(t *Tab, page *Doc, cellOf func(*Doc) func(x, y int) (uint32, int, int, byte, uint16)) []icyOutLayer {
	ls := t.layersOut(page)
	var out []icyOutLayer
	for i, l := range ls {
		d := l.doc
		if i > 0 && l.clear == layerBlack {
			below := flatten(ls[:i], 0, ls[0].doc)
			d = d.clone()
			for k, v := range d.cells[:d.cols*d.rows] {
				if !page.blankAt(v) && cellBg(v) == 0 {
					d.cells[k] = v&^0xF000 | below.get(k%d.cols, k/d.cols)&0xF000
				}
			}
		}
		cell, bottom := cellOf(d), i == 0
		ol := icyOutLayer{title: t.layerName(i), flags: icyLayerFlags(l)}
		if l.keep != nil {
			ol.colour = l.keep.IcyColour
		}
		ol.cell = func(x, y int) (uint32, int, int, byte, uint16) {
			if !bottom && layerClearAt(page, d.get(x, y)) {
				return 32, 0, 0, 0, icyInvisible
			}
			return cell(x, y)
		}
		out = append(out, ol)
	}
	return out
}
