package main

// Amstrad CPC pages ("AMSCII", the CPC's text art): the CPC's text screen in its
// three modes. A cell is a character of the ROM's 256 (0-31 are glyphs too,
// printed after code 1) with a pen and a paper; pens and papers are inks, and
// each ink is one of the CPC's 27 colours (the firmware's numbers 0-26). The
// screen has a border colour of its own. Mode 1 (40x25, 4 inks) is the AMSCII
// compos' main category, mode 0 (20x25, 16 inks) and mode 2 (80x25, 2 inks) the
// others (logiker.com/AMSCII-Compo-Rules).
//
// Files: the .bin the compos ask for, as Logiker's Amstrad ASCII Exporter for
// Playscii writes it: an AMSDOS header, a little Z80 program and the bytes it
// prints through the firmware's TXT OUTPUT (mode, border, inks, then every cell
// with pen/paper codes). CharEdit reads the exporter's files (both of its
// programs) and any such stream of control codes; it writes its own program
// (cpcStub), which prints a counted stream. A .dsk (an Extended CPC disk image
// in AMSDOS's DATA format) holds the same .bin; Playscii's .psci with the
// exporter's CPC_Mode0/1/2 charsets and "cpc" palette (editorformats.go).
//
// The font is the CPC firmware ROM's (fonts/amstrad-cpc-NOTICE.txt: Amstrad
// allows passing it on). Mode 0 cells are 16x8 pixels (wide pixels), mode 1 8x8,
// mode 2 8x16 (a mode 2 pixel is half as wide as tall), so every mode shows in
// the CPC's own shape.

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
	"path/filepath"
	"strings"
)

//go:embed fonts/amstrad-cpc-charset.font
var cpcROMFont []byte

var cpcFont = func() *Font {
	bits := make([]byte, 4096)
	for c := 0; c < 256; c++ {
		for r := 0; r < 8; r++ {
			v := cpcROMFont[c*8+r]
			bits[c*16+r*2], bits[c*16+r*2+1] = v, v
		}
	}
	return &Font{id: "cpc", name: "Amstrad CPC", kind: "cpc", syncterm: -1, bits: bits}
}()

func init() {
	fonts = append(fonts, cpcFont)
	mirrorPairs["cpc"] = asciiPairs
	editorOwn["cpcbin"], editorOwn["cpcdsk"] = true, true
	// The page type in the lists that name every page type.
	pageModes = append(pageModes, "cpc")
	modeOrder = append(modeOrder, "cpc")
	modeNames["cpc"] = "Amstrad CPC"
	openAs = append(openAs, "cpc")
	openAsNames = append(openAsNames, "Amstrad CPC")
	modeNoStream["cpc"] = "an Amstrad CPC screen: a picture of the screen, no stream to draw in order"
	// The exports every page has (formats in editor.go list the older page types).
	for i, f := range formats {
		switch f.id {
		case "utf8", "png", "png2", "gif", "apng", "psci":
			formats[i].modes += " cpc"
		}
	}
}

func (a *App) cpc() bool { return a.mode == "cpc" }

// cpcColour is firmware colour n (0-26): green, red and blue each off, half or full
// (n = 9 green + 3 red + blue), as the Gate Array makes them.
func cpcColour(n int) uint32 {
	n = max(0, min(26, n))
	lv := [3]uint32{0x00, 0x80, 0xFF}
	return 0xFF000000 | lv[n/3%3]<<16 | lv[n/9]<<8 | lv[n%3]
}

// cpcColourNames are the firmware colours' names, as the CPC's manual gives them.
var cpcColourNames = [27]string{"Black", "Blue", "Bright blue", "Red", "Magenta", "Mauve", "Bright red", "Purple",
	"Bright magenta", "Green", "Cyan", "Sky blue", "Yellow", "White", "Pastel blue", "Orange", "Pink", "Pastel magenta",
	"Bright green", "Sea green", "Bright cyan", "Lime", "Pastel green", "Pastel cyan", "Bright yellow", "Pastel yellow",
	"Bright white"}

// A CPC screen mode.
type cpcModeInfo struct {
	cols, inks int
	name       string
}

var cpcModes = [3]cpcModeInfo{
	{20, 16, "Mode 0 (20x25, 16 inks)"},
	{40, 4, "Mode 1 (40x25, 4 inks)"},
	{80, 2, "Mode 2 (80x25, 2 inks)"},
}

const cpcRows = 25

// cpcStartInks are the inks as the firmware sets them at power-on (14 and 15
// flash: their first colour); the border starts blue (1).
var cpcStartInks = [16]int{1, 24, 20, 6, 26, 0, 2, 8, 10, 12, 14, 16, 18, 22, 1, 16}

// cpcSetup is a CPC page's screen: its mode, its inks' colours and the border.
// It belongs to the page (all its frames), and undo keeps it (Snap.cpc).
type cpcSetup struct {
	ok     bool // set up (a Tab of another type has none)
	mode   int
	ink    [16]int // firmware colours 0-26
	border int
}

func newCPCSetup(mode int) cpcSetup {
	return cpcSetup{ok: true, mode: mode, ink: cpcStartInks, border: 1}
}

func (s cpcSetup) info() cpcModeInfo { return cpcModes[max(0, min(2, s.mode))] }

// mask keeps a pen or paper to the mode's inks, as the firmware does.
func (s cpcSetup) mask(i int) int { return i & (s.info().inks - 1) }

// cpcSnap is the CPC screen for an undo step (nil on other pages).
func (a *App) cpcSnap() *cpcSetup {
	if !a.cpc() {
		return nil
	}
	s := a.cpcSet
	return &s
}

// cpcEnsure gives a page that becomes a CPC page the CPC's start-up screen.
func (t *Tab) cpcEnsure() {
	if !t.cpcSet.ok {
		t.cpcSet = newCPCSetup(1)
	}
}

func cpcBlank() uint32 { return makeCell(32, 1, 0, 0) }

// newCPCDoc is an empty screen of a mode: spaces in pen 1 on paper 0.
func newCPCDoc(mode int) *Doc {
	d := newDoc(cpcModes[max(0, min(2, mode))].cols, cpcRows)
	fillCells(d.cells, cpcBlank())
	return d
}

// cpcPalette is the page's colours: ink i is pal[i].
func (s cpcSetup) palette() [16]uint32 {
	var p [16]uint32
	for i := range p {
		p[i] = cpcColour(s.ink[i])
	}
	return p
}

// borderPx is the colour around a machine page's picture (exports).
func (a *App) borderPx(d *Doc) uint32 {
	if a.cpc() {
		return cpcColour(a.cpcSet.border)
	}
	if a.atari() {
		return a.atariRGB(d.border)
	}
	return a.pal[d.borderColour()]
}

// cpcCellW and cpcCellH are a cell's size in pixels in each mode.
func (a *App) cpcCellW() int {
	if a.cpcSet.mode == 0 {
		return 16
	}
	return 8
}

func (a *App) cpcCellH() int {
	if a.cpcSet.mode == 2 {
		return 16
	}
	return 8
}

// cpcWide doubles each bit of a glyph row: a mode 0 pixel is two wide.
var cpcWide = func() (t [256]int) {
	for b := 0; b < 256; b++ {
		for i := 0; i < 8; i++ {
			if b&(1<<i) != 0 {
				t[b] |= 3 << (2 * i)
			}
		}
	}
	return
}()

// setCPCMode changes a CPC page's mode: its width (every frame) and inks; one undo step.
func (a *App) setCPCMode(m int) {
	m = max(0, min(2, m))
	if m == a.cpcSet.mode {
		return
	}
	a.endPlay()
	cols := cpcModes[m].cols
	cut := false
	for y := 0; y < a.doc.rows; y++ {
		if a.doc.lastCol(y) >= cols {
			cut = true
		}
	}
	a.withEveryFrame(func(d *Doc) *Doc {
		n := newCPCDoc(m)
		n.ensureRows(d.rows)
		for y := 0; y < d.rows; y++ {
			for x := 0; x < min(cols, d.cols); x++ {
				n.cells[y*cols+x] = d.cells[y*d.cols+x]
			}
		}
		return n
	})
	a.cpcSet.mode = m
	a.cx = min(a.cx, cols-1)
	n := cpcModes[m].inks
	a.fg, a.bg = a.fg%n, a.bg%n
	if a.fg == a.bg {
		a.fg, a.bg = 1%n, 0
	}
	if cut {
		a.msg(fmt.Sprintf("Mode %d has %d columns: what was further right is cut (%s undoes it)", m, cols, keyName("undo")))
	} else {
		a.msg(cpcModes[m].name)
	}
}

// setCPCInk gives ink i firmware colour c (undo keeps the old one).
func (a *App) setCPCInk(i, c int) {
	if a.cpcSet.ink[i] == c {
		return
	}
	a.pushUndo()
	a.cpcSet.ink[i] = c
	a.setPalette()
}

func (a *App) setCPCBorder(c int) {
	if a.cpcSet.border == c {
		return
	}
	a.pushUndo()
	a.cpcSet.border = c
}

// ---------------------------------------------------------------- characters

// cpcRunes is what each character looks like in Unicode (en.wikipedia.org,
// Amstrad CPC character set); 0xEF, 0xFC and 0xFD have no code point of their
// own and get look-alikes.
var cpcRunes = []rune("◻⎾⏊⏌⚡⊠✓⍾←→↓↑↡↲⊗⊙⊟◷◶◵◴⍻⎍⊣⧖⍿␦⊖◰◱◲◳" +
	" !\"#$%&'()*+,-./0123456789:;<=>?@ABCDEFGHIJKLMNOPQRSTUVWXYZ[\\]↑_`abcdefghijklmnopqrstuvwxyz{|}~🮖" +
	" ▘▝▀▖▌▞▛▗▚▐▜▄▙▟█·╵╶└╷│┌├╴┘─┴┐┤┬┼" +
	"^´¨£©¶§‘¼½¾±÷¬¿¡αβγδεθλμπσφψχωΣΩ" +
	"🮠🮡🮣🮢🮧🮥🮦🮤🮨🮩🮮╳╱╲🮕▒▔▕▁▏◤◥◢◣🮎🮍🮏🮌🮜🮝🮞🮟" +
	"☺☹♣♦♥♠○●□■♂♀♩♪☼🚀⭡⭣⭠⭢▲▼▶◀🯆🯅🯇🯈⌛☂⭥⭤")

// cpcFromRuneMap is the character of a rune; '^' is the circumflex (0xA0), as
// on the screen, the CPC's 0x5E being an arrow.
var cpcFromRuneMap = func() map[rune]int {
	m := map[rune]int{}
	for c, r := range cpcRunes {
		if _, ok := m[r]; !ok {
			m[r] = c
		}
	}
	m['^'] = 0xA0
	m['↑'] = 0x5E
	m['’'] = 0x27
	m['¦'] = 0x7C
	return m
}()

func cpcRune(c int) rune {
	if c >= 0 && c < len(cpcRunes) {
		return cpcRunes[c]
	}
	return ' '
}

func cpcFromRune(r rune) int {
	if r >= 0x20 && r < 0x7F && r != '^' {
		return int(r)
	}
	if c, ok := cpcFromRuneMap[r]; ok {
		return c
	}
	return -1
}

var cpcFkeys = [][]int{
	{0x8F, 0x8C, 0x83, 0x85, 0x8A, 0xCF, 0x7F, 0xCE, 0x90, 0x80}, // blocks and shades
	{0x81, 0x82, 0x84, 0x88, 0x86, 0x89, 0x87, 0x8B, 0x8D, 0x8E}, // quarter blocks
	{0xD4, 0xD5, 0xD6, 0xD7, 0xD0, 0xD1, 0xD2, 0xD3, 0xCC, 0xCD}, // triangles and edges
	{0x96, 0x9C, 0x93, 0x99, 0x9A, 0x95, 0x97, 0x9D, 0x9E, 0x9B}, // lines
	{0xC0, 0xC1, 0xC2, 0xC3, 0xD8, 0xD9, 0xDA, 0xDB, 0xE7, 0xE9}, // slopes, halves, dots
}

// ---------------------------------------------------------------- the .bin stream

// TXT OUTPUT's control codes and how many bytes follow each (the CPC 464
// Firmware Guide, SOFT 968, chapter 4).
var cpcParams = [32]int{0, 1, 0, 0, 1, 1, 0, 0, 0, 0, 0, 0, 0, 0, 1, 1, 0, 0, 0, 0, 0, 0, 1, 1, 0, 9, 4, 0, 3, 2, 0, 2}

// cpcScreen is a CPC text screen being printed on (the firmware's text VDU, one
// window the size of the screen).
type cpcScreen struct {
	d           *Doc
	s           cpcSetup
	x, y        int
	pen, paper  int
	transparent bool
	off         bool         // code 21 turned printing off
	notes       map[int]bool // codes CharEdit does not show (SYMBOL, windows, graphics, flashing inks)
}

func newCPCScreen() *cpcScreen {
	return &cpcScreen{d: newCPCDoc(1), s: newCPCSetup(1), pen: 1, notes: map[int]bool{}}
}

// fit moves the cursor back on the screen as the firmware does before printing:
// past the right edge to the next line, past the bottom with a scroll.
func (c *cpcScreen) fit() {
	cols := c.d.cols
	if c.x >= cols {
		c.x, c.y = 0, c.y+1
	}
	if c.x < 0 {
		c.x, c.y = cols-1, c.y-1
	}
	for c.y >= cpcRows {
		c.scroll(1)
		c.y--
	}
	for c.y < 0 {
		c.scroll(-1)
		c.y++
	}
}

// scroll moves the screen up one line (dir 1) or down (-1); the new line is paper.
func (c *cpcScreen) scroll(dir int) {
	cols := c.d.cols
	if dir > 0 {
		copy(c.d.cells, c.d.cells[cols:cpcRows*cols])
		c.clear(cols*(cpcRows-1), cols*cpcRows)
	} else {
		copy(c.d.cells[cols:cpcRows*cols], c.d.cells[:(cpcRows-1)*cols])
		c.clear(0, cols)
	}
}

func (c *cpcScreen) clear(from, to int) {
	for i := max(0, from); i < min(to, len(c.d.cells)); i++ {
		c.d.cells[i] = makeCell(32, c.pen, c.paper, 0)
	}
}

func (c *cpcScreen) put(ch int) {
	if c.off {
		return
	}
	c.fit()
	i := c.y*c.d.cols + c.x
	paper := c.paper
	if c.transparent { // printed over what is there: shown as the new character on the old paper
		paper = cellBg(c.d.cells[i])
	}
	c.d.cells[i] = makeCell(ch, c.pen, paper, 0)
	c.x++
}

// out prints a control code with its parameters, or a character.
func (c *cpcScreen) out(code int, p []byte) {
	mask := func(v byte) int { return c.s.mask(int(v)) }
	switch code {
	case 1:
		c.put(int(p[0]))
	case 4:
		m := int(p[0] & 3)
		if m == 3 {
			m = 0 // the Gate Array's mode 3 is not the firmware's: taken as 0
		}
		c.s.mode = m
		c.d = newCPCDoc(m)
		c.pen, c.paper = mask(byte(c.pen)), mask(byte(c.paper))
		c.clear(0, len(c.d.cells))
		c.x, c.y = 0, 0
	case 5, 23:
		c.notes[5] = true
	case 6:
		c.off = false
	case 21:
		c.off = true
	case 8:
		c.x--
		if c.x < 0 {
			c.fit()
		}
	case 9:
		c.x++
	case 10:
		c.y++
		if c.y >= cpcRows {
			c.fit()
		}
	case 11:
		c.y--
		if c.y < 0 {
			c.fit()
		}
	case 12:
		c.clear(0, len(c.d.cells))
		c.x, c.y = 0, 0
	case 13:
		c.x = 0
	case 14:
		c.paper = mask(p[0])
	case 15:
		c.pen = mask(p[0])
	case 16:
		c.fit()
		c.clear(c.y*c.d.cols+c.x, c.y*c.d.cols+c.x+1)
	case 17, 18, 19, 20:
		c.fit()
		at, row := c.y*c.d.cols+c.x, c.y*c.d.cols
		switch code {
		case 17:
			c.clear(row, at+1)
		case 18:
			c.clear(at, row+c.d.cols)
		case 19:
			c.clear(0, at+1)
		case 20:
			c.clear(at, len(c.d.cells))
		}
	case 22:
		c.transparent = p[0]&1 != 0
		if c.transparent {
			c.notes[22] = true
		}
	case 24:
		c.pen, c.paper = c.paper, c.pen
	case 25, 26:
		c.notes[code] = true
	case 28:
		c.s.ink[p[0]&15] = min(26, int(p[1]&31))
		if p[1] != p[2] {
			c.notes[28] = true
		}
	case 29:
		c.s.border = min(26, int(p[0]&31))
		if p[0] != p[1] {
			c.notes[28] = true
		}
	case 30:
		c.x, c.y = 0, 0
	case 31:
		c.x, c.y = int(p[0])-1, int(p[1])-1
	}
}

// print runs a stream of bytes through the screen. With program, a 0x1F
// (LOCATE) where a code is due ends it: Logiker's exporter ends its streams so,
// and its programs stop there; in a bare stream it is LOCATE.
func (c *cpcScreen) print(b []byte, program bool) {
	for i := 0; i < len(b); {
		code := int(b[i])
		i++
		if code >= 32 {
			c.put(code)
			continue
		}
		if code == 0x1F && program {
			return
		}
		n := cpcParams[code]
		if i+n > len(b) {
			return
		}
		c.out(code, b[i:i+n])
		i += n
	}
}

// note says in words what the stream did that the page cannot show, or "".
func (c *cpcScreen) note() string {
	var out []string
	for _, k := range []struct {
		code int
		what string
	}{{25, "redefines characters (SYMBOL): shown in the ROM's"}, {26, "sets text windows: printed on the whole screen"},
		{22, "prints transparently: the top character shows"}, {28, "has flashing inks: their first colour shows"},
		{5, "draws with graphics codes: left out"}} {
		if c.notes[k.code] {
			out = append(out, k.what)
		}
	}
	return strings.Join(out, "; ")
}

// cpcStream is the page as TXT OUTPUT codes: the mode, the border, every ink
// of the mode, then the 25 rows cell by cell, a paper or pen code where it
// changes (a space needs no pen, a full block no paper), code 1 before
// characters 0-31. This is the exporter's layout, all inks set first.
func cpcStream(d *Doc, s cpcSetup) []byte {
	m := s.info()
	out := []byte{4, byte(s.mode), 29, byte(s.border), byte(s.border)}
	for i := 0; i < m.inks; i++ {
		out = append(out, 28, byte(i), byte(s.ink[i]), byte(s.ink[i]))
	}
	pen, paper := -1, -1
	for y := 0; y < cpcRows; y++ {
		for x := 0; x < m.cols; x++ {
			v := cpcBlank()
			if x < d.cols && y < d.rows {
				v = d.get(x, y)
			}
			ch, fg, bg := cellCh(v), s.mask(cellFg(v)), s.mask(cellBg(v))
			if bg != paper && ch != 0x8F {
				out, paper = append(out, 14, byte(bg)), bg
			}
			if fg != pen && ch != 32 && ch != 0x80 {
				out, pen = append(out, 15, byte(fg)), fg
			}
			if ch < 32 {
				out = append(out, 1)
			}
			out = append(out, byte(ch))
		}
	}
	return out
}

// cpcStub is CharEdit's program for a .bin, loaded and run at 0x1000: it prints
// the stream after it through TXT OUTPUT (0xBB5A), counted, then waits for a key
// (KM WAIT CHAR, 0xBB06) and returns.
//
//	1000 21 15 10   LD HL,1015h     ; the stream
//	1003 01 nn nn   LD BC,length
//	1006 7E         LD A,(HL)
//	1007 23         INC HL
//	1008 C5         PUSH BC
//	1009 CD 5A BB   CALL TXT OUTPUT
//	100C C1         POP BC
//	100D 0B         DEC BC
//	100E 78         LD A,B
//	100F B1         OR C
//	1010 20 F4      JR NZ,1006h
//	1012 C3 06 BB   JP KM WAIT CHAR
func cpcStub(n int) []byte {
	return []byte{0x21, 0x15, 0x10, 0x01, byte(n), byte(n >> 8), 0x7E, 0x23, 0xC5, 0xCD, 0x5A, 0xBB, 0xC1,
		0x0B, 0x78, 0xB1, 0x20, 0xF4, 0xC3, 0x06, 0xBB}
}

const cpcLoad = 0x1000

// cpcFileName is a name for AMSDOS: 8 letters or digits, upper case.
func cpcFileName(path string) string {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	var n []byte
	for _, r := range strings.ToUpper(base) {
		if (r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') && len(n) < 8 {
			n = append(n, byte(r))
		}
	}
	if len(n) == 0 {
		return "AMSCII"
	}
	return string(n)
}

// amsdosHeader is the 128 bytes before a file on an AMSDOS disk
// (cpcwiki.eu/index.php/AMSDOS_Header): name, type 2 (binary), load and entry
// address, length, and the checksum of bytes 0-66.
func amsdosHeader(name string, length, load, entry int) []byte {
	h := make([]byte, 128)
	copy(h[1:12], fmt.Sprintf("%-8s%s", name, "BIN"))
	h[18] = 2
	h[19], h[20] = byte(length), byte(length>>8)
	h[21], h[22] = byte(load), byte(load>>8)
	h[24], h[25] = byte(length), byte(length>>8)
	h[26], h[27] = byte(entry), byte(entry>>8)
	h[64], h[65], h[66] = byte(length), byte(length>>8), byte(length>>16)
	sum := 0
	for _, v := range h[:67] {
		sum += int(v)
	}
	h[67], h[68] = byte(sum), byte(sum>>8)
	return h
}

// writeCPCBin is the page as a .bin that runs on a CPC (RUN"NAME.BIN") and in
// emulators (dropped on Caprice Forever).
func writeCPCBin(d *Doc, s cpcSetup, name string) []byte {
	data := cpcStream(d, s)
	code := append(cpcStub(len(data)), data...)
	return append(amsdosHeader(name, len(code), cpcLoad, cpcLoad), code...)
}

// amsdosFile is a file with an AMSDOS header: its type, load address and the
// bytes after the header (as long as the header says).
type amsdosFile struct {
	kind, load, entry int
	body              []byte
}

func readAMSDOS(b []byte) (amsdosFile, bool) {
	if len(b) < 128 {
		return amsdosFile{}, false
	}
	sum := 0
	for _, v := range b[:67] {
		sum += int(v)
	}
	if sum&0xFFFF != int(b[67])|int(b[68])<<8 || sum == 0 {
		return amsdosFile{}, false
	}
	n := int(b[24]) | int(b[25])<<8
	if n == 0 || 128+n > len(b) { // a file on a disk is padded to 128 bytes; a short one is cut
		n = len(b) - 128
	}
	return amsdosFile{kind: int(b[18]), load: int(b[21]) | int(b[22])<<8, entry: int(b[26]) | int(b[27])<<8, body: b[128 : 128+n]}, true
}

// cpcArtStream finds the stream a .bin prints: its program loads HL with the
// stream's address and calls TXT OUTPUT (Logiker's exporter's two programs and
// CharEdit's do). For CharEdit's program the count in BC says where it ends.
func cpcArtStream(f amsdosFile) ([]byte, bool) {
	code := f.body
	if f.kind != 2 || !bytes.Contains(code[:min(len(code), 64)], []byte{0xCD, 0x5A, 0xBB}) {
		return nil, false
	}
	for i := 0; i+2 < min(len(code), 40); i++ {
		if code[i] != 0x21 {
			continue
		}
		at := int(code[i+1]) | int(code[i+2])<<8
		off := at - f.load
		if off <= i+2 || off >= len(code) {
			continue
		}
		data := code[off:]
		if i+5 < len(code) && code[i+3] == 0x01 { // LD BC,length: CharEdit's
			if n := int(code[i+4]) | int(code[i+5])<<8; n <= len(data) {
				data = data[:n]
			}
		}
		return data, true
	}
	return nil, false
}

// isCPCBin: an AMSDOS binary that prints a stream of text codes.
func isCPCBin(b []byte) bool {
	f, ok := readAMSDOS(b)
	if !ok {
		return false
	}
	_, ok = cpcArtStream(f)
	return ok
}

// parseCPCBin reads a .bin: the screen its stream prints, and a note on what the
// page could not show.
func parseCPCBin(b []byte) (*Doc, cpcSetup, string, error) {
	f, ok := readAMSDOS(b)
	if !ok {
		return nil, cpcSetup{}, "", fmt.Errorf("no AMSDOS header")
	}
	data, ok := cpcArtStream(f)
	if !ok {
		return nil, cpcSetup{}, "", fmt.Errorf("not a program that prints text art")
	}
	c := newCPCScreen()
	c.print(data, true)
	return c.d, c.s, c.note(), nil
}

// ---------------------------------------------------------------- .dsk

// A CPC disk in AMSDOS's DATA format: 40 tracks of 9 sectors of 512 bytes,
// sectors C1-C9, the directory in the first 4 sectors (64 entries), 1 KB blocks
// from block 2. Written as an Extended DSK image (cpcwiki.eu/index.php/Format:DSK_disk_image_file_format).
const (
	dskTracks     = 40
	dskSectors    = 9
	dskSectorSize = 512
	dskTrackSize  = 256 + dskSectors*dskSectorSize
)

// dskInterleave is the order the sectors are on a track, as AMSDOS formats it.
var dskInterleave = []int{0, 5, 1, 6, 2, 7, 3, 8, 4}

// writeDSK is a disk holding one file (name.BIN, with its AMSDOS header).
func writeDSK(name string, file []byte) ([]byte, error) {
	blocks := (len(file) + 1023) / 1024
	if 2+blocks > dskTracks*dskSectors/2 {
		return nil, fmt.Errorf("the file does not fit a disk")
	}
	sectors := make([][]byte, dskTracks*dskSectors) // logical order
	for i := range sectors {
		sectors[i] = bytes.Repeat([]byte{0xE5}, dskSectorSize)
	}
	data := append([]byte{}, file...)
	if r := len(data) % 128; r != 0 {
		data = append(data, bytes.Repeat([]byte{0x1A}, 128-r)...)
	}
	for i := 0; i*dskSectorSize < len(data); i++ {
		copy(sectors[4+i], bytes.Repeat([]byte{0}, dskSectorSize))
		copy(sectors[4+i], data[i*dskSectorSize:])
	}
	dir := make([]byte, 0, 2048)
	records := len(data) / 128
	for ext := 0; ext*128 < records || ext == 0; ext++ {
		e := make([]byte, 32)
		copy(e[1:12], fmt.Sprintf("%-8s%s", name, "BIN"))
		e[12] = byte(ext)
		e[15] = byte(min(128, records-ext*128))
		for k := 0; k < 16; k++ {
			if blk := ext*16 + k; blk < blocks {
				e[16+k] = byte(2 + blk)
			}
		}
		dir = append(dir, e...)
	}
	for i := 0; i < len(dir); i += dskSectorSize {
		copy(sectors[i/dskSectorSize], dir[i:min(len(dir), i+dskSectorSize)])
	}
	out := make([]byte, 0, 256+dskTracks*dskTrackSize)
	head := make([]byte, 256)
	copy(head, "EXTENDED CPC DSK File\r\nDisk-Info\r\n")
	copy(head[0x22:], "CharEdit")
	head[0x30], head[0x31] = dskTracks, 1
	for t := 0; t < dskTracks; t++ {
		head[0x34+t] = dskTrackSize / 256
	}
	out = append(out, head...)
	for t := 0; t < dskTracks; t++ {
		th := make([]byte, 256)
		copy(th, "Track-Info\r\n")
		th[0x10], th[0x14], th[0x15], th[0x16], th[0x17] = byte(t), 2, dskSectors, 0x4E, 0xE5
		for k, s := range dskInterleave {
			si := th[0x18+8*k:]
			si[0], si[2], si[3] = byte(t), byte(0xC1+s), 2
			si[6], si[7] = byte(dskSectorSize&255), byte(dskSectorSize>>8)
		}
		out = append(out, th...)
		for _, s := range dskInterleave {
			out = append(out, sectors[t*dskSectors+s]...)
		}
	}
	return out, nil
}

func isDSK(b []byte) bool {
	return bytes.HasPrefix(b, []byte("EXTENDED CPC DSK File")) || bytes.HasPrefix(b, []byte("MV - CPC"))
}

// dskFile is a file on a disk image.
type dskFile struct {
	name string
	data []byte
}

// readDSK gives the files of a DATA format disk (user 0-15), in directory order.
func readDSK(b []byte) ([]dskFile, error) {
	if !isDSK(b) || len(b) < 256 {
		return nil, fmt.Errorf("not a CPC disk image")
	}
	ext := b[0] == 'E'
	tracks, sides := int(b[0x30]), max(1, int(b[0x31]))
	sec := map[[2]int][]byte{} // (track, sector id) -> bytes, side 0
	off := 256
	for t := 0; t < tracks*sides && t < 204*2; t++ {
		size := int(b[0x32]) | int(b[0x33])<<8
		if ext {
			size = int(b[0x34+t]) * 256
		}
		if size == 0 {
			continue
		}
		if off+256 > len(b) {
			break
		}
		th := b[off : off+256]
		if !bytes.HasPrefix(th, []byte("Track-Info")) {
			break
		}
		p := off + 256
		for k := 0; k < int(th[0x15]) && k < 29; k++ {
			si := th[0x18+8*k : 0x20+8*k]
			n := 128 << min(int(si[3]), 6)
			if ext {
				n = int(si[6]) | int(si[7])<<8
			}
			if p+n > len(b) {
				break
			}
			if th[0x11] == 0 {
				sec[[2]int{int(th[0x10]), int(si[2])}] = b[p : p+n]
			}
			p += n
		}
		off += size
	}
	logical := func(n int) []byte {
		s := sec[[2]int{n / dskSectors, 0xC1 + n%dskSectors}]
		if len(s) < dskSectorSize {
			return make([]byte, dskSectorSize)
		}
		return s[:dskSectorSize]
	}
	if _, ok := sec[[2]int{0, 0xC1}]; !ok {
		return nil, fmt.Errorf("not a disk in AMSDOS's DATA format")
	}
	var dir []byte
	for n := 0; n < 4; n++ {
		dir = append(dir, logical(n)...)
	}
	type part struct {
		ext, rc int
		blocks  []int
	}
	var order []string
	parts := map[string][]part{}
	for e := 0; e < 64; e++ {
		ent := dir[e*32 : e*32+32]
		if ent[0] > 15 {
			continue
		}
		name := make([]byte, 11)
		for i := range name {
			name[i] = ent[1+i] & 0x7F
		}
		key := strings.TrimSpace(string(name[:8])) + "." + strings.TrimSpace(string(name[8:]))
		p := part{ext: int(ent[12]), rc: int(ent[15])}
		for _, blk := range ent[16:32] {
			if blk != 0 {
				p.blocks = append(p.blocks, int(blk))
			}
		}
		if _, ok := parts[key]; !ok {
			order = append(order, key)
		}
		parts[key] = append(parts[key], p)
	}
	var out []dskFile
	for _, key := range order {
		ps := parts[key]
		for i := 1; i < len(ps); i++ { // extents in order
			for j := i; j > 0 && ps[j].ext < ps[j-1].ext; j-- {
				ps[j], ps[j-1] = ps[j-1], ps[j]
			}
		}
		var data []byte
		for _, p := range ps {
			var chunk []byte
			for _, blk := range p.blocks {
				chunk = append(chunk, logical(2*blk)...)
				chunk = append(chunk, logical(2*blk+1)...)
			}
			data = append(data, chunk[:min(len(chunk), p.rc*128)]...)
		}
		out = append(out, dskFile{name: key, data: data})
	}
	return out, nil
}

// dskArt is the first file on a disk that is CPC text art, its name, and how many such files there are.
func dskArt(b []byte) (dskFile, int, error) {
	files, err := readDSK(b)
	if err != nil {
		return dskFile{}, 0, err
	}
	var first dskFile
	n := 0
	for _, f := range files {
		if isCPCBin(f.data) {
			if n == 0 {
				first = f
			}
			n++
		}
	}
	if n == 0 {
		return dskFile{}, 0, fmt.Errorf("no text art program on the disk")
	}
	return first, n, nil
}

// ---------------------------------------------------------------- Playscii

// Playscii with Logiker's exporter: charsets CPC_Mode0, CPC_Mode1, CPC_Mode2
// (glyph i is character i, but glyph 0 is the empty space) or the older "cpc"
// (characters 32-255 from glyph 0), palette "cpc" (colour i+1 is firmware colour
// i). The exporter's .bin starts the inks with the first colours the picture
// uses (row by row, a cell's paper before its pen); so does reading one here.
func isCPCCharset(cs string) bool {
	return cs == "cpc" || strings.HasPrefix(cs, "CPC_Mode")
}

// psciCPC turns Playscii's frames into CPC pages: the visible layers put
// together as parsePsci does (lowest z first; a tile with a background covers
// what is under it, one with only a glyph puts the glyph over).
func psciCPC(f *psciFile) *psciArt {
	art := &psciArt{mode: "cpc"}
	w, h := f.Width, f.Height
	var tops [][]psciCell
	cells := 0
	for _, fr := range f.Frames[:min(len(f.Frames), 10000)] {
		if cells += w * h; cells > maxPageCells {
			break
		}
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
		tops = append(tops, top)
		art.holds = append(art.holds, int(math.Round(math.Min(math.Max(fr.Delay, 0), 3600)*1000)))
	}
	s := newCPCSetup(1)
	switch {
	case strings.HasPrefix(f.Charset, "CPC_Mode"):
		s.mode = int(f.Charset[len(f.Charset)-1]-'0') % 3
	case f.Width <= 20:
		s.mode = 0
	case f.Width > 40:
		s.mode = 2
	}
	glyph := func(i int) (int, bool) {
		if f.Charset == "cpc" {
			return (i + 32) & 255, i < 224
		}
		if i == 0 {
			return 32, true
		}
		return i & 255, i < 256
	}
	colour := func(i int) int { return max(0, min(26, i-1)) }
	if f.Palette != "cpc" {
		pal := psciPalettes[f.Palette]
		colour = func(i int) int {
			if i <= 0 || i > len(pal) {
				return 0
			}
			return cpcNearest(0xFF000000 | pal[i-1])
		}
	}
	// The inks: the colours as they first come.
	n := s.info().inks
	var used []int
	inkOf := func(c int) int {
		for i, u := range used {
			if u == c {
				return i
			}
		}
		if len(used) < n {
			used = append(used, c)
			s.ink[len(used)-1] = c
			return len(used) - 1
		}
		best, bd := 0, -1
		for i, u := range used {
			if d := rgbDist(cpcColour(u), cpcColour(c)); bd < 0 || d < bd {
				best, bd = i, d
			}
		}
		return best
	}
	missed, over := 0, false
	for _, top := range tops {
		d := newDoc(w, h)
		for i, t := range top {
			ch, ok := 32, true
			if t.ch >= 0 && t.fg > 0 {
				ch, ok = glyph(t.ch)
			}
			if !ok {
				missed++
			}
			bg := inkOf(colour(t.bg))
			fg := bg
			if ch != 32 {
				fg = inkOf(colour(t.fg))
			}
			d.cells[i] = makeCell(ch, fg, bg, 0)
		}
		if len(used) >= n {
			over = true
		}
		art.docs = append(art.docs, cpcScreenSize(d, s.mode))
	}
	if b := f.SelectedBg; b >= 1 && b <= 27 && f.Palette == "cpc" {
		s.border = b - 1
	} else {
		s.border = s.ink[0]
	}
	art.cpc = &s
	var notes []string
	if missed > 0 {
		notes = append(notes, fmt.Sprintf("%d cells show the nearest character", missed))
	}
	if over && len(used) > n {
		notes = append(notes, fmt.Sprintf("more colours than mode %d's %d inks: the rest shown as the nearest ink", s.mode, n))
	}
	art.note = strings.Join(notes, "; ")
	return art
}

// psciCell is a Playscii tile after its layers are put together (-1: no glyph).
type psciCell struct{ ch, fg, bg int }

// cpcScreenSize is d as a CPC screen of mode m: 25 rows, the mode's width.
func cpcScreenSize(d *Doc, m int) *Doc {
	n := newCPCDoc(m)
	for y := 0; y < min(d.rows, cpcRows); y++ {
		for x := 0; x < min(d.cols, n.cols); x++ {
			n.cells[y*n.cols+x] = d.cells[y*d.cols+x]
		}
	}
	return n
}

func rgbDist(a, b uint32) int {
	dr, dg, db := int(a>>16&255)-int(b>>16&255), int(a>>8&255)-int(b>>8&255), int(a&255)-int(b&255)
	return dr*dr + dg*dg + db*db
}

// cpcNearest is the firmware colour nearest to c.
func cpcNearest(c uint32) int {
	best, bd := 0, -1
	for i := 0; i < 27; i++ {
		if d := rgbDist(cpcColour(i), c); bd < 0 || d < bd {
			best, bd = i, d
		}
	}
	return best
}

// writePsciCPC is a CPC page for Playscii with the exporter (charset CPC_ModeN,
// palette cpc); the border goes in the selected background colour.
func writePsciCPC(docs []*Doc, holds []int, s cpcSetup) (out []byte, missed int) {
	m := s.info()
	f := psciFile{Width: m.cols, Height: cpcRows, Camera: [3]float64{float64(m.cols) / 2, -float64(cpcRows) / 2, float64(m.cols) * 0.9},
		Charset: fmt.Sprintf("CPC_Mode%d", s.mode), Palette: "cpc", SelectedChar: 1, SelectedFg: s.ink[1] + 1, SelectedBg: s.border + 1}
	for i, d := range docs {
		hold := 100
		if i < len(holds) && holds[i] > 0 {
			hold = holds[i]
		}
		one := 1.0
		l := psciLayer{Name: "Layer 1", Visible: &one, Tiles: make([]psciTile, 0, m.cols*cpcRows)}
		for y := 0; y < cpcRows; y++ {
			for x := 0; x < m.cols; x++ {
				v := d.get(x, y)
				ch := cellCh(v)
				switch {
				case ch == 32:
					ch = 0
				case ch == 0:
					missed++ // glyph 0 of the charset is the space
				}
				l.Tiles = append(l.Tiles, psciTile{Char: float64(ch), Fg: float64(s.ink[s.mask(cellFg(v))] + 1), Bg: float64(s.ink[s.mask(cellBg(v))] + 1)})
			}
		}
		f.Frames = append(f.Frames, psciFrame{Delay: float64(hold) / 1000, Layers: []psciLayer{l}})
	}
	out, _ = json.MarshalIndent(f, "", " ")
	return out, missed
}

// ---------------------------------------------------------------- in CharEdit

// cpcTarget is the file being saved (writeOut), for the name inside a .dsk.
var cpcTarget string

// loadCPC opens a CPC .bin or .dsk into the current tab; false if b is neither.
func (a *App) loadCPC(b []byte, name, path, as string) bool {
	if as != "auto" && as != "cpc" {
		return false
	}
	kind := ""
	switch {
	case isDSK(b):
		kind = "cpcdsk"
	case isCPCBin(b):
		kind = "cpcbin"
	default:
		return false
	}
	if kind == "cpcdsk" {
		if _, _, err := dskArt(b); err != nil {
			if as == "auto" && !strings.HasSuffix(strings.ToLower(name), ".dsk") {
				return false
			}
			a.msg("Could not read " + name + ": " + err.Error())
			return true
		}
	}
	a.srcKind, a.ownFont, a.ownLower, a.ownPal = kind, nil, nil, nil
	a.mode = "cpc"
	a.fg, a.bg = 1, 0
	a.finishLoad(b, name, path, nil, kind)
	return true
}

// reparseCPC reads the page again from its .bin or .dsk; false if it has none.
func (a *App) reparseCPC() bool {
	if a.cpc() && a.srcKind == "" && a.source != nil { // the bare stream of codes (Open as Amstrad CPC)
		c := newCPCScreen()
		c.print(a.source, false)
		a.doc, a.cpcSet = c.d, c.s
		a.setPalette()
		a.cx, a.cy, a.sel, a.float = 0, 0, nil, nil
		if n := c.note(); n != "" {
			a.msg(a.name + ": " + n)
		}
		return true
	}
	if a.srcKind != "cpcbin" && a.srcKind != "cpcdsk" {
		return false
	}
	src, what, n := a.source, "", 1
	if a.srcKind == "cpcdsk" {
		f, k, err := dskArt(a.source)
		if err != nil {
			a.doc = newCPCDoc(a.cpcSet.mode)
			return true
		}
		src, what, n = f.data, f.name, k
	}
	d, s, note, err := parseCPCBin(src)
	if err != nil {
		a.doc = newCPCDoc(a.cpcSet.mode)
		return true
	}
	a.doc, a.cpcSet = d, s
	a.setPalette()
	a.cx, a.cy, a.sel, a.float = 0, 0, nil, nil
	switch {
	case n > 1:
		note = strings.TrimPrefix(fmt.Sprintf("%s; %d text art programs on the disk, %s shown", note, n, what), "; ")
	case what != "":
		note = strings.TrimPrefix(note+"; "+what+" on the disk", "; ")
	}
	if note != "" {
		a.msg(a.name + ": " + note)
	}
	return true
}

// encodeCPC writes a CPC page's own formats; ok false for other formats.
func (a *App) encodeCPC(fmtID string, d *Doc) (out []byte, ok bool, err error) {
	name := cpcFileName(map[bool]string{true: cpcTarget, false: a.name}[cpcTarget != ""])
	switch fmtID {
	case "cpcbin":
		return writeCPCBin(d, a.cpcSet, name), true, nil
	case "cpcdsk":
		out, err = writeDSK(name, writeCPCBin(d, a.cpcSet, name))
		return out, true, err
	}
	return nil, false, nil
}

// ---------------------------------------------------------------- the colours in the panel

// drawCPCColours draws the mode's inks (left click: pen, right: paper), then the
// 27 colours: a click gives the pen's ink that colour, a right click the
// paper's, a middle click makes it the border. It returns the y under them.
func (a *App) drawCPCColours(u *UI, x, y, perRow int) int {
	fb := u.fb
	s := a.cpcSet
	n := s.info().inks
	for i := 0; i < n; i++ {
		r := Rect{x + i%perRow*20, y + i/perRow*26, 18, 18}
		fb.fill(r, cpcColour(s.ink[i]))
		fb.frame(r, cEdgeDk)
		if i == a.fg {
			fb.frame(r.inset(-2), cHi)
		}
		if i == a.bg {
			fb.dashed(r.inset(-2), cOrange)
		}
		u.setHint(r, fmt.Sprintf("Ink %d: %s (colour %d). Left: pen, right: paper. The colours beside it set the inks", i, cpcColourNames[s.ink[i]], s.ink[i]))
		if c := u.take(r); c != nil {
			if c.button == 3 {
				a.setBg(i)
			} else {
				a.setFg(i)
			}
		}
	}
	// The 27 colours: in the panel 9 to a row under the inks, in the strip one row beside them.
	px, py, sw, per := x, y+((n+perRow-1)/perRow)*26+4, 18, 9
	if perRow >= 16 {
		px, py, sw, per = x+n*20+12, y, 10, 27
	}
	for c := 0; c < 27; c++ {
		r := Rect{px + c%per*(sw+2), py + c/per*(18+6), sw, 18}
		fb.fill(r, cpcColour(c))
		fb.frame(r, cEdgeDk)
		if c == s.ink[a.fg] {
			fb.frame(r.inset(-2), cHi)
		}
		if c == s.ink[a.bg] {
			fb.dashed(r.inset(-2), cOrange)
		}
		if c == s.border { // the border: a mark under the swatch
			fb.fill(Rect{r.x + 2, r.y + r.h + 2, r.w - 4, 2}, cHi)
		}
		u.setHint(r, fmt.Sprintf("Colour %d: %s. Left: pen's ink (%d), right: paper's ink (%d), middle: the border", c, cpcColourNames[c], a.fg, a.bg))
		if k := u.take(r); k != nil {
			switch k.button {
			case 3:
				a.setCPCInk(a.bg, c)
			case 2:
				a.setCPCBorder(c)
			default:
				a.setCPCInk(a.fg, c)
			}
		}
	}
	return py + (26/per+1)*(18+6) - 6
}

// cpcAutosave is a CPC page's screen in an autosave.
type cpcAutosave struct {
	Mode   int     `json:"mode"`
	Inks   [16]int `json:"inks"`
	Border int     `json:"border"`
}

func (c cpcAutosave) setup() cpcSetup {
	s := cpcSetup{ok: true, mode: max(0, min(2, c.Mode)), border: max(0, min(26, c.Border))}
	for i, v := range c.Inks {
		s.ink[i] = max(0, min(26, v))
	}
	return s
}
