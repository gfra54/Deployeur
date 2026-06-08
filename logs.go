package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

// logs tails deploy logs. With a repo it follows that repo; without one it
// follows every repo at once (live multi-site view). By default it follows
// (tail -f); with last>0 it prints the last N lines and exits.
func logs(repo string, last int) error {
	if repo == "" {
		paths, _ := filepath.Glob(filepath.Join(logDir, "*.log"))
		if len(paths) == 0 {
			return fmt.Errorf("aucun log pour l'instant (%s)", logDir)
		}
		return tailLogs(last, paths...)
	}
	path := filepath.Join(logDir, repo+".log")
	if !exists(path) {
		return fmt.Errorf("aucun log pour %q (%s)", repo, path)
	}
	return tailLogs(last, path)
}

// tailLogs streams the given log files: the last N lines then exit (last>0), or
// the tail followed live (last==0). With several files, tail prefixes each
// block with a `==> file <==` header and keeps them updated in real time.
func tailLogs(last int, paths ...string) error {
	args := []string{"-n", "40", "-f"}
	if last > 0 {
		args = []string{"-n", strconv.Itoa(last)}
	}
	args = append(args, paths...)
	c := exec.Command("tail", args...)
	c.Stdout, c.Stderr = os.Stdout, os.Stderr
	return c.Run()
}
