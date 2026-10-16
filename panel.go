package main

import "fmt"

// The gadgets for drawing: tools, characters, F-keys, colours, the brush and
// the tool's options. Each is drawn at a given place, so the two layouts can
// arrange the same gadgets: across the strip under the page (Focus, drawPane)
// or down a rail and a side panel (Studio, drawRail and drawPanel).

// drawTools draws the tool buttons in cols columns w wide from (x, y), then the
// style toggles in rows as wide as the columns; it returns the y under them.
func (a *App) drawTools(u *UI, x, y, w, cols int) int {
	i := 0
	for _, t := range tools {
		if t.id == "half" && !a.pc() && !a.tt() && !a.uni() {
			continue
		}
		r := Rect{x + (i%cols)*(w+4), y + (i/cols)*(smallH+4), w, smallH}
		name, hint := t.name, t.hint
		if t.id == "half" && a.tt() {
			name, hint = "Sixel", "Sixels: left sets, right clears one of a cell's 2x3 graphics pixels (the row needs a graphics colour to its left)"
		}
		if u.button(r, name, a.tool == t.id, hint) != nil {
			a.start = false
			a.setTool(t.id)
		}
		i++
	}
	sy := y + (i+cols-1)/cols*(smallH+4) // under the tools this page has
	sx := x
	right := x + cols*(w+4) - 4
	toggle := func(label string, on bool, hint string) bool {
		r := Rect{sx, sy, buttonW(label) - 4, smallH}
		if sx > x && r.x+r.w > right {
			sx, sy = x, sy+smallH+4
			r.x, r.y = sx, sy
		}
		sx += r.w + 4
		if u.button(r, label, on, hint) != nil {
			return !on
		}
		return on
	}
	if a.amiga() || a.uni() {
		a.ital = toggle("I", a.ital, "Italic (SGR 3)")
		a.under = toggle("U", a.under, "Underline (SGR 4)")
	}
	if a.zx() {
		a.flash = toggle("Flash", a.flash, "Flash: new cells swap ink and paper twice a second")
	}
	if a.tt() {
		a.showCodes = toggle("Codes", a.showCodes, "Show the attribute codes' names in their cells (T text colour, G graphics colour, D double, F flash ...)")
	}
	if a.screen() {
		a.rev = toggle("Rev", a.rev, "Reverse: typing gives reversed characters, like RVS ON ("+keyName("reverse")+")")
	}
	a.insert = toggle("Ins", a.insert, "Insert mode for typing (Insert key)")
	return sy + smallH
}

// drawCharset draws the page font's 256 characters, cols to a row, at the
// font's own size, and returns where they are. On a teletext page the first
// half is the codes (attributes, then letters), the second the same codes as sixels.
func (a *App) drawCharset(u *UI, x, y, cols int) Rect {
	if a.uni() {
		return a.drawUniCharset(u, x, y, cols)
	}
	fb := u.fb
	CW, H := a.cellW(), a.cellH()
	cm := Rect{x, y, cols * CW, 256 / cols * H}
	fb.fill(cm, cPaper)
	for c := 0; c < 256; c++ {
		px, py := cm.x+(c%cols)*CW, cm.y+(c/cols)*H
		switch {
		case a.tt() && c >= 128:
			if k := c - 128; k >= 0x20 && (k < 0x40 || k >= 0x60) {
				a.drawTTCell(fb, px, py, 1, ttCell{code: k, mosaic: true, fg: 7, control: -1}, false)
			}
		case a.printable(c):
			a.drawCell(fb, px, py, 1, makeCell(c, a.previewFg(), 0, 0))
		default:
			fb.fill(Rect{px + 1, py + 1, CW - 2, H - 2}, 0xFF1A1B1E)
		}
	}
	fb.frame(Rect{cm.x + (a.brush%cols)*CW, cm.y + (a.brush/cols)*H, CW, H}, cOrange)
	a.charHover = -1
	if u.hover(cm) {
		a.charHover = (u.my-cm.y)/H*cols + (u.mx-cm.x)/CW
		u.hint = fmt.Sprintf("Character %d (0x%02X). Click: brush (types it with Type). Right click: put it on the active F-key.", a.charHover, a.charHover)
		if a.tt() {
			k := a.charHover & 0x7F
			what := fmt.Sprintf("code %d (0x%02X)", k, k)
			if k < 0x20 {
				what += ": " + ttCodeName(k)
			}
			u.hint = what + ". Click: brush. Right click: put it on the active F-key."
		}
	}
	if c := u.take(cm); c != nil {
		ch := (c.y-cm.y)/H*cols + (c.x-cm.x)/CW
		if a.tt() {
			ch &= 0x7F // the sixel rows are the same codes
		}
		if a.printable(ch) {
			switch {
			case c.button == 3:
				a.fkeys[a.mode][a.fset][a.factive] = ch
				savePrefs()
			case a.tool == "type" && !c.shift:
				a.typeByte(ch)
			default:
				a.brush = ch
				if a.tool != "draw" && a.tool != "fill" && a.tool != "type" {
					a.setTool("draw")
				}
			}
		}
	}
	return cm
}

// drawFkeys draws the F-key set switch and the ten F-keys in a row at (x, y),
// and returns the y under them.
func (a *App) drawFkeys(u *UI, x, y int) int {
	a.drawFsetSwitch(u, x, y)
	return a.drawFkeyRow(u, x+fsetSwitchW()+6, y)
}

// fsetSwitchW is the width of the F-key set switch: two arrows and the set's number.
func fsetSwitchW() int { return 2*fsetArrowW() + max(12, textW("8")+4) }

func fsetArrowW() int { return max(18, textW(">")+10) }

// drawFsetSwitch draws the F-key set's number between its two arrows.
func (a *App) drawFsetSwitch(u *UI, x, y int) {
	bw := fsetArrowW()
	if u.button(Rect{x, y, bw, smallH}, "<", false, "Previous F-key set ("+keyName("fkeys_previous")+")") != nil {
		a.setFset(a.fset - 1)
	}
	label := fmt.Sprintf("%d", a.fset+1)
	u.fb.text(x+bw+4, y+2, label, cText)
	if u.button(Rect{x + fsetSwitchW() - bw, y, bw, smallH}, ">", false, "Next F-key set ("+keyName("fkeys_next")+")") != nil {
		a.setFset(a.fset + 1)
	}
}

// fkeyRowW is the width of the ten F-keys.
const fkeyRowW = 10*28 - 2

// drawFkeyRow draws the ten F-keys from (x, y) and returns the y under them.
func (a *App) drawFkeyRow(u *UI, x, y int) int {
	fb := u.fb
	H := a.cellH()
	fx := x
	for i, c := range a.fkeys[a.mode][a.fset] {
		r := Rect{fx + i*28, y, 26, H + lineH + 6}
		fb.fill(r, cGrey)
		fb.bevel(r, true)
		fb.fill(Rect{r.x + 9, r.y + 2, 8, H}, cPaper)
		a.drawCell(fb, r.x+9, r.y+2, 1, a.previewCell(c))
		name := fmt.Sprintf("F%d", i+1)
		col := uint32(cText)
		if i == a.factive {
			col = cOrange
		}
		fb.text(r.x+(r.w-textW(name))/2, r.y+H+3, name, col)
		if u.take(r) != nil {
			a.factive, a.brush = i, c
			if a.tool != "draw" && a.tool != "fill" && a.tool != "type" {
				a.setTool("draw")
			}
		}
		u.setHint(r, fmt.Sprintf("F%d: character %s. F1-F10 type these; click makes it the brush", i+1, a.charLabel(c)))
	}
	return y + H + lineH + 6
}

// drawColours draws the 16 colours, perRow to a row, from (x, y), and returns the y under them.
func (a *App) drawColours(u *UI, x, y, perRow int) int {
	fb := u.fb
	for i := 0; i < 16; i++ {
		r := Rect{x + i%perRow*20, y + i/perRow*26, 18, 18}
		fb.fill(r, a.pal[i])
		fb.frame(r, cEdgeDk)
		fgNow, fgName := a.fg, "foreground"
		if a.atari() {
			fgNow, fgName = a.doc.ink, "the text colour"
		}
		if i == fgNow {
			fb.frame(r.inset(-2), cHi)
		}
		bgNow := a.bg
		bgName := "background"
		if a.screen() {
			bgNow, bgName = a.doc.bg, "the screen colour, middle: the border"
		}
		if i == bgNow {
			fb.dashed(r.inset(-2), cOrange)
		}
		if (a.screen() || a.zx()) && i == a.doc.borderColour() { // the border: a mark under the swatch
			fb.fill(Rect{r.x + 4, r.y + r.h + 3, r.w - 8, 2}, cHi)
		}
		u.setHint(r, fmt.Sprintf("Colour %d: #%06X. Left: %s, right: %s, Shift+click: change it", i, a.pal[i]&0xFFFFFF, fgName, bgName))
		if c := u.take(r); c != nil {
			switch {
			case c.shift:
				openColour(i)
			case c.button == 3:
				a.setBg(i)
			case c.button == 2 && (a.screen() || a.zx()):
				if a.doc.border != i {
					a.pushUndo()
					a.doc.border = i
				}
			default:
				a.setFg(i)
			}
		}
	}
	if a.uni() && perRow < 16 { // the 256 and 24-bit colours: under the 16 in the panel, beside them in the strip
		return a.drawUniColours(u, x, y+(15/perRow+1)*26-4, perRow*20-2, true)
	}
	if a.uni() {
		a.drawUniColours(u, x+16*20+6, y+1, 0, false)
	}
	return y + (15/perRow+1)*26 - 8
}

// drawBrush draws the brush, the Swap button and the brush's numbers at
// (x, y), and returns the x where what follows the brush box starts.
func (a *App) drawBrush(u *UI, x, y int) int {
	fb := u.fb
	H := a.cellH()
	br := Rect{x, y, 8*2 + 6, H*2 + 6}
	fb.fill(br, cPaper)
	fb.frame(br, cEdgeDk)
	a.drawCell(fb, br.x+3, br.y+3, 2, a.brushCell())
	u.setHint(br, "The brush")
	if u.button(Rect{br.x + br.w + 8, y, buttonW("Swap"), smallH}, "Swap", false, "Swap foreground and background ("+keyName("swap_colours")+")") != nil {
		a.fg, a.bg = a.bg, a.fg
		a.ufg, a.ubg, a.ufgOn, a.ubgOn = a.ubg, a.ufg, a.ubgOn, a.ufgOn
	}
	fgShown, bgShown := a.fg, a.bg
	if a.screen() {
		bgShown = a.doc.bg
	}
	if a.atari() {
		fgShown = a.doc.ink
	}
	note := fmt.Sprintf("char %d  fg %d  bg %d", a.brush, fgShown, bgShown)
	if a.uni() {
		note = fmt.Sprintf("%s  fg %s  bg %s", a.charLabel(a.brush), colourName(a.uniFg()), colourName(a.uniBg()))
	}
	fb.text(br.x+br.w+8+buttonW("Swap")+10, y+2, note, cText)
	return br.x + br.w + 8
}

// drawBlockOrToolOptions draws, from (x, y) in rows ending before right, the
// buttons that turn the selected block over, or else the tool's options; it
// returns the y under them.
func (a *App) drawBlockOrToolOptions(u *UI, x, y, right int) int {
	if a.sel == nil && a.float == nil {
		return a.drawToolOptions(u, x, y, right)
	}
	what := "the selection"
	if a.float != nil {
		what = "the block being pasted"
	}
	bx := x
	for _, b := range []struct {
		label, hint string
		op          int
	}{
		{"Mirror", "Flip " + what + " left-right (" + keyName("mirror") + ")", flipH},
		{"Upside down", "Flip " + what + " upside down (" + keyName("upside_down") + ")", flipV},
		{"Turn", "Turn " + what + " a quarter clockwise (" + keyName("turn") + ")", turnCW},
		{"Invert", "Invert " + what + ": reversed characters, or the colours swapped (" + keyName("invert") + ")", invertBlock},
	} {
		r := Rect{bx, y, buttonW(b.label), smallH}
		if bx > x && r.x+r.w > right {
			bx, y = x, y+smallH+4
			r.x, r.y = bx, y
		}
		if u.button(r, b.label, false, b.hint) != nil {
			a.blockOp(b.op)
		}
		bx += r.w + 4
	}
	return y + smallH
}
