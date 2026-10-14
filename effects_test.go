package main

import (
	"encoding/json"
	"math/rand"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// effectPages are the page types the effects are tried on.
var effectPages = []struct{ mode, machine, stres string }{
	{"pc", "", ""}, {"amiga", "", ""}, {"c64", "", ""}, {"c64", "pet", ""}, {"atari", "", ""},
	{"st", "", "low"}, {"st", "", "medium"}, {"st", "", "high"}, {"zx", "", ""}, {"tt", "", ""},
}

func effectApp(t *testing.T, mode, machine, stres string) *App {
	a := testApp(t, mode)
	if machine != "" {
		a.machine = machine
	}
	if stres != "" {
		a.stres = stres
	}
	return a
}

// effectPage fills the page with random art of its type (teletext: codes too).
func effectPage(a *App, r *rand.Rand) {
	switch {
	case a.tt():
		d := newTTDoc()
		for y := 0; y < d.rows; y++ {
			for x := 0; x < d.cols; x++ {
				switch n := r.Intn(10); {
				case n == 0:
					d.set(x, y, makeCell(r.Intn(0x20), 0, 0, 0)) // a code
				case n < 6:
					d.set(x, y, makeCell(0x21+r.Intn(0x5E), 0, 0, 0))
				}
			}
		}
		a.doc = d
	case a.zx():
		d := newZXDoc()
		for i := range d.cells {
			if r.Intn(3) > 0 {
				b := r.Intn(2) == 0
				d.cells[i] = zxCell(32+r.Intn(130), r.Intn(8), r.Intn(8), b, false)
			}
		}
		a.doc = d
	default:
		cols, rows := 40, 25
		if a.mode == "atari" {
			rows = 24
		}
		if a.pc() || a.amiga() {
			cols = 80
		}
		randomPage(a, r, cols, rows)
	}
}

// Every effect on every page type: the frames it should have, all the page's
// size, the last exactly the picture, the same with the same seed, and
// teletext codes never touched.
func TestEffectsEndOnPicture(t *testing.T) {
	r := rand.New(rand.NewSource(21))
	for _, pg := range effectPages {
		a := effectApp(t, pg.mode, pg.machine, pg.stres)
		effectPage(a, r)
		d := a.doc.clone()
		n := d.cols * d.rows
		for pat := 0; pat < nPatterns; pat++ {
			if !isEffect(pat) {
				continue
			}
			s := animSpec{pat: pat, hold: 40, seed: 77}
			docs, holds, note := a.transitionFrames(d, s, maxPageCells)
			name := pg.mode + pg.machine + pg.stres + " " + patterns[pat].id
			if len(docs) != effectDefaults[pat][0] || len(holds) != len(docs) || note != "" {
				t.Fatalf("%s: %d frames, %d holds (%s)", name, len(docs), len(holds), note)
			}
			if !slices.Equal(docs[len(docs)-1].cells[:n], d.cells[:n]) || holds[len(holds)-1] < animEndHold {
				t.Fatalf("%s: does not end on the picture", name)
			}
			changed := false
			for i, f := range docs {
				if f.cols != d.cols || f.rows != d.rows || len(f.cells) < n {
					t.Fatalf("%s frame %d: %dx%d", name, i, f.cols, f.rows)
				}
				if !slices.Equal(f.cells[:n], d.cells[:n]) {
					changed = true
				}
				if a.tt() {
					for c := 0; c < n; c++ {
						if d.cells[c]&0x7F < 0x20 && f.cells[c] != d.cells[c] {
							t.Fatalf("%s frame %d: the code at %d,%d changed", name, i, c%d.cols, c/d.cols)
						}
					}
				}
			}
			if !changed {
				t.Fatalf("%s: every frame is the picture", name)
			}
			again, _, _ := a.transitionFrames(d, s, maxPageCells)
			for i := range docs {
				if !slices.Equal(docs[i].cells[:n], again[i].cells[:n]) {
					t.Fatalf("%s: frame %d differs with the same seed", name, i)
				}
			}
			if pat != patShine && pat != patPowerOn {
				s.seed = 78
				other, _, _ := a.transitionFrames(d, s, maxPageCells)
				same := true
				for i := range docs {
					same = same && slices.Equal(docs[i].cells[:n], other[i].cells[:n])
				}
				if same {
					t.Errorf("%s: another seed gives the same frames", name)
				}
			}
		}
		if !slices.Equal(a.doc.cells, d.cells) {
			t.Fatalf("%s: the page changed", pg.mode)
		}
	}
}

// Effects keep within the frames and cells they may take.
func TestEffectLimits(t *testing.T) {
	a := testApp(t, "pc")
	randomPage(a, rand.New(rand.NewSource(4)), 80, 25)
	docs, _, note := a.transitionFrames(a.doc, animSpec{pat: patMatrix, n: 300, hold: 40, seed: 1}, 80*25*20)
	if len(docs) > 20 || len(docs) < 2 || note == "" {
		t.Errorf("limit: %d frames (%s)", len(docs), note)
	}
	if got := effectLength(patDrop, 100000); got != maxEffectFrames {
		t.Errorf("length %d", got)
	}
	if got := effectLength(patDrop, 0); got != effectDefaults[patDrop][0] {
		t.Errorf("default length %d", got)
	}
	// A one-cell page and an empty page still give frames that end on the picture.
	for _, sz := range [][2]int{{1, 1}, {3, 1}, {1, 4}} {
		d := newDoc(sz[0], sz[1])
		d.set(0, 0, makeCell('X', 14, 1, 0))
		for pat := 0; pat < nPatterns; pat++ {
			if isEffect(pat) {
				docs, _, _ := a.transitionFrames(d, animSpec{pat: pat, hold: 40, seed: 3}, maxPageCells)
				if len(docs) < 2 || !slices.Equal(docs[len(docs)-1].cells, d.cells) {
					t.Fatalf("%dx%d %s: %d frames", sz[0], sz[1], patterns[pat].id, len(docs))
				}
			}
		}
	}
}

// Spiral goes round clockwise from the outer edge in; Blinds opens bands of rows together.
func TestSpiralAndBlinds(t *testing.T) {
	o := scanOpts{aspect: 1}
	if got := scanOrder(patSpiral, o, 4, 3, nil); !slices.Equal(got, at(4,
		[2]int{0, 0}, [2]int{1, 0}, [2]int{2, 0}, [2]int{3, 0}, [2]int{3, 1}, [2]int{3, 2}, [2]int{2, 2}, [2]int{1, 2}, [2]int{0, 2}, [2]int{0, 1},
		[2]int{1, 1}, [2]int{2, 1})) {
		t.Errorf("spiral %v", got)
	}
	got := scanOrder(patBlinds, o, 2, 8, nil)
	if want := at(2, [2]int{0, 0}, [2]int{0, 4}, [2]int{1, 0}, [2]int{1, 4}, [2]int{0, 1}, [2]int{0, 5}); !slices.Equal(got[:6], want) {
		t.Errorf("blinds %v", got)
	}
	if !hasToggles(patSpiral) || !hasToggles(patBlinds) || hasToggles(patMatrix) || !isScan(patBlinds) || isScan(patShine) {
		t.Error("toggles")
	}
}

// noControls takes the control bytes off an ANSI page: the reference (the
// plain file, row by row) reads differently after some of them (0x1A ends a
// DOS file there; 0x9B before digits is a CSI), so a stream that draws the
// cells in another order would not be compared fairly.
func noControls(d *Doc) {
	for i, v := range d.cells {
		if ch := v & 255; ch < 0x20 || ch == 0x7F || ch == 0x9B {
			d.cells[i] = v&^255 | 0x41
		}
	}
}

// The delta stream: played through the page type's reader it shows every
// frame at that frame's end, and ends exactly on the picture.
func TestDeltaStreamRoundTrip(t *testing.T) {
	r := rand.New(rand.NewSource(31))
	for _, m := range streamModes {
		a := testApp(t, m.mode)
		if m.mode == "st" && m.cols == 80 {
			a.stres = "medium"
		}
		randomPage(a, r, m.cols, m.rows)
		if m.fmtID == "ansi" {
			noControls(a.doc)
		}
		d := a.doc.clone()
		e := a.paperOf(d)
		check := func(what string, docs []*Doc) {
			out, ends := writeDeltaFrames(m.fmtID, a.mode, e, docs, a.streamOpts(false))
			if len(ends) != len(docs) || ends[len(ends)-1] != len(out) {
				t.Fatalf("%s %s: %d ends for %d frames", m.mode, what, len(ends), len(docs))
			}
			for i, f := range docs {
				want := readBack(a, m.fmtID, plainWrite(a, m.fmtID, f), d.cols)
				got := readBack(a, m.fmtID, out[:ends[i]], d.cols)
				if ok, why := cellsMatch(m.fmtID, want, got); !ok {
					t.Fatalf("%s %dx%d %s frame %d of %d: %s", m.mode, d.cols, d.rows, what, i, len(docs), why)
				}
			}
			got := readBack(a, m.fmtID, out, d.cols)
			if got.rows != d.rows {
				t.Fatalf("%s %s: %d rows, want %d", m.mode, what, got.rows, d.rows)
			}
		}
		for pat := 0; pat < nPatterns; pat++ {
			if isEffect(pat) {
				docs, _, _ := a.transitionFrames(d, animSpec{pat: pat, n: 24, hold: 40, seed: 5}, maxPageCells)
				check(patterns[pat].id, docs)
			}
		}
		// Frames that change at random, back to the paper too.
		var docs []*Doc
		f := e.clone()
		for i := 0; i < 12; i++ {
			f = f.clone()
			for j := 0; j < 60; j++ {
				c := r.Intn(d.cols * d.rows)
				if r.Intn(3) == 0 {
					f.cells[c] = e.cells[c]
				} else {
					f.cells[c] = d.cells[r.Intn(d.cols*d.rows)]
				}
			}
			docs = append(docs, f)
		}
		check("random", append(docs, d.clone()))
	}
}

// A frame effect as the saved file's order: Save writes the delta stream, the
// same bytes every time (the seed is kept), Play sends it, undo and autosave keep it.
func TestEffectDrawOrderSaved(t *testing.T) {
	a := testApp(t, "pc")
	randomPage(a, rand.New(rand.NewSource(9)), 80, 25)
	noControls(a.doc)
	a.markSaved()
	o := &drawOrder{Pattern: "matrix", Seed: 4242, Frames: 30}
	a.applyOrder(o)
	if a.order == nil || !a.orderedSave("ansi") {
		t.Fatal("no order")
	}
	b1, _ := a.encode("ansi")
	b2, _ := a.encode("ansi")
	if string(b1) != string(b2) {
		t.Fatal("two saves differ")
	}
	if string(a.playBytes()) != string(a.writeOrdered("ansi", a.doc, false)) {
		t.Error("Play does not send the effect")
	}
	want := readBack(a, "ansi", plainWrite(a, "ansi", a.doc), 80)
	if ok, why := cellsMatch("ansi", want, readBack(a, "ansi", b1, 80)); !ok {
		t.Fatal(why)
	}
	a.order = &drawOrder{Pattern: "matrix", Seed: 4243, Frames: 30}
	if b3, _ := a.encode("ansi"); string(b3) == string(b1) {
		t.Error("another seed, the same file")
	}
	a.order = o
	j, _ := json.Marshal(o)
	var back drawOrder
	if json.Unmarshal(j, &back) != nil || back != *o {
		t.Errorf("json %s", j)
	}
	p := filepath.Join(t.TempDir(), "m.ans")
	if !a.writeOut(p, "ansi") {
		t.Fatal(a.message)
	}
	if b, _ := os.ReadFile(p); len(b) < len(b1) || string(b[:len(b1)]) != string(b1) {
		t.Error("the file is not the effect")
	}
	a.typeByte('Z')
	if err := writeAutosave(a.Tab); err != nil {
		t.Fatal(err)
	}
	b := restart(t, "pc")
	files := leftAutosaves()
	if len(files) != 1 {
		t.Fatalf("%d autosaves", len(files))
	}
	if err := b.restoreAutosave(files[0]); err != nil {
		t.Fatal(err)
	}
	if b.order == nil || *b.order != *o {
		t.Fatalf("autosave: %+v", b.order)
	}
	// Teletext and Spectrum pages: frames only.
	for _, mode := range []string{"tt", "zx"} {
		c := testApp(t, mode)
		effectPage(c, rand.New(rand.NewSource(2)))
		c.applyOrder(&drawOrder{Pattern: "drop", Seed: 1})
		if c.order != nil {
			t.Errorf("%s: an order was set", mode)
		}
		c.applyFrames(animSpec{pat: patDrop, hold: 30, seed: 1})
		if c.nFrames() != effectDefaults[patDrop][0] {
			t.Errorf("%s: %d frames: %s", mode, c.nFrames(), c.message)
		}
	}
}

// The requester: an effect fills in its own length and hold, makes frames with
// its seed, or the saved file's order with the seed and length.
func TestAnimateRequesterEffects(t *testing.T) {
	a := testApp(t, "pc")
	randomPage(a, rand.New(rand.NewSource(6)), 80, 25)
	openAnimate()
	d := a.dialog.(*AnimReq)
	d.pat = patDecrypt
	d.syncDefaults()
	if d.length.String() != "55" || d.hold.String() != "40" {
		t.Fatalf("defaults %s %s", d.length.String(), d.hold.String())
	}
	if p := d.preview(); len(p) != 55 {
		t.Errorf("preview %d frames", len(p))
	}
	d.length.set("20")
	seed := d.seed
	d.frames = false
	d.apply()
	if a.order == nil || a.order.Pattern != "decrypt" || a.order.Seed != seed || a.order.Frames != 20 {
		t.Fatalf("order %+v", a.order)
	}
	openAnimate() // opens on the page's order, its seed and length
	d = a.dialog.(*AnimReq)
	if d.pat != patDecrypt || d.seed != seed || d.length.String() != "20" {
		t.Fatalf("reopened: %d %d %s", d.pat, d.seed, d.length.String())
	}
	d.key("S", false, false)
	if d.seed == seed {
		t.Error("S kept the seed")
	}
	d.frames = true
	d.apply()
	if a.nFrames() != 20 {
		t.Fatalf("%d frames", a.nFrames())
	}
	// Back to a scan pattern: its own hold again.
	openAnimate()
	d = a.dialog.(*AnimReq)
	d.pat = patMelt
	d.syncDefaults()
	d.pat = patSpiral
	d.syncDefaults()
	if d.hold.String() != "60" {
		t.Errorf("hold %s", d.hold.String())
	}
	closeDialog()
}
