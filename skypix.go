package main

// SkyPix: the Amiga BBS graphics of the early 1990s (SkyPix by Michael Cox, shown by the
// SkyPix terminal and JR-Comm, sent by boards such as SkyLine). A SkyPix file is an ANSI
// stream for a 640x200 screen of 16 colours with drawing commands of its own:
// ESC [ n ; ... ! (n the command). CharEdit opens one on a PC page of 80x25 cells in the
// 8x8 VGA font: the drawing (and the text the file draws) is a picture under the cells,
// as on IGS pages (igs_page.go), and what is typed goes over it. Saving writes the file
// as it came, then, after a comment command that says so, the typed cells.
//
// The commands (ANSI.TXT, the SkyPix spec in icy_tools/crates/icy_parser_core/doc/skypix)
// and what they do are IcyDraw 0.6's (icy_parser_core/src/skypix, icy_engine/src/
// amiga_screen_buffer: sky_paint.rs, skypix_impl.rs): CharEdit draws the files as IcyDraw
// does (checked pixel for pixel with its engine, but for the text font: IcyDraw has an
// Amiga 8x8 IBM font, CharEdit the VGA's 8x8; and the Amiga fonts a file asks for by name,
// which CharEdit draws in that font too).
//
//	 0 comment (text to the next "!")   1 x;y pixel in pen A   2 x;y line from the pen
//	 3 m;x;y flood fill (0 up to pen A's colour, 1 the colour at x,y)
//	 4 x1;y1;x2;y2 filled rectangle     5 x;y;a;b ellipse      13 filled ellipse
//	 6 x;y;w;h grab a brush             7 sx;sy;dx;dy;w;h;minterm;mask put it
//	 8 x;y move the pen                 9 sample (not played)  10 size;name! font (0: back)
//	11 16 colours $0BGR                12 the SkyPix colours  14 n pause (jiffies; not kept)
//	15 n pen A (text colour)           16 transfer (skipped)  17 m 8/16 colours
//	18 n pen B (text background)       19 x;y text cursor in pixels
//	21/22 controller, gadgets (skipped) 99 end of SkyPix
//
// After any command text is drawn without its background (Amiga JAM1) and RETURN and
// LINE FEED do nothing, until SGR 0 or the font command with size 0 (as IcyDraw).

import (
	"bytes"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

const skyW, skyH = 640, 200

// skyPalette is SkyPix's 16 colours (Amiga 12-bit values, IcyDraw's SKYPIX_PALETTE).
var skyPalette = skyColours([16][3]int{{0, 0, 0}, {1, 1, 15}, {13, 13, 13}, {15, 0, 0}, {0, 15, 1}, {3, 10, 15}, {15, 15, 2}, {12, 0, 14},
	{0, 11, 6}, {0, 13, 13}, {0, 10, 15}, {0, 7, 12}, {0, 0, 15}, {7, 0, 15}, {12, 0, 14}, {12, 0, 8}})

func skyColours(c [16][3]int) [16]uint32 {
	var out [16]uint32
	for i, v := range c {
		out[i] = 0xFF000000 | uint32(v[0]*255/15)<<16 | uint32(v[1]*255/15)<<8 | uint32(v[2]*255/15)
	}
	return out
}

// skyAnsi is the palette entry of ANSI colour n (SGR 30-37, 40-47).
var skyAnsi = [8]int{0, 3, 4, 6, 1, 7, 5, 2}

// skyMarker starts the typed cells in a file CharEdit saved: a comment, which SkyPix terminals skip.
const skyMarker = "\x1b[0!CharEdit cells!"

// skyLayer is a SkyPix file read: its picture and what saving keeps.
type skyLayer struct {
	src     []byte
	drawing []byte // the file up to the typed cells (all of it for another program's file)
	pix     []uint8
	pal     [16]uint32
	cells   *Doc // the typed cells as read
	note    string
}

// skyPage: is the page a SkyPix file's (a picture under its cells)?
func (a *App) skyPage() bool { return a.pc() && a.sky != nil }

// skyCmd is a command read: its number, values and text.
type skyCmd struct {
	n    int
	vals []int
	text string
}

// skyRead cuts a stream into its parts the way IcyDraw's SkypixParser does: text bytes,
// controls, ANSI sequences (letter) and SkyPix commands (ending in "!").
func skyRead(b []byte, text func(c byte), ctrl func(c byte), ansi func(final byte, vals []int), sky func(c skyCmd)) {
	for i := 0; i < len(b); i++ {
		c := b[i]
		switch c {
		case 0x1B:
			if i+1 >= len(b) || b[i+1] != '[' {
				text(0x1B)
				continue
			}
			j, vals, cur, has, neg := i+2, []int(nil), 0, false, false
			push := func() {
				if has {
					if neg {
						cur = -cur
					}
					vals = append(vals, cur)
				}
				cur, has, neg = 0, false, false
			}
			done := false
			for ; j < len(b) && !done; j++ {
				d := b[j]
				switch {
				case d >= '0' && d <= '9':
					if cur < 1<<24 {
						cur = cur*10 + int(d-'0')
					}
					has = true
				case d == '-':
					neg, has = true, true
				case d == ';':
					push()
				case d == '!':
					push()
					cmd := skyCmd{n: 0}
					if len(vals) > 0 {
						cmd.n, vals = vals[0], vals[1:]
					}
					cmd.vals = vals
					// Comments, fonts and transfers have text up to the next "!" (a font of size 0 has
					// none, nor has "ESC[!").
					hasText := j > i+2 && (cmd.n == 0 || cmd.n == 16 || cmd.n == 10 && (len(vals) == 0 || vals[0] != 0))
					if hasText {
						k := bytes.IndexByte(b[j+1:], '!')
						if k < 0 {
							return // cut off: the rest is the command's text
						}
						cmd.text = string(b[j+1 : j+1+k])
						j += 1 + k
					}
					sky(cmd)
					done = true
				case d >= 'A' && d <= 'Z' || d >= 'a' && d <= 'z':
					push()
					ansi(d, vals)
					done = true
				default:
					done = true // not a sequence: dropped with what came so far (IcyDraw warns)
				}
			}
			i = j - 1
		case 7, 8, 9, 10, 11, 12, 13, 0x7F:
			ctrl(c)
		default:
			text(c)
		}
	}
}

// skyFace is a font SkyPix text is drawn in: w x h cells (w at most 8), glyph rows left-aligned.
type skyFace struct {
	w, h int
	rows [256][32]byte
}

// skyFaceOf is a CharEdit font as a SkyPix face of 8 x 8.
func skyFaceOf(f *Font) *skyFace {
	s := &skyFace{w: 8, h: 8}
	for c := 0; c < 256; c++ {
		for r := 0; r < 8; r++ {
			s.rows[c][r] = f.row(c, r)
		}
	}
	return s
}

// skyVM draws a SkyPix stream.
type skyVM struct {
	pix        []uint8
	pal        [16]uint32
	def, cur   *skyFace // the 8x8 font, and the one text is drawn in (an Amiga font by name, or def)
	fonts      func(name string, size int) *skyFace
	x, y       int // the text cursor in pixels
	fg, bg     int
	inverse    bool
	jam1       bool
	penX, penY int
	brush      []uint8
	bw, bh     int
	work       int
	skipped    map[string]bool
}

func newSkyVM(def *skyFace, fonts func(string, int) *skyFace) *skyVM {
	if fonts == nil {
		fonts = func(string, int) *skyFace { return nil }
	}
	return &skyVM{pix: make([]uint8, skyW*skyH), pal: skyPalette, def: def, cur: def, fonts: fonts, fg: 2, skipped: map[string]bool{}}
}

// skyWorkMax stops a hostile file: pixels set in all (real files set well under a million).
const skyWorkMax = 50_000_000

func (v *skyVM) put(x, y int, c int) {
	v.work++
	if x >= 0 && y >= 0 && x < skyW && y < skyH {
		v.pix[y*skyW+x] = uint8(c)
	}
}

func (v *skyVM) at(x, y int) int { return int(v.pix[y*skyW+x]) }

func (v *skyVM) busy() bool { return v.work > skyWorkMax }

func (v *skyVM) line(x0, y0, x1, y1, c int) {
	dx, dy := abs(x1-x0), abs(y1-y0)
	sx, sy := 1, 1
	if x0 >= x1 {
		sx = -1
	}
	if y0 >= y1 {
		sy = -1
	}
	err := dx - dy
	for !v.busy() {
		v.put(x0, y0, c)
		if x0 == x1 && y0 == y1 {
			return
		}
		e2 := 2 * err
		if e2 > -dy {
			err -= dy
			x0 += sx
		}
		if e2 < dx {
			err += dx
			y0 += sy
		}
	}
}

// fill floods from x,y: mode 0 up to colour c (the outline), mode 1 the colour at x,y.
func (v *skyVM) fill(mode, x, y, c int) {
	if x < 0 || y < 0 || x >= skyW || y >= skyH {
		return
	}
	start := v.at(x, y)
	if mode == 0 && start == c || mode == 1 && start == c {
		return
	}
	seen := make([]bool, skyW*skyH)
	stack := []int{y*skyW + x}
	for len(stack) > 0 && !v.busy() {
		p := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if seen[p] {
			continue
		}
		seen[p] = true
		px, py := p%skyW, p/skyW
		cur := int(v.pix[p])
		if mode == 0 && cur == c || mode == 1 && cur != start {
			continue
		}
		v.put(px, py, c)
		if px+1 < skyW {
			stack = append(stack, p+1)
		}
		if px > 0 {
			stack = append(stack, p-1)
		}
		if py+1 < skyH {
			stack = append(stack, p+skyW)
		}
		if py > 0 {
			stack = append(stack, p-skyW)
		}
	}
}

func (v *skyVM) bar(x1, y1, x2, y2, c int) {
	// IcyDraw: a rectangle from (x1, y1), x2-x1+1 wide, cut to the screen.
	l, t, r, b := max(x1, 0), max(y1, 0), min(x2, skyW-1), min(y2, skyH-1)
	for y := t; y <= b && !v.busy(); y++ {
		for x := l; x <= r; x++ {
			v.put(x, y, c)
		}
	}
}

func (v *skyVM) ellipse(cx, cy, rx, ry, c int) {
	if rx <= 0 || ry <= 0 || v.busy() {
		return
	}
	x, y := 0, ry
	rx2, ry2 := int64(rx)*int64(rx), int64(ry)*int64(ry)
	px, py := int64(0), 2*rx2*int64(y)
	four := func() {
		v.put(cx+x, cy+y, c)
		v.put(cx-x, cy+y, c)
		v.put(cx+x, cy-y, c)
		v.put(cx-x, cy-y, c)
	}
	p := ry2 - rx2*int64(ry) + rx2/4
	for px < py && !v.busy() {
		four()
		x++
		px += 2 * ry2
		if p < 0 {
			p += ry2 + px
		} else {
			y--
			py -= 2 * rx2
			p += ry2 + px - py
		}
	}
	p = ry2*int64(x*2+1)*int64(x*2+1)/4 + rx2*int64(y-1)*int64(y-1) - rx2*ry2
	for y >= 0 && !v.busy() {
		four()
		y--
		py -= 2 * rx2
		if p > 0 {
			p += rx2 - py
		} else {
			x++
			px += 2 * ry2
			p += rx2 - py + px
		}
	}
}

func (v *skyVM) fillEllipse(cx, cy, rx, ry, c int) {
	if rx <= 0 || ry <= 0 || v.busy() {
		return
	}
	for y := -ry; y <= ry && !v.busy(); y++ {
		r := float64(y) / float64(ry)
		ext := int(math.Round(math.Sqrt(1-r*r) * float64(rx)))
		for x := -ext; x <= ext; x++ {
			v.put(cx+x, cy+y, c)
		}
	}
	v.ellipse(cx, cy, rx, ry, c)
}

// glyph draws a character at the cursor: in JAM1 without its background.
func (v *skyVM) glyph(ch byte) {
	fg, bg, f := v.fg, v.bg, v.cur
	if v.inverse {
		fg, bg = bg, fg
	}
	for r := 0; r < f.h && r < 32; r++ {
		bits := f.rows[ch][r]
		for b := 0; b < f.w; b++ {
			on := bits&(0x80>>b) != 0
			if !on && v.jam1 {
				continue
			}
			c := bg
			if on {
				c = fg
			}
			v.put(v.x+b, v.y+r, c)
		}
	}
	v.x += f.w
	if v.x >= skyW {
		v.x, v.y = 0, v.y+f.h
	}
}

// scrollUp moves the picture up a line of the current font.
func (v *skyVM) scrollUp() {
	h := v.cur.h
	if h <= 0 || h >= skyH {
		return
	}
	copy(v.pix, v.pix[h*skyW:])
	for i := (skyH - h) * skyW; i < len(v.pix); i++ {
		v.pix[i] = 0
	}
}

func (v *skyVM) text(c byte) {
	for v.y+v.cur.h > skyH && v.cur.h < skyH { // scrolled before drawing (IcyDraw's print_char)
		v.scrollUp()
		v.y -= v.cur.h
	}
	v.glyph(c)
}

func (v *skyVM) clamp() {
	v.x, v.y = min(max(v.x, 0), skyW-1), min(max(v.y, 0), skyH-1)
}

func (v *skyVM) ctrl(c byte) {
	w, h := v.cur.w, v.cur.h
	switch c {
	case 8: // backspace
		v.x = max(0, v.x-w)
	case 9: // to the next tab stop of 8 columns
		v.x, v.y = min((v.x/w/8+1)*8, skyW/w-1)*w, v.y/h*h
	case 10, 13:
		if v.jam1 {
			return
		}
		v.x = 0
		if c == 10 {
			v.y += h
			for v.y >= skyH && h < skyH {
				v.scrollUp()
				v.y -= h
			}
		}
	case 11: // cursor up
		v.y -= h
	case 12: // form feed: the screen cleared
		v.clear(0, len(v.pix))
		v.x, v.y = 0, 0
	}
}

func (v *skyVM) clear(from, to int) {
	for i := max(from, 0); i < min(to, len(v.pix)); i++ {
		v.pix[i] = uint8(v.bg)
	}
}

func (v *skyVM) ansi(final byte, vals []int) {
	n := func(i, def int) int {
		if i < len(vals) && vals[i] > 0 {
			return vals[i]
		}
		return def
	}
	w, h := v.cur.w, v.cur.h
	switch final {
	case 'A':
		v.y -= h * n(0, 1)
	case 'B':
		v.y += h * n(0, 1)
	case 'C':
		v.x = min(v.x+w*n(0, 1), skyW-1)
	case 'D':
		v.x = max(v.x-w*n(0, 1), 0)
	case 'H', 'f':
		v.x, v.y = (n(1, 1)-1)*w, (n(0, 1)-1)*h
	case 'E':
		v.x, v.y = 0, v.y+h*n(0, 1)
	case 'F':
		v.x, v.y = 0, v.y-h*n(0, 1)
	case 'G':
		v.x = (n(0, 1) - 1) * w
	case 'J':
		k := 0
		if len(vals) > 0 {
			k = vals[0]
		}
		switch k {
		case 0:
			v.clear(v.y*skyW, len(v.pix))
		case 1:
			v.clear(0, min(v.y+h, skyH)*skyW)
		case 2:
			v.clear(0, len(v.pix))
			v.x, v.y = 0, 0
		}
	case 'K':
		k := 0
		if len(vals) > 0 {
			k = vals[0]
		}
		for y := v.y; y < min(v.y+h, skyH); y++ {
			switch k {
			case 0:
				v.clear(y*skyW+v.x, (y+1)*skyW)
			case 1:
				v.clear(y*skyW, y*skyW+v.x)
			case 2:
				v.clear(y*skyW, (y+1)*skyW)
			}
		}
	case 'm':
		if len(vals) == 0 {
			vals = []int{0}
		}
		for _, p := range vals {
			switch {
			case p == 0:
				v.fg, v.bg, v.inverse, v.jam1, v.cur = 2, 0, false, false, v.def
			case p == 7:
				v.inverse = true
			case p >= 30 && p <= 37:
				v.fg = skyAnsi[p-30]
			case p >= 40 && p <= 47:
				v.bg = skyAnsi[p-40]
			}
		}
	}
	v.clamp()
}

func (v *skyVM) sky(c skyCmd) {
	val := func(i int) int {
		if i < len(c.vals) {
			return c.vals[i]
		}
		return 0
	}
	need := map[int]int{1: 2, 2: 2, 3: 3, 4: 4, 5: 4, 6: 4, 7: 8, 8: 2, 9: 4, 10: 1, 11: 16, 13: 4, 14: 1, 16: 3, 17: 1, 19: 2, 21: 3, 22: 6}
	if len(c.vals) < need[c.n] {
		return // IcyDraw drops a command short of values
	}
	pen := v.fg & 255
	v.jam1 = true
	switch c.n {
	case 1:
		v.put(val(0), val(1), pen)
	case 2:
		v.line(v.penX, v.penY, val(0), val(1), pen)
		v.penX, v.penY = val(0), val(1)
	case 3:
		if m := val(0); m == 0 || m == 1 {
			v.fill(m, val(1), val(2), pen)
		}
	case 4:
		v.bar(val(0), val(1), val(2), val(3), pen)
	case 5:
		v.ellipse(val(0), val(1), val(2), val(3), pen)
	case 13:
		v.fillEllipse(val(0), val(1), val(2), val(3), pen)
	case 6:
		x0, y0, w, h := val(0), val(1), val(2), val(3)
		if w > 0 && h > 0 && w*h <= skyW*skyH {
			v.brush, v.bw, v.bh = make([]uint8, w*h), w, h
			for y := 0; y < h; y++ {
				for x := 0; x < w; x++ {
					if xx, yy := x0+x, y0+y; xx >= 0 && yy >= 0 && xx < skyW && yy < skyH {
						v.brush[y*w+x] = v.pix[yy*skyW+xx]
					}
				}
			}
		}
	case 7:
		if v.brush != nil {
			sx, sy, dx, dy, w, h := val(0), val(1), val(2), val(3), val(4), val(5)
			for y := 0; y < h && sy+y < v.bh && !v.busy(); y++ {
				for x := 0; x < w && sx+x < v.bw; x++ {
					if sx+x >= 0 && sy+y >= 0 {
						v.put(dx+x, dy+y, int(v.brush[(sy+y)*v.bw+sx+x]))
					}
				}
			}
		}
	case 8:
		v.penX, v.penY = val(0), val(1)
	case 10:
		if val(0) == 0 { // back to the 8x8 font: text with its background again (the cursor stays:
			// IcyDraw means to move it down by the fonts' difference but measures both in the font in use)
			v.cur, v.jam1 = v.def, false
		} else if f := v.fonts(c.text, val(0)); f != nil {
			v.cur = f
		} else {
			v.cur = v.def
			v.skipped[fmt.Sprintf("the font %s %d", strings.TrimSpace(c.text), val(0))] = true
		}
	case 11:
		for i := 0; i < 16; i++ {
			x := val(i)
			v.pal[i] = 0xFF000000 | uint32((x&15)*17)<<16 | uint32((x>>4&15)*17)<<8 | uint32((x>>8&15)*17)
		}
	case 12:
		v.pal = skyPalette
	case 15:
		v.fg = val(0)
		if len(c.vals) == 0 {
			v.fg = 2
		}
	case 18:
		v.bg = val(0)
	case 17:
		v.pal = skyPalette
		if val(0) == 1 {
			for i := 8; i < 16; i++ {
				v.pal[i] = skyPalette[i-8]
			}
		}
	case 19:
		v.x, v.y, v.penX, v.penY = val(0), val(1), val(0), val(1)
	case 99:
		v.pal, v.fg, v.bg, v.inverse, v.jam1, v.cur = skyPalette, 2, 0, false, false, v.def
	case 9:
		v.skipped["sounds"] = true
	case 16:
		v.skipped["brushes and files sent by transfer"] = true
	case 14:
		v.skipped["pauses"] = true
	case 21, 22:
		v.skipped["gadgets"] = true
	}
}

// run draws a stream.
func (v *skyVM) run(b []byte) {
	skyRead(b, func(c byte) {
		if !v.busy() {
			v.text(c)
		}
	}, v.ctrl, v.ansi, v.sky)
}

// isSkyPix: does b hold SkyPix commands (two at least, of the numbers SkyPix has)?
func isSkyPix(b []byte) bool {
	n := 0
	for i := 0; i+3 < len(b) && n < 2; i++ {
		if b[i] != 0x1B || b[i+1] != '[' {
			continue
		}
		j, num := i+2, 0
		for j < len(b) && j < i+40 && (b[j] >= '0' && b[j] <= '9' || b[j] == ';' || b[j] == '-') {
			j++
		}
		if j < len(b) && b[j] == '!' && j > i+2 {
			first := string(b[i+2 : j])
			if k := strings.IndexByte(first, ';'); k >= 0 {
				first = first[:k]
			}
			num, _ = strconv.Atoi(first)
			if num >= 0 && num <= 22 || num == 99 {
				n++
			}
		}
	}
	return n >= 2
}

// readSkyPix reads a SkyPix file: the picture of its drawing part, and the cells CharEdit saved after it.
func readSkyPix(b []byte) *skyLayer {
	l := &skyLayer{src: b, drawing: b, cells: newDoc(80, 25)}
	l.cells.ensureRows(25)
	fillCells(l.cells.cells, makeCell(32, 2, 0, 0))
	if k := bytes.Index(b, []byte(skyMarker)); k >= 0 {
		l.drawing = b[:k]
		skyCells(b[k+len(skyMarker):], l.cells)
	}
	vm := newSkyVM(skyDefaultFace(), skyFontByName)
	vm.run(l.drawing)
	l.pix, l.pal = vm.pix, vm.pal
	var left []string
	for k := range vm.skipped {
		left = append(left, k)
	}
	if vm.busy() {
		left = append(left, "the rest: the drawing takes too long")
	}
	if len(left) > 0 {
		sort.Strings(left)
		l.note = "not shown: " + strings.Join(left, ", ")
	}
	return l
}

// skyCells reads the typed cells CharEdit wrote: pens, positions and text.
func skyCells(b []byte, d *Doc) {
	x, y, fg, bg := 0, 0, 2, 0
	skyRead(b, func(c byte) {
		if x < d.cols && y < d.rows {
			d.set(x, y, makeCell(int(c), fg&15, bg&15, 0))
		}
		x++
	}, func(byte) {}, func(final byte, vals []int) {
		if final == 'H' && len(vals) == 2 {
			y, x = vals[0]-1, vals[1]-1
		}
	}, func(c skyCmd) {
		switch {
		case c.n == 15 && len(c.vals) == 1:
			fg = c.vals[0]
		case c.n == 18 && len(c.vals) == 1:
			bg = c.vals[0]
		}
	})
}

// skyBad are the bytes a typed cell cannot be written as: SkyPix takes them as controls.
func skyBad(c int) bool { return c == 0x1B || c >= 7 && c <= 13 || c == 0x7F }

// writeSkyPix gives the file: the drawing as it came (nil: none), then the typed cells
// (pens, the 8x8 font back, the position, the text). missed counts characters that are
// SkyPix's controls (written as spaces).
func writeSkyPix(l *skyLayer, d *Doc) (out []byte, missed int) {
	if l != nil && skySameCells(l.cells, d) {
		return l.src, 0
	}
	if l != nil {
		out = append(out, l.drawing...)
	}
	out = append(out, skyMarker...)
	fg, bg := -1, -1
	for y := 0; y < min(d.rows, 25); y++ {
		run := false
		for x := 0; x < min(d.cols, 80); x++ {
			v := d.get(x, y)
			if isBlank(v) {
				run = false
				continue
			}
			if cellFg(v) != fg || cellBg(v) != bg {
				fg, bg = cellFg(v), cellBg(v)
				out = append(out, fmt.Sprintf("\x1b[15;%d!\x1b[18;%d!\x1b[10;0!", fg, bg)...)
				run = false
			}
			if !run {
				out = append(out, fmt.Sprintf("\x1b[%d;%dH", y+1, x+1)...)
				run = true
			}
			c := cellCh(v)
			if skyBad(c) {
				c, missed = 32, missed+1
			}
			out = append(out, byte(c))
		}
	}
	return out, missed
}

func skySameCells(a, b *Doc) bool {
	for y := 0; y < 25; y++ {
		for x := 0; x < 80; x++ {
			if va, vb := a.get(x, y), b.get(x, y); va != vb && !(isBlank(va) && isBlank(vb)) {
				return false
			}
		}
	}
	return b.cols <= 80 && b.rows <= 25
}

// ---------------------------------------------------------------- in CharEdit

func init() {
	formats = append(formats, Format{"skypix", "SkyPix (.spx)", ".spx", "pc"})
	editorOwn["skypix"] = true
	openAs = append(openAs, "skypix")
	openAsNames = append(openAsNames, "SkyPix (Amiga)")
}

// skyFontsAsked: files the font question was asked for in this run (by their first bytes).
var skyFontsAsked = map[string]bool{}

// loadSkyPix opens a SkyPix file on a PC page; false if b is none.
func (a *App) loadSkyPix(b []byte, name, path, as string) bool {
	if as != "skypix" && (as != "auto" || !isSkyPix(b)) {
		return false
	}
	if missing := skyFontsMissing(b); len(missing) > 0 && !skyFontsAsked[string(b[:min(len(b), 64)])] {
		skyFontsAsked[string(b[:min(len(b), 64)])] = true // asked once a file: "Not now" opens it in the VGA's font
		t := a.Tab
		var names []string
		for _, i := range missing {
			if f := skyFontFiles[i]; f.name != "" {
				names = append(names, fmt.Sprintf("%s %d", strings.TrimSuffix(f.name, ".font"), f.size))
			}
		}
		why := "SkyPix files draw their text in the Amiga's own fonts"
		if len(names) > 0 {
			why += " (this one: " + strings.Join(names, ", ") + ")"
		}
		a.dialog = &FontReq{title: "Amiga fonts", what: "the Amiga fonts", fetch: func() error { return fetchSkyFonts(missing) },
			why: why + ". They do not come with this program, but IcyDraw's source has them. Download them now? (" +
				"checked before they are used, kept in " + shownDir(appDataDir()) + ".) Not now: the text shows in the VGA's 8x8 font.",
			load: func() (string, error) { return "The Amiga fonts are here", nil },
			after: func() {
				if a.backTo(t) {
					a.loadBytes(b, name, path, as)
				}
			}}
	}
	a.mode, a.ice, a.srcKind = "pc", true, "skypix"
	a.fontFor["pc"] = "vga50"
	a.finishLoad(b, name, path, nil, "skypix")
	return true
}

// reparseSky reads the page again from its SkyPix file; false for other files.
func (a *App) reparseSky() bool {
	if a.srcKind != "skypix" || a.source == nil {
		a.sky = nil
		return false
	}
	l := readSkyPix(a.source)
	a.sky = l
	a.doc = l.cells.clone()
	a.cx, a.cy, a.sel, a.float = 0, 0, nil, nil
	note := ""
	if l.note != "" {
		note = " (" + l.note + ")"
	}
	a.msg(a.name + ": SkyPix drawing, kept as it is; what you type goes over it" + note)
	return true
}

// encodeSky writes the page as SkyPix; ok false for other formats.
func (a *App) encodeSky(fmtID string, d *Doc) ([]byte, bool) {
	if fmtID != "skypix" {
		return nil, false
	}
	out, missed := writeSkyPix(a.sky, d)
	if missed > 0 {
		a.msg(fmt.Sprintf("%d cells hold characters SkyPix takes as controls: written as spaces", missed))
	}
	return out, true
}

// drawSkyCell draws an empty cell of a SkyPix page: the picture under it. False for other cells.
func (a *App) drawSkyCell(fb *FB, px, py, z, x, y int, v uint32) bool {
	l := a.sky
	if l == nil || !isBlank(v) || !a.skyPage() || x < 0 || y < 0 || x >= 80 || y >= 25 || a.cellH() != 8 {
		return false
	}
	for r := 0; r < 8; r++ {
		row := l.pix[(y*8+r)*skyW+x*8:]
		for b := 0; b < 8; b++ {
			fb.fill(Rect{px + b*z, py + r*z, z, z}, a.pal[row[b]&15])
		}
	}
	return true
}
