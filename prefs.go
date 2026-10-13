package main

// Settings kept between runs, in ~/.config/amiansi/prefs.json.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type Prefs struct {
	Mode    string             `json:"mode"`
	Font    map[string]string  `json:"font"`
	Palette map[string]string  `json:"palette"`
	Zoom    int                `json:"zoom"`
	Ch8     bool               `json:"ch8"`
	Smear   bool               `json:"smear"`
	Fkeys   map[string][][]int `json:"fkeys"`
	Custom  []string           `json:"custom"`
	Dir     string             `json:"dir"`
	Baud    int                `json:"baud"`
	UIScale int                `json:"uiscale"`
	Width   int                `json:"width"`
	Height  int                `json:"height"`
}

var prefs Prefs

func prefsPath() string {
	d, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(d, "amiansi", "prefs.json")
}

func loadPrefs() {
	if b, err := os.ReadFile(prefsPath()); err == nil {
		json.Unmarshal(b, &prefs)
	}
	a.mode = "amiga"
	if prefs.Mode == "pc" {
		a.mode = "pc"
	}
	a.fontFor = map[string]string{"amiga": "topaz1p", "pc": "vga"}
	for k, v := range prefs.Font {
		a.fontFor[k] = v
	}
	a.paletteFor = map[string]string{"amiga": "amiga", "pc": "vga"}
	for k, v := range prefs.Palette {
		a.paletteFor[k] = v
	}
	a.zoom = max(1, min(4, prefs.Zoom))
	if prefs.Zoom == 0 {
		a.zoom = 2
	}
	a.ch8, a.smear = prefs.Ch8, prefs.Smear
	a.fkeys = defaultFkeys()
	if f := prefs.Fkeys; len(f["amiga"]) > 0 && len(f["pc"]) > 0 {
		a.fkeys = f
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
	p := prefs
	p.Mode, p.Font, p.Palette, p.Zoom, p.Ch8, p.Smear = a.mode, a.fontFor, a.paletteFor, a.zoom, a.ch8, a.smear
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
