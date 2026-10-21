package mcpapp

// The wire shapes the tools answer in.
//
// They are FLAT on purpose. jobview.Job nests the LLM's output under `enrichment`, which is
// right for a JSON client reading one document and wrong for a model reading ten results in
// a chat turn: every nesting level is a place the salary can be missed. So the handful of
// enrichment fields a person actually asks about are lifted to the top, and the rest of
// that object is not published here at all.
//
// The `jsonschema` tags are not documentation. The SDK derives each tool's output schema
// from these types, and that schema is what ChatGPT reads to decide whether a field answers
// the question it was asked.

// JobSummary is one posting as a search result: enough to choose from, never the full body.
type JobSummary struct {
	Slug    string `json:"slug" jsonschema:"the posting's identifier"`
	Title   string `json:"title"`
	Company string `json:"company"`
	// CompanySlug travels beside the display name because it is what get_company takes.
	// Resolving a name back to a slug is a guess; carrying it is not.
	CompanySlug string `json:"company_slug,omitempty" jsonschema:"the employer's identifier"`
	Location    string `json:"location,omitempty"`
	// Countries is the faceted geography, which Location's free text is not. A posting
	// reading "EMEA" carries the country list the dictionaries resolved.
	Countries []string `json:"countries,omitempty" jsonschema:"ISO 3166-1 alpha-2 codes the role is open in"`
	WorkMode  string   `json:"work_mode,omitempty" jsonschema:"remote, hybrid or onsite"`
	// Seniority is OURS, not a standard's. Values outside the common four (intern, staff,
	// principal) are published as they are — 18.1% of open postings carry one, and
	// flattening them into a smaller vocabulary is what the OJCP surface has to do.
	Seniority      string   `json:"seniority,omitempty"`
	Category       string   `json:"category,omitempty"`
	EmploymentType string   `json:"employment_type,omitempty"`
	Skills         []string `json:"skills,omitempty"`
	// Salary fields are pointers: a posting that states no salary must not read as one
	// offering zero.
	SalaryMin       *int   `json:"salary_min,omitempty"`
	SalaryMax       *int   `json:"salary_max,omitempty"`
	SalaryCurrency  string `json:"salary_currency,omitempty" jsonschema:"ISO 4217 code"`
	SalaryPeriod    string `json:"salary_period,omitempty" jsonschema:"year, month, day or hour"`
	VisaSponsorship *bool  `json:"visa_sponsorship,omitempty"`
	EnglishLevel    string `json:"english_level,omitempty"`
	PostedAt        string `json:"posted_at,omitempty" jsonschema:"RFC3339 UTC"`
	// FirstSeenAt stands in for the index's reality class, which is frozen at push time and
	// goes stale; this cannot. get_job computes the live verdict (see AGENTS.md).
	FirstSeenAt string `json:"first_seen_at,omitempty" jsonschema:"RFC3339 UTC; when freehire first saw this posting — a posted_at far newer than this means the date was refreshed"`
	// Summary is the search index's truncated preview, never the full body. Ten full job
	// descriptions in one turn spend the context window on text the model will reduce to a
	// line each; get_job is where a full body is read, for the one posting a person picked.
	Summary string `json:"summary,omitempty" jsonschema:"a short preview, not the full description"`
	// Source names the board this posting was collected from, and OfficialJobURL is the
	// employer's own link. Both travel with every result: this channel renders results as
	// prose, where a link is easy to drop, and a posting repeated without its source reads
	// as though freehire were the employer.
	Source         string `json:"source" jsonschema:"the job board this posting was collected from"`
	URL            string `json:"url" jsonschema:"the posting's page on freehire"`
	OfficialJobURL string `json:"official_job_url,omitempty" jsonschema:"the employer's own posting, when known"`
}

// SearchJobsResult is what search_jobs answers.
type SearchJobsResult struct {
	Query string `json:"query,omitempty"`
	// Total is the whole match count, which Jobs is a page of. The two are separate so a
	// model can say "showing 10 of 4,312" instead of implying it has seen everything.
	Total  int          `json:"total"`
	Offset int          `json:"offset"`
	Jobs   []JobSummary `json:"jobs"`
	// IgnoredParams names the filters this tool could not honour. An answer that widens
	// without saying so is the failure mode this repository reports everywhere rather than
	// refusing — a retired facet must not break a saved search, and silence is what let a
	// mistyped country pass for a search of that country.
	IgnoredParams []string `json:"ignored_params,omitempty" jsonschema:"filters that were not applied; the answer is wider than asked"`
	// Semantic reports what became of a `semantic: true` request: SemanticApplied, or
	// SemanticUnavailable when it was answered by keywords alone. Omitted when not asked.
	Semantic string `json:"semantic,omitempty" jsonschema:"applied: matched by meaning as asked; unavailable: answered by keywords only"`
	// TotalIsLowerBound: a semantic answer has no count, only "this many and maybe more".
	TotalIsLowerBound bool `json:"total_is_lower_bound,omitempty" jsonschema:"true: total is a minimum, not a count — say 'at least', never 'of N'"`
}

// The two values of SearchJobsResult.Semantic.
const (
	SemanticApplied     = "applied"
	SemanticUnavailable = "unavailable"
)

// JobResult is one posting in full: the summary plus what only a chosen posting is worth
// reading.
type JobResult struct {
	JobSummary
	Description string `json:"description" jsonschema:"the posting's full text"`
	// Requirements is what the posting itself states it needs, separated into what is
	// required and what is merely preferred — a distinction the prose usually buries.
	Requirements []Requirement `json:"requirements,omitempty"`
	// ApplyVia names the ATS behind the application form when one was captured, so the
	// model can say where the application actually goes.
	ApplyVia string `json:"apply_via,omitempty" jsonschema:"the applicant tracking system handling applications"`
	// ApplyRequires is what the employer's own form will REFUSE the application without —
	// the standard fields plus every required question, as captured in apply_forms.
	//
	// It is the part of this catalogue almost nobody else can answer, and it is worth a
	// chat turn: a candidate deciding whether to start an application wants to know it
	// wants three essays BEFORE opening the page, not after. Absent where no form was
	// captured, which is most of the catalogue — an empty list would read as "this
	// employer asks for nothing".
	ApplyRequires []string `json:"apply_requires,omitempty" jsonschema:"what the employer's application form requires"`

	// The liveness signals answer "is this posting real and still open", as the job page
	// does. They are computed on read by the handler and are ABSENT where a lookup failed:
	// an empty level or a false flag must never stand in for a verdict nobody issued.
	//
	// They are flat like the rest of this shape, and the ghost criteria travel with the
	// level, because internal/job/ghost states facts about a posting, never an accusation
	// about an employer — a level without its criteria would be exactly that.
	LastSeenAt string `json:"last_seen_at,omitempty" jsonschema:"RFC3339 UTC; when a re-crawl last confirmed the posting live on its source"`
	Reality    string `json:"reality,omitempty" jsonschema:"fresh, stale or likely-evergreen: whether the posting looks like a live opening, judged from its age and repost history"`
	// FakeFreshness is a pointer for the reason the salary fields are: once reality was
	// computed, a computed false must reach the wire, distinct from a lookup that failed.
	FakeFreshness *bool    `json:"fake_freshness,omitempty" jsonschema:"true when the posted date is recent but freehire has seen the posting for 90 days or more; present whenever reality is"`
	GhostLevel    string   `json:"ghost_level,omitempty" jsonschema:"possible or likely: observed signs that the posting is not being actively filled; absent when there is too little evidence"`
	GhostCriteria []string `json:"ghost_criteria,omitempty" jsonschema:"the signs behind ghost_level: evergreen_posting, ats_absent (gone from the employer's own board), silent_applications, user_reports"`
}

// Requirement is one stated requirement and whether it is binding.
type Requirement struct {
	Text     string `json:"text"`
	Priority string `json:"priority" jsonschema:"required or preferred"`
}

// CompanySummary is one employer.
type CompanySummary struct {
	Slug     string `json:"slug" jsonschema:"the employer's identifier"`
	Name     string `json:"name"`
	Tagline  string `json:"tagline,omitempty"`
	OpenJobs int    `json:"open_jobs" jsonschema:"how many postings this employer has open right now"`
	URL      string `json:"url" jsonschema:"the employer's page on freehire"`
}

// SearchCompaniesResult is what search_companies answers.
type SearchCompaniesResult struct {
	Query     string           `json:"query,omitempty"`
	Total     int              `json:"total"`
	Offset    int              `json:"offset"`
	Companies []CompanySummary `json:"companies"`
	// IgnoredParams, for the same reason the job search carries it: an answer that widens
	// without saying so leaves the model to read a zero as an empty catalogue.
	IgnoredParams []string `json:"ignored_params,omitempty" jsonschema:"filters that were not applied; the answer is wider than asked"`
}

// CompanyResult is one employer in full.
//
// It is barely wider than the summary, and deliberately: what we hold about a company is a
// tagline, an industry list and a home country. The `company_info` JSONB beside them is not
// published — no surface in this repository reads it, and inventing a projection for a blob
// nothing else parses would be a shape with no source of truth behind it.
type CompanyResult struct {
	CompanySummary
	Industries []string `json:"industries,omitempty"`
	HQCountry  string   `json:"hq_country,omitempty" jsonschema:"ISO 3166-1 alpha-2 code"`
}

// Posting states check_jobs_open answers with.
const (
	StatusOpen     = "open"
	StatusClosed   = "closed"
	StatusNotFound = "not_found"
)

// JobOpenState is one posting's answer to "is it still up?".
//
// not_found covers a slug that never existed AND one this surface refuses (private, or a
// duplicate of another posting) — the same indistinguishability get_job keeps, because
// whether a private posting exists under some slug is not an anonymous caller's business.
type JobOpenState struct {
	Slug       string `json:"slug"`
	Status     string `json:"status" jsonschema:"open, closed or not_found"`
	ClosedAt   string `json:"closed_at,omitempty" jsonschema:"RFC3339 UTC; when freehire saw the posting close"`
	LastSeenAt string `json:"last_seen_at,omitempty" jsonschema:"RFC3339 UTC; when a re-crawl last confirmed the posting live"`
}

// JobsOpenResult is what check_jobs_open answers.
type JobsOpenResult struct {
	Jobs []JobOpenState `json:"jobs"`
	// Unchecked names the slugs past the per-call cap. A long shortlist is neither refused
	// nor silently cut: the model can say what is left and ask again.
	Unchecked []string `json:"unchecked,omitempty" jsonschema:"slugs beyond the per-call cap, which were not checked"`
}

// The deep-research shapes. ChatGPT's deep-research and company-knowledge connectors call a
// tool named `search` and one named `fetch` and read their answers in these fixed shapes; a
// server without them is skipped by those modes entirely. They are thin adaptations of
// search_jobs and get_job, not a second catalogue view.
type ResearchResults struct {
	Results []ResearchHit `json:"results"`
}

type ResearchHit struct {
	ID    string `json:"id" jsonschema:"the posting's identifier"`
	Title string `json:"title"`
	URL   string `json:"url"`
}

type ResearchDocument struct {
	ID       string           `json:"id"`
	Title    string           `json:"title"`
	Text     string           `json:"text"`
	URL      string           `json:"url"`
	Metadata ResearchMetadata `json:"metadata"`
}

// ResearchMetadata carries attribution and the liveness verdict alongside the text: a
// research report that quotes a posting without its board reads as though freehire were the
// employer.
type ResearchMetadata struct {
	Company        string `json:"company"`
	Location       string `json:"location,omitempty"`
	Source         string `json:"source"`
	OfficialJobURL string `json:"official_job_url,omitempty"`
	PostedAt       string `json:"posted_at,omitempty"`
	Reality        string `json:"reality,omitempty"`
	GhostLevel     string `json:"ghost_level,omitempty"`
}

// SimilarJobsResult is what similar_jobs answers: the posting's nearest neighbours, nearest
// first. Empty where none have been computed yet, which is true of the newest postings.
type SimilarJobsResult struct {
	Jobs []JobSummary `json:"jobs"`
}

// SalaryStatsResult is what salary_stats answers. Currencies and pay periods are never
// combined: each band is one (seniority, currency, period), and a band below the rollup's
// minimum sample size is absent rather than shown on too few postings.
type SalaryStatsResult struct {
	Bands         []SalaryBand `json:"bands"`
	IgnoredParams []string     `json:"ignored_params,omitempty" jsonschema:"scope values that were not applied; the answer is wider than asked"`
}

type SalaryBand struct {
	Seniority  string `json:"seniority,omitempty" jsonschema:"empty for the band across every seniority"`
	Currency   string `json:"currency" jsonschema:"ISO 4217 code"`
	Period     string `json:"period" jsonschema:"year, month, day or hour"`
	SampleSize int    `json:"sample_size" jsonschema:"how many open postings stating a salary the band is computed from"`
	P25        int    `json:"p25"`
	P50        int    `json:"p50" jsonschema:"the median"`
	P75        int    `json:"p75"`
}

// MarketStatsResult is what job_market_stats answers: how many open postings match, and what
// dominates them.
type MarketStatsResult struct {
	Total         int                     `json:"total" jsonschema:"open postings matching the filters"`
	Facets        map[string][]FacetCount `json:"facets,omitempty" jsonschema:"for each of category, seniority, skills, work_mode, countries, regions, employment_type, english_level, visa_sponsorship and relocation: its most common values among the matches, highest count first"`
	IgnoredParams []string                `json:"ignored_params,omitempty" jsonschema:"filters that were not applied; the answer is wider than asked"`
}

type FacetCount struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}
