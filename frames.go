package main

// Frames: a page may hold several pictures (an animation, a C64 file's screens,
// a teletext carousel's subpages). The tab's doc is always the frame being
// worked on; the others wait in Tab.frames and are never changed in place, so
// undo steps may share them.

import (
	"bytes"
	"fmt"
	"slices"
	"strings"
	"time"
)

// Frame is one picture of a page with several.
type Frame struct {
	doc   *Doc   // stale while the frame is the current one: Tab.doc is it then
	hold  int    // ms shown when played; 0: holdDefault(mode)
	name  string // a Petmate screen's name
	lower int    // PETSCII: this screen's case, -1 not said, 0 upper, 1 lower
	seg   []byte // .tti: the subpage text it came from; nil for a new frame
	src   int    // .petmate: its framebuf in the file (-1 for a new frame)
}

// holdDefault is how long a frame without a hold of its own is shown: teletext
// carousels turn every 8 seconds, animations run at 10 pictures a second.
func holdDefault(mode string) int {
	if mode == "tt" || mode == "vd" {
		return 8000
	}
	return 100
}

// holdSteps are the hold times the keys and the strip step through (ms).
var holdSteps = []int{20, 40, 60, 80, 100, 150, 200, 300, 500, 750, 1000, 1500, 2000, 3000, 5000, 8000}

func (t *Tab) nFrames() int {
	if t.frames == nil {
		return 1
	}
	return len(t.frames)
}

// frameDoc is frame i's picture (the live one for the current frame).
func (t *Tab) frameDoc(i int) *Doc {
	if t.frames == nil || i == t.cur {
		return t.doc
	}
	return t.frames[i].doc
}

// allDocs are every frame's pictures in order.
func (t *Tab) allDocs() []*Doc {
	out := make([]*Doc, t.nFrames())
	for i := range out {
		out[i] = t.frameDoc(i)
	}
	return out
}

// holdOf is frame i's hold time in ms.
func (t *Tab) holdOf(i int) int {
	if t.frames != nil && i < len(t.frames) && t.frames[i].hold > 0 {
		return t.frames[i].hold
	}
	return holdDefault(t.mode)
}

// ensureFrames makes the one picture of a page a frame list of one.
func (t *Tab) ensureFrames() {
	if t.frames == nil {
		t.frames, t.cur = []Frame{{doc: t.doc, lower: -1, src: -1}}, 0
	}
}

// frameCells counts the cells of all frames.
func (t *Tab) frameCells() int {
	n := 0
	for _, d := range t.allDocs() {
		n += d.cols * d.rows
	}
	return n
}

// keepCase notes the current frame's PETSCII case in its frame.
func (a *App) keepCase() {
	if a.frames != nil && a.c64() {
		a.frames[a.cur].lower = map[bool]int{true: 1}[a.lower()]
	}
}

// showCase puts the current frame's own PETSCII case on.
func (a *App) showCase() {
	if a.frames != nil && a.c64() && a.frames[a.cur].lower >= 0 {
		a.setCase(a.frames[a.cur].lower == 1)
	}
}

// goFrame makes frame i the one worked on.
func (a *App) goFrame(i int) {
	if a.frames == nil || i < 0 || i >= len(a.frames) {
		return
	}
	a.endPlay()
	if i != a.cur {
		a.keepCase()
		a.frames[a.cur].doc = a.doc
		a.cur = i
		a.doc = a.frames[i].doc.clone()
		a.showCase()
	}
	a.sel, a.float, a.moving, a.inRun = nil, nil, false, false
	a.cx, a.cy = min(a.cx, a.doc.cols-1), min(a.cy, a.doc.rows-1)
	a.applyLook()
	a.frameMsg()
}

func (a *App) frameMsg() {
	if a.frames == nil {
		return
	}
	s := fmt.Sprintf("Frame %d of %d, %s", a.cur+1, len(a.frames), holdText(a.mode, a.holdOf(a.cur)))
	if n := a.frames[a.cur].name; n != "" {
		s += ", " + n
	}
	a.msg(s)
}

// holdText says a hold time: whole seconds on teletext (the .tti CT line), else ms.
func holdText(mode string, ms int) string {
	if mode == "tt" || mode == "vd" {
		return fmt.Sprintf("%d s", (ms+500)/1000)
	}
	return fmt.Sprintf("%d ms", ms)
}

// blankLike is an empty picture of this page's type, the size and colours of d.
func (a *App) blankLike(d *Doc) *Doc {
	var n *Doc
	switch {
	case a.zx():
		n = newZXDoc()
	case a.vd():
		n = newVDDoc()
	case a.tt():
		n = newTTDoc()
	case a.mt():
		n = newMTDoc()
	case a.cpc():
		n = newCPCDoc(a.cpcSet.mode)
	case a.msx():
		n = newMSXDoc(a.msxSet.mode)
	case a.mz():
		n = newMZDoc()
	case a.st():
		n = newSTDoc(a.res())
		n.ensureRows(d.rows)
		n.rows = d.rows
	default:
		n = newDoc(d.cols, d.rows)
	}
	n.bg, n.border, n.ink, n.glyph0 = d.bg, d.border, d.ink, d.glyph0
	return n
}

// addFrame puts a picture in after the current frame and goes to it.
func (a *App) addFrame(d *Doc, copied bool, what string) {
	a.endPlay()
	if a.layers != nil {
		a.msg(layersNoFrames)
		return
	}
	if a.frameCells()+d.cols*d.rows > maxPageCells {
		a.msg("No more frames: all of them together would be larger than the largest page")
		return
	}
	a.pushFramesUndo(0)
	a.ensureFrames()
	a.keepCase()
	f := Frame{doc: d, hold: a.frames[a.cur].hold, lower: a.frames[a.cur].lower, src: -1}
	if copied { // a copied subpage keeps its lines (Fastext links, enhancements)
		f.seg = a.frames[a.cur].seg
	}
	a.frames[a.cur].doc = a.doc
	a.frames = slices.Insert(a.frames, a.cur+1, f)
	a.restructured = true
	a.goFrame(a.cur + 1)
	a.msg(fmt.Sprintf("%s: frame %d of %d (%s undoes it)", what, a.cur+1, len(a.frames), keyName("undo")))
}

func (a *App) newFrame() { a.addFrame(a.blankLike(a.doc), false, "New frame") }
func (a *App) dupFrame() { a.addFrame(a.doc.clone(), true, "Copied frame") }

func (a *App) deleteFrame() {
	if a.nFrames() < 2 {
		a.msg("The last frame cannot be deleted")
		return
	}
	a.endPlay()
	a.pushFramesUndo(a.doc.cols * a.doc.rows)
	a.frames = slices.Delete(a.frames, a.cur, a.cur+1)
	a.cur = min(a.cur, len(a.frames)-1)
	a.doc = a.frames[a.cur].doc.clone()
	a.restructured = true
	a.showCase()
	a.goFrame(a.cur)
	a.msg(fmt.Sprintf("Frame deleted: now frame %d of %d (%s undoes it)", a.cur+1, len(a.frames), keyName("undo")))
}

// moveFrame moves the current frame d places, earlier (-1) or later (+1).
func (a *App) moveFrame(d int) {
	j := a.cur + d
	if a.frames == nil || j < 0 || j >= len(a.frames) {
		return
	}
	a.endPlay()
	a.pushFramesUndo(0)
	a.keepCase()
	a.frames[a.cur].doc = a.doc
	a.frames[a.cur], a.frames[j] = a.frames[j], a.frames[a.cur]
	a.cur = j
	a.restructured = true
	a.frameMsg()
}

// setHold sets the current frame's hold time.
func (a *App) setHold(ms int) {
	ms = max(20, min(99000, ms))
	if a.holdOf(a.cur) == ms {
		return
	}
	a.endPlay()
	a.pushFramesUndo(0)
	a.ensureFrames()
	a.frames[a.cur].hold = ms
	a.frameMsg()
}

// stepHold makes the hold one step shorter (-1) or longer (+1); teletext by whole seconds, 1-99.
func (a *App) stepHold(d int) {
	h := a.holdOf(a.cur)
	if a.tt() {
		a.setHold(max(1, min(99, (h+500)/1000+d)) * 1000)
		return
	}
	i, _ := slices.BinarySearch(holdSteps, h)
	if d > 0 {
		if i < len(holdSteps) && holdSteps[i] == h {
			i++
		}
	} else {
		i--
	}
	a.setHold(holdSteps[max(0, min(len(holdSteps)-1, i))])
}

// withEveryFrame replaces every frame's picture with fn of it, as one undo step
// (a page's width is the same for all its frames).
func (a *App) withEveryFrame(fn func(*Doc) *Doc) {
	old := 0
	for _, d := range a.allDocs() {
		old += d.cols * d.rows
	}
	a.pushFramesUndo(old - a.doc.cols*a.doc.rows)
	for i := range a.frames {
		if i != a.cur {
			a.frames[i].doc = fn(a.frames[i].doc)
		}
	}
	a.doc = fn(a.doc)
}

// ---------------------------------------------------------------- undo of frames

// pushFramesUndo keeps the whole frame list as an undo step (new, copied,
// deleted or moved frames, hold times, all frames' width). extra counts the
// cells of pictures the step lets go of, which the step keeps alive.
func (a *App) pushFramesUndo(extra int) {
	s := a.snapshot()
	s.whole = true
	if a.frames != nil {
		s.frames = slices.Clone(a.frames)
		s.frames[a.cur].doc = s.doc
		s.cost += 8 * len(s.frames)
	}
	s.cost += extra
	a.pushSnap(s)
}

// snapshotLike is a snapshot of the page as it is now, of the kind s is: the
// whole frame list, or the picture of s's frame.
func (a *App) snapshotLike(s Snap) Snap {
	if s.whole {
		n := a.snapshot()
		n.whole = true
		if a.frames != nil {
			n.frames = slices.Clone(a.frames)
			n.frames[a.cur].doc = n.doc
		}
		return n
	}
	if a.frames != nil && s.cur != a.cur && s.cur < len(a.frames) {
		d := a.frames[s.cur].doc.clone()
		return Snap{doc: d, cx: a.cx, cy: a.cy, gen: a.gen, cur: s.cur, cost: len(d.cells), order: a.order, mode: a.mode, cpc: a.cpcSnap(), vic: a.vicSnap(), step: a.steps, msx: a.msxSnap()}
	}
	return a.snapshot()
}

// framesFormats are the page formats that keep every frame.
var framesFormats = map[string]bool{"marq": true, "petmate": true, "tti": true, "ansi": true, "seq": true, "ata": true, "vt52": true}

// savedFrames: the page's frames are now its file's (data), in order. For a
// .tti it gives how many subpages were written anew.
func (a *App) savedFrames(fmtID string, data []byte) (changed int) {
	if fmtID == "tti" && a.frames != nil {
		_, subs := ttiHead(data)
		for i := range a.frames {
			if i < len(subs) {
				if !bytes.Equal(subs[i], a.frames[i].seg) {
					changed++
				}
				a.frames[i].seg = subs[i]
			}
		}
	}
	for i := range a.frames {
		a.frames[i].src = i
	}
	a.restructured = false
	return changed
}

// writeTTIFrames writes the page's frames as a .tti carousel, on the file it came from.
func (a *App) writeTTIFrames() []byte {
	var head []byte
	if a.srcKind == "tti" && a.source != nil {
		head, _ = ttiHead(a.source)
	} else {
		head = []byte("DE,Made with CharEdit\r\n")
	}
	fr := make([]ttiFrame, a.nFrames())
	for i := range fr {
		fr[i].doc = a.frameDoc(i)
		if a.frames != nil {
			fr[i].hold, fr[i].seg = a.frames[i].hold, a.frames[i].seg
		}
	}
	return writeTTIFrames(head, fr, a.restructured)
}

// keepFramesHint names the formats of this page that keep every frame.
func (a *App) keepFramesHint() string {
	var exts []string
	for _, f := range a.modeFormats() {
		if framesFormats[f.id] {
			exts = append(exts, f.ext)
		}
	}
	if len(exts) == 0 {
		return "no file of this page type keeps frames yet"
	}
	return "save as " + strings.Join(exts, ", ") + " to keep them"
}

// ---------------------------------------------------------------- playing the frames

// playSpeeds are the speeds the frames play at, and their names.
var (
	playSpeeds     = []float64{1, 0.5, 2, 4}
	playSpeedNames = []string{"1x", "1/2x", "2x", "4x"}
)

// playKey plays the frames, or a page of one picture as from a modem; again stops.
func (a *App) playKey() {
	if a.playing != nil {
		a.stopPlay(true)
		return
	}
	if a.nFrames() > 1 {
		a.playFrames()
		return
	}
	a.play()
}

// playFrames shows the frames one after the other, each for its hold time,
// from the first; Loop starts again after the last. The page is not changed.
func (a *App) playFrames() {
	a.endPlay()
	n := a.nFrames()
	a.keepCase()
	p := &Play{saved: a.snapshot(), frames: a.allDocs(), holds: make([]int, n), lowers: make([]int, n)}
	for i := range p.holds {
		p.holds[i] = a.holdOf(i)
		p.lowers[i] = -1
		if a.frames != nil {
			p.lowers[i] = a.frames[i].lower
		}
	}
	p.next = time.Now().Add(a.scaledHold(p.holds[0]))
	a.playing = p
	a.showPlayed()
	a.sel, a.float = nil, nil
	a.msg(fmt.Sprintf("Playing %d frames (%s or Esc stops)", n, keyName("play")))
}

// scaledHold is a hold time at the chosen speed.
func (a *App) scaledHold(ms int) time.Duration {
	return time.Duration(float64(ms)/playSpeeds[a.playSpeed%len(playSpeeds)]) * time.Millisecond
}

// showPlayed puts the frame being played on show.
func (a *App) showPlayed() {
	p := a.playing
	a.doc = p.frames[p.fi]
	if l := p.lowers[p.fi]; l >= 0 && a.c64() {
		a.setCase(l == 1)
	}
	a.applyLook()
}

// frameStep goes on to the frames whose time has come; without Loop the last one ends it.
func (a *App) frameStep(now time.Time) {
	p := a.playing
	moved := false
	for !now.Before(p.next) {
		p.fi++
		if p.fi >= len(p.frames) {
			if a.noLoop {
				a.stopPlay(false)
				return
			}
			p.fi = 0
		}
		p.next = p.next.Add(a.scaledHold(p.holds[p.fi]))
		moved = true
		if p.next.Before(now.Add(-time.Second)) { // far behind (the window was held): go on from now
			p.next = now
		}
	}
	if moved {
		a.showPlayed()
	}
}
