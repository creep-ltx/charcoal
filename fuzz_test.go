package main

import (
	"os"
	"path/filepath"
	"strings"
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
	{ // other editors' files (editorformats.go)
		d := newDoc(3, 2)
		d.cells[1] = makeCell(1, 4, 5, 0)
		pal := paletteByID("vga").colors
		f.Add(writeIcy(d, pal, fontByID("vga"), true, nil))
		f.Add(writeXP(d, pal, true))
		out, _ := writePsci([]*Doc{d}, nil, psciPage{mode: "c64", machine: "c64"})
		f.Add(out)
		f.Add(writeMarqPet(d, false))
		f.Add(writePetmateJSON([]PetScreen{{doc: d}}))
		pd, _ := writePD(d, pal, fontByID("vga"), true)
		f.Add(pd)
		f.Add(writeBSV(d))
		f.Add([]byte(skyFDrawing))
		for _, buf := range []int{icyPETSCII, icyATASCII, icyAtariST} { // other machines' pages (icymach.go)
			o := &icyOut{buffer: buf, fontMod: 1, w: 3, h: 2, fontH: 8, pal: []uint32{0, 0xFFFFFF},
				screen: icyScreenRecord(map[int]int{icyPETSCII: 5, icyATASCII: 9, icyAtariST: 8}[buf], 1, 3, 113),
				cell:   func(x, y int) (uint32, int, int, byte, uint16) { return uint32(x + 130), 100, 0, 1, 8 }}
			f.Add(o.encode(nil))
		}
	}
	{ // the Unicode page's files (uniformats.go)
		d := newDoc(3, 2)
		d.uni = true
		d.cells[1] = uniCell(0x2588, styleOf(196, rgbFlag|0x123456, underline))
		f.Add(writeUniAnsi(d))
		out, _ := writeDur([]*Doc{d, d}, []int{100, 250}, "t", "a")
		f.Add(out)
		f.Add([]byte(`{"DurMovie":{"formatVersion":7,"colorFormat":"256","sizeX":2,"sizeY":2,"framerate":0,"frames":[{"delay":0,"contents":["ab"],"colorMap":[[[1,2]]]}]}}`))
		m, _, _ := writeMirc(d, &vga16)
		f.Add(m)
		f.Add([]byte("\x0304,12a\x03\x02b\x04FF0000,00ff00c\x1d\x1f\x16\x0f\x0399,99\x031,"))
	}
	{ // Amstrad CPC (cpc.go)
		d, s := newCPCDoc(1), newCPCSetup(1)
		d.cells[3] = makeCell(5, 2, 3, 0)
		bin := writeCPCBin(d, s, "F")
		f.Add(bin)
		dsk, _ := writeDSK("F", bin)
		f.Add(dsk)
		out, _ := writePsciCPC([]*Doc{d}, nil, s)
		f.Add(out)
		f.Add([]byte("\x04\x00\x1c\x01\x02\x02\x1f\x05\x05\x01\x01\x19A12345678\x1a\x01\x02\x03\x04\x0b\x0bX"))
	}
	{ // MSX and Sharp MZ-700 (msx.go, mz.go)
		d, s := newMSXDoc(1), newMSXSetup(1)
		d.cells[3] = makeCell(0xDB, 0, 0, 0)
		f.Add(writeMSXROM(msxDataOf(d, s, nil)))
		f.Add(writeMSXBin(msxDataOf(d, s, make([]byte, 2048))))
		f.Add(writePsciMSX([]*Doc{d}, nil, s))
		m := newMZDoc()
		m.cells[5] = mzCell(300, 2, 5)
		f.Add(writeMZF(m, "F"))
		f.Add([]byte("\x01IMAGE\r\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x10\x00\x00\x12\x00\x12!\x00\x12\x00\xd0"))
	}
	{ // Minitel and Viewdata (minitel.go, viewdata.go)
		d := newMTDoc()
		d.cells[45] = makeCell('A', 3, 0, mtDW|mtDH)
		f.Add(mtWrite(d))
		f.Add([]byte("\x0c\x1f\x42\x45\x1b\x41H\x19\x42e\x0e\x1b\x54\x7f\x12\x43\x1b[2;5H\x1b[3P\x1f\x40\x41AB\x0a\x1f\x23 x\x1f\x41\x41"))
		f.Add(writeVDStream(newVDDoc(), true))
		f.Add([]byte(`{"pid": {}, "content": {"data": "\u001bAx\r\ny", "type": "rawV"}}`))
	}
	{ // C64 beyond PETSCII (c64x*.go)
		d := newDoc(40, 25)
		d.cells[3] = makeCell(65, 9, 0, 0)
		prg := c64xPrg(d, false, vicSetup{Mode: vicECM, Regs: [3]int{1, 2, 3}}, c64xFont(make([]byte, 2048), "f", "bin"))
		f.Add(prg)
		f.Add(writeD64("f.d64", prg))
		for _, n := range []string{"text-mc-per-char-notiles-ctm5.ctm", "text-hi-per-tile-ctm7.ctm", "text-hi-per-char-tiles-ctm82.ctm", "text-mc-per-char-tiles-ctm8.ctm"} {
			if b, err := os.ReadFile(filepath.Join("testdata", "charpad", n)); err == nil {
				f.Add(b)
			}
		}
	}
	{ // the Plus/4 (ted.go, ted_formats.go)
		d := tedFPage(1, 40, 25, false)
		f.Add(writeTEDPrg(d, true))
		f.Add(writeTEDSE(d))
		f.Add(writeSeqOn(d, false, true, true))
		f.Add(writeMarq(d, false, "plus4"))
		f.Add(writePetmateFrames(nil, []PetScreen{{doc: d, fb: -1, machine: "plus4"}}))
		f.Add([]byte("\x1cA\x82B\x16\xc6\x1bQ\x0d\x84\x9aF\x16"))
	}
	{ // Atari ST IGS (igs.go)
		f.Add([]byte(igsFSample))
		f.Add([]byte("G#&>0,636,4,0,>CL@,16,0)1,3:1)319,99,x,0:0C1,2:1L319,99,+2,0:&>20,60,20,0,W@2,0,x,a@b@c@:"))
		f.Add([]byte("G#X>7,1,\r\n" + strings.Repeat("XX--XX--XX--XX--@", 16) + ":A>4,1,1:Z>r,R,9999,9999:G>0,6,0,0,99,99,5,5:"))
		f.Add([]byte("G#X>11,1,3,\xff\x00\xff\x00G#T>2,1,41:T>2,1,50:J>1,1,500,9,0,3600:E>31,20,1:W>9,9,Hi@F>5,5:"))
	}
	{ // files that run on the machine: Atari .xex/.atr, Spectrum .tap (atari8.go, zxtap.go)
		d := newAtariDoc()
		d.cells[3] = makeCell(0x41|0x80, 0, 0, 0)
		f.Add(writeXex(d, nil))
		f.Add(writeXex(d, make([]byte, 1024)))
		f.Add(writeATR(d, nil)[:16+40*128])
		f.Add(writeTap(make([]byte, 6912), 3, "f"))
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
		check("uni ansi", func() []*Doc { d, _ := parseUniArt(b, 0); return one(d) })
		check("dur", func() []*Doc {
			if art, err := parseDur(b); err == nil {
				return art.docs
			}
			return nil
		})
		check("mirc", func() []*Doc { return one(parseMirc(b)) })
		check("pcb", func() []*Doc { return one(parsePCBoard(b, 80)) })
		check("avt", func() []*Doc { return one(parseAvatar(b, 80)) })
		for _, k := range bbsCodeIDs { // BBS colour codes (bbscodes.go)
			check("codes "+k, func() []*Doc { d, _, _ := parseArt(b, ParseOpts{codes: k}); return one(d) })
		}
		check("idf", func() []*Doc { return own(parseIDF(b)) })
		check("adf", func() []*Doc { return own(parseADF(b)) })
		check("tnd", func() []*Doc { return own(parseTundra(b)) })
		check("seq", func() []*Doc { d, _ := parseSeq(b, 40); return one(d) })
		check("seq image", func() []*Doc { d, _ := parseSeq(b, 40, "imageseq"); return one(d) }) // Image BBS's £ codes (c64bbs.go)
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
		check("icy", func() []*Doc { return own(parseIcy(b)) })
		check("pd", func() []*Doc { // PabloDraw, TheDraw, BSAVE (othformats.go)
			if p, err := parsePD(b); err == nil {
				return one(p.doc)
			}
			return nil
		})
		check("td", func() []*Doc { pages, _ := parseTD(b); return pages })
		check("skypix", func() []*Doc { isSkyPix(b); return one(readSkyPix(b).cells) }) // skypix.go
		check("bsv", func() []*Doc { d, _ := parseBSV(b); return one(d) })
		check("icy machine", func() []*Doc { // IcyDraw's pages of other machines (icymach.go)
			if doc, err := parseIcyDraw(b); err == nil {
				if m := icyMachinePage(doc, &gtiaPAL); m != nil {
					return one(m.doc)
				}
			}
			return nil
		})
		check("xp", func() []*Doc { return own(parseXP(b)) })
		check("psci", func() []*Doc {
			art, err := parsePsci(b)
			if err != nil {
				return nil
			}
			return art.docs
		})
		check("pet", func() []*Doc { d, _, _ := parseMarqPet(b); return one(d) })
		check("petmate json", func() []*Doc { return screens(parsePetmateJSON(b)) })
		check("petmate", func() []*Doc { return screens(parsePetmate(b)) })
		check("prg", func() []*Doc { d, _, _ := parsePrg(b); return one(d) })
		check("plus4 prg", func() []*Doc { d, _, _ := parseTEDPrg(b); return one(d) })
		check("tedse", func() []*Doc { d, _, _ := parseTEDSE(b, b); return one(d) })
		check("plus4 seq", func() []*Doc { // and what it shows is shown again by CharEdit's stream
			d, l := parseSeqOn(b, 40, true)
			if d.rows*d.cols <= 64000 {
				back, _ := parseSeqOn(writeSeqOn(d, l == 1, false, true), 40, true)
				if diff := tedFSame(d, back, true); diff != "" {
					t.Fatalf("plus4 seq written again: %s", diff)
				}
			}
			return one(d)
		})
		check("vt52", func() []*Doc { return one(parseVT52(b, stResolutions[0], 0)) })
		check("igs", func() []*Doc { // and what it writes back reads back
			l := readIGS(b, -1)
			d := l.cells.clone()
			d.set(0, 0, makeCell('Z', 1, 2, 0))
			if back := readIGS(writeIGS(l, d), l.res); !igsSameCells(back.cells, d) {
				t.Fatalf("igs: the cells do not read back")
			}
			return one(l.cells)
		})
		check("cpc bin", func() []*Doc { d, _, _, _ := parseCPCBin(b); return one(d) })
		check("cpc dsk", func() []*Doc {
			if f, _, err := dskArt(b); err == nil {
				d, _, _, _ := parseCPCBin(f.data)
				return one(d)
			}
			return nil
		})
		check("xex", func() []*Doc { d, _, _ := readAtariProgram(b); return one(d) })
		check("msx program", func() []*Doc { d, _, _, _ := readMSXProgram(b); return one(d) })
		check("msx text", func() []*Doc { return one(msxText(b, 2)) })
		check("mzf", func() []*Doc {
			if tp, ok := readMZF(b); ok {
				d, _ := readMZScreen(tp)
				return one(d)
			}
			return nil
		})
		check("mz text", func() []*Doc { return one(mzText(b)) })
		check("tap", func() []*Doc {
			if scr, _, err := readTap(b); err == nil {
				d, _ := parseSCR(scr)
				return one(d)
			}
			return nil
		})
		check("cpc stream", func() []*Doc { c := newCPCScreen(); c.print(b, false); return one(c.d) })
		check("vdt", func() []*Doc { return parseVDT(b) })
		if pages := parseVDT(b); len(pages) > 0 { // whatever a stream shows, CharEdit's stream shows it again
			p := pages[len(pages)-1]
			if back := parseVDT(mtWrite(p)); len(back) != 1 || mtFDiff(mtFull(p), back[0]) != "" {
				t.Fatalf("vdt written again: %s", mtFDiff(mtFull(p), back[len(back)-1]))
			}
		}
		check("viewdata stream", func() []*Doc { return parseVDStream(b) })
		check("viewdata bin", func() []*Doc { return one(parseVDBin(b)) })
		check("telstar", func() []*Doc { d, _ := parseTelstar(b); return one(d) })
		check("c64 prg", func() []*Doc { d, _, _, _, _ := parseC64xPrg(b); return one(d) })
		check("d64", func() []*Doc {
			if f, _, err := d64Art(b); err == nil {
				d, _, _ := readC64Prg(f.data)
				return one(d)
			}
			return nil
		})
		check("ctm", func() []*Doc {
			if p, err := parseCTM(b); err == nil {
				d, _ := p.page()
				return one(d)
			}
			return nil
		})
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
