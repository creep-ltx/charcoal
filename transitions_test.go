package main

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

// Every pattern, toggled any way, draws every used cell exactly once and nothing else.
func TestScanOrderCoversEveryCell(t *testing.T) {
	r := rand.New(rand.NewSource(7))
	for _, sz := range [][2]int{{80, 25}, {40, 25}, {1, 1}, {7, 3}, {3, 7}, {160, 50}, {2, 1}} {
		cols, rows := sz[0], sz[1]
		mask := make([]bool, cols*rows)
		for i := range mask {
			mask[i] = r.Intn(3) > 0
		}
		used := func(x, y int) bool { return mask[y*cols+x] }
		for pat := 0; pat < nPatterns; pat++ {
			for f := 0; f < 4; f++ {
				for _, u := range []func(x, y int) bool{nil, used} {
					o := scanOpts{reverse: f&1 != 0, flip: f&2 != 0}
					got := scanOrder(pat, o, cols, rows, u)
					seen := make([]bool, cols*rows)
					n := 0
					for _, c := range got {
						if c < 0 || c >= cols*rows || seen[c] || (u != nil && !mask[c]) {
							t.Fatalf("%s %dx%d %+v: cell %d drawn twice, unused or outside", patterns[pat].id, cols, rows, o, c)
						}
						seen[c] = true
						n++
					}
					want := cols * rows
					if u != nil {
						want = 0
						for _, m := range mask {
							if m {
								want++
							}
						}
					}
					if n != want {
						t.Fatalf("%s %dx%d %+v: %d cells, want %d", patterns[pat].id, cols, rows, o, n, want)
					}
				}
			}
		}
	}
}

func at(cols int, cells ...[2]int) []int {
	var out []int
	for _, c := range cells {
		out = append(out, c[1]*cols+c[0])
	}
	return out
}

// The patterns go the way TheDraw's manual describes them.
func TestScanOrderShapes(t *testing.T) {
	order := func(pat int, o scanOpts, cols, rows int) []int { return scanOrder(pat, o, cols, rows, nil) }
	plain := scanOpts{}
	if got := order(patTop, plain, 3, 2); !slices.Equal(got, []int{0, 1, 2, 3, 4, 5}) {
		t.Errorf("top %v", got)
	}
	if got := order(patLeft, plain, 3, 2); !slices.Equal(got, []int{0, 3, 1, 4, 2, 5}) {
		t.Errorf("left %v", got)
	}
	if got := order(patWiggle, plain, 3, 2); !slices.Equal(got, []int{0, 1, 2, 5, 4, 3}) {
		t.Errorf("wiggle %v", got)
	}
	if got := order(patTop, scanOpts{reverse: true}, 3, 2); !slices.Equal(got, []int{5, 4, 3, 2, 1, 0}) {
		t.Errorf("top reversed %v", got)
	}
	if got := order(patTop, scanOpts{flip: true}, 3, 2); !slices.Equal(got, []int{3, 4, 5, 0, 1, 2}) {
		t.Errorf("top flipped %v", got)
	}
	if got := order(patTop, scanOpts{flip: true, reverse: true}, 3, 2); !slices.Equal(got, []int{2, 1, 0, 5, 4, 3}) {
		t.Errorf("top flipped and reversed %v", got)
	}
	// From the centre: circles and the diamond; the clock's arm starts at 12 o'clock.
	for _, pat := range []int{patCircles, patDiamond} {
		if got := order(pat, plain, 81, 25); got[0] != 12*81+40 {
			t.Errorf("%s starts at %d", patterns[pat].id, got[0])
		}
	}
	if got := order(patClock, plain, 81, 25); got[0]%81 != 40 || got[0]/81 > 12 {
		t.Errorf("clock starts at %d,%d", got[0]%81, got[0]/81)
	}
	if got := order(patClock, plain, 81, 25); got[len(got)-1]%81 >= 40 {
		t.Errorf("clock ends on the right: %d,%d", got[len(got)-1]%81, got[len(got)-1]/81)
	}
	// The pyramid grows from the bottom centre; with Flip from the top centre.
	if got := order(patPyramid, plain, 81, 25); got[0] != 24*81+40 {
		t.Errorf("pyramid starts at %d,%d", got[0]%81, got[0]/81)
	}
	if got := order(patPyramid, scanOpts{flip: true}, 81, 25); got[0] != 40 {
		t.Errorf("flipped pyramid starts at %d,%d", got[0]%81, got[0]/81)
	}
	// Gate: both sides at once; door: top and bottom, both sides.
	if got := order(patGate, plain, 10, 4); !slices.Equal(got[:4], at(10, [2]int{0, 0}, [2]int{9, 0}, [2]int{0, 1}, [2]int{9, 1})) {
		t.Errorf("gate %v", got[:4])
	}
	if got := order(patDoor, plain, 10, 4); !slices.Equal(got[:4], at(10, [2]int{0, 0}, [2]int{9, 0}, [2]int{0, 3}, [2]int{9, 3})) {
		t.Errorf("door %v", got[:4])
	}
	// Squares: the border first, from the upper-left corner clockwise.
	sq := order(patSquares, plain, 20, 10)
	if !slices.Equal(sq[:3], []int{0, 1, 2}) {
		t.Errorf("squares start %v", sq[:3])
	}
	for _, c := range sq[:2*20+2*8] {
		if x, y := c%20, c/20; x > 1 && x < 18 && y > 0 && y < 9 {
			t.Fatalf("squares: %d,%d inside before the border is done", x, y)
		}
	}
	// Checkerboard: every even square before any odd one, the first from the top-left.
	ch := order(patChecker, plain, 32, 8)
	if !slices.Equal(ch[:8], []int{0, 1, 2, 3, 4, 5, 6, 7}) {
		t.Errorf("checkerboard starts %v", ch[:8])
	}
	for i, c := range ch {
		odd := ((c%32)/8+(c/32)/4)&1 == 1
		if odd != (i >= len(ch)/2) {
			t.Fatalf("checkerboard: cell %d (%d,%d) at %d", c, c%32, c/32, i)
		}
	}
	// Angle: diagonals from the upper-left corner.
	if got := order(patAngle, plain, 10, 5); got[0] != 0 || got[1] != 1 || got[len(got)-1] != 49 {
		t.Errorf("angle %v", got)
	}
	// Wiggle out: the centre row first, then the rows around it, alternating direction.
	wo := order(patWiggleOut, plain, 5, 5)
	if !slices.Equal(wo[:5], []int{10, 11, 12, 13, 14}) || wo[5] != 9 || wo[6] != 19 {
		t.Errorf("wiggle out %v", wo)
	}
	// Scramble is the same every time (a file saved twice is the same file).
	if a, b := order(patScramble, plain, 80, 25), order(patScramble, plain, 80, 25); !slices.Equal(a, b) || slices.IsSorted(a) {
		t.Error("scramble is not a fixed shuffle")
	}
}

// randomPage fills a page of the tab's type with random art; empty cells and quotes too.
func randomPage(a *App, r *rand.Rand, cols, rows int) {
	tpl := newDoc(cols, rows)
	tpl.bg, tpl.border, tpl.ink, tpl.glyph0 = a.doc.bg, a.doc.border, a.doc.ink, a.doc.glyph0
	d := a.paperOf(tpl)
	used := rows - r.Intn(3) // sometimes empty rows at the end
	for y := 0; y < used; y++ {
		for x := 0; x < cols; x++ {
			if r.Intn(10) < 4 {
				continue
			}
			var v uint32
			switch a.mode {
			case "c64":
				sc := r.Intn(256)
				if r.Intn(8) == 0 {
					sc = 0x22 // a quote: quote mode
				}
				v = makeCell(sc, r.Intn(16), 0, 0)
			case "atari":
				v = makeCell(r.Intn(256), 0, 0, 0)
			case "st":
				n := a.res().colours
				v = makeCell(32+r.Intn(224), r.Intn(n), r.Intn(n), 0)
				if r.Intn(10) == 0 {
					v = makeCell(32, r.Intn(n), r.Intn(n), 0) // a space with a background
				}
			default:
				v = makeCell(32+r.Intn(224), r.Intn(16), r.Intn(8), 0)
				if r.Intn(10) == 0 {
					v = makeCell(32, 7, r.Intn(16), 0)
				}
				if a.amiga() && r.Intn(10) == 0 {
					v |= italic
				}
				if r.Intn(20) == 0 {
					v = makeCell(r.Intn(32), r.Intn(16), 0, 0) // control bytes: written as spaces
				}
			}
			d.cells[y*cols+x] = v
		}
	}
	a.doc = d
}

// readBack reads a stream of the page's type, as opening the file does.
func readBack(a *App, fmtID string, b []byte, cols int) *Doc {
	switch fmtID {
	case "seq":
		d, _ := parseSeq(b, cols)
		return d
	case "ata":
		return parseAta(b, cols)
	case "vt52":
		return parseVT52(b, a.res(), cols)
	}
	d, _, _ := parseArt(b, ParseOpts{amiga: a.amiga(), forceCols: cols})
	return d
}

// plainWrite is the page written row by row: what the draw order must come back as.
func plainWrite(a *App, fmtID string, d *Doc) []byte {
	switch fmtID {
	case "seq":
		return writeSeq(d, a.lower(), true)
	case "ata":
		return writeAta(d, true)
	case "vt52":
		return writeVT52(d, a.res())
	}
	return writeAnsi(d, a.amiga(), true, true, a.info.writeSauce)
}

var streamModes = []struct {
	mode, fmtID string
	cols, rows  int
}{
	{"pc", "ansi", 80, 25}, {"pc", "ansi", 80, 60}, {"amiga", "ansi", 80, 25}, {"pc", "ansi", 132, 7},
	{"c64", "seq", 40, 25}, {"c64", "seq", 40, 12}, {"atari", "ata", 40, 24}, {"atari", "ata", 38, 10},
	{"st", "vt52", 40, 25}, {"st", "vt52", 80, 30},
}

// roundTrip saves the page in its draw order and reads it back: the picture
// must be exactly what the plain (row by row) file gives, and as tall.
func roundTrip(t *testing.T, a *App, fmtID string, o *drawOrder) []byte {
	t.Helper()
	a.order = nil
	plain, err := a.encode(fmtID)
	if err != nil {
		t.Fatal(err)
	}
	a.order = o
	b, err := a.encode(fmtID)
	if err != nil {
		t.Fatal(err)
	}
	if p := o.pat(); p != patTop && p != patType && a.doc.rows >= 5 && usedOf(a, a.doc) >= 20 && string(b) == string(plain) {
		t.Fatalf("%s %s: the draw order wrote the plain file", a.mode, o)
	}
	want := readBack(a, fmtID, plain, a.doc.cols)
	got := readBack(a, fmtID, b, a.doc.cols)
	if ok, why := cellsMatch(fmtID, want, got); !ok {
		t.Fatalf("%s %s %dx%d %s: %s", a.mode, fmtID, a.doc.cols, a.doc.rows, o, why)
	}
	if got.rows != a.doc.rows {
		t.Fatalf("%s %s %s: %d rows, want %d", a.mode, fmtID, o, got.rows, a.doc.rows)
	}
	return b
}

// Every pattern that can be a draw order, in every stream format, gives exactly the picture back.
func TestDrawOrderRoundTrip(t *testing.T) {
	r := rand.New(rand.NewSource(11))
	for _, m := range streamModes {
		a := testApp(t, m.mode)
		if m.mode == "st" && m.cols == 80 {
			a.stres = "medium"
		}
		randomPage(a, r, m.cols, m.rows)
		for pat := 0; pat < nPatterns; pat++ {
			if framesOnly(pat, false) {
				continue
			}
			for f := 0; f < 4; f++ {
				o := &drawOrder{Pattern: patterns[pat].id, Reverse: f&1 != 0, Flip: f&2 != 0}
				if pat == patFadeIn && (a.fadeRefused() != "" || f > 0) {
					continue
				}
				b := roundTrip(t, a, m.fmtID, o)
				if pat == patTop && f == 0 && m.fmtID == "ansi" && len(b) > 3*len(plainWrite(a, m.fmtID, a.doc)) {
					t.Errorf("%s: the Top order takes %d bytes, the plain file %d", m.mode, len(b), len(plainWrite(a, m.fmtID, a.doc)))
				}
			}
		}
	}
}

// Fuzz-ish: random pages, sizes and patterns all come back exactly.
func TestDrawOrderRandomPages(t *testing.T) {
	r := rand.New(rand.NewSource(99))
	for i := 0; i < 150; i++ {
		m := streamModes[r.Intn(len(streamModes))]
		a := testApp(t, m.mode)
		cols, rows := m.cols, 1+r.Intn(m.rows)
		if m.mode == "pc" || m.mode == "amiga" {
			cols = 1 + r.Intn(120)
		}
		if m.mode == "st" && m.cols == 80 {
			a.stres = "medium"
		}
		randomPage(a, r, cols, rows)
		pat := r.Intn(nTheDraw + 1)
		if pat == patFadeIn && a.fadeRefused() != "" {
			pat = patScramble
		}
		roundTrip(t, a, m.fmtID, &drawOrder{Pattern: patterns[pat].id, Reverse: r.Intn(2) == 0, Flip: r.Intn(2) == 0})
	}
}

// Only cells that show something are drawn: an almost empty page makes a small file.
func TestDrawOrderSkipsBlankCells(t *testing.T) {
	a := testApp(t, "pc")
	a.doc = newDoc(80, 25)
	a.doc.set(40, 12, makeCell('X', 14, 1, 0))
	a.doc.set(0, 0, makeCell(' ', 7, 4, 0)) // a coloured space counts
	a.order = &drawOrder{Pattern: "clock"}
	steps := a.drawSteps(a.doc, a.order)
	if len(steps) != 2 {
		t.Fatalf("%d steps, want 2", len(steps))
	}
	b := a.writeOrdered("ansi", a.doc, false)
	if len(b) > 60 {
		t.Errorf("%d bytes for two cells: %q", len(b), b)
	}
}

// PETSCII quotes: quote mode never swallows a cursor or colour code.
func TestDrawOrderPetsciiQuotes(t *testing.T) {
	a := testApp(t, "c64")
	a.doc = newDoc(40, 25)
	a.doc.glyph0 = true
	for x := 0; x < 40; x += 3 {
		a.doc.set(x, 24, makeCell(0x22, x%16, 0, 0)) // quotes on the bottom row, last ones too
		a.doc.set(x, 3, makeCell(0xA2, 5, 0, 0))     // reversed quotes
		a.doc.set(39-x, 10, makeCell('A'&63, 2, 0, 0))
	}
	for _, pat := range []string{"top", "left", "scramble", "wiggle", "circles"} {
		for f := 0; f < 4; f++ {
			roundTrip(t, a, "seq", &drawOrder{Pattern: pat, Reverse: f&1 != 0, Flip: f&2 != 0})
		}
	}
}

// The order is kept with the page: Save writes it, undo takes it back, Play sends it, autosave keeps it.
func TestDrawOrderKeptWithPage(t *testing.T) {
	a := testApp(t, "pc")
	randomPage(a, rand.New(rand.NewSource(3)), 80, 25)
	a.markSaved()
	a.applyOrder(&drawOrder{Pattern: "diamond", Flip: true})
	if a.order == nil || !a.dirty {
		t.Fatal("order not set, or the page not changed")
	}
	if got := string(a.playBytes()); got != string(a.writeOrdered("ansi", a.doc, false)) {
		t.Error("Play does not send the draw order")
	}
	p := filepath.Join(t.TempDir(), "x.ans")
	if !a.writeOut(p, "ansi") {
		t.Fatal(a.message)
	}
	b, _ := os.ReadFile(p)
	if want, _ := a.encode("ansi"); string(b) != string(want) || a.order == nil {
		t.Error("the saved file is not the draw order")
	}
	a.cx, a.cy = 1, 1
	a.typeByte('Q')
	a.doUndo()
	if a.order == nil || a.order.Pattern != "diamond" {
		t.Error("undoing an edit lost the order")
	}
	a.doUndo() // the order itself
	if a.order != nil {
		t.Error("undo did not take the order back")
	}
	a.doRedo()
	if a.order == nil || !a.order.Flip {
		t.Error("redo did not give the order back")
	}
	// Autosave keeps it.
	a.typeByte('Z')
	if err := writeAutosave(a.Tab); err != nil {
		t.Fatal(err)
	}
	b2 := restart(t, "pc")
	files := leftAutosaves()
	if len(files) != 1 {
		t.Fatalf("%d autosaves", len(files))
	}
	if err := b2.restoreAutosave(files[0]); err != nil {
		t.Fatal(err)
	}
	if b2.order == nil || b2.order.Pattern != "diamond" || !b2.order.Flip {
		t.Fatalf("autosave lost the order: %+v", b2.order)
	}
	// Clearing it saves row by row again.
	ordered := b2.writeOrdered("ansi", b2.doc, b2.info.writeSauce)
	b2.applyOrder(nil)
	if got, _ := b2.encode("ansi"); b2.order != nil || string(got[:min(len(got), len(ordered))]) == string(ordered) {
		t.Error("cleared order still written")
	}
}

// Teletext and Spectrum pages have no stream: no draw order, frames only.
func TestDrawOrderRefusedOnScreens(t *testing.T) {
	for _, mode := range []string{"tt", "zx"} {
		a := testApp(t, mode)
		for x := 3; x < 9; x++ {
			a.doc.set(x, 5, a.doc.get(x, 5)&^255|'A')
		}
		a.applyOrder(&drawOrder{Pattern: "top"})
		if a.order != nil {
			t.Errorf("%s: a draw order was set", mode)
		}
		a.applyFrames(animSpec{pat: patCircles, n: 1, hold: 50})
		if a.nFrames() < 2 {
			t.Errorf("%s: no frames: %s", mode, a.message)
		}
	}
}

// usedOf counts the cells of d that show something.
func usedOf(a *App, d *Doc) int {
	e := a.paperOf(d)
	n := 0
	for i := range d.cells[:d.cols*d.rows] {
		if usedCell(a.mode, d.cells[i], e.cells[i]) {
			n++
		}
	}
	return n
}

// Frames: one every n cells, each with more of the picture, the last the picture itself.
func TestTransitionFrames(t *testing.T) {
	a := testApp(t, "pc")
	randomPage(a, rand.New(rand.NewSource(5)), 80, 25)
	d := a.doc.clone()
	used := usedOf(a, d)
	for pat := 0; pat < nTheDraw; pat++ {
		docs, holds, note := a.transitionFrames(d, animSpec{pat: pat, n: 100, hold: 50}, maxPageCells)
		if want := (used + 99) / 100; len(docs) != want || note != "" {
			t.Fatalf("%s: %d frames, want %d (%s)", patterns[pat].id, len(docs), want, note)
		}
		last := 0
		for i, f := range docs {
			n := usedOf(a, f)
			if n != min(used, (i+1)*100) || n <= last && i > 0 {
				t.Fatalf("%s frame %d: %d cells shown", patterns[pat].id, i, n)
			}
			last = n
		}
		if ok, why := cellsMatch("ansi", d, docs[len(docs)-1]); !ok || holds[len(holds)-1] != animEndHold || holds[0] != 50 {
			t.Fatalf("%s: last frame %s, holds %v", patterns[pat].id, why, holds)
		}
		// Erasing: the picture first, then less and less, at last nothing.
		docs, _, _ = a.transitionFrames(d, animSpec{pat: pat, erase: true, n: 100, hold: 50}, maxPageCells)
		if len(docs) != (used+99)/100+1 || usedOf(a, docs[0]) != used || usedOf(a, docs[len(docs)-1]) != 0 {
			t.Fatalf("%s erasing: %d frames", patterns[pat].id, len(docs))
		}
	}
	// Fades: n steps; scroll: a frame a row; typewriter: a cursor ahead of the text.
	docs, _, _ := a.transitionFrames(d, animSpec{pat: patFadeIn, n: 8, hold: 50}, maxPageCells)
	if len(docs) != 8 || !slices.Equal(docs[7].cells, d.cells[:80*25]) {
		t.Fatalf("fade in: %d frames", len(docs))
	}
	docs, _, _ = a.transitionFrames(d, animSpec{pat: patFadeOut, n: 8, hold: 50}, maxPageCells)
	if len(docs) != 9 || !slices.Equal(docs[0].cells, d.cells[:80*25]) || usedOf(a, docs[8]) != 0 {
		t.Fatalf("fade out: %d frames", len(docs))
	}
	docs, _, _ = a.transitionFrames(d, animSpec{pat: patScroll, n: 1, hold: 50}, maxPageCells)
	if len(docs) != 25 || !slices.Equal(docs[24].cells, d.cells[:80*25]) || docs[0].get(5, 24) != d.get(5, 0) {
		t.Fatalf("scroll: %d frames", len(docs))
	}
	docs, _, _ = a.transitionFrames(d, animSpec{pat: patType, n: 10, hold: 50}, maxPageCells)
	if cellCh(docs[0].cells[10]) != 219 {
		t.Errorf("typewriter: no cursor after the first 10 cells: %d", cellCh(docs[0].cells[10]))
	}
	// Too many frames for the cells left: fewer, larger steps.
	docs, _, note := a.transitionFrames(d, animSpec{pat: patTop, n: 1, hold: 50}, 2000*20)
	if len(docs) > 20 || note == "" {
		t.Errorf("limit: %d frames (%s)", len(docs), note)
	}
}

// A frame every so many ms at the modem's speed: the holds add up to the file's time.
func TestTransitionFramesByTime(t *testing.T) {
	a := testApp(t, "pc")
	randomPage(a, rand.New(rand.NewSource(8)), 80, 25)
	d := a.doc
	docs, holds, _ := a.transitionFrames(d, animSpec{pat: patCircles, ms: 200, baud: 9600}, maxPageCells)
	e := a.paperOf(d)
	order := a.cellsInOrder(d, e, patCircles, false, false)
	steps := make([]drawStep, len(order))
	for i, c := range order {
		steps[i] = drawStep{c % 80, c / 80, d.cells[c]}
	}
	_, offs := writeDrawSteps("ansi", d, steps, a.streamOpts(false))
	total := offs[len(offs)-1] * 10000 / 9600
	sum := 0
	for _, h := range holds[:len(holds)-1] {
		sum += h
		if h < 190 {
			t.Fatalf("a frame of %d ms", h)
		}
	}
	if len(docs) < 5 || sum > total || total-sum > 600 {
		t.Errorf("%d frames, holds %d ms of %d", len(docs), sum, total)
	}
}

// Frames take the place of the current frame, as one undo step; the strip shows them.
func TestApplyFrames(t *testing.T) {
	a := testApp(t, "c64")
	randomPage(a, rand.New(rand.NewSource(9)), 40, 25)
	want := a.doc.clone()
	a.newFrame() // frame 2, empty
	a.goFrame(0)
	a.applyFrames(animSpec{pat: patSquares, n: 50, hold: 40})
	n := a.nFrames()
	if n < 4 || a.cur != n-2 || !slices.Equal(a.doc.cells, want.cells[:40*25]) {
		t.Fatalf("%d frames, at %d", n, a.cur)
	}
	if !a.stripShown() {
		t.Error("the strip is not shown")
	}
	a.doUndo()
	if a.nFrames() != 2 || a.cur != 0 || !slices.Equal(a.doc.cells, want.cells[:40*25]) {
		t.Fatalf("undo: %d frames", a.nFrames())
	}
	// The frames save as a GIF.
	a.doRedo()
	b, err := a.encode("gif")
	if err != nil || len(b) < 100 {
		t.Fatalf("gif: %v", err)
	}
}

// The requester: keys pick the pattern and toggles; Apply sets the order or makes frames.
func TestAnimateRequester(t *testing.T) {
	a := testApp(t, "pc")
	randomPage(a, rand.New(rand.NewSource(4)), 80, 25)
	openAnimate()
	d, ok := a.dialog.(*AnimReq)
	if !ok || !dialogAnimates() {
		t.Fatal("no requester")
	}
	d.key("Down", false, false) // Clock -> Circles
	d.key("R", false, false)
	d.key("F", false, false)
	if d.pat != patCircles || !d.reverse || !d.flip {
		t.Fatalf("pattern %d %v %v", d.pat, d.reverse, d.flip)
	}
	if docs := d.preview(); len(docs) < 10 {
		t.Errorf("preview of %d frames", len(docs))
	}
	d.key("Return", false, false)
	if a.dialog != nil || a.order == nil || a.order.Pattern != "circles" || !a.order.Reverse || !a.order.Flip {
		t.Fatalf("order %+v", a.order)
	}
	openAnimate()
	d = a.dialog.(*AnimReq)
	if d.pat != patCircles || !d.reverse {
		t.Error("the requester does not start from the page's order")
	}
	d.frames = true
	d.pat = patWiggle
	d.apply()
	if a.nFrames() < 2 || a.order == nil {
		t.Fatalf("frames %d", a.nFrames())
	}
}

// The draw order follows a pattern on a real piece of art (ansilove's test file) too.
func TestDrawOrderRealArt(t *testing.T) {
	b, err := os.ReadFile("testdata/alove/bs-alove.bin")
	if err != nil {
		t.Skip(err)
	}
	a := testApp(t, "pc")
	d, _, _ := parseArt(b, ParseOpts{binary: true})
	a.doc = d
	start := time.Now()
	for pat := 0; pat <= nTheDraw; pat++ {
		roundTrip(t, a, "ansi", &drawOrder{Pattern: patterns[pat].id})
	}
	if el := time.Since(start); el > 20*time.Second {
		t.Errorf("slow: %v", el)
	}
	t.Log(fmt.Sprintf("%dx%d, %d used cells", d.cols, d.rows, usedOf(a, d)))
}
