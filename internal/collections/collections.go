// Package collections defines the fixed, code-owned set of curated job
// collections — editorial themes about a company (e.g. Y Combinator, Big Tech)
// that are not derivable from a job's text or its ATS source. The registry here is
// the single source of truth for which collections exist and how their members are
// resolved; cmd/import-collections populates the membership, and the search facet
// (jobs.collections) serves it.
package collections

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/strelov1/freehire/internal/normalize"
)

// Dataset is a remote source of member company names for a collection: a URL the
// import worker fetches and a pure parser that extracts the company names from the
// payload (matching to our catalogue happens via normalize.Slug in Match).
type Dataset struct {
	URL   string
	Parse func([]byte) ([]string, error)
}

// Collection is one curated theme: a URL slug, the display copy rendered on the
// /collections hub and the job-search facet, and its membership source — exactly
// one of Slugs (a static hand list of canonical company slugs) or Dataset (a
// remote list of company names). The import worker resolves the source to a set of
// member companies.
type Collection struct {
	Slug        string
	Title       string
	Description string
	Slugs       []string // static hand list (e.g. bigtech)
	Dataset     *Dataset // remote company-name dataset (e.g. yc, unicorn)
}

// All is the fixed registry, in display order. Adding a collection is one entry
// here — a static Slugs list or a Dataset; the import worker resolves whichever is
// set.
var All = []Collection{
	{
		Slug:        "yc",
		Title:       "Y Combinator",
		Description: "Open roles at Y Combinator–backed companies, from current batches to graduated unicorns.",
		Dataset:     &Dataset{URL: ycDatasetURL, Parse: ParseYC},
	},
	{
		Slug:        "mag7",
		Title:       "Magnificent Seven",
		Description: "Open roles at the Magnificent Seven — Apple, Microsoft, Alphabet, Amazon, Meta, Nvidia and Tesla.",
		Slugs:       Mag7Slugs,
	},
	{
		Slug:        "bigtech",
		Title:       "Big Tech",
		Description: "Open roles at the largest, most established technology companies.",
		Slugs:       BigTechSlugs,
	},
	{
		Slug:        "unicorn",
		Title:       "Unicorns",
		Description: "Open roles at unicorns — private companies valued at over $1 billion.",
		Dataset:     &Dataset{URL: unicornDatasetURL, Parse: ParseCompanyCSV},
	},
	{
		Slug:        "fortune500",
		Title:       "Fortune 500",
		Description: "Open roles at Fortune 500 companies — the largest US corporations by revenue.",
		Dataset:     &Dataset{URL: fortune500DatasetURL, Parse: ParseCompanyCSV},
	},
}

// Default dataset URLs (overridable per collection via <SLUG>_DATASET_URL in the
// import worker). yc-oss is the maintained open mirror of the YC company
// directory; the unicorn and fortune500 CSVs are open snapshots with the company
// name in a "Company" column (see ParseCompanyCSV).
const (
	ycDatasetURL         = "https://yc-oss.github.io/api/companies/all.json"
	unicornDatasetURL    = "https://raw.githubusercontent.com/elmoallistair/datasets/main/unicorn_startups.csv"
	fortune500DatasetURL = "https://raw.githubusercontent.com/EatMoreOranges/Fortune-500-Dataset/main/data/2023-fortune-500-data.csv"
)

// Mag7Slugs is the Magnificent Seven — the 2025 canonical mega-cap tech cohort.
// Name variants (alphabet/google, meta/facebook) are both listed so a company
// matches whichever name our adapters use. It is a deliberate subset of
// BigTechSlugs, surfaced as its own focused collection.
var Mag7Slugs = []string{
	"apple", "microsoft", "google", "alphabet",
	"amazon", "meta", "facebook", "nvidia", "tesla",
}

// BigTechSlugs is the hand-curated company-slug list for the bigtech collection.
// "Big Tech" is taken in the broad "tech giants" sense: the canonical Magnificent
// Seven (the 2025 standard — Apple, Microsoft, Alphabet/Google, Amazon, Meta,
// Nvidia, Tesla) plus the next tier of large, established public technology
// companies. It deliberately excludes high-growth startups/unicorns (Stripe,
// Airbnb, Uber, …) — those are not Big Tech and several are YC, so they surface in
// the `yc` collection instead. Name variants (alphabet/google, meta/facebook) are
// both listed so a company matches whichever name our adapters use.
// Entries are canonical company slugs (as produced by normalize.Slug), matched
// against the companies present in the catalogue at import time; unmatched entries
// are simply logged.
var BigTechSlugs = []string{
	// Magnificent Seven (+ name variants).
	"apple",
	"microsoft",
	"google",
	"alphabet",
	"amazon",
	"meta",
	"facebook",
	"nvidia",
	"tesla",
	// Established public tech giants.
	"netflix",
	"oracle",
	"salesforce",
	"ibm",
	"intel",
	"adobe",
	"cisco",
	"sap",
	"qualcomm",
	"broadcom",
	"amd",
	"dell",
	"servicenow",
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

// ParseCompanyCSV extracts the company names from a CSV that has a "Company"
// column: it locates that column by header (not a fixed index, so an upstream
// column reorder doesn't silently read the wrong field) and returns each non-empty
// value. Shared by the unicorn and fortune500 datasets.
func ParseCompanyCSV(data []byte) ([]string, error) {
	r := csv.NewReader(strings.NewReader(string(data)))
	r.FieldsPerRecord = -1 // tolerate ragged rows rather than aborting the whole parse
	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("collections: read company csv header: %w", err)
	}
	col := -1
	for i, h := range header {
		if strings.EqualFold(strings.TrimSpace(h), "Company") {
			col = i
			break
		}
	}
	if col < 0 {
		return nil, fmt.Errorf("collections: csv has no Company column")
	}
	rows, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("collections: read company csv: %w", err)
	}
	names := make([]string, 0, len(rows))
	for _, row := range rows {
		if col < len(row) {
			if name := strings.TrimSpace(row[col]); name != "" {
				names = append(names, name)
			}
		}
	}
	return names, nil
}
