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
	err := Enrichment{Seniority: "sr"}.Validate()
	if err == nil {
		t.Fatal("expected error for seniority \"sr\"")
	}
	if !strings.Contains(err.Error(), "seniority") {
		t.Errorf("error must identify the offending field, got: %v", err)
	}
}

// When several enum fields are invalid, Validate reports the first one in
// declaration order (work_mode is checked before seniority).
func TestValidateReportsFirstOffender(t *testing.T) {
	err := Enrichment{WorkMode: "telepathic", Seniority: "sr"}.Validate()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "work_mode") {
		t.Errorf("want first offender work_mode, got: %v", err)
	}
	if strings.Contains(err.Error(), "seniority") {
		t.Errorf("should report only the first offender, got: %v", err)
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
		{WorkMode: "remote", Regions: []string{"eu", "emea"}},
		{WorkMode: "remote", Regions: []string{"us", "ru"}},
	}
	for i, e := range valid {
		if err := e.Validate(); err != nil {
			t.Errorf("case %d: expected valid, got error: %v", i, err)
		}
	}
}

func TestValidateRejectsRegionElement(t *testing.T) {
	err := Enrichment{Regions: []string{"eu", "europe"}}.Validate()
	if err == nil {
		t.Fatal("expected error for invalid region element")
	}
	if !strings.Contains(err.Error(), "regions") {
		t.Errorf("error must identify the offending field, got: %v", err)
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
		Seniority: "senior",                     // valid -> kept
		Category:  "astrology",                  // invalid scalar -> blanked
		Domains:   []string{"fintech", "bogus"}, // keep only known
		Regions:   []string{"nope"},             // all unknown -> nil
	}
	e.Sanitize()

	if e.Seniority != "senior" {
		t.Errorf("Seniority = %q, want it kept", e.Seniority)
	}
	if e.Category != "" {
		t.Errorf("Category = %q, want blanked", e.Category)
	}
	if len(e.Domains) != 1 || e.Domains[0] != "fintech" {
		t.Errorf("Domains = %v, want [fintech]", e.Domains)
	}
	if e.Regions != nil {
		t.Errorf("Regions = %v, want nil", e.Regions)
	}
	if err := e.Validate(); err != nil {
		t.Errorf("Validate after Sanitize = %v, want nil", err)
	}
}
