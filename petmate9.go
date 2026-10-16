package main

// Petmate9's machines in .petmate (wbochar/petmate9, src/utils/presetExport.ts and
// vdcAttr.ts): a screen's charset names its machine: "upper"/"lower" the C64,
// "c128Upper"/"c128Lower" the C128 (80 columns wide: its VDC), "vic20Upper"/
// "vic20Lower", "petGfx"/"petBiz" (the PET's graphics and business sets; 80 wide: the
// 8032), "c16Upper"/"c16Lower" (ted.go). A VDC screen's colours are the VDC's RGBI
// numbers, and each pixel may carry "attr", the VDC's attribute byte (bit 7 the other
// set, 6 reverse, 5 underline, 4 blink, 0-3 the colour).

import "strings"

// petmate9Machine is the machine page a Petmate screen of this charset and width opens on
// ("" the C64, or the page's machine for a custom set).
func petmate9Machine(charset string, width int) string {
	switch cs := strings.ToLower(charset); {
	case strings.HasPrefix(cs, "c16"):
		return "plus4"
	case strings.HasPrefix(cs, "vic20"):
		return "vic20"
	case cs == "petgfx" || cs == "petbiz":
		if width >= 80 {
			return "pet80"
		}
		return "pet"
	case strings.HasPrefix(cs, "c128"):
		if width >= 80 {
			return "c128vdc"
		}
		return "c128"
	}
	return ""
}

// petmate9Charset is Petmate9's charset name for a machine's set.
func petmate9Charset(machine string, lower bool) string {
	switch machine {
	case "plus4":
		return map[bool]string{true: "c16Lower", false: "c16Upper"}[lower]
	case "vic20":
		return map[bool]string{true: "vic20Lower", false: "vic20Upper"}[lower]
	case "pet", "pet80":
		return map[bool]string{true: "petBiz", false: "petGfx"}[lower]
	case "c128", "c128vdc":
		return map[bool]string{true: "c128Lower", false: "c128Upper"}[lower]
	}
	return map[bool]string{true: "lower", false: "upper"}[lower]
}

// petmate9Cell is a VDC pixel as a cell of CharEdit's 80-column C128 page (C64 colour
// numbers; reverse as the reversed glyph).
func petmate9VDCCell(code, color int, attr *int) uint32 {
	if attr != nil {
		color = *attr & 15
		if *attr&0x40 != 0 {
			code ^= 0x80
		}
	}
	return makeCell(code&255, c64OfVDC(color), 0, 0)
}

// petmate9Known: a charset name of the machines' own sets (not a custom font's).
func petmate9Known(cs string) bool {
	switch strings.ToLower(cs) {
	case "upper", "lower", "c128upper", "c128lower", "c128vdc", "vic20upper", "vic20lower", "petgfx", "petbiz", "c16upper", "c16lower":
		return true
	}
	return false
}
