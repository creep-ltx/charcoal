package main

// The font editor: the brush character's pixels in a grid, drawn with the
// mouse, turned over, moved, copied; the page behind shows every change at
// once. Editing a built-in font makes a copy of it the page's own font
// (fontfile.go). Also the requesters to load and save a font, and the
// question a dropped font file asks.

import (
	"fmt"
	"math/bits"
	"os"
	"path/filepath"
	"strings"
)

// fontStep is an undo step of the font editor: a font's glyphs before a change.
type fontStep struct {
	f    *Font
	bits []byte
}

type FontEd struct {
	undo, redo []fontStep
	clip       []byte // a glyph's rows, copied
	paint      bool   // a drag sets pixels (true) or clears them
	pen        int    // multicolour: the pixel pair a click draws (1-3; 0 = none yet: 3)
	pairPaint  int    // multicolour: the pair value a drag draws
}

func openFontEditor() {
	if a.uni() {
		a.msg("A Unicode page draws with Unifont (and VGA): fonts of 256 characters are for the other pages")
		return
	}
	if a.st() || a.tt() || a.cpc() || a.mt() || a.mz() {
		a.msg("An " + modeNames[a.mode] + " page keeps its own font")
		return
	}
	a.dialog = &FontEd{}
}

// editableFont is the page's own font, made first as a copy of the font the
// page shows (on a PETSCII page both of the machine's sets, so the case can still change).
func (a *App) editableFont() *Font {
	if f := a.ownInUse(); f != nil {
		return f
	}
	cur, H := a.font(), a.cellH()
	own := func(f *Font, id string) *Font {
		g := f.clone()
		g.id, g.name, g.src, g.format, g.h = id, f.name+" (edited)", "", "", H
		return g
	}
	if a.c64() {
		fam := a.mach().family
		up, low := fontByID(fam+"upper"), fontByID(fam+"lower")
		if up == nil || low == nil {
			up, low = cur, nil
		}
		ou := own(up, ownID)
		var ol *Font
		if low != nil {
			ol = own(low, ownLowerID)
		}
		a.useOwnFont(ou, ol, false)
		return a.ownInUse()
	}
	a.useOwnFont(own(cur, ownID), nil, false)
	return a.ownFont
}

// change does one undoable change to the page's font.
func (d *FontEd) change(fn func(f *Font)) {
	f := a.editableFont()
	d.undo = append(d.undo, fontStep{f, append([]byte(nil), f.bits...)})
	if len(d.undo) > 300 {
		d.undo = d.undo[1:]
	}
	d.redo = nil
	fn(f)
	f.edited()
	a.fontChanged()
}

func (d *FontEd) step(from, to *[]fontStep) {
	if len(*from) == 0 {
		return
	}
	s := (*from)[len(*from)-1]
	*from = (*from)[:len(*from)-1]
	*to = append(*to, fontStep{s.f, append([]byte(nil), s.f.bits...)})
	copy(s.f.bits, s.bits)
	s.f.edited()
	a.fontChanged()
}

func (d *FontEd) undoStep() { d.step(&d.undo, &d.redo) }
func (d *FontEd) redoStep() { d.step(&d.redo, &d.undo) }

// The glyph operations, on character c of f.
func glyphInvert(f *Font, c int) {
	for r := 0; r < f.rows(); r++ {
		f.setRow(c, r, ^f.row(c, r))
	}
}

func glyphFlipH(f *Font, c int) {
	for r := 0; r < f.rows(); r++ {
		f.setRow(c, r, bits.Reverse8(f.row(c, r)))
	}
}

func glyphFlipV(f *Font, c int) {
	h := f.rows()
	for r := 0; r < h/2; r++ {
		top, bottom := f.row(c, r), f.row(c, h-1-r)
		f.setRow(c, r, bottom)
		f.setRow(c, h-1-r, top)
	}
}

func glyphClear(f *Font, c int) {
	for r := 0; r < f.rows(); r++ {
		f.setRow(c, r, 0)
	}
}

// glyphShift moves the pixels by dx, dy; what goes out comes in on the other side.
func glyphShift(f *Font, c, dx, dy int) {
	h := f.rows()
	rows := make([]byte, h)
	for r := range rows {
		rows[r] = bits.RotateLeft8(f.row(c, r), -dx)
	}
	for r := range rows {
		f.setRow(c, ((r+dy)%h+h)%h, rows[r])
	}
}

func glyphRows(f *Font, c int) []byte {
	out := make([]byte, f.rows())
	for r := range out {
		out[r] = f.row(c, r)
	}
	return out
}

func glyphSetRows(f *Font, c int, rows []byte) {
	for r := 0; r < min(f.rows(), len(rows)); r++ {
		f.setRow(c, r, rows[r])
	}
}

func glyphPixel(f *Font, c, x, y int) bool { return f.row(c, y)&(0x80>>x) != 0 }

func glyphSetPixel(f *Font, c, x, y int, on bool) {
	v := f.row(c, y) &^ (0x80 >> x)
	if on {
		v |= 0x80 >> x
	}
	f.setRow(c, y, v)
}

// shownRow is row r of character c as the page shows it, H rows a cell (the
// font may be one of 16 rows shown at 8).
func shownRow(f *Font, c, r, H int) byte {
	if H == 8 {
		return f.bits[(c&255)*16+r*2]
	}
	return f.bits[(c&255)*16+r]
}

func (d *FontEd) op(fn func(f *Font, c int)) {
	c := a.fontEdChar()
	d.change(func(f *Font) { fn(f, c) })
}

func (d *FontEd) copyGlyph() {
	f, H := a.font(), a.cellH()
	d.clip = make([]byte, H)
	for r := range d.clip {
		d.clip[r] = shownRow(f, a.brush, r, H)
	}
	a.msg(fmt.Sprintf("Character %d copied", a.brush))
}

func (d *FontEd) pasteGlyph() {
	if d.clip == nil {
		a.msg("Copy a character first (" + "Ctrl+C in the font editor)")
		return
	}
	rows := d.clip
	d.op(func(f *Font, c int) { glyphSetRows(f, c, rows) })
}

func (d *FontEd) draw(u *UI) {
	fb := u.fb
	f, H, W := a.font(), a.cellH(), a.cellW()
	c := a.fontEdChar()
	// The window stands at the right, so the page shows beside it; the grid
	// is as big as the room left by the gadgets and the character map under it.
	fixed := lineH + 24 + lineH + 8 + 10 + 3*(gadH+6) + 4 + 8*H + 16 + gadH + 12
	maxCS := 24
	if H <= 8 {
		maxCS = 32
	}
	if a.fontEdPairs() { // the row of pens
		fixed += gadH + 6
	}
	cs := max(6, min(maxCS, (fb.h-48-fixed)/H))
	w, h := 540, min(fb.h-48, fixed+H*cs)
	r := Rect{fb.w - w - 12, 40, w, h}
	if fb.w < w+300 {
		r = centre(w, h)
	}
	in := u.window(r, "Font: "+f.name)
	y := in.y
	fb.text(in.x, y, fmt.Sprintf("Character %d (0x%02X), 8x%d", c, c, H), cHi)
	if ch := a.charName(c); ch != "" {
		fb.text(in.x+textW("Character 255 (0xFF), 8x16")+16, y, ch, cText)
	}
	y += lineH + 8

	g := Rect{in.x, y, 8 * cs, H * cs}
	fb.fill(g, cPaper)
	if a.fontEdPairs() { // multicolour: pixel pairs in their colours (c64x.go)
		d.drawPairs(u, f, c, g, cs)
	} else {
		d.drawPixels(u, f, c, g, cs, H)
	}

	// Beside the grid: the character at the page's size and bigger, alone and
	// as a 3x3 block (how it joins its neighbours).
	cell := a.cell(c)&^255 | uint32(c)
	if a.screen() {
		cell = makeCell(c, a.previewFg(), 0, 0)
	}
	if a.vicMode() != vicHires { // in the brush's colour: multicolour shows only in 8-15
		cell = a.cell(c)
	}
	px := g.x + g.w + 20
	py := g.y
	for _, z := range []int{1, 2, 4} {
		a.drawCell(fb, px, py, z, cell)
		px += W*z + 12
	}
	py += H*4 + 12
	px = g.x + g.w + 20
	for ty := 0; ty < 3; ty++ {
		for tx := 0; tx < 3; tx++ {
			a.drawCell(fb, px+tx*W*2, py+ty*H*2, 2, cell)
		}
	}
	y = g.y + g.h + 10
	d.gadgets(u, in, c, y, W, H)
}

// drawPixels is the grid of a character's pixels, drawn on with the mouse.
func (d *FontEd) drawPixels(u *UI, f *Font, c int, g Rect, cs, H int) {
	fb := u.fb
	for py := 0; py < H; py++ {
		v := shownRow(f, c, py, H)
		for px := 0; px < 8; px++ {
			col := uint32(0xFF16171A)
			if v&(0x80>>px) != 0 {
				col = cHi
			}
			fb.fill(Rect{g.x + px*cs + 1, g.y + py*cs + 1, cs - 1, cs - 1}, col)
		}
	}
	fb.frame(Rect{g.x - 1, g.y - 1, g.w + 2, g.h + 2}, cEdgeDk)
	u.setHint(g, "Left click or drag: set pixels (a set pixel clicked clears); right: clear")
	at := func(x, y int) (int, int) { return (x - g.x) / cs, (y - g.y) / cs }
	if cl := u.take(g); cl != nil {
		px, py := at(cl.x, cl.y)
		d.paint = cl.button != 3 && !(shownRow(f, c, py, H)&(0x80>>px) != 0)
		on := d.paint
		d.change(func(f *Font) { glyphSetPixel(f, c, px, py, on) })
		u.drag = "fontgrid"
	} else if u.drag == "fontgrid" && g.has(u.mx, u.my) {
		px, py := at(u.mx, u.my)
		if of := a.ownInUse(); of != nil && glyphPixel(of, c, px, py) != d.paint {
			glyphSetPixel(of, c, px, py, d.paint) // the same undo step as the click
			of.edited()
			a.fontChanged()
		}
	}
}

// gadgets are the buttons under the grid, the whole font and the requester's buttons.
func (d *FontEd) gadgets(u *UI, in Rect, c, y, W, H int) {
	fb := u.fb
	bx := in.x
	btn := func(label, hint string) bool {
		bw := buttonW(label)
		hit := u.button(Rect{bx, y, bw, gadH}, label, false, hint) != nil
		bx += bw + 6
		return hit
	}
	row := func() { bx, y = in.x, y+gadH+6 }
	if btn("<", "The character before ("+"Left)") {
		a.brush = (c + 255) & 255
	}
	if btn(">", "The next character (Right)") {
		a.brush = (c + 1) & 255
	}
	bx += 10
	if btn("Invert", "Invert the character (I)") {
		d.op(glyphInvert)
	}
	if btn("Flip H", "Mirror it left-right (H)") {
		d.op(glyphFlipH)
	}
	if btn("Flip V", "Turn it upside down (V)") {
		d.op(glyphFlipV)
	}
	if btn("Clear", "Clear it (Delete)") {
		d.op(glyphClear)
	}
	row()
	fb.text(bx, y+3, "Move", cText)
	bx += textW("Move") + 8
	for _, m := range []struct {
		label  string
		dx, dy int
	}{{"Left", -1, 0}, {"Right", 1, 0}, {"Up", 0, -1}, {"Down", 0, 1}} {
		if btn(m.label, "Move the pixels "+strings.ToLower(m.label)+" (Shift+"+m.label+"); what goes out comes in on the other side") {
			dx, dy := m.dx, m.dy
			d.op(func(f *Font, c int) { glyphShift(f, c, dx, dy) })
		}
	}
	row()
	if a.fontEdPairs() { // the pixel pair a click draws
		d.penGadgets(u, &bx, y)
		row()
	}
	if btn("Copy", "Copy the character's pixels (Ctrl+C)") {
		d.copyGlyph()
	}
	if btn("Paste", "Paste them onto this character (Ctrl+V)") {
		d.pasteGlyph()
	}
	bx += 10
	if btn("Undo", "Undo (Ctrl+Z)") {
		d.undoStep()
	}
	if btn("Redo", "Redo (Ctrl+Y)") {
		d.redoStep()
	}
	y += gadH + 10

	// The whole font: a click picks the character to edit (and the brush).
	cm := Rect{in.x, y, 32 * W, 8 * H}
	fb.fill(cm, cPaper)
	for ch := 0; ch < 256; ch++ {
		v := a.cell(ch)&^255 | uint32(ch)
		if a.screen() {
			v = makeCell(ch, a.previewFg(), 0, 0)
		}
		a.drawCell(fb, cm.x+(ch%32)*W, cm.y+(ch/32)*H, 1, v)
	}
	fb.frame(Rect{cm.x + (c%32)*W, cm.y + (c/32)*H, W, H}, cOrange)
	u.setHint(cm, "Click a character to edit it")
	if cl := u.take(cm); cl != nil {
		a.brush = (cl.y-cm.y)/H*32 + (cl.x-cm.x)/W
	}
	fb.text(cm.x+cm.w+16, cm.y, fmt.Sprintf("%d undo steps", len(d.undo)), cText)
	if a.ownInUse() == nil {
		fb.text(cm.x+cm.w+16, cm.y+lineH, "Built in: a change", cText)
		fb.text(cm.x+cm.w+16, cm.y+2*lineH, "makes a copy", cText)
	}

	switch buttonRow(u, in, "Load font...", "Save font...", "Close") {
	case 0:
		openFontRequester()
		if fr, ok := a.dialog.(*FileReq); ok {
			fr.back = d
		}
	case 1:
		openFontSaveRequester(d)
	case 2:
		closeDialog()
	}
}

// charName is a few words about a character of the page's set, "" if none.
func (a *App) charName(c int) string {
	switch {
	case a.amiga() && c >= 32 && c < 127, a.amiga() && c >= 0xA0:
		return string(rune(c))
	case a.pc() && c >= 32 && c < 127:
		return string(rune(c))
	}
	return ""
}

func (d *FontEd) key(k string, ctrl, shift bool) bool {
	c := a.brush & 255
	switch {
	case ctrl && k == "Z" && shift, ctrl && k == "Y":
		d.redoStep()
	case ctrl && k == "Z":
		d.undoStep()
	case ctrl && k == "C":
		d.copyGlyph()
	case ctrl && k == "V":
		d.pasteGlyph()
	case shift && k == "Left":
		d.op(func(f *Font, c int) { glyphShift(f, c, -1, 0) })
	case shift && k == "Right":
		d.op(func(f *Font, c int) { glyphShift(f, c, 1, 0) })
	case shift && k == "Up":
		d.op(func(f *Font, c int) { glyphShift(f, c, 0, -1) })
	case shift && k == "Down":
		d.op(func(f *Font, c int) { glyphShift(f, c, 0, 1) })
	case k == "Left":
		a.brush = (c + 255) & 255
	case k == "Right":
		a.brush = (c + 1) & 255
	case k == "Up":
		a.brush = (c + 256 - 32) & 255
	case k == "Down":
		a.brush = (c + 32) & 255
	case ctrl:
		return false
	case k == "I":
		d.op(glyphInvert)
	case k == "H":
		d.op(glyphFlipH)
	case k == "V":
		d.op(glyphFlipV)
	case k == "Delete" || k == "Backspace":
		d.op(glyphClear)
	case a.fontEdPairs() && len(k) == 1 && k[0] >= '1' && k[0] <= '3':
		d.pen = int(k[0] - '0')
	case k == "Escape" || k == "Return" || k == "Keypad Enter":
		closeDialog()
	default:
		return false
	}
	return true
}

// ---------------------------------------------------------------- loading and saving fonts

// fontFilesNote says which font files the page takes.
func (a *App) fontFilesNote() string {
	switch a.mode {
	case "c64":
		return "C64 character sets: .64c, .bin (2048 bytes a set, 4096 both)"
	case "atari":
		return "Atari fonts: .fnt (1024 bytes)"
	case "zx":
		return "Spectrum: .ch8 / .bin (768 bytes, codes 32-127), UDGs (168 bytes)"
	}
	return "Fonts: .fnt .f08 .f14 .f16 (raw, 256 characters), .psf, an XBin's font"
}

func openFontRequester() {
	if a.uni() {
		a.msg("A Unicode page draws with Unifont (and VGA): fonts of 256 characters are for the other pages")
		return
	}
	if a.st() || a.tt() || a.cpc() || a.mt() || a.mz() {
		a.msg("An " + modeNames[a.mode] + " page keeps its own font")
		return
	}
	d := &FileReq{sel: -1, title: "Load a font for this page", note: a.fontFilesNote()}
	dir := a.lastDir
	if f := a.ownInUse(); f != nil && f.src != "" {
		dir = filepath.Dir(f.src)
	}
	d.listFirst(startDir(dir))
	d.path.enter = d.enter
	d.pick = func(p string) {
		a.loadFontFile(p)
		if d.back != nil {
			a.dialog = d.back
		}
	}
	a.dialog = d
	ui.focus = &d.path
}

// openFontSaveRequester asks where to save the page's font; back is the font editor to return to.
func openFontSaveRequester(back Dialog) {
	f := a.font()
	up, low := f, (*Font)(nil)
	if own := a.ownInUse(); own != nil && a.c64() {
		up, low = a.ownFont, a.ownLower
	}
	d := &FileReq{save: true, sel: -1, title: "Save the font", note: "The name's ending says the kind: .psf, .f08/.f14/.f16, .64c, .fnt, .ch8, .udg"}
	dir := a.lastDir
	if f.src != "" {
		dir = filepath.Dir(f.src)
	}
	d.listFirst(startDir(dir))
	name := strings.TrimSuffix(f.name, " (edited)")
	name = strings.NewReplacer("/", "-", "\\", "-", ":", "-", " ", "_").Replace(name)
	d.path.set(filepath.Join(d.dir, name+fontExt(f)))
	d.path.enter = d.enter
	d.pick = func(p string) {
		write := func() {
			if ext := strings.ToLower(filepath.Ext(p)); ext == "" {
				p += fontExt(f)
			}
			data, err := fontFileBytes(up, low, filepath.Ext(p))
			if err == nil {
				err = writeAtomic(p, data)
			}
			if err != nil {
				a.msg("Could not save the font: " + err.Error())
			} else {
				a.msg(fmt.Sprintf("Saved the font as %s (%d bytes)", p, len(data)))
				if own := a.ownInUse(); own != nil {
					own.src = p
					own.name = strings.TrimSuffix(filepath.Base(p), filepath.Ext(p))
				}
			}
			if back != nil {
				a.dialog = back
			}
		}
		if _, err := os.Stat(p); err == nil {
			a.dialog = &Confirm{text: "\"" + filepath.Base(p) + "\" is already there. Replace it?", button: "Replace", yes: write}
			return
		}
		write()
	}
	a.dialog = d
	ui.focus = &d.path
}

// dropFile opens a dropped file; one that looks like a font asks first.
func dropFile(path string) {
	st, err := os.Stat(path)
	if err != nil || st.IsDir() || st.Size() > 1<<20 {
		a.openPath(path, "auto")
		return
	}
	b, err := os.ReadFile(path)
	if err != nil || !looksLikeFont(path, b) {
		a.openPath(path, "auto")
		return
	}
	_, _, perr := parseFontFile(path, b, a.mode, a.font())
	a.dialog = &FontAsk{path: path, err: perr}
}

// FontAsk asks what a dropped font file is for: the page's font, or art to open.
type FontAsk struct {
	path string
	err  error // why it cannot be this page's font
}

func (d *FontAsk) use() {
	closeDialog()
	a.loadFontFile(d.path)
}

func (d *FontAsk) art() {
	closeDialog()
	a.openPath(d.path, "auto")
}

func (d *FontAsk) draw(u *UI) {
	in := u.window(centre(560, 170), "A font?")
	text := filepath.Base(d.path) + " looks like a font. Use it as this page's font, or open it as art?"
	if d.err != nil {
		text = filepath.Base(d.path) + " looks like a font, but not one for this page: " + d.err.Error()
	}
	for i, l := range wrap(text, in.w) {
		u.fb.text(in.x, in.y+i*lineH, l, cText)
	}
	labels := []string{"Use as font", "Open as art", "Cancel"}
	if d.err != nil {
		labels = labels[1:]
	}
	switch i := buttonRow(u, in, labels...); {
	case i < 0:
	case labels[i] == "Use as font":
		d.use()
	case labels[i] == "Open as art":
		d.art()
	default:
		closeDialog()
	}
}

func (d *FontAsk) key(k string, ctrl, shift bool) bool {
	switch k {
	case "Return", "Keypad Enter", "U":
		if d.err == nil {
			d.use()
		}
	case "A", "O":
		d.art()
	case "Escape":
		closeDialog()
	default:
		return false
	}
	return true
}
