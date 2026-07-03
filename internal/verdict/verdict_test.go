package verdict

import (
	"reflect"
	"testing"
)

func TestCompute_CoverageCountAndPercent(t *testing.T) {
	// 1000 open role vacancies, 370 uncovered → 630 covered → 63%.
	v := Compute(Input{Total: 1000, UncoveredTotal: 370})
	if v.Total != 1000 {
		t.Errorf("Total = %d, want 1000", v.Total)
	}
	if v.Covered != 630 {
		t.Errorf("Covered = %d, want 630", v.Covered)
	}
	if v.CoveragePercent != 63 {
		t.Errorf("CoveragePercent = %d, want 63", v.CoveragePercent)
	}
}

func TestCompute_ZeroTotalIsZeroPercent(t *testing.T) {
	// A role with no open vacancies must not divide by zero.
	v := Compute(Input{Total: 0, UncoveredTotal: 0})
	if v.Total != 0 || v.Covered != 0 || v.CoveragePercent != 0 {
		t.Errorf("got Total=%d Covered=%d Percent=%d, want all 0", v.Total, v.Covered, v.CoveragePercent)
	}
	if len(v.Gaps) != 0 {
		t.Errorf("len(gaps) = %d, want 0", len(v.Gaps))
	}
}

func TestCompute_ClampsNegativeCoverageFromEstimateSkew(t *testing.T) {
	// Total and uncovered come from two independent Meilisearch estimates; skew can put
	// uncovered above total. Coverage must floor at 0, not report a negative count/percent.
	v := Compute(Input{Total: 100, UncoveredTotal: 130, UncoveredSkills: map[string]int64{"go": 40}})
	if v.Covered != 0 || v.CoveragePercent != 0 {
		t.Errorf("Covered=%d CoveragePercent=%d, want 0/0", v.Covered, v.CoveragePercent)
	}
}

func TestCompute_GapCarriesNewVacancies(t *testing.T) {
	// kubernetes appears in 190 of the 1000-vacancy role's uncovered vacancies → +190, +19%.
	v := Compute(Input{Total: 1000, UncoveredTotal: 190, UncoveredSkills: map[string]int64{"kubernetes": 190}})
	if len(v.Gaps) != 1 {
		t.Fatalf("len(gaps) = %d, want 1", len(v.Gaps))
	}
	g := v.Gaps[0]
	if g.Name != "kubernetes" || g.NewVacancies != 190 || g.UnlockPercent != 19 {
		t.Errorf("gap = %+v, want {kubernetes 190 19}", g)
	}
}

func TestCompute_GapsRankedBiggestWinFirst(t *testing.T) {
	v := Compute(Input{Total: 1000, UncoveredTotal: 200, UncoveredSkills: map[string]int64{"kafka": 120, "grpc": 64}})
	got := []string{v.Gaps[0].Name, v.Gaps[1].Name}
	want := []string{"kafka", "grpc"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("gap order = %v, want %v", got, want)
	}
}

func TestCompute_GapsTieBreakBySlug(t *testing.T) {
	// Equal new-vacancy counts break ties by ascending slug for determinism.
	v := Compute(Input{Total: 1000, UncoveredTotal: 100, UncoveredSkills: map[string]int64{"rust": 50, "go": 50}})
	got := []string{v.Gaps[0].Name, v.Gaps[1].Name}
	want := []string{"go", "rust"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("tie-break order = %v, want %v", got, want)
	}
}

func TestCompute_GapsCappedAtMax(t *testing.T) {
	skills := make(map[string]int64, MaxGaps+5)
	for i := 0; i < MaxGaps+5; i++ {
		skills[string(rune('a'+i))] = int64(100 - i)
	}
	v := Compute(Input{Total: 1000, UncoveredTotal: 500, UncoveredSkills: skills})
	if len(v.Gaps) != MaxGaps {
		t.Fatalf("len(gaps) = %d, want %d", len(v.Gaps), MaxGaps)
	}
}
