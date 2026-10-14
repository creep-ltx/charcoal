package main

// The interface's look as values, so it can change while the program runs: a
// theme sets the colours below, and uiFace measures and draws the interface
// text. Today there is one theme (the dark greys) and one face (Topaz); light,
// Workbench and a system font come on top of this.

// Theme is one look of the interface.
type Theme struct {
	Name     string
	Panel    uint32 // panels and gadgets
	Status   uint32 // the status line
	Text     uint32
	Hi       uint32 // headings, chosen items
	Dim      uint32 // quiet text: folders, notes
	EdgeLt   uint32 // a gadget's lit edge
	EdgeDk   uint32 // its shaded edge
	On       uint32 // a gadget that is on
	Accent   uint32 // the cursor, warnings, the current item
	Surround uint32 // outside the page
	Faint    uint32 // marks that are off: unlit bars, lines between groups
}

var themes = []Theme{
	{
		Name:     "Dark",
		Panel:    0xFF26282D,
		Status:   0xFF1C1E22,
		Text:     0xFF9EA3AD,
		Hi:       0xFFE2E5EA,
		Dim:      0xFF6B707A,
		EdgeLt:   0xFF41454D,
		EdgeDk:   0xFF0E0F11,
		On:       0xFF34507A,
		Accent:   0xFFFF8A00,
		Surround: 0xFF141518,
		Faint:    0xFF41454D,
	},
	{
		Name:     "Light",
		Panel:    0xFFE8E6E1,
		Status:   0xFFDAD7D1,
		Text:     0xFF35363B,
		Hi:       0xFF101114,
		Dim:      0xFF6E7076,
		EdgeLt:   0xFFFFFFFF,
		EdgeDk:   0xFFA8A49C,
		On:       0xFFC3D3EC,
		Accent:   0xFFB04A0A,
		Surround: 0xFFC9C6C0,
		Faint:    0xFFBDB9B1,
	},
	{
		// Workbench 3, made calmer: its grey, black text, white and dark edges, its blue.
		Name:     "Workbench",
		Panel:    0xFFB2B6BE,
		Status:   0xFF9CA1AA,
		Text:     0xFF000000,
		Hi:       0xFF14264A,
		Dim:      0xFF474C55,
		EdgeLt:   0xFFFFFFFF,
		EdgeDk:   0xFF3B3F47,
		On:       0xFF8EA8D2,
		Accent:   0xFF9C3A00,
		Surround: 0xFF56607A,
		Faint:    0xFF858A94,
	},
}

// themeNames lists the themes for the menus and Settings.
func themeNames() []string {
	var n []string
	for _, t := range themes {
		n = append(n, t.Name)
	}
	return n
}

// setTheme switches to the named theme and remembers it.
func setTheme(name string) {
	useTheme(name)
	prefs.Theme = theme.Name
	if prefs.Theme == themes[0].Name {
		prefs.Theme = ""
	}
	savePrefs()
}

// The current theme's colours, read by all drawing code. useTheme sets them.
var (
	cGrey, cDark, cText, cHi, cDim uint32
	cEdgeLt, cEdgeDk               uint32
	cBlue, cOrange                 uint32
	cStartBack, cFaint             uint32
	theme                          Theme
)

// cPaper is the page's black, not part of a theme.
const cPaper = 0xFF000000

func init() { useTheme("Dark") }

// useTheme switches to the named theme; an unknown name keeps the first.
func useTheme(name string) {
	t := themes[0]
	for _, th := range themes {
		if th.Name == name {
			t = th
		}
	}
	theme = t
	cGrey, cDark, cText, cHi, cDim = t.Panel, t.Status, t.Text, t.Hi, t.Dim
	cEdgeLt, cEdgeDk = t.EdgeLt, t.EdgeDk
	cBlue, cOrange = t.On, t.Accent
	cStartBack, cFaint = t.Surround, t.Faint
}

// ---------------------------------------------------------------- the interface text

// A face measures and draws the interface text. Layout asks it for widths in
// pixels and never counts characters, so a face whose letters differ in width
// (a system font) can take Topaz's place.
type face interface {
	draw(f *FB, x, y int, s string, c uint32) int // returns the x after the text
	width(s string) int
}

// bitmapFace is a fixed-width bitmap font, Latin-1, one pixel per bit.
type bitmapFace struct{ font *Font }

func (b bitmapFace) draw(f *FB, x, y int, s string, c uint32) int {
	for _, r := range s {
		ch := byte('?')
		if r < 256 {
			ch = byte(r)
		}
		f.glyph(b.font, x, y, ch, c, 0, 1, 1, false)
		x += 8
	}
	return x
}

func (b bitmapFace) width(s string) int {
	n := 0
	for range s {
		n++
	}
	return n * 8
}

var uiFace face

// lineH is the height of a line of interface text.
var lineH = 16

func (f *FB) text(x, y int, s string, c uint32) int { return face0().draw(f, x, y, s, c) }

func textW(s string) int { return face0().width(s) }

// face0 is the interface face, Topaz until main (or a test) sets another.
func face0() face {
	if uiFace == nil {
		setFace(bitmapFace{fontByID("topaz2")}, 16)
	}
	return uiFace
}

// digitsW is the width of n digits: the size of a number field, a column of figures.
func digitsW(n int) int { return n * textW("0") }

// fitText returns how many runes of s fit in w pixels.
func fitText(s []rune, w int) int {
	n := 0
	for n < len(s) && textW(string(s[:n+1])) <= w {
		n++
	}
	return n
}

// setFace makes f the interface text, with lines h pixels high, and resizes
// what follows the text's height.
func setFace(f face, h int) {
	uiFace, lineH = f, h
	gadH, smallH = lineH+6, lineH+4
	tileH = tileScreen + 12 + lineH + 8
}
