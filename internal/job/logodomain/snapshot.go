package logodomain

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Version is the snapshot format version. The proxy refuses a version it does not
// implement rather than guessing at the fields, because a map it misreads is a map that
// silently matches nothing.
const Version = 1

// NormalizationID names the rule Entries' keys were built with, so the proxy can check
// that the keys it looks up are keyed the way it hashes names. The two implementations
// live in two repositories and would otherwise drift in silence — and the symptom of
// drift is indistinguishable from the map simply not covering a company.
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

// publishedMode is what the map is chmod'ed to before the rename. os.CreateTemp makes a
// file 0600, and the proxy runs as a different user: without this the rename publishes a
// map only the writer can read, which on the host looks exactly like the publisher never
// having run.
const publishedMode = 0o644

// WriteFile publishes the snapshot at path, atomically.
//
// The proxy reloads whenever the file's mtime moves, so it can read at any moment: a
// plain os.WriteFile would hand it a truncated map and the JSON decode would fail for as
// long as the write lasts. Writing a temp file in the SAME directory and renaming makes
// the swap a single atomic operation — same directory because a rename across filesystems
// is neither atomic nor permitted.
func WriteFile(path string, s Snapshot) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("logodomain: create temp: %w", err)
	}
	// What stops a failure anywhere below leaving a temp file next to the map. Its error
	// is deliberately dropped: on the success path the rename has already consumed the
	// name and this is EXPECTED to fail, so reporting it would mean reporting every
	// successful write.
	defer func() { _ = os.Remove(tmp.Name()) }()

	if err := json.NewEncoder(tmp).Encode(s); err != nil {
		// Close's error is dropped on every failure path below: the write has already
		// failed, the deferred Remove is what actually cleans up, and replacing the real
		// cause with a close error would lose the only useful half of the report.
		_ = tmp.Close()
		return fmt.Errorf("logodomain: encode: %w", err)
	}
	// Sync before the rename: the rename is atomic with respect to readers, but a crash
	// between them would publish a name pointing at unwritten blocks.
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
