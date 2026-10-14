package main

import "testing"

// withFonts makes every Commodore ROM and the ST fonts "present" (empty
// stand-ins where the real ones are missing), or takes the ST fonts away.
func withFonts(t *testing.T, st bool) {
	old := fonts
	t.Cleanup(func() { fonts = old })
	var list []*Font
	for _, f := range old {
		if f.kind != "st" || st {
			list = append(list, f)
		}
	}
	fonts = list
	add := func(id, kind string) {
		if fontByID(id) == nil {
			fonts = append(fonts, &Font{id: id, name: id, kind: kind, bits: make([]byte, 256*16)})
		}
	}
	for _, fam := range []string{"pet", "vic20", "c64", "c128"} {
		add(fam+"upper", "c64")
		add(fam+"lower", "c64")
	}
	if st {
		add("st8x8", "st")
		add("st8x16", "st")
	}
}

func tileNamed(t *testing.T, label string) startTile {
	for _, tl := range startTiles() {
		if tl.label == label {
			return tl
		}
	}
	t.Fatalf("no tile %q", label)
	return startTile{}
}

func TestStartTilePicks(t *testing.T) {
	cases := []struct {
		label, mode, machine string
		cols, rows           int
	}{
		{"Amiga", "amiga", "", 80, 25},
		{"PC", "pc", "", 80, 25},
		{"Atari 8-bit", "atari", "", 40, 24},
		{"Atari ST", "st", "", 80, 25},
		{"C64", "c64", "c64", 40, 25},
		{"PET", "c64", "pet", 40, 25},
		{"VIC-20", "c64", "vic20", 22, 23},
		{"C128", "c64", "c128", 40, 25},
		{"ZX Spectrum", "zx", "", 32, 24},
		{"Teletext", "tt", "", 40, 25},
	}
	// From every page type to every tile: the page takes the tile's type and size.
	for _, from := range []string{"amiga", "pc", "atari", "zx", "tt", "c64"} {
		for _, c := range cases {
			a := testApp(t, from)
			withFonts(t, true)
			a.hasC64 = true
			a.start = true
			a.pickTile(tileNamed(t, c.label))
			if a.dialog != nil {
				t.Fatalf("%s -> %s: a requester opened", from, c.label)
			}
			if a.start {
				t.Errorf("%s -> %s: the start screen is still shown", from, c.label)
			}
			if a.mode != c.mode || (c.machine != "" && a.machine != c.machine) {
				t.Errorf("%s -> %s: mode %s machine %s", from, c.label, a.mode, a.machine)
			}
			if a.doc.cols != c.cols || a.doc.rows != c.rows {
				t.Errorf("%s -> %s: %dx%d, want %dx%d", from, c.label, a.doc.cols, a.doc.rows, c.cols, c.rows)
			}
			if len(a.tabs) != 1 {
				t.Errorf("%s -> %s: %d tabs", from, c.label, len(a.tabs))
			}
		}
	}
}

func TestStartTileSizesMatchPages(t *testing.T) {
	testApp(t, "amiga")
	withFonts(t, true)
	for _, tl := range startTiles() {
		got := tl.tab().doc
		a := testApp(t, "amiga")
		withFonts(t, true)
		a.hasC64 = true
		a.pickTile(tl)
		if got.cols != a.doc.cols || got.rows != a.doc.rows {
			t.Errorf("%s: the tile says %dx%d, the page is %dx%d", tl.label, got.cols, got.rows, a.doc.cols, a.doc.rows)
		}
	}
}

func TestStartTileFollowsLastMachine(t *testing.T) {
	a := testApp(t, "amiga")
	a.defMachine, a.defSTRes = "pet80", "stlow"
	if tl := tileNamed(t, "PET"); tl.machine != "pet80" || tl.tab().doc.cols != 80 {
		t.Errorf("PET tile: %+v", tl)
	}
	if tl := tileNamed(t, "Atari ST"); tl.stres != "stlow" || tl.tab().doc.cols != 40 {
		t.Errorf("ST tile: %+v", tl)
	}
}

func TestStartTileNeedsDownload(t *testing.T) {
	a := testApp(t, "amiga")
	withFonts(t, false) // no ST font
	a.start = true
	a.pickTile(tileNamed(t, "Atari ST"))
	if _, ok := a.dialog.(*FontReq); !ok {
		t.Fatalf("no download question: %T", a.dialog)
	}
	if a.mode != "amiga" || !a.start {
		t.Errorf("the page changed before the font came: mode %s start %v", a.mode, a.start)
	}
}

func TestStartAtLaunch(t *testing.T) {
	testApp(t, "amiga")
	if !startShownAtLaunch(0) {
		t.Error("no files: the start screen should show")
	}
	if startShownAtLaunch(1) || startShownAtLaunch(3) {
		t.Error("with files the start screen should not show")
	}
	prefs.NoStart = true
	if startShownAtLaunch(0) {
		t.Error("switched off, it should not show")
	}
}

func TestStartPrefSaved(t *testing.T) {
	a := testApp(t, "pc")
	prefs.NoStart = true
	savePrefs()
	p, ok := readPrefsFile()
	if !ok || !p.NoStart || p.Mode != "pc" {
		t.Fatalf("prefs.json: ok %v no_start_screen %v mode %q", ok, p.NoStart, p.Mode)
	}
	// Off: a new tab is an empty page of the type being worked on.
	a.addTab()
	if a.start || a.mode != "pc" {
		t.Errorf("new tab with the start screen off: start %v mode %s", a.start, a.mode)
	}
	prefs.NoStart = false
	a.addTab()
	if !a.start {
		t.Error("new tab with the start screen on: not shown")
	}
}

func TestStartKeys(t *testing.T) {
	a := testApp(t, "amiga")
	withFonts(t, true)
	a.hasC64 = true
	prefs.Recent = []string{"/nowhere/a.ans", "/nowhere/b.ans"}
	a.start, a.startCols = true, 5
	press := func(k string) { editorKey(k, false, false, false) }
	press("Right")
	press("Down")
	if a.startSel != 6 {
		t.Fatalf("Right, Down: item %d, want 6 (VIC-20)", a.startSel)
	}
	press("Down") // past the last row of tiles: the first recent file
	if a.startSel != 10 {
		t.Fatalf("Down from the last row: item %d, want 10", a.startSel)
	}
	press("Up")
	if a.startSel != 6 {
		t.Fatalf("Up from the recent files: item %d, want 6", a.startSel)
	}
	press("Return")
	if a.start || a.mode != "c64" || a.machine != "vic20" {
		t.Errorf("Return on VIC-20: start %v mode %s machine %s", a.start, a.mode, a.machine)
	}

	// Esc: the page of the type being worked on, untouched.
	a = testApp(t, "pc")
	a.start = true
	press("Escape")
	if a.start || a.mode != "pc" {
		t.Errorf("Esc: start %v mode %s", a.start, a.mode)
	}
	// A tool key or an editing key lets it go too.
	a.start = true
	editorKey("D", false, false, true) // Alt+D: the Draw tool
	if a.start {
		t.Error("a tool key left the start screen up")
	}
	a.start = true
	press("F12") // the help: the start screen stays behind it
	if !a.start {
		t.Error("F12 let the start screen go")
	}
	closeDialog()
	press("F1") // types the F-key set's first character
	if a.start {
		t.Error("F1 left the start screen up")
	}
	a.start = true
	press("Delete")
	if a.start {
		t.Error("Delete left the start screen up")
	}
	// Tab goes round every item: 10 tiles, 2 recent files, Open.
	prefs.Recent = []string{"/nowhere/a.ans", "/nowhere/b.ans"}
	a.start, a.startSel = true, 0
	for i := 0; i < 13; i++ {
		press("Tab")
	}
	if a.startSel != 0 {
		t.Errorf("13 Tabs: item %d, want 0", a.startSel)
	}
}

func TestShowStart(t *testing.T) {
	a := testApp(t, "amiga")
	a.showStart()
	if !a.start || len(a.tabs) != 1 {
		t.Fatalf("an empty tab shows it in place: start %v tabs %d", a.start, len(a.tabs))
	}
	a.start = false
	a.typeByte('x')
	a.showStart()
	if !a.start || len(a.tabs) != 2 || !a.untouched() {
		t.Errorf("a page with art: a new tab: start %v tabs %d", a.start, len(a.tabs))
	}

	// A file opened from the start screen takes its tab, even an (unblank) empty Spectrum page.
	a = testApp(t, "zx")
	a.showStart()
	a.openPath("testdata/alove/bs-alove.xb", "auto")
	if len(a.tabs) != 1 || a.start || a.path == "" {
		t.Errorf("open from the start screen: %d tabs, start %v, path %q", len(a.tabs), a.start, a.path)
	}
}
