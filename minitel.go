package main

// Minitel pages (Videotex as the Minitel's STUM 1B gives it, CEPT profile 2):
// 40 x 25 cells of 8 x 10 dots, row 0 the status row. A cell holds what the
// Minitel's page memory holds: a letter of the G0 set (or a G2 one: £, é, ...)
// with its colour, flash, inverse and size, or a mosaic of the G1 set (2 x 3
// blocks) with its colour, background, flash and separation. A letter has no
// background of its own: it shows the background, underline and concealing of
// its zone, which starts at the nearest space or mosaic to its left on the row
// (a space with other zone attributes than the zone it is in starts a new zone).
//
// Cells:
//   bits 0-7   a letter's code (0x20-0x7F G0, 0x80+ the characters of mtExtra)
//              or, with mtG1, a mosaic as teletext writes it (0x20-0x3F, 0x60-0x7F)
//   bits 8-10  the colour 0-7 (Minitel numbers: black, red, green, yellow, blue, magenta, cyan, white)
//   bits 12-14 the background
//   flags      mtG1, mtLine (underline; separated on a mosaic), mtBlink, mtInv,
//              mtMask, mtDW, mtDH, mtRight and mtTop (which part of a big letter)
//
// Files: .vdt, the Videotex stream a Minitel is sent (MiEdit, MiniPavi and the
// 3615 servers keep pages so): read by running it on a small Minitel (mtTerm),
// written so a Minitel shows the page (mtWrite checks its own stream on mtTerm).
// The letters are Frédéric Bisson's Minitel font (CC0, fonts/minitel-NOTICE.txt).

import (
	"bytes"
	"fmt"
	"strings"
)

const mtW, mtH = 8, 10 // a Minitel cell in dots

const (
	mtG1    = 1 << 16   // a mosaic (G1), not a letter
	mtLine  = underline // 1 << 17: underlined (a letter's zone, a space starting one) or separated (a mosaic)
	mtBlink = 1 << 18
	mtInv   = 1 << 19 // inverse (letters)
	mtMask  = 1 << 20 // concealed (a zone attribute)
	mtDW    = 1 << 21 // double width
	mtDH    = 1 << 22 // double height
	mtRight = 1 << 23 // the right half of a double-width letter
	mtTop   = 1 << 24 // the top half of a double-height letter

	mtZone  = mtLine | mtMask | 0x7000 // what a space's zone holds: background, underline, concealing
	mtSizes = mtDW | mtDH | mtRight | mtTop
)

// mtBlank is a cell as the Minitel clears the screen: a black mosaic space.
var mtBlank = makeCell(0x20, 7, 0, mtG1)

// mtExtra are the letters and signs beyond G0 a Minitel 1B shows (G2 and the
// accented letters), cell codes 0x80 on.
var mtExtra = []rune("£§←↑→↓°±÷¼½¾Œœßàâäèéêëîïôöùûüç")

// The Minitel's colours as the STUM gives them, in its numbering, and its grey
// levels on the black-and-white Minitel 1 (0% black ... 100% white).
var (
	minitelPalette     = pal("minitel", "Minitel colour", 0x000000, 0xFF0000, 0x00FF00, 0xFFFF00, 0x0000FF, 0xFF00FF, 0x00FFFF, 0xFFFFFF)
	minitelGreyPalette = pal("minitelgrey", "Minitel grey levels", 0x000000, 0x7F7F7F, 0xB2B2B2, 0xE5E5E5, 0x666666, 0x999999, 0xCCCCCC, 0xFFFFFF)
)

var mtColourNames = []string{"black", "red", "green", "yellow", "blue", "magenta", "cyan", "white"}

// mtFont names the Minitel's characters; they are drawn by mtDots, not from its bits.
var mtFont = &Font{id: "minitel", name: "Minitel (EF9345)", kind: "mt", syncterm: -1, bits: make([]byte, 4096)}

func init() {
	fonts = append(fonts, mtFont)
	palettes = append(palettes[:len(palettes)-1], minitelPalette, minitelGreyPalette, palettes[len(palettes)-1]) // before Custom
	mirrorPairs["mt"] = asciiPairs
	pageModes = append(pageModes, "mt")
	modeOrder = append(modeOrder, "mt")
	modeNames["mt"] = "Minitel"
	openAs = append(openAs, "mt")
	openAsNames = append(openAsNames, "Minitel")
	modeNoStream["mt"] = "a Minitel page: a picture of the screen, no stream to draw in order"
	formats = append(formats, Format{"vdt", "Minitel page (.vdt)", ".vdt", "mt"})
	for i, f := range formats {
		switch f.id {
		case "png", "png2", "gif", "apng", "utf8":
			formats[i].modes += " mt"
		}
	}
}

func (a *App) mt() bool { return a.mode == "mt" }

// mtMosaic: is v a mosaic cell? mtBits gives its six blocks (bit 0 top left,
// 1 top right, 2 and 3 the middle, 4 and 5 the bottom, as teletext's sixels).
func mtMosaic(v uint32) bool { return v&mtG1 != 0 }

func mtBits(code int) int { return code&0x1F | (code&0x40)>>1 }

// mtCode is the mosaic code of six blocks.
func mtCode(bits int) int { return 0x20 | bits&0x1F | (bits&0x20)<<1 }

// Brush codes 0xC0-0xFF are the 64 mosaics (0xC0 + their blocks): the
// character table and the F-keys hold them so.
const mtBrushMosaic = 0xC0

// mtCellOf makes a cell of brush code c in colour fg on bg with flags.
func mtCellOf(c, fg, bg int, flags uint32) uint32 {
	if c >= mtBrushMosaic {
		return makeCell(mtCode(c-mtBrushMosaic), fg, bg, mtG1|flags&(mtLine|mtBlink))
	}
	return makeCell(c, fg, bg, flags&^mtG1)
}

// mtBrushOf is the brush code of a cell.
func mtBrushOf(v uint32) int {
	if mtMosaic(v) {
		return mtBrushMosaic + mtBits(cellCh(v))
	}
	return cellCh(v)
}

func mtPrintable(c int) bool {
	return c >= 0x20 && c < 0x80+len(mtExtra) || c >= mtBrushMosaic && c <= 0xFF
}

func mtRune(c int) rune {
	switch {
	case c >= mtBrushMosaic:
		return sextantRune(c - mtBrushMosaic)
	case c >= 0x80 && c < 0x80+len(mtExtra):
		return mtExtra[c-0x80]
	}
	switch c {
	case 0x5E:
		return '↑'
	case 0x60:
		return '—'
	case 0x7B:
		return '▏'
	case 0x7C:
		return '│'
	case 0x7D:
		return '▕'
	case 0x7E:
		return '‾'
	case 0x7F:
		return '█'
	}
	if c >= 0x20 && c < 0x7F {
		return rune(c)
	}
	return ' '
}

func mtFromRune(r rune) int {
	switch r {
	case '↑':
		return 0x5E
	case '—', '–':
		return 0x60
	case '│', '|':
		return 0x7C
	case '‾':
		return 0x7E
	case '█':
		return 0x7F
	case '^', '`', '{', '}', '~':
		return -1
	}
	if r >= 0x20 && r < 0x7F {
		return int(r)
	}
	for i, e := range mtExtra {
		if e == r {
			return 0x80 + i
		}
	}
	if b := sextantBits(r); b >= 0 {
		return mtBrushMosaic + b
	}
	return -1
}

// sextantRune is the Unicode block sextant of six blocks (bit 0 top left ... bit 5 bottom right).
func sextantRune(bits int) rune {
	switch bits {
	case 0:
		return ' '
	case 63:
		return '█'
	case 21:
		return '▌'
	case 42:
		return '▐'
	}
	n := bits // U+1FB00 counts the patterns 1-62 leaving out 21 and 42
	if n > 21 {
		n--
	}
	if n > 41 {
		n--
	}
	return rune(0x1FB00 + n - 1)
}

func sextantBits(r rune) int {
	for b := 0; b < 64; b++ {
		if sextantRune(b) == r {
			return b
		}
	}
	return -1
}

// ---------------------------------------------------------------- how a row looks

// mtShow is how one cell looks once its zone is known.
type mtShow struct {
	code        int // a letter's code (0x20 for a space) or a mosaic's blocks
	mosaic, sep bool
	fg, bg      int // the colours as shown (inverse already applied)
	blink, inv  bool
	line, mask  bool // underlined, concealed
	dw, dh      bool
	right, top  bool
	delim       bool // a space that starts a zone
}

// mtDecode reads row y of a Minitel page as the screen shows it: the zone
// attributes come from the nearest space or mosaic to the left.
func mtDecode(d *Doc, y int) []mtShow {
	out := make([]mtShow, d.cols)
	zbg, zline, zmask := 0, false, false
	for x := 0; x < d.cols; x++ {
		v := d.get(x, y)
		s := mtShow{code: cellCh(v), fg: cellFg(v) & 7, blink: v&mtBlink != 0}
		if mtMosaic(v) {
			s.mosaic, s.code, s.sep, s.bg = true, mtBits(cellCh(v)), v&mtLine != 0, cellBg(v)&7
			zbg, zline = s.bg, false // a mosaic gives the zone its background and ends underlining
			s.mask = zmask
			out[x] = s
			continue
		}
		s.inv, s.dw, s.dh, s.right, s.top = v&mtInv != 0, v&mtDW != 0, v&mtDH != 0, v&mtRight != 0, v&mtTop != 0
		if s.code == 0x20 && (cellBg(v)&7 != zbg || (v&mtLine != 0) != zline || (v&mtMask != 0) != zmask) {
			zbg, zline, zmask = cellBg(v)&7, v&mtLine != 0, v&mtMask != 0
			s.delim = true // shown as a space, not underlined
		}
		s.bg, s.mask = zbg, zmask
		s.line = zline && !s.delim
		if s.inv {
			s.fg, s.bg = s.bg, s.fg
		}
		out[x] = s
	}
	return out
}

// mtGlyph is the dots of a letter code.
func mtGlyph(code int) *[10]byte {
	switch {
	case code >= 0x20 && code < 0x80:
		return &mtG0Glyphs[code-0x20]
	case code >= 0x80 && code < 0x80+len(mtExtraGlyphs):
		return &mtExtraGlyphs[code-0x80]
	}
	return &mtG0Glyphs[0]
}

// mtDots tells whether dot (px, py) of a cell is ink.
func mtDots(s mtShow, px, py int) bool {
	if s.mosaic {
		col, row, last := px/4, 0, 2
		switch {
		case py >= 7:
			row, last = 2, 9
		case py >= 3:
			row, last = 1, 6
		}
		if s.bits()&(1<<(row*2+col)) == 0 {
			return false
		}
		return !s.sep || (px%4 != 0 && py != last) // separated: a line of background left of and under each block
	}
	if s.line && py == mtH-1 && (!s.dh || !s.top) {
		return true
	}
	sx, sy := px, py
	if s.dw {
		sx = (px + map[bool]int{true: mtW}[s.right]) / 2
	}
	if s.dh {
		sy = (py + map[bool]int{false: mtH}[s.top]) / 2
	}
	return mtGlyph(s.code)[sy]&(0x80>>sx) != 0
}

func (s mtShow) bits() int { return s.code }

// drawMTCell draws a decoded cell; reveal shows concealed cells.
func (a *App) drawMTCell(fb *FB, px, py, z int, s mtShow, reveal bool) {
	bg, fg := a.pal[s.bg&7], a.pal[s.fg&7]
	show := !(s.mask && !reveal)
	if s.blink && a.blink != s.inv { // flashing: the ink goes, in the inverse phase for inverse letters
		show = false
	}
	cl := fb.clip
	inside := px >= cl.x && py >= cl.y && px+mtW*z <= cl.x+cl.w && py+mtH*z <= cl.y+cl.h
	for y := 0; y < mtH; y++ {
		for x := 0; x < mtW; x++ {
			c := bg
			if show && mtDots(s, x, y) {
				c = fg
			}
			if !inside {
				fb.fill(Rect{px + x*z, py + y*z, z, z}, c)
				continue
			}
			for yy := 0; yy < z; yy++ {
				o := (py+y*z+yy)*fb.w + px + x*z
				for xx := 0; xx < z; xx++ {
					fb.px[o+xx] = c
				}
			}
		}
	}
}

// mtCellAlone is how a cell looks on its own (the character table, the F-keys).
func mtCellAlone(v uint32) mtShow {
	if !mtMosaic(v) && cellCh(v) >= mtBrushMosaic {
		return mtShow{mosaic: true, code: cellCh(v) - mtBrushMosaic, fg: cellFg(v) & 7, bg: cellBg(v) & 7}
	}
	d := newDoc(1, 1)
	d.cells[0] = v &^ (mtRight | mtTop | mtDW | mtDH)
	s := mtDecode(d, 0)[0]
	s.line = v&mtLine != 0 && !s.mosaic
	return s
}

// newMTDoc is an empty Minitel screen.
func newMTDoc() *Doc {
	d := newDoc(40, 25)
	fillCells(d.cells, mtBlank)
	return d
}

// ---------------------------------------------------------------- the Minitel (reading .vdt)

// mtTerm is a Minitel's screen and decoder, enough to read pages: the
// Videotex codes of the STUM 1B (page mode).
type mtTerm struct {
	d                           *Doc
	delim                       []bool // a space in the page memory that started a zone
	x, y                        int
	g1                          bool
	fg, bg                      int
	blink, inv, sep, line, mask bool
	dw, dh                      bool
	wbg                         int // waiting zone attributes, -1 none (wline, wmask 0 off, 1 on)
	wline, wmask                int
	last                        int // the last character printed, for REP
	lastG1                      bool
	saved                       *mtTerm // the state before going to the status row
	frames                      []*Doc  // the screens a clear ended (with content)
	drawn                       bool    // something was printed since the last clear
	// The stream's state between bytes.
	esc         []byte // the sequence being read
	want        int    // bytes the sequence still needs (-1: up to a final byte)
	ss2, accent int
	rep, us     bool
	usRow       int
	drcs        bool // inside a DRCS definition (skipped up to the next US)
}

func newMTTerm() *mtTerm {
	t := &mtTerm{d: newMTDoc(), delim: make([]bool, 1000), y: 1, fg: 7, wbg: -1, wline: -1, wmask: -1, last: 0x20, ss2: -1, accent: -1}
	return t
}

// reset is the attributes after US, RS, FF or a cursor position: white on black, G0, normal size.
func (t *mtTerm) reset() {
	t.g1, t.fg, t.bg, t.blink, t.inv, t.sep, t.line, t.mask, t.dw, t.dh = false, 7, 0, false, false, false, false, false, false, false
	t.wbg, t.wline, t.wmask = -1, -1, -1
}

func (t *mtTerm) set(x, y int, v uint32, delim bool) {
	if x < 0 || x >= 40 || y < 0 || y >= 25 {
		return
	}
	// A big letter written over part of another leaves the other's parts as they were.
	t.d.cells[y*40+x] = v
	t.delim[y*40+x] = delim
}

// clear blanks cells from..to (inclusive, as y*40+x) as the Minitel does: black mosaic spaces.
func (t *mtTerm) clear(from, to int) {
	if t.drawn && from <= 40 && to >= 999 && len(t.frames) < maxSplitFrames {
		t.frames = append(t.frames, t.d.clone())
		t.drawn = false
	}
	for i := max(0, from); i <= min(999, to); i++ {
		t.d.cells[i] = mtBlank
		t.delim[i] = false
	}
}

func (t *mtTerm) down() {
	if t.y == 0 {
		t.leaveStatus()
		return
	}
	if t.y++; t.y > 24 {
		t.y = 1 // page mode: back to the top
	}
}

func (t *mtTerm) leaveStatus() {
	if t.saved != nil {
		s := t.saved
		t.x, t.y, t.g1, t.fg, t.bg, t.blink, t.inv, t.sep, t.line, t.mask = s.x, s.y, s.g1, s.fg, s.bg, s.blink, s.inv, s.sep, s.line, s.mask
		t.dw, t.dh, t.wbg, t.wline, t.wmask = s.dw, s.dh, s.wbg, s.wline, s.wmask
		t.saved = nil
		return
	}
	t.y = 1
}

func (t *mtTerm) locate(y, x int) {
	if x < 0 || x > 39 || y < 0 || y > 24 {
		return
	}
	if y == 0 && t.y != 0 {
		s := *t
		t.saved = &s
	}
	if y != 0 {
		t.saved = nil
	}
	t.x, t.y = x, y
	t.reset()
}

func (t *mtTerm) setSize(w, h bool) {
	if t.g1 || t.y == 0 {
		return
	}
	if h && t.y <= 1 {
		return
	}
	t.dw, t.dh = w, h
}

func (t *mtTerm) shift(g1 bool) {
	t.sep, t.inv, t.dw, t.dh = false, false, false, false // the STUM: size and inverse end in G1, separation in G0
	if g1 {
		t.line = false
		t.wline = -1
		if t.wbg >= 0 {
			t.bg, t.wbg = t.wbg, -1
		}
	}
	t.g1 = g1
}

// print puts a character at the cursor and moves on.
func (t *mtTerm) print(c int) {
	t.last, t.lastG1 = c, t.g1
	t.drawn = true
	w := 1
	switch {
	case t.g1:
		if t.wbg >= 0 { // a mosaic takes a waiting background at once
			t.bg, t.wbg = t.wbg, -1
		}
		code := c
		if code >= 0x40 && code < 0x60 {
			code += 0x20 // the STUM's table leaves them out; a Minitel shows the mosaics 0x60-0x7F (pages use 0x5F for the full block)
		}
		var f uint32 = mtG1
		if t.sep {
			f |= mtLine
		}
		if t.blink {
			f |= mtBlink
		}
		t.set(t.x, t.y, makeCell(code, t.fg, t.bg, f), false)
	default:
		delim := c == 0x20 && (t.wbg >= 0 || t.wline >= 0 || t.wmask >= 0)
		if delim { // a space after a zone attribute starts a zone
			if t.wbg >= 0 {
				t.bg = t.wbg
			}
			if t.wline >= 0 {
				t.line = t.wline == 1
			}
			if t.wmask >= 0 {
				t.mask = t.wmask == 1
			}
			t.wbg, t.wline, t.wmask = -1, -1, -1
		}
		var f uint32
		if t.blink && !delim {
			f |= mtBlink
		}
		if t.inv {
			f |= mtInv
		}
		if delim {
			if t.line {
				f |= mtLine
			}
			if t.mask {
				f |= mtMask
			}
		}
		dw, dh := t.dw && t.x < 39, t.dh
		if dw {
			w = 2
		}
		for py := 0; py < map[bool]int{true: 2, false: 1}[dh]; py++ {
			for px := 0; px < w; px++ {
				g := f
				if dw {
					g |= mtDW
					if px == 1 {
						g |= mtRight
					}
				}
				if dh {
					g |= mtDH
					if py == 1 {
						g |= mtTop
					}
				}
				t.set(t.x+px, t.y-py, makeCell(c, t.fg, t.bg, g), delim)
			}
		}
	}
	t.x += w
	if t.x >= 40 {
		if t.y == 0 {
			t.x = 39 // no overflow on the status row
			return
		}
		t.x = 0
		t.down()
		if t.dh {
			t.down()
		}
	}
}

// feed runs a stream of bytes on the Minitel.
func (t *mtTerm) feed(b []byte) {
	for _, c := range b {
		t.byte(int(c & 0x7F))
	}
}

func (t *mtTerm) byte(c int) {
	switch {
	case t.drcs: // a character set being sent: skipped up to the US that ends it
		if c == 0x1F {
			t.drcs, t.us, t.usRow = false, true, -1
		}
		return
	case t.us: // US row col (or US 0x23: a DRCS definition starts)
		if t.usRow < 0 {
			if c == 0x23 {
				t.us, t.drcs = false, true
				return
			}
			t.usRow = c
			return
		}
		t.us = false
		r, col := t.usRow, c
		switch {
		case r >= 0x40 && r <= 0x7F && col >= 0x40 && col <= 0x7F:
			t.locate(r-0x40, col-0x41)
		case r >= 0x30 && r <= 0x32 && col >= 0x30 && col <= 0x39: // the old form: tens and units of the row
			t.locate((r-0x30)*10+col-0x30, 0)
		}
		return
	case t.rep:
		t.rep = false
		if c >= 0x40 {
			for i := 0; i < c-0x40; i++ {
				t.g1 = t.lastG1
				t.print(t.last)
			}
		}
		return
	case t.esc != nil:
		t.escByte(c)
		return
	case t.ss2 >= 0 && c >= 0x20:
		t.g2(c)
		return
	}
	if c < 0x20 { // a control code after SS2 is done, and the SS2 dropped (the STUM)
		t.ss2, t.accent = -1, -1
	}
	switch c {
	case 0x08: // BS
		if t.x--; t.x < 0 {
			if t.y == 0 {
				t.x = 0
				return
			}
			t.x = 39
			if t.y--; t.y < 1 {
				t.y = 24
			}
		}
	case 0x09: // HT
		if t.x++; t.x > 39 {
			if t.y == 0 {
				t.x = 39
				return
			}
			t.x = 0
			t.down()
		}
	case 0x0A: // LF
		t.down()
	case 0x0B: // VT
		if t.y > 1 {
			t.y--
		} else if t.y == 1 {
			t.y = 24
		}
	case 0x0C: // FF: clears rows 1-24
		t.clear(40, 999)
		t.x, t.y, t.saved = 0, 1, nil
		t.reset()
	case 0x0D: // CR
		t.x = 0
	case 0x0E: // SO: mosaics
		t.shift(true)
	case 0x0F: // SI: letters
		t.shift(false)
	case 0x12: // REP n
		t.rep = true
	case 0x13: // SEP and a key code
		t.esc, t.want = []byte{0x13}, 1
	case 0x16: // SYN, taken as SS2 as VDT2BMP does: pages use it for accents (TF1's "Télématique")
		if !t.g1 {
			t.ss2 = 0
		}
	case 0x18: // CAN: spaces of the current set from the cursor to the end of the row
		x, y := t.x, t.y
		for i := t.x; i < 40; i++ {
			var f uint32
			if t.g1 {
				f = mtG1
			}
			t.set(i, y, makeCell(0x20, t.fg, t.bg, f), false)
		}
		t.x, t.y = x, y
	case 0x19: // SS2: a G2 character follows
		if !t.g1 {
			t.ss2 = 0
		}
	case 0x1A: // SUB: the error sign
		t.print(0x7F)
	case 0x1B:
		t.esc, t.want = []byte{}, 1
	case 0x1E: // RS: home
		t.locate(1, 0)
	case 0x1F: // US
		t.us, t.usRow = true, -1
	default:
		if c >= 0x20 {
			if t.ss2 >= 0 {
				t.g2(c)
				return
			}
			t.print(c)
		}
	}
}

// g2 reads the character after SS2: a sign, or an accent and its letter.
func (t *mtTerm) g2(c int) {
	if t.accent >= 0 {
		acc := t.accent
		t.accent, t.ss2 = -1, -1
		t.print(mtComposed(acc, c))
		return
	}
	switch c {
	case 0x41, 0x42, 0x43, 0x48, 0x4B: // grave, acute, circumflex, diaeresis, cedilla
		t.accent = c
		return
	}
	t.ss2 = -1
	if code, ok := mtG2[c]; ok {
		t.print(code)
		return
	}
	t.print(0x5F) // the STUM: a low bar for a sign the Minitel lacks
}

// mtG2 is the G2 signs a Minitel 1B shows, as cell codes.
var mtG2 = map[int]int{0x23: 0x80, 0x24: 0x24, 0x26: 0x23, 0x27: 0x81, 0x2C: 0x82, 0x2D: 0x83, 0x2E: 0x84, 0x2F: 0x85,
	0x30: 0x86, 0x31: 0x87, 0x38: 0x88, 0x3C: 0x89, 0x3D: 0x8A, 0x3E: 0x8B, 0x6A: 0x8C, 0x7A: 0x8D, 0x7B: 0x8E}

// mtAccents gives the accented letters: accent code and letter -> the letter of mtExtra.
var mtAccents = map[[2]int]rune{
	{0x41, 'a'}: 'à', {0x43, 'a'}: 'â', {0x48, 'a'}: 'ä', {0x41, 'e'}: 'è', {0x42, 'e'}: 'é', {0x43, 'e'}: 'ê',
	{0x48, 'e'}: 'ë', {0x43, 'i'}: 'î', {0x48, 'i'}: 'ï', {0x43, 'o'}: 'ô', {0x48, 'o'}: 'ö', {0x41, 'u'}: 'ù',
	{0x43, 'u'}: 'û', {0x48, 'u'}: 'ü', {0x4B, 'c'}: 'ç',
}

// mtComposed is the cell code of a letter with an accent; one the Minitel lacks shows the bare letter.
func mtComposed(acc, c int) int {
	if r, ok := mtAccents[[2]int{acc, c}]; ok {
		return mtFromRune(r)
	}
	return c
}

// mtSS2 is how a cell code past G0 is sent: SS2 and its G2 code (and the letter).
func mtSS2(code int) []byte {
	for g, c := range mtG2 {
		if c == code && c >= 0x80 {
			return []byte{0x19, byte(g)}
		}
	}
	if code >= 0x80 && code < 0x80+len(mtExtra) {
		r := mtExtra[code-0x80]
		for k, v := range mtAccents {
			if v == r {
				return []byte{0x19, byte(k[0]), byte(k[1])}
			}
		}
	}
	return []byte{byte(code)}
}

func (t *mtTerm) escByte(c int) {
	t.esc = append(t.esc, byte(c))
	s := t.esc
	if s[0] == 0x13 { // SEP x: a key, nothing on the screen
		t.esc = nil
		return
	}
	switch {
	case len(s) == 1:
		switch {
		case c == 0x5B: // CSI: parameters up to a final byte
			return
		case c >= 0x40 && c <= 0x5F:
			t.esc = nil
			t.attribute(c)
		case c == 0x39: // PRO1 x
			t.want = 1
		case c == 0x3A: // PRO2 x y
			t.want = 2
		case c == 0x3B: // PRO3 x y z
			t.want = 3
		case c == 0x23 || c == 0x25: // ESC # SP x (whole-screen mask), ESC % ...
			t.want = 2
		case c >= 0x28 && c <= 0x2B: // a character set designation: ESC ( @, ESC ) c, or with a space first
			t.want = 1
		default: // anything else: ignored
			t.esc = nil
		}
		return
	case s[0] == 0x5B:
		if c >= 0x40 && c <= 0x7E {
			t.csi(string(s[1:len(s)-1]), c)
			t.esc = nil
		} else if len(s) > 16 {
			t.esc = nil
		}
		return
	case s[0] >= 0x28 && s[0] <= 0x2B && c == 0x20 && len(s) == 2:
		return // ESC ( SP B: one more byte
	}
	if t.want--; t.want <= 0 {
		t.esc = nil
	}
}

// attribute applies ESC and a C1 code.
func (t *mtTerm) attribute(c int) {
	switch {
	case c <= 0x47:
		t.fg = c - 0x40
	case c == 0x48:
		t.blink = true
	case c == 0x49:
		t.blink = false
	case c == 0x4C:
		t.setSize(false, false)
	case c == 0x4D:
		t.setSize(false, true)
	case c == 0x4E:
		t.setSize(true, false)
	case c == 0x4F:
		t.setSize(true, true)
	case c >= 0x50 && c <= 0x57:
		if t.g1 {
			t.bg = c - 0x50
		} else {
			t.wbg = c - 0x50
		}
	case c == 0x58:
		t.wmask = 1
	case c == 0x5F:
		t.wmask = 0
	case c == 0x59, c == 0x5A:
		on := c == 0x5A
		if t.g1 {
			t.sep = on
		} else {
			t.wline = map[bool]int{true: 1, false: 0}[on]
		}
	case c == 0x5C:
		if !t.g1 {
			t.inv = false
		}
	case c == 0x5D:
		if !t.g1 {
			t.inv = true
		}
	}
}

func (t *mtTerm) csi(params string, final int) {
	var ps []int
	for _, p := range strings.Split(params, ";") {
		n := 0
		for _, ch := range p {
			if ch >= '0' && ch <= '9' {
				n = min(n*10+int(ch-'0'), 999)
			}
		}
		ps = append(ps, n)
	}
	p0 := ps[0]
	n := max(1, p0)
	at := t.y*40 + t.x
	switch final {
	case 'A':
		t.y = max(1, t.y-n)
	case 'B':
		t.y = min(24, t.y+n)
	case 'C':
		t.x = min(39, t.x+n)
	case 'D':
		t.x = max(0, t.x-n)
	case 'H', 'f':
		row, col := 1, 1
		if len(ps) > 0 && ps[0] > 0 {
			row = ps[0]
		}
		if len(ps) > 1 && ps[1] > 0 {
			col = ps[1]
		}
		if row >= 1 && row <= 24 && col >= 1 && col <= 40 {
			t.x, t.y = col-1, row
		}
	case 'J':
		switch p0 {
		case 0:
			t.clear(at, 999)
		case 1:
			t.clear(40, at)
		case 2:
			t.clear(40, 999)
		}
	case 'K':
		row := t.y * 40
		switch p0 {
		case 0:
			t.clear(at, row+39)
		case 1:
			t.clear(row, at)
		case 2:
			t.clear(row, row+39)
		}
	case 'P': // delete n characters: the rest of the row moves left
		row := t.d.cells[t.y*40 : t.y*40+40]
		copy(row[t.x:], row[min(40, t.x+n):])
		for i := max(t.x, 40-n); i < 40; i++ {
			row[i] = mtBlank
		}
	case '@': // insert n characters
		row := t.d.cells[t.y*40 : t.y*40+40]
		copy(row[min(40, t.x+n):], row[t.x:])
		for i := t.x; i < min(40, t.x+n); i++ {
			row[i] = mtBlank
		}
	case 'M', 'L': // delete or insert rows
		for k := 0; k < n; k++ {
			if final == 'M' {
				copy(t.d.cells[t.y*40:1000-40], t.d.cells[t.y*40+40:1000])
				fillCells(t.d.cells[960:1000], mtBlank)
			} else {
				copy(t.d.cells[t.y*40+40:1000], t.d.cells[t.y*40:1000-40])
				fillCells(t.d.cells[t.y*40:t.y*40+40], mtBlank)
			}
		}
	}
}

// page is the screen as the Minitel shows it, in CharEdit's cells: every
// letter and plain space takes its zone's background, underline and concealing.
func (t *mtTerm) page() *Doc {
	d := t.d.clone()
	mtSettle(d, t.delim)
	return d
}

// row is row y of the screen settled (as page) and the zone before each cell.
func (t *mtTerm) row(y int) (settled, zone []uint32) {
	return mtRow(t.d.cells[y*40:y*40+40], t.delim[y*40:y*40+40])
}

// mtRow reads a row of cells left to right: settled is the row with every
// letter and plain space given the attributes of its zone, zone[x] the zone
// (background, underline, concealing as in a cell) a letter at x is in. delim
// says which spaces start a zone; nil: a space whose attributes differ from its zone's.
func mtRow(cells []uint32, delim []bool) (settled, zone []uint32) {
	settled, zone = make([]uint32, len(cells)), make([]uint32, len(cells))
	var z uint32
	for x, v := range cells {
		zone[x] = z
		settled[x] = v
		switch {
		case mtMosaic(v): // a mosaic gives the zone its background and ends underlining
			z = v&0x7000 | z&mtMask
			continue
		case cellCh(v) == 0x20 && (delim != nil && delim[x] || delim == nil && v&mtZone != z):
			z = v & mtZone
			settled[x] = v &^ mtBlink
			continue
		}
		v = v&^mtZone | z
		if cellCh(v) == 0x20 {
			v &^= mtBlink // a space looks the same flashing or not
		}
		settled[x] = v
	}
	return settled, zone
}

// mtSettle gives the letters (and spaces that start no zone) of a screen their
// zone attributes; delim tells which spaces started a zone (nil: those that differ).
func mtSettle(d *Doc, delim []bool) {
	for y := 0; y < d.rows; y++ {
		var dl []bool
		if delim != nil {
			dl = delim[y*d.cols : y*d.cols+d.cols]
		}
		s, _ := mtRow(d.cells[y*d.cols:y*d.cols+d.cols], dl)
		copy(d.cells[y*d.cols:], s)
	}
}

// parseVDT reads a Minitel stream: the screen at its end, and the screens a
// clear (FF) ended before it, each with something on it.
func parseVDT(b []byte) []*Doc {
	t := newMTTerm()
	t.feed(b)
	var out []*Doc
	for _, f := range t.frames {
		mtSettle(f, nil)
		out = append(out, f)
	}
	if t.drawn || len(out) == 0 {
		out = append(out, t.page())
	}
	return out
}

// isVDT: does b look like a Minitel stream? It must clear the screen or place
// the cursor (US) and use Videotex's attributes, and hold no ANSI.
func isVDT(b []byte, name string) bool {
	head := b[:min(len(b), 8192)]
	if bytes.Contains(head, []byte("\x1b[")) && !strings.HasSuffix(strings.ToLower(name), ".vdt") {
		ansi := 0
		for _, m := range [][]byte{[]byte("m"), []byte("H"), []byte("J")} {
			ansi += bytes.Count(head, append([]byte("\x1b["), m...))
		}
		if ansi > 2 {
			return false
		}
	}
	us, ff, attrs := 0, 0, 0
	for i, c := range head {
		switch c {
		case 0x1F:
			if i+2 < len(head) && head[i+1] >= 0x40 && head[i+1] <= 0x58 && head[i+2] >= 0x41 && head[i+2] <= 0x68 {
				us++
			}
		case 0x0C:
			ff++
		case 0x1B:
			if i+1 < len(head) && head[i+1] >= 0x40 && head[i+1] <= 0x5F && head[i+1] != 0x5B {
				attrs++
			}
		case 0x0E, 0x0F:
			attrs++
		}
	}
	if strings.HasSuffix(strings.ToLower(name), ".vdt") {
		return us+ff+attrs > 0 || len(b) > 0
	}
	return (us >= 2 || ff >= 1) && attrs >= 3
}

// ---------------------------------------------------------------- writing .vdt

// mtWriter makes a stream and runs it on a Minitel as it goes, so it always
// knows the state the Minitel is in.
type mtWriter struct {
	out []byte
	t   *mtTerm
}

func (w *mtWriter) emit(b ...byte) {
	w.out = append(w.out, b...)
	w.t.feed(b)
}

// moveTo puts the cursor at (x, y) of rows 1-24: cursor steps, CR LF or US
// (which also resets the attributes).
func (w *mtWriter) moveTo(x, y int) {
	t := w.t
	switch {
	case t.x == x && t.y == y:
	case t.y == y && x > t.x && x-t.x <= 3:
		for t.x < x {
			w.emit(0x09)
		}
	case t.y >= 1 && t.y+1 == y && x == 0:
		w.emit(0x0D, 0x0A)
	default:
		w.emit(0x1F, byte(0x40+y), byte(0x41+x))
	}
}

func (w *mtWriter) shift(g1 bool) {
	if w.t.g1 != g1 {
		w.emit(map[bool]byte{true: 0x0E, false: 0x0F}[g1])
	}
}

func (w *mtWriter) esc(c byte) { w.emit(0x1B, c) }

// attrs sets the colour, flash and (letters) inverse and size of the next character.
func (w *mtWriter) attrs(v uint32) {
	t := w.t
	if fg := cellFg(v) & 7; t.fg != fg {
		w.esc(byte(0x40 + fg))
	}
	space := !mtMosaic(v) && cellCh(v) == 0x20
	if b := v&mtBlink != 0; t.blink != b && !space {
		w.esc(map[bool]byte{true: 0x48, false: 0x49}[b])
	}
	if mtMosaic(v) {
		if bg := cellBg(v) & 7; t.bg != bg {
			w.esc(byte(0x50 + bg))
		}
		if s := v&mtLine != 0; t.sep != s {
			w.esc(map[bool]byte{true: 0x5A, false: 0x59}[s])
		}
		return
	}
	if inv := v&mtInv != 0; t.inv != inv {
		w.esc(map[bool]byte{true: 0x5D, false: 0x5C}[inv])
	}
	dw, dh := v&mtDW != 0, v&mtDH != 0
	if t.dw != dw || t.dh != dh {
		code := byte(0x4C)
		switch {
		case dw && dh:
			code = 0x4F
		case dw:
			code = 0x4E
		case dh:
			code = 0x4D
		}
		w.esc(code)
	}
}

// put writes the cell v at the cursor; zone is the zone the cells to its left
// put it in (background, underline, concealing as in a cell).
func (w *mtWriter) put(v uint32, zone uint32) {
	t := w.t
	if mtMosaic(v) {
		w.shift(true)
		w.attrs(v)
		w.emit(byte(cellCh(v)))
		return
	}
	w.shift(false)
	w.attrs(v)
	c := cellCh(v)
	if c != 0x20 {
		w.emit(mtSS2(c)...)
		return
	}
	pending := t.wbg >= 0 || t.wline >= 0 || t.wmask >= 0
	if v&mtZone != zone || pending { // a space that starts a zone: its attributes, then the space
		bg, line, mask := t.bg, t.line, t.mask // what the space would take
		if t.wbg >= 0 {
			bg = t.wbg
		}
		if t.wline >= 0 {
			line = t.wline == 1
		}
		if t.wmask >= 0 {
			mask = t.wmask == 1
		}
		if want := int(cellBg(v) & 7); want != bg {
			w.esc(byte(0x50 + want))
		}
		if want := v&mtLine != 0; want != line {
			w.esc(map[bool]byte{true: 0x5A, false: 0x59}[want])
		}
		if want := v&mtMask != 0; want != mask {
			w.esc(map[bool]byte{true: 0x58, false: 0x5F}[want])
		}
		if t.wbg < 0 && t.wline < 0 && t.wmask < 0 { // nothing differs, yet it must start a zone
			w.esc(byte(0x50 + cellBg(v)&7))
		}
	}
	w.emit(0x20)
}

// shown is cell x of row y as the Minitel shows it now, and the zone it is in.
func (w *mtWriter) shown(x, y int) (uint32, uint32) {
	s, z := w.t.row(y)
	return s[x], z[x]
}

// context makes the Minitel's background, underline and concealing those of
// the zone a letter at (x, y) is in, by writing the cell that starts the zone
// again (or by US to the row's start for a row's first zone). A Minitel shows
// a letter in its zone's attributes; one that kept the attributes current when
// it was written (as VDT2BMP draws them) then shows it the same.
func (w *mtWriter) context(want *Doc, x, y int) {
	t := w.t
	_, zones := mtRow(want.cells[y*40:y*40+40], nil)
	z := zones[x]
	ctx := uint32(t.bg)<<12 | map[bool]uint32{true: mtLine}[t.line] | map[bool]uint32{true: mtMask}[t.mask]
	if t.y == y && t.x <= x && ctx == z && t.wbg < 0 && t.wline < 0 && t.wmask < 0 {
		return
	}
	start := -1
	for i := x - 1; i >= 0; i-- {
		v := want.get(i, y)
		if mtMosaic(v) || cellCh(v) == 0x20 && v&mtZone != zones[i] {
			start = i
			break
		}
	}
	if start < 0 {
		if y > 0 && z == 0 {
			w.emit(0x1F, byte(0x40+y), 0x41)
		}
		return
	}
	w.moveTo(start, y)
	for i := start; i < x; i++ { // the zone's start and its cells up to x, written again
		v := want.get(i, y)
		if v&mtSizes != 0 {
			w.moveTo(x, y)
			return
		}
		_, zone := w.shown(i, y)
		w.put(v, zone)
	}
}

// mtOrigin: is v the cell a big letter is written from (its bottom left)?
func mtOrigin(v uint32) bool { return mtMosaic(v) || v&(mtRight|mtTop) == 0 }

// mtFull is the page as 25 settled rows.
func mtFull(d *Doc) *Doc {
	want := newMTDoc()
	for y := 0; y < min(25, d.rows); y++ {
		for x := 0; x < min(40, d.cols); x++ {
			want.cells[y*40+x] = d.get(x, y)
		}
	}
	mtSettle(want, nil)
	return want
}

// mtWrite gives a .vdt stream that shows the page: FF, rows 1-24, then the
// status row if anything is on it, and LF back from it. The stream is run on
// mtTerm as it is made; cells that still come out wrong (big letters over
// each other, parts on their own) are written again one by one.
func mtWrite(d *Doc) []byte {
	want := mtFull(d)
	w := &mtWriter{t: newMTTerm()}
	w.emit(0x0C)
	for y := 1; y < 25; y++ {
		for x := 0; x < 40; x++ {
			v := want.get(x, y)
			if got, _ := w.shown(x, y); !mtOrigin(v) || got == v {
				continue
			}
			if v&mtDH != 0 && y < 2 {
				v &^= mtDH | mtTop
			}
			if !mtMosaic(v) && cellCh(v) != 0x20 {
				w.context(want, x, y)
			}
			w.moveTo(x, y)
			_, zone := w.shown(x, y)
			w.put(v, zone)
			w.runs(want, x, y)
		}
	}
	// Then whatever is not as wanted, cell by cell.
	for pass := 0; pass < 4; pass++ {
		fixed := false
		for y := 1; y < 25; y++ {
			for x := 0; x < 40; x++ {
				v := want.get(x, y)
				if got, _ := w.shown(x, y); got == v {
					continue
				}
				fixed = true
				if !mtOrigin(v) { // a part on its own: the whole letter from its origin, which is put right after
					ox, oy := x-map[bool]int{true: 1}[v&mtRight != 0], y+map[bool]int{true: 1}[v&mtTop != 0]
					if ox >= 0 && oy <= 24 && (v&mtDH == 0 || oy >= 2) {
						w.moveTo(ox, oy)
						_, zone := w.shown(ox, oy)
						w.put(v&^(mtRight|mtTop), zone)
						continue
					}
					v &^= mtSizes
				}
				if v&mtDH != 0 && y < 2 {
					v &^= mtSizes
				}
				w.moveTo(x, y)
				_, zone := w.shown(x, y)
				w.put(v, zone)
			}
		}
		if !fixed {
			break
		}
	}
	status := false
	for x := 0; x < 40; x++ {
		status = status || want.get(x, 0) != mtBlank
	}
	if status {
		w.emit(0x1F, 0x40, 0x41)
		for x := 0; x < 40; x++ {
			v := want.get(x, 0) &^ mtSizes
			if got, _ := w.shown(x, 0); got == v {
				continue
			}
			if w.t.x != x {
				w.emit(0x1F, 0x40, byte(0x41+x))
			}
			_, zone := w.shown(x, 0)
			w.put(v, zone)
		}
		w.emit(0x0A) // back from the status row
	}
	return w.out
}

// runs, after the cell at x was written, repeats it with REP while the cells
// after it in the row want the same.
func (w *mtWriter) runs(want *Doc, x, y int) {
	t := w.t
	v := want.get(x, y)
	if t.y != y || t.x != x+1 || v&mtSizes != 0 || !mtMosaic(v) && cellCh(v) >= 0x80 {
		return
	}
	n := 0
	for i := x + 1; i < 40 && n < 63 && want.get(i, y) == v; i++ {
		n++
	}
	if n >= 3 {
		w.emit(0x12, byte(0x40+n))
	}
}

// mtNote describes a .vdt that showed several screens.
func mtNote(name string, n int) string {
	return fmt.Sprintf("%s: %d screens (each clear starts one), opened as frames", name, n)
}
