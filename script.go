package main

// A test script for running without a person: CHARCOAL_SCRIPT holds steps
// separated by ";" that are played into the window one per frame, e.g.
//
//	CHARCOAL_SCRIPT="click 100 200; rclick 10 10; move 50 60; drag 1 2 3 4 [shift]; key ctrl+S; text abc; wheel 3; drop /tmp/a.png; do party_check; shot /tmp/a.png; quit"
//
// Used with SDL_VIDEODRIVER=offscreen to check the program headless.

import (
	"image"
	"image/png"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/veandco/go-sdl2/sdl"
)

var script []string
var scriptWait time.Time // a "wait" step holds the script until then

func init() {
	for _, s := range strings.Split(os.Getenv("CHARCOAL_SCRIPT"), ";") {
		if s = strings.TrimSpace(s); s != "" {
			script = append(script, s)
		}
	}
}

func shot(path string) {
	fb := ui.fb
	img := image.NewRGBA(image.Rect(0, 0, fb.w, fb.h))
	for i, p := range fb.px {
		img.Pix[i*4], img.Pix[i*4+1], img.Pix[i*4+2], img.Pix[i*4+3] = byte(p>>16), byte(p>>8), byte(p), 255
	}
	f, err := os.Create(path)
	if err != nil {
		return
	}
	png.Encode(f, img)
	f.Close()
}

// scriptStep plays the next step; it returns false when there is none.
func scriptStep() bool {
	if len(script) == 0 {
		return false
	}
	if time.Now().Before(scriptWait) {
		return true
	}
	pass() // lay out first, as a person would see the window
	step := strings.Fields(script[0])
	script = script[1:]
	n := func(i int) int { v, _ := strconv.Atoi(step[i]); return v }
	press := func(x, y, b int, shift bool) {
		ui.mx, ui.my = x, y
		a.pageMotion(x, y)
		ui.click = &Click{x: x, y: y, button: b, shift: shift}
		if ui.popup != nil {
			ui.popupClick(ui.click)
			ui.click = nil
		}
		pass()
		ui.drag = ""
		a.pageRelease()
	}
	switch step[0] {
	case "click":
		press(n(1), n(2), 1, len(step) > 3 && step[3] == "shift")
	case "rclick":
		press(n(1), n(2), 3, false)
	case "move":
		ui.mx, ui.my = n(1), n(2)
		a.pageMotion(n(1), n(2))
	case "drag":
		ui.mx, ui.my = n(1), n(2)
		ui.click = &Click{x: n(1), y: n(2), button: 1, shift: len(step) > 5 && step[5] == "shift"}
		pass()
		for i := 1; i <= 8; i++ {
			x, y := n(1)+(n(3)-n(1))*i/8, n(2)+(n(4)-n(2))*i/8
			ui.mx, ui.my = x, y
			if ui.drag == "" {
				a.pageMotion(x, y)
			}
			pass()
		}
		ui.drag = ""
		a.pageRelease()
	case "wheel":
		ui.wheel = n(1)
		pass()
	case "text":
		textInput(strings.Join(step[1:], " "))
	case "key":
		k := step[1]
		var mod uint16
		for {
			if strings.HasPrefix(k, "ctrl+") {
				mod |= sdl.KMOD_LCTRL
				k = k[5:]
			} else if strings.HasPrefix(k, "alt+") {
				mod |= sdl.KMOD_LALT
				k = k[4:]
			} else if strings.HasPrefix(k, "shift+") {
				mod |= sdl.KMOD_LSHIFT
				k = k[6:]
			} else {
				break
			}
		}
		code := sdl.GetKeyFromName(k)
		keyDown(&sdl.KeyboardEvent{Type: sdl.KEYDOWN, Keysym: sdl.Keysym{Sym: code, Mod: mod}})
	case "drop": // a file dropped on the window
		handleEvent(&sdl.DropEvent{Type: sdl.DROPFILE, File: step[1]})
	case "do": // an action of keys.go by its id, also one without a key
		if ac := actionByID[step[1]]; ac != nil {
			ac.run()
		}
	case "wait": // milliseconds
		scriptWait = time.Now().Add(time.Duration(n(1)) * time.Millisecond)
	case "shot":
		pass()
		shot(step[1])
	case "quit":
		a.quit = true
	}
	return true
}
