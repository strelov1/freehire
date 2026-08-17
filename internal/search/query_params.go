package search

import (
	"net/url"
	"sort"
	"strings"
)

// scalarFilters are the non-facet query params filterFromValues reads: each is a
// single typed value rather than a facet's value set, so none appears in
// StringFacets. Listed here so UnknownParams can tell a filter this package
// honours from one it silently drops. A test asserts each still narrows a query,
// so the list cannot rot into a lie as filters come and go.
var scalarFilters = []string{
	"visa_sponsorship",
	"salary_min",
	"salary_max",
	"experience_years_min",
	"posted_within_days",
}

// UnknownParam is a query param no filter reads, reported back to the caller so
// a silently dropped filter cannot pass for a real result. DidYouMean carries the
// vocabulary's name for it when the caller only got the grammatical number wrong
// — the common miss, since most facets are plural (`countries`, not `country`).
// It is empty when nothing close enough exists; a guess would mislead more than
// silence.
type UnknownParam struct {
	Param      string `json:"param"`
	DidYouMean string `json:"did_you_mean,omitempty"`
}

// UnknownParams reports the query params of v that no filter in this package
// reads, sorted by name so the report is stable. alsoKnown lets the caller
// declare its own non-filter params (pagination, sort, response format) as
// legitimate — this package owns the filter vocabulary and nothing else.
//
// This is the counterpart to FilterFromValues' permissiveness: that function
// ignores whatever it does not recognize, which keeps old saved searches and
// shared links working, but leaves a typo indistinguishable from an unfiltered
// query. Reporting the difference is the whole point — a caller that asked for
// `country=it` and got the entire catalogue back needs to know why.
func UnknownParams(v url.Values, alsoKnown []string) []UnknownParam {
	known := knownParams(alsoKnown)

	var out []UnknownParam
	for param := range v {
		if known[param] {
			continue
		}
		out = append(out, UnknownParam{Param: param, DidYouMean: suggestParam(param, known)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Param < out[j].Param })
	return out
}

// knownParams builds the set of every param the filter reads: each facet plus
// its `_exclude` and `_mode` conventions, the scalar filters, and whatever the
// caller declared as its own.
func knownParams(alsoKnown []string) map[string]bool {
	known := make(map[string]bool, len(StringFacets)*3+len(scalarFilters)+len(alsoKnown))
	for param := range StringFacets {
		known[param] = true
		known[param+"_exclude"] = true
		known[param+"_mode"] = true
	}
	for _, param := range scalarFilters {
		known[param] = true
	}
	for _, param := range alsoKnown {
		known[param] = true
	}
	return known
}

// suggestParam returns the known param that differs from this one only in
// grammatical number, or "" when there is none. It deliberately does not do
// fuzzy matching: the observed mistake is reaching for `country` when the facet
// is `countries`, and an edit-distance guess close enough to catch that also
// invents suggestions for params that were never a typo at all.
func suggestParam(param string, known map[string]bool) string {
	candidates := []string{param + "s", param + "es"}
	if stem, ok := strings.CutSuffix(param, "y"); ok {
		candidates = append(candidates, stem+"ies")
	}
	if stem, ok := strings.CutSuffix(param, "s"); ok {
		candidates = append(candidates, stem)
	}
	for _, c := range candidates {
		if known[c] {
			return c
		}
	}
	return ""
}
