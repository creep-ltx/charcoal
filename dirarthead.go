package main

// The DirArt header line as part of the art. LIST starts a directory with the
// disk's header line, reversed: `0 "DISK NAME       " ID 2A`, the name being
// the 16 bytes at $90-$9F of the BAM (track 18, sector 0), padded with $A0;
// the five bytes after it ($A2-$A6: the disk ID, a $A0 and the DOS type)
// follow the closing quote. DirArt uses the name as the art's first line.
//
// On a DirArt page the first row is the header's name and the rows below it
// are the entries. LIST prints the whole header line reversed, so the row's
// cells are read as reversed characters and a character that is not reversed
// is written as the same character (it shows reversed: the save says how many).
// LIST shows the name 3 columns left of the entries' names ("0 " and the
// quote, against "0    " and the quote); the page keeps them in one column.
// The disk ID and DOS type are kept from the disk the page came from (a new
// disk: "CE 2A").

// dirartHeadName is the header name's 16 bytes in a disk image (nil: none).
func dirartHeadName(img []byte) []byte {
	tracks := d64Tracks(len(img))
	if tracks == 0 {
		return nil
	}
	bam := d64Offset(18, 0, tracks)
	return img[bam+0x90 : bam+0xA0]
}

// readDirArtPage is a disk's directory as a DirArt page: the header's name
// (reversed, as LIST shows it), then a row per entry.
func readDirArtPage(img []byte) (*Doc, error) {
	body, err := readDirArt(img)
	if err != nil {
		return nil, err
	}
	d := newDoc(dirartCols, body.rows+1)
	d.glyph0, d.border = body.glyph0, body.border
	copy(d.cells[dirartCols:], body.cells[:body.rows*dirartCols])
	for x, sc := range dirartName(dirartHeadName(img)) {
		d.set(x, 0, makeCell(sc|0x80, 1, 0, 0))
	}
	return d, nil
}

// dirartHeadSame: the page's first row is the name LIST shows already (so
// the disk's bytes, $A0s and all, are kept).
func dirartHeadSame(old []byte, d *Doc) bool {
	for x, sc := range dirartName(old) {
		if v := cellCh(d.get(x, 0)); v|0x80 != sc|0x80 {
			return false
		}
	}
	return true
}

// writeDirArtPage writes a DirArt page (its first row the header's name) as
// writeDirArt writes the rows below it.
func writeDirArtPage(d *Doc, src []byte, name string) ([]byte, dirartNote) {
	body := newDoc(dirartCols, max(1, d.rows-1))
	body.glyph0 = d.glyph0
	for i := range body.cells {
		body.cells[i] = makeCell(32, 1, 0, 0)
	}
	for y := 1; y < d.rows; y++ {
		for x := 0; x < dirartCols && x < d.cols; x++ {
			body.set(x, y-1, d.get(x, y))
		}
	}
	img, note := writeDirArt(body, src, name)
	head := dirartHeadName(img)
	if head == nil || d.rows == 0 {
		return img, note
	}
	if old := dirartHeadName(src); old != nil && dirartHeadSame(old, d) {
		copy(head, old)
		return img, note
	}
	for x := range head {
		sc := 32
		if x < d.cols {
			sc = cellCh(d.get(x, 0))
		}
		if sc < 128 && sc != 32 {
			note.plain++ // shows reversed anyway
		}
		b := screenToName(sc &^ 0x80)
		if b < 0 {
			b, note.bad = 32, note.bad+1
		}
		head[x] = byte(b)
	}
	return img, note
}
