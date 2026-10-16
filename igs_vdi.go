package main

// GEM VDI's drawing, as IGS uses it (igs.go): lines with widths, patterns and arrow or round ends,
// rectangles, circles, ellipses, arcs and pie slices from VDI's 1800-step sine table, polygons filled
// scan line by scan line from the bottom up, flood fill, polymarkers, text in the system fonts with GEM's
// effects, and blits. Ported from IcyDraw's engine (mkrueger/icy_tools, Apache-2.0/MIT, vdi_paint/*.rs and
// util.rs; its names are GEM VDI's routines). Every table here is icy_tools'.

import (
	"math"
	"math/bits"
	"slices"
)

// ---------------------------------------------------------------- pixels

func (vm *igsVM) set(x, y int, c uint8) {
	if x < 0 || y < 0 || x >= vm.w || y >= vm.h {
		return
	}
	vm.work++
	vm.pix[y*vm.w+x] = c
}

// setMode sets a pixel in the writing mode (M): replace, transparent (only set bits), XOR, reverse
// transparent (only unset bits).
func (vm *igsVM) setMode(x, y int, c uint8, solid bool) {
	if x < 0 || y < 0 || x >= vm.w || y >= vm.h {
		return
	}
	vm.work++
	o := y*vm.w + x
	switch vm.drawMode {
	case 2:
		if !solid {
			return
		}
	case 3:
		c ^= vm.pix[o]
	case 4:
		if c != 0 {
			return
		}
	}
	vm.pix[o] = c & uint8(vm.colours()-1)
}

func (vm *igsVM) fillPattern() []uint16 {
	switch vm.fillKind {
	case 0:
		return []uint16{0}
	case 2:
		return igsTypePatterns[max(0, min(23, vm.fillIndex-1))][:]
	case 3:
		if i := vm.fillIndex - 1; i < 6 {
			return igsHatches[max(0, i)][:]
		} else if i < 12 {
			return igsWideHatches[i-6][:]
		}
	case 4:
		if p := vm.userPat[vm.fillIndex&7]; len(p) > 0 {
			return p
		}
	case 5:
		return igsRandomPattern[:]
	case 6:
		return igsStarTrek[:]
	}
	return []uint16{0xFFFF}
}

// fillPixel sets a pixel of a filled shape: the fill pattern's bit for it (patterns are fixed to the
// screen, 16 pixels wide) gives the fill colour or 0.
func (vm *igsVM) fillPixel(x, y int) {
	if x < 0 || y < 0 || x >= vm.w || y >= vm.h {
		return
	}
	p := vm.fillPattern()
	on := p[y%len(p)]&(0x8000>>(x%16)) != 0
	c := uint8(0)
	if on {
		c = vm.fillColor
	}
	vm.setMode(x, y, c, on)
}

// ---------------------------------------------------------------- lines

func (vm *igsVM) lineMask() uint16 {
	return [8]uint16{0xFFFF, 0xFFFF, 0xFFF0, 0xC0C0, 0xFF18, 0xFF00, 0xF191, vm.userMask}[max(0, min(7, vm.lineKind))]
}

func (vm *igsVM) vline(x, y0, y1 int, c uint8, mask uint16) {
	if y1 < y0 {
		y0, y1 = y1, y0
	}
	for y := y0; y <= y1; y++ {
		mask = bits.RotateLeft16(mask, 1)
		if mask&1 != 0 {
			vm.set(x, y, c)
		}
		if vm.busy() {
			return
		}
	}
}

func (vm *igsVM) hline(y, x0, x1 int, c uint8, mask uint16) {
	mask = bits.RotateLeft16(mask, x0&15)
	for x := x0; x <= x1; x++ {
		mask = bits.RotateLeft16(mask, 1)
		if mask&1 != 0 {
			vm.set(x, y, c)
		}
		if vm.busy() {
			return
		}
	}
}

// line is VDI's thin line (Bresenham, the pattern stepped per pixel), always in replace mode.
func (vm *igsVM) line(x0, y0, x1, y1 int, c uint8, mask uint16) {
	if x1 < x0 {
		x0, y0, x1, y1 = x1, y1, x0, y0
	}
	if x0 == x1 {
		vm.vline(x0, y0, y1, c, mask)
		return
	}
	if y0 == y1 {
		vm.hline(y0, x0, x1, c, mask)
		return
	}
	dx, dy := x1-x0, y1-y0
	yinc := 1
	if dy < 0 {
		dy, yinc = -dy, -1
	}
	x, y := x0, y0
	if dx >= dy {
		eps, e1, e2 := -dx, 2*dy, 2*dx
		for ; dx >= 0; dx-- {
			mask = bits.RotateLeft16(mask, 1)
			if mask&1 != 0 {
				vm.set(x, y, c)
			}
			x++
			if eps += e1; eps >= 0 {
				eps -= e2
				y += yinc
			}
		}
	} else {
		eps, e1, e2 := -dy, 2*dx, 2*dy
		for ; dy >= 0; dy-- {
			mask = bits.RotateLeft16(mask, 1)
			if mask&1 != 0 {
				vm.set(x, y, c)
			}
			y += yinc
			if eps += e1; eps >= 0 {
				eps -= e2
				x++
			}
		}
	}
}

// polyline draws thin lines through the points (x, y pairs), closed back to the first if close.
func (vm *igsVM) polyline(p []int, c uint8, close bool) {
	if len(p) < 2 {
		return
	}
	mask := vm.lineMask()
	x, y := p[0], p[1]
	for i := 2; i+1 < len(p); i += 2 {
		vm.line(x, y, p[i], p[i+1], c, mask)
		x, y = p[i], p[i+1]
	}
	if close {
		vm.line(x, y, p[0], p[1], c, mask)
	}
}

// pixelSize: a pixel's width and height in micrometres, as VDI gives them per resolution.
func (vm *igsVM) pixelSize() (int, int) {
	return [3]int{338, 169, 372}[vm.res], 372
}

// multDiv is a*b/c rounded half away from zero, as VDI's SMUL_DIV.
func multDiv(a, b, c int) int {
	if c == 0 {
		return 0
	}
	p := int64(a) * int64(b)
	d := int64(c)
	ap, ad := p, d
	if ap < 0 {
		ap = -ap
	}
	if ad < 0 {
		ad = -ad
	}
	r := (2*ap/ad + 1) / 2
	if (p < 0) != (d < 0) && p != 0 {
		r = -r
	}
	return int(r)
}

// quarterCircle is a wide line's pen: the half width of each row above and below its centre.
func quarterCircle(width, xs, ys int) []int {
	rows := (width*xs/ys)/2 + 1
	radius := (width + 1) / 2
	circle := make([]int, max(radius+2, rows)*4)
	x, y := 0, radius
	d := 3 - 2*y
	for x < y {
		circle[y], circle[x] = x, y
		if d < 0 {
			d += 4*x + 6
		} else {
			d += 4*(x-y) + 10
			y--
		}
		x++
	}
	if x == y {
		circle[x] = x
	}
	low := 0
	out := make([]int, 0, rows)
	for i := 0; i < rows; i++ {
		high := max(((2*i+1)*ys/xs)/2, low)
		sum := 0
		for j := low; j <= high; j++ {
			if j < len(circle) {
				sum += circle[j]
			}
		}
		out = append(out, sum/(high-low+1))
		low = high + 1
	}
	return out
}

func quadTransform(q, x, y int) (int, int) {
	if q != 1 && q != 4 {
		x = -x
	}
	if q != 1 && q != 2 {
		y = -y
	}
	return x, y
}

// perpOffset is the point of the pen's quarter circle nearest to direction (x, y) (VDI's perp_off).
func perpOffset(x, y int, off []int) (int, int) {
	q := 4
	switch {
	case x >= 0 && y >= 0:
		q = 1
	case x < 0 && y >= 0:
		q = 2
	case x < 0 && y < 0:
		q = 3
	}
	x, y = quadTransform(q, x, y)
	last := len(off) - 1
	u, v, idx := off[0], 0, 0
	best, xv, yv := math.MaxInt, 0, 0
	for {
		m := abs(u*y - v*x)
		if m < best || (m == best && abs(xv-yv) > abs(u-v)) {
			best, xv, yv = m, u, v
		} else {
			break
		}
		if v == last {
			if u <= 1 {
				break
			}
			u--
		} else if off[idx+1] >= u-1 {
			v++
			idx++
			u = off[idx]
		} else {
			u--
		}
	}
	return quadTransform(q, xv, yv)
}

// fillSolid fills a polygon and its outline in one colour, as VDI fills wide lines and arrow heads.
func (vm *igsVM) fillSolid(p []int, c uint8) {
	if len(p) < 6 {
		return
	}
	k, i, col, b, lk := vm.fillKind, vm.fillIndex, vm.fillColor, vm.fillBorder, vm.lineKind
	vm.fillKind, vm.fillColor, vm.fillBorder, vm.lineKind = 1, c, true, 1
	vm.fillPoly(p)
	vm.fillKind, vm.fillIndex, vm.fillColor, vm.fillBorder, vm.lineKind = k, i, col, b, lk
}

func (vm *igsVM) pen(x, y int, off []int, c uint8) {
	for row, o := range off {
		ys := []int{y - row, y + row}
		if row == 0 {
			ys = ys[:1]
		}
		for _, yy := range ys {
			for xx := x - o; xx <= x+o; xx++ {
				vm.setMode(xx, yy, c, true)
			}
		}
	}
}

// styledLine draws lines through the points with the line attributes: type, width (only solid lines are
// wide, in odd widths) and ends.
func (vm *igsVM) styledLine(flat []int, c uint8) {
	var pts [][2]int
	for i := 0; i+1 < len(flat); i += 2 {
		pts = append(pts, [2]int{flat[i], flat[i+1]})
	}
	if len(pts) < 2 {
		return
	}
	width := 1
	if vm.lineKind == 1 {
		width = (max(1, vm.lineWidth)-1)/2*2 + 1
	}
	if vm.lineEnds[0] == 1 {
		vm.arrowHead(pts, true, width, c)
	}
	if vm.lineEnds[1] == 1 {
		vm.arrowHead(pts, false, width, c)
	}
	if width <= 1 {
		var p []int
		for _, q := range pts {
			p = append(p, q[0], q[1])
		}
		vm.polyline(p, c, false)
		return
	}
	xs, ys := vm.pixelSize()
	off := quarterCircle(width, xs, ys)
	x1, y1 := pts[0][0], pts[0][1]
	if vm.lineEnds[0] != 0 {
		vm.pen(x1, y1, off, c)
	}
	for i := 1; i < len(pts); i++ {
		x2, y2 := pts[i][0], pts[i][1]
		vx, vy := x2-x1, y2-y1
		if vx == 0 && vy == 0 {
			continue
		}
		switch {
		case vx == 0:
			vx, vy = off[0], 0
		case vy == 0:
			vx, vy = 0, len(off)-1
		default:
			px := multDiv(-vy, ys, xs)
			vy = multDiv(vx, xs, ys)
			vx, vy = perpOffset(px, vy, off)
		}
		vm.fillSolid([]int{x1 + vx, y1 + vy, x1 - vx, y1 - vy, x2 - vx, y2 - vy, x2 + vx, y2 + vy}, c)
		if i < len(pts)-1 || vm.lineEnds[1] != 0 {
			vm.pen(x2, y2, off, c)
		}
		x1, y1 = x2, y2
	}
}

// arrowHead draws an arrow at the start or end of pts and moves that end back to the arrow's base.
func (vm *igsVM) arrowHead(pts [][2]int, start bool, width int, c uint8) {
	xs, ys := vm.pixelSize()
	length := 8
	if width >= 4 {
		length = 3*width - 1
	}
	half := length / 2
	last := len(pts) - 1
	tip := pts[last]
	if start {
		tip = pts[0]
	}
	reached, dx, dy, dist := -1, 0, 0, 0
	for k := 0; k < last; k++ {
		i := last - 1 - k
		if start {
			i = k + 1
		}
		dx = tip[0] - pts[i][0]
		dy = multDiv(tip[1]-pts[i][1], ys, xs)
		dist = int(math.Hypot(float64(dx), float64(dy)))
		if dist >= length {
			reached = i
			break
		}
	}
	if reached < 0 {
		return
	}
	hx := multDiv(length, multDiv(dx, 1000, dist), 1000)
	hy := multDiv(multDiv(length, multDiv(dy, 1000, dist), 1000), xs, ys)
	bx := multDiv(half, multDiv(dy, -1000, dist), 1000)
	by := multDiv(multDiv(half, multDiv(dx, 1000, dist), 1000), xs, ys)
	vm.fillSolid([]int{tip[0] + bx - hx, tip[1] + by - hy, tip[0] - bx - hx, tip[1] - by - hy, tip[0], tip[1]}, c)
	base := [2]int{tip[0] - hx, tip[1] - hy}
	if start {
		for i := 0; i < reached; i++ {
			pts[i] = base
		}
	} else {
		for i := reached + 1; i <= last; i++ {
			pts[i] = base
		}
	}
}

// marker draws the polymarker at (x, y): point, plus, star, square, diagonal cross or diamond.
func (vm *igsVM) marker(x, y int) {
	p := igsMarkers[max(1, min(6, vm.markerType))-1]
	n, i := p[0], 1
	lk := vm.lineKind
	vm.lineKind = 1
	for k := 0; k < n && i < len(p); k++ {
		cnt := p[i]
		i++
		var q []int
		for j := 0; j < cnt && i+1 < len(p); j++ {
			q = append(q, vm.markerSize*p[i]+x, vm.markerSize*p[i+1]+y)
			i += 2
		}
		vm.polyline(q, vm.markerColor, false)
	}
	vm.lineKind = lk
}

var igsMarkers = [6][]int{
	{1, 2, 0, 0, 0, 0},
	{2, 2, 0, -3, 0, 3, 2, -4, 0, 4, 0},
	{3, 2, 0, -3, 0, 3, 2, 3, 2, -3, -2, 2, 3, -2, -3, 2},
	{1, 5, -4, -3, 4, -3, 4, 3, -4, 3, -4, -3},
	{2, 2, -4, -3, 4, 3, 2, -4, 3, 4, -3},
	{1, 5, -4, 0, 0, -3, 4, 0, 0, 3, -4, 0},
}

// ---------------------------------------------------------------- filled shapes

// fillPoly fills a polygon (x, y pairs) scan line by scan line from the bottom up, as VDI does, and
// draws its outline in the fill colour if the border is on.
func (vm *igsVM) fillPoly(p []int) {
	if len(p) < 4 {
		return
	}
	ymin, ymax := p[1], p[1]
	for i := 3; i < len(p); i += 2 {
		ymin, ymax = min(ymin, p[i]), max(ymax, p[i])
	}
	n := len(p) / 2
	var edges []int
	for y := min(ymax, vm.h-1); y >= max(ymin+1, 0); y-- {
		if vm.busy() {
			return
		}
		edges = edges[:0]
		for i := 0; i < n && len(edges) < 512; i++ {
			j := (i + 1) % n
			y1, y2 := p[i*2+1], p[j*2+1]
			dy, dy1, dy2 := y2-y1, y-y1, y-y2
			if dy1^dy2 < 0 {
				x1, x2 := p[i*2], p[j*2]
				dx := (x2 - x1) << 1
				if dx < 0 {
					edges = append(edges, ((dy2*dx/dy+1)>>1)+x2)
				} else {
					edges = append(edges, ((dy1*dx/dy+1)>>1)+x1)
				}
			}
		}
		if len(edges) < 2 {
			continue
		}
		slices.Sort(edges)
		for k := 0; k+1 < len(edges); k += 2 {
			for x := max(edges[k], 0); x <= min(edges[k+1], vm.w-1); x++ {
				vm.fillPixel(x, y)
			}
		}
	}
	if vm.fillBorder {
		vm.polyline(p, vm.fillColor, true)
	}
}

func (vm *igsVM) fillRect(l, t, r, b int) {
	if t > b {
		t, b = b, t
	}
	if l > r {
		l, r = r, l
	}
	for y := max(t, 0); y <= min(b, vm.h-1); y++ {
		for x := max(l, 0); x <= min(r, vm.w-1); x++ {
			vm.fillPixel(x, y)
		}
	}
}

// rect is B: a filled rectangle, its border in the fill colour if the border is on.
func (vm *igsVM) rect(l, t, r, b int) {
	vm.fillRect(l, t, r, b)
	if !vm.fillBorder {
		return
	}
	if t > b {
		t, b = b, t
	}
	if l > r {
		l, r = r, l
	}
	c := vm.fillColor
	vm.hline(t, l, r, c, 0xFFFF)
	if b != t {
		vm.hline(b, l, r, c, 0xFFFF)
	}
	if b-t > 1 {
		vm.vline(l, t+1, b-1, c, 0xFFFF)
		if r != l {
			vm.vline(r, t+1, b-1, c, 0xFFFF)
		}
	}
}

func (vm *igsVM) roundRectPoints(l, t, r, b int) []int {
	if l > r {
		l, r = r, l
	}
	if t < b { // VDI's names: "top" is the larger y here
		t, b = b, t
	}
	xr := max(0, min(vm.w>>6, (r-l)/2)-1)
	yr := min(vm.circleY(xr), (t-b)/2)
	const s225, s450, s675 = 12539, 23170, 30273
	xo := []int{0, s225 * xr / 32767, s450 * xr / 32767, s675 * xr / 32767, xr}
	yo := []int{yr, s675 * yr / 32767, s450 * yr / 32767, s225 * yr / 32767, 0}
	var p []int
	xc, yc := r-xr, b+yr
	for i := 0; i < 5; i++ {
		p = append(p, xc+xo[i], yc-yo[i])
	}
	yc = t - yr
	for i := 0; i < 5; i++ {
		p = append(p, xc+xo[4-i], yc+yo[4-i])
	}
	xc = l + xr
	for i := 0; i < 5; i++ {
		p = append(p, xc-xo[i], yc+yo[i])
	}
	yc = b + yr
	for i := 0; i < 5; i++ {
		p = append(p, xc-xo[4-i], yc-yo[4-i])
	}
	return append(p, p[0], p[1])
}

// roundRect is U (and B with its rounded flag): filled, the border drawn if on.
func (vm *igsVM) roundRect(l, t, r, b int) {
	p := vm.roundRectPoints(l, t, r, b)
	vm.fillPoly(p)
	if vm.fillBorder {
		vm.polyline(p, vm.fillColor, false)
	}
}

// circleY is a circle's height for radius r: pixels are not square.
func (vm *igsVM) circleY(r int) int {
	xs, ys := vm.pixelSize()
	return r * xs / ys
}

func (vm *igsVM) circle(x, y, r int) {
	p := igsCurve(x, y, r, vm.circleY(r), 0, 3600)
	vm.fillPoly(p)
	if vm.fillBorder {
		vm.polyline(p, vm.fillColor, false)
	}
}

func (vm *igsVM) ellipse(x, y, rx, ry int) {
	p := igsCurve(x, y, rx, ry, 0, 3600)
	vm.fillPoly(p)
	if vm.fillBorder {
		vm.polyline(p, vm.fillColor, false)
	}
}

// arc is K and J: angles in degrees, drawn as a line with the line attributes.
func (vm *igsVM) arc(x, y, rx, ry, a0, a1 int) {
	vm.styledLine(igsCurve(x, y, rx, ry, a0*10, a1*10), vm.lineColor)
}

// pie is V and Y: a filled slice, its outline in the fill colour if the border is on.
func (vm *igsVM) pie(x, y, rx, ry, a0, a1 int) {
	p := append(igsCurve(x, y, rx, ry, a0*10, a1*10), x, y)
	vm.fillPoly(p)
	if vm.fillBorder {
		vm.polyline(p, vm.fillColor, true)
	}
}

// ---------------------------------------------------------------- VDI's sines (1800 steps to pi)

var igsSines = [113]int{
	0, 915, 1830, 2744, 3658, 4572, 5484, 6395, 7305, 8214, 9121, 10026, 10929, 11831, 12729, 13626, 14519, 15410, 16298, 17183, 18064, 18942, 19816, 20686,
	21553, 22415, 23272, 24125, 24974, 25817, 26656, 27489, 28317, 29140, 29956, 30767, 31572, 32371, 33163, 33949, 34729, 35501, 36267, 37026, 37777, 38521,
	39258, 39986, 40708, 41421, 42126, 42823, 43511, 44191, 44862, 45525, 46179, 46824, 47459, 48086, 48703, 49310, 49908, 50496, 51075, 51643, 52201, 52750,
	53287, 53815, 54332, 54838, 55334, 55819, 56293, 56756, 57208, 57649, 58078, 58497, 58903, 59299, 59683, 60055, 60415, 60764, 61101, 61426, 61739, 62040,
	62328, 62605, 62870, 63122, 63362, 63589, 63804, 64007, 64197, 64375, 64540, 64693, 64833, 64960, 65075, 65177, 65266, 65343, 65407, 65458, 65496, 65522,
	65534,
}

func isin(a uint16) uint16 {
	a %= 896
	i, rem := a>>3, a&7
	s := igsSines[i]
	if rem != 0 && int(i) < len(igsSines)-1 {
		s += ((igsSines[i+1] - s) * int(rem)) >> 3
	}
	return uint16(s)
}

func icos(a uint16) uint16 { return isin(900 - a) }

func umulShift(a uint16, b int) uint16 { return uint16((int64(a)*int64(b) + 32768) >> 16) }

func igsPoint(xc, yc, rx, ry int, angle uint16) (int, int) {
	angle %= 3600
	neg := 1
	switch {
	case angle > 2700:
		angle, neg = 3600-angle, 0
	case angle > 1800:
		angle, neg = angle-1800, 2
	case angle > 900:
		angle, neg = 1800-angle, 3
	}
	var dx, dy int16
	switch {
	case angle > 896:
		dx, dy = 0, int16(ry)
	case angle < 900-896:
		dx, dy = int16(rx), 0
	default:
		dx, dy = int16(umulShift(icos(angle), rx)), int16(umulShift(isin(angle), ry))
	}
	if neg&2 != 0 {
		dx = -dx
	}
	if neg&1 != 0 {
		dy = -dy
	}
	return xc + int(dx), yc + int(dy)
}

// igsCurve is VDI's gdp_curve: the points of an arc from angle a0 to a1 (tenths of degrees).
func igsCurve(xm, ym, rx, ry, a0, a1 int) []int {
	del := a1 - a0
	if del < 0 {
		del += 3600
	}
	rx, ry = abs(rx), abs(ry)
	steps := max(32, min(128, max(rx, ry)>>2))
	x, y := igsPoint(xm, ym, rx, ry, uint16(a0))
	p := []int{x, y}
	lx, ly := x, y
	for i := 1; i < steps; i++ {
		x, y = igsPoint(xm, ym, rx, ry, uint16(del*i/steps+a0))
		if x != lx || y != ly {
			p = append(p, x, y)
			lx, ly = x, y
		}
	}
	x, y = igsPoint(xm, ym, rx, ry, uint16(a1))
	return append(p, x, y)
}

// ---------------------------------------------------------------- flood fill

// floodFill fills the area of (x, y)'s colour around it with the fill pattern.
func (vm *igsVM) floodFill(x, y int) {
	if x < 0 || y < 0 || x >= vm.w || y >= vm.h {
		return
	}
	old := vm.pix[y*vm.w+x]
	if old == vm.fillColor {
		return
	}
	vm.work += len(vm.pix) / 4 // the bookkeeping costs too: a loop of fills is stopped in time
	seen := make([]bool, len(vm.pix))
	stack := [][2]int{{x, y}}
	for len(stack) > 0 {
		q := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		px, py := q[0], q[1]
		if px < 0 || py < 0 || px >= vm.w || py >= vm.h {
			continue
		}
		o := py*vm.w + px
		if seen[o] || vm.pix[o] != old {
			continue
		}
		vm.fillPixel(px, py)
		seen[o] = true
		stack = append(stack, [2]int{px - 1, py}, [2]int{px + 1, py}, [2]int{px, py - 1}, [2]int{px, py + 1})
	}
}

// ---------------------------------------------------------------- text

// igsTextFont gives the system font and its metrics for a text height (E's size): the 6x6, 8x8 or 8x16
// font, doubled for 12x12, 16x16 and 16x32.
type igsTextFont struct {
	w, h, yOff, ulPos, ulH, ulW, thicken, scale int
}

func igsFontFor(size int) igsTextFont {
	switch {
	case size <= 8:
		return igsTextFont{6, 6, 4, 5, 1, 7, 1, 1}
	case size == 9:
		return igsTextFont{8, 8, 6, 7, 1, 9, 1, 1}
	case size <= 15:
		return igsTextFont{8, 16, 13, 15, 1, 9, 1, 1}
	case size <= 17:
		return igsTextFont{6, 6, 9, 10, 2, 13, 1, 2}
	case size <= 19:
		return igsTextFont{8, 8, 13, 14, 2, 17, 2, 2}
	}
	return igsTextFont{8, 16, 27, 30, 2, 17, 2, 2}
}

func (f igsTextFont) pixel(c byte, x, y int) bool {
	switch f.w*100 + f.h {
	case 606:
		return igsFont6[c][y]&(0x80>>x) != 0
	case 808:
		if g := fontByID("st8x8"); g != nil {
			return g.bits[int(c)*16+y*2]&(0x80>>x) != 0
		}
	case 816:
		if g := fontByID("st8x16"); g != nil {
			return g.bits[int(c)*16+y]&(0x80>>x) != 0
		}
	}
	return false
}

func (vm *igsVM) rotate(x, y, w, skew, yOff int) (int, int) {
	switch vm.textRot {
	case 1:
		return y - yOff, -1 + w - (x + skew)
	case 2:
		return w - (x + skew) - 1, -y + yOff
	case 3:
		return -y + yOff, x + skew
	}
	return x + skew, y - yOff
}

func (vm *igsVM) rotateUnderline(x, y, w, skew, yOff int) (int, int) {
	switch vm.textRot {
	case 1:
		return y - yOff, w - (x + skew)
	case 2:
		return w - (x + skew) - 1, -y + yOff
	case 3:
		return -y + yOff - 1, x + skew - 1
	}
	return x + skew, y - yOff
}

// writeText is W: text at (x, y) (the baseline's left end) in the text colour, size, rotation and
// effects (thickened, ghosted, skewed, underlined, outlined).
func (vm *igsVM) writeText(x, y int, text []byte) {
	readIGSFont6()
	f := igsFontFor(vm.textSize)
	fx := vm.textEffects
	outlined := fx&16 != 0
	ot := 0
	if outlined {
		ot = 1
	}
	c := vm.textCol
	sw, sh := f.w*f.scale, f.h*f.scale
	switch vm.textRot {
	case 1:
		y -= sh - 1
	case 2:
		x -= sw - 1
	}
	ghost := uint16(0xFFFF)
	if fx&2 != 0 {
		ghost = 0x5555
	}
	mask := ghost
	for _, ch := range text {
		if vm.busy() {
			return
		}
		if outlined {
			for gy := 0; gy < f.h; gy++ {
				for gx := 0; gx < f.w; gx++ {
					mask = bits.RotateLeft16(mask, 1)
					if !f.pixel(ch, gx, gy) || mask&1 == 0 {
						continue
					}
					for sy := 0; sy < f.scale; sy++ {
						for sx := 0; sx < f.scale; sx++ {
							rx, ry := vm.rotate(gx*f.scale+sx, gy*f.scale+sy, sw, 0, f.yOff)
							for dy := -1; dy <= 1; dy++ {
								for dx := -1; dx <= 1; dx++ {
									vm.set(x+rx+dx, y+ry+dy, c)
								}
							}
						}
					}
				}
			}
			m := ghost
			for gy := 0; gy < f.h; gy++ {
				for gx := 0; gx < f.w; gx++ {
					m = bits.RotateLeft16(m, 1)
					if !f.pixel(ch, gx, gy) || m&1 == 0 {
						continue
					}
					for sy := 0; sy < f.scale; sy++ {
						for sx := 0; sx < f.scale; sx++ {
							rx, ry := vm.rotate(gx*f.scale+sx, gy*f.scale+sy, sw, 0, f.yOff)
							vm.set(x+rx, y+ry, 0)
						}
					}
				}
			}
		} else {
			for gy := 0; gy < f.h; gy++ {
				mask = bits.RotateLeft16(mask, 1)
				for gx := 0; gx < f.w; gx++ {
					if f.pixel(ch, gx, gy) && mask&1 != 0 {
						for sy := 0; sy < f.scale; sy++ {
							for sx := 0; sx < f.scale; sx++ {
								px, py := gx*f.scale+sx, gy*f.scale+sy
								skew := 0
								if fx&4 != 0 {
									skew = (sh-1-py)/2 - py%2
								}
								rx, ry := vm.rotate(px, py, sw, skew, f.yOff)
								vm.set(x+rx, y+ry, c)
								if fx&1 != 0 {
									for t := 1; t <= f.thicken; t++ {
										switch vm.textRot {
										case 0:
											vm.set(x+rx+t, y+ry, c)
										case 1:
											vm.set(x+rx, y+ry-t, c)
										case 2:
											vm.set(x+rx-t, y+ry, c)
										case 3:
											vm.set(x+rx, y+ry+t, c)
										}
									}
								}
							}
						}
					}
					mask = bits.RotateLeft16(mask, 1)
				}
			}
		}
		if fx&8 != 0 {
			um := ghost
			uw := f.ulW
			if outlined {
				uw += 2 * ot
			}
			for y2 := 0; y2 < f.ulH; y2++ {
				for ux := 0; ux < uw; ux++ {
					um = bits.RotateLeft16(um, 1)
					if um&1 == 0 {
						continue
					}
					uy := f.ulPos + y2
					skew := 0
					if fx&4 != 0 {
						skew = (sh-1-uy)/2 - uy%2
					}
					rx, ry := vm.rotateUnderline(ux, uy, sw, skew, f.yOff)
					vm.set(x+rx, y+ry, c)
				}
			}
		}
		adv := sw + 2*ot
		switch vm.textRot {
		case 0:
			x += adv
		case 1:
			y -= adv
		case 2:
			x -= adv
		case 3:
			y += adv
		}
	}
}

// ---------------------------------------------------------------- blits

// blitPx is a blit's logic operation on a source and a destination pixel (G's mode).
func blitPx(mode int, s, d uint8) uint8 {
	switch mode {
	case 0:
		return 0
	case 1:
		return s & d
	case 2:
		return s &^ d
	case 4:
		return ^s & d
	case 5:
		return d
	case 6:
		return s ^ d
	case 7:
		return s | d
	case 8:
		return ^(s | d)
	case 9:
		return ^(s ^ d)
	case 10:
		return ^d
	case 11:
		return s | ^d
	case 12:
		return ^s
	case 13:
		return ^s | d
	case 14:
		return ^(s & d)
	case 15:
		return 0xFF
	}
	return s
}

// grab is G: 0 screen to screen, 1 screen to memory, 2 memory to screen, 3 a piece of memory to the
// screen, 4 memory to memory; the second value is the blit's logic operation.
func (vm *igsVM) grab(a []int) {
	mode := a[1]
	if mode < 0 || mode > 15 {
		mode = 3
	}
	switch a[0] {
	case 0:
		if p, w, h, ok := vm.copyRect(vm.pix, vm.w, vm.h, a[2], a[3], a[4], a[5]); ok {
			vm.blitToScreen(p, w, h, 0, 0, w, h, a[6], a[7], mode)
		}
	case 1:
		if p, w, h, ok := vm.copyRect(vm.pix, vm.w, vm.h, a[2], a[3], a[4], a[5]); ok {
			vm.blit, vm.blitW, vm.blitH = p, w, h
		}
	case 2:
		vm.blitToScreen(vm.blit, vm.blitW, vm.blitH, 0, 0, vm.blitW, vm.blitH, a[2], a[3], mode)
	case 3:
		x0, y0, x1, y1 := min(a[2], a[4]), min(a[3], a[5]), max(a[2], a[4]), max(a[3], a[5])
		vm.blitToScreen(vm.blit, vm.blitW, vm.blitH, x0, y0, x1-x0+1, y1-y0+1, a[6], a[7], mode)
	case 4:
		if p, w, h, ok := vm.copyRect(vm.blit, vm.blitW, vm.blitH, a[2], a[3], a[4], a[5]); ok {
			vm.blitInto(vm.blit, vm.blitW, vm.blitH, p, w, h, 0, 0, w, h, a[6], a[7], mode)
		}
	}
}

// copyRect copies the rectangle between two corners out of a w x h surface (cut to it).
func (vm *igsVM) copyRect(src []uint8, sw, sh, x0, y0, x1, y1 int) ([]uint8, int, int, bool) {
	l, t, r, b := min(x0, x1), min(y0, y1), max(x0, x1), max(y0, y1)
	if l < 0 || t < 0 || l >= sw || t >= sh { // as IcyDraw: a corner off the screen grabs nothing
		return nil, 0, 0, false
	}
	r, b = min(r, sw-1), min(b, sh-1)
	w, h := r-l+1, b-t+1
	if w <= 0 || h <= 0 {
		return nil, 0, 0, false
	}
	p := make([]uint8, w*h)
	for y := 0; y < h; y++ {
		copy(p[y*w:(y+1)*w], src[(t+y)*sw+l:(t+y)*sw+l+w])
	}
	return p, w, h, true
}

func (vm *igsVM) blitToScreen(src []uint8, sw, sh, sx, sy, w, h, dx, dy, mode int) {
	vm.blitInto(vm.pix, vm.w, vm.h, src, sw, sh, sx, sy, w, h, dx, dy, mode)
}

// blitInto puts the w x h part at (sx, sy) of a source surface onto a destination at (dx, dy).
func (vm *igsVM) blitInto(dst []uint8, dw, dh int, src []uint8, sw, sh, sx, sy, w, h, dx, dy, mode int) {
	if dx < 0 {
		sx, w, dx = sx-dx, w+dx, 0
	}
	if dy < 0 {
		sy, h, dy = sy-dy, h+dy, 0
	}
	if sx < 0 || sy < 0 || sx >= sw || sy >= sh || dx >= dw || dy >= dh {
		return
	}
	w, h = min(w, sw-sx, dw-dx), min(h, sh-sy, dh-dy)
	m := uint8(vm.colours() - 1)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			o := (dy+y)*dw + dx + x
			dst[o] = blitPx(mode, src[(sy+y)*sw+sx+x], dst[o]) & m
		}
		vm.work += w
	}
}

// wipeBlit is X 11,0: the blit memory (the whole screen's 32000 bytes, or one of 8 bands) filled with a
// byte (256: a random byte each), as the ST's planar memory holds it.
func (vm *igsVM) wipeBlit(section, value int) {
	vm.fullBlit()
	var mem []byte
	for i := 0; i < 32000; i++ {
		v := byte(max(0, min(255, value)))
		if value == 256 {
			v = byte(vm.rng.Intn(256))
		}
		mem = append(mem, v)
	}
	vm.planar(mem, section)
}

// loadBlit is X 11,1 and 2: the blit memory (or a band of it) from the bytes after the command.
func (vm *igsVM) loadBlit(section int, data []byte) {
	vm.fullBlit()
	mem := make([]byte, 32000)
	if section >= 1 && section <= 8 {
		copy(mem[(section-1)*4000:], data)
	} else {
		copy(mem, data)
	}
	vm.planar(mem, section)
}

func (vm *igsVM) fullBlit() {
	if vm.blitW == vm.w && vm.blitH == vm.h {
		return
	}
	p := make([]uint8, vm.w*vm.h)
	for y := 0; y < min(vm.blitH, vm.h); y++ {
		copy(p[y*vm.w:y*vm.w+min(vm.blitW, vm.w)], vm.blit[y*vm.blitW:y*vm.blitW+min(vm.blitW, vm.w)])
	}
	vm.blit, vm.blitW, vm.blitH = p, vm.w, vm.h
}

// planar decodes ST screen memory (16 pixels a group, a word per plane) into the blit memory's rows of
// a band (1-8) or all of them (0).
func (vm *igsVM) planar(mem []byte, section int) {
	planes := [3]int{4, 2, 1}[vm.res]
	y0, y1 := 0, vm.h
	if section >= 1 && section <= 8 {
		y0, y1 = (section-1)*vm.h/8, section*vm.h/8
	}
	bpr := vm.w / 8 * planes // bytes per row
	for y := y0; y < y1; y++ {
		for g := 0; g < vm.w/16; g++ {
			o := y*bpr + g*2*planes
			for x := 0; x < 16; x++ {
				var c uint8
				for p := 0; p < planes; p++ {
					wd := uint16(mem[o+2*p])<<8 | uint16(mem[o+2*p+1])
					c |= uint8(wd>>(15-x)&1) << p
				}
				vm.blit[y*vm.w+g*16+x] = c
			}
		}
	}
}

// ---------------------------------------------------------------- VDI's patterns

var igsTypePatterns = [24][8]uint16{
	{0x0000, 0x4444, 0x0000, 0x1111, 0x0000, 0x4444, 0x0000, 0x1111},
	{0x0000, 0x5555, 0x0000, 0x5555, 0x0000, 0x5555, 0x0000, 0x5555},
	{0x8888, 0x5555, 0x2222, 0x5555, 0x8888, 0x5555, 0x2222, 0x5555},
	{0xAAAA, 0x5555, 0xAAAA, 0x5555, 0xAAAA, 0x5555, 0xAAAA, 0x5555},
	{0xAAAA, 0xDDDD, 0xAAAA, 0x7777, 0xAAAA, 0xDDDD, 0xAAAA, 0x7777},
	{0xAAAA, 0xFFFF, 0xAAAA, 0xFFFF, 0xAAAA, 0xFFFF, 0xAAAA, 0xFFFF},
	{0xEEEE, 0xFFFF, 0xBBBB, 0xFFFF, 0xEEEE, 0xFFFF, 0xBBBB, 0xFFFF},
	{0xFFFF, 0xFFFF, 0xFFFF, 0xFFFF, 0xFFFF, 0xFFFF, 0xFFFF, 0xFFFF},
	{0xFFFF, 0x8080, 0x8080, 0x8080, 0xFFFF, 0x0808, 0x0808, 0x0808}, // brick
	{0x2020, 0x4040, 0x8080, 0x4141, 0x2222, 0x1414, 0x0808, 0x1010}, // diagonal bricks
	{0x0000, 0x0000, 0x1010, 0x2828, 0x0000, 0x0000, 0x0101, 0x8282}, // grass
	{0x0202, 0x0202, 0xAAAA, 0x5050, 0x2020, 0x2020, 0xAAAA, 0x0505}, // trees
	{0x4040, 0x8080, 0x0000, 0x0808, 0x0404, 0x0202, 0x0000, 0x2020}, // dashed x's
	{0x6606, 0xC6C6, 0xD8D8, 0x1818, 0x8181, 0x8DB1, 0x0C33, 0x6000}, // cobble stones
	{0x0000, 0x0000, 0x0400, 0x0000, 0x0010, 0x0000, 0x8000, 0x0000}, // sand
	{0xF8F8, 0x6C6C, 0xC6C6, 0x8F8F, 0x1F1F, 0x3636, 0x6363, 0xF1F1}, // rough weave
	{0xAAAA, 0x0000, 0x8888, 0x1414, 0x2222, 0x4141, 0x8888, 0x0000}, // quilt
	{0x0808, 0x0000, 0xAAAA, 0x0000, 0x0808, 0x0000, 0x8888, 0x0000}, // patterned cross
	{0x7777, 0x9898, 0xF8F8, 0xF8F8, 0x7777, 0x8989, 0x8F8F, 0x8F8F}, // balls
	{0x8080, 0x8080, 0x4141, 0x3E3E, 0x0808, 0x0808, 0x1414, 0xE3E3}, // vertical scales
	{0x8181, 0x4242, 0x2424, 0x1818, 0x0606, 0x0101, 0x8080, 0x8080}, // diagonal scales
	{0xF0F0, 0xF0F0, 0xF0F0, 0xF0F0, 0x0F0F, 0x0F0F, 0x0F0F, 0x0F0F}, // checker board
	{0x0808, 0x1C1C, 0x3E3E, 0x7F7F, 0xFFFF, 0x7F7F, 0x3E3E, 0x1C1C}, // filled diamond
	{0x1111, 0x2222, 0x4444, 0xFFFF, 0x8888, 0x4444, 0x2222, 0xFFFF}, // herringbone
}

var igsHatches = [6][8]uint16{
	{0x0101, 0x0202, 0x0404, 0x0808, 0x1010, 0x2020, 0x4040, 0x8080},
	{0x6060, 0xC0C0, 0x8181, 0x0303, 0x0606, 0x0C0C, 0x1818, 0x3030},
	{0x4242, 0x8181, 0x8181, 0x4242, 0x2424, 0x1818, 0x1818, 0x2424},
	{0x8080, 0x8080, 0x8080, 0x8080, 0x8080, 0x8080, 0x8080, 0x8080},
	{0xFFFF, 0x0000, 0x0000, 0x0000, 0x0000, 0x0000, 0x0000, 0x0000},
	{0xFFFF, 0x8080, 0x8080, 0x8080, 0x8080, 0x8080, 0x8080, 0x8080},
}

var igsWideHatches = [6][16]uint16{
	{0x0001, 0x0002, 0x0004, 0x0008, 0x0010, 0x0020, 0x0040, 0x0080, 0x0100, 0x0200, 0x0400, 0x0800, 0x1000, 0x2000, 0x4000, 0x8000},
	{0x8003, 0x0007, 0x000E, 0x001C, 0x0038, 0x0070, 0x00E0, 0x01C0, 0x0380, 0x0700, 0x0E00, 0x1C00, 0x3800, 0x7000, 0xE000, 0xC001},
	{0x8001, 0x4002, 0x2004, 0x1008, 0x0810, 0x0420, 0x0240, 0x0180, 0x0180, 0x0240, 0x0420, 0x0810, 0x1008, 0x2004, 0x4002, 0x8001},
	{0x8000, 0x8000, 0x8000, 0x8000, 0x8000, 0x8000, 0x8000, 0x8000, 0x8000, 0x8000, 0x8000, 0x8000, 0x8000, 0x8000, 0x8000, 0x8000},
	{0xFFFF, 0x0000, 0x0000, 0x0000, 0x0000, 0x0000, 0x0000, 0x0000, 0x0000, 0x0000, 0x0000, 0x0000, 0x0000, 0x0000, 0x0000, 0x0000},
	{0xFFFF, 0x8080, 0x8080, 0x8080, 0x8080, 0x8080, 0x8080, 0x8080, 0xFFFF, 0x8080, 0x8080, 0x8080, 0x8080, 0x8080, 0x8080, 0x8080},
}

// A fixed "random" pattern (VDI makes a new one each time; a fixed one draws the same every time).
var igsRandomPattern = [16]uint16{0x1c8c, 0x6987, 0x4b96, 0xbfbc, 0xaa0e, 0x1a66, 0x052b, 0xc73d, 0xf810, 0xad4e,
	0xf44a, 0x49d3, 0x66c9, 0x0677, 0xadf1, 0x718a}

var igsStarTrek = [16]uint16{0x07E0, 0x0FF0, 0x1FD8, 0x1808, 0x1808, 0x1008, 0x1E78, 0x1348, 0x1108, 0x0810,
	0x0B70, 0x0650, 0x07A0, 0x1E20, 0x1BC0, 0x1800}
