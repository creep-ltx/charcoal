package main

// Locked layers: no tool changes a locked layer (nor a hidden one, as in IcyDraw) (IcyDraw's edit lock, read from
// and written to .icy files). The tools refuse at their start where that is easy
// (a press on the page, typing); every other change is caught by pushUndo, which
// every edit of the page's cells calls first: on a locked layer it keeps the page
// as it was, and lockCheck puts it back after the event if the cells changed. A
// change of the page's own fields (a screen colour, the border) is no change of
// a layer and is kept. The page's structure (width, height, rows and columns,
// crop) is every layer's and changes a locked one too, as in IcyDraw.

import (
	"fmt"
	"slices"
)

// layerLocked: the layer being worked on is locked.
func (t *Tab) layerLocked() bool {
	return t.layers != nil && t.lay < len(t.layers) && t.layers[t.lay].locked
}

// layerBlocked: the layer being worked on takes no edits: locked, or hidden (as
// in IcyDraw: what is drawn on a hidden layer would not be seen).
func (t *Tab) layerBlocked() bool {
	return t.layerLocked() || t.layers != nil && t.lay < len(t.layers) && t.layers[t.lay].hidden
}

// lockedMsg is what a refused tool says.
func (a *App) lockedMsg() string {
	if !a.layerLocked() {
		return fmt.Sprintf("%s is hidden: show it to draw on it (%s)", a.layerName(a.lay), keyOrMenu("layer_show", "Page > Show the layer"))
	}
	return fmt.Sprintf("%s is locked (%s unlocks it)", a.layerName(a.lay), keyOrMenu("layer_lock", "Page > Lock the layer"))
}

// keyOrMenu is the key of action id, or where it is in the menus when it has none.
func keyOrMenu(id, menu string) string {
	if k := keyName(id); k != "(no key)" {
		return k
	}
	return menu
}

// lockRefused: the current layer is locked or hidden, said on the status line.
func (a *App) lockRefused() bool {
	if !a.layerBlocked() {
		return false
	}
	a.msg(a.lockedMsg())
	return true
}

// toggleLock locks or unlocks layer i.
func (a *App) toggleLock(i int) {
	if a.layers == nil || i < 0 || i >= len(a.layers) {
		a.msg("The page has one layer: a layer is locked on a page of several")
		return
	}
	a.pushLayersUndo(0)
	a.layers[i].locked = !a.layers[i].locked
	if i == a.lay {
		a.float, a.moving, a.inRun = nil, false, false
	}
	a.msg(fmt.Sprintf("%s %s", a.layerName(i), map[bool]string{true: "locked: no tool changes it", false: "unlocked"}[a.layers[i].locked]))
}

// lockGuard is called by pushUndo: on a locked layer the page as it was is kept
// (the first time in an event) instead of an undo step, for lockCheck.
func (a *App) lockGuard() bool {
	if a.layering || !a.layerBlocked() {
		return false
	}
	if a.lockSnap == nil {
		s := a.snapshot()
		a.lockSnap = &s
	}
	return true
}

// lockCheck runs after every event: if an edit reached a locked layer its cells
// are put back (the tool's drag ends); any other change (the page's fields, a
// text mode) is kept, as the undo step pushUndo would have made.
func (a *App) lockCheck() {
	s := a.lockSnap
	if s == nil {
		return
	}
	a.lockSnap = nil
	if a.lay != s.lay || a.layers == nil {
		return // the layer changed under it (an undo, another tab): nothing to compare
	}
	if s.doc.cols == a.doc.cols && s.doc.rows == a.doc.rows && slices.Equal(s.doc.cells, a.doc.cells) {
		a.pushSnap(*s) // not the layer's cells (a screen colour, a text mode, the draw order): the step it would have been
		return
	}
	a.restore(*s)
	a.button, a.shapeFrom, a.moving, a.selecting, a.last = 0, nil, false, false, nil
	a.msg(a.lockedMsg())
}
