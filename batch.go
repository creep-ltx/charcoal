package main

// Animate a folder: a sysop's screens (menus, logon, bulletins) all animated at
// once, for the board, without opening each. Every art file of a folder (and
// its subfolders, if asked) is read as opening it would, given one effect or a
// different one each (picked from a seed, so a run can be repeated), and
// written to a new folder with the board's pause codes (bbspause.go), the
// names kept. N variants of each screen are named the way the board picks a
// random one (randomScreens). The originals are never touched unless the
// person asks to replace them, and then each keeps a .bak.
//
// The work runs in a scratch tab, one file at a time (batchRun.step), so the
// window stays alive (the Animate a folder requester, batch_ui.go) and the
// command line (charedit --animate) shares all of it.

import (
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// batchSpec is what a run does.
type batchSpec struct {
	Inputs     []string        // folders and files
	Subfolders bool            // folders: their subfolders too
	Types      map[string]bool // the kinds to animate: "ansi", "seq", "ata", "vt52", "other" (nil: all)
	Effect     string          // a pattern id, or "random"
	Pool       []string        // random: the ids to pick from (nil: every one that can be a file's)
	NoRepeat   bool            // random: never the same effect twice in a row
	Toggles    bool            // Reverse and Flip at random too
	Seed       int64
	Every      int      // a draw order's pause every n cells (0: 40)
	Pause      int      // ... of ms (0: 40)
	Hold       int      // a frame effect's hold, ms (0: the effect's own)
	Board      string   // "" Settings' My BBS, "none", or a board id
	Copies     []string // boards that also get a copy (name-<id>.ext)
	Variants   int      // versions of each screen (1: one)
	Out        string   // the output folder ("": <folder>-animated)
	Replace    bool     // write over the originals (each kept as .bak)
	AEPath     string   // Ami-Express: the screens folder on the board, for ~SR_
	// Size (animcost.go): the detail level ("": medium), and a budget each
	// version must keep: at most MaxKB KB, at most MaxSeconds s at AtBps (0:
	// no limit), no heavier than MaxWeight ("": any, "light", "medium").
	// Effects that do not fit are played lower, then passed over.
	Detail     string
	MaxKB      int
	MaxSeconds int
	AtBps      int // 0: 14400; telnet: -1
	MaxWeight  string
}

// budgeted: the run checks each version against a budget.
func (s *batchSpec) budgeted() bool { return s.MaxKB > 0 || s.MaxSeconds > 0 || s.MaxWeight != "" }

// bps is the speed the run's times are given at.
func (s *batchSpec) bps() int {
	if s.AtBps == 0 {
		return 14400
	}
	return s.AtBps
}

// level is the run's detail level.
func (s *batchSpec) level() int {
	if d := detailByID(s.Detail); s.Detail != "" && d >= 0 {
		return d
	}
	return detailMedium
}

// fits says why a version costing c is over the budget ("" it fits).
func (s *batchSpec) fits(c animCost) string {
	switch {
	case s.MaxKB > 0 && c.size > s.MaxKB*1024:
		return fmt.Sprintf("%s, over %d KB", kbWords(c.size), s.MaxKB)
	case s.MaxSeconds > 0 && c.at(s.bps()) > s.MaxSeconds*1000:
		return fmt.Sprintf("%s at %s, over %d s", durWords(c.at(s.bps())), speedName(s.bps()), s.MaxSeconds)
	case s.MaxWeight != "" && c.weight() > weightByName(s.MaxWeight):
		return weightNames[c.weight()] + ", not " + s.MaxWeight
	}
	return ""
}

func weightByName(n string) int {
	for i, w := range weightNames {
		if strings.EqualFold(w, n) {
			return i
		}
	}
	return weightHeavy
}

// batchKinds: the kinds of file a run can animate, for the requester and the report.
var batchKinds = []struct{ id, name string }{
	{"ansi", "ANSI (.ans .asc .txt .nfo .diz)"}, {"seq", "PETSCII (.seq)"}, {"ata", "ATASCII (.ata)"},
	{"vt52", "Atari ST (.vt)"}, {"other", "other art (.bin .xb .pcb .avt ...: written as .ans / .seq)"},
}

// batchArtExt: endings that are art; other files are taken only when they hold ANSI codes.
var batchArtExt = regexp.MustCompile(`(?i)\.(ans|ansi|asc|txt|nfo|diz|ice|bin|xb|xbin|pcb|avt|idf|adf|tnd|seq|pet|ata|vt|c|petmate|prg|scr|tti|ep1|an[1-9]|as[1-9])$`)

var streamExt = map[string]string{"ansi": ".ans", "seq": ".seq", "ata": ".ata", "vt52": ".vt"}

// batchPatterns are the effects a saved file can draw (not the frames-only ones).
func batchPatterns() []int {
	var out []int
	for p := 0; p < nPatterns; p++ {
		if !framesOnly(p, false) {
			out = append(out, p)
		}
	}
	return out
}

// patternByName finds an effect by id or name ("Wiggle out", "wiggle-out", "WIGGLEOUT").
func patternByName(s string) int {
	norm := func(s string) string {
		return strings.ToLower(strings.NewReplacer(" ", "", "-", "", "_", "").Replace(s))
	}
	for i, p := range patterns {
		if norm(p.id) == norm(s) || norm(p.name) == norm(s) {
			return i
		}
	}
	return -1
}

// ---------------------------------------------------------------- random screens per board

// randomScreen is how a board shows one of several versions of a screen.
type randomScreen struct {
	max  int                                   // the most versions it takes (0: no limit)
	name func(base, ext string, i int) string  // version i (0-based) of base+ext
	stub func(n int, path, base string) []byte // the file left under the plain name (nil: version 0 is that file)
	note string
	how  string
}

// randomScreens: what each board does (~/Projects/Knowledge/bbs/screens-variants-utf8-wide.md).
var randomScreens = map[string]*randomScreen{
	// Ami-Express 5 (express.e processMciCmd "SR_"): "~<n>SR_<path>NAME|" in a
	// screen shows <path>NNN.NAME (+ .TXT or the caller's screen ending,
	// findSecurityScreen) with NNN = Rnd(n)+1 written as three digits.
	"amiexpress": {max: 999,
		name: func(base, ext string, i int) string { return fmt.Sprintf("%03d.%s%s", i+1, base, ext) },
		stub: func(n int, path, base string) []byte { return []byte(fmt.Sprintf("~%dSR_%s%s|\r\n", n, path, base)) },
		note: "001.NAME.TXT ... are the versions; NAME.TXT holds ~<n>SR_<screens folder>NAME| and shows one at random",
		how:  "verified: read in Ami-Express 5's source (express.e processMciCmd SR_, findSecurityScreen)"},
	// Mystic 1.10 (bbs_io.pas OutFile): asked for a name without an ending,
	// when name.an1 is there it rolls Random(9): 0 = .ans itself, 1-8 = .an1-.an8.
	"mystic": {max: 9,
		name: func(base, ext string, i int) string {
			if i == 0 {
				return base + ext
			}
			return base + strings.ToLower(ext[:min(3, len(ext))]) + fmt.Sprint(i)
		},
		note: "name.ans and name.an1 ... name.an8 (the last letter of the ending is the number); Mystic picks one by itself",
		how:  "verified: read in Mystic 1.10's source (bbs_io.pas OutFile, Random(9))"},
	// Synchronet (prntfile.cpp menu, random_menu): a menu name with * or ?
	// picks one of the names it matches (cut at the first dot) at random.
	"synchronet": {
		name: func(base, ext string, i int) string {
			if i == 0 {
				return base + ext
			}
			return fmt.Sprintf("%s%d%s", base, i, ext)
		},
		note: "name.ans, name1.ans, name2.ans ...: shown at random by a wildcard, bbs.menu(\"name*\") or @MENU:name*@ (the stock logon.js shows logon, logon1 ... one after another, and picks random*.* at random)",
		how:  "verified: read in Synchronet's source (prntfile.cpp random_menu, atcodes.cpp MENU, exec/logon.js)"},
}

// plainVariant names version i on a board with no random screens: NAME-1.ANS ...
func plainVariant(base, ext string, i int) string {
	if i == 0 {
		return base + ext
	}
	return fmt.Sprintf("%s-%d%s", base, i+1, ext)
}

// variantNote says how board id takes versions of a screen, for the requester and the report.
func variantNote(id string) string {
	if r := randomScreens[id]; r != nil {
		return r.note
	}
	switch id {
	case "pcboard":
		return "PCBoard has no random screens (its names choose by security level, graphics and language): the versions are numbered NAME-2 ..."
	case "tempest":
		return "Tempest's random screens are not known here: the versions are numbered NAME-2 ..."
	}
	return "No board chosen: the versions are numbered NAME-2 ... (pick a board to name them its way)"
}

// variantNames are the file names of n versions of rel (a path in the output
// folder) for board id, and the stub left under the plain name (Ami-Express).
func (s *batchSpec) variantNames(id, rel string, n int) (names []string, stub []byte) {
	if n <= 1 {
		return []string{rel}, nil // one version: the name as it is
	}
	dir, file := filepath.Split(rel)
	ext := filepath.Ext(file)
	base := strings.TrimSuffix(file, ext)
	r := randomScreens[id]
	if r != nil && r.max > 0 {
		n = min(n, r.max)
	}
	for i := 0; i < n; i++ {
		if r != nil {
			names = append(names, dir+r.name(base, ext, i))
		} else {
			names = append(names, dir+plainVariant(base, ext, i))
		}
	}
	if r != nil && r.stub != nil && n > 1 {
		path := s.AEPath
		if path == "" {
			path = "BBS:Node0/Screens/"
		}
		if !strings.HasSuffix(path, "/") && !strings.HasSuffix(path, ":") {
			path += "/"
		}
		path += filepath.ToSlash(dir)
		stub = r.stub(n, path, base)
	}
	return names, stub
}

// ---------------------------------------------------------------- the run

// batchItem is one file's outcome.
type batchItem struct {
	rel     string   // the source, from the folder
	outs    []string // what was written, from the output folder
	effects []string // the effect of each version
	before  int64
	after   int64        // all versions and copies
	orders  []*drawOrder // each version's order
	sizes   []int64      // each output's size
	times   []int        // each version's play time at the run's speed, ms (-1: a copy or a stub)
	skipped string
	note    string
	err     bool // a file could not be written
}

// batchRun is a run in progress.
type batchRun struct {
	spec    batchSpec
	board   string // the board whose codes the files get ("" none)
	roots   []string
	files   [][2]string // root, path from it ("" root: a file given by itself)
	outRoot string
	pool    []int
	rng     *rand.Rand
	last    int
	next    int
	items   []*batchItem
	started time.Time
	report  string          // the report's path, when written
	used    map[string]bool // output paths written in this run (two sources must not end as one file)
	err     string          // why the run could not start
}

// newBatchRun checks the spec and lists the files; err says why it cannot run.
func newBatchRun(s batchSpec) *batchRun {
	r := &batchRun{spec: s, last: -1, started: time.Now(), used: map[string]bool{}}
	switch s.Board {
	case "":
		r.board = prefs.MyBBS
	case "none":
	default:
		r.board = s.Board
	}
	if r.board != "" && boardByID(r.board) == nil {
		r.err = "No such board: " + r.board
		return r
	}
	if r.spec.Variants < 1 {
		r.spec.Variants = 1
	}
	if s.Detail != "" && detailByID(s.Detail) < 0 {
		r.err = "No such detail level: " + s.Detail + " (small, medium or large)"
		return r
	}
	if s.MaxWeight != "" && !strings.EqualFold(s.MaxWeight, "light") && !strings.EqualFold(s.MaxWeight, "medium") {
		r.err = "No such weight: " + s.MaxWeight + " (light or medium)"
		return r
	}
	if s.Effect == "random" {
		ids := s.Pool
		if len(ids) == 0 {
			for _, p := range batchPatterns() {
				ids = append(ids, patterns[p].id)
			}
		}
		for _, id := range ids {
			if p := patternByName(id); p >= 0 && !framesOnly(p, false) && !containsInt(r.pool, p) {
				r.pool = append(r.pool, p)
			}
		}
		if len(r.pool) == 0 {
			r.err = "No effect ticked to pick from"
			return r
		}
	} else if p := patternByName(s.Effect); p < 0 || framesOnly(p, false) {
		r.err = "No such effect for a saved file: " + s.Effect
		return r
	} else {
		r.pool = []int{p}
	}
	r.rng = rand.New(rand.NewSource(s.Seed))
	if len(s.Inputs) == 0 {
		r.err = "No folder or files given"
		return r
	}
	var dirs []string
	for _, in := range s.Inputs {
		p, err := filepath.Abs(expandHome(in))
		if err != nil {
			r.err = err.Error()
			return r
		}
		st, err := os.Stat(p)
		if err != nil {
			r.err = "Cannot read " + in + ": " + err.Error()
			return r
		}
		if st.IsDir() {
			dirs = append(dirs, p)
		}
	}
	out := s.Out
	switch {
	case s.Replace:
		out = ""
	case out == "" && len(dirs) == 1 && len(s.Inputs) == 1:
		out = strings.TrimRight(dirs[0], string(filepath.Separator)) + "-animated"
	case out == "":
		r.err = "Name an output folder (--out) for several folders or files"
		return r
	}
	if out != "" {
		o, err := filepath.Abs(expandHome(out))
		if err != nil {
			r.err = err.Error()
			return r
		}
		r.outRoot = filepath.Clean(o)
	}
	for _, in := range s.Inputs {
		p, _ := filepath.Abs(expandHome(in))
		p = filepath.Clean(p)
		if st, _ := os.Stat(p); st.IsDir() {
			if r.outRoot == p {
				r.err = "The output folder is the folder of the originals: tick \"replace the originals\" to write over them"
				return r
			}
			r.roots = append(r.roots, p)
			r.listDir(p, p)
		} else {
			if r.outRoot == filepath.Dir(p) {
				r.err = "The output folder is the folder of the originals: tick \"replace the originals\" to write over them"
				return r
			}
			r.files = append(r.files, [2]string{filepath.Dir(p), filepath.Base(p)})
		}
	}
	if len(r.files) == 0 {
		r.err = "No files there"
	}
	return r
}

func containsInt(s []int, v int) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// listDir adds the files of dir (and its subfolders when asked), sorted by name.
func (r *batchRun) listDir(root, dir string) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	sort.Slice(ents, func(i, j int) bool { return strings.ToLower(ents[i].Name()) < strings.ToLower(ents[j].Name()) })
	for _, e := range ents {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		p := filepath.Join(dir, name)
		st, err := os.Stat(p)
		if err != nil {
			continue
		}
		if st.IsDir() {
			if r.spec.Subfolders && p != r.outRoot {
				r.listDir(root, p)
			}
			continue
		}
		rel, _ := filepath.Rel(root, p)
		r.files = append(r.files, [2]string{root, rel})
	}
}

// done: every file has been through.
func (r *batchRun) done() bool { return r.err != "" || r.next >= len(r.files) }

// outPath is where a file of the output folder (rel from it) goes.
func (r *batchRun) outPath(root, rel string) string {
	if r.spec.Replace {
		return filepath.Join(root, rel)
	}
	if len(r.roots) > 1 { // several folders: each keeps its name inside the output folder
		return filepath.Join(r.outRoot, filepath.Base(root), rel)
	}
	return filepath.Join(r.outRoot, rel)
}

// step animates the next file; it runs in a scratch tab and leaves the editor as it was.
func (r *batchRun) step() {
	if r.done() {
		return
	}
	f := r.files[r.next]
	r.next++
	it := &batchItem{rel: f[1]}
	if len(r.roots) > 1 {
		it.rel = filepath.Join(filepath.Base(f[0]), f[1])
	}
	r.items = append(r.items, it)
	full := filepath.Join(f[0], f[1])
	b, err := os.ReadFile(full)
	if err != nil {
		it.skipped = "cannot be read: " + err.Error()
		return
	}
	it.before = int64(len(b))
	if why := batchNotArt(f[1], b); why != "" {
		it.skipped = why
		return
	}
	withScratch(func() { r.animate(it, f[0], f[1], full, b) })
}

// batchNotArt says why a file is surely no screen to animate, or "".
func batchNotArt(name string, b []byte) string {
	lname := strings.ToLower(name)
	switch {
	case len(b) == 0:
		return "empty"
	case strings.HasSuffix(lname, ".bak"):
		return "a backup (.bak)"
	case lname == "animate-report.txt":
		return "a report of an earlier run"
	case len(b) >= 2 && b[0] == 0xE3 && b[1] == 0x10:
		return "an Amiga icon (.info), not art"
	}
	if what := notArt(b); what != "" {
		return what + ", not art"
	}
	if strings.HasSuffix(lname, ".c") && !isMarq(b) {
		return "C source, not a screen"
	}
	if !batchArtExt.MatchString(name) && !bytes.Contains(b, []byte("\x1b[")) {
		return "not an art file (no art ending, no ANSI codes)"
	}
	return ""
}

// withScratch runs fn with an empty tab of its own as the current one; the
// editor's tab, tool, requester and message are as they were afterwards.
func withScratch(fn func()) {
	keep, tool, fset, dlg, msg, at := a.Tab, a.tool, a.fset, a.dialog, a.message, a.messageAt
	a.Tab = a.makeTab("amiga")
	defer func() {
		a.Tab, a.tool, a.fset, a.dialog, a.message, a.messageAt = keep, tool, fset, dlg, msg, at
		if a.Tab != nil {
			a.applyLook()
		}
	}()
	fn()
}

var modeNoStream = map[string]string{
	"tt": "a teletext page: a picture of the screen, no stream to draw in order",
	"zx": "a ZX Spectrum screen: a picture of the screen, no stream to draw in order",
}

// animate reads one file into the scratch tab and writes its versions.
func (r *batchRun) animate(it *batchItem, root, rel, full string, b []byte) {
	a.dialog = nil
	a.loadBytes(b, filepath.Base(full), full, "auto")
	if _, asked := a.dialog.(*FontReq); asked && !a.hasC64 && (seqExt.MatchString(rel) || isPetmate(b) || isMarq(b) || strings.HasSuffix(strings.ToLower(rel), ".prg")) {
		it.skipped = "PETSCII: the Commodore character ROMs are not here yet (open a .seq in CharEdit once to fetch them)"
		return
	}
	if a.path != full {
		it.skipped = strings.TrimSpace(a.message)
		if it.skipped == "" {
			it.skipped = "cannot be read as art"
		}
		return
	}
	if why := modeNoStream[a.mode]; why != "" {
		it.skipped = why
		return
	}
	if n := a.nFrames(); n > 1 {
		it.skipped = fmt.Sprintf("%d screens (frames), not one picture", n)
		return
	}
	if a.split.clears >= 2 || a.doc.fullest {
		it.skipped = "already an animation (it clears the screen and draws again)"
		return
	}
	fmtID := a.pageStreamFormat()
	kind := fmtID
	if !streamFormats[a.saveFmt] && a.saveFmt != "text" {
		kind = "other"
	}
	if r.spec.Types != nil && !r.spec.Types[kind] {
		it.skipped = "not one of the kinds chosen (" + kind + ")"
		return
	}
	board := boardByID(r.board)
	if board != nil && !board.shows(fmtID) {
		it.skipped = fmt.Sprintf("%s: %s has no pause codes for %s files (board None animates it at the modem's pace)",
			map[string]string{"seq": "PETSCII", "ata": "ATASCII", "vt52": "Atari ST"}[fmtID], board.name, streamExt[fmtID])
		return
	}
	if a.doc.cols*a.doc.rows > maxAnimCells {
		it.skipped = fmt.Sprintf("too large to animate (%dx%d)", a.doc.cols, a.doc.rows)
		return
	}
	if len(a.cellsInOrder(a.doc, a.paperOf(a.doc), patTop, false, false)) == 0 {
		it.skipped = "nothing drawn on it"
		return
	}
	outRel := rel
	if kind == "other" {
		outRel = strings.TrimSuffix(rel, filepath.Ext(rel)) + streamExt[fmtID]
		it.note = "written as " + streamExt[fmtID]
		if r.used[r.outPath(root, outRel)] || fileThere(r.outPath(root, outRel)) && r.spec.Replace { // name.bin and name.ans in one folder
			ext := strings.TrimPrefix(filepath.Ext(rel), ".")
			outRel = strings.TrimSuffix(rel, filepath.Ext(rel)) + "-" + ext + streamExt[fmtID]
			it.note = "written as " + filepath.Base(outRel) + " (another file has the name)"
		}
	} else if a.saveFmt == "text" {
		it.note = "plain text: now with ANSI codes"
	}
	names, stub := r.spec.variantNames(r.board, outRel, r.spec.Variants)
	if r.spec.Variants > len(names) {
		it.note = joinNote(it.note, fmt.Sprintf("%s takes %d versions at most", boardName(r.board), len(names)))
	}
	write := func(rel string, data []byte, ms int) bool {
		p := r.outPath(root, rel)
		err := os.MkdirAll(filepath.Dir(p), 0o755)
		if err == nil && r.spec.Replace {
			err = keepBackup(p, data)
		}
		if err == nil {
			err = writeAtomic(p, data)
		}
		if err != nil {
			it.note = joinNote(it.note, "not written: "+err.Error())
			it.err = true
			return false
		}
		it.outs = append(it.outs, rel)
		it.after += int64(len(data))
		it.sizes = append(it.sizes, int64(len(data)))
		it.times = append(it.times, ms)
		r.used[p] = true
		return true
	}
	type copyOf struct {
		b     *bbsBoard
		names []string
		stub  []byte
	}
	var copies []copyOf
	for _, id := range r.spec.Copies {
		if cb := boardByID(id); cb != nil && cb.shows(fmtID) && id != r.board {
			cn, cs := r.spec.variantNames(id, outRel, r.spec.Variants)
			copies = append(copies, copyOf{cb, cn, cs})
		}
	}
	for i, name := range names {
		var o *drawOrder
		var cost animCost
		if r.spec.budgeted() {
			var why string
			if o, cost, why = r.pickFitting(fmtID); o == nil {
				if len(it.outs) == 0 {
					it.skipped = why
				} else {
					it.note = joinNote(it.note, fmt.Sprintf("only %d versions: %s", i, why))
				}
				return
			}
		} else if o = r.pick(); o != nil {
			cost = a.costOf(fmtID, o, r.board)
		}
		if o == nil {
			it.skipped = "none of the chosen effects works on this page (" + a.fadeRefused() + ")"
			return
		}
		a.order = o
		data, err := a.encodeFor(fmtID, r.board, nil)
		if err != nil {
			it.skipped = err.Error()
			return
		}
		desc := o.String()
		if isEffect(o.pat()) {
			desc += fmt.Sprintf(", seed %d", o.Seed)
		}
		if !write(name, data, cost.at(r.spec.bps())) {
			continue
		}
		it.effects = append(it.effects, desc)
		it.orders = append(it.orders, o)
		for _, c := range copies { // the same version with another board's codes, named its way
			if i >= len(c.names) {
				continue
			}
			if data, err := a.encodeFor(fmtID, c.b.id, nil); err == nil && write(copyPath(c.names[i], c.b.id), data, -1) {
				it.effects = append(it.effects, "the same, with "+c.b.name+"'s codes")
			}
		}
	}
	if stub != nil && write(outRel, stub, -1) {
		it.effects = append(it.effects, fmt.Sprintf("shows one of the %d at random: %s", len(names), strings.TrimSpace(string(stub))))
	}
	for _, c := range copies {
		if c.stub != nil && write(copyPath(outRel, c.b.id), c.stub, -1) {
			it.effects = append(it.effects, c.b.name+": shows one at random (rename to the plain names on the board)")
		}
	}
}

func joinNote(a, b string) string {
	if a == "" {
		return b
	}
	return a + "; " + b
}

// pick is the next effect: the chosen one, or one from the pool at random
// (never the last one again when asked), with its own seed and toggles.
func (r *batchRun) pick() *drawOrder {
	cand := []int{}
	for _, p := range r.pool {
		if p == patFadeIn && a.fadeRefused() != "" {
			continue
		}
		cand = append(cand, p)
	}
	if len(cand) == 0 {
		return nil
	}
	if r.spec.NoRepeat && len(cand) > 1 {
		var c2 []int
		for _, p := range cand {
			if p != r.last {
				c2 = append(c2, p)
			}
		}
		cand = c2
	}
	p := cand[r.rng.Intn(len(cand))]
	r.last = p
	o := &drawOrder{Pattern: patterns[p].id, BBS: "none", Detail: detailIDs[r.spec.level()]}
	if r.board != "" {
		o.BBS = r.board
	}
	if r.spec.Toggles && hasToggles(p) {
		o.Reverse, o.Flip = r.rng.Intn(2) == 1, r.rng.Intn(2) == 1
	}
	if isEffect(p) {
		o.Seed = 1 + r.rng.Int63n(1<<31-1)
		if r.spec.Hold > 0 {
			o.Hold = r.spec.Hold
		}
	} else {
		o.Every, o.Pause = r.spec.Every, r.spec.Pause
	}
	return o
}

// pickFitting is the next effect that keeps the budget on this page: the
// candidates in a random order (the chosen one alone when not Random), each
// at the run's level, then lower ones, the first that fits taken. why says
// what the smallest would have cost when none fits.
func (r *batchRun) pickFitting(fmtID string) (*drawOrder, animCost, string) {
	cand := []int{}
	for _, p := range r.pool {
		if p == patFadeIn && a.fadeRefused() != "" {
			continue
		}
		if r.spec.NoRepeat && len(r.pool) > 1 && p == r.last {
			continue
		}
		cand = append(cand, p)
	}
	if len(cand) == 0 {
		return nil, animCost{}, "none of the chosen effects works on this page (" + a.fadeRefused() + ")"
	}
	r.rng.Shuffle(len(cand), func(i, j int) { cand[i], cand[j] = cand[j], cand[i] })
	best, bestWhy := animCost{}, ""
	for _, p := range cand {
		o := &drawOrder{Pattern: patterns[p].id, BBS: "none"}
		if r.board != "" {
			o.BBS = r.board
		}
		if r.spec.Toggles && hasToggles(p) {
			o.Reverse, o.Flip = r.rng.Intn(2) == 1, r.rng.Intn(2) == 1
		}
		if isEffect(p) {
			o.Seed = 1 + r.rng.Int63n(1<<31-1)
			if r.spec.Hold > 0 {
				o.Hold = r.spec.Hold
			}
		} else {
			o.Every, o.Pause = r.spec.Every, r.spec.Pause
		}
		for det := r.spec.level(); det < nDetails; det++ { // Large, Medium, Small: lower each time
			o.Detail = detailIDs[det]
			c := a.costOf(fmtID, o, r.board)
			why := r.spec.fits(c)
			if why == "" {
				r.last = p
				return o, c, ""
			}
			if bestWhy == "" || c.size < best.size {
				best, bestWhy = c, fmt.Sprintf("nothing fits the budget (the smallest, %s: %s)", o, why)
			}
		}
	}
	return nil, animCost{}, bestWhy
}

// ---------------------------------------------------------------- the report

// counts: files animated, skipped, not written.
func (r *batchRun) counts() (done, skipped, failed int) {
	for _, it := range r.items {
		switch {
		case it.skipped != "":
			skipped++
		case it.err:
			failed++
		default:
			done++
		}
	}
	return
}

// effectWords says what the run's effect choice was.
func (r *batchRun) effectWords() string {
	s := r.spec
	var w string
	if s.Effect == "random" {
		w = fmt.Sprintf("a different one for each file, from %d", len(r.pool))
		if len(s.Pool) == 0 {
			w += " (all)"
		}
		if s.NoRepeat {
			w += ", never the same twice in a row"
		}
	} else {
		w = patterns[r.pool[0]].name
	}
	if s.Toggles {
		w += ", Reverse and Flip at random"
	}
	return w
}

// reportText is the run's report.
func (r *batchRun) reportText() string {
	var sb strings.Builder
	s := r.spec
	fmt.Fprintf(&sb, "Charcoal %s: Animate a folder, %s\n\n", version, r.started.Format("2006-01-02 15:04"))
	from := strings.Join(r.roots, ", ")
	if len(r.roots) == 0 {
		from = fmt.Sprintf("%d files", len(r.files))
	}
	if s.Subfolders {
		from += " (and subfolders)"
	}
	fmt.Fprintf(&sb, "From:     %s\n", from)
	if s.Replace {
		fmt.Fprintf(&sb, "To:       the same files (the originals kept as .bak)\n")
	} else {
		fmt.Fprintf(&sb, "To:       %s\n", r.outRoot)
	}
	fmt.Fprintf(&sb, "Effect:   %s\n", r.effectWords())
	fmt.Fprintf(&sb, "Seed:     %d (the same seed picks the same again)\n", s.Seed)
	fmt.Fprintf(&sb, "Detail:   %s%s\n", detailNames[s.level()], map[bool]string{true: " (lower where the budget asks)"}[s.budgeted()])
	if s.budgeted() {
		var b []string
		if s.MaxKB > 0 {
			b = append(b, fmt.Sprintf("at most %d KB", s.MaxKB))
		}
		if s.MaxSeconds > 0 {
			b = append(b, fmt.Sprintf("at most %d s at %s", s.MaxSeconds, speedName(s.bps())))
		}
		if s.MaxWeight == "light" {
			b = append(b, "light effects only")
		} else if s.MaxWeight != "" {
			b = append(b, "light and medium effects only")
		}
		fmt.Fprintf(&sb, "Budget:   %s, each screen\n", strings.Join(b, ", "))
	}
	fmt.Fprintf(&sb, "Times:    at %s (bytes x 10 / bps, plus the board's pauses)\n", speedName(s.bps()))
	if b := boardByID(r.board); b != nil {
		every, ms := (&drawOrder{Every: s.Every, Pause: s.Pause}).pacing()
		hold := "each effect's own hold"
		if s.Hold > 0 {
			hold = fmt.Sprintf("%d ms a frame", s.Hold)
		}
		fmt.Fprintf(&sb, "Board:    %s pauses: %d ms every %d cells, %s\n          (%s)\n", b.name, ms, every, hold, b.note)
	} else {
		fmt.Fprintf(&sb, "Board:    none (the files play at the modem's pace)\n")
	}
	if len(s.Copies) > 0 {
		fmt.Fprintf(&sb, "Copies:   %s (name-<board>.ext)\n", strings.Join(s.Copies, ", "))
	}
	if s.Variants > 1 {
		fmt.Fprintf(&sb, "Versions: %d of each screen\n", s.Variants)
		for _, l := range wrap(variantNote(r.board), 90) {
			fmt.Fprintf(&sb, "          %s\n", l)
		}
	}
	done, skipped, failed := r.counts()
	var before, after int64
	fmt.Fprintf(&sb, "\nAnimated (%d):\n", done+failed)
	for _, it := range r.items {
		if it.skipped != "" {
			continue
		}
		before += it.before
		after += it.after
		fmt.Fprintf(&sb, "  %s  %s -> %s bytes\n", it.rel, sizeWords(it.before), sizeWords(it.after))
		for i, o := range it.outs {
			e := ""
			if i < len(it.effects) {
				e = it.effects[i]
			}
			cost := ""
			if i < len(it.sizes) {
				cost = kbWords(int(it.sizes[i]))
				if it.times[i] >= 0 {
					cost += ", " + durWords(it.times[i])
				}
			}
			fmt.Fprintf(&sb, "      %-28s %-20s %s\n", o, cost, e)
		}
		if it.note != "" {
			fmt.Fprintf(&sb, "      (%s)\n", it.note)
		}
	}
	fmt.Fprintf(&sb, "\nSkipped (%d):\n", skipped)
	for _, it := range r.items {
		if it.skipped != "" {
			fmt.Fprintf(&sb, "  %s: %s\n", it.rel, it.skipped)
		}
	}
	if r.next < len(r.files) {
		fmt.Fprintf(&sb, "\nCancelled: %d files were not looked at.\n", len(r.files)-r.next)
	}
	fmt.Fprintf(&sb, "\n%d animated, %d skipped", done, skipped)
	if failed > 0 {
		fmt.Fprintf(&sb, ", %d not written", failed)
	}
	fmt.Fprintf(&sb, "; %s -> %s bytes.\n", sizeWords(before), sizeWords(after))
	fmt.Fprintf(&sb, "The same run again: %s\n", r.commandLine())
	return sb.String()
}

func sizeWords(n int64) string {
	s := fmt.Sprint(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

// commandLine is the charedit --animate line that repeats the run.
func (r *batchRun) commandLine() string {
	s := r.spec
	q := func(p string) string {
		if strings.ContainsAny(p, " '\"$`\\!*?()&;|<>") {
			return "'" + strings.ReplaceAll(p, "'", `'\''`) + "'"
		}
		return p
	}
	parts := []string{"charedit --animate"}
	for _, in := range s.Inputs {
		if p, err := filepath.Abs(expandHome(in)); err == nil {
			in = p
		}
		parts = append(parts, q(in))
	}
	if s.Subfolders {
		parts = append(parts, "--subfolders")
	}
	parts = append(parts, "--effect", s.Effect)
	if s.Effect == "random" && len(s.Pool) > 0 {
		parts = append(parts, "--from", strings.Join(s.Pool, ","))
	}
	if s.Effect == "random" && !s.NoRepeat {
		parts = append(parts, "--repeats")
	}
	if s.Toggles {
		parts = append(parts, "--toggles")
	}
	parts = append(parts, "--seed", fmt.Sprint(s.Seed), "--detail", detailIDs[s.level()])
	if s.MaxKB > 0 {
		parts = append(parts, "--max-kb", fmt.Sprint(s.MaxKB))
	}
	if s.MaxSeconds > 0 {
		parts = append(parts, "--max-seconds", fmt.Sprint(s.MaxSeconds))
	}
	if s.MaxSeconds > 0 || s.AtBps != 0 {
		parts = append(parts, "--at", speedLabel(s.bps()))
	}
	if s.MaxWeight == "light" {
		parts = append(parts, "--light")
	} else if s.MaxWeight != "" {
		parts = append(parts, "--weight", s.MaxWeight)
	}
	b := r.board
	if b == "" {
		b = "none"
	}
	parts = append(parts, "--bbs", b)
	if len(s.Copies) > 0 {
		parts = append(parts, "--copies", strings.Join(s.Copies, ","))
	}
	if s.Every > 0 {
		parts = append(parts, "--every", fmt.Sprint(s.Every))
	}
	if s.Pause > 0 {
		parts = append(parts, "--pause", fmt.Sprint(s.Pause))
	}
	if s.Hold > 0 {
		parts = append(parts, "--hold", fmt.Sprint(s.Hold))
	}
	if s.Variants > 1 {
		parts = append(parts, "--variants", fmt.Sprint(s.Variants))
		if r.board == "amiexpress" && s.AEPath != "" {
			parts = append(parts, "--ae-path", q(s.AEPath))
		}
	}
	if s.Types != nil {
		var ks []string
		for _, k := range batchKinds {
			if s.Types[k.id] {
				ks = append(ks, k.id)
			}
		}
		parts = append(parts, "--types", strings.Join(ks, ","))
	}
	if s.Replace {
		parts = append(parts, "--replace")
	} else {
		parts = append(parts, "--out", q(r.outRoot))
	}
	return strings.Join(parts, " ")
}

// writeReport saves the report as animate-report.txt in the output folder.
func (r *batchRun) writeReport() error {
	dir := r.outRoot
	if r.spec.Replace {
		if len(r.roots) > 0 {
			dir = r.roots[0]
		} else {
			dir = r.files[0][0]
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	r.report = filepath.Join(dir, "animate-report.txt")
	return writeAtomic(r.report, []byte(r.reportText()))
}

// ---------------------------------------------------------------- the command line

const animateUsage = `usage: charedit --animate FOLDER|FILE... [options]
Animates every art file (.ans .asc .txt .seq .ata .vt ...) and writes them to a new
folder, the names kept; the originals are never changed unless --replace is given.
  --effect NAME|random  an effect (top, clock, matrix, "wiggle out" ...; --effects lists
                        them) or random: a different one for each file (the default)
  --from LIST           random: pick only from these effects (comma separated)
  --repeats             random: the same effect may come twice in a row
  --toggles             Reverse and Flip at random too
  --seed N              the random choices (the same seed gives the same files)
  --detail LEVEL        small, medium (the default) or large: Small is for modems, Large the full look
  --max-kb N            each version at most N KB (an effect that does not fit is played at a
                        lower detail, then passed over; a screen nothing fits is skipped)
  --max-seconds N       each version plays in at most N seconds at --at's speed
  --at BPS              the speed for --max-seconds and the report's times: 2400 ... 57600,
                        or telnet (only the pauses count); 14400 when not given
  --light               light effects only (at most 1.5x the screen's plain file)
  --weight light|medium no heavier than this
  --bbs ID              amiexpress, mystic, pcboard, synchronet, tempest or none: the
                        board whose pause codes the files get (default: My BBS in Settings)
  --copies LIST         also a copy for these boards beside each file (name-<board>.ext)
  --every N --pause MS  a draw order pauses MS ms every N cells (40, 40)
  --hold MS             a frame effect holds each frame MS ms (its own)
  --variants N          N versions of each screen, named the way the board picks one at random
  --ae-path PATH        Ami-Express: the screens folder on the board (BBS:Node0/Screens/)
  --types LIST          only these kinds: ansi, seq, ata, vt52, other
  --subfolders          folders' subfolders too
  --out DIR             the output folder (default FOLDER-animated)
  --replace             write over the originals, each kept as .bak
The report is printed and saved as animate-report.txt in the output folder.
Exit status: 0 done, 1 a file could not be written or nothing was animated, 2 a bad command line.`

// animateCLI runs charedit --animate (args after it); it returns the exit status.
func animateCLI(args []string, stdout, stderr *os.File) int {
	s := batchSpec{Effect: "random", NoRepeat: true, Seed: newSeed()}
	bad := func(f string, v ...any) int {
		fmt.Fprintf(stderr, "charedit --animate: "+f+"\n", v...)
		fmt.Fprintln(stderr, "charedit --help shows the options")
		return 2
	}
	list := func(v string) []string {
		var out []string
		for _, x := range strings.Split(v, ",") {
			if x = strings.TrimSpace(x); x != "" {
				out = append(out, x)
			}
		}
		return out
	}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		val := func() (string, bool) {
			if i+1 >= len(args) {
				return "", false
			}
			i++
			return args[i], true
		}
		num := func(lo int) (int, bool) {
			v, ok := val()
			n := 0
			if _, err := fmt.Sscan(v, &n); !ok || err != nil || n < lo {
				return 0, false
			}
			return n, true
		}
		var ok bool
		switch arg {
		case "-h", "--help":
			fmt.Fprintln(stdout, animateUsage)
			return 0
		case "--effects":
			for _, p := range batchPatterns() {
				fmt.Fprintf(stdout, "%-13s %s\n", patterns[p].id, patterns[p].help)
			}
			return 0
		case "--effect":
			var v string
			if v, ok = val(); ok {
				if strings.EqualFold(v, "random") {
					s.Effect = "random"
				} else if p := patternByName(v); p >= 0 && !framesOnly(p, false) {
					s.Effect = patterns[p].id
				} else {
					return bad("no effect called %q for a saved file (--effects lists them)", v)
				}
			}
		case "--from":
			var v string
			if v, ok = val(); ok {
				for _, x := range list(v) {
					p := patternByName(x)
					if p < 0 || framesOnly(p, false) {
						return bad("no effect called %q for a saved file", x)
					}
					s.Pool = append(s.Pool, patterns[p].id)
				}
			}
		case "--repeats":
			s.NoRepeat, ok = false, true
		case "--toggles":
			s.Toggles, ok = true, true
		case "--seed":
			var v string
			if v, ok = val(); ok {
				if _, err := fmt.Sscan(v, &s.Seed); err != nil {
					return bad("--seed wants a number")
				}
			}
		case "--bbs":
			var v string
			if v, ok = val(); ok {
				v = strings.ToLower(v)
				if v != "none" && boardByID(v) == nil {
					return bad("no board %q (amiexpress, mystic, pcboard, synchronet, tempest or none)", v)
				}
				s.Board = v
			}
		case "--copies":
			var v string
			if v, ok = val(); ok {
				for _, x := range list(strings.ToLower(v)) {
					if boardByID(x) == nil {
						return bad("no board %q", x)
					}
					s.Copies = append(s.Copies, x)
				}
			}
		case "--every":
			s.Every, ok = num(1)
		case "--pause":
			s.Pause, ok = num(20)
		case "--hold":
			s.Hold, ok = num(20)
		case "--variants":
			s.Variants, ok = num(1)
		case "--ae-path":
			s.AEPath, ok = val()
		case "--types":
			var v string
			if v, ok = val(); ok {
				s.Types = map[string]bool{}
				for _, x := range list(strings.ToLower(v)) {
					known := false
					for _, k := range batchKinds {
						known = known || k.id == x
					}
					if !known {
						return bad("no kind %q (ansi, seq, ata, vt52, other)", x)
					}
					s.Types[x] = true
				}
			}
		case "--detail":
			var v string
			if v, ok = val(); ok {
				if v == "" || detailByID(v) < 0 {
					return bad("--detail is small, medium or large")
				}
				s.Detail = detailIDs[detailByID(v)]
			}
		case "--max-kb":
			s.MaxKB, ok = num(1)
		case "--max-seconds":
			s.MaxSeconds, ok = num(1)
		case "--at":
			var v string
			if v, ok = val(); ok {
				if strings.EqualFold(v, "telnet") {
					s.AtBps = telnet
				} else if _, err := fmt.Sscan(v, &s.AtBps); err != nil || s.AtBps < 300 {
					return bad("--at wants a speed in bits per second (2400, 14400 ...) or telnet")
				}
			}
		case "--light":
			s.MaxWeight, ok = "light", true
		case "--weight":
			var v string
			if v, ok = val(); ok {
				v = strings.ToLower(v)
				if v != "light" && v != "medium" {
					return bad("--weight is light or medium")
				}
				s.MaxWeight = v
			}
		case "--subfolders":
			s.Subfolders, ok = true, true
		case "--out":
			s.Out, ok = val()
		case "--replace":
			s.Replace, ok = true, true
		default:
			if strings.HasPrefix(arg, "-") {
				return bad("unknown option %s", arg)
			}
			s.Inputs, ok = append(s.Inputs, arg), true
		}
		if !ok {
			return bad("%s wants a value", arg)
		}
	}
	if len(s.Inputs) == 0 {
		return bad("no folder or files given")
	}
	if s.Replace && s.Out != "" {
		return bad("--replace and --out together: the files go either over the originals or to --out")
	}
	r := newBatchRun(s)
	if r.err != "" {
		fmt.Fprintln(stderr, "charedit --animate: "+r.err)
		return 2
	}
	for !r.done() {
		r.step()
	}
	if err := r.writeReport(); err != nil {
		fmt.Fprintln(stderr, "charedit --animate: the report could not be saved: "+err.Error())
	}
	fmt.Fprint(stdout, r.reportText())
	if r.report != "" {
		fmt.Fprintln(stdout, "Report saved as "+r.report)
	}
	done, _, failed := r.counts()
	if failed > 0 || done == 0 {
		return 1
	}
	return 0
}

// headlessApp sets up the editor's state with no window, as main does.
func headlessApp() {
	a = &App{tool: "draw", fg: 7, brush: 0x2F, charHover: -1, layout: "one", defZoom: 2,
		defFont: map[string]string{}, defPalette: map[string]string{}, baud: 14400}
	a.hasC64 = loadC64Fonts()
	loadSTFonts()
	loadPrefs()
	a.focusTab(a.newTab("amiga"))
}

func fileThere(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
