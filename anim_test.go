package main

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image/gif"
	"image/png"
	"testing"
)

// animPage is a PC page of four frames: a red block moving right, and a copy
// of the third frame (merged into it).
func animPage(t *testing.T) *App {
	a := testApp(t, "pc")
	a.doc.set(0, 0, makeCell(0xDB, 4, 0, 0))
	for i := 1; i < 3; i++ {
		a.dupFrame()
		a.doc.set(i-1, 0, makeCell(32, 7, 0, 0))
		a.doc.set(i, 0, makeCell(0xDB, 4, 0, 0))
	}
	a.setHold(300)
	a.dupFrame() // the same as frame 3
	a.setHold(200)
	return a
}

func TestGIFExport(t *testing.T) {
	a := animPage(t)
	b, err := a.encode("gif")
	if err != nil {
		t.Fatal(err)
	}
	g, err := gif.DecodeAll(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Image) != 3 || g.LoopCount != 0 {
		t.Fatalf("%d frames, loop %d", len(g.Image), g.LoopCount)
	}
	if g.Delay[0] != 10 || g.Delay[1] != 10 || g.Delay[2] != 50 {
		t.Fatalf("delays %v (the copy of frame 3 adds its 200 ms to it)", g.Delay)
	}
	if g.Config.Width != 80*a.cellW() || g.Config.Height != 25*a.cellH() {
		t.Fatalf("canvas %dx%d", g.Config.Width, g.Config.Height)
	}
	// Frame 2 is only the part that changed: the two cells.
	if r := g.Image[1].Rect; r.Min.X != 0 || r.Dx() != 2*a.cellW() || r.Dy() != a.cellH() {
		t.Fatalf("frame 2 is %v", r)
	}
	if c := g.Image[0].At(4, 4); rgbaOf(c) != a.pal[4]&0xFFFFFF {
		t.Fatalf("a cell's colour %v, want %06x", c, a.pal[4])
	}
	a.noLoop = true
	b, _ = a.encode("gif")
	if g, _ := gif.DecodeAll(bytes.NewReader(b)); g.LoopCount != -1 {
		t.Fatalf("no loop: %d", g.LoopCount)
	}
}

func rgbaOf(c interface{ RGBA() (r, g, b, a uint32) }) uint32 {
	r, g, b, _ := c.RGBA()
	return (r>>8)<<16 | (g>>8)<<8 | b>>8
}

func TestAPNGExport(t *testing.T) {
	a := animPage(t)
	b, err := a.encode("apng")
	if err != nil {
		t.Fatal(err)
	}
	// A viewer without APNG: the first frame as a PNG.
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil || img.Bounds().Dx() != 80*a.cellW() || rgbaOf(img.At(4, 4)) != a.pal[4]&0xFFFFFF {
		t.Fatalf("plain PNG: %v", err)
	}
	types, datas, err := pngChunks(b)
	if err != nil {
		t.Fatal(err)
	}
	// Every CRC is right.
	for i := 8; i < len(b); {
		n := int(binary.BigEndian.Uint32(b[i:]))
		if crc32.ChecksumIEEE(b[i+4:i+8+n]) != binary.BigEndian.Uint32(b[i+8+n:]) {
			t.Fatalf("bad CRC in %s", b[i+4:i+8])
		}
		i += 12 + n
	}
	seq, fctl, ihdr, plte := -1, 0, []byte(nil), []byte(nil)
	var delays []int
	for i, ty := range types {
		d := datas[i]
		switch ty {
		case "IHDR":
			ihdr = d
		case "PLTE":
			plte = d
		case "acTL":
			if n := binary.BigEndian.Uint32(d); n != 3 {
				t.Fatalf("acTL says %d frames", n)
			}
			if p := binary.BigEndian.Uint32(d[4:]); p != 0 {
				t.Fatalf("plays %d", p)
			}
			if fctl > 0 {
				t.Fatal("acTL after an fcTL")
			}
		case "fcTL", "fdAT":
			s := int(binary.BigEndian.Uint32(d))
			if s != seq+1 {
				t.Fatalf("%s sequence %d after %d", ty, s, seq)
			}
			seq = s
			if ty == "fcTL" {
				fctl++
				delays = append(delays, int(binary.BigEndian.Uint16(d[20:]))*1000/int(binary.BigEndian.Uint16(d[22:])))
			} else {
				// Its data with the IHDR of its size is a PNG of its own.
				fc := datas[i-1]
				h := append([]byte{}, ihdr...)
				copy(h, fc[4:12])
				var one bytes.Buffer
				one.WriteString("\x89PNG\r\n\x1a\n")
				pngChunk(&one, "IHDR", h)
				pngChunk(&one, "PLTE", plte)
				pngChunk(&one, "IDAT", d[4:])
				pngChunk(&one, "IEND", nil)
				part, err := png.Decode(&one)
				if err != nil {
					t.Fatalf("frame data: %v", err)
				}
				if part.Bounds().Dx() != 2*a.cellW() {
					t.Fatalf("frame part %v", part.Bounds())
				}
			}
		}
	}
	if fctl != 3 || len(delays) != 3 || delays[0] != 100 || delays[2] != 500 {
		t.Fatalf("%d fcTL, delays %v", fctl, delays)
	}
	a.noLoop = true
	b, _ = a.encode("apng")
	types, datas, _ = pngChunks(b)
	for i, ty := range types {
		if ty == "acTL" && binary.BigEndian.Uint32(datas[i][4:]) != 1 {
			t.Fatal("no loop: plays is not 1")
		}
	}
}

// A one-picture page saved as GIF is an export: the page keeps its own file.
func TestGIFIsExport(t *testing.T) {
	a := animPage(t)
	p := t.TempDir() + "/a.gif"
	if !a.writeOut(p, "gif") || a.path == p || a.saveFmt == "gif" {
		t.Fatalf("path %q fmt %q: %s", a.path, a.saveFmt, a.message)
	}
}
