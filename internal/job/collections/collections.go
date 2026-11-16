// Package collections defines the fixed, code-owned set of curated company tags: facts about
// a company not derivable from a job's text or its ATS source. The registry here is the single
// source of truth for which tags exist and how their members are resolved;
// cmd/import-collections populates the membership, and the search facet (jobs.collections)
// serves it.
package collections

import (
	"context"
	_ "embed"
	"fmt"
	"net/http"
	"slices"
	"sort"

	"github.com/strelov1/freehire/internal/dict/normalize"
)

// Record is one member of a dataset: the company's name as the source publishes
// it, plus per-member attributes the source carries (a sponsorship route, a locality, a
// registry number). Meta is untyped on purpose: every register has its own columns and only
// that entry's own Gate reads them.
type Record struct {
	Name string
	Meta map[string]string
}

// Dataset is a source of member records for a collection, resolved by the import
// worker via its pure Parse (matching to our catalogue happens via normalize.CompanySlug in
// Collection.Members). The payload is either fetched from URL or supplied inline via Data (an
// embedded file, for a list that is our own curated fact) — exactly one is set. ResolveURL is
// set instead of URL by a source that republishes at a new address per snapshot, so the
// address is discovered at fetch time.
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

// Valid reports whether the dataset declares exactly one payload source. None leaves the
// collection unresolvable; two leave the winner ambiguous, so both are rejected (and by a
// registry test).
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

// names adapts a name-only parser to the record shape; only a register with per-member
// attributes writes a Record parser of its own.
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

// Kind separates the three sorts of company tag. An editorial collection is our own curated
// theme (Big Tech, Unicorns). A credential is a verifiable fact from an authoritative public
// register (a visa-sponsor licence), with an issuing body and snapshot date, presented
// differently because a user may act on it. A backer names the outside accelerator or fund
// that selected the company (Y Combinator, a16z) and renders as that brand's mark.
//
// The zero value is deliberately invalid: Kind decides the render group, so a forgotten one
// must fail the registry test rather than default silently.
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
	// Country is the ISO code of the country a credential's register covers. It lets
	// a register row naming a local subsidiary ("Google (UK) Limited", "SAP Danmark
	// A/S") match the parent company the catalogue knows ("google", "sap"): when the
	// row's own slug matches nothing, the same slug without a trailing mention of
	// THIS country is tried. Measured 2026-10-21, that recovered thousands of UK
	// sponsors and more than doubled Ireland's, most of them the large employers the
	// filter exists for. Empty for editorial collections, which strip nothing.
	Country string
	// Exclude lists catalogue company slugs a name match must never tag: a same-named
	// different employer that the source gives nothing to tell apart — no website, no
	// headcount (see mismatch). Each entry carries its reason beside it. A last resort
	// for a source that publishes names only, never a substitute for those checks.
	Exclude []string
}

// Company is the subset of a catalogue company a Gate may consult: its canonical
// slug, the countries it has open jobs in (job-derived, maintained by
// RefreshCompanyFacets), and its headquarters country. Deliberately narrow: a gate is a pure
// decision over already-loaded facts, with no database access.
type Company struct {
	Slug      string
	Countries []string
	HQCountry string
	// Website is the company's own site from its company_info, "" when unknown.
	Website string
	// OpenJobs is the company's open posting count (companies.job_count).
	OpenJobs int
}

// Admits reports whether this collection accepts a name-matched company/record
// pair. A collection with no Gate admits every match.
func (c Collection) Admits(company Company, r Record) bool {
	return c.Gate == nil || c.Gate(company, r)
}

// HandList reports whether this collection's membership is authored in this repo (a static
// Slugs list or an embedded file) rather than fetched from a third party. Only a hand list's
// unmatched entries are worth naming: they are ours to fix. Embedded files count, so a typo in
// one is visible.
func (c Collection) HandList() bool {
	return c.Slugs != nil || (c.Dataset != nil && len(c.Dataset.Data) > 0)
}

// All is the fixed registry, in display order. Adding a collection is one entry here, with a
// static Slugs list or a Dataset.
var All = []Collection{
	{
		Slug:        "yc",
		Title:       "Y Combinator",
		Description: "Open roles at Y Combinator–backed companies, from current batches to graduated unicorns.",
		Kind:        KindBacker,
		Dataset:     &Dataset{URL: ycDatasetURL, Parse: ParseYC},
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
		Dataset:     &Dataset{URL: a16zPortfolioURL, Parse: ParseA16zPortfolio},
	},
	{
		Slug:        "a16z-speedrun",
		Title:       "a16z Speedrun",
		Description: "Open roles at companies from the a16z Speedrun accelerator's cohorts.",
		Kind:        KindBacker,
		Dataset:     &Dataset{Records: speedrunMembers(speedrunTierAccelerator)},
	},
	{
		Slug:        "sequoia",
		Title:       "Sequoia",
		Description: "Open roles at companies in the Sequoia Capital portfolio.",
		Kind:        KindBacker,
		Dataset:     &Dataset{ResolveURL: resolveSequoiaIndex, Parse: ParseSequoia},
	},
	{
		Slug:        "accel",
		Title:       "Accel",
		Description: "Open roles at companies in the Accel portfolio.",
		Kind:        KindBacker,
		Dataset:     &Dataset{URL: accelDatasetURL, Parse: ParseAccel},
	},
	{
		Slug:        "greylock",
		Title:       "Greylock",
		Description: "Open roles at companies in the Greylock portfolio.",
		Kind:        KindBacker,
		Dataset:     &Dataset{URL: greylockDatasetURL, Parse: ParseGreylock},
	},
	{
		Slug:        "usv",
		Title:       "Union Square Ventures",
		Description: "Open roles at companies in the Union Square Ventures portfolio.",
		Kind:        KindBacker,
		Dataset:     &Dataset{URL: usvDatasetURL, Parse: ParseUSV},
	},
	{
		Slug:        "bond",
		Title:       "BOND",
		Description: "Open roles at companies in the BOND portfolio.",
		Kind:        KindBacker,
		Dataset:     &Dataset{URL: bondDatasetURL, Parse: ParseBond},
	},
	{
		Slug:        "lightspeed",
		Title:       "Lightspeed",
		Description: "Open roles at companies in the Lightspeed Venture Partners portfolio.",
		Kind:        KindBacker,
		Dataset:     &Dataset{URL: lightspeedPortfolioURL, Parse: ParseLightspeed},
		// clark: the catalogue's is Clark Construction, not Lightspeed's Clark.
		Exclude: []string{"clark"},
	},
	{
		Slug:        "index-ventures",
		Title:       "Index Ventures",
		Description: "Open roles at companies in the Index Ventures portfolio.",
		Kind:        KindBacker,
		Dataset:     &Dataset{URL: indexPortfolioURL, Parse: ParseIndex},
		// lever: the catalogue's "Lever" is aggregator postings filed under the ATS's
		// name, not the company Index backed.
		Exclude: []string{"lever"},
	},
	{
		Slug:        "softbank-vision-fund",
		Title:       "SoftBank Vision Fund",
		Description: "Open roles at companies in the SoftBank Vision Fund portfolio.",
		Kind:        KindBacker,
		Dataset:     &Dataset{URL: visionFundPortfolioURL, Parse: ParseVisionFund},
	},
	{
		Slug:        "founders-fund",
		Title:       "Founders Fund",
		Description: "Open roles at the companies Founders Fund names in its portfolio.",
		Kind:        KindBacker,
		Dataset:     &Dataset{URL: foundersFundPortfolioURL, Parse: ParseFoundersFund},
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
		Country:     "GB",
		Gate:        AllOf(RequireCountry("GB"), RequireRoute(ukWorkRoutes...)),
	},
	{
		Slug:        "nl-recognised-sponsor",
		Title:       "Licensed NL sponsor",
		Description: "Open roles at employers on the IND public register of recognised sponsors for work and highly skilled migrants. The recognition belongs to the employer — it is not a commitment to sponsor any particular role.",
		Kind:        KindCredential,
		Dataset:     &Dataset{URL: nlSponsorURL, Parse: ParseNLSponsors, IdentityKey: "kvk"},
		// No route gate: the IND register publishes recognition, not routes.
		Country: "NL",
		Gate:    RequireCountry("NL"),
	},
	{
		Slug:        "us-h1b-sponsor",
		Title:       "H-1B sponsor history",
		Description: "Open roles at employers with USCIS-approved H-1B petitions in the last five fiscal years. Historical approval belongs to the employer — it is not a commitment to sponsor any particular role.",
		Kind:        KindCredential,
		Dataset:     &Dataset{Records: FetchUSH1BSponsors, IdentityKey: "tin4"},
		// No route gate: the USCIS Data Hub is H-1B-specific already, unlike the UK
		// register's multi-route file.
		Country: "US",
		Gate:    RequireCountry("US"),
	},
	{
		Slug:        "ca-lmia-employer",
		Title:       "Canada LMIA employer",
		Description: "Open roles at employers ESDC issued a positive Labour Market Impact Assessment in the last four quarters, through the High Wage, Global Talent or Permanent Resident streams. The approval belongs to the employer — it is not a commitment to sponsor any particular role.",
		Kind:        KindCredential,
		Dataset:     &Dataset{Records: FetchCALMIASponsors},
		Country:     "CA",
		Gate:        AllOf(RequireCountry("CA"), RequireRoute(caWorkStreams...)),
	},
	{
		Slug:        "ie-permit-employer",
		Title:       "Ireland permit employer",
		Description: "Open roles at employers DETE issued Irish employment permits to this year or in the two years before. The permits belong to the employer — it is not a commitment to sponsor any particular role.",
		Kind:        KindCredential,
		Dataset:     &Dataset{Records: FetchIEPermitEmployers},
		Country:     "IE",
		Gate:        RequireCountry("IE"),
	},
	{
		Slug:        "dk-fast-track-company",
		Title:       "Denmark Fast-track company",
		Description: "Open roles at companies SIRI has certified for the Danish Fast-track scheme for hiring foreign skilled workers. The certification belongs to the employer — it is not a commitment to sponsor any particular role.",
		Kind:        KindCredential,
		Dataset:     &Dataset{URL: dkSponsorURL, Parse: ParseDKSponsors, IdentityKey: "cvr"},
		Country:     "DK",
		Gate:        RequireCountry("DK"),
	},
}

// Default dataset URLs (overridable per collection via <SLUG>_DATASET_URL in the import
// worker). The unicorn and fortune500 CSVs carry the name in a "Company" column (see
// ParseCompanyCSV).
const (
	ycDatasetURL         = "https://yc-oss.github.io/api/companies/all.json"
	unicornDatasetURL    = "https://raw.githubusercontent.com/elmoallistair/datasets/main/unicorn_startups.csv"
	fortune500DatasetURL = "https://raw.githubusercontent.com/EatMoreOranges/Fortune-500-Dataset/main/data/2023-fortune-500-data.csv"
	techstarsDatasetURL  = "https://raw.githubusercontent.com/ark-storzhv/techstars-parser/main/TechStars.csv"
	europeanDatasetURL   = "https://raw.githubusercontent.com/nickbiird/icp-radar/main/public/data/startups_processed.json"
)

// AICompanySlugs is a hand-curated list of prominent AI-native companies. "AI company" is a
// fact about the company (all its roles belong here), distinct from the job-level
// `enrichment.category = ml_ai` facet. Entries are canonical company slugs
// (normalize.CompanySlug); commonly variant names are all listed so the match lands whatever
// name our adapters use. Unmatched entries are logged.
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

// AINativeSlugs is the curated AI-native cohort sourced from the remotepilot.dev directory.
// Entries are canonical company slugs (normalize.CompanySlug) verified against the exact name
// our adapters ingest, e.g. "openrouter.ai" → openrouter-ai. Only that form is listed: a
// brand-name variant (openrouter) resolves to a duplicate company row and would double the
// cohort. It overlaps AICompanySlugs by design.
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

// Mag7Slugs is the Magnificent Seven mega-cap tech cohort. Name variants (alphabet/google,
// meta/facebook) are both listed. A deliberate subset of BigTechSlugs.
var Mag7Slugs = []string{
	"apple", "microsoft", "google", "alphabet",
	"amazon", "meta", "facebook", "nvidia", "tesla",
}

// BigTechSlugs is the hand-curated company-slug list for the bigtech collection.
// "Big Tech" is the broad "tech giants" sense: the Magnificent Seven plus the next tier of
// large, established public technology companies. It deliberately excludes high-growth
// startups/unicorns (Stripe, Airbnb, Uber, …), which surface in `yc` instead. Name variants
// are all listed. Entries are canonical company slugs (normalize.CompanySlug), matched against
// the catalogue at import time; unmatched entries are logged.
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
// internationally. It is our own curated fact, so it is embedded rather than fetched. One
// canonical company slug (normalize.CompanySlug) per line; matched against the catalogue at
// import time, unmatched slugs are logged by name.
//
//go:embed eastern_roots.txt
var easternRootsData []byte

// indianRootsData is the same shape for the indian-roots collection (IT-services majors, the
// domestic product ecosystem, and Indian-founder companies headquartered abroad). No
// maintained open dataset of these exists, so it is embedded too.
//
//go:embed indian_roots.txt
var indianRootsData []byte

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
// companies from a past run, e.g. after a rename. import-collections adds them to the managed
// set so Reconcile strips them on the next run. An entry is safe to drop once an import has
// cleared the tag.
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
	// Mismatch counts companies whose name matched a member the source describes as
	// a different organisation — another website (see sameSite) or a headcount the
	// company's own postings outgrow (see plausibleSize).
	Mismatch       int
	UnmatchedNames []string
	// MismatchNames shows what was refused and why, so a run's log lets a person
	// judge whether it refused the right companies.
	MismatchNames []string
}

// Members resolves this collection's candidate records to the company slugs that
// earn its tag, and reports how the candidates fell out. Matched slugs are
// deduplicated and sorted; unmatched names are returned verbatim, in input order,
// for logging.
//
// Both kinds match on normalize.CompanySlug, the catalogue's key. They differ in the
// ambiguity drop: a credential first discards rows whose name identifies more than one
// organisation.
//
// A record the source describes as a different organisation — another website, or a
// headcount the company's postings outgrow — is refused before the gate (see
// mismatch): the name matched, the organisation did not.
//
// A company is tagged when ANY of its records passes the gate: a register lists an
// organisation once per route, so a work-route row must win over an earlier temporary one.
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
	mismatched := make(map[string]struct{}, len(records))
	for _, r := range records {
		slug := normalize.CompanySlug(r.Name)
		company, known := companies[slug]
		if !known && c.Country != "" {
			if parent := withoutCountrySuffix(slug, c.Country); parent != "" {
				slug = parent
				company, known = companies[slug]
			}
		}
		if slug == "" || !known {
			stat.Unmatched++
			stat.UnmatchedNames = append(stat.UnmatchedNames, r.Name)
			continue
		}
		why := mismatch(r, company)
		if slices.Contains(c.Exclude, slug) {
			why = "excluded by hand"
		}
		if why != "" {
			if _, seen := mismatched[slug]; !seen && len(stat.MismatchNames) < MaxLoggedUnmatched {
				stat.MismatchNames = append(stat.MismatchNames, fmt.Sprintf("%s (%s)", r.Name, why))
			}
			mismatched[slug] = struct{}{}
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
	for slug := range mismatched {
		if _, ok := admitted[slug]; !ok {
			stat.Mismatch++
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
	Name     string `json:"name"`
	Website  string `json:"website"`
	Status   string `json:"status"`
	TeamSize int    `json:"team_size"`
}

// euStartup is the subset of an icp-radar dataset entry we consume (the company
// name lives in a capitalised "Name" field, unlike yc-oss's lowercase "name").
type euStartup struct {
	Name string `json:"Name"`
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
