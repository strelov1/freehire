package search

import (
	"net/url"
	"slices"
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
	"experience_years_max",
	"posted_within_days",
	"open_within_days",
}

// qSearchableFields are the fields `q` matches against, in the same order as the
// jobs index's SearchableAttributes (see facetSettings in client.go). q_fields
// restricts `q` to a subset of this vocabulary.
var qSearchableFields = []string{"title", "company", "description", "location"}

// QFieldsFromValues resolves the q_fields query param into the
// AttributesToSearchOn list buildSearchRequest should pass to Meilisearch. The
// returned fields are always in qSearchableFields' canonical order regardless
// of how the caller wrote them, because Meilisearch's ranking is sensitive to
// attribute order and a caller-controlled order would make identical
// restrictions rank differently for no predictable reason.
//
// When q_fields is absent (or empty after dropping stray comma fragments —
// the same tolerance splitFacetValues gives every facet, so `q_fields=` and
// `q_fields=title,` behave like a bare `?skills=`), both return values are
// nil: no restriction, nothing to report. When it names a field outside
// qSearchableFields, the ENTIRE value is treated as invalid — even the names
// that were valid are dropped — and reported via the same UnknownParam shape
// UnknownParams uses. Partial application would silently narrow a caller's
// search past what a typo made them ask for, with no signal beyond the
// report; whole-value drop keeps the failure in the same class as an
// unrecognized facet: coarse, but honest.
func QFieldsFromValues(v url.Values) (fields []string, ignored []UnknownParam) {
	raw := v.Get("q_fields")
	if raw == "" {
		return nil, nil
	}
	requested := make(map[string]bool)
	for _, name := range strings.Split(raw, ",") {
		if name == "" {
			continue
		}
		if !slices.Contains(qSearchableFields, name) {
			return nil, []UnknownParam{{Param: "q_fields"}}
		}
		requested[name] = true
	}
	for _, field := range qSearchableFields {
		if requested[field] {
			fields = append(fields, field)
		}
	}
	return fields, nil
}

// maxUnknownParamsReported bounds how many ignored params one response echoes.
//
// The search endpoints are public and unauthenticated, and every reported param
// is copied from the request into the response body — so an unbounded report
// lets a hostile query turn junk params into response bytes. A real mistake is
// one param, or a handful; anything past this many is not someone to help.
const maxUnknownParamsReported = 10

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
	return unknownAgainst(v, knownParams(alsoKnown))
}

// ActiveFilterParams reports which of v's query params ARE part of the jobs filter
// vocabulary (every facet, its `_exclude`/`_mode` conventions, and the scalar filters) and
// were actually present in the request — the mirror image of UnknownParams, which reports
// the ones that are NOT. Backed by the same knownParams list as UnknownParams so the two
// can never drift into disagreeing about what the vocabulary is.
//
// It exists for the one case a filter param this package DOES recognize still could not be
// applied: Meilisearch rejecting the assembled filter during the deploy window a filterable
// attribute is declared in code before the live index catches up (see AGENTS.md's "Adding a
// filterable attribute"). The caller degrades by dropping the whole dynamic filter and
// reports every param this returns — Meilisearch's own rejection does not reliably name
// which single one it minded, so nothing here guesses at one. DidYouMean is never set: every
// entry is, by construction, already-recognized vocabulary, not a typo to correct.
func ActiveFilterParams(v url.Values) []UnknownParam {
	known := knownParams(nil)
	var out []UnknownParam
	for param := range v {
		if param != "" && known[param] {
			out = append(out, UnknownParam{Param: param})
		}
	}
	return SortAndCap(out)
}

// UnknownCompanyParams is UnknownParams for the company search, whose filter
// (CompanyFilterFromValues) reads a different vocabulary: its own facet list,
// and none of the `_exclude` / `_mode` conventions the jobs filter honours. A
// jobs facet sent here — `seniority=senior` on a company search — is therefore
// ignored, and reported.
func UnknownCompanyParams(v url.Values, alsoKnown []string) []UnknownParam {
	known := make(map[string]bool, len(companyFacets)+len(alsoKnown))
	for _, f := range companyFacets {
		known[f.param] = true
	}
	for _, param := range alsoKnown {
		known[param] = true
	}
	return unknownAgainst(v, known)
}

// UnknownParamsAgainst is the same report for an endpoint whose vocabulary this
// package does not own at all — the Talent Network catalogue is the first, and it
// filters on facets derived from a CV rather than on anything in the job index.
//
// The vocabulary is the caller's; only the report is shared. That split is the point:
// the ignored-params convention is a promise the whole API makes, and a second
// endpoint growing its own spelling of it — or its own suggestion logic — is how a
// convention stops being one. The caller must NOT be tempted to reach for
// UnknownParams above and pass its facets as alsoKnown: that would silently accept
// every job-search facet as legitimate on an endpoint that reads none of them.
func UnknownParamsAgainst(v url.Values, known []string) []UnknownParam {
	set := make(map[string]bool, len(known))
	for _, param := range known {
		set[param] = true
	}
	return unknownAgainst(v, set)
}

// unknownAgainst is the shared body of the reports: everything in v that the
// given vocabulary does not contain, named, suggested and bounded.
func unknownAgainst(v url.Values, known map[string]bool) []UnknownParam {
	var out []UnknownParam
	for param := range v {
		// "?=value" parses to an empty name. There is nothing to report about it
		// and nothing the caller could correct.
		if param == "" || known[param] {
			continue
		}
		out = append(out, UnknownParam{Param: param, DidYouMean: suggestParam(param, known)})
	}
	return SortAndCap(out)
}

// SortAndCap orders an ignored-param report by name and bounds it to
// maxUnknownParamsReported.
//
// Exported because a caller can have entries of its own to add: a param this
// package calls legitimate vocabulary but that endpoint discards anyway (the
// coverage endpoint's `skills`, which belong in the body). Those have to be
// folded into the same report before it is ordered and bounded — appended
// afterwards they would sit outside the sort and push the total past the cap
// the bound exists to enforce.
//
// Capping after sorting matters: it makes the report the same prefix every
// time, rather than whichever entries map iteration happened to reach first.
func SortAndCap(params []UnknownParam) []UnknownParam {
	sort.Slice(params, func(i, j int) bool { return params[i].Param < params[j].Param })
	if len(params) > maxUnknownParamsReported {
		params = params[:maxUnknownParamsReported]
	}
	return params
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

// facetModifiers are the suffixes a facet param can carry. A number mistake
// survives them (`country_exclude` for `countries_exclude`), so suggestParam has
// to pluralize the facet name inside the key rather than at its end.
var facetModifiers = []string{"_exclude", "_mode"}

// suggestParam returns the known param that differs from this one only in
// grammatical number, or "" when there is none. It deliberately does not do
// fuzzy matching: the observed mistake is reaching for `country` when the facet
// is `countries`, and an edit-distance guess close enough to catch that also
// invents suggestions for params that were never a typo at all.
func suggestParam(param string, known map[string]bool) string {
	name, modifier := param, ""
	for _, mod := range facetModifiers {
		if stem, ok := strings.CutSuffix(param, mod); ok {
			name, modifier = stem, mod
			break
		}
	}
	for _, candidate := range numberVariants(name) {
		if known[candidate+modifier] {
			return candidate + modifier
		}
	}
	return ""
}

// numberVariants returns the other grammatical numbers of a facet name.
func numberVariants(name string) []string {
	variants := []string{name + "s", name + "es"}
	if stem, ok := strings.CutSuffix(name, "y"); ok {
		variants = append(variants, stem+"ies")
	}
	if stem, ok := strings.CutSuffix(name, "s"); ok {
		variants = append(variants, stem)
	}
	return variants
}
