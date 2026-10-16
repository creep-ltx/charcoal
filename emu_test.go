package main

// The machine files CharEdit writes, run in real emulators: C64 .prg/.d64 in
// VICE (x64sc), Atari .xex/.atr in atari800's core, Spectrum .tap in Fuse,
// Amstrad CPC .dsk in MAME. Each emulator's picture of the screen is compared
// with CharEdit's PNG export of the same page: every pixel of the screen must
// show the same colour index (palettes differ between emulators, so a colour
// is matched by being used for the same pixels, one to one, not by its RGB).
//
// Off unless CHARCOAL_EMU=1 (the emulators are not on every computer and take
// a while). How to set one up and what each needs: tools/emucheck/README.md.

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"testing"
	"time"
)

// emuFImg is an emulator's picture: one value a pixel (RGB, or the
// machine's own colour number where the emulator gives that).
type emuFImg struct {
	w, h int
	px   []uint32
	rgb  func(v uint32) uint32 // a value's RGB, for the saved picture and the palette report
}

// emuFCase is one file run in an emulator, and the page it must show.
type emuFCase struct {
	name string
	want *FB // CharEdit's PNG export
	// the screen's area in the PNG (the border around it is the machine's border)
	x0, y0, w, h int
	// the emulator's pixels per PNG pixel, as a fraction (MAME's CPC is not 1:1)
	sxN, sxD, syN, syD int
	ring               bool // also the border just around the screen
	cellW              int  // a cell's size in PNG pixels, for the report
	cellH              int
}

// emuFSetup is shared by the tests: where emulators, ROMs and pictures are.
type emuFSetup struct {
	cache string // ~/.cache/charcoal-emu (ROMs, the built a800shot)
	out   string // the pictures: /tmp/charcoal-emu unless CHARCOAL_EMU_OUT
}

func emuFStart(t *testing.T) emuFSetup {
	t.Helper()
	if os.Getenv("CHARCOAL_EMU") != "1" {
		t.Skip("CHARCOAL_EMU=1 runs the files in emulators (tools/emucheck/README.md)")
	}
	s := emuFSetup{cache: os.Getenv("CHARCOAL_EMU_DIR"), out: os.Getenv("CHARCOAL_EMU_OUT")}
	if s.cache == "" {
		home, _ := os.UserHomeDir()
		s.cache = filepath.Join(home, ".cache", "charcoal-emu")
	}
	if s.out == "" {
		s.out = filepath.Join(os.TempDir(), "charcoal-emu")
	}
	if err := os.MkdirAll(s.out, 0o755); err != nil {
		t.Fatal(err)
	}
	return s
}

// emuFNeed skips the test when a program is missing.
func emuFNeed(t *testing.T, prog string) string {
	t.Helper()
	p, err := exec.LookPath(prog)
	if err != nil {
		t.Skipf("%s is not installed (tools/emucheck/README.md)", prog)
	}
	return p
}

// emuFEnv is the environment an emulator runs in: no window system (an
// emulator must never open a window on the user's screen), its own HOME so
// it neither reads nor writes the user's settings.
func emuFEnv(home string, extra ...string) []string {
	var env []string
	for _, e := range os.Environ() {
		k, _, _ := strings.Cut(e, "=")
		switch k {
		case "DISPLAY", "WAYLAND_DISPLAY", "HOME", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME",
			"XDG_RUNTIME_DIR", "SDL_VIDEODRIVER", "SDL_AUDIODRIVER", "GDK_BACKEND", "BROADWAY_DISPLAY":
			continue
		}
		env = append(env, e)
	}
	env = append(env, "HOME="+home, "XDG_CONFIG_HOME="+filepath.Join(home, ".config"),
		"XDG_DATA_HOME="+filepath.Join(home, ".local/share"), "XDG_CACHE_HOME="+filepath.Join(home, ".cache"),
		"SDL_VIDEODRIVER=dummy", "SDL_AUDIODRIVER=dummy")
	return append(env, extra...)
}

// emuFCmd is a command limited in time (secs) and, where systemd is there,
// in memory (4 GB). The command gets exactly env (env -i): systemd-run itself
// needs the session's XDG_RUNTIME_DIR to reach systemd, the emulator may not.
func emuFCmd(secs int, dir string, env []string, prog string, args ...string) *exec.Cmd {
	all := append([]string{"-k", "5", fmt.Sprint(secs), "env", "-i"}, env...) // KILL 5 s after a TERM not obeyed (Fuse)
	all = append(append(all, prog), args...)
	name := "timeout"
	if _, err := exec.LookPath("systemd-run"); err == nil && os.Getenv("CHARCOAL_EMU_NOSCOPE") == "" {
		all = append([]string{"--user", "--scope", "-q", "-p", "MemoryMax=4G", "timeout"}, all...)
		name = "systemd-run"
	}
	c := exec.Command(name, all...)
	c.Dir = dir
	for _, e := range os.Environ() {
		if k, _, _ := strings.Cut(e, "="); k != "DISPLAY" && k != "WAYLAND_DISPLAY" {
			c.Env = append(c.Env, e)
		}
	}
	return c
}

// emuFRun runs a command (see emuFCmd) and gives its output.
func emuFRun(t *testing.T, secs int, dir string, env []string, prog string, args ...string) string {
	t.Helper()
	out, _ := emuFCmd(secs, dir, env, prog, args...).CombinedOutput()
	return string(out)
}

// emuFReadPNG reads a picture as RGB values.
func emuFReadPNG(t *testing.T, p string) emuFImg {
	t.Helper()
	f, err := os.Open(p)
	if err != nil {
		t.Fatalf("the emulator's picture: %v", err)
	}
	defer f.Close()
	m, err := png.Decode(f)
	if err != nil {
		t.Fatalf("%s: %v", p, err)
	}
	b := m.Bounds()
	g := emuFImg{w: b.Dx(), h: b.Dy(), px: make([]uint32, b.Dx()*b.Dy()), rgb: func(v uint32) uint32 { return v }}
	for y := 0; y < g.h; y++ {
		for x := 0; x < g.w; x++ {
			r, gg, bb, _ := m.At(b.Min.X+x, b.Min.Y+y).RGBA()
			g.px[y*g.w+x] = (r>>8)<<16 | (gg>>8)<<8 | bb>>8
		}
	}
	return g
}

// emuFSave writes a picture (the emulator's, for a person to look at).
func emuFSave(t *testing.T, p string, w, h int, rgb func(x, y int) uint32) {
	t.Helper()
	m := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			v := rgb(x, y)
			m.Set(x, y, color.RGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 255})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, m); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

// emuFPair is a colour as CharEdit draws it and as the emulator does.
type emuFPair struct{ ours, theirs uint32 }

// emuFMatch finds where the screen is in the emulator's picture (any offset
// with every pixel matching) and gives the colour pairs; on no match, the
// cells that differ at the best offset.
func emuFMatch(c emuFCase, g emuFImg) (ox, oy int, pairs []emuFPair, bad []string) {
	type pt struct{ wx, wy int }
	var pts []pt
	for y := 0; y < c.h; y++ {
		for x := 0; x < c.w; x++ {
			pts = append(pts, pt{c.x0 + x, c.y0 + y})
		}
	}
	if c.ring { // the border's 4 pixels around the screen
		for y := -4; y < c.h+4; y++ {
			for x := -4; x < c.w+4; x++ {
				if x < 0 || y < 0 || x >= c.w || y >= c.h {
					pts = append(pts, pt{c.x0 + x, c.y0 + y})
				}
			}
		}
	}
	div := func(a, b int) int { // rounded down, also below 0 (the ring)
		if a < 0 {
			return -((-a + b - 1) / b)
		}
		return a / b
	}
	gx := func(ox, wx int) int { return ox + div((wx-c.x0)*c.sxN, c.sxD) }
	gy := func(oy, wy int) int { return oy + div((wy-c.y0)*c.syN, c.syD) }
	// The emulator's pixels must cover the screen (and ring) at the offset.
	minX, maxX := gx(0, c.x0-4), gx(0, c.x0+c.w+4)
	minY, maxY := gy(0, c.y0-4), gy(0, c.y0+c.h+4)
	best, bestN := [2]int{-1, -1}, -1
	try := func(ox, oy int, full bool) (int, map[uint32]uint32) {
		fwd, back := map[uint32]uint32{}, map[uint32]uint32{}
		n := 0
		for _, p := range pts {
			w := c.want.px[p.wy*c.want.w+p.wx]
			v := g.px[gy(oy, p.wy)*g.w+gx(ox, p.wx)]
			f, ok1 := fwd[w]
			b, ok2 := back[v]
			if (ok1 && f != v) || (ok2 && b != w) {
				if !full {
					return n, nil
				}
				continue
			}
			fwd[w], back[v] = v, w
			n++
		}
		return n, fwd
	}
	for oy := -minY; oy+maxY <= g.h; oy++ {
		for ox := -minX; ox+maxX <= g.w; ox++ {
			n, fwd := try(ox, oy, false)
			if fwd != nil {
				for w, v := range fwd {
					pairs = append(pairs, emuFPair{w, v})
				}
				sort.Slice(pairs, func(i, j int) bool { return pairs[i].ours < pairs[j].ours })
				return ox, oy, pairs, nil
			}
			if n > bestN {
				best, bestN = [2]int{ox, oy}, n
			}
		}
	}
	if bestN < 0 {
		return 0, 0, nil, []string{fmt.Sprintf("the emulator's picture (%dx%d) is smaller than the screen", g.w, g.h)}
	}
	// Report the cells that differ at the best offset: take the colour pairs
	// most pixels agree on, then list the cells with other pixels.
	ox, oy = best[0], best[1]
	votes := map[[2]uint32]int{}
	for _, p := range pts {
		votes[[2]uint32{c.want.px[p.wy*c.want.w+p.wx], g.px[gy(oy, p.wy)*g.w+gx(ox, p.wx)]}]++
	}
	fwd := map[uint32]uint32{}
	type vote struct {
		k [2]uint32
		n int
	}
	var vs []vote
	for k, n := range votes {
		vs = append(vs, vote{k, n})
	}
	sort.Slice(vs, func(i, j int) bool { return vs[i].n > vs[j].n })
	used := map[uint32]bool{}
	for _, v := range vs {
		if _, ok := fwd[v.k[0]]; !ok && !used[v.k[1]] {
			fwd[v.k[0]], used[v.k[1]] = v.k[1], true
		}
	}
	seen := map[[2]int]bool{}
	for _, p := range pts {
		w := c.want.px[p.wy*c.want.w+p.wx]
		v := g.px[gy(oy, p.wy)*g.w+gx(ox, p.wx)]
		if fwd[w] == v {
			continue
		}
		cx, cy := (p.wx-c.x0)/c.cellW, (p.wy-c.y0)/c.cellH
		if p.wx < c.x0 || p.wy < c.y0 || p.wx >= c.x0+c.w || p.wy >= c.y0+c.h {
			cx, cy = -1, -1 // the border
		}
		if !seen[[2]int{cx, cy}] {
			seen[[2]int{cx, cy}] = true
			where := fmt.Sprintf("cell %d,%d", cx, cy)
			if cx < 0 {
				where = "the border"
			}
			bad = append(bad, fmt.Sprintf("%s: pixel %d,%d is %06X here, %06X in the emulator (offset %d,%d)",
				where, p.wx-c.x0, p.wy-c.y0, w, g.rgb(v), ox, oy))
		}
	}
	return ox, oy, nil, bad
}

// emuFCheck compares, saves both pictures in the output folder and reports
// the colour pairs.
func emuFCheck(t *testing.T, s emuFSetup, c emuFCase, g emuFImg) []emuFPair {
	t.Helper()
	base := filepath.Join(s.out, c.name)
	emuFSave(t, base+"-emu.png", g.w, g.h, func(x, y int) uint32 { return g.rgb(g.px[y*g.w+x]) })
	emuFSave(t, base+"-charedit.png", c.want.w, c.want.h, func(x, y int) uint32 { return c.want.px[y*c.want.w+x] })
	ox, oy, pairs, bad := emuFMatch(c, g)
	if bad != nil {
		if len(bad) > 12 {
			bad = append(bad[:12], fmt.Sprintf("... %d more", len(bad)-12))
		}
		t.Errorf("%s: the emulator's screen differs from CharEdit's picture (%s-emu.png):\n  %s", c.name, base, strings.Join(bad, "\n  "))
		return nil
	}
	var ps []string
	for _, p := range pairs {
		ps = append(ps, fmt.Sprintf("%06X=%06X", p.ours, g.rgb(p.theirs)))
	}
	t.Logf("%s: the same screen (at %d,%d in the emulator's picture); colours ours=theirs: %s", c.name, ox, oy, strings.Join(ps, " "))
	return pairs
}

// emuFCaseFor is the PNG export of the page and where its screen is.
func emuFCaseFor(a *App, name string, sx, sy [2]int) emuFCase {
	fb := a.renderDoc(a.doc, 1)
	z := a.cpcZoom(1)
	w, h := a.doc.cols*a.cellW()*z, max(a.doc.rows, map[bool]int{true: 25, false: 24}[a.c64()])*a.cellH()*z
	if a.cpc() {
		h = 25 * a.cellH() * z
	}
	bx, top, _ := a.borderSize(w, h, z)
	return emuFCase{name: name, want: fb, x0: bx, y0: top, w: w, h: h, sxN: sx[0], sxD: sx[1], syN: sy[0], syD: sy[1],
		ring: true, cellW: a.cellW() * z, cellH: a.cellH() * z}
}

// emuFText types lines on the page from its top-left corner (a page a person
// can read in the saved pictures), in the current colours.
func emuFText(a *App, lines ...string) {
	for y, l := range lines {
		a.cx, a.cy = 2, 2+y*2
		for _, r := range l {
			a.typeByte(a.charToByte(r))
		}
	}
}

// ---- C64: VICE ----

// emuFVice runs a .prg or .d64 in x64sc (PAL, autostarted, warp) and gives
// its screenshot.
func emuFVice(t *testing.T, s emuFSetup, file string, cycles int) emuFImg {
	t.Helper()
	x64 := emuFNeed(t, "x64sc")
	home := t.TempDir()
	shot := filepath.Join(home, "shot.png")
	out := emuFRun(t, 120, home, emuFEnv(home), x64, "-default", "-console", "-warp", "+sound",
		"-autostartprgmode", "1", "-limitcycles", fmt.Sprint(cycles), "-exitscreenshot", shot, "-autostart", file)
	if _, err := os.Stat(shot); err != nil {
		t.Fatalf("x64sc gave no screenshot:\n%s", out)
	}
	return emuFReadPNG(t, shot)
}

// emuFC64Page is a C64 page of random cells in the given mode, with a
// made-up character set when own.
func emuFC64Page(t *testing.T, r *rand.Rand, mode int, own, lower bool) *App {
	a := c64xFApp(t)
	if lower {
		a.setCase(true)
	}
	if own {
		a.useOwnFont(c64xFFont(r.Int63()), nil, true)
	}
	a.setVICMode(mode)
	for i := 0; i < 3; i++ {
		a.setVICReg(i, r.Intn(16))
	}
	a.doc.bg, a.doc.border = r.Intn(16), r.Intn(16)
	for i := range a.doc.cells {
		a.doc.cells[i] = makeCell(r.Intn(256), r.Intn(16), 0, 0)
	}
	return a
}

func TestEmuC64(t *testing.T) {
	s := emuFStart(t)
	emuFNeed(t, "x64sc")
	r := rand.New(rand.NewSource(64))
	for _, c := range []struct {
		name       string
		mode       int
		own, lower bool
		fmtID      string
	}{
		{"c64-petmate-upper", vicHires, false, false, "prg"},
		{"c64-petmate-lower", vicHires, false, true, "prg"},
		{"c64-own-hires", vicHires, true, false, "prg"},
		{"c64-rom-multicolour", vicMulti, false, false, "prg"},
		{"c64-own-multicolour", vicMulti, true, false, "prg"},
		{"c64-rom-ecm", vicECM, false, false, "prg"},
		{"c64-own-ecm", vicECM, true, false, "prg"},
		{"c64-disk-petmate", vicHires, false, false, "d64"},
		{"c64-disk-own-multicolour", vicMulti, true, false, "d64"},
		{"c64-rom-lower-multicolour", vicMulti, false, true, "prg"},
		{"c64-rom-lower-ecm", vicECM, false, true, "prg"},
		{"c64-disk-petmate-lower", vicHires, false, true, "d64"},
		{"c64-short-page", vicHires, false, false, "prg"},
		{"c64-text", vicHires, false, true, "prg"},
	} {
		t.Run(c.name, func(t *testing.T) {
			a := emuFC64Page(t, r, c.mode, c.own, c.lower)
			switch c.name {
			case "c64-short-page": // 10 rows: the rest of the screen is the screen colour
				a.setRows(10)
			case "c64-text":
				a.doc = newDoc(40, 25)
				a.doc.bg, a.doc.border, a.doc.glyph0 = 6, 14, true
				fillCells(a.doc.cells, makeCell(32, 14, 0, 0))
				a.fg = 1
				emuFText(a, "CharEdit's .prg in VICE (x64sc)", "the same page as CharEdit's PNG,", "pixel for pixel.")
			}
			p := filepath.Join(t.TempDir(), "art."+c.fmtID)
			if !a.writeOut(p, c.fmtID) {
				t.Fatal(a.message)
			}
			cycles := 10000000
			if c.fmtID == "d64" {
				cycles = 60000000 // the 1541 loads at its own pace
			}
			emuFCheck(t, s, emuFCaseFor(a, c.name, [2]int{1, 1}, [2]int{1, 1}), emuFVice(t, s, p, cycles))
		})
	}
}

// ---- Atari 8-bit: atari800's core (a800shot) ----

// emuFA800 runs a file in atari800's core for frames frames and gives the
// screen as GTIA colour values (and atari800's RGB for each).
func emuFA800(t *testing.T, s emuFSetup, file string, frames int, opts ...string) emuFImg {
	t.Helper()
	shot := filepath.Join(s.cache, "bin", "a800shot")
	if _, err := os.Stat(shot); err != nil {
		t.Skip("a800shot is not built: run tools/emucheck/build-a800shot.sh")
	}
	home := t.TempDir()
	raw := filepath.Join(home, "screen.raw")
	args := append([]string{fmt.Sprint(frames), raw}, opts...)
	out := emuFRun(t, 60, home, emuFEnv(home), shot, append(args, file)...)
	b, err := os.ReadFile(raw)
	if err != nil || len(b) != 384*240+768 {
		t.Fatalf("a800shot gave no screen:\n%s", out)
	}
	pal := b[384*240:]
	g := emuFImg{w: 384, h: 240, px: make([]uint32, 384*240)}
	for i := range g.px {
		g.px[i] = uint32(b[i])
	}
	g.rgb = func(v uint32) uint32 {
		return uint32(pal[v*3])<<16 | uint32(pal[v*3+1])<<8 | uint32(pal[v*3+2])
	}
	return g
}

func TestEmuAtari(t *testing.T) {
	s := emuFStart(t)
	r := rand.New(rand.NewSource(800))
	// The Atari 800XL with AltirraOS (built into atari800: no ROM files), PAL.
	machine := []string{"-xl", "-xl-rev", "altirra", "-nobasic", "-pal"}
	for _, c := range []struct {
		name       string
		cols, rows int
		own        bool
		fmtID      string
	}{
		{"atari-xex-40-rom", 40, 24, false, "xex"},
		{"atari-xex-38-rom", 38, 24, false, "xex"},
		{"atari-xex-40-own", 40, 24, true, "xex"},
		{"atari-xex-40-short", 40, 6, false, "xex"},
		{"atari-atr-38-own", 38, 24, true, "atr"},
		{"atari-atr-40-rom", 40, 24, false, "atr"},
		{"atari-800-xex-40-rom", 40, 24, false, "xex"}, // the 400/800 (no international set)
		{"atari-800-atr-38-own", 38, 24, true, "atr"},
		{"atari-text", 40, 24, false, "xex"},
	} {
		t.Run(c.name, func(t *testing.T) {
			a := testApp(t, "atari")
			machFPage(r, a, c.cols, c.rows)
			if c.own {
				font := make([]byte, 1024)
				r.Read(font)
				copy(font, make([]byte, 8)) // an empty space: the screen editor's margin
				f := atariFont(ownID, "mine", font)
				f.h = 8
				a.useOwnFont(f, nil, true)
			}
			if c.name == "atari-text" {
				fillCells(a.doc.cells, makeCell(32, 0, 0, 0))
				a.doc.bg, a.doc.ink, a.doc.border = 0x94, 0xCA, 0
				emuFText(a, "CharEdit's .xex in atari800", "(800XL, AltirraOS, PAL):", "the same page as CharEdit's PNG.")
			}
			p := filepath.Join(t.TempDir(), "art."+c.fmtID)
			if !a.writeOut(p, c.fmtID) {
				t.Fatal(a.message)
			}
			ec := emuFCaseFor(a, c.name, [2]int{1, 1}, [2]int{1, 1})
			// A 38-column page sits in a 40-column screen: the 2 columns on its
			// left are the screen's colour, not the border.
			ec.ring = c.cols == 40
			m := machine
			if strings.HasPrefix(c.name, "atari-800-") { // the 800 with AltirraOS for it
				m = []string{"-atari", "-800-rev", "altirra", "-pal"}
			}
			pairs := emuFCheck(t, s, ec, emuFA800(t, s, p, 400, m...))
			emuFAtariColours(t, a, pairs)
		})
	}
}

// atari800's own colours (its default PAL and NTSC model) are CharEdit's:
// its 128 colours compared one by one. The measured palette files in
// atari800's source (act/), when there, are compared too, for the report.
func TestEmuAtariPalette(t *testing.T) {
	s := emuFStart(t)
	empty := filepath.Join(t.TempDir(), "empty.xex") // no program: only the palette is wanted
	os.WriteFile(empty, []byte{0xFF, 0xFF, 0x00, 0x06, 0x00, 0x06, 0x60}, 0o644)
	for _, tv := range []struct {
		opt  string
		ours *[256]uint32
	}{{"-pal", &gtiaPAL}, {"-ntsc", &gtiaNTSC}} {
		g := emuFA800(t, s, empty, 1, "-xl", "-xl-rev", "altirra", tv.opt)
		worst, at := 0, 0
		for v := 0; v < 256; v += 2 {
			if d := emuFRGBDist(g.rgb(uint32(v)), tv.ours[v]); d > worst {
				worst, at = d, v
			}
		}
		t.Logf("atari800 %s: largest difference from CharEdit's table %d (colour $%02X: %06X, ours %06X)",
			tv.opt, worst, at, g.rgb(uint32(at)), tv.ours[at]&0xFFFFFF)
		if worst > 3 {
			t.Errorf("atari800's %s colours are not CharEdit's", tv.opt)
		}
	}
	acts, _ := filepath.Glob(filepath.Join(s.cache, "src", "atari800", "act", "*.act"))
	for _, f := range acts {
		b, err := os.ReadFile(f)
		if err != nil || len(b) != 768 {
			continue
		}
		var sum, lum, theirsLum int
		for v := 0; v < 256; v += 2 {
			rgb := uint32(b[v*3])<<16 | uint32(b[v*3+1])<<8 | uint32(b[v*3+2])
			sum += emuFRGBDist(rgb, gtiaPAL[v])
			lum += emuFLuma(gtiaPAL[v])
			theirsLum += emuFLuma(rgb)
		}
		t.Logf("%s: mean difference from CharEdit's PAL %d; mean brightness %d, ours %d; $94 is %02X%02X%02X, ours %06X",
			filepath.Base(f), sum/128, theirsLum/128, lum/128, b[0x94*3], b[0x94*3+1], b[0x94*3+2], gtiaPAL[0x94]&0xFFFFFF)
	}
}

// emuFRGBDist is the largest difference of the three channels.
func emuFRGBDist(a, b uint32) int {
	d := 0
	for sh := 0; sh <= 16; sh += 8 {
		x := int(a>>sh&255) - int(b>>sh&255)
		d = max(d, x, -x)
	}
	return d
}

// emuFLuma is a colour's brightness (Rec. 601 weights), 0-255.
func emuFLuma(c uint32) int {
	return (299*int(c>>16&255) + 587*int(c>>8&255) + 114*int(c&255)) / 1000
}

// emuFAtariColours reports how atari800's colours differ from CharEdit's for
// the registers on the page.
func emuFAtariColours(t *testing.T, a *App, pairs []emuFPair) {
	for _, p := range pairs {
		v := p.theirs // atari800's value is the GTIA's colour register value
		if ours := a.atariRGB(int(v)); ours != p.ours {
			t.Errorf("colour $%02X: CharEdit draws %06X but its own table gives %06X", v, p.ours, ours)
		}
	}
}

// ---- ZX Spectrum: Fuse ----

// emuFFMF reads a Fuse movie (FMF, uncompressed) and gives the screen of
// its last whole frame: 320x240, colours 0-15 (bright = 8+). The format is
// in Fuse's movie.c.
func emuFFMF(t *testing.T, b []byte) emuFImg {
	t.Helper()
	if len(b) < 16 || string(b[:7]) != "FMF_V1e" || b[7] != 'U' {
		t.Fatalf("not an uncompressed little-endian FMF movie: %q", b[:min(len(b), 16)])
	}
	var bitmap, attr [40 * 240]byte
	var good [2][40 * 240]byte
	frames := 0
	i := 16
	byte1 := func() (int, bool) {
		if i >= len(b) {
			return 0, false
		}
		i++
		return int(b[i-1]), true
	}
	// unrle reads n bytes of a run-length coded plane: a byte twice is
	// followed by how many more times it comes.
	unrle := func(n int) ([]byte, bool) {
		out := make([]byte, 0, n)
		prev := -1
		for len(out) < n {
			v, ok := byte1()
			if !ok {
				return nil, false
			}
			out = append(out, byte(v))
			if v == prev {
				k, ok := byte1()
				if !ok {
					return nil, false
				}
				for ; k > 0; k-- {
					out = append(out, byte(v))
				}
				prev = -1
				continue
			}
			prev = v
		}
		return out[:n], len(out) == n
	}
read:
	for {
		k, ok := byte1()
		if !ok {
			break
		}
		switch k {
		case 'N': // a new frame: the one before is whole
			if i+3 > len(b) {
				break read
			}
			i += 3
			good[0], good[1] = bitmap, attr
			frames++
		case '$':
			if i+6 > len(b) {
				break read
			}
			x, y, w := int(b[i]), int(b[i+1])|int(b[i+2])<<8, int(b[i+3])
			h := int(b[i+4]) | int(b[i+5])<<8
			i += 6
			if x+w > 40 || y+h > 240 {
				t.Fatalf("FMF: an area %d,%d %dx%d", x, y, w, h)
			}
			p1, ok1 := unrle(w * h)
			p2, ok2 := unrle(w * h)
			if !ok1 || !ok2 {
				break read
			}
			for yy := 0; yy < h; yy++ {
				for xx := 0; xx < w; xx++ {
					bitmap[(y+yy)*40+x+xx] = p1[yy*w+xx]
					attr[(y+yy)*40+x+xx] = p2[yy*w+xx]
				}
			}
		case 'S': // sound: encoding, frequency, channels, length-1 frames
			if i+6 > len(b) {
				break read
			}
			n := (int(b[i+4]) | int(b[i+5])<<8) + 1
			size := map[byte]int{'P': 2, 'U': 1, 'A': 1}[b[i]] * map[byte]int{'S': 2, 'M': 1}[b[i+3]]
			i += 6 + n*size
		case 'X':
			good[0], good[1] = bitmap, attr
			frames++
			break read
		default:
			t.Fatalf("FMF: chunk %q at %d", k, i-1)
		}
	}
	if frames == 0 {
		t.Fatal("FMF: no whole frame")
	}
	g := emuFImg{w: 320, h: 240, px: make([]uint32, 320*240), rgb: func(v uint32) uint32 { return zxPalette.colors[v&15] & 0xFFFFFF }}
	for y := 0; y < 240; y++ {
		for x := 0; x < 320; x++ {
			d, at := good[0][y*40+x/8], good[1][y*40+x/8]
			bright := at >> 6 & 1 * 8
			v := at>>3&7 | bright // paper
			if d&(0x80>>(x%8)) != 0 {
				v = at&7 | bright
			}
			if v == 8 {
				v = 0 // bright black is black
			}
			g.px[y*320+x] = uint32(v)
		}
	}
	return g
}

// emuFBroadway starts GTK's Broadway display server (GTK programs draw to it
// with no window anywhere) and gives the environment for GTK programs.
func emuFBroadway(t *testing.T, home string) []string {
	t.Helper()
	bw := emuFNeed(t, "broadwayd")
	run := filepath.Join(home, "run")
	os.MkdirAll(run, 0o700)
	// A free display number: broadwayd N listens on port 8080+N and on run/broadwayN+1.socket.
	n := 0
	for k := 40; k < 99; k++ {
		if l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", 8080+k)); err == nil {
			l.Close()
			n = k
			break
		}
	}
	env := emuFEnv(home, "XDG_RUNTIME_DIR="+run)
	c := emuFCmd(90, home, env, bw, fmt.Sprintf(":%d", n))
	log := filepath.Join(home, "broadwayd.log")
	lf, err := os.Create(log)
	if err != nil {
		t.Fatal(err)
	}
	c.Stdout, c.Stderr = lf, lf
	if err := c.Start(); err != nil {
		t.Fatal(err)
	}
	// timeout passes a TERM on to broadwayd (a KILL would leave it running)
	t.Cleanup(func() { c.Process.Signal(syscall.SIGTERM); c.Wait(); lf.Close() })
	for k := 0; k < 50; k++ { // its socket is an abstract one: wait for its word
		if b, _ := os.ReadFile(log); bytes.Contains(b, []byte("Listening")) {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	return append(env, "GDK_BACKEND=broadway", fmt.Sprintf("BROADWAY_DISPLAY=:%d", n))
}

// emuFFuse loads a .tap in Fuse (a 48K Spectrum, the tape autoloaded) and
// gives the screen some seconds later, from a movie Fuse records.
func emuFFuse(t *testing.T, file string, secs int) emuFImg {
	t.Helper()
	fuse := emuFNeed(t, "fuse")
	home := t.TempDir()
	env := emuFBroadway(t, home)
	movie := filepath.Join(home, "screen.fmf")
	out := emuFRun(t, secs, home, env, fuse, "--machine", "48", "--no-sound", "--no-confirm-actions",
		"--auto-load", "--traps", "--movie-compr", "None", "--movie-start", movie, "--tape", file)
	b, err := os.ReadFile(movie)
	if err != nil {
		t.Fatalf("fuse recorded no movie:\n%s", out)
	}
	return emuFFMF(t, b)
}

func TestEmuSpectrum(t *testing.T) {
	s := emuFStart(t)
	emuFNeed(t, "fuse")
	r := rand.New(rand.NewSource(48))
	for _, c := range []struct {
		name string
		own  bool
	}{{"zx-tap-rom", false}, {"zx-tap-own", true}, {"zx-text", false}} {
		t.Run(c.name, func(t *testing.T) {
			a := testApp(t, "zx")
			if c.own {
				f := zxFont.clone()
				f.id, f.name, f.h, f.format = ownID, "mine", 8, "zx"
				for ch := 33; ch < 128; ch++ { // the space stays empty
					for y := 0; y < 8; y++ {
						f.setRow(ch, y, byte(r.Intn(256)))
					}
				}
				a.useOwnFont(f, nil, true)
			}
			for i := range a.doc.cells {
				a.doc.cells[i] = zxCell(32+r.Intn(96), r.Intn(8), r.Intn(8), r.Intn(2) == 0, false)
			}
			a.doc.border = r.Intn(8)
			if c.name == "zx-text" {
				fillCells(a.doc.cells, zxCell(32, 0, 7, false, false))
				a.doc.border = 7
				a.fg, a.bg = 1, 7
				emuFText(a, "CharEdit's .tap in Fuse", "(a 48K Spectrum):", "the same page as", "CharEdit's PNG.")
			}
			p := filepath.Join(t.TempDir(), "art.tap")
			if !a.writeOut(p, "tap") {
				t.Fatal(a.message)
			}
			emuFCheck(t, s, emuFCaseFor(a, c.name, [2]int{1, 1}, [2]int{1, 1}), emuFFuse(t, p, 15))
		})
	}
}

// ---- Amstrad CPC: MAME ----

// emuFMame runs MAME's cpc6128 with the disk in drive A, types RUN"name and
// gives the snapshot MAME takes when it stops.
func emuFMame(t *testing.T, s emuFSetup, disk, name string, secs int) emuFImg {
	t.Helper()
	mame := "/usr/lib/mame/mame" // Arch's /usr/bin/mame is a script that writes ~/.mame first
	if _, err := os.Stat(mame); err != nil {
		mame = emuFNeed(t, "mame")
	}
	roms := filepath.Join(s.cache, "roms")
	if _, err := os.Stat(filepath.Join(roms, "cpc6128", "cpc6128.rom")); err != nil {
		t.Skip("no CPC ROMs: see tools/emucheck/README.md")
	}
	home := t.TempDir()
	snap := filepath.Join(home, "snap")
	out := emuFRun(t, 120, home, emuFEnv(home), mame, "cpc6128", "-rompath", roms, "-flop1", disk,
		"-video", "none", "-sound", "none", "-nothrottle", "-skip_gameinfo", "-seconds_to_run", fmt.Sprint(secs),
		"-snapshot_directory", snap, "-cfg_directory", filepath.Join(home, "cfg"),
		"-nvram_directory", filepath.Join(home, "nvram"), "-autoboot_delay", "2",
		"-autoboot_command", `run"`+name+`\n`)
	shots, _ := filepath.Glob(filepath.Join(snap, "*", "*.png"))
	if len(shots) == 0 {
		t.Fatalf("MAME took no snapshot:\n%s", out)
	}
	return emuFReadPNG(t, shots[len(shots)-1])
}

func TestEmuCPC(t *testing.T) {
	s := emuFStart(t)
	r := rand.New(rand.NewSource(6128))
	for k, mode := range []int{0, 1, 2, 1} {
		name := fmt.Sprintf("cpc-dsk-mode%d", mode)
		if k == 3 {
			name = "cpc-text"
		}
		t.Run(name, func(t *testing.T) {
			a := testApp(t, "cpc")
			a.setCPCMode(mode)
			n := a.cpcSet.info().inks
			for i := 0; i < n; i++ {
				a.setCPCInk(i, r.Intn(27))
			}
			a.setCPCBorder(r.Intn(27))
			for i := range a.doc.cells {
				a.doc.cells[i] = makeCell(r.Intn(256), r.Intn(n), r.Intn(n), 0)
			}
			if mode == 1 && name == "cpc-text" {
				fillCells(a.doc.cells, cpcBlank())
				a.fg, a.bg = 1, 0
				emuFText(a, "CharEdit's .bin on a .dsk in MAME", "(cpc6128): the same page as", "CharEdit's PNG.")
			}
			p := filepath.Join(t.TempDir(), "art.dsk")
			if !a.writeOut(p, "cpcdsk") {
				t.Fatal(a.message)
			}
			g := emuFMame(t, s, p, "art", 20)
			// MAME's picture of the CPC: the scale is found from its size.
			ec := emuFCaseFor(a, name, [2]int{1, 1}, [2]int{1, 1})
			emuFScaleTo(&ec, g)
			emuFCheck(t, s, ec, g)
		})
	}
}

// emuFScaleTo sets the scale from CharEdit's 640x400 CPC screen to the
// emulator's, guessed from the picture's size (a CPC screen is 640 of the
// emulator's widest pixels across, 200 lines).
func emuFScaleTo(c *emuFCase, g emuFImg) {
	c.sxN, c.sxD = 1, 1
	c.syN, c.syD = 1, 1
	if g.w < 640 {
		c.sxN, c.sxD = 1, 2
	}
	if g.h < 400 {
		c.syN, c.syD = 1, 2
	}
}
