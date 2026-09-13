package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteSeedsWritesOneFilePerProvider(t *testing.T) {
	dir := t.TempDir()
	byProvider := map[string][]seedItem{
		"greenhouse": {{Board: "acme", Company: "Acme"}},
		"lever":      {{Board: "beta", Company: "Beta"}},
	}

	if err := writeSeeds(dir, byProvider); err != nil {
		t.Fatalf("writeSeeds: %v", err)
	}

	for provider, want := range byProvider {
		data, err := os.ReadFile(filepath.Join(dir, provider+".json"))
		if err != nil {
			t.Fatalf("read %s.json: %v", provider, err)
		}
		var got []seedItem
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("unmarshal %s.json: %v", provider, err)
		}
		if len(got) != len(want) || got[0] != want[0] {
			t.Errorf("%s.json = %+v, want %+v", provider, got, want)
		}
	}
}

// TestWriteSeedsMatchesHarvestBoardsSeedShape confirms the written JSON decodes into the
// exact object shape cmd/harvest-boards's own seed loader accepts (cmd/harvest-boards/seed.go
// unmarshals a "board"/"company" object array): a top-level array of objects carrying only
// those two lowercase keys, so the file can be passed to that tool unmodified.
func TestWriteSeedsMatchesHarvestBoardsSeedShape(t *testing.T) {
	dir := t.TempDir()
	byProvider := map[string][]seedItem{
		"workday": {{Board: "acme.wd1.myworkdayjobs.com/careers", Company: "Acme"}},
	}
	if err := writeSeeds(dir, byProvider); err != nil {
		t.Fatalf("writeSeeds: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "workday.json"))
	if err != nil {
		t.Fatalf("read workday.json: %v", err)
	}

	var harvestBoardsShape []struct {
		Board   string `json:"board"`
		Company string `json:"company"`
	}
	if err := json.Unmarshal(data, &harvestBoardsShape); err != nil {
		t.Fatalf("does not decode as cmd/harvest-boards's seed shape: %v", err)
	}
	if len(harvestBoardsShape) != 1 ||
		harvestBoardsShape[0].Board != "acme.wd1.myworkdayjobs.com/careers" ||
		harvestBoardsShape[0].Company != "Acme" {
		t.Errorf("decoded = %+v, want one entry for acme", harvestBoardsShape)
	}
}

func TestWriteSeedsCreatesOutputDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "seeds")

	err := writeSeeds(dir, map[string][]seedItem{
		"greenhouse": {{Board: "acme", Company: "Acme"}},
	})
	if err != nil {
		t.Fatalf("writeSeeds: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "greenhouse.json")); err != nil {
		t.Errorf("expected greenhouse.json in newly created directory: %v", err)
	}
}
