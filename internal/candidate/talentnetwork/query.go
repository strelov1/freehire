package talentnetwork

import (
	"math"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// The catalogue's query vocabulary. It lives here, not in the handler, because the
// filters are facts about a candidate and a recruiter surface will want the same ones.

const (
	// maxFilterTerms bounds one filter list. A longer one is truncated, not refused: an
	// over-eager filter in a hand-edited URL should not become an error page.
	maxFilterTerms = 25

	// defaultLimit is the page size when none is asked for or the one asked for is unusable.
	defaultLimit = 24

	// maxLimit is the largest page. A request for more is reported as unread and served the
	// default, since silent clamping hands back a page mistaken for the whole result.
	maxLimit = 100

	// maxYears bounds min_years so an absurd value is reported in meta.ignored_params
	// instead of answered with a silently empty catalogue.
	maxYears = 60
)

// Param names, in one place so KnownParams and QueryFromValues cannot drift (a param read
// but not listed would be reported as ignored while working).
const (
	paramCategories      = "categories"
	paramSeniorities     = "seniorities"
	paramSkills          = "skills"
	paramTimezoneRegions = "tz"
	paramCities          = "cities"
	paramSpecializations = "specializations"
	paramMinYears        = "min_years"
	paramLimit           = "limit"
	paramOffset          = "offset"
)

// FacetParams is every param that selects a SET of dictionary values, the ones a filter
// pane offers and Facets counts (not min_years or paging).
//
// cmd/gen-contracts emits it as TALENT_FACET_PARAMS for the browser: a param the UI offers
// but the API does not read only shows up in meta.ignored_params and WIDENS the results
// beyond the visitor's chips, so the two ends are generated, not just tested.
func FacetParams() []string {
	return []string{
		paramCategories, paramSeniorities, paramSkills,
		paramTimezoneRegions, paramCities, paramSpecializations,
	}
}

// KnownParams is every query param the catalogue reads, for the ignored-params report:
// the facets, plus the threshold and the paging that are not facets.
func KnownParams() []string {
	return append(FacetParams(), paramMinYears, paramLimit, paramOffset)
}

// QueryFromValues reads a catalogue query out of URL parameters, and reports the params
// it recognised but could not READ (`min_years=lots`, `limit=1000`). They are reported
// like unknown params because both WIDEN the answer. Unusable paging falls back to the
// default rather than failing the request.
func QueryFromValues(v url.Values) (Query, []string) {
	var unread []string

	q := Query{
		Categories:      terms(v.Get(paramCategories)),
		Seniorities:     terms(v.Get(paramSeniorities)),
		Skills:          terms(v.Get(paramSkills)),
		TimezoneRegions: terms(v.Get(paramTimezoneRegions)),
		Cities:          terms(v.Get(paramCities)),
		Specializations: terms(v.Get(paramSpecializations)),
	}

	q.MinYears, unread = boundedInt(v, paramMinYears, 0, maxYears, 0, unread)
	q.Limit, unread = boundedInt(v, paramLimit, 1, maxLimit, defaultLimit, unread)
	q.Offset, unread = boundedInt(v, paramOffset, 0, math.MaxInt32, 0, unread)

	sort.Strings(unread)
	return q, unread
}

// terms splits a comma-joined filter, trimming terms and dropping blanks, so an absent
// parameter and an empty one give the same query.
func terms(raw string) []string {
	if raw == "" {
		return nil
	}
	var out []string
	for _, term := range strings.Split(raw, ",") {
		if t := strings.TrimSpace(term); t != "" {
			out = append(out, t)
		}
	}
	if len(out) == 0 {
		return nil
	}
	if len(out) > maxFilterTerms {
		out = out[:maxFilterTerms]
	}
	return out
}

// boundedInt reads one integer param. An absent one takes the default silently; one that
// is not an integer or is outside [min, max] takes the default AND is named in unread.
// strconv.Atoi on purpose: "12abc" must not read as 12.
func boundedInt(v url.Values, param string, min, max, fallback int, unread []string) (int, []string) {
	raw := v.Get(param)
	if raw == "" {
		return fallback, unread
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < min || n > max {
		return fallback, append(unread, param)
	}
	return n, unread
}
