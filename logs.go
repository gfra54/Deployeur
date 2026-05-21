package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

// logs tails a repo's log. By default it follows (tail -f); with last>0 it
// prints the last N lines and exits.
func logs(repo string, last int) error {
	if repo == "" {
		return fmt.Errorf("usage: deployeur logs <repo> [--last [N]]")
	}
	path := filepath.Join(logDir, repo+".log")
	if !exists(path) {
		return fmt.Errorf("aucun log pour %q (%s)", repo, path)
	}
	var args []string
	if last > 0 {
		args = []string{"-n", strconv.Itoa(last), path}
	} else {
		args = []string{"-n", "40", "-f", path}
	}
	c := exec.Command("tail", args...)
	c.Stdout, c.Stderr = os.Stdout, os.Stderr
	return c.Run()
}
