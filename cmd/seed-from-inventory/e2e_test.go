package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEndToEndMixedInventory exercises the full parse -> recognize -> group -> write
// pipeline against a small inventory mixing two recognized providers, a duplicate board
// within one provider, and one URL no known ATS pattern matches.
func TestEndToEndMixedInventory(t *testing.T) {
	inPath := filepath.Join(t.TempDir(), "inventory.csv")
	csv := "name,slug,url\n" +
		"Acme,acme,https://boards.greenhouse.io/acme\n" +
		"Acme (also listed),acme-dup,https://boards.greenhouse.io/acme\n" +
		"Beta,beta,https://jobs.lever.co/beta\n" +
		"VanityCo,vanity,https://careers.vanityco.example\n"
	if err := os.WriteFile(inPath, []byte(csv), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	outDir := filepath.Join(t.TempDir(), "seeds")

	var stdout bytes.Buffer
	if code := run(inPath, outDir, &stdout); code != 0 {
		t.Fatalf("run: want exit 0, got %d (stdout: %s)", code, stdout.String())
	}

	entries, err := os.ReadDir(outDir)
	if err != nil {
		t.Fatalf("read output dir: %v", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if len(names) != 2 {
		t.Fatalf("output dir contains %v, want exactly greenhouse.json and lever.json", names)
	}

	var greenhouse []seedItem
	readSeedFile(t, filepath.Join(outDir, "greenhouse.json"), &greenhouse)
	if len(greenhouse) != 1 || greenhouse[0] != (seedItem{Board: "acme", Company: "Acme"}) {
		t.Errorf("greenhouse.json = %+v, want exactly one deduplicated entry for acme/Acme", greenhouse)
	}

	var lever []seedItem
	readSeedFile(t, filepath.Join(outDir, "lever.json"), &lever)
	if len(lever) != 1 || lever[0] != (seedItem{Board: "beta", Company: "Beta"}) {
		t.Errorf("lever.json = %+v, want exactly one entry for beta/Beta", lever)
	}

	if !strings.Contains(stdout.String(), "1 rows unrecognized") {
		t.Errorf("stdout %q: want the vanity-domain row counted as unrecognized", stdout.String())
	}
}

func readSeedFile(t *testing.T, path string, out *[]seedItem) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if err := json.Unmarshal(data, out); err != nil {
		t.Fatalf("unmarshal %s: %v", path, err)
	}
}
