package main

import (
	"os"
	"strconv"
	"strings"
)

// processName is a running program's name, "" when it cannot be told.
func processName(pid int) string {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/comm")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
