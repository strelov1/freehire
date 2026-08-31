// Package collections defines the fixed, code-owned set of curated company tags —
// facts about a company that are not derivable from a job's text or its ATS
// source. A tag is one of two Kinds: an editorial theme we curate (Y Combinator,
// Big Tech) or a credential drawn from an authoritative public register. The
// registry here is the single source of truth for which tags exist and how their
// members are resolved; cmd/import-collections populates the membership, and the
// search facet (jobs.collections) serves it.
package collections

import (
	"context"
	_ "embed"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/strelov1/freehire/internal/dict/normalize"
)

// Record is one member of a dataset: the company's name as the source publishes
// it, plus whatever per-member attributes that source carries (a sponsorship
// route, a locality, a registry number). Meta is untyped on purpose — every
// register has its own columns, and only that entry's own Gate reads its own keys,
// so a typed union across sources would grow a field per source and be read by
// nobody else.
type Record struct {
	Name string
	Meta map[string]string
}

// Dataset is a source of member records for a collection, resolved by the import
// worker via its pure Parse (matching to our catalogue happens via normalize.Slug
// in Match). The payload is either fetched from URL (an external dataset we
// control) or supplied inline via Data (a file embedded in the binary, for a list
// that is our own curated fact rather than a third-party feed) — exactly one is set.
// ResolveURL is set instead of URL by a source that republishes at a new address
// per snapshot (the GOV.UK sponsor register embeds the snapshot date in its CSV
// path), so the address has to be discovered at fetch time rather than pinned.
type Dataset struct {
	URL        string
	Data       []byte
	ResolveURL func(context.Context, *http.Client) (string, error)
	// Records is set instead of the other three by a source no single payload can
	// express — a directory paginated across many responses, say. It fetches and
	// parses itself, so Parse is unused alongside it. It must read its source
	// completely and return an error otherwise: a partial read is indistinguishable
	// from a shrunken source, and would reconcile the tag off every company it
	// failed to reach.
	Records func(context.Context, *http.Client) ([]Record, error)
	Parse   func([]byte) ([]Record, error)
	// IdentityKey names the Record.Meta field that tells two same-named
	// organisations apart in this source — the UK register's town, the Dutch
	// register's KvK number. Read by the import worker's ambiguity guard (see
	// DropAmbiguous); empty when the source publishes no such field.
	IdentityKey string
}

// Valid reports whether the dataset declares exactly one payload source. Declaring
// none leaves the collection unresolvable; declaring two leaves which one wins to
// the reader of the import worker, so both are rejected here and by a registry test.
func (d Dataset) Valid() bool {
	n := 0
	if d.URL != "" {
		n++
	}
	if len(d.Data) > 0 {
		n++
	}
	if d.ResolveURL != nil {
		n++
	}
	if d.Records != nil {
		n++
	}
	return n == 1
}

// names adapts a name-only parser to the record shape. The editorial datasets
// publish nothing but a name, so their parsers stay as they are and are wrapped
// here; only a register that carries per-member attributes writes a Record parser
// of its own.
func names(parse func([]byte) ([]string, error)) func([]byte) ([]Record, error) {
	return func(data []byte) ([]Record, error) {
		parsed, err := parse(data)
		if err != nil {
			return nil, err
		}
		out := make([]Record, len(parsed))
		for i, n := range parsed {
			out[i] = Record{Name: n}
		}
		return out, nil
	}
}

// Kind separates the three sorts of company tag the registry carries. An editorial
// collection is our own curated theme (Big Tech, Unicorns) — a judgement call. A
// credential is a verifiable fact drawn from an authoritative public register (a
// visa-sponsor licence), which carries an issuing body and a snapshot date and is
// presented differently because a user may act on it. A backer names the outside
// accelerator or fund that selected the company (Y Combinator, a16z), which is why
// it renders as that brand's own mark rather than as another filter chip.
//
// The zero value is deliberately invalid: Kind decides which group a tag renders
// in, so a forgotten one must fail the registry test rather than default silently.
type Kind string

const (
	KindEditorial  Kind = "editorial"
	KindCredential Kind = "credential"
	KindBacker     Kind = "backer"
)

// Collection is one curated company tag: a URL slug, the display copy rendered on
// the /collections hub and the job-search facet, its Kind, and its membership
// source — exactly one of Slugs (a static hand list of canonical company slugs) or
// Dataset (a remote member list). The import worker resolves the source to a set of
// member companies.
type Collection struct {
	Slug        string
	Title       string
	Description string
	Kind        Kind
	Slugs       []string // static hand list (e.g. bigtech)
	Dataset     *Dataset // remote member dataset (e.g. yc, unicorn)
	// Gate, when set, must hold before a name-matched company is tagged. It is what
	// separates "this register lists a company by this name" from "this is that
	// company" — the register's own metadata and the company's geography decide.
	// A nil Gate admits every name match, which is what every editorial collection
	// wants and has always done.
	Gate func(Company, Record) bool
}

// Company is the subset of a catalogue company a Gate may consult: its canonical
// slug, the countries it has open jobs in (job-derived, maintained by
// RefreshCompanyFacets), and its headquarters country. Deliberately narrow — a gate
// is a pure decision over facts already loaded by the import worker, not a hook
// with database access.
type Company struct {
	Slug      string
	Countries []string
	HQCountry string
}

// Admits reports whether this collection accepts a name-matched company/record
// pair. A collection with no Gate admits every match.
func (c Collection) Admits(company Company, r Record) bool {
	return c.Gate == nil || c.Gate(company, r)
}

// All is the fixed registry, in display order. Adding a collection is one entry
// here — a static Slugs list or a Dataset; the import worker resolves whichever is
// set.
var All = []Collection{
	{
		Slug:        "yc",
		Title:       "Y Combinator",
		Description: "Open roles at Y Combinator–backed companies, from current batches to graduated unicorns.",
		Kind:        KindBacker,
		Dataset:     &Dataset{URL: ycDatasetURL, Parse: names(ParseYC)},
	},
	{
		Slug:        "techstars",
		Title:       "Techstars",
		Description: "Open roles at Techstars-backed companies.",
		Kind:        KindBacker,
		Dataset:     &Dataset{URL: techstarsDatasetURL, Parse: names(ParseTechstarsCSV)},
	},
	{
		Slug:        "a16z-portfolio",
		Title:       "a16z",
		Description: "Open roles at companies in the Andreessen Horowitz portfolio.",
		Kind:        KindBacker,
		Dataset:     &Dataset{Records: speedrunMembers(speedrunTierPortfolio)},
	},
	{
		Slug:        "a16z-speedrun",
		Title:       "a16z Speedrun",
		Description: "Open roles at companies from the a16z Speedrun accelerator's cohorts.",
		Kind:        KindBacker,
		Dataset:     &Dataset{Records: speedrunMembers(speedrunTierAccelerator)},
	},
	{
		Slug:        "european",
		Title:       "European Startups",
		Description: "Open roles at European startups across the continent's tech hubs.",
		Kind:        KindEditorial,
		Dataset:     &Dataset{URL: europeanDatasetURL, Parse: names(ParseEUStartups)},
	},
	{
		Slug:        "ai",
		Title:       "AI Companies",
		Description: "Open roles at AI-native companies — foundation-model labs, ML platforms and applied-AI products.",
		Kind:        KindEditorial,
		Slugs:       AICompanySlugs,
	},
	{
		Slug:        "mag7",
		Title:       "Magnificent Seven",
		Description: "Open roles at the Magnificent Seven — Apple, Microsoft, Alphabet, Amazon, Meta, Nvidia and Tesla.",
		Kind:        KindEditorial,
		Slugs:       Mag7Slugs,
	},
	{
		Slug:        "bigtech",
		Title:       "Big Tech",
		Description: "Open roles at the largest, most established technology companies.",
		Kind:        KindEditorial,
		Slugs:       BigTechSlugs,
	},
	{
		Slug:        "unicorn",
		Title:       "Unicorns",
		Description: "Open roles at unicorns — private companies valued at over $1 billion.",
		Kind:        KindEditorial,
		Dataset:     &Dataset{URL: unicornDatasetURL, Parse: names(ParseCompanyCSV)},
	},
	{
		Slug:        "fortune500",
		Title:       "Fortune 500",
		Description: "Open roles at Fortune 500 companies — the largest US corporations by revenue.",
		Kind:        KindEditorial,
		Dataset:     &Dataset{URL: fortune500DatasetURL, Parse: names(ParseCompanyCSV)},
	},
	{
		Slug:        "eastern-roots",
		Title:       "Eastern Roots",
		Description: "Open roles at globally distributed companies founded by Eastern European (incl. Russian-speaking) founders or with Eastern European engineering roots.",
		Kind:        KindEditorial,
		Dataset:     &Dataset{Data: easternRootsData, Parse: names(ParseSlugList)},
	},
	{
		Slug:        "indian-roots",
		Title:       "Indian Roots",
		Description: "Open roles at companies founded in India or by Indian founders — the IT-services majors, the domestic product ecosystem, and the globally headquartered companies their founders built abroad.",
		Kind:        KindEditorial,
		Dataset:     &Dataset{Data: indianRootsData, Parse: names(ParseSlugList)},
	},
	{
		Slug:        "ai-native",
		Title:       "AI-Native",
		Description: "Open roles at AI-native companies building AI-first products and infrastructure — model and inference APIs, vector databases, and agent/dev tooling.",
		Kind:        KindEditorial,
		Slugs:       AINativeSlugs,
	},
	{
		Slug:        "uk-skilled-worker-sponsor",
		Title:       "Licensed UK sponsor",
		Description: "Open roles at employers on the GOV.UK register of licensed sponsors for the Skilled Worker and related work routes. The licence belongs to the employer — it is not a commitment to sponsor any particular role.",
		Kind:        KindCredential,
		Dataset:     &Dataset{ResolveURL: ResolveUKSponsorCSV, Parse: ParseUKSponsors, IdentityKey: "town"},
		Gate:        AllOf(RequireCountry("GB"), RequireRoute(ukWorkRoutes...)),
	},
	{
		Slug:        "nl-recognised-sponsor",
		Title:       "Licensed NL sponsor",
		Description: "Open roles at employers on the IND public register of recognised sponsors for work and highly skilled migrants. The recognition belongs to the employer — it is not a commitment to sponsor any particular role.",
		Kind:        KindCredential,
		Dataset:     &Dataset{URL: nlSponsorURL, Parse: ParseNLSponsors, IdentityKey: "kvk"},
		// No route gate: the IND register publishes recognition, not routes.
		Gate: RequireCountry("NL"),
	},
	{
		Slug:        "us-h1b-sponsor",
		Title:       "H-1B sponsor history",
		Description: "Open roles at employers with USCIS-approved H-1B petitions in the last five fiscal years. Historical approval belongs to the employer — it is not a commitment to sponsor any particular role.",
		Kind:        KindCredential,
		Dataset:     &Dataset{Records: FetchUSH1BSponsors, IdentityKey: "tin4"},
		// No route gate: the USCIS Data Hub is H-1B-specific already, unlike the UK
		// register's multi-route file.
		Gate: RequireCountry("US"),
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

// indianRootsData is the same shape for the indian-roots collection: the IT-services
// majors, the domestic product ecosystem, and the Indian-founder companies
// headquartered abroad. Embedded rather than fetched for the same reason, and here
// there is no alternative — no maintained open dataset of Indian-founded companies
// exists, only forks of one stale Kaggle funding CSV.
//
//go:embed indian_roots.txt
var indianRootsData []byte

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

// MaxLoggedUnmatched bounds the unmatched entries a hand list reports by name, so a
// mis-edited list cannot flood a run's output.
const MaxLoggedUnmatched = 50

// MatchStat is how one collection's candidates fell out: how many companies earned
// the tag, how many candidates matched no company at all, how many matched a company
// but failed the gate, and how many register rows the ambiguity guard dropped.
// Gated and Ambiguous are zero for a gateless editorial collection — they are what a
// dry run is read for.
type MatchStat struct {
	Matched, Unmatched int
	Gated, Ambiguous   int
	UnmatchedNames     []string
}

// Members resolves this collection's candidate records to the company slugs that
// earn its tag, and reports how the candidates fell out. Matched slugs are
// deduplicated and sorted; unmatched names are returned verbatim, in input order,
// for logging.
//
// Two things differ by kind. An editorial collection matches on normalize.Slug —
// unchanged, because every existing collection reconciles through this path and a
// changed match rule would silently rewrite its membership. A credential matches on
// RegisterSlug, which strips the legal form a public register appends to a name, and
// first drops rows whose name identifies more than one organisation.
//
// A company is tagged when ANY of its records passes the gate: a register lists an
// organisation once per route it holds, so a work-route row must win even when a
// temporary-route row for the same body is listed first.
func (c Collection) Members(records []Record, companies map[string]Company) ([]string, MatchStat) {
	var stat MatchStat
	// Both kinds slug the same way, because both look the result up in a map keyed by the
	// CATALOGUE's company slug — and that is normalize.CompanySlug. Editorial matching used
	// normalize.Slug until the catalogue started stripping corporate forms; a dataset naming
	// "Acme Robotics Limited" then asked for a slug ingest can no longer produce, matched
	// nothing, and reported no error. What still separates a credential is the ambiguity drop
	// and the gates, not the spelling.
	if c.Kind == KindCredential {
		before := len(records)
		records = DropAmbiguous(records, c.identityKey())
		stat.Ambiguous = before - len(records)
	}

	admitted := make(map[string]struct{}, len(records))
	gatedOut := make(map[string]struct{}, len(records))
	for _, r := range records {
		slug := normalize.CompanySlug(r.Name)
		company, known := companies[slug]
		if slug == "" || !known {
			stat.Unmatched++
			stat.UnmatchedNames = append(stat.UnmatchedNames, r.Name)
			continue
		}
		if !c.Admits(company, r) {
			gatedOut[slug] = struct{}{}
			continue
		}
		admitted[slug] = struct{}{}
	}

	matched := make([]string, 0, len(admitted))
	for slug := range admitted {
		matched = append(matched, slug)
	}
	sort.Strings(matched)
	stat.Matched = len(matched)
	for slug := range gatedOut {
		// A company with both an admitted and a rejected row is not gated out.
		if _, ok := admitted[slug]; !ok {
			stat.Gated++
		}
	}
	return matched, stat
}

// identityKey is this collection's dataset organisation discriminator, if any.
func (c Collection) identityKey() string {
	if c.Dataset == nil {
		return ""
	}
	return c.Dataset.IdentityKey
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
	rows, err := csvColumns(data, delim, colName)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(rows))
	for _, row := range rows {
		if row[0] != "" {
			names = append(names, row[0])
		}
	}
	return names, nil
}

// csvColumns extracts the named columns from a CSV with the given delimiter,
// returning one trimmed slice per data row in the order the columns were asked for.
// Columns are located by header rather than by index, so an upstream reorder cannot
// silently read the wrong field, and a missing column is an error rather than a
// column of blanks.
func csvColumns(data []byte, delim rune, colNames ...string) ([][]string, error) {
	r := csv.NewReader(strings.NewReader(string(data)))
	r.Comma = delim
	r.FieldsPerRecord = -1 // tolerate ragged rows rather than aborting the whole parse
	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("collections: read csv header: %w", err)
	}

	idx := make([]int, len(colNames))
	for n, want := range colNames {
		idx[n] = -1
		for i, h := range header {
			if strings.EqualFold(strings.TrimSpace(h), want) {
				idx[n] = i
				break
			}
		}
		if idx[n] < 0 {
			return nil, fmt.Errorf("collections: csv has no %q column", want)
		}
	}

	rows, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("collections: read csv: %w", err)
	}
	out := make([][]string, 0, len(rows))
	for _, row := range rows {
		vals := make([]string, len(idx))
		for n, i := range idx {
			if i < len(row) {
				vals[n] = strings.TrimSpace(row[i])
			}
		}
		out = append(out, vals)
	}
	return out, nil
}

// easternRootsSlugs is the eastern-roots membership file parsed to its slugs, for the test
// that guards every hand list against the catalogue's slug rule. Exported to the package's
// tests only, because the file is embedded and there is no other way to read it back.
func easternRootsSlugs() []string {
	return embeddedSlugList(easternRootsData)
}

// indianRootsSlugs is the same read-back for the indian-roots membership file.
func indianRootsSlugs() []string {
	return embeddedSlugList(indianRootsData)
}

func embeddedSlugList(data []byte) []string {
	names, err := ParseSlugList(data)
	if err != nil {
		return nil
	}
	return names
}
