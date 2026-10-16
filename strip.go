package main

// The frame strip (a row under the page while it has several frames) and
// the onion skin (the frame before, faint, under the empty cells).

import (
	"fmt"
	"strconv"
)

// stripShown: the strip is on show (Ctrl+Shift+F switches it, else it is
// there while the page has several frames).
func (a *App) stripShown() bool {
	return !a.bare && (a.nFrames() > 1 && a.strip != -1 || a.strip == 1)
}

func (a *App) toggleStrip() {
	if a.stripShown() {
		a.strip = -1
	} else {
		a.strip = 1
	}
}

func (a *App) toggleOnion() {
	a.onion = !a.onion
	switch {
	case !a.onion:
		a.msg("Onion skin off")
	case a.nFrames() < 2:
		a.msg("Onion skin on: it shows the frame before, faint, once there are two (" + keyName("frame_new") + " makes one)")
	default:
		a.msg("Onion skin on: the frame before shows faint under the empty cells")
	}
}

var fieldHold = &Field{max: 5}

// stripBox is where a frame's box was drawn, for dropping a dragged frame.
type stripBox struct {
	r Rect
	i int
}

var (
	stripBoxes []stripBox
	stripDrag  = -1 // the frame being dragged along the strip
)

func init() {
	fieldHold.enter = func() {
		if n, err := strconv.Atoi(fieldHold.String()); err == nil {
			if a.tt() {
				n *= 1000
			}
			a.setHold(n)
		}
		fieldHold.set(a.holdField())
		ui.focus = nil
	}
}

// holdField is the hold time as the strip's field shows it: seconds on teletext, else ms.
func (a *App) holdField() string {
	h := a.holdOf(a.cur)
	if a.tt() {
		return strconv.Itoa((h + 500) / 1000)
	}
	return strconv.Itoa(h)
}

// moveFrameTo moves the current frame to place j, as one undo step.
func (a *App) moveFrameTo(j int) {
	if a.frames == nil || j < 0 || j >= len(a.frames) || j == a.cur {
		return
	}
	a.endPlay()
	a.pushFramesUndo(0)
	a.keepCase()
	a.frames[a.cur].doc = a.doc
	f := a.frames[a.cur]
	a.frames = append(a.frames[:a.cur], a.frames[a.cur+1:]...)
	a.frames = append(a.frames[:j], append([]Frame{f}, a.frames[j:]...)...)
	a.cur = j
	a.restructured = true
	a.frameMsg()
}

// stripRelease drops a frame dragged along the strip where the mouse let go.
func (a *App) stripRelease() {
	from := stripDrag
	stripDrag = -1
	if from < 0 || from != a.cur {
		return
	}
	for _, b := range stripBoxes {
		if b.r.has(ui.mx, ui.my) && b.i != from {
			a.moveFrameTo(b.i)
			return
		}
	}
}

// drawStrip draws the frame strip in r: the frames' numbered boxes on the
// left, the hold time and the frame buttons on the right.
func (a *App) drawStrip(u *UI, r Rect) {
	fb := u.fb
	fb.fill(r, cDark)
	fb.fill(Rect{r.x, r.y, r.w, 1}, cEdgeDk)
	y, h := r.y+2, r.h-3
	n := a.nFrames()

	// The right group first, so the boxes know their room.
	type btn struct {
		label, hint string
		on          bool
		do          func()
	}
	btns := []btn{
		{"New", "New empty frame after this one (" + keyName("frame_new") + ")", false, a.newFrame},
		{"Copy", "Copy this frame: a new frame after it (" + keyName("frame_duplicate") + ")", false, a.dupFrame},
		{"Delete", "Delete this frame (" + keyName("frame_delete") + ")", false, a.deleteFrame},
		{"Onion", "Onion skin: the frame before, faint, under the empty cells (" + keyName("onion") + ")", a.onion, a.toggleOnion},
		{"Play", "Play the frames, each for its hold time (" + keyName("play") + "; again or Esc stops)", a.playing != nil && a.playing.frames != nil, a.playKey},
		{"Loop", "Playing starts again after the last frame", !a.noLoop, func() { a.noLoop = !a.noLoop }},
	}
	if a.playing != nil && a.playing.frames != nil {
		btns[4].label = "Stop"
	}
	w := choiceW(playSpeedNames) + 4
	for _, b := range btns {
		w += max(buttonW(b.label), buttonW("Stop")) + 4
	}
	unit := "ms"
	if a.tt() {
		unit = "s"
	}
	fieldW := digitsW(7)
	w += textW("Hold") + 6 + fieldW + 4 + textW(unit) + 12
	x := r.x + r.w - w
	fb.text(x, y+3, "Hold", cText)
	x += textW("Hold") + 6
	if u.focus != fieldHold {
		fieldHold.set(a.holdField())
	}
	u.field(Rect{x, y, fieldW, h}, fieldHold, fmt.Sprintf("How long frame %d is shown when played, in %s (Enter sets it; %s / %s step it)",
		a.cur+1, map[bool]string{true: "seconds", false: "milliseconds"}[a.tt()], keyName("frame_hold_less"), keyName("frame_hold_more")))
	x += fieldW + 4
	x = fb.text(x, y+3, unit, cText) + 12
	for _, b := range btns {
		bw := buttonW(b.label)
		if b.label == "Play" || b.label == "Stop" {
			bw = buttonW("Stop")
		}
		if u.button(Rect{x, y, bw, h}, b.label, b.on, b.hint) != nil {
			b.do()
		}
		x += bw + 4
	}
	u.choice("playspeed", Rect{x, y, choiceW(playSpeedNames), h}, playSpeedNames, a.playSpeed, "How fast the frames play", func(i int) {
		a.playSpeed = i
	})
	right := r.x + r.w - w - 8

	// A stream that shows several pictures: offered as frames.
	if n == 1 && (a.split.clears >= 2 || a.split.homes >= 2) && !a.dirty {
		x = r.x + 6
		if a.split.clears >= 2 {
			l := fmt.Sprintf("Split: %d pictures (clear screen)", a.split.clears)
			if u.button(Rect{x, y, buttonW(l), h}, l, false, "Make a frame of each picture the file shows, cut where it clears the screen") != nil {
				a.splitFrames(false)
			}
			x += buttonW(l) + 4
		}
		if a.split.homes >= 2 {
			l := fmt.Sprintf("Split at cursor home (%d)", a.split.homes)
			if u.button(Rect{x, y, buttonW(l), h}, l, false, "Cut where the cursor goes home too (some animations draw over the last picture)") != nil {
				a.splitFrames(true)
			}
		}
		return
	}

	// The boxes: as many as fit, the current one always on show.
	boxW := func(i int) int { return textW(strconv.Itoa(i+1)) + 12 }
	room := right - r.x - 8 - 2*20
	first := 0
	for {
		used := 0
		for i := first; i <= a.cur && i < n; i++ {
			used += boxW(i) + 2
		}
		if used <= room || first >= a.cur {
			break
		}
		first++
	}
	x = r.x + 6
	if first > 0 {
		if u.button(Rect{x, y, 18, h}, "<", false, "More frames before ("+keyName("frame_previous")+")") != nil {
			a.goFrame(a.cur - 1)
		}
		x += 20
	}
	stripBoxes = stripBoxes[:0]
	last := n - 1
	for i := first; i < n; i++ {
		bw := boxW(i)
		if i > first && x+bw > r.x+6+room+20*min(1, first) {
			last = i - 1
			break
		}
		br := Rect{x, y, bw, h}
		bg, col := uint32(cGrey), uint32(cText)
		on := i == a.cur
		if a.playing != nil && a.playing.frames != nil {
			on = i == a.playing.fi // the frame being played
		}
		if on {
			bg, col = cBlue, cHi
		}
		if stripDrag >= 0 && i != stripDrag && br.has(u.mx, u.my) {
			bg = cEdgeLt // where a dragged frame would go
		}
		fb.fill(br, bg)
		fb.text(br.x+6, br.y+(h-lineH)/2, strconv.Itoa(i+1), col)
		hint := fmt.Sprintf("Frame %d of %d, %s", i+1, n, holdText(a.mode, a.holdOf(i)))
		if a.frames != nil && a.frames[i].name != "" {
			hint += ", " + a.frames[i].name
		}
		u.setHint(br, hint+" (drag it to move it)")
		stripBoxes = append(stripBoxes, stripBox{br, i})
		if u.take(br) != nil {
			a.goFrame(i)
			stripDrag, u.drag = i, "frame"
		}
		x += bw + 2
	}
	if last < n-1 {
		if u.button(Rect{x, y, 18, h}, ">", false, "More frames after ("+keyName("frame_next")+")") != nil {
			a.goFrame(a.cur + 1)
		}
	}
}

// onionBlank: an empty cell, where the frame before shows through. Spectrum
// and ST cells carry their paper colour even when empty.
func (a *App) onionBlank(v uint32) bool {
	if a.zx() || a.st() || a.cpc() {
		ch := v & 255
		return ch == 32 || ch == 0
	}
	if a.tt() {
		return v&0x7F == 0x20
	}
	return a.doc.blankAt(v)
}

// drawOnion draws the frame before, faint, in the empty cells of rows y0-y1
// and columns x0-x1 of the page drawn at (ox, oy).
func (a *App) drawOnion(fb *FB, ox, oy, x0, x1, y0, y1 int) {
	if !a.onion || a.frames == nil || a.cur == 0 || a.playing != nil {
		return
	}
	prev := a.frames[a.cur-1].doc
	z := a.zoom
	rowPx, colPx := a.cellH()*z, a.cellW()*z
	for y := y0; y < y1 && y < prev.rows; y++ {
		var row []ttCell
		if a.tt() {
			row = ttDecode(prev, y, a.ttBlack())
		}
		for x := x0; x < x1 && x < prev.cols; x++ {
			v := a.doc.get(x, y)
			if !a.onionBlank(v) {
				continue
			}
			pv := prev.get(x, y)
			if a.onionBlank(pv) && (!a.zx() && !a.st() && !a.cpc() || cellBg(pv) == cellBg(v)) {
				continue
			}
			px, py := ox+x*colPx, oy+y*rowPx
			paper := uint32(cPaper)
			switch {
			case a.screen():
				paper = a.screenRGB(a.doc.bg)
			case a.zx() || a.st() || a.cpc():
				paper = a.pal[cellBg(v)&15]
			}
			if a.tt() {
				if row[x].code == 0x20 && row[x].bg == 0 {
					continue
				}
				a.drawTTCell(fb, px, py, z, row[x], false)
			} else {
				a.drawCell(fb, px, py, z, pv)
			}
			fb.blend(Rect{px, py, colPx, rowPx}, paper, 190)
		}
	}
}
