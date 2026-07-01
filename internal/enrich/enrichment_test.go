package enrich

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func ptr[T any](v T) *T { return &v }

// A fully-populated value must survive a JSON marshal/unmarshal round trip
// unchanged. reflect.DeepEqual follows pointers, so pointer fields compare by
// pointed-to value.
func TestRoundTripFidelity(t *testing.T) {
	original := Enrichment{
		WorkMode:           "remote",
		EmploymentType:     "full_time",
		Relocation:         "supported",
		VisaSponsorship:    ptr(true),
		Regions:            []string{"eu"},
		Countries:          []string{"US", "DE"},
		Cities:             []string{"Berlin"},
		TimezoneNote:       "UTC±2 overlap",
		SalaryMin:          ptr(80000),
		SalaryMax:          ptr(120000),
		SalaryCurrency:     "USD",
		SalaryPeriod:       "year",
		Seniority:          "senior",
		ExperienceYearsMin: ptr(5),
		EnglishLevel:       "b2",
		EducationLevel:     "bachelor",
		Skills:             []string{"go", "postgresql"},
		Category:           "backend",
		Domains:            []string{"fintech", "saas"},
		PostingLanguage:    "en",
		CompanyType:        "product",
		CompanySize:        "51-200",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var got Enrichment
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if !reflect.DeepEqual(original, got) {
		t.Errorf("round trip mismatch:\n original = %+v\n got      = %+v", original, got)
	}
}

// Undetermined fields must be omitted from the JSON, not serialized as zero/
// empty values. A present zero (e.g. experience 0) must NOT be omitted.
func TestOmitemptyOnUndeterminedFields(t *testing.T) {
	e := Enrichment{Seniority: "senior"}

	data, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(data)

	for _, key := range []string{
		"salary_min", "salary_max", "salary_currency", "salary_period",
		"work_mode", "visa_sponsorship", "experience_years_min", "skills",
	} {
		if strings.Contains(got, key) {
			t.Errorf("expected %q to be omitted, got: %s", key, got)
		}
	}
	if !strings.Contains(got, "seniority") {
		t.Errorf("expected seniority to be present, got: %s", got)
	}
}

// A present zero-valued int field is meaningful and must round-trip, not be
// dropped by omitempty (the reason those fields are pointers).
func TestZeroValuedPointerFieldIsPreserved(t *testing.T) {
	e := Enrichment{ExperienceYearsMin: ptr(0)}

	data, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(data), "experience_years_min") {
		t.Errorf("present zero must not be omitted, got: %s", data)
	}

	var got Enrichment
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.ExperienceYearsMin == nil || *got.ExperienceYearsMin != 0 {
		t.Errorf("expected experience_years_min = 0, got %v", got.ExperienceYearsMin)
	}
}

func TestValidateAccepts(t *testing.T) {
	valid := []Enrichment{
		{}, // empty payload: every field optional
		{Seniority: "senior", WorkMode: "remote", Skills: []string{"go", "postgresql"}},
		{Domains: []string{"fintech", "saas"}, Category: "backend"},
		// ISO and free-text fields are not enum-validated in this phase.
		{Countries: []string{"ZZ"}, SalaryCurrency: "XXX", PostingLanguage: "qq"},
	}
	for i, e := range valid {
		if err := e.Validate(); err != nil {
			t.Errorf("case %d: expected valid, got error: %v", i, err)
		}
	}
}

func TestValidateRejectsScalarEnum(t *testing.T) {
	// A SERVED enum field is still validated (relocation reaches the wire shape).
	err := Enrichment{Relocation: "seasonal"}.Validate()
	if err == nil {
		t.Fatal("expected error for relocation \"seasonal\"")
	}
	if !strings.Contains(err.Error(), "relocation") {
		t.Errorf("error must identify the offending field, got: %v", err)
	}
}

// When several SERVED enum fields are invalid, Validate reports the first one in
// declaration order (relocation is checked before english_level).
func TestValidateReportsFirstOffender(t *testing.T) {
	err := Enrichment{Relocation: "telepathic", EnglishLevel: "sr"}.Validate()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "relocation") {
		t.Errorf("want first offender relocation, got: %v", err)
	}
	if strings.Contains(err.Error(), "english_level") {
		t.Errorf("should report only the first offender, got: %v", err)
	}
}

// Relax: the dict-covered discovery facets are captured raw — an out-of-vocab
// work_mode/seniority/category/employment_type/education_level and a non-vocab
// regions element pass Validate (they are unserved, so they cannot corrupt
// production data).
func TestValidateAcceptsOutOfVocabDiscoveryFacets(t *testing.T) {
	discovery := []Enrichment{
		{Seniority: "staff_plus"},
		{Category: "ml_platform"},
		{WorkMode: "remote_first"},
		{Regions: []string{"eu", "europe"}},
		{EmploymentType: "freelance"},
		{EducationLevel: "associate"},
	}
	for i, e := range discovery {
		if err := e.Validate(); err != nil {
			t.Errorf("case %d: discovery facet should pass Validate, got: %v", i, err)
		}
	}
}

func TestValidateRejectsMultiEnumElement(t *testing.T) {
	err := Enrichment{Domains: []string{"fintech", "not_a_domain"}}.Validate()
	if err == nil {
		t.Fatal("expected error for invalid domain element")
	}
	if !strings.Contains(err.Error(), "domains") {
		t.Errorf("error must identify the offending field, got: %v", err)
	}
}

func TestValidateAcceptsRegions(t *testing.T) {
	valid := []Enrichment{
		{WorkMode: "remote", Regions: []string{"global"}},
		{WorkMode: "remote", Regions: []string{"eu", "mena"}},
		{WorkMode: "remote", Regions: []string{"north_america", "cis"}},
	}
	for i, e := range valid {
		if err := e.Validate(); err != nil {
			t.Errorf("case %d: expected valid, got error: %v", i, err)
		}
	}
}

// Global reach must be distinguishable from unknown reach: an explicit "global"
// region serializes the key, an unknown (empty regions) payload omits it.
func TestGlobalReachDistinctFromUnknown(t *testing.T) {
	global, err := json.Marshal(Enrichment{WorkMode: "remote", Regions: []string{"global"}})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(global), "regions") {
		t.Errorf("explicit global must serialize regions, got: %s", global)
	}

	unknown, err := json.Marshal(Enrichment{WorkMode: "remote"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(unknown), "regions") {
		t.Errorf("unknown reach must omit regions, got: %s", unknown)
	}
}

func TestSanitizeDropsOutOfVocabValues(t *testing.T) {
	e := Enrichment{
		Relocation:   "seasonal",                   // SERVED invalid scalar -> blanked
		EnglishLevel: "b2",                         // SERVED valid -> kept
		Domains:      []string{"fintech", "bogus"}, // SERVED multi -> keep only known
		// Discovery facets are captured raw (unserved):
		Category:       "astrology",      // kept
		Seniority:      "staff_plus",     // kept
		Regions:        []string{"nope"}, // kept
		EmploymentType: "freelance",      // kept
		EducationLevel: "associate",      // kept
	}
	e.Sanitize()

	if e.Relocation != "" {
		t.Errorf("Relocation = %q, want blanked (served field)", e.Relocation)
	}
	if e.EnglishLevel != "b2" {
		t.Errorf("EnglishLevel = %q, want kept", e.EnglishLevel)
	}
	if len(e.Domains) != 1 || e.Domains[0] != "fintech" {
		t.Errorf("Domains = %v, want [fintech] (served multi filtered)", e.Domains)
	}
	// Discovery facets survive untouched.
	if e.Category != "astrology" {
		t.Errorf("Category = %q, want kept raw (discovery)", e.Category)
	}
	if e.Seniority != "staff_plus" {
		t.Errorf("Seniority = %q, want kept raw (discovery)", e.Seniority)
	}
	if len(e.Regions) != 1 || e.Regions[0] != "nope" {
		t.Errorf("Regions = %v, want kept raw (discovery)", e.Regions)
	}
	if e.EmploymentType != "freelance" {
		t.Errorf("EmploymentType = %q, want kept raw (discovery)", e.EmploymentType)
	}
	if e.EducationLevel != "associate" {
		t.Errorf("EducationLevel = %q, want kept raw (discovery)", e.EducationLevel)
	}
	if err := e.Validate(); err != nil {
		t.Errorf("Validate after Sanitize = %v, want nil", err)
	}
}

func TestSanitizeDropsImplausibleSalary(t *testing.T) {
	t.Run("non-positive salary is dropped", func(t *testing.T) {
		e := Enrichment{SalaryMin: ptr(-1), SalaryMax: ptr(0)}
		e.Sanitize()
		if e.SalaryMin != nil {
			t.Errorf("SalaryMin = %v, want nil (negative dropped)", *e.SalaryMin)
		}
		if e.SalaryMax != nil {
			t.Errorf("SalaryMax = %v, want nil (zero dropped)", *e.SalaryMax)
		}
	})

	t.Run("valid positive salary kept, including high-denomination currencies", func(t *testing.T) {
		// 58.8M CLP/year (~$61k) is a real salary — no absolute upper clamp.
		e := Enrichment{SalaryMin: ptr(49_980_000), SalaryMax: ptr(58_800_000), SalaryCurrency: "CLP"}
		e.Sanitize()
		if e.SalaryMin == nil || *e.SalaryMin != 49_980_000 {
			t.Errorf("SalaryMin = %v, want kept", e.SalaryMin)
		}
		if e.SalaryMax == nil || *e.SalaryMax != 58_800_000 {
			t.Errorf("SalaryMax = %v, want kept", e.SalaryMax)
		}
	})

	t.Run("inconsistent min > max drops both", func(t *testing.T) {
		e := Enrichment{SalaryMin: ptr(200_000), SalaryMax: ptr(100_000)}
		e.Sanitize()
		if e.SalaryMin != nil || e.SalaryMax != nil {
			t.Errorf("min>max should drop both, got min=%v max=%v", e.SalaryMin, e.SalaryMax)
		}
	})

	t.Run("min alone or max alone is preserved when positive", func(t *testing.T) {
		e := Enrichment{SalaryMax: ptr(120_000)}
		e.Sanitize()
		if e.SalaryMax == nil || *e.SalaryMax != 120_000 {
			t.Errorf("lone positive SalaryMax = %v, want kept", e.SalaryMax)
		}
	})
}
