package main

// Animations in streams: an ANSI, PETSCII, ATASCII or VT52 file that clears
// the screen between pictures (an ANSImation) is cut into frames at each
// clear, or on request at each cursor home; frames are written back the same
// way, a clear before every frame after the first.

import (
	"bytes"
	"fmt"
)

// Limits of a split: no more frames, nor more cells in all, than these.
const maxSplitFrames = 10000

// hasInk: something is drawn on the picture (ST cells carry their paper colour).
func hasInk(d *Doc, mode string) bool {
	if mode == "st" {
		for _, v := range d.cells[:d.rows*d.cols] {
			if ch := v & 255; ch != 32 && ch != 0 {
				return true
			}
		}
		return false
	}
	return d.inkCells() > 0
}

// splitStream plays the file through the page type's terminal and takes a
// frame at every clear (atHome: at every cursor home too), and the last
// screen; each frame holds as long as its bytes take at baud bits per second.
// homes counts the cursor homes after something was drawn.
// codes: a BBS colour-code kind (bbscodes.go) the file is read with, as on opening it.
func splitStream(b []byte, mode string, cols int, amiga bool, res STRes, atHome bool, baud int, codes ...string) (docs []*Doc, holds []int, homes int) {
	if baud <= 0 {
		baud = 14400
	}
	var d *Doc
	var em emulator
	var setHooks func(clear, home func())
	switch mode {
	case "c64", "plus4": // plus4: a Plus/4's PETSCII (ted.go)
		d = newDoc(cols, 25)
		d.glyph0 = true
		t := newPetTermOn(d, mode == "plus4")
		em, setHooks = t, func(c, h func()) { t.beforeClear, t.beforeHome = c, h }
	case "atari":
		d = newDoc(cols, 24)
		d.bg, d.ink, d.border, d.glyph0 = atariBg, atariInk, 0, true
		t := newAtariTerm(d)
		em, setHooks = t, func(c, h func()) { t.beforeClear = c }
	case "st":
		d = newDoc(cols, res.rows)
		t := newSTTerm(d, res)
		fillCells(d.cells, t.blankCell())
		em, setHooks = t, func(c, h func()) { t.beforeClear = c }
	default:
		if s, end := readSauce(b); s != nil {
			b = b[:end]
		}
		if n := dataEnd(b); n < len(b) {
			b = b[:n]
		}
		// DOS's end of file, with only line ends after it: not part of the last picture.
		if i := bytes.LastIndexByte(b, 0x1A); i >= 0 && len(bytes.Trim(b[i:], "\x1a\r\n\x00 ")) == 0 {
			b = b[:i]
		}
		d = newDoc(cols, 25)
		t := newTerm(d, amiga)
		em, setHooks = t, func(c, h func()) { t.beforeClear, t.beforeHome = c, h }
		if len(codes) > 0 && codes[0] != "" {
			em = newCodeTerm(t, codes[0]) // (the hooks stay on the Term it feeds)
		}
	}
	pos, last, cells := 0, 0, 0
	full := false
	take := func(home bool) {
		x, y := em.cursor()
		cur := emDoc(em)
		if full || !hasInk(cur, mode) {
			return
		}
		if home {
			homes++
			if !atHome {
				return
			}
		}
		f := cur.clone()
		f.fit(cursorRows(x, y))
		if len(docs) >= maxSplitFrames || cells+f.cols*f.rows > maxPageCells {
			full = true
			return
		}
		cells += f.cols * f.rows
		docs = append(docs, f)
		holds = append(holds, max(20, (pos-last)*10*1000/baud))
		last = pos
	}
	setHooks(func() { take(false) }, func() { take(true) })
	for pos = 0; pos < len(b); pos++ {
		em.write(b[pos : pos+1])
	}
	take(false)
	return docs, holds, homes
}

// emDoc is the page an emulator of any page type draws on.
func emDoc(em emulator) *Doc {
	if t, ok := em.(*STTerm); ok {
		return t.doc
	}
	return docOf(em)
}

// splitInfo is what a stream page's file holds as an animation.
type splitInfo struct {
	clears, homes int // frames when cut at clears; cursor homes after ink
}

// streamSplit looks at the page's file: how many pictures it shows.
func (a *App) streamSplit() splitInfo {
	if a.source == nil || a.srcKind != "" || a.sourceBin || a.zx() || a.tt() || a.cpc() || a.mt() {
		return splitInfo{}
	}
	docs, _, homes := splitStream(a.source, a.tedMode(), a.doc.cols, a.amiga(), a.res(), false, lineBps(a.baud), a.codes)
	return splitInfo{len(docs), homes}
}

// splitFrames cuts the page's file into frames, at each clear (or each cursor
// home), as one undo step.
func (a *App) splitFrames(atHome bool) {
	a.endPlay()
	docs, holds, _ := splitStream(a.source, a.tedMode(), a.doc.cols, a.amiga(), a.res(), atHome, lineBps(a.baud), a.codes)
	if len(docs) < 2 {
		a.msg("The file shows only one picture")
		return
	}
	a.pushFramesUndo(0)
	a.frames = make([]Frame, len(docs))
	for i, d := range docs {
		a.frames[i] = Frame{doc: d, hold: holds[i], lower: -1, src: -1}
	}
	a.cur, a.restructured = 0, true
	a.doc = a.frames[0].doc.clone()
	a.split = splitInfo{}
	a.sel, a.float = nil, nil
	a.applyLook()
	how := "clear screen"
	if atHome {
		how = "cursor home"
	}
	a.msg(fmt.Sprintf("Split at each %s: %d frames, each held as long as it took at %d bps (%s undoes it)", how, len(docs), lineBps(a.baud), keyName("undo")))
}

// writeStreamFrames writes every frame as one stream: a clear before every frame after the first.
func (a *App) writeStreamFrames(fmtID string, docs []*Doc, sauce bool) []byte {
	out, _ := a.writeStreamFramesPaced(fmtID, docs, sauce)
	return out
}

// writeStreamFramesPaced is writeStreamFrames with a BBS pause after every
// frame but the last: the frame's hold.
func (a *App) writeStreamFramesPaced(fmtID string, docs []*Doc, sauce bool) ([]byte, []pause) {
	var out []byte
	var ps []pause
	for i, d := range docs {
		if i > 0 {
			ps = append(ps, pause{len(out), a.holdOf(i - 1)})
		}
		clear := i > 0 || a.info.clear
		switch fmtID {
		case "seq":
			out = append(out, writeSeqOn(d, a.lower(), clear, a.ted())...)
		case "ata":
			out = append(out, writeAta(d, clear)...)
		case "vt52":
			out = append(out, writeVT52(d, a.res())...) // starts with ESC E: a clear
		default:
			out = append(out, writeAnsi(d, a.amiga(), true, clear, sauce)...)
		}
	}
	return out, ps
}

// splitPage: the page is a stream file's frames (reading the file again would give one picture).
func (a *App) splitPage() bool { return a.srcKind == "" && a.nFrames() > 1 }
