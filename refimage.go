package main

// The reference picture: a picture shown with a page to trace over, never
// turned into characters (the scene does not take conversions). It shows in
// the empty cells (or faintly over everything), can be faded, hidden, moved
// and scaled, is remembered for the art file (prefs.json "references") and in
// the autosave, and is never part of what is saved or exported.

import (
	"bytes"
	"fmt"
	"image"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/veandco/go-sdl2/sdl"
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"
)

// RefImage is a page's reference picture and how it is shown.
type RefImage struct {
	path    string
	w, h    int
	px      []uint32 // 0xAARRGGBB
	grey    []uint32 // the same in greys, made when first wanted
	x, y    float64  // its top left corner, in pixels of the page's font (zoom 1)
	scale   float64  // page pixels for one picture pixel
	opacity int      // percent
	hidden  bool
	over    bool // over everything (faint), not only in the empty cells
	greyOn  bool
}

// RefPref is a reference as prefs.json and the autosave keep it.
type RefPref struct {
	Path    string  `json:"path"`
	X       float64 `json:"x"`
	Y       float64 `json:"y"`
	Scale   float64 `json:"scale"`
	Opacity int     `json:"opacity"`
	Hidden  bool    `json:"hidden,omitempty"`
	Over    bool    `json:"over,omitempty"`
	Grey    bool    `json:"grey,omitempty"`
	Used    int64   `json:"used,omitempty"` // when, for letting go of the oldest
}

const (
	maxRefPixels = 16 << 20 // a bigger picture is refused (64 MB once decoded)
	maxRefPrefs  = 200      // art files whose reference prefs.json remembers
)

var refOpacities = []int{10, 20, 30, 40, 50, 60, 70, 80, 90, 100}

// isPicture: do the bytes start like a picture the reference can show?
func isPicture(b []byte) bool {
	for _, m := range []string{"\x89PNG", "GIF8", "\xff\xd8\xff", "BM"} {
		if bytes.HasPrefix(b, []byte(m)) {
			return (len(b) > 14 || m != "BM") && !isIcyDraw(b) // an IcyDraw file is art in a PNG
		}
	}
	return len(b) >= 12 && string(b[:4]) == "RIFF" && string(b[8:12]) == "WEBP"
}

// pictureFile: is the file at path a picture?
func pictureFile(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	head := make([]byte, 16)
	n, _ := f.Read(head)
	return isPicture(head[:n])
}

// loadRefImage reads a picture (PNG, JPEG, GIF, BMP, WebP).
func loadRefImage(path string) (*RefImage, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return nil, fmt.Errorf("%s is not a picture CharEdit can read (PNG, JPEG, GIF, BMP, WebP)", filepath.Base(path))
	}
	if cfg.Width < 1 || cfg.Height < 1 || cfg.Width*cfg.Height > maxRefPixels {
		return nil, fmt.Errorf("%s is %dx%d: too big for a reference (16 million pixels at most)", filepath.Base(path), cfg.Width, cfg.Height)
	}
	if _, err := f.Seek(0, 0); err != nil {
		return nil, err
	}
	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %v", filepath.Base(path), err)
	}
	b := img.Bounds()
	rgba := image.NewNRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(rgba, rgba.Rect, img, b.Min, draw.Src)
	r := &RefImage{path: path, w: b.Dx(), h: b.Dy(), px: make([]uint32, b.Dx()*b.Dy()), scale: 1, opacity: 50}
	for i := range r.px {
		p := rgba.Pix[i*4 : i*4+4]
		r.px[i] = uint32(p[3])<<24 | uint32(p[0])<<16 | uint32(p[1])<<8 | uint32(p[2])
	}
	return r, nil
}

// pixels are the picture's pixels as shown (in greys when asked).
func (r *RefImage) pixels() []uint32 {
	if !r.greyOn {
		return r.px
	}
	if r.grey == nil {
		r.grey = make([]uint32, len(r.px))
		for i, p := range r.px {
			l := (299*(p>>16&255) + 587*(p>>8&255) + 114*(p&255)) / 1000
			r.grey[i] = p&0xFF000000 | l<<16 | l<<8 | l
		}
	}
	return r.grey
}

func (r *RefImage) pref() *RefPref {
	return &RefPref{Path: r.path, X: r.x, Y: r.y, Scale: r.scale, Opacity: r.opacity, Hidden: r.hidden, Over: r.over, Grey: r.greyOn}
}

// apply sets how the picture is shown from a kept RefPref.
func (r *RefImage) apply(p *RefPref) {
	r.x, r.y, r.hidden, r.over, r.greyOn = p.X, p.Y, p.Hidden, p.Over, p.Grey
	if p.Scale > 0.001 && p.Scale < 1000 {
		r.scale = p.Scale
	}
	if p.Opacity >= 5 && p.Opacity <= 100 {
		r.opacity = p.Opacity
	}
}

// pageW and pageH are the page's size in pixels of its font.
func (a *App) pagePxW() float64 { return float64(a.doc.cols * a.cellW()) }
func (a *App) pagePxH() float64 { return float64(a.doc.rows * a.cellH()) }

// ---------------------------------------------------------------- opening, keeping

// openRef makes the picture at path the page's reference, fitted to the page's width.
func (a *App) openRef(path string) {
	r, err := loadRefImage(path)
	if err != nil {
		a.msg("No reference: " + err.Error())
		return
	}
	if abs, err := filepath.Abs(path); err == nil {
		r.path = abs
	}
	if a.ref != nil { // a new picture keeps how the old one was shown
		r.opacity, r.over, r.greyOn = a.ref.opacity, a.ref.over, a.ref.greyOn
	}
	a.ref = r
	a.start = false // the page shows, not the start screen
	a.refFit(false)
	a.rememberRef()
	a.msg(fmt.Sprintf("Reference %s (%dx%d), never saved with the art; %s hides it, %s moves it", filepath.Base(path), r.w, r.h, keyName("ref_show"), keyName("ref_move")))
}

// refPrefKey is the art file's name in prefs.json's references.
func refPrefKey(path string) string {
	if p, err := filepath.Abs(path); err == nil {
		return p
	}
	return path
}

// rememberRef keeps the page's reference with its file in prefs.json.
func (a *App) rememberRef() {
	if a.path == "" || viewer != nil {
		return
	}
	key := refPrefKey(a.path)
	if a.ref == nil {
		if _, ok := prefs.Refs[key]; !ok {
			return
		}
		delete(prefs.Refs, key)
	} else {
		if prefs.Refs == nil {
			prefs.Refs = map[string]*RefPref{}
		}
		p := a.ref.pref()
		p.Used = time.Now().Unix()
		prefs.Refs[key] = p
		for len(prefs.Refs) > maxRefPrefs { // let go of the longest unused
			oldest, at := "", int64(math.MaxInt64)
			for k, v := range prefs.Refs {
				if v.Used < at {
					oldest, at = k, v.Used
				}
			}
			delete(prefs.Refs, oldest)
		}
	}
	savePrefs()
}

// restoreRef brings back the reference kept for the page's file (or p, from an autosave).
func (a *App) restoreRef(p *RefPref) {
	if p == nil && a.path != "" {
		p = prefs.Refs[refPrefKey(a.path)]
	}
	if p == nil || p.Path == "" {
		return
	}
	r, err := loadRefImage(p.Path)
	if err != nil {
		a.msg("The page's reference picture could not be shown: " + err.Error())
		return
	}
	r.apply(p)
	a.ref = r
}

// askReference asks whether a picture (dropped or opened) is to be the page's reference.
func askReference(path string) {
	a.dialog = &Confirm{
		text:   "\"" + filepath.Base(path) + "\" is a picture. Use it as this page's reference, to trace over? (It is only shown, never turned into characters or saved with the art.)",
		button: "Use as reference",
		yes: func() {
			a.start = false
			a.openRef(path)
		},
	}
}

// openRefRequester picks a picture with the file requester.
func openRefRequester() {
	d := &FileReq{sel: -1, pick: func(p string) { a.openRef(p) }, title: "Reference picture",
		note: "A picture to trace over: PNG, JPEG, GIF, BMP or WebP"}
	d.listFirst(startDir(a.lastDir))
	d.path.enter = d.enter
	a.dialog = d
	ui.focus = &d.path
}

// ---------------------------------------------------------------- showing

// drawRef draws the reference into the page drawn at (ox, oy): in the empty
// cells of rows y0-y1 and columns x0-x1, or faintly over all of them.
func (a *App) drawRef(fb *FB, ox, oy, x0, x1, y0, y1 int) {
	r := a.ref
	if r == nil || r.hidden || a.playing != nil || viewer != nil {
		return
	}
	cl := fb.clip
	if cl.w <= 0 || cl.h <= 0 {
		return
	}
	z := float64(a.zoom)
	xs := make([]int, cl.w) // the picture's column under each screen column, -1 none
	for i := range xs {
		ix := math.Floor((float64(cl.x+i-ox)/z - r.x) / r.scale)
		xs[i] = -1
		if ix >= 0 && ix < float64(r.w) {
			xs[i] = int(ix)
		}
	}
	ys := make([]int, cl.h)
	for i := range ys {
		iy := math.Floor((float64(cl.y+i-oy)/z - r.y) / r.scale)
		ys[i] = -1
		if iy >= 0 && iy < float64(r.h) {
			ys[i] = int(iy)
		}
	}
	px := r.pixels()
	op := r.opacity * 255 / 100
	colPx, rowPx := a.cellW()*a.zoom, a.cellH()*a.zoom
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			if !r.over && !a.onionBlank(a.doc.get(x, y)) {
				continue
			}
			c := Rect{ox + x*colPx, oy + y*rowPx, colPx, rowPx}.and(cl)
			for sy := c.y; sy < c.y+c.h; sy++ {
				iy := ys[sy-cl.y]
				if iy < 0 {
					continue
				}
				row := fb.px[sy*fb.w : sy*fb.w+fb.w]
				for sx := c.x; sx < c.x+c.w; sx++ {
					ix := xs[sx-cl.x]
					if ix < 0 {
						continue
					}
					p := px[iy*r.w+ix]
					al := op * int(p>>24) / 255
					if al == 0 {
						continue
					}
					q, na := row[sx], 255-al
					row[sx] = 0xFF000000 |
						uint32((int(q>>16&255)*na+int(p>>16&255)*al)/255)<<16 |
						uint32((int(q>>8&255)*na+int(p>>8&255)*al)/255)<<8 |
						uint32((int(q&255)*na+int(p&255)*al)/255)
				}
			}
		}
	}
}

// ---------------------------------------------------------------- changing it

// withRef runs fn on the page's reference and keeps the result; false when there is none.
func (a *App) withRef(fn func(r *RefImage)) bool {
	if a.ref == nil {
		a.msg("The page has no reference picture: " + keyName("ref_open") + " or the Ref gadget opens one, or drop a picture on the window")
		return false
	}
	fn(a.ref)
	a.rememberRef()
	return true
}

func (a *App) refShow() {
	a.withRef(func(r *RefImage) {
		r.hidden = !r.hidden
		a.msg(map[bool]string{true: "Reference hidden", false: "Reference shown"}[r.hidden])
	})
}

func (a *App) refFade(d int) {
	a.withRef(func(r *RefImage) {
		i := 0
		for i < len(refOpacities)-1 && refOpacities[i] < r.opacity {
			i++
		}
		i = max(0, min(len(refOpacities)-1, i+d))
		r.opacity, r.hidden = refOpacities[i], false
		a.msg(fmt.Sprintf("Reference at %d%%", r.opacity))
	})
}

func (a *App) refOver() {
	a.withRef(func(r *RefImage) {
		r.over, r.hidden = !r.over, false
		if r.over && r.opacity > 40 {
			r.opacity = 30
		}
		a.msg(map[bool]string{true: "Reference over everything (faint)", false: "Reference under the art: in the empty cells"}[r.over])
	})
}

func (a *App) refGrey() {
	a.withRef(func(r *RefImage) {
		r.greyOn, r.hidden = !r.greyOn, false
		a.msg(map[bool]string{true: "Reference in greys", false: "Reference in colour"}[r.greyOn])
	})
}

// refFit fits the picture to the page's width, or (whole) all of it in the page.
func (a *App) refFit(whole bool) {
	r := a.ref
	if r == nil {
		return
	}
	W, H := a.pagePxW(), a.pagePxH()
	r.scale = W / float64(r.w)
	if whole && H > 0 {
		r.scale = min(r.scale, H/float64(r.h))
	}
	r.x, r.y = (W-float64(r.w)*r.scale)/2, 0
	r.hidden = false
}

// refScaleBy scales the picture keeping its middle where it is.
func (a *App) refScaleBy(k float64) {
	a.withRef(func(r *RefImage) {
		s := max(0.01, min(100, r.scale*k))
		cx, cy := r.x+float64(r.w)*r.scale/2, r.y+float64(r.h)*r.scale/2
		r.scale = s
		r.x, r.y = cx-float64(r.w)*s/2, cy-float64(r.h)*s/2
		r.hidden = false
		a.msg(fmt.Sprintf("Reference at %.0f%% of the picture's size", s*100))
	})
}

func (a *App) refSize(how string) {
	a.withRef(func(r *RefImage) {
		switch how {
		case "width":
			a.refFit(false)
			a.msg("Reference as wide as the page")
		case "page":
			a.refFit(true)
			a.msg("Reference fits the page")
		default:
			r.scale, r.hidden = 1, false
			a.msg("Reference at 1:1: a picture pixel for a pixel of the font")
		}
	})
}

func (a *App) refRemove() {
	if a.ref == nil {
		a.msg("The page has no reference picture")
		return
	}
	a.ref = nil
	a.refMove = false
	a.rememberRef()
	a.msg("Reference taken away (the picture file stays where it is)")
}

func (a *App) toggleRefMove() {
	if a.refMove {
		a.endRefMove()
		return
	}
	if !a.withRef(func(r *RefImage) { r.hidden = false }) {
		return
	}
	a.refMove = true
	a.msg("Moving the reference: drag it, or arrows (a cell; Shift: a pixel), + and - size it, Return or Esc ends")
}

func (a *App) endRefMove() {
	a.refMove, a.refDrag = false, nil
	a.rememberRef()
	a.msg("Reference stays there")
}

// refKey: the keys while the reference is being moved; false for the others.
func (a *App) refKey(k string, shift bool) bool {
	r := a.ref
	if r == nil {
		a.refMove = false
		return false
	}
	dx, dy := float64(a.cellW()), float64(a.cellH())
	if shift {
		dx, dy = 1, 1
	}
	switch k {
	case "Left":
		r.x -= dx
	case "Right":
		r.x += dx
	case "Up":
		r.y -= dy
	case "Down":
		r.y += dy
	case "=", "+", "Keypad +":
		a.refScaleBy(1.1)
	case "-", "Keypad -":
		a.refScaleBy(1 / 1.1)
	case "Return", "Keypad Enter", "Escape":
		a.endRefMove()
	default:
		return false
	}
	return true
}

func ctrlHeld() bool { return ctrlMod(sdl.GetModState()) }

// refPress starts dragging the reference (in the move mode, or with Ctrl held).
func (a *App) refPress(mx, my int) bool {
	if a.ref == nil || a.ref.hidden || !(a.refMove || ctrlHeld()) {
		return false
	}
	a.refDrag = &[4]int{mx, my, int(math.Round(a.ref.x * 64)), int(math.Round(a.ref.y * 64))}
	return true
}

// refMotion follows the mouse while the reference is dragged.
func (a *App) refMotion(mx, my int) bool {
	if a.refDrag == nil || a.ref == nil {
		return false
	}
	d, z := a.refDrag, float64(a.zoom)
	a.ref.x = float64(d[2])/64 + float64(mx-d[0])/z
	a.ref.y = float64(d[3])/64 + float64(my-d[1])/z
	return true
}

func (a *App) refRelease() {
	if a.refDrag != nil {
		a.refDrag = nil
		a.rememberRef()
	}
}

// ---------------------------------------------------------------- the Ref gadget

// refMenu opens the Ref gadget's list under r.
func (a *App) refMenu(u *UI, r Rect) {
	type item struct {
		label string
		do    func()
	}
	items := []item{{"Open picture ...", openRefRequester}}
	if ref := a.ref; ref != nil {
		items = append(items,
			item{map[bool]string{true: "Show", false: "Hide"}[ref.hidden], a.refShow},
			item{fmt.Sprintf("Fainter (%d%%)", ref.opacity), func() { a.refFade(-1) }},
			item{"Stronger", func() { a.refFade(1) }},
			item{map[bool]string{true: "Under the art", false: "Over everything"}[ref.over], a.refOver},
			item{map[bool]string{true: "In colour", false: "In greys"}[ref.greyOn], a.refGrey},
			item{"Fit width", func() { a.refSize("width") }},
			item{"Fit page", func() { a.refSize("page") }},
			item{"1:1", func() { a.refSize("1") }},
			item{"Bigger", func() { a.refScaleBy(1.1) }},
			item{"Smaller", func() { a.refScaleBy(1 / 1.1) }},
			item{"Move ...", a.toggleRefMove},
			item{"Take away", a.refRemove},
		)
	}
	var labels []string
	w := r.w
	for _, it := range items {
		labels = append(labels, it.label)
		w = max(w, textW(it.label)+40)
	}
	h := len(labels)*(lineH+2) + 4
	px := min(r.x, max(0, u.fb.w-w))
	u.popup = &Popup{id: "ref", r: Rect{px, r.y + r.h, w, h}, items: labels, cur: -1, pick: func(i int) { items[i].do() }}
}

// refGadget draws the Ref gadget in the bar.
func (a *App) refGadget(u *UI, r Rect) {
	on := a.ref != nil && !a.ref.hidden
	hint := "Reference picture to trace over: open, show or hide, fade, move, size (" + keyName("ref_open") + ", " + keyName("ref_show") + ")"
	if a.ref != nil {
		hint = "Reference: " + filepath.Base(a.ref.path) + fmt.Sprintf(", %d%%", a.ref.opacity) + strings.Repeat(", hidden", map[bool]int{true: 1}[a.ref.hidden]) + ". Click for the choices"
	}
	if u.button(r, "Ref", on, hint) != nil {
		a.refMenu(u, r)
	}
}
