package main

// Settings kept between runs, in prefs.json in configDir().

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Prefs struct {
	Mode      string              `json:"mode"`
	Font      map[string]string   `json:"font"`
	Palette   map[string]string   `json:"palette"`
	Zoom      int                 `json:"zoom"`
	Ch8       bool                `json:"ch8"`
	Smear     bool                `json:"smear"`
	Fkeys     map[string][][]int  `json:"fkeys"`
	Custom    []string            `json:"custom"`
	Dir       string              `json:"dir"`
	Baud      int                 `json:"baud"`
	UIScale   int                 `json:"uiscale"`
	Grid      bool                `json:"grid"`
	Machine   string              `json:"machine"`
	STRes     string              `json:"st_resolution"`
	ViewAuto  bool                `json:"view_auto_scroll"`
	ViewStep  bool                `json:"view_step_scroll"`
	ViewSpeed float64             `json:"view_scroll_speed"`
	C64Asked  bool                `json:"c64asked"`             // the first-run question about the C64 character set
	NoBackup  bool                `json:"no_backup"`            // saving over a file does not keep the old one as .bak
	NoStart   bool                `json:"no_start_screen"`      // start on an empty page, not the start screen
	Recent    []string            `json:"recent"`               // the last files opened or saved, newest first
	Refs      map[string]*RefPref `json:"references,omitempty"` // each art file's reference picture (refimage.go)
	Shades    map[string][]int    `json:"shades,omitempty"`     // the shade brush's characters per page type, lightest first
	ArtFonts  map[string]ArtFont  `json:"art_fonts"`            // art file -> the font CharEdit keeps for it (fontfile.go)
	MyBBS     string              `json:"my_bbs,omitempty"`     // the board whose pause codes animated saves get ("": none; bbspause.go)
	Width     int                 `json:"width"`
	Height    int                 `json:"height"`
}

var prefs Prefs

func prefsPath() string { return filepath.Join(configDir(), "prefs.json") }

// pageModes are the page types, as prefs.json names them.
var pageModes = []string{"amiga", "pc", "c64", "atari", "st", "zx", "tt"}

func loadPrefs() {
	if b, err := os.ReadFile(prefsPath()); err == nil {
		if err := json.Unmarshal(b, &prefs); err != nil {
			// Kept for the person to look at, not silently written over.
			prefs = Prefs{}
			bad := prefsPath() + ".bad"
			if os.Rename(prefsPath(), bad) == nil {
				a.msg("The settings file could not be read (" + err.Error() + "): it is kept as " + bad + ", the settings start afresh")
			} else {
				a.msg("The settings file could not be read: " + err.Error())
			}
		}
	}
	a.defFont = map[string]string{"amiga": "topaz1p", "pc": "vga", "c64": "c64upper", "atari": "atari", "st": "st8x8", "zx": "zx", "tt": "zx"}
	for k, v := range prefs.Font {
		a.defFont[k] = v
	}
	a.defPalette = map[string]string{"amiga": "amiga", "pc": "vga", "c64": "c64", "atari": "atari", "st": "st", "zx": "zx", "tt": "teletext"}
	for k, v := range prefs.Palette {
		a.defPalette[k] = v
	}
	a.defZoom = max(1, min(8, prefs.Zoom))
	if prefs.Zoom == 0 {
		a.defZoom = 2
	}
	a.grid = prefs.Grid
	a.defMachine = machineByID(prefs.Machine).id
	a.defSTRes = stResByID(prefs.STRes).id
	a.fkeys = defaultFkeys()
	for _, m := range pageModes {
		if f := prefs.Fkeys[m]; validFkeys(f) {
			a.fkeys[m] = f
		}
	}
	a.custom = palettes[0].colors
	if len(prefs.Custom) == 16 {
		for i, s := range prefs.Custom {
			if v, err := strconv.ParseUint(strings.TrimPrefix(s, "#"), 16, 32); err == nil {
				a.custom[i] = 0xFF000000 | uint32(v)
			}
		}
	}
	a.lastDir = prefs.Dir
	prefs.UIScale = max(1, min(maxUIScale, prefs.UIScale))
	if prefs.ViewSpeed < scrollSpeeds[0] || prefs.ViewSpeed > scrollSpeeds[len(scrollSpeeds)-1] {
		prefs.ViewSpeed = 4
	}
	if prefs.Width > 16384 || prefs.Height > 16384 {
		prefs.Width, prefs.Height = 0, 0
	}
	a.baud = 14400
	for _, b := range bauds {
		if b == prefs.Baud {
			a.baud = b
		}
	}
}

// validFkeys: F-key sets as prefs.json may hold them: at least one set, each
// of exactly ten bytes.
func validFkeys(sets [][]int) bool {
	if len(sets) == 0 {
		return false
	}
	for _, set := range sets {
		if len(set) != 10 {
			return false
		}
		for _, c := range set {
			if c < 0 || c > 255 {
				return false
			}
		}
	}
	return true
}

// readPrefsFile reads what prefs.json holds now (another CharEdit or CharView
// may have written it since this one started).
func readPrefsFile() (Prefs, bool) {
	var p Prefs
	b, err := os.ReadFile(prefsPath())
	if err != nil || json.Unmarshal(b, &p) != nil {
		return p, false
	}
	return p, true
}

func savePrefs() {
	if viewer != nil { // the viewer leaves the editor's settings alone
		return
	}
	p := prefs
	if now, ok := readPrefsFile(); ok { // the viewer's own settings are the viewer's
		p.ViewAuto, p.ViewStep, p.ViewSpeed = now.ViewAuto, now.ViewStep, now.ViewSpeed
		p.C64Asked = p.C64Asked || now.C64Asked
		for k, v := range now.ArtFonts { // fonts another CharEdit kept meanwhile
			if _, ok := p.ArtFonts[k]; !ok && !droppedArtFonts[k] {
				if p.ArtFonts == nil {
					p.ArtFonts = map[string]ArtFont{}
				}
				p.ArtFonts[k] = v
			}
		}
	}
	// The page being worked on sets what new tabs start with.
	for k, v := range a.fontFor {
		if !strings.HasPrefix(v, ownID) { // a page's own font stays with the page
			a.defFont[k] = v
		}
	}
	for k, v := range a.paletteFor {
		if v != "file" {
			a.defPalette[k] = v
		}
	}
	a.defZoom = a.zoom
	p.Mode, p.Font, p.Palette, p.Zoom, p.Ch8, p.Smear = a.mode, a.defFont, a.defPalette, a.zoom, a.ch8, a.smear
	p.Grid = a.grid
	p.Machine = a.defMachine
	p.STRes = a.defSTRes
	p.UIScale = scale
	p.Fkeys, p.Dir, p.Baud = a.fkeys, a.lastDir, a.baud
	p.Custom = nil
	for _, c := range a.custom {
		p.Custom = append(p.Custom, fmt.Sprintf("#%06x", c&0xFFFFFF))
	}
	if window != nil {
		w, h := window.GetSize()
		p.Width, p.Height = int(w), int(h)
	}
	prefs = p
	path := prefsPath()
	if path == "" {
		return
	}
	os.MkdirAll(filepath.Dir(path), 0o755)
	if b, err := json.MarshalIndent(p, "", "  "); err == nil {
		writeAtomic(path, append(b, '\n'))
	}
}
