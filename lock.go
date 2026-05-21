package main

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
)

var errLocked = errors.New("déploiement déjà en cours")

// lockRepo takes a non-blocking exclusive lock for a repo. The returned file
// must be closed to release it. Returns errLocked if a deploy already holds it.
func lockRepo(repo string) (*os.File, error) {
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(logDir, repo+".lock"), os.O_CREATE|os.O_RDWR, 0o640)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, errLocked
	}
	return f, nil
}
