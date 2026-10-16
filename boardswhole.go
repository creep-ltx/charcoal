package main

// Boards that wait whole seconds only: FAME (Amiga) "~Wn", CNet (Amiga)
// Ctrl-Q "Wn}", and Image BBS 1.2 (C64) "£Wn". None can wait less than a
// second, so an animation for them is coarse: each frame's hold is rounded to
// the nearest second (at least one, where a hold is wanted), and a draw order's
// even pauses (every n cells) are gathered, the rounding carried as on the
// other boards, so the whole file waits about as long as asked. They are
// offered marked "(whole seconds)" in Save for board X, Animate and Animate a
// folder, and the play time shown is the seconds they really wait (waited).
//
// What each one does, and where it was read:
//
//   FAME 1.23 (FAME.guide, "MCI-Commands"; closed source): a text file is read
//     for codes only when its very first byte is "~". "~Wx" waits x seconds,
//     at most 10; a code ends at a space or "|" (the terminator); "~~" prints
//     one "~"; "~0" resets the colours (CharEdit's first code: it starts the
//     file with the "~" FAME needs and changes nothing, every stream sets its
//     colours itself). Letters may be either case.
//   CNet 3 (systext/help/MCI of CNet 3.05c, Aminet comm/cnet): codes are
//     Ctrl-Q (0x11) + letter + argument + "}" ("{W5}" as its editor shows it),
//     or Ctrl-Y (0x19) + letter + one character. "{Wn}" waits n seconds, "{@6}"
//     turns off word wrap and the More? prompt for the file. There is no escape
//     for a Ctrl-Q or Ctrl-Y in the art (the PC's ◄ and ↓): written as spaces.
//   Image BBS 1.2 (ml-1_2-y2k.asm): "£Wx" waits x seconds, 1-15 (x as Image's
//     one character, J-O for 10-15), but counts changes of the clock's seconds,
//     so the first one is short: x-1 to x seconds. Image 2.0's "£W" returns at
//     once (it compares the seconds with themselves), so this board is 1.2's.
//     Lines as for £S (imageLayout in c64bbs.go).

import "strconv"

func init() {
	bbsBoards = append(bbsBoards,
		&bbsBoard{
			id: "fame", name: "FAME (whole seconds)", formats: " ansi ", unit: 1000, max: 10, whole: true,
			head:  "~0|",
			delay: func(n int) string { return "~W" + strconv.Itoa(n) + "|" },
			escape: func(b byte) string {
				if b == '~' {
					return "~~"
				}
				return ""
			},
			note: "~0| first, ~Wn| waits n whole seconds (up to 10), a ~ in the art as ~~ (from its guide, not tried)",
			how:  "read: FAME's guide (FAME.guide, MCI-Commands), not tried on a board",
		},
		&bbsBoard{
			id: "cnet", name: "CNet (whole seconds)", formats: " ansi ", unit: 1000, max: 99, whole: true,
			head:  "\x11@6}",
			delay: func(n int) string { return "\x11W" + strconv.Itoa(n) + "}" },
			escape: func(b byte) string {
				if b == 0x11 || b == 0x19 {
					return " " // they start CNet's codes, and nothing writes them as art
				}
				return ""
			},
			note: "Ctrl-Q @6} first (no word wrap, no More?), Ctrl-Q Wn} waits n whole seconds; ◄ and ↓ (Ctrl-Q, Ctrl-Y) in the art become spaces",
			how:  "read: CNet 3.05c's MCI help (systext/help/MCI), not tried on a board",
		},
		&bbsBoard{
			id: "imagebbsw", name: "Image BBS 1.2 (C64, whole seconds)", formats: " seq ", unit: 1000, max: 15, whole: true,
			paceFn: paceImageW,
			note:   "£Wx waits x whole seconds (1-15; the first second is short, and Image 2.0 does not wait at all), a £ in the art as ££",
			how:    "read: Image BBS 1.2 source (ml-1_2-y2k.asm: the MCI table, £W, dskin), not tried on a board",
		})
}

// wholeUnits: each hold rounded to the nearest unit, at least one where a hold
// is wanted; a draw order's even pauses carry their rounding to the next, so
// many short ones become a few whole seconds.
func (b *bbsBoard) wholeUnits(ps []pause) []int {
	out := make([]int, len(ps))
	total, given := 0, 0
	for i, p := range ps {
		if p.ms <= 0 {
			continue
		}
		if !p.spread {
			out[i] = max(1, (p.ms+b.unit/2)/b.unit)
			continue
		}
		total += p.ms
		want := (total + b.unit/2) / b.unit
		out[i] = want - given
		given = want
	}
	return out
}

// waited are the pauses as board b waits them: the same on boards with short
// units (their rounding is carried), whole seconds on the others. A nil board
// changes nothing.
func (b *bbsBoard) waited(ps []pause) []pause {
	if b == nil || !b.whole || ps == nil {
		return ps
	}
	us := b.units(ps)
	out := make([]pause, len(ps))
	for i, p := range ps {
		out[i] = pause{at: p.at, ms: us[i] * b.unit, spread: p.spread}
	}
	return out
}

// paceImageW writes an animated stream for Image BBS 1.2 with "£W" codes at
// the pauses (at most 15 seconds each, more codes for longer); every £ of the
// art is ££, the lines as imageLayout makes them.
func paceImageW(b *bbsBoard, stream []byte, ps []pause) []byte {
	us := b.units(ps)
	var pieces []c64Piece
	pi := 0
	flush := func(at int) {
		for ; pi < len(ps) && ps[pi].at <= at; pi++ {
			for n := us[pi]; n > 0; n -= b.max {
				pieces = append(pieces, imageCode('W', min(n, b.max)))
			}
		}
	}
	for i, c := range stream {
		flush(i)
		p := artPiece(c)
		if c == pound {
			p.s, p.art = []byte{pound, pound}, false
		}
		pieces = append(pieces, p)
	}
	flush(len(stream))
	return imageLayout(pieces)
}
