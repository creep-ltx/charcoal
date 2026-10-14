package main

import (
	"bytes"
	"compress/gzip"
	"compress/zlib"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"image/png"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/klauspost/compress/zstd"
)

// edPage is a PC page of random cells, with runs (longer than 255 on wide
// pages) and character 1, the iCEDraw run marker.
func edPage(r *rand.Rand, cols, rows int) *Doc {
	d := newDoc(cols, rows)
	for i := range d.cells {
		d.cells[i] = makeCell(r.Intn(256), r.Intn(16), r.Intn(16), 0)
	}
	for y := 0; y < rows; y += 3 { // a run over the whole row
		v := makeCell(r.Intn(256), r.Intn(16), r.Intn(16), 0)
		for x := 0; x < cols; x++ {
			d.cells[y*cols+x] = v
		}
	}
	if len(d.cells) > 2 {
		d.cells[1] = makeCell(1, 0, 0, 0) // character 1 on black: IcyDraw's "fake repeat"
		d.cells[2] = makeCell(1, 7, 1, 0)
	}
	return d
}

// edPalette is 16 colours that 0-63 values keep exactly.
func edPalette(r *rand.Rand) [16]uint32 {
	var p [16]uint32
	for i := range p {
		c := uint32(0xFF000000)
		for sh := 0; sh < 24; sh += 8 {
			v := uint32(r.Intn(64))
			c |= (v<<2 | v>>4) << sh
		}
		p[i] = c
	}
	return p
}

func edFont(r *rand.Rand) *Font {
	f := &Font{id: ownID, name: "random", kind: "pc", syncterm: -1, bits: make([]byte, 4096)}
	r.Read(f.bits)
	return f
}

func sameCells(t *testing.T, what string, want, got *Doc) {
	t.Helper()
	if want.cols != got.cols || max(1, want.rows) != got.rows {
		t.Fatalf("%s: %dx%d read back as %dx%d", what, want.cols, want.rows, got.cols, got.rows)
	}
	for i := range want.cells[:want.cols*want.rows] {
		if cellCh(want.cells[i]) != cellCh(got.cells[i]) || cellAttr(want.cells[i]) != cellAttr(got.cells[i]) {
			t.Fatalf("%s: cell %d,%d %08x read back as %08x", what, i%want.cols, i/want.cols, want.cells[i], got.cells[i])
		}
	}
}

// ansiloveIDF is libansilove's iCEDraw reader (src/loaders/icedraw.c): the
// character/attribute pairs it draws.
func ansiloveIDF(b []byte) (width int, pairs []byte) {
	width = int(b[9])<<8 + int(b[8]) + 1
	state := 0
	for loop := 12; loop < len(b)-4096-48; {
		c := b[loop]
		switch state {
		case 0:
			if c == 1 {
				state = 2
				loop++
			} else {
				pairs = append(pairs, c)
				state = 1
			}
			loop++
		case 1:
			pairs = append(pairs, c)
			state = 0
			loop++
		case 2:
			n := int(c)
			for ; n > 0; n-- {
				pairs = append(pairs, b[loop+2], b[loop+3])
			}
			loop += 4
			state = 0
		}
	}
	return
}

// icyIDF is IcyDraw's iCEDraw reader (icy_engine formats/io/ice_draw.rs).
func icyIDF(b []byte) (width int, pairs []byte) {
	x1, x2 := int(binary.LittleEndian.Uint16(b[4:])), int(binary.LittleEndian.Uint16(b[8:]))
	width = x2 - x1 + 1
	end := len(b) - 4096 - 48
	for o := 12; o+1 < end; {
		n, ch, at := 1, b[o], b[o+1]
		o += 2
		if ch == 1 && at == 0 {
			n = int(b[o]) + int(b[o+1])<<8
			if o+3 >= end {
				break
			}
			ch, at = b[o+2], b[o+3]
			o += 4
		}
		for ; n > 0; n-- {
			pairs = append(pairs, ch, at)
		}
	}
	return
}

func TestIDFWriter(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	for _, size := range [][2]int{{80, 30}, {300, 7}, {1, 1}} {
		d, pal, f := edPage(r, size[0], size[1]), edPalette(r), edFont(r)
		if size[0] == 1 {
			d = newDoc(1, 1)
		}
		out := writeIDF(d, pal, f)
		back, info, err := parseIDF(out)
		if err != nil {
			t.Fatal(err)
		}
		sameCells(t, "idf", d, back)
		if !bytes.Equal(info.font.bits, f.bits) || *info.pal != pal || !info.ice {
			t.Fatalf("idf: font or palette changed")
		}
		var want []byte
		for _, v := range d.cells {
			want = append(want, byte(cellCh(v)), cellAttr(v))
		}
		for name, read := range map[string]func([]byte) (int, []byte){"ansilove": ansiloveIDF, "IcyDraw": icyIDF} {
			w, pairs := read(out)
			if w != d.cols || !bytes.Equal(pairs, want) {
				t.Fatalf("%dx%d: %s reads %d columns, %d cells (want %d)", d.cols, d.rows, name, w, len(pairs)/2, len(want)/2)
			}
		}
	}
}

func TestADFWriter(t *testing.T) {
	r := rand.New(rand.NewSource(2))
	d, pal, f := edPage(r, 80, 40), edPalette(r), edFont(r)
	out, err := writeADF(d, pal, f)
	if err != nil {
		t.Fatal(err)
	}
	back, info, err := parseADF(out)
	if err != nil {
		t.Fatal(err)
	}
	sameCells(t, "adf", d, back)
	if !bytes.Equal(info.font.bits, f.bits) || *info.pal != pal {
		t.Fatalf("adf: font or palette changed")
	}
	// The other 48 EGA colours are the EGA's own (rgbRGB): 0x24 bright red, 0x10 dark green, 0x09 bright blue.
	if out[1+0x24*3] != 63 || out[1+0x10*3+1] != 21 || out[1+0x09*3+2] != 63 || out[1+0x09*3] != 0 {
		t.Fatalf("adf: EGA palette %v", out[1:1+24])
	}
	// libansilove: (length - 4289) / 2 / 80 rows, the font at 193, cells from 4289.
	if (len(out)-4289)/2/80 != d.rows || out[0] != 1 {
		t.Fatalf("adf: ansilove reads %d rows", (len(out)-4289)/2/80)
	}
	if _, err := writeADF(newDoc(81, 1), pal, f); err == nil {
		t.Fatal("adf: 81 columns written")
	}
}

type tndCell struct {
	ch     byte
	fg, bg uint32
}

// ansiloveTundra is libansilove's TundraDraw reader (src/loaders/tundra.c), 80 columns.
func ansiloveTundra(b []byte) map[[2]int]tndCell {
	out := map[[2]int]tndCell{}
	col, row := 0, 0
	var fg, bg uint32
	be := func(i int) uint32 { return uint32(b[i])<<16 | uint32(b[i+1])<<8 | uint32(b[i+2]) }
	for loop := 9; loop < len(b); loop++ {
		if col == 80 {
			col, row = 0, row+1
		}
		c := b[loop]
		ch := c
		switch c {
		case 1:
			row = int(binary.BigEndian.Uint32(b[loop+1:]))
			col = int(binary.BigEndian.Uint32(b[loop+5:]))
			loop += 8
		case 2:
			fg, ch = be(loop+3), b[loop+1]
			loop += 5
		case 4:
			bg, ch = be(loop+3), b[loop+1]
			loop += 5
		case 6:
			fg, bg, ch = be(loop+3), be(loop+7), b[loop+1]
			loop += 9
		}
		if ch != 1 && ch != 2 && ch != 4 && ch != 6 {
			out[[2]int{col, row}] = tndCell{ch, fg, bg}
			col++
		}
	}
	return out
}

// icyTundra is IcyDraw's TundraDraw reader (icy_engine formats/io/tundra.rs).
func icyTundra(b []byte) []tndCell {
	var out []tndCell
	fg, bg := uint32(0xAAAAAA), uint32(0)
	for o := 9; o < len(b); {
		cmd := b[o]
		o++
		if cmd == 1 {
			o += 8
			continue
		}
		if cmd > 1 && cmd <= 6 {
			ch := b[o]
			o++
			if cmd&2 != 0 {
				fg = uint32(b[o+1])<<16 | uint32(b[o+2])<<8 | uint32(b[o+3])
				o += 4
			}
			if cmd&4 != 0 {
				bg = uint32(b[o+1])<<16 | uint32(b[o+2])<<8 | uint32(b[o+3])
				o += 4
			}
			cmd = ch
		}
		out = append(out, tndCell{cmd, fg, bg})
	}
	return out
}

func TestTundraWriter(t *testing.T) {
	r := rand.New(rand.NewSource(3))
	for _, ice := range []bool{true, false} {
		d, pal := edPage(r, 80, 20), edPalette(r)
		out, err := writeTundra(d, pal, ice)
		if err != nil {
			t.Fatal(err)
		}
		back, info, err := parseTundra(out)
		if err != nil {
			t.Fatal(err)
		}
		rgbOf := func(p [16]uint32, i int) uint32 { return p[i] & 0xFFFFFF }
		model := ansiloveTundra(out)
		icy := icyTundra(out)
		if len(icy) != 80*20 {
			t.Fatalf("IcyDraw reads %d cells", len(icy))
		}
		for i, v := range d.cells {
			x, y := i%80, i/80
			fg, bg := rgbOf(pal, cellFg(v)), rgbOf(pal, shownBg(v, ice))
			w := back.cells[i]
			if cellCh(w) != cellCh(v) || rgbOf(*info.pal, cellFg(w)) != fg || rgbOf(*info.pal, cellBg(w)) != bg {
				t.Fatalf("tnd %d,%d: %08x read back as %08x", x, y, v, w)
			}
			if c := icy[i]; c != (tndCell{byte(cellCh(v)), fg, bg}) {
				t.Fatalf("tnd %d,%d: IcyDraw reads %v", x, y, c)
			}
			if ch := cellCh(v); ch != 1 && ch != 2 && ch != 4 && ch != 6 && model[[2]int{x, y}] != (tndCell{byte(ch), fg, bg}) {
				// ansilove skips characters 1, 2, 4 and 6, so the rest of the row moves left:
				// only rows without them can be compared.
				clean := true
				for k := y * 80; k <= i; k++ {
					if c := cellCh(d.cells[k]); c == 1 || c == 2 || c == 4 || c == 6 {
						clean = false
					}
				}
				if clean && y == 0 {
					t.Fatalf("tnd %d,%d: ansilove reads %v", x, y, model[[2]int{x, y}])
				}
			}
		}
	}
	// ansilove on a row without the characters it cannot draw: every cell.
	d := newDoc(80, 2)
	for i := range d.cells {
		d.cells[i] = makeCell(32+i%90, i%16, (i/3)%16, 0)
	}
	pal := paletteByID("vga").colors
	out, _ := writeTundra(d, pal, true)
	m := ansiloveTundra(out)
	for i, v := range d.cells {
		if m[[2]int{i % 80, i / 80}] != (tndCell{byte(cellCh(v)), pal[cellFg(v)] & 0xFFFFFF, pal[cellBg(v)] & 0xFFFFFF}) {
			t.Fatalf("ansilove cell %d: %v", i, m[[2]int{i % 80, i / 80}])
		}
	}
}

func TestXP(t *testing.T) {
	r := rand.New(rand.NewSource(4))
	d, pal := edPage(r, 37, 11), edPalette(r)
	back, info, err := parseXP(writeXP(d, pal, true))
	if err != nil {
		t.Fatal(err)
	}
	for i, v := range d.cells {
		w := back.cells[i]
		if cellCh(w) != cellCh(v) || info.pal[cellFg(w)] != pal[cellFg(v)] || info.pal[cellBg(w)] != pal[cellBg(v)] {
			t.Fatalf("xp cell %d: %08x read back as %08x", i, v, w)
		}
	}
	// Two layers: the second shows where its background is not pink.
	var raw bytes.Buffer
	le := func(v int32) { binary.Write(&raw, binary.LittleEndian, v) }
	le(-1)
	le(2)
	for l := 0; l < 2; l++ {
		le(2)
		le(1)
		for x := 0; x < 2; x++ {
			le(int32('A' + l*2 + x))
			bg := []byte{0, 0, 0x80}
			if l == 1 && x == 0 {
				bg = []byte{0xFF, 0, 0xFF}
			}
			raw.Write(append([]byte{0xFF, 0xFF, 0xFF}, bg...))
		}
	}
	var z bytes.Buffer
	zw := gzip.NewWriter(&z)
	zw.Write(raw.Bytes())
	zw.Close()
	if !isXP("a.xp", z.Bytes()) {
		t.Fatal("not taken for REXPaint")
	}
	back, _, err = parseXP(z.Bytes())
	if err != nil || row(back, 0) != "AD" {
		t.Fatalf("xp layers: %v %q", err, row(back, 0))
	}
}

func TestIcyWriter(t *testing.T) {
	r := rand.New(rand.NewSource(5))
	for _, ice := range []bool{true, false} {
		d, pal, f := edPage(r, 90, 12), edPalette(r), edFont(r)
		if !ice {
			for i, v := range d.cells { // blinking cells: background 8-15
				d.cells[i] = v | 8<<12*uint32(i%2)
			}
		}
		out := writeIcy(d, pal, f, ice, nil)
		if !isIcyDraw(out) || isPicture(out) {
			t.Fatal("an IcyDraw file is not told from a picture")
		}
		if _, err := png.Decode(bytes.NewReader(out)); err != nil {
			t.Fatalf("the PNG does not decode: %v", err)
		}
		doc, err := parseIcyDraw(out)
		if err != nil {
			t.Fatal(err)
		}
		back, info, note := icyPage(doc)
		sameCells(t, "icy", d, back)
		if !bytes.Equal(info.font.bits, f.bits) || *info.pal != pal || info.ice != ice || note != "" {
			t.Fatalf("icy: font, palette or iCE changed (%v %q)", info.ice, note)
		}
	}
}

// icyV0 makes an IcyDraw 0.4 file: base64 records in tEXt / zTXt chunks.
func icyV0(_ *testing.T) []byte {
	var out bytes.Buffer
	out.Write([]byte("\x89PNG\r\n\x1a\n"))
	pngChunk(&out, "IHDR", []byte{0, 0, 0, 1, 0, 0, 0, 1, 8, 6, 0, 0, 0})
	text := func(key string, data []byte, z bool) {
		enc := []byte(base64.StdEncoding.EncodeToString(data))
		if z {
			var c bytes.Buffer
			zw := zlib.NewWriter(&c)
			zw.Write(enc)
			zw.Close()
			pngChunk(&out, "zTXt", append(append([]byte(key), 0, 0), c.Bytes()...))
			return
		}
		pngChunk(&out, "tEXt", append(append([]byte(key), 0), enc...))
	}
	head := []byte{0, 0, 0, 0, 0, 0, 1, 0, 1, 0, 0} // CP437, blink
	head = binary.LittleEndian.AppendUint32(head, 3)
	head = binary.LittleEndian.AppendUint32(head, 2)
	text("ICED", head, false)
	text("PALETTE", []byte("ICE Palette\n#Palette Name: x\n000000\n0000aa\n00aa00\n00aaaa\naa0000\n"), true)
	layer := icyString("Background")
	layer = append(layer, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0) // role, unused, mode, colour
	layer = binary.LittleEndian.AppendUint32(layer, 1)  // visible
	layer = append(layer, 0)                            // transparency
	layer = append(layer, make([]byte, 8)...)           // offset
	layer = binary.LittleEndian.AppendUint32(layer, 3)
	layer = binary.LittleEndian.AppendUint32(layer, 2)
	layer = append(layer, 0, 0)               // default font page
	layer = append(layer, make([]byte, 8)...) // length
	// Row 0: a short cell 'A' (fg 4 = red in BIOS order, bg 1), an invisible one, a long RGB one.
	layer = append(layer, 0x00, 0x40, 'A', 4, 1, 0)
	layer = append(layer, 0x00, 0x80)
	long := binary.LittleEndian.AppendUint32(nil, 'B')
	long = binary.LittleEndian.AppendUint32(long, 0x0000AA)
	long = binary.LittleEndian.AppendUint32(long, 2)
	long = append(long, 0, 1) // font page, fg is RGB
	layer = append(append(layer, 0, 0), long...)
	text("LAYER_0", layer, true)
	// Row 1 in a continuation chunk: 'C' then end of line.
	text("LAYER_0~1", []byte{0x01, 0x40, 'C', 15, 0, 0, 0x00, 0xC0}, false)
	text("END", nil, false)
	pngChunk(&out, "IEND", nil)
	return out.Bytes()
}

func TestIcyReadsOldAndPacked(t *testing.T) {
	b := icyV0(t)
	if !isIcyDraw(b) {
		t.Fatal("a version 0 file is not taken for IcyDraw's")
	}
	doc, err := parseIcyDraw(b)
	if err != nil {
		t.Fatal(err)
	}
	d, info, _ := icyPage(doc)
	if row(d, 0) != "A B" || row(d, 1) != "C" || info.ice {
		t.Fatalf("v0: %q %q ice %v", row(d, 0), row(d, 1), info.ice)
	}
	if v := d.get(0, 0); cellFg(v) != 1 || cellBg(v) != 4 { // red on blue, in cell (ANSI) order
		t.Fatalf("v0 colours: %08x", v)
	}
	if v := d.get(2, 0); cellFg(v) != 4 { // RGB 0000AA: the palette's blue
		t.Fatalf("v0 RGB colour: %08x", v)
	}
	if v := d.get(0, 1); cellFg(v) != 15 || cellAttr(v)&0x80 != 0 { // bold: bright
		t.Fatalf("v0 bold: %08x", v)
	}
	// Version 1 with zstd records, as IcyDraw writes by default.
	r := rand.New(rand.NewSource(6))
	page, pal, f := edPage(r, 20, 5), edPalette(r), edFont(r)
	plain := writeIcy(page, pal, f, true, nil)
	types, datas, err := pngChunks(plain)
	if err != nil {
		t.Fatal(err)
	}
	enc, _ := zstd.NewWriter(nil)
	var out bytes.Buffer
	out.Write(plain[:8])
	for i, typ := range types {
		data := datas[i]
		if typ == "icYD" {
			n := int(binary.LittleEndian.Uint16(data[1:]))
			key, body := string(data[3:3+n]), data[3+n+4:]
			if key == "ICED" {
				body = append([]byte{}, body...)
				body[2] = 2
			} else if key != "END" {
				body = enc.EncodeAll(body, nil)
			}
			data = icyRecord(key, body)
		}
		pngChunk(&out, typ, data)
	}
	doc, err = parseIcyDraw(out.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	back, _, _ := icyPage(doc)
	sameCells(t, "icy zstd", page, back)
}

// psciModel is what Playscii's ArtFromDisk takes from a file (art.py): the keys
// it reads without a default must be there, and every frame must have as many
// layers as the first, with width x height tiles.
func psciModel(t *testing.T, b []byte) {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"width", "height", "charset", "palette", "camera", "frames"} {
		if _, ok := m[k]; !ok {
			t.Fatalf("psci: no %q", k)
		}
	}
	if len(m["camera"].([]any)) != 3 {
		t.Fatal("psci: the camera is not x, y, z")
	}
	w, h := int(m["width"].(float64)), int(m["height"].(float64))
	frames := m["frames"].([]any)
	nl := len(frames[0].(map[string]any)["layers"].([]any))
	for _, f := range frames {
		fr := f.(map[string]any)
		if _, ok := fr["delay"]; !ok {
			t.Fatal("psci: a frame without delay")
		}
		layers := fr["layers"].([]any)
		if len(layers) != nl {
			t.Fatal("psci: frames with different layers")
		}
		for _, l := range layers {
			lm := l.(map[string]any)
			if _, ok := lm["z"]; !ok {
				t.Fatal("psci: a layer without z")
			}
			tiles := lm["tiles"].([]any)
			if len(tiles) != w*h {
				t.Fatalf("psci: %d tiles for %dx%d", len(tiles), w, h)
			}
			for _, tl := range tiles {
				for _, k := range []string{"char", "fg", "bg"} {
					if _, ok := tl.(map[string]any)[k]; !ok {
						t.Fatalf("psci: a tile without %s", k)
					}
				}
			}
		}
	}
}

func TestPsciRoundTrip(t *testing.T) {
	r := rand.New(rand.NewSource(7))
	// PC: every character and colour.
	d := edPage(r, 30, 6)
	out, missed := writePsci([]*Doc{d}, []int{250}, psciPage{mode: "pc", pal: paletteByID("vga").colors, ice: true})
	psciModel(t, out)
	art, err := parsePsci(out)
	if err != nil || missed != 0 || art.mode != "pc" || len(art.docs) != 1 || art.holds[0] != 250 {
		t.Fatalf("pc: %v missed %d", err, missed)
	}
	sameCells(t, "psci pc", d, art.docs[0])

	// Amiga: what Topaz has.
	d = newDoc(16, 2)
	for i := range d.cells {
		c := 32 + r.Intn(192)
		if c >= 128 {
			c += 32 // 128-159 are not in amiga_topaz
		}
		d.cells[i] = makeCell(c, r.Intn(16), r.Intn(16), 0)
	}
	d.cells[0] = makeCell(0x85, 1, 2, 0) // not in amiga_topaz
	out, missed = writePsci([]*Doc{d}, nil, psciPage{mode: "amiga", pal: paletteByID("vga").colors, ice: true})
	art, err = parsePsci(out)
	if err != nil || missed != 1 || art.mode != "amiga" {
		t.Fatalf("amiga: %v missed %d", err, missed)
	}
	d.cells[0] = makeCell(32, 1, 2, 0)
	sameCells(t, "psci amiga", d, art.docs[0])

	// C64, both sets, reversed characters, three frames with their holds.
	for _, lower := range []bool{false, true} {
		var docs []*Doc
		for k := 0; k < 3; k++ {
			d := newDoc(40, 25)
			d.bg, d.border = 6, 6
			for i := range d.cells {
				fg := r.Intn(16)
				for fg == d.bg {
					fg = r.Intn(16)
				}
				d.cells[i] = makeCell(r.Intn(256), fg, 0, 0)
			}
			docs = append(docs, d)
		}
		out, _ := writePsci(docs, []int{100, 2000, 40}, psciPage{mode: "c64", machine: "c64", lower: lower})
		psciModel(t, out)
		art, err := parsePsci(out)
		if err != nil || art.mode != "c64" || art.lower != lower || len(art.docs) != 3 || art.holds[1] != 2000 {
			t.Fatalf("c64 lower %v: %v", lower, err)
		}
		for k, d := range docs {
			got := art.docs[k]
			if got.bg != d.bg {
				t.Fatalf("c64 background %d read back as %d", d.bg, got.bg)
			}
			for i, v := range d.cells {
				if w := got.cells[i]; cellCh(w) != cellCh(v) || cellFg(w) != cellFg(v) {
					// A space and a reversed space look the same as their reverses' twins.
					t.Fatalf("c64 frame %d cell %d: %08x read back as %08x", k, i, v, w)
				}
			}
		}
	}

	// Atari: characters, reverse, ink and background.
	d = newDoc(40, 24)
	d.bg, d.ink, d.border, d.glyph0 = atariBg, atariInk, 0, true
	for i := range d.cells {
		d.cells[i] = makeCell(r.Intn(256), 0, 0, 0)
	}
	d.cells[0] = makeCell(0x20, 0, 0, 0) // the background is the commonest colour
	out, _ = writePsci([]*Doc{d}, nil, psciPage{mode: "atari", pal: atariPalette.colors})
	art, err = parsePsci(out)
	if err != nil || art.mode != "atari" {
		t.Fatal(err)
	}
	got := art.docs[0]
	if got.bg != d.bg || got.ink != d.ink {
		t.Fatalf("atari colours %d/%d read back as %d/%d", d.ink, d.bg, got.ink, got.bg)
	}
	for i, v := range d.cells {
		if cellCh(got.cells[i]) != cellCh(v) {
			t.Fatalf("atari cell %d: %02x read back as %02x", i, cellCh(v), cellCh(got.cells[i]))
		}
	}
}

// Playscii's own example files (MIT, from its art/ folder) open: layers put
// together, frames kept.
func TestPsciPlaysciiFiles(t *testing.T) {
	b, err := os.ReadFile("testdata/playscii/hello1.psci")
	if err != nil {
		t.Fatal(err)
	}
	art, err := parsePsci(b)
	if err != nil {
		t.Fatal(err)
	}
	if art.mode != "c64" || len(art.docs) != 7 || art.docs[0].cols != 8 || !strings.Contains(art.note, "layers") {
		t.Fatalf("hello1: %s %d frames %q", art.mode, len(art.docs), art.note)
	}
	drawn := 0
	for _, d := range art.docs {
		for _, v := range d.cells {
			if c := cellCh(v) & 127; c != 32 && c != 96 {
				drawn++
			}
		}
	}
	if drawn < 20 {
		t.Fatalf("hello1: %d drawn cells", drawn)
	}
}

func TestMarqPet(t *testing.T) {
	r := rand.New(rand.NewSource(8))
	d := newDoc(40, 25)
	d.bg, d.border = 3, 9
	for i := range d.cells {
		d.cells[i] = makeCell(r.Intn(256), r.Intn(16), 0, 0)
	}
	out := writeMarqPet(d, true)
	if len(out) != 2005 || out[0] != 40 || out[1] != 25 || out[2] != 9 || out[3] != 3 || out[4] != 1 || !isMarqPet("x.PET", out) {
		t.Fatalf("pet head %v", out[:5])
	}
	back, lower, err := parseMarqPet(out)
	if err != nil || lower != 1 || back.bg != 3 || back.border != 9 {
		t.Fatalf("pet: %v", err)
	}
	for i, v := range d.cells {
		if back.cells[i] != v {
			t.Fatalf("pet cell %d", i)
		}
	}
	bas := string(writeMarqBas(d, true))
	if !strings.Contains(bas, "35 poke 53272,23\n") || !strings.Contains(bas, "2570 end\n") || strings.Count(bas, " data ") != 250 {
		t.Fatalf("bas:\n%s", bas[:200])
	}
	asm := string(writeMarqAsm(d, false))
	if !strings.Contains(asm, "jumi:\tjmp jumi") || strings.Count(asm, "\t!byte ") != 51 || strings.Contains(asm, "$d018") {
		t.Fatalf("asm:\n%s", asm[:300])
	}
	if small := string(writeMarqAsm(newDoc(10, 2), false)); strings.Contains(small, "jmp") || !strings.HasPrefix(small, "; Border, bg, chars, colors\nimg:\n") {
		t.Fatalf("asm data:\n%s", small)
	}
}

func TestPetmateJSON(t *testing.T) {
	r := rand.New(rand.NewSource(9))
	var scr []PetScreen
	for k := 0; k < 2; k++ {
		d := newDoc(40, 25)
		d.bg, d.border = k, 5
		for i := range d.cells {
			d.cells[i] = makeCell(r.Intn(256), r.Intn(16), 0, 0)
		}
		scr = append(scr, PetScreen{doc: d, lower: k, name: "screen", fb: -1})
	}
	out := writePetmateJSON(scr)
	if !isPetmateJSON(out) {
		t.Fatal("not taken for Petmate's JSON")
	}
	back, err := parsePetmateJSON(out)
	if err != nil || len(back) != 2 || back[1].lower != 1 || back[1].name != "screen" {
		t.Fatalf("json: %v", err)
	}
	for k := range scr {
		for i, v := range scr[k].doc.cells {
			if back[k].doc.cells[i] != v {
				t.Fatalf("json screen %d cell %d", k, i)
			}
		}
	}
}

// Through CharEdit: each format saved, opened again, gives the page; Ctrl+S then
// writes the same format again.
func TestEditorFormatsInCharEdit(t *testing.T) {
	r := rand.New(rand.NewSource(10))
	dir := t.TempDir()
	for _, c := range []struct{ fmtID, ext string }{{"idf", ".idf"}, {"adf", ".adf"}, {"tnd", ".tnd"}, {"icy", ".icy"}, {"xp", ".xp"}, {"psci", ".psci"}} {
		a := testApp(t, "pc")
		a.ice = true
		a.doc = edPage(r, 80, 9)
		a.setPalette()
		p := filepath.Join(dir, "page"+c.ext)
		if !a.writeOut(p, c.fmtID) {
			t.Fatalf("%s: not saved: %s", c.fmtID, a.message)
		}
		want, pal := a.doc.clone(), a.pal
		b, _ := os.ReadFile(p)
		a.focusTab(a.newTab("pc"))
		a.loadBytes(b, filepath.Base(p), p, "auto")
		if a.mode != "pc" || a.saveFmt != c.fmtID || a.srcKind != c.fmtID {
			t.Fatalf("%s: opened as %s, saves as %s", c.fmtID, a.mode, a.saveFmt)
		}
		a.setPalette()
		for i, v := range want.cells {
			w := a.doc.cells[i]
			if cellCh(w) != cellCh(v) || a.pal[cellFg(w)]&0xFFFFFF != pal[cellFg(v)]&0xFFFFFF || a.pal[cellBg(w)]&0xFFFFFF != pal[cellBg(v)]&0xFFFFFF {
				t.Fatalf("%s: cell %d %08x opened as %08x", c.fmtID, i, v, w)
			}
		}
		a.doc.set(0, 0, makeCell('Z', 1, 2, 0))
		a.dirty = true
		a.save(false)
		if b2, _ := os.ReadFile(p); editorKind(p, b2) != c.fmtID {
			t.Fatalf("%s: Ctrl+S wrote another format", c.fmtID)
		}
	}
	// The C64 formats.
	for _, c := range []struct{ fmtID, ext string }{{"pet", ".pet"}, {"pmjson", ".json"}, {"psci", ".psci"}} {
		a := testApp(t, "c64")
		a.hasC64 = true
		a.machine = "c64"
		d := newDoc(40, 25)
		d.bg, d.border = 6, 14
		for i := range d.cells {
			d.cells[i] = makeCell(r.Intn(128), 1+r.Intn(5), 0, 0)
		}
		a.doc = d
		p := filepath.Join(dir, "c64"+c.ext)
		if !a.writeOut(p, c.fmtID) {
			t.Fatalf("%s: not saved: %s", c.fmtID, a.message)
		}
		b, _ := os.ReadFile(p)
		a.focusTab(a.newTab("pc"))
		a.hasC64 = true
		a.loadBytes(b, filepath.Base(p), p, "auto")
		if a.mode != "c64" || a.saveFmt != c.fmtID {
			t.Fatalf("%s: opened as %s, saves as %s", c.fmtID, a.mode, a.saveFmt)
		}
		if a.doc.bg != 6 {
			t.Fatalf("%s: background %d", c.fmtID, a.doc.bg)
		}
		for i, v := range d.cells {
			if w := a.doc.cells[i]; cellCh(w) != cellCh(v) || cellFg(w) != cellFg(v) {
				t.Fatalf("%s: cell %d %08x opened as %08x", c.fmtID, i, v, w)
			}
		}
	}
}

// Files IcyDraw wrote (testdata/icydraw/README) open as the same page as the XBin they came from.
func TestReadsIcyDrawsFiles(t *testing.T) {
	b, err := os.ReadFile("testdata/alove/bs-alove.xb")
	if err != nil {
		t.Fatal(err)
	}
	ref, refInfo, err := parseXBin(b)
	if err != nil {
		t.Fatal(err)
	}
	for _, ext := range []string{"icy", "idf", "adf", "tnd"} {
		b, err := os.ReadFile("testdata/icydraw/icydraw-alove." + ext)
		if err != nil {
			t.Fatal(err)
		}
		if k := editorKind("icydraw-alove."+ext, b); k != ext {
			t.Fatalf("%s taken for %q", ext, k)
		}
		d, info, _, err := parseEditorPC(ext, b)
		if err != nil {
			t.Fatalf("%s: %v", ext, err)
		}
		if d.cols != ref.cols || d.usedRows() != ref.usedRows() {
			t.Fatalf("%s: %dx%d, the XBin %dx%d", ext, d.cols, d.usedRows(), ref.cols, ref.usedRows())
		}
		pal := *refInfo.pal
		got := pal
		if info.pal != nil {
			got = *info.pal
		}
		// IcyDraw's save evens out colours that do not show (the foreground of a space,
		// the background of a full block), so cells are compared as they look.
		glyph := func(c int) (blank, full bool) {
			blank, full = true, true
			for r := 0; r < 16; r++ {
				blank = blank && refInfo.font.bits[c*16+r] == 0
				full = full && refInfo.font.bits[c*16+r] == 0xFF
			}
			return
		}
		for i := range ref.cells[:ref.cols*ref.usedRows()] {
			v, w := ref.cells[i], d.cells[i]
			blank, full := glyph(cellCh(v))
			if b2, _ := glyph(cellCh(w)); blank && b2 { // NUL is written as a space
				w = makeCell(cellCh(v), cellFg(w), cellBg(w), 0)
			}
			if cellCh(v) != cellCh(w) || !blank && pal[cellFg(v)]&0xFFFFFF != got[cellFg(w)]&0xFFFFFF || !full && pal[cellBg(v)]&0xFFFFFF != got[cellBg(w)]&0xFFFFFF {
				if ext == "tnd" && cellCh(v) >= 1 && cellCh(v) <= 6 {
					continue
				}
				t.Fatalf("%s cell %d,%d: %08x, the XBin %08x", ext, i%ref.cols, i/ref.cols, w, v)
			}
		}
		if ext != "tnd" && !bytes.Equal(info.font.bits, refInfo.font.bits) {
			t.Fatalf("%s: the font differs", ext)
		}
	}
}

// FuzzEditorFormats sends bytes to the readers of other editors' files only (they
// are in FuzzParsers too, which runs slowly through all the stream readers):
//
//	systemd-run --user --scope -q -p MemoryMax=6G go test -run XXX -fuzz FuzzEditorFormats -fuzztime 60s -parallel 4 .
func FuzzEditorFormats(f *testing.F) {
	d := edPage(rand.New(rand.NewSource(14)), 5, 3)
	pal := paletteByID("vga").colors
	f.Add(writeIcy(d, pal, fontByID("vga"), true, nil))
	f.Add(icyV0(nil))
	f.Add(writeXP(d, pal, true))
	for _, m := range []psciPage{{mode: "c64", machine: "c64"}, {mode: "pc"}, {mode: "atari", pal: atariPalette.colors}} {
		out, _ := writePsci([]*Doc{d, d}, nil, m)
		f.Add(out)
	}
	f.Add(writeMarqPet(d, false))
	f.Add(writePetmateJSON([]PetScreen{{doc: d}}))
	f.Fuzz(func(t *testing.T, b []byte) {
		start := time.Now()
		defer func() {
			if el := time.Since(start); el > 300*time.Millisecond {
				t.Fatalf("took %v", el)
			}
		}()
		check := func(name string, docs ...*Doc) {
			for _, d := range docs {
				if d != nil && (d.rows*d.cols > maxPageCells || len(d.cells) < d.rows*d.cols) {
					t.Fatalf("%s made a page of %dx%d", name, d.cols, d.rows)
				}
			}
		}
		if d, _, err := parseIcy(b); err == nil {
			check("icy", d)
		}
		if d, _, err := parseXP(b); err == nil {
			check("xp", d)
		}
		if art, err := parsePsci(b); err == nil {
			check("psci", art.docs...)
		}
		if d, _, err := parseMarqPet(b); err == nil {
			check("pet", d)
		}
		if scr, err := parsePetmateJSON(b); err == nil {
			for _, s := range scr {
				check("petmate json", s.doc)
			}
		}
	})
}

// The SAUCE record of an iCEDraw file is kept, without comment lines (ansilove
// would look for the font and palette in the wrong place).
func TestEditorSauce(t *testing.T) {
	a := testApp(t, "pc")
	a.doc = edPage(rand.New(rand.NewSource(15)), 80, 3)
	a.info = Info{writeSauce: true, title: "Title", author: "Me", notes: []string{"a comment"}}
	p := filepath.Join(t.TempDir(), "s.idf")
	if !a.writeOut(p, "idf") {
		t.Fatal(a.message)
	}
	b, _ := os.ReadFile(p)
	if bytes.Contains(b, []byte("COMNT")) || !bytes.HasPrefix(b[len(b)-128:], []byte("SAUCE00")) || b[len(b)-129] != 0x1A {
		t.Fatal("idf: SAUCE with comments, or none")
	}
	if _, _, err := parseIDF(b); err != nil {
		t.Fatal(err)
	}
	a.focusTab(a.newTab("pc"))
	a.loadBytes(b, "s.idf", p, "auto")
	if a.info.title != "Title" || a.info.author != "Me" || !a.info.writeSauce {
		t.Fatalf("idf: SAUCE info %+v", a.info)
	}
}

// xterm's 256 colours start in ANSI order: 1 is red, 4 blue (an IcyDraw 'x' colour uses them).
func TestXterm256ANSIOrder(t *testing.T) {
	for n, want := range map[int]uint32{1: 0xAA0000, 4: 0x0000AA, 9: 0xFF5555, 12: 0x5555FF, 16: 0, 231: 0xFFFFFF, 232: 0x080808} {
		if got := xterm256(n); got != want {
			t.Errorf("xterm256(%d) = %06x, want %06x", n, got, want)
		}
	}
}

// A .tnd of more than 16 colours is read with the VGA's numbers (the nearest);
// saved again it has only VGA colours, and those must come back with the same
// numbers, not numbered in the order they first appear (the big check's
// avg-thebatman.tnd: cell 0,0 came back as colours 0/1 instead of 8/6).
func TestRGBColoursKeepVGANumbers(t *testing.T) {
	vga := paletteByID("vga").colors
	tnd := append([]byte{24}, "TUNDRA24"...)
	be := func(c uint32) []byte { return []byte{0, byte(c >> 16), byte(c >> 8), byte(c)} }
	for i := 0; i < 160; i++ {
		fg, bg := vga[(8+i)%16]&0xFFFFFF^1, vga[(6+i*3)%16]&0xFFFFFF
		if i == 40 {
			fg = 0x123456 // the 17th colour, nearest of VGA's
		}
		tnd = append(append(append(tnd, 6, byte('A'+i%26)), be(fg)...), be(bg)...)
	}
	d, info, err := parseTundra(tnd)
	if err != nil || info.pal != nil {
		t.Fatalf("tnd: %v, own palette %v", err, info.pal != nil)
	}
	if v := d.cells[0]; cellFg(v) != 8 || cellBg(v) != 6 {
		t.Fatalf("tnd cell 0: %08x", v)
	}
	out, _ := writeTundra(d, vga, true)
	back, info, err := parseTundra(out)
	if err != nil || info.pal == nil {
		t.Fatalf("tnd again: %v", err)
	}
	for i, v := range d.cells {
		if back.cells[i] != v {
			t.Fatalf("tnd again cell %d: %08x became %08x", i, v, back.cells[i])
		}
	}
	// REXPaint: VGA colours keep their numbers, others take the free ones.
	x := newDoc(4, 1)
	x.cells[0] = makeCell('a', 9, 12, 0)
	x.cells[1] = makeCell('b', 0, 3, 0)
	pal := vga
	pal[3] = 0xFF102030 // not a VGA colour: takes the first free number
	xd, xinfo, err := parseXP(writeXP(x, pal, true))
	if err != nil {
		t.Fatal(err)
	}
	if v := xd.cells[0]; cellFg(v) != 9 || cellBg(v) != 12 {
		t.Fatalf("xp cell 0: %08x", v)
	}
	if v := xd.cells[1]; cellFg(v) != 0 || xinfo.pal[cellBg(v)]&0xFFFFFF != 0x102030 {
		t.Fatalf("xp cell 1: %08x", v)
	}
}

// Playscii charsets CharEdit has no table of its own never stop a file opening:
// jpetscii (the big check's om-untitled.psci) is matched to the C64's sets, any
// other opens as a PC page with a note.
func TestPsciOtherCharsets(t *testing.T) {
	psci := func(charset string, chars ...int) []byte {
		var tiles []string
		for _, c := range chars {
			tiles = append(tiles, `{"char":`+strconv.Itoa(c)+`,"fg":2,"bg":1,"xform":0}`)
		}
		return []byte(`{"active_frame":0,"active_layer":0,"camera":[0,0,0],"charset":"` + charset +
			`","palette":"c64_original","width":` + strconv.Itoa(len(chars)) + `,"height":1,"frames":[{"delay":0.1,"layers":[{"z":0,"visible":1,"name":"l","tiles":[` +
			strings.Join(tiles, ",") + `]}]}]}`)
	}
	// jpetscii: 1-26 are the upper set's A-Z, 29 is a glyph of its own.
	art, err := parsePsci(psci("jpetscii", 1, 2, 3, 29))
	if err != nil || art.mode != "c64" || art.lower {
		t.Fatalf("jpetscii: %v %+v", err, art)
	}
	if got := []int{cellCh(art.docs[0].cells[0]), cellCh(art.docs[0].cells[1])}; got[0] != 1 || got[1] != 2 {
		t.Fatalf("jpetscii glyphs read as %v", got)
	}
	if !strings.Contains(art.note, "nearest") {
		t.Fatalf("jpetscii note %q", art.note)
	}
	art, err = parsePsci(psci("apple2", 65, 66))
	if err != nil || art.mode != "pc" || !strings.Contains(art.note, `"apple2"`) || cellCh(art.docs[0].cells[0]) != 65 {
		t.Fatalf("apple2: %v %+v", err, art)
	}
}
