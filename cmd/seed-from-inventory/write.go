package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// writeSeeds writes one JSON seed file per provider into dir, named "<provider>.json", each
// holding that provider's []seedItem — the exact shape cmd/harvest-boards's own seed loader
// (cmd/harvest-boards/seed.go) accepts, so a file here can be passed to that tool unmodified.
func writeSeeds(dir string, byProvider map[string][]seedItem) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create output directory %s: %w", dir, err)
	}
	for provider, items := range byProvider {
		data, err := json.MarshalIndent(items, "", "  ")
		if err != nil {
			return fmt.Errorf("marshal seed for %s: %w", provider, err)
		}
		path := filepath.Join(dir, provider+".json")
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return fmt.Errorf("write seed for %s: %w", provider, err)
		}
	}
	return nil
}
