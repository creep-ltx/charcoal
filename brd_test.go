package main

import (
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Round 7's board leftovers: whole-second boards, Animate a folder with BBS
// colour codes, PCBoard's @X00/@XFF, Telegard/Renegade/Mystic theme codes,
// the DirArt header line. Helpers are brdF...

func init() {
	boardModels["fame"] = brdFModelFAME
	boardModels["cnet"] = brdFModelCNet
}

// brdFModelFAME: FAME's MCI as its guide gives it: codes only when the first
// byte is "~"; "~~" is a "~", "~0|" resets the colours, "~Wn|" waits n
// seconds (1-10); anything else is an error (art taken for a code).
func brdFModelFAME(file []byte) (out []byte, ms int, err error) {
	if len(file) == 0 || file[0] != '~' {
		return nil, 0, fmt.Errorf("the first byte is not ~: FAME would not read the codes")
	}
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
		case bytes.HasPrefix(rest, []byte("0|")):
			out = append(out, "\x1b[0m"...)
			p += 2
		case len(rest) > 0 && (rest[0] == 'W' || rest[0] == 'w'):
			q := bytes.IndexByte(rest, '|')
			if q < 0 {
				return nil, 0, fmt.Errorf("~W without its terminator")
			}
			n, err := strconv.Atoi(string(rest[1:q]))
			if err != nil || n < 1 || n > 10 {
				return nil, 0, fmt.Errorf("bad ~W%s|", rest[1:q])
			}
			ms += n * 1000
			p += q + 1
		default:
			return nil, 0, fmt.Errorf("unexpected code ~%q", rest[:min(6, len(rest))])
		}
	}
	return out, ms, nil
}

// brdFModelCNet: CNet's MCI from its help: Ctrl-Q + command + argument + "}";
// "{@n}" sets the file's MCI flags, "{Wn}" waits n seconds. Ctrl-Y (the
// short form) must not occur; nor any other code.
func brdFModelCNet(file []byte) (out []byte, ms int, err error) {
	for p := 0; p < len(file); p++ {
		switch file[p] {
		case 0x19:
			return nil, 0, fmt.Errorf("a Ctrl-Y at %d starts a CNet code", p)
		case 0x11:
		default:
			out = append(out, file[p])
			continue
		}
		q := bytes.IndexByte(file[p:], '}')
		if q < 2 {
			return nil, 0, fmt.Errorf("a Ctrl-Q code without its }")
		}
		code := string(file[p+1 : p+q])
		switch {
		case code == "@6":
		case code[0] == 'W':
			n, err := strconv.Atoi(code[1:])
			if err != nil || n < 1 || n > 99 {
				return nil, 0, fmt.Errorf("bad {%s}", code)
			}
			ms += n * 1000
		default:
			return nil, 0, fmt.Errorf("unexpected code {%s}", code)
		}
		p += q
	}
	return out, ms, nil
}

// brdFModelImageW: Image BBS 1.2's lines (as cbbsModelImage) with £W n
// waiting n seconds (the first one short on the board: counted whole here).
func brdFModelImageW(file []byte) (out []byte, ms int, err error) {
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
			return nil, 0, fmt.Errorf("the last line has no RETURN: Image drops it")
		}
		pos = end
		if cr {
			pos++
		}
		if len(line) > 0 && line[0] == '^' {
			return nil, 0, fmt.Errorf("a line starts with ↑")
		}
		for i := 0; i < len(line); {
			if line[i] != pound {
				out = append(out, line[i])
				i++
				continue
			}
			if i+1 < len(line) && line[i+1] == pound {
				out = append(out, pound)
				i += 2
				continue
			}
			if i+2 >= len(line) {
				return nil, 0, fmt.Errorf("a code cut by a line's end: %q", line[i:])
			}
			p := int(line[i+2] & 15)
			switch line[i+1] & 0x3F {
			case 'W' & 0x3F:
				if p < 1 {
					return nil, 0, fmt.Errorf("£W0")
				}
				ms += p * 1000
			case 'B' & 0x3F:
				if p != 0 {
					return nil, 0, fmt.Errorf("£B%d rings bells", p)
				}
			default:
				return nil, 0, fmt.Errorf("unexpected code %q", line[i:i+3])
			}
			i += 3
		}
		if len(line) < imageLine {
			out = append(out, 0x0D)
		}
	}
	return out, ms, nil
}

// The rounding: holds to the nearest second, at least one; a draw order's
// even pauses carried; what Save reports and Play waits are those seconds.
func TestBrdWholeUnits(t *testing.T) {
	b := boardByID("fame")
	ps := []pause{{at: 1, ms: 40}, {at: 2, ms: 1499}, {at: 3, ms: 1500}, {at: 4, ms: 0}, {at: 5, ms: 2600}}
	if got, want := fmt.Sprint(b.units(ps)), "[1 1 2 0 3]"; got != want {
		t.Fatalf("holds: %s, want %s", got, want)
	}
	var sp []pause
	for i := range 60 {
		sp = append(sp, pause{at: i, ms: 40, spread: true})
	}
	us, total := b.units(sp), 0
	for _, u := range us {
		total += u
	}
	if total != 2 { // 60 x 40 ms = 2.4 s
		t.Fatalf("spread pauses: %d s, want 2 (%v)", total, us)
	}
	w := b.waited(ps)
	if w[0].ms != 1000 || w[4].ms != 3000 || w[3].ms != 0 {
		t.Fatalf("waited: %v", w)
	}
	if m := boardByID("mystic"); fmt.Sprint(m.waited(ps)) != fmt.Sprint(ps) {
		t.Fatalf("a board with short units changed its pauses")
	}
	// A hold of 15 s: FAME splits it into codes of at most 10.
	file := b.pace([]byte("ab"), []pause{{at: 1, ms: 15000}})
	if string(file) != "~0|a~W10|~W5|b" {
		t.Fatalf("FAME file %q", file)
	}
	file = boardByID("cnet").pace([]byte("a\x11b\x19"), []pause{{at: 1, ms: 300}})
	if string(file) != "\x11@6}a\x11W1} b " {
		t.Fatalf("CNet file %q", file)
	}
}

// Every animated stream of random pages through the two Amiga boards' models
// (with ◄ ↓ ~ in the art), and C64 streams through Image 1.2's.
func TestBrdWholeSecondBoards(t *testing.T) {
	r := rand.New(rand.NewSource(71))
	a := testApp(t, "pc")
	randomPage(a, r, 80, 25)
	for i := range a.doc.cells {
		if r.Intn(25) == 0 {
			a.doc.cells[i] = a.doc.cells[i]&^255 | uint32("~|\x11\x19 {}"[r.Intn(7)])
		}
	}
	for _, o := range []*drawOrder{
		{Pattern: "scramble"}, {Pattern: "top", Every: 7, Pause: 300}, {Pattern: "matrix", Frames: 12, Seed: 5},
		{Pattern: "decrypt", Frames: 8, Seed: 9, Hold: 2600},
	} {
		a.order = o
		s, ps := a.writeOrderedPaced("ansi", a.doc, false)
		for _, id := range []string{"fame", "cnet"} {
			checkPaced(t, id, s, ps, o.String())
		}
	}
	a.order = nil

	c := testApp(t, "c64")
	cbbsArt(c, r, 40, 25)
	names, streams, pss := cbbsRuns(t, c)
	b := boardByID("imagebbsw")
	for k, stream := range streams {
		file := b.pace(stream, pss[k])
		got, ms, err := brdFModelImageW(file)
		if err != nil {
			t.Fatalf("Image W %s: %v", names[k], err)
		}
		if want := stream; !bytes.Equal(got, want) && !bytes.Equal(got, append(bytes.Clone(want), 0x0D)) {
			t.Fatalf("Image W %s: the terminal gets other bytes", names[k])
		}
		asked := 0
		for _, p := range b.waited(pss[k]) {
			asked += p.ms
		}
		if ms != asked {
			t.Fatalf("Image W %s: waits %d ms, %d asked", names[k], ms, asked)
		}
	}
}

// Saving a frame strip for FAME: the status line's time and the cost are the
// whole seconds the board waits.
func TestBrdWholeSecondsSaved(t *testing.T) {
	a := testApp(t, "pc")
	randomPage(a, rand.New(rand.NewSource(2)), 80, 25)
	docs := []*Doc{a.doc.clone(), a.doc.clone(), a.doc.clone()}
	docs[1].cells[0] ^= 1
	a.frames = []Frame{{doc: docs[0], hold: 300}, {doc: docs[1], hold: 2400}, {doc: docs[2], hold: 60}}
	prefs.MyBBS = "fame"
	defer func() { prefs.MyBBS = "" }()
	var ps []pause
	out, err := a.encodeFor("ansi", "fame", &ps)
	if err != nil || out[0] != '~' {
		t.Fatalf("FAME save: %v %q", err, out[:min(10, len(out))])
	}
	total := 0
	for _, p := range ps {
		total += p.ms
	}
	if total != 3000 { // 300 ms -> 1 s, 2.4 s -> 2 s
		t.Fatalf("FAME save waits %d ms, want 3000", total)
	}
	if c := a.costOf("ansi", nil, "fame"); c.pauseMs != 3000 {
		t.Fatalf("cost: %d ms of pauses", c.pauseMs)
	}
	if n := pacedNote("fame", ps); n != "; FAME (whole seconds) pauses, 3.0 s in all" {
		t.Fatalf("note %q", n)
	}
	if w := a.playPauses(a.playBytes()); len(w) != 2 || w[0].ms != 1000 || w[1].ms != 2000 {
		t.Fatalf("Play waits %v", w)
	}
}

// A page in a board's colour codes with a draw order or an effect is saved as
// its animation in those codes: read back with the codes it is the page the
// plain ANSI animation gives, and the colours are codes, not SGRs, wherever
// the kind has a code for every colour.
func TestBrdCodesAnimated(t *testing.T) {
	r := rand.New(rand.NewSource(17))
	for _, kind := range bbsCodeIDs {
		for _, o := range []*drawOrder{{Pattern: "scramble"}, {Pattern: "matrix", Frames: 8, Seed: 3}, {Pattern: "fadein"}} {
			a := testApp(t, "pc")
			a.doc = bbsFPage(r, kind, 40, 6)
			for i, v := range a.doc.cells {
				if kind == "wwiv" && cellCh(v) == 3 {
					a.doc.cells[i] = v&^255 | ' ' // a heart is always a code to WWIV
				}
				if kind == "pipe" || kind == "celerity" {
					a.doc.cells[i] = a.doc.cells[i] &^ (0x80 << 8) // no blink: every colour has a code
				}
			}
			a.saveFmt, a.codes, a.order = kind, kind, o
			out, err := a.encodeFor(kind, "", nil)
			if err != nil {
				t.Fatal(err)
			}
			plain, _ := a.encodeFor("ansi", "", nil)
			if bytes.Equal(out, plain) {
				t.Fatalf("%s %s: written as ANSI", kind, o)
			}
			for i := range out {
				if n, _, _ := sgrAt(out, i); n > 0 {
					t.Fatalf("%s %s: an SGR is left at %d: %q", kind, o, i, out[i:i+n])
				}
			}
			want, _, _ := parseArt(plain, ParseOpts{forceCols: 40})
			got, _, _ := parseArt(out, ParseOpts{codes: kind, forceCols: 40})
			bbsFSame(t, kind+" "+o.String(), want, got)
			if !a.animatedSave(kind) {
				t.Fatalf("%s: not counted as an animated save", kind)
			}
		}
	}
}

// With a board: Mystic's pipe codes and Synchronet's Ctrl-A through the
// boards' models (the same bytes as an .ans saved for them), and the pauses
// reach the save of a pipe-code file for Ami-Express too.
func TestBrdCodesAnimatedForBoard(t *testing.T) {
	r := rand.New(rand.NewSource(5))
	for _, k := range [][2]string{{"pipe", "mystic"}, {"ctrla", "synchronet"}} {
		a := testApp(t, "pc")
		a.doc = bbsFPage(r, k[0], 40, 6)
		a.order = &drawOrder{Pattern: "top", Every: 9, Pause: 600}
		stream, ps := a.writeOrderedPaced("ansi", a.doc, false)
		b := boardByID(k[1])
		if !bytes.Equal(b.paceIn(k[0], stream, ps), b.pace(stream, ps)) {
			t.Fatalf("%s on %s: not the board's own file", k[0], k[1])
		}
		var got []pause
		a.saveFmt = k[0]
		out, _ := a.encodeFor(k[0], k[1], &got)
		if _, _, err := boardModels[k[1]](out); err != nil || len(got) != len(ps) {
			t.Fatalf("%s on %s: %v (%d pauses)", k[0], k[1], err, len(got))
		}
	}
	a := testApp(t, "pc")
	a.doc = bbsFPage(r, "pipe", 40, 6)
	a.order = &drawOrder{Pattern: "top", Every: 9, Pause: 600}
	var ps []pause
	out, _ := a.encodeFor("pipe", "amiexpress", &ps)
	if out[0] != '~' || len(ps) == 0 || !bytes.Contains(out, []byte("w|")) {
		t.Fatalf("pipe file for Ami-Express: %q", out[:min(30, len(out))])
	}
	got, _, _ := parseArt(out, ParseOpts{codes: "pipe", forceCols: 40})
	if got == nil {
		t.Fatal("not readable")
	}
}

// Animate a folder: files in BBS colour codes count as ANSI and are written
// in their own codes, keeping their names, with the board's pauses.
func TestBrdBatchKeepsCodes(t *testing.T) {
	testApp(t, "pc")
	src := t.TempDir()
	r := rand.New(rand.NewSource(8))
	pages := map[string]*Doc{}
	for _, k := range [][2]string{{"pipe", "menu.ans"}, {"ctrla", "logon.msg"}, {"wildcat", "news.bbs"}} {
		d := bbsFPage(r, k[0], 80, 8)
		for i := range d.cells {
			d.cells[i] &^= 0x80 << 8
			if c := cellCh(d.cells[i]); c < 32 || c == '|' || c == '@' {
				d.cells[i] = d.cells[i]&^255 | '#'
			}
		}
		out, _ := writeBBSCodes(k[0], d, false, false)
		os.WriteFile(filepath.Join(src, k[1]), out, 0o644)
		pages[k[1]] = d
	}
	for _, board := range []string{"none", "mystic"} {
		out := filepath.Join(t.TempDir(), board)
		run := runBatch(t, batchSpec{Inputs: []string{src}, Effect: "random", Seed: 4, Board: board, Variants: 1, Out: out,
			Types: map[string]bool{"ansi": true}})
		if done, skipped, failed := run.counts(); done != 3 || skipped+failed != 0 {
			t.Fatalf("%s: %d done, %d skipped, %d failed\n%s", board, done, skipped, failed, run.reportText())
		}
		for _, it := range run.items {
			kind := map[string]string{"menu.ans": "pipe", "logon.msg": "ctrla", "news.bbs": "wildcat"}[it.rel]
			if len(it.outs) != 1 || it.outs[0] != it.rel || !strings.Contains(it.note, bbsCodeName[kind]) {
				t.Fatalf("%s %s: outs %v note %q", board, it.rel, it.outs, it.note)
			}
			data, _ := os.ReadFile(filepath.Join(out, it.outs[0]))
			for i := range data {
				if n, _, _ := sgrAt(data, i); n > 0 {
					t.Fatalf("%s %s: an SGR in a %s file", board, it.rel, kind)
				}
			}
			opts := ParseOpts{codes: kind, forceCols: 80}
			if board == "mystic" && kind == "pipe" { // what Mystic sends (its |PO and |DE are no colours)
				var err error
				if data, _, err = modelMystic(data); err != nil {
					t.Fatalf("Mystic: %v", err)
				}
				opts.codes = ""
			}
			if board == "mystic" && kind != "pipe" {
				continue // Mystic's own escapes (|PI) are for Mystic, which reads no Ctrl-A or @ codes
			}
			got, _, _ := parseArt(data, opts)
			if ok, why := cellsMatch("ansi", pages[it.rel], got); !ok { // (a space's own colour does not show)
				t.Fatalf("%s %s: %s", board, it.rel, why)
			}
		}
	}
}

// PCBoard: a file with both @X00 and @XFF keeps and brings back the colour
// with them; alone each is a colour (black on black, bright white on white),
// as art editors wrote them. A save never makes the pair of a page's colours.
func TestBrdPCBoardSaveRestore(t *testing.T) {
	d := parsePCBoard([]byte("@XFFa@X1Fb@X00c@X4Ed@XFFe@X00@X2Af@XffG"), 80)
	want := []int{0x07, 0x1F, 0x1F, 0x4E, 0x1F, 0x2A, 0x1F}
	for x, w := range want {
		if a := bbsFAttr(d, x, 0); a != w {
			t.Fatalf("pair, cell %d (%c): %02X, want %02X", x, cellCh(d.get(x, 0)), a, w)
		}
	}
	for _, c := range []struct {
		src  string
		want []int
	}{{"@X1Fa@X00b@X4Ec", []int{0x1F, 0x00, 0x4E}}, {"@X1Fa@XFFb@X4Ec", []int{0x1F, 0xFF, 0x4E}}} {
		d := parsePCBoard([]byte(c.src), 80)
		for x, w := range c.want {
			if a := bbsFAttr(d, x, 0); a != w {
				t.Fatalf("%s: cell %d %02X, want %02X", c.src, x, a, w)
			}
		}
		out := writePCBoard(d, false) // opened and saved: reads the same
		if back := parsePCBoard(out, 80); bbsFAttr(back, 1, 0) != c.want[1] || pcbLoss(d) != "" {
			t.Fatalf("%s saved as %q", c.src, out)
		}
	}
	p := newDoc(80, 1)
	p.set(0, 0, attrCell('x', 0x00))
	p.set(1, 0, attrCell('y', 0xFF))
	p.set(2, 0, attrCell('z', 0x1E))
	out := writePCBoard(p, false)
	if pcbSavesColour(out) {
		t.Fatalf("written with the pair: %q", out)
	}
	back := parsePCBoard(out, 80)
	if bbsFAttr(back, 0, 0) != 0x00 || bbsFAttr(back, 1, 0) != 0x7F || bbsFAttr(back, 2, 0) != 0x1E || row(back, 0) != "xyz" {
		t.Fatalf("read back %q %02X %02X %02X", row(back, 0), bbsFAttr(back, 0, 0), bbsFAttr(back, 1, 0), bbsFAttr(back, 2, 0))
	}
	if n := pcbLoss(p); !strings.Contains(n, "1 bright white") {
		t.Fatalf("loss %q", n)
	}
	// An animated file for the PCBoard board (which always saves and restores
	// with them): a stream with black on black or bright white on white keeps ANSI.
	for _, s := range []string{"\x1b[0;30;40mx", "\x1b[0;1;5;37;47mx"} {
		if boardColourer("pcb", []byte(s)) != nil {
			t.Fatalf("%q converted to @X", s)
		}
	}
	if boardColourer("pcb", []byte("\x1b[0;1;33;44mx")) == nil {
		t.Fatal("yellow on blue not converted")
	}
}

// Telegard's `XY codes: read, written, round-tripped with backticks and hex
// digits in the art, found in files, and its ~ codes kept as text.
func TestBrdTelegard(t *testing.T) {
	d := bbsFRead("telegard", "`0EHi`1F~UN`x1`7")
	if row(d, 0) != "Hi~UN`x1`7" || bbsFAttr(d, 0, 0) != 0x0E || bbsFAttr(d, 2, 0) != 0x1F {
		t.Fatalf("read %q %02X %02X", row(d, 0), bbsFAttr(d, 0, 0), bbsFAttr(d, 2, 0))
	}
	r := rand.New(rand.NewSource(9))
	for n := 0; n < 300; n++ {
		cols := 1 + r.Intn(90)
		p := newDoc(cols, 3)
		for i := range p.cells[:cols*3] {
			ch := "``0123456789ABCDEFab ~#"[r.Intn(23)]
			p.cells[i] = attrCell(ch, byte(r.Intn(256)))
		}
		out, _ := writeBBSCodes("telegard", p, n%2 == 0, false)
		back, _, _ := parseArt(out, ParseOpts{codes: "telegard", forceCols: cols})
		bbsFSame(t, "telegard", p, back)
	}
	if k := bbsCodesKind([]byte("`0F Welcome `0Eto `0Fthe `09board `07\r\n`0FCall `0Eagain `07soon\r\n"), "logon.msg"); k != "telegard" {
		t.Fatalf("detected as %q", k)
	}
	if k := bbsCodesKind([]byte("some `code` and `0E once `x"), "readme.txt"); k != "" {
		t.Fatalf("text detected as %q", k)
	}
	if dir := os.Getenv("CHARCOAL_TELEGARD_FILES"); dir != "" { // e.g. the tgarts pack's CUSTOM.ZIP unpacked
		files, _ := filepath.Glob(filepath.Join(dir, "*.MSG"))
		for _, f := range files {
			b, _ := os.ReadFile(f)
			if k := bbsCodesKind(b, filepath.Base(f)); k != "telegard" {
				t.Errorf("%s: detected as %q", f, k)
			}
			d, _, _ := parseArt(b, ParseOpts{codes: "telegard"})
			out, _ := writeBBSCodes("telegard", d, false, false)
			back, _, _ := parseArt(out, ParseOpts{codes: "telegard", forceCols: d.cols})
			bbsFSame(t, f, d, back)
		}
		t.Logf("%d Telegard files", len(files))
	}
}

// Mystic's |T0-|T9 and Renegade's ^0-^9 show the boards' default colours; a
// save writes a theme code again only for the cells drawn in it, and a file
// with |T2 and an explicit |11 of the same colour saves byte for byte.
func TestBrdThemeColours(t *testing.T) {
	d := bbsFRead("pipe", "|T1a^8b|07c|T9d|PI")
	if row(d, 0) != "a^8bcd|" || bbsFAttr(d, 0, 0) != 0x09 || bbsFAttr(d, 4, 0) != 0x07 || bbsFAttr(d, 5, 0) != 0x1F {
		t.Fatalf("pipe: %q %02X %02X %02X", row(d, 0), bbsFAttr(d, 0, 0), bbsFAttr(d, 4, 0), bbsFAttr(d, 5, 0))
	}
	d = bbsFRead("renegade", "^8a|07b^xc|T1d|32e|PI")
	if row(d, 0) != "ab^xc|T1de|PI" || bbsFAttr(d, 0, 0) != 0x8E || bbsFAttr(d, 1, 0) != 0x87 {
		t.Fatalf("renegade: %q %02X %02X", row(d, 0), bbsFAttr(d, 0, 0), bbsFAttr(d, 1, 0))
	}
	if k := bbsCodesKind([]byte("^1Welcome |07to ^2the |15board |07now ^3x |11y\r\n"), "x.ans"); k == "renegade" {
		t.Fatal("Renegade codes guessed")
	}
	a := testApp(t, "pc")
	dir := t.TempDir()
	for _, c := range []struct{ kind, src string }{
		{"pipe", "|T2Welcome |11to |T2the board\r\n|T5x|15y\r\n|07"},
		{"renegade", "^3Welcome |11to ^3the board\r\n^0x|15y\r\n|07"},
	} {
		p := filepath.Join(dir, c.kind+".ans")
		os.WriteFile(p, []byte(c.src), 0o644)
		a.openPath(p, c.kind)
		if a.codes != c.kind || a.saveFmt != c.kind || !strings.Contains(a.message, "default colours") {
			t.Fatalf("%s opened as %q/%q: %s", c.kind, a.codes, a.saveFmt, a.message)
		}
		if bbsFAttr(a.doc, 0, 0) != 0x0B || bbsFAttr(a.doc, 8, 0) != 0x0B {
			t.Fatalf("%s colours %02X %02X", c.kind, bbsFAttr(a.doc, 0, 0), bbsFAttr(a.doc, 8, 0))
		}
		a.dirty = true
		if !a.writeOut(p, a.saveFmt) {
			t.Fatal("save failed")
		}
		if b, _ := os.ReadFile(p); string(b) != c.src {
			t.Fatalf("%s saved as\n%q, was\n%q", c.kind, b, c.src)
		}
		// A cell given another colour loses its theme code; the rest keep theirs.
		a.openPath(p, c.kind)
		a.doc.set(0, 0, attrCell('W', 0x0C))
		a.dirty = true
		a.writeOut(p, a.saveFmt)
		b, _ := os.ReadFile(p)
		if !strings.HasPrefix(string(b), "|12|16W") || strings.Count(string(b), c.src[:2]) != strings.Count(c.src, c.src[:2]) {
			t.Fatalf("%s after an edit: %q", c.kind, b)
		}
	}
	// Renegade's own characters in art are kept apart from codes.
	q := newDoc(12, 1)
	for i, c := range []byte("^5 ^x |12 |x") {
		q.set(i, 0, attrCell(c, 0x1E))
	}
	out, _ := writeBBSCodes("renegade", q, false, false)
	back, _, _ := parseArt(out, ParseOpts{codes: "renegade", forceCols: 12})
	bbsFSame(t, "renegade art", q, back)
}

// The DirArt header line: the page's first row is the disk's name, read
// reversed (as LIST shows it), written to the BAM, kept byte for byte when
// unchanged, and a disk's entries come after it.
func TestBrdDirArtHeader(t *testing.T) {
	page := chsFDirArtPage(11, 6)
	for x, r := range "MY  HEADER  LINE" {
		page.set(x, 0, makeCell(petsciiToScreen(byte(r))|0x80, 1, 0, 0))
	}
	page.set(3, 0, makeCell(int('A'&0x3F), 1, 0, 0)) // not reversed: shows reversed
	img, note := writeDirArtPage(page, nil, "x")
	if note.plain != 1 || !strings.Contains(note.String(), "1 characters of the first row") {
		t.Fatalf("note %+v %q", note, note.String())
	}
	if got := string(dirartHeadName(img)); got != "MY A"+"HEADER  LINE" {
		t.Fatalf("header %q", got)
	}
	back, err := readDirArtPage(img)
	if err != nil || back.rows != 6 {
		t.Fatalf("read back: %v %d rows", err, back.rows)
	}
	page.set(3, 0, makeCell(int('A'&0x3F)|0x80, 1, 0, 0))
	if s := chsFSameCodes(page, back); s != "" {
		t.Fatal(s)
	}
	ents, _, _, _ := dirartEntries(img)
	if len(ents) != 5 {
		t.Fatalf("%d entries, want 5", len(ents))
	}
	// A disk's own header bytes ($A0 padding) stay when the row is unchanged.
	src := writeD64Empty("short")
	head := dirartHeadName(src)
	if head[5] != 0xA0 {
		t.Fatalf("padding %02x", head[5])
	}
	d, _ := readDirArtPage(src)
	img2, _ := writeDirArtPage(d, src, "x")
	if !bytes.Equal(dirartHeadName(img2), head) {
		t.Fatalf("header changed: %q", dirartHeadName(img2))
	}
	// A page of just the header row writes a disk.
	one := newDoc(16, 1)
	one.set(0, 0, makeCell(0x81, 1, 0, 0))
	if img3, _ := writeDirArtPage(one, nil, "x"); dirartHeadName(img3)[0] != 'A' {
		t.Fatalf("one-row page: %q", dirartHeadName(img3))
	}
}

// TestEmuBrdDirArtHeader: LIST in VICE shows the page's first row as the
// header line (CHARCOAL_EMU=1).
func TestEmuBrdDirArtHeader(t *testing.T) {
	s := emuFStart(t)
	x64 := emuFNeed(t, "x64sc")
	a := c64xFApp(t)
	page := chsFDirArtPage(12, 7)
	for x, r := range "CHAREDIT  HEADER" {
		page.set(x, 0, makeCell(petsciiToScreen(byte(r))|0x80, 1, 0, 0))
	}
	for x, r := range "FIRST ENTRY ROW!" {
		page.set(x, 1, makeCell(petsciiToScreen(byte(r)), 1, 0, 0))
	}
	img, _ := writeDirArtPage(page, nil, "dirart test")
	dir := t.TempDir()
	disk := filepath.Join(dir, "art.d64")
	os.WriteFile(disk, img, 0o644)
	want := newDoc(40, page.rows+1)
	want.bg, want.border, want.glyph0 = 6, 14, true
	for i := range want.cells {
		want.cells[i] = makeCell(32, 14, 0, 0)
	}
	put := func(x, y int, text string, rev bool) int {
		for _, r := range []byte(text) {
			sc := petsciiToScreen(r)
			if rev {
				sc |= 0x80
			}
			want.set(x, y, makeCell(sc, 14, 0, 0))
			x++
		}
		return x
	}
	x := put(put(0, 0, "0 ", false), 0, "\"", true)
	for i := 0; i < 16; i++ { // the page's first row, as it is
		want.set(x+i, 0, makeCell(cellCh(page.get(i, 0)), 14, 0, 0))
	}
	put(x+16, 0, "\" CE 2A", true)
	for y := 1; y < page.rows; y++ {
		x := put(0, y, "0    \"", false)
		for i := 0; i < 16; i++ {
			want.set(x+i, y, makeCell(cellCh(page.get(i, y)), 14, 0, 0))
		}
		put(x+16, y, "\" DEL", false)
	}
	put(0, page.rows, "664 BLOCKS FREE.", false)
	a.doc = want
	fb := a.renderDoc(want, 1)
	bx, top, _ := a.borderSize(40*8, max(want.rows, 25)*8, 1)
	c := emuFCase{name: "c64-dirart-header", want: fb, x0: bx, y0: top, w: 40 * 8, h: (page.rows + 1) * 8,
		sxN: 1, sxD: 1, syN: 1, syD: 1, cellW: 8, cellH: 8}
	home := t.TempDir()
	shot := filepath.Join(home, "shot.png")
	out := emuFRun(t, 120, home, emuFEnv(home), x64, "-default", "-console", "-warp", "+sound", "-8", disk,
		"-keybuf", `load"$",8\nlist\n`, "-limitcycles", "40000000", "-exitscreenshot", shot)
	if _, err := os.Stat(shot); err != nil {
		t.Fatalf("x64sc gave no screenshot:\n%s", out)
	}
	emuFCheck(t, s, c, emuFReadPNG(t, shot))
}

// A DirArt page with no header row (Marq's DIRART .c, a new page) saves every
// row as a file, the disk named after the file, also on a second save over
// that disk; opened from the disk, the header line comes first.
func TestBrdDirArtNoHeader(t *testing.T) {
	marq := []byte("unsigned char frame0000[]={\n" + strings.Repeat("1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,\n", 3) + "};\n// META: 16 3 DIRART upper\n")
	m := c64xFApp(t)
	m.loadBytes(marq, "x.c", "", "auto")
	if !m.dirart() || m.dirartHead {
		t.Fatalf("Marq's DirArt: %s head %v", m.machine, m.dirartHead)
	}
	art := m.doc.clone()
	p := filepath.Join(t.TempDir(), "art.d64")
	for range 2 { // the second save goes over the disk the first wrote
		m.dirty = true
		if !m.writeOut(p, "dirart") {
			t.Fatal(m.message)
		}
		img, _ := os.ReadFile(p)
		if ents, _, _, _ := dirartEntries(img); len(ents) != 3 {
			t.Fatalf("%d entries, want 3", len(ents))
		}
		if !strings.HasPrefix(string(dirartHeadName(img)), "ART") {
			t.Fatalf("disk name %q", dirartHeadName(img))
		}
	}
	b := chsFOpen(t, p)
	if !b.dirartHead || b.doc.rows != 4 || cellCh(b.doc.get(0, 0)) != 0x81 { // reversed A of "ART"
		t.Fatalf("reopened: head %v, %d rows, %d", b.dirartHead, b.doc.rows, cellCh(b.doc.get(0, 0)))
	}
	for y := 0; y < 3; y++ {
		for x := 0; x < 16; x++ {
			if cellCh(b.doc.get(x, y+1)) != cellCh(art.get(x, y)) {
				t.Fatalf("art row %d changed", y)
			}
		}
	}
}
