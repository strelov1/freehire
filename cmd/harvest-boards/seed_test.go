package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestChooseCompany(t *testing.T) {
	// A real API name (different from the board id) always wins.
	if got := chooseCompany("Acme Inc", "Role Co", "acme"); got != "Acme Inc" {
		t.Errorf("api name should win, got %q", got)
	}
	// When the prober only echoed the board id back, a seed-provided name is preferred.
	if got := chooseCompany("acme", "Acme From Role", "acme"); got != "Acme From Role" {
		t.Errorf("seed name should fill, got %q", got)
	}
	// Empty prober name with a seed name uses the seed name.
	if got := chooseCompany("", "Acme From Role", "acme"); got != "Acme From Role" {
		t.Errorf("seed name should fill empty, got %q", got)
	}
	// No usable name anywhere falls back to the board id.
	if got := chooseCompany("acme", "", "acme"); got != "acme" {
		t.Errorf("should fall back to board, got %q", got)
	}
}

func TestLoadSeedItemsStringArray(t *testing.T) {
	path := writeTemp(t, `["a", "b", "c"]`)
	got, err := loadSeedItems(path)
	if err != nil {
		t.Fatalf("loadSeedItems: %v", err)
	}
	want := []seedItem{{Board: "a"}, {Board: "b"}, {Board: "c"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestLoadSeedItemsObjectArray(t *testing.T) {
	path := writeTemp(t, `[{"board":"x","company":"X Corp"},{"board":"y"}]`)
	got, err := loadSeedItems(path)
	if err != nil {
		t.Fatalf("loadSeedItems: %v", err)
	}
	want := []seedItem{{Board: "x", Company: "X Corp"}, {Board: "y"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "seed.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
