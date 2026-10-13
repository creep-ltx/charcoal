//go:build !windows

package main

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"syscall"
)

func beforeSDL() {}

// firstScale is the window scale before the person has picked one (a Mac
// doubles by itself on a Retina screen).
func firstScale() int { return 1 }

// running tells whether the CharEdit with this process id still runs: then its
// autosaves are its own business. The number may have gone to another program
// since, so its name must match ours as well.
func running(pid int) bool {
	if pid <= 0 || pid == os.Getpid() {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil || p.Signal(syscall.Signal(0)) != nil {
		return false
	}
	other, self := processName(pid), processName(os.Getpid())
	if other == "" || self == "" {
		return true
	}
	return sameProgram(other, self)
}

// fileRoots are the tops to start from in the file requester: / and the
// mounted disks (macOS /Volumes; Linux /media, /run/media and /mnt).
func fileRoots() []string {
	out := []string{"/"}
	var parents []string
	if runtime.GOOS == "darwin" {
		parents = []string{"/Volumes"}
	} else {
		u := os.Getenv("USER")
		parents = []string{"/media", "/run/media/" + u, "/mnt"}
		if u != "" {
			parents = append(parents, "/media/"+u)
		}
	}
	for _, par := range parents {
		ents, err := os.ReadDir(par)
		if err != nil {
			continue
		}
		var found []string
		for _, e := range ents {
			p := filepath.Join(par, e.Name())
			if st, err := os.Stat(p); err == nil && st.IsDir() && e.Name()[0] != '.' && p != "/media/"+os.Getenv("USER") {
				found = append(found, p)
			}
		}
		sort.Strings(found)
		out = append(out, found...)
	}
	return out
}
