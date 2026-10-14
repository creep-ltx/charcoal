package main

import (
	"slices"
	"testing"
)

func ptSet(pts []shapePt) map[[2]int]byte {
	m := map[[2]int]byte{}
	for _, p := range pts {
		m[[2]int{p.x, p.y}] = p.kind
	}
	return m
}

func TestShapeLine(t *testing.T) {
	pts := shapePoints(shapeLine, 0, 0, 6, 3, false)
	if len(pts) != 7 || pts[0] != (shapePt{0, 0, 0}) || pts[6] != (shapePt{6, 3, 0}) {
		t.Fatalf("diagonal line: %v", pts)
	}
	for i := 1; i < len(pts); i++ { // each step to a neighbour
		if abs(pts[i].x-pts[i-1].x) > 1 || abs(pts[i].y-pts[i-1].y) > 1 {
			t.Fatalf("gap in %v", pts)
		}
	}
	if h := shapePoints(shapeLine, 5, 2, 1, 2, false); len(h) != 5 || h[2].kind != 'h' {
		t.Fatalf("horizontal line: %v", h)
	}
	if v := shapePoints(shapeLine, 3, 0, 3, 4, false); len(v) != 5 || v[0].kind != 'v' {
		t.Fatalf("vertical line: %v", v)
	}
	if p := shapePoints(shapeLine, 2, 2, 2, 2, false); len(p) != 1 || p[0].kind != 0 {
		t.Fatalf("a point: %v", p)
	}
}

func TestShapeBox(t *testing.T) {
	out := ptSet(shapePoints(shapeBox, 4, 3, 0, 0, false)) // dragged from the bottom right
	if len(out) != 2*(5+4)-4 {
		t.Fatalf("outline of 5x4: %d cells", len(out))
	}
	for p, k := range map[[2]int]byte{{0, 0}: '1', {4, 0}: '2', {0, 3}: '3', {4, 3}: '4', {2, 0}: 'h', {0, 1}: 'v', {4, 2}: 'v', {1, 3}: 'h'} {
		if out[p] != k {
			t.Errorf("box %v is %q, want %q", p, out[p], k)
		}
	}
	if _, in := out[[2]int{2, 1}]; in {
		t.Error("the outline has an inside")
	}
	filled := ptSet(shapePoints(shapeBox, 0, 0, 4, 3, true))
	if len(filled) != 20 || filled[[2]int{2, 1}] != 0 || filled[[2]int{0, 0}] != '1' {
		t.Fatalf("filled 5x4: %d cells", len(filled))
	}
	if row := shapePoints(shapeBox, 0, 0, 3, 0, false); len(row) != 4 || row[0].kind != 'h' {
		t.Fatalf("a box one row high: %v", row)
	}
}

func TestShapeEllipse(t *testing.T) {
	rects := [][4]int{{3, 2, 3, 2}, {0, 0, 40, 2}, {0, 0, 2, 40}, {5, 7, 65, 31}}
	for w := 0; w <= 14; w++ {
		for h := 0; h <= 14; h++ {
			rects = append(rects, [4]int{1, 2, 1 + w, 2 + h})
		}
	}
	for _, r := range rects {
		for _, filled := range []bool{false, true} {
			set := ptSet(shapePoints(shapeEllipse, r[0], r[1], r[2], r[3], filled))
			minX, maxX, minY, maxY := 1<<30, -1<<30, 1<<30, -1<<30
			for p := range set {
				minX, maxX, minY, maxY = min(minX, p[0]), max(maxX, p[0]), min(minY, p[1]), max(maxY, p[1])
				// Symmetric in both directions.
				if _, ok := set[[2]int{r[0] + r[2] - p[0], p[1]}]; !ok {
					t.Errorf("%v: %v has no left-right twin", r, p)
				}
				if _, ok := set[[2]int{p[0], r[1] + r[3] - p[1]}]; !ok {
					t.Errorf("%v: %v has no top-bottom twin", r, p)
				}
			}
			if minX != r[0] || maxX != r[2] || minY != r[1] || maxY != r[3] {
				t.Errorf("%v filled %v: spans %d-%d x %d-%d", r, filled, minX, maxX, minY, maxY)
			}
			if filled { // every row without gaps
				for y := minY; y <= maxY; y++ {
					var xs []int
					for p := range set {
						if p[1] == y {
							xs = append(xs, p[0])
						}
					}
					slices.Sort(xs)
					if len(xs) == 0 || xs[len(xs)-1]-xs[0]+1 != len(xs) {
						t.Errorf("%v row %d: %v", r, y, xs)
					}
				}
			}
		}
	}
	// The outline of a 11x7 ellipse is a ring: its middle is empty.
	if _, in := ptSet(shapePoints(shapeEllipse, 0, 0, 10, 6, false))[[2]int{5, 3}]; in {
		t.Error("the outline has its middle")
	}
}

func TestSymPoints(t *testing.T) {
	if p := symPoints(1, 2, 10, 6, 0); len(p) != 1 {
		t.Fatalf("no mirror: %v", p)
	}
	p := symPoints(1, 2, 10, 6, mirrorX|mirrorY)
	want := []symPt{{1, 2, false, false}, {8, 2, true, false}, {1, 3, false, true}, {8, 3, true, true}}
	if !slices.Equal(p, want) {
		t.Fatalf("both ways: %v", p)
	}
	if p := symPoints(4, 0, 9, 6, mirrorX); len(p) != 1 { // the middle column of 9
		t.Fatalf("on the centre line: %v", p)
	}
}

func TestMirrorDrawing(t *testing.T) {
	a := testApp(t, "pc")
	a.mirror = mirrorX | mirrorY
	a.brush = '/'
	a.pushUndo()
	a.paintAt(1, 1, 1)
	W, H := a.doc.cols, a.doc.rows
	for _, c := range []struct {
		x, y int
		ch   int
	}{{1, 1, '/'}, {W - 2, 1, '\\'}, {1, H - 2, '\\'}, {W - 2, H - 2, '/'}} {
		if got := cellCh(a.doc.get(c.x, c.y)); got != c.ch {
			t.Errorf("(%d,%d) is %q, want %q", c.x, c.y, got, c.ch)
		}
	}
}

func TestPaintOnly(t *testing.T) {
	a := testApp(t, "pc")
	a.doc.set(0, 0, makeCell('A', 2, 1, 0))
	a.fg, a.bg, a.brush = 4, 5, 'B'
	a.paintOnly = "colour"
	a.paintAt(0, 0, 1)
	if v := a.doc.get(0, 0); cellCh(v) != 'A' || cellFg(v) != 4 || cellBg(v) != 5 {
		t.Errorf("colour only: %c %d %d", cellCh(v), cellFg(v), cellBg(v))
	}
	a.fg, a.bg = 6, 0
	a.paintOnly = "char"
	a.paintAt(0, 0, 1)
	if v := a.doc.get(0, 0); cellCh(v) != 'B' || cellFg(v) != 4 || cellBg(v) != 5 {
		t.Errorf("character only: %c %d %d", cellCh(v), cellFg(v), cellBg(v))
	}
	c := testApp(t, "c64") // reversing goes with the colours on a PETSCII page
	c.doc.set(0, 0, makeCell(0x81, 2, 0, 0))
	c.fg, c.brush = 5, 0x02
	c.paintOnly = "char"
	c.paintAt(0, 0, 1)
	if v := c.doc.get(0, 0); cellCh(v) != 0x82 || cellFg(v) != 2 {
		t.Errorf("PETSCII character only: %#x %d", cellCh(v), cellFg(v))
	}
}

func TestShadeSteps(t *testing.T) {
	a := testApp(t, "pc")
	a.fg, a.bg = 7, 1
	a.tool, a.shade = "draw", true
	stroke := func(button int) {
		a.strokeSeen = nil
		a.paintAt(3, 0, button)
		a.paintAt(3, 0, button) // the same stroke: one step
	}
	for _, want := range []int{0xB0, 0xB1, 0xB2, 0xDB, 0xDB} {
		stroke(1)
		if got := cellCh(a.doc.get(3, 0)); got != want {
			t.Fatalf("up: %#x, want %#x", got, want)
		}
	}
	if cellBg(a.doc.get(3, 0)) != 1 {
		t.Error("the shade does not take the brush's colours")
	}
	for _, want := range []int{0xB2, 0xB1, 0xB0, 32, 32} {
		stroke(3)
		if got := cellCh(a.doc.get(3, 0)); got != want {
			t.Fatalf("down: %#x, want %#x", got, want)
		}
	}
	a.doc.set(5, 0, makeCell('x', 7, 0, 0))
	if _, ok := a.shadeCell(a.doc.get(5, 0), false); ok {
		t.Error("down from a character not in the sequence")
	}
	if v, ok := a.shadeCell(a.doc.get(5, 0), true); !ok || cellCh(v) != 0xB0 {
		t.Error("up from a character not in the sequence")
	}
	prefs.Shades = map[string][]int{"pc": {32, '.', 'o', 'O'}}
	if s := a.shadeSteps(); !slices.Equal(s, []int{32, '.', 'o', 'O'}) {
		t.Errorf("own sequence: %v", s)
	}
	prefs.Shades = map[string][]int{"pc": {32, 999}}
	if s := a.shadeSteps(); s[1] != 0xB0 {
		t.Errorf("a bad sequence is not used: %v", s)
	}
	c := testApp(t, "c64")
	if v, _ := c.shadeCell(blank, true); cellCh(v) != 0x66 {
		t.Errorf("C64 first step %#x", cellCh(v))
	}
	if v, _ := c.shadeCell(makeCell(0x66, 1, 0, 0), true); cellCh(v) != 0xA0 {
		t.Errorf("C64 second step %#x", cellCh(v))
	}
}

func TestShapeOneUndoStep(t *testing.T) {
	a := testApp(t, "pc")
	a.setTool("shape")
	a.shape, a.shapeStyle = shapeBox, "single"
	n := len(a.undo)
	a.shapePress(Hover{x: 1, y: 1}, 1, false)
	for x := 2; x < 12; x++ { // dragging draws it again each time, from the page as it was
		a.shapeMotion(Hover{x: x, y: x / 2})
	}
	a.shapeMotion(Hover{x: 6, y: 4})
	a.shapeRelease()
	if len(a.undo) != n+1 {
		t.Fatalf("%d undo steps for one box", len(a.undo)-n)
	}
	if cellCh(a.doc.get(1, 1)) != 0xDA || cellCh(a.doc.get(6, 4)) != 0xD9 || cellCh(a.doc.get(3, 1)) != 0xC4 || cellCh(a.doc.get(6, 2)) != 0xB3 {
		t.Fatal("not a box of single lines")
	}
	if !a.doc.blankAt(a.doc.get(11, 5)) || !a.doc.blankAt(a.doc.get(3, 2)) {
		t.Fatal("an earlier size of the box is left on the page")
	}
	a.doUndo()
	if !a.doc.empty() {
		t.Fatal("undo leaves part of the box")
	}

	// A shape on the second frame: undone there, the first frame untouched.
	a.typeByte('A')
	a.newFrame()
	a.shape, a.shapeStyle = shapeEllipse, "brush"
	a.shapePress(Hover{x: 0, y: 0}, 1, true)
	a.shapeMotion(Hover{x: 8, y: 6})
	a.shapeRelease()
	if a.cur != 1 || a.doc.blankAt(a.doc.get(4, 3)) {
		t.Fatal("the filled ellipse is not on frame 2")
	}
	a.doUndo()
	if a.cur != 1 || !a.doc.empty() || cellCh(a.frames[0].doc.get(0, 0)) != 'A' {
		t.Fatal("undoing the ellipse")
	}
}

func TestShapeHalfAndSixels(t *testing.T) {
	a := testApp(t, "pc")
	a.setTool("shape")
	a.shape, a.shapeStyle, a.fg = shapeLine, "half", 4
	a.shapePress(Hover{x: 0, y: 0, half: 1}, 1, false) // the bottom half of row 0
	a.shapeMotion(Hover{x: 3, y: 0, half: 1})
	for x := 0; x <= 3; x++ {
		if v := a.doc.get(x, 0); cellCh(v) != 0xDF || cellFg(v) != 0 || cellBg(v) != 4 { // top black, bottom 4
			t.Fatalf("half block %d: %#x fg %d bg %d", x, cellCh(v), cellFg(v), cellBg(v))
		}
	}
	tt := testApp(t, "tt")
	tt.setTool("shape")
	tt.shape, tt.shapeStyle = shapeLine, "half"
	tt.shapePress(Hover{x: 0, y: 0, sx: 0, sy: 0}, 1, false)
	tt.shapeMotion(Hover{x: 1, y: 0, sx: 1, sy: 0}) // the top sixel row of two cells
	for x := 0; x < 2; x++ {
		if c := cellCh(tt.doc.get(x, 0)); c != 0x23 {
			t.Fatalf("sixels of cell %d: %#x", x, c)
		}
	}
}

func TestShapeKeys(t *testing.T) {
	a := testApp(t, "pc")
	for _, id := range []string{"shape_line", "shape_box", "shape_ellipse", "shape_filled", "shape_style", "shade_brush",
		"mirror_left_right", "mirror_top_bottom", "paint_colour_only", "paint_char_only"} {
		if actionByID[id] == nil || len(keysNow[id]) == 0 {
			t.Errorf("%s has no key", id)
		}
	}
	runKey(keysNow["shape_ellipse"][0])
	if a.tool != "shape" || a.shape != shapeEllipse {
		t.Error("the ellipse key")
	}
	runKey(keysNow["shape_style"][0])
	if a.shapeStyle != "single" {
		t.Errorf("style after one step: %s", a.shapeStyle)
	}
	runKey(keysNow["mirror_top_bottom"][0])
	if a.mirror != mirrorY {
		t.Error("the mirror key")
	}
}
