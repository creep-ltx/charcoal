package main

// C64 BBS graphics: the Commodore boards' own codes in PETSCII screens.
//
// A C64 board sends its screens (SEQ files on its disks, "C/G" files) as they
// are, so the PETSCII colour, reverse and cursor codes are the board's colour
// codes too. Beside them each board reads its own "MCI" codes while it shows a
// file, and a few of them are slow enough to pace an animation:
//
//   Image BBS 1.2/2.0/3.0  £ + letter + one character (£C1 colour, £F1 clear,
//                          £R1 reverse, £S5 slower, £W5 wait, £V2 the caller's
//                          name ...; ££ is a £). £Sx waits x tenths of a
//                          second before each character of the line. Files are
//                          read in lines of up to 80 bytes; a line that starts
//                          with ↑ ends the file, and the last line is lost
//                          without a RETURN after it.
//   C*Base 3.x             Ctrl-P (0x10) waits until the jiffy clock's next
//                          even tick (about 1/30 s) and prints the next byte
//                          without looking for a code in it; £ + N T S V O P R L
//                          W I B and @: start codes; lines of up to 255 bytes.
//   Color 64 8.0           £ + an unshifted letter (rainbow modes, £v speed of
//                          unknown units, £c wait for a key ...): nothing to
//                          pace with, so it is not offered.
//
// What was read where (Image: Pinacolada64/ImageBBS, v1.2/source/ml-1_2-y2k.asm
// MCI table at sub_a288, dskin; v2/asm mci-commands.asm, swap3.asm read0,
// disk-io.asm dskin. C*Base 3.3.8, the source from cbasereferenceguide.github.io:
// src/ml0123.a65 bufout2, getnxt2, waiter2, mcichk, readln3. Color 64 8.0's
// manual color64.txt chapter 3.8): the knowledge note on C64 boards.
//
// Saving for one of them (Settings' My BBS or Animate, bbspause.go) writes an
// animated .seq with the board's pauses; opening a .seq with Image's codes
// reads its colours, clears and reverse as Image shows them ("imageseq").

import (
	"bytes"
	"strings"
)

const pound = 0x5C // £ in PETSCII: the lead byte of Image, C*Base and Color 64 codes

func init() {
	bbsBoards = append(bbsBoards,
		&bbsBoard{
			id: "imagebbs", name: "Image BBS (C64)", formats: " seq ", unit: 100, max: 15,
			paceFn: paceImage,
			note:   "£Sx slows the next character by x tenths of a second (£S0 back to full speed), a £ in the art as ££, lines of at most 80 bytes",
			how:    "read: Image BBS 1.2 and 2.0 source (MCI table, dskin, read0), not tried on a board",
		},
		&bbsBoard{
			id: "cbase", name: "C*Base (C64)", formats: " seq ", unit: 33, max: 1 << 20,
			paceFn: paceCBase,
			note:   "Ctrl-P waits about 1/30 s and prints the next byte as it is; a £ or @ in the art gets a reverse code after it so it starts no code",
			how:    "read: C*Base 3.3.8 source (ml0123.a65: bufout2, getnxt2, waiter2, mcichk), not tried on a board",
		})
	openAs = append(openAs, "imageseq")
	openAsNames = append(openAsNames, "C64: Image BBS £ codes")
	formats = append(formats, Format{"imageseq", "PETSCII with Image BBS £ codes (.seq)", ".seq", "c64"})
	bbsCodeName["imageseq"] = "Image BBS £ codes"
	framesFormats["imageseq"] = true
}

// petState follows what the C64's screen editor does with the bytes it is
// sent: reverse and quote mode (in which control codes print as glyphs).
type petState struct{ rev, quote bool }

func (s *petState) see(b []byte) {
	for _, c := range b {
		switch c {
		case 0x12:
			if !s.quote {
				s.rev = true
			}
		case 0x92:
			if !s.quote {
				s.rev = false
			}
		case 0x0D, 0x8D:
			s.rev, s.quote = false, false
		case 0x22:
			s.quote = !s.quote
		}
	}
}

// noop is a byte the terminal does nothing with: reverse on again, or off again.
func (s petState) noop() byte {
	if s.rev {
		return 0x12
	}
	return 0x92
}

// printable: a byte the screen shows (Image 1.2 slows only these: & 0x7F >= 0x20).
func printable(c byte) bool { return c&0x7F >= 0x20 }

// c64Piece is one piece of a board file: a code with the art bytes it carries,
// or one byte of the art. A board reads its files in lines, and a code cut by
// a line's end is read with whatever the line buffer held before: the layouts
// never cut a piece.
type c64Piece struct {
	s    []byte // the bytes in the file
	term []byte // what of them reaches the terminal (for petState)
	art  bool   // one byte of the art, free to start the next line instead
}

func artPiece(c byte) c64Piece { return c64Piece{s: []byte{c}, term: []byte{c}, art: true} }

// ---------------------------------------------------------------- Image BBS

// imageDigit is Image's one-character number: 0-9, then J-O for 10-15 (it
// takes the low four bits of the byte).
func imageDigit(n int) byte {
	if n < 10 {
		return byte('0' + n)
	}
	return byte('J' + n - 10)
}

// imageCode: a code piece of Image's ("£" + command + one character).
func imageCode(cmd byte, n int) c64Piece {
	return c64Piece{s: []byte{pound, cmd, imageDigit(n)}}
}

// imageLine: Image's dskin reads up to 80 bytes, or up to a RETURN (not kept;
// the line is printed and then a RETURN). An 80-byte line gets no RETURN.
const imageLine = 80

// paceImage writes an animated stream for Image BBS. A pause of n tenths is
// "£S" + n before the next character that shows (its print speed: n tenths
// before each character) and "£S0" after it; more than 1.5 s is spread over
// the next characters. Every £ of the art is ££.
//
// £W (whole seconds) is not used: its first second is cut short, and in the
// 2.0 source it returns at once (it compares the clock's seconds with
// themselves).
func paceImage(b *bbsBoard, stream []byte, ps []pause) []byte {
	us := b.units(ps)
	var pieces []c64Piece
	var run *c64Piece // the slowed characters: one piece, so nothing comes between them at their speed
	pending, speed, pi := 0, 0, 0
	for i, c := range stream {
		for ; pi < len(ps) && ps[pi].at <= i; pi++ {
			pending += us[pi]
		}
		esc := []byte{c}
		if c == pound {
			esc = []byte{pound, pound}
		}
		if pending == 0 || !printable(c) {
			p := artPiece(c)
			p.s = esc
			p.art = len(esc) == 1
			pieces = append(pieces, p)
			continue
		}
		if run == nil {
			run, speed = &c64Piece{}, 0
		}
		n := min(15, pending)
		pending -= n
		if n != speed {
			run.s = append(run.s, pound, 'S', imageDigit(n))
			speed = n
		}
		run.s, run.term = append(run.s, esc...), append(run.term, c)
		if next := i + 1; pending == 0 || next >= len(stream) || !printable(stream[next]) || len(run.s) > 60 {
			run.s = append(run.s, pound, 'S', '0')
			pieces = append(pieces, *run)
			run = nil
		}
	}
	return imageLayout(pieces)
}

// imageLayout puts the pieces in Image's lines of 80 bytes. Where a piece
// would not fit, the line is filled with "£B0" (no bells: nothing), moving
// one or two of the art's bytes to the next line so the fill comes out in
// whole codes; a line that would start with ↑ (which ends the file) gets
// "£B0" first; the file ends with a RETURN unless it ends a line already.
func imageLayout(pieces []c64Piece) []byte {
	var out []byte
	var st petState
	var line []c64Piece
	n := 0
	emit := func(p c64Piece) {
		out = append(out, p.s...)
		st.see(p.term)
	}
	flush := func() {
		for _, p := range line {
			emit(p)
		}
		line, n = nil, 0
	}
	var add func(p c64Piece)
	add = func(p c64Piece) {
		if n == 0 && p.s[0] == '^' {
			line, n = append(line, imageCode('B', 0)), 3
		}
		if n+len(p.s) > imageLine {
			var carry []c64Piece
			for (imageLine-n)%3 != 0 && len(carry) < 2 && len(line) > 0 && line[len(line)-1].art {
				carry = append([]c64Piece{line[len(line)-1]}, carry...)
				line, n = line[:len(line)-1], n-1
			}
			room := imageLine - n
			flush()
			for ; room >= 3; room -= 3 {
				emit(imageCode('B', 0))
			}
			for ; room > 0; room-- {
				emit(c64Piece{s: []byte{st.noop()}, term: []byte{st.noop()}})
			}
			for _, c := range carry {
				add(c)
			}
		}
		line, n = append(line, p), n+len(p.s)
		if n == imageLine {
			flush() // a full line: no RETURN after it
		}
	}
	for _, p := range pieces {
		if len(p.s) == 1 && p.s[0] == 0x0D {
			flush()
			emit(p) // a RETURN ends the line
			continue
		}
		add(p)
	}
	if n > 0 {
		flush()
		out = append(out, 0x0D) // without it Image drops the last line
	}
	return out
}

// ---------------------------------------------------------------- C*Base

// cbaseLine: C*Base reads a file in lines of up to 255 bytes, a RETURN (kept
// in the line) ending one.
const cbaseLine = 255

// paceCBase writes an animated stream for C*Base: each unit of a pause is a
// Ctrl-P and a reverse code that changes nothing (in quote mode, where that
// would print, the Ctrl-P goes before the next bytes of the art instead).
// After a Ctrl-P C*Base prints the next byte without looking for a code in it,
// so in quote mode a £ or @ of the art is written after one; elsewhere a
// reverse code after it keeps it from the next bytes.
func paceCBase(b *bbsBoard, stream []byte, ps []pause) []byte {
	us := b.units(ps)
	var pieces []c64Piece
	var st petState
	pending, pi := 0, 0
	for i, c := range stream {
		for ; pi < len(ps) && ps[pi].at <= i; pi++ {
			pending += us[pi]
		}
		if !st.quote {
			for ; pending > 0; pending-- {
				pieces = append(pieces, c64Piece{s: []byte{0x10, st.noop()}, term: []byte{st.noop()}})
			}
		}
		var p c64Piece
		switch {
		case pending > 0 || st.quote && (c == pound || c == '@'):
			p = c64Piece{s: []byte{0x10, c}, term: []byte{c}}
			pending = max(0, pending-1)
		case c == pound || c == '@':
			p = c64Piece{s: []byte{c, st.noop()}, term: []byte{c, st.noop()}}
		default:
			p = artPiece(c)
		}
		pieces = append(pieces, p)
		st.see(p.term)
	}
	return cbaseLayout(pieces)
}

// cbaseLayout puts the pieces in C*Base's lines: one that would not fit fills
// the line with reverse codes that change nothing.
func cbaseLayout(pieces []c64Piece) []byte {
	var out []byte
	var st petState
	n := 0
	for _, p := range pieces {
		if n+len(p.s) > cbaseLine {
			for ; n < cbaseLine; n++ {
				out = append(out, st.noop())
			}
			n = 0
		}
		out = append(out, p.s...)
		st.see(p.term)
		n += len(p.s)
		if p.s[len(p.s)-1] == 0x0D || n == cbaseLine {
			n = 0
		}
	}
	return out
}

// ---------------------------------------------------------------- reading Image's codes

// imageTerm reads a PETSCII stream as Image BBS shows it: £C colours, £F
// clears, £R reverse, ££ a £; its other codes (the caller's name, speed,
// waits ...) stay on the page as text, so they go back as they were.
type imageTerm struct {
	*PetTerm
	pend []byte
}

func newImageTerm(t *PetTerm) *imageTerm { return &imageTerm{PetTerm: t} }

func (t *imageTerm) write(b []byte) { t.feed(b, false) }

// flush reads a code cut by the end of the file as far as it goes.
func (t *imageTerm) flush() { t.feed(nil, true) }

func (t *imageTerm) feed(b []byte, final bool) {
	if len(t.pend) > 0 {
		b = append(append([]byte{}, t.pend...), b...)
		t.pend = nil
	}
	for i := 0; i < len(b); {
		if b[i] != pound {
			j := i + 1
			for j < len(b) && b[j] != pound {
				j++
			}
			t.PetTerm.write(b[i:j])
			i = j
			continue
		}
		n, more := imageCodeAt(b[i:], final)
		if more {
			t.pend = append(t.pend, b[i:]...)
			return
		}
		switch {
		case n == 2: // ££
			t.PetTerm.write([]byte{pound})
		case n == 3:
			p := b[i+2] & 15
			switch b[i+1] & 0x3F {
			case 3:
				t.fg = int(p)
			case 6:
				t.PetTerm.write([]byte{0x93})
			case 18:
				t.rev = p&1 != 0
			}
		default:
			t.PetTerm.write(b[i : i+1])
			n = 1
		}
		i += n
	}
}

// imageCodeAt reads an Image code at b[0] (a £) that CharEdit shows: the bytes
// it takes (2 for ££, 3 for £C, £F, £R; 0 = shown as text), or more = b ends
// before it can tell. £C0 (the caller's default colour) is shown as text.
func imageCodeAt(b []byte, final bool) (n int, more bool) {
	if len(b) < 2 {
		return 0, !final
	}
	if b[1] == pound {
		return 2, false
	}
	switch b[1] & 0x3F { // Image takes the command from the low six bits
	case 3, 6, 18:
		if len(b) < 3 {
			return 0, !final
		}
		if b[1]&0x3F == 3 && b[2]&15 == 0 {
			return 0, false
		}
		return 3, false
	}
	return 0, false
}

// isImageCodes: a .seq written with Image's codes. Art draws with £ too (the
// petscii-bbs screens hold up to 27 of them), so it counts only codes typed as
// letters (£C1, £f1 ...) with a digit or a J-O after them: three at least, two
// that change what shows, and at least half of the file's £ in codes.
func isImageCodes(b []byte) bool {
	pounds, codes, acting := 0, 0, 0
	kinds := map[byte]bool{}
	for i := 0; i < len(b); i++ {
		if b[i] != pound {
			continue
		}
		pounds++
		if i+1 < len(b) && b[i+1] == pound {
			pounds++
			codes++
			i++
			continue
		}
		if i+2 >= len(b) {
			continue
		}
		l, p := b[i+1], b[i+2]
		letter := l >= 0x41 && l <= 0x5A || l >= 0xC1 && l <= 0xDA
		param := p >= '0' && p <= '9' || p >= 0x41 && p <= 0x4F || p >= 0xC1 && p <= 0xCF
		if !letter || !param || !imageCommand(l) {
			continue
		}
		codes++
		kinds[l&0x3F] = true
		if n, _ := imageCodeAt(b[i:], true); n == 3 {
			acting++
		}
		i += 2
	}
	return acting >= 2 && codes >= 3 && len(kinds) >= 2 && codes*2 >= pounds-codes
}

// imageCommand: Image 1.2 or 2.0 takes £ + c as one of its commands (c & 63
// indexes its table of 40; the entries that only print the £ are left out).
func imageCommand(c byte) bool {
	i := c & 0x3F
	return i >= 1 && i <= 25 || i == 31 || i == 33 || i >= 35 && i <= 37
}

// imageEscape writes a PETSCII stream for a page read with Image's codes: a £
// that Image (and imageTerm) would take as one of the codes shown, or as ££,
// is written ££. Other codes, kept as text, go back as they came.
func imageEscape(stream []byte) []byte {
	if bytes.IndexByte(stream, pound) < 0 {
		return stream
	}
	var out []byte
	for i, c := range stream {
		out = append(out, c)
		if c != pound {
			continue
		}
		if i+1 >= len(stream) {
			continue
		}
		if n, _ := imageCodeAt(stream[i:], true); n > 0 {
			out = append(out, pound)
		}
	}
	return out
}

// encodeImageSeq writes the page for "imageseq": the .seq writer's stream
// with the £ that would start a code doubled. An animated page (a draw order,
// frames) is its .seq animation, with board's pauses; Image's pauses write
// every £ as ££ already, and another C64 board's file is in that board's codes.
func (a *App) encodeImageSeq(fmtID, board string, ps *[]pause, d *Doc) ([]byte, bool) {
	if fmtID != "imageseq" {
		return nil, false
	}
	if a.orderedSave("seq") || a.nFrames() > 1 {
		out, _ := a.encodeFor("seq", board, ps)
		if b := boardByID(board); b == nil || !b.shows("seq") {
			out = imageEscape(out)
		}
		return out, true
	}
	if lim := a.mach().cols; d.cols > lim {
		d = d.withCols(lim)
	}
	return imageEscape(writeSeq(d, a.lower(), a.info.clear)), true
}

// boardExts are the endings of the files a board's codes go in: ".ans", ".seq".
func boardExts(b *bbsBoard) string {
	var exts []string
	for _, f := range strings.Fields(b.formats) {
		exts = append(exts, streamExt[f])
	}
	return strings.Join(exts, " ")
}

// boardsNamed lists the boards that show fmtID, for the help.
func boardsNamed(fmtID string) string {
	var names []string
	for _, b := range boardsFor(fmtID) {
		names = append(names, b.name)
	}
	return strings.Join(names, ", ")
}
