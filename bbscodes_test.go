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

// bbsFRead reads s in kind's codes as an 80-column PC page.
func bbsFRead(kind, s string) *Doc {
	d, _, _ := parseArt([]byte(s), ParseOpts{codes: kind})
	return d
}

// bbsFAttr is the PC attribute of cell x, y.
func bbsFAttr(d *Doc, x, y int) int { return int(cellAttr(d.get(x, y))) }

// The boards' tables, each read as its source does.
func TestBBSCodesRead(t *testing.T) {
	for _, c := range []struct {
		kind, in string
		x        int
		want     int
		text     string
	}{
		// Pipe codes: |15|17 is bright white on blue (Synchronet's wiki example).
		{"pipe", "|15|17X", 0, 0x1F, "X"},
		{"pipe", "|17|04X", 0, 0x14, "X"},                   // a foreground keeps the background
		{"pipe", "|17|28|04X", 0, 0xC4, "X"},                // |24-|31: background with bit 7, kept by |04
		{"pipe", "|28|17X", 0, 0x17, "X"},                   // |16-|23 turn bit 7 off
		{"pipe", "a|PIb", 1, 0x07, "a|b"},                   // Mystic's |PI is a |
		{"pipe", "|UN |PO|DE|32", 0, 0x07, "|UN |PO|DE|32"}, // codes it cannot show stay as text
		{"pipe", "x|CLy", 0, 0x07, "y"},                     // |CL clears
		{"pipe", "|1519%", 0, 0x0F, "19%"},                  // two digits only (IcyDraw's Members01.an1)
		{"pipe", "\x1b[1;31m|17X", 0, 0x1C, "X"},            // mixed with ANSI
		// Synchronet Ctrl-A.
		{"ctrla", "\x01H\x01Y\x014X", 0, 0x1E, "X"},
		{"ctrla", "\x01H\x01r\x01N\x01bX", 0, 0x01, "X"},     // N is light grey; letters in any case
		{"ctrla", "\x01I\x01W\x011X", 0, 0xC7, "X"},          // blink kept by a background digit
		{"ctrla", "\x01H\x01+\x01N\x01-X", 0, 0x0F, "X"},     // + pushes, - pops
		{"ctrla", "\x01H\x01-X", 0, 0x07, "X"},               // - with nothing pushed: normal
		{"ctrla", "a\x01Ab\x01zc", 0, 0x07, "a\x01b\x1ac"},   // A and z are characters
		{"ctrla", "a\x01Pb\x01.c", 0, 0x07, "a\x01Pb\x01.c"}, // a pause and a delay stay as text
		{"ctrla", "ab\x01<\x01<X", 0, 0x07, "Xb"},            // < moves left
		{"ctrla", "a\x01\x83b", 0, 0x07, "a    b"},           // over 127: right by n - 127
		{"ctrla", "ab\x01Zcd", 0, 0x07, "ab"},                // Z ends the file
		// WWIV.
		{"wwiv", "\x034X", 0, 0x1F, "X"},        // heart 4: bright white on blue (its default)
		{"wwiv", "\x03xX", 0, 0x07, "X"},        // any other printable: light grey
		{"wwiv", "|#2X|17Y", 0, 0x0E, "XY"},     // |#n as a heart
		{"wwiv", "|#2X|17Y", 1, 0x1E, "XY"},     // |16-|31 keep the foreground
		{"wwiv", "|7X|@UN@", 0, 0x07, "X|@UN@"}, // one digit is enough; macros stay
		{"wwiv", "|25X", 0, 0x97, "X"},          // bit 7 from 24 on
		// Wildcat!
		{"wildcat", "@1F@X@0E@Y", 0, 0x1F, "XY"},
		{"wildcat", "@1F@X@0E@Y", 1, 0x0E, "XY"},
		{"wildcat", "a@CLS@b@1f@", 0, 0x07, "b@1f@"}, // small hex is text
		// Celerity: |b|S|W is bright white on blue (Synchronet's wiki example).
		{"celerity", "|b|S|WX", 0, 0x1F, "X"},
		{"celerity", "|y|dX|Zz", 0, 0x08, "X|Zz"},
	} {
		d := bbsFRead(c.kind, c.in)
		if got := row(d, 0); got != c.text {
			t.Errorf("%s %q: text %q, want %q", c.kind, c.in, got, c.text)
			continue
		}
		if got := bbsFAttr(d, c.x, 0); got != c.want {
			t.Errorf("%s %q: cell %d colours %02X, want %02X", c.kind, c.in, c.x, got, c.want)
		}
	}
}

// bbsFPage is a random page that leans on the characters each writer must
// escape (| @ ☺ digits, the code letters) and every colour.
func bbsFPage(r *rand.Rand, kind string, cols, rows int) *Doc {
	chars := []byte("|@|@\x01\x01\x0112345678900CLPISkbBwW#AQPz.;,ZYz -\xdb\xb0\x0f")
	d := newDoc(cols, rows)
	for i := range d.cells[:cols*rows] {
		ch := chars[r.Intn(len(chars))]
		if r.Intn(4) == 0 {
			ch = byte(32 + r.Intn(224))
		}
		attr := r.Intn(256)
		if kind == "celerity" {
			attr &= 0x7F // no code for bit 7
		}
		if r.Intn(3) > 0 && i > 0 { // runs of one colour, as in art
			attr = int(cellAttr(d.cells[i-1]))
		}
		d.cells[i] = attrCell(ch, byte(attr))
	}
	return d
}

// Every kind gives back the page it wrote, cell for cell, whatever the art
// holds (codes' lead characters, digits, letters next to them).
func TestBBSCodesRoundTrip(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for _, kind := range bbsCodeIDs {
		for n := 0; n < 300; n++ {
			cols, rows := 1+r.Intn(90), 1+r.Intn(6)
			if n%3 == 0 {
				cols = 80
			}
			d := bbsFPage(r, kind, cols, rows)
			for _, clear := range []bool{false, true} {
				out, loss := writeBBSCodes(kind, d, clear, n%2 == 0)
				if loss.blink+loss.hearts != 0 {
					t.Fatalf("%s: lost %+v", kind, loss)
				}
				back, _, _ := parseArt(out, ParseOpts{codes: kind, forceCols: cols})
				bbsFSame(t, kind, d, back)
			}
		}
	}
}

// A board's codes that CharEdit cannot show go back into the file as they came.
func TestBBSCodesKeepUnknown(t *testing.T) {
	for _, c := range []struct{ kind, in, keep string }{
		{"pipe", "|07|16Hi |UN, |PO|DE|AA |99", "|UN, |PO|DE|AA |99"},
		{"ctrla", "\x01N\x01PHi\x01. \x01T\x01\"menu", "\x01PHi\x01. \x01T\x01\"menu"},
		{"wwiv", "|07|16Hi |@UN@ \x0fAB", "|@UN@ \x0fAB"},
		{"wildcat", "@07@Hi @FIRST@ @1f@", "@FIRST@ @1f@"},
		{"celerity", "|w|k|SHi |Z |x", "|Z |x"},
	} {
		d := bbsFRead(c.kind, c.in)
		out, _ := writeBBSCodes(c.kind, d, false, false)
		if !bytes.Contains(out, []byte(c.keep)) {
			t.Errorf("%s: %q written as %q, %q lost", c.kind, c.in, out, c.keep)
		}
	}
}

// The writers use the shortest codes: one per change of foreground or
// background, nothing where the colours stay.
func TestBBSCodesShortest(t *testing.T) {
	d := newDoc(80, 1)
	for i, a := range []int{0x07, 0x07, 0x0F, 0x1F, 0x1F, 0x9F, 0x0E} {
		d.set(i, 0, attrCell('x', byte(a)))
	}
	for kind, want := range map[string]string{
		"pipe":     "|07|16xx|15x|17xx|25x|14|16x\r\n",
		"wwiv":     "|07|16xx|15x|17xx|25x|14|16x\r\n|07",
		"wildcat":  "@07@xx@0F@x@1F@xx@9F@x@0E@x\r\n@07@",
		"ctrla":    "\x01Nxx\x01Hx\x014xx\x01Ix\x01N\x01H\x01Yx\r\n\x01N",
		"celerity": "|k|S|wxx|Wx|b|S|Wxxx|k|S|Yx\r\n|w",
	} {
		out, _ := writeBBSCodes(kind, d, false, false)
		if kind == "pipe" {
			want += "|07"
		}
		if string(out) != want {
			t.Errorf("%s: %q, want %q", kind, out, want)
		}
	}
}

// Celerity has no blink and WWIV no way to write ♥: the save says so.
func TestBBSCodesLoss(t *testing.T) {
	d := newDoc(80, 1)
	d.set(0, 0, attrCell(0x03, 0x9F))
	d.set(1, 0, attrCell('5', 0x9F))
	if _, l := writeBBSCodes("celerity", d, false, false); l.blink != 2 || !strings.Contains(l.note("celerity"), "2 blinking") {
		t.Fatalf("celerity: %+v", l)
	}
	if out, l := writeBBSCodes("wwiv", d, false, false); l.hearts != 1 || bytes.IndexByte(out, 0x03) >= 0 {
		t.Fatalf("wwiv: %+v %q", l, out)
	}
}

// Found by their bytes or ending; text art drawn with | @ and numbers is not.
func TestBBSCodesDetect(t *testing.T) {
	for _, c := range []struct{ name, in, want string }{
		{"a.ans", "|15Hello |07there |15you|07.", "pipe"},
		{"a.ans", "\x1b[0m|15Hi|17 |15x|07 |15y", "pipe"}, // mixed with ANSI (Mystic)
		{"menu.msg", "\x01nHello", "ctrla"},               // one is enough with the ending
		{"menu.asc", "\x01n\x01hHello \x01y\x01bthere\x01n \x01hx\x01n", "ctrla"},
		{"menu.msg", "\x032Hello", "wwiv"},
		{"x.ans", "\x032He\x031llo\x032 x\x033 \x032y", "wwiv"},
		{"x.bbs", "@1F@Hi", "wildcat"},
		{"x.txt", "@1F@Hi @0E@x @1F@y @0E@", "wildcat"},
		// Not codes:
		{"x.txt", ".x|01|x. A  .x|02|x. B  .x|03|x. C", ""}, // numbered boxes
		{"x.txt", "oo|22 ..... oo|23 ..... oo|24 .... oo|22", ""},
		{"x.txt", "g@WW@@@@@@D4@@@@@@@mW@s @4D@AM@ @D4@", ""}, // @ art
		{"x.txt", "|2019|2020|2021|", ""},                     // a table of years
		{"x.txt", "\x01C\x01C\x01C\x01C", ""},                 // ☺C ☺C: one code over and over
		{"x.txt", "\x0304,01Hi \x0304,01x \x0312y", ""},       // mIRC colours
		{"x.txt", "|b|S|W Celerity |b is never guessed", ""},
		{"x.seq", "|15a|07b|15c|07", ""}, // another machine's file
		{"x.gif", "GIF89a|15a|07b|15c|07", ""},
	} {
		if got := bbsCodesKind([]byte(c.in), c.name); got != c.want {
			t.Errorf("%s %q: %q, want %q", c.name, c.in, got, c.want)
		}
	}
}

// CharEdit opens them as PC pages that save back in the same codes; Open as
// picks a kind for any file; Save as another format leaves the codes.
func TestBBSCodesOpenSave(t *testing.T) {
	a := testApp(t, "pc")
	dir := t.TempDir()
	p := filepath.Join(dir, "menu.ans")
	src := "|CL|15|17Main menu|07|16\r\n|11(|15M|11)essages |PO\r\n"
	os.WriteFile(p, []byte(src), 0o644)
	a.openPath(p, "auto")
	if a.mode != "pc" || a.saveFmt != "pipe" || a.codes != "pipe" {
		t.Fatalf("opened as %s / %s / %s", a.mode, a.saveFmt, a.codes)
	}
	if row(a.doc, 0) != "Main menu" || bbsFAttr(a.doc, 0, 0) != 0x1F || row(a.doc, 1) != "(M)essages |PO" {
		t.Fatalf("page: %q %q %02X", row(a.doc, 0), row(a.doc, 1), bbsFAttr(a.doc, 0, 0))
	}
	want := a.doc.clone()
	a.dirty = true
	if !a.writeOut(p, a.saveFmt) {
		t.Fatal("save failed")
	}
	b, _ := os.ReadFile(p)
	if bytes.Contains(b, []byte("\x1b")) || !bytes.Contains(b, []byte("|PO")) {
		t.Fatalf("saved %q", b)
	}
	a.openPath(p, "auto")
	sameCells(t, "reopened", want, a.doc)
	// Save as ANSI: the page's own file is ANSI now, no codes read any more.
	q := filepath.Join(dir, "menu2.ans")
	a.writeOut(q, "ansi")
	if a.codes != "" || a.saveFmt != "ansi" {
		t.Fatalf("after Save as ANSI: %q %q", a.codes, a.saveFmt)
	}
	// Mixed with ANSI escapes: read with the codes, saved in them again.
	m := filepath.Join(dir, "mixed.ans")
	os.WriteFile(m, []byte("\x1b[2J|15|17Main|07|16 |11x|15y |11z|15w"), 0o644)
	a.openPath(m, "auto")
	if a.codes != "pipe" || a.saveFmt != "pipe" || bbsFAttr(a.doc, 0, 0) != 0x1F {
		t.Fatalf("mixed: %q %q %02X", a.codes, a.saveFmt, bbsFAttr(a.doc, 0, 0))
	}
	// Open as: a .txt with Celerity codes.
	r := filepath.Join(dir, "cel.txt")
	os.WriteFile(r, []byte("|b|S|WHi"), 0o644)
	a.openPath(r, "celerity")
	if a.saveFmt != "celerity" || bbsFAttr(a.doc, 0, 0) != 0x1F {
		t.Fatalf("Open as celerity: %s %02X", a.saveFmt, bbsFAttr(a.doc, 0, 0))
	}
	// Ctrl-A E means bright backgrounds.
	s := filepath.Join(dir, "ice.msg")
	os.WriteFile(s, []byte("\x01E\x014Hi"), 0o644)
	a.openPath(s, "auto")
	if !a.ice || a.saveFmt != "ctrla" {
		t.Fatalf("Ctrl-A E: ice %v, %s", a.ice, a.saveFmt)
	}
	// The formats are offered on PC pages only.
	ids := map[string]bool{}
	for _, f := range a.modeFormats() {
		ids[f.id] = true
	}
	for _, k := range bbsCodeIDs {
		if !ids[k] {
			t.Errorf("%s missing from Save as", k)
		}
	}
}

// bbsFRealDir holds real files when they have been fetched (see the knowledge
// note: Synchronet's text/menu, IcyDraw's benches/data, WWIV's install/gfiles).
func bbsFRealDir(t *testing.T, sub string) string {
	dir := os.Getenv("CHARCOAL_BBS_FILES")
	if dir == "" {
		dir = "/tmp/charcoal-bbs"
	}
	p := filepath.Join(dir, sub)
	if _, err := os.Stat(p); err != nil {
		t.Skip("no real files in " + p)
	}
	return p
}

// IcyDraw's benchmark picture Members01 in PCBoard, Avatar, Renegade (.an1)
// and Ctrl-A (.msg) form: the same page from every reader. IcyDraw writes an
// @ in a Ctrl-A file as @@, which Synchronet shows as @@ (an empty @-code is
// none), so that one cell is let off.
func TestBBSCodesIcyDrawMembers(t *testing.T) {
	dir := bbsFRealDir(t, "src/icy_tools/crates/icy_parser_core/benches/data")
	rd := func(n string) []byte { b, _ := os.ReadFile(filepath.Join(dir, n)); return b }
	pcb := parsePCBoard(rd("Members01.pcb"), 80)
	if ok, why := cellsMatch("ansi", pcb, parseAvatar(rd("Members01.avt"), 80)); !ok {
		t.Fatal("avt:", why)
	}
	an1, _, _ := parseArt(rd("Members01.an1"), ParseOpts{codes: bbsCodesKind(rd("Members01.an1"), "Members01.an1")})
	if ok, why := cellsMatch("ansi", pcb, an1); !ok {
		t.Fatal("an1:", why)
	}
	msgBytes := bytes.ReplaceAll(rd("Members01.msg"), []byte("@@"), []byte("@"))
	msg, _, _ := parseArt(msgBytes, ParseOpts{codes: bbsCodesKind(rd("Members01.msg"), "Members01.msg")})
	if ok, why := cellsMatch("ansi", pcb, msg); !ok {
		t.Fatal("msg:", why)
	}
}

// Synchronet's own menus (text/menu, every board's look in Ctrl-A codes) and
// WWIV's gfiles: each is found, read with no code left over, and written back
// to the same page.
func TestBBSCodesRealFiles(t *testing.T) {
	n := 0
	for _, sub := range []string{"sbbs-menu", "src/wwiv/install/gfiles"} {
		dir := bbsFRealDir(t, sub)
		filepath.Walk(dir, func(p string, fi os.FileInfo, err error) error {
			if err != nil || fi.IsDir() || !bbsFMenuExt.MatchString(p) {
				return nil
			}
			b, _ := os.ReadFile(p)
			kind := bbsCodesKind(b, filepath.Base(p))
			if kind == "" {
				return nil
			}
			n++
			d, _, _ := parseArt(b, ParseOpts{codes: kind})
			out, _ := writeBBSCodes(kind, d, false, false)
			back, _, _ := parseArt(out, ParseOpts{codes: kind, forceCols: d.cols})
			bbsFSame(t, p, d, back)
			return nil
		})
	}
	if n < 50 {
		t.Fatalf("only %d files with codes", n)
	}
}

var bbsFMenuExt = regexp.MustCompile(`(?i)\.(msg|asc|ans)$`)

// bbsFSame: every cell of want is in got (characters and colours; empty
// cells at the ends of rows, which no writer sends, are let off).
func bbsFSame(t *testing.T, what string, want, got *Doc) {
	t.Helper()
	if want.cols != got.cols || max(want.usedRows(), got.usedRows()) > want.rows {
		t.Fatalf("%s: %dx%d read back as %dx%d", what, want.cols, want.rows, got.cols, got.rows)
	}
	for y := 0; y < want.rows; y++ {
		for x := 0; x < want.cols; x++ {
			a, b := want.get(x, y), got.get(x, y)
			if isBlank(a) && isBlank(b) {
				continue
			}
			if cellCh(a) != cellCh(b) || cellAttr(a) != cellAttr(b) {
				t.Fatalf("%s: cell %d,%d %08x read back as %08x", what, x, y, a, b)
			}
		}
	}
}

// No file of the big check is taken for codes except the WWIV ANSIs that hold hearts.
func TestBBSCodesBigCheck(t *testing.T) {
	if _, err := os.Stat("/tmp/bigcheck/files"); err != nil {
		t.Skip("no /tmp/bigcheck")
	}
	filepath.Walk("/tmp/bigcheck/files", func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() || fi.Size() > 4<<20 {
			return nil
		}
		b, _ := os.ReadFile(p)
		if k := bbsCodesKind(b, filepath.Base(p)); k != "" && !(k == "wwiv" && strings.HasPrefix(filepath.Base(p), "WWANS")) {
			t.Errorf("%s taken for %s", p, k)
		}
		return nil
	})
}

// bbsFMystic is Mystic 1.10's OutFile for the codes the pipe writer uses
// (bbs_io.pas 1329-1335, ParseMCI, Pipe2Ansi): "|" takes the next two bytes
// whatever they are; |00-|15 set the foreground, |16-|23 the background, |PI
// prints "|", |CL clears; any other two are printed with the "|". It gives
// the ANSI stream Mystic sends.
func bbsFMystic(b []byte) []byte {
	var out []byte
	fg, bg := 7, 0
	sgr := func() {
		v := biosANSI[fg]
		bold := 0
		if v > 7 {
			bold = 1
		}
		out = fmt.Appendf(out, "\x1b[0;%d;3%d;4%dm", bold, v&7, biosANSI[bg])
	}
	for i := 0; i < len(b); i++ {
		if b[i] != '|' || i+2 >= len(b) {
			out = append(out, b[i])
			continue
		}
		code := string(b[i+1 : i+3])
		i += 2
		switch v := int(code[0]-'0')*10 + int(code[1]-'0'); {
		case code == "PI":
			out = append(out, '|')
		case code == "CL":
			out = append(out, "\x1b[2J\x1b[H"...)
		case isDigit(code[0]) && isDigit(code[1]) && v < 16:
			fg = v
			sgr()
		case isDigit(code[0]) && isDigit(code[1]) && v < 24:
			bg = v - 16
			sgr()
		default:
			out = append(out, '|')
			out = append(out, code...)
		}
	}
	return out
}

// What the pipe writer saves shows on Mystic as the page (bit 7 aside:
// Mystic 1.10 has no code for it).
func TestBBSCodesOnMystic(t *testing.T) {
	r := rand.New(rand.NewSource(7))
	for n := 0; n < 400; n++ {
		cols, rows := 1+r.Intn(90), 1+r.Intn(5)
		d := bbsFPage(r, "celerity", cols, rows) // no bit 7
		out, _ := writeBBSCodes("pipe", d, n%2 == 0, false)
		back, _, _ := parseArt(bbsFMystic(out), ParseOpts{forceCols: cols})
		bbsFSame(t, "mystic", d, back)
	}
}

// A pipe-code save says how many cells need |24-|31, which Mystic 1.10 and
// Renegade do not show (bright backgrounds in iCE, blinking characters).
func TestBBSFPipeBrightNote(t *testing.T) {
	d := newDoc(4, 1)
	d.set(0, 0, makeCell('A', 7, 9, 0))  // bright blue background (iCE) / blinking A
	d.set(1, 0, makeCell(' ', 7, 9, 0))  // the same on a space
	d.set(2, 0, makeCell('B', 15, 1, 0)) // no bit 7
	if _, l := writeBBSCodes("pipe", d, false, true); l.bright != 2 || l.note("pipe") == "" {
		t.Fatalf("iCE: %d cells", l.bright)
	}
	if _, l := writeBBSCodes("pipe", d, false, false); l.bright != 1 {
		t.Fatalf("blink: %d cells (a blinking space shows nothing)", l.bright)
	}
	if _, l := writeBBSCodes("ctrla", d, false, true); l.bright != 0 {
		t.Fatal("only pipe codes count them")
	}
}

// A pipe-code file with two screens splits into two frames read with its codes
// (not shown as text), and saves back as frames, each after the board's clear.
func TestBBSFSplitFrames(t *testing.T) {
	src := []byte("|15Hello|07 one\r\n|CL|12Bye|07 two\r\n")
	docs, _, _ := splitStream(src, "pc", 80, false, STRes{}, false, 0, "pipe")
	if len(docs) != 2 || row(docs[0], 0) != "Hello one" || row(docs[1], 0) != "Bye two" || bbsFAttr(docs[1], 0, 0) != 0x0C {
		t.Fatalf("%d frames: %q", len(docs), func() []string {
			var r []string
			for _, d := range docs {
				r = append(r, row(d, 0))
			}
			return r
		}())
	}
	var out []byte
	for i, d := range docs {
		o, _ := writeBBSCodes("pipe", d, i > 0, false)
		out = append(out, o...)
	}
	back, _, _ := splitStream(out, "pc", 80, false, STRes{}, false, 0, "pipe")
	if len(back) != 2 || row(back[1], 0) != "Bye two" {
		t.Fatalf("saved frames read back as %d", len(back))
	}
}

// A file with ANSI escapes and WWIV hearts that also draws ♥ stays ANSI: WWIV's
// codes cannot hold a ♥. One whose codes hold everything saves back in them.
func TestBBSFMixedKeepsWhatHoldsIt(t *testing.T) {
	dir := t.TempDir()
	cases := []struct{ name, src, want string }{
		{"hearts.ans", "\x1b[0m\x1b[1;31m\x03\x1b[0m love \x033Hi \x035there \x037you \x033a \x035b \x037c\r\n", "ansi"},
		{"mystic.ans", "\x1b[2J|15|17Main|07|16 |11x|15y |11z|15w", "pipe"},
	}
	for _, c := range cases {
		a := testApp(t, "pc")
		p := filepath.Join(dir, c.name)
		os.WriteFile(p, []byte(c.src), 0o644)
		a.openPath(p, "auto")
		if a.codes == "" || a.saveFmt != c.want {
			t.Errorf("%s: codes %q, saves as %q, want %q", c.name, a.codes, a.saveFmt, c.want)
		}
	}
}
