package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A carousel like the real ones: file header lines before the first PN, then
// subpages with PN/SC/FL, an enhancement packet (OL,26), an unknown line,
// codes written as 0x80+code and rows with trailing spaces.
func carouselTTI() []byte {
	var b strings.Builder
	b.WriteString("DE,Description goes here\r\nDS,inserter\r\nSP,C:\\TTI\\CAROUSEL.tti\r\nCT,8,T\r\nPS,8000\r\nRE,0\r\n")
	for i, word := range []string{"FIRST", "SECOND", "THIRD"} {
		b.WriteString("PN,1110" + string(rune('1'+i)) + "\r\nSC,000" + string(rune('1'+i)) + "\r\n")
		if i == 1 {
			b.WriteString("OL,26,@rD@^C@^M\r\nZZ,an unknown line\r\n")
		}
		b.WriteString("OL,0,        P111 header\r\n")
		b.WriteString("OL,1,\x81" + word + "   \r\n")
		b.WriteString("OL,2,\x1bB" + word + "\r\n")
		b.WriteString("FL,100,100,100,100,100,100\r\n")
	}
	return []byte(b.String())
}

func loadTTIFile(t *testing.T, data []byte) (*App, string) {
	a := testApp(t, "pc")
	p := filepath.Join(t.TempDir(), "carousel.tti")
	if err := os.WriteFile(p, data, 0o644); err != nil {
		t.Fatal(err)
	}
	a.loadBytes(data, filepath.Base(p), p, "auto")
	return a, p
}

func ttRowText(d *Doc, y int) string {
	var s []byte
	for x := 0; x < 40; x++ {
		c := cellCh(d.get(x, y)) & 0x7F
		if c < 0x20 {
			c = '.'
		}
		s = append(s, byte(c))
	}
	return strings.TrimRight(string(s), " ")
}

func TestTTISubpagesSplit(t *testing.T) {
	b := carouselTTI()
	segs := ttiSubpages(b)
	if len(segs) != 3 || !bytes.Equal(bytes.Join(segs, nil), b) {
		t.Fatalf("%d subpages, joined equal %v", len(segs), bytes.Equal(bytes.Join(segs, nil), b))
	}
	if !bytes.HasPrefix(segs[0], []byte("DE,")) || !bytes.HasPrefix(segs[2], []byte("PN,11103")) {
		t.Fatal("subpages cut in the wrong place")
	}
	if n := len(ttiSubpages([]byte("OL,1,hi\r\n"))); n != 1 {
		t.Fatalf("a file without PN: %d subpages", n)
	}
}

func TestTTICarouselRoundTrip(t *testing.T) {
	orig := carouselTTI()
	a, p := loadTTIFile(t, orig)
	if len(a.tabs) != 3 {
		t.Fatalf("%d tabs, want one per subpage", len(a.tabs))
	}
	for i, word := range []string{"FIRST", "SECOND", "THIRD"} {
		tab := a.tabs[i]
		if tab.mode != "tt" || tab.srcKind != "tti" || tab.srcIndex != i || tab.path != p {
			t.Fatalf("tab %d: mode %s kind %s index %d path %q", i, tab.mode, tab.srcKind, tab.srcIndex, tab.path)
		}
		if got := ttRowText(tab.doc, 1); got != "."+word {
			t.Fatalf("tab %d row 1 %q", i, got)
		}
	}
	// Saved unchanged, every tab gives the file back byte for byte.
	for _, tab := range a.tabs {
		a.focusTab(tab)
		if !a.writeOut(p, "tti") {
			t.Fatal(a.message)
		}
		if got, _ := os.ReadFile(p); !bytes.Equal(got, orig) {
			t.Fatalf("unchanged save of subpage %d changed the file:\n%q", tab.srcIndex+1, got)
		}
	}
	// Edit subpage 2: only its page rows change.
	a.focusTab(a.tabs[1])
	a.doc.set(0, 5, makeCell('X', 0, 0, 0))
	if !a.writeOut(p, "tti") {
		t.Fatal(a.message)
	}
	if !strings.Contains(a.message, "the other 2 kept") {
		t.Fatalf("status line: %q", a.message)
	}
	got, _ := os.ReadFile(p)
	os0, gs := ttiSubpages(orig), ttiSubpages(got)
	if len(gs) != 3 || !bytes.Equal(gs[0], os0[0]) || !bytes.Equal(gs[2], os0[2]) {
		t.Fatalf("the other subpages changed:\n%q", got)
	}
	var keep []string
	for _, l := range strings.Split(string(gs[1]), "\r\n") {
		if _, ok := ttiPageRow([]byte(l)); !ok {
			keep = append(keep, l)
		}
	}
	want := "PN,11102|SC,0002|OL,26,@rD@^C@^M|ZZ,an unknown line|FL,100,100,100,100,100,100|"
	if strings.Join(keep, "|") != want {
		t.Fatalf("header lines of the edited subpage: %q", strings.Join(keep, "|"))
	}
	if d := parseTTI(gs[1]); ttRowText(d, 5) != "X" || ttRowText(d, 1) != ".SECOND" {
		t.Fatalf("edited subpage reads back %q %q", ttRowText(d, 5), ttRowText(d, 1))
	}
	// Then subpage 3: the edit of subpage 2 stays.
	a.focusTab(a.tabs[2])
	a.doc.set(0, 6, makeCell('Y', 0, 0, 0))
	a.writeOut(p, "tti")
	got, _ = os.ReadFile(p)
	gs = ttiSubpages(got)
	if ttRowText(parseTTI(gs[1]), 5) != "X" || ttRowText(parseTTI(gs[2]), 6) != "Y" || !bytes.Equal(gs[0], os0[0]) {
		t.Fatalf("second save lost the first edit:\n%q", got)
	}
}

func TestTTIOneSubpage(t *testing.T) {
	orig := []byte("DE,Single\nPN,15200\nSC,0000\nPS,8000\nCT,8,T\nOL,28,@@@|gpCu_\nOL,1,\x1bAHELLO\nOL,3,WORLD   \n")
	a, p := loadTTIFile(t, orig)
	if len(a.tabs) != 1 || a.srcKind != "tti" {
		t.Fatalf("%d tabs, kind %q", len(a.tabs), a.srcKind)
	}
	a.writeOut(p, "tti")
	if got, _ := os.ReadFile(p); !bytes.Equal(got, orig) {
		t.Fatalf("unchanged save changed the file:\n%q", got)
	}
	a.doc.set(0, 2, makeCell('Z', 0, 0, 0))
	a.writeOut(p, "tti")
	got, _ := os.ReadFile(p)
	want := "DE,Single\nPN,15200\nSC,0000\nPS,8000\nCT,8,T\nOL,28,@@@|gpCu_\nOL,1,\x1bAHELLO\nOL,2,Z\nOL,3,WORLD\n"
	if string(got) != want {
		t.Fatalf("edited one-subpage file:\n%q\nwant\n%q", got, want)
	}
}

func TestTTINewPage(t *testing.T) {
	a := testApp(t, "tt")
	p := filepath.Join(t.TempDir(), "new.tti")
	a.doc.set(0, 1, makeCell('A', 0, 0, 0))
	if !a.writeOut(p, "tti") {
		t.Fatal(a.message)
	}
	got, _ := os.ReadFile(p)
	if string(got) != "DE,Made with CharEdit\r\nPN,10000\r\nSC,0000\r\nPS,8000\r\nOL,1,A\r\n" {
		t.Fatalf("new page: %q", got)
	}
}

// A .tti row may hold ESC E (magenta) and ESC Y (contiguous graphics), which
// are VT52 commands too: the file must still open as teletext.
func TestTTINotVT52(t *testing.T) {
	a, _ := loadTTIFile(t, []byte("PN,10000\r\nOL,1,\x1bEone\x1bY\r\nOL,2,\x1bEtwo\x1bY\r\n"))
	if a.mode != "tt" || cellCh(a.doc.get(0, 1)) != 0x05 || cellCh(a.doc.get(4, 1)) != 0x19 {
		t.Fatalf("opened as %s", a.mode)
	}
}
