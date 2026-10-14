package main

// Immediate-mode gadgets in the Workbench 3 look. Every pass lays out and
// draws the whole window; a gadget that sees the pending click takes it.

import (
	"strings"
)

type Click struct {
	x, y, button int // 1 left, 2 middle, 3 right
	shift        bool
}

type UI struct {
	fb         *FB
	mx, my     int
	click      *Click // the mouse press of this pass, nil once taken
	inert      bool   // a requester is open: the window behind takes no clicks
	hint       string // what the gadget under the mouse does
	drag       string // the gadget holding the mouse (scrollbars, sliders)
	popup      *Popup
	focus      *Field
	wheel      int // wheel steps of this pass (positive = down)
	wheelTaken bool
	menuBar    Rect   // where the menu titles are
	closedMenu string // the menu this pass's click closed: its title does not open it again
}

// hover and take see only what is inside the clip: a gadget scrolled out of a panel takes nothing.
func (u *UI) hover(r Rect) bool { return !u.inert && r.has(u.mx, u.my) && u.fb.clip.has(u.mx, u.my) }

// take returns the pending click if it falls in r, and consumes it.
func (u *UI) take(r Rect) *Click {
	if u.inert || u.click == nil || !r.has(u.click.x, u.click.y) || !u.fb.clip.has(u.click.x, u.click.y) {
		return nil
	}
	c := u.click
	u.click = nil
	return c
}

// takeWheel returns the wheel steps if the mouse is over r.
func (u *UI) takeWheel(r Rect) int {
	if u.wheel == 0 || u.wheelTaken || !u.hover(r) {
		return 0
	}
	u.wheelTaken = true
	return u.wheel
}

func (u *UI) setHint(r Rect, h string) {
	if h != "" && u.hover(r) {
		u.hint = h
	}
}

// button: a raised box with a label. on draws it pressed in (a selected tool).
func (u *UI) button(r Rect, label string, on bool, hint string) *Click {
	f := u.fb
	bg := uint32(cGrey)
	if on {
		bg = cBlue
	}
	f.fill(r, bg)
	f.bevel(r, on)
	f.text(r.x+(r.w-textW(label))/2, r.y+(r.h-lineH)/2, label, cText)
	u.setHint(r, hint)
	return u.take(r)
}

func buttonW(label string) int { return textW(label) + 16 }

// check: an Amiga checkbox with a label after it.
func (u *UI) check(x, y int, label string, on bool, hint string) (bool, int) {
	f := u.fb
	box := Rect{x, y + 1, 26, lineH + 4}
	f.fill(box, cGrey)
	f.bevel(box, false)
	if on {
		// the tick
		for _, p := range [][2]int{{7, 10}, {8, 11}, {9, 12}, {10, 13}, {11, 12}, {12, 11}, {13, 10}, {14, 9}, {15, 8}, {16, 7}, {17, 6}, {18, 5}} {
			f.fill(Rect{x + p[0], y + 1 + p[1], 2, 2}, cHi)
		}
	}
	w := 26 + 6 + textW(label)
	f.text(x+32, y+3, label, cText)
	r := Rect{x, y, w, lineH + 6}
	u.setHint(r, hint)
	if u.take(r) != nil {
		return !on, w
	}
	return on, w
}

func checkW(label string) int { return 32 + textW(label) }

// ---------------------------------------------------------------- pop-up lists

type Popup struct {
	id    string
	r     Rect
	items []string
	cur   int
	pick  func(int)
	hot   int // the item the mouse or the arrow keys are on, -1: none

	// A menu from the menu bar: each item's keys and tick, "" items are the
	// lines between groups, nav opens the menu to the left or right.
	menu  bool
	keys  []string
	ticks []bool
	nav   func(d int)
	mouse [2]int // where the mouse was: the arrow keys win until it moves
}

func (p *Popup) rowH(i int) int {
	switch {
	case !p.menu:
		return lineH + 2
	case p.items[i] == "":
		return 7
	}
	return lineH + 6
}

// row is item i's place in the list.
func (p *Popup) row(i int) Rect {
	y := p.r.y + 2
	for j := range i {
		y += p.rowH(j)
	}
	return Rect{p.r.x + 2, y, p.r.w - 4, p.rowH(i)}
}

func (p *Popup) height() int {
	h := 4
	for i := range p.items {
		h += p.rowH(i)
	}
	return h
}

// at is the item at (x, y), -1 for none or a line between groups.
func (p *Popup) at(x, y int) int {
	for i := range p.items {
		if p.row(i).has(x, y) && p.items[i] != "" {
			return i
		}
	}
	return -1
}

// step moves the hot item by d, over the lines between groups.
func (p *Popup) step(d int) {
	n := len(p.items)
	i := p.hot
	if i < 0 && d < 0 {
		i = n
	}
	for range n {
		i = (i + d + n) % n
		if p.items[i] != "" {
			p.hot = i
			return
		}
	}
}

// choice: a gadget showing the current item; a click opens the list.
func (u *UI) choice(id string, r Rect, items []string, cur int, hint string, pick func(int)) {
	f := u.fb
	f.fill(r, cGrey)
	f.bevel(r, false)
	label := ""
	if cur >= 0 && cur < len(items) {
		label = items[cur]
	}
	// The cycle mark, then the item.
	f.text(r.x+6, r.y+(r.h-lineH)/2, "\u00bb", cText)
	f.fill(Rect{r.x + 20, r.y + 3, 1, r.h - 6}, cDark)
	f.fill(Rect{r.x + 21, r.y + 3, 1, r.h - 6}, cEdgeLt)
	f.text(r.x+28, r.y+(r.h-lineH)/2, label, cText)
	u.setHint(r, hint)
	if c := u.take(r); c != nil {
		if c.button == 3 { // right click: the next one, like a cycle gadget
			pick((cur + 1) % len(items))
			return
		}
		w := r.w
		for _, it := range items {
			w = max(w, textW(it)+40)
		}
		h := len(items)*(lineH+2) + 4
		y := r.y + r.h
		if y+h > u.fb.h {
			y = max(0, r.y-h)
		}
		u.popup = &Popup{id: id, r: Rect{r.x, y, w, h}, items: items, cur: cur, pick: pick, hot: -1}
	}
}

func choiceW(items []string) int {
	w := 0
	for _, it := range items {
		w = max(w, textW(it))
	}
	return w + 40
}

// drawPopup draws the open list over everything and takes clicks first.
func (u *UI) drawPopup() {
	p := u.popup
	if p == nil {
		return
	}
	f := u.fb
	if m := [2]int{u.mx, u.my}; m != p.mouse { // the mouse moved: it picks the hot item
		p.mouse = m
		if i := p.at(u.mx, u.my); i >= 0 || p.r.has(u.mx, u.my) {
			p.hot = i
		}
	}
	f.fill(p.r, cGrey)
	f.frame(p.r, cEdgeDk)
	for i, it := range p.items {
		ir := p.row(i)
		if it == "" {
			f.fill(Rect{ir.x + 6, ir.y + ir.h/2, ir.w - 12, 1}, cFaint)
			continue
		}
		col := uint32(cText)
		if i == p.hot {
			f.fill(ir, cBlue)
			col = cHi
		}
		ty := ir.y + (ir.h-lineH)/2
		if !p.menu {
			mark := "  "
			if i == p.cur {
				mark = "\u00bb "
			}
			f.text(ir.x+4, ty, mark+it, col)
			continue
		}
		if p.ticks[i] {
			tickMark(f, ir.x+8, ir.y+ir.h/2, col)
		}
		f.text(ir.x+28, ty, it, col)
		if k := p.keys[i]; k != "" {
			kc := uint32(cDim)
			if i == p.hot {
				kc = cText
			}
			f.text(ir.x+ir.w-10-textW(k), ty, k, kc)
		}
	}
}

// tickMark draws a small tick with its middle-left at (x, y).
func tickMark(f *FB, x, y int, c uint32) {
	for i := range 4 { // down to the bottom
		f.fill(Rect{x + i, y - 1 + i, 2, 2}, c)
	}
	for i := range 7 { // and up to the right
		f.fill(Rect{x + 4 + i, y + 2 - i, 2, 2}, c)
	}
}

// popupClick handles a press while the list is open.
func (u *UI) popupClick(c *Click) {
	p := u.popup
	if p.r.has(c.x, c.y) {
		i := p.at(c.x, c.y)
		if i < 0 {
			return // a line between groups: the menu stays
		}
		u.popup = nil
		p.pick(i)
		return
	}
	u.popup = nil
}

// popupKey handles a key while the list is open: arrows move, Return picks, Esc closes.
func (u *UI) popupKey(k string) {
	p := u.popup
	switch k {
	case "Escape":
		u.popup = nil
	case "Up":
		p.step(-1)
	case "Down":
		p.step(1)
	case "Left", "Right":
		if p.nav != nil {
			p.nav(map[string]int{"Left": -1, "Right": 1}[k])
		}
	case "Return", "Keypad Enter":
		if p.hot >= 0 {
			u.popup = nil
			p.pick(p.hot)
		}
	}
}

// ---------------------------------------------------------------- text fields

type Field struct {
	text   []rune
	cur    int
	max    int
	scroll int
	enter  func()
}

func (fl *Field) set(s string) {
	fl.text = []rune(s)
	fl.cur = len(fl.text)
	fl.scroll = 0
}
func (fl *Field) String() string { return string(fl.text) }

func (fl *Field) insert(s string) {
	for _, r := range s {
		if r < 32 || r == 127 || (fl.max > 0 && len(fl.text) >= fl.max) {
			continue
		}
		fl.text = append(fl.text[:fl.cur], append([]rune{r}, fl.text[fl.cur:]...)...)
		fl.cur++
	}
}

// key handles an editing key; it returns false for keys it does not use.
func (fl *Field) key(k string, ctrl bool) bool {
	switch k {
	case "Left":
		fl.cur = max(0, fl.cur-1)
	case "Right":
		fl.cur = min(len(fl.text), fl.cur+1)
	case "Home":
		fl.cur = 0
	case "End":
		fl.cur = len(fl.text)
	case "Backspace":
		if ctrl {
			fl.text, fl.cur = fl.text[fl.cur:], 0
		} else if fl.cur > 0 {
			fl.text = append(fl.text[:fl.cur-1], fl.text[fl.cur:]...)
			fl.cur--
		}
	case "Delete":
		if fl.cur < len(fl.text) {
			fl.text = append(fl.text[:fl.cur], fl.text[fl.cur+1:]...)
		}
	case "U", "X":
		if !ctrl {
			return false
		}
		fl.text, fl.cur, fl.scroll = nil, 0, 0
	case "A", "E":
		if !ctrl {
			return false
		}
		fl.cur = map[string]int{"A": 0, "E": len(fl.text)}[k]
	case "Return", "Keypad Enter":
		if fl.enter != nil {
			fl.enter()
		}
	default:
		return false
	}
	return true
}

// field draws a string gadget; a click focuses it and places the cursor.
func (u *UI) field(r Rect, fl *Field, hint string) {
	f := u.fb
	f.fill(r, cGrey)
	f.bevel(r, true)
	f.bevel(r.inset(2), false)
	in := r.inset(4)
	f.fill(in, cGrey)
	// What shows is measured in pixels: the cursor stays in view, with room for its block.
	avail := in.w - 4
	curW := textW(" ")
	if fl.cur < len(fl.text) {
		curW = textW(string(fl.text[fl.cur]))
	}
	if fl.cur < fl.scroll {
		fl.scroll = fl.cur
	}
	fl.scroll = max(0, min(fl.scroll, len(fl.text)))
	for fl.scroll < fl.cur && textW(string(fl.text[fl.scroll:fl.cur]))+curW > avail {
		fl.scroll++
	}
	end := fl.scroll + fitText(fl.text[fl.scroll:], avail)
	ty := in.y + (in.h-lineH)/2
	f.text(in.x+2, ty, string(fl.text[fl.scroll:end]), cText)
	if u.focus == fl {
		cx := in.x + 2 + textW(string(fl.text[fl.scroll:fl.cur]))
		f.fill(Rect{cx, ty, curW, lineH}, cOrange)
		if fl.cur < len(fl.text) {
			f.text(cx, ty, string(fl.text[fl.cur]), cText)
		}
	}
	u.setHint(r, hint)
	if c := u.take(r); c != nil {
		u.focus = fl
		fl.cur = fl.scroll + fitText(fl.text[fl.scroll:], c.x-in.x-2)
	}
}

// ---------------------------------------------------------------- scrollbars and sliders

// scrollbar draws a proportional bar; pos is the first visible unit of total, page units are shown.
// It returns the new position.
func (u *UI) scrollbar(id string, r Rect, vertical bool, pos, page, total int) int {
	f := u.fb
	f.fill(r, cGrey)
	f.bevel(r, true)
	track := r.inset(2)
	length := track.h
	if !vertical {
		length = track.w
	}
	if total <= page || total <= 0 {
		total = max(page, 1)
	}
	tl := max(12, length*page/total)
	tl = min(tl, length)
	maxPos := max(0, total-page)
	off := 0
	if maxPos > 0 {
		off = (length - tl) * pos / maxPos
	}
	var knob Rect
	if vertical {
		knob = Rect{track.x, track.y + off, track.w, tl}
	} else {
		knob = Rect{track.x + off, track.y, tl, track.h}
	}
	f.fill(knob, cGrey)
	f.bevel(knob, false)
	if c := u.take(r); c != nil {
		u.drag = id
		if !knob.has(c.x, c.y) { // a click in the track jumps a page
			if (vertical && c.y < knob.y) || (!vertical && c.x < knob.x) {
				pos -= page
			} else {
				pos += page
			}
			u.drag = ""
		} else {
			dragGrab = c.y - knob.y
			if !vertical {
				dragGrab = c.x - knob.x
			}
		}
	}
	if u.drag == id && length > tl && maxPos > 0 {
		m := u.my - track.y - dragGrab
		if !vertical {
			m = u.mx - track.x - dragGrab
		}
		pos = m * maxPos / (length - tl)
	}
	return max(0, min(pos, maxPos))
}

var dragGrab int

// slider: a horizontal 0-255 slider for the colour requester.
func (u *UI) slider(id string, r Rect, v int) int {
	f := u.fb
	f.fill(r, cGrey)
	f.bevel(r, true)
	track := r.inset(2)
	kw := 16
	off := (track.w - kw) * v / 255
	knob := Rect{track.x + off, track.y, kw, track.h}
	f.fill(knob, cGrey)
	f.bevel(knob, false)
	if c := u.take(r); c != nil {
		u.drag = id
	}
	if u.drag == id {
		v = (u.mx - track.x - kw/2) * 255 / max(1, track.w-kw)
	}
	return max(0, min(255, v))
}

// ---------------------------------------------------------------- windows

// window draws a requester frame with a title bar and returns the inside.
func (u *UI) window(r Rect, title string) Rect {
	f := u.fb
	f.fill(r, cGrey)
	f.frame(r, cEdgeDk)
	f.bevel(r.inset(1), false)
	tb := Rect{r.x + 2, r.y + 2, r.w - 4, lineH + 4}
	f.fill(tb, cBlue)
	f.text(tb.x+6, tb.y+2, title, cText)
	return Rect{r.x + 10, r.y + lineH + 14, r.w - 20, r.h - lineH - 24}
}

// wrap breaks text into lines at most w pixels wide (a word wider than that gets a line of its own).
func wrap(s string, w int) []string { return wrapBy(s, w, textW) }

// wrapCols breaks plain text (a report, not the screen) into lines of at most n characters.
func wrapCols(s string, n int) []string {
	return wrapBy(s, n, func(s string) int { return len([]rune(s)) })
}

func wrapBy(s string, w int, width func(string) int) []string {
	var out []string
	for _, para := range strings.Split(s, "\n") {
		line := ""
		for _, word := range strings.Fields(para) {
			if line != "" && width(line+" "+word) > w {
				out = append(out, line)
				line = ""
			}
			if line != "" {
				line += " "
			}
			line += word
		}
		out = append(out, line)
	}
	return out
}
