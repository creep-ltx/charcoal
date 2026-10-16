package main

// Shapes and brushes: the Shape tool (line, box, ellipse, outline or filled,
// in the brush, in box-drawing lines or in half blocks / sixels), the shade
// brush (Draw steps a cell up or down a shade sequence), symmetry (what is
// drawn is mirrored around the page's centre) and colour-only / character-only
// painting. The options show under the brush while Draw, Shape, Colour or Half
// is the tool.

import "slices"

const (
	shapeLine = iota
	shapeBox
	shapeEllipse
)

var shapeNames = []string{"Line", "Box", "Ellipse"}

// drawOpts are the drawing options, shared by all pages (embedded in App).
type drawOpts struct {
	shape       int    // shapeLine, shapeBox or shapeEllipse
	shapeFilled bool   // boxes and ellipses filled
	shapeStyle  string // "brush", "single", "double" (box-drawing lines) or "half" (half blocks; sixels on teletext)
	mirror      int    // bit 1: left-right, bit 2: top-bottom, around the page's centre
	shade       bool   // Draw steps cells up (left) and down (right) a shade sequence
	paintOnly   string // "", "colour" (the characters stay) or "char" (the colours stay)

	shapeFrom   *[3]int      // the drag of a shape: where it started (x, y in the shape's units) and the button
	shapeFill   bool         // this drag's shape is filled (Shift at the press turns the toggle round)
	strokeSeen  map[int]bool // the cells this stroke of the shade brush has stepped already
	refMove     bool         // arrows and drags move the reference picture (refimage.go)
	refDrag     *[4]int      // a drag of the reference: the mouse and the picture's place where it started
	refDragSave bool         // the drag started from a Ctrl+press: put it in the settings when let go
}

const (
	mirrorX = 1 // left-right
	mirrorY = 2 // top-bottom
)

// shapePt is one cell (or half block, or sixel) of a shape; kind says which
// box-drawing piece it is: 0 the brush, 'h' 'v' lines, '1'-'4' the corners
// top-left, top-right, bottom-left, bottom-right.
type shapePt struct {
	x, y int
	kind byte
}

// shapePoints gives the points of a shape from (x0, y0) to (x1, y1): a line
// between them, or a box or an ellipse in the rectangle they span.
func shapePoints(shape, x0, y0, x1, y1 int, filled bool) []shapePt {
	var pts []shapePt
	seen := map[[2]int]bool{}
	add := func(x, y int, k byte) {
		if !seen[[2]int{x, y}] {
			seen[[2]int{x, y}] = true
			pts = append(pts, shapePt{x, y, k})
		}
	}
	switch shape {
	case shapeLine:
		k := byte(0)
		switch {
		case y0 == y1 && x0 != x1:
			k = 'h'
		case x0 == x1 && y0 != y1:
			k = 'v'
		}
		line(x0, y0, x1, y1, func(x, y int) { add(x, y, k) })
	case shapeBox:
		l, r, t, b := min(x0, x1), max(x0, x1), min(y0, y1), max(y0, y1)
		switch {
		case l == r && t == b:
			add(l, t, 0)
		case t == b:
			line(l, t, r, t, func(x, y int) { add(x, y, 'h') })
		case l == r:
			line(l, t, l, b, func(x, y int) { add(x, y, 'v') })
		default:
			add(l, t, '1')
			add(r, t, '2')
			add(l, b, '3')
			add(r, b, '4')
			for x := l + 1; x < r; x++ {
				add(x, t, 'h')
				add(x, b, 'h')
			}
			for y := t + 1; y < b; y++ {
				add(l, y, 'v')
				add(r, y, 'v')
			}
			if filled {
				for y := t + 1; y < b; y++ {
					for x := l + 1; x < r; x++ {
						add(x, y, 0)
					}
				}
			}
		}
	case shapeEllipse:
		outline := ellipseRect(x0, y0, x1, y1)
		for _, p := range outline {
			add(p[0], p[1], 0)
		}
		if filled { // an ellipse is convex: each row from its leftmost to its rightmost point
			span := map[int][2]int{}
			for _, p := range outline {
				s, ok := span[p[1]]
				if !ok {
					s = [2]int{p[0], p[0]}
				}
				span[p[1]] = [2]int{min(s[0], p[0]), max(s[1], p[0])}
			}
			ys := make([]int, 0, len(span))
			for y := range span {
				ys = append(ys, y)
			}
			slices.Sort(ys)
			for _, y := range ys {
				for x := span[y][0]; x <= span[y][1]; x++ {
					add(x, y, 0)
				}
			}
		}
	}
	return pts
}

// ellipseRect gives the outline of the ellipse that fills the rectangle
// (x0, y0)-(x1, y1), corners included (Alois Zingl's integer algorithm).
func ellipseRect(x0, y0, x1, y1 int) [][2]int {
	var out [][2]int
	set := func(x, y int) { out = append(out, [2]int{x, y}) }
	a, b := int64(abs(x1-x0)), int64(abs(y1-y0))
	b1 := b & 1
	dx, dy := 4*(1-a)*b*b, 4*(b1+1)*a*a
	err := dx + dy + b1*a*a
	if x0 > x1 {
		x0, x1 = x1, x0
	}
	if y0 > y1 {
		y0 = y1
	}
	y0 += int((b + 1) / 2)
	y1 = y0 - int(b1)
	a8, b8 := 8*a*a, 8*b*b
	for {
		set(x1, y0)
		set(x0, y0)
		set(x0, y1)
		set(x1, y1)
		e2 := 2 * err
		if e2 <= dy {
			y0++
			y1--
			dy += a8
			err += dy
		}
		if e2 >= dx || 2*err > dy {
			x0++
			x1--
			dx += b8
			err += dx
		}
		if x0 > x1 {
			break
		}
	}
	for int64(y0-y1) <= b { // a narrow ellipse stops early: finish its tips
		set(x0-1, y0)
		set(x1+1, y0)
		y0++
		set(x0-1, y1)
		set(x1+1, y1)
		y1--
	}
	return out
}

// symPt is a point and the mirrors that made it.
type symPt struct {
	x, y   int
	fx, fy bool
}

// symPoints gives (x, y) and its mirror images around the centre of a w x h
// grid, as the mirror option asks; a point on the centre line comes once.
func symPoints(x, y, w, h, mirror int) []symPt {
	out := []symPt{{x, y, false, false}}
	add := func(p symPt) {
		for _, q := range out {
			if q.x == p.x && q.y == p.y {
				return
			}
		}
		out = append(out, p)
	}
	if mirror&mirrorX != 0 {
		add(symPt{w - 1 - x, y, true, false})
	}
	if mirror&mirrorY != 0 {
		add(symPt{x, h - 1 - y, false, true})
	}
	if mirror == mirrorX|mirrorY {
		add(symPt{w - 1 - x, h - 1 - y, true, true})
	}
	return out
}

// mirrorCell gives the cell its mirrored character (/ and \, the box corners, half blocks).
func (a *App) mirrorCell(v uint32, fx, fy bool) uint32 {
	if a.tt() || (!fx && !fy) {
		return v
	}
	if a.uni() {
		c := uniCode(v)
		if fx {
			c = uniMirror(c, flipH)
		}
		if fy {
			c = uniMirror(c, flipV)
		}
		return uniWithCode(v, c)
	}
	f := a.font()
	c := cellCh(v)
	if fx {
		c = f.mapped(c, flipH)
	}
	if fy {
		c = f.mapped(c, flipV)
	}
	return v&^255 | uint32(c)
}

// putCell puts v at (x, y) as the paint option says: all of it, only its
// colours (the character stays) or only its character (the colours stay).
func (a *App) putCell(x, y int, v uint32) {
	if x < 0 || y < 0 || x >= a.doc.cols || y >= a.doc.rows {
		return
	}
	old := a.doc.get(x, y)
	switch a.paintOnly {
	case "colour":
		if a.atari() || a.tt() { // no colours in the cells
			return
		}
		keep := uint32(255)
		if a.screen() {
			keep = 127 // reversing belongs to the colours here
		}
		if a.uni() {
			keep |= uniHiMask // the character's high byte
		}
		v = old&keep | v&^keep
	case "char":
		keep := uint32(255)
		if a.screen() {
			keep = 127
		}
		if a.uni() {
			keep |= uniHiMask
		}
		v = v&keep | old&^keep
	}
	a.setCell(x, y, v)
}

// ---------------------------------------------------------------- the shade brush

// shadeSteps is the page's shade sequence, lightest first (prefs.json "shades" can give its own).
func (a *App) shadeSteps() []int {
	if s := prefs.Shades[a.mode]; len(s) >= 2 {
		ok := true
		for _, c := range s {
			ok = ok && c >= 0 && (c < 256 || a.uni()) && (c == 32 || a.printable(c))
		}
		if ok {
			return s
		}
	}
	switch a.mode {
	case "pc":
		return []int{32, 0xB0, 0xB1, 0xB2, 0xDB} // ░▒▓█
	case "uni":
		return []int{32, '░', '▒', '▓', '█'}
	case "c64":
		return []int{32, 0x66, 0xA0} // the checkerboard, the reversed space
	case "atari":
		return []int{32, '.', ':', '+', '#', 0xA0}
	case "zx":
		return []int{32, '.', ':', '+', '#', 0x8F} // the full block graphic
	case "cpc":
		return []int{32, 0x90, 0xCF, 0x8F} // the dot, the checkerboard, the full block
	case "tt":
		return nil
	}
	return []int{32, '.', ':', '+', '#'} // Amiga, ST
}

// shadeCell steps a cell's character one up (or down) the shade sequence. A
// character not in it goes to the first step up (and is left alone going
// down). The cell takes the brush's colours.
func (a *App) shadeCell(v uint32, up bool) (uint32, bool) {
	steps := a.shadeSteps()
	if len(steps) == 0 {
		return v, false
	}
	c := cellCh(v)
	if a.uni() {
		c = uniCode(v)
	}
	i := slices.Index(steps, c)
	if a.blankShade(v) {
		i = 0
	}
	switch {
	case i < 0 && up:
		i = 1
	case i < 0:
		return v, false
	case up && i < len(steps)-1:
		i++
	case !up && i > 0:
		i--
	default:
		return v, false
	}
	if i == 0 {
		return blank, true
	}
	rev := a.rev
	a.rev = false // the sequence says when a character is reversed
	n := a.cell(steps[i])
	a.rev = rev
	return n, true
}

// blankShade: an empty cell is the sequence's first step.
func (a *App) blankShade(v uint32) bool {
	c := cellCh(v)
	if a.uni() {
		c = uniCode(v)
	}
	return c == 32 || (c == 0 && !a.doc.glyph0)
}

// shadeAt steps the cell at (x, y) once per stroke.
func (a *App) shadeAt(x, y int, up bool) {
	if x < 0 || y < 0 || x >= a.doc.cols || y >= a.doc.rows {
		return
	}
	if a.strokeSeen == nil {
		a.strokeSeen = map[int]bool{}
	}
	k := y*a.doc.cols + x
	if a.strokeSeen[k] {
		return
	}
	a.strokeSeen[k] = true
	if n, ok := a.shadeCell(a.doc.get(x, y), up); ok {
		a.setCell(x, y, n)
	}
}

// ---------------------------------------------------------------- the Shape tool

// boxChars are the box-drawing pieces of a line style: h, v, top-left,
// top-right, bottom-left, bottom-right.
func (a *App) boxChars() []int {
	switch {
	case a.pc() && a.shapeStyle == "single":
		return []int{0xC4, 0xB3, 0xDA, 0xBF, 0xC0, 0xD9}
	case a.pc() && a.shapeStyle == "double":
		return []int{0xCD, 0xBA, 0xC9, 0xBB, 0xC8, 0xBC}
	case a.c64() && a.shapeStyle == "single": // screen codes
		return []int{0x40, 0x5D, 0x70, 0x6E, 0x6D, 0x7D}
	case a.atari() && a.shapeStyle == "single": // ATASCII
		return []int{0x12, 0x7C, 0x11, 0x05, 0x1A, 0x03}
	case a.uni() && a.shapeStyle == "single":
		return []int{'─', '│', '┌', '┐', '└', '┘'}
	case a.uni() && a.shapeStyle == "double":
		return []int{'═', '║', '╔', '╗', '╚', '╝'}
	case a.uni() && a.shapeStyle == "round":
		return []int{'─', '│', '╭', '╮', '╰', '╯'}
	case a.uni() && a.shapeStyle == "heavy":
		return []int{'━', '┃', '┏', '┓', '┗', '┛'}
	case a.cpc() && a.shapeStyle == "single": // the CPC's line characters
		return []int{0x9A, 0x95, 0x96, 0x9C, 0x93, 0x99}
	}
	return nil
}

// shapeStyles are the line styles the page offers, ids and names.
func (a *App) shapeStyles() ([]string, []string) {
	switch {
	case a.pc():
		return []string{"brush", "single", "double", "half"}, []string{"Brush", "Single lines", "Double lines", "Half blocks"}
	case a.c64(), a.atari(), a.cpc():
		return []string{"brush", "single"}, []string{"Brush", "Box lines"}
	case a.uni():
		return []string{"brush", "single", "double", "round", "heavy", "half"}, []string{"Brush", "Single lines", "Double lines", "Rounded lines", "Heavy lines", "Half blocks"}
	case a.tt():
		return []string{"brush", "half"}, []string{"Brush", "Sixels"}
	}
	return []string{"brush"}, []string{"Brush"}
}

// styleNow is the shape style that applies on this page.
func (a *App) styleNow() string {
	ids, _ := a.shapeStyles()
	if slices.Contains(ids, a.shapeStyle) {
		return a.shapeStyle
	}
	return "brush"
}

// shapeUnits turns a point of the page into the shape's units: cells, half
// cells (PC) or sixels (teletext).
func (a *App) shapeUnits(p Hover) (int, int) {
	if a.styleNow() == "half" {
		if a.tt() {
			return p.x*2 + p.sx, p.y*3 + p.sy
		}
		return p.x, p.y*2 + p.half
	}
	return p.x, p.y
}

// shapePress starts a shape at p: one undo step for all of it.
func (a *App) shapePress(p Hover, button int, shift bool) {
	a.pushUndo()
	x, y := a.shapeUnits(p)
	a.shapeFrom = &[3]int{x, y, button}
	a.shapeFill = a.shapeFilled != shift
	a.drawShapeTo(x, y)
}

// shapeMotion draws the shape again from where it started to p.
func (a *App) shapeMotion(p Hover) {
	if a.shapeFrom == nil || len(a.undo) == 0 {
		return
	}
	base := a.undo[len(a.undo)-1].doc // the page as it was at the press
	if base.cols == a.doc.cols && base.rows == a.doc.rows {
		copy(a.doc.cells, base.cells)
	} else {
		a.doc = base.clone()
	}
	x, y := a.shapeUnits(p)
	a.drawShapeTo(x, y)
}

// drawShapeTo draws the shape being dragged, from its start to (x, y).
func (a *App) drawShapeTo(x, y int) {
	f := a.shapeFrom
	style := a.styleNow()
	erase := f[2] == 3
	pts := shapePoints(a.shape, f[0], f[1], x, y, a.shapeFill)
	switch {
	case style == "half" && a.tt():
		for _, p := range pts {
			a.sixelSym(p.x, p.y, !erase)
		}
	case style == "half":
		col := a.halfFg()
		if erase {
			col = a.halfBg()
		}
		for _, p := range pts {
			a.halfSym(p.x, p.y, col)
		}
	default:
		box := a.boxChars()
		for _, p := range pts {
			v := a.brushCell()
			if box != nil && p.kind != 0 {
				v = a.cell(box[map[byte]int{'h': 0, 'v': 1, '1': 2, '2': 3, '3': 4, '4': 5}[p.kind]])
			}
			if erase {
				v = blank
			}
			for _, s := range symPoints(p.x, p.y, a.doc.cols, a.doc.rows, a.mirror) {
				a.putCell(s.x, s.y, a.mirrorCell(v, s.fx, s.fy))
			}
		}
	}
}

// shapeRelease ends the shape.
func (a *App) shapeRelease() { a.shapeFrom = nil }

// ---------------------------------------------------------------- choosing from the keys

func (a *App) shapeKey(s int) {
	a.start = false
	a.shape = s
	a.setTool("shape")
	a.msg(shapeNames[s] + " (Shape tool): drag on the page; Shift: filled; right button erases")
}

func (a *App) toggleFilled() {
	a.shapeFilled = !a.shapeFilled
	a.msg(map[bool]string{true: "Shapes filled", false: "Shapes outlined"}[a.shapeFilled])
}

func (a *App) cycleShapeStyle() {
	ids, names := a.shapeStyles()
	i := (slices.Index(ids, a.styleNow()) + 1) % len(ids)
	a.shapeStyle = ids[i]
	a.msg("Shapes in " + names[i])
}

func (a *App) toggleMirror(bit int) {
	a.mirror ^= bit
	a.msg(mirrorNames[a.mirror])
}

var mirrorNames = []string{"No mirror", "Mirror left-right", "Mirror top-bottom", "Mirror both ways"}

func (a *App) toggleShade() {
	if a.shadeSteps() == nil {
		a.msg("A teletext page has no shade characters")
		return
	}
	a.shade = !a.shade
	if a.shade && a.tool != "draw" {
		a.start = false
		a.setTool("draw")
	}
	a.msg(map[bool]string{true: "Shade brush: left steps a cell up the shades, right down", false: "Shade brush off"}[a.shade])
}

func (a *App) setPaintOnly(what string) {
	if a.paintOnly == what {
		what = ""
	}
	a.paintOnly = what
	a.msg(map[string]string{"": "Painting characters and colours", "colour": "Painting colours only: the characters stay",
		"char": "Painting characters only: the colours stay"}[what])
}

// ---------------------------------------------------------------- the options under the brush

// drawToolOptions draws the options of the tool in use from (x, y), in rows
// that end before right, and returns the y under them.
func (a *App) drawToolOptions(u *UI, x, y, right int) int {
	if a.tool != "draw" && a.tool != "shape" && a.tool != "colour" && a.tool != "half" {
		return y
	}
	x0 := x
	place := func(w int) Rect {
		if x > x0 && x+w > right {
			x, y = x0, y+smallH+4
		}
		r := Rect{x, y, w, smallH}
		x += w + 4
		return r
	}
	toggle := func(label string, on bool, hint string) bool {
		return u.button(place(buttonW(label)), label, on, hint) != nil
	}
	choice := func(id string, items []string, cur int, hint string, pick func(int)) {
		u.choice(id, place(choiceW(items)), items, cur, hint, pick)
	}
	if a.tool == "shape" {
		choice("shape", shapeNames, a.shape, "The shape: line, box or ellipse (drag on the page; "+keyName("shape_line")+", "+keyName("shape_box")+", "+keyName("shape_ellipse")+")", func(i int) { a.shape = i })
		if toggle("Filled", a.shapeFilled, "Boxes and ellipses filled; Shift at the press does the other ("+keyName("shape_filled")+")") {
			a.shapeFilled = !a.shapeFilled
		}
		if ids, names := a.shapeStyles(); len(ids) > 1 {
			choice("shapestyle", names, slices.Index(ids, a.styleNow()), "Drawn with the brush, in box-drawing lines (boxes and straight lines) or in half blocks ("+keyName("shape_style")+")", func(i int) { a.shapeStyle = ids[i] })
		}
	}
	if a.tool == "draw" {
		if toggle("Shade", a.shade, "Shade brush: left steps a cell up the shades (PC: the shade blocks to the full block), right down ("+keyName("shade_brush")+")") {
			a.toggleShade()
		}
	}
	if (a.tool == "draw" || a.tool == "shape") && !a.tt() && !a.atari() { // teletext and Atari cells hold no colours
		if toggle("Colour only", a.paintOnly == "colour", "Paint only the brush's colours: the characters stay ("+keyName("paint_colour_only")+")") {
			a.setPaintOnly("colour")
		}
		if toggle("Char only", a.paintOnly == "char", "Paint only the brush's character: the colours stay ("+keyName("paint_char_only")+")") {
			a.setPaintOnly("char")
		}
	}
	choice("mirror", mirrorNames, a.mirror, "Symmetry: what you draw is mirrored around the page's centre ("+keyName("mirror_left_right")+", "+keyName("mirror_top_bottom")+")", func(i int) { a.mirror = i })
	return y + smallH
}

// halfSym paints a half block (Half tool) and its mirror images.
func (a *App) halfSym(x, hy, colour int) {
	for _, s := range symPoints(x, hy, a.doc.cols, a.doc.rows*2, a.mirror) {
		if s.x >= 0 && s.y >= 0 && s.x < a.doc.cols && s.y < a.doc.rows*2 {
			a.paintHalf(s.x, s.y, colour)
		}
	}
}

// sixelSym sets or clears a teletext sixel (Sixel tool) and its mirror images.
func (a *App) sixelSym(gx, gy int, on bool) {
	for _, s := range symPoints(gx, gy, a.doc.cols*2, a.doc.rows*3, a.mirror) {
		if s.x >= 0 && s.y >= 0 && s.x < a.doc.cols*2 && s.y < a.doc.rows*3 {
			a.paintSixel(s.x, s.y, on)
		}
	}
}
