package main

// Where the settings and the data live on each system:
//
//	Linux    ~/.config/charcoal (settings), ~/.local/share/charcoal (data), or
//	         under $XDG_CONFIG_HOME / $XDG_DATA_HOME
//	Windows  %AppData%\Charcoal, both
//	macOS    ~/Library/Application Support/Charcoal, both
//
// XDG_CONFIG_HOME and XDG_DATA_HOME are honoured on every system when they hold
// an absolute path (the tests point them at a scratch folder).

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const isMac = runtime.GOOS == "darwin"

// ownFolderSystem: Windows and macOS keep settings and data together in one
// folder named Charcoal under the system's place for them.
const ownFolderSystem = runtime.GOOS == "windows" || runtime.GOOS == "darwin"

// systemAppDir is %AppData% on Windows, ~/Library/Application Support on macOS.
func systemAppDir() string {
	d, err := os.UserConfigDir()
	if err != nil || !filepath.IsAbs(d) {
		return filepath.Join(os.TempDir(), "Charcoal")
	}
	return filepath.Join(d, "Charcoal")
}

// configDir is the folder for prefs.json and keys.
func configDir() string {
	if c := os.Getenv("XDG_CONFIG_HOME"); filepath.IsAbs(c) {
		return filepath.Join(c, "charcoal")
	}
	if ownFolderSystem {
		return systemAppDir()
	}
	home, err := os.UserHomeDir()
	if err != nil || !filepath.IsAbs(home) {
		return filepath.Join(os.TempDir(), "charcoal")
	}
	return filepath.Join(home, ".config", "charcoal")
}

// appDataDir is the folder for downloaded fonts and the autosaves.
func appDataDir() string {
	if d := os.Getenv("XDG_DATA_HOME"); filepath.IsAbs(d) || !ownFolderSystem {
		return filepath.Join(dataDir(), "charcoal")
	}
	return systemAppDir()
}

// shownDir is a folder as the help and the messages write it: the home folder as ~.
func shownDir(p string) string { return shortPath(p) + string(filepath.Separator) }

// hasPathEnd tells whether a typed path ends in a folder separator.
func hasPathEnd(p string) bool { return p != "" && os.IsPathSeparator(p[len(p)-1]) }

// withPathEnd adds a separator unless the folder already ends in one (/ or C:\).
func withPathEnd(dir string) string {
	if hasPathEnd(dir) {
		return dir
	}
	return dir + string(filepath.Separator)
}

// homeDir is the home folder, or "" when there is none.
func homeDir() string {
	h, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(h) == "" {
		return ""
	}
	return h
}
