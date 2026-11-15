// Package modroot locates the module root by walking up from the working directory to the
// directory holding go.mod.
//
// Used by tests that police a rule across the whole repository. A counted "../" depth is
// silent when wrong: the walk just covers less of the repo and the guard keeps passing.
package modroot

import (
	"errors"
	"os"
	"path/filepath"
)

// ErrNotFound is returned when no go.mod exists at or above the working directory.
var ErrNotFound = errors.New("modroot: no go.mod at or above the working directory")

// Find returns the directory holding go.mod.
func Find() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", ErrNotFound
		}
		dir = parent
	}
}
