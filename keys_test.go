package main

import (
	"os"
	"strings"
	"testing"

	"github.com/veandco/go-sdl2/sdl"
)

func TestParseCombo(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"Ctrl+Z", "Ctrl+Z"},
		{"ctrl+shift+up", "Ctrl+Shift+Up"},
		{"Alt+f", "Alt+F"},
		{"Ctrl++", "Ctrl++"},
		{"Ctrl+Keypad +", "Ctrl+Keypad +"},
		{"shift+alt+ctrl+pgdn", "Ctrl+Alt+Shift+PageDown"},
		{"Shift+F1", "Shift+F1"},
		{"F11", "F11"},
	} {
		got, err := parseCombo(c.in)
		if err != nil || got.String() != c.want {
			t.Errorf("parseCombo(%q) = %v, %v; want %s", c.in, got, err, c.want)
		}
	}
	for _, bad := range []string{"", "Ctrl+", "Ctrl+Nonsense", "x", "Shift+X", "Up", "Alt+F4", "Return"} {
		if c, err := parseCombo(bad); err == nil {
			t.Errorf("parseCombo(%q) = %v, want an error", bad, c)
		}
	}
}

func TestDefaultKeysHaveNoClashes(t *testing.T) {
	seen := map[combo]string{}
	for id, keys := range defaultKeys() {
		for _, c := range keys {
			if o, ok := seen[c]; ok {
				t.Errorf("%s is the default of both %s and %s", c, o, id)
			}
			seen[c] = id
			if c.mods&modAlt != 0 && c.key == "F4" {
				t.Errorf("%s binds Alt+F4", id)
			}
		}
	}
}

func TestKeyFile(t *testing.T) {
	text := "# a comment\n\nundo = Ctrl+U\nredo=ctrl+z, Ctrl+Nonsense\nno_such = Ctrl+P\njunk line\ncopy = Alt+F4\n"
	k, problems := applyKeyFile(text)
	if len(problems) != 4 {
		t.Errorf("problems %q, want 4", problems)
	}
	if len(k["undo"]) != 1 || k["undo"][0].String() != "Ctrl+U" {
		t.Errorf("undo %v", k["undo"])
	}
	if len(k["redo"]) != 1 || k["redo"][0].String() != "Ctrl+Z" {
		t.Errorf("redo %v", k["redo"])
	}
	if len(k["copy"]) != 1 || k["copy"][0].String() != "Ctrl+C" {
		t.Errorf("copy should keep its default when its line is all bad: %v", k["copy"])
	}
	// The written file reads back as the defaults, without complaint.
	k2, problems := applyKeyFile(keyFileText())
	if len(problems) != 0 {
		t.Errorf("the default file has problems: %q", problems)
	}
	def := defaultKeys()
	for id := range def {
		if len(k2[id]) != len(def[id]) {
			t.Errorf("%s: %v from the file, %v by default", id, k2[id], def[id])
		}
	}
}

func TestKeyFileTakesKeyFromOthers(t *testing.T) {
	k, _ := applyKeyFile("grid = Ctrl+Z\n")
	if len(k["undo"]) != 0 {
		t.Errorf("undo kept %v after grid took Ctrl+Z", k["undo"])
	}
}

func TestLoadKeysWritesDefaults(t *testing.T) {
	a := testApp(t, "pc")
	loadKeys()
	b, err := os.ReadFile(keysPath())
	if err != nil || !strings.Contains(string(b), "undo = Ctrl+Z") {
		t.Fatalf("keys file not written: %v", err)
	}
	os.WriteFile(keysPath(), []byte("undo = Ctrl+U\nbogus = Ctrl+P\n"), 0o644)
	loadKeys()
	defer setKeys(defaultKeys())
	if !strings.Contains(a.message, "bogus") {
		t.Errorf("message %q", a.message)
	}
	a.typeByte('x')
	editorKey("U", true, false, false)
	if a.doc.get(0, 0) != blank {
		t.Error("Ctrl+U did not undo")
	}
	if !strings.Contains(keysHelp(), "Ctrl+U: Undo") {
		t.Error("the help does not show the new key")
	}
}

func TestHelpNamesEveryAction(t *testing.T) {
	setKeys(defaultKeys())
	h := helpText()
	for _, ac := range actions {
		for _, c := range keysNow[ac.id] {
			if !strings.Contains(h, c.String()) && !strings.Contains(h, c.String()[:len(c.String())-len(c.key)]) {
				t.Errorf("the help does not name %s (%s)", c, ac.id)
			}
		}
	}
}

func TestRightAltIsNotAlt(t *testing.T) {
	a := testApp(t, "pc")
	a.sel = &Sel{0, 0, 0, 0}
	a.setCell(0, 0, a.cell('/'))
	// AltGr+H (right Alt) must not mirror; left Alt+H must.
	keyDown(&sdl.KeyboardEvent{Type: sdl.KEYDOWN, Keysym: sdl.Keysym{Sym: sdl.K_h, Mod: sdl.KMOD_RALT}})
	if cellCh(a.doc.get(0, 0)) != '/' {
		t.Error("right Alt worked as Alt")
	}
	keyDown(&sdl.KeyboardEvent{Type: sdl.KEYDOWN, Keysym: sdl.Keysym{Sym: sdl.K_h, Mod: sdl.KMOD_LALT}})
	if cellCh(a.doc.get(0, 0)) != '\\' {
		t.Errorf("left Alt+H did not mirror: %c", cellCh(a.doc.get(0, 0)))
	}
}
