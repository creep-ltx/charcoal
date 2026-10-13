package main

// CharView: the same program started as "charview" (or "charcool --view"),
// showing only the art, one file after the other.

import (
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
	zoomSet bool // the zoom was chosen by hand: keep it from file to file
	fit     bool // fit the next file to the window once it is laid out
	info    bool // the line at the bottom
}

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
	text, col := "n next  p previous  m play  e edit  +/- zoom  g grid  i hide this line  q quit", uint32(cText)
	if a.message != "" {
		text, col = a.message, cOrange
	}
	fb.text(x, r.y+4, text, col)
}

func viewerKey(k string, ctrl bool) {
	v := viewer
	rowPx := a.cellH() * a.zoom
	switch k {
	case "N", "Right", "Space":
		v.show(v.i + 1)
	case "P", "Left", "Backspace":
		v.show(v.i - 1)
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
		a.zoomAt(a.zoom+1, a.page.x+a.page.w/2, a.page.y)
	case "-", "Keypad -":
		v.zoomSet = true
		a.zoomAt(a.zoom-1, a.page.x+a.page.w/2, a.page.y)
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
				a.msg("Could not start CharCool: " + err.Error())
			} else {
				a.msg("Opened in CharCool")
			}
		}
	case "Escape", "Q":
		a.quit = true
	}
	a.scrollY = max(0, a.scrollY)
}
