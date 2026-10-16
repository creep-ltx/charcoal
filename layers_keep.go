package main

// Keeping layers: in the autosave, and beside a file of one picture (an .ans
// saved from a page of layers). The side file is the work stages' idea
// (stages.go): <data dir>/layers/<sha1 of the path>.json holds the layers with
// the SHA-1 of the bytes written; opening the file takes them back only if it
// still has those bytes and reads as the same page type and width.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const layersVer = 1

// layersFile is the side file of a file of one picture saved from a page of layers.
type layersFile struct {
	Ver    int              `json:"version"`
	Path   string           `json:"path"`
	SHA1   string           `json:"sha1"` // of the file's bytes as written
	Mode   string           `json:"mode"`
	Lay    int              `json:"layer"` // the layer worked on
	Layers []layersFileItem `json:"layers"`
}

type layersFileItem struct {
	Name   string     `json:"name,omitempty"`
	Hidden bool       `json:"hidden,omitempty"`
	Clear  int        `json:"clear,omitempty"`
	Keep   *layerKeep `json:"keep,omitempty"`
	Pic    stagePic   `json:"picture"`
}

func layersDir() string { return filepath.Join(appDataDir(), "layers") }

func layersSideFile(path string) string {
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	return filepath.Join(layersDir(), sha1Hex([]byte(path))+".json")
}

// writeLayersFile keeps the tab's layers beside its file (path, now holding data,
// written as fmtID); a page of one layer, or a format that keeps layers itself,
// removes an old side file instead.
func writeLayersFile(t *Tab, path string, data []byte, fmtID string) error {
	if t.layers == nil || layersKeptBy(fmtID, t.mode) {
		os.Remove(layersSideFile(path))
		return nil
	}
	if err := os.MkdirAll(layersDir(), 0o700); err != nil {
		return err
	}
	f := layersFile{Ver: layersVer, Path: path, SHA1: sha1Hex(data), Mode: t.mode, Lay: t.lay}
	for i, l := range t.layers {
		f.Layers = append(f.Layers, layersFileItem{Name: l.name, Hidden: l.hidden, Clear: l.clear, Keep: l.keep, Pic: packDoc(t.layerDoc(i), 0)})
	}
	js, err := json.Marshal(&f)
	if err != nil {
		return err
	}
	return writeAtomic(layersSideFile(path), js)
}

// readLayersFile reads a side file and unpacks its layers; it is checked so that a damaged one is none.
func readLayersFile(p string) (*layersFile, []Layer, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, nil, err
	}
	var f layersFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, nil, err
	}
	if f.Ver != layersVer || len(f.Layers) < 2 || len(f.Layers) > maxLayers || f.Lay < 0 || f.Lay >= len(f.Layers) {
		return nil, nil, errors.New("not a layers file")
	}
	ls := make([]Layer, len(f.Layers))
	cells := 0
	for i, it := range f.Layers {
		d := it.Pic.doc()
		if d == nil {
			return nil, nil, errors.New("a damaged layer")
		}
		if cells += d.cols * d.rows; cells > maxPageCells || d.cols != f.Layers[0].Pic.Cols {
			return nil, nil, errors.New("layers too large or not of one width")
		}
		ls[i] = Layer{doc: d, name: it.Name, hidden: it.Hidden, clear: min(max(it.Clear, 0), layerBlack), keep: it.Keep}
	}
	return &f, ls, nil
}

// sameLook: two pages of one width look the same (cell for cell, or both
// empty there; a row past the end of one is empty).
func sameLook(x, y *Doc) bool {
	if x.cols != y.cols {
		return false
	}
	for r := range max(x.rows, y.rows) {
		for c := range x.cols {
			if v, w := x.get(c, r), y.get(c, r); v != w && !(x.blankAt(v) && y.blankAt(w)) {
				return false
			}
		}
	}
	return true
}

// restoreLayersFile takes back the layers of the file just opened (bytes b), if
// they were kept for exactly these bytes and look as the page does.
func (a *App) restoreLayersFile(path string, b []byte) {
	if a.layers != nil || a.frames != nil || !a.layered() {
		return
	}
	f, ls, err := readLayersFile(layersSideFile(path))
	if err != nil || f.SHA1 != sha1Hex(b) || f.Mode != a.mode {
		return
	}
	cur := ls[f.Lay].doc
	cur.ensureRows(max(a.doc.rows, f.Layers[0].Pic.Rows))
	layerFields(cur, a.doc)
	// The bytes are the ones written from these layers, read as the same page
	// type and width: the page is them put together (as far as the format keeps
	// cells: an ATASCII file has no colours in them, so cells are not compared).
	if cur.cols != a.doc.cols {
		return
	}
	a.layers, a.lay, a.doc = ls, f.Lay, cur.clone()
	a.layers[f.Lay].doc = cur
	a.msg(fmt.Sprintf("Opened %s with its %d layers", filepath.Base(path), len(ls)))
}

// layersSaveNote is what saving a page of layers in a format of one picture adds to the message.
func (t *Tab) layersSaveNote(fmtID string, own bool) string {
	if t.layers == nil || layersKeptBy(fmtID, t.mode) {
		return ""
	}
	hidden := 0
	for _, l := range t.layers {
		if l.hidden {
			hidden++
		}
	}
	s := fmt.Sprintf("; its %d layers put together", len(t.layers))
	if own {
		s += " (kept beside the file for CharEdit)"
	}
	if hidden > 0 {
		s += fmt.Sprintf(", %d hidden layers left out", hidden)
	}
	return s
}

// ---------------------------------------------------------------- autosave

// autosaveLayer is one layer of an autosaved page.
type autosaveLayer struct {
	Cols   int        `json:"cols"`
	Rows   int        `json:"rows"`
	Name   string     `json:"name,omitempty"`
	Hidden bool       `json:"hidden,omitempty"`
	Clear  int        `json:"clear,omitempty"`
	Keep   *layerKeep `json:"keep,omitempty"`
}
