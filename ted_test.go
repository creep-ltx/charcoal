package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image/png"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// tedFReal: the Plus/4 character set the tests have is the real ROM's (not a made-up one).
var tedFReal bool

// tedFEnsureROM gives the tests the Plus/4's character set: the ROM where the
// app keeps it (or CHARCOAL_TED_ROM, a kernal-318004-05.bin), else a made-up
// one, so that everything but the glyphs' pixels can be tested anywhere.
func tedFEnsureROM(t *testing.T) {
	t.Helper()
	loadC64Fonts()
	if hasROM("plus4") {
		tedFReal = tedFReal || tedFFontIsROM()
		return
	}
	r := cbmROMs[len(cbmROMs)-1]
	if r.family != "plus4" {
		t.Fatal("the Plus/4 ROM is not the last of cbmROMs")
	}
	if p := os.Getenv("CHARCOAL_TED_ROM"); p != "" {
		if b, err := os.ReadFile(p); err == nil && r.ok(b) {
			addCBMFonts(r, b)
			tedFReal = true
			return
		}
	}
	fake := make([]byte, r.size)
	rand.New(rand.NewSource(4)).Read(fake)
	addCBMFonts(r, fake)
}

// tedFFontIsROM: is the loaded Plus/4 set the ROM's? (its SHA-1 over the glyphs)
func tedFFontIsROM() bool {
	f := fontByID("plus4upper")
	sum := sha1.Sum(f.bits)
	return hex.EncodeToString(sum[:]) == tedFUpperSHA1
}

// tedFUpperSHA1 is the SHA-1 of the upper case set as CharEdit holds it (16 bytes a glyph, rows doubled).
const tedFUpperSHA1 = "33c83cf7661e3bc9d81800e17e79bc2653179935"

// tedFApp is an app with an empty Plus/4 page.
func tedFApp(t *testing.T) *App {
	t.Helper()
	if !loadC64Fonts() {
		t.Skip("no C64 character ROM here")
	}
	tedFEnsureROM(t)
	a := testApp(t, "c64")
	a.hasC64 = true
	a.setMachine("plus4")
	if !a.ted() {
		t.Fatalf("not a Plus/4 page: %s %s", a.mode, a.machine)
	}
	return a
}

// tedFPage is a page of random cells: any screen code, any TED colour (or the
// KERNAL's luminance only, as a .seq holds them), some flashing.
func tedFPage(seed int64, cols, rows int, kernal bool) *Doc {
	r := rand.New(rand.NewSource(seed))
	d := newDoc(cols, rows)
	d.bg, d.border, d.glyph0 = r.Intn(128), r.Intn(128), true
	for i := range d.cells {
		sc := r.Intn(256)
		if r.Intn(3) == 0 {
			sc = 0x20
		}
		attr := r.Intn(128)
		if kernal {
			attr = int(tedKernal[attr&15])
		}
		if r.Intn(4) == 0 {
			attr |= 0x80
		}
		d.cells[i] = tedCell(sc, attr)
	}
	return d
}

// tedFSame: do two pages show the same? Spaces show no colour; with hueOnly the luminance is not compared.
func tedFSame(a, b *Doc, hueOnly bool) string {
	for y := 0; y < a.rows; y++ {
		for x := 0; x < a.cols; x++ {
			u, v := a.get(x, y), b.get(x, y)
			if cellCh(u) != cellCh(v) {
				return fmt.Sprintf("cell %d,%d: character %02x, want %02x", x, y, cellCh(v), cellCh(u))
			}
			if cellCh(u) == 0x20 {
				continue
			}
			m := 0xFF
			if hueOnly {
				m = 0x8F
			}
			if tedAttr(u)&m != tedAttr(v)&m {
				return fmt.Sprintf("cell %d,%d (%02x): colour %02x, want %02x", x, y, cellCh(u), tedAttr(v), tedAttr(u))
			}
		}
	}
	return ""
}

func TestTEDPaletteVICE(t *testing.T) {
	p := tedVICE()
	// Read off VICE 3.9 xplus4 screenshots (see TestTEDInXplus4 for all 128).
	for c, want := range map[int]uint32{0x00: 0x000000, 0x71: 0xFFFFFF, 0x7F: 0xDBFF8B, 0x7E: 0xFFDDFF, 0x7D: 0xDEFAFF,
		0x46: 0x6D62FF, 0x32: 0x8E3730, 0x10: 0x000000, 0x01: 0x202020, 0x0E: 0x2E00AC} {
		if got := p.c[c] & 0xFFFFFF; got != want {
			t.Errorf("$%02X: %06X, want %06X", c, got, want)
		}
	}
	// Black is black at every luminance; the others grow lighter.
	for l := 0; l < 8; l++ {
		if p.c[l*16]&0xFFFFFF != 0 {
			t.Errorf("black at luminance %d: %06X", l, p.c[l*16])
		}
	}
	grey := func(c uint32) int { return int(c>>16&255) + int(c>>8&255) + int(c&255) }
	for h := 1; h < 16; h++ {
		for l := 1; l < 8; l++ {
			if grey(p.c[l*16+h]) <= grey(p.c[(l-1)*16+h]) {
				t.Errorf("hue %d: luminance %d is not lighter than %d", h, l, l-1)
			}
		}
	}
	if len(tedPalettes) != 3 || tedPaletteByID("tedyape").c[0x71]&0xFFFFFF != 0xFAFAFA || tedPaletteByID("tedcolodore").c[0x01]&0xFFFFFF != 0x171717 {
		t.Error("the .vpl palettes")
	}
}

func TestTEDKernalROM(t *testing.T) {
	p := os.Getenv("CHARCOAL_TED_ROM")
	var b []byte
	for _, f := range append([]string{p}, cbmROMs[len(cbmROMs)-1].paths()...) {
		if f == "" {
			continue
		}
		if k, err := os.ReadFile(f); err == nil && cbmROMs[len(cbmROMs)-1].ok(k) {
			b = k
			break
		}
	}
	if b == nil {
		t.Skip("no Plus/4 KERNAL ROM here (CHARCOAL_TED_ROM)")
	}
	// The colour codes ($E133) and their colours ($E143), and the TED's power-on table ($F338).
	if !bytes.Equal(b[0xE133-0xC000:0xE143-0xC000], petColour[:]) || !bytes.Equal(b[0xE143-0xC000:0xE153-0xC000], tedKernal[:]) {
		t.Error("the KERNAL's colour tables")
	}
	if b[0xF338-0xC000+0x15] != tedStartBg || b[0xF338-0xC000+0x19] != tedStartBorder {
		t.Error("the power-on colours")
	}
	// The character set: the TED reverses codes 128-255.
	r := cbmROMs[len(cbmROMs)-1]
	if fontByID("plus4upper") == nil || !tedFFontIsROM() {
		loadC64Fonts()
		if fontByID("plus4upper") == nil {
			addCBMFonts(r, b)
		}
	}
	if !tedFFontIsROM() {
		t.Fatal("the upper case set is not the ROM's (SHA-1)")
	}
	f := fontByID("plus4lower")
	for row := 0; row < 8; row++ {
		if f.bits[0x81*16+row*2] != ^b[0x1400+0x01*8+row] || f.bits[0x01*16+row*2] != b[0x1400+0x01*8+row] {
			t.Fatal("lower case 'a' and its reverse")
		}
	}
}

func TestTEDSeqRoundTrip(t *testing.T) {
	for seed := int64(1); seed <= 40; seed++ {
		d := tedFPage(seed, 40, 25, true)
		lower := seed%2 == 0
		b := writeSeqOn(d, lower, seed%3 == 0, true)
		back, l := parseSeqOn(b, 40, true)
		if l != map[bool]int{true: 1, false: 0}[lower] {
			t.Fatalf("seed %d: case %d", seed, l)
		}
		if diff := tedFSame(d, back, false); diff != "" {
			t.Fatalf("seed %d: %s", seed, diff)
		}
	}
	// Any luminance is written as the KERNAL's: the hue and flashing stay.
	d := tedFPage(99, 40, 25, false)
	back, _ := parseSeqOn(writeSeqOn(d, false, false, true), 40, true)
	if diff := tedFSame(d, back, true); diff != "" {
		t.Fatal(diff)
	}
	if n := tedSeqLoss([]*Doc{d}); n == 0 {
		t.Error("no cells counted as changed")
	}
	// The Plus/4's codes: colours at the KERNAL's luminance, FLASH ON/OFF, RETURN ends flashing.
	got, _ := parseSeqOn([]byte("\x1cA\x82B\x84C\x82D\x0dE\x9aF\x16\xc6G\x1bQH"), 40, true)
	for i, want := range []struct {
		x, y, attr int
	}{{0, 0, 0x32}, {1, 0, 0xB2}, {2, 0, 0x32}, {3, 0, 0xB2}, {0, 1, 0x32}, {1, 1, 0x2E}, {2, 1, 0xC6}, {3, 1, 0xC6}} {
		if a := tedAttr(got.get(want.x, want.y)); a != want.attr {
			t.Errorf("%d: %d,%d is $%02X, want $%02X", i, want.x, want.y, a, want.attr)
		}
	}
	if cellCh(got.get(3, 1)) != 8 { // ESC Q is a command, H follows it
		t.Errorf("after ESC: %02x", cellCh(got.get(3, 1)))
	}
	// The same bytes on a C64 page are the C64's colours.
	c64, _ := parseSeq([]byte("\x1cA"), 40)
	if cellFg(c64.get(0, 0)) != 2 || tedAttr(c64.get(0, 0)) != 2 {
		t.Error("a C64 .seq")
	}
}

func TestTEDPrg(t *testing.T) {
	marq := filepath.Join(os.Getenv("HOME"), ".cache", "charcoal-ted", "marq", "z", "petscii_release", "linux64", "data", "template-plus4.prg")
	if b, err := os.ReadFile(marq); err == nil { // the head is Marq's template's
		if !bytes.Equal(b[:tedPrgData], tedPrgHead) || len(b) != tedPrgSize {
			t.Error("not Marq's template")
		}
	}
	for seed := int64(1); seed <= 6; seed++ {
		d := tedFPage(seed, 40, 25, false)
		lower := seed%2 == 0
		prg := writeTEDPrg(d, lower)
		back, l, err := parseTEDPrg(prg)
		if err != nil || l != map[bool]int{true: 1}[lower] || back.bg != d.bg || back.border != d.border {
			t.Fatalf("seed %d: %v, case %d, colours %d %d", seed, err, l, back.bg, back.border)
		}
		for i := range d.cells {
			if back.cells[i] != d.cells[i] {
				t.Fatalf("seed %d: cell %d", seed, i)
			}
		}
		// Run it: the screen at $0C00, the attributes at $0800, the TED's registers.
		mem := make([]byte, 65536)
		copy(mem[0x1001:], prg[2:])
		c64xF6502(t, mem, 0x100D)
		if mem[0xFF15] != byte(d.bg) || mem[0xFF19] != byte(d.border) || mem[0xFF13] != map[bool]byte{true: 0xD4, false: 0xD0}[lower] {
			t.Fatalf("seed %d: registers %02x %02x %02x", seed, mem[0xFF15], mem[0xFF19], mem[0xFF13])
		}
		for i, v := range d.cells {
			if mem[0x0C00+i] != byte(cellCh(v)) || mem[0x0800+i] != byte(tedAttr(v)) {
				t.Fatalf("seed %d: cell %d in memory", seed, i)
			}
		}
	}
	if isTEDPrg(writePrg(newDoc(40, 25), false)) || isOurC64Prg(writeTEDPrg(newDoc(40, 25), false)) {
		t.Error("a C64 program taken for a Plus/4 one, or the other way round")
	}
}

func TestTEDSE(t *testing.T) {
	d := tedFPage(5, 40, 25, false)
	b := writeTEDSE(d)
	if len(b) != 2+2000+48 || b[0] != 0 || b[1] != 0xC0 || !bytes.HasPrefix(b[2+1000:], []byte(" CHARCOAL")) {
		t.Fatalf("the screen file: %d bytes", len(b))
	}
	proj := writeTEDSEProj(d, true, 0x41, 0x9A)
	if len(proj) != 2+21 || proj[2+16] != 0x41 || proj[2+17] != 0x0A || proj[2+18] != 1 || proj[2+19] != 1 {
		t.Fatalf("the project: % x", proj)
	}
	back, lower, err := parseTEDSE(b, proj)
	if err != nil || lower != 1 || back.bg != d.bg || back.border != d.border {
		t.Fatalf("%v %d %d %d", err, lower, back.bg, back.border)
	}
	for i := range d.cells {
		if back.cells[i] != d.cells[i] {
			t.Fatalf("cell %d", i)
		}
	}
	// Without the project: 40 columns, black on black; a project's own size (TEDSE's "save project" leaves out 24 bytes).
	if back, _, err := parseTEDSE(b, nil); err != nil || back.cols != 40 || back.rows != 25 || back.bg != 0 {
		t.Fatalf("no project: %v", err)
	}
	wide := tedFPage(6, 80, 30, false)
	b2 := writeTEDSE(wide)[:2+2*80*30+24]
	if back, _, err := parseTEDSE(b2, writeTEDSEProj(wide, false, 0, 0)); err != nil || back.cols != 80 || back.rows != 30 {
		t.Fatalf("80 x 30: %v", err)
	}
	if !isTEDSE("x.scrn", b) || isTEDSE("x.prg", b) || isTEDSE("x.scrn", b[:100]) {
		t.Error("detection")
	}
	// TEDSE's own screens (github.com/xahmol/TEDScreenEdit: tedse.tscr.prg, tedse.hsc1-4.prg).
	if dir := os.Getenv("CHARCOAL_TEDSE_FILES"); dir != "" {
		files, _ := filepath.Glob(filepath.Join(dir, "tedse.*.prg"))
		n := 0
		for _, f := range files {
			b, _ := os.ReadFile(f)
			if w, _ := tedseSize(len(b), 0, 0); w == 0 {
				continue
			}
			d, _, err := parseTEDSE(b, nil)
			if err != nil || d.inkCells() < 100 {
				t.Errorf("%s: %v", f, err)
			}
			n++
		}
		if n < 5 {
			t.Errorf("only %d of TEDSE's screens", n)
		}
	}
}

func TestTEDMarqAndPetmate(t *testing.T) {
	b, err := os.ReadFile("testdata/marq/plus4-stripes.c")
	if err != nil {
		t.Fatal(err)
	}
	scr, err := parseMarq(b)
	if err != nil || len(scr) != 1 || scr[0].machine != "plus4" {
		t.Fatalf("%v %+v", err, scr)
	}
	d := scr[0].doc
	if d.border != 113 || d.bg != 0 || tedAttr(d.get(39, 24)) != 113 {
		t.Fatalf("colours: %d %d %02x", d.border, d.bg, tedAttr(d.get(39, 24)))
	}
	out := writeMarq(d, false, "plus4")
	if !bytes.Contains(out, []byte("// META: 40 25 PLUS4 upper")) {
		t.Fatal("META")
	}
	back, _ := parseMarq(out)
	if diff := tedFSame(d, back[0].doc, false); diff != "" || back[0].doc.border != 113 {
		t.Fatal(diff)
	}
	// Petmate9's workspace: c16Upper / c16Lower and the TED's attribute bytes.
	p := tedFPage(8, 40, 25, false)
	ws := writePetmateFrames(nil, []PetScreen{{doc: p, lower: 1, fb: -1, machine: "plus4"}})
	var f struct {
		Framebufs []struct {
			Charset         string
			BackgroundColor int
			Framebuf        [][]petmatePixel
		}
	}
	if json.Unmarshal(ws, &f) != nil || f.Framebufs[0].Charset != "c16Lower" || f.Framebufs[0].BackgroundColor != p.bg ||
		f.Framebufs[0].Framebuf[0][0].Color != tedAttr(p.get(0, 0)) {
		t.Fatalf("Petmate: %s", ws[:min(200, len(ws))])
	}
	ps, err := parsePetmate(ws)
	if err != nil || ps[0].machine != "plus4" || ps[0].lower != 1 || ps[0].doc.bg != p.bg {
		t.Fatalf("Petmate back: %v", err)
	}
	if diff := tedFSame(p, ps[0].doc, false); diff != "" {
		t.Fatal(diff)
	}
	// A C64 screen in the same workspace stays a C64's.
	ws = writePetmateFrames(ws, []PetScreen{{doc: newDoc(40, 25), lower: 0, fb: 0}})
	if ps, _ := parsePetmate(ws); ps[0].machine != "" || !strings.Contains(string(ws), `"charset":"upper"`) {
		t.Error("a C64 screen over a Plus/4 one")
	}
}

func TestTEDEditor(t *testing.T) {
	a := tedFApp(t)
	if a.doc.bg != tedStartBg || a.doc.border != tedStartBorder || a.fg != 0 {
		t.Fatalf("a new page: %02x %02x brush %02x", a.doc.bg, a.doc.border, a.fg)
	}
	// Typing in a colour, flashing, reversed.
	a.fg, a.flash = 0x53, true
	a.typeByte(a.charToByte('H'))
	a.flash, a.rev = false, true
	a.typeByte(a.charToByte('i'))
	if v := a.doc.get(0, 0); cellCh(v) != 8 || tedAttr(v) != 0xD3 {
		t.Fatalf("typed: %02x %02x", cellCh(v), tedAttr(v))
	}
	if v := a.doc.get(1, 0); cellCh(v) != 0x89 || tedAttr(v) != 0x53 {
		t.Fatalf("typed reversed: %02x %02x", cellCh(v), tedAttr(v))
	}
	// Pick takes the whole colour and flashing; the Colour tool paints them.
	a.pick(0, 0)
	if a.fg != 0x53 || !a.flash {
		t.Fatal("pick")
	}
	a.tool, a.fg, a.flash = "colour", 0x2A, false
	a.pushUndo()
	a.paintAt(0, 0, 1)
	if v := a.doc.get(0, 0); cellCh(v) != 8 || tedAttr(v) != 0x2A {
		t.Fatalf("colour tool: %02x", tedAttr(v))
	}
	a.tool = "draw"
	// Colour keys: hue i at the brush's luminance, again for i+8; stepping keeps the luminance; lighter, darker.
	a.fg = 0x52
	a.colourKey(6, false)
	if a.fg != 0x56 {
		t.Fatalf("key: %02x", a.fg)
	}
	a.colourKey(6, false)
	if a.fg != 0x5E {
		t.Fatalf("key again: %02x", a.fg)
	}
	a.stepFg(3)
	a.tedLum(1)
	if a.fg != 0x61 {
		t.Fatalf("step and lighter: %02x", a.fg)
	}
	a.tedLum(9)
	if a.fg != 0x71 {
		t.Fatalf("lightest: %02x", a.fg)
	}
	a.colourKey(0, true) // the screen colour: black
	if a.doc.bg != 0x70 {
		t.Fatalf("screen colour: %02x", a.doc.bg)
	}
	a.doUndo()
	if a.doc.bg != tedStartBg {
		t.Fatal("undo the screen colour")
	}
	// The picture: the border as VICE shows it, the cells in their colours.
	fb := a.renderDoc(a.doc, 1)
	if fb.w != 384 || fb.h != 288 {
		t.Fatalf("picture %dx%d", fb.w, fb.h)
	}
	if fb.px[0] != a.tedRGB(tedStartBorder) {
		t.Fatal("border colour")
	}
	a.doc.set(2, 0, tedCell(0xA0, 0x86))
	if tedFReal { // the glyphs' pixels need the ROM's
		if fb.px[40*fb.w+32+16*8] != a.tedRGB(tedStartBg) {
			t.Fatal("screen colour")
		}
		ink := false
		for y := 40; y < 48; y++ {
			for x := 40; x < 48; x++ {
				if fb.px[y*fb.w+x] == a.tedRGB(0x53) {
					ink = true
				}
			}
		}
		if !ink {
			t.Fatal("the reversed i is not in its colour")
		}
		a.blink = false // a flashing cell's ink is off
		fb = a.renderDoc(a.doc, 1)
		if fb.px[44*fb.w+32+20] != a.tedRGB(tedStartBg) {
			t.Fatal("flash off phase")
		}
		a.blink = true
		fb = a.renderDoc(a.doc, 1)
		if fb.px[44*fb.w+32+20] != a.tedRGB(0x06) {
			t.Fatal("flash on phase")
		}
	}
	// Every format of the page saves, and the page's own ones open as the same page.
	want := a.doc.clone()
	dir := t.TempDir()
	for _, f := range []struct {
		fmtID, name string
		exact       bool
	}{{"prg", "a.prg", true}, {"d64", "A Disk.d64", true}, {"tedse", "a.scrn", true}, {"marq", "a.c", false},
		{"petmate", "a.petmate", true}, {"seq", "a.seq", false}, {"png", "a.png", false}, {"html", "a.html", false},
		{"gif", "a.gif", false}, {"utf8", "a.txt", false}} {
		p := filepath.Join(dir, f.name)
		if !a.writeOut(p, f.fmtID) {
			t.Fatalf("%s: %s", f.fmtID, a.message)
		}
		if f.fmtID == "png" {
			r, _ := os.Open(p)
			img, err := png.Decode(r)
			r.Close()
			if err != nil || img.Bounds().Dx() != 384 {
				t.Fatalf("png: %v", err)
			}
			continue
		}
		if f.fmtID == "html" || f.fmtID == "gif" || f.fmtID == "utf8" {
			continue
		}
		b := tedFApp(t)
		b.openPath(p, "auto")
		if !b.ted() {
			t.Fatalf("%s opened as %s %s (%s)", f.name, b.mode, b.machine, b.message)
		}
		switch {
		case f.fmtID == "seq":
			b.doc.bg = want.bg // a .seq has no screen colour
			if diff := tedFSame(want, b.doc, true); diff != "" {
				t.Fatalf("%s: %s", f.name, diff)
			}
		case f.fmtID == "marq": // no flashing in the PETSCII editor
			if b.doc.bg != want.bg || tedAttr(b.doc.get(0, 0)) != tedAttr(want.get(0, 0))&127 {
				t.Fatalf("%s", f.name)
			}
		default:
			if diff := tedFSame(want, b.doc, false); diff != "" || b.doc.bg != want.bg || b.doc.borderColour() != want.borderColour() {
				t.Fatalf("%s: %s (%02x %02x)", f.name, diff, b.doc.bg, b.doc.border)
			}
		}
		if f.exact && b.saveFmt != f.fmtID {
			t.Errorf("%s: saved again as %s", f.name, b.saveFmt)
		}
	}
}

func TestTEDMachineSwitch(t *testing.T) {
	a := tedFApp(t)
	a.setMachine("c64")
	a.fg = 2 // red
	a.typeByte(a.charToByte('X'))
	a.doc.bg = 6
	a.setMachine("plus4")
	c64 := paletteByID("c64").colors
	if !a.ted() || a.doc.bg != a.tedNearest(c64[6]&0xFFFFFF) || tedAttr(a.doc.get(0, 0)) != a.tedNearest(c64[2]&0xFFFFFF) || tedAttr(a.doc.get(0, 0))&15 != 2 {
		t.Fatalf("to the Plus/4: bg %02x cell %02x", a.doc.bg, tedAttr(a.doc.get(0, 0)))
	}
	if a.fg != tedAttr(a.doc.get(0, 0)) {
		t.Fatalf("the brush: %02x", a.fg)
	}
	a.doUndo()
	if a.ted() || a.doc.bg != 6 || cellFg(a.doc.get(0, 0)) != 2 {
		t.Fatalf("undone: %s %d", a.machine, a.doc.bg)
	}
	a.doRedo()
	if !a.ted() {
		t.Fatal("redone")
	}
	a.setMachine("c64")
	if a.ted() || a.doc.bg > 15 || cellFg(a.doc.get(0, 0)) != 2 || tedAttr(a.doc.get(0, 0)) != 2 {
		t.Fatalf("back to the C64: %d %d", a.doc.bg, cellFg(a.doc.get(0, 0)))
	}
}

// Animate's draw orders and effects on a Plus/4 page: the saved stream shows the page.
func TestTEDAnimate(t *testing.T) {
	a := tedFApp(t)
	a.doc = tedFPage(3, 40, 25, true)
	want := a.doc.clone()
	for _, pat := range []int{patTop, patScramble, patSpiral, patFadeIn} {
		a.order = &drawOrder{Pattern: patterns[pat].id}
		b, err := a.encode("seq")
		if err != nil {
			t.Fatal(err)
		}
		back, _ := parseSeqOn(b, 40, true)
		if diff := tedFSame(want, back, false); diff != "" {
			t.Fatalf("pattern %d: %s", pat, diff)
		}
	}
	a.order = nil
	e := newMachineDoc(a.mach())
	for pat := 0; pat < nPatterns; pat++ {
		if !isEffect(pat) {
			continue
		}
		frames := a.effectFrames(want, e, pat, 7, 0)
		if diff := tedFSame(want, frames[len(frames)-1], false); diff != "" {
			t.Fatalf("effect %d: %s", pat, diff)
		}
	}
	// Fading goes through TED colours.
	if c := a.fadeCell(tedCell(1, 0x71), a.tedRGB(0), 0.5); tedAttr(c) == 0x71 || tedAttr(c) == 0 {
		t.Errorf("half faded white: %02x", tedAttr(c))
	}
}

// TestTEDInXplus4 runs CharEdit's .prg in VICE's xplus4 and compares its
// screenshot with CharEdit's picture, pixel for pixel, border included.
// CHARCOAL_XPLUS4 is a program taking (prg, png): it runs the program and
// saves the screen (e.g. xplus4 -autostartprgmode 1 -limitcycles 30000000
// -exitscreenshot png -autostart prg, with the ROMs in its folder).
func TestTEDInXplus4(t *testing.T) {
	run := os.Getenv("CHARCOAL_XPLUS4")
	if run == "" {
		t.Skip("CHARCOAL_XPLUS4 not set")
	}
	a := tedFApp(t)
	if !tedFReal {
		t.Skip("not the real character ROM")
	}
	dir := t.TempDir()
	pages := []*Doc{tedFPage(11, 40, 25, false), newDoc(40, 25)}
	// All 128 colours as reversed spaces, 2 cells each.
	g := pages[1]
	g.bg, g.border, g.glyph0 = 0x71, 0x00, true
	for i := range g.cells {
		g.cells[i] = tedCell(1, 0x00)
		if i < 256 {
			g.cells[i] = tedCell(0xA0, i/2)
		}
	}
	for i, d := range pages {
		for c := range d.cells { // no flashing: the screenshot's moment is not known
			d.cells[c] &^= 0x8000
		}
		a.doc = d
		for _, lower := range []bool{false, true} {
			a.setCase(lower)
			prg := filepath.Join(dir, fmt.Sprintf("p%d%v.prg", i, lower))
			shot := filepath.Join(dir, fmt.Sprintf("p%d%v.png", i, lower))
			os.WriteFile(prg, writeTEDPrg(d, lower), 0o644)
			if out, err := exec.Command(run, prg, shot).CombinedOutput(); err != nil {
				t.Fatalf("%v: %s", err, out)
			}
			r, _ := os.Open(shot)
			img, err := png.Decode(r)
			r.Close()
			if err != nil {
				t.Fatal(err)
			}
			fb := a.renderDoc(d, 1)
			if img.Bounds().Dx() != fb.w || img.Bounds().Dy() != fb.h {
				t.Fatalf("VICE %v, CharEdit %dx%d", img.Bounds(), fb.w, fb.h)
			}
			bad := 0
			for y := 0; y < fb.h; y++ {
				for x := 0; x < fb.w; x++ {
					r, gg, b, _ := img.At(x, y).RGBA()
					if uint32(r>>8)<<16|uint32(gg>>8)<<8|uint32(b>>8) != fb.px[y*fb.w+x]&0xFFFFFF {
						bad++
					}
				}
			}
			if bad > 0 {
				t.Errorf("page %d lower %v: %d pixels differ", i, lower, bad)
			}
		}
	}
}

// FuzzTED: any stream a Plus/4 prints, CharEdit's .seq shows again (hue and
// flashing; the luminance is the KERNAL's), and the Plus/4 readers take any bytes.
func FuzzTED(f *testing.F) {
	d := tedFPage(2, 40, 25, true)
	f.Add(writeSeqOn(d, true, true, true))
	f.Add(writeTEDPrg(d, false))
	f.Add(writeTEDSE(d))
	f.Add([]byte("\x12\x22\x1c\x82A\x0d\x16\xff\x1b\x1bB\x14\x94\x8e\x0e"))
	f.Fuzz(func(t *testing.T, b []byte) {
		d, l := parseSeqOn(b, 40, true)
		if d.rows*d.cols > 64000 {
			return
		}
		back, l2 := parseSeqOn(writeSeqOn(d, l == 1, false, true), 40, true)
		if diff := tedFSame(d, back, true); diff != "" || (l == 1) != (l2 == 1) {
			t.Fatalf("%s (case %d %d)", diff, l, l2)
		}
		parseTEDPrg(b)
		parseTEDSE(b, b)
		if len(b) > 30 {
			parseTEDSE(b[23:], b[:23])
		}
	})
}

// TestTEDMutations is FuzzTED's check over 20000 mutated streams (the fuzz
// engine stalls in this package, as the HANDOFF says).
func TestTEDMutations(t *testing.T) {
	r := rand.New(rand.NewSource(16))
	seeds := [][]byte{writeSeqOn(tedFPage(2, 40, 25, true), true, true, true), writeSeqOn(tedFPage(3, 40, 25, false), false, false, true),
		[]byte("\x12\x22\x1c\x82A\x0d\x16\xff\x1b\x1bB\x14\x94\x8e\x0e")}
	codes := []byte{0x0D, 0x8D, 0x12, 0x92, 0x82, 0x84, 0x16, 0x1B, 0x22, 0x14, 0x94, 0x13, 0x93, 0x11, 0x91, 0x1D, 0x9D, 0x1C, 0x9A, 0x0E, 0x8E}
	for i := 0; i < 20000; i++ {
		b := append([]byte{}, seeds[r.Intn(len(seeds))]...)
		for n := r.Intn(12); n >= 0 && len(b) > 0; n-- {
			p := r.Intn(len(b))
			switch r.Intn(3) {
			case 0:
				b[p] = codes[r.Intn(len(codes))]
			case 1:
				b[p] = byte(r.Intn(256))
			default:
				b = append(b[:p], b[min(len(b), p+r.Intn(40)):]...)
			}
		}
		d, l := parseSeqOn(b, 40, true)
		back, _ := parseSeqOn(writeSeqOn(d, l == 1, false, true), 40, true)
		if diff := tedFSame(d, back, true); diff != "" {
			t.Fatalf("mutation %d: %s\n% x", i, diff, b)
		}
	}
}

func TestTEDAutosave(t *testing.T) {
	a := tedFApp(t)
	a.doc = tedFPage(21, 40, 25, false)
	a.dirty = true
	want := a.doc.clone()
	if err := writeAutosave(a.Tab); err != nil {
		t.Fatal(err)
	}
	b := restart(t, "pc")
	b.hasC64 = true
	files := leftAutosaves()
	if len(files) != 1 || b.restoreAutosave(files[0]) != nil {
		t.Fatal("autosave")
	}
	if !b.ted() || b.doc.bg != want.bg || b.doc.border != want.border {
		t.Fatalf("autosave: %s %s %02x", b.mode, b.machine, b.doc.bg)
	}
	if diff := tedFSame(want, b.doc, false); diff != "" {
		t.Fatal(diff)
	}
}

// Petmate9's own C16 workspace: a Plus/4 page with its colours; saved again it keeps Petmate9's fields.
func TestTEDPetmate9File(t *testing.T) {
	a := tedFApp(t)
	a.setMachine("c64")
	a.openPath("testdata/petmate9/c16_colorbars.petmate", "auto")
	if !a.ted() || a.doc.bg != 113 || a.doc.border != 107 {
		t.Fatalf("%s %s %d %d (%s)", a.mode, a.machine, a.doc.bg, a.doc.border, a.message)
	}
	for c := 0; c < 128; c++ { // rows 2-9: two cells of every colour, from column 4
		y, x := 2+c/16, 4+(c%16)*2
		if v := a.doc.get(x, y); cellCh(v) != 160 || tedAttr(v) != c {
			t.Fatalf("colour %d at %d,%d: %02x %02x", c, x, y, cellCh(v), tedAttr(v))
		}
	}
	p := filepath.Join(t.TempDir(), "bars.petmate")
	a.pushUndo()
	a.doc.set(0, 0, tedCell(1, 0xF2))
	if !a.writeOut(p, "petmate") {
		t.Fatal(a.message)
	}
	b, _ := os.ReadFile(p)
	for _, want := range []string{`"charset":"c16Upper"`, `"borderOn":true`, `"zoom":`, `"name":"TED Color Bars"`, `{"code":1,"color":242}`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("saved again without %s", want)
		}
	}
}

// CHARCOAL_PETMATE9 = a Petmate9 checkout: its C16 workspaces open as Plus/4 pages, every screen.
func TestTEDPetmate9Checkout(t *testing.T) {
	dir := os.Getenv("CHARCOAL_PETMATE9")
	if dir == "" {
		t.Skip("CHARCOAL_PETMATE9 not set")
	}
	files, _ := filepath.Glob(filepath.Join(dir, "_tests", "c16*.petmate"))
	if len(files) < 2 {
		t.Fatalf("only %d files", len(files))
	}
	for _, f := range files {
		a := tedFApp(t)
		a.openPath(f, "auto")
		b, _ := os.ReadFile(f)
		scr, _ := parsePetmate(b)
		if !a.ted() || a.nFrames() != len(scr) || a.doc.inkCells() == 0 {
			t.Errorf("%s: %s %s, %d frames of %d", f, a.mode, a.machine, a.nFrames(), len(scr))
		}
	}
}
