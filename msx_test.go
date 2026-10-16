package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// msxFZ80 runs the little programs of MSX and MZ-700 files: the Z80
// instructions they use, with mem as the whole memory. bios is called for a
// CALL below 0x0200 (the MSX BIOS); it returns false to stop. The program also
// stops at JR $ (a program that stays) and at its last RET.
func msxFZ80(t *testing.T, mem []byte, pc, sp int, bios func(at int, r *msxFRegs) bool) *msxFRegs {
	t.Helper()
	r := &msxFRegs{}
	word := func(at int) int { return int(mem[at&0xFFFF]) | int(mem[(at+1)&0xFFFF])<<8 }
	push := func(v int) { sp -= 2; mem[sp] = byte(v); mem[sp+1] = byte(v >> 8) }
	depth := 0
	for steps := 0; steps < 1_000_000; steps++ {
		op := mem[pc]
		switch op {
		case 0xF3, 0xFB: // DI, EI
			pc++
		case 0x21:
			r.hl, pc = word(pc+1), pc+3
		case 0x11:
			r.de, pc = word(pc+1), pc+3
		case 0x01:
			r.bc, pc = word(pc+1), pc+3
		case 0x3E:
			r.a, pc = int(mem[pc+1]), pc+2
		case 0x32: // LD (nn),A
			mem[word(pc+1)] = byte(r.a)
			pc += 3
		case 0x18: // JR e
			if mem[pc+1] == 0xFE {
				return r
			}
			pc += 2 + int(int8(mem[pc+1]))
		case 0xED:
			switch mem[pc+1] {
			case 0xB0: // LDIR
				for {
					mem[r.de&0xFFFF] = mem[r.hl&0xFFFF]
					r.hl, r.de, r.bc = r.hl+1, r.de+1, (r.bc-1)&0xFFFF
					if r.bc == 0 {
						break
					}
				}
				pc += 2
			case 0x5B: // LD DE,(nn)
				r.de, pc = word(word(pc+2)), pc+4
			default:
				t.Fatalf("Z80: ED %02X at %04X", mem[pc+1], pc)
			}
		case 0xCD:
			to := word(pc + 1)
			if to < 0x200 {
				if !bios(to, r) {
					return r
				}
				pc += 3
				continue
			}
			push(pc + 3)
			depth++
			pc = to
		case 0xC9:
			if depth == 0 {
				return r
			}
			depth--
			pc, sp = word(sp), sp+2
		default:
			t.Fatalf("Z80: opcode %02X at %04X", op, pc)
		}
	}
	t.Fatal("Z80: the program does not stop")
	return r
}

type msxFRegs struct{ a, bc, de, hl int }

// msxFMachine is what an MSX shows after a program: the VRAM and the registers
// the BIOS calls set.
type msxFMachine struct {
	vram              []byte
	mode, width       int // 0 text, 1 SCREEN 1; LINLEN
	reg7              int // the colour register: text colour<<4 | backdrop
	keyWaited, inited bool
}

// msxFRun loads a .rom or .bin as an MSX would and runs it with a model of
// the BIOS routines it calls (INITXT/INIT32 as MSX BASIC's: the tables at the
// usual places, the ROM's font copied in, CHGCLR's register 7 as MSX BASIC
// writes it).
func msxFRun(t *testing.T, file []byte) *msxFMachine {
	t.Helper()
	mem := make([]byte, 0x10000)
	pc := 0
	switch {
	case isMSXROM(file):
		copy(mem[0x4000:], file)
		pc = int(binary.LittleEndian.Uint16(file[2:]))
	case isMSXBin(file):
		start := int(binary.LittleEndian.Uint16(file[1:]))
		end := int(binary.LittleEndian.Uint16(file[3:]))
		copy(mem[start:], file[7:7+end-start+1])
		pc = int(binary.LittleEndian.Uint16(file[5:]))
	default:
		t.Fatal("not an MSX file")
	}
	m := &msxFMachine{vram: make([]byte, 0x4000)}
	setw := func(at, v int) { mem[at], mem[at+1] = byte(v), byte(v>>8) }
	rom := msxFont.native()
	msxFZ80(t, mem, pc, 0xF000, func(at int, r *msxFRegs) bool {
		switch at {
		case msxERAFNK, msxKILBUF:
		case msxINITXT:
			if m.keyWaited {
				return false // back to BASIC
			}
			m.mode, m.width, m.inited = 0, int(mem[msxLINL40]), true
			setw(msxNAMBAS, 0)
			cg := 0x0800
			if m.width > 40 {
				cg = 0x1000
			}
			setw(msxCGPBAS, cg)
			copy(m.vram[cg:], rom)
		case msxINIT32:
			m.mode, m.width, m.inited = 1, int(mem[msxLINL32]), true
			setw(msxNAMBAS, 0x1800)
			setw(msxCGPBAS, 0)
			setw(msxT32COL, 0x2000)
			copy(m.vram, rom)
			for i := 0; i < 32; i++ {
				m.vram[0x2000+i] = mem[msxFORCLR]<<4 | mem[msxBAKCLR]
			}
		case msxCHGCLR:
			if m.mode == 1 {
				m.reg7 = int(mem[msxFORCLR])<<4 | int(mem[msxBDRCLR])
			} else {
				m.reg7 = int(mem[msxFORCLR])<<4 | int(mem[msxBAKCLR])
			}
		case msxLDIRVM:
			copy(m.vram[r.de:r.de+r.bc], mem[r.hl:r.hl+r.bc])
		case msxCHGET:
			m.keyWaited = true
		default:
			t.Fatalf("the program calls %04X", at)
		}
		return true
	})
	if !m.inited {
		t.Fatal("the program set no screen up")
	}
	return m
}

// msxFScreen draws what the modelled MSX shows, glyph pixels as colours, to be
// compared with the page drawn by CharEdit (the same size: one value a pixel).
func msxFScreen(m *msxFMachine, s msxSetup) (w, h int, px []int) {
	cols, cw := m.width, 6
	nam, cg := 0, 0x0800
	if m.mode == 1 {
		cols, cw, nam, cg = 32, 8, 0x1800, 0
	} else if cols > 40 {
		cg = 0x1000
	}
	w, h = cols*cw, msxRows*8
	px = make([]int, w*h)
	for y := 0; y < msxRows; y++ {
		for x := 0; x < cols; x++ {
			ch := int(m.vram[nam+y*cols+x])
			fg, bg := m.reg7>>4, m.reg7&15
			if m.mode == 1 {
				v := int(m.vram[0x2000+ch>>3])
				fg, bg = v>>4, v&15
				if fg == 0 {
					fg = m.reg7 & 15
				}
				if bg == 0 {
					bg = m.reg7 & 15
				}
			} else if fg == 0 {
				fg = bg
			}
			for r := 0; r < 8; r++ {
				bits := m.vram[cg+ch*8+r]
				for b := 0; b < cw; b++ {
					c := bg
					if bits&(0x80>>b) != 0 {
						c = fg
					}
					px[(y*8+r)*w+x*cw+b] = c
				}
			}
		}
	}
	return
}

// msxFPage draws the page as CharEdit does, as colour numbers (by RGB).
func msxFPage(t *testing.T, a *App) (w, h int, px []int) {
	t.Helper()
	fb := &FB{}
	W, H := a.cellW(), 8
	w, h = a.doc.cols*W, msxRows*H
	fb.resize(w, h*a.cellH()/8)
	for y := 0; y < msxRows; y++ {
		for x := 0; x < a.doc.cols; x++ {
			a.drawCell(fb, x*W, y*a.cellH(), 1, a.doc.get(x, y))
		}
	}
	idx := map[uint32]int{}
	for i := 15; i >= 1; i-- {
		idx[a.pal[i]] = i
	}
	idx[0xFF000000] = 1
	px = make([]int, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			v := fb.px[y*a.cellH()/8*fb.w+x]
			c, ok := idx[v]
			if !ok {
				t.Fatalf("colour %08X is not the palette's", v)
			}
			px[y*w+x] = c
		}
	}
	return
}

// msxFSame compares two screens of colour numbers (black and transparent the same).
func msxFSame(t *testing.T, what string, w, h int, got, want []int) {
	t.Helper()
	bad := 0
	for i := range want {
		g, wv := got[i], want[i]
		if g == 0 {
			g = 1
		}
		if wv == 0 {
			wv = 1
		}
		if g != wv {
			if bad < 5 {
				t.Errorf("%s: pixel %d,%d is colour %d, CharEdit shows %d", what, i%w, i/w, g, wv)
			}
			bad++
		}
	}
	if bad > 0 {
		t.Errorf("%s: %d pixels differ", what, bad)
	}
}

// msxFRandom fills an MSX page with random characters and colours.
func msxFRandom(a *App, r *rand.Rand) {
	for i := range a.doc.cells {
		a.doc.cells[i] = makeCell(r.Intn(256), 0, 0, 0)
	}
	a.msxSet.fg, a.msxSet.bg, a.msxSet.border = 1+r.Intn(15), 1+r.Intn(15), r.Intn(16)
	for i := range a.msxSet.ct {
		a.msxSet.ct[i] = byte(r.Intn(256))
	}
}

func TestMSXFonts(t *testing.T) {
	for _, f := range []*Font{fontByID("msxcbios"), fontByID("msxcbiosjp"), fontByID("msxcbiosbr")} {
		if f == nil || f.kind != "msx" || f.rows() != 8 {
			t.Fatalf("an MSX font is missing: %v", f)
		}
		if f.row(32, 3) != 0 || f.row('A', 3) == 0 {
			t.Errorf("%s: space or A is wrong", f.name)
		}
	}
	// 'A' of C-BIOS's international set, as in src/font_us.asm.
	want := []byte{0x70, 0x88, 0x88, 0xF8, 0x88, 0x88, 0x00, 0x00}
	for r, v := range want {
		if got := msxFont.row('A', r); got != v {
			t.Errorf("A row %d: %02X, want %02X", r, got, v)
		}
	}
	if len(msxRunes) != 256 {
		t.Errorf("msxRunes has %d characters", len(msxRunes))
	}
	for c := 32; c < 127; c++ {
		if msxRune(c) != rune(c) || msxFromRune(rune(c)) != c {
			t.Errorf("ASCII %d does not map to itself", c)
		}
	}
	if msxFromRune('█') != 0xDB || msxFromRune('Ç') != 0x80 || msxFromRune('┼') != 0x15 {
		t.Error("the graphics are not where the MSX has them")
	}
	for _, set := range msxFkeys {
		for _, c := range set {
			if c < 1 || c > 255 {
				t.Errorf("F-key character %d", c)
			}
		}
	}
}

func TestMSXPalettes(t *testing.T) {
	if msx1Palette.colors[4] != 0xFF5455ED || msx1Palette.colors[15] != 0xFFFFFFFF || msx1Palette.colors[1] != 0xFF000000 {
		t.Errorf("MSX1 palette: %08X %08X", msx1Palette.colors[4], msx1Palette.colors[15])
	}
	// The V9938's medium green is 0x611 (green 6, red 1, blue 1).
	if msx2Palette.colors[2] != 0xFF24DA24 || msx2Palette.colors[15] != 0xFFFFFFFF {
		t.Errorf("MSX2 palette: %08X", msx2Palette.colors[2])
	}
	a := testApp(t, "msx")
	if a.pal != msx1Palette.colors {
		t.Error("a new MSX page is not in the MSX1 palette")
	}
	a.paletteFor["msx"] = "vga"
	a.setPalette()
	if a.pal != msx1Palette.colors {
		t.Error("an MSX page took a PC palette")
	}
}

func TestMSXNewPage(t *testing.T) {
	a := testApp(t, "msx")
	if a.doc.cols != 40 || a.doc.rows != 24 || a.cellW() != 6 || a.cellH() != 8 || a.saveFmt != "msxrom" {
		t.Fatalf("new MSX page: %dx%d cells %dx%d %s", a.doc.cols, a.doc.rows, a.cellW(), a.cellH(), a.saveFmt)
	}
	for _, c := range []struct{ mode, cols, w, h int }{{1, 32, 8, 8}, {2, 80, 6, 16}, {0, 40, 6, 8}} {
		a.setMSXMode(c.mode)
		if a.doc.cols != c.cols || a.cellW() != c.w || a.cellH() != c.h || a.doc.rows != 24 {
			t.Errorf("mode %d: %dx%d cells %dx%d", c.mode, a.doc.cols, a.doc.rows, a.cellW(), a.cellH())
		}
	}
	// Width and height stay the machine's.
	a.setCols(60)
	a.setRows(30)
	if a.doc.cols != 40 || a.doc.rows != 24 {
		t.Errorf("the screen changed size: %dx%d", a.doc.cols, a.doc.rows)
	}
	// Typing.
	a.cx, a.cy = 0, 0
	for _, r := range "Hi█" {
		a.typeByte(a.charToByte(r))
	}
	if cellCh(a.doc.get(0, 0)) != 'H' || cellCh(a.doc.get(2, 0)) != 0xDB {
		t.Error("typing went wrong")
	}
}

func TestMSXColours(t *testing.T) {
	a := testApp(t, "msx")
	a.setFg(10)
	a.setBg(6)
	if a.msxSet.fg != 10 || a.msxSet.bg != 6 {
		t.Fatalf("SCREEN 0's colours: %d %d", a.msxSet.fg, a.msxSet.bg)
	}
	a.doUndo()
	if a.msxSet.fg != 10 || a.msxSet.bg != 4 {
		t.Errorf("undo of the background: %d %d", a.msxSet.fg, a.msxSet.bg)
	}
	a.doUndo()
	if a.msxSet.fg != 15 {
		t.Errorf("undo of the foreground: %d", a.msxSet.fg)
	}
	a.setMSXMode(1)
	a.brush = 'A' // group 8: 'A'-'G' are 65-71
	a.setFg(2)
	a.setBg(0)
	if a.msxSet.ct[8] != 0x20 || a.msxSet.ct[9] != 0xF4 {
		t.Errorf("SCREEN 1's colour table: %02X %02X", a.msxSet.ct[8], a.msxSet.ct[9])
	}
	a.setMSXBorder(13)
	// A cell of group 8 with a transparent background shows the border.
	fb := &FB{}
	fb.resize(8, 8)
	a.drawCell(fb, 0, 0, 1, makeCell('A', 0, 0, 0))
	if fb.px[0] != a.pal[13] {
		t.Errorf("transparent background: %08X", fb.px[0])
	}
	a.doUndo()
	a.doUndo()
	if a.msxSet.ct[8] != 0x24 || a.msxSet.border != 4 {
		t.Errorf("undo: %02X border %d", a.msxSet.ct[8], a.msxSet.border)
	}
	a.defaultColours()
	if a.msxSet.ct[8] != 0xF4 {
		t.Errorf("default colours: %02X", a.msxSet.ct[8])
	}
	a.doUndo()
	a.setMSXMode(0)
	a.doUndo() // the mode change
	if a.msxSet.mode != 1 || a.doc.cols != 32 {
		t.Errorf("undo of the mode: %d %d", a.msxSet.mode, a.doc.cols)
	}
}

// TestMSXPrograms runs every program CharEdit writes in the Z80 with a model of
// the BIOS and compares what the MSX would show with CharEdit's page, pixel
// for pixel; then reads the file back.
func TestMSXPrograms(t *testing.T) {
	r := rand.New(rand.NewSource(1985))
	for mode := 0; mode < 3; mode++ {
		for _, own := range []bool{false, true} {
			for _, fmtID := range []string{"msxrom", "msxbin"} {
				name := fmt.Sprintf("%s mode %d own %v", fmtID, mode, own)
				a := testApp(t, "msx")
				a.setMSXMode(mode)
				msxFRandom(a, r)
				if own {
					f := msxFont.clone()
					f.id, f.name = ownID, "mine"
					for c := 0; c < 256; c++ {
						for y := 0; y < 8; y++ {
							f.setRow(c, y, byte(r.Intn(256)))
						}
					}
					a.useOwnFont(f, nil, true)
				}
				out, err := a.encodeFor(fmtID, "", nil)
				if err != nil {
					t.Fatal(err)
				}
				if fmtID == "msxrom" && (len(out) != 16384 || out[0] != 'A' || out[1] != 'B') {
					t.Fatalf("%s: not a 16 KB cartridge", name)
				}
				m := msxFRun(t, out)
				if !own { // the BIOS's font is the machine's: draw the page in it
					a.fontFor["msx"] = "msxcbios"
				}
				w, h, want := msxFPage(t, a)
				gw, gh, got := msxFScreen(m, a.msxSet)
				if gw != w || gh != h {
					t.Fatalf("%s: the MSX shows %dx%d, CharEdit %dx%d", name, gw, gh, w, h)
				}
				msxFSame(t, name, w, h, got, want)
				if fmtID == "msxbin" && !m.keyWaited {
					t.Errorf("%s: the program does not wait for a key", name)
				}
				d, s, font, err := readMSXProgram(out)
				if err != nil {
					t.Fatalf("%s: %v", name, err)
				}
				if s != a.msxSet || !slicesEqualCells(d.cells, a.doc.cells) || (font != nil) != own {
					t.Errorf("%s: read back differently", name)
				}
			}
		}
	}
}

func slicesEqualCells(a, b []uint32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestMSXEditorRoundTrip saves every MSX format from the editor and opens it again.
func TestMSXEditorRoundTrip(t *testing.T) {
	r := rand.New(rand.NewSource(7))
	dir := t.TempDir()
	for _, fmtID := range []string{"msxrom", "msxbin", "psci"} {
		for mode := 0; mode < 3; mode++ {
			a := testApp(t, "msx")
			a.setMSXMode(mode)
			msxFRandom(a, r)
			if fmtID == "psci" { // Playscii has no transparent colour and no colour 0
				a.msxSet.border = 1 + r.Intn(15)
				for i := range a.msxSet.ct {
					a.msxSet.ct[i] = byte(1+r.Intn(15))<<4 | byte(1+r.Intn(15))
				}
			}
			want, set := a.doc.clone(), a.msxSet
			p := filepath.Join(dir, fmt.Sprintf("x%d.%s", mode, map[string]string{"msxrom": "rom", "msxbin": "bin", "psci": "psci"}[fmtID]))
			if !a.writeOut(p, fmtID) {
				t.Fatal(a.message)
			}
			b, _ := os.ReadFile(p)
			a2 := testApp(t, "amiga")
			a2.loadBytes(b, filepath.Base(p), p, "auto")
			if !a2.msx() {
				t.Fatalf("%s mode %d opened as %s: %s", fmtID, mode, a2.mode, a2.message)
			}
			if !slicesEqualCells(a2.doc.cells, want.cells) {
				t.Errorf("%s mode %d: other cells", fmtID, mode)
			}
			got := a2.msxSet
			if fmtID == "psci" && mode != 1 && (got.fg != set.fg || got.bg != set.bg) {
				t.Errorf("psci mode %d: colours %d/%d, want %d/%d", mode, got.fg, got.bg, set.fg, set.bg)
			}
			if fmtID == "psci" { // colours as Playscii's nearest: compare what shows
				for ch := 0; ch < 256; ch++ {
					f1, b1 := set.colours(ch)
					f2, b2 := got.colours(ch)
					if mode != 1 || !msxFUsed(want, ch) {
						continue
					}
					if f1 != f2 || b1 != b2 {
						t.Errorf("psci mode %d: character %d in %d/%d, want %d/%d", mode, ch, f2, b2, f1, b1)
						break
					}
				}
				continue
			}
			if got != set {
				t.Errorf("%s mode %d: setup %+v, want %+v", fmtID, mode, got, set)
			}
			if a2.saveFmt != fmtID {
				t.Errorf("%s: Save would write %s", fmtID, a2.saveFmt)
			}
		}
	}
}

func msxFUsed(d *Doc, ch int) bool {
	for _, v := range d.cells {
		if cellCh(v) == ch {
			return true
		}
	}
	return false
}

// TestMSXPsciCharset: Playscii's msx charset is the codes from 32 on, then 0-31.
func TestMSXPsciCharset(t *testing.T) {
	if psciMSXIndex(32) != 0 || psciMSXIndex(0) != 224 || msxOfPsci(224) != 0 || msxOfPsci(0) != 32 {
		t.Error("Playscii's msx charset order")
	}
	for c := 2; c < 16; c++ {
		if got := msxOfPsciColour(psciOfMSX(c)); got != c {
			t.Errorf("colour %d comes back as %d", c, got)
		}
	}
	// A real Playscii file of the msx charset, as Playscii writes it.
	src := `{"active_frame": 0, "active_layer": 0, "camera": [20, -12, 20], "charset": "msx", "frames": [{"delay": 0.1, "layers": [{"name": "L", "tiles": [` +
		strings.Repeat(`{"char": 33, "fg": 15, "bg": 8, "xform": 0}, `, 40*24-1) + `{"char": 187, "fg": 15, "bg": 8, "xform": 0}], "visible": 1, "z": 0}]}], "height": 24, "palette": "msx", "selected_bg_color": 1, "selected_char": 0, "selected_fg_color": 1, "selected_xform": 0, "width": 40}`
	art, err := parsePsci([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	if art.mode != "msx" || art.msx == nil || art.msx.mode != 0 {
		t.Fatalf("opened as %s", art.mode)
	}
	if cellCh(art.docs[0].cells[0]) != 'A' || cellCh(art.docs[0].cells[40*24-1]) != 0xDB {
		t.Errorf("characters: %d %d", cellCh(art.docs[0].cells[0]), cellCh(art.docs[0].cells[40*24-1]))
	}
	if art.msx.fg != 15 || art.msx.bg != 4 { // Playscii's white (15) and its blue (8: #2020FF)
		t.Errorf("colours %d on %d", art.msx.fg, art.msx.bg)
	}
}

func TestMSXOpenAsText(t *testing.T) {
	a := testApp(t, "amiga")
	a.loadBytes([]byte("HELLO\r\n\x01\x58MSX\n"), "x.txt", "", "msx")
	if !a.msx() || a.saveFmt != "utf8" {
		t.Fatalf("opened as %s, saved as %s", a.mode, a.saveFmt)
	}
	if cellCh(a.doc.get(0, 0)) != 'H' || cellCh(a.doc.get(0, 1)) != 0x18 || cellCh(a.doc.get(1, 1)) != 'M' {
		t.Error("the text is not on the screen")
	}
	// Auto: a text file is not an MSX file.
	b := testApp(t, "amiga")
	b.loadBytes([]byte("HELLO\r\n"), "x.txt", "", "auto")
	if b.msx() {
		t.Error("plain text opened as MSX")
	}
}

func TestMSXPicture(t *testing.T) {
	a := testApp(t, "msx")
	for mode, size := range [][2]int{{320, 240}, {320, 240}, {640, 480}} {
		a.setMSXMode(mode)
		fb := a.renderDoc(a.doc, 1)
		if fb.w != size[0] || fb.h != size[1] {
			t.Errorf("mode %d: picture %dx%d", mode, fb.w, fb.h)
		}
		if fb.px[0] != a.msxRGB(0) {
			t.Errorf("mode %d: the border is %08X", mode, fb.px[0])
		}
	}
}

func TestMSXAutosave(t *testing.T) {
	a := testApp(t, "msx")
	a.setMSXMode(1)
	a.msxSet.ct[3], a.msxSet.border = 0x9A, 7
	m := a.Tab.msxAutosave()
	if m.setup() != a.msxSet {
		t.Errorf("autosave: %+v", m.setup())
	}
}

func TestMSXROMFont(t *testing.T) {
	rom := make([]byte, 32768)
	binary.LittleEndian.PutUint16(rom[4:], 0x1BBF)
	copy(rom[0x1BBF:], msxCBIOSJP)
	if f := msxROMFont(rom); !bytes.Equal(f, msxCBIOSJP) {
		t.Error("the font at CGTABL was not found")
	}
	if msxROMFont(rom[:1000]) != nil {
		t.Error("a short file gave a font")
	}
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	os.MkdirAll(appDataDir(), 0o755)
	os.WriteFile(filepath.Join(appDataDir(), "msx.rom"), rom, 0o644)
	keep := fonts
	defer func() { fonts = keep }()
	if p := loadMSXROMFont(); p == "" || fontByID("msxrom") == nil {
		t.Error("the BIOS ROM's font was not added")
	}
}

// ---- the emulator check (CHARCOAL_EMU=1; tools/emucheck/README.md) ----

// msxFMame runs a cartridge in MAME's machine (with C-BIOS in place of its
// BIOS: tools/emucheck/get-msx-roms.sh) and gives the snapshot MAME takes when
// it stops.
func msxFMame(t *testing.T, s emuFSetup, machine, cart string, secs int) emuFImg {
	t.Helper()
	mame := "/usr/lib/mame/mame" // Arch's /usr/bin/mame is a script that writes ~/.mame first
	if _, err := os.Stat(mame); err != nil {
		mame = emuFNeed(t, "mame")
	}
	roms := filepath.Join(s.cache, "roms")
	if _, err := os.Stat(filepath.Join(roms, machine)); err != nil {
		t.Skip("no C-BIOS for MAME: run tools/emucheck/get-msx-roms.sh")
	}
	home := t.TempDir()
	snap := filepath.Join(home, "snap")
	out := emuFRun(t, 120, home, emuFEnv(home), mame, machine, "-rompath", roms, "-cart1", cart,
		"-video", "none", "-sound", "none", "-nothrottle", "-skip_gameinfo", "-seconds_to_run", fmt.Sprint(secs),
		"-snapshot_directory", snap, "-cfg_directory", filepath.Join(home, "cfg"),
		"-nvram_directory", filepath.Join(home, "nvram"))
	shots, _ := filepath.Glob(filepath.Join(snap, "*", "*.png"))
	if len(shots) == 0 {
		t.Fatalf("MAME took no snapshot:\n%s", out)
	}
	return emuFReadPNG(t, shots[len(shots)-1])
}

// TestEmuMSX runs CharEdit's cartridges in MAME (an HB-10P for SCREEN 0 and 1,
// a PHC-23 MSX2 for 80 columns, both on C-BIOS) and compares the screen with
// CharEdit's PNG, pixel for pixel, with the page drawn in C-BIOS's font (the
// BIOS's own) or carrying an own font.
func TestEmuMSX(t *testing.T) {
	s := emuFStart(t)
	r := rand.New(rand.NewSource(1983))
	for _, c := range []struct {
		name    string
		mode    int
		own     bool
		machine string
	}{
		{"msx-text", 0, false, "hb10p"}, {"msx-screen0", 0, false, "hb10p"}, {"msx-screen1", 1, false, "hb10p"},
		{"msx-screen1-own", 1, true, "hb10p"}, {"msx-screen0-own", 0, true, "hb10p"}, {"msx-width80", 2, false, "phc23"},
	} {
		t.Run(c.name, func(t *testing.T) {
			a := testApp(t, "msx")
			a.setMSXMode(c.mode)
			if c.name == "msx-text" {
				emuFText(a, "CharEdit's .rom in MAME (HB-10P,", "C-BIOS): the same page as", "CharEdit's PNG.")
			} else {
				msxFRandom(a, r)
				if c.mode == 1 {
					a.msxSet.border = 1 + r.Intn(15) // the backdrop: colour 0 shows it
				}
			}
			if c.own {
				f := msxFont.clone()
				f.id, f.name = ownID, "mine"
				for ch := 0; ch < 256; ch++ {
					for y := 0; y < 8; y++ {
						f.setRow(ch, y, byte(r.Intn(256)))
					}
				}
				a.useOwnFont(f, nil, true)
			}
			p := filepath.Join(t.TempDir(), "art.rom")
			if !a.writeOut(p, "msxrom") {
				t.Fatal(a.message)
			}
			g := msxFMame(t, s, c.machine, p, 6)
			ec := emuFCaseFor(a, c.name, [2]int{1, 1}, [2]int{1, 1})
			if c.mode == 2 && g.h < 384 { // the MSX2's lines once each: CharEdit's twice
				ec.syN, ec.syD = 1, 2
			}
			pairs := emuFCheck(t, s, ec, g)
			for _, pr := range pairs { // MAME's TMS9918 colours are msx1Palette's
				if c.machine == "hb10p" && pr.ours&0xFFFFFF != g.rgb(pr.theirs)&0xFFFFFF {
					t.Errorf("colour %06X is %06X in MAME", pr.ours&0xFFFFFF, g.rgb(pr.theirs)&0xFFFFFF)
				}
			}
		})
	}
}

// TestMSXMZSamples writes sample pages to look at (CHARCOAL_MSXMZ_SAMPLES=dir):
// an MSX cartridge in each mode and an MZ-700 tape file.
func TestMSXMZSamples(t *testing.T) {
	dir := os.Getenv("CHARCOAL_MSXMZ_SAMPLES")
	if dir == "" {
		t.Skip("CHARCOAL_MSXMZ_SAMPLES=dir writes the samples")
	}
	for mode := 0; mode < 3; mode++ {
		a := testApp(t, "msx")
		a.setMSXMode(mode)
		emuFText(a, "MSX "+a.msxSet.info().name, "CharEdit", "Blocks: █▄▀▌▐▘▝▖▗")
		if mode == 1 {
			for g := 0; g < 32; g++ {
				a.msxSet.ct[g] = byte((g%15+1)<<4 | (g+5)%15 + 1)
			}
			a.msxSet.border = 13
			for c := 0; c < 256; c++ {
				a.doc.cells[(10+c/32)*32+c%32] = makeCell(c, 0, 0, 0)
			}
		}
		if !a.writeOut(filepath.Join(dir, fmt.Sprintf("msx-mode%d.rom", mode)), "msxrom") {
			t.Fatal(a.message)
		}
	}
	a := testApp(t, "mz")
	a.fg, a.bg = 7, 1
	emuFText(a, "SHARP MZ-700", "CharEdit")
	for c := 0; c < 512; c++ {
		a.doc.cells[(9+c/40)*40+c%40] = mzCell(c, c%7+1, 0)
	}
	if !a.writeOut(filepath.Join(dir, "mz700.mzf"), "mzf") {
		t.Fatal(a.message)
	}
}

// TestMSXOwnFont: a 2048-byte set loads as the page's font and goes into the
// cartridge; the font editor opens on an MSX page and not on an MZ-700 page.
func TestMSXOwnFont(t *testing.T) {
	a := testApp(t, "msx")
	up, _, err := parseFontFile("set.bin", msxCBIOSJP, "msx", a.font())
	if err != nil {
		t.Fatal(err)
	}
	a.useOwnFont(up, nil, true)
	if a.font() != up || a.msxProgramFont() == nil || !bytes.Equal(a.msxProgramFont(), msxCBIOSJP) {
		t.Error("the own font is not the page's, or not the program's")
	}
	if _, _, err := parseFontFile("x.bin", make([]byte, 1000), "msx", nil); err == nil {
		t.Error("a 1000-byte file was taken as an MSX set")
	}
	openFontEditor()
	if a.dialog == nil {
		t.Error("the font editor does not open on an MSX page")
	}
	b := testApp(t, "mz")
	openFontEditor()
	if b.dialog != nil {
		t.Error("the font editor opened on an MZ-700 page")
	}
}
