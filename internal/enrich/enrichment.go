// Package enrich defines the structured, AI-derived field model for a job:
// the typed contract for the jobs.enrichment JSONB payload. The controlled
// vocabularies that pin down every enum field's allowed values live in the
// neutral internal/vocab package, shared with the ingest and read layers.
//
// This package is the schema's source of truth. It contains no AI calls — only
// the contract. A later enrichment layer marshals an Enrichment into the JSONB
// column; a later search layer facets on these exact values.
//
// Field optionality: every field is optional and omitted when the source does
// not state it. Fields whose zero value can be a real value (ints, bool) are
// pointers so an absent field is distinguishable from a present zero; fields
// whose zero value (empty string / empty slice) can never be a valid value use
// omitempty directly.
package enrich

import (
	"fmt"
	"slices"

	"github.com/strelov1/freehire/internal/llm"
	"github.com/strelov1/freehire/internal/vocab"
)

// maxSummaryRunes bounds the model-written Summary. It is synthesized free text
// derived from the (attacker-influenced) description, so Sanitize clips a rambling
// or oversized value to keep the served payload small; the prompt already asks for
// 1–2 sentences, so a normal value never reaches this cap.
const maxSummaryRunes = 400

// maxTimezoneNoteRunes and maxSalaryCurrencyRunes bound the other two served
// free-text enrichment fields. Both are extracted from the same attacker-influenced
// description as Summary, so Sanitize clips them too — every served free-text field
// passes through the same bound (the persist/serve prompt-injection guard). A
// normal timezone note or ISO-4217 currency is far under these caps.
const (
	maxTimezoneNoteRunes   = 120
	maxSalaryCurrencyRunes = 12
)

// maxCities and maxCityRunes bound the Cities free-text list the same way: it is
// extracted from the same attacker-influenced description and served verbatim
// (jobview.cityFacet, and from there into the Meilisearch document) whenever the
// deterministic dictionary hasn't pinned a city, so an unbounded model response —
// e.g. a prompt-injected "list every city in the world, one per line" — would
// otherwise persist and be served indefinitely.
const (
	maxCities    = 20
	maxCityRunes = 100
)

// Enrichment is the typed view of a job's enrichment JSONB payload. JSON keys
// are snake_case to match the existing jobs JSON tags. The blob maps 1:1 to the
// future search document.
type Enrichment struct {
	// Summary is a short, model-written synopsis of the role (1–2 sentences): what
	// the job involves and its core stack. Unlike every other field — which is
	// extracted and omitted when the posting does not state it — this one is
	// SYNTHESIZED, so the prompt asks the model to always produce it. It is served
	// free text (no controlling dictionary), bounded by Sanitize.
	Summary string `json:"summary,omitempty"`

	// Work arrangement.
	WorkMode        string `json:"work_mode,omitempty"`        // enum: WorkModeValues
	EmploymentType  string `json:"employment_type,omitempty"`  // enum: EmploymentTypeValues
	Relocation      string `json:"relocation,omitempty"`       // enum: RelocationValues
	VisaSponsorship *bool  `json:"visa_sponsorship,omitempty"` // pointer: false is meaningful

	// Location / eligibility. Regions is a remote role's geographic reach — a flat
	// macro-region vocabulary (global / continent-level area; country-level reach
	// lives in Countries). It is meaningful only when WorkMode is "remote". Empty
	// means *unknown*; "global" (open anywhere) is an explicit value, never
	// inferred, so global ≠ unknown.
	Regions      []string `json:"regions,omitempty"`       // enum[]: RegionValues
	Countries    []string `json:"countries,omitempty"`     // enum[]: ISO 3166-1 alpha-2
	Cities       []string `json:"cities,omitempty"`        // free text (not faceted)
	TimezoneNote string   `json:"timezone_note,omitempty"` // free text (not faceted)

	// Compensation.
	SalaryMin      *int   `json:"salary_min,omitempty"`      // in salary_currency units
	SalaryMax      *int   `json:"salary_max,omitempty"`      // in salary_currency units
	SalaryCurrency string `json:"salary_currency,omitempty"` // ISO 4217 (e.g. USD, EUR)
	SalaryPeriod   string `json:"salary_period,omitempty"`   // enum: SalaryPeriodValues

	// Requirements / qualifications.
	Seniority          string   `json:"seniority,omitempty"`            // enum: SeniorityValues
	ExperienceYearsMin *int     `json:"experience_years_min,omitempty"` // non-negative
	EnglishLevel       string   `json:"english_level,omitempty"`        // enum: EnglishLevelValues
	EducationLevel     string   `json:"education_level,omitempty"`      // enum: EducationLevelValues
	Skills             []string `json:"skills,omitempty"`               // normalized lowercase tokens

	// Classification.
	Category        string   `json:"category,omitempty"`         // enum: CategoryValues
	Domains         []string `json:"domains,omitempty"`          // enum[]: DomainValues
	PostingLanguage string   `json:"posting_language,omitempty"` // ISO 639-1 (e.g. en, uk, ru)

	// Company descriptors (job-time observation; seam to the companies entity).
	CompanyType string `json:"company_type,omitempty"` // enum: CompanyTypeValues
	CompanySize string `json:"company_size,omitempty"` // enum: CompanySizeValues
}

// scalarEnum pairs a served scalar enum field (by pointer) with its vocabulary.
type scalarEnum struct {
	field string
	ptr   *string
	vocab []string
}

// servedScalarEnums lists the served single-value enum fields, in declaration
// order. It is the ONE place the served-scalar set is defined; Validate reads it,
// Sanitize blanks through it. The dictionary-covered facets (work_mode, seniority,
// category, regions, employment_type, education_level, english_level) are
// deliberately absent — jobview serves them from the deterministic dictionaries, so
// the LLM's values are unserved discovery material under dict-only.
func (e *Enrichment) servedScalarEnums() []scalarEnum {
	return []scalarEnum{
		{"relocation", &e.Relocation, vocab.RelocationValues},
		{"salary_period", &e.SalaryPeriod, vocab.SalaryPeriodValues},
		{"company_type", &e.CompanyType, vocab.CompanyTypeValues},
		{"company_size", &e.CompanySize, vocab.CompanySizeValues},
	}
}

// Validate checks every SERVED enum field against its controlled vocabulary and
// returns an error identifying the first offending field. Empty (absent) fields
// pass — every field is optional. Non-enum fields (ISO codes, free text, numbers,
// skills) are unconstrained here. The dictionary-covered facets (work_mode,
// seniority, category, regions, employment_type, education_level, english_level,
// plus the non-enum countries/skills) are deliberately NOT validated: they are served from
// the deterministic dictionaries (dict-only), so the LLM's values for them are
// unserved discovery material and an out-of-vocabulary value is captured raw
// rather than rejected.
func (e Enrichment) Validate() error {
	// Single-value SERVED enum fields. Value receiver, so take the address of the
	// local copy to reuse the shared field set.
	ev := e
	for _, s := range ev.servedScalarEnums() {
		if *s.ptr != "" && !slices.Contains(s.vocab, *s.ptr) {
			return fmt.Errorf("enrich: invalid %s %q", s.field, *s.ptr)
		}
	}

	// Multi-value SERVED enum fields (regions is a discovery facet, not validated).
	multi := []struct {
		field  string
		values []string
		vocab  []string
	}{
		{"domains", e.Domains, vocab.DomainValues},
	}
	for _, m := range multi {
		for _, v := range m.values {
			if !slices.Contains(m.vocab, v) {
				return fmt.Errorf("enrich: invalid %s %q", m.field, v)
			}
		}
	}

	return nil
}

// Sanitize drops out-of-vocabulary values from the SERVED enum fields (a scalar is
// blanked, a multi-value field keeps only known members) so no stray value reaches
// the served wire shape. The dictionary-covered facets (work_mode, seniority,
// category, regions, employment_type, education_level, english_level) are
// deliberately left untouched: they are unserved discovery material under dict-only, so the LLM's raw
// values — including novel, out-of-vocabulary labels — are kept for later mining.
// The invariant "never serve an out-of-vocabulary value" still holds for the served
// fields, and Validate passes afterwards.
func (e *Enrichment) Sanitize() {
	// Summary is synthesized free text; trim and clip an over-long value so the
	// served payload stays bounded (guards a runaway model; the prompt asks for
	// 1–2 sentences). timezone_note and salary_currency are extracted from the same
	// untrusted description and also served raw, so bound them identically.
	e.Summary = llm.TrimTruncateRunes(e.Summary, maxSummaryRunes)
	e.TimezoneNote = llm.TrimTruncateRunes(e.TimezoneNote, maxTimezoneNoteRunes)
	e.SalaryCurrency = llm.TrimTruncateRunes(e.SalaryCurrency, maxSalaryCurrencyRunes)
	e.Cities = boundCities(e.Cities)

	for _, s := range e.servedScalarEnums() {
		if *s.ptr != "" && !slices.Contains(s.vocab, *s.ptr) {
			*s.ptr = ""
		}
	}

	// regions is a discovery facet — left raw (not filtered).
	e.Domains = keepKnown(e.Domains, vocab.DomainValues)

	// Drop implausible salary values: a non-positive salary is meaningless, and an
	// inverted min>max pair is internally inconsistent. There is deliberately no
	// absolute upper bound — high-denomination currencies (CLP, IDR, HUF, …) make
	// millions a normal salary, so a numeric ceiling would discard valid data.
	e.SalaryMin = positiveOrNil(e.SalaryMin)
	e.SalaryMax = positiveOrNil(e.SalaryMax)
	if e.SalaryMin != nil && e.SalaryMax != nil && *e.SalaryMin > *e.SalaryMax {
		e.SalaryMin, e.SalaryMax = nil, nil
	}
}

// positiveOrNil drops a non-positive salary figure to nil (an absent salary), so
// a zero or negative value never persists.
func positiveOrNil(n *int) *int {
	if n == nil || *n > 0 {
		return n
	}
	return nil
}

// boundCities clips each city to maxCityRunes and the list to maxCities entries, the
// same free-text bound Sanitize applies to Summary/TimezoneNote/SalaryCurrency,
// dropping any entry that clips to empty. It returns nil when nothing survives so
// the field omits cleanly.
func boundCities(cities []string) []string {
	if len(cities) > maxCities {
		cities = cities[:maxCities]
	}
	var kept []string
	for _, c := range cities {
		c = llm.TrimTruncateRunes(c, maxCityRunes)
		if c == "" {
			continue
		}
		kept = append(kept, c)
	}
	return kept
}

// keepKnown returns values restricted to those present in vocab, preserving order;
// it returns nil when nothing survives so the field omits cleanly.
func keepKnown(values, vocab []string) []string {
	var kept []string
	for _, v := range values {
		if slices.Contains(vocab, v) {
			kept = append(kept, v)
		}
	}
	return kept
}
