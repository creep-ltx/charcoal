package main

// More formats: a C64 program that shows the screen (.prg), the C array of
// Marq's PETSCII editor and Petmate (.c), XBin with its font and palette (.xb),
// and a web page (.html).

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"
)

// ---------------------------------------------------------------- C64 screens

// c64Screen gives the first 40x25 cells as screen codes and colours.
func c64Screen(d *Doc) (chars, cols []byte) {
	chars, cols = make([]byte, 1000), make([]byte, 1000)
	for y := 0; y < 25; y++ {
		for x := 0; x < 40; x++ {
			v := d.get(x, y)
			chars[y*40+x], cols[y*40+x] = byte(cellCh(v)), byte(cellFg(v))
		}
	}
	return
}

func (d *Doc) borderColour() int {
	if d.border < 0 {
		return d.bg
	}
	return d.border
}

// petmatePrg is the start of Petmate's executable .prg template (assets/template.prg,
// github.com/nurpax/petmate, MIT licence: third_party/petmate-LICENSE.txt): a BASIC
// line "SYS2061" and code that puts up the screen and stays there. Marq's PETSCII
// editor and Petmate open .prg files of exactly this form, so CharEdit writes it too.
// The border, background and character set are the values before STA $D020,
// STA $D021 and STA $D018; 1000 screen codes follow at 0x62, then 1000 colours.
var petmatePrg = []byte{0x01, 0x08, 0x0B, 0x08, 0xEF, 0x00, 0x9E, 0x32, 0x30, 0x36, 0x31, 0x00, 0x00, 0x00, 0xA9, 0x0B, 0x8D, 0x11, 0xD0, 0xA9, 0x14, 0x8D, 0x18, 0xD0, 0xA9, 0x00, 0x8D, 0x20, 0xD0, 0xA9, 0x00, 0x8D, 0x21, 0xD0, 0xA2, 0x00, 0xA0, 0xFA, 0xBD, 0x61, 0x08, 0x9D, 0x00, 0x04, 0xBD, 0x5B, 0x09, 0x9D, 0xFA, 0x04, 0xBD, 0x55, 0x0A, 0x9D, 0xF4, 0x05, 0xBD, 0x4F, 0x0B, 0x9D, 0xEE, 0x06, 0xBD, 0x49, 0x0C, 0x9D, 0x00, 0xD8, 0xBD, 0x43, 0x0D, 0x9D, 0xFA, 0xD8, 0xBD, 0x3D, 0x0E, 0x9D, 0xF4, 0xD9, 0xBD, 0x37, 0x0F, 0x9D, 0xEE, 0xDA, 0xE8, 0x88, 0xD0, 0xCC, 0xA9, 0x1B, 0x8D, 0x11, 0xD0, 0x4C, 0x5E, 0x08}

const (
	prgCharset = 20 // offsets in the file
	prgBorder  = 25
	prgBg      = 30
	prgScreen  = 0x62
)

// writePrg gives a C64 program that shows the first 40x25 cells, in Petmate's form.
func writePrg(d *Doc, lower bool) []byte {
	chars, cols := c64Screen(d)
	out := append([]byte{}, petmatePrg...)
	out[prgBorder], out[prgBg] = byte(d.borderColour()), byte(d.bg)
	out[prgCharset] = 0x14
	if lower {
		out[prgCharset] = 0x17
	}
	out = append(out, chars...)
	return append(out, cols...)
}

// marqMachines are the machines Marq's PETSCII editor names in its META line,
// and the Commodore machine page each opens as (Plus/4 and the C64 modes as a C64).
var marqMachines = map[string]string{"C64": "c64", "C64FLICKER": "c64", "DIRART": "c64", "PET": "pet",
	"PETHI": "pet80", "PLUS4": "c64", "VIC20": "vic20"}

// marqName is the META name for a machine page; PETs have no colours in the file.
func marqName(machine string) (name string, colours bool) {
	switch machine {
	case "pet":
		return "PET", false
	case "pet80":
		return "PETHI", false
	case "vic20":
		return "VIC20", true
	case "plus4":
		return "PLUS4", true
	}
	return "C64", true
}

// writeMarq gives the C array Marq's PETSCII editor and Petmate read, at the page's own size,
// for the page's machine.
func writeMarq(d *Doc, lower bool, machine string) []byte {
	return writeMarqFrames([]*Doc{d}, lower, machine)
}

// writeMarqFrames gives the C array of several screens, as Petmate writes them: frame0000,
// frame0001 ... in order and one META line at the end. Marq's format has one size for all
// screens: the widest and tallest; smaller screens are padded with empty cells.
func writeMarqFrames(docs []*Doc, lower bool, machine string) []byte {
	w, h := 1, 1
	for _, d := range docs {
		w, h = max(w, d.cols), max(h, d.rows)
	}
	name, colours := marqName(machine)
	var b strings.Builder
	for i, d := range docs {
		parts := 1
		if colours {
			parts = 2
			fmt.Fprintf(&b, "unsigned char frame%04d[]={// border,bg,chars,colors\n", i)
			fmt.Fprintf(&b, "%d,%d,\n", d.borderColour(), d.bg) // a Plus/4's: whole TED colours (ted.go)
		} else {
			fmt.Fprintf(&b, "unsigned char frame%04d[]={\n", i)
		}
		for part := 0; part < parts; part++ {
			for y := 0; y < h; y++ {
				for x := 0; x < w; x++ {
					v := d.get(x, y)
					switch {
					case part == 0:
						fmt.Fprintf(&b, "%d,", cellCh(v))
					case machine == "plus4": // hue and luminance; the editor has no flash
						fmt.Fprintf(&b, "%d,", tedAttr(v)&127)
					default:
						fmt.Fprintf(&b, "%d,", cellFg(v))
					}
				}
				b.WriteString("\n")
			}
		}
		b.WriteString("};\n")
	}
	set := "upper"
	if lower {
		set = "lower"
	}
	fmt.Fprintf(&b, "// META: %d %d %s %s\n", w, h, name, set)
	return []byte(b.String())
}

var (
	marqNumbers  = regexp.MustCompile(`\d+`)
	marqMeta     = regexp.MustCompile(`// META:\s*(\d+) (\d+) (\S+) (upper|lower)`)
	marqArrays   = regexp.MustCompile(`(?s)\{(.*?)\}`)
	marqComments = regexp.MustCompile(`//[^\n]*`)
)

func isMarq(b []byte) bool {
	return bytes.Contains(b[:min(len(b), 400)], []byte("unsigned char")) && bytes.Contains(b, []byte("{"))
}

// parseMarq reads the frames of a PETSCII editor / Petmate C array.
func parseMarq(b []byte) ([]PetScreen, error) {
	s := string(b)
	lower, w, h, machine := -1, 40, 25, ""
	if m := marqMeta.FindStringSubmatch(s); m != nil {
		w, _ = strconv.Atoi(m[1])
		h, _ = strconv.Atoi(m[2])
		machine = marqMachines[strings.ToUpper(m[3])]
		lower = map[string]int{"upper": 0, "lower": 1}[m[4]]
		w, h = max(1, min(w, 1000)), max(1, min(h, 10000))
	}
	var out []PetScreen
	for _, m := range marqArrays.FindAllStringSubmatch(s, -1) {
		body := marqComments.ReplaceAllString(m[1], "")
		var nums []int
		for _, n := range marqNumbers.FindAllString(body, -1) {
			v, _ := strconv.Atoi(n)
			nums = append(nums, v)
		}
		d := newDoc(w, h)
		switch {
		case len(nums) >= 2+2*w*h && machine == "plus4": // whole TED colours (ted.go)
			d.border, d.bg = nums[0]&127, nums[1]&127
			for i := 0; i < w*h; i++ {
				d.cells[i] = tedCell(nums[2+i]&255, nums[2+w*h+i]&255)
			}
		case len(nums) >= 2+2*w*h: // border, background, characters, colours
			d.border, d.bg = nums[0]&15, nums[1]&15
			for i := 0; i < w*h; i++ {
				d.cells[i] = makeCell(nums[2+i]&255, nums[2+w*h+i]&15, 0, 0)
			}
		case len(nums) >= w*h: // a PET's: the characters only
			d.border, d.bg = 0, 0
			for i := 0; i < w*h; i++ {
				d.cells[i] = makeCell(nums[i]&255, 1, 0, 0)
			}
		default:
			continue
		}
		out = append(out, PetScreen{doc: d, lower: lower, machine: machine})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no %dx%d screen in the array", w, h)
	}
	return out, nil
}

// ---------------------------------------------------------------- XBin

// maxXBinRows: an XBin stores its height in 16 bits.
const maxXBinRows = 65535

// xbinSaveError says why a page cannot be saved as an XBin (nil if it can).
func xbinSaveError(d *Doc) error {
	if d.rows > maxXBinRows || d.cols > 65535 {
		return fmt.Errorf("an XBin can be at most %d rows tall (this page is %d)", maxXBinRows, d.rows)
	}
	return nil
}

// writeXBin gives an XBin with the palette and the font in it, so it looks the same anywhere.
// A page taller than an XBin can be (see xbinSaveError) is cut at maxXBinRows.
func writeXBin(d *Doc, pal [16]uint32, f *Font, cellH int, ice bool) []byte {
	rows := max(1, min(d.rows, maxXBinRows))
	if cellH < 1 || cellH > 16 {
		cellH = 16 // the fonts are kept as 8x16: 8 rows (every other one), 9-16 rows as they are
	}
	out := []byte{'X', 'B', 'I', 'N', 0x1A, byte(d.cols), byte(d.cols >> 8), byte(rows), byte(rows >> 8), byte(cellH)}
	flags := byte(1 | 2) // palette, font
	if ice {
		flags |= 8
	}
	out = append(out, flags)
	for _, c := range biosPalette(pal) { // the file's palette is in BIOS order
		out = append(out, byte(c>>16&255)>>2, byte(c>>8&255)>>2, byte(c&255)>>2)
	}
	skip := 1
	if cellH == 8 {
		skip = 2
	}
	for c := 0; c < 256; c++ {
		for r := 0; r < cellH; r++ {
			if i := c*16 + r*skip; f != nil && i < len(f.bits) {
				out = append(out, f.bits[i])
			} else {
				out = append(out, 0)
			}
		}
	}
	for y := 0; y < rows; y++ {
		for x := 0; x < d.cols; x++ {
			v := d.get(x, y)
			out = append(out, byte(cellCh(v)), cellAttr(v))
		}
	}
	return out
}

type XBinInfo struct {
	pal   *[16]uint32
	font  *Font
	ice   bool
	cellH int
}

func isXBin(b []byte) bool { return len(b) >= 11 && string(b[:5]) == "XBIN\x1A" }

// parseXBin reads an XBin, compressed or not.
func parseXBin(b []byte) (*Doc, XBinInfo, error) {
	var info XBinInfo
	if !isXBin(b) {
		return nil, info, fmt.Errorf("not an XBin")
	}
	w, h := int(b[5])|int(b[6])<<8, int(b[7])|int(b[8])<<8
	fh, flags := int(b[9]), b[10]
	if flags&2 == 0 && (fh < 1 || fh > 32) {
		fh = 16 // no font in the file: the height byte means nothing
	}
	if w < 1 || w > 1000 || fh < 1 || fh > 32 {
		return nil, info, fmt.Errorf("an XBin of %d columns and %d-pixel font is not one I can show", w, fh)
	}
	p := 11
	if flags&1 != 0 {
		if len(b) < p+48 {
			return nil, info, fmt.Errorf("the XBin is cut short")
		}
		var pal [16]uint32
		for i := range pal {
			r, g, bl := uint32(b[p]&63), uint32(b[p+1]&63), uint32(b[p+2]&63)
			pal[i] = 0xFF000000 | (r<<2|r>>4)<<16 | (g<<2|g>>4)<<8 | (bl<<2 | bl>>4)
			p += 3
		}
		pal = biosPalette(pal)
		info.pal = &pal
	}
	if flags&2 != 0 {
		n := 256
		if flags&16 != 0 {
			n = 512
		}
		if len(b) < p+n*fh {
			return nil, info, fmt.Errorf("the XBin is cut short")
		}
		// Kept as an 8x16 font: 8 rows doubled, 9-16 rows as they are (h), taller ones fitted into 16.
		bits := make([]byte, 4096)
		for c := 0; c < 256; c++ {
			for r := 0; r < 16; r++ {
				src := r * fh / 16
				if fh > 8 && fh <= 16 {
					if r >= fh {
						continue
					}
					src = r
				}
				bits[c*16+r] = b[p+c*fh+src]
			}
		}
		info.font = &Font{id: "xbin", name: "The file's own font", kind: "pc", syncterm: -1, bits: bits, format: "xbin"}
		info.cellH = min(16, max(8, fh))
		if fh < 8 {
			info.cellH = 8
		}
		info.font.h = info.cellH
		p += n * fh
	}
	info.ice = flags&8 != 0
	// The header's height is not trusted for memory: the cells are what the data holds,
	// up to that height and the largest page.
	h = min(h, maxRowsFor(w))
	want := w * h * 2
	var cells []byte
	if flags&4 == 0 {
		cells = b[min(p, len(b)):min(len(b), p+want)]
	} else {
		cells = make([]byte, 0, min(want, 64*2*(len(b)-p)+2))
		for p < len(b) && len(cells) < want {
			k, n := b[p]>>6, int(b[p]&63)+1
			p++
			switch k {
			case 0:
				for i := 0; i < n && p+1 < len(b); i++ {
					cells = append(cells, b[p], b[p+1])
					p += 2
				}
			case 1: // one character, n attributes
				ch := b[min(p, len(b)-1)]
				p++
				for i := 0; i < n && p < len(b); i++ {
					cells = append(cells, ch, b[p])
					p++
				}
			case 2: // one attribute, n characters
				at := b[min(p, len(b)-1)]
				p++
				for i := 0; i < n && p < len(b); i++ {
					cells = append(cells, b[p], at)
					p++
				}
			case 3:
				if p+1 >= len(b) {
					p = len(b)
					break
				}
				for i := 0; i < n; i++ {
					cells = append(cells, b[p], b[p+1])
				}
				p += 2
			}
		}
	}
	n := min(len(cells), want) / 2
	d := newDoc(w, max(1, (n+w-1)/w))
	for i := 0; i < n; i++ {
		d.cells[i] = attrCell(cells[2*i], cells[2*i+1])
	}
	return d, info, nil
}

// ---------------------------------------------------------------- a web page

func (a *App) writeHTML() []byte {
	d := a.doc
	hex := func(c uint32) string { return fmt.Sprintf("#%06x", c&0xFFFFFF) }
	bgOf := func(v uint32) uint32 {
		switch {
		case a.screen():
			return a.screenRGB(d.bg)
		case a.ice:
			return a.pal[cellBg(v)]
		}
		return a.pal[cellBg(v)&7]
	}
	var b strings.Builder
	title := a.info.title
	if title == "" {
		title = a.name
	}
	page := a.pal[0]
	if a.screen() {
		page = a.screenRGB(d.borderColour())
	}
	fmt.Fprintf(&b, "<!doctype html>\n<html><head><meta charset=\"utf-8\"><title>%s</title>\n", html.EscapeString(title))
	fmt.Fprintf(&b, "<style>body{background:%s;margin:2em}pre{font:16px/1 \"Topaz a600a1200a400\",\"Px437 IBM VGA 8x16\",monospace;display:inline-block;background:%s;padding:0;margin:0}</style>\n",
		hex(page), hex(a.pal[0]))
	b.WriteString("</head><body><pre>")
	rows := max(d.rows, 1)
	for y := 0; y < rows; y++ {
		last := uint64(1<<63 - 1)
		open := false
		for x := 0; x < d.cols; x++ {
			v := d.get(x, y)
			fg, bg := a.pal[cellFg(v)], bgOf(v)
			if a.atari() {
				fg = a.pal[d.ink]
			}
			if a.ted() {
				fg = a.tedRGB(tedAttr(v))
			}
			key := uint64(fg)<<32 | uint64(bg)
			if v&italic != 0 {
				key ^= 1 << 62
			}
			if v&underline != 0 {
				key ^= 1 << 61
			}
			if key != last {
				if open {
					b.WriteString("</span>")
				}
				style := "color:" + hex(fg) + ";background:" + hex(bg)
				if v&italic != 0 {
					style += ";font-style:italic"
				}
				if v&underline != 0 {
					style += ";text-decoration:underline"
				}
				fmt.Fprintf(&b, "<span style=\"%s\">", style)
				last, open = key, true
			}
			b.WriteString(html.EscapeString(string(a.runeOf(cellCh(v)))))
		}
		if open {
			b.WriteString("</span>")
		}
		b.WriteString("\n")
	}
	b.WriteString("</pre></body></html>\n")
	return []byte(b.String())
}

// ---------------------------------------------------------------- Petmate workspaces

type petmatePixel struct {
	Code  int `json:"code"`
	Color int `json:"color"`
}

type petmateScreen struct {
	Width           int              `json:"width"`
	Height          int              `json:"height"`
	BackgroundColor int              `json:"backgroundColor"`
	BorderColor     int              `json:"borderColor"`
	Charset         string           `json:"charset"`
	Name            string           `json:"name,omitempty"`
	Framebuf        [][]petmatePixel `json:"framebuf"`
}

type petmateFile struct {
	Version     int             `json:"version"`
	Screens     []int           `json:"screens"`
	Framebufs   []petmateScreen `json:"framebufs"`
	CustomFonts map[string]any  `json:"customFonts"`
}

func isPetmate(b []byte) bool {
	head := b[:min(len(b), 200)]
	return bytes.HasPrefix(bytes.TrimSpace(head), []byte("{")) && bytes.Contains(b, []byte(`"framebufs"`))
}

// PetScreen is one screen of a file that holds several (.petmate, a .c with frames).
type PetScreen struct {
	doc     *Doc
	lower   int // -1 not said, 0 upper, 1 lower
	name    string
	machine string // the machine the file says it is for ("" not said), see machines
	fb      int    // .petmate: its framebuf's place in the file (-1 for a new screen)
}

// petmateOrder is the framebufs in the order of the workspace's screens (Petmate
// writes screens 0..n-1, but an older file may name them otherwise).
func petmateOrder(screens []int, n int) []int {
	seen := map[int]bool{}
	var out []int
	for _, i := range screens {
		if i >= 0 && i < n && !seen[i] {
			seen[i] = true
			out = append(out, i)
		}
	}
	if len(out) == 0 {
		for i := 0; i < n; i++ {
			out = append(out, i)
		}
	}
	return out
}

// petmateLower: the character set name Petmate (and Petmate9) gives says lower case.
func petmateLower(charset string) int {
	if strings.Contains(strings.ToLower(charset), "lower") {
		return 1
	}
	return 0
}

func parsePetmate(b []byte) ([]PetScreen, error) {
	var f petmateFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("not a Petmate file: %v", err)
	}
	var out []PetScreen
	total := 0 // cells in all screens, kept under one largest page
	for _, i := range petmateOrder(f.Screens, len(f.Framebufs)) {
		s := f.Framebufs[i]
		w := max(1, min(s.Width, 1000))
		h := max(1, min(s.Height, 10000)) // like parseMarq
		if total += w * h; total > maxPageCells {
			break
		}
		d := newDoc(w, h)
		d.bg, d.border = s.BackgroundColor&15, s.BorderColor&15
		ted := strings.HasPrefix(s.Charset, "c16") // Petmate9's C16/Plus/4 screens: whole TED colours (ted.go)
		if ted {
			d.bg, d.border = s.BackgroundColor&127, s.BorderColor&127
		}
		for y, row := range s.Framebuf {
			for x, p := range row {
				if x < w && y < h {
					d.cells[y*w+x] = makeCell(p.Code&255, p.Color&15, 0, 0)
					if ted {
						d.cells[y*w+x] = tedCell(p.Code&255, p.Color&255)
					}
				}
			}
		}
		out = append(out, PetScreen{doc: d, lower: petmateLower(s.Charset), name: s.Name, fb: i, machine: map[bool]string{true: "plus4"}[ted]})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("the Petmate file has no screens")
	}
	return out, nil
}

// writePetmate gives a Petmate workspace with this one page as its screen.
func writePetmate(d *Doc, lower bool, name string) []byte {
	return writePetmateFrames(nil, []PetScreen{{doc: d, lower: map[bool]int{true: 1}[lower], name: name, fb: -1}})
}

// writePetmateFrames gives a Petmate workspace of the screens in order. With the
// workspace they came from (src), every screen keeps its framebuf's fields that
// CharEdit does not know (zoom, columnMode, a Petmate9 guide layer ...), and the
// file keeps its custom fonts and other top-level fields; screens become 0..n-1,
// as Petmate writes them.
func writePetmateFrames(src []byte, scr []PetScreen) []byte {
	top := map[string]json.RawMessage{}
	var oldFbs []map[string]json.RawMessage
	var oldUI []json.RawMessage
	if src != nil && json.Unmarshal(src, &top) == nil {
		json.Unmarshal(top["framebufs"], &oldFbs)
		json.Unmarshal(top["framebufUIStates"], &oldUI)
	} else {
		top = map[string]json.RawMessage{}
	}
	raw := func(v any) json.RawMessage { b, _ := json.Marshal(v); return b }
	if _, ok := top["version"]; !ok {
		top["version"] = raw(2)
	}
	if _, ok := top["customFonts"]; !ok {
		top["customFonts"] = raw(map[string]any{})
	}
	fbs := make([]map[string]json.RawMessage, len(scr))
	ui := make([]json.RawMessage, len(scr))
	screens := make([]int, len(scr))
	for i, s := range scr {
		d := s.doc
		fb := map[string]json.RawMessage{}
		if s.fb >= 0 && s.fb < len(oldFbs) {
			for k, v := range oldFbs[s.fb] {
				fb[k] = v
			}
		}
		if s.fb >= 0 && s.fb < len(oldUI) {
			ui[i] = oldUI[s.fb]
		} else {
			ui[i] = raw(nil)
		}
		rows := max(d.rows, 1)
		cells := make([][]petmatePixel, rows)
		for y := range cells {
			row := make([]petmatePixel, d.cols)
			for x := range row {
				v := d.get(x, y)
				row[x] = petmatePixel{cellCh(v), cellFg(v)}
				if s.machine == "plus4" { // Petmate9 keeps the TED's attribute byte
					row[x].Color = tedAttr(v)
				}
			}
			cells[y] = row
		}
		fb["width"], fb["height"], fb["framebuf"] = raw(d.cols), raw(rows), raw(cells)
		fb["backgroundColor"], fb["borderColor"] = raw(d.bg), raw(d.borderColour())
		var cs string
		if json.Unmarshal(fb["charset"], &cs) != nil || petmateLower(cs) != max(s.lower, 0) || strings.HasPrefix(cs, "c16") != (s.machine == "plus4") {
			fb["charset"] = raw(map[bool]string{true: "lower", false: "upper"}[s.lower == 1])
			if s.machine == "plus4" {
				fb["charset"] = raw(map[bool]string{true: "c16Lower", false: "c16Upper"}[s.lower == 1])
			}
		}
		if s.name != "" {
			fb["name"] = raw(s.name)
		}
		fbs[i], screens[i] = fb, i
	}
	top["framebufs"], top["screens"] = raw(fbs), raw(screens)
	if _, ok := top["framebufUIStates"]; ok {
		top["framebufUIStates"] = raw(ui)
	}
	out, _ := json.Marshal(top)
	return out
}

// ---------------------------------------------------------------- CharEdit's own .prg

// parsePrg reads a .prg in Petmate's form (made by Petmate, Marq's PETSCII editor or
// CharEdit). Other C64 programs are not taken apart.
func parsePrg(b []byte) (*Doc, int, error) {
	if len(b) != prgScreen+2000 {
		return nil, -1, fmt.Errorf("only C64 programs in Petmate's form can be opened")
	}
	for i, c := range petmatePrg {
		if i != prgCharset && i != prgBorder && i != prgBg && b[i] != c {
			return nil, -1, fmt.Errorf("only C64 programs in Petmate's form can be opened")
		}
	}
	d := newDoc(40, 25)
	d.border, d.bg, d.glyph0 = int(b[prgBorder]&15), int(b[prgBg]&15), true
	lower := 0
	if b[prgCharset]&2 != 0 { // $17 / $16: the lower case set
		lower = 1
	}
	for i := 0; i < 1000; i++ {
		d.cells[i] = makeCell(int(b[prgScreen+i]), int(b[prgScreen+1000+i]&15), 0, 0)
	}
	return d, lower, nil
}

func isOurPrg(b []byte) bool {
	_, _, err := parsePrg(b)
	return err == nil
}
