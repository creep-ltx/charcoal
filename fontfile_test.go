package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// testGlyphs gives n glyphs of h rows that differ from each other.
func testGlyphs(n, h int) []byte {
	b := make([]byte, n*h)
	for i := range b {
		b[i] = byte(i*7 + i/h*13)
	}
	return b
}

func TestRawPCFontSizes(t *testing.T) {
	for _, h := range []int{8, 14, 16} {
		data := testGlyphs(256, h)
		f, low, err := parseFontFile("/x/My Font.f"+string(rune('0'+h%10)), data, "pc", nil)
		if err != nil || low != nil {
			t.Fatalf("%d rows: %v", h, err)
		}
		if f.rows() != h || f.name != "My Font" || f.kind != "pc" || f.id != ownID {
			t.Errorf("%d rows: got rows %d name %q kind %q id %q", h, f.rows(), f.name, f.kind, f.id)
		}
		if !bytes.Equal(f.native(), data) {
			t.Errorf("%d rows: the glyphs changed", h)
		}
		out, _ := fontFileBytes(f, nil, ".f16")
		if !bytes.Equal(out, data) {
			t.Errorf("%d rows: saved raw differs", h)
		}
	}
	for _, n := range []int{100, 1000, 256 * 20, 768} {
		if _, _, err := parseFontFile("x.fnt", make([]byte, n), "pc", nil); err == nil {
			t.Errorf("%d bytes taken as a PC font", n)
		}
	}
	if _, _, err := parseFontFile("x.fnt", make([]byte, 2048), "st", nil); err == nil {
		t.Error("an ST page took a font")
	}
}

func TestPSFHeaders(t *testing.T) {
	data := testGlyphs(256, 16)
	psf1 := append([]byte{0x36, 0x04, 0, 16}, data...)
	f, _, err := parseFontFile("a.psf", psf1, "pc", nil)
	if err != nil || f.rows() != 16 || f.format != "psf1" || !bytes.Equal(f.native(), data) {
		t.Fatalf("PSF1: %v", err)
	}
	if out, _ := fontFileBytes(f, nil, ".psf"); !bytes.Equal(out, psf1) {
		t.Error("PSF1 written back differs")
	}
	// 512 glyphs (mode bit 0): the first 256 are used.
	big := append([]byte{0x36, 0x04, 1, 8}, testGlyphs(512, 8)...)
	if f, _, err := parseFontFile("b.psf", big, "pc", nil); err != nil || f.rows() != 8 || !bytes.Equal(f.native(), big[4:4+2048]) {
		t.Errorf("PSF1 of 512: %v", err)
	}
	if _, _, err := parseFontFile("c.psf", psf1[:100], "pc", nil); err == nil {
		t.Error("a cut PSF1 was taken")
	}

	psf2 := func(width, height, count, head int) []byte {
		h := make([]byte, head)
		for i, v := range []uint32{0x864AB572, 0, uint32(head), 0, uint32(count), uint32(height * ((width + 7) / 8)), uint32(height), uint32(width)} {
			binary.LittleEndian.PutUint32(h[i*4:], v)
		}
		return append(h, testGlyphs(count, height*((width+7)/8))...)
	}
	f, _, err = parseFontFile("d.psfu", psf2(8, 14, 256, 40), "amiga", nil)
	if err != nil || f.rows() != 14 || f.format != "psf2" || f.kind != "amiga" {
		t.Fatalf("PSF2 with a 40-byte header: %v", err)
	}
	if !bytes.Equal(f.native(), testGlyphs(256, 14)) {
		t.Error("PSF2 glyphs differ")
	}
	out, _ := fontFileBytes(f, nil, ".psf")
	g, _, err := parseFontFile("e.psf", out, "pc", nil)
	if err != nil || !bytes.Equal(g.native(), f.native()) {
		t.Errorf("PSF2 round trip: %v", err)
	}
	if _, _, err := parseFontFile("f.psf", psf2(9, 16, 256, 32), "pc", nil); err == nil || !strings.Contains(err.Error(), "9 pixels") {
		t.Errorf("a 9-wide PSF2: %v", err)
	}
	bad := psf2(8, 16, 256, 32)
	binary.LittleEndian.PutUint32(bad[20:], 15) // charsize not height
	if _, _, err := parseFontFile("g.psf", bad, "pc", nil); err == nil {
		t.Error("a PSF2 whose header does not fit was taken")
	}
}

func TestC64Charsets(t *testing.T) {
	one := testGlyphs(256, 8)
	// .64c: two bytes of load address first.
	f, low, err := parseFontFile("set.64c", append([]byte{0x00, 0x38}, one...), "c64", nil)
	if err != nil || low != nil || f.rows() != 8 || f.kind != "c64" || !bytes.Equal(f.native(), one) {
		t.Fatalf(".64c: %v", err)
	}
	out, _ := fontFileBytes(f, nil, ".64c")
	if len(out) != 2050 || !bytes.Equal(out[2:], one) {
		t.Errorf(".64c written: %d bytes", len(out))
	}
	// Both sets in one file.
	both := testGlyphs(512, 8)
	up, low, err := parseFontFile("both.bin", both, "c64", nil)
	if err != nil || low == nil || low.id != ownLowerID || !strings.HasSuffix(low.id, "lower") {
		t.Fatalf("4096 bytes: %v", err)
	}
	if !bytes.Equal(up.native(), both[:2048]) || !bytes.Equal(low.native(), both[2048:]) {
		t.Error("4096 bytes: the sets differ")
	}
	if out, _ := fontFileBytes(up, low, ".bin"); !bytes.Equal(out, both) {
		t.Error("both sets written back differ")
	}
	// 128 characters: the top half shows them reversed.
	half := testGlyphs(128, 8)
	f, _, err = parseFontFile("half.bin", half, "c64", nil)
	if err != nil || f.row(130, 3) != ^half[2*8+3] {
		t.Errorf("1024 bytes: %v", err)
	}
	// A .64c may be short (Petmate reads up to 2048 bytes after the address).
	if f, _, err := parseFontFile("short.64c", append([]byte{0, 0x30}, half[:512]...), "c64", nil); err != nil || f.row(63, 7) != half[63*8+7] {
		t.Errorf("short .64c: %v", err)
	}
	for _, n := range []int{100, 3000, 768} {
		if _, _, err := parseFontFile("x.bin", make([]byte, n), "c64", nil); err == nil {
			t.Errorf("%d bytes taken as a C64 set", n)
		}
	}
}

func TestAtariFontFile(t *testing.T) {
	f, _, err := parseFontFile("altirra.fnt", atariFontStd, "atari", nil)
	if err != nil || f.rows() != 8 {
		t.Fatal(err)
	}
	if !bytes.Equal(f.bits, fontByID("atari").bits) {
		t.Error("the loaded ROM font is not the built-in one")
	}
	if out, _ := fontFileBytes(f, nil, ".fnt"); !bytes.Equal(out, atariFontStd) {
		t.Error("written back in the ROM's order, it differs")
	}
	if _, _, err := parseFontFile("x.fnt", make([]byte, 2048), "atari", nil); err == nil {
		t.Error("2048 bytes taken as an Atari font")
	}
}

func TestZXFontAndUDGs(t *testing.T) {
	set := testGlyphs(96, 8)
	f, _, err := parseFontFile("mine.ch8", set, "zx", zxFont)
	if err != nil || f.row('A', 2) != set[('A'-32)*8+2] {
		t.Fatal(err)
	}
	if f.row(130, 0) != zxFont.row(130, 0) || f.row(150, 4) != zxFont.row(150, 4) {
		t.Error("the block graphics or UDGs changed with the character set")
	}
	if out, _ := fontFileBytes(f, nil, ".ch8"); !bytes.Equal(out, set) {
		t.Error("768 bytes written back differ")
	}
	udg := testGlyphs(21, 8)
	g, _, err := parseFontFile("game.udg", udg, "zx", f)
	if err != nil || g.row(144, 1) != udg[1] || g.row(164, 7) != udg[20*8+7] || g.row('A', 2) != set[('A'-32)*8+2] {
		t.Fatalf("UDGs onto the loaded set: %v", err)
	}
	if out, _ := fontFileBytes(g, nil, ".udg"); !bytes.Equal(out, udg) {
		t.Error("UDGs written back differ")
	}
	if zxFont.row('A', 2) == set[('A'-32)*8+2] && set[('A'-32)*8+2] != 0x7C {
		t.Error("the built-in Spectrum font was changed")
	}
}

func TestBuiltInPCFonts(t *testing.T) {
	for _, c := range []struct {
		id, sauce, file string
		rows            int
	}{{"ega", "IBM EGA", "fonts/IBM_EGA_8x14.f14", 14}, {"vga50", "IBM VGA50", "fonts/IBM_VGA_8x8.f08", 8},
		{"vga850", "IBM VGA 850", "fonts/IBM_VGA_8x16_850.f16", 16}, {"vga866", "IBM VGA 866", "fonts/IBM_VGA_8x16_866.f16", 16}} {
		f := fontByID(c.id)
		if f == nil || f.kind != "pc" || f.rows() != c.rows || f.sauce != c.sauce {
			t.Fatalf("%s: %+v", c.id, f)
		}
		b, _ := os.ReadFile(c.file)
		if !bytes.Equal(f.native(), b) {
			t.Errorf("%s is not its file", c.id)
		}
		if fontBySauce(c.sauce, "pc") != f || fontBySauce(c.sauce, "amiga") != nil {
			t.Errorf("SAUCE %q", c.sauce)
		}
	}
	for name, id := range map[string]string{"IBM VGA": "vga", "ibm vga 437": "vga", "IBM EGA43": "vga50", "IBM VGA50 437": "vga50", " IBM EGA ": "ega"} {
		if f := fontBySauce(name, "pc"); f == nil || f.id != id {
			t.Errorf("SAUCE %q gives %v", name, f)
		}
	}
	// The full block fills the cell; the EGA's A is its ROM's.
	ega := fontByID("ega")
	for r := 0; r < 14; r++ {
		if ega.row(0xDB, r) != 0xFF {
			t.Fatal("EGA full block")
		}
	}
	if !bytes.Equal(glyphRows(ega, 'A'), []byte{0, 0, 0x10, 0x38, 0x6C, 0xC6, 0xC6, 0xFE, 0xC6, 0xC6, 0xC6, 0, 0, 0}) {
		t.Errorf("EGA A = % x", glyphRows(ega, 'A'))
	}
	a := testApp(t, "pc")
	for id, h := range map[string]int{"vga": 16, "ega": 14, "vga50": 8, "vga866": 16} {
		a.fontFor["pc"] = id
		if a.cellH() != h {
			t.Errorf("%s: cell height %d", id, a.cellH())
		}
	}
	// A SAUCE font name picks the font when a file opens, and saving writes it.
	a.fontFor["pc"] = "ega"
	a.doc.set(0, 0, makeCell('A', 7, 0, 0))
	a.info.writeSauce = true
	ans, err := a.encode("ansi")
	if err != nil {
		t.Fatal(err)
	}
	if s, _ := readSauce(ans); s == nil || s.font != "IBM EGA" {
		t.Fatalf("SAUCE written: %+v", s)
	}
	b := testApp(t, "pc")
	b.loadBytes(ans, "x.ans", "", "auto")
	if b.font().id != "ega" || b.cellH() != 14 {
		t.Errorf("opened with %s", b.font().id)
	}
}

func TestXBinKeepsA14RowFont(t *testing.T) {
	f := newFont(ownID, "mine", "pc", testGlyphs(256, 14), 14, 256)
	d := newDoc(4, 2)
	d.set(1, 1, makeCell('Q', 3, 1, 0))
	x := writeXBin(d, palettes[0].colors, f, 14, false)
	if x[9] != 14 {
		t.Fatalf("font height %d", x[9])
	}
	back, info, err := parseXBin(x)
	if err != nil || info.font == nil || info.cellH != 14 || info.font.rows() != 14 {
		t.Fatalf("%v %+v", err, info)
	}
	if !bytes.Equal(info.font.native(), f.native()) || back.get(1, 1) != d.get(1, 1) {
		t.Error("the XBin's font or cells changed")
	}
	// 8 rows stay 8 rows (doubled inside).
	g := newFont(ownID, "eight", "pc", testGlyphs(256, 8), 8, 256)
	_, info, _ = parseXBin(writeXBin(d, palettes[0].colors, g, 8, false))
	if info.cellH != 8 || !bytes.Equal(info.font.native(), g.native()) {
		t.Error("8-row XBin font")
	}
	// A page with its own font opens and saves the same.
	a := testApp(t, "pc")
	a.loadBytes(x, "x.xb", "", "auto")
	if a.cellH() != 14 || a.font() != a.ownFont {
		t.Fatalf("XBin page: cell height %d", a.cellH())
	}
	again, _ := a.encode("xbin")
	if !bytes.Equal(again, x) {
		t.Error("an XBin with a 14-row font saves differently")
	}
}

func TestPetmateCustomFonts(t *testing.T) {
	d := newDoc(40, 25)
	d.set(0, 0, makeCell(1, 1, 0, 0))
	ws := writePetmateFrames(nil, []PetScreen{{doc: d, lower: 0, fb: -1}, {doc: d.clone(), lower: 0, fb: -1}})
	if petmateOwnFont(ws) != nil {
		t.Fatal("a workspace without custom fonts gave one")
	}
	f := newFont(ownID, "Bubbles", "c64", testGlyphs(256, 8), 8, 256)
	with := petmateWithFont(ws, f, nil, []int{0, 0})
	var top struct {
		Framebufs []struct {
			Charset string `json:"charset"`
		} `json:"framebufs"`
		CustomFonts map[string]petmateFont `json:"customFonts"`
	}
	if err := json.Unmarshal(with, &top); err != nil {
		t.Fatal(err)
	}
	cf, ok := top.CustomFonts["custom_1"]
	if !ok || cf.Name != "Bubbles" || len(cf.Font.Bits) != 2048 || len(cf.Font.CharOrder) != 256 || cf.Font.CharOrder[255] != 255 {
		t.Fatalf("customFonts: %+v", top.CustomFonts)
	}
	for _, fb := range top.Framebufs {
		if fb.Charset != "custom_1" {
			t.Errorf("framebuf charset %q", fb.Charset)
		}
	}
	back := petmateOwnFont(with)
	if back == nil || back.name != "Bubbles" || !bytes.Equal(back.native(), f.native()) || back.format != "petmate:custom_1" {
		t.Fatalf("read back: %+v", back)
	}
	// Saved again (edited), the font keeps its id; the screens still parse.
	back.setRow(65, 0, 0xAA)
	again := petmateWithFont(writePetmateFrames(with, []PetScreen{{doc: d, fb: 0}, {doc: d, fb: 1}}), back, nil, []int{0, 0})
	top.CustomFonts = nil
	json.Unmarshal(again, &top)
	if len(top.CustomFonts) != 1 || top.CustomFonts["custom_1"].Font.Bits[65*8] != 0xAA {
		t.Errorf("saved again: %d fonts", len(top.CustomFonts))
	}
	if scr, err := parsePetmate(again); err != nil || len(scr) != 2 || scr[0].doc.get(0, 0) != d.get(0, 0) {
		t.Errorf("screens: %v", err)
	}
	// With the ROM's set chosen, the screens name it again (the custom font stays in the file).
	rom := petmateWithFont(again, nil, nil, []int{0, 1})
	json.Unmarshal(rom, &top)
	if top.Framebufs[0].Charset != "upper" || top.Framebufs[1].Charset != "lower" {
		t.Errorf("ROM charsets: %+v", top.Framebufs)
	}
	// A real Petmate workspace (its own keys) keeps them.
	src := []byte(`{"version":2,"screens":[0],"framebufs":[{"width":2,"height":1,"backgroundColor":6,"borderColor":14,"charset":"custom_2","zoom":{"zoomLevel":2},"framebuf":[[{"code":1,"color":3},{"code":2,"color":4}]]}],"customFonts":{"custom_2":{"name":"thin","font":{"bits":[` + strings.Repeat("255,", 2047) + `255],"charOrder":[0,1]}}}}`)
	pf := petmateOwnFont(src)
	if pf == nil || pf.name != "thin" || pf.row(200, 5) != 0xFF {
		t.Fatalf("Petmate's own: %+v", pf)
	}
	scr, _ := parsePetmate(src)
	out := petmateWithFont(writePetmateFrames(src, scr), pf, nil, []int{0})
	if !bytes.Contains(out, []byte(`"zoom":{"zoomLevel":2}`)) || !bytes.Contains(out, []byte(`"charset":"custom_2"`)) || bytes.Contains(out, []byte("custom_1")) {
		t.Errorf("Petmate's fields: %s", out[:200])
	}
}

func TestArtFontKeptForANSI(t *testing.T) {
	a := testApp(t, "pc")
	dir := t.TempDir()
	fontPath := filepath.Join(dir, "Thin.f14")
	glyphs := testGlyphs(256, 14)
	os.WriteFile(fontPath, glyphs, 0o644)
	a.loadFontFile(fontPath)
	if a.ownInUse() == nil || a.cellH() != 14 || !a.dirty {
		t.Fatalf("font not in use: %s", a.message)
	}
	a.doc.set(0, 0, makeCell('#', 2, 0, 0))
	art := filepath.Join(dir, "pic.ans")
	if !a.writeOut(art, "ansi") {
		t.Fatal(a.message)
	}
	if !strings.Contains(a.message, "a .ans cannot hold the font") {
		t.Errorf("status line: %s", a.message)
	}
	e, ok := prefs.ArtFonts[art]
	if !ok || e.H != 14 || e.Name != "Thin" {
		t.Fatalf("not remembered: %+v", prefs.ArtFonts)
	}
	// prefs.json on disk has it (another start reads it).
	prefs = Prefs{}
	loadPrefs()
	if _, ok := prefs.ArtFonts[art]; !ok {
		t.Fatal("prefs.json lost the font")
	}
	// Opened again (another tab): the font comes back.
	b, _ := os.ReadFile(art)
	a.focusTab(a.newTab("pc"))
	a.loadBytes(b, "pic.ans", art, "auto")
	a.restoreArtFont(art)
	if a.ownInUse() == nil || a.cellH() != 14 || !bytes.Equal(a.font().native(), glyphs) || a.dirty {
		t.Fatalf("not restored: %s", a.message)
	}
	// Saved as an XBin it holds the font itself: nothing to remember for that file.
	xb := filepath.Join(dir, "pic.xb")
	if !a.writeOut(xb, "xbin") || strings.Contains(a.message, "cannot") {
		t.Fatal(a.message)
	}
	if _, ok := prefs.ArtFonts[xb]; ok {
		t.Error("an XBin's font was remembered too")
	}
	// The ROM font chosen again and saved: the file forgets the kept font.
	a.fontFor["pc"] = "vga"
	a.writeOut(art, "ansi")
	if _, ok := prefs.ArtFonts[art]; ok {
		t.Error("a file saved with the built-in font still has a kept one")
	}
	prefs = Prefs{}
	loadPrefs()
	if _, ok := prefs.ArtFonts[art]; ok {
		t.Error("prefs.json still has the dropped font")
	}
}

func TestAutosaveKeepsOwnFont(t *testing.T) {
	a := testApp(t, "amiga")
	f := newFont(ownID, "Mine", "amiga", testGlyphs(256, 8), 8, 256)
	a.useOwnFont(f, nil, true)
	a.doc.set(2, 0, makeCell('x', 7, 0, 0))
	if err := writeAutosave(a.Tab); err != nil {
		t.Fatal(err)
	}
	id := a.autoID
	if _, err := os.Stat(filepath.Join(autosaveDir(), id+".font")); err != nil {
		t.Fatal("no .font beside the autosave")
	}
	js, _ := os.ReadFile(filepath.Join(autosaveDir(), id+".json"))
	var m autosaveMeta
	json.Unmarshal(js, &m)
	a.focusTab(a.newTab("pc"))
	a.doc.set(0, 0, makeCell('y', 7, 0, 0)) // not pristine: the autosave gets a tab of its own
	if err := a.restoreAutosave(autosaveFile{id, m}); err != nil {
		t.Fatal(err)
	}
	if a.mode != "amiga" || a.ownInUse() == nil || a.cellH() != 8 || !bytes.Equal(a.font().native(), f.native()) || a.font().name != "Mine" {
		t.Fatalf("restored: mode %s font %v", a.mode, a.font().name)
	}
	removeAutosave(a.autoID)
	if _, err := os.Stat(filepath.Join(autosaveDir(), a.autoID+".font")); err == nil {
		t.Error("the .font stays after the autosave is dropped")
	}
}

func TestFontEditorOperations(t *testing.T) {
	f := newFont(ownID, "t", "pc", make([]byte, 14*256), 14, 256)
	glyphSetPixel(f, 'A', 0, 0, true)
	glyphSetPixel(f, 'A', 7, 13, true)
	if f.row('A', 0) != 0x80 || f.row('A', 13) != 0x01 {
		t.Fatal("pixels")
	}
	glyphFlipH(f, 'A')
	if f.row('A', 0) != 0x01 || f.row('A', 13) != 0x80 {
		t.Error("flip H")
	}
	glyphFlipV(f, 'A')
	if f.row('A', 0) != 0x80 || f.row('A', 13) != 0x01 {
		t.Error("flip V")
	}
	glyphShift(f, 'A', 1, 1) // right and down; what goes out comes in on the other side
	if f.row('A', 1) != 0x40 || f.row('A', 0) != 0x80 {
		t.Errorf("shift: % x", glyphRows(f, 'A'))
	}
	glyphShift(f, 'A', -1, -1)
	glyphInvert(f, 'A')
	if f.row('A', 0) != 0x7F || f.row('A', 5) != 0xFF {
		t.Error("invert")
	}
	glyphClear(f, 'A')
	if !bytes.Equal(glyphRows(f, 'A'), make([]byte, 14)) {
		t.Error("clear")
	}
	if f.bits['A'*16+14] != 0 || f.bits['A'*16+15] != 0 {
		t.Error("rows past 14 were written")
	}
	// An 8-row font keeps its rows doubled.
	g := newFont(ownID, "e", "c64", make([]byte, 2048), 8, 256)
	glyphSetPixel(g, 1, 3, 2, true)
	if g.bits[16+4] != 0x10 || g.bits[16+5] != 0x10 {
		t.Error("a doubled row was set once")
	}

	// In the editor: a change makes a copy of the built-in font; undo and redo.
	a := testApp(t, "pc")
	vga := fontByID("vga")
	before := append([]byte(nil), vga.bits...)
	a.brush = 'B'
	d := &FontEd{}
	d.op(glyphInvert)
	if a.ownInUse() == nil || a.font().name != "IBM VGA (CP437) (edited)" || !a.dirty {
		t.Fatal("no copy made")
	}
	if !bytes.Equal(vga.bits, before) {
		t.Fatal("the built-in font was changed")
	}
	if a.font().row('B', 3) != ^vga.row('B', 3) {
		t.Error("invert in the editor")
	}
	d.copyGlyph()
	a.brush = 'C'
	d.pasteGlyph()
	if !bytes.Equal(glyphRows(a.font(), 'C'), glyphRows(a.font(), 'B')) {
		t.Error("copy and paste")
	}
	d.undoStep()
	if !bytes.Equal(glyphRows(a.font(), 'C'), glyphRows(vga, 'C')) {
		t.Error("undo of paste")
	}
	d.undoStep()
	if !bytes.Equal(a.font().bits, before) {
		t.Error("undo of invert")
	}
	d.redoStep()
	if a.font().row('B', 3) != ^vga.row('B', 3) {
		t.Error("redo")
	}
	// Keys: H mirrors the brush character, Ctrl+Z takes it back.
	a.brush = 'L'
	d.key("H", false, false)
	if a.font().row('L', 5) != bitsReverse(vga.row('L', 5)) {
		t.Error("H key")
	}
	d.key("Z", true, false)
	if a.font().row('L', 5) != vga.row('L', 5) {
		t.Error("Ctrl+Z")
	}
	// Saved as a file, it reads back the same.
	p := filepath.Join(t.TempDir(), "edited.psf")
	data, err := fontFileBytes(a.font(), nil, filepath.Ext(p))
	if err != nil {
		t.Fatal(err)
	}
	g2, _, err := parseFontFile(p, data, "pc", nil)
	if err != nil || !bytes.Equal(g2.native(), a.font().native()) {
		t.Errorf("saved font: %v", err)
	}
}

func bitsReverse(v byte) byte {
	var o byte
	for i := 0; i < 8; i++ {
		if v&(1<<i) != 0 {
			o |= 0x80 >> i
		}
	}
	return o
}

func TestLooksLikeFont(t *testing.T) {
	for _, c := range []struct {
		name string
		n    int
		want bool
	}{{"a.f16", 4096, true}, {"a.psf", 10, true}, {"a.64c", 2050, true}, {"a.bin", 4096, true}, {"a.bin", 4000, false},
		{"a.ans", 4096, false}, {"a.ch8", 768, true}} {
		if got := looksLikeFont(c.name, make([]byte, c.n)); got != c.want {
			t.Errorf("%s %d: %v", c.name, c.n, got)
		}
	}
	if !looksLikeFont("noname", []byte{0x72, 0xB5, 0x4A, 0x86, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}) {
		t.Error("PSF2 by its magic")
	}
}

func TestSpectrumScreenWithOwnFont(t *testing.T) {
	a := testApp(t, "zx")
	set := testGlyphs(96, 8)
	p := filepath.Join(t.TempDir(), "odd.ch8")
	os.WriteFile(p, set, 0o644)
	a.loadFontFile(p)
	if a.ownInUse() == nil {
		t.Fatal(a.message)
	}
	a.doc.set(3, 2, a.cell('K'))
	scr := filepath.Join(filepath.Dir(p), "pic.scr")
	if !a.writeOut(scr, "scr") || !strings.Contains(a.message, "a .scr cannot hold the font") {
		t.Fatal(a.message)
	}
	b, _ := os.ReadFile(scr)
	if b[scrAddr(3, 2*8+1)] != set[('K'-32)*8+1] {
		t.Error("the screen is not drawn in the page's font")
	}
	// Opened again, the cells are matched to the kept font: K again.
	a.focusTab(a.newTab("zx"))
	a.openPath(scr, "auto")
	if cellCh(a.doc.get(3, 2)) != 'K' || a.ownInUse() == nil {
		t.Errorf("reopened: %c, %s", cellCh(a.doc.get(3, 2)), a.message)
	}
}

func TestDropAsksForFonts(t *testing.T) {
	a := testApp(t, "pc")
	dir := t.TempDir()
	fp := filepath.Join(dir, "x.f16")
	os.WriteFile(fp, testGlyphs(256, 16), 0o644)
	dropFile(fp)
	ask, ok := a.dialog.(*FontAsk)
	if !ok || ask.err != nil {
		t.Fatalf("dialog %T", a.dialog)
	}
	ask.key("Return", false, false)
	if a.dialog != nil || a.ownInUse() == nil || a.font().name != "x" {
		t.Error("Return did not use the font")
	}
	// A font that does not fit the page says why and can still open as art.
	a.focusTab(a.newTab("atari"))
	dropFile(fp)
	if ask, ok := a.dialog.(*FontAsk); !ok || ask.err == nil {
		t.Errorf("an Atari page and a PC font: %T", a.dialog)
	}
	closeDialog()
	art := filepath.Join(dir, "a.ans")
	os.WriteFile(art, []byte("hello"), 0o644)
	dropFile(art)
	if a.dialog != nil || a.path != art {
		t.Error("art was not opened at once")
	}
}

// FuzzFontFile: no font file, however made, may crash the loader or give a font of the wrong size.
func FuzzFontFile(f *testing.F) {
	f.Add("a.psf", append([]byte{0x36, 0x04, 0, 16}, testGlyphs(256, 16)...))
	f.Add("a.64c", append([]byte{0, 0x30}, testGlyphs(256, 8)...))
	f.Add("a.f14", testGlyphs(256, 14))
	f.Add("a.xb", writeXBin(newDoc(2, 2), palettes[0].colors, fontByID("ega"), 14, false))
	f.Fuzz(func(t *testing.T, name string, b []byte) {
		for _, mode := range []string{"pc", "amiga", "c64", "atari", "zx"} {
			up, low, err := parseFontFile(name, b, mode, nil)
			if err != nil {
				continue
			}
			if len(up.bits) != 4096 || up.rows() < 1 || up.rows() > 16 || (low != nil && len(low.bits) != 4096) {
				t.Fatalf("%s on %s: %d bytes, %d rows", name, mode, len(up.bits), up.rows())
			}
		}
	})
}
