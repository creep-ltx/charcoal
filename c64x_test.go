package main

import (
	"bytes"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// c64xFApp is a C64 page with the ROMs loaded (the test is skipped without them).
func c64xFApp(t *testing.T) *App {
	t.Helper()
	if !loadC64Fonts() {
		t.Skip("no C64 character ROM here")
	}
	a := testApp(t, "c64")
	a.hasC64 = true
	return a
}

// c64xFFont is a made-up character set: every glyph different.
func c64xFFont(seed int64) *Font {
	r := rand.New(rand.NewSource(seed))
	set := make([]byte, 2048)
	r.Read(set)
	return c64xFont(set, "test", "bin")
}

func c64xFPage(r *rand.Rand) *Doc {
	d := newDoc(40, 25)
	d.bg, d.border, d.glyph0 = r.Intn(16), r.Intn(16), true
	for i := range d.cells {
		d.cells[i] = makeCell(r.Intn(256), r.Intn(16), 0, 0)
	}
	return d
}

// c64xF6502 runs a C64 program in a small 6502 (the instructions the
// programs here use) from pc until it jumps to itself; mem is the whole
// 64 KB (I/O and colour RAM as plain memory).
func c64xF6502(t *testing.T, mem []byte, pc int) {
	t.Helper()
	var A, X, Y byte
	z := false
	w := func(a int) int { return int(mem[a&0xFFFF]) | int(mem[(a+1)&0xFFFF])<<8 }
	for steps := 0; steps < 2000000; steps++ {
		op := mem[pc]
		abs := w(pc + 1)
		switch op {
		case 0xA9:
			A, z, pc = mem[pc+1], mem[pc+1] == 0, pc+2
		case 0xA2:
			X, z, pc = mem[pc+1], mem[pc+1] == 0, pc+2
		case 0xA0:
			Y, z, pc = mem[pc+1], mem[pc+1] == 0, pc+2
		case 0x8D:
			mem[abs], pc = A, pc+3
		case 0x9D:
			mem[(abs+int(X))&0xFFFF], pc = A, pc+3
		case 0xBD:
			A = mem[(abs+int(X))&0xFFFF]
			z, pc = A == 0, pc+3
		case 0xE8:
			X++
			z, pc = X == 0, pc+1
		case 0x88:
			Y--
			z, pc = Y == 0, pc+1
		case 0xEE:
			mem[abs]++
			z, pc = mem[abs] == 0, pc+3
		case 0xE0:
			z, pc = X == mem[pc+1], pc+2
		case 0xD0, 0xF0:
			pc += 2
			if z == (op == 0xF0) {
				pc += int(int8(mem[pc-1]))
			}
		case 0x4C:
			if abs == pc {
				return
			}
			pc = abs
		default:
			t.Fatalf("6502: opcode %02X at %04X", op, pc)
		}
	}
	t.Fatal("6502: the program did not stop")
}

// c64xFRun loads a .prg and runs it from SYS 2061; it gives the memory.
func c64xFRun(t *testing.T, prg []byte) []byte {
	t.Helper()
	mem := make([]byte, 65536)
	// The VIC-II as the KERNAL leaves it (its table at $ECB9).
	for reg, v := range map[int]byte{0xD011: 0x1B, 0xD016: 0xC8, 0xD018: 0x14, 0xD020: 14, 0xD021: 6, 0xD022: 1, 0xD023: 2, 0xD024: 3} {
		mem[reg] = v
	}
	load := int(prg[0]) | int(prg[1])<<8
	copy(mem[load:], prg[2:])
	if load != 0x0801 || !bytes.Equal(mem[0x0805:0x080B], []byte{0x9E, '2', '0', '6', '1', 0}) {
		t.Fatalf("not a SYS 2061 program: % x", prg[:16])
	}
	c64xF6502(t, mem, 0x080D)
	return mem
}

// c64xFCheckScreen: the memory shows d's screen, the set-up and the character set (nil: the ROM's).
func c64xFCheckScreen(t *testing.T, what string, mem []byte, d *Doc, vic vicSetup, lower bool, f *Font) {
	t.Helper()
	chars, cols := c64Screen(d)
	screen := 0x0400
	if !bytes.Equal(mem[screen:screen+1000], chars) || !bytes.Equal(mem[0xD800:0xD800+1000], cols) {
		t.Fatalf("%s: the screen or colours are not in place", what)
	}
	want := map[int]int{0xD020: d.borderColour(), 0xD021: d.bg, 0xD011: 0x1B, 0xD016: 0xC8}
	if vic.Mode != vicHires {
		want[0xD022], want[0xD023] = vic.Regs[0], vic.Regs[1]
	}
	if vic.Mode == vicECM {
		want[0xD024], want[0xD011] = vic.Regs[2], 0x5B
	}
	if vic.Mode == vicMulti {
		want[0xD016] = 0xD8
	}
	switch {
	case f != nil:
		want[0xD018] = 0x18
		for c := 0; c < 256; c++ {
			for r := 0; r < 8; r++ {
				if mem[0x2000+c*8+r] != f.row(c, r) {
					t.Fatalf("%s: character %d row %d at $2000", what, c, r)
				}
			}
		}
	case lower:
		want[0xD018] = 0x17
	default:
		want[0xD018] = 0x14
	}
	for reg, v := range want {
		if int(mem[reg]) != v {
			t.Fatalf("%s: $%04X = $%02X, want $%02X", what, reg, mem[reg], v)
		}
	}
}

// The program's code is laid out as its comments say, and the bytes set per
// screen are the operands of LDA/LDY #.
func TestC64xPrgLayout(t *testing.T) {
	if len(c64xPrgCode) != c64xScreen || c64xScreen != 0x088C-0x0801+2 {
		t.Fatalf("the code is %d bytes, the screen at %d", len(c64xPrgCode), c64xScreen)
	}
	s := c64xSlot
	for _, o := range []int{s.border, s.bg, s.regs, s.regs + 5, s.regs + 10, s.d018, s.d016, s.d011} {
		if c64xPrgCode[o-1] != 0xA9 || c64xPrgCode[o+1] != 0x8D {
			t.Fatalf("slot %#x is not LDA #n / STA", o)
		}
	}
	if c64xPrgCode[s.pages-1] != 0xA0 {
		t.Fatal("pages is not LDY #n")
	}
	// The operands that name data addresses point where the data is.
	if w := int(c64xPrgCode[0x1A]) | int(c64xPrgCode[0x1B])<<8; w != 0x0801-2+c64xCharset {
		t.Fatalf("the character set is copied from $%04X", w)
	}
}

// Every combination of character set, mode and case runs on the (small)
// 6502 and puts up the screen as CharEdit shows it; reading the program
// back gives the same page.
func TestC64xPrgRuns(t *testing.T) {
	r := rand.New(rand.NewSource(64))
	for _, own := range []bool{false, true} {
		for mode := vicHires; mode <= vicECM; mode++ {
			for _, lower := range []bool{false, true} {
				d := c64xFPage(r)
				vic := vicSetup{Mode: mode, Regs: [3]int{r.Intn(16), r.Intn(16), r.Intn(16)}}
				var f *Font
				if own {
					f = c64xFFont(int64(mode))
				}
				prg := c64xPrg(d, lower, vic, f)
				what := vicModeNames[mode] + map[bool]string{true: " own set", false: " ROM"}[own]
				c64xFCheckScreen(t, what, c64xFRun(t, prg), d, vic, lower && !own, f)
				back, l, bv, set, err := parseC64xPrg(prg)
				if err != nil {
					t.Fatal(err)
				}
				if bv != vic || l != map[bool]int{true: 1}[lower && !own] || back.bg != d.bg || back.border != d.border {
					t.Fatalf("%s: read back %+v case %d", what, bv, l)
				}
				for i := range d.cells {
					if back.cells[i] != d.cells[i] {
						t.Fatalf("%s: cell %d", what, i)
					}
				}
				if own != (set != nil) || (own && !bytes.Equal(set, f.native())) {
					t.Fatalf("%s: the character set", what)
				}
				if isOurPrg(prg) || !isOurC64Prg(prg) {
					t.Fatalf("%s: taken for Petmate's form", what)
				}
			}
		}
	}
	// Petmate's own program puts its screen up in the same 6502 (the interpreter is right).
	d := c64xFPage(r)
	c64xFCheckScreen(t, "Petmate's form", c64xFRun(t, writePrg(d, true)), d, defaultVIC, true, nil)
	// Petmate's form is not CharEdit's.
	if isC64xPrg(writePrg(newDoc(40, 25), false)) {
		t.Fatal("Petmate's .prg read as CharEdit's")
	}
}

// The VIC-II's multicolour and ECM pixels.
func TestC64xCells(t *testing.T) {
	f := c64xFont(make([]byte, 2048), "t", "bin")
	f.setRow(1, 0, 0b00_01_10_11)
	f.setRow(1, 1, 0b10000001)
	var pix [64]int
	regs := [3]int{5, 6, 7}
	// Multicolour, colour 8-15: pairs.
	vicCell(f, makeCell(1, 10, 0, 0), vicMulti, 9, regs, &pix)
	if got := pix[:8]; !equalInts(got, []int{9, 9, 5, 5, 6, 6, 2, 2}) {
		t.Fatalf("multicolour pairs: %v", got)
	}
	// Multicolour, colour 0-7: hi-res.
	vicCell(f, makeCell(1, 3, 0, 0), vicMulti, 9, regs, &pix)
	if got := pix[8:16]; !equalInts(got, []int{3, 9, 9, 9, 9, 9, 9, 3}) {
		t.Fatalf("multicolour hi-res cell: %v", got)
	}
	// ECM: code 1+64*k on background k, the glyph is 1.
	for k, bg := range []int{9, 5, 6, 7} {
		vicCell(f, makeCell(1+64*k, 12, 0, 0), vicECM, 9, regs, &pix)
		if got := pix[8:16]; !equalInts(got, []int{12, bg, bg, bg, bg, bg, bg, 12}) {
			t.Fatalf("ECM background %d: %v", k, got)
		}
	}
}

// A .d64 holds the program: the directory, the chain and the BAM agree, for
// programs of one sector to many.
func TestC64xD64(t *testing.T) {
	for _, n := range []int{1, 253, 254, 255, 508, 4189, 30000} {
		prg := make([]byte, n)
		rand.New(rand.NewSource(int64(n))).Read(prg)
		img := writeD64("/x/My Art!.d64", prg)
		if len(img) != d64Size {
			t.Fatal("size")
		}
		files, err := d64Dir(img)
		if err != nil || len(files) != 1 {
			t.Fatalf("%d: %v %v", n, files, err)
		}
		f := files[0]
		if f.name != "MY ART!" || f.kind != 0x82 || !bytes.Equal(f.data, prg) {
			t.Fatalf("%d bytes: %q %02x %d bytes back", n, f.name, f.kind, len(f.data))
		}
		// The BAM: free sectors counted and marked, the file's and the directory's taken.
		bam := img[d64Offset(18, 0, 35):]
		free, sectors := 0, (n+253)/254
		for tr := 1; tr <= 35; tr++ {
			e := bam[4*tr : 4*tr+4]
			bits := 0
			for s := 0; s < d64Sectors[tr]; s++ {
				if e[1+s/8]&(1<<(s%8)) != 0 {
					bits++
				}
			}
			if bits != int(e[0]) {
				t.Fatalf("track %d: %d free, %d bits", tr, e[0], bits)
			}
			free += bits
		}
		if free != 683-2-sectors {
			t.Fatalf("%d bytes: %d sectors free", n, free)
		}
		dir := img[d64Offset(18, 1, 35):]
		if int(dir[30])|int(dir[31])<<8 != sectors || dir[3] != 17 || string(bam[0x90:0x97]) != "MY ART!" || bam[2] != 'A' {
			t.Fatalf("%d: directory size %d, first track %d", n, dir[30], dir[3])
		}
	}
	// Reading takes the first program CharEdit can open.
	d := c64xFPage(rand.New(rand.NewSource(1)))
	prg := c64xPrg(d, false, vicSetup{Mode: vicMulti, Regs: [3]int{1, 2, 3}}, nil)
	f, n, err := d64Art(writeD64("a.d64", prg))
	if err != nil || n != 1 || !bytes.Equal(f.data, prg) {
		t.Fatalf("d64Art: %v", err)
	}
	if _, _, err := d64Art(writeD64("a.d64", []byte{1, 8, 0, 0})); err == nil {
		t.Fatal("a disk without art")
	}
	// Broken images end quietly.
	img := writeD64("a.d64", prg)
	o := d64Offset(18, 1, 35)
	img[o], img[o+1] = 18, 1 // the directory points at itself
	if files, _ := d64Dir(img); len(files) != 1 {
		t.Fatal("a directory loop")
	}
}

// CharPad's projects: every version of c64lib's test pictures gives the
// same page, character set and set-up as version 5.
func TestC64xCharPad(t *testing.T) {
	files, _ := filepath.Glob("testdata/charpad/*.ctm")
	if len(files) < 30 {
		t.Fatalf("%d CharPad files", len(files))
	}
	type look struct {
		d   *Doc
		set []byte
		vic vicSetup
	}
	ref := map[string]look{}
	group := regexp.MustCompile(`-ctm(5|6|7|8|82|8-beta)\.ctm$`)
	for _, p := range files {
		b, _ := os.ReadFile(p)
		pr, err := parseCTM(b)
		if err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		d, note := pr.page()
		l := look{d, pr.chars, pr.vic()}
		if note != "" {
			t.Errorf("%s: %s", p, note)
		}
		g := group.ReplaceAllString(filepath.Base(p), "")
		if strings.HasSuffix(p, "ctm5.ctm") {
			ref[g] = l
		}
	}
	for _, p := range files {
		b, _ := os.ReadFile(p)
		pr, _ := parseCTM(b)
		d, _ := pr.page()
		g := group.ReplaceAllString(filepath.Base(p), "")
		r, ok := ref[g]
		if !ok {
			t.Fatalf("no version 5 of %s", g)
		}
		if !bytes.Equal(pr.chars, r.set) || pr.vic().Mode != r.vic.Mode || d.rows != r.d.rows {
			t.Errorf("%s: not as version 5 (%d characters, mode %d)", p, len(pr.chars)/8, pr.mode)
			continue
		}
		for i := range d.cells {
			if d.cells[i] != r.d.cells[i] {
				t.Errorf("%s: cell %d %08x, version 5 %08x", p, i, d.cells[i], r.d.cells[i])
				break
			}
		}
	}
	// What the pictures are: multicolour where the name says so, the map 40x25 in characters.
	b, _ := os.ReadFile("testdata/charpad/text-mc-per-char-notiles-ctm7.ctm")
	pr, _ := parseCTM(b)
	if pr.mode != vicMulti || pr.bg != 14 || pr.regs[0] != 12 || pr.regs[1] != 15 || pr.mapW != 40 || pr.mapH != 25 {
		t.Fatalf("the v7 header: %+v", pr)
	}
	// Bad files are refused, not panicked on.
	for _, bad := range [][]byte{[]byte("CTM\x04"), []byte("CTM\x07\x00\x00\x00\x00\x00\x00\x09\x00"), b[:100], append([]byte("CTM\x05"), make([]byte, 16)...)} {
		if _, err := parseCTM(bad); err == nil && len(bad) < 30 {
			t.Errorf("% x read", bad)
		}
	}
}

// The editor: modes from the page gadgets' functions, undo, ECM painting,
// every format that holds the mode and set saves and opens again; others keep it for the file.
func TestC64xEditor(t *testing.T) {
	a := c64xFApp(t)
	if a.vic != defaultVIC || !a.vicII() {
		t.Fatalf("a new page: %+v", a.vic)
	}
	a.fg = 10
	for _, c := range "Hi" {
		a.typeByte(a.charToByte(c))
	}
	a.setVICMode(vicMulti)
	a.setVICReg(0, 5)
	a.doUndo()
	if a.vic.Regs[0] != 1 || a.vic.Mode != vicMulti {
		t.Fatalf("undo a register: %+v", a.vic)
	}
	a.doUndo()
	if a.vic.Mode != vicHires {
		t.Fatal("undo the mode")
	}
	a.doRedo()
	a.doRedo()
	if a.vic.Mode != vicMulti || a.vic.Regs[0] != 5 {
		t.Fatal("redo")
	}
	// The page draws as the VIC-II: a multicolour cell's pairs.
	fb := a.renderDoc(a.doc, 1)
	var pix [64]int
	vicCell(a.font(), a.doc.get(0, 0), vicMulti, a.doc.bg, a.vic.Regs, &pix)
	for i, c := range pix {
		if got := fb.px[(32+i/8)*fb.w+32+i%8]; got != a.pal[c] {
			t.Fatalf("pixel %d: %08x, want %08x", i, got, a.pal[c])
		}
	}
	// ECM: typing gives the chosen background, the Colour tool's right button sets it.
	a.setVICMode(vicECM)
	a.ecmBg = 2
	a.cx, a.cy = 0, 1
	a.typeByte(a.charToByte('A'))
	if cellCh(a.doc.get(0, 1)) != 1|2<<6 {
		t.Fatalf("ECM typing: %02x", cellCh(a.doc.get(0, 1)))
	}
	a.tool, a.ecmBg = "colour", 3
	a.paintAt(0, 1, 3)
	if cellCh(a.doc.get(0, 1)) != 1|3<<6 {
		t.Fatalf("ECM colour tool: %02x", cellCh(a.doc.get(0, 1)))
	}
	a.tool = "draw"
	// An own character set; every format that holds it opens with it.
	own := c64xFFont(7)
	a.useOwnFont(own, nil, true)
	want, wantVIC := a.doc.clone(), a.vic
	dir := t.TempDir()
	for _, f := range []struct{ fmtID, name string }{{"prg", "art.prg"}, {"d64", "Art Disk.d64"}} {
		p := filepath.Join(dir, f.name)
		if !a.writeOut(p, f.fmtID) {
			t.Fatal(a.message)
		}
		b := c64xFApp(t)
		b.openPath(p, "auto")
		if !b.c64() || b.vic != wantVIC || b.saveFmt != f.fmtID || b.ownInUse() == nil || !bytes.Equal(b.ownInUse().native(), own.native()) {
			t.Fatalf("%s: %s %s %+v (%s)", f.name, b.mode, b.saveFmt, b.vic, b.message)
		}
		for i, v := range want.cells[:1000] {
			if b.doc.cells[i] != v {
				t.Fatalf("%s: cell %d %08x became %08x", f.name, i, v, b.doc.cells[i])
			}
		}
		if f.fmtID == "d64" {
			files, _ := d64Dir(b.source)
			if len(files) != 1 || files[0].name != "ART DISK" {
				t.Fatalf("the disk's file: %+v", files)
			}
		}
		b.typeByte('Z') // Ctrl+S writes the same again
		if !b.writeOut(p, b.saveFmt) {
			t.Fatal(b.message)
		}
		a = b
		a.useOwnFont(own, nil, true)
		a.doc = want.clone()
	}
	// A .seq cannot hold the mode: CharEdit keeps it for the file.
	p := filepath.Join(dir, "art.seq")
	if !a.writeOut(p, "seq") || !strings.Contains(a.message, "keeps it for the file") {
		t.Fatalf("seq: %s", a.message)
	}
	b := c64xFApp(t)
	prefs.VICModes = map[string]vicSetup{filepath.Clean(p): wantVIC} // testApp cleared prefs
	b.openPath(p, "auto")
	if b.vic != wantVIC {
		t.Fatalf("the .seq's mode: %+v", b.vic)
	}
	// The autosave keeps the mode.
	b.dirty = true
	if err := writeAutosave(b.Tab); err != nil {
		t.Fatal(err)
	}
	c := restart(t, "pc")
	c.hasC64 = true
	left := leftAutosaves()
	if len(left) != 1 || c.restoreAutosave(left[0]) != nil || c.vic != wantVIC {
		t.Fatalf("autosave: %+v", c.vic)
	}
}

// CharPad files open as a page and load as the page's font with the project's mode.
func TestC64xCharPadInEditor(t *testing.T) {
	a := c64xFApp(t)
	p, _ := filepath.Abs("testdata/charpad/text-mc-per-char-notiles-ctm7.ctm")
	a.openPath(p, "auto")
	if !a.c64() || a.vic.Mode != vicMulti || a.vic.Regs[0] != 12 || a.ownInUse() == nil || a.doc.bg != 14 || a.path != "" || a.saveFmt != "prg" {
		t.Fatalf("opened: %s %+v bg %d path %q (%s)", a.mode, a.vic, a.doc.bg, a.path, a.message)
	}
	b := c64xFApp(t)
	b.loadFontFile(p)
	if b.vic.Mode != vicMulti || b.ownInUse() == nil || b.ownInUse().row(1, 1) != a.ownInUse().row(1, 1) {
		t.Fatalf("as a font: %+v (%s)", b.vic, b.message)
	}
	if _, err := fontFileBytes(b.ownInUse(), nil, ".ctm"); err == nil {
		t.Fatal("a .ctm written")
	}
}

// The ACME source: a whole screen assembles (with ACME, when it is here) to a
// program that puts up the page; the BASIC listing's DATA hold the set and the screen.
func TestC64xAsmAndBasic(t *testing.T) {
	a := c64xFApp(t)
	r := rand.New(rand.NewSource(5))
	acme := os.Getenv("CHARCOAL_ACME")
	if acme == "" {
		acme, _ = exec.LookPath("acme")
	}
	for _, own := range []bool{false, true} {
		for mode := vicHires; mode <= vicECM; mode++ {
			a.doc = c64xFPage(r)
			a.vic = vicSetup{Mode: mode, Regs: [3]int{r.Intn(16), r.Intn(16), r.Intn(16)}}
			a.ownFont, a.ownLower, a.fontFor["c64"] = nil, nil, "c64upper"
			var f *Font
			if own {
				f = c64xFFont(int64(mode + 10))
				a.useOwnFont(f, nil, false)
			}
			src, _, _ := a.encodeC64x("marqasm", a.doc)
			if acme != "" {
				dir := t.TempDir()
				os.WriteFile(filepath.Join(dir, "x.s"), src, 0o644)
				cmd := exec.Command(acme, "-o", filepath.Join(dir, "x.prg"), "-f", "cbm", filepath.Join(dir, "x.s"))
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("acme: %v\n%s", err, out)
				}
				prg, _ := os.ReadFile(filepath.Join(dir, "x.prg"))
				c64xFCheckScreen(t, "acme "+vicModeNames[mode], c64xFRun(t, prg), a.doc, a.vic, false, f)
			}
			bas, _, _ := a.encodeC64x("marqbas", a.doc)
			var data []byte
			for _, m := range regexp.MustCompile(`(?m)^\d+ data (.*)$`).FindAllStringSubmatch(string(bas), -1) {
				for _, v := range strings.Split(m[1], ",") {
					n, _ := strconv.Atoi(v)
					data = append(data, byte(n))
				}
			}
			chars, cols := c64Screen(a.doc)
			if own {
				if !bytes.Equal(data[:2048], f.native()) || !strings.Contains(string(bas), "poke 56576,peek(56576)and252:poke 53272,20:poke 648,196") {
					t.Fatal("BASIC: the character set")
				}
				data = data[2048:]
			}
			for i := range chars {
				if data[2*i] != chars[i] || data[2*i+1] != cols[i] {
					t.Fatalf("BASIC: cell %d", i)
				}
			}
			if mode == vicMulti && !strings.Contains(string(bas), "poke 53270,216") || mode == vicECM && !strings.Contains(string(bas), "poke 53265,91") {
				t.Fatal("BASIC: the mode")
			}
		}
	}
	if acme == "" {
		t.Log("ACME not found (set CHARCOAL_ACME): the sources were not assembled")
	}
}

// FuzzC64x: CharPad projects and disk images from any bytes (FuzzParsers has them too; this one is quicker).
func FuzzC64x(f *testing.F) {
	files, _ := filepath.Glob("testdata/charpad/*.ctm")
	for _, p := range files {
		b, _ := os.ReadFile(p)
		f.Add(b)
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		if p, err := parseCTM(b); err == nil {
			d, _ := p.page()
			if d.cols != 40 || d.rows > maxRowsFor(40) || len(d.cells) < d.cols*d.rows {
				t.Fatalf("a page of %dx%d", d.cols, d.rows)
			}
			p.font("f")
		}
		img := make([]byte, d64Size)
		copy(img[d64Offset(18, 0, 35):], b)
		d64Art(img)
	})
}

// Mutated CharPad files, programs and disks never crash the readers (the fuzz
// engine stalls in this package, so a fixed number of random mutations as well).
func TestC64xMutations(t *testing.T) {
	r := rand.New(rand.NewSource(99))
	files, _ := filepath.Glob("testdata/charpad/*.ctm")
	var seeds [][]byte
	for _, p := range files {
		b, _ := os.ReadFile(p)
		seeds = append(seeds, b)
	}
	d := c64xFPage(r)
	prg := c64xPrg(d, false, vicSetup{Mode: vicMulti}, c64xFFont(1))
	seeds = append(seeds, prg)
	disk := writeD64("x.d64", prg)
	n := 20000
	if testing.Short() {
		n = 2000
	}
	for i := 0; i < n; i++ {
		b := append([]byte(nil), seeds[r.Intn(len(seeds))]...)
		for k := r.Intn(4) + 1; k > 0; k-- {
			switch o := r.Intn(len(b)); r.Intn(3) {
			case 0:
				b[o] = byte(r.Intn(256))
			case 1:
				b = b[:o]
			default:
				b[o] ^= 1 << r.Intn(8)
			}
			if len(b) == 0 {
				break
			}
		}
		if p, err := parseCTM(b); err == nil {
			pd, _ := p.page()
			if pd.cols != 40 || len(pd.cells) < 40*pd.rows {
				t.Fatal("a broken page")
			}
		}
		parseC64xPrg(b)
		img := append([]byte(nil), disk...)
		for k := 0; k < 8; k++ {
			img[d64Offset(18, 0, 35)+r.Intn(512)] = byte(r.Intn(256))
			img[r.Intn(len(img))] = byte(r.Intn(256))
		}
		if f, _, err := d64Art(img); err == nil {
			readC64Prg(f.data)
		}
	}
}

// TestC64xSamples writes sample programs for screenshots when CHARCOAL_C64X_SAMPLES names a folder.
func TestC64xSamples(t *testing.T) {
	dir := os.Getenv("CHARCOAL_C64X_SAMPLES")
	if dir == "" {
		t.Skip("set CHARCOAL_C64X_SAMPLES")
	}
	text := func(d *Doc, x, y int, s string, col int) {
		for i, c := range strings.ToUpper(s) {
			sc := int(c)
			if c >= 'A' && c <= 'Z' {
				sc = int(c - 'A' + 1)
			}
			d.set(x+i, y, makeCell(sc, col, 0, 0))
		}
	}
	// Multicolour in the ROM's characters: graphics in 8-15, text hi-res.
	d := newDoc(40, 25)
	d.bg, d.border, d.glyph0 = 0, 11, true
	for y := 0; y < 25; y++ {
		for x := 0; x < 40; x++ {
			d.set(x, y, makeCell([]int{0x66, 0x5F, 0x69, 0xE6, 0x7F, 0xA0}[(x/4+y/3)%6], 8+(x+y)%8, 0, 0))
		}
	}
	text(d, 10, 12, " MULTICOLOUR TEXT ", 1)
	os.WriteFile(filepath.Join(dir, "mc-rom.prg"), c64xPrg(d, false, vicSetup{Mode: vicMulti, Regs: [3]int{2, 7}}, nil), 0o644)
	// ECM: the same letters on four backgrounds.
	d = newDoc(40, 25)
	d.bg, d.border, d.glyph0 = 6, 14, true
	for y := 0; y < 25; y++ {
		for x := 0; x < 40; x++ {
			d.set(x, y, makeCell(32|(x/10)<<6, 1, 0, 0))
		}
	}
	for k := 0; k < 4; k++ {
		for y := 2; y < 23; y += 4 {
			text(d, k*10+1, y, "ECM BG"+strconv.Itoa(k), 1+y%14)
			for x := k*10 + 1; x < k*10+9; x++ {
				v := d.get(x, y)
				d.set(x, y, v&^0xC0|uint32(k)<<6)
			}
		}
	}
	os.WriteFile(filepath.Join(dir, "ecm-rom.prg"), c64xPrg(d, false, vicSetup{Mode: vicECM, Regs: [3]int{2, 5, 4}}, nil), 0o644)
	// A character set of its own, multicolour: shaded pairs.
	set := make([]byte, 2048)
	for c := 0; c < 256; c++ {
		for r := 0; r < 8; r++ {
			pair := byte((c + r/2) % 4)
			set[c*8+r] = pair<<6 | pair<<4 | byte((c/4+r)%4)<<2 | byte((c/16+r/4)%4)
		}
	}
	d = newDoc(40, 25)
	d.bg, d.border, d.glyph0 = 0, 0, true
	for y := 0; y < 25; y++ {
		for x := 0; x < 40; x++ {
			d.set(x, y, makeCell((x*7+y*3)&255, 8+(x/5)%8, 0, 0))
		}
	}
	os.WriteFile(filepath.Join(dir, "mc-own.prg"), c64xPrg(d, false, vicSetup{Mode: vicMulti, Regs: [3]int{11, 12}}, c64xFont(set, "shades", "bin")), 0o644)
	os.WriteFile(filepath.Join(dir, "mc-own.d64"), writeD64("mc own", c64xPrg(d, false, vicSetup{Mode: vicMulti, Regs: [3]int{11, 12}}, c64xFont(set, "shades", "bin"))), 0o644)
}
