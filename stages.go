package main

// Work stages: the page's history kept thin and small, so that a compo's work
// stages (Revision 10, Deadline 6, Zoo at least 10) can be written at the end
// from the work itself. Design: docs/workstages-design.md.
//
// Tab.steps is the page's depth (the steps that led to it: pushSnap adds one,
// undo and redo set the step's own). Tab.stages keeps a picture at every depth
// that is a multiple of `every`; past stageMax pictures or stageBytes, `every`
// doubles and the pictures off its multiples go, so the log stays spread evenly
// over the whole work. Saving the page in its own format writes the log to the
// data folder; opening the same file (same bytes) takes it back.

import (
	"bytes"
	"compress/flate"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"time"
)

const (
	stageMax   = 64       // pictures a log keeps at most (it keeps 32-64 once it is thinned)
	stageBytes = 24 << 20 // their packed cells at most
	stageVer   = 1
)

// stagePic is one picture of the work: the cells packed, and what the Doc
// needs to be a page again.
type stagePic struct {
	Step   int       `json:"step"`
	At     time.Time `json:"at"`
	Cols   int       `json:"cols"`
	Rows   int       `json:"rows"`
	Bg     int       `json:"bg"`
	Border int       `json:"border"`
	Ink    int       `json:"ink"`
	Glyph0 bool      `json:"glyph0,omitempty"`
	Uni    bool      `json:"uni,omitempty"`
	Pairs  [][3]int  `json:"pairs,omitempty"` // a Unicode page's styles past 256, numbered from 256 in this picture
	Cells  []byte    `json:"cells"`           // flate of the cells, 4 bytes each, little-endian
}

// stageLog is a page's thinned history.
type stageLog struct {
	Ver   int        `json:"version"`
	Mode  string     `json:"mode"`
	Every int        `json:"every"`
	Depth int        `json:"depth"`          // the page's depth when the log was written
	Path  string     `json:"path,omitempty"` // the file it belongs to
	SHA1  string     `json:"sha1,omitempty"` // of the file's bytes as written
	Pics  []stagePic `json:"pictures"`
}

func (l *stageLog) bytes() int {
	n := 0
	for _, p := range l.Pics {
		n += len(p.Cells)
	}
	return n
}

// packDoc makes a picture of d at depth step.
func packDoc(d *Doc, step int) stagePic {
	p := stagePic{Step: step, At: time.Now(), Cols: d.cols, Rows: d.rows, Bg: d.bg, Border: d.border, Ink: d.ink, Glyph0: d.glyph0, Uni: d.uni}
	n := d.cols * d.rows
	raw := make([]byte, 0, 4*n)
	local := map[int]int{}
	for _, v := range d.cells[:n] {
		if d.uni {
			if pr := uniPair(v); pr >= 256 {
				l, ok := local[pr]
				if !ok {
					fg, bg, fl := styleParts(pr)
					l = 256 + len(p.Pairs)
					local[pr] = l
					p.Pairs = append(p.Pairs, [3]int{fg, bg, int(fl)})
				}
				v = uniWithPair(v, l)
			}
		}
		raw = binary.LittleEndian.AppendUint32(raw, v)
	}
	var buf bytes.Buffer
	w, _ := flate.NewWriter(&buf, flate.BestSpeed)
	w.Write(raw)
	w.Close()
	p.Cells = buf.Bytes()
	return p
}

// doc unpacks a picture; nil when it is damaged.
func (p stagePic) doc() *Doc {
	if p.Cols < 1 || p.Rows < 1 || p.Cols > 1000 || p.Rows > maxRowsFor(p.Cols) || p.Cols*p.Rows > maxPageCells {
		return nil
	}
	n := p.Cols * p.Rows
	raw, err := io.ReadAll(io.LimitReader(flate.NewReader(bytes.NewReader(p.Cells)), int64(4*n)+1))
	if err != nil || len(raw) != 4*n {
		return nil
	}
	d := newDoc(p.Cols, p.Rows)
	for i := range d.cells[:n] {
		d.cells[i] = binary.LittleEndian.Uint32(raw[4*i:])
	}
	d.bg, d.border, d.ink, d.glyph0 = p.Bg, p.Border, p.Ink, p.Glyph0
	if p.Uni {
		uniRemapPairs([]*Doc{d}, p.Pairs) // the picture's own table: Pairs[0] is its style 256
	}
	return d
}

// recordStage is called by pushSnap with the page before the step (depth s.step).
func (t *Tab) recordStage(s Snap) {
	if s.doc == nil {
		return
	}
	l := t.stages
	if l == nil || l.Mode != t.mode {
		// A new log; also when the page type changed (a conversion): its pictures
		// would be drawn in another page's look.
		l = &stageLog{Ver: stageVer, Mode: t.mode, Every: 1}
		t.stages = l
	}
	// Pictures deeper than this were on a branch that undo left.
	l.Pics = slices.DeleteFunc(l.Pics, func(p stagePic) bool { return p.Step > s.step })
	if s.step%l.Every != 0 || (len(l.Pics) > 0 && l.Pics[len(l.Pics)-1].Step == s.step) {
		return
	}
	if s.cur != t.cur && t.frames != nil {
		return // a step of another frame (undo's jump): not the picture being drawn
	}
	d := s.doc
	switch { // the layers put together (layers.go): stages are what was seen
	case s.layers != nil:
		d = flatten(s.layers, s.lay, s.doc)
	case t.layers != nil && !s.lwhole && s.lay == t.lay:
		d = flatten(t.layers, t.lay, s.doc)
	}
	l.Pics = append(l.Pics, packDoc(d, s.step))
	for len(l.Pics) > stageMax || (l.bytes() > stageBytes && len(l.Pics) > 2) {
		l.Every *= 2
		l.Pics = slices.DeleteFunc(l.Pics, func(p stagePic) bool { return p.Step%l.Every != 0 })
	}
}

// ---------------------------------------------------------------- kept with the file

func stagesDir() string { return filepath.Join(appDataDir(), "stages") }

func stagesFile(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	h := sha1.Sum([]byte(path))
	return filepath.Join(stagesDir(), hex.EncodeToString(h[:])+".json")
}

func sha1Hex(b []byte) string {
	h := sha1.Sum(b)
	return hex.EncodeToString(h[:])
}

// writeStages keeps the tab's log for its file, which now holds data.
func writeStages(t *Tab, path string, data []byte) error {
	if t.stages == nil || t.stages.Mode != t.mode {
		os.Remove(stagesFile(path)) // an old log of another work under this name
		return nil
	}
	if err := os.MkdirAll(stagesDir(), 0o700); err != nil {
		return err
	}
	l := *t.stages
	l.Depth, l.Path, l.SHA1 = t.steps, path, sha1Hex(data)
	js, err := json.Marshal(&l)
	if err != nil {
		return err
	}
	return writeAtomic(stagesFile(path), js)
}

// readStageLog reads a log file; it is checked so that a damaged one is no log.
func readStageLog(p string) (*stageLog, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	var l stageLog
	if err := json.Unmarshal(b, &l); err != nil {
		return nil, err
	}
	if l.Ver != stageVer || l.Every < 1 || l.Depth < 0 || len(l.Pics) > 4*stageMax {
		return nil, errors.New("not a stage log")
	}
	for i, pc := range l.Pics {
		if pc.Step < 0 || pc.Step > l.Depth || (i > 0 && pc.Step <= l.Pics[i-1].Step) {
			return nil, errors.New("stage log out of order")
		}
	}
	return &l, nil
}

// restoreStages takes back the log of the file just opened (bytes b), if it
// was written for exactly these bytes and this page type.
func (a *App) restoreStages(path string, b []byte) {
	l, err := readStageLog(stagesFile(path))
	if err != nil || l.SHA1 != sha1Hex(b) || l.Mode != a.mode {
		return
	}
	a.stages, a.steps = l, l.Depth
}

// ---------------------------------------------------------------- picking the stages

// stageCand is a picture the stages can be taken from.
type stageCand struct {
	step int
	pic  *stagePic // packed (from the log), or
	doc  *Doc      // a page in memory (the undo stack, the page now)
	lays []Layer   // the page's layers, doc being layer li (nil: doc is the page)
	li   int
}

func (c stageCand) page() *Doc {
	if c.doc != nil && c.lays != nil {
		return flatten(c.lays, c.li, c.doc)
	}
	if c.doc != nil {
		return c.doc
	}
	return c.pic.doc()
}

// stageCands are all pictures of the work in order of depth, one per depth:
// the log's, the undo stack's (same page type, the frame being drawn) and the page now.
func (a *App) stageCands() []stageCand {
	by := map[int]stageCand{}
	if l := a.stages; l != nil && l.Mode == a.mode {
		for i := range l.Pics {
			if l.Pics[i].Step <= a.steps {
				by[l.Pics[i].Step] = stageCand{step: l.Pics[i].Step, pic: &l.Pics[i]}
			}
		}
	}
	for _, s := range a.undo {
		if s.doc != nil && s.mode == a.mode && !s.whole && !s.lwhole && (a.frames == nil || s.cur == a.cur) && (a.layers == nil || s.lay == a.lay) && s.step <= a.steps {
			by[s.step] = stageCand{step: s.step, doc: s.doc, lays: a.layers, li: a.lay} // the other layers as they are now
		}
	}
	now := a.seen()
	if a.playing != nil {
		now = a.playing.saved.doc
	}
	by[a.steps] = stageCand{step: a.steps, doc: now}
	var out []stageCand
	for _, c := range by {
		out = append(out, c)
	}
	slices.SortFunc(out, func(x, y stageCand) int { return x.step - y.step })
	return out
}

// pickStages chooses n of the candidates spread evenly over the work: stage k
// wants depth k*depth/n, the nearest deeper than the stage before; the last is
// the page now. Fewer candidates than n give fewer stages (never copies).
// The empty start (depth 0) is a stage only when nothing else is left.
func pickStages(c []stageCand, n int) []stageCand {
	if len(c) == 0 || n < 1 {
		return nil
	}
	last := c[len(c)-1]
	if len(c) > 1 && c[0].step == 0 {
		c = c[1:] // the page before any work
	}
	if n >= len(c) {
		return slices.Clone(c)
	}
	depth := last.step
	var out []stageCand
	from := 0 // the first candidate still free
	for k := 1; k <= n; k++ {
		want := float64(k) * float64(depth) / float64(n)
		// Leave enough candidates for the stages still to come.
		hi := len(c) - (n - k) - 1
		best := from
		for i := from; i <= hi; i++ {
			if absf(float64(c[i].step)-want) < absf(float64(c[best].step)-want) {
				best = i
			}
		}
		out = append(out, c[best])
		from = best + 1
	}
	return out
}

func absf(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

// ---------------------------------------------------------------- writing them

// stageNames are the files of n stages: base-stage01.png ... (two digits at least).
func stageName(base string, k, n int, ext string) string {
	w := max(2, len(fmt.Sprint(n)))
	return fmt.Sprintf("%s-stage%0*d%s", base, w, k, ext)
}

// encodeStage writes picture d as fmtID in the page's look now: a copy of the
// tab with d as its only picture, nothing kept from its file.
func (a *App) encodeStage(d *Doc, fmtID string) ([]byte, error) {
	keep, msg, at := a.Tab, a.message, a.messageAt
	t := *a.Tab
	t.doc, t.frames, t.cur, t.order, t.playing = d, nil, 0, nil, nil
	t.layers, t.lay, t.seeing = nil, 0, false // d is the picture: the layers put together already
	t.source, t.srcKind, t.igs, t.undo, t.redo = nil, "", nil, nil, nil
	t.gen, t.savedGen = -1, -2 // never "unchanged"
	a.Tab = &t
	defer func() { a.Tab, a.message, a.messageAt = keep, msg, at }()
	return a.encodeFor(fmtID, "", nil)
}

// exportStages writes n stages into dir as base-stageNN.png (zoom 1 or 2) and,
// with native, in the page's own format too. It returns the files written.
func (a *App) exportStages(dir, base string, n, zoom int, native string) ([]string, error) {
	a.endPlay()
	a.keepCase()
	picked := pickStages(a.stageCands(), n)
	if len(picked) == 0 {
		return nil, errors.New("nothing drawn yet")
	}
	pngFmt := map[bool]string{true: "png2", false: "png"}[zoom == 2]
	var files []string
	for k, c := range picked {
		d := c.page()
		if d == nil {
			return files, fmt.Errorf("stage %d: its picture in the log is damaged", k+1)
		}
		jobs := []string{pngFmt}
		if native != "" && native != "png" && native != "png2" {
			jobs = append(jobs, native)
		}
		for _, f := range jobs {
			out, err := a.encodeStage(d, f)
			if err != nil {
				return files, fmt.Errorf("stage %d: %v", k+1, err)
			}
			p := filepath.Join(dir, stageName(base, k+1, len(picked), formatExt(f)))
			if err := writeAtomic(p, out); err != nil {
				return files, err
			}
			files = append(files, p)
		}
	}
	return files, nil
}

// formatExt is a format's file ending.
func formatExt(id string) string {
	for _, f := range formats {
		if f.id == id {
			return f.ext
		}
	}
	return "." + id
}

// writeAutosaveStages keeps the log beside an autosave (none: no file).
func writeAutosaveStages(t *Tab, p string) error {
	if t.stages == nil || t.stages.Mode != t.mode {
		os.Remove(p)
		return nil
	}
	l := *t.stages
	l.Depth, l.Path, l.SHA1 = t.steps, t.path, ""
	js, err := json.Marshal(&l)
	if err != nil {
		return err
	}
	return writeAtomic(p, js)
}

// readAutosaveStages is an autosave's log and the page's depth (nil, 0: none).
func readAutosaveStages(p, mode string) (*stageLog, int) {
	l, err := readStageLog(p)
	if err != nil || l.Mode != mode {
		return nil, 0
	}
	return l, l.Depth
}
