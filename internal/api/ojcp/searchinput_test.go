package ojcp

import (
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/dict/vocab"
)

func TestSearchInputBecomesOurOwnQueryVocabulary(t *testing.T) {
	remote := true
	input := SearchInput{
		Query:    "go engineer",
		Location: &SearchLocation{City: "Berlin", Country: "DE", RemoteOK: &remote},
		Filters: &SearchFilters{
			EmploymentType:   "full_time",
			SalaryMin:        120000,
			ExperienceLevel:  "senior",
			PostedWithinDays: 7,
		},
		Pagination: &SearchPagination{Limit: 20, Offset: 40},
	}

	values, _ := input.QueryValues()

	for key, want := range map[string]string{
		"q":      "go engineer",
		"cities": "Berlin",
		// Lowercase: the facet is stored that way, and NormalizeCountry canonicalises to it.
		"countries":          "de",
		"work_mode":          "remote",
		"employment_type":    "full_time",
		"salary_min":         "120000",
		"seniority":          "senior",
		"posted_within_days": "7",
	} {
		if got := values.Get(key); got != want {
			t.Errorf("%s = %q, want %q", key, got, want)
		}
	}

	// The page is NOT among the values: the filter does not read `limit` or `offset`, so
	// putting them there would only mean parsing them straight back out of a string.
	if limit, offset := input.Page(); limit != 20 || offset != 40 {
		t.Errorf("page = %d/%d, want 20/40", limit, offset)
	}
}

func TestSearchInputTranslatesTheStandardsSeniorityWords(t *testing.T) {
	// Two groups here. The first is the standard's own words. `entry` now stops at `junior`:
	// it used to sweep in the internships too, which was the right answer only while this map
	// gave `intern` no key of its own, so nobody could ask for one — an agent that wants both
	// asks twice, and one that wants junior roles without internships can finally ask at all.
	//
	// The second is the levels the standard has no word for, which we publish under our own
	// names. Those have to be accepted back or an agent reading one off our own posting is
	// told we do not understand it.
	for _, tc := range []struct{ theirs, want string }{
		{"entry", "junior"},
		{"mid", "middle"},
		{"executive", "c_level"},
		{"senior", "senior"},
		{"lead", "lead"},

		{"intern", "intern"},
		{"junior", "junior"},
		{"staff", "staff"},
		{"principal", "principal"},
	} {
		t.Run(tc.theirs, func(t *testing.T) {
			input := SearchInput{Filters: &SearchFilters{ExperienceLevel: tc.theirs}}

			values, unsupported := input.QueryValues()

			if got := values.Get("seniority"); got != tc.want {
				t.Errorf("seniority = %q, want %q", got, tc.want)
			}
			if len(unsupported) != 0 {
				t.Errorf("unsupported = %v, want none for a level we can express", unsupported)
			}
		})
	}
}

func TestSearchInputReportsAFilterItCannotHonour(t *testing.T) {
	// The house rule: an endpoint whose answer WIDENS because it did not understand a
	// parameter must say so. Silently ignoring radius_miles hands an agent the whole
	// catalogue where it asked for jobs within 20 miles, and nothing in the answer admits it.
	input := SearchInput{
		Location: &SearchLocation{State: "Bavaria", RadiusMiles: 20},
		Filters:  &SearchFilters{ExperienceLevel: "director"},
	}

	values, unsupported := input.QueryValues()

	for _, want := range []string{"location.state", "location.radius_miles", "filters.experience_level"} {
		if !slices.Contains(unsupported, want) {
			t.Errorf("unsupported = %v, want it to name %q", unsupported, want)
		}
	}
	if values.Get("seniority") != "" {
		t.Errorf("seniority = %q, want no filter applied for a level we cannot express", values.Get("seniority"))
	}
}

func TestSearchInputReportsAValueItsVocabularyDoesNotHold(t *testing.T) {
	// The mirror of a dropped filter, and the more confusing failure of the two: an
	// unrecognised value passed through reaches the index as a filter nothing matches, so the
	// agent reads "no such jobs" where the truth is "I did not understand you".
	input := SearchInput{Filters: &SearchFilters{EmploymentType: "gig_economy_hustle"}}

	values, unsupported := input.QueryValues()

	if got := values.Get("employment_type"); got != "" {
		t.Errorf("employment_type = %q, want no filter applied for a value we do not hold", got)
	}
	if !slices.Contains(unsupported, "filters.employment_type") {
		t.Errorf("unsupported = %v, want it to name the value it could not use", unsupported)
	}
}

func TestSearchInputPassesEveryEmploymentTypeItDoesHold(t *testing.T) {
	// Walks the real vocabulary rather than a list written here: a type added to `vocab` and
	// not reachable through this surface would otherwise be reported unsupported forever,
	// with every hand-written case still green.
	for _, value := range vocab.EmploymentTypeValues {
		t.Run(value, func(t *testing.T) {
			values, unsupported := SearchInput{Filters: &SearchFilters{EmploymentType: value}}.QueryValues()

			if values.Get("employment_type") != value {
				t.Errorf("employment_type = %q, want %q", values.Get("employment_type"), value)
			}
			if len(unsupported) != 0 {
				t.Errorf("unsupported = %v, want none for a type we hold", unsupported)
			}
		})
	}
}

func TestSearchInputResolvesHoweverAnAgentSpellsACountry(t *testing.T) {
	// The agent picks the spelling, not us: an OJCP client may send an alpha-2, an alpha-3 or
	// the country's name. Resolution doubles as the check.
	for _, spelling := range []string{"US", "us", "USA", "United States"} {
		t.Run(spelling, func(t *testing.T) {
			values, unsupported := SearchInput{Location: &SearchLocation{Country: spelling}}.QueryValues()

			if got := values.Get("countries"); got != "us" {
				t.Errorf("countries = %q, want the canonical code", got)
			}
			if len(unsupported) != 0 {
				t.Errorf("unsupported = %v, want none for a country we can place", unsupported)
			}
		})
	}
}

func TestSearchInputReportsACountryItCannotPlace(t *testing.T) {
	// Passed through, it would reach the index as a filter matching nothing — an answer that
	// narrowed to zero while reading as an honest empty result.
	values, unsupported := SearchInput{Location: &SearchLocation{Country: "Atlantis"}}.QueryValues()

	if got := values.Get("countries"); got != "" {
		t.Errorf("countries = %q, want no filter applied", got)
	}
	if !slices.Contains(unsupported, "location.country") {
		t.Errorf("unsupported = %v, want it to name the country it could not place", unsupported)
	}
}

func TestSearchInputHoldsThePageWithinWhatTheStandardAllows(t *testing.T) {
	// pagination.limit has a maximum of 50 in the input schema. Honouring a larger number
	// would answer with a page the standard's own schema rejects.
	input := SearchInput{Pagination: &SearchPagination{Limit: 500}}

	if limit, _ := input.Page(); limit != maxSearchLimit {
		t.Errorf("limit = %d, want it capped at the schema's maximum", limit)
	}
}

func TestSearchInputAppliesTheStandardsDefaultPage(t *testing.T) {
	input := SearchInput{Query: "go"}

	if limit, offset := input.Page(); limit != defaultSearchLimit || offset != 0 {
		t.Errorf("page = %d/%d, want the schema's default", limit, offset)
	}
}

func TestSearchInputNeverAsksForOnsiteWhenRemoteIsNotRequired(t *testing.T) {
	// remote_ok=false means "on-site is acceptable too", not "on-site only". Translating it
	// into work_mode=onsite would drop every remote posting from an agent that was happy
	// with either.
	notRequired := false
	input := SearchInput{Location: &SearchLocation{RemoteOK: &notRequired}}

	values, unsupported := input.QueryValues()

	if got := values.Get("work_mode"); got != "" {
		t.Errorf("work_mode = %q, want no work-mode filter", got)
	}
	if len(unsupported) != 0 {
		t.Errorf("unsupported = %v, want none — the field was understood", unsupported)
	}
}
