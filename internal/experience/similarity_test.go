package experience

import (
	"testing"

	"github.com/google/uuid"
)

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
