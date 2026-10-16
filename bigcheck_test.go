//go:build bigcheck

package main

// The big check: every file under BIGCHECK_DIR is opened, checked and saved
// and opened again. Run with
//
//	BIGCHECK_DIR=/tmp/bigcheck/files go test -tags bigcheck -run BigCheck -timeout 30m -v
//
// It writes report.txt and samples/ (PNGs for a contact sheet) next to the files.

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

type checkResult struct {
	file, group, ext string
	mode, want       string
	problem          string
	rows, cols       int
	frames           int
	split            int // pictures a stream file shows, cut at its clears
	took             time.Duration
}

func expectedMode(path string, b []byte) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".seq", ".petmate":
		return "c64"
	case ".c":
		if strings.Contains(string(b[:min(len(b), 400)]), "unsigned char") {
			return "c64"
		}
		return "" // C source, not art
	case ".xb", ".pcb", ".avt", ".idf", ".adf", ".tnd":
		return "pc"
	case ".tti", ".ep1":
		return "tt"
	}
	if strings.Contains(path, "/ata/") {
		return "atari"
	}
	amigaOnly := strings.Contains(path, "/asciiarena/") || strings.Contains(path, "/bbsstyle/")
	if s, _ := readSauce(b); s != nil {
		f := strings.ToLower(s.font)
		switch {
		case strings.HasPrefix(f, "amiga"):
			return "amiga"
		case strings.HasPrefix(f, "ibm"):
			return "pc"
		}
	}
	if amigaOnly { // asciiarena.se and the BBS styles are Amiga art
		return "amiga"
	}
	return "" // not known
}

func checkOne(path, group string, sample bool, sampleDir string) (res checkResult) {
	res.file, res.group, res.ext = path, group, strings.ToLower(filepath.Ext(path))
	start := time.Now()
	defer func() {
		res.took = time.Since(start)
		if r := recover(); r != nil {
			res.problem = fmt.Sprintf("PANIC: %v", r)
		}
	}()
	b, err := os.ReadFile(path)
	if err != nil {
		res.problem = "unreadable"
		return
	}
	res.want = expectedMode(path, b)
	a.tabs, a.Tab, a.message = nil, nil, ""
	a.focusTab(a.newTab("amiga"))
	a.loadBytes(b, filepath.Base(path), path, "auto")
	res.mode, res.rows, res.cols = a.mode, a.doc.usedRows(), a.doc.cols
	if strings.Contains(a.message, ", not art") {
		res.mode = "not art"
		return // an archive or a program: rightly not opened
	}
	if strings.HasPrefix(a.message, "Could not") {
		res.problem = "load: " + a.message
		return
	}
	if strings.HasPrefix(a.message, "Only C64 programs in Petmate") {
		res.mode = "refused"
		return // a C64 program of someone else's: rightly not opened
	}
	if res.want != "" && res.want != res.mode {
		res.problem = fmt.Sprintf("page type %s, should be %s", res.mode, res.want)
	}
	art := b
	if _, end := readSauce(b); end < len(b) {
		art = b[:end]
	}
	if res.rows == 0 && len(strings.TrimSpace(string(art))) > 16 && a.srcKind != "petmate" && a.srcKind != "marq" {
		res.problem = strings.TrimSpace(res.problem + " empty page")
	}
	// Save in the page's own format and open again.
	orig := a.doc.clone()
	var origFrames []*Doc
	for _, d := range a.allDocs() {
		origFrames = append(origFrames, d.clone())
	}
	res.frames = len(origFrames)
	fmtID, mode := a.saveFmt, a.mode
	a.gen = nextGen() // written by CharEdit's own writer, not the file as read
	out, err := a.encode(fmtID)
	if err != nil {
		res.problem = strings.TrimSpace(res.problem + " save: " + err.Error())
		return
	}
	lower, ice := a.lower(), a.ice
	a.tabs, a.Tab = nil, nil
	a.focusTab(a.newTab(mode))
	name := "again" + map[string]string{"tti": ".tti", "ep1": ".ep1", "tturl": ".url", "scr": ".scr", "vt52": ".vt", "pcb": ".pcb", "avt": ".avt", "ansi": ".ans", "text": ".asc", "bin": ".bin", "xbin": ".xb", "seq": ".seq",
		"ata": ".ata", "marq": ".c", "petmate": ".petmate", "prg": ".prg",
		// other editors' files: .adf and .xp are known by their name only, as in the app
		"idf": ".idf", "adf": ".adf", "tnd": ".tnd", "icy": ".icy", "xp": ".xp", "psci": ".psci", "pet": ".pet", "pmjson": ".json",
		"uniansi": ".ans", "mirc": ".irc", "dur": ".dur", "dirart": ".d64"}[fmtID]
	as := mode
	if fmtID == "xbin" || fmtID == "marq" || fmtID == "petmate" || fmtID == "prg" {
		as = "auto"
	}
	a.loadBytes(out, name, "", as)
	if fmtID == "bin" || fmtID == "xbin" || fmtID == "marq" || fmtID == "petmate" {
		// these keep their width and colours by themselves
	} else if a.doc.cols != orig.cols { // a stream gets its width back from the page, as when shown again
		a.forceCols = orig.cols
		a.reparse()
	}
	if fmtID != "prg" && a.doc.rows != orig.rows {
		res.problem = strings.TrimSpace(res.problem + fmt.Sprintf(" height %d became %d", orig.rows, a.doc.rows))
	}
	if ok, why := cellsMatch(fmtID, orig, a.doc); !ok {
		res.problem = strings.TrimSpace(res.problem + " round trip (" + fmtID + "): " + why)
	}
	if framesFormats[fmtID] && len(origFrames) > 1 {
		if a.nFrames() != len(origFrames) {
			res.problem = strings.TrimSpace(res.problem + fmt.Sprintf(" %d frames became %d", len(origFrames), a.nFrames()))
		} else {
			for i, d := range origFrames {
				if ok, why := cellsMatch(fmtID, d, a.frameDoc(i)); !ok {
					res.problem = strings.TrimSpace(res.problem + fmt.Sprintf(" frame %d round trip (%s): %s", i+1, fmtID, why))
					break
				}
			}
		}
	}
	if mode == "c64" && a.lower() != lower {
		res.problem = strings.TrimSpace(res.problem + " character set changed")
	}
	if ice != a.ice && fmtID == "ansi" && a.info.writeSauce {
		res.problem = strings.TrimSpace(res.problem + " iCE flag changed")
	}
	// A stream that shows several pictures: cut into frames, saved and cut again.
	if splitN := a.splitCheckOf(b, path, mode, &res); splitN > 0 {
		res.split = splitN
	}
	// A picture of it.
	a.focusTab(a.newTab(mode))
	a.loadBytes(b, filepath.Base(path), path, "auto")
	a.doc = orig
	png, err := a.encode("png")
	if err != nil || len(png) < 50 {
		res.problem = strings.TrimSpace(res.problem + " picture failed")
	} else if sample {
		os.WriteFile(filepath.Join(sampleDir, group+"__"+filepath.Base(path)+".png"), png, 0o644)
	}
	return
}

func TestBigCheck(t *testing.T) {
	root := os.Getenv("BIGCHECK_DIR")
	if root == "" {
		t.Skip("BIGCHECK_DIR not set")
	}
	os.Setenv("XDG_CONFIG_HOME", t.TempDir())
	a = &App{tool: "draw", fg: 7, brush: 0x2F, charHover: -1, layout: "one"}
	a.hasC64 = loadC64Fonts()
	loadPrefs()
	if !a.hasC64 {
		t.Fatal("the C64 character set is needed for the check")
	}
	sampleDir := filepath.Join(root, "..", "samples")
	os.RemoveAll(sampleDir)
	os.MkdirAll(sampleDir, 0o755)
	var files []string
	skipped := 0
	filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() {
			return nil
		}
		// Not art: links an archive made, macOS "._" files, web pages saved in place of files.
		if fi.Mode()&os.ModeSymlink != 0 || strings.HasPrefix(fi.Name(), "._") || fi.Size() == 0 || fi.Size() >= 4<<20 {
			skipped++
			return nil
		}
		if head, _ := os.ReadFile(p); len(head) > 0 {
			h := strings.ToLower(string(head[:min(len(head), 64)]))
			if strings.HasPrefix(h, "<!doctype html") || strings.HasPrefix(h, "<html") {
				skipped++
				return nil
			}
		}
		files = append(files, p)
		return nil
	})
	sort.Strings(files)
	rng := rand.New(rand.NewSource(1))
	var results []checkResult
	for _, f := range files {
		rel, _ := filepath.Rel(root, f)
		group := strings.SplitN(rel, string(filepath.Separator), 2)[0]
		if group == "c64" {
			group = "c64-" + filepath.Base(filepath.Dir(f))
		}
		results = append(results, checkOne(f, group, rng.Intn(25) == 0, sampleDir))
	}
	// The report.
	var sb strings.Builder
	type stat struct {
		n, bad, known, rightMode int
		modes                    map[string]int
		slow                     time.Duration
	}
	stats := map[string]*stat{}
	var problems []string
	for _, r := range results {
		s := stats[r.group]
		if s == nil {
			s = &stat{modes: map[string]int{}}
			stats[r.group] = s
		}
		s.n++
		s.modes[r.mode]++
		if r.want != "" {
			s.known++
			if r.want == r.mode {
				s.rightMode++
			}
		}
		if r.took > s.slow {
			s.slow = r.took
		}
		if r.problem != "" {
			s.bad++
			problems = append(problems, fmt.Sprintf("%-10s %s: %s", r.group, strings.TrimPrefix(r.file, root+"/"), r.problem))
		}
	}
	var groups []string
	for g := range stats {
		groups = append(groups, g)
	}
	sort.Strings(groups)
	fmt.Fprintf(&sb, "%-12s %6s %6s %12s %10s  %s\n", "group", "files", "issues", "type right", "slowest", "page types")
	for _, g := range groups {
		s := stats[g]
		fmt.Fprintf(&sb, "%-12s %6d %6d %5d of %4d %10s  %v\n", g, s.n, s.bad, s.rightMode, s.known, s.slow.Round(time.Millisecond), s.modes)
	}
	multi, frames, splits, splitFr := 0, 0, 0, 0
	for _, r := range results {
		if r.frames > 1 {
			multi++
			frames += r.frames
		}
		if r.split > 1 {
			splits++
			splitFr += r.split
		}
	}
	fmt.Fprintf(&sb, "\nfiles with several frames: %d (%d frames)\n", multi, frames)
	fmt.Fprintf(&sb, "stream files that split at their clears: %d (%d frames)\n", splits, splitFr)
	sb.WriteString("\n")
	for _, p := range problems {
		sb.WriteString(p + "\n")
	}
	os.WriteFile(filepath.Join(root, "..", "report.txt"), []byte(sb.String()), 0o644)
	t.Logf("%d files (%d skipped as not art), %d with issues; report in %s", len(results), skipped, len(problems), filepath.Join(root, "..", "report.txt"))
}

// splitCheckOf opens the file again, cuts it into frames at its clears, saves
// them and cuts that again: the same frames. It gives the frames, 0 if the file is no animation.
func (a *App) splitCheckOf(b []byte, path, mode string, res *checkResult) int {
	a.tabs, a.Tab = nil, nil
	a.focusTab(a.newTab(mode))
	a.loadBytes(b, filepath.Base(path), path, "auto")
	if a.split.clears < 2 {
		return 0
	}
	a.splitFrames(false)
	n := a.nFrames()
	fmtID := a.saveFmt
	if fmtID == "text" {
		fmtID = "ansi"
	}
	before := a.allDocs()
	out, err := a.encode(fmtID)
	if err != nil {
		res.problem = strings.TrimSpace(res.problem + " split save: " + err.Error())
		return n
	}
	codes := ""
	if isBBSCodes(fmtID) {
		codes = fmtID // saved in a board's colour codes: read back with them
	}
	docs, _, _ := splitStream(out, a.mode, a.doc.cols, a.amiga(), a.res(), false, a.baud, codes)
	if len(docs) != n {
		// Empty pictures in the file are no frames; written, they are the same count.
		res.problem = strings.TrimSpace(res.problem + fmt.Sprintf(" split: %d frames saved, %d read back", n, len(docs)))
		return n
	}
	for i := range docs {
		if ok, why := cellsMatch(fmtID, before[i], docs[i]); !ok {
			res.problem = strings.TrimSpace(res.problem + fmt.Sprintf(" split frame %d round trip: %s", i+1, why))
			break
		}
	}
	return n
}
