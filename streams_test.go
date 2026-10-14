package main

import (
	"testing"
)

// streamFrames is a page of three frames of its type, each with something drawn.
func streamFrames(t *testing.T, mode string) *App {
	withFonts(t, true)
	a := testApp(t, mode)
	a.hasC64 = true
	for i := 0; i < 3; i++ {
		if i > 0 {
			a.newFrame()
		}
		for j := 0; j <= i; j++ {
			a.doc.set(2+j*3, 1+i, a.cell('A'+i))
		}
	}
	return a
}

// Frames saved as a stream and cut again at the clears are the same frames.
func TestStreamFramesRoundTrip(t *testing.T) {
	for _, c := range []struct{ mode, fmtID string }{{"pc", "ansi"}, {"amiga", "ansi"}, {"c64", "seq"}, {"atari", "ata"}, {"st", "vt52"}} {
		a := streamFrames(t, c.mode)
		a.info.writeSauce = false
		out, err := a.encode(c.fmtID)
		if err != nil {
			t.Fatal(err)
		}
		docs, holds, _ := splitStream(out, a.mode, a.doc.cols, a.amiga(), a.res(), false, 1000)
		if len(docs) != 3 {
			t.Fatalf("%s: %d frames back", c.mode, len(docs))
		}
		for i, d := range docs {
			if ok, why := cellsMatch(c.fmtID, a.frameDoc(i), d); !ok {
				t.Fatalf("%s frame %d: %s", c.mode, i+1, why)
			}
			if holds[i] < 20 {
				t.Fatalf("%s: hold %d", c.mode, holds[i])
			}
		}
		// Opened, the file offers its three pictures; the split makes them frames.
		b := testApp(t, c.mode)
		b.hasC64 = true
		b.loadBytes(out, "anim."+map[string]string{"ansi": "ans", "seq": "seq", "ata": "ata", "vt52": "vt"}[c.fmtID], "", c.mode)
		if b.split.clears != 3 || b.nFrames() != 1 {
			t.Fatalf("%s: offers %d pictures, %d frames", c.mode, b.split.clears, b.nFrames())
		}
		b.splitFrames(false)
		if b.nFrames() != 3 || !b.dirty {
			t.Fatalf("%s: split gave %d frames", c.mode, b.nFrames())
		}
		b.doUndo()
		if b.nFrames() != 1 {
			t.Fatalf("%s: undo of the split", c.mode)
		}
	}
}

// A still picture that starts by clearing the screen or sending the cursor
// home is not an animation.
func TestStillANSINotSplit(t *testing.T) {
	for _, art := range []string{"\x1b[2J\x1b[1;1HHello\r\nworld\r\n", "\x1b[1;1H\x1b[2JHello\r\n\x1b[H", "\x1b[H\x1b[31mRed\x1b[0m\r\n"} {
		docs, _, homes := splitStream([]byte(art), "pc", 80, false, STRes{}, false, 14400)
		if len(docs) != 1 || homes >= 2 {
			t.Fatalf("%q: %d frames, %d homes", art, len(docs), homes)
		}
	}
	// Drawn over at the cursor home: offered, and cut there on request.
	art := []byte("one\x1b[Htwo\x1b[Hthree")
	if docs, _, homes := splitStream(art, "pc", 80, false, STRes{}, false, 14400); len(docs) != 1 || homes != 2 {
		t.Fatalf("homes %d", homes)
	}
	if docs, _, _ := splitStream(art, "pc", 80, false, STRes{}, true, 14400); len(docs) != 3 || cellCh(docs[1].get(0, 0)) != 't' {
		t.Fatalf("at home: %d frames", len(docs))
	}
}

// A frame holds as long as its bytes take at the speed.
func TestSplitHoldsFromBaud(t *testing.T) {
	art := []byte("\x1b[2Jaaaaaaaaaa\x1b[2Jbbbbbbbbbbbbbbbbbbbb")
	docs, holds, _ := splitStream(art, "pc", 80, false, STRes{}, false, 1000)
	if len(docs) != 2 {
		t.Fatalf("%d frames", len(docs))
	}
	// 1000 bps is 100 bytes a second: the first picture is the 17 bytes up to
	// the next clear's J (170 ms), the second the 21 from there (210 ms).
	if holds[0] != 170 || holds[1] != 210 {
		t.Fatalf("holds %v", holds)
	}
}

// DOS's end-of-file byte at the end (line ends after it) is not drawn in the last frame.
func TestSplitEOF(t *testing.T) {
	docs, _, _ := splitStream([]byte("\x1b[2Jone\x1b[2Jtwo\x1a\r\n\r\n"), "pc", 80, false, STRes{}, false, 14400)
	if len(docs) != 2 || cellCh(docs[1].get(3, 0)) == 0x1A {
		t.Fatalf("%d frames, the EOF byte drawn", len(docs))
	}
}
