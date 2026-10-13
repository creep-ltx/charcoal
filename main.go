// amiansi - an Amiga and PC ANSI / ASCII art viewer and editor, drawn with
// the real bitmap fonts.
//
//	amiansi [FILE]
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
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
)

func setClipboard(s string) { sdl.SetClipboardText(s) }
func getClipboard() string {
	s, _ := sdl.GetClipboardText()
	return s
}
func shiftHeld() bool { return sdl.GetModState()&sdl.KMOD_SHIFT != 0 }

func main() {
	runtime.LockOSThread()
	if len(os.Args) > 1 && (os.Args[1] == "-h" || os.Args[1] == "--help") {
		fmt.Println("usage: amiansi [FILE]\nAn Amiga / PC ANSI and ASCII art viewer and editor. Press ? in the window for the keys.")
		return
	}
	a = &App{tool: "draw", fg: 7, brush: 0x2F, name: "untitled.ans", saveFmt: "ansi", charHover: -1,
		info: Info{writeSauce: true}, doc: newDoc(80, 25)}
	loadPrefs()
	uiFont = fontByID("topaz2")
	a.applyMode()
	if len(os.Args) > 1 {
		p, _ := filepath.Abs(os.Args[1])
		if _, err := os.Stat(p); err == nil {
			a.openPath(p, "auto")
		} else {
			a.path, a.name = p, filepath.Base(p)
			a.msg("New file: " + p)
		}
	}

	if err := sdl.Init(sdl.INIT_VIDEO); err != nil {
		fmt.Fprintln(os.Stderr, "amiansi:", err)
		os.Exit(1)
	}
	defer sdl.Quit()
	sdl.SetHint(sdl.HINT_RENDER_SCALE_QUALITY, "0")
	sdl.SetHint(sdl.HINT_VIDEO_X11_NET_WM_BYPASS_COMPOSITOR, "0")
	scale = max(1, min(4, prefs.UIScale))
	w, h := 1400, 940
	if prefs.Width > 200 && prefs.Height > 200 {
		w, h = prefs.Width, prefs.Height
	} else if dm, err := sdl.GetDesktopDisplayMode(0); err == nil {
		w, h = min(w, int(dm.W)-40), min(h, int(dm.H)-80)
	}
	var err error
	window, err = sdl.CreateWindow("amiansi", sdl.WINDOWPOS_CENTERED, sdl.WINDOWPOS_CENTERED, int32(w), int32(h),
		sdl.WINDOW_RESIZABLE)
	if err != nil {
		fmt.Fprintln(os.Stderr, "amiansi:", err)
		os.Exit(1)
	}
	window.SetMinimumSize(640, 480)
	renderer, err = sdl.CreateRenderer(window, -1, sdl.RENDERER_ACCELERATED)
	if err != nil {
		renderer, err = sdl.CreateRenderer(window, -1, sdl.RENDERER_SOFTWARE)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "amiansi:", err)
		os.Exit(1)
	}
	sdl.StartTextInput()
	sdl.EventState(sdl.DROPFILE, sdl.ENABLE)

	nextBlink := time.Now().Add(500 * time.Millisecond)
	shownMsg := ""
	redraw := true
	for !a.quit {
		wait := time.Until(nextBlink)
		if a.playing != nil || len(script) > 0 {
			wait = 15 * time.Millisecond
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
		if a.playing != nil {
			a.playStep()
			redraw = true
		}
		if a.message != "" && time.Since(a.messageAt) > 4*time.Second && shownMsg == a.message {
			a.message = ""
			redraw = true
		}
		shownMsg = a.message
		if scriptStep() {
			redraw = true
		}
		if redraw {
			pass()
			present()
			redraw = false
		}
	}
	savePrefs()
}

// pass lays out and draws the whole window once, handing the pending click to whoever is under it.
func pass() {
	ww, wh := window.GetSize()
	ui.fb.resize(int(ww)/scale, int(wh)/scale)
	ui.hint, ui.wheelTaken = "", false
	ui.inert = a.dialog != nil
	a.drawWindow(ui)
	ui.inert = false
	if a.dialog != nil {
		ui.fb.blend(Rect{0, 0, ui.fb.w, ui.fb.h}, cEdgeDk, 120)
		a.dialog.draw(ui)
	}
	ui.drawPopup()
	ui.click, ui.wheel = nil, 0
	title := a.name + " - amiansi"
	if a.dirty {
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
			fmt.Fprintln(os.Stderr, "amiansi:", err)
			os.Exit(1)
		}
		texW, texH = fb.w, fb.h
	}
	texture.Update(nil, unsafe.Pointer(&fb.px[0]), fb.w*4)
	renderer.SetDrawColor(0, 0, 0, 255)
	renderer.Clear()
	renderer.Copy(texture, nil, &sdl.Rect{X: 0, Y: 0, W: int32(fb.w * scale), H: int32(fb.h * scale)})
	renderer.Present()
}

func requestQuit() {
	if a.dirty {
		a.dialog = &Confirm{text: "The page has changes that are not saved. Quit and throw them away?", yes: func() { a.quit = true }}
		return
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
		c := &Click{x: ui.mx, y: ui.my, button: int(e.Button), shift: shiftHeld(), double: e.Clicks >= 2}
		if ui.popup != nil {
			ui.popupClick(c)
			return true
		}
		if ui.focus != nil && a.dialog == nil {
			ui.focus = nil // a click elsewhere lets go of the width field
		}
		ui.click = c
		pass()
	case *sdl.MouseWheelEvent:
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
		if e.Type == sdl.DROPFILE && a.dialog == nil {
			p := e.File
			confirmDiscard(func() { a.openPath(p, "auto") })
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
	if a.dialog != nil || a.playing != nil || ui.popup != nil || sdl.GetModState()&sdl.KMOD_CTRL != 0 {
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
	ctrl, shift := mod&sdl.KMOD_CTRL != 0, mod&sdl.KMOD_SHIFT != 0
	if ui.popup != nil {
		if k == "Escape" {
			ui.popup = nil
		}
		return
	}
	if a.playing != nil {
		if k == "Escape" {
			a.stopPlay(true)
		}
		return
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
	}
	if a.dialog != nil {
		a.dialog.key(k, ctrl, shift)
		return
	}
	editorKey(k, ctrl, shift)
}

func editorKey(k string, ctrl, shift bool) {
	if ctrl {
		switch k {
		case "Z":
			if shift {
				a.doRedo()
			} else {
				a.doUndo()
			}
		case "Y":
			a.doRedo()
		case "H":
			a.bare = !a.bare
			if a.bare {
				a.msg("Gadgets hidden: Ctrl+H brings them back")
			}
		case "S":
			a.save(shift)
		case "O":
			confirmDiscard(openOpenRequester)
		case "N":
			confirmDiscard(func() { a.newDoc() })
		case "Q":
			requestQuit()
		case "A":
			a.sel = &Sel{0, 0, a.doc.cols - 1, max(0, a.doc.usedRows()-1)}
		case "C":
			a.copySel(false)
		case "X":
			a.copySel(true)
		case "V":
			a.paste()
		case "Up":
			a.setFset(a.fset + 1)
		case "Down":
			a.setFset(a.fset - 1)
		case "Left":
			if shift {
				a.setBg((a.bg + 15) % 16)
			} else {
				a.fg = (a.fg + 15) % 16
			}
		case "Right":
			if shift {
				a.setBg((a.bg + 1) % 16)
			} else {
				a.fg = (a.fg + 1) % 16
			}
		case "=", "+", "Keypad +":
			a.zoom = min(4, a.zoom+1)
			savePrefs()
		case "-", "Keypad -":
			a.zoom = max(1, a.zoom-1)
			savePrefs()
		}
		return
	}
	var fk int
	if n, _ := fmt.Sscanf(k, "F%d", &fk); n == 1 && fk >= 1 && fk <= 10 {
		a.typeByte(a.fkeys[a.mode][a.fset][fk-1])
		return
	}
	visRows := max(1, a.page.h/(a.cellH()*a.zoom))
	nav := true
	switch k {
	case "Left":
		a.cx--
	case "Right":
		a.cx++
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
		nav = false
	}
	if nav {
		a.moved()
		return
	}
	switch k {
	case "Escape":
		if a.float != nil {
			a.float = nil
		} else {
			a.sel = nil
		}
	case "Backspace":
		if a.cx > 0 {
			a.pushUndo()
			a.cx--
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
