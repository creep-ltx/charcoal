package main

// The Animate a folder requester (batch.go does the work): the folder, which
// files, the effect (one, or a different one each from a seed), the pace and
// the board, versions for boards that pick a screen at random, and where the
// files go. Then a progress bar (one file a step, Cancel stops), then the
// report, which is also saved in the output folder.

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type BatchReq struct {
	folder, out, seed  Field
	every, pause, hold Field
	variants, aePath   Field
	subfolders         bool
	kinds              map[string]bool
	effect             int // 0: random, else batchPatterns()[effect-1]
	ticked             map[int]bool
	fromTicked         bool
	noRepeat, toggles  bool
	board              string // "" My BBS, "none", or an id
	copies             string // ids, comma separated (drawOrder.toggleCopy)
	replace            bool
	outTyped           bool   // the output folder was typed (else it follows the folder)
	outFor             string // the folder the output name was made for
	err                string
	run                *batchRun
	report             []string // the finished report's lines
	top                int
	detail             int    // the detail level (animcost.go)
	maxKB, maxSecs     Field  // the budget (empty: none)
	at                 int    // the budget's and the report's speed
	weight             string // "" any, "light", "medium"
}

// openAnimateFolder opens the requester, on the folder of the page's file (or the last one).
func openAnimateFolder() {
	a.endPlay()
	d := &BatchReq{kinds: map[string]bool{}, ticked: map[int]bool{}, noRepeat: true, toggles: true}
	for _, k := range batchKinds {
		d.kinds[k.id] = true
	}
	for _, p := range batchPatterns() {
		d.ticked[p] = true
	}
	dir := a.lastDir
	if a.path != "" {
		dir = filepath.Dir(a.path)
	}
	d.folder.set(startDir(dir))
	d.seed.max, d.every.max, d.pause.max, d.hold.max, d.variants.max = 10, 5, 5, 5, 3
	d.seed.set(strconv.FormatInt(newSeed(), 10))
	d.every.set("40")
	d.pause.set("40")
	d.variants.set("1")
	d.aePath.set("BBS:Node0/Screens/")
	d.maxKB.max, d.maxSecs.max = 5, 4
	d.at = a.baud
	if d.detail = detailByID(prefs.AnimLevel); prefs.AnimLevel == "" || d.detail < 0 {
		d.detail = detailMedium
	}
	d.syncOut()
	a.dialog = d
}

// batchBusy: a run is going (the window keeps drawing so it moves on).
func batchBusy() bool {
	d, ok := a.dialog.(*BatchReq)
	return ok && d.run != nil && d.report == nil
}

// syncOut makes the output folder <folder>-animated while nobody has typed one.
func (d *BatchReq) syncOut() {
	f := strings.TrimRight(expandHome(strings.TrimSpace(d.folder.String())), "/\\")
	if d.outTyped && d.out.String() != "" || f == d.outFor {
		return
	}
	d.outFor = f
	if f != "" {
		d.out.set(f + "-animated")
	}
}

// spec is what the requester asks for.
func (d *BatchReq) spec() batchSpec {
	s := batchSpec{Inputs: []string{strings.TrimSpace(d.folder.String())}, Subfolders: d.subfolders, Types: map[string]bool{},
		NoRepeat: d.noRepeat, Toggles: d.toggles, Board: d.board, Replace: d.replace, Out: strings.TrimSpace(d.out.String()),
		AEPath: strings.TrimSpace(d.aePath.String())}
	for k, on := range d.kinds {
		if on {
			s.Types[k] = true
		}
	}
	pats := batchPatterns()
	if d.effect == 0 {
		s.Effect = "random"
		if d.fromTicked {
			for _, p := range pats {
				if d.ticked[p] {
					s.Pool = append(s.Pool, patterns[p].id)
				}
			}
			if len(s.Pool) == 0 {
				s.Pool = []string{"-"} // nothing ticked: refused by newBatchRun
			}
		}
	} else {
		s.Effect = patterns[pats[d.effect-1]].id
	}
	s.Seed, _ = strconv.ParseInt(d.seed.String(), 10, 64)
	s.Every = fieldInt(&d.every, 40, 1, 1<<20)
	s.Pause = fieldInt(&d.pause, 40, 20, 99000)
	if s.Every == 40 {
		s.Every = 0
	}
	if s.Pause == 40 {
		s.Pause = 0
	}
	if d.hold.String() != "" {
		s.Hold = fieldInt(&d.hold, 0, 20, 99000)
	}
	s.Variants = fieldInt(&d.variants, 1, 1, 999)
	if d.copies != "" {
		s.Copies = strings.Split(d.copies, ",")
	}
	if s.Replace {
		s.Out = ""
	}
	s.Detail = detailIDs[d.detail]
	if d.maxKB.String() != "" {
		s.MaxKB = fieldInt(&d.maxKB, 0, 1, 1<<20)
	}
	if d.maxSecs.String() != "" {
		s.MaxSeconds = fieldInt(&d.maxSecs, 0, 1, 99999)
	}
	s.AtBps, s.MaxWeight = d.at, d.weight
	return s
}

// start begins the run, or says why it cannot.
func (d *BatchReq) start() {
	r := newBatchRun(d.spec())
	if r.err != "" {
		d.err = r.err
		return
	}
	d.err, d.run = "", r
	ui.focus = nil
}

// effectBoard is the board the files get codes for ("" none).
func (d *BatchReq) effectBoard() string {
	switch d.board {
	case "":
		return prefs.MyBBS
	case "none":
		return ""
	}
	return d.board
}

func (d *BatchReq) draw(u *UI) {
	switch {
	case d.report != nil:
		d.drawReport(u)
	case d.run != nil:
		d.drawProgress(u)
	default:
		d.drawSetup(u)
	}
}

func (d *BatchReq) drawSetup(u *UI) {
	fb := u.fb
	d.syncOut()
	in := u.window(centre(1000, 790), "Animate a folder")
	x0, y := in.x, in.y
	label := func(x int, s string) int { return fb.text(x, y+3, s, cText) + 8 }
	field := func(x, chars int, f *Field, hint string) int {
		u.field(Rect{x, y, digitsW(chars) + 16, gadH}, f, hint)
		return x + digitsW(chars) + 24
	}
	line := func(s string, col uint32) {
		for _, l := range wrap(s, in.w) {
			fb.text(x0, y, l, col)
			y += lineH
		}
	}
	// The folder.
	x := label(x0, "Folder")
	bw := buttonW("Choose...")
	u.field(Rect{x, y, in.x + in.w - x - bw - 8, gadH}, &d.folder, "The folder of the screens to animate")
	if u.button(Rect{in.x + in.w - bw, y, bw, gadH}, "Choose...", false, "Pick the folder in a requester") != nil {
		d.chooseFolder()
		return
	}
	y += gadH + 6
	x = x0
	var w int
	d.subfolders, w = u.check(x, y, "Subfolders too", d.subfolders, "The folders inside it as well (kept as folders in the output)")
	x += w + 24
	x = label(x, "Files:")
	for _, k := range batchKinds {
		name := k.name
		if k.id == "other" {
			name = "other art (.bin .xb .pcb .avt ...)"
		}
		if x+checkW(name) > in.x+in.w {
			x, y = x0+16, y+gadH+4
		}
		d.kinds[k.id], w = u.check(x, y, name, d.kinds[k.id], "Animate files of this kind ("+k.name+"); files that are not art are skipped anyway")
		x += w + 16
	}
	y += gadH + 10
	// The effect.
	pats := batchPatterns()
	items := []string{"Random: a different effect for each file"}
	for _, p := range pats {
		items = append(items, patterns[p].name)
	}
	x = label(x0, "Effect")
	u.choice("batcheffect", Rect{x, y, choiceW(items), gadH}, items, d.effect, "One effect for every file, or a different one for each", func(i int) { d.effect = i })
	x += choiceW(items) + 16
	x = label(x, "Seed")
	x = field(x, 10, &d.seed, "The random choices: the same seed gives the same files again")
	if u.button(Rect{x - 8, y, buttonW("New"), gadH}, "New", false, "Another seed") != nil {
		d.seed.set(strconv.FormatInt(newSeed(), 10))
	}
	y += gadH + 6
	x = x0
	if d.effect == 0 {
		from := []string{"from all of them", "from the ticked ones"}
		u.choice("batchfrom", Rect{x, y, choiceW(from), gadH}, from, map[bool]int{true: 1}[d.fromTicked], "Which effects the random pick may take",
			func(i int) { d.fromTicked = i == 1 })
		x += choiceW(from) + 16
		d.noRepeat, w = u.check(x, y, "Never the same twice in a row", d.noRepeat, "The next file never gets the effect the one before it got")
		x += w + 16
	}
	d.toggles, _ = u.check(x, y, "Reverse and Flip at random", d.toggles, "TheDraw's Reverse and Flip, on or off at random for each file (patterns that have them)")
	y += gadH + 6
	if d.effect == 0 && d.fromTicked {
		cols := 6
		cw := in.w / cols
		ws := typicalWeights(d.detail)
		for i, p := range pats {
			cx, cy := x0+(i%cols)*cw, y+(i/cols)*(gadH+2)
			on, _ := u.check(cx, cy, patterns[p].name, d.ticked[p], fmt.Sprintf("%s (%s on a typical screen at %s)", patterns[p].help, weightNames[ws[p]], detailNames[d.detail]))
			d.ticked[p] = on
			weightBars(fb, cx+cw-30, cy+6, ws[p])
		}
		y += (len(pats)+cols-1)/cols*(gadH+2) + 2
		lx := x0
		for i, n := range weightNames {
			weightBars(fb, lx, y+3, i)
			lx = fb.text(lx+22, y, n, cDim) + 12
		}
		fb.text(lx, y, "on a made-up 80x25 screen at "+detailNames[d.detail]+"; Effects below weighs each screen itself", cDim)
		y += lineH + 6
	} else if d.effect > 0 {
		line(patterns[pats[d.effect-1]].help, cText)
		y += 4
	}
	y += 4
	// The size: the level, the weights taken, the budget.
	x = label(x0, "Detail")
	var levels []string
	cur := 0
	for i, l := range detailsUp {
		levels = append(levels, detailNames[l])
		if l == d.detail {
			cur = i
		}
	}
	u.choice("batchdetail", Rect{x, y, choiceW(levels), gadH}, levels, cur,
		"Small: fewer frames and changes, a draw order in runs of cells (for modems); Large: the full look",
		func(i int) { d.detail = detailsUp[i] })
	x += choiceW(levels) + 16
	x = label(x, "Effects")
	weights := []string{"any weight", "light only", "light and medium"}
	wcur := map[string]int{"light": 1, "medium": 2}[d.weight]
	u.choice("batchweight", Rect{x, y, choiceW(weights), gadH}, weights, wcur,
		"Light: at most 1.5x the screen's plain file; medium: at most 4x (weighed on each screen, lower levels tried)",
		func(i int) { d.weight = []string{"", "light", "medium"}[i] })
	y += gadH + 6
	x = label(x0, "Budget: at most")
	x = field(x, 5, &d.maxKB, "Each version of a screen at most this many KB (empty: no limit)")
	x = label(x, "KB a screen, plays in under")
	x = field(x, 4, &d.maxSecs, "Each version plays in at most this many seconds at the speed (empty: no limit)")
	x = label(x, "s at")
	var speeds []string
	scur := 2
	for i, b := range bauds {
		speeds = append(speeds, speedName(b))
		if b == d.at {
			scur = i
		}
	}
	u.choice("batchat", Rect{x, y, choiceW(speeds), gadH}, speeds, scur, "The caller's speed, for the budget and the report's times",
		func(i int) { d.at = bauds[i] })
	y += gadH + 4
	if d.maxKB.String() != "" || d.maxSecs.String() != "" || d.weight != "" {
		line("With a budget an effect that does not fit is played at a lower level, then another is tried; a screen nothing fits is skipped, and the report says why.", cDim)
	} else {
		line("Empty fields: no budget. The report gives each file's size and play time at the speed.", cDim)
	}
	y += 6
	// The pace and the board.
	x = label(x0, "A pause of")
	x = field(x, 5, &d.pause, "How long the board waits each time, in ms")
	x = label(x, "ms every")
	x = field(x, 5, &d.every, "Cells drawn between two pauses (scan patterns)")
	x = label(x, "cells; frame effects hold")
	x = field(x, 5, &d.hold, "ms each frame of a frame effect is held (empty: each effect's own)")
	label(x, "ms a frame (empty: their own)")
	y += gadH + 8
	boards := []string{"My BBS (" + boardName(prefs.MyBBS) + ")", "None"}
	cur = 0
	if d.board == "none" {
		cur = 1
	}
	for i, b := range bbsBoards {
		boards = append(boards, b.name)
		if b.id == d.board {
			cur = i + 2
		}
	}
	x = label(x0, "BBS")
	u.choice("batchbbs", Rect{x, y, choiceW(boards), gadH}, boards, cur,
		"The board whose pause codes the files get, and whose way the versions are named (My BBS is in Settings)",
		func(i int) {
			switch i {
			case 0:
				d.board = ""
			case 1:
				d.board = "none"
			default:
				d.board = bbsBoards[i-2].id
			}
		})
	x += choiceW(boards) + 16
	x = label(x, "Also a copy for")
	for _, b := range bbsBoards {
		if x+checkW(b.name) > in.x+in.w {
			x, y = x0+16, y+gadH+4
		}
		on := strings.Contains(","+d.copies+",", ","+b.id+",")
		v, cw := u.check(x, y, b.name, on, "Each file also as name-"+b.id+".ans with "+b.name+"'s codes")
		if v != on {
			o := drawOrder{Copies: d.copies}
			o.toggleCopy(b.id)
			d.copies = o.Copies
		}
		x += cw + 14
	}
	y += gadH + 4
	if b := boardByID(d.effectBoard()); b != nil {
		line(b.name+": "+b.note+". Only .ans files: a board with no codes for .seq, .ata or .vt skips those (None animates them at the modem's pace).", cDim)
	} else {
		line("No board: the files play at the modem's pace (over telnet they flash past). None is right for art packs and 16colo.rs.", cDim)
	}
	y += 6
	// Versions.
	x = label(x0, "Versions of each screen")
	x = field(x, 3, &d.variants, "For boards that show one of several versions at random: each with another effect")
	if fieldInt(&d.variants, 1, 1, 999) > 1 && d.effectBoard() == "amiexpress" {
		x = label(x, "Screens folder on the board")
		u.field(Rect{x, y, in.x + in.w - x, gadH}, &d.aePath, "Where the screens are on the Amiga (the node's SCREENS tooltype), for ~<n>SR_")
	}
	y += gadH + 4
	if fieldInt(&d.variants, 1, 1, 999) > 1 {
		line(variantNote(d.effectBoard())+".", cDim)
	}
	y += 6
	// The output.
	x = label(x0, "Save to")
	if d.replace {
		fb.text(x, y+3, "the same files: each original kept as name.bak", cHi)
	} else {
		was := d.out.String()
		u.field(Rect{x, y, in.x + in.w - x, gadH}, &d.out, "A new folder for the animated files (the names kept); never the folder of the originals")
		if d.out.String() != was {
			d.outTyped = true
		}
	}
	y += gadH + 6
	d.replace, _ = u.check(x0, y, "Replace the originals (keeps .bak copies)", d.replace,
		"Write the animated files over the originals; each original is kept beside it as .bak")
	y += gadH + 6
	if d.err != "" {
		line(d.err, cHi)
	}
	switch buttonRow(u, in, "Cancel", "Animate") {
	case 0:
		closeDialog()
	case 1:
		d.start()
	}
}

// chooseFolder picks the folder in a requester, then comes back here.
func (d *BatchReq) chooseFolder() {
	fr := &FileReq{sel: -1, title: "Choose the folder to animate", note: "Open the folder, then Choose", pickDir: true, back: d}
	fr.listFirst(startDir(expandHome(strings.TrimSpace(d.folder.String()))))
	fr.path.enter = fr.enter
	fr.pick = func(p string) {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			p = filepath.Dir(p)
		}
		d.folder.set(p)
		a.dialog = d
	}
	a.dialog = fr
	ui.focus = &fr.path
}

// drawProgress moves the run on for a moment and shows how far it is.
func (d *BatchReq) drawProgress(u *UI) {
	r := d.run
	until := time.Now().Add(40 * time.Millisecond)
	for !r.done() && time.Now().Before(until) {
		r.step()
	}
	fb := u.fb
	in := u.window(centre(700, 200), "Animate a folder")
	y := in.y
	n := len(r.files)
	fb.text(in.x, y, fmt.Sprintf("%d of %d files", r.next, n), cText)
	y += lineH + 8
	bar := Rect{in.x, y, in.w, gadH}
	fb.fill(bar, cDark)
	if n > 0 {
		fb.fill(Rect{bar.x, bar.y, bar.w * r.next / n, bar.h}, cBlue)
	}
	fb.frame(bar, cEdgeDk)
	y += gadH + 8
	if r.next > 0 && r.next <= n {
		fb.text(in.x, y, r.items[len(r.items)-1].rel, cDim)
	}
	if buttonRow(u, in, "Cancel") == 0 || r.done() {
		d.finish()
	}
}

// finish ends the run (done or cancelled): the report is saved and shown.
func (d *BatchReq) finish() {
	text := d.run.reportText()
	if err := d.run.writeReport(); err != nil {
		text += "\nThe report could not be saved: " + err.Error() + "\n"
	} else {
		text += "This report is saved as " + d.run.report + "\n"
	}
	d.report = strings.Split(strings.TrimRight(text, "\n"), "\n")
	d.top = 0
	done, skipped, _ := d.run.counts()
	a.msg(fmt.Sprintf("Animate a folder: %d animated, %d skipped; the report is in %s", done, skipped, d.run.report))
}

func (d *BatchReq) drawReport(u *UI) {
	fb := u.fb
	in := u.window(centre(1000, ui.fb.h*5/6), "Animate a folder: the report")
	listR := Rect{in.x, in.y, in.w - 18, in.h - gadH - 12}
	fb.fill(listR, cGrey)
	fb.bevel(listR, true)
	rows := max(1, (listR.h-4)/lineH)
	if w := u.takeWheel(listR); w != 0 {
		d.top += w * 3
	}
	d.top = u.scrollbar("batchreport", Rect{listR.x + listR.w, listR.y, 18, listR.h}, true, d.top, rows, len(d.report))
	old := fb.clip
	fb.clip = listR.inset(2)
	for i := d.top; i < min(len(d.report), d.top+rows); i++ {
		col := uint32(cText)
		if l := d.report[i]; strings.HasPrefix(l, "Animated") || strings.HasPrefix(l, "Skipped") || strings.HasPrefix(l, "Charcoal") {
			col = cHi
		}
		fb.text(listR.x+6, listR.y+2+(i-d.top)*lineH, d.report[i], col)
	}
	fb.clip = old
	labels := []string{"Animate more", "Close"}
	switch buttonRow(u, in, labels...) {
	case 0:
		d.run, d.report = nil, nil
		d.seed.set(strconv.FormatInt(newSeed(), 10))
	case 1:
		closeDialog()
	}
}

func (d *BatchReq) key(k string, ctrl, shift bool) bool {
	switch {
	case k == "Escape" && d.run != nil && d.report == nil:
		d.finish() // cancel: the report says what was done
	case k == "Escape":
		closeDialog()
	case (k == "Return" || k == "Keypad Enter") && d.report != nil:
		closeDialog()
	case (k == "Return" || k == "Keypad Enter") && d.run == nil:
		d.start()
	case d.report != nil && (k == "Down" || k == "Up" || k == "PageDown" || k == "PageUp"):
		step := map[string]int{"Down": 1, "Up": -1, "PageDown": 20, "PageUp": -20}[k]
		d.top = max(0, min(len(d.report)-1, d.top+step))
	default:
		return false
	}
	return true
}

// typicalWeights are the templates' weights at a level on a typical screen
// (made up: 80x25, runs of blocks and letters in a few colours), for the
// ticked list; worked out once per level.
var typicalW = map[int][]int{}

func typicalWeights(detail int) []int {
	if w, ok := typicalW[detail]; ok {
		return w
	}
	w := make([]int, nPatterns)
	withScratch(func() {
		a.Tab = a.makeTab("pc")
		a.doc = typicalScreen()
		for p := 0; p < nPatterns; p++ {
			if framesOnly(p, false) {
				continue
			}
			w[p] = a.costOf("ansi", &drawOrder{Pattern: patterns[p].id, Seed: 1, Detail: detailIDs[detail]}, "").weight()
		}
	})
	typicalW[detail] = w
	return w
}

// typicalScreen is a made-up screen of ANSI art: runs of shades, blocks and letters.
func typicalScreen() *Doc {
	d := newDoc(80, 25)
	r := rand.New(rand.NewSource(1994))
	glyphs := []int{176, 177, 178, 219, 220, 223, 'A', 'n', 's', 'i', '.', ':'}
	for y := 1; y < 23; y++ {
		for x := r.Intn(6); x < 80; {
			n := 2 + r.Intn(9)
			fg, bg, ch := r.Intn(16), r.Intn(3), glyphs[r.Intn(len(glyphs))]
			for i := 0; i < n && x < 80; i++ {
				d.set(x, y, makeCell(ch, fg, bg, 0))
				x++
			}
			x += r.Intn(5)
		}
	}
	return d
}
