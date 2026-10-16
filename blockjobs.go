package main

// Block jobs: fill the selection, move it by dragging, crop the page to it.

import "fmt"

// fillSel fills the selection with the brush (character and colours). With
// nothing selected it switches to the Fill tool instead.
func (a *App) fillSel() {
	if a.float != nil {
		a.msg("Put the block down first (click), or Esc")
		return
	}
	if a.sel == nil {
		a.setTool("fill")
		a.msg("Fill tool (select a block first to fill all of it with " + keyName("fill_block") + ")")
		return
	}
	r := a.selRect()
	a.pushUndo()
	v := a.brushCell()
	for y := r.y; y < min(r.y+r.h, a.doc.rows); y++ { // filling never adds rows
		for x := r.x; x < r.x+r.w; x++ {
			if a.uni() { // whole wide characters, only where both cells are inside (wide.go)
				if !uniIsLead(v) || x+1 < r.x+r.w {
					wideSet(a.doc, x, y, v)
				}
				continue
			}
			a.doc.set(x, y, v)
		}
	}
	a.msg(fmt.Sprintf("Filled %dx%d with the brush", r.w, min(r.h, a.doc.rows-r.y)))
}

// liftSel starts moving the selection: it is taken off the page (leaving
// empty cells) and follows the mouse, held at cell (hx, hy). One undo step
// covers the whole move.
func (a *App) liftSel(hx, hy int) {
	r := a.selRect()
	r.h = min(r.h, a.doc.rows-r.y)
	if r.h <= 0 {
		return
	}
	b := a.doc.copyRect(r.x, r.y, r.w, r.h)
	a.pushUndo()
	e := a.emptyCell()
	for y := r.y; y < r.y+r.h; y++ {
		for x := r.x; x < r.x+r.w; x++ {
			a.doc.set(x, y, e)
		}
	}
	a.float, a.sel = b, nil
	a.floatDX, a.floatDY, a.moving = hx-r.x, hy-r.y, true
}

// floatAt is where the floating block's top left cell is with the mouse at (hx, hy).
func (a *App) floatAt(hx, hy int) (int, int) {
	return hx - a.floatDX, hy - a.floatDY
}

// dropMoved puts a moved block down where the mouse let go of it, inside the
// page, and selects it there. It is part of the lift's undo step.
func (a *App) dropMoved() {
	f := a.float
	x0, y0 := a.cx, a.cy
	if a.hover != nil {
		x0, y0 = a.floatAt(a.hover.x, a.hover.y)
	}
	x0 = max(0, min(a.doc.cols-f.w, x0))
	y0 = max(0, min(a.maxRows()-f.h, y0))
	for y := 0; y < f.h; y++ {
		for x := 0; x < f.w; x++ {
			a.setCell(x0+x, y0+y, f.cells[y*f.w+x])
		}
	}
	a.float, a.moving = nil, false
	a.floatDX, a.floatDY = 0, 0
	a.sel = &Sel{x0, y0, x0 + f.w - 1, y0 + f.h - 1}
	a.msg(fmt.Sprintf("Moved to %d,%d", x0+1, y0+1))
}

// cancelMove puts a block being moved back where it was.
func (a *App) cancelMove() {
	a.float, a.moving = nil, false
	a.doUndo()
	a.redo = a.redo[:0]
}

// cropToSel makes the page just the selection (Amiga and PC pages only: a
// machine's screen keeps its size).
func (a *App) cropToSel() {
	if !a.freeWidth() {
		a.msg("Only Amiga, PC and Unicode pages can be cropped: a machine's screen keeps its size")
		return
	}
	if a.sel == nil {
		a.msg("Select the part to keep first, then crop")
		return
	}
	r := a.selRect()
	r.h = min(r.h, a.doc.rows-r.y)
	if r.h <= 0 {
		return
	}
	a.endPlay()
	a.withEveryFrame(func(d *Doc) *Doc { // every frame: they keep one width
		h := max(1, min(r.h, d.rows-r.y))
		b := d.copyRect(r.x, r.y, r.w, h)
		n := newDoc(r.w, h)
		n.bg, n.border, n.ink, n.glyph0 = d.bg, d.border, d.ink, d.glyph0
		copy(n.cells, b.cells)
		return n
	})
	a.sel, a.cx, a.cy, a.scrollX, a.scrollY = nil, 0, 0, 0, 0
	a.msg(fmt.Sprintf("Cropped to %dx%d (%s undoes it)", r.w, r.h, keyName("undo")))
}
