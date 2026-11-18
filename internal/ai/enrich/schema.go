package enrich

import (
	"fmt"
	"sync"

	"github.com/strelov1/freehire/internal/dict/vocab"
	"github.com/strelov1/freehire/internal/platform/llmschema"
)

// schemaName labels the shape in the provider's logs; the geo-less variant is named apart
// so the client caches it separately.
const (
	schemaName      = "job_enrichment"
	schemaNameNoGeo = "job_enrichment_nogeo"
)

// unaskedFields are contract fields the prompt does not request, so the schema must not
// either: under strict mode every property is required. They come from the deterministic
// dictionaries (`internal/job/jobderive`). category is asked: the title dictionary leaves
// ~2.7M open postings unfiled, and search reads the model's answer when the column is empty
// (#4412).
var unaskedFields = []string{
	"work_mode", "employment_type",
	"seniority", "english_level", "education_level",
	"experience_years_min", "posting_language",
}

// geoFields are dropped when the dictionary already pinned geography; `geoFacet` discards
// the model's copy.
var geoFields = []string{"regions", "countries"}

// constrainedEnums are the enum fields pinned to their vocabulary. Regions and countries
// are deliberately absent: the prompt lets the model give its own label there, and an enum
// would block the novel values that discovery facets collect.
func constrainedEnums() []llmschema.Override {
	return []llmschema.Override{
		llmschema.Enum("relocation", vocab.RelocationValues),
		llmschema.Enum("salary_period", vocab.SalaryPeriodValues),
		llmschema.Enum("domains", vocab.DomainValues),
		llmschema.Enum("company_type", vocab.CompanyTypeValues),
		llmschema.Enum("company_size", vocab.CompanySizeValues),
		llmschema.Enum("category", vocab.CategoryValues),
	}
}

var (
	schemaOnce sync.Once
	schemas    map[bool]llmschema.Schema
	schemaErr  error
)

// requestSchema returns the schema matching buildSystemPrompt; askGeo mirrors its switch.
func requestSchema(askGeo bool) (llmschema.Schema, string, error) {
	schemaOnce.Do(func() {
		schemas = map[bool]llmschema.Schema{}
		for _, ask := range []bool{true, false} {
			omit := unaskedFields
			if !ask {
				omit = append(append([]string{}, unaskedFields...), geoFields...)
			}

			overrides := append([]llmschema.Override{llmschema.Omit(omit...)}, constrainedEnums()...)

			s, err := llmschema.Of[Enrichment](overrides...)
			if err != nil {
				schemaErr = fmt.Errorf("enrich: build schema (askGeo=%v): %w", ask, err)
				return
			}
			schemas[ask] = s
		}
	})

	if schemaErr != nil {
		return nil, "", schemaErr
	}

	if askGeo {
		return schemas[true], schemaName, nil
	}

	return schemas[false], schemaNameNoGeo, nil
}
