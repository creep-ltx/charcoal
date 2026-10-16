package main

// Tests of the rest of other editors' files (round 4, area "oth"): IcyDraw's pages of
// other machines (icymach.go), Playscii's other charsets (psciown.go), SkyPix (skypix.go).

import (
	"bufio"
	"image/png"
	"math/rand"
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
