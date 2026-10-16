package main

// A board's own colour codes in an animated file saved for it (bbspause.go):
// Mystic's pipe codes, Synchronet's Ctrl-A and PCBoard's @X take the place of
// the stream's ANSI colours (SGR), so the file is the board's own kind of
// screen, a sysop can change its colours in the board's editor, and it is
// smaller (|14 for ESC[1;33m). The board turns its codes into ANSI for the
// caller, so the caller sees the same colours.
//
// Only a stream whose colours all have a code is changed: one SGR the board
// has no code for (italic and underline on Amiga pages; blink for Mystic 1.10,
// which eats |24-|31) leaves every colour of the file in ANSI, as before, so
// the board's idea of the current colour never differs from the terminal's.

import "fmt"

// sgrColours turns the SGR sequences of an ANSI stream into a board's codes.
type sgrColours struct {
	kind  string // "pipe", "ctrla" (bbscodes.go) or "pcb" (PCBoard's @X)
	fg    int    // the pen as SGR sets it: colours in ANSI order
	bg    int
	bold  bool
	blink bool
	attr  int // the attribute byte the board was last given (BIOS order; -1 none)
}

// boardColourer is the converter for the stream, nil when the board has no
// colour codes or one of the stream's SGRs has none on the board.
func boardColourer(kind string, stream []byte) *sgrColours {
	if kind == "" {
		return nil
	}
	c := &sgrColours{kind: kind, fg: 7, attr: -1}
	sim := *c // PCBoard: @X00 saves the colour and @XFF restores it, so those two have no code: ANSI stays
	for i := 0; i < len(stream); i++ {
		if n, ps, ok := sgrAt(stream, i); n > 0 {
			if !ok {
				return nil
			}
			for _, p := range ps {
				if !c.knows(p) {
					return nil
				}
			}
			if sim.code(stream, i); kind == "pcb" && (sim.attr == 0x00 || sim.attr == 0xFF) {
				return nil
			}
			i += n - 1
		}
	}
	return c
}

// sgrAt reads an SGR (ESC [ params m) at stream[i]: its length (0: none there),
// its parameters (an empty one is 0) and whether they are all numbers.
func sgrAt(stream []byte, i int) (n int, ps []int, ok bool) {
	if i+1 >= len(stream) || stream[i] != 0x1B || stream[i+1] != '[' {
		return 0, nil, false
	}
	v, have := 0, false
	for j := i + 2; j < len(stream) && j < i+64; j++ {
		switch c := stream[j]; {
		case c >= '0' && c <= '9':
			v, have = v*10+int(c-'0'), true
			if v > 999 {
				return 0, nil, false
			}
		case c == ';':
			ps, v, have = append(ps, v), 0, false
		case c == 'm':
			if have || len(ps) == 0 || stream[j-1] == ';' {
				ps = append(ps, v)
			}
			return j + 1 - i, ps, true
		default:
			return 0, nil, false // another sequence (a cursor move ...): not a colour
		}
	}
	return 0, nil, false
}

// knows: the board has a code for SGR parameter p.
func (c *sgrColours) knows(p int) bool {
	switch {
	case p == 0, p == 1, p == 22, p >= 30 && p <= 37, p == 39, p >= 40 && p <= 47, p == 49:
		return true
	case p == 5, p == 25:
		return c.kind != "pipe" && c.kind != "celerity" // Mystic 1.10 eats |24-|31, Celerity has none: a blinking file keeps ANSI
	}
	return false
}

// code turns the SGR at stream[i] into the board's codes: the bytes it takes
// (0: no SGR there) and the codes ("" when the colours do not change).
func (c *sgrColours) code(stream []byte, i int) (n int, out []byte) {
	n, ps, ok := sgrAt(stream, i)
	if n == 0 || !ok {
		return 0, nil
	}
	for _, p := range ps {
		switch {
		case p == 0:
			c.fg, c.bg, c.bold, c.blink = 7, 0, false, false
		case p == 1:
			c.bold = true
		case p == 22:
			c.bold = false
		case p == 5:
			c.blink = true
		case p == 25:
			c.blink = false
		case p >= 30 && p <= 37:
			c.fg = p - 30
		case p == 39:
			c.fg = 7
		case p >= 40 && p <= 47:
			c.bg = p - 40
		case p == 49:
			c.bg = 0
		}
	}
	fg := c.fg
	if c.bold {
		fg += 8
	}
	a := biosANSI[fg] | biosANSI[c.bg]<<4
	if c.blink {
		a |= 0x80
	}
	if a == c.attr {
		return n, nil
	}
	if c.kind == "pcb" {
		out = fmt.Appendf(nil, "@X%02X", a)
	} else {
		out = appendCodes(c.kind, nil, c.attr, a, false)
	}
	c.attr = a
	return n, out
}
