package main

// The Commodore Plus/4, C16 and C116: PETSCII pages for the TED chip. The
// screen is the C64's 40x25 of screen codes, but each cell has a whole
// attribute byte: bits 0-3 the hue, 4-6 the luminance (16 x 8 = 128 numbers,
// 121 colours: black has no luminance), bit 7 flash. The page's cell keeps
// that byte in bits 8-15 (cellFg = hue, cellBg = luminance | flash << 3), the
// screen colour ($FF15) and the border ($FF19) are whole TED colours in
// Doc.bg and Doc.border. Codes 128-255 are the TED's reverse of 0-127.
//
// The characters are the ROM's, at $D000-$D7FF of the KERNAL ROM (the same in
// 318004-01, -05 and 318005-05): fetched from VICE like the other Commodore
// ROMs (pet.go). What was checked how is in the HANDOFF's Plus/4 section.

import (
	"fmt"
	"math"
	"slices"
	"strings"
)

func init() {
	machines = append(machines, Machine{"plus4", "Plus/4", "plus4", 40, 25, "tedvice", false, 16})
	cbmROMs = append(cbmROMs, cbmROM{"plus4", "Plus/4", "PLUS4/kernal-318004-05.bin", 16384,
		"7c7e07f016391174a557e790c4ef1cbe33512cdb", "plus4-kernal.bin",
		[]string{"/usr/share/vice/PLUS4/kernal-318004-05.bin", "/usr/share/vice/PLUS4/kernal"}})
	// Save as: the Plus/4's own after the C64's program and disk.
	at := slices.IndexFunc(formats, func(f Format) bool { return f.id == "d64" })
	formats = slices.Insert(formats, at+1,
		Format{"prg", "Plus/4 program that shows it (.prg)", ".prg", "c64 (Plus/4)"},
		Format{"d64", "Plus/4 disk with the program (.d64)", ".d64", "c64 (Plus/4)"},
		Format{"tedse", "TED Screen Editor screen (.scrn)", ".scrn", "c64 (Plus/4)"})
	formats = append(formats, Format{"marqbas", "For other editors: PETSCII editor BASIC (.bas)", ".bas", "c64 (Plus/4)"},
		Format{"marqasm", "For other editors: PETSCII editor ACME (.s)", ".s", "c64 (Plus/4)"})
	editorOwn["tedse"] = true
	marqMachines["PLUS4"] = "plus4"
}

// ted: the page is for the Plus/4 (C16, C116).
func (a *App) ted() bool { return a.c64() && a.machine == "plus4" }

// tedKernal is the colour each of the 16 PETSCII colour codes gives on a
// Plus/4: the KERNAL's table at $E143 (the codes, in C64 order, at $E133).
// *Verified* in kernal-318004-05.bin (and -01, 318005-05).
var tedKernal = [16]byte{0x00, 0x71, 0x32, 0x63, 0x44, 0x35, 0x46, 0x77, 0x48, 0x29, 0x5A, 0x6B, 0x5C, 0x6D, 0x2E, 0x5F}

// The power-on screen: $FF15 = $71, $FF19 = $6E (the KERNAL's TED table at $F338).
const (
	tedStartBg     = 0x71
	tedStartBorder = 0x6E
)

var tedHueNames = [16]string{"black", "white", "red", "cyan", "purple", "green", "blue", "yellow", "orange", "brown",
	"yellow-green", "pink", "blue-green", "light blue", "dark blue", "light green"}

// tedColourName is a TED colour by name and luminance.
func tedColourName(c int) string {
	if c&15 == 0 {
		return "black"
	}
	return fmt.Sprintf("%s, luminance %d", tedHueNames[c&15], c>>4&7)
}

// tedAttr is a cell's attribute byte (colour and flash).
func tedAttr(v uint32) int { return int(v>>8) & 255 }

// tedCell is screen code ch in attribute byte attr.
func tedCell(ch, attr int) uint32 { return makeCell(ch, attr&15, attr>>4&15, 0) }

// newMachineDoc is an empty screen of a Commodore machine: the Plus/4's in its power-on colours.
func newMachineDoc(m Machine) *Doc {
	d := newDoc(m.cols, m.rows)
	if m.id == "plus4" {
		d.bg, d.border = tedStartBg, tedStartBorder
		fillCells(d.cells, tedCell(32, 0))
	}
	return d
}

// ---------------------------------------------------------------- palettes

// A TED palette: the 128 colour numbers as RGB.
type tedPalette struct {
	id, name string
	c        [128]uint32
}

// tedVICE computes the colours as VICE 3.9's xplus4 shows them by default (its
// internal palette, not a .vpl file): src/plus4/ted-color.c gives each hue an
// angle and direction and each luminance a Y, video-color.c turns them into
// RGB (saturation 56 / 1.75, phase -9.5 degrees, UV to CbCr, the TED's
// default ColorSaturation 1250, gamma/contrast/brightness 1000: neutral), in
// single-precision floats as VICE does. *Verified*: an xplus4 3.9 screenshot
// of all 128 colours gives exactly these values.
func tedVICE() *tedPalette {
	p := &tedPalette{id: "tedvice", name: "Plus/4 (VICE)"}
	const red, grn, blu, orn, brn, ylg, pnk, blg, lbl, dbl, lgn = 112.5, -112.5, 0, -45, 157.5, -157.5, 90, -90, -22.5, 22.5, -135
	hues := [16]struct {
		angle float64
		dir   int
	}{{orn, 0}, {brn, 0}, {red, 1}, {red, -1}, {grn, -1}, {grn, 1}, {blu, 1}, {blu, -1},
		{orn, -1}, {brn, 1}, {ylg, 1}, {pnk, 1}, {blg, 1}, {lbl, 1}, {dbl, 1}, {lgn, 1}}
	lums := [8]float32{32, 48, 64, 80, 120, 144, 192, 256}
	clamp := func(v float32) uint32 { return uint32(max(0, min(255, int(v+0.5)))) }
	for l := 0; l < 8; l++ {
		for h := 0; h < 16; h++ {
			y := lums[l]
			if h == 0 {
				y = 0
			}
			sat := float32(56.0) / float32(1.75)
			ang := (hues[h].angle - 9.5) * (math.Pi / 180)
			cb := float32(float64(sat)*math.Cos(ang)) / float32(0.493111)
			cr := float32(float64(sat)*math.Sin(ang)) / float32(0.877283)
			switch {
			case hues[h].dir == 0:
				cb, cr = 0, 0
			case hues[h].dir < 0:
				cb, cr = -cb, -cr
			}
			cb, cr = cb*1.25, cr*1.25
			r := y + float32(1.402)*cr
			g := y - float32(0.344136)*cb - float32(0.714136)*cr
			b := y + float32(1.772)*cb
			p.c[l*16+h] = 0xFF000000 | clamp(r)<<16 | clamp(g)<<8 | clamp(b)
		}
	}
	return p
}

// tedTable is a palette given as 128 RGB values.
func tedTable(id, name string, rgb ...uint32) *tedPalette {
	p := &tedPalette{id: id, name: name}
	for i := range p.c {
		p.c[i] = 0xFF000000 | rgb[i]
	}
	return p
}

// tedPalettes: VICE's own (the default), and the two .vpl files VICE ships for
// the TED: yape-pal.vpl ("extracted from a screenshot made with YAPE") and
// colodore_ted.vpl (Pepto's colodore.com measurements), from
// github.com/VICE-Team/svn-mirror vice/data/PLUS4.
var tedPalettes = []*tedPalette{
	tedVICE(),
	tedTable("tedyape", "Plus/4 (YAPE)",
		0x000000, 0x272727, 0x600f10, 0x00403f, 0x560466, 0x004b00, 0x1a1a8c, 0x353400, 0x531e00, 0x472800, 0x184300, 0x610834, 0x00471e, 0x04297a, 0x28138f, 0x084800,
		0x000000, 0x373737, 0x6f1e1f, 0x004f4e, 0x651375, 0x045a05, 0x2a2a9c, 0x444400, 0x632e00, 0x563800, 0x285200, 0x701743, 0x00562e, 0x14388a, 0x38229e, 0x175800,
		0x000000, 0x434343, 0x7c2b2c, 0x0b5c5b, 0x722082, 0x116711, 0x3637a8, 0x515000, 0x6f3a00, 0x634400, 0x345f00, 0x7d2450, 0x0a633a, 0x214596, 0x452fab, 0x246500,
		0x000000, 0x555555, 0x8d3c3d, 0x1c6d6c, 0x833193, 0x227822, 0x4748b9, 0x626100, 0x804b11, 0x745500, 0x457000, 0x8e3561, 0x1b744b, 0x3256a7, 0x5640bc, 0x357600,
		0x000000, 0x797979, 0xb26162, 0x409190, 0xa755b7, 0x469d47, 0x6c6cde, 0x868614, 0xa57035, 0x997a22, 0x6a9415, 0xb35986, 0x3f9870, 0x567bcc, 0x7a64e1, 0x599a22,
		0x000000, 0xa9a9a9, 0xe19091, 0x70c1c0, 0xd785e7, 0x76cc76, 0x9c9cff, 0xb6b644, 0xd5a065, 0xc8a952, 0x9ac445, 0xe289b5, 0x6fc8a0, 0x86aafb, 0xaa94ff, 0x89ca52,
		0x000000, 0xc7c7c7, 0xffafb0, 0x8fe0df, 0xf6a3ff, 0x94eb95, 0xbabaff, 0xd4d462, 0xf3be83, 0xe7c870, 0xb8e263, 0xffa7d4, 0x8de7be, 0xa4c9ff, 0xc8b3ff, 0xa8e870,
		0x000000, 0xfafafa, 0xffe2e3, 0xc2ffff, 0xffd6ff, 0xc7ffc8, 0xededff, 0xffff95, 0xfff1b6, 0xfffba3, 0xebff96, 0xffdaff, 0xc0fff1, 0xd7fcff, 0xfbe6ff, 0xdbffa3),
	tedTable("tedcolodore", "Plus/4 (Colodore)", tedColodore[:]...),
}

func tedPaletteByID(id string) *tedPalette {
	for _, p := range tedPalettes {
		if p.id == id {
			return p
		}
	}
	return tedPalettes[0]
}

// tedPal is the page's TED palette.
func (a *App) tedPal() *tedPalette { return tedPaletteByID(a.paletteFor["c64"]) }

// tedRGB is TED colour c (0-127; the flash bit is ignored) on this page.
func (a *App) tedRGB(c int) uint32 { return a.tedPal().c[c&127] }

// kernal16 is the palette's 16 colours the PETSCII colour codes give: what
// a.pal holds on a Plus/4 page, for whatever looks at 16 colours only.
func (p *tedPalette) kernal16() [16]uint32 {
	var out [16]uint32
	for i, c := range tedKernal {
		out[i] = p.c[c]
	}
	return out
}

// screenRGB is a screen colour of the page (its background or border) as RGB:
// a whole TED colour on a Plus/4 page, one of the 16 elsewhere.
func (a *App) screenRGB(c int) uint32 {
	if a.ted() {
		return a.tedRGB(c)
	}
	return a.pal[c&15]
}

// tedNearest is the TED colour nearest to rgb (the weighting of nearestColour).
func (a *App) tedNearest(rgb uint32) int {
	best, bd := 0, math.MaxInt
	p := a.tedPal()
	for i, c := range p.c {
		dr, dg, db := int(c>>16&255)-int(rgb>>16&255), int(c>>8&255)-int(rgb>>8&255), int(c&255)-int(rgb&255)
		if d := 2*dr*dr + 4*dg*dg + 3*db*db; d < bd {
			best, bd = i, d
		}
	}
	return best
}

// ---------------------------------------------------------------- the brush

// tedBrush is the attribute byte new cells get: the brush's colour (a.fg is
// a whole TED colour on a Plus/4 page) and the Flash toggle.
func (a *App) tedBrush() int {
	attr := a.fg & 127
	if a.flash {
		attr |= 0x80
	}
	return attr
}

// tedStep moves the brush's hue by d (Ctrl+arrows), keeping its luminance.
func (a *App) tedStep(d int) {
	h := (a.fg&15 + d + 16) % 16
	a.fg = a.fg&0x70 | h
	a.msg("Colour: " + tedColourName(a.fg))
}

// tedLum makes the brush lighter (d > 0) or darker, as Marq's PETSCII editor's wheel does.
func (a *App) tedLum(d int) {
	l := max(0, min(7, a.fg>>4&7+d))
	a.fg = a.fg&15 | l<<4
	a.msg("Colour: " + tedColourName(a.fg))
}

// tedKey is a colour key (Ctrl/Alt+0-7 and the like): hue i at the brush's
// luminance; asked again, hue i+8.
func (a *App) tedKey(i int, bg bool) {
	cur := a.fg
	if bg {
		cur = a.doc.bg
	}
	if cur&15 == i {
		i += 8
	}
	c := cur&0x70 | i&15
	if bg {
		a.setBg(c)
		return
	}
	a.fg = c
	a.msg("Colour: " + tedColourName(c))
}

// ---------------------------------------------------------------- drawing

// tedInk is the RGB of a cell's ink, and whether the cell shows it now
// (flashing cells show only their background half of the time).
func (a *App) tedInk(v uint32) (uint32, bool) {
	attr := tedAttr(v)
	return a.tedRGB(attr), attr&0x80 == 0 || a.blink
}

// drawTEDColours draws the colours of a Plus/4 page: in the side panel (perRow
// < 16) every hue (across) at every luminance (down), in the strip under the
// page the 16 hues at the brush's luminance and the 8 luminances of its hue.
// Left: the brush's colour, right: the screen colour, middle: the border; the
// wheel over them makes the brush lighter or darker.
func (a *App) drawTEDColours(u *UI, x, y, perRow int) int {
	fb := u.fb
	pick := func(r Rect, c int) {
		mark := func(c2 int) bool { return c2&127 == c || (c&15 == 0 && c2&15 == 0) }
		if mark(a.fg) {
			fb.frame(r.inset(-1), cHi)
		}
		if mark(a.doc.bg) {
			fb.dashed(r.inset(-2), cOrange)
		}
		if mark(a.doc.borderColour()) {
			fb.fill(Rect{r.x + 1, r.y + r.h + 1, max(1, r.w-2), 1}, cHi)
		}
		u.setHint(r, fmt.Sprintf("$%02X %s: #%06X. Left: colour, right: the screen colour, middle: the border. Wheel: lighter or darker",
			c, tedColourName(c), a.tedRGB(c)&0xFFFFFF))
		if k := u.take(r); k != nil {
			switch k.button {
			case 3:
				a.setBg(c)
			case 2:
				if a.doc.border != c {
					a.pushUndo()
					a.doc.border = c
				}
			default:
				a.fg = c
			}
		}
	}
	if perRow >= 16 { // one row: the hues, then the luminances of the brush's hue
		for h := 0; h < 16; h++ {
			r := Rect{x + h*20, y, 18, 18}
			c := a.fg&0x70 | h
			fb.fill(r, a.tedRGB(c))
			fb.frame(r, cEdgeDk)
			pick(r, c)
		}
		lx := x + 16*20 + 8
		for l := 0; l < 8; l++ {
			r := Rect{lx + l*12, y, 10, 18}
			c := a.fg&15 | l<<4
			fb.fill(r, a.tedRGB(c))
			fb.frame(r, cEdgeDk)
			pick(r, c)
		}
		if d := u.takeWheel(Rect{x, y, lx + 8*12 - x, 18}); d != 0 {
			a.tedLum(-d)
		}
		return y + 18
	}
	const w, h = 17, 12       // the panel is panelW wide: 16 hues fit
	for l := 7; l >= 0; l-- { // light at the top
		for hue := 0; hue < 16; hue++ {
			r := Rect{x + hue*w, y + (7-l)*h, w - 2, h - 2}
			fb.fill(r, a.tedRGB(hue|l<<4))
			pick(r, hue|l<<4)
		}
	}
	if d := u.takeWheel(Rect{x, y, 16 * w, 8 * h}); d != 0 {
		a.tedLum(-d)
	}
	return y + 8*h
}

// ---------------------------------------------------------------- C64 and Plus/4 pages

// tedFromC64 is the TED colour nearest to C64 colour c in the C64 palette p.
func (a *App) tedFromC64(c int, p [16]uint32) int { return a.tedNearest(p[c&15] & 0xFFFFFF) }

// convertMachineColours turns a page's colours when it moves between a C64
// (or VIC-20, C128) and a Plus/4: each to the nearest the other can show.
func (a *App) convertMachineColours(from, to string) {
	if (from == "plus4") == (to == "plus4") {
		return
	}
	c64 := paletteByID(machineByID(map[bool]string{true: to, false: from}[from == "plus4"]).palette).colors
	tedPal := a.tedPal()
	if a.paletteFor["c64"] != "" && !strings.HasPrefix(a.paletteFor["c64"], "ted") {
		tedPal = tedPalettes[0]
	}
	nearest16 := func(rgb uint32) int {
		best, bd := 0, math.MaxInt
		for i, c := range c64 {
			dr, dg, db := int(c>>16&255)-int(rgb>>16&255), int(c>>8&255)-int(rgb>>8&255), int(c&255)-int(rgb&255)
			if d := 2*dr*dr + 4*dg*dg + 3*db*db; d < bd {
				best, bd = i, d
			}
		}
		return best
	}
	nearestTED := func(rgb uint32) int {
		best, bd := 0, math.MaxInt
		for i, c := range tedPal.c {
			dr, dg, db := int(c>>16&255)-int(rgb>>16&255), int(c>>8&255)-int(rgb>>8&255), int(c&255)-int(rgb&255)
			if d := 2*dr*dr + 4*dg*dg + 3*db*db; d < bd {
				best, bd = i, d
			}
		}
		return best
	}
	conv := func(d *Doc) *Doc {
		n := d.clone()
		if to == "plus4" {
			n.bg = nearestTED(c64[d.bg&15])
			if d.border >= 0 {
				n.border = nearestTED(c64[d.border&15])
			}
			for i, v := range d.cells {
				n.cells[i] = tedCell(cellCh(v), nearestTED(c64[cellFg(v)]))
			}
		} else {
			n.bg = nearest16(tedPal.c[d.bg&127])
			if d.border >= 0 {
				n.border = nearest16(tedPal.c[d.border&127])
			}
			for i, v := range d.cells {
				n.cells[i] = makeCell(cellCh(v), nearest16(tedPal.c[tedAttr(v)&127]), 0, 0)
			}
		}
		return n
	}
	a.withEveryFrame(conv)
	if to == "plus4" {
		a.fg = nearestTED(c64[a.fg&15])
	} else {
		a.fg = nearest16(tedPal.c[a.fg&127])
	}
}

// tedColodore is colodore_ted.vpl (VICE): "TED PAL palette based on http://www.colodore.com/".
var tedColodore = [128]uint32{
	0x000000, 0x171717, 0x46070a, 0x002a26, 0x3e0246, 0x003300, 0x0f0d70, 0x1f2100, 0x3e0e00, 0x301700, 0x0f2b00, 0x460326, 0x00310a, 0x031761, 0x1f0770, 0x033100,
	0x000000, 0x262626, 0x591417, 0x013b37, 0x510c59, 0x054501, 0x1e1c85, 0x303200, 0x511c01, 0x422700, 0x1e3c00, 0x590e37, 0x014217, 0x0f2675, 0x301385, 0x0f4300,
	0x000000, 0x373737, 0x6d2327, 0x0c4e49, 0x641b6d, 0x12580c, 0x2e2c9b, 0x414400, 0x642c0c, 0x553800, 0x2e4e00, 0x6d1d49, 0x0c5527, 0x1d378a, 0x41229b, 0x1d5600,
	0x000000, 0x4a4a4a, 0x813338, 0x1a615d, 0x792a82, 0x206c1a, 0x3f3db1, 0x545700, 0x793d1a, 0x684a07, 0x3f6200, 0x812d5d, 0x1a6938, 0x2d49a0, 0x5433b1, 0x2d6907,
	0x000000, 0x7b7b7b, 0xb86267, 0x449690, 0xaf58b9, 0x4ca144, 0x706deb, 0x878a1f, 0xaf6e44, 0x9d7c2b, 0x70961f, 0xb85a90, 0x449e67, 0x5b7bd9, 0x8762eb, 0x5b9e2b,
	0x000000, 0x9b9b9b, 0xdb8186, 0x61b7b1, 0xd176dc, 0x69c360, 0x8f8cff, 0xa8ab38, 0xd18d60, 0xbf9c45, 0x8fb738, 0xdb79b1, 0x61c086, 0x799bfd, 0xa880ff, 0x79c045,
	0x000000, 0xe0e0e0, 0xffc3c9, 0xa0fef8, 0xffb7ff, 0xa9ff9f, 0xd3d0ff, 0xedf171, 0xffd19f, 0xffe081, 0xd3fe71, 0xffbaf8, 0xa0ffc9, 0xbbe0ff, 0xedc3ff, 0xbbff81,
	0x000000, 0xffffff, 0xffffff, 0xfdffff, 0xffffff, 0xfffffd, 0xffffff, 0xffffc9, 0xfffffd, 0xffffdb, 0xffffc9, 0xffffff, 0xfdffff, 0xffffff, 0xffffff, 0xffffdb,
}

// tedMode is the page type as splitStream knows it: "plus4" for a Plus/4's PETSCII.
func (a *App) tedMode() string {
	if a.ted() {
		return "plus4"
	}
	return a.mode
}

// tedBrushFor gives the brush a colour of the machine a page moves to when its
// colours are not converted: on a Plus/4 black on a new page (its power-on
// white), white on a file read again (on black); light blue on a C64.
func (a *App) tedBrushFor(from, to string, fresh bool) {
	switch {
	case to == "plus4" && from != "plus4":
		a.fg, a.flash = map[bool]int{true: 0x00, false: 0x71}[fresh], false
	case from == "plus4" && to != "plus4":
		a.fg = 14
	}
}
