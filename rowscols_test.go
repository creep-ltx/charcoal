package main

import "testing"

func rowText(a *App, y int) string {
	s := ""
	for x := 0; x < a.doc.cols; x++ {
		s += string(rune(cellCh(a.doc.get(x, y))))
	}
	return s
}

func put(a *App, x, y int, s string) {
	for i, c := range s {
		a.doc.set(x+i, y, a.cell(int(c)))
	}
}

func TestRowsInsertDelete(t *testing.T) {
	a := testApp(t, "pc")
	a.doc = a.doc.withCols(10)
	put(a, 0, 0, "aaa")
	put(a, 0, 1, "bbb")
	put(a, 0, 2, "ccc")
	rows := a.doc.rows
	a.cy = 1
	editorKey("Up", false, false, true) // Alt+Up deletes the row
	if rowText(a, 1)[:3] != "ccc" || a.doc.rows != rows-1 || len(a.undo) != 1 {
		t.Errorf("delete row: row 1 %q, %d rows (was %d), %d undo", rowText(a, 1), a.doc.rows, rows, len(a.undo))
	}
	editorKey("Down", false, false, true) // Alt+Down inserts one
	if rowText(a, 1)[:3] != "   " || rowText(a, 2)[:3] != "ccc" || a.doc.rows != rows {
		t.Errorf("insert row: %q %q, %d rows", rowText(a, 1), rowText(a, 2), a.doc.rows)
	}
	a.doUndo()
	a.doUndo()
	if rowText(a, 1)[:3] != "bbb" || a.doc.rows != rows {
		t.Error("undo did not bring the row back")
	}
}

func TestRowsFixedHeight(t *testing.T) {
	for _, mode := range []string{"zx", "tt"} {
		a := testApp(t, mode)
		rows := a.doc.rows
		put(a, 0, rows-1, "zz")
		a.cy = 0
		a.insertRow()
		if a.doc.rows != rows || cellCh(a.doc.get(0, rows-1)) == 'z' {
			t.Errorf("%s insert row: %d rows, last row %q", mode, a.doc.rows, rowText(a, rows-1))
		}
		a.deleteRow()
		if a.doc.rows != rows || a.doc.get(0, rows-1) != a.emptyCell() {
			t.Errorf("%s delete row: %d rows", mode, a.doc.rows)
		}
	}
}

func TestColumnsInsertDelete(t *testing.T) {
	a := testApp(t, "c64")
	cols := a.doc.cols
	put(a, 0, 0, "abc")
	a.doc.set(cols-1, 0, a.cell('z'))
	a.cx = 1
	editorKey("Right", false, false, true) // Alt+Right inserts a column
	if a.doc.cols != cols || rowText(a, 0)[:4] != "a bc" || cellCh(a.doc.get(cols-1, 0)) == 'z' {
		t.Errorf("C64 insert column: %d cols, %q", a.doc.cols, rowText(a, 0))
	}
	editorKey("Left", false, false, true) // Alt+Left deletes it
	if rowText(a, 0)[:3] != "abc" || a.doc.cols != cols {
		t.Errorf("delete column: %q", rowText(a, 0))
	}

	a = testApp(t, "pc")
	put(a, 0, 0, "abc")
	a.cx = 0
	a.insertColumn() // the last column is empty: the width stays
	if a.doc.cols != 80 || rowText(a, 0)[:4] != " abc" {
		t.Errorf("PC insert column: %d cols %q", a.doc.cols, rowText(a, 0)[:4])
	}
	a.doc.set(79, 0, a.cell('z'))
	a.insertColumn() // something in the last column: the page grows
	if a.doc.cols != 81 || cellCh(a.doc.get(80, 0)) != 'z' {
		t.Errorf("PC insert column with a full last column: %d cols", a.doc.cols)
	}
}

func TestPageTopBottom(t *testing.T) {
	a := testApp(t, "pc")
	put(a, 0, 24, "end")
	a.cx, a.cy = 5, 5
	editorKey("End", true, false, false)
	if a.cx != 3 || a.cy != 24 {
		t.Errorf("Ctrl+End: %d,%d", a.cx, a.cy)
	}
	editorKey("Home", true, false, false)
	if a.cx != 0 || a.cy != 0 {
		t.Errorf("Ctrl+Home: %d,%d", a.cx, a.cy)
	}
}
