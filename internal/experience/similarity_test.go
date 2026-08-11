package experience

import (
	"testing"

	"github.com/google/uuid"
)

func TestRichness(t *testing.T) {
	tests := []struct {
		name                 string
		atom                 Atom
		wantContext, wantMet bool
	}{
		{
			name:        "claim only",
			atom:        Atom{Claim: "Built a plugin"},
			wantContext: true, wantMet: true,
		},
		{
			name:        "digit in claim is not thin on metrics",
			atom:        Atom{Claim: "Cut checkout latency by 40%"},
			wantContext: true, wantMet: false,
		},
		{
			name:        "metrics array covers numbers",
			atom:        Atom{Claim: "Cut latency", Metrics: []string{"20s -> 1s"}},
			wantContext: true, wantMet: false,
		},
		{
			name:        "context present",
			atom:        Atom{Claim: "Built a plugin", Context: "Chromium extension"},
			wantContext: false, wantMet: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotC, gotM := Richness(tt.atom)
			if gotC != tt.wantContext || gotM != tt.wantMet {
				t.Errorf("Richness = (%v, %v), want (%v, %v)", gotC, gotM, tt.wantContext, tt.wantMet)
			}
		})
	}
}

func TestSoftDuplicateClustersPluginPair(t *testing.T) {
	a := uuid.New()
	b := uuid.New()
	atoms := []Atom{
		{ID: a, Claim: "Built a Chromium plugin with custom audio transcription pipeline using faster-whisper models, with configurable profiles for live and batch processing"},
		{ID: b, Claim: "Built a Chromium plugin that transcribes audio using faster-whisper models with configurable profiles and VAD-based filtering to reduce hallucination artifacts"},
	}
	got := SoftDuplicateClusters(atoms)
	if len(got) != 1 || len(got[0]) != 2 {
		t.Fatalf("clusters = %v, want one cluster of two", got)
	}
}

func TestSoftDuplicateClustersDistinctNumbers(t *testing.T) {
	atoms := []Atom{
		{ID: uuid.New(), Claim: "Cut latency 20s to 1s"},
		{ID: uuid.New(), Claim: "Cut p99 30s to 1s"},
	}
	if got := SoftDuplicateClusters(atoms); len(got) != 0 {
		t.Fatalf("clusters = %v, want none — numbers differ", got)
	}
}

func TestSoftDuplicateClustersStopwordsOnly(t *testing.T) {
	atoms := []Atom{
		{ID: uuid.New(), Claim: "I was in the room with the team"},
		{ID: uuid.New(), Claim: "We were on the call for the team"},
	}
	if got := SoftDuplicateClusters(atoms); len(got) != 0 {
		t.Fatalf("clusters = %v, want none — stopword-only overlap", got)
	}
}

func TestSoftDuplicateClustersCrossEmployment(t *testing.T) {
	roleA, roleB := uuid.New(), uuid.New()
	claim := "Built a Chromium plugin with custom audio transcription pipeline using faster-whisper models"
	atoms := []Atom{
		{ID: uuid.New(), EmploymentID: &roleA, Claim: claim},
		{ID: uuid.New(), EmploymentID: &roleB, Claim: claim},
	}
	if got := SoftDuplicateClusters(atoms); len(got) != 0 {
		t.Fatalf("clusters = %v, want none — different employments", got)
	}
}
