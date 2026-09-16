package ojcp

import (
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/strelov1/freehire/internal/dict/vocab"
)

// Page bounds from the standard's own search-jobs input schema. The maximum is binding: a
// larger page would produce a response the standard's schema rejects.
const (
	defaultSearchLimit = 10
	maxSearchLimit     = 50
)

// SearchInput is OJCP's `search_jobs` input. Everything but Query is optional, and an
// absent block means the agent said nothing about it — never a zero value we should filter
// on.
type SearchInput struct {
	Query      string            `json:"query"`
	Location   *SearchLocation   `json:"location,omitempty"`
	Filters    *SearchFilters    `json:"filters,omitempty"`
	Pagination *SearchPagination `json:"pagination,omitempty"`
	// CandidateContext is accepted and IGNORED. This surface is anonymous and takes no
	// candidate PII; the field is declared so an agent sending it gets an answer rather
	// than a rejection, per the standard's own extensibility rule.
	CandidateContext any `json:"candidate_context,omitempty"`
}

// SearchLocation is where the agent wants the work to be.
type SearchLocation struct {
	City    string `json:"city,omitempty"`
	State   string `json:"state,omitempty"`
	Country string `json:"country,omitempty"`
	// RemoteOK is a POINTER because false and absent differ: false means "on-site is
	// acceptable too", which is not a filter at all.
	RemoteOK    *bool   `json:"remote_ok,omitempty"`
	RadiusMiles float64 `json:"radius_miles,omitempty"`
}

// SearchFilters is the standard's filter block.
type SearchFilters struct {
	EmploymentType   string  `json:"employment_type,omitempty"`
	SalaryMin        float64 `json:"salary_min,omitempty"`
	SalaryMax        float64 `json:"salary_max,omitempty"`
	ExperienceLevel  string  `json:"experience_level,omitempty"`
	PostedWithinDays int     `json:"posted_within_days,omitempty"`
}

// SearchPagination is the standard's page selector.
type SearchPagination struct {
	Limit  int `json:"limit,omitempty"`
	Offset int `json:"offset,omitempty"`
}

// seniorityFromStandard translates OJCP's experience levels into ours. `entry` maps to BOTH
// of the levels it covers rather than to one: an agent asking for entry-level work means
// the internships and the junior roles, and picking one would silently hide the other.
//
// `director` is absent because we hold no level that means it. Our `lead` is a team lead,
// not a director, and answering one for the other would be a wrong result rather than a
// missing filter — such a request is reported unsupported instead.
var seniorityFromStandard = map[string]string{
	"entry":     "intern,junior",
	"mid":       "middle",
	"senior":    "senior",
	"lead":      "lead",
	"executive": "c_level",
}

// QueryValues translates an OJCP search into this catalogue's own query vocabulary — the
// same url.Values the public search endpoints build their filter from — plus the names of
// the input fields it could not honour.
//
// Reporting the second is the house rule, not a nicety: an endpoint whose answer WIDENS
// because it did not understand a parameter has to say so. An agent that asked for jobs
// within 20 miles and silently received the whole catalogue has no way to tell.
//
// The PAGE is not in here. `FilterFromValues` does not read `limit` or `offset` — they are
// the endpoint's own parameters, not the filter's — so putting them in these values would
// only mean parsing them straight back out of a string. See Page.
func (in SearchInput) QueryValues() (url.Values, []string) {
	values := url.Values{}
	var unsupported []string

	if q := strings.TrimSpace(in.Query); q != "" {
		values.Set("q", q)
	}
	unsupported = append(unsupported, in.applyLocation(values)...)
	unsupported = append(unsupported, in.applyFilters(values)...)

	return values, unsupported
}

// Page is the window the agent asked for, held within what the standard's own input schema
// allows: honouring a larger limit would answer with a page that schema rejects.
func (in SearchInput) Page() (limit, offset int) {
	limit, offset = defaultSearchLimit, 0
	if in.Pagination == nil {
		return limit, offset
	}
	if in.Pagination.Limit > 0 {
		limit = min(in.Pagination.Limit, maxSearchLimit)
	}
	if in.Pagination.Offset > 0 {
		offset = in.Pagination.Offset
	}
	return limit, offset
}

func (in SearchInput) applyLocation(values url.Values) []string {
	if in.Location == nil {
		return nil
	}
	loc := in.Location

	var unsupported []string
	if loc.City != "" {
		values.Set("cities", loc.City)
	}
	if loc.Country != "" {
		values.Set("countries", loc.Country)
	}
	// We hold no state/province facet — the geography dictionary resolves cities and
	// countries — so a state-scoped search would be answered as if the state were not
	// stated.
	if loc.State != "" {
		unsupported = append(unsupported, "location.state")
	}
	// Distance search needs coordinates the catalogue does not carry.
	if loc.RadiusMiles != 0 {
		unsupported = append(unsupported, "location.radius_miles")
	}
	// Only TRUE is a filter. False says on-site is acceptable too, so translating it into
	// work_mode=onsite would drop every remote posting from an agent happy with either.
	if loc.RemoteOK != nil && *loc.RemoteOK {
		values.Set("work_mode", "remote")
	}
	return unsupported
}

func (in SearchInput) applyFilters(values url.Values) []string {
	if in.Filters == nil {
		return nil
	}
	f := in.Filters

	var unsupported []string
	if f.EmploymentType != "" {
		// Checked against our own vocabulary, not passed through. An unrecognised value
		// reaches the index as a filter nothing matches, so the answer NARROWS to nothing —
		// the opposite failure from a dropped filter, and the more confusing one: an agent
		// reads "no such jobs" where the truth is "I did not understand you".
		if slices.Contains(vocab.EmploymentTypeValues, f.EmploymentType) {
			values.Set("employment_type", f.EmploymentType)
		} else {
			unsupported = append(unsupported, "filters.employment_type")
		}
	}
	if f.SalaryMin != 0 {
		values.Set("salary_min", formatNumber(f.SalaryMin))
	}
	if f.SalaryMax != 0 {
		values.Set("salary_max", formatNumber(f.SalaryMax))
	}
	if f.PostedWithinDays != 0 {
		values.Set("posted_within_days", strconv.Itoa(f.PostedWithinDays))
	}
	if f.ExperienceLevel != "" {
		if ours, known := seniorityFromStandard[f.ExperienceLevel]; known {
			values.Set("seniority", ours)
		} else {
			unsupported = append(unsupported, "filters.experience_level")
		}
	}
	return unsupported
}

// formatNumber renders a salary bound the way our own query parser reads it: the schema
// types these as numbers, but a whole amount must not reach the filter as "120000.000000".
func formatNumber(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}
