package main

// Detail levels and what an animation costs a caller. A saved animation is
// paced by the connection: every byte takes 10 bits on the line, so a file of
// a few hundred KB keeps a caller at 14400 bps waiting minutes. Each template
// has three levels: Small (modem-friendly: fewer frames, little or no glyph
// churn, short trails, a draw order in runs of cells), Medium, and Large (the
// full look). The cost of the chosen one is shown before it is applied: the
// file's size and how long it plays at each speed (bytes x 10 / bps, plus the
// board's pauses; over telnet only the pauses count).

import (
	"fmt"
	"math"
	"strings"
)

// The detail levels. Large is the zero value: pages saved before there were
// levels keep their look.
const (
	detailLarge = iota
	detailMedium
	detailSmall
	nDetails
)

var detailIDs = [nDetails]string{"large", "medium", "small"}
var detailNames = [nDetails]string{"Large", "Medium", "Small"}

// detailsUp are the levels from the smallest file to the full look (for lists).
var detailsUp = []int{detailSmall, detailMedium, detailLarge}

// detailByID reads a level ("" is Large); -1 for none.
func detailByID(s string) int {
	if s == "" {
		return detailLarge
	}
	for i, id := range detailIDs {
		if strings.EqualFold(id, s) || strings.EqualFold(detailNames[i], s) || strings.EqualFold(id[:1], s) {
			return i
		}
	}
	return -1
}

// effectFramesAt: a frame effect's frames at Small and Medium (Large: effectDefaults).
var effectFramesAt = map[int][2]int{
	patMatrix:   {22, 40},
	patDecrypt:  {16, 30},
	patDrop:     {18, 32},
	patAssemble: {14, 28},
	patShine:    {12, 20},
	patShadeIn:  {14, 26},
	patMelt:     {10, 22},
	patPowerOn:  {12, 20},
}

// effectOwn is an effect's own frames and hold at a level: fewer frames are
// held longer, so it takes about as long.
func effectOwn(pat, detail int) (frames, hold int) {
	def := effectDefaults[pat]
	if detail == detailLarge || detail < 0 || detail >= nDetails {
		return def[0], def[1]
	}
	frames = effectFramesAt[pat][map[int]int{detailSmall: 0, detailMedium: 1}[detail]]
	return frames, int(math.Round(float64(def[0]*def[1]) / float64(frames)))
}

// effectLengthAt is the frame count of an effect at a level: n, or its own when n is 0.
func effectLengthAt(pat, n, detail int) int {
	if n <= 0 {
		n, _ = effectOwn(pat, detail)
	}
	return max(2, min(maxEffectFrames, n))
}

// scanRun is how many cells of a row a draw order puts down together at a
// level: one by one at Large; in short runs below, so the cursor moves less.
func scanRun(detail int) int {
	return map[int]int{detailMedium: 3, detailSmall: 8}[detail] | 1
}

// fadePassesAt is how many times a fade-in draws the picture at a level.
func fadePassesAt(detail int) int {
	return map[int]int{detailLarge: fadePasses, detailMedium: 3, detailSmall: 2}[detail]
}

// detail is the order's level.
func (o *drawOrder) detail() int {
	if d := detailByID(o.Detail); d >= 0 {
		return d
	}
	return detailLarge
}

// ---------------------------------------------------------------- speeds and time

// telnet is the speed list's "telnet (fast)": the bytes take no time, only the pauses.
const telnet = 0

// costSpeeds are the speeds the cost of an animation is shown at.
var costSpeeds = []int{2400, 9600, 14400, 33600, 57600, telnet}

func speedName(bps int) string {
	if bps <= 0 {
		return "telnet (fast)"
	}
	return fmt.Sprintf("%d bps", bps)
}

// playMs is how long sent bytes take at bps with pauses of pauseMs in all.
func playMs(sent, pauseMs, bps int) int {
	if bps <= 0 {
		return pauseMs
	}
	return int((int64(sent)*10*1000+int64(bps)/2)/int64(bps)) + pauseMs
}

// durWords: 4 min 20 s, 12.5 s, 0.3 s.
func durWords(ms int) string {
	switch {
	case ms >= 3600*1000:
		return fmt.Sprintf("%d h %d min", ms/3600000, ms/60000%60)
	case ms >= 60*1000:
		return fmt.Sprintf("%d min %d s", ms/60000, (ms/1000)%60)
	case ms >= 10*1000:
		return fmt.Sprintf("%d s", (ms+500)/1000)
	}
	return fmt.Sprintf("%.1f s", float64(ms)/1000)
}

// kbWords: 630 KB, 4.2 KB, 812 bytes.
func kbWords(n int) string {
	switch {
	case n >= 100*1024:
		return fmt.Sprintf("%d KB", (n+512)/1024)
	case n >= 1024:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	}
	return fmt.Sprintf("%d bytes", n)
}

// animCost is what saving a page with an order costs.
type animCost struct {
	size    int // the saved file, with the board's codes and SAUCE: what is written
	sent    int // what the caller's terminal gets (the board takes its codes out)
	pauseMs int // the board's pauses in all
	plain   int // the page saved row by row, no animation
	used    int // the cells that show something
}

// at is how long it plays at bps.
func (c animCost) at(bps int) int { return playMs(c.sent, c.pauseMs, bps) }

// perCell is the bytes the animation takes per cell that shows something.
func (c animCost) perCell() float64 { return float64(c.sent) / float64(max(1, c.used)) }

// The weight classes, by bytes per cell that shows something (a plain ANSI
// screen takes about 2 to 5).
const (
	lightPerCell  = 7.0
	mediumPerCell = 20.0
)

const (
	weightLight = iota
	weightMedium
	weightHeavy
)

var weightNames = []string{"light", "medium", "heavy"}

func (c animCost) weight() int {
	switch p := c.perCell(); {
	case p <= lightPerCell:
		return weightLight
	case p <= mediumPerCell:
		return weightMedium
	}
	return weightHeavy
}

// costOf is what saving the current page as fmtID with order o and board's
// codes costs: the bytes are exactly what Save would write. The page, its
// order and its message are as they were afterwards.
func (a *App) costOf(fmtID string, o *drawOrder, board string) animCost {
	keepOrder, keepInfo, msg, at := a.order, a.info, a.message, a.messageAt
	defer func() { a.order, a.info, a.message, a.messageAt = keepOrder, keepInfo, msg, at }()
	var c animCost
	a.order = nil
	plain, _ := a.encodeFor(fmtID, "", nil)
	c.plain = len(plain)
	a.info = keepInfo
	a.order = o
	sent, _ := a.encodeFor(fmtID, "", nil)
	c.sent = len(sent)
	a.info = keepInfo
	var ps []pause
	file, _ := a.encodeFor(fmtID, board, &ps)
	c.size = len(file)
	for _, p := range ps {
		c.pauseMs += p.ms
	}
	c.used = len(a.cellsInOrder(a.doc, a.paperOf(a.doc), patTop, false, false))
	return c
}
