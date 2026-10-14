package main

// The templates' preview video: with CHARCOAL_PREVIEW=<folder> this test draws
// every new animation template on real art through the program's own renderer
// and writes 1280x720 PNG pictures plus an ffmpeg concat list (list.txt) to the
// folder; tools/preview.sh turns them into preview/templates.mp4. Without the
// variable it is skipped. CHARCOAL_PREVIEW_ART is the folder of the art files
// (default /tmp/Art); the C64 ROMs are copied from ~/.local/share/charcoal.

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const previewW, previewH = 1280, 720

type previewClip struct {
	pat        int
	art, label string
	x, y, w, h int // the part of the art shown (w, h 0: all)
}

func TestTemplatePreview(t *testing.T) {
	out := os.Getenv("CHARCOAL_PREVIEW")
	if out == "" {
		t.Skip("CHARCOAL_PREVIEW not set")
	}
	art := os.Getenv("CHARCOAL_PREVIEW_ART")
	if art == "" {
		art = "/tmp/Art"
	}
	a := testApp(t, "amiga")
	data := appDataDir()
	os.MkdirAll(data, 0o755)
	home, _ := os.UserHomeDir()
	if h := os.Getenv("CHARCOAL_REAL_HOME"); h != "" {
		home = h
	}
	for _, f := range []string{"chargen.bin", "c128-chargen.bin", "vic20-chargen.bin", "pet-characters.bin"} {
		if b, err := os.ReadFile(filepath.Join(home, ".local/share/charcoal", f)); err == nil {
			os.WriteFile(filepath.Join(data, f), b, 0o644)
		}
	}
	a.hasC64 = loadC64Fonts()
	titles := a.Tab
	alove := "testdata/alove/bs-alove.bin"
	mist := filepath.Join(art, "pt-mist3.ans")
	absinthe := filepath.Join(art, "anst-absinthe40k.ans")
	nash := filepath.Join(art, "c64-uncle_nash.c")
	rio := filepath.Join(art, "Rio.tti")
	clips := []previewClip{
		{patMatrix, mist, "PC ANSI: Platinum / MiSTiGRiS", 0, 0, 80, 23},
		{patDecrypt, absinthe, "Amiga ANSI: Absinthe", 0, 0, 80, 22},
		{patSpiral, nash, "C64 PETSCII: Uncle Nash", 0, 0, 0, 0},
		{patDrop, mist, "PC ANSI: Platinum / MiSTiGRiS", 0, 0, 80, 23},
		{patAssemble, rio, "Teletext: Rio", 0, 0, 0, 0},
		{patBlinds, rio, "Teletext: Rio", 0, 0, 0, 0},
		{patShine, absinthe, "Amiga ANSI: Absinthe", 0, 0, 80, 22},
		{patShadeIn, alove, "PC ANSI: ansilove's test art", 0, 0, 80, 22},
		{patMelt, nash, "C64 PETSCII: Uncle Nash", 0, 0, 0, 0},
		{patPowerOn, alove, "PC ANSI: ansilove's test art", 0, 0, 80, 22},
	}
	if only := os.Getenv("CHARCOAL_PREVIEW_ONLY"); only != "" {
		var keep []previewClip
		for _, c := range clips {
			if strings.Contains(only, patterns[c.pat].id) {
				keep = append(keep, c)
			}
		}
		clips = keep
	}
	var list strings.Builder
	n := 0
	emit := func(fb *FB, ms int) {
		p := filepath.Join(out, fmt.Sprintf("f%05d.png", n))
		n++
		if err := writeCanvas(p, fb); err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&list, "file '%s'\nduration %.3f\n", p, float64(ms)/1000)
	}
	sheet := []*FB{}
	a.focusTab(titles)
	emit(a.renderDoc(titleCard("CharEdit: animation templates", "ten new ones, drawn by CharEdit itself"), 1), 2500)
	for i, c := range clips {
		// The title card, drawn by CharEdit on an Amiga page in Topaz.
		a.focusTab(titles)
		emit(a.renderDoc(titleCard(fmt.Sprintf("%d. %s", i+1, patterns[c.pat].name), c.label), 1), 1600)
		// The art.
		a.openPath(c.art, "auto")
		if a.doc == nil || a.doc.cols == 0 {
			t.Fatalf("%s: not opened: %s", c.art, a.message)
		}
		d := cropDoc(a.doc, c.x, c.y, c.w, c.h)
		def := effectDefaults[c.pat]
		s := animSpec{pat: c.pat, n: def[0], hold: def[1], seed: 1989 + int64(i)}
		if isScan(c.pat) {
			e := a.paperOf(d)
			s.n, s.hold = max(1, (len(a.cellsInOrder(d, e, c.pat, false, false))+59)/60), 40
		}
		docs, holds, note := a.transitionFrames(d, s, maxPageCells)
		t.Logf("%s on %s: %dx%d, %d frames %s", patterns[c.pat].name, filepath.Base(c.art), d.cols, d.rows, len(docs), note)
		emit(a.renderDoc(a.paperOf(d), 1), 300) // the clear screen first
		for j, f := range docs {
			ms := holds[j]
			if j == len(docs)-1 {
				ms = 1500
			}
			fb := a.renderDoc(f, 1)
			emit(fb, ms)
			if j == len(docs)*45/100 {
				sheet = append(sheet, fb)
			}
		}
		a.closeTabForPreview()
	}
	// The bonus reel: the earlier eighteen, a few seconds each.
	if os.Getenv("CHARCOAL_PREVIEW_BONUS") != "" {
		a.focusTab(titles)
		emit(a.renderDoc(titleCard("And the first eighteen", "TheDraw's fourteen, Charcoal's first four"), 1), 2000)
		arts := []struct{ path, label string }{{alove, "PC"}, {absinthe, "Amiga"}, {nash, "C64"}, {mist, "PC"}}
		for pat := 0; pat < patMatrix; pat++ {
			art := arts[pat%len(arts)]
			a.focusTab(titles)
			emit(a.renderDoc(titleCard(patterns[pat].name, art.label), 1), 700)
			a.openPath(art.path, "auto")
			d := cropDoc(a.doc, 0, 0, 80, 22)
			s := animSpec{pat: pat, hold: 40, n: 8}
			switch pat {
			case patScroll:
				s.n = 1
			case patFadeIn, patFadeOut:
				s.n, s.hold = 16, 60
			default:
				s.n = max(1, (len(a.cellsInOrder(d, a.paperOf(d), pat, false, false))+49)/50)
			}
			docs, holds, _ := a.transitionFrames(d, s, maxPageCells)
			if pat != patFadeOut {
				emit(a.renderDoc(a.paperOf(d), 1), 200)
			}
			for j, f := range docs {
				ms := holds[j]
				if j == len(docs)-1 {
					ms = 800
				}
				emit(a.renderDoc(f, 1), ms)
			}
			a.closeTabForPreview()
		}
	}
	// ffmpeg's concat list repeats the last picture so its duration counts.
	fmt.Fprintf(&list, "file '%s'\n", filepath.Join(out, fmt.Sprintf("f%05d.png", n-1)))
	if err := os.WriteFile(filepath.Join(out, "list.txt"), []byte(list.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeSheet(filepath.Join(out, "sheet.png"), sheet); err != nil {
		t.Fatal(err)
	}
}

// closeTabForPreview drops the art's tab so the next file opens a fresh one.
func (a *App) closeTabForPreview() {
	for i, t := range a.tabs {
		if t == a.Tab && len(a.tabs) > 1 {
			a.tabs = append(a.tabs[:i], a.tabs[i+1:]...)
			a.focusTab(a.tabs[0])
			return
		}
	}
}

// titleCard is an Amiga page with a title and a line under it, white on dark.
func titleCard(title, sub string) *Doc {
	d := newDoc(48, 9)
	put := func(y int, s string, fg int) {
		x := (d.cols - len(s)) / 2
		for i := 0; i < len(s); i++ {
			d.set(x+i, y, makeCell(int(s[i]), fg, 0, 0))
		}
	}
	put(3, title, 15)
	put(5, sub, 7)
	return d
}

func cropDoc(d *Doc, x, y, w, h int) *Doc {
	if w == 0 || h == 0 {
		return d.clone()
	}
	w, h = min(w, d.cols-x), min(h, d.rows-y)
	n := newDoc(w, h)
	n.bg, n.border, n.ink, n.glyph0 = d.bg, d.border, d.ink, d.glyph0
	for r := 0; r < h; r++ {
		for c := 0; c < w; c++ {
			n.set(c, r, d.get(x+c, y+r))
		}
	}
	return n
}

// canvas puts fb on a dark 1280x720 picture as large as fits: whole times
// larger where that fills most of it, else smoothly scaled.
func canvas(fb *FB) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, previewW, previewH))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = 12, 12, 16, 255
	}
	f := min(float64(previewW)/float64(fb.w), float64(previewH)/float64(fb.h))
	if whole := float64(int(f)); whole >= 1 && whole >= 0.8*f {
		f = whole
	}
	w, h := int(float64(fb.w)*f), int(float64(fb.h)*f)
	x0, y0 := (previewW-w)/2, (previewH-h)/2
	px := func(x, y int) uint32 { return fb.px[min(fb.h-1, y)*fb.w+min(fb.w-1, x)] }
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var r, g, b float64
			if f == float64(int(f)) {
				p := px(x/int(f), y/int(f))
				r, g, b = float64(p>>16&255), float64(p>>8&255), float64(p&255)
			} else { // bilinear
				sx, sy := (float64(x)+0.5)/f-0.5, (float64(y)+0.5)/f-0.5
				ix, iy := int(max(0, sx)), int(max(0, sy))
				fx, fy := max(0, sx-float64(ix)), max(0, sy-float64(iy))
				for _, s := range [4]struct {
					dx, dy int
					w      float64
				}{{0, 0, (1 - fx) * (1 - fy)}, {1, 0, fx * (1 - fy)}, {0, 1, (1 - fx) * fy}, {1, 1, fx * fy}} {
					p := px(ix+s.dx, iy+s.dy)
					r += s.w * float64(p>>16&255)
					g += s.w * float64(p>>8&255)
					b += s.w * float64(p&255)
				}
			}
			o := ((y0+y)*previewW + x0 + x) * 4
			img.Pix[o], img.Pix[o+1], img.Pix[o+2] = byte(r+0.5), byte(g+0.5), byte(b+0.5)
		}
	}
	return img
}

func writeCanvas(path string, fb *FB) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, canvas(fb))
}

// writeSheet puts one late frame of every effect on a sheet, five a row.
func writeSheet(path string, fbs []*FB) error {
	const cw, ch, per = 512, 288, 5
	rows := (len(fbs) + per - 1) / per
	img := image.NewRGBA(image.Rect(0, 0, cw*per, ch*max(1, rows)))
	for i, fb := range fbs {
		c := canvas(fb)
		ox, oy := i%per*cw, i/per*ch
		for y := 0; y < ch; y++ {
			for x := 0; x < cw; x++ {
				s := (y*previewH/ch*previewW + x*previewW/cw) * 4
				o := ((oy+y)*img.Rect.Dx() + ox + x) * 4
				copy(img.Pix[o:o+4], c.Pix[s:s+4])
			}
		}
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
