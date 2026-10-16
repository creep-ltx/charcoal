package main

// Animated files in a board's colour codes (bbscodes.go kinds): a page read
// from a file in pipe codes, Ctrl-A, WWIV, Wildcat! or Celerity codes and given
// a draw order (Animate, Animate a folder) or a frame strip with a board's
// pauses is saved as its animated ANSI stream whose colours are written in the
// file's own codes (the board turns them into ANSI for the caller), the art's
// characters that would start a code escaped as writeBBSCodes escapes them,
// and the board's pauses put in as for an .ans (bbsBoard.paceIn).

import (
	"bytes"
	"fmt"
	"slices"
	"strings"
)

// codesEscape: how the art's byte stream[i] is written in a file of kind's
// codes ("" as it is). cc is the colours written as codes (nil: the stream's
// colours stay ANSI), track follows the colours either way.
func codesEscape(kind string, stream []byte, i int, cc, track *sgrColours) string {
	c := stream[i]
	n1, n2 := -1, -1
	if i+1 < len(stream) {
		n1 = int(stream[i+1])
	}
	if i+2 < len(stream) {
		n2 = int(stream[i+2])
	}
	// again: the colours written once more right after the character, so it
	// cannot make a code with what follows.
	again := func() string {
		if n1 < 0 {
			return string(c)
		}
		if cc != nil {
			a := cc.attr
			if a < 0 {
				a = 0x07
			}
			return string(appendCodes(kind, []byte{c}, -1, a, false))
		}
		return string(c) + string(sgrFull(track))
	}
	switch kind {
	case "pipe":
		// Mystic takes the two bytes after any "|": a "|" that could make a
		// code with them, or meets a code or an escape within them, is |PI.
		if c == '|' && (n1 < 0 || n2 < 0 || n1 == 0x1B || n2 == 0x1B || n1 == '|' || n2 == '|' || pipeActs(byte(n1), byte(n2))) {
			return "|PI"
		}
	case "ctrla":
		switch c {
		case 0x01:
			return "\x01A"
		case 0x1A:
			return "\x01z"
		}
	case "wwiv":
		if c == 0x03 {
			return " " // a heart and a character are always a code there
		}
		if c == '|' && n1 >= 0 && (isDigit(byte(n1)) || n1 == '#') {
			return again()
		}
	case "wildcat":
		if c == '@' && n2 >= 0 && (isUpperHex(byte(n1)) && isUpperHex(byte(n2)) || n1 == 'C' && n2 == 'L') {
			return again()
		}
	case "celerity":
		if c == '|' && n1 >= 0 && (n1 == 'S' || strings.IndexByte(celerityLetters, byte(n1)) >= 0) {
			return again()
		}
	case "telegard":
		if c == '`' && n2 >= 0 && isUpperHex(byte(n1)) && isUpperHex(byte(n2)) {
			return again()
		}
	}
	if kind == "renegade" && (c == '^' && n1 >= 0 && isDigit(byte(n1)) || c == '|' && n2 >= 0 && isDigit(byte(n1)) && isDigit(byte(n2))) {
		return again() // a user colour, or two digits (always a code on Renegade)
	}
	return ""
}

// sgrFull is an SGR that sets all of the colours c follows.
func sgrFull(c *sgrColours) []byte {
	s := "\x1b[0;"
	if c.bold {
		s += "1;"
	}
	if c.blink {
		s += "5;"
	}
	return fmt.Appendf(nil, "%s%d;%dm", s, 30+c.fg, 40+c.bg)
}

// animatedCodes: saving the page in kind's codes writes an animation: a draw
// order, or frames with a board's pauses (frames without a board are written
// one picture after another, each after the board's clear, as before).
func (a *App) animatedCodes(kind, board string) bool {
	if !isBBSCodes(kind) {
		return false
	}
	if a.orderedSave("ansi") {
		return true
	}
	b := boardByID(board)
	return a.nFrames() > 1 && b != nil && b.shows("ansi")
}

// animFormat is the format an animated save of a page whose stream format is
// f writes: the file's BBS colour codes when it is in them, else f.
func (a *App) animFormat(f string) string {
	if f == "ansi" && isBBSCodes(a.saveFmt) {
		return a.saveFmt
	}
	return f
}

// encodeCodesAnimated writes the page's animation in kind's codes with
// board's pauses (none: "" or a board without codes for .ans files).
func (a *App) encodeCodesAnimated(kind, board string, ps *[]pause) []byte {
	var stream []byte
	var p []pause
	if a.nFrames() > 1 {
		stream, p = a.writeStreamFramesPaced("ansi", a.allDocs(), false)
	} else {
		stream, p = a.writeOrderedPaced("ansi", a.doc, false)
	}
	b := boardByID(board)
	if b == nil || !b.shows("ansi") {
		return (&bbsBoard{unit: 1}).paceIn(kind, stream, nil)
	}
	if ps != nil {
		*ps = b.waited(p)
	}
	return b.paceIn(kind, stream, p)
}

// ---------------------------------------------------------------- Telegard, theme and user colours

// Telegard 3 colours text with a backtick and the attribute as two capital
// hex digits ("`0E" yellow on black, "`1F" white on blue; its CUSTOM.DOC: "color
// codes - `xx or |xx"; every one of the 2,500 codes in the sample screens of
// the tgarts pack is like that). Its ~XX codes (the caller's data) stay text.
//
// Theme and user colours: Mystic's |T0-|T9 set the colour to the theme's
// colour n and Renegade's ^0-^9 to the caller's colour scheme's (any sysop or
// caller can change them). CharEdit shows them in the boards' defaults and
// writes the code again for the cells the file drew in it, as long as they
// keep that colour (themeMarks); every other cell keeps explicit codes. A ^ and
// a digit is text to Mystic, so Renegade's codes are only read with Open as
// "PC: Renegade pipe codes" (kind "renegade": no |CL, |PI or |T; two digits
// after a | are always a code; |24-|99 show nothing, so no bit 7).

func init() {
	bbsCodeIDs = append(bbsCodeIDs, "telegard")
	bbsCodeName["telegard"] = "Telegard `XY codes"
	at := slices.IndexFunc(formats, func(f Format) bool { return f.id == "celerity" })
	formats = slices.Insert(formats, at+1, Format{"telegard", "Telegard `XY codes (.msg)", ".msg", "pc"})
	openAs = append(openAs, "telegard")
	openAsNames = append(openAsNames, "PC: Telegard `XY")
	bbsCodeIDs = append(bbsCodeIDs, "renegade")
	bbsCodeName["renegade"] = "Renegade pipe codes"
	at = slices.IndexFunc(formats, func(f Format) bool { return f.id == "pipe" })
	formats = slices.Insert(formats, at+1, Format{"renegade", "Renegade pipe codes |07 ^3 (.ans)", ".ans", "pc"})
	openAs = append(openAs, "renegade")
	openAsNames = append(openAsNames, "PC: Renegade pipe codes")
}

// mysticThemeColours: a new theme's colours in Mystic (1.10's theme editor,
// bbs_cfg_theme.pas; |T0-|T9 send Attr2Ansi(Theme.Colors[n]), bbs_io.pas).
var mysticThemeColours = [10]int{0x01, 0x09, 0x0B, 0x08, 0x07, 0x0F, 0x18, 0x17, 0x19, 0x1F}

// renegadeUserColours: Renegade's "Default Color Scheme" (BOOT.PAS; ^n is
// UserColor(n) = Scheme.Color[n+1], COMMON.PAS).
var renegadeUserColours = [10]int{0x0F, 0x07, 0x0D, 0x0B, 0x09, 0x0E, 0x1F, 0x0C, 0x8E, 0x0A}

// renegade reads Renegade's codes (COMMON.PAS): | + two digits (00-15 the
// foreground, 16-23 the background, 24-99 nothing) and ^0-^9 (0: shown as text).
func (c *codeTerm) renegade(b []byte) int {
	if len(b) < 2 || b[0] == '^' && !isDigit(b[1]) {
		return 0
	}
	if b[0] == '^' {
		c.theme = ""
		c.setTheme(string(b[:2]), renegadeUserColours[b[1]-'0'])
		return 2
	}
	if len(b) < 3 || !isDigit(b[1]) || !isDigit(b[2]) {
		return 0
	}
	switch v := int(b[1]-'0')*10 + int(b[2]-'0'); {
	case v < 16:
		c.theme = ""
		c.setFg(v)
	case v < 24:
		c.theme = ""
		c.setBg(v-16, false)
	}
	return 3
}

// setTheme sets the colours of a theme or user colour code.
func (c *codeTerm) setTheme(code string, attr int) {
	c.setAttr(attr)
	c.theme, c.themeAttr = code, attr
}

// mark notes, before the terminal draws byte b, that the cell it goes in is
// drawn in a theme code's colours (while no other colour came since).
func (c *codeTerm) mark(b byte) {
	if c.theme == "" {
		return
	}
	if c.bios() != c.themeAttr || c.state != 0 {
		if c.bios() != c.themeAttr {
			c.theme = "" // an ANSI colour since
		}
		return
	}
	if c.marks == nil || b < 0x20 && !c.glyphCtl {
		return
	}
	x, y := c.x, c.y
	if x >= c.doc.cols {
		x, y = 0, y+1
	}
	c.marks[[2]int{x, y}] = c.theme
}

// themeColour is the colours a theme or user code shows (boards' defaults).
func themeColour(code string) int {
	switch {
	case len(code) == 3 && code[:2] == "|T" && isDigit(code[2]):
		return mysticThemeColours[code[2]-'0']
	case len(code) == 2 && code[0] == '^' && isDigit(code[1]):
		return renegadeUserColours[code[1]-'0']
	}
	return -1
}

// telegard reads "`" + two capital hex digits: the attribute (0: shown as text).
func (c *codeTerm) telegard(b []byte) int {
	if len(b) < 3 || !isUpperHex(b[1]) || !isUpperHex(b[2]) {
		return 0
	}
	c.setAttr(upperHexVal(b[1])<<4 | upperHexVal(b[2]))
	return 3
}

// telegardCodes counts a file's Telegard codes when it is written in them (0:
// it is not): three at least, two different and repeated, and at least half
// of its backticks (text seldom has one before two hex digits).
func telegardCodes(b []byte) int {
	n, ticks := 0, 0
	kinds := map[string]bool{}
	for i := 0; i < len(b); i++ {
		if b[i] != '`' {
			continue
		}
		ticks++
		if i+2 < len(b) && isUpperHex(b[i+1]) && isUpperHex(b[i+2]) {
			n++
			kinds[string(b[i+1:i+3])] = true
			i += 2
		}
	}
	if n >= 3 && len(kinds) >= 2 && n > len(kinds)+1 && n*2 >= ticks {
		return n
	}
	return 0
}

// themeMarks: the cells a file in kind's codes draws in a theme or user
// colour code's colours -> the code (nil: none), read as the page is read.
func themeMarks(src []byte, kind string, cols int) map[[2]int]string {
	if kind != "pipe" && kind != "renegade" || cols <= 0 {
		return nil
	}
	if !bytes.Contains(src, []byte("|T")) && !bytes.Contains(src, []byte("^")) {
		return nil
	}
	t := newTerm(newDoc(cols, 25), false)
	ct := newCodeTerm(t, kind)
	ct.marks = map[[2]int]string{}
	ct.write(src[:dataEnd(src)])
	ct.flush()
	if len(ct.marks) == 0 {
		return nil
	}
	return ct.marks
}

// bbsThemes: for a page read from a file in pipe or Renegade codes, the cells
// it drew in theme or user colours (nil: none), so a save writes those codes
// again for them (only where the cell still has that colour).
func (a *App) bbsThemes(kind string) map[[2]int]string {
	if a.codes != kind {
		return nil
	}
	return themeMarks(a.source, kind, a.doc.cols)
}

// bbsThemeNote tells, when a file is opened, that its theme or user colours
// are shown in the boards' defaults ("" none).
func bbsThemeNote(kind string, src []byte) string {
	if themeMarks(src, kind, 80) == nil {
		return ""
	}
	return "; |T0-|T9 and ^0-^9 shown in Mystic's and Renegade's default colours, written back as codes"
}
