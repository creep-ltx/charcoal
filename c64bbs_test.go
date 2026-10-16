package main

import (
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Models of how the C64 boards show a screen file (c64bbs.go names where in
// their sources each part was read): what reaches the terminal and how long
// they wait. They fail on anything the writers must never produce: a code cut
// by a line's end, art taken for a code, a lost last line.

// cbbsModelImage: Image BBS's read0 + dskin (lines of 80 bytes or up to a
// RETURN; a line starting with ↑ ends the file; the last byte of the file
// comes with the end-of-file status, which turns a line without a RETURN into
// "↑") and its MCI (££, £S speed in tenths per character, £B bells).
func cbbsModelImage(file []byte) (out []byte, ms int, err error) {
	speed := 0
	for pos := 0; pos < len(file); {
		end, cr := pos, false
		for end < len(file) && end-pos < imageLine {
			if file[end] == 0x0D {
				cr = true
				break
			}
			end++
		}
		line := file[pos:end]
		if !cr && len(line) < imageLine {
			return nil, 0, fmt.Errorf("the last line (%q) has no RETURN: Image drops it", line[:min(10, len(line))])
		}
		pos = end
		if cr {
			pos++
		}
		if len(line) > 0 && line[0] == '^' {
			return nil, 0, fmt.Errorf("a line starts with ↑ at %d: Image stops there", end-len(line))
		}
		for i := 0; i < len(line); {
			c := line[i]
			if c == pound {
				if i+1 >= len(line) {
					return nil, 0, fmt.Errorf("a £ at the end of a line")
				}
				if line[i+1] == pound {
					ms += speed * 100
					out = append(out, pound)
					i += 2
					continue
				}
				if i+2 >= len(line) {
					return nil, 0, fmt.Errorf("a code cut by a line's end: %q", line[i:])
				}
				p := int(line[i+2] & 15)
				switch line[i+1] & 0x3F {
				case 'S' & 0x3F:
					speed = p
				case 'B' & 0x3F:
					if p != 0 {
						return nil, 0, fmt.Errorf("£B%d rings bells", p)
					}
				default:
					return nil, 0, fmt.Errorf("unexpected code %q", line[i:i+3])
				}
				i += 3
				continue
			}
			if printable(c) {
				ms += speed * 100
			} else if speed != 0 {
				return nil, 0, fmt.Errorf("a control code %02x at speed %d (Image 2.0 would slow it too)", c, speed)
			}
			out = append(out, c)
			i++
		}
		if len(line) < imageLine {
			out = append(out, 0x0D)
			speed = 0
		}
	}
	return out, ms, nil
}

// cbbsCBaseCommand: the letters after a £ that C*Base's mcichk acts on.
func cbbsCBaseCommand(c byte) bool { return strings.IndexByte("NTSVOPRLWIB", c) >= 0 }

// cbbsModelCBase: C*Base's readln3 (lines of up to 255 bytes, the RETURN
// kept) and bufout2 (Ctrl-P waits and prints the next byte as it is; £ + a
// command letter and @: are codes; both look at the next byte of the line).
func cbbsModelCBase(file []byte) (out []byte, ms int, err error) {
	for pos := 0; pos < len(file); {
		end := pos
		for end < len(file) && end-pos < cbaseLine {
			end++
			if file[end-1] == 0x0D {
				break
			}
		}
		line := file[pos:end]
		pos = end
		for i := 0; i < len(line); i++ {
			c := line[i]
			switch {
			case c == 0x10:
				if i+1 >= len(line) {
					return nil, 0, fmt.Errorf("a Ctrl-P at the end of a line")
				}
				ms += 33
				i++
				out = append(out, line[i])
				continue
			case c == pound || c == '@':
				if i+1 >= len(line) {
					return nil, 0, fmt.Errorf("a %q at the end of a line", c)
				}
				if n := line[i+1]; c == pound && cbbsCBaseCommand(n) || c == '@' && n == ':' {
					return nil, 0, fmt.Errorf("art taken for a code: %q", line[i:i+2])
				}
			}
			out = append(out, c)
		}
	}
	return out, ms, nil
}

// cbbsDropNoops leaves out the reverse codes that change nothing (the
// writers' fillers): what the terminal shows is the same without them.
func cbbsDropNoops(b []byte) []byte {
	var out []byte
	var st petState
	for _, c := range b {
		if !st.quote && (c == 0x12 && st.rev || c == 0x92 && !st.rev) {
			continue
		}
		out = append(out, c)
		st.see([]byte{c})
	}
	return out
}

// cbbsArt makes a random PETSCII page full of the boards' code characters: £ @ : ↑ " and their command letters.
func cbbsArt(a *App, r *rand.Rand, cols, rows int) {
	randomPage(a, r, cols, rows)
	special := []int{0x1C, 0x00, 0x3A, 0x1E, 0x22, 0x0E, 0x13, 0x02, 0x03, 0x06, 0x12, 0x1C, 0x1C}
	for i := range a.doc.cells {
		if r.Intn(5) == 0 {
			sc := special[r.Intn(len(special))]
			if r.Intn(4) == 0 {
				sc |= 0x80
			}
			a.doc.cells[i] = makeCell(sc, r.Intn(16), 0, 0)
		}
	}
}

// cbbsRuns are the animated streams of a page: draw orders, effects, frames.
func cbbsRuns(t *testing.T, a *App) (names []string, streams [][]byte, pss [][]pause) {
	t.Helper()
	for _, o := range []*drawOrder{
		{Pattern: "scramble"}, {Pattern: "top", Every: 7, Pause: 30}, {Pattern: "circles", Every: 300, Pause: 13000},
		{Pattern: "fadein"}, {Pattern: "typewriter", Every: 1, Pause: 120},
		{Pattern: "matrix", Frames: 20, Seed: 5}, {Pattern: "decrypt", Frames: 12, Seed: 9, Hold: 70},
	} {
		a.order = o
		s, ps := a.writeOrderedPaced("seq", a.doc, false)
		if len(ps) == 0 {
			t.Fatalf("%s: no pauses", o)
		}
		names, streams, pss = append(names, o.String()), append(streams, s), append(pss, ps)
	}
	a.order = nil
	docs := []*Doc{a.doc.clone(), a.doc.clone(), a.doc.clone()}
	docs[1].cells[0] ^= 1
	a.frames = []Frame{{doc: docs[0], hold: 40}, {doc: docs[1], hold: 2500}, {doc: docs[2], hold: 60}}
	s, ps := a.writeStreamFramesPaced("seq", docs, false)
	a.frames = nil
	return append(names, "frames"), append(streams, s), append(pss, ps)
}

// Every animated stream of random pages through both boards' models: the
// terminal gets the plain stream (fillers that change nothing aside), and the
// board waits what was asked.
func TestCbbsPausesThroughBoards(t *testing.T) {
	r := rand.New(rand.NewSource(64))
	for _, size := range [][2]int{{40, 25}, {40, 12}, {80, 25}} {
		a := testApp(t, "c64")
		if size[0] == 80 {
			a.machine = "c128v"
		}
		cbbsArt(a, r, size[0], size[1])
		names, streams, pss := cbbsRuns(t, a)
		for k, stream := range streams {
			what := fmt.Sprintf("%dx%d %s", size[0], size[1], names[k])
			ps := pss[k]
			// Image: the pauses wait at the next character that shows; those after the last are left out.
			last := bytes.LastIndexFunc(stream, func(c rune) bool { return printable(byte(c)) })
			asked := 0
			for _, p := range ps {
				if p.at <= last {
					asked += p.ms
				}
			}
			file := boardByID("imagebbs").pace(stream, ps)
			got, ms, err := cbbsModelImage(file)
			if err != nil {
				t.Fatalf("Image %s: %v", what, err)
			}
			want := cbbsDropNoops(stream)
			if got = cbbsDropNoops(got); !bytes.Equal(got, want) && !bytes.Equal(got, append(want, 0x0D)) {
				i := 0
				for i < len(got) && i < len(want) && got[i] == want[i] {
					i++
				}
				t.Fatalf("Image %s: the terminal gets other bytes from %d: %q / %q", what, i, got[i:min(len(got), i+20)], want[i:min(len(want), i+20)])
			}
			if d := ms - asked; d > 50 || d < -50 {
				t.Fatalf("Image %s: waits %d ms, %d asked", what, ms, asked)
			}
			// C*Base: every pause, at about 1/30 s.
			asked = 0
			for _, p := range ps {
				if p.at < len(stream) {
					asked += p.ms
				}
			}
			file = boardByID("cbase").pace(stream, ps)
			got, ms, err = cbbsModelCBase(file)
			if err != nil {
				t.Fatalf("C*Base %s: %v", what, err)
			}
			if !bytes.Equal(cbbsDropNoops(got), want) {
				t.Fatalf("C*Base %s: the terminal gets other bytes", what)
			}
			// (a £ or @ in quote mode is written after a Ctrl-P: about 1/30 s more each)
			if d := ms - asked; d > 17+33*bytes.Count(stream, []byte{pound})+33*bytes.Count(stream, []byte("@")) || d < -17 {
				t.Fatalf("C*Base %s: waits %d ms, %d asked", what, ms, asked)
			}
		}
	}
}

// Image: a long pause is spread over the next characters at 1.5 s each, no
// code is cut by its 80-byte lines (the model checks), ↑ never starts a line,
// and the file ends with a RETURN.
func TestCbbsImageLayout(t *testing.T) {
	stream := append(bytes.Repeat([]byte("^"), 300), "AB\x0dCD"...)
	ps := []pause{{at: 0, ms: 3200}, {at: 79, ms: 100}, {at: 160, ms: 900}, {at: 301, ms: 400}}
	file := boardByID("imagebbs").pace(stream, ps)
	if !bytes.HasPrefix(file, []byte("\x5cSO^^\x5cS2^\x5cS0^")) {
		t.Fatalf("3.2 s: %q", file[:20])
	}
	got, ms, err := cbbsModelImage(file)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(cbbsDropNoops(got), append(stream, 0x0D)) || ms != 4600 {
		t.Fatalf("%d ms, %q", ms, got)
	}
	// A £ is ££ also in a slowed character.
	file = boardByID("imagebbs").pace([]byte{pound, 'A'}, []pause{{at: 0, ms: 300}})
	if !bytes.Equal(file, []byte("\x5cS3\x5c\x5c\x5cS0A\x0d")) {
		t.Fatalf("£: %q", file)
	}
}

// C*Base: a £ before a command letter, an @ before a colon, and pauses in
// quote mode.
func TestCbbsCBaseEscapes(t *testing.T) {
	stream := []byte("\x5cN@:\"\x5cS@:\"\x0d")
	ps := []pause{{at: 1, ms: 66}, {at: 5, ms: 99}}
	file := boardByID("cbase").pace(stream, ps)
	want := []byte("\x5c\x92\x10\x92\x10\x92N@\x92:\"\x10\x5c\x10S\x10@:\"\x0d")
	if !bytes.Equal(file, want) {
		t.Fatalf("got %q\nwant %q", file, want)
	}
	got, ms, err := cbbsModelCBase(file)
	if err != nil || !bytes.Equal(cbbsDropNoops(got), stream) || ms != 5*33 {
		t.Fatalf("%v %d ms %q", err, ms, got)
	}
}

var cbbsSGRs = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// Mystic, Synchronet and PCBoard get their own colour codes in an animated
// file; a stream with colours one has no code for keeps ANSI.
func TestCbbsBoardColours(t *testing.T) {
	a := testApp(t, "pc")
	randomPage(a, rand.New(rand.NewSource(5)), 80, 25)
	for i := range a.doc.cells { // no blink: Mystic 1.10 has no code for it; no black on black: PCBoard's @X00 saves a colour
		a.doc.cells[i] = makeCell(cellCh(a.doc.cells[i]), max(1, cellFg(a.doc.cells[i])), cellBg(a.doc.cells[i])&7, 0)
	}
	a.order = &drawOrder{Pattern: "top"}
	for id, code := range map[string]string{"mystic": "|14", "synchronet": "\x01Y", "pcboard": "@X0E"} {
		a.order.BBS = id
		file, err := a.encode("ansi")
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Contains(file, []byte(code)) || cbbsSGRs.Match(file) {
			t.Fatalf("%s: no %q, or SGRs left", id, code)
		}
		plain := a.writeOrdered("ansi", a.doc, false)
		if len(file) >= len(plain) {
			t.Fatalf("%s: %d bytes, %d plain", id, len(file), len(plain))
		}
	}
	// Blink: Mystic keeps the ANSI colours, Synchronet writes Ctrl-A I.
	a.doc.set(0, 0, makeCell('B', 1, 12, 0))
	a.order.BBS = "mystic"
	if file, _ := a.encode("ansi"); !bytes.Contains(file, []byte("\x1b[")) || bytes.Contains(file, []byte("|14")) {
		t.Fatal("Mystic: a blinking page lost its ANSI colours")
	}
	a.order.BBS = "synchronet"
	if file, _ := a.encode("ansi"); !bytes.Contains(file, []byte("\x01I")) {
		t.Fatal("Synchronet: no Ctrl-A I for blink")
	}
	// An italic cell on an Amiga page: ANSI stays.
	if c := boardColourer("ctrla", []byte("\x1b[0;3;31mx")); c != nil {
		t.Fatal("italic converted")
	}
	if n, ps, ok := sgrAt([]byte("\x1b[m"), 0); n != 3 || !ok || len(ps) != 1 || ps[0] != 0 {
		t.Fatalf("ESC[m: %d %v %v", n, ps, ok)
	}
	if n, _, _ := sgrAt([]byte("\x1b[2J"), 0); n != 0 {
		t.Fatal("a clear taken for a colour")
	}
}

// Reading Image's codes: £C colours, £F clears, £R reverse, ££ a £; the
// caller's name and the speed codes stay as text, and the file goes back as
// it came.
func TestCbbsImageRead(t *testing.T) {
	file := []byte("\x93\x5cC2RED\x5cR1REV\x5cR0 \x5c\x5c5 \x5cV2 \x5cS3SLOW\x0d\x5cCJ\x5cC0X")
	if !isImageCodes(file) {
		t.Fatal("not taken for Image's codes")
	}
	d, _ := parseSeq(file, 40, "imageseq")
	if c := d.get(0, 0); cellCh(c) != 0x12 || cellFg(c) != 2 {
		t.Fatalf("£C2: %x", c)
	}
	if c := d.get(3, 0); cellCh(c) != 0x92 { // R reversed
		t.Fatalf("£R1: %x", c)
	}
	if c := d.get(7, 0); cellCh(c) != 0x1C || d.get(8, 0)&255 != '5' {
		t.Fatalf("££: %x", c)
	}
	if got := string([]rune{petRune(cellCh(d.get(10, 0)), false), petRune(cellCh(d.get(11, 0)), false), petRune(cellCh(d.get(12, 0)), false)}); got != "£V2" {
		t.Fatalf("£V2 shown as %q", got)
	}
	if c := d.get(0, 1); cellCh(c) != 0x1C || cellFg(c) != 10 { // £CJ is light red; £C0 (the caller's colour) is text
		t.Fatalf("£CJ / £C0: %x", c)
	}
	back := imageEscape(writeSeq(d, false, false))
	d2, _ := parseSeq(back, 40, "imageseq")
	if ok, why := cellsMatch("seq", d, d2); !ok {
		t.Fatalf("round trip: %s (%q)", why, back)
	}
	for _, keep := range []string{"\x5cV2", "\x5cS3", "\x5cC0"} {
		if !bytes.Contains(back, []byte(keep)) {
			t.Fatalf("%q not written back: %q", keep, back)
		}
	}
	// Plain PETSCII art with £ in it is not taken for codes.
	for _, s := range []string{"\x5c\x5c\x5c ART \x5cC", "£ \x5c1 \x5cA2 \x5cB", "\x5cC1 only one"} {
		if isImageCodes([]byte(s)) {
			t.Fatalf("%q taken for Image's codes", s)
		}
	}
}

// In the editor: a .seq with Image's codes opens with them and saves them
// again; Open as asks for them; Save as plain .seq writes PETSCII codes.
func TestCbbsImageInEditor(t *testing.T) {
	a := testApp(t, "c64")
	a.hasC64 = true
	dir := t.TempDir()
	p := filepath.Join(dir, "s.menu.seq")
	file := []byte("\x93\x5cC7MENU \x5cV2\x0d\x5cR1 \x5c\x5c1 \x5cR0\x5cC3OK\x0d")
	os.WriteFile(p, file, 0o644)
	a.openPath(p, "auto")
	if a.codes != "imageseq" || a.saveFmt != "imageseq" {
		t.Fatalf("codes %q, saves as %q", a.codes, a.saveFmt)
	}
	if c := a.doc.get(0, 0); cellFg(c) != 7 {
		t.Fatalf("£C7: %x", c)
	}
	if !a.writeOut(p, "imageseq") {
		t.Fatal("not saved")
	}
	back, _ := os.ReadFile(p)
	if !bytes.Contains(back, []byte("\x5cV2")) || !bytes.Contains(back, []byte("\x12 \x5c1")) {
		t.Fatalf("saved %q", back)
	}
	d1 := a.doc.clone()
	a.openPath(p, "auto")
	if ok, why := cellsMatch("seq", d1, a.doc); !ok || a.codes != "imageseq" {
		t.Fatalf("reopened: %s, codes %q", why, a.codes)
	}
	q := filepath.Join(dir, "plain.seq")
	a.writeOut(q, "seq")
	if a.codes != "" {
		t.Fatal("plain .seq still keeps Image's codes")
	}
	// Open as with Image's codes on a file that has too few to be guessed.
	r := filepath.Join(dir, "one.seq")
	os.WriteFile(r, []byte("\x5cC2X"), 0o644)
	a.openPath(r, "auto")
	if a.codes != "" {
		t.Fatal("one code guessed")
	}
	a.openPath(r, "imageseq")
	if a.codes != "imageseq" || cellFg(a.doc.get(0, 0)) != 2 {
		t.Fatalf("Open as: codes %q", a.codes)
	}
}

// Real PETSCII screens (sblendorio/petscii-bbs, MPL-2.0, fetched by hand into
// CHARCOAL_C64BBS_FILES) draw with £: none is taken for Image's codes, and
// every one goes through both boards' models.
func TestCbbsRealFiles(t *testing.T) {
	dir := os.Getenv("CHARCOAL_C64BBS_FILES")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, ".cache", "charcoal-c64bbs", "petscii-bbs")
	}
	var files []string
	filepath.WalkDir(dir, func(p string, e os.DirEntry, err error) error {
		if err == nil && !e.IsDir() && strings.EqualFold(filepath.Ext(p), ".seq") {
			files = append(files, p)
		}
		return nil
	})
	if len(files) == 0 {
		t.Skip("no .seq files in", dir)
	}
	for _, p := range files {
		b, _ := os.ReadFile(p)
		if isImageCodes(b) {
			t.Errorf("%s taken for Image's codes", p)
		}
		// Pauses after a RETURN: a stray " in a real file leaves quote mode on for
		// the rest of its line, where a filler would print (c64bbs.go says so).
		at := func(i int) int {
			if j := bytes.IndexByte(b[i:], 0x0D); j >= 0 {
				return i + j + 1
			}
			return len(b)
		}
		ps := []pause{{at: at(len(b) / 3), ms: 500}, {at: at(len(b) / 2), ms: 2000}}
		for id, model := range map[string]func([]byte) ([]byte, int, error){"imagebbs": cbbsModelImage, "cbase": cbbsModelCBase} {
			got, _, err := model(boardByID(id).pace(b, ps))
			if err != nil {
				t.Fatalf("%s %s: %v", id, p, err)
			}
			want := cbbsDropNoops(b)
			if got = cbbsDropNoops(got); !bytes.Equal(got, want) && !bytes.Equal(got, append(want, 0x0D)) {
				t.Fatalf("%s %s: the terminal gets other bytes", id, p)
			}
		}
	}
	t.Logf("%d files", len(files))
}

// Animate a folder for a C64 board: its .seq screens get the board's codes
// and read back through its model as the screen; an .ans is skipped.
func TestCbbsBatch(t *testing.T) {
	testApp(t, "c64")
	a.hasC64 = true
	src := t.TempDir()
	r := rand.New(rand.NewSource(9))
	want := map[string]*Doc{}
	for i := range 3 {
		d := newDoc(40, 12+i)
		d.glyph0 = true // as parseSeq: screen code 0 is @, not empty
		for j := range d.cells {
			if r.Intn(3) > 0 {
				d.cells[j] = makeCell([]int{0x1C, 0x00, 0x1E, 0x01 + r.Intn(26), 0xA0}[r.Intn(5)], r.Intn(16), 0, 0)
			}
		}
		name := fmt.Sprintf("s.screen%d.seq", i)
		os.WriteFile(filepath.Join(src, name), writeSeq(d, false, true), 0o644)
		want[name] = d
	}
	writeTestArt(t, src, "PC.ANS", "pc", 1)
	for _, board := range []string{"none", "imagebbs", "cbase"} {
		out := filepath.Join(t.TempDir(), board)
		run := runBatch(t, batchSpec{Inputs: []string{src}, Effect: "random", Seed: 4, Board: board, Out: out})
		done, skipped, _ := run.counts()
		if board == "none" {
			done, skipped = done-1, skipped+1 // (the .ans too)
		}
		if done != 3 || skipped != 1 {
			t.Fatalf("%s: %d done, %d skipped\n%s", board, done, skipped, run.reportText())
		}
		model := map[string]func([]byte) ([]byte, int, error){"none": func(b []byte) ([]byte, int, error) { return b, 0, nil }, "imagebbs": cbbsModelImage, "cbase": cbbsModelCBase}[board]
		for name, d := range want {
			file, err := os.ReadFile(filepath.Join(out, name))
			if err != nil {
				t.Fatal(err)
			}
			stream, _, err := model(file)
			if err != nil {
				t.Fatalf("%s %s: %v", board, name, err)
			}
			got, _ := parseSeq(stream, 40)
			if ok, why := cellsMatch("seq", d, got); !ok {
				t.Fatalf("%s %s: %s", board, name, why)
			}
		}
	}
}

// A page read with Image's codes, given a draw order: Save writes the
// animation (with Image's pauses when it is My BBS), and a copy for C*Base.
func TestCbbsImageAnimated(t *testing.T) {
	a := testApp(t, "c64")
	a.hasC64 = true
	dir := t.TempDir()
	p := filepath.Join(dir, "s.anim.seq")
	os.WriteFile(p, []byte("\x93\x5cC7HELLO \x5c\x5c5 \x5cV2\x0d\x5cR1 WORLD \x5cR0\x5cC3OK\x0d"), 0o644)
	a.openPath(p, "auto")
	if a.saveFmt != "imageseq" {
		t.Fatalf("saves as %q", a.saveFmt)
	}
	want := a.doc.clone()
	for _, board := range []string{"none", "imagebbs"} {
		a.order = &drawOrder{Pattern: "scramble", BBS: board, Copies: "cbase", Every: 3, Pause: 200}
		if !a.writeOut(p, "imageseq") {
			t.Fatal("not saved")
		}
		file, _ := os.ReadFile(p)
		stream := file
		if board == "imagebbs" {
			var err error
			if stream, _, err = cbbsModelImage(file); err != nil {
				t.Fatal(err)
			}
			stream = bytes.ReplaceAll(stream, []byte{pound}, []byte{pound, pound}) // what reached the terminal, as Image's codes
		}
		got, _ := parseSeq(stream, 40, "imageseq")
		if ok, why := cellsMatch("seq", want, got); !ok {
			t.Fatalf("%s: %s", board, why)
		}
		if bytes.Count(file, []byte{0x1D})+bytes.Count(file, []byte{0x11}) < 5 {
			t.Fatalf("%s: no draw order in %q", board, file)
		}
		cfile, err := os.ReadFile(filepath.Join(dir, "s.anim-cbase.seq"))
		if err != nil {
			t.Fatal(err)
		}
		if _, ms, err := cbbsModelCBase(cfile); err != nil || ms == 0 {
			t.Fatalf("C*Base copy: %v, %d ms", err, ms)
		}
	}
}
