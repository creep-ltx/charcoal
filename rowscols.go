package main

// Rows and columns: delete or insert one at the cursor (Alt+arrows by default).

import "fmt"

// emptyCell is what a new row or column is filled with: a space as the page
// starts (black ink on white paper on a Spectrum, a plain space on teletext).
func (a *App) emptyCell() uint32 {
	switch {
	case a.zx():
		return zxCell(32, 0, 7, false, false)
	case a.tt():
		return makeCell(0x20, 0, 0, 0)
	case a.cpc():
		return cpcBlank()
	case a.mt():
		return mtBlank
	case a.msx():
		return msxBlank()
	case a.mz():
		return mzCell(32, a.fg, a.bg)
	}
	return blank
}

// fixedHeight: a Spectrum screen and a teletext page have the rows they have.
func (a *App) fixedHeight() bool {
	return a.zx() || a.tt() || a.cpc() || a.igsPage() || a.skyPage() || a.mt() || a.msx() || a.mz()
}

// freeWidth: only Amiga and PC pages can be any width.
func (a *App) freeWidth() bool { return a.amiga() || a.pc() || a.uni() }

func (a *App) fillRow(y int, v uint32) {
	for x := 0; x < a.doc.cols; x++ {
		a.doc.set(x, y, v)
	}
}

// deleteRow takes the cursor's row out; the rows below move up. The page gets
// a row shorter, but a fixed-height page gets an empty row at the bottom.
func (a *App) deleteRow() {
	if a.layers != nil && !a.layering { // every layer (layers.go)
		a.everyLayerDo(a.deleteRow)
		return
	}
	d := a.doc
	if a.cy >= d.rows {
		return
	}
	a.endPlay()
	a.pushUndo()
	c := d.cols
	copy(d.cells[a.cy*c:d.rows*c], d.cells[(a.cy+1)*c:d.rows*c])
	if a.fixedHeight() || d.rows == 1 {
		a.fillRow(d.rows-1, a.emptyCell())
	} else {
		d.rows--
		fillCells(d.cells[d.rows*c:(d.rows+1)*c], blank)
	}
	a.cy = min(a.cy, d.rows-1)
	a.sel = nil
	a.moved()
}

// insertRow puts an empty row in at the cursor; the rows below move down. The
// page grows a row, except a fixed-height (or full) page, which loses its last.
func (a *App) insertRow() {
	if a.layers != nil && !a.layering { // every layer (layers.go)
		a.everyLayerDo(a.insertRow)
		return
	}
	d := a.doc
	if a.cy >= d.rows {
		return
	}
	a.endPlay()
	a.pushUndo()
	if !a.fixedHeight() && d.rows < a.maxRows() {
		d.ensureRows(d.rows + 1)
	}
	c := d.cols
	copy(d.cells[(a.cy+1)*c:d.rows*c], d.cells[a.cy*c:(d.rows-1)*c])
	a.fillRow(a.cy, a.emptyCell())
	a.sel = nil
	a.moved()
}

// deleteColumn takes the cursor's column out of every row; the cells to its
// right move left and the last column becomes empty (the width stays).
func (a *App) deleteColumn() {
	if a.layers != nil && !a.layering { // every layer (layers.go)
		a.everyLayerDo(a.deleteColumn)
		return
	}
	d := a.doc
	a.endPlay()
	a.pushUndo()
	e := a.emptyCell()
	for y := 0; y < d.rows; y++ {
		for x := a.cx; x < d.cols-1; x++ {
			d.set(x, y, d.get(x+1, y))
		}
		d.set(d.cols-1, y, e)
	}
	a.sel = nil
}

// insertColumn puts an empty column in at the cursor; the cells to its right
// move right. A machine's page keeps its width, so its last column falls off;
// an Amiga or PC page grows a column when its last one holds something.
func (a *App) insertColumn() {
	if a.layers != nil && !a.layering { // every layer (layers.go)
		a.everyLayerDo(a.insertColumn)
		return
	}
	d := a.doc
	a.endPlay()
	a.pushUndo()
	if a.freeWidth() && d.cols < 1000 {
		for y := 0; y < d.rows; y++ {
			if !d.blankAt(d.get(d.cols-1, y)) {
				a.doc = d.withCols(d.cols + 1)
				d = a.doc
				a.msg(fmt.Sprintf("The page is %d columns wide now, so nothing falls off", d.cols))
				break
			}
		}
	}
	e := a.emptyCell()
	for y := 0; y < d.rows; y++ {
		for x := d.cols - 1; x > a.cx; x-- {
			d.set(x, y, d.get(x-1, y))
		}
		d.set(a.cx, y, e)
	}
	a.sel = nil
}

// pageTop and pageBottom: the cursor to the first cell, or to the end of the last row.
func (a *App) pageTop() {
	a.cx, a.cy = 0, 0
	a.moved()
}

func (a *App) pageBottom() {
	a.cy = a.doc.rows - 1
	a.cx = min(a.doc.cols-1, a.doc.lastCol(a.cy)+1)
	a.moved()
}
