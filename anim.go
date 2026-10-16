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
	keep := a.doc
	a.doc = d // drawCell takes a screen page's colours from the page
	defer func() { a.doc = keep }()
	H := a.cellH()
	rows := max(1, min(d.rows, 32000/(H*z)))
	if a.screen() {
		rows = max(rows, map[bool]int{true: 25, false: 24}[a.c64()])
	}
	border := 0
	fb := &FB{}
	if a.screen() || a.zx() || a.cpc() { // with the machine's border around it
		border = 32 * z
		fb.resize(d.cols*a.cellW()*z+2*border, rows*H*z+2*border)
		fb.fill(Rect{0, 0, fb.w, fb.h}, a.borderPx(d))
	} else {
		fb.resize(d.cols*a.cellW()*z, rows*H*z)
	}
	a.paintCells(fb, border, border, z, func(x, y int) uint32 { return d.get(x, y) }, d.cols, rows)
	return fb
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
	H := a.cellH()
	rows := max(1, min(d.rows, 32000/(H*z)))
	if a.screen() {
		rows = max(rows, map[bool]int{true: 25, false: 24}[a.c64()])
	}
	w, h := d.cols*a.cellW()*z, rows*H*z
	if a.screen() || a.zx() || a.cpc() {
		w, h = w+64*z, h+64*z
	}
	return w, h
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
