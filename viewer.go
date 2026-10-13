package main

// CharView: the same program started as "charview" (or "charedit --view"),
// showing only the art, one file after the other.

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type Viewer struct {
	files   []string
	i       int
	zoomSet bool     // the zoom was chosen by hand: keep it from file to file
	fit     bool     // fit the next file to the window once it is laid out
	info    bool     // the line at the bottom
	auto    bool     // scrolling by itself
	step    bool     // a text row at a time (as a terminal does) instead of smoothly
	speed   float64  // rows a second
	pos     float64  // where the smooth scroll is, in pixels
	acc     float64  // part of a row waiting to be stepped
	endWait float64  // seconds spent at the end of the art
	shrink  int      // art wider than the window is shown 1/shrink of its size
	cache   struct { // the shrunk art, drawn once
		doc    *Doc
		shrink int
		key    string
		px     []uint32
		w, h   int
	}
}

// The scroll speeds the [ and ] keys step through, in rows a second.
var scrollSpeeds = []float64{0.5, 1, 2, 3, 4, 6, 8, 12, 16, 24, 32, 48}

var viewer *Viewer

func wantViewer() bool {
	if len(os.Args) > 1 && os.Args[1] == "--view" {
		os.Args = append(os.Args[:1], os.Args[2:]...)
		return true
	}
	return strings.HasPrefix(strings.ToLower(filepath.Base(os.Args[0])), "charview")
}

var artExt = regexp.MustCompile(`(?i)\.(ans|ansi|asc|nfo|diz|txt|bin|seq|pet|ice|vt|cia|drk)$`)

// viewerFiles turns the arguments into files; a folder gives the art files in it.
func viewerFiles(args []string) []string {
	var out []string
	for _, arg := range args {
		p, _ := filepath.Abs(arg)
		st, err := os.Stat(p)
		if err != nil || !st.IsDir() {
			out = append(out, p)
			continue
		}
		ents, _ := os.ReadDir(p)
		var names []string
		for _, e := range ents {
			if !e.IsDir() && !strings.HasPrefix(e.Name(), ".") && artExt.MatchString(e.Name()) {
				names = append(names, e.Name())
			}
		}
		sort.Slice(names, func(i, j int) bool { return strings.ToLower(names[i]) < strings.ToLower(names[j]) })
		for _, n := range names {
			out = append(out, filepath.Join(p, n))
		}
	}
	return out
}

// show puts file i on the screen, in a fresh page.
func (v *Viewer) show(i int) {
	if len(v.files) == 0 {
		return
	}
	v.i = (i%len(v.files) + len(v.files)) % len(v.files)
	if a.playing != nil {
		a.stopPlay(true)
	}
	a.tabs, a.Tab = nil, nil
	a.focusTab(a.newTab("amiga"))
	p := v.files[v.i]
	b, err := os.ReadFile(p)
	if err != nil {
		a.name = filepath.Base(p)
		a.msg("Could not read: " + err.Error())
		return
	}
	a.loadBytes(b, filepath.Base(p), p, "auto")
	if !v.zoomSet {
		v.fit = true
	}
	v.pos, v.acc, v.endWait = 0, 0, 0
}

// tick moves an auto-scroll on by dt seconds; it reports whether anything moved.
// Tall art scrolls down, wide art (if its height fits) sideways; at the end it
// waits two seconds and goes to the next file.
func (v *Viewer) tick(dt float64) bool {
	if !v.auto || a.dialog != nil || a.playing != nil {
		return false
	}
	cw, ch := a.contentSize()
	horiz := ch <= a.page.h && cw > a.page.w
	unit := v.rowPx() // a text row
	cur, end := a.scrollY, ch-a.page.h
	if horiz {
		unit, cur, end = max(1, 8*a.zoom/max(1, v.shrink)), a.scrollX, cw-a.page.w
	}
	if end <= 0 || cur >= end {
		v.endWait += dt
		if v.endWait >= 2 {
			v.show(v.i + 1)
			return true
		}
		return false
	}
	if int(v.pos) != cur { // scrolled by hand meanwhile
		v.pos = float64(cur)
	}
	if v.step {
		v.acc += v.speed * dt
		for v.acc >= 1 {
			v.acc--
			cur += unit
		}
		v.pos = float64(cur)
	} else {
		v.pos += v.speed * float64(unit) * dt
		cur = int(v.pos)
	}
	cur = min(cur, end)
	if horiz {
		a.scrollX = cur
	} else {
		a.scrollY = cur
	}
	return true
}

func (v *Viewer) setSpeed(dir int) {
	i := 0
	for i < len(scrollSpeeds)-1 && scrollSpeeds[i] < v.speed {
		i++
	}
	i = max(0, min(len(scrollSpeeds)-1, i+dir))
	v.speed = scrollSpeeds[i]
	saveViewerPrefs()
}

func (v *Viewer) scrollText() string {
	if !v.auto {
		return ""
	}
	how := "smooth"
	if v.step {
		how = "step"
	}
	return fmt.Sprintf("auto %s %g rows/s", how, v.speed)
}

func (a *App) drawViewer(u *UI) {
	fb := u.fb
	W, H := fb.w, fb.h
	fb.fill(Rect{0, 0, W, H}, 0xFF141518)
	bottom := H
	if viewer.info {
		bottom = H - lineH - 6
	}
	a.page = Rect{0, 0, W - 18, bottom - 18}
	if viewer.fit {
		viewer.fit = false
		a.zoom = max(1, min(8, a.page.w/(a.doc.cols*8)))
		viewer.shrink = 1
		if w := a.doc.cols * 8; w > a.page.w { // wider than the window: shown smaller
			viewer.shrink = min(8, (w+a.page.w-1)/a.page.w)
		}
		a.scrollX, a.scrollY = 0, 0
	}
	a.drawPage(u, "0")
	if !viewer.info {
		return
	}
	r := Rect{0, bottom, W, H - bottom}
	fb.fill(r, cDark)
	x := fb.text(8, r.y+4, fmt.Sprintf("%d/%d", viewer.i+1, len(viewer.files)), cHi) + 16
	x = fb.text(x, r.y+4, a.name, cHi) + 16
	x = fb.text(x, r.y+4, fmt.Sprintf("%s, %s, %dx%d", modeNames[a.mode], a.font().name, a.doc.cols, max(a.doc.usedRows(), 1)), cText) + 24
	if st := viewer.scrollText(); st != "" {
		x = fb.text(x, r.y+4, st, cHi) + 16
	}
	text, col := "n next  p prev  a auto-scroll  s smooth/step  [ ] speed  m play  e edit  +/- zoom  g grid  i hide  q quit", uint32(cText)
	if a.message != "" {
		text, col = a.message, cOrange
	}
	fb.text(x, r.y+4, text, col)
}

// rowPx is the height of a text row on the screen.
func (v *Viewer) rowPx() int {
	if v.shrink > 1 {
		return max(1, a.cellH()/v.shrink)
	}
	return a.cellH() * a.zoom
}

func viewerKey(k string, ctrl, shift bool) {
	v := viewer
	rowPx := v.rowPx()
	switch {
	case shift && k == "Right": // wide art: sideways by hand
		a.scrollX += 8 * a.zoom * 4
		return
	case shift && k == "Left":
		a.scrollX = max(0, a.scrollX-8*a.zoom*4)
		return
	}
	switch k {
	case "N", "Right", "Space":
		v.show(v.i + 1)
	case "P", "Left", "Backspace":
		v.show(v.i - 1)
	case "A":
		v.auto, v.endWait = !v.auto, 0
		saveViewerPrefs()
	case "S":
		v.step = !v.step
		saveViewerPrefs()
	case "]", "Keypad *":
		v.setSpeed(1)
	case "[", "Keypad /":
		v.setSpeed(-1)
	case "Down":
		a.scrollY += rowPx
	case "Up":
		a.scrollY -= rowPx
	case "PageDown":
		a.scrollY += a.page.h - rowPx
	case "PageUp":
		a.scrollY -= a.page.h - rowPx
	case "Home":
		a.scrollY = 0
	case "End":
		a.scrollY = a.doc.usedRows() * rowPx
	case "=", "+", "Keypad +":
		v.zoomSet = true
		if v.shrink > 1 {
			v.setShrink(v.shrink - 1)
		} else {
			a.zoomAt(a.zoom+1, a.page.x+a.page.w/2, a.page.y)
		}
	case "-", "Keypad -":
		v.zoomSet = true
		if a.zoom > 1 {
			a.zoomAt(a.zoom-1, a.page.x+a.page.w/2, a.page.y)
		} else {
			v.setShrink(v.shrink + 1)
		}
	case "0", "F":
		v.zoomSet, v.fit = false, true
	case "G":
		a.grid = !a.grid
	case "I":
		v.info = !v.info
	case "M":
		a.play()
	case "E":
		if exe, err := os.Executable(); err == nil && a.path != "" {
			if err := exec.Command(exe, a.path).Start(); err != nil {
				a.msg("Could not start CharEdit: " + err.Error())
			} else {
				a.msg("Opened in CharEdit")
			}
		}
	case "Escape", "Q":
		a.quit = true
	}
	a.scrollY = max(0, a.scrollY)
}

// saveViewerPrefs keeps the viewer's scroll settings; the editor's stay as they are.
func saveViewerPrefs() {
	prefs.ViewAuto, prefs.ViewStep, prefs.ViewSpeed = viewer.auto, viewer.step, viewer.speed
	path := prefsPath()
	if path == "" {
		return
	}
	var p Prefs
	if b, err := os.ReadFile(path); err == nil {
		json.Unmarshal(b, &p)
	}
	p.ViewAuto, p.ViewStep, p.ViewSpeed = viewer.auto, viewer.step, viewer.speed
	os.MkdirAll(filepath.Dir(path), 0o755)
	if b, err := json.MarshalIndent(p, "", "  "); err == nil {
		writeAtomic(path, append(b, '\n'))
	}
}

// setShrink changes how much smaller the art is shown, keeping the top in place.
func (v *Viewer) setShrink(n int) {
	n = max(1, min(8, n))
	if v.shrink < 1 {
		v.shrink = 1
	}
	a.scrollY = a.scrollY * v.shrink / n
	a.scrollX = a.scrollX * v.shrink / n
	v.shrink = n
}

// shrunk gives the art drawn at full size and then averaged down by v.shrink,
// made once and kept until the art, the font or the colours change.
func (v *Viewer) shrunk() ([]uint32, int, int) {
	c := &v.cache
	key := fmt.Sprintf("%s %v %v %d", a.font().id, a.pal, a.ice, a.cellH())
	if c.doc == a.doc && c.shrink == v.shrink && c.key == key && a.playing == nil {
		return c.px, c.w, c.h
	}
	s, H := v.shrink, a.cellH()
	rows := max(a.doc.usedRows(), 1)
	full := &FB{}
	full.resize(a.doc.cols*8, rows*H)
	a.paintCells(full, 0, 0, 1, func(x, y int) uint32 { return a.doc.get(x, y) }, a.doc.cols, rows)
	w, h := full.w/s, full.h/s
	px := make([]uint32, w*h)
	n := uint32(s * s)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var r, g, b uint32
			for dy := 0; dy < s; dy++ {
				row := full.px[(y*s+dy)*full.w+x*s:]
				for dx := 0; dx < s; dx++ {
					p := row[dx]
					r, g, b = r+(p>>16&255), g+(p>>8&255), b+(p&255)
				}
			}
			px[y*w+x] = 0xFF000000 | (r/n)<<16 | (g/n)<<8 | b/n
		}
	}
	c.doc, c.shrink, c.key, c.px, c.w, c.h = a.doc, s, key, px, w, h
	return px, w, h
}
