package main

import "testing"

func TestPCFormats(t *testing.T) {
	d := newDoc(80, 3)
	d.set(0, 0, makeCell('A', 12, 1, 0))
	d.set(1, 0, makeCell(0xDB, 14, 4, 0))
	for x := 10; x < 30; x++ { // a run for Avatar's ^Y
		d.set(x, 1, makeCell(0xB1, 3, 0, 0))
	}
	d.set(79, 2, makeCell('Z', 7, 0, 0))
	for name, back := range map[string]*Doc{
		"pcb": parsePCBoard(writePCBoard(d, true), 80),
		"avt": parseAvatar(writeAvatar(d, true), 80),
	} {
		for y := 0; y < 3; y++ {
			for x := 0; x < 80; x++ {
				a, b := d.get(x, y), back.get(x, y)
				if a&255 == 32 && b&255 == 32 {
					continue
				}
				if a != b {
					t.Fatalf("%s %d,%d: %08x became %08x", name, x, y, a, b)
				}
			}
		}
	}
	if !isPCBoard([]byte("@X1FHi @X07there @X4Ex")) || isPCBoard([]byte("mail@example.com")) {
		t.Fatal("PCBoard sniffing")
	}
	// PC attribute bytes are BIOS order: 0x1E is yellow on blue.
	if v := attrCell('X', 0x1E); cellFg(v) != 11 || cellBg(v) != 4 || cellAttr(v) != 0x1E {
		t.Fatalf("attribute order: fg %d bg %d", cellFg(v), cellBg(v))
	}
	// iCEDraw: 2 columns, a run of three cells, then font and palette.
	idf := []byte{4, '1', '.', '4', 0, 0, 0, 0, 1, 0, 1, 0, 'A', 0x1F, 1, 0, 3, 0, 'B', 0x2E}
	idf = append(idf, make([]byte, 4096+48)...)
	di, info, err := parseIDF(idf)
	if err != nil || di.cols != 2 || cellCh(di.get(0, 0)) != 'A' || cellCh(di.get(1, 1)) != 'B' || cellBg(di.get(1, 1)) != 2 || info.font == nil {
		t.Fatalf("idf: %v", err)
	}
	// TundraDraw: fg red, bg blue for one character, then a plain one.
	tnd := append([]byte{24}, "TUNDRA24"...)
	tnd = append(tnd, 6, 'X', 0, 0xFF, 0, 0, 0, 0, 0, 0xFF, 'Y')
	dt, ti, err := parseTundra(tnd)
	if err != nil || cellCh(dt.get(0, 0)) != 'X' || ti.pal == nil || ti.pal[cellFg(dt.get(0, 0))]&0xFFFFFF != 0xFF0000 || cellCh(dt.get(1, 0)) != 'Y' {
		t.Fatalf("tundra: %v %x", err, dt.get(0, 0))
	}
}
