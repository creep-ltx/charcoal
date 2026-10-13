package main

// Atari ST pages: the ST's VT52 terminal (TOS), in its three resolutions. Files
// are the bytes a BBS sends: ESC b / ESC c colours, ESC Y cursor, ESC E clear...
//
// The font is EmuTOS's (the free TOS, GPL): bios/fnt_st_8x8.c and fnt_st_8x16.c,
// downloaded on first use from a fixed EmuTOS version and checked by SHA-1.

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"time"
)

// An ST screen resolution.
type STRes struct {
	id, name   string
	cols, rows int
	colours    int // pens: 16, 4 or 2
	font       string
	palette    string
	fg, bg     int // the colours text starts in
}

var stResolutions = []STRes{
	{"stlow", "ST low (40x25)", 40, 25, 16, "st8x8", "st", 15, 0},
	{"stmed", "ST medium (80x25)", 80, 25, 4, "st8x8", "stmed", 3, 0},
	{"sthigh", "ST high (80x25)", 80, 25, 2, "st8x16", "sthigh", 1, 0},
}

func stResByID(id string) STRes {
	for _, r := range stResolutions {
		if r.id == id {
			return r
		}
	}
	return stResolutions[1]
}

// The ST's colours as TOS sets them, by pen (hardware register) number.
var stPalettes = []*Palette{
	pal("st", "Atari ST (TOS)", 0xFFFFFF, 0xFF0000, 0x00FF00, 0xFFFF00, 0x0000FF, 0xFF00FF, 0x00FFFF, 0xAAAAAA,
		0x555555, 0xFF5555, 0x55FF55, 0xFFFF55, 0x5555FF, 0xFF55FF, 0x55FFFF, 0x000000),
	pal("stmed", "Atari ST medium", 0xFFFFFF, 0xFF0000, 0x00FF00, 0x000000, 0xFFFFFF, 0xFF0000, 0x00FF00, 0x000000,
		0xFFFFFF, 0xFF0000, 0x00FF00, 0x000000, 0xFFFFFF, 0xFF0000, 0x00FF00, 0x000000),
	pal("sthigh", "Atari ST high (mono)", 0xFFFFFF, 0x000000, 0xFFFFFF, 0x000000, 0xFFFFFF, 0x000000, 0xFFFFFF, 0x000000,
		0xFFFFFF, 0x000000, 0xFFFFFF, 0x000000, 0xFFFFFF, 0x000000, 0xFFFFFF, 0x000000),
}

// ---------------------------------------------------------------- the font

const emutosVersion = "02c2db75fc875265c1449efea014450466cf20b7" // EmuTOS master, 2026-09-23

var stFontFiles = []struct{ id, name, file, sha1 string }{
	{"st8x8", "Atari ST 8x8 (EmuTOS)", "fnt_st_8x8.c", "e4287c1956d383d3777b7af4ba04289854dddc2a"},
	{"st8x16", "Atari ST 8x16 (EmuTOS)", "fnt_st_8x16.c", "fc916a8a36639f4cfb45f4aad0cbc6baae95855c"},
}

func stFontPath(file string) string { return filepath.Join(appDataDir(), "emutos-"+file) }

func hasSTFont() bool { return fontByID("st8x8") != nil && fontByID("st8x16") != nil }

var hexWord = regexp.MustCompile(`0x([0-9a-fA-F]{4})`)

// loadSTFonts reads EmuTOS's GEM fonts: dat_table is one bitmap 256 bytes wide,
// a byte per character per row, 8 or 16 rows.
func loadSTFonts() bool {
	for _, f := range stFontFiles {
		if fontByID(f.id) != nil {
			continue
		}
		src, err := os.ReadFile(stFontPath(f.file))
		if err != nil || !sha1Is(src, f.sha1) {
			continue
		}
		start := bytes.Index(src, []byte("dat_table[]"))
		end := bytes.Index(src[max(0, start):], []byte("};"))
		if start < 0 || end < 0 {
			continue
		}
		var form []byte
		for _, m := range hexWord.FindAllSubmatch(src[start:start+end], -1) {
			v, _ := strconv.ParseUint(string(m[1]), 16, 16)
			form = append(form, byte(v>>8), byte(v))
		}
		h := len(form) / 256
		if h != 8 && h != 16 {
			continue
		}
		bits := make([]byte, 4096)
		for c := 0; c < 256; c++ {
			for r := 0; r < 16; r++ {
				src := r
				if h == 8 {
					src = r / 2
				}
				bits[c*16+r] = form[src*256+c]
			}
		}
		fonts = append(fonts, &Font{id: f.id, name: f.name, kind: "st", syncterm: -1, bits: bits})
	}
	return hasSTFont()
}

func sha1Is(b []byte, want string) bool {
	sum := sha1.Sum(b)
	return hex.EncodeToString(sum[:]) == want
}

// stFontURL is where an EmuTOS font source is fetched from.
var stFontURL = func(file string) string {
	return "https://raw.githubusercontent.com/emutos/emutos/" + emutosVersion + "/bios/" + file
}

// downloadSTFonts fetches the two font sources from EmuTOS and checks them;
// a right copy already here is not fetched again.
func downloadSTFonts() error {
	if err := fetchSTFonts(); err != nil {
		return err
	}
	if !loadSTFonts() {
		return fmt.Errorf("saved the EmuTOS fonts but could not read them")
	}
	return nil
}

// fetchSTFonts is the network part of downloadSTFonts (safe off the main thread:
// it does not touch the font list); fonts already on disk are kept.
func fetchSTFonts() error {
	client := &http.Client{Timeout: 20 * time.Second}
	for _, f := range stFontFiles {
		if b, err := os.ReadFile(stFontPath(f.file)); err == nil && sha1Is(b, f.sha1) {
			continue
		}
		b, err := fetchBody(client, stFontURL(f.file), 1<<20)
		if err != nil {
			return err
		}
		if !sha1Is(b, f.sha1) {
			return fmt.Errorf("%s is not the EmuTOS font expected", f.file)
		}
		if err := os.MkdirAll(filepath.Dir(stFontPath(f.file)), 0o755); err != nil {
			return err
		}
		if err := writeAtomic(stFontPath(f.file), b); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------- VT52

// isVT52 sees the ST's escapes (ESC b, ESC c, ESC Y, ESC E ...) and no ANSI ones.
func isVT52(b []byte) bool {
	n := 0
	for i := 0; i+1 < len(b); i++ {
		if b[i] == 0x1B {
			switch b[i+1] {
			case '[':
				return false
			case 'b', 'c', 'Y', 'E', 'p', 'q', 'j', 'k', 'l', 'o', 'v', 'w', 'e', 'f':
				n++
			}
		}
	}
	return n >= 3
}

// STTerm prints VT52 into a Doc, like TOS's screen; the page is endless downwards.
type STTerm struct {
	beforeClear func()
	doc         *Doc
	x, y        int
	fg, bg      int
	rev, wrap   bool
	sx, sy      int
	state       int // 0 text, 1 ESC, 2 ESC Y row, 3 ESC Y column, 4 ESC b, 5 ESC c
	row         int
	rows        int // the screen's rows: ESC Y goes no further down (TOS clamps it)
}

func newSTTerm(d *Doc, r STRes) *STTerm {
	return &STTerm{doc: d, fg: r.fg, bg: r.bg, wrap: true, rows: r.rows}
}

func (t *STTerm) cursor() (int, int) { return t.x, t.y }

func (t *STTerm) blankCell() uint32 { return makeCell(32, t.fg, t.bg, 0) }

func (t *STTerm) write(b []byte) {
	d := t.doc
	for _, c := range b {
		switch t.state {
		case 1:
			t.state = 0
			t.esc(c)
			continue
		case 2:
			t.row, t.state = int(c)-32, 3
			continue
		case 3:
			t.y, t.x = max(0, min(max(1, t.rows)-1, t.row)), max(0, min(d.cols-1, int(c)-32))
			t.state = 0
			continue
		case 4:
			t.fg, t.state = int(c)&15, 0
			continue
		case 5:
			t.bg, t.state = int(c)&15, 0
			continue
		}
		// The controls as TOS takes them (EmuTOS bios/vt52.c normal_ascii):
		// 7-13 act, ESC starts a sequence, every other byte below 32 is thrown away.
		switch c {
		case 0x1B:
			t.state = 1
		case '\r':
			t.x = 0
		case '\n', 0x0B, 0x0C: // vertical tab and form feed are line feeds too
			t.y++
		case 0x08:
			t.x = max(0, t.x-1)
		case 0x09:
			t.x = min(d.cols-1, (t.x&^7)+8)
		case 0x07: // the bell
		default:
			if c < 0x20 {
				continue
			}
			fg, bg := t.fg, t.bg
			if t.rev {
				fg, bg = bg, fg
			}
			if t.x >= d.cols {
				if !t.wrap {
					t.x = d.cols - 1
				} else {
					t.x, t.y = 0, t.y+1
				}
			}
			d.set(t.x, t.y, makeCell(int(c), fg, bg, 0))
			t.x++
		}
	}
}

func (t *STTerm) esc(c byte) {
	d := t.doc
	switch c {
	case 'A':
		t.y = max(0, t.y-1)
	case 'B':
		t.y++
	case 'C':
		t.x = min(d.cols-1, t.x+1)
	case 'D':
		t.x = max(0, t.x-1)
	case 'E': // clear the screen, home
		if t.beforeClear != nil {
			t.beforeClear()
		}
		d.clearPage(t.blankCell(), t.rows) // also gives back the rows below the screen
		t.x, t.y = 0, 0
	case 'H':
		t.x, t.y = 0, 0
	case 'I':
		t.y = max(0, t.y-1)
	case 'J':
		for k := t.y*d.cols + t.x; k < d.rows*d.cols; k++ {
			d.cells[k] = t.blankCell()
		}
	case 'K':
		for x := t.x; x < d.cols; x++ {
			d.set(x, t.y, t.blankCell())
		}
	case 'd':
		for k := 0; k <= t.y*d.cols+t.x && k < d.rows*d.cols; k++ {
			d.cells[k] = t.blankCell()
		}
	case 'l':
		for x := 0; x < d.cols; x++ {
			d.set(x, t.y, t.blankCell())
		}
		t.x = 0
	case 'o':
		for x := 0; x <= t.x; x++ {
			d.set(x, t.y, t.blankCell())
		}
	case 'L':
		sh := &Term{doc: d, y: t.y, screenH: t.rows}
		sh.shiftRows(1)
		t.x = 0
	case 'M':
		sh := &Term{doc: d, y: t.y, screenH: t.rows}
		sh.shiftRows(-1)
		t.x = 0
	case 'Y':
		t.state = 2
	case 'b':
		t.state = 4
	case 'c':
		t.state = 5
	case 'j':
		t.sx, t.sy = t.x, t.y
	case 'k':
		t.x, t.y = t.sx, t.sy
	case 'p':
		t.rev = true
	case 'q':
		t.rev = false
	case 'v':
		t.wrap = true
	case 'w':
		t.wrap = false
	}
}

func parseVT52(b []byte, r STRes, cols int) *Doc {
	if cols <= 0 {
		cols = r.cols
	}
	d := newDoc(cols, r.rows)
	t := newSTTerm(d, r)
	fillCells(d.cells, t.blankCell())
	var f fullest
	t.beforeClear = func() { f.look(t.doc) }
	writeWatching(t, b, &f)
	if d = f.pick(t.doc); d == t.doc {
		d.fit(cursorRows(t.x, t.y))
	} else {
		d.fit(0)
	}
	return d
}

// writeVT52 gives the page as VT52: clear screen, wrap off (as TOS starts), the
// colours where they change (as '0'+pen, which TOS reads as pen & 15), CR LF
// between rows. Every row is written, but no CR LF after the last one: on the
// ST's bottom line the line feed would scroll the screen and lose the top row.
// An empty last row is a cursor right (ESC C) instead, so the file still ends on
// that row (and opens exactly as tall). Wrap stays off at the end: that is how
// TOS starts (EmuTOS vt52_init).
func writeVT52(d *Doc, r STRes) []byte {
	out := []byte{0x1B, 'b', byte('0' + r.fg), 0x1B, 'c', byte('0' + r.bg), 0x1B, 'E', 0x1B, 'w'}
	fg, bg := r.fg, r.bg
	for y := 0; y < d.rows; y++ {
		last := d.lastCol(y)
		for x := 0; x <= last; x++ {
			v := d.get(x, y)
			if f := cellFg(v); f != fg && cellCh(v) != 32 {
				fg = f
				out = append(out, 0x1B, 'b', byte('0'+f))
			}
			if b := cellBg(v); b != bg {
				bg = b
				out = append(out, 0x1B, 'c', byte('0'+b))
			}
			out = append(out, stSafeByte(byte(cellCh(v))))
		}
		switch {
		case y < d.rows-1:
			out = append(out, '\r', '\n')
		case last < 0:
			out = append(out, 0x1B, 'C')
		}
	}
	return out
}

// stSafeByte: TOS prints every byte from 32 up; the ones below are controls or
// thrown away (and would pull the rest of the row left), so they cannot be
// shown by printing: they are written as spaces. stUnprintable counts them.
func stSafeByte(c byte) byte {
	if c < 0x20 {
		return ' '
	}
	return c
}

// stUnprintable counts the cells an ST cannot show by printing (glyphs 0-31),
// which writeVT52 saves as spaces.
func stUnprintable(d *Doc) int {
	n := 0
	for _, v := range d.cells[:d.rows*d.cols] {
		if cellCh(v) < 0x20 {
			n++
		}
	}
	return n
}

// The F-key sets of an ST page: ASCII line drawing, then the ST's own symbols
// from 127 up (the ones below 32 cannot be printed on an ST, see stSafeByte):
// ⌂ † ¶ © ® ™ « » ¿ ¡, then ± ≥ ≤ ÷ ≈ ⌠ ⌡ Σ Ω π, then • · √ ⁿ ² ³ ° ≡ ∞ §.
var stFkeys = [][]int{
	{'-', '=', '|', '/', '\\', '_', '+', '*', '#', '.'},
	{'(', ')', '[', ']', '<', '>', '{', '}', ':', ';'},
	{0x7F, 0xBB, 0xBC, 0xBD, 0xBE, 0xBF, 0xAE, 0xAF, 0xA8, 0xAD},
	{0xF1, 0xF2, 0xF3, 0xF6, 0xF7, 0xF4, 0xF5, 0xE4, 0xEA, 0xE3},
	{0xF9, 0xFA, 0xFB, 0xFC, 0xFD, 0xFE, 0xF8, 0xF0, 0xDF, 0xDD},
}

// guessSTRes tells the resolution a VT52 file was made for: pens above 3 exist
// only in low resolution, columns past 40 only in medium and high.
func guessSTRes(b []byte) string {
	maxPen, maxCol := 0, 0
	for i := 0; i+2 < len(b); i++ {
		if b[i] != 0x1B {
			continue
		}
		switch b[i+1] {
		case 'b', 'c':
			maxPen = max(maxPen, int(b[i+2])&15)
		case 'Y':
			if i+3 < len(b) {
				maxCol = max(maxCol, int(b[i+3])-32)
			}
		}
	}
	switch {
	case maxCol >= 40:
		return "stmed"
	case maxPen > 3:
		return "stlow"
	}
	return ""
}
