package experience

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/candidate/perioddate"
	"github.com/strelov1/freehire/internal/candidate/resumeextract"
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

// seedBank fills a fresh store with the owner's employments and atoms and returns the store,
// so these tests exercise the live composition — Store.Professional — rather than a
// parameter-taking twin of it. Each atom's EmploymentID is an INDEX into employments, or -1 for
// evidence with no place, because the ids are the store's to mint.
func seedBank(t *testing.T, employments []Employment, atoms []Atom, placeOf []int) *Store {
	t.Helper()
	s, _ := newStore()
	ctx := context.Background()

	ids := make([]uuid.UUID, len(employments))
	for i, e := range employments {
		created, err := s.CreateEmployment(ctx, owner, e)
		if err != nil {
			t.Fatalf("CreateEmployment: %v", err)
		}
		ids[i] = created.ID
	}
	for i, a := range atoms {
		if place := placeOf[i]; place >= 0 {
			a.EmploymentID = &ids[place]
		}
		// The seed states each atom's authorship, since the bank derives the label from it:
		// a fixture that banked everything as the candidate could not express the
		// unpublishable rows these tests exist to check.
		if _, err := s.AddAtom(ctx, owner, a, authorOf(a.Provenance)); err != nil {
			t.Fatalf("AddAtom: %v", err)
		}
	}
	return s
}

// authorOf names the entry point that would have produced a given label, so a fixture can
// still seed the full range of standings now that the bank derives them.
func authorOf(p Provenance) Author {
	switch p {
	case ProvenanceStatedInChat:
		return AuthorQuoted
	case ProvenanceAgentInferred:
		return AuthorAgent
	default:
		return AuthorCandidate
	}
}

// professionalOf is the assertion subject of every test below: the live path the fit chain and
// the profile read both take.
func professionalOf(t *testing.T, s *Store) resumeextract.Professional {
	t.Helper()
	got, err := s.Professional(context.Background(), owner, structuredWithoutExperience())
	if err != nil {
		t.Fatalf("Professional: %v", err)
	}
	return got
}

func TestProfessionalTakesExperienceFromTheBank(t *testing.T) {
	s := seedBank(t, []Employment{{
		Kind: KindJob, Company: "RingCentral", Role: "Senior Software Engineer",
		Location: "USA, Remote", Start: &perioddate.PeriodDate{Year: 2023, Month: 9}, Current: true,
		Summary: "Global SaaS leader", Stack: []string{"go", "mongodb"},
	}}, []Atom{
		{Claim: "Cut latency 20s to 1s", Provenance: ProvenanceCVImport},
	}, []int{0})

	got := professionalOf(t, s)

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
func TestProfessionalKeepsTheStructuresOtherSections(t *testing.T) {
	got := professionalOf(t, seedBank(t, nil, nil, nil))

	if got.Headline != "Senior Backend Engineer" || got.Summary == "" || got.TotalYears != 14 {
		t.Errorf("headline/summary/years lost: %+v", got)
	}
	if len(got.Education) != 1 || len(got.Languages) != 2 || len(got.Skills) != 2 {
		t.Errorf("education/languages/skills lost: %+v", got)
	}
}

// The whole point of the contact-free projection: identity never crosses over, and the
// composition must not reopen the hole the whitelist closed.
func TestProfessionalCarriesNoContacts(t *testing.T) {
	got := professionalOf(t, seedBank(t, nil, nil, nil))

	rendered := got.Headline + got.Summary + strings.Join(got.Languages, " ")
	for _, contact := range []string{"Ilya Strelov", "someone@example.test", "+00 000", "https://example.test"} {
		if strings.Contains(rendered, contact) {
			t.Errorf("contact %q reached the de-identified projection", contact)
		}
	}
}

// An agent's own inference is not evidence the candidate stands behind, so it must not
// reach the model that scores their fit — nor any CV composed from this projection.
func TestProfessionalWithholdsUnpublishableAtoms(t *testing.T) {
	s := seedBank(t, []Employment{{Kind: KindJob, Company: "RingCentral", Role: "SWE"}}, []Atom{
		{Claim: "Confirmed achievement", Provenance: ProvenanceStatedInChat},
		{Claim: "The agent's guess", Provenance: ProvenanceAgentInferred},
	}, []int{0, 0})

	got := professionalOf(t, s)

	highlights := got.Experience[0].Highlights
	if len(highlights) != 1 || highlights[0] != "Confirmed achievement" {
		t.Errorf("highlights = %q, want only what the candidate asserted", highlights)
	}
}

// Chronological re-sort must not widen what reaches a CV: agent_inferred stays bank-only
// while free-form period labels still order roles correctly for Reset / stale-base reseed.
func TestWorkHistory_ChronologyKeepsProvenanceGate(t *testing.T) {
	s := seedBank(t, []Employment{
		{Kind: KindJob, Company: "Northwind", Role: "Staff", Start: &perioddate.PeriodDate{Year: 2018, Month: 10}, End: &perioddate.PeriodDate{Year: 2024}},
		{Kind: KindJob, Company: "Fabrikam", Role: "Staff", Start: &perioddate.PeriodDate{Year: 2024}, End: &perioddate.PeriodDate{Year: 2025}},
	}, []Atom{
		{Claim: "Confirmed at Fabrikam", Provenance: ProvenanceCVImport},
		{Claim: "Model paraphrase at Fabrikam", Provenance: ProvenanceAgentInferred},
		{Claim: "Confirmed at Northwind", Provenance: ProvenanceStatedInChat},
		{Claim: "Unplaced model guess", Provenance: ProvenanceAgentInferred},
		{Claim: "Unplaced confirmed cert", Provenance: ProvenanceManual},
	}, []int{1, 1, 0, -1, -1})

	hist, err := s.WorkHistory(context.Background(), owner)
	if err != nil {
		t.Fatalf("WorkHistory: %v", err)
	}
	if len(hist) != 3 {
		t.Fatalf("roles = %d, want Fabrikam + Northwind + placeless bucket", len(hist))
	}
	if hist[0].Company != "Fabrikam" || hist[1].Company != "Northwind" {
		t.Fatalf("order = [%s, %s], want Fabrikam then Northwind (not lexicographic label order)", hist[0].Company, hist[1].Company)
	}
	if len(hist[0].Highlights) != 1 || hist[0].Highlights[0] != "Confirmed at Fabrikam" {
		t.Errorf("Fabrikam highlights = %q, want only the confirmed claim", hist[0].Highlights)
	}
	if len(hist[1].Highlights) != 1 || hist[1].Highlights[0] != "Confirmed at Northwind" {
		t.Errorf("Northwind highlights = %q, want only the confirmed claim", hist[1].Highlights)
	}
	if hist[2].Company != "" || hist[2].Title != "" {
		t.Errorf("placeless entry = %+v, want blank place", hist[2])
	}
	if len(hist[2].Highlights) != 1 || hist[2].Highlights[0] != "Unplaced confirmed cert" {
		t.Errorf("placeless highlights = %q, want only the confirmed claim", hist[2].Highlights)
	}
}

// Evidence with no place is still evidence. Dropping it would defeat the change's whole
// claim — that the fit analysis begins to see what a candidate confirmed in chat — so it
// is carried in an entry that names no place rather than a fabricated one.
func TestProfessionalCarriesPlacelessEvidence(t *testing.T) {
	s := seedBank(t, nil, []Atom{
		{Claim: "AWS Certified Solutions Architect", Provenance: ProvenanceStatedInChat},
	}, []int{-1})

	got := professionalOf(t, s)

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
func TestProfessionalKeepsARoleWithNoAtoms(t *testing.T) {
	s := seedBank(t, []Employment{{Kind: KindJob, Company: "Sber", Role: "Team Lead"}}, nil, nil)

	got := professionalOf(t, s)

	if len(got.Experience) != 1 || got.Experience[0].Company != "Sber" {
		t.Errorf("experience = %+v, want the role kept even with no highlights", got.Experience)
	}
}

// An empty bank yields no experience at all rather than falling back to the structure's
// copy. The fit chain treats that as "no analysis", which is the correct degradation:
// scoring a candidate on a work history nothing owns is worse than not scoring them.
func TestProfessionalDoesNotFallBackToTheStructure(t *testing.T) {
	got := professionalOf(t, seedBank(t, nil, nil, nil))

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
	},
		AuthorCandidate,
	); err != nil {
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
