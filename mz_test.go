package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
)

func TestMZFonts(t *testing.T) {
	if mzCode(0) != 0x20 || mzCode(0x20) != 0 || mzCode(0x41) != 0x41 {
		t.Error("mzCode")
	}
	// Russell Marks's set: display code 1 is A, 0 the space, 0x20 the digit 0.
	if mzFont.row(32, 3) != 0 || mzFont.row(mzCode(1), 3) == 0 || mzFont.row(0, 3) == 0 {
		t.Error("the space, A or 0 is wrong")
	}
	for set := 0; set < 2; set++ {
		if len(mzRunes[set]) != 256 {
			t.Errorf("set %d has %d runes", set+1, len(mzRunes[set]))
		}
	}
	for _, c := range []struct {
		r    rune
		code int
	}{{'A', 1}, {'Z', 26}, {'0', 0x20}, {'a', 0x81}, {' ', 0}, {'█', 0x43}, {'£', 0x1B}} {
		if got := mzFromRune(c.r); got != mzCode(c.code) {
			t.Errorf("%q is %d, want display code %d", c.r, got, c.code)
		}
		if mzRune(mzCode(c.code)) != c.r {
			t.Errorf("display code %d is %q", c.code, mzRune(mzCode(c.code)))
		}
	}
	// The quarter blocks of the F-keys: bit 0 top left, 1 top right, 2 bottom left, 3 bottom right.
	for b := 1; b < 16; b++ {
		q := 0
		if mzFont.row(0xF0|b, 2)&0x40 != 0 {
			q |= 1
		}
		if mzFont.row(0xF0|b, 2)&0x04 != 0 {
			q |= 2
		}
		if mzFont.row(0xF0|b, 6)&0x40 != 0 {
			q |= 4
		}
		if mzFont.row(0xF0|b, 6)&0x04 != 0 {
			q |= 8
		}
		if q != b {
			t.Errorf("quarter block %02X has quarters %d", 0xF0|b, q)
		}
	}
	if !validMZFkeys(mzFkeys) || validMZFkeys([][]int{{1, 2, 3, 4, 5, 6, 7, 8, 9, 512}}) {
		t.Error("validMZFkeys")
	}
	// The second set without the ROM: each character its code.
	if mzHexFont.row(mzCode(0xA5), 0) != 0xFF {
		t.Error("the second set's boxes")
	}
}

func TestMZPage(t *testing.T) {
	a := testApp(t, "mz")
	if a.doc.cols != 40 || a.doc.rows != 25 || a.cellW() != 8 || a.cellH() != 8 || a.saveFmt != "mzf" || !a.doc.glyph0 {
		t.Fatalf("new MZ-700 page: %dx%d %s cells %dx%d glyph0 %v", a.doc.cols, a.doc.rows, a.saveFmt, a.cellW(), a.cellH(), a.doc.glyph0)
	}
	if a.pal != mzPalette.colors {
		t.Error("the MZ-700's colours")
	}
	if cellFg(a.doc.cells[0]) != 7 || cellBg(a.doc.cells[0]) != 1 {
		t.Error("an empty MZ-700 page is white on blue")
	}
	a.setCols(80)
	a.setRows(10)
	if a.doc.cols != 40 || a.doc.rows != 25 {
		t.Error("the screen changed size")
	}
	a.fg, a.bg = 6, 2
	a.cx, a.cy = 0, 0
	for _, r := range "A0a" {
		a.typeByte(a.charToByte(r))
	}
	if cellCh(a.doc.get(1, 0)) != 0 || cellCh(a.doc.get(2, 0)) != 0x81 || cellFg(a.doc.get(0, 0)) != 6 {
		t.Error("typing")
	}
	if a.doc.lastCol(0) < 1 || a.doc.blankAt(a.doc.get(1, 0)) {
		t.Error("the digit 0 counts as empty")
	}
	// The second set from the brush.
	a.brush = 256 + 0x45
	a.setCell(5, 5, a.brushCell())
	if v := a.doc.get(5, 5); v&mzSet2 == 0 || mzChar(v) != 256+0x45 {
		t.Errorf("a second-set cell: %08X", v)
	}
	a.pick(5, 5)
	if a.brush != 256+0x45 || a.fg != 6 || a.bg != 2 {
		t.Errorf("pick: brush %d %d/%d", a.brush, a.fg, a.bg)
	}
	if mzAttr(a.doc.get(5, 5)) != 0xE2 {
		t.Errorf("attribute %02X", mzAttr(a.doc.get(5, 5)))
	}
	// Drawn: a second-set cell takes the second set's glyphs.
	fb := &FB{}
	fb.resize(8, 8)
	a.drawCell(fb, 0, 0, 1, a.doc.get(5, 5))
	if fb.px[0] != a.pal[6] { // the box's corner
		t.Errorf("second set drawn %08X", fb.px[0])
	}
}

// mzFRun loads an .mzf and runs it in the Z80; it gives the video RAM.
func mzFRun(t *testing.T, file []byte) (codes, attrs []byte) {
	t.Helper()
	tp, ok := readMZF(file)
	if !ok {
		t.Fatal("not an .mzf")
	}
	mem := make([]byte, 0x10000)
	copy(mem[tp.load:], tp.data)
	msxFZ80(t, mem, tp.start, 0x10F0, func(at int, r *msxFRegs) bool {
		t.Fatalf("the program calls the monitor at %04X", at)
		return false
	})
	return mem[mzVRAM : mzVRAM+1000], mem[mzAttrRAM : mzAttrRAM+1000]
}

func mzFRandom(a *App, r *rand.Rand) {
	for i := range a.doc.cells {
		a.doc.cells[i] = mzCell(r.Intn(512), r.Intn(8), r.Intn(8))
	}
}

func TestMZFile(t *testing.T) {
	r := rand.New(rand.NewSource(700))
	a := testApp(t, "mz")
	mzFRandom(a, r)
	p := filepath.Join(t.TempDir(), "my art.mzf")
	if !a.writeOut(p, "mzf") {
		t.Fatal(a.message)
	}
	b, _ := os.ReadFile(p)
	tp, ok := readMZF(b)
	if !ok || tp.name != "MY ART" || tp.load != 0x1200 || tp.start != 0x1200 || b[0] != 1 || b[17] != 0x0D {
		t.Fatalf("the header: %+v", tp)
	}
	codes, attrs := mzFRun(t, b)
	for i, v := range a.doc.cells {
		if int(codes[i]) != mzCode(cellCh(v)) || attrs[i] != mzAttr(v) {
			t.Fatalf("cell %d: video RAM %02X %02X, page %02X %02X", i, codes[i], attrs[i], mzCode(cellCh(v)), mzAttr(v))
		}
	}
	a2 := testApp(t, "amiga")
	a2.loadBytes(b, "my art.mzf", p, "auto")
	if !a2.mz() || a2.saveFmt != "mzf" || !slicesEqualCells(a2.doc.cells, a.doc.cells) {
		t.Fatalf("read back as %s: %s", a2.mode, a2.message)
	}
	// Saving the unchanged page gives the same file.
	if out, _ := a2.encodeFor("mzf", "", nil); !bytes.Equal(out[18:], b[18:]) {
		t.Error("saved again differently")
	}
}

// TestMZOtherPrograms: an .mzf that copies its two blocks to $D000 and $D800
// with a C library's memcpy (as pixelrat's Princess, Demodulation 2021, does),
// or with LDIR in another order.
func TestMZOtherPrograms(t *testing.T) {
	codes, attrs := make([]byte, 1000), make([]byte, 1000)
	for i := range codes {
		codes[i], attrs[i] = byte(i), byte(i*7)&^8 // bit 3 is not used
	}
	head := func(n int) []byte {
		h := make([]byte, 128)
		h[0] = 1
		copy(h[1:], "IMAGE\r")
		binary.LittleEndian.PutUint16(h[18:], uint16(n))
		binary.LittleEndian.PutUint16(h[20:], 0x1200)
		binary.LittleEndian.PutUint16(h[22:], 0x1200)
		return h
	}
	// push 1000, push src, push $D000, call memcpy (z88dk's way, as in Princess.mzf)
	prog := []byte{0x21, 0xE8, 0x03, 0xE5, 0x21, 0x30, 0x12, 0xE5, 0x21, 0x00, 0xD0, 0xE5, 0xCD, 0x29, 0x1B,
		0x21, 0xE8, 0x03, 0xE5, 0x21, 0x18, 0x16, 0xE5, 0x21, 0x00, 0xD8, 0xE5, 0xCD, 0x29, 0x1B, 0xC9}
	data := append(append(append(append([]byte{}, prog...), make([]byte, 0x30-len(prog))...), codes...), attrs...)
	file := append(head(len(data)), data...)
	tp, ok := readMZF(file)
	if !ok {
		t.Fatal("header")
	}
	d, err := readMZScreen(tp)
	if err != nil {
		t.Fatal(err)
	}
	for i := range codes {
		if v := d.cells[i]; mzCode(cellCh(v)) != int(codes[i]) || mzAttr(v) != attrs[i] {
			t.Fatalf("cell %d", i)
		}
	}
	// LD DE,$D800 / LD HL,src / LD BC,1000 / LDIR, the attributes first.
	prog = []byte{0x11, 0x00, 0xD8, 0x21, 0x18, 0x16, 0x01, 0xE8, 0x03, 0xED, 0xB0, 0x11, 0x00, 0xD0, 0x21, 0x30, 0x12, 0x01, 0xE8, 0x03, 0xED, 0xB0, 0xC9}
	data = append(append(append(append([]byte{}, prog...), make([]byte, 0x30-len(prog))...), codes...), attrs...)
	tp, _ = readMZF(append(head(len(data)), data...))
	if d, err = readMZScreen(tp); err != nil || mzAttr(d.cells[999]) != attrs[999] {
		t.Fatalf("LDIR: %v", err)
	}
	// A real one, when there: CHARCOAL_MZ_FILES=dir of .mzf pictures.
	dir := os.Getenv("CHARCOAL_MZ_FILES")
	if dir == "" {
		return
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*.mzf"))
	for _, f := range files {
		b, _ := os.ReadFile(f)
		a := testApp(t, "amiga")
		a.loadBytes(b, filepath.Base(f), f, "auto")
		if !a.mz() {
			t.Errorf("%s did not open: %s", filepath.Base(f), a.message)
			continue
		}
		out := filepath.Join(os.TempDir(), "charcoal-msxmz", filepath.Base(f)+".png")
		os.MkdirAll(filepath.Dir(out), 0o755)
		a.writeOut(out, "png")
		t.Logf("%s: %d cells drawn, picture %s", filepath.Base(f), a.doc.inkCells(), out)
	}
}

func TestMZTextAndUTF8(t *testing.T) {
	a := testApp(t, "amiga")
	a.loadBytes([]byte("HELLO £\nworld"), "x.txt", "", "mz")
	if !a.mz() || a.saveFmt != "utf8" {
		t.Fatalf("opened as %s", a.mode)
	}
	if mzCode(cellCh(a.doc.get(0, 0))) != 8 || mzCode(cellCh(a.doc.get(6, 0))) != 0x1B || mzCode(cellCh(a.doc.get(0, 1))) != 0x97 {
		t.Error("the text")
	}
	a.doc.set(1, 1, mzCell(256+0x59, 7, 1)) // the second set's plane
	out, err := a.encodeFor("utf8", "", nil)
	if err != nil || string(out[:len("HELLO £\nw🛧rld")]) != "HELLO £\nw🛧rld" {
		t.Errorf("UTF-8: %q", out)
	}
}

func TestMZROMFont(t *testing.T) {
	rom := make([]byte, 4096)
	for c := 1; c < 512; c++ {
		for r := 0; r < 8; r++ {
			rom[c*8+r] = byte(c + r) // the low bit the leftmost pixel
		}
	}
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)
	os.MkdirAll(appDataDir(), 0o755)
	os.WriteFile(filepath.Join(appDataDir(), "mz700fon.int"), rom, 0o644)
	keep := fonts
	defer func() { fonts = keep; delete(mzROMSet2, "mz700rom") }()
	if p := loadMZROMFonts(); p == "" {
		t.Fatal("the ROM was not found")
	}
	f := fontByID("mz700rom")
	if f == nil || mzROMSet2["mz700rom"] == nil {
		t.Fatal("no ROM fonts")
	}
	if f.row(mzCode(1), 0) != 0x80 { // byte 1: the leftmost pixel only
		t.Errorf("set 1, A: %02X", f.row(mzCode(1), 0))
	}
	if mzROMSet2["mz700rom"].row(mzCode(1), 0) != 0x80 { // byte 257 = 1
		t.Errorf("set 2: %02X", mzROMSet2["mz700rom"].row(mzCode(1), 0))
	}
	a := testApp(t, "mz")
	a.fontFor["mz"] = "mz700rom"
	if a.font() != f || a.mzSet2Font() != mzROMSet2["mz700rom"] {
		t.Error("the page does not draw with the ROM")
	}
}

func TestMZAutosaveAndPicture(t *testing.T) {
	a := testApp(t, "mz")
	fb := a.renderDoc(a.doc, 1)
	if fb.w != 320 || fb.h != 200 || fb.px[0] != a.pal[1] {
		t.Errorf("picture %dx%d %08X", fb.w, fb.h, fb.px[0])
	}
}

// ---- the emulator check (CHARCOAL_EMU=1) ----

// mzFPad puts 4 pixels round MAME's picture (it is the screen and nothing
// else; emuFMatch looks for a screen with room round it).
func mzFPad(g emuFImg) emuFImg {
	p := emuFImg{w: g.w + 8, h: g.h + 8, rgb: g.rgb}
	p.px = make([]uint32, p.w*p.h)
	for i := range p.px {
		p.px[i] = 0xFEDCBA
	}
	for y := 0; y < g.h; y++ {
		copy(p.px[(y+4)*p.w+4:], g.px[y*g.w:(y+1)*g.w])
	}
	return p
}

// mzFMonitor is a 4 KB stand-in for the MZ-700's monitor ROM (Sharp's may not
// be passed on), for MAME's mz700: at reset it copies the .mzf's bytes from
// itself to their load address and jumps to the start, as the monitor's L
// command would after reading the tape.
func mzFMonitor(t *testing.T, tp mzTape) []byte {
	rom := make([]byte, 4096)
	const at = 0x0100
	if at+len(tp.data) > len(rom) {
		t.Fatalf("the program (%d bytes) does not fit the monitor", len(tp.data))
	}
	copy(rom, []byte{0xF3, 0x31, 0xF0, 0x10, // DI / LD SP,$10F0
		0x21, at & 255, at >> 8, 0x11, byte(tp.load), byte(tp.load >> 8), 0x01, byte(len(tp.data)), byte(len(tp.data) >> 8), 0xED, 0xB0,
		0xC3, byte(tp.start), byte(tp.start >> 8)})
	copy(rom[at:], tp.data)
	return rom
}

// mzFCGROM is a character ROM of the page's font as MAME reads it: both sets,
// the low bit the leftmost pixel.
func mzFCGROM(a *App) []byte {
	rom := make([]byte, 4096)
	for set, f := range []*Font{a.font(), a.mzSet2Font()} {
		for c := 0; c < 256; c++ {
			for r := 0; r < 8; r++ {
				v := f.row(mzCode(c), r)
				var o byte
				for i := 0; i < 8; i++ {
					if v&(0x80>>i) != 0 {
						o |= 1 << i
					}
				}
				rom[set*2048+c*8+r] = o
			}
		}
	}
	return rom
}

// TestEmuMZ runs CharEdit's .mzf in MAME's mz700 (with a stand-in monitor that
// loads it, and a character ROM of CharEdit's font) and compares the screen
// with the PNG, colours by RGB too (MAME's are the 3-bit ones).
func TestEmuMZ(t *testing.T) {
	s := emuFStart(t)
	mame := "/usr/lib/mame/mame"
	if _, err := os.Stat(mame); err != nil {
		mame = emuFNeed(t, "mame")
	}
	r := rand.New(rand.NewSource(1982))
	for _, name := range []string{"mz-text", "mz-random"} {
		t.Run(name, func(t *testing.T) {
			a := testApp(t, "mz")
			if name == "mz-text" {
				emuFText(a, "CharEdit's .mzf in MAME", "(mz700): the same page", "as CharEdit's PNG.")
				for i := 0; i < 16; i++ { // the colours and a few of the second set
					a.doc.cells[20*40+i*2] = mzCell(0xF0+i, i%8, (i+3)%8)
					a.doc.cells[22*40+i*2] = mzCell(256+0x80+i, 7, i%8)
				}
			} else {
				mzFRandom(a, r)
			}
			p := filepath.Join(t.TempDir(), "art.mzf")
			if !a.writeOut(p, "mzf") {
				t.Fatal(a.message)
			}
			b, _ := os.ReadFile(p)
			tp, _ := readMZF(b)
			home := t.TempDir()
			roms := filepath.Join(home, "roms", "mz700")
			os.MkdirAll(roms, 0o755)
			os.WriteFile(filepath.Join(roms, "1z-013a.rom"), mzFMonitor(t, tp), 0o644)
			os.WriteFile(filepath.Join(roms, "mz700fon.int"), mzFCGROM(a), 0o644)
			snap := filepath.Join(home, "snap")
			out := emuFRun(t, 120, home, emuFEnv(home), mame, "mz700", "-rompath", filepath.Join(home, "roms"),
				"-video", "none", "-sound", "none", "-nothrottle", "-skip_gameinfo", "-seconds_to_run", "3",
				"-snapshot_directory", snap, "-cfg_directory", filepath.Join(home, "cfg"), "-nvram_directory", filepath.Join(home, "nvram"))
			shots, _ := filepath.Glob(filepath.Join(snap, "*", "*.png"))
			if len(shots) == 0 {
				t.Fatalf("MAME took no snapshot:\n%s", out)
			}
			g := mzFPad(emuFReadPNG(t, shots[len(shots)-1]))
			ec := emuFCaseFor(a, name, [2]int{1, 1}, [2]int{1, 1})
			ec.ring = false // the MZ-700's picture has no border of its own
			pairs := emuFCheck(t, s, ec, g)
			for _, pr := range pairs {
				if pr.ours&0xFFFFFF != g.rgb(pr.theirs)&0xFFFFFF {
					t.Errorf("colour %06X is %06X in MAME", pr.ours&0xFFFFFF, g.rgb(pr.theirs)&0xFFFFFF)
				}
			}
			t.Logf("%d colours", len(pairs))
		})
	}
	_ = fmt.Sprint
}
