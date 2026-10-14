package main

// The Animate requester: a pattern (TheDraw's fourteen, then Charcoal's own),
// Reverse and Flip, and what to make: the saved file's draw order, or frames.
// A small preview plays the effect on the page's first screen.

import (
	"fmt"
	"strconv"
	"time"
)

// maxAnimCells: larger pages are not animated (the orders and frames would take too long).
const maxAnimCells = 2 << 20

type AnimReq struct {
	pat                  int
	reverse, flip, erase bool
	frames               bool // make frames (else the draw order of the saved file)
	byTime               bool // a frame every ms at the modem's speed, not every n cells
	cells, ms, steps     Field
	rows, hold           Field
	prevKey              string
	prev                 []*Doc
	prevFB               []*FB
	prevAt               time.Time
}

// dialogAnimates: the Animate requester is open (its preview plays).
func dialogAnimates() bool {
	_, ok := a.dialog.(*AnimReq)
	return ok
}

func openAnimate() {
	a.endPlay()
	if a.doc.cols*a.doc.rows > maxAnimCells {
		a.msg(fmt.Sprintf("The page is too large to animate (%dx%d)", a.doc.cols, a.doc.rows))
		return
	}
	d := &AnimReq{pat: patClock, cells: Field{max: 6}, ms: Field{max: 6}, steps: Field{max: 3}, rows: Field{max: 4}, hold: Field{max: 5}}
	d.cells.set("40")
	d.ms.set("100")
	d.steps.set("8")
	d.rows.set("1")
	d.hold.set("60")
	for _, f := range []*Field{&d.cells, &d.ms, &d.steps, &d.rows, &d.hold} {
		f.enter = d.apply
	}
	if o := a.order; o != nil && o.pat() >= 0 {
		d.pat, d.reverse, d.flip = o.pat(), o.Reverse, o.Flip
	}
	if a.pageStreamFormat() == "" || a.order == nil && a.nFrames() > 1 {
		d.frames = true
	}
	a.dialog = d
}

// makesFrames: Apply makes frames (asked for, or the only thing this pattern and page can be).
func (d *AnimReq) makesFrames() bool {
	return d.frames || framesOnly(d.pat, d.erase) || a.pageStreamFormat() == ""
}

func fieldInt(f *Field, def, lo, hi int) int {
	v, err := strconv.Atoi(f.String())
	if err != nil {
		return def
	}
	return max(lo, min(hi, v))
}

func (d *AnimReq) spec() animSpec {
	s := animSpec{pat: d.pat, reverse: d.reverse, flip: d.flip, erase: d.erase && hasToggles(d.pat),
		hold: fieldInt(&d.hold, 60, 20, 99000), baud: a.baud}
	if !hasToggles(d.pat) {
		s.reverse, s.flip = false, false
	}
	switch d.pat {
	case patFadeIn, patFadeOut:
		s.n = fieldInt(&d.steps, 8, 2, 64)
	case patScroll:
		s.n = fieldInt(&d.rows, 1, 1, 9999)
	default:
		s.n = fieldInt(&d.cells, 40, 1, 1<<20)
		if d.byTime && a.pageStreamFormat() != "" {
			s.ms = fieldInt(&d.ms, 100, 10, 60000)
		}
	}
	return s
}

func (d *AnimReq) apply() {
	s := d.spec()
	closeDialog()
	if d.makesFrames() {
		a.applyFrames(s)
		return
	}
	if d.pat == patFadeIn {
		if why := a.fadeRefused(); why != "" {
			a.msg(why)
			return
		}
	}
	a.applyOrder(&drawOrder{Pattern: patterns[d.pat].id, Reverse: s.reverse, Flip: s.flip})
}

// preview: the effect on the page's first screen, about 40 frames.
func (d *AnimReq) preview() []*Doc {
	key := fmt.Sprint(d.pat, d.reverse, d.flip, d.erase, a.gen, a.cur, a.mode, a.machine, a.stres, a.doc.cols, a.doc.rows)
	if key == d.prevKey {
		return d.prev
	}
	src := a.doc
	rows := min(src.rows, 50)
	c := newDoc(src.cols, rows)
	c.bg, c.border, c.ink, c.glyph0 = src.bg, src.border, src.ink, src.glyph0
	copy(c.cells, src.cells[:rows*src.cols])
	s := d.spec()
	s.ms, s.hold = 0, 60
	switch d.pat {
	case patFadeIn, patFadeOut:
		s.n = 8
	case patScroll:
		s.n = max(1, rows/30)
	default:
		e := a.paperOf(c)
		n := len(a.cellsInOrder(c, e, d.pat, s.reverse, s.flip))
		s.n = max(1, (n+39)/40)
	}
	docs, _, _ := a.transitionFrames(c, s, 4<<20)
	if (d.pat == patFadeIn || d.pat == patFadeOut) && a.fadeRefused() != "" {
		docs = nil
	}
	if len(docs) == 0 {
		docs = []*Doc{c}
	}
	d.prevKey, d.prev, d.prevFB, d.prevAt = key, docs, make([]*FB, len(docs)), time.Now()
	return docs
}

// drawPreview plays the preview in r: each frame 70 ms, the last one a while longer.
func (d *AnimReq) drawPreview(fb *FB, r Rect) {
	fb.fill(r, cDark)
	docs := d.preview()
	const step, rest = 70, 1200
	total := len(docs)*step + rest
	t := int(time.Since(d.prevAt)/time.Millisecond) % total
	i := min(len(docs)-1, t/step)
	if d.prevFB[i] == nil {
		lowerWas := a.lower()
		d.prevFB[i] = a.renderDoc(docs[i], 1)
		if a.c64() {
			a.setCase(lowerWas)
		}
	}
	blitScaled(fb, r.inset(4), d.prevFB[i])
}

// blitScaled draws src into r as large as fits, its shape kept (nearest pixel).
func blitScaled(dst *FB, r Rect, src *FB) {
	if src.w == 0 || src.h == 0 || r.w <= 0 || r.h <= 0 {
		return
	}
	w, h := r.w, src.h*r.w/src.w
	if h > r.h {
		w, h = src.w*r.h/src.h, r.h
	}
	if w >= src.w && h >= src.h { // small enough: shown at its own size
		w, h = src.w, src.h
	}
	x0, y0 := r.x+(r.w-w)/2, r.y+(r.h-h)/2
	clip := dst.clip.and(r)
	for y := max(y0, clip.y); y < min(y0+h, clip.y+clip.h); y++ {
		sy := (y - y0) * src.h / h
		for x := max(x0, clip.x); x < min(x0+w, clip.x+clip.w); x++ {
			dst.px[y*dst.w+x] = src.px[sy*src.w+(x-x0)*src.w/w]
		}
	}
}

func (d *AnimReq) draw(u *UI) {
	fb := u.fb
	in := u.window(centre(820, 560), "Animate")
	// The patterns.
	ly := in.y
	listW := 180
	fb.text(in.x, ly, "TheDraw's patterns", cHi)
	ly += lineH + 4
	for i := 0; i < nPatterns; i++ {
		if i == nTheDraw {
			ly += 8
			fb.text(in.x, ly, "Charcoal's own", cHi)
			ly += lineH + 4
		}
		r := Rect{in.x, ly, listW, lineH + 4}
		col := uint32(cText)
		if i == d.pat {
			fb.fill(r, cBlue)
			col = cHi
		}
		fb.text(r.x+8, r.y+2, patterns[i].name, col)
		u.setHint(r, patterns[i].help)
		if u.take(r) != nil {
			d.pat = i
		}
		ly += lineH + 4
	}
	// The preview, and what the pattern does.
	px := in.x + listW + 16
	pw := in.x + in.w - px
	pv := Rect{px, in.y, pw, 270}
	d.drawPreview(fb, pv)
	fb.frame(pv, cEdgeDk)
	y := pv.y + pv.h + 8
	help := patterns[d.pat].help
	if d.pat < nTheDraw {
		help = "TheDraw: " + help
	}
	for _, l := range wrap(help, pw/charW) {
		fb.text(px, y, l, cText)
		y += lineH
	}
	y += 6
	// Toggles.
	x := px
	if hasToggles(d.pat) {
		var w int
		d.reverse, w = u.check(x, y, "Reverse", d.reverse, "The whole order backwards: the first cell drawn is the last (TheDraw's REVERSE)")
		x += w + 20
		d.flip, w = u.check(x, y, "Flip", d.flip, "Rows mirrored top to bottom: a scan from the top starts at the bottom (TheDraw's FLIP)")
		x += w + 20
		d.erase, _ = u.check(x, y, "Erase", d.erase, "Frames: the picture goes away in this order instead of coming (TheDraw's Fadeout)")
		y += gadH + 8
	}
	// The result.
	fb.text(px, y+3, "Make", cText)
	if a.pageStreamFormat() == "" || framesOnly(d.pat, d.erase && hasToggles(d.pat)) {
		why := "frames (this one is no file's draw order)"
		if a.pageStreamFormat() == "" {
			why = "frames (a " + modeNames[a.mode] + " page is no stream to draw in order)"
		}
		fb.text(px+48, y+3, why, cHi)
	} else {
		ext := map[string]string{"ansi": ".ans", "seq": ".seq", "ata": ".ata", "vt52": ".vt"}[a.pageStreamFormat()]
		items := []string{"the draw order of the saved " + ext, "frames"}
		cur := map[bool]int{true: 1}[d.frames]
		u.choice("animresult", Rect{px + 48, y, choiceW(items), gadH}, items, cur,
			"Draw order: saving as "+ext+" draws the picture in this order (any terminal plays it); frames: the strip, Play, GIF and APNG",
			func(i int) { d.frames = i == 1 })
	}
	y += gadH + 8
	if d.makesFrames() {
		x := px
		label := func(s string) { x = fb.text(x, y+3, s, cText) + 8 }
		field := func(f *Field, chars int, hint string) {
			u.field(Rect{x, y, chars*charW + 16, gadH}, f, hint)
			x += chars*charW + 24
		}
		switch d.pat {
		case patFadeIn, patFadeOut:
			label("Steps")
			field(&d.steps, 3, "How many frames the fade takes")
		case patScroll:
			label("Rows a frame")
			field(&d.rows, 4, "How many rows the picture moves up each frame")
		default:
			if a.pageStreamFormat() != "" {
				items := []string{"A frame every N cells", fmt.Sprintf("A frame every N ms at %d bps", a.baud)}
				u.choice("animstep", Rect{x, y, choiceW(items), gadH}, items, map[bool]int{true: 1}[d.byTime],
					"Cut the frames by cells, or by the time the file takes over a modem (the speed in the bar)",
					func(i int) { d.byTime = i == 1 })
				x += choiceW(items) + 8
			} else {
				label("A frame every")
			}
			if d.byTime && a.pageStreamFormat() != "" {
				label("N")
				field(&d.ms, 6, "Milliseconds of modem time a frame")
			} else {
				label("N")
				field(&d.cells, 6, "Cells drawn a frame")
			}
		}
		if !(d.byTime && a.pageStreamFormat() != "" && d.pat != patFadeIn && d.pat != patFadeOut && d.pat != patScroll) {
			label("Hold")
			field(&d.hold, 5, "How long each frame is shown, in ms (the finished picture stays at least 2 s)")
			label("ms")
		}
		y += gadH + 8
		fb.text(px, y, "The frames take the place of this frame; undo gives it back.", cText)
	} else {
		fb.text(px, y, fmt.Sprintf("Play (%s) shows it at %d bps, the speed in the bar.", keyName("play"), a.baud), cText)
		y += lineH
		fb.text(px, y, "Kept with the page until changed or cleared.", cText)
	}
	labels := []string{"Cancel", "Apply"}
	if a.order != nil {
		labels = []string{"Clear draw order", "Cancel", "Apply"}
	}
	switch hit := buttonRow(u, in, labels...); {
	case hit < 0:
	case labels[hit] == "Apply":
		d.apply()
	case labels[hit] == "Cancel":
		closeDialog()
	default:
		closeDialog()
		a.applyOrder(nil)
	}
}

func (d *AnimReq) key(k string, ctrl, shift bool) bool {
	switch k {
	case "Escape":
		closeDialog()
	case "Return", "Keypad Enter":
		d.apply()
	case "Up":
		d.pat = (d.pat + nPatterns - 1) % nPatterns
	case "Down":
		d.pat = (d.pat + 1) % nPatterns
	case "Tab":
		ui.focus = nil
	case "R", "F", "E":
		if ui.focus != nil || !hasToggles(d.pat) {
			return false
		}
		switch k {
		case "R":
			d.reverse = !d.reverse
		case "F":
			d.flip = !d.flip
		default:
			d.erase = !d.erase
		}
	default:
		return false
	}
	return true
}
