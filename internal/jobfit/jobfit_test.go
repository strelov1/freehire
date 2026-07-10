package jobfit

import "testing"

func TestBuildAnalysis_WeightedOverallAndVerdict(t *testing.T) {
	// A strong, uniform verdict → weighted overall equals the common score, Strong Fit.
	v := recruiterVerdict{
		TitleAlignment:      dimScore{Score: 80},
		ExperienceRelevance: dimScore{Score: 80},
		SeniorityFit:        dimScore{Score: 80},
		SkillsCoverage:      dimScore{Score: 80},
		CompanyContext:      dimScore{Score: 80},
		Recommendation:      "Apply.",
	}
	got := buildAnalysis(nil, v)
	if got.OverallScore != 80 {
		t.Errorf("OverallScore = %d, want 80 (weights sum to 100)", got.OverallScore)
	}
	if got.Verdict != VerdictStrong {
		t.Errorf("Verdict = %q, want %q", got.Verdict, VerdictStrong)
	}
	if len(got.Dimensions) != 5 {
		t.Fatalf("Dimensions = %d, want the 5 canonical dimensions", len(got.Dimensions))
	}
	// The dimensions are server-built in a fixed order regardless of the model.
	if got.Dimensions[0].Key != DimTitleAlignment {
		t.Errorf("Dimensions[0].Key = %q, want %q", got.Dimensions[0].Key, DimTitleAlignment)
	}
}

func TestBuildAnalysis_WeightingFavoursTitleAndExperience(t *testing.T) {
	// Title (25) + Experience (25) high, the rest zero → overall = 50, not 20 (equal weight).
	v := recruiterVerdict{
		TitleAlignment:      dimScore{Score: 100},
		ExperienceRelevance: dimScore{Score: 100},
	}
	got := buildAnalysis(nil, v)
	if got.OverallScore != 50 {
		t.Errorf("OverallScore = %d, want 50 (Title 25 + Experience 25)", got.OverallScore)
	}
}

func TestVerdictFor_Thresholds(t *testing.T) {
	cases := map[int]string{
		90: VerdictStrong, 75: VerdictStrong,
		74: VerdictGood, 60: VerdictGood,
		59: VerdictModerate, 45: VerdictModerate,
		44: VerdictWeak, 30: VerdictWeak,
		29: VerdictPoor, 0: VerdictPoor,
	}
	for score, want := range cases {
		if got := verdictFor(score); got != want {
			t.Errorf("verdictFor(%d) = %q, want %q", score, got, want)
		}
	}
}

func TestSanitizeVerdict_ClampsAndBounds(t *testing.T) {
	v := recruiterVerdict{
		TitleAlignment:      dimScore{Score: 150, Comment: "  strong title  "},
		ExperienceRelevance: dimScore{Score: -20},
		Strengths:           []string{"  Ships fast  ", "", "Solid Go"},
		Gaps:                []string{"No Kafka"},
		Recommendation:      "  Apply with a tailored CV.  ",
	}
	sanitizeVerdict(&v)
	if v.TitleAlignment.Score != 100 {
		t.Errorf("TitleAlignment clamp = %d, want 100", v.TitleAlignment.Score)
	}
	if v.ExperienceRelevance.Score != 0 {
		t.Errorf("ExperienceRelevance clamp = %d, want 0", v.ExperienceRelevance.Score)
	}
	if v.TitleAlignment.Comment != "strong title" {
		t.Errorf("comment = %q, want trimmed", v.TitleAlignment.Comment)
	}
	if len(v.Strengths) != 2 {
		t.Errorf("Strengths = %v, want empties dropped (2)", v.Strengths)
	}
	if v.Recommendation != "Apply with a tailored CV." {
		t.Errorf("Recommendation = %q, want trimmed", v.Recommendation)
	}
}

func TestSanitizeRequirements_CoercesAndDrops(t *testing.T) {
	in := []Requirement{
		{Text: "  Go  ", Priority: "required", Status: "covered", Evidence: "3y at Acme"},
		{Text: "Kafka", Priority: "nice-to-have", Status: "missing-gap"}, // priority coerced → preferred
		{Text: "Rust", Priority: "preferred", Status: "wat"},             // invalid status → dropped
		{Text: "", Priority: "required", Status: "covered"},              // empty text → dropped
	}
	got := sanitizeRequirements(in)
	if len(got) != 2 {
		t.Fatalf("sanitizeRequirements len = %d, want 2 (invalid status + empty dropped)", len(got))
	}
	if got[0].Text != "Go" {
		t.Errorf("req[0].Text = %q, want trimmed", got[0].Text)
	}
	if got[1].Priority != PriorityPreferred {
		t.Errorf("req[1].Priority = %q, want coerced to %q", got[1].Priority, PriorityPreferred)
	}
}
