package main

// BBS colour codes: text whose colours are a board's own codes instead of (or
// beside) ANSI escapes. Boards turn them into ANSI for the caller, so sysops
// and board artists keep menus and screens in them:
//
//   pipe      |00-|15 foreground, |16-|23 background, |24-|31 bright background
//             or blink (Mystic, Renegade, WWIV 5, Synchronet's Renegade mode,
//             IcyDraw's "Renegade"); |CL clears, |PI is a "|" (Mystic)
//   ctrla     Synchronet's Ctrl-A + letter (K R G Y B M C W, H bright, I blink,
//             E bright background, 0-7 background, N normal, L clear ...)
//   wwiv      WWIV's heart codes (Ctrl-C + 0-9, each caller's own colours: the
//             defaults are read) and its pipe codes |nn and |#n
//   wildcat   Wildcat!'s @BF@ (background and foreground as hex digits)
//   celerity  Celerity's | + letter (k b g c r m y w, d B G C R M Y W, S swaps)
//
// Reading: the codes are taken out of the stream in front of the ANSI terminal
// (codeTerm), so files that mix them with ANSI escapes (common on Mystic) read
// as the board shows them. A code the board would act on but CharEdit cannot
// show (a pause, the caller's name, a delay) stays in the page as text, and the
// writers put it back as it was: it survives a round trip and still works on
// the board. Writing: the shortest codes where the colours change, rows ended
// with CR LF as PCBoard's and Avatar's writers do.
//
// The tables come from the boards' sources (Mystic 1.10 bbs_io.pas, Renegade
// COMMON.PAS, WWIV common/output.cpp, Synchronet con_out.cpp ctrl_a and
// putmsg.cpp, IcyDraw's icy_parser_core renegade.rs and ctrla.rs) and
// Synchronet's colour-code wiki page (Wildcat!, Celerity).

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"
)

// bbsCodeIDs are the format ids (also the Open as values) of the code kinds.
var bbsCodeIDs = []string{"pipe", "ctrla", "wwiv", "wildcat", "celerity"}

func isBBSCodes(id string) bool {
	for _, k := range bbsCodeIDs {
		if k == id {
			return true
		}
	}
	return false
}

// bbsCodeName names a kind for messages.
var bbsCodeName = map[string]string{
	"pipe": "pipe codes", "ctrla": "Synchronet Ctrl-A codes", "wwiv": "WWIV heart codes",
	"wildcat": "Wildcat! @ codes", "celerity": "Celerity codes",
}

// wwivColours: what WWIV's heart codes 0-9 (and |#0-|#9) show for a caller who
// kept the default colours (WWIV 5 source: wwivutil print, fsed, localui).
// Every caller can change them, so the writers never use hearts.
var wwivColours = [10]int{0x07, 0x0B, 0x0E, 0x05, 0x1F, 0x02, 0x0C, 0x09, 0x06, 0x03}

// ---------------------------------------------------------------- finding them

var bbsMsgExt = regexp.MustCompile(`(?i)\.msg$`)

// bbsCodesKind says which codes a file is written in ("" none). A file ending
// they are known by (.msg Synchronet/WWIV, .bbs Wildcat!) needs one code;
// anything else three. Text art draws with | and @ and numbers its boxes, so
// (checked on the big check's 8479 files, none taken by mistake) a kind counts
// only when its codes repeat (two different ones at least, and more codes
// than different ones + 1), when they
// are at least a quarter of the | or @ in the file and, for Ctrl-A and hearts,
// at least half of the ☺ / ♥ bytes. Files that are not text (other magic,
// NULs, many control bytes) and other machines' files are never taken.
// Celerity's | + letter is too common in plain text to be guessed: Open as only.
func bbsCodesKind(b []byte, name string) string {
	lname := strings.ToLower(name)
	if notArt(b) != "" || bbsOtherExt.MatchString(lname) {
		return ""
	}
	b = b[:dataEnd(b)]
	ctl, nul := 0, 0
	for _, c := range b {
		if c == 0 {
			nul++ // old WWIV files hold runs of NULs
		} else if c < 0x20 && !strings.ContainsRune("\r\n\t\x1b\x01\x03\x0c\x1a", rune(c)) {
			ctl++
		}
	}
	if ctl*50 > len(b) || nul*10 > len(b) { // a binary file, not text
		return ""
	}
	need := map[string]int{"pipe": 3, "ctrla": 3, "wwiv": 3, "wildcat": 3}
	if bbsMsgExt.MatchString(lname) {
		need["ctrla"], need["wwiv"] = 1, 1
	}
	if strings.HasSuffix(lname, ".bbs") {
		need["wildcat"] = 1
	}
	n := map[string]int{}
	kinds := map[string]map[string]bool{"pipe": {}, "ctrla": {}, "wwiv": {}, "wildcat": {}}
	seen := func(k string, code []byte) {
		n[k]++
		kinds[k][string(code)] = true
	}
	bars, ats, ctrlA, hearts := 0, 0, 0, 0
	for i := 0; i < len(b); i++ {
		switch b[i] {
		case '|':
			bars++
			if i+2 < len(b) && isDigit(b[i+1]) && isDigit(b[i+2]) && (i+3 >= len(b) || !isDigit(b[i+3])) {
				if v := int(b[i+1]-'0')*10 + int(b[i+2]-'0'); v <= 31 {
					seen("pipe", b[i+1:i+3])
				}
			}
			if i+2 < len(b) && b[i+1] == '#' && isDigit(b[i+2]) {
				seen("wwiv", b[i:i+3])
			}
			if i+2 < len(b) && b[i+1] == 'T' && isDigit(b[i+2]) && (i+3 >= len(b) || !isDigit(b[i+3])) {
				seen("pipe", b[i+1:i+3]) // Mystic's theme colours (brdcodes.go)
			}
		case 0x01:
			ctrlA++
			if i+1 < len(b) && strings.IndexByte("KRGYBMCWHINkrgybmcwhin01234567", b[i+1]) >= 0 {
				seen("ctrla", b[i+1:i+2])
			}
		case 0x03:
			hearts++
			// A heart + one digit; mIRC's Ctrl-C takes two digits and a comma.
			if i+1 < len(b) && isDigit(b[i+1]) && (i+2 >= len(b) || !isDigit(b[i+2]) && b[i+2] != ',') {
				seen("wwiv", b[i:i+2])
			}
		case '@':
			ats++
			if i+3 < len(b) && isUpperHex(b[i+1]) && isUpperHex(b[i+2]) && b[i+3] == '@' {
				seen("wildcat", b[i+1:i+3])
			}
		}
	}
	ok := map[string]bool{
		"pipe":    n["pipe"]*4 >= bars,
		"ctrla":   n["ctrla"]*2 >= ctrlA,
		"wwiv":    n["wwiv"]*2 >= hearts && (hearts > 0 || n["wwiv"]*4 >= bars),
		"wildcat": n["wildcat"]*4 >= ats,
	}
	best := ""
	for _, k := range []string{"ctrla", "wwiv", "wildcat", "pipe"} {
		repeats := n[k] > len(kinds[k])+1 && len(kinds[k]) >= 2 || need[k] == 1
		if ok[k] && repeats && n[k] >= need[k] && (best == "" || n[k] > n[best]) {
			best = k
		}
	}
	if tg := telegardCodes(b); tg > 0 && (best == "" || tg > n[best]) { // brdcodes.go
		best = "telegard"
	}
	return best
}

// bbsOtherExt: other machines' and editors' files, never read for codes.
var bbsOtherExt = regexp.MustCompile(`(?i)\.(seq|pet|ata|vt|tti|ep1|scr|prg|c|petmate|xb|xbin|bin|idf|adf|tnd|icy|xp|psci|json|url|m7|pcb|avt)$`)

func isDigit(c byte) bool    { return c >= '0' && c <= '9' }
func isUpperHex(c byte) bool { return isDigit(c) || c >= 'A' && c <= 'F' }
func upperHexVal(c byte) int { return strings.IndexByte("0123456789ABCDEF", c) }
func ctrlaUsesIce(b []byte) bool {
	return bytes.Contains(b, []byte{1, 'E'}) || bytes.Contains(b, []byte{1, 'e'})
}

// ---------------------------------------------------------------- reading

// codeTerm takes a kind's codes out of the stream before the ANSI terminal
// sees it; everything else goes to the terminal as it is.
type codeTerm struct {
	*Term
	kind  string
	pend  []byte   // the start of a code cut by the end of a write
	stack []uint32 // Ctrl-A + pushes the colours, - pops them
	done  bool     // Ctrl-A Z: the end of the file
}

func newCodeTerm(t *Term, kind string) *codeTerm { return &codeTerm{Term: t, kind: kind} }

func (c *codeTerm) write(b []byte) { c.feed(b, false) }

// flush reads a code cut by the end of the file as far as it goes.
func (c *codeTerm) flush() { c.feed(nil, true) }

func (c *codeTerm) feed(b []byte, final bool) {
	if len(c.pend) > 0 {
		b = append(append([]byte{}, c.pend...), b...)
		c.pend = nil
	}
	for i := 0; i < len(b) && !c.done; {
		if c.state != 0 { // inside an ANSI escape: the terminal's
			c.Term.byte(b[i])
			i++
			continue
		}
		n, more := c.code(b[i:], final)
		if more {
			c.pend = append(c.pend, b[i:]...)
			return
		}
		if n == 0 {
			c.Term.byte(b[i])
			n = 1
		}
		i += n
	}
}

// code reads the code at the start of b: the bytes it takes (0 = not a code,
// shown as text), or more = it may be a code but b ends before it can tell.
func (c *codeTerm) code(b []byte, final bool) (n int, more bool) {
	want := map[string]int{"pipe": 3, "ctrla": 2, "wwiv": 3, "wildcat": 5, "celerity": 2, "telegard": 3}[c.kind]
	lead := map[string]string{"pipe": "|^", "ctrla": "\x01", "wwiv": "|\x03", "wildcat": "@", "celerity": "|", "telegard": "`"}[c.kind]
	if strings.IndexByte(lead, b[0]) < 0 {
		return 0, false
	}
	if len(b) < want && !final {
		return 0, true
	}
	switch c.kind {
	case "pipe":
		return c.pipe(b), false
	case "ctrla":
		return c.ctrlA(b), false
	case "wwiv":
		return c.wwiv(b), false
	case "wildcat":
		return c.wildcat(b), false
	case "celerity":
		return c.celerity(b), false
	case "telegard":
		return c.telegard(b), false // brdcodes.go
	}
	return 0, false
}

// The colours as a PC attribute byte (BIOS order: 1 blue, 4 red; bit 7 blink
// or a bright background), and setting them from one.
func (c *codeTerm) bios() int {
	a := biosANSI[c.fg+map[bool]int{true: 8}[c.bold]] | biosANSI[c.bg]<<4
	if c.blink {
		a |= 0x80
	}
	return a
}

func (c *codeTerm) setFg(bios int) {
	v := biosANSI[bios&15]
	c.fg, c.bold = v&7, v > 7
}

func (c *codeTerm) setBg(bios int, hi bool) { c.bg, c.blink, c.hiBg = biosANSI[bios&7], hi, false }

func (c *codeTerm) setAttr(a int) {
	c.resetAttrs()
	c.setFg(a & 15)
	c.setBg(a>>4&7, a&0x80 != 0)
}

func (c *codeTerm) clearScreen() { c.Term.write([]byte("\x1b[2J\x1b[H")) }

// pipeActs: "|" + x + y is a code the pipe reader acts on (|T0-|T9: Mystic's
// theme colours, brdcodes.go).
func pipeActs(x, y byte) bool {
	if isDigit(x) && isDigit(y) {
		return int(x-'0')*10+int(y-'0') <= 31
	}
	return x == 'C' && y == 'L' || x == 'P' && y == 'I' || x == 'T' && isDigit(y)
}

// pipe: |00-|15 set the foreground (background and blink kept: Renegade, WWIV,
// Synchronet), |16-|23 the background with blink off, |24-|31 the background
// with bit 7 (WWIV, Synchronet and IcyDraw; Mystic 1.10 and Renegade show
// nothing for them). |CL clears the screen, |PI is a "|" (Mystic). Any other
// | + two characters (Mystic's |PO, |DE, |UN ... and Renegade's eaten |32-|99)
// stays as text.
func (c *codeTerm) pipe(b []byte) int {
	if b[0] == '^' { // Renegade's user colours (brdcodes.go)
		return c.renegadeColour(b)
	}
	if len(b) < 3 || !pipeActs(b[1], b[2]) {
		return 0
	}
	switch {
	case b[1] == 'T': // Mystic's theme colours (brdcodes.go)
		c.setAttr(mysticThemeColours[b[2]-'0'])
	case b[1] == 'C':
		c.clearScreen()
	case b[1] == 'P':
		c.Term.byte('|')
	default:
		v := int(b[1]-'0')*10 + int(b[2]-'0')
		switch {
		case v < 16:
			c.setFg(v)
		case v < 24:
			c.setBg(v-16, false)
		default:
			c.setBg(v-24, true)
		}
	}
	return 3
}

// ctrlaActs: Ctrl-A + x is a code the Ctrl-A reader acts on; any other stays
// as text (☺ and the character), so a pause (P , ; .), the time, a hot spot
// and so on are written back as they were.
func ctrlaActs(x byte) bool {
	if x >= 0x80 {
		return true
	}
	return strings.IndexByte("KRGYBMCWkrgybmcw01234567HhIiEeNnLl'Jj><[]AaZzQq+-_", x) >= 0
}

// ctrlA: Synchronet's ctrl_a() (con_out.cpp) and putmsg.cpp: letters set the
// foreground's colour (bright kept), digits the background (bright and blink
// kept), H bright, I blink, E bright background, N normal (light grey),
// + pushes the colours, - pops them (or, with none pushed, goes back to normal
// if bright, blink or a background is on), _ back to normal if blink or a
// background is on; L clears, ' homes, J clears to the end of the screen, >
// to the end of the line, < moves left, [ CR, ] LF, a byte over 127 moves
// right (byte - 127); A is a Ctrl-A, z a Ctrl-Z, Z ends the file, Q resets
// the More? count (nothing to see).
func (c *codeTerm) ctrlA(b []byte) int {
	if len(b) < 2 || !ctrlaActs(b[1]) {
		return 0
	}
	x := b[1]
	if x >= 0x80 {
		c.Term.write([]byte(fmt.Sprintf("\x1b[%dC", int(x)-0x7F)))
		return 2
	}
	if i := strings.IndexByte("KRGYBMCW", upper(x)); i >= 0 {
		c.fg = i // ANSI order, as the cells
		return 2
	}
	if x >= '0' && x <= '7' {
		c.bg, c.hiBg = int(x-'0'), false // ANSI order too: 1 red, 4 blue
		return 2
	}
	switch upper(x) {
	case 'H':
		c.bold = true
	case 'I', 'E':
		c.blink = true
	case 'N':
		c.resetAttrs()
	case '+':
		if len(c.stack) < 64 {
			c.stack = append(c.stack, uint32(c.bios()))
		}
	case '-':
		if n := len(c.stack); n > 0 {
			c.setAttr(int(c.stack[n-1]))
			c.stack = c.stack[:n-1]
		} else if a := c.bios(); a&0x88 != 0 || a&0x70 != 0 {
			c.resetAttrs()
		}
	case '_':
		if a := c.bios(); a&0x80 != 0 || a&0x70 != 0 {
			c.resetAttrs()
		}
	case 'L':
		c.clearScreen()
	case '\'':
		c.Term.write([]byte("\x1b[H"))
	case 'J':
		c.Term.write([]byte("\x1b[J"))
	case '>':
		c.Term.write([]byte("\x1b[K"))
	case '<':
		c.Term.write([]byte("\x1b[D"))
	case '[':
		c.Term.byte('\r')
	case ']':
		c.Term.byte('\n')
	case 'A':
		c.Term.byte(0x01)
	case 'Z':
		if x == 'Z' { // only the capital ends the file (putmsg.cpp)
			c.done = true
		} else {
			c.Term.byte(0x1A)
		}
	}
	return 2
}

func upper(x byte) byte {
	if x >= 'a' && x <= 'z' {
		return x - 32
	}
	return x
}

// wwiv: WWIV 5's Output::outstr. A heart (Ctrl-C) takes the next byte when it
// is printable: a digit is that colour of the caller's (the defaults here),
// anything else colour 7 (out of range: light grey). | + one or two digits n
// sets the foreground (n < 16, background and blink kept) or the attribute's
// upper half to n << 4 (16-31: the background, bit 7 from 24 on); |# + a digit
// is the caller's colour too. |@ |{ |[ (macros) and others stay as text.
func (c *codeTerm) wwiv(b []byte) int {
	if b[0] == 0x03 {
		if len(b) < 2 || b[1] < 32 || b[1] > 126 {
			return 0
		}
		col := 0x07
		if isDigit(b[1]) {
			col = wwivColours[b[1]-'0']
		}
		c.setAttr(col)
		return 2
	}
	if len(b) < 2 {
		return 0
	}
	if isDigit(b[1]) {
		n, v := 2, int(b[1]-'0')
		if len(b) > 2 && isDigit(b[2]) {
			n, v = 3, v*10+int(b[2]-'0')
		}
		if v < 16 {
			c.setFg(v)
		} else {
			hi := v << 4 & 0xF0
			c.setBg(hi>>4&7, hi&0x80 != 0)
		}
		return n
	}
	if b[1] == '#' {
		if len(b) > 2 && isDigit(b[2]) {
			c.setAttr(wwivColours[b[2]-'0'])
			return 3
		}
		c.setAttr(wwivColours[0]) // no digit is colour 0
		return 2
	}
	return 0
}

// wildcatActs: the bytes after an "@" make a code the Wildcat! reader acts on.
func wildcatActs(b []byte) bool {
	return len(b) >= 3 && isUpperHex(b[0]) && isUpperHex(b[1]) && b[2] == '@' || bytes.HasPrefix(b, []byte("CLS@"))
}

// wildcat: @BF@, background and foreground as capital hex digits (bit 7 of the
// attribute blink or a bright background, as PCBoard's @X), @CLS@ clears.
func (c *codeTerm) wildcat(b []byte) int {
	if !wildcatActs(b[1:]) {
		return 0
	}
	if bytes.HasPrefix(b[1:], []byte("CLS@")) { // C is a hex digit too: the whole word
		c.clearScreen()
		return 5
	}
	c.setAttr(upperHexVal(b[1])<<4 | upperHexVal(b[2]))
	return 4
}

const celerityLetters = "kbgcrmywdBGCRMYW" // the foregrounds 0-15

// celerity: | + a letter of celerityLetters sets the foreground (background
// kept); |S swaps: the foreground's colour becomes the background and the
// foreground black (Synchronet putmsg.cpp, whose wiki shows |b|S|W as bright
// white on blue). Other letters stay as text.
func (c *codeTerm) celerity(b []byte) int {
	if len(b) < 2 {
		return 0
	}
	if b[1] == 'S' {
		c.setAttr((c.bios() & 7) << 4)
		return 2
	}
	if i := strings.IndexByte(celerityLetters, b[1]); i >= 0 {
		c.setFg(i)
		return 2
	}
	return 0
}

// ---------------------------------------------------------------- writing

// bbsLoss counts what a writer could not keep.
type bbsLoss struct {
	blink  int // cells with bit 7 (Celerity has no code for it)
	hearts int // ♥ (Ctrl-C) characters written as spaces (WWIV reads them as colour codes)
	bright int // pipe-code cells with bit 7, written |24-|31 (Mystic 1.10 and Renegade do not show it)
}

// writeBBSCodes gives the page in kind's codes: a code where the colours
// change, rows ended with CR LF unless they fill the width, the board's clear
// first when asked, normal colours at the end. ice: bit 7 is a bright
// background (Ctrl-A E instead of I).
func writeBBSCodes(kind string, d *Doc, clear, ice bool, themes ...map[int]string) ([]byte, bbsLoss) {
	var out []byte
	var theme map[int]string // attribute -> |T0-|T9 / ^0-^9 code to write it with (bbsThemes)
	if len(themes) > 0 {
		theme = themes[0]
	}
	var loss bbsLoss
	if clear {
		out = append(out, map[string]string{"pipe": "|CL", "ctrla": "\x01L", "wildcat": "@CLS@"}[kind]...)
		if kind == "wwiv" || kind == "celerity" {
			out = append(out, "\x1b[2J"...) // no code of their own: the boards pass ANSI on
		}
	}
	attr := -1
	for y := 0; y < d.rows; y++ {
		last := d.lastCol(y)
		// What is written after cell x of this row: its next cells, and after a row
		// that fills the width (no line end) the next row's (-1: a line end or nothing).
		cellAt := func(x int) (int, int) {
			yy, l := y, last
			if x > last && last == d.cols-1 && y+1 < d.rows {
				x, yy = x-d.cols, y+1
				l = d.lastCol(yy)
			}
			if x > l {
				return -1, -1
			}
			v := d.get(x, yy)
			a := int(cellAttr(v))
			if kind == "celerity" {
				a &^= 0x80
			}
			return cellCh(v), a
		}
		chAt := func(x int) int { c, _ := cellAt(x); return c }
		for x := 0; x <= last; x++ {
			v := d.get(x, y)
			a := int(cellAttr(v))
			if kind == "celerity" && a&0x80 != 0 {
				a &^= 0x80
				loss.blink++
			}
			if kind == "pipe" && a&0x80 != 0 && (ice || cellCh(v) != 32) { // (blink on a space shows nothing)
				loss.bright++
			}
			if a != attr {
				if code, ok := theme[a]; ok && kind == "pipe" { // a theme or user colour the file had (brdcodes.go)
					out = append(out, code...)
				} else {
					out = appendCodes(kind, out, attr, a, ice)
				}
				attr = a
			}
			_, na := cellAt(x + 1)
			codeNext := na >= 0 && na != a // the next cell starts with a colour code
			n1, n2 := chAt(x+1), chAt(x+2)
			ch := byte(cellCh(v))
			force := false // a code (the same colours again) after this character keeps it from the next ones
			switch {
			case kind == "pipe" && ch == '|':
				// Mystic takes the two bytes after any "|" (printing all three when
				// they are no code), so a "|" with a code or a "|" in the next two
				// bytes, or that could make one with them, is |PI; so is one near
				// the end of a full row (the next row or the last code follows).
				_, a1 := cellAt(x + 1)
				_, a2 := cellAt(x + 2)
				if n1 >= 0 && (n2 >= 0 && pipeActs(byte(n1), byte(n2)) || n1 == '|' || n2 == '|' || a2 >= 0 && a2 != a1) || codeNext || n2 < 0 && last == d.cols-1 {
					out = append(out, "|PI"...)
					continue
				}
			case kind == "ctrla" && ch == 0x01:
				// A ☺ before a character the reader shows as text is written raw,
				// so codes kept as text (pauses ...) go back as they came.
				if n1 >= 0x20 && n1 < 0x7F && !ctrlaActs(byte(n1)) && !codeNext {
					out = append(out, 0x01)
				} else {
					out = append(out, 0x01, 'A')
				}
				continue
			case kind == "ctrla" && ch == 0x1A:
				out = append(out, 0x01, 'z')
				continue
			case kind == "wwiv" && ch == 0x03:
				ch = ' '
				loss.hearts++
			case kind == "wwiv" && ch == '|':
				force = n1 >= 0 && (isDigit(byte(n1)) || n1 == '#')
			case kind == "wildcat" && ch == '@':
				// Two hex digits (or CLS) after an @ could meet the @ of the
				// next code: "@4E" + "@1F@" reads as @4E@.
				n3 := chAt(x + 3)
				force = n2 >= 0 && isUpperHex(byte(n1)) && isUpperHex(byte(n2)) || n1 == 'C' && n2 == 'L' && n3 == 'S'
			case kind == "celerity" && ch == '|':
				force = n1 >= 0 && (n1 == 'S' || strings.IndexByte(celerityLetters, byte(n1)) >= 0)
			case kind == "pipe" && ch == '^':
				force = n1 >= 0 && isDigit(byte(n1)) // Renegade's user colour ^0-^9 (brdcodes.go)
			case kind == "telegard" && ch == '`':
				force = n2 >= 0 && isUpperHex(byte(n1)) && isUpperHex(byte(n2))
			}
			switch ch { // bytes a terminal acts on
			case 0x00, 0x07, 0x08, 0x09, 0x0A, 0x0C, 0x0D, 0x1A, 0x1B:
				ch = ' '
			}
			out = append(out, ch)
			if force && chAt(x+1) >= 0 {
				attr = -1
			}
		}
		if last < d.cols-1 {
			out = append(out, "\r\n"...)
		}
	}
	if attr != 0x07 && attr >= 0 {
		out = appendCodes(kind, out, attr, 0x07, ice)
	}
	return out, loss
}

// appendCodes writes the codes that change the colours from attribute prev
// (-1: not known) to a, both PC attribute bytes.
func appendCodes(kind string, out []byte, prev, a int, ice bool) []byte {
	switch kind {
	case "pipe", "wwiv":
		// |00-|15 keep the background and blink; |16-|31 set both.
		if prev < 0 || prev&15 != a&15 {
			out = fmt.Appendf(out, "|%02d", a&15)
		}
		if prev < 0 || prev>>4 != a>>4 {
			out = fmt.Appendf(out, "|%02d", 16+(a>>4&7)+(a>>7)*8)
		}
	case "wildcat":
		out = fmt.Appendf(out, "@%02X@", a)
	case "ctrla":
		cur := prev
		if prev < 0 || prev&0x08 != 0 && a&0x08 == 0 || prev&0x80 != 0 && a&0x80 == 0 {
			out = append(out, 0x01, 'N') // only N turns bright and blink off
			cur = 0x07
		}
		if a&0x08 != 0 && cur&0x08 == 0 {
			out = append(out, 0x01, 'H')
		}
		if a&0x80 != 0 && cur&0x80 == 0 {
			out = append(out, 0x01, map[bool]byte{false: 'I', true: 'E'}[ice])
		}
		if a&7 != cur&7 {
			out = append(out, 0x01, "KBGCRMYW"[a&7]) // BIOS order: 1 blue
		}
		if a>>4&7 != cur>>4&7 {
			out = append(out, 0x01, byte('0'+biosANSI[a>>4&7])) // the digits go in ANSI order: 1 red
		}
	case "telegard": // `XY: the attribute (brdcodes.go)
		out = fmt.Appendf(out, "`%02X", a)
	case "celerity":
		if prev < 0 || prev>>4 != a>>4 {
			// |S makes the foreground's colour the background (and the foreground black).
			out = append(out, '|', celerityLetters[a>>4&7], '|', 'S')
			prev = a & 0xF0
		}
		if prev&15 != a&15 {
			out = append(out, '|', celerityLetters[a&15])
		}
	}
	return out
}

// bbsOpenNote says how a file with codes was read and will be saved.
func bbsOpenNote(kind, saveFmt string) string {
	if saveFmt == kind {
		return "Read with its " + bbsCodeName[kind] + "; Save writes them again"
	}
	return "Read with its " + bbsCodeName[kind] + " and ANSI; Save writes ANSI (Save as has the codes)"
}

// lossNote tells what a save in kind could not keep ("" all of it).
func (l bbsLoss) add(m bbsLoss) bbsLoss {
	return bbsLoss{blink: l.blink + m.blink, hearts: l.hearts + m.hearts, bright: l.bright + m.bright}
}

func (l bbsLoss) note(kind string) string {
	var parts []string
	if l.blink > 0 {
		parts = append(parts, fmt.Sprintf("%d blinking / bright-background cells lost it (%s have no code for it)", l.blink, bbsCodeName[kind]))
	}
	if l.bright > 0 {
		parts = append(parts, fmt.Sprintf("%d cells have a bright background or blink: written |24-|31, which WWIV, Synchronet and IcyDraw show and Mystic 1.10 and Renegade do not", l.bright))
	}
	if l.hearts > 0 {
		parts = append(parts, fmt.Sprintf("%d ♥ became spaces (WWIV reads Ctrl-C as a colour code)", l.hearts))
	}
	return strings.Join(parts, "; ")
}
