package main

// TheDraw fonts in the editor (tdf.go reads them): the Logo tool types a font's
// glyphs at the cursor, one undo step per glyph; the font picker lists every font
// with a preview; the line requester lays out a whole line and hands it over as a
// block to stamp. Fonts come from folders the user adds and from Roy/SAC's
// collection, downloaded on request (fonts are other people's art: none come with
// the program).

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// tdfPrefs is what prefs.json keeps for the Logo tool.
type tdfPrefs struct {
	Paths   []string `json:"paths,omitempty"`   // folders and files of .tdf the user added
	Font    string   `json:"font,omitempty"`    // the font in use (tdfFont.sum)
	Outline int      `json:"outline,omitempty"` // outline style 0-18
	Spacing int      `json:"spacing,omitempty"` // 0: the font's own, n: n-1 columns
	Text    string   `json:"text,omitempty"`    // the line requester's text
}

func (a *App) tdfPrefs() *tdfPrefs {
	if prefs.TDF == nil {
		prefs.TDF = &tdfPrefs{}
	}
	return prefs.TDF
}

// tdfSpacing is the spacing the tool uses: -1 for the font's own.
func tdfSpacing() int {
	if prefs.TDF == nil {
		return -1
	}
	return prefs.TDF.Spacing - 1
}

func tdfStyle() int {
	if prefs.TDF == nil {
		return 0
	}
	return max(0, min(len(tdfOutlines)-1, prefs.TDF.Outline))
}

// ---------------------------------------------------------------- the library

// tdfLib is every font found: in the download folder and in the user's paths.
var tdfLib struct {
	fonts  []*tdfFont
	loaded bool
	fits   map[*tdfFont]bool // an Amiga page can show every character of the font
}

// tdfDir is where downloaded fonts are kept.
func tdfDir() string { return filepath.Join(appDataDir(), "tdf") }

// tdfZipURL is Roy/SAC's collection of TheDraw fonts (a variable for the tests).
var tdfZipURL = "https://www.roysac.com/images/galleries/THEDRAWFONTS/TOOL/TDFONTSALL.ZIP"

const (
	tdfFileMax  = 64 << 20 // a font file larger than this is no font file
	tdfFilesMax = 20000    // files looked at in one folder tree
)

// tdfLoad reads the library if it has not been read yet.
func tdfLoad() {
	if !tdfLib.loaded {
		tdfReload()
	}
}

// tdfReload reads every font again; it returns the files that could not be read.
func tdfReload() []string {
	var paths []string
	paths = append(paths, tdfDir())
	if prefs.TDF != nil {
		paths = append(paths, prefs.TDF.Paths...)
	}
	fonts, bad := tdfScan(paths)
	tdfLib.fonts, tdfLib.loaded, tdfLib.fits = fonts, true, map[*tdfFont]bool{}
	return bad
}

// tdfScan reads the fonts of files and folders (and .zip files in them), each font once.
func tdfScan(paths []string) ([]*tdfFont, []string) {
	var fonts []*tdfFont
	var bad []string
	seen := map[string]bool{}
	add := func(fs []*tdfFont) {
		for _, f := range fs {
			if !seen[f.sum] {
				seen[f.sum] = true
				fonts = append(fonts, f)
			}
		}
	}
	files := 0
	for _, p := range paths {
		st, err := os.Stat(p)
		if err != nil {
			continue
		}
		if !st.IsDir() {
			fs, err := tdfReadFile(p)
			if err != nil {
				bad = append(bad, p)
			}
			add(fs)
			continue
		}
		filepath.WalkDir(p, func(path string, e fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if files >= tdfFilesMax {
				return filepath.SkipAll
			}
			ext := strings.ToLower(filepath.Ext(path))
			if e.IsDir() || (ext != ".tdf" && ext != ".zip") {
				return nil
			}
			files++
			fs, err := tdfReadFile(path)
			if err != nil && ext == ".tdf" {
				bad = append(bad, path)
			}
			add(fs)
			return nil
		})
	}
	slices.SortStableFunc(fonts, func(x, y *tdfFont) int {
		return strings.Compare(strings.ToLower(x.name), strings.ToLower(y.name))
	})
	return fonts, bad
}

// tdfReadFile reads the fonts of a .tdf file, or of the .tdf files in a .zip.
func tdfReadFile(path string) ([]*tdfFont, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if st.Size() > tdfFileMax {
		return nil, fmt.Errorf("%s is too large for a font file", path)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if bytes.HasPrefix(b, []byte("PK\x03\x04")) {
		return tdfFromZip(b, path)
	}
	return parseTDF(b, path)
}

// tdfFromZip reads the .tdf files inside a zip.
func tdfFromZip(b []byte, path string) ([]*tdfFont, error) {
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return nil, err
	}
	var fonts []*tdfFont
	for _, zf := range zr.File {
		if !strings.EqualFold(filepath.Ext(zf.Name), ".tdf") || zf.UncompressedSize64 > tdfFileMax {
			continue
		}
		data, err := tdfUnzip(zf)
		if err != nil {
			continue
		}
		if fs, err := parseTDF(data, path+"/"+zf.Name); err == nil {
			fonts = append(fonts, fs...)
		}
	}
	return fonts, nil
}

func tdfUnzip(zf *zip.File) ([]byte, error) {
	rc, err := zf.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(io.LimitReader(rc, tdfFileMax))
}

// tdfNow is the font in use (the first one when none was chosen); nil with no fonts.
func tdfNow() *tdfFont {
	tdfLoad()
	if len(tdfLib.fonts) == 0 {
		return nil
	}
	if prefs.TDF != nil {
		for _, f := range tdfLib.fonts {
			if f.sum == prefs.TDF.Font {
				return f
			}
		}
	}
	return tdfLib.fonts[0]
}

func tdfChoose(f *tdfFont) {
	a.tdfPrefs().Font = f.sum
	savePrefs()
}

// fetchTDF downloads Roy/SAC's collection into tdfDir() (the network part).
func fetchTDF() error {
	client := &http.Client{Timeout: 120 * time.Second}
	b, err := fetchBody(client, tdfZipURL, 64<<20)
	if err != nil {
		return err
	}
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return fmt.Errorf("the download is not the zip file expected: %v", err)
	}
	n := 0
	for _, zf := range zr.File {
		name := filepath.Base(filepath.FromSlash(zf.Name))
		if !strings.EqualFold(filepath.Ext(name), ".tdf") || zf.UncompressedSize64 > tdfFileMax {
			continue
		}
		data, err := tdfUnzip(zf)
		if err != nil || !isTDF(data) {
			continue
		}
		if err := os.MkdirAll(tdfDir(), 0o755); err != nil {
			return err
		}
		if err := writeAtomic(filepath.Join(tdfDir(), name), data); err != nil {
			return err
		}
		n++
	}
	if n == 0 {
		return fmt.Errorf("the download holds no TheDraw font files")
	}
	return nil
}

func loadTDFDownloaded() (string, error) {
	tdfReload()
	if len(tdfLib.fonts) == 0 {
		return "", fmt.Errorf("no fonts could be read")
	}
	return fmt.Sprintf("%d TheDraw fonts are here (in %s)", len(tdfLib.fonts), shownDir(tdfDir())), nil
}

func tdfWhy() string {
	return "TheDraw fonts are other people's art, so none come with this program. Roy/SAC (roysac.com) keeps a " +
		"collection of over 3,500 of them for free download. Download it now? (3 MB, kept in " + shownDir(tdfDir()) +
		".) Or close this and add a folder of your own .tdf files in the font list."
}

// openTDFDownload asks to download Roy/SAC's collection; after runs once it is here.
func openTDFDownload(after func()) {
	a.dialog = &FontReq{title: "TheDraw fonts", why: tdfWhy(), after: after,
		what: "Roy/SAC's TheDraw fonts", fetch: fetchTDF, load: loadTDFDownloaded}
}

// addTDFPath adds a .tdf file or a folder of them to the library.
func addTDFPath(p string) int {
	p = filepath.Clean(p)
	tp := a.tdfPrefs()
	before := len(tdfLib.fonts)
	if !slices.Contains(tp.Paths, p) {
		tp.Paths = append(tp.Paths, p)
	}
	tdfReload()
	savePrefs()
	return len(tdfLib.fonts) - before
}

// openTDFFile is a .tdf opened or dropped: its fonts join the list, the list opens.
func openTDFFile(path string) {
	tdfLoad()
	n := addTDFPath(path)
	fs, _ := tdfReadFile(path)
	a.msg(fmt.Sprintf("%s: %d fonts (%d new) in the Logo tool's list", filepath.Base(path), len(fs), n))
	if len(fs) > 0 {
		tdfChoose(fs[0]) // by its sum: the library's copy of it
	}
	if a.tdfPage() {
		a.start = false
		a.setTool("logo")
	}
	openTDFPicker(nil)
}

// ---------------------------------------------------------------- glyphs on the page

// tdfPage: can this page take TheDraw fonts? (Their characters are CP437.)
func (a *App) tdfPage() bool { return a.pc() || a.amiga() || a.uni() }

// tdfCode is the page's character for a CP437 byte; false when the page has none
// (an Amiga page shows Latin-1: no block or line characters).
func (a *App) tdfCode(ch byte) (int, bool) {
	switch {
	case a.uni():
		c := uniFromRune(byteRune(int(ch), false))
		return c, c >= 0
	case a.amiga():
		r := byteRune(int(ch), false)
		if r <= 0xFF && a.printable(int(r)) {
			return int(r), true
		}
		return 0, false
	}
	return int(ch), true
}

// tdfPageCell is a glyph cell as a cell of the page.
func (a *App) tdfPageCell(c tdfCell) (uint32, bool) {
	code, ok := a.tdfCode(c.ch)
	if !ok {
		return 0, false
	}
	if c.kind == tdfInk {
		return a.cell(code), true
	}
	fg, bg := biosANSI[c.attr&15], biosANSI[c.attr>>4]
	if a.uni() {
		return uniCell(code, styleOf(fg, bg, 0)), true
	}
	return makeCell(code, fg, bg, 0), true
}

// tdfMissing is the first character of the picture the page cannot show (0: none).
func (a *App) tdfMissing(p *tdfPic) rune {
	for _, c := range p.cells {
		if c.kind != tdfNone {
			if _, ok := a.tdfCode(c.ch); !ok {
				return byteRune(int(c.ch), false)
			}
		}
	}
	return 0
}

// tdfFits: can this page show every character the font draws? (Only Amiga pages lack some.)
func (a *App) tdfFits(f *tdfFont) bool {
	if !a.amiga() {
		return true
	}
	if v, ok := tdfLib.fits[f]; ok {
		return v
	}
	fits := true
	for c := rune(tdfFirst); c < tdfFirst+tdfChars && fits; c++ {
		if p := f.render(c, tdfStyle()); p != nil && a.tdfMissing(p) != 0 {
			fits = false
		}
	}
	if tdfLib.fits == nil {
		tdfLib.fits = map[*tdfFont]bool{}
	}
	tdfLib.fits[f] = fits
	return fits
}

// tdfDoc is a picture as a small page, for previews.
func (a *App) tdfDoc(p *tdfPic) *Doc {
	d := newDoc(max(1, p.w), max(1, p.h))
	d.uni = a.uni()
	for y := 0; y < p.h; y++ {
		for x := 0; x < p.w; x++ {
			if c := p.at(x, y); c.kind != tdfNone {
				v, ok := a.tdfPageCell(c)
				if !ok {
					v = a.cell('?')
				}
				d.set(x, y, v)
			}
		}
	}
	return d
}

// tdfBlock is a laid-out line as a block to stamp (cells the font leaves out are blank).
func (a *App) tdfBlock(p *tdfPic) *Block {
	d := a.tdfDoc(p)
	b := d.copyRect(0, 0, d.cols, d.rows)
	b.uni = a.uni()
	return b
}

// logo is the Logo tool's run of typing: where Return goes back to, and the
// generations of the glyphs typed (Backspace undoes them one by one).
var logo struct {
	tab   *Tab
	left  int // the column Return goes back to
	x, y  int // where the last glyph left the cursor (x may be past the page's width)
	gens  []int
	shown string // the font last named in the status line
}

// logoRun: does typing go on from the last glyph? Otherwise a new run starts at the cursor.
func (a *App) logoRun() {
	if logo.tab == a.Tab {
		if i := slices.Index(logo.gens, a.gen); i >= 0 { // undone back to after one of the run's glyphs
			logo.gens = logo.gens[:i+1]
			if !a.logoAt() {
				logo.x, logo.y = a.cx, a.cy
			}
			return
		}
		if a.logoAt() {
			return
		}
	}
	logo.tab, logo.left, logo.x, logo.y, logo.gens = a.Tab, a.cx, a.cx, a.cy, nil
}

// logoAt: is the cursor where the last glyph left it?
func (a *App) logoAt() bool { return a.cy == logo.y && a.cx == min(a.doc.cols-1, logo.x) }

// logoText types text with the Logo tool.
func (a *App) logoText(s string) {
	f := tdfNow()
	if f == nil {
		openTDFPicker(nil)
		return
	}
	for _, r := range s {
		a.logoGlyph(f, r)
	}
}

// logoGlyph puts one glyph at the cursor and moves on past it (one undo step).
func (a *App) logoGlyph(f *tdfFont, r rune) {
	a.logoRun()
	if r == ' ' {
		logo.x += f.spaceWidth()
		a.cx = min(a.doc.cols-1, logo.x)
		a.ensureVisible(a.cx, a.cy)
		return
	}
	p := f.render(r, tdfStyle())
	if p == nil {
		a.msg(fmt.Sprintf("%q has no %q", f.name, r))
		return
	}
	if m := a.tdfMissing(p); m != 0 {
		a.msg(fmt.Sprintf("%q draws %q with %q, which an Amiga page (Latin-1) does not have", f.name, r, m))
		return
	}
	x := logo.x
	if x+p.w > a.doc.cols && x > logo.left { // it would not fit: on to the next line
		x, logo.y = logo.left, logo.y+f.lineHeight()
	}
	if logo.y >= a.maxRows() {
		a.msg("The page ends here")
		return
	}
	a.cx, a.cy = min(a.doc.cols-1, x), logo.y
	a.pushUndo()
	for y := 0; y < p.h; y++ {
		for px := 0; px < p.w; px++ {
			if c := p.at(px, y); c.kind != tdfNone {
				if v, ok := a.tdfPageCell(c); ok {
					a.setCell(x+px, logo.y+y, v)
				}
			}
		}
	}
	sp := tdfSpacing()
	if sp < 0 {
		sp = f.spacing
	}
	logo.x = x + p.w + sp
	a.cx = min(a.doc.cols-1, logo.x)
	logo.gens = append(logo.gens, a.gen)
	a.ensureVisible(a.cx, a.cy)
	a.blink = true
}

// logoKey: Backspace undoes the last glyph, Return goes to the next line of
// letters. False for other keys.
func (a *App) logoKey(k string) bool {
	switch k {
	case "Backspace":
		if logo.tab != a.Tab || len(logo.gens) == 0 || logo.gens[len(logo.gens)-1] != a.gen || !a.logoAt() {
			return false
		}
		logo.gens = logo.gens[:len(logo.gens)-1]
		a.doUndo()
		logo.x, logo.y = a.cx, a.cy
		return true
	case "Return", "Keypad Enter":
		f := tdfNow()
		if f == nil {
			return false
		}
		a.logoRun()
		gens := logo.gens
		a.cx, a.cy = logo.left, min(a.maxRows()-1, logo.y+f.lineHeight())
		before := a.gen
		a.moved() // past the last row: the page grows (an undo step of its own)
		if a.gen != before {
			gens = append(gens, a.gen)
		}
		left := logo.left
		logo.tab, logo.left, logo.x, logo.y, logo.gens = a.Tab, left, a.cx, a.cy, gens
		return true
	}
	return false
}

// logoClick: a click with the Logo tool puts the cursor there; a new run starts.
func (a *App) logoClick(x, y int) {
	a.cx, a.cy, a.sel = x, y, nil
	logo.tab = nil
	a.moved()
}

// logoTool picks the Logo tool (only on pages that can take the fonts).
func (a *App) logoTool() bool {
	if !a.tdfPage() {
		a.msg("TheDraw fonts are CP437 art: the Logo tool works on PC, Amiga and Unicode pages")
		return false
	}
	tdfLoad()
	if len(tdfLib.fonts) == 0 {
		a.start = false
		a.setTool("logo")
		openTDFDownload(func() { openTDFPicker(nil) })
		return false
	}
	if f := tdfNow(); f != nil && logo.shown != f.sum {
		logo.shown = f.sum
		a.msg("Logo: type in " + f.name + " (" + tdfKindNames[f.kind] + "); Backspace takes the last letter back")
	}
	return true
}

// ---------------------------------------------------------------- the tool's options

var tdfSpacingNames = []string{"Font's spacing", "Spacing 0", "Spacing 1", "Spacing 2", "Spacing 3", "Spacing 4", "Spacing 5"}

func tdfOutlineNames() []string {
	out := make([]string, len(tdfOutlines))
	for i := range out {
		out[i] = fmt.Sprintf("Outline %d", i+1)
	}
	return out
}

// drawLogoOptions draws the Logo tool's options from (x, y) in rows ending before right; the y under them.
func (a *App) drawLogoOptions(u *UI, x, y, right int) int {
	x0 := x
	place := func(w int) Rect {
		if x > x0 && x+w > right {
			x, y = x0, y+smallH+4
		}
		r := Rect{x, y, min(w, right-x0), smallH}
		x += w + 4
		return r
	}
	f := tdfNow()
	label := "Choose a font..."
	if f != nil {
		label = f.name + " (" + tdfKindNames[f.kind] + ")"
	}
	if u.button(place(buttonW(label)), label, false, "The TheDraw font the Logo tool types in: the list of fonts ("+keyName("logo_font")+")") != nil {
		openTDFPicker(nil)
	}
	tp := a.tdfPrefs()
	names := tdfSpacingNames
	if f != nil {
		names = slices.Clone(names)
		names[0] = fmt.Sprintf("Font's spacing (%d)", f.spacing)
	}
	u.choice("tdfspacing", place(choiceW(names)), names, max(0, min(len(names)-1, tp.Spacing)), "Columns between the letters", func(i int) {
		tp.Spacing = i
		savePrefs()
	})
	if f != nil && f.kind == tdfOutline {
		on := tdfOutlineNames()
		u.choice("tdfoutline", place(choiceW(on)), on, tdfStyle(), "TheDraw's 19 outline styles: the lines an outline font is drawn with", func(i int) {
			tp.Outline = i
			tdfLib.fits = map[*tdfFont]bool{}
			savePrefs()
		})
		r := place(6 * a.cellW())
		a.drawOutlineSample(u.fb, r.x, r.y, tdfStyle())
	}
	if u.button(place(buttonW("Line...")), "Line...", false, "Type a whole line and stamp it ("+keyName("logo_line")+")") != nil {
		openTDFLine()
	}
	return y + smallH
}

// drawOutlineSample draws an outline style's corners and beams (top and bottom) in a
// row of cells, in the page's font and the brush's colours.
func (a *App) drawOutlineSample(fb *FB, px, py, style int) {
	for x, c := range "EAFKBL" {
		v, ok := a.tdfPageCell(tdfCell{kind: tdfInk, ch: tdfOutlines[style][c-'A']})
		if !ok {
			return
		}
		a.drawCell(fb, px+x*a.cellW(), py, 1, v)
	}
}

// ---------------------------------------------------------------- the font picker

// TDFReq lists the fonts with a preview of each; picking one makes it the Logo tool's.
type TDFReq struct {
	find, sample Field
	list         []*tdfFont // the fonts the search finds
	listFor      string     // the search the list is for
	sel, top     int
	thumbs       map[string]*FB
	back         Dialog // the requester to go back to (the line requester), nil: none
	all          bool   // an Amiga page lists the fonts it cannot show too
	lastClk      time.Time
}

func openTDFPicker(back Dialog) {
	tdfLoad()
	d := &TDFReq{back: back, thumbs: map[string]*FB{}, listFor: "\x00"}
	d.find.max, d.sample.max = 60, 60
	d.sample.set("Logo")
	if t := a.tdfPrefs().Text; t != "" {
		d.sample.set(t)
	}
	d.find.enter, d.sample.enter = d.use, d.use
	d.filter()
	if f := tdfNow(); f != nil {
		d.sel = max(0, slices.Index(d.list, f))
		d.top = max(0, d.sel-2)
	}
	a.dialog = d
	ui.focus = &d.find
}

// filter makes the list of fonts whose name or file holds the search.
func (d *TDFReq) filter() {
	q := strings.ToLower(strings.TrimSpace(d.find.String()))
	if d.all {
		q += "\x00all" // the list is made again when the box changes
	}
	if q == d.listFor {
		return
	}
	var cur *tdfFont
	if d.sel >= 0 && d.sel < len(d.list) {
		cur = d.list[d.sel]
	}
	d.listFor, d.list = q, nil
	q = strings.TrimSuffix(q, "\x00all")
	for _, f := range tdfLib.fonts {
		if !d.all && !a.tdfFits(f) {
			continue
		}
		if q == "" || strings.Contains(strings.ToLower(f.name), q) || strings.Contains(strings.ToLower(filepath.Base(f.file)), q) ||
			strings.ToLower(tdfKindNames[f.kind]) == q {
			d.list = append(d.list, f)
		}
	}
	d.sel = max(0, slices.Index(d.list, cur))
	d.top = 0
}

func (d *TDFReq) use() {
	if d.sel < 0 || d.sel >= len(d.list) {
		return
	}
	f := d.list[d.sel]
	tdfChoose(f)
	if s := strings.TrimSpace(d.sample.String()); s != "" && s != "Logo" {
		a.tdfPrefs().Text = s
	}
	closeDialog()
	if d.back != nil {
		a.dialog = d.back
	} else if a.tdfPage() {
		a.start = false
		a.setTool("logo")
		a.logoTool()
	}
	if !a.tdfFits(f) {
		a.msg(f.name + " uses CP437 characters an Amiga page lacks: letters that need them are not typed")
	}
}

func (d *TDFReq) cancel() {
	closeDialog()
	if d.back != nil {
		a.dialog = d.back
	}
}

// thumb is the font's preview: the sample text in the font, as the page shows it.
func (d *TDFReq) thumb(f *tdfFont, text string) *FB {
	key := fmt.Sprintf("%s|%s|%d|%d|%d|%d|%s", f.sum, a.mode, tdfStyle(), tdfSpacing(), a.fg, a.bg, text)
	if fb, ok := d.thumbs[key]; ok {
		return fb
	}
	if len(d.thumbs) > 400 {
		clear(d.thumbs)
	}
	l := f.layout(text, tdfStyle(), tdfSpacing())
	fb := a.renderDoc(a.tdfDoc(l.pic), 1)
	d.thumbs[key] = fb
	return fb
}

func (d *TDFReq) sampleText(f *tdfFont) string {
	s := strings.TrimSpace(d.sample.String())
	if s == "" {
		s = f.name
	}
	return s
}

func (d *TDFReq) rowH() int { return 3*lineH + 10 }

func (d *TDFReq) draw(u *UI) {
	fb := u.fb
	d.filter()
	in := u.window(centre(900, ui.fb.h*5/6), "TheDraw fonts")
	y := in.y
	// The search and the sample text.
	fw := (in.w - 40) / 2
	fb.text(in.x, y+3, "Find", cText)
	u.field(Rect{in.x + colAfter(40, "Find"), y, fw - colAfter(40, "Find"), gadH}, &d.find, "Part of a font's name or file (or outline, block, colour)")
	sx := in.x + fw + 20
	fb.text(sx, y+3, "Text", cText)
	u.field(Rect{sx + colAfter(40, "Text"), y, in.x + in.w - sx - colAfter(40, "Text"), gadH}, &d.sample, "What the previews show")
	y += gadH + 8

	count := fmt.Sprintf("%d fonts", len(d.list))
	if len(d.list) != len(tdfLib.fonts) {
		count = fmt.Sprintf("%d of %d fonts", len(d.list), len(tdfLib.fonts))
	}
	fb.text(in.x, y, count, cDim)
	if a.amiga() { // most fonts draw with block and line characters Latin-1 lacks
		if on, _ := u.check(in.x+in.w-checkW("Also fonts an Amiga page cannot show"), y-2, "Also fonts an Amiga page cannot show", d.all,
			"Most TheDraw fonts draw with CP437's blocks and lines; an Amiga page (Latin-1) has none of them"); on != d.all {
			d.all = on
		}
		y += 6
	}
	y += lineH + 4

	// The list: name, kind and file at the left, the preview at the right.
	big := min(200, (in.h-3*gadH)/3)
	listR := Rect{in.x, y, in.w - 18, in.y + in.h - gadH - 16 - big - y}
	fb.fill(listR, cDark)
	fb.bevel(listR, true)
	rh := d.rowH()
	rows := max(1, (listR.h-4)/rh)
	if w := u.takeWheel(listR); w != 0 {
		d.top += w
	}
	d.top = u.scrollbar("tdflist", Rect{listR.x + listR.w, listR.y, 18, listR.h}, true, d.top, rows, len(d.list))
	old := fb.clip
	fb.clip = listR.inset(2).and(old)
	if len(tdfLib.fonts) == 0 {
		ty := listR.y + 10
		for _, l := range wrap("No TheDraw fonts yet. Download Roy/SAC's collection (over 3,500 fonts), or add a folder or file of your own .tdf fonts (a .zip of them works too).", listR.w-20) {
			fb.text(listR.x+10, ty, l, cText)
			ty += lineH
		}
	}
	nameW := min(220, listR.w/3)
	for i := d.top; i < min(len(d.list), d.top+rows); i++ {
		f := d.list[i]
		r := Rect{listR.x + 2, listR.y + 2 + (i-d.top)*rh, listR.w - 4, rh}
		if i == d.sel {
			fb.fill(r, cBlue)
		}
		col, note := uint32(cText), tdfKindNames[f.kind]
		if !a.tdfFits(f) {
			col, note = cDim, note+", not for Amiga pages"
		}
		fb.text(r.x+6, r.y+4, fitString(f.name, nameW-10), col)
		fb.text(r.x+6, r.y+4+lineH, fitString(note, nameW-10), cDim)
		fb.text(r.x+6, r.y+4+2*lineH, fitString(filepath.Base(f.file), nameW-10), cDim)
		blitScaled(fb, Rect{r.x + nameW, r.y + 3, r.w - nameW - 6, r.h - 6}, d.thumb(f, d.sampleText(f)))
		if c := u.take(r); c != nil {
			double := i == d.sel && time.Since(d.lastClk) < 400*time.Millisecond
			d.sel, d.lastClk = i, time.Now()
			if double {
				fb.clip = old
				d.use()
				return
			}
		}
	}
	fb.clip = old

	// The chosen font larger, at the page's own size if it fits.
	pr := Rect{in.x, listR.y + listR.h + 8, in.w, big}
	fb.fill(pr, cDark)
	if d.sel >= 0 && d.sel < len(d.list) {
		f := d.list[d.sel]
		blitScaled(fb, pr.inset(4), d.thumb(f, d.sampleText(f)))
	}
	switch buttonRow(u, in, "Add fonts...", "Download...", "Cancel", "Use") {
	case 0:
		d.addFolder()
	case 1:
		openTDFDownload(func() { openTDFPicker(d.back) })
	case 2:
		d.cancel()
	case 3:
		d.use()
	}
}

// fitString cuts s to fit w pixels.
func fitString(s string, w int) string {
	r := []rune(s)
	return string(r[:fitText(r, w)])
}

// addFolder adds a folder or a file of fonts, chosen in a requester.
func (d *TDFReq) addFolder() {
	fr := &FileReq{sel: -1, title: "Add TheDraw fonts", note: "A .tdf or .zip file, or open a folder and Choose", pickDir: true, back: d}
	fr.listFirst(startDir(a.lastDir))
	fr.path.enter = fr.enter
	fr.pick = func(p string) {
		n := addTDFPath(p)
		a.msg(fmt.Sprintf("%d new fonts from %s", n, p))
		d.listFor = "\x00"
		clear(d.thumbs)
		a.dialog = d
		ui.focus = &d.find
	}
	a.dialog = fr
	ui.focus = &fr.path
}

func (d *TDFReq) key(k string, ctrl, shift bool) bool {
	rows := max(1, (ui.fb.h*5/6-6*gadH-200)/d.rowH())
	move := func(n int) {
		if len(d.list) == 0 {
			return
		}
		d.sel = max(0, min(len(d.list)-1, d.sel+n))
		if d.sel < d.top {
			d.top = d.sel
		} else if d.sel >= d.top+rows {
			d.top = d.sel - rows + 1
		}
	}
	switch k {
	case "Up":
		move(-1)
	case "Down":
		move(1)
	case "PageUp":
		move(-rows)
	case "PageDown":
		move(rows)
	case "Escape":
		d.cancel()
	case "Return", "Keypad Enter":
		d.use()
	case "Tab":
		if ui.focus == &d.find {
			ui.focus = &d.sample
		} else {
			ui.focus = &d.find
		}
	default:
		return false
	}
	return true
}

// ---------------------------------------------------------------- the line requester

// TDFLineReq lays out a line of text in the font; Stamp hands it over as a block
// that a click puts down (Shift: without blanks), like a paste.
type TDFLineReq struct {
	text Field
}

func openTDFLine() {
	if !a.tdfPage() {
		a.msg("TheDraw fonts are CP437 art: they work on PC, Amiga and Unicode pages")
		return
	}
	tdfLoad()
	if len(tdfLib.fonts) == 0 {
		openTDFDownload(openTDFLine)
		return
	}
	d := &TDFLineReq{}
	d.text.max = 200
	d.text.set(a.tdfPrefs().Text)
	if d.text.String() == "" {
		d.text.set("Charcoal")
	}
	d.text.enter = d.stamp
	a.dialog = d
	ui.focus = &d.text
}

func (d *TDFLineReq) line() (tdfLine, *tdfFont) {
	f := tdfNow()
	if f == nil {
		return tdfLine{pic: newTDFPic(1, 1)}, nil
	}
	return f.layout(d.text.String(), tdfStyle(), tdfSpacing()), f
}

func (d *TDFLineReq) stamp() {
	l, f := d.line()
	if f == nil || strings.TrimSpace(d.text.String()) == "" {
		return
	}
	if m := a.tdfMissing(l.pic); m != 0 {
		a.msg(fmt.Sprintf("%q uses %q, which an Amiga page (Latin-1) does not have", f.name, m))
		return
	}
	a.tdfPrefs().Text = d.text.String()
	savePrefs()
	closeDialog()
	a.startPaste(a.tdfBlock(l.pic))
	a.msg(fmt.Sprintf("%dx%d in %s: click to stamp it (Shift: without blanks), Esc to stop", l.pic.w, l.pic.h, f.name))
}

func (d *TDFLineReq) draw(u *UI) {
	fb := u.fb
	in := u.window(centre(900, 400), "Type a line in a TheDraw font")
	y := in.y
	l, f := d.line()
	label := "Choose a font..."
	if f != nil {
		label = f.name + " (" + tdfKindNames[f.kind] + ")"
	}
	fb.text(in.x, y+3, "Font", cText)
	if u.button(Rect{in.x + colAfter(48, "Font", "Text"), y, buttonW(label), gadH}, label, false, "Another font: the list ("+keyName("logo_font")+")") != nil {
		openTDFPicker(d)
		return
	}
	y += gadH + 8
	fb.text(in.x, y+3, "Text", cText)
	u.field(Rect{in.x + colAfter(48, "Font", "Text"), y, in.w - colAfter(48, "Font", "Text"), gadH}, &d.text, "The line; Return stamps it")
	y += gadH + 8
	note := fmt.Sprintf("%d x %d cells", l.pic.w, l.pic.h)
	if l.pic.w > a.doc.cols {
		note += fmt.Sprintf(", wider than the page (%d)", a.doc.cols)
	}
	if len(l.missing) > 0 {
		note += ". Not in the font: " + string(l.missing)
	}
	if m := a.tdfMissing(l.pic); m != 0 {
		note += fmt.Sprintf(". An Amiga page has no %q", m)
	}
	fb.text(in.x, y, note, cDim)
	y += lineH + 6
	pr := Rect{in.x, y, in.w, in.y + in.h - gadH - 10 - y}
	fb.fill(pr, cDark)
	if f != nil {
		blitScaled(fb, pr.inset(4), a.renderDoc(a.tdfDoc(l.pic), 1))
	}
	switch buttonRow(u, in, "Cancel", "Stamp") {
	case 0:
		closeDialog()
	case 1:
		d.stamp()
	}
}

func (d *TDFLineReq) key(k string, ctrl, shift bool) bool {
	switch k {
	case "Escape":
		closeDialog()
	case "Return", "Keypad Enter":
		d.stamp()
	default:
		return false
	}
	return true
}

func init() {
	tools = append(tools, struct{ id, name, hint string }{"logo", "Logo",
		"Logo: type in a TheDraw font at the cursor (Backspace takes a letter back, Return starts the next line of letters)"})
}
