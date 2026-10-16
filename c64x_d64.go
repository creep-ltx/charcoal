package main

// .d64: a 1541 disk image with the page's program on it (the compos take a
// .prg or a .d64), and the first art program read back from one.
//
// The layout (Commodore 1541 user's guide, appendix "Disk format"; the d64
// layout as VICE and c1541 write it): 35 tracks of 21 (1-17), 19 (18-24), 18
// (25-30) and 17 (31-35) sectors of 256 bytes, 683 in all (174848 bytes; an
// image may add an error byte per sector, or have 40 tracks). Track 18
// sector 0 is the BAM: the first directory sector (18/1), "A", then four
// bytes per track (free sectors, a bit per sector, set = free), the disk's
// name (16 bytes padded with $A0) at $90, its id at $A2 and "2A" at $A5. The
// directory is a chain of sectors from 18/1, eight 32-byte entries each: file
// type ($82 = PRG, closed), first track/sector, name, size in sectors. A
// file is a chain of sectors: the first two bytes name the next one, or are
// 0 and the index of the last byte used. Files are written as the 1541 does:
// from track 17 down and 19 up, every 10th sector on a track.

import (
	"fmt"
	"path/filepath"
	"strings"
)

var d64Sectors = func() []int {
	n := make([]int, 41)
	for t := 1; t <= 40; t++ {
		switch {
		case t <= 17:
			n[t] = 21
		case t <= 24:
			n[t] = 19
		case t <= 30:
			n[t] = 18
		default:
			n[t] = 17
		}
	}
	return n
}()

const d64Size = 174848

// d64Offset is where track t sector s is in an image, -1 if not on the disk.
func d64Offset(t, s, tracks int) int {
	if t < 1 || t > tracks || s < 0 || s >= d64Sectors[t] {
		return -1
	}
	o := 0
	for i := 1; i < t; i++ {
		o += d64Sectors[i]
	}
	return (o + s) * 256
}

// d64Tracks is how many tracks an image of n bytes has (0: not a .d64).
func d64Tracks(n int) int {
	switch n {
	case d64Size, d64Size + 683:
		return 35
	case 196608, 196608 + 768:
		return 40
	}
	return 0
}

// petsciiName is a file or disk name as the 1541 keeps it: 16 bytes, PETSCII,
// padded with $A0. Letters are shown as capitals (the upper case set).
func petsciiName(s string) []byte {
	out := make([]byte, 0, 16)
	for _, r := range strings.ToUpper(s) {
		if len(out) == 16 {
			break
		}
		switch {
		case r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == ' ', strings.ContainsRune("!#$%&'()+,-.;=@[]", r):
			out = append(out, byte(r))
		default:
			out = append(out, '-')
		}
	}
	for len(out) < 16 {
		out = append(out, 0xA0)
	}
	return out
}

// writeD64 gives a 35-track disk with one program on it, named after the file
// it is saved as (path; "" gives CHARCOAL).
func writeD64(path string, prg []byte) []byte {
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	if path == "" || name == "" || name == "." {
		name = "charcoal"
	}
	img := make([]byte, d64Size)
	free := make([][]bool, 36)
	for t := 1; t <= 35; t++ {
		free[t] = make([]bool, d64Sectors[t])
		for s := range free[t] {
			free[t][s] = true
		}
	}
	free[18][0], free[18][1] = false, false
	sec := func(t, s int) []byte { o := d64Offset(t, s, 35); return img[o : o+256] }
	// The file's sectors.
	var chain [][2]int
	order := []int{}
	for t := 17; t >= 1; t-- {
		order = append(order, t)
	}
	for t := 19; t <= 35; t++ {
		order = append(order, t)
	}
	need := max(1, (len(prg)+253)/254)
	for _, t := range order {
		s := 0
		for len(chain) < need {
			n := d64Sectors[t]
			k := 0
			for k < n && !free[t][s] {
				s, k = (s+1)%n, k+1
			}
			if k == n {
				break // the track is full
			}
			free[t][s] = false
			chain = append(chain, [2]int{t, s})
			s = (s + 10) % n
		}
		if len(chain) == need {
			break
		}
	}
	for i, ts := range chain {
		b := sec(ts[0], ts[1])
		part := prg[i*254 : min(len(prg), (i+1)*254)]
		copy(b[2:], part)
		if i+1 < len(chain) {
			b[0], b[1] = byte(chain[i+1][0]), byte(chain[i+1][1])
		} else {
			b[0], b[1] = 0, byte(len(part)+1)
		}
	}
	// The directory: one entry.
	dir := sec(18, 1)
	dir[0], dir[1] = 0, 0xFF
	dir[2], dir[3], dir[4] = 0x82, byte(chain[0][0]), byte(chain[0][1])
	copy(dir[5:21], petsciiName(name))
	dir[30], dir[31] = byte(len(chain)), byte(len(chain)>>8)
	// The BAM.
	bam := sec(18, 0)
	bam[0], bam[1], bam[2] = 18, 1, 0x41
	for t := 1; t <= 35; t++ {
		e := bam[4*t : 4*t+4]
		for s, f := range free[t] {
			if f {
				e[0]++
				e[1+s/8] |= 1 << (s % 8)
			}
		}
	}
	copy(bam[0x90:0xA0], petsciiName(name))
	bam[0xA0], bam[0xA1] = 0xA0, 0xA0
	bam[0xA2], bam[0xA3], bam[0xA4] = 'C', 'E', 0xA0
	bam[0xA5], bam[0xA6] = '2', 'A'
	for i := 0xA7; i <= 0xAA; i++ {
		bam[i] = 0xA0
	}
	return img
}

// d64File is a file read from a disk image.
type d64File struct {
	name  string // as text
	kind  byte   // the directory's file type
	data  []byte // a PRG's load address included
	index int    // its place in the directory, from 1
}

// d64Dir reads every closed file of an image (a broken chain ends the file).
func d64Dir(img []byte) ([]d64File, error) {
	tracks := d64Tracks(len(img))
	if tracks == 0 {
		return nil, fmt.Errorf("not a .d64 disk image (%d bytes)", len(img))
	}
	bam := d64Offset(18, 0, tracks)
	t, s := int(img[bam]), int(img[bam+1])
	if t != 18 {
		t, s = 18, 1
	}
	var out []d64File
	seen := map[[2]int]bool{}
	n := 0
	for t != 0 && !seen[[2]int{t, s}] {
		seen[[2]int{t, s}] = true
		o := d64Offset(t, s, tracks)
		if o < 0 {
			break
		}
		b := img[o : o+256]
		for e := 0; e < 8; e++ {
			ent := b[e*32 : e*32+32]
			kind := ent[2]
			if kind&0x80 == 0 || kind&7 == 0 {
				continue
			}
			n++
			out = append(out, d64File{name: d64Text(ent[5:21]), kind: kind, data: d64Chain(img, tracks, int(ent[3]), int(ent[4])), index: n})
		}
		t, s = int(b[0]), int(b[1])
	}
	return out, nil
}

// d64Text is a directory name as text (the $A0 padding cut off).
func d64Text(b []byte) string {
	var s strings.Builder
	for _, c := range b {
		if c == 0xA0 {
			break
		}
		if c >= 0x20 && c < 0x7F {
			s.WriteByte(c)
		} else {
			s.WriteByte('?')
		}
	}
	return s.String()
}

// d64Chain reads a file's chain of sectors from t/s.
func d64Chain(img []byte, tracks, t, s int) []byte {
	var out []byte
	seen := map[[2]int]bool{}
	for t != 0 && !seen[[2]int{t, s}] && len(out) < len(img) {
		seen[[2]int{t, s}] = true
		o := d64Offset(t, s, tracks)
		if o < 0 {
			break
		}
		b := img[o : o+256]
		if b[0] == 0 {
			end := int(b[1])
			if end < 2 {
				end = 1
			}
			return append(out, b[2:end+1]...)
		}
		out = append(out, b[2:]...)
		t, s = int(b[0]), int(b[1])
	}
	return out
}

// d64Art is the first program on the disk that CharEdit can open (Petmate's
// form or its own) and how many art programs there are.
func d64Art(img []byte) (d64File, int, error) {
	files, err := d64Dir(img)
	if err != nil {
		return d64File{}, 0, err
	}
	var first d64File
	n := 0
	for _, f := range files {
		if f.kind&7 == 2 && isOurC64Prg(f.data) {
			if n == 0 {
				first = f
			}
			n++
		}
	}
	if n == 0 {
		return d64File{}, 0, fmt.Errorf("no screen in Petmate's or CharEdit's form on the disk (%d files)", len(files))
	}
	return first, n, nil
}

// isD64 is the test of loadBytes: a .d64 by name and size.
func isD64(name string, b []byte) bool {
	return strings.HasSuffix(strings.ToLower(name), ".d64") && d64Tracks(len(b)) > 0
}
