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
	p := buildSystemPrompt(true)

	if !strings.Contains(p, "regions") {
		t.Errorf("prompt must mention regions, got:\n%s", p)
	}
	for _, v := range RegionValues {
		if !strings.Contains(p, v) {
			t.Errorf("prompt must list region value %q", v)
		}
	}
}

// The prompt must NOT request the dict-backed facets: jobview serves them from the
// deterministic dictionaries (internal/jobderive), so the LLM's copies are never
// served and asking for them only burns output tokens. Removing them from the prompt
// is the whole point of the enrich-prompt-trim change.
func TestSystemPromptOmitsDictBackedFacets(t *testing.T) {
	p := buildSystemPrompt(true)
	for _, f := range []string{
		"work_mode", "seniority", "category", "skills",
		"employment_type", "education_level", "english_level",
		"posting_language", "experience_years_min",
	} {
		if strings.Contains(p, f) {
			t.Errorf("prompt must not request dict-backed facet %q (served from dictionaries), got:\n%s", f, p)
		}
	}
}

// The prompt must still request every served or hybrid field it is the source of:
// the synthesized summary, salary extraction, the served enums, the geographic
// hybrid (countries/regions), and relocation. countries/regions keep their novel
// own-label allowance (they are the sole remaining discovery facets); the served
// enums keep the strict "exactly one allowed value" instruction.
func TestSystemPromptKeepsServedAndHybridFields(t *testing.T) {
	p := buildSystemPrompt(true)
	for _, f := range []string{
		"summary",
		"salary_min", "salary_max", "salary_currency", "salary_period",
		"visa_sponsorship", "timezone_note",
		"company_type", "company_size", "domains",
		"relocation", "countries", "regions",
	} {
		if !strings.Contains(p, f) {
			t.Errorf("prompt must still request served/hybrid field %q, got:\n%s", f, p)
		}
	}
	if !strings.Contains(p, "exactly one of the allowed values") {
		t.Errorf("served enum fields must keep the strict instruction")
	}
	if !strings.Contains(p, "concise lowercase label of your own") {
		t.Errorf("countries/regions must keep the novel own-label allowance")
	}
}

// A fractional hourly rate ("$26.08/hr") must be rounded to a whole currency unit,
// never decimal-stripped into an inflated integer (26.08 -> 2608). The guard anchors
// the weak model on the exact counter-example, so the prompt must carry it verbatim.
func TestSystemPromptGuardsFractionalHourlySalary(t *testing.T) {
	for _, askGeo := range []bool{true, false} {
		p := buildSystemPrompt(askGeo)
		for _, want := range []string{"whole", "26.08", "2608"} {
			if !strings.Contains(p, want) {
				t.Errorf("askGeo=%v: salary guard must mention %q, got:\n%s", askGeo, want, p)
			}
		}
	}
}

// When the dictionary already pinned the job's geography, the LLM's countries/regions
// are discarded by geoFacet, so the prompt must not ask for them — dropping the enum,
// the exception block, the "Other keys" mention, and the geographic-area paragraph.
func TestSystemPromptOmitsGeoWhenPinned(t *testing.T) {
	p := buildSystemPrompt(false)
	for _, absent := range []string{"regions", "countries", "geographic area", "Exception for countries"} {
		if strings.Contains(p, absent) {
			t.Errorf("pinned-geo prompt must omit %q, got:\n%s", absent, p)
		}
	}
	// Non-geo fields it is still the source of must remain.
	for _, want := range []string{"summary", "salary_min", "domains", "company_type"} {
		if !strings.Contains(p, want) {
			t.Errorf("pinned-geo prompt must keep non-geo field %q, got:\n%s", want, p)
		}
	}
}

// When the dictionary left geography unpinned, the LLM fills the bucket, so the
// prompt must still request countries/regions and explain the regions facet.
func TestSystemPromptAsksGeoWhenUnpinned(t *testing.T) {
	p := buildSystemPrompt(true)
	for _, want := range []string{"regions", "countries", "geographic area"} {
		if !strings.Contains(p, want) {
			t.Errorf("unpinned-geo prompt must request %q, got:\n%s", want, p)
		}
	}
}

// GeoPinned mirrors jobview.geoPinned: a country, or a region more specific than the
// open-anywhere "global" bucket, counts as pinned; nothing, or only "global", does not.
func TestGeoPinned(t *testing.T) {
	cases := []struct {
		name       string
		countries  []string
		regions    []string
		wantPinned bool
	}{
		{"country pins", []string{"us"}, nil, true},
		{"non-global region pins", nil, []string{"eu"}, true},
		{"only global is unpinned", nil, []string{"global"}, false},
		{"empty is unpinned", nil, nil, false},
	}
	for _, c := range cases {
		if got := GeoPinned(c.countries, c.regions); got != c.wantPinned {
			t.Errorf("%s: GeoPinned(%v,%v)=%v, want %v", c.name, c.countries, c.regions, got, c.wantPinned)
		}
	}
}
