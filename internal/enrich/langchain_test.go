package enrich

import (
	"strings"
	"testing"
)

// The per-call timeout / generate / choices behavior now lives in internal/llm
// and is tested there (TestGenerateJSONTimesOutOnStalledModel). These tests cover
// the enrich-specific prompt construction.

// The user prompt must include the job URL: some ATS encode the location (and
// sometimes the role) in the URL path (e.g. SuccessFactors
// /job/Limburg-Maschinenfuhrer/<id>/), which the model can read even when the
// structured location field is empty.
func TestUserPromptIncludesURL(t *testing.T) {
	url := "https://jobs.tetrapak.com/job/Limburg-Maschinenfuhrer/883999301/"
	got := userPrompt(JobInput{Title: "Engineer", Company: "Acme", URL: url, Description: "x"})
	if !strings.Contains(got, url) {
		t.Errorf("userPrompt must include the URL (a location signal for slug-based ATS), got:\n%s", got)
	}
}

// The system prompt must pin the region (reach) vocabulary it asks the model to
// use, drawn from the same list Validate enforces, so prompt and validator
// cannot drift.
func TestSystemPromptIncludesRegionVocabulary(t *testing.T) {
	p := buildSystemPrompt()

	if !strings.Contains(p, "regions") {
		t.Errorf("prompt must mention regions, got:\n%s", p)
	}
	for _, v := range RegionValues {
		if !strings.Contains(p, v) {
			t.Errorf("prompt must list region value %q", v)
		}
	}
}

// Relax: the prompt permits a novel own-label for the six discovery facets while
// keeping the strict "exactly one allowed value" instruction for the served fields.
func TestSystemPromptRelaxesDiscoveryFacets(t *testing.T) {
	p := buildSystemPrompt()
	if !strings.Contains(p, "exactly one of the allowed values") {
		t.Errorf("served enum fields must keep the strict instruction")
	}
	if !strings.Contains(p, "concise lowercase label of your own") {
		t.Errorf("discovery facets must permit a novel own label")
	}
	for _, f := range []string{"work_mode", "regions", "seniority", "category"} {
		if !strings.Contains(p, f) {
			t.Errorf("discovery instruction should name the facet %q", f)
		}
	}
}
