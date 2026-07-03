package verdict

import "testing"

func rowByName(rows []SkillRow, name string) (SkillRow, bool) {
	for _, r := range rows {
		if r.Name == name {
			return r, true
		}
	}
	return SkillRow{}, false
}

func TestCompute_SkillStatusStrongHiddenMissing(t *testing.T) {
	v := Compute(Input{
		Total:      1000,
		RoleSkills: map[string]int64{"kubernetes": 600, "kafka": 300, "rust": 100},
		Declared:   []string{"kubernetes"},
		Body:       []string{"kubernetes", "kafka"},
	})

	want := map[string]string{"kubernetes": StatusStrong, "kafka": StatusHidden, "rust": StatusMissing}
	for name, ws := range want {
		r, ok := rowByName(v.Skills, name)
		if !ok {
			t.Fatalf("no skill row for %q", name)
		}
		if r.Status != ws {
			t.Errorf("%s status = %q, want %q", name, r.Status, ws)
		}
	}
	k, _ := rowByName(v.Skills, "kubernetes")
	if k.MarketFrequency != 60 {
		t.Errorf("kubernetes market_frequency = %d, want 60", k.MarketFrequency)
	}
	if k.Advice != "" {
		t.Errorf("strong skill advice = %q, want empty", k.Advice)
	}
	if r, _ := rowByName(v.Skills, "kafka"); r.Advice == "" {
		t.Errorf("hidden skill advice is empty, want guidance")
	}
	if r, _ := rowByName(v.Skills, "rust"); r.Advice == "" {
		t.Errorf("missing skill advice is empty, want guidance")
	}
}

func TestCompute_MustHaveByFrequencyThreshold(t *testing.T) {
	v := Compute(Input{
		Total:      1000,
		RoleSkills: map[string]int64{"python": 620, "cobol": 30},
	})
	if r, _ := rowByName(v.Skills, "python"); !r.MustHave {
		t.Errorf("python must_have = false, want true (62%% ≥ threshold)")
	}
	if r, _ := rowByName(v.Skills, "cobol"); r.MustHave {
		t.Errorf("cobol must_have = true, want false (3%% < threshold)")
	}
}

func TestCompute_MustHaveCoverage(t *testing.T) {
	// Two must-haves (≥50%): kubernetes held (strong), postgresql missing.
	v := Compute(Input{
		Total:      1000,
		RoleSkills: map[string]int64{"kubernetes": 600, "postgresql": 550, "rust": 100},
		Declared:   []string{"kubernetes"},
		Body:       []string{"kubernetes"},
	})
	if v.MustHaveTotal != 2 {
		t.Errorf("MustHaveTotal = %d, want 2", v.MustHaveTotal)
	}
	if v.MustHaveCovered != 1 {
		t.Errorf("MustHaveCovered = %d, want 1", v.MustHaveCovered)
	}
}

func TestCompute_StackMatchBreadth(t *testing.T) {
	// a strong, b hidden, c & d missing → 2 of 4 held → 50%.
	v := Compute(Input{
		Total:      1000,
		RoleSkills: map[string]int64{"a": 10, "b": 10, "c": 10, "d": 10},
		Declared:   []string{"a"},
		Body:       []string{"a", "b"},
	})
	if v.StackMatchPercent != 50 {
		t.Errorf("StackMatchPercent = %d, want 50", v.StackMatchPercent)
	}
}

func TestCompute_CoherencePercent(t *testing.T) {
	// declared {kubernetes, redis}, only kubernetes backed by the body → 1/2 = 50%.
	v := Compute(Input{
		Total:    1000,
		Declared: []string{"kubernetes", "redis"},
		Body:     []string{"kubernetes"},
	})
	if v.CoherencePercent != 50 {
		t.Errorf("CoherencePercent = %d, want 50", v.CoherencePercent)
	}
}

func TestCompute_CoherenceZeroWhenNoDeclared(t *testing.T) {
	v := Compute(Input{
		Total:    1000,
		Declared: nil,
		Body:     []string{"kubernetes"},
	})
	if v.CoherencePercent != 0 {
		t.Errorf("CoherencePercent = %d, want 0", v.CoherencePercent)
	}
}
