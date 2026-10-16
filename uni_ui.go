package main

// The Unicode page in the window: drawing its cells, the character table by
// blocks, the 256 and 24-bit colours, the Half tool, switching to and from the
// page type (docs/unicode-design.md).

import (
	"fmt"
	"strconv"
	"strings"
)

// drawUniCell draws a Unicode cell at pixel (px, py), every font pixel z by z.
func (a *App) drawUniCell(fb *FB, px, py, z int, v uint32) {
	fg, bg := uniColoursOf(v)
	fgc, bgc := 0xFF000000|colourRGB(fg, &a.pal), 0xFF000000|colourRGB(bg, &a.pal)
	code := uniCode(v)
	fl := uniFlagsOf(v)
	it, ul := fl&italic != 0, fl&underline != 0
	g, ok := uniGlyph(code, a.fontFor["uni"] != "unifont")
	switch {
	case code == 0 || code == 32:
		g = make([]byte, 16)
	case !ok:
		g = missingGlyph
	}
	cl := fb.clip
	inside := px >= cl.x && py >= cl.y && px+8*z <= cl.x+cl.w && py+16*z <= cl.y+cl.h
	for r := 0; r < 16; r++ {
		bits := int(g[r])
		if it {
			bits >>= max(0, (6-r/2)>>1)
		}
		if ul && r >= 14 {
			bits = 0xFF
		}
		for b := 0; b < 8; b++ {
			c := bgc
			if bits&(0x80>>b) != 0 {
				c = fgc
			}
			if inside {
				for yy := 0; yy < z; yy++ {
					o := (py+r*z+yy)*fb.w + px + b*z
					for xx := 0; xx < z; xx++ {
						fb.px[o+xx] = c
					}
				}
			} else {
				fb.fill(Rect{px + b*z, py + r*z, z, z}, c)
			}
		}
	}
}

// uniCellNote draws what the status line says about a cell under the mouse; it returns the x after it.
func (a *App) uniCellNote(fb *FB, x, y int, v uint32) int {
	code := uniCode(v)
	if g, ok := uniGlyph(code, a.fontFor["uni"] != "unifont"); ok && code > 32 {
		for r := 0; r < 16; r++ {
			for b := 0; b < 8; b++ {
				if g[r]&(0x80>>b) != 0 {
					fb.fill(Rect{x + b, y + r - 2, 1, 1}, cText)
				}
			}
		}
	}
	fg, bg := uniColoursOf(v)
	return fb.text(x+digitsW(2), y, fmt.Sprintf("%s fg %s bg %s", a.charLabel(code), colourName(fg), colourName(bg)), cText)
}

// charLabel names a character of the page: its number, or U+ on a Unicode page.
func (a *App) charLabel(c int) string {
	if a.uni() {
		return fmt.Sprintf("U+%04X", uniRune(c))
	}
	return fmt.Sprint(c)
}

// previewCell is a character as the F-keys and the character table show it.
func (a *App) previewCell(c int) uint32 {
	if a.uni() {
		return uniCell(c, styleOf(7, 0, a.uniFlags()))
	}
	return makeCell(c, a.previewFg(), 0, a.styleFlags())
}

// ---------------------------------------------------------------- the character table

// uniBlocks are the pages of 256 codes the character table shows (a code of
// D8xx-DBxx is a plane-1 character, see uniRune).
var uniBlocks = []struct {
	name string
	base int
}{
	{"Box drawing, blocks, shapes", 0x2500},
	{"Legacy computing: sextants, wedges", 0xD800},
	{"Braille", 0x2800},
	{"Legacy computing: octants", 0xDA00},
	{"Legacy computing: more", 0xD900},
	{"Legacy computing: more (2)", 0xDB00},
	{"Latin-1", 0x0000},
	{"Latin Extended", 0x0100},
	{"Latin Extended-B", 0x0200},
	{"Greek", 0x0300},
	{"Cyrillic", 0x0400},
	{"Punctuation, sub- and superscripts", 0x2000},
	{"Letterlike, numbers, arrows", 0x2100},
	{"Mathematical operators", 0x2200},
	{"Technical", 0x2300},
	{"Control pictures, enclosed", 0x2400},
	{"Symbols", 0x2600},
	{"Dingbats, arrows", 0x2700},
	{"Arrows, mathematics", 0x2900},
	{"Mathematics", 0x2A00},
	{"Symbols and arrows", 0x2B00},
}

// uniHeaderH is the height of the block switch above the character table.
func uniHeaderH() int { return smallH + 4 }

// setUniBlock shows block i (wrapping round) in the character table.
func (a *App) setUniBlock(i int) {
	n := len(uniBlocks)
	a.uniBlock = (i%n + n) % n
}

// uniHas: does the page's font draw this code?
func (a *App) uniHas(code int) bool {
	if code < 0x20 || (code >= 0x7F && code < 0xA0) {
		return false
	}
	_, ok := uniGlyph(code, a.fontFor["uni"] != "unifont")
	return ok
}

// drawUniCharset draws the block switch and the block's 256 codes, cols to a row,
// and returns where they are.
func (a *App) drawUniCharset(u *UI, x, y, cols int) Rect {
	fb := u.fb
	blk := uniBlocks[a.uniBlock%len(uniBlocks)]
	bw := fsetArrowW()
	if u.button(Rect{x, y, bw, smallH}, "<", false, "The block before ("+keyName("uni_block_previous")+")") != nil {
		a.setUniBlock(a.uniBlock - 1)
	}
	if u.button(Rect{x + bw + 4, y, bw, smallH}, ">", false, "The next block ("+keyName("uni_block_next")+")") != nil {
		a.setUniBlock(a.uniBlock + 1)
	}
	rs := []rune(blk.name)
	label := string(rs[:fitText(rs, 32*8-2*bw-12)])
	fb.text(x+2*bw+12, y+2, label, cText)
	u.setHint(Rect{x + 2*bw + 12, y, textW(label), smallH}, fmt.Sprintf("U+%04X to U+%04X", uniRune(max(blk.base, 0x20)), uniRune(blk.base+255)))
	top := y + uniHeaderH()
	H, CW := 16, 8
	cm := Rect{x, top, cols * CW, 256 / cols * H}
	fb.fill(cm, cPaper)
	for i := 0; i < 256; i++ {
		code := blk.base + i
		px, py := cm.x+(i%cols)*CW, cm.y+(i/cols)*H
		if a.uniHas(code) {
			a.drawCell(fb, px, py, 1, a.previewCell(code))
		} else {
			fb.fill(Rect{px + 1, py + 1, CW - 2, H - 2}, 0xFF1A1B1E)
		}
	}
	if k := a.brush - blk.base; k >= 0 && k < 256 {
		fb.frame(Rect{cm.x + (k%cols)*CW, cm.y + (k/cols)*H, CW, H}, cOrange)
	}
	if d := u.takeWheel(cm); d != 0 {
		a.setUniBlock(a.uniBlock + map[bool]int{true: 1, false: -1}[d > 0])
	}
	a.charHover = -1
	if u.hover(cm) {
		code := blk.base + (u.my-cm.y)/H*cols + (u.mx-cm.x)/CW
		a.charHover = code
		u.hint = fmt.Sprintf("%s %c. Click: brush (types it with Type). Right click: put it on the active F-key. Wheel: other blocks.", a.charLabel(code), uniRune(code))
	}
	if c := u.take(cm); c != nil {
		code := blk.base + (c.y-cm.y)/H*cols + (c.x-cm.x)/CW
		if a.uniHas(code) {
			switch {
			case c.button == 3:
				a.fkeys[a.mode][a.fset][a.factive] = code
				savePrefs()
			case a.tool == "type" && !c.shift:
				a.typeByte(code)
			default:
				a.brush = code
				if a.tool != "draw" && a.tool != "fill" && a.tool != "type" {
					a.setTool("draw")
				}
			}
		}
	}
	return Rect{x, y, max(cm.w, 2*bw+12+textW(label)), cm.h + uniHeaderH()}
}

// uniFkeys are the Unicode page's F-key sets: blocks, shades, lines, sextants, Braille.
var uniFkeys = [][]int{
	{'░', '▒', '▓', '█', '▀', '▄', '▌', '▐', '■', '·'},
	{'┌', '┐', '└', '┘', '─', '│', '├', '┤', '┬', '┴'},
	{'╔', '╗', '╚', '╝', '═', '║', '╠', '╣', '╦', '╩'},
	{'╭', '╮', '╰', '╯', '╱', '╲', '╳', '◢', '◣', '◤'},
	{'▖', '▗', '▘', '▝', '▚', '▞', '▙', '▛', '▜', '▟'},
	{0xD800, 0xD801, 0xD803, 0xD806, 0xD80E, 0xD813, 0xD81D, 0xD82C, 0xD83A, 0xD85F}, // sextants, wedges
	{'⠁', '⠃', '⠇', '⡇', '⣇', '⣧', '⣷', '⣿', '⠿', '⢸'},
	{'★', '☆', '♥', '♦', '♣', '♠', '●', '○', '◆', '◇'},
}

// validUniFkeys: saved Unicode F-key sets (codes beyond a byte).
func validUniFkeys(sets [][]int) bool {
	if len(sets) == 0 {
		return false
	}
	for _, set := range sets {
		if len(set) != 10 {
			return false
		}
		for _, c := range set {
			if c < 0x20 || c > 0xFFFF {
				return false
			}
		}
	}
	return true
}

// ---------------------------------------------------------------- colours

// drawUniColours draws xterm's colours 16-255 and a button for 24-bit colours.
// In the side panel (row) they go under the 16, w pixels wide, and it returns
// the y under them; in the strip under the page (!row) they go in one band to
// the right of the 16, and it returns the x after them. Left click: foreground,
// right: background.
func (a *App) drawUniColours(u *UI, x, y, w int, row bool) int {
	fb := u.fb
	cols, cw, ch := 18, max(4, w/18), max(4, w/18)
	if !row {
		cols, cw, ch = 48, 4, 4
	}
	for i := 0; i < 240; i++ {
		n := 16 + i
		col, r := i%cols, i/cols
		if n < 232 && cols == 18 { // the cube by its six red levels: rows of green, columns of blue
			k := n - 16
			col, r = k/36%3*6+k%6, k/108*6+k/6%6
		}
		cell := Rect{x + col*cw, y + r*ch, cw, ch}
		fb.fill(cell, 0xFF000000|xterm256(n))
		if a.ufgOn && a.ufg == n {
			fb.frame(cell.inset(-1), cHi)
		}
		if a.ubgOn && a.ubg == n {
			fb.dashed(cell.inset(-1), cOrange)
		}
		if u.hover(cell) {
			u.hint = fmt.Sprintf("Colour %d: #%06X. Left: foreground, right: background", n, xterm256(n))
		}
		if c := u.take(cell); c != nil {
			if c.button == 3 {
				a.setUniBg(n)
			} else {
				a.setUniFg(n)
			}
		}
	}
	rows := (240 + cols - 1) / cols
	bx, by := x, y+rows*ch+6
	if !row {
		bx, by = x+cols*cw+6, y
	}
	label := "24-bit..."
	if u.button(Rect{bx, by, buttonW(label), smallH}, label, a.ufg&rgbFlag != 0 && a.ufgOn || a.ubg&rgbFlag != 0 && a.ubgOn,
		"Any colour: red, green and blue for the foreground or the background ("+keyName("uni_colour")+")") != nil {
		openUniColour()
	}
	if !row {
		return bx + buttonW(label)
	}
	return by + smallH
}

// UniColourReq picks a 24-bit colour for the brush: sliders, or the hex code typed.
type UniColourReq struct {
	rgb [3]int
	hex Field
}

func openUniColour() {
	c := colourRGB(a.uniFg(), &a.pal)
	d := &UniColourReq{rgb: [3]int{int(c >> 16 & 255), int(c >> 8 & 255), int(c & 255)}, hex: Field{max: 7}}
	d.hex.enter = func() { d.setHex(); a.setUniFg(d.colour()); closeDialog() }
	a.dialog = d
}

func (d *UniColourReq) colour() int {
	return rgbColour(uint32(d.rgb[0])<<16 | uint32(d.rgb[1])<<8 | uint32(d.rgb[2]))
}

// setHex takes the typed code (#RRGGBB or RRGGBB, or #RGB) if it is one.
func (d *UniColourReq) setHex() {
	s := strings.TrimPrefix(strings.TrimSpace(d.hex.String()), "#")
	if len(s) == 3 {
		s = string([]byte{s[0], s[0], s[1], s[1], s[2], s[2]})
	}
	if v, err := strconv.ParseUint(s, 16, 32); err == nil && len(s) == 6 {
		d.rgb = [3]int{int(v >> 16 & 255), int(v >> 8 & 255), int(v & 255)}
	}
}

func (d *UniColourReq) draw(u *UI) {
	fb := u.fb
	in := u.window(centre(460, 230), "24-bit colour")
	for k, name := range []string{"Red", "Green", "Blue"} {
		y := in.y + k*(gadH+8)
		fb.text(in.x, y+3, name, cText)
		d.rgb[k] = u.slider(fmt.Sprint("urgb", k), Rect{in.x + 56, y, in.w - 56 - 100, gadH}, d.rgb[k])
		fb.text(in.x+in.w-90, y+3, fmt.Sprintf("%3d", d.rgb[k]), cText)
	}
	sw := Rect{in.x + in.w - 50, in.y, 50, 3*gadH + 16}
	fb.fill(sw, 0xFF000000|uint32(d.rgb[0])<<16|uint32(d.rgb[1])<<8|uint32(d.rgb[2]))
	fb.frame(sw, cEdgeDk)
	hy := in.y + 3*(gadH+8) + 4
	fb.text(in.x, hy+3, "Hex", cText)
	if u.focus != &d.hex {
		d.hex.set(colourName(rgbFlag | d.colour()&0xFFFFFF))
	} else {
		d.setHex()
	}
	u.field(Rect{in.x + 56, hy, textW("#DDDDDD") + 16, gadH}, &d.hex, "The colour as #RRGGBB (Return: the foreground)")
	if n := d.colour(); n&rgbFlag == 0 {
		fb.text(in.x+56+textW("#DDDDDD")+28, hy+3, fmt.Sprintf("xterm's %d", n), cDim)
	}
	switch buttonRow(u, in, "Foreground", "Background", "Cancel") {
	case 0:
		a.setUniFg(d.colour())
		closeDialog()
	case 1:
		a.setUniBg(d.colour())
		closeDialog()
	case 2:
		closeDialog()
	}
}

func (d *UniColourReq) key(k string, ctrl, shift bool) bool {
	switch k {
	case "Escape":
		closeDialog()
	case "Return", "Keypad Enter":
		a.setUniFg(d.colour())
		closeDialog()
	default:
		return false
	}
	return true
}

// uniPick takes a cell's character and colours for the brush.
func (a *App) uniPick(v uint32) {
	a.brush = uniCode(v)
	if a.brush < 0x20 {
		a.brush = 32
	}
	fg, bg := uniColoursOf(v)
	a.setUniFg(fg)
	a.setUniBg(bg)
	fl := uniFlagsOf(v)
	a.ital, a.under = fl&italic != 0, fl&underline != 0
}

// halfFg / halfBg are the colours the Half tool paints with: on a Unicode page
// any colour, elsewhere one of the 16.
func (a *App) halfFg() int {
	if a.uni() {
		return a.uniFg()
	}
	return a.fg
}

func (a *App) halfBg() int {
	if a.uni() {
		return a.uniBg()
	}
	return a.bg
}

// uniPaintHalf paints the top or bottom half of a cell in any colour with ▀ ▄ █ and spaces.
func (a *App) uniPaintHalf(x, hy, colour int) {
	y, bottom := hy>>1, hy&1
	v := a.doc.get(x, y)
	fg, bg := uniColoursOf(v)
	var top, bot int
	switch uniRune(uniCode(v)) {
	case '█':
		top, bot = fg, fg
	case '▀':
		top, bot = fg, bg
	case '▄':
		top, bot = bg, fg
	default:
		top, bot = bg, bg
	}
	if bottom == 1 {
		bot = colour
	} else {
		top = colour
	}
	cell := uniCell('▀', pairOf(top, bot))
	if top == bot { // one colour: a space on it (the background can be any colour here)
		cell = uniCell(' ', pairOf(7, top))
	}
	a.setCell(x, y, cell)
}

// ---------------------------------------------------------------- the page type

// setModeUni switches to or from a Unicode page; false when setMode should go on
// (the page left is converted to PC first, then setMode shows it as m).
func (a *App) setModeUni(m string) bool {
	clean := a.source == nil && !a.dirty && a.doc.empty() && a.nFrames() == 1
	if m == "uni" {
		if clean { // an empty page: a new Unicode page of its width
			cols := a.doc.cols
			if a.mode != "pc" && a.mode != "amiga" {
				cols = 80
			}
			a.doc = newDoc(cols, 25)
			a.mode, a.saveFmt, a.info.writeSauce = "uni", "uniansi", false
			if a.path == "" {
				a.name = "untitled.ans"
			}
			a.applyMode()
			savePrefs()
			return true
		}
		if a.mode != "pc" && a.mode != "amiga" {
			a.convertFromMachine()
			return true
		}
		a.convertUni("uni")
		savePrefs()
		return true
	}
	// Leaving a Unicode page.
	if clean {
		a.doc = newDoc(80, 25)
		a.mode, a.saveFmt = "pc", "ansi"
		if m == "pc" {
			a.applyMode()
			savePrefs()
			return true
		}
		return false // setMode makes the machine's empty page
	}
	to := m
	if m != "amiga" {
		to = "pc"
	}
	a.convertUni(to)
	return m == to
}

// convertFromMachine makes a page of another machine (PETSCII, Atari, teletext
// ...) a Unicode page: each character as Unicode shows it, colours the nearest.
func (a *App) convertFromMachine() {
	from := a.mode
	pal := a.pal
	runeOf := a.runeOf
	a.endPlay()
	cells := func(d *Doc) *Doc {
		n := newDoc(d.cols, d.rows)
		n.uni = true
		for i, v := range d.cells[:d.rows*d.cols] {
			r := runeOf(cellCh(v))
			if r < 0x20 {
				r = ' '
			}
			code, _ := uniCodeOf(r)
			fg, bg := cellFg(v), cellBg(v)
			switch {
			case from == "atari":
				fg, bg = d.ink, d.bg
			case a.screen():
				bg = d.bg
			}
			if v&0x80 != 0 && a.screen() { // reversed
				fg, bg = bg, fg
			}
			fgc, bgc := rgbColour(pal[fg&15]), rgbColour(pal[bg&15])
			n.cells[i] = uniCell(code, pairOf(fgc, bgc))
		}
		return n
	}
	a.withEveryFrame(cells)
	a.mode, a.saveFmt, a.source, a.srcKind = "uni", "uniansi", nil, ""
	a.ownFont, a.ownLower, a.ownPal = nil, nil, nil
	a.name = strings.TrimSuffix(a.name, extOf(a.name)) + ".utf8.ans"
	a.path = ""
	a.dirty = true
	a.applyMode()
	a.msg("Converted to a Unicode page in the " + modeNames[from] + " page's colours (" + keyName("undo") + " undoes it)")
}

// restoreUniMode: an undo step from before (or after) a conversion to or from
// Unicode brings its page type back with it.
func (a *App) restoreUniMode(m string) {
	if m == "" || m == a.mode || (m != "uni" && a.mode != "uni") {
		return
	}
	a.mode = m
	a.saveFmt = map[bool]string{true: "uniansi", false: "ansi"}[m == "uni"]
	if m == "c64" || m == "atari" || m == "st" || m == "zx" || m == "tt" {
		a.saveFmt = map[string]string{"c64": "seq", "atari": "ata", "st": "vt52", "zx": "scr", "tt": "tti"}[m]
	}
	a.applyMode()
}
