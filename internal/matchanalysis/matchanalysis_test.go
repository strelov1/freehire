package matchanalysis

import (
	"strings"
	"testing"
)

func TestBuildAnalysis_WeightedOverallAndVerdict(t *testing.T) {
	// A strong, uniform verdict → weighted overall equals the common score, Strong Fit.
	v := recruiterVerdict{
		TitleAlignment:      dimScore{Score: 80},
		ExperienceRelevance: dimScore{Score: 80},
		SeniorityFit:        dimScore{Score: 80},
		SkillsCoverage:      dimScore{Score: 80},
		CompanyContext:      dimScore{Score: 80},
		LocationFit:         dimScore{Score: 80},
		Recommendation:      "Apply.",
	}
	got := buildAnalysis(nil, v, nil)
	if got.OverallScore != 80 {
		t.Errorf("OverallScore = %d, want 80 (weights sum to 100)", got.OverallScore)
	}
	if got.Verdict != VerdictStrong {
		t.Errorf("Verdict = %q, want %q", got.Verdict, VerdictStrong)
	}
	if len(got.Dimensions) != 6 {
		t.Fatalf("Dimensions = %d, want the 6 canonical dimensions", len(got.Dimensions))
	}
	// The dimensions are server-built in a fixed order regardless of the model.
	if got.Dimensions[0].Key != DimTitleAlignment {
		t.Errorf("Dimensions[0].Key = %q, want %q", got.Dimensions[0].Key, DimTitleAlignment)
	}
	// Location & work-mode fit is the sixth dimension.
	if got.Dimensions[5].Key != DimLocationFit {
		t.Errorf("Dimensions[5].Key = %q, want %q", got.Dimensions[5].Key, DimLocationFit)
	}
}

func TestBuildAnalysis_WeightingFavoursTitleAndExperience(t *testing.T) {
	// Title (20) + Experience (25) high, the rest zero → overall = 45, not 33 (equal weight).
	v := recruiterVerdict{
		TitleAlignment:      dimScore{Score: 100},
		ExperienceRelevance: dimScore{Score: 100},
	}
	got := buildAnalysis(nil, v, nil)
	if got.OverallScore != 45 {
		t.Errorf("OverallScore = %d, want 45 (Title 20 + Experience 25)", got.OverallScore)
	}
}

func TestDimensionWeightsSumTo100(t *testing.T) {
	sum := 0
	for _, s := range dimensionSpecs {
		sum += s.weight
	}
	if sum != 100 {
		t.Fatalf("dimension weights sum to %d, want 100", sum)
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

func TestSanitizeVerdict_RecommendationLength(t *testing.T) {
	under := strings.Repeat("a", maxRecommendRunes)
	v := recruiterVerdict{Recommendation: under}
	sanitizeVerdict(&v)
	if v.Recommendation != under {
		t.Errorf("in-budget recommendation truncated: got %d runes, want %d", len([]rune(v.Recommendation)), maxRecommendRunes)
	}

	over := strings.Repeat("b", maxRecommendRunes+50)
	v = recruiterVerdict{Recommendation: over}
	sanitizeVerdict(&v)
	if got := len([]rune(v.Recommendation)); got != maxRecommendRunes {
		t.Errorf("oversized recommendation = %d runes, want ceiling %d", got, maxRecommendRunes)
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

func TestBuildAnalysis_ThreadsHiddenSignals(t *testing.T) {
	signals := []Signal{{Quote: "fast-paced", Insight: "expect overtime risk"}}
	got := buildAnalysis(nil, recruiterVerdict{}, signals)
	if len(got.HiddenSignals) != 1 || got.HiddenSignals[0].Quote != "fast-paced" {
		t.Errorf("HiddenSignals = %+v, want the passed-in signal", got.HiddenSignals)
	}
}

func TestBuildAnalysis_NilSignalsYieldsEmptyNotNil(t *testing.T) {
	got := buildAnalysis(nil, recruiterVerdict{}, nil)
	if got.HiddenSignals == nil {
		t.Error("HiddenSignals = nil, want an empty (non-nil) slice")
	}
}

func TestSanitizeSignals_DropsBlankTruncatesCaps(t *testing.T) {
	in := []Signal{
		{Quote: "  \"fast-paced, high ownership\"  ", Insight: "  Expect a self-driven pace.  "},
		{Quote: "", Insight: "Should be dropped — blank quote"},
		{Quote: "Some quote", Insight: ""},                                   // blank insight → dropped
		{Quote: strings.Repeat("q", 250), Insight: strings.Repeat("i", 250)}, // over-length → truncated
		{Quote: "extra 1", Insight: "extra 1"},
		{Quote: "extra 2", Insight: "extra 2"},
		{Quote: "extra 3", Insight: "extra 3"},
		{Quote: "extra 4", Insight: "extra 4"}, // 8 total in, cap at maxSignals=5
	}
	got := sanitizeSignals(in)
	if len(got) != maxSignals {
		t.Fatalf("sanitizeSignals len = %d, want %d (blanks dropped, rest capped)", len(got), maxSignals)
	}
	if got[0].Quote != `"fast-paced, high ownership"` {
		t.Errorf("Quote[0] = %q, want trimmed", got[0].Quote)
	}
	if got[0].Insight != "Expect a self-driven pace." {
		t.Errorf("Insight[0] = %q, want trimmed", got[0].Insight)
	}
	if n := len([]rune(got[1].Quote)); n != maxSignalQuoteRunes {
		t.Errorf("Quote[1] rune len = %d, want truncated to %d", n, maxSignalQuoteRunes)
	}
	if n := len([]rune(got[1].Insight)); n != maxSignalInsightRunes {
		t.Errorf("Insight[1] rune len = %d, want truncated to %d", n, maxSignalInsightRunes)
	}
}

func TestSanitizeSignals_NilInYieldsEmptyNotNil(t *testing.T) {
	got := sanitizeSignals(nil)
	if got == nil {
		t.Error("sanitizeSignals(nil) = nil, want an empty (non-nil) slice")
	}
	if len(got) != 0 {
		t.Errorf("sanitizeSignals(nil) len = %d, want 0", len(got))
	}
}

func TestSanitizeRequirements_EvidenceStrength(t *testing.T) {
	in := []Requirement{
		{Text: "Go", Priority: "required", Status: "covered", EvidenceStrength: "METRIC"},         // recognised, case-folded
		{Text: "Kafka", Priority: "preferred", Status: "synonym-only", EvidenceStrength: ""},      // blank positive → keyword
		{Text: "Rust", Priority: "required", Status: "covered", EvidenceStrength: "off-the-wall"}, // unknown positive → keyword
		{Text: "K8s", Priority: "required", Status: "missing-gap", EvidenceStrength: "metric"},    // missing-* → dropped
		{Text: "SQL", Priority: "preferred", Status: "missing-have", EvidenceStrength: "scope"},   // missing-* → dropped
	}
	got := sanitizeRequirements(in)
	want := []string{StrengthMetric, StrengthKeyword, StrengthKeyword, "", ""}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i].EvidenceStrength != w {
			t.Errorf("req[%d] (%s/%s) EvidenceStrength = %q, want %q", i, got[i].Priority, got[i].Status, got[i].EvidenceStrength, w)
		}
	}
}

// TestProductionSanitizeBoundsLocked pins the production defaults. These are the
// sanitize ceilings that ship when MATCH_ANALYSIS_* is unset — change them only
// with an intentional product decision, not as drive-by tuning. Env overrides
// remain available for local experiments.
func TestProductionSanitizeBoundsLocked(t *testing.T) {
	want := Bounds{
		MaxCommentRunes:       240,
		MaxListItemRunes:      200,
		MaxRecommendRunes:     1200,
		MaxReqTextRunes:       200,
		MaxReqEvidenceRunes:   240,
		MaxStrengths:          6,
		MaxGaps:               6,
		MaxRequirements:       30,
		MaxSignals:            5,
		MaxSignalQuoteRunes:   200,
		MaxSignalInsightRunes: 200,
	}
	if got := DefaultBounds(); got != want {
		t.Fatalf("DefaultBounds() = %+v, want production defaults %+v — do not change without review", got, want)
	}
	// Fresh package state (no SetBounds yet in this test) must match the same table.
	if got := CurrentBounds(); got != want {
		t.Fatalf("CurrentBounds() = %+v before SetBounds, want production defaults %+v", got, want)
	}
}

func TestSetBoundsRejectsNonPositive(t *testing.T) {
	prev := CurrentBounds()
	t.Cleanup(func() { SetBounds(prev) })

	b := DefaultBounds()
	b.MaxRecommendRunes = 0
	b.MaxSignals = -3
	b.MaxStrengths = 12
	SetBounds(b)
	got := CurrentBounds()
	if got.MaxRecommendRunes != DefaultMaxRecommendRunes {
		t.Fatalf("MaxRecommendRunes = %d after 0, want default %d", got.MaxRecommendRunes, DefaultMaxRecommendRunes)
	}
	if got.MaxSignals != DefaultMaxSignals {
		t.Fatalf("MaxSignals = %d after -3, want default %d", got.MaxSignals, DefaultMaxSignals)
	}
	if got.MaxStrengths != 12 {
		t.Fatalf("MaxStrengths = %d, want 12", got.MaxStrengths)
	}
}

func TestSetBoundsRaisesRecommendationCeiling(t *testing.T) {
	prev := CurrentBounds()
	t.Cleanup(func() { SetBounds(prev) })

	b := DefaultBounds()
	b.MaxRecommendRunes = 2000
	SetBounds(b)
	over := strings.Repeat("r", 1500)
	v := recruiterVerdict{Recommendation: over}
	sanitizeVerdict(&v)
	if got := len([]rune(v.Recommendation)); got != 1500 {
		t.Fatalf("recommendation rune len = %d, want 1500 under raised ceiling", got)
	}
}
