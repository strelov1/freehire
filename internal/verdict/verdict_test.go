package verdict

import (
	"reflect"
	"testing"
)

// intptr is a test helper for the optional *int fields.
func intptr(n int) *int { return &n }

func TestCompute_RanksByDemandWithSlugTiebreak(t *testing.T) {
	market := MarketSkills{
		Counts: map[string]int64{"go": 10, "python": 10, "rust": 5},
		Total:  100,
	}
	v := Compute(market, nil)

	// Equal counts (go, python) break ties by ascending slug → go before python.
	got := []string{v.Skills[0].Name, v.Skills[1].Name, v.Skills[2].Name}
	want := []string{"go", "python", "rust"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("rank order = %v, want %v", got, want)
	}
	for i, s := range v.Skills {
		if s.Rank != i+1 {
			t.Errorf("skill %q rank = %d, want %d", s.Name, s.Rank, i+1)
		}
	}
}

func TestCompute_TruncatesToTopN(t *testing.T) {
	counts := make(map[string]int64, TopN+5)
	for i := 0; i < TopN+5; i++ {
		// Distinct counts so ordering is unambiguous.
		counts[string(rune('a'+i))] = int64(100 - i)
	}
	v := Compute(MarketSkills{Counts: counts, Total: 1000}, nil)
	if len(v.Skills) != TopN {
		t.Fatalf("len(skills) = %d, want %d", len(v.Skills), TopN)
	}
}

func TestCompute_StackMatchAndCoverage(t *testing.T) {
	// 4 market skills, candidate has 1 → 25%.
	market := MarketSkills{
		Counts: map[string]int64{"go": 40, "python": 30, "sql": 20, "rust": 10},
		Total:  100,
	}
	v := Compute(market, []string{"python"})
	if v.StackMatch != 25 {
		t.Errorf("StackMatch = %d, want 25", v.StackMatch)
	}
	for _, s := range v.Skills {
		if s.Name == "python" && !s.Have {
			t.Errorf("python should be Have")
		}
		if s.Name == "go" && s.Have {
			t.Errorf("go should not be Have")
		}
	}
}

func TestCompute_FewerThanTopNDenominator(t *testing.T) {
	// Only 8 skills; candidate has 2 → round(2/8*100) = 25.
	counts := map[string]int64{}
	for i := 0; i < 8; i++ {
		counts[string(rune('a'+i))] = int64(100 - i)
	}
	v := Compute(MarketSkills{Counts: counts, Total: 1000}, []string{"a", "b"})
	if len(v.Skills) != 8 {
		t.Fatalf("len(skills) = %d, want 8", len(v.Skills))
	}
	if v.StackMatch != 25 {
		t.Errorf("StackMatch = %d, want 25 (2/8)", v.StackMatch)
	}
}

func TestCompute_UnlockOnGapsOnly(t *testing.T) {
	// go: 340/1000 = 34%. Candidate lacks go → unlock 34; python owned → no unlock.
	market := MarketSkills{
		Counts: map[string]int64{"go": 340, "python": 500},
		Total:  1000,
	}
	v := Compute(market, []string{"python"})
	byName := map[string]Skill{}
	for _, s := range v.Skills {
		byName[s.Name] = s
	}
	if got := byName["go"]; got.Have || got.Unlock == nil || *got.Unlock != 34 {
		t.Errorf("go = %+v, want Have=false Unlock=34", got)
	}
	if got := byName["python"]; !got.Have || got.Unlock != nil {
		t.Errorf("python = %+v, want Have=true Unlock=nil", got)
	}
}

func TestCompute_MustHaveShareBoundary(t *testing.T) {
	// at: exactly 40% → must-have; below: 39% → not.
	market := MarketSkills{
		Counts: map[string]int64{"at": 400, "below": 390},
		Total:  1000,
	}
	v := Compute(market, nil)
	byName := map[string]Skill{}
	for _, s := range v.Skills {
		byName[s.Name] = s
	}
	if !byName["at"].MustHave {
		t.Errorf("skill at 40%% share should be a must-have")
	}
	if byName["below"].MustHave {
		t.Errorf("skill at 39%% share should not be a must-have")
	}
	if v.MustHaveTotal != 1 {
		t.Errorf("MustHaveTotal = %d, want 1", v.MustHaveTotal)
	}
}

func TestCompute_MustHaveCovered(t *testing.T) {
	market := MarketSkills{
		Counts: map[string]int64{"go": 600, "python": 500, "sql": 100},
		Total:  1000,
	}
	// go & python are must-haves (>=40%); candidate has go only.
	v := Compute(market, []string{"go"})
	if v.MustHaveTotal != 2 {
		t.Errorf("MustHaveTotal = %d, want 2", v.MustHaveTotal)
	}
	if v.MustHaveCovered != 1 {
		t.Errorf("MustHaveCovered = %d, want 1", v.MustHaveCovered)
	}
}

func TestCompute_EmptyCandidate(t *testing.T) {
	market := MarketSkills{Counts: map[string]int64{"go": 40, "python": 30}, Total: 100}
	v := Compute(market, nil)
	if v.StackMatch != 0 {
		t.Errorf("StackMatch = %d, want 0", v.StackMatch)
	}
	for _, s := range v.Skills {
		if s.Have {
			t.Errorf("skill %q should be a gap", s.Name)
		}
	}
}

func TestCompute_ZeroTotalNoPanic(t *testing.T) {
	// Total 0 must not divide-by-zero; unlock defaults to 0, nothing is a must-have.
	market := MarketSkills{Counts: map[string]int64{"go": 0, "python": 0}, Total: 0}
	v := Compute(market, nil)
	if len(v.Skills) != 2 {
		t.Fatalf("len(skills) = %d, want 2", len(v.Skills))
	}
	for _, s := range v.Skills {
		if s.MustHave {
			t.Errorf("no must-haves when total is 0")
		}
		if s.Unlock == nil || *s.Unlock != 0 {
			t.Errorf("unlock = %v, want 0", s.Unlock)
		}
	}
}

func TestVerdict_MustHaveGaps(t *testing.T) {
	market := MarketSkills{
		Counts: map[string]int64{"go": 600, "python": 500, "sql": 100},
		Total:  1000,
	}
	// go, python must-haves; candidate has go. Gap = python only.
	v := Compute(market, []string{"go"})
	got := v.MustHaveGaps()
	want := []string{"python"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("MustHaveGaps() = %v, want %v", got, want)
	}
}
