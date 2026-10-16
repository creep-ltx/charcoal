package main

// The cost in the Animate requester: each template's weight on the page (marks
// in the lists), the chosen one's file size and play time at every speed, a
// warning past the time set in Settings, and the preview as a caller at the
// chosen speed sees it (the stream played through the page type's terminal,
// with the board's pauses).

import (
	"fmt"
	"time"
)

// costKey is what the costs depend on, besides the pattern.
func (d *AnimReq) costKey() string {
	return fmt.Sprint(a.gen, a.cur, a.mode, a.machine, a.stres, a.doc.cols, a.doc.rows, d.seed, d.detail, d.reverse, d.flip,
		d.board(), d.every.String(), d.pauseMs.String())
}

// board is the board whose codes the saved file gets ("" none).
func (d *AnimReq) board() string {
	id := d.bbs
	switch id {
	case "":
		id = prefs.MyBBS
	case "none":
		id = ""
	}
	if b := boardByID(id); b == nil || !b.shows(a.pageStreamFormat()) {
		return ""
	}
	return id
}

// maxMarkCells: on larger pages the lists show no weights (they would take too long).
const maxMarkCells = 60000

// cost is what pattern pat costs as the saved file (the chosen one with the
// fields' frames and hold); compute: work it out now if it is not known.
func (d *AnimReq) cost(pat int, compute bool) (animCost, bool) {
	f := a.pageStreamFormat()
	if f == "" || framesOnly(pat, false) || pat == patFadeIn && a.fadeRefused() != "" {
		return animCost{}, false
	}
	key := d.costKey() + fmt.Sprint(":", pat)
	if pat == d.pat {
		key += ":" + d.length.String() + ":" + d.hold.String()
	}
	if c, ok := d.costs[key]; ok {
		return c, true
	}
	if !compute {
		return animCost{}, false
	}
	if len(d.costs) > 400 {
		d.costs = map[string]animCost{}
	}
	c := a.costOf(a.animFormat(f), d.orderFor(pat), d.board()) // (a file in BBS colour codes: in them, brdcodes.go)
	d.costs[key] = c
	return c, true
}

// fillCosts works out the chosen pattern's cost, and one more of the lists'
// each time the requester is drawn (it is drawn all the time while open).
func (d *AnimReq) fillCosts() {
	d.cost(d.pat, true)
	if a.doc.cols*a.doc.rows > maxMarkCells {
		return
	}
	for p := 0; p < nPatterns; p++ {
		if _, ok := d.cost(p, false); !ok && !framesOnly(p, false) {
			d.cost(p, true)
			return
		}
	}
}

// weightBars: one, two or three bars (light, medium, heavy) at x, y.
func weightBars(fb *FB, x, y, w int) {
	for i := 0; i < 3; i++ {
		h := 4 + 3*i
		col := cFaint
		if i <= w {
			col = cText
			if w == weightHeavy {
				col = cOrange
			}
		}
		fb.fill(Rect{x + 6*i, y + 10 - h, 4, h}, col)
	}
}

// warnSign: a small warning triangle with its mark, 12 pixels wide.
func warnSign(fb *FB, x, y int) {
	for i := 0; i < 11; i++ {
		fb.fill(Rect{x + 6 - (i+1)/2, y + 2 + i, (i+1)/2*2 + 1, 1}, cOrange)
	}
	fb.fill(Rect{x + 6, y + 5, 1, 4}, cDark)
	fb.fill(Rect{x + 6, y + 10, 1, 1}, cDark)
}

// drawCost shows the legend of the weights and the chosen template's cost in r.
func (d *AnimReq) drawCost(u *UI, r Rect, file bool) {
	fb := u.fb
	old := fb.clip // a big interface font: what does not fit is cut, not drawn over the window's edge
	fb.clip = r.and(old)
	defer func() { fb.clip = old }()
	x, y := r.x, r.y
	if !file {
		for _, l := range wrap("Frames: the strip, Play, GIF and APNG. The saved file's cost shows when Make is the saved file.", r.w) {
			fb.text(x, y, l, cDim)
			y += lineH
		}
		return
	}
	for i, n := range weightNames {
		weightBars(fb, x, y+3, i)
		x = fb.text(x+22, y, n, cDim) + 12
	}
	y += lineH + 2
	fb.text(r.x, y, "on this page, at "+detailNames[d.detail], cDim)
	y += lineH + 8
	c, ok := d.cost(d.pat, true)
	if !ok {
		return
	}
	fb.text(r.x, y, "The saved file", cHi)
	y += lineH + 2
	row := func(l, v string, col uint32) {
		if y+lineH > r.y+r.h { // no room for the whole row: none of it
			return
		}
		fb.text(r.x, y, l, col)
		fb.text(r.x+digitsW(12), y, v, col)
		y += lineH
	}
	row("Size", kbWords(c.size), cText)
	row("Plain", kbWords(c.plain)+" (no animation)", cDim)
	if c.plain > 0 {
		row("", fmt.Sprintf("%.1fx the plain file, %s", c.ratio(), weightNames[c.weight()]), cDim)
	}
	if b := boardByID(d.board()); b != nil {
		row("Pauses", durWords(c.pauseMs)+" ("+b.name+")", cDim)
	}
	y += 4
	fb.text(r.x, y, "Plays in", cHi)
	y += lineH + 2
	for _, bps := range costSpeeds {
		col := uint32(cText)
		if bps == a.baud {
			col = cHi
		}
		row(speedLabel(bps), durWords(c.at(bps)), col)
	}
	if a.baud != telnet && !containsInt(costSpeeds, a.baud) {
		row(speedLabel(a.baud), durWords(c.at(a.baud)), cHi)
	}
	y += 6
	if ms := c.at(a.baud); ms > animWarnMs() {
		warnSign(fb, r.x, y+1)
		for i, l := range wrap(fmt.Sprintf("For fast connections: %s at %s", durWords(ms), speedName(a.baud)), r.w-16) {
			fb.text(r.x+16, y+i*lineH, l, cOrange)
		}
		u.setHint(Rect{r.x, y, r.w, 2 * lineH}, fmt.Sprintf("Longer than %d s at the caller's speed (Settings): a smaller detail level or a lighter template plays sooner", animWarnMs()/1000))
	}
}

// speedPreview plays the saved stream of the page's first screen through
// its terminal at the caller's speed, with the board's pauses.
type speedPreview struct {
	key              string
	bytes            []byte
	ps               []pause
	term             emulator
	pos, pi, fbPos   int
	last, wait, done time.Time
	fb               *FB
	keep             *Doc // the page the stream was written for (its paper colours)
}

// drawPreviewAt draws the preview in r and says what it shows: the saved
// file as a caller sees it at the speed, or the frames at their holds.
func (d *AnimReq) drawPreviewAt(fb *FB, r Rect, file bool) string {
	f := a.pageStreamFormat()
	if !file || f == "" || d.pat == patFadeIn && a.fadeRefused() != "" {
		d.sp = nil
		d.drawPreview(fb, r)
		return "The frames at their holds (the first 50 rows)"
	}
	sp := d.speedPreview()
	now := time.Now()
	sp.advance(now)
	if sp.fb == nil || sp.fbPos != sp.pos {
		lowerWas := a.lower()
		sp.fb = a.renderDoc(emDoc(sp.term), 1)
		if a.c64() {
			a.setCase(lowerWas)
		}
		sp.fbPos = sp.pos
	}
	fb.fill(r, cDark)
	blitScaled(fb, r.inset(4), sp.fb)
	what := "As a caller sees it at " + speedName(a.baud)
	if d.board() != "" {
		what += ", with " + boardName(d.board()) + "'s pauses"
	} else if a.baud == telnet {
		what += ": no pauses, it flashes past"
	}
	return what + " (the first 50 rows)"
}

// speedPreview is the preview's stream, made again when anything it shows changes.
func (d *AnimReq) speedPreview() *speedPreview {
	s := d.spec()
	key := fmt.Sprint(d.costKey(), d.pat, s.n, s.hold, a.baud)
	if d.sp != nil && d.sp.key == key {
		return d.sp
	}
	src := a.seen() // the layers put together (layers.go)
	rows := min(src.rows, 50)
	c := newDoc(src.cols, rows)
	c.bg, c.border, c.ink, c.glyph0 = src.bg, src.border, src.ink, src.glyph0
	copy(c.cells, src.cells[:rows*src.cols])
	f := a.pageStreamFormat()
	if limit := map[string]int{"seq": a.mach().cols, "ata": 40}[f]; limit > 0 && c.cols > limit {
		c = c.withCols(limit)
	}
	keep := a.order
	a.order = d.orderFor(d.pat)
	out, ps := a.writeOrderedPaced(f, c, false)
	a.order = keep
	if d.board() == "" {
		ps = nil
	}
	d.sp = &speedPreview{key: key, bytes: out, ps: ps, keep: c}
	d.sp.restart(time.Now())
	return d.sp
}

func (sp *speedPreview) restart(now time.Time) {
	c := sp.keep
	t := newDoc(c.cols, c.rows)
	t.bg, t.ink, t.border, t.glyph0 = c.bg, c.ink, c.border, c.glyph0
	var em emulator
	switch {
	case a.c64():
		em = newPetTermOn(t, a.ted())
	case a.atari():
		em = newAtariTerm(t)
	case a.st():
		em = newSTTerm(t, a.res())
		fillCells(t.cells, em.(*STTerm).blankCell())
	default:
		em = newTerm(t, a.amiga())
	}
	sp.term, sp.pos, sp.pi, sp.fb = em, 0, 0, nil
	sp.last, sp.wait, sp.done = now, time.Time{}, time.Time{}
}

// advance sends the bytes the line carries by now (none during a pause), as
// Play does; the finished picture rests a moment, then it starts again.
func (sp *speedPreview) advance(now time.Time) {
	if sp.pos >= len(sp.bytes) {
		if sp.done.IsZero() {
			sp.done = now
		}
		if now.Sub(sp.done) > 1500*time.Millisecond {
			sp.restart(now)
		}
		return
	}
	p := &Play{bytes: sp.bytes, pos: sp.pos, term: sp.term, last: sp.last, pauses: sp.ps, pi: sp.pi, wait: sp.wait}
	p.stepLine(now, a.baud)
	sp.pos, sp.last, sp.pi, sp.wait = p.pos, p.last, p.pi, p.wait
}
