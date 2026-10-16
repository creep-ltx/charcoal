package main

import (
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// Models of how each board shows a screen file: what reaches the terminal and
// how long it waits (ms). They follow the boards' sources (bbspause.go names
// them) and fail on any code they do not expect, so a code the writer cut in
// two, or art taken for a code, shows up as an error.

// modelAE: Ami-Express 5's displayFile + processMci (express.e).
func modelAE(file []byte) (out []byte, ms int, err error) {
	if len(file) == 0 || file[0] != '~' {
		return nil, 0, fmt.Errorf("the first byte is not ~: the codes would be printed")
	}
	nonStop := false
	for pos := 0; pos < len(file); {
		// Fgets(fh, buf, 999): up to 998 bytes, a LF ends the piece.
		end := pos
		for end < len(file) && end-pos < aeChunk {
			end++
			if file[end-1] == '\n' {
				break
			}
		}
		line := file[pos:end]
		pos = end
		lf := false
		if n := len(line) - 1; n > 1 && line[n-1] == '\r' && line[n] == '\n' {
			line, lf = line[:n-1], true
		} else if line[n] == '\n' {
			line, lf = line[:n], true
		}
		// processMci
		for p := 0; p < len(line); {
			if line[p] != '~' {
				out = append(out, line[p])
				p++
				continue
			}
			p++
			num := ""
			for len(num) < 3 && p < len(line) && line[p] >= '0' && line[p] <= '9' {
				num += string(line[p])
				p++
			}
			term := bytes.IndexByte(line[p:], '|')
			space := bytes.IndexByte(line[p:], ' ')
			t := 0
			stop := len(line) - p
			if term >= 0 {
				stop, t = term, 1
			}
			if space >= 0 && space < stop {
				stop, t = space, 0
			}
			cmd := string(line[p : p+stop])
			switch {
			case cmd == "":
				p += t
			case cmd == "w":
				n := 1
				if num != "" {
					n, _ = strconv.Atoi(num)
				}
				ms += n * 20
				p += 1 + t
			case cmd == "NS":
				nonStop = true
				p += 2 + t
			case strings.HasPrefix(cmd, "~"):
				out = append(out, cmd...)
				p += len(cmd) + t
			default:
				return nil, 0, fmt.Errorf("unexpected code ~%s%q at %d", num, cmd, pos)
			}
		}
		if lf {
			out = append(out, '\r', '\n')
		}
	}
	if !nonStop {
		return nil, 0, fmt.Errorf("no ~NS|: the More? prompt would stop it")
	}
	return out, ms, nil
}

// modelMystic: Mystic's OutFile (bbs_io.pas).
func modelMystic(file []byte) (out []byte, ms int, err error) {
	if !bytes.HasPrefix(file, []byte("|PO")) {
		return nil, 0, fmt.Errorf("no |PO first")
	}
	attr := 0x07
	for p := 0; p < len(file); p++ {
		switch c := file[p]; c {
		case 26:
			return out, ms, nil
		case '|':
			if p+2 >= len(file) {
				return nil, 0, fmt.Errorf("a | at the end")
			}
			switch code := string(file[p+1 : p+3]); code {
			case "DE":
				ms += 500
			case "PI":
				out = append(out, '|')
			case "PO":
			default:
				// |00-|15 the foreground, |16-|23 the background (BIOS order; the rest kept).
				n, err := strconv.Atoi(code)
				switch {
				case err != nil || n < 0 || n > 23:
					return nil, 0, fmt.Errorf("unexpected code |%s", code)
				case n < 16:
					attr = attr&0xF0 | n
				default:
					attr = attr&0x8F | (n-16)<<4
				}
				out = append(out, cbbsSGR(attr)...)
			}
			p += 2
		default:
			out = append(out, c)
		}
	}
	return out, ms, nil
}

// modelPCB: PCBoard's displayfile (dosfgets lines, findtoken, newline).
func modelPCB(file []byte) (out []byte, ms int, err error) {
	if i := bytes.IndexByte(file, 26); i >= 0 {
		file = file[:i]
	}
	if !bytes.HasPrefix(file, []byte("@POFF@")) {
		return nil, 0, fmt.Errorf("no @POFF@ first")
	}
	attr, saved := 0x07, 0x07 // @X00 keeps the colour, @XFF brings it back
	for pos := 0; pos < len(file); {
		if file[pos] == '\n' { // a LF left over at the start of a line
			pos++
			continue
		}
		cr := bytes.IndexByte(file[pos:], '\r')
		var line []byte
		okay := 1
		switch {
		case cr >= 0 && cr < pcbLine:
			line, okay = file[pos:pos+cr], 0
			pos += cr + 1
			if pos < len(file) && file[pos] == '\n' {
				pos++
			}
		case cr == pcbLine:
			line, okay = file[pos:pos+pcbLine], 0
			pos += pcbLine
		default:
			n := min(pcbLine, len(file)-pos)
			line = file[pos : pos+n]
			pos += n
		}
		if len(line) > 0 && strings.ContainsRune("%!$", rune(line[0])) || bytes.HasPrefix(line, []byte("@HANGUP@")) {
			return nil, 0, fmt.Errorf("a line starts with %q", line[:min(8, len(line))])
		}
		for p := 0; p < len(line); p++ {
			rest := line[p:]
			if bytes.HasPrefix(rest, []byte("@POFF@")) {
				p += 5
				continue
			}
			if len(rest) >= 4 && rest[0] == '@' && rest[1] == 'X' && isUpperHex(rest[2]) && isUpperHex(rest[3]) {
				switch v := upperHexVal(rest[2])<<4 | upperHexVal(rest[3]); v {
				case 0x00:
					saved = attr
				case 0xFF:
					attr = saved
					out = append(out, cbbsSGR(attr)...)
				default:
					attr = v
					out = append(out, cbbsSGR(attr)...)
				}
				p += 3
				continue
			}
			if bytes.HasPrefix(rest, []byte("@DELAY:")) {
				q := 7
				for q < len(rest) && rest[q] >= '0' && rest[q] <= '9' {
					q++
				}
				if q == len(rest) || rest[q] != '@' {
					return nil, 0, fmt.Errorf("a @DELAY code cut in two: %q", rest)
				}
				n, _ := strconv.Atoi(string(rest[7:q]))
				if n > 255 {
					return nil, 0, fmt.Errorf("@DELAY:%d@ is past 255", n)
				}
				ms += n * 100
				p += q
				continue
			}
			out = append(out, line[p])
		}
		if okay == 0 {
			out = append(out, '\r', '\n')
		}
	}
	return out, ms, nil
}

// modelSync: Synchronet's putmsg (Ctrl-A codes; every @ and | and LF of the
// art followed by Ctrl-A Q, so no @-code or pipe colour can come of the art).
func modelSync(file []byte) (out []byte, ms int, err error) {
	attr := 0x07
	for p := 0; p < len(file); p++ {
		c := file[p]
		if c == 1 {
			if p+1 >= len(file) {
				return nil, 0, fmt.Errorf("a Ctrl-A at the end")
			}
			p++
			switch file[p] {
			case ',':
				ms += 100
			case ';':
				ms += 500
			case '.':
				ms += 2000
			case 'A':
				out = append(out, 1)
			case 'z':
				out = append(out, 26)
			case 'Q':
			default:
				// Colours (con_out.cpp ctrl_a): N normal, H bright, I blink, letters the
				// foreground (BIOS order), digits the background (ANSI order).
				x := file[p]
				switch i := strings.IndexByte("KBGCRMYW", x); {
				case x == 'N':
					attr = 0x07
				case x == 'H':
					attr |= 0x08
				case x == 'I':
					attr |= 0x80
				case i >= 0:
					attr = attr&^7 | i
				case x >= '0' && x <= '7':
					attr = attr&0x8F | biosANSI[x-'0']<<4
				default:
					return nil, 0, fmt.Errorf("unexpected Ctrl-A %q", x)
				}
				out = append(out, cbbsSGR(attr)...)
			}
			continue
		}
		if c == 26 {
			return nil, 0, fmt.Errorf("a raw Ctrl-Z ends a file with SAUCE")
		}
		if (c == '@' || c == '|' || c == '\n') && !bytes.HasPrefix(file[p+1:], []byte{1, 'Q'}) {
			return nil, 0, fmt.Errorf("a %q without Ctrl-A Q after it", c)
		}
		out = append(out, c)
	}
	return out, ms, nil
}

// modelTempest: Tempest's ~ codes as its manual gives them.
func modelTempest(file []byte) (out []byte, ms int, err error) {
	for p := 0; p < len(file); p++ {
		if file[p] != '~' {
			out = append(out, file[p])
			continue
		}
		rest := file[p+1:]
		switch {
		case bytes.HasPrefix(rest, []byte("~")):
			out = append(out, '~')
			p++
		case bytes.HasPrefix(rest, []byte("DELAY(")):
			q := bytes.IndexByte(rest, ')')
			n, err := strconv.Atoi(string(rest[6:q]))
			if err != nil || n < 1 || n > 500 {
				return nil, 0, fmt.Errorf("bad ~DELAY(%s)", rest[6:q])
			}
			ms += n * 20
			p += q + 1
		default:
			return nil, 0, fmt.Errorf("unexpected code ~%q", rest[:min(6, len(rest))])
		}
	}
	return out, ms, nil
}

// cbbsSGR is what a board sends for attribute a (BIOS order): an SGR that sets all of it.
func cbbsSGR(a int) []byte {
	v := biosANSI[a&15]
	s := "\x1b[0;"
	if v > 7 {
		s += "1;"
	}
	if a&0x80 != 0 {
		s += "5;"
	}
	return fmt.Appendf(nil, "%s%d;%dm", s, 30+v&7, 40+biosANSI[a>>4&7])
}

// cbbsCanon is what reaches a terminal with its colours made comparable: the
// SGRs the boards have codes for, and the models' colour marks (0, attribute),
// become one mark (0, attribute) before the next byte whenever the colours
// changed; other bytes (and SGRs no board has a code for) stay as they are.
func cbbsCanon(b []byte) []byte {
	var out []byte
	c := &sgrColours{fg: 7}
	attr, shown := 0x07, -1
	for i := 0; i < len(b); i++ {
		if b[i] == 0 && i+1 < len(b) {
			attr = int(b[i+1])
			v := biosANSI[attr&15]
			c.fg, c.bold, c.bg, c.blink = v&7, v > 7, biosANSI[attr>>4&7], attr&0x80 != 0
			i++
			continue
		}
		if n, ps, ok := sgrAt(b, i); n > 0 && ok && !slices.ContainsFunc(ps, func(p int) bool { return !(&sgrColours{kind: "ctrla"}).knows(p) }) {
			c.attr = -1
			c.code(b, i)
			attr = c.attr
			i += n - 1
			continue
		}
		if attr != shown {
			out = append(out, cbbsSGR(attr)...)
			shown = attr
		}
		out = append(out, b[i])
	}
	return out
}

var boardModels = map[string]func([]byte) ([]byte, int, error){
	"amiexpress": modelAE, "mystic": modelMystic, "pcboard": modelPCB, "synchronet": modelSync, "tempest": modelTempest,
}

// shown is what the board sends of the plain stream: Ami-Express ends every
// line with CR LF, Mystic and PCBoard stop at a Ctrl-Z.
func shown(id string, stream []byte) []byte {
	switch id {
	case "amiexpress":
		s := bytes.ReplaceAll(stream, []byte("\r\n"), []byte("\n"))
		return bytes.ReplaceAll(s, []byte("\n"), []byte("\r\n"))
	case "mystic", "pcboard":
		if i := bytes.IndexByte(stream, 26); i >= 0 {
			return stream[:i]
		}
	case "cnet": // Ctrl-Q and Ctrl-Y start its codes: the art's are spaces (boardswhole.go)
		out := bytes.Clone(stream)
		for i, c := range out {
			if c == 0x11 || c == 0x19 {
				out[i] = ' '
			}
		}
		return out
	}
	return stream
}

// checkPaced runs board id's file for stream through its model: the same bytes
// reach the terminal, and it waits what the pauses ask (to the board's unit).
func checkPaced(t *testing.T, id string, stream []byte, ps []pause, what string) []byte {
	t.Helper()
	b := boardByID(id)
	file := b.pace(stream, ps)
	got, ms, err := boardModels[id](file)
	if err != nil {
		t.Fatalf("%s %s: %v", id, what, err)
	}
	got = cbbsCanon(got) // a board's colour codes and ANSI's SGRs that give the same colours are the same
	if want := cbbsCanon(shown(id, stream)); !bytes.Equal(got, want) {
		i := 0
		for i < len(got) && i < len(want) && got[i] == want[i] {
			i++
		}
		t.Fatalf("%s %s: the terminal gets other bytes from %d (%d vs %d bytes): %q / %q", id, what, i, len(got), len(want),
			got[i:min(len(got), i+20)], want[i:min(len(want), i+20)])
	}
	units := 0
	asked := 0
	cut := len(stream) // Mystic and PCBoard stop at a Ctrl-Z (a raw one is art in a file with SAUCE)
	if i := bytes.IndexByte(stream, 26); i >= 0 && (id == "mystic" || id == "pcboard") {
		cut = i
	}
	for i, u := range b.units(ps) {
		if ps[i].at <= cut {
			units += u
			asked += b.waited(ps)[i].ms // whole seconds on some boards (boardswhole.go)
		}
	}
	if ms != units*b.unit {
		t.Fatalf("%s %s: waits %d ms, the units say %d", id, what, ms, units*b.unit)
	}
	if d := ms - asked; d > b.unit/2 || -d > b.unit/2 {
		t.Fatalf("%s %s: waits %d ms, %d asked", id, what, ms, asked)
	}
	return file
}

// Every board, every kind of animated stream, on random PC and Amiga pages.
func TestBBSPausesThroughBoards(t *testing.T) {
	r := rand.New(rand.NewSource(77))
	for _, mode := range []string{"pc", "amiga"} {
		for _, size := range [][2]int{{80, 25}, {132, 30}, {40, 60}} {
			a := testApp(t, mode)
			randomPage(a, r, size[0], size[1])
			for i := range a.doc.cells { // the board's own code characters in the art
				if r.Intn(30) == 0 {
					a.doc.cells[i] = a.doc.cells[i]&^255 | uint32("~|@%!$\x01\x1a"[r.Intn(8)])
				}
			}
			a.info.writeSauce = size[0] != 80
			type run struct {
				what   string
				stream []byte
				ps     []pause
			}
			var runs []run
			for _, o := range []*drawOrder{
				{Pattern: "scramble"}, {Pattern: "top", Every: 7, Pause: 30}, {Pattern: "circles", Every: 300, Pause: 13000},
				{Pattern: "fadein"}, {Pattern: "typewriter", Every: 1, Pause: 20},
				{Pattern: "matrix", Frames: 20, Seed: 5}, {Pattern: "decrypt", Frames: 12, Seed: 9, Hold: 70},
			} {
				a.order = o
				s, ps := a.writeOrderedPaced("ansi", a.doc, a.info.writeSauce)
				if len(ps) == 0 {
					t.Fatalf("%s: no pauses", o)
				}
				runs = append(runs, run{o.String(), s, ps})
			}
			a.order = nil
			docs := []*Doc{a.doc.clone(), a.doc.clone(), a.doc.clone()}
			docs[1].cells[0] ^= 1
			a.frames = []Frame{{doc: docs[0], hold: 40}, {doc: docs[1], hold: 1500}, {doc: docs[2], hold: 60}}
			s, ps := a.writeStreamFramesPaced("ansi", docs, a.info.writeSauce)
			runs = append(runs, run{"frames", s, ps})
			a.frames = nil
			for _, b := range bbsBoards {
				if !b.shows("ansi") {
					continue // the C64 boards: c64bbs_test.go
				}
				for _, x := range runs {
					checkPaced(t, b.id, x.stream, x.ps, fmt.Sprintf("%s %dx%d %s", mode, size[0], size[1], x.what))
				}
			}
		}
	}
}

// Ami-Express: "~" first, no code across a 998-byte piece even on art full of
// "~" with long lines, a long pause split into codes of at most 999 ticks, and
// the art read back from what reaches the terminal is the page.
func TestBBSPausesAmiExpress(t *testing.T) {
	r := rand.New(rand.NewSource(3))
	a := testApp(t, "amiga")
	randomPage(a, r, 132, 40)
	for i := range a.doc.cells {
		if r.Intn(4) == 0 {
			a.doc.cells[i] = makeCell('~', r.Intn(16), r.Intn(8), 0)
		}
	}
	a.info.writeSauce = true
	a.order = &drawOrder{Pattern: "scramble", Every: 3, Pause: 20}
	stream, ps := a.writeOrderedPaced("ansi", a.doc, true)
	ps = append(ps, pause{at: len(stream) / 2, ms: 45000})
	file := checkPaced(t, "amiexpress", stream, ps, "tildes")
	if !bytes.HasPrefix(file, []byte("~NS|")) {
		t.Fatalf("starts %q", file[:8])
	}
	if !bytes.Contains(file, []byte("~999w|")) {
		t.Fatal("a 45 s pause is not split into codes of 999 ticks")
	}
	if n := bytes.Count(file, []byte("~~|")); n < 100 {
		t.Fatalf("only %d escaped tildes", n)
	}
	// Every piece Fgets reads holds whole codes only (the model fails otherwise); check the cut directly too.
	for pos := 0; pos < len(file); {
		end := pos
		for end < len(file) && end-pos < aeChunk {
			end++
			if file[end-1] == '\n' {
				break
			}
		}
		piece := file[pos:end]
		if i := bytes.LastIndexByte(piece, '~'); i >= 0 && i < len(piece)-1 && bytes.IndexByte(piece[i:], '|') < 0 {
			t.Fatalf("a code cut at %d: %q", end, piece[i:])
		}
		pos = end
	}
	terminal, _, err := modelAE(file)
	if err != nil {
		t.Fatal(err)
	}
	want := readBack(a, "ansi", stream, a.doc.cols)
	got := readBack(a, "ansi", terminal, a.doc.cols)
	if ok, why := cellsMatch("ansi", want, got); !ok {
		t.Fatalf("read back: %s", why)
	}
	plainArt := readBack(a, "ansi", plainWrite(a, "ansi", a.doc), a.doc.cols)
	if ok, why := cellsMatch("ansi", plainArt, got); !ok {
		t.Fatalf("not the page: %s", why)
	}
}

// Rounding: a board's units add up to the time asked, the remainders carried.
func TestBBSPauseUnits(t *testing.T) {
	b := boardByID("mystic")
	ps := make([]pause, 40)
	for i := range ps {
		ps[i] = pause{at: i, ms: 40}
	}
	total := 0
	for _, u := range b.units(ps) {
		total += u
	}
	if total != 3 { // 1600 ms in half seconds
		t.Fatalf("%d half seconds for 1.6 s", total)
	}
	if got := string(boardByID("synchronet").delay(27)); got != "\x01.\x01;\x01,\x01," {
		t.Fatalf("Synchronet 2.7 s: %q", got)
	}
}

// Saving: Settings' My BBS goes into every animated save, the page's own
// choice wins, "none" is plain, stills never get codes, and the ticked copies
// are written beside the file.
func TestBBSPausesSaved(t *testing.T) {
	a := testApp(t, "pc")
	randomPage(a, rand.New(rand.NewSource(8)), 80, 25)
	dir := t.TempDir()
	p := filepath.Join(dir, "art.ans")
	read := func(name string) []byte {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	prefs.MyBBS = "amiexpress"
	if !a.writeOut(p, "ansi") || bytes.HasPrefix(read("art.ans"), []byte("~")) {
		t.Fatal("a still picture got pause codes")
	}
	a.order = &drawOrder{Pattern: "diamond"}
	if !a.writeOut(p, "ansi") || !bytes.HasPrefix(read("art.ans"), []byte("~NS|")) {
		t.Fatal("My BBS's codes are not in the saved draw order")
	}
	a.order = &drawOrder{Pattern: "diamond", BBS: "none"}
	plain, _ := a.encode("ansi")
	a.writeOut(p, "ansi")
	if got := read("art.ans"); !bytes.Equal(got, plain) || bytes.HasPrefix(got, []byte("~")) {
		t.Fatal("none for the page still wrote codes")
	}
	a.order = &drawOrder{Pattern: "diamond", BBS: "mystic", Copies: "amiexpress,pcboard,mystic"}
	a.writeOut(p, "ansi")
	if !bytes.HasPrefix(read("art.ans"), []byte("|PO")) {
		t.Fatal("the page's own board is not used")
	}
	if !bytes.HasPrefix(read("art-amiexpress.ans"), []byte("~NS|")) || !bytes.HasPrefix(read("art-pcboard.ans"), []byte("@POFF@")) {
		t.Fatal("the copies are not written")
	}
	if _, err := os.Stat(filepath.Join(dir, "art-mystic.ans")); err == nil {
		t.Fatal("a copy for the main file's own board")
	}
	// The saved copy, through its board, is the plain draw order.
	a.order = &drawOrder{Pattern: "diamond", BBS: "none"}
	stream, _ := a.encode("ansi")
	got, _, err := modelPCB(read("art-pcboard.ans"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(cbbsCanon(got), cbbsCanon(shown("pcboard", stream))) {
		t.Fatal("the PCBoard copy does not show the draw order")
	}
	// Frames on the strip: the holds become pauses.
	a.order = nil
	a.frames = []Frame{{doc: a.doc.clone(), hold: 500}, {doc: a.doc.clone(), hold: 500}}
	prefs.MyBBS = "synchronet"
	a.writeOut(p, "ansi")
	if !bytes.Contains(read("art.ans"), []byte("\x01;")) {
		t.Fatal("the frame strip's hold is not a Synchronet pause")
	}
	// The page's choice is kept with it (autosave, undo) as part of the order.
	o := drawOrder{Pattern: "top", BBS: "pcboard", Copies: "mystic", Every: 5, Pause: 100}
	o.toggleCopy("amiexpress")
	if o.Copies != "amiexpress,mystic" || !o.hasCopy("mystic") {
		t.Fatalf("copies %q", o.Copies)
	}
}
