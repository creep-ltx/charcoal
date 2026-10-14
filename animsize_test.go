package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// measureFiles are real screens the sizes are measured on (when they are on this machine).
var measureFiles = []string{
	"/tmp/bigcheck/files/pc/roots02/us-roots.ans",
	"/tmp/bigcheck/files/pc/supa02/us-promo.ans",
	"/tmp/bigcheck/files/pc/roots02/kf-roots.ans",
	"/tmp/bigcheck/files/bbsstyle/130302_015..TXT",
	"/tmp/bigcheck/files/bbsstyle/0f742d_005..TXT",
	"testdata/alove/bs-alove.bin",
}

// TestMeasureEffectSizes prints each effect's saved size on real screens
// (CHARCOAL_MEASURE=1): the numbers behind the trims and the detail levels.
func TestMeasureEffectSizes(t *testing.T) {
	if os.Getenv("CHARCOAL_MEASURE") == "" {
		t.Skip("CHARCOAL_MEASURE=1 measures")
	}
	testApp(t, "pc")
	var sb strings.Builder
	pats := batchPatterns()
	fmt.Fprintf(&sb, "%-22s %7s", "file", "plain")
	for _, p := range pats {
		fmt.Fprintf(&sb, " %8s", patterns[p].id[:min(8, len(patterns[p].id))])
	}
	sb.WriteString("\n")
	for _, f := range measureFiles {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		withScratch(func() {
			a.loadBytes(b, filepath.Base(f), f, "auto")
			fm := a.pageStreamFormat()
			a.order = nil
			plain, _ := a.encodeFor(fm, "", nil)
			fmt.Fprintf(&sb, "%-22s %7d", filepath.Base(f), len(plain))
			for _, p := range pats {
				a.order = &drawOrder{Pattern: patterns[p].id, Seed: 7}
				out, _ := a.encodeFor(fm, "", nil)
				fmt.Fprintf(&sb, " %8d", len(out))
			}
			fmt.Fprintf(&sb, "   %dx%d %s\n", a.doc.cols, a.doc.rows, a.mode)
		})
	}
	t.Log("\n" + sb.String())
}

// byteKinds splits an ANSI stream's bytes into cursor moves, colours and characters.
func byteKinds(b []byte) (moves, sgr, chars int) {
	for i := 0; i < len(b); i++ {
		switch {
		case b[i] == 0x1B && i+1 < len(b) && b[i+1] == '[':
			j := i + 2
			for j < len(b) && (b[j] >= '0' && b[j] <= '9' || b[j] == ';') {
				j++
			}
			if j < len(b) && b[j] == 'm' {
				sgr += j - i + 1
			} else {
				moves += j - i + 1
			}
			i = j
		case b[i] == '\r' || b[i] == '\n' || b[i] == 8:
			moves++
		default:
			chars++
		}
	}
	return
}

func TestMeasureByteKinds(t *testing.T) {
	if os.Getenv("CHARCOAL_MEASURE") == "" {
		t.Skip("CHARCOAL_MEASURE=1 measures")
	}
	testApp(t, "pc")
	f := measureFiles[1]
	b, err := os.ReadFile(f)
	if err != nil {
		t.Skip(err)
	}
	withScratch(func() {
		a.loadBytes(b, filepath.Base(f), f, "auto")
		for _, p := range batchPatterns() {
			a.order = &drawOrder{Pattern: patterns[p].id, Seed: 7}
			out, _ := a.encodeFor("ansi", "", nil)
			m, s, c := byteKinds(out)
			t.Logf("%-12s %7d  moves %7d  sgr %7d  chars %7d", patterns[p].id, len(out), m, s, c)
		}
	})
}
