package main

// The two arrangements of the window. Studio (the default): the tools in a
// rail on the left, the page's gadgets, colours, characters and the tool's
// options in a panel on the right, the page in between. Focus: the page as
// wide as the window, the page's gadgets in the toolbar and the drawing
// gadgets in a strip under the page (drawPane). The same gadgets in both
// (panel.go); View > Layout or Settings switches.

func (a *App) studio() bool { return prefs.UILayout != "focus" }

// setUILayout switches the arrangement and remembers it.
func (a *App) setUILayout(l string) {
	if l == "studio" {
		l = ""
	}
	prefs.UILayout = l
	savePrefs()
}

// panelScroll is how far the side panel is scrolled down, in pixels.
var panelScroll int

const panelW = 308

// railW is the width of the tool rail.
func railW() int { return buttonW("Colour") + 12 }

// drawStudio lays out the Studio arrangement in body (between the tabs and the status line).
func (a *App) drawStudio(u *UI, body Rect) {
	W := u.fb.w
	rail := Rect{body.x, body.y, railW(), body.h}
	panel := Rect{W - panelW, body.y, panelW, body.h}
	if W-rail.w-panel.w < 240 { // a tiny window: the page wins over the panel
		panel.w, panel.x = 0, W
	}
	center := Rect{rail.x + rail.w, body.y, panel.x - rail.x - rail.w, body.h}
	end := center.y + center.h
	if a.fed() != nil { // the letter strip of a TheDraw font (fed.go)
		strip := Rect{center.x, end - fedStripH(), center.w, fedStripH()}
		if strip.y-center.y >= 80 {
			end = strip.y
			a.drawFedStrip(u, strip)
		}
	}
	if a.stripShown() {
		strip := Rect{center.x, end - gadH - 3, center.w, gadH + 3}
		if strip.y-center.y >= 80 {
			end = strip.y
			a.drawStrip(u, strip)
		}
	}
	a.drawPages(u, Rect{center.x, center.y, center.w, end - center.y})
	if a.playing != nil && !u.inert { // the page on show is the one being played: hands off
		u.inert = true
		defer func() { u.inert = false }()
	}
	a.drawRail(u, rail)
	if panel.w > 0 {
		a.drawPanel(u, panel)
	}
}

// drawRail draws the tools down the left side.
func (a *App) drawRail(u *UI, r Rect) {
	fb := u.fb
	fb.fill(r, cGrey)
	fb.fill(Rect{r.x + r.w - 1, r.y, 1, r.h}, cEdgeDk)
	old := fb.clip
	fb.clip = r.and(old)
	a.drawTools(u, r.x+6, r.y+8, r.w-12, 1)
	fb.clip = old
}

// drawPanel draws the side panel: the page, the colours, the characters and
// the tool's options, one under the other, scrolled with the wheel.
func (a *App) drawPanel(u *UI, r Rect) {
	fb := u.fb
	fb.fill(r, cGrey)
	fb.fill(Rect{r.x, r.y, 1, r.h}, cEdgeDk)
	if d := u.takeWheel(r); d != 0 {
		panelScroll += d * 3 * lineH
	}
	old := fb.clip
	fb.clip = r.and(old)
	defer func() { fb.clip = old }()

	x, w := r.x+12, r.w-24
	y := r.y + 10 - panelScroll
	heading := func(title string) {
		fb.text(x, y, title, cHi)
		y += lineH + 6
	}
	rule := func() {
		y += 10
		fb.fill(Rect{r.x + 1, y, r.w - 1, 1}, cEdgeDk)
		y += 10
	}

	heading("Page")
	fl := &flow{x0: x, x: x, y: y, right: x + w}
	a.drawPageGadgets(u, fl, true)
	y = fl.y + gadH
	rule()

	heading("Colours")
	y = a.drawColours(u, x+2, y+2, 8) + 10
	a.drawBrush(u, x, y)
	y += 2*a.cellH() + 6
	rule()

	heading("Characters")
	a.drawFsetSwitch(u, x+w-fsetSwitchW(), y-lineH-6)
	y = a.drawFkeyRow(u, x, y) + 10
	cols := 32
	if 32*a.cellW() > w {
		cols = 16
	}
	cm := a.drawCharset(u, x, y, cols)
	y += cm.h
	rule()

	if a.layered() { // layers_ui.go
		heading("Layers")
		y = a.drawLayers(u, x, y, w)
		rule()
	}

	what := "The tool"
	for _, t := range tools {
		if t.id == a.tool {
			what = t.name
		}
	}
	if a.sel != nil || a.float != nil {
		what = "The block"
	}
	heading(what)
	y = a.drawBlockOrToolOptions(u, x, y, x+w)

	// Scrolled no further than the end shows.
	content := y + panelScroll + 12 - r.y
	panelScroll = max(0, min(panelScroll, content-r.h))
}
