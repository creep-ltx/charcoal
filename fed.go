package main

// The TheDraw font editor: a .tdf font in a tab of its own, each letter a small
// PC page drawn with the editor's own tools (TheDraw's TDFONTS works the same
// way: "the font editor is similar to TheDraw in many respects"). The page's
// width and height are the letter's (W and H); the letter strip under the page
// (drawFedStrip) picks the letter ("!" to "~" in two rows, as TDFONTS shows
// them), names the font, sets its spacing and the letter's descender lines, and
// copies or deletes letters. Ctrl+S writes the .tdf (encodeTDFGlyph,
// encodeTDFFont in tdf.go).
//
// The model: fedTabs maps a font's tab to its fedFont, so the tab itself needs
// no new fields. Like the frames (frames.go), **t.doc is the live current
// letter**; slots[cur].doc is stale while it is on the page. Each letter keeps
// its own undo and redo, swapped in and out with it. A letter read from a file
// is written back byte for byte while its page is as it was read (raw, orig).
// A letter exists (defined) when it came from the file or once something is
// drawn on it; Delete makes it blank and not defined.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// fedSlot is one letter of the font being edited.
type fedSlot struct {
	doc        *Doc   // the letter as a page (nil: never shown; while current: stale, a.doc is it)
	desc       []bool // per row: a descender line ('&': not counted when letters line up)
	defined    bool
	raw        []byte // the letter as the file holds it
	orig       *Doc   // and as it was read: while the page is this, raw is written
	origDesc   []bool
	undo, redo []Snap
}

// fedFont is a font open in the editor.
type fedFont struct {
	name    string
	kind    int // tdfOutline, tdfBlock, tdfColour
	spacing int // the file's byte: the gap between letters + 1 (tdfFont.gap)
	slots   [tdfChars]fedSlot
	cur     int    // the letter on the page
	before  []byte // the file's header and fonts before this one, as read (saved back around it)
	after   []byte // the file's fonts after this one
	place   string // "font 2 of 3": where it sits in its file ("": a file of its own)
	copying bool   // Copy: the next letter clicked gets the current one
	preview struct {
		gen, cur, style int
		fb              *FB
	}
}

var fedTabs = map[*Tab]*fedFont{}

// gap is the columns between letters (the spacing TheDraw shows).
func (ff *fedFont) gap() int { return max(0, ff.spacing-1) }

// fed is the current tab's font, nil when the tab is no font.
func (a *App) fed() *fedFont { return fedTabs[a.Tab] }

// fedSize is the size a new font's letters start at.
const fedNewW, fedNewH = 8, 4

// fedLetter names a slot's character.
func fedLetter(i int) string { return string(rune(tdfFirst + i)) }

// fedGlyphDoc is a letter read from a font as a page: a colour font's cells in
// their colours, a block or outline font's characters in grey on black (the
// font keeps no colours); cells the glyph leaves out are blank.
func fedGlyphDoc(kind int, g *tdfGlyph) (*Doc, []bool) {
	d := newDoc(max(1, g.width()), max(1, len(g.rows)))
	for y, row := range g.rows {
		for x, c := range row {
			at := byte(0x07)
			if kind == tdfColour {
				at = c.attr
			}
			d.set(x, y, attrCell(c.ch, at))
		}
	}
	return d, slices.Clone(g.desc)
}

// newFedFont is a font from a file's font (nil: a new empty one of kind).
func newFedFont(f *tdfFont, name string, kind, spacing int) *fedFont {
	ff := &fedFont{name: name, kind: kind, spacing: spacing}
	if f != nil {
		ff.name, ff.kind, ff.spacing = f.name, f.kind, f.spacing
		if ff.name == "(no name)" {
			ff.name = ""
		}
		for i := range ff.slots {
			g := f.glyph(rune(tdfFirst + i))
			raw := f.rawGlyph(i)
			if g == nil || raw == nil {
				continue
			}
			s := &ff.slots[i]
			s.doc, s.desc = fedGlyphDoc(f.kind, g)
			s.defined, s.raw, s.orig, s.origDesc = true, raw, s.doc.clone(), slices.Clone(s.desc)
		}
	}
	ff.cur = 'A' - tdfFirst
	if !ff.slots[ff.cur].defined {
		for i := range ff.slots {
			if ff.slots[i].defined {
				ff.cur = i
				break
			}
		}
	}
	return ff
}

// fitDesc is desc for a page of rows rows (new rows are no descenders).
func fitDesc(desc []bool, rows int) []bool {
	out := make([]bool, rows)
	copy(out, desc)
	return out
}

// docBlank: nothing is drawn on the page.
func docBlank(d *Doc) bool {
	for _, v := range d.cells[:d.cols*d.rows] {
		if v != blank {
			return false
		}
	}
	return true
}

// fedTake keeps the page in the current letter's slot (the tab stays on it).
func (a *App) fedTake() {
	ff := a.fed()
	if ff == nil {
		return
	}
	s := &ff.slots[ff.cur]
	s.doc = a.doc
	s.desc = fitDesc(s.desc, a.doc.rows)
	if !docBlank(a.doc) {
		s.defined = true
	}
}

// fedRefused: the page is not one a letter can be (another page type, frames, layers).
func (a *App) fedRefused() bool {
	switch {
	case !a.pc():
		a.msg("A TheDraw font is drawn on a PC page: switch the page back to PC (Page menu)")
	case a.nFrames() > 1:
		a.msg("A letter has no frames: delete the other frames first")
	case a.layers != nil:
		a.msg("A letter has no layers: merge them first (Page menu)")
	default:
		return false
	}
	return true
}

// fedGo puts letter i on the page.
func (a *App) fedGo(i int) {
	ff := a.fed()
	if ff == nil || i < 0 || i >= tdfChars || a.fedRefused() {
		return
	}
	if ff.copying {
		ff.copying = false
		a.fedCopyTo(i)
		return
	}
	if i == ff.cur {
		return
	}
	a.endPlay()
	a.fedTake()
	s := &ff.slots[ff.cur]
	s.undo, s.redo = a.undo, a.redo
	cols, rows := a.doc.cols, a.doc.rows
	ff.cur = i
	n := &ff.slots[i]
	if n.doc == nil {
		n.doc = newDoc(cols, rows) // a new letter: the size of the last one
		n.desc = fitDesc(nil, rows)
	}
	a.doc, a.undo, a.redo = n.doc, n.undo, n.redo
	n.undo, n.redo = nil, nil
	a.sel, a.float, a.inRun = nil, nil, false
	a.cx, a.cy = min(a.cx, a.doc.cols-1), min(a.cy, a.doc.rows-1)
	a.scrollX, a.scrollY = 0, 0
	if !n.defined {
		a.msg(fmt.Sprintf("%q is not in the font yet: draw it here (%dx%d; W and H change the size)", fedLetter(i), a.doc.cols, a.doc.rows))
	}
}

// fedCopyTo copies the current letter to letter i (a letter not in the font).
func (a *App) fedCopyTo(i int) {
	ff := a.fed()
	if i == ff.cur {
		a.msg("Copy: stopped")
		return
	}
	if ff.slots[i].defined {
		a.msg(fmt.Sprintf("%q is in the font already: delete it first to copy %q there", fedLetter(i), fedLetter(ff.cur)))
		return
	}
	a.fedTake()
	from := ff.slots[ff.cur]
	ff.slots[i] = fedSlot{doc: a.doc.clone(), desc: slices.Clone(from.desc), defined: true}
	a.fedChanged()
	a.fedGo(i)
	a.msg(fmt.Sprintf("Copied %q to %q", fedLetter(ff.cur), fedLetter(i)))
}

// fedDelete takes the current letter out of the font (its page is cleared; Ctrl+Z brings the drawing back).
func (a *App) fedDelete() {
	ff := a.fed()
	if ff == nil || a.fedRefused() {
		return
	}
	a.pushUndo()
	fillCells(a.doc.cells[:a.doc.cols*a.doc.rows], blank)
	ff.slots[ff.cur].defined = false
	a.msg(fmt.Sprintf("%q is out of the font (%s brings the drawing back; drawing on it puts it in again)", fedLetter(ff.cur), keyName("undo")))
}

// fedChanged: the font itself changed (name, spacing, descenders, a copy): the tab has changes to save.
func (a *App) fedChanged() {
	a.gen = nextGen()
	a.dirty = true
}

// fedDescN is how many of the letter's last rows are descender lines.
func (ff *fedFont) descN(rows int) int {
	d := fitDesc(ff.slots[ff.cur].desc, rows)
	n := 0
	for y := rows - 1; y >= 0 && d[y]; y-- {
		n++
	}
	return n
}

// fedSetDesc makes the letter's last n rows its descender lines.
func (a *App) fedSetDesc(n int) {
	ff := a.fed()
	if ff == nil {
		return
	}
	rows := a.doc.rows
	n = max(0, min(n, rows-1))
	s := &ff.slots[ff.cur]
	s.desc = make([]bool, rows)
	for y := rows - n; y < rows; y++ {
		s.desc[y] = true
	}
	a.fedChanged()
}

// ---------------------------------------------------------------- opening and saving

// fedOpen opens a font in a new tab for editing; path is the .tdf it is saved to ("" asks).
func (a *App) fedOpen(ff *fedFont, path, name string) {
	a.endPlay()
	if !(a.start && a.untouched()) && !a.pristine() {
		a.focusTab(a.newTab("pc"))
	}
	a.fedOpenHere(ff, path, name)
}

// fedOpenHere makes the current tab the font's.
func (a *App) fedOpenHere(ff *fedFont, path, name string) {
	a.start = false
	t := a.Tab
	a.mode, a.ice = "pc", false // a colour font's bit 7 blinks, as in TheDraw
	a.zoom = max(a.zoom, 4)     // a letter is small
	a.applyMode()
	fedTabs[t] = ff
	s := &ff.slots[ff.cur]
	if s.doc == nil {
		s.doc, s.desc = newDoc(fedNewW, fedNewH), fitDesc(nil, fedNewH)
	}
	a.doc, a.frames, a.layers = s.doc, nil, nil
	a.undo, a.redo = nil, nil
	a.path, a.name, a.saveFmt = path, name, "tdf"
	a.source, a.srcKind, a.info = nil, "", Info{}
	a.cx, a.cy, a.sel, a.float = 0, 0, nil, nil
	a.gen = nextGen()
	a.markSaved()
	if path == "" && slices.ContainsFunc(ff.slots[:], func(s fedSlot) bool { return s.defined }) {
		a.dirty = true
	}
	if a.tool == "logo" {
		a.setTool("draw")
	}
}

// fedOpenFont opens a font of the library (the TheDraw fonts list's Edit). A file
// of this one font is saved in place; from a file of several fonts (or a .zip)
// the font is saved as a file of its own.
func (a *App) fedOpenFont(f *tdfFont) {
	ff := newFedFont(f, "", 0, 0)
	path, name := f.file, filepath.Base(f.file)
	b, err := os.ReadFile(f.file)
	fs, e := parseTDF(b, f.file)
	switch {
	case err != nil || e != nil || f.index >= len(fs) || fs[f.index].sum != f.sum || !strings.EqualFold(filepath.Ext(f.file), ".tdf") || !fedWritable(f.file):
		path, name = "", fedFileName(ff.name) // in a .zip, or a file CharEdit may not write: a file of its own
	case len(fs) > 1: // the font's place in its pack: the other fonts are saved back around it as they are
		g := fs[f.index]
		ff.before, ff.after = b[:g.off:g.off], b[g.end:fs[len(fs)-1].end]
		ff.place = fmt.Sprintf("font %d of %d", f.index+1, len(fs))
	}
	a.fedOpen(ff, path, name)
	where := "Ctrl+S saves " + name
	switch {
	case path == "":
		where = "it cannot be saved where it is (a .zip, or a file CharEdit may not write): saving asks for " + name
	case ff.place != "":
		where = "Ctrl+S saves it back into " + name + " (" + ff.place + "; the other fonts as they are)"
	}
	a.msg(fmt.Sprintf("Editing %s (%s): the letter strip under the page picks the letter; %s", f.name, tdfKindNames[f.kind], strings.Replace(where, "Ctrl+S", keyName("save"), 1)))
}

// fedWritable: may CharEdit write the file?
func fedWritable(p string) bool {
	f, err := os.OpenFile(p, os.O_WRONLY, 0)
	if err != nil {
		return false
	}
	f.Close()
	return true
}

// fedFileName is a file name for a font of this name.
func fedFileName(name string) string {
	n := strings.Map(func(r rune) rune {
		if r < 'A' && (r < '0' || r > '9') && r != '-' || strings.ContainsRune(`<>:"/\|?*[]`, r) {
			return '_'
		}
		return r
	}, strings.TrimSpace(name))
	if n == "" {
		n = "font"
	}
	return n + ".tdf"
}

// fedEncode is the font as a .tdf file. The letters' raw bytes are taken again
// from what is written, so the next save of an unchanged letter is the same.
func (a *App) fedEncode() ([]byte, error) {
	ff := a.fed()
	if ff == nil {
		return nil, errors.New("this tab holds no TheDraw font")
	}
	if !a.pc() || a.nFrames() > 1 || a.layers != nil {
		return nil, errors.New("a TheDraw font is a PC page of one frame and one layer")
	}
	a.fedTake()
	var glyphs [tdfChars][]byte
	var loss tdfLoss
	for i := range ff.slots {
		s := &ff.slots[i]
		if !s.defined || s.doc == nil {
			continue
		}
		desc := fitDesc(s.desc, s.doc.rows)
		if s.raw != nil && s.orig != nil && slices.Equal(desc, fitDesc(s.origDesc, s.orig.rows)) && sameDoc(s.doc, s.orig) {
			glyphs[i] = s.raw
			continue
		}
		g, l, err := encodeTDFGlyph(ff.kind, s.doc, desc)
		if err != nil {
			return nil, fmt.Errorf("%q: %v", fedLetter(i), err)
		}
		glyphs[i] = g
		loss.add(l)
	}
	font, err := encodeTDFFont(ff.name, ff.kind, ff.spacing, &glyphs)
	if err != nil {
		return nil, err
	}
	for i := range ff.slots {
		s := &ff.slots[i]
		if glyphs[i] != nil {
			s.raw, s.orig, s.origDesc = glyphs[i], s.doc.clone(), fitDesc(s.desc, s.doc.rows)
		}
	}
	n := 0
	for _, g := range glyphs {
		if g != nil {
			n++
		}
	}
	saveNote = fmt.Sprintf(": %s, %d letters", tdfKindNames[ff.kind], n)
	if loss.cr > 0 {
		saveNote += fmt.Sprintf("; %d cells of character 13 written as spaces (it ends a line in a TheDraw font)", loss.cr)
	}
	if loss.amp > 0 {
		saveNote += fmt.Sprintf("; %d '&' written as spaces (TheDraw would read a line's end: light magenta on black, or at a letter's right edge)", loss.amp)
	}
	if ff.before != nil { // back into its pack, between the other fonts as they were read
		saveNote += ", " + ff.place + " in the file (the others as they were)"
		out := append(slices.Clone(ff.before), font...)
		return append(out, ff.after...), nil
	}
	return tdfFileBytes(font), nil
}

// sameDoc: the same size and cells.
func sameDoc(x, y *Doc) bool {
	return x.cols == y.cols && x.rows == y.rows && slices.Equal(x.cells[:x.cols*x.rows], y.cells[:y.cols*y.rows])
}

// fedFont is the font as the reader sees it (for the preview and the tests).
func (ff *fedFont) tdf(cur *Doc) *tdfFont {
	var glyphs [tdfChars][]byte
	for i := range ff.slots {
		s := ff.slots[i]
		d := s.doc
		if i == ff.cur && cur != nil {
			d = cur
		}
		if (!s.defined && !(i == ff.cur && cur != nil && !docBlank(cur))) || d == nil {
			continue
		}
		glyphs[i], _, _ = encodeTDFGlyph(ff.kind, d, fitDesc(s.desc, d.rows))
	}
	b, err := encodeTDFFont(ff.name, ff.kind, ff.spacing, &glyphs)
	if err != nil {
		return nil
	}
	fs, err := parseTDF(tdfFileBytes(b), "")
	if err != nil || len(fs) != 1 {
		return nil
	}
	return fs[0]
}

// ---------------------------------------------------------------- autosave (autosave.go)

// A font tab's autosave keeps the whole font beside the page's cells: id.tdfed
// holds the letter on the page, the font's place in the file (0: a file of its
// own, n: the n-th font of a pack) and the file as Ctrl+S would write it.
func writeFedAutosave(t *Tab, base string) error {
	ff := fedTabs[t]
	if ff == nil {
		return nil
	}
	old := a.Tab
	a.Tab = t
	b, err := a.fedEncode()
	a.Tab, saveNote = old, ""
	if err != nil {
		return nil // the page itself is kept; the font as it was last saved
	}
	at := 0
	if ff.before != nil {
		fmt.Sscanf(ff.place, "font %d", &at)
	}
	return writeAtomic(base+".tdfed", append([]byte{byte(ff.cur), byte(at)}, b...))
}

// restoreFedAutosave makes the restored tab a font tab again (false: it was none).
func (a *App) restoreFedAutosave(id string, path, name string) bool {
	b, err := os.ReadFile(filepath.Join(autosaveDir(), id+".tdfed"))
	if err != nil || len(b) < 3 {
		return false
	}
	file := b[2:]
	fs, err := parseTDF(file, path)
	at := max(1, int(b[1]))
	if err != nil || at > len(fs) || b[1] == 0 && len(fs) != 1 {
		return false
	}
	f := fs[at-1]
	ff := newFedFont(f, "", 0, 0)
	if b[1] > 0 {
		ff.before, ff.after = file[:f.off:f.off], file[f.end:fs[len(fs)-1].end]
		ff.place = fmt.Sprintf("font %d of %d", at, len(fs))
	}
	if c := int(b[0]); c < tdfChars {
		ff.cur = c
	}
	for i := range ff.slots { // nothing of it is in the file yet
		ff.slots[i].raw, ff.slots[i].orig = nil, nil
	}
	page := a.doc
	a.fedOpenHere(ff, path, name)
	if s := &ff.slots[ff.cur]; page != nil && sameDocSize(page, s.doc) {
		a.doc, s.doc = page, page
	}
	a.dirty = true
	return true
}

func sameDocSize(x, y *Doc) bool { return x.cols == y.cols && x.rows == y.rows }

// ---------------------------------------------------------------- the letter strip

var (
	fieldFedName    = &Field{max: 12}
	fieldFedSpacing = &Field{max: 2}
	fieldFedDesc    = &Field{max: 2}
)

func init() {
	fieldFedName.enter = func() {
		if ff := a.fed(); ff != nil && fieldFedName.String() != ff.name {
			ff.name = fieldFedName.String()
			a.fedChanged()
		}
		ui.focus = nil
	}
	fieldFedSpacing.enter = func() {
		if ff := a.fed(); ff != nil {
			if n, err := strconv.Atoi(strings.TrimSpace(fieldFedSpacing.String())); err == nil && n != ff.gap() {
				ff.spacing = max(0, min(40, n)) + 1 // the file's byte is the gap + 1 (tdfFont.gap)
				a.fedChanged()
			}
		}
		ui.focus = nil
	}
	fieldFedDesc.enter = func() {
		if ff := a.fed(); ff != nil {
			if n, err := strconv.Atoi(strings.TrimSpace(fieldFedDesc.String())); err == nil && n != ff.descN(a.doc.rows) {
				a.fedSetDesc(n)
			}
		}
		ui.focus = nil
	}
	formats = append(formats, Format{"tdf", "TheDraw font (.tdf)", ".tdf", "tdf"})
	editorOwn["tdf"] = true
}

// fedFormats are what a font tab saves as: only the font.
func (a *App) fedFormats() []Format {
	for _, f := range formats {
		if f.id == "tdf" {
			return []Format{f}
		}
	}
	return nil
}

// fedStripH is the letter strip's height: two rows of letters and the font's row.
func fedStripH() int { return 3*(gadH+3) + 3 }

// drawFedStrip draws the letter strip in r.
func (a *App) drawFedStrip(u *UI, r Rect) {
	ff := a.fed()
	fb := u.fb
	fb.fill(r, cDark)
	fb.fill(Rect{r.x, r.y, r.w, 1}, cEdgeDk)
	h := gadH
	// The preview: the letter as the Logo tool types it, at the right of the letters.
	pw := min(r.w/4, 220)
	pr := Rect{r.x + r.w - pw - 6, r.y + 3, pw, 2*(h+3) - 3}
	// The letters, in two rows as TheDraw's TDFONTS shows them.
	boxW := max(textW("W"), textW("@")) + 6
	per := (tdfChars + 1) / 2
	x0 := r.x + 6
	if room := pr.x - 8 - x0; boxW*per > room {
		boxW = max(8, room/per)
	}
	for i := 0; i < tdfChars; i++ {
		br := Rect{x0 + (i%per)*boxW, r.y + 3 + (i/per)*(h+3), boxW - 1, h}
		s := ff.slots[i]
		bg, col := uint32(cDark), uint32(cDim) // not in the font: no box
		if s.defined || i == ff.cur && !docBlank(a.doc) {
			bg, col = cGrey, cText
		}
		if i == ff.cur {
			bg, col = cBlue, cHi
		}
		fb.fill(br, bg)
		l := fedLetter(i)
		fb.text(br.x+(br.w-textW(l))/2, br.y+(h-lineH)/2, l, col)
		hint := fmt.Sprintf("%q: ", l)
		switch {
		case s.defined && s.doc != nil:
			hint += fmt.Sprintf("%dx%d", s.doc.cols, s.doc.rows)
		case s.defined || i == ff.cur:
			hint += "in the font"
		default:
			hint += "not in the font: click to draw it"
		}
		if ff.copying {
			hint = fmt.Sprintf("Copy %q to %q", fedLetter(ff.cur), l)
		}
		u.setHint(br, hint)
		if u.take(br) != nil {
			a.fedGo(i)
		}
	}
	a.drawFedPreview(fb, pr)

	// The font's row.
	y := r.y + 3 + 2*(h+3)
	x := r.x + 6
	label := func(s string) {
		x = fb.text(x, y+3, s, cText) + 6
	}
	field := func(fl *Field, w int, now, hint string) {
		if u.focus != fl {
			fl.set(now)
		}
		u.field(Rect{x, y, w, h}, fl, hint)
		x += w + 12
	}
	label("Font")
	field(fieldFedName, textW("MMMMMMMMMMMM")+8, ff.name, "The font's name, up to 12 characters (Enter sets it)")
	x = fb.text(x, y+3, tdfKindNames[ff.kind], cDim) + 12
	label("Spacing")
	field(fieldFedSpacing, digitsW(2)+8, strconv.Itoa(ff.gap()), "Columns TheDraw puts between the letters, 0-40 (Enter sets it)")
	x += 8
	label(fmt.Sprintf("%q %dx%d", fedLetter(ff.cur), a.doc.cols, a.doc.rows))
	label("Descender lines")
	field(fieldFedDesc, digitsW(2)+8, strconv.Itoa(ff.descN(a.doc.rows)), "The letter's last lines that hang below the others (g, j, p, q, y): TheDraw leaves them out when it lines letters up")
	if u.button(Rect{x, y, buttonW("Copy to"), h}, "Copy to", ff.copying, "Copy this letter to another one: click the letter after this") != nil {
		ff.copying = !ff.copying
		if ff.copying {
			a.msg(fmt.Sprintf("Copy %q: click the letter to copy it to", fedLetter(ff.cur)))
		}
	}
	x += buttonW("Copy to") + 4
	if u.button(Rect{x, y, buttonW("Delete"), h}, "Delete", false, "Take this letter out of the font") != nil {
		a.fedDelete()
	}
}

// drawFedPreview draws the current letter as the font draws it (outline fonts in the Logo tool's outline style).
func (a *App) drawFedPreview(fb *FB, r Rect) {
	ff := a.fed()
	fb.fill(r, cGrey)
	p := &ff.preview
	if p.fb == nil || p.gen != a.gen || p.cur != ff.cur || p.style != tdfStyle() {
		p.gen, p.cur, p.style, p.fb = a.gen, ff.cur, tdfStyle(), nil
		if f := ff.tdf(a.doc); f != nil {
			if pic := f.render(rune(tdfFirst+ff.cur), tdfStyle()); pic != nil {
				p.fb = a.renderDoc(a.tdfDoc(pic), 1)
			}
		}
	}
	if p.fb != nil {
		blitScaled(fb, r.inset(2), p.fb)
	}
}

// ---------------------------------------------------------------- a new font

// FedNewReq asks for a new font's name and kind.
type FedNewReq struct {
	name    Field
	kind    int
	spacing Field
}

func openFedNew() {
	d := &FedNewReq{kind: tdfBlock}
	d.name.max, d.spacing.max = 12, 2
	d.name.set("New font")
	d.spacing.set("1")
	d.name.enter, d.spacing.enter = d.make, d.make
	a.dialog = d
	ui.focus = &d.name
}

func (d *FedNewReq) make() {
	sp, _ := strconv.Atoi(strings.TrimSpace(d.spacing.String()))
	ff := newFedFont(nil, d.name.String(), d.kind, max(0, min(40, sp))+1) // the byte is the gap + 1
	closeDialog()
	a.fedOpen(ff, "", fedFileName(ff.name))
	a.msg(fmt.Sprintf("New %s font: draw %q, then pick the next letter in the strip under the page", tdfKindNames[ff.kind], fedLetter(ff.cur)))
}

var fedKindHints = []string{
	"Outline: letters drawn with A-P and @ that become any of TheDraw's 19 outline styles",
	"Block: characters only, drawn in the colours of the moment",
	"Colour: characters with their own colours",
}

func (d *FedNewReq) draw(u *UI) {
	fb := u.fb
	in := u.window(centre(520, 220), "New TheDraw font")
	y := in.y
	col := colAfter(48, "Name", "Kind", "Spacing")
	fb.text(in.x, y+3, "Name", cText)
	u.field(Rect{in.x + col, y, textW("MMMMMMMMMMMM") + 8, gadH}, &d.name, "Up to 12 characters")
	y += gadH + 8
	fb.text(in.x, y+3, "Kind", cText)
	x := in.x + col
	for _, k := range []int{tdfBlock, tdfColour, tdfOutline} {
		if u.button(Rect{x, y, buttonW(tdfKindNames[k]), gadH}, tdfKindNames[k], d.kind == k, fedKindHints[k]) != nil {
			d.kind = k
		}
		x += buttonW(tdfKindNames[k]) + 4
	}
	y += gadH + 8
	fb.text(in.x, y+3, "Spacing", cText)
	u.field(Rect{in.x + col, y, digitsW(2) + 8, gadH}, &d.spacing, "Columns between the letters, 0-40")
	y += gadH + 8
	for _, l := range wrap(fedKindHints[d.kind], in.w) {
		fb.text(in.x, y, l, cDim)
		y += lineH
	}
	switch buttonRow(u, in, "Cancel", "Make") {
	case 0:
		closeDialog()
	case 1:
		d.make()
	}
}

func (d *FedNewReq) key(k string, ctrl, shift bool) bool {
	switch k {
	case "Escape":
		closeDialog()
	case "Return", "Keypad Enter":
		d.make()
	case "Tab":
		if ui.focus == &d.name {
			ui.focus = &d.spacing
		} else {
			ui.focus = &d.name
		}
	default:
		return false
	}
	return true
}

// openFedEdit: the TheDraw fonts list, to pick a font to edit.
func openFedEdit() {
	openTDFPicker(nil)
	if d, ok := a.dialog.(*TDFReq); ok {
		d.editing, d.all = true, true // any font can be edited, whatever page is open
		d.listFor = "\x00"
		d.filter()
	}
}
