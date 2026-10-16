package main

// The layers in the side panel (Studio layout): one row a layer, the top one
// first, as every editor shows them; quiet while the page has one layer.

import "fmt"

// fieldLayer is the name of the layer being renamed.
var fieldLayer = &Field{max: 32}

// layerNaming is the layer fieldLayer names (-1: none).
var layerNaming = -1

func init() {
	fieldLayer.enter = func() {
		if layerNaming >= 0 {
			a.renameLayer(layerNaming, fieldLayer.String())
		}
		layerNaming = -1
		ui.focus = nil
	}
}

// drawLayers draws the layers' rows and buttons at x, y, w wide; it returns the y under them.
func (a *App) drawLayers(u *UI, x, y, w int) int {
	fb := u.fb
	h := gadH
	if a.layers == nil {
		fb.text(x, y+3, "One layer", cDim)
		if u.button(Rect{x + w - buttonW("New"), y, buttonW("New"), h}, "New", false, "New layer above this one ("+keyName("layer_new")+")") != nil {
			a.newLayer(false)
		}
		return y + h
	}
	if u.focus != fieldLayer {
		layerNaming = -1
	}
	for i := len(a.layers) - 1; i >= 0; i-- {
		l := a.layers[i]
		row := Rect{x, y, w, h}
		if i == a.lay {
			fb.fill(row, cBlue)
		}
		if on, _ := u.check(x, y-2, "", !l.hidden, fmt.Sprintf("Show / hide %s (%s)", a.layerName(i), keyName("layer_show"))); on == l.hidden {
			a.toggleLayer(i)
		}
		nx := x + checkW("") + 4
		nr := Rect{nx, y, w - (nx - x), h}
		if layerNaming == i {
			u.field(nr, fieldLayer, "The layer's name (Enter keeps it)")
		} else {
			name := a.layerName(i)
			if l.clear == layerBlack {
				name += "  (black shows below)"
			}
			col := uint32(cText)
			if l.hidden {
				col = cDim
			}
			fb.text(nx+2, y+3, fitString(name, nr.w-4), col)
			hint := fmt.Sprintf("Work on %s (%s / %s)", a.layerName(i), keyName("layer_above"), keyName("layer_below"))
			if i == a.lay {
				hint = "Click to rename the layer"
			}
			u.setHint(nr, hint)
			if u.take(nr) != nil {
				if i == a.lay {
					layerNaming = i
					fieldLayer.set(a.layerName(i))
					u.focus = fieldLayer
				} else {
					a.goLayer(i)
				}
			}
		}
		y += h + 2
	}
	y += 4
	fl := &flow{x0: x, x: x, y: y, right: x + w}
	for _, b := range []struct {
		label, id string
		do        func()
	}{
		{"New", "layer_new", func() { a.newLayer(false) }},
		{"Copy", "layer_copy", func() { a.newLayer(true) }},
		{"Up", "layer_raise", func() { a.moveLayer(1) }},
		{"Down", "layer_lower", func() { a.moveLayer(-1) }},
		{"Merge", "layer_merge_down", func() { a.mergeDown() }},
		{"Delete", "layer_delete", func() { a.deleteLayer() }},
	} {
		hint := actionByID[b.id].help
		if k := keyName(b.id); k != "(no key)" {
			hint += " (" + k + ")"
		}
		if u.button(fl.place(buttonW(b.label)), b.label, false, hint) != nil {
			b.do()
		}
	}
	if a.blackClears() && a.lay > 0 {
		if u.button(fl.place(buttonW("Black")), "Black", a.layers[a.lay].clear == layerBlack, actionByID["layer_black"].help) != nil {
			a.toggleBlackClear()
		}
	}
	others := []string{"Others shown", "Others dim", "Others hidden"}
	u.choice("layerothers", fl.place(choiceW(others)), others, a.others, actionByID["layer_others"].help, func(i int) { a.others = i })
	return fl.y + h
}
