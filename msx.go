package main

// MSX pages: the text screens of the MSX's TMS9918 (and the MSX2's V9938).
// SCREEN 0 is 40x24 characters of 6x8 pixels in two colours for the whole
// screen; SCREEN 1 is 32x24 characters of 8x8 with a colour pair for each
// group of 8 characters (the colour table: characters 0-7, 8-15, ...) and a
// border (the backdrop, which colour 0 shows through); the MSX2's SCREEN 0
// WIDTH 80 is 80x24 (pixels half as wide). A cell is a character only: its
// colours are the screen's (msxSetup), as on the machine, so typing a letter
// in SCREEN 1 gives it its group's colours, and changing a colour changes
// every character of the screen (or of the group) at once.
//
// The font is C-BIOS's (fonts/msx-cbios-NOTICE.txt, BSD licence): Microsoft's
// MSX BIOS ROM may not be passed on. An MSX BIOS ROM put in CharEdit's data
// folder (msx.rom) gives its own font too ("MSX (BIOS ROM)").
//
// Files: a cartridge (.rom, 16 KB, runs on any MSX and on C-BIOS) and a BLOAD
// file (.bin: BLOAD"ART.BIN",R from MSX BASIC) that set the screen up through
// the BIOS and copy the page (and an own font) into the VRAM; CharEdit reads
// both back. Playscii's "msx" charset (.psci) is read and written.

import (
	"bytes"
	_ "embed"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

//go:embed fonts/msx-cbios.font
var msxCBIOSIntl []byte

//go:embed fonts/msx-cbios-jp.font
var msxCBIOSJP []byte

//go:embed fonts/msx-cbios-br.font
var msxCBIOSBR []byte

var msxFont = newFont("msxcbios", "C-BIOS (international)", "msx", msxCBIOSIntl, 8, 256)

// The palettes: MSX1's TMS9918 as MAME has it (R. Nabet's, from TI's data book),
// and the MSX2's V9938 power-on palette (openMSX's V9938_PALETTE: 3-bit levels).
var (
	msx1Palette = pal("msx1", "MSX1 (TMS9918)", 0x000000, 0x000000, 0x21C842, 0x5EDC78, 0x5455ED, 0x7D76FC, 0xD4524D, 0x42EBF5,
		0xFC5554, 0xFF7978, 0xD4C154, 0xE6CE80, 0x21B03B, 0xC95BBA, 0xCCCCCC, 0xFFFFFF)
	msx2Palette = func() *Palette {
		grb := []int{0x000, 0x000, 0x611, 0x733, 0x117, 0x327, 0x151, 0x627, 0x171, 0x373, 0x661, 0x664, 0x411, 0x265, 0x555, 0x777}
		var c []uint32
		for _, v := range grb {
			lv := func(n int) uint32 { return uint32((n & 7) * 255 / 7) }
			c = append(c, lv(v>>4)<<16|lv(v>>8)<<8|lv(v))
		}
		return pal("msx2", "MSX2 (V9938)", c...)
	}()
)

// msxColourNames are the TMS9918's colours as TI's data book names them.
var msxColourNames = [16]string{"Transparent", "Black", "Medium green", "Light green", "Dark blue", "Light blue", "Dark red", "Cyan",
	"Medium red", "Light red", "Dark yellow", "Light yellow", "Dark green", "Magenta", "Gray", "White"}

func init() {
	fonts = append(fonts, msxFont, newFont("msxcbiosjp", "C-BIOS (Japanese)", "msx", msxCBIOSJP, 8, 256),
		newFont("msxcbiosbr", "C-BIOS (Brazilian)", "msx", msxCBIOSBR, 8, 256))
	palettes = append(palettes[:len(palettes)-1], msx1Palette, msx2Palette, palettes[len(palettes)-1]) // before Custom
	mirrorPairs["msx"] = asciiPairs
	editorOwn["msxrom"], editorOwn["msxbin"] = true, true
	pageModes = append(pageModes, "msx")
	modeOrder = append(modeOrder, "msx")
	modeNames["msx"] = "MSX"
	openAs = append(openAs, "msx")
	openAsNames = append(openAsNames, "MSX")
	modeNoStream["msx"] = "an MSX screen: a picture of the screen, no stream to draw in order"
	for i, f := range formats {
		switch f.id {
		case "utf8", "png", "png2", "gif", "apng", "psci":
			formats[i].modes += " msx"
		}
	}
	at := len(formats)
	for i, f := range formats {
		if f.id == "cpcdsk" { // the machines' programs together
			at = i + 1
		}
	}
	formats = slices.Insert(formats, at,
		Format{"msxrom", "MSX cartridge that shows it (.rom)", ".rom", "msx"},
		Format{"msxbin", "MSX BASIC BLOAD file that shows it (.bin)", ".bin", "msx"})
}

func (a *App) msx() bool { return a.mode == "msx" }

// ---------------------------------------------------------------- the screen

// An MSX text screen mode.
type msxModeInfo struct {
	cols, cellW, cellH int
	name               string
}

var msxModes = [3]msxModeInfo{
	{40, 6, 8, "SCREEN 0 (40x24)"},
	{32, 8, 8, "SCREEN 1 (32x24)"},
	{80, 6, 16, "SCREEN 0, WIDTH 80 (MSX2)"},
}

const msxRows = 24

// msxSetup is an MSX page's screen: its mode and colours. It belongs to the
// page (all its frames), and undo keeps it (Snap.msx).
type msxSetup struct {
	ok             bool
	mode           int      // msxModes
	fg, bg, border int      // SCREEN 0: text and background (also the border); SCREEN 1: border = the backdrop
	ct             [32]byte // SCREEN 1's colour table: foreground<<4 | background of characters 8i to 8i+7
}

// newMSXSetup is the screen as MSX BASIC starts: COLOR 15,4,4.
func newMSXSetup(mode int) msxSetup {
	s := msxSetup{ok: true, mode: max(0, min(2, mode)), fg: 15, bg: 4, border: 4}
	for i := range s.ct {
		s.ct[i] = 0xF4
	}
	return s
}

func (s msxSetup) info() msxModeInfo { return msxModes[max(0, min(2, s.mode))] }

// colours are the foreground and background colours (0-15) character ch shows in.
func (s msxSetup) colours(ch int) (int, int) {
	if s.mode == 1 {
		v := s.ct[ch>>3&31]
		return int(v >> 4), int(v & 15)
	}
	return s.fg, s.bg
}

// backdrop is the colour around the screen, which colour 0 shows: in SCREEN 0
// the background (register 7's low half is both).
func (s msxSetup) backdrop() int {
	if s.mode == 1 {
		return s.border
	}
	return s.bg
}

func (a *App) msxSnap() *msxSetup {
	if !a.msx() {
		return nil
	}
	s := a.msxSet
	return &s
}

func (t *Tab) msxEnsure() {
	if !t.msxSet.ok {
		t.msxSet = newMSXSetup(0)
	}
}

// msxBlank is an empty cell: a space (its colours are the screen's).
func msxBlank() uint32 { return makeCell(32, 0, 0, 0) }

func newMSXDoc(mode int) *Doc {
	d := newDoc(msxModes[max(0, min(2, mode))].cols, msxRows)
	fillCells(d.cells, msxBlank())
	return d
}

// msxRGB is colour i of the page; 0 (transparent) shows the backdrop, and a
// transparent backdrop is black.
func (a *App) msxRGB(i int) uint32 {
	if i == 0 {
		i = a.msxSet.backdrop()
	}
	if i == 0 {
		return 0xFF000000
	}
	return a.pal[i&15]
}

func (a *App) msxCellW() int { return a.msxSet.info().cellW }
func (a *App) msxCellH() int { return a.msxSet.info().cellH }

// drawMSXCell draws a cell in the screen's colours: the glyph's left 6 pixels
// in SCREEN 0, all 8 in SCREEN 1, every row twice at WIDTH 80.
func (a *App) drawMSXCell(fb *FB, px, py, z int, v uint32) {
	ch := cellCh(v)
	f := a.font()
	fg, bg := a.msxSet.colours(ch)
	fgc, bgc := a.msxRGB(fg), a.msxRGB(bg)
	W, H := a.cellW(), a.cellH()
	for r := 0; r < H; r++ {
		bits := f.row(ch, r*8/H)
		for b := 0; b < W; b++ {
			c := bgc
			if bits&(0x80>>b) != 0 {
				c = fgc
			}
			fb.fill(Rect{px + b*z, py + r*z, z, z}, c)
		}
	}
}

// setMSXMode changes the screen mode: the width (every frame); one undo step.
func (a *App) setMSXMode(m int) {
	m = max(0, min(2, m))
	if m == a.msxSet.mode {
		return
	}
	a.endPlay()
	cols := msxModes[m].cols
	cut := false
	for y := 0; y < a.doc.rows; y++ {
		if a.doc.lastCol(y) >= cols {
			cut = true
		}
	}
	a.withEveryFrame(func(d *Doc) *Doc {
		n := newMSXDoc(m)
		for y := 0; y < min(d.rows, msxRows); y++ {
			for x := 0; x < min(cols, d.cols); x++ {
				n.cells[y*cols+x] = d.cells[y*d.cols+x]
			}
		}
		return n
	})
	a.msxSet.mode = m
	a.cx = min(a.cx, cols-1)
	if cut {
		a.msg(fmt.Sprintf("%s has %d columns: what was further right is cut (%s undoes it)", msxModes[m].name, cols, keyName("undo")))
	} else {
		a.msg(msxModes[m].name)
	}
}

// msxSetColour sets a colour of the screen (undoable): the foreground or
// background of SCREEN 0, or of the brush character's group in SCREEN 1.
func (a *App) msxSetColour(bg bool, c int) {
	c &= 15
	s := a.msxSet
	if s.mode == 1 {
		g := a.brush >> 3 & 31
		v := s.ct[g]
		if bg {
			v = v&0xF0 | byte(c)
		} else {
			v = v&0x0F | byte(c)<<4
		}
		s.ct[g] = v
	} else if bg {
		s.bg = c
	} else {
		s.fg = c
	}
	if s != a.msxSet {
		a.pushUndo()
		a.msxSet = s
	}
	a.msxBrushColours()
}

func (a *App) setMSXBorder(c int) {
	if a.msxSet.border == c&15 {
		return
	}
	a.pushUndo()
	a.msxSet.border = c & 15
}

// msxBrushColours makes the brush's colours the ones its character shows in.
func (a *App) msxBrushColours() {
	a.fg, a.bg = a.msxSet.colours(a.brush)
}

// msxPick takes the character under the mouse as the brush.
func (a *App) msxPick(v uint32) {
	a.brush = cellCh(v)
	a.msxBrushColours()
}

// ---------------------------------------------------------------- characters

// msxRunes is what each character of the MSX's international set looks like in
// Unicode (en.wikipedia.org, MSX character set; 0-31 as printed after code 1).
var msxRunes = []rune("\u0000☺☻♥♦♣♠•◘○◙♂♀♪♫☼╂┴┬┤├┼│─┌┐└┘╳╱╲┿" +
	" !\"#$%&'()*+,-./0123456789:;<=>?@ABCDEFGHIJKLMNOPQRSTUVWXYZ[\\]^_`abcdefghijklmnopqrstuvwxyz{|}~⌂" +
	"ÇüéâäàåçêëèïîìÄÅÉæÆôöòûùÿÖÜ¢£¥₧ƒáíóúñÑªº¿⌐¬½¼¡«»ÃãĨĩÕõŨũĲĳ¾∽◊‰¶§" +
	"▂▚▆🮂▬🮅▎▞▊🮇🮊🮙🮘🭭🭯🭬🭮🮚🮛▘▗▝▖🮖Δ‡ω█▄▌▐▀αßΓπΣσµτΦΘΩδ∞⌀∈∩≡±≥≤⌠⌡÷≈°∙·√ⁿ²■▒")

var msxFromRuneMap = func() map[rune]int {
	m := map[rune]int{}
	for c, r := range msxRunes {
		if _, ok := m[r]; !ok && c > 0 {
			m[r] = c
		}
	}
	return m
}()

func msxRune(c int) rune {
	if c > 0 && c < len(msxRunes) {
		return msxRunes[c]
	}
	return ' '
}

func msxFromRune(r rune) int {
	if r >= 0x20 && r < 0x7F {
		return int(r)
	}
	if c, ok := msxFromRuneMap[r]; ok {
		return c
	}
	return -1
}

var msxFkeys = [][]int{
	{0xDB, 0xDC, 0xDF, 0xDD, 0xDE, 0xD3, 0xD5, 0xD6, 0xD4, 0xD7}, // blocks, quarters, the checker
	{0xC0, 0xC2, 0xC3, 0xC4, 0xC5, 0xC6, 0xC8, 0xC9, 0xCA, 0xC1}, // bars
	{0x18, 0x19, 0x1A, 0x1B, 0x17, 0x16, 0x15, 0x14, 0x13, 0x12}, // lines
	{0x11, 0x1C, 0x1D, 0x1E, 0xCB, 0xCC, 0xCD, 0xCE, 0xCF, 0xD0}, // diagonals and wedges
	{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x09, 0x0D, 0x0E}, // symbols
}

// ---------------------------------------------------------------- the programs (.rom, .bin)

// The BIOS entries and system variables the programs use (the MSX2 Technical
// Handbook; C-BIOS has them at the same addresses).
const (
	msxCHGCLR = 0x0062
	msxLDIRVM = 0x005C
	msxINITXT = 0x006C
	msxINIT32 = 0x006F
	msxCHGET  = 0x009F
	msxERAFNK = 0x00CC
	msxKILBUF = 0x0156
	msxLINL40 = 0xF3AE
	msxLINL32 = 0xF3AF
	msxT32COL = 0xF3BF
	msxFORCLR = 0xF3E9
	msxBAKCLR = 0xF3EA
	msxBDRCLR = 0xF3EB
	msxNAMBAS = 0xF922
	msxCGPBAS = 0xF924
)

// msxMark follows the program in every file CharEdit writes: how it is read back.
var msxMark = []byte("CharEdit MSX")

const (
	msxROMBase = 0x4000 // a cartridge's page
	msxROMCode = 0x4010 // after the cartridge header
	msxBinBase = 0x9000 // where BLOAD puts the .bin: above a short BASIC program, below the disk's buffers
)

// msxScreenData is what the program copies into the VRAM: the name table (the
// page's 24 rows), SCREEN 1's colour table, and the font when it is the page's own.
type msxScreenData struct {
	s     msxSetup
	names []byte
	font  []byte // nil: the machine's own
}

func msxDataOf(d *Doc, s msxSetup, font []byte) msxScreenData {
	cols := s.info().cols
	names := make([]byte, cols*msxRows)
	for y := 0; y < msxRows; y++ {
		for x := 0; x < cols; x++ {
			c := 32
			if x < d.cols && y < d.rows {
				c = cellCh(d.get(x, y))
			}
			names[y*cols+x] = byte(c)
		}
	}
	return msxScreenData{s: s, names: names, font: font}
}

// msxProgram is the Z80 program at org, then the mark and the data. rom: a
// cartridge (it never returns); else BLOAD's ,R (a key goes back to BASIC, on a
// cleared screen 0).
func msxProgram(sd msxScreenData, org int, rom bool) []byte {
	s := sd.s
	var p []byte
	word := func(v int) []byte { return []byte{byte(v), byte(v >> 8)} }
	call := func(at int) { p = append(append(p, 0xCD), word(at)...) }
	poke := func(at, v int) { p = append(append(p, 0x3E, byte(v), 0x32), word(at)...) } // LD A,v / LD (at),A
	// The places in p where the data's addresses go, filled in below.
	type fix struct{ at, what int }
	var fixes []fix
	copyVRAM := func(what, dst, n int) { // LD HL,data / LD DE,(dst) / LD BC,n / CALL LDIRVM
		fixes = append(fixes, fix{len(p) + 1, what})
		p = append(p, 0x21, 0, 0, 0xED, 0x5B)
		p = append(p, word(dst)...)
		p = append(append(p, 0x01), word(n)...)
		call(msxLDIRVM)
	}
	call(msxERAFNK) // no function keys on the bottom line
	fg, bg, border := s.fg, s.bg, s.border
	if s.mode != 1 {
		border = bg // SCREEN 0's backdrop is its background (C-BIOS takes BDRCLR for it, MSX BASIC BAKCLR)
	}
	poke(msxFORCLR, fg)
	poke(msxBAKCLR, bg)
	poke(msxBDRCLR, border)
	if s.mode == 1 {
		poke(msxLINL32, 32)
		call(msxINIT32)
	} else {
		poke(msxLINL40, s.info().cols) // 80: the MSX2's TEXT2
		call(msxINITXT)
	}
	call(msxCHGCLR)
	if sd.font != nil {
		copyVRAM(2, msxCGPBAS, len(sd.font))
	}
	copyVRAM(0, msxNAMBAS, len(sd.names))
	if s.mode == 1 {
		copyVRAM(1, msxT32COL, 32)
	}
	if rom {
		p = append(p, 0x18, 0xFE) // JR $: the cartridge is the program
	} else {
		call(msxKILBUF)
		call(msxCHGET) // wait for a key
		poke(msxLINL40, 40)
		call(msxINITXT) // back to BASIC on a clear screen 0
		p = append(p, 0xC9)
	}
	// The mark, the setup, then the data.
	p = append(p, msxMark...)
	flags := 0
	if sd.font != nil {
		flags |= 1
	}
	p = append(p, 1, byte(s.mode), byte(s.fg), byte(s.bg), byte(s.border), byte(flags))
	at := map[int]int{}
	at[0] = org + len(p)
	p = append(p, sd.names...)
	at[1] = org + len(p) // the colour table (kept in every mode, so the page reads back whole)
	p = append(p, s.ct[:]...)
	if sd.font != nil {
		at[2] = org + len(p)
		p = append(p, sd.font...)
	}
	for _, f := range fixes {
		copy(p[f.at:], word(at[f.what]))
	}
	return p
}

// writeMSXROM is a 16 KB cartridge: "AB", INIT = the program.
func writeMSXROM(sd msxScreenData) []byte {
	out := make([]byte, 16384)
	for i := range out {
		out[i] = 0xFF
	}
	copy(out, []byte{'A', 'B', msxROMCode & 255, msxROMCode >> 8})
	for i := 4; i < 16; i++ {
		out[i] = 0
	}
	copy(out[msxROMCode-msxROMBase:], msxProgram(sd, msxROMCode, true))
	return out
}

// writeMSXBin is a BLOAD file: 0xFE, start, end, run address, then the program.
func writeMSXBin(sd msxScreenData) []byte {
	p := msxProgram(sd, msxBinBase, false)
	out := []byte{0xFE}
	out = binary.LittleEndian.AppendUint16(out, msxBinBase)
	out = binary.LittleEndian.AppendUint16(out, uint16(msxBinBase+len(p)-1))
	out = binary.LittleEndian.AppendUint16(out, msxBinBase)
	return append(out, p...)
}

func isMSXROM(b []byte) bool {
	return len(b) >= 16 && len(b) <= 64<<10 && b[0] == 'A' && b[1] == 'B'
}

func isMSXBin(b []byte) bool {
	if len(b) < 8 || b[0] != 0xFE {
		return false
	}
	start, end := int(binary.LittleEndian.Uint16(b[1:])), int(binary.LittleEndian.Uint16(b[3:]))
	return end >= start && end-start+1 <= len(b)-7
}

// readMSXProgram reads a page out of a .rom or .bin CharEdit wrote.
func readMSXProgram(b []byte) (*Doc, msxSetup, []byte, error) {
	i := bytes.Index(b, msxMark)
	if i < 0 || !(isMSXROM(b) || isMSXBin(b)) {
		return nil, msxSetup{}, nil, fmt.Errorf("not an MSX program CharEdit wrote")
	}
	p := b[i+len(msxMark):]
	if len(p) < 6 || p[0] != 1 || p[1] > 2 {
		return nil, msxSetup{}, nil, fmt.Errorf("an MSX program of a later CharEdit")
	}
	s := newMSXSetup(int(p[1]))
	s.fg, s.bg, s.border = int(p[2]&15), int(p[3]&15), int(p[4]&15)
	own := p[5]&1 != 0
	p = p[6:]
	cols := s.info().cols
	n := cols * msxRows
	need := n + 32
	if own {
		need += 2048
	}
	if len(p) < need {
		return nil, msxSetup{}, nil, fmt.Errorf("the program is cut short")
	}
	d := newMSXDoc(s.mode)
	for k := 0; k < n; k++ {
		d.cells[k] = makeCell(int(p[k]), 0, 0, 0)
	}
	copy(s.ct[:], p[n:n+32])
	p = p[n+32:]
	var font []byte
	if own {
		font = append([]byte(nil), p[:2048]...)
	}
	return d, s, font, nil
}

// msxProgramFont is the font a program carries: the page's own, none for the
// machine's (C-BIOS's sets and the BIOS ROM's stand for the MSX's own).
func (a *App) msxProgramFont() []byte {
	f := a.ownInUse()
	if f == nil {
		return nil
	}
	out := make([]byte, 2048)
	for c := 0; c < 256; c++ {
		for r := 0; r < 8; r++ {
			out[c*8+r] = f.row(c, r*f.rows()/8)
		}
	}
	return out
}

// ---------------------------------------------------------------- the BIOS ROM's font

// msxROMPlaces are where an MSX BIOS ROM may be: CharEdit's data folder (put it
// there yourself: the MSX's ROMs may not be passed on).
func msxROMPlaces() []string {
	var out []string
	for _, n := range []string{"msx.rom", "MSX.ROM", "msx1.rom", "MSX1.ROM", "msx2.rom", "MSX2.ROM"} {
		out = append(out, filepath.Join(appDataDir(), n))
	}
	return out
}

// msxROMFont takes the character set out of a 32 KB BIOS ROM: CGTABL at
// 0x0004 points at it. Space must be empty and 'A' not.
func msxROMFont(rom []byte) []byte {
	if len(rom) != 32768 {
		return nil
	}
	at := int(binary.LittleEndian.Uint16(rom[4:]))
	if at+2048 > len(rom) {
		return nil
	}
	f := rom[at : at+2048]
	if !bytes.Equal(f[32*8:33*8], make([]byte, 8)) || bytes.Equal(f[65*8:66*8], make([]byte, 8)) {
		return nil
	}
	return f
}

// loadMSXROMFont adds "MSX (BIOS ROM)" when a ROM is found; it gives its path.
func loadMSXROMFont() string {
	if f := fontByID("msxrom"); f != nil {
		return f.src
	}
	for _, p := range msxROMPlaces() {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if data := msxROMFont(b); data != nil {
			f := newFont("msxrom", "MSX (BIOS ROM)", "msx", data, 8, 256)
			f.src = p
			fonts = append(fonts, f)
			return p
		}
	}
	return ""
}

// ---------------------------------------------------------------- Playscii

// Playscii's "msx" charset holds the codes 32-255 and then 0-31.
func psciMSXIndex(c int) int { return (c - 32) & 255 }
func msxOfPsci(i int) int    { return (i + 32) & 255 }

// psciMSXColours is Playscii's "msx" palette, colours 1-15 (its own order).
var psciMSXColours = []uint32{0x000000, 0xFF2020, 0xFF6D6D, 0x20DB20, 0xDBDB20, 0x6DFF6D, 0xDBDB92, 0x2020FF, 0x209220,
	0x306DFF, 0xDB49B6, 0xB62020, 0xB6B6B6, 0x49DBFF, 0xFFFFFF}

// psciOfMSX and msxOfPsciColour turn a TMS9918 colour into Playscii's and back
// (by the nearest colour of the MSX2 palette, which Playscii's is close to).
func psciOfMSX(c int) int {
	if c <= 1 {
		return 1
	}
	return psciNearest(msx2Palette.colors[c]&0xFFFFFF, psciMSXColours)
}

func msxOfPsciColour(i int) int {
	if i <= 1 || i > len(psciMSXColours) {
		return 1
	}
	best, bd := 1, 1<<30
	for c := 2; c < 16; c++ {
		if d := rgbDist(msx2Palette.colors[c], 0xFF000000|psciMSXColours[i-1]); d < bd {
			best, bd = c, d
		}
	}
	return best
}

// writePsciMSX writes the frames with Playscii's msx charset and palette, each
// cell in the colours the screen gives it.
func writePsciMSX(docs []*Doc, holds []int, s msxSetup) []byte {
	var frames []*Doc
	cells := map[*Doc][]psciTile{}
	w, h := s.info().cols, msxRows
	for _, d := range docs {
		var t []psciTile
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				ch := cellCh(d.get(x, y))
				fg, bg := s.colours(ch)
				if fg == 0 {
					fg = s.backdrop()
				}
				if bg == 0 {
					bg = s.backdrop()
				}
				t = append(t, psciTile{Char: float64(psciMSXIndex(ch)), Fg: float64(psciOfMSX(fg)), Bg: float64(psciOfMSX(bg))})
			}
		}
		frames = append(frames, d)
		cells[d] = t
	}
	f := psciFile{Width: w, Height: h, Charset: "msx", Palette: "msx", Camera: [3]float64{float64(w) / 2, -float64(h) / 2, float64(w) * 0.9}}
	for i, d := range frames {
		hold := 100
		if i < len(holds) && holds[i] > 0 {
			hold = holds[i]
		}
		one := 1.0
		f.Frames = append(f.Frames, psciFrame{Delay: float64(hold) / 1000, Layers: []psciLayer{{Name: "Layer 1", Visible: &one, Tiles: cells[d]}}})
	}
	out, _ := json.MarshalIndent(f, "", " ")
	return out
}

// msxPsciTop puts a Playscii frame's layers together (by z, visible ones):
// the top tile of each cell, ch -1 where none has a character.
func msxPsciTop(f *psciFile, fr psciFrame) []psciCell {
	w, h := f.Width, f.Height
	top := make([]psciCell, w*h)
	for i := range top {
		top[i] = psciCell{-1, 0, 0}
	}
	layers := append([]psciLayer{}, fr.Layers...)
	for i := 1; i < len(layers); i++ {
		for j := i; j > 0 && layers[j].Z < layers[j-1].Z; j-- {
			layers[j], layers[j-1] = layers[j-1], layers[j]
		}
	}
	for _, l := range layers {
		if l.Visible != nil && *l.Visible == 0 {
			continue
		}
		for i, tl := range l.Tiles[:min(len(l.Tiles), w*h)] {
			ch, fg, bg := int(tl.Char), int(tl.Fg), int(tl.Bg)
			switch m := &top[i]; {
			case bg > 0:
				if fg <= 0 {
					ch = -1
				}
				*m = psciCell{ch, fg, bg}
			case fg > 0 && ch > 0:
				m.ch, m.fg = ch, fg
			}
		}
	}
	return top
}

// psciMSX reads a Playscii picture in the msx charset: 32 columns or fewer is
// SCREEN 1 (each group of 8 characters in its commonest colours), else SCREEN
// 0 (40 or 80 columns, the commonest colours of all).
func psciMSX(f *psciFile) *psciArt {
	mode := 0
	switch {
	case f.Width <= 32:
		mode = 1
	case f.Width > 40:
		mode = 2
	}
	s := newMSXSetup(mode)
	cols := s.info().cols
	count := map[[2]int]int{}
	group := map[int]map[byte]int{}
	art := &psciArt{mode: "msx"}
	for fi, fr := range f.Frames {
		if fi >= 1000 {
			break
		}
		top := msxPsciTop(f, fr)
		d := newMSXDoc(mode)
		for y := 0; y < min(f.Height, msxRows); y++ {
			for x := 0; x < min(f.Width, cols); x++ {
				t := top[y*f.Width+x]
				if t.ch < 0 {
					continue
				}
				ch := msxOfPsci(t.ch)
				d.cells[y*cols+x] = makeCell(ch, 0, 0, 0)
				fg, bg := msxOfPsciColour(t.fg), msxOfPsciColour(t.bg)
				count[[2]int{fg, bg}]++
				if group[ch>>3] == nil {
					group[ch>>3] = map[byte]int{}
				}
				group[ch>>3][byte(fg<<4|bg)]++
			}
		}
		art.docs = append(art.docs, d)
		art.holds = append(art.holds, int(math.Round(math.Min(math.Max(fr.Delay, 0), 3600)*1000)))
	}
	if len(art.docs) == 0 {
		art.docs, art.holds = []*Doc{newMSXDoc(mode)}, []int{100}
	}
	best := 0
	for k, n := range count {
		if n > best || n == best && (k[0] < s.fg || k[0] == s.fg && k[1] < s.bg) {
			best, s.fg, s.bg = n, k[0], k[1]
		}
	}
	s.border = s.bg
	for g, m := range group {
		best := 0
		for v, n := range m {
			if n > best || n == best && v < s.ct[g] {
				best, s.ct[g] = n, v
			}
		}
	}
	art.msx = &s
	if f.Width > cols || f.Height > msxRows {
		art.note = fmt.Sprintf("%dx%d: the MSX screen shows %dx%d", f.Width, f.Height, cols, msxRows)
	}
	return art
}

// ---------------------------------------------------------------- in CharEdit

// loadMSX opens an MSX .rom or .bin CharEdit wrote; false if b is neither.
func (a *App) loadMSX(b []byte, name, path, as string) bool {
	if as != "auto" && as != "msx" {
		return false
	}
	kind := ""
	switch {
	case isMSXROM(b):
		kind = "msxrom"
	case isMSXBin(b):
		kind = "msxbin"
	}
	_, _, font, err := readMSXProgram(b)
	if kind == "" || err != nil {
		if as == "auto" {
			return false // another machine's .bin, or an MSX program of someone else's
		}
		if kind != "" {
			a.msg("Could not open " + name + ": " + err.Error())
			return true
		}
		// Open as MSX: text in the MSX's characters (code 1 + a letter for 0-31).
		a.srcKind, a.ownFont, a.ownLower, a.ownPal = "msxtext", nil, nil, nil
		a.mode = "msx"
		a.msxEnsure()
		a.finishLoad(b, name, path, nil, "utf8")
		return true
	}
	a.srcKind, a.ownFont, a.ownLower, a.ownPal = kind, nil, nil, nil
	a.mode = "msx"
	a.finishLoad(b, name, path, nil, kind)
	if font != nil {
		f := newFont(ownID, strings.TrimSuffix(name, filepath.Ext(name))+" (font)", "msx", font, 8, 256)
		f.src, f.format = name, "msx"
		a.useOwnFont(f, nil, false)
	}
	return true
}

// reparseMSX reads the page again from its program; false if it has none.
func (a *App) reparseMSX() bool {
	if a.srcKind == "msxtext" {
		a.doc = msxText(a.source, a.msxSet.mode)
		a.setPalette()
		a.cx, a.cy, a.sel, a.float = 0, 0, nil, nil
		return true
	}
	if a.srcKind != "msxrom" && a.srcKind != "msxbin" {
		return false
	}
	d, s, _, err := readMSXProgram(a.source)
	if err != nil {
		d, s = newMSXDoc(a.msxSet.mode), newMSXSetup(a.msxSet.mode)
	}
	a.doc, a.msxSet = d, s
	a.setPalette()
	a.cx, a.cy, a.sel, a.float = 0, 0, nil, nil
	a.msxBrushColours()
	return true
}

// msxText puts text in the MSX's characters on a screen: CR, LF and CR LF end
// lines, code 1 + 0x40-0x5F is a character 0-31 (as MSX BASIC prints them),
// other control codes are left out; what is past the screen is cut.
func msxText(b []byte, mode int) *Doc {
	d := newMSXDoc(mode)
	x, y := 0, 0
	for i := 0; i < len(b) && y < msxRows; i++ {
		c := int(b[i])
		switch {
		case c == '\r' || c == '\n':
			if c == '\r' && i+1 < len(b) && b[i+1] == '\n' {
				i++
			}
			x, y = 0, y+1
			continue
		case c == 1 && i+1 < len(b) && b[i+1] >= 0x40 && b[i+1] < 0x60:
			i++
			c = int(b[i]) - 0x40
		case c < 32 || c == 0x7F:
			continue
		}
		if x < d.cols {
			d.cells[y*d.cols+x] = makeCell(c, 0, 0, 0)
		}
		x++
	}
	return d
}

// encodeMSX writes an MSX page's own formats; ok false for other formats.
func (a *App) encodeMSX(fmtID string, d *Doc) (out []byte, ok bool, err error) {
	if !a.msx() {
		return nil, false, nil
	}
	switch fmtID {
	case "msxrom":
		return writeMSXROM(msxDataOf(d, a.msxSet, a.msxProgramFont())), true, nil
	case "msxbin":
		return writeMSXBin(msxDataOf(d, a.msxSet, a.msxProgramFont())), true, nil
	}
	return nil, false, nil
}

// ---------------------------------------------------------------- the colours in the panel

// drawMSXColours draws the 16 colours: left click is the foreground, right
// click the background (of the screen in SCREEN 0, of the brush's group of 8
// characters in SCREEN 1), middle click the border (SCREEN 1). It returns the
// y under them.
func (a *App) drawMSXColours(u *UI, x, y, perRow int) int {
	fb := u.fb
	s := a.msxSet
	fg, bg := s.colours(a.brush)
	what := "the screen"
	if s.mode == 1 {
		what = fmt.Sprintf("characters %d-%d (the brush's group)", a.brush&^7, a.brush|7)
	}
	for i := 0; i < 16; i++ {
		r := Rect{x + i%perRow*20, y + i/perRow*26, 18, 18}
		if i == 0 { // transparent: the backdrop shows through
			fb.fill(r, a.msxRGB(0))
			for k := 3; k < r.w-3; k++ { // a stroke across: see-through
				fb.fill(Rect{r.x + k, r.y + r.h - 1 - k, 1, 1}, cDim)
			}
		} else {
			fb.fill(r, a.pal[i])
		}
		fb.frame(r, cEdgeDk)
		if i == fg {
			fb.frame(r.inset(-2), cHi)
		}
		if i == bg {
			fb.dashed(r.inset(-2), cOrange)
		}
		if s.mode == 1 && i == s.border {
			fb.fill(Rect{r.x + 4, r.y + r.h + 3, r.w - 8, 2}, cHi)
		}
		hint := fmt.Sprintf("Colour %d: %s. Left: foreground, right: background of %s", i, msxColourNames[i], what)
		if s.mode == 1 {
			hint += "; middle: the border"
		}
		u.setHint(r, hint)
		if c := u.take(r); c != nil {
			switch {
			case c.button == 2 && s.mode == 1:
				a.setMSXBorder(i)
			case c.button == 3:
				a.msxSetColour(true, i)
			default:
				a.msxSetColour(false, i)
			}
		}
	}
	return y + (15/perRow+1)*26 - 8
}

// msxAutosave is an MSX page's screen in an autosave.
type msxAutosave struct {
	Mode   int    `json:"mode"`
	Fg     int    `json:"fg"`
	Bg     int    `json:"bg"`
	Border int    `json:"border"`
	CT     []byte `json:"colour_table"`
}

func (t *Tab) msxAutosave() *msxAutosave {
	s := t.msxSet
	return &msxAutosave{Mode: s.mode, Fg: s.fg, Bg: s.bg, Border: s.border, CT: append([]byte(nil), s.ct[:]...)}
}

func (m msxAutosave) setup() msxSetup {
	s := newMSXSetup(m.Mode)
	s.fg, s.bg, s.border = m.Fg&15, m.Bg&15, m.Border&15
	copy(s.ct[:], m.CT)
	return s
}
