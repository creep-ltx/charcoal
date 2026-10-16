package main

// Viewdata pages (Prestel and the Viewdata boards of today, Telstar, CCl4,
// NXtel): a teletext page of 24 rows. The characters and attributes are
// teletext's (teletext.go: the cells hold the codes, attributes take a cell
// and act to their right), only the files differ:
//   - the stream a Viewdata terminal is sent (.cpt, as the Amiga's vtx datatype
//     names it; .car a carousel of them): CS clears, attributes come as ESC and
//     code+0x40 (or 0x80+code), a row shorter than 40 ends with CR LF;
//   - a frame of 960 bytes (.bin, 40 x 24 codes; attributes as 0x00-0x1F,
//     0x80-0x9F or MacViewdata's 0xC0-0xDF);
//   - a Telstar frame (.json): its "content" holds the page (rawV, rawT or an
//     edit.tf link); saving keeps every other field of the frame.

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
)

func init() {
	pageModes = append(pageModes, "vd")
	modeOrder = append(modeOrder, "vd")
	modeNames["vd"] = "Viewdata"
	openAs = append(openAs, "vd")
	openAsNames = append(openAsNames, "Viewdata")
	modeNoStream["vd"] = "a Viewdata page: a picture of the screen, no stream to draw in order"
	mirrorPairs["vd"] = asciiPairs
	formats = append(formats,
		Format{"vdcpt", "Viewdata stream (.cpt)", ".cpt", "vd"},
		Format{"vdbin", "Viewdata frame (.bin, 960 bytes)", ".bin", "vd"},
		Format{"vdtel", "Telstar frame (.json)", ".json", "vd"})
	for i, f := range formats {
		switch f.id {
		case "png", "png2", "gif", "apng", "utf8", "tturl", "tti":
			formats[i].modes += " vd"
		}
	}
}

// vd: a Viewdata page (a.tt() is true on it too: everything teletext does, it does).
func (a *App) vd() bool { return a.mode == "vd" }

const vdRows = 24

func newVDDoc() *Doc {
	d := newTTDoc()
	d.rows = vdRows
	d.cells = d.cells[:40*vdRows]
	return d
}

// vdScreen is a Viewdata terminal's screen, enough to read pages.
type vdScreen struct {
	d      *Doc
	x, y   int
	frames []*Doc // the screens a CS ended
	drawn  bool
}

func newVDScreen() *vdScreen { return &vdScreen{d: newVDDoc()} }

func (s *vdScreen) put(c int) {
	s.d.cells[s.y*40+s.x] = makeCell(c&0x7F, 0, 0, 0)
	s.drawn = true
	if s.x++; s.x >= 40 {
		s.x = 0
		s.down()
	}
}

func (s *vdScreen) down() {
	if s.y++; s.y >= vdRows {
		s.y = 0
	}
}

// feed runs a stream: CS (0x0C) clears, APH (0x1E) goes home, BS HT LF VT CR
// move, ESC and 0x40-0x5F is an attribute, so is 0x80-0x9F.
func (s *vdScreen) feed(b []byte) {
	esc := false
	for _, c := range b {
		switch {
		case esc:
			esc = false
			if c >= 0x40 && c <= 0x5F {
				s.put(int(c) - 0x40)
			}
		case c >= 0x80 && c <= 0x9F:
			s.put(int(c) - 0x80)
		case c >= 0x20:
			s.put(int(c & 0x7F))
		case c == 0x1B:
			esc = true
		case c == 0x0C:
			if s.drawn && len(s.frames) < maxSplitFrames {
				s.frames = append(s.frames, s.d.clone())
			}
			s.d, s.x, s.y, s.drawn = newVDDoc(), 0, 0, false
		case c == 0x1E:
			s.x, s.y = 0, 0
		case c == 0x08:
			if s.x--; s.x < 0 {
				s.x = 39
				if s.y--; s.y < 0 {
					s.y = vdRows - 1
				}
			}
		case c == 0x09:
			if s.x++; s.x >= 40 {
				s.x = 0
				s.down()
			}
		case c == 0x0A:
			s.down()
		case c == 0x0B:
			if s.y--; s.y < 0 {
				s.y = vdRows - 1
			}
		case c == 0x0D:
			s.x = 0
		}
	}
}

// parseVDStream reads a Viewdata stream: its pages, each CS starting one.
func parseVDStream(b []byte) []*Doc {
	s := newVDScreen()
	s.feed(b)
	out := s.frames
	if s.drawn || len(out) == 0 {
		out = append(out, s.d)
	}
	return out
}

// writeVDStream gives the stream of a page: rows with their trailing spaces
// left out and CR LF after them (a full row wraps by itself), attributes as
// ESC and code+0x40, 7 bits. clear puts CS first.
func writeVDStream(d *Doc, clear bool) []byte {
	var out []byte
	if clear {
		out = append(out, 0x0C)
	}
	rows := min(d.rows, vdRows)
	last := rows - 1
	for last >= 0 && vdRowEnd(d, last) < 0 {
		last--
	}
	for y := 0; y <= last; y++ {
		end := vdRowEnd(d, y)
		for x := 0; x <= end; x++ {
			c := byte(cellCh(d.get(x, y)) & 0x7F)
			if c < 0x20 {
				out = append(out, 0x1B, c+0x40)
			} else {
				out = append(out, c)
			}
		}
		if end < 39 && y < last {
			out = append(out, 0x0D, 0x0A)
		}
	}
	return out
}

// vdRowEnd is the last column of row y that is not a space, -1 for none.
func vdRowEnd(d *Doc, y int) int {
	for x := d.cols - 1; x >= 0; x-- {
		if cellCh(d.get(x, y))&0x7F != 0x20 {
			return x
		}
	}
	return -1
}

// isVDBin: a 960-byte frame (40 x 24) of codes, some of them attributes. A
// PC .bin of 960 bytes has an attribute in every other byte (often above 0x9F).
func isVDBin(b []byte, name string) bool {
	if len(b) != 960 || !strings.HasSuffix(strings.ToLower(name), ".bin") {
		return false
	}
	ctl := 0
	for _, c := range b {
		switch {
		case c >= 0xA0 && c < 0xC0 || c >= 0xE0:
			return false
		case c&0x7F < 0x20 || c >= 0xC0:
			ctl++
		}
	}
	return ctl >= 1 && ctl < 300
}

// parseVDBin reads a 960-byte frame: codes 0x00-0x1F, or with the top bit
// (0x80-0x9F, BBC style), or 0xC0-0xDF (MacViewdata: ESC's letter with the top bit).
func parseVDBin(b []byte) *Doc {
	d := newVDDoc()
	for i := 0; i < 960 && i < len(b); i++ {
		c := int(b[i])
		switch {
		case c >= 0xC0 && c <= 0xDF:
			c -= 0xC0
		default:
			c &= 0x7F
		}
		d.cells[i] = makeCell(c, 0, 0, 0)
	}
	return d
}

// writeVDBin gives the 960 bytes, attributes with the top bit (0x80-0x9F):
// a CS byte (0x0C) never appears, so frames can be joined into a carousel.
func writeVDBin(d *Doc) []byte {
	out := make([]byte, 960)
	for i := range out {
		c := byte(cellCh(d.get(i%40, i/40)) & 0x7F)
		if c < 0x20 {
			c |= 0x80
		}
		out[i] = c
	}
	return out
}

// isTelstar: a Telstar frame (JSON with a "pid" and a "content").
func isTelstar(b []byte) bool {
	t := bytes.TrimSpace(b[:min(len(b), 1<<20)])
	if len(t) == 0 || t[0] != '{' || !bytes.Contains(t, []byte(`"pid"`)) || !bytes.Contains(t, []byte(`"content"`)) {
		return false
	}
	var f struct {
		Content *struct {
			Type string `json:"type"`
		} `json:"content"`
	}
	return json.Unmarshal(b, &f) == nil && f.Content != nil
}

// parseTelstar reads a Telstar frame's page; note says what could not be read.
func parseTelstar(b []byte) (d *Doc, note string) {
	var f struct {
		Content struct {
			Data string `json:"data"`
			Type string `json:"type"`
		} `json:"content"`
	}
	if err := json.Unmarshal(b, &f); err != nil {
		return newVDDoc(), "not a Telstar frame: " + err.Error()
	}
	data := []byte(f.Content.Data)
	switch strings.ToLower(f.Content.Type) {
	case "rawv", "raw", "":
		return parseVDStream(data)[0], ""
	case "rawt":
		d := newVDDoc()
		for i := 0; i < 960 && i < len(data); i++ {
			d.cells[i] = makeCell(int(data[i]&0x7F), 0, 0, 0)
		}
		return d, ""
	case "edit.tf", "edittf":
		if t, _ := parseEditTFMeta(data); t != nil {
			d := newVDDoc()
			copy(d.cells, t.cells[:960])
			return d, ""
		}
		return newVDDoc(), "its edit.tf link could not be read"
	}
	return newVDDoc(), "content of type " + f.Content.Type + " is not read (Telstar markup)"
}

var telstarName = regexp.MustCompile(`^(\d+)([a-z])\.json$`)

// writeTelstar gives the page as a Telstar frame: the old frame (if the page
// came from one) with its content replaced by the page as rawV, else a new
// information frame numbered by the file's name (e.g. 200a.json: page 200, frame a).
func writeTelstar(d *Doc, old []byte, name string) ([]byte, error) {
	frame := map[string]any{}
	if old != nil && isTelstar(old) {
		if err := json.Unmarshal(old, &frame); err != nil {
			return nil, err
		}
	} else {
		page, id := 0, "a"
		if m := telstarName.FindStringSubmatch(strings.ToLower(name)); m != nil {
			page, _ = strconv.Atoi(m[1])
			id = m[2]
		}
		frame["pid"] = map[string]any{"page-no": page, "frame-id": id}
		frame["visible"] = true
		frame["frame-type"] = "information"
	}
	frame["content"] = map[string]any{"data": string(writeVDStream(d, false)), "type": "rawV"}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "    ")
	if err := enc.Encode(frame); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
