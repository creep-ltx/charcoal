package main

import (
	"bytes"
	"crypto/sha1"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeTestArt saves a made-up picture as dir/name (ANSI on a PC page, or an .ata).
func writeTestArt(t *testing.T, dir, name, mode string, seed int64) {
	t.Helper()
	keep := a.Tab
	defer func() { a.Tab = keep }()
	a.Tab = a.makeTab(mode)
	r := rand.New(rand.NewSource(seed))
	cols, rows := 80, 12+r.Intn(10)
	if mode == "atari" {
		cols, rows = 40, 24
		a.doc = newAtariDoc()
	} else {
		a.doc = newDoc(cols, rows)
	}
	a.info.writeSauce = false
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			if r.Intn(10) < 4 {
				continue
			}
			switch mode {
			case "atari":
				a.doc.cells[y*cols+x] = makeCell(33+r.Intn(90), 0, 0, 0)
			default:
				ch := 33 + r.Intn(94)
				if ch == '@' {
					ch = '~' // a board's own character, to be escaped (PCBoard has no escape for @)
				}
				if r.Intn(4) == 0 {
					ch = 176 + r.Intn(48)
				}
				a.doc.cells[y*cols+x] = makeCell(ch, r.Intn(16), r.Intn(8), 0)
			}
		}
	}
	f := map[string]string{"pc": "ansi", "atari": "ata"}[mode]
	b, err := a.encode(f)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), b, 0o644); err != nil {
		t.Fatal(err)
	}
}

// artFolder is a folder of n made-up ANSI screens.
func artFolder(t *testing.T, n int) string {
	dir := t.TempDir()
	for i := 0; i < n; i++ {
		writeTestArt(t, dir, fmt.Sprintf("SCREEN%02d.ANS", i), "pc", int64(i+1))
	}
	return dir
}

// runBatch runs a spec to the end.
func runBatch(t *testing.T, s batchSpec) *batchRun {
	t.Helper()
	r := newBatchRun(s)
	if r.err != "" {
		t.Fatalf("run refused: %s", r.err)
	}
	for !r.done() {
		r.step()
	}
	if err := r.writeReport(); err != nil {
		t.Fatal(err)
	}
	return r
}

// effectsOf lists each animated file's first effect.
func effectsOf(r *batchRun) []string {
	var out []string
	for _, it := range r.items {
		out = append(out, it.effects...)
	}
	return out
}

func folderHashes(t *testing.T, dir string) map[string][20]byte {
	t.Helper()
	out := map[string][20]byte{}
	filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			b, _ := os.ReadFile(p)
			out[p] = sha1.Sum(b)
		}
		return nil
	})
	return out
}

func TestBatchRandomRepeatable(t *testing.T) {
	testApp(t, "pc")
	src := artFolder(t, 12)
	run := func(seed int64, out string) (*batchRun, map[string][20]byte) {
		r := runBatch(t, batchSpec{Inputs: []string{src}, Effect: "random", NoRepeat: true, Toggles: true, Seed: seed, Board: "none", Out: out})
		return r, folderHashes(t, out)
	}
	r1, h1 := run(5, filepath.Join(t.TempDir(), "a"))
	r2, h2 := run(5, filepath.Join(t.TempDir(), "b"))
	e1, e2 := effectsOf(r1), effectsOf(r2)
	if strings.Join(e1, "|") != strings.Join(e2, "|") {
		t.Fatalf("the same seed picked other effects:\n%v\n%v", e1, e2)
	}
	if len(h1) != len(h2) {
		t.Fatalf("%d vs %d files", len(h1), len(h2))
	}
	for p, h := range h1 {
		q := filepath.Join(r2.outRoot, strings.TrimPrefix(p, r1.outRoot))
		if filepath.Base(p) != "animate-report.txt" && h2[q] != h {
			t.Errorf("%s differs between two runs with one seed", filepath.Base(p))
		}
	}
	r3, _ := run(6, filepath.Join(t.TempDir(), "c"))
	if strings.Join(effectsOf(r3), "|") == strings.Join(e1, "|") {
		t.Error("another seed picked the same effects")
	}
}

func TestBatchNoRepeatsAndTickedList(t *testing.T) {
	testApp(t, "pc")
	src := artFolder(t, 30)
	r := runBatch(t, batchSpec{Inputs: []string{src}, Effect: "random", NoRepeat: true, Seed: 9, Board: "none", Pool: []string{"top", "clock"},
		Variants: 2, Out: filepath.Join(t.TempDir(), "o")})
	last := ""
	seen := map[string]bool{}
	for _, e := range effectsOf(r) {
		if e != "Top" && e != "Clock" {
			t.Fatalf("an effect not ticked: %q", e)
		}
		if e == last {
			t.Fatalf("%s twice in a row", e)
		}
		last = e
		seen[e] = true
	}
	if len(seen) != 2 {
		t.Fatalf("effects used: %v", seen)
	}
	// With repeats allowed a pool of one is fine, and a single effect is every file's.
	r = runBatch(t, batchSpec{Inputs: []string{src}, Effect: "random", NoRepeat: true, Seed: 9, Board: "none", Pool: []string{"diamond"},
		Out: filepath.Join(t.TempDir(), "p")})
	for _, e := range effectsOf(r) {
		if e != "Diamond" {
			t.Fatalf("pool of one gave %q", e)
		}
	}
	if r := newBatchRun(batchSpec{Inputs: []string{src}, Effect: "random", Pool: []string{"scroll"}}); r.err == "" {
		t.Fatal("a frames-only effect in the pool was taken")
	}
}

func TestBatchVariantNames(t *testing.T) {
	s := &batchSpec{AEPath: "BBS:Node0/Screens"}
	for _, c := range []struct {
		board, rel string
		n          int
		want       string
		stub       string
	}{
		{"amiexpress", "LOGON.TXT", 3, "001.LOGON.TXT 002.LOGON.TXT 003.LOGON.TXT", "~3SR_BBS:Node0/Screens/LOGON|\r\n"},
		{"amiexpress", "conf1/BULL.TXT", 2, "conf1/001.BULL.TXT conf1/002.BULL.TXT", "~2SR_BBS:Node0/Screens/conf1/BULL|\r\n"},
		{"amiexpress", "LOGON.TXT", 1, "LOGON.TXT", ""},
		{"mystic", "logon.ans", 3, "logon.ans logon.an1 logon.an2", ""},
		{"mystic", "LOGON.ANS", 12, "LOGON.ANS LOGON.an1 LOGON.an2 LOGON.an3 LOGON.an4 LOGON.an5 LOGON.an6 LOGON.an7 LOGON.an8", ""},
		{"synchronet", "logon.ans", 3, "logon.ans logon1.ans logon2.ans", ""},
		{"pcboard", "LOGON.ANS", 3, "LOGON.ANS LOGON-2.ANS LOGON-3.ANS", ""},
		{"", "LOGON.ANS", 2, "LOGON.ANS LOGON-2.ANS", ""},
	} {
		names, stub := s.variantNames(c.board, filepath.FromSlash(c.rel), c.n)
		if got := filepath.ToSlash(strings.Join(names, " ")); got != c.want {
			t.Errorf("%s %s x%d: %s, want %s", c.board, c.rel, c.n, got, c.want)
		}
		if string(stub) != c.stub {
			t.Errorf("%s %s: stub %q, want %q", c.board, c.rel, stub, c.stub)
		}
	}
	if !strings.Contains(variantNote("pcboard"), "no random") || !strings.Contains(variantNote("tempest"), "not known") {
		t.Error("the boards without random screens are not said so")
	}
}

// Every file written, through its board's model, ends exactly on the original picture.
func TestBatchOutputReadsBack(t *testing.T) {
	testApp(t, "pc")
	src := artFolder(t, 6)
	alove := filepath.Join("testdata", "alove")
	for _, n := range []string{"bs-alove.bin", "bs-alove.xb", "bs-alove.pcb"} {
		b, err := os.ReadFile(filepath.Join(alove, n))
		if err != nil {
			t.Fatal(err)
		}
		os.WriteFile(filepath.Join(src, n), b, 0o644)
	}
	for _, board := range []string{"none", "amiexpress", "mystic", "pcboard", "synchronet", "tempest"} {
		out := filepath.Join(t.TempDir(), board)
		r := runBatch(t, batchSpec{Inputs: []string{src}, Effect: "random", NoRepeat: true, Toggles: true, Seed: 3, Board: board, Variants: 2,
			Copies: []string{"synchronet"}, Out: out, AEPath: "BBS:S/"})
		done, skipped, failed := r.counts()
		if done != 9 || skipped != 0 || failed != 0 {
			t.Fatalf("%s: %d done, %d skipped, %d failed\n%s", board, done, skipped, failed, r.reportText())
		}
		for i, it := range r.items {
			var want *Doc
			amiga := false
			withScratch(func() {
				p := filepath.Join(src, it.rel)
				b, _ := os.ReadFile(p)
				a.loadBytes(b, filepath.Base(p), p, "auto")
				want, amiga = a.doc.clone(), a.amiga()
			})
			for j, o := range it.outs {
				data, err := os.ReadFile(filepath.Join(out, o))
				if err != nil {
					t.Fatal(err)
				}
				id := board
				if strings.Contains(o, "-synchronet") {
					id = "synchronet"
				}
				if bytes.Contains(data, []byte("SR_")) && id == "amiexpress" && len(data) < 100 {
					continue // the stub that picks one
				}
				stream := data
				if m := boardModels[id]; m != nil {
					if stream, _, err = m(data); err != nil {
						t.Fatalf("%s %s: %v", board, o, err)
					}
				}
				got, _, _ := parseArt(stream, ParseOpts{amiga: amiga, forceCols: want.cols})
				if ok, why := cellsMatch("ansi", want, got); !ok {
					t.Fatalf("%s %s (%s, item %d out %d): %s", board, o, it.effects[j], i, j, why)
				}
			}
		}
	}
}

func TestBatchSkipReasons(t *testing.T) {
	testApp(t, "pc")
	src := t.TempDir()
	writeTestArt(t, src, "MENU.ANS", "pc", 1)
	writeTestArt(t, src, "pic.ata", "atari", 2)
	os.WriteFile(filepath.Join(src, "files.zip"), []byte("PK\x03\x04rest"), 0o644)
	os.WriteFile(filepath.Join(src, "empty.ans"), nil, 0o644)
	os.WriteFile(filepath.Join(src, "README"), []byte("just words\n"), 0o644)
	os.WriteFile(filepath.Join(src, "page.scr"), make([]byte, 6912), 0o644)
	os.WriteFile(filepath.Join(src, "tele.tti"), []byte("DE,x\r\nPN,10000\r\nOL,1,Hello teletext\r\n"), 0o644)
	os.WriteFile(filepath.Join(src, "old.ANS.bak"), []byte("\x1b[31mx"), 0o644)
	os.WriteFile(filepath.Join(src, "pet.seq"), []byte("\x93HELLO"), 0o644)
	os.WriteFile(filepath.Join(src, "blank.ans"), []byte("     \r\n"), 0o644)
	os.WriteFile(filepath.Join(src, "anim.ans"), []byte("\x1b[2JONE\x1b[2JTWO\x1b[2JTHREE"), 0o644)
	os.WriteFile(filepath.Join(src, "Disk.info"), []byte{0xE3, 0x10, 0, 1}, 0o644)
	r := runBatch(t, batchSpec{Inputs: []string{src}, Effect: "clock", Board: "amiexpress", Seed: 1, Out: filepath.Join(t.TempDir(), "o")})
	why := map[string]string{}
	for _, it := range r.items {
		why[it.rel] = it.skipped
	}
	for name, want := range map[string]string{
		"files.zip":   "ZIP archive",
		"empty.ans":   "empty",
		"README":      "not an art file",
		"page.scr":    "ZX Spectrum",
		"tele.tti":    "teletext",
		"old.ANS.bak": "backup",
		"pet.seq":     "PETSCII",
		"pic.ata":     "has no pause codes for .ata",
		"blank.ans":   "nothing drawn",
		"anim.ans":    "already an animation",
		"Disk.info":   "Amiga icon",
		"MENU.ANS":    "",
	} {
		if got := why[name]; !strings.Contains(got, want) || want == "" && got != "" {
			t.Errorf("%s: %q, want %q", name, got, want)
		}
	}
	rep := r.reportText()
	for _, s := range []string{"Skipped (11)", "files.zip: a ZIP archive", "MENU.ANS", "Clock"} {
		if !strings.Contains(rep, s) {
			t.Errorf("the report lacks %q:\n%s", s, rep)
		}
	}
	if b, err := os.ReadFile(filepath.Join(r.outRoot, "animate-report.txt")); err != nil || string(b) != rep {
		t.Errorf("the report file: %v", err)
	}
	// With no board the Atari file is animated (at the modem's pace) and reads back.
	r = runBatch(t, batchSpec{Inputs: []string{filepath.Join(src, "pic.ata")}, Effect: "random", Seed: 1, Board: "none", Out: filepath.Join(t.TempDir(), "o")})
	if len(r.items) != 1 || r.items[0].skipped != "" {
		t.Fatalf("the .ata with no board: %+v", r.items)
	}
	orig, _ := os.ReadFile(filepath.Join(src, "pic.ata"))
	got, _ := os.ReadFile(filepath.Join(r.outRoot, "pic.ata"))
	if ok, why := cellsMatch("ata", parseAta(orig, 0), parseAta(got, 0)); !ok || bytes.Equal(orig, got) {
		t.Fatalf("the .ata did not come back animated: %v %s", ok, why)
	}
}

func TestBatchOriginalsUntouched(t *testing.T) {
	testApp(t, "pc")
	src := artFolder(t, 4)
	os.Mkdir(filepath.Join(src, "sub"), 0o755)
	writeTestArt(t, filepath.Join(src, "sub"), "BULL.ANS", "pc", 77)
	before := folderHashes(t, src)
	// The output may not be the folder itself.
	if r := newBatchRun(batchSpec{Inputs: []string{src}, Effect: "top", Out: src}); !strings.Contains(r.err, "replace the originals") {
		t.Fatalf("writing into the source folder was not refused: %q", r.err)
	}
	// The default output is beside it; subfolders keep their place; an output inside is not read again.
	out := filepath.Join(src, "animated")
	r := runBatch(t, batchSpec{Inputs: []string{src}, Subfolders: true, Effect: "top", Board: "none", Seed: 1, Out: out})
	if done, _, _ := r.counts(); done != 5 {
		t.Fatalf("%d animated:\n%s", done, r.reportText())
	}
	if _, err := os.Stat(filepath.Join(out, "sub", "BULL.ANS")); err != nil {
		t.Fatal("the subfolder's file is not in the output's subfolder")
	}
	after := folderHashes(t, src)
	for p, h := range before {
		if after[p] != h {
			t.Errorf("%s changed", p)
		}
	}
	r = runBatch(t, batchSpec{Inputs: []string{src}, Effect: "top", Board: "none", Seed: 1})
	if r.outRoot != src+"-animated" {
		t.Errorf("default output %s", r.outRoot)
	}
	// Replace: the file is animated in place and the original kept as .bak.
	p := filepath.Join(src, "SCREEN00.ANS")
	orig, _ := os.ReadFile(p)
	runBatch(t, batchSpec{Inputs: []string{src}, Effect: "top", Board: "none", Seed: 1, Replace: true})
	now, _ := os.ReadFile(p)
	bak, _ := os.ReadFile(p + ".bak")
	if bytes.Equal(now, orig) || !bytes.Equal(bak, orig) {
		t.Fatal("replace: the file was not animated or the .bak is not the original")
	}
}

func TestBatchCommandLine(t *testing.T) {
	testApp(t, "pc")
	src := artFolder(t, 3)
	devnull, _ := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	defer devnull.Close()
	stdout, _ := os.CreateTemp(t.TempDir(), "out")
	defer stdout.Close()
	out := filepath.Join(t.TempDir(), "o")
	if c := animateCLI([]string{src, "--effect", "Wiggle out", "--bbs", "mystic", "--variants", "2", "--seed", "4", "--out", out}, stdout, devnull); c != 0 {
		t.Fatalf("a good run: exit %d", c)
	}
	b, _ := os.ReadFile(stdout.Name())
	if !strings.Contains(string(b), "3 animated") || !strings.Contains(string(b), "SCREEN00.an1") {
		t.Fatalf("the printed report:\n%s", b)
	}
	if _, err := os.Stat(filepath.Join(out, "SCREEN02.an1")); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{}, {src, "--effect", "nosuch"}, {src, "--bbs", "wildcat"}, {src, "--variants"}, {src, "--frobnicate"},
		{src, "--out", src}, {src, "--replace", "--out", out}, {src, "--effect", "scroll"},
	} {
		if c := animateCLI(args, devnull, devnull); c != 2 {
			t.Errorf("%v: exit %d, want 2", args, c)
		}
	}
	junk := t.TempDir()
	os.WriteFile(filepath.Join(junk, "a.zip"), []byte("PK\x03\x04"), 0o644)
	if c := animateCLI([]string{junk, "--out", filepath.Join(t.TempDir(), "j")}, devnull, devnull); c != 1 {
		t.Errorf("nothing to animate: exit %d, want 1", c)
	}
	if c := animateCLI([]string{"--help"}, devnull, devnull); c != 0 {
		t.Errorf("--help: exit %d", c)
	}
}

// A realistic run over real art packs (when the big check's files are on this machine).
func TestBatchRealArt(t *testing.T) {
	packs, _ := filepath.Glob("/tmp/bigcheck/files/pc/*")
	if len(packs) == 0 {
		t.Skip("no /tmp/bigcheck files")
	}
	testApp(t, "pc")
	src := t.TempDir()
	n := 0
	for _, pk := range packs {
		files, _ := filepath.Glob(filepath.Join(pk, "*"))
		for _, f := range files {
			if n >= 14 {
				break
			}
			if st, err := os.Stat(f); err != nil || st.IsDir() || st.Size() > 60000 {
				continue
			}
			b, _ := os.ReadFile(f)
			if os.WriteFile(filepath.Join(src, filepath.Base(f)), b, 0o644) == nil {
				n++
			}
		}
	}
	r := runBatch(t, batchSpec{Inputs: []string{src}, Effect: "random", NoRepeat: true, Toggles: true, Seed: 1, Board: "amiexpress", Variants: 2,
		Out: filepath.Join(t.TempDir(), "o")})
	for _, it := range r.items {
		if it.skipped != "" {
			continue
		}
		var want *Doc
		amiga := false
		withScratch(func() {
			p := filepath.Join(src, it.rel)
			b, _ := os.ReadFile(p)
			a.loadBytes(b, filepath.Base(p), p, "auto")
			want, amiga = a.doc.clone(), a.amiga()
		})
		for j, o := range it.outs[:len(it.outs)-1] { // the last is the stub
			data, _ := os.ReadFile(filepath.Join(r.outRoot, o))
			stream, _, err := modelAE(data)
			if err != nil {
				t.Fatalf("%s: %v", o, err)
			}
			got, _, _ := parseArt(stream, ParseOpts{amiga: amiga, forceCols: want.cols})
			if ok, why := cellsMatch("ansi", want, got); !ok {
				t.Errorf("%s (%s): %s", o, it.effects[j], why)
			}
		}
	}
	t.Logf("%d files:\n%s", n, r.reportText())
}
