package enrich

import (
	"fmt"
	"sync"

	"github.com/strelov1/freehire/internal/llmschema"
	"github.com/strelov1/freehire/internal/vocab"
)

// schemaName labels the shape in the provider's logs. The geo-less variant is named
// apart so the two are distinguishable there and cached separately by the client.
const (
	schemaName      = "job_enrichment"
	schemaNameNoGeo = "job_enrichment_nogeo"
)

// unaskedFields are contract fields the prompt deliberately does not request, so the
// schema must not either. Under strict mode every property is required — leaving one
// in would order the model to produce a value that `jobview` then discards, which is
// the token burn `enrich-prompt-trim` removed. They are served from the deterministic
// dictionaries (`internal/jobderive`), not the LLM.
var unaskedFields = []string{
	"work_mode", "employment_type",
	"seniority", "english_level", "education_level",
	"category", "experience_years_min", "posting_language",
}

// geoFields are dropped when the deterministic dictionary already pinned the job's
// geography: `geoFacet` discards the model's copy, so asking for it only costs tokens.
var geoFields = []string{"regions", "countries"}

// constrainedEnums are the enum fields the schema pins to their vocabulary.
//
// Regions and countries are deliberately ABSENT. They are the dict-then-LLM hybrid
// discovery facets, and the prompt explicitly permits a label of the model's own when
// no allowed value fits; an enum would foreclose exactly the novel value that mechanism
// exists to collect (see "Unserved discovery facets are captured raw, not validated").
func constrainedEnums() []llmschema.Override {
	return []llmschema.Override{
		llmschema.Enum("relocation", vocab.RelocationValues),
		llmschema.Enum("salary_period", vocab.SalaryPeriodValues),
		llmschema.Enum("domains", vocab.DomainValues),
		llmschema.Enum("company_type", vocab.CompanyTypeValues),
		llmschema.Enum("company_size", vocab.CompanySizeValues),
	}
}

var (
	schemaOnce sync.Once
	schemas    map[bool]llmschema.Schema
	schemaErr  error
)

// requestSchema returns the schema matching what buildSystemPrompt asks for. askGeo
// mirrors the prompt's own switch, so the two cannot describe different requests.
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
