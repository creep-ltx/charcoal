package main

// ZX Spectrum tapes (.tap): what LOAD "" loads, in every emulator and through a
// tape player or a TZXDuino on a real Spectrum. CharEdit writes three blocks
// with their headers:
//
//	"name" a BASIC program, started at line 10:
//	       10 BORDER b: LOAD ""SCREEN$
//	       20 PAUSE 0: GO TO 20
//	"name" the screen: 6912 bytes of CODE at 16384 (= the .scr)
//
// so LOAD "" shows the page, in the page's border, and keeps it there. A block
// is a length (2 bytes, little end first), a flag (0 header, 255 data), the
// bytes and an XOR checksum of flag and bytes. A header is 17 bytes: type (0
// program, 3 CODE), name (10, padded with spaces), length, two parameters
// (program: the start line and the program's length; CODE: the address and
// 32768). From the ZX Spectrum manual's chapter 20 and the ROM's SA-BYTES.
//
// Reading takes the first SCREEN$ on the tape (CODE of 6912 bytes at 16384,
// any tape's loading screen) and the border of CharEdit's loader.

import (
	"errors"
	"path/filepath"
	"strconv"
	"strings"
)

// zxTapBlock is a block of a tape: its flag byte and bytes.
func zxTapBlock(flag byte, data []byte) []byte {
	n := len(data) + 2
	out := append([]byte{byte(n), byte(n >> 8), flag}, data...)
	sum := flag
	for _, b := range data {
		sum ^= b
	}
	return append(out, sum)
}

// zxTapHeader is a header block's 17 bytes.
func zxTapHeader(typ byte, name string, length, p1, p2 int) []byte {
	h := []byte{typ}
	nm := []byte(name)
	for i := 0; i < 10; i++ {
		c := byte(' ')
		if i < len(nm) && nm[i] >= 32 && nm[i] < 127 {
			c = nm[i]
		}
		h = append(h, c)
	}
	return append(h, byte(length), byte(length>>8), byte(p1), byte(p1>>8), byte(p2), byte(p2>>8))
}

// zxNumber is a number in a BASIC line: its digits, then 14 and the five-byte
// form (small integers: 0, sign 0, low, high, 0).
func zxNumber(n int) []byte {
	return append([]byte(strconv.Itoa(n)), 0x0E, 0, 0, byte(n), byte(n>>8), 0)
}

// zxLine is a BASIC line: number (high byte first), length, the tokens, ENTER.
func zxLine(n int, body ...[]byte) []byte {
	var t []byte
	for _, b := range body {
		t = append(t, b...)
	}
	t = append(t, 0x0D)
	return append([]byte{byte(n >> 8), byte(n), byte(len(t)), byte(len(t) >> 8)}, t...)
}

// The tokens the loader uses.
const (
	zxBORDER  = 0xE7
	zxLOAD    = 0xEF
	zxSCREEN  = 0xAA
	zxPAUSE   = 0xF2
	zxGOTO    = 0xEC
	zxTapLine = 10
)

// zxLoader is the BASIC program.
func zxLoader(border int) []byte {
	prog := zxLine(10, []byte{zxBORDER}, zxNumber(border&7), []byte{':', zxLOAD, '"', '"', zxSCREEN})
	return append(prog, zxLine(20, []byte{zxPAUSE}, zxNumber(0), []byte{':', zxGOTO}, zxNumber(20))...)
}

// zxTapName is a tape name made from a file's: 10 characters at most.
func zxTapName(file string) string {
	n := strings.TrimSuffix(filepath.Base(file), filepath.Ext(file))
	var out []byte
	for _, c := range []byte(n) {
		if c >= 32 && c < 127 && len(out) < 10 {
			out = append(out, c)
		}
	}
	if len(out) == 0 {
		return "screen"
	}
	return string(out)
}

// writeTap gives the tape: the loader, then the screen.
func writeTap(scr []byte, border int, name string) []byte {
	prog := zxLoader(border)
	out := zxTapBlock(0, zxTapHeader(0, name, len(prog), zxTapLine, len(prog)))
	out = append(out, zxTapBlock(0xFF, prog)...)
	out = append(out, zxTapBlock(0, zxTapHeader(3, name, 6912, 16384, 32768))...)
	return append(out, zxTapBlock(0xFF, scr)...)
}

type zxTapBlk struct {
	flag byte
	data []byte
	ok   bool // the checksum agrees
}

// zxTapBlocks cuts a tape into its blocks; false if it does not look like a tape.
func zxTapBlocks(b []byte) ([]zxTapBlk, bool) {
	var out []zxTapBlk
	for i := 0; i+2 <= len(b); {
		n := int(b[i]) | int(b[i+1])<<8
		if n < 2 || i+2+n > len(b) {
			return out, len(out) > 0 && i+2+n > len(b) // a cut-off end
		}
		blk := b[i+2 : i+2+n]
		var sum byte
		for _, c := range blk {
			sum ^= c
		}
		out = append(out, zxTapBlk{flag: blk[0], data: blk[1 : n-1], ok: sum == 0})
		i += 2 + n
	}
	return out, len(out) > 0
}

// isTap: the file starts with a header block whose checksum agrees.
func isTap(b []byte) bool {
	return len(b) >= 21 && b[0] == 19 && b[1] == 0 && b[2] == 0 && b[3] <= 3 && func() bool {
		var s byte
		for _, c := range b[2:21] {
			s ^= c
		}
		return s == 0
	}()
}

var errNoScreen = errors.New("there is no SCREEN$ on this tape")

// readTap finds the tape's first screen (CODE 6912 bytes at 16384, or a
// headerless data block of 6912) and the border CharEdit's loader sets (-1: none).
func readTap(b []byte) (scr []byte, border int, err error) {
	blocks, ok := zxTapBlocks(b)
	if !ok {
		return nil, -1, errors.New("not a tape")
	}
	border = -1
	for i, k := range blocks {
		if k.flag == 0 && len(k.data) == 17 && k.data[0] == 0 && i+1 < len(blocks) && border < 0 {
			border = zxLoaderBorder(blocks[i+1].data)
		}
		isScr := k.flag == 0 && len(k.data) == 17 && k.data[0] == 3 &&
			int(k.data[11])|int(k.data[12])<<8 == 6912 && int(k.data[13])|int(k.data[14])<<8 == 16384
		if isScr && i+1 < len(blocks) && len(blocks[i+1].data) >= 6912 {
			return blocks[i+1].data[:6912], border, nil
		}
		if k.flag == 0xFF && len(k.data) == 6912 && (i == 0 || blocks[i-1].flag != 0) {
			return k.data, border, nil
		}
	}
	return nil, border, errNoScreen
}

// zxLoaderBorder is the number after the first BORDER of a BASIC program, -1 if none.
func zxLoaderBorder(prog []byte) int {
	for i := 0; i+1 < len(prog); i++ {
		if prog[i] == zxBORDER && prog[i+1] >= '0' && prog[i+1] <= '7' && (i+2 >= len(prog) || prog[i+2] < '0' || prog[i+2] > '9') {
			return int(prog[i+1] - '0')
		}
	}
	return -1
}

// ---------------------------------------------------------------- in the editor

func init() {
	editorOwn["tap"] = true
	for i, f := range formats { // after .scr
		if f.id == "scr" {
			formats = append(formats[:i+1], append([]Format{{"tap", "ZX Spectrum tape: LOAD \"\" shows it (.tap)", ".tap", "zx"}}, formats[i+1:]...)...)
			break
		}
	}
}

func (a *App) encodeZXTap(fmtID string, d *Doc) (out []byte, ok bool, err error) {
	if fmtID != "tap" || !a.zx() {
		return nil, false, nil
	}
	name := cpcTarget
	if name == "" {
		name = a.name
	}
	return writeTap(writeSCRFont(d, a.font()), max(0, d.border), zxTapName(name)), true, nil
}

// loadZXTap opens a tape with a screen on it.
func (a *App) loadZXTap(b []byte, name, path, as string) bool {
	if as != "auto" && as != "zx" {
		return false
	}
	isExt := strings.EqualFold(filepath.Ext(name), ".tap")
	if !isTap(b) && !isExt {
		return false
	}
	if _, _, err := readTap(b); err != nil {
		if as == "auto" && !isExt {
			return false
		}
		a.msg("Could not open " + name + ": " + err.Error())
		return true
	}
	a.srcKind, a.ownFont, a.ownLower, a.ownPal = "tap", nil, nil, nil
	a.mode = "zx"
	a.finishLoad(b, name, path, nil, "tap")
	return true
}

func (a *App) reparseZXTap() bool {
	if a.srcKind != "tap" {
		return false
	}
	scr, border, err := readTap(a.source)
	if err != nil {
		a.doc = newZXDoc()
		return true
	}
	d, approx := parseSCRFont(scr, a.font())
	if border >= 0 {
		d.border = border
	}
	a.doc = d
	a.cx, a.cy, a.sel, a.float = 0, 0, nil, nil
	if approx > 0 {
		a.msg(strconv.Itoa(approx) + " of 768 cells are drawings, not characters: shown as the nearest character")
	}
	return true
}
