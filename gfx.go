package main

// A plain 32-bit pixel buffer (0xAARRGGBB) and the drawing on it.

type FB struct {
	w, h int
	px   []uint32
	clip Rect
}

type Rect struct{ x, y, w, h int }

func (r Rect) has(x, y int) bool { return x >= r.x && y >= r.y && x < r.x+r.w && y < r.y+r.h }
func (r Rect) inset(n int) Rect  { return Rect{r.x + n, r.y + n, r.w - 2*n, r.h - 2*n} }
func (r Rect) and(o Rect) Rect {
	x0, y0 := max(r.x, o.x), max(r.y, o.y)
	x1, y1 := min(r.x+r.w, o.x+o.w), min(r.y+r.h, o.y+o.h)
	if x1 < x0 || y1 < y0 {
		return Rect{x0, y0, 0, 0}
	}
	return Rect{x0, y0, x1 - x0, y1 - y0}
}

func (f *FB) resize(w, h int) {
	f.w, f.h = w, h
	if len(f.px) < w*h {
		f.px = make([]uint32, w*h)
	}
	f.px = f.px[:w*h]
	f.clip = Rect{0, 0, w, h}
}

func (f *FB) fill(r Rect, c uint32) {
	r = r.and(f.clip)
	for y := r.y; y < r.y+r.h; y++ {
		row := f.px[y*f.w+r.x : y*f.w+r.x+r.w]
		for i := range row {
			row[i] = c
		}
	}
}

// blend mixes c over the rectangle; a is 0-255.
func (f *FB) blend(r Rect, c uint32, a int) {
	r = r.and(f.clip)
	cr, cg, cb := int(c>>16&255)*a, int(c>>8&255)*a, int(c&255)*a
	na := 255 - a
	for y := r.y; y < r.y+r.h; y++ {
		row := f.px[y*f.w+r.x : y*f.w+r.x+r.w]
		for i, p := range row {
			pr := (int(p>>16&255)*na + cr) / 255
			pg := (int(p>>8&255)*na + cg) / 255
			pb := (int(p&255)*na + cb) / 255
			row[i] = 0xFF000000 | uint32(pr)<<16 | uint32(pg)<<8 | uint32(pb)
		}
	}
}

func (f *FB) frame(r Rect, c uint32) {
	f.fill(Rect{r.x, r.y, r.w, 1}, c)
	f.fill(Rect{r.x, r.y + r.h - 1, r.w, 1}, c)
	f.fill(Rect{r.x, r.y, 1, r.h}, c)
	f.fill(Rect{r.x + r.w - 1, r.y, 1, r.h}, c)
}

func (f *FB) dashed(r Rect, c uint32) {
	for x := r.x; x < r.x+r.w; x++ {
		if (x-r.x)%5 < 3 {
			f.fill(Rect{x, r.y, 1, 1}, c)
			f.fill(Rect{x, r.y + r.h - 1, 1, 1}, c)
		}
	}
	for y := r.y; y < r.y+r.h; y++ {
		if (y-r.y)%5 < 3 {
			f.fill(Rect{r.x, y, 1, 1}, c)
			f.fill(Rect{r.x + r.w - 1, y, 1, 1}, c)
		}
	}
}

// bevel draws the Workbench 3 edge: light top and left, dark bottom and right (or the other way).
func (f *FB) bevel(r Rect, in bool) {
	lt, dk := uint32(cEdgeLt), uint32(cEdgeDk)
	if in {
		lt, dk = dk, lt
	}
	f.fill(Rect{r.x, r.y, r.w, 1}, lt)
	f.fill(Rect{r.x, r.y, 1, r.h}, lt)
	f.fill(Rect{r.x + 1, r.y + r.h - 1, r.w - 1, 1}, dk)
	f.fill(Rect{r.x + r.w - 1, r.y + 1, 1, r.h - 1}, dk)
}

// glyph draws one character of a font, scaled by sx and sy, with an optional background.
func (f *FB) glyph(fn *Font, x, y int, ch byte, fg, bg uint32, sx, sy int, opaque bool) {
	for r := 0; r < 16; r++ {
		bits := fn.bits[int(ch)*16+r]
		for b := 0; b < 8; b++ {
			on := bits&(0x80>>b) != 0
			if !on && !opaque {
				continue
			}
			c := bg
			if on {
				c = fg
			}
			f.fill(Rect{x + b*sx, y + r*sy, sx, sy}, c)
		}
	}
}
