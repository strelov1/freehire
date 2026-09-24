package ojcp

import (
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/strelov1/freehire/internal/dict/location"
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

// seniorityFromStandard translates OJCP's experience levels into ours.
//
// This map is the INBOUND half of a pair, and the two halves have to name the same set.
// jobPostingFrom publishes our seniority verbatim wherever the standard has no word for it
// (see experienceLevel in jobposting.go), so a posting of ours can say `intern`, `junior`,
// `staff` or `principal`. An agent that reads one of those off a posting and sends it back
// as a filter must not be told we do not understand it: the filter would be dropped and the
// answer would widen to the whole catalogue. Publishing a value we then refuse to filter on
// is a silent widening one step removed, so all four are accepted back here, and
// TestEverySenioritySurvivesTheRoundTrip holds that closed.
//
// `entry` is the one asymmetry, and it is deliberate: it is a standard word we never
// publish, so it is accepted inbound only, and it narrows to `junior` alone. It used to
// cover `intern` as well, on the reasoning that an agent asking for entry-level work wants
// the internships too. That reasoning held only while `intern` had no name of its own here.
// It has one now — an agent can ask for `intern` directly — so an internship and a junior
// role are two different things a candidate searches for: an agent that wants both asks
// twice, and one that wants junior roles WITHOUT internships can finally say so, which the
// old mapping made impossible.
//
// `intern` is OUR value, not a standard one. The standard's documented set is still
// entry/mid/senior/lead/director/executive (see testdata/schemas/tools/search-jobs-input.json);
// adding intern/staff/principal to it is only PROPOSED, in ojcp-org/ojcp#23. Nothing here
// waits on that: the field is an open string either way. When that PR lands, re-copy the
// vendored schemas per testdata/schemas/README.md rather than editing this comment.
//
// `director` is absent because we hold no level that means it. Our `lead` is a team lead,
// not a director, and answering one for the other would be a wrong result rather than a
// missing filter — such a request is reported unsupported instead.
var seniorityFromStandard = map[string]string{
	// The standard's words.
	"entry":     "junior",
	"mid":       "middle",
	"senior":    "senior",
	"lead":      "lead",
	"executive": "c_level",
	// Ours, published verbatim where the standard has no word.
	"intern":    "intern",
	"junior":    "junior",
	"staff":     "staff",
	"principal": "principal",
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
	// City is passed through UNCHECKED, unlike the country beside it. The city facet is a
	// high-cardinality open vocabulary (jobview folds a dictionary beacon list with the
	// model's own reading), so there is no closed set to check against — and refusing an
	// unlisted city would drop a real search. An unmatched city therefore narrows the answer,
	// which is the one case here that stays silent. `getJobFacets` is where an agent resolves
	// a city before filtering on it.
	if loc.City != "" {
		values.Set("cities", loc.City)
	}
	// Resolved rather than passed through, and the resolution is also the check: an agent may
	// send "US", "USA" or "United States", and a value the dictionary cannot place is one the
	// index will match nothing for — the answer NARROWS to nothing while looking like an
	// honest empty result.
	if loc.Country != "" {
		if country := location.NormalizeCountry(loc.Country); country != "" {
			values.Set("countries", country)
		} else {
			unsupported = append(unsupported, "location.country")
		}
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
