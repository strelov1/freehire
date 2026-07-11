// Package collections defines the fixed, code-owned set of curated job
// collections — editorial themes about a company (e.g. Y Combinator, Big Tech)
// that are not derivable from a job's text or its ATS source. The registry here is
// the single source of truth for which collections exist and how their members are
// resolved; cmd/import-collections populates the membership, and the search facet
// (jobs.collections) serves it.
package collections

import (
	_ "embed"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/strelov1/freehire/internal/normalize"
)

// Dataset is a source of member company names for a collection, resolved by the
// import worker into a name list its pure Parse extracts (matching to our catalogue
// happens via normalize.Slug in Match). The payload is either fetched from URL (an
// external dataset we control) or supplied inline via Data (a file embedded in the
// binary, for a list that is our own curated fact rather than a third-party feed) —
// exactly one is set.
type Dataset struct {
	URL   string
	Data  []byte
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
		Slug:        "techstars",
		Title:       "Techstars",
		Description: "Open roles at Techstars-backed companies.",
		Dataset:     &Dataset{URL: techstarsDatasetURL, Parse: ParseTechstarsCSV},
	},
	{
		Slug:        "european",
		Title:       "European Startups",
		Description: "Open roles at European startups across the continent's tech hubs.",
		Dataset:     &Dataset{URL: europeanDatasetURL, Parse: ParseEUStartups},
	},
	{
		Slug:        "ai",
		Title:       "AI Companies",
		Description: "Open roles at AI-native companies — foundation-model labs, ML platforms and applied-AI products.",
		Slugs:       AICompanySlugs,
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
	{
		Slug:        "eastern-roots",
		Title:       "Eastern Roots",
		Description: "Open roles at globally distributed companies founded by Eastern European (incl. Russian-speaking) founders or with Eastern European engineering roots.",
		Dataset:     &Dataset{Data: easternRootsData, Parse: ParseSlugList},
	},
	{
		Slug:        "ai-native",
		Title:       "AI-Native",
		Description: "Open roles at AI-native companies building AI-first products and infrastructure — model and inference APIs, vector databases, and agent/dev tooling.",
		Slugs:       AINativeSlugs,
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
	techstarsDatasetURL  = "https://raw.githubusercontent.com/ark-storzhv/techstars-parser/main/TechStars.csv"
	europeanDatasetURL   = "https://raw.githubusercontent.com/nickbiird/icp-radar/main/public/data/startups_processed.json"
)

// AICompanySlugs is a hand-curated list of prominent AI-native companies —
// foundation-model labs, ML infrastructure/platforms, and applied-AI products.
// "AI company" is a fact about the company (so all of its roles belong here),
// distinct from the job-level `enrichment.category = ml_ai` facet (a single ML/AI
// role at any company). Entries are canonical company slugs (normalize.Slug);
// where a name is commonly written several ways, the variants are both listed so
// the match lands whatever name our adapters use. Unmatched entries are logged.
var AICompanySlugs = []string{
	// Foundation-model labs.
	"openai", "anthropic", "mistral", "mistral-ai", "cohere",
	"ai21-labs", "ai21", "xai", "stability-ai", "inflection-ai",
	"reka-ai", "contextual-ai", "deepmind",
	// ML infrastructure / platforms / tooling.
	"hugging-face", "scale-ai", "weights-biases", "together-ai", "replicate",
	"pinecone", "anyscale", "baseten", "fireworks-ai", "lambda-labs",
	"langchain", "llamaindex", "modal-labs",
	// Applied-AI products.
	"perplexity", "perplexity-ai", "character-ai", "midjourney", "runway",
	"runway-ml", "elevenlabs", "eleven-labs", "synthesia", "jasper",
	"glean", "harvey", "harvey-ai", "suno", "descript", "cresta",
	"anysphere", "cursor",
}

// AINativeSlugs is the curated AI-native cohort sourced from the remotepilot.dev
// directory — model/inference APIs, vector databases, and agent/dev tooling built
// around AI. Entries are canonical company slugs (normalize.Slug), each verified
// against the exact name our adapters ingest (the board-file company), e.g.
// "openrouter.ai" → openrouter-ai, "trmlabs" → trmlabs, "qdrant.tech" → qdrant-tech.
// Only the exact ingested form is listed: a brand-name variant (openrouter,
// qdrant, …) resolves to a separate duplicate company row and would double the
// cohort. It overlaps AICompanySlugs by design — a company may belong to both.
var AINativeSlugs = []string{
	// Model & inference APIs.
	"deepgram", "cohere", "together-ai", "fireworks-ai", "openrouter-ai",
	// Vector databases & data infrastructure.
	"pinecone", "qdrant-tech", "weaviate", "supabase", "planetscale", "tensorwave",
	// Agent & developer tooling.
	"langchain", "composio", "livekit", "linear", "resend",
	// Other AI-native companies.
	"trmlabs", "jasper", "concentrate-ai", "andromeda",
	"infinity-constellation", "vclusterlabs", "urun", "tollbit",
}

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

// easternRootsData is the embedded membership file for the eastern-roots collection:
// companies with Eastern European / Russian-speaking founding roots that operate
// internationally (a hand-curated seed plus the larger eastern-roots company list).
// "Eastern roots" is a fact about the company, so all of its roles belong here. It is
// our own curated fact, not a third-party feed, so it is committed to the repo and
// embedded rather than fetched. One canonical company slug (normalize.Slug) per line;
// the list is matched against the catalogue at import time and unmatched slugs are
// simply logged.
//
//go:embed eastern_roots.txt
var easternRootsData []byte

// ParseSlugList parses a newline-delimited slug list (the embedded russian-roots
// file): one entry per line, blank lines and #-comment lines skipped, surrounding
// whitespace trimmed. Entries are returned verbatim (Match normalizes them).
func ParseSlugList(data []byte) ([]string, error) {
	var out []string
	for _, line := range strings.Split(string(data), "\n") {
		s := strings.TrimSpace(line)
		if s == "" || strings.HasPrefix(s, "#") {
			continue
		}
		out = append(out, s)
	}
	return out, nil
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

// RetiredSlugs are collection slugs no longer in All but that may still be tagged on
// companies from a past run — e.g. after a rename (russian-roots → eastern-roots).
// import-collections adds them to the managed set so Reconcile strips them on the next
// run (they have no wanted members), a self-healing cleanup that needs no manual SQL.
// An entry is safe to drop once a production import has run and cleared the tag.
var RetiredSlugs = []string{"russian-roots"}

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

// euStartup is the subset of an icp-radar dataset entry we consume (the company
// name lives in a capitalised "Name" field, unlike yc-oss's lowercase "name").
type euStartup struct {
	Name string `json:"Name"`
}

// ParseEUStartups extracts the company names from the icp-radar European-startups
// dataset (a JSON array of company objects with a "Name" field).
func ParseEUStartups(data []byte) ([]string, error) {
	var raw []euStartup
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("collections: parse european dataset: %w", err)
	}
	names := make([]string, 0, len(raw))
	for _, c := range raw {
		if c.Name != "" {
			names = append(names, c.Name)
		}
	}
	return names, nil
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

// ParseCompanyCSV extracts company names from a comma-separated CSV with a
// "Company" column. Shared by the unicorn and fortune500 datasets.
func ParseCompanyCSV(data []byte) ([]string, error) {
	return parseCSVColumn(data, ',', "Company")
}

// ParseTechstarsCSV extracts company names from the Techstars portfolio CSV, which
// is semicolon-separated with the company in a "name" column.
func ParseTechstarsCSV(data []byte) ([]string, error) {
	return parseCSVColumn(data, ';', "name")
}

// parseCSVColumn extracts the named column from a CSV with the given delimiter: it
// locates the column by header (not a fixed index, so an upstream column reorder
// doesn't silently read the wrong field) and returns each non-empty value.
func parseCSVColumn(data []byte, delim rune, colName string) ([]string, error) {
	r := csv.NewReader(strings.NewReader(string(data)))
	r.Comma = delim
	r.FieldsPerRecord = -1 // tolerate ragged rows rather than aborting the whole parse
	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("collections: read csv header: %w", err)
	}
	col := -1
	for i, h := range header {
		if strings.EqualFold(strings.TrimSpace(h), colName) {
			col = i
			break
		}
	}
	if col < 0 {
		return nil, fmt.Errorf("collections: csv has no %q column", colName)
	}
	rows, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("collections: read csv: %w", err)
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
