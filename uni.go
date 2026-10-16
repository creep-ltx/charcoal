package main

// The Unicode page (docs/unicode-design.md): any character Charcoal can draw,
// 256 and 24-bit colours, in the same 32-bit cell every other page uses.
//
// A Unicode cell:
//   bits 0-7, 18-25          the character code (16 bits: the BMP, with the
//                            surrogates standing for the plane-1 legacy computing blocks)
//   bits 8-15, 16-17, 26-31  the style (16 bits): 0-255 are fg = p&15, bg = p>>4
//                            (a PC cell's colours), the rest from uniStyles:
//                            colours of the 256 or 24-bit, italic, underline

import (
	"bufio"
	"bytes"
	"compress/gzip"
	_ "embed"
	"encoding/hex"
	"fmt"
	"math/bits"
	"strconv"
	"strings"
	"sync"
	"unicode"
)

const (
	uniHiShift   = 18                         // the character code's high byte
	uniPairShift = 26                         // the style's bits 10-15
	uniHiMask    = uint32(255) << uniHiShift  // where the code's high byte lives
	uniPairMask  = uint32(63) << uniPairShift // where the style's high bits live
	rgbFlag      = 1 << 24                    // a colour that is 0xRRGGBB, not one of the 256
	uniPairMax   = 1 << 16
)

func (a *App) uni() bool { return a.mode == "uni" }

// uniCode is a Unicode cell's character code, uniPair its style (colours, italic, underline).
func uniCode(v uint32) int { return int(v&255) | int(v>>uniHiShift&255)<<8 }
func uniPair(v uint32) int { return int(v>>8&1023) | int(v>>uniPairShift)<<10 }

// uniCell makes a Unicode cell of a character code and a style (pairOf, styleOf).
func uniCell(code, pair int) uint32 {
	return uint32(code&255) | uint32(code>>8&255)<<uniHiShift | uint32(pair&1023)<<8 | uint32(pair>>10&63)<<uniPairShift
}

// uniWithCode / uniWithPair change one half of a cell.
func uniWithCode(v uint32, code int) uint32 { return uniCell(code, uniPair(v)) }
func uniWithPair(v uint32, pair int) uint32 { return uniCell(uniCode(v), pair) }

// ---------------------------------------------------------------- characters

// The surrogates are never characters: D800-D8FF stand for U+1FB00-1FBFF (Symbols
// for Legacy Computing), D900-DCFF for U+1CC00-1CFFF (its supplement).
func uniRune(code int) rune {
	switch {
	case code >= 0xD800 && code <= 0xD8FF:
		return rune(0x1FB00 + code - 0xD800)
	case code >= 0xD900 && code <= 0xDCFF:
		return rune(0x1CC00 + code - 0xD900)
	case code >= 0xDD00 && code <= 0xDFFF:
		return '?'
	case code == 0:
		return ' '
	}
	return rune(code)
}

// uniCodeOf is the code of a character, false if a cell cannot hold it.
func uniCodeOf(r rune) (int, bool) {
	switch {
	case r >= 0xD800 && r <= 0xDFFF:
		return '?', false
	case r < 0x10000:
		return int(r), true
	case r >= 0x1FB00 && r <= 0x1FBFF:
		return int(0xD800 + r - 0x1FB00), true
	case r >= 0x1CC00 && r <= 0x1CFFF:
		return int(0xD900 + r - 0x1CC00), true
	}
	return '?', false
}

// ---------------------------------------------------------------- colours

// A colour of a Unicode page: 0-255 (0-15 the page's palette, 16-255 xterm's)
// or rgbFlag | 0xRRGGBB. A style is two colours and italic / underline.
type uniStyle struct {
	fg, bg int
	flags  uint32 // italic, underline
}

// uniStyles is the one table of styles 256 and up, for every page and tab: it
// only grows, so a style number never changes its meaning (undo steps, frames,
// the clipboard and other tabs share cells freely).
var uniStyles struct {
	sync.RWMutex
	list  []uniStyle // style 256 + i
	index map[uniStyle]int
	full  bool // the table ran out once: nearest styles were used
}

// pairOf is the style of a foreground and background colour.
func pairOf(fg, bg int) int { return styleOf(fg, bg, 0) }

// styleOf is the style of two colours, italic and underline.
func styleOf(fg, bg int, flags uint32) int {
	flags &= italic | underline
	if fg >= 0 && fg < 16 && bg >= 0 && bg < 16 && flags == 0 {
		return fg | bg<<4
	}
	c := uniStyle{fg, bg, flags}
	uniStyles.RLock()
	p, ok := uniStyles.index[c]
	uniStyles.RUnlock()
	if ok {
		return p
	}
	uniStyles.Lock()
	defer uniStyles.Unlock()
	if p, ok := uniStyles.index[c]; ok {
		return p
	}
	if uniStyles.index == nil {
		uniStyles.index = map[uniStyle]int{}
	}
	if 256+len(uniStyles.list) >= uniPairMax {
		uniStyles.full = true
		return nearestStyle(c)
	}
	p = 256 + len(uniStyles.list)
	uniStyles.list = append(uniStyles.list, c)
	uniStyles.index[c] = p
	return p
}

// nearestStyle is the existing style closest to c (the table is full; held locked).
func nearestStyle(c uniStyle) int {
	f, b := colourRGB(c.fg, &vga16), colourRGB(c.bg, &vga16)
	best, bd := nearest16(f)|nearest16(b)<<4, 1<<62
	if c.flags == 0 {
		bd = rgbDist(f, xterm256(nearest16(f))) + rgbDist(b, xterm256(nearest16(b)))
	}
	for i, o := range uniStyles.list {
		if o.flags != c.flags {
			continue
		}
		if d := rgbDist(f, colourRGB(o.fg, &vga16)) + rgbDist(b, colourRGB(o.bg, &vga16)); d < bd {
			best, bd = 256+i, d
		}
	}
	return best
}

// styleParts gives a style's colours and italic / underline.
func styleParts(p int) (fg, bg int, flags uint32) {
	if p < 256 {
		return p & 15, p >> 4, 0
	}
	uniStyles.RLock()
	defer uniStyles.RUnlock()
	if i := p - 256; i < len(uniStyles.list) {
		c := uniStyles.list[i]
		return c.fg, c.bg, c.flags
	}
	return 7, 0, 0 // a style this run never made (a damaged autosave): plain
}

// pairColours gives a style's two colours.
func pairColours(p int) (fg, bg int) {
	fg, bg, _ = styleParts(p)
	return fg, bg
}

// uniColoursOf is a Unicode cell's foreground and background colour, uniFlagsOf its italic / underline.
func uniColoursOf(v uint32) (fg, bg int) { return pairColours(uniPair(v)) }
func uniFlagsOf(v uint32) uint32 {
	_, _, f := styleParts(uniPair(v))
	return f
}

// uniRestyle gives a cell other colours, keeping its italic / underline.
func uniRestyle(v uint32, fg, bg int) uint32 { return uniWithPair(v, styleOf(fg, bg, uniFlagsOf(v))) }

// colourRGB is a colour's 0xRRGGBB, the first 16 from pal (ANSI order).
func colourRGB(c int, pal *[16]uint32) uint32 {
	switch {
	case c&rgbFlag != 0:
		return uint32(c) & 0xFFFFFF
	case c >= 0 && c < 16:
		return pal[c] & 0xFFFFFF
	}
	return xterm256(c)
}

func rgbDist(x, y uint32) int {
	d := 0
	for _, sh := range []uint{0, 8, 16} {
		e := int(x>>sh&255) - int(y>>sh&255)
		d += e * e
	}
	return d
}

// rgbColour is the colour for an RGB value: one of the 256 when it is exactly
// one of xterm's (16-255), else a 24-bit colour.
func rgbColour(rgb uint32) int {
	rgb &= 0xFFFFFF
	if n, ok := xtermIndex[rgb]; ok {
		return n
	}
	return rgbFlag | int(rgb)
}

var xtermIndex = func() map[uint32]int {
	m := map[uint32]int{}
	for n := 255; n >= 16; n-- {
		m[xterm256(n)] = n
	}
	return m
}()

// nearest256 is the one of xterm's 256 (16-255, which every terminal agrees on) closest to rgb.
func nearest256(rgb uint32) int {
	best, bd := 16, -1
	for n := 16; n < 256; n++ {
		if d := rgbDist(rgb, xterm256(n)); bd < 0 || d < bd {
			best, bd = n, d
		}
	}
	return best
}

// colourName names a colour for the status line and hints.
func colourName(c int) string {
	if c&rgbFlag != 0 {
		return "#" + strings.ToUpper(strconv.FormatInt(int64(c&0xFFFFFF)|1<<24, 16)[1:])
	}
	return strconv.Itoa(c)
}

// uniBlank: is a Unicode cell empty (a space on black, not underlined)?
func uniBlank(v uint32) bool {
	if c := uniCode(v); c != 32 && c != 0 {
		return false
	}
	_, bg, fl := styleParts(uniPair(v))
	return bg == 0 && fl&underline == 0
}

// ---------------------------------------------------------------- converting pages

// pcToUni is a PC (or Amiga) cell as a Unicode cell: the same colours, the
// character's Unicode.
func pcToUni(v uint32, amiga bool) uint32 {
	code, _ := uniCodeOf(byteRune(cellCh(v), amiga))
	if fl := v & (italic | underline); fl != 0 {
		return uniCell(code, styleOf(cellFg(v), cellBg(v), fl))
	}
	return uniCell(code, int(v>>8&255))
}

// uniToPC is a Unicode cell on a PC (or Amiga) page: the nearest it can be.
// exact is false when the character or a colour had to change.
func uniToPC(v uint32, amiga bool, pal *[16]uint32) (out uint32, exact bool) {
	r := uniRune(uniCode(v))
	ch, ok := -1, false
	switch {
	case r == ' ':
		ch, ok = 32, true
	case amiga:
		if r <= 0xFF && !(r < 0x20 || (r >= 0x7F && r < 0xA0)) {
			ch, ok = int(r), true
		}
	default:
		if b, in := cp437Rev[r]; in && b != 0 {
			ch, ok = int(b), true
		}
	}
	if !ok {
		ch = '?'
	}
	fg, bg, fl := styleParts(uniPair(v))
	p := uniPair(v)
	if p >= 256 {
		f, b := fg, bg
		if f > 15 {
			f = nearest16(colourRGB(fg, pal))
		}
		if b > 15 {
			b = nearest16(colourRGB(bg, pal))
		}
		p, ok = f|b<<4, ok && f == fg && b == bg
	}
	return makeCell(ch, p&15, p>>4, fl), ok
}

// docToUni / docFromUni convert every cell of a page.
func docToUni(d *Doc, amiga bool) *Doc {
	n := d.clone()
	for i, v := range n.cells[:n.rows*n.cols] {
		n.cells[i] = pcToUni(v, amiga)
	}
	n.uni = true
	return n
}

func docFromUni(d *Doc, amiga bool, pal *[16]uint32) (*Doc, int) {
	n := d.clone()
	changed := 0
	for i, v := range n.cells[:n.rows*n.cols] {
		o, exact := uniToPC(v, amiga, pal)
		if !exact {
			changed++
		}
		n.cells[i] = o
	}
	n.uni = false
	return n, changed
}

// convertUni makes the page a Unicode page (to = "uni") or turns a Unicode page
// into a PC or Amiga page, every frame, as one undo step.
func (a *App) convertUni(to string) {
	from := a.mode
	if to == from {
		return
	}
	if to == "uni" && from != "pc" && from != "amiga" {
		a.msg("Only PC and Amiga pages convert to Unicode")
		return
	}
	a.endPlay()
	changed := 0
	a.withEveryFrame(func(d *Doc) *Doc {
		if to == "uni" {
			return docToUni(d, from == "amiga")
		}
		n, c := docFromUni(d, to == "amiga", &a.pal)
		changed += c
		return n
	})
	a.mode, a.saveFmt, a.source, a.srcKind = to, map[bool]string{true: "uniansi", false: "ansi"}[to == "uni"], nil, ""
	a.ownFont, a.ownLower, a.ownPal = nil, nil, nil
	if to == "uni" {
		a.ice = true
		a.paletteFor["uni"] = a.paletteFor[from]
		if from == "amiga" {
			a.fontFor["uni"] = "unifont"
		}
	}
	// A new file (Save asks for its name): the original keeps its bytes.
	base := strings.TrimSuffix(strings.TrimSuffix(a.name, extOf(a.name)), ".utf8")
	a.name, a.path = base+map[bool]string{true: ".utf8.ans", false: ".ans"}[to == "uni"], ""
	a.dirty = true
	a.applyMode()
	switch {
	case to == "uni":
		a.msg("Converted to a Unicode page (" + keyName("undo") + " undoes it)")
	case changed > 0:
		a.msg(strconv.Itoa(changed) + " cells had characters or colours a " + modeNames[to] + " page lacks: now the nearest (" + keyName("undo") + " undoes it)")
	default:
		a.msg("Converted to a " + modeNames[to] + " page")
	}
}

func extOf(name string) string {
	if i := strings.LastIndexByte(name, '.'); i > 0 {
		return name[i:]
	}
	return ""
}

// ---------------------------------------------------------------- the brush

// uniOpts are the Unicode page's own brush settings (App embeds it).
type uniOpts struct {
	ufg, ubg     int  // the brush's colours beyond the 16 (with ufgOn, ubgOn)
	ufgOn, ubgOn bool // off: the brush takes fg / bg (0-15)
	uniBlock     int  // the block the character table shows (uniBlocks)
}

// uniFg / uniBg are the brush's colours on a Unicode page.
func (a *App) uniFg() int {
	if a.ufgOn {
		return a.ufg
	}
	return a.fg & 15
}

func (a *App) uniBg() int {
	if a.ubgOn {
		return a.ubg
	}
	return a.bg & 15
}

// setUniFg / setUniBg set the brush's colour (any of the 256 or 24-bit).
func (a *App) setUniFg(c int) {
	if c >= 0 && c < 16 {
		a.ufgOn = false
		a.setFg(c)
		return
	}
	a.ufg, a.ufgOn = c, true
}

func (a *App) setUniBg(c int) {
	if c >= 0 && c < 16 {
		a.ubgOn = false
		a.setBg(c)
		return
	}
	a.ubg, a.ubgOn = c, true
}

// uniCellOf is a cell of a character code in the brush's colours.
func (a *App) uniCellOf(code int) uint32 {
	return uniCell(code, styleOf(a.uniFg(), a.uniBg(), a.uniFlags()))
}

func (a *App) uniFlags() uint32 {
	var f uint32
	if a.ital {
		f |= italic
	}
	if a.under {
		f |= underline
	}
	return f
}

// uniFromRune is the code typing a character puts in, -1 if the page cannot show it.
func uniFromRune(r rune) int {
	if r < 0x20 || r == 0x7F {
		return -1
	}
	code, ok := uniCodeOf(r)
	if !ok {
		return -1
	}
	return code
}

// ---------------------------------------------------------------- the font

// Unifont's blocks for text art (tools/unifont.sh; OFL, fonts/unifont-NOTICE.txt).
//
//go:embed fonts/unifont-art.hex.gz
var unifontHex []byte

type uniFontData struct {
	at     []int32 // code -> offset in glyphs, -1 none
	glyphs []byte  // 16 rows of 8 pixels each
	wide   []bool  // code was 16 pixels wide (squeezed)
	mirror [3]map[int]int
}

var uniFontOnce sync.Once
var uniFontTab *uniFontData

func uniFont() *uniFontData {
	uniFontOnce.Do(func() {
		f := &uniFontData{at: make([]int32, 0x10000), wide: make([]bool, 0x10000)}
		for i := range f.at {
			f.at[i] = -1
		}
		zr, err := gzip.NewReader(bytes.NewReader(unifontHex))
		if err != nil {
			panic("unifont: " + err.Error())
		}
		sc := bufio.NewScanner(zr)
		for sc.Scan() {
			cp, glyph, ok := strings.Cut(sc.Text(), ":")
			if !ok {
				continue
			}
			r, err := strconv.ParseUint(cp, 16, 32)
			if err != nil {
				continue
			}
			code, ok := uniCodeOf(rune(r))
			if !ok {
				continue
			}
			b, err := hex.DecodeString(glyph)
			if err != nil {
				continue
			}
			var g [16]byte
			switch len(b) {
			case 16:
				copy(g[:], b)
			case 32: // 16 pixels wide: squeezed into the cell, two pixels to one
				for row := 0; row < 16; row++ {
					w := uint16(b[row*2])<<8 | uint16(b[row*2+1])
					var o byte
					for x := 0; x < 8; x++ {
						if w&(0xC000>>(2*x)) != 0 {
							o |= 0x80 >> x
						}
					}
					g[row] = o
				}
				f.wide[code] = true
			default:
				continue
			}
			f.at[code] = int32(len(f.glyphs))
			f.glyphs = append(f.glyphs, g[:]...)
		}
		uniFontTab = f
	})
	return uniFontTab
}

// uniGlyph is the 8x16 picture of a code; with vga, CP437's characters come from
// the IBM VGA font (a converted PC page looks as it did). ok is false when no font has it.
func uniGlyph(code int, vga bool) (g []byte, ok bool) {
	r := uniRune(code)
	if vga {
		if b, in := cp437Rev[r]; in && (b != 0 || r == 0) {
			if f := fontByID("vga"); f != nil {
				return f.bits[int(b)*16 : int(b)*16+16], true
			}
		}
	}
	f := uniFont()
	if code >= 0 && code < len(f.at) && f.at[code] >= 0 {
		o := f.at[code]
		return f.glyphs[o : o+16], true
	}
	return nil, false
}

// missingGlyph is drawn for a character no font has: a small box.
var missingGlyph = []byte{0, 0, 0x7E, 0x42, 0x42, 0x42, 0x42, 0x42, 0x42, 0x42, 0x42, 0x42, 0x7E, 0, 0, 0}

// uniMirror is the code that looks like code turned over (flipH, flipV,
// turnCW), code itself if none: CP437's characters as the PC font has them,
// the rest by Unifont's pixels (exact images only; letters never).
func uniMirror(code, k int) int {
	r := uniRune(code)
	if b, ok := cp437Rev[r]; ok && b != 0 {
		if f := fontByID("vga"); f != nil {
			c, _ := uniCodeOf(cp437[f.mapped(int(b), k)])
			return c
		}
	}
	if m, ok := uniTurns[k][r]; ok {
		c, _ := uniCodeOf(m)
		return c
	}
	if m, ok := uniDotsTurned(r, k); ok {
		c, _ := uniCodeOf(m)
		return c
	}
	if k == turnCW || unicode.IsLetter(r) || unicode.IsDigit(r) {
		return code
	}
	f := uniFont()
	uniMirrorOnce[k].Do(func() { f.mirror[k] = uniMirrorTable(f, k) })
	if m, ok := f.mirror[k][code]; ok {
		return m
	}
	return code
}

var uniMirrorOnce [3]sync.Once

// uniMirrorTable pairs the glyphs that are exact mirror images of each other.
func uniMirrorTable(f *uniFontData, k int) map[int]int {
	key := func(g []byte) string { return string(g) }
	flip := func(g []byte) []byte {
		o := make([]byte, 16)
		for r := 0; r < 16; r++ {
			if k == flipV {
				o[r] = g[15-r]
			} else {
				o[r] = bits.Reverse8(g[r])
			}
		}
		return o
	}
	byPic := map[string]int{}
	for code := 0x10000 - 1; code >= 0; code-- {
		if o := f.at[code]; o >= 0 && !f.wide[code] {
			byPic[key(f.glyphs[o:o+16])] = code
		}
	}
	m := map[int]int{}
	for code := 0; code < 0x10000; code++ {
		o := f.at[code]
		if o < 0 || f.wide[code] {
			continue
		}
		r := uniRune(code)
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			continue
		}
		g := f.glyphs[o : o+16]
		if other, ok := byPic[key(flip(g))]; ok && other != code && !bytes.Equal(g, make([]byte, 16)) {
			m[code] = other
		}
	}
	for c, o := range m { // keep pairs that agree both ways
		if m[o] != c {
			delete(m, c)
		}
	}
	return m
}

// uniTurns are the pairs and turns everyone knows, beyond CP437's.
var uniTurns = func() [3]map[rune]rune {
	t := [3]map[rune]rune{{}, {}, {}}
	pair := func(k int, x, y rune) { t[k][x], t[k][y] = y, x }
	for _, p := range [][2]rune{{'▌', '▐'}, {'▘', '▝'}, {'▖', '▗'}, {'▛', '▜'}, {'▙', '▟'}, {'▚', '▞'}, {'╭', '╮'}, {'╰', '╯'}, {'╱', '╲'}, {'◢', '◣'}, {'◤', '◥'}} {
		pair(flipH, p[0], p[1])
	}
	for _, p := range [][2]rune{{'▀', '▄'}, {'▘', '▖'}, {'▝', '▗'}, {'▛', '▙'}, {'▜', '▟'}, {'▚', '▞'}, {'╭', '╰'}, {'╮', '╯'}, {'╱', '╲'}, {'◢', '◥'}, {'◣', '◤'}} {
		pair(flipV, p[0], p[1])
	}
	for _, c := range [][]rune{{'─', '│'}, {'━', '┃'}, {'┌', '┐', '┘', '└'}, {'┏', '┓', '┛', '┗'}, {'├', '┬', '┤', '┴'}, {'╭', '╮', '╯', '╰'},
		{'▘', '▝', '▗', '▖'}, {'▛', '▜', '▟', '▙'}, {'▚', '▞'}, {'╱', '╲'}, {'◢', '◣', '◤', '◥'}} {
		for i, r := range c {
			t[turnCW][r] = c[(i+1)%len(c)]
		}
	}
	return t
}()

// ---------------------------------------------------------------- the page's lists

func init() {
	modeNames["uni"] = "Unicode"
	modeOrder = append(modeOrder, "uni")
	pageModes = append(pageModes, "uni")
	openAs = append(openAs, "uni")
	openAsNames = append(openAsNames, "Unicode (UTF-8)")
	vga := fontByID("vga")
	fonts = append(fonts,
		&Font{id: "univga", name: "VGA + Unifont", kind: "uni", syncterm: -1, bits: vga.bits},
		&Font{id: "unifont", name: "Unifont", kind: "uni", syncterm: -1, bits: vga.bits})
	palettes = append(palettes[:len(palettes)-1:len(palettes)-1], append([]*Palette{xtermPalette, mircPalette}, palettes[len(palettes)-1])...)
	for _, f := range []Format{
		{"uniansi", "UTF-8 ANSI, 256 and 24-bit colours (.ans)", ".ans", "uni"},
		{"dur", "durdraw (.dur)", ".dur", "uni"},
		{"mirc", "mIRC colour codes (.irc)", ".irc", "uni pc amiga"},
	} {
		formats = append(formats, f)
	}
	framesFormats["dur"] = true
	for i := range formats { // the formats every page has
		switch formats[i].id {
		case "utf8", "png", "png2", "gif", "apng", "html":
			formats[i].modes += " uni"
		}
	}
}

// xtermPalette is xterm's own 16 colours; mircPalette mIRC's 16 in ANSI order
// (mIRC 1 black, 5 brown, 3 green, 7 orange, 2 navy, 6 purple, 10 teal, 15 light
// grey, 14 grey, 4 red, 9 light green, 8 yellow, 12 blue, 13 pink, 11 cyan, 0 white).
var xtermPalette = func() *Palette {
	p := &Palette{id: "xterm", name: "xterm"}
	for i, c := range []uint32{0x000000, 0xCD0000, 0x00CD00, 0xCDCD00, 0x0000EE, 0xCD00CD, 0x00CDCD, 0xE5E5E5,
		0x7F7F7F, 0xFF0000, 0x00FF00, 0xFFFF00, 0x5C5CFF, 0xFF00FF, 0x00FFFF, 0xFFFFFF} {
		p.colors[i] = 0xFF000000 | c
	}
	return p
}()

var mircPalette = func() *Palette {
	p := &Palette{id: "mirc", name: "mIRC"}
	for i, m := range ansiToMirc {
		p.colors[i] = 0xFF000000 | mircRGB[m]
	}
	return p
}()

// uniPairsSnapshot is the style table as it is now (style 256 + i), for an autosave.
func uniPairsSnapshot() [][3]int {
	uniStyles.RLock()
	defer uniStyles.RUnlock()
	out := make([][3]int, len(uniStyles.list))
	for i, c := range uniStyles.list {
		out[i] = [3]int{c.fg, c.bg, int(c.flags)}
	}
	return out
}

// uniRemapPairs gives cells saved with another run's pair table this run's pairs.
func uniRemapPairs(docs []*Doc, saved [][3]int) {
	cache := map[int]int{}
	for _, d := range docs {
		d.uni = true
		for i, v := range d.cells[:d.rows*d.cols] {
			p := uniPair(v)
			if p < 256 {
				continue
			}
			n, ok := cache[p]
			if !ok {
				n = 7
				if k := p - 256; k < len(saved) {
					n = styleOf(saved[k][0], saved[k][1], uint32(saved[k][2]))
				}
				cache[p] = n
			}
			d.cells[i] = uniWithPair(v, n)
		}
	}
}

// uniOwn: fmtID is a Unicode page's own file (read back as it was written), not an export.
func (a *App) uniOwn(fmtID string) bool {
	return a.uni() && (fmtID == "uniansi" || fmtID == "dur" || fmtID == "mirc")
}

// uniCompact lets go of the styles no cell uses any more (pages closed, undo
// steps dropped), when the table is three quarters full. It renumbers the cells
// of every page, frame, undo step, block and the clipboard, so it runs only at
// a safe point: no requester open (they may hold pictures), nothing playing.
// It returns the styles freed.
func (a *App) uniCompact() int {
	uniStyles.RLock()
	n := len(uniStyles.list)
	uniStyles.RUnlock()
	if n < (uniPairMax-256)*3/4 || a.dialog != nil {
		return 0
	}
	for _, t := range a.tabs {
		if t.playing != nil {
			return 0
		}
	}
	// Every cell slice there is, each once (undo steps and frames share pictures).
	seen := map[*uint32]bool{}
	var slices [][]uint32
	add := func(c []uint32) {
		if len(c) > 0 && !seen[&c[0]] {
			seen[&c[0]] = true
			slices = append(slices, c)
		}
	}
	// A page's pictures are Unicode cells when the page (or the undo step) is a
	// Unicode page's; steps from before a conversion hold PC cells.
	addDocs := func(mode string, d *Doc, fr []Frame) {
		if mode != "uni" {
			return
		}
		if d != nil {
			add(d.cells)
		}
		for _, f := range fr {
			if f.doc != nil {
				add(f.doc.cells)
			}
		}
	}
	for _, t := range a.tabs {
		addDocs(t.mode, t.doc, t.frames)
		for _, s := range append(append([]Snap(nil), t.undo...), t.redo...) {
			addDocs(s.mode, s.doc, s.frames)
		}
		if t.float != nil && t.float.uni {
			add(t.float.cells)
		}
	}
	if a.clip != nil && a.clip.uni {
		add(a.clip.cells)
	}
	uniStyles.Lock()
	defer uniStyles.Unlock()
	used := make([]bool, len(uniStyles.list))
	for _, c := range slices {
		for _, v := range c {
			if p := uniPair(v); p >= 256 && p-256 < len(used) {
				used[p-256] = true
			}
		}
	}
	renum := make([]int, len(uniStyles.list))
	var keep []uniStyle
	for i, u := range used {
		if u {
			renum[i] = 256 + len(keep)
			keep = append(keep, uniStyles.list[i])
		}
	}
	freed := len(uniStyles.list) - len(keep)
	if freed == 0 {
		return 0
	}
	for _, c := range slices {
		for i, v := range c {
			if p := uniPair(v); p >= 256 && p-256 < len(renum) {
				c[i] = uniWithPair(v, renum[p-256])
			}
		}
	}
	uniStyles.list = keep
	uniStyles.index = map[uniStyle]int{}
	for i, s := range keep {
		uniStyles.index[s] = 256 + i
	}
	uniStyles.full = false
	return freed
}

// uniFullNote is what the status line says when the style table ran out.
func uniFullNote() string {
	uniStyles.RLock()
	defer uniStyles.RUnlock()
	if !uniStyles.full {
		return ""
	}
	return fmt.Sprintf(" (more than %d colour combinations in this session: new ones take the nearest; close pages you are done with)", uniPairMax-256)
}

// uniDotsTurned turns Braille (2x4 dots) and sextants (2x3 blocks) over by their bits.
func uniDotsTurned(r rune, k int) (rune, bool) {
	if k == turnCW {
		return r, false
	}
	switch {
	case r >= 0x2800 && r <= 0x28FF: // dots 1-3 and 7 the left column, 4-6 and 8 the right
		b := int(r - 0x2800)
		perm := [8]int{3, 4, 5, 0, 1, 2, 7, 6} // left-right: dot n goes to perm[n]
		if k == flipV {
			perm = [8]int{6, 2, 1, 7, 5, 4, 0, 3}
		}
		o := 0
		for i, to := range perm {
			if b&(1<<i) != 0 {
				o |= 1 << to
			}
		}
		return rune(0x2800 + o), true
	case r >= 0x1FB00 && r <= 0x1FB3B: // sextants: patterns 1-62 without 21 (▌) and 42 (▐)
		p := int(r-0x1FB00) + 1
		if p >= 21 {
			p++
		}
		if p >= 42 {
			p++
		}
		o := 0
		for i := 0; i < 6; i++ {
			if p&(1<<i) == 0 {
				continue
			}
			row, col := i/2, i%2
			if k == flipH {
				col = 1 - col
			} else {
				row = 2 - row
			}
			o |= 1 << (row*2 + col)
		}
		switch o {
		case 0:
			return ' ', true
		case 21:
			return '▌', true
		case 42:
			return '▐', true
		case 63:
			return '█', true
		}
		if o > 42 {
			o--
		}
		if o > 21 {
			o--
		}
		return rune(0x1FB00 + o - 1), true
	}
	return r, false
}
