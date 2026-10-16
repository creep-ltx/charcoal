package main

// The menu bar: every command in one logical place, with its keys beside it.
// An item names an action in keys.go, so the keys shown are the ones in use
// (a key preset or the user's own keys file too); the few commands without an
// action give their own run.

type menuItem struct {
	label  string
	action string      // the action it runs, and whose keys it shows
	run    func()      // or this, when there is no action
	tick   func() bool // a tick beside it while true
}

// sep is the line between groups of items.
var menuSep = menuItem{}

type menu struct {
	title string
	items []menuItem
}

var menuBar []menu

func init() {
	tool := func(label, id string) menuItem {
		return menuItem{label: label, action: "tool_" + id, tick: func() bool { return a.tool == id }}
	}
	machine := func(label, m string) menuItem {
		return menuItem{label: label, run: func() { a.start = false; a.setMode(m) },
			tick: func() bool { return a.mode == m && !a.start }}
	}
	arrange := func(label, l string) menuItem {
		return menuItem{label: label, run: func() { a.setUILayout(l) }, tick: func() bool { return a.studio() == (l == "studio") }}
	}
	look := func(name string) menuItem {
		return menuItem{label: name + " theme", run: func() { setTheme(name) }, tick: func() bool { return theme.Name == name }}
	}
	layout := func(label, l string) menuItem {
		return menuItem{label: label, run: func() { a.setLayout(l) }, tick: func() bool { return a.layout == l }}
	}
	menuBar = []menu{
		{"File", []menuItem{
			{label: "New page", action: "new_page"},
			{label: "Start screen", action: "start_screen"},
			{label: "Open...", action: "open"},
			{label: "Open the last file again", action: "open_recent"},
			menuSep,
			{label: "Save", action: "save"},
			{label: "Save as...", action: "save_as"},
			{label: "Info and SAUCE...", action: "info"},
			{label: "Check for a party...", action: "party_check"},
			{label: "Export work stages...", action: "work_stages"},
			menuSep,
			{label: "Join a drawing session...", action: "collab_join"},
			{label: "Host a drawing session...", action: "collab_host"},
			{label: "End the drawing session", action: "collab_end"},
			menuSep,
			{label: "Load a font...", action: "font_load"},
			{label: "Font editor...", action: "font_editor"},
			menuSep,
			{label: "Settings...", run: openSettings},
			menuSep,
			{label: "Close the page", action: "close_page"},
			{label: "Quit", action: "quit"},
		}},
		{"Edit", []menuItem{
			{label: "Undo", action: "undo"},
			{label: "Redo", action: "redo"},
			menuSep,
			{label: "Cut", action: "cut"},
			{label: "Copy", action: "copy"},
			{label: "Copy what is seen", action: "copy_seen"},
			{label: "Paste", action: "paste"},
			{label: "Select all", action: "select_all"},
			menuSep,
			{label: "Fill the selection", action: "fill_block"},
			{label: "Crop to the selection", action: "crop"},
			{label: "Mirror left-right", action: "mirror"},
			{label: "Turn upside down", action: "upside_down"},
			{label: "Turn a quarter", action: "turn"},
			{label: "Invert", action: "invert"},
			menuSep,
			{label: "Insert a row", action: "insert_row"},
			{label: "Delete the row", action: "delete_row"},
			{label: "Insert a column", action: "insert_column"},
			{label: "Delete the column", action: "delete_column"},
		}},
		{"View", []menuItem{
			{label: "Zoom in", action: "zoom_in"},
			{label: "Zoom out", action: "zoom_out"},
			{label: "Fit the width", action: "fit_width"},
			menuSep,
			{label: "Grid", action: "grid", tick: func() bool { return a.grid }},
			{label: "Frame strip", action: "frames_strip", tick: func() bool { return a.stripShown() }},
			{label: "Hide the gadgets", action: "hide_gadgets", tick: func() bool { return a.bare }},
			menuSep,
			layout("One page", "one"),
			layout("Side by side", "cols"),
			layout("Top and bottom", "rows"),
			arrange("Studio layout: tools left, a side panel", "studio"),
			arrange("Focus layout: the page wide, gadgets under it", "focus"),
			menuSep,
			look("Dark"),
			look("Light"),
			look("Workbench"),
			{label: "Next page", action: "next_page"},
			{label: "Previous page", action: "previous_page"},
			menuSep,
			{label: "Reference picture...", action: "ref_open"},
			{label: "Show the reference", action: "ref_show", tick: func() bool { return a.ref != nil && !a.ref.hidden }},
			{label: "Move the reference", action: "ref_move"},
			{label: "Reference fainter", action: "ref_fainter"},
			{label: "Reference stronger", action: "ref_stronger"},
			{label: "Remove the reference", action: "ref_remove"},
		}},
		{"Page", []menuItem{
			machine("Amiga", "amiga"),
			machine("PC", "pc"),
			machine("Atari 8-bit", "atari"),
			machine("Atari ST", "st"),
			machine("PETSCII (Commodore)", "c64"),
			machine("ZX Spectrum", "zx"),
			machine("Teletext", "tt"),
			machine("Amstrad CPC", "cpc"),
			machine("Minitel", "mt"),
			machine("Viewdata", "vd"),
			machine("MSX", "msx"),
			machine("Sharp MZ-700", "mz"),
			machine("Unicode (terminals, IRC)", "uni"),
			{label: "Convert to Unicode", action: "convert_unicode"},
			menuSep,
			{label: "iCE colours", action: "ice", tick: func() bool { return a.ice }},
			{label: "9-pixel letter spacing", action: "ninepx", tick: func() bool { return a.pc() && a.ninePx() }},
			{label: "Reverse characters", action: "reverse", tick: func() bool { return a.rev }},
			menuSep,
			{label: "Default colours", action: "default_colours"},
			{label: "Swap the colours", action: "swap_colours"},
			{label: "Take the colours under the cursor", action: "take_colours"},
			{label: "A 24-bit colour (Unicode)...", action: "uni_colour"},
			menuSep,
			{label: "New layer", action: "layer_new"},
			{label: "Copy the layer", action: "layer_copy"},
			{label: "Delete the layer", action: "layer_delete"},
			{label: "Layer above", action: "layer_above"},
			{label: "Layer below", action: "layer_below"},
			{label: "Move the layer up", action: "layer_raise"},
			{label: "Move the layer down", action: "layer_lower"},
			{label: "Merge down", action: "layer_merge_down"},
			{label: "Merge all layers", action: "layer_merge_all"},
			{label: "Show the layer", action: "layer_show", tick: func() bool { return a.layers == nil || !a.layers[a.lay].hidden }},
			{label: "Black shows below", action: "layer_black", tick: func() bool { return a.layers != nil && a.layers[a.lay].clear == layerBlack }},
			{label: "Other layers dim / hidden", action: "layer_others", tick: func() bool { return a.others != 0 }},
		}},
		{"Draw", []menuItem{
			tool("Type", "type"),
			tool("Draw", "draw"),
			tool("Colour", "colour"),
			tool("Fill", "fill"),
			tool("Pick", "pick"),
			tool("Select", "select"),
			tool("Half blocks", "half"),
			tool("Logo (TheDraw fonts)", "logo"),
			{label: "TheDraw fonts...", action: "logo_font"},
			{label: "Type a line in the font...", action: "logo_line"},
			menuSep,
			{label: "Line", action: "shape_line"},
			{label: "Box", action: "shape_box"},
			{label: "Ellipse", action: "shape_ellipse"},
			{label: "Filled shapes", action: "shape_filled", tick: func() bool { return a.shapeFilled }},
			{label: "Next shape style", action: "shape_style"},
			menuSep,
			{label: "Shade brush", action: "shade_brush", tick: func() bool { return a.shade }},
			{label: "Mirror left-right", action: "mirror_left_right", tick: func() bool { return a.mirror&mirrorX != 0 }},
			{label: "Mirror top-bottom", action: "mirror_top_bottom", tick: func() bool { return a.mirror&mirrorY != 0 }},
			{label: "Paint colours only", action: "paint_colour_only", tick: func() bool { return a.paintOnly == "colour" }},
			{label: "Paint characters only", action: "paint_char_only", tick: func() bool { return a.paintOnly == "char" }},
		}},
		{"Animate", []menuItem{
			{label: "Templates...", action: "animate"},
			{label: "Animate a folder...", action: "animate_folder"},
			menuSep,
			{label: "Play", action: "play", tick: func() bool { return a.playing != nil }},
			menuSep,
			{label: "New frame", action: "frame_new"},
			{label: "Copy the frame", action: "frame_duplicate"},
			{label: "Delete the frame", action: "frame_delete"},
			{label: "Move the frame earlier", action: "frame_earlier"},
			{label: "Move the frame later", action: "frame_later"},
			menuSep,
			{label: "Next frame", action: "frame_next"},
			{label: "Previous frame", action: "frame_previous"},
			{label: "Onion skin", action: "onion", tick: func() bool { return a.onion }},
		}},
		{"Help", []menuItem{
			{label: "Keys and mouse", action: "help"},
		}},
	}
	for _, m := range menuBar { // a typo in an action id shows at the start, not as a dead item
		for _, it := range m.items {
			if it.action != "" && actionByID[it.action] == nil {
				panic("menus.go: no action " + it.action)
			}
		}
	}
}

// do runs the item.
func (it menuItem) do() {
	if it.action != "" {
		actionByID[it.action].run()
	} else if it.run != nil {
		it.run()
	}
}

// keys is what the item shows on the right: its action's first key.
func (it menuItem) keys() string {
	if it.action == "" || len(keysNow[it.action]) == 0 {
		return ""
	}
	return keysNow[it.action][0].String()
}

// drawMenuBar draws the menu titles along the top and returns where the rest starts.
func (a *App) drawMenuBar(u *UI) int {
	fb := u.fb
	h := lineH + 6
	bar := Rect{0, 0, fb.w, h}
	u.menuBar = bar
	fb.fill(bar, cDark)
	x := 4
	for i, m := range menuBar {
		r := Rect{x, 0, textW(m.title) + 20, h}
		x += r.w
		id := "menu:" + m.title
		open := u.popup != nil && u.popup.id == id
		col := cText
		if open {
			fb.fill(r, cBlue)
			col = cHi
		} else if u.hover(r) {
			col = cHi
		}
		fb.text(r.x+10, r.y+3, m.title, col)
		// While a menu is open, the mouse moving onto another title opens that one
		// (resting there, it leaves a menu the arrow keys chose).
		if p := u.popup; !open && p != nil && p.menu && r.has(u.mx, u.my) && p.mouse != [2]int{u.mx, u.my} {
			a.openMenu(u, i)
		}
		if u.take(r) != nil && u.closedMenu != id {
			a.openMenu(u, i)
		}
	}
	u.closedMenu = ""
	fb.fill(Rect{0, h - 1, fb.w, 1}, cEdgeDk)
	return h
}

// openMenu opens menu i under its title.
func (a *App) openMenu(u *UI, i int) {
	i = (i + len(menuBar)) % len(menuBar)
	m := menuBar[i]
	x := 4
	for _, o := range menuBar[:i] {
		x += textW(o.title) + 20
	}
	p := &Popup{id: "menu:" + m.title, menu: true, hot: -1, pick: func(j int) { m.items[j].do() },
		mouse: [2]int{u.mx, u.my}} // only a move of the mouse from here switches menus
	w := 0
	for _, it := range m.items {
		p.items = append(p.items, it.label)
		p.keys = append(p.keys, it.keys())
		p.ticks = append(p.ticks, it.tick != nil && it.tick())
		w = max(w, textW(it.label)+textW(it.keys()))
	}
	w += 30 + 24 + 16 // the tick's room, the gap before the keys, the edges
	p.r = Rect{x, u.menuBar.h, max(w, 180), 0}
	p.r.h = p.height()
	if p.r.x+p.r.w > u.fb.w {
		p.r.x = max(0, u.fb.w-p.r.w)
	}
	p.nav = func(d int) { a.openMenu(u, i+d) }
	u.popup = p
}
