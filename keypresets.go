package main

// Key presets: CharEdit's actions on the keys of another editor, for people who
// come from it. The keys file chooses one with a line "preset = moebius" (Settings >
// Keys like writes it); the file's own lines still win over it.
//
// Each preset gives some actions that editor's keys (read from its source or
// manual: the knowledge base, bbs/text-art-editor-keys.md). An action it does not
// name keeps CharEdit's key, unless the preset gave that key to another action:
// then it has no key (the keys file can give it one). What cannot be matched:
// plain letters (Moebius's and PabloDraw's block keys M C F E S R X Y T U O while a
// block is up, Moebius's tool letters K B I P) type, and Alt+F4 is the window
// manager's, so F-key set 4 stays on Ctrl+F4.

import (
	"fmt"
	"os"
	"strings"
)

type keyPreset struct {
	id, name string
	keys     map[string]string // action -> keys, as the keys file writes them
}

// fkeySets gives F-key sets 1-10 on Alt+F1-F10 (set 4: Ctrl+F4).
func fkeySets(m map[string]string) map[string]string {
	for i := 1; i <= 10; i++ {
		k := fmt.Sprintf("Alt+F%d", i)
		if i == 4 {
			k = "Ctrl+F4"
		}
		m[fmt.Sprintf("fkeys_%d", i)] = k
	}
	return m
}

var keyPresets = []keyPreset{
	{"charedit", "CharEdit", map[string]string{}},
	// Moebius 4.x: app/menu.js accelerators, app/document/input/keyboard.js.
	{"moebius", "Moebius", fkeySets(map[string]string{
		"new_page":          "Ctrl+N, Ctrl+T",
		"redo":              "Ctrl+Y, Ctrl+Shift+Z",
		"fkeys_next":        "Ctrl+.",
		"fkeys_previous":    "Ctrl+Comma",
		"fkeys_1":           "Alt+F1, Ctrl+/", // Ctrl+/: Default Character Set
		"ninepx":            "Ctrl+F",         // Use 9px Font
		"ref_show":          "Ctrl+Tab",       // Toggle Reference Image
		"next_page":         "Ctrl+PageDown",
		"mirror_left_right": "Ctrl+Alt+M", // Mirror Mode
		"tool_select":       "Alt+B",      // Start Selection
		"zoom_in":           "Ctrl+=, Ctrl+Shift+=, Ctrl++, Ctrl+Keypad +",
		"zoom_out":          "Ctrl+-, Ctrl+Keypad -",
	})},
	// PabloDraw 3: Source/Pablo/Formats/Character/Actions/**.cs, Tools/*.cs, CharacterHandler.cs.
	{"pablodraw", "PabloDraw", fkeySets(map[string]string{
		"page_top":      "Ctrl+PageUp, Ctrl+Home",
		"page_bottom":   "Ctrl+PageDown, Ctrl+End",
		"next_page":     "Ctrl+Tab",
		"previous_page": "Ctrl+Shift+Tab",
		"tool_draw":     "Alt+P, Alt+B", // pencil, brush
		"tool_colour":   "Alt+Shift+B",  // colour brush
		"tool_pick":     "Alt+I",        // ink dropper
		"tool_select":   "Alt+E",        // selection
		"shape_line":    "Alt+L",        // line
		"shape_box":     "Alt+R",        // rectangle
		"shape_ellipse": "Alt+O",        // ellipse
		"redo":          "Ctrl+Y, Ctrl+Shift+Z",
	})},
	// TheDraw 4.63: THEDRAW.DOC, the command summary.
	{"thedraw", "TheDraw", fkeySets(map[string]string{
		"open":         "Alt+L, Ctrl+O",
		"save":         "Alt+S, Ctrl+S",
		"quit":         "Alt+X, Ctrl+Q",
		"help":         "Alt+H, F12",
		"undo":         "Alt+R, Ctrl+Z", // Restore/Undo Changes
		"insert_row":   "Alt+I, Alt+Down",
		"delete_row":   "Alt+Y, Alt+Up",
		"take_colours": "Alt+U, Alt+/",
		"tool_select":  "Alt+B", // Block Action Commands
		"tool_fill":    "Alt+P", // Paint/Fill Commands
		"tool_colour":  "Alt+-", // Color Draw Mode
		"fg_next":      "Ctrl+Up",
		"fg_previous":  "Ctrl+Down",
	})},
	// ACiDDraw 1.25: ACIDDRAW.DOC.
	{"aciddraw", "ACiDDraw", fkeySets(map[string]string{
		"open":        "Alt+L, Ctrl+O",
		"save":        "Alt+S, Ctrl+S",
		"quit":        "Alt+X, Ctrl+Q",
		"help":        "Alt+H, F12",
		"undo":        "Alt+R, Ctrl+Z", // restores the backup taken before a block command
		"insert_row":  "Ctrl+I, Alt+Down",
		"delete_row":  "Ctrl+Y, Alt+Up",
		"tool_select": "Alt+B",
		"fg_next":     "Ctrl+Up",
		"fg_previous": "Ctrl+Down",
		"redo":        "Ctrl+Shift+Z", // Ctrl+Y deletes a row there (and Ctrl+I, Info's key, inserts one)
	})},
}

func presetByID(id string) *keyPreset {
	for i := range keyPresets {
		if keyPresets[i].id == strings.ToLower(strings.TrimSpace(id)) {
			return &keyPresets[i]
		}
	}
	return nil
}

// presetKeys gives every action its keys under the preset (CharEdit's for "" or an unknown one).
func presetKeys(id string) map[string][]combo {
	k := defaultKeys()
	p := presetByID(id)
	if p == nil {
		return k
	}
	taken := map[combo]bool{}
	for act, list := range p.keys {
		k[act] = nil
		for _, s := range strings.Split(list, ",") {
			if strings.TrimSpace(s) == "" {
				continue
			}
			c, err := parseCombo(s)
			if err != nil {
				panic("keypresets.go: " + p.id + " " + act + ": " + err.Error())
			}
			k[act] = append(k[act], c)
			taken[c] = true
		}
	}
	for act, keys := range k {
		if _, own := p.keys[act]; own {
			continue
		}
		out := keys[:0:0]
		for _, c := range keys {
			if !taken[c] {
				out = append(out, c)
			}
		}
		k[act] = out
	}
	return k
}

// keyFilePreset is the preset a keys file chooses ("preset = moebius"), "" for none.
func keyFilePreset(text string) string {
	for _, line := range strings.Split(text, "\n") {
		id, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if ok && strings.EqualFold(strings.TrimSpace(id), "preset") {
			return strings.ToLower(strings.TrimSpace(v))
		}
	}
	return ""
}

// setKeyPreset writes the preset into the keys file (the user's own lines stay)
// and makes it the keys in use. The file as it was is kept as keys.bak.
func setKeyPreset(id string) error {
	p := keysPath()
	old, err := os.ReadFile(p)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err == nil {
		writeAtomic(p+".bak", old)
	}
	text := string(old)
	if err != nil {
		text = keyFileText()
	}
	var lines []string
	for _, l := range strings.Split(text, "\n") {
		if k, _, ok := strings.Cut(strings.TrimSpace(l), "="); ok && strings.EqualFold(strings.TrimSpace(k), "preset") {
			continue
		}
		lines = append(lines, l)
	}
	if id != "" && id != "charedit" {
		lines = append([]string{"preset = " + id}, lines...)
	}
	if err := writeAtomic(p, []byte(strings.Join(lines, "\n"))); err != nil {
		return err
	}
	loadKeys()
	return nil
}

// drawKeysLike is Settings' "Keys like" row; it returns the y below it.
func drawKeysLike(u *UI, in Rect, y int) int {
	u.fb.text(in.x, y+3, "Keys like", cText)
	var names []string
	cur := 0
	b, _ := os.ReadFile(keysPath())
	now := keyFilePreset(string(b))
	for i, p := range keyPresets {
		names = append(names, p.name)
		if p.id == now {
			cur = i
		}
	}
	u.choice("keyslike", Rect{in.x + settingsCol(), y, choiceW(names), gadH}, names, cur,
		"Another editor's keys for CharEdit's actions (your own lines in the keys file still win; the file before is kept as keys.bak). Help (F12) lists the keys in use", func(i int) {
			if err := setKeyPreset(keyPresets[i].id); err != nil {
				a.msg("Could not write the keys file: " + err.Error())
				return
			}
			a.msg("Keys like " + keyPresets[i].name + " (the file before is " + keysPath() + ".bak)")
		})
	return y + gadH + 8
}
