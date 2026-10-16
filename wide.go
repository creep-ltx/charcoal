package main

// Wide characters on the Unicode page (docs/wide-design.md): a character a
// terminal shows two columns wide (CJK, kana, Hangul, fullwidth forms, most
// emoji) takes two cells, a lead (the character) and a continuation (code
// uniCont, in the lead's style). The rule everywhere is the terminal's: writing
// into either half of a wide character leaves a space in the other half.

import "sort"

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
