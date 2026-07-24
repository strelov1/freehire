package enrich

import (
	"encoding/json"
	"math"
)

// Tolerant JSON decoding for Enrichment. The model reliably picks the right
// *value* but occasionally wraps it in the wrong *shape*: a scalar where the
// contract wants an array ("eu" vs ["eu"]), an array where it wants a scalar
// (["senior"] vs "senior"), or a float where it wants an int (17.5 vs 17). A
// strict json.Unmarshal rejects the whole payload over one such wrapper, burning
// an outbox attempt on data that was otherwise fine. UnmarshalJSON below coerces
// each mis-shaped field to its canonical form so the value survives.
//
// The exported Enrichment field types are unchanged (string / []string / *int);
// the tolerance lives only in this decode boundary. Sanitize/Validate then run on
// the canonical shapes exactly as before.

// stringOrFirst decodes a JSON string, or an array of strings (taking the first
// element — an empty array yields an empty string). This "first element wins" rule is
// specific to the model's scalar-vs-array slips; flexjson has no string type, so there
// is deliberately no shared implementation.
type stringOrFirst string

func (f *stringOrFirst) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		*f = stringOrFirst(s)
		return nil
	}
	var arr []string
	if err := json.Unmarshal(b, &arr); err != nil {
		return err
	}
	if len(arr) > 0 {
		*f = stringOrFirst(arr[0])
	}
	return nil
}

// sliceOrWrap decodes a JSON array of strings, or a single string (wrapped
// into a one-element slice — an empty string yields an empty slice).
type sliceOrWrap []string

func (f *sliceOrWrap) UnmarshalJSON(b []byte) error {
	var arr []string
	if err := json.Unmarshal(b, &arr); err == nil {
		*f = arr
		return nil
	}
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	if s != "" {
		*f = sliceOrWrap{s}
	}
	return nil
}

// roundInt decodes any JSON number, rounding to the nearest integer so a
// fractional value (an hourly rate like 17.5, or 0.5 years) is preserved instead
// of failing the decode. It is deliberately STRICTER than flexjson.Int: a string
// ("100k") fails the whole decode here, so the runner treats the response as
// unparseable and re-samples — flexjson.Int would silently coerce it to a
// plausible-looking 100 (and garbage to 0).
type roundInt int

func (f *roundInt) UnmarshalJSON(b []byte) error {
	var n float64
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	*f = roundInt(math.Round(n))
	return nil
}

// enrichmentJSON mirrors Enrichment field-for-field (identical json tags) but
// types every string/[]string/int field as its flex* counterpart, so decoding
// tolerates the model's shape slips. It is a distinct type from Enrichment, so
// Enrichment.UnmarshalJSON does not recurse.
type enrichmentJSON struct {
	// Summary is plain free text — no shape coercion, but it must be listed here (and
	// copied below) or the tolerant decode silently drops it.
	Summary string `json:"summary,omitempty"`

	WorkMode        stringOrFirst `json:"work_mode,omitempty"`
	EmploymentType  stringOrFirst `json:"employment_type,omitempty"`
	Relocation      stringOrFirst `json:"relocation,omitempty"`
	VisaSponsorship *bool      `json:"visa_sponsorship,omitempty"`

	Regions      sliceOrWrap `json:"regions,omitempty"`
	Countries    sliceOrWrap `json:"countries,omitempty"`
	Cities       sliceOrWrap `json:"cities,omitempty"`
	TimezoneNote stringOrFirst      `json:"timezone_note,omitempty"`

	SalaryMin      *roundInt   `json:"salary_min,omitempty"`
	SalaryMax      *roundInt   `json:"salary_max,omitempty"`
	SalaryCurrency stringOrFirst `json:"salary_currency,omitempty"`
	SalaryPeriod   stringOrFirst `json:"salary_period,omitempty"`

	Seniority          stringOrFirst      `json:"seniority,omitempty"`
	ExperienceYearsMin *roundInt        `json:"experience_years_min,omitempty"`
	EnglishLevel       stringOrFirst      `json:"english_level,omitempty"`
	EducationLevel     stringOrFirst      `json:"education_level,omitempty"`
	Skills             sliceOrWrap `json:"skills,omitempty"`

	Category        stringOrFirst      `json:"category,omitempty"`
	Domains         sliceOrWrap `json:"domains,omitempty"`
	PostingLanguage stringOrFirst      `json:"posting_language,omitempty"`

	CompanyType stringOrFirst `json:"company_type,omitempty"`
	CompanySize stringOrFirst `json:"company_size,omitempty"`
}

// UnmarshalJSON decodes into the tolerant shadow struct, then copies into the
// canonical Enrichment shapes. A genuinely malformed document still errors.
func (e *Enrichment) UnmarshalJSON(data []byte) error {
	var s enrichmentJSON
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	*e = Enrichment{
		Summary:            s.Summary,
		WorkMode:           string(s.WorkMode),
		EmploymentType:     string(s.EmploymentType),
		Relocation:         string(s.Relocation),
		VisaSponsorship:    s.VisaSponsorship,
		Regions:            []string(s.Regions),
		Countries:          []string(s.Countries),
		Cities:             []string(s.Cities),
		TimezoneNote:       string(s.TimezoneNote),
		SalaryMin:          roundIntPtr(s.SalaryMin),
		SalaryMax:          roundIntPtr(s.SalaryMax),
		SalaryCurrency:     string(s.SalaryCurrency),
		SalaryPeriod:       string(s.SalaryPeriod),
		Seniority:          string(s.Seniority),
		ExperienceYearsMin: roundIntPtr(s.ExperienceYearsMin),
		EnglishLevel:       string(s.EnglishLevel),
		EducationLevel:     string(s.EducationLevel),
		Skills:             []string(s.Skills),
		Category:           string(s.Category),
		Domains:            []string(s.Domains),
		PostingLanguage:    string(s.PostingLanguage),
		CompanyType:        string(s.CompanyType),
		CompanySize:        string(s.CompanySize),
	}
	return nil
}

// roundIntPtr converts an optional roundInt to an optional int, preserving absence.
func roundIntPtr(n *roundInt) *int {
	if n == nil {
		return nil
	}
	v := int(*n)
	return &v
}
