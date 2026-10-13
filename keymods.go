package main

import "github.com/veandco/go-sdl2/sdl"

// The shortcut modifier. Ctrl everywhere; on a Mac Cmd as well, so the keys
// file's "Ctrl" means Cmd or Ctrl there. AltGr is never Ctrl: Windows reports
// it as left Ctrl + right Alt, and it types @ { [ \ $ | ~ on many keyboards.
func ctrlMod(mod sdl.Keymod) bool {
	if isMac && mod&sdl.KMOD_GUI != 0 {
		return true
	}
	return mod&sdl.KMOD_CTRL != 0 && mod&sdl.KMOD_RALT == 0
}

// shortcutMod tells whether a key press with these modifiers is a shortcut, so
// the character it may also type is not put on the page: Ctrl (Cmd on a Mac) or
// the left Alt (the left Option on a Mac). The right Alt / Option is for typing.
func shortcutMod(mod sdl.Keymod) bool {
	if isMac && mod&sdl.KMOD_GUI != 0 {
		return true
	}
	return mod&sdl.KMOD_RALT == 0 && mod&(sdl.KMOD_CTRL|sdl.KMOD_LALT) != 0
}

// macKeysNote is the help's word on the Mac's keys.
func macKeysNote() string {
	if !isMac {
		return ""
	}
	return " On a Mac, Cmd works wherever Ctrl is written (Ctrl does too; Cmd+Q, Cmd+H and Cmd+M are the Mac's own), " +
		"Alt is the left Option key, and the right Option key types characters such as @ { [ | \\."
}
