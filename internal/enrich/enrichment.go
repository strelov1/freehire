// Package enrich defines the structured, AI-derived field model for a job:
// the typed contract for the jobs.enrichment JSONB payload and the controlled
// vocabularies that pin down every enum field's allowed values.
//
// This package is the schema's source of truth. It contains no AI calls — only
// the contract. A later enrichment layer marshals an Enrichment into the JSONB
// column; a later search layer facets on these exact values. Keeping the
// vocabularies here as one definition prevents value fragmentation (e.g.
// "senior" vs "Senior" vs "sr") across those phases.
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
)

// Enrichment is the typed view of a job's enrichment JSONB payload. JSON keys
// are snake_case to match the existing jobs JSON tags. The blob maps 1:1 to the
// future search document.
type Enrichment struct {
	// Work arrangement.
	WorkMode        string `json:"work_mode,omitempty"`        // enum: WorkModeValues
	EmploymentType  string `json:"employment_type,omitempty"`  // enum: EmploymentTypeValues
	Relocation      string `json:"relocation,omitempty"`       // enum: RelocationValues
	VisaSponsorship *bool  `json:"visa_sponsorship,omitempty"` // pointer: false is meaningful

	// Location / eligibility. Regions is a remote role's geographic reach — a flat,
	// mixed-level vocabulary (global / macro-region / select country). It is
	// meaningful only when WorkMode is "remote". Empty means *unknown*; "global"
	// (open anywhere) is an explicit value, never inferred, so global ≠ unknown.
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

// Controlled vocabularies. Each is the ordered, canonical list of allowed
// values for one enum field. They are exported so a later enrichment prompt and
// a later facet config reference the same lists. ISO-standard fields
// (countries, salary_currency, posting_language) and the open skills field have
// no bundled closed vocabulary here and are not enum-validated in this phase.
var (
	WorkModeValues = []string{"remote", "hybrid", "onsite"}
	// RegionValues is the geographic-area vocabulary: global, macro-regions, and a
	// few countries treated as area codes (extend as the curated facet grows).
	// `cis` (post-Soviet space: Belarus, Moldova, the Caucasus) and `central_asia`
	// (the five Central Asian republics) cover the RU/CIS segment that dominates
	// the Telegram sources; `ru` stays its own area.
	RegionValues = []string{
		"global", "eu", "emea", "eea", "uk", "americas",
		"north_america", "latam", "apac", "mena", "africa", "us", "ru",
		"cis", "central_asia",
	}
	EmploymentTypeValues = []string{"full_time", "part_time", "contract", "internship"}
	RelocationValues     = []string{"not_supported", "supported", "required"}
	SalaryPeriodValues   = []string{"year", "month", "day", "hour"}
	SeniorityValues      = []string{"intern", "junior", "middle", "senior", "lead", "staff", "principal", "c_level"}
	EnglishLevelValues   = []string{"none", "a1", "a2", "b1", "b2", "c1", "c2", "native"}
	EducationLevelValues = []string{"none", "bachelor", "master", "phd"}
	CategoryValues       = []string{
		"backend", "frontend", "fullstack", "mobile", "devops", "sre",
		"data_engineering", "data_science", "data_analytics", "ml_ai",
		"qa", "security", "hardware", "embedded", "blockchain",
		"design", "product", "project_management", "management",
		"marketing", "sales", "support", "other",
	}
	DomainValues = []string{
		"fintech", "gambling", "ecommerce", "crypto", "healthcare",
		"saas", "gamedev", "edtech", "adtech", "govtech",
		"media", "travel", "logistics", "other",
	}
	CompanyTypeValues = []string{"product", "startup", "outsource", "outstaff", "agency", "inhouse", "government"}
	CompanySizeValues = []string{"1-10", "11-50", "51-200", "201-500", "501-1000", "1000+"}
)

// Validate checks every enum field against its controlled vocabulary and
// returns an error identifying the first offending field. Empty (absent) fields
// pass — every field is optional. Non-enum fields (ISO codes, free text,
// numbers, skills) are unconstrained here. Multi-value enum fields are checked
// element by element.
func (e Enrichment) Validate() error {
	// Single-value enum fields, in declaration order.
	scalars := []struct {
		field string
		value string
		vocab []string
	}{
		{"work_mode", e.WorkMode, WorkModeValues},
		{"employment_type", e.EmploymentType, EmploymentTypeValues},
		{"relocation", e.Relocation, RelocationValues},
		{"salary_period", e.SalaryPeriod, SalaryPeriodValues},
		{"seniority", e.Seniority, SeniorityValues},
		{"english_level", e.EnglishLevel, EnglishLevelValues},
		{"education_level", e.EducationLevel, EducationLevelValues},
		{"category", e.Category, CategoryValues},
		{"company_type", e.CompanyType, CompanyTypeValues},
		{"company_size", e.CompanySize, CompanySizeValues},
	}
	for _, s := range scalars {
		if s.value != "" && !slices.Contains(s.vocab, s.value) {
			return fmt.Errorf("enrich: invalid %s %q", s.field, s.value)
		}
	}

	// Multi-value enum fields, in declaration order.
	multi := []struct {
		field  string
		values []string
		vocab  []string
	}{
		{"regions", e.Regions, RegionValues},
		{"domains", e.Domains, DomainValues},
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

// Sanitize drops enum values the model emitted outside their controlled
// vocabulary: a scalar field is blanked, a multi-value field keeps only known
// members. This salvages the rest of an otherwise-good payload instead of
// dead-lettering the whole job over one stray value (the model occasionally
// invents a category/region no matter how the vocabulary grows). The invariant
// "never persist an out-of-vocabulary value" still holds — the value is dropped,
// not stored — so Validate passes afterwards.
func (e *Enrichment) Sanitize() {
	scalars := []struct {
		value *string
		vocab []string
	}{
		{&e.WorkMode, WorkModeValues},
		{&e.EmploymentType, EmploymentTypeValues},
		{&e.Relocation, RelocationValues},
		{&e.SalaryPeriod, SalaryPeriodValues},
		{&e.Seniority, SeniorityValues},
		{&e.EnglishLevel, EnglishLevelValues},
		{&e.EducationLevel, EducationLevelValues},
		{&e.Category, CategoryValues},
		{&e.CompanyType, CompanyTypeValues},
		{&e.CompanySize, CompanySizeValues},
	}
	for _, s := range scalars {
		if *s.value != "" && !slices.Contains(s.vocab, *s.value) {
			*s.value = ""
		}
	}

	e.Regions = keepKnown(e.Regions, RegionValues)
	e.Domains = keepKnown(e.Domains, DomainValues)

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
