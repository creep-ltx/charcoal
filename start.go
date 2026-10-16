package main

// The start screen: shown in the page area when CharEdit starts with no files
// (and in a new tab): a new page for each machine, drawn in its own font and
// colours, the recent files, and Open. Typing, a tool or Esc lets it go and the
// page of the last used type is there to draw on.

import (
	"fmt"
	"os"
	"path/filepath"
)

// startTile is one "New page for" choice.
type startTile struct {
	label   string // the machine's name, drawn in its own font
	mode    string
	machine string // a PETSCII page's machine
	stres   string // an ST page's resolution
	fg      int    // the colour the name is drawn in
	colours int    // how many colours the swatch row shows (0: none)
}

// startTiles are the tiles, in the order shown. The PET and C128 tiles take
// the 80-column machine when that was the last one used, the ST tile the last
// used resolution.
func startTiles() []startTile {
	pet, c128 := "pet", "c128"
	if a.defMachine == "pet80" {
		pet = "pet80"
	}
	if a.defMachine == "c128vdc" {
		c128 = "c128vdc"
	}
	st := stResByID(a.defSTRes)
	return []startTile{
		{"Amiga", "amiga", "", "", 7, 8},
		{"PC", "pc", "", "", 7, 16},
		{"Atari 8-bit", "atari", "", "", 0, 0},
		{"Atari ST", "st", "", st.id, st.fg, st.colours},
		{"C64", "c64", "c64", "", 14, 16},
		{"PET", "c64", pet, "", 1, 0},
		{"VIC-20", "c64", "vic20", "", 6, 8},
		{"C128", "c64", c128, "", 13, 16},
		{"ZX Spectrum", "zx", "", "", 0, 8},
		{"Teletext", "tt", "", "", 7, 8},
		{"Amstrad CPC", "cpc", "", "", 1, 4},
		{"Minitel", "mt", "", "", 7, 8},
		{"Viewdata", "vd", "", "", 7, 8},
		{"Unicode", "uni", "", "", 7, 16},
	}
}

// ready: can a page of this type be had without a download?
func (tl startTile) ready() bool {
	switch tl.mode {
	case "c64":
		return hasROM(machineByID(tl.machine).family)
	case "st":
		return hasSTFont()
	}
	return true
}

// tab is an empty page of the tile's type, not in the tab list: the tile draws
// it, and its size is the size the new page will have.
func (tl startTile) tab() *Tab {
	t := a.makeTab(tl.mode)
	if tl.machine != "" {
		m := machineByID(tl.machine)
		t.machine = m.id
		t.paletteFor["c64"] = m.palette
		t.doc = newDoc(m.cols, m.rows)
	}
	if tl.stres != "" {
		t.stres = tl.stres
		t.doc = newSTDoc(stResByID(tl.stres))
	}
	return t
}

// pickTile makes the current (empty) tab a page of the tile's type, as the
// gadget bar's page type, machine and resolution gadgets would. A machine
// that needs a download asks for it first.
func (a *App) pickTile(tl startTile) {
	if !tl.ready() {
		t := a.Tab
		again := func() {
			if a.backTo(t) && tl.ready() {
				a.pickTile(tl)
			}
		}
		if tl.mode == "st" {
			openSTReq(again)
		} else {
			openC64Req(again)
		}
		return
	}
	// The empty tab makes way for a new page of the tile's type, made just as
	// the tile drew it (an empty Spectrum page is not blank to usedRows, so
	// setMode would keep its size).
	a.endPlay()
	old, t := a.Tab, tl.tab()
	a.tabs[a.tabIndex(old)] = t
	for p := range a.shown {
		if a.shown[p] == old {
			a.shown[p] = t
		}
	}
	a.Tab = nil
	a.focusTab(t)
	if tl.machine != "" {
		a.defMachine = tl.machine
	}
	if tl.stres != "" {
		a.defSTRes = tl.stres
	}
	switch {
	case tl.mode == "st":
		a.fg, a.bg = a.res().fg, a.res().bg
	case tl.mode == "zx":
		a.fg, a.bg = 0, 7
	case tl.mode == "cpc":
		a.fg, a.bg = 1, 0
	case tl.mode == "mt":
		a.fg, a.bg = 7, 0
	case old.mode == "st" || old.mode == "zx" || old.mode == "cpc":
		a.fg, a.bg = 7, 0
	}
	savePrefs()
	a.msg(fmt.Sprintf("New %s page, %d x %d", tl.label, a.doc.cols, a.doc.rows))
}

// startShownAtLaunch: CharEdit shows the start screen when it starts with no
// files, unless the person switched it off.
func startShownAtLaunch(files int) bool {
	return files == 0 && viewer == nil && !prefs.NoStart
}

// showStart shows the start screen: in this tab when nothing is on it yet,
// else in a new one.
func (a *App) showStart() {
	if !a.untouched() {
		a.focusTab(a.newTab(a.mode))
	}
	a.start, a.startSel = true, 0
}

// untouched: an untitled page nothing was done to, which the start screen
// may stand in for. (Not pristine: an empty Spectrum page has paper colours.)
func (t *Tab) untouched() bool {
	return t.path == "" && !t.dirty && t.source == nil && t.frames == nil && len(t.undo) == 0
}

// The start screen's items for the keyboard: the tiles, then the recent
// files, then Open.
func (a *App) startItems() (tiles, recent int) {
	return len(startTiles()), min(len(prefs.Recent), maxRecent)
}

// startKey handles a key on the start screen; false lets the editor have it
// (Ctrl and Alt shortcuts never come here). Keys that edit the page let the
// start screen go first.
func (a *App) startKey(k string, shift bool) bool {
	tiles, recent := a.startItems()
	n := tiles + recent + 1
	cols := max(1, a.startCols)
	i := max(0, min(a.startSel, n-1))
	switch k {
	case "Escape":
		a.start = false
		a.msg(modeNames[a.mode] + " page: " + keyName("start_screen") + " shows the start screen again")
		return true
	case "Tab":
		if shift {
			i = (i + n - 1) % n
		} else {
			i = (i + 1) % n
		}
	case "Left":
		if i < tiles && i%cols > 0 {
			i--
		}
	case "Right":
		if i < tiles && i%cols < cols-1 && i+1 < tiles {
			i++
		}
	case "Up":
		switch {
		case i < tiles:
			if i >= cols {
				i -= cols
			}
		case i == tiles: // the first recent file (or Open): up to the last row of tiles
			i = min(tiles-1, (tiles-1)/cols*cols+a.startCol)
		default:
			i--
		}
	case "Down":
		switch {
		case i < tiles:
			if i+cols < tiles {
				i += cols
			} else if i/cols == (tiles-1)/cols {
				a.startCol = i % cols
				i = tiles
			} else {
				i = tiles - 1
			}
		case i < n-1:
			i++
		}
	case "Return", "Keypad Enter":
		a.startActivate(i)
		return true
	case "Backspace", "Delete", "Insert", "Home", "End", "PageUp", "PageDown":
		a.start = false
		return false
	default:
		var fk int
		if n, _ := fmt.Sscanf(k, "F%d", &fk); n == 1 && fk >= 1 && fk <= 10 && keymap[combo{0, k}] == "" {
			a.start = false // an F-key that types a character (F12, the help, keeps it)
		}
		return false
	}
	if i < tiles {
		a.startCol = i % cols
	}
	a.startSel = i
	return true
}

// startActivate does what item i of the start screen does.
func (a *App) startActivate(i int) {
	tiles, recent := a.startItems()
	switch {
	case i < tiles:
		a.pickTile(startTiles()[i])
	case i < tiles+recent:
		a.openRecent(prefs.Recent[i-tiles])
	default:
		openOpenRequester()
	}
}

// openRecent opens a recent file from the start screen.
func (a *App) openRecent(p string) {
	if _, err := os.Stat(p); err != nil {
		a.msg("Could not open: " + err.Error())
		return
	}
	a.openPath(p, "auto")
}

const (
	tileW      = 196
	tileScreen = 64 // the height of a tile's little screen
)

var tileH = tileScreen + 12 + lineH + 8

// drawStart draws the start screen in r.
func (a *App) drawStart(u *UI, r Rect) {
	fb := u.fb
	old := fb.clip
	fb.clip = r.and(old)
	defer func() { fb.clip = old }()
	fb.fill(r, cStartBack)

	tiles := startTiles()
	_, recent := a.startItems()
	gap := 12
	cols := max(1, min(5, (r.w-48+gap)/(tileW+gap)))
	a.startCols = cols
	rows := (len(tiles) + cols - 1) / cols
	w := cols*tileW + (cols-1)*gap
	listH := max(1, recent)*(lineH+6) + 8
	h := lineH + 14 + rows*(tileH+gap) + 22 + lineH + 14 + listH + 22 + gadH
	x0 := r.x + max(24, (r.w-w)/2)
	y := r.y + max(20, (r.h-h)/2)

	fb.text(x0, y, "New page for", cHi)
	last := modeNames[a.mode]
	note := "Esc or typing: " + map[bool]string{true: "an", false: "a"}[last == "Amiga" || last == "Atari" || last == "Atari ST"] + " " + last + " page"
	fb.text(x0+w-textW(note), y, note, cDim)
	y += lineH + 14
	for i, tl := range tiles {
		tr := Rect{x0 + (i%cols)*(tileW+gap), y + (i/cols)*(tileH+gap), tileW, tileH}
		a.drawTile(u, tr, tl, i == a.startSel)
		if u.take(tr) != nil {
			a.startSel = i
			a.pickTile(tl)
		}
	}
	y += rows*(tileH+gap) + 22

	fb.text(x0, y+3, "Recent files", cHi)
	or := Rect{x0 + w - buttonW("Open...") - 2, y, buttonW("Open..."), gadH}
	if a.startSel == len(tiles)+recent {
		fb.frame(Rect{or.x - 2, or.y - 2, or.w + 4, or.h + 4}, cHi)
	}
	if u.button(or, "Open...", false, "Open a file ("+keyName("open")+")") != nil {
		a.startSel = len(tiles) + recent
		openOpenRequester()
	}
	y += gadH + 8
	if recent == 0 {
		fb.text(x0+8, y+3, "None yet", cDim)
	}
	for i := 0; i < recent; i++ {
		p := prefs.Recent[i]
		rr := Rect{x0, y + i*(lineH+6), w, lineH + 6}
		sel := a.startSel == len(tiles)+i
		switch {
		case sel:
			fb.fill(rr, cBlue)
		case u.hover(rr):
			fb.fill(rr, cGrey)
		}
		name, dir := filepath.Base(p), shownDir(filepath.Dir(p))
		col := uint32(cText)
		if sel {
			col = cHi
		}
		x := fb.text(rr.x+8, rr.y+3, name, col)
		room := rr.x + rr.w - 8 - (x + 16)
		if room > textW("....") {
			if textW(dir) > room {
				rs := []rune(dir)
				i := 0
				for i < len(rs) && textW(string(rs[i:])) > room-textW("...") {
					i++
				}
				dir = "..." + string(rs[i:])
			}
			fb.text(x+16, rr.y+3, dir, map[bool]uint32{true: cText, false: cDim}[sel])
		}
		u.setHint(rr, p)
		if u.take(rr) != nil {
			a.startSel = len(tiles) + i
			a.openRecent(p)
			return
		}
	}
	y += listH + 22

	fb.fill(Rect{x0, y - 12, w, 1}, cEdgeLt)
	if v, _ := u.check(x0, y, "Show this at start", !prefs.NoStart, "Off: CharEdit starts on an empty page of the last used type ("+keyName("start_screen")+" shows this screen)"); v == prefs.NoStart {
		prefs.NoStart = !v
		savePrefs()
	}
	right := "Drop a file here to open it   CharEdit " + version
	fb.text(x0+w-textW(right), y+3, right, cDim)
}

// drawTile draws one "New page for" tile: a little screen with the machine's
// name in its own font and colours and a row of its colours, then its size.
func (a *App) drawTile(u *UI, r Rect, tl startTile, sel bool) {
	fb := u.fb
	bg := uint32(cGrey)
	if sel {
		bg = cBlue
	} else if u.hover(r) {
		bg = cEdgeLt
	}
	fb.fill(r, bg)
	fb.bevel(r, false)
	scr := Rect{r.x + 6, r.y + 6, r.w - 12, tileScreen}
	t := tl.tab()
	size := fmt.Sprintf("%d x %d", t.doc.cols, t.doc.rows)
	if !tl.ready() {
		fb.fill(scr, cEdgeDk)
		fb.text(scr.x+(scr.w-textW(tl.label))/2, scr.y+scr.h/2-lineH-1, tl.label, cText)
		fb.text(scr.x+(scr.w-textW("font: a download"))/2, scr.y+scr.h/2+3, "font: a download", cDim)
	} else {
		keep := a.Tab
		a.Tab = t
		a.applyLook()
		paper := a.paperColour()
		fb.fill(scr, paper)
		cw, ch := a.cellW(), a.cellH()
		z := 1
		if ch <= 10 { // 8 rows, or the Minitel's 10
			z = 2
		}
		var cells []int
		for _, c := range tl.label {
			if b := a.charToByte(c); b >= 0 {
				cells = append(cells, b)
			}
		}
		tw := len(cells) * cw * z
		if tw > scr.w-8 { // too wide at 2x: drawn at 1x
			z = 1
			tw = len(cells) * cw
		}
		ty := scr.y + 12
		if tl.colours == 0 {
			ty = scr.y + (scr.h-ch*z)/2
		}
		tx := scr.x + (scr.w-tw)/2
		for i, b := range cells {
			if a.tt() {
				a.drawTTCell(fb, tx+i*cw*z, ty, z, ttCell{code: b, fg: tl.fg, control: -1}, false)
				continue
			}
			if a.mt() {
				a.drawMTCell(fb, tx+i*cw*z, ty, z, mtShow{code: b, fg: tl.fg}, false)
				continue
			}
			a.drawCell(fb, tx+i*cw*z, ty, z, makeCell(b, tl.fg, a.paperIndex(), 0))
		}
		if tl.colours > 0 { // the machine's colours, small
			sw, sh := 8, 6
			n := tl.colours
			x := scr.x + (scr.w-n*(sw+2)+2)/2
			for i := 0; i < n; i++ {
				fb.fill(Rect{x + i*(sw+2), scr.y + scr.h - sh - 12, sw, sh}, a.pal[i])
			}
		}
		a.Tab = keep
		a.applyLook()
	}
	if sel {
		fb.frame(scr, cHi)
	} else {
		fb.frame(scr, cEdgeDk)
	}
	ly := scr.y + scr.h + 8
	col := uint32(cText)
	if sel {
		col = cHi
	}
	fb.text(r.x+8, ly, tl.label, col)
	fb.text(r.x+r.w-8-textW(size), ly, size, map[bool]uint32{true: cText, false: cDim}[sel])
	hint := "A new " + tl.label + " page, " + size
	if !tl.ready() {
		hint = "A new " + tl.label + " page: its font is downloaded first"
	}
	u.setHint(r, hint)
}

// paperIndex is the colour number an empty cell of the page shows.
func (a *App) paperIndex() int {
	switch {
	case a.screen():
		return a.doc.bg
	case a.tt():
		return 0
	}
	return cellBg(a.doc.cells[0]) & 15
}

// paperColour is the colour an empty page shows.
func (a *App) paperColour() uint32 {
	if a.atari() {
		return a.atariRGB(a.doc.bg)
	}
	i := a.paperIndex()
	if !a.screen() && !a.st() && !a.zx() && !a.ice {
		i &= 7
	}
	return a.pal[i]
}
