package main

import (
	"bytes"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------- GTIA colours

func TestMachGTIAPalettes(t *testing.T) {
	luma := func(c uint32) float64 {
		return 0.299*float64(c>>16&255) + 0.587*float64(c>>8&255) + 0.114*float64(c&255)
	}
	for _, tc := range []struct {
		name string
		t    *[256]uint32
	}{{"PAL", &gtiaPAL}, {"NTSC", &gtiaNTSC}} {
		for lum := 0; lum < 16; lum++ { // hue 0 is grey, brighter with every step
			c := tc.t[lum]
			if c>>16&255 != c>>8&255 || c>>8&255 != c&255 {
				t.Errorf("%s: $%02X is not grey: %06X", tc.name, lum, c&0xFFFFFF)
			}
			if lum > 0 && luma(c) <= luma(tc.t[lum-1]) {
				t.Errorf("%s: $%02X not brighter than the one before", tc.name, lum)
			}
		}
		for hue := 1; hue < 16; hue++ { // every hue has colour, and its luminance rises
			c := tc.t[hue<<4|8]
			if c>>16&255 == c>>8&255 && c>>8&255 == c&255 {
				t.Errorf("%s: hue %d is grey", tc.name, hue)
			}
			if luma(tc.t[hue<<4|14]) <= luma(tc.t[hue<<4|2]) {
				t.Errorf("%s: hue %d does not get brighter", tc.name, hue)
			}
		}
		// The start-up screen is blue on both: 148 ($94) and its text $9A.
		for _, v := range []int{0x94, 0x9A} {
			c := tc.t[v]
			if !(c&255 > c>>16&255+40 && c&255 > c>>8&255) {
				t.Errorf("%s: $%02X is not blue: %06X", tc.name, v, c&0xFFFFFF)
			}
		}
		// Hue 1 is the colour burst's: gold / orange (red over blue).
		if c := tc.t[0x18]; c>>16&255 <= c&255 {
			t.Errorf("%s: hue 1 is not gold: %06X", tc.name, c&0xFFFFFF)
		}
	}
	if gtiaNearest(gtiaPAL[0x94], &gtiaPAL) != 0x94 || gtiaNearest(gtiaNTSC[0x36], &gtiaNTSC) != 0x36 {
		t.Error("gtiaNearest")
	}
}

func TestMachAtariColoursInEditor(t *testing.T) {
	a := testApp(t, "atari")
	if a.doc.bg != 0x94 || a.doc.ink != 0xCA || a.doc.border != 0 {
		t.Fatalf("start-up registers %d %d %d", a.doc.ink, a.doc.bg, a.doc.border)
	}
	if a.atariRGB(atariText(a.doc)) != gtiaPAL[0x9A] {
		t.Fatal("the text takes the screen's hue")
	}
	a.atariColourKey(3, true) // hue 3, the luminance kept
	if a.doc.bg != 0x34 {
		t.Fatalf("hue key gave %d", a.doc.bg)
	}
	a.stepBg(1)
	if a.doc.bg != 0x36 {
		t.Fatalf("step gave %d", a.doc.bg)
	}
	a.stepFg(-1)
	if a.doc.ink != 0xC8 {
		t.Fatalf("ink step gave %d", a.doc.ink)
	}
	a.setAtariBorder(0x46)
	a.doUndo()
	if a.doc.border != 0 {
		t.Fatal("undo of the border")
	}
	a.defaultColours()
	if a.doc.bg != atariBg || a.doc.ink != atariInk {
		t.Fatal("start-up colours")
	}
	a.paletteFor["atari"] = "atarintsc"
	a.setPalette()
	if a.atariRGB(0x94) != gtiaNTSC[0x94] || a.pal != atariNTSCPalette.colors {
		t.Fatal("NTSC palette")
	}
	// The PNG shows the registers: border, screen and text colour.
	a.doc.set(0, 0, makeCell(0xA0, 0, 0, 0)) // an inverse space: the text colour
	fb := a.renderDoc(a.doc, 1)
	if fb.px[0] != gtiaNTSC[0] || fb.px[32*fb.w+40] != gtiaNTSC[0x94] || fb.px[33*fb.w+33] != gtiaNTSC[0x9A] {
		t.Fatalf("PNG colours %06X %06X %06X", fb.px[0], fb.px[32*fb.w+40], fb.px[33*fb.w+33])
	}
}

// Old autosaves held colour numbers 0-15: they come back in register values.
func TestMachAtariOldAutosave(t *testing.T) {
	d := newDoc(40, 24)
	d.bg, d.ink, d.border = 2, 3, 0
	atariFromOld(d)
	if d.bg != atariBg || d.ink != atariInk || d.border != 0 {
		t.Fatalf("old start-up colours became %d %d %d", d.bg, d.ink, d.border)
	}
	a := testApp(t, "atari")
	a.doc.bg, a.doc.ink, a.dirty = 0x24, 0x0E, true
	if err := writeAutosave(a.Tab); err != nil {
		t.Fatal(err)
	}
	b := restart(t, "pc")
	files := leftAutosaves()
	if len(files) != 1 || b.restoreAutosave(files[0]) != nil {
		t.Fatal("autosave")
	}
	if !b.atari() || b.doc.bg != 0x24 || b.doc.ink != 0x0E {
		t.Fatalf("autosave gave %s %d %d", b.mode, b.doc.bg, b.doc.ink)
	}
}

// ---------------------------------------------------------------- .xex and .atr

// machF6502 runs the program from pc until it comes back to the address it
// started a loop at twice (the program's endless loop); the instructions the
// Atari program uses only.
func machF6502(t *testing.T, mem []byte, pc int) {
	t.Helper()
	var A byte
	seen := map[int]int{}
	for steps := 0; steps < 100000; steps++ {
		op := mem[pc]
		abs := int(mem[pc+1]) | int(mem[pc+2])<<8
		switch op {
		case 0xA9:
			A, pc = mem[pc+1], pc+2
		case 0x8D:
			mem[abs], pc = A, pc+3
		case 0x85:
			mem[mem[pc+1]], pc = A, pc+2
		case 0x4C:
			if seen[abs]++; seen[abs] == 2 {
				return
			}
			pc = abs
		default:
			t.Fatalf("6502: opcode %02X at %04X", op, pc)
		}
	}
	t.Fatal("6502: the program did not loop")
}

// machFLoadXex loads a binary load file as Atari DOS does; it gives memory and
// the run address.
func machFLoadXex(t *testing.T, b []byte) ([]byte, int) {
	t.Helper()
	if b[0] != 0xFF || b[1] != 0xFF {
		t.Fatal("no $FFFF")
	}
	mem := make([]byte, 65536)
	for i := 2; i < len(b); {
		from, to := int(b[i])|int(b[i+1])<<8, int(b[i+2])|int(b[i+3])<<8
		i += 4
		if i+to-from+1 > len(b) {
			t.Fatalf("segment %04X-%04X cut off", from, to)
		}
		copy(mem[from:], b[i:i+to-from+1])
		i += to - from + 1
	}
	return mem, int(mem[0x2E0]) | int(mem[0x2E1])<<8
}

// machFBootATR boots a disk as the OS does: sector 1's header gives the number
// of boot sectors and where they go; the OS calls the address 6 bytes on.
func machFBootATR(t *testing.T, b []byte) ([]byte, int) {
	t.Helper()
	if b[0] != 0x96 || b[1] != 0x02 || int(b[4])|int(b[5])<<8 != 128 {
		t.Fatalf("ATR header % x", b[:16])
	}
	if paras := int(b[2]) | int(b[3])<<8 | int(b[6])<<16; paras*16 != len(b)-16 || len(b)-16 != 720*128 {
		t.Fatalf("ATR size: %d paragraphs, %d bytes", paras, len(b)-16)
	}
	disk := b[16:]
	n, load := int(disk[1]), int(disk[2])|int(disk[3])<<8
	mem := make([]byte, 65536)
	copy(mem[load:], disk[:n*128])
	if init := int(disk[4]) | int(disk[5])<<8; mem[init] != 0x60 {
		t.Fatalf("DOSINI %04X is no RTS", init)
	}
	return mem, load + 6
}

// machFAntic draws the screen the OS shows from the shadow registers: the display
// list, CHBAS (the ROM's font at $E000), CHACT and the colour registers; the
// 320 x 192 picture with the border colour's index in border.
func machFAntic(t *testing.T, mem []byte, rom []byte) (px []uint32, border uint32) {
	t.Helper()
	copy(mem[0xE000:], rom)
	dl := int(mem[0x230]) | int(mem[0x231])<<8
	if mem[0x22F] != 0x22 {
		t.Fatalf("SDMCTL %02X", mem[0x22F])
	}
	var lines []int // the address of each text line
	addr, y := 0, 0
	for i := dl; len(lines) < 30; {
		ins := mem[i]
		switch {
		case ins&15 == 0:
			y += int(ins>>4&7) + 1
			i++
		case ins&15 == 1:
			if ins&0x40 == 0 || int(mem[i+1])|int(mem[i+2])<<8 != dl {
				t.Fatalf("display list: jump %02X", ins)
			}
			i = len(mem) // end
		case ins&15 == 2:
			if ins&0x40 != 0 {
				addr = int(mem[i+1]) | int(mem[i+2])<<8
				i += 3
			} else {
				i++
			}
			lines = append(lines, addr)
			addr += 40
		default:
			t.Fatalf("display list: mode %02X", ins)
		}
		if i >= len(mem) {
			break
		}
	}
	if y != 24 || len(lines) != 24 {
		t.Fatalf("display list: %d blank lines, %d text lines", y, len(lines))
	}
	if dl>>10 != (dl+31)>>10 || lines[0]>>12 != (lines[0]+959)>>12 {
		t.Fatal("the display list or the screen crosses a boundary ANTIC cannot")
	}
	font := int(mem[0x2F4]) << 8
	c1, c2, c4 := int(mem[0x2C5]), int(mem[0x2C6]), int(mem[0x2C8])
	on, off := gtiaPAL[c2&0xF0|c1&0x0E], gtiaPAL[c2&0xFE]
	px = make([]uint32, 320*192)
	for row, a := range lines {
		for x := 0; x < 40; x++ {
			ch := mem[a+x]
			for r := 0; r < 8; r++ {
				bits := mem[font+int(ch&0x7F)*8+r]
				if ch&0x80 != 0 && mem[0x2F3]&2 != 0 {
					bits = ^bits
				}
				for b := 0; b < 8; b++ {
					c := off
					if bits&(0x80>>b) != 0 {
						c = on
					}
					px[(row*8+r)*320+x*8+b] = c
				}
			}
		}
	}
	return px, gtiaPAL[c4&0xFE]
}

// machFPage fills an Atari page with random characters and colours.
func machFPage(r *rand.Rand, a *App, cols, rows int) {
	a.setCols(cols)
	a.doc.bg, a.doc.ink, a.doc.border = r.Intn(128)*2, r.Intn(256), r.Intn(128)*2
	for y := 0; y < a.doc.rows; y++ {
		for x := 0; x < cols; x++ {
			a.doc.set(x, y, makeCell(r.Intn(256), 0, 0, 0))
		}
	}
	if rows < a.doc.rows {
		a.doc.rows = rows
		a.doc.cells = a.doc.cells[:rows*cols]
	}
}

// Every combination runs on a model Atari and shows exactly the PNG export's
// picture: 38 and 40 columns, the ROM font and an own font, short pages; as a
// .xex and booted from the .atr.
func TestMachXexShowsThePage(t *testing.T) {
	r := rand.New(rand.NewSource(7))
	for _, c := range []struct {
		cols, rows int
		own        bool
	}{{40, 24, false}, {38, 24, false}, {40, 24, true}, {38, 10, true}, {40, 3, false}} {
		a := testApp(t, "atari")
		machFPage(r, a, c.cols, c.rows)
		if c.own {
			font := make([]byte, 1024)
			r.Read(font)
			copy(font, make([]byte, 8)) // a space that is empty: the margin shows it
			f := atariFont(ownID, "mine", font)
			f.h = 8
			a.useOwnFont(f, nil, true)
		}
		png := a.renderDoc(a.doc, 1)
		if png.w != c.cols*8+64 || png.h != 24*8+64 {
			t.Fatalf("PNG %dx%d", png.w, png.h)
		}
		for _, fmtID := range []string{"xex", "atr"} {
			out, err := a.encode(fmtID)
			if err != nil {
				t.Fatal(err)
			}
			var mem []byte
			var run int
			if fmtID == "xex" {
				mem, run = machFLoadXex(t, out)
			} else {
				mem, run = machFBootATR(t, out)
			}
			machF6502(t, mem, run)
			if mem[0x4D] != 0 {
				t.Fatal("ATRACT not kept 0")
			}
			px, border := machFAntic(t, mem, atariFontStd)
			if border != png.px[0] {
				t.Fatalf("%s %+v: border %06X, the PNG's %06X", fmtID, c, border, png.px[0])
			}
			left := (40 - c.cols) * 8
			for y := 0; y < 192; y++ {
				for x := 0; x < 320; x++ {
					want := png.px[(y+32)*png.w+32+x-left]
					if x < left {
						want = gtiaPAL[a.doc.bg&0xFE] // the screen editor's margin
					}
					if px[y*320+x] != want {
						t.Fatalf("%s %+v: pixel %d,%d %06X, the PNG's %06X", fmtID, c, x, y, px[y*320+x], want)
					}
				}
			}
		}
	}
}

func TestMachXexRoundTrip(t *testing.T) {
	r := rand.New(rand.NewSource(3))
	for i := 0; i < 50; i++ {
		d := newAtariDoc()
		d.cols = []int{38, 40}[i%2]
		d.rows = 1 + r.Intn(24)
		d.cells = make([]uint32, d.cols*d.rows)
		for k := range d.cells {
			d.cells[k] = makeCell(r.Intn(256), 0, 0, 0)
		}
		d.bg, d.ink, d.border = r.Intn(256), r.Intn(256), r.Intn(256)
		var font []byte
		if i%3 == 0 {
			font = make([]byte, 1024)
			r.Read(font)
		}
		for _, out := range [][]byte{writeXex(d, font), writeATR(d, font)} {
			got, f, err := readAtariProgram(out)
			if err != nil {
				t.Fatal(err)
			}
			if got.cols != d.cols || got.rows != d.rows || got.bg != d.bg || got.ink != d.ink || got.border != d.border || !bytes.Equal(f, font) {
				t.Fatalf("%dx%d %d/%d/%d read back as %dx%d %d/%d/%d", d.cols, d.rows, d.ink, d.bg, d.border, got.cols, got.rows, got.ink, got.bg, got.border)
			}
			for k, v := range d.cells {
				if cellCh(got.cells[k]) != cellCh(v) {
					t.Fatalf("cell %d: %02X read back as %02X", k, cellCh(v), cellCh(got.cells[k]))
				}
			}
		}
	}
	for k := 0; k < 256; k++ {
		if atariFromInternal(atariInternal(k)) != k {
			t.Fatalf("screen code of %02X", k)
		}
	}
	// Other programs are not ours.
	if _, _, err := readAtariProgram([]byte{0xFF, 0xFF, 0x00, 0x20, 0x01, 0x20, 0x4C, 0x00}); err == nil {
		t.Fatal("a foreign program opened")
	}
}

func TestMachXexInEditor(t *testing.T) {
	a := testApp(t, "atari")
	r := rand.New(rand.NewSource(11))
	machFPage(r, a, 38, 24)
	font := make([]byte, 1024)
	r.Read(font)
	f := atariFont(ownID, "mine", font)
	f.h = 8
	a.useOwnFont(f, nil, true)
	want := a.doc.clone()
	dir := t.TempDir()
	for _, name := range []string{"art.xex", "art.atr"} {
		p := filepath.Join(dir, name)
		if !a.writeOut(p, strings.TrimPrefix(filepath.Ext(name), ".")) {
			t.Fatal(a.message)
		}
		b := testApp(t, "pc")
		b.openPath(p, "auto")
		if !b.atari() || b.saveFmt != filepath.Ext(name)[1:] || b.doc.cols != 38 || b.doc.bg != want.bg || b.doc.ink != want.ink || b.doc.border != want.border {
			t.Fatalf("%s opened as %s (%s) %d columns: %s", name, b.mode, b.saveFmt, b.doc.cols, b.message)
		}
		if g := b.ownInUse(); g == nil || !bytes.Equal(g.bits, f.bits) {
			t.Fatalf("%s: the font did not come back", name)
		}
		for k, v := range want.cells {
			if cellCh(b.doc.cells[k]) != cellCh(v) {
				t.Fatalf("%s: cell %d", name, k)
			}
		}
		b.typeByte('Z')
		if !b.writeOut(p, b.saveFmt) {
			t.Fatal(b.message)
		}
		c := testApp(t, "pc")
		c.openPath(p, "auto")
		if cellCh(c.doc.get(0, 0)) != 'Z' {
			t.Fatalf("%s: Ctrl+S did not keep the typing", name)
		}
	}
	// A page in the ROM's font carries no font: CHBAS stays $E0.
	a = testApp(t, "atari")
	if out := writeXex(a.doc, a.atariProgramFont()); len(out) != 2+4+0x80+4+960+6 {
		t.Fatalf("a ROM font page's .xex is %d bytes", len(out))
	}
	a.fontFor["atari"] = "atarixl" // the international set is no ROM font on a 400/800: carried
	if a.atariProgramFont() == nil {
		t.Fatal("the international set must go in the file")
	}
}

func TestMachAtariROMFont(t *testing.T) {
	testApp(t, "atari")
	rom := make([]byte, 16384)
	copy(rom[0x2000:], atariFontStd)
	copy(rom[0x0C00:], atariFontIntl)
	std, intl := atariROMFonts(rom)
	if !bytes.Equal(std, atariFontStd) || !bytes.Equal(intl, atariFontIntl) {
		t.Fatal("XL ROM")
	}
	osb := make([]byte, 10240)
	copy(osb[0x800:], atariFontStd)
	if std, intl := atariROMFonts(osb); !bytes.Equal(std, atariFontStd) || intl != nil {
		t.Fatal("OS-B ROM")
	}
	if std, _ := atariROMFonts(make([]byte, 16384)); std != nil {
		t.Fatal("an empty ROM gave a font")
	}
	keep := fonts
	defer func() { fonts = keep }()
	if err := os.WriteFile(filepath.Join(appDataDir(), "ATARIXL.ROM"), rom, 0o644); err != nil {
		os.MkdirAll(appDataDir(), 0o755)
		os.WriteFile(filepath.Join(appDataDir(), "ATARIXL.ROM"), rom, 0o644)
	}
	if p := loadAtariROMFonts(); p == "" || fontByID("atarirom") == nil || fontByID("atarixlrom") == nil {
		t.Fatalf("ROM fonts not found (%q)", p)
	}
	a := testApp(t, "atari")
	a.fontFor["atari"] = "atarirom"
	if a.atariProgramFont() != nil {
		t.Fatal("the ROM's own font must not go in the file")
	}
}

// ---------------------------------------------------------------- Spectrum .tap

// machFTapBlocks checks every block of a tape byte for byte: lengths and checksums.
func machFTapBlocks(t *testing.T, b []byte) [][]byte {
	t.Helper()
	var out [][]byte
	for i := 0; i < len(b); {
		n := int(b[i]) | int(b[i+1])<<8
		blk := b[i+2 : i+2+n]
		var x byte
		for _, c := range blk[:n-1] {
			x ^= c
		}
		if x != blk[n-1] {
			t.Fatalf("block at %d: checksum %02X, want %02X", i, blk[n-1], x)
		}
		out = append(out, blk[:n-1])
		i += 2 + n
	}
	return out
}

func TestMachTapBlocks(t *testing.T) {
	scr := make([]byte, 6912)
	for i := range scr {
		scr[i] = byte(i * 7)
	}
	b := writeTap(scr, 5, "My art")
	blocks := machFTapBlocks(t, b)
	if len(blocks) != 4 {
		t.Fatalf("%d blocks", len(blocks))
	}
	prog := blocks[1][1:]
	// 10 BORDER 5: LOAD ""SCREEN$ / 20 PAUSE 0: GO TO 20, as the ROM tokenises them.
	want := []byte{0, 10, 14, 0, 0xE7, '5', 0x0E, 0, 0, 5, 0, 0, ':', 0xEF, '"', '"', 0xAA, 0x0D,
		0, 20, 19, 0, 0xF2, '0', 0x0E, 0, 0, 0, 0, 0, ':', 0xEC, '2', '0', 0x0E, 0, 0, 20, 0, 0, 0x0D}
	if !bytes.Equal(prog, want) {
		t.Fatalf("program\n% x\nwant\n% x", prog, want)
	}
	if h := blocks[0]; h[0] != 0 || h[1] != 0 || string(h[2:12]) != "My art    " ||
		!bytes.Equal(h[12:], []byte{byte(len(want)), 0, 10, 0, byte(len(want)), 0}) {
		t.Fatalf("program header % x", h)
	}
	if h := blocks[2]; h[0] != 0 || h[1] != 3 || string(h[2:12]) != "My art    " || !bytes.Equal(h[12:], []byte{0x00, 0x1B, 0x00, 0x40, 0x00, 0x80}) {
		t.Fatalf("screen header % x", h)
	}
	if blocks[3][0] != 0xFF || !bytes.Equal(blocks[3][1:], scr) {
		t.Fatal("screen block")
	}
	got, border, err := readTap(b)
	if err != nil || border != 5 || !bytes.Equal(got, scr) {
		t.Fatalf("read back: border %d, %v", border, err)
	}
	if zxTapName("/x/A very long name.scr") != "A very lon" || zxTapName("é.tap") != "screen" {
		t.Fatal("tape names")
	}
	// Another tape's loading screen: a header, then the screen with junk after it.
	other := append(zxTapBlock(0, zxTapHeader(3, "game", 6912, 16384, 32768)), zxTapBlock(0xFF, scr)...)
	if got, border, err := readTap(append(other, zxTapBlock(0xFF, []byte{1, 2, 3})...)); err != nil || border != -1 || !bytes.Equal(got, scr) {
		t.Fatal("a loading screen")
	}
	if _, _, err := readTap(zxTapBlock(0, zxTapHeader(0, "prog", 3, 10, 3))); err == nil {
		t.Fatal("a tape without a screen opened")
	}
}

func TestMachTapInEditor(t *testing.T) {
	a := testApp(t, "zx")
	for x := 0; x < 32; x++ {
		a.doc.set(x, x%24, zxCell(65+x%26, x%8, 7-x%8, x%3 == 0, x%5 == 0))
	}
	a.doc.border = 2
	want := a.doc.clone()
	p := filepath.Join(t.TempDir(), "art.tap")
	if !a.writeOut(p, "tap") {
		t.Fatal(a.message)
	}
	b := testApp(t, "pc")
	b.openPath(p, "auto")
	if !b.zx() || b.saveFmt != "tap" || b.doc.border != 2 {
		t.Fatalf("opened as %s (%s), border %d: %s", b.mode, b.saveFmt, b.doc.border, b.message)
	}
	for k, v := range want.cells {
		if b.doc.cells[k] != v {
			t.Fatalf("cell %d: %08x became %08x", k, v, b.doc.cells[k])
		}
	}
	src, _ := os.ReadFile(p)
	if blocks := machFTapBlocks(t, src); string(blocks[0][2:12]) != "art       " || !bytes.Equal(blocks[3][1:], writeSCR(want)) {
		t.Fatal("the tape's name or screen")
	}
	b.typeByte('Q')
	if !b.writeOut(p, b.saveFmt) {
		t.Fatal(b.message)
	}
}
