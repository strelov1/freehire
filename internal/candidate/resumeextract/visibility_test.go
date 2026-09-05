package resumeextract

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/candidate/perioddate"
)

// noEndedExperience is a work history where no entry reads as current — every entry has
// a concrete End and Current: false. Deliberately NOT ordered by recency; the masking
// rule is content-based (Current), so position must not matter to it.
func noEndedExperience() []Experience {
	return []Experience{
		{Title: "Engineer", Company: "Difference Co", Start: &perioddate.PeriodDate{Year: 2015, Month: 6}, End: &perioddate.PeriodDate{Year: 2017, Month: 12}},
		{Title: "Senior Engineer", Company: "Babbage Systems", Start: &perioddate.PeriodDate{Year: 2018, Month: 1}, End: &perioddate.PeriodDate{Year: 2021, Month: 2}},
	}
}

// oneCurrentExperience has exactly one entry reading as current, deliberately NOT at
// index 0, so a masking rule that (wrongly) relied on array position would mask the
// wrong entry here.
func oneCurrentExperience() []Experience {
	return []Experience{
		{Title: "Engineer", Company: "Difference Co", Start: &perioddate.PeriodDate{Year: 2015, Month: 6}, End: &perioddate.PeriodDate{Year: 2017, Month: 12}},
		{Title: "Staff Engineer", Company: "Analytical Engines", Start: &perioddate.PeriodDate{Year: 2021, Month: 3}, Current: true},
	}
}

// multipleCurrentExperience models two concurrent roles plus one genuinely-ended role
// that must stay unmasked.
func multipleCurrentExperience() []Experience {
	return []Experience{
		{Title: "Freelance Consultant", Company: "Self-employed", Start: &perioddate.PeriodDate{Year: 2022, Month: 1}, Current: true},
		{Title: "Engineer", Company: "Difference Co", Start: &perioddate.PeriodDate{Year: 2015, Month: 6}, End: &perioddate.PeriodDate{Year: 2017, Month: 12}},
		{Title: "Advisor", Company: "Babbage Systems", Start: &perioddate.PeriodDate{Year: 2023, Month: 5}, Current: true},
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
		t.Errorf("Current=true Experience[0].Company = %q, want masked as %q", got.Experience[0].Company, currentEmployerLabel)
	}
	if got.Experience[1].Company != "Difference Co" {
		t.Errorf("ended Experience[1].Company = %q, want unchanged (\"Difference Co\")", got.Experience[1].Company)
	}
	if got.Experience[2].Company != currentEmployerLabel {
		t.Errorf("Current=true Experience[2].Company = %q, want masked as %q", got.Experience[2].Company, currentEmployerLabel)
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

// TestAnonymous_StripsProjectLinks guards against a project link — e.g.
// "github.com/<handle>" or a personal portfolio domain — reaching the anonymous public
// projection. A project's Link is a stronger de-anonymizing identifier than the name
// that Professional() already strips, so it must not survive Anonymous() either. The
// project's other fields (name, highlights) are legitimate signal and must stay.
func TestAnonymous_StripsProjectLinks(t *testing.T) {
	s := fullStructured() // carries a project with a non-empty Link

	got := s.Anonymous()

	if len(got.Projects) == 0 {
		t.Fatalf("Projects is empty, fixture should carry at least one")
	}
	for _, p := range got.Projects {
		if p.Link != "" {
			t.Errorf("project %q carries Link %q, want stripped", p.Name, p.Link)
		}
		if p.Name == "" {
			t.Errorf("project lost its Name too — only Link should be stripped")
		}
	}

	blob, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal projection: %v", err)
	}
	if strings.Contains(string(blob), `"link"`) {
		t.Errorf("anonymous projection JSON still carries a project link key: %s", blob)
	}
}

// TestAnonymous_StripProjectLinksDoesNotMutateSource mirrors
// TestAnonymous_DoesNotMutateSource: stripping a project's Link must operate on a copy,
// not the backing array Professional()'s Projects slice shares with s.Projects.
func TestAnonymous_StripProjectLinksDoesNotMutateSource(t *testing.T) {
	s := fullStructured()
	original := s.Projects[0].Link

	_ = s.Anonymous()

	if s.Projects[0].Link != original {
		t.Errorf("source Structured.Projects[0].Link = %q, mutated (want %q)", s.Projects[0].Link, original)
	}
}

// TestAnonymous_IsAWhitelist mirrors TestProfessional_IsAWhitelist (professional_test.go)
// against Anonymous()'s actual output. Professional's own whitelist test guards fields
// reaching that internal projection (used by the LLM fit chain, the assistant, and the
// authenticated profile read) — it does NOT guard whether a field that's fine for those
// authenticated, internal consumers should also reach the UNAUTHENTICATED, public Talent
// Network route. This is that second gate: it fails the moment a field is added to
// Professional without anyone re-reviewing that public exposure.
func TestAnonymous_IsAWhitelist(t *testing.T) {
	blob, err := json.Marshal(fullStructured().Anonymous())
	if err != nil {
		t.Fatalf("marshal projection: %v", err)
	}

	var got map[string]json.RawMessage
	if err := json.Unmarshal(blob, &got); err != nil {
		t.Fatalf("unmarshal projection: %v", err)
	}
	keys := make([]string, 0, len(got))
	for k := range got {
		keys = append(keys, k)
	}
	slices.Sort(keys)

	want := []string{
		"certifications", "education", "experience", "headline", "languages",
		"location", "projects", "skills", "summary", "total_years",
	}
	slices.Sort(want)

	if !slices.Equal(keys, want) {
		t.Errorf("Anonymous() top-level keys = %v, want %v — a field reached the public, "+
			"unauthenticated Talent Network route without being reviewed for that exposure", keys, want)
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

// TestPublic_StripsProjectLinks is Public()'s counterpart to
// TestAnonymous_StripsProjectLinks: public mode shows the candidate's name, but a
// project link is still a personal URL the page must not hand out — same rationale as
// the contact-field stripping this projection already does.
func TestPublic_StripsProjectLinks(t *testing.T) {
	s := fullStructured() // carries a project with a non-empty Link

	got := s.Public()

	if len(got.Projects) == 0 {
		t.Fatalf("Projects is empty, fixture should carry at least one")
	}
	for _, p := range got.Projects {
		if p.Link != "" {
			t.Errorf("project %q carries Link %q, want stripped", p.Name, p.Link)
		}
		if p.Name == "" {
			t.Errorf("project lost its Name too — only Link should be stripped")
		}
	}

	blob, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal projection: %v", err)
	}
	if strings.Contains(string(blob), `"link"`) {
		t.Errorf("public projection JSON still carries a project link key: %s", blob)
	}
}

// TestPublic_StripProjectLinksDoesNotMutateSource is Public()'s counterpart to
// TestAnonymous_StripProjectLinksDoesNotMutateSource.
func TestPublic_StripProjectLinksDoesNotMutateSource(t *testing.T) {
	s := fullStructured()
	original := s.Projects[0].Link

	_ = s.Public()

	if s.Projects[0].Link != original {
		t.Errorf("source Structured.Projects[0].Link = %q, mutated (want %q)", s.Projects[0].Link, original)
	}
}

// TestPublic_IsAWhitelist is TestAnonymous_IsAWhitelist's counterpart for Public(),
// which additionally carries full_name (the whole point of public mode).
func TestPublic_IsAWhitelist(t *testing.T) {
	blob, err := json.Marshal(fullStructured().Public())
	if err != nil {
		t.Fatalf("marshal projection: %v", err)
	}

	var got map[string]json.RawMessage
	if err := json.Unmarshal(blob, &got); err != nil {
		t.Fatalf("unmarshal projection: %v", err)
	}
	keys := make([]string, 0, len(got))
	for k := range got {
		keys = append(keys, k)
	}
	slices.Sort(keys)

	want := []string{
		"certifications", "education", "experience", "full_name", "headline",
		"languages", "location", "projects", "skills", "summary", "total_years",
	}
	slices.Sort(want)

	if !slices.Equal(keys, want) {
		t.Errorf("Public() top-level keys = %v, want %v — a field reached the public, "+
			"unauthenticated Talent Network route without being reviewed for that exposure", keys, want)
	}
}

// TestProfessional_IsAWhitelist (professional_test.go) already covers Professional's
// field set via reflect.TypeOf; TestAnonymous_IsAWhitelist/TestPublic_IsAWhitelist above
// use the marshaled JSON of the actual public projections instead —
// reflect.TypeOf(Public{}) would see the embedded Professional as a single unnamed
// field, not its flattened JSON keys.
