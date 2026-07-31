package cvmatch_test

import (
	"encoding/json"
	"testing"

	"github.com/strelov1/freehire/internal/cvmatch"
)

// The four weights are the denominator of a score served out of 100. A re-weighting that
// forgets one of them would not fail any scoring test — every category would still divide
// by the sum — so the sum is asserted on its own.
func TestCategoryWeightsSumTo100(t *testing.T) {
	got := cvmatch.WeightRequirements + cvmatch.WeightKeyword + cvmatch.WeightTitle + cvmatch.WeightSeniority
	if got != 100 {
		t.Fatalf("category weights sum to %d, want 100", got)
	}
}

// The wire shape is what the SPA's Job Match panel renders and what cmd/gen-contracts
// generates from, so its keys are asserted rather than eyeballed. A category the scorer
// could not evaluate must still serialize its reason, and must not pretend to have items.
func TestScoreWireShape(t *testing.T) {
	s := cvmatch.Score{
		Overall: 75,
		Categories: []cvmatch.ScoredCategory{
			{
				ID:        cvmatch.CategoryKeyword,
				Label:     "Keyword Match",
				Earned:    20,
				Weight:    cvmatch.WeightKeyword,
				Available: true,
				Items: []cvmatch.LineItem{
					{Points: 20, Text: "4 of 6 role keywords present", Status: cvmatch.StatusPass},
				},
			},
			{
				ID:        cvmatch.CategorySeniority,
				Label:     "Seniority Fit",
				Weight:    cvmatch.WeightSeniority,
				Available: false,
				Reason:    "the vacancy's title states no seniority",
			},
		},
		Contributing:  []string{cvmatch.CategoryKeyword},
		MatchedSkills: []string{"go"},
		MissingSkills: []string{"kubernetes"},
		Requirements: []cvmatch.RequirementCheck{
			{Text: "5+ years of Go", Priority: "required", Coverage: cvmatch.Covered, Skills: []string{"go"}},
			{Text: "strong communication", Priority: "preferred", Coverage: cvmatch.Unverifiable, CachedStatus: "covered"},
		},
	}

	blob, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal score: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(blob, &got); err != nil {
		t.Fatalf("unmarshal score: %v", err)
	}

	for _, key := range []string{"overall", "categories", "contributing", "matched_skills", "missing_skills", "requirements"} {
		if _, ok := got[key]; !ok {
			t.Errorf("score JSON is missing key %q; got %s", key, blob)
		}
	}

	cats, ok := got["categories"].([]any)
	if !ok || len(cats) != 2 {
		t.Fatalf("categories did not survive marshalling: %s", blob)
	}
	first, _ := cats[0].(map[string]any)
	for _, key := range []string{"id", "label", "earned", "weight", "available", "items"} {
		if _, ok := first[key]; !ok {
			t.Errorf("category JSON is missing key %q; got %s", key, blob)
		}
	}
	if _, ok := first["reason"]; ok {
		t.Errorf("an available category serialized a reason; got %s", blob)
	}

	second, _ := cats[1].(map[string]any)
	if second["reason"] != "the vacancy's title states no seniority" {
		t.Errorf("an unavailable category dropped its reason; got %s", blob)
	}
	if _, ok := second["items"]; ok {
		t.Errorf("an unavailable category serialized an empty items list; got %s", blob)
	}
}

// An unverifiable requirement carries the earlier analysis's status; a re-derived one must
// not, or the panel would show a stale verdict beside a live one.
func TestRequirementCheckWireShape(t *testing.T) {
	blob, err := json.Marshal(cvmatch.RequirementCheck{
		Text:     "5+ years of Go",
		Priority: "required",
		Coverage: cvmatch.Missing,
		Skills:   []string{"go"},
		Missing:  []string{"go"},
	})
	if err != nil {
		t.Fatalf("marshal requirement: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(blob, &got); err != nil {
		t.Fatalf("unmarshal requirement: %v", err)
	}
	for _, key := range []string{"text", "priority", "coverage", "skills", "missing"} {
		if _, ok := got[key]; !ok {
			t.Errorf("requirement JSON is missing key %q; got %s", key, blob)
		}
	}
	if _, ok := got["cached_status"]; ok {
		t.Errorf("a re-derived requirement serialized a cached status; got %s", blob)
	}
}
