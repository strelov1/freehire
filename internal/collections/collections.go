// Package collections defines the fixed, code-owned set of curated job
// collections — editorial themes about a company (e.g. Y Combinator, Big Tech)
// that are not derivable from a job's text or its ATS source. The registry here is
// the single source of truth for which collections exist and how their members are
// resolved; cmd/import-collections populates the membership, and the search facet
// (jobs.collections) serves it.
package collections

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/strelov1/freehire/internal/normalize"
)

// Collection is one curated theme: a URL slug plus the display copy rendered on
// the /collections pages. Membership resolution is deterministic per collection
// (see the import worker) and not modelled here.
type Collection struct {
	Slug        string
	Title       string
	Description string
}

// All is the fixed v1 registry, in display order. Adding a collection is one
// entry here (plus a resolver in the import worker).
var All = []Collection{
	{
		Slug:        "yc",
		Title:       "Y Combinator",
		Description: "Open roles at Y Combinator–backed companies, from current batches to graduated unicorns.",
	},
	{
		Slug:        "bigtech",
		Title:       "Big Tech",
		Description: "Open roles at the largest, most established technology companies.",
	},
}

// BigTechSlugs is the hand-curated company-slug list for the bigtech collection.
// Entries are canonical company slugs (as produced by normalize.Slug), matched
// against the companies present in the catalogue at import time.
var BigTechSlugs = []string{
	"google",
	"alphabet",
	"meta",
	"amazon",
	"apple",
	"microsoft",
	"netflix",
	"nvidia",
	"oracle",
	"ibm",
	"salesforce",
	"adobe",
	"sap",
	"intel",
	"cisco",
	"qualcomm",
	"uber",
	"airbnb",
	"stripe",
	"paypal",
	"spotify",
	"snowflake",
}

// Lookup returns the registry entry for a slug, or ok=false when no collection has
// that slug.
func Lookup(slug string) (Collection, bool) {
	for _, c := range All {
		if c.Slug == slug {
			return c, true
		}
	}
	return Collection{}, false
}

// Slugs returns the registry's collection slugs — the set of tags the import
// worker manages on companies.
func Slugs() []string {
	out := make([]string, len(All))
	for i, c := range All {
		out[i] = c.Slug
	}
	return out
}

// Match maps each candidate (a company name or slug) to a canonical company slug
// via normalize.Slug and splits the candidates into those whose slug is present in
// `existing` and those whose original value is not. Matched slugs are deduplicated
// and sorted; unmatched values are returned verbatim (for logging) in input order.
// A candidate that normalizes to an empty slug is treated as unmatched.
func Match(candidates []string, existing map[string]struct{}) (matched, unmatched []string) {
	seen := make(map[string]struct{}, len(candidates))
	for _, c := range candidates {
		slug := normalize.Slug(c)
		if _, ok := existing[slug]; slug != "" && ok {
			if _, dup := seen[slug]; !dup {
				seen[slug] = struct{}{}
				matched = append(matched, slug)
			}
			continue
		}
		unmatched = append(unmatched, c)
	}
	sort.Strings(matched)
	return matched, unmatched
}

// Reconcile computes a company's new collection set: it removes every tag in
// `managed` from `current` (so a tag the company no longer qualifies for is
// dropped) and adds `want` (the managed tags it now qualifies for, a subset of
// `managed`), preserving any tag in `current` that the registry does not manage.
// The result is deduplicated, sorted, and always non-nil.
func Reconcile(current, managed, want []string) []string {
	isManaged := make(map[string]struct{}, len(managed))
	for _, m := range managed {
		isManaged[m] = struct{}{}
	}
	set := make(map[string]struct{}, len(current)+len(want))
	for _, c := range current {
		if _, m := isManaged[c]; !m {
			set[c] = struct{}{}
		}
	}
	for _, w := range want {
		set[w] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for s := range set {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// ycCompany is the subset of a yc-oss dataset entry we consume: the company name,
// which we match by normalized name against our companies.
type ycCompany struct {
	Name string `json:"name"`
}

// ParseYC extracts the company names from a yc-oss dataset payload (a JSON array of
// company objects). Only the name is read; matching to our catalogue happens via
// normalize.Slug in Match.
func ParseYC(data []byte) ([]string, error) {
	var raw []ycCompany
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("collections: parse yc dataset: %w", err)
	}
	names := make([]string, 0, len(raw))
	for _, c := range raw {
		if c.Name != "" {
			names = append(names, c.Name)
		}
	}
	return names, nil
}
