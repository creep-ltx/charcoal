package main

// TheDraw fonts (.tdf): the big lettering ANSI artists type logos with. A file
// holds one or more fonts of three kinds (TheDraw 4.63's manual, "The Font
// Editor"; the layout as TheDraw 4.63's TDFONTS.TDF and Roy/SAC's collection have
// it, the same as IcyDraw's retrofont reads it):
//
//	file:  19, "TheDraw FONTS file", 0x1A, then fonts until a 0 byte (or the end)
//	font:  55 AA 00 FF, name length, name (12 bytes, 0-padded), 4 bytes (0),
//	       type (0 outline, 1 block, 2 colour), spacing (1-41: the columns
//	       between letters + 1, tdfFont.gap), u16 size of the
//	       glyph block, 94 u16 offsets into it for '!'..'~' (FFFF: none), the block
//	glyph: width, height (the lines not marked as descenders), then the cells to
//	       a 0 byte: 13 ends a line, '&' at the end of a line marks it as a
//	       descender (not a cell: TheDraw leaves those lines out when it lines
//	       letters up; an '&' inside a line is a character, as the fonts' widths
//	       show), a colour font's cells are character + DOS attribute
//
// The cells: a block font draws its characters in the brush's colours, 0xFF is a
// "solid space" (a space that is drawn). An outline font holds letters A-N that
// become the line or block characters of one of TheDraw's 19 outline styles
// (tdfOutlines, read out of THEDRAW.EXE), O and P are solid spaces (TheDraw's
// table has 0xFF and a space there), @ the solid space that fills columns and
// beams. Block and outline letters fill their box (width x lines) with spaces in
// the brush's colours, as IcyDraw does; a space in a glyph leaves that blank. A
// colour font keeps its own colours: its space on a black background is left out
// (the page shows through), a space on a colour is drawn (half of Roy/SAC's colour
// fonts draw with coloured spaces).
//
// tdf_ui.go has the Logo tool, the font picker, the line requester and the fonts'
// download; this file reads fonts and lays text out.

import (
	"crypto/sha1"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"unicode"
)

const (
	tdfOutline = 0
	tdfBlock   = 1
	tdfColour  = 2

	tdfFirst = '!'
	tdfChars = 94 // '!' to '~'

	tdfMaxRows = 32   // more lines than any real glyph (12): a broken one stops there
	tdfMaxCols = 255  // the width is a byte
	tdfLineMax = 2000 // columns a laid-out line may take
)

var tdfKindNames = []string{"Outline", "Block", "Colour"}

// tdfOutlines are TheDraw's 19 outline styles: the characters A to P stand for
// (THEDRAW.EXE 4.63 at 0x2305B, 16 bytes each; *verified* by reading the file). A/B
// are the top and bottom of a beam, C/D a column's left and right, E/F the outer
// upper corners, G/H the inner upper ones, I/J the inner lower, K/L the outer lower,
// M/N the joins; O (0xFF) and P are blanks.
var tdfOutlines = [19][16]byte{
	{0xC4, 0xC4, 0xB3, 0xB3, 0xDA, 0xBF, 0xDA, 0xBF, 0xC0, 0xD9, 0xC0, 0xD9, 0xB4, 0xC3, 0xFF, 0x20},
	{0xCD, 0xC4, 0xB3, 0xB3, 0xD5, 0xB8, 0xDA, 0xBF, 0xD4, 0xBE, 0xC0, 0xD9, 0xB5, 0xC3, 0xFF, 0x20},
	{0xC4, 0xCD, 0xB3, 0xB3, 0xDA, 0xBF, 0xD5, 0xB8, 0xC0, 0xD9, 0xD4, 0xBE, 0xB4, 0xC6, 0xFF, 0x20},
	{0xCD, 0xCD, 0xB3, 0xB3, 0xD5, 0xB8, 0xD5, 0xB8, 0xD4, 0xBE, 0xD4, 0xBE, 0xB5, 0xC6, 0xFF, 0x20},
	{0xC4, 0xC4, 0xBA, 0xB3, 0xD6, 0xBF, 0xDA, 0xB7, 0xC0, 0xBD, 0xD3, 0xD9, 0xB6, 0xC3, 0xFF, 0x20},
	{0xCD, 0xC4, 0xBA, 0xB3, 0xC9, 0xB8, 0xDA, 0xB7, 0xD4, 0xBC, 0xD3, 0xD9, 0xB9, 0xC3, 0xFF, 0x20},
	{0xC4, 0xCD, 0xBA, 0xB3, 0xD6, 0xBF, 0xD5, 0xBB, 0xC0, 0xBD, 0xC8, 0xBE, 0xB6, 0xC6, 0xFF, 0x20},
	{0xCD, 0xCD, 0xBA, 0xB3, 0xC9, 0xB8, 0xD5, 0xBB, 0xD4, 0xBC, 0xC8, 0xBE, 0xB9, 0xC6, 0xFF, 0x20},
	{0xC4, 0xC4, 0xB3, 0xBA, 0xDA, 0xB7, 0xD6, 0xBF, 0xD3, 0xD9, 0xC0, 0xBD, 0xB4, 0xC7, 0xFF, 0x20},
	{0xCD, 0xC4, 0xB3, 0xBA, 0xD5, 0xBB, 0xD6, 0xBF, 0xC8, 0xBE, 0xC0, 0xBD, 0xB5, 0xC7, 0xFF, 0x20},
	{0xC4, 0xCD, 0xB3, 0xBA, 0xDA, 0xB7, 0xC9, 0xB8, 0xD3, 0xD9, 0xD4, 0xBC, 0xB4, 0xCC, 0xFF, 0x20},
	{0xCD, 0xCD, 0xB3, 0xBA, 0xD5, 0xBB, 0xC9, 0xB8, 0xC8, 0xBE, 0xD4, 0xBC, 0xB5, 0xCC, 0xFF, 0x20},
	{0xC4, 0xC4, 0xBA, 0xBA, 0xD6, 0xB7, 0xD6, 0xB7, 0xD3, 0xBD, 0xD3, 0xBD, 0xB6, 0xC7, 0xFF, 0x20},
	{0xCD, 0xC4, 0xBA, 0xBA, 0xC9, 0xBB, 0xD6, 0xB7, 0xC8, 0xBC, 0xD3, 0xBD, 0xB9, 0xC7, 0xFF, 0x20},
	{0xC4, 0xCD, 0xBA, 0xBA, 0xD6, 0xB7, 0xC9, 0xBB, 0xD3, 0xBD, 0xC8, 0xBC, 0xB6, 0xCC, 0xFF, 0x20},
	{0xCD, 0xCD, 0xBA, 0xBA, 0xC9, 0xBB, 0xC9, 0xBB, 0xC8, 0xBC, 0xC8, 0xBC, 0xB9, 0xCC, 0xFF, 0x20},
	{0xDC, 0xDC, 0xDB, 0xDB, 0xDC, 0xDC, 0xDC, 0xDC, 0xDB, 0xDB, 0xDB, 0xDB, 0xDB, 0xDB, 0xFF, 0x20},
	{0xDF, 0xDF, 0xDB, 0xDB, 0xDB, 0xDB, 0xDB, 0xDB, 0xDF, 0xDF, 0xDF, 0xDF, 0xDB, 0xDB, 0xFF, 0x20},
	{0xDF, 0xDC, 0xDE, 0xDD, 0xDE, 0xDD, 0xDC, 0xDC, 0xDF, 0xDF, 0xDE, 0xDD, 0xDB, 0xDB, 0xFF, 0x20},
}

// tdfFont is one font of a .tdf file. Its glyphs are read when first used.
type tdfFont struct {
	name    string
	kind    int // tdfOutline, tdfBlock, tdfColour
	spacing int // the file's spacing byte: TheDraw puts one column less between letters (gap)
	file    string
	index   int    // the font's place in its file
	sum     string // what the font holds (SHA-1): the same font in two files is listed once
	tab     [tdfChars]uint16
	blk     []byte
	glyphs  [tdfChars]*tdfGlyph
	read    [tdfChars]bool
	lineH   int // the lines a Return moves down (0: not worked out yet)
}

// tdfCell is a cell of a glyph or of laid-out text.
type tdfCell struct {
	kind uint8 // tdfNone (the page shows through), tdfInk (brush colours), tdfOwn (its attr)
	ch   byte  // CP437
	attr byte  // DOS attribute: fg | bg<<4 (bit 7 blink or bright background)
}

const (
	tdfNone uint8 = iota
	tdfInk
	tdfOwn
)

// tdfGlyph is a glyph as read: rows of cells as the file has them (outline
// letters still letters).
type tdfGlyph struct {
	w, h int // the file's width and height (lines without descenders)
	rows [][]tdfCell
	desc []bool // the row is a descender (marked '&')
}

var errNotTDF = errors.New("not a TheDraw font file")

// isTDF: does the file start as a TheDraw font file?
func isTDF(b []byte) bool {
	return len(b) >= 20 && b[0] == 19 && string(b[1:19]) == "TheDraw FONTS file" && b[19] == 0x1A
}

// parseTDF reads the fonts of a .tdf file. A font cut short or broken ends the
// list (the fonts before it are kept; err says what was wrong when none were read).
func parseTDF(b []byte, file string) ([]*tdfFont, error) {
	if !isTDF(b) {
		return nil, errNotTDF
	}
	var fonts []*tdfFont
	o := 20
	fail := func(why string) ([]*tdfFont, error) {
		if len(fonts) > 0 {
			return fonts, nil
		}
		return nil, fmt.Errorf("%s: %s", "TheDraw font file", why)
	}
	for o < len(b) && b[o] != 0 {
		if o+4+1+12+4+4+2*tdfChars > len(b) {
			return fail("cut short")
		}
		if binary.LittleEndian.Uint32(b[o:]) != 0xFF00AA55 {
			return fail("a font does not start with 55 AA 00 FF")
		}
		o += 4
		n := min(int(b[o]), 12)
		name := b[o+1 : o+1+n]
		if i := strings.IndexByte(string(name), 0); i >= 0 {
			name = name[:i]
		}
		o += 1 + 12 + 4
		f := &tdfFont{name: tdfName(name), kind: int(b[o]), spacing: int(b[o+1]), file: file, index: len(fonts)}
		if f.kind > tdfColour {
			return fail(fmt.Sprintf("font %q is of an unknown type %d", f.name, f.kind))
		}
		size := int(binary.LittleEndian.Uint16(b[o+2:]))
		o += 4
		for i := range f.tab {
			f.tab[i] = binary.LittleEndian.Uint16(b[o+2*i:])
		}
		o += 2 * tdfChars
		if o+size > len(b) {
			return fail(fmt.Sprintf("font %q is cut short", f.name))
		}
		f.blk = b[o : o+size : o+size]
		o += size
		h := sha1.New()
		h.Write([]byte{byte(f.kind), byte(f.spacing)})
		h.Write([]byte(f.name))
		h.Write(b[o-size-2*tdfChars : o])
		f.sum = fmt.Sprintf("%x", h.Sum(nil))
		fonts = append(fonts, f)
	}
	if len(fonts) == 0 {
		return nil, errors.New("the TheDraw font file holds no fonts")
	}
	return fonts, nil
}

// tdfName is a font's name as text: CP437 bytes, trimmed.
func tdfName(b []byte) string {
	var sb strings.Builder
	for _, c := range b {
		sb.WriteRune(byteRune(int(c), false))
	}
	if s := strings.TrimSpace(sb.String()); s != "" {
		return s
	}
	return "(no name)"
}

// has: does the font have a glyph for the character?
func (f *tdfFont) has(c rune) bool {
	return c >= tdfFirst && c < tdfFirst+tdfChars && int(f.tab[c-tdfFirst]) < len(f.blk)
}

// resolve is the character the font draws for c: c, or its other case when only
// that is there (most fonts have capitals only); -1 when neither is there.
func (f *tdfFont) resolve(c rune) rune {
	switch {
	case f.has(c):
		return c
	case f.has(unicode.ToUpper(c)):
		return unicode.ToUpper(c)
	case f.has(unicode.ToLower(c)):
		return unicode.ToLower(c)
	}
	return -1
}

// glyph reads the glyph of c (nil when the font has none).
func (f *tdfFont) glyph(c rune) *tdfGlyph {
	if !f.has(c) {
		return nil
	}
	i := c - tdfFirst
	if !f.read[i] {
		f.read[i] = true
		f.glyphs[i] = f.decode(int(f.tab[i]))
	}
	return f.glyphs[i]
}

func (f *tdfFont) decode(p int) *tdfGlyph {
	b := f.blk
	if p+2 > len(b) {
		return nil
	}
	g := &tdfGlyph{w: int(b[p]), h: int(b[p+1]), rows: [][]tdfCell{nil}, desc: []bool{false}}
	p += 2
	for p < len(b) && b[p] != 0 {
		c := b[p]
		p++
		last := len(g.rows) - 1
		switch {
		case c == 13:
			if len(g.rows) == tdfMaxRows {
				return g
			}
			g.rows, g.desc = append(g.rows, nil), append(g.desc, false)
			continue
		case c == '&' && (p >= len(b) || b[p] == 0 || b[p] == 13):
			g.desc[last] = true // the mark ends its line; elsewhere '&' is a character
			continue
		}
		cell := tdfCell{kind: tdfInk, ch: c}
		if f.kind == tdfColour {
			if p >= len(b) {
				break
			}
			cell = tdfCell{kind: tdfOwn, ch: c, attr: b[p]}
			p++
		}
		if len(g.rows[last]) < tdfMaxCols {
			g.rows[last] = append(g.rows[last], cell)
		}
	}
	// A last line break with nothing after it is no line.
	for len(g.rows) > 1 && len(g.rows[len(g.rows)-1]) == 0 && !g.desc[len(g.rows)-1] {
		g.rows, g.desc = g.rows[:len(g.rows)-1], g.desc[:len(g.rows)-1]
	}
	return g
}

// width is how many columns the glyph takes: the file's width, or its longest line if longer.
func (g *tdfGlyph) width() int {
	w := g.w
	for _, r := range g.rows {
		w = max(w, len(r))
	}
	return min(w, tdfMaxCols)
}

// body is the glyph's lines that are not descenders.
func (g *tdfGlyph) body() int {
	n := 0
	for _, d := range g.desc {
		if !d {
			n++
		}
	}
	return n
}

// lineHeight is how far a Return moves down: the tallest glyph's lines without its descenders.
func (f *tdfFont) lineHeight() int {
	if f.lineH == 0 {
		f.lineH = 1
		for c := rune(tdfFirst); c < tdfFirst+tdfChars; c++ {
			if g := f.glyph(c); g != nil {
				f.lineH = max(f.lineH, g.body())
			}
		}
	}
	return f.lineH
}

// gap is the columns TheDraw puts between letters: the file's spacing byte less
// one (TheDraw's TDFONTS shows the Outline font's 2 as "Font spacing is 1
// character", and TheDraw types its letters one column apart; *verified* in
// TheDraw 4.63 under QEMU). The manual's "legal values are 0 to 40" are gaps.
func (f *tdfFont) gap() int { return max(0, f.spacing-1) }

// spaceWidth is how far a space moves on: half the letters' usual width.
func (f *tdfFont) spaceWidth() int {
	total, n := 0, 0
	for _, c := range "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz" {
		if g := f.glyph(c); g != nil {
			total += g.width()
			n++
		}
	}
	if n == 0 {
		return max(1, f.gap())
	}
	return max(1, (total/n+1)/2)
}

// tdfPic is a glyph or a line of text ready for the page.
type tdfPic struct {
	w, h  int
	cells []tdfCell
}

func newTDFPic(w, h int) *tdfPic {
	return &tdfPic{w: w, h: h, cells: make([]tdfCell, w*h)}
}

func (p *tdfPic) at(x, y int) tdfCell {
	if x < 0 || y < 0 || x >= p.w || y >= p.h {
		return tdfCell{}
	}
	return p.cells[y*p.w+x]
}

func (p *tdfPic) set(x, y int, c tdfCell) {
	if x >= 0 && y >= 0 && x < p.w && y < p.h {
		p.cells[y*p.w+x] = c
	}
}

// render makes the picture of character c in outline style style (0-18); nil
// when the font has no glyph for it (in either case).
func (f *tdfFont) render(c rune, style int) *tdfPic {
	g := f.glyph(f.resolve(c))
	if g == nil {
		return nil
	}
	p := newTDFPic(g.width(), len(g.rows))
	if f.kind != tdfColour { // the box is filled with spaces in the brush's colours (descenders: only what they draw)
		for y := range g.rows {
			if !g.desc[y] {
				for x := 0; x < p.w; x++ {
					p.set(x, y, tdfCell{kind: tdfInk, ch: ' '})
				}
			}
		}
	}
	style = max(0, min(len(tdfOutlines)-1, style))
	for y, row := range g.rows {
		for x, cell := range row {
			switch f.kind {
			case tdfColour:
				switch {
				case cell.ch == ' ' && cell.attr&0xF0 == 0:
					continue // nothing: the page shows through
				case cell.ch == 0xFF:
					cell.ch = ' '
				}
			case tdfBlock:
				switch cell.ch {
				case ' ':
					continue
				case 0xFF:
					cell.ch = ' '
				}
			case tdfOutline:
				switch ch := cell.ch; {
				case ch == ' ':
					continue
				case ch == '@' || ch == 0xFF:
					cell.ch = ' '
				case ch >= 'A' && ch <= 'P':
					cell.ch = tdfOutlines[style][ch-'A']
					if cell.ch == 0xFF {
						cell.ch = ' '
					}
				}
			}
			p.set(x, y, cell)
		}
	}
	return p
}

// tdfLine is a line of text laid out in a font.
type tdfLine struct {
	pic     *tdfPic
	missing []rune // characters the font has no glyph for (left out)
}

// layout lays text out in one line: each glyph, then spacing columns (spacing < 0:
// the font's own); a space moves on by spaceWidth.
func (f *tdfFont) layout(text string, style, spacing int) tdfLine {
	if spacing < 0 {
		spacing = f.gap()
	}
	var pics []*tdfPic
	var xs []int
	var out tdfLine
	x, h := 0, 1
	for _, r := range text {
		if x > tdfLineMax {
			break // a line wider than any page: the rest is left out
		}
		if r == ' ' {
			x += f.spaceWidth()
			continue
		}
		p := f.render(r, style)
		if p == nil {
			if !strings.ContainsRune(string(out.missing), r) {
				out.missing = append(out.missing, r)
			}
			continue
		}
		if len(pics) > 0 {
			x += spacing
		}
		pics, xs = append(pics, p), append(xs, x)
		x += p.w
		h = max(h, p.h)
	}
	out.pic = newTDFPic(max(1, x), h)
	for i, p := range pics {
		for y := 0; y < p.h; y++ {
			for px := 0; px < p.w; px++ {
				if c := p.at(px, y); c.kind != tdfNone {
					out.pic.set(xs[i]+px, y, c)
				}
			}
		}
	}
	return out
}

// ---------------------------------------------------------------- writing fonts (the font editor, fed.go)

// rawGlyph is glyph i as the file holds it: width, height, the cells and the 0
// that ends them (nil when the font has none or it is cut short).
func (f *tdfFont) rawGlyph(i int) []byte {
	p := int(f.tab[i])
	if p+2 > len(f.blk) {
		return nil
	}
	for q := p + 2; q < len(f.blk); q++ {
		c := f.blk[q]
		if c == 0 {
			return f.blk[p : q+1 : q+1]
		}
		if f.kind == tdfColour && c != 13 && !(c == '&' && (q+1 >= len(f.blk) || f.blk[q+1] == 0 || f.blk[q+1] == 13)) {
			q++ // the attribute (which may be 0)
		}
	}
	return nil
}

// tdfLoss counts what a glyph's page holds that a TheDraw font cannot.
type tdfLoss struct {
	cr   int // character 13 (the font's line end): written as a space
	amp  int // an '&' TheDraw would read as a line's end (in light magenta on black in a colour font, at a block letter's right edge): a space
	nul  int // character 0 (the glyph's end): written as TheDraw's solid space, 0xFF, which looks the same
	flag int // italic or underline: not kept
}

func (l tdfLoss) any() bool { return l.cr+l.amp > 0 }

func (l *tdfLoss) add(m tdfLoss) {
	l.cr, l.amp, l.nul, l.flag = l.cr+m.cr, l.amp+m.amp, l.nul+m.nul, l.flag+m.flag
}

// encodeTDFGlyph writes a glyph from its page: the page's width, the lines that
// are not descenders as its height, then each row's cells (a block or outline
// font's characters; a colour font's character and DOS attribute), 13 between
// rows, '&' at the end of each descender row, a 0 at the end. A row's blank end
// is left out (a space in a block font, a space on black in a colour font: both
// leave the page as it is, as no cell does); a last row with nothing left keeps
// one space, as the reader drops a last empty line.
func encodeTDFGlyph(kind int, d *Doc, desc []bool) ([]byte, tdfLoss, error) {
	var loss tdfLoss
	if d.cols > tdfMaxCols || d.rows > 255 {
		return nil, loss, fmt.Errorf("a letter of %dx%d: TheDraw's are at most 255x255", d.cols, d.rows)
	}
	isDesc := func(y int) bool { return y < len(desc) && desc[y] }
	h := 0
	for y := 0; y < d.rows; y++ {
		if !isDesc(y) {
			h++
		}
	}
	out := []byte{byte(d.cols), byte(h)}
	transparent := func(v uint32) bool {
		return cellCh(v) == ' ' && (kind != tdfColour || cellAttr(v)&0xF0 == 0)
	}
	for y := 0; y < d.rows; y++ {
		if y > 0 {
			out = append(out, 13)
		}
		end := d.cols
		for end > 0 && transparent(d.get(end-1, y)) {
			end--
		}
		if end == 0 && y == d.rows-1 && !isDesc(y) {
			end = 1 // the last line kept
		}
		for x := 0; x < end; x++ {
			v := d.get(x, y)
			c, at := byte(cellCh(v)), cellAttr(v)
			if v&(italic|underline) != 0 {
				loss.flag++
			}
			switch {
			case c == 0:
				c = 0xFF
				loss.nul++
			case c == 13:
				c = ' '
				if kind == tdfColour {
					c = 0xFF // keeps its colours
				}
				loss.cr++
			case c == '&' && kind == tdfColour && (at == 0 || at == 13):
				if at == 0 {
					c = 0xFF // black on black: a solid space looks the same
				} else {
					c = 0xFF
					loss.amp++
				}
			}
			out = append(out, c)
			if kind == tdfColour {
				out = append(out, at)
			}
		}
		if kind != tdfColour && end > 0 && cellCh(d.get(end-1, y)) == '&' && !isDesc(y) {
			// An '&' at a line's end would mark a descender: a space after it
			// (one that leaves the page as it is), or none at the right edge.
			if end < d.cols {
				out = append(out, ' ')
			} else {
				out[len(out)-1] = ' '
				loss.amp++
			}
		}
		if isDesc(y) {
			out = append(out, '&')
		}
	}
	return append(out, 0), loss, nil
}

// encodeTDFFont writes one font: name (12 bytes, CP437), kind, spacing (the byte: gap + 1) and the
// glyphs ('!' to '~'; nil: not in the font), as TheDraw's TDFONTS writes them.
func encodeTDFFont(name string, kind, spacing int, glyphs *[tdfChars][]byte) ([]byte, error) {
	var nb []byte
	for _, r := range name {
		if r >= 32 && r < 127 {
			nb = append(nb, byte(r))
		} else if c, ok := cp437Rev[r]; ok && c >= 32 {
			nb = append(nb, c)
		} else {
			nb = append(nb, '?')
		}
	}
	nb = nb[:min(len(nb), 12)]
	out := []byte{0x55, 0xAA, 0x00, 0xFF, byte(len(nb))}
	out = append(out, nb...)
	out = append(out, make([]byte, 12-len(nb)+4)...)
	out = append(out, byte(kind), byte(max(0, min(spacing, 41)))) // the gap + 1
	var blk, tab []byte
	for _, g := range glyphs {
		if g == nil {
			tab = binary.LittleEndian.AppendUint16(tab, 0xFFFF)
			continue
		}
		if len(blk) >= 0xFFFF {
			return nil, errors.New("the font is too big for a TheDraw file (64 KB of letters at most)")
		}
		tab = binary.LittleEndian.AppendUint16(tab, uint16(len(blk)))
		blk = append(blk, g...)
	}
	if len(blk) > 0xFFFF {
		return nil, errors.New("the font is too big for a TheDraw file (64 KB of letters at most)")
	}
	out = binary.LittleEndian.AppendUint16(out, uint16(len(blk)))
	out = append(out, tab...)
	return append(out, blk...), nil
}

// tdfFileBytes is a .tdf file of fonts (each from encodeTDFFont): the header and
// the fonts, nothing after them. TheDraw's TDFONTS writes no 0 at the end and
// refuses a file with one ("Header read fault": it reads another font's header;
// *verified* with TDFONTS 4.63 under QEMU), though IcyDraw writes one.
func tdfFileBytes(fonts ...[]byte) []byte {
	out := []byte("\x13TheDraw FONTS file\x1a")
	for _, f := range fonts {
		out = append(out, f...)
	}
	return out
}
