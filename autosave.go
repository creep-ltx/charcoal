package main

// Safety: pages with changes are saved aside every two minutes (autosave), and
// offered back at the next start if CharEdit stopped without saving them; the
// file a save writes over is kept as name.bak; the last files opened or saved
// are remembered (Recent in the Open requester, Ctrl+Shift+O).

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const autosaveEvery = 2 * time.Minute

var lastAutosave = time.Now()

func autosaveDir() string { return filepath.Join(appDataDir(), "autosave") }

// autosaveMeta is what an autosave says about its page, next to the cells.
type autosaveMeta struct {
	PID          int             `json:"pid"` // the CharEdit that wrote it
	Saved        time.Time       `json:"saved"`
	Path         string          `json:"path"` // the page's file, "" if it never had one
	Name         string          `json:"name"`
	Mode         string          `json:"mode"`
	SaveFmt      string          `json:"save_format"`
	Machine      string          `json:"machine,omitempty"`
	STRes        string          `json:"st_resolution,omitempty"`
	Font         string          `json:"font,omitempty"`
	Palette      string          `json:"palette,omitempty"`
	Ice          bool            `json:"ice,omitempty"`
	Cols         int             `json:"cols"`
	Rows         int             `json:"rows"`
	Bg           int             `json:"bg"`
	Border       int             `json:"border"`
	Ink          int             `json:"ink"`
	Glyph0       bool            `json:"glyph0,omitempty"`
	GTIA         bool            `json:"gtia,omitempty"` // an Atari page's colours are register values (gtia.go)
	Title        string          `json:"title,omitempty"`
	Author       string          `json:"author,omitempty"`
	Group        string          `json:"group,omitempty"`
	WriteSauce   bool            `json:"write_sauce"`
	Clear        bool            `json:"clear,omitempty"`
	Notes        []string        `json:"notes,omitempty"`
	Subpage      int             `json:"tti_subpage,omitempty"` // a .tti carousel's subpage, from 1 (saving keeps the others)
	Screen       int             `json:"screen,omitempty"`      // 0.1b3: a .c / .petmate file's screen, from 1 (saving keeps the others)
	SrcKind      string          `json:"source_kind,omitempty"` // the kind of the page's file ("tti", "marq", "petmate" ...), read again from Path
	Cur          int             `json:"frame,omitempty"`       // the frame being worked on
	Frames       []autosaveFrame `json:"frames,omitempty"`      // a page of several frames: their cells follow one another in .cells
	Restructured bool            `json:"restructured,omitempty"`
	Ref          *RefPref        `json:"reference,omitempty"`  // the page's reference picture
	OwnFont      *ArtFont        `json:"own_font,omitempty"`   // the page's own font, in id.font (fontfile.go)
	Order        *drawOrder      `json:"draw_order,omitempty"` // the order Save draws a stream file in
	UniPairs     [][3]int        `json:"uni_pairs,omitempty"`  // a Unicode page's styles 256 and up: fg, bg, italic|underline (uni.go)
	CPC          *cpcAutosave    `json:"cpc,omitempty"`        // an Amstrad CPC page's mode, inks and border
	MSX          *msxAutosave    `json:"msx,omitempty"`        // an MSX page's screen mode and colours (msx.go)
	VIC          *vicSetup       `json:"vic,omitempty"`        // a C64 page's text mode and $D022-$D024 (c64x.go)
}

// autosaveFrame is one frame of an autosaved page.
type autosaveFrame struct {
	Cols   int    `json:"cols"`
	Rows   int    `json:"rows"`
	Bg     int    `json:"bg"`
	Border int    `json:"border"`
	Ink    int    `json:"ink"`
	Glyph0 bool   `json:"glyph0,omitempty"`
	Hold   int    `json:"hold,omitempty"`
	Name   string `json:"name,omitempty"`
	Lower  int    `json:"lower"`
	Seg    []byte `json:"tti_subpage_text,omitempty"` // base64
	Src    int    `json:"source_screen"`
}

// An autosave on disk: id.json and id.cells (the cells, 4 bytes each, little-endian).
type autosaveFile struct {
	id   string
	meta autosaveMeta
}

var autosaveSeq int

// writeAutosave saves tab t aside, lossless: its cells as they are and what
// it needs to be a page again.
func writeAutosave(t *Tab) error {
	if t.autoID == "" {
		autosaveSeq++
		t.autoID = fmt.Sprintf("%d-%d-%d", time.Now().UnixNano(), os.Getpid(), autosaveSeq)
	}
	dir := autosaveDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	docs := t.allDocs()
	if t.playing != nil { // the page, not the screen being played
		docs[t.cur] = t.playing.saved.doc
	}
	d := docs[0]
	var cells []byte
	for _, fd := range docs {
		for _, v := range fd.cells[:fd.cols*fd.rows] {
			cells = binary.LittleEndian.AppendUint32(cells, v)
		}
	}
	m := autosaveMeta{PID: os.Getpid(), Saved: time.Now(), Path: t.path, Name: t.name, Mode: t.mode, SaveFmt: t.saveFmt,
		Machine: t.machine, STRes: t.stres, Font: t.fontFor[t.mode], Palette: t.paletteFor[t.mode], Ice: t.ice,
		Cols: d.cols, Rows: d.rows, Bg: d.bg, Border: d.border, Ink: d.ink, Glyph0: d.glyph0,
		Title: t.info.title, Author: t.info.author, Group: t.info.group, WriteSauce: t.info.writeSauce,
		Clear: t.info.clear, Notes: t.info.notes}
	m.SrcKind, m.Order = t.srcKind, t.order
	m.GTIA = t.mode == "atari"
	if t.mode == "uni" {
		m.UniPairs = uniPairsSnapshot()
	}
	if t.mode == "cpc" {
		m.CPC = &cpcAutosave{Mode: t.cpcSet.mode, Inks: t.cpcSet.ink, Border: t.cpcSet.border}
	}
	if t.mode == "msx" {
		m.MSX = t.msxAutosave()
	}
	if t.mode == "c64" {
		v := t.vic
		m.VIC = &v
	}
	if t.ref != nil {
		m.Ref = t.ref.pref()
	}
	if t.frames != nil {
		m.Cur, m.Restructured = t.cur, t.restructured
		for i, f := range t.frames {
			fd := docs[i]
			m.Frames = append(m.Frames, autosaveFrame{Cols: fd.cols, Rows: fd.rows, Bg: fd.bg, Border: fd.border, Ink: fd.ink,
				Glyph0: fd.glyph0, Hold: f.hold, Name: f.name, Lower: f.lower, Seg: f.seg, Src: f.src})
		}
		if t.mode == "c64" {
			m.Frames[t.cur].Lower = map[bool]int{true: 1}[strings.HasSuffix(t.fontFor["c64"], "lower")]
		}
	}
	base := filepath.Join(dir, t.autoID)
	if t.ownFont != nil {
		data := append([]byte(nil), t.ownFont.bits...)
		if t.ownLower != nil {
			data = append(data, t.ownLower.bits...)
		}
		if err := writeAtomic(base+".font", data); err != nil {
			return err
		}
		m.OwnFont = &ArtFont{File: t.autoID + ".font", Name: t.ownFont.name, Kind: t.ownFont.kind, H: t.ownFont.h,
			Lower: t.ownLower != nil, Format: t.ownFont.format}
	}
	js, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := writeAtomic(base+".cells", cells); err != nil {
		return err
	}
	if err := writeAtomic(base+".json", append(js, '\n')); err != nil {
		return err
	}
	t.autoGen = t.gen
	return nil
}

// dropAutosave deletes a tab's autosave (it was saved, or closed on purpose).
func dropAutosave(t *Tab) {
	if t.autoID == "" {
		return
	}
	removeAutosave(t.autoID)
	t.autoID, t.autoGen = "", 0
}

func removeAutosave(id string) {
	base := filepath.Join(autosaveDir(), id)
	os.Remove(base + ".json")
	os.Remove(base + ".cells")
	os.Remove(base + ".font")
}

// autosaveTick saves aside, every autosaveEvery, each page changed since its last autosave.
func (a *App) autosaveTick(now time.Time) {
	if viewer != nil || now.Sub(lastAutosave) < autosaveEvery {
		return
	}
	lastAutosave = now
	a.autosaveNow()
}

func (a *App) autosaveNow() {
	for _, t := range a.tabs {
		if !t.dirty || (t.autoID != "" && t.autoGen == t.gen) {
			continue
		}
		if err := writeAutosave(t); err != nil {
			a.msg("Could not save " + t.name + " aside (autosave): " + err.Error())
		}
	}
}

// sameProgram compares two programs' file names: case aside, .exe aside.
func sameProgram(a, b string) bool {
	trim := func(s string) string { return strings.TrimSuffix(strings.ToLower(s), ".exe") }
	return trim(a) == trim(b)
}

// leftAutosaves are the autosaves of CharEdits that stopped without saving
// their pages (oldest first).
func leftAutosaves() []autosaveFile {
	ents, err := os.ReadDir(autosaveDir())
	if err != nil {
		return nil
	}
	var out []autosaveFile
	for _, e := range ents {
		id, ok := strings.CutSuffix(e.Name(), ".json")
		if !ok {
			continue
		}
		b, err := os.ReadFile(filepath.Join(autosaveDir(), e.Name()))
		var m autosaveMeta
		if err != nil || json.Unmarshal(b, &m) != nil || running(m.PID) {
			continue
		}
		if _, err := os.Stat(filepath.Join(autosaveDir(), id+".cells")); err != nil {
			continue
		}
		out = append(out, autosaveFile{id, m})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].meta.Saved.Before(out[j].meta.Saved) })
	return out
}

// restoreAutosave opens an autosave in a tab of its own, as a page with
// changes (not saved yet); its file is the page's file, if it had one.
func (a *App) restoreAutosave(f autosaveFile) error {
	m := f.meta
	b, err := os.ReadFile(filepath.Join(autosaveDir(), f.id+".cells"))
	if err != nil {
		return err
	}
	frames := m.Frames
	if len(frames) == 0 { // one picture (as autosaves before frames)
		frames = []autosaveFrame{{Cols: m.Cols, Rows: m.Rows, Bg: m.Bg, Border: m.Border, Ink: m.Ink, Glyph0: m.Glyph0, Lower: -1, Src: -1}}
	}
	total := 0
	for _, f := range frames {
		if f.Cols < 1 || f.Rows < 1 || f.Cols > 1000 || f.Rows > maxRowsFor(f.Cols) {
			total = -1
			break
		}
		total += f.Cols * f.Rows
		if total > maxPageCells {
			break
		}
	}
	if total < 1 || total > maxPageCells || len(b) != 4*total || m.Cur < 0 || m.Cur >= len(frames) {
		return fmt.Errorf("%s: the autosave is damaged", m.Name)
	}
	mode := m.Mode
	known := false
	for _, pm := range pageModes {
		known = known || pm == mode
	}
	if !known {
		mode = "pc"
	}
	if !a.pristine() {
		a.focusTab(a.newTab(mode))
	}
	docs := make([]*Doc, len(frames))
	for i, f := range frames {
		d := newDoc(f.Cols, f.Rows)
		for j := range d.cells[:f.Cols*f.Rows] {
			d.cells[j] = binary.LittleEndian.Uint32(b[4*j:])
		}
		b = b[4*f.Cols*f.Rows:]
		d.bg, d.border, d.ink, d.glyph0 = f.Bg, f.Border, f.Ink, f.Glyph0
		if mode == "atari" && !m.GTIA { // colours 0-15 before 2026-10-16
			atariFromOld(d)
		}
		docs[i] = d
	}
	if mode == "uni" {
		uniRemapPairs(docs, m.UniPairs) // this run's table numbers them differently
	}
	a.mode, a.doc = mode, docs[m.Cur]
	if m.Machine != "" {
		a.machine = machineByID(m.Machine).id
	}
	if m.STRes != "" {
		a.stres = stResByID(m.STRes).id
	}
	if m.CPC != nil {
		a.cpcSet = m.CPC.setup()
	}
	if m.MSX != nil {
		a.msxSet = m.MSX.setup()
	}
	if m.VIC != nil && m.VIC.Mode >= 0 && m.VIC.Mode <= vicECM {
		a.vic = *m.VIC
	}
	if m.Font != "" {
		a.fontFor[mode] = m.Font
	}
	if m.OwnFont != nil {
		if up, low, err := readFontCopy(*m.OwnFont, autosaveDir()); err == nil {
			a.ownFont, a.ownLower = up, low
		}
	}
	if m.Palette != "" && m.Palette != "file" {
		a.paletteFor[mode] = m.Palette
	}
	a.ice = m.Ice
	a.path, a.name = m.Path, m.Name
	if a.name == "" {
		a.name = "untitled"
	}
	if m.SaveFmt != "" {
		a.saveFmt = m.SaveFmt
	}
	a.info = Info{title: m.Title, author: m.Author, group: m.Group, writeSauce: m.WriteSauce, clear: m.Clear, notes: m.Notes}
	a.source, a.srcKind, a.frames, a.cur = nil, "", nil, 0
	kind, at := m.SrcKind, 0 // at: an autosave from before frames of one subpage or screen (from 1)
	if m.Subpage > 0 {
		kind, at = "tti", m.Subpage
	}
	if m.Screen > 0 {
		at = m.Screen
	}
	if m.Path != "" && (kind == "tti" || kind == "marq" || kind == "petmate") {
		// The file as it is now: a carousel's head lines, Petmate's own fields, and
		// the other subpages or screens of an autosave from before frames.
		if fb, err := os.ReadFile(m.Path); err == nil && (kind != "tti" || isTTI(fb)) {
			a.source, a.srcKind = fb, kind
		}
	}
	switch {
	case len(m.Frames) > 0:
		a.frames, a.cur, a.restructured = make([]Frame, len(frames)), m.Cur, m.Restructured
		for i, f := range frames {
			a.frames[i] = Frame{doc: docs[i], hold: f.Hold, name: f.Name, lower: f.Lower, seg: f.Seg, src: f.Src}
		}
		a.showCase()
	case at > 0 && a.source != nil:
		a.reparse() // the file's subpages or screens as frames, this one as autosaved
		if at <= a.nFrames() && a.frames != nil {
			a.frames[at-1].doc, a.cur = docs[0], at-1
			a.showCase()
		}
		a.doc = docs[0]
	}
	a.restoreRef(m.Ref)
	if m.Order != nil && m.Order.pat() >= 0 {
		a.order = m.Order
	}
	a.gen = nextGen() // changed, not saved
	a.dirty = true
	a.cx, a.cy = 0, 0
	a.applyMode()
	// Kept aside again under this CharEdit's name before the old one goes.
	if err := writeAutosave(a.Tab); err != nil {
		return err
	}
	removeAutosave(f.id)
	return nil
}

// AutosaveReq offers the pages a CharEdit left unsaved when it stopped.
type AutosaveReq struct{ files []autosaveFile }

func offerAutosaves() bool {
	files := leftAutosaves()
	if len(files) == 0 {
		return false
	}
	a.dialog = &AutosaveReq{files: files}
	return true
}

func (d *AutosaveReq) names() string {
	var n []string
	for _, f := range d.files {
		s := f.meta.Name
		if f.meta.Path != "" {
			s = f.meta.Path
		}
		n = append(n, s+" ("+f.meta.Saved.Format("2006-01-02 15:04")+")")
	}
	return strings.Join(n, ", ")
}

func (d *AutosaveReq) reopen() {
	closeDialog()
	var errs []string
	for _, f := range d.files {
		if err := a.restoreAutosave(f); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		a.msg("Could not open again: " + strings.Join(errs, "; "))
		return
	}
	a.msg("Opened again; they are not saved yet (" + keyName("save") + " saves)")
}

func (d *AutosaveReq) throwAway() {
	closeDialog()
	n := len(d.files)
	text := "Throw the unsaved page away? It cannot be had back."
	if n > 1 {
		text = fmt.Sprintf("Throw the %d unsaved pages away? They cannot be had back.", n)
	}
	a.dialog = &Confirm{text: text, yes: func() {
		for _, f := range d.files {
			removeAutosave(f.id)
		}
		a.msg("Thrown away")
	}}
}

func (d *AutosaveReq) draw(u *UI) {
	in := u.window(centre(620, 190), "Pages not saved")
	text := "A page was not saved when CharEdit last stopped, but kept aside: " + d.names() +
		". Open it again in a tab? (Later: ask again at the next start.)"
	if len(d.files) > 1 {
		text = fmt.Sprintf("%d pages were not saved when CharEdit last stopped, but kept aside: %s. "+
			"Open them again in tabs? (Later: ask again at the next start.)", len(d.files), d.names())
	}
	for i, l := range wrap(text, in.w) {
		u.fb.text(in.x, in.y+i*lineH, l, cText)
	}
	switch buttonRow(u, in, "Throw away", "Later", "Open again") {
	case 0:
		d.throwAway()
	case 1:
		closeDialog()
	case 2:
		d.reopen()
	}
}

func (d *AutosaveReq) key(k string, ctrl, shift bool) bool {
	switch k {
	case "Return", "Keypad Enter":
		d.reopen()
	case "Escape":
		closeDialog()
	default:
		return false
	}
	return true
}

// ---------------------------------------------------------------- .bak

// keepBackup keeps the file a save is about to write over as path.bak (unless
// it already holds just what is being saved).
func keepBackup(path string, data []byte) error {
	old, err := os.ReadFile(path)
	if err != nil || bytes.Equal(old, data) {
		return nil
	}
	return writeAtomic(path+".bak", old)
}

// ---------------------------------------------------------------- recent files

const maxRecent = 10

// addRecent puts path first in the recent files.
func addRecent(path string) {
	out := []string{path}
	for _, p := range prefs.Recent {
		if p != path && len(out) < maxRecent {
			out = append(out, p)
		}
	}
	prefs.Recent = out
}

// reopenRecent opens the most recent file that is not open now (or goes to it).
func (a *App) reopenRecent() {
	var found []string
	for _, p := range prefs.Recent {
		if _, err := os.Stat(p); err == nil {
			found = append(found, p)
		}
	}
	if len(found) == 0 {
		a.msg("No recent files")
		return
	}
	for _, p := range found {
		open := false
		for _, t := range a.tabs {
			open = open || t.path == p
		}
		if !open {
			a.openPath(p, "auto")
			return
		}
	}
	a.openPath(found[0], "auto")
}

// shortPath writes the home folder as ~.
func shortPath(p string) string {
	if h, err := os.UserHomeDir(); err == nil && h != "" && strings.HasPrefix(p, h+string(filepath.Separator)) {
		return "~" + p[len(h):]
	}
	return p
}
