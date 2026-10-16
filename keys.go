package main

// The editor's shortcuts: one table of actions (keys.go) drives them, the help
// lists them, and the keys file in configDir() changes them ("action = key, key").
// Typing, the F-keys of the F-key set, the arrows and the other cursor keys,
// Return, Tab, Backspace, Delete, Insert and Esc are fixed.

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/veandco/go-sdl2/sdl"
)

// Modifier bits of a key combination. Alt is the left Alt only: the right
// one is AltGr, which types characters on many keyboards.
const (
	modCtrl = 1 << iota
	modAlt
	modShift
)

// combo is a key with its modifiers; key is SDL's name for it ("A", "Up", "F1", "Keypad +").
type combo struct {
	mods uint8
	key  string
}

func (c combo) String() string {
	s := ""
	if c.mods&modCtrl != 0 {
		s += "Ctrl+"
	}
	if c.mods&modAlt != 0 {
		s += "Alt+"
	}
	if c.mods&modShift != 0 {
		s += "Shift+"
	}
	return s + c.key
}

func modsOf(ctrl, alt, shift bool) uint8 {
	var m uint8
	if ctrl {
		m |= modCtrl
	}
	if alt {
		m |= modAlt
	}
	if shift {
		m |= modShift
	}
	return m
}

// keyAliases are names people write that SDL does not know.
var keyAliases = map[string]string{"pgup": "PageUp", "pgdn": "PageDown", "pgdown": "PageDown", "esc": "Escape",
	"del": "Delete", "ins": "Insert", "enter": "Return", "comma": ",", "plus": "+", "minus": "-"}

// parseCombo reads "Ctrl+Shift+Up", "alt+f", "Ctrl++", "Ctrl+Keypad +" (any case).
func parseCombo(s string) (combo, error) {
	s = strings.TrimSpace(s)
	var c combo
	for {
		l := strings.ToLower(s)
		switch {
		case strings.HasPrefix(l, "ctrl+") && len(s) > 5:
			c.mods |= modCtrl
			s = s[5:]
		case strings.HasPrefix(l, "control+") && len(s) > 8:
			c.mods |= modCtrl
			s = s[8:]
		case strings.HasPrefix(l, "alt+") && len(s) > 4:
			c.mods |= modAlt
			s = s[4:]
		case strings.HasPrefix(l, "shift+") && len(s) > 6:
			c.mods |= modShift
			s = s[6:]
		default:
			name := strings.TrimSpace(s)
			if al, ok := keyAliases[strings.ToLower(name)]; ok {
				name = al
			}
			if name == "" {
				return c, fmt.Errorf("no key in %q", s)
			}
			code := sdl.GetKeyFromName(name)
			if code == sdl.K_UNKNOWN {
				return c, fmt.Errorf("unknown key %q", name)
			}
			c.key = sdl.GetKeyName(code)
			if c.key == "" {
				return c, fmt.Errorf("unknown key %q", name)
			}
			if typing := len(c.key) == 1 || c.key == "Space"; c.mods&(modCtrl|modAlt) == 0 && (typing || (c.mods == 0 && fixedKeys[c.key])) {
				// A plain key types or moves the cursor: it stays as it is.
				return c, fmt.Errorf("%s needs Ctrl or Alt", c)
			}
			if c.mods&modAlt != 0 && c.key == "F4" {
				return c, fmt.Errorf("Alt+F4 is left to the window manager")
			}
			return c, nil
		}
	}
}

// fixedKeys keep their own work without modifiers.
var fixedKeys = map[string]bool{"Up": true, "Down": true, "Left": true, "Right": true, "Home": true, "End": true,
	"PageUp": true, "PageDown": true, "Return": true, "Keypad Enter": true, "Tab": true, "Backspace": true,
	"Delete": true, "Insert": true, "Escape": true}

func fkeyName(k string) bool {
	if len(k) < 2 || k[0] != 'F' {
		return false
	}
	n, err := strconv.Atoi(k[1:])
	return err == nil && n >= 1 && n <= 24
}

// Action is one thing a key can do.
type Action struct {
	id   string
	keys string // the default keys, as the keys file writes them
	help string // one line for the help; "" joins the action before it
	run  func()
}

var (
	actions    []*Action
	actionByID = map[string]*Action{}
	keysNow    map[string][]combo // each action's keys now
	keymap     map[combo]string   // key -> action id
)

func init() {
	fg := func(i int) func() { return func() { a.colourKey(i, false) } }
	bg := func(i int) func() { return func() { a.colourKey(i, true) } }
	set := func(i int) func() { return func() { a.pickFset(i) } }
	actions = []*Action{
		{"help", "F12", "Open this help", openHelp},
		{"open", "Ctrl+O", "Open a file", openOpenRequester},
		{"open_recent", "Ctrl+Shift+O", "Open the most recent file again (the Open requester lists the last 10)", func() { a.reopenRecent() }},
		{"save", "Ctrl+S", "Save", func() { a.save(false) }},
		{"save_as", "Ctrl+Shift+S", "Save as (another name or format)", func() { a.save(true) }},
		{"info", "Ctrl+I", "Info: SAUCE title, author, group and save options", openInfo},
		{"new_page", "Ctrl+T, Ctrl+N", "New page (tab), on the start screen when it is on", func() { a.addTab() }},
		{"start_screen", "Ctrl+Shift+N", "Start screen: a new page for any machine, the recent files", func() { a.showStart() }},
		{"close_page", "Ctrl+W", "Close the page", func() { a.closeTab(a.Tab) }},
		{"next_page", "Ctrl+Tab, Ctrl+PageDown", "Next page", func() { a.cycleTab(1) }},
		{"previous_page", "Ctrl+Shift+Tab, Ctrl+PageUp", "Previous page", func() { a.cycleTab(-1) }},
		{"layout", "Ctrl+L", "One page / side by side / top and bottom", func() {
			a.setLayout(map[string]string{"one": "cols", "cols": "rows", "rows": "one"}[a.layout])
		}},
		{"quit", "Ctrl+Q", "Quit", requestQuit},

		{"undo", "Ctrl+Z", "Undo", func() { a.doUndo() }},
		{"redo", "Ctrl+Y, Ctrl+Shift+Z", "Redo", func() { a.doRedo() }},
		{"select_all", "Ctrl+A", "Select all", func() { a.sel = &Sel{0, 0, a.doc.cols - 1, max(0, a.doc.usedRows()-1)} }},
		{"copy", "Ctrl+C", "Copy the block", func() { a.copySel(false) }},
		{"cut", "Ctrl+X", "Cut the block", func() { a.copySel(true) }},
		{"paste", "Ctrl+V", "Paste (text from other programs too)", func() { a.paste() }},
		{"mirror", "Alt+H", "Mirror the block left-right (nothing selected: the brush character)", func() { a.blockOp(flipH) }},
		{"upside_down", "Alt+V", "Turn the block upside down (nothing selected: the brush character)", func() { a.blockOp(flipV) }},
		{"turn", "Alt+R", "Turn the block a quarter clockwise (nothing selected: the brush character)", func() { a.blockOp(turnCW) }},
		{"invert", "Alt+I", "Invert the block (reversed characters, or the two colours swapped)", func() { a.blockOp(invertBlock) }},

		{"font_load", "Ctrl+Shift+L", "Load a font or character set for the page: PC .fnt/.f08/.f14/.f16/.psf, C64 .64c/.bin, Atari .fnt, Spectrum .ch8 and UDGs", openFontRequester},
		{"font_editor", "Ctrl+Shift+E", "Font editor: draw the brush character's pixels, the page shows them at once; save the font as a file", openFontEditor},

		{"fill_block", "Alt+F", "Fill the selection with the brush (nothing selected: the Fill tool)", func() { a.fillSel() }},
		{"crop", "Ctrl+K", "Crop the page to the selection (Amiga and PC pages)", func() { a.cropToSel() }},

		{"frame_next", "Alt+PageDown, Alt+.", "Next frame", func() { a.goFrame(a.cur + 1) }},
		{"frame_previous", "Alt+PageUp, Alt+Comma", "Previous frame", func() { a.goFrame(a.cur - 1) }},
		{"frame_first", "Alt+Home", "First frame", func() { a.goFrame(0) }},
		{"frame_last", "Alt+End", "Last frame", func() { a.goFrame(a.nFrames() - 1) }},
		{"frame_new", "Alt+N", "New empty frame after this one", func() { a.newFrame() }},
		{"frame_duplicate", "Ctrl+Shift+D", "Copy the frame: a new frame after it, the same picture", func() { a.dupFrame() }},
		{"frame_delete", "Alt+Delete", "Delete the frame", func() { a.deleteFrame() }},
		{"frame_earlier", "Alt+Shift+PageUp", "Move the frame earlier", func() { a.moveFrame(-1) }},
		{"frame_later", "Alt+Shift+PageDown", "Move the frame later", func() { a.moveFrame(1) }},
		{"frame_hold_less", "Alt+[, Alt+Keypad -", "Show the frame a shorter time when played", func() { a.stepHold(-1) }},
		{"frame_hold_more", "Alt+], Alt+Keypad +", "Show the frame a longer time when played", func() { a.stepHold(1) }},
		{"play", "Ctrl+P", "Play the frames (one frame: play it as if it came over a modem); again or Esc stops", func() { a.playKey() }},
		{"onion", "Alt+O", "Onion skin on / off: the frame before, faint, under the empty cells", func() { a.toggleOnion() }},
		{"frames_strip", "Ctrl+Shift+F", "Show / hide the frame strip", func() { a.toggleStrip() }},

		// Layers (layers.go).
		{"layer_new", "Alt+Shift+N", "New layer above this one: what you draw on it goes over the layers below, its empty cells show them", func() { a.newLayer(false) }},
		{"layer_copy", "", "Copy the layer: a new layer above it, the same picture", func() { a.newLayer(true) }},
		{"layer_delete", "", "Delete the layer", func() { a.deleteLayer() }},
		{"layer_above", "Ctrl+Shift+PageUp", "Work on the layer above", func() { a.goLayer(a.lay + 1) }},
		{"layer_below", "Ctrl+Shift+PageDown", "Work on the layer below", func() { a.goLayer(a.lay - 1) }},
		{"layer_raise", "Alt+Shift+Up", "Move the layer up, over the one above it", func() { a.moveLayer(1) }},
		{"layer_lower", "Alt+Shift+Down", "Move the layer down, under the one below it", func() { a.moveLayer(-1) }},
		{"layer_merge_down", "Alt+Shift+M", "Merge the layer into the one below", func() { a.mergeDown() }},
		{"layer_merge_all", "", "Merge all layers into one: what is seen (hidden layers are left out)", func() { a.mergeAll() }},
		{"layer_show", "Alt+Shift+E", "Show / hide the layer", func() { a.toggleLayer(a.lay) }},
		{"layer_lock", "", "Lock / unlock the layer: no tool changes a locked layer", func() { a.toggleLock(a.lay) }},
		{"layer_black", "", "Black shows below: on this layer a black background lets the background below show, for letters over a picture", func() { a.toggleBlackClear() }},
		{"layer_others", "", "The other layers: shown, dim or hidden while you work on this one", func() { a.cycleOthers() }},
		{"copy_seen", "", "Copy what is seen: the selection with every visible layer, not only the current one", func() { a.copySeen() }},
		{"animate", "Alt+J", "Animate: TheDraw's scan patterns (and fades, typewriter, scroll) as the saved file's draw order or as frames", openAnimate},
		{"party_check", "", "Check for a party: what the page breaks of a compo's rules (size, colours, font, animation)", openPartyCheck},
		{"collab_join", "", "Join a drawing session: a Moebius or IcyDraw server's page in a new tab, drawn on together", openCollabJoin},
		{"collab_host", "", "Host a drawing session: share the page, Moebius and IcyDraw users join it", openCollabHost},
		{"collab_end", "", "End the drawing session: leave it, or stop hosting (the page stays)", collabEndKey},
		{"collab_chat", "Ctrl+Return", "Chat in the drawing session: Return sends, Esc goes back to the page", collabChatKey},
		{"work_stages", "", "Export work stages: pictures of the work from its start to now, for compos that want them", func() { openStagesReq(nil) }},
		{"animate_folder", "Alt+Shift+J", "Animate a folder: every screen of a folder animated for your BBS at once, into a new folder (also: charedit --animate)", openAnimateFolder},

		// The reference picture (refimage.go).
		{"ref_open", "Ctrl+Shift+I", "Open a picture as the page's reference, to trace over (never saved with the art)", openRefRequester},
		{"ref_show", "Ctrl+Shift+R", "Show / hide the reference picture", func() { a.refShow() }},
		{"ref_fainter", "Ctrl+[", "Reference fainter", func() { a.refFade(-1) }},
		{"ref_stronger", "Ctrl+]", "Reference stronger", func() { a.refFade(1) }},
		{"ref_move", "Ctrl+Shift+M", "Move the reference: drag, or arrows (Shift: a pixel), + and - size it; again, Return or Esc ends (Ctrl+drag moves it any time)", func() { a.toggleRefMove() }},
		{"ref_bigger", "Ctrl+Alt+=", "Reference bigger", func() { a.refScaleBy(1.1) }},
		{"ref_smaller", "Ctrl+Alt+-", "Reference smaller", func() { a.refScaleBy(1 / 1.1) }},
		{"ref_fit_width", "", "Reference as wide as the page", func() { a.refSize("width") }},
		{"ref_fit_page", "", "Reference fits the page", func() { a.refSize("page") }},
		{"ref_actual", "", "Reference at 1:1 (a picture pixel for a pixel of the font)", func() { a.refSize("1") }},
		{"ref_over", "", "Reference over everything (faint) or under the art (in the empty cells)", func() { a.refOver() }},
		{"ref_grey", "", "Reference in greys or in colour", func() { a.refGrey() }},
		{"ref_remove", "", "Take the reference picture away", func() { a.refRemove() }},

		{"select_left", "Shift+Left", "Select with the keyboard: from the cursor, growing as it moves (Esc lets go)", func() { a.selectKey("Left") }},
		{"select_right", "Shift+Right", "", func() { a.selectKey("Right") }},
		{"select_up", "Shift+Up", "", func() { a.selectKey("Up") }},
		{"select_down", "Shift+Down", "", func() { a.selectKey("Down") }},
		{"select_home", "Shift+Home", "", func() { a.selectKey("Home") }},
		{"select_end", "Shift+End", "", func() { a.selectKey("End") }},
		{"select_page_up", "Shift+PageUp", "", func() { a.selectKey("PageUp") }},
		{"select_page_down", "Shift+PageDown", "", func() { a.selectKey("PageDown") }},
		{"page_top", "Ctrl+Home", "Cursor to the top of the page", func() { a.pageTop() }},
		{"page_bottom", "Ctrl+End", "Cursor to the bottom of the page", func() { a.pageBottom() }},
		{"delete_row", "Alt+Up", "Delete the cursor's row", func() { a.deleteRow() }},
		{"insert_row", "Alt+Down", "Insert a row at the cursor", func() { a.insertRow() }},
		{"delete_column", "Alt+Left", "Delete the cursor's column", func() { a.deleteColumn() }},
		{"insert_column", "Alt+Right", "Insert a column at the cursor (machine pages keep their width: the last column falls off)", func() { a.insertColumn() }},
		{"tool_type", "Alt+T", "Type tool", func() { a.toolKey("type") }},
		{"tool_draw", "Alt+D", "Draw tool", func() { a.toolKey("draw") }},
		{"tool_colour", "Alt+K", "Colour tool", func() { a.toolKey("colour") }},
		{"tool_fill", "Alt+L", "Fill tool", func() { a.toolKey("fill") }},
		{"tool_pick", "Alt+P", "Pick tool", func() { a.toolKey("pick") }},
		{"tool_select", "Alt+S", "Select tool", func() { a.toolKey("select") }},
		{"tool_half", "Alt+B", "Half tool (PC pages), Sixel tool (teletext, Minitel)", func() { a.toolKey("half") }},
		{"tool_logo", "Alt+G", "Logo tool: type in a TheDraw font (PC, Amiga and Unicode pages)", func() { a.toolKey("logo") }},
		{"logo_font", "Alt+Shift+G", "TheDraw fonts: the list, with a preview of each (add folders, download a collection)", func() { openTDFPicker(nil) }},
		{"logo_line", "Ctrl+Shift+G", "Type a line in the TheDraw font and stamp it", openTDFLine},

		// Shapes and brushes (shapes.go).
		{"shape_line", "Alt+Shift+L", "Shape tool: line (drag; the shape tool's choices are under the brush)", func() { a.shapeKey(shapeLine) }},
		{"shape_box", "Alt+Shift+R", "Shape tool: box", func() { a.shapeKey(shapeBox) }},
		{"shape_ellipse", "Alt+Shift+O", "Shape tool: ellipse", func() { a.shapeKey(shapeEllipse) }},
		{"shape_filled", "Alt+Shift+F", "Shapes filled or outlined (Shift at the press does the other)", func() { a.toggleFilled() }},
		{"shape_style", "Alt+Shift+B", "Shapes in the brush, box-drawing lines or half blocks (by page type)", func() { a.cycleShapeStyle() }},
		{"shade_brush", "Alt+Shift+S", "Shade brush on / off (Draw: left steps a cell up the shades, right down)", func() { a.toggleShade() }},
		{"mirror_left_right", "Alt+Shift+H", "Symmetry: mirror what is drawn left-right around the page's centre", func() { a.toggleMirror(mirrorX) }},
		{"mirror_top_bottom", "Alt+Shift+V", "Symmetry: mirror what is drawn top-bottom", func() { a.toggleMirror(mirrorY) }},
		{"paint_colour_only", "Alt+Shift+K", "Draw and shapes paint only colours (the characters stay)", func() { a.setPaintOnly("colour") }},
		{"paint_char_only", "Alt+Shift+C", "Draw and shapes paint only characters (the colours stay)", func() { a.setPaintOnly("char") }},

		{"fg_previous", "Ctrl+Up", "Foreground colour -1", func() { a.stepFg(-1) }},
		{"fg_next", "Ctrl+Down", "Foreground colour +1", func() { a.stepFg(1) }},
		{"bg_previous", "Ctrl+Left", "Background colour -1", func() { a.stepBg(-1) }},
		{"bg_next", "Ctrl+Right", "Background colour +1", func() { a.stepBg(1) }},
		{"fg_0", "Ctrl+0", "Foreground colour 0-7 (the same again: its bright twin, 8-15)", fg(0)},
		{"fg_1", "Ctrl+1", "", fg(1)},
		{"fg_2", "Ctrl+2", "", fg(2)},
		{"fg_3", "Ctrl+3", "", fg(3)},
		{"fg_4", "Ctrl+4", "", fg(4)},
		{"fg_5", "Ctrl+5", "", fg(5)},
		{"fg_6", "Ctrl+6", "", fg(6)},
		{"fg_7", "Ctrl+7", "", fg(7)},
		{"bg_0", "Alt+0", "Background colour 0-7 (the same again: 8-15; PETSCII and Atari: the screen colour)", bg(0)},
		{"bg_1", "Alt+1", "", bg(1)},
		{"bg_2", "Alt+2", "", bg(2)},
		{"bg_3", "Alt+3", "", bg(3)},
		{"bg_4", "Alt+4", "", bg(4)},
		{"bg_5", "Alt+5", "", bg(5)},
		{"bg_6", "Alt+6", "", bg(6)},
		{"bg_7", "Alt+7", "", bg(7)},
		{"take_colours", "Alt+U", "Take the character and colours under the cursor", func() { a.pick(a.cx, a.cy) }},
		{"default_colours", "Ctrl+D", "Default colours (7 on 0; the machine's own on other pages)", func() { a.defaultColours() }},
		{"swap_colours", "Ctrl+Shift+X", "Swap foreground and background", func() { a.swapColours() }},
		{"ice", "Ctrl+E", "iCE colours on / off (PC and Amiga: backgrounds 8-15 instead of blink)", func() { a.toggleIce() }},
		{"ninepx", "Alt+9", "9-pixel letter spacing on / off (PC pages: VGA's 9-dot cells, kept in the SAUCE record)", func() { a.toggleNinePx() }},
		{"convert_unicode", "", "Make the page a Unicode page (PC: CP437 to Unicode, exact; undoable)", func() { a.start = false; a.setMode("uni") }},
		{"uni_block_next", "Ctrl+Alt+PageDown", "Unicode page: the next block of characters in the table", func() { a.setUniBlock(a.uniBlock + 1) }},
		{"uni_block_previous", "Ctrl+Alt+PageUp", "Unicode page: the block before", func() { a.setUniBlock(a.uniBlock - 1) }},
		{"uni_colour", "", "Unicode page: a 24-bit colour for the brush", func() { openUniColour() }},
		{"fkeys_1", "Ctrl+F1", "F-key set 1-10", set(0)},
		{"fkeys_2", "Ctrl+F2", "", set(1)},
		{"fkeys_3", "Ctrl+F3", "", set(2)},
		{"fkeys_4", "Ctrl+F4", "", set(3)},
		{"fkeys_5", "Ctrl+F5", "", set(4)},
		{"fkeys_6", "Ctrl+F6", "", set(5)},
		{"fkeys_7", "Ctrl+F7", "", set(6)},
		{"fkeys_8", "Ctrl+F8", "", set(7)},
		{"fkeys_9", "Ctrl+F9", "", set(8)},
		{"fkeys_10", "Ctrl+F10", "", set(9)},
		{"fkeys_next", "Ctrl+Shift+Up", "Next F-key set", func() { a.setFset(a.fset + 1) }},
		{"fkeys_previous", "Ctrl+Shift+Down", "Previous F-key set", func() { a.setFset(a.fset - 1) }},

		{"zoom_in", "Ctrl+=, Ctrl+Shift+=, Ctrl++, Ctrl+Keypad +", "Zoom in", func() { a.setZoom(a.zoom + 1) }},
		{"zoom_out", "Ctrl+-, Ctrl+Keypad -", "Zoom out", func() { a.setZoom(a.zoom - 1) }},
		{"fit_width", "Ctrl+9", "Zoom to fit the page's width", func() { a.fitZoom() }},
		{"grid", "Ctrl+G", "Grid on / off", func() { a.grid = !a.grid; savePrefs() }},
		{"reverse", "Ctrl+R", "Reverse characters on / off (PETSCII, Atari)", func() { a.rev = !a.rev }},
		{"hide_gadgets", "Ctrl+H", "Hide all gadgets (only the art and the status line), again brings them back", func() {
			a.bare = !a.bare
			if a.bare {
				a.msg("Gadgets hidden: " + keyName("hide_gadgets") + " brings them back")
			}
		}},
	}
	for _, ac := range actions {
		actionByID[ac.id] = ac
	}
	setKeys(defaultKeys())
}

// defaultKeys gives every action its default keys.
func defaultKeys() map[string][]combo {
	m := map[string][]combo{}
	for _, ac := range actions {
		m[ac.id] = nil
		for _, s := range strings.Split(ac.keys, ",") {
			if strings.TrimSpace(s) == "" {
				continue
			}
			c, err := parseCombo(s)
			if err != nil {
				panic("keys.go: " + ac.id + ": " + err.Error())
			}
			m[ac.id] = append(m[ac.id], c)
		}
	}
	return m
}

// setKeys makes k the keys in use.
func setKeys(k map[string][]combo) {
	keysNow = k
	keymap = map[combo]string{}
	for _, ac := range actions { // in table order, so a key given twice goes to the first
		for _, c := range k[ac.id] {
			if _, taken := keymap[c]; !taken {
				keymap[c] = ac.id
			}
		}
	}
}

// keyName is how the help and messages name an action's first key.
func keyName(id string) string {
	if ks := keysNow[id]; len(ks) > 0 {
		return ks[0].String()
	}
	return "(no key)"
}

// runKey does what the key is bound to; false when it is bound to nothing.
func runKey(c combo) bool {
	id, ok := keymap[c]
	if !ok {
		return false
	}
	actionByID[id].run()
	return true
}

// applyKeyFile reads the keys file's lines over the defaults. A line gives an
// action all its keys; a key it takes is taken away from any other action.
// Lines that are not understood are skipped and named in problems.
func applyKeyFile(text string) (map[string][]combo, []string) {
	preset := keyFilePreset(text)
	k := presetKeys(preset) // a "preset = moebius" line: another editor's keys (keypresets.go)
	var problems []string
	if preset != "" && presetByID(preset) == nil {
		problems = append(problems, fmt.Sprintf("no key preset called %q", preset))
	}
	given := map[combo]string{}
	for n, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		id, list, ok := strings.Cut(line, "=")
		id = strings.ToLower(strings.TrimSpace(id))
		if !ok {
			problems = append(problems, fmt.Sprintf("line %d has no =", n+1))
			continue
		}
		if id == "preset" {
			continue
		}
		if actionByID[id] == nil {
			problems = append(problems, fmt.Sprintf("line %d: no action called %q", n+1, id))
			continue
		}
		if preset != "" && sameKeys(list, actionByID[id].keys) {
			continue // the file only repeats CharEdit's default: the preset's keys stay
		}
		var keys []combo
		bad := false
		for _, s := range strings.Split(list, ",") {
			if strings.TrimSpace(s) == "" {
				continue
			}
			c, err := parseCombo(s)
			if err != nil {
				problems = append(problems, fmt.Sprintf("line %d: %v", n+1, err))
				bad = true
				continue
			}
			keys = append(keys, c)
		}
		if bad && len(keys) == 0 {
			continue // keep the defaults rather than leave the action without a key by mistake
		}
		for _, c := range keys {
			if other, ok := given[c]; ok && other != id {
				problems = append(problems, fmt.Sprintf("line %d: %s is also given to %s", n+1, c, other))
			}
			given[c] = id
		}
		k[id] = keys
	}
	// A key the file gives to an action is taken from the defaults of the others.
	for id, keys := range k {
		out := keys[:0:0]
		for _, c := range keys {
			if owner, ok := given[c]; !ok || owner == id {
				out = append(out, c)
			}
		}
		k[id] = out
	}
	return k, problems
}

func keysPath() string { return filepath.Join(configDir(), "keys") }

// keyFileText is the keys file with every action at its default keys.
func keyFileText() string {
	var b strings.Builder
	b.WriteString(`# CharEdit's keys. One action a line: action = key, key
# Write keys like Ctrl+Z, Ctrl+Shift+Up, Alt+F, Ctrl+F1, Ctrl+Keypad +. Alt means the left Alt only (the
# right one is AltGr, for typing characters). Nothing after = leaves the action without a key.
# On a Mac, Ctrl here means Cmd or Ctrl, and Alt the left Option key (the right one types characters).
# A line that is not understood is skipped and named in the status line. Delete this file to get the
# defaults back (it is written again at the next start).
# Fixed, not set here: typing, F1-F10 (the F-key set), the arrows, Home, End, PgUp, PgDn, Return, Tab,
# Backspace, Delete, Insert and Esc. Alt+F4 is the window manager's.
# A line "preset = moebius" (or pablodraw, thedraw, aciddraw) takes that editor's keys; the lines below
# still win where they differ from CharEdit's own. Settings > Keys like writes it.
`)
	last := ""
	for _, ac := range actions {
		if ac.help != "" {
			last = ac.help
			b.WriteString("\n# " + last + "\n")
		}
		fmt.Fprintf(&b, "%s = %s\n", ac.id, ac.keys)
	}
	return b.String()
}

// loadKeys reads the keys file, writing it with the defaults first if it is not there.
func loadKeys() {
	p := keysPath()
	if p == "" {
		return
	}
	b, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		os.MkdirAll(filepath.Dir(p), 0o755)
		writeAtomic(p, []byte(keyFileText()))
		return
	}
	if err != nil {
		a.msg("The keys file could not be read: " + err.Error())
		return
	}
	k, problems := applyKeyFile(string(b))
	setKeys(k)
	if len(problems) > 0 {
		more := ""
		if len(problems) > 1 {
			more = fmt.Sprintf(" (and %d more)", len(problems)-1)
		}
		a.msg("Keys file " + p + ": " + problems[0] + more + "; skipped")
	}
}

// keysHelp is the KEYS part of the help, made from the table so it is always right.
func keysHelp() string {
	var lines []string
	for i := 0; i < len(actions); {
		j := i + 1
		for j < len(actions) && actions[j].help == "" {
			j++
		}
		if keys := groupKeys(actions[i:j]); keys != "(no key)" { // the keys file can give it one
			lines = append(lines, keys+": "+actions[i].help)
		}
		i = j
	}
	return strings.Join(lines, "\n")
}

// groupKeys names the keys of a run of actions: "Ctrl+0-7" when they are one
// key each in a row of digits or F-keys, else all of them.
func groupKeys(g []*Action) string {
	if len(g) > 1 {
		var first combo
		prefix, start, ok := "", 0, true
		for i, ac := range g {
			ks := keysNow[ac.id]
			if len(ks) != 1 {
				ok = false
				break
			}
			c := ks[0]
			p, num := c.key, -1
			if n, err := strconv.Atoi(c.key); err == nil && len(c.key) == 1 {
				p, num = "", n
			} else if fkeyName(c.key) {
				n, _ := strconv.Atoi(c.key[1:])
				p, num = "F", n
			}
			if i == 0 {
				first, prefix, start = c, p, num
			}
			if num < 0 || c.mods != first.mods || p != prefix || num != start+i {
				ok = false
				break
			}
		}
		if ok {
			last := keysNow[g[len(g)-1].id][0]
			return first.String() + "-" + last.key
		}
	}
	var names []string
	same := len(g) > 1
	for _, ac := range g {
		ks := keysNow[ac.id]
		same = same && len(ks) == 1 && ks[0].mods == keysNow[g[0].id][0].mods
		for _, c := range ks {
			names = append(names, c.String())
		}
	}
	if same { // Shift+Left/Right/Up/Down
		first := keysNow[g[0].id][0]
		for i := 1; i < len(g); i++ {
			names[i] = keysNow[g[i].id][0].key
		}
		names[0] = first.String()
		return strings.Join(names, "/")
	}
	if len(names) == 0 {
		return "(no key)"
	}
	return strings.Join(names, ", ")
}

// sameKeys: the two key lists name the same keys (in any order and case).
func sameKeys(a, b string) bool {
	norm := func(s string) map[combo]bool {
		m := map[combo]bool{}
		for _, p := range strings.Split(s, ",") {
			if c, err := parseCombo(p); err == nil {
				m[c] = true
			}
		}
		return m
	}
	x, y := norm(a), norm(b)
	if len(x) != len(y) {
		return false
	}
	for c := range x {
		if !y[c] {
			return false
		}
	}
	return true
}
