package main

// Tests of the rest of other editors' files (round 4, area "oth"): IcyDraw's pages of
// other machines (icymach.go), Playscii's other charsets (psciown.go), SkyPix (skypix.go).

import (
	"bufio"
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math/bits"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// othFPage is a CharEdit page of a machine for an IcyDraw round trip.
type othFPage struct {
	mode, machine, stres string
	lower                bool
}

var othFPages = []othFPage{
	{"c64", "c64", "", false}, {"c64", "c64", "", true}, {"c64", "c128", "", false}, {"c64", "c128", "", true},
	{"c64", "vic20", "", false}, {"c64", "vic20", "", true}, {"c64", "pet", "", false}, {"c64", "pet", "", true},
	{"c64", "pet80", "", false}, {"c64", "pet80", "", true}, {"c64", "c128vdc", "", false}, {"c64", "c128vdc", "", true},
	{"c64", "plus4", "", false}, {"c64", "plus4", "", true},
	{"atari", "", "", false}, {"st", "", "stlow", false}, {"st", "", "stmed", false}, {"st", "", "sthigh", false},
}

func (p othFPage) String() string {
	return p.mode + "/" + p.machine + p.stres + map[bool]string{true: "/lower"}[p.lower]
}

// othFApp is an app on an empty page of p (stand-in fonts where the ROMs are missing).
func othFApp(t *testing.T, p othFPage) *App {
	t.Helper()
	a := testApp(t, p.mode)
	a.hasC64 = true
	switch p.mode {
	case "c64":
		a.machine = p.machine
		a.paletteFor["c64"] = machineByID(p.machine).palette
		a.setCase(p.lower)
		a.doc = newMachineDoc(a.mach())
	case "st":
		a.stres = p.stres
		a.doc = newSTDoc(a.res())
	case "atari":
		a.doc = newAtariDoc()
	}
	a.setPalette()
	return a
}

// othFRandom fills the page with random cells and screen colours the page type can hold.
func othFRandom(r *rand.Rand, a *App, rows int) {
	d := a.doc
	d.ensureRows(rows)
	d.rows = rows
	switch {
	case a.ted():
		d.bg, d.border = r.Intn(128), r.Intn(128)
	case a.c64() && a.mach().mono:
		d.bg, d.border = 0, 0
	case a.c64() && a.machine == "c128vdc":
		d.bg, d.border = r.Intn(16), -1
	case a.c64():
		d.bg, d.border = r.Intn(16), r.Intn(16)
	case a.atari():
		d.bg, d.ink = r.Intn(128)*2, r.Intn(128)*2
	}
	for i := range d.cells[:d.cols*d.rows] {
		ch := r.Intn(256)
		switch {
		case a.ted():
			d.cells[i] = tedCell(ch, r.Intn(256))
		case a.c64() && a.mach().mono:
			d.cells[i] = makeCell(ch, 1, 0, 0)
		case a.c64():
			d.cells[i] = makeCell(ch, r.Intn(a.mach().colours), 0, 0)
		case a.atari():
			d.cells[i] = makeCell(ch, 7, 0, 0)
		case a.st():
			n := a.res().colours
			d.cells[i] = makeCell(ch, r.Intn(n), r.Intn(n), 0)
		}
	}
}

// Every machine page goes to IcyDraw's file and comes back the same: page type, machine,
// case, colours and every cell; Ctrl+S writes the .icy again.
func TestOthIcyMachineRoundTrip(t *testing.T) {
	withFonts(t, true)
	r := rand.New(rand.NewSource(41))
	dir := t.TempDir()
	for _, p := range othFPages {
		a := othFApp(t, p)
		othFRandom(r, a, a.doc.rows+r.Intn(9))
		want := a.doc.clone()
		path := filepath.Join(dir, strings.ReplaceAll(p.String(), "/", "-")+".icy")
		if !a.writeOut(path, "icy") {
			t.Fatalf("%v: not saved: %s", p, a.message)
		}
		b, _ := os.ReadFile(path)
		doc, err := parseIcyDraw(b)
		if err != nil {
			t.Fatalf("%v: %v", p, err)
		}
		if !icyMachineBuffer(doc.buffer) || doc.screen.machine == 0 {
			t.Fatalf("%v: buffer %d, screen %+v", p, doc.buffer, doc.screen)
		}
		a.focusTab(a.newTab("pc"))
		a.hasC64 = true
		a.loadBytes(b, filepath.Base(path), path, "auto")
		if a.mode != p.mode || a.saveFmt != "icy" || a.srcKind != "icy" {
			t.Fatalf("%v: opened as %s (saves %s, %s): %s", p, a.mode, a.saveFmt, a.srcKind, a.message)
		}
		switch p.mode {
		case "c64":
			if a.machine != p.machine || a.lower() != p.lower {
				t.Fatalf("%v: opened on %s, lower %v", p, a.machine, a.lower())
			}
		case "st":
			if a.stres != p.stres {
				t.Fatalf("%v: opened in %s", p, a.stres)
			}
		}
		got := a.doc
		if got.cols != want.cols || got.rows != want.rows {
			t.Fatalf("%v: %dx%d came back %dx%d", p, want.cols, want.rows, got.cols, got.rows)
		}
		if p.mode == "atari" {
			// PAL shows some hues alike (1 and 15): the same colour may come back as another register.
			g := a.gtia()
			if g[got.bg&0xFE] != g[want.bg&0xFE] || g[atariText(got)&0xFE] != g[atariText(want)&0xFE] {
				t.Fatalf("%v: colours %02x/%02x came back %02x/%02x", p, want.bg, atariText(want), got.bg, atariText(got))
			}
		} else if p.mode == "c64" && (got.bg != want.bg || got.border != want.border) {
			t.Fatalf("%v: screen %d border %d came back %d %d", p, want.bg, want.border, got.bg, got.border)
		}
		for i, v := range want.cells[:want.cols*want.rows] {
			if w := got.cells[i]; w != v {
				t.Fatalf("%v: cell %d,%d %08x came back %08x", p, i%want.cols, i/want.cols, v, w)
			}
		}
		if a.ownInUse() != nil {
			t.Fatalf("%v: the machine's own set came back as the page's own font", p)
		}
		a.doc.set(0, 0, makeCell(1, 1, 0, 0))
		a.dirty = true
		a.save(false)
		if b2, _ := os.ReadFile(path); editorKind(path, b2) != "icy" {
			t.Fatalf("%v: Ctrl+S wrote another format", p)
		}
	}
}

// A PETSCII page with a character set of its own keeps it in the .icy (IcyDraw's font record) and
// opens with it again; so does one in the lower case set.
func TestOthIcyOwnCharset(t *testing.T) {
	withFonts(t, true)
	for _, lower := range []bool{false, true} {
		a := othFApp(t, othFPage{"c64", "c64", "", lower})
		set := make([]byte, 2048)
		for i := range set {
			set[i] = byte(i * 13)
		}
		own := newFont(ownID, "mine", "c64", set, 8, 256)
		if lower {
			a.useOwnFont(fontByID("c64upper").clone(), own, true)
		} else {
			a.useOwnFont(own, nil, true)
		}
		path := filepath.Join(t.TempDir(), "own.icy")
		if !a.writeOut(path, "icy") {
			t.Fatal(a.message)
		}
		b, _ := os.ReadFile(path)
		a.focusTab(a.newTab("pc"))
		a.hasC64 = true
		a.loadBytes(b, "own.icy", path, "auto")
		f := a.ownInUse()
		if f == nil || a.lower() != lower && !strings.HasPrefix(a.fontFor["c64"], ownID) {
			t.Fatalf("lower %v: no own font (%s)", lower, a.fontFor["c64"])
		}
		if !sameGlyphs8(a.font(), set) {
			t.Fatalf("lower %v: the set came back changed", lower)
		}
	}
}

// IcyDraw refuses SCREEN records it does not know; so does CharEdit (IcyDraw's own test vectors).
func TestOthIcyScreenRecord(t *testing.T) {
	s, err := parseIcyScreen([]byte{1, 0, 3, 0, 4, 0, 11, 0, 0, 0, 1, 0, 4, 0, 2, 0, 1, 0})
	if err != nil || s.machine != 2 || s.mode != 1 || !s.hasBg || s.bg != 11 || s.hasBorder {
		t.Fatalf("%+v %v", s, err)
	}
	if s, err := parseIcyScreen([]byte{1, 0}); err != nil || s.machine != 0 {
		t.Fatalf("empty: %+v %v", s, err)
	}
	for _, b := range [][]byte{{}, {1}, {2, 0}, {1, 0, 1}, {1, 0, 2, 0, 4, 0, 0}, {1, 0, 2, 0, 3, 0, 0, 0, 0},
		{1, 0, 1, 0, 4, 0, 255, 255, 0, 0}, {1, 0, 1, 0, 4, 0, 8, 0, 3, 0}, {1, 0, 4, 0, 4, 0, 0, 0, 0, 0},
		{1, 0, 2, 0, 4, 0, 0, 0, 0, 0, 2, 0, 4, 0, 0, 0, 0, 0}} {
		if _, err := parseIcyScreen(b); err == nil {
			t.Errorf("% x taken", b)
		}
	}
}

// othFDump is what IcyDraw's engine reads in a file (ccicy dump: see the knowledge note).
type othFDump struct {
	typ, machine string
	bg, border   string
	w, h         int
	pal          []string
	cells        [][5]string // ch fg bg page attr
}

func othFReadDump(t *testing.T, text string) othFDump {
	t.Helper()
	var d othFDump
	sc := bufio.NewScanner(strings.NewReader(text))
	sc.Buffer(nil, 64<<20)
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 0 {
			continue
		}
		switch f[0] {
		case "ERROR":
			t.Fatalf("IcyDraw: %s", sc.Text())
		case "TYPE":
			d.typ = f[1]
		case "MACHINE":
			d.machine = strings.Join(f[1:], " ")
		case "SCREEN":
			d.bg, d.border = f[1], f[2]
		case "SIZE":
			d.w, _ = strconv.Atoi(f[1])
			d.h, _ = strconv.Atoi(f[2])
		case "PALETTE":
			d.pal = f[1:]
		case "ROW":
			for _, c := range f[1:] {
				p := strings.Split(c, ":")
				d.cells = append(d.cells, [5]string{p[0], p[1], p[2], p[3], p[4]})
			}
		}
	}
	return d
}

// othFNum is a palette number of a dumped colour ("p12"), -1 for others.
func othFNum(c string) int {
	if !strings.HasPrefix(c, "p") {
		return -1
	}
	n, _ := strconv.Atoi(c[1:])
	return n
}

// othFCompare: does the page show what IcyDraw reads (its dump)?
func othFCompare(t *testing.T, what string, m *icyMach, d othFDump) {
	t.Helper()
	page := m.doc
	for y := 0; y < min(page.rows, d.h); y++ {
		for x := 0; x < min(page.cols, d.w); x++ {
			c := d.cells[y*d.w+x]
			v := page.cells[y*page.cols+x]
			ch, _ := strconv.ParseInt(c[0], 16, 32)
			fg, bg := othFNum(c[1]), othFNum(c[2])
			attr, _ := strconv.ParseInt(c[4], 16, 32)
			wantCh, ok := int(ch), true
			switch m.mode {
			case "c64":
				switch m.machine {
				case "plus4":
					at := fg
					if attr&icyBlinkAttr != 0 {
						at |= 128
					}
					ok = tedAttr(v) == at
				case "c128vdc":
					ok = vdcOfC64[cellFg(v)] == fg
				case "pet", "pet80":
				default:
					ok = cellFg(v) == fg
				}
			case "atari":
				if bg > 0 {
					wantCh |= 128
				}
			case "st":
				ok = cellFg(v) == fg && cellBg(v) == bg
			}
			if cellCh(v) != wantCh || !ok {
				t.Fatalf("%s: cell %d,%d %08x, IcyDraw has %v", what, x, y, v, c)
			}
		}
	}
}

// Files IcyDraw's engine wrote (ccicy make; not in testdata: their font records are the
// machines' ROMs) open on the machine's page as IcyDraw shows them (ccicy dump next to each).
func TestOthIcyDrawMachineFiles(t *testing.T) {
	dir := os.Getenv("CHARCOAL_ICYFILES")
	if dir == "" {
		h, _ := os.UserHomeDir()
		dir = filepath.Join(h, ".cache/charcoal-others/icyfiles")
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*.icy"))
	if len(files) == 0 {
		t.Skip("no IcyDraw files in", dir)
	}
	loadC64Fonts() // the real ROMs where the app keeps them: IcyDraw's sets must be found to be them
	machineOf := map[string]string{"C64": "c64", "C128": "c128", "Vic20": "vic20", "Pet": "pet", "C16": "plus4", "Pet80": "pet80", "C128Vdc": "c128vdc"}
	for _, f := range files {
		b, _ := os.ReadFile(f)
		text, err := os.ReadFile(strings.TrimSuffix(f, ".icy") + ".dump")
		if err != nil {
			t.Fatalf("%s: no dump", f)
		}
		dump := othFReadDump(t, string(text))
		doc, err := parseIcyDraw(b)
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		m := icyMachinePage(doc, &gtiaPAL)
		if m == nil {
			t.Fatalf("%s: not a machine's page", f)
		}
		name := filepath.Base(f)
		switch dump.typ {
		case "Petscii":
			p := strings.TrimSuffix(strings.TrimPrefix(dump.machine, "Some(Petscii { machine: "), " })")
			parts := strings.Split(p, ", charset: ")
			if m.mode != "c64" || m.machine != machineOf[parts[0]] || m.lower != (parts[1] == "Lower") && m.machine != "c128vdc" {
				t.Fatalf("%s: opened as %s %s lower %v; IcyDraw: %s", name, m.mode, m.machine, m.lower, dump.machine)
			}
			bg, _ := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(dump.bg, "Some("), ")"))
			want := bg
			if m.machine == "c128vdc" {
				want = c64OfVDC(bg)
			}
			if m.doc.bg != want {
				t.Fatalf("%s: screen colour %d, IcyDraw %s", name, m.doc.bg, dump.bg)
			}
			if m.font != nil && hasROM(machineByID(m.machine).family) {
				t.Errorf("%s: IcyDraw's set taken as the page's own (it is the ROM's)", name)
			}
		case "Atascii":
			if m.mode != "atari" {
				t.Fatalf("%s: opened as %s", name, m.mode)
			}
		case "AtariSt":
			res := map[string]string{"Low": "stlow", "Medium": "stmed", "High": "sthigh"}
			r := strings.TrimSuffix(strings.TrimPrefix(dump.machine, "Some(AtariSt { resolution: "), " })")
			if m.mode != "st" || m.stres != res[r] {
				t.Fatalf("%s: opened as %s %s; IcyDraw %s", name, m.mode, m.stres, dump.machine)
			}
		}
		if m.doc.rows != dump.h {
			t.Fatalf("%s: %d rows, IcyDraw %d", name, m.doc.rows, dump.h)
		}
		othFCompare(t, name, m, dump)
	}
}

// IcyDraw's engine reads CharEdit's machine pages as that machine's page with the same cells:
//
//	CHARCOAL_ICYMACH="$HOME/.cache/charcoal-others/sandbox.sh target/release/examples/ccicy dump" \
//	CHARCOAL_ICYDIR=$HOME/.cache/charcoal-others/out go test -run OthIcyExternal .
func TestOthIcyExternal(t *testing.T) {
	cmd, dir := os.Getenv("CHARCOAL_ICYMACH"), os.Getenv("CHARCOAL_ICYDIR")
	if cmd == "" || dir == "" {
		t.Skip("CHARCOAL_ICYMACH / CHARCOAL_ICYDIR not set")
	}
	os.MkdirAll(dir, 0o755)
	withFonts(t, true)
	r := rand.New(rand.NewSource(43))
	for _, p := range othFPages {
		a := othFApp(t, p)
		othFRandom(r, a, a.doc.rows+3)
		path := filepath.Join(dir, "cc-"+strings.ReplaceAll(p.String(), "/", "-")+".icy")
		if !a.writeOut(path, "icy") {
			t.Fatal(a.message)
		}
		args := append(strings.Fields(cmd), path)
		out, err := exec.Command(args[0], args[1:]...).Output()
		if err != nil {
			t.Fatalf("%v: %v", p, err)
		}
		dump := othFReadDump(t, string(out))
		b, _ := os.ReadFile(path)
		doc, _ := parseIcyDraw(b)
		m := icyMachinePage(doc, a.gtia())
		if dump.w != m.doc.cols || dump.h != m.doc.rows {
			t.Fatalf("%v: IcyDraw reads %dx%d", p, dump.w, dump.h)
		}
		want := map[string]string{"c64": "Petscii", "atari": "Atascii", "st": "AtariSt"}[p.mode]
		if dump.typ != want {
			t.Fatalf("%v: IcyDraw reads a %s page", p, dump.typ)
		}
		othFCompare(t, p.String(), m, dump)
		for i, v := range a.doc.cells[:a.doc.cols*a.doc.rows] {
			if p.mode != "atari" && m.doc.cells[i] != v || p.mode == "atari" && cellCh(m.doc.cells[i]) != cellCh(v) {
				t.Fatalf("%v: cell %d", p, i)
			}
		}
		t.Logf("%v: IcyDraw reads %s %s, screen %s border %s", p, dump.typ, dump.machine, dump.bg, dump.border)
	}
}

// IcyDraw's engine draws CharEdit's machine pages as CharEdit does, pixel for pixel (the real
// character sets; machines whose ROM or font is not here are skipped):
//
//	CHARCOAL_ICYRENDER="$HOME/.cache/charcoal-others/sandbox.sh target/release/examples/ccicy render" \
//	CHARCOAL_ICYDIR=$HOME/.cache/charcoal-others/out go test -run OthIcyRender .
func TestOthIcyRender(t *testing.T) {
	cmd, dir := os.Getenv("CHARCOAL_ICYRENDER"), os.Getenv("CHARCOAL_ICYDIR")
	if cmd == "" || dir == "" {
		t.Skip("CHARCOAL_ICYRENDER / CHARCOAL_ICYDIR not set")
	}
	os.MkdirAll(dir, 0o755)
	loadC64Fonts()
	old := fonts
	t.Cleanup(func() { fonts = old })
	stOK := loadSTFonts()
	r := rand.New(rand.NewSource(44))
	checked := 0
	for _, p := range othFPages {
		if p.mode == "c64" && !hasROM(machineByID(p.machine).family) || p.mode == "st" && !stOK {
			t.Logf("%v: no font here, skipped", p)
			continue
		}
		a := othFApp(t, p)
		othFRandom(r, a, 25)
		if a.ted() { // no flashing: IcyDraw draws the off phase
			for i, v := range a.doc.cells {
				a.doc.cells[i] = tedCell(cellCh(v), tedAttr(v)&127)
			}
		}
		a.blink = false
		path := filepath.Join(dir, "rd-"+strings.ReplaceAll(p.String(), "/", "-")+".icy")
		if !a.writeOut(path, "icy") {
			t.Fatal(a.message)
		}
		out := path + ".png"
		args := append(strings.Fields(cmd), path, out)
		if msg, err := exec.Command(args[0], args[1:]...).CombinedOutput(); err != nil {
			t.Fatalf("%v: %v %s", p, err, msg)
		}
		f, err := os.Open(out)
		if err != nil {
			t.Fatal(err)
		}
		img, err := png.Decode(f)
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
		fb := a.renderDoc(a.doc, 1)
		W, H := a.doc.cols*a.cellW(), 25*a.cellH()
		bx, top, _ := a.borderSize(W, H, 1)
		ib := img.Bounds()
		sy := ib.Dy() / H // IcyDraw draws 80-column PETs and the VDC with rows twice
		if ib.Dx() != W || sy < 1 || ib.Dy() != H*sy {
			t.Fatalf("%v: IcyDraw draws %dx%d, CharEdit %dx%d", p, ib.Dx(), ib.Dy(), W, H)
		}
		bad := 0
		for y := 0; y < H; y++ {
			for x := 0; x < W; x++ {
				r8, g8, b8, _ := img.At(x, y*sy).RGBA()
				icy := r8>>8<<16 | g8>>8<<8 | b8>>8
				if c := fb.px[(top+y)*fb.w+bx+x] & 0xFFFFFF; c != icy {
					if bad++; bad == 1 {
						t.Errorf("%v: pixel %d,%d CharEdit %06x, IcyDraw %06x", p, x, y, c, icy)
					}
				}
			}
		}
		if bad > 0 {
			t.Errorf("%v: %d pixels differ", p, bad)
		}
		checked++
	}
	t.Logf("%d pages drawn alike", checked)
}

// ---------------------------------------------------------------- Playscii's own charsets and palettes

// othFPNG is a PNG of the picture.
func othFPNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// othFFakeSet registers a made-up Playscii charset (16 x 4 glyphs of 7 x 8, glyph i's rows
// are i, i+1, ...) and palette (20 colours) and puts their pictures where fetched ones go.
func othFFakeSet(t *testing.T) (glyphs []byte, colours []uint32) {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 16*7, 4*8))
	for i := 0; i < 64; i++ {
		for r := 0; r < 8; r++ {
			v := byte(i+r) << 1 // 7 pixels wide: bit 0 never set
			glyphs = append(glyphs, v)
			for b := 0; b < 7; b++ {
				c := color.NRGBA{0, 0, 0, 255}
				if v&(0x80>>b) != 0 {
					c = color.NRGBA{200, 100, 50, 255}
				}
				img.Set(i%16*7+b, i/16*8+r, c)
			}
		}
	}
	set := othFPNG(t, img)
	pimg := image.NewNRGBA(image.Rect(0, 0, 20, 2))
	for x := 0; x < 20; x++ {
		c := uint32(x*12+3)<<16 | uint32(255-x*10)<<8 | uint32(x*7)
		colours = append(colours, c)
		for y := 0; y < 2; y++ {
			pimg.Set(x, y, color.NRGBA{uint8(c >> 16), uint8(c >> 8), uint8(c), 255})
		}
	}
	pal := othFPNG(t, pimg)
	sum := func(b []byte) string { s := sha1.Sum(b); return hex.EncodeToString(s[:]) }
	psciSets["zzfake"] = psciSet{"Fake", 16, 4, sum(set)}
	psciPaletteFiles["zzpal"] = sum(pal)
	t.Cleanup(func() { delete(psciSets, "zzfake"); delete(psciPaletteFiles, "zzpal") })
	for rel, b := range map[string][]byte{"charsets/zzfake.png": set, "palettes/zzpal.png": pal} {
		os.MkdirAll(filepath.Dir(psciFilePath(rel)), 0o755)
		if err := os.WriteFile(psciFilePath(rel), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return glyphs, colours
}

// othFPsci is a Playscii file of one frame and layer: tiles as (char, fg, bg).
func othFPsci(charset, palette string, w int, tiles [][3]int) []byte {
	var ts []string
	for _, tl := range tiles {
		ts = append(ts, fmt.Sprintf(`{"char":%d,"fg":%d,"bg":%d,"xform":0}`, tl[0], tl[1], tl[2]))
	}
	return []byte(fmt.Sprintf(`{"active_frame":0,"active_layer":0,"camera":[0,0,0],"charset":%q,"palette":%q,"width":%d,"height":%d,`+
		`"frames":[{"delay":0.1,"layers":[{"z":0,"visible":1,"name":"l","tiles":[%s]}]}]}`, charset, palette, w, len(tiles)/w, strings.Join(ts, ",")))
}

// A Playscii file in one of Playscii's sets CharEdit has no page for opens on a PC page
// with the set as its font and the palette as its colours (the 16 the art uses most of
// a bigger one); Ctrl+S writes the same charset, palette, glyphs and colours.
func TestOthPsciOwnSet(t *testing.T) {
	a := testApp(t, "pc")
	glyphs, colours := othFFakeSet(t)
	r := rand.New(rand.NewSource(45))
	var tiles [][3]int
	for i := 0; i < 8*5; i++ { // colours 1-12 much, 13-20 once each at most
		fg, bg := 1+r.Intn(12), 1+r.Intn(12)
		if i < 8 {
			bg = 13 + i
		}
		tiles = append(tiles, [3]int{r.Intn(64), fg, bg})
	}
	src := othFPsci("zzfake", "zzpal", 8, tiles)
	path := filepath.Join(t.TempDir(), "own.psci")
	os.WriteFile(path, src, 0o644)
	a.loadBytes(src, "own.psci", path, "auto")
	if a.mode != "pc" || a.srcKind != "psci" {
		t.Fatalf("opened as %s (%s): %s", a.mode, a.srcKind, a.message)
	}
	f := a.ownInUse()
	if f == nil || f.rows() != 8 {
		t.Fatalf("no own font: %v", f)
	}
	for c := 0; c < 64; c++ {
		for row := 0; row < 8; row++ {
			if f.row(c, row) != glyphs[c*8+row] {
				t.Fatalf("glyph %d row %d: %02x, want %02x", c, row, f.row(c, row), glyphs[c*8+row])
			}
		}
	}
	if a.ownPal == nil || a.paletteFor["pc"] != "file" {
		t.Fatal("no own colours")
	}
	a.setPalette()
	for i, tl := range tiles {
		v := a.doc.cells[i]
		if cellCh(v) != tl[0] || a.pal[cellFg(v)]&0xFFFFFF != colours[tl[1]-1] {
			t.Fatalf("tile %d %v opened as %08x (fg %06x)", i, tl, v, a.pal[cellFg(v)])
		}
	}
	a.doc.set(0, 0, makeCell(5, cellFg(a.doc.cells[0]), cellBg(a.doc.cells[0]), 0))
	a.dirty = true
	a.save(false)
	b, _ := os.ReadFile(path)
	var back psciFile
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if back.Charset != "zzfake" || back.Palette != "zzpal" {
		t.Fatalf("saved as %s / %s", back.Charset, back.Palette)
	}
	for i, tl := range back.Frames[0].Layers[0].Tiles {
		want := tiles[i]
		if i == 0 {
			want[0] = 5
		}
		if int(tl.Char) != want[0] || int(tl.Fg) != want[1] || want[2] <= 12 && int(tl.Bg) != want[2] {
			t.Fatalf("tile %d saved as %+v, was %v", i, tl, want)
		}
		if want[2] > 12 && colours[int(tl.Bg)-1] != a.pal[cellBg(a.doc.cells[i])]&0xFFFFFF {
			t.Fatalf("tile %d: background saved as %d", i, int(tl.Bg))
		}
	}
}

// A set that is not here yet is asked for, fetched (checked by SHA-1) and then the file opens with it.
func TestOthPsciFetch(t *testing.T) {
	a := testApp(t, "pc")
	othFFakeSet(t)
	set, _ := os.ReadFile(psciFilePath("charsets/zzfake.png"))
	pal, _ := os.ReadFile(psciFilePath("palettes/zzpal.png"))
	os.RemoveAll(filepath.Join(appDataDir(), "playscii"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/charsets/zzfake.png":
			w.Write(set)
		case "/palettes/zzpal.png":
			w.Write(pal)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	old := psciURL
	psciURL = func(rel string) string { return srv.URL + "/" + rel }
	t.Cleanup(func() { psciURL = old })
	src := othFPsci("zzfake", "zzpal", 2, [][3]int{{1, 2, 3}, {4, 5, 6}})
	a.loadBytes(src, "f.psci", "", "auto")
	req, ok := a.dialog.(*FontReq)
	if !ok {
		t.Fatalf("no question asked: %T %s", a.dialog, a.message)
	}
	if !strings.Contains(req.why, `"zzfake"`) || !strings.Contains(req.why, `"zzpal"`) {
		t.Fatalf("the question: %s", req.why)
	}
	if err := req.fetch(); err != nil {
		t.Fatal(err)
	}
	if _, err := req.load(); err != nil {
		t.Fatal(err)
	}
	a.dialog = nil
	req.after()
	if a.mode != "pc" || a.ownInUse() == nil || a.ownPal == nil {
		t.Fatalf("not opened with the set: %s", a.message)
	}
	// A wrong file from the server is refused.
	os.RemoveAll(filepath.Join(appDataDir(), "playscii"))
	set[len(set)-5] ^= 1
	if err := fetchPsci([]string{"charsets/zzfake.png"}); err == nil {
		t.Fatal("a changed picture was taken")
	}
}

// Playscii's real pictures (CHARCOAL_PLAYSCII: a Playscii checkout, by default the one in
// ~/.cache/charcoal-others): every set CharEdit knows gives a font whose letters are drawn,
// every palette its colours; the SHA-1s are the files'.
func TestOthPsciRealPictures(t *testing.T) {
	dir := os.Getenv("CHARCOAL_PLAYSCII")
	if dir == "" {
		h, _ := os.UserHomeDir()
		dir = filepath.Join(h, ".cache/charcoal-others/playscii")
	}
	if _, err := os.Stat(filepath.Join(dir, "charsets")); err != nil {
		t.Skip("no Playscii checkout in", dir)
	}
	testApp(t, "pc")
	for rel := range psciRelAll() {
		b, err := os.ReadFile(filepath.Join(dir, rel))
		if err != nil {
			t.Fatalf("%s: %v", rel, err)
		}
		if !sha1Is(b, psciSum(rel)) {
			t.Fatalf("%s: SHA-1 is not the table's", rel)
		}
		os.MkdirAll(filepath.Dir(psciFilePath(rel)), 0o755)
		os.WriteFile(psciFilePath(rel), b, 0o644)
	}
	for name, s := range psciSets {
		f := psciCharsetFont(name)
		if f == nil {
			t.Fatalf("%s: no font", name)
		}
		// The letter A: the place .char gives it.
		chars, _ := os.ReadFile(filepath.Join(dir, "charsets", name+".char"))
		at := othFCharIndex(string(chars), 'A')
		if at < 0 {
			t.Fatalf("%s: no A in its map", name)
		}
		set := 0
		for r := 0; r < f.rows(); r++ {
			set += bits.OnesCount8(f.row(at, r))
		}
		if set < 6 {
			t.Fatalf("%s: A (glyph %d of %dx%d) has %d pixels", name, at, s.cols, s.rows, set)
		}
	}
	for name := range psciPaletteFiles {
		if p := psciPaletteColours(name); len(p) < 2 {
			t.Fatalf("palette %s: %d colours", name, len(p))
		}
	}
	if p := psciPaletteColours("speccy"); len(p) != 15 {
		t.Fatalf("speccy: %d colours (Playscii's log says 15)", len(p))
	}
	// A picture in the ZX81's set: opened with it, saved back with it.
	a := testApp(t, "pc")
	for rel := range psciRelAll() {
		b, _ := os.ReadFile(filepath.Join(dir, rel))
		os.MkdirAll(filepath.Dir(psciFilePath(rel)), 0o755)
		os.WriteFile(psciFilePath(rel), b, 0o644)
	}
	src := othFPsci("speccy", "speccy", 3, [][3]int{{38, 2, 1}, {39, 3, 4}, {40, 15, 5}})
	a.loadBytes(src, "zx81.psci", "", "auto")
	if a.ownInUse() == nil || cellCh(a.doc.cells[0]) != 38 {
		t.Fatalf("zx81: %s", a.message)
	}
	// Playscii's own art: all of it opens; new.psci (dos, ansi-240) with the 16 colours it uses.
	files, _ := filepath.Glob(filepath.Join(dir, "*", "*.psci"))
	more, _ := filepath.Glob(filepath.Join(dir, "games", "*", "*", "*.psci"))
	for _, f := range append(files, more...) {
		b, _ := os.ReadFile(f)
		art, err := parsePsci(b)
		if err != nil || len(art.need) > 0 {
			t.Fatalf("%s: %v %v", f, err, art.need)
		}
		if filepath.Base(f) == "new.psci" && (art.own == nil || art.own.pal == nil || art.own.palette != "ansi-240") {
			t.Fatalf("new.psci: %+v", art.own)
		}
	}
	t.Logf("%d of Playscii's files open", len(files)+len(more))
}

// othFCharIndex is the place of a character in a Playscii .char map (charset.py).
func othFCharIndex(text string, want rune) int {
	var lines []string
	for _, l := range strings.Split(text, "\n") {
		if !strings.HasPrefix(l, "//") {
			lines = append(lines, l)
		}
	}
	i := 0
	for _, l := range lines[2:] {
		for _, c := range strings.TrimRight(l, "\r") {
			if c == want {
				return i
			}
			i++
		}
	}
	return -1
}

// Playscii's ST font opens on an ST page: low for 40 columns, high for two colours, medium else.
func TestOthPsciAtariST(t *testing.T) {
	for _, c := range []struct {
		w     int
		cols  [][2]int
		stres string
	}{{40, [][2]int{{2, 1}}, "stlow"}, {80, [][2]int{{2, 1}, {1, 2}}, "sthigh"}, {80, [][2]int{{2, 1}, {3, 4}}, "stmed"}} {
		var tiles [][3]int
		for i := 0; i < c.w*2; i++ {
			fb := c.cols[i%len(c.cols)]
			tiles = append(tiles, [3]int{65 + i%26, fb[0], fb[1]})
		}
		art, err := parsePsci(othFPsci("atari_st", "ansi", c.w, tiles))
		if err != nil || art.mode != "st" || art.stres != c.stres {
			t.Fatalf("%d columns: %v %s %s", c.w, err, art.mode, art.stres)
		}
		d := art.docs[0]
		if d.cols != stResByID(c.stres).cols || cellCh(d.cells[1]) != 66 {
			t.Fatalf("%s: %d columns, %08x", c.stres, d.cols, d.cells[1])
		}
		pens := paletteByID(stResByID(c.stres).palette).colors
		// ANSI red (2) on black (1): the ST's nearest pens.
		if c.stres == "stlow" && (pens[cellFg(d.cells[0])]&0xFFFFFF != 0xFF0000 || pens[cellBg(d.cells[0])]&0xFFFFFF != 0) {
			t.Fatalf("pens %d on %d", cellFg(d.cells[0]), cellBg(d.cells[0]))
		}
	}
}

// An ST page from Playscii saves back in the ST's font and its file's palette, and reads back the same.
func TestOthPsciAtariSTSave(t *testing.T) {
	a := testApp(t, "pc")
	withFonts(t, true)
	var tiles [][3]int
	for i := 0; i < 40*4; i++ {
		tiles = append(tiles, [3]int{i & 255, 1 + i%16, 1 + i/16%16})
	}
	src := othFPsci("atari_st", "ansi", 40, tiles)
	path := filepath.Join(t.TempDir(), "st.psci")
	os.WriteFile(path, src, 0o644)
	a.loadBytes(src, "st.psci", path, "auto")
	if a.mode != "st" || a.stres != "stlow" {
		t.Fatalf("opened as %s %s: %s", a.mode, a.stres, a.message)
	}
	want := a.doc.clone()
	a.dirty = true
	a.save(false)
	b, _ := os.ReadFile(path)
	art, err := parsePsci(b)
	if err != nil || art.mode != "st" || !strings.Contains(string(b), `"atari_st"`) || !strings.Contains(string(b), `"ansi"`) {
		t.Fatalf("saved: %v %s", err, b[:min(len(b), 200)])
	}
	for i, v := range want.cells[:want.cols*want.rows] {
		if w := art.docs[0].cells[i]; w != v {
			t.Fatalf("cell %d %08x came back %08x", i, v, w)
		}
	}
}

// ---------------------------------------------------------------- PabloDraw, TheDraw, BSAVE

// A PC page saved as PabloDraw's .pd (a font of its own, other widths through SAUCE) and as
// a BSAVE screen opens again the same; Ctrl+S writes the format again.
func TestOthPDAndBSV(t *testing.T) {
	r := rand.New(rand.NewSource(46))
	dir := t.TempDir()
	for _, c := range []struct {
		fmtID   string
		cols    int
		ownFont bool
	}{{"pd", 80, false}, {"pd", 132, true}, {"pd", 40, false}, {"bsv", 80, false}} {
		a := testApp(t, "pc")
		a.ice = true
		d := newDoc(c.cols, 30)
		for i := range d.cells {
			ch := 7 + r.Intn(249)
			if i%7 == 0 {
				ch = 32
			}
			d.cells[i] = makeCell(ch, r.Intn(16), r.Intn(16), 0)
		}
		a.doc = d
		if c.ownFont {
			f := fontByID("vga").clone()
			f.id, f.name = ownID, "mine"
			f.setRow(65, 3, 0x81)
			a.useOwnFont(f, nil, true)
		}
		a.setPalette()
		p := filepath.Join(dir, fmt.Sprintf("page%d.%s", c.cols, c.fmtID))
		if !a.writeOut(p, c.fmtID) {
			t.Fatalf("%s: not saved: %s", c.fmtID, a.message)
		}
		want, pal := a.doc.clone(), a.pal
		b, _ := os.ReadFile(p)
		a.focusTab(a.newTab("pc"))
		a.loadBytes(b, filepath.Base(p), p, "auto")
		if a.mode != "pc" || a.srcKind != c.fmtID || a.saveFmt != c.fmtID {
			t.Fatalf("%s: opened as %s (%s, saves %s): %s", c.fmtID, a.mode, a.srcKind, a.saveFmt, a.message)
		}
		a.setPalette()
		if a.doc.cols != want.cols {
			t.Fatalf("%s: %d columns came back %d", c.fmtID, want.cols, a.doc.cols)
		}
		for i, v := range want.cells {
			w := a.doc.cells[i]
			if cellCh(w) != cellCh(v) || cellCh(v) != 32 && a.pal[cellFg(w)] != pal[cellFg(v)] || a.pal[cellBg(w)] != pal[cellBg(v)] {
				t.Fatalf("%s %d: cell %d %08x came back %08x", c.fmtID, c.cols, i, v, w)
			}
		}
		if c.ownFont && (a.ownInUse() == nil || a.font().row(65, 3) != 0x81) {
			t.Fatalf("pd: the own font did not come back")
		}
		a.doc.set(0, 0, makeCell('Z', 1, 2, 0))
		a.dirty = true
		a.save(false)
		if b2, _ := os.ReadFile(p); editorKind(p, b2) != c.fmtID {
			t.Fatalf("%s: Ctrl+S wrote another format", c.fmtID)
		}
	}
}

// PabloDraw's .NET strings: lengths in 7-bit groups.
func TestOthNetString(t *testing.T) {
	for _, s := range []string{"", "CP 437 8x16", strings.Repeat("x", 300)} {
		b := appendNetString(nil, s)
		got, rest, err := netString(append(b, 9))
		if err != nil || got != s || len(rest) != 1 {
			t.Fatalf("%d: %q %v", len(s), got, err)
		}
	}
	if _, _, err := netString([]byte{0x85}); err == nil {
		t.Fatal("cut short taken")
	}
}

// TheDraw's crunched pages: colours, next line, runs, blink (UNCRUNCH.PAS's codes).
func TestOthUncrunch(t *testing.T) {
	d, ok := uncrunchTD([]byte{0x0E, 0x11, 'A', 25, 2, 26, 1, 0xDB, 24, 27, 0x04, 'B', 26, 0, 5}, 10, 3)
	if !ok {
		t.Fatal("not read")
	}
	want := []struct {
		x, y int
		ch   int
		attr byte
	}{{0, 0, 'A', 0x1E}, {1, 0, ' ', 0x1E}, {3, 0, ' ', 0x1E}, {4, 0, 0xDB, 0x1E}, {5, 0, 0xDB, 0x1E}, {6, 0, ' ', 0x07},
		{0, 1, 'B', 0x94}, {1, 1, 5, 0x94}}
	for _, w := range want {
		v := d.get(w.x, w.y)
		if cellCh(v) != w.ch || cellAttr(v) != w.attr {
			t.Fatalf("%d,%d: %02x %02x, want %02x %02x", w.x, w.y, cellCh(v), cellAttr(v), w.ch, w.attr)
		}
	}
	if _, ok := uncrunchTD([]byte{'A', 28}, 10, 3); ok {
		t.Fatal("code 28 taken")
	}
}

// TheDraw 4.63's own example files (CHARCOAL_THEDRAW: a folder with DEMO.TD and SHADING.TD,
// by default ~/.cache/charcoal-others/thedraw/td463): the page layers open as frames.
func TestOthTheDrawFiles(t *testing.T) {
	dir := os.Getenv("CHARCOAL_THEDRAW")
	if dir == "" {
		h, _ := os.UserHomeDir()
		dir = filepath.Join(h, ".cache/charcoal-others/thedraw/td463")
	}
	b, err := os.ReadFile(filepath.Join(dir, "DEMO.TD"))
	if err != nil {
		t.Skip("no TheDraw files in", dir)
	}
	pages, err := parseTD(b)
	if err != nil || len(pages) != 2 || pages[0].cols != 80 || pages[0].rows != 25 {
		t.Fatalf("DEMO.TD: %v, %d pages", err, len(pages))
	}
	line := func(d *Doc, y int) string {
		var s []byte
		for x := 0; x < d.cols; x++ {
			s = append(s, byte(cellCh(d.get(x, y))))
		}
		return strings.TrimSpace(strings.ReplaceAll(string(s), "\xff", " ")) // TheDraw's blank 255 between words
	}
	if got := line(pages[0], 0); got != "Here's something fun to try out (and show off)." {
		t.Fatalf("DEMO.TD page 1 line 1: %q", got)
	}
	if got := line(pages[1], 4); got != "Welcome to page layer 2!" {
		t.Fatalf("DEMO.TD page 2 line 5: %q", got)
	}
	b, _ = os.ReadFile(filepath.Join(dir, "SHADING.TD"))
	if pages, err := parseTD(b); err != nil || len(pages) != 3 { // "Three pages are occupied" (THEDRAW.DOC)
		t.Fatalf("SHADING.TD: %v, %d pages", err, len(pages))
	}
	a := testApp(t, "pc")
	a.loadBytes(b, "SHADING.TD", filepath.Join(dir, "SHADING.TD"), "auto")
	if a.mode != "pc" || a.nFrames() != 3 || a.path != "" || a.saveFmt != "ansi" {
		t.Fatalf("opened as %s, %d frames, path %q, saves %s", a.mode, a.nFrames(), a.path, a.saveFmt)
	}
}
