package main

// The editor's state and everything it does to the page.

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

type Palette struct {
	id, name string
	colors   [16]uint32
}

var bright = []uint32{0x555555, 0xff5555, 0x55ff55, 0xffff55, 0x5555ff, 0xff55ff, 0x55ffff, 0xffffff}

func pal(id, name string, c ...uint32) *Palette {
	p := &Palette{id: id, name: name}
	c = append(c, bright...)
	for i := range p.colors {
		p.colors[i] = 0xFF000000 | c[i]
	}
	return p
}

var palettes = []*Palette{
	pal("amiga", "Amiga ANSI", 0x000000, 0xaa0000, 0x00aa00, 0xaaaa00, 0x0000aa, 0xaa00aa, 0x00aaaa, 0xaaaaaa),
	pal("vga", "PC VGA", 0x000000, 0xaa0000, 0x00aa00, 0xaa5500, 0x0000aa, 0xaa00aa, 0x00aaaa, 0xaaaaaa),
	pal("wb", "Workbench 2/3", 0x000000, 0xbb0000, 0x00bb00, 0xeeaa00, 0x6688bb, 0xbb66bb, 0x55aaaa, 0xaaaaaa,
		0x666666, 0xff6666, 0x66ff66, 0xffee55, 0x88aaff, 0xff88ff, 0x88ffff, 0xffffff),
	c64Palette,
	atariPalette,
	zxPalette,
	teletextPalette,
	{id: "custom", name: "Custom"},
}

func init() { // the other Commodore palettes go after the C64's, the ST's after the Atari's
	var out []*Palette
	for _, p := range palettes {
		out = append(out, p)
		if p.id == "c64" {
			out = append(out, cbmPalettes...)
		}
		if p.id == "atari" {
			out = append(out, stPalettes...)
		}
	}
	palettes = out
}

func paletteByID(id string) *Palette {
	for _, p := range palettes {
		if p.id == id {
			return p
		}
	}
	return palettes[0]
}

// The F-key sets (bytes). Amiga sets are Latin-1, PC sets CP437.
func defaultFkeys() map[string][][]int {
	return map[string][][]int{
		"amiga": {
			{0x5F, 0xAF, 0x2F, 0x5C, 0x7C, 0x3A, 0x2E, 0xB8, 0xB7, 0x27},
			{0xB4, 0x60, 0xA8, 0xB0, 0xA6, 0x2D, 0x2C, 0x3B, 0x28, 0x29},
			{0xAB, 0xBB, 0x3C, 0x3E, 0x3D, 0x2B, 0x2A, 0x5E, 0x7E, 0x22},
			{0xD7, 0xF7, 0xA4, 0xF8, 0xD8, 0xDF, 0xB5, 0xB6, 0xA7, 0xA9},
			{0x5B, 0x5D, 0x7B, 0x7D, 0x23, 0x40, 0x24, 0x25, 0x26, 0x21},
		},
		"pc": {
			{0xB0, 0xB1, 0xB2, 0xDB, 0xDF, 0xDC, 0xDD, 0xDE, 0xFE, 0xFA},
			{0xDA, 0xBF, 0xC0, 0xD9, 0xC4, 0xB3, 0xC3, 0xB4, 0xC2, 0xC1},
			{0xC9, 0xBB, 0xC8, 0xBC, 0xCD, 0xBA, 0xCC, 0xB9, 0xCB, 0xCA},
			{0xC5, 0xCE, 0xD5, 0xB8, 0xD4, 0xBE, 0xD6, 0xB7, 0xD3, 0xBD},
			{0xF9, 0xF8, 0xFD, 0xF7, 0xFB, 0xEF, 0xA9, 0xAA, 0x10, 0x11},
		},
		"c64":   c64Fkeys,
		"atari": atariFkeys,
		"st":    stFkeys,
		"zx":    zxFkeys,
		"tt":    ttFkeys,
		"uni":   uniFkeys,
		"cpc":   cpcFkeys,
		"vd":    ttFkeys,
		"mt":    mtFkeys,
		"msx":   msxFkeys,
		"mz":    mzFkeys,
	}
}

var bauds = []int{1200, 2400, 9600, 14400, 33600, 57600, 115200, telnet} // telnet: no speed limit, only a board's pauses

type Info struct {
	title, author, group string
	writeSauce, clear    bool
	notes                []string // SAUCE comment lines, kept from the file
	utf8                 bool     // the file's record was in UTF-8 (Moebius): written back so
	font                 string   // the file's SAUCE font name when CharEdit has no such font
	fontID               string   // the font shown for it: while it is, the name is written back
}

type Sel struct{ x0, y0, x1, y1 int }
type Hover struct {
	x, y, half int
	sx, sy     int // the teletext sixel under the mouse: column 0-1, row 0-2
}
type Snap struct {
	doc    *Doc
	cx, cy int
	gen    int     // the page's generation (see nextGen)
	cur    int     // the frame it belongs to
	whole  bool    // a step of the frame list (new, delete, move ...): frames is the list
	frames []Frame // with whole: the list as it was (nil: a page of one picture)
	cost   int     // the cells the step keeps, for undoBudget
	order  *drawOrder
	mode   string    // the page type: undoing a conversion to or from Unicode changes it back
	cpc    *cpcSetup // a CPC page's inks and mode as they were (nil on other pages)
	mach   string    // a PETSCII page's machine: undoing a move to or from the Plus/4 brings its colours back
	vic    *vicSetup // a C64 page's text mode and $D022-$D024 as they were (nil on other pages)
	step   int       // the page's depth in this state (work stages: stages.go)
	msx    *msxSetup // an MSX page's screen mode and colours as they were (nil on other pages)
	lay    int       // the layer it belongs to (layers.go)
	lwhole bool      // a step of the layer list (new, delete, merge, show ...): layers is the list
	layers []Layer   // with lwhole: the list as it was (nil: a page of one layer)
}
type Play struct {
	bytes []byte
	pos   int
	term  emulator
	saved Snap
	last  time.Time
	// Playing the frames: each shown for its hold time.
	frames []*Doc
	holds  []int // ms
	lowers []int // PETSCII case of each, -1 not said
	fi     int
	next   time.Time // when the next frame comes
	// A board's pauses in the stream (Animate's board): waited for at any speed.
	pauses []pause
	pi     int
	wait   time.Time
}

// Tab is one open page with everything that belongs to it.
type Tab struct {
	doc                 *Doc
	mode                string // "amiga", "pc", "c64" (PETSCII), "atari", "st", "zx" or "tt" (teletext)
	fontFor, paletteFor map[string]string
	zoom                int
	ch8, smear, ice     bool
	cx, cy              int
	path, name          string
	dirty               bool
	source              []byte // the file as read (or last saved), to read again with other rules
	sourceBin           bool
	sourceUTF8          bool   // source was decoded from a UTF-8 file
	codes               string // the source's BBS colour codes (bbscodes.go: "pipe", "ctrla" ...), "" none
	sauce               *Sauce
	sauceFlags          int  // the file's SAUCE letter spacing and aspect ratio bits (TFlags 1-4), written back
	rawCtl              bool // the file (with SAUCE) held ○ ◘ • or character 0 as glyphs: saved so again
	saveFmt             string
	forceCols           int
	info                Info
	sel                 *Sel
	selecting           bool
	float               *Block
	floatDX, floatDY    int  // where in the floating block the mouse holds it
	moving              bool // the floating block was lifted off the page by a drag (Select tool)
	undo, redo          []Snap
	gen, savedGen       int  // the page's generation now and when it was last read or saved
	inRun               bool // typing goes on in one undo step
	runX, runY          int  // where the run of typing left the cursor
	hover, last         *Hover
	playing             *Play
	scrollX, scrollY    int
	page                Rect // where the page view was laid out
	pal                 [16]uint32
	srcKind             string      // "xbin", "marq", "petmate" or "prg" for files that are not a stream of codes; "tti" for a teletext .tti
	ownFont             *Font       // the page's own font: from its file (XBin, Petmate), a font file or the font editor
	ownLower            *Font       // a PETSCII page's own lower case set
	ownPal              *[16]uint32 // a palette that came in the file (XBin)
	machine             string      // the Commodore machine of a PETSCII page
	stres               string      // the resolution of an Atari ST page
	autoID              string      // its autosave's name, "" when it has none
	autoGen             int         // the generation the autosave holds
	start               bool        // the start screen is shown instead of the (empty) page
	frames              []Frame     // the page's pictures when it has several (nil: one, the doc); see frames.go
	noLoop              bool        // playing the frames stops after the last one
	cur                 int         // the frame being worked on: doc is it
	restructured        bool        // frames were added, deleted or moved since the file was read or saved
	onion               bool        // the frame before shows faint under the empty cells
	strip               int         // the frame strip: 0 while the page has several frames, 1 on, -1 off
	split               splitInfo   // the pictures the page's file (a stream) shows, offered as frames
	ref                 *RefImage   // the reference picture to trace over (refimage.go), nil: none
	order               *drawOrder  // the order Save draws a stream file in (transitions.go); nil: row by row
	cpcSet              cpcSetup    // an Amstrad CPC page's mode, inks and border (cpc.go)
	msxSet              msxSetup    // an MSX page's screen mode and colours (msx.go)
	vic                 vicSetup    // a C64 page's text mode (multicolour, ECM) and $D022-$D024 (c64x.go)
	igs                 *igsLayer   // an IGS file's picture under the cells and what saving keeps (igs_page.go)
	sky                 *skyLayer   // a SkyPix file's picture under the cells (skypix.go)
	steps               int         // the page's depth: the steps that led to it (work stages: stages.go)
	stages              *stageLog   // the work's history, thinned, for the work stages (stages.go)
	layers              []Layer     // the page's layers, bottom first, when it has several (nil: one, the doc); see layers.go
	lay                 int         // the layer being worked on: doc is it
	others              int         // how the other layers are shown: 0 as they are, 1 dim, 2 hidden (view only)
	seeing              bool        // doc is the layers put together for a while (asSeen); the live layer is in layers[lay]
	layering            bool        // an edit runs on every layer (everyLayerDo): its own undo steps are not kept
}

// App is the editor: the tab being worked on (its fields read as a's own),
// the other tabs, and what all tabs share (tool, brush, colours, F-keys).
type App struct {
	*Tab
	tabs          []*Tab
	layout        string // "one", "cols" (side by side) or "rows" (top and bottom)
	shown         [2]*Tab
	syncScroll    bool
	grid          bool
	tool          string
	fg, bg, brush int
	ital, under   bool
	rev, insert   bool
	flash         bool   // ZX Spectrum: new cells flash
	mtAttr        uint32 // Minitel: flash, inverse, underline (separated) and concealing that new cells get (minitel_ui.go)
	mtSize        int    // Minitel: the size letters are typed in (0 normal, 1 double width, 2 double height, 3 double size)
	mtSizeShown   bool   // the note on big letters was shown
	showCodes     bool   // teletext: show the attribute codes' names in their cells
	ttMeta        int    // teletext: edit.tf's page digit (bits 0-2 character set, bit 3 codes 0 and 16 are black)
	playSpeed     int    // frames play at speeds[playSpeed] (0: 1x)
	clip          *Block
	clipText      string
	fset, factive int
	fkeys         map[string][][]int
	button        int
	blink         bool
	lastDir       string
	baud          int
	message       string
	messageAt     time.Time
	custom        [16]uint32
	dialog        Dialog
	charHover     int
	quit          bool
	bare          bool // gadgets hidden: only the page and the status line
	hasC64        bool
	defFont       map[string]string // the fonts and palettes a new tab starts with
	defPalette    map[string]string
	defZoom       int
	defMachine    string
	defSTRes      string
	ch8Default    bool
	smearDefault  bool
	startSel      int // the start screen's chosen item (keyboard)
	startCols     int // tiles in a row of the start screen, as last drawn
	startCol      int // the column Up goes back to from the recent files
	drawOpts          // shapes, shade, mirror, colour / character only (shapes.go)
	uniOpts           // the Unicode page's colours beyond 16 and character block (uni.go)
	c64xOpts          // the brush's background in the C64's ECM (c64x.go)
}

var a *App

func (a *App) amiga() bool { return a.mode == "amiga" }
func (a *App) pc() bool    { return a.mode == "pc" }
func (a *App) c64() bool   { return a.mode == "c64" }
func (a *App) atari() bool { return a.mode == "atari" }
func (a *App) st() bool    { return a.mode == "st" }
func (a *App) zx() bool    { return a.mode == "zx" }
func (a *App) tt() bool    { return a.mode == "tt" || a.mode == "vd" } // teletext, and Viewdata (viewdata.go)

// ttBlack: does the page draw codes 0 and 16 black (edit.tf's own choice)?
func (a *App) ttBlack() bool { return a.ttMeta&8 != 0 }

// cellW is a cell's width in pixels: 12 for teletext, 8 for the rest.
func (a *App) cellW() int {
	if a.tt() {
		return ttW
	}
	if a.mt() {
		return mtW
	}
	if a.pc() && a.ninePx() {
		return 9 // VGA's 9-dot cells (SAUCE letter spacing)
	}
	if a.cpc() {
		return a.cpcCellW()
	}
	if a.msx() {
		return a.msxCellW()
	}
	return 8
}

// res is the Atari ST resolution of an ST page.
func (a *App) res() STRes { return stResByID(a.stres) }

// screen: a machine's text screen (C64, Atari): 8x8 characters, one background
// colour for the whole screen, a border, inverse characters in the top half.
func (a *App) screen() bool { return a.c64() || a.atari() }

func (a *App) font() *Font {
	if f := a.ownInUse(); f != nil {
		return f
	}
	if a.st() {
		if f := fontByID(a.res().font); f != nil {
			return f
		}
	}
	if a.zx() {
		return zxFont
	}
	if a.cpc() {
		return cpcFont
	}
	if a.tt() {
		return ttFont
	}
	if a.mt() {
		return mtFont
	}
	if a.c64() {
		fam := a.mach().family
		if f := fontByID(a.fontFor["c64"]); f != nil && strings.HasPrefix(f.id, fam) {
			return f
		}
		if f := fontByID(fam + "upper"); f != nil {
			return f
		}
	}
	if f := fontByID(a.fontFor[a.mode]); f != nil && f.kind == a.mode {
		return f
	}
	for _, f := range fonts {
		if f.kind == a.mode && f.id != "topaz1" {
			return f
		}
	}
	return fonts[1]
}

// lower: a C64 page in the lower + upper case set.
func (a *App) lower() bool { return strings.HasSuffix(a.font().id, "lower") }

// mach is the Commodore machine of a PETSCII page.
func (a *App) mach() Machine { return machineByID(a.machine) }

// setCase picks the upper or lower case set of the page's machine.
func (a *App) setCase(lower bool) {
	if strings.HasPrefix(a.fontFor["c64"], ownID) && a.ownFont != nil && (!lower || a.ownLower != nil) {
		a.fontFor["c64"] = map[bool]string{true: ownLowerID, false: ownID}[lower]
		return
	}
	a.fontFor["c64"] = a.mach().family + map[bool]string{true: "lower", false: "upper"}[lower]
}

func (a *App) cellH() int {
	if a.st() {
		return map[bool]int{true: 16, false: 8}[a.res().font == "st8x16"]
	}
	if a.zx() {
		return 8
	}
	if a.cpc() {
		return a.cpcCellH()
	}
	if a.msx() {
		return a.msxCellH()
	}
	if a.mz() {
		return 8
	}
	if a.tt() {
		return ttH
	}
	if a.mt() {
		return mtH
	}
	if f := a.font(); (a.pc() || a.amiga()) && f.h > 0 {
		return f.h // a font of 8 or 14 rows
	}
	if a.screen() || (a.ch8 && a.amiga()) {
		return 8
	}
	return 16
}

func (a *App) styleFlags() uint32 {
	if !a.amiga() {
		return 0
	}
	var f uint32
	if a.ital {
		f |= italic
	}
	if a.under {
		f |= underline
	}
	return f
}

// cell makes a cell of the page in the brush's colours.
func (a *App) cell(ch int) uint32 {
	if a.uni() {
		return a.uniCellOf(ch)
	}
	if a.tt() {
		return makeCell(ch&0x7F, 0, 0, 0)
	}
	if a.mt() {
		return mtCellOf(ch, a.fg, a.bg, a.mtAttr)
	}
	if a.msx() { // the screen's colours (msx.go)
		return makeCell(ch, 0, 0, 0)
	}
	if a.mz() { // 256 and up: the second set (mz.go)
		return mzCell(ch, a.fg, a.bg)
	}
	if a.zx() {
		return zxCell(ch, a.fg, a.bg, a.fg > 7 || a.bg > 7, a.flash)
	}
	if a.screen() {
		if a.vicECM() { // the code's top bits are the background (c64x.go)
			return makeCell(a.ecmChar(ch), a.fg, 0, 0)
		}
		if a.rev {
			ch |= 0x80
		}
		if a.atari() {
			return makeCell(ch, 0, 0, 0) // one text colour for the whole screen
		}
		if a.ted() { // the brush's whole TED colour and flash (ted.go)
			return tedCell(ch, a.tedBrush())
		}
		return makeCell(ch, a.fg, 0, 0)
	}
	return makeCell(ch, a.fg, a.bg, a.styleFlags())
}

func (a *App) brushCell() uint32 {
	if a.uni() {
		return a.uniCellOf(a.brush)
	}
	if a.tt() {
		return makeCell(a.brush&0x7F, 0, 0, 0)
	}
	if a.zx() || a.mt() || a.msx() || a.mz() {
		return a.cell(a.brush)
	}
	if a.atari() {
		return makeCell(a.brush, 0, 0, 0)
	}
	if a.ted() {
		return tedCell(a.brush, a.tedBrush())
	}
	if a.c64() {
		return makeCell(a.ecmChar(a.brush), a.fg, 0, 0)
	}
	return makeCell(a.brush, a.fg, a.bg, a.styleFlags())
}

// previewFg is a colour that shows on the page's background, for the character table.
func (a *App) previewFg() int {
	if a.st() {
		return a.res().fg
	}
	if a.atari() {
		return 0 // the screen's text colour (an Atari cell has none)
	}
	if a.cpc() {
		return 1
	}
	if !a.c64() {
		return 7
	}
	// Black or white, whichever shows better on the screen colour (by its brightness).
	c := a.screenRGB(a.doc.bg)
	r, g, b := float64(c>>16&255), float64(c>>8&255), float64(c&255)
	if 0.299*r+0.587*g+0.114*b > 110 {
		return 0
	}
	return 1
}

func (a *App) setPalette() {
	if a.ted() { // the KERNAL's 16 colours of the page's TED palette (ted.go)
		a.pal = a.tedPal().kernal16()
		return
	}
	if a.cpc() { // the page's inks
		a.cpcEnsure()
		a.pal = a.cpcSet.palette()
		return
	}
	if a.skyPage() { // the SkyPix file's colours (skypix.go)
		a.pal = a.sky.pal
		return
	}
	if a.ownPal != nil && a.paletteFor[a.mode] == "file" {
		a.pal = *a.ownPal
		return
	}
	if a.igsPage() { // the IGS file's colour registers
		a.pal = a.igs.pal
		return
	}
	p := paletteByID(a.paletteFor[a.mode])
	if a.c64() && !strings.Contains(" c64 colodore vic20 vdc petgreen petamber petwhite custom ", " "+p.id+" ") {
		p = paletteByID(a.mach().palette)
	}
	if a.atari() && !isGTIAPalette(p) { // a GTIA palette (gtia.go)
		p = atariPalette
	}
	if a.zx() && p.id != "custom" {
		p = zxPalette
	}
	if a.tt() && p.id != "custom" {
		p = teletextPalette
	}
	if a.mt() && p != minitelPalette && p.id != "custom" {
		p = minitelGreyPalette
	}
	if a.msx() && p != msx1Palette && p != msx2Palette && p.id != "custom" {
		p = msx1Palette
	}
	if a.mz() && p.id != "custom" {
		p = mzPalette
	}
	if a.st() && p.id != "custom" { // the ST's colours are the resolution's
		p = paletteByID(a.res().palette)
	}
	if p.id == "custom" {
		a.pal = a.custom
	} else {
		a.pal = p.colors
	}
}

// printable: is this byte a glyph on the current page?
func (a *App) printable(c int) bool {
	if a.uni() {
		return c >= 0x20 && c != 0x7F && c < 0x10000
	}
	if a.tt() {
		return c < 0x80
	}
	if a.mt() {
		return mtPrintable(c)
	}
	if a.zx() {
		return c >= 32 && c < 165
	}
	if a.screen() || a.cpc() || a.msx() {
		return true
	}
	if a.mz() {
		return c >= 0 && c < 512
	}
	if a.amiga() {
		return !(c < 0x20 || (c >= 0x7F && c < 0xA0))
	}
	return c != 0
}

func (a *App) charToByte(r rune) int {
	if a.uni() {
		return uniFromRune(r)
	}
	if a.tt() {
		return ttFromRune(r)
	}
	if a.mt() {
		return mtFromRune(r)
	}
	if a.zx() {
		return zxFromRune(r)
	}
	if a.cpc() {
		return cpcFromRune(r)
	}
	if a.msx() {
		return msxFromRune(r)
	}
	if a.mz() {
		return mzFromRune(r)
	}
	if a.c64() {
		return petFromRune(r, a.lower())
	}
	if a.atari() {
		return atariFromRune(r)
	}
	if a.amiga() {
		if r <= 0xFF && a.printable(int(r)) {
			return int(r)
		}
		return -1
	}
	if r >= 0x20 && r < 0x7F {
		return int(r)
	}
	if b, ok := cp437Rev[r]; ok {
		return int(b)
	}
	return -1
}

// runeOf is the character a byte of the page shows, as Unicode.
func (a *App) runeOf(c int) rune {
	if a.uni() {
		return uniRune(c)
	}
	if a.c64() {
		return petRune(c, a.lower())
	}
	if a.atari() {
		return atariRune(c)
	}
	if a.zx() {
		return zxRune(c)
	}
	if a.cpc() {
		return cpcRune(c)
	}
	if a.msx() {
		return msxRune(c)
	}
	if a.mz() {
		return mzRune(c)
	}
	if a.tt() {
		return ttRune(c)
	}
	if a.mt() {
		return mtRune(c)
	}
	return byteRune(c, a.amiga())
}

func (a *App) msg(s string) {
	a.message = s
	a.messageAt = time.Now()
}

// ---------------------------------------------------------------- undo

// Every state of a page has a generation number; the one it was saved in is
// remembered, so undoing back to it makes the page unchanged again.
var genSeq int

func nextGen() int { genSeq++; return genSeq }

func (a *App) snapshot() Snap {
	d := a.doc.clone()
	return Snap{doc: d, cx: a.cx, cy: a.cy, gen: a.gen, cur: a.cur, cost: len(d.cells), order: a.order, mode: a.mode, cpc: a.cpcSnap(), vic: a.vicSnap(), mach: a.machine, step: a.steps, msx: a.msxSnap(), lay: a.lay}
}

// undoBudget caps the cells all undo steps of a page may keep (64 MB): tall art
// keeps fewer steps instead of eating the memory.
const undoBudget = 16 << 20

func (a *App) pushUndo() {
	if a.layering { // one step for every layer is kept already (layers.go)
		return
	}
	a.pushSnap(a.snapshot())
}

// pushSnap puts an undo step on the stack, letting go of the oldest ones past
// 300 steps or undoBudget cells.
func (a *App) pushSnap(s Snap) {
	a.Tab.recordStage(s) // the work stages' history (stages.go)
	a.steps = s.step + 1
	a.undo = append(a.undo, s)
	total := 0
	for _, s := range a.undo {
		total += s.cost
	}
	drop := 0
	for len(a.undo)-drop > 1 && (len(a.undo)-drop > 300 || total > undoBudget) {
		total -= a.undo[drop].cost
		drop++
	}
	if drop > 0 { // move the rest down and let go of the dropped pages
		n := copy(a.undo, a.undo[drop:])
		clear(a.undo[n:])
		a.undo = a.undo[:n]
	}
	clear(a.redo)
	a.redo = a.redo[:0]
	a.gen = nextGen()
	a.inRun = false
	a.dirty = true
}

// markSaved: the page as it is now is what the file holds.
func (a *App) markSaved() {
	a.dirty = false
	a.savedGen = a.gen
	a.inRun = false
}

func (a *App) restore(s Snap) {
	switch {
	case s.whole:
		a.frames, a.cur = slices.Clone(s.frames), s.cur
		a.sel, a.float = nil, nil
	case s.lwhole: // the layer list as it was (layers.go)
		a.layers, a.lay = slices.Clone(s.layers), s.lay
		a.float = nil
	case a.layers != nil && s.lay != a.lay && s.lay < len(a.layers):
		// An edit of another layer: undoing it goes there, to show what was undone.
		a.leaveLayer()
		a.lay = s.lay
		a.float = nil
	case a.frames != nil && s.cur != a.cur && s.cur < len(a.frames):
		// An edit of another frame: undoing it goes there, to show what was undone.
		a.keepCase()
		a.frames[a.cur].doc = a.doc
		a.cur = s.cur
		a.sel, a.float = nil, nil
	}
	a.doc, a.cx, a.cy, a.gen, a.order = s.doc, s.cx, s.cy, s.gen, s.order
	a.steps = s.step
	a.restoreUniMode(s.mode)
	if s.cpc != nil && a.cpc() {
		a.cpcSet = *s.cpc
		a.setPalette()
	}
	if s.vic != nil && a.c64() {
		a.vic = *s.vic
	}
	if s.msx != nil && a.msx() {
		a.msxSet = *s.msx
		a.msxBrushColours()
	}
	if a.c64() && s.mach != "" && s.mach != a.machine && hasROM(machineByID(s.mach).family) {
		lower := a.lower()
		a.machine = s.mach
		a.setCase(lower)
		a.paletteFor["c64"] = a.mach().palette
		a.setPalette()
	}
	a.showCase()
	a.dirty = a.gen != a.savedGen
	a.inRun = false
}

func (a *App) doUndo() {
	if len(a.undo) == 0 {
		a.msg("Nothing to undo")
		return
	}
	a.redo = append(a.redo, a.snapshotLike(a.undo[len(a.undo)-1]))
	a.restore(a.undo[len(a.undo)-1])
	a.undo[len(a.undo)-1] = Snap{}
	a.undo = a.undo[:len(a.undo)-1]
}
func (a *App) doRedo() {
	if len(a.redo) == 0 {
		a.msg("Nothing to redo")
		return
	}
	a.undo = append(a.undo, a.snapshotLike(a.redo[len(a.redo)-1]))
	a.restore(a.redo[len(a.redo)-1])
	a.redo[len(a.redo)-1] = Snap{}
	a.redo = a.redo[:len(a.redo)-1]
}

// ---------------------------------------------------------------- editing

// maxRows is the most rows the page may have: a Spectrum screen is 24 rows, a
// teletext page 25, every other page as many as the page size limit allows.
func (a *App) maxRows() int {
	switch {
	case a.zx(), a.vd(), a.msx():
		return 24
	case a.tt(), a.cpc(), a.mt(), a.mz():
		return 25
	case a.layers != nil: // all layers together under the largest page (layers.go)
		return min(maxRowsFor(a.doc.cols), a.layersMaxRows())
	}
	return maxRowsFor(a.doc.cols)
}

// setCell sets a cell; a cell below the last row adds rows, up to maxRows.
func (a *App) setCell(x, y int, v uint32) {
	if x < 0 || x >= a.doc.cols || y < 0 || y >= a.maxRows() {
		return
	}
	a.doc.set(x, y, v)
}

func (a *App) typeByte(b int) {
	// A run of typing is one undo step: the run goes on while the cursor is
	// where the last character left it.
	if !a.inRun || a.cx != a.runX || a.cy != a.runY {
		a.pushUndo()
	}
	a.dirty = true
	d := a.doc
	if a.cx >= d.cols {
		a.cx, a.cy = 0, a.cy+1
	}
	if a.insert && a.cy < d.rows {
		for x := d.cols - 1; x > a.cx; x-- {
			d.set(x, a.cy, d.get(x-1, a.cy))
		}
	}
	if a.mt() {
		a.mtType(b) // big letters take two or four cells (minitel_ui.go)
	} else {
		a.setCell(a.cx, a.cy, a.cell(b))
		a.cx++
	}
	if a.cx >= d.cols {
		a.cx, a.cy = 0, a.cy+1
	}
	a.moved()
	a.inRun, a.runX, a.runY = true, a.cx, a.cy
}

func (a *App) moved() {
	a.cx = max(0, min(a.doc.cols-1, a.cx))
	a.cy = max(0, min(a.maxRows()-1, a.cy))
	if a.cy >= a.doc.rows { // going past the last row adds rows to the page
		run := a.inRun
		a.pushUndo()
		a.inRun = run
		a.doc.ensureRows(a.cy + 1)
	}
	a.blink = true
	a.ensureVisible(a.cx, a.cy)
}

func (a *App) ensureVisible(x, y int) {
	H, W := a.cellH()*a.zoom, a.cellW()*a.zoom
	top, left := y*H, x*W
	if top < a.scrollY {
		a.scrollY = max(0, top-H)
	} else if top+H > a.scrollY+a.page.h {
		a.scrollY = top + 2*H - a.page.h
	}
	if left < a.scrollX {
		a.scrollX = max(0, left-W)
	} else if left+W > a.scrollX+a.page.w {
		a.scrollX = left + 2*W - a.page.w
	}
}

// deleteAt removes the cell and pulls the rest of the row left.
func (a *App) deleteAt(x, y int) {
	d := a.doc
	for i := x; i < d.cols-1; i++ {
		d.set(i, y, d.get(i+1, y))
	}
	d.set(d.cols-1, y, blank)
}

func line(x0, y0, x1, y1 int, fn func(x, y int)) {
	dx, dy := abs(x1-x0), -abs(y1-y0)
	sx, sy := 1, 1
	if x0 > x1 {
		sx = -1
	}
	if y0 > y1 {
		sy = -1
	}
	err := dx + dy
	for {
		fn(x0, y0)
		if x0 == x1 && y0 == y1 {
			return
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// paintAt paints with the Draw or Colour tool at (x, y) and, with the mirror
// option, at its mirror images.
func (a *App) paintAt(x, y, button int) {
	for _, s := range symPoints(x, y, a.doc.cols, a.doc.rows, a.mirror) {
		a.paintOne(s.x, s.y, button, s.fx, s.fy)
	}
}

func (a *App) paintOne(x, y, button int, fx, fy bool) {
	old := a.doc.get(x, y)
	switch a.tool {
	case "draw":
		switch {
		case a.shade && a.shadeSteps() != nil:
			a.shadeAt(x, y, button != 3)
		case button == 3 && a.mt(): // the cleared screen's black mosaic space
			a.setCell(x, y, mtBlank)
		case button == 3:
			a.setCell(x, y, blank)
		default:
			a.putCell(x, y, a.mirrorCell(a.brushCell(), fx, fy))
		}
	case "colour":
		if button == 3 && a.vicECM() { // ECM: the background is the code's top bits (c64x.go)
			a.setCell(x, y, old&^0xC0|uint32(a.ecmBg)<<6)
			return
		}
		if (button == 3 && a.c64()) || a.atari() {
			return // a C64 cell has no background of its own, an Atari cell no colour at all
		}
		if a.ted() { // the whole attribute: colour and flash
			a.setCell(x, y, old&^0xFF00|uint32(a.tedBrush())<<8)
			return
		}
		if button == 3 {
			a.setCell(x, y, old&^0xF000|uint32(a.bg)<<12)
		} else {
			a.setCell(x, y, old&^0xF00|uint32(a.fg)<<8)
		}
	}
}

// paintSixel sets (on) or clears one of a teletext cell's 2x3 sixels; gx, gy count sixels.
func (a *App) paintSixel(gx, gy int, on bool) {
	if a.mt() {
		a.mtPaintSixel(gx, gy, on)
		return
	}
	x, y := gx/2, gy/3
	if x < 0 || y < 0 || x >= a.doc.cols || y >= a.doc.rows {
		return
	}
	bit := (gy%3)*2 + gx%2
	code := cellCh(a.doc.get(x, y)) & 0x7F
	bits := 0
	if code >= 0x20 && (code < 0x40 || code >= 0x60) {
		bits = code&0x1F | (code&0x40)>>1
	}
	if on {
		bits |= 1 << bit
	} else {
		bits &^= 1 << bit
	}
	a.doc.set(x, y, makeCell(0x20|bits&0x1F|(bits&0x20)<<1, 0, 0, 0))
	if row := ttDecode(a.doc, y, a.ttBlack()); !row[x].mosaic && bits != 0 {
		a.msg("This cell is in text: put a graphics colour (F-keys, set 2) to its left to see sixels")
	}
}

// paintHalf: a cell holds two pixels, top and bottom.
func (a *App) paintHalf(x, hy, colour int) {
	if a.uni() {
		a.uniPaintHalf(x, hy, colour)
		return
	}
	y, bottom := hy>>1, hy&1
	v := a.doc.get(x, y)
	ch, fg, bg := cellCh(v), cellFg(v), cellBg(v)
	var top, bot int
	switch ch {
	case 0xDB:
		top, bot = fg, fg
	case 0xDF:
		top, bot = fg, bg
	case 0xDC:
		top, bot = bg, fg
	default:
		top, bot = bg, bg
	}
	if bottom == 1 {
		bot = colour
	} else {
		top = colour
	}
	lim := 7
	if a.ice {
		lim = 15
	}
	var cell uint32
	switch {
	case top == bot && top <= lim:
		cell = makeCell(32, 7, top, 0)
	case top == bot:
		cell = makeCell(0xDB, top, 0, 0)
	case bot <= lim:
		cell = makeCell(0xDF, top, bot, 0)
	case top <= lim:
		cell = makeCell(0xDC, bot, top, 0)
	default:
		cell = makeCell(0xDF, top, bot&7, 0)
	}
	a.setCell(x, y, cell)
}

func (a *App) floodFill(x, y int) {
	d := a.doc
	target, want := d.get(x, y), a.brushCell()
	if target == want || y >= d.rows {
		return
	}
	stack := [][2]int{{x, y}}
	for len(stack) > 0 {
		p := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		px, py := p[0], p[1]
		if px < 0 || py < 0 || px >= d.cols || py >= d.rows || d.get(px, py) != target {
			continue
		}
		d.set(px, py, want)
		stack = append(stack, [2]int{px + 1, py}, [2]int{px - 1, py}, [2]int{px, py + 1}, [2]int{px, py - 1})
	}
}

func (a *App) pick(x, y int) {
	v := a.doc.get(x, y)
	if a.uni() {
		a.uniPick(v)
		return
	}
	if a.mt() {
		a.mtPick(v)
		return
	}
	if a.msx() {
		a.msxPick(v)
		return
	}
	a.brush = cellCh(v)
	if a.mz() {
		a.brush = mzChar(v)
	}
	if !a.atari() {
		a.fg = cellFg(v)
	}
	if a.ted() { // the whole TED colour, and whether it flashes
		a.fg, a.flash = tedAttr(v)&127, tedAttr(v)&128 != 0
	}
	if !a.screen() {
		a.bg = cellBg(v)
	}
	a.ital, a.under = v&italic != 0, v&underline != 0
}

func (a *App) setTool(t string) {
	if t == "half" && !a.pc() && !a.tt() && !a.uni() && !a.mt() {
		t = "draw"
	}
	a.tool = t
	if t != "select" && t != "type" && !a.selecting {
		a.sel = nil
	}
}

// toolKey picks a tool from the keyboard and says which.
func (a *App) toolKey(t string) {
	if t == "half" && !a.pc() && !a.tt() && !a.mt() {
		a.msg("The Half tool is for PC pages, the Sixel tool for teletext and Minitel pages")
		return
	}
	if t == "logo" && !a.logoTool() { // TheDraw fonts: the page must take them, and fonts be here (tdf_ui.go)
		return
	}
	a.start = false // a tool means drawing: on the page of the last used type
	a.setTool(t)
	for _, tl := range tools {
		if tl.id == t {
			name := tl.name
			if t == "half" && (a.tt() || a.mt()) {
				name = "Sixel"
			}
			a.msg(name + " tool")
		}
	}
}

// setFg sets the brush's colour; on an Atari page, the text colour of the whole screen.
func (a *App) setFg(i int) {
	if a.msx() { // the screen's colours, or the brush's group's (msx.go)
		a.msxSetColour(false, i)
		return
	}
	if a.atari() {
		if a.doc.ink != i {
			a.pushUndo()
			a.doc.ink = i
		}
		return
	}
	a.fg, a.ufgOn = i, false
}

// fgNow is the colour typing gives: the brush's, or an Atari screen's text colour.
func (a *App) fgNow() int {
	if a.atari() {
		return a.doc.ink
	}
	return a.fg
}

func (a *App) setBg(i int) {
	if a.msx() {
		a.msxSetColour(true, i)
		return
	}
	if a.screen() { // one background for the whole screen
		if a.doc.bg != i {
			a.pushUndo()
			a.doc.bg = i
		}
		return
	}
	a.bg, a.ubgOn = i, false
	if i > 7 && !a.ice && !a.uni() && !a.cpc() { // a Unicode page's backgrounds never blink
		a.msg("Backgrounds 8-15 need iCE colours (otherwise they blink on PC)")
	}
}

// bgNow is the background the brush gives: its own, or a screen's colour.
func (a *App) bgNow() int {
	if a.screen() {
		return a.doc.bg
	}
	return a.bg
}

// nColours is how many colours the brush's foreground (or background) can
// have on this page; 0 on teletext, where colours are codes on the page.
func (a *App) nColours(bg bool) int {
	switch {
	case a.tt():
		return 0
	case a.mt(), a.mz():
		return 8
	case a.st():
		return a.res().colours
	case a.cpc():
		return a.cpcSet.info().inks
	case a.c64() && !bg && a.mach().mono:
		return 1
	case a.ted():
		return 128
	case a.c64() && !bg:
		return a.mach().colours
	}
	return 16
}

// colourKeyRefused says why the colour keys do nothing here, or "".
func (a *App) colourKeyRefused(bg bool) string {
	switch n := a.nColours(bg); {
	case n == 0:
		return "Teletext colours are codes on the page: type them from F-key sets 1 and 2"
	case n == 1:
		return "A PET screen has one colour"
	}
	return ""
}

// stepFg and stepBg step the foreground or background colour through the page's colours.
func (a *App) stepFg(d int) {
	if why := a.colourKeyRefused(false); why != "" {
		a.msg(why)
		return
	}
	if a.ted() { // the hue steps, the luminance stays (ted.go)
		a.tedStep(d)
		return
	}
	if a.atari() {
		a.atariStep(d, false)
		return
	}
	n := a.nColours(false)
	a.setFg((a.fgNow()%n + d + n) % n)
}

func (a *App) stepBg(d int) {
	if why := a.colourKeyRefused(true); why != "" {
		a.msg(why)
		return
	}
	if a.atari() {
		a.atariStep(d, true)
		return
	}
	n := a.nColours(true)
	if a.ted() {
		a.setBg(a.doc.bg&0x70 | (a.doc.bg&15+d+16)%16)
		return
	}
	a.setBg((a.bgNow()%n + d + n) % n)
}

// colourKey sets colour i as the foreground (or background); asking for the
// colour it already is gives its bright twin (i+8), and again back, as Moebius does.
func (a *App) colourKey(i int, bg bool) {
	if why := a.colourKeyRefused(bg); why != "" {
		a.msg(why)
		return
	}
	if a.atari() {
		a.atariColourKey(i, bg)
		return
	}
	n := a.nColours(bg)
	if a.ted() && i < 16 {
		a.tedKey(i, bg)
		return
	}
	if i >= n {
		a.msg(fmt.Sprintf("This page has %d colours (0-%d)", n, n-1))
		return
	}
	cur := a.fgNow()
	if bg {
		cur = a.bgNow()
	}
	if cur == i && i+8 < n {
		i += 8
	}
	if bg {
		a.setBg(i)
	} else {
		a.setFg(i)
	}
}

// defaultColours gives the brush the page's start-up colours: 7 on 0 on Amiga
// and PC pages, the machine's own elsewhere. A screen's colours (Atari text,
// C64 screen) are left as they are.
func (a *App) defaultColours() {
	switch {
	case a.tt():
		a.msg(a.colourKeyRefused(false))
		return
	case a.atari(): // the start-up registers
		if a.doc.bg != atariBg || a.doc.ink != atariInk || a.doc.border != atariBorder {
			a.pushUndo()
			a.doc.bg, a.doc.ink, a.doc.border = atariBg, atariInk, atariBorder
		}
		a.msg("The Atari's start-up colours: 709 = 202, 710 = 148, 712 = 0")
		return
	case a.st():
		a.fg, a.bg = a.res().fg, a.res().bg
	case a.zx():
		a.fg, a.bg = 0, 7 // black ink on white paper, as at start-up
	case a.cpc():
		a.fg, a.bg = 1, 0 // pen 1 on paper 0, as at start-up
	case a.msx(): // COLOR 15,4,4 (msx.go)
		if s := newMSXSetup(a.msxSet.mode); s != a.msxSet {
			a.pushUndo()
			a.msxSet = s
		}
		a.msxBrushColours()
	case a.mz():
		a.fg, a.bg = 7, 1 // white on blue, as the monitor starts
	case a.ted(): // black on a light screen, white on a dark one
		a.fg, a.flash = map[bool]int{true: 0x00, false: 0x71}[a.doc.bg>>4&7 >= 4 && a.doc.bg&15 != 0], false
	case a.c64():
		a.fg = map[string]int{"vic20": 6, "pet": 1, "pet80": 1}[a.machine]
		if a.fg == 0 {
			a.fg = 14 // light blue
		}
	default:
		a.fg, a.bg = 7, 0
	}
	a.msg(fmt.Sprintf("Colours: %d on %d", a.fgNow(), a.bgNow()))
}

// swapColours swaps the brush's foreground and background.
func (a *App) swapColours() {
	if why := a.colourKeyRefused(false); why != "" {
		a.msg(why)
		return
	}
	if a.msx() { // the screen's (or the group's) colours change places
		fg, bg := a.fg, a.bg
		a.msxSetColour(false, bg)
		a.msxSetColour(true, fg)
		return
	}
	a.fg, a.bg = a.bg, a.fg
	a.ufg, a.ubg, a.ufgOn, a.ubgOn = a.ubg, a.ufg, a.ubgOn, a.ufgOn
}

// toggleIce switches iCE colours (backgrounds 8-15 instead of blink) on Amiga and PC pages.
func (a *App) toggleIce() {
	if !a.pc() && !a.amiga() {
		a.msg("iCE colours are for PC and Amiga pages")
		return
	}
	a.ice = !a.ice
	a.msg(map[bool]string{true: "iCE colours on: backgrounds 8-15", false: "iCE colours off: backgrounds 8-15 blink"}[a.ice])
}

// toggleNinePx switches a PC page between 8- and 9-pixel letter spacing (the
// SAUCE flag Moebius's "Use 9px Font" sets): the line characters 0xC0-0xDF
// stretch into the ninth column, the others get a gap.
func (a *App) toggleNinePx() {
	if !a.pc() {
		a.msg("9-pixel letter spacing is for PC pages")
		return
	}
	nine := !a.ninePx()
	a.sauceFlags &^= 6
	if nine {
		a.sauceFlags |= 4
	} else {
		a.sauceFlags |= 2
	}
	a.info.writeSauce = true
	a.gen = nextGen()
	a.msg(map[bool]string{true: "9-pixel letter spacing (VGA): saved in the SAUCE record", false: "8-pixel letter spacing"}[a.ninePx()])
}

// pickFset makes F-key set n (0-based) the active one, if the page has it.
func (a *App) pickFset(n int) {
	if sets := a.fkeys[a.mode]; n >= len(sets) {
		a.msg(fmt.Sprintf("This page has %d F-key sets", len(sets)))
		return
	}
	a.fset = n
}

func (a *App) setFset(n int) {
	sets := a.fkeys[a.mode]
	a.fset = (n%len(sets) + len(sets)) % len(sets)
}

// ---------------------------------------------------------------- selection and clipboard

func (a *App) selRect() Rect {
	s := a.sel
	x, y := min(s.x0, s.x1), min(s.y0, s.y1)
	return Rect{x, y, abs(s.x1-s.x0) + 1, abs(s.y1-s.y0) + 1}
}

func (a *App) copySel(cut bool) {
	if a.sel == nil {
		a.msg("Select a block first (Select tool)")
		return
	}
	r := a.selRect()
	a.clip = a.doc.copyRect(r.x, r.y, r.w, r.h)
	a.clipText = strings.Join(docText(a.doc, a.runeOf, r.x, r.y, r.w, r.h), "\n")
	setClipboard(a.clipText)
	if cut {
		a.clearSel()
	}
	verb := "Copied"
	if cut {
		verb = "Cut"
	}
	a.msg(fmt.Sprintf("%s %dx%d", verb, r.w, r.h))
}

func (a *App) clearSel() {
	r := a.selRect()
	a.pushUndo()
	for y := r.y; y < min(r.y+r.h, a.doc.rows); y++ { // clearing never adds rows
		for x := r.x; x < r.x+r.w; x++ {
			a.doc.set(x, y, blank)
		}
	}
}

func (a *App) paste() {
	text := getClipboard()
	if a.clip != nil && (text == a.clipText || text == "") {
		a.startPaste(a.clip)
	} else if text != "" {
		a.startPaste(a.textBlock(text))
	} else {
		a.msg("The clipboard is empty")
	}
}

func (a *App) startPaste(b *Block) {
	a.float = b
	a.floatDX, a.floatDY, a.moving = 0, 0, false
	a.sel = nil
	if a.hover == nil {
		a.hover = &Hover{x: a.cx, y: a.cy}
	}
	a.msg("Click to stamp the block (Shift: without blanks), Esc to stop")
}

func (a *App) stamp(x0, y0 int, transparent bool) {
	f := a.float
	a.pushUndo()
	for y := 0; y < f.h; y++ {
		for x := 0; x < f.w; x++ {
			v := f.cells[y*f.w+x]
			switch {
			case f.uni && !a.uni(): // from a Unicode page: the nearest the page can show
				v, _ = uniToPC(v, a.amiga(), &a.pal)
			case !f.uni && a.uni():
				v = pcToUni(v, false)
			}
			if transparent && ((a.uni() && uniBlank(v)) || (!a.uni() && isBlank(v))) {
				continue
			}
			a.setCell(x0+x, y0+y, v) // a block below the last row adds rows, up to the page's limit
		}
	}
}

func (a *App) textBlock(text string) *Block {
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	text = strings.TrimSuffix(text, "\n")
	lines := strings.Split(text, "\n")
	w := 1
	for _, l := range lines {
		w = max(w, utf8.RuneCountInString(l))
	}
	w = min(w, a.doc.cols)
	b := &Block{w: w, h: len(lines), cells: make([]uint32, w*len(lines)), uni: a.uni()}
	fillCells(b.cells, blank)
	for y, l := range lines {
		x := 0
		for _, r := range l {
			if x >= w {
				break
			}
			c := 32
			if r != '\t' {
				c = a.charToByte(r)
			}
			if c < 0 {
				c = '?'
			}
			b.cells[y*w+x] = a.cell(c)
			x++
		}
	}
	return b
}

// ---------------------------------------------------------------- loading

func (a *App) reparse() {
	a.frames, a.cur, a.restructured = nil, 0, false
	a.layers, a.lay = nil, 0
	if a.reparseUni() {
		return
	}
	if a.reparseEditor() || a.reparseCPC() || a.reparseCTM() || a.reparseIGS() || a.reparseSky() || a.reparseVideotex() || a.reparseAtari8() || a.reparseZXTap() || a.reparseTED() || a.reparseMSX() || a.reparseMZ() || a.reparseDirArt() {
		return
	}
	switch a.srcKind {
	case "pcb", "avt":
		cols := a.forceCols
		if cols == 0 {
			cols = 80
		}
		if a.srcKind == "pcb" {
			a.doc = parsePCBoard(a.source, cols)
		} else {
			a.doc = parseAvatar(a.source, cols)
		}
		a.cx, a.cy, a.sel, a.float = 0, 0, nil, nil
		return
	case "xbin", "idf", "adf", "tnd":
		if d, _, err := parseOwn(a.srcKind, a.source); err == nil {
			a.doc = d
		}
		a.cx, a.cy, a.sel, a.float = 0, 0, nil, nil
		return
	case "marq", "petmate", "prg", "d64":
		if scr, err := a.screens(a.source, a.srcKind); err == nil {
			a.frames = make([]Frame, len(scr))
			for i, s := range scr {
				a.frames[i] = Frame{doc: s.doc, lower: s.lower, name: s.name, src: s.fb}
			}
			a.doc = a.frames[0].doc.clone()
			a.showCase()
		}
		a.cx, a.cy, a.sel, a.float = 0, 0, nil, nil
		return
	}
	if a.tt() {
		switch {
		case a.srcKind == "tti":
			head, segs := ttiHead(a.source)
			a.frames = make([]Frame, len(segs))
			for i, seg := range segs {
				a.frames[i] = Frame{doc: parseTTI(seg), hold: ttiHold(seg, head), seg: seg, lower: -1, src: -1}
			}
			a.doc = a.frames[0].doc.clone()
		case isEditTF(a.source):
			a.doc, a.ttMeta = parseEditTFMeta(a.source)
		case isEP1(a.source):
			a.doc = parseEP1(a.source)
		case (len(a.source) == 1000 || len(a.source) == 1024) && !isTTI(a.source): // a Mode 7 dump
			a.doc = parseMode7(a.source)
		default:
			a.doc = parseTTI(a.source)
		}
		if a.vd() { // a teletext file read as Viewdata: 24 rows
			a.vdCut()
		}
	} else if a.zx() {
		d, approx := parseSCRFont(a.source, a.font())
		a.doc = d
		if approx > 0 {
			a.msg(fmt.Sprintf("%d of 768 cells are drawings, not characters: shown as the nearest character", approx))
		}
	} else if a.st() {
		a.doc = parseVT52(a.source, a.res(), a.forceCols)
	} else if a.atari() {
		a.doc = parseAta(a.source, a.forceCols)
	} else if a.c64() {
		cols := a.forceCols
		if cols == 0 {
			cols = a.mach().cols
		}
		d, lower := parseSeqOn(a.source, cols, a.ted(), a.codes)
		a.doc = d
		if lower >= 0 {
			a.setCase(lower == 1)
		}
	} else {
		d, _, fr := parseArt(a.source, ParseOpts{amiga: a.amiga(), binary: a.sourceBin, forceCols: a.forceCols, utf8: a.sourceUTF8, codes: a.codes})
		a.doc = d
		if fr >= 0 {
			for _, f := range fonts {
				if f.syncterm == fr && f.kind == a.mode {
					a.fontFor[a.mode] = f.id
				}
			}
		}
	}
	a.cx, a.cy, a.sel, a.float = 0, 0, nil, nil
}

var textExt = regexp.MustCompile(`(?i)\.(txt|asc|nfo|diz)$`)
var seqExt = regexp.MustCompile(`(?i)\.(seq|pet)$`)

// loadBytes reads a file into the current tab; as is "auto", "amiga", "pc" or "c64".
func (a *App) loadBytes(b []byte, name, path, as string) {
	a.srcKind, a.ownFont, a.ownLower, a.ownPal = "", nil, nil, nil
	a.codes, a.sky = "", nil
	if a.loadUni(b, name, path, as) { // Unicode pages: UTF-8 ANSI, durdraw, mIRC (uniformats.go)
		return
	}
	if a.loadEditor(b, name, path, as) { // other editors' files (editorformats.go)
		return
	}
	if a.loadCPC(b, name, path, as) { // Amstrad CPC .bin and .dsk (cpc.go)
		return
	}
	if a.loadAtari8(b, name, path, as) || a.loadZXTap(b, name, path, as) { // Atari .xex/.atr, Spectrum .tap (atari8.go, zxtap.go)
		return
	}
	if a.loadMSX(b, name, path, as) || a.loadMZ(b, name, path, as) { // MSX .rom/.bin, MZ-700 .mzf (msx.go, mz.go)
		return
	}
	a.igs = nil
	if a.loadC64x(b, name, path, as) { // CharPad projects (c64x_ctm.go)
		return
	}
	if a.loadIGS(b, name, path, as) { // Atari ST IGS (igs_page.go)
		return
	}
	if a.loadSkyPix(b, name, path, as) { // Amiga SkyPix (skypix.go)
		return
	}
	if a.loadVideotex(b, name, path, as) { // Minitel .vdt, Viewdata .cpt/.bin/.json (minitel_ui.go)
		return
	}
	if a.loadDirArt(b, name, path, as) { // a disk's directory as a DirArt page (dirart.go)
		return
	}
	if a.loadTED(b, name, path, as) { // TED Screen Editor's .scrn, Open as Plus/4 (ted_formats.go)
		return
	}
	if what := notArt(b); what != "" && as == "auto" {
		a.msg("Could not open " + name + ": it is " + what + ", not art")
		return
	}
	lname := strings.ToLower(name)
	ownKind := ""
	switch {
	case isXBin(b):
		ownKind = "xbin"
	case isIDF(b):
		ownKind = "idf"
	case isTundra(b):
		ownKind = "tnd"
	case strings.HasSuffix(lname, ".adf"):
		ownKind = "adf"
	}
	if ownKind != "" && (as == "auto" || as == "pc") {
		_, info, err := parseOwn(ownKind, b)
		if err != nil {
			a.msg("Could not read " + name + ": " + err.Error())
			return
		}
		a.mode, a.srcKind, a.ice = "pc", ownKind, info.ice
		if info.font != nil {
			a.ownFont, a.fontFor["pc"] = info.font, "xbin"
		}
		if info.pal != nil {
			a.ownPal, a.paletteFor["pc"] = info.pal, "file"
		}
		sauce, _ := readSauce(b) // Moebius writes one after an XBin: title, author, group, comments
		a.finishLoad(b, name, path, sauce, "xbin")
		return
	}
	// PCBoard and Avatar: text with their own colour codes.
	if as == "auto" || as == "pc" || as == "pcb" || as == "avt" {
		k := ""
		switch {
		case as == "pcb" || as == "avt":
			k = as
		case strings.HasSuffix(lname, ".pcb") || (isPCBoard(b) && !bytes.Contains(b, []byte("\x1b["))):
			k = "pcb"
		case strings.HasSuffix(lname, ".avt") || (isAvatar(b) && !bytes.Contains(b, []byte("\x1b["))):
			k = "avt"
		}
		if k != "" {
			a.mode, a.srcKind, a.ice = "pc", k, false
			a.finishLoad(b, name, path, nil, k)
			return
		}
	}
	if as == "imageseq" { // a .seq with Image BBS's £ codes (c64bbs.go)
		a.codes, as = as, "c64"
	}
	// Other boards' colour codes (bbscodes.go): read in front of the ANSI terminal.
	if isBBSCodes(as) {
		a.codes, as = as, "pc"
	} else if as == "auto" || as == "pc" {
		if k := bbsCodesKind(b, name); k != "" {
			a.codes, as = k, "pc"
		}
	}
	kind := ""
	switch {
	case isPetmate(b):
		kind = "petmate"
	case isMarq(b):
		kind = "marq"
	case strings.HasSuffix(strings.ToLower(name), ".prg") && (isOurC64Prg(b) || isTEDPrg(b)):
		kind = "prg"
	case isD64(name, b): // a disk with a screen's program on it (c64x_d64.go)
		if _, _, err := d64Art(b); errors.Is(err, errD64NoScreen) {
			a.msg("Only C64 programs in Petmate's form (Petmate, PETSCII editor) or CharEdit's can be opened: " + name + " has " + err.Error())
			return
		} else if err != nil {
			a.msg("Could not open " + name + ": " + err.Error())
			return
		}
		kind = "d64"
	case strings.HasSuffix(strings.ToLower(name), ".prg"):
		a.msg("Only C64 programs in Petmate's form (Petmate, PETSCII editor) or CharEdit's can be opened")
		return
	}
	if kind != "" && (as == "auto" || as == "c64") {
		if !a.hasC64 {
			t := a.Tab
			openC64Req(func() {
				if a.backTo(t) {
					a.loadBytes(b, name, path, as)
				}
			})
			return
		}
		scr, err := a.screens(b, kind)
		if err != nil {
			a.msg("Could not read " + name + ": " + err.Error())
			return
		}
		if scr[0].machine == "plus4" && a.tedNeedsROM(func() { a.loadBytes(b, name, path, as) }) {
			return
		}
		a.mode, a.srcKind = "c64", kind
		a.screenMachine(scr[0].machine)
		a.finishLoad(b, name, path, nil, kind)
		if kind == "petmate" {
			if f := petmateOwnFont(b); f != nil {
				a.useOwnFont(f, nil, false)
			}
		}
		a.c64xAfterLoad(b, kind)     // CharEdit's .prg and .d64: mode and character set
		a.tedCharsAfterLoad(b, kind) // the Plus/4's with a character set (tedchars.go)
		if len(scr) > 1 {
			a.msg(fmt.Sprintf("%s: %d screens, opened as frames (%s and %s go through them)", name, len(scr),
				keyName("frame_previous"), keyName("frame_next")))
		}
		return
	}
	sauce, _ := readSauce(b)
	binary := strings.HasSuffix(strings.ToLower(name), ".bin") || (sauce != nil && sauce.dataType == 5)
	utf := sauce == nil && !binary && fromUtf8(b, true) != nil
	sfont := ""
	if sauce != nil {
		sfont = strings.ToLower(sauce.font)
	}
	mode := ""
	switch {
	case as != "auto":
		mode = as
	case seqExt.MatchString(name) && sauce == nil:
		mode = "c64"
	case strings.HasSuffix(strings.ToLower(name), ".ata") && sauce == nil:
		mode = "atari"
	case isEditTF(b) || isEP1(b) || isMode7(b, name) || (strings.HasSuffix(strings.ToLower(name), ".tti") && isTTI(b)):
		mode = "tt" // before VT52: a .tti row may hold ESC codes
	case sauce == nil && isVT52(b):
		mode = "st"
	case strings.HasSuffix(strings.ToLower(name), ".scr") && len(b) == 6912:
		mode = "zx"
	case sauce == nil && !utf && looksAtascii(b):
		mode = "atari"
	case strings.HasPrefix(sfont, "amiga"):
		mode = "amiga"
	case sauce != nil && regexp.MustCompile(`^(ibm|cp437|vga|ega|mda)`).MatchString(sfont):
		mode = "pc"
	case binary:
		mode = "pc"
	case utf:
		mode = map[bool]string{true: "pc", false: "amiga"}[utf8LooksPc(b)]
	default:
		mode = map[bool]string{true: "amiga", false: "pc"}[guessAmiga(b)]
	}
	if mode == "st" && !hasSTFont() {
		t := a.Tab
		openSTReq(func() {
			if !a.backTo(t) {
				return
			}
			a.forceCols = 0
			a.setMode("st")
		})
	}
	if mode == "c64" && a.hasC64 && a.codes == "" && (as == "auto" || as == "c64") && a.machine != "plus4" && isImageCodes(b) { // Image is a C64 board
		a.codes = "imageseq" // Image BBS's £ codes (c64bbs.go)
	}
	if mode == "c64" && !a.hasC64 {
		a.codes = ""
		// Shown as PC for now; with the character set it is read again as PETSCII.
		t := a.Tab
		openC64Req(func() {
			if !a.backTo(t) {
				return
			}
			a.forceCols = 0
			a.setMode("c64")
		})
		mode = "pc"
	}
	a.mode = mode
	src, fromUTF8 := b, false
	if utf && mode != "c64" && mode != "atari" && mode != "st" && mode != "zx" && mode != "tt" {
		src, fromUTF8 = fromUtf8(b, mode == "amiga"), true
		a.msg("Read as UTF-8 text")
	}
	a.ice = false
	if fromUTF8 && regexp.MustCompile(`\x1b\[(\d*;)*(48;5;|10[0-7]m)`).Match(b) {
		a.ice = true // bright backgrounds in a file for today's terminals, which do not blink
	}
	if sauce != nil {
		if f := fontBySauce(sauce.font, mode); f != nil {
			a.fontFor[mode] = f.id
		}
		a.ice = sauce.flags&1 != 0
	}
	a.source, a.sourceBin, a.sauce, a.forceCols = src, binary && mode == "pc", sauce, 0
	a.sourceUTF8, a.sauceFlags, a.rawCtl = fromUTF8, 0, mode == "pc" && rawControls(b)
	if sauce != nil && (sauce.dataType == 1 || sauce.dataType == 5) {
		a.sauceFlags = sauce.flags & 0x1E // letter spacing and aspect ratio, kept for saving
	}
	a.info = Info{writeSauce: sauce != nil}
	if sauce != nil {
		a.info.title, a.info.author, a.info.group, a.info.notes = sauce.title, sauce.author, sauce.group, sauce.notes
		a.info.utf8 = sauce.utf8
		if f := a.font(); sauce.font != "" && !strings.EqualFold(f.sauce, sauce.font) {
			a.info.font, a.info.fontID = sauce.font, f.id // IBM VGA25G, other code pages: kept
		}
	}
	a.path, a.name = path, name
	head := b[:min(len(b), 4096)]
	switch {
	case mode == "c64":
		a.saveFmt = "seq"
		if a.codes == "imageseq" {
			a.saveFmt = a.codes
		}
	case mode == "atari":
		a.saveFmt = "ata"
	case mode == "zx":
		a.saveFmt = "scr"
	case mode == "cpc": // a stream of the CPC's text codes without a program round it
		a.saveFmt = "cpcbin"
	case mode == "tt":
		a.saveFmt = map[bool]string{true: "ep1", false: "tti"}[isEP1(b)]
		if isEditTF(b) {
			a.saveFmt = "tturl"
		}
	case mode == "st":
		a.saveFmt = "vt52"
		if r := guessSTRes(b); r != "" {
			a.stres = r
		}
	case a.codes != "":
		// Saved in its codes again, also when it has ANSI escapes too (a Mystic
		// file often has both): the board shows the codes; Save as has ANSI.
		a.saveFmt = a.codes
	case binary:
		a.saveFmt = "bin"
	case textExt.MatchString(name) && !bytes.ContainsAny(head, "\x1b\x9b"):
		a.saveFmt = "text"
	default:
		a.saveFmt = "ansi"
	}
	if fromUTF8 { // a file for today's terminals is saved for them again, not in CP437
		if f, ok := map[string]string{"ansi": "ansiutf8", "text": "utf8"}[a.saveFmt]; ok {
			a.saveFmt = f
		}
	}
	if a.codes != "" {
		a.msg(bbsOpenNote(a.codes, a.saveFmt))
		if a.codes == "ctrla" && ctrlaUsesIce(b) {
			a.ice = true // Ctrl-A E: bright backgrounds
		}
	}
	a.undo, a.redo = nil, nil
	if a.saveFmt == "tti" && isTTI(b) {
		a.srcKind = "tti"
	}
	a.reparse()
	if a.codes != "" && a.saveFmt == a.codes && bytes.Contains(b, []byte("\x1b[")) {
		// A file with ANSI escapes too saves back in its codes only if they can hold
		// the whole page: an ANSI with a WWIV heart footer that also draws ♥ (a code
		// to WWIV) stays ANSI.
		if _, l := writeBBSCodes(a.codes, a.doc, false, a.ice); l.blink > 0 || l.hearts > 0 {
			a.saveFmt = "ansi"
			a.msg(bbsOpenNote(a.codes, a.saveFmt))
		}
	}
	a.applyMode()
	a.gen = nextGen()
	a.markSaved()
	a.scrollX, a.scrollY = 0, 0
	if n := a.nFrames(); n > 1 && a.srcKind == "tti" {
		a.msg(fmt.Sprintf("%s: a carousel of %d subpages, opened as frames (%s and %s go through them)", name, n,
			keyName("frame_previous"), keyName("frame_next")))
		return
	}
	a.split = a.streamSplit()
	switch {
	case a.split.clears >= 2:
		a.msg(fmt.Sprintf("An animation of %d pictures: %s shows its frames", a.split.clears, keyName("frames_strip")))
	case a.doc.fullest:
		a.msg("An animation that ends nearly empty: showing its fullest picture (Play shows it all)")
	}
}

// notArt names files that are surely not art (archives, programs, pictures), or "".
func notArt(b []byte) string {
	for _, k := range []struct{ magic, what string }{
		{"PK\x03\x04", "a ZIP archive"}, {"Rar!", "a RAR archive"}, {"\x1f\x8b", "a gzip archive"}, {"7z\xbc\xaf", "a 7-Zip archive"},
		{"MZ", "a DOS or Windows program"}, {"\x7fELF", "a Linux program"}, {"\x89PNG", "a PNG picture"}, {"GIF8", "a GIF picture"},
		{"\xff\xd8\xff", "a JPEG picture"}, {"%PDF", "a PDF"}, {"ID3", "an MP3"}, {"OggS", "an Ogg file"},
		{"fLaC", "a FLAC file"}, {"RIFF", "a sound or video file"}, {"IMPM", "an Impulse Tracker module"},
		{"Extended Module", "an XM module"}, {"\x1aE\xdf\xa3", "a video"},
	} {
		if bytes.HasPrefix(b, []byte(k.magic)) {
			return k.what
		}
	}
	if len(b) >= 12 && string(b[4:8]) == "ftyp" {
		return "an MP4 video"
	}
	if len(b) >= 48 && string(b[44:48]) == "SCRM" {
		return "a Scream Tracker module"
	}
	if len(b) >= 1084 && strings.Contains("M.K. M!K! FLT4 4CHN 6CHN 8CHN", string(b[1080:1084])) {
		return "a MOD module"
	}
	if len(b) > 1 && b[0] == 0xFF && b[1]&0xE0 == 0xE0 {
		return "an MP3"
	}
	return ""
}

// screensCache keeps the last file of screens read, so that the tabs of a file
// with many screens do not each read all of it again.
var screensCache struct {
	data *byte
	n    int
	kind string
	scr  []PetScreen
}

// screens reads a file that holds whole C64 screens.
// screenMachine puts a PETSCII page on the machine its file names (when its
// character set is here).
func (a *App) screenMachine(id string) {
	if id != "" && hasROM(machineByID(id).family) {
		a.machine = id
		a.paletteFor["c64"] = machineByID(id).palette // a PET is green, a VIC-20 has its own colours
	}
}

func (a *App) screens(b []byte, kind string) ([]PetScreen, error) {
	if len(b) > 0 && screensCache.data == &b[0] && screensCache.n == len(b) && screensCache.kind == kind {
		out := make([]PetScreen, len(screensCache.scr))
		for i, s := range screensCache.scr { // each tab gets a page of its own
			out[i] = PetScreen{doc: s.doc.clone(), lower: s.lower, name: s.name, machine: s.machine, fb: s.fb}
		}
		return out, nil
	}
	scr, err := a.readScreens(b, kind)
	if err == nil && len(b) > 0 {
		screensCache.data, screensCache.n, screensCache.kind, screensCache.scr = &b[0], len(b), kind, scr
		out := make([]PetScreen, len(scr))
		for i, s := range scr {
			out[i] = PetScreen{doc: s.doc.clone(), lower: s.lower, name: s.name, machine: s.machine, fb: s.fb}
		}
		return out, nil
	}
	return scr, err
}

func (a *App) readScreens(b []byte, kind string) ([]PetScreen, error) {
	switch kind {
	case "petmate":
		return parsePetmate(b)
	case "prg", "d64":
		if kind == "d64" { // its first screen's program (c64x_d64.go)
			f, _, err := d64Art(b)
			if err != nil {
				return nil, err
			}
			b = f.data
		}
		if scr, ok := tedScreen(b); ok { // a Plus/4's (ted_formats.go)
			return scr, nil
		}
		d, lower, err := readC64Prg(b)
		if err != nil {
			return nil, err
		}
		return []PetScreen{{doc: d, lower: lower}}, nil
	}
	return parseMarq(b)
}

// finishLoad is the end of loading for files that are not streams of codes.
func (a *App) finishLoad(b []byte, name, path string, sauce *Sauce, fmtID string) {
	a.source, a.sourceBin, a.sauce, a.forceCols = b, false, sauce, 0
	a.sourceUTF8, a.sauceFlags, a.rawCtl = false, 0, false
	a.info = Info{}
	if sauce != nil {
		a.info = Info{writeSauce: true, title: sauce.title, author: sauce.author, group: sauce.group, notes: sauce.notes}
	}
	a.path, a.name, a.saveFmt = path, name, fmtID
	a.undo, a.redo = nil, nil
	a.reparse()
	a.applyMode()
	a.gen = nextGen()
	a.markSaved()
	a.scrollX, a.scrollY = 0, 0
}

// applyMode makes the shared tool and F-key set fit the page being worked on,
// and sets up its look (applyLook).
func (a *App) applyMode() {
	a.applyLook()
	if a.tool == "half" && !a.pc() && !a.tt() && !a.uni() && !a.mt() { // Half on PC and Unicode pages, Sixel on teletext and Minitel
		a.setTool("draw")
	}
	if a.fset >= len(a.fkeys[a.mode]) {
		a.fset = 0
	}
}

// applyLook sets up how the current tab's page is drawn and nothing else, so
// it may run for every page on show, every frame.
func (a *App) applyLook() {
	a.doc.glyph0 = a.screen() || a.mz() // the MZ-700's character 0 is the digit 0 (mz.go)
	a.doc.uni = a.uni()
	a.setPalette()
}

var modeNames = map[string]string{"amiga": "Amiga", "pc": "PC", "c64": "PETSCII", "atari": "Atari", "st": "Atari ST", "zx": "ZX Spectrum", "tt": "Teletext"}

func (a *App) setMode(m string) {
	if m == a.mode {
		return
	}
	a.endPlay()
	if a.layers != nil && !layerMode(m) {
		a.msg("Merge the layers first (Page > Merge all layers): a " + modeNames[m] + " page has one layer")
		return
	}
	if (m == "uni" || a.mode == "uni") && a.setModeUni(m) { // cells are converted (uni.go)
		return
	}
	if m == "c64" && !a.hasC64 {
		t := a.Tab
		openC64Req(func() {
			if a.backTo(t) {
				a.setMode("c64")
			}
		})
		return
	}
	if m == "st" && !hasSTFont() {
		t := a.Tab
		openSTReq(func() {
			if a.backTo(t) {
				a.setMode("st")
			}
		})
		return
	}
	if m == "st" {
		a.fg, a.bg = a.res().fg, a.res().bg
	}
	if m == "zx" {
		a.fg, a.bg = 0, 7
	}
	if m == "cpc" {
		a.cpcEnsure()
		a.fg, a.bg = 1, 0
	}
	if m == "mt" || a.mode == "mt" {
		a.fg, a.bg = 7, 0
	}
	if m == "msx" {
		a.msxEnsure()
	}
	if m == "mz" {
		a.fg, a.bg = 7, 1
	} else if a.mode == "mz" {
		a.fg, a.bg = 7, 0
	}
	was := a.mode
	a.mode = m
	switch {
	case a.source != nil && !a.dirty:
		a.reparse()
	case a.source == nil && !a.dirty && a.doc.empty():
		// An empty page takes the new machine's width and colours.
		switch {
		case m == "c64":
			a.doc = newMachineDoc(a.mach())
		case m == "atari":
			a.doc = newAtariDoc()
		case m == "st":
			a.doc = newSTDoc(a.res())
		case m == "zx":
			a.doc = newZXDoc()
			a.fg, a.bg = 0, 7
		case m == "tt":
			a.doc = newTTDoc()
		case m == "cpc":
			a.doc = newCPCDoc(a.cpcSet.mode)
		case m == "mt":
			a.doc = newMTDoc()
		case m == "vd":
			a.doc = newVDDoc()
		case m == "msx":
			a.doc = newMSXDoc(a.msxSet.mode)
		case m == "mz":
			a.doc = newMZDoc()
		case was == "cpc" || was == "mt" || was == "vd" || was == "msx" || was == "mz":
			a.doc = newDoc(80, 25)
		case was == "c64" || was == "atari" || was == "st" || was == "zx" || was == "tt":
			a.doc = newDoc(80, 25)
		}
		fmtID, ext := map[string]string{"c64": "seq", "atari": "ata", "st": "vt52", "zx": "scr", "tt": "tti"}[m], ""
		if m == "cpc" {
			fmtID = "cpcbin"
		}
		if f, ok := map[string]string{"mt": "vdt", "vd": "vdcpt", "msx": "msxrom", "mz": "mzf"}[m]; ok {
			fmtID = f
		}
		if fmtID == "" {
			fmtID = "ansi"
		}
		ext = "." + map[string]string{"ansi": "ans", "seq": "seq", "ata": "ata", "vt52": "vt", "scr": "scr", "tti": "tti"}[fmtID]
		if m == "cpc" {
			ext = ".bin"
		}
		if e, ok := map[string]string{"mt": ".vdt", "vd": ".cpt", "msx": ".rom", "mz": ".mzf"}[m]; ok {
			ext = e
		}
		a.saveFmt = fmtID
		if a.path == "" {
			a.name = "untitled" + ext
		}
	case a.dirty:
		a.msg("Shown with the " + modeNames[m] + " font and colours; the bytes are unchanged")
	}
	a.applyMode()
	savePrefs()
}

// setMachine makes a PETSCII page one for another Commodore machine: its
// character set, palette and width.
func (a *App) setMachine(id string) {
	m := machineByID(id)
	a.endPlay()
	if !hasROM(m.family) {
		t := a.Tab
		openC64Req(func() {
			if a.backTo(t) {
				a.setMachine(id)
			}
		})
		return
	}
	lower := a.lower()
	reread := a.source != nil && !a.dirty && !a.splitPage() && (a.srcKind == "" || a.srcKind == "seq")
	fresh := a.source == nil && !a.dirty && a.doc.empty()
	if !reread && !fresh { // to or from a Plus/4: the colours become the nearest (one undo step, ted.go)
		a.convertMachineColours(a.machine, id)
	} else {
		a.tedBrushFor(a.machine, id, fresh)
	}
	a.machine, a.defMachine = id, id
	a.setCase(lower)
	a.paletteFor["c64"] = m.palette
	a.setPalette()
	switch {
	case reread:
		a.forceCols = m.cols
		a.reparse()
		a.setCase(lower)
	case fresh:
		a.doc = newMachineDoc(m)
		a.applyMode()
	case a.doc.cols != m.cols && a.srcKind != "marq" && a.srcKind != "petmate":
		// The page takes the machine's width; Ctrl+Z brings the old one back.
		cut := false
		for y := 0; y < a.doc.usedRows(); y++ {
			if a.doc.lastCol(y) >= m.cols {
				cut = true
			}
		}
		a.withEveryFrame(func(d *Doc) *Doc { return d.withCols(m.cols) })
		a.cx = min(a.cx, m.cols-1)
		if cut {
			a.msg(fmt.Sprintf("The %s has %d columns: what was further right is cut (%s undoes it)", m.name, m.cols, keyName("undo")))
		}
	}
	savePrefs()
}

// newSTDoc is an empty ST screen of a resolution, in its start-up colours.
func newSTDoc(r STRes) *Doc {
	d := newDoc(r.cols, r.rows)
	fillCells(d.cells, makeCell(32, r.fg, r.bg, 0))
	return d
}

// setSTRes changes an ST page's resolution: its width, font and colours.
func (a *App) setSTRes(id string) {
	r := stResByID(id)
	a.endPlay()
	if a.igsSetRes(id) { // an IGS page draws its picture again (igs_page.go)
		return
	}
	a.stres, a.defSTRes = id, id
	a.paletteFor["st"] = r.palette
	a.setPalette()
	a.fg, a.bg = r.fg, r.bg
	switch {
	case a.source != nil && !a.dirty && !a.splitPage():
		a.forceCols = r.cols
		a.reparse()
	case a.source == nil && !a.dirty && a.doc.empty():
		a.doc = newSTDoc(r)
	case a.doc.cols != r.cols:
		a.withEveryFrame(func(d *Doc) *Doc { return d.withCols(r.cols) })
		a.cx = min(a.cx, r.cols-1)
	}
	savePrefs()
}

// setRows sets the page's height: more empty rows at the bottom, or fewer
// (what is cut can be had back with Ctrl+Z).
func (a *App) setRows(n int) {
	a.endPlay()
	n = max(1, min(a.maxRows(), n))
	if n == a.doc.rows || a.zx() || a.tt() || a.cpc() || a.mt() || a.msx() || a.mz() { // a Spectrum screen is 24 rows, a teletext page and a CPC screen 25
		return
	}
	if a.layers != nil { // every layer is cut; the shorter ones stay as they are (layers.go)
		a.withEveryLayer(func(d *Doc) *Doc {
			c := d.clone()
			c.rows = min(c.rows, n)
			return c
		})
	} else {
		a.pushUndo()
	}
	if n > a.doc.rows {
		a.doc.ensureRows(n)
	} else {
		if a.doc.usedRows() > n {
			a.msg(fmt.Sprintf("Rows below %d are cut (%s undoes it)", n, keyName("undo")))
		}
		a.doc.rows = n
		fillCells(a.doc.cells[n*a.doc.cols:], blank)
	}
	a.cy = min(a.cy, n-1)
	a.sel = nil
}

func (a *App) setCols(n int) {
	a.endPlay()
	n = max(8, min(1000, n))
	if n == a.doc.cols || a.c64() || a.st() || a.zx() || a.tt() || a.cpc() || a.mt() || a.msx() || a.mz() || (a.atari() && n != 38 && n != 40) {
		return // a machine's screen has the width it has
	}
	if a.source != nil && !a.dirty && !a.splitPage() {
		a.forceCols = n
		a.reparse()
	} else {
		a.withEveryFrame(func(d *Doc) *Doc { return d.withCols(n) })
	}
	a.cx = min(a.cx, n-1)
}

// newAtariDoc is an empty Atari screen in the Atari's own start-up colours.
func newAtariDoc() *Doc {
	d := newDoc(40, 24)
	d.bg, d.ink, d.border = atariBg, atariInk, 0
	d.glyph0 = true // as parseAta: character 0 is the heart, not an empty cell
	return d
}

// ---------------------------------------------------------------- tabs

// newTab adds an empty page of a type to the tabs.
func (a *App) newTab(mode string) *Tab {
	t := a.makeTab(mode)
	a.tabs = append(a.tabs, t)
	return t
}

// makeTab is an empty page of a type, with the fonts, palettes and machine
// new pages start with.
func (a *App) makeTab(mode string) *Tab {
	t := &Tab{mode: mode, zoom: a.defZoom, fontFor: map[string]string{}, paletteFor: map[string]string{},
		info: Info{writeSauce: true}, saveFmt: "ansi", name: "untitled.ans"}
	for k, v := range a.defFont {
		t.fontFor[k] = v
	}
	for k, v := range a.defPalette {
		t.paletteFor[k] = v
	}
	if t.zoom == 0 {
		t.zoom = 2
	}
	t.ch8, t.smear = a.ch8Default, a.smearDefault
	t.doc = newDoc(80, 25)
	t.machine = a.defMachine
	if mode == "c64" {
		m := machineByID(t.machine)
		t.doc = newMachineDoc(m)
		t.saveFmt, t.name = "seq", "untitled.seq"
	}
	t.stres = a.defSTRes
	if mode == "tt" {
		t.doc = newTTDoc()
		t.saveFmt, t.name = "tti", "untitled.tti"
	}
	if mode == "zx" {
		t.doc = newZXDoc()
		t.saveFmt, t.name = "scr", "untitled.scr"
	}
	if mode == "st" {
		t.doc = newSTDoc(stResByID(t.stres))
		t.saveFmt, t.name = "vt52", "untitled.vt"
	}
	if mode == "uni" {
		t.doc.uni = true
		t.saveFmt, t.ice = "uniansi", true
		t.info.writeSauce = false // terminal art has none: cat would show it
	}
	if mode == "atari" {
		t.doc = newAtariDoc()
		t.saveFmt, t.name = "ata", "untitled.ata"
	}
	t.vic = defaultVIC
	if mode == "cpc" {
		t.cpcSet = newCPCSetup(1)
		t.doc = newCPCDoc(1)
		t.saveFmt, t.name = "cpcbin", "untitled.bin"
	}
	if mode == "mt" {
		t.doc = newMTDoc()
		t.saveFmt, t.name = "vdt", "untitled.vdt"
		t.cy = 1 // under the status row
	}
	if mode == "vd" {
		t.doc = newVDDoc()
		t.saveFmt, t.name = "vdcpt", "untitled.cpt"
	}
	if mode == "msx" {
		t.msxSet = newMSXSetup(0)
		t.doc = newMSXDoc(0)
		t.saveFmt, t.name = "msxrom", "untitled.rom"
	}
	if mode == "mz" {
		t.doc = newMZDoc()
		t.saveFmt, t.name = "mzf", "untitled.mzf"
	}
	return t
}

// pristine: an untitled tab nobody has touched, which an opened file may take over.
func (t *Tab) pristine() bool {
	return t.path == "" && !t.dirty && t.source == nil && t.frames == nil && t.doc.empty() && len(t.undo) == 0
}

// focusTab makes t the tab being worked on, and shows it in the active pane.
func (a *App) focusTab(t *Tab) {
	if a.Tab == t {
		return
	}
	if a.Tab != nil {
		a.endPlay() // a tab plays only while it is the one being worked on
		a.Tab.hover = nil
	}
	pane := 0
	if a.layout != "one" && a.shown[1] == a.Tab {
		pane = 1
	}
	if a.layout != "one" && a.shown[1-pane] == t {
		pane = 1 - pane // already on show in the other pane: just go there
	}
	a.shown[pane] = t
	a.Tab = t
	a.applyMode()
}

// backTo goes back to tab t after a download, if it is still open.
func (a *App) backTo(t *Tab) bool {
	if a.tabIndex(t) < 0 {
		return false
	}
	a.focusTab(t)
	return true
}

func (a *App) tabIndex(t *Tab) int {
	for i, o := range a.tabs {
		if o == t {
			return i
		}
	}
	return -1
}

func (a *App) cycleTab(n int) {
	i := a.tabIndex(a.Tab)
	a.focusTab(a.tabs[((i+n)%len(a.tabs)+len(a.tabs))%len(a.tabs)])
}

// addTab opens a new tab: the start screen when it is on, else an empty page of the type being worked on.
func (a *App) addTab() {
	a.focusTab(a.newTab(a.mode))
	a.start, a.startSel = !prefs.NoStart, 0
}

func (a *App) closeTab(t *Tab) {
	if t.dirty {
		a.dialog = &Confirm{text: "\"" + t.name + "\" has changes that are not saved. Save them before closing?",
			button: "Throw away", yes: func() { t.dirty = false; a.closeTab(t) },
			save: func() {
				a.focusTab(t)
				a.saveThen(func() { a.closeTab(t) })
			}}
		return
	}
	i := a.tabIndex(t)
	if i < 0 {
		return
	}
	a.tabs = append(a.tabs[:i], a.tabs[i+1:]...)
	dropAutosave(t) // closed on purpose: saved, or its changes thrown away
	if len(a.tabs) == 0 {
		a.newTab(t.mode)
	}
	next := a.tabs[min(i, len(a.tabs)-1)]
	for p := range a.shown {
		if a.shown[p] == t {
			a.shown[p] = next
		}
	}
	if a.layout != "one" && a.shown[0] == a.shown[1] { // never the same tab in both panes
		a.shown[1] = nil
		for _, o := range a.tabs {
			if o != a.shown[0] {
				a.shown[1] = o
				break
			}
		}
		if a.shown[1] == nil { // only one tab is left
			a.layout = "one"
		}
	}
	if a.Tab == t {
		a.Tab = nil
		a.focusTab(next)
	}
}

// saveThen saves the current tab and then does then: at once when it has a
// file, after the requester when it has none. Nothing is done if saving fails.
func (a *App) saveThen(then func()) {
	if a.path == "" {
		openSaveRequester()
		if d, ok := a.dialog.(*FileReq); ok {
			d.after = then
		}
		return
	}
	if a.writeOut(a.path, a.saveFmt) && !a.dirty {
		then()
	}
}

// setLayout shows one page, or two side by side ("cols") or one over the other ("rows").
func (a *App) setLayout(l string) {
	a.layout = l
	if l == "one" {
		a.shown = [2]*Tab{a.Tab, nil}
		return
	}
	a.shown[0] = a.Tab
	if a.shown[1] == nil || a.shown[1] == a.Tab || a.tabIndex(a.shown[1]) < 0 {
		a.shown[1] = nil
		for _, t := range a.tabs { // the next tab, or a new one
			if t != a.Tab {
				a.shown[1] = t
				break
			}
		}
		if a.shown[1] == nil {
			cur := a.Tab
			a.shown[1] = a.newTab(a.mode)
			a.Tab = cur
		}
	}
}

func (a *App) openPath(path, as string) {
	b, err := os.ReadFile(path)
	if err != nil {
		a.msg("Could not open: " + err.Error())
		return
	}
	if as == "auto" && isPicture(b) { // a picture is no art: it can be the page's reference
		askReference(path)
		return
	}
	if as == "auto" && isTDF(b) { // TheDraw fonts: into the Logo tool's list (tdf_ui.go)
		openTDFFile(path)
		return
	}
	for _, t := range a.tabs { // already open: go there
		if t.path == path && as == "auto" {
			a.focusTab(t)
			addRecent(path)
			return
		}
	}
	a.endPlay()
	if !a.pristine() && !(a.start && a.untouched()) { // the start screen's tab takes the file too
		a.focusTab(a.newTab(a.mode))
	}
	a.start = false
	a.refMove = false
	kept := a.ref // a reference put on the empty page stays, unless the file has its own
	a.stages, a.steps = nil, 0
	a.loadBytes(b, filepath.Base(path), path, as)
	a.restoreRef(nil)
	if kept != nil && a.ref == kept {
		a.rememberRef()
	}
	a.lastDir = filepath.Dir(path)
	addRecent(path)
	savePrefs()
	if a.message == "" || time.Since(a.messageAt) > time.Second {
		a.msg("Opened " + path)
	}
	a.restoreArtFont(path)
	a.restoreVICMode(path) // a C64 page's multicolour or ECM (c64x.go)
	// The work's history, for its work stages (stages.go).
	a.restoreStages(path, b)
}

func (a *App) anyDirty() int {
	n := 0
	for _, t := range a.tabs {
		if t.dirty {
			n++
		}
	}
	return n
}

// ---------------------------------------------------------------- flipping

// The block operations: flipH, flipV and turnCW (fonts.go), and invert.
const invertBlock = 3

var blockOpNames = [4]string{"Flipped left-right", "Flipped upside down", "Turned a quarter clockwise", "Inverted"}

// blockOp flips, turns or inverts the floating block (what is being pasted), or else
// the selection on the page. Characters become their mirrored or turned twins where
// the font has one (Commodore sets: Marq's tables).
func (a *App) blockOp(op int) {
	var b *Block
	var r Rect
	switch {
	case a.float != nil:
		b = a.float
	case a.sel != nil:
		r = a.selRect()
		r.h = min(r.h, a.doc.rows-r.y)
		b = a.doc.copyRect(r.x, r.y, r.w, r.h)
		if a.tt() {
			b.mosaic = a.ttMosaic(r)
		}
		if a.mt() {
			if msg := mtBlockOpRefused(b, op); msg != "" {
				a.msg(msg)
				return
			}
		}
	case op != invertBlock && (!a.tt() || a.brush >= 0x20):
		// Nothing selected: the brush character itself turns into its twin.
		was := a.brush
		a.brush = a.font().mapped(a.brush, op)
		if a.brush == was {
			a.msg(fmt.Sprintf("The brush (character %d) has no twin for that: select a block to flip or turn it", was))
		} else {
			a.msg(fmt.Sprintf("%s: the brush is character %d now", blockOpNames[op], a.brush))
		}
		return
	default:
		a.msg("Select a block (Select tool) or copy and paste one first, then flip, turn or invert it")
		return
	}
	if a.tt() {
		if msg := ttBlockOpRefused(b, op); msg != "" {
			a.msg(msg)
			return
		}
	}
	out := a.transformBlock(b, op)
	if a.float != nil {
		*a.float = *out
		a.msg(blockOpNames[op] + ": click to stamp it")
		return
	}
	a.pushUndo()
	if out.w == b.w && out.h == b.h {
		for y := 0; y < out.h; y++ {
			for x := 0; x < out.w; x++ {
				a.doc.set(r.x+x, r.y+y, out.cells[y*out.w+x])
			}
		}
		a.msg(blockOpNames[op])
		return
	}
	// Turned into another shape: lift it off the page and let it be put down.
	for y := r.y; y < r.y+r.h; y++ {
		for x := r.x; x < r.x+r.w; x++ {
			a.doc.set(x, y, blank)
		}
	}
	a.startPaste(out)
	a.hover = &Hover{x: r.x, y: r.y}
	a.msg(blockOpNames[op] + ": it has a new shape, so it is lifted off the page - click to put it down, Esc to drop it")
}

// transformBlock gives b flipped, turned or inverted.
func (a *App) transformBlock(b *Block, op int) *Block {
	f := a.font()
	out := &Block{w: b.w, h: b.h, cells: make([]uint32, len(b.cells))}
	if op == turnCW {
		out.w, out.h = b.h, b.w
	}
	if b.mosaic != nil {
		out.mosaic = make([]bool, len(b.mosaic))
	}
	for y := 0; y < b.h; y++ {
		for x := 0; x < b.w; x++ {
			i := y*b.w + x
			v := b.cells[i]
			mosaic := b.mosaic != nil && b.mosaic[i]
			var o int
			switch op {
			case flipH:
				o = y*out.w + (b.w - 1 - x)
			case flipV:
				o = (b.h-1-y)*out.w + x
			case turnCW:
				o = x*out.w + (b.h - 1 - y)
			default:
				o = i
			}
			c := cellCh(v)
			switch {
			case op == invertBlock:
				v = a.invertCell(v)
			case a.uni():
				v = uniWithCode(v, uniMirror(uniCode(v), op))
			case a.mt() && mtMosaic(v) && op != invertBlock:
				v = v&^255 | uint32(ttSixelFlip(c, op == flipV))
			case mosaic && (c < 0x40 || c >= 0x60):
				v = v&^255 | uint32(ttSixelFlip(c, op == flipV))
			case a.tt():
				if c >= 0x20 { // letters: only the known pairs; codes stay
					v = v&^255 | uint32(f.mapped(c, op))
				}
			default:
				v = v&^255 | uint32(f.mapped(c, op))
			}
			out.cells[o] = v
			if out.mosaic != nil {
				out.mosaic[o] = mosaic
			}
		}
	}
	return out
}

// invertCell: reversed on Commodore and Atari pages, ink and paper swapped on a
// Spectrum, foreground and background swapped elsewhere (without iCE colours a
// background can only be 0-7, so a bright foreground loses its brightness there).
func (a *App) invertCell(v uint32) uint32 {
	switch {
	case a.screen():
		return v ^ 0x80
	case a.uni():
		fg, bg := uniColoursOf(v)
		return uniRestyle(v, bg, fg)
	}
	fg, bg := cellFg(v), cellBg(v)
	if (a.pc() || a.amiga()) && !a.ice {
		fg &= 7
	}
	return v&^0xFF00 | uint32(bg)<<8 | uint32(fg)<<12
}

// ttMosaic tells which cells of r on a teletext page show sixels.
func (a *App) ttMosaic(r Rect) []bool {
	m := make([]bool, r.w*r.h)
	for y := 0; y < r.h; y++ {
		row := ttDecode(a.doc, r.y+y, a.ttBlack())
		for x := 0; x < r.w; x++ {
			if r.x+x < len(row) {
				m[y*r.w+x] = row[r.x+x].mosaic
			}
		}
	}
	return m
}

// ttBlockOpRefused says why a teletext block cannot be turned over this way, or "".
func ttBlockOpRefused(b *Block, op int) string {
	switch op {
	case turnCW:
		return "Teletext blocks cannot be turned: a cell's sixels are 2 wide and 3 high"
	case invertBlock:
		return "Teletext has no reversed characters to invert to"
	case flipH:
		for _, v := range b.cells {
			if cellCh(v) < 0x20 {
				return "This block holds teletext codes, which act on the cells to their right: it cannot be flipped left-right (upside down works)"
			}
		}
	}
	return ""
}

// ttSixelFlip mirrors a sixel cell's 2x3 pixels left-right, or upside down.
func ttSixelFlip(c int, vertical bool) int {
	p := c&0x1F | (c&0x40)>>1 // bit row*2+col
	q := 0
	for row := 0; row < 3; row++ {
		for col := 0; col < 2; col++ {
			if p&(1<<(row*2+col)) == 0 {
				continue
			}
			nr, nc := row, 1-col
			if vertical {
				nr, nc = 2-row, col
			}
			q |= 1 << (nr*2 + nc)
		}
	}
	return 0x20 | q&0x1F | (q&0x20)<<1
}

// ---------------------------------------------------------------- saving

type Format struct{ id, name, ext, modes string }

var formats = []Format{
	{"ansi", "ANSI (.ans)", ".ans", "amiga pc"},
	{"ansiutf8", "ANSI in UTF-8, for terminals (.ans)", ".ans", "amiga pc"},
	{"text", "Plain text, no colours (.asc)", ".asc", "amiga pc"},
	{"seq", "PETSCII stream (.seq)", ".seq", "c64"},
	{"ata", "ATASCII (.ata)", ".ata", "atari"},
	{"vt52", "Atari ST VT52 (.vt)", ".vt", "st"},
	{"scr", "ZX Spectrum screen (.scr)", ".scr", "zx"},
	{"cpcbin", "Amstrad CPC program that shows it (.bin)", ".bin", "cpc"},
	{"cpcdsk", "Amstrad CPC disk with the program (.dsk)", ".dsk", "cpc"},
	{"tti", "Teletext page (.tti)", ".tti", "tt"},
	{"ep1", "Teletext page (.ep1)", ".ep1", "tt"},
	{"tturl", "edit.tf link (.url)", ".url", "tt"},
	{"utf8", "UTF-8 text, no colours (.txt)", ".txt", "amiga pc c64 atari st zx tt"},
	{"bin", "PC binary (.bin)", ".bin", "pc"},
	{"pcb", "PCBoard @X codes (.pcb)", ".pcb", "pc"},
	{"avt", "Avatar (.avt)", ".avt", "pc"},
	{"pipe", "Pipe codes |07: Mystic, Renegade (.ans)", ".ans", "pc"},
	{"ctrla", "Synchronet Ctrl-A codes (.msg)", ".msg", "pc"},
	{"wwiv", "WWIV pipe codes (.msg)", ".msg", "pc"},
	{"wildcat", "Wildcat! @XX@ codes (.bbs)", ".bbs", "pc"},
	{"celerity", "Celerity |x codes (.ans)", ".ans", "pc"},
	{"xbin", "XBin, font and palette inside (.xb)", ".xb", "amiga pc"},
	{"prg", "C64 program that shows it (.prg)", ".prg", "c64 (C64)"},
	{"marq", "C array: PETSCII editor, Petmate (.c)", ".c", "c64"},
	{"petmate", "Petmate workspace (.petmate)", ".petmate", "c64"},
	{"png", "Picture (.png)", ".png", "amiga pc c64 atari st zx tt"},
	{"png2", "Picture, twice the size (.png)", ".png", "amiga pc c64 atari st zx tt"},
	{"gif", "Animation (.gif)", ".gif", "amiga pc c64 atari st zx tt"},
	{"apng", "Animation, APNG (.png)", ".png", "amiga pc c64 atari st zx tt"},
	{"html", "Web page (.html)", ".html", "amiga pc c64 atari st zx"},
	// The files of other editors (editorformats.go).
	{"idf", "For other editors: iCEDraw (.idf)", ".idf", "amiga pc"},
	{"adf", "For other editors: ArtWorx (.adf)", ".adf", "amiga pc"},
	{"tnd", "For other editors: TundraDraw (.tnd)", ".tnd", "amiga pc"},
	{"icy", "For other editors: IcyDraw (.icy)", ".icy", "amiga pc"},
	{"xp", "For other editors: REXPaint (.xp)", ".xp", "pc"},
	{"psci", "For other editors: Playscii (.psci)", ".psci", "amiga pc c64 atari"},
	{"pet", "For other editors: PETSCII editor (.pet)", ".pet", "c64 (C64)"},
	{"marqbas", "For other editors: PETSCII editor BASIC (.bas)", ".bas", "c64 (C64)"},
	{"marqasm", "For other editors: PETSCII editor ACME (.s)", ".s", "c64 (C64)"},
	{"pmjson", "For other editors: Petmate JSON (.json)", ".json", "c64"},
}

// modeFormats are the formats the current page can be saved in.
func (a *App) modeFormats() []Format {
	var out []Format
	for _, f := range formats {
		if f.modes == "c64 (C64)" && (!a.c64() || a.machine != "c64") {
			continue // a C64 program runs on a C64 only
		}
		if a.tedHides(f) { // the Plus/4's own, and what it cannot have (ted_formats.go)
			continue
		}
		if a.dirartHides(f) { // DirArt's .d64 on DirArt pages only (dirart.go)
			continue
		}
		if strings.Contains(f.modes, a.mode) {
			out = append(out, f)
		}
	}
	return out
}

func (a *App) encode(fmtID string) ([]byte, error) {
	return a.encodeFor(fmtID, a.pageBoard(), nil)
}

// encodeFor writes the page as fmtID; an animated stream gets board's pause
// codes (bbspause.go; "" none), and *ps (when not nil) is where it pauses.
func (a *App) encodeFor(fmtID, board string, ps *[]pause) ([]byte, error) {
	defer a.asSeen()() // the layers put together (layers.go)
	d, am, info := a.doc, a.amiga(), a.info
	paced := func(out []byte, p []pause) []byte {
		if b := boardByID(board); b != nil && b.shows(fmtID) {
			if ps != nil {
				*ps = p
			}
			return b.pace(out, p)
		}
		return out
	}
	withSauce := func(art []byte, fileType int) []byte {
		if !info.writeSauce {
			return art
		}
		flags := a.sauceFlags & 0x1E // letter spacing and aspect ratio, as the file had them
		if a.ice {
			flags |= 1
		}
		return append(art, makeSauce(SauceInfo{title: info.title, author: info.author, group: info.group, notes: info.notes,
			cols: d.cols, rows: max(1, d.rows), font: a.sauceFont(), fileType: fileType, flags: flags, utf8: info.utf8}, len(art))...)
	}
	nl := "\r\n"
	if !a.pc() {
		nl = "\n"
	}
	if (fmtID == "ansi" || fmtID == "text") && d.cols != 80 && !info.writeSauce {
		// Only the record tells other programs the width: without it the art falls apart.
		info.writeSauce, a.info.writeSauce = true, true
		a.msg(fmt.Sprintf("Saved with a SAUCE record: it is the only way to keep a width of %d", d.cols))
	}
	if limit := map[string]int{"seq": a.mach().cols, "ata": 40}[fmtID]; limit > 0 && d.cols > limit {
		a.msg(fmt.Sprintf("Only the first %d columns fit this format; the rest is not saved", limit))
		d = d.withCols(limit)
	}
	if a.nFrames() > 1 && (fmtID == "ansi" || fmtID == "seq" || fmtID == "ata" || fmtID == "vt52") {
		docs := a.allDocs()
		if limit := map[string]int{"seq": a.mach().cols, "ata": 40}[fmtID]; limit > 0 {
			for i, fd := range docs {
				if fd.cols > limit {
					docs[i] = fd.withCols(limit)
				}
			}
		}
		out := paced(a.writeStreamFramesPaced(fmtID, docs, info.writeSauce))
		if fmtID == "ansi" {
			d = &Doc{cols: docs[0].cols, rows: max(25, docs[0].rows)} // SAUCE: the first frame's size
			out = withSauce(out, 1)
		}
		return out, nil
	}
	if a.orderedSave(fmtID) { // the page's draw order (transitions.go)
		out := paced(a.writeOrderedPaced(fmtID, d, info.writeSauce))
		if fmtID == "ansi" {
			out = withSauce(out, 1)
		}
		return out, nil
	}
	if out, ok, err := a.encodeUni(fmtID, d); ok { // uniformats.go
		return out, err
	}
	if out, ok, err := a.encodeDirArt(fmtID, d); ok { // DirArt .d64 (dirart.go)
		return out, err
	}
	if out, ok, err := a.encodeTED(fmtID, d); ok { // the Plus/4's .prg, .d64, .scrn and .seq (ted_formats.go)
		return out, err
	}
	if out, ok, err := a.encodeC64x(fmtID, d); ok { // C64 .prg and .d64 beyond Petmate's form, asm and BASIC (c64x_prg.go)
		return out, err
	}
	if out, ok, err := a.encodeCPC(fmtID, d); ok { // Amstrad CPC .bin and .dsk (cpc.go)
		return out, err
	}
	if out, ok, err := a.encodeMSX(fmtID, d); ok { // MSX .rom and .bin (msx.go)
		return out, err
	}
	if out, ok, err := a.encodeMZ(fmtID, d); ok { // MZ-700 .mzf (mz.go)
		return out, err
	}
	if out, ok, err := a.encodeAtari8(fmtID, d); ok { // Atari .xex and .atr (atari8.go)
		return out, err
	}
	if out, ok, err := a.encodeZXTap(fmtID, d); ok { // Spectrum .tap (zxtap.go)
		return out, err
	}
	if out, ok := a.encodeImageSeq(fmtID, board, ps, d); ok { // PETSCII with Image BBS's £ codes (c64bbs.go)
		return out, nil
	}
	if out, ok := a.encodeSky(fmtID, d); ok { // Amiga SkyPix (skypix.go)
		return out, nil
	}
	if out, ok := a.encodeIGS(fmtID, d); ok { // Atari ST IGS (igs_page.go)
		return out, nil
	}
	if out, ok, err := a.encodeVideotex(fmtID, d); ok { // Minitel and Viewdata (minitel_ui.go)
		return out, err
	}
	switch fmtID {
	case "ansi":
		return withSauce(writeAnsi(d, am, true, info.clear, info.writeSauce, info.writeSauce && a.rawCtl), 1), nil
	case "ansiutf8":
		// No SAUCE record: a terminal would show it, and durdraw and Moebius's UTF-8
		// export write none either; the lines give the width.
		return writeAnsi(d, am, true, info.clear, false, false, true), nil
	case "text":
		return withSauce(writeAnsi(d, am, false, false, info.writeSauce, info.writeSauce && a.rawCtl), 0), nil
	case "seq":
		return writeSeq(d, a.lower(), info.clear), nil
	case "ata":
		return writeAta(d, info.clear), nil
	case "vt52":
		return writeVT52(d, a.res()), nil
	case "scr":
		return writeSCRFont(d, a.font()), nil
	case "tti":
		if a.srcKind == "tti" && isTTI(a.source) || a.nFrames() > 1 {
			return a.writeTTIFrames(), nil
		}
		return writeTTI(d), nil
	case "ep1":
		return writeEP1(d), nil
	case "tturl":
		return writeEditTFURLMeta(d, a.ttMeta), nil
	case "utf8":
		return []byte(strings.Join(docText(d, a.runeOf, 0, 0, 0, 0), nl) + nl), nil
	case "bin":
		if err := binSaveError(d); err != nil {
			return nil, err
		}
		art := writeBin(d)
		if !info.writeSauce && d.cols == 160 {
			return art, nil // 160 is what a .bin without a record is taken to be
		}
		return append(art, makeSauce(SauceInfo{title: info.title, author: info.author, group: info.group, notes: info.notes,
			cols: d.cols, font: a.sauceFont(), binaryText: true, flags: map[bool]int{true: 1}[a.ice] | a.sauceFlags&0x1E, utf8: info.utf8}, len(art))...), nil
	case "xbin":
		if err := xbinSaveError(d); err != nil {
			return nil, err
		}
		out := writeXBin(d, a.pal, a.font(), a.cellH(), a.ice)
		if info.writeSauce && (info.title != "" || info.author != "" || info.group != "" || len(info.notes) > 0) {
			// A record for the title, author, group and comments (Moebius writes one too).
			out = append(out, makeSauce(SauceInfo{title: info.title, author: info.author, group: info.group, notes: info.notes,
				cols: d.cols, rows: max(1, d.rows), dataType: 6, utf8: info.utf8}, len(out))...)
		}
		return out, nil
	case "pcb":
		return writePCBoard(d, info.clear), nil
	case "avt":
		return writeAvatar(d, info.clear), nil
	case "pipe", "ctrla", "wwiv", "wildcat", "celerity":
		// Frames one after another, each after the board's clear (as a stream file's frames).
		docs := []*Doc{d}
		if a.nFrames() > 1 {
			docs = a.allDocs()
		}
		var out []byte
		var loss bbsLoss
		for i, fd := range docs {
			o, l := writeBBSCodes(fmtID, fd, info.clear || i > 0, a.ice)
			out, loss = append(out, o...), loss.add(l)
		}
		if n := loss.note(fmtID); n != "" {
			a.msg("Saved, but " + n)
		}
		return out, nil
	case "prg", "marq":
		if fmtID == "prg" {
			if d.cols != 40 || d.usedRows() > 25 {
				a.msg("Only the first 40x25 cells fit a C64 screen")
			}
			return writePrg(d, a.lower()), nil
		}
		if a.unchangedSource(fmtID) {
			return a.source, nil
		}
		return writeMarqFrames(a.allDocs(), a.lower(), a.machine), nil
	case "petmate":
		if a.unchangedSource(fmtID) {
			return a.source, nil
		}
		scr := a.petScreens()
		ws := writePetmateFrames(map[bool][]byte{true: a.source}[a.srcKind == "petmate"], scr)
		var lowers []int
		for _, s := range scr {
			lowers = append(lowers, s.lower)
		}
		if a.ownInUse() != nil {
			return petmateWithFont(ws, a.ownFont, a.ownLower, lowers), nil
		}
		return petmateWithFont(ws, nil, nil, lowers), nil
	case "html":
		return a.writeHTML(), nil
	case "png", "png2":
		z := 1
		if fmtID == "png2" {
			z = 2
		}
		blinkWas := a.blink
		a.blink = true // flashing and blinking cells in their "on" look, as the animations draw them
		fb := a.renderDoc(d, z)
		a.blink = blinkWas
		img := image.NewRGBA(image.Rect(0, 0, fb.w, fb.h))
		for i, p := range fb.px {
			img.Pix[i*4], img.Pix[i*4+1], img.Pix[i*4+2], img.Pix[i*4+3] = byte(p>>16), byte(p>>8), byte(p), 255
		}
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	case "gif", "apng":
		return a.writeAnim(fmtID)
	}
	if out, ok, err := a.encodeEditor(fmtID, d); ok {
		return out, err
	}
	return nil, fmt.Errorf("unknown format %s", fmtID)
}

// unchangedSource: the page is its file as read or last saved, in that format,
// so saving it again gives the same bytes.
func (a *App) unchangedSource(fmtID string) bool {
	return a.source != nil && a.srcKind == fmtID && a.saveFmt == fmtID && a.gen == a.savedGen && !a.restructured
}

// petScreens are the frames as Petmate screens, each with its framebuf in the file it came from.
func (a *App) petScreens() []PetScreen {
	a.keepCase()
	out := make([]PetScreen, a.nFrames())
	for i := range out {
		out[i] = PetScreen{doc: a.frameDoc(i), lower: map[bool]int{true: 1}[a.lower()], name: strings.TrimSuffix(a.name, filepath.Ext(a.name)), fb: -1, machine: a.machine}
		if a.frames != nil {
			f := a.frames[i]
			out[i].name, out[i].fb = f.name, f.src
			if f.lower >= 0 {
				out[i].lower = f.lower
			}
		}
	}
	return out
}

// writeAtomic writes a file whole or not at all: a new file next to it,
// renamed over it. A symlink is followed (the file it points at is replaced,
// the link stays); in a folder that cannot take a new file the file is
// written over in place.
func writeAtomic(p string, data []byte) error {
	if real, err := filepath.EvalSymlinks(p); err == nil {
		p = real
	}
	mode := os.FileMode(0o644)
	if st, e := os.Stat(p); e == nil {
		mode = st.Mode().Perm()
	}
	dir := filepath.Dir(p)
	f, err := os.CreateTemp(dir, ".charcoal-")
	if err != nil {
		if !os.IsPermission(err) {
			return err
		}
		return writeInPlace(p, data, mode)
	}
	tmp := f.Name()
	_, err = f.Write(data)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Chmod(tmp, mode)
	}
	if err == nil {
		err = os.Rename(tmp, p)
	}
	if err != nil {
		os.Remove(tmp)
	}
	return err
}

// writeInPlace overwrites a file through itself (for a folder we may not write in).
func writeInPlace(p string, data []byte, mode os.FileMode) error {
	f, err := os.OpenFile(p, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

func (a *App) writeOut(path, fmtID string) bool {
	a.endPlay() // the page is saved, never the screen being played
	if st, err := os.Stat(filepath.Dir(path)); err != nil || !st.IsDir() {
		a.msg("Could not save: there is no drawer " + filepath.Dir(path))
		return false
	}
	var ps []pause
	cpcTarget = path // the name inside a CPC .bin or .dsk
	data, err := a.encodeFor(fmtID, a.pageBoard(), &ps)
	cpcTarget = ""
	bakErr, changed := error(nil), 0
	if err == nil && !prefs.NoBackup {
		bakErr = keepBackup(path, data)
	}
	if err == nil {
		err = writeAtomic(path, data)
	}
	if err != nil {
		a.msg("Could not save: " + err.Error())
		return false
	}
	own := strings.Contains(" ansi text bin seq imageseq ata vt52 scr tti ep1 tturl xbin marq petmate prg pcb avt ", " "+fmtID+" ") || editorOwn[fmtID] || isBBSCodes(fmtID) || a.uniOwn(fmtID)
	if own && a.nFrames() > 1 && !framesFormats[fmtID] {
		// One picture: written as an export, so the page is not taken for saved.
		a.lastDir = filepath.Dir(path)
		savePrefs()
		a.msg(fmt.Sprintf("%s holds one picture: frame %d written; the other %d are only here (%s)",
			filepath.Base(path), a.cur+1, a.nFrames()-1, a.keepFramesHint()))
		return true
	}
	if own { // the page's own file; the others are exports
		a.path, a.name, a.saveFmt = path, filepath.Base(path), fmtID
		a.source, a.sourceBin, a.forceCols = data, fmtID == "bin", 0
		a.srcKind = map[string]string{"xbin": "xbin", "marq": "marq", "petmate": "petmate", "prg": "prg", "pcb": "pcb", "avt": "avt", "tti": "tti"}[fmtID]
		if editorOwn[fmtID] || a.uniOwn(fmtID) {
			a.srcKind = fmtID
		}
		a.codes = map[bool]string{true: fmtID}[isBBSCodes(fmtID) || fmtID == "imageseq"] // the source is now in these codes (or none)
		a.markSaved()
		writeStages(a.Tab, path, data) // the work's history goes with the file (stages.go); losing it loses no art
		dropAutosave(a.Tab)
		a.rememberRef() // the reference goes with the file's new name
		addRecent(path)
		if framesFormats[fmtID] {
			changed = a.savedFrames(fmtID, data)
		}
	}
	a.lastDir = filepath.Dir(path)
	savePrefs()
	note := a.keepArtFont(path, fmtID) + a.keepVICMode(path, fmtID) + a.tedSaved(path, fmtID) + a.dirartSaved(fmtID)
	if a.animatedSave(fmtID) {
		note += pacedNote(a.pageBoard(), ps) + a.writeCopies(path, fmtID)
	}
	if bakErr != nil {
		a.msg(fmt.Sprintf("Saved %s, but the old version could not be kept as .bak: %v", path, bakErr) + note)
		return true
	}
	if fmtID == "vt52" {
		if n := stUnprintable(a.seen()); n > 0 {
			a.msg(fmt.Sprintf("Saved %s; %d characters below 32 became spaces (an ST cannot print them)", path, n) + note)
			return true
		}
	}
	if a.nFrames() > 1 && (fmtID == "ansi" || fmtID == "seq" || fmtID == "ata" || fmtID == "vt52") {
		held := "the hold times are not kept (these files play at the modem's speed; Animate's BBS pauses can keep them)"
		if len(ps) > 0 {
			held = "the holds kept as pauses"
		}
		a.msg(fmt.Sprintf("Saved %s (%d bytes): %d frames, a clear screen between them; %s", path, len(data), a.nFrames(), held) + note)
		return true
	}
	if fmtID == "tti" && a.nFrames() > 1 {
		a.msg(fmt.Sprintf("Saved %s (%d bytes): %d subpages, %d changed", path, len(data), a.nFrames(), changed) + note)
		return true
	}
	if framesFormats[fmtID] && a.nFrames() > 1 {
		a.msg(fmt.Sprintf("Saved %s (%d bytes): %d frames", path, len(data), a.nFrames()) + note)
		return true
	}
	a.msg(fmt.Sprintf("Saved %s (%d bytes)", path, len(data)) + note)
	return true
}

func (a *App) save(as bool) {
	a.endPlay()
	if as || a.path == "" {
		openSaveRequester()
		return
	}
	a.writeOut(a.path, a.saveFmt)
}

// ---------------------------------------------------------------- playback at modem speed

type emulator interface {
	write([]byte)
	cursor() (int, int)
}

// playBytes is what Play sends: the file as read, or the page written out
// (a changed page, or a file whose format is not a stream of codes).
func (a *App) playBytes() []byte {
	defer a.asSeen()() // the layers put together (layers.go)
	if b := a.orderedPlayBytes(); b != nil {
		return b
	}
	if a.source != nil && !a.dirty && a.srcKind == "" && (a.nFrames() == 1 || a.pageBoard() == "") { // a paced file's codes are the board's, not the terminal's
		return a.source
	}
	if a.nFrames() > 1 && !a.zx() && !a.tt() && !a.mt() { // every frame, a clear between them
		return a.writeStreamFrames(map[string]string{"c64": "seq", "atari": "ata", "st": "vt52"}[a.mode], a.allDocs(), false)
	}
	if b := a.videotexPlayBytes(); b != nil { // minitel_ui.go
		return b
	}
	switch {
	case a.c64():
		return writeSeqOn(a.doc, a.lower(), false, a.ted())
	case a.atari():
		return writeAta(a.doc, false)
	case a.st():
		return writeVT52(a.doc, a.res())
	}
	return writeAnsi(a.doc, a.amiga(), true, false)
}

func (a *App) play() {
	if a.playing != nil {
		a.stopPlay(true)
		return
	}
	if (a.source != nil && a.sourceBin && !a.dirty) || a.zx() || a.tt() && !a.vd() || a.cpc() || a.msx() || a.mz() {
		a.msg("This file has no codes to play: it is a picture of the screen")
		return
	}
	b := a.playBytes()
	_, end := readSauce(b)
	b = b[:end]
	d := newDoc(a.doc.cols, 25)
	d.bg, d.ink, d.border = a.doc.bg, a.doc.ink, a.doc.border
	var em emulator = newTerm(d, a.amiga())
	if a.c64() {
		em = newPetTermOn(d, a.ted())
	}
	if a.atari() {
		em = newAtariTerm(d)
	}
	if a.st() {
		em = newSTTerm(d, a.res())
	}
	if a.mt() || a.vd() { // a Minitel or a Viewdata terminal (minitel_ui.go)
		d, em = a.videotexPlayer()
	}
	a.playing = &Play{bytes: b, term: em, saved: a.snapshot(), last: time.Now(), pauses: a.playPauses(b)}
	a.doc, a.sel, a.float = d, nil, nil
	a.scrollY = 0
}

func (a *App) playStep() { a.playStepAt(time.Now()) }

func (a *App) playStepAt(now time.Time) {
	p := a.playing
	if p.frames != nil {
		a.frameStep(now)
		return
	}
	if p.stepLine(now, a.baud) {
		return
	}
	x, y := p.term.cursor()
	a.cx, a.cy = min(x, a.doc.cols-1), max(0, min(y, maxRowsFor(a.doc.cols)-1))
	a.doc.ensureRows(a.cy + 1)
	a.ensureVisible(a.cx, a.cy)
	if p.pos >= len(p.bytes) {
		a.stopPlay(false)
	}
}

func (a *App) stopPlay(skip bool) {
	p := a.playing
	if skip && p.term != nil {
		p.term.write(p.bytes[p.pos:])
	}
	a.playing = nil
	// Playing does not change the page: put back the page as it was.
	a.restore(p.saved)
	if p.frames != nil {
		a.msg(fmt.Sprintf("Played %d frames", len(p.frames)))
		return
	}
	a.msg(fmt.Sprintf("Played %d bytes", len(p.bytes)))
}

// endPlay stops a running Play at once and puts the page back, before anything
// that reads or changes the page (saving, other modes, other tabs ...).
func (a *App) endPlay() {
	if a.Tab == nil || a.playing == nil {
		return
	}
	p := a.playing
	a.playing = nil
	a.restore(p.saved)
}

// ninePx: the page's SAUCE asks for 9-pixel letter spacing (VGA's 9-dot cells).
func (t *Tab) ninePx() bool { return t.sauceFlags&6 == 4 }

// sauceFont is the SAUCE font name to save: the file's own when CharEdit showed
// it in a font of another name (IBM VGA25G, a code page it lacks) and it was not changed.
func (a *App) sauceFont() string {
	if a.info.font != "" && a.font().id == a.info.fontID {
		return a.info.font
	}
	return a.font().sauce
}
