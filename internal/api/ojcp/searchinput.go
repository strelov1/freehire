package ojcp

import (
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/strelov1/freehire/internal/dict/location"
	"github.com/strelov1/freehire/internal/dict/vocab"
)

// Page bounds from the standard's search-jobs input schema; a larger page would be rejected.
const (
	defaultSearchLimit = 10
	maxSearchLimit     = 50
)

// SearchInput is OJCP's `search_jobs` input. Everything but Query is optional; an absent
// block is never a zero value to filter on.
type SearchInput struct {
	Query      string            `json:"query"`
	Location   *SearchLocation   `json:"location,omitempty"`
	Filters    *SearchFilters    `json:"filters,omitempty"`
	Pagination *SearchPagination `json:"pagination,omitempty"`
	// CandidateContext is accepted and ignored: this surface is anonymous and takes no
	// candidate PII. Declared so an agent sending it is not rejected.
	//
	// The jsonschema tag is required: without it the SDK publishes the boolean schema `true`,
	// which strict MCP clients reject for the whole tool list.
	CandidateContext any `json:"candidate_context,omitempty" jsonschema:"accepted and ignored: this surface is anonymous and takes no candidate data"`
}

// SearchLocation is where the agent wants the work to be.
type SearchLocation struct {
	City    string `json:"city,omitempty"`
	State   string `json:"state,omitempty"`
	Country string `json:"country,omitempty"`
	// RemoteOK is a pointer because false ("on-site is acceptable too") is not a filter.
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
// It is the inbound half of experienceLevel (jobposting.go): our seniority is published
// verbatim where the standard has no word (`intern`, `junior`, `staff`, `principal`), so
// all of those must be accepted back or the filter is dropped and the answer widens.
// TestEverySenioritySurvivesTheRoundTrip holds that.
//
// `entry` is inbound-only (a standard word we never publish) and narrows to `junior` alone,
// so an agent can ask for internships and junior roles separately.
//
// `intern`, `staff` and `principal` are ours, not in the standard's documented set
// (testdata/schemas/tools/search-jobs-input.json); the field is an open string either way.
//
// `director` is absent: our `lead` is a team lead, and answering one for the other would be
// wrong, so it is reported unsupported.
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

// QueryValues translates an OJCP search into this catalogue's query vocabulary (the
// url.Values the public search endpoints build their filter from) plus the names of the
// input fields it could not honour, which the response must report.
//
// The page is not in here: `FilterFromValues` does not read `limit` or `offset`. See Page.
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

// Page is the window the agent asked for, capped at the schema's maximum.
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
	// City is passed through unchecked: the facet is an open vocabulary, so refusing an
	// unlisted city would drop a real search. An unmatched city narrows the answer silently;
	// `getJobFacets` is where an agent resolves one.
	if loc.City != "" {
		values.Set("cities", loc.City)
	}
	// Resolved, which doubles as the check: "US", "USA" and "United States" all work, and an
	// unplaceable value would narrow to an empty result that looks honest.
	if loc.Country != "" {
		if country := location.NormalizeCountry(loc.Country); country != "" {
			values.Set("countries", country)
		} else {
			unsupported = append(unsupported, "location.country")
		}
	}
	// No state/province facet exists, so the state would be silently ignored.
	if loc.State != "" {
		unsupported = append(unsupported, "location.state")
	}
	// Distance search needs coordinates the catalogue does not carry.
	if loc.RadiusMiles != 0 {
		unsupported = append(unsupported, "location.radius_miles")
	}
	// Only true is a filter; false must not become work_mode=onsite.
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
		// Checked, not passed through: an unrecognised value would narrow to nothing and read
		// as "no such jobs".
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

// formatNumber renders a salary bound the way our query parser reads it (no "120000.000000").
func formatNumber(v float64) string {
	return strconv.FormatFloat(v, 'f', -1, 64)
}
