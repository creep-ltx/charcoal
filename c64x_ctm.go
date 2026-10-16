package main

// CharPad's project files (.ctm, versions 5 to 9; CharPad 2.0 to CharPad C64
// Pro 3.x), read for their character set (loaded as the page's font, with
// the project's mode and colours) or opened as a page (the map, its tiles
// laid out as characters).
//
// Sources: CharPad 2.7's manual, "CTM File Format, Version 5/6/7" (the copies
// in github.com/martinpiper/C64Public, ExternalTools/CharPad Free Edition
// 2.7.2/Help); versions 8, 8.2 ("R", 82) and 9 as C64Studio's
// CharpadProject.cs and emoon/retro-image's charpad/blocks.rs read them (the
// latter after c64lib's reader, MIT). Checked on c64lib's test projects
// (testdata/charpad: the same picture in versions 5 to 8.2).
//
// Version 5: a 20-byte header (colours, colouring method, flags: b0 tiles, b1
// tiles "expanded", b2 multicolour; counts and sizes), then characters,
// one attribute byte per character (colour in the low nybble), tiles, tile
// colours, the map. Versions 6+: a short header, then blocks, each after a
// marker $DA $Bn (n counting from 0): characters (count-1, 8 bytes each),
// attributes (6, 7) or materials (8+), per-character colours (8+, colouring
// per character), tiles (count-1, width, height, 16-bit cells), tile colours
// (per tile), tile tags, tile names (zero-ended), the map (width, height,
// 16-bit cells). Colouring: 0 one colour for all, 1 per tile, 2 per
// character. Screen modes (7+): 0 hi-res, 1 multicolour, 2 ECM, 3 and 4
// bitmaps (not text: refused).

import (
	"encoding/binary"
	"fmt"
	"path/filepath"
	"strings"
)

// ctmProject is what CharEdit takes from a CharPad project.
type ctmProject struct {
	version          int
	mode             int    // vicHires, vicMulti, vicECM
	bg               int    // $D021
	regs             [3]int // $D022-$D024
	chars            []byte // 8 bytes a character
	colouring        int    // 0 global, 1 per tile, 2 per character
	charColour       int    // with global colouring
	charColours      []byte // per character (low nybble)
	tileW, tileH     int    // 1x1 without tiles
	tiles            []int  // character of each tile cell, tile after tile (nil: tile = character)
	tileColours      []byte // per tile
	mapW, mapH       int
	mapCells         []int
	tiled, expanded  bool
	colourStride     int // bytes a colour entry (the 8 prerelease: 4, colour last)
	colourOffset     int
	separateMaterial bool
}

// ctmReader reads a CharPad file with bounds checked.
type ctmReader struct {
	b   []byte
	pos int
	err error
}

func (r *ctmReader) take(n int) []byte {
	if r.err != nil || n < 0 || r.pos+n > len(r.b) {
		if r.err == nil {
			r.err = fmt.Errorf("the CharPad file ends early")
		}
		return make([]byte, max(0, min(n, 1<<20)))
	}
	out := r.b[r.pos : r.pos+n]
	r.pos += n
	return out
}

func (r *ctmReader) byte() int { return int(r.take(1)[0]) }
func (r *ctmReader) word() int { return int(binary.LittleEndian.Uint16(r.take(2))) }
func (r *ctmReader) words(n int) []int {
	b := r.take(2 * n)
	out := make([]int, len(b)/2)
	for i := range out {
		out[i] = int(binary.LittleEndian.Uint16(b[2*i:]))
	}
	return out
}

// ctmMax caps what a file may ask for (CharPad's own limits: 8192x8192 map, 10x10 tiles).
const ctmMaxCells = 4 << 20

func isCTM(b []byte) bool {
	return len(b) >= 10 && string(b[:3]) == "CTM" && (b[3] >= 5 && b[3] <= 9 || b[3] == 82)
}

// parseCTM reads a CharPad project (versions 5-9).
func parseCTM(b []byte) (*ctmProject, error) {
	if !isCTM(b) {
		if len(b) >= 4 && string(b[:3]) == "CTM" {
			return nil, fmt.Errorf("CharPad file version %d: versions 5 to 9 can be read", b[3])
		}
		return nil, fmt.Errorf("not a CharPad file")
	}
	p := &ctmProject{version: int(b[3]), colourStride: 1}
	r := &ctmReader{b: b, pos: 4}
	switch p.version {
	case 5:
		return parseCTM5(p, r)
	case 6:
		h := r.take(6)
		p.bg, p.regs[0], p.regs[1], p.charColour, p.colouring = int(h[0]), int(h[1]), int(h[2]), int(h[3]), int(h[4])
		p.mode = map[bool]int{true: vicMulti}[h[5]&1 != 0]
		p.tiled = h[5]&2 != 0
	case 7:
		h := r.take(8)
		p.bg, p.regs[0], p.regs[1], p.regs[2], p.charColour, p.colouring = int(h[0]), int(h[1]), int(h[2]), int(h[3]), int(h[4]), int(h[5])
		p.mode, p.tiled = int(h[6]), h[7]&1 != 0
	default: // 8, 8.2 and 9
		h := r.take(3)
		p.mode, p.colouring, p.tiled = int(h[0]), int(h[1]), h[2]&1 != 0
		p.separateMaterial = true
		if p.version == 9 {
			c := r.take(12) // the flexigrid's size and a byte not used, then the colours
			p.bg, p.regs[0], p.regs[1], p.regs[2], p.charColour = int(c[5]), int(c[6]), int(c[7]), int(c[8]), int(c[9])
		} else {
			c := r.take(7)
			p.bg, p.regs[0], p.regs[1], p.regs[2], p.charColour = int(c[0]), int(c[1]), int(c[2]), int(c[3]), int(c[4])
			if r.pos < len(b) && b[r.pos] != 0xDA { // a prerelease 8: the character colour follows, colour entries are 4 bytes
				p.charColour = r.byte()
				p.colourStride, p.colourOffset = 4, 3
			}
		}
	}
	if p.mode > vicECM {
		return nil, fmt.Errorf("the CharPad project is a bitmap, not characters")
	}
	if p.colouring > 2 || (p.colouring == 1 && !p.tiled) {
		return nil, fmt.Errorf("the CharPad file's colouring method %d is not known", p.colouring)
	}
	next := 0
	marker := func() {
		m := r.take(2)
		if r.err == nil && (m[0] != 0xDA || m[1] != byte(0xB0+next&15)) {
			r.err = fmt.Errorf("the CharPad file's block %d is not where it should be", next)
		}
		next++
	}
	marker()
	n := r.word() + 1
	p.chars = r.take(8 * n)
	marker()
	attrs := r.take(n)
	if !p.separateMaterial && p.colouring == 2 {
		p.charColours = attrs
	}
	if p.separateMaterial && p.colouring == 2 {
		marker()
		p.charColours = p.colourTable(r.take(n * p.colourStride))
	}
	if p.tiled {
		marker()
		nt := r.word() + 1
		p.tileW, p.tileH = r.byte(), r.byte()
		if p.tileW == 0 || p.tileH == 0 {
			return nil, fmt.Errorf("the CharPad file's tiles have no size")
		}
		if p.tileW*p.tileH*nt > ctmMaxCells {
			return nil, fmt.Errorf("the CharPad file's tiles are too large")
		}
		p.tiles = r.words(p.tileW * p.tileH * nt)
		if p.colouring == 1 {
			marker()
			p.tileColours = p.colourTable(r.take(nt * p.colourStride))
		}
		marker()
		r.take(nt) // tags
		marker()
		for i := 0; i < nt && r.err == nil; i++ { // names
			for r.byte() != 0 && r.err == nil {
			}
		}
	} else {
		p.tileW, p.tileH = 1, 1
	}
	marker()
	p.mapW, p.mapH = r.word(), r.word()
	if p.mapW*p.mapH > ctmMaxCells {
		return nil, fmt.Errorf("the CharPad file's map is too large")
	}
	p.mapCells = r.words(p.mapW * p.mapH)
	if r.err != nil {
		return nil, r.err
	}
	return p, nil
}

// colourTable takes the colour of each entry of a colour block.
func (p *ctmProject) colourTable(b []byte) []byte {
	if p.colourStride == 1 {
		return b
	}
	out := make([]byte, len(b)/p.colourStride)
	for i := range out {
		out[i] = b[i*p.colourStride+p.colourOffset]
	}
	return out
}

func parseCTM5(p *ctmProject, r *ctmReader) (*ctmProject, error) {
	h := r.take(16)
	p.bg, p.regs[0], p.regs[1], p.charColour, p.colouring = int(h[0]), int(h[1]), int(h[2]), int(h[3]), int(h[4])
	flags := h[5]
	p.tiled, p.expanded = flags&1 != 0, flags&2 != 0
	if flags&4 != 0 {
		p.mode = vicMulti
	}
	n := int(binary.LittleEndian.Uint16(h[6:])) + 1
	nt := int(binary.LittleEndian.Uint16(h[8:])) + 1
	p.tileW, p.tileH = int(h[10]), int(h[11])
	p.mapW, p.mapH = int(binary.LittleEndian.Uint16(h[12:])), int(binary.LittleEndian.Uint16(h[14:]))
	if !p.tiled {
		p.tileW, p.tileH = 1, 1
	}
	if p.tileW == 0 || p.tileH == 0 {
		return nil, fmt.Errorf("the CharPad file's tiles have no size")
	}
	if p.mapW*p.mapH > ctmMaxCells || p.tileW*p.tileH*nt > ctmMaxCells {
		return nil, fmt.Errorf("the CharPad file's map is too large")
	}
	p.chars = r.take(8 * n)
	attrs := r.take(n)
	if p.colouring == 2 {
		p.charColours = attrs
	}
	if p.tiled {
		if !p.expanded {
			p.tiles = r.words(p.tileW * p.tileH * nt)
		}
		if p.colouring == 1 {
			p.tileColours = r.take(nt)
		}
	}
	p.mapCells = r.words(p.mapW * p.mapH)
	if r.err != nil {
		return nil, r.err
	}
	return p, nil
}

// font is the project's first 256 characters as the page's own font.
func (p *ctmProject) font(name string) *Font {
	f := newFont(ownID, name, "c64", p.chars, 8, min(256, len(p.chars)/8))
	f.format = "ctm"
	return f
}

// vic is the project's screen set-up.
func (p *ctmProject) vic() vicSetup {
	v := vicSetup{Mode: p.mode}
	for i, c := range p.regs {
		v.Regs[i] = c & 15
	}
	return v
}

// page lays the map out as characters: each tile's cells, the colour by the
// project's colouring method. note says what did not fit.
func (p *ctmProject) page() (d *Doc, note string) {
	w, h := p.mapW*p.tileW, p.mapH*p.tileH
	if w == 0 || h == 0 {
		w, h = 40, 25
	}
	d = newDoc(40, max(1, min(h, maxRowsFor(40))))
	d.bg, d.glyph0 = p.bg&15, true
	big := 0
	for y := 0; y < min(h, d.rows) && p.mapW > 0 && p.mapH > 0; y++ {
		for x := 0; x < min(w, 40); x++ {
			mx, my, tx, ty := x/p.tileW, y/p.tileH, x%p.tileW, y%p.tileH
			t := p.mapCells[my*p.mapW+mx]
			ch := t
			if p.tiled {
				i := t*p.tileW*p.tileH + ty*p.tileW + tx
				switch {
				case p.tiles == nil: // expanded: tile t is characters t*cells ...
					ch = i
				case i < len(p.tiles):
					ch = p.tiles[i]
				default:
					ch = 0
				}
			}
			col := p.charColour
			switch {
			case p.colouring == 2 && ch < len(p.charColours):
				col = int(p.charColours[ch])
			case p.colouring == 1 && t < len(p.tileColours):
				col = int(p.tileColours[t])
			}
			if ch > 255 {
				big++
			}
			d.set(x, y, makeCell(ch&255, col&15, 0, 0))
		}
	}
	var notes []string
	if w > 40 {
		notes = append(notes, fmt.Sprintf("the map is %d characters wide: the first 40 are shown", w))
	}
	if n := len(p.chars) / 8; n > 256 {
		notes = append(notes, fmt.Sprintf("%d characters: the first 256 are the font", n))
	}
	if big > 0 {
		notes = append(notes, fmt.Sprintf("%d cells name characters past 255", big))
	}
	return d, strings.Join(notes, "; ")
}

// loadCTM opens a CharPad project as a C64 page (from loadC64x).
func (a *App) loadCTM(b []byte, name, path string) {
	p, err := parseCTM(b)
	if err != nil {
		a.msg("Could not read " + name + ": " + err.Error())
		return
	}
	a.mode, a.srcKind = "c64", "ctm"
	if id := a.mach().id; id != "c64" && id != "c128" {
		a.machine = "c64"
	}
	a.finishLoad(b, name, path, nil, "prg")
	// CharEdit does not write .ctm: the page is a new C64 program, so Ctrl+S asks where.
	a.path, a.name = "", strings.TrimSuffix(name, filepath.Ext(name))+".prg"
	a.vic = p.vic()
	a.useOwnFont(p.font(strings.TrimSuffix(name, filepath.Ext(name))), nil, false)
	_, note := p.page()
	msg := fmt.Sprintf("%s: CharPad %s, %s; Save as writes it as a C64 program", name, ctmVersion(p.version), strings.ToLower(vicModeNames[p.mode]))
	if note != "" {
		msg += " (" + note + ")"
	}
	a.msg(msg)
}

func ctmVersion(v int) string {
	if v == 82 {
		return "CTM 8.2"
	}
	return fmt.Sprintf("CTM %d", v)
}

// reparseCTM reads a page opened from a CharPad file again.
func (a *App) reparseCTM() bool {
	if a.srcKind != "ctm" || !a.c64() {
		return false
	}
	if p, err := parseCTM(a.source); err == nil {
		a.doc, _ = p.page()
	} else {
		a.doc = newDoc(40, 25)
	}
	a.cx, a.cy, a.sel, a.float = 0, 0, nil, nil
	return true
}

// loadC64x opens the files of this area: a .d64 and a CharPad project. It is
// called for every file first (from loadBytes), and gives the tab the
// normal screen set-up before any file sets its own.
func (a *App) loadC64x(b []byte, name, path, as string) bool {
	a.vic = defaultVIC
	if as != "auto" && as != "c64" {
		return false
	}
	if !isCTM(b) {
		return false
	}
	if !a.hasC64 {
		t := a.Tab
		openC64Req(func() {
			if a.backTo(t) {
				a.loadBytes(b, name, path, as)
			}
		})
		return true
	}
	a.loadCTM(b, name, path)
	return true
}

// ctmFontLoaded: a CharPad file loaded as the page's font also brings its mode and colours.
func (a *App) ctmFontLoaded(path string, b []byte) {
	if !a.c64() || !strings.EqualFold(filepath.Ext(path), ".ctm") {
		return
	}
	if p, err := parseCTM(b); err == nil {
		a.vic = p.vic()
		a.doc.bg = p.bg & 15
		a.msg(fmt.Sprintf("Font %s (%d characters) for this page, %s; %s", filepath.Base(path), min(256, len(p.chars)/8),
			strings.ToLower(vicModeNames[p.mode]), a.fontKeptHint()))
	}
}
