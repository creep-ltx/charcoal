package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"image/png"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// cpcFRandomPage is a random screen of mode m: any character, pens and papers of the mode.
func cpcFRandomPage(r *rand.Rand, m int) (*Doc, cpcSetup) {
	s := newCPCSetup(m)
	for i := range s.ink {
		s.ink[i] = r.Intn(27)
	}
	s.border = r.Intn(27)
	d := newCPCDoc(m)
	n := s.info().inks
	for i := range d.cells {
		d.cells[i] = makeCell(r.Intn(256), r.Intn(n), r.Intn(n), 0)
	}
	return d, s
}

// cpcFLook is what a cell shows: a space has no pen, a full block no paper.
func cpcFLook(v uint32) uint32 {
	switch ch := cellCh(v); ch {
	case 32, 0x80:
		return makeCell(ch, 0, cellBg(v), 0)
	case 0x8F:
		return makeCell(ch, cellFg(v), 0, 0)
	}
	return v
}

func cpcFSameLook(t *testing.T, what string, want, got *Doc) {
	t.Helper()
	if want.cols != got.cols || got.rows != cpcRows {
		t.Fatalf("%s: %dx%d became %dx%d", what, want.cols, want.rows, got.cols, got.rows)
	}
	for i := range want.cells[:want.cols*cpcRows] {
		if cpcFLook(want.cells[i]) != cpcFLook(got.cells[i]) {
			t.Fatalf("%s: cell %d,%d %08x became %08x", what, i%want.cols, i/want.cols, want.cells[i], got.cells[i])
		}
	}
}

func TestCPCFontAndColours(t *testing.T) {
	sum := sha1.Sum(cpcROMFont)
	if hex.EncodeToString(sum[:]) != "0718dc1d75187837154ba11eae1220ca53b81cdf" {
		t.Fatal("the CPC ROM's character set changed")
	}
	// 'A' as the ROM draws it, and the 2x2 block graphics: bit 0 top left, 1 top right, 2 bottom left, 3 bottom right.
	if f := cpcFont.bits; f['A'*16] != 0x18 || f['A'*16+8] != 0x7E || f[0x81*16] != 0xF0 || f[0x82*16] != 0x0F || f[0x84*16+14] != 0xF0 {
		t.Fatal("glyphs")
	}
	for n, want := range map[int]uint32{0: 0x000000, 1: 0x000080, 2: 0x0000FF, 3: 0x800000, 6: 0xFF0000, 9: 0x008000, 13: 0x808080,
		15: 0xFF8000, 20: 0x00FFFF, 24: 0xFFFF00, 26: 0xFFFFFF} {
		if got := cpcColour(n) & 0xFFFFFF; got != want {
			t.Errorf("colour %d (%s) is %06X, not %06X", n, cpcColourNames[n], got, want)
		}
	}
	for c := 0; c < 256; c++ { // every character has a rune, and typing the rune gives it back
		r := cpcRune(c)
		if back := cpcFromRune(r); back != c && !(r == '↑' && c == 0x0B) {
			t.Errorf("character %02X: %q types %02X", c, r, back)
		}
	}
	if cpcFromRune('^') != 0xA0 || cpcFromRune('A') != 'A' || cpcFromRune('£') != 0xA3 || cpcFromRune('█') != 0x8F {
		t.Fatal("typing")
	}
}

// cpcFZ80 runs the little programs of CPC .bin files: the Z80 instructions they
// use, and the firmware calls they make (TXT OUTPUT collects the bytes printed).
func cpcFZ80(t *testing.T, mem []byte, pc int) (printed []byte) {
	t.Helper()
	var a, b, c, h, l byte
	var zf bool
	var stack []int
	word := func(at int) int { return int(mem[at]) | int(mem[at+1])<<8 }
	for steps := 0; steps < 1_000_000; steps++ {
		op := mem[pc]
		switch op {
		case 0x21: // LD HL,nn
			l, h, pc = mem[pc+1], mem[pc+2], pc+3
		case 0x01: // LD BC,nn
			c, b, pc = mem[pc+1], mem[pc+2], pc+3
		case 0x3E: // LD A,n
			a, pc = mem[pc+1], pc+2
		case 0x7E: // LD A,(HL)
			a, pc = mem[int(h)<<8|int(l)], pc+1
		case 0x23: // INC HL
			hl := (int(h)<<8 | int(l)) + 1
			h, l, pc = byte(hl>>8), byte(hl), pc+1
		case 0x0B: // DEC BC
			bc := (int(b)<<8 | int(c)) - 1
			b, c, pc = byte(bc>>8), byte(bc), pc+1
		case 0x78: // LD A,B
			a, pc = b, pc+1
		case 0xB1: // OR C
			a |= c
			zf, pc = a == 0, pc+1
		case 0xFE: // CP n
			zf, pc = a == mem[pc+1], pc+2
		case 0xC5: // PUSH BC
			stack, pc = append(stack, int(b)<<8|int(c)), pc+1
		case 0xC1: // POP BC
			v := stack[len(stack)-1]
			stack, b, c, pc = stack[:len(stack)-1], byte(v>>8), byte(v), pc+1
		case 0x18, 0x20, 0x28: // JR, JR NZ, JR Z
			if op == 0x18 || op == 0x20 && !zf || op == 0x28 && zf {
				pc += 2 + int(int8(mem[pc+1]))
			} else {
				pc += 2
			}
		case 0xC8, 0xC9: // RET Z, RET
			if op == 0xC8 && !zf {
				pc++
				break
			}
			if len(stack) == 0 {
				return
			}
			pc, stack = stack[len(stack)-1], stack[:len(stack)-1]
		case 0xCD, 0xC3: // CALL nn, JP nn
			to := word(pc + 1)
			if to >= 0xBB00 { // the firmware
				switch to {
				case 0xBB5A: // TXT OUTPUT (all registers kept)
					printed = append(printed, a)
				case 0xBB06: // KM WAIT CHAR: the end of the show
					return
				}
				if op == 0xC3 {
					return
				}
				pc += 3
				break
			}
			if op == 0xCD {
				stack = append(stack, pc+3)
			}
			pc = to
		default:
			t.Fatalf("instruction %02X at %04X", op, pc)
		}
	}
	t.Fatal("the program does not end")
	return
}

// cpcFRun loads a .bin as AMSDOS does and runs it: what it prints.
func cpcFRun(t *testing.T, bin []byte) []byte {
	t.Helper()
	f, ok := readAMSDOS(bin)
	if !ok || f.kind != 2 {
		t.Fatal("no AMSDOS header of a binary file")
	}
	mem := make([]byte, 0x10000)
	copy(mem[f.load:], f.body)
	return cpcFZ80(t, mem, f.entry)
}

// CharEdit's .bin: the program prints exactly the stream, and the stream reads
// back as the page in every mode.
func TestCPCBinRoundTrip(t *testing.T) {
	r := rand.New(rand.NewSource(7))
	for m := 0; m < 3; m++ {
		for k := 0; k < 5; k++ {
			d, s := cpcFRandomPage(r, m)
			bin := writeCPCBin(d, s, "TEST")
			if !isCPCBin(bin) {
				t.Fatal("not recognised")
			}
			if got := cpcFRun(t, bin); !bytes.Equal(got, cpcStream(d, s)) {
				t.Fatalf("mode %d: the program printed %d bytes, the stream is %d", m, len(got), len(cpcStream(d, s)))
			}
			back, s2, note, err := parseCPCBin(bin)
			if err != nil || note != "" {
				t.Fatal(err, note)
			}
			s.ok = true
			for i := s.info().inks; i < 16; i++ { // the file sets the mode's inks only
				s.ink[i] = s2.ink[i]
			}
			if s2 != s {
				t.Fatalf("mode %d: setup %+v became %+v", m, s, s2)
			}
			cpcFSameLook(t, fmt.Sprintf("mode %d", m), d, back)
		}
	}
}

// The two programs Logiker's Amstrad ASCII Exporter puts in front of its stream
// (version 1.x, and the one in later files with the stream at 0x1026), as found
// in the AMSCII Compo 2024 entries; the stream ends with 0x1F.
var cpcFExporterStubs = [][]byte{
	{0x3E, 0x01, 0xCD, 0x0E, 0xBC, 0x21, 0x1D, 0x10, 0x7E, 0x23, 0xCD, 0x5A, 0xBB, 0x7E, 0x23, 0xCD, 0x5A, 0xBB,
		0xFE, 0x01, 0x28, 0xF2, 0xFE, 0x1F, 0x20, 0xF3, 0xC3, 0x06, 0xBB},
	{0xCD, 0x05, 0x10, 0x18, 0x1D, 0x3E, 0x01, 0xCD, 0x0E, 0xBC, 0x21, 0x26, 0x10, 0x7E, 0x23, 0xFE, 0x1F, 0xC8,
		0xCD, 0x5A, 0xBB, 0xFE, 0x01, 0x28, 0x02, 0x18, 0xF2, 0x7E, 0x23, 0xCD, 0x5A, 0xBB, 0x18, 0xEB, 0xCD, 0x06, 0xBB, 0xC9},
}

// cpcFExporterBin is a page as the exporter writes it: inks as they first come,
// a paper code and a pen code (but not for a space) where they change, code 1
// before characters 0-31, 0x1F at the end, and junk after it (a file on a disk).
func cpcFExporterBin(d *Doc, s cpcSetup, stub []byte) []byte {
	out := []byte{4, byte(s.mode), 29, byte(s.border), byte(s.border)}
	var inks []int
	ink := func(i int) byte {
		c := s.ink[i]
		for k, v := range inks {
			if v == c {
				return byte(k)
			}
		}
		inks = append(inks, c)
		out = append(out, 28, byte(len(inks)-1), byte(c), byte(c))
		return byte(len(inks) - 1)
	}
	pen, paper := -1, -1
	for _, v := range d.cells[:d.cols*cpcRows] {
		ch := cellCh(v)
		bg := int(ink(cellBg(v)))
		fg := pen
		if ch != 32 {
			fg = int(ink(cellFg(v)))
		}
		if bg != paper {
			out, paper = append(out, 14, byte(bg)), bg
		}
		if fg != pen {
			out, pen = append(out, 15, byte(fg)), fg
		}
		if ch < 32 {
			out = append(out, 1)
		}
		out = append(out, byte(ch))
	}
	out = append(out, 0x1F)
	code := append(append([]byte{}, stub...), out...)
	file := append(amsdosHeader("ART", len(code), 0x1000, 0x1000), code...)
	return append(file, 0x1A, 0x55, 0x1F, 0xE5) // padding after the end
}

func TestCPCReadsExporterBins(t *testing.T) {
	r := rand.New(rand.NewSource(3))
	for i, stub := range cpcFExporterStubs {
		for m := 0; m < 3; m++ {
			d, s := cpcFRandomPage(r, m)
			// Distinct inks (the exporter merges inks of one colour) and characters its
			// programs print right: they mistake a code's parameter 1 for code 1 and
			// stop at a 0x1F after it.
			for k := range s.ink {
				s.ink[k] = k
			}
			for k, v := range d.cells {
				if cellCh(v) < 32 {
					d.cells[k] = makeCell(cellCh(v)+32, cellFg(v), cellBg(v), 0)
				}
			}
			bin := cpcFExporterBin(d, s, stub)
			back, s2, _, err := parseCPCBin(bin)
			if err != nil {
				t.Fatal(err)
			}
			if s2.mode != m || s2.border != s.border {
				t.Fatalf("stub %d mode %d: %+v", i, m, s2)
			}
			// The inks are numbered as they came: compare the colours shown.
			for k, v := range d.cells[:d.cols*cpcRows] {
				w := back.cells[k]
				if cellCh(v) != cellCh(w) || s.ink[cellBg(v)] != s2.ink[cellBg(w)] || cellCh(v) != 32 && s.ink[cellFg(v)] != s2.ink[cellFg(w)] {
					t.Fatalf("stub %d mode %d cell %d: %08x became %08x", i, m, k, v, w)
				}
			}
			// What the programs print is what CharEdit reads (up to the end mark).
			printed := cpcFRun(t, bin)
			c := newCPCScreen()
			c.print(printed, false)
			for k := range back.cells[:back.cols*cpcRows] {
				if back.cells[k] != c.d.cells[k] {
					t.Fatalf("stub %d: the program shows cell %d as %08x, CharEdit %08x", i, k, c.d.cells[k], back.cells[k])
				}
			}
		}
	}
}

// The text screen's codes as the firmware acts on them.
func TestCPCControlCodes(t *testing.T) {
	c := newCPCScreen()
	c.print([]byte("\x04\x01"+"AB\r\nC"+"\x1f\x0a\x03X"+"\x0f\x02\x0e\x03Y"+"\x18Z"+"\x01\x07"+"\x1c\x02\x06\x06\x1d\x09\x09"), false)
	d := c.d
	at := func(x, y int) uint32 { return d.get(x, y) }
	if cellCh(at(0, 0)) != 'A' || cellCh(at(1, 0)) != 'B' || cellCh(at(0, 1)) != 'C' {
		t.Fatal("CR LF")
	}
	if cellCh(at(9, 2)) != 'X' || at(10, 2) != makeCell('Y', 2, 3, 0) || at(11, 2) != makeCell('Z', 3, 2, 0) || cellCh(at(12, 2)) != 7 {
		t.Fatalf("LOCATE, PEN, PAPER, inverse, code 1: %08x %08x %08x", at(10, 2), at(11, 2), at(12, 2))
	}
	if c.s.ink[2] != 6 || c.s.border != 9 {
		t.Fatal("INK, BORDER")
	}
	// Q at the last cell, R past it: the screen scrolls up a line.
	c.print([]byte("\x1f\x28\x19QR"), false)
	if cellCh(at(39, 23)) != 'Q' || cellCh(at(0, 24)) != 'R' || cellCh(at(0, 0)) != 'C' {
		t.Fatalf("scroll: %q %q %q", rune(cellCh(at(39, 23))), rune(cellCh(at(0, 24))), rune(cellCh(at(0, 0))))
	}
	// Mode 0 masks pens to 16, mode 2 to 2; flashing inks and SYMBOL are noted.
	c = newCPCScreen()
	c.print([]byte("\x04\x02\x0f\x03A\x1c\x00\x01\x02\x19\x41\x01\x02\x03\x04\x05\x06\x07\x08"), false)
	if c.d.cols != 80 || cellFg(c.d.get(0, 0)) != 1 || !strings.Contains(c.note(), "flashing") || !strings.Contains(c.note(), "SYMBOL") {
		t.Fatalf("mode 2: %d cols, pen %d, note %q", c.d.cols, cellFg(c.d.get(0, 0)), c.note())
	}
	// Any bytes: no crash, always a screen.
	r := rand.New(rand.NewSource(1))
	for k := 0; k < 200; k++ {
		b := make([]byte, r.Intn(3000))
		r.Read(b)
		c := newCPCScreen()
		c.print(b, k%2 == 0)
		if c.d.rows != cpcRows || len(c.d.cells) != c.d.cols*cpcRows {
			t.Fatal("not a screen")
		}
	}
}

func TestCPCDisk(t *testing.T) {
	r := rand.New(rand.NewSource(5))
	d, s := cpcFRandomPage(r, 2)
	bin := writeCPCBin(d, s, "PICTURE")
	dsk, err := writeDSK("PICTURE", bin)
	if err != nil {
		t.Fatal(err)
	}
	if len(dsk) != 256+40*(256+9*512) || !isDSK(dsk) {
		t.Fatalf("%d bytes", len(dsk))
	}
	files, err := readDSK(dsk)
	if err != nil || len(files) != 1 || files[0].name != "PICTURE.BIN" {
		t.Fatalf("%v %v", files, err)
	}
	if got := files[0].data; !bytes.Equal(got[:len(bin)], bin) || len(got)%128 != 0 {
		t.Fatal("the file on the disk is not the .bin")
	}
	f, n, err := dskArt(dsk)
	if err != nil || n != 1 {
		t.Fatal(err)
	}
	back, _, _, err := parseCPCBin(f.data)
	if err != nil {
		t.Fatal(err)
	}
	cpcFSameLook(t, "dsk", d, back)
	// A file of more than one extent (16 KB) comes back whole.
	big := append(amsdosHeader("BIG", 40000, 0x1000, 0x1000), bytes.Repeat([]byte{0x55}, 40000)...)
	dsk, _ = writeDSK("BIG", big)
	files, _ = readDSK(dsk)
	if len(files) != 1 || !bytes.Equal(files[0].data[:len(big)], big) {
		t.Fatal("three extents")
	}
	// Damaged images give an error, not a crash.
	for _, n := range []int{0, 10, 256, 300, 5000, len(dsk) - 1} {
		readDSK(dsk[:n])
	}
}

func TestCPCPlayscii(t *testing.T) {
	r := rand.New(rand.NewSource(11))
	for m := 0; m < 3; m++ {
		d, s := cpcFRandomPage(r, m)
		for i := range s.ink { // distinct colours: Playscii's tiles hold colours, not inks
			s.ink[i] = (i*5 + m) % 27
		}
		for i, v := range d.cells {
			if cellCh(v) == 0 { // the exporter's charsets have a space as glyph 0
				d.cells[i] = makeCell(1, cellFg(v), cellBg(v), 0)
			}
		}
		out, missed := writePsciCPC([]*Doc{d, d}, []int{100, 250}, s)
		if missed != 0 || !isPsci(out) {
			t.Fatal(missed)
		}
		art, err := parsePsci(out)
		if err != nil || art.mode != "cpc" || art.cpc == nil || len(art.docs) != 2 || art.holds[1] != 250 {
			t.Fatalf("%v %+v", err, art)
		}
		s2 := *art.cpc
		if s2.mode != m || s2.border != s.border {
			t.Fatalf("mode %d border %d", s2.mode, s2.border)
		}
		for k, v := range d.cells[:d.cols*cpcRows] {
			w := art.docs[0].cells[k]
			if cellCh(v) != cellCh(w) || s.ink[cellBg(v)] != s2.ink[cellBg(w)] || cellCh(v) != 32 && s.ink[cellFg(v)] != s2.ink[cellFg(w)] {
				t.Fatalf("mode %d cell %d: %08x became %08x", m, k, v, w)
			}
		}
	}
	// The older "cpc" charset starts at character 32.
	b := []byte(`{"charset":"cpc","palette":"cpc","width":40,"height":25,"selected_bg_color":3,"frames":[{"delay":0.1,"layers":[{"z":0,"visible":1,"name":"L","tiles":[` +
		strings.Repeat(`{"char":33,"fg":27,"bg":2,"xform":0},`, 999) + `{"char":0,"fg":27,"bg":2,"xform":0}]}]}]}`)
	art, err := parsePsci(b)
	if err != nil || art.mode != "cpc" {
		t.Fatal(err)
	}
	if v := art.docs[0].cells[0]; cellCh(v) != 'A' || art.cpc.ink[cellFg(v)] != 26 || art.cpc.ink[cellBg(v)] != 1 || art.cpc.border != 2 {
		t.Fatalf("%08x %+v", v, art.cpc)
	}
}

// The page in the editor: typing, the mode, the inks, undo, saving and opening
// .bin, .dsk and .psci, the autosave, the picture.
func TestCPCEditor(t *testing.T) {
	a := testApp(t, "cpc")
	if a.doc.cols != 40 || a.doc.rows != 25 || a.saveFmt != "cpcbin" || a.cellW() != 8 || a.cellH() != 8 || a.nColours(false) != 4 {
		t.Fatalf("new page: %dx%d %s", a.doc.cols, a.doc.rows, a.saveFmt)
	}
	a.fg, a.bg = 2, 3
	for _, r := range "Hi^" {
		a.typeByte(a.charToByte(r))
	}
	if a.doc.get(0, 0) != makeCell('H', 2, 3, 0) || cellCh(a.doc.get(2, 0)) != 0xA0 {
		t.Fatalf("typing: %08x", a.doc.get(0, 0))
	}
	a.setCPCInk(2, 15)
	a.setCPCBorder(4)
	if a.pal[2] != cpcColour(15) {
		t.Fatal("the ink's colour")
	}
	a.doUndo()
	if a.cpcSet.border != 1 {
		t.Fatal("undo the border")
	}
	a.doUndo()
	if a.cpcSet.ink[2] != 20 || a.pal[2] != cpcColour(20) {
		t.Fatal("undo the ink")
	}
	a.doRedo()
	a.doRedo()
	if a.cpcSet.ink[2] != 15 || a.cpcSet.border != 4 {
		t.Fatal("redo")
	}
	a.setCPCMode(0)
	if a.doc.cols != 20 || a.cellW() != 16 || a.nColours(false) != 16 || cellCh(a.doc.get(0, 0)) != 'H' {
		t.Fatal("mode 0")
	}
	a.setCPCMode(2)
	if a.doc.cols != 80 || a.cellH() != 16 {
		t.Fatal("mode 2")
	}
	a.doUndo()
	a.doUndo()
	if a.cpcSet.mode != 1 || a.doc.cols != 40 {
		t.Fatalf("undo the modes: mode %d, %d columns", a.cpcSet.mode, a.doc.cols)
	}
	a.setRows(30)
	a.setCols(50)
	if a.doc.rows != 25 || a.doc.cols != 40 {
		t.Fatal("a CPC screen keeps its size")
	}
	want, ws := a.doc.clone(), a.cpcSet
	dir := t.TempDir()
	for _, f := range []struct{ fmtID, name string }{{"cpcbin", "art.bin"}, {"cpcdsk", "My Art.dsk"}, {"psci", "art.psci"}} {
		p := filepath.Join(dir, f.name)
		if !a.writeOut(p, f.fmtID) {
			t.Fatal(a.message)
		}
		b := testApp(t, "amiga")
		b.openPath(p, "auto")
		if !b.cpc() || b.cpcSet.border != ws.border || b.saveFmt != f.fmtID {
			t.Fatalf("%s opened as %s (%s), border %d", f.name, b.mode, b.saveFmt, b.cpcSet.border)
		}
		for k, v := range want.cells {
			w := b.doc.cells[k]
			if cpcFLook(v) != cpcFLook(w) && (f.fmtID != "psci" || cellCh(v) != cellCh(w) || ws.ink[cellBg(v)] != b.cpcSet.ink[cellBg(w)]) {
				t.Fatalf("%s: cell %d %08x became %08x", f.name, k, v, w)
			}
		}
		if f.fmtID == "cpcdsk" {
			files, _ := readDSK(b.source)
			if len(files) != 1 || files[0].name != "MYART.BIN" {
				t.Fatalf("the file on the disk: %v", files)
			}
		}
		// Ctrl+S writes the same format again.
		b.typeByte('Z')
		if !b.writeOut(p, b.saveFmt) {
			t.Fatal(b.message)
		}
	}
	// The autosave keeps the mode, inks and border.
	a.setCPCMode(0)
	a.dirty = true
	if err := writeAutosave(a.Tab); err != nil {
		t.Fatal(err)
	}
	b := restart(t, "pc")
	files := leftAutosaves()
	if len(files) != 1 || b.restoreAutosave(files[0]) != nil {
		t.Fatal("autosave")
	}
	if !b.cpc() || b.cpcSet != a.cpcSet || b.doc.cols != 20 {
		t.Fatalf("autosave: %s %+v", b.mode, b.cpcSet)
	}
	// The picture has the border round it, in the border's colour.
	pngb, err := a.encode("png")
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(bytes.NewReader(pngb))
	if err != nil {
		t.Fatal(err)
	}
	if w, h := img.Bounds().Dx(), img.Bounds().Dy(); w != 320+64 || h != 200+64 {
		t.Fatalf("picture %dx%d", w, h)
	}
	if r, g, bb, _ := img.At(1, 1).RGBA(); uint32(r>>8)<<16|uint32(g>>8)<<8|uint32(bb>>8) != cpcColour(4)&0xFFFFFF {
		t.Fatal("border colour")
	}
	// The formats offered, the menus and the start screen know the page.
	var ids []string
	for _, f := range a.modeFormats() {
		ids = append(ids, f.id)
	}
	if got := strings.Join(ids, " "); got != "cpcbin cpcdsk utf8 png png2 gif apng psci" {
		t.Fatalf("formats: %s", got)
	}
	found := false
	for _, tl := range startTiles() {
		found = found || tl.mode == "cpc"
	}
	if !found || modeNames["cpc"] == "" {
		t.Fatal("start screen")
	}
}

// A file that only looks a bit like a CPC file opens as before.
func TestCPCNotOtherFiles(t *testing.T) {
	a := testApp(t, "pc")
	a.loadBytes([]byte("hello\r\n"), "x.bin", "", "auto")
	if a.mode != "pc" {
		t.Fatal(a.mode)
	}
	hdr := amsdosHeader("X", 3, 0x1000, 0x1000) // an AMSDOS binary that prints nothing
	a.loadBytes(append(hdr, 0xC9, 0, 0), "x.bin", "", "auto")
	if a.mode != "pc" {
		t.Fatal("a program that is not art")
	}
	// Open as Amstrad CPC: the bytes as the CPC's text codes.
	a.loadBytes([]byte("\x04\x00\x0f\x05HI"), "x.txt", "", "cpc")
	if !a.cpc() || a.cpcSet.mode != 0 || a.doc.get(1, 0) != makeCell('I', 5, 0, 0) {
		t.Fatalf("open as: %s %08x", a.mode, a.doc.get(1, 0))
	}
}

// With CHARCOAL_CPC_FILES=dir, every .bin, .dsk and .psci under dir opens, and
// its picture is written to /tmp/charcoal-cpc for a look (the AMSCII compo
// entries, fetched by hand: not in the repository).
func TestCPCRealFiles(t *testing.T) {
	root := os.Getenv("CHARCOAL_CPC_FILES")
	if root == "" {
		t.Skip("CHARCOAL_CPC_FILES not set")
	}
	out := "/tmp/charcoal-cpc"
	os.MkdirAll(out, 0o755)
	n := 0
	filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
		ext := strings.ToLower(filepath.Ext(p))
		if err != nil || fi.IsDir() || (ext != ".bin" && ext != ".dsk" && ext != ".psci") {
			return nil
		}
		a := testApp(t, "amiga")
		a.openPath(p, "auto")
		if !a.cpc() {
			t.Errorf("%s opened as %s: %s", p, a.mode, a.message)
			return nil
		}
		n++
		png, err := a.encode("png2")
		if err != nil {
			t.Fatal(err)
		}
		name := strings.ReplaceAll(strings.TrimPrefix(p, root), "/", "_") + ".png"
		os.WriteFile(filepath.Join(out, name), png, 0o644)
		t.Logf("%s: mode %d, inks %v, border %d; %s", p, a.cpcSet.mode, a.cpcSet.ink[:a.cpcSet.info().inks], a.cpcSet.border, a.message)
		return nil
	})
	t.Logf("%d files", n)
}

// FuzzCPC: the CPC readers on any bytes, with a .bin's header in front too.
func FuzzCPC(f *testing.F) {
	d, s := newCPCDoc(0), newCPCSetup(0)
	bin := writeCPCBin(d, s, "F")
	f.Add(bin)
	dsk, _ := writeDSK("F", bin)
	f.Add(dsk)
	f.Fuzz(func(t *testing.T, b []byte) {
		for _, in := range [][]byte{b, append(append(amsdosHeader("F", len(b)+21, 0x1000, 0x1000), cpcStub(len(b))...), b...)} {
			if d, _, _, err := parseCPCBin(in); err == nil && (d.rows != cpcRows || len(d.cells) != d.cols*cpcRows) {
				t.Fatal("not a screen")
			}
		}
		if f, _, err := dskArt(b); err == nil {
			parseCPCBin(f.data)
		}
		if art, err := parsePsci(b); err == nil && art.mode == "cpc" && art.cpc == nil {
			t.Fatal("no setup")
		}
	})
}
