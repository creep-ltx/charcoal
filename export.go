package main

// More formats: a C64 program that shows the screen (.prg), the C array of
// Marq's PETSCII editor and Petmate (.c), XBin with its font and palette (.xb),
// and a web page (.html).

import (
	"bytes"
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"
)

// ---------------------------------------------------------------- C64 screens

// c64Screen gives the first 40x25 cells as screen codes and colours.
func c64Screen(d *Doc) (chars, cols []byte) {
	chars, cols = make([]byte, 1000), make([]byte, 1000)
	for y := 0; y < 25; y++ {
		for x := 0; x < 40; x++ {
			v := d.get(x, y)
			chars[y*40+x], cols[y*40+x] = byte(cellCh(v)), byte(cellFg(v))
		}
	}
	return
}

func (d *Doc) borderColour() int {
	if d.border < 0 {
		return d.bg
	}
	return d.border
}

// writePrg gives a C64 program: a BASIC line "10 SYS 2061" and machine code that
// puts the screen up, waits for a key and goes back to BASIC.
func writePrg(d *Doc, lower bool) []byte {
	chars, cols := c64Screen(d)
	code := func(data int) []byte {
		lo, hi := func(a int) byte { return byte(a) }, func(a int) byte { return byte(a >> 8) }
		charset := byte(0x15)
		if lower {
			charset = 0x17
		}
		c := []byte{
			0xA9, byte(d.borderColour()), 0x8D, 0x20, 0xD0, // LDA #border, STA $D020
			0xA9, byte(d.bg), 0x8D, 0x21, 0xD0, // LDA #bg, STA $D021
			0xA9, charset, 0x8D, 0x18, 0xD0, // the character set: STA $D018
			0xA9, 0x80, 0x8D, 0x91, 0x02, // no Shift+C= switching: STA $0291
			0xA2, 0x00, // LDX #0
		}
		for _, part := range []struct{ src, dst int }{
			{data, 0x0400}, {data + 250, 0x04FA}, {data + 500, 0x05F4}, {data + 750, 0x06EE},
			{data + 1000, 0xD800}, {data + 1250, 0xD8FA}, {data + 1500, 0xD9F4}, {data + 1750, 0xDAEE},
		} {
			c = append(c, 0xBD, lo(part.src), hi(part.src), 0x9D, lo(part.dst), hi(part.dst)) // LDA src,X  STA dst,X
		}
		c = append(c, 0xE8, 0xE0, 250, 0xD0, byte(256-(48+5))) // INX, CPX #250, BNE loop
		c = append(c,
			0x20, 0xE4, 0xFF, 0xF0, 0xFB, // wait: JSR GETIN, BEQ wait
			0xA9, 0x0E, 0x8D, 0x20, 0xD0, // the usual colours back
			0xA9, 0x06, 0x8D, 0x21, 0xD0,
			0xA9, 0x00, 0x8D, 0x91, 0x02,
			0xA9, 0x93, 0x20, 0xD2, 0xFF, // clear the screen
			0x60) // RTS: READY.
		return c
	}
	const start = 0x080D
	n := len(code(0))
	out := []byte{0x01, 0x08, // load address $0801
		0x0B, 0x08, 0x0A, 0x00, 0x9E, '2', '0', '6', '1', 0x00, // 10 SYS2061
		0x00, 0x00} // end of BASIC
	out = append(out, code(start+n)...)
	out = append(out, chars...)
	return append(out, cols...)
}

// writeMarq gives the C array Marq's PETSCII editor and Petmate read.
func writeMarq(d *Doc, lower bool) []byte {
	chars, cols := c64Screen(d)
	var b strings.Builder
	b.WriteString("unsigned char frame0000[]={// border,bg,chars,colors\n")
	fmt.Fprintf(&b, "%d,%d,\n", d.borderColour(), d.bg)
	for _, part := range [][]byte{chars, cols} {
		for y := 0; y < 25; y++ {
			for x := 0; x < 40; x++ {
				fmt.Fprintf(&b, "%d,", part[y*40+x])
			}
			b.WriteString("\n")
		}
	}
	b.WriteString("};\n")
	set := "upper"
	if lower {
		set = "lower"
	}
	fmt.Fprintf(&b, "// META: 40 25 C64 %s\n", set)
	return []byte(b.String())
}

var marqNumbers = regexp.MustCompile(`\d+`)

func isMarq(b []byte) bool {
	return bytes.Contains(b[:min(len(b), 400)], []byte("unsigned char")) && bytes.Contains(b, []byte("{"))
}

// parseMarq reads the first frame of a PETSCII editor / Petmate C array; lower is -1 if not said.
func parseMarq(b []byte) (*Doc, int, error) {
	s := string(b)
	lower := -1
	if strings.Contains(s, "C64 lower") {
		lower = 1
	} else if strings.Contains(s, "C64 upper") {
		lower = 0
	}
	start, end := strings.Index(s, "{"), strings.Index(s, "}")
	if start < 0 || end < start {
		return nil, -1, fmt.Errorf("no array in the file")
	}
	body := s[start+1 : end]
	body = regexp.MustCompile(`//[^\n]*`).ReplaceAllString(body, "")
	var nums []int
	for _, m := range marqNumbers.FindAllString(body, -1) {
		v, _ := strconv.Atoi(m)
		nums = append(nums, v)
	}
	if len(nums) < 2002 {
		return nil, -1, fmt.Errorf("the array has %d numbers, a screen needs 2002", len(nums))
	}
	d := newDoc(40, 25)
	d.border, d.bg = nums[0]&15, nums[1]&15
	for i := 0; i < 1000; i++ {
		d.cells[i] = makeCell(nums[2+i]&255, nums[1002+i]&15, 0, 0)
	}
	d.trim(1)
	return d, lower, nil
}

// ---------------------------------------------------------------- XBin

// writeXBin gives an XBin with the palette and the font in it, so it looks the same anywhere.
func writeXBin(d *Doc, pal [16]uint32, f *Font, cellH int, ice bool) []byte {
	rows := max(d.usedRows(), 1)
	out := []byte{'X', 'B', 'I', 'N', 0x1A, byte(d.cols), byte(d.cols >> 8), byte(rows), byte(rows >> 8), byte(cellH)}
	flags := byte(1 | 2) // palette, font
	if ice {
		flags |= 8
	}
	out = append(out, flags)
	for _, c := range pal {
		out = append(out, byte(c>>16&255)>>2, byte(c>>8&255)>>2, byte(c&255)>>2)
	}
	skip := 16 / cellH
	for c := 0; c < 256; c++ {
		for r := 0; r < cellH; r++ {
			out = append(out, f.bits[c*16+r*skip])
		}
	}
	for y := 0; y < rows; y++ {
		for x := 0; x < d.cols; x++ {
			v := d.get(x, y)
			out = append(out, byte(cellCh(v)), byte(cellFg(v)|cellBg(v)<<4))
		}
	}
	return out
}

type XBinInfo struct {
	pal   *[16]uint32
	font  *Font
	ice   bool
	cellH int
}

func isXBin(b []byte) bool { return len(b) >= 11 && string(b[:5]) == "XBIN\x1A" }

// parseXBin reads an XBin, compressed or not.
func parseXBin(b []byte) (*Doc, XBinInfo, error) {
	var info XBinInfo
	if !isXBin(b) {
		return nil, info, fmt.Errorf("not an XBin")
	}
	w, h := int(b[5])|int(b[6])<<8, int(b[7])|int(b[8])<<8
	fh, flags := int(b[9]), b[10]
	if w < 1 || w > 1000 || fh < 1 || fh > 32 {
		return nil, info, fmt.Errorf("an XBin of %d columns and %d-pixel font is not one I can show", w, fh)
	}
	p := 11
	if flags&1 != 0 {
		if len(b) < p+48 {
			return nil, info, fmt.Errorf("the XBin is cut short")
		}
		var pal [16]uint32
		for i := range pal {
			r, g, bl := uint32(b[p]&63), uint32(b[p+1]&63), uint32(b[p+2]&63)
			pal[i] = 0xFF000000 | (r<<2|r>>4)<<16 | (g<<2|g>>4)<<8 | (bl<<2 | bl>>4)
			p += 3
		}
		info.pal = &pal
	}
	if flags&2 != 0 {
		n := 256
		if flags&16 != 0 {
			n = 512
		}
		if len(b) < p+n*fh {
			return nil, info, fmt.Errorf("the XBin is cut short")
		}
		// Kept as an 8x16 font: rows doubled from 8, others fitted into 16.
		bits := make([]byte, 4096)
		for c := 0; c < 256; c++ {
			for r := 0; r < 16; r++ {
				src := r * fh / 16
				bits[c*16+r] = b[p+c*fh+src]
			}
		}
		info.font = &Font{id: "xbin", name: "The file's own font", kind: "pc", syncterm: -1, bits: bits}
		info.cellH = 16
		if fh <= 8 {
			info.cellH = 8
		}
		p += n * fh
	}
	info.ice = flags&8 != 0
	cells := make([]byte, 0, w*h*2)
	if flags&4 == 0 {
		cells = append(cells, b[p:min(len(b), p+w*h*2)]...)
	} else {
		for p < len(b) && len(cells) < w*h*2 {
			k, n := b[p]>>6, int(b[p]&63)+1
			p++
			switch k {
			case 0:
				for i := 0; i < n && p+1 < len(b); i++ {
					cells = append(cells, b[p], b[p+1])
					p += 2
				}
			case 1: // one character, n attributes
				ch := b[min(p, len(b)-1)]
				p++
				for i := 0; i < n && p < len(b); i++ {
					cells = append(cells, ch, b[p])
					p++
				}
			case 2: // one attribute, n characters
				at := b[min(p, len(b)-1)]
				p++
				for i := 0; i < n && p < len(b); i++ {
					cells = append(cells, b[p], at)
					p++
				}
			case 3:
				if p+1 >= len(b) {
					p = len(b)
					break
				}
				for i := 0; i < n; i++ {
					cells = append(cells, b[p], b[p+1])
				}
				p += 2
			}
		}
	}
	d := newDoc(w, max(h, 1))
	for i := 0; i+1 < len(cells) && i/2 < w*h; i += 2 {
		d.cells[i/2] = makeCell(int(cells[i]), int(cells[i+1]&15), int(cells[i+1]>>4), 0)
	}
	d.trim(1)
	return d, info, nil
}

// ---------------------------------------------------------------- a web page

func (a *App) writeHTML() []byte {
	d := a.doc
	hex := func(c uint32) string { return fmt.Sprintf("#%06x", c&0xFFFFFF) }
	bgOf := func(v uint32) uint32 {
		switch {
		case a.c64():
			return a.pal[d.bg]
		case a.ice:
			return a.pal[cellBg(v)]
		}
		return a.pal[cellBg(v)&7]
	}
	var b strings.Builder
	title := a.info.title
	if title == "" {
		title = a.name
	}
	page := a.pal[0]
	if a.c64() {
		page = a.pal[d.borderColour()]
	}
	fmt.Fprintf(&b, "<!doctype html>\n<html><head><meta charset=\"utf-8\"><title>%s</title>\n", html.EscapeString(title))
	fmt.Fprintf(&b, "<style>body{background:%s;margin:2em}pre{font:16px/1 \"Topaz a600a1200a400\",\"Px437 IBM VGA 8x16\",monospace;display:inline-block;background:%s;padding:0;margin:0}</style>\n",
		hex(page), hex(a.pal[0]))
	b.WriteString("</head><body><pre>")
	rows := max(d.usedRows(), 1)
	for y := 0; y < rows; y++ {
		last := uint64(1<<63 - 1)
		open := false
		for x := 0; x < d.cols; x++ {
			v := d.get(x, y)
			fg, bg := a.pal[cellFg(v)], bgOf(v)
			key := uint64(fg)<<32 | uint64(bg)
			if v&italic != 0 {
				key ^= 1 << 62
			}
			if v&underline != 0 {
				key ^= 1 << 61
			}
			if key != last {
				if open {
					b.WriteString("</span>")
				}
				style := "color:" + hex(fg) + ";background:" + hex(bg)
				if v&italic != 0 {
					style += ";font-style:italic"
				}
				if v&underline != 0 {
					style += ";text-decoration:underline"
				}
				fmt.Fprintf(&b, "<span style=\"%s\">", style)
				last, open = key, true
			}
			b.WriteString(html.EscapeString(string(a.runeOf(cellCh(v)))))
		}
		if open {
			b.WriteString("</span>")
		}
		b.WriteString("\n")
	}
	b.WriteString("</pre></body></html>\n")
	return []byte(b.String())
}
