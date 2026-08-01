package main

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestCandidates(t *testing.T) {
	tests := []struct {
		name    string
		slug    string
		company string
		want    []string
	}{
		{
			name: "single word yields one candidate",
			slug: "derq", company: "Derq",
			want: []string{"derq"},
		},
		{
			name: "multi-word name yields both renderings",
			slug: "much-better-adventures", company: "Much Better Adventures",
			want: []string{"much-better-adventures", "muchbetteradventures"},
		},
		{
			name: "legal form is stripped from the name-derived candidates",
			slug: "arch-capital-group-ltd", company: "Arch Capital Group Ltd.",
			want: []string{"arch-capital-group-ltd", "arch-capital-group", "archcapitalgroup"},
		},
		{
			name: "aggregator slug carrying a domain suffix keeps its own form too",
			slug: "derq-com", company: "Derq",
			want: []string{"derq-com", "derq"},
		},
		{
			name: "a candidate is never repeated",
			slug: "flosum", company: "flosum",
			want: []string{"flosum"},
		},
		{
			name: "a name that folds to nothing leaves only the catalogue slug",
			slug: "mystery-corp", company: "???",
			want: []string{"mystery-corp"},
		},
		{
			name: "too-short candidates are dropped",
			slug: "hp", company: "HP",
			want: nil,
		},
		{
			name: "non-latin name is transliterated",
			slug: "iandeks", company: "Яндекс",
			want: []string{"iandeks"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := candidates(tt.slug, tt.company); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("candidates(%q, %q) = %v, want %v", tt.slug, tt.company, got, tt.want)
			}
		})
	}
}

// The seed must parse as the {board, company} shape cmd/harvest-boards reads, with the
// expected employer on every entry — it is what the corroboration gate tests, and an entry
// without one is silently ungated.
func TestSeedEntriesShape(t *testing.T) {
	got := seedEntries([]orphan{
		{Slug: "derq", Company: "Derq"},
		{Slug: "arch-capital-group-ltd", Company: "Arch Capital Group Ltd."},
	})

	blob, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back []struct {
		Board   string `json:"board"`
		Company string `json:"company"`
	}
	if err := json.Unmarshal(blob, &back); err != nil {
		t.Fatalf("seed does not parse as {board, company}: %v", err)
	}
	if len(back) == 0 {
		t.Fatal("seed is empty")
	}
	seen := map[string]bool{}
	for _, e := range back {
		if e.Company == "" {
			t.Errorf("entry %q carries no expected employer", e.Board)
		}
		if seen[e.Board] {
			t.Errorf("board %q emitted twice", e.Board)
		}
		seen[e.Board] = true
	}
	if !seen["derq"] || !seen["archcapitalgroup"] {
		t.Errorf("expected both companies' candidates, got %v", seen)
	}
}

// Two companies can propose the same candidate board (a shared short name). Emitting it
// twice would probe it twice and, worse, let whichever entry sorted last decide which
// employer the gate compares against — so a contested candidate is dropped rather than
// silently attributed to one of them.
func TestSeedEntriesDropsContestedCandidates(t *testing.T) {
	got := seedEntries([]orphan{
		{Slug: "acme-inc", Company: "Acme Inc"},
		{Slug: "acme-llc", Company: "Acme LLC"},
	})
	for _, e := range got {
		if e.Board == "acme" {
			t.Errorf("candidate %q is claimed by two employers and must be dropped", e.Board)
		}
	}
	// The candidates that are not contested still survive.
	var boards []string
	for _, e := range got {
		boards = append(boards, e.Board)
	}
	if len(boards) != 2 {
		t.Errorf("uncontested candidates should remain, got %v", boards)
	}
}
