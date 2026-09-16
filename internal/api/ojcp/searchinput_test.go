package ojcp

import (
	"slices"
	"testing"
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
		"q":                  "go engineer",
		"cities":             "Berlin",
		"countries":          "DE",
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
	// The standard names six levels to our eight. `entry` covers two of ours, so it becomes
	// both rather than picking one — an agent asking for entry-level work should see the
	// internships and the junior roles, not half of them.
	for _, tc := range []struct{ theirs, want string }{
		{"entry", "intern,junior"},
		{"mid", "middle"},
		{"executive", "c_level"},
		{"senior", "senior"},
		{"lead", "lead"},
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
