package resumeextract

import (
	"encoding/json"
	"testing"
)

// noEndedExperience is a work history where no entry reads as current — every End is a
// concrete date. Deliberately NOT ordered by recency; the masking rule is content-based
// (the End label), so position must not matter to it.
func noEndedExperience() []Experience {
	return []Experience{
		{Title: "Engineer", Company: "Difference Co", Start: "2015-06", End: "2017-12"},
		{Title: "Senior Engineer", Company: "Babbage Systems", Start: "2018-01", End: "2021-02"},
	}
}

// oneCurrentExperience has exactly one entry reading as current, deliberately NOT at
// index 0, so a masking rule that (wrongly) relied on array position would mask the
// wrong entry here.
func oneCurrentExperience() []Experience {
	return []Experience{
		{Title: "Engineer", Company: "Difference Co", Start: "2015-06", End: "2017-12"},
		{Title: "Staff Engineer", Company: "Analytical Engines", Start: "2021-03", End: "Present"},
	}
}

// multipleCurrentExperience models two concurrent roles (End: "" and End: "CURRENT",
// case-varied and whitespace-padded to exercise the trim+lowercase normalization) plus
// one genuinely-ended role that must stay unmasked.
func multipleCurrentExperience() []Experience {
	return []Experience{
		{Title: "Freelance Consultant", Company: "Self-employed", Start: "2022-01", End: ""},
		{Title: "Engineer", Company: "Difference Co", Start: "2015-06", End: "2017-12"},
		{Title: "Advisor", Company: "Babbage Systems", Start: "2023-05", End: "  CURRENT  "},
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

func TestAnonymous_NoEntryCurrent_NothingMasked(t *testing.T) {
	s := fullStructured()
	s.Experience = noEndedExperience()

	got := s.Anonymous()

	if len(got.Experience) != 2 {
		t.Fatalf("Experience len = %d, want 2", len(got.Experience))
	}
	if got.Experience[0].Company != "Difference Co" {
		t.Errorf("Experience[0].Company = %q, want unchanged (\"Difference Co\")", got.Experience[0].Company)
	}
	if got.Experience[1].Company != "Babbage Systems" {
		t.Errorf("Experience[1].Company = %q, want unchanged (\"Babbage Systems\")", got.Experience[1].Company)
	}
}

func TestAnonymous_SingleCurrentEntry_OnlyThatMasked(t *testing.T) {
	s := fullStructured()
	s.Experience = oneCurrentExperience() // the current entry is at index 1, not 0

	got := s.Anonymous()

	if len(got.Experience) != 2 {
		t.Fatalf("Experience len = %d, want 2", len(got.Experience))
	}
	if got.Experience[0].Company != "Difference Co" {
		t.Errorf("ended Experience[0].Company = %q, want unchanged (\"Difference Co\")", got.Experience[0].Company)
	}
	if got.Experience[1].Company != currentEmployerLabel {
		t.Errorf("current Experience[1].Company = %q, want masked as %q", got.Experience[1].Company, currentEmployerLabel)
	}
	// Everything else on the masked entry is untouched.
	if got.Experience[1].Title != "Staff Engineer" {
		t.Errorf("Experience[1].Title = %q, want unchanged", got.Experience[1].Title)
	}
}

func TestAnonymous_MultipleCurrentEntries_AllMasked(t *testing.T) {
	s := fullStructured()
	s.Experience = multipleCurrentExperience()

	got := s.Anonymous()

	if len(got.Experience) != 3 {
		t.Fatalf("Experience len = %d, want 3", len(got.Experience))
	}
	if got.Experience[0].Company != currentEmployerLabel {
		t.Errorf("End=\"\" Experience[0].Company = %q, want masked as %q", got.Experience[0].Company, currentEmployerLabel)
	}
	if got.Experience[1].Company != "Difference Co" {
		t.Errorf("ended Experience[1].Company = %q, want unchanged (\"Difference Co\")", got.Experience[1].Company)
	}
	if got.Experience[2].Company != currentEmployerLabel {
		t.Errorf("End=\"  CURRENT  \" Experience[2].Company = %q, want masked as %q", got.Experience[2].Company, currentEmployerLabel)
	}
}

func TestAnonymous_DoesNotMutateSource(t *testing.T) {
	s := fullStructured()
	s.Experience = oneCurrentExperience()
	original := s.Experience[1].Company

	_ = s.Anonymous()

	if s.Experience[1].Company != original {
		t.Errorf("source Structured.Experience[1].Company = %q, mutated (want %q)", s.Experience[1].Company, original)
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

func TestPublic_ExperienceUnmodifiedIncludingCurrent(t *testing.T) {
	s := fullStructured()
	s.Experience = multipleCurrentExperience()

	got := s.Public()

	if len(got.Experience) != 3 {
		t.Fatalf("Experience len = %d, want 3", len(got.Experience))
	}
	if got.Experience[0].Company != "Self-employed" {
		t.Errorf("current Experience[0].Company = %q, want unchanged (\"Self-employed\")", got.Experience[0].Company)
	}
	if got.Experience[1].Company != "Difference Co" {
		t.Errorf("Experience[1].Company = %q, want unchanged", got.Experience[1].Company)
	}
	if got.Experience[2].Company != "Babbage Systems" {
		t.Errorf("current Experience[2].Company = %q, want unchanged (\"Babbage Systems\")", got.Experience[2].Company)
	}
}
