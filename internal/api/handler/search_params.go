package handler

import (
	"net/url"
	"slices"

	"github.com/strelov1/freehire/internal/search/search"
)

// searchParams are the query params the search endpoints read themselves rather
// than hand to the filter: the query text, the sort directive and the pagination
// window. search.UnknownParams owns the filter vocabulary and nothing else, so
// each endpoint declares its own transport params beside itself (see
// facetsParams in facets.go, companiesParams in companies.go).
var searchParams = []string{"q", "sort", "order", "limit", "offset"}

// jobSearchParams is searchParams plus q_fields and q_prefix, the field-scoping and
// prefix-matching parameters only the two job-search endpoints (not the swipe deck,
// which shares searchParams but reads neither) understand.
var jobSearchParams = slices.Concat(searchParams, []string{"q_fields", qPrefixParam})

// qPrefixParam asks for q's last word to match as the start of a longer word, for a box
// suggesting results while someone is still typing. Off by default: see
// search.SearchParams.Prefix for what the prefix match costs.
const qPrefixParam = "q_prefix"

// publicJobSearchParams is what /jobs/search itself reads beyond the filter vocabulary.
var publicJobSearchParams = slices.Concat(jobSearchParams, []string{hideSeenParam, hideAppliedCompaniesParam, minSkillMatchParam})

// agentSearchParams is jobSearchParams plus the agent endpoint's own two: the
// response-format selector and the opt-in semantic search.
var agentSearchParams = slices.Concat(jobSearchParams, []string{"description_format", "semantic"})

// ignoredParams reports the query params of this request that neither the filter
// nor the endpoint itself reads. They are echoed in the response meta instead of
// being refused: rejecting them would break saved searches and shared links that
// still carry retired params, while staying silent is what let a mistyped
// `country=it` pass for a search of Italy.
func ignoredParams(vals url.Values, own []string) []search.UnknownParam {
	return search.UnknownParams(vals, own)
}

// mergedJobSearchIgnored combines runJobSearch's value-level ignored-param report
// (q_fields naming a field outside the searchable vocabulary, filter params the
// Meilisearch-rejection degrade dropped) with the name-level ignoredParams(vals, own)
// report, into one sorted, capped list. Shared so both job-search endpoints apply it
// identically.
func mergedJobSearchIgnored(vals url.Values, own []string, fromSearch []search.UnknownParam) []search.UnknownParam {
	return search.SortAndCap(append(fromSearch, ignoredParams(vals, own)...))
}

// searchSortable is the allowlist of sort params mapped to their index attribute;
// anything else is ignored so a bad param cannot make Meilisearch reject the query.
var searchSortable = map[string]string{
	"created_at": "created_at",
	"posted_at":  "posted_at",
	// The "Most viewed" ordering. This entry must NEVER ship ahead of the live index declaring
	// view_count sortable (see search.facetSettings and "Adding a sortable attribute" in
	// internal/search/search/AGENTS.md): Meilisearch rejects the whole query for an undeclared
	// sort attribute and that package maps it to a 500.
	"view_count": "view_count",
	"salary_min": "enrichment.salary_min",
	"salary_max": "enrichment.salary_max",
}

// sortMatch is the profile-match sort. Unlike every value in searchSortable it names
// no index attribute: it ranks by cosine against the caller's own skill vector, so it
// is resolved from the request rather than looked up in that table.
const sortMatch = "match"

// defaultSort is the order when the caller names none: an empty query has no relevance to
// rank by, so the freshest first; a text query keeps relevance order (nil).
func defaultSort(query string) []string {
	if query == "" {
		return []string{"posted_at:desc"}
	}
	return nil
}

// searchSortValues builds the Meilisearch sort directive from ?sort=<field>&order=<dir>.
// Without a valid sort, a no-text browse sorts freshest first (posted_at desc) and a text query keeps relevance order (nil).
//
// ranked reports that the request carries a match vector: an explicit sort directive would take precedence over vector ranking and silently discard the match order.
func searchSortValues(vals url.Values, ranked bool) []string {
	if ranked {
		return nil
	}
	attr, ok := searchSortable[vals.Get("sort")]
	if !ok {
		return defaultSort(vals.Get("q"))
	}
	order := vals.Get("order")
	if order != "asc" && order != "desc" {
		order = "desc"
	}
	return []string{attr + ":" + order}
}
