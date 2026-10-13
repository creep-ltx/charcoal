package main

// Settings kept between runs, in ~/.config/charcoal/prefs.json.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Prefs struct {
	Mode      string             `json:"mode"`
	Font      map[string]string  `json:"font"`
	Palette   map[string]string  `json:"palette"`
	Zoom      int                `json:"zoom"`
	Ch8       bool               `json:"ch8"`
	Smear     bool               `json:"smear"`
	Fkeys     map[string][][]int `json:"fkeys"`
	Custom    []string           `json:"custom"`
	Dir       string             `json:"dir"`
	Baud      int                `json:"baud"`
	UIScale   int                `json:"uiscale"`
	Grid      bool               `json:"grid"`
	Machine   string             `json:"machine"`
	STRes     string             `json:"st_resolution"`
	ViewAuto  bool               `json:"view_auto_scroll"`
	ViewStep  bool               `json:"view_step_scroll"`
	ViewSpeed float64            `json:"view_scroll_speed"`
	C64Asked  bool               `json:"c64asked"` // the first-run question about the C64 character set
	Width     int                `json:"width"`
	Height    int                `json:"height"`
}

var prefs Prefs

func prefsPath() string {
	d, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(d, "charcoal", "prefs.json")
}

func loadPrefs() {
	if b, err := os.ReadFile(prefsPath()); err == nil {
		json.Unmarshal(b, &prefs)
	}
	a.defFont = map[string]string{"amiga": "topaz1p", "pc": "vga", "c64": "c64upper", "atari": "atari", "st": "st8x8", "zx": "zx"}
	for k, v := range prefs.Font {
		a.defFont[k] = v
	}
	a.defPalette = map[string]string{"amiga": "amiga", "pc": "vga", "c64": "c64", "atari": "atari", "st": "st", "zx": "zx"}
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
	for _, m := range []string{"amiga", "pc", "c64", "atari", "st", "zx"} {
		if f := prefs.Fkeys[m]; len(f) > 0 {
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
	a.baud = 14400
	for _, b := range bauds {
		if b == prefs.Baud {
			a.baud = b
		}
	}
}

func savePrefs() {
	if viewer != nil { // the viewer leaves the editor's settings alone
		return
	}
	p := prefs
	// The page being worked on sets what new tabs start with.
	for k, v := range a.fontFor {
		if v != "xbin" { // a file's own font stays with the file
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
