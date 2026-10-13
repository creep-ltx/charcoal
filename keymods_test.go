package main

import (
	"testing"

	"github.com/veandco/go-sdl2/sdl"
)

// AltGr (Windows: left Ctrl + right Alt) types and is no shortcut; Ctrl and the
// left Alt are; Cmd is Ctrl on a Mac only.
func TestShortcutMods(t *testing.T) {
	altGr := sdl.Keymod(sdl.KMOD_LCTRL | sdl.KMOD_RALT)
	if ctrlMod(altGr) || shortcutMod(altGr) || ctrlMod(sdl.KMOD_RALT) || shortcutMod(sdl.KMOD_RALT|sdl.KMOD_LSHIFT) {
		t.Fatal("AltGr taken as a shortcut")
	}
	if !ctrlMod(sdl.KMOD_LCTRL) || !ctrlMod(sdl.KMOD_RCTRL|sdl.KMOD_LSHIFT) || !shortcutMod(sdl.KMOD_LCTRL) || !shortcutMod(sdl.KMOD_LALT) {
		t.Fatal("Ctrl / left Alt not a shortcut")
	}
	if ctrlMod(sdl.KMOD_LGUI) != isMac || shortcutMod(sdl.KMOD_RGUI) != isMac {
		t.Fatal("Cmd")
	}
	if shortcutMod(sdl.KMOD_LSHIFT) || shortcutMod(0) {
		t.Fatal("plain typing taken as a shortcut")
	}
}
