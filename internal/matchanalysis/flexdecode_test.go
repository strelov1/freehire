package matchanalysis

import (
	"encoding/json"
	"testing"
)

// The recruiter stage is prompted for integer dimension scores, but the model can return
// them as strings ("85") or ratios ("8/10"). A string score would abort the whole verdict
// decode (six dimensions + free-text) — this is the user-visible fit-analysis path.
func TestRecruiterVerdict_StringScoreDecodes(t *testing.T) {
	raw := `{
		"title_alignment": {"score": "85", "comment": "strong"},
		"skills_coverage": {"score": "8/10", "comment": "partial"},
		"seniority_fit":   {"score": 70, "comment": "ok"}
	}`
	var v recruiterVerdict
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		t.Fatalf("unmarshal verdict with string scores failed: %v", err)
	}
	if v.TitleAlignment.Score != 85 {
		t.Errorf("TitleAlignment.Score = %d, want 85", v.TitleAlignment.Score)
	}
	if v.SkillsCoverage.Score != 8 {
		t.Errorf("SkillsCoverage.Score = %d, want 8", v.SkillsCoverage.Score)
	}
	if v.SeniorityFit.Score != 70 {
		t.Errorf("SeniorityFit.Score = %d, want 70", v.SeniorityFit.Score)
	}
	if v.TitleAlignment.Comment != "strong" {
		t.Errorf("TitleAlignment.Comment = %q, want %q", v.TitleAlignment.Comment, "strong")
	}
}
