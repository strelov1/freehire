// Package enrich is the typed contract for the jobs.enrichment JSONB payload. Controlled
// vocabularies live in internal/dict/vocab. It contains no AI calls.
//
// Every field is optional and omitted when the source does not state it. Fields whose zero
// value can be real (ints, bool) are pointers so absent differs from zero.
package enrich

import (
	"fmt"
	"slices"
	"strings"

	"github.com/strelov1/freehire/internal/dict/vocab"
	"github.com/strelov1/freehire/internal/platform/llm"
)

// maxSummaryRunes bounds the model-written Summary, which derives from the
// attacker-influenced description; Sanitize clips it.
const maxSummaryRunes = 400

// maxTimezoneNoteRunes and maxSalaryCurrencyRunes bound the other served free-text fields,
// clipped by Sanitize for the same reason as Summary (prompt-injection guard).
const (
	maxTimezoneNoteRunes   = 120
	maxSalaryCurrencyRunes = 12
)

// maxCities and maxCityRunes bound the served Cities list (jobview.cityFacet) against an
// injected "list every city in the world".
const (
	maxCities    = 20
	maxCityRunes = 100
)

// The enrichment model is the third producer of experience_years_min and shares the
// ceiling vocab.MaxExperienceYears. It is bounded here too because jobderive's bound sits
// on the write path and never sees this payload.

// maxRequirements and maxRequirementTextRunes bound the served Requirements list the same
// way.
const (
	maxRequirements         = 30
	maxRequirementTextRunes = 200
)

// Enrichment is the typed view of a job's enrichment JSONB payload (snake_case keys).
type Enrichment struct {
	// Summary is a 1-2 sentence synopsis. Unlike the other fields it is synthesized, so the
	// prompt always asks for it. Free text, bounded by Sanitize.
	Summary string `json:"summary,omitempty"`

	// Work arrangement.
	WorkMode        string `json:"work_mode,omitempty"`        // enum: WorkModeValues
	EmploymentType  string `json:"employment_type,omitempty"`  // enum: EmploymentTypeValues
	Relocation      string `json:"relocation,omitempty"`       // enum: RelocationValues
	VisaSponsorship *bool  `json:"visa_sponsorship,omitempty"` // pointer: false is meaningful

	// Location / eligibility. Regions is a remote role's reach (country-level lives in
	// Countries), meaningful only when WorkMode is "remote". Empty means unknown; "global"
	// is explicit, never inferred.
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

	// Stated requirements (job-only, no CV; unrelated to matchanalysis.Requirement).
	// Priority is not schema-constrained: llmschema.Enum cannot reach a property of an array
	// of objects and would silently produce an inert constraint, so the prompt plus
	// Sanitize's coercion enforce it.
	Requirements []Requirement `json:"requirements,omitempty"`
}

// Requirement is one requirement stated in the posting, job-only and not compared against
// any CV (unlike matchanalysis.Requirement).
type Requirement struct {
	Text     string `json:"text"`
	Priority string `json:"priority"` // "required" or "preferred"
}

// scalarEnum pairs a served scalar enum field (by pointer) with its vocabulary.
type scalarEnum struct {
	field string
	ptr   *string
	vocab []string
}

// servedScalarEnums lists the served single-value enum fields; Validate and Sanitize both
// read it. Dictionary-covered facets (work_mode, seniority, category, regions,
// employment_type, education_level, english_level) are absent: jobview serves them from
// the dictionaries, so the LLM's values are unserved discovery material.
func (e *Enrichment) servedScalarEnums() []scalarEnum {
	return []scalarEnum{
		{"relocation", &e.Relocation, vocab.RelocationValues},
		{"salary_period", &e.SalaryPeriod, vocab.SalaryPeriodValues},
		{"company_type", &e.CompanyType, vocab.CompanyTypeValues},
		{"company_size", &e.CompanySize, vocab.CompanySizeValues},
	}
}

// Validate checks every served enum field against its vocabulary and returns an error for
// the first offender. Empty fields pass. Dictionary-covered facets, countries and skills
// are not validated: an out-of-vocabulary value is captured raw, not rejected.
func (e Enrichment) Validate() error {
	// Value receiver: take the address of the local copy to reuse the shared field set.
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

// Sanitize drops out-of-vocabulary values from the served enum fields (a scalar is blanked,
// a multi-value field keeps known members) so none reaches the wire. Dictionary-covered
// facets are left raw as discovery material; Validate passes afterwards.
func (e *Enrichment) Sanitize() {
	// Clip free text that derives from the untrusted description.
	e.Summary = llm.TrimTruncateRunes(e.Summary, maxSummaryRunes)
	e.TimezoneNote = llm.TrimTruncateRunes(e.TimezoneNote, maxTimezoneNoteRunes)
	e.SalaryCurrency = llm.TrimTruncateRunes(e.SalaryCurrency, maxSalaryCurrencyRunes)
	e.Cities = boundCities(e.Cities)
	e.Requirements = BoundRequirements(e.Requirements)

	for _, s := range e.servedScalarEnums() {
		if *s.ptr != "" && !slices.Contains(s.vocab, *s.ptr) {
			*s.ptr = ""
		}
	}

	// category is asked now (#4412) and search reads it where the dictionary column is empty, so
	// an out-of-vocabulary answer (a gateway that does not enforce the schema) is dropped here.
	// Validate still lets it through: a bad category must not fail the whole enrichment.
	if e.Category != "" && !slices.Contains(vocab.CategoryValues, e.Category) {
		e.Category = ""
	}

	// regions is a discovery facet, left raw.
	e.Domains = keepKnown(e.Domains, vocab.DomainValues)

	// Drop non-positive salaries and inverted min>max pairs. No upper bound: high-denomination
	// currencies (CLP, IDR, HUF) make millions normal.
	e.SalaryMin = positiveOrNil(e.SalaryMin)
	e.SalaryMax = positiveOrNil(e.SalaryMax)
	if e.SalaryMin != nil && e.SalaryMax != nil && *e.SalaryMin > *e.SalaryMax {
		e.SalaryMin, e.SalaryMax = nil, nil
	}

	// Unlike salary, years take an absolute ceiling: a career has a length.
	e.ExperienceYearsMin = believableYears(e.ExperienceYearsMin)
}

// believableYears drops a years-of-experience figure that is negative or past
// vocab.MaxExperienceYears, leaving the field unstated rather than clamped.
func believableYears(n *int) *int {
	if n == nil || (*n >= 0 && *n <= vocab.MaxExperienceYears) {
		return n
	}
	return nil
}

// positiveOrNil drops a non-positive salary figure to nil.
func positiveOrNil(n *int) *int {
	if n == nil || *n > 0 {
		return n
	}
	return nil
}

// boundCities clips each city to maxCityRunes and the list to maxCities, dropping entries
// that clip to empty. Returns nil when nothing survives.
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

// BoundRequirements clips each text to maxRequirementTextRunes and the list to
// maxRequirements, dropping empty entries, and coerces Priority to required/preferred (the
// request schema cannot constrain it). Returns nil when nothing survives.
//
// Exported so the deterministic extractor of a posting's own requirements obeys the same
// ceiling.
func BoundRequirements(reqs []Requirement) []Requirement {
	if len(reqs) > maxRequirements {
		reqs = reqs[:maxRequirements]
	}
	var kept []Requirement
	for _, r := range reqs {
		r.Text = llm.TrimTruncateRunes(r.Text, maxRequirementTextRunes)
		if r.Text == "" {
			continue
		}
		r.Priority = coerceRequirementPriority(r.Priority)
		kept = append(kept, r)
	}
	return kept
}

// coerceRequirementPriority maps anything but "required" to "preferred", as
// matchanalysis.coercePriority does; duplicated because enrich must not import matchanalysis.
func coerceRequirementPriority(p string) string {
	if strings.EqualFold(strings.TrimSpace(p), "required") {
		return "required"
	}
	return "preferred"
}

// keepKnown returns values present in vocab, in order; nil when none survive.
func keepKnown(values, vocab []string) []string {
	var kept []string
	for _, v := range values {
		if slices.Contains(vocab, v) {
			kept = append(kept, v)
		}
	}
	return kept
}
