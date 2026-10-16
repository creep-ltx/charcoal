package main

// Atari ST IGS ("Instant Graphics and Sound", Larry Mears 1988-2024; spec IG220.TXT): the ST BBS graphics
// language. `G#` and one-letter commands (L line, B box, F fill, W text, G blit, S palette, & loop ...)
// in a VT52 stream draw pixels with GEM VDI. This file reads the commands and runs them into a picture of
// the ST's screen (register numbers per pixel); igs_vdi.go draws, igs_page.go puts the picture under an
// Atari ST page's cells. Design: docs/igs-design.md. Ported from IcyDraw's engine (mkrueger/icy_tools,
// Apache-2.0/MIT, used under MIT); nothing is taken from EmuTOS's sources but its fonts, downloaded at run time.

import (
	"bytes"
	"math/rand"
	"os"
	"regexp"
	"strconv"
)

// ---------------------------------------------------------------- reading the commands

// An IGS value: a number, or r / R (a random number in the r or R range), or in a loop x / y (the step
// forwards or backwards) and +n -n !n (the step plus n, the step minus n, n minus the step).
type igsVal struct {
	kind byte // 0 number, 'r', 'R', 'x', 'y', '+', '-', '!'
	n    int
}

// An IGS command as it stands in the file.
type igsCmd struct {
	letter   byte
	start    int  // where it starts: its own "G#", or its letter when it is chained after a ':'
	letterAt int  // its letter
	bodyEnd  int  // its own bytes end here (the terminator is not one of them)
	end      int  // after it and its terminator
	own      bool // right after "G#" (not chained)
	chained  bool // ended by ':' or '@': the next command may follow without "G#"
	vals     []igsVal
	text     []byte // W's text, X 4's zone string, X 7's pattern, X 11's bitmap bytes
	loop     *igsLoop
}

func (c *igsCmd) num(i int) int {
	if i < len(c.vals) && c.vals[i].kind == 0 {
		return c.vals[i].n
	}
	return 0
}

// An item of an IGS file: a command, or VT52 bytes between commands. An ESC E (clear the screen) is an
// item of its own.
type igsItem struct {
	cmd        *igsCmd
	start, end int
	visible    bool // VT52 bytes that print something
	wipe       bool // ESC E
}

// igsLetters are the command letters (IG 2.20 and the VT52 ones).
const igsLetters = "AbBCDEfFgGHIJkKLMmnNOPpQRsStTqUVvWwXYzZ&<?cdilr"

func igsLetter(c byte) bool { return c != 0 && bytes.IndexByte([]byte(igsLetters), c) >= 0 }

// igsFixed is how many values a command takes; -1: as many as come before the ':'; the variable ones
// are worked out from their first value(s) in igsWant.
var igsFixed = map[byte]int{
	'A': 3, 'B': 5, 'C': 2, 'D': 2, 'E': 3, 'F': 2, 'g': 1, 'H': 1, 'I': 1, 'J': 6, 'k': 1, 'K': 5,
	'L': 4, 'M': 1, 'm': 2, 'n': 6, 'N': -1, 'O': 3, 'P': 2, 'p': 2, 'Q': 4, 'R': 2, 's': 1, 'S': 4,
	't': 1, 'T': 3, 'q': 1, 'U': 5, 'V': 5, 'v': 1, 'W': 2, 'w': 1, 'Y': 6, 'Z': 4, '<': -1,
	'c': 2, 'd': 1, 'i': 2, 'l': 1, 'r': 1,
}

// igsWant is how many values command c takes, given those read so far.
func igsWant(c byte, v []igsVal) int {
	first := 0
	if len(v) > 0 {
		first = v[0].n
	}
	switch c {
	case 'z', 'f': // a count, then that many points
		if len(v) == 0 {
			return 1
		}
		return 1 + 2*max(0, min(first, 1024))
	case 'b':
		switch first {
		case 20:
			return 7
		case 22, 23:
			return 2
		}
		return 1
	case 'G':
		if n, ok := map[int]int{0: 8, 1: 6, 2: 4, 3: 8, 4: 8}[first]; ok {
			return n
		}
		return 2
	case '?':
		if first == 1 || first == 2 {
			return 2
		}
		return 1
	case 'X':
		switch first {
		case 0:
			return 6
		case 1, 2, 10:
			return 3
		case 4:
			if len(v) >= 2 && v[1].n >= 9997 && v[1].n <= 9999 {
				return 2
			}
			return 7
		case 5, 6, 7:
			return 2
		case 8:
			return 5
		case 11:
			if len(v) >= 2 && (v[1].n == 1 || v[1].n == 2) {
				return 3 // then the bitmap's bytes
			}
			return 4
		case 12:
			return 5
		}
		return -1
	}
	if n, ok := igsFixed[c]; ok {
		return n
	}
	return -1
}

func isDigitByte(c byte) bool { return c >= '0' && c <= '9' }

// igsReadVal reads a value from j: spaces, '>' and '_' before it are skipped, CR and LF anywhere in it
// (IG 2.18 lets a value go on over lines). It returns the value and where the byte that ended it is.
func igsReadVal(b []byte, j int) (igsVal, int) {
	for j < len(b) && (b[j] == ' ' || b[j] == '>' || b[j] == '_' || b[j] == '\r' || b[j] == '\n') {
		j++
	}
	var v igsVal
	neg, digits := false, false
	if j+1 < len(b) && b[j] == '-' && isDigitByte(b[j+1]) {
		neg = true
		j++
	}
	for ; j < len(b); j++ {
		c := b[j]
		switch {
		case isDigitByte(c):
			if v.n < 1<<20 {
				v.n = v.n*10 + int(c-'0')
			}
			digits = true
		case c == '\r' || c == '\n' || c == '_':
		case (c == 'r' || c == 'R') && !digits && v.kind == 0:
			v.kind = c
		default:
			if neg {
				v.n = -v.n
			}
			return v, j
		}
	}
	if neg {
		v.n = -v.n
	}
	return v, j
}

// igsAhead: does the byte at t start something that is not the command's (another "G#" or an escape)?
func igsAhead(b []byte, t int) bool {
	return t < len(b) && (b[t] == 0x1B || (b[t] == 'G' && t+1 < len(b) && b[t+1] == '#'))
}

// igsCommand reads the command whose letter is at i.
func igsCommand(b []byte, start, i int, own bool) *igsCmd {
	c := &igsCmd{letter: b[i], start: start, letterAt: i, own: own}
	if c.letter == '&' {
		igsReadLoop(b, c)
		return c
	}
	j := i + 1
	for {
		v, t := igsReadVal(b, j)
		c.vals = append(c.vals, v)
		want := igsWant(c.letter, c.vals)
		if t >= len(b) {
			c.bodyEnd, c.end = len(b), len(b)
			return c
		}
		ch := b[t]
		switch {
		case c.letter == 'W' && len(c.vals) == 2: // the text, up to '@'; CR and LF are not part of it
			c.bodyEnd, c.end = igsReadText(b, t+1, c)
			return c
		case c.letter == 'X' && c.num(0) == 7 && len(c.vals) == 2 && ch != ':': // 16 rows of 16 + '@'
			k, rows := t+1, 0
			for ; k < len(b) && rows < 16 && b[k] != ':' && !igsAhead(b, k); k++ {
				if b[k] == '@' {
					rows++
				}
				if b[k] != '\r' && b[k] != '\n' {
					c.text = append(c.text, b[k])
				}
			}
			for k < len(b) && (b[k] == '\r' || b[k] == '\n' || b[k] == ' ') {
				k++
			}
			c.bodyEnd, c.end = k, k
			if k < len(b) && b[k] == ':' {
				c.end, c.chained = k+1, true
			}
			return c
		case c.letter == 'X' && c.num(0) == 4 && len(c.vals) == 7 && ch != ':': // the zone's string
			k := t + 1
			for ; k < len(b) && b[k] != ':' && b[k] != '\n' && !igsAhead(b, k); k++ {
				c.text = append(c.text, b[k])
			}
			c.bodyEnd, c.end = k, min(k+1, len(b))
			c.chained = k < len(b) && b[k] == ':'
			return c
		case c.letter == 'X' && c.num(0) == 11 && len(c.vals) == 3 && (c.num(1) == 1 || c.num(1) == 2):
			n := 32000 // the whole screen's memory, or one of 8 bands of 4000 bytes
			if c.num(2) >= 1 && c.num(2) <= 8 {
				n = 4000
			}
			k := min(len(b), t+1+n)
			c.text = b[t+1 : k]
			c.bodyEnd, c.end = k, k
			return c
		case ch == ':':
			c.bodyEnd, c.end, c.chained = t, t+1, true
			return c
		case want >= 0 && len(c.vals) >= want && ch != ',':
			c.bodyEnd, c.end = t, t+1
			if igsAhead(b, t) {
				c.end = t
			}
			return c
		case igsAhead(b, t) || ch == '@':
			c.bodyEnd, c.end = t, t
			if ch == '@' {
				c.end++
			}
			return c
		}
		j = t + 1
	}
}

// igsReadText reads W's text from j to its '@' (CR and LF are left out, as IG 2.20 does); a text with
// no '@' within 512 bytes ends at its line's end, one with none before an escape there.
func igsReadText(b []byte, j int, c *igsCmd) (bodyEnd, end int) {
	k := j
	for ; k < len(b) && b[k] != 0x1B; k++ {
		switch b[k] {
		case '@':
			c.chained = true
			return k + 1, k + 1
		case '\r', '\n':
			if k-j > 512 && b[k] == '\n' {
				return k, k + 1
			}
			continue
		}
		c.text = append(c.text, b[k])
	}
	return k, k
}

// An IGS loop (&): from, to, step, delay, the command (or a "chain gang" of commands), how many values
// each step takes, and those values (a ring read on and on, as IG 2.17 reads them).
type igsLoop struct {
	from, to, step int
	target         []byte // one command, or the chain gang's commands
	gang           bool
	xor, refresh   bool // | and @ after the command
	count          int
	params         []igsVal
	texts          [][]byte // W@'s texts, one per step
}

func (l *igsLoop) steps() int {
	if l.step == 0 {
		return 0
	}
	return abs(l.to-l.from)/abs(l.step) + 1
}

// igsReadLoop reads a loop's tokens: comma separated (')' and a chain gang's command letter after a
// number also separate), ':' between groups of values, up to the ':' after the last value.
func igsReadLoop(b []byte, c *igsCmd) {
	l := &igsLoop{}
	c.loop = l
	var toks [][]byte
	var tok []byte
	push := func() {
		if len(tok) > 0 {
			toks = append(toks, tok)
			tok = nil
		}
	}
	vals := func() int { // the values read after the head (not the ':' between groups)
		n := 0
		for k := 6; k < len(toks); k++ {
			if string(toks[k]) != ":" {
				n++
			}
		}
		return n
	}
	head := func() {
		l.from, _ = strconv.Atoi(string(toks[0]))
		l.to, _ = strconv.Atoi(string(toks[1]))
		l.step, _ = strconv.Atoi(string(toks[2]))
		id := toks[4]
		if len(id) > 0 && id[0] == '>' { // a chain gang: >CL@
			e := bytes.IndexByte(id, '@')
			if e < 0 {
				e = len(id)
			}
			for _, ch := range id[1:e] {
				if igsLetter(ch) && ch != '&' {
					l.target = append(l.target, ch)
				}
			}
			l.gang = true
			id = id[min(len(id), e+1):]
		} else if len(id) > 0 {
			l.target = []byte{id[0]}
			id = id[1:]
		}
		for k, ch := range id {
			switch {
			case ch == '|':
				l.xor = true
			case ch == '@':
				l.refresh = true
			case isDigitByte(ch): // the count written straight after the modifiers: W@2
				n, _ := strconv.Atoi(string(id[k:]))
				l.count = n
				toks = append(toks[:5], append([][]byte{[]byte(strconv.Itoa(n))}, toks[5:]...)...)
				return
			}
		}
	}
	gang := false
	j := c.letterAt + 1
	headDone := false
	finish := func(k int, chained bool) {
		c.bodyEnd, c.end, c.chained = k, min(k+1, len(b)), chained
		if len(toks) >= 6 {
			l.count, _ = strconv.Atoi(string(toks[5]))
			for _, t := range toks[6:] {
				l.params = append(l.params, igsLoopToken(t))
			}
		}
	}
	for ; j < len(b); j++ {
		ch := b[j]
		if !headDone && len(toks) >= 5 {
			head()
			headDone = true
		}
		// W@: after the values, one text per step, each ended by '@'.
		if headDone && l.refresh && !l.gang && len(l.target) == 1 && l.target[0] == 'W' && len(toks) >= 6 {
			cnt, _ := strconv.Atoi(string(toks[5]))
			if vals() >= cnt && len(tok) == 0 && ch != ',' && ch != ' ' && ch != '\r' && ch != '\n' {
				k := j
				for len(l.texts) < l.steps() && k < len(b) && len(l.texts) < 4096 && !igsAhead(b, k) {
					e := k
					var t []byte
					for ; e < len(b) && b[e] != '@' && !igsAhead(b, e); e++ {
						if b[e] != '\r' && b[e] != '\n' {
							t = append(t, b[e])
						}
					}
					l.texts = append(l.texts, t)
					k = e
					if e < len(b) && b[e] == '@' {
						k++
					}
				}
				for k < len(b) && b[k] == ' ' {
					k++
				}
				finish(min(k, len(b)), k < len(b) && b[k] == ':')
				if k >= len(b) || b[k] != ':' {
					c.end = min(k, len(b))
				}
				return
			}
		}
		switch {
		case ch == ',' && !gang:
			push()
		case ch == ')':
			push()
		case ch == ':':
			push()
			if len(toks) >= 6 {
				cnt, _ := strconv.Atoi(string(toks[5]))
				if vals() >= cnt {
					finish(j, true)
					return
				}
				toks = append(toks, []byte(":"))
			}
		case ch == '\n':
			push()
			if len(toks) >= 6 {
				cnt, _ := strconv.Atoi(string(toks[5]))
				if vals() >= cnt {
					finish(j, false)
					return
				}
			}
		case ch == ' ' || ch == '\r' || ch == '_':
		case ch == '>':
			if len(tok) == 0 && len(toks) == 4 { // a chain gang starts
				tok, gang = append(tok, ch), true
			}
		case ch == '@' && gang:
			tok = append(tok, ch)
			push()
			gang = false
		case igsAhead(b, j):
			push()
			if !headDone && len(toks) >= 5 {
				head()
			}
			finish(j, false)
			c.end = j
			return
		case headDone && l.gang && igsLetter(ch) && len(tok) > 0 && isDigitByte(tok[len(tok)-1]):
			push() // 0C1,1: the chain gang's index, then its command's letter in place of ')'
		default:
			tok = append(tok, ch)
		}
		if len(toks) > 1<<16 {
			finish(j, false)
			return
		}
	}
	push()
	if !headDone && len(toks) >= 5 {
		head()
	}
	finish(len(b), false)
	c.end = len(b)
}

func igsLoopToken(t []byte) igsVal {
	if string(t) == ":" {
		return igsVal{kind: ':'}
	}
	if len(t) > 1 && (t[0] == '+' || t[0] == '-' || t[0] == '!') { // the step plus, minus, from n
		v := igsLoopToken(t[1:])
		if v.kind == 0 {
			v.kind = t[0]
			return v
		}
	}
	switch string(t) {
	case "x", "y", "r", "R":
		return igsVal{kind: t[0]}
	}
	n, _ := strconv.Atoi(string(t))
	return igsVal{n: max(-1<<20, min(1<<20, n))}
}

// igsChainedAt: is there a command chained after a ':' at i? Its letter followed by '>', as the spec
// writes them, or by a space and a value ("k 1:"); "We all" after "p>24,19:" is text (the spec's example).
func igsChainedAt(b []byte, i int) bool {
	if i+1 >= len(b) || !igsLetter(b[i]) {
		return false
	}
	if b[i+1] == '>' {
		return true
	}
	return b[i+1] == ' ' && i+2 < len(b) && (isDigitByte(b[i+2]) || b[i+2] == 'r' || b[i+2] == 'R')
}

// igsParse cuts an IGS file into its items.
func igsParse(b []byte) []igsItem {
	var items []igsItem
	ts, vis, esc := -1, false, 0 // the VT52 bytes' start, whether they print, bytes owed to an escape
	flush := func(end int) {
		if ts >= 0 && end > ts {
			items = append(items, igsItem{start: ts, end: end, visible: vis})
		}
		ts, vis = -1, false
	}
	chained := false
	for i := 0; i < len(b); {
		c := b[i]
		if chained { // after a ':' the next command needs no "G#": a letter followed by '>' (or a space and a value)
			chained = false
			if igsChainedAt(b, i) {
				cmd := igsCommand(b, i, i, false)
				items = append(items, igsItem{cmd: cmd, start: cmd.start, end: cmd.end})
				i, chained = max(cmd.end, i+1), cmd.chained
				continue
			}
			if c == ' ' { // a space between chained commands is not shown (IcyDraw drops it too)
				i, chained = i+1, true
				continue
			}
			if c == '\r' || c == '\n' { // the line's end after the chain is not shown
				i++
				if c == '\r' && i < len(b) && b[i] == '\n' {
					i++
				}
				continue
			}
		}
		if esc == 0 && c == 'G' && i+2 < len(b) && b[i+1] == '#' && igsLetter(b[i+2]) {
			flush(i)
			cmd := igsCommand(b, i, i+2, true)
			items = append(items, igsItem{cmd: cmd, start: cmd.start, end: cmd.end})
			i, chained = max(cmd.end, i+3), cmd.chained
			continue
		}
		if ts < 0 {
			ts = i
		}
		switch esc {
		case 0:
			if c == 0x1B {
				esc = -1
			} else if c >= 0x20 {
				vis = true
			}
		case -1: // the escape's letter
			esc = 0
			switch c {
			case 'Y':
				esc = 2
			case 'b', 'c':
				esc = 1
			case 'E':
				flush(i - 1)
				items = append(items, igsItem{start: i - 1, end: i + 1, wipe: true})
				i++
				continue
			case 0x1B:
				esc = -1
			}
		default:
			esc--
		}
		i++
	}
	flush(len(b))
	return items
}

// isIGS: does b hold IGS commands? Two at least ("G#" and a letter followed by '>', a space or a digit),
// or one in a file named .ig*; an ANSI file is not IGS.
func isIGS(b []byte, name string) bool {
	n := 0
	for _, m := range igsSniff.FindAllIndex(b[:min(len(b), 1<<16)], 8) {
		_ = m
		n++
	}
	if n == 0 || bytes.Contains(b[:min(len(b), 4096)], []byte("\x1b[")) {
		return false
	}
	return n >= 2 || igsExt.MatchString(name)
}

var igsSniff = regexp.MustCompile(`G#[` + regexp.QuoteMeta(igsLetters) + `][> ]?[0-9rR>]`)
var igsExt = regexp.MustCompile(`(?i)\.(ig[0-9s]?|gr[0-9])$`)

// ---------------------------------------------------------------- the machine

const (
	igsLow = iota
	igsMedium
	igsHigh
)

var igsResIDs = []string{"stlow", "stmed", "sthigh"}

func igsResOf(id string) int {
	for i, r := range igsResIDs {
		if r == id {
			return i
		}
	}
	return igsLow
}

// The VDI pens' colour registers (IG's chart): pen 1 is register 15 (black) in low resolution.
var igsPenMap = [3][16]uint8{
	{0, 15, 1, 2, 4, 6, 3, 5, 7, 8, 9, 10, 12, 14, 11, 13},
	{0, 3, 1, 2, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3, 3},
	{0, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1},
}

// stLevel is an ST colour level 0-7 as the screen shows it (x 34: 7 is 0xEE).
func stLevel(n int) uint32 { return uint32(max(0, min(7, n)) * 34) }

func stRGB(r, g, b int) uint32 { return 0xFF000000 | stLevel(r)<<16 | stLevel(g)<<8 | stLevel(b) }

// stWord is an ST/STE colour register's word: a nibble per channel, the STE's extra low bit in bit 3.
func stWord(v int) uint32 {
	lv := func(s uint) uint32 {
		n := (v >> s) & 15
		return uint32(((n&7)<<1 | n>>3) * 17)
	}
	return 0xFF000000 | lv(8)<<16 | lv(4)<<8 | lv(0)
}

func igsPal(rgb ...[3]int) [16]uint32 {
	var p [16]uint32
	for i := range p {
		c := rgb[i%len(rgb)]
		p[i] = stRGB(c[0], c[1], c[2])
	}
	return p
}

// The palettes: TOS's desktop (= VDI's default in low resolution), IG's own default, medium and high.
var (
	igsDesktop = igsPal([3]int{7, 7, 7}, [3]int{7, 0, 0}, [3]int{0, 7, 0}, [3]int{7, 7, 0}, [3]int{0, 0, 7},
		[3]int{7, 0, 7}, [3]int{0, 7, 7}, [3]int{5, 5, 5}, [3]int{3, 3, 3}, [3]int{7, 3, 3}, [3]int{3, 7, 3},
		[3]int{7, 7, 3}, [3]int{3, 3, 7}, [3]int{7, 3, 7}, [3]int{3, 7, 7}, [3]int{0, 0, 0})
	igsDefault = igsPal([3]int{7, 7, 7}, [3]int{7, 0, 0}, [3]int{0, 7, 0}, [3]int{0, 0, 0}, [3]int{0, 0, 7},
		[3]int{7, 7, 0}, [3]int{0, 7, 7}, [3]int{5, 4, 6}, [3]int{5, 3, 2}, [3]int{7, 4, 3}, [3]int{0, 4, 3},
		[3]int{0, 3, 0}, [3]int{3, 3, 3}, [3]int{2, 1, 0}, [3]int{1, 3, 4}, [3]int{0, 0, 0})
	igsMedPal  = igsPal([3]int{7, 7, 7}, [3]int{7, 0, 0}, [3]int{0, 7, 0}, [3]int{0, 0, 0})
	igsHighPal = igsPal([3]int{7, 7, 7}, [3]int{0, 0, 0})
)

func igsResPal(res int) [16]uint32 {
	return [3][16]uint32{igsDesktop, igsMedPal, igsHighPal}[res]
}

// igsVM runs IGS commands into a picture of the ST's screen.
type igsVM struct {
	res, w, h int
	pix       []uint8 // colour registers, w x h
	pal       [16]uint32

	// VDI's attributes.
	drawTo                                     [2]int
	markerColor, lineColor, fillColor, textCol uint8
	textEffects, textSize, textRot             int
	markerType, markerSize                     int
	lineKind, drawMode, lineWidth              int
	lineEnds                                   [2]int // 0 square, 1 arrow, 2 rounded
	userMask                                   uint16
	sprayOn                                    bool
	sprayPen, sprayFirst                       uint8
	rotSaved                                   *[16]uint32
	fillKind, fillIndex                        int // 0 hollow 1 solid 2 pattern 3 hatch 4 user 5 random 6 Star Trek
	fillBorder                                 bool
	blitW, blitH                               int
	blit                                       []uint8
	userPat                                    [8][]uint16
	small, big                                 [2]int // the r and R ranges
	rng                                        *rand.Rand
	lastText                                   []byte

	// VT52 on the screen.
	tx, ty, tfg, tbg, tsx, tsy int
	trev, twrap                bool

	work  int     // pixels and steps done; past igsWorkMax the rest is not drawn (a hostile file)
	probe *[2]int // when not nil nothing is drawn: the largest x and y drawn at are noted (igsGuessRes)
}

const igsWorkMax = 40_000_000

func newIgsVM(res int) *igsVM {
	vm := &igsVM{rng: rand.New(rand.NewSource(1988))}
	vm.setRes(res)
	vm.pal = igsResPal(res)
	vm.resetAttrs()
	vm.small, vm.big = [2]int{0, 199}, [2]int{0, 199}
	for i := range vm.userPat {
		vm.userPat[i] = []uint16{0}
	}
	return vm
}

func (vm *igsVM) setRes(res int) {
	vm.res = res
	vm.w, vm.h = [3]int{320, 640, 640}[res], [3]int{200, 200, 400}[res]
	vm.pix = make([]uint8, vm.w*vm.h)
	vm.tfg, vm.tbg = [3]int{15, 3, 1}[res], 0
	vm.tx, vm.ty, vm.twrap, vm.trev = 0, 0, false, false
}

func (vm *igsVM) colours() int { return [3]int{16, 4, 2}[vm.res] }

func (vm *igsVM) defaultPen() uint8 { return [3]uint8{15, 3, 1}[vm.res] }

func (vm *igsVM) resetAttrs() {
	p := vm.defaultPen()
	vm.markerType, vm.markerColor, vm.drawMode, vm.markerSize = 1, p, 1, 1
	vm.lineKind, vm.lineColor, vm.lineWidth, vm.lineEnds = 1, p, 1, [2]int{}
	vm.fillKind, vm.fillColor = 1, p
	vm.textCol, vm.textRot, vm.textEffects, vm.textSize = p, 0, 0, 9
	vm.drawTo = [2]int{}
	vm.fillBorder = false
	vm.userMask = 0xAAAA
}

func (vm *igsVM) val(v igsVal) int {
	switch v.kind {
	case 'r':
		return vm.random(vm.small)
	case 'R':
		return vm.random(vm.big)
	}
	return v.n
}

func (vm *igsVM) random(r [2]int) int {
	lo, hi := min(r[0], r[1]), max(r[0], r[1])
	return lo + vm.rng.Intn(hi-lo+1)
}

// args evaluates a command's values (random ones drawn now), clamped to VDI's 16 bits.
func (vm *igsVM) args(c *igsCmd) []int {
	out := make([]int, len(c.vals))
	for i, v := range c.vals {
		out[i] = max(-32768, min(32767, vm.val(v)))
	}
	return out
}

// ---------------------------------------------------------------- running

// igsDraws: does the command change pixels (or the whole screen)?
func igsDraws(c *igsCmd) bool {
	switch c.letter {
	case 'B', 'L', 'D', 'O', 'Q', 'K', 'J', 'V', 'Y', 'U', 'Z', 'f', 'z', 'P', 'F', 'W', '&':
		return true
	case 'G':
		return c.num(0) != 1 && c.num(0) != 4 // not into the blit memory only
	case 'X':
		return c.num(0) == 0 || (c.num(0) == 11 && c.num(1) == 1)
	}
	return false
}

// igsWipes: does the command clear or reset the screen (or its palette)?
func igsWipes(c *igsCmd) bool { return c.letter == 's' || c.letter == 'I' || c.letter == 'R' }

// igsTextCmd: IG's own VT52 commands (colours, cursor, lines) that act on the text.
func igsTextCmd(c *igsCmd) bool { return bytes.IndexByte([]byte("cpmldirvw"), c.letter) >= 0 }

// igsPaletteCmd: does the command change colour registers?
func igsPaletteCmd(c *igsCmd) bool {
	return c.letter == 'S' || (c.letter == 'X' && (c.num(0) == 1 || c.num(0) == 8 || c.num(0) == 12))
}

func (vm *igsVM) busy() bool { return vm.work > igsWorkMax }

// run runs a command.
func (vm *igsVM) run(c *igsCmd) {
	if vm.busy() {
		return
	}
	vm.work += 16
	if c.loop != nil {
		vm.runLoop(c.loop)
		return
	}
	a := vm.args(c)
	vm.exec(c.letter, a, c.text)
}

func igsAt(a []int, i int) int {
	if i < len(a) {
		return a[i]
	}
	return 0
}

// exec runs command letter with its values a (and its text).
func (vm *igsVM) exec(letter byte, a []int, text []byte) {
	need := igsWant(letter, igsVals(a))
	if need > 0 && len(a) < need && !(letter == 'B' && len(a) == 4) {
		return // too few values: IG would wait for them, IcyDraw drops the command
	}
	if vm.probe != nil {
		vm.noteCoords(letter, a)
		return
	}
	switch letter {
	case 'B':
		if igsAt(a, 4) != 0 {
			vm.roundRect(a[0], a[1], a[2], a[3])
		} else {
			vm.rect(a[0], a[1], a[2], a[3])
		}
	case 'L':
		vm.styledLine([]int{a[0], a[1], a[2], a[3]}, vm.lineColor)
		vm.drawTo = [2]int{a[2], a[3]}
	case 'D':
		vm.styledLine([]int{vm.drawTo[0], vm.drawTo[1], a[0], a[1]}, vm.lineColor)
		vm.drawTo = [2]int{a[0], a[1]}
	case 'O':
		vm.circle(a[0], a[1], a[2])
	case 'Q':
		vm.ellipse(a[0], a[1], a[2], a[3])
	case 'K':
		vm.arc(a[0], a[1], a[2], a[2], a[3], a[4])
	case 'J':
		vm.arc(a[0], a[1], a[2], a[3], a[4], a[5])
	case 'V':
		vm.pie(a[0], a[1], a[2], vm.circleY(a[2]), a[3], a[4])
	case 'Y':
		vm.pie(a[0], a[1], a[2], vm.circleY(a[3]), a[4], a[5])
	case 'U':
		vm.roundRect(a[0], a[1], a[2], a[3])
	case 'Z':
		vm.fillRect(a[0], a[1], a[2], a[3])
	case 'z':
		pts := a[1 : 1+2*max(0, min(a[0], 1024))]
		vm.styledLine(pts, vm.lineColor)
		if len(pts) >= 2 {
			vm.drawTo = [2]int{pts[len(pts)-2], pts[len(pts)-1]}
		}
	case 'f':
		if pts := a[1 : 1+2*max(0, min(a[0], 1024))]; len(pts) >= 2 {
			vm.fillPoly(pts)
		}
	case 'P':
		vm.marker(a[0], a[1])
		vm.drawTo = [2]int{a[0], a[1]}
	case 'F':
		vm.floodFill(a[0], a[1])
	case 'W':
		if text == nil {
			text = vm.lastText
		}
		vm.lastText = text
		vm.writeText(a[0], a[1], text)
	case 'C':
		pen := igsPenMap[vm.res][a[1]&15]
		switch a[0] {
		case 0:
			vm.markerColor = pen
		case 1:
			vm.lineColor = pen
		case 2:
			vm.fillColor = pen
		case 3:
			vm.textCol = pen
		}
	case 'A':
		vm.setFill(a[0], a[1])
		vm.fillBorder = a[2] != 0
	case 'T':
		vm.lineType(a[0], a[1], a[2])
	case 'E':
		vm.textEffects, vm.textSize, vm.textRot = a[0]&31, a[1], a[2]&3
	case 'M':
		if a[0] >= 1 && a[0] <= 4 {
			vm.drawMode = a[0]
		}
	case 'H':
		if a[0] != 0 {
			vm.fillKind, vm.drawMode, vm.fillBorder = 0, 2, true
		} else {
			vm.fillKind, vm.drawMode, vm.fillBorder = 1, 1, false
		}
	case 'S':
		if p := a[0]; p >= 0 && p < vm.colours() {
			vm.pal[igsPenMap[vm.res][p]] = stRGB(a[1], a[2], a[3])
		}
	case 'G':
		vm.grab(a)
	case 'I':
		vm.initialize(a[0])
	case 's':
		vm.clear(a[0])
	case 'R':
		vm.resolution(a[0], a[1])
	case 'X':
		vm.extended(a, text)
	case 'c', 'p', 'm', 'l', 'd', 'i', 'r', 'v', 'w':
		vm.textCommand(letter, a)
	}
}

// noteCoords notes the largest x and y a drawing command draws at.
func (vm *igsVM) noteCoords(letter byte, a []int) {
	var xs, ys []int
	switch letter {
	case 'L', 'B', 'Z', 'U':
		xs, ys = []int{0, 2}, []int{1, 3}
	case 'D', 'P', 'F', 'W', 'O', 'Q', 'K', 'J', 'V', 'Y':
		xs, ys = []int{0}, []int{1}
	case 'f', 'z':
		for i := 1; i+1 < len(a); i += 2 {
			xs, ys = append(xs, i), append(ys, i+1)
		}
	case 'G':
		xs, ys = []int{2, 4, 6}, []int{3, 5, 7}
	}
	for _, i := range xs {
		vm.probe[0] = max(vm.probe[0], igsAt(a, i))
	}
	for _, i := range ys {
		vm.probe[1] = max(vm.probe[1], igsAt(a, i))
	}
}

func igsVals(a []int) []igsVal {
	v := make([]igsVal, len(a))
	for i, n := range a {
		v[i].n = n
	}
	return v
}

func (vm *igsVM) setFill(kind, index int) {
	switch kind {
	case 0, 1:
		vm.fillKind = kind
	case 2:
		if index >= 1 && index <= 24 {
			vm.fillKind, vm.fillIndex = 2, index
		}
	case 3:
		if index >= 1 && index <= 12 {
			vm.fillKind, vm.fillIndex = 3, index
		}
	case 4:
		switch {
		case index == 9:
			vm.fillKind = 6
		case index == 8:
			vm.fillKind = 5
		case index >= 0 && index <= 7:
			vm.fillKind, vm.fillIndex = 4, index
		}
	}
}

// lineType is T: 1 = polymarker type and size, 2 = line type and width (1-41) or ends (0, 50-54, 60-64).
func (vm *igsVM) lineType(what, kind, n int) {
	switch what {
	case 1:
		if kind < 1 || kind > 6 {
			kind = 1
		}
		if n < 1 || n > 8 {
			n = 1
		}
		vm.markerType, vm.markerSize = kind, n
	case 2:
		if kind < 1 || kind > 7 {
			kind = 1
		}
		vm.lineKind = kind
		if n > 0 && n < 42 {
			switch kind {
			case 1:
				vm.lineWidth = n
			case 7: // a user line pattern's number: rows of fill patterns 6 and 7
				vm.userMask = vm.userLine(min(n, 32))
				vm.lineWidth = 1
			default:
				vm.lineWidth = 1
			}
			return
		}
		ends := map[int][2]int{0: {0, 0}, 50: {1, 1}, 51: {1, 0}, 52: {0, 1}, 53: {1, 2}, 54: {2, 1}, 60: {2, 2},
			61: {2, 0}, 62: {0, 2}, 63: {2, 1}, 64: {1, 2}}
		vm.lineEnds = ends[n]
	}
}

func (vm *igsVM) userLine(n int) uint16 {
	i := max(1, n) - 1
	p := vm.userPat[6+i/16]
	if len(p) < 16 {
		return 0xAAAA
	}
	return p[i%16]
}

func (vm *igsVM) initialize(mode int) {
	switch mode {
	case 0:
		vm.pal = igsResPal(vm.res)
		vm.resetAttrs()
	case 1, 4:
		vm.pal = igsResPal(vm.res)
	case 2:
		vm.resetAttrs()
	case 3:
		if vm.res == igsLow {
			vm.pal = igsDefault
		} else {
			vm.pal = igsResPal(vm.res)
		}
	}
}

func (vm *igsVM) resolution(res, palette int) {
	if res < 0 || res > 2 {
		return
	}
	if res != vm.res {
		p := [3]uint8{15, 3, 1}[res]
		vm.markerColor, vm.lineColor, vm.fillColor, vm.textCol = p, p, p, p
		vm.setRes(res)
		vm.blit, vm.blitW, vm.blitH = nil, 0, 0
	}
	switch {
	case res != igsLow:
		vm.pal = igsResPal(res)
	case palette == 1 || palette == 3:
		vm.pal = igsDesktop
	case palette == 2:
		vm.pal = igsDefault
	}
}

// clear is s: 0 clear and home, 1 clear up to the cursor, 2 from the cursor down, 3 the whole screen, 4
// the whole screen and home, 5 a quick VT52 reset (colours too).
func (vm *igsVM) clear(mode int) {
	ch := vm.cellH()
	switch mode {
	case 0, 3, 4:
		clear(vm.pix)
		if mode != 3 {
			vm.tx, vm.ty = 0, 0
		}
	case 1:
		for i := 0; i < min(len(vm.pix), (vm.ty+1)*ch*vm.w); i++ {
			vm.pix[i] = uint8(vm.tbg)
		}
	case 2:
		for i := vm.ty * ch * vm.w; i < len(vm.pix); i++ {
			vm.pix[i] = uint8(vm.tbg)
		}
	case 5:
		vm.pal = igsResPal(vm.res)
		clear(vm.pix)
		vm.tx, vm.ty, vm.trev, vm.twrap = 0, 0, false, false
		vm.tfg, vm.tbg = [3]int{15, 3, 1}[vm.res], 0
	}
}

func (vm *igsVM) extended(a []int, text []byte) {
	switch a[0] {
	case 0: // spray paint: x, y, width, height, density; X 0,pen,0,0,0,0 cycles the pens
		x, y, w, h, n := igsAt(a, 1), igsAt(a, 2), igsAt(a, 3), igsAt(a, 4), igsAt(a, 5)
		if w == 0 && h == 0 && n == 0 {
			vm.sprayOn = x > 0 && x < vm.colours()
			vm.sprayPen, vm.sprayFirst = uint8(max(0, x)), uint8(max(0, x))
			return
		}
		w, h = max(0, min(255, w)), max(0, min(255, h))
		keep := vm.markerColor
		for i := 0; i < max(0, min(9999, n)); i++ {
			if vm.sprayOn {
				vm.markerColor = igsPenMap[vm.res][vm.sprayPen&15]
				if int(vm.sprayPen)+1 >= vm.colours() {
					vm.sprayPen = vm.sprayFirst
				} else {
					vm.sprayPen++
				}
			}
			vm.marker(x+vm.rng.Intn(w+1), y+vm.rng.Intn(h+1))
			if vm.work += 64; vm.busy() {
				break
			}
		}
		vm.markerColor = keep
	case 1: // a colour register's word
		if r := igsAt(a, 1); r >= 0 && r < vm.colours() {
			vm.pal[r] = stWord(igsAt(a, 2))
		}
	case 2:
		if len(a) >= 4 {
			vm.big = [2]int{a[1], a[3]}
		} else {
			vm.small = [2]int{igsAt(a, 1), igsAt(a, 2)}
		}
	case 7: // a 16 x 16 fill pattern
		if p := igsAt(a, 1); p >= 0 && p < 8 {
			if pat := igsPattern(text); pat != nil {
				vm.userPat[p] = pat
			}
		}
	case 8: // rotate colour registers
		s, e, n := igsAt(a, 1), igsAt(a, 2), igsAt(a, 3)
		if n == 0 || s == e {
			if vm.rotSaved != nil {
				vm.pal, vm.rotSaved = *vm.rotSaved, nil
			}
		} else if s >= 0 && e >= 0 && max(s, e) < vm.colours() {
			if vm.rotSaved == nil {
				p := vm.pal
				vm.rotSaved = &p
			}
			lo, hi := min(s, e), max(s, e)
			seg := append([]uint32(nil), vm.pal[lo:hi+1]...)
			k := abs(n) % len(seg)
			for i := range seg {
				if s < e {
					vm.pal[lo+(i+k)%len(seg)] = seg[i]
				} else {
					vm.pal[lo+i] = seg[(i+k)%len(seg)]
				}
			}
		}
	case 10:
		vm.drawTo = [2]int{igsAt(a, 1), igsAt(a, 2)}
	case 11:
		switch igsAt(a, 1) {
		case 0:
			vm.wipeBlit(igsAt(a, 2), igsAt(a, 3))
		case 1, 2:
			vm.loadBlit(igsAt(a, 2), text)
			if igsAt(a, 1) == 1 {
				vm.blitToScreen(vm.blit, vm.blitW, vm.blitH, 0, 0, vm.blitW, vm.blitH, 0, 0, 3)
			}
		}
	case 12: // four colour registers' words from bank*4
		bank := max(0, min(3, igsAt(a, 1)))
		for k := 0; k < 4 && 2+k < len(a); k++ {
			if r := bank*4 + k; r < vm.colours() {
				vm.pal[r] = stWord(a[2+k])
			}
		}
	}
}

// igsPattern reads X 7's 16 rows of 16 characters, each ended by '@' (X or x is a set pixel).
func igsPattern(t []byte) []uint16 {
	var out []uint16
	for len(out) < 16 {
		at := bytes.IndexByte(t, '@')
		if at < 0 {
			return nil
		}
		var w uint16
		seg := t[:at]
		if len(seg) > 16 {
			seg = seg[len(seg)-16:]
		}
		for k, c := range seg {
			if c == 'X' || c == 'x' {
				w |= 0x8000 >> k
			}
		}
		out = append(out, w)
		t = t[at+1:]
	}
	return out
}

// runLoop runs a loop's steps: each takes count values from the ring of values (x and y stepped), and the
// command runs as many times as they hold its values; a chain gang's values each start with the index of
// its command.
func (vm *igsVM) runLoop(l *igsLoop) {
	if l.step == 0 || len(l.params) == 0 || len(l.target) == 0 || l.count <= 0 {
		return
	}
	fwd := l.from <= l.to
	step := abs(l.step)
	if !fwd {
		step = -step
	}
	ring, text := 0, 0
	keep := vm.drawMode
	for cur := l.from; (fwd && cur <= l.to) || (!fwd && cur >= l.to); cur += step {
		if vm.busy() {
			return
		}
		vm.work += 64
		rev := l.to + l.from - cur
		var all []int
		guard := 0
		for len(all) < min(l.count, 4096) && guard < 1<<16 {
			guard++
			t := l.params[ring]
			ring = (ring + 1) % len(l.params)
			switch t.kind {
			case ':':
				continue
			case 'x':
				all = append(all, cur)
			case 'y':
				all = append(all, rev)
			case '+', '-', '!':
				k := t.n
				switch t.kind {
				case '+':
					all = append(all, cur+k)
				case '-':
					all = append(all, cur-k)
				default:
					all = append(all, k-cur)
				}
			default:
				all = append(all, vm.val(t))
			}
		}
		for i := range all {
			all[i] = max(-32768, min(32767, all[i]))
		}
		if l.gang {
			for p := 0; p < len(all); {
				cmd := l.target[abs(all[p])%len(l.target)]
				rest := all[p+1:]
				n := igsWant(cmd, igsVals(rest))
				if n <= 0 || n > len(rest) {
					break
				}
				vm.exec(cmd, rest[:n], nil)
				p += 1 + n
			}
			continue
		}
		cmd := l.target[0]
		if cmd == 'W' && l.refresh && len(l.texts) > 0 && len(all) >= 2 {
			vm.exec('W', all[:2], l.texts[text%len(l.texts)])
			text++
			continue
		}
		n := igsWant(cmd, igsVals(all))
		if n <= 0 {
			continue
		}
		for p := 0; p+n <= len(all); p += n {
			vm.exec(cmd, all[p:p+n], nil)
		}
	}
	vm.drawMode = keep
}

// ---------------------------------------------------------------- VT52 on the screen

func (vm *igsVM) cellH() int {
	if vm.res == igsHigh {
		return 16
	}
	return 8
}

func (vm *igsVM) rows() int { return vm.h / vm.cellH() }
func (vm *igsVM) cols() int { return vm.w / 8 }

// putGlyph draws character c in the cell at column x, row y, in the screen's font.
func (vm *igsVM) putGlyph(c byte, x, y, fg, bg int) {
	ch := vm.cellH()
	f := fontByID(map[bool]string{true: "st8x16", false: "st8x8"}[ch == 16])
	m := uint8(vm.colours() - 1)
	for r := 0; r < ch; r++ {
		var bits byte
		if f != nil {
			if ch == 16 {
				bits = f.bits[int(c)*16+r]
			} else {
				bits = f.bits[int(c)*16+r*2]
			}
		}
		row := (y*ch + r) * vm.w
		for b := 0; b < 8; b++ {
			px := x*8 + b
			if row+px >= len(vm.pix) || px >= vm.w {
				continue
			}
			if bits&(0x80>>b) != 0 {
				vm.pix[row+px] = uint8(fg) & m
			} else {
				vm.pix[row+px] = uint8(bg) & m
			}
		}
	}
}

// scrollText moves the text rows from row y down up by n (n < 0: down), as VT52's line feed at the
// bottom, delete line and insert line do; the rows let in are the background colour.
func (vm *igsVM) scrollText(y, n int) {
	ch := vm.cellH()
	top, rows := y*ch*vm.w, vm.rows()
	end := rows * ch * vm.w
	if top >= end || n == 0 {
		return
	}
	k := min(abs(n), rows-y) * ch * vm.w
	if n > 0 {
		copy(vm.pix[top:end], vm.pix[top+k:end])
		for i := end - k; i < end; i++ {
			vm.pix[i] = uint8(vm.tbg)
		}
	} else {
		copy(vm.pix[top+k:end], vm.pix[top:end-k])
		for i := top; i < top+k; i++ {
			vm.pix[i] = uint8(vm.tbg)
		}
	}
}

func (vm *igsVM) lf() {
	vm.ty++
	if vm.ty >= vm.rows() {
		vm.ty = vm.rows() - 1
		vm.scrollText(0, 1)
	}
}

func (vm *igsVM) clearCells(x0, y0, x1, y1 int) { // cells x0..x1 of rows y0..y1, inclusive
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			vm.putGlyph(' ', x, y, vm.tbg, vm.tbg)
		}
	}
}

// vt52 prints VT52 bytes onto the screen, as TOS does (st.go's STTerm does it into cells).
func (vm *igsVM) vt52(b []byte) {
	cols, rows := vm.cols(), vm.rows()
	state, row := 0, 0
	for _, c := range b {
		if vm.busy() {
			return
		}
		vm.work += 64
		switch state {
		case 1:
			state = 0
			vm.esc(c, &state)
			continue
		case 2:
			row, state = int(c)-32, 3
			continue
		case 3:
			vm.ty, vm.tx = max(0, min(rows-1, row)), max(0, min(cols-1, int(c)-32))
			state = 0
			continue
		case 4:
			vm.tfg, state = int(c)&15, 0
			continue
		case 5:
			vm.tbg, state = int(c)&15, 0
			continue
		}
		switch c {
		case 0x1B:
			state = 1
		case '\r':
			vm.tx = 0
		case '\n', 0x0B, 0x0C:
			vm.lf()
		case 0x08:
			vm.tx = max(0, vm.tx-1)
		case 0x09:
			vm.tx = min(cols-1, (vm.tx&^7)+8)
		default:
			if c < 0x20 {
				continue
			}
			if vm.tx >= cols {
				if vm.twrap {
					vm.tx = 0
					vm.lf()
				} else {
					vm.tx = cols - 1
				}
			}
			fg, bg := vm.tfg, vm.tbg
			if vm.trev {
				fg, bg = bg, fg
			}
			vm.putGlyph(c, vm.tx, vm.ty, fg, bg)
			vm.tx++
		}
	}
}

func (vm *igsVM) esc(c byte, state *int) {
	cols, rows := vm.cols(), vm.rows()
	switch c {
	case 'A':
		vm.ty = max(0, vm.ty-1)
	case 'B':
		vm.ty = min(rows-1, vm.ty+1)
	case 'C':
		vm.tx = min(cols-1, vm.tx+1)
	case 'D':
		vm.tx = max(0, vm.tx-1)
	case 'E':
		vm.clearCells(0, 0, cols-1, rows-1)
		vm.tx, vm.ty = 0, 0
	case 'H':
		vm.tx, vm.ty = 0, 0
	case 'I':
		if vm.ty == 0 {
			vm.scrollText(0, -1)
		} else {
			vm.ty--
		}
	case 'J':
		vm.clearCells(vm.tx, vm.ty, cols-1, vm.ty)
		if vm.ty+1 < rows {
			vm.clearCells(0, vm.ty+1, cols-1, rows-1)
		}
	case 'K':
		vm.clearCells(vm.tx, vm.ty, cols-1, vm.ty)
	case 'L':
		vm.scrollText(vm.ty, -1)
		vm.tx = 0
	case 'M':
		vm.scrollText(vm.ty, 1)
		vm.tx = 0
	case 'Y':
		*state = 2
	case 'b':
		*state = 4
	case 'c':
		*state = 5
	case 'd':
		if vm.ty > 0 {
			vm.clearCells(0, 0, cols-1, vm.ty-1)
		}
		vm.clearCells(0, vm.ty, vm.tx, vm.ty)
	case 'j':
		vm.tsx, vm.tsy = vm.tx, vm.ty
	case 'k':
		vm.tx, vm.ty = vm.tsx, vm.tsy
	case 'l':
		vm.clearCells(0, vm.ty, cols-1, vm.ty)
		vm.tx = 0
	case 'o':
		vm.clearCells(0, vm.ty, vm.tx, vm.ty)
	case 'p':
		vm.trev = true
	case 'q':
		vm.trev = false
	case 'v':
		vm.twrap = true
	case 'w':
		vm.twrap = false
	case 0x1B:
		*state = 1
	}
}

// textCommand runs IG's VT52 commands on the screen's text.
func (vm *igsVM) textCommand(letter byte, a []int) {
	cols, rows := vm.cols(), vm.rows()
	switch letter {
	case 'c':
		if a[0] == 0 {
			vm.tbg = a[1] & 15
		} else {
			vm.tfg = a[1] & 15
		}
	case 'p':
		vm.tx, vm.ty = max(0, min(cols-1, a[0])), max(0, min(rows-1, a[1]))
	case 'm':
		n := max(0, min(a[1], 255))
		switch a[0] {
		case 0:
			vm.tx, vm.ty = 0, 0
		case 1:
			vm.ty = max(0, vm.ty-n)
		case 2:
			vm.ty = min(rows-1, vm.ty+n)
		case 3:
			vm.tx = min(cols-1, vm.tx+n)
		case 4:
			vm.tx = max(0, vm.tx-n)
		}
	case 'l':
		switch a[0] {
		case 0:
			vm.clearCells(0, vm.ty, cols-1, vm.ty)
			vm.tx = 0
		case 1:
			vm.clearCells(0, vm.ty, vm.tx, vm.ty)
		case 2:
			vm.clearCells(vm.tx, vm.ty, cols-1, vm.ty)
		}
	case 'd':
		vm.scrollText(vm.ty, max(0, min(a[0], rows)))
	case 'i':
		for k := 0; k < max(0, min(igsAt(a, 1), rows)); k++ {
			if igsAt(a, 0) == 0 && vm.ty > 0 {
				vm.ty--
				continue
			}
			vm.scrollText(vm.ty, -1)
		}
	case 'r':
		if a[0] == 0 {
			vm.tsx, vm.tsy = vm.tx, vm.ty
		} else {
			vm.tx, vm.ty = vm.tsx, vm.tsy
		}
	case 'v':
		vm.trev = a[0] != 0
	case 'w':
		vm.twrap = a[0] != 0
	}
}

// ---------------------------------------------------------------- the 6x6 font (VDI's small text)

var igsFont6 [256][6]uint8 // 6 pixels per row, left aligned

// loadIGSFont6 reads EmuTOS's fnt_st_6x6.c: one bitmap 192 bytes wide, 6 pixels per character.
func loadIGSFont6(src []byte) bool {
	s := bytes.Index(src, []byte("dat_table["))
	if s < 0 {
		return false
	}
	e := bytes.Index(src[s:], []byte("};"))
	if e < 0 {
		return false
	}
	var form []byte
	for _, m := range hexWord.FindAllSubmatch(src[s:s+e], -1) {
		v, _ := strconv.ParseUint(string(m[1]), 16, 16)
		form = append(form, byte(v>>8), byte(v))
	}
	if len(form) != 192*6 {
		return false
	}
	for c := 0; c < 256; c++ {
		for r := 0; r < 6; r++ {
			var v uint8
			for x := 0; x < 6; x++ {
				bit := c*6 + x
				if form[r*192+bit/8]&(0x80>>(bit%8)) != 0 {
					v |= 0x80 >> x
				}
			}
			igsFont6[c][r] = v
		}
	}
	igsFont6OK = true
	return true
}

var igsFont6OK bool

// readIGSFont6 loads the 6x6 font from its downloaded file, once.
func readIGSFont6() {
	if igsFont6OK {
		return
	}
	if b, err := os.ReadFile(stFontPath("fnt_st_6x6.c")); err == nil && sha1Is(b, stFont6SHA1) {
		loadIGSFont6(b)
	}
}
