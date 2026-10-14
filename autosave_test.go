package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// restart is a new editor with the same settings and data folders, as after a crash.
func restart(t *testing.T, mode string) *App {
	cfg, data := os.Getenv("XDG_CONFIG_HOME"), os.Getenv("XDG_DATA_HOME")
	a := testApp(t, mode)
	t.Setenv("XDG_CONFIG_HOME", cfg)
	t.Setenv("XDG_DATA_HOME", data)
	return a
}

func TestAutosaveRoundTrip(t *testing.T) {
	a := testApp(t, "amiga")
	a.ital = true
	for _, c := range "Hi" {
		a.typeByte(int(c))
	}
	a.path, a.name = "/nowhere/art.ans", "art.ans"
	a.info.title = "A title"
	amiga := a.Tab
	a.focusTab(a.newTab("zx"))
	a.fg, a.bg = 2, 5
	a.typeByte('Z')
	a.doc.border = 3
	zx := a.Tab
	a.focusTab(a.newTab("pc")) // a clean tab: not kept
	lastAutosave = time.Now()
	a.autosaveTick(time.Now())
	if amiga.autoID != "" {
		t.Fatal("autosaved before two minutes")
	}
	a.autosaveTick(time.Now().Add(3 * time.Minute))
	if amiga.autoID == "" || zx.autoID == "" || a.Tab.autoID != "" {
		t.Fatalf("autosave ids %q %q %q", amiga.autoID, zx.autoID, a.Tab.autoID)
	}
	wantAmiga, wantZX := amiga.doc.clone(), zx.doc.clone()

	b := restart(t, "pc")
	files := leftAutosaves()
	if len(files) != 2 {
		t.Fatalf("%d autosaves found, want 2", len(files))
	}
	if !offerAutosaves() {
		t.Fatal("no offer")
	}
	b.dialog.(*AutosaveReq).reopen()
	if len(b.tabs) != 2 {
		t.Fatalf("%d tabs after reopening", len(b.tabs))
	}
	for i, want := range []*Doc{wantAmiga, wantZX} {
		got := b.tabs[i]
		if got.doc.cols != want.cols || got.doc.rows != want.rows || fmt.Sprint(got.doc.cells[:want.rows*want.cols]) != fmt.Sprint(want.cells[:want.rows*want.cols]) ||
			got.doc.border != want.border || !got.dirty {
			t.Errorf("tab %d (%s) did not come back the same", i, got.mode)
		}
	}
	if b.tabs[0].mode != "amiga" || b.tabs[0].path != "/nowhere/art.ans" || b.tabs[0].info.title != "A title" || b.tabs[1].mode != "zx" {
		t.Errorf("tabs: %s %q %q, %s", b.tabs[0].mode, b.tabs[0].path, b.tabs[0].info.title, b.tabs[1].mode)
	}
	// They are kept aside again under the new editor, the old files are gone.
	if left := leftAutosaves(); len(left) != 2 || left[0].id == files[0].id {
		t.Errorf("after reopening: %d autosaves", len(left))
	}
	// Saving one drops its autosave; closing the other drops its.
	b.focusTab(b.tabs[0])
	p := filepath.Join(t.TempDir(), "art.ans")
	if !b.writeOut(p, "ansi") {
		t.Fatal(b.message)
	}
	if b.tabs[0].autoID != "" {
		t.Error("saving kept the autosave")
	}
	b.tabs[1].dirty = false
	b.closeTab(b.tabs[1])
	if left := leftAutosaves(); len(left) != 0 {
		t.Errorf("%d autosaves left after save and close", len(left))
	}
}

func TestAutosaveThrowAway(t *testing.T) {
	a := testApp(t, "pc")
	a.typeByte('x')
	if err := writeAutosave(a.Tab); err != nil {
		t.Fatal(err)
	}
	b := restart(t, "pc")
	offerAutosaves()
	b.dialog.(*AutosaveReq).throwAway()
	c, ok := b.dialog.(*Confirm)
	if !ok {
		t.Fatal("throwing away does not ask first")
	}
	c.yes()
	if len(leftAutosaves()) != 0 {
		t.Error("the autosave is still there")
	}
}

func TestBackupOnSave(t *testing.T) {
	a := testApp(t, "pc")
	dir := t.TempDir()
	p := filepath.Join(dir, "x.ans")
	os.WriteFile(p, []byte("old art"), 0o644)
	a.typeByte('n')
	if !a.writeOut(p, "ansi") {
		t.Fatal(a.message)
	}
	if b, err := os.ReadFile(p + ".bak"); err != nil || string(b) != "old art" {
		t.Errorf(".bak holds %q, %v", b, err)
	}
	// Saving the same again keeps the .bak as it is.
	a.writeOut(p, "ansi")
	if b, _ := os.ReadFile(p + ".bak"); string(b) != "old art" {
		t.Errorf("an unchanged save replaced the .bak with %q", b)
	}
	prefs.NoBackup = true
	os.Remove(p + ".bak")
	a.typeByte('m')
	a.writeOut(p, "ansi")
	if _, err := os.Stat(p + ".bak"); err == nil {
		t.Error("a .bak was kept with the setting off")
	}
}

func TestRecentFiles(t *testing.T) {
	a := testApp(t, "pc")
	dir := t.TempDir()
	var paths []string
	for i := 0; i < 12; i++ {
		p := filepath.Join(dir, fmt.Sprintf("f%d.ans", i))
		os.WriteFile(p, []byte(fmt.Sprintf("file %d", i)), 0o644)
		paths = append(paths, p)
		a.openPath(p, "auto")
	}
	if len(prefs.Recent) != maxRecent || prefs.Recent[0] != paths[11] {
		t.Fatalf("recent: %d, first %q", len(prefs.Recent), prefs.Recent[0])
	}
	a.openPath(paths[5], "auto") // again: to the front, once
	if prefs.Recent[0] != paths[5] || len(prefs.Recent) != maxRecent {
		t.Errorf("recent after opening again: %q", prefs.Recent[:2])
	}
	a.closeTab(a.Tab) // f5 closed: Ctrl+Shift+O opens it again
	editorKey("O", true, true, false)
	if a.path != paths[5] {
		t.Errorf("Ctrl+Shift+O opened %q", a.path)
	}
}

// A page of frames comes back with every frame, its hold, name, case and
// carousel subpage text, on the frame being worked on.
func TestAutosaveFrames(t *testing.T) {
	orig := carouselTTI()
	a, p := loadTTIFile(t, orig)
	a.goFrame(1)
	a.pushUndo()
	a.doc.set(0, 5, makeCell('X', 0, 0, 0))
	a.setHold(3000)
	a.newFrame()
	a.doc.set(0, 6, makeCell('N', 0, 0, 0))
	if err := writeAutosave(a.Tab); err != nil {
		t.Fatal(err)
	}
	want := a.allDocs()
	b := restart(t, "pc")
	files := leftAutosaves()
	if len(files) != 1 {
		t.Fatalf("%d autosaves", len(files))
	}
	if err := b.restoreAutosave(files[0]); err != nil {
		t.Fatal(err)
	}
	if b.nFrames() != 4 || b.cur != 2 || b.srcKind != "tti" || b.holdOf(1) != 3000 || b.holdOf(0) != 8000 || !b.restructured {
		t.Fatalf("%d frames, cur %d, kind %q, holds %d %d", b.nFrames(), b.cur, b.srcKind, b.holdOf(0), b.holdOf(1))
	}
	for i, w := range want {
		if fmt.Sprint(b.frameDoc(i).cells[:w.rows*w.cols]) != fmt.Sprint(w.cells[:w.rows*w.cols]) {
			t.Fatalf("frame %d did not come back the same", i)
		}
	}
	if b.frames[0].seg == nil || b.frames[2].seg != nil {
		t.Fatal("subpage texts")
	}
	// Saved, it is the carousel with the edit, the hold and the new subpage.
	if !b.writeOut(p, "tti") {
		t.Fatal(b.message)
	}
	got, _ := os.ReadFile(p)
	_, subs := ttiHead(got)
	if len(subs) != 4 || ttRowText(parseTTI(subs[1]), 5) != "X" || ttRowText(parseTTI(subs[2]), 6) != "N" ||
		!strings.Contains(string(subs[1]), "CT,3,T") {
		t.Fatalf("saved: %q", got)
	}
}

// An autosave from 0.1b3 of one subpage of a carousel (a tab per subpage then)
// comes back as the carousel's frames with that subpage as autosaved.
func TestAutosaveOldSubpage(t *testing.T) {
	orig := carouselTTI()
	a, p := loadTTIFile(t, orig)
	d := parseTTI(ttiSubpages(orig)[1])
	d.set(0, 9, makeCell('Q', 0, 0, 0))
	cells := make([]byte, 0, 4*40*25)
	for _, v := range d.cells[:d.cols*d.rows] {
		cells = binary.LittleEndian.AppendUint32(cells, v)
	}
	m := autosaveMeta{PID: 1 << 30, Saved: time.Now(), Path: p, Name: "carousel.tti", Mode: "tt", SaveFmt: "tti",
		Cols: d.cols, Rows: d.rows, Subpage: 2}
	js, _ := json.Marshal(m)
	os.MkdirAll(autosaveDir(), 0o700)
	os.WriteFile(filepath.Join(autosaveDir(), "old.json"), js, 0o644)
	os.WriteFile(filepath.Join(autosaveDir(), "old.cells"), cells, 0o644)
	_ = a
	b := restart(t, "pc")
	if err := b.restoreAutosave(autosaveFile{"old", m}); err != nil {
		t.Fatal(err)
	}
	if b.nFrames() != 3 || b.cur != 1 || ttRowText(b.doc, 9) != "Q" || ttRowText(b.frameDoc(2), 1) != ".THIRD" || !b.dirty {
		t.Fatalf("%d frames, cur %d", b.nFrames(), b.cur)
	}
}
