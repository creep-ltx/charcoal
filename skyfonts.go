package main

// The Amiga fonts SkyPix files ask for by name ("ESC[10;13!Helvetica.font!"): the Kickstart
// and Workbench fonts, and the 8x8 IBM set SkyPix terminals draw their text in. They are
// the Amiga's own, so they do not come with CharEdit: they are fetched on first use, as
// IcyDraw 0.6 keeps them (icy_tools, crates/icy_engine/data/fonts/Amiga, yaff files made
// with monobit), at a fixed version, checked by SHA-1. Like IcyDraw, CharEdit draws every
// glyph left-aligned in a cell at most 8 pixels wide and as tall as the font's size.

import (
	"bufio"
	"bytes"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const skyFontVersion = "eddbe237287eda8980435962d7e3900e26605f0d" // icy_tools, IcyDraw 0.6.0

// skyFontFiles are the fonts by name and size (IcyDraw's fonts/skypix.rs); "" is the 8x8 IBM set.
var skyFontFiles = []struct {
	name string
	size int
	file string
	sha1 string
}{
	{"", 8, "Amiga8x8_IBM.yaff", "0c57dd8d23c3994634d4be4bc2fbfc8078e3cbd1"},
	{"Topaz.font", 8, "original/amiga-ks13-topaz-08.yaff", "980b579b9335331da546a84cd00452f20968dfb0"},
	{"Topaz.font", 9, "original/amiga-ks13-topaz-09.yaff", "3bf902bde906e90e7dd36ce60ca2e4f931168c42"},
	{"Topaz.font", 11, "original/workbench-3.1/Topaz_8x11.yaff", "b6340b1c1967df44a5a672a37c123b6707390ce3"},
	{"Diamond.font", 12, "original/workbench-3.1/Diamond_12.yaff", "be8b555dbc0454721a06c2ad5d81689f4445c703"},
	{"Diamond.font", 20, "original/workbench-3.1/Diamond_20.yaff", "40e9c31b5d739c835e1538725f458f79667e9670"},
	{"Emerald.font", 17, "original/workbench-3.1/Emerald_17.yaff", "d8aa19a2272ad75d3d046db56c7cc11ec24aa736"},
	{"Emerald.font", 20, "original/workbench-3.1/Emerald_20.yaff", "ba802db9ed15e1a872cfba7ce52db620cd10a828"},
	{"pearl.font", 8, "original/pearl_08.yaff", "17b275b657e401b98fc364eb355f75461003d4d2"},
	{"Garnet.font", 9, "original/workbench-3.1/Garnet_9.yaff", "2f78ae70209ff5eced464ab47db77ddefdfe13f7"},
	{"Garnet.font", 16, "original/workbench-3.1/Garnet_16.yaff", "ca06f7c2e2cdebb0b71eb494d0a3367f1708a0b6"},
	{"Helvetica.font", 9, "original/workbench-3.1/Helvetica_9.yaff", "b327a2f1cc946226bcb3b9be94fa385261bd3e84"},
	{"Helvetica.font", 11, "original/workbench-3.1/Helvetica_11.yaff", "168ba480e17fb793b058bd193a2329b53cd29ebf"},
	{"Helvetica.font", 13, "original/workbench-3.1/Helvetica_13.yaff", "440bb7b3f259e36491fb241661a5486eba1a9826"},
	{"Helvetica.font", 15, "original/workbench-3.1/Helvetica_15.yaff", "30b1ee7f4be4be7670cc59e6686d521a52b15b8f"},
	{"Helvetica.font", 18, "original/workbench-3.1/Helvetica_18.yaff", "f95c95b6647d9ee912d467a733330e4f36ffea32"},
	{"Helvetica.font", 24, "original/workbench-3.1/Helvetica_24.yaff", "bc976033634a1deb1140c50298a91991aa3257f9"},
	{"Opal.font", 9, "original/workbench-3.1/Opal_9.yaff", "673fd67fceea0599de0613c8456e5a8d19c28d96"},
	{"Opal.font", 12, "original/workbench-3.1/Opal_12.yaff", "1f96ed31de81b1d74684986f46cf835639a0939c"},
	{"Ruby.font", 8, "original/workbench-3.1/Ruby_8.yaff", "faaa0e1225114e297aaffdea98812a9d3ab37f6b"},
	{"Ruby.font", 12, "original/workbench-3.1/Ruby_12.yaff", "2b54c96b6f51a1cf0b5ef1b663756c07c190c40a"},
	{"Ruby.font", 15, "original/workbench-3.1/Ruby_15.yaff", "b6b4eade1de815188a52603e98d182ca810dc3d9"},
	{"Sapphire.font", 14, "original/workbench-3.1/Sapphire_14.yaff", "7207ad25b13001d9749baf2b466f6cc513f8e338"},
	{"Sapphire.font", 15, "original/workbench-1.0/Sapphire_15.yaff", "629cb8de7b785ad0a2aaa693507d45f10f6765d2"},
	{"Sapphire.font", 18, "original/workbench-1.0/Sapphire_18.yaff", "c3d7d4713610e1a06f571f0b77a40532354e5496"},
	{"Sapphire.font", 19, "original/workbench-3.1/Sapphire_19.yaff", "d8330993d06dda5931e2390409d5c4637031371a"},
	{"Times.font", 11, "original/workbench-3.1/Times_11.yaff", "43f43d1c4a62e43f078d767e5366855d2b6a71cd"},
	{"Times.font", 13, "original/workbench-3.1/Times_13.yaff", "e21b55df0614636a4f02e3c5c99499f41eeaf77e"},
	{"Times.font", 15, "original/workbench-3.1/Times_15.yaff", "fc75ac8d95ce243ffd0248d374aedafdea4aca1f"},
	{"Times.font", 18, "original/workbench-3.1/Times_18.yaff", "a44436b30eb7c73b39153bd291b839302898c3c9"},
	{"Times.font", 24, "original/workbench-3.1/Times_24.yaff", "536a283a6130a8849f99a1925ffcc3cdc44527f0"},
	{"Times.font", 30, "original/workbench-3.1/Times_30.yaff", "29e579a3a7e68fed09fcbaefd6647cc5020cd8f6"},
	{"Times.font", 36, "original/workbench-3.1/Times_36.yaff", "133afc81daa6d98d194044b45c3aa82045e3d699"},
	{"Courier.font", 11, "original/workbench-3.1/Courier_11.yaff", "dd53dfd84a75084ffc98bc3e0e9f0a849ffb5293"},
	{"Courier.font", 13, "original/workbench-3.1/Courier_13.yaff", "aa0620b3ec27e9f6d2932a98f64bddc25dc00f07"},
	{"Courier.font", 15, "original/workbench-3.1/Courier_15.yaff", "40d3e68d1098e163f63cc06fbec010f9b480c572"},
	{"Courier.font", 18, "original/workbench-3.1/Courier_18.yaff", "7d2248b88722d7f0f6b38a7603bfab5f0830ff54"},
	{"Courier.font", 24, "original/workbench-3.1/Courier_24.yaff", "14dec07fba0077e233ef4235b16fcecee1b617a9"},
	{"Courier.font", 30, "original/workbench-3.1/Courier_30.yaff", "ab586b03cf1986549bef38d9db3b2040e70cdaa5"},
	{"Courier.font", 36, "original/workbench-3.1/Courier_36.yaff", "37234ad13ec8a452a20479717d125d26f2268964"},
}

func skyFontPath(file string) string {
	return filepath.Join(appDataDir(), "amiga-fonts", strings.ReplaceAll(file, "/", "_"))
}

var skyFontURL = func(file string) string {
	return "https://raw.githubusercontent.com/mkrueger/icy_tools/" + skyFontVersion + "/crates/icy_engine/data/fonts/Amiga/" + file
}

// skyFaces are the fonts read so far, by file.
var skyFaces = map[string]*skyFace{}

// skyFaceFile reads a fetched font (nil if it is not here).
func skyFaceFile(file, sum string) *skyFace {
	if f, ok := skyFaces[file]; ok {
		return f
	}
	b, err := os.ReadFile(skyFontPath(file))
	if err != nil || !sha1Is(b, sum) {
		return nil
	}
	f := parseYaff(b)
	skyFaces[file] = f
	return f
}

// skyDefaultFace is the 8x8 font SkyPix text is drawn in: the Amiga 8x8 IBM set when it is
// here, else CharEdit's VGA 8x8.
func skyDefaultFace() *skyFace {
	if f := skyFaceFile(skyFontFiles[0].file, skyFontFiles[0].sha1); f != nil && f.w == 8 && f.h == 8 {
		return f
	}
	return skyFaceOf(fontByID("vga50"))
}

// skyFontByName is a font a file names, nil if CharEdit has none of that name and size here.
func skyFontByName(name string, size int) *skyFace {
	for _, f := range skyFontFiles[1:] {
		if strings.EqualFold(f.name, strings.TrimSpace(name)) && f.size == size {
			return skyFaceFile(f.file, f.sha1)
		}
	}
	return nil
}

// skyFontsMissing are the font files a SkyPix file draws with that are not here.
func skyFontsMissing(b []byte) []int {
	want := map[int]bool{0: true}
	skyRead(b, func(byte) {}, func(byte) {}, func(byte, []int) {}, func(c skyCmd) {
		if c.n != 10 || len(c.vals) == 0 {
			return
		}
		for i, f := range skyFontFiles[1:] {
			if strings.EqualFold(f.name, strings.TrimSpace(c.text)) && f.size == c.vals[0] {
				want[i+1] = true
			}
		}
	})
	var out []int
	for i := range skyFontFiles {
		if want[i] {
			if b, err := os.ReadFile(skyFontPath(skyFontFiles[i].file)); err != nil || !sha1Is(b, skyFontFiles[i].sha1) {
				out = append(out, i)
			}
		}
	}
	return out
}

// fetchSkyFonts fetches fonts (the network part).
func fetchSkyFonts(which []int) error {
	client := &http.Client{Timeout: 20 * time.Second}
	for _, i := range which {
		f := skyFontFiles[i]
		if b, err := os.ReadFile(skyFontPath(f.file)); err == nil && sha1Is(b, f.sha1) {
			continue
		}
		b, err := fetchBody(client, skyFontURL(f.file), 1<<20)
		if err != nil {
			return err
		}
		if !sha1Is(b, f.sha1) {
			return fmt.Errorf("%s is not the font expected", f.file)
		}
		if err := os.MkdirAll(filepath.Dir(skyFontPath(f.file)), 0o755); err != nil {
			return err
		}
		if err := writeAtomic(skyFontPath(f.file), b); err != nil {
			return err
		}
	}
	return nil
}

// parseYaff reads a yaff font (monobit's text format) as IcyDraw does: glyphs by their
// codepoint labels ("0x41:"), then Unicode labels ("u+0041:") for slots still empty; cells
// as wide as the bounding box or cell size but at most 8, as tall as the pixel size, size,
// line height, bounding box or cell size says (first found).
func parseYaff(b []byte) *skyFace {
	f := &skyFace{w: 8, h: 16}
	props := map[string]string{}
	type glyph struct {
		codes, unis []int
		rows        []byte
	}
	var glyphs []*glyph
	var cur *glyph
	inRows := false
	sc := bufio.NewScanner(bytes.NewReader(b))
	sc.Buffer(nil, 1<<20)
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), " \t\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if line[0] != ' ' && line[0] != '\t' { // a label or a property
			if !strings.HasSuffix(line, ":") { // a property of the font: "key: value"
				if k := strings.Index(line, ":"); k > 0 {
					props[strings.ToLower(strings.TrimSpace(line[:k]))] = strings.TrimSpace(line[k+1:])
				}
				cur = nil
				continue
			}
			key := strings.TrimSpace(line[:len(line)-1]) // a label: "0x3a:", "u+003a:", "':':"
			if cur == nil || inRows {
				cur = &glyph{}
				glyphs = append(glyphs, cur)
				inRows = false
			}
			lk := strings.ToLower(key)
			switch {
			case strings.HasPrefix(lk, "u+"):
				if v, err := strconv.ParseInt(lk[2:], 16, 32); err == nil {
					cur.unis = append(cur.unis, int(v))
				}
			case strings.HasPrefix(lk, "0x"):
				if v, err := strconv.ParseInt(lk[2:], 16, 32); err == nil {
					cur.codes = append(cur.codes, int(v))
				}
			case len(lk) > 0 && lk[0] >= '0' && lk[0] <= '9':
				if v, err := strconv.ParseInt(lk, 0, 32); err == nil {
					cur.codes = append(cur.codes, int(v))
				}
			}
			continue
		}
		if cur == nil {
			continue
		}
		t := strings.TrimSpace(line)
		if strings.Contains(t, ":") { // a glyph's own property (bearings ...)
			inRows = true
			continue
		}
		if t == "-" {
			inRows = true
			continue
		}
		var v byte
		for x, c := range t {
			if x < 8 && c == '@' {
				v |= 0x80 >> x
			}
		}
		cur.rows = append(cur.rows, v)
		inRows = true
	}
	pair := func(s string) (int, int, bool) {
		p := strings.Split(s, "x")
		if len(p) != 2 {
			return 0, 0, false
		}
		w, e1 := strconv.Atoi(strings.TrimSpace(p[0]))
		h, e2 := strconv.Atoi(strings.TrimSpace(p[1]))
		return w, h, e1 == nil && e2 == nil
	}
	if w, _, ok := pair(props["bounding-box"]); ok {
		f.w = w
	} else if w, _, ok := pair(props["cell-size"]); ok {
		f.w = w
	}
	f.w = min(max(f.w, 1), 8)
	set := false
	for _, k := range []string{"pixel-size", "size", "line-height"} {
		if v, err := strconv.Atoi(props[k]); err == nil && !set {
			f.h, set = v, true
		}
	}
	for _, k := range []string{"bounding-box", "cell-size"} {
		if _, h, ok := pair(props[k]); ok && !set {
			f.h, set = h, true
		}
	}
	f.h = min(max(f.h, 1), 32)
	filled := [256]bool{}
	for _, g := range glyphs {
		for _, c := range g.codes {
			if c >= 0 && c < 256 {
				copy(f.rows[c][:], g.rows)
				filled[c] = true
			}
		}
	}
	for _, g := range glyphs {
		for _, c := range g.unis {
			if c >= 0 && c < 256 && f.rows[c] == ([32]byte{}) {
				copy(f.rows[c][:], g.rows)
			}
		}
	}
	return f
}
