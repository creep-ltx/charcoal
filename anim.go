package main

// Animations out: every frame drawn as a picture and written as an animated
// GIF (Go's image/gif) or APNG (apng.go), each frame shown for its hold time.

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/gif"
)

// renderDoc draws a page as the PNG export shows it: with the machine's
// border on screen pages, at most 32000 pixels tall.
func (a *App) renderDoc(d *Doc, z int) *FB {
	z = a.cpcZoom(z)
	keep := a.doc
	a.doc = d // drawCell takes a screen page's colours from the page
	defer func() { a.doc = keep }()
	H := a.cellH()
	rows := max(1, min(d.rows, 32000/(H*z)))
	if a.screen() {
		rows = max(rows, map[bool]int{true: 25, false: 24}[a.c64()])
	}
	w, h := d.cols*a.cellW()*z, rows*H*z
	bx, top, bottom := a.borderSize(w, h, z)
	fb := &FB{}
	fb.resize(w+2*bx, h+top+bottom)
	if bx > 0 { // with the machine's border around it
		fb.fill(Rect{0, 0, fb.w, fb.h}, a.borderPx(d))
	}
	a.paintCells(fb, bx, top, z, func(x, y int) uint32 { return d.get(x, y) }, d.cols, rows)
	return fb
}

// cpcZoom doubles a CPC mode 0 or 1 page in pictures, so every mode comes out
// at the size of mode 2's 640 x 400 (the monitor's lines, each shown twice).
func (a *App) cpcZoom(z int) int {
	if a.cpc() && a.cpcSet.mode != 2 {
		return 2 * z
	}
	return z
}

// borderSize is the border left and right, at the top and at the bottom of a
// w x h picture: none on Amiga and PC pages, 32 pixels (times the zoom) on
// other machines' pages. A CPC's is as its monitor shows it, placed as in
// Caprice Forever's screenshots (the AMSCII compos' own): 768 x 544, the
// 640 x 400 screen at 64,82.
func (a *App) borderSize(w, h, z int) (int, int, int) {
	switch {
	case a.cpc():
		return w / 10, h * 82 / 400, h * 62 / 400
	case a.screen() || a.zx():
		return 32 * z, 32 * z, 32 * z
	}
	return 0, 0, 0
}

// animFrame is one picture of an animation: the part that changed from the
// picture before (all of it for the first), on the whole canvas's palette.
type animFrame struct {
	img   *image.Paletted // its Rect is where it goes on the canvas
	delay int             // ms
}

// maxAnimBytes caps the pictures an export keeps at once.
const maxAnimBytes = 512 << 20

// animFrames draws every frame and keeps only what changes; a frame the same
// as the one before only makes that one last longer.
func (a *App) animFrames(z int) (frames []animFrame, w, h int, pal color.Palette, err error) {
	a.endPlay()
	a.keepCase()
	docs := a.allDocs()
	lowerWas, blinkWas := a.lower(), a.blink
	defer func() {
		if a.c64() {
			a.setCase(lowerWas)
		}
		a.blink = blinkWas
	}()
	a.blink = true // flashing and blinking cells in their "on" look
	for _, d := range docs {
		w, h = max(w, d.cols), max(h, d.rows)
	}
	render := func(i int) *FB {
		if a.frames != nil && a.frames[i].lower >= 0 && a.c64() {
			a.setCase(a.frames[i].lower == 1)
		}
		return a.renderDoc(docs[i], z)
	}
	// The canvas is the largest picture; smaller ones are padded with their border (or black).
	cw, ch := 0, 0
	for i := range docs {
		d := docs[i]
		fw, fh := a.renderSize(d, z)
		cw, ch = max(cw, fw), max(ch, fh)
	}
	index := map[uint32]uint8{}
	var prev []uint8
	total := 0
	for i := range docs {
		fb := render(i)
		pad := uint32(0xFF000000)
		if a.screen() || a.zx() || a.cpc() {
			pad = a.borderPx(docs[i])
		}
		cur := make([]uint8, cw*ch)
		for y := 0; y < ch; y++ {
			for x := 0; x < cw; x++ {
				c := pad
				if x < fb.w && y < fb.h {
					c = fb.px[y*fb.w+x]
				}
				c |= 0xFF000000
				k, ok := index[c]
				if !ok {
					if len(pal) == 256 {
						k = uint8(color.Palette(pal).Index(rgb(c)))
					} else {
						k = uint8(len(pal))
						index[c] = k
						pal = append(pal, rgb(c))
					}
				}
				cur[y*cw+x] = k
			}
		}
		hold := a.holdOf(i)
		r := image.Rect(0, 0, cw, ch)
		if prev != nil {
			r = changedBox(prev, cur, cw, ch)
			if r.Empty() { // the same picture: the one before stays longer
				frames[len(frames)-1].delay += hold
				continue
			}
		}
		img := &image.Paletted{Pix: make([]uint8, r.Dx()*r.Dy()), Stride: r.Dx(), Rect: r}
		for y := r.Min.Y; y < r.Max.Y; y++ {
			copy(img.Pix[(y-r.Min.Y)*r.Dx():], cur[y*cw+r.Min.X:y*cw+r.Max.X])
		}
		if total += len(img.Pix); total > maxAnimBytes {
			return nil, 0, 0, nil, fmt.Errorf("the animation is too large to write (%d frames of %dx%d)", len(docs), cw, ch)
		}
		frames = append(frames, animFrame{img, hold})
		prev = cur
	}
	for _, f := range frames {
		f.img.Palette = pal
	}
	return frames, cw, ch, pal, nil
}

// renderSize is the size renderDoc gives a page, without drawing it.
func (a *App) renderSize(d *Doc, z int) (int, int) {
	z = a.cpcZoom(z)
	H := a.cellH()
	rows := max(1, min(d.rows, 32000/(H*z)))
	if a.screen() {
		rows = max(rows, map[bool]int{true: 25, false: 24}[a.c64()])
	}
	w, h := d.cols*a.cellW()*z, rows*H*z
	bx, top, bottom := a.borderSize(w, h, z)
	return w + 2*bx, h + top + bottom
}

func rgb(c uint32) color.RGBA { return color.RGBA{uint8(c >> 16), uint8(c >> 8), uint8(c), 255} }

// changedBox is the smallest rectangle holding every pixel that differs.
func changedBox(prev, cur []uint8, w, h int) image.Rectangle {
	x0, y0, x1, y1 := w, h, -1, -1
	for y := 0; y < h; y++ {
		row, prow := cur[y*w:(y+1)*w], prev[y*w:(y+1)*w]
		if bytes.Equal(row, prow) {
			continue
		}
		y0, y1 = min(y0, y), y
		for x := 0; x < w; x++ {
			if row[x] != prow[x] {
				x0 = min(x0, x)
				break
			}
		}
		for x := w - 1; x >= 0; x-- {
			if row[x] != prow[x] {
				x1 = max(x1, x)
				break
			}
		}
	}
	if x1 < 0 {
		return image.Rectangle{}
	}
	return image.Rect(x0, y0, x1+1, y1+1)
}

// writeAnim gives the page's frames as an animated GIF or APNG.
func (a *App) writeAnim(fmtID string) ([]byte, error) {
	frames, w, h, pal, err := a.animFrames(1)
	if err != nil {
		return nil, err
	}
	if fmtID == "apng" {
		return writeAPNG(frames, w, h, !a.noLoop)
	}
	return writeGIF(frames, w, h, pal, !a.noLoop)
}

// writeGIF writes the frames as a GIF: delays in hundredths (at least 2: most
// viewers show less as 10), each frame drawn over the one before.
func writeGIF(frames []animFrame, w, h int, pal color.Palette, loop bool) ([]byte, error) {
	g := &gif.GIF{Config: image.Config{ColorModel: pal, Width: w, Height: h}, LoopCount: 0}
	if !loop {
		g.LoopCount = -1
	}
	for _, f := range frames {
		g.Image = append(g.Image, f.img)
		g.Delay = append(g.Delay, max(2, (f.delay+5)/10))
		g.Disposal = append(g.Disposal, gif.DisposalNone)
	}
	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, g); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
