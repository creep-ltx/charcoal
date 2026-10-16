package main

// The Unicode page's files (docs/unicode-design.md): UTF-8 ANSI with 256 and
// 24-bit colours, durdraw's .dur (gzipped JSON, frames), mIRC colour codes, and
// a web page. Readers never trust sizes: pages stay within maxPageCells.

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// ---------------------------------------------------------------- the terminal's colours

// uniColours are the terminal's colours now (bold, blink, reverse and conceal applied).
func (t *Term) uniColours() (fg, bg int) {
	fg, bg = t.fg, t.bg
	if t.bold || t.hiFg {
		fg += 8
	}
	if t.blink {
		bg += 8
	}
	if t.fgX >= 0 {
		fg = t.fgX
	}
	if t.bgX >= 0 {
		bg = t.bgX
	}
	if t.reverse {
		fg, bg = bg, fg
	}
	if t.conceal {
		fg = bg
	}
	return fg, bg
}

// uniAttr is attr for a Unicode page whose colours go beyond the 16.
func (t *Term) uniAttr() uint32 {
	fg, bg := t.uniColours()
	var flags uint32
	if t.ital {
		flags |= italic
	}
	if t.under {
		flags |= underline
	}
	return uniCell(0, styleOf(fg, bg, flags))
}

// uniSgr reads 38/48;5;n and 38/48;2;r;g;b at p[i]; it returns the index of
// their last number. Colours 0-15 go the 16-colour way (bold makes them bright).
func (t *Term) uniSgr(p []int, i int) int {
	fore := p[i] == 38
	clamp := func(v int) uint32 { return uint32(min(255, max(0, v))) }
	c := -1
	switch {
	case i+2 < len(p) && p[i+1] == 5:
		c = min(255, max(0, p[i+2]))
		i += 2
	case i+4 < len(p) && p[i+1] == 2:
		c = rgbColour(clamp(p[i+2])<<16 | clamp(p[i+3])<<8 | clamp(p[i+4]))
		i += 4
	default:
		return len(p)
	}
	switch {
	case c < 16 && fore:
		t.fg, t.hiFg, t.fgX = c&7, c > 7, -1
	case c < 16:
		t.bg, t.blink, t.hiBg, t.bgX = c&7, c > 7, c > 7, -1
	case fore:
		t.fgX = c
	default:
		t.bgX = c
	}
	return i
}

// ---------------------------------------------------------------- UTF-8 ANSI

// uniText is a file's art as text: UTF-8, or else CP437 (an ANSI opened as Unicode).
func uniText(b []byte) string {
	if utf8.Valid(b) {
		return string(b)
	}
	var sb strings.Builder
	for _, c := range b {
		if c >= 0x80 {
			sb.WriteRune(cp437[c])
		} else {
			sb.WriteByte(c)
		}
	}
	return sb.String()
}

// uniLongestLine counts the characters of the longest line (escape codes left out).
func uniLongestLine(s string) int {
	most, n, esc := 0, 0, 0
	for _, r := range s {
		switch {
		case esc == 1:
			esc = 0
			if r == '[' {
				esc = 2
			}
		case esc == 2:
			if r >= 0x40 && r <= 0x7E {
				esc = 0
			}
		case r == 0x1B:
			esc = 1
		case r == '\n':
			n = 0
		case r < 0x20 || (r >= 0x7F && r < 0xA0):
		default:
			n++
			most = max(most, n)
		}
	}
	return most
}

// parseUniArt reads UTF-8 ANSI (or CP437 ANSI) into a Unicode page.
func parseUniArt(b []byte, forceCols int) (*Doc, *Sauce) {
	sauce, end := readSauce(b)
	text := uniText(b[:end])
	cols := 80
	switch {
	case forceCols > 0:
		cols = min(forceCols, 1000)
	case sauce != nil && sauce.dataType == 1 && sauce.tinfo1 >= 1 && sauce.tinfo1 <= 1000:
		cols = sauce.tinfo1
	default:
		if n := uniLongestLine(text); n > cols && n <= 1000 {
			cols = n
		}
	}
	d := newDoc(cols, 25)
	d.uni = true
	t := newTerm(d, false)
	t.uni, t.lateWrap = true, true
	var f fullest
	t.beforeClear = func() { f.look(t.doc) }
	for _, r := range text {
		switch {
		case r < 0x80:
			t.byte(byte(r))
		case t.state != 0 || r < 0xA0: // inside an escape code, or a C1 control: nothing to draw
		case r == utf8.RuneError:
			t.putCode('?')
		default:
			code, _ := uniCodeOf(r)
			t.putCode(code)
		}
	}
	if d = f.pick(t.doc); d == t.doc {
		d.fit(cursorRows(t.x, t.y))
		if sauceArt(sauce) && sauce.tinfo2 > d.rows && sauce.tinfo2 <= maxRowsFor(d.cols) && t.screens <= 1 {
			d.ensureRows(sauce.tinfo2)
		}
	} else {
		d.fit(0)
	}
	d.uni = true
	return d, sauce
}

// putCode puts a character code beyond a byte (put does the rest).
func (t *Term) putCode(code int) {
	d := t.doc
	if t.x >= d.cols {
		t.x, t.y = 0, t.y+1
	}
	d.set(t.x, t.y, uniCell(code, 0)|t.attr())
	t.x++
	if t.x >= d.cols && !t.lateWrap {
		t.x, t.y = 0, t.y+1
	}
}

// sgrColour is the SGR code of a colour, fore- or background.
func sgrColour(c int, fore bool) string {
	base, bright, ext := 30, 90, "38"
	if !fore {
		base, bright, ext = 40, 100, "48"
	}
	switch {
	case c&rgbFlag != 0:
		return fmt.Sprintf("%s;2;%d;%d;%d", ext, c>>16&255, c>>8&255, c&255)
	case c < 8:
		return strconv.Itoa(base + c)
	case c < 16:
		return strconv.Itoa(bright + c - 8)
	}
	return ext + ";5;" + strconv.Itoa(c)
}

// writeUniAnsi gives a Unicode page as UTF-8 ANSI for today's terminals: SGR codes
// only where something changes (90-97 and 100-107 for the bright 16, 38/48;5 for
// the 256, 38/48;2 for 24-bit), blanks at the end of a row dropped, a reset and
// a line feed after every row (so a terminal never carries a colour over).
func writeUniAnsi(d *Doc) []byte {
	var out bytes.Buffer
	type pen struct {
		fg, bg int
		it, ul bool
	}
	plain := pen{fg: 7, bg: 0}
	out.WriteString("\x1b[0m")
	ends, widest := make([]int, d.rows), 0
	for y := range ends {
		end := d.cols
		for end > 0 && uniBlank(d.get(end-1, y)) {
			end--
		}
		ends[y], widest = end, max(widest, end)
	}
	if widest < d.cols && d.rows > 0 {
		ends[0] = d.cols // one row in full, so the lines give the page's width (no SAUCE record needed)
	}
	for y := 0; y < d.rows; y++ {
		cur := plain
		end := ends[y]
		for x := 0; x < end; x++ {
			v := d.get(x, y)
			fg, bg, fl := styleParts(uniPair(v))
			want := pen{fg, bg, fl&italic != 0, fl&underline != 0}
			if uniCode(v) == 32 && !want.ul { // a space shows only its background
				want.fg, want.it = cur.fg, cur.it
			}
			if want != cur {
				var codes []string
				if (cur.it && !want.it) || (cur.ul && !want.ul) {
					codes, cur = append(codes, "0"), plain
				}
				if want.it && !cur.it {
					codes = append(codes, "3")
				}
				if want.ul && !cur.ul {
					codes = append(codes, "4")
				}
				if want.fg != cur.fg {
					codes = append(codes, sgrColour(want.fg, true))
				}
				if want.bg != cur.bg {
					codes = append(codes, sgrColour(want.bg, false))
				}
				out.WriteString("\x1b[" + strings.Join(codes, ";") + "m")
				cur = want
			}
			r := uniRune(uniCode(v))
			if r < 0x20 || r == 0x7F {
				r = ' '
			}
			out.WriteRune(r)
		}
		if cur != plain {
			out.WriteString("\x1b[0m")
		}
		out.WriteByte('\n')
	}
	return out.Bytes()
}

// ---------------------------------------------------------------- durdraw

// durdraw's colour numbers (durdraw_color_curses.py, durformat.md): in 16-colour
// files the foreground is 1 + the PC's (BIOS) colour, the background the BIOS
// colour 0-7 (8: black, 9-16: durdraw's iCE order); in 256-colour files the
// foreground 1-16 is mapped the way its ANSI export maps it, 17-255 are xterm's,
// the background is xterm's (durdraw keeps 256-colour backgrounds black).
var (
	durBios     = [8]int{0, 4, 2, 6, 1, 5, 3, 7}
	durIceBg    = [17]int{0, 4, 2, 6, 1, 5, 3, 7, 8, 12, 10, 14, 9, 13, 11, 15, 16}
	dur256Fg    = [17]int{0, 4, 2, 6, 1, 5, 3, 7, 8, 12, 10, 14, 9, 13, 11, 15, 16}
	durMaxCells = maxPageCells // all frames together
)

func durFg(f int, c256 bool) int {
	switch {
	case c256 && f >= 0 && f <= 16:
		return dur256Fg[f]
	case c256 && f <= 255 && f > 0:
		return f
	case c256:
		return 7
	case f <= 0:
		return 0
	case f <= 16:
		b := f - 1
		return durBios[b&7] + b&8
	}
	return 7
}

func durBg(g int, c256 bool) int {
	switch {
	case c256 && (g == 8 || g < 0 || g > 255):
		return 0
	case c256:
		return g
	case g >= 0 && g <= 7:
		return durBios[g]
	case g >= 9 && g <= 15:
		return durIceBg[g]
	}
	return 0
}

// durColours gives a page colour pair in durdraw's numbers.
func durColours(fg, bg int, c256 bool) [2]int {
	ansiToBios := [8]int{0, 4, 2, 6, 1, 5, 3, 7} // its own inverse
	if !c256 {
		f := 1 + ansiToBios[fg&7] + fg&8
		g := ansiToBios[bg&7]
		if bg > 8 && bg < 16 {
			for i := 9; i <= 15; i++ {
				if durIceBg[i] == bg {
					g = i
				}
			}
		}
		return [2]int{f, g}
	}
	f := fg
	if fg <= 16 {
		for i, v := range dur256Fg {
			if v == fg {
				f = i
				break
			}
		}
	}
	g := bg
	if bg == 8 {
		g = nearest256(xterm256(8))
	}
	return [2]int{f, g}
}

// durdraw's tables for files before format 7 (durdraw_color_curses.py legacy_*).
var (
	durLegacy16      = map[int]int{1: 8, 2: 4, 3: 6, 4: 2, 5: 7, 6: 3, 7: 5, 8: 1, 9: 16, 10: 12, 11: 14, 12: 10, 13: 15, 14: 11, 15: 13, 16: 9}
	durLegacy16Bg    = map[int]int{1: 7, 2: 3, 3: 5, 4: 1, 5: 6, 6: 2, 7: 4, 8: 8}
	durLegacy16To256 = map[int]int{1: 7, 2: 3, 3: 5, 4: 1, 5: 6, 6: 2, 7: 4, 8: 16, 9: 15, 10: 11, 11: 13, 12: 9, 13: 14, 14: 10, 15: 12, 16: 8}
	durLegacy256     = map[int]int{1: 7, 2: 3, 3: 5, 4: 1, 5: 6, 6: 2, 7: 4, 8: 16, 9: 12, 10: 10, 11: 14, 12: 9, 13: 13, 14: 11, 15: 15, 16: 8}
)

type durMovie struct {
	FormatVersion int             `json:"formatVersion"`
	ColorFormat   json.RawMessage `json:"colorFormat"`
	PreferredFont string          `json:"preferredFont"`
	Encoding      string          `json:"encoding"`
	Name          string          `json:"name"`
	Artist        string          `json:"artist"`
	Framerate     float64         `json:"framerate"`
	Columns       int             `json:"columns,omitempty"`
	Lines         int             `json:"lines,omitempty"`
	SizeX         int             `json:"sizeX,omitempty"`
	SizeY         int             `json:"sizeY,omitempty"`
	Extra         json.RawMessage `json:"extra"`
	Frames        []durFrame      `json:"frames"`
}

type durFrame struct {
	FrameNumber int       `json:"frameNumber"`
	Delay       float64   `json:"delay"`
	Contents    []string  `json:"contents"`
	ColorMap    [][][]int `json:"colorMap"`
}

// durArt is a durdraw file as Charcoal frames.
type durArt struct {
	docs          []*Doc
	holds         []int
	name, artist  string
	colours256    bool
	formatVersion int
}

func isDur(b []byte) bool {
	if len(b) > 2 && b[0] == 0x1F && b[1] == 0x8B {
		zr, err := gzip.NewReader(bytes.NewReader(b))
		if err != nil {
			return false
		}
		head := make([]byte, 64)
		n, _ := io.ReadFull(zr, head)
		return bytes.Contains(head[:n], []byte(`"DurMovie"`))
	}
	t := bytes.TrimLeft(b[:min(len(b), 64)], " \t\r\n")
	return bytes.HasPrefix(t, []byte("{")) && bytes.Contains(b[:min(len(b), 200)], []byte(`"DurMovie"`))
}

// parseDur reads a durdraw file (gzipped JSON, or plain JSON).
func parseDur(b []byte) (*durArt, error) {
	if len(b) > 2 && b[0] == 0x1F && b[1] == 0x8B {
		zr, err := gzip.NewReader(bytes.NewReader(b))
		if err != nil {
			return nil, err
		}
		u, err := io.ReadAll(io.LimitReader(zr, 64<<20+1))
		if err != nil {
			return nil, err
		}
		if len(u) > 64<<20 {
			return nil, errors.New("too large unpacked")
		}
		b = u
	}
	var f struct {
		DurMovie *durMovie `json:"DurMovie"`
	}
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("not a durdraw file (%v); durdraw's oldest files (Python pickles) cannot be read", err)
	}
	m := f.DurMovie
	if m == nil {
		return nil, errors.New("no DurMovie in it")
	}
	w, h := m.Columns, m.Lines
	if w == 0 && h == 0 {
		w, h = m.SizeX, m.SizeY
	}
	if w < 1 || h < 1 || w > 4096 || h > maxRowsFor(w) || len(m.Frames) == 0 {
		return nil, fmt.Errorf("a canvas of %dx%d with %d frames", w, h, len(m.Frames))
	}
	if len(m.Frames)*w*h > durMaxCells {
		return nil, errors.New("too many frames for its size")
	}
	cf := strings.Trim(string(m.ColorFormat), `" `)
	c256 := cf == "256" || cf == "xterm-256"
	// Files before format 7 numbered colours another way: durdraw converts them as it loads them.
	var legacyFg, legacyBg map[int]int
	if m.FormatVersion < 7 {
		high := false
		for _, fr := range m.Frames {
			for _, row := range fr.ColorMap {
				for _, p := range row {
					high = high || (len(p) > 0 && p[0] > 16)
				}
			}
		}
		switch {
		case high:
			legacyFg, c256 = durLegacy256, true
		case c256:
			legacyFg = durLegacy16To256
		default:
			legacyFg, legacyBg = durLegacy16, durLegacy16Bg
		}
	}
	art := &durArt{name: m.Name, artist: m.Artist, colours256: c256, formatVersion: m.FormatVersion}
	hold := 100
	if m.Framerate > 0 && m.Framerate < 1000 {
		hold = int(math.Round(1000 / m.Framerate))
	}
	for _, fr := range m.Frames {
		d := newDoc(w, h)
		d.uni = true
		for y, line := range fr.Contents {
			if y >= h {
				break
			}
			x := 0
			for _, r := range line {
				if x >= w {
					break
				}
				code, _ := uniCodeOf(r)
				if !unicode.IsPrint(r) { // as durdraw: what is not printable is a space
					code = ' '
				}
				d.cells[y*w+x] = uniCell(code, 7)
				x++
			}
		}
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				var pair []int
				if m.FormatVersion >= 8 {
					if y < len(fr.ColorMap) && x < len(fr.ColorMap[y]) {
						pair = fr.ColorMap[y][x]
					}
				} else if x < len(fr.ColorMap) && y < len(fr.ColorMap[x]) { // version 7: [column][line]
					pair = fr.ColorMap[x][y]
				}
				if len(pair) < 2 {
					continue
				}
				f, g := pair[0], pair[1]
				if n, ok := legacyFg[f]; ok {
					f = n
				}
				if n, ok := legacyBg[g]; ok {
					g = n
				}
				i := y*w + x
				d.cells[i] = uniWithPair(d.cells[i], pairOf(durFg(f, c256), durBg(g, c256)))
			}
		}
		ms := hold
		if fr.Delay > 0 && fr.Delay < 3600 {
			ms = int(math.Round(fr.Delay * 1000))
		}
		art.docs = append(art.docs, d)
		art.holds = append(art.holds, max(1, ms))
	}
	return art, nil
}

// writeDur writes frames as a durdraw file (format 8, gzipped): 16 colours when
// the page uses no more, else 256 (24-bit colours become the nearest of the 256).
func writeDur(docs []*Doc, holds []int, name, artist string) ([]byte, int) {
	w, h := 1, 1
	for _, d := range docs {
		w, h = max(w, d.cols), max(h, d.rows)
	}
	c256, rgb := false, 0
	for _, d := range docs {
		for _, v := range d.cells[:d.rows*d.cols] {
			fg, bg := uniColoursOf(v)
			if fg > 15 || bg > 15 || bg == 8 {
				c256 = true
			}
			if fg&rgbFlag != 0 || bg&rgbFlag != 0 {
				rgb++
			}
		}
	}
	// The commonest hold is the frame rate; the others are delays.
	count := map[int]int{}
	common := 100
	for _, ms := range holds {
		count[ms]++
		if count[ms] > count[common] {
			common = ms
		}
	}
	m := durMovie{FormatVersion: 8, ColorFormat: json.RawMessage(`"16"`), PreferredFont: "fixed", Encoding: "utf-8",
		Name: name, Artist: artist, Framerate: math.Round(1e6/float64(max(1, common))) / 1e3, Columns: w, Lines: h,
		Extra: json.RawMessage("null")}
	if c256 {
		m.ColorFormat = json.RawMessage(`"256"`)
	}
	for i, d := range docs {
		fr := durFrame{FrameNumber: i + 1}
		if i < len(holds) && holds[i] != common {
			fr.Delay = float64(holds[i]) / 1000
		}
		for y := 0; y < h; y++ {
			var line []rune
			var cm [][]int
			for x := 0; x < w; x++ {
				v := d.get(x, y)
				if x >= d.cols || y >= d.rows {
					v = blank
				}
				r := uniRune(uniCode(v))
				if r < 0x20 {
					r = ' '
				}
				line = append(line, r)
				fg, bg := uniColoursOf(v)
				if fg&rgbFlag != 0 {
					fg = nearest256(colourRGB(fg, &vga16))
				}
				if bg&rgbFlag != 0 {
					bg = nearest256(colourRGB(bg, &vga16))
				}
				p := durColours(fg, bg, c256)
				cm = append(cm, p[:])
			}
			fr.Contents = append(fr.Contents, string(line))
			fr.ColorMap = append(fr.ColorMap, cm)
		}
		m.Frames = append(m.Frames, fr)
	}
	js, _ := json.MarshalIndent(map[string]*durMovie{"DurMovie": &m}, "", " ")
	var out bytes.Buffer
	zw, _ := gzip.NewWriterLevel(&out, gzip.BestCompression)
	zw.Write(js)
	zw.Close()
	return out.Bytes(), rgb
}

// ---------------------------------------------------------------- mIRC colour codes

// mircRGB are mIRC's 99 colours (0-15 mIRC's own, 16-98 the extended set as
// modern.ircdocs.horse lists them); mircToAnsi puts 0-15 on the page's 16.
var mircRGB = [99]uint32{
	0xFFFFFF, 0x000000, 0x00007F, 0x009300, 0xFF0000, 0x7F0000, 0x9C009C, 0xFC7F00,
	0xFFFF00, 0x00FC00, 0x009393, 0x00FFFF, 0x0000FC, 0xFF00FF, 0x7F7F7F, 0xD2D2D2,
	0x470000, 0x472100, 0x474700, 0x324700, 0x004700, 0x00472C, 0x004747, 0x002747, 0x000047, 0x2E0047, 0x470047, 0x47002A,
	0x740000, 0x743A00, 0x747400, 0x517400, 0x007400, 0x007449, 0x007474, 0x004074, 0x000074, 0x4B0074, 0x740074, 0x740045,
	0xB50000, 0xB56300, 0xB5B500, 0x7DB500, 0x00B500, 0x00B571, 0x00B5B5, 0x0063B5, 0x0000B5, 0x7500B5, 0xB500B5, 0xB5006B,
	0xFF0000, 0xFF8C00, 0xFFFF00, 0xB2FF00, 0x00FF00, 0x00FFA0, 0x00FFFF, 0x008CFF, 0x0000FF, 0xA500FF, 0xFF00FF, 0xFF0098,
	0xFF5959, 0xFFB459, 0xFFFF71, 0xCFFF60, 0x6FFF6F, 0x65FFC9, 0x6DFFFF, 0x59B4FF, 0x5959FF, 0xC459FF, 0xFF66FF, 0xFF59BC,
	0xFF9C9C, 0xFFD39C, 0xFFFF9C, 0xE2FF9C, 0x9CFF9C, 0x9CFFDB, 0x9CFFFF, 0x9CD3FF, 0x9C9CFF, 0xDC9CFF, 0xFF9CFF, 0xFF94D3,
	0x000000, 0x131313, 0x282828, 0x363636, 0x4D4D4D, 0x656565, 0x818181, 0x9F9F9F, 0xBCBCBC, 0xE2E2E2, 0xFFFFFF,
}

var (
	mircToAnsi = [16]int{15, 0, 4, 2, 9, 1, 5, 3, 11, 10, 6, 14, 12, 13, 8, 7}
	ansiToMirc = func() [16]int {
		var t [16]int
		for m, c := range mircToAnsi {
			t[c] = m
		}
		return t
	}()
	// mircOf is the mIRC number of each page colour that has one exactly.
	mircOf = func() map[int]int {
		m := map[int]int{}
		for c, n := range ansiToMirc {
			m[c] = n
		}
		for n := 98; n >= 16; n-- {
			m[mircColour(n)] = n
		}
		return m
	}()
)

// mircColour is the page colour of mIRC colour n (0-98).
func mircColour(n int) int {
	if n < 16 {
		return mircToAnsi[n]
	}
	return rgbColour(mircRGB[n])
}

// mircNumber is the mIRC number for a page colour: its own, or the nearest of the 99.
func mircNumber(c int, pal *[16]uint32) (int, bool) {
	if n, ok := mircOf[c]; ok {
		return n, true
	}
	rgb := colourRGB(c, pal)
	best, bd := 0, -1
	for n, m := range mircRGB {
		if d := rgbDist(rgb, m); bd < 0 || d < bd {
			best, bd = n, d
		}
	}
	return best, false
}

var mircCode = regexp.MustCompile(`\x03\d`)

// looksMirc: text with mIRC colour codes and no ANSI escapes.
func looksMirc(b []byte) bool {
	return len(mircCode.FindAllIndex(b[:min(len(b), 1<<16)], 3)) >= 2 && !bytes.Contains(b, []byte("\x1b[")) && !bytes.ContainsRune(b, 0)
}

// parseMirc reads text with mIRC's codes: ^C fg[,bg] (0-98, 99 the default),
// ^D RRGGBB[,RRGGBB], ^B bold (no colour change), ^] italic, ^_ underline,
// ^V reverse, ^O plain. The page is as wide as the longest line.
func parseMirc(b []byte) *Doc {
	var text string
	if utf8.Valid(b) {
		text = string(b)
	} else { // mIRC's own code page: Latin-1
		rs := make([]rune, len(b))
		for i, c := range b {
			rs[i] = rune(c)
		}
		text = string(rs)
	}
	text = strings.ReplaceAll(text, "\r\n", "\n")
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	type cell struct {
		code   int
		fg, bg int
		it, ul bool
	}
	var rows [][]cell
	cols := 1
	for _, l := range lines {
		if len(rows) >= maxRowsFor(cols) {
			break
		}
		fg, bg := -1, -1
		it, ul, rev := false, false, false
		var row []cell
		rs := []rune(l)
		num := func(i, most int) (int, int) { // up to most digits at rs[i:]
			v, n := 0, 0
			for n < most && i+n < len(rs) && rs[i+n] >= '0' && rs[i+n] <= '9' {
				v = v*10 + int(rs[i+n]-'0')
				n++
			}
			return v, n
		}
		hexAt := func(i int) (uint32, bool) {
			if i+6 > len(rs) {
				return 0, false
			}
			v, err := strconv.ParseUint(string(rs[i:i+6]), 16, 32)
			return uint32(v), err == nil
		}
		for i := 0; i < len(rs); i++ {
			r := rs[i]
			switch r {
			case 0x03:
				f, n := num(i+1, 2)
				if n == 0 {
					fg, bg = -1, -1
					continue
				}
				i += n
				fg = f
				if i+2 < len(rs) && rs[i+1] == ',' && rs[i+2] >= '0' && rs[i+2] <= '9' {
					g, m := num(i+2, 2)
					bg = g
					i += 1 + m
				}
				continue
			case 0x04:
				v, ok := hexAt(i + 1)
				if !ok {
					fg, bg = -1, -1
					continue
				}
				fg = 1000 + int(v) // 1000+: an RGB colour, below
				i += 6
				if i+1 < len(rs) && rs[i+1] == ',' {
					if g, ok := hexAt(i + 2); ok {
						bg = 1000 + int(g)
						i += 7
					}
				}
				continue
			case 0x02, 0x1E, 0x11: // bold, strike-through, monospace: no colour
				continue
			case 0x1D:
				it = !it
				continue
			case 0x1F:
				ul = !ul
				continue
			case 0x16:
				rev = !rev
				continue
			case 0x0F:
				fg, bg, it, ul, rev = -1, -1, false, false, false
				continue
			}
			if r < 0x20 || r == 0x7F {
				continue
			}
			colour := func(n, def int) int {
				switch {
				case n >= 1000:
					return rgbColour(uint32(n - 1000))
				case n >= 0 && n <= 98:
					return mircColour(n)
				}
				return def
			}
			f, g := colour(fg, 7), colour(bg, 0)
			if rev {
				f, g = g, f
			}
			code, _ := uniCodeOf(r)
			row = append(row, cell{code, f, g, it, ul})
		}
		cols = min(max(cols, len(row)), 4096)
		rows = append(rows, row)
	}
	d := newDoc(cols, max(1, min(len(rows), maxRowsFor(cols))))
	d.uni = true
	for y, row := range rows {
		if y >= d.rows {
			break
		}
		for x, c := range row {
			if x >= cols {
				break
			}
			var fl uint32
			if c.it {
				fl |= italic
			}
			if c.ul {
				fl |= underline
			}
			d.cells[y*cols+x] = uniCell(c.code, styleOf(c.fg, c.bg, fl))
		}
	}
	return d
}

// ircLineMax is the text an IRC line carries: 512 bytes less the command
// ("PRIVMSG #channel :"), the line end and the sender's address the server adds.
const ircLineMax = 450

// writeMirc writes a Unicode page with mIRC's codes, a line per row (every line
// starts plain on IRC: the colours are set again). It returns the bytes and the
// longest line's length. Colours mIRC lacks become the nearest of its 99.
func writeMirc(d *Doc, pal *[16]uint32) ([]byte, int, int) {
	var out bytes.Buffer
	longest, approx := 0, 0
	for y := 0; y < d.rows; y++ {
		start := out.Len()
		end := d.cols
		for end > 0 && uniBlank(d.get(end-1, y)) {
			end--
		}
		curF, curB := -1, -1
		it, ul := false, false
		for x := 0; x < end; x++ {
			v := d.get(x, y)
			fg, bg := uniColoursOf(v)
			f, okf := mircNumber(fg, pal)
			g, okg := mircNumber(bg, pal)
			if !okf || !okg {
				approx++
			}
			r := uniRune(uniCode(v))
			if r < 0x20 || r == 0x7F {
				r = ' '
			}
			fl := uniFlagsOf(v)
			if wantIt := fl&italic != 0; wantIt != it {
				out.WriteByte(0x1D)
				it = wantIt
			}
			if wantUl := fl&underline != 0; wantUl != ul {
				out.WriteByte(0x1F)
				ul = wantUl
			}
			if r == ' ' && !ul && curF >= 0 {
				f = curF // a space shows only its background
			}
			if f != curF || g != curB {
				next := r // the character right after the code
				out.WriteByte(0x03)
				withBg := g != curB || next == ','
				digits := func(n int, more bool) string { // two digits when a digit follows
					if n < 10 && !more {
						return strconv.Itoa(n)
					}
					return fmt.Sprintf("%02d", n)
				}
				isDigit := next >= '0' && next <= '9'
				out.WriteString(digits(f, isDigit || withBg))
				if withBg {
					out.WriteString("," + digits(g, isDigit))
				}
				curF, curB = f, g
			}
			out.WriteRune(r)
		}
		if it {
			out.WriteByte(0x1D)
		}
		if ul {
			out.WriteByte(0x1F)
		}
		longest = max(longest, out.Len()-start)
		out.WriteByte('\n')
	}
	return out.Bytes(), longest, approx
}

// ---------------------------------------------------------------- a web page

// writeUniHTML gives a Unicode page as a web page: the characters as text in
// spans of their colours (the browser's monospace font draws them).
func writeUniHTML(d *Doc, pal *[16]uint32, title string) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "<!doctype html>\n<html><head><meta charset=\"utf-8\"><title>%s</title>\n", html.EscapeString(title))
	b.WriteString("<style>body{background:#000;margin:0}pre{margin:1em;font:16px/1 'DejaVu Sans Mono',Menlo,Consolas,monospace;letter-spacing:0}" +
		"pre span{white-space:pre}</style></head><body><pre>")
	for y := 0; y < d.rows; y++ {
		end := d.cols
		for end > 0 && uniBlank(d.get(end-1, y)) {
			end--
		}
		style := ""
		open := false
		for x := 0; x < end; x++ {
			v := d.get(x, y)
			fg, bg := uniColoursOf(v)
			s := fmt.Sprintf("color:#%06x;background:#%06x", colourRGB(fg, pal), colourRGB(bg, pal))
			fl := uniFlagsOf(v)
			if fl&italic != 0 {
				s += ";font-style:italic"
			}
			if fl&underline != 0 {
				s += ";text-decoration:underline"
			}
			if s != style {
				if open {
					b.WriteString("</span>")
				}
				b.WriteString(`<span style="` + s + `">`)
				style, open = s, true
			}
			r := uniRune(uniCode(v))
			if r < 0x20 {
				r = ' '
			}
			b.WriteString(html.EscapeString(string(r)))
		}
		if open {
			b.WriteString("</span>")
		}
		b.WriteByte('\n')
	}
	b.WriteString("</pre></body></html>\n")
	return []byte(b.String())
}

// ---------------------------------------------------------------- opening and saving

// needsUni: a UTF-8 file of art a PC or Amiga page cannot show as it is
// (characters CP437 lacks, or colours beyond the 16).
var extColours = regexp.MustCompile(`\x1b\[[0-9;]*(?:[34]8;5;(?:1[6-9]|[2-9][0-9]|1[0-9][0-9]|2[0-5][0-9])|[34]8;2;)`)

func needsUni(b []byte) bool {
	if !utf8.Valid(b) {
		return false
	}
	if extColours.Match(b) {
		return true
	}
	for _, r := range string(b) {
		if r > 0xFF {
			if _, ok := cp437Rev[r]; !ok {
				return true
			}
		}
	}
	return false
}

// loadUni opens a file on a Unicode page: a durdraw file, mIRC text, or UTF-8
// ANSI that needs one; with Open as Unicode, any ANSI. False: not for it.
func (a *App) loadUni(b []byte, name, path, as string) bool {
	if as != "auto" && as != "uni" {
		return false
	}
	lname := strings.ToLower(name)
	sauce, end := readSauce(b)
	kind := ""
	switch {
	case isDur(b) || strings.HasSuffix(lname, ".dur"):
		kind = "dur"
	case strings.HasSuffix(lname, ".irc") || (sauce == nil && looksMirc(b) && bbsCodesKind(b, name) != "wwiv"):
		// (WWIV's hearts are Ctrl-C too: a heart takes one digit, mIRC's mostly fg,bg; bbscodes.go decides)
		kind = "mirc"
	case as == "uni":
		kind = "uniansi"
	case (sauce == nil || sauce.dataType <= 1) && utf8.Valid(b[:end]) && strings.HasSuffix(lname, ".utf8.ans"):
		kind = "uniansi" // durdraw's name for UTF-8 ANSI
	case (sauce == nil || sauce.dataType <= 1) && needsUni(b[:end]):
		kind = "uniansi"
	}
	if kind == "" {
		return false
	}
	a.uniCompact() // a safe point to let go of colours no page uses any more
	note := ""
	switch kind {
	case "dur":
		art, err := parseDur(b)
		if err != nil {
			a.msg("Could not read " + name + ": " + err.Error())
			return true
		}
		if art.colours256 {
			note = "256 colours"
		} else {
			note = "16 colours"
		}
	case "mirc":
		sauce = nil
		a.paletteFor["uni"] = "mirc" // the 16 as mIRC shows them (its orange, its greys)
	}
	a.mode, a.srcKind, a.ice = "uni", kind, true
	if kind != "uniansi" {
		sauce = nil
	}
	a.finishLoad(b, name, path, sauce, kind)
	if kind == "dur" {
		if art, err := parseDur(b); err == nil {
			a.info.title, a.info.author = art.name, art.artist
		}
	}
	if kind == "uniansi" {
		a.saveFmt = "uniansi"
	}
	msg := name + ": a Unicode page"
	if note != "" {
		msg += " (" + note + ")"
	}
	if n := a.nFrames(); n > 1 {
		msg += fmt.Sprintf(", %d frames (%s and %s go through them)", n, keyName("frame_previous"), keyName("frame_next"))
	}
	a.msg(msg + uniFullNote())
	return true
}

// reparseUni reads the page again from its file; false if it is no Unicode page's file.
func (a *App) reparseUni() bool {
	if !a.uni() {
		return false
	}
	switch a.srcKind {
	case "dur":
		art, err := parseDur(a.source)
		if err != nil {
			a.doc = newDoc(80, 25)
			break
		}
		if len(art.docs) == 1 {
			a.doc = art.docs[0]
		} else {
			a.frames = make([]Frame, len(art.docs))
			for i, d := range art.docs {
				a.frames[i] = Frame{doc: d, hold: art.holds[i], lower: -1, src: -1}
			}
			a.doc = a.frames[0].doc.clone()
		}
	case "mirc":
		a.doc = parseMirc(a.source)
	default:
		a.doc, _ = parseUniArt(a.source, a.forceCols)
	}
	if n := a.forceCols; n > 0 && a.doc.cols != n { // the W field on a file that has its own width
		a.doc = a.doc.withCols(n)
		for i := range a.frames {
			a.frames[i].doc = a.frames[i].doc.withCols(n)
		}
	}
	a.cx, a.cy, a.sel, a.float = 0, 0, nil, nil
	return true
}

// encodeUni writes the Unicode page's formats, and mIRC from a PC or Amiga page;
// false: not one of them.
func (a *App) encodeUni(fmtID string, d *Doc) ([]byte, bool, error) {
	switch {
	case fmtID == "mirc":
		if !d.uni && !a.uni() {
			d = docToUni(d, a.amiga())
			if !a.ice {
				uniDropBlink(d)
			}
		}
		out, longest, approx := writeMirc(d, &a.pal)
		switch {
		case longest > ircLineMax:
			a.msg(fmt.Sprintf("Saved, but the longest line is %d bytes: IRC carries about %d (512 with the command and the sender), so it would be cut. Make the page narrower or use fewer colour changes", longest, ircLineMax))
		case approx > 0:
			a.msg(fmt.Sprintf("%d cells have colours mIRC lacks: saved as the nearest of its 99", approx))
		}
		return out, true, nil
	case !a.uni():
		return nil, false, nil
	case fmtID == "uniansi":
		out := writeUniAnsi(d)
		info := a.info
		if !info.writeSauce && d.cols < 80 {
			// A wider page shows its width by its lines (writeUniAnsi writes one in
			// full); readers take a narrower one for 80 columns without the record.
			info.writeSauce, a.info.writeSauce = true, true
			a.msg(fmt.Sprintf("Saved with a SAUCE record: it is the only way to keep a width of %d", d.cols))
		}
		if info.writeSauce {
			out = append(out, makeSauce(SauceInfo{title: info.title, author: info.author, group: info.group, notes: info.notes,
				cols: d.cols, rows: max(1, d.rows), fileType: 1, utf8: true}, len(out))...)
		}
		return out, true, nil
	case fmtID == "dur":
		holds := make([]int, a.nFrames())
		for i := range holds {
			holds[i] = a.holdOf(i)
		}
		out, rgb := writeDur(a.allDocs(), holds, a.info.title, a.info.author)
		if rgb > 0 {
			a.msg(fmt.Sprintf("durdraw has no 24-bit colours: %d cells are saved in the nearest of the 256", rgb))
		}
		return out, true, nil
	case fmtID == "html":
		return writeUniHTML(d, &a.pal, a.name), true, nil
	case fmtID == "utf8":
		return []byte(strings.Join(docText(d, a.runeOf, 0, 0, 0, 0), "\n") + "\n"), true, nil
	case fmtID == "ansi" || fmtID == "text":
		return nil, true, errors.New("a Unicode page saves as UTF-8 ANSI (" + map[bool]string{true: "ANSI", false: "text"}[fmtID == "ansi"] + " is for PC and Amiga pages)")
	}
	return nil, false, nil
}
