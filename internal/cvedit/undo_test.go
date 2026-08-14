package cvedit

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestUndoingAnOlderEditKeepsTheNewerOnes(t *testing.T) {
	repo := newFakeRepo()
	e, c := newEditor(repo, nil)
	cvID := repo.cvID

	first := commitSet(t, e, repo, ActorCandidate, OriginEditor, "summary", "Distributed systems")
	c.at = c.at.Add(2 * time.Minute)
	commitSet(t, e, repo, ActorCandidate, OriginEditor, "title", "Staff Engineer")

	if _, _, err := e.Revert(context.Background(), cvID, 1, first.ID); err != nil {
		t.Fatalf("Revert: %v", err)
	}

	if repo.state.Summary != "Ten years of Go" {
		t.Fatalf("summary = %q, want the text from before the reverted edit", repo.state.Summary)
	}
	if repo.state.Title != "Staff Engineer" {
		t.Fatalf("title = %q, want the later edit to have survived", repo.state.Title)
	}
}

func TestAnUndoIsItselfARevision(t *testing.T) {
	repo := newFakeRepo()
	e, _ := newEditor(repo, nil)
	cvID := repo.cvID

	first := commitSet(t, e, repo, ActorCandidate, OriginEditor, "summary", "Distributed systems")

	_, undo, err := e.Revert(context.Background(), cvID, 1, first.ID)
	if err != nil {
		t.Fatalf("Revert: %v", err)
	}

	if undo.RevertsID != first.ID {
		t.Fatalf("the undo does not name what it undid: %+v", undo)
	}
	if len(repo.revisions) != 2 {
		t.Fatalf("got %d revisions, want the edit and its undo", len(repo.revisions))
	}
	reverted, err := repo.Get(context.Background(), first.ID, 1)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !reverted.Reverted() {
		t.Fatal("the reverted revision is not marked as undone")
	}
}

func TestAnUndoCanItselfBeUndone(t *testing.T) {
	repo := newFakeRepo()
	e, _ := newEditor(repo, nil)
	cvID := repo.cvID

	first := commitSet(t, e, repo, ActorCandidate, OriginEditor, "summary", "Distributed systems")
	_, undo, err := e.Revert(context.Background(), cvID, 1, first.ID)
	if err != nil {
		t.Fatalf("Revert: %v", err)
	}

	if _, _, err := e.Revert(context.Background(), cvID, 1, undo.ID); err != nil {
		t.Fatalf("Revert(undo): %v", err)
	}
	if repo.state.Summary != "Distributed systems" {
		t.Fatalf("summary = %q, want the original edit back", repo.state.Summary)
	}
}

func TestUndoingTwiceIsRefused(t *testing.T) {
	repo := newFakeRepo()
	e, _ := newEditor(repo, nil)
	cvID := repo.cvID

	first := commitSet(t, e, repo, ActorCandidate, OriginEditor, "summary", "Distributed systems")
	if _, _, err := e.Revert(context.Background(), cvID, 1, first.ID); err != nil {
		t.Fatalf("Revert: %v", err)
	}

	_, _, err := e.Revert(context.Background(), cvID, 1, first.ID)
	if !errors.Is(err, ErrNothingToUndo) {
		t.Fatalf("second Revert returned %v, want ErrNothingToUndo", err)
	}
}

func TestUndoingWhatIsNoLongerThereExplainsItself(t *testing.T) {
	repo := newFakeRepo()
	e, c := newEditor(repo, nil)
	cvID := repo.cvID

	// Rewrite a bullet, then delete the whole entry it lived in: the inverse would restore
	// text into a place that no longer exists.
	_, rewrite, err := e.Commit(context.Background(), cvID, 1, Change{
		Actor: ActorCandidate, Origin: OriginEditor,
		Ops: []Op{{Kind: OpSet, Path: mustParse(t, "experience[1].bullets[0]"), Value: "Learned a lot"}},
	})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	c.at = c.at.Add(2 * time.Minute)
	if _, _, err := e.Commit(context.Background(), cvID, 1, Change{
		Actor: ActorCandidate, Origin: OriginEditor,
		Ops: []Op{{Kind: OpRemove, Path: mustParse(t, "experience[1]")}},
	}); err != nil {
		t.Fatalf("Commit(remove): %v", err)
	}

	_, _, err = e.Revert(context.Background(), cvID, 1, rewrite.ID)
	if !errors.Is(err, ErrCannotUndo) {
		t.Fatalf("Revert returned %v, want ErrCannotUndo", err)
	}
	if len(repo.state.Experience) != 1 {
		t.Fatal("a refused undo changed the document")
	}
}

func TestARunIsUndoneAsOne(t *testing.T) {
	repo := newFakeRepo()
	e, c := newEditor(repo, nil)
	cvID := repo.cvID
	batch := uuid.New()

	for i, edit := range []struct{ path, value string }{
		{"summary", "Kubernetes at scale"},
		{"experience[0].bullets[0]", "Ran the cluster"},
		{"experience[1].bullets[0]", "Learned Kubernetes"},
	} {
		c.at = c.at.Add(time.Duration(i+1) * time.Minute)
		if _, _, err := e.Commit(context.Background(), cvID, 1, Change{
			Actor: ActorAgent, Origin: OriginTailorAgent, BatchID: batch,
			Ops: []Op{{Kind: OpSet, Path: mustParse(t, edit.path), Value: edit.value}},
		}); err != nil {
			t.Fatalf("Commit(%s): %v", edit.path, err)
		}
	}

	if _, err := e.RevertBatch(context.Background(), cvID, 1, batch); err != nil {
		t.Fatalf("RevertBatch: %v", err)
	}

	before := sample()
	if repo.state.Summary != before.Summary ||
		repo.state.Experience[0].Bullets[0] != before.Experience[0].Bullets[0] ||
		repo.state.Experience[1].Bullets[0] != before.Experience[1].Bullets[0] {
		t.Fatalf("the run was not fully undone: %+v", repo.state)
	}
}

func TestUndoingOneEditOfARunLeavesTheRest(t *testing.T) {
	repo := newFakeRepo()
	e, c := newEditor(repo, nil)
	cvID := repo.cvID
	batch := uuid.New()

	_, first, err := e.Commit(context.Background(), cvID, 1, Change{
		Actor: ActorAgent, Origin: OriginTailorAgent, BatchID: batch,
		Ops: []Op{{Kind: OpSet, Path: mustParse(t, "summary"), Value: "Kubernetes at scale"}},
	})
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	c.at = c.at.Add(2 * time.Minute)
	if _, _, err := e.Commit(context.Background(), cvID, 1, Change{
		Actor: ActorAgent, Origin: OriginTailorAgent, BatchID: batch,
		Ops: []Op{{Kind: OpSet, Path: mustParse(t, "experience[0].bullets[0]"), Value: "Ran the cluster"}},
	}); err != nil {
		t.Fatalf("Commit: %v", err)
	}

	if _, _, err := e.Revert(context.Background(), cvID, 1, first.ID); err != nil {
		t.Fatalf("Revert: %v", err)
	}

	if repo.state.Summary != "Ten years of Go" {
		t.Fatalf("summary = %q, want the undone edit reversed", repo.state.Summary)
	}
	if repo.state.Experience[0].Bullets[0] != "Ran the cluster" {
		t.Fatalf("the run's other edit was lost: %q", repo.state.Experience[0].Bullets[0])
	}
}

func TestUndoingARunThatNeverHappenedIsRefused(t *testing.T) {
	repo := newFakeRepo()
	e, _ := newEditor(repo, nil)

	_, err := e.RevertBatch(context.Background(), uuid.New(), 1, uuid.New())
	if !errors.Is(err, ErrNothingToUndo) {
		t.Fatalf("RevertBatch returned %v, want ErrNothingToUndo", err)
	}
}

// A revision id names an entry in ONE CV's history. Undoing it through a different CV of the
// same owner would apply that CV's inverses to this document — a write to the wrong place,
// driven entirely by a path parameter.
func TestUndoingARevisionOfAnotherCVIsRefused(t *testing.T) {
	repo := newFakeRepo()
	e, _ := newEditor(repo, nil)
	theirCV, otherCV := uuid.New(), uuid.New()

	rev := commitSet(t, e, repo, ActorCandidate, OriginEditor, "summary", "Distributed systems")

	_, _, err := e.Revert(context.Background(), otherCV, 1, rev.ID)
	if !errors.Is(err, ErrNothingToUndo) {
		t.Fatalf("undoing another CV's revision returned %v, want ErrNothingToUndo", err)
	}
	_ = theirCV
	if repo.state.Summary != "Distributed systems" {
		t.Fatalf("the wrong document was rewritten: %q", repo.state.Summary)
	}
}

// Undoing a reorder must reverse the reorder and nothing else. Deriving the inverse from a
// diff cannot express that: the differ has no `move` in its vocabulary, so a reorder's inverse
// came back as field-by-field rewrites of every entry the move touched — and applying those
// overwrote whatever had been edited since.
func TestUndoingAMoveLeavesLaterEditsAlone(t *testing.T) {
	repo := newFakeRepo()
	e, c := newEditor(repo, nil)
	to := 1

	_, reorder, err := e.Commit(context.Background(), repo.cvID, 1, Change{
		Actor: ActorCandidate, Origin: OriginEditor,
		Ops: []Op{{Kind: OpMove, Path: mustParse(t, "experience[0]"), To: &to}},
	})
	if err != nil {
		t.Fatalf("Commit(move): %v", err)
	}
	if repo.state.Experience[0].Company != "Initech" {
		t.Fatalf("the move did not happen: %+v", repo.state.Experience)
	}

	// A later edit to the entry that moved.
	c.at = c.at.Add(2 * time.Minute)
	commitSet(t, e, repo, ActorCandidate, OriginEditor, "experience[1].role", "Staff Engineer")

	if _, _, err := e.Revert(context.Background(), repo.cvID, 1, reorder.ID); err != nil {
		t.Fatalf("Revert: %v", err)
	}

	if repo.state.Experience[0].Company != "Acme" {
		t.Fatalf("the reorder was not undone: %+v", repo.state.Experience)
	}
	if got := repo.state.Experience[0].Role; got != "Staff Engineer" {
		t.Fatalf("role = %q, want the later edit to have survived the undo", got)
	}
}

// A foreign edit that lands between two calls of the same run and reshapes the list they both
// address must refuse the run's undo rather than silently misplace what it restores: the
// batch's stored inverse still names a position in the list as it stood before the foreign
// insert, and that position is in range for the list's new shape too — just not the same
// element anymore.
func TestRevertBatchRefusesWhenAForeignEditReshapedTheSameList(t *testing.T) {
	repo := newFakeRepo()
	e, c := newEditor(repo, nil)
	cvID := repo.cvID
	batch := uuid.New()

	// The run's first call: drop the second bullet.
	c.at = c.at.Add(time.Minute)
	if _, _, err := e.Commit(context.Background(), cvID, 1, Change{
		Actor: ActorAgent, Origin: OriginTailorAgent, BatchID: batch,
		Ops: []Op{{Kind: OpRemove, Path: mustParse(t, "experience[0].bullets[1]")}},
	}); err != nil {
		t.Fatalf("Commit(remove): %v", err)
	}

	// A foreign edit lands between the run's two calls, inserting ahead of the position the
	// run's next call and its own earlier inverse both address.
	c.at = c.at.Add(time.Minute)
	if _, _, err := e.Commit(context.Background(), cvID, 1, Change{
		Actor: ActorCandidate, Origin: OriginEditor,
		Ops: []Op{{Kind: OpInsert, Path: mustParse(t, "experience[0].bullets[0]"), Value: "Candidate's own line"}},
	}); err != nil {
		t.Fatalf("Commit(foreign insert): %v", err)
	}

	// The run's second call: it reads the fresh state, so this write itself lands correctly.
	c.at = c.at.Add(time.Minute)
	if _, _, err := e.Commit(context.Background(), cvID, 1, Change{
		Actor: ActorAgent, Origin: OriginTailorAgent, BatchID: batch,
		Ops: []Op{{Kind: OpSet, Path: mustParse(t, "experience[0].bullets[0]"), Value: "Shipped it, tailored"}},
	}); err != nil {
		t.Fatalf("Commit(set): %v", err)
	}
	before := append([]string(nil), repo.state.Experience[0].Bullets...)

	if _, err := e.RevertBatch(context.Background(), cvID, 1, batch); !errors.Is(err, ErrCannotUndo) {
		t.Fatalf("RevertBatch = %v, want ErrCannotUndo", err)
	}
	if !reflect.DeepEqual(repo.state.Experience[0].Bullets, before) {
		t.Fatalf("a refused undo changed the document: %v", repo.state.Experience[0].Bullets)
	}
}
