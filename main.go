// Charcoal: CharEdit and CharView, a character art editor and viewer for Amiga ANSI, PC ANSI, Atari
// ATASCII, Atari ST VT52, Commodore PETSCII (PET, VIC-20, C64, C128), ZX Spectrum and teletext pages,
// drawn with the real bitmap fonts.
//
//	charedit [FILE...]
//	charview FILE|FOLDER...
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unsafe"

	"github.com/veandco/go-sdl2/sdl"
)

var (
	ui       = &UI{fb: &FB{}}
	window   *sdl.Window
	renderer *sdl.Renderer
	texture  *sdl.Texture
	texW     int
	texH     int
	scale    = 1
	pxScale  = 1 // screen pixels per frame buffer pixel: scale times 2 on a Retina screen
)

// maxUIScale is the biggest window scale (Settings offers 1x to this).
const maxUIScale = 4

func setClipboard(s string) { sdl.SetClipboardText(s) }
func getClipboard() string {
	s, _ := sdl.GetClipboardText()
	return s
}
func shiftHeld() bool { return sdl.GetModState()&sdl.KMOD_SHIFT != 0 }

// dropFinderArgs drops the -psn_ argument older macOS versions give a program
// started from the Finder.
func dropFinderArgs() {
	out := os.Args[:1]
	for _, s := range os.Args[1:] {
		if !strings.HasPrefix(s, "-psn_") {
			out = append(out, s)
		}
	}
	os.Args = out
}

// version is Charcoal's version: X.YbN is a beta, X.Y a release. Tag releases as vX.YbN.
const version = "0.1b13"

func main() {
	runtime.LockOSThread()
	dropFinderArgs()
	if wantViewer() {
		viewer = &Viewer{info: true}
	}
	if viewer != nil && len(os.Args) > 1 && (os.Args[1] == "-h" || os.Args[1] == "--help") {
		fmt.Println("usage: charview FILE|FOLDER...\nShows Amiga and PC ANSI, Atari, Atari ST, PETSCII (PET, VIC-20, C64, C128), ZX Spectrum, Amstrad CPC and teletext art.\nn / Right / Space next, p / Left previous, a auto-scroll, m play, e edit, q / Esc quit.")
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Println("Charcoal " + version)
		return
	}
	if len(os.Args) > 1 && (os.Args[1] == "-h" || os.Args[1] == "--help") {
		fmt.Println("usage: charedit [FILE...]\nA character art editor for Amiga and PC ANSI, Atari, Atari ST, PETSCII (PET, VIC-20, C64, C128),\nZX Spectrum, Amstrad CPC and teletext pages. Help > Keys and mouse (F12) in the window lists the keys.")
		fmt.Println("\n" + animateUsage)
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "--animate" { // Animate a folder, no window (batch.go)
		headlessApp()
		os.Exit(animateCLI(os.Args[2:], os.Stdout, os.Stderr))
	}
	a = &App{tool: "draw", fg: 7, brush: 0x2F, charHover: -1, layout: "one"}
	a.hasC64 = loadC64Fonts()
	loadSTFonts()
	loadAtariROMFonts()
	loadMSXROMFont()
	loadMZROMFonts()
	loadPrefs()
	loadKeys()
	useTheme(prefs.Theme)
	applyUIFont()
	mode := prefs.Mode // the last page type, if it can be had without a download
	switch {
	case mode == "c64" && a.hasC64, mode == "st" && hasSTFont():
	case mode == "pc", mode == "atari", mode == "zx", mode == "tt", mode == "cpc", mode == "mt", mode == "vd", mode == "msx", mode == "mz":
	case mode == "uni":
	default:
		mode = "amiga"
	}
	a.ch8Default, a.smearDefault = prefs.Ch8, prefs.Smear
	a.focusTab(a.newTab(mode))
	if viewer != nil {
		viewer.auto, viewer.step, viewer.speed = prefs.ViewAuto, prefs.ViewStep, prefs.ViewSpeed
		if viewer.speed <= 0 {
			viewer.speed = 4
		}
		viewer.files = viewerFiles(os.Args[1:])
		if len(viewer.files) == 0 {
			fmt.Fprintln(os.Stderr, "usage: charview FILE|FOLDER...")
			// Started by a double click there is no terminal to read that in.
			sdl.ShowSimpleMessageBox(sdl.MESSAGEBOX_INFORMATION, "CharView",
				"CharView shows art files one after the other. Drop files or a folder on it, or open art files with it.", nil)
			os.Exit(1)
		}
		viewer.show(0)
	}
	for _, arg := range os.Args[1:] {
		if viewer != nil {
			break
		}
		p, _ := filepath.Abs(arg)
		if _, err := os.Stat(p); err == nil {
			a.openPath(p, "auto")
		} else {
			if !a.pristine() {
				a.focusTab(a.newTab(a.mode))
			}
			a.path, a.name = p, filepath.Base(p)
			if seqExt.MatchString(p) && a.hasC64 {
				a.setMode("c64")
			}
			a.msg("New file: " + p)
		}
	}
	if len(a.tabs) > 0 {
		a.focusTab(a.tabs[0])
	}
	if viewer == nil && a.dialog == nil {
		offerAutosaves() // pages a CharEdit left unsaved when it stopped
	}
	a.start = startShownAtLaunch(len(os.Args) - 1)
	if !a.hasC64 && !prefs.C64Asked && a.dialog == nil && viewer == nil {
		openC64Req(nil) // ask once, on the first run
	}

	beforeSDL() // Windows: draw in the screen's own pixels (no blurry stretching)
	if err := sdl.Init(sdl.INIT_VIDEO); err != nil {
		fmt.Fprintln(os.Stderr, "charedit:", err)
		os.Exit(1)
	}
	defer sdl.Quit()
	sdl.SetHint(sdl.HINT_RENDER_SCALE_QUALITY, "0")
	sdl.SetHint(sdl.HINT_VIDEO_X11_NET_WM_BYPASS_COMPOSITOR, "0")
	scale = max(1, min(maxUIScale, prefs.UIScale))
	if prefs.UIScale == 0 {
		scale = firstScale()
	}
	w, h := 1400, 940
	if prefs.Width > 200 && prefs.Height > 200 {
		w, h = prefs.Width, prefs.Height
	} else if dm, err := sdl.GetDesktopDisplayMode(0); err == nil {
		w, h = min(w, int(dm.W)-40), min(h, int(dm.H)-80)
	}
	var err error
	window, err = sdl.CreateWindow("CharEdit "+version, sdl.WINDOWPOS_CENTERED, sdl.WINDOWPOS_CENTERED, int32(w), int32(h),
		sdl.WINDOW_RESIZABLE|sdl.WINDOW_ALLOW_HIGHDPI)
	if err != nil {
		fmt.Fprintln(os.Stderr, "charedit:", err)
		os.Exit(1)
	}
	window.SetMinimumSize(640, 480)
	renderer, err = sdl.CreateRenderer(window, -1, sdl.RENDERER_ACCELERATED)
	if err != nil {
		renderer, err = sdl.CreateRenderer(window, -1, sdl.RENDERER_SOFTWARE)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "charedit:", err)
		os.Exit(1)
	}
	sdl.StartTextInput()
	imeAt = func(r Rect) { // an input method's window next to the cursor (window points)
		sdl.SetTextInputRect(&sdl.Rect{X: int32(r.x * scale), Y: int32(r.y * scale), W: int32(r.w * scale), H: int32(r.h * scale)})
	}
	sdl.EventState(sdl.DROPFILE, sdl.ENABLE)

	nextBlink := time.Now().Add(500 * time.Millisecond)
	lastTick := time.Now()
	shownMsg := ""
	redraw := true
	for !a.quit {
		wait := time.Until(nextBlink)
		if a.playing != nil || len(script) > 0 || (viewer != nil && viewer.auto) || dialogAnimates() {
			wait = 15 * time.Millisecond
		} else if download.what != "" {
			wait = min(wait, 100*time.Millisecond) // look for the end of the download
		}
		if collabActive() {
			wait = min(wait, 30*time.Millisecond) // the drawing session's messages (collab.go)
		}
		ev := sdl.WaitEventTimeout(max(1, int(wait/time.Millisecond)))
		for ; ev != nil; ev = sdl.PollEvent() {
			if handleEvent(ev) {
				redraw = true
			}
		}
		if time.Now().After(nextBlink) {
			a.blink = !a.blink
			nextBlink = time.Now().Add(500 * time.Millisecond)
			redraw = true
		}
		if pollDownload() {
			redraw = true
		}
		if a.playing != nil {
			a.playStep()
			redraw = true
		}
		if dialogAnimates() { // the Animate requester's preview
			redraw = true
		}
		if viewer != nil {
			now := time.Now()
			if viewer.tick(now.Sub(lastTick).Seconds()) {
				redraw = true
			}
			lastTick = now
		}
		if a.message != "" && time.Since(a.messageAt) > 4*time.Second && shownMsg == a.message {
			a.message = ""
			redraw = true
		}
		shownMsg = a.message
		if scriptStep() {
			redraw = true
		}
		a.autosaveTick(time.Now())
		if a.collabTick() {
			redraw = true
		}
		if redraw {
			pass()
			present()
			redraw = false
		}
	}
	for _, t := range a.tabs { // a clean quit: nothing is left to offer back
		dropAutosave(t)
	}
	savePrefs()
}

// pass lays out and draws the whole window once, handing the pending click to whoever is under it.
func pass() {
	// On a Retina screen the renderer has twice the window's points: draw at
	// twice the scale there, so it is sharp; the mouse stays in points / scale.
	ww, wh := window.GetSize()
	ow, oh, err := renderer.GetOutputSize()
	if err != nil || ow <= 0 || ww <= 0 {
		ow, oh = ww, wh
	}
	pxScale = scale * max(1, int((ow+ww/2)/ww))
	ui.fb.resize(int(ow)/pxScale, int(oh)/pxScale)
	ui.hint, ui.wheelTaken = "", false
	ui.inert = a.dialog != nil
	need := paneNeed
	a.drawWindow(ui)
	if paneNeed != need { // the strip under the page wants another height: lay it out again at once
		ui.hint = ""
		a.drawWindow(ui)
	}
	ui.inert = false
	if a.dialog != nil {
		if _, fe := a.dialog.(*FontEd); !fe { // the font editor leaves the page bright: it shows the font live
			ui.fb.blend(Rect{0, 0, ui.fb.w, ui.fb.h}, cEdgeDk, 120)
		}
		a.dialog.draw(ui)
		if ui.popup == nil {
			ui.drawTooltip() // a requester covers the status line, where hints go
		}
	}
	ui.drawPopup()
	ui.click, ui.wheel = nil, 0
	title := a.name + " - CharEdit"
	if a.path != "" { // the folder too: the status line shows only the name
		title = a.name + " - " + shownDir(filepath.Dir(a.path)) + " - CharEdit"
	}
	if viewer != nil {
		title = fmt.Sprintf("%s (%d/%d) - CharView", a.name, viewer.i+1, len(viewer.files))
	}
	if a.dirty && viewer == nil {
		title = "* " + title
	}
	if window.GetTitle() != title {
		window.SetTitle(title)
	}
}

func present() {
	fb := ui.fb
	if texture == nil || texW != fb.w || texH != fb.h {
		if texture != nil {
			texture.Destroy()
		}
		var err error
		texture, err = renderer.CreateTexture(sdl.PIXELFORMAT_ARGB8888, sdl.TEXTUREACCESS_STREAMING, int32(fb.w), int32(fb.h))
		if err != nil {
			fmt.Fprintln(os.Stderr, "charedit:", err)
			os.Exit(1)
		}
		texW, texH = fb.w, fb.h
	}
	texture.Update(nil, unsafe.Pointer(&fb.px[0]), fb.w*4)
	renderer.SetDrawColor(0, 0, 0, 255)
	renderer.Clear()
	renderer.Copy(texture, nil, &sdl.Rect{X: 0, Y: 0, W: int32(fb.w * pxScale), H: int32(fb.h * pxScale)})
	renderer.Present()
}

func requestQuit() {
	if n := a.anyDirty(); n > 0 {
		text := "A page has changes that are not saved. Save them before quitting?"
		if n > 1 {
			text = fmt.Sprintf("%d pages have changes that are not saved. Save them before quitting?", n)
		}
		a.dialog = &Confirm{text: text, button: "Quit, throw away", yes: func() { a.quit = true }, save: saveAllThenQuit}
		return
	}
	a.quit = true
}

// saveAllThenQuit saves every changed page, one after the other (asking for a
// name where a page has none), and quits once all are saved. A page that
// cannot be saved stops it, with the reason on the status line.
func saveAllThenQuit() {
	for _, t := range a.tabs {
		if t.dirty {
			a.focusTab(t)
			a.saveThen(saveAllThenQuit)
			return
		}
	}
	a.quit = true
}

// handleEvent returns true when the window needs drawing again.
func handleEvent(ev sdl.Event) bool {
	switch e := ev.(type) {
	case *sdl.QuitEvent:
		requestQuit()
	case *sdl.WindowEvent:
		if e.Event == sdl.WINDOWEVENT_LEAVE {
			ui.mx, ui.my = -1, -1
			if a.button == 0 && a.float == nil {
				a.hover = nil
			}
		}
	case *sdl.MouseMotionEvent:
		ui.mx, ui.my = int(e.X)/scale, int(e.Y)/scale
		if a.dialog == nil && ui.popup == nil && ui.drag == "" {
			a.pageMotion(ui.mx, ui.my)
		}
	case *sdl.MouseButtonEvent:
		ui.mx, ui.my = int(e.X)/scale, int(e.Y)/scale
		if e.Type == sdl.MOUSEBUTTONUP {
			ui.drag = ""
			a.pageRelease()
			return true
		}
		c := &Click{x: ui.mx, y: ui.my, button: int(e.Button), shift: shiftHeld()}
		if p := ui.popup; p != nil {
			ui.popupClick(c)
			// A click on the menu titles while a menu is open goes on to them:
			// another title opens its menu, the open one's closes it.
			if !p.menu || p.r.has(c.x, c.y) || !ui.menuBar.has(c.x, c.y) {
				return true
			}
			ui.closedMenu = p.id
		}
		if ui.focus != nil && a.dialog == nil {
			ui.focus = nil // a click elsewhere lets go of the width field
		}
		ui.click = c
		pass()
	case *sdl.MouseWheelEvent:
		if ctrlMod(sdl.GetModState()) && a.dialog == nil {
			a.zoomAt(a.zoom+int(e.Y), ui.mx, ui.my)
			return true
		}
		ui.wheel = -int(e.Y)
		if ui.popup == nil {
			pass()
		}
	case *sdl.TextInputEvent:
		textInput(e.GetText())
	case *sdl.KeyboardEvent:
		if e.Type == sdl.KEYDOWN {
			keyDown(e)
		}
	case *sdl.DropEvent:
		if e.Type == sdl.DROPFILE {
			if a.dialog != nil {
				a.msg("Close the requester first, then drop " + filepath.Base(e.File) + " again")
			} else {
				dropFile(e.File) // a font file asks whether it is the page's font, a picture whether it is its reference
			}
		}
	default:
		return false
	}
	return true
}

func textInput(s string) {
	if ui.focus != nil {
		ui.focus.insert(s)
		return
	}
	if viewer != nil {
		return
	}
	// Ctrl and left Alt are shortcuts; right Alt is AltGr (see shortcutMod).
	if a.dialog != nil || a.playing != nil || ui.popup != nil || shortcutMod(sdl.GetModState()) {
		return
	}
	if a.refMove {
		a.msg("Moving the reference: Return or Esc ends it, then type")
		return
	}
	a.start = false // typing starts on a page of the last used type
	if a.tool == "logo" && a.tdfPage() {
		a.logoText(s) // in a TheDraw font (tdf_ui.go)
		return
	}
	for _, r := range s {
		b := a.charToByte(r)
		if b < 0 {
			set := "CP437"
			if a.amiga() {
				set = "Latin-1"
			}
			a.msg(fmt.Sprintf("\"%c\" is not in the %s set", r, set))
			continue
		}
		a.typeByte(b)
	}
}

func keyDown(e *sdl.KeyboardEvent) {
	k := sdl.GetKeyName(e.Keysym.Sym)
	mod := sdl.Keymod(e.Keysym.Mod)
	ctrl, shift := ctrlMod(mod), mod&sdl.KMOD_SHIFT != 0 // AltGr is no Ctrl
	if ui.popup != nil {
		ui.popupKey(k)
		return
	}
	if a.playing != nil {
		if k == "Escape" || k == "M" || keymap[combo{modsOf(ctrl, mod&sdl.KMOD_LALT != 0, shift), k}] == "play" {
			a.stopPlay(true)
			return
		}
		if viewer == nil {
			return
		}
	}
	if ui.focus != nil {
		if ctrl && k == "V" {
			ui.focus.insert(getClipboard())
			return
		}
		if ui.focus.key(k, ctrl) {
			return
		}
		if a.dialog == nil && (k == "Escape" || k == "Tab") {
			ui.focus = nil
			return
		}
		if a.dialog == nil && !ctrl {
			return // F-keys, arrows and the rest stay off the page while the W/H field has the keys
		}
	}
	if a.dialog != nil {
		a.dialog.key(k, ctrl, shift)
		return
	}
	editorKey(k, ctrl, shift, mod&sdl.KMOD_LALT != 0)
}

// cursorKey moves the cursor for an arrow, Home, End, PgUp, PgDn, Return or
// Tab; false for any other key. The caller calls moved.
func (a *App) cursorKey(k string) bool {
	visRows := max(1, a.page.h/(a.cellH()*a.zoom))
	switch k {
	case "Left":
		a.cx--
	case "Right":
		a.cx++
		if a.uni() && uniIsCont(a.doc.get(a.cx, a.cy)) {
			a.cx++ // over a wide character's right half
		}
	case "Up":
		a.cy--
	case "Down":
		a.cy++
	case "Home":
		a.cx = 0
	case "End":
		a.cx = min(a.doc.cols-1, a.doc.lastCol(a.cy)+1)
	case "PageUp":
		a.cy -= visRows
	case "PageDown":
		a.cy += visRows
	case "Return", "Keypad Enter":
		a.cx, a.cy = 0, a.cy+1
	case "Tab":
		a.cx = min(a.doc.cols-1, (a.cx&^7)+8)
	default:
		return false
	}
	return true
}

// selectKey moves the cursor and grows the selection with it, starting one at
// the cursor when there is none (any tool).
func (a *App) selectKey(k string) {
	if a.float != nil {
		a.msg("Put the block down first (click), or Esc")
		return
	}
	if a.sel == nil || a.sel.x1 != a.cx || a.sel.y1 != a.cy {
		a.sel = &Sel{a.cx, a.cy, a.cx, a.cy}
	}
	a.cursorKey(k)
	a.moved()
	a.sel.x1, a.sel.y1 = a.cx, a.cy
	r := a.selRect()
	a.msg(fmt.Sprintf("%dx%d selected", r.w, r.h))
}

func editorKey(k string, ctrl, shift, alt bool) {
	if viewer != nil {
		viewerKey(k, ctrl, shift)
		return
	}
	if a.start && !ctrl && !alt && a.startKey(k, shift) {
		return
	}
	if a.refMove && !ctrl && !alt && a.refKey(k, shift) {
		return
	}
	if runKey(combo{modsOf(ctrl, alt, shift), k}) {
		return
	}
	if ctrl || alt {
		return // a shortcut bound to nothing
	}
	var fk int
	if n, _ := fmt.Sscanf(k, "F%d", &fk); n == 1 && fk >= 1 && fk <= 10 {
		if sets := a.fkeys[a.mode]; a.fset < len(sets) && fk <= len(sets[a.fset]) {
			a.typeByte(sets[a.fset][fk-1])
		}
		return
	}
	if a.tool == "logo" && a.tdfPage() && a.logoKey(k) { // Backspace and Return of the Logo tool (tdf_ui.go)
		return
	}
	if a.cursorKey(k) {
		a.moved()
		return
	}
	switch k {
	case "Escape":
		if a.moving {
			a.cancelMove()
		} else if a.float != nil {
			a.float = nil
		} else {
			a.sel = nil
		}
	case "Backspace":
		if a.cx > 0 {
			a.pushUndo()
			a.cx--
			if a.uni() && uniIsCont(a.doc.get(a.cx, a.cy)) && uniIsLead(a.doc.get(a.cx-1, a.cy)) {
				a.cx-- // over a whole wide character
			}
			if a.insert {
				a.deleteAt(a.cx, a.cy)
			} else {
				a.setCell(a.cx, a.cy, makeCell(32, a.fg, a.bg, 0))
			}
			a.moved()
		}
	case "Delete":
		if a.sel != nil {
			a.clearSel()
		} else {
			a.pushUndo()
			a.deleteAt(a.cx, a.cy)
		}
	case "Insert":
		a.insert = !a.insert
	}
}
