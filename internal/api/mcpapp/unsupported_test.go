package mcpapp

import (
	"slices"
	"testing"
)

// Measured against production on 2026-09-18: `/jobs/search?category=ai` answers 0 results
// and reports nothing. "ai" is not one of our categories, so the filter matches no posting
// — and a model that asked for AI jobs reads that zero as "freehire holds no AI jobs" and
// says so. An empty catalogue and a word outside the vocabulary are not the same answer.
//
// So a value we hold no facet for is DROPPED and named, rather than narrowing to nothing.
// The free text still does its work, and the model learns which word was not understood.

func TestAValueOutsideAClosedVocabularyIsDroppedAndNamed(t *testing.T) {
	in := SearchInput{Query: "ai engineer", Category: []string{"ai"}}

	if got := in.QueryValues().Get("category"); got != "" {
		t.Errorf("category = %q, want the unknown value dropped rather than filtered on", got)
	}
	if got := in.Unsupported(); !slices.Contains(got, "category=ai") {
		t.Errorf("unsupported = %v, want it to name category=ai", got)
	}
}

func TestKnownValuesSurviveBesideAnUnknownOne(t *testing.T) {
	// Dropping the whole facet because one word was wrong would throw away a filter the
	// caller got right.
	in := SearchInput{Category: []string{"backend", "ai"}}

	if got := in.QueryValues().Get("category"); got != "backend" {
		t.Errorf("category = %q, want the known value kept", got)
	}
	if got := in.Unsupported(); !slices.Contains(got, "category=ai") {
		t.Errorf("unsupported = %v, want only the unknown value named", got)
	}
}

func TestEveryClosedVocabularyIsChecked(t *testing.T) {
	cases := []struct {
		param string
		input SearchInput
	}{
		{"work_mode", SearchInput{WorkMode: []string{"anywhere"}}},
		{"seniority", SearchInput{Seniority: []string{"rockstar"}}},
		{"category", SearchInput{Category: []string{"ai"}}},
		{"employment_type", SearchInput{EmploymentType: []string{"freelance"}}},
		{"english_level", SearchInput{EnglishLevel: []string{"fluent"}}},
	}

	for _, tc := range cases {
		t.Run(tc.param, func(t *testing.T) {
			if got := tc.input.QueryValues().Get(tc.param); got != "" {
				t.Errorf("%s = %q, want dropped", tc.param, got)
			}
			if len(tc.input.Unsupported()) == 0 {
				t.Errorf("%s: nothing reported; the caller cannot tell the filter was ignored", tc.param)
			}
		})
	}
}

func TestVocabularyCheckingIsCaseInsensitive(t *testing.T) {
	// Measured against production on 2026-09-18: countries=de and countries=DE both answer
	// 59,942, so the search itself does not care about case. A check stricter than the
	// thing it guards would reject values that work.
	in := SearchInput{Seniority: []string{"Senior"}, EnglishLevel: []string{"B2"}}

	if got := in.Unsupported(); len(got) != 0 {
		t.Errorf("unsupported = %v, want none — the search accepts these", got)
	}
	if got := in.QueryValues().Get("seniority"); got != "Senior" {
		t.Errorf("seniority = %q, want the caller's spelling passed through", got)
	}
}

func TestAnythingThatIsNotACountryCodeIsNamed(t *testing.T) {
	// "germany" is the mistake a model makes when it has a country name and needs a code.
	// Left in, it filters on a value no posting carries and answers zero.
	in := SearchInput{Countries: []string{"DE", "germany"}}

	if got := in.QueryValues().Get("countries"); got != "DE" {
		t.Errorf("countries = %q, want only the code kept", got)
	}
	if got := in.Unsupported(); !slices.Contains(got, "countries=germany") {
		t.Errorf("unsupported = %v, want it to name countries=germany", got)
	}
}

func TestAnOpenVocabularyIsNotPolicedAtAll(t *testing.T) {
	// Skills, cities, company slugs and sources have no closed list to check against, and
	// inventing one here would be a second dictionary drifting from the real ones.
	in := SearchInput{Skills: []string{"brainfuck"}, Cities: []string{"atlantis"}}

	if got := in.Unsupported(); len(got) != 0 {
		t.Errorf("unsupported = %v, want none — these vocabularies are open", got)
	}
}

func TestACompanySearchAlsoNamesWhatItCouldNotHonour(t *testing.T) {
	// The same hole, in the tool nobody thought to check. A country NAME where a code belongs
	// filters on a value no company carries and answers zero, which the model reports as "no
	// such companies" — and a company search had no way to say otherwise until it did.
	in := CompanySearchInput{Query: "fintech", Countries: []string{"DE", "germany"}}

	if got := in.QueryValues().Get("countries"); got != "DE" {
		t.Errorf("countries = %q, want only the code kept", got)
	}
	if got := in.Unsupported(); !slices.Contains(got, "countries=germany") {
		t.Errorf("unsupported = %v, want it to name countries=germany", got)
	}
}

func TestEveryCheckedFacetReadsItsOwnField(t *testing.T) {
	// The one hazard the single table does not remove by construction: a row copied from the
	// one above it keeps the neighbour's closure, so two facets read one field. The compiler
	// is happy, and the symptom is a filter that is never checked — silently, which is the
	// failure the old map-plus-switch had and this table was written to end.
	//
	// So each row is given a value only IT should see, and must be the only one to notice.
	for _, facet := range checkedFacets {
		t.Run(facet.param, func(t *testing.T) {
			in := SearchInput{}
			switch facet.param {
			case "work_mode":
				in.WorkMode = []string{"probe"}
			case "seniority":
				in.Seniority = []string{"probe"}
			case "category":
				in.Category = []string{"probe"}
			case "employment_type":
				in.EmploymentType = []string{"probe"}
			case "english_level":
				in.EnglishLevel = []string{"probe"}
			default:
				t.Fatalf("%s is in the table but this test does not know how to set it", facet.param)
			}

			if got := in.Unsupported(); len(got) != 1 || got[0] != facet.param+"=probe" {
				t.Errorf("unsupported = %v, want exactly [%s=probe] — the row reads the wrong field",
					got, facet.param)
			}
		})
	}
}
