package resumeextract

import (
	"encoding/json"
	"reflect"
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
	p := s.Professional()

	if p.Headline != s.Headline {
		t.Errorf("Headline = %q, want %q", p.Headline, s.Headline)
	}
	if p.Location != s.Location {
		t.Errorf("Location = %q, want %q", p.Location, s.Location)
	}
	if p.Summary != s.Summary {
		t.Errorf("Summary = %q, want %q", p.Summary, s.Summary)
	}
	if p.TotalYears != s.TotalYears {
		t.Errorf("TotalYears = %d, want %d", p.TotalYears, s.TotalYears)
	}
	if !reflect.DeepEqual(p.Experience, s.Experience) {
		t.Errorf("Experience = %+v, want %+v", p.Experience, s.Experience)
	}
	if !reflect.DeepEqual(p.Education, s.Education) {
		t.Errorf("Education = %+v, want %+v", p.Education, s.Education)
	}
	if !reflect.DeepEqual(p.Languages, s.Languages) {
		t.Errorf("Languages = %v, want %v", p.Languages, s.Languages)
	}
	if !reflect.DeepEqual(p.Skills, s.Skills) {
		t.Errorf("Skills = %v, want %v", p.Skills, s.Skills)
	}
	if !reflect.DeepEqual(p.Certifications, s.Certifications) {
		t.Errorf("Certifications = %v, want %v", p.Certifications, s.Certifications)
	}
	if !reflect.DeepEqual(p.Projects, s.Projects) {
		t.Errorf("Projects = %+v, want %+v", p.Projects, s.Projects)
	}
}

// TestProfessional_IsAWhitelist is the point of the type: a field added to Structured
// must not reach the projection until someone adds it there too. A blacklist (deleting
// known contact keys) would leak the new field instead. Enumerating the expected field
// set here makes that a test failure rather than a silent disclosure.
func TestProfessional_IsAWhitelist(t *testing.T) {
	want := map[string]struct{}{
		"headline": {}, "location": {}, "summary": {}, "total_years": {},
		"experience": {}, "education": {}, "languages": {}, "skills": {},
		"certifications": {}, "projects": {},
	}

	typ := reflect.TypeOf(Professional{})
	got := make(map[string]struct{}, typ.NumField())
	for i := range typ.NumField() {
		name, _, _ := strings.Cut(typ.Field(i).Tag.Get("json"), ",")
		got[name] = struct{}{}
	}

	if !reflect.DeepEqual(got, want) {
		t.Errorf("Professional fields = %v, want %v — a field was added to the projection "+
			"without being reviewed for personal data", keys(got), keys(want))
	}
}

func keys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
