package jobmatch

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestCompute_WorkedExample(t *testing.T) {
	// Job wants 5 skills; profile has react, typescript exactly and gcp
	// (a neighbour of aws). 2 exact + 1 adjacent of 5 → 50%.
	got := Compute(
		[]string{"react", "typescript", "graphql", "nodejs", "aws"},
		[]string{"react", "typescript", "gcp"},
	)

	if got.Total != 5 {
		t.Errorf("Total = %d, want 5", got.Total)
	}
	if got.ExactCount != 2 {
		t.Errorf("ExactCount = %d, want 2", got.ExactCount)
	}
	if got.AdjacentCount != 1 {
		t.Errorf("AdjacentCount = %d, want 1", got.AdjacentCount)
	}
	if got.CoveragePercent != 50 {
		t.Errorf("CoveragePercent = %d, want 50", got.CoveragePercent)
	}
	if !reflect.DeepEqual(got.Matched, []string{"react", "typescript"}) {
		t.Errorf("Matched = %v, want [react typescript]", got.Matched)
	}
	if !reflect.DeepEqual(got.Adjacent, []AdjacentSkill{{Name: "aws", Via: "gcp"}}) {
		t.Errorf("Adjacent = %+v, want [{aws gcp}]", got.Adjacent)
	}
	if !reflect.DeepEqual(got.Missing, []string{"graphql", "nodejs"}) {
		t.Errorf("Missing = %v, want [graphql nodejs]", got.Missing)
	}
}

func TestCompute_ExactTakesPrecedenceOverAdjacent(t *testing.T) {
	// Profile holds react exactly and vue (a neighbour) — react must count exact.
	got := Compute([]string{"react"}, []string{"react", "vue"})
	if got.ExactCount != 1 || got.AdjacentCount != 0 {
		t.Errorf("exact=%d adjacent=%d, want exact=1 adjacent=0", got.ExactCount, got.AdjacentCount)
	}
	if got.CoveragePercent != 100 {
		t.Errorf("CoveragePercent = %d, want 100", got.CoveragePercent)
	}
}

func TestCompute_HalfWeightRounding(t *testing.T) {
	// 0 exact, 1 adjacent (aws via gcp) of 3 → round(0.5/3*100) = round(16.67) = 17.
	got := Compute([]string{"aws", "kafka", "rust"}, []string{"gcp"})
	if got.ExactCount != 0 || got.AdjacentCount != 1 {
		t.Fatalf("exact=%d adjacent=%d, want exact=0 adjacent=1", got.ExactCount, got.AdjacentCount)
	}
	if got.CoveragePercent != 17 {
		t.Errorf("CoveragePercent = %d, want 17", got.CoveragePercent)
	}
}

func TestCompute_RoundsHalfPercentAwayFromZero(t *testing.T) {
	// 1 exact of 8 unrelated skills → 1/8 = 12.5% → round half away from zero = 13.
	job := []string{"go", "rust", "elixir", "haskell", "scala", "clojure", "erlang", "ocaml"}
	got := Compute(job, []string{"go"})
	if got.ExactCount != 1 || got.AdjacentCount != 0 {
		t.Fatalf("exact=%d adjacent=%d, want 1/0", got.ExactCount, got.AdjacentCount)
	}
	if got.CoveragePercent != 13 {
		t.Errorf("CoveragePercent = %d, want 13 (round(12.5))", got.CoveragePercent)
	}
}

func TestCompute_NoRecognisedJobSkills(t *testing.T) {
	got := Compute(nil, []string{"go"})
	if got.Total != 0 || got.CoveragePercent != 0 {
		t.Errorf("Total=%d percent=%d, want 0/0", got.Total, got.CoveragePercent)
	}
	if len(got.Matched) != 0 || len(got.Adjacent) != 0 || len(got.Missing) != 0 {
		t.Errorf("lists not empty: %+v", got)
	}
}

func TestCompute_EmptyListsMarshalAsArrays(t *testing.T) {
	// A missing-only result must serialise matched/adjacent as [] not null.
	got := Compute([]string{"rust"}, nil)
	b, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, `"matched":[]`) || !strings.Contains(s, `"adjacent":[]`) {
		t.Errorf("json = %s, want matched/adjacent as []", s)
	}
	if !strings.Contains(s, `"missing":["rust"]`) {
		t.Errorf("json = %s, want missing [rust]", s)
	}
}
