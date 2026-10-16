package main

// Atari 8-bit files that run on the machine: a .xex (a binary load file, what
// DOS's "L" and every loader and emulator run) that shows the page on a stock
// Atari 400/800/XL/XE, and an .atr disk image that boots straight into the
// same program. And the real OS ROM's font, when an OS ROM is on this computer.
//
// The program (atariXexCode) sets the OS's shadow registers and leaves the rest
// to the OS's vertical blank: its own display list (24 lines of ANTIC mode 2,
// GRAPHICS 0's text), the colour registers (COLOR1/2/4 = 709/710/712, gtia.go),
// CHBAS (756: $E0 the ROM font, else the page's own font in the file) and
// CHACT 2 (inverse characters shown inverse); then it keeps the attract mode
// (colours cycling after minutes without a key) off forever. Memory:
//
//	$2000 the program, then "CharEdit" + width + rows   $2060 the display list
//	$2400 the font (1 KB, only when the page has its own)   $3000 the screen (960 bytes)
//
// A 38-wide page (the screen editor's margin) is shown from column 2, as the
// screen editor shows it. Pages taller than 24 rows show their first 24.

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	xexCode, xexDL, xexFont, xexScreen = 0x2000, 0x2060, 0x2400, 0x3000
	xexMagic                           = "CharEdit"
)

// Offsets of the values in atariXexCode.
const (
	xexSlotColor1 = 0x10
	xexSlotColor2 = 0x15
	xexSlotColor4 = 0x1A
	xexSlotChbas  = 0x1F
	xexSlotRTS    = 0x34
	xexSlotMagic  = 0x35
)

// atariXexCode is the program at $2000 (the colours and CHBAS filled in).
var atariXexCode = []byte{
	0xA9, 0x00, 0x8D, 0x2F, 0x02, // LDA #0, STA SDMCTL: no screen while the display list changes
	0xA9, xexDL & 255, 0x8D, 0x30, 0x02, // LDA #<dl, STA SDLSTL
	0xA9, xexDL >> 8, 0x8D, 0x31, 0x02, // LDA #>dl, STA SDLSTH
	0xA9, 0xCA, 0x8D, 0xC5, 0x02, // LDA #text, STA COLOR1 (709)
	0xA9, 0x94, 0x8D, 0xC6, 0x02, // LDA #screen, STA COLOR2 (710)
	0xA9, 0x00, 0x8D, 0xC8, 0x02, // LDA #border, STA COLOR4 (712)
	0xA9, 0xE0, 0x8D, 0xF4, 0x02, // LDA #font page, STA CHBAS (756)
	0xA9, 0x02, 0x8D, 0xF3, 0x02, // LDA #2, STA CHACT (755)
	0xA9, 0x22, 0x8D, 0x2F, 0x02, // LDA #$22, STA SDMCTL: normal playfield, display list on
	0xA9, 0x00, 0x85, 0x4D, // loop: LDA #0, STA ATRACT
	0x4C, 0x2D, 0x20, // JMP loop
	0x60, // RTS: the boot disk's DOSINI
}

// atariInternal is the screen code (ANTIC's order) of an ATASCII character.
func atariInternal(c int) byte {
	a := c & 0x7F
	switch {
	case a < 32:
		a += 64
	case a < 96:
		a -= 32
	}
	return byte(a | c&0x80)
}

// atariFromInternal is the ATASCII character of a screen code.
func atariFromInternal(b byte) int {
	a := int(b & 0x7F)
	switch {
	case a < 32:
		a += 32
	case a < 64:
		a += 32
	case a < 96:
		a -= 64
	}
	return a | int(b&0x80)
}

// atariXexImage is the program's memory from $2000 to the screen's end. font is
// 1024 bytes in the ROM's order, or nil for the ROM's own.
func atariXexImage(d *Doc, font []byte) []byte {
	mem := make([]byte, xexScreen+960-xexCode)
	copy(mem, atariXexCode)
	mem[xexSlotColor1], mem[xexSlotColor2], mem[xexSlotColor4] = byte(d.ink), byte(d.bg), byte(max(0, d.border))
	mem[xexSlotChbas] = 0xE0
	if font != nil {
		mem[xexSlotChbas] = xexFont >> 8
		copy(mem[xexFont-xexCode:], font)
	}
	copy(mem[xexSlotMagic:], xexMagic)
	rows := min(d.rows, 24)
	mem[xexSlotMagic+len(xexMagic)], mem[xexSlotMagic+len(xexMagic)+1] = byte(d.cols), byte(rows)
	dl := mem[xexDL-xexCode:]
	n := copy(dl, []byte{0x70, 0x70, 0x70, 0x42, xexScreen & 255, xexScreen >> 8})
	for i := 1; i < 24; i++ {
		dl[n] = 0x02
		n++
	}
	copy(dl[n:], []byte{0x41, xexDL & 255, xexDL >> 8})
	scr := mem[xexScreen-xexCode:]
	left := 0
	if d.cols < 40 {
		left = 40 - d.cols // the screen editor's margin
	}
	for y := 0; y < rows; y++ {
		for x := 0; x < d.cols && x+left < 40; x++ {
			scr[y*40+x+left] = atariInternal(cellCh(d.get(x, y)))
		}
	}
	return mem
}

// writeXex gives the page as a binary load file: the program, the font when it
// has one, the screen, and the run address.
func writeXex(d *Doc, font []byte) []byte {
	mem := atariXexImage(d, font)
	seg := func(out []byte, from, to int) []byte { // from..to inclusive
		out = append(out, byte(from), byte(from>>8), byte(to), byte(to>>8))
		if from >= xexCode {
			return append(out, mem[from-xexCode:to-xexCode+1]...)
		}
		return out
	}
	out := []byte{0xFF, 0xFF}
	out = seg(out, xexCode, xexDL+31)
	if font != nil {
		out = seg(out, xexFont, xexFont+1023)
	}
	out = seg(out, xexScreen, xexScreen+959)
	return append(seg(out, 0x02E0, 0x02E1), xexCode&255, xexCode>>8) // RUNAD
}

// writeATR gives a single density disk (720 sectors of 128 bytes) that boots
// into the program: the OS loads the boot sectors to $1FFA (6 bytes of boot
// header, then $2000 on) and calls $2000, which never returns.
func writeATR(d *Doc, font []byte) []byte {
	img := append([]byte{0, 0, 0xFA, 0x1F, (xexCode + xexSlotRTS) & 255, (xexCode + xexSlotRTS) >> 8}, atariXexImage(d, font)...)
	sectors := (len(img) + 127) / 128
	img[1] = byte(sectors)
	const size = 720 * 128
	out := make([]byte, 16+size)
	copy(out, []byte{0x96, 0x02, size / 16 & 255, size / 16 >> 8, 128, 0})
	copy(out[16:], img)
	return out
}

func isXex(b []byte) bool { return len(b) > 6 && b[0] == 0xFF && b[1] == 0xFF }

func isATR(b []byte) bool { return len(b) >= 16+3*128 && b[0] == 0x96 && b[1] == 0x02 }

// xexMemory loads a binary load file into 64 KB, as DOS would (segments may
// lack the $FFFF before them; a cut-off last segment is kept as far as it goes).
func xexMemory(b []byte) ([]byte, error) {
	mem := make([]byte, 65536)
	i, segs := 0, 0
	for i+4 <= len(b) {
		if b[i] == 0xFF && b[i+1] == 0xFF {
			i += 2
			continue
		}
		from, to := int(b[i])|int(b[i+1])<<8, int(b[i+2])|int(b[i+3])<<8
		if to < from {
			return nil, errors.New("a segment ends before it starts")
		}
		i += 4
		n := min(to-from+1, len(b)-i)
		copy(mem[from:], b[i:i+n])
		i += n
		segs++
	}
	if segs == 0 {
		return nil, errors.New("no segments")
	}
	return mem, nil
}

// atrMemory loads an .atr's boot sectors where the OS would put them.
func atrMemory(b []byte) ([]byte, error) {
	ssize := int(b[4]) | int(b[5])<<8
	if ssize != 128 && ssize != 256 {
		return nil, fmt.Errorf("sectors of %d bytes", ssize)
	}
	data := b[16:]
	sector := func(n int) []byte { // the first three sectors are 128 bytes on every density
		off := (n - 1) * 128
		if n > 3 {
			off = 3*128 + (n-4)*ssize
		}
		size := 128
		if n > 3 {
			size = ssize
		}
		if off+size > len(data) {
			return nil
		}
		return data[off : off+128] // the OS reads 128 bytes of each boot sector
	}
	first := sector(1)
	if first == nil {
		return nil, errors.New("no boot sector")
	}
	count, load := int(first[1]), int(first[2])|int(first[3])<<8
	mem := make([]byte, 65536)
	for s := 1; s <= count; s++ {
		sec := sector(s)
		if sec == nil {
			break
		}
		copy(mem[min(65536, load+(s-1)*128):], sec)
	}
	return mem, nil
}

var errNotOurXex = errors.New("only Atari programs CharEdit wrote can be opened")

// parseXexMemory reads the page out of the program's memory: ours only (the code
// at $2000 and CharEdit's mark). font is the 1024 bytes in the ROM's order when
// the program brings its own.
func parseXexMemory(mem []byte) (d *Doc, font []byte, err error) {
	code := mem[xexCode:]
	for i, c := range atariXexCode {
		switch i {
		case xexSlotColor1, xexSlotColor2, xexSlotColor4, xexSlotChbas:
			continue
		}
		if code[i] != c {
			return nil, nil, errNotOurXex
		}
	}
	if string(code[xexSlotMagic:xexSlotMagic+len(xexMagic)]) != xexMagic {
		return nil, nil, errNotOurXex
	}
	cols, rows := int(code[xexSlotMagic+len(xexMagic)]), int(code[xexSlotMagic+len(xexMagic)+1])
	if cols != 38 && cols != 40 {
		cols = 40
	}
	rows = max(1, min(24, rows))
	d = newDoc(cols, rows)
	d.bg, d.ink, d.border, d.glyph0 = int(code[xexSlotColor2]), int(code[xexSlotColor1]), int(code[xexSlotColor4]), true
	left := 40 - cols
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			d.set(x, y, makeCell(atariFromInternal(mem[xexScreen+y*40+x+left]), 0, 0, 0))
		}
	}
	if chbas := int(code[xexSlotChbas]); chbas != 0xE0 {
		font = append([]byte(nil), mem[chbas<<8&0xFC00:chbas<<8&0xFC00+1024]...)
	}
	return d, font, nil
}

// readAtariProgram reads a .xex or an .atr CharEdit wrote.
func readAtariProgram(b []byte) (*Doc, []byte, error) {
	var mem []byte
	var err error
	if isATR(b) {
		mem, err = atrMemory(b)
	} else {
		mem, err = xexMemory(b)
	}
	if err != nil {
		return nil, nil, err
	}
	return parseXexMemory(mem)
}

// ---------------------------------------------------------------- in the editor

func init() {
	editorOwn["xex"], editorOwn["atr"] = true, true
	for i, f := range formats { // after .ata
		if f.id == "ata" {
			formats = append(formats[:i+1], append([]Format{
				{"xex", "Atari program that shows it (.xex)", ".xex", "atari"},
				{"atr", "Atari boot disk with the program (.atr)", ".atr", "atari"}}, formats[i+1:]...)...)
			break
		}
	}
}

// atariProgramFont is the font a .xex carries: none for the standard ROM set
// (built-in AltirraOS or the OS ROM's), else the page's font in the ROM's order.
func (a *App) atariProgramFont() []byte {
	f := a.font()
	if f == nil || f.kind != "atari" || f.id == "atari" || f.id == "atarirom" {
		return nil
	}
	b, _ := fontFileBytes(f, nil, ".fnt")
	return b
}

// encodeAtari8 writes .xex and .atr.
func (a *App) encodeAtari8(fmtID string, d *Doc) (out []byte, ok bool, err error) {
	if !a.atari() || (fmtID != "xex" && fmtID != "atr") {
		return nil, false, nil
	}
	if d.usedRows() > 24 {
		a.msg("The Atari's screen has 24 rows: the program shows the first 24")
	}
	if fmtID == "xex" {
		return writeXex(d, a.atariProgramFont()), true, nil
	}
	return writeATR(d, a.atariProgramFont()), true, nil
}

// loadAtari8 opens a .xex or .atr CharEdit wrote.
func (a *App) loadAtari8(b []byte, name, path, as string) bool {
	if as != "auto" && as != "atari" {
		return false
	}
	kind := ""
	switch {
	case isATR(b):
		kind = "atr"
	case isXex(b):
		kind = "xex"
	default:
		return false
	}
	if _, _, err := readAtariProgram(b); err != nil {
		ext := strings.ToLower(filepath.Ext(name))
		if as == "auto" && ext != ".xex" && ext != ".atr" && ext != ".com" && ext != ".exe" {
			return false
		}
		a.msg("Could not open " + name + ": " + err.Error())
		return true
	}
	a.srcKind, a.ownFont, a.ownLower, a.ownPal = kind, nil, nil, nil
	a.mode = "atari"
	a.finishLoad(b, name, path, nil, kind)
	if _, font, err := readAtariProgram(b); err == nil && font != nil {
		f := atariFont(ownID, strings.TrimSuffix(name, filepath.Ext(name))+" (font)", font)
		f.src, f.format, f.h = name, "atari", 8
		a.useOwnFont(f, nil, false)
	}
	return true
}

// reparseAtari8 reads the page again from its program; false if it has none.
func (a *App) reparseAtari8() bool {
	if a.srcKind != "xex" && a.srcKind != "atr" {
		return false
	}
	d, _, err := readAtariProgram(a.source)
	if err != nil {
		d = newAtariDoc()
	}
	a.doc = d
	a.cx, a.cy, a.sel, a.float = 0, 0, nil, nil
	return true
}

// ---------------------------------------------------------------- the OS ROM's font

// atariROMPlaces are where an Atari OS ROM may be: CharEdit's data folder (put it
// there yourself: Atari's ROMs may not be passed on) and atari800's usual folders.
func atariROMPlaces() []string {
	var dirs []string
	dirs = append(dirs, appDataDir())
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".atari800"), filepath.Join(home, ".config", "atari800"))
	}
	dirs = append(dirs, "/usr/share/atari800", "/usr/local/share/atari800")
	var out []string
	for _, d := range dirs {
		for _, n := range []string{"ATARIXL.ROM", "atarixl.rom", "ATARIOSB.ROM", "atariosb.rom", "ATARIOSA.ROM", "atariosa.rom"} {
			out = append(out, filepath.Join(d, n))
		}
	}
	return out
}

// atariROMFonts takes the character sets out of an OS ROM: 10 KB (400/800, $D800-
// $FFFF: the set at $E000) or 16 KB (XL/XE, $C000-$FFFF: the set at $E000, the
// international one at $CC00). It checks that space is empty and 'A' is not.
func atariROMFonts(rom []byte) (std, intl []byte) {
	ok := func(f []byte) bool {
		return bytes.Equal(f[:8], make([]byte, 8)) && !bytes.Equal(f[33*8:34*8], make([]byte, 8))
	}
	switch len(rom) {
	case 10240:
		if f := rom[0x0800:0x0C00]; ok(f) {
			return f, nil
		}
	case 16384:
		std, intl = rom[0x2000:0x2400], rom[0x0C00:0x1000]
		if !ok(std) {
			return nil, nil
		}
		if !ok(intl) {
			intl = nil
		}
		return std, intl
	}
	return nil, nil
}

// loadAtariROMFonts adds "Atari (OS ROM)" (and the XL's international set) when
// an OS ROM is found; it tells where from, "" if none.
func loadAtariROMFonts() string {
	if fontByID("atarirom") != nil {
		return fontByID("atarirom").src
	}
	for _, p := range atariROMPlaces() {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		std, intl := atariROMFonts(b)
		if std == nil {
			continue
		}
		f := atariFont("atarirom", "Atari (OS ROM)", std)
		f.src = p
		fonts = append(fonts, f)
		if intl != nil {
			g := atariFont("atarixlrom", "Atari XL international (OS ROM)", intl)
			g.src = p
			fonts = append(fonts, g)
		}
		return p
	}
	return ""
}
