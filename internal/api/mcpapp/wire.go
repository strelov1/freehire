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
	Slug    string `json:"slug" jsonschema:"the posting's identifier, to pass to get_job"`
	Title   string `json:"title"`
	Company string `json:"company"`
	// CompanySlug travels beside the display name because it is what get_company takes.
	// Resolving a name back to a slug is a guess; carrying it is not.
	CompanySlug string `json:"company_slug,omitempty" jsonschema:"the employer's identifier, to pass to get_company"`
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
	// Summary is the search index's truncated preview, never the full body. Ten full job
	// descriptions in one turn spend the context window on text the model will reduce to a
	// line each; get_job is where a full body is read, for the one posting a person picked.
	Summary string `json:"summary,omitempty" jsonschema:"a short preview; call get_job for the full description"`
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
}

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
}

// Requirement is one stated requirement and whether it is binding.
type Requirement struct {
	Text     string `json:"text"`
	Priority string `json:"priority" jsonschema:"required or preferred"`
}

// CompanySummary is one employer.
type CompanySummary struct {
	Slug     string `json:"slug" jsonschema:"the employer's identifier, to pass to get_company"`
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
