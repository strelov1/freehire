package handler

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/experience"
)

// stubBank is an in-memory experienceBankTools, so the gate — the one rule the whole
// capability exists to keep — is exercised without a database.
type stubBank struct {
	atoms       map[uuid.UUID]experience.Atom
	owner       map[uuid.UUID]int64
	list        []experience.Atom
	employments []experience.Employment
	matches     []experience.Match
	addErr      error
	// readAs is the owner the list reads belong to, since the stub serves one caller.
	readAs int64
}

func newStubBank() *stubBank {
	return &stubBank{atoms: map[uuid.UUID]experience.Atom{}, owner: map[uuid.UUID]int64{}, readAs: 1}
}

func (b *stubBank) add(userID int64, a experience.Atom) experience.Atom {
	a.ID = uuid.New()
	b.atoms[a.ID] = a
	b.owner[a.ID] = userID
	return a
}

func (b *stubBank) GetAtom(_ context.Context, id uuid.UUID, userID int64) (experience.Atom, error) {
	if a, ok := b.atoms[id]; ok && b.owner[id] == userID {
		return a, nil
	}
	return experience.Atom{}, experience.ErrNotFound
}

func (b *stubBank) Retrieve(context.Context, int64, experience.Query, int) ([]experience.Match, error) {
	return b.matches, nil
}
func (b *stubBank) ListEmployments(context.Context, int64) ([]experience.Employment, error) {
	return b.employments, nil
}
func (b *stubBank) ListAtoms(context.Context, int64) ([]experience.Atom, error) { return b.list, nil }
func (b *stubBank) AddAtom(_ context.Context, userID int64, a experience.Atom) (experience.Atom, error) {
	if b.addErr != nil {
		return experience.Atom{}, b.addErr
	}
	if strings.TrimSpace(a.Claim) == "" {
		return experience.Atom{}, experience.ErrEmptyClaim
	}
	stored := b.add(userID, a)
	b.reindex()
	return stored, nil
}
func (b *stubBank) UpdateAtom(_ context.Context, id uuid.UUID, userID int64, a experience.Atom) (experience.Atom, error) {
	if _, ok := b.atoms[id]; !ok || b.owner[id] != userID {
		return experience.Atom{}, experience.ErrNotFound
	}
	a.ID = id
	b.atoms[id] = a
	b.reindex()
	return a, nil
}

func (b *stubBank) CreateEmployment(_ context.Context, userID int64, e experience.Employment) (experience.Employment, error) {
	if err := e.Validate(); err != nil {
		return experience.Employment{}, err
	}
	return b.addEmployment(userID, e), nil
}

func (b *stubBank) UpdateEmployment(_ context.Context, id uuid.UUID, userID int64, e experience.Employment) (experience.Employment, error) {
	for i, existing := range b.employments {
		if existing.ID == id && b.owner[id] == userID {
			e.ID = id
			b.employments[i] = e
			return e, nil
		}
	}
	return experience.Employment{}, experience.ErrNotFound
}

func (b *stubBank) DeleteEmployment(_ context.Context, id uuid.UUID, userID int64) error {
	for i, existing := range b.employments {
		if existing.ID == id && b.owner[id] == userID {
			b.employments = append(b.employments[:i], b.employments[i+1:]...)
			for aid, a := range b.atoms {
				if a.EmploymentID != nil && *a.EmploymentID == id {
					delete(b.atoms, aid)
				}
			}
			b.reindex()
			return nil
		}
	}
	return experience.ErrNotFound
}

func (b *stubBank) DeleteAtom(_ context.Context, id uuid.UUID, userID int64) error {
	if _, ok := b.atoms[id]; !ok || b.owner[id] != userID {
		return experience.ErrNotFound
	}
	delete(b.atoms, id)
	b.reindex()
	return nil
}

func (b *stubBank) MergeAtoms(_ context.Context, userID int64, aID, bID uuid.UUID) (experience.Atom, error) {
	a, err := b.GetAtom(context.Background(), aID, userID)
	if err != nil {
		return experience.Atom{}, err
	}
	other, err := b.GetAtom(context.Background(), bID, userID)
	if err != nil {
		return experience.Atom{}, err
	}
	if aID == bID {
		return experience.Atom{}, experience.ErrInvalidMerge
	}
	sameBucket := (a.EmploymentID == nil && other.EmploymentID == nil) ||
		(a.EmploymentID != nil && other.EmploymentID != nil && *a.EmploymentID == *other.EmploymentID)
	if !sameBucket {
		return experience.Atom{}, experience.ErrMergeCrossEmployment
	}
	// Prefer the richer side as keep — mirrors Store keep-selection enough for HTTP tests.
	richness := func(a experience.Atom) int {
		n := len(a.Metrics) + len(a.Skills)
		if strings.TrimSpace(a.Context) != "" {
			n++
		}
		return n
	}
	keep, lose := a, other
	if richness(other) > richness(a) {
		keep, lose = other, a
	}
	if strings.TrimSpace(keep.Context) == "" {
		keep.Context = lose.Context
	}
	keep.Metrics = append(append([]string{}, keep.Metrics...), lose.Metrics...)
	keep.Skills = append(append([]string{}, keep.Skills...), lose.Skills...)
	if !keep.Provenance.Publishable() && lose.Provenance.Publishable() {
		keep.Provenance = lose.Provenance
	}
	delete(b.atoms, lose.ID)
	b.atoms[keep.ID] = keep
	b.reindex()
	return keep, nil
}

// reindex rebuilds the owner-filtered list the read paths walk.
func (b *stubBank) reindex() {
	b.list = b.list[:0]
	for id, a := range b.atoms {
		if b.owner[id] == b.readAs {
			b.list = append(b.list, a)
		}
	}
	sort.Slice(b.list, func(i, j int) bool { return b.list[i].Claim < b.list[j].Claim })
}

// addEmployment records a place for readAs.
func (b *stubBank) addEmployment(userID int64, e experience.Employment) experience.Employment {
	e.ID = uuid.New()
	b.owner[e.ID] = userID
	b.employments = append(b.employments, e)
	return e
}

func gateHandlers(t *testing.T) (*assistantHandlers, *stubBank) {
	t.Helper()
	bank := newStubBank()
	return &assistantHandlers{experience: bank}, bank
}

// This is the requirement the whole change exists to make durable. Until now it lived in a
// paragraph of the system prompt, which a long conversation eventually loses; here it is a
// branch in the service path that no amount of context can talk its way past.
func TestCVEditGateRefusesEvidenceTheCandidateNeverGave(t *testing.T) {
	h, bank := gateHandlers(t)
	gate := bankGate{bank: h.experience}
	ctx := context.Background()

	inferred := bank.add(1, experience.Atom{
		Claim:      "Probably led the Kubernetes migration",
		Provenance: experience.ProvenanceAgentInferred,
	})

	err := gate.Publishable(ctx, 1, inferred.ID.String())
	if err == nil {
		t.Fatal("an agent's own inference was allowed onto the CV")
	}
	// The message is the model's only route to correcting itself inside the turn, so it
	// has to say what to do rather than merely that something was wrong.
	for _, want := range []string{"confirm", "experience_add"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("refusal %q does not tell the model to %q", err, want)
		}
	}
}

func TestCVEditGateAllowsWhatTheCandidateAsserted(t *testing.T) {
	h, bank := gateHandlers(t)
	gate := bankGate{bank: h.experience}
	ctx := context.Background()

	for _, provenance := range []experience.Provenance{
		experience.ProvenanceCVImport,
		experience.ProvenanceStatedInChat,
		experience.ProvenanceManual,
	} {
		atom := bank.add(1, experience.Atom{
			Claim:      "Cut latency 20s to 1s via " + string(provenance),
			Provenance: provenance,
		})
		if err := gate.Publishable(ctx, 1, atom.ID.String()); err != nil {
			t.Errorf("evidence with provenance %s was refused: %v", provenance, err)
		}
	}
}

// A batch cites an evidence id per operation, so "that id" names nothing a model can act on
// when thirteen of them went out together. The refusal carries the id that missed.
func TestCVEditGateNamesTheIdThatMissed(t *testing.T) {
	h, _ := gateHandlers(t)
	gate := bankGate{bank: h.experience}
	missing := uuid.New()

	err := gate.Publishable(context.Background(), 1, missing.String())
	if err == nil {
		t.Fatal("an id matching no achievement was accepted")
	}
	if !strings.Contains(err.Error(), missing.String()) {
		t.Errorf("error = %v, want it to name the id that missed", err)
	}
}

func TestCVEditGateRefusesAnUnknownOrForeignAtom(t *testing.T) {
	h, bank := gateHandlers(t)
	gate := bankGate{bank: h.experience}
	ctx := context.Background()

	if err := gate.Publishable(ctx, 1, uuid.New().String()); err == nil {
		t.Error("an id matching no achievement was accepted")
	}
	if err := gate.Publishable(ctx, 1, "not-a-uuid"); err == nil {
		t.Error("a malformed id was accepted")
	}
	// Omitting the id must not be a way around the gate — that would make it decorative.
	if err := gate.Publishable(ctx, 1, ""); err == nil {
		t.Error("an empty id was accepted")
	}

	// Another user's evidence is not evidence for this candidate.
	theirs := bank.add(2, experience.Atom{Claim: "Ran the cluster", Provenance: experience.ProvenanceManual})
	if err := gate.Publishable(ctx, 1, theirs.ID.String()); err == nil {
		t.Error("one candidate cited another candidate's achievement")
	}
}

// WHICH edits need a citation is no longer a property of an operation's name — it is the
// path being written, and it is enforced by the editor rather than here. That moved the
// boundary: a technology in a stack line and an item in a skill group assert exactly what a
// bullet does, and under the old vocabulary both arrived uncited. See
// TestAClaimAboutTheCandidateNeedsEvidence and TestRearrangingNeedsNoEvidence in
// internal/cvedit.

// A tool result is replayed into the model's context on every later turn, so the profile
// read must report the bank's SHAPE and not its contents. A few hundred achievements
// replayed each turn would consume the window and defeat trimming.
func TestProfileToolReportsShapeNotContents(t *testing.T) {
	h, bank := gateHandlers(t)
	h.profile = &profileHandlers{}
	ctx := context.Background()

	role := uuid.New()
	bank.employments = []experience.Employment{
		{ID: role, Kind: experience.KindJob, Company: "RingCentral", Role: "SWE", Current: true, Stack: []string{"go"}},
	}
	for i := 0; i < 200; i++ {
		bank.list = append(bank.list, experience.Atom{
			EmploymentID: &role,
			Claim:        "A very specific achievement number " + string(rune('A'+i%26)),
			Skills:       []string{"go"},
			Provenance:   experience.ProvenanceCVImport,
		})
	}

	summary := h.experienceSummary(ctx, 1, []string{"go", "kubernetes"})
	if summary == nil {
		t.Fatal("no experience summary for a populated bank")
	}
	if summary.TotalAchievements != 200 {
		t.Errorf("total = %d, want 200", summary.TotalAchievements)
	}
	if len(summary.Employments) != 1 || summary.Employments[0].Achievements != 200 {
		t.Errorf("employments = %+v, want one role carrying all 200", summary.Employments)
	}

	blob, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(blob), "A very specific achievement") {
		t.Error("the summary carries achievement text — that is what experience_search is for")
	}

	// A skill the candidate claims with nothing to show for it is the interviewer's work
	// list and the tailoring agent's warning.
	if len(summary.SkillsWithoutEvidence) != 1 || summary.SkillsWithoutEvidence[0] != "kubernetes" {
		t.Errorf("skills without evidence = %q, want [kubernetes]", summary.SkillsWithoutEvidence)
	}
	if summary.NeedsContextCount != 200 {
		t.Errorf("needs_context_count = %d, want 200 for claim-only atoms", summary.NeedsContextCount)
	}
}
