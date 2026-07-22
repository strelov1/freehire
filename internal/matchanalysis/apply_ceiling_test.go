package matchanalysis

import "testing"

func TestApplyCeilingCapsAndDerivesVerdict(t *testing.T) {
	a := &Analysis{OverallScore: 88, Verdict: verdictFor(88)}
	ApplyCeiling(a, 60)
	if a.OverallScore != 60 {
		t.Errorf("OverallScore = %d, want 60", a.OverallScore)
	}
	if a.Verdict != verdictFor(60) {
		t.Errorf("Verdict = %q, want %q (derived from capped score)", a.Verdict, verdictFor(60))
	}
}

func TestApplyCeilingNoOpWhenCeilingNotBinding(t *testing.T) {
	a := &Analysis{OverallScore: 72, Verdict: verdictFor(72)}
	ApplyCeiling(a, 100) // no unmet blocker → ceiling 100 → unchanged
	if a.OverallScore != 72 {
		t.Errorf("OverallScore = %d, want 72 (unchanged)", a.OverallScore)
	}
}
