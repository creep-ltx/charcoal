package main

// The Plus/4's files (ted.go has the page): .seq as a Plus/4 prints it (pet.go's
// reader and writer with ted), the program Marq's PETSCII editor writes for a
// Plus/4 (.prg, also on a .d64), TED Screen Editor's screens (.scrn with its
// .proj), and the Plus/4 parts of Marq's .c and Petmate's workspace (export.go).

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func init() {
	openAs = append(openAs, "plus4")
	openAsNames = append(openAsNames, "Plus/4 PETSCII")
}

// tedSeqInk is the colour a .seq starts in on a Plus/4 page: white, so that a
// file which sets no colour shows on the black page (the KERNAL's own start is
// black on white).
const tedSeqInk = 0x71

// tedByte is what a Plus/4's screen editor does with c beyond the C64's (the
// KERNAL's CHROUT at $DC5E / $DD47, *verified* in kernal-318004-05.bin): the 16
// colour codes give the KERNAL's colours ($E133/$E143), 0x82 FLASH ON and 0x84
// FLASH OFF ($053C), RETURN ends flashing; ESC takes the next byte (an ESC
// command, not shown: the screen editor's insert/scroll commands are not
// played). 0x16 + a byte is Petmate9's own code for a whole TED colour (its
// "tedFull" .seq export; a real Plus/4 ignores 0x16). True when c is taken.
func (t *PetTerm) tedByte(c byte) bool {
	switch t.esc {
	case 2:
		t.fg, t.flash, t.esc = int(c&127), c&0x80 != 0, 0
		return true
	case 1:
		t.esc = 0
		return true
	}
	if col := petColourOf(c); col >= 0 {
		t.fg = int(tedKernal[col])
		return true
	}
	switch c {
	case 0x82:
		t.flash = true
		return true
	case 0x84:
		t.flash = false
		return true
	case 0x1B:
		t.esc = 1
		return true
	case 0x16:
		t.esc = 2
		return true
	case 0x0D, 0x8D:
		t.flash = false // and on to the C64's RETURN
	}
	return false
}

// tedSeqLoss counts the cells a .seq shows in another colour: a colour code
// gives a hue only at the KERNAL's luminance.
func tedSeqLoss(docs []*Doc) int {
	n := 0
	for _, d := range docs {
		for _, v := range d.cells {
			if sc := cellCh(v); sc != 0x20 && tedAttr(v)&0x7F != int(tedKernal[cellFg(v)]) && cellFg(v) != 0 {
				n++
			}
		}
	}
	return n
}

// ---------------------------------------------------------------- Marq's .prg

// tedPrgHead is the start of the program Marq's PETSCII editor writes for a
// Plus/4 (data/template-plus4.prg in petscii.zip, kameli.net/marq, WTFPL):
// 10 SYS4109, then LDA #$D0/$D4 STA $FF13 (upper / lower case set), LDA #border
// STA $FF19, LDA #bg STA $FF15, copy 1000 screen codes to $0C00 and 1000
// attributes to $0800 in four runs of 250, JMP *. The data follows at file
// offset 88 ($1057): 2088 bytes in all, the size Marq's editor imports.
// *Verified*: VICE 3.9's xplus4 runs CharEdit's files and shows the page.
var tedPrgHead = []byte{0x01, 0x10, 0x0B, 0x10, 0x0A, 0x00, 0x9E, 0x34, 0x31, 0x30, 0x39, 0x00, 0x00, 0x00, 0xA9, 0xD4,
	0x8D, 0x13, 0xFF, 0xA9, 0x7E, 0x8D, 0x19, 0xFF, 0xA9, 0x7F, 0x8D, 0x15, 0xFF, 0xA2, 0x00, 0xA0, 0xFA, 0xBD, 0x57, 0x10,
	0x9D, 0x00, 0x0C, 0xBD, 0x51, 0x11, 0x9D, 0xFA, 0x0C, 0xBD, 0x4B, 0x12, 0x9D, 0xF4, 0x0D, 0xBD, 0x45, 0x13, 0x9D, 0xEE,
	0x0E, 0xBD, 0x3F, 0x14, 0x9D, 0x00, 0x08, 0xBD, 0x39, 0x15, 0x9D, 0xFA, 0x08, 0xBD, 0x33, 0x16, 0x9D, 0xF4, 0x09, 0xBD,
	0x2D, 0x17, 0x9D, 0xEE, 0x0A, 0xE8, 0x88, 0xD0, 0xCC, 0x4C, 0x54, 0x10}

const (
	tedPrgCase   = 15 // offsets in the file
	tedPrgBorder = 20
	tedPrgBg     = 25
	tedPrgData   = 88
	tedPrgSize   = tedPrgData + 2000
)

// writeTEDPrg gives a Plus/4 program that shows the first 40x25 cells, in Marq's form.
func writeTEDPrg(d *Doc, lower bool) []byte {
	out := make([]byte, tedPrgSize)
	copy(out, tedPrgHead)
	out[tedPrgCase] = map[bool]byte{true: 0xD4, false: 0xD0}[lower]
	out[tedPrgBorder], out[tedPrgBg] = byte(d.borderColour()&127), byte(d.bg&127)
	for y := 0; y < 25; y++ {
		for x := 0; x < 40; x++ {
			v := d.get(x, y)
			out[tedPrgData+y*40+x], out[tedPrgData+1000+y*40+x] = byte(cellCh(v)), byte(tedAttr(v))
		}
	}
	return out
}

var errTEDPrg = errors.New("not a Plus/4 program in the PETSCII editor's form")

// parseTEDPrg reads a .prg of Marq's form (any of his or CharEdit's); the
// character set is the one $FF13 picks.
func parseTEDPrg(b []byte) (*Doc, int, error) {
	if len(b) != tedPrgSize {
		return nil, -1, errTEDPrg
	}
	for i, c := range tedPrgHead {
		if i != tedPrgCase && i != tedPrgBorder && i != tedPrgBg && b[i] != c {
			return nil, -1, errTEDPrg
		}
	}
	d := newDoc(40, 25)
	d.border, d.bg, d.glyph0 = int(b[tedPrgBorder]&127), int(b[tedPrgBg]&127), true
	for i := 0; i < 1000; i++ {
		d.cells[i] = tedCell(int(b[tedPrgData+i]), int(b[tedPrgData+1000+i]))
	}
	return d, map[bool]int{true: 1}[b[tedPrgCase]&4 != 0], nil
}

func isTEDPrg(b []byte) bool {
	_, _, err := parseTEDPrg(b)
	return err == nil
}

// ---------------------------------------------------------------- TED Screen Editor

// TEDSE (github.com/xahmol/TEDScreenEdit, its README's file format and
// src/main.c, *read*) saves a screen as a PRG: the attributes (w x h), 24
// bytes with its version (" V 0.99 - 20220518-1541 "), then the screen codes
// (w x h), from $C000; "save screen" writes 24 more bytes after them, "save
// project" does not. The size is not in the file: the project's NAME.proj (21
// bytes) has width and height (high byte first) at 4-7, the screen colour at
// 10 and the border at 20, and the brush and the menus' colours.
const tedseSig = 24

// tedseSize: the width and height a .scrn of n bytes (load address included)
// holds, given the project's or else 40 columns; 0, 0 if it cannot be one.
func tedseSize(n, w, h int) (int, int) {
	n -= 2
	for _, extra := range []int{2 * tedseSig, tedseSig} {
		cells := n - extra
		if cells <= 0 || cells%2 != 0 {
			continue
		}
		cells /= 2
		switch {
		case w > 0 && h > 0 && w*h == cells:
			return w, h
		case w <= 0 && cells%40 == 0 && cells/40 <= 1000:
			return 40, cells / 40
		}
	}
	return 0, 0
}

// isTEDSE: a TEDSE screen by its name (.scrn) and size.
func isTEDSE(name string, b []byte) bool {
	w, _ := tedseSize(len(b), 0, 0)
	return strings.EqualFold(filepath.Ext(name), ".scrn") && w > 0
}

// tedseProj reads a project file: width, height, screen colour, border, lower case set.
func tedseProj(p []byte) (w, h, bg, border int, lower bool, ok bool) {
	if len(p) < 2+21 {
		return 0, 0, 0, 0, false, false
	}
	p = p[2:]
	w, h = int(p[4])<<8|int(p[5]), int(p[6])<<8|int(p[7])
	return w, h, int(p[10] & 127), int(p[20] & 127), p[1] == 1, w > 0 && h > 0 && w*h <= maxPageCells
}

// parseTEDSE reads a .scrn (with its project's size and colours when proj is given).
func parseTEDSE(b, proj []byte) (*Doc, int, error) {
	pw, ph, bg, border, lower, ok := tedseProj(proj)
	if !ok {
		pw, ph, bg, border, lower = 0, 0, 0, 0, false // TEDSE starts black on black
	}
	w, h := tedseSize(len(b), pw, ph)
	if w == 0 && ok { // the project is not this screen's
		w, h = tedseSize(len(b), 0, 0)
	}
	if w == 0 {
		return nil, -1, fmt.Errorf("not a TED Screen Editor screen (%d bytes)", len(b))
	}
	d := newDoc(w, h)
	d.bg, d.border, d.glyph0 = bg, border, true
	attr, chars := b[2:2+w*h], b[2+w*h+tedseSig:]
	for i := range d.cells {
		d.cells[i] = tedCell(int(chars[i]), int(attr[i]))
	}
	return d, map[bool]int{true: 1}[lower], nil
}

// writeTEDSE gives the page as TEDSE's "save screen" writes it.
func writeTEDSE(d *Doc) []byte {
	n := d.cols * max(1, d.rows)
	out := make([]byte, 2+2*n+2*tedseSig)
	out[0], out[1] = 0x00, 0xC0
	sig := fmt.Sprintf(" CHARCOAL %s", version)
	for i := 0; i < tedseSig; i++ {
		c := byte(' ')
		if i < len(sig) {
			c = strings.ToUpper(sig)[i]
		}
		out[2+n+i] = c
	}
	for i := 0; i < n; i++ {
		v := d.get(i%d.cols, i/d.cols)
		out[2+i], out[2+n+tedseSig+i] = byte(tedAttr(v)), byte(cellCh(v))
	}
	return out
}

// writeTEDSEProj gives the project file for the screen: its size and colours,
// the brush, and TEDSE's own menu colours (its defaults: light green, white,
// cyan, yellow, white, as cc65's COLOR_ values).
func writeTEDSEProj(d *Doc, lower bool, brush, attr int) []byte {
	w, h := d.cols, max(1, d.rows)
	p := []byte{0x00, 0xC0, 0, map[bool]byte{true: 1}[lower], 0, 0, byte(w >> 8), byte(w), byte(h >> 8), byte(h),
		byte((w * h) >> 8), byte(w * h), byte(d.bg & 127), 0x7F, 0x71, 0x73, 0x77, 0x71,
		byte(brush), byte(attr & 15), byte(attr >> 4 & 7), byte(attr >> 7), byte(d.borderColour() & 127)}
	return p
}

// tedseProjPath is the project file next to a .scrn.
func tedseProjPath(path string) string {
	return strings.TrimSuffix(path, filepath.Ext(path)) + ".proj"
}

// ---------------------------------------------------------------- loading and saving

// tedNeedsROM asks for the Commodore character sets when the Plus/4's is
// missing (then runs again); true when it asked.
func (a *App) tedNeedsROM(again func()) bool {
	if hasROM("plus4") {
		return false
	}
	t := a.Tab
	openC64Req(func() {
		if a.backTo(t) && hasROM("plus4") {
			again()
		}
	})
	return true
}

// loadTED opens TEDSE screens, and anything with Open as "Plus/4 PETSCII"
// (a .seq read as a Plus/4 prints it); the Plus/4's .prg, .c and .petmate go
// through the C64's way (screens).
func (a *App) loadTED(b []byte, name, path, as string) bool {
	if as == "plus4" || (as == "auto" || as == "c64") && isTEDSE(name, b) {
		if a.tedNeedsROM(func() { a.loadBytes(b, name, path, as) }) {
			return true
		}
	}
	if as == "plus4" {
		a.machine = "plus4"
		a.paletteFor["c64"] = machineByID("plus4").palette
		a.loadBytes(b, name, path, "c64")
		return true
	}
	if !((as == "auto" || as == "c64") && isTEDSE(name, b)) {
		return false
	}
	var proj []byte
	if path != "" {
		if p, err := os.ReadFile(tedseProjPath(path)); err == nil && len(p) < 64 {
			proj = p
		}
	}
	if _, _, err := parseTEDSE(b, proj); err != nil {
		a.msg("Could not read " + name + ": " + err.Error())
		return true
	}
	a.mode, a.srcKind, a.machine = "c64", "tedse", "plus4"
	a.paletteFor["c64"] = machineByID("plus4").palette
	a.finishLoad(b, name, path, nil, "tedse") // reparseTED reads the project again
	if _, _, _, _, lower, ok := tedseProj(proj); ok {
		a.setCase(lower)
	}
	return true
}

// reparseTED reads a TEDSE source again.
func (a *App) reparseTED() bool {
	if a.srcKind != "tedse" {
		return false
	}
	var proj []byte
	if a.path != "" {
		if p, err := os.ReadFile(tedseProjPath(a.path)); err == nil && len(p) < 64 {
			proj = p
		}
	}
	if d, _, err := parseTEDSE(a.source, proj); err == nil {
		a.doc = d
	}
	a.cx, a.cy, a.sel, a.float = 0, 0, nil, nil
	return true
}

// tedScreen reads a .prg (or the first art program of a .d64) of the Plus/4.
func tedScreen(b []byte) ([]PetScreen, bool) {
	d, lower, err := parseTEDPrg(b)
	if err != nil {
		return nil, false
	}
	return []PetScreen{{doc: d, lower: lower, machine: "plus4"}}, true
}

// tedHides: formats a Plus/4 page does not offer (they hold the C64's 16 colours only).
func (a *App) tedHides(f Format) bool {
	if f.modes == "c64 (Plus/4)" {
		return !a.ted()
	}
	return a.ted() && (f.id == "psci" || f.id == "pmjson")
}

// encodeTED writes a Plus/4 page's own files.
func (a *App) encodeTED(fmtID string, d *Doc) ([]byte, bool, error) {
	if !a.ted() {
		return nil, false, nil
	}
	switch fmtID {
	case "prg", "d64":
		if d.cols != 40 || d.usedRows() > 25 {
			a.msg("Only the first 40x25 cells fit a Plus/4 screen")
		}
		prg := writeTEDPrg(d, a.lower())
		if fmtID == "prg" {
			return prg, true, nil
		}
		return writeD64(cpcTarget, prg), true, nil
	case "tedse":
		if a.unchangedSource(fmtID) {
			return a.source, true, nil
		}
		return writeTEDSE(d), true, nil
	case "seq":
		return writeSeqOn(d, a.lower(), a.info.clear, true), true, nil
	}
	return nil, false, nil
}

// tedSaved finishes a save of a Plus/4 page: a TEDSE screen gets its project
// file; the note says what the format could not keep.
func (a *App) tedSaved(path, fmtID string) string {
	if !a.ted() {
		return ""
	}
	switch fmtID {
	case "tedse":
		p := writeTEDSEProj(a.doc, a.lower(), a.brush, a.tedBrush())
		if old, err := os.ReadFile(tedseProjPath(path)); err == nil && bytes.Equal(old, p) {
			return ""
		}
		if err := writeAtomic(tedseProjPath(path), p); err != nil {
			return "; its .proj (size and colours) could not be written: " + err.Error()
		}
		return "; size and colours in " + filepath.Base(tedseProjPath(path)) + " (TEDSE's project)"
	case "seq":
		if n := tedSeqLoss(a.allDocs()); n > 0 {
			return fmt.Sprintf("; %d cells show the KERNAL's luminance of their colour (a .seq has 16 colours)", n)
		}
	case "marq":
		n := 0
		for _, d := range a.allDocs() {
			for _, v := range d.cells {
				if tedAttr(v)&0x80 != 0 {
					n++
				}
			}
		}
		if n > 0 {
			return fmt.Sprintf("; %d flashing cells stop flashing (the PETSCII editor has no flash)", n)
		}
	}
	return ""
}

// newPetTermOn is the PETSCII terminal of a C64 or, with ted, a Plus/4.
func newPetTermOn(d *Doc, ted bool) *PetTerm {
	t := newPetTerm(d)
	if ted {
		t.ted, t.fg = true, tedSeqInk
	}
	return t
}
