package main

import (
	"testing"
	"time"
)

// FuzzParsers sends the same bytes to every parser: none may crash, take more than
// a second, or make a page larger than maxPageCells. Run it under a memory limit:
//
//	systemd-run --user --scope -q -p MemoryMax=6G timeout 200 go test -run XXX -fuzz FuzzParsers -fuzztime 120s -parallel 2 .
func FuzzParsers(f *testing.F) {
	for _, s := range []string{
		"",
		"\x1b[99999999Bx",
		"\x1b[1000000000000000000X",
		"\x1b[99999999C\x1b[99999999Lx\x1b[5Mx",
		"\x16\x01\x9f\x16\x02A\x19B\x10",
		"@X1Fhi@CLS@@POS:99999@x\t",
		"XBIN\x1a\x50\x00\xff\xff\x10\x04",
		"\x041.4\x00\x00\x00\x00\x4f\x00\x18\x00",
		"\x18TUNDRA24\x01\x00\x01\x86\x9f\x00\x00\x00\x00A",
		`{"framebufs":[{"width":100000000,"height":100000000}]}`,
		"unsigned char a[]={0,0,1,2};\n// META: 1 1 C64 upper\n",
		"\x11\x11\x11\x9d\x9c\x1d\x9b",
		"\x1bY\x7f\x7fx\x1bL\x1bM",
	} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		check := func(name string, run func() []*Doc) {
			start := time.Now()
			docs := run()
			if el := time.Since(start); el > time.Second {
				t.Fatalf("%s took %v", name, el)
			}
			for _, d := range docs {
				if d == nil {
					continue
				}
				if d.rows*d.cols > maxPageCells || d.rows > maxPageRows {
					t.Fatalf("%s made a page of %dx%d", name, d.cols, d.rows)
				}
				if len(d.cells) < d.rows*d.cols {
					t.Fatalf("%s: %d cells for %dx%d", name, len(d.cells), d.cols, d.rows)
				}
			}
		}
		one := func(d *Doc) []*Doc { return []*Doc{d} }
		screens := func(s []PetScreen, _ error) []*Doc {
			var out []*Doc
			for _, p := range s {
				out = append(out, p.doc)
			}
			return out
		}
		own := func(d *Doc, _ XBinInfo, _ error) []*Doc { return one(d) }
		check("ansi amiga", func() []*Doc { d, _, _ := parseArt(b, ParseOpts{amiga: true}); return one(d) })
		check("ansi pc", func() []*Doc { d, _, _ := parseArt(b, ParseOpts{}); return one(d) })
		check("bin", func() []*Doc { d, _, _ := parseArt(b, ParseOpts{binary: true}); return one(d) })
		check("ata", func() []*Doc { return one(parseAta(b, 40)) })
		check("pcb", func() []*Doc { return one(parsePCBoard(b, 80)) })
		check("avt", func() []*Doc { return one(parseAvatar(b, 80)) })
		check("idf", func() []*Doc { return own(parseIDF(b)) })
		check("adf", func() []*Doc { return own(parseADF(b)) })
		check("tnd", func() []*Doc { return own(parseTundra(b)) })
		check("seq", func() []*Doc { d, _ := parseSeq(b, 40); return one(d) })
		check("edit.tf", func() []*Doc { d, _ := parseEditTF(b); return one(d) })
		check("tti", func() []*Doc { return one(parseTTI(b)) })
		check("ep1", func() []*Doc { return one(parseEP1(b)) })
		check("mode7", func() []*Doc { return one(parseMode7(b)) }) // any length (Open as)
		check("scr", func() []*Doc { d, _ := parseSCR(b); return one(d) })
		check("marq", func() []*Doc { return screens(parseMarq(b)) })
		if scr, err := parseMarq(b); err == nil && len(scr) <= 64 && scr[0].doc.cols*scr[0].doc.rows <= 64000 {
			docs := screens(scr, nil)
			if back, err := parseMarq(writeMarqFrames(docs, false, "c64")); err != nil || len(back) != len(docs) {
				t.Fatalf("marq frames: %d written, %d read back (%v)", len(docs), len(back), err)
			}
		}
		check("xbin", func() []*Doc { return own(parseXBin(b)) })
		check("petmate", func() []*Doc { return screens(parsePetmate(b)) })
		check("prg", func() []*Doc { d, _, _ := parsePrg(b); return one(d) })
		check("vt52", func() []*Doc { return one(parseVT52(b, stResolutions[0], 0)) })
		for _, mode := range []string{"pc", "c64", "atari", "st"} {
			docs, holds, _ := splitStream(b, mode, 40, false, stResolutions[0], true, 14400)
			total := 0
			for _, d := range docs {
				total += d.cols * d.rows
			}
			if len(docs) > maxSplitFrames || total > maxPageCells || len(holds) != len(docs) {
				t.Fatalf("split %s: %d frames, %d cells", mode, len(docs), total)
			}
		}
	})
}
