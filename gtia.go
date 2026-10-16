package main

// The Atari 8-bit's colours. The GTIA chip makes 256 colour values (hue in
// the top four bits, luminance in the low four); text modes ignore the
// luminance's lowest bit, so a GRAPHICS 0 screen has 128 colours. An Atari
// page keeps the machine's three colour registers in its Doc:
//
//	d.bg     COLPF2 (710, SETCOLOR 2): the screen
//	d.ink    COLPF1 (709, SETCOLOR 1): the text, but only its luminance: the
//	         characters take the screen's hue (atariText)
//	d.border COLBK  (712, SETCOLOR 4): the border
//
// as values 0-255, what a program pokes there. Start-up: 148 ($94), 202 ($CA), 0.
//
// The RGB of each value comes from one of two emulators' models. The default
// (palette "atari", since 2026-10-16) is Altirra's (altirraTable below), what
// compo screenshots are taken with. The other is atari800's palette generator
// (src/colours_ntsc.c and colours_pal.c, github.com/atari800/atari800; its
// "standard" preset: gamma 2.35, black level 16, white 235, colour delay 26.8
// degrees on NTSC and 23.2 on PAL), written again here from its description:
// luminance from GTIA's luma steps (CGIA.PDF), hue as a phase delay of the
// colour subcarrier, NTSC decoded as YIQ, PAL as YUV averaged over the even
// and odd lines' delays. Only the numbers of the model are taken, no code.

import (
	"fmt"
	"math"
)

// The Atari's start-up registers.
const atariBg, atariInk, atariBorder = 0x94, 0xCA, 0x00

// gtiaPAL is the default table (Altirra's PAL), gtiaAltNTSC Altirra's NTSC;
// gtia800PAL and gtiaNTSC are atari800's.
var gtiaPAL, gtiaAltNTSC = altirraTable(true), altirraTable(false)
var gtia800PAL, gtiaNTSC = gtiaTable(true), gtiaTable(false)

// The palettes in the Palette gadget; their 16 colours are the hues at a middle
// luminance (other pages' code may index them), an Atari page uses gtiaFor.
// The ids "atari" and "atarintsc" are older than the Altirra palettes: "atari"
// was atari800's PAL until 2026-10-16 and now is Altirra's, the default.
var atariPalette, atariNTSCPalette = gtiaPalette("atari", "Atari PAL (Altirra)", &gtiaPAL), gtiaPalette("atarintsc", "Atari NTSC (atari800)", &gtiaNTSC)
var atari800Palette, atariAltNTSCPalette = gtiaPalette("atari800pal", "Atari PAL (atari800)", &gtia800PAL), gtiaPalette("atarintscalt", "Atari NTSC (Altirra)", &gtiaAltNTSC)

// gtiaPalettes are the Atari page's palettes, in the gadget's order.
var gtiaPalettes = []*Palette{atariPalette, atariAltNTSCPalette, atari800Palette, atariNTSCPalette}

func gtiaPalette(id, name string, t *[256]uint32) *Palette {
	p := &Palette{id: id, name: name}
	for i := range p.colors {
		p.colors[i] = t[i<<4|8]
	}
	return p
}

func init() { // the other GTIA palettes after the default one
	for i, p := range palettes {
		if p == atariPalette {
			palettes = append(palettes[:i+1], append(append([]*Palette{}, gtiaPalettes[1:]...), palettes[i+1:]...)...)
			break
		}
	}
}

// isGTIAPalette: one of the Atari page's palettes.
func isGTIAPalette(p *Palette) bool {
	for _, q := range gtiaPalettes {
		if p == q {
			return true
		}
	}
	return false
}

// gtiaFor is the colour table a palette id gives (Altirra's PAL for any other id).
func gtiaFor(id string) *[256]uint32 {
	switch id {
	case "atarintsc":
		return &gtiaNTSC
	case "atari800pal":
		return &gtia800PAL
	case "atarintscalt":
		return &gtiaAltNTSC
	}
	return &gtiaPAL
}

// gtia is the colour table of the Atari page's palette.
func (a *App) gtia() *[256]uint32 { return gtiaFor(a.paletteFor["atari"]) }

// atariRGB is the colour a register value shows (luminance bit 0 ignored).
func (a *App) atariRGB(v int) uint32 { return a.gtia()[v&0xFE] }

// atariText is the colour value the characters show: COLPF2's hue, COLPF1's luminance.
func atariText(d *Doc) int { return d.bg&0xF0 | d.ink&0x0E }

// gtiaTable makes the 256 colours of a PAL or an NTSC machine.
func gtiaTable(pal bool) [256]uint32 {
	// GTIA's luminance steps, the same on both (atari800 takes them from CGIA.PDF).
	luma := [16]float64{0.6941, 0.7091, 0.7241, 0.7401, 0.7560, 0.7741, 0.7931, 0.8121,
		0.8260, 0.8470, 0.8700, 0.8930, 0.9160, 0.9420, 0.9690, 1.0000}
	const black, white, gamma = 16.0 / 255, 235.0 / 255, 2.35
	var out [256]uint32
	for hue := 0; hue < 16; hue++ {
		var u, v float64 // chroma: I/Q on NTSC, U/V on PAL
		if hue > 0 {
			if pal {
				u, v = gtiaPALChroma(hue)
			} else {
				const burst = 303.0 * math.Pi / 180 // the colour burst in YIQ (YUV's 180 degrees turned 33)
				angle := burst + float64(hue-1)*26.8*math.Pi/180
				u, v = math.Cos(angle)*0.175, math.Sin(angle)*0.175
			}
		}
		for lum := 0; lum < 16; lum++ {
			y := (luma[lum] - luma[0]) / (luma[15] - luma[0])
			y = y*(white-black) + black
			var r, g, b float64
			if pal {
				r, g, b = y+1.13983*v, y-0.39465*u-0.58060*v, y+2.03211*u
			} else {
				r, g, b = y+0.9563*u+0.6210*v, y-0.2721*u-0.6474*v, y-1.1070*u+1.7046*v
			}
			c := func(x float64) uint32 { // the TV's gamma, then sRGB
				if x >= 0 {
					x = math.Pow(x, gamma)
				} else {
					x /= 12.92
				}
				if x <= 0.0031308 {
					x *= 12.92
				} else {
					x = 1.055*math.Pow(x, 1/2.4) - 0.055
				}
				return uint32(max(0, min(255, int(x*255))))
			}
			out[hue<<4|lum] = 0xFF000000 | c(r)<<16 | c(g)<<8 | c(b)
		}
	}
	return out
}

// altirraTable makes the 256 colours of Altirra's default PAL or NTSC palette
// (Avery Lee's Altirra 4.40, src/Altirra/source/palettegenerator.cpp and the
// presets default_pal / default_ntsc in gtia.cpp; GPL-2+): written again here
// from its model, only its constants taken. Luminance is the 800XL's measured
// luma ramp; chroma a vector of the saturation at the hue's angle (hue 1 at
// the start angle, a step per hue), on PAL the average of the even and odd
// lines' phases and inversions; YIQ to RGB. PAL: no colour matching, RGB as it
// is. NTSC: brightness -0.04, contrast 1.04, then NTSC primaries to sRGB.
func altirraTable(pal bool) [256]uint32 {
	ramp := [16]float64{0, 0.0658340, 0.1435022, 0.2093362, 0.2750246, 0.3408586, 0.4185267, 0.4843608,
		0.5156392, 0.5814733, 0.6591414, 0.7249754, 0.7906638, 0.8564978, 0.9341660, 1}
	// The PAL GTIA's phase per hue 1-15 in hue steps, and its sign, even and odd lines.
	phase := [15][4]float64{{0, 1, 0, 1}, {1, 1, 1, 1}, {-6, -1, 2, 1}, {-5, -1, -5, -1}, {-4, -1, -4, -1},
		{-3, -1, -3, -1}, {-1, -1, -1, -1}, {0, -1, 0, -1}, {1, -1, 1, -1}, {-6, 1, 2, -1}, {-4, 1, -4, 1},
		{-3, 1, -3, 1}, {-2, 1, -2, 1}, {-1, 1, -1, 1}, {0, 1, 0, 1}}
	start, hueRange, sat, bright, contrast := -12.0, 18.3*15, 0.29, 0.0, 1.0
	if !pal {
		start, hueRange, sat, bright, contrast = -57, 27.1*15, 0.20, -0.04, 1.04
	}
	rad := math.Pi / 180
	step := hueRange / 15 * rad
	// NTSC (SMPTE C primaries) to XYZ, XYZ to linear sRGB.
	fromNTSC := [3][3]float64{{0.6068909, 0.1735011, 0.2003480}, {0.2989164, 0.5865990, 0.1144845}, {0, 0.0660957, 1.1162243}}
	toSRGB := [3][3]float64{{3.2404542, -1.5371385, -0.4985314}, {-0.9692660, 1.8760108, 0.0415560}, {0.0556434, -0.2040259, 1.0572252}}
	var out [256]uint32
	for hue := 0; hue < 16; hue++ {
		var i, q float64
		if hue > 0 {
			if pal {
				ph := phase[hue-1]
				a2, a3 := start*rad+step*ph[0], start*rad+step*ph[2]
				i = (math.Cos(a2)*ph[1] + math.Cos(a3)*ph[3]) * 0.5 * sat
				q = (math.Sin(a2)*ph[1] + math.Sin(a3)*ph[3]) * 0.5 * sat
			} else {
				a := start*rad + step*float64(hue-1)
				i, q = sat*math.Cos(a), sat*math.Sin(a)
			}
		}
		chroma := [3]float64{0.956*i + 0.621*q, -0.272*i - 0.647*q, -1.107*i + 1.704*q}
		for lum := 0; lum < 16; lum++ {
			y := contrast*ramp[lum] + bright
			var rgb [3]float64
			for k := range rgb {
				rgb[k] = y + chroma[k]
			}
			if !pal { // gamma 2.2 to linear, NTSC primaries to sRGB, sRGB's curve
				var lin, xyz [3]float64
				for k := range lin {
					lin[k] = math.Pow(max(rgb[k], 0), 2.2)
				}
				for r := range xyz {
					for k := range lin {
						xyz[r] += fromNTSC[r][k] * lin[k]
					}
				}
				for r := range rgb {
					x := 0.0
					for k := range xyz {
						x += toSRGB[r][k] * xyz[k]
					}
					if x < 0.0031308 {
						rgb[r] = x * 12.92
					} else {
						rgb[r] = 1.055*math.Pow(x, 1/2.4) - 0.055
					}
				}
			}
			c := func(x float64) uint32 { return uint32(max(0, min(255, math.Round(x*255)))) }
			out[hue<<4|lum] = 0xFF000000 | c(rgb[0])<<16 | c(rgb[1])<<8 | c(rgb[2])
		}
	}
	return out
}

// gtiaPALChroma is a PAL hue's U and V: the PAL GTIA delays the subcarrier by a base
// delay, maybe a second fixed one, and 0-7 steps of the DEL pin's delay, different
// in even and odd lines (measured on oscillograms, see atari800's colours_pal.c);
// a TV averages the two lines.
func gtiaPALChroma(hue int) (float64, float64) {
	type del struct{ add, mult float64 }
	even := [15]del{{1, 5}, {1, 6}, {1, 7}, {0, 0}, {0, 1}, {0, 2}, {0, 4}, {0, 5}, {0, 6}, {0, 7}, {1, 1}, {1, 2}, {1, 3}, {1, 4}, {1, 5}}
	odd := [15]del{{1, 1}, {1, 0}, {0, 7}, {0, 6}, {0, 5}, {0, 4}, {0, 2}, {0, 1}, {0, 0}, {1, 7}, {1, 5}, {1, 4}, {1, 3}, {1, 2}, {1, 1}}
	const base, add, step = 0.421894970414201, 0.446563064859117, 23.2 / 360 // in subcarrier periods
	delay := func(d del) float64 { return base + add*d.add + step*d.mult }
	evenBurst, oddBurst := delay(even[0]), delay(odd[0]) // the bursts are hue 1's
	sub := (evenBurst + oddBurst) / 2
	diff := evenBurst - oddBurst
	diff -= math.Floor(diff)
	sat := 0.0
	if !(diff > 0.45 && diff < 0.55) {
		sat = math.Sqrt2 / math.Sqrt(2*math.Cos(diff*2*math.Pi)+2)
	}
	sat *= 0.175
	ea := (0.5 - (delay(even[hue-1]) - sub)) * 2 * math.Pi
	oa := (0.5 + (delay(odd[hue-1]) - sub)) * 2 * math.Pi
	return (math.Cos(ea) + math.Cos(oa)) / 2 * sat, (math.Sin(ea) + math.Sin(oa)) / 2 * sat
}

// gtiaNearest is the colour value (even luminance) nearest to an RGB colour.
func gtiaNearest(rgb uint32, t *[256]uint32) int {
	best, bd := 0, math.MaxInt
	for v := 0; v < 256; v += 2 {
		c := t[v]
		dr, dg, db := int(c>>16&255)-int(rgb>>16&255), int(c>>8&255)-int(rgb>>8&255), int(c&255)-int(rgb&255)
		if d := 2*dr*dr + 4*dg*dg + 3*db*db; d < bd {
			best, bd = v, d
		}
	}
	return best
}

// atariOld16 were the Atari page's 16 colours before 2026-10-16 (Doc colours were
// numbers 0-15 then); old autosaves are read through it.
var atariOld16 = [16]uint32{0x000000, 0xFFFFFF, 0x1C3F9E, 0x8EB4F0, 0x555555, 0xAAAAAA, 0xA83224, 0xE07A3A,
	0xD6C840, 0x3E9A3A, 0x9AE07A, 0x2E8C8C, 0x7A3AA8, 0xD07AD0, 0x6B4A1E, 0xE8D8A8}

// atariFromOld turns an old page's colour numbers into register values.
func atariFromOld(d *Doc) {
	conv := func(i int) int {
		switch i & 15 {
		case 2:
			return atariBg // the old start-up colours give the real ones
		case 3:
			return atariInk
		}
		return gtiaNearest(atariOld16[i&15], &gtia800PAL) // the table of the time
	}
	d.bg, d.ink, d.border = conv(d.bg), conv(d.ink), conv(max(0, d.border))
}

// ---------------------------------------------------------------- the colours in the panel

// drawAtariColours draws the 128 colours, 16 hues across and 8 luminances down: a
// click sets the text's luminance (COLPF1), a right click the screen (COLPF2), a
// middle click the border (COLBK). Under them the three registers' values, as
// a program pokes them. It returns the y under them.
func (a *App) drawAtariColours(u *UI, x, y, perRow int) int {
	fb := u.fb
	cw := perRow * 20 / 16 // 10 in the panel, 20 in the strip
	ch := min(cw, 12)
	d := a.doc
	text := atariText(d)
	for v := 0; v < 256; v += 2 {
		r := Rect{x + v>>4*cw, y + v&15/2*ch, cw - 1, ch - 1}
		fb.fill(r, a.atariRGB(v))
		if v == d.ink&0xFE {
			fb.frame(r.inset(-1), cHi)
		}
		if v == d.bg&0xFE {
			fb.dashed(r.inset(-1), cOrange)
		}
		if v == d.border&0xFE {
			fb.fill(Rect{r.x + 1, r.y + r.h - 2, r.w - 2, 2}, cHi)
		}
		u.setHint(r, fmt.Sprintf("Colour %d ($%02X): hue %d, luminance %d. Left: the text (only its luminance: the text takes the screen's hue), right: the screen, middle: the border", v, v, v>>4, v&15))
		if k := u.take(r); k != nil {
			switch k.button {
			case 3:
				a.setBg(v)
			case 2:
				a.setAtariBorder(v)
			default:
				a.setFg(v)
			}
		}
	}
	y += 8 * ch
	fb.fill(Rect{x, y + 3, 10, 10}, a.atariRGB(text))
	s := fmt.Sprintf("709: %d  710: %d  712: %d", d.ink, d.bg, d.border)
	fb.text(x+14, y+1, s, cText)
	u.setHint(Rect{x, y, 14 + textW(s), lineH}, fmt.Sprintf("The colour registers: COLOR1 (709, the text's luminance) %d, COLOR2 (710, the screen) %d, COLOR4 (712, the border) %d. The text shows as %d", d.ink, d.bg, d.border, text))
	return y + lineH
}

func (a *App) setAtariBorder(v int) {
	if a.doc.border != v {
		a.pushUndo()
		a.doc.border = v
	}
}

// atariColourKey: on an Atari page the colour keys 0-7 set the text's luminance
// (0, 2 ... 14), with the background keys the screen's hue (0-15).
func (a *App) atariColourKey(i int, bg bool) {
	if bg {
		a.setBg(i<<4 | a.doc.bg&0x0F)
		a.msg(fmt.Sprintf("Screen colour %d (hue %d)", a.doc.bg, i))
		return
	}
	if i > 7 {
		a.msg("The text has 8 luminances on an Atari (0-7)")
		return
	}
	a.setFg(a.doc.ink&0xF0 | i*2)
	a.msg(fmt.Sprintf("Text luminance %d (709: %d)", i*2, a.doc.ink))
}

// atariStep steps the text's luminance, or the screen's colour through all 128.
func (a *App) atariStep(d int, bg bool) {
	if bg {
		a.setBg((a.doc.bg&0xFE + 2*d + 256) & 0xFE)
		return
	}
	a.setFg(a.doc.ink&0xF0 | (a.doc.ink&0x0E+2*d+16)&0x0E)
}
