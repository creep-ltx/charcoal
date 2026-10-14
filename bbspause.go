package main

// BBS pauses: an animated stream (a draw order, a frame effect's delta stream
// or the frame strip saved one frame after another) is paced only by the
// connection: over a modem at 2400 bps it plays, over telnet with no speed
// limit it arrives at once and the effect flashes past. Many BBSes have a code
// of their own, read while they show a screen file, that waits a while. The
// writers here put that board's codes into the saved file at the frames' ends
// (or every N cells of a draw order), so the board shows it at the chosen pace.
// The codes are taken out by the board, so the terminal sees exactly the plain
// stream's bytes (the tests run each file through a model of its board).
//
// A file with one board's codes shows them as junk text anywhere else (another
// board, a viewer, 16colo.rs), so the default is none: only the board the
// person chose (Settings' "My BBS", or the page's own choice in Animate).

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

// pause: wait ms after the stream's first at bytes.
type pause struct{ at, ms int }

// bbsBoard is one BBS's way of pausing a displayed file.
type bbsBoard struct {
	id, name string
	formats  string // the stream formats it shows: " ansi seq "
	unit     int    // ms of one unit of its delay code
	max      int    // the most units one code may hold
	head     string // written first (codes that must start the file)
	delay    func(n int) string
	escape   func(b byte) string          // a byte of the art as the board needs it written ("" = as it is)
	lfOnly   bool                         // a CR LF is written as LF (the board sends CR LF for it)
	layout   func(pieces []string) []byte // the board reads the file in pieces: no code may be cut (nil: joined)
	note     string                       // what the code is, for the help and the requester
	how      string                       // how this was established
}

func (b *bbsBoard) shows(fmtID string) bool { return strings.Contains(b.formats, " "+fmtID+" ") }

// bbsBoards: the boards whose codes are known (bbspause_test.go models each).
var bbsBoards = []*bbsBoard{
	{
		// Ami-Express 5 (express.e d5beb95): displayFile reads the file with
		// Fgets(fh, buf, 999) and runs processMci on each piece only when the
		// file's first byte is "~". "~<n>w|" is Delay(n) (ticks of 1/50 s, n up
		// to 3 digits), "~NS|" turns off the More? prompt, "~~|" prints "~".
		// A LF (or CR LF) ending a piece is sent as CR LF.
		id: "amiexpress", name: "Ami-Express (/X)", formats: " ansi ", unit: 20, max: 999,
		head:  "~NS|",
		delay: func(n int) string { return "~" + strconv.Itoa(n) + "w|" },
		escape: func(b byte) string {
			if b == '~' {
				return "~~|"
			}
			return ""
		},
		lfOnly: true, layout: layoutAE,
		note: "~NS| first, ~<n>w| waits n/50 s, a ~ in the art as ~~|",
		how:  "verified: read in Ami-Express 5's source (express.e: displayFile, processMciCmd)",
	},
	{
		// Mystic (bbs_io.pas OutFile, 1.10a30; the wiki's display codes for
		// 1.12): "|DE" flushes and waits 500 ms, "|PO" turns off the More?
		// prompt for the file, "|PI" prints "|"; a 0x1A ends the file.
		id: "mystic", name: "Mystic", formats: " ansi ", unit: 500, max: 1,
		head:  "|PO",
		delay: func(n int) string { return strings.Repeat("|DE", n) },
		escape: func(b byte) string {
			if b == '|' {
				return "|PI"
			}
			return ""
		},
		note: "|PO first, |DE waits half a second (the shortest it has), a | in the art as |PI",
		how:  "verified: read in Mystic 1.10's source (bbs_io.pas OutFile, ParseMCI) and its wiki (display codes)",
	},
	{
		// PCBoard 15.3 (source v0.014: display/xlate.c DELAY, pcbmacro.c
		// findtoken, display/files.c and dosfgets): "@DELAY:n@" waits n
		// tenths of a second (an unsigned char: up to 255; DOS timer ticks, so
		// in steps of about 55 ms), "@POFF@" turns off the More? prompt. The
		// file is read in lines of up to 2047 bytes ending at a CR (layoutPCB).
		// No escape exists: "@X" with two hex digits in the art is a colour
		// there anyway, and a 0x1A ends the file.
		id: "pcboard", name: "PCBoard", formats: " ansi ", unit: 100, max: 255,
		head:   "@POFF@",
		delay:  func(n int) string { return "@DELAY:" + strconv.Itoa(n) + "@" },
		layout: layoutPCB,
		note:   "@POFF@ first, @DELAY:n@ waits n/10 s (up to 255)",
		how:    "verified: read in PCBoard 15.3's source (display/xlate.c, pcbmacro.c, files.c, dosfgets.c)",
	},
	{
		// Synchronet (src/sbbs3: putmsg.cpp, con_out.cpp ctrl_a): Ctrl-A ","
		// waits 100 ms, ";" 500 ms, "." 2 s. Ctrl-A "A" prints a Ctrl-A,
		// Ctrl-A "z" a Ctrl-Z (a raw one ends a file with SAUCE), Ctrl-A "Q"
		// sets the More? line count to 0 (after every LF, and after an "@" or
		// "|" so the art's text can never be taken for an @-code or a pipe colour).
		id: "synchronet", name: "Synchronet", formats: " ansi ", unit: 100, max: 1 << 20,
		delay: func(n int) string {
			return strings.Repeat("\x01.", n/20) + strings.Repeat("\x01;", n%20/5) + strings.Repeat("\x01,", n%5)
		},
		escape: func(b byte) string {
			switch b {
			case 0x01:
				return "\x01A"
			case 0x1A:
				return "\x01z"
			case '@', '|', '\n':
				return string([]byte{b}) + "\x01Q"
			}
			return ""
		},
		note: "Ctrl-A , waits 0.1 s, Ctrl-A ; 0.5 s, Ctrl-A . 2 s; Ctrl-A Q after each line, @ and |",
		how:  "verified: read in Synchronet's source (src/sbbs3 putmsg.cpp, con_out.cpp ctrl_a, atcodes.cpp)",
	},
	{
		// Tempest (TDocs-Tempest, chapter V): "~DELAY(n)" pauses n ticks
		// (50 a second), "~~" prints "~". Read in its documentation only.
		id: "tempest", name: "Tempest", formats: " ansi ", unit: 20, max: 500,
		delay: func(n int) string { return "~DELAY(" + strconv.Itoa(n) + ")" },
		escape: func(b byte) string {
			if b == '~' {
				return "~~"
			}
			return ""
		},
		note: "~DELAY(n) waits n/50 s, a ~ in the art as ~~ (from its manual, not tried)",
		how:  "read: Tempest's manual (TDocs-Tempest, chapter V, Special Control Codes)",
	},
}

func boardByID(id string) *bbsBoard {
	for _, b := range bbsBoards {
		if b.id == id {
			return b
		}
	}
	return nil
}

// boardsFor are the boards that show a stream of format fmtID.
func boardsFor(fmtID string) []*bbsBoard {
	var out []*bbsBoard
	for _, b := range bbsBoards {
		if b.shows(fmtID) {
			out = append(out, b)
		}
	}
	return out
}

// units turns the pauses' ms into the board's units, carrying what rounding
// leaves over to the next pause, so the whole file waits as long as asked.
func (b *bbsBoard) units(ps []pause) []int {
	out := make([]int, len(ps))
	total, given := 0, 0
	for i, p := range ps {
		total += max(0, p.ms)
		want := (total + b.unit/2) / b.unit
		out[i] = want - given
		given = want
	}
	return out
}

// pace writes stream with the board's delay codes at the pauses (in order of at).
func (b *bbsBoard) pace(stream []byte, ps []pause) []byte {
	us := b.units(ps)
	var pieces []string // codes and the art's bytes (escaped): a board must never cut one in two
	if b.head != "" {
		pieces = append(pieces, b.head)
	}
	pi := 0
	flush := func(at int) {
		for ; pi < len(ps) && ps[pi].at <= at; pi++ {
			for n := us[pi]; n > 0; n -= b.max {
				pieces = append(pieces, b.delay(min(n, b.max)))
			}
		}
	}
	for i := 0; i < len(stream); i++ {
		flush(i)
		c := stream[i]
		if b.lfOnly && c == '\r' && i+1 < len(stream) && stream[i+1] == '\n' {
			continue // the board sends CR LF for a LF anyway
		}
		if e := ""; b.escape != nil {
			if e = b.escape(c); e != "" {
				pieces = append(pieces, e)
				continue
			}
		}
		pieces = append(pieces, string([]byte{c}))
	}
	flush(len(stream))
	if b.layout != nil {
		return b.layout(pieces)
	}
	return []byte(strings.Join(pieces, ""))
}

// aeChunk: Ami-Express reads a screen with Fgets(fh, buf, 999): pieces of up
// to 998 bytes, a LF ending one.
const aeChunk = 998

// layoutAE: a code cut in two by Ami-Express's pieces would be printed. Where
// one would not fit, the rest of the piece is filled with empty codes ("~|"
// prints nothing, a "~" as the very last byte of a piece neither), so it
// starts the next piece.
func layoutAE(pieces []string) []byte {
	var out []byte
	room := aeChunk
	for _, p := range pieces {
		if len(p) > room {
			for ; room >= 2; room -= 2 {
				out = append(out, '~', '|')
			}
			if room == 1 {
				out = append(out, '~')
			}
			room = aeChunk
		}
		out = append(out, p...)
		room -= len(p)
		if p[len(p)-1] == '\n' || room == 0 {
			room = aeChunk
		}
	}
	return out
}

// pcbLine: PCBoard reads a display file with dosfgets into 2049 bytes: lines
// of up to 2047 bytes, a CR (and the LF after it) ending one.
const pcbLine = 2047

// layoutPCB: a code cut in two by PCBoard's lines would be printed, so before
// a line runs out a zero delay ("@DELAY:000@", as long as needed, at least 9
// bytes) fills it to the end: the next line starts on a clean piece. Never
// before a CR (a CR just past a full line makes an empty line more), a LF
// (dosfgets drops a LF that starts a line) or a % ! $ (a line
// starting with one of those may name a file to show or run).
func layoutPCB(pieces []string) []byte {
	var out []byte
	room := pcbLine
	afterCR := false
	for _, p := range pieces {
		if afterCR && p == "\n" { // part of the line's end, read with the CR
			out = append(out, p...)
			afterCR = false
			continue
		}
		afterCR = false
		if room >= 9 && room <= 24 && !strings.ContainsRune("\r\n%!$", rune(p[0])) {
			out = append(out, "@DELAY:"+strings.Repeat("0", room-8)+"@"...)
			room = pcbLine
		}
		out = append(out, p...)
		room -= len(p)
		if p == "\r" {
			room, afterCR = pcbLine, true
		} else if room <= 0 {
			room += pcbLine
		}
	}
	return out
}

// ---------------------------------------------------------------- the page's choice

// pageBoard is the board whose codes an animated save of this page gets ("" none):
// the page's own choice in Animate, else Settings' My BBS.
func (a *App) pageBoard() string {
	if a.order != nil && a.order.BBS != "" {
		if a.order.BBS == "none" {
			return ""
		}
		return a.order.BBS
	}
	return prefs.MyBBS
}

// animatedSave: saving the page as fmtID writes an animation (paced by pauses).
func (a *App) animatedSave(fmtID string) bool {
	return streamFormats[fmtID] && (a.orderedSave(fmtID) || a.nFrames() > 1)
}

// boardName is how a board is called in the interface ("" and unknown: none).
func boardName(id string) string {
	if b := boardByID(id); b != nil {
		return b.name
	}
	return "none"
}

// orderPacing: a draw order's pause (ms) every n cells, with the defaults.
func (o *drawOrder) pacing() (every, ms int) {
	every, ms = o.Every, o.Pause
	if every <= 0 {
		every = 40
	}
	if ms <= 0 {
		ms = 40
	}
	return
}

// effectHold is how long a frame effect's frames are held in a paced file.
func (o *drawOrder) effectHold() int {
	if o.Hold > 0 {
		return o.Hold
	}
	return effectDefaults[o.pat()][1]
}

// copyPath is the name of a board's copy beside path: name-amiexpress.ans.
func copyPath(path, id string) string {
	ext := filepath.Ext(path)
	return strings.TrimSuffix(path, ext) + "-" + id + ext
}

// copyBoards are the boards the page also saves a copy for (not the main file's own).
func (a *App) copyBoards(fmtID string) []*bbsBoard {
	if a.order == nil || a.order.Copies == "" {
		return nil
	}
	var out []*bbsBoard
	for _, id := range strings.Split(a.order.Copies, ",") {
		if b := boardByID(id); b != nil && b.shows(fmtID) && id != a.pageBoard() {
			out = append(out, b)
		}
	}
	return out
}

// hasCopy: the page saves a copy for board id.
func (o *drawOrder) hasCopy(id string) bool {
	return o != nil && strings.Contains(","+o.Copies+",", ","+id+",")
}

// toggleCopy ticks or unticks board id's copy.
func (o *drawOrder) toggleCopy(id string) {
	var ids []string
	on := o.hasCopy(id)
	for _, b := range bbsBoards {
		if b.id == id && !on || b.id != id && o.hasCopy(b.id) {
			ids = append(ids, b.id)
		}
	}
	o.Copies = strings.Join(ids, ",")
}

// pacedNote says what a save with board id wrote, for the status line.
func pacedNote(id string, ps []pause) string {
	b := boardByID(id)
	if b == nil {
		return ""
	}
	total := 0
	for _, p := range ps {
		total += p.ms
	}
	return fmt.Sprintf("; %s pauses, %.1f s in all", b.name, float64(total)/1000)
}

// writeCopies saves the copies for the boards ticked in Animate beside path
// (name-amiexpress.ans ...) and says so for the status line.
func (a *App) writeCopies(path, fmtID string) string {
	var done, failed []string
	for _, b := range a.copyBoards(fmtID) {
		p := copyPath(path, b.id)
		data, err := a.encodeFor(fmtID, b.id, nil)
		if err == nil {
			err = writeAtomic(p, data)
		}
		if err != nil {
			failed = append(failed, filepath.Base(p)+": "+err.Error())
			continue
		}
		done = append(done, filepath.Base(p))
	}
	s := ""
	if len(done) > 0 {
		s += "; copies: " + strings.Join(done, ", ")
	}
	if len(failed) > 0 {
		s += "; not saved: " + strings.Join(failed, ", ")
	}
	return s
}
