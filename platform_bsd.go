//go:build !windows && !linux

package main

import (
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// processName is a running program's name, "" when it cannot be told (ps has
// it on macOS and the BSDs, which have no /proc).
func processName(pid int) string {
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "comm=").Output()
	if err != nil {
		return ""
	}
	return filepath.Base(strings.TrimSpace(string(out)))
}
