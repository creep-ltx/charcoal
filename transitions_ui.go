package main

// The Animate requester: a pattern (TheDraw's fourteen, then Charcoal's own),
// Reverse and Flip, and what to make: the saved file's draw order, or frames.
// A small preview plays the effect on the page's first screen.

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"
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
	length               Field  // a frame effect's frames
	seed                 int64  // a frame effect's random parts
	shown                int    // the pattern whose defaults the fields hold
	bbs, copies          string // BBS pauses: drawOrder.BBS and .Copies (bbspause.go)
	every, pauseMs       Field  // a draw order's pause (ms) every n cells
	prevKey              string
	prev                 []*Doc
	prevHolds            []int
	prevFB               []*FB
	prevAt               time.Time
	detail, shownDet     int                 // the detail level (animcost.go), and the one the fields hold
	costs                map[string]animCost // what each template costs on this page (animcost_ui.go)
	sp                   *speedPreview       // the preview as a caller sees it at the chosen speed
}

// newSeed is a fresh seed for a frame effect's random parts.
func newSeed() int64 { return 1 + rand.Int63n(1<<31-1) }

// dialogAnimates: the Animate requester is open (its preview plays).
func dialogAnimates() bool {
	_, ok := a.dialog.(*AnimReq)
	return ok || batchBusy() // Animate a folder moves on while drawn (batch_ui.go)
}

func openAnimate() {
	a.endPlay()
	if a.doc.cols*a.doc.rows > maxAnimCells {
		a.msg(fmt.Sprintf("The page is too large to animate (%dx%d)", a.doc.cols, a.doc.rows))
		return
	}
	d := &AnimReq{pat: patClock, cells: Field{max: 6}, ms: Field{max: 6}, steps: Field{max: 3}, rows: Field{max: 4}, hold: Field{max: 5},
		length: Field{max: 3}, seed: newSeed(), shown: -1, every: Field{max: 5}, pauseMs: Field{max: 5}, costs: map[string]animCost{}}
	d.detail = detailByID(prefs.AnimLevel)
	if prefs.AnimLevel == "" || d.detail < 0 {
		d.detail = detailMedium
	}
	d.cells.set("40")
	d.ms.set("100")
	d.steps.set("8")
	d.rows.set("1")
	d.hold.set("60")
	d.every.set("40")
	d.pauseMs.set("40")
	for _, f := range []*Field{&d.cells, &d.ms, &d.steps, &d.rows, &d.hold, &d.length, &d.every, &d.pauseMs} {
		f.enter = d.apply
	}
	if o := a.order; o != nil && o.pat() >= 0 {
		d.pat, d.reverse, d.flip, d.detail = o.pat(), o.Reverse, o.Flip, o.detail()
		if isEffect(d.pat) {
			d.syncDefaults()
			d.length.set(strconv.Itoa(effectLengthAt(d.pat, o.Frames, d.detail)))
			if o.Seed != 0 {
				d.seed = o.Seed
			}
		}
	}
	d.syncDefaults()
	if o := a.order; o != nil {
		d.bbs, d.copies = o.BBS, o.Copies
		every, ms := o.pacing()
		d.every.set(strconv.Itoa(every))
		d.pauseMs.set(strconv.Itoa(ms))
		if o.Hold > 0 && isEffect(d.pat) {
			d.hold.set(strconv.Itoa(o.Hold))
		}
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

// syncDefaults puts a frame effect's own length and hold (at the detail
// level) in the fields when it or the level is chosen.
func (d *AnimReq) syncDefaults() {
	if d.pat == d.shown && d.detail == d.shownDet {
		return
	}
	if isEffect(d.pat) {
		frames, hold := effectOwn(d.pat, d.detail)
		d.length.set(strconv.Itoa(frames))
		d.hold.set(strconv.Itoa(hold))
	} else if d.shown < 0 || isEffect(d.shown) {
		d.hold.set("60")
	}
	d.shown, d.shownDet = d.pat, d.detail
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
		hold: fieldInt(&d.hold, 60, 20, 99000), baud: a.baud, detail: d.detail}
	if !hasToggles(d.pat) {
		s.reverse, s.flip = false, false
	}
	switch {
	case isEffect(d.pat):
		own, _ := effectOwn(d.pat, d.detail)
		s.n = fieldInt(&d.length, own, 2, maxEffectFrames)
		s.seed = d.seed
	case d.pat == patFadeIn, d.pat == patFadeOut:
		s.n = fieldInt(&d.steps, 8, 2, 64)
	case d.pat == patScroll:
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
	if prefs.AnimLevel != detailIDs[d.detail] {
		prefs.AnimLevel = detailIDs[d.detail]
		savePrefs()
	}
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
	a.applyOrder(d.orderFor(d.pat))
}

// orderFor is the draw order Apply would give the page with pattern pat
// (the chosen one with the fields' frames and hold; another with its own).
func (d *AnimReq) orderFor(pat int) *drawOrder {
	s := d.spec()
	o := &drawOrder{Pattern: patterns[pat].id, Reverse: s.reverse, Flip: s.flip, BBS: d.bbs, Copies: d.copies, Detail: detailIDs[d.detail]}
	if !hasToggles(pat) {
		o.Reverse, o.Flip = false, false
	}
	if isEffect(pat) {
		o.Seed = s.seed
		if pat == d.pat {
			o.Frames = s.n
			if _, own := effectOwn(pat, d.detail); s.hold != own {
				o.Hold = s.hold
			}
		}
	} else if every, ms := fieldInt(&d.every, 40, 1, 1<<20), fieldInt(&d.pauseMs, 40, 20, 99000); every != 40 || ms != 40 {
		o.Every, o.Pause = every, ms
	}
	return o
}

// preview: the effect on the page's first screen, about 40 frames.
func (d *AnimReq) preview() []*Doc {
	s := d.spec()
	key := fmt.Sprint(d.pat, d.reverse, d.flip, d.erase, a.gen, a.cur, a.mode, a.machine, a.stres, a.doc.cols, a.doc.rows, s.seed, s.n, s.hold, s.detail)
	if key == d.prevKey {
		return d.prev
	}
	src := a.doc
	rows := min(src.rows, 50)
	c := newDoc(src.cols, rows)
	c.bg, c.border, c.ink, c.glyph0 = src.bg, src.border, src.ink, src.glyph0
	copy(c.cells, src.cells[:rows*src.cols])
	s.ms = 0
	if !isEffect(d.pat) {
		s.hold = 60
	}
	switch d.pat {
	case patMatrix, patDecrypt, patDrop, patAssemble, patShine, patShadeIn, patMelt, patPowerOn:
		s.n = min(s.n, 150)
	case patFadeIn, patFadeOut:
		s.n = 8
	case patScroll:
		s.n = max(1, rows/30)
	default:
		e := a.paperOf(c)
		n := len(a.cellsInOrderAt(c, e, d.pat, s.reverse, s.flip, s.detail))
		s.n = max(1, (n+39)/40)
	}
	docs, holds, _ := a.transitionFrames(c, s, 4<<20)
	if (d.pat == patFadeIn || d.pat == patFadeOut) && a.fadeRefused() != "" {
		docs = nil
	}
	if len(docs) == 0 {
		docs, holds = []*Doc{c}, []int{60}
	}
	d.prevKey, d.prev, d.prevHolds, d.prevFB, d.prevAt = key, docs, holds, make([]*FB, len(docs)), time.Now()
	return docs
}

// drawPreview plays the preview in r: each frame for its hold, the last one a while longer.
func (d *AnimReq) drawPreview(fb *FB, r Rect) {
	fb.fill(r, cDark)
	docs := d.preview()
	const rest = 1200
	total := rest
	for _, h := range d.prevHolds[:len(docs)-1] {
		total += h
	}
	t := int(time.Since(d.prevAt)/time.Millisecond) % total
	i := 0
	for i < len(docs)-1 && t >= d.prevHolds[i] {
		t -= d.prevHolds[i]
		i++
	}
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
	d.syncDefaults()
	in := u.window(centre(1000, 740), "Animate")
	file := !d.makesFrames()
	if file {
		d.fillCosts()
	}
	// The patterns: TheDraw's, and Charcoal's own beside them, each with its
	// weight on this page at this level when it is the saved file.
	ly := in.y
	listW := 150
	lx := in.x
	fb.text(lx, ly, "TheDraw's", cHi)
	ly += lineH + 4
	for i := 0; i < nPatterns; i++ {
		if i == nTheDraw {
			lx, ly = in.x+listW+8, in.y
			fb.text(lx, ly, "Charcoal's own", cHi)
			ly += lineH + 4
		}
		r := Rect{lx, ly, listW, lineH + 4}
		col := uint32(cText)
		if i == d.pat {
			fb.fill(r, cBlue)
			col = cHi
		}
		fb.text(r.x+8, r.y+2, patterns[i].name, col)
		hint := patterns[i].help
		if c, ok := d.cost(i, false); ok && file {
			w := c.weight()
			weightBars(fb, r.x+r.w-22, r.y+4, w)
			hint += fmt.Sprintf(" (%s here: %s, %s at %s)", weightNames[w], kbWords(c.size), durWords(c.at(a.baud)), speedName(a.baud))
		}
		u.setHint(r, hint)
		if u.take(r) != nil {
			d.pat = i
		}
		ly += lineH + 4
	}
	// Under the lists: the weights' legend and what the chosen one costs.
	cy := in.y + (nPatterns-nTheDraw+1)*(lineH+4) + 10
	d.drawCost(u, Rect{in.x, cy, 2*listW + 8, in.y + in.h - gadH - 12 - cy}, file)
	// The preview, and what the pattern does.
	px := in.x + 2*listW + 24
	pw := in.x + in.w - px
	pv := Rect{px, in.y, pw, 250}
	caption := d.drawPreviewAt(fb, pv, file)
	fb.frame(pv, cEdgeDk)
	y := pv.y + pv.h + 2
	fb.text(px, y, caption, cDim)
	y += lineH + 6
	help := patterns[d.pat].help
	if d.pat < nTheDraw {
		help = "TheDraw: " + help
	}
	for _, l := range wrap(help, pw) {
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
	// The detail level and the caller's speed.
	x = fb.text(px, y+3, "Detail", cText) + 8
	var levels []string
	cur := 0
	for i, l := range detailsUp {
		levels = append(levels, detailNames[l])
		if l == d.detail {
			cur = i
		}
	}
	u.choice("animdetail", Rect{x, y, choiceW(levels), gadH}, levels, cur,
		"Small: fewer frames and changes, a draw order in runs of cells (for modems); Large: the full look (D)",
		func(i int) { d.detail = detailsUp[i] })
	x += choiceW(levels) + 20
	x = fb.text(x, y+3, "Caller's speed", cText) + 8
	var speeds []string
	scur := 2
	for i, b := range bauds {
		speeds = append(speeds, speedName(b))
		if b == a.baud {
			scur = i
		}
	}
	u.choice("animspeed", Rect{x, y, choiceW(speeds), gadH}, speeds, scur,
		"The preview and Play ("+keyName("play")+") play at this speed, as a caller sees it (the speed in the bar)",
		func(i int) {
			a.baud = bauds[i]
			savePrefs()
		})
	y += gadH + 8
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
		if isEffect(d.pat) {
			items[0] = "the saved " + ext + ", frame by frame"
		}
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
			u.field(Rect{x, y, digitsW(chars) + 16, gadH}, f, hint)
			x += digitsW(chars) + 24
		}
		switch {
		case isEffect(d.pat):
			label("Frames")
			field(&d.length, 3, "How many frames the effect takes")
		case d.pat == patFadeIn, d.pat == patFadeOut:
			label("Steps")
			field(&d.steps, 3, "How many frames the fade takes")
		case d.pat == patScroll:
			label("Rows a frame")
			field(&d.rows, 4, "How many rows the picture moves up each frame")
		default:
			if a.pageStreamFormat() != "" {
				items := []string{"A frame every N cells", fmt.Sprintf("A frame every N ms at %d bps", lineBps(a.baud))}
				u.choice("animstep", Rect{x, y, choiceW(items), gadH}, items, map[bool]int{true: 1}[d.byTime],
					"Cut the frames by cells, or by the time the file takes over a modem (the caller's speed)",
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
		if !(d.byTime && a.pageStreamFormat() != "" && isScan(d.pat)) {
			label("Hold")
			field(&d.hold, 5, "How long each frame is shown, in ms (the finished picture stays at least 2 s)")
			label("ms")
		}
		if isEffect(d.pat) {
			d.shuffleButton(u, x, y)
		}
		y += gadH + 8
		fb.text(px, y, "The frames take the place of this frame; undo gives it back.", cDim)
		if b := boardByID(prefs.MyBBS); b != nil && b.shows(a.pageStreamFormat()) {
			y += lineH
			fb.text(px, y, "Saved as one stream, the holds become "+b.name+" pauses (Settings' My BBS).", cDim)
		}
	} else {
		if isEffect(d.pat) {
			fb.text(px, y+3, "Frames", cText)
			x := px + digitsW(7) + 8
			u.field(Rect{x, y, digitsW(3) + 16, gadH}, &d.length, "How many frames the effect takes")
			d.shuffleButton(u, x+digitsW(3)+24, y)
			fb.text(x+digitsW(3)+32+buttonW("Shuffle"), y+3, "The file draws each frame's changes only.", cDim)
			y += gadH + 8
		}
		y = d.drawPauses(u, px, y, pw)
		for _, l := range wrap(fmt.Sprintf("Play (%s) shows it at %s%s. Kept with the page until changed or cleared.", keyName("play"), speedName(a.baud),
			map[bool]string{true: ", with the board's pauses"}[d.board() != ""]), pw) {
			fb.text(px, y, l, cDim)
			y += lineH
		}
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

// shuffleButton: a new seed for the effect's random parts.
func (d *AnimReq) shuffleButton(u *UI, x, y int) {
	if u.button(Rect{x, y, buttonW("Shuffle"), gadH}, "Shuffle", false, "Other random glyphs, places and times (S)") != nil {
		d.seed = newSeed()
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
	case "S":
		if ui.focus != nil || !isEffect(d.pat) {
			return false
		}
		d.seed = newSeed()
	case "D":
		if ui.focus != nil {
			return false
		}
		d.detail = (d.detail + nDetails - 1) % nDetails // Large, Small, Medium, Large ...
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

// drawPauses: the BBS pauses part of the draw order's options (bbspause.go):
// the board (My BBS, none, or one for this page), the pace, and copies.
func (d *AnimReq) drawPauses(u *UI, x, y, w int) int {
	fb := u.fb
	f := a.pageStreamFormat()
	ext := map[string]string{"ansi": ".ans", "seq": ".seq", "ata": ".ata", "vt52": ".vt"}[f]
	boards := boardsFor(f)
	if len(boards) == 0 {
		fb.text(x, y, "BBS pauses: no board's codes known for "+ext+" files (the modem's speed is the pace).", cText)
		return y + lineH + 6
	}
	items := []string{"My BBS (" + boardName(prefs.MyBBS) + ")", "None for this page"}
	cur := 0
	if d.bbs == "none" {
		cur = 1
	}
	for i, b := range boards {
		items = append(items, b.name)
		if b.id == d.bbs {
			cur = i + 2
		}
	}
	lx := fb.text(x, y+3, "BBS pauses", cText) + 8
	u.choice("animbbs", Rect{lx, y, choiceW(items), gadH}, items, cur,
		"The board whose pause codes the saved file gets, so it plays at its pace over telnet too (My BBS is in Settings). None is the safe choice for art packs, 16colo.rs and several boards: another board's codes show as junk text.",
		func(i int) {
			switch i {
			case 0:
				d.bbs = ""
			case 1:
				d.bbs = "none"
			default:
				d.bbs = boards[i-2].id
			}
		})
	y += gadH + 6
	eff := prefs.MyBBS
	if d.bbs != "" {
		eff = map[bool]string{true: "", false: d.bbs}[d.bbs == "none"]
	}
	if b := boardByID(eff); b != nil && b.shows(f) || d.copies != "" {
		fx := x
		label := func(s string) { fx = fb.text(fx, y+3, s, cText) + 8 }
		field := func(fl *Field, chars int, hint string) {
			u.field(Rect{fx, y, digitsW(chars) + 16, gadH}, fl, hint)
			fx += digitsW(chars) + 24
		}
		if isEffect(d.pat) {
			label("Hold")
			field(&d.hold, 5, "How long the board waits after each frame, in ms")
			label("ms a frame")
		} else {
			label("A pause of")
			field(&d.pauseMs, 5, "How long the board waits each time, in ms")
			label("ms every")
			field(&d.every, 5, "Cells drawn between two pauses")
			label("cells")
		}
		y += gadH + 6
		if b != nil && b.shows(f) {
			for _, l := range wrap(b.name+": "+b.note+".", w) {
				fb.text(x, y, l, cDim)
				y += lineH
			}
		}
	}
	cx := fb.text(x, y+3, "Also save a copy for", cText) + 8
	for _, b := range boards {
		if cx+checkW(b.name) > x+w {
			cx, y = x+16, y+gadH+4
		}
		on := strings.Contains(","+d.copies+",", ","+b.id+",")
		o := drawOrder{Copies: d.copies}
		v, cw := u.check(cx, y, b.name, on, "Saving also writes name-"+b.id+ext+" with "+b.name+"'s codes beside the file")
		if v != on {
			o.toggleCopy(b.id)
			d.copies = o.Copies
		}
		cx += cw + 16
	}
	return y + gadH + 6
}
