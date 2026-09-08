package talentnetwork

import (
	"math"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// The catalogue's query vocabulary. It lives here rather than in the handler because the
// filters are facts about a candidate, and the endpoint is one of possibly several
// readers — the approved-recruiter surface will want the same vocabulary with a wider
// projection behind it.

const (
	// maxFilterTerms bounds one filter list. A longer one is truncated rather than
	// refused: a URL is typed by hand and pasted between people, and refusing renders as
	// an error page for what is at worst an over-eager filter.
	maxFilterTerms = 25

	// defaultLimit is the page size when none is asked for, and the fallback when the
	// one asked for is unusable.
	defaultLimit = 24

	// maxLimit is the largest page the catalogue will build. It is a real bound, not a
	// clamp target: a request for more is reported as unread and served the default,
	// because silently clamping hands a caller a page they mistake for the whole result.
	maxLimit = 100
)

// Param names, in one place so KnownParams and QueryFromValues cannot drift. A param
// that reads but is not listed here would be reported as ignored while working, which
// misleads harder than saying nothing.
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

// KnownParams is every query param the catalogue reads, for the ignored-params report.
func KnownParams() []string {
	return []string{
		paramCategories, paramSeniorities, paramSkills, paramTimezoneRegions,
		paramCities, paramSpecializations, paramMinYears, paramLimit, paramOffset,
	}
}

// QueryFromValues reads a catalogue query out of URL parameters, and reports the params
// it recognised but could not READ — a `min_years=lots`, a `limit=1000`.
//
// Those go in the same report as a param nobody recognises, for the same reason: both
// WIDEN the answer. An endpoint whose answer widens when it does not understand its
// input has to say so, or a typo is indistinguishable from an unfiltered search.
//
// Unusable paging falls back to the default rather than failing the request. The filters
// are what a visitor came for; refusing the whole page over a hand-edited `limit` trades
// a working answer for an error page.
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

	q.MinYears, unread = boundedInt(v, paramMinYears, 0, math.MaxInt32, 0, unread)
	q.Limit, unread = boundedInt(v, paramLimit, 1, maxLimit, defaultLimit, unread)
	q.Offset, unread = boundedInt(v, paramOffset, 0, math.MaxInt32, 0, unread)

	sort.Strings(unread)
	return q, unread
}

// terms splits a comma-joined filter, trimming each term and dropping blanks, so an
// absent parameter and a present-but-empty one produce the same query. A trailing comma
// and stray spaces are what a UI joining chips actually emits.
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

// boundedInt reads one integer param. An absent one takes the default silently; a
// present one that is not an integer, or falls outside [min, max], takes the default AND
// is named in unread.
//
// strconv.Atoi over a prefix parse on purpose: "12abc" must not read as 12, or a
// parameter that is not a number at all would be accepted as one.
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
