package logodomain

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewSnapshotDeclaresTheContract(t *testing.T) {
	s := NewSnapshot(map[string]string{"g2i": "g2i.co"}, time.Unix(0, 0).UTC())
	if s.Version != Version {
		t.Errorf("Version = %d, want %d", s.Version, Version)
	}
	if s.Normalization != NormalizationID {
		t.Errorf("Normalization = %q, want %q", s.Normalization, NormalizationID)
	}
	if s.Entries["g2i"] != "g2i.co" {
		t.Errorf("Entries = %v", s.Entries)
	}
}

func TestWriteFileRoundTrips(t *testing.T) {
	path := filepath.Join(t.TempDir(), "domains.json")
	want := NewSnapshot(map[string]string{"g2i": "g2i.co", "g2i inc.": "g2i.co"}, time.Unix(0, 0).UTC())
	if err := WriteFile(path, want); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var got Snapshot
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.Version != want.Version || got.Normalization != want.Normalization {
		t.Errorf("header = %d/%q, want %d/%q", got.Version, got.Normalization, want.Version, want.Normalization)
	}
	if len(got.Entries) != 2 || got.Entries["g2i inc."] != "g2i.co" {
		t.Errorf("Entries = %v", got.Entries)
	}
}

func TestWriteFileLeavesNoTempFileBehind(t *testing.T) {
	// The proxy reloads on mtime and must never read a half-written map, so the write
	// goes to a temp file and renames. A leftover temp file would also be picked up by
	// whatever mirrors this directory.
	dir := t.TempDir()
	path := filepath.Join(dir, "domains.json")
	if err := WriteFile(path, NewSnapshot(map[string]string{"g2i": "g2i.co"}, time.Now())); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "domains.json" {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("directory holds %v, want only domains.json", names)
	}
}

func TestWriteFileIsReadableByAnotherUser(t *testing.T) {
	// The proxy runs as its own user. os.CreateTemp makes the file 0600, so without an
	// explicit chmod the rename publishes a map only the writer can read — which looks
	// exactly like the publisher never having run.
	path := filepath.Join(t.TempDir(), "domains.json")
	if err := WriteFile(path, NewSnapshot(map[string]string{"g2i": "g2i.co"}, time.Now())); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode&0o044 == 0 {
		t.Errorf("mode = %v, want group- and world-readable", mode)
	}
}

func TestWriteFileReplacesAnExistingMap(t *testing.T) {
	path := filepath.Join(t.TempDir(), "domains.json")
	if err := WriteFile(path, NewSnapshot(map[string]string{"old": "old.com"}, time.Now())); err != nil {
		t.Fatalf("first WriteFile: %v", err)
	}
	if err := WriteFile(path, NewSnapshot(map[string]string{"new": "new.com"}, time.Now())); err != nil {
		t.Fatalf("second WriteFile: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var got Snapshot
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if _, stale := got.Entries["old"]; stale {
		t.Errorf("Entries still hold the previous map: %v", got.Entries)
	}
}
