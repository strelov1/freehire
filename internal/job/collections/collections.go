// Package collections defines the fixed, code-owned set of curated company tags: facts about
// a company not derivable from a job's text or its ATS source. The registry here is the single
// source of truth for which tags exist and how their members are resolved;
// cmd/import-collections populates the membership, and the search facet (jobs.collections)
// serves it.
package collections

import (
	"context"
	_ "embed"
	"net/http"
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
