// Package facetsnapshot defines the fixed set of job facets the /open transparency
// page displays and maps a Meilisearch facet count into flat snapshot rows keyed by
// public facet param names. It is the single source of truth shared by the
// cmd/rollup-facets worker (which writes the rows) and is deliberately db-agnostic:
// it depends only on the search vocabulary, so the worker converts its Rows into the
// db insert params.
package facetsnapshot

import (
	"sort"

	"github.com/strelov1/freehire/internal/search"
)

// Facets are the public facet param names the snapshot covers — the four the /open
// page renders. Anything else in a facet count is dropped.
var Facets = []string{"countries", "skills", "seniority", "work_mode"}

// Row is one (facet, value, count) entry of the snapshot, keyed by public facet
// param name (e.g. "seniority", not the index attribute "enrichment.seniority").
type Row struct {
	Facet string
	Value string
	Count int64
}

// Attributes is the list of index attributes to request a facet count for — the
// index attribute behind each covered public param, resolved through the shared
// search vocabulary. Sorted for a deterministic request.
func Attributes() []string {
	attrs := make([]string, 0, len(Facets))
	for _, param := range Facets {
		attrs = append(attrs, search.StringFacets[param])
	}
	sort.Strings(attrs)
	return attrs
}

// Rows flattens a facet count into snapshot rows, keeping only the covered facets
// and re-keying each from its index attribute to its public param name. Uncovered
// attributes in the result are ignored.
func Rows(res search.FacetResult) []Row {
	paramByAttr := make(map[string]string, len(Facets))
	for _, param := range Facets {
		paramByAttr[search.StringFacets[param]] = param
	}

	rows := make([]Row, 0)
	for attr, dist := range res.Facets {
		param, ok := paramByAttr[attr]
		if !ok {
			continue
		}
		for value, count := range dist {
			rows = append(rows, Row{Facet: param, Value: value, Count: count})
		}
	}
	return rows
}
