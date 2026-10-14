package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// The interface text in a TrueType or OpenType font: the system's sans serif,
// the Go font that comes with the program, or a font file. Glyphs are drawn
// once into alpha masks and blended in the text's colour after that.

type ttFace struct {
	face   font.Face
	ascent int
	adv    map[rune]fixed.Int26_6
	masks  map[rune]*glyphMask
}

// glyphMask is a glyph's coverage, placed relative to the pen on the baseline.
type glyphMask struct {
	dx, dy, w, h int
	a            []uint8
}

func newTTFace(data []byte, px int) (*ttFace, int, error) {
	var f *opentype.Font
	var err error
	if len(data) >= 4 && string(data[:4]) == "ttcf" { // a collection: its first font
		var c *opentype.Collection
		if c, err = opentype.ParseCollection(data); err == nil {
			f, err = c.Font(0)
		}
	} else {
		f, err = opentype.Parse(data)
	}
	if err != nil {
		return nil, 0, err
	}
	face, err := opentype.NewFace(f, &opentype.FaceOptions{Size: float64(px), DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		return nil, 0, err
	}
	m := face.Metrics()
	t := &ttFace{face: face, ascent: m.Ascent.Ceil(), adv: map[rune]fixed.Int26_6{}, masks: map[rune]*glyphMask{}}
	h := m.Ascent.Ceil() + m.Descent.Ceil()
	if _, _, ok := face.GlyphBounds('M'); !ok {
		return nil, 0, errors.New("the font has no Latin letters")
	}
	return t, h + 1, nil
}

func (t *ttFace) advance(r rune) fixed.Int26_6 {
	if v, ok := t.adv[r]; ok {
		return v
	}
	v, ok := t.face.GlyphAdvance(r)
	if !ok {
		v, _ = t.face.GlyphAdvance('?')
	}
	t.adv[r] = v
	return v
}

func (t *ttFace) mask(r rune) *glyphMask {
	if g, ok := t.masks[r]; ok {
		return g
	}
	g := &glyphMask{}
	dr, img, mp, _, ok := t.face.Glyph(fixed.P(0, 0), r)
	if !ok {
		dr, img, mp, _, ok = t.face.Glyph(fixed.P(0, 0), '?')
	}
	if ok && !dr.Empty() {
		g.dx, g.dy, g.w, g.h = dr.Min.X, dr.Min.Y, dr.Dx(), dr.Dy()
		g.a = make([]uint8, g.w*g.h)
		for y := 0; y < g.h; y++ {
			for x := 0; x < g.w; x++ {
				_, _, _, al := img.At(mp.X+x, mp.Y+y).RGBA()
				g.a[y*g.w+x] = uint8(al >> 8)
			}
		}
	}
	t.masks[r] = g
	return g
}

func (t *ttFace) width(s string) int {
	var w fixed.Int26_6
	for _, r := range s {
		w += t.advance(r)
	}
	return w.Round()
}

func (t *ttFace) draw(f *FB, x, y int, s string, c uint32) int {
	var pen fixed.Int26_6
	base := y + t.ascent
	cr, cg, cb := int(c>>16&255), int(c>>8&255), int(c&255)
	clip := f.clip
	for _, r := range s {
		g := t.mask(r)
		gx := x + pen.Round() + g.dx
		for j := 0; j < g.h; j++ {
			py := base + g.dy + j
			if py < clip.y || py >= clip.y+clip.h {
				continue
			}
			for i := 0; i < g.w; i++ {
				al := int(g.a[j*g.w+i])
				px := gx + i
				if al == 0 || px < clip.x || px >= clip.x+clip.w {
					continue
				}
				p := &f.px[py*f.w+px]
				na := 255 - al
				pr := (int(*p>>16&255)*na + cr*al) / 255
				pg := (int(*p>>8&255)*na + cg*al) / 255
				pb := (int(*p&255)*na + cb*al) / 255
				*p = 0xFF000000 | uint32(pr)<<16 | uint32(pg)<<8 | uint32(pb)
			}
		}
		pen += t.advance(r)
	}
	return x + pen.Round()
}

// uiFontChoices are the interface fonts as Settings lists them, by prefs.UIFont.
var uiFontChoices = []struct{ id, name string }{
	{"", "The system's"},
	{"bundled", "Go (comes with Charcoal)"},
	{"topaz", "Topaz (the Amiga's)"},
	{"file", "A font file..."},
}

// uiFontSizes are the sizes Settings offers, in pixels.
var uiFontSizes = []int{11, 12, 13, 14, 15, 16, 18, 20}

const defaultUIFontSize = 13

// applyUIFont makes the interface text what the settings say; a font that
// cannot be had falls back to the Go font, then to Topaz, and says so.
func applyUIFont() {
	topaz := func() { setFace(bitmapFace{fontByID("topaz2")}, 16) }
	if prefs.UIFont == "topaz" {
		topaz()
		return
	}
	size := prefs.UIFontSize
	if size == 0 {
		size = defaultUIFontSize
	}
	data, what := goregular.TTF, ""
	switch prefs.UIFont {
	case "file":
		b, err := os.ReadFile(prefs.UIFontFile)
		if err != nil {
			what = "The interface font could not be read (" + err.Error() + "): the Go font instead"
		} else {
			data = b
		}
	case "":
		if b := systemFont(); b != nil {
			data = b
		}
	}
	face, h, err := newTTFace(data, size)
	if err != nil && prefs.UIFont != "bundled" {
		what = "The interface font does not work (" + err.Error() + "): the Go font instead"
		face, h, err = newTTFace(goregular.TTF, size)
	}
	if err != nil {
		topaz()
		return
	}
	setFace(face, h)
	if what != "" && a != nil {
		a.msg(what)
	}
}

// systemFont reads the system's sans serif, or gives nil.
func systemFont() []byte {
	var paths []string
	switch runtime.GOOS {
	case "darwin":
		paths = []string{"/System/Library/Fonts/Helvetica.ttc", "/System/Library/Fonts/SFNS.ttf", "/Library/Fonts/Arial.ttf"}
	case "windows":
		dir := os.Getenv("WINDIR")
		if dir == "" {
			dir = `C:\Windows`
		}
		paths = []string{filepath.Join(dir, "Fonts", "segoeui.ttf"), filepath.Join(dir, "Fonts", "arial.ttf")}
	default: // fontconfig knows the desktop's choice
		if out, err := exec.Command("fc-match", "-f", "%{file}", "sans-serif:style=Regular").Output(); err == nil {
			paths = append(paths, strings.TrimSpace(string(out)))
		}
		paths = append(paths, "/usr/share/fonts/noto/NotoSans-Regular.ttf", "/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
			"/usr/share/fonts/TTF/DejaVuSans.ttf")
	}
	for _, p := range paths {
		low := strings.ToLower(p)
		if !strings.HasSuffix(low, ".ttf") && !strings.HasSuffix(low, ".otf") && !strings.HasSuffix(low, ".ttc") {
			continue // fontconfig may name a .pcf or .pfb: not for us
		}
		if b, err := os.ReadFile(p); err == nil {
			return b
		}
	}
	return nil
}
