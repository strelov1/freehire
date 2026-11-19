package mcpapp

import (
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"strconv"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
)

// Page bounds: a huge page would spend the turn's context on a list nobody reads.
const (
	defaultPageSize = 10
	maxPageSize     = 50
)

// The tool inputs.
//
// /jobs/search accepts far more parameters than belong here: this schema sits in front of
// the model on every turn, so only the subset someone asks for in a sentence is published;
// the rest stays on REST and the OpenAPI document.
//
// Every field is optional. An omitted filter must not become a filter on the zero value,
// which would silently narrow the answer.

// SearchInput is search_jobs' input.
type SearchInput struct {
	// The operators are listed because a model only writes what the schema says exists.
	Query string `json:"query,omitempty" jsonschema:"free text, matched against title, company, location and the start of the description (not the full text, so details such as equity or visa may be missing from the match). Use few, specific words. Operators: double quotes require every quoted word in any order (not a phrase); -word excludes; uppercase OR between terms matches any alternative (at most 5), e.g. golang OR rust; title: at the start of an alternative matches it in the title only, e.g. title:\"product owner\" OR title:produktchef"`

	Countries stringList `json:"countries,omitempty" jsonschema:"ISO 3166-1 alpha-2 codes, e.g. DE, BR"`
	Cities    stringList `json:"cities,omitempty"`
	// WorkMode is a list because "remote or hybrid" cannot be a boolean.
	WorkMode stringList `json:"work_mode,omitempty" jsonschema:"any of remote, hybrid, onsite"`
	// Regions are where the role can be done FROM ("remote, LATAM"). They union with countries
	// and cities, as on the website. The list is the whole of vocab.RegionValues, held by a test.
	Regions stringList `json:"regions,omitempty" jsonschema:"any of global, north_america, latam, eu, uk, mena, africa, apac, cis"`

	// The listed values are the WHOLE of vocab.SeniorityValues, held there by
	// TestSeniorityDescriptionNamesEveryLevel. It is a description, not an enum, so a level
	// missing from it is still accepted but invisible to the agent reading the schema.
	Seniority stringList `json:"seniority,omitempty" jsonschema:"any of intern, junior, middle, senior, lead, staff, principal, c_level"`
	Category  stringList `json:"category,omitempty" jsonschema:"role family, e.g. backend, frontend, devops, data"`
	Skills    stringList `json:"skills,omitempty" jsonschema:"technologies, e.g. go, kubernetes, react"`
	// RequireAllSkills turns skills from "any of" into "all of".
	RequireAllSkills bool       `json:"require_all_skills,omitempty" jsonschema:"true: every listed skill must be present, not just one"`
	ExcludeSkills    stringList `json:"exclude_skills,omitempty" jsonschema:"technologies the posting must not ask for, e.g. php"`
	EmploymentType   stringList `json:"employment_type,omitempty" jsonschema:"any of full_time, part_time, contract, internship"`
	// Engagement is the contract form, which employment_type cannot tell apart: a contract
	// role may be freelance B2B or a fixed-term employment contract.
	Engagement stringList `json:"engagement,omitempty" jsonschema:"contract form, e.g. b2b, c2c, ir35_outside, uop"`

	// SalaryMin is a floor in SalaryCurrency; without a currency it is ambiguous, so the two travel together.
	SalaryMin      int    `json:"salary_min,omitempty"`
	SalaryCurrency string `json:"salary_currency,omitempty" jsonschema:"ISO 4217 code, e.g. USD"`

	// VisaSponsorship is a POINTER because false and absent differ: false is not a filter at
	// all, absent means nothing was said.
	VisaSponsorship *bool      `json:"visa_sponsorship,omitempty"`
	EnglishLevel    stringList `json:"english_level,omitempty" jsonschema:"CEFR levels, e.g. B2, C1"`
	Relocation      stringList `json:"relocation,omitempty" jsonschema:"any of supported (the employer helps you move), required (the role needs you to move), not_supported"`

	CompanySlugs        stringList `json:"company_slugs,omitempty" jsonschema:"employer identifiers, as carried by a result's company_slug"`
	ExcludeCompanySlugs stringList `json:"exclude_company_slugs,omitempty" jsonschema:"employers to leave out, as carried by a result's company_slug"`
	Sources             stringList `json:"sources,omitempty" jsonschema:"job boards to restrict to"`

	PostedWithinDays int `json:"posted_within_days,omitempty"`

	Limit  int `json:"limit,omitempty" jsonschema:"results per page"`
	Offset int `json:"offset,omitempty"`
}

// CompanySearchInput is search_companies' input, deliberately smaller than the job one to
// save schema tokens on every turn.
type CompanySearchInput struct {
	Query     string     `json:"query,omitempty" jsonschema:"free text, matched against the employer's name"`
	Countries stringList `json:"countries,omitempty" jsonschema:"ISO 3166-1 alpha-2 codes the employer hires in"`
	Limit     int        `json:"limit,omitempty"`
	Offset    int        `json:"offset,omitempty"`
}

// JobDetailInput and the other named inputs below are separate types because the SDK derives
// a tool's input schema from the Go type: field names and descriptions are the published contract.
type JobDetailInput struct {
	Slug string `json:"slug" jsonschema:"the posting's identifier, as carried by a search result's slug"`
}

type CheckJobsOpenInput struct {
	Slugs stringList `json:"slugs" jsonschema:"posting identifiers, as carried by search results' slug; up to 50 are checked per call"`
}

// ResearchQuery and ResearchID are the inputs of search and fetch, whose shapes ChatGPT's
// deep-research and company-knowledge connectors fix: one query string in, one id in.
type ResearchQuery struct {
	Query string `json:"query" jsonschema:"free text describing the jobs to find; matched against title, company, location and the start of the description, so use few, specific words"`
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
// It builds url.Values for search.FilterFromValues, the same path the REST handlers and the
// OJCP transport take, so there is no second filter builder that could disagree.
//
// A field the caller left unset writes NO parameter: a written zero would silently narrow.
func (in SearchInput) QueryValues() url.Values {
	v := url.Values{}
	setNonEmpty(v, "q", in.Query)
	setList(v, "countries", countryCodes(in.Countries))
	setList(v, "cities", in.Cities)
	setList(v, "company_slug", in.CompanySlugs)
	setList(v, "company_slug_exclude", in.ExcludeCompanySlugs)
	setList(v, "source", in.Sources)
	// The one table also decides what Unsupported reports, so what is SENT and what is
	// REPORTED cannot disagree about a value.
	for _, facet := range checkedFacets {
		setList(v, facet.param, keepKnown(facet.param, facet.values(in)))
	}
	if in.RequireAllSkills && v.Has("skills") {
		v.Set("skills_mode", "and")
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

// Unsupported names the filter values a company search cannot honour, as the job search does.
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

// setList writes a facet as the comma-separated form the search core splits.
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

// stringList is a list filter that also accepts a single bare value: models routinely send a
// one-item list as a string (work_mode: "remote"). stringListSchema publishes both shapes, so
// the SDK's validation lets either through to UnmarshalJSON.
type stringList []string

func (l *stringList) UnmarshalJSON(data []byte) error {
	// Only a JSON string is one value: null must stay "no filter", and json.Unmarshal would
	// read it into a string as "" without an error.
	if len(data) > 0 && data[0] == '"' {
		var one string
		if err := json.Unmarshal(data, &one); err != nil {
			return err
		}
		*l = stringList{one}
		return nil
	}
	return json.Unmarshal(data, (*[]string)(l))
}

var stringListSchema = &jsonschema.Schema{
	Types: []string{"null", "array", "string"},
	Items: &jsonschema.Schema{Type: "string"},
}

// inputSchema infers a tool's input schema with stringList published as a string or a list.
func inputSchema[In any]() *jsonschema.Schema {
	schema, err := jsonschema.For[In](&jsonschema.ForOptions{
		TypeSchemas: map[reflect.Type]*jsonschema.Schema{reflect.TypeFor[stringList](): stringListSchema},
	})
	if err != nil {
		panic(fmt.Sprintf("mcpapp: input schema for %T: %v", *new(In), err))
	}
	return schema
}
