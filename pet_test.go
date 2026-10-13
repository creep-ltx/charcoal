package main

import (
	"crypto/sha1"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

// sameSeqCells: the pages agree, a space's colour not counted (a space shows none).
func sameSeqCells(t *testing.T, what string, a, b *Doc) {
	t.Helper()
	for y := 0; y < a.rows; y++ {
		for x := 0; x < a.cols; x++ {
			v, w := a.get(x, y), b.get(x, y)
			if cellCh(v) == 0x20 {
				v, w = v&255, w&255
			}
			if v != w {
				t.Fatalf("%s: cell %d,%d: %x became %x", what, x, y, v, w)
			}
		}
	}
}

// Saved .seq files end without a RETURN (it would scroll the C64's screen), yet
// every row comes back, empty ones at the end included (audit #10).
func TestSeqHeight(t *testing.T) {
	for _, tc := range []struct {
		rows   int
		filled []int
		full   int
	}{
		{3, []int{0}, -1}, {5, []int{0, 2}, -1}, {4, []int{3}, -1}, {25, []int{0, 24}, -1},
		{2, []int{}, -1}, {1, []int{}, -1}, {3, []int{0}, 2}, {3, []int{0}, 1},
	} {
		d := newDoc(40, tc.rows)
		d.glyph0 = true
		for _, y := range tc.filled {
			d.set(0, y, makeCell(1+y, 2, 0, 0))
			d.set(7, y, makeCell(0xA0, 5, 0, 0))
		}
		if tc.full >= 0 {
			for x := 0; x < 40; x++ {
				d.set(x, tc.full, makeCell(0x51, 7, 0, 0))
			}
		}
		for _, clear := range []bool{true, false} {
			out := writeSeq(d, false, clear)
			if c := out[len(out)-1]; c == 0x0D || c == 0x8D {
				t.Fatalf("%+v: ends with RETURN", tc)
			}
			back, _ := parseSeq(out, 40)
			if back.rows != tc.rows {
				t.Fatalf("%+v clear=%v: %d rows came back: % x", tc, clear, back.rows, out)
			}
			sameSeqCells(t, "height", d, back)
		}
	}
}

// After an odd number of quotes the C64 prints control codes as glyphs (quote
// mode), so the writer closes it with a second quote and a DEL first (audit #26).
func TestSeqQuoteMode(t *testing.T) {
	d := newDoc(40, 4)
	d.glyph0 = true
	put := func(x, y int, s string, fg int) {
		for i, r := range s {
			d.set(x+i, y, makeCell(petFromRune(r, false), fg, 0, 0))
		}
	}
	put(0, 0, `SAY "HI`, 1)
	put(7, 0, `THERE"`, 2)                     // a colour change inside the quotes
	d.set(10, 0, makeCell(0x80|0x01, 2, 0, 0)) // and reverse
	put(37, 1, `X"`, 3)
	put(39, 1, `Q`, 4) // quote in column 38, a new colour in the last column
	put(0, 2, `"`, 5)
	for x := 1; x < 40; x++ { // a full row with an open quote, then a colour on the next row
		d.set(x, 2, makeCell(0x51, 5, 0, 0))
	}
	put(0, 3, "Z", 6)
	out := writeSeq(d, false, true)
	quote := false
	for i, c := range out {
		switch {
		case c == 0x22:
			quote = !quote
		case c == 0x0D || c == 0x8D:
			quote = false
		case c == 0x14: // DEL acts even in quote mode
		case petToScreen(c) < 0 && quote:
			t.Fatalf("control %02x at %d is printed as a glyph in quote mode: % x", c, i, out)
		}
	}
	back, _ := parseSeq(out, 40)
	if back.rows != 4 {
		t.Fatalf("%d rows", back.rows)
	}
	sameSeqCells(t, "quotes", d, back)
}

// fakeROMs swaps the ROM list and the download places for a test server.
func fakeROMs(t *testing.T, srv *httptest.Server, roms []cbmROM) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	oldROMs, oldURLs := cbmROMs, romURLs
	cbmROMs = roms
	romURLs = func(r cbmROM) []string { return []string{srv.URL + "/" + r.vice} }
	t.Cleanup(func() { cbmROMs, romURLs = oldROMs, oldURLs })
}

func fakeROM(family, name string, fill byte) (cbmROM, []byte) {
	b := make([]byte, 4096)
	for i := range b {
		b[i] = fill + byte(i)
	}
	sum := sha1.Sum(b)
	return cbmROM{family, name, family + ".bin", 4096, hex.EncodeToString(sum[:]), family + "-chargen.bin", nil}, b
}

// A download that gets only some ROMs says which are missing; ROMs already here
// are not fetched again; the error tells what really went wrong (audit #22).
func TestDownloadChargen(t *testing.T) {
	good, goodBytes := fakeROM("zzgood", "Good", 1)
	gone, _ := fakeROM("zzgone", "Gone", 2)
	cut, _ := fakeROM("zzcut", "Cut", 3)
	bad, _ := fakeROM("zzbad", "Bad", 4)
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		switch r.URL.Path {
		case "/zzgood.bin":
			w.Write(goodBytes)
		case "/zzcut.bin": // the body is cut off
			w.Header().Set("Content-Length", "4096")
			w.Write(goodBytes[:100])
		case "/zzbad.bin":
			w.Write(make([]byte, 4096))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	fakeROMs(t, srv, []cbmROM{good, gone, cut, bad})
	err := downloadChargen()
	if err == nil {
		t.Fatal("a download with missing ROMs said nothing")
	}
	msg := err.Error()
	for _, want := range []string{"Gone", "Cut", "Bad", "404"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q does not name %q", msg, want)
		}
	}
	if strings.Contains(msg, "Good") || strings.Contains(msg, "200 OK") {
		t.Errorf("error %q", msg)
	}
	if !hasROM("zzgood") || hasROM("zzgone") {
		t.Fatal("the ROM that came was not loaded")
	}
	if _, err := fetchBody(http.DefaultClient, srv.URL+"/zzcut.bin", 65536); err == nil ||
		!strings.Contains(err.Error(), "unexpected EOF") {
		t.Fatalf("a cut-off body: %v", err)
	}
	// Only the good one again: it is on disk and right, so nothing is fetched.
	fakeROMs(t, srv, []cbmROM{good})
	if err := os.MkdirAll(appDataDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(good.paths()[0], goodBytes, 0o644)
	hits.Store(0)
	if err := downloadChargen(); err != nil || hits.Load() != 0 {
		t.Fatalf("err %v, %d requests for a ROM already here", err, hits.Load())
	}
	// A wrong local copy (right size, wrong SHA-1) is not used.
	os.WriteFile(good.paths()[0], make([]byte, 4096), 0o644)
	if good.onDisk() != nil {
		t.Fatal("a wrong ROM on disk was taken")
	}
}

// dataDir ignores a relative XDG_DATA_HOME, as the XDG rules ask.
func TestDataDir(t *testing.T) {
	if ownFolderSystem {
		t.Skip("the XDG fallback is for Linux")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "relative/dir")
	if got := dataDir(); got != filepath.Join(home, ".local", "share") {
		t.Fatalf("relative XDG_DATA_HOME: %s", got)
	}
	t.Setenv("XDG_DATA_HOME", "/somewhere/data")
	if got := dataDir(); got != "/somewhere/data" {
		t.Fatalf("absolute XDG_DATA_HOME: %s", got)
	}
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("HOME", "")
	if got := dataDir(); !filepath.IsAbs(got) {
		t.Fatalf("no home: %q", got)
	}
}

func TestSeq(t *testing.T) {
	// Clear, white "HI", reverse red block, RETURN, cyan "A" in column 0.
	d, lower := parseSeq([]byte{0x93, 0x05, 'H', 'I', 0x12, 0x1C, 0x20, 0x0D, 0x9F, 'A'}, 40)
	if lower != -1 || cellCh(d.get(0, 0)) != 8 || cellCh(d.get(1, 0)) != 9 || cellFg(d.get(0, 0)) != 1 {
		t.Fatalf("text: %x %x", d.get(0, 0), d.get(1, 0))
	}
	if cellCh(d.get(2, 0)) != 0xA0 || cellFg(d.get(2, 0)) != 2 {
		t.Fatalf("reverse: %x", d.get(2, 0))
	}
	if cellCh(d.get(0, 1)) != 1 || cellFg(d.get(0, 1)) != 3 {
		t.Fatalf("after RETURN (reverse off): %x", d.get(0, 1))
	}
	// Round trip through the writer, every screen code once.
	src := newDoc(40, 8)
	for c := 0; c < 256; c++ {
		src.set(c%40, c/40, makeCell(c, c%16, 0, 0))
	}
	back, _ := parseSeq(writeSeq(src, true, true), 40)
	for y := 0; y < 8; y++ {
		for x := 0; x < 40; x++ {
			v, w := src.get(x, y), back.get(x, y)
			if cellCh(v) == 0x20 { // a space carries no colour
				v, w = v&255, w&255
			}
			if v != w {
				t.Fatalf("round trip %d,%d: %x != %x", x, y, w, v)
			}
		}
	}
	// A menu in mixed case with no set code: the lower case set.
	if _, lower := parseSeq([]byte("\xcdAIN MENU\x0d\xc6ILES \xc7AMES \xcdESSAGES"), 40); lower != 1 {
		t.Fatal("mixed case menu")
	}
	// Typing.
	if petFromRune('a', false) != 1 || petFromRune('A', true) != 0x41 || petFromRune('£', false) != 0x1C {
		t.Fatal("typing")
	}
}

func TestMirror(t *testing.T) {
	vga := fontByID("vga")
	for _, p := range [][3]int{{0xC9, 0xBB, 0}, {0xDA, 0xBF, 0}, {0xDD, 0xDE, 0}, {'(', ')', 0}, {'/', '\\', 0},
		{0xDC, 0xDF, 1}, {0xDA, 0xC0, 1}, {0xC9, 0xC8, 1}} {
		if got := vga.flipped(p[0], p[2] == 1); got != p[1] {
			t.Errorf("flip %02X vertical=%d: got %02X, want %02X", p[0], p[2], got, p[1])
		}
	}
	for _, c := range []int{'A', 'H', 'O', 0xDB, ' '} {
		if got := vga.flipped(c, false); got != c {
			t.Errorf("%02X should stay, got %02X", c, got)
		}
	}
}

func TestMirrorC64(t *testing.T) {
	if !loadC64Fonts() {
		t.Skip("no C64 character ROM here")
	}
	f := fontByID("c64upper")
	for _, p := range [][3]int{{0x70, 0x6E, 0}, {0x6D, 0x7D, 0}, {0x55, 0x49, 0}, {0x4D, 0x4E, 0}, {0x6B, 0x73, 0},
		{0x70, 0x6D, 1}, {0x72, 0x71, 1}, {0x55, 0x4A, 1}} {
		if got := f.flipped(p[0], p[2] == 1); got != p[1] {
			t.Errorf("C64 flip %02X vertical=%d: got %02X, want %02X", p[0], p[2], got, p[1])
		}
	}
}
