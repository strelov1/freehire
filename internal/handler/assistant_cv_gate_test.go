package handler

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/cv"
	"github.com/strelov1/freehire/internal/experience"
)

// stubBank is an in-memory experienceBankTools, so the gate — the one rule the whole
// capability exists to keep — is exercised without a database.
type stubBank struct {
	atoms       map[uuid.UUID]experience.Atom
	owner       map[uuid.UUID]int64
	list        []experience.Atom
	employments []experience.Employment
}

func newStubBank() *stubBank {
	return &stubBank{atoms: map[uuid.UUID]experience.Atom{}, owner: map[uuid.UUID]int64{}}
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
	return nil, nil
}
func (b *stubBank) ListEmployments(context.Context, int64) ([]experience.Employment, error) {
	return b.employments, nil
}
func (b *stubBank) ListAtoms(context.Context, int64) ([]experience.Atom, error) { return b.list, nil }
func (b *stubBank) AddAtom(_ context.Context, userID int64, a experience.Atom) (experience.Atom, error) {
	return b.add(userID, a), nil
}
func (b *stubBank) UpdateAtom(_ context.Context, id uuid.UUID, _ int64, a experience.Atom) (experience.Atom, error) {
	a.ID = id
	b.atoms[id] = a
	return a, nil
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
	ctx := context.Background()

	inferred := bank.add(1, experience.Atom{
		Claim:      "Probably led the Kubernetes migration",
		Provenance: experience.ProvenanceAgentInferred,
	})

	err := h.requireEvidence(ctx, 1, cv.PatchAddBullet, inferred.ID.String())
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
		if err := h.requireEvidence(ctx, 1, cv.PatchAddBullet, atom.ID.String()); err != nil {
			t.Errorf("evidence with provenance %s was refused: %v", provenance, err)
		}
	}
}

// Omitting the id must not be a way around the gate — that would make it decorative.
func TestCVEditGateRefusesAnUncitedBullet(t *testing.T) {
	h, _ := gateHandlers(t)

	err := h.requireEvidence(context.Background(), 1, cv.PatchAddBullet, "")
	if err == nil {
		t.Fatal("a bullet with no evidence at all was allowed")
	}
	if !strings.Contains(err.Error(), "experience_search") {
		t.Errorf("refusal %q does not point the model at experience_search", err)
	}
}

func TestCVEditGateRefusesAnUnknownOrForeignAtom(t *testing.T) {
	h, bank := gateHandlers(t)
	ctx := context.Background()

	if err := h.requireEvidence(ctx, 1, cv.PatchAddBullet, uuid.New().String()); err == nil {
		t.Error("an id matching no achievement was accepted")
	}
	if err := h.requireEvidence(ctx, 1, cv.PatchAddBullet, "not-a-uuid"); err == nil {
		t.Error("a malformed id was accepted")
	}

	// Another user's evidence is not evidence for this candidate.
	theirs := bank.add(2, experience.Atom{Claim: "Ran the cluster", Provenance: experience.ProvenanceManual})
	if err := h.requireEvidence(ctx, 1, cv.PatchAddBullet, theirs.ID.String()); err == nil {
		t.Error("one candidate cited another candidate's achievement")
	}
}

// The gate covers claims, not housekeeping. Reordering, removing and the technology line
// rearrange or delete what is already on the page and assert nothing new; requiring
// evidence for them would make the agent useless without protecting anything.
func TestCVEditGateOnlyCoversOpsThatAssertSomething(t *testing.T) {
	h, _ := gateHandlers(t)
	ctx := context.Background()

	for _, op := range []cv.PatchOp{
		cv.PatchRemoveBullet, cv.PatchReorderBullets, cv.PatchSetStack,
		cv.PatchSetSkillGroup, cv.PatchSetSummary, cv.PatchSetHeaderField,
	} {
		if err := h.requireEvidence(ctx, 1, op, ""); err != nil {
			t.Errorf("op %q was gated: %v", op, err)
		}
	}
	for _, op := range []cv.PatchOp{cv.PatchAddBullet, cv.PatchReplaceBullet} {
		if err := h.requireEvidence(ctx, 1, op, ""); err == nil {
			t.Errorf("op %q was not gated", op)
		}
	}
}

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
}
