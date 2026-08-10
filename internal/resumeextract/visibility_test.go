package resumeextract

import (
	"encoding/json"
	"testing"
)

// multiExperience returns experience newest-first (index 0 is the current role),
// mirroring the reverse-chronological convention CVs are written in and the same
// convention internal/experience.Store.ListEmployments enforces for the bank.
func multiExperience() []Experience {
	return []Experience{
		{Title: "Staff Engineer", Company: "Analytical Engines", Start: "2021-03", End: "Present"},
		{Title: "Senior Engineer", Company: "Babbage Systems", Start: "2018-01", End: "2021-02"},
		{Title: "Engineer", Company: "Difference Co", Start: "2015-06", End: "2017-12"},
	}
}

func TestAnonymous_ZeroExperience(t *testing.T) {
	s := fullStructured()
	s.Experience = nil

	got := s.Anonymous()

	if len(got.Experience) != 0 {
		t.Errorf("Experience = %v, want empty", got.Experience)
	}
}

func TestAnonymous_SingleEntryMasked(t *testing.T) {
	s := fullStructured() // has exactly one experience entry, company "Analytical Engines"

	got := s.Anonymous()

	if len(got.Experience) != 1 {
		t.Fatalf("Experience len = %d, want 1", len(got.Experience))
	}
	if got.Experience[0].Company != currentEmployerLabel {
		t.Errorf("Experience[0].Company = %q, want %q", got.Experience[0].Company, currentEmployerLabel)
	}
	// Everything else on the entry is untouched.
	if got.Experience[0].Title != "Staff Engineer" {
		t.Errorf("Experience[0].Title = %q, want unchanged", got.Experience[0].Title)
	}
}

func TestAnonymous_MultipleEntriesOnlyNewestMasked(t *testing.T) {
	s := fullStructured()
	s.Experience = multiExperience()

	got := s.Anonymous()

	if len(got.Experience) != 3 {
		t.Fatalf("Experience len = %d, want 3", len(got.Experience))
	}
	if got.Experience[0].Company != currentEmployerLabel {
		t.Errorf("newest Experience[0].Company = %q, want masked as %q", got.Experience[0].Company, currentEmployerLabel)
	}
	if got.Experience[1].Company != "Babbage Systems" {
		t.Errorf("Experience[1].Company = %q, want unchanged (\"Babbage Systems\")", got.Experience[1].Company)
	}
	if got.Experience[2].Company != "Difference Co" {
		t.Errorf("Experience[2].Company = %q, want unchanged (\"Difference Co\")", got.Experience[2].Company)
	}
}

func TestAnonymous_DoesNotMutateSource(t *testing.T) {
	s := fullStructured()
	s.Experience = multiExperience()
	original := s.Experience[0].Company

	_ = s.Anonymous()

	if s.Experience[0].Company != original {
		t.Errorf("source Structured.Experience[0].Company = %q, mutated (want %q)", s.Experience[0].Company, original)
	}
}

func TestAnonymous_OmitsContactFields(t *testing.T) {
	blob, err := json.Marshal(fullStructured().Anonymous())
	if err != nil {
		t.Fatalf("marshal projection: %v", err)
	}

	var got map[string]json.RawMessage
	if err := json.Unmarshal(blob, &got); err != nil {
		t.Fatalf("unmarshal projection: %v", err)
	}
	for _, key := range []string{"full_name", "email", "phone", "links"} {
		if _, present := got[key]; present {
			t.Errorf("anonymous projection carries contact field %q: %s", key, blob)
		}
	}
}

func TestPublic_KeepsName(t *testing.T) {
	s := fullStructured()

	got := s.Public()

	if got.FullName != s.FullName {
		t.Errorf("FullName = %q, want %q", got.FullName, s.FullName)
	}
}

func TestPublic_OmitsContactFields(t *testing.T) {
	blob, err := json.Marshal(fullStructured().Public())
	if err != nil {
		t.Fatalf("marshal projection: %v", err)
	}

	var got map[string]json.RawMessage
	if err := json.Unmarshal(blob, &got); err != nil {
		t.Fatalf("unmarshal projection: %v", err)
	}
	for _, key := range []string{"email", "phone", "links"} {
		if _, present := got[key]; present {
			t.Errorf("public projection carries contact field %q: %s", key, blob)
		}
	}
	if _, present := got["full_name"]; !present {
		t.Errorf("public projection is missing full_name: %s", blob)
	}
}

func TestPublic_ExperienceUnmodifiedIncludingNewest(t *testing.T) {
	s := fullStructured()
	s.Experience = multiExperience()

	got := s.Public()

	if len(got.Experience) != 3 {
		t.Fatalf("Experience len = %d, want 3", len(got.Experience))
	}
	if got.Experience[0].Company != "Analytical Engines" {
		t.Errorf("newest Experience[0].Company = %q, want unchanged (\"Analytical Engines\")", got.Experience[0].Company)
	}
	if got.Experience[1].Company != "Babbage Systems" {
		t.Errorf("Experience[1].Company = %q, want unchanged", got.Experience[1].Company)
	}
	if got.Experience[2].Company != "Difference Co" {
		t.Errorf("Experience[2].Company = %q, want unchanged", got.Experience[2].Company)
	}
}
