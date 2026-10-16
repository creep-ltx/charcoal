package main

import (
	"bytes"
	"encoding/json"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// vdFRandom is a Viewdata page of random codes, attributes among them.
func vdFRandom(r *rand.Rand) *Doc {
	d := newVDDoc()
	for i := range d.cells {
		c := 0x20
		switch r.Intn(4) {
		case 0:
			c = r.Intn(0x20)
		case 1:
			c = 0x20 + r.Intn(0x60)
		}
		d.cells[i] = makeCell(c, 0, 0, 0)
	}
	return d
}

func vdFSame(t *testing.T, what string, want, got *Doc) {
	t.Helper()
	if got.rows != want.rows {
		t.Fatalf("%s: %d rows, want %d", what, got.rows, want.rows)
	}
	for i := 0; i < want.rows*40; i++ {
		if cellCh(want.cells[i])&0x7F != cellCh(got.cells[i])&0x7F {
			t.Fatalf("%s: cell %d,%d is %02X, want %02X", what, i%40, i/40, cellCh(got.cells[i]), cellCh(want.cells[i]))
		}
	}
}

func TestViewdataRoundTrips(t *testing.T) {
	r := rand.New(rand.NewSource(7))
	for i := 0; i < 100; i++ {
		d := vdFRandom(r)
		if i%10 == 0 { // full rows, empty rows and an empty end
			for x := 0; x < 40; x++ {
				d.cells[3*40+x] = makeCell('A'+x%26, 0, 0, 0)
				d.cells[4*40+x] = makeCell(0x20, 0, 0, 0)
				d.cells[23*40+x] = makeCell(0x20, 0, 0, 0)
			}
		}
		s := writeVDStream(d, true)
		if bytes.ContainsFunc(s, func(r rune) bool { return r >= 0x80 }) {
			t.Fatal("the stream is not 7 bits")
		}
		got := parseVDStream(s)
		if len(got) != 1 {
			t.Fatalf("page %d: %d frames", i, len(got))
		}
		vdFSame(t, "stream", d, got[0])
		vdFSame(t, ".bin", d, parseVDBin(writeVDBin(d)))
		if bytes.IndexByte(writeVDBin(d), 0x0C) >= 0 {
			t.Fatal("a .bin frame holds a CS byte")
		}
		tel, err := writeTelstar(d, nil, "200b.json")
		if err != nil {
			t.Fatal(err)
		}
		back, note := parseTelstar(tel)
		if note != "" {
			t.Fatal(note)
		}
		vdFSame(t, "Telstar", d, back)
	}
}

func TestViewdataFiles(t *testing.T) {
	// MacViewdata's frames: attributes as 0xC0-0xDF.
	b := bytes.Repeat([]byte{' '}, 960)
	b[0], b[1], b[41] = 0xC1, 'A', 0x9D // red text, A; new background in BBC form
	d := parseVDBin(b)
	if cellCh(d.get(0, 0)) != 1 || cellCh(d.get(1, 0)) != 'A' || cellCh(d.get(1, 1)) != 0x1D {
		t.Errorf("% X", []int{cellCh(d.get(0, 0)), cellCh(d.get(1, 0)), cellCh(d.get(1, 1))})
	}
	if !isVDBin(b, "f.bin") || isVDBin(b, "f.txt") || isVDBin(bytes.Repeat([]byte{'x', 0x07}, 480), "pc.bin") || isVDBin(bytes.Repeat([]byte{'x', 0xB1}, 480), "pc.bin") {
		t.Error("isVDBin")
	}
	// Two pages in a carousel stream: CS between them.
	pages := parseVDStream([]byte("\x0cONE\r\n\x1bAred\x0cTWO"))
	if len(pages) != 2 || cellCh(pages[1].get(0, 0)) != 'T' || cellCh(pages[0].get(0, 1)) != 1 {
		t.Fatalf("%d pages", len(pages))
	}
	// A Telstar frame keeps its other fields when saved again; a new one is numbered by its name.
	old := []byte(`{"pid": {"page-no": 290, "frame-id": "b"}, "routing-table": [1, 2], "content": {"data": "\u001bAHello", "type": "rawV"}}`)
	if !isTelstar(old) {
		t.Fatal("not seen as Telstar")
	}
	d, _ = parseTelstar(old)
	if cellCh(d.get(0, 0)) != 1 || cellCh(d.get(1, 0)) != 'H' {
		t.Fatal("rawV not read")
	}
	d.set(1, 0, makeCell('J', 0, 0, 0))
	out, err := writeTelstar(d, old, "x.json")
	if err != nil {
		t.Fatal(err)
	}
	var f map[string]any
	json.Unmarshal(out, &f)
	if f["routing-table"] == nil || f["pid"].(map[string]any)["page-no"].(float64) != 290 {
		t.Errorf("fields lost: %s", out)
	}
	if !strings.Contains(string(out), `\u001bAJello`) {
		t.Errorf("content: %s", out)
	}
	out, _ = writeTelstar(d, nil, "123c.json")
	json.Unmarshal(out, &f)
	if p := f["pid"].(map[string]any); p["page-no"].(float64) != 123 || p["frame-id"] != "c" {
		t.Errorf("pid: %v", p)
	}
	// edit.tf and rawT content.
	tt := newTTDoc()
	tt.set(0, 23, makeCell(0x11, 0, 0, 0))
	link := "https://edit.tf/#" + editTFHash(tt, false)
	d, note := parseTelstar([]byte(`{"pid": {}, "content": {"data": "` + link + `", "type": "edit.tf"}}`))
	if note != "" || cellCh(d.get(0, 23)) != 0x11 || d.rows != 24 {
		t.Errorf("edit.tf: %q", note)
	}
	_, note = parseTelstar([]byte(`{"pid": {}, "content": {"data": "[R]x", "type": "markup"}}`))
	if note == "" {
		t.Error("markup: no note")
	}
}

// TestViewdataTelstarFrames reads the frames under CHARCOAL_TELSTAR_FILES
// (Telstar's own, github.com/johnnewcombe/telstar-2) and saves each again.
func TestViewdataTelstarFrames(t *testing.T) {
	dir := os.Getenv("CHARCOAL_TELSTAR_FILES")
	if dir == "" {
		t.Skip("CHARCOAL_TELSTAR_FILES not set")
	}
	n := 0
	filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || filepath.Ext(p) != ".json" {
			return nil
		}
		b, _ := os.ReadFile(p)
		if !isTelstar(b) {
			return nil
		}
		n++
		d, note := parseTelstar(b)
		out, err := writeTelstar(d, b, filepath.Base(p))
		if err != nil {
			t.Fatal(err)
		}
		back, note2 := parseTelstar(out)
		if note == "" {
			vdFSame(t, p, d, back)
		} else if note2 != "" {
			t.Logf("%s: %s", filepath.Base(p), note)
		}
		return nil
	})
	t.Logf("%d Telstar frames", n)
}
