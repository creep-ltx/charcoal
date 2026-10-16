package main

// C64 beyond PETSCII (roadmap step 5): the VIC-II's other text modes on a C64
// (and C128 40-column) page, for the graphics compos that take custom
// character sets (X, CSDb's charset compos).
//
//   - Multicolour character mode ($D016 bit 4): a cell whose colour RAM
//     value is 8-15 is drawn in pixel pairs, 00 the screen colour ($D021),
//     01 $D022, 10 $D023, 11 the cell's colour 0-7; a cell of colour 0-7
//     stays hi-res in that colour.
//   - Extended background colour mode ($D011 bit 6): 64 characters; bits
//     6-7 of the screen code pick the cell's background from $D021-$D024,
//     the colour RAM gives the foreground (0-15).
//
// (VIC-II data sheet, register $11/$16 and "Character display mode"; C64
// Programmer's Reference Guide, chapter 3, "Multi-color mode" and "Extended
// background color mode".) The page's custom character set is its own font
// (fontfile.go), loaded from .bin/.64c or CharPad's .ctm (c64x_ctm.go) and
// edited in the font editor. Files that hold all of it: CharEdit's .prg
// (c64x_prg.go), the .d64 with that program on it (c64x_d64.go), the ACME
// source and the BASIC listing. Formats that cannot (.seq, .c, .petmate ...)
// are saved with a note, and CharEdit remembers the mode for the file, as it
// does fonts.

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
)

// vicSetup is how a C64 page's screen is set up beyond the screen and border
// colours (the Doc's bg and border): the text mode and $D022-$D024.
type vicSetup struct {
	Mode int    `json:"mode"` // vicHires, vicMulti, vicECM
	Regs [3]int `json:"regs"` // $D022, $D023, $D024: multicolours 1 and 2, or backgrounds 1-3
}

const (
	vicHires = 0
	vicMulti = 1
	vicECM   = 2
)

// defaultVIC is the screen after the KERNAL's start: normal text, $D022-$D024
// set to 1, 2, 3 (its VIC table at $ECB9).
var defaultVIC = vicSetup{Regs: [3]int{1, 2, 3}}

var vicModeNames = []string{"Normal text", "Multicolour", "Extended bg (ECM)"}

// c64Colours are the C64's colours by number, as the Programmer's Reference Guide names them.
var c64Colours = [16]string{"black", "white", "red", "cyan", "purple", "green", "blue", "yellow", "orange", "brown",
	"light red", "dark grey", "grey", "light green", "light blue", "light grey"}

// c64xOpts are the brush settings of the extended modes (embedded in App).
type c64xOpts struct {
	ecmBg int // the background (0-3: $D021-$D024) the brush gives in ECM to characters 0-63
}

func init() {
	editorOwn["d64"] = true
	fontExts[".ctm"] = true
	// The .d64 goes right after the .prg in the Save as list.
	at := slices.IndexFunc(formats, func(f Format) bool { return f.id == "prg" })
	formats = slices.Insert(formats, at+1, Format{"d64", "C64 disk with the program (.d64)", ".d64", "c64 (C64)"})
}

// vicII: the page is for a machine with a VIC-II (C64, C128 in 40 columns).
func (a *App) vicII() bool {
	id := a.mach().id
	return a.c64() && (id == "c64" || id == "c128")
}

// vicMode is the page's text mode; hi-res on other machines and pages.
func (a *App) vicMode() int {
	if a.vicII() {
		return a.vic.Mode
	}
	return vicHires
}

func (a *App) vicMC() bool  { return a.vicMode() == vicMulti }
func (a *App) vicECM() bool { return a.vicMode() == vicECM }

// vicSnap is the setup for an undo step (nil off C64 pages).
func (a *App) vicSnap() *vicSetup {
	if !a.c64() {
		return nil
	}
	s := a.vic
	return &s
}

// vicExtended: the page shows more than the ROM's characters in one colour each:
// another text mode or its own character set.
func (a *App) vicExtended() bool { return a.vicMode() != vicHires || (a.c64() && a.ownInUse() != nil) }

func (a *App) setVICMode(m int) {
	if m == a.vic.Mode || m < 0 || m > vicECM {
		return
	}
	a.pushUndo()
	a.vic.Mode = m
	switch m {
	case vicMulti:
		a.msg("Multicolour: colours 8-15 draw in pixel pairs (screen colour, MC 1, MC 2, colour-8); 0-7 stay hi-res")
	case vicECM:
		a.msg("Extended background: characters 0-63, each on one of four backgrounds (the code's top two bits)")
	}
}

func (a *App) setVICReg(i, c int) {
	if a.vic.Regs[i] == c {
		return
	}
	a.pushUndo()
	a.vic.Regs[i] = c & 15
}

// ecmChar is the screen code a character gives in ECM: one of 0-63 gets the
// brush's background; a code past 63 (picked from the table) keeps its own.
func (a *App) ecmChar(ch int) int {
	if !a.vicECM() || ch >= 64 {
		return ch
	}
	return ch | a.ecmBg<<6
}

// vicCell is the colour of each of the cell's 8x8 pixels as the VIC-II shows it in
// the page's mode, as palette numbers.
func vicCell(f *Font, v uint32, mode int, bg int, regs [3]int, pix *[64]int) {
	ch, col := cellCh(v), cellFg(v)
	multi := false
	switch mode {
	case vicECM:
		bg = [4]int{bg, regs[0], regs[1], regs[2]}[ch>>6]
		ch &= 63
	case vicMulti:
		multi = col >= 8
		col &= 7
	}
	for r := 0; r < 8; r++ {
		bits := int(f.row(ch, r))
		for b := 0; b < 8; b++ {
			c := bg
			if multi {
				switch bits >> (6 - b&^1) & 3 {
				case 1:
					c = regs[0]
				case 2:
					c = regs[1]
				case 3:
					c = col
				}
			} else if bits&(0x80>>b) != 0 {
				c = col
			}
			pix[r*8+b] = c & 15
		}
	}
}

// drawVICCell draws a cell of a page in multicolour or ECM.
func (a *App) drawVICCell(fb *FB, px, py, z int, v uint32) {
	var pix [64]int
	vicCell(a.font(), v, a.vic.Mode, a.doc.bg, a.vic.Regs, &pix)
	for i, c := range pix {
		fb.fill(Rect{px + i%8*z, py + i/8*z, z, z}, a.pal[c])
	}
}

// drawVICGadgets puts the mode and its colours in the page's gadgets (view.go);
// row places a gadget with its label.
func (a *App) drawVICGadgets(u *UI, row func(string, int) Rect) {
	if !a.vicII() {
		return
	}
	u.choice("vicmode", row("Screen", choiceW(vicModeNames)), vicModeNames, a.vic.Mode,
		"The VIC-II's text mode: normal, multicolour (colours 8-15 in pixel pairs) or extended background colour (64 characters on 4 backgrounds)", func(i int) {
			a.setVICMode(i)
		})
	var names []string
	for i, n := range c64Colours {
		names = append(names, fmt.Sprintf("%d %s", i, n))
	}
	reg := func(i int, label, hint string) {
		u.choice(fmt.Sprintf("vicreg%d", i), row(label, choiceW(names)), names, a.vic.Regs[i], hint, func(c int) { a.setVICReg(i, c) })
	}
	switch a.vic.Mode {
	case vicMulti:
		reg(0, "MC 1", "$D022: the colour of pixel pairs 01 in multicolour cells")
		reg(1, "MC 2", "$D023: the colour of pixel pairs 10 in multicolour cells")
	case vicECM:
		reg(0, "BG 1", "$D022: the background of screen codes 64-127")
		reg(1, "BG 2", "$D023: the background of screen codes 128-191")
		reg(2, "BG 3", "$D024: the background of screen codes 192-255")
		bgs := []string{"BG 0 (screen)", "BG 1", "BG 2", "BG 3"}
		u.choice("ecmbg", row("Paint on", choiceW(bgs)), bgs, a.ecmBg,
			"The background characters 0-63 get when drawn or typed (a code picked from the table keeps its own); the Colour tool's right button paints it", func(i int) {
				a.ecmBg = i
			})
	}
}

// ---------------------------------------------------------------- kept with the file

// vicFormats are the formats that hold the screen mode (and the character set) inside.
var vicFormats = map[string]bool{"prg": true, "d64": true, "marqasm": true, "marqbas": true}

// keepVICMode runs after the page was saved as path: a format that cannot hold
// the page's mode has CharEdit remember it for the file. It gives a note for
// the status line.
func (a *App) keepVICMode(path, fmtID string) string {
	if !a.c64() {
		return ""
	}
	switch fmtID {
	case "png", "png2", "gif", "apng", "html", "utf8":
		return "" // drawn in (or no colours at all)
	}
	key := filepath.Clean(path)
	if a.vicMode() == vicHires || vicFormats[fmtID] {
		if _, ok := prefs.VICModes[key]; ok {
			delete(prefs.VICModes, key)
			savePrefs()
		}
		return ""
	}
	if prefs.VICModes == nil {
		prefs.VICModes = map[string]vicSetup{}
	}
	prefs.VICModes[key] = a.vic
	savePrefs()
	return fmt.Sprintf("; a .%s cannot hold %s mode (a .prg or .d64 can), CharEdit keeps it for the file",
		strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), "."), strings.ToLower(vicModeNames[a.vic.Mode]))
}

// restoreVICMode gives a page opened from path the mode CharEdit kept for it.
func (a *App) restoreVICMode(path string) {
	s, ok := prefs.VICModes[filepath.Clean(path)]
	if !ok || !a.c64() || s.Mode < 0 || s.Mode > vicECM {
		return
	}
	for i := range s.Regs {
		s.Regs[i] &= 15
	}
	a.vic = s
}

// c64xCarriesFont: the format holds a C64 page's own character set inside (fontfile.go).
func c64xCarriesFont(fmtID string) bool { return vicFormats[fmtID] }

// ---------------------------------------------------------------- the font editor

// fontEdChar is the character the font editor edits: the brush's (in ECM its glyph, 0-63).
func (a *App) fontEdChar() int {
	if a.vicECM() {
		return a.brush & 63
	}
	return a.brush & 255
}

// fontEdPairs: the font editor draws pixel pairs, a multicolour cell's pixels
// (the page in multicolour and the brush's colour 8-15).
func (a *App) fontEdPairs() bool { return a.vicMC() && a.fg >= 8 }

// pairColours are the palette colours of pixel pairs 00, 01, 10 and 11 in the brush's colour.
func (a *App) pairColours() [4]uint32 {
	return [4]uint32{a.pal[a.doc.bg&15], a.pal[a.vic.Regs[0]&15], a.pal[a.vic.Regs[1]&15], a.pal[a.fg&7]}
}

func glyphPair(f *Font, c, x, y int) int { return int(f.row(c, y)) >> (6 - 2*x) & 3 }

func glyphSetPair(f *Font, c, x, y, v int) {
	sh := 6 - 2*x
	f.setRow(c, y, f.row(c, y)&^(3<<sh)|byte(v&3)<<sh)
}

// drawPairs is the grid of a multicolour character: four pairs a row, each in
// its colour; a click draws the pen's pair, the right button clears it (00).
func (d *FontEd) drawPairs(u *UI, f *Font, c int, g Rect, cs int) {
	fb := u.fb
	cols := a.pairColours()
	for py := 0; py < 8; py++ {
		for px := 0; px < 4; px++ {
			fb.fill(Rect{g.x + px*2*cs + 1, g.y + py*cs + 1, 2*cs - 1, cs - 1}, cols[glyphPair(f, c, px, py)])
		}
	}
	fb.frame(Rect{g.x - 1, g.y - 1, g.w + 2, g.h + 2}, cEdgeDk)
	u.setHint(g, "Multicolour: a click or drag draws the pen's pixel pair (1-3 pick it); right: the screen colour (00)")
	if d.pen == 0 {
		d.pen = 3
	}
	at := func(x, y int) (int, int) { return (x - g.x) / (2 * cs), (y - g.y) / cs }
	if cl := u.take(g); cl != nil {
		px, py := at(cl.x, cl.y)
		d.pairPaint = d.pen
		if cl.button == 3 {
			d.pairPaint = 0
		}
		v := d.pairPaint
		d.change(func(f *Font) { glyphSetPair(f, c, px, py, v) })
		u.drag = "fontgrid"
	} else if u.drag == "fontgrid" && g.has(u.mx, u.my) {
		px, py := at(u.mx, u.my)
		if of := a.ownInUse(); of != nil && glyphPair(of, c, px, py) != d.pairPaint {
			glyphSetPair(of, c, px, py, d.pairPaint) // the same undo step as the click
			of.edited()
			a.fontChanged()
		}
	}
}

// penGadgets are the multicolour pens: pairs 01, 10 and 11 in their colours.
func (d *FontEd) penGadgets(u *UI, bx *int, y int) {
	fb := u.fb
	fb.text(*bx, y+3, "Pen", cText)
	*bx += textW("Pen") + 8
	cols := a.pairColours()
	for p, name := range []string{"01 MC 1", "10 MC 2", "11 colour"} {
		r := Rect{*bx, y, gadH + 4 + textW(name) + 8, gadH}
		fb.fill(Rect{r.x + 2, r.y + 2, gadH - 4, gadH - 4}, cols[p+1])
		fb.frame(Rect{r.x + 2, r.y + 2, gadH - 4, gadH - 4}, cEdgeDk)
		col := uint32(cText)
		if d.pen == p+1 {
			col = cOrange
		}
		fb.text(r.x+gadH+4, r.y+3, name, col)
		u.setHint(r, fmt.Sprintf("Draw pixel pair %s (%d)", name[:2], p+1))
		if u.take(r) != nil {
			d.pen = p + 1
		}
		*bx += r.w + 6
	}
}
