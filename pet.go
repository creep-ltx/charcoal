package main

// PETSCII: the Commodore 64 screen. A cell's byte is a screen code (128-255
// reversed), its foreground one of the 16 C64 colours; the background is one
// colour for the whole screen (Doc.bg). Files are .seq: the bytes a C64 BBS
// sends, printed by the C64's screen editor.
//
// The character set is the C64's character ROM, which is not ours to pass on:
// it is read from chargen.bin in ~/.local/share/charcool/ (or the copies that
// bbmp and VICE keep), see c64Paths. downloadChargen fetches it from the VICE
// emulator's own repository, which distributes it.

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

func c64Paths() []string {
	home, _ := os.UserHomeDir()
	data := os.Getenv("XDG_DATA_HOME")
	if data == "" {
		data = filepath.Join(home, ".local", "share")
	}
	return []string{
		filepath.Join(data, "charcool", "chargen.bin"),
		filepath.Join(data, "bbmp", "extras", "chargen.bin"),
		"/usr/share/vice/C64/chargen-901225-01.bin",
		"/usr/share/vice/C64/chargen",
		"/usr/lib/vice/C64/chargen",
	}
}

// loadC64Fonts adds the two C64 character sets to fonts, if the ROM is found.
// Rows are doubled to the 16-row form the other fonts use.
func loadC64Fonts() bool {
	if fontByID("c64upper") != nil {
		return true
	}
	for _, p := range c64Paths() {
		b, err := os.ReadFile(p)
		if err != nil || len(b) != 4096 {
			continue
		}
		for set, f := range []struct{ id, name string }{{"c64upper", "C64 upper case + graphics"}, {"c64lower", "C64 lower + upper case"}} {
			bits := make([]byte, 4096)
			for c := 0; c < 256; c++ {
				for r := 0; r < 8; r++ {
					v := b[set*2048+c*8+r]
					bits[c*16+r*2], bits[c*16+r*2+1] = v, v
				}
			}
			fonts = append(fonts, &Font{id: f.id, name: f.name, kind: "c64", syncterm: -1, bits: bits})
		}
		return true
	}
	return false
}

// The VICE project's copies of the C64 character ROM (901225-01) and its SHA-1.
var chargenURLs = []string{
	"https://sourceforge.net/p/vice-emu/code/HEAD/tree/trunk/vice/data/C64/chargen-901225-01.bin?format=raw",
	"https://raw.githubusercontent.com/VICE-Team/svn-mirror/main/vice/data/C64/chargen-901225-01.bin",
}

const chargenSHA1 = "adc7c31e18c7c7413d54802ef2f4193da14711aa"

// downloadChargen fetches the character ROM from VICE, checks it and keeps it
// as the first of c64Paths.
func downloadChargen() error {
	client := &http.Client{Timeout: 20 * time.Second}
	var last error
	for _, u := range chargenURLs {
		resp, err := client.Get(u)
		if err != nil {
			last = err
			continue
		}
		b, err := io.ReadAll(io.LimitReader(resp.Body, 65536))
		resp.Body.Close()
		if err != nil || resp.StatusCode != 200 {
			last = fmt.Errorf("%s: %s", u, resp.Status)
			continue
		}
		sum := sha1.Sum(b)
		if len(b) != 4096 || hex.EncodeToString(sum[:]) != chargenSHA1 {
			last = fmt.Errorf("the file from %s is not the C64 character ROM", u)
			continue
		}
		dst := c64Paths()[0]
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		if err := writeAtomic(dst, b); err != nil {
			return err
		}
		if !loadC64Fonts() {
			return fmt.Errorf("saved %s but could not read it back", dst)
		}
		return nil
	}
	return last
}

// The colour codes of PETSCII, by C64 colour number.
var petColour = [16]byte{0x90, 0x05, 0x1C, 0x9F, 0x9C, 0x1E, 0x1F, 0x9E, 0x81, 0x95, 0x96, 0x97, 0x98, 0x99, 0x9A, 0x9B}

func petColourOf(c byte) int {
	for i, v := range petColour {
		if v == c {
			return i
		}
	}
	return -1
}

// petToScreen turns a printable PETSCII byte into a screen code (without the reverse bit); -1 if it is a control.
func petToScreen(c byte) int {
	switch {
	case c >= 0x20 && c <= 0x3F:
		return int(c)
	case c >= 0x40 && c <= 0x5F:
		return int(c) - 0x40
	case c >= 0x60 && c <= 0x7F:
		return int(c) - 0x20
	case c >= 0xA0 && c <= 0xBF:
		return int(c) - 0x40
	case c >= 0xC0 && c <= 0xFE:
		return int(c) - 0x80
	case c == 0xFF:
		return 0x5E
	}
	return -1
}

// screenToPet is the PETSCII byte that prints a screen code (0-127).
func screenToPet(sc int) byte {
	switch {
	case sc < 0x20:
		return byte(sc + 0x40)
	case sc < 0x40:
		return byte(sc)
	case sc < 0x60:
		return byte(sc + 0x80)
	default:
		return byte(sc + 0x40)
	}
}

// PetTerm prints PETSCII into a Doc, like the C64's screen editor. As with
// ANSI, the page is endless downwards instead of scrolling.
type PetTerm struct {
	doc      *Doc
	x, y, fg int
	rev      bool
	lower    int // the character set asked for: -1 none, 0 upper, 1 lower
}

func newPetTerm(d *Doc) *PetTerm { return &PetTerm{doc: d, fg: 14, lower: -1} }

func (t *PetTerm) cursor() (int, int) { return t.x, t.y }

func (t *PetTerm) write(b []byte) {
	d := t.doc
	for _, c := range b {
		if col := petColourOf(c); col >= 0 {
			t.fg = col
			continue
		}
		switch c {
		case 0x12:
			t.rev = true
		case 0x92:
			t.rev = false
		case 0x0D, 0x8D:
			t.x, t.y, t.rev = 0, t.y+1, false
		case 0x93:
			fillCells(d.cells[:d.rows*d.cols], blank)
			t.x, t.y = 0, 0
		case 0x13:
			t.x, t.y = 0, 0
		case 0x11:
			t.y++
		case 0x91:
			t.y = max(0, t.y-1)
		case 0x1D:
			if t.x++; t.x >= d.cols {
				t.x, t.y = 0, t.y+1
			}
		case 0x9D:
			if t.x--; t.x < 0 {
				if t.y > 0 {
					t.x, t.y = d.cols-1, t.y-1
				} else {
					t.x = 0
				}
			}
		case 0x0E:
			t.lower = 1
		case 0x8E:
			t.lower = 0
		case 0x14: // delete
			if t.x > 0 {
				for x := t.x - 1; x < d.cols-1; x++ {
					d.set(x, t.y, d.get(x+1, t.y))
				}
				d.set(d.cols-1, t.y, blank)
				t.x--
			}
		case 0x94: // insert
			for x := d.cols - 1; x > t.x; x-- {
				d.set(x, t.y, d.get(x-1, t.y))
			}
			d.set(t.x, t.y, blank)
		default:
			sc := petToScreen(c)
			if sc < 0 {
				continue
			}
			if t.rev {
				sc |= 0x80
			}
			d.set(t.x, t.y, makeCell(sc, t.fg, 0, 0))
			if t.x++; t.x >= d.cols {
				t.x, t.y = 0, t.y+1
			}
		}
	}
}

// parseSeq reads a .seq file; it returns the character set it asks for (-1, 0 upper, 1 lower).
func parseSeq(b []byte, cols int) (*Doc, int) {
	if cols <= 0 {
		cols = 40
	}
	d := newDoc(cols, 25)
	t := newPetTerm(d)
	t.write(b)
	d.trim(1)
	return d, t.lower
}

// writeSeq gives the page as PETSCII: colour and reverse codes only where they change,
// RETURN after a row that does not fill the width.
func writeSeq(d *Doc, lower, clear bool) []byte {
	var out []byte
	if clear {
		out = append(out, 0x93)
	}
	if lower {
		out = append(out, 0x0E)
	} else {
		out = append(out, 0x8E)
	}
	fg, rev := -1, false
	rows := d.usedRows()
	for y := 0; y < rows; y++ {
		last := d.lastCol(y)
		for x := 0; x <= last; x++ {
			v := d.get(x, y)
			sc := cellCh(v)
			if r := sc&0x80 != 0; r != rev {
				rev = r
				if r {
					out = append(out, 0x12)
				} else {
					out = append(out, 0x92)
				}
			}
			if sc != 0x20 && cellFg(v) != fg { // a space shows no colour
				fg = cellFg(v)
				out = append(out, petColour[fg])
			}
			out = append(out, screenToPet(sc&0x7F))
		}
		if last < d.cols-1 {
			out = append(out, 0x0D)
			rev = false // RETURN ends reverse on a C64
		}
	}
	return out
}

// petFromRune is the screen code a typed character gives, -1 if none.
func petFromRune(r rune, lower bool) int {
	switch {
	case r >= 'A' && r <= 'Z':
		if lower {
			return int(r-'A') + 0x41
		}
		return int(r-'A') + 1
	case r >= 'a' && r <= 'z':
		return int(r-'a') + 1
	case r >= 0x20 && r <= 0x3F:
		return int(r)
	}
	switch r {
	case '@':
		return 0
	case '[':
		return 0x1B
	case '£':
		return 0x1C
	case ']':
		return 0x1D
	case '↑', '^':
		return 0x1E
	case '←':
		return 0x1F
	case 'π':
		if !lower {
			return 0x5E
		}
	}
	return -1
}

// petRune is a Unicode stand-in for a screen code, for copying as text.
func petRune(sc int, lower bool) rune {
	rev := sc&0x80 != 0
	sc &= 0x7F
	switch {
	case sc == 0x20 && rev:
		return '█'
	case sc == 0:
		return '@'
	case sc < 0x1B:
		if lower {
			return rune('a' + sc - 1)
		}
		return rune('A' + sc - 1)
	case sc < 0x40:
		return []rune("[£]↑← !\"#$%&'()*+,-./0123456789:;<=>?")[sc-0x1B]
	case lower && sc >= 0x41 && sc <= 0x5A:
		return rune('A' + sc - 0x41)
	}
	if r, ok := map[int]rune{0x40: '─', 0x5D: '│', 0x5B: '┼', 0x70: '┌', 0x6E: '┐', 0x6D: '└', 0x7D: '┘', 0x6B: '├',
		0x73: '┤', 0x71: '┴', 0x72: '┬', 0x61: '▌', 0x62: '▄', 0x66: '▒', 0x55: '╭', 0x49: '╮', 0x4A: '╰', 0x4B: '╯',
		0x4D: '╲', 0x4E: '╱', 0x56: '╳', 0x51: '●', 0x57: '○', 0x53: '♥', 0x5A: '♦', 0x58: '♣', 0x41: '♠', 0x5E: 'π'}[sc]; ok {
		return r
	}
	return '▒'
}

var c64Palette = pal("c64", "C64 (Pepto)", 0x000000, 0xFFFFFF, 0x68372B, 0x70A4B2, 0x6F3D86, 0x588D43, 0x352879, 0xB8C76F,
	0x6F4F25, 0x433900, 0x9A6759, 0x444444, 0x6C6C6C, 0x9AD284, 0x6C5EB5, 0x959595)

var c64Fkeys = [][]int{
	{0xA0, 0x66, 0x61, 0xE1, 0x62, 0xE2, 0x6C, 0x7B, 0x7E, 0x7C},
	{0x40, 0x5D, 0x70, 0x6E, 0x6D, 0x7D, 0x6B, 0x73, 0x71, 0x72},
	{0x55, 0x49, 0x4A, 0x4B, 0x5B, 0x4D, 0x4E, 0x5F, 0x69, 0x56},
	{0x51, 0x57, 0x53, 0x5A, 0x58, 0x41, 0x5E, 0x4F, 0x50, 0x7A},
	{0x63, 0x64, 0x65, 0x67, 0x68, 0x6A, 0x74, 0x75, 0x76, 0x77},
}
