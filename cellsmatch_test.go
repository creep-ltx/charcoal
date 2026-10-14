package main

import "fmt"

// cellsMatch compares two pages the way the saved format can keep them.
func cellsMatch(fmtID string, x, y *Doc) (bool, string) {
	rows := max(x.usedRows(), y.usedRows())
	if x.cols != y.cols && fmtID != "text" {
		return false, fmt.Sprintf("width %d became %d", x.cols, y.cols)
	}
	for r := 0; r < rows; r++ {
		for c := 0; c < x.cols; c++ {
			a, b := x.get(c, r), y.get(c, r)
			switch fmtID {
			case "text", "ata", "tti", "ep1", "tturl":
				a, b = a&255, b&255
			case "seq", "marq", "petmate", "prg":
				a, b = a&0xFFF, b&0xFFF
				if a&255 == 32 { // a space carries no colour
					a, b = a&255, b&255
				}
			case "ansi", "pcb", "avt", "vt52":
				if isBlank(a) && isBlank(b) {
					continue
				}
				if a&255 == 32 || a&255 == 0 { // a space's colour is only its background
					a, b = a&0xF000|32, b&0xF000|32
				}
			}
			if a != b {
				return false, fmt.Sprintf("cell %d,%d: %08x became %08x", c, r, a, b)
			}
		}
	}
	if x.bg != y.bg && fmtID != "ansi" && fmtID != "text" && fmtID != "seq" && fmtID != "ata" {
		return false, "background changed"
	}
	return true, ""
}
