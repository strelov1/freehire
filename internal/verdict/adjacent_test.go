package verdict

import (
	"strings"
	"testing"
)

func TestCompute_AdjacentStatusWhenCloseSkillHeld(t *testing.T) {
	// Role wants tensorflow; the CV lacks it but lists pytorch (adjacent).
	v := Compute(Input{
		Total:      100,
		RoleSkills: map[string]int64{"tensorflow": 60},
		Body:       []string{"pytorch"},
		All:        []string{"pytorch"},
	})
	r, ok := rowByName(v.Skills, "tensorflow")
	if !ok {
		t.Fatal("no tensorflow row")
	}
	if r.Status != StatusAdjacent {
		t.Errorf("status = %q, want adjacent", r.Status)
	}
	if r.Adjacent != "pytorch" {
		t.Errorf("adjacent = %q, want pytorch", r.Adjacent)
	}
}

func TestCompute_MissingWhenNoAdjacentHeld(t *testing.T) {
	v := Compute(Input{
		Total:      100,
		RoleSkills: map[string]int64{"rust": 60},
		Body:       []string{"pytorch"},
		All:        []string{"pytorch"},
	})
	r, _ := rowByName(v.Skills, "rust")
	if r.Status != StatusMissing {
		t.Errorf("status = %q, want missing", r.Status)
	}
}

func TestCompute_AdjacentDoesNotInflateCoverage(t *testing.T) {
	// tensorflow is must-have (60% ≥ 50%) but only adjacent → not covered.
	v := Compute(Input{
		Total:      100,
		RoleSkills: map[string]int64{"tensorflow": 60},
		Body:       []string{"pytorch"},
		All:        []string{"pytorch"},
	})
	if v.MustHaveTotal != 1 {
		t.Errorf("MustHaveTotal = %d, want 1", v.MustHaveTotal)
	}
	if v.MustHaveCovered != 0 {
		t.Errorf("MustHaveCovered = %d, want 0 (adjacent isn't covered)", v.MustHaveCovered)
	}
	if v.StackMatchPercent != 0 {
		t.Errorf("StackMatchPercent = %d, want 0", v.StackMatchPercent)
	}
}

func TestCompute_AdjacentAdviceNamesCloseSkill(t *testing.T) {
	v := Compute(Input{
		Total:      100,
		RoleSkills: map[string]int64{"tensorflow": 60},
		Body:       []string{"pytorch"},
		All:        []string{"pytorch"},
	})
	r, _ := rowByName(v.Skills, "tensorflow")
	if !strings.Contains(r.Advice, "pytorch") {
		t.Errorf("advice = %q, want it to name pytorch", r.Advice)
	}
}

func TestCompute_BundleRowsFromCVSkills(t *testing.T) {
	v := Compute(Input{
		Total: 100,
		All:   []string{"docker", "kubernetes", "ci-cd", "terraform", "aws"},
	})
	var cloudOps bool
	for _, b := range v.Bundles {
		if b.Name == "cloud-ops" && b.Has {
			cloudOps = true
		}
	}
	if !cloudOps {
		t.Errorf("bundles = %+v, want cloud-ops covered", v.Bundles)
	}
}
