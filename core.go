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
	uni        bool // a Unicode page's cells: code and colour pair in the high bits too (uni.go)
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
	if x < 0 || y < 0 || x >= d.cols || y >= maxRowsFor(d.cols) {
		return // past the largest page: ignored
	}
	if y >= d.rows {
		d.ensureRows(y + 1)
	}
	d.cells[y*d.cols+x] = v
}

// ensureRows makes the page at least n rows tall (never more than maxRowsFor its
// width), the new rows blank.
func (d *Doc) ensureRows(n int) {
	n = min(n, maxRowsFor(d.cols))
	if n <= d.rows {
		return
	}
	need := n * d.cols
	if need > len(d.cells) {
		grown := make([]uint32, min(max(need, len(d.cells)*2), maxRowsFor(d.cols)*d.cols))
		copy(grown, d.cells[:d.rows*d.cols])
		d.cells = grown
	}
	// Rows once cut off (fit, setRows) may still hold old cells: blank them.
	fillCells(d.cells[d.rows*d.cols:need], blank)
	d.rows = n
}

func (d *Doc) clone() *Doc {
	c := &Doc{cols: d.cols, rows: d.rows, cells: make([]uint32, d.rows*d.cols), bg: d.bg, border: d.border, ink: d.ink, glyph0: d.glyph0, fullest: d.fullest, uni: d.uni}
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
	if d.uni {
		return uniBlank(v)
	}
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

// empty: nothing is drawn on the page. An empty Spectrum page is white paper
// (not blank to usedRows), so a page of one space cell everywhere is empty too.
func (d *Doc) empty() bool {
	if d.usedRows() == 0 {
		return true
	}
	cells := d.cells[:d.rows*d.cols]
	first := cells[0]
	if ch := first & 255; (ch != 32 && ch != 0) || (ch == 0 && d.glyph0) || first&underline != 0 {
		return false
	}
	for _, v := range cells {
		if v != first {
			return false
		}
	}
	return true
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
	n.bg, n.border, n.ink, n.glyph0, n.uni = d.bg, d.border, d.ink, d.glyph0, d.uni
	w := min(cols, d.cols)
	for y := 0; y < d.rows; y++ {
		copy(n.cells[y*cols:y*cols+w], d.cells[y*d.cols:y*d.cols+w])
		if n.uni && cols < d.cols {
			wideNormalRow(n, y, cols-1, cols-1) // a wide character cut by the new width (wide.go)
		}
	}
	return n
}

type Block struct {
	w, h   int
	cells  []uint32
	mosaic []bool // teletext: which cells show sixels (nil: not known, letters)
	uni    bool   // cut from a Unicode page (pasted elsewhere, its cells are converted)
}

func (d *Doc) copyRect(x0, y0, w, h int) *Block {
	b := &Block{w: w, h: h, cells: make([]uint32, w*h), uni: d.uni}
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
	utf8                       bool // the text fields are UTF-8 (Moebius), not CP437
}

// sauceText reads a SAUCE text field: CP437, padded with spaces or zeros.
func sauceText(b []byte) string {
	b = bytes.TrimRight(b, "\x00 ")
	if utf8.Valid(b) && utf8.RuneCount(b) < len(b) {
		return string(b) // Moebius writes the fields in UTF-8
	}
	r := make([]rune, len(b))
	for i, c := range b {
		if c < 0x80 {
			r[i] = rune(c)
		} else {
			r[i] = cp437[c]
		}
	}
	return string(r)
}

// sauceStart is where the SAUCE record (with its COMNT block, if any) begins, len(b) if there is none.
func sauceStart(b []byte) int {
	n := len(b)
	if n < 128 || string(b[n-128:n-121]) != "SAUCE00" {
		return n
	}
	o := n - 128
	if k := int(b[o+104]); k > 0 {
		if c := o - 5 - k*64; c >= 0 && string(b[c:c+5]) == "COMNT" {
			return c
		}
		if c := utf8Comments(b, o, k); c >= 0 {
			return c
		}
	}
	return o
}

// utf8Comments finds a COMNT block written in UTF-8 (Moebius pads each line to 64
// characters, so a line with letters outside ASCII takes more than 64 bytes);
// -1 if there is none.
func utf8Comments(b []byte, o, k int) int {
	from := max(0, o-5-k*64*4)
	to := o - k*64 // COMNT ends here or before
	if to-5 <= from {
		return -1
	}
	c := bytes.LastIndex(b[from:to], []byte("COMNT"))
	if c < 0 {
		return -1
	}
	c += from
	if blk := b[c+5 : o]; utf8.Valid(blk) && utf8.RuneCount(blk) == k*64 {
		return c
	}
	return -1
}

// dataEnd is where the data of a binary file (a .bin, iCEDraw, ArtWorx, TundraDraw)
// ends: before the SAUCE record and the one EOF byte in front of it. Zero bytes
// before that are data (a font, a palette, black cells), so they stay.
func dataEnd(b []byte) int {
	end := sauceStart(b)
	if end < len(b) && end > 0 && b[end-1] == 0x1A {
		end--
	}
	return end
}

// readSauce returns the SAUCE record (or nil) and where the text art ends.
func readSauce(b []byte) (*Sauce, int) {
	end, n := len(b), len(b)
	var s *Sauce
	if n >= 128 && string(b[n-128:n-121]) == "SAUCE00" {
		o := n - 128
		u16 := func(i int) int { return int(b[o+i]) | int(b[o+i+1])<<8 }
		s = &Sauce{
			title: sauceText(b[o+7 : o+42]), author: sauceText(b[o+42 : o+62]), group: sauceText(b[o+62 : o+82]),
			date: sauceText(b[o+82 : o+90]), fileSize: uint32(u16(90)) | uint32(u16(92))<<16,
			dataType: int(b[o+94]), fileType: int(b[o+95]), tinfo1: u16(96), tinfo2: u16(98),
			comments: int(b[o+104]), flags: int(b[o+105]), font: sauceText(b[o+106 : o+128]),
		}
		for _, f := range [][]byte{b[o+7 : o+82]} {
			if utf8.Valid(f) && utf8.RuneCount(f) < len(f) {
				s.utf8 = true
			}
		}
		end = sauceStart(b)
		if end < o {
			blk := b[end+5 : o]
			if len(blk) != s.comments*64 { // UTF-8: 64 characters a line
				s.utf8 = true
				r := []rune(string(blk))
				for i := 0; i < s.comments; i++ {
					s.notes = append(s.notes, strings.TrimRight(string(r[i*64:i*64+64]), " \x00"))
				}
			} else {
				for i := 0; i < s.comments; i++ {
					s.notes = append(s.notes, sauceText(blk[i*64:i*64+64]))
				}
			}
		}
	}
	if s != nil {
		// Text art ends before the record, the EOF byte in front of it and any zero
		// padding; some programs write two EOF bytes, so all are dropped. (Binary
		// files, where these bytes can be cells, use dataEnd instead.)
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
	binaryText           bool     // DataType 5 (a .bin): the width goes in FileType, in pairs of columns
	dataType             int      // another DataType than Character (1): 6 for XBin
	utf8                 bool     // write the text in UTF-8, as the file had it (Moebius), not CP437
	notes                []string // comment lines, kept from the file (a COMNT block of 64 characters each)
}

// sauceUTF8 writes s into b as UTF-8 (whole characters that fit), padded with pad.
func sauceUTF8(b []byte, s string, pad byte) {
	n := 0
	for _, r := range s {
		if l := utf8.RuneLen(r); l > 0 && n+l <= len(b) {
			n += utf8.EncodeRune(b[n:], r)
		} else {
			break
		}
	}
	for ; n < len(b); n++ {
		b[n] = pad
	}
}

// sauceBytes writes s into b as CP437, padded with pad.
func sauceBytes(b []byte, s string, pad byte) {
	rs := []rune(s)
	for k := range b {
		switch {
		case k >= len(rs):
			b[k] = pad
		case rs[k] < 0x80:
			b[k] = byte(rs[k])
		default:
			if c, ok := cp437Rev[rs[k]]; ok && c >= 0x80 {
				b[k] = c
			} else {
				b[k] = '?'
			}
		}
	}
}

// makeSauce gives the EOF byte, the comment lines (if any) and a 128-byte SAUCE
// record, to append after the art.
func makeSauce(info SauceInfo, artSize int) []byte {
	notes := info.notes[:min(len(info.notes), 255)]
	out := []byte{0x1A}
	if len(notes) > 0 {
		out = append(out, "COMNT"...)
		for _, n := range notes {
			if info.utf8 { // as Moebius: 64 characters, however many bytes
				r := []rune(n)
				r = r[:min(len(r), 64)]
				out = append(out, string(r)+strings.Repeat(" ", 64-len(r))...)
				continue
			}
			line := make([]byte, 64)
			sauceBytes(line, n, 32)
			out = append(out, line...)
		}
	}
	b := make([]byte, 128)
	put := func(i int, s string, n int, pad byte) { sauceBytes(b[i:i+n], s, pad) }
	if info.utf8 {
		put = func(i int, s string, n int, pad byte) { sauceUTF8(b[i:i+n], s, pad) }
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
		// The width in pairs of columns: writeBin pads an odd width with one blank column.
		// 0 (no width) if it does not fit in a byte; binSaveError refuses that first.
		if w := (info.cols + 1) / 2; w <= 255 {
			b[95] = byte(w)
		}
		b[94] = 5 // DataType: BinaryText
	} else {
		b[94] = 1 // DataType: Character
		if info.dataType > 0 {
			b[94] = byte(info.dataType)
		}
		b[95] = byte(info.fileType)
		u16(96, info.cols)
		u16(98, info.rows)
	}
	b[104] = byte(len(notes))
	b[105] = byte(info.flags)
	put(106, info.font, 22, 0)
	return append(out, b...)
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
	screenH                                    int // the rows insert / delete line move; 0 means 25
	beforeClear                                func()
	beforeHome                                 func() // the cursor is sent home (CSI H, CSI 1;1H)
	fg, bg                                     int
	bold, blink, ital, under, reverse, conceal bool
	glyphCtl                                   bool // 0x00, 0x07, 0x08 and 0x09 are glyphs, not controls
	lateWrap                                   bool // wrap when the next character comes (VT), not at once (ANSI.SYS)
	screens                                    int  // clear screens seen
	hiBg                                       bool // blink stands for a bright background colour (48;5;n, 100-107)
	uni                                        bool // a Unicode page: 38/48;5 and 38/48;2 colours are kept exactly (uni.go)
	fgX, bgX                                   int  // on a Unicode page, a colour beyond the 16 (-1: none)
	hiFg                                       bool // on a Unicode page, 90-97: a bright colour, not bold (30-37 end it)
}

func newTerm(d *Doc, amiga bool) *Term {
	t := &Term{doc: d, amiga: amiga, fontRequest: -1}
	t.resetAttrs()
	return t
}

func (t *Term) resetAttrs() {
	t.fg, t.bg = 7, 0
	t.bold, t.blink, t.ital, t.under, t.reverse, t.conceal, t.hiBg = false, false, false, false, false, false, false
	t.fgX, t.bgX, t.hiFg = -1, -1, false
}

func (t *Term) attr() uint32 {
	if t.uni && (t.fgX >= 0 || t.bgX >= 0 || t.ital || t.under) {
		return t.uniAttr()
	}
	f, b := t.fg, t.bg
	if t.reverse {
		f, b = t.bg, t.fg
	}
	if t.bold || t.hiFg {
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
	if t.uni && (t.bgX >= 0 || (t.reverse && t.fgX >= 0)) {
		_, bg := t.uniColours()
		return uniCell(32, pairOf(7, bg))
	}
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
	if t.x >= d.cols && !t.lateWrap {
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
	if t.glyphCtl && (c == 0x00 || c == 0x07 || c == 0x08 || c == 0x09) {
		t.put(c)
		return
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
		if len(t.inter) < 8 {
			t.inter = append(t.inter, c)
		}
		return
	}
	t.state = 0
	if c >= 0x40 && c <= 0x7E {
		t.csi(c)
	} else if c == 0x1B {
		t.state = 1
	}
}

const (
	noParam  = -1
	maxParam = 9999 // CSI numbers are cut to this: no screen is larger, and huge ones cannot overflow
)

// csiParam reads one CSI number: digits only (cut to maxParam), noParam otherwise.
func csiParam(s string) int {
	if s == "" {
		return noParam
	}
	v := 0
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return noParam
		}
		v = min(maxParam, v*10+int(s[i]-'0'))
	}
	return v
}

func (t *Term) csi(f byte) {
	raw := string(t.params)
	priv := len(raw) > 0 && strings.IndexByte("<=>?", raw[0]) >= 0
	if priv {
		raw = raw[1:]
	}
	var p []int
	for _, s := range strings.Split(raw, ";") {
		p = append(p, csiParam(s))
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
		if t.beforeHome != nil && n(0, 1) == 1 && n(1, 1) == 1 {
			t.beforeHome()
		}
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
		for x := t.x; x < min(d.cols, t.x+n(0, 1)); x++ {
			d.set(x, t.y, t.blankCell())
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
			t.fg, t.fgX, t.hiFg = v-30, -1, false
		case v == 39:
			t.fg, t.fgX, t.hiFg = 7, -1, false
		case v >= 40 && v <= 47:
			t.bg, t.bgX = v-40, -1
			if t.uni && t.hiBg { // after 100-107 a plain colour, as terminals have it
				t.blink, t.hiBg = false, false
			}
		case v == 49:
			t.bg, t.bgX = 0, -1
			if t.hiBg {
				t.blink, t.hiBg = false, false // the bright background was a colour, not blink
			}
		case v >= 90 && v <= 97 && t.uni:
			t.fg, t.hiFg, t.fgX = v-90, true, -1
		case v >= 90 && v <= 97:
			t.fg, t.bold, t.fgX = v-90, true, -1
		case v >= 100 && v <= 107:
			t.bg, t.blink, t.hiBg, t.bgX = v-100, true, true, -1
		case (v == 38 || v == 48) && t.uni:
			i = t.uniSgr(p, i)
		case v == 38 || v == 48:
			// 256 colours (Moebius's UTF-8 export writes its 16 as 38;5;n) and 24-bit
			// colours, as the nearest of the 16.
			c := -1
			if i+2 < len(p) && p[i+1] == 5 {
				c = nearest16(xterm256(p[i+2]))
				i += 2
			} else if i+4 < len(p) && p[i+1] == 2 {
				c = nearest16(uint32(min(255, max(0, p[i+2])))<<16 | uint32(min(255, max(0, p[i+3])))<<8 | uint32(min(255, max(0, p[i+4]))))
				i += 4
			} else {
				i = len(p)
			}
			if c >= 0 && v == 38 {
				t.fg, t.bold = c&7, c > 7
			} else if c >= 0 {
				t.bg, t.blink, t.hiBg = c&7, c > 7, c > 7
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
	t.screens++
	if t.beforeClear != nil {
		t.beforeClear()
	}
	t.doc.clearPage(b, t.screenH)
	t.x, t.y = 0, 0
}

// clearPage blanks the page with v and makes it one screen (h rows, 25 if 0) tall
// at most: the rows below are empty now, and a file that clears a long page again
// and again would otherwise blank all of it every time.
func (d *Doc) clearPage(v uint32, h int) {
	if h <= 0 {
		h = 25
	}
	fillCells(d.cells[:d.rows*d.cols], v)
	d.rows = min(d.rows, h)
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

// shiftRows inserts (n > 0) or deletes (n < 0) lines at the cursor's row, the way a
// terminal does on its screen: only the screenH rows from the cursor down move, and
// what is pushed past them is lost. (The page itself is endless, so moving all of it
// would make every insert as slow as the page is long.)
func (t *Term) shiftRows(n int) {
	d, c, top := t.doc, t.doc.cols, t.y
	h := t.screenH
	if h <= 0 {
		h = 25
	}
	n = max(-h, min(h, n))
	if n == 0 {
		return
	}
	d.ensureRows(top + 1)
	if top >= d.rows {
		return // past the largest page
	}
	end := min(d.rows, top+h) // the rows that move: [top, end)
	if n > 0 {
		d.ensureRows(min(top+h, d.rows+n)) // a short page grows rather than lose its last rows
		end = min(top+h, d.rows)
		k := min(n, end-top)
		copy(d.cells[(top+k)*c:end*c], d.cells[top*c:(end-k)*c])
		fillCells(d.cells[top*c:(top+k)*c], blank)
	} else {
		k := min(-n, end-top)
		copy(d.cells[top*c:(end-k)*c], d.cells[(top+k)*c:end*c])
		fillCells(d.cells[(end-k)*c:end*c], blank)
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
	col, esc := 0, 0 // the column in the line, for tabs; esc: in an escape sequence
	for _, r := range s {
		switch {
		case esc == 1:
			esc = 0
			if r == '[' {
				esc = 2
			}
		case esc == 2:
			if r >= 0x40 && r <= 0x7E {
				esc = 0
			}
		case r == 0x1B:
			esc = 1
		case r == '\n' || r == '\r':
			col = 0
		default:
			col++
		}
		switch {
		case r == '\t' && esc == 0:
			// The page reads the bytes below 32 that stand for ○ ◘ • as glyphs:
			// a real tab becomes the spaces it stands for.
			n := 8 - (col-1)%8
			out = append(out, bytes.Repeat([]byte{' '}, n)...)
			col += n - 1
		case (r == 0 || r == 7 || r == 8) && esc == 0:
			col-- // nothing to draw
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
		_, inPC := cp437Rev[r]
		switch {
		case (r >= 0x2500 && r <= 0x259F) || r == 0x25A0:
			pc++
		case r > 0xFF && inPC:
			pc++ // only CP437 has it (Latin-1 cannot hold it at all)
		case r >= 0xA0 && r <= 0xFF && !inPC:
			am++ // only Latin-1 has it (¯ ¸ ´ ¨ ...)
		case r >= 0xA0 && r <= 0xFF:
			// in both sets: says nothing
		}
	}
	return pc > am
}

type ParseOpts struct {
	amiga     bool
	forceCols int
	binary    bool
	utf8      bool   // the bytes came from a UTF-8 file (fromUtf8)
	codes     string // a BBS's colour codes in the text (bbscodes.go), "" none
}

// rawControls: the art (a file with a SAUCE record, read with glyphCtl) holds
// 0x00, 0x07, 0x08 or 0x09 as characters; saving it again keeps them so.
func rawControls(b []byte) bool {
	s, end := readSauce(b)
	return sauceArt(s) && bytes.ContainsAny(b[:end], "\x00\x07\x08\x09")
}

// sauceArt: the record says the file is character art (ASCII or ANSI) of its own size.
func sauceArt(s *Sauce) bool { return s != nil && s.dataType == 1 && s.fileType <= 1 }

// longestLine counts the characters of the longest line (escape codes left out).
func longestLine(b []byte) int {
	most, n, esc := 0, 0, 0
	for _, c := range b {
		switch {
		case esc == 1:
			esc = 0
			if c == '[' {
				esc = 2
			}
		case esc == 2:
			if c >= 0x40 && c <= 0x7E {
				esc = 0
			}
		case c == 0x1B:
			esc = 1
		case c == '\n':
			n = 0
		case c == '\r':
		default:
			n++
			most = max(most, n)
		}
	}
	return most
}

// parseArt reads art into a Doc. b = the whole file (with or without SAUCE).
// It returns the SyncTERM font asked for, or -1.
func parseArt(b []byte, o ParseOpts) (*Doc, *Sauce, int) {
	sauce, end := readSauce(b)
	cols, binary := 80, o.binary
	if sauce != nil && sauce.dataType == 5 && sauce.fileType > 0 {
		binary = true
	}
	art := b[:end]
	if binary {
		// A .bin's 0x00 and 0x1A bytes are cells: only the record and its EOF byte are cut,
		// and FileSize is used only where it points inside the data.
		n := dataEnd(b)
		if sauce != nil && sauce.fileSize > 0 && int64(sauce.fileSize) < int64(n) {
			n = int(sauce.fileSize)
		}
		art = b[:n]
	}
	if sauce != nil && sauce.dataType == 1 && sauce.tinfo1 >= 1 && sauce.tinfo1 <= 1000 {
		cols = sauce.tinfo1 // Moebius saves pages from 1 column wide
	}
	if sauce != nil && sauce.dataType == 5 && sauce.fileType > 0 {
		cols = sauce.fileType * 2
	} else if binary && (sauce == nil || sauce.dataType != 1) {
		cols = binWidth(art) // a .bin without its width
	}
	if o.forceCols > 0 {
		cols = min(o.forceCols, 1000)
	}
	fontRequest := -1
	if binary { // PC .BIN: a byte and an attribute byte per cell, as many rows as the cells fill
		cells := min(len(art)/2, cols*maxRowsFor(cols))
		d := newDoc(cols, max(1, (cells+cols-1)/cols))
		for i := 0; i < cells; i++ {
			d.cells[i] = attrCell(art[i*2], art[i*2+1])
		}
		return d, sauce, fontRequest
	}
	if o.utf8 && sauce == nil && o.forceCols == 0 && o.codes == "" { // codes would count as characters
		if n := longestLine(art); n > cols && n <= 256 {
			cols = n // a UTF-8 file has no record to say its width (longer lines are prose: they wrap)
		}
	}
	d := newDoc(cols, 25)
	t := newTerm(d, o.amiga)
	// An art editor's file (it has a SAUCE record) holds every character as a glyph:
	// Moebius and PabloDraw write ○ (9), ◘ (8), • (7) and character 0 as they are.
	t.glyphCtl = (sauceArt(sauce) || o.utf8) && !o.amiga // fromUtf8 left only glyphs there
	t.lateWrap = o.utf8                                  // UTF-8 art is for today's terminals, which wrap at the next character
	var f fullest
	t.beforeClear = func() { f.look(t.doc) }
	if o.codes != "" && !o.amiga {
		ct := newCodeTerm(t, o.codes)
		writeWatching(ct, art, &f)
		ct.flush()
	} else {
		writeWatching(t, art, &f)
	}
	fontRequest = t.fontRequest
	if d = f.pick(t.doc); d == t.doc {
		d.fit(cursorRows(t.x, t.y))
		if sauceArt(sauce) && sauce.flags&0x18 != 0 && sauce.tinfo2 > d.rows && sauce.tinfo2 <= maxRowsFor(d.cols) && t.screens <= 1 {
			// The record's height: empty rows at the end that the file has no line
			// ends for (Moebius writes nothing for them). Only from editors that
			// write the aspect ratio bits, as Moebius always does: older ones often
			// wrote 25 (the screen) or one row more for the last line end.
			d.ensureRows(sauce.tinfo2)
		}
	} else {
		d.fit(0)
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
//
// keep1A[2] writes the file for today's terminals, as Moebius's UTF-8 export
// does: every character in UTF-8 (☺ ○ ◙ ← too: in UTF-8 they cannot be taken for
// controls), every row in full and ended by LF, so the lines give the width
// without a SAUCE record (fromUtf8 reads it back).
func writeAnsi(d *Doc, amiga, colours, clear bool, keep1A ...bool) []byte {
	raw1A := len(keep1A) > 0 && keep1A[0]
	rawCtl := len(keep1A) > 1 && keep1A[1] && !amiga // keep1A[1]: 0x00, 0x07, 0x08, 0x09 as they are
	utf := len(keep1A) > 2 && keep1A[2]
	var out []byte
	nl := "\r\n"
	if amiga || utf {
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
		if utf {
			last = d.cols - 1
		}
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
			if utf {
				switch {
				case !amiga:
					out = utf8.AppendRune(out, cp437[ch])
				case ch < 0x20 || (ch >= 0x7F && ch < 0xA0):
					out = append(out, ' ')
				default:
					out = utf8.AppendRune(out, rune(ch)) // Latin-1
				}
				continue
			}
			// Bytes a terminal would take as controls cannot be stored as glyphs:
			// art editors (Moebius, PabloDraw) write ○ ◘ • and character 0 as they
			// are, but a BBS terminal tabs, backs up and beeps on them. Only a file
			// that had them so (rawCtl) keeps them so.
			switch ch {
			case 0, 0x08, 0x09, 0x07:
				if !rawCtl {
					ch = 32
				}
			case 0x1B, 0x0A, 0x0D:
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
		if last < d.cols-1 || utf {
			out = append(out, nl...)
		}
	}
	if colours {
		out = append(out, "\x1b[0m"...)
	}
	return out
}

// writeBin gives a PC .BIN: two bytes per cell, every row in full. A .bin's width
// is stored in pairs of columns, so an odd width gets one blank column more.
func writeBin(d *Doc) []byte {
	rows, w := max(d.rows, 1), d.cols+d.cols%2
	out := make([]byte, rows*w*2)
	for i := 0; i < rows*w; i++ {
		v := d.get(i%w, i/w)
		out[i*2] = byte(cellCh(v))
		out[i*2+1] = cellAttr(v)
	}
	return out
}

// binSaveError says why a page cannot be saved as a .bin (nil if it can): SAUCE
// stores a .bin's width in one byte of column pairs, so 510 columns at most.
func binSaveError(d *Doc) error {
	if d.cols > 510 {
		return fmt.Errorf("a .bin can be at most 510 columns wide (this page is %d): save as XBin instead", d.cols)
	}
	return nil
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
			v := d.get(x, y)
			if d.uni {
				if uniIsCont(v) && uniIsLead(d.get(x-1, y)) {
					continue // a wide character's right half
				}
				sb.WriteRune(uniRune(uniCode(v))) // the page's whole character, not its low byte
				continue
			}
			sb.WriteRune(runeOf(cellCh(v)))
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
	case *codeTerm:
		return t.doc
	case *PetTerm:
		return t.doc
	case *imageTerm:
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

// vga16 are the 16 colours of a PC's text mode, in ANSI order (1 red, 4 blue).
var vga16 = [16]uint32{0x000000, 0xAA0000, 0x00AA00, 0xAA5500, 0x0000AA, 0xAA00AA, 0x00AAAA, 0xAAAAAA,
	0x555555, 0xFF5555, 0x55FF55, 0xFFFF55, 0x5555FF, 0xFF55FF, 0x55FFFF, 0xFFFFFF}

// xterm256 is the RGB of colour n of the 256: the 16 first, a 6x6x6 cube, 24 greys.
func xterm256(n int) uint32 {
	switch {
	case n < 0 || n > 255:
		return 0
	case n < 16:
		return vga16[n]
	case n < 232:
		n -= 16
		l := func(v int) uint32 {
			if v == 0 {
				return 0
			}
			return uint32(55 + v*40)
		}
		return l(n/36)<<16 | l(n/6%6)<<8 | l(n%6)
	}
	g := uint32(8 + (n-232)*10)
	return g<<16 | g<<8 | g
}

// nearest16 is the one of the 16 colours closest to rgb.
func nearest16(rgb uint32) int {
	best, bd := 0, -1
	for i, c := range vga16 {
		d := 0
		for _, sh := range []uint{0, 8, 16} {
			x := int(rgb>>sh&255) - int(c>>sh&255)
			d += x * x
		}
		if bd < 0 || d < bd {
			best, bd = i, d
		}
	}
	return best
}
