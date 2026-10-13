package main

// The main window: the gadget bar, the page, the bottom pane and the status line.

import (
	"fmt"
	"strconv"
	"time"
)

// ---------------------------------------------------------------- drawing cells

// drawCell draws a page cell at pixel (px, py), every font pixel z by z.
func (a *App) drawCell(fb *FB, px, py, z int, v uint32) {
	f, H := a.font(), a.cellH()
	skip := 1
	if H == 8 {
		skip = 2
	}
	ch, fg, bg := cellCh(v), cellFg(v), cellBg(v)
	it, ul := v&italic != 0, v&underline != 0
	var fgc, bgc uint32
	smear := false
	switch {
	case a.c64():
		bgc, it, ul = a.pal[a.doc.bg], false, false
	case a.ice:
		bgc = a.pal[bg]
	default:
		bgc = a.pal[bg&7]
		if bg > 7 && a.pc() && !a.blink {
			ch = 32 // blinking text, off phase
		}
	}
	if a.amiga() && a.smear && fg > 7 {
		fgc, smear = a.pal[fg&7], true
	} else {
		fgc = a.pal[fg]
	}
	if !a.printable(ch) {
		ch = 32
	}
	cl := fb.clip
	inside := px >= cl.x && py >= cl.y && px+8*z <= cl.x+cl.w && py+H*z <= cl.y+cl.h
	for r := 0; r < H; r++ {
		bits := int(f.bits[ch*16+r*skip])
		r8 := r
		if H == 16 {
			r8 = r >> 1
		}
		if it {
			bits >>= max(0, (6-r8)>>1)
		}
		if smear {
			bits |= bits >> 1
		}
		if ul && r8 == 7 {
			bits = 0xFF
		}
		for b := 0; b < 8; b++ {
			c := bgc
			if bits&(0x80>>b) != 0 {
				c = fgc
			}
			if inside {
				for yy := 0; yy < z; yy++ {
					o := (py+r*z+yy)*fb.w + px + b*z
					for xx := 0; xx < z; xx++ {
						fb.px[o+xx] = c
					}
				}
			} else {
				fb.fill(Rect{px + b*z, py + r*z, z, z}, c)
			}
		}
	}
}

// paintCells draws w x h cells from get at pixel (x0, y0).
func (a *App) paintCells(fb *FB, x0, y0, z int, get func(x, y int) uint32, w, h int) {
	H := a.cellH()
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			a.drawCell(fb, x0+x*8*z, y0+y*H*z, z, get(x, y))
		}
	}
}

// ---------------------------------------------------------------- layout

const gadH = lineH + 6

var fieldCols = &Field{max: 4}

func init() {
	fieldCols.enter = func() {
		if n, err := strconv.Atoi(fieldCols.String()); err == nil {
			a.setCols(n)
		}
		fieldCols.set(strconv.Itoa(a.doc.cols))
		ui.focus = nil
	}
}

type flow struct {
	x0, x, y, right int
}

func (f *flow) place(w int) Rect {
	if f.x > f.x0 && f.x+w > f.right {
		f.x, f.y = f.x0, f.y+gadH+4
	}
	r := Rect{f.x, f.y, w, gadH}
	f.x += w + 4
	return r
}
func (f *flow) gap() { f.x += 10 }

func (a *App) drawWindow(u *UI) {
	if viewer != nil {
		a.drawViewer(u)
		return
	}
	fb := u.fb
	W, H := fb.w, fb.h
	fb.fill(Rect{0, 0, W, H}, cGrey)

	top := 0
	if !a.bare {
		top = a.drawBar(u)
		top = a.drawTabs(u, top)
	}

	// The status line and the pane, from the bottom up.
	status := Rect{0, H - lineH - 6, W, lineH + 6}
	paneH := a.paneHeight()
	if a.bare {
		paneH = 0
	}
	pane := Rect{0, status.y - paneH, W, paneH}
	if pane.y-top < 80 { // a tiny window: the page wins over the pane
		pane.h = max(0, status.y-top-80)
		pane.y = status.y - pane.h
	}
	a.drawPages(u, Rect{0, top, W, pane.y - top})
	a.drawPane(u, pane)
	a.drawStatus(u, status)
}

// drawPages lays out one page, or two side by side or one over the other.
func (a *App) drawPages(u *UI, area Rect) {
	if a.layout == "one" || a.shown[1] == nil {
		a.shown[0] = a.Tab
		a.page = Rect{area.x, area.y, area.w - 18, area.h - 18}
		a.drawPage(u, "0")
		return
	}
	var rs [2]Rect
	if a.layout == "cols" {
		w := (area.w - 4) / 2
		rs[0], rs[1] = Rect{area.x, area.y, w, area.h}, Rect{area.x + w + 4, area.y, area.w - w - 4, area.h}
		u.fb.fill(Rect{area.x + w, area.y, 4, area.h}, cEdgeDk)
	} else {
		h := (area.h - 4) / 2
		rs[0], rs[1] = Rect{area.x, area.y, area.w, h}, Rect{area.x, area.y + h + 4, area.w, area.h - h - 4}
		u.fb.fill(Rect{area.x, area.y + h, area.w, 4}, cEdgeDk)
	}
	active := a.Tab
	pressed := (*Tab)(nil)
	for i, t := range a.shown {
		a.Tab = t
		a.applyMode()
		r := rs[i]
		// The pane's title: its tab, bright in the pane being worked on.
		col := uint32(cText)
		if t == active {
			col = cHi
			u.fb.fill(Rect{r.x, r.y, r.w, lineH + 2}, cBlue)
		}
		title := t.name
		if t.dirty {
			title = "* " + title
		}
		u.fb.text(r.x+6, r.y+1, title, col)
		if u.take(Rect{r.x, r.y, r.w, lineH + 2}) != nil && t != active {
			pressed = t
		}
		r.y, r.h = r.y+lineH+2, r.h-lineH-2
		t.page = Rect{r.x, r.y, r.w - 18, r.h - 18}
		hadClick := u.click != nil
		a.drawPage(u, strconv.Itoa(i))
		if hadClick && u.click == nil && t != active && pressed == nil && (rs[i].has(ui.mx, ui.my)) {
			pressed = t
		}
	}
	if a.syncScroll {
		other := a.shown[0]
		if other == active {
			other = a.shown[1]
		}
		other.scrollX, other.scrollY = active.scrollX, active.scrollY
	}
	a.Tab = active
	if pressed != nil {
		a.Tab = pressed
	}
	a.applyMode()
}

// drawTabs draws the strip of open pages under the gadget bar.
func (a *App) drawTabs(u *UI, top int) int {
	fb := u.fb
	h := lineH + 4
	fb.fill(Rect{0, top, fb.w, h}, cDark)
	x := 4
	for _, t := range a.tabs {
		label := t.name
		if t.dirty {
			label = "* " + label
		}
		w := textW(label) + 30
		r := Rect{x, top + 1, w, h - 1}
		on := t == a.Tab
		shown := t == a.shown[0] || (a.layout != "one" && t == a.shown[1])
		bg := uint32(cDark)
		switch {
		case on:
			bg = cBlue
		case shown:
			bg = cGrey
		}
		fb.fill(r, bg)
		col := uint32(cText)
		if on {
			col = cHi
		}
		fb.text(r.x+6, r.y+1, label, col)
		xr := Rect{r.x + w - 18, r.y, 16, r.h}
		fb.text(xr.x+4, r.y+1, "x", cText)
		u.setHint(r, t.path+"  (Ctrl+Tab next page, Ctrl+W closes, middle click closes)")
		if c := u.take(xr); c != nil {
			a.closeTab(t)
			break
		}
		if c := u.take(r); c != nil {
			if c.button == 2 {
				a.closeTab(t)
				break
			}
			a.focusTab(t)
		}
		x += w + 2
		if x > fb.w-40 {
			break
		}
	}
	if u.button(Rect{x + 2, top + 1, 22, h - 1}, "+", false, "New page (Ctrl+T)") != nil {
		a.addTab()
	}
	top += h
	fb.fill(Rect{0, top, fb.w, 1}, cEdgeDk)
	return top + 1
}

// drawBar draws the gadget bar and returns where the page starts.
func (a *App) drawBar(u *UI) int {
	fb := u.fb
	W := fb.w
	fl := &flow{x0: 6, x: 6, y: 4, right: W - 6}
	if u.button(fl.place(buttonW("New")), "New", false, "New page in a new tab (Ctrl+T)") != nil {
		a.addTab()
	}
	if u.button(fl.place(buttonW("Open")), "Open", false, "Open a file in a new tab (Ctrl+O)") != nil {
		openOpenRequester()
	}
	if u.button(fl.place(buttonW("Save")), "Save", false, "Save (Ctrl+S)") != nil {
		a.save(false)
	}
	if u.button(fl.place(buttonW("Save as")), "Save as", false, "Save as / export (Ctrl+Shift+S)") != nil {
		a.save(true)
	}
	fl.gap()
	modeHint := "Amiga: Latin-1, Amiga fonts, 0x9B CSI. PC: CP437, VGA font, CRLF. C64: PETSCII, 40 columns, .seq files."
	for _, m := range []string{"amiga", "pc", "c64"} {
		if u.button(fl.place(buttonW(modeNames[m])), modeNames[m], a.mode == m, modeHint) != nil {
			a.setMode(m)
		}
	}
	var fontNames, fontIDs []string
	cur := 0
	for _, f := range fonts {
		if f.kind == a.mode {
			if f == a.font() {
				cur = len(fontIDs)
			}
			fontNames = append(fontNames, f.name)
			fontIDs = append(fontIDs, f.id)
		}
	}
	allFonts := []string{}
	for _, f := range fonts {
		allFonts = append(allFonts, f.name)
	}
	u.choice("font", fl.place(choiceW(allFonts)), fontNames, cur, "Font (on a C64 page: the character set)", func(i int) {
		a.fontFor[a.mode] = fontIDs[i]
		savePrefs()
	})
	var palNames []string
	pcur := 0
	for i, p := range palettes {
		palNames = append(palNames, p.name)
		if p.id == a.paletteFor[a.mode] {
			pcur = i
		}
	}
	u.choice("palette", fl.place(choiceW(palNames)), palNames, pcur, "Palette (Shift+click a colour below to change it)", func(i int) {
		a.paletteFor[a.mode] = palettes[i].id
		a.setPalette()
		savePrefs()
	})
	fl.gap()
	r := fl.place(textW("W") + 8 + 6*charW + 8)
	fb.text(r.x, r.y+3, "W", cText)
	if u.focus != fieldCols {
		fieldCols.set(strconv.Itoa(a.doc.cols))
	}
	u.field(Rect{r.x + 16, r.y, r.w - 16, r.h}, fieldCols, "Page width in columns (Enter sets it)")
	zooms := []string{"1x", "2x", "3x", "4x", "5x", "6x", "7x", "8x", "Fit"}
	u.choice("zoom", fl.place(choiceW(zooms)), zooms, a.zoom-1, "Zoom (Ctrl+wheel, Ctrl+Plus/Minus; Ctrl+0 fits the width)", func(i int) {
		if i == 8 {
			a.fitZoom()
		} else {
			a.setZoom(i + 1)
		}
	})
	r = fl.place(checkW("Grid"))
	if v, _ := u.check(r.x, r.y, "Grid", a.grid, "Lines between the cells; brighter every 10 columns and every 25 rows (Ctrl+G)"); v != a.grid {
		a.grid = v
		savePrefs()
	}
	layouts := []string{"One page", "Side by side", "Top / bottom"}
	ids := []string{"one", "cols", "rows"}
	lcur := 0
	for i, id := range ids {
		if id == a.layout {
			lcur = i
		}
	}
	u.choice("layout", fl.place(choiceW(layouts)), layouts, lcur, "Show one page, or two pages side by side or one over the other (Ctrl+L)", func(i int) {
		a.setLayout(ids[i])
	})
	if a.layout != "one" {
		r = fl.place(checkW("Sync"))
		a.syncScroll, _ = u.check(r.x, r.y, "Sync", a.syncScroll, "Scroll both pages together")
	}
	if a.amiga() {
		r = fl.place(checkW("8x8"))
		if v, _ := u.check(r.x, r.y, "8x8", a.ch8, "Amiga fonts at 8x8 (no line doubling)"); v != a.ch8 {
			a.ch8, a.ch8Default = v, v
			savePrefs()
		}
	}
	if !a.c64() {
		r = fl.place(checkW("iCE"))
		a.ice, _ = u.check(r.x, r.y, "iCE", a.ice, "Background colours 8-15 instead of blink (iCE colours)")
	}
	if a.amiga() {
		r = fl.place(checkW("Bold smear"))
		if v, _ := u.check(r.x, r.y, "Bold smear", a.smear, "Draw bold as the Amiga console does (smeared glyph, same colour) instead of bright colours"); v != a.smear {
			a.smear, a.smearDefault = v, v
			savePrefs()
		}
	}
	fl.gap()
	var baudNames []string
	bcur := 2
	for i, b := range bauds {
		baudNames = append(baudNames, strconv.Itoa(b))
		if b == a.baud {
			bcur = i
		}
	}
	u.choice("baud", fl.place(choiceW(baudNames)), baudNames, bcur, "Play speed (bits per second)", func(i int) {
		a.baud = bauds[i]
		savePrefs()
	})
	playLabel := "Play"
	if a.playing != nil {
		playLabel = "Stop"
	}
	if u.button(fl.place(buttonW("Play")), playLabel, a.playing != nil, "Play the file as if it came over a modem (Esc stops)") != nil {
		a.play()
	}
	if u.button(fl.place(buttonW("Info")), "Info", false, "SAUCE info (title, author, group) and save options") != nil {
		openInfo()
	}
	if u.button(fl.place(buttonW("Settings")), "Settings", false, "Settings: window size, the C64 character set") != nil {
		openSettings()
	}
	if u.button(fl.place(buttonW("?")), "?", false, "Keys and mouse") != nil {
		openHelp()
	}
	top := fl.y + gadH + 4
	fb.fill(Rect{0, top, W, 1}, cEdgeDk)
	return top + 1
}

// ---------------------------------------------------------------- zoom

func (a *App) setZoom(z int) {
	a.zoomAt(z, a.page.x+a.page.w/2, a.page.y)
}

// zoomAt changes the zoom keeping the page still under the point (mx, my).
func (a *App) zoomAt(z, mx, my int) {
	z = max(1, min(8, z))
	if z == a.zoom {
		return
	}
	px, py := mx-a.page.x+a.scrollX, my-a.page.y+a.scrollY
	a.scrollX = px*z/a.zoom - (mx - a.page.x)
	a.scrollY = py*z/a.zoom - (my - a.page.y)
	a.scrollX, a.scrollY = max(0, a.scrollX), max(0, a.scrollY)
	a.zoom = z
	savePrefs()
}

// fitZoom picks the biggest zoom that shows the whole width.
func (a *App) fitZoom() {
	a.scrollX = 0
	a.setZoom(max(1, min(8, a.page.w/(a.doc.cols*8))))
	a.scrollX = 0
}

// ---------------------------------------------------------------- the page

func (a *App) contentSize() (int, int) {
	z, H := a.zoom, a.cellH()
	rows := max(a.doc.rows+25, (a.page.h+H*z-1)/(H*z))
	return a.doc.cols * 8 * z, rows * H * z
}

func (a *App) drawPage(u *UI, id string) {
	fb, p := u.fb, a.page
	z, H := a.zoom, a.cellH()
	cw, ch := a.contentSize()
	// Scrollbars.
	vbar := Rect{p.x + p.w, p.y, 18, p.h}
	hbar := Rect{p.x, p.y + p.h, p.w, 18}
	a.scrollY = u.scrollbar("vscroll"+id, vbar, true, a.scrollY, p.h, ch)
	a.scrollX = u.scrollbar("hscroll"+id, hbar, false, a.scrollX, p.w, cw)
	fb.fill(Rect{p.x + p.w, p.y + p.h, 18, 18}, cGrey)
	if w := u.takeWheel(p); w != 0 {
		if shiftHeld() {
			a.scrollX += w * 8 * z * 4
		} else {
			a.scrollY += w * H * z * 3
		}
	}
	a.scrollY = max(0, min(a.scrollY, max(0, ch-p.h)))
	a.scrollX = max(0, min(a.scrollX, max(0, cw-p.w)))

	old := fb.clip
	fb.clip = p.and(old)
	fb.fill(p, 0xFF141518) // beyond the page width
	fb.fill(Rect{p.x, p.y, min(p.w, cw-a.scrollX), p.h}, cPaper)
	ox, oy := p.x-a.scrollX, p.y-a.scrollY
	rowPx, colPx := H*z, 8*z
	y0, y1 := a.scrollY/rowPx, (a.scrollY+p.h)/rowPx+1
	x0, x1 := a.scrollX/colPx, min(a.doc.cols, (a.scrollX+p.w)/colPx+1)
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			a.drawCell(fb, ox+x*colPx, oy+y*rowPx, z, a.doc.get(x, y))
		}
	}
	if a.grid {
		w := min(p.w, cw-a.scrollX)
		for x := x0; x <= x1; x++ {
			al := 34
			if x%10 == 0 {
				al = 80
			}
			fb.blend(Rect{ox + x*colPx, p.y, 1, p.h}, 0xFFFFFFFF, al)
		}
		for y := y0; y <= y1; y++ {
			al := 34
			if y%25 == 0 {
				al = 110
			}
			fb.blend(Rect{p.x, oy + y*rowPx, w, 1}, 0xFFFFFFFF, al)
		}
	}
	cell := func(x, y, w, h int) Rect { return Rect{ox + x*colPx, oy + y*rowPx, w * colPx, h * rowPx} }
	if a.float != nil && a.hover != nil {
		f := a.float
		hx, hy := a.hover.x, a.hover.y
		a.paintCells(fb, ox+hx*colPx, oy+hy*rowPx, z, func(x, y int) uint32 { return f.cells[y*f.w+x] }, f.w, f.h)
		fb.dashed(cell(hx, hy, f.w, f.h), cOrange)
	}
	if a.sel != nil {
		r := a.selRect()
		fb.blend(cell(r.x, r.y, r.w, r.h), cOrange, 46)
		fb.dashed(cell(r.x, r.y, r.w, r.h), cOrange)
	}
	if a.hover != nil && a.float == nil && a.tool != "type" && a.tool != "select" && a.playing == nil && viewer == nil {
		r := cell(a.hover.x, a.hover.y, 1, 1)
		if a.tool == "half" {
			r.h /= 2
			r.y += a.hover.half * r.h
		}
		fb.frame(r, 0xFFCCCCCC)
	}
	if (a.blink && viewer == nil) || a.playing != nil { // the cursor, Amiga style: an orange block
		r := cell(a.cx, a.cy, 1, 1)
		if a.insert {
			h := max(2*z, r.h/4)
			r.y, r.h = r.y+r.h-h, h
		}
		fb.blend(r, cOrange, 166)
	}
	fb.clip = old

	// A press on the page.
	if c := u.take(p); c != nil {
		a.pagePress(c)
	}
}

// cellAt turns a window position into a cell of the page.
func (a *App) cellAt(mx, my int) (Hover, bool) {
	z := a.zoom
	fx := float64(mx-a.page.x+a.scrollX) / float64(8*z)
	fy := float64(my-a.page.y+a.scrollY) / float64(a.cellH()*z)
	x, y := int(fx), int(fy)
	if fx < 0 {
		x = -1
	}
	if fy < 0 {
		y = -1
	}
	half := 0
	if fy-float64(y) >= 0.5 {
		half = 1
	}
	inside := fx >= 0 && x < a.doc.cols && fy >= 0
	return Hover{max(0, min(a.doc.cols-1, x)), max(0, y), half}, inside
}

func (a *App) pagePress(c *Click) {
	if a.playing != nil || viewer != nil {
		return
	}
	ui.focus = nil
	p, inside := a.cellAt(c.x, c.y)
	if !inside {
		return
	}
	a.hover = &p
	if a.float != nil {
		if c.button == 1 {
			a.stamp(p.x, p.y, c.shift)
		} else if c.button == 3 {
			a.float = nil
		}
		return
	}
	if c.button == 2 || a.tool == "pick" {
		a.pick(p.x, p.y)
		return
	}
	a.button = c.button
	switch a.tool {
	case "type":
		a.cx, a.cy, a.sel = p.x, p.y, nil
		a.moved()
	case "select":
		a.sel = &Sel{p.x, p.y, p.x, p.y}
		a.selecting = true
	case "fill":
		if c.button == 1 {
			a.pushUndo()
			a.floodFill(p.x, p.y)
		}
	case "half":
		a.pushUndo()
		hy := p.y*2 + p.half
		col := a.fg
		if c.button == 3 {
			col = a.bg
		}
		if c.shift && a.last != nil {
			line(a.last.x, a.last.y, p.x, hy, func(x, y int) { a.paintHalf(x, y, col) })
		} else {
			a.paintHalf(p.x, hy, col)
		}
		a.last = &Hover{x: p.x, y: hy}
	default: // draw, colour
		a.pushUndo()
		if c.shift && a.last != nil {
			line(a.last.x, a.last.y, p.x, p.y, func(x, y int) { a.paintAt(x, y, c.button) })
		} else {
			a.paintAt(p.x, p.y, c.button)
		}
		a.last = &Hover{x: p.x, y: p.y}
	}
}

// pageMotion follows the mouse over the page: hover, and painting while a button is down.
func (a *App) pageMotion(mx, my int) {
	if a.playing != nil {
		return
	}
	p, _ := a.cellAt(mx, my)
	over := a.page.has(mx, my) && a.dialog == nil
	switch {
	case over || a.button != 0:
		a.hover = &p
	case a.float == nil:
		a.hover = nil
	}
	if a.button == 0 {
		return
	}
	switch {
	case a.selecting:
		a.sel.x1, a.sel.y1 = p.x, p.y
	case a.tool == "half":
		hy := p.y*2 + p.half
		col := a.fg
		if a.button == 3 {
			col = a.bg
		}
		if a.last != nil {
			line(a.last.x, a.last.y, p.x, hy, func(x, y int) { a.paintHalf(x, y, col) })
		}
		a.last = &Hover{x: p.x, y: hy}
	case a.tool == "draw" || a.tool == "colour":
		if a.last != nil {
			b := a.button
			line(a.last.x, a.last.y, p.x, p.y, func(x, y int) { a.paintAt(x, y, b) })
		}
		a.last = &Hover{x: p.x, y: p.y}
	}
}

func (a *App) pageRelease() {
	if a.selecting {
		a.selecting = false
		r := a.selRect()
		a.msg(fmt.Sprintf("%dx%d selected: Ctrl+C copy, Ctrl+X cut, Del clear, Alt+H / Alt+V flip", r.w, r.h))
	}
	a.button = 0
}

// ---------------------------------------------------------------- the bottom pane

func (a *App) paneHeight() int {
	return max(8*a.cellH(), 5*(smallH+4)) + 14
}

const smallH = lineH + 4

var tools = []struct{ id, name, hint string }{
	{"type", "Type", "Type: click to place the cursor, then type"},
	{"draw", "Draw", "Draw: left paints the brush, right erases, Shift+click draws a line"},
	{"colour", "Colour", "Colour: left paints the foreground, right the background, the characters stay"},
	{"fill", "Fill", "Fill: fills the area of equal cells with the brush"},
	{"pick", "Pick", "Pick: takes the character and colours of a cell (middle click does this with any tool)"},
	{"select", "Select", "Select: drag a block; Ctrl+C / Ctrl+X / Ctrl+V / Del"},
	{"half", "Half", "Half blocks: paint pixels of half a cell (PC page)"},
}

// drawPane draws the strip under the page: tools, characters, F-keys and colours, kept small.
func (a *App) drawPane(u *UI, pane Rect) {
	fb := u.fb
	if pane.h <= 0 {
		return
	}
	fb.fill(Rect{pane.x, pane.y, pane.w, 1}, cEdgeDk)
	old := fb.clip
	fb.clip = pane.and(old)
	x, y := pane.x+8, pane.y+7
	H := a.cellH()

	// Tools, two columns, and the style toggles under them.
	bw := buttonW("Colour")
	i := 0
	for _, t := range tools {
		if t.id == "half" && !a.pc() {
			continue
		}
		r := Rect{x + (i%2)*(bw+4), y + (i/2)*(smallH+4), bw, smallH}
		if u.button(r, t.name, a.tool == t.id, t.hint) != nil {
			a.setTool(t.id)
		}
		i++
	}
	sy := y + 4*(smallH+4)
	sx := x
	toggle := func(label string, on bool, hint string) bool {
		r := Rect{sx, sy, buttonW(label) - 4, smallH}
		sx += r.w + 4
		if u.button(r, label, on, hint) != nil {
			return !on
		}
		return on
	}
	if a.amiga() {
		a.ital = toggle("I", a.ital, "Italic (Amiga console SGR 3)")
		a.under = toggle("U", a.under, "Underline (Amiga console SGR 4)")
	}
	if a.c64() {
		a.rev = toggle("Rev", a.rev, "Reverse: typing gives reversed characters, like RVS ON (Ctrl+R)")
	}
	a.insert = toggle("Ins", a.insert, "Insert mode for typing (Insert key)")
	x += 2*bw + 4 + 14

	// The characters, at the page font's own size.
	cm := Rect{x, y, 32 * 8, 8 * H}
	fb.fill(cm, cPaper)
	for c := 0; c < 256; c++ {
		px, py := cm.x+(c%32)*8, cm.y+(c/32)*H
		if a.printable(c) {
			a.drawCell(fb, px, py, 1, makeCell(c, a.previewFg(), 0, 0))
		} else {
			fb.fill(Rect{px + 1, py + 1, 6, H - 2}, 0xFF1A1B1E)
		}
	}
	fb.frame(Rect{cm.x + (a.brush%32)*8, cm.y + (a.brush/32)*H, 8, H}, cOrange)
	a.charHover = -1
	if u.hover(cm) {
		a.charHover = (u.my-cm.y)/H*32 + (u.mx-cm.x)/8
		u.hint = fmt.Sprintf("Character %d (0x%02X). Click: brush (types it with Type). Right click: put it on the active F-key.", a.charHover, a.charHover)
	}
	if c := u.take(cm); c != nil {
		ch := (c.y-cm.y)/H*32 + (c.x-cm.x)/8
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
	x += cm.w + 14

	// The F-keys in one row, the set switch before them.
	if u.button(Rect{x, y, 18, smallH}, "<", false, "Previous F-key set (Ctrl+Down)") != nil {
		a.setFset(a.fset - 1)
	}
	label := fmt.Sprintf("%d", a.fset+1)
	fb.text(x+22, y+2, label, cText)
	if u.button(Rect{x + 30, y, 18, smallH}, ">", false, "Next F-key set (Ctrl+Up)") != nil {
		a.setFset(a.fset + 1)
	}
	fx := x + 54
	for i, c := range a.fkeys[a.mode][a.fset] {
		r := Rect{fx + i*28, y, 26, H + lineH + 6}
		fb.fill(r, cGrey)
		fb.bevel(r, true)
		fb.fill(Rect{r.x + 9, r.y + 2, 8, H}, cPaper)
		a.drawCell(fb, r.x+9, r.y+2, 1, makeCell(c, a.previewFg(), 0, a.styleFlags()))
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
		u.setHint(r, fmt.Sprintf("F%d: character %d. F1-F10 type these; click makes it the brush", i+1, c))
	}

	// The colours in one row under them, then the brush.
	cy := y + H + lineH + 14
	for i := 0; i < 16; i++ {
		r := Rect{x + i*20, cy, 18, 18}
		fb.fill(r, a.pal[i])
		fb.frame(r, cEdgeDk)
		if i == a.fg {
			fb.frame(r.inset(-2), cHi)
		}
		bgNow := a.bg
		bgName := "background"
		if a.c64() {
			bgNow, bgName = a.doc.bg, "the screen colour"
		}
		if i == bgNow {
			fb.dashed(r.inset(-2), cOrange)
		}
		u.setHint(r, fmt.Sprintf("Colour %d: #%06X. Left: foreground, right: %s, Shift+click: change it", i, a.pal[i]&0xFFFFFF, bgName))
		if c := u.take(r); c != nil {
			switch {
			case c.shift:
				openColour(i)
			case c.button == 3:
				a.setBg(i)
			default:
				a.fg = i
			}
		}
	}
	by := cy + 26
	br := Rect{x, by, 8*2 + 6, H*2 + 6}
	fb.fill(br, cPaper)
	fb.frame(br, cEdgeDk)
	a.drawCell(fb, br.x+3, br.y+3, 2, a.brushCell())
	u.setHint(br, "The brush")
	if u.button(Rect{br.x + br.w + 8, by, buttonW("Swap"), smallH}, "Swap", false, "Swap foreground and background") != nil {
		a.fg, a.bg = a.bg, a.fg
	}
	bgShown := a.bg
	if a.c64() {
		bgShown = a.doc.bg
	}
	fb.text(br.x+br.w+8+buttonW("Swap")+10, by+2, fmt.Sprintf("char %d  fg %d  bg %d", a.brush, a.fg, bgShown), cText)
	fb.clip = old
}

// pageGlyph draws a character of the page's font in the interface's text colour.
func (a *App) pageGlyph(fb *FB, x, y, c int) {
	fb.glyph(a.font(), x, y, byte(c), cText, 0, 1, 1, false)
}

func luma(c uint32) int {
	return (int(c>>16&255)*3 + int(c>>8&255)*6 + int(c&255)) / 10
}

// ---------------------------------------------------------------- the status line

func (a *App) drawStatus(u *UI, r Rect) {
	fb := u.fb
	fb.fill(Rect{r.x, r.y, r.w, 1}, cEdgeDk)
	fb.fill(Rect{r.x, r.y + 1, r.w, r.h - 1}, cDark)
	y := r.y + 4
	x := 8
	pos := fmt.Sprintf("Cursor %d,%d", a.cx+1, a.cy+1)
	if a.insert {
		pos += " INS"
	}
	x = fb.text(x, y, pos, cText) + 24
	if a.hover != nil {
		v := a.doc.get(a.hover.x, a.hover.y)
		c := cellCh(v)
		x = fb.text(x, y, fmt.Sprintf("Mouse %d,%d: ", a.hover.x+1, a.hover.y+1), cText)
		if a.printable(c) {
			a.pageGlyph(fb, x, y, c)
		}
		x = fb.text(x+charW*2, y, fmt.Sprintf("%d fg %d bg %d", c, cellFg(v), cellBg(v)), cText) + 24
	}
	set := map[string]string{"amiga": "Amiga Latin-1", "pc": "PC CP437", "c64": "C64 PETSCII"}[a.mode]
	x = fb.text(x, y, fmt.Sprintf("%s, %s, %dx%d", set, a.font().name, a.doc.cols, max(a.doc.usedRows(), 1)), cText) + 24
	name := a.path
	if name == "" {
		name = a.name
	}
	if a.dirty {
		name = "* " + name
	}
	x = fb.text(x, y, name, cText) + 24
	text, col := u.hint, uint32(cText)
	if a.message != "" && time.Since(a.messageAt) < 4*time.Second {
		text, col = a.message, cOrange
	}
	fb.text(x, y, text, col)
}
