package logodomain

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Version is the snapshot format version. The proxy refuses a version it does not
// implement rather than misread the fields.
const Version = 1

// NormalizationID names the rule Entries' keys were built with, so the proxy can check
// they match how it hashes names. The two implementations live in two repositories;
// drift would look like the map not covering a company.
const NormalizationID = "lower-collapse-ws"

// Snapshot is the published map. Entries is normalized company name to bare registrable
// domain.
type Snapshot struct {
	Version       int               `json:"version"`
	Normalization string            `json:"normalization"`
	GeneratedAt   time.Time         `json:"generated_at"`
	Entries       map[string]string `json:"entries"`
}

// NewSnapshot stamps entries with the contract the proxy validates against.
func NewSnapshot(entries map[string]string, now time.Time) Snapshot {
	return Snapshot{
		Version:       Version,
		Normalization: NormalizationID,
		GeneratedAt:   now.UTC(),
		Entries:       entries,
	}
}

// publishedMode is the chmod applied before the rename. os.CreateTemp makes a 0600
// file and the proxy runs as another user, so without this the map is unreadable and
// looks like the publisher never ran.
const publishedMode = 0o644

// WriteFile publishes the snapshot at path, atomically. The proxy reloads on mtime
// change and may read at any moment, so a plain os.WriteFile would expose a truncated
// map. A temp file in the SAME directory, then rename (a cross-filesystem rename is
// neither atomic nor permitted).
func WriteFile(path string, s Snapshot) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("logodomain: create temp: %w", err)
	}
	// Removes the temp file after any failure below. Its error is dropped: on success the
	// rename already consumed the name, so this is expected to fail.
	defer func() { _ = os.Remove(tmp.Name()) }()

	if err := json.NewEncoder(tmp).Encode(s); err != nil {
		// Close's error is dropped on failure paths: the write already failed and the
		// deferred Remove cleans up; a close error would hide the real cause.
		_ = tmp.Close()
		return fmt.Errorf("logodomain: encode: %w", err)
	}
	// Sync before the rename: a crash between them would publish a name pointing at
	// unwritten blocks.
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("logodomain: sync: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("logodomain: close: %w", err)
	}
	if err := os.Chmod(tmp.Name(), publishedMode); err != nil {
		return fmt.Errorf("logodomain: chmod: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("logodomain: rename: %w", err)
	}
	return nil
}
