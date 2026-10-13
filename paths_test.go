package main

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The settings and data folders: the XDG overrides win on every system; without
// them Linux keeps its old places and Windows and macOS use their own.
func TestAppFolders(t *testing.T) {
	cfg, data := t.TempDir(), t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfg)
	t.Setenv("XDG_DATA_HOME", data)
	if got := configDir(); got != filepath.Join(cfg, "charcoal") {
		t.Fatalf("configDir with XDG_CONFIG_HOME: %s", got)
	}
	if got := appDataDir(); got != filepath.Join(data, "charcoal") {
		t.Fatalf("appDataDir with XDG_DATA_HOME: %s", got)
	}
	if got := autosaveDir(); got != filepath.Join(data, "charcoal", "autosave") {
		t.Fatalf("autosaveDir: %s", got)
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_DATA_HOME", "relative")
	c, d := configDir(), appDataDir()
	switch runtime.GOOS {
	case "linux":
		if c != filepath.Join(home, ".config", "charcoal") || d != filepath.Join(home, ".local", "share", "charcoal") {
			t.Fatalf("Linux: %s %s", c, d)
		}
	case "darwin":
		want := filepath.Join(home, "Library", "Application Support", "Charcoal")
		if c != want || d != want {
			t.Fatalf("macOS: %s %s", c, d)
		}
	case "windows":
		if c != d || !strings.HasSuffix(c, `\Charcoal`) {
			t.Fatalf("Windows: %s %s", c, d)
		}
	}
}

func TestPathEnds(t *testing.T) {
	sep := string(filepath.Separator)
	if withPathEnd(sep) != sep || withPathEnd("a") != "a"+sep || withPathEnd("a"+sep) != "a"+sep {
		t.Fatal("withPathEnd")
	}
	if hasPathEnd("") || !hasPathEnd("x"+sep) {
		t.Fatal("hasPathEnd")
	}
}

// Parent above a drive's top lists the drives; picking one lists it.
func TestFileReqRoots(t *testing.T) {
	roots := fileRoots()
	if len(roots) == 0 {
		t.Fatal("no drives")
	}
	d := &FileReq{sel: -1}
	d.list(roots[0])
	if d.err != "" {
		t.Fatal(d.err)
	}
	d.parent()
	if d.dir != "" || len(d.entries) != len(roots) || d.entries[0].full != roots[0] {
		t.Fatalf("drives list: %q %+v", d.dir, d.entries)
	}
	d.activate(0)
	if d.dir != filepath.Clean(roots[0]) || d.err != "" {
		t.Fatalf("picked drive: %q %s", d.dir, d.err)
	}
	if startDir("") == "" {
		t.Fatal("startDir")
	}
}
