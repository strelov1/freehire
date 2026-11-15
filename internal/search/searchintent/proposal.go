package searchintent

import (
	"fmt"
	"sync"

	"github.com/strelov1/freehire/internal/dict/vocab"
	"github.com/strelov1/freehire/internal/job/collections"
	"github.com/strelov1/freehire/internal/platform/llmschema"
)

// proposal is the shape the model answers in: one field per filter this surface can
// ground.
//
// It is a typed struct, not a map, because the strict-mode schema forbids free-form keys;
// it also lets closed vocabularies reach the model as enums (see requestSchema).
//
// intent() converts it for resolution; a test keeps the two in step.
type proposal struct {
	// Summary is one sentence naming the search these values build, asked for in the same
	// response so it cannot describe a different search.
	Summary string `json:"summary"`
	// Query is free text for a concept no filter expresses, and nothing else.
	Query string `json:"query"`

	Category       flexStrings `json:"category"`
	Seniority      flexStrings `json:"seniority"`
	RoleType       flexStrings `json:"role_type"`
	Skills         flexStrings `json:"skills"`
	Domains        flexStrings `json:"domains"`
	AIArchetype    flexStrings `json:"ai_archetype"`
	WorkMode       flexStrings `json:"work_mode"`
	Regions        flexStrings `json:"regions"`
	Countries      flexStrings `json:"countries"`
	Cities         flexStrings `json:"cities"`
	Relocation     flexStrings `json:"relocation"`
	EmploymentType flexStrings `json:"employment_type"`
	CompanyType    flexStrings `json:"company_type"`
	CompanySize    flexStrings `json:"company_size"`
	Collections    flexStrings `json:"collections"`
	EnglishLevel   flexStrings `json:"english_level"`
	EducationLevel flexStrings `json:"education_level"`
	SalaryPeriod   flexStrings `json:"salary_period"`

	// Exclude is what the person ruled out ("remote, but not in the USA").
	Exclude exclusions `json:"exclude"`

	SalaryMin          *flexInt `json:"salary_min"`
	PostedWithinDays   *flexInt `json:"posted_within_days"`
	ExperienceYearsMax *flexInt `json:"experience_years_max"`
	VisaSponsorship    flexBool `json:"visa_sponsorship"`
}

// exclusions are the filters people phrase negatively. A deliberate subset of the fields
// above: every facet can be excluded, but each field offered costs tokens on every request.
type exclusions struct {
	Skills      flexStrings `json:"skills"`
	Countries   flexStrings `json:"countries"`
	Regions     flexStrings `json:"regions"`
	Cities      flexStrings `json:"cities"`
	Category    flexStrings `json:"category"`
	Domains     flexStrings `json:"domains"`
	CompanyType flexStrings `json:"company_type"`
	RoleType    flexStrings `json:"role_type"`
}

// intent flattens the proposal into the facet map resolution walks. Every key is present
// even when empty: the drift test reads it as the list of filters the surface offers.
func (p proposal) intent() intent {
	return intent{
		Facets: map[string][]string{
			"category":        p.Category,
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
		SalaryMin:          p.SalaryMin.plain(),
		PostedWithinDays:   p.PostedWithinDays.plain(),
		ExperienceYearsMax: p.ExperienceYearsMax.plain(),
		VisaSponsorship:    p.VisaSponsorship.plain(),
	}
}

// schemaName labels the response format for the gateway's logs.
const schemaName = "search_intent"

var (
	schemaOnce sync.Once
	schema     llmschema.Schema
	schemaErr  error
)

// requestSchema derives the model's response format from proposal and pins every closed
// vocabulary to its enum.
//
// Skills, cities and countries stay free strings: each has thousands of values, and the
// dictionaries check them afterwards (invented values are dropped and reported).
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
			// The bounds are asked for as text for two reasons. Zero is a real experience
			// ceiling but the model writes it for "unset"; as text "" and "0" differ (see
			// flexInt). And flexInt is a struct with no exported fields, which reflection
			// would declare as an empty object, so the model answers "{}". schema_test.go
			// pins it.
			llmschema.AsText("salary_min", "posted_within_days", "experience_years_max"),
		)
		if schemaErr != nil {
			schemaErr = fmt.Errorf("searchintent: build schema: %w", schemaErr)
		}
	})
	return schema, schemaErr
}
