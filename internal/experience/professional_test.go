package experience

import (
	"context"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/resumeextract"
)

func structuredWithoutExperience() resumeextract.Structured {
	return resumeextract.Structured{
		FullName:   "Ilya Strelov",
		Email:      "someone@example.test",
		Phone:      "+00 000",
		Links:      []string{"https://example.test"},
		Headline:   "Senior Backend Engineer",
		Summary:    "14 years of high-load systems",
		TotalYears: 14,
		Education:  []resumeextract.Education{{Degree: "BSc", Institution: "DSTU"}},
		Languages:  []string{"English", "Russian"},
		Skills:     []string{"go", "kubernetes"},
		// The structure's own experience is deliberately present and must be ignored: the
		// bank owns the work history now.
		Experience: []resumeextract.Experience{{Company: "STALE", Title: "STALE"}},
	}
}

func TestProfessionalFromTakesExperienceFromTheBank(t *testing.T) {
	employments := []Employment{{
		Kind: KindJob, Company: "RingCentral", Role: "Senior Software Engineer",
		Location: "USA, Remote", Start: "2023-09", End: "Present", Current: true,
		Summary: "Global SaaS leader", Stack: []string{"go", "mongodb"},
	}}
	atoms := []Atom{{
		EmploymentID: &employments[0].ID, Claim: "Cut latency 20s to 1s", Provenance: ProvenanceCVImport,
	}}

	got := ProfessionalFrom(structuredWithoutExperience(), employments, atoms)

	if len(got.Experience) != 1 {
		t.Fatalf("experience = %+v, want the bank's single role", got.Experience)
	}
	role := got.Experience[0]
	if role.Company != "RingCentral" || role.Title != "Senior Software Engineer" {
		t.Errorf("role = %+v, want the banked employment, not the structure's stale one", role)
	}
	if len(role.Highlights) != 1 || role.Highlights[0] != "Cut latency 20s to 1s" {
		t.Errorf("highlights = %q, want the role's banked claims", role.Highlights)
	}
	if len(role.Stack) != 2 {
		t.Errorf("stack = %q, want the banked stack", role.Stack)
	}
}

// The non-experience sections have no home in the bank and keep coming from the structure.
func TestProfessionalFromKeepsTheStructuresOtherSections(t *testing.T) {
	got := ProfessionalFrom(structuredWithoutExperience(), nil, nil)

	if got.Headline != "Senior Backend Engineer" || got.Summary == "" || got.TotalYears != 14 {
		t.Errorf("headline/summary/years lost: %+v", got)
	}
	if len(got.Education) != 1 || len(got.Languages) != 2 || len(got.Skills) != 2 {
		t.Errorf("education/languages/skills lost: %+v", got)
	}
}

// The whole point of the contact-free projection: identity never crosses over, and the
// composition must not reopen the hole the whitelist closed.
func TestProfessionalFromCarriesNoContacts(t *testing.T) {
	got := ProfessionalFrom(structuredWithoutExperience(), nil, nil)

	rendered := got.Headline + got.Summary + strings.Join(got.Languages, " ")
	for _, contact := range []string{"Ilya Strelov", "someone@example.test", "+00 000", "https://example.test"} {
		if strings.Contains(rendered, contact) {
			t.Errorf("contact %q reached the de-identified projection", contact)
		}
	}
}

// An agent's own inference is not evidence the candidate stands behind, so it must not
// reach the model that scores their fit — nor any CV composed from this projection.
func TestProfessionalFromWithholdsUnpublishableAtoms(t *testing.T) {
	employments := []Employment{{Kind: KindJob, Company: "RingCentral", Role: "SWE"}}
	atoms := []Atom{
		{EmploymentID: &employments[0].ID, Claim: "Confirmed achievement", Provenance: ProvenanceStatedInChat},
		{EmploymentID: &employments[0].ID, Claim: "The agent's guess", Provenance: ProvenanceAgentInferred},
	}

	got := ProfessionalFrom(structuredWithoutExperience(), employments, atoms)

	highlights := got.Experience[0].Highlights
	if len(highlights) != 1 || highlights[0] != "Confirmed achievement" {
		t.Errorf("highlights = %q, want only what the candidate asserted", highlights)
	}
}

// Evidence with no place is still evidence. Dropping it would defeat the change's whole
// claim — that the fit analysis begins to see what a candidate confirmed in chat — so it
// is carried in an entry that names no place rather than a fabricated one.
func TestProfessionalFromCarriesPlacelessEvidence(t *testing.T) {
	atoms := []Atom{{Claim: "AWS Certified Solutions Architect", Provenance: ProvenanceStatedInChat}}

	got := ProfessionalFrom(structuredWithoutExperience(), nil, atoms)

	if len(got.Experience) != 1 {
		t.Fatalf("experience = %+v, want the placeless evidence carried", got.Experience)
	}
	entry := got.Experience[0]
	if entry.Company != "" || entry.Title != "" {
		t.Errorf("entry = %+v, want no invented place", entry)
	}
	if len(entry.Highlights) != 1 || entry.Highlights[0] != "AWS Certified Solutions Architect" {
		t.Errorf("highlights = %q, want the placeless claim", entry.Highlights)
	}
}

// A role the bank holds with no evidence under it is still career history the fit chain
// should see — a person's job titles matter even before their bullets do.
func TestProfessionalFromKeepsARoleWithNoAtoms(t *testing.T) {
	employments := []Employment{{Kind: KindJob, Company: "Sber", Role: "Team Lead"}}

	got := ProfessionalFrom(structuredWithoutExperience(), employments, nil)

	if len(got.Experience) != 1 || got.Experience[0].Company != "Sber" {
		t.Errorf("experience = %+v, want the role kept even with no highlights", got.Experience)
	}
}

// An empty bank yields no experience at all rather than falling back to the structure's
// copy. The fit chain treats that as "no analysis", which is the correct degradation:
// scoring a candidate on a work history nothing owns is worse than not scoring them.
func TestProfessionalFromDoesNotFallBackToTheStructure(t *testing.T) {
	got := ProfessionalFrom(structuredWithoutExperience(), nil, nil)

	if len(got.Experience) != 0 {
		t.Errorf("experience = %+v, want none — the structure's copy is not a fallback", got.Experience)
	}
}

func TestStoreProfessional(t *testing.T) {
	s, _ := newStore()
	ctx := context.Background()

	role, err := s.CreateEmployment(ctx, owner, Employment{Kind: KindJob, Company: "RingCentral", Role: "SWE"})
	if err != nil {
		t.Fatalf("CreateEmployment: %v", err)
	}
	if _, err := s.AddAtom(ctx, owner, Atom{
		EmploymentID: &role.ID, Claim: "Cut latency", Provenance: ProvenanceCVImport,
	}); err != nil {
		t.Fatalf("AddAtom: %v", err)
	}

	got, err := s.Professional(ctx, owner, structuredWithoutExperience())
	if err != nil {
		t.Fatalf("Professional: %v", err)
	}
	if len(got.Experience) != 1 || got.Experience[0].Company != "RingCentral" {
		t.Errorf("experience = %+v, want the owner's banked role", got.Experience)
	}

	stranger, err := s.Professional(ctx, stranger, structuredWithoutExperience())
	if err != nil {
		t.Fatalf("Professional(stranger): %v", err)
	}
	if len(stranger.Experience) != 0 {
		t.Errorf("a stranger saw %d of the owner's roles", len(stranger.Experience))
	}
}
