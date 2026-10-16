package main

// CharEdit's C64 program for screens Petmate's form cannot show: a custom
// character set, multicolour or extended background colour mode. A plain
// screen in the ROM's characters is still written in Petmate's form
// (export.go), which Petmate and Marq's PETSCII editor open.
//
// The program (hand-assembled below, run by the tests in a small 6502
// interpreter) loads at $0801: a BASIC line "10 SYS2061", then code that
// blanks the screen, copies the character set (when there is one) to $2000,
// the screen codes to $0400 and the colours to $D800, sets the border,
// $D021-$D024, $D018, $D016 and $D011, and loops. Its data follows the code:
// 1000 screen codes at $088C, 1000 colours at $0C74, then the 2048 bytes of
// the character set at $105C.
//
// Where the character set goes (VIC-II bank rules: Programmer's Reference
// Guide ch. 3, "Character memory"; VIC-II data sheet, memory map): in bank 0
// (the KERNAL's, $0000-$3FFF) the VIC sees the character ROM at $1000-$1FFF,
// so a set of its own must be at $0000, $0800, $2000, $2800, $3000 or $3800;
// $0000 holds the zero page and $0800 this program. $2000 is the first free
// one: $D018 = $18 (screen at $0400: bits 4-7 = 1; characters at $2000:
// bits 1-3 = 4). The ROM's sets are $14 (upper case) and $17 (lower), as in
// Petmate's program.

import (
	"fmt"
	"path/filepath"
	"strings"
)

// c64xPrgCode is the program from $0801 to $088B (file offsets: address -
// $0801 + 2, after the load address). The bytes c64xSlot names are set per screen.
var c64xPrgCode = []byte{
	0x01, 0x08, // load address $0801
	0x0B, 0x08, 0x0A, 0x00, 0x9E, 0x32, 0x30, 0x36, 0x31, 0x00, 0x00, 0x00, // 10 SYS2061
	0xA9, 0x0B, 0x8D, 0x11, 0xD0, // $080D LDA #$0B / STA $D011: screen off while copying
	0xA0, 0x00, 0xF0, 0x14, // $0812 LDY #pages (0 or 8) / BEQ $082A
	0xA2, 0x00, // $0816 LDX #0
	0xBD, 0x5C, 0x10, // $0818 LDA $105C,X (the high byte counts up)
	0x9D, 0x00, 0x20, // $081B STA $2000,X (likewise)
	0xE8, 0xD0, 0xF7, // $081E INX / BNE $0818
	0xEE, 0x1A, 0x08, 0xEE, 0x1D, 0x08, // $0821 INC $081A / INC $081D
	0x88, 0xD0, 0xEE, // $0827 DEY / BNE $0818
	0xA2, 0x00, // $082A LDX #0
	0xBD, 0x8C, 0x08, 0x9D, 0x00, 0x04, // $082C LDA $088C,X / STA $0400,X
	0xBD, 0x86, 0x09, 0x9D, 0xFA, 0x04, // LDA $0986,X / STA $04FA,X
	0xBD, 0x80, 0x0A, 0x9D, 0xF4, 0x05, // LDA $0A80,X / STA $05F4,X
	0xBD, 0x7A, 0x0B, 0x9D, 0xEE, 0x06, // LDA $0B7A,X / STA $06EE,X
	0xBD, 0x74, 0x0C, 0x9D, 0x00, 0xD8, // LDA $0C74,X / STA $D800,X
	0xBD, 0x6E, 0x0D, 0x9D, 0xFA, 0xD8, // LDA $0D6E,X / STA $D8FA,X
	0xBD, 0x68, 0x0E, 0x9D, 0xF4, 0xD9, // LDA $0E68,X / STA $D9F4,X
	0xBD, 0x62, 0x0F, 0x9D, 0xEE, 0xDA, // LDA $0F62,X / STA $DAEE,X
	0xE8, 0xE0, 0xFA, 0xD0, 0xCB, // $085C INX / CPX #250 / BNE $082C
	0xA9, 0x00, 0x8D, 0x20, 0xD0, // $0861 border
	0xA9, 0x00, 0x8D, 0x21, 0xD0, // $0866 screen colour
	0xA9, 0x00, 0x8D, 0x22, 0xD0, // $086B $D022
	0xA9, 0x00, 0x8D, 0x23, 0xD0, // $0870 $D023
	0xA9, 0x00, 0x8D, 0x24, 0xD0, // $0875 $D024
	0xA9, 0x00, 0x8D, 0x18, 0xD0, // $087A character set and screen
	0xA9, 0x00, 0x8D, 0x16, 0xD0, // $087F $C8, or $D8 for multicolour
	0xA9, 0x00, 0x8D, 0x11, 0xD0, // $0884 $1B, or $5B for ECM
	0x4C, 0x89, 0x08, // $0889 JMP $0889
}

// The bytes of c64xPrgCode set per screen (file offsets).
var c64xSlot = struct{ pages, border, bg, regs, d018, d016, d011 int }{0x14, 0x63, 0x68, 0x6D, 0x7C, 0x81, 0x86}

const (
	c64xScreen  = 0x8D               // file offset of the screen codes ($088C)
	c64xCharset = c64xScreen + 2000  // and of the character set ($105C)
	c64xLenROM  = c64xCharset        // a program with the ROM's characters
	c64xLenOwn  = c64xCharset + 2048 // one with its own
)

// c64xPrg gives CharEdit's program for the first 40x25 cells; font nil = the ROM's set.
func c64xPrg(d *Doc, lower bool, vic vicSetup, font *Font) []byte {
	chars, cols := c64Screen(d)
	out := append([]byte{}, c64xPrgCode...)
	out[c64xSlot.border], out[c64xSlot.bg] = byte(d.borderColour()&15), byte(d.bg&15)
	for i, r := range vic.Regs {
		out[c64xSlot.regs+5*i] = byte(r & 15)
	}
	out[c64xSlot.d018], out[c64xSlot.d016], out[c64xSlot.d011] = 0x14, 0xC8, 0x1B
	if lower {
		out[c64xSlot.d018] = 0x17
	}
	if vic.Mode == vicMulti {
		out[c64xSlot.d016] = 0xD8
	}
	if vic.Mode == vicECM {
		out[c64xSlot.d011] = 0x5B
	}
	out = append(out, chars...)
	out = append(out, cols...)
	if font != nil {
		out[c64xSlot.pages], out[c64xSlot.d018] = 8, 0x18
		for c := 0; c < 256; c++ {
			for r := 0; r < 8; r++ {
				out = append(out, font.row(c, r))
			}
		}
	}
	return out
}

// isC64xPrg: is b CharEdit's own program (not Petmate's form)?
func isC64xPrg(b []byte) bool {
	_, _, _, _, err := parseC64xPrg(b)
	return err == nil
}

// parseC64xPrg reads CharEdit's program: the screen, its case (0 upper, 1
// lower), mode and $D022-$D024, and its character set (nil: the ROM's).
func parseC64xPrg(b []byte) (*Doc, int, vicSetup, []byte, error) {
	bad := fmt.Errorf("not CharEdit's C64 program")
	if len(b) != c64xLenROM && len(b) != c64xLenOwn {
		return nil, -1, vicSetup{}, nil, bad
	}
	for i, c := range c64xPrgCode {
		switch i {
		case c64xSlot.pages, c64xSlot.border, c64xSlot.bg, c64xSlot.regs, c64xSlot.regs + 5, c64xSlot.regs + 10,
			c64xSlot.d018, c64xSlot.d016, c64xSlot.d011:
			continue
		}
		if b[i] != c {
			return nil, -1, vicSetup{}, nil, bad
		}
	}
	own := len(b) == c64xLenOwn
	if pages := b[c64xSlot.pages]; (own && pages != 8) || (!own && pages != 0) {
		return nil, -1, vicSetup{}, nil, bad
	}
	d := newDoc(40, 25)
	d.border, d.bg, d.glyph0 = int(b[c64xSlot.border]&15), int(b[c64xSlot.bg]&15), true
	for i := 0; i < 1000; i++ {
		d.cells[i] = makeCell(int(b[c64xScreen+i]), int(b[c64xScreen+1000+i]&15), 0, 0)
	}
	var vic vicSetup
	for i := range vic.Regs {
		vic.Regs[i] = int(b[c64xSlot.regs+5*i] & 15)
	}
	switch {
	case b[c64xSlot.d011]&0x40 != 0:
		vic.Mode = vicECM
	case b[c64xSlot.d016]&0x10 != 0:
		vic.Mode = vicMulti
	}
	lower := 0
	if !own && b[c64xSlot.d018]&2 != 0 {
		lower = 1
	}
	var set []byte
	if own {
		set = b[c64xCharset:]
	}
	return d, lower, vic, set, nil
}

// writeC64Prg is the page's .prg: Petmate's form when that can show it,
// CharEdit's own for a character set of its own, multicolour or ECM.
func (a *App) writeC64Prg(d *Doc) []byte {
	font := a.ownInUse()
	if font == nil && a.vicMode() == vicHires {
		return writePrg(d, a.lower())
	}
	vic := a.vic
	vic.Mode = a.vicMode()
	return c64xPrg(d, a.lower(), vic, font)
}

// c64xFont is a character set read from a file made into the page's own font.
func c64xFont(set []byte, name, format string) *Font {
	f := newFont(ownID, name, "c64", set, 8, 256)
	f.format = format
	return f
}

// c64xAfterLoad gives a page read from CharEdit's program (or a .d64 with
// one) its mode and character set. kind is the page's srcKind.
func (a *App) c64xAfterLoad(b []byte, kind string) {
	switch kind {
	case "d64":
		f, _, err := d64Art(b)
		if err != nil {
			return
		}
		b = f.data
	case "prg":
	default:
		return
	}
	_, _, vic, set, err := parseC64xPrg(b)
	if err != nil {
		return
	}
	a.vic = vic
	if set != nil {
		a.useOwnFont(c64xFont(set, strings.TrimSuffix(a.name, filepath.Ext(a.name))+" (charset)", "prg"), nil, false)
	}
}

// ---------------------------------------------------------------- ACME source and BASIC listing

// c64xAsmRegs are the lines the ACME source gets for a page beyond the ROM's
// characters in normal text: the registers set before the copy loop.
func c64xAsmRegs(vic vicSetup, own bool) string {
	var s strings.Builder
	if own {
		s.WriteString("\tlda\t#$18\n\tsta\t$d018\n")
	}
	for i, r := range vic.Regs {
		if vic.Mode == vicMulti && i == 2 {
			break
		}
		if vic.Mode != vicHires {
			fmt.Fprintf(&s, "\tlda\t#%d\n\tsta\t$d02%d\n", r, 2+i)
		}
	}
	if vic.Mode == vicMulti {
		s.WriteString("\tlda\t#$d8\n\tsta\t$d016\n")
	}
	return s.String()
}

// c64xAsmCharset is the character set as ACME bytes, 8 to a line, one glyph a line.
func c64xAsmCharset(f *Font, label string) string {
	var s strings.Builder
	s.WriteString(label)
	for c := 0; c < 256; c++ {
		s.WriteString("\t!byte ")
		for r := 0; r < 8; r++ {
			if r > 0 {
				s.WriteString(",")
			}
			fmt.Fprintf(&s, "$%02x", f.row(c, r))
		}
		s.WriteString("\n")
	}
	return s.String()
}

// writeC64Asm is the ACME source (Marq's form, export.go) with the page's mode
// and character set when it has them.
func (a *App) writeC64Asm(d *Doc) []byte {
	font, vic := a.ownInUse(), a.vic
	vic.Mode = a.vicMode()
	src := writeMarqAsm(d, a.lower() && font == nil)
	if font == nil && vic.Mode == vicHires {
		return src
	}
	s := string(src)
	whole := d.cols == 40 && max(1, d.rows) == 25
	if whole {
		regs := c64xAsmRegs(vic, font != nil)
		s = strings.Replace(s, "\tsta\t$d021\n", "\tsta\t$d021\n"+regs, 1)
		if vic.Mode == vicECM {
			s = strings.Replace(s, "\tlda\t#27\n\tsta\t$d011\n", "\tlda\t#91\n\tsta\t$d011\n", 1)
		}
		if font != nil {
			s += "\n; Character set: the VIC reads it at $2000 ($d018 = $18)\n\t*=$2000\n" + c64xAsmCharset(font, "")
		}
		return []byte(s)
	}
	head := fmt.Sprintf("; Screen mode: %s", strings.ToLower(vicModeNames[vic.Mode]))
	if vic.Mode != vicHires {
		head += fmt.Sprintf(", $d022-$d024 = %d,%d,%d", vic.Regs[0], vic.Regs[1], vic.Regs[2])
	}
	s = head + "\n" + s
	if font != nil {
		s += "; Character set (2048 bytes)\n" + c64xAsmCharset(font, "charset:\n")
	}
	return []byte(s)
}

// writeC64Bas is the BASIC listing (Marq's form, export.go) with the page's
// mode and character set. A character set does not fit under BASIC's program
// (the DATA lines alone are some 20 KB from $0801), so the VIC is moved to
// bank 3 ($C000-$FFFF, no character ROM there): screen at $C400, characters
// at $C800 ($D018 = 20), both in the 4 KB at $C000 that BASIC leaves alone;
// location 648 tells the KERNAL where the screen is.
func (a *App) writeC64Bas(d *Doc) []byte {
	font, vic := a.ownInUse(), a.vic
	vic.Mode = a.vicMode()
	if font == nil && vic.Mode == vicHires {
		return writeMarqBas(d, a.lower())
	}
	var s strings.Builder
	s.WriteString("10 rem petcat -text -w2 -o export.prg export.bas\n20 print chr$(147)\n")
	fmt.Fprintf(&s, "30 poke 53280,%d\n", d.borderColour())
	if a.lower() && font == nil {
		s.WriteString("35 poke 53272,23\n")
	}
	fmt.Fprintf(&s, "40 poke 53281,%d\n", d.bg)
	if vic.Mode != vicHires {
		fmt.Fprintf(&s, "42 poke 53282,%d:poke 53283,%d:poke 53284,%d\n", vic.Regs[0], vic.Regs[1], vic.Regs[2])
	}
	screen := 1024
	if font != nil {
		s.WriteString("45 for i=0 to 2047:read a:poke 51200+i,a:next\n")
		screen = 50176
	}
	fmt.Fprintf(&s, "50 for i=0 to 999:read a:poke %d+i,a:read a:poke 55296+i,a:next\n", screen)
	if font != nil {
		s.WriteString("55 poke 56576,peek(56576)and252:poke 53272,20:poke 648,196\n")
	}
	switch vic.Mode {
	case vicMulti:
		s.WriteString("57 poke 53270,216\n")
	case vicECM:
		s.WriteString("57 poke 53265,91\n")
	}
	s.WriteString("60 goto 60\n")
	line := 70
	data := func(vals []byte) {
		for i := 0; i < len(vals); i += 8 {
			fmt.Fprintf(&s, "%d data ", line)
			for j := i; j < min(i+8, len(vals)); j++ {
				if j > i {
					s.WriteString(",")
				}
				fmt.Fprint(&s, vals[j])
			}
			s.WriteString("\n")
			line += 10
		}
	}
	if font != nil {
		var set []byte
		for c := 0; c < 256; c++ {
			for r := 0; r < 8; r++ {
				set = append(set, font.row(c, r))
			}
		}
		data(set)
	}
	chars, cols := c64Screen(d)
	var pairs []byte
	for i := range chars {
		pairs = append(pairs, chars[i], cols[i])
	}
	data(pairs)
	fmt.Fprintf(&s, "%d end\n", line)
	return []byte(s.String())
}

// encodeC64x writes the formats of this file: the .prg, the .d64 and the ACME
// and BASIC exports of a C64 page; ok false for other formats.
func (a *App) encodeC64x(fmtID string, d *Doc) (out []byte, ok bool, err error) {
	if !a.c64() {
		return nil, false, nil
	}
	switch fmtID {
	case "prg", "d64":
		if d.cols != 40 || d.usedRows() > 25 {
			a.msg("Only the first 40x25 cells fit a C64 screen")
		}
		prg := a.writeC64Prg(d)
		if fmtID == "prg" {
			return prg, true, nil
		}
		return writeD64(cpcTarget, prg), true, nil
	case "marqasm":
		return a.writeC64Asm(d), true, nil
	case "marqbas":
		return a.writeC64Bas(d), true, nil
	}
	return nil, false, nil
}

// isOurC64Prg: a .prg CharEdit can open (Petmate's form or its own).
func isOurC64Prg(b []byte) bool { return isOurPrg(b) || isC64xPrg(b) }

// readC64Prg reads either form of .prg.
func readC64Prg(b []byte) (*Doc, int, error) {
	if d, lower, err := parsePrg(b); err == nil {
		return d, lower, nil
	}
	d, lower, _, _, err := parseC64xPrg(b)
	if err != nil {
		return nil, -1, fmt.Errorf("only C64 programs in Petmate's form or CharEdit's can be opened")
	}
	return d, lower, nil
}
