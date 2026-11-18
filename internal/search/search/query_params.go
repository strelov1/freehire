package search

import (
	"net/url"
	"slices"
	"sort"
	"strings"
)

// scalarFilters are the non-facet params filterFromValues reads: single typed values, none in
// StringFacets. UnknownParams uses the list to tell honoured filters from dropped ones; a test
// asserts each still narrows a query.
var scalarFilters = []string{
	"visa_sponsorship",
	"salary_min",
	"salary_max",
	"experience_years_min",
	"experience_years_max",
	"posted_within_days",
	"open_within_days",
	ApplyQuestionsMaxParam,
}

// WatchlistParam narrows a search to the caller's own watchlist (internal/engage/companywatch).
// It describes the CALLER, not the posting, so FilterFromValues does not read it: whoever runs
// the search resolves the company slugs and adds them (AndInStrings). Narrows counts it, which
// lets `watchlist=true` be saved as an alert.
const WatchlistParam = "watchlist"

// WantsWatchlist reports whether a parsed query asks for the caller's watchlist.
func WantsWatchlist(v url.Values) bool { return isTrue(v, WatchlistParam) }

// HasInvalidVisaSponsorship reports a nonempty visa scalar that the search drops. Alert
// admission and delivery share it so a legacy value cannot silently widen a subscription.
func HasInvalidVisaSponsorship(v url.Values) bool {
	raw := v.Get("visa_sponsorship")
	return raw != "" && !checkedScalars["visa_sponsorship"](raw)
}

// isTrue reads boolean modifiers with the same case and whitespace rules as their validator.
func isTrue(v url.Values, param string) bool {
	return strings.EqualFold(strings.TrimSpace(v.Get(param)), "true")
}

// ExperienceIncludeUnspecifiedParam widens the experience bounds to also admit postings that
// state no requirement. A modifier, not a filter: alone it reads nothing, so it stays off
// scalarFilters (whose test demands each entry narrow a query by itself).
const ExperienceIncludeUnspecifiedParam = "experience_include_unspecified"

// SeniorityIncludeUnspecifiedParam widens the included seniority levels to also admit postings
// with no level. Alone it reads nothing.
const SeniorityIncludeUnspecifiedParam = "seniority_include_unspecified"

// ExperienceOnlySeniorityUnspecifiedParam confines the experience bounds to postings with no
// seniority level, so a labelled posting passes them whatever years it states. With
// seniority_include_unspecified it reads "one of these levels, or no level and within the
// bound", which two ANDed facets cannot express. Alone it reads nothing.
const ExperienceOnlySeniorityUnspecifiedParam = "experience_only_seniority_unspecified"

// WorkModeRemoteAnywhereParam changes how work_mode composes with the location facets: instead
// of ANDing them (which narrows remote work to the home country), the work-mode group becomes
// "remote anywhere, OR a selected physical mode within the location group". See
// filterFromValues, where the swap happens. Alone it reads nothing.
const WorkModeRemoteAnywhereParam = "work_mode_remote_anywhere"

// WorkModeRemoteRegionsParam and WorkModeRemoteCountriesParam bound the remote arm of the
// remote-anywhere disjunction, keeping remote reach apart from home geography. They read like
// `regions`/`countries` (regions expand the same way), only while the modifier is in force.
const (
	WorkModeRemoteRegionsParam   = "work_mode_remote_regions"
	WorkModeRemoteCountriesParam = "work_mode_remote_countries"
)

// remoteReachFacets maps each remote-reach param to the location facet it reads as.
var remoteReachFacets = map[string]string{
	WorkModeRemoteRegionsParam:   "regions",
	WorkModeRemoteCountriesParam: "countries",
}

// scalarModifiers are params filterFromValues reads only to change how a filter applies; known
// to UnknownParams, never a filter alone.
var scalarModifiers = []string{
	ExperienceIncludeUnspecifiedParam,
	SeniorityIncludeUnspecifiedParam,
	ExperienceOnlySeniorityUnspecifiedParam,
	WorkModeRemoteAnywhereParam,
	WorkModeRemoteRegionsParam,
	WorkModeRemoteCountriesParam,
}

// qSearchableFields are the fields `q` matches, in the order of the jobs index's
// SearchableAttributes (facetSettings in client.go). q_fields restricts `q` to a subset.
var qSearchableFields = []string{"title", "company", "description", "location"}

// QFieldsFromValues resolves q_fields into the AttributesToSearchOn list for
// buildSearchRequest. Fields come back in qSearchableFields' canonical order whatever order the
// caller wrote, because Meilisearch ranking is sensitive to attribute order.
//
// It reads through splitFacetValues like every facet: repeated keys and comma-joined values
// resolve to the same set, and stray comma fragments are dropped.
//
// Absent or empty yields nil, nil (no restriction). A name outside qSearchableFields makes the
// ENTIRE value invalid, valid names included, reported as an UnknownParam: partial application
// would silently narrow the search past what a typo asked for.
func QFieldsFromValues(v url.Values) (fields []string, ignored []UnknownParam) {
	requested := make(map[string]bool)
	for _, name := range splitFacetValues(v["q_fields"]) {
		if !slices.Contains(qSearchableFields, name) {
			return nil, []UnknownParam{{Param: "q_fields"}}
		}
		requested[name] = true
	}
	if len(requested) == 0 {
		return nil, nil
	}
	for _, field := range qSearchableFields {
		if requested[field] {
			fields = append(fields, field)
		}
	}
	return fields, nil
}

// maxUnknownParamsReported bounds how many ignored params one response echoes. The search
// endpoints are public and every reported param is copied into the response body, so an
// unbounded report would let junk params turn into response bytes.
const maxUnknownParamsReported = 10

// UnknownParam is a query param no filter reads, reported so a silently dropped filter cannot
// pass for a real result. DidYouMean carries the vocabulary's name when the caller only got the
// grammatical number wrong (most facets are plural); empty when nothing close exists.
type UnknownParam struct {
	Param      string `json:"param"`
	DidYouMean string `json:"did_you_mean,omitempty"`
}

// UnknownParams reports the params of v that no filter in this package reads, sorted by name.
// alsoKnown declares the caller's own non-filter params (pagination, sort, format); this package
// owns the filter vocabulary only.
//
// It is the counterpart to FilterFromValues, which ignores unrecognized params so old saved
// searches and shared links keep working, but would leave a typo indistinguishable from an
// unfiltered query.
func UnknownParams(v url.Values, alsoKnown []string) []UnknownParam {
	return unknownAgainst(v, knownParams(alsoKnown))
}

// ActiveFilterParams reports which of v's params ARE in the jobs filter vocabulary (facets, their
// `_exclude`/`_mode` forms, scalar filters), the mirror of UnknownParams and backed by the same
// knownParams list.
//
// It serves the case where a recognized filter still cannot be applied: Meilisearch rejecting the
// filter while a filterable attribute is declared in code before the live index catches up (see
// AGENTS.md "Adding a filterable attribute"). The caller drops the whole dynamic filter and
// reports every param returned, since Meilisearch's rejection does not reliably name one.
// DidYouMean is never set.
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

// UnknownCompanyParams is UnknownParams for the company search, which reads its own facet list
// and none of the `_exclude`/`_mode` conventions, so a jobs facet sent there is reported.
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

// UnknownParamsAgainst is the same report for an endpoint whose vocabulary this package does not
// own (the Talent Network catalogue filters on CV-derived facets). The vocabulary is the caller's
// and only the report is shared, so the ignored-params convention keeps one spelling. Do not pass
// such facets as alsoKnown to UnknownParams: that would accept every job-search facet.
func UnknownParamsAgainst(v url.Values, known []string) []UnknownParam {
	set := make(map[string]bool, len(known))
	for _, param := range known {
		set[param] = true
	}
	return unknownAgainst(v, set)
}

// unknownAgainst is the shared body of the reports: everything in v outside the vocabulary,
// named, suggested and bounded.
func unknownAgainst(v url.Values, known map[string]bool) []UnknownParam {
	var out []UnknownParam
	for param := range v {
		// "?=value" parses to an empty name; nothing to report.
		if param == "" || known[param] {
			continue
		}
		out = append(out, UnknownParam{Param: param, DidYouMean: suggestParam(param, known)})
	}
	return SortAndCap(out)
}

// SortAndCap orders an ignored-param report by name and bounds it to maxUnknownParamsReported.
//
// Exported so a caller can fold in entries of its own (a param this package calls vocabulary but
// the endpoint discards, like the coverage endpoint's `skills`) before ordering and capping;
// appended afterwards they would escape the sort and the cap. Capping after sorting keeps the
// report the same prefix every time rather than whatever map iteration reached first.
func SortAndCap(params []UnknownParam) []UnknownParam {
	sort.Slice(params, func(i, j int) bool { return params[i].Param < params[j].Param })
	if len(params) > maxUnknownParamsReported {
		params = params[:maxUnknownParamsReported]
	}
	return params
}

// knownParams builds the set of every param the filter reads: each facet with its `_exclude` and
// `_mode` forms, the scalar filters, and the caller's own.
func knownParams(alsoKnown []string) map[string]bool {
	known := make(map[string]bool, len(StringFacets)*3+len(scalarFilters)+len(scalarModifiers)+len(alsoKnown))
	for param := range StringFacets {
		known[param] = true
		known[param+"_exclude"] = true
		known[param+"_mode"] = true
	}
	for _, param := range scalarFilters {
		known[param] = true
	}
	for _, param := range scalarModifiers {
		known[param] = true
	}
	for _, param := range alsoKnown {
		known[param] = true
	}
	return known
}

// facetModifiers are the suffixes a facet param can carry. suggestParam pluralizes the facet name
// inside the key, since a number mistake survives them (`country_exclude`).
var facetModifiers = []string{"_exclude", "_mode"}

// suggestParam returns the known param differing from this one only in grammatical number, or
// "". It avoids fuzzy matching: an edit-distance guess close enough to catch `country` for
// `countries` also invents suggestions for params that were never typos.
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
