package main

// The main window: the gadget bar, the page, the bottom pane and the status line.

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ---------------------------------------------------------------- drawing cells

// drawCell draws a page cell at pixel (px, py), every font pixel z by z.
func (a *App) drawCell(fb *FB, px, py, z int, v uint32) {
	if a.tt() { // a teletext code on its own: a letter, or an attribute's name
		c := cellCh(v) & 0x7F
		cell := ttCell{code: c, fg: 7, control: -1}
		if c < 0x20 {
			cell.code, cell.control = 0x20, c
		}
		a.drawTTCell(fb, px, py, z, cell, true)
		return
	}
	if a.uni() {
		a.drawUniCell(fb, px, py, z, v)
		return
	}
	if a.vicMode() != vicHires { // multicolour and ECM (c64x.go)
		a.drawVICCell(fb, px, py, z, v)
		return
	}
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
	case a.screen():
		bgc, it, ul = a.pal[a.doc.bg], false, false
	case a.st():
		bgc = a.pal[bg]
	case a.zx():
		if v&flashFlag != 0 && !a.blink { // flashing: ink and paper swap
			fg, bg = bg, fg
		}
		bgc = a.pal[bg]
	case a.cpc(): // pens and papers past the mode's inks wrap, as on the CPC
		fg, bg = a.cpcSet.mask(fg), a.cpcSet.mask(bg)
		bgc = a.pal[bg]
	case a.ice:
		bgc = a.pal[bg]
	default:
		bgc = a.pal[bg&7]
		if bg > 7 && a.pc() && !a.blink {
			ch = 32 // blinking text, off phase
		}
	}
	switch {
	case a.atari():
		fgc = a.pal[a.doc.ink]
	case a.c64() && a.mach().mono: // the PET: one colour on the screen
		fgc = a.pal[1]
	case a.c64() && a.mach().colours == 8: // the VIC-20 shows 8 text colours
		fgc = a.pal[fg&7]
	case a.amiga() && a.smear && fg > 7:
		fgc, smear = a.pal[fg&7], true
	default:
		fgc = a.pal[fg]
	}
	if !a.printable(ch) {
		ch = 32
	}
	cl := fb.clip
	W := a.cellW()
	inside := px >= cl.x && py >= cl.y && px+W*z <= cl.x+cl.w && py+H*z <= cl.y+cl.h
	for r := 0; r < H; r++ {
		bits := int(f.bits[ch*16+r*skip])
		r8 := r * 8 / H // the row of an 8-row cell (italic, underline)
		if it {
			bits >>= max(0, (6-r8)>>1)
		}
		if smear {
			bits |= bits >> 1
		}
		if W == 16 { // a CPC mode 0 cell: pixels twice as wide
			bits = cpcWide[bits&255]
		}
		if ul && r8 == 7 {
			bits = 0xFF
		}
		if W == 9 { // VGA's ninth column: the line characters 0xC0-0xDF repeat their eighth
			ninth := 0
			if ch >= 0xC0 && ch <= 0xDF {
				ninth = bits & 1
			}
			bits = bits<<1 | ninth
		}
		for b := 0; b < W; b++ {
			c := bgc
			if bits&(1<<(W-1-b)) != 0 {
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

// drawTTCell draws a decoded teletext cell; with labels, an attribute's cell shows its name.
func (a *App) drawTTCell(fb *FB, px, py, z int, c ttCell, labels bool) {
	bg, fg := a.pal[c.bg&7], a.pal[c.fg&7]
	show := !(c.flash && !a.blink) && !(c.conceal && !labels)
	for y := 0; y < ttH; y++ {
		for x := 0; x < ttW; x++ {
			col := bg
			if show && ttInk(c, x, y) {
				col = fg
			}
			fb.fill(Rect{px + x*z, py + y*z, z, z}, col)
		}
	}
	if labels && c.control >= 0 {
		name, colour := ttLabel(c.control)
		ink := uint32(0xFF808080)
		if colour >= 0 {
			ink = a.pal[colour]
		}
		fb.text(px+(ttW*z-8)/2, py+(ttH*z-16)/2, name, ink)
	}
}

// paintCells draws w x h cells from get at pixel (x0, y0).
func (a *App) paintCells(fb *FB, x0, y0, z int, get func(x, y int) uint32, w, h int) {
	a.paintCellsAt(fb, x0, y0, z, get, w, h, 0, 0)
}

// paintCellsAt is paintCells for cells whose (0, 0) is the page's (px, py): an IGS page's picture
// shows under the empty ones.
func (a *App) paintCellsAt(fb *FB, x0, y0, z int, get func(x, y int) uint32, w, h, px, py int) {
	if a.tt() { // teletext rows are decoded as a whole
		d := newTTDoc()
		d.cols, d.rows = w, h
		d.cells = make([]uint32, w*h)
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				d.cells[y*w+x] = get(x, y)
			}
		}
		for y := 0; y < h; y++ {
			row := ttDecode(d, y, a.ttBlack())
			for x := 0; x < w; x++ {
				a.drawTTCell(fb, x0+x*ttW*z, y0+y*ttH*z, z, row[x], false)
			}
		}
		return
	}
	H := a.cellH()
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			a.drawCellXY(fb, x0+x*a.cellW()*z, y0+y*H*z, z, px+x, py+y, get(x, y))
		}
	}
}

// ---------------------------------------------------------------- layout

// gadH and smallH follow the interface text's height (setFace).
var gadH = lineH + 6

var fieldCols = &Field{max: 4}
var fieldRows = &Field{max: 6}

func init() {
	fieldRows.enter = func() {
		if n, err := strconv.Atoi(fieldRows.String()); err == nil {
			a.setRows(n)
		}
		fieldRows.set(strconv.Itoa(a.doc.rows))
		ui.focus = nil
	}
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
		top = a.drawMenuBar(u)
		top = a.drawBar(u, top)
		top = a.drawTabs(u, top)
	}

	// The status line and the pane, from the bottom up.
	status := Rect{0, H - lineH - 6, W, lineH + 6}
	if a.studio() && !a.bare {
		a.drawStudio(u, Rect{0, top, W, status.y - top})
		a.drawStatus(u, status)
		return
	}
	paneH := a.paneHeight()
	if a.bare {
		paneH = 0
	}
	pane := Rect{0, status.y - paneH, W, paneH}
	if pane.y-top < 80 { // a tiny window: the page wins over the pane
		pane.h = max(0, status.y-top-80)
		pane.y = status.y - pane.h
	}
	end := pane.y
	if a.stripShown() {
		strip := Rect{0, pane.y - gadH - 3, W, gadH + 3}
		if strip.y-top >= 80 {
			end = strip.y
			a.drawStrip(u, strip)
		}
	}
	a.drawPages(u, Rect{0, top, W, end - top})
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
		a.applyLook() // only the look: the tool and F-keys stay the active page's
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
	a.applyLook()
	if pressed != nil {
		a.endPlay()
		a.Tab = pressed
		a.applyMode()
	}
}

// drawTabs draws the strip of open pages under the gadget bar.
func (a *App) drawTabs(u *UI, top int) int {
	fb := u.fb
	h := lineH + 4
	fb.fill(Rect{0, top, fb.w, h}, cDark)
	label := func(t *Tab) string {
		if t.dirty {
			return "* " + t.name
		}
		return t.name
	}
	// More tabs than fit: the strip starts late enough that the active tab is
	// on show, and small arrows at the ends say there are more.
	room := fb.w - 4 - 28 - 2*20
	first, cur := 0, max(0, a.tabIndex(a.Tab))
	for {
		used := 0
		for i := first; i <= cur && i < len(a.tabs); i++ {
			used += textW(label(a.tabs[i])) + 32
		}
		if used <= room || first >= cur {
			break
		}
		first++
	}
	x := 4
	if first > 0 {
		if u.button(Rect{x, top + 1, 18, h - 1}, "<", false, "More pages to the left ("+keyName("previous_page")+")") != nil {
			a.cycleTab(-1)
		}
		x += 20
	}
	last := len(a.tabs) - 1
	for i := first; i < len(a.tabs); i++ {
		t := a.tabs[i]
		label := label(t)
		w := textW(label) + 30
		if i > first && x+w > 4+room+20*min(1, first) {
			last = i - 1
			break
		}
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
		u.setHint(r, t.path+"  ("+keyName("next_page")+" next page, "+keyName("close_page")+" closes, middle click closes)")
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
	}
	if last < len(a.tabs)-1 {
		if u.button(Rect{x, top + 1, 18, h - 1}, ">", false, "More pages to the right ("+keyName("next_page")+")") != nil {
			a.cycleTab(1)
		}
		x += 20
	}
	if u.button(Rect{x + 2, top + 1, 22, h - 1}, "+", false, "New page ("+keyName("new_page")+")") != nil {
		a.addTab()
	}
	top += h
	fb.fill(Rect{0, top, fb.w, 1}, cEdgeDk)
	return top + 1
}

// drawBar draws the toolbar (and in the Focus layout the page's gadgets) and returns where the page starts.
func (a *App) drawBar(u *UI, top int) int {
	fb := u.fb
	W := fb.w
	fl := &flow{x0: 6, x: 6, y: top + 4, right: W - 6}
	if u.button(fl.place(buttonW("New")), "New", false, "New page in a new tab ("+keyName("new_page")+")") != nil {
		a.addTab()
	}
	if u.button(fl.place(buttonW("Open")), "Open", false, "Open a file in a new tab ("+keyName("open")+")") != nil {
		openOpenRequester()
	}
	if u.button(fl.place(buttonW("Save")), "Save", false, "Save ("+keyName("save")+")") != nil {
		a.save(false)
	}
	if u.button(fl.place(buttonW("Save as")), "Save as", false, "Save as / export ("+keyName("save_as")+")") != nil {
		a.save(true)
	}
	fl.gap()
	if u.button(fl.place(buttonW("Undo")), "Undo", false, "Undo ("+keyName("undo")+")") != nil {
		a.doUndo()
	}
	if u.button(fl.place(buttonW("Redo")), "Redo", false, "Redo ("+keyName("redo")+")") != nil {
		a.doRedo()
	}
	fl.gap()
	zooms := []string{"1x", "2x", "3x", "4x", "5x", "6x", "7x", "8x", "Fit"}
	u.choice("zoom", fl.place(choiceW(zooms)), zooms, a.zoom-1, "Zoom (Ctrl+wheel, "+keyName("zoom_in")+" / "+keyName("zoom_out")+"; "+keyName("fit_width")+" fits the width)", func(i int) {
		if i == 8 {
			a.fitZoom()
		} else {
			a.setZoom(i + 1)
		}
	})
	r := fl.place(checkW("Grid"))
	if v, _ := u.check(r.x, r.y, "Grid", a.grid, "Lines between the cells; brighter every 10 columns and every 25 rows ("+keyName("grid")+")"); v != a.grid {
		a.grid = v
		savePrefs()
	}
	a.refGadget(u, fl.place(buttonW("Ref")))
	layouts := []string{"One page", "Side by side", "Top / bottom"}
	ids := []string{"one", "cols", "rows"}
	lcur := 0
	for i, id := range ids {
		if id == a.layout {
			lcur = i
		}
	}
	u.choice("layout", fl.place(choiceW(layouts)), layouts, lcur, "Show one page, or two pages side by side or one over the other ("+keyName("layout")+")", func(i int) {
		a.setLayout(ids[i])
	})
	if a.layout != "one" {
		r = fl.place(checkW("Sync"))
		a.syncScroll, _ = u.check(r.x, r.y, "Sync", a.syncScroll, "Scroll both pages together")
	}
	if !a.studio() {
		a.drawPageGadgets(u, fl, false)
	}
	fl.gap()
	if u.button(fl.place(buttonW("Animate...")), "Animate...", false, "Animate: TheDraw's scan patterns and Charcoal's effects as the saved file's draw order or as frames ("+keyName("animate")+")") != nil {
		openAnimate()
	}
	var baudNames []string
	bcur := 2
	for i, b := range bauds {
		baudNames = append(baudNames, speedLabel(b))
		if b == a.baud {
			bcur = i
		}
	}
	u.choice("baud", fl.place(choiceW(baudNames)), baudNames, bcur, "Play speed: a caller's modem (bits per second), or telnet (only a board's pauses)", func(i int) {
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
	top = fl.y + gadH + 4
	fb.fill(Rect{0, top, W, 1}, cEdgeDk)
	return top + 1
}

// drawPageGadgets places the page's gadgets with fl: its machine, font,
// palette, size and look. compact (the side panel) lists the machines in one gadget.
func (a *App) drawPageGadgets(u *UI, fl *flow, compact bool) {
	fb := u.fb
	// In the panel each list is a row of its own with its name before it.
	labelW := textW("Resolution") + 10
	row := func(label string, w int) Rect {
		if !compact {
			return fl.place(w)
		}
		if fl.x > fl.x0 {
			fl.x, fl.y = fl.x0, fl.y+gadH+4
		}
		fb.text(fl.x0, fl.y+3, label, cText)
		r := Rect{fl.x0 + labelW, fl.y, fl.right - fl.x0 - labelW, gadH}
		fl.x = fl.right + 1 // what follows starts a new row
		return r
	}
	if !compact {
		fl.gap()
	}
	if compact { // the side panel: the machine in a list
		var names []string
		cur := 0
		for i, m := range modeOrder {
			names = append(names, modeNames[m])
			if a.mode == m && !a.start {
				cur = i
			}
		}
		u.choice("mode", row("Machine", choiceW(names)), names, cur, "The machine the page is for: its characters, colours, fonts and file format", func(i int) {
			a.start = false
			a.setMode(modeOrder[i])
		})
	} else {
		modeHint := "Amiga: Latin-1, Amiga fonts, 0x9B CSI. PC: CP437, VGA font, CRLF. Atari: ATASCII, .ata. ST: Atari ST VT52, .vt. PETSCII: PET, VIC-20, C64, C128, .seq. ZX: Spectrum screen, .scr. TT: teletext, .tti. CPC: Amstrad CPC, .bin. Uni: Unicode (terminals, IRC; a PC or Amiga page is converted, undoably), UTF-8 ANSI, .dur, .irc."
		for _, m := range modeOrder {
			label := modeNames[m]
			if m == "cpc" {
				label = "CPC"
			}
			if m == "uni" {
				label = "Uni"
			}
			if m == "st" {
				label = "ST"
			}
			if m == "zx" {
				label = "ZX"
			}
			if m == "tt" {
				label = "TT"
			}
			if u.button(fl.place(buttonW(label)), label, a.mode == m && !a.start, modeHint) != nil {
				a.start = false
				a.setMode(m)
			}
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
	for _, o := range []struct {
		f  *Font
		id string
	}{{a.ownFont, ownID}, {a.ownLower, ownLowerID}} { // the page's own font
		if o.f != nil && o.f.kind == a.mode {
			if a.font() == o.f {
				cur = len(fontIDs)
			}
			fontNames = append(fontNames, o.f.name)
			fontIDs = append(fontIDs, o.id)
		}
	}
	if a.c64() {
		var mNames []string
		mcur := 0
		for i, m := range machines {
			mNames = append(mNames, m.name)
			if m.id == a.machine {
				mcur = i
			}
		}
		u.choice("machine", row("Model", choiceW(mNames)), mNames, mcur, "The Commodore machine: its character set, colours and screen width", func(i int) {
			a.start = false
			a.setMachine(machines[i].id)
		})
		a.drawVICGadgets(u, row) // the C64's text modes (c64x.go)
		var keep []int
		for i, id := range fontIDs { // only this machine's two sets, and the page's own
			if strings.HasPrefix(id, a.mach().family) || strings.HasPrefix(id, ownID) {
				keep = append(keep, i)
			}
		}
		var n2, id2 []string
		cur2 := 0
		for _, i := range keep {
			if i == cur {
				cur2 = len(n2)
			}
			n2, id2 = append(n2, fontNames[i]), append(id2, fontIDs[i])
		}
		fontNames, fontIDs, cur = n2, id2, cur2
	}
	if len(fontNames) > 0 {
		if !a.st() && !a.tt() && !a.uni() && !a.cpc() { // fonts from files and the font editor (fontedit.go)
			fontNames = append(fontNames, "Load a font...", "Edit the font...")
			fontIDs = append(fontIDs, "+load", "+edit")
		}
		u.choice("font", row("Font", choiceW(fontNames)), fontNames, cur, "Font (on a PETSCII page: upper case + graphics, or lower + upper case); load one from a file, or edit it", func(i int) {
			switch fontIDs[i] {
			case "+load":
				openFontRequester()
				return
			case "+edit":
				openFontEditor()
				return
			}
			a.fontFor[a.mode] = fontIDs[i]
			savePrefs()
		})
	}
	var palNames, palIDs []string
	pcur := 0
	for _, p := range palettes {
		palNames, palIDs = append(palNames, p.name), append(palIDs, p.id)
	}
	if a.ownPal != nil {
		palNames, palIDs = append(palNames, "The file's own"), append(palIDs, "file")
	}
	for i, id := range palIDs {
		if id == a.paletteFor[a.mode] {
			pcur = i
		}
	}
	if !a.cpc() { // a CPC page's colours are its inks (the Colours)
		u.choice("palette", row("Palette", choiceW(palNames)), palNames, pcur, "Palette (Shift+click a colour below to change it)", func(i int) {
			a.paletteFor[a.mode] = palIDs[i]
			a.setPalette()
			savePrefs()
		})
	}
	if compact {
		row("Size", 0)
		fl.x = fl.x0 + labelW
	} else {
		fl.gap()
	}
	switch {
	case a.tt():
		r := fl.place(textW("40 x 25") + 8)
		fb.text(r.x+4, r.y+3, "40 x 25", cText)
		u.setHint(r, "A teletext page")
	case a.zx():
		r := fl.place(textW("32 x 24") + 8)
		fb.text(r.x+4, r.y+3, "32 x 24", cText)
		u.setHint(r, "The Spectrum's screen")
	case a.cpc():
		var names []string
		for _, m := range cpcModes {
			names = append(names, m.name)
		}
		u.choice("cpcmode", row("Mode", choiceW(names)), names, a.cpcSet.mode, "The CPC's screen mode: its width and how many inks (AMSCII compos: mode 1)", func(i int) {
			a.start = false
			a.setCPCMode(i)
		})
	case a.st():
		var names []string
		cur := 0
		for i, r := range stResolutions {
			names = append(names, r.name)
			if r.id == a.stres {
				cur = i
			}
		}
		u.choice("stres", row("Resolution", choiceW(names)), names, cur, "The ST's resolution: its width, colours and font", func(i int) {
			a.start = false
			a.setSTRes(stResolutions[i].id)
		})
	case a.c64(): // the machine's width, not to be changed
		label := fmt.Sprintf("%d wide", a.doc.cols)
		r := fl.place(textW(label) + 8)
		fb.text(r.x+4, r.y+3, label, cText)
		u.setHint(r, "The page is as wide as the machine's screen")
	case a.atari(): // the screen editor's 38 or a terminal's 40
		widths := []string{"38 columns", "40 columns"}
		u.choice("atariwidth", row("Width", choiceW(widths)), widths, map[bool]int{true: 1}[a.doc.cols == 40], "38: text made in the Atari's screen editor (it keeps a margin of 2); 40: terminal art", func(i int) {
			a.setCols([]int{38, 40}[i])
		})
	default:
		r := fl.place(textW("W") + 8 + digitsW(6) + 8)
		fb.text(r.x, r.y+3, "W", cText)
		if u.focus != fieldCols {
			fieldCols.set(strconv.Itoa(a.doc.cols))
		}
		u.field(Rect{r.x + textW("W") + 8, r.y, r.w - textW("W") - 8, r.h}, fieldCols, "Page width in columns (Enter sets it)")
	}
	rh := fl.place(textW("H") + 8 + digitsW(7) + 8)
	fb.text(rh.x, rh.y+3, "H", cText)
	if u.focus != fieldRows {
		fieldRows.set(strconv.Itoa(a.doc.rows))
	}
	u.field(Rect{rh.x + textW("H") + 8, rh.y, rh.w - textW("H") - 8, rh.h}, fieldRows, "Page height in rows (Enter sets it). Enter or Down past the last row adds one")
	var r Rect
	if a.amiga() {
		r = fl.place(checkW("8x8"))
		if v, _ := u.check(r.x, r.y, "8x8", a.ch8, "Amiga fonts at 8x8 (no line doubling)"); v != a.ch8 {
			a.ch8, a.ch8Default = v, v
			savePrefs()
		}
	}
	if a.pc() || a.amiga() {
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
}

// modeOrder is the machines in the order the gadgets show them.
var modeOrder = []string{"amiga", "pc", "atari", "st", "c64", "zx", "tt"}

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
	a.setZoom(max(1, min(8, a.page.w/(a.doc.cols*a.cellW()))))
	a.scrollX = 0
}

// ---------------------------------------------------------------- the page

func (a *App) contentSize() (int, int) {
	z, H := a.zoom, a.cellH()
	rows := a.doc.rows + 3 // the page and a little of what is below it, to see where it ends
	if viewer != nil {     // the viewer shows the art and no more
		rows = max(a.doc.rows, 1)
		if viewer.shrink > 1 {
			return a.doc.cols * a.cellW() / viewer.shrink, rows * H / viewer.shrink
		}
	}
	return a.doc.cols * a.cellW() * z, rows * H * z
}

func (a *App) drawPage(u *UI, id string) {
	fb, p := u.fb, a.page
	if a.start && viewer == nil {
		if a.untouched() {
			a.drawStart(u, Rect{p.x, p.y, p.w + 18, p.h + 18})
			return
		}
		a.start = false // something was put on the page or opened in it
	}
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
			a.scrollX += w * a.cellW() * z * 4
		} else {
			a.scrollY += w * H * z * 3
		}
	}
	a.scrollY = max(0, min(a.scrollY, max(0, ch-p.h)))
	a.scrollX = max(0, min(a.scrollX, max(0, cw-p.w)))

	old := fb.clip
	fb.clip = p.and(old)
	if viewer != nil && viewer.shrink > 1 { // art wider than the window, shown smaller
		fb.fill(p, cStartBack)
		px, w, h := viewer.shrunk()
		for y := 0; y < p.h && a.scrollY+y < h; y++ {
			src := (a.scrollY+y)*w + a.scrollX
			n := min(p.w, w-a.scrollX)
			if n > 0 {
				copy(fb.px[(p.y+y)*fb.w+p.x:(p.y+y)*fb.w+p.x+n], px[src:src+n])
			}
		}
		fb.clip = old
		return
	}
	fb.fill(p, cStartBack) // outside the page
	ox, oy := p.x-a.scrollX, p.y-a.scrollY
	rowPx, colPx := H*z, a.cellW()*z
	pageR := Rect{ox, oy, a.doc.cols * colPx, a.doc.rows * rowPx}
	fb.fill(pageR.and(p), cPaper)
	y0, y1 := a.scrollY/rowPx, min(a.doc.rows, (a.scrollY+p.h)/rowPx+1)
	x0, x1 := a.scrollX/colPx, min(a.doc.cols, (a.scrollX+p.w)/colPx+1)
	for y := y0; y < y1; y++ {
		if a.tt() { // a teletext row is read left to right as the chip reads it
			row := ttDecode(a.doc, y, a.ttBlack())
			for x := x0; x < x1; x++ {
				a.drawTTCell(fb, ox+x*colPx, oy+y*rowPx, z, row[x], a.showCodes)
			}
			continue
		}
		for x := x0; x < x1; x++ {
			a.drawCellXY(fb, ox+x*colPx, oy+y*rowPx, z, x, y, a.doc.get(x, y))
		}
	}
	a.drawRef(fb, ox, oy, x0, x1, y0, y1)
	a.drawOnion(fb, ox, oy, x0, x1, y0, y1)
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
		hx, hy := a.floatAt(a.hover.x, a.hover.y)
		a.paintCellsAt(fb, ox+hx*colPx, oy+hy*rowPx, z, func(x, y int) uint32 { return f.cells[y*f.w+x] }, f.w, f.h, hx, hy)
		fb.dashed(cell(hx, hy, f.w, f.h), cOrange)
	}
	if a.sel != nil {
		r := a.selRect()
		fb.blend(cell(r.x, r.y, r.w, r.h), cOrange, 46)
		fb.dashed(cell(r.x, r.y, r.w, r.h), cOrange)
	}
	if a.hover != nil && a.float == nil && a.tool != "type" && a.tool != "select" && a.playing == nil && viewer == nil {
		r := cell(a.hover.x, a.hover.y, 1, 1)
		halfTool := a.tool == "half" || (a.tool == "shape" && a.styleNow() == "half")
		if halfTool && a.tt() { // the sixel under the mouse
			r.w /= 2
			r.x += a.hover.sx * r.w
			top := []int{0, 6, 14, 20}
			r.y, r.h = r.y+top[a.hover.sy]*z, (top[a.hover.sy+1]-top[a.hover.sy])*z
		} else if halfTool {
			r.h /= 2
			r.y += a.hover.half * r.h
		}
		fb.frame(r, 0xFFCCCCCC)
	}
	if (a.blink && viewer == nil && a.playing == nil) || (a.playing != nil && a.playing.frames == nil) { // the cursor, Amiga style: an orange block
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
	fx := float64(mx-a.page.x+a.scrollX) / float64(a.cellW()*z)
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
	sx, sy := 0, 0 // teletext sixels: two columns, rows of 6, 8 and 6 of the cell's 20 lines
	if fx-float64(x) >= 0.5 {
		sx = 1
	}
	switch f := fy - float64(y); {
	case f >= 0.7:
		sy = 2
	case f >= 0.3:
		sy = 1
	}
	inside := fx >= 0 && x < a.doc.cols && fy >= 0 && y < a.doc.rows
	return Hover{max(0, min(a.doc.cols-1, x)), max(0, y), half, sx, sy}, inside
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
	if a.moving {
		return // a block being moved goes down where the held button is let go
	}
	if a.float != nil {
		if c.button == 1 {
			x0, y0 := a.floatAt(p.x, p.y)
			a.stamp(x0, y0, c.shift)
		} else if c.button == 3 {
			a.float = nil
		}
		return
	}
	if c.button == 1 && a.refPress(c.x, c.y) { // the reference picture is moved (Ctrl, or its move mode)
		a.button = c.button
		return
	}
	if c.button == 2 || a.tool == "pick" {
		a.pick(p.x, p.y)
		return
	}
	a.button = c.button
	a.strokeSeen = nil
	switch a.tool {
	case "type":
		a.cx, a.cy, a.sel = p.x, p.y, nil
		a.moved()
	case "select":
		if a.sel != nil && c.button == 1 && !c.shift && a.selRect().has(p.x, p.y) {
			a.liftSel(p.x, p.y) // a drag inside the selection moves it
			a.msg("Moving the block: let go to put it down, Esc puts it back")
			break
		}
		a.sel = &Sel{p.x, p.y, p.x, p.y}
		a.selecting = true
	case "fill":
		if c.button == 1 && a.doc.get(p.x, p.y) != a.brushCell() { // a fill that changes nothing is no step
			a.pushUndo()
			a.floodFill(p.x, p.y)
		}
	case "half":
		a.pushUndo()
		if a.tt() { // teletext: sixels on (left) or off (right)
			gx, gy := p.x*2+p.sx, p.y*3+p.sy
			if c.shift && a.last != nil {
				line(a.last.x, a.last.y, gx, gy, func(x, y int) { a.sixelSym(x, y, c.button != 3) })
			} else {
				a.sixelSym(gx, gy, c.button != 3)
			}
			a.last = &Hover{x: gx, y: gy}
			break
		}
		hy := p.y*2 + p.half
		col := a.halfFg()
		if c.button == 3 {
			col = a.halfBg()
		}
		if c.shift && a.last != nil {
			line(a.last.x, a.last.y, p.x, hy, func(x, y int) { a.halfSym(x, y, col) })
		} else {
			a.halfSym(p.x, hy, col)
		}
		a.last = &Hover{x: p.x, y: hy}
	case "shape":
		a.shapePress(p, c.button, c.shift)
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
	if a.playing != nil || a.start {
		return
	}
	p, _ := a.cellAt(mx, my)
	if a.button != 0 && p.y >= a.doc.rows { // dragging below the page stays on its last row: only Enter and Down add rows
		p.y, p.half, p.sy = max(0, a.doc.rows-1), 1, 2
	}
	over := a.page.has(mx, my) && a.dialog == nil
	switch {
	case over || a.button != 0:
		a.hover = &p
	case a.float == nil:
		a.hover = nil
	}
	if a.button == 0 || a.refMotion(mx, my) {
		return
	}
	switch {
	case a.tool == "shape":
		a.shapeMotion(p)
	case a.selecting:
		a.sel.x1, a.sel.y1 = p.x, p.y
	case a.tool == "half" && a.tt():
		gx, gy := p.x*2+p.sx, p.y*3+p.sy
		on := a.button != 3
		if a.last != nil {
			line(a.last.x, a.last.y, gx, gy, func(x, y int) { a.sixelSym(x, y, on) })
		}
		a.last = &Hover{x: gx, y: gy}
	case a.tool == "half":
		hy := p.y*2 + p.half
		col := a.halfFg()
		if a.button == 3 {
			col = a.halfBg()
		}
		if a.last != nil {
			line(a.last.x, a.last.y, p.x, hy, func(x, y int) { a.halfSym(x, y, col) })
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
	a.stripRelease()
	a.refRelease()
	a.shapeRelease()
	if a.moving && a.float != nil {
		a.dropMoved()
	}
	if a.selecting {
		a.selecting = false
		r := a.selRect()
		a.msg(fmt.Sprintf("%dx%d selected: %s copy, %s cut, Del clear; Mirror, Upside down, Turn, Invert under the colours", r.w, r.h, keyName("copy"), keyName("cut")))
	}
	a.button = 0
}

// ---------------------------------------------------------------- the bottom pane

func (a *App) paneHeight() int {
	chars := 8 * a.cellH()
	if a.uni() {
		chars += uniHeaderH() // the block switch above the characters
	}
	return max(max(chars, 5*(smallH+4))+14, paneNeed)
}

var smallH = lineH + 4

var tools = []struct{ id, name, hint string }{
	{"type", "Type", "Type: click to place the cursor, then type"},
	{"draw", "Draw", "Draw: left paints the brush, right erases, Shift+click draws a line"},
	{"colour", "Colour", "Colour: left paints the foreground, right the background, the characters stay"},
	{"fill", "Fill", "Fill: fills the area of equal cells with the brush"},
	{"pick", "Pick", "Pick: takes the character and colours of a cell (middle click does this with any tool)"},
	{"select", "Select", "Select: drag a block; copy, cut, paste, Del"},
	{"shape", "Shape", "Shape: drag a line, box or ellipse in the brush (the choices under the brush); Shift: filled; right button erases"},
	{"half", "Half", "Half blocks: paint pixels of half a cell (PC page; Sixel on a teletext page)"},
}

// drawPane draws the strip under the page (the Focus layout): tools, characters, F-keys and colours, kept small.
func (a *App) drawPane(u *UI, pane Rect) {
	fb := u.fb
	if pane.h <= 0 {
		return
	}
	fb.fill(Rect{pane.x, pane.y, pane.w, 1}, cEdgeDk)
	if a.playing != nil && !u.inert { // the page on show is the one being played: hands off
		u.inert = true
		defer func() { u.inert = false }()
	}
	old := fb.clip
	fb.clip = pane.and(old)
	x, y := pane.x+8, pane.y+7
	H := a.cellH()

	// Tools, two columns, and the style toggles under them.
	bw := buttonW("Colour")
	a.drawTools(u, x, y, bw, 2)
	x += 2*bw + 4 + 14

	cm := a.drawCharset(u, x, y, 32)
	x += cm.w + 14

	a.drawFkeys(u, x, y)

	// The colours in one row under them, then the brush.
	cy := y + H + lineH + 14
	a.drawColours(u, x, cy, 16)
	by := cy + 26
	bx := a.drawBrush(u, x, by)
	end := a.drawBlockOrToolOptions(u, bx, by+smallH+6, pane.x+pane.w-8)
	paneNeed = end + 7 - pane.y // a second row of options makes the strip taller from the next pass
	fb.clip = old
}

// paneNeed is the height the strip under the page last needed.
var paneNeed int

// pageGlyph draws a character of the page's font in the interface's text colour.
func (a *App) pageGlyph(fb *FB, x, y, c int) {
	fb.glyph(a.font(), x, y, byte(c), cText, 0, 1, 1, false)
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
	if n := a.nFrames(); n > 1 {
		pos += fmt.Sprintf("  Frame %d/%d", a.cur+1, n)
	}
	pos += a.orderNote()
	x = fb.text(x, y, pos, cText) + 24
	if a.hover != nil {
		v := a.doc.get(a.hover.x, a.hover.y)
		c := cellCh(v)
		x = fb.text(x, y, fmt.Sprintf("Mouse %d,%d: ", a.hover.x+1, a.hover.y+1), cText)
		if a.uni() {
			x = a.uniCellNote(fb, x, y, v) + 24
		} else if a.printable(c) {
			a.pageGlyph(fb, x, y, c)
		}
		if !a.uni() {
			x = fb.text(x+digitsW(2), y, fmt.Sprintf("%d fg %d bg %d", c, cellFg(v), cellBg(v)), cText) + 24
		}
	}
	set := map[string]string{"amiga": "Amiga Latin-1", "pc": "PC CP437", "c64": a.mach().name + " PETSCII", "atari": "Atari ATASCII", "st": a.res().name + " VT52", "zx": "ZX Spectrum", "tt": "Teletext", "uni": "Unicode"}[a.mode]
	if a.cpc() {
		set = fmt.Sprintf("Amstrad CPC mode %d", a.cpcSet.mode)
	}
	x = fb.text(x, y, fmt.Sprintf("%s, %s, %dx%d", set, a.font().name, a.doc.cols, a.doc.rows), cText) + 24
	name := filepath.Base(a.path) // the whole path is in the title bar and Info
	if a.path == "" {
		name = a.name
	}
	if a.dirty {
		name = "* " + name
	}
	x = fb.text(x, y, name, cText) + 24
	text, col := u.hint, uint32(cText)
	if text == "" && download.what != "" {
		text = "Downloading " + download.what + " ..."
	}
	if a.message != "" && time.Since(a.messageAt) < 4*time.Second {
		text, col = a.message, cOrange
	}
	fb.text(x, y, text, col)
}
