package experience

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestScoreRanksSkillEvidenceAboveIncidentalWords(t *testing.T) {
	q := Query{Text: "Experience operating Kubernetes in production", Skills: []string{"kubernetes"}}

	evidence := Atom{
		Claim:      "Migrated the platform to a managed cluster",
		Skills:     []string{"kubernetes"},
		Provenance: ProvenanceCVImport,
	}
	wordy := Atom{
		Claim:      "Wrote the production experience guidelines for the operating team",
		Provenance: ProvenanceCVImport,
	}

	if score(evidence, nil, q) <= score(wordy, nil, q) {
		t.Errorf("evidence scored %.2f, word-overlap scored %.2f — the skill match must dominate",
			score(evidence, nil, q), score(wordy, nil, q))
	}
}

// A requirement can name no skill at all ("led a team of five"), which is exactly why
// retrieval cannot be a skill prefilter.
func TestScoreFindsEvidenceWithNoSkillSlug(t *testing.T) {
	q := Query{Text: "Led a team of engineers"}

	led := Atom{Claim: "Led a team of five engineers through the migration", Provenance: ProvenanceManual}
	unrelated := Atom{Claim: "Cut report load by 95%", Provenance: ProvenanceManual}

	if score(led, nil, q) <= 0 {
		t.Error("an atom matching the requirement's words alone scored nothing")
	}
	if score(led, nil, q) <= score(unrelated, nil, q) {
		t.Error("the matching atom did not outrank the unrelated one")
	}
}

// Stopwords are the difference between a ranking and a coin flip: without them every atom
// shares "the", "a" and "with" with every requirement.
func TestScoreIgnoresStopwords(t *testing.T) {
	q := Query{Text: "the and with a of in to for"}
	atom := Atom{Claim: "The team and the platform, with a lot of work in it", Provenance: ProvenanceManual}

	if got := score(atom, nil, q); got != 0 {
		t.Errorf("score = %.2f, want 0 — a requirement of stopwords matches nothing", got)
	}
}

// The role's stack is a real but weaker signal: a bullet whose own text never names the
// technology still counts when the role ran on it — just below one that names it.
func TestScoreCountsTheRoleStackBelowTheBulletsOwnSkills(t *testing.T) {
	q := Query{Skills: []string{"mongodb"}}
	role := &Employment{Kind: KindJob, Company: "RingCentral", Stack: []string{"mongodb"}}

	named := Atom{Claim: "Rewrote the MongoDB index", Skills: []string{"mongodb"}, Provenance: ProvenanceCVImport}
	byRole := Atom{Claim: "Cut message-posting latency 20s to 1s", Provenance: ProvenanceCVImport}

	viaRole := score(byRole, role, q)
	if viaRole <= 0 {
		t.Error("an atom from a role running on the technology scored nothing")
	}
	if viaRole >= score(named, role, q) {
		t.Error("the role's stack scored as high as the bullet naming the technology itself")
	}
}

// Recency is a property of the role, not the bullet.
func TestScorePrefersEvidenceFromACurrentRole(t *testing.T) {
	q := Query{Skills: []string{"go"}}
	atom := Atom{Claim: "Built the service", Skills: []string{"go"}, Provenance: ProvenanceCVImport}

	current := &Employment{Kind: KindJob, Company: "Now", Current: true}
	past := &Employment{Kind: KindJob, Company: "Then"}

	if score(atom, current, q) <= score(atom, past, q) {
		t.Error("evidence from the current role did not outrank the same evidence from a past one")
	}
}

// An agent's own inference is still returned — the agent may want to ask about it — but it
// must never outrank what the candidate actually asserted.
func TestScorePenalisesUnconfirmedEvidence(t *testing.T) {
	q := Query{Skills: []string{"kafka"}}
	confirmed := Atom{Claim: "Ran the event bus", Skills: []string{"kafka"}, Provenance: ProvenanceStatedInChat}
	inferred := Atom{Claim: "Ran the event bus", Skills: []string{"kafka"}, Provenance: ProvenanceAgentInferred}

	if score(inferred, nil, q) >= score(confirmed, nil, q) {
		t.Error("an agent_inferred atom scored at or above a confirmed one")
	}
	if score(inferred, nil, q) <= 0 {
		t.Error("an agent_inferred atom scored nothing at all — it should rank low, not vanish")
	}
}

func TestRetrieveReturnsRankedMatchesWithinTheLimit(t *testing.T) {
	s, _ := newStore()
	ctx := context.Background()

	role, err := s.CreateEmployment(ctx, owner, Employment{
		Kind: KindJob, Company: "RingCentral", Role: "SWE", Current: true, Stack: []string{"mongodb"},
	})
	if err != nil {
		t.Fatalf("CreateEmployment: %v", err)
	}
	for _, a := range []Atom{
		{EmploymentID: &role.ID, Claim: "Rewrote the MongoDB index", Skills: []string{"mongodb"}, Provenance: ProvenanceCVImport},
		{EmploymentID: &role.ID, Claim: "Cut message-posting latency 20s to 1s", Provenance: ProvenanceCVImport},
		{Claim: "Wrote the onboarding handbook", Provenance: ProvenanceManual},
	} {
		if _, err := s.AddAtom(ctx, owner, a); err != nil {
			t.Fatalf("AddAtom: %v", err)
		}
	}

	matches, err := s.Retrieve(ctx, owner, Query{Text: "MongoDB indexing at scale", Skills: []string{"mongodb"}}, 2)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("matches = %d, want the limit of 2", len(matches))
	}
	if matches[0].Atom.Claim != "Rewrote the MongoDB index" {
		t.Errorf("top match = %q, want the atom naming MongoDB", matches[0].Atom.Claim)
	}
	if matches[0].Score < matches[1].Score {
		t.Error("matches are not ordered by score")
	}
}

// A requirement nothing answers must come back empty rather than with the least-bad atom:
// a zero-scoring "match" would read to the agent as evidence and produce an invented bullet.
func TestRetrieveReturnsNothingWhenNothingMatches(t *testing.T) {
	s, _ := newStore()
	ctx := context.Background()

	if _, err := s.AddAtom(ctx, owner, Atom{Claim: "Wrote the onboarding handbook", Provenance: ProvenanceManual}); err != nil {
		t.Fatalf("AddAtom: %v", err)
	}

	matches, err := s.Retrieve(ctx, owner, Query{Text: "Rust compiler internals", Skills: []string{"rust"}}, 5)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(matches) != 0 {
		t.Errorf("matches = %+v, want none — an unmatched requirement is a gap, not a weak match", matches)
	}
}

func TestRetrieveIsOwnerScoped(t *testing.T) {
	s, _ := newStore()
	ctx := context.Background()

	if _, err := s.AddAtom(ctx, owner, Atom{Claim: "Ran the cluster", Skills: []string{"kubernetes"}, Provenance: ProvenanceManual}); err != nil {
		t.Fatalf("AddAtom: %v", err)
	}
	matches, err := s.Retrieve(ctx, stranger, Query{Skills: []string{"kubernetes"}}, 5)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(matches) != 0 {
		t.Errorf("a stranger retrieved %d of the owner's atoms", len(matches))
	}
}

// An atom pointing at a role that is gone should be unreachable — AddAtom refuses a
// foreign or unknown employment, and deleting a role cascades to its atoms. This asserts
// the defensive path anyway, by planting the state the API cannot produce: retrieval must
// score such an atom on its own signals rather than dropping or panicking on it, because
// the alternative is losing evidence to a state nobody can explain.
func TestRetrieveToleratesADanglingEmploymentPointer(t *testing.T) {
	s, repo := newStore()
	ctx := context.Background()

	ghost := uuid.New()
	if _, err := repo.InsertAtomIfNew(ctx, owner, Atom{
		EmploymentID: &ghost, Claim: "Ran the cluster", Skills: []string{"kubernetes"},
		Provenance: ProvenanceManual,
	}, ClaimKey("Ran the cluster")); err != nil {
		t.Fatalf("plant atom: %v", err)
	}

	matches, err := s.Retrieve(ctx, owner, Query{Skills: []string{"kubernetes"}}, 5)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(matches) != 1 {
		t.Fatalf("matches = %d, want the atom still scored on its own skills", len(matches))
	}
	if matches[0].Employment != nil {
		t.Error("a dangling pointer resolved to an employment")
	}
}
