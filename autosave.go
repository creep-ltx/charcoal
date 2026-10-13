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
	"strconv"
	"strings"
	"syscall"
	"time"
)

const autosaveEvery = 2 * time.Minute

var lastAutosave = time.Now()

func autosaveDir() string { return filepath.Join(dataDir(), "charcoal", "autosave") }

// autosaveMeta is what an autosave says about its page, next to the cells.
type autosaveMeta struct {
	PID        int       `json:"pid"` // the CharEdit that wrote it
	Saved      time.Time `json:"saved"`
	Path       string    `json:"path"` // the page's file, "" if it never had one
	Name       string    `json:"name"`
	Mode       string    `json:"mode"`
	SaveFmt    string    `json:"save_format"`
	Machine    string    `json:"machine,omitempty"`
	STRes      string    `json:"st_resolution,omitempty"`
	Font       string    `json:"font,omitempty"`
	Palette    string    `json:"palette,omitempty"`
	Ice        bool      `json:"ice,omitempty"`
	Cols       int       `json:"cols"`
	Rows       int       `json:"rows"`
	Bg         int       `json:"bg"`
	Border     int       `json:"border"`
	Ink        int       `json:"ink"`
	Glyph0     bool      `json:"glyph0,omitempty"`
	Title      string    `json:"title,omitempty"`
	Author     string    `json:"author,omitempty"`
	Group      string    `json:"group,omitempty"`
	WriteSauce bool      `json:"write_sauce"`
	Clear      bool      `json:"clear,omitempty"`
	Notes      []string  `json:"notes,omitempty"`
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
	d := t.doc
	cells := make([]byte, 4*d.cols*d.rows)
	for i, v := range d.cells[:d.cols*d.rows] {
		binary.LittleEndian.PutUint32(cells[4*i:], v)
	}
	m := autosaveMeta{PID: os.Getpid(), Saved: time.Now(), Path: t.path, Name: t.name, Mode: t.mode, SaveFmt: t.saveFmt,
		Machine: t.machine, STRes: t.stres, Font: t.fontFor[t.mode], Palette: t.paletteFor[t.mode], Ice: t.ice,
		Cols: d.cols, Rows: d.rows, Bg: d.bg, Border: d.border, Ink: d.ink, Glyph0: d.glyph0,
		Title: t.info.title, Author: t.info.author, Group: t.info.group, WriteSauce: t.info.writeSauce,
		Clear: t.info.clear, Notes: t.info.notes}
	js, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	base := filepath.Join(dir, t.autoID)
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

// running tells whether the CharEdit with this process id still runs: then
// its autosaves are its own business.
func running(pid int) bool {
	if pid <= 0 || pid == os.Getpid() {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil || p.Signal(syscall.Signal(0)) != nil {
		return false
	}
	// The number may have gone to another program since.
	if comm, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/comm"); err == nil {
		self, _ := os.ReadFile("/proc/self/comm")
		return bytes.Equal(comm, self)
	}
	return true
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
	if m.Cols < 1 || m.Rows < 1 || m.Rows > maxRowsFor(m.Cols) || len(b) != 4*m.Cols*m.Rows {
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
	d := newDoc(m.Cols, m.Rows)
	for i := range d.cells[:m.Cols*m.Rows] {
		d.cells[i] = binary.LittleEndian.Uint32(b[4*i:])
	}
	d.bg, d.border, d.ink, d.glyph0 = m.Bg, m.Border, m.Ink, m.Glyph0
	a.mode, a.doc = mode, d
	if m.Machine != "" {
		a.machine = machineByID(m.Machine).id
	}
	if m.STRes != "" {
		a.stres = stResByID(m.STRes).id
	}
	if m.Font != "" {
		a.fontFor[mode] = m.Font
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
	a.source, a.srcKind = nil, ""
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
	for i, l := range wrap(text, in.w/charW) {
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
