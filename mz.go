package main

// Sharp MZ-700 pages: the MZ-700's text screen, 40x25 characters of 8x8, each
// with a foreground and a background of the machine's 8 colours and one of two
// character sets (the attribute byte's top bit picks the second). A cell's
// character is Sharp's display code (what the video RAM holds, not ASCII); the
// set is mzSet2 in the cell. Inside CharEdit codes 0x00 (the space) and 0x20
// (the digit 0) change places, so that the space is 32 as on every other page;
// the files hold the real display codes (mzCode).
//
// The font: Sharp's character ROM may not be passed on, so CharEdit has Russell
// Marks's public domain MZ-700-like first set (fonts/mz700-NOTICE.txt); the
// second set is drawn as its codes in a box. The real ROM (mz700fon.int, both
// sets) is used when it is in CharEdit's data folder or MAME's ROMs.
//
// Files: a tape file (.mzf) whose program copies the screen into the video RAM
// (L from the monitor, or loaded in an emulator); CharEdit reads it back, and
// also other .mzf pictures that copy two 1000-byte blocks to $D000 and $D800.

import (
	"archive/zip"
	"bytes"
	_ "embed"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"
)

//go:embed fonts/mz700-rjm.font
var mzRJMFont []byte

const (
	mzCols, mzRows = 40, 25
	mzSet2         = 1 << 20 // the cell's character is of the second set
)

// mzCode turns a display code into CharEdit's character and back (0x00 and
// 0x20 change places; every other code is itself).
func mzCode(c int) int {
	switch c & 255 {
	case 0x00:
		return 0x20
	case 0x20:
		return 0x00
	}
	return c & 255
}

// mzFontFrom makes a 256-glyph font from glyphs in display code order
// (glyph(c, r) is row r of display code c, the top bit the leftmost pixel).
func mzFontFrom(id, name, kind string, glyph func(c, r int) byte) *Font {
	data := make([]byte, 2048)
	for c := 0; c < 256; c++ {
		for r := 0; r < 8; r++ {
			data[mzCode(c)*8+r] = glyph(c, r)
		}
	}
	return newFont(id, name, kind, data, 8, 256)
}

var mzFont = mzFontFrom("mz700", "MZ-700 (public domain look-alike)", "mz", func(c, r int) byte { return mzRJMFont[c*8+r] })

// mzHexFont: the second set without Sharp's ROM, each character its code in a
// box (3x5 digits).
var mzHexFont = func() *Font {
	digits := [16][5]byte{{7, 5, 5, 5, 7}, {2, 6, 2, 2, 7}, {7, 1, 7, 4, 7}, {7, 1, 3, 1, 7}, {5, 5, 7, 1, 1}, {7, 4, 7, 1, 7},
		{7, 4, 7, 5, 7}, {7, 1, 1, 2, 2}, {7, 5, 7, 5, 7}, {7, 5, 7, 1, 7}, {2, 5, 7, 5, 5}, {6, 5, 6, 5, 6},
		{3, 4, 4, 4, 3}, {6, 5, 5, 5, 6}, {7, 4, 6, 4, 7}, {7, 4, 6, 4, 4}}
	return mzFontFrom("mz700set2", "MZ-700 second set (codes)", "mz2", func(c, r int) byte {
		switch r {
		case 0, 7:
			return 0xFF
		case 6:
			return 0x81
		}
		return 0x80 | digits[c>>4][r-1]<<4 | digits[c&15][r-1]<<1 | 1
	})
}()

// mzROMSet2 is the second set of Sharp's ROM for each ROM font (by its id).
var mzROMSet2 = map[string]*Font{}

// mzPalette is the MZ-700's 8 colours: colour code n is green (4), red (2) and
// blue (1), each on or off.
var mzPalette = pal("mz700", "Sharp MZ-700", 0x000000, 0x0000FF, 0xFF0000, 0xFF00FF, 0x00FF00, 0x00FFFF, 0xFFFF00, 0xFFFFFF)

var mzColourNames = [8]string{"Black", "Blue", "Red", "Magenta", "Green", "Cyan", "Yellow", "White"}

func init() {
	fonts = append(fonts, mzFont)
	palettes = append(palettes[:len(palettes)-1], mzPalette, palettes[len(palettes)-1]) // before Custom
	editorOwn["mzf"] = true
	pageModes = append(pageModes, "mz")
	modeOrder = append(modeOrder, "mz")
	modeNames["mz"] = "Sharp MZ-700"
	openAs = append(openAs, "mz")
	openAsNames = append(openAsNames, "Sharp MZ-700")
	modeNoStream["mz"] = "an MZ-700 screen: a picture of the screen, no stream to draw in order"
	for i, f := range formats {
		switch f.id {
		case "utf8", "png", "png2", "gif", "apng":
			formats[i].modes += " mz"
		}
	}
	at := len(formats)
	for i, f := range formats {
		if f.id == "cpcdsk" || f.id == "msxbin" { // the machines' programs together
			at = i + 1
		}
	}
	formats = slices.Insert(formats, at, Format{"mzf", "Sharp MZ-700 tape file that shows it (.mzf)", ".mzf", "mz"})
}

func (a *App) mz() bool { return a.mode == "mz" }

// mzBlank is an empty cell as the monitor clears the screen: white on blue.
var mzBlank = makeCell(32, 7, 1, 0)

func newMZDoc() *Doc {
	d := newDoc(mzCols, mzRows)
	d.glyph0 = true // character 0 is the digit 0
	fillCells(d.cells, mzBlank)
	return d
}

// mzCell is a cell of character c (0-255 the first set, 256-511 the second).
func mzCell(c, fg, bg int) uint32 {
	var f uint32
	if c&256 != 0 {
		f = mzSet2
	}
	return makeCell(c, fg&7, bg&7, f)
}

// mzChar is a cell's character, 256 and up for the second set.
func mzChar(v uint32) int {
	if v&mzSet2 != 0 {
		return cellCh(v) | 256
	}
	return cellCh(v)
}

// mzSet2Font is the second set that goes with the page's font.
func (a *App) mzSet2Font() *Font {
	if f := mzROMSet2[a.font().id]; f != nil {
		return f
	}
	return mzHexFont
}

func (a *App) drawMZCell(fb *FB, px, py, z int, v uint32) {
	f := a.font()
	if v&mzSet2 != 0 {
		f = a.mzSet2Font()
	}
	ch := cellCh(v)
	fgc, bgc := a.pal[cellFg(v)&7], a.pal[cellBg(v)&7]
	for r := 0; r < 8; r++ {
		bits := f.row(ch, r)
		for b := 0; b < 8; b++ {
			c := bgc
			if bits&(0x80>>b) != 0 {
				c = fgc
			}
			fb.fill(Rect{px + b*z, py + r*z, z, z}, c)
		}
	}
}

// mzAttr is the attribute byte of a cell: the second set (bit 7), the
// foreground (bits 4-6) and the background (bits 0-2).
func mzAttr(v uint32) byte {
	a := byte(cellFg(v)&7)<<4 | byte(cellBg(v)&7)
	if v&mzSet2 != 0 {
		a |= 0x80
	}
	return a
}

// ---------------------------------------------------------------- characters

// mzRunes is the European MZ-700's two sets in display code order, as Unicode
// (en.wikipedia.org, Sharp MZ character set); U+FFFD where it has none.
var mzRunes = [2][]rune{
	[]rune(" ABCDEFGHIJKLMNOPQRSTUVWXYZ£└┘├┴0123456789-=;/.,🭶🭰🭼🭿🭸🭲🮂▎🭺🭴▄▐▁▕▂🮇" +
		"}♠◥█♦←♣•◦?◘╭╮◣◢:↑<[♥]@◤>↓\\→▚┌┐┤┬π!\"#$%&'()+*▞╳╯╰▔▏🭽🭾🭷🭱╱╲🭹🭳▀▌🭻🭵▃🮈" +
		"|abcdefghijklmnopqrstuvwxyzä𜰰𜰱𜰲𜰳�𜱀𜱁�`~𜱃���ßüöÜÄÖ𜰴𜰵𜰶𜰷𜰸𜰹𜰺𜰻𜰼𜰽𜰾𜰿{┼^_" +
		"🢳🢷🢵🢶🢴🅷🅲🛸𜰁𜰀�����😐▒🮍🮌🮎🮏🮜🮝🮟🮞��°␨��🐍𜰅𜰆𜰇𜰉𜰋🮙𜰍𜰏𜰎𜰐𜰑𜰒��𜰊🮐" +
		" 𜰡𜰢𜰣𜰤𜰥𜰦𜰧𜰨𜰩𜰪𜰫𜰬𜰭𜰮𜰯"),
	[]rune(" 𜳖𜳗𜳘𜳙𜳚𜳛𜳜𜳝𜳞𜳟𜳠𜳡𜳢𜳣𜳤𜳥𜳦𜳧𜳨𜳩𜳪𜳫𜳬𜳭𜳮𜳯𜱈𜱉𜱊☀𜱋𜳰𜳱𜳲𜳳𜳴𜳵𜳶𜳷𜳸𜳹⧉⮺𜱌𜱍𜱎𜱏𜱐𜱑𜱒𜱓𜱔𜱕����𜱬𜱫𜱮𜱭⬤𜱗" +
		"𜱖𜱙𜱘𜱚𜱛𜱜𜱝𜱞𜱟𜱱𜱰𜱳𜱲𜱵𜱷𜱶𜱴𜱡𜱠𜱣𜱢𜱦𜱥𜱨𜱧🛧𜷹𜷸✈↖↙↘↗𜱹𜱸𜱻𜱺𜱼𜱿𜱾𜱽𜲁𜲀𜲃𜲂⏫⏪⏬⏩𜲈↕↔🍷𜱄𜱅𜲉𜲊𜲲𜲳𜲶𜲷𜲴𜲵𜲸" +
		"𜲹☂⚬⚪○◾◼■©🞓🞑🞏⛵⊖⦶⊕🍓𜲦𜲧𜲨𜲩𜲪𜲫𜲬𜲭𜲮𜲯𜲰𜲱🍎💣𜱩𜱪𜲡𜲢𜲣𜲥𜲠γΣ𜲤λΩω∞∝√÷™𜲌⊠☼⊞𜰈𜲐𜲑±𜲄𜲅𜲆𜲇🯪🯨🯫" +
		"🯩𜳊𜳋𜳌𜳍𜲾𜲿𜳀𜳁𜳆𜳇𜳈𜳉𜲺𜲻𜲼𜲽𜳎𜳏𜳐𜳑𜳂𜳃𜳄𜳅𜰕𜰗𜰖𜰘𜰜𜰛𜰝𜰞𜰙𜰚Æ½¼⅓⅔≥≤𜳒𜳓𜳔𜳕𜱆𜱇𜲒𜲓𜲔𜲕𜲖𜲘𜲚𜲗𜲙𜲞𜲟🏢⛶≠⌠⌡"),
}

// mzRune is what character c (0-511, CharEdit's numbering) looks like in Unicode.
func mzRune(c int) rune {
	set, code := c>>8&1, mzCode(c)
	if code < len(mzRunes[set]) {
		return mzRunes[set][code]
	}
	return '�'
}

var mzFromRuneMap = func() map[rune]int {
	m := map[rune]int{}
	for set := 0; set < 2; set++ {
		for code, r := range mzRunes[set] {
			if _, ok := m[r]; !ok && r != '�' {
				m[r] = mzCode(code) | set<<8
			}
		}
	}
	return m
}()

// mzFromRune is the character a typed rune gives (the first set first).
func mzFromRune(r rune) int {
	if r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' {
		if c, ok := mzFromRuneMap[r]; ok {
			return c
		}
	}
	if c, ok := mzFromRuneMap[r]; ok {
		return c
	}
	return -1
}

// mzFkeys: the quarter blocks (codes 0xF0-0xFF of the first set: bit 0 top
// left, 1 top right, 2 bottom left, 3 bottom right), halves, lines, symbols.
var mzFkeys = [][]int{
	{0xF1, 0xF2, 0xF4, 0xF8, 0xF3, 0xFC, 0xF5, 0xFA, 0xF6, 0xFF},
	{0x43, 0x3A, 0x7A, 0x3B, 0x7B, 0xD0, 0x37, 0x71, 0x3D, 0x70},
	{0x5C, 0x5D, 0x1C, 0x1D, 0x1E, 0x5E, 0x5F, 0x1F, 0xBD, 0x80},
	{0x4B, 0x4C, 0x6F, 0x6E, 0x76, 0x77, 0x6D, 0x4D, 0x4E, 0x42},
	{0x41, 0x53, 0x44, 0x46, 0x47, 0x48, 0x45, 0x50, 0x58, 0x5A},
}

// validMZFkeys: F-key sets of characters 0-511 (the second set is 256 up).
func validMZFkeys(sets [][]int) bool {
	if len(sets) == 0 {
		return false
	}
	for _, set := range sets {
		if len(set) != 10 {
			return false
		}
		for _, c := range set {
			if c < 0 || c > 511 {
				return false
			}
		}
	}
	return true
}

// ---------------------------------------------------------------- the .mzf

// An .mzf is a tape file as the monitor writes it: a 128-byte header (the
// attribute 1 for a program, the name ended by 0x0D, the size, the load and
// start addresses, a comment), then the bytes.
const (
	mzLoad    = 0x1200 // the load and start address (free RAM above the monitor's work area)
	mzVRAM    = 0xD000 // the display codes
	mzAttrRAM = 0xD800 // the attributes
)

var mzMark = []byte("CharEdit MZ-700")

// mzProgram is the program at mzLoad: copy the codes and the attributes into
// the video RAM, then stay (the monitor would clear the screen on return).
func mzProgram(d *Doc) []byte {
	codes, attrs := mzScreen(d)
	p := []byte{0xF3}          // DI
	ld := func(src, dst int) { // LD HL,src / LD DE,dst / LD BC,1000 / LDIR
		p = append(p, 0x21, byte(src), byte(src>>8), 0x11, byte(dst), byte(dst>>8), 0x01, 0xE8, 0x03, 0xED, 0xB0)
	}
	const code = 1 + 11 + 11 + 2 // the program's length
	data := mzLoad + code + len(mzMark)
	ld(data, mzVRAM)
	ld(data+1000, mzAttrRAM)
	p = append(p, 0x18, 0xFE) // JR $
	p = append(p, mzMark...)
	p = append(p, codes...)
	return append(p, attrs...)
}

// mzScreen is the page's video RAM: 1000 display codes and 1000 attributes.
func mzScreen(d *Doc) (codes, attrs []byte) {
	codes, attrs = make([]byte, 1000), make([]byte, 1000)
	for y := 0; y < mzRows; y++ {
		for x := 0; x < mzCols; x++ {
			v := mzBlank
			if x < d.cols && y < d.rows {
				v = d.get(x, y)
			}
			codes[y*mzCols+x] = byte(mzCode(cellCh(v)))
			attrs[y*mzCols+x] = mzAttr(v)
		}
	}
	return
}

// mzTapeName is a name for the tape header: up to 16 characters, Sharp's ASCII
// (upper case).
func mzTapeName(path string) string {
	base := strings.ToUpper(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
	var out []rune
	for _, r := range base {
		if r >= 0x20 && r < 0x5F && len(out) < 16 {
			out = append(out, r)
		}
	}
	if len(out) == 0 {
		return "CHAREDIT"
	}
	return string(out)
}

func writeMZF(d *Doc, name string) []byte {
	p := mzProgram(d)
	h := make([]byte, 128)
	h[0] = 1
	for i := 1; i <= 17; i++ {
		h[i] = 0x0D
	}
	copy(h[1:17], name)
	binary.LittleEndian.PutUint16(h[18:], uint16(len(p)))
	binary.LittleEndian.PutUint16(h[20:], mzLoad)
	binary.LittleEndian.PutUint16(h[22:], mzLoad)
	return append(h, p...)
}

// mzTape is an .mzf's header and bytes.
type mzTape struct {
	name        string
	load, start int
	data        []byte
}

func readMZF(b []byte) (mzTape, bool) {
	if len(b) < 128+1 || b[0] != 1 {
		return mzTape{}, false
	}
	size := int(binary.LittleEndian.Uint16(b[18:]))
	load, start := int(binary.LittleEndian.Uint16(b[20:])), int(binary.LittleEndian.Uint16(b[22:]))
	if size == 0 || len(b) < 128+size || len(b) > 128+size+1024 || load+size > 0x10000 {
		return mzTape{}, false
	}
	name := b[1:18]
	if i := bytes.IndexByte(name, 0x0D); i >= 0 {
		name = name[:i]
	}
	return mzTape{name: string(name), load: load, start: start, data: b[128 : 128+size]}, true
}

// readMZScreen finds the picture an .mzf shows: CharEdit's own (after its
// mark), or two blocks of 1000 bytes that the program puts at $D000 and $D800:
// an LD HL,nn (or LD DE/BC) of an address in the file, near a mention of
// $D000 (or $D800), with 1000 bytes there.
func readMZScreen(t mzTape) (*Doc, error) {
	data := t.data
	if i := bytes.Index(data, mzMark); i >= 0 && len(data) >= i+len(mzMark)+2000 {
		p := data[i+len(mzMark):]
		return mzDocOf(p[:1000], p[1000:2000]), nil
	}
	find := func(hi byte) int {
		for i := 0; i+1 < len(data); i++ {
			if data[i] != 0x00 || data[i+1] != hi {
				continue
			}
			for j := max(0, i-12); j < min(len(data)-2, i+12); j++ {
				if data[j] != 0x21 && data[j] != 0x11 && data[j] != 0x01 {
					continue
				}
				at := int(data[j+1]) | int(data[j+2])<<8
				if at >= t.load && at-t.load+1000 <= len(data) && at != mzVRAM && at != mzAttrRAM {
					return at - t.load
				}
			}
		}
		return -1
	}
	c, a := find(0xD0), find(0xD8)
	if c < 0 || a < 0 {
		return nil, fmt.Errorf("no picture found in the program")
	}
	return mzDocOf(data[c:c+1000], data[a:a+1000]), nil
}

func mzDocOf(codes, attrs []byte) *Doc {
	d := newMZDoc()
	for i := range 1000 {
		at := attrs[i]
		c := mzCode(int(codes[i]))
		if at&0x80 != 0 {
			c |= 256
		}
		d.cells[i] = mzCell(c, int(at>>4&7), int(at&7))
	}
	return d
}

// ---------------------------------------------------------------- Sharp's ROM

// mzROMPlaces: CharEdit's data folder (put mz700fon.int there yourself: Sharp's
// ROM may not be passed on) and MAME's ROM folders (a folder or a .zip).
func mzROMPlaces() []string {
	dirs := []string{appDataDir()}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".mame", "roms"), filepath.Join(home, "mame", "roms"))
	}
	dirs = append(dirs, "/usr/share/mame/roms", "/usr/local/share/mame/roms")
	var out []string
	for _, d := range dirs {
		out = append(out, filepath.Join(d, "mz700fon.int"), filepath.Join(d, "mz700", "mz700fon.int"), filepath.Join(d, "mz700.zip"),
			filepath.Join(d, "mz700fon.jpn"), filepath.Join(d, "mz700j", "mz700fon.jpn"), filepath.Join(d, "mz700j.zip"))
	}
	return out
}

// mzReadROM gives the 4096 bytes of a character ROM file, or of the one in a
// MAME .zip.
func mzReadROM(p string) []byte {
	if strings.HasSuffix(p, ".zip") {
		z, err := zip.OpenReader(p)
		if err != nil {
			return nil
		}
		defer z.Close()
		for _, f := range z.File {
			if n := strings.ToLower(f.Name); (n == "mz700fon.int" || n == "mz700fon.jpn") && f.UncompressedSize64 == 4096 {
				r, err := f.Open()
				if err != nil {
					return nil
				}
				b := make([]byte, 4096)
				_, err = io.ReadFull(r, b)
				r.Close()
				if err == nil {
					return b
				}
			}
		}
		return nil
	}
	b, err := os.ReadFile(p)
	if err != nil || len(b) != 4096 {
		return nil
	}
	return b
}

// mzROMFonts makes the two sets of a character ROM: 512 glyphs of 8 bytes in
// display code order, the low bit the leftmost pixel (as MAME draws it).
func mzROMFonts(rom []byte, id, name string) (set1, set2 *Font) {
	rev := func(v byte) byte {
		var o byte
		for i := 0; i < 8; i++ {
			if v&(1<<i) != 0 {
				o |= 0x80 >> i
			}
		}
		return o
	}
	set1 = mzFontFrom(id, name, "mz", func(c, r int) byte { return rev(rom[c*8+r]) })
	set2 = mzFontFrom(id+"2", name+" (second set)", "mz2", func(c, r int) byte { return rev(rom[2048+c*8+r]) })
	return
}

// loadMZROMFonts adds "MZ-700 (Sharp ROM)" when the ROM is found; it gives its path.
func loadMZROMFonts() string {
	if f := fontByID("mz700rom"); f != nil {
		return f.src
	}
	for _, p := range mzROMPlaces() {
		b := mzReadROM(p)
		// The space (display code 0) is empty, the letter A (1) is not.
		if b == nil || !bytes.Equal(b[:8], make([]byte, 8)) || bytes.Equal(b[8:16], make([]byte, 8)) {
			continue
		}
		name := "MZ-700 (Sharp ROM)"
		if strings.Contains(p, "jpn") || strings.Contains(p, "mz700j") {
			name = "MZ-700 Japanese (Sharp ROM)"
		}
		s1, s2 := mzROMFonts(b, "mz700rom", name)
		s1.src, s2.src = p, p
		fonts = append(fonts, s1)
		mzROMSet2[s1.id] = s2
		return p
	}
	return ""
}

// ---------------------------------------------------------------- in CharEdit

// loadMZ opens an .mzf picture; false if b is none.
func (a *App) loadMZ(b []byte, name, path, as string) bool {
	if as != "auto" && as != "mz" {
		return false
	}
	t, ok := readMZF(b)
	if !ok && as == "mz" { // Open as Sharp MZ-700: text
		a.srcKind, a.ownFont, a.ownLower, a.ownPal = "mztext", nil, nil, nil
		a.mode = "mz"
		a.fg, a.bg = 7, 1
		a.finishLoad(b, name, path, nil, "utf8")
		return true
	}
	if !ok {
		return false
	}
	if _, err := readMZScreen(t); err != nil {
		ext := strings.ToLower(filepath.Ext(name))
		if as == "auto" && ext != ".mzf" && ext != ".m12" && ext != ".mzt" {
			return false
		}
		a.msg("Could not open " + name + ": " + err.Error())
		return true
	}
	a.srcKind, a.ownFont, a.ownLower, a.ownPal = "mzf", nil, nil, nil
	a.mode = "mz"
	a.fg, a.bg = 7, 1
	a.finishLoad(b, name, path, nil, "mzf")
	return true
}

// reparseMZ reads the page again from its .mzf; false if it has none.
func (a *App) reparseMZ() bool {
	if a.srcKind == "mztext" {
		a.doc = mzText(a.source)
		a.cx, a.cy, a.sel, a.float = 0, 0, nil, nil
		return true
	}
	if a.srcKind != "mzf" {
		return false
	}
	d := newMZDoc()
	if t, ok := readMZF(a.source); ok {
		if p, err := readMZScreen(t); err == nil {
			d = p
		}
	}
	a.doc = d
	a.cx, a.cy, a.sel, a.float = 0, 0, nil, nil
	return true
}

// mzText puts UTF-8 (or Latin-1) text on an MZ-700 screen, white on blue;
// characters the MZ-700 has not are left as spaces.
func mzText(b []byte) *Doc {
	d := newMZDoc()
	s := string(b)
	if !utf8.Valid(b) {
		r := make([]rune, len(b))
		for i, c := range b {
			r[i] = rune(c)
		}
		s = string(r)
	}
	x, y := 0, 0
	for _, line := range strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n") {
		if y >= mzRows {
			break
		}
		x = 0
		for _, r := range line {
			if c := mzFromRune(r); c >= 0 && x < mzCols {
				d.cells[y*mzCols+x] = mzCell(c, 7, 1)
			}
			x++
		}
		y++
	}
	return d
}

// encodeMZ writes an MZ-700 page's own formats; ok false for other formats.
func (a *App) encodeMZ(fmtID string, d *Doc) (out []byte, ok bool, err error) {
	if !a.mz() {
		return nil, false, nil
	}
	switch fmtID {
	case "mzf":
		name := cpcTarget
		if name == "" {
			name = a.name
		}
		return writeMZF(d, mzTapeName(name)), true, nil
	case "utf8": // with the second set's characters
		var lines []string
		for y := 0; y < d.rows; y++ {
			var l []rune
			for x := 0; x < d.cols; x++ {
				l = append(l, mzRune(mzChar(d.get(x, y))))
			}
			lines = append(lines, strings.TrimRight(string(l), " "))
		}
		return []byte(strings.Join(lines, "\n") + "\n"), true, nil
	}
	return nil, false, nil
}

// mzCharHint names a character of the table.
func mzCharHint(c int) string {
	return fmt.Sprintf("Set %d, display code %d (0x%02X)", c>>8+1, mzCode(c), mzCode(c))
}
