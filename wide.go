package main

// Wide characters on the Unicode page (docs/wide-design.md): a character a
// terminal shows two columns wide (CJK, kana, Hangul, fullwidth forms, most
// emoji) takes two cells, a lead (the character) and a continuation (code
// uniCont, in the lead's style). The rule everywhere is the terminal's: writing
// into either half of a wide character leaves a space in the other half.

import (
	"bufio"
	"bytes"
	"compress/gzip"
	_ "embed"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// uniCont is the code of a wide character's right half. The surrogates are
// never characters, and no cell held this one before wide characters.
const uniCont = 0xDFFF

// uniWideRune: does a terminal show r two columns wide? East Asian Width W or F
// (widetab.go, our own table: never Go's, whose Unicode version changes with
// the toolchain). Ambiguous (box drawing, blocks) is narrow.
func uniWideRune(r rune) bool {
	if r < 0x1100 {
		return false
	}
	i := sort.Search(len(uniWideRanges), func(i int) bool { return uniWideRanges[i][1] >= r })
	return i < len(uniWideRanges) && uniWideRanges[i][0] <= r
}

// uniWideCode: is code a wide character's code?
func uniWideCode(code int) bool { return code != uniCont && uniWideRune(uniRune(code)) }

// uniIsLead / uniIsCont: is a Unicode cell a wide character / a wide character's right half?
func uniIsLead(v uint32) bool { return uniWideCode(uniCode(v)) }
func uniIsCont(v uint32) bool { return uniCode(v) == uniCont }

// uniContOf is the right half of a lead; uniSpaceOf a space in a cell's style.
func uniContOf(lead uint32) uint32 { return uniCell(uniCont, uniPair(lead)) }
func uniSpaceOf(v uint32) uint32   { return uniCell(32, uniPair(v)) }

// wideNormalRow applies the terminal's rule to cells x0..x1 of row y: a wide
// character without its right half, or a right half without its character,
// becomes a space. It returns how many cells it changed.
func wideNormalRow(d *Doc, y, x0, x1 int) int {
	if !d.uni || y < 0 || y >= d.rows {
		return 0
	}
	x0, x1 = max(0, x0), min(d.cols-1, x1)
	row := d.cells[y*d.cols : (y+1)*d.cols]
	n := 0
	for x := x0; x <= x1; x++ {
		v := row[x]
		switch {
		case uniIsCont(v) && (x == 0 || !uniIsLead(row[x-1])):
			row[x] = uniSpaceOf(v)
			n++
		case uniIsLead(v) && (x+1 >= d.cols || !uniIsCont(row[x+1])):
			row[x] = uniSpaceOf(v)
			n++
		}
	}
	return n
}

// wideNormal applies it to the whole page.
func wideNormal(d *Doc) int {
	n := 0
	for y := 0; y < d.rows; y++ {
		n += wideNormalRow(d, y, 0, d.cols-1)
	}
	return n
}

// wideNormalCols applies it, on every row, around columns x (a cut between x-1 and x).
func wideNormalCols(d *Doc, xs ...int) {
	for y := 0; y < d.rows; y++ {
		for _, x := range xs {
			wideNormalRow(d, y, x-1, x)
		}
	}
}

// wideOK checks the page: every wide character followed by its right half and
// every right half after one. It returns the first cell that is not, or -1.
func wideOK(d *Doc) (x, y int) {
	if !d.uni {
		return -1, -1
	}
	for y := 0; y < d.rows; y++ {
		for x := 0; x < d.cols; x++ {
			v := d.get(x, y)
			if (uniIsCont(v) && !uniIsLead(d.get(x-1, y))) || (uniIsLead(v) && (x+1 >= d.cols || !uniIsCont(d.get(x+1, y)))) {
				return x, y
			}
		}
	}
	return -1, -1
}

// wideAny: does a text hold a wide character?
func wideAny(s string) bool {
	for _, r := range s {
		if uniWideRune(r) {
			return true
		}
	}
	return false
}

// wideIn: does a page hold any wide character (or a half of one)?
func wideIn(d *Doc) bool {
	if !d.uni {
		return false
	}
	for _, v := range d.cells[:d.rows*d.cols] {
		if uniIsCont(v) || uniIsLead(v) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------- the font

// Unifont's glyphs of every wide character it has (tools/unifont.sh; OFL,
// fonts/unifont-NOTICE.txt), read the first time a wide character is drawn.
//
//go:embed fonts/unifont-wide.hex.gz
var unifontWideHex []byte

type uniWideData struct {
	at     map[rune]int32 // character -> offset in glyphs
	glyphs []uint16       // 16 rows of 16 pixels each
	mu     sync.Mutex
	small  map[rune][]byte // squeezed into one cell (the character table, a lone character)
}

var uniWideOnce sync.Once
var uniWideTab *uniWideData

func uniWideFont() *uniWideData {
	uniWideOnce.Do(func() {
		f := &uniWideData{at: map[rune]int32{}, small: map[rune][]byte{}}
		zr, err := gzip.NewReader(bytes.NewReader(unifontWideHex))
		if err != nil {
			panic("unifont-wide: " + err.Error())
		}
		sc := bufio.NewScanner(zr)
		for sc.Scan() {
			cp, glyph, ok := strings.Cut(sc.Text(), ":")
			if !ok {
				continue
			}
			r, err := strconv.ParseUint(cp, 16, 32)
			if err != nil {
				continue
			}
			b, err := hex.DecodeString(glyph)
			if err != nil {
				continue
			}
			var g [16]uint16
			switch len(b) {
			case 32:
				for row := range g {
					g[row] = uint16(b[row*2])<<8 | uint16(b[row*2+1])
				}
			case 16: // drawn 8 wide: in the middle of the two cells
				for row := range g {
					g[row] = uint16(b[row]) << 4
				}
			default:
				continue
			}
			f.at[rune(r)] = int32(len(f.glyphs))
			f.glyphs = append(f.glyphs, g[:]...)
		}
		uniWideTab = f
	})
	return uniWideTab
}

// uniWideMissing is drawn for a wide character no font has: a box over both cells.
var uniWideMissing = func() []uint16 {
	g := make([]uint16, 16)
	for r := 2; r <= 12; r++ {
		g[r] = 0x4002
	}
	g[2], g[12] = 0x7FFE, 0x7FFE
	return g
}()

// uniWideGlyph is the 16x16 picture of a wide character (16 rows, bit 15 the left pixel).
func uniWideGlyph(r rune) ([]uint16, bool) {
	f := uniWideFont()
	if o, ok := f.at[r]; ok {
		return f.glyphs[o : o+16], true
	}
	return uniWideMissing, false
}

// uniWideSmall is a wide character squeezed into one cell, two pixels to one.
func uniWideSmall(r rune) ([]byte, bool) {
	f := uniWideFont()
	f.mu.Lock()
	defer f.mu.Unlock()
	if g, ok := f.small[r]; ok {
		return g, true
	}
	o, ok := f.at[r]
	if !ok {
		return nil, false
	}
	g := make([]byte, 16)
	for row := range g {
		w := f.glyphs[int(o)+row]
		for x := 0; x < 8; x++ {
			if w&(0xC000>>(2*x)) != 0 {
				g[row] |= 0x80 >> x
			}
		}
	}
	f.small[r] = g
	return g, true
}

// ---------------------------------------------------------------- editing

// wideSet writes cell v at (x, y) as a terminal would: a wide character takes
// (x+1, y) too, and whatever it writes over half of loses its other half (a
// space). Writing a wide character where it already stands does nothing, so a
// line or a fill drawn with one comes out as whole characters side by side, in
// either direction. A right half from a block stays only after a wide
// character. False: a wide character does not fit in the last column.
func wideSet(d *Doc, x, y int, v uint32) bool {
	old := d.get(x, y)
	lead := uniIsLead(v)
	if lead {
		if x+1 >= d.cols {
			return false
		}
		if (uniIsCont(old) && d.get(x-1, y) == v) || (d.get(x+1, y) == v && uniIsCont(d.get(x+2, y))) {
			return true
		}
	}
	if uniIsCont(v) {
		if !uniIsCont(old) || !uniIsLead(d.get(x-1, y)) { // not over a wide character's own right half
			wideBreak(d, x, y)
			v = uniSpaceOf(v)
		}
		d.set(x, y, v)
		return true
	}
	wideBreak(d, x, y)
	if lead {
		wideBreak(d, x+1, y)
		d.set(x+1, y, uniContOf(v))
	}
	d.set(x, y, v)
	return true
}

// wideBreak: cell (x, y) is about to be written: the wide character it is half of loses its other half.
func wideBreak(d *Doc, x, y int) {
	old := d.get(x, y)
	switch {
	case uniIsCont(old) && uniIsLead(d.get(x-1, y)):
		d.set(x-1, y, uniSpaceOf(d.get(x-1, y)))
	case uniIsLead(old) && uniIsCont(d.get(x+1, y)):
		d.set(x+1, y, uniSpaceOf(d.get(x+1, y)))
	}
}

// wideInsert makes room for w cells at (x, y): the rest of the row moves right
// (a wide character pushed half off the end becomes a space), the room is blank.
func wideInsert(d *Doc, x, y, w int) {
	wideBreak(d, x, y) // the cursor is never on a right half; a lone one is no character
	for i := d.cols - 1; i >= x+w; i-- {
		d.set(i, y, d.get(i-w, y))
	}
	for i := x; i < min(x+w, d.cols); i++ {
		d.set(i, y, blank)
	}
	wideNormalRow(d, y, d.cols-1, d.cols-1)
}

// wideDelete takes the cell at (x, y) out of its row, a wide character both its cells.
func wideDelete(d *Doc, x, y int) {
	w := 1
	switch v := d.get(x, y); {
	case uniIsCont(v) && uniIsLead(d.get(x-1, y)):
		x, w = x-1, 2
	case uniIsLead(v) && uniIsCont(d.get(x+1, y)):
		w = 2
	}
	for i := x; i < d.cols; i++ {
		if i+w < d.cols {
			d.set(i, y, d.get(i+w, y))
		} else {
			d.set(i, y, blank)
		}
	}
}

// wideFloodFill fills the cells like (x, y) around it with want, in reading
// order through wideSet; a wide character only where both its cells are in the area.
func (a *App) wideFloodFill(x, y int, target, want uint32) {
	d := a.doc
	in := map[[2]int]bool{}
	stack := [][2]int{{x, y}}
	for len(stack) > 0 {
		p := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if p[0] < 0 || p[1] < 0 || p[0] >= d.cols || p[1] >= d.rows || in[p] || d.get(p[0], p[1]) != target {
			continue
		}
		in[p] = true
		stack = append(stack, [2]int{p[0] + 1, p[1]}, [2]int{p[0] - 1, p[1]}, [2]int{p[0], p[1] + 1}, [2]int{p[0], p[1] - 1})
	}
	lead := uniIsLead(want)
	for py := 0; py < d.rows; py++ {
		for px := 0; px < d.cols; px++ {
			if in[[2]int{px, py}] && (!lead || in[[2]int{px + 1, py}]) {
				wideSet(d, px, py, want)
			}
		}
	}
}

// wideGrow widens a selection so it holds whole wide characters.
func wideGrow(d *Doc, r Rect) Rect {
	if !d.uni {
		return r
	}
	x0, x1 := r.x, r.x+r.w-1
	for y := r.y; y < min(r.y+r.h, d.rows); y++ {
		if uniIsCont(d.get(x0, y)) && uniIsLead(d.get(x0-1, y)) {
			x0 = r.x - 1
		}
		if uniIsLead(d.get(x1, y)) && uniIsCont(d.get(x1+1, y)) {
			x1 = r.x + r.w
		}
	}
	return Rect{x0, r.y, x1 - x0 + 1, r.h}
}

// wideInBlock: does a block hold a wide character?
func wideInBlock(b *Block) bool {
	for _, v := range b.cells {
		if uniIsLead(v) || uniIsCont(v) {
			return true
		}
	}
	return false
}

// wideFlipPairs puts the halves of a block mirrored left-right back in order.
func wideFlipPairs(b *Block) {
	for y := 0; y < b.h; y++ {
		row := b.cells[y*b.w : (y+1)*b.w]
		for x := 0; x+1 < b.w; x++ {
			if uniIsCont(row[x]) && uniIsLead(row[x+1]) {
				row[x], row[x+1] = row[x+1], row[x]
				x++
			}
		}
	}
}
