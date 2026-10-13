package main

import (
	"os"
	"path/filepath"
	"syscall"
	"unsafe"

	"github.com/veandco/go-sdl2/sdl"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")
)

// beforeSDL tells Windows the program draws in the screen's own pixels: else on
// a screen scaled to 150% Windows stretches the window and every pixel blurs.
func beforeSDL() {
	if p := user32.NewProc("SetProcessDPIAware"); p.Find() == nil {
		p.Call()
	}
}

// firstScale is the window scale before the person has picked one: 2x on a
// screen Windows scales to 175% or more.
func firstScale() int {
	if _, h, _, err := sdl.GetDisplayDPI(0); err == nil && h >= 168 {
		return 2
	}
	return 1
}

// running tells whether the CharEdit with this process id still runs: then its
// autosaves are its own business. Windows hands numbers out again soon, so the
// program's file name must match ours as well.
func running(pid int) bool {
	if pid <= 0 || pid == os.Getpid() {
		return false
	}
	const queryLimited = 0x1000 // PROCESS_QUERY_LIMITED_INFORMATION
	h, err := syscall.OpenProcess(queryLimited, false, uint32(pid))
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(h)
	var code uint32
	if syscall.GetExitCodeProcess(h, &code) != nil || code != 259 { // STILL_ACTIVE
		return false
	}
	p := kernel32.NewProc("QueryFullProcessImageNameW")
	if p.Find() != nil {
		return true
	}
	buf := make([]uint16, 1024)
	n := uint32(len(buf))
	if r, _, _ := p.Call(uintptr(h), 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&n))); r == 0 {
		return true
	}
	self, err := os.Executable()
	if err != nil {
		return true
	}
	return sameProgram(filepath.Base(syscall.UTF16ToString(buf[:n])), filepath.Base(self))
}

// fileRoots are the drives that are there: C:\, D:\ ...
func fileRoots() []string {
	var out []string
	mask, _, _ := kernel32.NewProc("GetLogicalDrives").Call()
	for i := 0; i < 26; i++ {
		if mask&(1<<i) != 0 {
			out = append(out, string(rune('A'+i))+`:\`)
		}
	}
	return out
}
