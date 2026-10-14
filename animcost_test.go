package main

import (
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Every detail level of every template that can be a saved file, in every
// stream format: the file reads back as exactly the picture, as tall.
func TestDetailLevelsRoundTrip(t *testing.T) {
	r := rand.New(rand.NewSource(41))
	for _, m := range streamModes {
		a := testApp(t, m.mode)
		if m.mode == "st" && m.cols == 80 {
			a.stres = "medium"
		}
		randomPage(a, r, m.cols, m.rows)
		if m.fmtID == "ansi" {
			noControls(a.doc)
		}
		for _, p := range batchPatterns() {
			if p == patFadeIn && a.fadeRefused() != "" {
				continue
			}
			for _, det := range []int{detailMedium, detailSmall} {
				o := &drawOrder{Pattern: patterns[p].id, Seed: 9, Detail: detailIDs[det], Reverse: r.Intn(2) == 0, Flip: r.Intn(2) == 0}
				if isEffect(p) {
					o.Reverse, o.Flip = false, false
				}
				roundTrip(t, a, m.fmtID, o)
			}
		}
	}
}

// The frame effects below Large: every frame of the delta stream is the
// frame, the last is the picture, and the same seed gives the same frames.
func TestDetailEffectFrames(t *testing.T) {
	r := rand.New(rand.NewSource(43))
	for _, m := range streamModes[:6] {
		a := testApp(t, m.mode)
		randomPage(a, r, m.cols, m.rows)
		if m.fmtID == "ansi" {
			noControls(a.doc)
		}
		d := a.doc.clone()
		e := a.paperOf(d)
		for p := 0; p < nPatterns; p++ {
			if !isEffect(p) {
				continue
			}
			for _, det := range []int{detailMedium, detailSmall} {
				docs, holds, _ := a.transitionFrames(d, animSpec{pat: p, seed: 5, detail: det, hold: 40}, maxPageCells)
				own, _ := effectOwn(p, det)
				if len(docs) != own || len(holds) != own {
					t.Fatalf("%s %s: %d frames, want %d", patterns[p].id, detailIDs[det], len(docs), own)
				}
				out, ends := writeDeltaFrames(m.fmtID, a.mode, e, docs, a.streamOpts(false))
				for i, f := range docs {
					want := readBack(a, m.fmtID, plainWrite(a, m.fmtID, f), d.cols)
					got := readBack(a, m.fmtID, out[:ends[i]], d.cols)
					if ok, why := cellsMatch(m.fmtID, want, got); !ok {
						t.Fatalf("%s %s %s frame %d: %s", m.mode, patterns[p].id, detailIDs[det], i, why)
					}
				}
				again := a.effectFramesAt(d, e, p, 5, 0, det)
				for i := range docs {
					if string(cellBytes(docs[i])) != string(cellBytes(again[i])) {
						t.Fatalf("%s %s: frame %d differs with the same seed", patterns[p].id, detailIDs[det], i)
					}
				}
			}
		}
	}
}

func cellBytes(d *Doc) []byte {
	b := make([]byte, 0, 4*len(d.cells))
	for _, v := range d.cells {
		b = append(b, byte(v), byte(v>>8), byte(v>>16), byte(v>>24))
	}
	return b
}

// A real screen: Small is no larger than Medium, Medium no larger than
// Large; a draw order at Small costs little more than the plain file.
func TestDetailSizes(t *testing.T) {
	b, err := os.ReadFile("testdata/alove/bs-alove.bin")
	if err != nil {
		t.Skip(err)
	}
	a := testApp(t, "pc")
	d, _, _ := parseArt(b, ParseOpts{binary: true})
	a.doc = d
	for _, p := range batchPatterns() {
		var size [nDetails]int
		var plain int
		for det := range size {
			c := a.costOf("ansi", &drawOrder{Pattern: patterns[p].id, Seed: 7, Detail: detailIDs[det]}, "")
			size[det], plain = c.size, c.plain
		}
		if size[detailSmall] > size[detailMedium] || size[detailMedium] > size[detailLarge] {
			t.Errorf("%s: small %d, medium %d, large %d", patterns[p].id, size[detailSmall], size[detailMedium], size[detailLarge])
		}
		if isScan(p) && float64(size[detailSmall]) > 1.2*float64(plain) {
			t.Errorf("%s: small %d against the plain %d", patterns[p].id, size[detailSmall], plain)
		}
		t.Logf("%-12s plain %6d  large %7d  medium %7d  small %7d", patterns[p].id, plain, size[detailLarge], size[detailMedium], size[detailSmall])
	}
}

// Fewer frames are held longer: an effect takes about as long at every level.
func TestDetailHolds(t *testing.T) {
	for p := range effectDefaults {
		lf, lh := effectOwn(p, detailLarge)
		for _, det := range []int{detailMedium, detailSmall} {
			f, h := effectOwn(p, det)
			if f >= lf || h <= lh {
				t.Errorf("%s %s: %d frames of %d ms against %d of %d", patterns[p].id, detailIDs[det], f, h, lf, lh)
			}
			if total, want := f*h, lf*lh; total < want*9/10 || total > want*11/10 {
				t.Errorf("%s %s: %d ms in all, Large %d", patterns[p].id, detailIDs[det], total, want)
			}
		}
	}
	for _, s := range []string{"", "large", "Medium", "s"} {
		if detailByID(s) < 0 {
			t.Errorf("detail %q", s)
		}
	}
	if detailByID("huge") >= 0 {
		t.Error("huge")
	}
}

// The cost shown is what Save writes: the size of the file to the byte, and
// the time is the bytes the caller gets x 10 / bps plus the board's pauses.
func TestCostMatchesFile(t *testing.T) {
	a := testApp(t, "pc")
	randomPage(a, rand.New(rand.NewSource(3)), 80, 25)
	noControls(a.doc)
	dir := t.TempDir()
	for _, board := range []string{"", "amiexpress", "mystic", "pcboard", "synchronet"} {
		for _, pat := range []string{"clock", "matrix", "fadein", "assemble"} {
			for _, det := range detailIDs {
				o := &drawOrder{Pattern: pat, Seed: 11, Detail: det, BBS: board}
				if board == "" {
					o.BBS = "none"
				}
				c := a.costOf("ansi", o, board)
				a.order = o
				p := filepath.Join(dir, pat+det+board+".ans")
				if !a.writeOut(p, "ansi") {
					t.Fatal(a.message)
				}
				file, _ := os.ReadFile(p)
				if len(file) != c.size {
					t.Fatalf("%s %s %s: cost says %d bytes, the file has %d", board, pat, det, c.size, len(file))
				}
				var ps []pause
				sent, _ := a.encodeFor("ansi", "", nil)
				_, _ = a.encodeFor("ansi", board, &ps)
				ms := 0
				for _, q := range ps {
					ms += q.ms
				}
				if c.sent != len(sent) || c.pauseMs != ms {
					t.Fatalf("%s %s %s: sent %d/%d, pauses %d/%d", board, pat, det, c.sent, len(sent), c.pauseMs, ms)
				}
				for _, bps := range costSpeeds {
					want := ms
					if bps > 0 {
						want += (len(sent)*10*1000 + bps/2) / bps
					}
					if got := c.at(bps); got != want {
						t.Fatalf("%s %s at %d: %d ms, want %d", pat, det, bps, got, want)
					}
				}
				if board != "" && c.at(telnet) != ms {
					t.Fatalf("telnet: %d", c.at(telnet))
				}
			}
		}
	}
}

// The weight classes: Small draw orders are light, a Large Matrix rain is heavy.
func TestCostWeights(t *testing.T) {
	b, err := os.ReadFile("testdata/alove/bs-alove.bin")
	if err != nil {
		t.Skip(err)
	}
	a := testApp(t, "pc")
	a.doc, _, _ = parseArt(b, ParseOpts{binary: true})
	if w := a.costOf("ansi", &drawOrder{Pattern: "clock", Detail: "small"}, "").weight(); w != weightLight {
		t.Errorf("small clock: %s", weightNames[w])
	}
	if w := a.costOf("ansi", &drawOrder{Pattern: "matrix", Seed: 1}, "").weight(); w != weightHeavy {
		t.Errorf("large matrix: %s", weightNames[w])
	}
	if durWords(260000) != "4 min 20 s" || durWords(12400) != "12 s" || durWords(300) != "0.3 s" {
		t.Errorf("words: %s %s %s", durWords(260000), durWords(12400), durWords(300))
	}
}

// Animate a folder with a budget: every version keeps it (its file's size,
// its time at the speed: bytes x 10 / bps with no board), lower levels are
// taken where the asked one does not fit, a screen nothing fits is skipped
// with the reason, and light only takes light effects.
func TestBatchBudget(t *testing.T) {
	testApp(t, "pc")
	src := artFolder(t, 8)
	out := filepath.Join(t.TempDir(), "o")
	r := runBatch(t, batchSpec{Inputs: []string{src}, Effect: "random", NoRepeat: true, Seed: 5, Board: "none", Detail: "large",
		MaxKB: 16, MaxSeconds: 10, AtBps: 14400, Out: out})
	lowered := 0
	for _, it := range r.items {
		if it.skipped != "" {
			t.Fatalf("%s skipped: %s", it.rel, it.skipped)
		}
		for i, o := range it.outs {
			b, _ := os.ReadFile(filepath.Join(out, o))
			if len(b) > 16*1024 || int64(len(b)) != it.sizes[i] {
				t.Fatalf("%s: %d bytes (report %d)", o, len(b), it.sizes[i])
			}
			if it.times[i] != playMs(len(b), 0, 14400) || it.times[i] > 10000 {
				t.Fatalf("%s: %d ms, the file says %d", o, it.times[i], playMs(len(b), 0, 14400))
			}
			if it.orders[i].detail() != detailLarge {
				lowered++
			}
		}
	}
	if lowered == 0 {
		t.Error("no effect was played at a lower level")
	}
	// One heavy effect, fixed: lowered to fit, or the screen skipped with the reason.
	r = runBatch(t, batchSpec{Inputs: []string{src}, Effect: "assemble", Seed: 5, Board: "none", Detail: "large", MaxKB: 30,
		Out: filepath.Join(t.TempDir(), "p")})
	for _, it := range r.items {
		if it.skipped == "" && it.orders[0].detail() == detailLarge {
			t.Errorf("%s: a large Assemble in 30 KB", it.rel)
		}
	}
	r = runBatch(t, batchSpec{Inputs: []string{src}, Effect: "random", Seed: 5, Board: "none", MaxKB: 1, Out: filepath.Join(t.TempDir(), "q")})
	for _, it := range r.items {
		if it.skipped == "" || !strings.Contains(it.skipped, "nothing fits the budget") {
			t.Fatalf("%s: %q", it.rel, it.skipped)
		}
	}
	// Light only.
	r = runBatch(t, batchSpec{Inputs: []string{src}, Effect: "random", Seed: 6, Board: "none", Detail: "large", MaxWeight: "light", Variants: 3,
		Out: filepath.Join(t.TempDir(), "l")})
	n := 0
	for _, it := range r.items {
		withScratch(func() {
			p := filepath.Join(src, it.rel)
			b, _ := os.ReadFile(p)
			a.loadBytes(b, filepath.Base(p), p, "auto")
			for _, o := range it.orders {
				if c := a.costOf("ansi", o, ""); c.weight() != weightLight {
					t.Errorf("%s: %s is %s", it.rel, o, weightNames[c.weight()])
				}
				n++
			}
		})
	}
	if n == 0 {
		t.Fatal("nothing animated")
	}
	if !strings.Contains(r.reportText(), "light effects only") || !strings.Contains(r.commandLine(), "--light") {
		t.Error("the report does not say light only")
	}
}

// The command line takes the detail and the budget, and refuses bad values.
func TestBatchBudgetCommandLine(t *testing.T) {
	testApp(t, "pc")
	src := artFolder(t, 3)
	out := filepath.Join(t.TempDir(), "o")
	devnull, _ := os.Open(os.DevNull)
	defer devnull.Close()
	so, _ := os.Create(filepath.Join(t.TempDir(), "stdout"))
	defer so.Close()
	if st := animateCLI([]string{src, "--seed", "3", "--bbs", "none", "--detail", "small", "--max-kb", "40", "--max-seconds", "60", "--at", "2400", "--light", "--out", out}, so, so); st != 0 {
		b, _ := os.ReadFile(so.Name())
		t.Fatalf("exit %d: %s", st, b)
	}
	rep, _ := os.ReadFile(filepath.Join(out, "animate-report.txt"))
	for _, want := range []string{"Detail:   Small", "at most 40 KB", "at most 60 s at 2400 bps", "--detail small", "--max-kb 40", "--max-seconds 60", "--at 2400", "--light"} {
		if !strings.Contains(string(rep), want) {
			t.Errorf("the report has no %q:\n%s", want, rep)
		}
	}
	for _, bad := range [][]string{{"--detail", "huge"}, {"--max-kb", "0"}, {"--at", "fast"}, {"--weight", "heavy"}} {
		if st := animateCLI(append([]string{src}, bad...), so, so); st != 2 {
			t.Errorf("%v: exit %d", bad, st)
		}
	}
}

// Play and the requester's preview play the stream at the caller's speed
// with the board's pauses: done after bytes x 10 / bps plus the pauses (over
// telnet, the pauses alone).
func TestPlayAtSpeedWithPauses(t *testing.T) {
	a := testApp(t, "pc")
	randomPage(a, rand.New(rand.NewSource(8)), 80, 25)
	noControls(a.doc)
	a.order = &drawOrder{Pattern: "drop", Seed: 3, Detail: "small", BBS: "mystic"}
	for _, bps := range []int{2400, 14400, telnet} {
		a.baud = bps
		c := a.costOf("ansi", a.order, "mystic")
		want := playMs(len(a.playBytes()), c.pauseMs, bps)
		if c.pauseMs == 0 {
			t.Fatal("no pauses")
		}
		a.play()
		p := a.playing
		if len(p.pauses) == 0 {
			t.Fatal("Play has no pauses")
		}
		start := p.last
		now := start
		for a.playing != nil && now.Sub(start) < time.Hour {
			now = now.Add(10 * time.Millisecond)
			a.playStepAt(now)
		}
		got := int(now.Sub(start) / time.Millisecond)
		if got < want-30 || got > want+30*len(p.pauses)+30 {
			t.Errorf("Play at %s: done after %d ms, want %d", speedName(bps), got, want)
		}
		// The requester's preview of the same.
		openAnimate()
		d := a.dialog.(*AnimReq)
		d.pat, d.detail, d.bbs, d.seed = patDrop, detailSmall, "mystic", 3
		d.syncDefaults()
		sp := d.speedPreview()
		if string(sp.bytes) != string(a.playBytes()) {
			t.Fatal("the preview plays other bytes than Play")
		}
		now = time.Now()
		sp.restart(now)
		start = now
		for sp.pos < len(sp.bytes) && now.Sub(start) < time.Hour {
			now = now.Add(10 * time.Millisecond)
			sp.advance(now)
		}
		got = int(now.Sub(start) / time.Millisecond)
		if got < want-30 || got > want+30*len(sp.ps)+30 {
			t.Errorf("preview at %s: done after %d ms, want %d", speedName(bps), got, want)
		}
		closeDialog()
	}
}

// The requester's cost is the cost of what Apply sets, and it warns past the time in Settings.
func TestAnimateRequesterCost(t *testing.T) {
	a := testApp(t, "pc")
	randomPage(a, rand.New(rand.NewSource(12)), 80, 25)
	noControls(a.doc)
	openAnimate()
	d := a.dialog.(*AnimReq)
	d.pat, d.detail = patMatrix, detailLarge
	d.syncDefaults()
	c, ok := d.cost(d.pat, true)
	if !ok {
		t.Fatal("no cost")
	}
	if want := a.costOf("ansi", d.orderFor(d.pat), ""); c != want {
		t.Fatalf("cost %+v, want %+v", c, want)
	}
	if c.at(14400) <= animWarnMs() {
		t.Fatalf("a large Matrix rain plays in %d ms", c.at(14400))
	}
	prefs.AnimWarn = 9999
	if c.at(14400) > animWarnMs() {
		t.Error("the setting is not taken")
	}
	d.apply()
	if a.order == nil || a.order.Detail != "large" {
		t.Fatalf("order %+v", a.order)
	}
	b, _ := a.encode("ansi")
	if len(b) != c.size {
		t.Errorf("saved %d bytes, the requester said %d", len(b), c.size)
	}
	if prefs.AnimLevel != "large" {
		t.Errorf("the level is not remembered: %q", prefs.AnimLevel)
	}
	openAnimate() // opens at the page's level
	if d := a.dialog.(*AnimReq); d.detail != detailLarge {
		t.Errorf("reopened at %s", detailNames[d.detail])
	}
	closeDialog()
}
