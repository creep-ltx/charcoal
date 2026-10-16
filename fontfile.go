package main

// A page's own font: loaded from a file (PC raw .fnt/.f08/.f14/.f16, PSF1/PSF2,
// an XBin's font, Atari 8-bit .fnt, C64/VIC/PET character sets .bin/.64c, ZX
// Spectrum 768-byte sets and UDGs), drawn in the font editor (fontedit.go),
// kept with the art where the format can hold it (XBin, Petmate's
// customFonts) and remembered by CharEdit where it cannot (prefs art_fonts,
// a copy of the font in the data folder's fonts/), and in autosaves.
//
// Fonts are kept as 256 glyphs of 16 bytes (fonts.go); a font of 8 rows has
// every row doubled, one of 9-16 rows uses its first h.

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// The page's own font is chosen by these ids in Tab.fontFor (an XBin's font was
// the first own font, hence the name); ownLowerID is a PETSCII page's own lower
// case set.
const ownID, ownLowerID = "xbin", "xbinlower"

// rows is how many pixel rows the font's glyphs have.
func (f *Font) rows() int {
	if f.h > 0 {
		return f.h
	}
	switch f.kind {
	case "c64", "atari", "zx", "cpc":
		return 8
	}
	if f.id == "st8x8" {
		return 8
	}
	return 16
}

// row gives row r (0 .. rows()-1) of glyph c.
func (f *Font) row(c, r int) byte {
	if f.rows() == 8 {
		return f.bits[(c&255)*16+r*2]
	}
	return f.bits[(c&255)*16+r]
}

// setRow sets row r of glyph c (both copies of a doubled row).
func (f *Font) setRow(c, r int, v byte) {
	if f.rows() == 8 {
		f.bits[(c&255)*16+r*2], f.bits[(c&255)*16+r*2+1] = v, v
		return
	}
	f.bits[(c&255)*16+r] = v
}

// edited: the glyphs changed, so the flip tables are made again when needed.
func (f *Font) edited() { f.mirror = [3][]int{} }

func (f *Font) clone() *Font {
	g := *f
	g.bits = append([]byte(nil), f.bits...)
	g.mirror = [3][]int{}
	return &g
}

// native is the font as a raw file holds it: 256 glyphs of rows() bytes.
func (f *Font) native() []byte {
	h := f.rows()
	out := make([]byte, 0, 256*h)
	for c := 0; c < 256; c++ {
		for r := 0; r < h; r++ {
			out = append(out, f.row(c, r))
		}
	}
	return out
}

// newFont makes a font of n glyphs of h rows each from data (glyph i at i*h);
// glyphs past n are empty.
func newFont(id, name, kind string, data []byte, h, n int) *Font {
	f := &Font{id: id, name: name, kind: kind, syncterm: -1, bits: make([]byte, 4096), h: h}
	for c := 0; c < min(n, 256); c++ {
		for r := 0; r < h && c*h+r < len(data); r++ {
			f.setRow(c, r, data[c*h+r])
		}
	}
	return f
}

// fontExts are the file endings of font files: a dropped one asks whether it is the page's font.
var fontExts = map[string]bool{".fnt": true, ".f08": true, ".f10": true, ".f12": true, ".f14": true, ".f16": true,
	".psf": true, ".psfu": true, ".64c": true, ".ch8": true, ".udg": true, ".font": true}

func isPSF1(b []byte) bool { return len(b) >= 4 && b[0] == 0x36 && b[1] == 0x04 }
func isPSF2(b []byte) bool { return len(b) >= 32 && bytes.Equal(b[:4], []byte{0x72, 0xB5, 0x4A, 0x86}) }

// looksLikeFont: is a file by its name or first bytes a font rather than art?
func looksLikeFont(name string, b []byte) bool {
	ext := strings.ToLower(filepath.Ext(name))
	if fontExts[ext] || isPSF1(b) || isPSF2(b) {
		return true
	}
	// A character set saved as .bin or .raw (art in .bin is rarely exactly this size).
	return (ext == ".bin" || ext == ".raw") && (len(b) == 1024 || len(b) == 2048 || len(b) == 3584 || len(b) == 4096)
}

// readPSF reads a PC Screen Font (version 1 or 2): its glyphs, height and count.
func readPSF(b []byte) (data []byte, h, n int, version int, err error) {
	switch {
	case isPSF1(b):
		h, n = int(b[3]), 256
		if b[2]&1 != 0 {
			n = 512
		}
		if h < 1 || len(b) < 4+n*h {
			return nil, 0, 0, 1, fmt.Errorf("the PSF font is cut short")
		}
		return b[4 : 4+n*h], h, n, 1, nil
	case isPSF2(b):
		u32 := func(i int) int { return int(binary.LittleEndian.Uint32(b[i:])) }
		head, count, size, height, width := u32(8), u32(16), u32(20), u32(24), u32(28)
		if width < 1 || width > 8 {
			return nil, 0, 0, 2, fmt.Errorf("the PSF font is %d pixels wide; only fonts 8 pixels wide fit a text cell here", width)
		}
		if height < 1 || size != height || count < 1 || count > 1<<16 || head < 32 || head > len(b) || len(b)-head < count*size {
			return nil, 0, 0, 2, fmt.Errorf("the PSF font's header does not fit its size")
		}
		return b[head : head+count*size], height, count, 2, nil
	}
	return nil, 0, 0, 0, fmt.Errorf("not a PSF font")
}

// parseFontFile reads a font file for a page of the given mode. cur is the
// page's font now (a Spectrum set or UDGs replace only part of it). low is a
// PETSCII page's lower case set when the file holds both sets.
func parseFontFile(name string, b []byte, mode string, cur *Font) (up, low *Font, err error) {
	base := strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
	ext := strings.ToLower(filepath.Ext(name))
	size := func() error {
		return fmt.Errorf("%s is %d bytes, not a %s", filepath.Base(name), len(b), map[string]string{
			"pc": "PC font (256 characters of 8, 14 or 16 rows: 2048, 3584 or 4096 bytes, or PSF)", "amiga": "font (256 characters of 8 or 16 rows, or PSF)",
			"c64":   "C64 character set (2048 or 4096 bytes, .64c with a load address, or 1024 for 128 characters)",
			"atari": "Atari font (1024 bytes)", "zx": "Spectrum character set (768 bytes) or UDGs (168 bytes)"}[mode])
	}
	switch mode {
	case "st", "tt", "cpc":
		return nil, nil, fmt.Errorf("an %s page keeps its own font", modeNames[mode])
	case "pc", "amiga":
		var data []byte
		var h, n int
		format := "raw"
		switch {
		case isPSF1(b) || isPSF2(b):
			var v int
			if data, h, n, v, err = readPSF(b); err != nil {
				return nil, nil, err
			}
			format = fmt.Sprint("psf", v)
		case isXBin(b):
			_, info, err := parseXBin(b)
			if err != nil {
				return nil, nil, err
			}
			if info.font == nil {
				return nil, nil, fmt.Errorf("%s holds no font", filepath.Base(name))
			}
			f := info.font
			f.id, f.name, f.kind, f.src, f.format = ownID, base, mode, name, "xbin"
			return f, nil, nil
		case len(b) > 0 && len(b)%256 == 0:
			h, n, data = len(b)/256, 256, b
		default:
			return nil, nil, size()
		}
		if h < 6 || h > 16 {
			if h > 16 {
				return nil, nil, fmt.Errorf("%s has %d rows: fonts of up to 16 rows can be used", filepath.Base(name), h)
			}
			return nil, nil, size()
		}
		f := newFont(ownID, base, mode, data, h, n)
		f.src, f.format = name, format
		return f, nil, nil
	case "c64":
		data := b
		format := "bin"
		if ext == ".64c" {
			if len(b) < 2+8 {
				return nil, nil, size()
			}
			data, format = b[2:], "64c"
		}
		n := len(data) / 8
		switch {
		case format == "64c" && n > 0 && n <= 512:
		case len(data) == 1024 || len(data) == 2048 || len(data) == 4096:
		default:
			return nil, nil, size()
		}
		mk := func(id string, d []byte) *Font {
			f := newFont(id, base, "c64", d, 8, len(d)/8)
			if len(d) <= 1024 { // 128 characters: the top half shows them reversed, as on a PET
				for c := 128; c < 256; c++ {
					for r := 0; r < 8; r++ {
						f.setRow(c, r, ^f.row(c-128, r))
					}
				}
			}
			f.src, f.format = name, format
			return f
		}
		if len(data) > 2048 {
			up, low = mk(ownID, data[:2048]), mk(ownLowerID, data[2048:min(len(data), 4096)])
			low.name = base + " (lower case)"
			return up, low, nil
		}
		return mk(ownID, data), nil, nil
	case "atari":
		if len(b) != 1024 {
			return nil, nil, size()
		}
		f := atariFont(ownID, base, b)
		f.src, f.format, f.h = name, "atari", 8
		return f, nil, nil
	case "zx":
		if cur == nil || cur.kind != "zx" {
			cur = zxFont
		}
		f := cur.clone()
		f.id, f.name, f.src, f.h = ownID, base, name, 8
		switch len(b) {
		case 768: // codes 32-127, as the ROM's
			for c := 32; c < 128; c++ {
				for r := 0; r < 8; r++ {
					f.setRow(c, r, b[(c-32)*8+r])
				}
			}
			f.format = "zx"
		case 168: // the 21 UDGs, A-U (codes 144-164)
			for c := 144; c < 165; c++ {
				for r := 0; r < 8; r++ {
					f.setRow(c, r, b[(c-144)*8+r])
				}
			}
			f.format = "zxudg"
			if cur.id != ownID {
				f.name = "ZX Spectrum + " + base
			} else {
				f.name = cur.name
			}
		default:
			return nil, nil, size()
		}
		return f, nil, nil
	}
	return nil, nil, fmt.Errorf("no fonts for this page")
}

// fontFileBytes writes the page's font (and a PETSCII page's lower case set)
// as a file of the kind its name ends in.
func fontFileBytes(f, low *Font, ext string) ([]byte, error) {
	ext = strings.ToLower(ext)
	h := f.rows()
	switch {
	case ext == ".psf" || ext == ".psfu":
		if f.format == "psf1" {
			return append([]byte{0x36, 0x04, 0, byte(h)}, f.native()...), nil
		}
		head := make([]byte, 32)
		for i, v := range []uint32{0x864AB572, 0, 32, 0, 256, uint32(h), uint32(h), 8} {
			binary.LittleEndian.PutUint32(head[i*4:], v)
		}
		return append(head, f.native()...), nil
	case f.kind == "c64":
		data := f.native()
		if low != nil {
			data = append(data, low.native()...)
		}
		if ext == ".64c" {
			return append([]byte{0x00, 0x30}, data...), nil // loaded at $3000
		}
		return data, nil
	case f.kind == "atari":
		out := make([]byte, 1024)
		for in := 0; in < 128; in++ { // the ROM's order: the inverse of atariFont's
			c := in
			switch {
			case in < 64:
				c = in + 32
			case in < 96:
				c = in - 64
			}
			for r := 0; r < 8; r++ {
				out[in*8+r] = f.row(c, r)
			}
		}
		return out, nil
	case f.kind == "zx":
		from, to := 32, 128
		if ext == ".udg" {
			from, to = 144, 165
		}
		var out []byte
		for c := from; c < to; c++ {
			for r := 0; r < 8; r++ {
				out = append(out, f.row(c, r))
			}
		}
		return out, nil
	}
	return f.native(), nil
}

// fontExt is the file ending a page's font is saved with by default.
func fontExt(f *Font) string {
	switch {
	case f.format == "psf1" || f.format == "psf2":
		return ".psf"
	case f.kind == "c64":
		return ".64c"
	case f.kind == "atari":
		return ".fnt"
	case f.kind == "zx" && f.format == "zxudg":
		return ".udg"
	case f.kind == "zx":
		return ".ch8"
	}
	return fmt.Sprintf(".f%02d", f.rows())
}

// ---------------------------------------------------------------- the page's own font

// ownInUse is the page's own font when the page shows it, else nil.
func (a *App) ownInUse() *Font {
	switch id := a.fontFor[a.mode]; {
	case id == ownID && a.ownFont != nil && a.ownFont.kind == a.mode:
		return a.ownFont
	case id == ownLowerID && a.ownLower != nil && a.ownLower.kind == a.mode:
		return a.ownLower
	}
	return nil
}

// useOwnFont makes up (and low, a PETSCII page's lower case set) the page's
// font. changed: the page differs from its file now (not when a file brings
// its own font).
func (a *App) useOwnFont(up, low *Font, changed bool) {
	lower := a.c64() && a.lower()
	a.ownFont, a.ownLower = up, low
	a.fontFor[a.mode] = ownID
	if lower && low != nil {
		a.fontFor[a.mode] = ownLowerID
	}
	if a.zx() && a.saveFmt == "scr" && a.source != nil && !changed {
		a.reparse() // a screen's cells are matched to the font
	}
	if changed {
		a.fontChanged()
	}
}

// fontChanged: the page's font is not what its file holds any more.
func (a *App) fontChanged() {
	a.gen = nextGen()
	a.dirty = true
}

// loadFontFile reads a font file into the page; it says what happened.
func (a *App) loadFontFile(path string) {
	b, err := os.ReadFile(path)
	if err != nil {
		a.msg("Could not read the font: " + err.Error())
		return
	}
	up, low, err := parseFontFile(path, b, a.mode, a.font())
	if err != nil {
		a.msg("Could not use the font: " + err.Error())
		return
	}
	a.useOwnFont(up, low, true)
	what := fmt.Sprintf("8x%d", up.rows())
	if low != nil {
		what = "upper and lower case sets"
	}
	a.msg(fmt.Sprintf("Font %s (%s) for this page; %s", up.name, what, a.fontKeptHint()))
}

// fontKeptHint says where the page's own font will be kept.
func (a *App) fontKeptHint() string {
	switch {
	case carriesFont(a.saveFmt):
		return "saving puts it in the file"
	case a.pc() || a.amiga():
		return "an XBin holds it inside; for other formats CharEdit remembers it with the file"
	case a.c64():
		return "a Petmate file holds it inside; for other formats CharEdit remembers it with the file"
	}
	return "CharEdit remembers it with the file when saved"
}

// carriesFont: the format holds the page's font inside.
func carriesFont(fmtID string) bool {
	return fmtID == "xbin" || fmtID == "petmate" || editorCarriesFont(fmtID)
}

// ArtFont is a font CharEdit keeps for an art file whose format cannot hold it.
type ArtFont struct {
	File   string `json:"file"` // in appDataDir()/fonts: the font's 4096 bytes (+4096 for a lower case set)
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	H      int    `json:"rows"`
	Lower  bool   `json:"lower,omitempty"` // a PETSCII lower case set follows
	Format string `json:"format,omitempty"`
}

// droppedArtFonts are files whose kept font this CharEdit let go (another's prefs.json may still name them).
var droppedArtFonts = map[string]bool{}

func artFontDir() string { return filepath.Join(appDataDir(), "fonts") }

// writeFontCopy keeps the page's own font in the data folder; the file's name is its contents' hash.
func writeFontCopy(up, low *Font) (ArtFont, error) {
	data := append([]byte(nil), up.bits...)
	if low != nil {
		data = append(data, low.bits...)
	}
	sum := sha1.Sum(data)
	name := hex.EncodeToString(sum[:8]) + ".font"
	if err := os.MkdirAll(artFontDir(), 0o755); err != nil {
		return ArtFont{}, err
	}
	p := filepath.Join(artFontDir(), name)
	if _, err := os.Stat(p); err != nil {
		if err := writeAtomic(p, data); err != nil {
			return ArtFont{}, err
		}
	}
	return ArtFont{File: name, Name: up.name, Kind: up.kind, H: up.h, Lower: low != nil, Format: up.format}, nil
}

// readFontCopy is writeFontCopy the other way.
func readFontCopy(e ArtFont, dir string) (up, low *Font, err error) {
	b, err := os.ReadFile(filepath.Join(dir, filepath.Base(e.File)))
	if err != nil {
		return nil, nil, err
	}
	want := 4096
	if e.Lower {
		want = 8192
	}
	if len(b) != want || e.H < 0 || e.H > 16 {
		return nil, nil, fmt.Errorf("the kept font %s is damaged", e.File)
	}
	up = &Font{id: ownID, name: e.Name, kind: e.Kind, syncterm: -1, bits: b[:4096], h: e.H, format: e.Format}
	if e.Lower {
		low = &Font{id: ownLowerID, name: e.Name + " (lower case)", kind: e.Kind, syncterm: -1, bits: b[4096:], h: e.H, format: e.Format}
	}
	return up, low, nil
}

// keepArtFont runs after the page was saved as path: a format that cannot
// hold the page's own font has it remembered for that file. It gives a note
// for the status line ("" when there is nothing to say).
func (a *App) keepArtFont(path, fmtID string) string {
	switch fmtID {
	case "png", "png2", "gif", "apng", "html", "utf8":
		return "" // pictures have the font drawn in; plain text has none
	}
	key := filepath.Clean(path)
	f := a.ownInUse()
	if f == nil || carriesFont(fmtID) {
		if _, ok := prefs.ArtFonts[key]; ok {
			delete(prefs.ArtFonts, key)
			droppedArtFonts[key] = true
			savePrefs()
		}
		return ""
	}
	up, low := a.ownFont, a.ownLower
	if up == nil || up.kind != a.mode {
		up, low = f, nil
	}
	e, err := writeFontCopy(up, low)
	if err != nil {
		return "; the font could not be kept: " + err.Error()
	}
	if prefs.ArtFonts == nil {
		prefs.ArtFonts = map[string]ArtFont{}
	}
	prefs.ArtFonts[key] = e
	delete(droppedArtFonts, key)
	savePrefs()
	what := "a ." + strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
	if fmtID == "prg" {
		what = "a .prg in Petmate's form"
	} else if filepath.Ext(path) == "" {
		what = "the format"
	}
	return fmt.Sprintf("; %s cannot hold the font, CharEdit keeps it for the file", what)
}

// restoreArtFont gives a page opened from path the font CharEdit kept for it.
func (a *App) restoreArtFont(path string) {
	e, ok := prefs.ArtFonts[filepath.Clean(path)]
	if !ok || e.Kind != a.mode || a.ownInUse() != nil {
		return
	}
	up, low, err := readFontCopy(e, artFontDir())
	if err != nil {
		a.msg("The font kept for " + filepath.Base(path) + " could not be read: " + err.Error())
		return
	}
	a.useOwnFont(up, low, false)
	a.msg(fmt.Sprintf("Opened %s with its font %s (kept by CharEdit)", path, up.name))
}

// ---------------------------------------------------------------- Petmate's custom fonts

// petmateFont is a custom font of a Petmate workspace (customFonts: id ->
// {name, font: {bits: 2048 numbers, charOrder: 256 numbers}}); a framebuf's
// charset names its id.
type petmateFont struct {
	Name string `json:"name"`
	Font struct {
		Bits      []int `json:"bits"`
		CharOrder []int `json:"charOrder"`
	} `json:"font"`
}

// petmateOwnFont gives the custom font the workspace's first screen uses, nil if it uses the ROM's.
func petmateOwnFont(b []byte) *Font {
	var f struct {
		Screens     []int                      `json:"screens"`
		Framebufs   []struct{ Charset string } `json:"framebufs"`
		CustomFonts map[string]petmateFont     `json:"customFonts"`
	}
	if json.Unmarshal(b, &f) != nil || len(f.Framebufs) == 0 {
		return nil
	}
	order := petmateOrder(f.Screens, len(f.Framebufs))
	id := f.Framebufs[order[0]].Charset
	cf, ok := f.CustomFonts[id]
	if !ok {
		return nil
	}
	data := make([]byte, 2048)
	for i, v := range cf.Font.Bits {
		if i < len(data) {
			data[i] = byte(v)
		}
	}
	font := newFont(ownID, cf.Name, "c64", data, 8, 256)
	font.format = "petmate:" + id
	return font
}

// petmateWithFont puts the page's own font (and lower case set) in a Petmate
// workspace as written by writePetmateFrames: a custom font, named by every
// screen in that case; with no own font, screens that named a custom font
// show the ROM's set again.
func petmateWithFont(ws []byte, up, low *Font, lowers []int) []byte {
	top := map[string]json.RawMessage{}
	if json.Unmarshal(ws, &top) != nil {
		return ws
	}
	var fbs []map[string]json.RawMessage
	cf := map[string]json.RawMessage{}
	json.Unmarshal(top["framebufs"], &fbs)
	json.Unmarshal(top["customFonts"], &cf)
	raw := func(v any) json.RawMessage { b, _ := json.Marshal(v); return b }
	add := func(f *Font) string {
		if f == nil {
			return ""
		}
		id, _ := strings.CutPrefix(f.format, "petmate:")
		if id == "" || id == f.format {
			for n := len(cf) + 1; ; n++ {
				if _, taken := cf[fmt.Sprintf("custom_%d", n)]; !taken {
					id = fmt.Sprintf("custom_%d", n)
					break
				}
			}
		}
		var pf petmateFont
		pf.Name = f.name
		for c := 0; c < 256; c++ {
			for r := 0; r < 8; r++ {
				pf.Font.Bits = append(pf.Font.Bits, int(f.row(c, r)))
			}
			pf.Font.CharOrder = append(pf.Font.CharOrder, c)
		}
		cf[id] = raw(pf)
		return id
	}
	usesLower := false
	for _, l := range lowers {
		usesLower = usesLower || l == 1
	}
	upID, lowID := add(up), ""
	if usesLower { // the lower case set goes in only when a screen shows it
		lowID = add(low)
	}
	for i, fb := range fbs {
		lower := i < len(lowers) && lowers[i] == 1
		id := upID
		if lower {
			id = lowID
		}
		var cs string
		json.Unmarshal(fb["charset"], &cs)
		switch {
		case id != "":
			fb["charset"] = raw(id)
		case cf[cs] != nil: // a custom font the page does not show
			fb["charset"] = raw(map[bool]string{true: "lower", false: "upper"}[lower])
		}
	}
	top["framebufs"], top["customFonts"] = raw(fbs), raw(cf)
	out, err := json.Marshal(top)
	if err != nil {
		return ws
	}
	return out
}
