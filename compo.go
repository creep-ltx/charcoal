package main

// Check for a party: a compo's published rules as data, and what the page
// breaks of the ones CharEdit can see (size, colours, font, animation, page
// type). Quiet: a short report, nothing changed on the page. The same
// requester exports the work stages (stages.go) and can write the files the
// compo wants into a folder ("Save for the party").
//
// Rules read on 2026-10-16 from each party's own page (the URL in the table);
// the quotes are theirs. Rules CharEdit cannot check (no AI, no conversions)
// are listed as a reminder.

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// compoPlat is one kind of entry a compo takes (PC ANSI, PETSCII ...).
type compoPlat struct {
	what      string   // as the compo names it
	modes     string   // the page types it can be ("pc amiga", "c64" ...)
	machine   string   // the machine a PETSCII entry runs on ("c64"), "" any
	cols      int      // the most columns (0: any)
	rows      int      // the most rows (0: any)
	exactCols int      // the width it must have (0: any up to cols)
	cells     int      // the most cells, columns x rows of the drawing (0: any)
	ice       bool     // iCE colours (bright backgrounds) allowed
	blink     bool     // blinking allowed
	ownFont   bool     // a font of its own allowed
	ownPal    bool     // a palette of its own allowed
	anim      bool     // frames or a draw order (an ANSImation) allowed
	vicModes  bool     // C64 multicolour and ECM allowed
	freeRows  []int    // rows that must stay empty (teletext's header and fastext rows)
	deliver   []string // the files to hand in, as format ids ("png" the screenshot)
}

// compoRule is one compo's rules.
type compoRule struct {
	id, name string
	edition  string // the year and page the rules were read from
	url      string
	plats    []compoPlat
	stages   int  // work stages asked for (0: none)
	atLeast  bool // stages is a minimum, not an exact number
	also     string
}

var compoRules = []compoRule{
	{id: "revision-text", name: "Revision: ASCII / ANSI / PETSCII", edition: "Revision 2026",
		url: "https://2026.revision-party.net/competitions/graphics/",
		// "Maximum size 80 x 500." "PC ANSI : No ICE Colors, 16 color ANSI only. Don't use blink mode."
		// "Default Commodore ROM font only, no scrolling pictures, no sprites." "Submit your PETSCII as a
		// c64 self-containing .prg or .d64." "Animations are not allowed" "You do NOT need to submit any
		// workstages." "Adding a screenshot is absolutely mandatory"
		plats: []compoPlat{
			{what: "PC ASCII / ANSI", modes: "pc", cols: 80, rows: 500, deliver: []string{"ansi", "png"}},
			{what: "Amiga ASCII", modes: "amiga", cols: 80, rows: 500, deliver: []string{"ansi", "png"}},
			{what: "PETSCII", modes: "c64", machine: "c64", cols: 40, rows: 25, vicModes: true, deliver: []string{"prg", "png"}},
		},
		also: "a screenshot is mandatory; tell them the font (SAUCE is not read); no AI, no conversions"},
	{id: "revision-oldskool", name: "Revision: Oldskool graphics", edition: "Revision 2026",
		url: "https://2026.revision-party.net/competitions/oldskool/",
		// "Include exactly 10 (ten) working stages of your entry." "the entry has to be delivered as both an
		// executable program and a PNG image." "No multi-screen or scrolling pictures"
		plats: []compoPlat{
			{what: "C64", modes: "c64", machine: "c64", cols: 40, rows: 25, ownFont: true, vicModes: true, deliver: []string{"prg", "png"}},
			{what: "Atari XL", modes: "atari", cols: 40, rows: 24, ownFont: true, deliver: []string{"xex", "png"}},
			{what: "ZX Spectrum", modes: "zx", cols: 32, rows: 24, ownFont: true, deliver: []string{"tap", "png"}},
			{what: "Amstrad CPC", modes: "cpc", rows: 25, deliver: []string{"cpcbin", "png"}},
		},
		stages: 10, also: "say which mode you use; stages apart from the final picture (a subfolder); no AI"},
	{id: "evoke", name: "Evoke: One screen ANSI / ASCII", edition: "Evoke 2026",
		url: "https://2026.evoke.eu/competitions/graphics/",
		// "Maximum width: 80 characters / Maximum length: 25 lines." "iCE colours are not allowed."
		// "Custom palettes are not allowed." "Ansimations are not allowed."
		plats: []compoPlat{
			{what: "PC ANSI / ASCII", modes: "pc", cols: 80, rows: 25, blink: true, ownFont: true, deliver: []string{"ansi", "png"}},
			{what: "Amiga ANSI / ASCII", modes: "amiga", cols: 80, rows: 25, blink: true, ownFont: true, deliver: []string{"ansi", "png"}},
		},
		also: "shown in PabloDraw (Amiga: Topaz 2+ unless you say); no REZ2ANS or other converters; no AI"},
	{id: "lovebyte", name: "Lovebyte: Text graphics, 256 characters", edition: "Lovebyte 2025 (no 2026 rules yet)",
		url: "https://lovebyte.party/",
		// "Text Graphics can be any dimension, and must be 256 characters or fewer."
		// "as long as the display is 256 characters or fewer (e.g. 16x16, 8x32, 17x14, etc...)"
		plats: []compoPlat{
			{what: "ANSI / ASCII", modes: "pc amiga", cells: 256, ice: true, blink: true, ownFont: true, ownPal: true, deliver: []string{"ansi", "png"}},
			{what: "PETSCII", modes: "c64", cells: 256, ownFont: true, vicModes: true, deliver: []string{"prg", "png"}},
			{what: "ATASCII", modes: "atari", cells: 256, ownFont: true, deliver: []string{"xex", "png"}},
			{what: "Teletext", modes: "tt", cells: 256, deliver: []string{"tti", "png"}},
		},
		also: "the drawing's columns x rows count; a screenshot, a 1280x720 MP4 and a FILE_ID.DIZ"},
	{id: "zoo", name: "Zoo: PETSCII graphics", edition: "Zoo 2026",
		url: "https://2026.zooparty.org/compos/",
		// "drawn with the original PETSCII character set using standard character mode. Maximum size is
		// 40x25 characters." "rasters or animation is a no-no." "Entry shall be provided as an executable."
		// "We require minimum of fifteen (15) workstages"
		plats: []compoPlat{
			{what: "PETSCII", modes: "c64", machine: "c64", cols: 40, rows: 25, deliver: []string{"prg", "png"}},
		},
		stages: 15, atLeast: true, also: "stages from the blank screen to the final one; no conversion at any stage; no AI"},
	{id: "deadline", name: "Deadline: Textmode graphics", edition: "Deadline 2026",
		url: "https://www.demoparty.berlin/competitions/",
		// "ASCII/ANSI: Maximum width is 80 characters, maximum height is 2048 lines." "PETSCII/ATASCII:
		// Maximum width is 40 characters, maximum height is 25 lines. ... ECM and MCM are allowed. Entries
		// must be provided as binaries" "No XBIN, custom fonts, custom palettes, blinking or Ansimations.
		// iCE Colors are supported."
		plats: []compoPlat{
			{what: "PC ASCII / ANSI", modes: "pc", cols: 80, rows: 2048, ice: true, deliver: []string{"ansi", "png"}},
			{what: "Amiga ASCII / ANSI", modes: "amiga", cols: 80, rows: 2048, ice: true, deliver: []string{"ansi", "png"}},
			{what: "PETSCII", modes: "c64", cols: 40, rows: 25, vicModes: true, deliver: []string{"prg", "png"}},
			{what: "ATASCII", modes: "atari", cols: 40, rows: 25, deliver: []string{"xex", "png"}},
		},
		also: "say PC or Amiga for ASCII (or a SAUCE record); no remote entries; no AI"},
	{id: "x", name: "X: PETSCII", edition: "X 2026",
		url: "https://xparty.net/compos",
		// "use the lower OR the uppercase ROM charset" "Use any fixed background color and use a fixed
		// colormap ($d800)" "Animations, interlace, sprites and effects are not allowed" ".prg only"
		plats: []compoPlat{
			{what: "PETSCII", modes: "c64", machine: "c64", cols: 40, rows: 25, deliver: []string{"prg", "png"}},
		},
		also: "explain your process and add work stages in a .zip (asked, no number); no AI"},
	{id: "amscii", name: "Logiker: AMSCII (Amstrad CPC)", edition: "4th AMSCII compo, 2024 (none planned)",
		url: "https://logiker.com/AMSCII-Compo-Rules",
		// "One character per cell with background and foreground colour" "Do not use enhanced techniques
		// (tricks) to add more colours, pixels, animations" "Exported .bin file obligatory / .dsk file if
		// possible / Screenshot/Preview as .png file"
		plats: []compoPlat{
			{what: "AMSCII", modes: "cpc", rows: 25, deliver: []string{"cpcbin", "cpcdsk", "png", "psci"}},
		},
		also: "mode 1 is the main category; .bin names of at most 8 characters; steps welcome; no conversions"},
	{id: "atascii", name: "Logiker: ATASCII (Atari 8-bit)", edition: "6th ATASCII compo, 2026",
		url: "https://logiker.com/ATASCII-Rules",
		// "Resolution in characters: width 40, height 24-28" "One colour for the background and one for the
		// foreground" "Exported .xex file obligatory" "Screenshot/preview as .png file (Use Altirra ...)"
		plats: []compoPlat{
			{what: "Pure ATASCII", modes: "atari", cols: 40, exactCols: 40, rows: 28, deliver: []string{"xex", "png"}},
		},
		also: "an altered font or other tricks go to the Wild category; steps welcome; no conversions"},
	{id: "teletext-advent", name: "Teletext Advent Calendar (Yle)", edition: "the 2026 call",
		url: "https://teletextart.co.uk/call-for-entries-teletext-advent-calendar-2026-on-yle-text",
		// "the page header (row 0) and fastext labels (row 24) are overwritten, so try not to use them"
		// "Level 1/1.5 Teletext only" "Web editor links (edit-tf and ZXNet) / Teletext files (TTI, EP1 etc.)"
		plats: []compoPlat{
			{what: "Teletext", modes: "tt", cols: 40, rows: 25, freeRows: []int{0, 24}, deliver: []string{"tti", "tturl"}},
		},
		also: "anything seasonal; no copyrighted characters"},
}

func compoByID(id string) *compoRule {
	for i := range compoRules {
		if compoRules[i].id == id {
			return &compoRules[i]
		}
	}
	return nil
}

// stagesText says what a compo wants of work stages.
func (r *compoRule) stagesText() string {
	switch {
	case r.stages == 0:
		return "no work stages asked for"
	case r.atLeast:
		return fmt.Sprintf("at least %d work stages", r.stages)
	}
	return fmt.Sprintf("exactly %d work stages", r.stages)
}

// compoPlatFor is the kind of entry the page can be, nil if none; why says
// why not.
func (a *App) compoPlatFor(r *compoRule) (p *compoPlat, why string) {
	var kinds []string
	for i := range r.plats {
		pl := &r.plats[i]
		kinds = append(kinds, pl.what)
		if !strings.Contains(" "+pl.modes+" ", " "+a.mode+" ") {
			continue
		}
		if pl.machine != "" && a.mach().id != pl.machine {
			return nil, fmt.Sprintf("%s entries run on a %s; this page is a %s", pl.what, machineByID(pl.machine).name, a.mach().name)
		}
		return pl, ""
	}
	name := modeNames[a.mode]
	return nil, fmt.Sprintf("This compo takes %s; this is a%s %s page", strings.Join(kinds, ", "), map[bool]string{true: "n"}[name != "" && strings.ContainsAny(name[:1], "AEIOU")], name)
}

// drawnSize is the size of what is drawn: the page on Amiga and PC pages (a
// free width: the page is the drawing), else the box around the cells that show.
func (a *App) drawnSize(d *Doc) (cols, rows int) {
	if a.pc() || a.amiga() {
		return d.cols, d.rows
	}
	e := blank
	if a.c64() || a.atari() || a.tt() {
		e = 32
	}
	x0, y0, x1, y1 := d.cols, d.rows, -1, -1
	for y := 0; y < d.rows; y++ {
		for x := 0; x < d.cols; x++ {
			if usedCell(a.mode, d.get(x, y), e) {
				x0, y0, x1, y1 = min(x0, x), min(y0, y), max(x1, x), max(y1, y)
			}
		}
	}
	if x1 < 0 {
		return 0, 0
	}
	return x1 - x0 + 1, y1 - y0 + 1
}

// compoCheck lists what the page breaks of r's rules (empty: nothing CharEdit can see).
func (a *App) compoCheck(r *compoRule) []string {
	p, why := a.compoPlatFor(r)
	if p == nil {
		return []string{why}
	}
	var out []string
	add := func(f string, v ...any) { out = append(out, fmt.Sprintf(f, v...)) }
	d := a.seen() // the layers put together (layers.go)
	if a.playing != nil {
		d = a.playing.saved.doc
	}
	if p.exactCols > 0 && d.cols != p.exactCols {
		add("The page is %d columns wide; %s is %d", d.cols, p.what, p.exactCols)
	} else if p.cols > 0 && d.cols > p.cols {
		add("The page is %d columns wide; the most is %d", d.cols, p.cols)
	}
	if p.rows > 0 && d.rows > p.rows {
		s := fmt.Sprintf("The page is %d rows tall; the most is %d", d.rows, p.rows)
		if u := d.usedRows(); u <= p.rows {
			s += fmt.Sprintf(" (rows %d-%d are empty: delete them)", u+1, d.rows)
		}
		add("%s", s)
	}
	if p.cells > 0 {
		if c, rw := a.drawnSize(d); c*rw > p.cells {
			add("The drawing is %dx%d = %d characters; the most is %d", c, rw, c*rw, p.cells)
		}
	}
	if a.pc() || a.amiga() {
		bright := 0
		for _, v := range d.cells[:d.cols*d.rows] {
			if cellBg(v) >= 8 {
				bright++
			}
		}
		switch {
		case bright > 0 && a.ice && !p.ice:
			add("%d cells have iCE colours (bright backgrounds): not allowed", bright)
		case bright > 0 && !a.ice && !p.blink:
			add("%d cells blink: not allowed", bright)
		}
	}
	if f := a.ownInUse(); f != nil && !p.ownFont {
		add("The page shows its own font (%s): only the standard font is allowed", f.name)
	}
	if !p.ownPal && (a.pc() || a.amiga()) && (a.paletteFor[a.mode] == "file" || a.paletteFor[a.mode] == "custom") {
		add("The page shows its own palette: not allowed (an .ans cannot carry it anyway)")
	}
	if !p.anim {
		if n := a.nFrames(); n > 1 {
			add("The page has %d frames: no animation (only the frame shown is the entry)", n)
		} else if a.order != nil && a.orderedSave(a.saveFmt) {
			add("Saving draws it in an order (Animate), an animation: take the order off")
		}
	}
	if !p.vicModes && a.vicII() && a.vic.Mode != 0 {
		add("The page is in %s: standard character mode only", map[int]string{1: "multicolour", 2: "ECM"}[a.vic.Mode])
	}
	for _, y := range p.freeRows {
		if y >= d.rows {
			continue
		}
		for x := 0; x < d.cols; x++ {
			if usedCell(a.mode, d.get(x, y), 32) {
				add("Row %d has art: it is overwritten when broadcast", y)
				break
			}
		}
	}
	return out
}

// handIn says the files the compo wants, as their endings.
func handIn(p *compoPlat) string {
	var s []string
	for _, f := range p.deliver {
		if f == "png" {
			s = append(s, "a PNG screenshot")
			continue
		}
		s = append(s, strings.TrimPrefix(formatExt(f), "."))
	}
	return strings.Join(s, ", ")
}

// ---------------------------------------------------------------- the requester

// PartyReq: pick a compo, see what the page breaks.
type PartyReq struct{ rule int }

func openPartyCheck() {
	a.endPlay()
	d := &PartyReq{}
	for i, r := range compoRules {
		if r.id == prefs.Party {
			d.rule = i
		}
	}
	a.dialog = d
}

func (d *PartyReq) draw(u *UI) {
	fb := u.fb
	r := &compoRules[d.rule]
	in := u.window(centre(620, 300), "Check for a party")
	var names []string
	for _, c := range compoRules {
		names = append(names, c.name)
	}
	y := in.y
	u.choice("party", Rect{in.x, y, min(in.w, choiceW(names)), gadH}, names, d.rule,
		"The compo whose rules the page is checked against", func(i int) {
			d.rule = i
			prefs.Party = compoRules[i].id
			savePrefs()
		})
	y += gadH + 4
	fb.text(in.x, y, r.edition+": "+r.url, cDim)
	y += lineH + 8
	line := func(s string, c uint32) {
		for _, l := range wrap(s, in.w-16) {
			fb.text(in.x+16, y, l, c)
			y += lineH
		}
	}
	probs := a.compoCheck(r)
	if len(probs) == 0 {
		fb.text(in.x, y, "✓", cText)
		line("Nothing CharEdit can check breaks the rules.", cText)
	}
	for _, s := range probs {
		fb.text(in.x, y, "•", cOrange)
		line(s, cText)
	}
	y += 6
	if p, _ := a.compoPlatFor(r); p != nil {
		line("Hand in: "+handIn(p), cDim)
	}
	line("Work stages: "+r.stagesText(), cDim)
	line("Also: "+r.also, cDim)
	switch buttonRow(u, in, "Work stages...", "Save for the party", "Close") {
	case 0:
		openStagesReq(r)
	case 1:
		a.saveForParty(r)
	case 2:
		closeDialog()
	}
}

func (d *PartyReq) key(k string, ctrl, shift bool) bool {
	switch k {
	case "Escape", "Return", "Keypad Enter":
		closeDialog()
	case "Up", "Down":
		d.rule = (d.rule + map[string]int{"Up": len(compoRules) - 1, "Down": 1}[k]) % len(compoRules)
		prefs.Party = compoRules[d.rule].id
		savePrefs()
	default:
		return false
	}
	return true
}

// ---------------------------------------------------------------- export work stages

// StagesReq: how many stages, how big, where.
type StagesReq struct {
	n, dir Field
	zoom   int // 0 1x, 1 2x
	native bool
}

// stagesBase is the page's name without its ending ("untitled" if none).
func (a *App) stagesBase() string {
	b := strings.TrimSuffix(a.name, filepath.Ext(a.name))
	if b == "" {
		b = "untitled"
	}
	return b
}

// stagesDirFor is the folder stages go to by default: base-stages beside the file.
func (a *App) stagesDirFor() string {
	dir := a.lastDir
	if a.path != "" {
		dir = filepath.Dir(a.path)
	}
	if dir == "" {
		dir, _ = os.UserHomeDir()
	}
	return filepath.Join(dir, a.stagesBase()+"-stages")
}

func openStagesReq(r *compoRule) {
	a.endPlay()
	d := &StagesReq{n: Field{max: 2}, dir: Field{max: 400}}
	n := 10
	if r == nil {
		r = compoByID(prefs.Party)
	}
	if r != nil && r.stages > 0 {
		n = r.stages
	}
	d.n.set(strconv.Itoa(n))
	d.dir.set(shortPath(a.stagesDirFor()))
	d.n.enter, d.dir.enter = d.ok, d.ok
	a.dialog = d
	ui.focus = &d.n
}

func (d *StagesReq) ok() {
	n, err := strconv.Atoi(strings.TrimSpace(d.n.String()))
	if err != nil || n < 1 || n > 99 {
		a.msg("Work stages: a number from 1 to 99")
		return
	}
	dir := expandHome(strings.TrimSpace(d.dir.String()))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		a.msg("Could not make the folder: " + err.Error())
		return
	}
	native := ""
	if d.native {
		native = a.saveFmt
	}
	closeDialog()
	files, err := a.exportStages(dir, a.stagesBase(), n, d.zoom+1, native)
	if err != nil {
		a.msg("Work stages: " + err.Error())
		return
	}
	a.msg(stagesNote(files, n, native != "", dir))
}

// stagesNote is the status line after an export.
func stagesNote(files []string, n int, native bool, dir string) string {
	got := len(files)
	if native {
		got /= 2
	}
	if got < n {
		return fmt.Sprintf("Only %d different stages in the page's history: %d written to %s", got, got, shownDir(dir))
	}
	return fmt.Sprintf("%d work stages written to %s", got, shownDir(dir))
}

func (d *StagesReq) draw(u *UI) {
	fb := u.fb
	in := u.window(centre(560, 230), "Export work stages")
	y := in.y
	col := colAfter(90, "Stages", "Folder")
	fb.text(in.x, y+3, "Stages", cText)
	u.field(Rect{in.x + col, y, digitsW(4) + 8, gadH}, &d.n, "How many pictures, spread evenly from the start of the work to now")
	y += gadH + 6
	fb.text(in.x, y+3, "Folder", cText)
	u.field(Rect{in.x + col, y, in.w - col, gadH}, &d.dir, "Made if missing; files are name-stage01.png ...")
	y += gadH + 6
	sizes := []string{"PNG", "PNG, twice the size"}
	u.choice("stagezoom", Rect{in.x + col, y, choiceW(sizes), gadH}, sizes, d.zoom, "", func(i int) { d.zoom = i })
	y += gadH + 6
	if a.saveFmt != "" && a.saveFmt != "png" && a.saveFmt != "png2" {
		d.native, _ = u.check(in.x+col, y, "Also as "+strings.TrimPrefix(formatExt(a.saveFmt), ".")+" files", d.native, "Each stage in the page's own format too")
		y += gadH + 6
	}
	hist := "History: nothing drawn here yet (stages come from the work done in CharEdit)"
	if a.steps > 0 {
		hist = fmt.Sprintf("History: %d steps, %d different pictures to take stages from", a.steps, len(a.stageCands())-1)
	}
	fb.text(in.x, y+4, hist, cDim)
	switch buttonRow(u, in, "Export", "Cancel") {
	case 0:
		d.ok()
	case 1:
		closeDialog()
	}
}

func (d *StagesReq) key(k string, ctrl, shift bool) bool {
	switch k {
	case "Tab":
		if ui.focus == &d.n {
			ui.focus = &d.dir
		} else {
			ui.focus = &d.n
		}
	case "Escape":
		closeDialog()
	case "Return", "Keypad Enter":
		d.ok()
	default:
		return false
	}
	return true
}

// ---------------------------------------------------------------- save for the party

// saveForParty writes the files r wants for this page into a folder beside the
// page's file: name-<compo>/, the stages (if asked for) in its "workstages".
func (a *App) saveForParty(r *compoRule) {
	p, why := a.compoPlatFor(r)
	if p == nil {
		a.msg(why)
		return
	}
	if a.path == "" {
		a.msg("Save the page first: the party's files go into a folder beside it")
		return
	}
	closeDialog()
	a.endPlay()
	base := a.stagesBase()
	dir := filepath.Join(filepath.Dir(a.path), base+"-"+r.id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		a.msg("Could not make the folder: " + err.Error())
		return
	}
	name := base
	if p.modes == "cpc" && len(name) > 8 { // AMSDOS names: 8 characters
		name = name[:8]
	}
	var files []string
	keepMsg, keepAt := a.message, a.messageAt
	for _, f := range p.deliver {
		path := filepath.Join(dir, name+formatExt(f))
		cpcTarget = filepath.Join(dir, name+".bin")
		out, err := a.encodeStage(a.seen(), f)
		cpcTarget = ""
		if err == nil {
			err = writeAtomic(path, out)
		}
		if err != nil {
			a.msg("Save for the party: " + err.Error())
			return
		}
		files = append(files, filepath.Base(path))
	}
	a.message, a.messageAt = keepMsg, keepAt
	note := ""
	if r.stages > 0 {
		sd := filepath.Join(dir, "workstages")
		if err := os.MkdirAll(sd, 0o755); err == nil {
			st, err := a.exportStages(sd, base, r.stages, 1, "")
			switch {
			case err != nil:
				note = "; work stages: " + err.Error()
			case len(st) < r.stages:
				note = fmt.Sprintf("; only %d work stages in the history (%s asks for %s)", len(st), r.name, r.stagesText())
			default:
				note = fmt.Sprintf(" + %d work stages", len(st))
			}
		}
	}
	msg := "Written to " + shownDir(dir) + ": " + strings.Join(files, ", ") + note
	if probs := a.compoCheck(r); len(probs) > 0 {
		msg += fmt.Sprintf(" (%d rules broken: see Check for a party)", len(probs))
	}
	a.msg(msg)
}
