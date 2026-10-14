package main

// Animation templates, after TheDraw 4.63's "Easy Animation": a still picture
// is put on the screen cell by cell in one of TheDraw's fourteen scan patterns
// (Top, Left, Clock, Circles, Checkerboard, Angle, Gate, Pyramid, Squares,
// Scramble, Door, Diamond, Wiggle, Wiggle out), with its two toggles Reverse
// (the whole order backwards) and Flip (rows mirrored top to bottom), plus a
// few of Charcoal's own (fade in / out, typewriter, scroll in).
//
// The result is either the draw order of the saved stream (.ans .seq .ata
// .vt: a clear screen, then each cell with a cursor move before it where it
// does not follow the one before, attributes only where they change; any
// terminal plays it at the modem's speed), kept with the page (Tab.order)
// until changed or cleared, or frames (the strip, Play, GIF and APNG).
// Only cells that show something are drawn, so files stay small.

import (
	"fmt"
	"math"
	"math/rand"
	"slices"
	"sort"
	"strconv"
	"strings"
)

// The patterns: TheDraw's fourteen, then Charcoal's own.
const (
	patTop = iota
	patLeft
	patClock
	patCircles
	patChecker
	patAngle
	patGate
	patPyramid
	patSquares
	patScramble
	patDoor
	patDiamond
	patWiggle
	patWiggleOut
	patFadeIn
	patFadeOut
	patType
	patScroll
	nPatterns
)

// nTheDraw: the patterns before this one are TheDraw's.
const nTheDraw = patFadeIn

var patterns = [nPatterns]struct{ id, name, help string }{
	{"top", "Top", "Starts at the upper-left corner and goes across the screen"},
	{"left", "Left", "Starts at the upper-left corner and goes down the screen"},
	{"clock", "Clock", "A clockwise arm turns around the centre of the screen"},
	{"circles", "Circles", "Growing circles from the centre of the screen"},
	{"checkerboard", "Checkerboard", "Alternating checkerboard squares from the top of the screen"},
	{"angle", "Angle", "A diagonal from the upper-left corner scans across the screen"},
	{"gate", "Gate", "Lines from the left and right sides close in, both at once"},
	{"pyramid", "Pyramid", "A pyramid grows upwards from the bottom centre"},
	{"squares", "Squares", "Squares from the screen's border grow inward"},
	{"scramble", "Scramble", "The whole screen in random order"},
	{"door", "Door", "Top and bottom at once, from both sides of the screen"},
	{"diamond", "Diamond", "A growing diamond from the centre of the screen"},
	{"wiggle", "Wiggle", "From the upper-left corner, row by row, alternating left and right"},
	{"wiggleout", "Wiggle out", "Wiggle from the centre of the screen, up and down at once"},
	{"fadein", "Fade in", "Charcoal's own: the colours come up out of the paper, dark to bright"},
	{"fadeout", "Fade out", "Charcoal's own: the colours sink into the paper (frames only)"},
	{"typewriter", "Typewriter", "Charcoal's own: row by row with a cursor, spaces too, at an even pace"},
	{"scroll", "Scroll in", "Charcoal's own: the picture scrolls in from the bottom (frames only)"},
}

func patternByID(id string) int {
	for i, p := range patterns {
		if p.id == id {
			return i
		}
	}
	return -1
}

// framesOnly: the pattern cannot be a saved file's draw order (the file's last
// picture would not be the page).
func framesOnly(pat int, erase bool) bool {
	return erase || pat == patFadeOut || pat == patScroll
}

// hasToggles: Reverse and Flip change this pattern.
func hasToggles(pat int) bool { return pat < nTheDraw || pat == patType }

// drawOrder is the order a page's stream file draws it in (kept with the page).
type drawOrder struct {
	Pattern string `json:"pattern"`
	Reverse bool   `json:"reverse,omitempty"`
	Flip    bool   `json:"flip,omitempty"`
}

func (o *drawOrder) pat() int { return patternByID(o.Pattern) }

func (o *drawOrder) String() string {
	s := patterns[max(0, o.pat())].name
	if o.Reverse {
		s += ", reversed"
	}
	if o.Flip {
		s += ", flipped"
	}
	return s
}

// ---------------------------------------------------------------- scan patterns

// scanOpts are TheDraw's two toggles, and the shape of a cell.
type scanOpts struct {
	reverse, flip bool
	aspect        float64 // a cell's height over its width (2 for 8x16 cells); 0 means 2
}

// scanSeed makes Scramble the same every time, so saving twice gives the same file.
const scanSeed = 1991

// scanOrder gives the cells (y*cols+x) a pattern draws, in order: every cell
// for which used says yes (all of them when used is nil), each once. Flip
// mirrors the scan's rows (a scan from the top starts at the bottom); Reverse
// turns the whole order round.
func scanOrder(pat int, o scanOpts, cols, rows int, used func(x, y int) bool) []int {
	asp := o.aspect
	if asp <= 0 {
		asp = 2
	}
	var cells []int
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			if used == nil || used(x, y) {
				cells = append(cells, y*cols+x)
			}
		}
	}
	key := scanKey(pat, cols, rows, asp)
	keys := make([][4]float64, len(cells))
	for i, c := range cells {
		x, y := c%cols, c/cols
		if o.flip { // the scan visits (x, rows-1-y) where it would visit (x, y)
			y = rows - 1 - y
		}
		k := key(x, y)
		keys[i] = [4]float64{k[0], k[1], k[2], float64(y*cols + x)} // the last breaks every tie
	}
	idx := make([]int, len(cells))
	for i := range idx {
		idx[i] = i
	}
	sort.Slice(idx, func(i, j int) bool {
		a, b := keys[idx[i]], keys[idx[j]]
		for k := range a {
			if a[k] != b[k] {
				return a[k] < b[k]
			}
		}
		return false
	})
	out := make([]int, len(cells))
	for i, k := range idx {
		out[i] = cells[k]
	}
	if o.reverse {
		slices.Reverse(out)
	}
	return out
}

// scanKey is where a pattern puts cell (x, y): cells are drawn by their keys, smallest first.
func scanKey(pat, cols, rows int, asp float64) func(x, y int) [3]float64 {
	cx, cy := float64(cols-1)/2, float64(rows-1)/2
	// clock: the angle from 12 o'clock, clockwise, of a cell seen from the centre
	// (rows count asp times a column, so circles look round).
	clock := func(x, y int) float64 {
		a := math.Atan2(float64(x)-cx, -(float64(y)-cy)*asp)
		if a < 0 {
			a += 2 * math.Pi
		}
		return a
	}
	dist := func(x, y int) float64 { return math.Hypot(float64(x)-cx, (float64(y)-cy)*asp) }
	switch pat {
	case patLeft:
		return func(x, y int) [3]float64 { return [3]float64{float64(x), float64(y)} }
	case patClock:
		return func(x, y int) [3]float64 { return [3]float64{clock(x, y), dist(x, y)} }
	case patCircles:
		return func(x, y int) [3]float64 { return [3]float64{dist(x, y), clock(x, y)} }
	case patChecker:
		// Squares of 8 columns, as tall as they are wide; the even ones first, then the odd.
		bw, bh := 8, max(1, int(math.Round(8/asp)))
		return func(x, y int) [3]float64 {
			bx, by := x/bw, y/bh
			return [3]float64{float64((bx + by) & 1), float64(by), float64(bx)}
		}
	case patAngle:
		return func(x, y int) [3]float64 { return [3]float64{float64(x) + float64(y)*asp, float64(y)} }
	case patGate:
		return func(x, y int) [3]float64 {
			return [3]float64{float64(min(x, cols-1-x)), float64(y), float64(x)}
		}
	case patPyramid:
		return func(x, y int) [3]float64 {
			up := float64(rows - 1 - y)
			return [3]float64{up*asp + math.Abs(float64(x)-cx), up, math.Abs(float64(x) - cx)}
		}
	case patSquares:
		start := clock(0, 0) // each ring goes round clockwise from its upper-left corner
		return func(x, y int) [3]float64 {
			ring := math.Floor(min(float64(min(x, cols-1-x))/asp, float64(min(y, rows-1-y))))
			a := clock(x, y) - start
			if a < 0 {
				a += 2 * math.Pi
			}
			return [3]float64{ring, a}
		}
	case patScramble:
		rank := make([]int32, cols*rows)
		for i, c := range rand.New(rand.NewSource(scanSeed)).Perm(cols * rows) {
			rank[c] = int32(i)
		}
		return func(x, y int) [3]float64 { return [3]float64{float64(rank[y*cols+x])} }
	case patDoor:
		return func(x, y int) [3]float64 {
			return [3]float64{float64(min(y, rows-1-y)), float64(min(x, cols-1-x)), float64(y)}
		}
	case patDiamond:
		return func(x, y int) [3]float64 {
			return [3]float64{math.Abs(float64(x)-cx) + math.Abs(float64(y)-cy)*asp, clock(x, y)}
		}
	case patWiggle:
		return func(x, y int) [3]float64 {
			if y%2 == 1 {
				x = cols - 1 - x
			}
			return [3]float64{float64(y), float64(x)}
		}
	case patWiggleOut:
		return func(x, y int) [3]float64 {
			k := math.Floor(math.Abs(float64(y) - cy))
			if int(k)%2 == 1 {
				x = cols - 1 - x
			}
			return [3]float64{k, float64(x), float64(y)}
		}
	}
	return func(x, y int) [3]float64 { return [3]float64{float64(y), float64(x)} } // Top, and the rest
}

// ---------------------------------------------------------------- which cells show

// usedCell: v shows something on paper e (the cell of a clear screen). Spaces
// count only for a background of their own; on teletext only letters and
// mosaics count (the codes are always there: they colour the row).
func usedCell(mode string, v, e uint32) bool {
	switch mode {
	case "tt":
		return v&0x7F > 0x20
	case "atari", "c64": // the character is all a cell has (a C64 space shows no colour)
		return v&255 != e&255
	case "zx":
		return v != e
	}
	if v == e {
		return false
	}
	if ch := v & 255; (ch == 32 || ch == 0) && v&underline == 0 {
		return cellBg(v) != cellBg(e)
	}
	return true
}

// paperOf is the page as a clear screen leaves it, the size and colours of d.
func (a *App) paperOf(d *Doc) *Doc {
	e := a.blankLike(d) // its first cell is the paper (an ST page's rows past the screen are not)
	n := newDoc(d.cols, d.rows)
	n.bg, n.border, n.ink, n.glyph0 = e.bg, e.border, e.ink, e.glyph0
	fillCells(n.cells, e.cells[0])
	return n
}

// aspect is a cell's height over its width on this page.
func (a *App) aspect() float64 { return float64(a.cellH()) / float64(max(1, a.cellW())) }

// cellsInOrder are the cells of d a pattern draws, in order. The typewriter
// types every row up to its last used cell, spaces too.
func (a *App) cellsInOrder(d *Doc, e *Doc, pat int, rev, flip bool) []int {
	used := func(x, y int) bool { return usedCell(a.mode, d.get(x, y), e.get(x, y)) }
	o := scanOpts{reverse: rev, flip: flip, aspect: a.aspect()}
	if pat == patType {
		last := make([]int, d.rows)
		for y := range last {
			last[y] = -1
			for x := d.cols - 1; x >= 0; x-- {
				if used(x, y) {
					last[y] = x
					break
				}
			}
		}
		return scanOrder(patTop, o, d.cols, d.rows, func(x, y int) bool { return x <= last[y] })
	}
	if pat >= nTheDraw {
		pat = patTop
	}
	return scanOrder(pat, o, d.cols, d.rows, used)
}

// ---------------------------------------------------------------- fading

// fadeCell is cell v with its colours part of the way (t, 0 to 1) from the
// paper colour to their own, each the nearest the page can show.
func (a *App) fadeCell(v uint32, paper uint32, t float64) uint32 {
	fg, bg := cellFg(v), cellBg(v)
	nfg, nbg := a.nColours(false), a.nColours(true)
	switch {
	case a.amiga() || a.pc():
		if !a.ice {
			nbg = 8
		}
	case a.zx(): // ink and paper share their brightness
		b := bg & 8
		nf := a.nearestColour(lerpRGB(paper, a.pal[fg], t), b, b+8)
		nb := a.nearestColour(lerpRGB(paper, a.pal[bg], t), b, b+8)
		return v&^0xFF00 | uint32(nf)<<8 | uint32(nb)<<12
	}
	nf := a.nearestColour(lerpRGB(paper, a.pal[fg], t), 0, nfg)
	nb := bg
	if !a.c64() && !a.atari() {
		blink := 0
		if bg >= nbg { // a PC background above 7 without iCE is blinking: kept
			blink, bg = bg&8, bg&7
		}
		nb = a.nearestColour(lerpRGB(paper, a.pal[bg], t), 0, nbg) | blink
	}
	return v&^0xFF00 | uint32(nf&15)<<8 | uint32(nb&15)<<12
}

func lerpRGB(p, c uint32, t float64) uint32 {
	l := func(s uint) uint32 {
		a, b := float64(p>>s&255), float64(c>>s&255)
		return uint32(math.Round(a+(b-a)*t)) << s
	}
	return l(16) | l(8) | l(0)
}

// nearestColour is the palette colour from lo up to (not with) hi nearest to rgb.
func (a *App) nearestColour(rgb uint32, lo, hi int) int {
	best, bd := lo, math.MaxInt
	for i := lo; i < min(hi, 16); i++ {
		c := a.pal[i]
		dr, dg, db := int(c>>16&255)-int(rgb>>16&255), int(c>>8&255)-int(rgb>>8&255), int(c&255)-int(rgb&255)
		if d := 2*dr*dr + 4*dg*dg + 3*db*db; d < bd {
			best, bd = i, d
		}
	}
	return best
}

// paperColour is the colour cells fade from: the clear screen's background.
func (a *App) paperRGB(d, e *Doc) uint32 {
	if a.screen() {
		return a.pal[d.bg]
	}
	return a.pal[cellBg(e.cells[0])]
}

// fadeRefused says why a page cannot fade, or "".
func (a *App) fadeRefused() string {
	switch {
	case a.tt():
		return "Teletext colours are codes in the rows: they cannot fade"
	case a.atari():
		return "An Atari page has one colour for all its text: nothing to fade"
	case a.nColours(false) < 2:
		return "A PET screen has one colour: nothing to fade"
	}
	return ""
}

// fadePasses is how many times a fade-in draws the picture in a saved file.
const fadePasses = 4

// ---------------------------------------------------------------- drawing as a stream

// drawStep puts cell v at (x, y).
type drawStep struct {
	x, y int
	v    uint32
}

// drawSteps are the steps that draw page d in order o (a fade draws it several times).
func (a *App) drawSteps(d *Doc, o *drawOrder) []drawStep {
	e := a.paperOf(d)
	pat := o.pat()
	order := a.cellsInOrder(d, e, pat, o.Reverse, o.Flip)
	var steps []drawStep
	if pat == patFadeIn {
		paper := a.paperRGB(d, e)
		for p := 1; p <= fadePasses; p++ {
			for _, c := range order {
				v := d.cells[c]
				if p < fadePasses {
					v = a.fadeCell(v, paper, float64(p)/fadePasses)
				}
				steps = append(steps, drawStep{c % d.cols, c / d.cols, v})
			}
		}
		return steps
	}
	for _, c := range order {
		steps = append(steps, drawStep{c % d.cols, c / d.cols, d.cells[c]})
	}
	return steps
}

// streamOpts: what a page type's stream needs to know.
type streamOpts struct {
	amiga, sauce, lower bool
	res                 STRes
}

// cursorWriter writes one page type's codes, keeping track of the terminal's cursor.
type cursorWriter interface {
	begin() []byte
	at() (int, int)
	move(x, y int) []byte
	put(v uint32, later func(x, y int) bool) []byte
	end() []byte
}

// writeDrawSteps writes the steps as a stream of format fmtID ("ansi", "seq",
// "ata" or "vt52") on a clear screen: each cell with a cursor move before it
// where it does not follow the one before. offs[i] is the stream's length
// after step i. The cursor ends where the page's last row counts, so the file
// opens exactly as tall as d.
func writeDrawSteps(fmtID string, d *Doc, steps []drawStep, o streamOpts) (out []byte, offs []int) {
	var w cursorWriter
	switch fmtID {
	case "seq":
		w = &petWriter{cols: d.cols, rows: d.rows, lower: o.lower, fg: -1}
	case "ata":
		w = &ataWriter{cols: d.cols}
	case "vt52":
		w = &vtWriter{cols: d.cols, res: o.res}
	default:
		w = &ansiWriter{cols: d.cols, amiga: o.amiga, raw1A: o.sauce}
	}
	lastAt := make([]int32, d.cols*d.rows) // the last step that draws each cell
	for i, s := range steps {
		lastAt[s.y*d.cols+s.x] = int32(i + 1)
	}
	out = w.begin()
	offs = make([]int, len(steps))
	for i, s := range steps {
		if x, y := w.at(); x != s.x || y != s.y {
			out = append(out, w.move(s.x, s.y)...)
		}
		later := func(x, y int) bool {
			return x >= 0 && y >= 0 && x < d.cols && y < d.rows && int(lastAt[y*d.cols+x]) > i+1
		}
		out = append(out, w.put(s.v, later)...)
		offs[i] = len(out)
	}
	// The page's last row: a file whose cursor stops above it would open shorter.
	if x, y := w.at(); cursorRows(x, y) < d.rows {
		if d.cols > 1 {
			out = append(out, w.move(1, d.rows-1)...)
		} else {
			out = append(out, w.move(0, d.rows)...)
		}
	}
	return append(out, w.end()...), offs
}

// shortest is the shortest of some ways to move the cursor.
func shortest(ways ...[]byte) []byte {
	var best []byte
	for _, w := range ways {
		if w != nil && (best == nil || len(w) < len(best)) {
			best = w
		}
	}
	return best
}

func repeat(b []byte, n int) []byte {
	var out []byte
	for range max(0, n) {
		out = append(out, b...)
	}
	return out
}

// ansiWriter: ANSI (PC) or the Amiga console: CSI cursor moves, SGR colours.
type ansiWriter struct {
	cols         int
	amiga, raw1A bool
	x, y         int
	cur          attrs
}

func (w *ansiWriter) at() (int, int) { return w.x, w.y }

func (w *ansiWriter) begin() []byte {
	w.cur = attrs{fg: 7}
	if w.amiga {
		return []byte("\x1b[0m\x0c")
	}
	return []byte("\x1b[0m\x1b[2J\x1b[1;1H")
}

func (w *ansiWriter) move(x, y int) []byte {
	csi := func(n int, f string) []byte {
		if n == 1 {
			return []byte("\x1b[" + f)
		}
		return []byte("\x1b[" + strconv.Itoa(n) + f)
	}
	var abs []byte
	switch {
	case x == 0 && y == 0:
		abs = []byte("\x1b[H")
	case x == 0:
		abs = []byte(fmt.Sprintf("\x1b[%dH", y+1))
	default:
		abs = []byte(fmt.Sprintf("\x1b[%d;%dH", y+1, x+1))
	}
	var vert, horiz []byte
	switch {
	case y > w.y:
		vert = csi(y-w.y, "B")
	case y < w.y:
		vert = csi(w.y-y, "A")
	}
	switch {
	case x > w.x:
		horiz = csi(x-w.x, "C")
	case x < w.x:
		horiz = csi(w.x-x, "D")
	}
	rel := append(append([]byte{}, vert...), horiz...)
	var nl []byte // line ends: to the start of a row below
	if y > w.y && y-w.y <= 3 {
		end := "\r\n"
		if w.amiga {
			end = "\n"
		}
		nl = repeat([]byte(end), y-w.y)
		if x > 0 {
			nl = append(nl, csi(x, "C")...)
		}
	}
	w.x, w.y = x, y
	return shortest(abs, rel, nl)
}

func (w *ansiWriter) put(v uint32, _ func(x, y int) bool) []byte {
	var out []byte
	fg, bg := cellFg(v), cellBg(v)
	want := attrs{fg: fg & 7, bold: fg > 7, bg: bg & 7, blink: bg > 7, it: v&italic != 0, ul: v&underline != 0}
	cur := w.cur
	var p []string
	if (cur.bold && !want.bold) || (cur.blink && !want.blink) || (cur.it && !want.it) || (cur.ul && !want.ul) {
		p = append(p, "0")
		cur = attrs{fg: 7}
	}
	if want.bold && !cur.bold {
		p = append(p, "1")
	}
	if want.it && !cur.it {
		p = append(p, "3")
	}
	if want.ul && !cur.ul {
		p = append(p, "4")
	}
	if want.blink && !cur.blink {
		p = append(p, "5")
	}
	if want.fg != cur.fg {
		p = append(p, strconv.Itoa(30+want.fg))
	}
	if want.bg != cur.bg {
		p = append(p, strconv.Itoa(40+want.bg))
	}
	if len(p) > 0 {
		out = append(out, "\x1b["+strings.Join(p, ";")+"m"...)
		w.cur = want
	}
	ch := byte(cellCh(v))
	switch ch { // bytes a terminal would take as controls (as writeAnsi)
	case 0, 0x1B, 0x0A, 0x0D, 0x08, 0x09, 0x07:
		ch = 32
	case 0x1A:
		if !w.raw1A {
			ch = 32
		}
	}
	if w.amiga && (ch < 0x20 || (ch >= 0x7F && ch < 0xA0)) {
		ch = 32
	}
	out = append(out, ch)
	if w.x++; w.x >= w.cols { // the terminal wraps at once
		w.x, w.y = 0, w.y+1
	}
	return out
}

func (w *ansiWriter) end() []byte { return []byte("\x1b[0m") }

// petWriter: PETSCII. No direct cursor address: HOME, RETURN and the four cursor keys.
type petWriter struct {
	cols     int
	rows     int
	lower    bool
	x, y, fg int
	rev      bool
}

func (w *petWriter) at() (int, int) { return w.x, w.y }

func (w *petWriter) begin() []byte {
	if w.lower {
		return []byte{0x93, 0x0E}
	}
	return []byte{0x93, 0x8E}
}

func (w *petWriter) move(x, y int) []byte {
	var rel []byte
	if y > w.y {
		rel = repeat([]byte{0x11}, y-w.y)
	} else {
		rel = repeat([]byte{0x91}, w.y-y)
	}
	if x > w.x {
		rel = append(rel, repeat([]byte{0x1D}, x-w.x)...)
	} else {
		rel = append(rel, repeat([]byte{0x9D}, w.x-x)...)
	}
	home := append(append([]byte{0x13}, repeat([]byte{0x11}, y)...), repeat([]byte{0x1D}, x)...)
	var ret []byte // RETURN: to the start of the next row (and reverse off)
	if y > w.y {
		ret = append(append([]byte{0x0D}, repeat([]byte{0x11}, y-w.y-1)...), repeat([]byte{0x1D}, x)...)
	}
	b := shortest(rel, home, ret)
	if len(b) > 0 && b[0] == 0x0D {
		w.rev = false
	}
	w.x, w.y = x, y
	return b
}

func (w *petWriter) step() {
	if w.x++; w.x >= w.cols {
		w.x, w.y = 0, w.y+1
	}
}

func (w *petWriter) put(v uint32, later func(x, y int) bool) []byte {
	var out []byte
	sc := cellCh(v)
	if r := sc&0x80 != 0; r != w.rev {
		w.rev = r
		out = append(out, map[bool]byte{true: 0x12, false: 0x92}[r])
	}
	if sc != 0x20 && cellFg(v) != w.fg { // a space shows no colour
		w.fg = cellFg(v)
		out = append(out, petColour[w.fg])
	}
	c := screenToPet(sc & 0x7F)
	out = append(out, c)
	w.step()
	if c == 0x22 {
		// A quote turns on the screen editor's quote mode, where cursor and
		// colour codes print as glyphs. Off again: another quote on a cell drawn
		// later anyway, or RETURN; past the page's last row (RETURN would make
		// the page a row taller) a quote on the empty row and DEL taking it off.
		switch {
		case later(w.x, w.y):
			out = append(out, 0x22)
			w.step()
		case w.y < w.rows:
			out = append(out, 0x0D)
			w.x, w.y, w.rev = 0, w.y+1, false
		default:
			out = append(out, 0x22, 0x14)
		}
	}
	return out
}

func (w *petWriter) end() []byte { return nil }

// ataWriter: ATASCII. The cursor keys only (left and right wrap round the row) and EOL.
type ataWriter struct {
	cols int
	x, y int
}

func (w *ataWriter) at() (int, int) { return w.x, w.y }
func (w *ataWriter) begin() []byte  { return []byte{0x7D} }

func (w *ataWriter) across(from, to int) []byte {
	r := (to - from + w.cols) % w.cols
	l := (from - to + w.cols) % w.cols
	if r <= l {
		return repeat([]byte{0x1F}, r)
	}
	return repeat([]byte{0x1E}, l)
}

func (w *ataWriter) move(x, y int) []byte {
	var rel []byte
	if y > w.y {
		rel = repeat([]byte{0x1D}, y-w.y)
	} else {
		rel = repeat([]byte{0x1C}, w.y-y)
	}
	rel = append(rel, w.across(w.x, x)...)
	var eol []byte
	if y > w.y {
		eol = append(append([]byte{0x9B}, repeat([]byte{0x1D}, y-w.y-1)...), w.across(0, x)...)
	}
	w.x, w.y = x, y
	return shortest(rel, eol)
}

func (w *ataWriter) put(v uint32, _ func(x, y int) bool) []byte {
	c := byte(cellCh(v))
	if c == 0x9B { // the EOL glyph cannot be sent, even after ESC
		c = 0x20
	}
	var out []byte
	if atariControl(c) {
		out = append(out, 0x1B)
	}
	out = append(out, c)
	if w.x++; w.x >= w.cols {
		w.x, w.y = 0, w.y+1
	}
	return out
}

func (w *ataWriter) end() []byte { return nil }

// vtWriter: VT52 on an Atari ST: ESC Y row column, wrap off (as writeVT52).
type vtWriter struct {
	cols   int
	res    STRes
	x, y   int
	fg, bg int
}

func (w *vtWriter) at() (int, int) { return w.x, w.y }

func (w *vtWriter) begin() []byte {
	w.fg, w.bg = w.res.fg, w.res.bg
	return []byte{0x1B, 'b', byte('0' + w.res.fg), 0x1B, 'c', byte('0' + w.res.bg), 0x1B, 'E', 0x1B, 'w'}
}

func (w *vtWriter) move(x, y int) []byte {
	var b []byte
	switch {
	case w.x < w.cols && y == w.y && x == w.x+1:
		b = []byte{0x1B, 'C'}
	case w.x < w.cols && y == w.y && x == w.x-1:
		b = []byte{0x1B, 'D'}
	case w.x < w.cols && y == w.y && x == 0:
		b = []byte{'\r'}
	case x == w.x && y == w.y+1 && w.x < w.cols:
		b = []byte{0x1B, 'B'}
	case x == w.x && y == w.y-1 && w.x < w.cols:
		b = []byte{0x1B, 'A'}
	default:
		// ESC Y reaches the screen's rows; a page taller than the screen goes on with cursor down.
		top := max(1, w.res.rows) - 1
		b = []byte{0x1B, 'Y', byte(32 + min(y, top)), byte(32 + x)}
		b = append(b, repeat([]byte{0x1B, 'B'}, y-top)...)
	}
	w.x, w.y = x, y
	return b
}

func (w *vtWriter) put(v uint32, _ func(x, y int) bool) []byte {
	var out []byte
	if f := cellFg(v); f != w.fg && cellCh(v) != 32 {
		w.fg = f
		out = append(out, 0x1B, 'b', byte('0'+f))
	}
	if b := cellBg(v); b != w.bg {
		w.bg = b
		out = append(out, 0x1B, 'c', byte('0'+b))
	}
	out = append(out, stSafeByte(byte(cellCh(v))))
	w.x++ // wrap is off: the cursor stays on the row
	return out
}

func (w *vtWriter) end() []byte { return nil }

// streamFormats are the formats a draw order is written in.
var streamFormats = map[string]bool{"ansi": true, "seq": true, "ata": true, "vt52": true}

// writeOrdered writes picture d in the page's draw order as fmtID.
func (a *App) writeOrdered(fmtID string, d *Doc, sauce bool) []byte {
	out, _ := writeDrawSteps(fmtID, d, a.drawSteps(d, a.order), a.streamOpts(sauce))
	return out
}

func (a *App) streamOpts(sauce bool) streamOpts {
	return streamOpts{amiga: a.amiga(), sauce: sauce, lower: a.lower(), res: a.res()}
}

// pageStreamFormat is the stream format of the page type ("" for teletext and the Spectrum).
func (a *App) pageStreamFormat() string {
	return map[string]string{"amiga": "ansi", "pc": "ansi", "c64": "seq", "atari": "ata", "st": "vt52"}[a.mode]
}

// orderedPlayBytes is what Play sends for a page with a draw order (nil: none).
func (a *App) orderedPlayBytes() []byte {
	f := a.pageStreamFormat()
	if a.order == nil || a.order.pat() < 0 || a.nFrames() > 1 || f == "" {
		return nil
	}
	d := a.doc
	if limit := map[string]int{"seq": a.mach().cols, "ata": 40}[f]; limit > 0 && d.cols > limit {
		d = d.withCols(limit)
	}
	return a.writeOrdered(f, d, false)
}

// orderedSave: Save writes this page's single picture in its draw order.
func (a *App) orderedSave(fmtID string) bool {
	return a.order != nil && a.order.pat() >= 0 && streamFormats[fmtID] && a.nFrames() == 1 && !framesOnly(a.order.pat(), false)
}

// orderNote is the status line's word on the draw order.
func (a *App) orderNote() string {
	if a.order == nil || a.nFrames() > 1 || a.pageStreamFormat() == "" {
		return ""
	}
	return "  Draws: " + a.order.String()
}

// ---------------------------------------------------------------- frames

// animSpec is what the Animate requester asks for.
type animSpec struct {
	pat           int
	reverse, flip bool
	erase         bool // the picture goes away instead of coming (frames)
	n             int  // a frame every n cells (scroll: rows; fade: frames in all)
	ms            int  // > 0: a frame every ms at the modem's speed instead of every n cells
	hold          int  // ms each frame is shown
	baud          int
}

// Holds of the ends of an effect: the finished picture stays a while.
const (
	animEndHold   = 2000
	animStartHold = 1000
)

// transitionFrames turns picture d into the frames of an effect, all of them
// together no more than maxCells cells. note says what had to give.
func (a *App) transitionFrames(d *Doc, s animSpec, maxCells int) (docs []*Doc, holds []int, note string) {
	e := a.paperOf(d)
	size := d.cols * d.rows
	maxK := min(maxSplitFrames, maxCells/max(1, size))
	if maxK < 2 {
		return nil, nil, "The page is too large to make frames of"
	}
	hold := max(20, s.hold)
	switch s.pat {
	case patFadeIn, patFadeOut:
		k := max(2, min(s.n, maxK-1))
		if k < s.n {
			note = fmt.Sprintf("%d steps fit", k)
		}
		paper := a.paperRGB(d, e)
		order := a.cellsInOrder(d, e, patTop, false, false)
		for i := 0; i <= k; i++ {
			t := float64(i) / float64(k)
			if s.pat == patFadeOut {
				t = 1 - t
			} else if i == 0 {
				continue // a fade in starts from the first step, not the empty paper
			}
			f := d.clone()
			for _, c := range order {
				switch t {
				case 0:
					f.cells[c] = e.cells[c]
				case 1:
				default:
					f.cells[c] = a.fadeCell(d.cells[c], paper, t)
				}
			}
			docs, holds = append(docs, f), append(holds, hold)
		}
	case patScroll:
		n := max(1, s.n)
		if k := (d.rows + n - 1) / n; k > maxK {
			n = (d.rows + maxK - 1) / maxK
			note = fmt.Sprintf("%d rows a frame, to fit", n)
		}
		for k := n; ; k += n {
			k = min(k, d.rows)
			f := e.clone()
			off := d.rows - k // the picture's top row is on this row
			for y := off; y < d.rows; y++ {
				copy(f.cells[y*d.cols:(y+1)*d.cols], d.cells[(y-off)*d.cols:(y-off+1)*d.cols])
			}
			docs, holds = append(docs, f), append(holds, hold)
			if k == d.rows {
				break
			}
		}
	default:
		order := a.cellsInOrder(d, e, s.pat, s.reverse, s.flip)
		if len(order) == 0 {
			return nil, nil, "Nothing is drawn on the page"
		}
		cuts, cutHolds := a.animCuts(d, e, order, s)
		if len(cuts) > maxK-1 { // fewer, larger steps
			n := (len(order) + maxK - 2) / (maxK - 1)
			cuts, cutHolds = nil, nil
			for c := n; ; c += n {
				cuts, cutHolds = append(cuts, min(c, len(order))), append(cutHolds, hold)
				if c >= len(order) {
					break
				}
			}
			note = fmt.Sprintf("%d cells a frame, to fit", n)
		}
		f := d.clone()
		if !s.erase {
			for _, c := range order {
				f.cells[c] = e.cells[c]
			}
		} else {
			docs, holds = append(docs, d.clone()), append(holds, animStartHold)
		}
		prev := 0
		for i, cut := range cuts {
			for _, c := range order[prev:cut] {
				if s.erase {
					f.cells[c] = e.cells[c]
				} else {
					f.cells[c] = d.cells[c]
				}
			}
			prev = cut
			fr := f.clone()
			if s.pat == patType && cut < len(order) { // the cursor where the next character goes
				c := order[cut]
				fr.cells[c] = a.typeCursor(d.cells[c])
			}
			docs, holds = append(docs, fr), append(holds, cutHolds[i])
		}
	}
	if len(docs) > 0 && !s.erase && s.pat != patFadeOut {
		holds[len(holds)-1] = max(holds[len(holds)-1], animEndHold)
	}
	return docs, holds, note
}

// animCuts are where the frames end in the order (cells drawn by then) and
// each frame's hold: every n cells, or every ms at the modem's speed (the
// bytes the page's stream needs for those cells).
func (a *App) animCuts(d, e *Doc, order []int, s animSpec) (cuts, holds []int) {
	if s.ms <= 0 {
		n := max(1, s.n)
		for c := n; ; c += n {
			cuts, holds = append(cuts, min(c, len(order))), append(holds, max(20, s.hold))
			if c >= len(order) {
				return
			}
		}
	}
	baud := max(300, s.baud)
	offs := make([]int, len(order))
	if f := a.pageStreamFormat(); f != "" {
		steps := make([]drawStep, len(order))
		for i, c := range order {
			v := d.cells[c]
			if s.erase {
				v = e.cells[c]
			}
			steps[i] = drawStep{c % d.cols, c / d.cols, v}
		}
		_, offs = writeDrawSteps(f, d, steps, a.streamOpts(false))
	} else { // teletext and the Spectrum have no stream: about two bytes a cell
		for i := range offs {
			offs[i] = 2 * (i + 1)
		}
	}
	per := max(1, s.ms*baud/10000)
	last := 0
	for i, o := range offs {
		if o-last >= per || i == len(offs)-1 {
			cuts = append(cuts, i+1)
			holds = append(holds, max(20, (o-last)*10000/baud))
			last = o
		}
	}
	return
}

// typeCursor is the typewriter's cursor on a cell: a block (or a line where there is no block).
func (a *App) typeCursor(v uint32) uint32 {
	switch {
	case a.c64(), a.atari():
		return v&^255 | 0xA0 // a reversed space
	case a.pc():
		return makeCell(219, 7, cellBg(v)&7, 0)
	case a.tt():
		return v&^0x7F | 0x7F // the solid block
	}
	return makeCell('_', 15&max(7, cellFg(v)), cellBg(v), 0)
}

// ---------------------------------------------------------------- applying

// applyOrder makes o the page's draw order (nil clears it), as an undo step.
func (a *App) applyOrder(o *drawOrder) {
	a.endPlay()
	if f := a.pageStreamFormat(); f == "" && o != nil {
		a.msg("A " + modeNames[a.mode] + " page is a picture of the screen, not a stream: make frames instead")
		return
	}
	if o != nil && a.order != nil && *o == *a.order {
		a.msg("The page already draws " + o.String())
		return
	}
	a.pushUndo()
	a.order = o
	if o == nil {
		a.msg("Draw order cleared: the page saves row by row again")
		return
	}
	ext := map[string]string{"ansi": ".ans", "seq": ".seq", "ata": ".ata", "vt52": ".vt"}[a.pageStreamFormat()]
	s := fmt.Sprintf("Draw order: %s. Saving as %s writes it, %s plays it (%s undoes it)", o, ext, keyName("play"), keyName("undo"))
	if screenRows := map[string]int{"c64": 25, "atari": 24, "st": a.res().rows}[a.mode]; a.doc.rows > max(25, screenRows) {
		s += fmt.Sprintf("; the page is %d rows: a terminal shows the order only on its first screen", a.doc.rows)
	}
	if a.nFrames() > 1 {
		s = fmt.Sprintf("Draw order %s kept, but the page has %d frames: those save one after another, row by row", o, a.nFrames())
	}
	a.msg(s)
}

// applyFrames turns the current frame into the frames of an effect, as one undo step.
func (a *App) applyFrames(s animSpec) {
	a.endPlay()
	if s.pat == patFadeIn || s.pat == patFadeOut {
		if why := a.fadeRefused(); why != "" {
			a.msg(why)
			return
		}
	}
	a.keepCase()
	other := a.frameCells() - a.doc.cols*a.doc.rows
	docs, holds, note := a.transitionFrames(a.doc, s, maxPageCells-other)
	if len(docs) < 2 {
		if note == "" {
			note = "Too little on the page for frames"
		}
		a.msg(note)
		return
	}
	a.pushFramesUndo(0)
	a.ensureFrames()
	base := a.frames[a.cur]
	nf := make([]Frame, len(docs))
	for i := range docs {
		nf[i] = Frame{doc: docs[i], hold: holds[i], lower: base.lower, src: -1}
	}
	first := a.cur
	a.frames = slices.Replace(a.frames, first, first+1, nf...)
	a.restructured = true
	a.cur = first
	if !s.erase && s.pat != patFadeOut {
		a.cur = first + len(docs) - 1 // the finished picture
	}
	a.doc = a.frames[a.cur].doc.clone()
	a.sel, a.float = nil, nil
	a.applyLook()
	what := patterns[s.pat].name
	if s.erase {
		what += " (erasing)"
	}
	msg := fmt.Sprintf("%s: %d frames, %d to %d (%s plays, %s undoes)", what, len(docs), first+1, first+len(docs), keyName("play"), keyName("undo"))
	if note != "" {
		msg += "; " + note
	}
	a.msg(msg)
}
