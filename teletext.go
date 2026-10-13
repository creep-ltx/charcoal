package main

// Teletext pages (BBC Micro Mode 7, Ceefax and friends): 40x25 cells of 7-bit
// codes. Codes 0-31 are spacing attributes, kept in the row as the screen keeps
// them: colours, text or graphics (2x3 "sixels"), flash, double height, conceal,
// separated, hold, background. A row's look comes from reading it left to right,
// as the SAA5050 chip does; cells store only the codes.
//
// The characters are Bedstead's (CC0, fonts/bedstead-NOTICE.txt): 5x9 dots,
// doubled and rounded like the chip's 12x20 cells. Files: edit.tf links (.url or a
// text with the link), .tti (MRG / ETS teletext pages), .ep1 and Mode 7 dumps (.bin).

import (
	"bytes"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const ttW, ttH = 12, 20 // a teletext cell in pixels

// ttFont names the teletext characters; they are drawn by ttGlyph, not from its bits.
var ttFont = &Font{id: "teletext", name: "Teletext (Bedstead)", kind: "tt", syncterm: -1, bits: make([]byte, 4096)}

var teletextPalette = pal("teletext", "Teletext", 0x000000, 0xFF0000, 0x00FF00, 0xFFFF00, 0x0000FF, 0xFF00FF, 0x00FFFF, 0xFFFFFF)

// ttCell is how one cell shows after the row's attributes are applied.
type ttCell struct {
	code    int  // the character or sixels shown (0x20 space)
	mosaic  bool // sixels, not a letter
	sep     bool // separated sixels
	fg, bg  int
	flash   bool
	conceal bool
	dh      int // 0 normal, 1 top half of double height, 2 bottom half
	control int // the attribute code in this cell, -1 for a character
}

// ttDecode reads row y as the SAA5050 does. A row under a double-height row
// shows the bottom halves of the row above instead of its own.
func ttDecode(d *Doc, y int, blackFg bool) []ttCell {
	if y > 0 && ttHasDouble(d, y-1) && !ttHasDouble2(d, y-1) {
		above := ttDecode(d, y-1, blackFg)
		for x := range above {
			if above[x].dh == 1 {
				above[x].dh = 2
			} else { // cells of normal height show their background under a double row
				above[x].code, above[x].mosaic, above[x].dh = 0x20, false, 0
			}
			above[x].control = -1
		}
		return above
	}
	out := make([]ttCell, d.cols)
	fg, bg := 7, 0
	mosaic, sep, hold, flash, conceal, dh := false, false, false, false, false, false
	held, heldSep := 0x20, false
	for x := 0; x < d.cols; x++ {
		c := cellCh(d.get(x, y)) & 0x7F
		if c >= 0x20 {
			cell := ttCell{code: c, fg: fg, bg: bg, flash: flash, conceal: conceal, sep: sep, control: -1}
			if mosaic && (c < 0x40 || c >= 0x60) {
				cell.mosaic = true
				held, heldSep = c, sep
			}
			if dh {
				cell.dh = 1
			}
			out[x] = cell
			continue
		}
		// Set-at attributes take effect in this cell.
		switch c {
		case 0x09:
			flash = false
		case 0x0C:
			if dh {
				held = 0x20
			}
			dh = false
		case 0x18:
			conceal = true
		case 0x19:
			sep = false
		case 0x1A:
			sep = true
		case 0x1C:
			bg = 0
		case 0x1D:
			bg = fg
		case 0x1E:
			hold = true
		}
		cell := ttCell{code: 0x20, fg: fg, bg: bg, flash: flash, conceal: conceal, control: c}
		if hold && mosaic { // a held graphic shows in the attribute's cell
			cell.code, cell.mosaic, cell.sep = held, true, heldSep
		}
		if dh {
			cell.dh = 1
		}
		out[x] = cell
		// Set-after attributes take effect from the next cell.
		switch {
		case c >= 0x01 && c <= 0x07, c == 0x00 && blackFg:
			if mosaic {
				held = 0x20
			}
			fg, mosaic, conceal = c, false, false
		case c >= 0x11 && c <= 0x17, c == 0x10 && blackFg:
			if !mosaic {
				held = 0x20
			}
			fg, mosaic, conceal = c-0x10, true, false
		case c == 0x08:
			flash = true
		case c == 0x0D:
			if !dh {
				held = 0x20
			}
			dh = true
		case c == 0x1F:
			hold = false
		}
	}
	return out
}

func ttHasDouble(d *Doc, y int) bool {
	for x := 0; x < d.cols; x++ {
		if cellCh(d.get(x, y))&0x7F == 0x0D {
			return true
		}
	}
	return false
}

// ttHasDouble2: is row y itself the bottom half of the row above (so it does not start another)?
func ttHasDouble2(d *Doc, y int) bool {
	return y > 0 && ttHasDouble(d, y-1) && !ttHasDouble2(d, y-1)
}

// ttGlyph tells whether pixel (px, py) of a 12x20 cell is ink, for a letter.
// The 5x9 dots are doubled; where two dots touch only at a corner the chip fills
// half a dot on each side (character rounding).
func ttGlyph(code, px, py int) bool {
	if code < 0x20 || code > 0x7F || px >= 10 || py >= 18 {
		return false
	}
	g := &teletextGlyphs[code-0x20]
	dot := func(x, y int) bool {
		return x >= 0 && x < 5 && y >= 0 && y < 9 && g[y]&(0x10>>x) != 0
	}
	gx, gy := px/2, py/2
	if dot(gx, gy) {
		return true
	}
	nx, ny := gx-1, gy-1
	if px&1 == 1 {
		nx = gx + 1
	}
	if py&1 == 1 {
		ny = gy + 1
	}
	return dot(gx, ny) && dot(nx, gy) && !dot(nx, ny)
}

// ttSixel tells whether pixel (px, py) of a 12x20 cell is ink, for sixels.
func ttSixel(code, px, py int, sep bool) bool {
	bits := code&0x1F | (code&0x40)>>1
	col := 0
	if px >= 6 {
		col = 1
	}
	row, top, h := 0, 0, 6
	switch {
	case py >= 14:
		row, top, h = 2, 14, 6
	case py >= 6:
		row, top, h = 1, 6, 8
	}
	if bits&(1<<(row*2+col)) == 0 {
		return false
	}
	if sep { // a gap on the left of each sixel and under it
		left := col * 6
		if px-left < 2 || py-top >= h-2 {
			return false
		}
	}
	return true
}

// ttInk is the ink of pixel (px, py) of a decoded cell, double height included.
func ttInk(c ttCell, px, py int) bool {
	switch c.dh {
	case 1:
		py = py / 2
	case 2:
		py = ttH/2 + py/2
	}
	if c.mosaic {
		return ttSixel(c.code, px, py, c.sep)
	}
	return ttGlyph(c.code, px, py)
}

// ---------------------------------------------------------------- files

const editTF = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"

var editTFLink = regexp.MustCompile(`#([0-9a-fA-F]):([A-Za-z0-9_-]+)`)

// editTFMatch finds an edit.tf page in the text: 1167 characters (25 rows) or 1120 (24).
func editTFMatch(b []byte) [][]byte {
	for _, m := range editTFLink.FindAllSubmatch(b[:min(len(b), 1<<16)], -1) {
		if n := len(m[2]); n == 1167 || n == 1120 {
			return m
		}
	}
	return nil
}

func isEditTF(b []byte) bool { return editTFMatch(b) != nil }

// The digit before the colon in an edit.tf link (read in edit.tf's
// teletext-editor.js): bits 0-2 the character set (0 English, 1 German,
// 2 Swedish, 3 Italian, 4 Belgian, 5 ASCII, 6 Hebrew, 7 Cyrillic), bit 3 black
// foreground allowed (codes 0 and 16 are black, not ignored).
const editTFBlack = 8

// parseEditTF reads the page in an edit.tf link; the bool is "black foreground allowed".
func parseEditTF(b []byte) (*Doc, bool) {
	d, meta := parseEditTFMeta(b)
	return d, meta&editTFBlack != 0
}

// parseEditTFMeta reads the page in an edit.tf link: 7 bits per cell, row by
// row, in base64url. It gives the whole meta digit back (character set and black).
func parseEditTFMeta(b []byte) (*Doc, int) {
	m := editTFMatch(b)
	if m == nil {
		return nil, 0
	}
	meta, _ := strconv.ParseUint(string(m[1]), 16, 8)
	s := m[2]
	rows := 25
	if len(s) == 1120 {
		rows = 24
	}
	d := newTTDoc()
	for i := 0; i < rows*40; i++ {
		code := 0
		for b := 0; b < 7; b++ {
			bit := i*7 + b
			v := strings.IndexByte(editTF, s[bit/6])
			if v >= 0 && v&(1<<(5-bit%6)) != 0 {
				code |= 1 << (6 - b)
			}
		}
		d.cells[i] = makeCell(code, 0, 0, 0)
	}
	return d, int(meta)
}

// editTFHash gives the part after the # of an edit.tf link for the page
// (English character set).
func editTFHash(d *Doc, blackFg bool) string {
	meta := 0
	if blackFg {
		meta = editTFBlack
	}
	return editTFHashMeta(d, meta)
}

// editTFHashMeta gives the part after the # with the meta digit as read by parseEditTFMeta.
func editTFHashMeta(d *Doc, meta int) string {
	b64 := make([]byte, 1167)
	for r := 0; r < 25; r++ {
		for c := 0; c < 40; c++ {
			code := cellCh(d.get(c, r)) & 0x7F
			for b := 0; b < 7; b++ {
				bit := 7*(r*40+c) + b
				if code&(1<<(6-b)) != 0 {
					b64[bit/6] |= 1 << (5 - bit%6)
				}
			}
		}
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "%x:", meta&15)
	for _, v := range b64 {
		sb.WriteByte(editTF[v])
	}
	return sb.String()
}

func writeEditTFURL(d *Doc, blackFg bool) []byte {
	return []byte("[InternetShortcut]\r\nURL=https://edit.tf/#" + editTFHash(d, blackFg) + "\r\n")
}

// writeEditTFURLMeta keeps the whole meta digit (the character set too).
func writeEditTFURLMeta(d *Doc, meta int) []byte {
	return []byte("[InternetShortcut]\r\nURL=https://edit.tf/#" + editTFHashMeta(d, meta) + "\r\n")
}

func isTTI(b []byte) bool { return bytes.Contains(b[:min(len(b), 4096)], []byte("OL,")) }

// A .tti file is lines of "XX,..." commands (MRG Systems' format, read by
// wxTED, vbit2, QTeletextMaker, edit.tf). Lines before the first PN belong to
// the whole file (DE description, DS source, SP path, and CT/PS/RE that the
// subpages take as their defaults); each PN line (page mppss: magazine 1-8,
// page 00-FF hex, subpage 00-99) starts a subpage with its own SC subcode, PS
// status, CT cycle time, RE region, FL Fastext links, PF page function and its
// OL rows: 0-24 the page, 25 hidden, 26-28 enhancement packets, 29 the
// magazine's M/29. Only OL rows 0-24 are the page; every other line is kept
// byte for byte when a subpage is saved again.

// ttiSubpages cuts a .tti file into its subpages, each from its PN line up to
// the next one; the lines before the first PN go with the first. Joined
// together they are the file again, byte for byte.
func ttiSubpages(b []byte) [][]byte {
	var out [][]byte
	start, seenPN := 0, false
	for i := 0; i < len(b); {
		end := bytes.IndexByte(b[i:], '\n')
		if end < 0 {
			end = len(b)
		} else {
			end += i + 1
		}
		if bytes.HasPrefix(b[i:end], []byte("PN,")) {
			if seenPN {
				out = append(out, b[start:i])
				start = i
			}
			seenPN = true
		}
		i = end
	}
	return append(out, b[start:])
}

// ttiPageRow tells whether a .tti line is one of the page's rows (OL,0 to
// OL,24 as parseTTI reads them), and which.
func ttiPageRow(line []byte) (int, bool) {
	line = bytes.TrimRight(line, "\r\n")
	if !bytes.HasPrefix(line, []byte("OL,")) {
		return 0, false
	}
	rest := line[3:]
	comma := bytes.IndexByte(rest, ',')
	if comma < 0 {
		return 0, false
	}
	row, err := strconv.Atoi(string(rest[:comma]))
	if err != nil || row < 0 || row > 24 {
		return 0, false
	}
	return row, true
}

// ttiSameChars: do two teletext pages hold the same codes (all a .tti keeps)?
func ttiSameChars(x, y *Doc) bool {
	for r := 0; r < 25; r++ {
		for c := 0; c < 40; c++ {
			if cellCh(x.get(c, r))&0x7F != cellCh(y.get(c, r))&0x7F {
				return false
			}
		}
	}
	return true
}

// ttiReplaceSubpage gives the subpage seg with its page rows written from d
// and every other line (PN, SC, PS, CT, FL, OL 25-29, unknown ones) kept as it
// was, in order; the new rows go where the first old one was. A page that
// still holds the same codes keeps its bytes as they are.
func ttiReplaceSubpage(seg []byte, d *Doc) []byte {
	if ttiSameChars(parseTTI(seg), d) {
		return seg
	}
	nl := "\r\n"
	if bytes.Contains(seg, []byte("\n")) && !bytes.Contains(seg, []byte("\r\n")) {
		nl = "\n"
	}
	rows := ttiRows(d, nl)
	var out []byte
	done := false
	for i := 0; i < len(seg); {
		end := bytes.IndexByte(seg[i:], '\n')
		if end < 0 {
			end = len(seg)
		} else {
			end += i + 1
		}
		line := seg[i:end]
		i = end
		if _, ok := ttiPageRow(line); ok {
			if !done {
				out, done = append(out, rows...), true
			}
			continue
		}
		out = append(out, line...)
	}
	if !done {
		if len(out) > 0 && out[len(out)-1] != '\n' {
			out = append(out, nl...)
		}
		out = append(out, rows...)
	}
	return out
}

// writeTTISubpage gives the .tti file b back with its subpage i (from 0)
// written from d and everything else as it was.
func writeTTISubpage(b []byte, i int, d *Doc) []byte {
	segs := ttiSubpages(b)
	if i < 0 || i >= len(segs) {
		return writeTTI(d)
	}
	var out []byte
	for j, s := range segs {
		if j == i {
			s = ttiReplaceSubpage(s, d)
		}
		out = append(out, s...)
	}
	return out
}

// parseTTI reads the first subpage of a .tti page: OL,row,text lines; codes 0-31
// are written as 0x80+code, or as ESC and code+0x40.
func parseTTI(b []byte) *Doc {
	d := newTTDoc()
	pages := 0
	for _, line := range bytes.Split(b, []byte("\n")) {
		line = bytes.TrimRight(line, "\r")
		if bytes.HasPrefix(line, []byte("PN,")) {
			if pages++; pages > 1 {
				break // only the first page (or subpage) of the file
			}
		}
		if !bytes.HasPrefix(line, []byte("OL,")) {
			continue
		}
		rest := line[3:]
		comma := bytes.IndexByte(rest, ',')
		if comma < 0 {
			continue
		}
		row, err := strconv.Atoi(string(rest[:comma]))
		if err != nil || row < 0 || row > 24 {
			continue
		}
		x := 0
		text := rest[comma+1:]
		for j := 0; j < len(text) && x < 40; j++ {
			c := int(text[j])
			switch {
			case c == 0x1B && j+1 < len(text) && text[j+1] >= 0x40 && text[j+1] <= 0x5F:
				j++ // ESC and code+0x40; any other ESC is the code 0x1B itself
				c = int(text[j]) - 0x40
			case c >= 0x80:
				c -= 0x80
			}
			d.set(x, row, makeCell(c&0x7F, 0, 0, 0))
			x++
		}
	}
	return d
}

// writeTTI gives a .tti page, codes 0-31 written as ESC and code+0x40.
func writeTTI(d *Doc) []byte {
	var out []byte
	out = append(out, "DE,Made with CharEdit\r\nPN,10000\r\nSC,0000\r\nPS,8000\r\n"...)
	return append(out, ttiRows(d, "\r\n")...)
}

// ttiRows gives a page's OL lines, codes 0-31 written as ESC and code+0x40.
func ttiRows(d *Doc, nl string) []byte {
	var out []byte
	for y := 0; y < min(d.rows, 25); y++ {
		last := -1
		for x := d.cols - 1; x >= 0; x-- {
			if c := cellCh(d.get(x, y)) & 0x7F; c != 0x20 {
				last = x
				break
			}
		}
		if last < 0 {
			continue
		}
		out = append(out, fmt.Sprintf("OL,%d,", y)...)
		for x := 0; x <= last; x++ {
			c := byte(cellCh(d.get(x, y)) & 0x7F)
			if c < 0x20 {
				out = append(out, 0x1B, c+0x40)
			} else {
				out = append(out, c)
			}
		}
		out = append(out, nl...)
	}
	return out
}

func isEP1(b []byte) bool { return len(b) >= 1006 && b[0] == 0xFE && b[1] == 0x01 }

// parseEP1 reads an .ep1 page: a 6-byte header, then 25 rows of 40 codes.
func parseEP1(b []byte) *Doc {
	d := newTTDoc()
	for i := 0; i < 1000 && 6+i < len(b); i++ {
		d.cells[i] = makeCell(int(b[6+i]&0x7F), 0, 0, 0)
	}
	return d
}

func writeEP1(d *Doc) []byte {
	out := []byte{0xFE, 0x01, 0x09, 0x00, 0x00, 0x00}
	for i := 0; i < 1000; i++ {
		out = append(out, byte(cellCh(d.get(i%40, i/40))&0x7F))
	}
	return append(out, 0x00, 0x00)
}

// isMode7: a BBC Micro screen dump (codes may carry the top bit): the 1000
// bytes on screen, or all 1024 of screen memory (&7C00-&7FFF), the first 1000 shown.
func isMode7(b []byte, name string) bool {
	if len(b) != 1000 && len(b) != 1024 || !strings.HasSuffix(strings.ToLower(name), ".bin") && !strings.HasSuffix(strings.ToLower(name), ".m7") {
		return false
	}
	ctl := 0
	for _, c := range b[:1000] {
		if c&0x7F < 0x20 && c&0x7F != 0 {
			ctl++
		}
	}
	return ctl >= 10
}

// parseMode7 reads a dump of any length: the first 1000 bytes, short ones padded with spaces.
func parseMode7(b []byte) *Doc {
	d := newTTDoc()
	for i := 0; i < 1000 && i < len(b); i++ {
		d.cells[i] = makeCell(int(b[i]&0x7F), 0, 0, 0)
	}
	return d
}

func newTTDoc() *Doc {
	d := newDoc(40, 25)
	d.glyph0 = true // code 0 is an attribute, not an empty cell
	fillCells(d.cells, makeCell(0x20, 0, 0, 0))
	return d
}

// ttFromRune: the English set is ASCII but for £ # ← ½ → ↑ — ¼ ‖ ¾ ÷.
func ttFromRune(r rune) int {
	switch r {
	case '£':
		return 0x23
	case '#':
		return 0x5F
	case '←':
		return 0x5B
	case '½':
		return 0x5C
	case '→':
		return 0x5D
	case '↑':
		return 0x5E
	case '—':
		return 0x60
	case '¼':
		return 0x7B
	case '‖':
		return 0x7C
	case '¾':
		return 0x7D
	case '÷':
		return 0x7E
	case '[', ']', '\\', '^', '`', '{', '|', '}', '~':
		return -1
	}
	if r >= 0x20 && r < 0x7F {
		return int(r)
	}
	return -1
}

func ttRune(c int) rune {
	c &= 0x7F
	if r, ok := map[int]rune{0x23: '£', 0x5F: '#', 0x5B: '←', 0x5C: '½', 0x5D: '→', 0x5E: '↑', 0x60: '—',
		0x7B: '¼', 0x7C: '‖', 0x7D: '¾', 0x7E: '÷', 0x7F: '■'}[c]; ok {
		return r
	}
	if c >= 0x20 {
		return rune(c)
	}
	return ' '
}

// ttLabel is the short name the editor shows in an attribute's cell.
func ttLabel(c int) (string, int) {
	switch {
	case c >= 0x01 && c <= 0x07:
		return "T", c // text colour
	case c >= 0x11 && c <= 0x17:
		return "G", c - 0x10 // graphics colour
	}
	names := map[int]string{0x00: "t", 0x10: "g", 0x08: "F", 0x09: "S", 0x0C: "N", 0x0D: "D", 0x18: "?", 0x19: "|",
		0x1A: ":", 0x1C: "K", 0x1D: "B", 0x1E: "H", 0x1F: "R"}
	if n, ok := names[c]; ok {
		return n, -1
	}
	return ".", -1
}

var ttCodeNames = map[int]string{0x08: "flash", 0x09: "steady", 0x0C: "normal height", 0x0D: "double height",
	0x18: "conceal", 0x19: "contiguous graphics", 0x1A: "separated graphics", 0x1C: "black background",
	0x1D: "new background (the colour so far)", 0x1E: "hold graphics", 0x1F: "release graphics"}

func ttCodeName(c int) string {
	colours := []string{"black", "red", "green", "yellow", "blue", "magenta", "cyan", "white"}
	switch {
	case c < 0x08:
		return "text " + colours[c]
	case c >= 0x10 && c < 0x18:
		return "graphics " + colours[c-0x10]
	}
	if n, ok := ttCodeNames[c]; ok {
		return n
	}
	return "nothing"
}

var ttFkeys = [][]int{
	{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x0D, 0x0C, 0x08},
	{0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x19, 0x1A, 0x1E},
	{0x1D, 0x1C, 0x1F, 0x09, 0x18, 0x7F, 0x20, 0x35, 0x6A, 0x3F},
	{0x21, 0x22, 0x24, 0x28, 0x30, 0x60, 0x23, 0x2C, 0x34, 0x70},
	{0x25, 0x2A, 0x33, 0x3C, 0x2F, 0x7C, 0x3D, 0x6F, 0x78, 0x77},
}
