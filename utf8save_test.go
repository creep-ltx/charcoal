package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"unicode/utf8"
)

// A UTF-8 file that fits a PC page opens as one and saves back in UTF-8, not in
// CP437: a terminal artist's file stays readable in the terminal.
func TestUTF8SaveStaysUTF8(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name, src, fmt string
	}{
		{"art.ans", "\x1b[1;33m█▓░ hello ╔═╗\x1b[0m\r\n\x1b[44m ▄▀ \x1b[0m\r\n", "ansiutf8"},
		{"note.txt", "╔═══╗\n║ hi║\n╚═══╝\n", "utf8"},
	}
	for _, c := range cases {
		a := testApp(t, "pc")
		p := filepath.Join(dir, c.name)
		os.WriteFile(p, []byte(c.src), 0o644)
		a.openPath(p, "auto")
		if a.mode != "pc" || a.saveFmt != c.fmt {
			t.Fatalf("%s: opened as %s, saves as %s; want pc, %s", c.name, a.mode, a.saveFmt, c.fmt)
		}
		want := a.doc.clone()
		a.dirty = true
		if !a.writeOut(p, a.saveFmt) {
			t.Fatalf("%s: save failed: %s", c.name, a.message)
		}
		b, _ := os.ReadFile(p)
		if !utf8.Valid(b) || !bytes.Contains(b, []byte("╔")) {
			t.Fatalf("%s: not saved in UTF-8: %q", c.name, b)
		}
		a.openPath(p, "auto")
		if a.mode != "pc" || a.saveFmt != c.fmt {
			t.Fatalf("%s: reopened as %s / %s", c.name, a.mode, a.saveFmt)
		}
		for y := 0; y < want.rows; y++ {
			for x := 0; x < want.cols; x++ {
				if g, w := a.doc.get(x, y), want.get(x, y); g != w {
					t.Fatalf("%s: cell %d,%d %08x, want %08x", c.name, x, y, g, w)
				}
			}
		}
	}
	// A CP437 file is still saved in CP437.
	a := testApp(t, "pc")
	p := filepath.Join(dir, "dos.ans")
	os.WriteFile(p, []byte("\x1b[1;33m\xdb\xb2\xb0\x1b[0m\r\n"), 0o644)
	a.openPath(p, "auto")
	if a.saveFmt != "ansi" {
		t.Fatalf("CP437 file saves as %s", a.saveFmt)
	}
}
