package main

// A Plus/4 page's own character set (ted.go has the page): carried by
// CharEdit's own Plus/4 program (.prg, .d64), by TED Screen Editor's project
// (NAME.chrs next to the .scrn, as TEDSE saves it) and by Marq's BASIC and ACME
// exports; edited in the font editor, where a character and its reverse move
// together as the TED shows them.
//
// Where the TED reads characters (*read* in VICE 3.9 src/plus4/ted.c,
// ted_update_memory_ptrs, and ted-draw.c): $FF12 bit 2 picks ROM (1) or RAM
// (0); $FF13 bits 7-2 are the set's address in 1 KB steps. With $FF07 bit 7
// clear (the KERNAL's default) codes 128-255 are 0-127 reversed by the TED and
// a set is 128 characters (1 KB, 1 KB aligned); with it set, the 256 codes each
// have a glyph (2 KB, the address's bit 10 ignored: 2 KB aligned). TEDSE's
// TED_CharsetCustom does the same ($FF13 high bits, $FF12 &= $FB) with its set
// at $B800, 128 characters.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func init() {
	fontExts[".chrs"] = true // TEDSE's character set
}

// tedxCode is CharEdit's Plus/4 program for a page with its own set, from
// $1001 (file offsets: address - $0FFF): 10 SYS4109, then SEI (the KERNAL's
// interrupt left alone would blink the cursor), the border and screen colours,
// the copy of the 1000 codes and 1000 attributes (Marq's loop), the TED's
// character set from RAM at $2000 (128 characters with the TED's reverse, or
// 256), and a loop. The data: "CHAREDIT PLUS/4", the codes at $1080, the
// attributes at $1468, the set at $2000 (the file is padded to it).
var tedxCode = []byte{
	0x01, 0x10, // load address $1001
	0x0B, 0x10, 0x0A, 0x00, 0x9E, 0x34, 0x31, 0x30, 0x39, 0x00, 0x00, 0x00, // 10 SYS4109
	0x78,                         // $100D SEI
	0xA9, 0x00, 0x8D, 0x19, 0xFF, // $100E LDA #border / STA $FF19
	0xA9, 0x00, 0x8D, 0x15, 0xFF, // $1013 LDA #screen / STA $FF15
	0xA2, 0x00, // $1018 LDX #0
	0xBD, 0x80, 0x10, 0x9D, 0x00, 0x0C, // $101A LDA $1080,X / STA $0C00,X
	0xBD, 0x7A, 0x11, 0x9D, 0xFA, 0x0C, // LDA $117A,X / STA $0CFA,X
	0xBD, 0x74, 0x12, 0x9D, 0xF4, 0x0D, // LDA $1274,X / STA $0DF4,X
	0xBD, 0x6E, 0x13, 0x9D, 0xEE, 0x0E, // LDA $136E,X / STA $0EEE,X
	0xBD, 0x68, 0x14, 0x9D, 0x00, 0x08, // LDA $1468,X / STA $0800,X
	0xBD, 0x62, 0x15, 0x9D, 0xFA, 0x08, // LDA $1562,X / STA $08FA,X
	0xBD, 0x5C, 0x16, 0x9D, 0xF4, 0x09, // LDA $165C,X / STA $09F4,X
	0xBD, 0x56, 0x17, 0x9D, 0xEE, 0x0A, // LDA $1756,X / STA $0AEE,X
	0xE8, 0xE0, 0xFA, 0xD0, 0xCB, // $104A INX / CPX #250 / BNE $101A
	0xAD, 0x12, 0xFF, 0x29, 0xFB, 0x8D, 0x12, 0xFF, // $104F LDA $FF12 / AND #$FB / STA $FF12: characters from RAM
	0xAD, 0x07, 0xFF, 0x29, 0x7F, 0x8D, 0x07, 0xFF, // $1057 LDA $FF07 / AND #$7F (128 characters) or ORA #$80 (256) / STA $FF07
	0xAD, 0x13, 0xFF, 0x29, 0x03, 0x09, 0x20, 0x8D, 0x13, 0xFF, // $105F LDA $FF13 / AND #3 / ORA #$20 / STA $FF13: the set at $2000
	0x4C, 0x69, 0x10, // $1069 JMP $1069
}

var tedxMark = []byte("CHAREDIT PLUS/4")

// The bytes of tedxCode set per page, and where the data is (file offsets).
var tedxSlot = struct{ border, bg, mode int }{0x10, 0x15, 0x5B} // mode: the AND #$7F / ORA #$80 pair

const (
	tedxScreen  = 0x1080 - 0x0FFF
	tedxCharset = 0x2000 - 0x0FFF
)

// tedHalfReversed: are characters 128-255 of f the reverse of 0-127 (what the
// TED shows by itself)?
func tedHalfReversed(f *Font) bool {
	for c := 0; c < 128; c++ {
		for r := 0; r < 8; r++ {
			if f.row(c+128, r) != ^f.row(c, r) {
				return false
			}
		}
	}
	return true
}

// tedSetBytes is the set as the TED reads it: 1024 bytes when the TED's own
// reverse shows 128-255, else 2048.
func tedSetBytes(f *Font) []byte {
	n := 256
	if tedHalfReversed(f) {
		n = 128
	}
	out := make([]byte, 0, n*8)
	for c := 0; c < n; c++ {
		for r := 0; r < 8; r++ {
			out = append(out, f.row(c, r))
		}
	}
	return out
}

// tedFontOf makes a set read from a file (1024 bytes: 128 characters, the
// TED's reverse for the rest; 2048: 256) into a page's own font.
func tedFontOf(set []byte, name, format string) *Font {
	data := make([]byte, 2048)
	copy(data, set)
	if len(set) <= 1024 {
		for i := 0; i < 1024; i++ {
			data[1024+i] = ^data[i]
		}
	}
	f := newFont(ownID, name, "c64", data, 8, 256)
	f.format = format
	return f
}

// tedxPrg gives CharEdit's Plus/4 program for the first 40x25 cells in font f.
func tedxPrg(d *Doc, f *Font) []byte {
	set := tedSetBytes(f)
	out := make([]byte, tedxCharset, tedxCharset+len(set))
	copy(out, tedxCode)
	out[tedxSlot.border], out[tedxSlot.bg] = byte(d.borderColour()&127), byte(d.bg&127)
	if len(set) == 2048 {
		out[tedxSlot.mode], out[tedxSlot.mode+1] = 0x09, 0x80
	}
	copy(out[len(tedxCode):], tedxMark)
	for y := 0; y < 25; y++ {
		for x := 0; x < 40; x++ {
			v := d.get(x, y)
			out[tedxScreen+y*40+x], out[tedxScreen+1000+y*40+x] = byte(cellCh(v)), byte(tedAttr(v))
		}
	}
	return append(out, set...)
}

// parseTEDxPrg reads CharEdit's Plus/4 program: the page and its set.
func parseTEDxPrg(b []byte) (*Doc, []byte, error) {
	bad := fmt.Errorf("not CharEdit's Plus/4 program")
	if len(b) != tedxCharset+1024 && len(b) != tedxCharset+2048 {
		return nil, nil, bad
	}
	for i, c := range tedxCode {
		if i != tedxSlot.border && i != tedxSlot.bg && i != tedxSlot.mode && i != tedxSlot.mode+1 && b[i] != c {
			return nil, nil, bad
		}
	}
	mode := [2]byte{b[tedxSlot.mode], b[tedxSlot.mode+1]}
	if string(b[len(tedxCode):len(tedxCode)+len(tedxMark)]) != string(tedxMark) ||
		!(len(b) == tedxCharset+1024 && mode == [2]byte{0x29, 0x7F} || len(b) == tedxCharset+2048 && mode == [2]byte{0x09, 0x80}) {
		return nil, nil, bad
	}
	d := newDoc(40, 25)
	d.border, d.bg, d.glyph0 = int(b[tedxSlot.border]&127), int(b[tedxSlot.bg]&127), true
	for i := 0; i < 1000; i++ {
		d.cells[i] = tedCell(int(b[tedxScreen+i]), int(b[tedxScreen+1000+i]))
	}
	return d, b[tedxCharset:], nil
}

// tedCharsAfterLoad gives a page read from CharEdit's Plus/4 program (or a
// .d64 with one) its character set. kind is the page's srcKind.
func (a *App) tedCharsAfterLoad(b []byte, kind string) {
	if kind == "d64" {
		f, _, err := d64Art(b)
		if err != nil {
			return
		}
		b = f.data
	} else if kind != "prg" {
		return
	}
	if _, set, err := parseTEDxPrg(b); err == nil {
		a.useOwnFont(tedFontOf(set, strings.TrimSuffix(a.name, filepath.Ext(a.name))+" (charset)", "prg"), nil, false)
	}
}

// tedFollow makes the font editor's change to a Plus/4 set show as the TED
// shows it: a character changed in 0-127 gives its reverse to 128-255 and the
// other way round (before: the font's bytes before the change).
func (a *App) tedFollow(f *Font, before []byte) {
	if !a.ted() || f == nil || len(before) != len(f.bits) {
		return
	}
	g := *f
	g.bits = before
	for c := 0; c < 256; c++ {
		for r := 0; r < 8; r++ {
			if f.row(c, r) != g.row(c, r) {
				for rr := 0; rr < 8; rr++ {
					f.setRow(c^128, rr, ^f.row(c, rr))
				}
				break
			}
		}
	}
}

// tedFollowGlyph is tedFollow for one character c just changed.
func (a *App) tedFollowGlyph(f *Font, c int) {
	if !a.ted() || f == nil {
		return
	}
	for r := 0; r < 8; r++ {
		f.setRow(c^128, r, ^f.row(c, r))
	}
}

// ---------------------------------------------------------------- TEDSE's .chrs

// tedseChrsPath is the character set file of a TEDSE project (NAME.chrs).
func tedseChrsPath(path string) string {
	return strings.TrimSuffix(path, filepath.Ext(path)) + ".chrs"
}

// writeTEDSEChrs is a set as TEDSE saves it: a PRG from $B800, 128 characters.
func writeTEDSEChrs(f *Font) []byte {
	out := []byte{0x00, 0xB8}
	for c := 0; c < 128; c++ {
		for r := 0; r < 8; r++ {
			out = append(out, f.row(c, r))
		}
	}
	return out
}

// tedseChrs reads the project's set when its .proj says the set was changed.
func tedseChrs(path string, proj []byte) *Font {
	if path == "" || len(proj) < 2+21 || proj[2] != 1 {
		return nil
	}
	b, err := os.ReadFile(tedseChrsPath(path))
	if err != nil || len(b) < 2+8 || len(b) > 2+2048 {
		return nil
	}
	return tedFontOf(b[2:], strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))+" (TEDSE charset)", "chrs")
}

// tedseSaveChrs writes NAME.chrs beside a TEDSE screen saved with the page's
// own set (and gives the status line's note).
func (a *App) tedseSaveChrs(path string) string {
	f := a.ownInUse()
	if f == nil {
		return ""
	}
	if err := writeAtomic(tedseChrsPath(path), writeTEDSEChrs(f)); err != nil {
		return "; its .chrs (the character set) could not be written: " + err.Error()
	}
	note := "; the character set in " + filepath.Base(tedseChrsPath(path))
	if !tedHalfReversed(f) {
		note += " (TEDSE keeps 128 characters: 128-255 show reversed there)"
	}
	return note
}

// ---------------------------------------------------------------- BASIC and ACME with a set

// tedSetBas is the BASIC listing (writeTEDBas, Marq's form) for a page with
// its own set: the set is POKEd to $E000 (RAM under the KERNAL's ROM: BASIC's
// DATA lines fill the memory from $1001 far past $2000, and a POKE there writes
// the RAM), then the TED reads it from RAM there.
func writeTEDBasSet(d *Doc, f *Font) []byte {
	set := tedSetBytes(f)
	src := string(writeTEDBas(d, false))
	ff07 := "and127"
	if len(set) == 2048 {
		ff07 = "or128"
	}
	extra := fmt.Sprintf("46 for i=0 to %d:read a:poke 57344+i,a:next\n"+
		"47 poke 65298,peek(65298)and251:poke 65287,peek(65287)%s:poke 65299,(peek(65299)and3)or224\n", len(set)-1, ff07)
	src = strings.Replace(src, "50 for i=0", extra+"50 for i=0", 1)
	// The set's DATA go before the screen's (the READs take them in order).
	at := strings.Index(src, "70 data ")
	var s strings.Builder
	line := 70
	for i := 0; i < len(set); i += 8 {
		fmt.Fprintf(&s, "%d data ", line)
		for j := i; j < i+8; j++ {
			if j > i {
				s.WriteString(",")
			}
			fmt.Fprint(&s, set[j])
		}
		s.WriteString("\n")
		line++
	}
	// Renumber the screen's DATA after the set's.
	rest := src[at:]
	var r strings.Builder
	for _, l := range strings.Split(strings.TrimSuffix(rest, "\n"), "\n") {
		_, body, _ := strings.Cut(l, " ")
		fmt.Fprintf(&r, "%d %s\n", line, body)
		line++
	}
	return []byte(src[:at] + s.String() + r.String())
}

// writeTEDAsmSet is the ACME source (writeTEDAsm, Marq's form) with the
// page's set: the TED's registers after the screen colour and the set at $2000.
func writeTEDAsmSet(d *Doc, f *Font) []byte {
	set := tedSetBytes(f)
	src := string(writeTEDAsm(d, false))
	if d.cols != 40 || max(1, d.rows) != 25 {
		var s strings.Builder
		fmt.Fprintf(&s, "; Character set (%d characters)\ncharset:\n", len(set)/8)
		tedAsmBytes(&s, set)
		return []byte(src + s.String())
	}
	ff07 := "\tand\t#$7f\n"
	if len(set) == 2048 {
		ff07 = "\tora\t#$80\n"
	}
	regs := "\tsei\n\tlda\t$ff12\n\tand\t#$fb\n\tsta\t$ff12\n\tlda\t$ff07\n" + ff07 + "\tsta\t$ff07\n" +
		"\tlda\t$ff13\n\tand\t#3\n\tora\t#$20\n\tsta\t$ff13\n"
	src = strings.Replace(src, "\tsta\t$ff15\n", "\tsta\t$ff15\n"+regs, 1)
	var s strings.Builder
	s.WriteString("\n; Character set: the TED reads it from RAM at $2000\n\t*=$2000\n")
	tedAsmBytes(&s, set)
	return []byte(src + s.String())
}

func tedAsmBytes(s *strings.Builder, set []byte) {
	for i := 0; i < len(set); i += 8 {
		s.WriteString("\t!byte ")
		for j := i; j < i+8; j++ {
			if j > i {
				s.WriteString(",")
			}
			fmt.Fprintf(s, "$%02x", set[j])
		}
		s.WriteString("\n")
	}
}
