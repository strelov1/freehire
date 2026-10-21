package mcpapp

import (
	"net/url"
	"strconv"
	"strings"
)

// Page bounds. A model asking for five hundred results would spend the whole turn's context
// on a list nobody reads, and ten is what a person scans in a chat before asking to narrow.
const (
	defaultPageSize = 10
	maxPageSize     = 50
)

// The tool inputs.
//
// /jobs/search accepts around fifty parameters. All fifty do not belong here: this schema
// sits in front of the model on every turn, and most of that vocabulary is machinery a
// person never says out loud (`skills_mode`, `role_type_exclude`, `open_within_days`).
// What is published is the subset someone actually asks for in a sentence; the rest stays
// reachable over REST and the OpenAPI document.
//
// Every field is optional. A filter the caller omits must not become a filter on the zero
// value — that is how an unspoken preference turns into a silently narrowed answer.

// SearchInput is search_jobs' input.
type SearchInput struct {
	// The operators are listed here because a model only writes what the schema tells it
	// exists; the engine has understood them since freehire#3270.
	Query string `json:"query,omitempty" jsonschema:"free text, matched against title, company, description and location. Operators: double quotes require every quoted word in any order (not a phrase); -word excludes; uppercase OR between terms matches any alternative (at most 5), e.g. golang OR rust; title: at the start of an alternative matches it in the title only, e.g. title:\"product owner\" OR title:produktchef"`

	Countries []string `json:"countries,omitempty" jsonschema:"ISO 3166-1 alpha-2 codes, e.g. DE, BR"`
	Cities    []string `json:"cities,omitempty"`
	// WorkMode is a list rather than a flag because "remote or hybrid" is a thing people
	// ask for and a boolean cannot express it.
	WorkMode []string `json:"work_mode,omitempty" jsonschema:"any of remote, hybrid, onsite"`
	// Regions are where the role can be done FROM, which is what a remote candidate asks
	// ("remote, LATAM"). They union with countries and cities, as on the website. The list is
	// the whole of vocab.RegionValues, held there by a test like seniority's.
	Regions []string `json:"regions,omitempty" jsonschema:"any of global, north_america, latam, eu, uk, mena, africa, apac, cis"`

	// The listed values are the WHOLE of vocab.SeniorityValues, held there by
	// TestSeniorityDescriptionNamesEveryLevel. It is a description, not an enum, so a level
	// missing from it is still accepted — and invisible to the agent reading the schema,
	// which is how `c_level` went unaskable here.
	Seniority []string `json:"seniority,omitempty" jsonschema:"any of intern, junior, middle, senior, lead, staff, principal, c_level"`
	Category  []string `json:"category,omitempty" jsonschema:"role family, e.g. backend, frontend, devops, data"`
	Skills    []string `json:"skills,omitempty" jsonschema:"technologies, e.g. go, kubernetes, react"`
	// RequireAllSkills turns skills from "any of" into "all of" — "Go AND Kubernetes" is a
	// different question from "Go OR Kubernetes", and a person means one of them.
	RequireAllSkills bool     `json:"require_all_skills,omitempty" jsonschema:"true: every listed skill must be present, not just one"`
	ExcludeSkills    []string `json:"exclude_skills,omitempty" jsonschema:"technologies the posting must not ask for, e.g. php"`
	EmploymentType   []string `json:"employment_type,omitempty" jsonschema:"any of full_time, part_time, contract, internship"`

	// SalaryMin is a floor, expressed in SalaryCurrency. A floor without a currency is
	// ambiguous across a catalogue holding both, so the two travel together.
	SalaryMin      int    `json:"salary_min,omitempty"`
	SalaryCurrency string `json:"salary_currency,omitempty" jsonschema:"ISO 4217 code, e.g. USD"`

	// VisaSponsorship is a POINTER because false and absent differ: false means "sponsorship
	// is not needed", which is not a filter at all, while absent means nothing was said.
	VisaSponsorship *bool    `json:"visa_sponsorship,omitempty"`
	EnglishLevel    []string `json:"english_level,omitempty" jsonschema:"CEFR levels, e.g. B2, C1"`
	Relocation      []string `json:"relocation,omitempty" jsonschema:"any of supported (the employer helps you move), required (the role needs you to move), not_supported"`

	CompanySlugs        []string `json:"company_slugs,omitempty" jsonschema:"employer identifiers, as carried by a result's company_slug"`
	ExcludeCompanySlugs []string `json:"exclude_company_slugs,omitempty" jsonschema:"employers to leave out, as carried by a result's company_slug"`
	Sources             []string `json:"sources,omitempty" jsonschema:"job boards to restrict to"`

	PostedWithinDays int `json:"posted_within_days,omitempty"`

	// Semantic is a POINTER for the reason VisaSponsorship is: absent and false both mean
	// the ordinary keyword search, and only true asks for anything.
	Semantic *bool `json:"semantic,omitempty" jsonschema:"true: also match postings that describe the query in other words. Use it for a need described in words, e.g. 'keep our payment systems secure' or 'help kids learn to read'; leave it off for a job title or a non-technical role, which keywords answer better. Needs query"`

	Limit  int `json:"limit,omitempty" jsonschema:"results per page"`
	Offset int `json:"offset,omitempty"`
}

// WantsSemantic reports whether the caller asked for semantic search.
func (in SearchInput) WantsSemantic() bool { return in.Semantic != nil && *in.Semantic }

// CompanySearchInput is search_companies' input. It is deliberately smaller than the job
// one: an employer has far fewer facets worth asking about, and a wide schema here would
// cost tokens on every turn to answer questions nobody puts to it.
type CompanySearchInput struct {
	Query     string   `json:"query,omitempty" jsonschema:"free text, matched against the employer's name"`
	Countries []string `json:"countries,omitempty" jsonschema:"ISO 3166-1 alpha-2 codes the employer hires in"`
	Limit     int      `json:"limit,omitempty"`
	Offset    int      `json:"offset,omitempty"`
}

// JobDetailInput and CompanyDetailInput are the two single-item reads. They exist as named
// types because the SDK derives a tool's input schema from the Go type, so the field name
// and its description are part of the published contract.
type JobDetailInput struct {
	Slug string `json:"slug" jsonschema:"the posting's identifier, as carried by a search result's slug"`
}

type CheckJobsOpenInput struct {
	Slugs []string `json:"slugs" jsonschema:"posting identifiers, as carried by search results' slug; up to 50 are checked per call"`
}

// ResearchQuery and ResearchID are the inputs of search and fetch, whose shapes ChatGPT's
// deep-research and company-knowledge connectors fix: one query string in, one id in.
type ResearchQuery struct {
	Query string `json:"query" jsonschema:"free text describing the jobs to find"`
}

type ResearchID struct {
	ID string `json:"id" jsonschema:"a result id from search"`
}

type SimilarJobsInput struct {
	Slug  string `json:"slug" jsonschema:"the posting's identifier, as carried by a search result's slug"`
	Limit int    `json:"limit,omitempty" jsonschema:"how many to return, up to 20"`
}

// SalaryInput scopes salary_stats. Every field is optional; a category alone also breaks
// the answer down by seniority.
type SalaryInput struct {
	Category  string `json:"category,omitempty" jsonschema:"role family, e.g. backend, frontend, devops, data"`
	Seniority string `json:"seniority,omitempty" jsonschema:"any of intern, junior, middle, senior, lead, staff, principal, c_level"`
	Country   string `json:"country,omitempty" jsonschema:"ISO 3166-1 alpha-2 code, e.g. DE"`
}

type CompanyDetailInput struct {
	Slug string `json:"slug" jsonschema:"the employer's identifier, as carried by a result's company_slug"`
}

// QueryValues renders the input as the query the search core already reads.
//
// It builds url.Values and hands them to search.FilterFromValues rather than assembling a
// filter itself — the same path the REST handlers and the OJCP transport take. That is what
// makes "this tool searches the same catalogue the website does" a fact about the code
// instead of a claim: there is no second filter builder that could disagree.
//
// A field the caller left unset writes NO parameter. The distinction matters more than it
// looks: a zero written out is an unspoken preference turned into a silent narrowing.
func (in SearchInput) QueryValues() url.Values {
	v := url.Values{}
	setNonEmpty(v, "q", in.Query)
	setList(v, "countries", countryCodes(in.Countries))
	setList(v, "cities", in.Cities)
	setList(v, "skills", in.Skills)
	if in.RequireAllSkills && v.Has("skills") {
		v.Set("skills_mode", "and")
	}
	setList(v, "skills_exclude", in.ExcludeSkills)
	setList(v, "company_slug", in.CompanySlugs)
	setList(v, "company_slug_exclude", in.ExcludeCompanySlugs)
	setList(v, "source", in.Sources)
	// The closed-vocabulary facets come from the one table that also decides what
	// Unsupported reports, so what is SENT and what is REPORTED cannot disagree about a
	// value, and a new facet joins both by being written once.
	for _, facet := range checkedFacets {
		setList(v, facet.param, keepKnown(facet.allowed, facet.values(in)))
	}
	setNonEmpty(v, "salary_currency", in.SalaryCurrency)
	setPositive(v, "salary_min", in.SalaryMin)
	setPositive(v, "posted_within_days", in.PostedWithinDays)
	if in.VisaSponsorship != nil {
		v.Set("visa_sponsorship", strconv.FormatBool(*in.VisaSponsorship))
	}
	return v
}

// Page is the bounded page the tool will actually serve.
func (in SearchInput) Page() (limit, offset int) { return page(in.Limit, in.Offset) }

// QueryValues renders the company input.
func (in CompanySearchInput) QueryValues() url.Values {
	v := url.Values{}
	setNonEmpty(v, "q", in.Query)
	setList(v, "countries", countryCodes(in.Countries))
	return v
}

// Unsupported names the filter values a company search cannot honour, the same way the job
// one does. It was missing here at first, which is the shape of gap worth noticing: the
// company tool is smaller, so it got less thought, and "answers zero and says nothing" is
// exactly as wrong on four fields as on fifteen.
func (in CompanySearchInput) Unsupported() []string {
	return unsupportedCountries(in.Countries)
}

// Page is the bounded page the tool will actually serve.
func (in CompanySearchInput) Page() (limit, offset int) { return page(in.Limit, in.Offset) }

func page(limit, offset int) (int, int) {
	switch {
	case limit <= 0:
		limit = defaultPageSize
	case limit > maxPageSize:
		limit = maxPageSize
	}
	return limit, max(offset, 0)
}

// setList writes a facet as the comma-separated form the search core splits. Values are
// joined rather than repeated only because one parameter reads more plainly in a log; the
// core accepts either.
func setList(v url.Values, param string, values []string) {
	kept := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			kept = append(kept, trimmed)
		}
	}
	if len(kept) > 0 {
		v.Set(param, strings.Join(kept, ","))
	}
}

func setNonEmpty(v url.Values, param, value string) {
	if trimmed := strings.TrimSpace(value); trimmed != "" {
		v.Set(param, trimmed)
	}
}

func setPositive(v url.Values, param string, value int) {
	if value > 0 {
		v.Set(param, strconv.Itoa(value))
	}
}
