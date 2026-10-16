package main

// Playscii's other character sets and palettes. A Playscii file names its charset
// and palette, and Playscii takes them from its own charsets/ and palettes/ folders
// (editorformats.go). Sets of machines CharEdit has a page for open on that page
// (editorformats.go, cpc.go); the others whose glyphs fit a text cell (8 pixels
// wide, 16 high at most) open on a PC page with Playscii's set as the page's own
// font, so the art shows as Playscii shows it: Apple II, Agat, BBC Master, the
// Intellivision, MSX, Pac-Man, Sharp MZ and the ZX81 ("speccy"). A palette other
// than ansi and ega becomes the page's own colours on such pages (and on dos/dos40
// pages); a palette of more than 16 colours gives the 16 the art uses most, the
// others shown as the nearest of them.
//
// The pictures come with Playscii (MIT licence), but the glyphs are the machines'
// own ROMs, so they do not come with CharEdit: they are downloaded on first use
// from Playscii's source at a fixed version and checked by SHA-1, as the
// Commodore ROMs and the EmuTOS font. Saving the page as .psci again writes the
// same charset and palette names, the glyph numbers and palette numbers as read.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// psciVersion is the Playscii source the pictures are fetched from (a mirror of JP LeBreton's).
const psciVersion = "79b3c24deb26d1d2e5855461c8819f0542ce5ea4"

// psciSet is one of Playscii's charsets CharEdit shows as a page's own font: its
// picture's map (glyphs across, down) and the picture's SHA-1.
type psciSet struct {
	name       string // what the page says
	cols, rows int
	sha1       string
}

var psciSets = map[string]psciSet{
	"agat":          {"Agat", 16, 6, "cdaa9d08fe94e392d3fee21fab8250d9ecd93089"},
	"apple2":        {"Apple II", 16, 8, "2b2d00e4b44c703ad3c7fc42a32806c24e222516"},
	"bbc_master":    {"BBC Master", 16, 14, "ca7ee6c69f8b791cfad1c94e6124d5f98c0b26ec"},
	"intellivision": {"Intellivision", 16, 14, "e32fecc09cdd33e723fc3a26faedc5c724b9ee0d"},
	"msx":           {"MSX", 32, 8, "d62642174ed60ef3f76c3ec470dab9361e14c788"},
	"pacman":        {"Pac-Man", 16, 6, "e41d3eeede40ece5e2df2cb742bab27ee06de5e4"},
	"sharp":         {"Sharp MZ", 16, 22, "c3c3101c17862d3f3b91cb490191a25165f9584d"},
	"speccy":        {"ZX81", 16, 4, "2ce645bd51258ad355d8f366330f9ad7a21af786"},
}

// psciPaletteFiles are the SHA-1s of Playscii's palette pictures CharEdit fetches
// (the ones in psciPalettes are built in).
var psciPaletteFiles = map[string]string{
	"ansi-240": "25919c650c78594df96212b0993cbbef86efea57", "apple2": "0e519bc2e4f0b465d86bad66148a6a9037057578",
	"arne16": "0383b089c60f1bec506fc72ca166636e4d2759eb", "cpc": "497c63ba0b8169e42a2843a6b6e881f504a52483",
	"dawnbringer16": "5bff8ebd69465a2629f266145b8d73a5e76ef187", "doom": "3f56288b5fbde0a763b8750f71138dd9309a5f9a",
	"dpaint": "46fb68b68f67e1e9cafb1cae15ef490a194a10d4", "gameboy": "1c05c3d5221e9c15fb7d08177cc44f452ee1800e",
	"hercules": "8c3a1a242be84c4554ef7946aa1b6b1cbe73b544", "heretic": "312e6018b24b2ce71e4f8cdef7470ffa1d16b1c5",
	"intellivision": "860622586d8d73bfe982fa1f820aff7e1bf8542d", "macintosh2": "8fa34b70daf35a3bd968b0a0d44ae57ebc677bd6",
	"mspaint": "c7ee672cba7edf4d85015925ebaa1ad86750ec9f", "msx": "3a8961f1e337984d7a4aeea5e70e39410be5470a",
	"nes": "500fa0ddefb77a214c41e6d8bcb489d41fd4d77f", "pacman": "4519357ef7b20c1930817a0d26ffa3a0778e26a3",
	"pico8": "99393a3ba30e76ac43cb37eea2ed0bb2a7f9ae64", "quake": "3c23a99bbb11f1dddef4ac8e89cdf06c41bb85ec",
	"sharecart1000": "60b03afa1019ccb9fc5373cc3313abb2083d3cf6", "speccy": "c495d748f4588fbfcc975a7cc0e1654510765047",
	"teletext": "dd56c623e1e55ddcd8d37666a547f07230b26ddb",
}

// psciFile is where a fetched picture is kept ("charsets/apple2.png").
func psciFilePath(rel string) string {
	return filepath.Join(appDataDir(), "playscii", filepath.FromSlash(rel))
}

var psciURL = func(rel string) string {
	return "https://raw.githubusercontent.com/michael-lazar/playscii/" + psciVersion + "/" + rel
}

// psciRel are the pictures of a charset and a palette CharEdit fetches, with their SHA-1s.
func psciRel(charset, palette string) map[string]string {
	out := map[string]string{}
	if s, ok := psciSets[charset]; ok {
		out["charsets/"+charset+".png"] = s.sha1
	}
	if sum, ok := psciPaletteFiles[palette]; ok {
		out["palettes/"+palette+".png"] = sum
	}
	return out
}

// psciPicture reads a fetched picture; nil if it is not here (or not the one expected).
func psciPicture(rel, sum string) image.Image {
	b, err := os.ReadFile(psciFilePath(rel))
	if err != nil || !sha1Is(b, sum) {
		return nil
	}
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		return nil
	}
	return img
}

// psciMissing are the pictures a file's charset and palette need that are not here.
func psciMissing(charset, palette string) []string {
	var out []string
	for rel, sum := range psciRel(charset, palette) {
		if b, err := os.ReadFile(psciFilePath(rel)); err != nil || !sha1Is(b, sum) {
			out = append(out, rel)
		}
	}
	sort.Strings(out)
	return out
}

// psciSum is the SHA-1 of one of Playscii's pictures CharEdit fetches ("" for others).
func psciSum(rel string) string {
	name := strings.TrimSuffix(filepath.Base(rel), ".png")
	switch {
	case rel == "charsets/"+name+".png":
		return psciSets[name].sha1
	case rel == "palettes/"+name+".png":
		return psciPaletteFiles[name]
	}
	return ""
}

// fetchPsci fetches pictures (the network part: no font list touched).
func fetchPsci(rels []string) error {
	client := &http.Client{Timeout: 20 * time.Second}
	for _, rel := range rels {
		sum := psciSum(rel)
		if sum == "" {
			return fmt.Errorf("%s is not one of Playscii's files CharEdit knows", rel)
		}
		if b, err := os.ReadFile(psciFilePath(rel)); err == nil && sha1Is(b, sum) {
			continue
		}
		b, err := fetchBody(client, psciURL(rel), 1<<20)
		if err != nil {
			return err
		}
		if !sha1Is(b, sum) {
			return fmt.Errorf("%s is not Playscii's file expected", rel)
		}
		if err := os.MkdirAll(filepath.Dir(psciFilePath(rel)), 0o755); err != nil {
			return err
		}
		if err := writeAtomic(psciFilePath(rel), b); err != nil {
			return err
		}
	}
	return nil
}

// psciCharsetFont is a fetched charset as a font: glyph i of the picture (left to
// right, top to bottom) is character i; a pixel that is not black is set (charset.py).
func psciCharsetFont(charset string) *Font {
	s, ok := psciSets[charset]
	if !ok {
		return nil
	}
	img := psciPicture("charsets/"+charset+".png", s.sha1)
	if img == nil {
		return nil
	}
	return psciFontOf(img, s.cols, s.rows, "Playscii "+s.name+" ("+charset+")")
}

// psciFontOf cuts a charset picture of cols x rows glyphs into a font (nil if they do not fit a cell).
func psciFontOf(img image.Image, cols, rows int, name string) *Font {
	bd := img.Bounds()
	gw, gh := bd.Dx()/cols, bd.Dy()/rows
	if gw < 1 || gw > 8 || gh < 6 || gh > 16 {
		return nil
	}
	data := make([]byte, 0, 256*gh)
	for i := 0; i < min(256, cols*rows); i++ {
		gx, gy := bd.Min.X+i%cols*gw, bd.Min.Y+i/cols*gh
		for r := 0; r < gh; r++ {
			var v byte
			for b := 0; b < gw; b++ {
				cr, cg, cb, ca := img.At(gx+b, gy+r).RGBA()
				if ca >= 0x8000 && cr|cg|cb >= 0x100 {
					v |= 0x80 >> b
				}
			}
			data = append(data, v)
		}
	}
	f := newFont(ownID, name, "pc", data, gh, min(256, cols*rows))
	f.format = "psci"
	return f
}

// psciPaletteColours are a Playscii palette's colours (1.. in order): built in, or a fetched
// picture's colours in the order they first appear row by row (palette.py); nil if not here.
func psciPaletteColours(name string) []uint32 {
	if p, ok := psciPalettes[name]; ok {
		return p
	}
	sum, ok := psciPaletteFiles[name]
	if !ok {
		return nil
	}
	img := psciPicture("palettes/"+name+".png", sum)
	if img == nil {
		return nil
	}
	var out []uint32
	seen := map[uint32]bool{}
	bd := img.Bounds()
	for y := bd.Min.Y; y < bd.Max.Y; y++ {
		for x := bd.Min.X; x < bd.Max.X && len(out) < 1023; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			c := r>>8<<16 | g>>8<<8 | b>>8
			if !seen[c] {
				seen[c] = true
				out = append(out, c)
			}
		}
	}
	return out
}

// psciOwn is how a page keeps a Playscii file's own charset and palette.
type psciOwn struct {
	charset, palette string
	font             *Font       // the charset as the page's font (nil: a set CharEdit has, dos)
	pal              *[16]uint32 // the page's colours (cell order), nil: the page's own palette
	index            [16]int     // the Playscii colour of each page colour
	colour           map[int]int // the page colour of each Playscii colour
}

// psciOwnColours picks the page's 16 colours of a palette: all of them for 16 or
// fewer, else the 16 the tiles use most (then by number).
func psciOwnColours(o *psciOwn, pal []uint32, f *psciFile) {
	use := map[int]int{}
	for _, fr := range f.Frames {
		for _, l := range fr.Layers {
			for _, tl := range l.Tiles {
				use[int(tl.Fg)]++
				use[int(tl.Bg)]++
			}
		}
	}
	var pick []int // Playscii colours 1..
	if len(pal) <= 16 {
		for i := range pal {
			pick = append(pick, i+1)
		}
	} else {
		for i := 1; i <= len(pal); i++ {
			pick = append(pick, i)
		}
		sort.SliceStable(pick, func(a, b int) bool { return use[pick[a]] > use[pick[b]] })
		pick = pick[:16]
		sort.Ints(pick)
	}
	o.pal, o.colour = &[16]uint32{}, map[int]int{}
	for i := range o.index {
		o.index[i] = pick[min(i, len(pick)-1)]
		o.pal[i] = 0xFF000000 | pal[o.index[i]-1]
	}
	for c, idx := range pick {
		o.colour[idx] = c
	}
	for i := 1; i <= len(pal); i++ {
		if _, ok := o.colour[i]; !ok {
			o.colour[i] = nearestOf(pal[i-1], *o.pal)
		}
	}
}

// psciOwnOf is the charset and palette of the page's Playscii file when the page keeps them.
func (a *App) psciOwnOf() *psciOwn {
	if a.srcKind != "psci" || !a.pc() || a.source == nil {
		return nil
	}
	art, err := parsePsci(a.source)
	if err != nil || art.mode != "pc" {
		return nil
	}
	return art.own
}

// psciNeeds asks to fetch Playscii's pictures a file needs; then opens it. True if it asked.
func (a *App) psciNeeds(art *psciArt, b []byte, name, path, as string) bool {
	if len(art.need) == 0 {
		return false
	}
	t := a.Tab
	what := "Playscii's character set"
	if !strings.HasPrefix(art.need[0], "charsets/") {
		what = "Playscii's palette"
	}
	a.dialog = &FontReq{title: what, why: psciWhy(art), what: what, fetch: func() error { return fetchPsci(art.need) },
		load: func() (string, error) {
			if m := psciMissing(art.ownCharset, art.ownPalette); len(m) > 0 {
				return "", fmt.Errorf("saved %s but could not read it back", strings.Join(m, ", "))
			}
			return what + " is here", nil
		},
		after: func() {
			if a.backTo(t) {
				a.loadBytes(b, name, path, as)
			}
		}}
	return true
}

func psciWhy(art *psciArt) string {
	var what []string
	if s, ok := psciSets[art.ownCharset]; ok && psciCharsetFont(art.ownCharset) == nil {
		what = append(what, "character set \""+art.ownCharset+"\" ("+s.name+")")
	}
	if _, ok := psciPaletteFiles[art.ownPalette]; ok && psciPaletteColours(art.ownPalette) == nil {
		what = append(what, "palette \""+art.ownPalette+"\"")
	}
	return "This Playscii file draws with Playscii's " + strings.Join(what, " and ") + ". They come with Playscii, " +
		"not with this program (the glyphs are the machines' own). Download them from Playscii's source now? (a few KB, " +
		"checked before they are used, kept in " + shownDir(appDataDir()) + ".)"
}

// psciOwnSetup gives the page being opened the Playscii file's own font and colours.
func (a *App) psciOwnSetup(o *psciOwn) {
	if o == nil {
		return
	}
	if o.font != nil {
		a.ownFont, a.fontFor["pc"] = o.font, ownID
	}
	if o.pal != nil {
		p := *o.pal
		a.ownPal, a.paletteFor["pc"] = &p, "file"
	}
}

// psciSTRes is the ST resolution a Playscii picture in the ST's font opens in: low
// for 40 columns or fewer, else high for two colours, else medium.
func psciSTRes(f *psciFile) string {
	if f.Width <= 40 {
		return "stlow"
	}
	used := map[float64]bool{}
	for _, fr := range f.Frames {
		for _, l := range fr.Layers {
			for _, tl := range l.Tiles {
				used[tl.Fg], used[tl.Bg] = true, true
			}
		}
	}
	delete(used, 0)
	if len(used) <= 2 {
		return "sthigh"
	}
	return "stmed"
}

// psciMachineSetup asks for the ST font when a Playscii file opens on an ST page; true if it asked.
func (a *App) psciMachineSetup(art *psciArt, b []byte, name, path, as string) bool {
	if art.mode != "st" {
		return false
	}
	if !hasSTFont() {
		t := a.Tab
		openSTReq(func() {
			if a.backTo(t) {
				a.loadBytes(b, name, path, as)
			}
		})
		return true
	}
	a.stres = art.stres
	return false
}

// psciRelAll are all of Playscii's pictures CharEdit may fetch, with their SHA-1s.
func psciRelAll() map[string]string {
	out := map[string]string{}
	for name, s := range psciSets {
		out["charsets/"+name+".png"] = s.sha1
	}
	for name, sum := range psciPaletteFiles {
		out["palettes/"+name+".png"] = sum
	}
	return out
}

// psciSTPalette is the palette an ST page writes Playscii's tiles in: its file's when it is
// one CharEdit has, else ansi. (No Playscii palette holds the ST's colours: each pen is
// written as the palette's nearest colour, read back as the nearest pen.)
func (a *App) psciSTPalette() string {
	if a.srcKind == "psci" && a.source != nil {
		var f struct {
			Palette string `json:"palette"`
		}
		if json.Unmarshal(a.source, &f) == nil && psciPaletteColours(f.Palette) != nil {
			return f.Palette
		}
	}
	return "ansi"
}

// psciPenColour is the Playscii colour (1..) an ST pen is written as: of the palette's
// colours that read back as this pen (the nearest pen to them), the nearest; else the nearest.
func psciPenColour(pens [16]uint32, pen int, pal []uint32) int {
	best, bd := 0, 1<<62
	for i, c := range pal {
		if nearestOf(c, pens) != pen {
			continue
		}
		p := pens[pen]
		dr, dg, db := int(c>>16&255)-int(p>>16&255), int(c>>8&255)-int(p>>8&255), int(c&255)-int(p&255)
		if d := dr*dr + dg*dg + db*db; d < bd {
			best, bd = i+1, d
		}
	}
	if best == 0 {
		return psciNearest(pens[pen], pal)
	}
	return best
}
