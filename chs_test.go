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
