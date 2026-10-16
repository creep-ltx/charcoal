package main

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"encoding/json"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// compoFStep is one step of work: cell n (row by row) gets an X, as an undo step.
func compoFStep(a *App) {
	a.pushUndo()
	n := a.steps - 1
	a.doc.set(n%a.doc.cols, n/a.doc.cols, makeCell('X', 7, 0, 0))
}

// compoFInk counts the X cells: on these pages it is the depth the picture was taken at.
func compoFInk(d *Doc) int {
	n := 0
	for _, v := range d.cells[:d.cols*d.rows] {
		if v&255 == 'X' {
			n++
		}
	}
	return n
}

func TestCompoStageLogThins(t *testing.T) {
	a := testApp(t, "pc")
	a.doc = newDoc(80, 80)
	for range 1000 {
		compoFStep(a)
	}
	if a.steps != 1000 {
		t.Fatalf("depth %d", a.steps)
	}
	l := a.stages
	if l == nil || len(l.Pics) > stageMax || len(l.Pics) < stageMax/2 {
		t.Fatalf("log of %v pictures", l)
	}
	for _, p := range l.Pics {
		if p.Step%l.Every != 0 {
			t.Fatalf("picture at %d with every %d", p.Step, l.Every)
		}
		if d := p.doc(); d == nil || compoFInk(d) != p.Step {
			t.Fatalf("picture at step %d does not show the page then", p.Step)
		}
	}
	// The log reaches back to the start, the undo stack only 300 steps.
	if l.Pics[0].Step != 0 || len(a.undo) != 300 {
		t.Fatalf("first picture %d, undo %d", l.Pics[0].Step, len(a.undo))
	}
	got := pickStages(a.stageCands(), 10)
	if len(got) != 10 || got[9].step != 1000 {
		t.Fatalf("stages %v", compoFSteps(got))
	}
	for k, c := range got {
		want := (k + 1) * 100
		if c.step < want-l.Every || c.step > want+l.Every {
			t.Errorf("stage %d at step %d, want about %d", k+1, c.step, want)
		}
		if compoFInk(c.page()) != c.step {
			t.Errorf("stage %d's picture is not the page at its step", k+1)
		}
	}
}

func compoFSteps(c []stageCand) []int {
	var s []int
	for _, x := range c {
		s = append(s, x.step)
	}
	return s
}

func TestCompoStagesFollowUndo(t *testing.T) {
	a := testApp(t, "pc")
	for range 50 {
		compoFStep(a)
	}
	for range 20 {
		a.doUndo()
	}
	if a.steps != 30 {
		t.Fatalf("depth after undo %d", a.steps)
	}
	a.doRedo()
	a.doUndo()
	for range 10 { // another branch: the undone steps are gone from the history
		compoFStep(a)
	}
	if a.steps != 40 {
		t.Fatalf("depth %d", a.steps)
	}
	for _, c := range a.stageCands() {
		if compoFInk(c.page()) != c.step {
			t.Fatalf("picture at %d shows %d cells: from the undone branch", c.step, compoFInk(c.page()))
		}
	}
	got := pickStages(a.stageCands(), 4)
	if !slices.Equal(compoFSteps(got), []int{10, 20, 30, 40}) {
		t.Fatalf("stages %v", compoFSteps(got))
	}
}

func TestCompoPickStages(t *testing.T) {
	c := func(steps ...int) []stageCand {
		var out []stageCand
		for _, s := range steps {
			out = append(out, stageCand{step: s})
		}
		return out
	}
	for _, tc := range []struct {
		have []int
		n    int
		want []int
	}{
		{[]int{0, 1, 2, 3}, 10, []int{1, 2, 3}},                // fewer pictures than stages: all, no copies
		{[]int{0}, 3, []int{0}},                                // nothing drawn: the page as it is
		{[]int{0, 5, 6, 7, 8, 100}, 2, []int{8, 100}},          // stage 1 wants 50: the nearest is 8
		{[]int{0, 90, 95, 99, 100}, 4, []int{90, 95, 99, 100}}, // late pictures: still four different ones
		{[]int{0, 10, 20, 30, 40, 50, 60}, 3, []int{20, 40, 60}},
	} {
		if got := compoFSteps(pickStages(c(tc.have...), tc.n)); !slices.Equal(got, tc.want) {
			t.Errorf("pickStages(%v, %d) = %v, want %v", tc.have, tc.n, got, tc.want)
		}
	}
}

func TestCompoStagesKeptWithTheFile(t *testing.T) {
	a := testApp(t, "pc")
	for range 40 {
		compoFStep(a)
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "art.ans")
	if !a.writeOut(p, "ansi") {
		t.Fatal(a.message)
	}
	b := restart(t, "pc")
	b.openPath(p, "auto")
	if b.stages == nil || b.steps != 40 {
		t.Fatalf("log not back: %v depth %d", b.stages != nil, b.steps)
	}
	for range 20 {
		compoFStep(b)
	}
	out := t.TempDir()
	files, err := b.exportStages(out, "art", 6, 1, "ansi")
	if err != nil || len(files) != 12 {
		t.Fatalf("export: %v %v", files, err)
	}
	if filepath.Base(files[0]) != "art-stage01.png" || filepath.Base(files[11]) != "art-stage06.ans" {
		t.Fatalf("names %v", files)
	}
	for _, f := range files {
		data, _ := os.ReadFile(f)
		if strings.HasSuffix(f, ".png") {
			if _, err := png.Decode(bytes.NewReader(data)); err != nil {
				t.Fatalf("%s: %v", f, err)
			}
		}
	}
	// The last stage is the page now; the first ones come from before the reopen.
	last, _ := os.ReadFile(files[10])
	now, _ := b.encode("png")
	if !bytes.Equal(last, now) {
		t.Error("the last stage is not the page now")
	}
	first, _ := os.ReadFile(files[1])
	if d, _, _ := parseArt(first, ParseOpts{}); compoFInk(d) != 10 {
		t.Errorf("stage 1 shows %d cells, want 10", compoFInk(d))
	}

	// Changed elsewhere: the history is not that file's any more.
	os.WriteFile(p, []byte("other art"), 0o644)
	c := restart(t, "pc")
	c.openPath(p, "auto")
	if c.stages != nil || c.steps != 0 {
		t.Error("a log taken for a file changed elsewhere")
	}
}

func TestCompoStagesInAutosave(t *testing.T) {
	a := testApp(t, "pc")
	for range 30 {
		compoFStep(a)
	}
	if err := writeAutosave(a.Tab); err != nil {
		t.Fatal(err)
	}
	b := restart(t, "pc")
	files := leftAutosaves()
	if len(files) != 1 {
		t.Skipf("%d autosaves (this CharEdit's own are not offered)", len(files))
	}
	if err := b.restoreAutosave(files[0]); err != nil {
		t.Fatal(err)
	}
	if b.stages == nil || b.steps != 30 || len(b.stages.Pics) != 30 {
		t.Fatalf("log after restore: depth %d", b.steps)
	}
}

func TestCompoStagesUnicode(t *testing.T) {
	a := testApp(t, "uni")
	d := newDoc(4, 1)
	d.uni = true
	d.cells[0] = uniCell(0x2588, styleOf(rgbFlag|0x123456, 200, italic))
	d.cells[1] = uniCell('A', styleOf(3, 4, 0))
	p := packDoc(d, 1)
	if len(p.Pairs) != 1 {
		t.Fatalf("pairs %v", p.Pairs)
	}
	// Another run (or a compaction) numbers the styles differently.
	styleOf(9, 99, 0)
	back := p.doc()
	fg, bg := uniColoursOf(back.cells[0])
	if uniCode(back.cells[0]) != 0x2588 || fg != rgbFlag|0x123456 || bg != 200 || uniFlagsOf(back.cells[0]) != italic || back.cells[1] != d.cells[1] {
		t.Fatalf("Unicode picture changed: %x", back.cells[:2])
	}
	_ = a
}

func TestCompoPackedSize(t *testing.T) {
	// Text art packs small: an 80x500 page of typical art (here: rows of a few runs).
	d := newDoc(80, 500)
	for y := range 500 {
		for x := range 80 {
			if (x+y)%7 < 3 {
				d.set(x, y, makeCell(0xB0+(x/20)%3, 1+y%7, 0, 0))
			}
		}
	}
	p := packDoc(d, 0)
	raw := 4 * 80 * 500
	if len(p.Cells)*10 > raw {
		t.Errorf("packed %d of %d bytes", len(p.Cells), raw)
	}
	t.Logf("80x500 page: %d bytes packed of %d", len(p.Cells), raw)
	// A damaged picture is no page.
	q := p
	q.Cells = q.Cells[:len(q.Cells)/2]
	if q.doc() != nil {
		t.Error("half a picture read")
	}
	var buf bytes.Buffer
	w, _ := flate.NewWriter(&buf, 1)
	w.Write(binary.LittleEndian.AppendUint32(nil, 1))
	w.Close()
	if (stagePic{Cols: 80, Rows: 500, Cells: buf.Bytes()}).doc() != nil {
		t.Error("too few cells read")
	}
}

// ---------------------------------------------------------------- the party check

func compoFCheck(a *App, id string) []string { return a.compoCheck(compoByID(id)) }

func compoFHas(probs []string, part string) bool {
	for _, p := range probs {
		if strings.Contains(p, part) {
			return true
		}
	}
	return false
}

func TestCompoRulesTable(t *testing.T) {
	ids := map[string]bool{}
	known := map[string]bool{}
	for _, f := range formats {
		known[f.id] = true
	}
	for _, r := range compoRules {
		if ids[r.id] || r.name == "" || !strings.HasPrefix(r.url, "https://") || len(r.plats) == 0 || r.edition == "" {
			t.Errorf("rule %q incomplete", r.id)
		}
		ids[r.id] = true
		for _, p := range r.plats {
			for _, m := range strings.Fields(p.modes) {
				if modeNames[m] == "" {
					t.Errorf("%s: no page type %q", r.id, m)
				}
			}
			for _, f := range p.deliver {
				if !known[f] {
					t.Errorf("%s: no format %q", r.id, f)
				}
			}
		}
	}
	for _, id := range []string{"revision-text", "revision-oldskool", "evoke", "lovebyte", "zoo", "deadline", "x", "amscii", "atascii", "teletext-advent"} {
		if !ids[id] {
			t.Errorf("no rules for %s", id)
		}
	}
	if r := compoByID("revision-oldskool"); r.stages != 10 || r.atLeast {
		t.Error("Revision oldskool: exactly 10 stages")
	}
	if r := compoByID("zoo"); r.stages != 15 || !r.atLeast {
		t.Error("Zoo: at least 15 stages")
	}
}

func TestCompoCheckPC(t *testing.T) {
	a := testApp(t, "pc")
	a.setCols(80)
	a.setRows(25)
	a.doc.set(0, 0, makeCell('A', 7, 0, 0))
	for _, id := range []string{"revision-text", "evoke", "deadline"} {
		if p := compoFCheck(a, id); len(p) > 0 {
			t.Errorf("%s: a plain 80x25 page breaks %v", id, p)
		}
	}
	if p := compoFCheck(a, "lovebyte"); !compoFHas(p, "2000") {
		t.Errorf("Lovebyte took 80x25: %v", p)
	}
	if p := compoFCheck(a, "zoo"); !compoFHas(p, "this is a PC page") {
		t.Errorf("Zoo took a PC page: %v", p)
	}
	// Blinking (bright background without iCE), then iCE.
	a.doc.set(1, 0, makeCell('B', 7, 9, 0))
	if p := compoFCheck(a, "revision-text"); !compoFHas(p, "1 cells blink") {
		t.Errorf("blink: %v", p)
	}
	if p := compoFCheck(a, "deadline"); !compoFHas(p, "blink") {
		t.Errorf("Deadline blink: %v", p)
	}
	a.ice = true
	if p := compoFCheck(a, "revision-text"); !compoFHas(p, "iCE") {
		t.Errorf("iCE: %v", p)
	}
	if p := compoFCheck(a, "deadline"); len(p) > 0 {
		t.Errorf("Deadline takes iCE: %v", p)
	}
	// Size: Evoke's one screen, Revision's 500 rows (with a hint when the rest is empty).
	a.setRows(26)
	if p := compoFCheck(a, "evoke"); !compoFHas(p, "26 rows") || !compoFHas(p, "empty") {
		t.Errorf("Evoke size: %v", p)
	}
	a.setRows(501)
	a.doc.set(0, 500, makeCell('Z', 7, 0, 0))
	if p := compoFCheck(a, "revision-text"); !compoFHas(p, "501 rows") || compoFHas(p, "empty") {
		t.Errorf("Revision size: %v", p)
	}
	// Lovebyte: 16x16 fits, 17x16 does not.
	b := testApp(t, "pc")
	b.setCols(16)
	b.setRows(16)
	if p := compoFCheck(b, "lovebyte"); len(p) > 0 {
		t.Errorf("16x16: %v", p)
	}
	b.setCols(17)
	if p := compoFCheck(b, "lovebyte"); !compoFHas(p, "272") {
		t.Errorf("17x16: %v", p)
	}
	// Frames: no animation.
	b.setCols(16)
	b.newFrame()
	if p := compoFCheck(b, "lovebyte"); !compoFHas(p, "2 frames") {
		t.Errorf("frames: %v", p)
	}
}

func TestCompoCheckMachines(t *testing.T) {
	a := testApp(t, "c64")
	a.machine = "c64"
	// A small drawing on the 40x25 screen: Lovebyte counts the drawing.
	for y := range 16 {
		for x := range 16 {
			a.doc.set(10+x, 3+y, makeCell(0xA0, 1, 0, 0))
		}
	}
	for _, id := range []string{"lovebyte", "zoo", "x", "revision-text", "deadline", "revision-oldskool"} {
		if p := compoFCheck(a, id); len(p) > 0 {
			t.Errorf("%s: %v", id, p)
		}
	}
	a.doc.set(0, 0, makeCell('A', 1, 0, 0))
	if p := compoFCheck(a, "lovebyte"); !compoFHas(p, "26x19") {
		t.Errorf("Lovebyte drawing size: %v", p)
	}
	a.vic.Mode = vicMulti
	if p := compoFCheck(a, "zoo"); !compoFHas(p, "multicolour") {
		t.Errorf("Zoo MCM: %v", p)
	}
	if p := compoFCheck(a, "deadline"); len(p) > 0 {
		t.Errorf("Deadline takes MCM: %v", p)
	}
	a.vic.Mode = 0
	a.machine = "vic20"
	if p := compoFCheck(a, "zoo"); !compoFHas(p, "run on a C64") {
		t.Errorf("Zoo VIC-20: %v", p)
	}

	at := testApp(t, "atari")
	at.setCols(38)
	if p := compoFCheck(at, "atascii"); !compoFHas(p, "38 columns") {
		t.Errorf("ATASCII 38: %v", p)
	}
	at.setCols(40)
	if p := compoFCheck(at, "atascii"); len(p) > 0 {
		t.Errorf("ATASCII 40: %v", p)
	}
	at.ownFont, at.fontFor["atari"] = &Font{name: "mine", kind: "atari", bits: make([]byte, 1024)}, ownID
	if p := compoFCheck(at, "atascii"); !compoFHas(p, "own font") {
		t.Errorf("ATASCII own font: %v", p)
	}

	tt := testApp(t, "tt")
	if p := compoFCheck(tt, "teletext-advent"); len(p) > 0 {
		t.Errorf("empty teletext page: %v", p)
	}
	tt.doc.set(5, 0, 'A')
	tt.doc.set(5, 24, 'B')
	if p := compoFCheck(tt, "teletext-advent"); !compoFHas(p, "Row 0") || !compoFHas(p, "Row 24") {
		t.Errorf("teletext rows: %v", p)
	}
}

func TestCompoSaveForParty(t *testing.T) {
	a := testApp(t, "pc")
	a.setCols(80)
	a.setRows(25)
	for range 30 {
		compoFStep(a)
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "my art.ans")
	if !a.writeOut(p, "ansi") {
		t.Fatal(a.message)
	}
	a.saveForParty(compoByID("revision-text"))
	for _, f := range []string{"my art.ans", "my art.png"} {
		if _, err := os.Stat(filepath.Join(dir, "my art-revision-text", f)); err != nil {
			t.Errorf("%s: %v (%s)", f, err, a.message)
		}
	}
	saved, _ := os.ReadFile(p)
	party, _ := os.ReadFile(filepath.Join(dir, "my art-revision-text", "my art.ans"))
	if !bytes.Equal(saved, party) {
		t.Error("the party's .ans differs from Save's")
	}
	// A compo of another platform writes nothing.
	a.saveForParty(compoByID("zoo"))
	if _, err := os.Stat(filepath.Join(dir, "my art-zoo")); err == nil {
		t.Error("Zoo's folder made for a PC page")
	}
	// Lovebyte has no stages; a compo with stages gets them in workstages/ (Evoke has none either:
	// use Revision oldskool's 10 on a CPC page).
	c := testApp(t, "cpc")
	for range 30 {
		compoFStep(c)
	}
	cp := filepath.Join(dir, "longername.bin")
	if !c.writeOut(cp, "cpcbin") {
		t.Fatal(c.message)
	}
	c.saveForParty(compoByID("amscii"))
	for _, f := range []string{"longerna.bin", "longerna.dsk", "longerna.png", "longerna.psci"} {
		if _, err := os.Stat(filepath.Join(dir, "longername-amscii", f)); err != nil {
			t.Errorf("%s: %v (%s)", f, err, c.message)
		}
	}
	c.saveForParty(compoByID("revision-oldskool"))
	st, _ := filepath.Glob(filepath.Join(dir, "longername-revision-oldskool", "workstages", "*.png"))
	if len(st) != 10 {
		t.Errorf("%d work stages written: %s", len(st), c.message)
	}
}

// TestCompoPackedRealArt measures packed pictures of real art (CHARCOAL_COMPO_FILES=a folder of .ans).
func TestCompoPackedRealArt(t *testing.T) {
	dir := os.Getenv("CHARCOAL_COMPO_FILES")
	if dir == "" {
		t.Skip("CHARCOAL_COMPO_FILES not set")
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*.[aA][nN][sS]"))
	raw, packed, n, worst := 0, 0, 0, 0.0
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		d, _, _ := parseArt(b, ParseOpts{})
		if d == nil || d.rows < 2 {
			continue
		}
		p := packDoc(d, 0)
		raw, packed, n = raw+4*d.cols*d.rows, packed+len(p.Cells), n+1
		worst = max(worst, float64(len(p.Cells))/float64(4*d.cols*d.rows))
	}
	if n == 0 {
		t.Skip("no .ans files")
	}
	t.Logf("%d files: %d bytes of cells packed to %d (%.1f%%), worst %.1f%%", n, raw, packed, 100*float64(packed)/float64(raw), 100*worst)
}

// FuzzCompoStageLog: any stage log file is read without a crash or a huge page.
func FuzzCompoStageLog(f *testing.F) {
	d := newDoc(8, 2)
	d.set(1, 1, makeCell('A', 3, 1, 0))
	good, _ := json.Marshal(&stageLog{Ver: stageVer, Mode: "pc", Every: 1, Depth: 3, Pics: []stagePic{packDoc(d, 1), packDoc(d, 3)}})
	f.Add(good)
	f.Add([]byte(`{"version":1,"every":1,"depth":9,"pictures":[{"step":1,"cols":1000,"rows":100000,"cells":"AAAA"}]}`))
	f.Fuzz(func(t *testing.T, b []byte) {
		p := filepath.Join(t.TempDir(), "log.json")
		os.WriteFile(p, b, 0o600)
		l, err := readStageLog(p)
		if err != nil {
			return
		}
		for _, pc := range l.Pics {
			if d := pc.doc(); d != nil && len(d.cells) > maxPageCells {
				t.Fatalf("page of %d cells", len(d.cells))
			}
		}
	})
}
