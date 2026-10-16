package main

// durdraw itself reads CharEdit's .dur files, run only when a durdraw checkout is there:
//
//	CHARCOAL_DURDRAW_SRC=/path/to/durdraw go test -run TestUniExternal .
//
// (github.com/cmang/durdraw; plain Python with curses, nothing to install.) Each file
// CharEdit writes goes through `durdraw --export-ansi` in a pseudo terminal
// (script(1)), and the ANSI durdraw writes must be the page.

import (
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestUniExternalDurdraw(t *testing.T) {
	src := os.Getenv("CHARCOAL_DURDRAW_SRC")
	if src == "" {
		t.Skip("CHARCOAL_DURDRAW_SRC not set")
	}
	dir := t.TempDir()
	runner := filepath.Join(dir, "run.py")
	os.WriteFile(runner, []byte(fmt.Sprintf("import sys\nsys.path.insert(0, %q)\nfrom durdraw.main import main\nsys.argv = ['durdraw'] + sys.argv[1:]\nmain()\n", src)), 0o644)
	r := rand.New(rand.NewSource(8))
	for _, k := range []string{"16", "256"} {
		colour := func() (int, int) {
			if k == "16" { // durdraw's export cannot write its iCE backgrounds 9-16
				return r.Intn(16), r.Intn(8)
			}
			return uniFColour(r, false), 0 // durdraw draws 256-colour pages on black
		}
		d := uniFPage(r, 60, 15, colour, false)
		for x := 0; x < 60; x++ { // durdraw's export ends at the last character: fill the edges
			d.cells[14*60+x] = uniCell('#', pairOf(colour()))
		}
		for y := 0; y < 15; y++ {
			d.cells[y*60+59] = uniCell('|', pairOf(colour()))
		}
		out, _ := writeDur([]*Doc{d, d}, []int{100, 300}, "", "")
		path := filepath.Join(dir, "page"+k+".dur")
		os.WriteFile(path, out, 0o644)
		cmd := exec.Command("script", "-qec", fmt.Sprintf("python3 -I %s --%scolor --export-ansi %s", runner, k, path), "/dev/null")
		cmd.Env = append(os.Environ(), "TERM=xterm-256color")
		cmd.Dir = dir
		if err := cmd.Run(); err != nil {
			t.Fatalf("durdraw: %v", err)
		}
		ref, err := os.ReadFile(strings.TrimSuffix(path, ".dur") + ".ansi")
		if err != nil {
			t.Fatalf("durdraw wrote nothing: %v", err)
		}
		got, _ := parseUniArt(ref, 60)
		uniFSame(t, k+" colours, as durdraw exports it", got, d)
	}
}

// TestWideExternalDurdraw: durdraw reads a .dur with wide characters CharEdit
// wrote, and its ANSI export (what a terminal shows) is the page.
func TestWideExternalDurdraw(t *testing.T) {
	src := os.Getenv("CHARCOAL_DURDRAW_SRC")
	if src == "" {
		t.Skip("CHARCOAL_DURDRAW_SRC not set")
	}
	dir := t.TempDir()
	runner := filepath.Join(dir, "run.py")
	os.WriteFile(runner, []byte(fmt.Sprintf("import sys\nsys.path.insert(0, %q)\nfrom durdraw.main import main\nsys.argv = ['durdraw'] + sys.argv[1:]\nmain()\n", src)), 0o644)
	d := wideFDoc(t, 12, "a中b字cかな!", "############")
	d.set(1, 0, uniCell(uniCode(d.get(1, 0)), pairOf(2, 0)))
	d.set(2, 0, uniContOf(d.get(1, 0)))
	out, _ := writeDur([]*Doc{d}, []int{100}, "", "")
	path := filepath.Join(dir, "wide.dur")
	os.WriteFile(path, out, 0o644)
	cmd := exec.Command("script", "-qec", fmt.Sprintf("python3 -I %s --16color --export-ansi %s", runner, path), "/dev/null")
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")
	cmd.Dir = dir
	if err := cmd.Run(); err != nil {
		t.Fatalf("durdraw: %v", err)
	}
	ref, err := os.ReadFile(strings.TrimSuffix(path, ".dur") + ".ansi")
	if err != nil {
		t.Fatalf("durdraw wrote nothing: %v", err)
	}
	got, _ := parseUniArt(ref, 12)
	wideFSame(t, "as durdraw exports it", got, d)
}
