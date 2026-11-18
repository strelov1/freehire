package experience

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/strelov1/freehire/internal/platform/db"
)

func TestStoreMergeAtomsUnionsAndDeletesLoser(t *testing.T) {
	s, _ := newStore()
	ctx := context.Background()

	a, err := s.AddAtom(ctx, owner, Atom{
		Claim:  "Built a Chromium plugin with faster-whisper for live and batch",
		Skills: []string{"python"}, Provenance: ProvenanceAgentInferred,
	},
		AuthorAgent,
	)
	if err != nil {
		t.Fatalf("AddAtom a: %v", err)
	}
	b, err := s.AddAtom(ctx, owner, Atom{
		Claim:   "Built a Chromium plugin with VAD filtering on faster-whisper",
		Context: "small/medium/large-v3 model profiles", Metrics: []string{"VAD"},
		Skills: []string{"nlp"}, Provenance: ProvenanceAgentInferred,
	},
		AuthorAgent,
	)
	if err != nil {
		t.Fatalf("AddAtom b: %v", err)
	}

	kept, err := s.MergeAtoms(ctx, owner, a.ID, b.ID)
	if err != nil {
		t.Fatalf("MergeAtoms: %v", err)
	}
	if kept.Context == "" || len(kept.Metrics) == 0 {
		t.Errorf("kept missing richness: context=%q metrics=%q", kept.Context, kept.Metrics)
	}
	if ClaimKey(kept.Claim) == "" {
		t.Error("claim emptied")
	}

	atoms, err := s.ListAtoms(ctx, owner)
	if err != nil {
		t.Fatalf("ListAtoms: %v", err)
	}
	if len(atoms) != 1 {
		t.Fatalf("atoms after merge = %d, want 1", len(atoms))
	}
	if atoms[0].ID != kept.ID {
		t.Errorf("surviving id = %s, want keep %s", atoms[0].ID, kept.ID)
	}
}

// raceInjectingRepo wraps fakeRepo and, the moment Store.MergeAtoms' GetAtom(trigger)
// returns, simulates a write landing in the exact gap MergeAtoms cannot close on its own:
// keep/lose selection and the merged-fields union happen in Go, between the two reads and
// the eventual write, with no transaction holding the rows still. victim is mutated to
// stand in for that concurrent write.
type raceInjectingRepo struct {
	*fakeRepo
	trigger uuid.UUID
	victim  uuid.UUID
}

func (r *raceInjectingRepo) GetAtom(ctx context.Context, id uuid.UUID, userID int64) (db.ExperienceAtom, error) {
	row, err := r.fakeRepo.GetAtom(ctx, id, userID)
	if id == r.trigger {
		v := r.atoms[r.victim]
		v.Context = "raced in from another request"
		v.UpdatedAt = r.stamp()
		r.atoms[r.victim] = v
	}
	return row, err
}

// The race the review flagged: a write landing between Store.MergeAtoms' reads and its
// write must not be silently discarded by an update computed from the now-stale snapshot.
func TestStoreMergeAtomsRejectsAConcurrentWriteInTheReadWriteGap(t *testing.T) {
	s, repo := newStore()
	ctx := context.Background()

	a, err := s.AddAtom(ctx, owner, Atom{Claim: "Did the thing", Provenance: ProvenanceManual}, AuthorCandidate)
	if err != nil {
		t.Fatalf("AddAtom a: %v", err)
	}
	b, err := s.AddAtom(ctx, owner, Atom{Claim: "Did the other thing", Provenance: ProvenanceManual}, AuthorCandidate)
	if err != nil {
		t.Fatalf("AddAtom b: %v", err)
	}

	racy := NewStore(&raceInjectingRepo{fakeRepo: repo, trigger: b.ID, victim: a.ID})
	if _, err := racy.MergeAtoms(ctx, owner, a.ID, b.ID); !errors.Is(err, ErrMergeConflict) {
		t.Fatalf("MergeAtoms racing a concurrent edit = %v, want ErrMergeConflict", err)
	}

	// The concurrent edit must survive untouched — not overwritten by a merge built from
	// the snapshot read just before it landed.
	got, err := s.GetAtom(ctx, a.ID, owner)
	if err != nil {
		t.Fatalf("GetAtom a: %v", err)
	}
	if got.Context != "raced in from another request" {
		t.Errorf("context = %q, want the concurrent edit preserved, not clobbered by the aborted merge", got.Context)
	}
	// The would-be loser must survive too — an aborted merge deletes nothing.
	if _, err := s.GetAtom(ctx, b.ID, owner); err != nil {
		t.Errorf("GetAtom b: %v, want the loser to survive an aborted merge", err)
	}
}

func TestStoreMergeAtomsNotFoundAndCrossEmployment(t *testing.T) {
	s, _ := newStore()
	ctx := context.Background()

	roleA, err := s.CreateEmployment(ctx, owner, Employment{Kind: KindJob, Company: "A", Role: "SWE"})
	if err != nil {
		t.Fatalf("CreateEmployment A: %v", err)
	}
	roleB, err := s.CreateEmployment(ctx, owner, Employment{Kind: KindJob, Company: "B", Role: "SWE"})
	if err != nil {
		t.Fatalf("CreateEmployment B: %v", err)
	}
	a, err := s.AddAtom(ctx, owner, Atom{
		EmploymentID: &roleA.ID, Claim: "Did the thing at A", Provenance: ProvenanceManual,
	},
		AuthorCandidate,
	)
	if err != nil {
		t.Fatalf("AddAtom a: %v", err)
	}
	b, err := s.AddAtom(ctx, owner, Atom{
		EmploymentID: &roleB.ID, Claim: "Did the thing at B", Provenance: ProvenanceManual,
	},
		AuthorCandidate,
	)
	if err != nil {
		t.Fatalf("AddAtom b: %v", err)
	}

	if _, err := s.MergeAtoms(ctx, owner, a.ID, b.ID); !errors.Is(err, ErrMergeCrossEmployment) {
		t.Errorf("cross employment = %v, want ErrMergeCrossEmployment", err)
	}
	if _, err := s.MergeAtoms(ctx, owner, a.ID, uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing = %v, want ErrNotFound", err)
	}
	if _, err := s.MergeAtoms(ctx, stranger, a.ID, b.ID); !errors.Is(err, ErrNotFound) {
		t.Errorf("not yours = %v, want ErrNotFound", err)
	}
	if _, err := s.MergeAtoms(ctx, owner, a.ID, a.ID); !errors.Is(err, ErrInvalidMerge) {
		t.Errorf("same id = %v, want ErrInvalidMerge", err)
	}
}

// TestRewriteKeepsTheStoredLabelWithoutReadingIt is the atomicity half of the anti-laundering
// rule. AuthorRewrite must reach a statement that leaves provenance alone, not read it and
// write it back: a read-then-write leaves a window in which a concurrent write lands and the
// rewrite restores a label it never saw.
//
// The fake proves it structurally — UpdateAtomKeepingProvenance never touches the column, so
// if the store took the reading path the assertion below would see the caller's value instead.
func TestRewriteKeepsTheStoredLabelWithoutReadingIt(t *testing.T) {
	s, _ := newStore()
	ctx := context.Background()

	// Banked by the agent: unpublishable, which is the label a laundering edit would want gone.
	atom, err := s.AddAtom(ctx, owner, Atom{Claim: "Ran the migration"}, AuthorAgent)
	if err != nil {
		t.Fatalf("AddAtom: %v", err)
	}
	if atom.Provenance != ProvenanceAgentInferred {
		t.Fatalf("seed provenance = %q, want agent_inferred", atom.Provenance)
	}

	// A keyed rewrite, asking loudly for a promotion it must not get.
	atom.Claim = "Ran the migration across three regions"
	atom.Provenance = ProvenanceManual
	got, err := s.UpdateAtom(ctx, atom.ID, owner, atom, AuthorRewrite)
	if err != nil {
		t.Fatalf("UpdateAtom: %v", err)
	}
	if got.Claim != "Ran the migration across three regions" {
		t.Errorf("Claim = %q, want the rewritten words — a rewrite still rewrites", got.Claim)
	}
	if got.Provenance != ProvenanceAgentInferred {
		t.Errorf("Provenance = %q, want agent_inferred untouched", got.Provenance)
	}
	if got.Provenance.Publishable() {
		t.Error("a rewrite promoted a model's reading to CV-publishable")
	}
}

// TestRewriteIgnoresAJunkProvenanceRatherThanRefusing closes the gap the atomic rewrite opened:
// with the label no longer overwritten before Validate, a caller's nonsense survived long
// enough to be rejected. "Inert" has to mean inert — the field is not written, so it is not
// judged either, and the stored standing is untouched.
func TestRewriteIgnoresAJunkProvenanceRatherThanRefusing(t *testing.T) {
	s, _ := newStore()
	ctx := context.Background()

	atom, err := s.AddAtom(ctx, owner, Atom{Claim: "Ran the migration"}, AuthorAgent)
	if err != nil {
		t.Fatalf("AddAtom: %v", err)
	}

	atom.Claim = "Ran the migration across three regions"
	atom.Provenance = "vibes"
	got, err := s.UpdateAtom(ctx, atom.ID, owner, atom, AuthorRewrite)
	if err != nil {
		t.Fatalf("a junk provenance must be ignored, not refused: %v", err)
	}
	if got.Claim != "Ran the migration across three regions" {
		t.Errorf("Claim = %q, want the rewritten words", got.Claim)
	}
	if got.Provenance != ProvenanceAgentInferred {
		t.Errorf("Provenance = %q, want the stored agent_inferred", got.Provenance)
	}
}
