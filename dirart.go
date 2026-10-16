package main

// C64 DirArt: a disk whose directory listing is the picture. LOAD"$",8 and
// LIST show one line per directory entry, its 16-character name in quotes:
// the art is the names, 16 characters a row, up to 144 rows (18 directory
// sectors of 8 entries on track 18). The DirArt page is a Commodore machine of
// 16 columns in the listing's light blue on blue (one colour, as the PET's);
// it is saved as a .d64 whose entries are DEL files of 0 blocks, and a .d64
// whose directory CharEdit has no screen program in opens as such a page.
//
// Names: the 1541 keeps them in PETSCII, padded with $A0. LIST prints a name
// between quotes in quote mode, so control codes show as reversed characters
// (PETSCII $01-$1F as reversed screen codes 129-159, $81-$9F as 193-223);
// what cannot be in a name: the quote, $00 (ends the BASIC line), RETURN
// ($0D, $8D), DEL ($14) and the other reversed characters (no RVS ON inside a
// name). The list of characters that cannot be used is Marq's PETSCII editor's
// (m_dirart.pde forbidden_chars) and Petmate9's (DIRART_ILLEGAL_CHARS); the
// PETSCII to screen code table Petmate9's (x1541.ts petsciiToScreen).
// Checked in VICE: LOAD"$",8 and LIST show the page (chs_test.go).

import (
	"fmt"
	"slices"
	"strings"
)

func init() {
	machines = append(machines, Machine{"dirart", "DirArt", "c64", 16, 25, "dirart", true, 1})
	// The listing's colours: light blue on blue (Pepto's C64 blue and light blue, as Marq's DirArt).
	dir := monoPal("dirart", "C64 directory listing", 0x6C5EB5)
	dir.colors[0] = 0xFF352879
	cbmPalettes = append(cbmPalettes, dir)
	at := slices.IndexFunc(formats, func(f Format) bool { return f.id == "d64" })
	formats = slices.Insert(formats, at+1, Format{"dirart", "C64 disk whose directory shows it (DirArt .d64)", ".d64", "c64 (DirArt)"})
	editorOwn["dirart"] = true
	marqMachines["DIRART"] = "dirart"
	openAs = append(openAs, "dirart")
	openAsNames = append(openAsNames, "C64 DirArt (a disk's directory)")
}

// dirart: the page is a DirArt page.
func (a *App) dirart() bool { return a.c64() && a.machine == "dirart" }

// dirartHides: formats a DirArt page does not offer (a program showing 16
// columns), and DirArt's .d64 on other pages.
func (a *App) dirartHides(f Format) bool {
	if f.modes == "c64 (DirArt)" {
		return !a.dirart()
	}
	return a.dirart() && (f.id == "prg" || f.id == "d64")
}

const (
	dirartCols = 16
	dirartMax  = 144 // 18 directory sectors of 8 entries
)

// dirartDirSectors are track 18's directory sectors in the 1541's order (an
// interleave of 3 after the BAM at sector 0).
var dirartDirSectors = []int{1, 4, 7, 10, 13, 16, 2, 5, 8, 11, 14, 17, 3, 6, 9, 12, 15, 18}

// petsciiToScreen is the screen code LIST shows for a byte of a name
// (Petmate9's x1541.ts; bytes 0-31 and 128-159 in quote mode, reversed).
func petsciiToScreen(p byte) int {
	switch {
	case p < 32:
		return int(p) + 128
	case p < 64:
		return int(p)
	case p < 96:
		return int(p) - 64
	case p < 128:
		return int(p) - 32
	case p < 160:
		return int(p) + 64
	case p < 192:
		return int(p) - 64
	case p < 255:
		return int(p) - 128
	}
	return 94
}

// screenToName is the byte a name holds for a screen code, -1 for one a
// name cannot show.
func screenToName(sc int) int {
	switch {
	case sc == 34 || sc == 128 || sc == 141 || sc == 148 || sc == 205 || sc >= 160 && sc < 192 || sc >= 224:
		return -1
	case sc < 32:
		return sc + 64
	case sc < 64:
		return sc
	case sc < 96:
		return sc + 128 // 192-223, as Petmate9 writes them
	case sc < 127:
		return sc + 128 // 224-254 (160 would end the name)
	case sc == 127:
		return 191
	case sc < 160:
		return sc - 128 // a control code: reversed in quote mode
	}
	return sc - 64 // 192-223: $80-$9F
}

// dirartEntry is one line of a directory: where its 32 bytes are in the image.
type dirartEntry struct {
	at     int // the entry's offset in the image
	kind   byte
	blocks int
}

// dirartEntries lists the entries LIST shows (type byte not 0), in order, and
// the free slots of the directory's sectors after the last of them (a slot
// before it would list a new entry among the old ones).
func dirartEntries(img []byte) (ents []dirartEntry, free []int, sectors [][2]int, err error) {
	tracks := d64Tracks(len(img))
	if tracks == 0 {
		return nil, nil, nil, fmt.Errorf("not a .d64 disk image (%d bytes)", len(img))
	}
	bam := d64Offset(18, 0, tracks)
	t, s := int(img[bam]), int(img[bam+1])
	if t != 18 {
		t, s = 18, 1
	}
	seen := map[[2]int]bool{}
	for t != 0 && !seen[[2]int{t, s}] {
		seen[[2]int{t, s}] = true
		o := d64Offset(t, s, tracks)
		if o < 0 {
			break
		}
		sectors = append(sectors, [2]int{t, s})
		for e := 0; e < 8; e++ {
			at := o + e*32
			if img[at+2] == 0 {
				free = append(free, at)
				continue
			}
			ents = append(ents, dirartEntry{at: at, kind: img[at+2], blocks: int(img[at+30]) | int(img[at+31])<<8})
		}
		t, s = int(img[o]), int(img[o+1])
	}
	if len(ents) > 0 {
		last := ents[len(ents)-1].at
		free = slices.DeleteFunc(free, func(at int) bool { return slotBefore(sectors, at, last, tracks) })
	}
	return ents, free, sectors, nil
}

// slotBefore: is the entry at offset at before the one at last in the chain's order?
func slotBefore(sectors [][2]int, at, last, tracks int) bool {
	pos := func(off int) int {
		for i, ts := range sectors {
			if o := d64Offset(ts[0], ts[1], tracks); off >= o && off < o+256 {
				return i*8 + (off-o)/32
			}
		}
		return -1
	}
	return pos(at) < pos(last)
}

// dirartName is a name's 16 bytes as screen codes: the $A0s (the padding,
// which LIST shows as the closing quote and spaces) as spaces.
func dirartName(b []byte) []int {
	out := make([]int, dirartCols)
	for i := range out {
		if b[i] == 0xA0 {
			out[i] = 32
		} else {
			out[i] = petsciiToScreen(b[i])
		}
	}
	return out
}

// readDirArt is the page a disk's directory shows: a row per entry.
func readDirArt(img []byte) (*Doc, error) {
	ents, _, _, err := dirartEntries(img)
	if err != nil {
		return nil, err
	}
	d := newDoc(dirartCols, max(1, len(ents)))
	d.glyph0, d.border = true, 1
	for i := range d.cells {
		d.cells[i] = makeCell(32, 1, 0, 0)
	}
	for y, e := range ents {
		for x, sc := range dirartName(img[e.at+5 : e.at+21]) {
			d.set(x, y, makeCell(sc, 1, 0, 0))
		}
	}
	return d, nil
}

// dirartRow is a page row as a name's 16 bytes; bad counts the cells a name
// cannot show (written as spaces).
func dirartRow(d *Doc, y int) (name []byte, bad int) {
	name = make([]byte, dirartCols)
	for x := range name {
		v := 32
		if x < d.cols && y < d.rows {
			v = cellCh(d.get(x, y))
		}
		b := screenToName(v)
		if b < 0 {
			b, bad = 32, bad+1
		}
		name[x] = byte(b)
	}
	return name, bad
}

// dirartSame: does a name LIST shows as these screen codes already (so a
// source disk's bytes, $A0s and all, are kept)?
func dirartSame(old []byte, d *Doc, y int) bool {
	for x, sc := range dirartName(old) {
		v := 32
		if x < d.cols && y < d.rows {
			v = cellCh(d.get(x, y))
		}
		if v != sc {
			return false
		}
	}
	return true
}

// dirartNote is what a DirArt save could not keep (for the status line).
type dirartNote struct{ bad, cut, kept, plain int } // plain: header cells not reversed (dirarthead.go)

func (n dirartNote) String() string {
	var s []string
	if n.bad > 0 {
		s = append(s, fmt.Sprintf("%d cells a file name cannot show became spaces", n.bad))
	}
	if n.cut > 0 {
		s = append(s, fmt.Sprintf("%d rows past the directory's %d entries were left out", n.cut, dirartMax))
	}
	if n.kept > 0 {
		s = append(s, fmt.Sprintf("the disk's %d files after the art stay listed", n.kept))
	}
	if n.plain > 0 {
		s = append(s, fmt.Sprintf("%d characters of the first row (the disk's header line) show reversed, as LIST shows the header", n.plain))
	}
	if len(s) == 0 {
		return ""
	}
	return "; " + strings.Join(s, ", ")
}

// writeDirArt gives the page as a DirArt disk: over src (a .d64 the page
// came from: its files, names that did not change and its header are kept)
// or a new disk named name. Rows become entries in order: the disk's own
// entries first, then new DEL entries of 0 blocks.
func writeDirArt(d *Doc, src []byte, name string) ([]byte, dirartNote) {
	var note dirartNote
	img := src
	if d64Tracks(len(src)) == 0 {
		img = writeD64Empty(name)
	} else {
		img = append([]byte(nil), src...)
	}
	tracks := d64Tracks(len(img))
	ents, free, sectors, err := dirartEntries(img)
	if err != nil {
		img = writeD64Empty(name)
		ents, free, sectors, _ = dirartEntries(img)
	}
	rows := max(1, d.rows)
	if rows > dirartMax {
		note.cut, rows = rows-dirartMax, dirartMax
	}
	bam := d64Offset(18, 0, tracks)
	newEntry := func() int { // a free slot, in a new directory sector when the chain is full
		if len(free) > 0 {
			at := free[0]
			free = free[1:]
			return at
		}
		used := map[int]bool{}
		for _, ts := range sectors {
			used[ts[1]] = true
		}
		for _, s := range dirartDirSectors {
			e := img[bam+4*18 : bam+4*18+4]
			if used[s] || e[1+s/8]&(1<<(s%8)) == 0 { // in the chain, or taken by a file
				continue
			}
			e[0]--
			e[1+s/8] &^= 1 << (s % 8)
			last := sectors[len(sectors)-1]
			lo, o := d64Offset(last[0], last[1], tracks), d64Offset(18, s, tracks)
			img[lo], img[lo+1] = 18, byte(s)
			clear(img[o : o+256])
			img[o], img[o+1] = 0, 0xFF
			sectors = append(sectors, [2]int{18, s})
			for e := 1; e < 8; e++ {
				free = append(free, o+e*32)
			}
			return o
		}
		return -1
	}
	for y := 0; y < rows; y++ {
		var at int
		if y < len(ents) {
			at = ents[y].at
			if dirartSame(img[at+5:at+21], d, y) {
				continue
			}
		} else {
			if at = newEntry(); at < 0 {
				note.cut += rows - y
				break
			}
			ent := img[at : at+32]
			for i := 2; i < 32; i++ { // bytes 0-1 are the sector's link in its first entry
				ent[i] = 0
			}
			ent[2] = 0x80 // DEL, closed, 0 blocks
		}
		n, bad := dirartRow(d, y)
		note.bad += bad
		copy(img[at+5:at+21], n)
	}
	// Entries past the page: DEL entries of 0 blocks go (made art too), files stay.
	for _, e := range ents[min(len(ents), rows):] {
		if e.kind&7 == 0 && e.blocks == 0 {
			clear(img[e.at+2 : e.at+32])
		} else {
			note.kept++
		}
	}
	return img, note
}

// writeD64Empty is a 35-track disk with no files: the BAM and one directory
// sector, named name (the saved file's name).
func writeD64Empty(name string) []byte {
	img := writeD64(name, []byte{0x01, 0x08})
	bam := d64Offset(18, 0, 35)
	// Free the one sector writeD64 gave the file, and its directory entry.
	dir := d64Offset(18, 1, 35)
	t, s := int(img[dir+3]), int(img[dir+4])
	e := img[bam+4*t : bam+4*t+4]
	e[0]++
	e[1+s/8] |= 1 << (s % 8)
	o := d64Offset(t, s, 35)
	clear(img[o : o+256])
	clear(img[dir+2 : dir+256])
	return img
}

// ---------------------------------------------------------------- in CharEdit

// loadDirArt opens a .d64 without a screen program of CharEdit's (or any, with
// Open as DirArt) as a DirArt page; false if b is not that.
func (a *App) loadDirArt(b []byte, name, path, as string) bool {
	if !(as == "dirart" || (as == "auto" || as == "c64") && isD64(name, b)) || d64Tracks(len(b)) == 0 {
		return false
	}
	if as != "dirart" {
		if _, _, err := d64Art(b); err == nil {
			return false // a screen's program: the C64's way
		}
	}
	if _, err := readDirArt(b); err != nil {
		a.msg("Could not open " + name + ": " + err.Error())
		return true
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
	a.mode, a.srcKind, a.machine = "c64", "dirart", "dirart"
	a.paletteFor["c64"] = "dirart"
	a.finishLoad(b, name, path, nil, "dirart")
	a.setCase(false)
	head := "the first row is the disk's header line (LIST shows it reversed)"
	if a.message == "" {
		a.msg(strings.ToUpper(head[:1]) + head[1:])
	} else {
		a.msg(a.message + "; " + head)
	}
	return true
}

// reparseDirArt reads the page again from its disk.
func (a *App) reparseDirArt() bool {
	if a.srcKind != "dirart" {
		return false
	}
	if d, err := readDirArtPage(a.source); err == nil { // the header line first (dirarthead.go)
		a.doc, a.dirartHead = d, true
	}
	a.cx, a.cy, a.sel, a.float = 0, 0, nil, nil
	return true
}

// encodeDirArt writes a DirArt page's disk; ok false for other formats.
func (a *App) encodeDirArt(fmtID string, d *Doc) ([]byte, bool, error) {
	if fmtID != "dirart" || !a.c64() {
		return nil, false, nil
	}
	var src []byte
	if a.srcKind == "dirart" {
		src = a.source
	}
	name := cpcTarget
	if name == "" {
		name = a.name
	}
	img, note := writeDirArt(d, src, name) // every row a file, the disk's name the file's (or its disk's)
	if a.dirartHead {
		img, note = writeDirArtPage(d, src, name) // the first row is the header line (dirarthead.go)
	}
	dirartLastNote = note.String()
	return img, true, nil
}

// dirartSaved is the status line's note of a DirArt save.
func (a *App) dirartSaved(fmtID string) string {
	if fmtID != "dirart" {
		return ""
	}
	n := dirartLastNote
	dirartLastNote = ""
	if a.lower() {
		n += "; the listing shows the upper case set (a disk keeps no case)"
	}
	return n
}

// dirartLastNote is what the last DirArt save could not keep, for the status line.
var dirartLastNote string
