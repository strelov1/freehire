package mcpapp

import (
	"reflect"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/dict/vocab"
)

// Every published filter, asserted one by one against the query parameter it must become.
//
// A table rather than a few spot checks because the failure this guards against is silent:
// a filter mapped to a key the search does not read is not an error anywhere — the
// parameter is simply absent, the answer comes back WIDER than asked, and the only symptom
// is a result set that looks generous. `country` for `countries` is the exact typo that has
// already passed for a real filter in this codebase.
func TestEveryPublishedFilterMapsToTheParameterSearchReads(t *testing.T) {
	yes := true
	cases := []struct {
		name  string
		input SearchInput
		param string
		want  string
	}{
		{"free text", SearchInput{Query: "go engineer"}, "q", "go engineer"},
		{"countries", SearchInput{Countries: []string{"DE", "BR"}}, "countries", "DE,BR"},
		{"cities", SearchInput{Cities: []string{"berlin"}}, "cities", "berlin"},
		{"work mode", SearchInput{WorkMode: []string{"remote", "hybrid"}}, "work_mode", "remote,hybrid"},
		{"seniority", SearchInput{Seniority: []string{"senior"}}, "seniority", "senior"},
		{"category", SearchInput{Category: []string{"backend"}}, "category", "backend"},
		{"skills", SearchInput{Skills: []string{"go"}}, "skills", "go"},
		{"employment type", SearchInput{EmploymentType: []string{"full_time"}}, "employment_type", "full_time"},
		{"salary floor", SearchInput{SalaryMin: 90000}, "salary_min", "90000"},
		{"salary currency", SearchInput{SalaryCurrency: "EUR"}, "salary_currency", "EUR"},
		{"visa sponsorship", SearchInput{VisaSponsorship: &yes}, "visa_sponsorship", "true"},
		{"english level", SearchInput{EnglishLevel: []string{"B2"}}, "english_level", "B2"},
		{"company", SearchInput{CompanySlugs: []string{"acme"}}, "company_slug", "acme"},
		{"source", SearchInput{Sources: []string{"greenhouse"}}, "source", "greenhouse"},
		{"posted within days", SearchInput{PostedWithinDays: 7}, "posted_within_days", "7"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.input.QueryValues().Get(tc.param); got != tc.want {
				t.Errorf("%s = %q, want %q", tc.param, got, tc.want)
			}
		})
	}
}

func TestAnUnsetFilterBecomesNoParameterAtAll(t *testing.T) {
	// The difference between "they did not say" and "they said none". A zero value written
	// out as a parameter is an unspoken preference turned into a silent narrowing.
	values := SearchInput{Query: "go"}.QueryValues()

	for _, param := range []string{"countries", "work_mode", "salary_min", "visa_sponsorship", "posted_within_days"} {
		if _, present := values[param]; present {
			t.Errorf("%s was written out although the caller never set it", param)
		}
	}
}

func TestVisaSponsorshipFalseIsAFilterAndAbsentIsNot(t *testing.T) {
	// The reason the field is a pointer: false means "sponsorship is not needed", which is
	// a real narrowing, while absent means nothing was said.
	no := false
	if got := (SearchInput{VisaSponsorship: &no}).QueryValues().Get("visa_sponsorship"); got != "false" {
		t.Errorf("visa_sponsorship = %q, want false", got)
	}
}

func TestThePageIsBoundedAndDefaulted(t *testing.T) {
	// A model asking for 500 results would spend the turn's context on a list nobody reads.
	if limit, _ := (SearchInput{Limit: 500}).Page(); limit != maxPageSize {
		t.Errorf("limit = %d, want it capped at %d", limit, maxPageSize)
	}
	if limit, _ := (SearchInput{}).Page(); limit != defaultPageSize {
		t.Errorf("limit = %d, want the default %d", limit, defaultPageSize)
	}
	if _, offset := (SearchInput{Offset: -3}).Page(); offset != 0 {
		t.Errorf("offset = %d, want a negative offset floored at 0", offset)
	}
}

func TestACompanySearchMapsItsOwnSmallVocabulary(t *testing.T) {
	values := CompanySearchInput{Query: "acme", Countries: []string{"DE"}}.QueryValues()

	if got := values.Get("q"); got != "acme" {
		t.Errorf("q = %q, want acme", got)
	}
	if got := values.Get("countries"); got != "DE" {
		t.Errorf("countries = %q, want DE", got)
	}
}

func TestACompanySearchSendsEveryCountryItWasGiven(t *testing.T) {
	// Measured against production on 2026-09-18: the company filter took `countries=DE,BR`
	// as ONE literal value and answered zero, while DE alone answered 16,586 and BR 3,897.
	// The search core now splits, so both codes must actually reach it.
	if got := (CompanySearchInput{Countries: []string{"DE", "BR"}}).QueryValues().Get("countries"); got != "DE,BR" {
		t.Errorf("countries = %q, want both codes", got)
	}
}

// TestSeniorityDescriptionNamesEveryLevel holds the hand-typed `jsonschema` description on
// SearchInput.Seniority against the real vocabulary.
//
// That tag is a DESCRIPTION, not an enum, so a level missing from it is still accepted by
// the code and simply invisible to the agent reading the tool schema. There is no error to
// see: the agent never asks for the level, gets no results it did not ask for, and nothing
// is logged. `c_level` sat unlisted that way — the app's MCP tool advertised seven of our
// eight levels, so an agent could not ask for executive roles through it at all.
//
// Walking vocab.SeniorityValues is the point, the same as internal/api/ojcp's guards: a
// test that retyped the list would only agree with the tag that retyped it.
func TestSeniorityDescriptionNamesEveryLevel(t *testing.T) {
	field, ok := reflect.TypeOf(SearchInput{}).FieldByName("Seniority")
	if !ok {
		t.Fatal("SearchInput has no Seniority field: this guard is pointed at the wrong name")
	}

	described := map[string]bool{}
	tag := field.Tag.Get("jsonschema")
	for _, token := range strings.Split(strings.TrimPrefix(tag, "any of "), ",") {
		described[strings.TrimSpace(token)] = true
	}

	for _, level := range vocab.SeniorityValues {
		if !described[level] {
			t.Errorf("seniority %q is missing from the jsonschema description %q: the code "+
				"still accepts it, but an agent reading the tool schema cannot know to ask "+
				"for it", level, tag)
		}
	}
}
