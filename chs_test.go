package main

// Tests of the character set work of 2026-10-16 (branch r5-charsets): the
// MZ-700's second set without Sharp's ROM (mz.go), a Plus/4 page's own
// character set (tedchars.go) and C64 DirArt (dirart.go).

import (
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// ---------------------------------------------------------------- MZ-700

func TestChsMZSecondSet(t *testing.T) {
	// Every character but the space is drawn, and (nearly) all differ.
	seen := map[string]int{}
	for c := 0; c < 256; c++ {
		var g []byte
		ink := 0
		for r := 0; r < 8; r++ {
			v := mzFreeSet2.row(mzCode(c), r)
			g = append(g, v)
			for ; v != 0; v &= v - 1 {
				ink++
			}
		}
		if (ink == 0) != (c == 0) {
			t.Errorf("display code %02X of the second set has %d pixels", c, ink)
		}
		seen[string(g)]++
	}
	if len(seen) < 250 {
		t.Errorf("only %d different glyphs in the second set", len(seen))
	}
	// The outlined letters are walls around a middle line: the A's (code 1)
	// middle line is paper, its wall ink.
	if mzFreeSet2.row(mzCode(1), 4)&0x40 != 0 || mzFreeSet2.row(mzCode(1), 4)&0x80 == 0 {
		t.Errorf("outlined A row 4 = %08b", mzFreeSet2.row(mzCode(1), 4))
	}
	// Turned copies: the left rocket is the up rocket turned a quarter.
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			up := mzFreeSet2.row(mzCode(0x3F), x)&(0x80>>(7-y)) != 0
			left := mzFreeSet2.row(mzCode(0x40), y)&(0x80>>x) != 0
			if up != left {
				t.Fatalf("rocket ships: pixel %d,%d", x, y)
			}
		}
	}
	// The faces' quarters meet: the top halves' bottom rows and the bottom
	// halves' top rows are the same outline column.
	if mzFreeSet2.row(mzCode(0x91), 7)&0x80 == 0 || mzFreeSet2.row(mzCode(0x95), 0)&0x80 == 0 {
		t.Error("the face's left edge does not run through")
	}
	// Without the ROM a page draws the second set with it; the ROM still wins.
	a := testApp(t, "mz")
	if a.mzSet2Font() != mzFreeSet2 {
		t.Error("the second set without the ROM")
	}
	rom := make([]byte, 4096)
	for i := 8; i < 4096; i++ {
		rom[i] = byte(i * 7)
	}
	s1, s2 := mzROMFonts(rom, "chsmzrom", "test ROM")
	fonts = append(fonts, s1)
	defer func() { fonts = fonts[:len(fonts)-1]; delete(mzROMSet2, s1.id) }()
	mzROMSet2[s1.id] = s2
	a.fontFor["mz"] = s1.id
	if a.font() != s1 || a.mzSet2Font() != s2 {
		t.Error("Sharp's ROM does not win over the look-alike")
	}
}

// TestChsMZFontFile: the font file is what tools/mz700-set2.py makes of
// tools/mz700-set2.txt (when Python is here), so the two never drift apart.
func TestChsMZFontFile(t *testing.T) {
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("no python3")
	}
	out := filepath.Join(t.TempDir(), "set2.font")
	if b, err := exec.Command(py, "tools/mz700-set2.py", "tools/mz700-set2.txt", out).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, b)
	}
	b, _ := os.ReadFile(out)
	if !bytes.Equal(b, mzSet2Data) {
		t.Error("fonts/mz700-set2.font is not tools/mz700-set2.txt: run python3 tools/mz700-set2.py")
	}
}

// TestChsMZRealFiles opens real MZ-700 pictures (CHARCOAL_MZ_FILES, as
// mz_test.go) and writes their PNGs without the ROM to /tmp/charcoal-chs.
func TestChsMZRealFiles(t *testing.T) {
	dir := os.Getenv("CHARCOAL_MZ_FILES")
	if dir == "" {
		t.Skip("CHARCOAL_MZ_FILES not set")
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*.mzf"))
	for _, f := range files {
		b, _ := os.ReadFile(f)
		a := testApp(t, "amiga")
		a.loadBytes(b, filepath.Base(f), f, "auto")
		if !a.mz() {
			t.Fatalf("%s did not open", f)
		}
		n := 0
		for _, v := range a.doc.cells {
			if v&mzSet2 != 0 {
				n++
			}
		}
		out := filepath.Join(os.TempDir(), "charcoal-chs", strings.TrimSuffix(filepath.Base(f), ".mzf")+"-mz.png")
		os.MkdirAll(filepath.Dir(out), 0o755)
		a.writeOut(out, "png")
		t.Logf("%s: %d cells of the second set; %s", filepath.Base(f), n, out)
	}
}

// ---------------------------------------------------------------- Plus/4 character sets

// chsF6502 runs a program in a small 6502 (the instructions CharEdit's
// Plus/4 programs use) from pc until it jumps to itself.
func chsF6502(t *testing.T, mem []byte, pc int) {
	t.Helper()
	var A, X, Y byte
	z := false
	for steps := 0; steps < 1000000; steps++ {
		op := mem[pc]
		abs := int(mem[pc+1]) | int(mem[(pc+2)&0xFFFF])<<8
		switch op {
		case 0x78: // SEI
			pc++
		case 0xA9:
			A, z, pc = mem[pc+1], mem[pc+1] == 0, pc+2
		case 0xA2:
			X, z, pc = mem[pc+1], mem[pc+1] == 0, pc+2
		case 0xAD:
			A, pc = mem[abs], pc+3
			z = A == 0
		case 0xBD:
			A, pc = mem[(abs+int(X))&0xFFFF], pc+3
			z = A == 0
		case 0x8D:
			mem[abs], pc = A, pc+3
		case 0x9D:
			mem[(abs+int(X))&0xFFFF], pc = A, pc+3
		case 0x29:
			A, pc = A&mem[pc+1], pc+2
			z = A == 0
		case 0x09:
			A, pc = A|mem[pc+1], pc+2
			z = A == 0
		case 0xA0:
			Y, z, pc = mem[pc+1], mem[pc+1] == 0, pc+2
		case 0x88:
			Y, pc = Y-1, pc+1
			z = Y == 0
		case 0xE8:
			X, pc = X+1, pc+1
			z = X == 0
		case 0xE0:
			z, pc = X == mem[pc+1], pc+2
		case 0xD0:
			pc += 2
			if !z {
				pc += int(int8(mem[pc-1]))
			}
		case 0x4C:
			if abs == pc {
				return
			}
			pc = abs
		default:
			t.Fatalf("opcode %02X at %04X", op, pc)
		}
	}
	t.Fatal("the program does not end in a loop")
}

// chsFTEDSet is a made-up Plus/4 set: 128 glyphs and the TED's reverse, or
// 256 of their own.
func chsFTEDSet(seed int64, all bool) *Font {
	r := rand.New(rand.NewSource(seed))
	n := 1024
	if all {
		n = 2048
	}
	set := make([]byte, n)
	r.Read(set)
	return tedFontOf(set, "test", "bin")
}

func TestChsTEDProgram(t *testing.T) {
	for seed := int64(1); seed <= 4; seed++ {
		all := seed%2 == 0
		f := chsFTEDSet(seed, all)
		if tedHalfReversed(f) == all {
			t.Fatalf("seed %d: half reversed %v", seed, !all)
		}
		d := tedFPage(seed, 40, 25, false)
		prg := tedxPrg(d, f)
		if want := tedxCharset + map[bool]int{false: 1024, true: 2048}[all]; len(prg) != want {
			t.Fatalf("seed %d: %d bytes, want %d", seed, len(prg), want)
		}
		back, set, err := parseTEDxPrg(prg)
		if err != nil || back.bg != d.bg || back.border != d.border || tedFSame(d, back, false) != "" {
			t.Fatalf("seed %d: read back %v %s", seed, err, tedFSame(d, back, false))
		}
		g := tedFontOf(set, "x", "prg")
		for c := 0; c < 256; c++ {
			for r := 0; r < 8; r++ {
				if g.row(c, r) != f.row(c, r) {
					t.Fatalf("seed %d: the set's character %d", seed, c)
				}
			}
		}
		if !isTEDPrg(prg) || isOurC64Prg(prg) || isTEDPrg(prg[:len(prg)-1]) {
			t.Fatalf("seed %d: told apart wrongly", seed)
		}
		// Run it: the screen, the attributes, the set at $2000 and the TED's registers.
		mem := make([]byte, 65536)
		copy(mem[0x1001:], prg[2:])
		mem[0xFF12], mem[0xFF13], mem[0xFF07] = 0xC4, 0xD1, 0x08 // the KERNAL's (ROM characters, ROM on)
		if all {
			mem[0xFF07] = 0x08
		} else {
			mem[0xFF07] = 0x88 // set before: the program must clear it
		}
		chsF6502(t, mem, 0x100D)
		if mem[0xFF15] != byte(d.bg) || mem[0xFF19] != byte(d.border) {
			t.Fatalf("seed %d: colours %02x %02x", seed, mem[0xFF15], mem[0xFF19])
		}
		if mem[0xFF12]&4 != 0 || mem[0xFF12]&^4 != 0xC0 || mem[0xFF13] != 0x21 || (mem[0xFF07]&0x80 != 0) != all || mem[0xFF07]&0x7F != 0x08 {
			t.Fatalf("seed %d: $FF12 %02x $FF13 %02x $FF07 %02x", seed, mem[0xFF12], mem[0xFF13], mem[0xFF07])
		}
		for i, v := range d.cells {
			if mem[0x0C00+i] != byte(cellCh(v)) || mem[0x0800+i] != byte(tedAttr(v)) {
				t.Fatalf("seed %d: cell %d in memory", seed, i)
			}
		}
		if !bytes.Equal(mem[0x2000:0x2000+len(set)], set) {
			t.Fatalf("seed %d: the set at $2000", seed)
		}
	}
}

// chsFTEDApp is a Plus/4 page with a set of its own.
func chsFTEDApp(t *testing.T, seed int64, all bool) (*App, *Font) {
	a := tedFApp(t)
	a.doc = tedFPage(seed, 40, 25, false)
	f := chsFTEDSet(seed, all)
	a.useOwnFont(f, nil, true)
	return a, f
}

func chsFSameFont(t *testing.T, what string, f, g *Font) {
	t.Helper()
	if g == nil {
		t.Fatalf("%s: no own font", what)
	}
	for c := 0; c < 256; c++ {
		for r := 0; r < 8; r++ {
			if g.row(c, r) != f.row(c, r) {
				t.Fatalf("%s: character %d row %d", what, c, r)
			}
		}
	}
}

func TestChsTEDEditor(t *testing.T) {
	for _, all := range []bool{false, true} {
		for _, fmtID := range []string{"prg", "d64"} {
			a, f := chsFTEDApp(t, 7, all)
			d := a.doc.clone()
			p := filepath.Join(t.TempDir(), "art."+fmtID)
			if !a.writeOut(p, fmtID) {
				t.Fatal(a.message)
			}
			b := chsFOpen(t, p)
			if !b.ted() || tedFSame(d, b.doc, false) != "" {
				t.Fatalf("%s: reopened as %s %s: %s", fmtID, b.mode, b.machine, tedFSame(d, b.doc, false))
			}
			chsFSameFont(t, fmtID, f, b.ownInUse())
		}
	}
	// Without a set of its own a page keeps Marq's form.
	a := tedFApp(t)
	out, err := a.encodeFor("prg", "", nil)
	if err != nil || len(out) != tedPrgSize {
		t.Errorf("a plain Plus/4 page: %d bytes", len(out))
	}
}

// chsFOpen is a new app (as a fresh start) with the file at p opened.
func chsFOpen(t *testing.T, p string) *App {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	tedFEnsureROM(t)
	n := testApp(t, "amiga")
	n.hasC64 = true
	n.loadBytes(b, filepath.Base(p), p, "auto")
	return n
}

func TestChsTEDSE(t *testing.T) {
	a, f := chsFTEDApp(t, 9, false)
	d := a.doc.clone()
	dir := t.TempDir()
	p := filepath.Join(dir, "art.scrn")
	if !a.writeOut(p, "tedse") {
		t.Fatal(a.message)
	}
	if !strings.Contains(a.message, "art.chrs") {
		t.Errorf("the note: %s", a.message)
	}
	proj, _ := os.ReadFile(filepath.Join(dir, "art.proj"))
	chrs, _ := os.ReadFile(filepath.Join(dir, "art.chrs"))
	if len(proj) != 23 || proj[2] != 1 || len(chrs) != 2+1024 || chrs[0] != 0x00 || chrs[1] != 0xB8 {
		t.Fatalf("project %d bytes (changed %v), set %d bytes", len(proj), proj[2:3], len(chrs))
	}
	b := chsFOpen(t, p)
	if !b.ted() || tedFSame(d, b.doc, false) != "" {
		t.Fatalf("reopened: %s", tedFSame(d, b.doc, false))
	}
	chsFSameFont(t, "tedse", f, b.ownInUse())
	// A set of 256 of its own: the .chrs keeps 128 and says so.
	a, _ = chsFTEDApp(t, 10, true)
	if !a.writeOut(p, "tedse") || !strings.Contains(a.message, "128") {
		t.Errorf("256 characters in a .chrs: %s", a.message)
	}
	// The .chrs as a font file, both ways.
	g, _, err := parseFontFile("x.chrs", chrs, "c64", nil)
	if err != nil {
		t.Fatal(err)
	}
	chsFSameFont(t, "font file", f, g)
	if out, err := fontFileBytes(g, nil, ".chrs"); err != nil || !bytes.Equal(out, chrs) || fontExt(g) != ".chrs" {
		t.Errorf("writing a .chrs: %v %v %s", err, bytes.Equal(out, chrs), fontExt(g))
	}
	// A project without a changed set opens in the ROM's characters.
	a = tedFApp(t)
	a.doc = d.clone()
	if !a.writeOut(p, "tedse") {
		t.Fatal(a.message)
	}
	proj, _ = os.ReadFile(filepath.Join(dir, "art.proj"))
	if proj[2] != 0 || chsFOpen(t, p).ownInUse() != nil {
		t.Error("a project without its own set")
	}
}

func TestChsTEDFontEditor(t *testing.T) {
	a := tedFApp(t)
	ed := &FontEd{}
	ed.change(func(f *Font) { glyphSetPixel(f, 1, 0, 0, !glyphPixel(f, 1, 0, 0)) })
	f := a.ownInUse()
	if f == nil || !tedHalfReversed(f) {
		t.Fatal("the reversed A did not follow the A")
	}
	ed.change(func(f *Font) { glyphClear(f, 129) })
	if f.row(1, 3) != 0xFF || !tedHalfReversed(f) {
		t.Error("the A did not follow its reverse")
	}
	ed.undoStep()
	ed.undoStep()
	if !tedHalfReversed(f) || f.row(1, 0) != fontByID("plus4upper").row(1, 0) {
		t.Error("undo")
	}
	// A C64 page's characters stay apart (the VIC-II has no reverse of its own).
	c := testApp(t, "c64")
	c.hasC64 = true
	(&FontEd{}).change(func(f *Font) { glyphClear(f, 1) })
	if g := c.ownInUse(); g == nil || g.row(129, 0) == 0xFF {
		t.Error("a C64 page's reversed A followed")
	}
}

func TestChsTEDBasicAndAsm(t *testing.T) {
	for _, all := range []bool{false, true} {
		a, f := chsFTEDApp(t, 11, all)
		bas, err := a.encodeFor("marqbas", "", nil)
		if err != nil {
			t.Fatal(err)
		}
		set := tedSetBytes(f)
		// The DATA give the set and then the screen, in the order the READs take them.
		var nums []int
		lines := strings.Split(strings.TrimSpace(string(bas)), "\n")
		last := 0
		for _, l := range lines {
			var n int
			fmt.Sscanf(l, "%d", &n)
			if n <= last {
				t.Fatalf("line %d after %d", n, last)
			}
			last = n
			if _, data, ok := strings.Cut(l, " data "); ok {
				for _, v := range strings.Split(data, ",") {
					var x int
					fmt.Sscanf(v, "%d", &x)
					nums = append(nums, x)
				}
			}
		}
		if len(nums) != len(set)+2000 {
			t.Fatalf("%d DATA values, want %d", len(nums), len(set)+2000)
		}
		for i, v := range set {
			if nums[i] != int(v) {
				t.Fatalf("set byte %d", i)
			}
		}
		if !strings.Contains(string(bas), "poke 57344+i,a") || !strings.Contains(string(bas), map[bool]string{false: "and127", true: "or128"}[all]) {
			t.Errorf("the set's POKEs:\n%s", bas[:400])
		}
		asm, _ := a.encodeFor("marqasm", "", nil)
		if !strings.Contains(string(asm), "*=$2000") || !strings.Contains(string(asm), "\tand\t#$fb\n") {
			t.Errorf("ACME source without the set")
		}
		_, tail, _ := strings.Cut(string(asm), "*=$2000")
		if n := strings.Count(tail, "!byte $"); n != len(set)/8 {
			t.Errorf("%d rows of the set in the ACME source", n)
		}
		// Assembled when ACME is here: the program equals CharEdit's .prg's work.
		acme := os.Getenv("CHARCOAL_ACME")
		if acme == "" {
			acme, _ = exec.LookPath("acme")
		}
		if acme == "" {
			continue
		}
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "x.s"), asm, 0o644)
		if out, err := exec.Command(acme, "-o", filepath.Join(dir, "x.prg"), "-f", "cbm", filepath.Join(dir, "x.s")).CombinedOutput(); err != nil {
			t.Fatalf("acme: %v %s", err, out)
		}
		prg, _ := os.ReadFile(filepath.Join(dir, "x.prg"))
		mem := make([]byte, 65536)
		copy(mem[0x1001:], prg[2:])
		mem[0xFF12], mem[0xFF13] = 0xC4, 0xD1
		chsF6502(t, mem, 0x100D)
		if !bytes.Equal(mem[0x2000:0x2000+len(set)], set) || mem[0xFF12]&4 != 0 || mem[0xFF13]&0xFC != 0x20 || (mem[0xFF07]&0x80 != 0) != all {
			t.Errorf("ACME's program: $FF12 %02x $FF13 %02x $FF07 %02x", mem[0xFF12], mem[0xFF13], mem[0xFF07])
		}
		for i, v := range a.doc.cells {
			if mem[0x0C00+i] != byte(cellCh(v)) || mem[0x0800+i] != byte(tedAttr(v)) {
				t.Fatalf("ACME's program: cell %d", i)
			}
		}
	}
}

// TestEmuChsPlus4 runs CharEdit's Plus/4 programs with a set of their own in
// VICE's xplus4 (CHARCOAL_EMU=1, as emu_test.go) and compares the screen with
// the PNG export: 128 characters with the TED's reverse, 256 of their own,
// the .d64, and the BASIC and ACME exports when petcat and ACME are there.
func TestEmuChsPlus4(t *testing.T) {
	s := emuFStart(t)
	xplus4 := emuFNeed(t, "xplus4")
	r := rand.New(rand.NewSource(4))
	for _, c := range []struct {
		name  string
		all   bool
		fmtID string
	}{
		{"plus4-own-128", false, "prg"},
		{"plus4-own-256", true, "prg"},
		{"plus4-disk-own-128", false, "d64"},
		{"plus4-basic-own-256", true, "marqbas"},
		{"plus4-acme-own-128", false, "marqasm"},
		{"plus4-text", false, "prg"},
	} {
		t.Run(c.name, func(t *testing.T) {
			a, _ := chsFTEDApp(t, r.Int63(), c.all)
			if !tedFReal {
				t.Skip("not the Plus/4's real character ROM")
			}
			for i := range a.doc.cells { // no flashing: the screenshot's moment is not known
				a.doc.cells[i] &^= 0x8000
			}
			if c.name == "plus4-text" {
				a.useOwnFont(tedFontOf(fontByID("plus4upper").native()[:1024], "upper", "bin"), nil, true)
				ed := &FontEd{}
				ed.change(func(f *Font) { glyphFlipV(f, 1) }) // an upside-down A, and its reverse
				a.doc = newMachineDoc(machineByID("plus4"))
				a.fg = 0x46
				emuFText(a, "CharEdit's Plus/4 .prg in xplus4:", "its own set (an upside-down A),", "pixel for pixel.")
			}
			dir := t.TempDir()
			p := filepath.Join(dir, "art."+map[string]string{"marqbas": "bas", "marqasm": "s"}[c.fmtID])
			if c.fmtID == "prg" || c.fmtID == "d64" {
				p = filepath.Join(dir, "art."+c.fmtID)
			}
			if !a.writeOut(p, c.fmtID) {
				t.Fatal(a.message)
			}
			cycles := 10000000
			switch c.fmtID {
			case "marqbas":
				petcat := os.Getenv("CHARCOAL_PETCAT")
				if petcat == "" {
					petcat, _ = exec.LookPath("petcat")
				}
				if petcat == "" {
					t.Skip("no petcat")
				}
				prg := filepath.Join(dir, "art.prg")
				if out, err := exec.Command(petcat, "-w3", "-o", prg, "--", p).CombinedOutput(); err != nil {
					t.Fatalf("petcat: %v %s", err, out)
				}
				p, cycles = prg, 250000000
			case "marqasm":
				acme := os.Getenv("CHARCOAL_ACME")
				if acme == "" {
					acme, _ = exec.LookPath("acme")
				}
				if acme == "" {
					t.Skip("no ACME")
				}
				prg := filepath.Join(dir, "art.prg")
				if out, err := exec.Command(acme, "-o", prg, "-f", "cbm", p).CombinedOutput(); err != nil {
					t.Fatalf("acme: %v %s", err, out)
				}
				p = prg
			case "d64":
				cycles = 60000000
			}
			home := t.TempDir()
			shot := filepath.Join(home, "shot.png")
			out := emuFRun(t, 300, home, emuFEnv(home), xplus4, "-default", "-console", "-warp", "+sound",
				"-autostartprgmode", "1", "-limitcycles", fmt.Sprint(cycles), "-exitscreenshot", shot, "-autostart", p)
			if _, err := os.Stat(shot); err != nil {
				t.Fatalf("xplus4 gave no screenshot:\n%s", out)
			}
			emuFCheck(t, s, emuFCaseFor(a, c.name, [2]int{1, 1}, [2]int{1, 1}), emuFReadPNG(t, shot))
		})
	}
}

// ---------------------------------------------------------------- DirArt

// chsFDirArtCodes are the screen codes a name can show.
func chsFDirArtCodes() []int {
	var out []int
	for sc := 0; sc < 256; sc++ {
		if screenToName(sc) >= 0 {
			out = append(out, sc)
		}
	}
	return out
}

// chsFDirArtPage is a DirArt page of random cells a name can show.
func chsFDirArtPage(seed int64, rows int) *Doc {
	r := rand.New(rand.NewSource(seed))
	ok := chsFDirArtCodes()
	d := newDoc(dirartCols, rows)
	d.glyph0, d.border = true, 1
	for i := range d.cells {
		d.cells[i] = makeCell(ok[r.Intn(len(ok))], 1, 0, 0)
	}
	return d
}

func chsFSameCodes(a, b *Doc) string {
	if a.rows != b.rows || a.cols != b.cols {
		return fmt.Sprintf("%dx%d, want %dx%d", b.cols, b.rows, a.cols, a.rows)
	}
	for y := 0; y < a.rows; y++ {
		for x := 0; x < a.cols; x++ {
			if cellCh(a.get(x, y)) != cellCh(b.get(x, y)) {
				return fmt.Sprintf("cell %d,%d: %d, want %d", x, y, cellCh(b.get(x, y)), cellCh(a.get(x, y)))
			}
		}
	}
	return ""
}

func TestChsDirArtNames(t *testing.T) {
	// The characters a name cannot show are Marq's and Petmate9's list.
	forbidden := []int{34, 128, 141, 148, 205}
	for c := 160; c < 192; c++ {
		forbidden = append(forbidden, c)
	}
	for c := 224; c < 256; c++ {
		forbidden = append(forbidden, c)
	}
	for sc := 0; sc < 256; sc++ {
		b := screenToName(sc)
		if (b < 0) != slices.Contains(forbidden, sc) {
			t.Errorf("screen code %d: byte %d", sc, b)
		}
		if b >= 0 && (petsciiToScreen(byte(b)) != sc || b == 0xA0 || b == 0 || b == 0x22 || b == 0x0D || b == 0x8D || b == 0x14) {
			t.Errorf("screen code %d is written as %02X, which LIST shows as %d", sc, b, petsciiToScreen(byte(b)))
		}
	}
}

func TestChsDirArtDisk(t *testing.T) {
	for _, rows := range []int{1, 8, 9, 40, 144} {
		d := chsFDirArtPage(int64(rows), rows)
		img, note := writeDirArt(d, nil, "/x/my art.d64")
		if len(img) != d64Size || note.String() != "" {
			t.Fatalf("%d rows: %d bytes, %s", rows, len(img), note)
		}
		back, err := readDirArt(img)
		if err != nil || chsFSameCodes(d, back) != "" {
			t.Fatalf("%d rows: %v %s", rows, err, chsFSameCodes(d, back))
		}
		// Every entry a closed DEL of 0 blocks; the BAM: 664 blocks free, the
		// directory's sectors taken; the disk's name.
		ents, _, sectors, _ := dirartEntries(img)
		for _, e := range ents {
			if e.kind != 0x80 || e.blocks != 0 {
				t.Fatalf("%d rows: entry %02X of %d blocks", rows, e.kind, e.blocks)
			}
		}
		if len(sectors) != (rows+7)/8 {
			t.Errorf("%d rows: %d directory sectors", rows, len(sectors))
		}
		bam := d64Offset(18, 0, 35)
		free := 0
		for tr := 1; tr <= 35; tr++ {
			if tr != 18 {
				free += int(img[bam+4*tr])
			}
		}
		used := 0
		for s := 0; s < 19; s++ {
			if img[bam+4*18+1+s/8]&(1<<(s%8)) == 0 {
				used++
			}
		}
		if free != 664 || used != 1+len(sectors) || int(img[bam+4*18]) != 19-used {
			t.Errorf("%d rows: %d blocks free, %d sectors of track 18 used (count %d)", rows, free, used, img[bam+4*18])
		}
		if d64Text(img[bam+0x90:bam+0xA0]) != "MY ART" {
			t.Errorf("disk name %q", d64Text(img[bam+0x90:bam+0xA0]))
		}
		if _, _, err := d64Art(img); err == nil {
			t.Error("a DirArt disk taken for a screen program")
		}
	}
	// Past 144 rows: left out, and said.
	d := chsFDirArtPage(3, 150)
	img, note := writeDirArt(d, nil, "x")
	if back, _ := readDirArt(img); back.rows != 144 || !strings.Contains(note.String(), "6 rows") {
		t.Errorf("150 rows: %d back, %s", back.rows, note)
	}
	// Cells a name cannot show are spaces, and counted.
	d = chsFDirArtPage(4, 2)
	d.set(0, 0, makeCell(34, 1, 0, 0))
	d.set(1, 1, makeCell(0xA0, 1, 0, 0))
	img, note = writeDirArt(d, nil, "x")
	back, _ := readDirArt(img)
	if cellCh(back.get(0, 0)) != 32 || cellCh(back.get(1, 1)) != 32 || !strings.Contains(note.String(), "2 cells") {
		t.Errorf("forbidden cells: %d %d %s", cellCh(back.get(0, 0)), cellCh(back.get(1, 1)), note)
	}
}

// TestChsDirArtOverDisk: a page from a disk with files on it is written back
// over that disk: the files, their data and the names that did not change
// ($A0 padding and all) stay.
func TestChsDirArtOverDisk(t *testing.T) {
	prg := append([]byte{0x01, 0x08}, bytes.Repeat([]byte("DEMO"), 2000)...)
	src := writeD64("demo", prg)
	// A second entry: a DEL of 0 blocks named with $A0 padding.
	dir := d64Offset(18, 1, 35)
	copy(src[dir+32+2:], []byte{0x80, 0, 0})
	copy(src[dir+32+5:dir+32+21], petsciiName("HELLO"))
	page, err := readDirArt(src)
	if err != nil || page.rows != 2 || cellCh(page.get(0, 1)) != 8 || cellCh(page.get(5, 1)) != 32 {
		t.Fatalf("read: %v %d rows", err, page.rows)
	}
	// Unchanged: the same bytes.
	if img, _ := writeDirArt(page, src, "x"); !bytes.Equal(img, src) {
		t.Error("an unchanged page changed the disk")
	}
	// A third row and a changed first: the program is still there, HELLO keeps its $A0s.
	more := newDoc(16, 3)
	copy(more.cells, page.cells)
	for x := 0; x < 16; x++ {
		more.set(x, 0, makeCell(0x51, 1, 0, 0))
		more.set(x, 2, makeCell(0x66, 1, 0, 0))
	}
	img, note := writeDirArt(more, src, "x")
	back, _ := readDirArt(img)
	if chsFSameCodes(more, back) != "" || note.String() != "" {
		t.Fatalf("written over the disk: %s %s", chsFSameCodes(more, back), note)
	}
	files, _ := d64Dir(img)
	if len(files) != 1 || !bytes.Equal(files[0].data, prg) {
		t.Fatal("the program on the disk was lost")
	}
	if !bytes.Equal(img[dir+32+5:dir+32+21], petsciiName("HELLO")) {
		t.Error("an unchanged name lost its padding")
	}
	// One row: the DEL entry goes, the program stays listed (and the note says so).
	one := newDoc(16, 1)
	copy(one.cells, more.cells[:16])
	img, note = writeDirArt(one, src, "x")
	if back, _ := readDirArt(img); back.rows != 1 || note.kept != 0 {
		t.Errorf("one row: %d rows, %s", back.rows, note)
	}
	// A full sector (8 entries) and one more row: a new directory sector, linked.
	full := chsFDirArtPage(5, 8)
	img8, _ := writeDirArt(full, nil, "x")
	nine := chsFDirArtPage(6, 9)
	img9, _ := writeDirArt(nine, img8, "x")
	if back, _ := readDirArt(img9); chsFSameCodes(nine, back) != "" {
		t.Errorf("a ninth row: %s", chsFSameCodes(nine, back))
	}
	if _, _, sectors, _ := dirartEntries(img9); len(sectors) != 2 || sectors[1] != [2]int{18, 4} {
		t.Errorf("directory sectors %v", sectors)
	}
}

func TestChsDirArtEditor(t *testing.T) {
	a := c64xFApp(t)
	a.setMachine("dirart")
	if !a.dirart() || a.doc.cols != 16 || a.pal[0] != 0xFF352879 || a.pal[1] != 0xFF6C5EB5 {
		t.Fatalf("a DirArt page: %s %dx%d", a.machine, a.doc.cols, a.doc.rows)
	}
	var ids []string
	for _, f := range a.modeFormats() {
		ids = append(ids, f.id)
	}
	if !slices.Contains(ids, "dirart") || slices.Contains(ids, "d64") || slices.Contains(ids, "prg") {
		t.Errorf("formats on a DirArt page: %v", ids)
	}
	a.doc = chsFDirArtPage(7, 12)
	d := a.doc.clone()
	p := filepath.Join(t.TempDir(), "art.d64")
	if !a.writeOut(p, "dirart") || a.srcKind != "dirart" {
		t.Fatal(a.message)
	}
	b := chsFOpen(t, p)
	// A page with no header row: every row a file; it opens with the disk's header line above them (dirarthead.go).
	body := newDoc(16, max(1, b.doc.rows-1))
	copy(body.cells, b.doc.cells[16:])
	if !b.dirart() || b.saveFmt != "dirart" || b.doc.rows != 13 || chsFSameCodes(d, body) != "" {
		t.Fatalf("reopened: %s %s %d rows %s", b.machine, b.saveFmt, b.doc.rows, chsFSameCodes(d, body))
	}
	// Ctrl+S after a change: written over the same disk.
	b.doc.set(0, 0, makeCell(0x41, 1, 0, 0))
	b.dirty = true
	if !b.writeOut(p, b.saveFmt) {
		t.Fatal(b.message)
	}
	if c := chsFOpen(t, p); cellCh(c.doc.get(0, 0)) != 0xC1 || c.doc.rows != 13 {
		t.Error("Ctrl+S")
	}
	// Other pages do not offer it; a disk with a screen program still opens as the screen.
	c := c64xFApp(t)
	for _, f := range c.modeFormats() {
		if f.id == "dirart" {
			t.Error("DirArt offered on a C64 page")
		}
	}
	p2 := filepath.Join(t.TempDir(), "screen.d64")
	if !c.writeOut(p2, "d64") || chsFOpen(t, p2).machine == "dirart" {
		t.Error("a screen's disk opened as DirArt")
	}
	// Open as DirArt shows that disk's directory.
	raw, _ := os.ReadFile(p2)
	e := c64xFApp(t)
	e.loadBytes(raw, "screen.d64", p2, "dirart")
	if !e.dirart() || e.doc.rows != 2 || cellCh(e.doc.get(0, 1)) != 19 { // the header, then "SCREEN"
		t.Errorf("Open as DirArt: %s %d rows %d", e.machine, e.doc.rows, cellCh(e.doc.get(0, 1)))
	}
	// Marq's and Petmate9's DirArt.
	marq := []byte("unsigned char frame0000[]={\n" + strings.Repeat("1,2,3,4,5,6,7,8,9,10,11,12,13,14,15,16,\n", 2) + "};\n// META: 16 2 DIRART upper\n")
	m := c64xFApp(t)
	m.loadBytes(marq, "x.c", "", "auto")
	if !m.dirart() || m.doc.cols != 16 || cellCh(m.doc.get(15, 1)) != 16 {
		t.Errorf("Marq's DirArt: %s %d", m.machine, m.doc.cols)
	}
	if out, _ := m.encodeFor("marq", "", nil); !strings.Contains(string(out), "META: 16 2 DIRART upper") {
		t.Errorf("Marq's DirArt written: %s", out)
	}
}

// TestEmuChsDirArt: LOAD"$",8 and LIST of CharEdit's DirArt disk in VICE
// (CHARCOAL_EMU=1) show the page: the listing's lines are found on VICE's
// screen as CharEdit draws them.
func TestEmuChsDirArt(t *testing.T) {
	s := emuFStart(t)
	x64 := emuFNeed(t, "x64sc")
	a := c64xFApp(t)
	page := chsFDirArtPage(8, 8)
	for x, r := range "CHAREDIT DIRART!" { // a row to read in the picture
		page.set(x, 0, makeCell(int(petsciiToScreen(byte(r))), 1, 0, 0))
	}
	img, _ := writeDirArt(page, nil, "dirart test")
	dir := t.TempDir()
	disk := filepath.Join(dir, "art.d64")
	os.WriteFile(disk, img, 0o644)
	// The listing as LIST shows it (light blue on blue): the header line
	// reversed from its quote, a line per entry, the blocks free.
	want := newDoc(40, page.rows+2)
	want.bg, want.border, want.glyph0 = 6, 14, true
	put := func(x, y int, text string, rev bool) int {
		for _, r := range []byte(text) {
			sc := petsciiToScreen(r)
			if r == 0xA0 {
				sc = 32
			}
			if rev {
				sc |= 0x80
			}
			want.set(x, y, makeCell(sc, 14, 0, 0))
			x++
		}
		return x
	}
	for i := range want.cells {
		want.cells[i] = makeCell(32, 14, 0, 0)
	}
	bam := d64Offset(18, 0, 35)
	put(put(0, 0, "0 ", false), 0, "\""+string(img[bam+0x90:bam+0xA0])+"\" CE 2A", true)
	for y := 0; y < page.rows; y++ {
		x := put(0, y+1, "0    \"", false)
		for i := 0; i < 16; i++ {
			want.set(x+i, y+1, makeCell(cellCh(page.get(i, y)), 14, 0, 0))
		}
		put(x+16, y+1, "\" DEL", false)
	}
	put(0, page.rows+1, "664 BLOCKS FREE.", false)
	a.doc = want
	fb := a.renderDoc(want, 1)
	bx, top, _ := a.borderSize(40*8, max(want.rows, 25)*8, 1)
	c := emuFCase{name: "c64-dirart-list", want: fb, x0: bx, y0: top, w: 40 * 8, h: (page.rows + 2) * 8,
		sxN: 1, sxD: 1, syN: 1, syD: 1, cellW: 8, cellH: 8}
	home := t.TempDir()
	shot := filepath.Join(home, "shot.png")
	out := emuFRun(t, 120, home, emuFEnv(home), x64, "-default", "-console", "-warp", "+sound", "-8", disk,
		"-keybuf", `load"$",8\nlist\n`, "-limitcycles", "40000000", "-exitscreenshot", shot)
	if _, err := os.Stat(shot); err != nil {
		t.Fatalf("x64sc gave no screenshot:\n%s", out)
	}
	emuFCheck(t, s, c, emuFReadPNG(t, shot))
}

// TestChsSamples writes sample files to look at (CHARCOAL_CHS_SAMPLES=dir):
// a DirArt disk and a Plus/4 program with a set of its own.
func TestChsSamples(t *testing.T) {
	dir := os.Getenv("CHARCOAL_CHS_SAMPLES")
	if dir == "" {
		t.Skip("CHARCOAL_CHS_SAMPLES not set")
	}
	art := []string{
		"     DIRART     ",
		"  \x12CHARCOAL\x92 DISK",
	}
	d := newDoc(16, 12)
	d.glyph0, d.border = true, 1
	for i := range d.cells {
		d.cells[i] = makeCell(32, 1, 0, 0)
	}
	for y, l := range art {
		x, rev := 0, 0
		for _, r := range []byte(l) {
			switch r {
			case 0x12:
				rev = 0x80
				continue
			case 0x92:
				rev = 0
				continue
			}
			d.set(x, y, makeCell(petsciiToScreen(r)|rev, 1, 0, 0))
			x++
		}
	}
	for y := 3; y < 12; y++ { // a ramp of blocks a name can show
		for x := 0; x < 16; x++ {
			sc := []int{0x20, 0x66, 0x5F, 0x69, 0x51, 0x57, 0x6C, 0x7B, 0x7E, 0x7C, 0x62, 0x79, 0x6F, 0x64, 0x63, 0x77}[(x+y)%16]
			d.set(x, y, makeCell(sc, 1, 0, 0))
		}
	}
	img, _ := writeDirArt(d, nil, "charcoal")
	os.WriteFile(filepath.Join(dir, "dirart.d64"), img, 0o644)
	if a, _ := chsFTEDApp(t, 3, false); a != nil {
		os.WriteFile(filepath.Join(dir, "plus4-own-set.prg"), tedxPrg(a.doc, a.ownInUse()), 0o644)
	}
}
