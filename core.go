package main

// The document, SAUCE, the ANSI parser and the ANSI writer. No window here,
// so the tests run without a display.
//
// A cell is one 32-bit number:
//   bits 0-7   the byte (Latin-1 on an Amiga page, CP437 on a PC page)
//   bits 8-11  foreground 0-15 (8-15 = bold / bright)
//   bits 12-15 background 0-15 (8-15 = blink, shown as bright with iCE colours)
//   bit 16     italic (Amiga), bit 17 underline (Amiga)

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	italic    = 1 << 16
	underline = 1 << 17
)

func cellCh(v uint32) int { return int(v & 255) }
func cellFg(v uint32) int { return int(v>>8) & 15 }
func cellBg(v uint32) int { return int(v>>12) & 15 }
func makeCell(ch, fg, bg int, flags uint32) uint32 {
	return uint32(ch&255) | uint32(fg&15)<<8 | uint32(bg&15)<<12 | flags
}

var blank = makeCell(32, 7, 0, 0)

type Doc struct {
	cols, rows int
	cells      []uint32
	bg         int  // the screen colour of a C64 page
	border     int  // its border colour, -1 for the same as bg
	ink        int  // the one text colour of an Atari page
	glyph0     bool // character 0 is a glyph (C64 "@", Atari heart), not an empty cell
	fullest    bool // an animation whose fullest picture is shown, not its last
}

func newDoc(cols, rows int) *Doc {
	d := &Doc{cols: cols, rows: rows, cells: make([]uint32, cols*max(rows, 1)), border: -1}
	fillCells(d.cells, blank)
	return d
}

func fillCells(c []uint32, v uint32) {
	for i := range c {
		c[i] = v
	}
}

// The largest page: rows, and cells in all (64 MB). Files and emulators never grow a page past these,
// so a hostile file cannot take all the memory.
const (
	maxPageRows  = 100000
	maxPageCells = 16 << 20
)

// maxRowsFor is the most rows a page this wide may have.
func maxRowsFor(cols int) int { return min(maxPageRows, maxPageCells/max(1, cols)) }

func (d *Doc) get(x, y int) uint32 {
	if x < 0 || y < 0 || x >= d.cols || y >= d.rows {
		return blank
	}
	return d.cells[y*d.cols+x]
}

func (d *Doc) set(x, y int, v uint32) {
	if x < 0 || y < 0 || x >= d.cols {
		return
	}
	if y >= d.rows {
		d.ensureRows(y + 1)
	}
	d.cells[y*d.cols+x] = v
}

func (d *Doc) ensureRows(n int) {
	if n <= d.rows {
		return
	}
	need := n * d.cols
	if need > len(d.cells) {
		grown := make([]uint32, max(need, len(d.cells)*2))
		copy(grown, d.cells[:d.rows*d.cols])
		fillCells(grown[d.rows*d.cols:], blank)
		d.cells = grown
	}
	d.rows = n
}

func (d *Doc) clone() *Doc {
	c := &Doc{cols: d.cols, rows: d.rows, cells: make([]uint32, d.rows*d.cols), bg: d.bg, border: d.border, ink: d.ink, glyph0: d.glyph0}
	copy(c.cells, d.cells[:d.rows*d.cols])
	if d.rows == 0 {
		c.cells = make([]uint32, d.cols)
		fillCells(c.cells, blank)
	}
	return c
}

func isBlank(v uint32) bool {
	ch := v & 255
	return (ch == 32 || ch == 0) && cellBg(v) == 0 && v&underline == 0
}

// blankAt: is v an empty cell on this page? (On C64 and Atari pages character 0 is drawn.)
func (d *Doc) blankAt(v uint32) bool {
	if d.glyph0 && v&255 == 0 {
		return false
	}
	return isBlank(v)
}

// inkCells counts the cells that are not empty.
func (d *Doc) inkCells() int {
	n := 0
	for _, v := range d.cells[:d.rows*d.cols] {
		if !d.blankAt(v) {
			n++
		}
	}
	return n
}

// fullest keeps the fullest picture an animation shows, for when it ends nearly empty.
type fullest struct {
	best *Doc
	n    int
}

func (f *fullest) look(d *Doc) {
	if n := d.inkCells(); n > f.n {
		f.best, f.n = d.clone(), n
	}
}

// pick gives the fullest picture if the last one has less than half its ink.
func (f *fullest) pick(d *Doc) *Doc {
	if f.best != nil && d.inkCells()*2 < f.n {
		f.best.fullest = true
		return f.best
	}
	return d
}

// lastCol is the last non-blank column of a row, -1 if the row is empty.
func (d *Doc) lastCol(y int) int {
	for x := d.cols - 1; x >= 0; x-- {
		if !d.blankAt(d.get(x, y)) {
			return x
		}
	}
	return -1
}

// usedRows counts the rows up to the last non-blank one.
func (d *Doc) usedRows() int {
	for y := d.rows - 1; y >= 0; y-- {
		if d.lastCol(y) >= 0 {
			return y + 1
		}
	}
	return 0
}

// fit makes the page as tall as its art or, if more, the rows a file's cursor
// went down to: empty lines at the end of a file are part of the page.
func (d *Doc) fit(cursorRows int) {
	n := max(d.usedRows(), cursorRows, 1)
	if n < d.rows {
		d.rows = n
	} else {
		d.ensureRows(n)
	}
}

// cursorRows: the rows a terminal's cursor has gone through.
func cursorRows(x, y int) int {
	if x > 0 {
		return y + 1
	}
	return y
}

// trim drops blank rows at the bottom.
func (d *Doc) trim(minRows int) {
	n := max(d.usedRows(), minRows)
	if n < d.rows {
		d.rows = n
	}
	d.ensureRows(n)
}

func (d *Doc) withCols(cols int) *Doc {
	n := newDoc(cols, d.rows)
	n.bg, n.border, n.ink, n.glyph0 = d.bg, d.border, d.ink, d.glyph0
	w := min(cols, d.cols)
	for y := 0; y < d.rows; y++ {
		copy(n.cells[y*cols:y*cols+w], d.cells[y*d.cols:y*d.cols+w])
	}
	return n
}

type Block struct {
	w, h  int
	cells []uint32
}

func (d *Doc) copyRect(x0, y0, w, h int) *Block {
	b := &Block{w: w, h: h, cells: make([]uint32, w*h)}
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			b.cells[y*w+x] = d.get(x0+x, y0+y)
		}
	}
	return b
}

// ---------------------------------------------------------------- SAUCE

type Sauce struct {
	title, author, group, date string
	fileSize                   uint32
	dataType, fileType         int
	tinfo1, tinfo2             int
	comments, flags            int
	font                       string
	notes                      []string
}

func latin1(b []byte) string {
	r := make([]rune, len(b))
	for i, c := range b {
		r[i] = rune(c)
	}
	return strings.TrimRight(string(r), "\x00 ")
}

// readSauce returns the SAUCE record (or nil) and where the art ends.
func readSauce(b []byte) (*Sauce, int) {
	end, n := len(b), len(b)
	var s *Sauce
	if n >= 128 && string(b[n-128:n-121]) == "SAUCE00" {
		o := n - 128
		u16 := func(i int) int { return int(b[o+i]) | int(b[o+i+1])<<8 }
		s = &Sauce{
			title: latin1(b[o+7 : o+42]), author: latin1(b[o+42 : o+62]), group: latin1(b[o+62 : o+82]),
			date: latin1(b[o+82 : o+90]), fileSize: uint32(u16(90)) | uint32(u16(92))<<16,
			dataType: int(b[o+94]), fileType: int(b[o+95]), tinfo1: u16(96), tinfo2: u16(98),
			comments: int(b[o+104]), flags: int(b[o+105]), font: latin1(b[o+106 : o+128]),
		}
		end = o
		if s.comments > 0 {
			c := o - 5 - s.comments*64
			if c >= 0 && string(b[c:c+5]) == "COMNT" {
				for i := 0; i < s.comments; i++ {
					s.notes = append(s.notes, latin1(b[c+5+i*64:c+69+i*64]))
				}
				end = c
			}
		}
	}
	if s != nil {
		// The record says where the art ends; the EOF byte before it is not art.
		// (Some art draws with 0x1A, a right arrow in CP437.)
		// Padding and EOF bytes (some programs write two) before the record.
		for end > 0 && (b[end-1] == 0 || b[end-1] == 0x1A) {
			end--
		}
	} else {
		// Without a record the art ends at the first EOF (Ctrl+Z) byte.
		for i := 0; i < end; i++ {
			if b[i] == 0x1A {
				end = i
				break
			}
		}
	}
	return s, end
}

type SauceInfo struct {
	title, author, group string
	cols, rows           int
	font                 string
	fileType, flags      int
	binaryText           bool // DataType 5 (a .bin): the width goes in FileType, in pairs of columns
}

// makeSauce gives the EOF byte and a 128-byte SAUCE record, to append after the art.
func makeSauce(info SauceInfo, artSize int) []byte {
	out := make([]byte, 129)
	out[0] = 0x1A
	b := out[1:]
	put := func(i int, s string, n int, pad byte) {
		rs := []rune(s)
		for k := 0; k < n; k++ {
			if k < len(rs) {
				b[i+k] = byte(rs[k])
			} else {
				b[i+k] = pad
			}
		}
	}
	put(0, "SAUCE00", 7, 32)
	put(7, info.title, 35, 32)
	put(42, info.author, 20, 32)
	put(62, info.group, 20, 32)
	put(82, time.Now().Format("20060102"), 8, 32)
	u16 := func(i, v int) { b[i] = byte(v); b[i+1] = byte(v >> 8) }
	u16(90, artSize&0xFFFF)
	u16(92, artSize>>16)
	if info.binaryText {
		b[94], b[95] = 5, byte(info.cols/2) // DataType: BinaryText
	} else {
		b[94] = 1 // DataType: Character
		b[95] = byte(info.fileType)
		u16(96, info.cols)
		u16(98, info.rows)
	}
	b[105] = byte(info.flags)
	put(106, info.font, 22, 0)
	return out
}

// ---------------------------------------------------------------- the parser

// Term is a terminal that writes into a Doc. The page is endless downwards:
// rows are counted from the top of the art, the way ANSI viewers do.
type Term struct {
	doc                                        *Doc
	amiga                                      bool
	x, y, sx, sy                               int
	state                                      int // 0 text, 1 ESC, 2 CSI, 3 ESC + one byte to skip
	params, inter                              []byte
	fontRequest                                int // a SyncTERM font number asked for by CSI 0;n SP D, -1 if none
	beforeClear                                func()
	fg, bg                                     int
	bold, blink, ital, under, reverse, conceal bool
}

func newTerm(d *Doc, amiga bool) *Term {
	t := &Term{doc: d, amiga: amiga, fontRequest: -1}
	t.resetAttrs()
	return t
}

func (t *Term) resetAttrs() {
	t.fg, t.bg = 7, 0
	t.bold, t.blink, t.ital, t.under, t.reverse, t.conceal = false, false, false, false, false, false
}

func (t *Term) attr() uint32 {
	f, b := t.fg, t.bg
	if t.reverse {
		f, b = t.bg, t.fg
	}
	if t.bold {
		f += 8
	}
	if t.blink {
		b += 8
	}
	if t.conceal {
		f = b & 7
	}
	v := uint32(f)<<8 | uint32(b)<<12
	if t.ital {
		v |= italic
	}
	if t.under {
		v |= underline
	}
	return v
}

func (t *Term) blankCell() uint32 {
	if t.reverse {
		return makeCell(32, 7, t.fg, 0)
	}
	return makeCell(32, 7, t.bg, 0)
}

func (t *Term) put(ch byte) {
	d := t.doc
	if t.x >= d.cols {
		t.x, t.y = 0, t.y+1
	}
	d.set(t.x, t.y, uint32(ch)|t.attr())
	// Wrap at once, like ANSI.SYS and the Amiga console.
	t.x++
	if t.x >= d.cols {
		t.x, t.y = 0, t.y+1
	}
}

func (t *Term) cursor() (int, int) { return t.x, t.y }

func (t *Term) write(b []byte) {
	for _, c := range b {
		t.byte(c)
	}
}

func (t *Term) byte(c byte) {
	switch t.state {
	case 1:
		t.esc(c)
		return
	case 2:
		t.csiByte(c)
		return
	case 3:
		t.state = 0
		return
	}
	if c == 0x1B {
		t.state = 1
		return
	}
	if t.amiga {
		switch c {
		case 0x9B: // the Amiga's 8-bit CSI
			t.startCsi()
			return
		case 0x84: // IND
			t.y++
			return
		case 0x85: // NEL
			t.x, t.y = 0, t.y+1
			return
		case 0x8D: // RI
			if t.y > 0 {
				t.y--
			}
			return
		case 0x0C: // form feed: clear screen and home
			t.clearAll(blank)
			return
		}
	}
	switch c {
	case 0x0A:
		t.x, t.y = 0, t.y+1
		return
	case 0x0D:
		t.x = 0
		return
	case 0x08:
		if t.x > 0 {
			t.x--
		}
		return
	case 0x09:
		t.x = min(t.doc.cols-1, (t.x&^7)+8)
		return
	case 0x07, 0x00:
		return
	}
	if t.amiga && (c < 0x20 || (c >= 0x7F && c < 0xA0)) {
		return // no glyphs there in Latin-1
	}
	t.put(c)
}

func (t *Term) esc(c byte) {
	t.state = 0
	switch c {
	case '[':
		t.startCsi()
	case 'c': // reset
		t.resetAttrs()
		t.clearAll(blank)
	case 'D': // index
		t.y++
	case 'E': // next line
		t.x, t.y = 0, t.y+1
	case 'M': // reverse index
		if t.y > 0 {
			t.y--
		}
	case '7':
		t.sx, t.sy = t.x, t.y
	case '8':
		t.x, t.y = t.sx, t.sy
	case '(', ')', '#', '%': // charset etc.: skip one byte
		t.state = 3
	}
}

func (t *Term) startCsi() {
	t.state = 2
	t.params = t.params[:0]
	t.inter = t.inter[:0]
}

func (t *Term) csiByte(c byte) {
	if c >= 0x30 && c <= 0x3F {
		if len(t.params) < 64 {
			t.params = append(t.params, c)
		}
		return
	}
	if c >= 0x20 && c <= 0x2F {
		t.inter = append(t.inter, c)
		return
	}
	t.state = 0
	if c >= 0x40 && c <= 0x7E {
		t.csi(c)
	} else if c == 0x1B {
		t.state = 1
	}
}

const noParam = -1

func (t *Term) csi(f byte) {
	raw := string(t.params)
	priv := len(raw) > 0 && strings.IndexByte("<=>?", raw[0]) >= 0
	if priv {
		raw = raw[1:]
	}
	var p []int
	for _, s := range strings.Split(raw, ";") {
		v, err := strconv.Atoi(s)
		if err != nil {
			v = noParam
		}
		p = append(p, v)
	}
	n := func(i, def int) int {
		if i >= len(p) || p[i] == noParam || (p[i] == 0 && def == 1) {
			return def
		}
		return p[i]
	}
	d := t.doc
	if string(t.inter) == " " {
		if f == 'D' && !priv {
			t.fontRequest = n(1, 0) // SyncTERM font select
		}
		return // Amiga SP p (cursor) and the rest
	}
	if len(t.inter) > 0 || priv {
		return // ?7h, >1m and friends: nothing to draw
	}
	switch f {
	case 'A':
		t.y = max(0, t.y-n(0, 1))
	case 'B':
		t.y += n(0, 1)
	case 'C':
		t.x = min(d.cols-1, t.x+n(0, 1))
	case 'D':
		t.x = max(0, min(d.cols-1, t.x)-n(0, 1))
	case 'E':
		t.x, t.y = 0, t.y+n(0, 1)
	case 'F':
		t.x, t.y = 0, max(0, t.y-n(0, 1))
	case 'G':
		t.x = min(d.cols-1, n(0, 1)-1)
	case 'd':
		t.y = n(0, 1) - 1
	case 'H', 'f':
		t.y = n(0, 1) - 1
		t.x = min(d.cols-1, n(1, 1)-1)
	case 'J':
		switch m := n(0, 0); m {
		case 2, 3:
			t.clearAll(t.blankCell())
		case 0:
			t.fill(t.y*d.cols+t.x, d.cols*max(d.rows, t.y+1))
		default:
			t.fill(0, t.y*d.cols+t.x+1)
		}
	case 'K':
		m := n(0, 0)
		a, b := 0, d.cols
		if m == 0 {
			a = t.x
		}
		if m == 1 {
			b = t.x + 1
		}
		for x := a; x < b; x++ {
			d.set(x, t.y, t.blankCell())
		}
	case 'X':
		for k := 0; k < n(0, 1); k++ {
			d.set(t.x+k, t.y, t.blankCell())
		}
	case '@':
		t.shiftRow(n(0, 1))
	case 'P':
		t.shiftRow(-n(0, 1))
	case 'L':
		t.shiftRows(n(0, 1))
	case 'M':
		t.shiftRows(-n(0, 1))
	case 's':
		t.sx, t.sy = t.x, t.y
	case 'u':
		if raw == "" { // Amiga: CSI n u = line length
			t.x, t.y = t.sx, t.sy
		}
	case 'm':
		t.sgr(p)
		// t, x, y (Amiga page length / offsets), h, l, n, r, S, T: nothing to draw.
	}
}

func (t *Term) sgr(p []int) {
	for i := 0; i < len(p); i++ {
		v := p[i]
		if v == noParam {
			v = 0
		}
		switch {
		case v == 0:
			t.resetAttrs()
		case v == 1:
			t.bold = true
		case v == 3:
			t.ital = true
		case v == 4:
			t.under = true
		case v == 5 || v == 6:
			t.blink = true
		case v == 7:
			t.reverse = true
		case v == 8:
			t.conceal = true
		case v == 22 || v == 21:
			t.bold = false
		case v == 23:
			t.ital = false
		case v == 24:
			t.under = false
		case v == 25:
			t.blink = false
		case v == 27:
			t.reverse = false
		case v == 28:
			t.conceal = false
		case v >= 30 && v <= 37:
			t.fg = v - 30
		case v == 39:
			t.fg = 7
		case v >= 40 && v <= 47:
			t.bg = v - 40
		case v == 49:
			t.bg = 0
		case v >= 90 && v <= 97:
			t.fg, t.bold = v-90, true
		case v >= 100 && v <= 107:
			t.bg, t.blink = v-100, true
		case v == 38 || v == 48:
			if i+1 < len(p) && p[i+1] == 5 {
				i += 2
			} else if i+1 < len(p) && p[i+1] == 2 {
				i += 4
			}
		}
	}
}

// fill blanks the linear cell range [start, to).
func (t *Term) fill(start, to int) {
	d := t.doc
	for k := start; k < to && k < d.rows*d.cols; k++ {
		d.cells[k] = t.blankCell()
	}
}

func (t *Term) clearAll(b uint32) {
	if t.beforeClear != nil {
		t.beforeClear()
	}
	fillCells(t.doc.cells[:t.doc.rows*t.doc.cols], b)
	t.x, t.y = 0, 0
}

func (t *Term) shiftRow(n int) {
	d := t.doc
	row := make([]uint32, d.cols)
	for x := range row {
		row[x] = d.get(x, t.y)
	}
	for x := t.x; x < d.cols; x++ {
		src := x - n
		if src >= t.x && src < d.cols {
			d.set(x, t.y, row[src])
		} else {
			d.set(x, t.y, t.blankCell())
		}
	}
}

func (t *Term) shiftRows(n int) {
	d, c, top := t.doc, t.doc.cols, t.y
	d.ensureRows(top + 1)
	rows := d.rows
	if n > 0 {
		d.ensureRows(rows + n)
		copy(d.cells[(top+n)*c:], d.cells[top*c:rows*c])
		fillCells(d.cells[top*c:(top+n)*c], blank)
	} else {
		k := min(-n, rows-top)
		copy(d.cells[top*c:], d.cells[(top+k)*c:rows*c])
		fillCells(d.cells[(rows-k)*c:rows*c], blank)
	}
}

// ---------------------------------------------------------------- loading

// amigaWeight says how much each byte 0x80-0xFF points to Amiga (above 0) or
// PC (below 0): learned from 1374 files whose machine is known (SAUCE, and
// the Amiga-only asciiarena.se and BBS styles) with tools/bigcheck/learn-amiga-pc.py.
// On files it had not seen it gets 98% of those without SAUCE right (the old
// hand-made rules: 90%). Ø · ¯ « » are Amiga; █ ▀ ▄ ▒ are PC.
var amigaWeight = [128]float64{
	-0.157, 0.153, 0.141, 0.150, 0.787, 1.482, 0.183, 0.156, // 0x80
	0.369, 0.552, 0.138, 0.151, 0.126, 0.149, 0.182, 0.210, // 0x88
	0.167, 0.552, 0.182, 0.519, 0.238, 0.154, 0.316, 0.425, // 0x90
	0.182, 0.167, 0.144, 0.145, 0.165, 0.307, 0.343, 0.167, // 0x98
	2.241, 3.146, 0.866, 0.284, 0.634, -0.061, 2.759, 0.290, // 0xA0
	1.127, 1.787, 0.222, 3.628, 2.879, 0.984, 1.274, 4.730, // 0xA8
	-1.349, -3.153, -2.180, -0.359, 1.015, 0.829, 0.717, 5.455, // 0xB0
	0.946, 0.930, -0.125, 3.649, 2.077, 2.077, 2.303, -0.156, // 0xB8
	-1.034, 0.126, 0.543, 0.452, -0.885, 0.168, 0.680, 0.121, // 0xC0
	-0.053, 0.380, 0.318, 0.201, 0.190, -0.491, 0.003, 0.344, // 0xC8
	0.286, 2.008, 0.140, 0.096, 0.297, 0.191, 0.410, 0.817, // 0xD0
	2.843, -0.945, -0.664, -4.354, -3.942, -2.635, -2.475, -3.998, // 0xD8
	0.839, 0.157, 0.279, 0.158, 0.547, 0.715, 0.785, 0.309, // 0xE0
	0.136, 0.171, 0.259, 0.309, 0.168, 0.210, 0.475, 0.586, // 0xE8
	0.061, 0.132, 0.821, 0.326, 0.182, 0.049, 0.398, 2.529, // 0xF0
	2.507, 0.505, -0.173, 0.194, 0.606, -0.284, -1.376, -0.018, // 0xF8
}

var amigaCSI = regexp.MustCompile(`\x9b[0-9;]*[mHJK]`)

// guessAmiga tells Amiga from PC for a file without SAUCE: the high bytes by their
// learned weights, line ends a little (art often passes through a PC on its way),
// and the Amiga's 8-bit CSI for sure.
func guessAmiga(b []byte) bool {
	var h [256]int
	for _, c := range b {
		h[c]++
	}
	hi := 0
	for _, n := range h[128:] {
		hi += n
	}
	s := 0.0
	if hi > 0 {
		for i, w := range amigaWeight {
			s += float64(h[128+i]) / float64(hi) * w
		}
		s *= float64(min(hi, 200)) / 20
	}
	lf, crlf := h['\n'], bytes.Count(b, []byte("\r\n"))
	if lf > 4 {
		if crlf*4 < lf {
			s += 0.5
		} else {
			s -= 0.3
		}
	}
	if amigaCSI.Match(b) {
		s += 10
	}
	// A byte 0x80-0x9F between letters is a CP437 letter (ä 0x84, å 0x86, ö 0x94):
	// on an Amiga it is a control code, never inside a word.
	letter := func(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }
	for i := 1; i+1 < len(b); i++ {
		if c := b[i]; c >= 0x80 && c < 0xA0 && c != 0x9B && letter(b[i-1]) && (letter(b[i+1]) || b[i+1] == ' ') {
			s -= 2
		}
	}
	if h[0x0C] > 0 && !bytes.Contains(b, []byte("\x1b[")) { // form feed: the Amiga's clear screen
		s++
	}
	return s >= 0
}

var cp437Rev = func() map[rune]byte {
	m := map[rune]byte{}
	for b := 255; b >= 0; b-- {
		m[cp437[b]] = byte(b)
	}
	return m
}()

// fromUtf8 decodes a UTF-8 file of art (as found on the web today) to bytes
// of the page's character set. It returns nil if the bytes are not UTF-8
// with at least one character outside ASCII.
func fromUtf8(b []byte, amiga bool) []byte {
	if !utf8.Valid(b) {
		return nil
	}
	s := string(b)
	wide := false
	for _, r := range s {
		if r >= 0x80 {
			wide = true
			break
		}
	}
	if !wide {
		return nil
	}
	out := make([]byte, 0, len(b))
	for _, r := range s {
		switch {
		case r < 0x80:
			out = append(out, byte(r))
		case amiga && r <= 0xFF:
			out = append(out, byte(r))
		case amiga:
			out = append(out, '?')
		default:
			if c, ok := cp437Rev[r]; ok {
				out = append(out, c)
			} else if r <= 0xFF {
				out = append(out, byte(r))
			} else {
				out = append(out, '?')
			}
		}
	}
	return out
}

// utf8LooksPc: box lines and blocks mean PC.
func utf8LooksPc(b []byte) bool {
	if !utf8.Valid(b) {
		return false
	}
	pc, am := 0, 0
	for _, r := range string(b) {
		if (r >= 0x2500 && r <= 0x259F) || r == 0x25A0 {
			pc++
		} else if r >= 0xA0 && r <= 0xFF {
			am++
		}
	}
	return pc > am
}

type ParseOpts struct {
	amiga     bool
	forceCols int
	binary    bool
}

// parseArt reads art into a Doc. b = the whole file (with or without SAUCE).
// It returns the SyncTERM font asked for, or -1.
func parseArt(b []byte, o ParseOpts) (*Doc, *Sauce, int) {
	sauce, end := readSauce(b)
	cols, binary := 80, o.binary
	if sauce != nil && sauce.dataType == 1 && sauce.tinfo1 >= 8 && sauce.tinfo1 <= 1000 {
		cols = sauce.tinfo1
	}
	if sauce != nil && sauce.dataType == 5 && sauce.fileType > 0 {
		binary, cols = true, sauce.fileType*2
	} else if binary && (sauce == nil || sauce.dataType != 1) {
		cols = binWidth(b[:end]) // a .bin without its width
	}
	if o.forceCols > 0 {
		cols = o.forceCols
	}
	art := b[:end]
	if binary {
		n := len(b)
		if sauce != nil {
			n = end
			if sauce.fileSize > 0 && int(sauce.fileSize) <= len(b) {
				n = int(sauce.fileSize)
			}
		}
		art = b[:n]
	}
	d := newDoc(cols, 25)
	fontRequest := -1
	if binary { // PC .BIN: a byte and an attribute byte per cell
		cells := len(art) / 2
		d.ensureRows((cells + cols - 1) / cols)
		for i := 0; i < cells; i++ {
			a := int(art[i*2+1])
			d.cells[i] = attrCell(art[i*2], byte(a))
		}
	} else {
		t := newTerm(d, o.amiga)
		var f fullest
		t.beforeClear = func() { f.look(t.doc) }
		writeWatching(t, art, &f)
		fontRequest = t.fontRequest
		if d = f.pick(t.doc); d == t.doc {
			d.fit(cursorRows(t.x, t.y))
		} else {
			d.fit(0)
		}
	}
	return d, sauce, fontRequest
}

// ---------------------------------------------------------------- saving

type attrs struct {
	fg, bg              int
	bold, blink, it, ul bool
}

// writeAnsi gives the art as ANSI bytes: SGR codes only where the colours
// change, blank cells at the end of a row dropped. A row that fills the width
// gets no line end (the terminal wraps there by itself).
//
// With keep1A, a 0x1A character is written as itself: a SAUCE record will mark
// where the art ends, so it cannot be taken for the end of the file.
func writeAnsi(d *Doc, amiga, colours, clear bool, keep1A ...bool) []byte {
	raw1A := len(keep1A) > 0 && keep1A[0]
	var out []byte
	nl := "\r\n"
	if amiga {
		nl = "\n"
	}
	cur := attrs{fg: 7}
	if colours {
		out = append(out, "\x1b[0m"...)
	}
	if clear {
		if amiga {
			out = append(out, '\x0c')
		} else {
			out = append(out, "\x1b[2J\x1b[1;1H"...)
		}
	}
	rows := d.rows // the whole page, empty rows at the end too
	for y := 0; y < rows; y++ {
		last := d.lastCol(y)
		for x := 0; x <= last; x++ {
			v := d.get(x, y)
			if colours {
				fg, bg := cellFg(v), cellBg(v)
				want := attrs{fg: fg & 7, bold: fg > 7, bg: bg & 7, blink: bg > 7, it: v&italic != 0, ul: v&underline != 0}
				var p []string
				if (cur.bold && !want.bold) || (cur.blink && !want.blink) || (cur.it && !want.it) || (cur.ul && !want.ul) {
					p = append(p, "0")
					cur = attrs{fg: 7}
				}
				if want.bold && !cur.bold {
					p = append(p, "1")
				}
				if want.it && !cur.it {
					p = append(p, "3")
				}
				if want.ul && !cur.ul {
					p = append(p, "4")
				}
				if want.blink && !cur.blink {
					p = append(p, "5")
				}
				if want.fg != cur.fg {
					p = append(p, strconv.Itoa(30+want.fg))
				}
				if want.bg != cur.bg {
					p = append(p, strconv.Itoa(40+want.bg))
				}
				if len(p) > 0 {
					out = append(out, "\x1b["+strings.Join(p, ";")+"m"...)
					cur = want
				}
			}
			ch := byte(cellCh(v))
			// Bytes a terminal would take as controls cannot be stored as glyphs.
			switch ch {
			case 0, 0x1B, 0x0A, 0x0D, 0x08, 0x09, 0x07:
				ch = 32
			case 0x1A:
				if !raw1A {
					ch = 32
				}
			}
			if amiga && (ch < 0x20 || (ch >= 0x7F && ch < 0xA0)) {
				ch = 32
			}
			out = append(out, ch)
		}
		if last < d.cols-1 {
			out = append(out, nl...)
		}
	}
	if colours {
		out = append(out, "\x1b[0m"...)
	}
	return out
}

// writeBin gives a PC .BIN: two bytes per cell, every row in full.
func writeBin(d *Doc) []byte {
	rows := max(d.rows, 1)
	out := make([]byte, rows*d.cols*2)
	for i := 0; i < rows*d.cols; i++ {
		v := d.get(i%d.cols, i/d.cols)
		out[i*2] = byte(cellCh(v))
		out[i*2+1] = cellAttr(v)
	}
	return out
}

// byteRune is the character a byte shows on the page.
func byteRune(c int, amiga bool) rune {
	if amiga {
		if c < 32 || (c >= 0x7F && c < 0xA0) {
			return ' '
		}
		return rune(c)
	}
	if c == 0 {
		return ' '
	}
	return cp437[c]
}

// docText gives the characters only, as Unicode lines with trailing blanks dropped.
// w == 0 means the whole page.
func docText(d *Doc, runeOf func(int) rune, x0, y0, w, h int) []string {
	if w == 0 {
		w, h = d.cols, d.rows
	}
	var lines []string
	for y := y0; y < y0+h; y++ {
		var sb strings.Builder
		for x := x0; x < x0+w; x++ {
			sb.WriteRune(runeOf(cellCh(d.get(x, y))))
		}
		lines = append(lines, strings.TrimRight(sb.String(), " "))
	}
	return lines
}

func describeSauce(s *Sauce) string {
	if s == nil {
		return "The file had no SAUCE record."
	}
	or := func(a, b string) string {
		if a == "" {
			return b
		}
		return a
	}
	t := fmt.Sprintf("The file's SAUCE: %s by %s / %s, %s, %dx%d, font \"%s\"", or(s.title, "(no title)"),
		or(s.author, "?"), or(s.group, "?"), s.date, s.tinfo1, s.tinfo2, or(s.font, "-"))
	if s.flags&1 != 0 {
		t += ", iCE colours"
	}
	t += "."
	if len(s.notes) > 0 {
		t += " Notes: " + strings.Join(s.notes, " / ")
	}
	return t
}

// writeWatching feeds an emulator the bytes, looking at the picture now and
// then so that an animation's fullest picture is kept.
func writeWatching(e interface{ write([]byte) }, b []byte, f *fullest) {
	step := max(256, len(b)/64) // about 64 looks, however long the file
	for i := 0; i < len(b); i += step {
		e.write(b[i:min(len(b), i+step)])
		if d := docOf(e); d != nil && len(b) > 2*step {
			f.look(d)
		}
	}
}

func docOf(e interface{ write([]byte) }) *Doc {
	switch t := e.(type) {
	case *Term:
		return t.doc
	case *PetTerm:
		return t.doc
	case *AtariTerm:
		return t.doc
	}
	return nil
}

// PC attribute bytes number the colours as the BIOS does (1 blue, 4 red); cells
// and ANSI number them the other way (1 red, 4 blue). The swap is its own inverse.
var biosANSI = [16]int{0, 4, 2, 6, 1, 5, 3, 7, 8, 12, 10, 14, 9, 13, 11, 15}

// attrCell makes a cell from a character and a PC attribute byte.
func attrCell(ch, attr byte) uint32 {
	return makeCell(int(ch), biosANSI[attr&15], biosANSI[attr>>4], 0)
}

// cellAttr is a cell's colours as a PC attribute byte.
func cellAttr(v uint32) byte {
	return byte(biosANSI[cellFg(v)] | biosANSI[cellBg(v)]<<4)
}

// biosPalette reorders a palette stored in BIOS order to the cells' order (and back: the same swap).
func biosPalette(p [16]uint32) [16]uint32 {
	var out [16]uint32
	for i := range out {
		out[i] = p[biosANSI[i]]
	}
	return out
}

// binWidth guesses a .bin's width: 160 (what the scene's viewers assume) or 80.
// A width that does not divide the file into whole rows is out; if both do,
// the one where the same character stacks over itself most wins.
func binWidth(art []byte) int {
	cells := len(art) / 2
	fits := func(w int) bool { return cells%w == 0 }
	switch {
	case fits(160) && !fits(80):
		return 160
	case fits(80) && !fits(160):
		return 80
	}
	score := func(w int) int {
		n := 0
		for i := 0; i+w < cells; i++ {
			if c := art[i*2]; c != ' ' && c != 0 && c == art[(i+w)*2] {
				n++
			}
		}
		return n
	}
	if score(80) > score(160) {
		return 80
	}
	return 160
}
