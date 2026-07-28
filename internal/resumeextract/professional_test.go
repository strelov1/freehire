package resumeextract

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// fullStructured is a Structured with every field populated, so a projection test can
// tell "the field was dropped" apart from "the field was empty anyway".
func fullStructured() Structured {
	return Structured{
		FullName:   "Ada Lovelace",
		Headline:   "Staff Backend Engineer",
		Location:   "Lisbon, PT",
		Email:      "ada@example.com",
		Phone:      "+351 900 000 000",
		Summary:    "Backend engineer with a decade on distributed systems.",
		TotalYears: 11,
		Experience: []Experience{{
			Title:      "Staff Engineer",
			Company:    "Analytical Engines",
			Location:   "Remote",
			Start:      "2021-03",
			End:        "Present",
			Summary:    "Payments platform.",
			Highlights: []string{"Cut p99 latency by half."},
			Stack:      []string{"Go", "PostgreSQL"},
		}},
		Education:      []Education{{Degree: "BSc Mathematics", Institution: "UCL", Year: "2012"}},
		Languages:      []string{"English", "Portuguese"},
		Links:          []string{"https://github.com/ada"},
		Skills:         []string{"Go", "Kafka"},
		Certifications: []string{"CKA"},
		Projects:       []Project{{Name: "difference-engine", Link: "https://example.com/de", Highlights: []string{"Toy VM."}}},
	}
}

func TestProfessional_OmitsContactFields(t *testing.T) {
	blob, err := json.Marshal(fullStructured().Professional())
	if err != nil {
		t.Fatalf("marshal projection: %v", err)
	}

	var got map[string]json.RawMessage
	if err := json.Unmarshal(blob, &got); err != nil {
		t.Fatalf("unmarshal projection: %v", err)
	}
	for _, key := range []string{"full_name", "email", "phone", "links"} {
		if _, present := got[key]; present {
			t.Errorf("projection carries contact field %q: %s", key, blob)
		}
	}
}

func TestProfessional_KeepsProfessionalFields(t *testing.T) {
	s := fullStructured()
	want := Professional{
		Headline:       s.Headline,
		Location:       s.Location,
		Summary:        s.Summary,
		TotalYears:     s.TotalYears,
		Experience:     s.Experience,
		Education:      s.Education,
		Languages:      s.Languages,
		Skills:         s.Skills,
		Certifications: s.Certifications,
		Projects:       s.Projects,
	}

	if got := s.Professional(); !reflect.DeepEqual(got, want) {
		t.Errorf("projection = %+v, want %+v", got, want)
	}
}

// TestProfessional_IsAWhitelist is the point of the type: a field added to Structured
// must not reach the projection until someone adds it there too. A blacklist (deleting
// known contact keys) would leak the new field instead. Enumerating the expected field
// set here makes that a test failure rather than a silent disclosure.
func TestProfessional_IsAWhitelist(t *testing.T) {
	want := []string{
		"headline", "location", "summary", "total_years", "experience",
		"education", "languages", "skills", "certifications", "projects",
	}

	typ := reflect.TypeOf(Professional{})
	got := make([]string, typ.NumField())
	for i := range typ.NumField() {
		got[i], _, _ = strings.Cut(typ.Field(i).Tag.Get("json"), ",")
	}
	slices.Sort(got)
	slices.Sort(want)

	if !slices.Equal(got, want) {
		t.Errorf("Professional fields = %v, want %v — a field reached the projection "+
			"without being reviewed for personal data", got, want)
	}
}
