package searchintent

import (
	"fmt"
	"sync"

	"github.com/strelov1/freehire/internal/collections"
	"github.com/strelov1/freehire/internal/llmschema"
	"github.com/strelov1/freehire/internal/vocab"
)

// proposal is the shape the model answers in — one field per filter this surface can
// ground, and nothing else.
//
// It exists as a typed struct rather than as a map of facet name to values because the
// schema the model is constrained by runs in strict mode, which forbids the free-form
// keys a map needs. Naming each filter buys more than legality, though: the closed
// vocabularies reach the model as enums (see requestSchema), so the values it may
// write are fixed by the schema instead of merely described to it, and a model that
// cannot name a facet cannot invent one.
//
// intent() turns it into the form resolution reads. The two are kept in step by a test
// rather than by discipline — a facet in one and not the other is invisible at runtime.
type proposal struct {
	// Summary is one sentence naming the search these values build. It is asked for in
	// the SAME response as the values so it can never describe a different search than
	// the one that was resolved.
	Summary string `json:"summary"`
	// Query is free text for a concept no filter expresses, and nothing else.
	Query string `json:"query"`

	Category       []string `json:"category"`
	Role           []string `json:"role"`
	Seniority      []string `json:"seniority"`
	RoleType       []string `json:"role_type"`
	Skills         []string `json:"skills"`
	Domains        []string `json:"domains"`
	AIArchetype    []string `json:"ai_archetype"`
	WorkMode       []string `json:"work_mode"`
	Regions        []string `json:"regions"`
	Countries      []string `json:"countries"`
	Cities         []string `json:"cities"`
	Relocation     []string `json:"relocation"`
	EmploymentType []string `json:"employment_type"`
	CompanyType    []string `json:"company_type"`
	CompanySize    []string `json:"company_size"`
	Collections    []string `json:"collections"`
	EnglishLevel   []string `json:"english_level"`
	EducationLevel []string `json:"education_level"`
	SalaryPeriod   []string `json:"salary_period"`

	// Exclude is what the person ruled out. "Remote, but not in the USA" is how people
	// actually describe a search, and a filter that can only add answers a different
	// question.
	Exclude exclusions `json:"exclude"`

	SalaryMin          *int `json:"salary_min"`
	PostedWithinDays   *int `json:"posted_within_days"`
	ExperienceYearsMax *int `json:"experience_years_max"`
	VisaSponsorship    bool `json:"visa_sponsorship"`
}

// exclusions are the filters people phrase negatively. It is deliberately a subset of
// the fields above: every facet supports exclusion technically, but nobody says "not
// B2 English" or "not a full-time role", and each field offered costs tokens on every
// request whether or not it is used.
type exclusions struct {
	Skills      []string `json:"skills"`
	Countries   []string `json:"countries"`
	Regions     []string `json:"regions"`
	Cities      []string `json:"cities"`
	Category    []string `json:"category"`
	Domains     []string `json:"domains"`
	CompanyType []string `json:"company_type"`
	RoleType    []string `json:"role_type"`
}

// intent flattens the proposal into the facet map resolution walks. Every key is
// present whether or not the model filled it, so this doubles as the statement of
// which filters the surface offers — the drift test reads it as exactly that.
func (p proposal) intent() intent {
	return intent{
		Facets: map[string][]string{
			"category":        p.Category,
			"role":            p.Role,
			"seniority":       p.Seniority,
			"role_type":       p.RoleType,
			"skills":          p.Skills,
			"domains":         p.Domains,
			"ai_archetype":    p.AIArchetype,
			"work_mode":       p.WorkMode,
			"regions":         p.Regions,
			"countries":       p.Countries,
			"cities":          p.Cities,
			"relocation":      p.Relocation,
			"employment_type": p.EmploymentType,
			"company_type":    p.CompanyType,
			"company_size":    p.CompanySize,
			"collections":     p.Collections,
			"english_level":   p.EnglishLevel,
			"education_level": p.EducationLevel,
			"salary_period":   p.SalaryPeriod,
		},
		Exclude: map[string][]string{
			"skills":       p.Exclude.Skills,
			"countries":    p.Exclude.Countries,
			"regions":      p.Exclude.Regions,
			"cities":       p.Exclude.Cities,
			"category":     p.Exclude.Category,
			"domains":      p.Exclude.Domains,
			"company_type": p.Exclude.CompanyType,
			"role_type":    p.Exclude.RoleType,
		},
		Query:              p.Query,
		Summary:            p.Summary,
		SalaryMin:          p.SalaryMin,
		PostedWithinDays:   p.PostedWithinDays,
		ExperienceYearsMax: p.ExperienceYearsMax,
		VisaSponsorship:    p.VisaSponsorship,
	}
}

// schemaName labels the response format for the gateway's logs.
const schemaName = "search_intent"

var (
	schemaOnce sync.Once
	schema     llmschema.Schema
	schemaErr  error
)

// requestSchema derives the model's response format from proposal and pins every
// closed vocabulary to its enum.
//
// The open vocabularies — skills, cities, countries, and the role catalogue — are left
// as free strings deliberately: each runs to thousands of values, and spending that
// many tokens on every request to constrain what the dictionaries already check after
// the fact would buy nothing. The values the model invents there are dropped and
// reported, which is the contract; an invented CLOSED value would be the same drop for
// a value that did not need to be invented at all.
func requestSchema() (llmschema.Schema, error) {
	schemaOnce.Do(func() {
		schema, schemaErr = llmschema.Of[proposal](
			llmschema.Enum("category", vocab.CategoryValues),
			llmschema.Enum("seniority", vocab.SeniorityValues),
			llmschema.Enum("role_type", vocab.RoleTypeValues),
			llmschema.Enum("domains", vocab.DomainValues),
			llmschema.Enum("ai_archetype", vocab.AIArchetypeValues),
			llmschema.Enum("work_mode", vocab.WorkModeValues),
			llmschema.Enum("regions", vocab.RegionValues),
			llmschema.Enum("relocation", vocab.RelocationValues),
			llmschema.Enum("employment_type", vocab.EmploymentTypeValues),
			llmschema.Enum("company_type", vocab.CompanyTypeValues),
			llmschema.Enum("company_size", vocab.CompanySizeValues),
			llmschema.Enum("collections", collections.Slugs()),
			llmschema.Enum("english_level", vocab.EnglishLevelValues),
			llmschema.Enum("education_level", vocab.EducationLevelValues),
			llmschema.Enum("salary_period", vocab.SalaryPeriodValues),
		)
		if schemaErr != nil {
			schemaErr = fmt.Errorf("searchintent: build schema: %w", schemaErr)
		}
	})
	return schema, schemaErr
}
