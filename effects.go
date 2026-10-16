package main

// Charcoal's frame effects: animation templates that are more than an order of
// cells (Matrix rain, Decrypt, Drop, Assemble, Shine, Shade in, Melt in, Power
// on). Each turns a picture into frames that end exactly on it; random parts
// come from a seed kept with the page, so saving twice gives the same file. On
// a stream page the saved file is a delta stream (writeDeltaFrames): a clear
// screen, then each frame's changed cells only, so a terminal plays it at the
// modem's speed. Every page type keeps its own rules: teletext codes stay where
// they are, Spectrum ink and paper share their brightness, pages without free
// colours (Atari 8-bit, PET, an ST in high resolution) use reverse instead.

import (
	"math"
	"math/rand"
	"sort"
)

// effectDefaults: how many frames an effect takes and how long each is held (ms).
var effectDefaults = map[int][2]int{
	patMatrix:   {75, 40},
	patDecrypt:  {55, 40},
	patDrop:     {60, 30},
	patAssemble: {55, 35},
	patShine:    {32, 40},
	patShadeIn:  {45, 40},
	patMelt:     {50, 30},
	patPowerOn:  {34, 40},
}

// maxEffectFrames: no effect takes more frames than this.
const maxEffectFrames = 400

// isScan: the pattern is an order of cells (it can be a draw order on its own).
func isScan(pat int) bool { return pat >= 0 && pat < nTheDraw || pat == patSpiral || pat == patBlinds }

// isEffect: the pattern is one of the frame effects above.
func isEffect(pat int) bool {
	_, ok := effectDefaults[pat]
	return ok
}

// effectLength is the frame count of an effect: n, or the effect's own when n is 0.
func effectLength(pat, n int) int {
	if n <= 0 {
		n = effectDefaults[pat][0]
	}
	return max(2, min(maxEffectFrames, n))
}

// fx is what an effect works with.
type fx struct {
	a          *App
	d, e, base *Doc // the picture, the clear screen, the start (the clear screen, teletext codes kept)
	cols, rows int
	used       []bool
	r          *rand.Rand
	glyphs     []int
	asp        float64
	paper      int // the clear screen's colour
	bright     int // a colour that stands out on it
	colours    bool
	detail     int // detailLarge, detailMedium or detailSmall: how much changes each frame
}

func (a *App) newFx(d, e *Doc, seed int64) *fx {
	x := &fx{a: a, d: d, e: e, cols: d.cols, rows: d.rows, r: rand.New(rand.NewSource(seed)), asp: a.aspect()}
	n := d.cols * d.rows
	x.base = d.clone()
	x.used = make([]bool, n)
	for c := 0; c < n; c++ {
		if usedCell(a.mode, d.cells[c], e.cells[c]) {
			x.used[c] = true
			x.base.cells[c] = e.cells[c]
		}
	}
	x.colours = !a.tt() && !a.atari() && a.nColours(false) >= 4
	switch {
	case a.c64():
		x.paper = d.bg
	case a.tt(), a.atari():
	default:
		x.paper = cellBg(e.cells[0])
	}
	if c := a.pal[x.paper&15]; 2*(c>>16&255)+4*(c>>8&255)+3*(c&255) < 9*128 {
		x.bright = x.colour(0xFFFFFF)
	} else {
		x.bright = x.colour(0)
	}
	x.glyphs = a.effectGlyphs()
	return x
}

// effectGlyphs are the characters random glyphs are taken from: digits, capitals and a few of the page's own symbols.
func (a *App) effectGlyphs() []int {
	var g []int
	add := func(lo, hi int) {
		for c := lo; c <= hi; c++ {
			g = append(g, c)
		}
	}
	switch a.mode {
	case "c64": // screen codes
		add(0x01, 0x1A)
		add(0x30, 0x39)
		g = append(g, 0x2A, 0x2B, 0x3C, 0x3D, 0x3E, 0x4D, 0x4E, 0x56, 0x5B)
		return g
	case "atari":
		add('0', '9')
		add('A', 'Z')
		g = append(g, '*', '+', '<', '>', '=', 0x10, 0x13, 0x18)
		return g
	}
	add('0', '9')
	add('A', 'Z')
	g = append(g, '*', '+', '<', '>', '=')
	switch a.mode {
	case "pc":
		add(0x9C, 0x9F) // not 0x9B: an 8-bit CSI to some terminals
		add(0xE0, 0xEF)
	case "amiga":
		g = append(g, 0xA2, 0xA3, 0xA5, 0xA7, 0xB1, 0xB5, 0xD7, 0xDF, 0xF0, 0xF8)
	case "st":
		add(0xC2, 0xDC) // the ST's Hebrew letters
	case "zx":
		add(0x81, 0x8E) // block graphics
	case "cpc":
		add(0x81, 0x8E) // block graphics
		add(0xD4, 0xD7) // triangles
	}
	return g
}

// flight is how long a falling or flying character is on its way, against
// Large's: below it the characters arrive sooner, so fewer frames show each
// on the way (every one of those is a character taken off and put down).
func (x *fx) flight() float64 {
	return map[int]float64{detailLarge: 1, detailMedium: 0.7, detailSmall: 0.4}[x.detail]
}

func (x *fx) glyph() int { return x.glyphs[x.r.Intn(len(x.glyphs))] }

// colour is the page's colour nearest to rgb.
func (x *fx) colour(rgb uint32) int {
	n := x.a.nColours(false)
	if x.a.zx() {
		n = 16
	}
	return x.a.nearestColour(rgb, 0, max(1, n))
}

// writable: an effect may put something in cell c (teletext codes stay).
func (x *fx) writable(c int) bool { return !x.a.tt() || x.d.cells[c]&0x7F >= 0x20 }

func (x *fx) put(f *Doc, cx, cy int, v uint32) {
	if cx < 0 || cy < 0 || cx >= x.cols || cy >= x.rows {
		return
	}
	if c := cy*x.cols + cx; x.writable(c) {
		f.cells[c] = v
	}
}

// cellOf is character ch in colour col on the clear screen's paper at cell c;
// inverse: reversed (ink and paper swapped).
func (x *fx) cellOf(c, ch, col int, inverse bool) uint32 {
	p := x.e.cells[c]
	switch {
	case x.a.tt():
		return x.d.cells[c]&^0x7F | uint32(ch&0x7F)
	case x.a.atari():
		if inverse {
			ch |= 0x80
		}
		return p&^255 | uint32(ch&255)
	case x.a.c64():
		if inverse {
			ch ^= 0x80
		}
		return makeCell(ch, col, cellBg(p), 0)
	case x.a.zx():
		ink, paper := col&7, cellBg(p)&7
		if inverse {
			ink, paper = paper, ink
		}
		return zxCell(ch, ink, paper, col&8 != 0, false)
	}
	bg := cellBg(p)
	if inverse {
		col, bg = bg, col
		if (x.a.pc() || x.a.amiga()) && !x.a.ice {
			bg &= 7
		}
	}
	return makeCell(ch, col, bg, 0)
}

// inkOf is the colour cell c of the picture shows most (its ink, or the paper
// of a space), never the clear screen's own colour.
func (x *fx) inkOf(c int) int {
	v := x.d.cells[c]
	col := cellFg(v)
	if ch := cellCh(v); !x.a.c64() && (ch == 32 || ch == 0 || ch == 255) {
		col = cellBg(v)
	}
	if col == x.paper {
		col = x.bright
	}
	return col
}

// glint is cell v lit up: amt 1 white (or reversed where the page has no
// colours to spare), less: part of the way to white, its own bright colour.
func (x *fx) glint(v uint32, amt float64) uint32 {
	a := x.a
	switch {
	case a.tt():
		if amt >= 0.75 && v&0x7F > 0x20 {
			return v&^0x7F | 0x7F // the solid block, in the row's colour
		}
		return v
	case a.zx(): // BRIGHT, and inverse in the middle of the band
		ink, paper := cellFg(v)&7, cellBg(v)&7
		if amt >= 0.75 {
			ink, paper = paper, ink
		}
		return v&^0xFF00 | uint32(ink|8)<<8 | uint32(paper|8)<<12
	case !x.colours:
		if amt < 0.5 {
			return v
		}
		if a.atari() || a.c64() {
			return v ^ 0x80
		}
		return v&^0xFF00 | uint32(cellBg(v))<<8 | uint32(cellFg(v))<<12
	case a.c64():
		c := a.nearestColour(lerpRGB(a.pal[cellFg(v)], 0xFFFFFF, amt), 0, a.nColours(false))
		return v&^0xF00 | uint32(c)<<8
	}
	// The ink lights up; the paper only where it is what the cell shows (a space).
	lit := a.fadeCell(v, 0xFFFFFF, 1-amt)
	if ch := cellCh(v); ch != 32 && ch != 0 && ch != 255 {
		lit = lit&^0xF000 | v&0xF000
	}
	return lit
}

// lineCell is a piece of a bright horizontal line at cell c (Power on).
func (x *fx) lineCell(c int) uint32 {
	ch := map[string]int{"pc": 0xC4, "c64": 0x40, "atari": 0x12}[x.a.mode]
	if ch == 0 {
		ch = '-'
	}
	if x.a.tt() {
		ch = 0x7F
	}
	return x.cellOf(c, ch, x.bright, false)
}

// shadeSeq are the shade steps a cell goes through before its own character (Shade in).
func (x *fx) shadeSeq() []int {
	s := x.a.shadeSteps()
	if len(s) > 0 && s[0] == 32 {
		s = s[1:]
	}
	if len(s) >= 4 { // the last, a full block, would only hide the cell's own shape
		s = s[:len(s)-1]
	}
	if len(s) == 0 {
		s = []int{'.', ':', '+'}
	}
	return s
}

func ease3(t float64) float64 { t = clamp01(t); return 1 - (1-t)*(1-t)*(1-t) }

func clamp01(t float64) float64 { return max(0, min(1, t)) }

// effectFrames are the k frames of effect pat on picture d (e: the clear
// screen); the last is d. Spiral and Blinds are orders, not effects.
func (a *App) effectFrames(d, e *Doc, pat int, seed int64, k int) []*Doc {
	return a.effectFramesAt(d, e, pat, seed, k, detailLarge)
}

// effectFramesAt is effectFrames at a detail level (k 0: the level's own frames).
func (a *App) effectFramesAt(d, e *Doc, pat int, seed int64, k, detail int) []*Doc {
	k = effectLengthAt(pat, k, detail)
	x := a.newFx(d, e, seed)
	x.detail = detail
	var frames []*Doc
	for i := 0; i < k; i++ {
		frames = append(frames, nil)
	}
	gen := map[int]func(*fx, []*Doc){
		patMatrix: (*fx).matrix, patDecrypt: (*fx).decrypt, patDrop: (*fx).drop, patAssemble: (*fx).assemble,
		patShine: (*fx).shine, patShadeIn: (*fx).shadeIn, patMelt: (*fx).melt, patPowerOn: (*fx).powerOn,
	}[pat]
	if gen == nil {
		return nil
	}
	gen(x, frames)
	frames[k-1] = d.clone()
	return frames
}

// at is frame i's time, 0 to 1.
func tAt(i, k int) float64 { return float64(i) / float64(max(1, k-1)) }

// Matrix rain: in every column a stream of changing glyphs falls, a bright head
// and a darker trail; behind it the picture stays. Columns start at random times.
func (x *fx) matrix(frames []*Doc) {
	k := len(frames)
	trail := max(4, min(14, x.rows/3))
	churn := 3 // a trail's glyph changes one frame in churn (0: never)
	switch x.detail {
	case detailMedium:
		trail, churn = max(3, min(8, x.rows/4)), 8
	case detailSmall:
		trail, churn = 2, 0
	}
	type stream struct {
		s, dur float64
		l      int
	}
	cols := make([]stream, x.cols)
	for i := range cols {
		s := x.r.Float64() * 0.35
		dur := (0.93 - s) * (0.5 + 0.5*x.r.Float64())
		l := max(3, int(float64(trail)*(0.6+0.8*x.r.Float64())))
		if x.detail == detailSmall {
			l = trail
		}
		cols[i] = stream{s, dur, l}
	}
	head, near, tail := x.colour(0xDDFFDD), x.colour(0x55FF55), x.colour(0x00AA00)
	if x.a.zx() {
		head, near, tail = 15, 12, 4
	}
	fix := func(c int) int {
		if c == x.paper {
			return x.bright
		}
		return c
	}
	head, near, tail = fix(head), fix(near), fix(tail)
	if x.detail == detailSmall {
		near = tail // the head and one colour behind it
	}
	inverseHead := !x.colours || head == near
	if !x.colours {
		head, near, tail = x.bright, x.bright, x.bright
	}
	glyph := make([]int, x.cols*x.rows)
	for c := range glyph {
		glyph[c] = x.glyph()
	}
	for i := range frames {
		u := tAt(i, k)
		f := x.base.clone()
		for c := range glyph {
			if churn > 0 && x.r.Intn(churn) == 0 {
				glyph[c] = x.glyph()
			}
		}
		for cx, st := range cols {
			hy := int(math.Floor((u - st.s) / st.dur * float64(x.rows+st.l)))
			if hy < 0 {
				continue
			}
			for y := 0; y < x.rows && y <= hy; y++ {
				c := y*x.cols + cx
				switch back := hy - y; {
				case back >= st.l:
					f.cells[c] = x.d.cells[c]
				case back == 0:
					if x.a.tt() {
						x.put(f, cx, y, x.cellOf(c, 0x7F, 0, false))
					} else {
						x.put(f, cx, y, x.cellOf(c, glyph[c], head, inverseHead))
					}
				case back <= 2:
					x.put(f, cx, y, x.cellOf(c, glyph[c], near, false))
				default:
					x.put(f, cx, y, x.cellOf(c, glyph[c], tail, false))
				}
			}
		}
		frames[i] = f
	}
}

// Decrypt: every cell shows random glyphs until it locks to its own character,
// with a flash; the locking sweeps from left to right, a little ragged.
func (x *fx) decrypt(frames []*Doc) {
	k := len(frames)
	n := x.cols * x.rows
	lock, show := make([]float64, n), make([]float64, n)
	for c := range lock {
		pos := float64(c%x.cols) / float64(max(1, x.cols-1))
		lock[c] = max(0.05, min(0.93, 0.14+0.7*pos+(x.r.Float64()-0.5)*0.18))
		show[c] = max(0, lock[c]-0.3-0.3*x.r.Float64())
	}
	// Below Large the random glyphs change less often (Small: one each, kept), and Small has no flash.
	held := make([]uint32, n)
	for i := range frames {
		u, prev := tAt(i, k), tAt(i-1, k)
		f := x.base.clone()
		for c := 0; c < n; c++ {
			if !x.used[c] {
				continue
			}
			switch {
			case u < show[c]:
			case u < lock[c]:
				switch {
				case x.detail == detailLarge:
					f.cells[c] = x.cellOf(c, x.glyph(), x.inkOf(c), false)
				case held[c] == 0 || x.detail == detailMedium && x.r.Intn(3) == 0:
					held[c] = x.cellOf(c, x.glyph(), x.inkOf(c), false)
					f.cells[c] = held[c]
				default:
					f.cells[c] = held[c]
				}
			case prev < lock[c] && i > 0 && x.detail != detailSmall:
				f.cells[c] = x.glint(x.d.cells[c], 1)
			default:
				f.cells[c] = x.d.cells[c]
			}
		}
		frames[i] = f
	}
}

// Drop: every character falls from the top row to its place, faster and
// faster; each column fills from the bottom up, the columns in a wave from the left.
func (x *fx) drop(frames []*Doc) {
	k := len(frames)
	type faller struct {
		c       int
		s, fall float64
	}
	var all []faller
	for cx := 0; cx < x.cols; cx++ {
		wave := 0.45*float64(cx)/float64(max(1, x.cols-1)) + 0.05*x.r.Float64()
		var ys []int
		for y := x.rows - 1; y >= 0; y-- {
			if x.used[y*x.cols+cx] {
				ys = append(ys, y)
			}
		}
		gap := 0.22 / float64(max(1, len(ys)))
		for j, y := range ys {
			all = append(all, faller{y*x.cols + cx, wave + float64(j)*gap, 0.24 * x.flight() * math.Sqrt(float64(y+1)/float64(x.rows))})
		}
	}
	for i := range frames {
		u := tAt(i, k)
		f := x.base.clone()
		for _, p := range all {
			if u >= p.s+p.fall {
				f.cells[p.c] = x.d.cells[p.c]
			}
		}
		for _, p := range all {
			if t := u - p.s; t >= 0 && t < p.fall {
				y := float64(p.c/x.cols) * (t / p.fall) * (t / p.fall)
				x.put(f, p.c%x.cols, int(y), x.d.cells[p.c])
			}
		}
		frames[i] = f
	}
}

// Assemble: every character flies in a straight line from a random place to
// its own, slowing down as it comes; one that starts later is drawn over the others.
func (x *fx) assemble(frames []*Doc) {
	k := len(frames)
	type flyer struct {
		c            int
		s, dur       float64
		fromX, fromY float64
	}
	var all []flyer
	for c, u := range x.used {
		if u {
			all = append(all, flyer{c, x.r.Float64() * 0.55, (0.28 + 0.12*x.r.Float64()) * x.flight(), x.r.Float64() * float64(x.cols-1), x.r.Float64() * float64(x.rows-1)})
		}
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].s < all[j].s })
	for i := range frames {
		u := tAt(i, k)
		f := x.base.clone()
		for _, p := range all {
			if u >= p.s+p.dur {
				f.cells[p.c] = x.d.cells[p.c]
			}
		}
		for _, p := range all {
			if t := (u - p.s) / p.dur; t >= 0 && t < 1 {
				e := ease3(t)
				tx, ty := float64(p.c%x.cols), float64(p.c/x.cols)
				x.put(f, int(math.Round(p.fromX+(tx-p.fromX)*e)), int(math.Round(p.fromY+(ty-p.fromY)*e)), x.d.cells[p.c])
			}
		}
		frames[i] = f
	}
}

// Shine: the finished picture, and a slanted band of light passes over it once (a logo's glint).
func (x *fx) shine(frames []*Doc) {
	k := len(frames)
	w := max(4, float64(x.cols)/6)
	span := float64(x.cols-1) + float64(x.rows-1)*x.asp
	for i := range frames {
		p := -w + (span+2*w)*clamp01((tAt(i, k)-0.06)/0.86)
		f := x.d.clone()
		for c, u := range x.used {
			if !u {
				continue
			}
			q := float64(c%x.cols) + float64(c/x.cols)*x.asp
			switch dist := math.Abs(q - p); {
			case dist < w*0.4 || x.detail == detailSmall && dist < w*0.6:
				f.cells[c] = x.glint(x.d.cells[c], 1)
			case dist < w && x.detail != detailSmall:
				f.cells[c] = x.glint(x.d.cells[c], 0.5)
			}
		}
		frames[i] = f
	}
}

// Shade in: every cell comes up through the page's shade steps (PC: light,
// medium, dark shade) to its own character, in a ragged sweep from the upper left.
func (x *fx) shadeIn(frames []*Doc) {
	k := len(frames)
	steps := x.shadeSeq()
	switch { // fewer steps below Large: the middle ones
	case x.detail == detailSmall && len(steps) > 1:
		steps = steps[len(steps)/2 : len(steps)/2+1]
	case x.detail == detailMedium && len(steps) > 2:
		steps = steps[len(steps)-2:]
	}
	stepLen := min(0.07, 0.26/float64(len(steps)))
	span := float64(x.cols-1) + float64(x.rows-1)*x.asp
	start := make([]float64, len(x.used))
	for c := range start {
		start[c] = 0.52*(float64(c%x.cols)+float64(c/x.cols)*x.asp)/max(1, span) + 0.16*x.r.Float64()
	}
	for i := range frames {
		u := tAt(i, k)
		f := x.base.clone()
		for c, used := range x.used {
			if !used || u < start[c] {
				continue
			}
			if s := int((u - start[c]) / stepLen); s < len(steps) {
				f.cells[c] = x.cellOf(c, steps[s], x.inkOf(c), false)
			} else {
				f.cells[c] = x.d.cells[c]
			}
		}
		frames[i] = f
	}
}

// Melt in: Doom's screen melt the other way round: every column slides down
// into place, faster and faster, each a little behind or ahead of its neighbour.
func (x *fx) melt(frames []*Doc) {
	k := len(frames)
	delay := make([]float64, x.cols)
	dl := 0.25 * x.r.Float64()
	for i := range delay {
		if i > 0 {
			dl = max(0, min(0.3, dl+float64(x.r.Intn(3)-1)*0.025))
		}
		delay[i] = dl
	}
	for i := range frames {
		u := tAt(i, k)
		f := x.base.clone()
		for cx := 0; cx < x.cols; cx++ {
			t := clamp01((u - delay[cx]) / 0.62)
			p := int(math.Round(float64(x.rows) * math.Pow(t, 1.7)))
			for y := 0; y < p; y++ {
				src := (y+x.rows-p)*x.cols + cx
				if x.writable(src) {
					x.put(f, cx, y, x.d.cells[src])
				}
			}
		}
		frames[i] = f
	}
}

// Power on: a television switched on: a bright line grows across the middle,
// then opens up and down to the whole picture, its edges bright.
func (x *fx) powerOn(frames []*Doc) {
	k := len(frames)
	cy := float64(x.rows-1) / 2
	mid := x.rows / 2
	cx := float64(x.cols-1) / 2
	for i := range frames {
		u := tAt(i, k)
		f := x.base.clone()
		switch {
		case u < 0.2: // the line
			w := (cx + 1) * math.Sqrt(clamp01(u/0.14))
			for c := 0; c < x.cols; c++ {
				if math.Abs(float64(c)-cx) <= w {
					x.put(f, c, mid, x.lineCell(mid*x.cols+c))
				}
			}
		default:
			h := (cy + 1) * ease3((u-0.2)/0.62)
			for y := 0; y < x.rows; y++ {
				dy := math.Abs(float64(y) - cy)
				if dy > h {
					continue
				}
				edge := dy > h-1.2 && h < cy+0.9
				for c := 0; c < x.cols; c++ {
					j := y*x.cols + c
					switch {
					case !edge:
						f.cells[j] = x.d.cells[j]
					case x.used[j]:
						f.cells[j] = x.glint(x.d.cells[j], 1)
					default:
						x.put(f, c, y, x.lineCell(j))
					}
				}
			}
		}
		frames[i] = f
	}
}
