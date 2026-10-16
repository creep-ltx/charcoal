package main

// PETSCII: the Commodore 64 screen. A cell's byte is a screen code (128-255
// reversed), its foreground one of the 16 C64 colours; the background is one
// colour for the whole screen (Doc.bg). Files are .seq: the bytes a C64 BBS
// sends, printed by the C64's screen editor.
//
// The character set is the C64's character ROM, which is not ours to pass on:
// it is read from chargen.bin in appDataDir() (or the copies that
// bbmp and VICE keep), see c64Paths. downloadChargen fetches it from the VICE
// emulator's own repository, which distributes it.

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// dataDir is where programs keep their data: $XDG_DATA_HOME if it is an
// absolute path (the XDG rules say to ignore a relative one), else
// ~/.local/share; with no home folder known, the temporary folder.
func dataDir() string {
	if data := os.Getenv("XDG_DATA_HOME"); filepath.IsAbs(data) {
		return data
	}
	home, err := os.UserHomeDir()
	if err != nil || !filepath.IsAbs(home) {
		return os.TempDir()
	}
	return filepath.Join(home, ".local", "share")
}

// A Commodore character ROM: where VICE keeps it, its size and SHA-1, and the
// places it may already be on this computer.
type cbmROM struct {
	family, name string // font id prefix ("c64"), shown name
	vice         string // path under VICE's data folder
	size         int
	sha1         string
	local        string // the file name in appDataDir()
	others       []string
}

var cbmROMs = []cbmROM{
	{"c64", "C64", "C64/chargen-901225-01.bin", 4096, "adc7c31e18c7c7413d54802ef2f4193da14711aa", "chargen.bin",
		[]string{"bbmp/extras/chargen.bin", "/usr/share/vice/C64/chargen-901225-01.bin", "/usr/share/vice/C64/chargen", "/usr/lib/vice/C64/chargen"}},
	{"vic20", "VIC-20", "VIC20/chargen-901460-03.bin", 4096, "4fd85ab6647ee2ac7ba40f729323f2472d35b9b4", "vic20-chargen.bin",
		[]string{"/usr/share/vice/VIC20/chargen-901460-03.bin", "/usr/share/vice/VIC20/chargen"}},
	{"pet", "PET", "PET/characters-2.901447-10.bin", 2048, "0157a2d55b7ac4eaeb38475889ebeea52e2593db", "pet-characters.bin",
		[]string{"/usr/share/vice/PET/characters-2.901447-10.bin", "/usr/share/vice/PET/chargen"}},
	{"c128", "C128", "C128/chargen-390059-01.bin", 8192, "29ed066d513f2d5c09ff26d9166ba23c2afb2b3f", "c128-chargen.bin",
		[]string{"/usr/share/vice/C128/chargen-390059-01.bin", "/usr/share/vice/C128/chargen"}},
}

func (r cbmROM) paths() []string {
	out := []string{filepath.Join(appDataDir(), r.local)}
	for _, o := range r.others {
		if !filepath.IsAbs(o) {
			o = filepath.Join(dataDir(), o)
		}
		out = append(out, o)
	}
	return out
}

// c64Paths: where the C64 ROM may be.
func c64Paths() []string { return cbmROMs[0].paths() }

// loadC64Fonts adds the character sets of every Commodore ROM found (upper case
// + graphics and lower + upper case for each machine) and tells whether the
// C64's is among them. Rows are doubled to the 16-row form the other fonts use.
func loadC64Fonts() bool {
	for _, r := range cbmROMs {
		if fontByID(r.family+"upper") != nil {
			continue
		}
		if b := r.onDisk(); b != nil {
			addCBMFonts(r, b)
		}
	}
	return fontByID("c64upper") != nil
}

// ok: is this the ROM (size and SHA-1)?
func (r cbmROM) ok(b []byte) bool {
	sum := sha1.Sum(b)
	return len(b) == r.size && hex.EncodeToString(sum[:]) == r.sha1
}

// onDisk is the first right copy of the ROM on this computer, nil if none.
func (r cbmROM) onDisk() []byte {
	for _, p := range r.paths() {
		fi, err := os.Stat(p)
		if err != nil || fi.Size() != int64(r.size) {
			continue
		}
		if b, err := os.ReadFile(p); err == nil && r.ok(b) {
			return b
		}
	}
	return nil
}

// hasROM: is this machine's character set here?
func hasROM(family string) bool { return fontByID(family+"upper") != nil }

func addCBMFonts(r cbmROM, b []byte) {
	for set, kase := range []string{"upper", "lower"} {
		bits := make([]byte, 4096)
		for c := 0; c < 256; c++ {
			for row := 0; row < 8; row++ {
				var v byte
				switch r.family {
				case "pet": // 128 glyphs a set; the screen shows codes 128-255 inverted
					v = b[set*1024+(c&127)*8+row]
					if c >= 128 {
						v = ^v
					}
				case "c128": // the C64's set first, the C128's own second
					v = b[4096+set*2048+c*8+row]
				default:
					v = b[set*2048+c*8+row]
				}
				bits[c*16+row*2], bits[c*16+row*2+1] = v, v
			}
		}
		name := r.name + map[string]string{"upper": " upper/graphics", "lower": " lower/upper"}[kase]
		fonts = append(fonts, &Font{id: r.family + kase, name: name, kind: "c64", syncterm: -1, bits: bits})
	}
}

// downloadChargen fetches the Commodore character ROMs from VICE, checks them and
// keeps them in appDataDir(); ROMs already here (and right) are not
// fetched again. Whatever it gets is loaded; if any machine's ROM could not be
// had, the error names those machines (the others work).
func downloadChargen() error {
	missing, first := fetchChargen()
	loadC64Fonts()
	if len(missing) > 0 {
		return fmt.Errorf("could not get the character set of %s (%v)", strings.Join(missing, ", "), first)
	}
	for _, r := range cbmROMs {
		if !hasROM(r.family) {
			return fmt.Errorf("saved the %s character set but could not read it back", r.name)
		}
	}
	return nil
}

// fetchChargen is the network part of downloadChargen (safe off the main thread:
// it does not touch the font list). It skips ROMs already on disk and names the
// machines whose ROM did not come, with the first reason.
func fetchChargen() (missing []string, first error) {
	client := &http.Client{Timeout: 20 * time.Second}
	for _, r := range cbmROMs {
		if r.onDisk() != nil {
			continue
		}
		if err := fetchROM(client, r); err != nil {
			missing = append(missing, r.name)
			if first == nil {
				first = err
			}
		}
	}
	return missing, first
}

// romURLs are the places a ROM is fetched from, in order.
var romURLs = func(r cbmROM) []string {
	return []string{
		"https://sourceforge.net/p/vice-emu/code/HEAD/tree/trunk/vice/data/" + r.vice + "?format=raw",
		"https://raw.githubusercontent.com/VICE-Team/svn-mirror/main/vice/data/" + r.vice,
	}
}

// fetchBody gets a URL: the body (at most limit bytes), or what went wrong.
func fetchBody(client *http.Client, u string, limit int64) ([]byte, error) {
	resp, err := client.Get(u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", u, resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit))
	if err != nil {
		return nil, fmt.Errorf("%s: %v", u, err)
	}
	return b, nil
}

func fetchROM(client *http.Client, r cbmROM) error {
	var last error
	for _, u := range romURLs(r) {
		b, err := fetchBody(client, u, 65536)
		if err != nil {
			last = err
			continue
		}
		if !r.ok(b) {
			last = fmt.Errorf("the file from %s is not the %s character ROM", u, r.name)
			continue
		}
		dst := r.paths()[0]
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		return writeAtomic(dst, b)
	}
	return last
}

// ---------------------------------------------------------------- the machines

// A Commodore machine a PETSCII page is made for.
type Machine struct {
	id, name   string
	family     string // the character ROM
	cols, rows int
	palette    string // the palette it starts with
	mono       bool   // one text colour (the PET's screen)
	colours    int    // text colours it can show (the VIC-20's 8)
}

var machines = []Machine{
	{"pet", "PET 40", "pet", 40, 25, "petgreen", true, 1},
	{"pet80", "PET 80", "pet", 80, 25, "petgreen", true, 1},
	{"vic20", "VIC-20", "vic20", 22, 23, "vic20", false, 8},
	{"c64", "C64", "c64", 40, 25, "c64", false, 16},
	{"c128", "C128 40", "c128", 40, 25, "c64", false, 16},
	{"c128vdc", "C128 80", "c128", 80, 25, "vdc", false, 16},
}

func machineByID(id string) Machine {
	for _, m := range machines {
		if m.id == id {
			return m
		}
	}
	return machines[3]
}

// More Commodore palettes. Colodore is Pepto's later measurement of the C64.
// The VIC-20's are close to VICE's; the C128's 80-column chip shows RGBI colours,
// put here in C64 colour order as the C128's KERNAL translates them (read, not measured).
var cbmPalettes = []*Palette{
	pal("colodore", "C64 (Colodore)", 0x000000, 0xFFFFFF, 0x813338, 0x75CEC8, 0x8E3C97, 0x56AC4D, 0x2E2C9B, 0xEDF171,
		0x8E5029, 0x553800, 0xC46C71, 0x4A4A4A, 0x7B7B7B, 0xA9FF9F, 0x706DEB, 0xB2B2B2),
	pal("vic20", "VIC-20", 0x000000, 0xFFFFFF, 0xB61F21, 0x4DF0FF, 0xB43FFF, 0x44E237, 0x1A34FF, 0xDCD71B,
		0xCA5400, 0xE9B072, 0xE79293, 0x9AF7FD, 0xE09FFF, 0x8FE493, 0x8290FF, 0xE5DE85),
	pal("vdc", "C128 VDC", 0x000000, 0xFFFFFF, 0xAA0000, 0x55FFFF, 0xFF55FF, 0x00AA00, 0x0000AA, 0xFFFF55,
		0xAA00AA, 0xAA5500, 0xFF5555, 0x555555, 0x00AAAA, 0x55FF55, 0x5555FF, 0xAAAAAA),
	monoPal("petgreen", "PET green screen", 0x41FF00),
	monoPal("petamber", "PET amber screen", 0xFFB000),
	monoPal("petwhite", "PET white screen", 0xEEEEEE),
}

// monoPal: a monitor of one colour; colour 0 is the screen, the rest the text.
func monoPal(id, name string, c uint32) *Palette {
	p := &Palette{id: id, name: name}
	for i := range p.colors {
		p.colors[i] = 0xFF000000 | c
	}
	p.colors[0] = 0xFF000000
	return p
}

// The colour codes of PETSCII, by C64 colour number.
var petColour = [16]byte{0x90, 0x05, 0x1C, 0x9F, 0x9C, 0x1E, 0x1F, 0x9E, 0x81, 0x95, 0x96, 0x97, 0x98, 0x99, 0x9A, 0x9B}

func petColourOf(c byte) int {
	for i, v := range petColour {
		if v == c {
			return i
		}
	}
	return -1
}

// petToScreen turns a printable PETSCII byte into a screen code (without the reverse bit); -1 if it is a control.
func petToScreen(c byte) int {
	switch {
	case c >= 0x20 && c <= 0x3F:
		return int(c)
	case c >= 0x40 && c <= 0x5F:
		return int(c) - 0x40
	case c >= 0x60 && c <= 0x7F:
		return int(c) - 0x20
	case c >= 0xA0 && c <= 0xBF:
		return int(c) - 0x40
	case c >= 0xC0 && c <= 0xFE:
		return int(c) - 0x80
	case c == 0xFF:
		return 0x5E
	}
	return -1
}

// screenToPet is the PETSCII byte that prints a screen code (0-127).
func screenToPet(sc int) byte {
	switch {
	case sc < 0x20:
		return byte(sc + 0x40)
	case sc < 0x40:
		return byte(sc)
	case sc < 0x60:
		return byte(sc + 0x80)
	default:
		return byte(sc + 0x40)
	}
}

// PetTerm prints PETSCII into a Doc, like the C64's screen editor. As with
// ANSI, the page is endless downwards instead of scrolling.
type PetTerm struct {
	beforeClear func()
	beforeHome  func() // HOME (0x13)
	doc         *Doc
	x, y, fg    int
	rev         bool
	lower       int // the character set asked for: -1 none, 0 upper, 1 lower
}

func newPetTerm(d *Doc) *PetTerm { return &PetTerm{doc: d, fg: 14, lower: -1} }

func (t *PetTerm) cursor() (int, int) { return t.x, t.y }

func (t *PetTerm) write(b []byte) {
	d := t.doc
	for _, c := range b {
		if col := petColourOf(c); col >= 0 {
			t.fg = col
			continue
		}
		switch c {
		case 0x12:
			t.rev = true
		case 0x92:
			t.rev = false
		case 0x0D, 0x8D:
			t.x, t.y, t.rev = 0, t.y+1, false
		case 0x93:
			if t.beforeClear != nil {
				t.beforeClear()
			}
			d.clearPage(blank, 25) // also gives back the rows below the screen
			t.x, t.y = 0, 0
		case 0x13:
			if t.beforeHome != nil {
				t.beforeHome()
			}
			t.x, t.y = 0, 0
		case 0x11:
			t.y++
		case 0x91:
			t.y = max(0, t.y-1)
		case 0x1D:
			if t.x++; t.x >= d.cols {
				t.x, t.y = 0, t.y+1
			}
		case 0x9D:
			if t.x--; t.x < 0 {
				if t.y > 0 {
					t.x, t.y = d.cols-1, t.y-1
				} else {
					t.x = 0
				}
			}
		case 0x0E:
			t.lower = 1
		case 0x8E:
			t.lower = 0
		case 0x14: // delete
			if t.x == 0 && t.y > 0 { // the KERNAL goes back to the end of the line above and blanks it
				t.x, t.y = d.cols-1, t.y-1
				d.set(t.x, t.y, blank)
			} else if t.x > 0 {
				for x := t.x - 1; x < d.cols-1; x++ {
					d.set(x, t.y, d.get(x+1, t.y))
				}
				d.set(d.cols-1, t.y, blank)
				t.x--
			}
		case 0x94: // insert
			for x := d.cols - 1; x > t.x; x-- {
				d.set(x, t.y, d.get(x-1, t.y))
			}
			d.set(t.x, t.y, blank)
		default:
			sc := petToScreen(c)
			if sc < 0 {
				continue
			}
			if t.rev {
				sc |= 0x80
			}
			d.set(t.x, t.y, makeCell(sc, t.fg, 0, 0))
			if t.x++; t.x >= d.cols {
				t.x, t.y = 0, t.y+1
			}
		}
	}
}

// parseSeq reads a .seq file; it returns the character set it asks for (-1, 0 upper, 1 lower).
// codes "imageseq": read with Image BBS's £ codes (c64bbs.go).
func parseSeq(b []byte, cols int, codes ...string) (*Doc, int) {
	if cols <= 0 {
		cols = 40
	}
	d := newDoc(cols, 25)
	d.glyph0 = true
	t := newPetTerm(d)
	var f fullest
	t.beforeClear = func() { f.look(t.doc) }
	if len(codes) > 0 && codes[0] == "imageseq" {
		it := newImageTerm(t)
		writeWatching(it, b, &f)
		it.flush()
	} else {
		writeWatching(t, b, &f)
	}
	if d = f.pick(t.doc); d == t.doc {
		d.fit(cursorRows(t.x, t.y))
	} else {
		d.fit(0)
	}
	if t.lower < 0 && capitalisedWords(b) >= 3 {
		// BBSes switch to lower case before sending such files: "Main menu" is
		// a shifted letter and plain ones, which only reads in the lower case set.
		return d, 1
	}
	return d, t.lower
}

// writeSeq gives the page as PETSCII: colour and reverse codes only where they change,
// RETURN after a row that does not fill the width.
//
// Every row is written, but no RETURN after the last one: on the C64's bottom
// line it would scroll the screen and lose the top row. An empty last row is a
// cursor right instead, so the file still ends on that row (and opens exactly
// as tall).
//
// Quote mode: each " printed toggles the screen editor's quote mode, in which
// colour, reverse and cursor codes print as reversed glyphs instead of acting
// (only RETURN and DEL still act; KERNAL 901227-03, $E716-$E7EE). So before a
// control code after an odd number of quotes, another " closes quote mode and a
// DEL takes that quote off the screen again.
func writeSeq(d *Doc, lower, clear bool) []byte {
	var out []byte
	if clear {
		out = append(out, 0x93)
	}
	if lower {
		out = append(out, 0x0E)
	} else {
		out = append(out, 0x8E)
	}
	fg, rev, quote := -1, false, false
	control := func(c byte) {
		if quote {
			out = append(out, 0x22, 0x14)
			quote = false
		}
		out = append(out, c)
	}
	rows := d.rows
	for y := 0; y < rows; y++ {
		last := d.lastCol(y)
		for x := 0; x <= last; x++ {
			v := d.get(x, y)
			sc := cellCh(v)
			if r := sc&0x80 != 0; r != rev {
				rev = r
				if r {
					control(0x12)
				} else {
					control(0x92)
				}
			}
			if sc != 0x20 && cellFg(v) != fg { // a space shows no colour
				fg = cellFg(v)
				control(petColour[fg])
			}
			c := screenToPet(sc & 0x7F)
			if c == 0x22 {
				quote = !quote
			}
			out = append(out, c)
		}
		switch {
		case y < rows-1 && last < d.cols-1:
			out = append(out, 0x0D)
			rev, quote = false, false // RETURN ends reverse and quote mode on a C64
		case y == rows-1 && last < 0:
			control(0x1D)
		}
	}
	return out
}

// petFromRune is the screen code a typed character gives, -1 if none.
func petFromRune(r rune, lower bool) int {
	switch {
	case r >= 'A' && r <= 'Z':
		if lower {
			return int(r-'A') + 0x41
		}
		return int(r-'A') + 1
	case r >= 'a' && r <= 'z':
		return int(r-'a') + 1
	case r >= 0x20 && r <= 0x3F:
		return int(r)
	}
	switch r {
	case '@':
		return 0
	case '[':
		return 0x1B
	case '£':
		return 0x1C
	case ']':
		return 0x1D
	case '↑', '^':
		return 0x1E
	case '←':
		return 0x1F
	case 'π':
		if !lower {
			return 0x5E
		}
	}
	return -1
}

// petRune is a Unicode stand-in for a screen code, for copying as text.
func petRune(sc int, lower bool) rune {
	rev := sc&0x80 != 0
	sc &= 0x7F
	switch {
	case sc == 0x20 && rev:
		return '█'
	case sc == 0:
		return '@'
	case sc < 0x1B:
		if lower {
			return rune('a' + sc - 1)
		}
		return rune('A' + sc - 1)
	case sc < 0x40:
		return []rune("[£]↑← !\"#$%&'()*+,-./0123456789:;<=>?")[sc-0x1B]
	case lower && sc >= 0x41 && sc <= 0x5A:
		return rune('A' + sc - 0x41)
	}
	if r, ok := map[int]rune{0x40: '─', 0x5D: '│', 0x5B: '┼', 0x70: '┌', 0x6E: '┐', 0x6D: '└', 0x7D: '┘', 0x6B: '├',
		0x73: '┤', 0x71: '┴', 0x72: '┬', 0x61: '▌', 0x62: '▄', 0x66: '▒', 0x55: '╭', 0x49: '╮', 0x4A: '╰', 0x4B: '╯',
		0x4D: '╲', 0x4E: '╱', 0x56: '╳', 0x51: '●', 0x57: '○', 0x53: '♥', 0x5A: '♦', 0x58: '♣', 0x41: '♠', 0x5E: 'π'}[sc]; ok {
		return r
	}
	return '▒'
}

var c64Palette = pal("c64", "C64 (Pepto)", 0x000000, 0xFFFFFF, 0x68372B, 0x70A4B2, 0x6F3D86, 0x588D43, 0x352879, 0xB8C76F,
	0x6F4F25, 0x433900, 0x9A6759, 0x444444, 0x6C6C6C, 0x9AD284, 0x6C5EB5, 0x959595)

var c64Fkeys = [][]int{
	{0xA0, 0x66, 0x61, 0xE1, 0x62, 0xE2, 0x6C, 0x7B, 0x7E, 0x7C},
	{0x40, 0x5D, 0x70, 0x6E, 0x6D, 0x7D, 0x6B, 0x73, 0x71, 0x72},
	{0x55, 0x49, 0x4A, 0x4B, 0x5B, 0x4D, 0x4E, 0x5F, 0x69, 0x56},
	{0x51, 0x57, 0x53, 0x5A, 0x58, 0x41, 0x5E, 0x4F, 0x50, 0x7A},
	{0x63, 0x64, 0x65, 0x67, 0x68, 0x6A, 0x74, 0x75, 0x76, 0x77},
}

// capitalisedWords counts a shifted letter followed by two or more plain ones.
func capitalisedWords(b []byte) int {
	n, run, capital := 0, 0, false
	for _, c := range b {
		switch {
		case c >= 0xC1 && c <= 0xDA:
			capital, run = true, 0
		case c >= 0x41 && c <= 0x5A && capital:
			if run++; run == 2 {
				n++
			}
		default:
			capital, run = false, 0
		}
	}
	return n
}
