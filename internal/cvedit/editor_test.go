package cvedit

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/cv"
)

// fakeRepo is an in-memory Repository, enough to unit-test the editor without a database.
// It models the one thing that matters for correctness here: a commit reads, writes and
// records against a single consistent view.
type fakeRepo struct {
	state     State
	updatedAt time.Time
	revisions []Revision
	saves     int
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{state: sample(), updatedAt: time.Date(2026, 7, 31, 9, 0, 0, 0, time.UTC)}
}

func (r *fakeRepo) Edit(ctx context.Context, _ uuid.UUID, _ int64, fn func(context.Context, Tx) error) error {
	return fn(ctx, r)
}

func (r *fakeRepo) List(_ context.Context, _ uuid.UUID, _ int64, limit int32) ([]Revision, error) {
	out := make([]Revision, 0, len(r.revisions))
	for i := len(r.revisions) - 1; i >= 0 && len(out) < int(limit); i-- {
		out = append(out, r.revisions[i])
	}
	return out, nil
}

func (r *fakeRepo) Get(_ context.Context, id uuid.UUID, _ int64) (Revision, error) {
	for _, rev := range r.revisions {
		if rev.ID == id {
			return rev, nil
		}
	}
	return Revision{}, errors.New("no such revision")
}

func (r *fakeRepo) State(context.Context) (State, time.Time, error) {
	return r.state, r.updatedAt, nil
}

func (r *fakeRepo) Save(_ context.Context, s State) (cv.Meta, error) {
	r.state = s
	r.saves++
	r.updatedAt = r.updatedAt.Add(time.Second)
	return cv.Meta{Title: s.Title, TemplateID: s.TemplateID, UpdatedAt: r.updatedAt}, nil
}

func (r *fakeRepo) Newest(context.Context) (Revision, bool, error) {
	if len(r.revisions) == 0 {
		return Revision{}, false, nil
	}
	return r.revisions[len(r.revisions)-1], true, nil
}

func (r *fakeRepo) Revision(_ context.Context, id uuid.UUID) (Revision, bool, error) {
	for _, rev := range r.revisions {
		if rev.ID == id {
			return rev, true, nil
		}
	}
	return Revision{}, false, nil
}

func (r *fakeRepo) Insert(_ context.Context, rev Revision) (Revision, error) {
	rev.ID = uuid.New()
	rev.CreatedAt = r.updatedAt
	rev.UpdatedAt = r.updatedAt
	r.revisions = append(r.revisions, rev)
	return rev, nil
}

func (r *fakeRepo) Amend(_ context.Context, id uuid.UUID, ops []Op, title string) (Revision, error) {
	for i, rev := range r.revisions {
		if rev.ID == id {
			r.revisions[i].Ops = ops
			r.revisions[i].Title = title
			r.revisions[i].UpdatedAt = r.updatedAt
			return r.revisions[i], nil
		}
	}
	return Revision{}, errors.New("no such revision")
}

func (r *fakeRepo) MarkReverted(_ context.Context, id uuid.UUID) (bool, error) {
	for i, rev := range r.revisions {
		if rev.ID == id && rev.RevertedAt == nil {
			at := r.updatedAt
			r.revisions[i].RevertedAt = &at
			return true, nil
		}
	}
	return false, nil
}

func (r *fakeRepo) InBatch(_ context.Context, batchID uuid.UUID) ([]Revision, error) {
	var out []Revision
	for i := len(r.revisions) - 1; i >= 0; i-- {
		if r.revisions[i].BatchID == batchID && r.revisions[i].RevertedAt == nil {
			out = append(out, r.revisions[i])
		}
	}
	return out, nil
}

func (r *fakeRepo) Trim(_ context.Context, keep int32) error {
	if extra := len(r.revisions) - int(keep); extra > 0 {
		r.revisions = r.revisions[extra:]
	}
	return nil
}

// clock lets a test move time without sleeping, which is the only way to exercise the
// coalescing window.
type clock struct{ at time.Time }

func (c *clock) now() time.Time { return c.at }

func newEditor(repo *fakeRepo, gate EvidenceGate) (*Editor, *clock) {
	c := &clock{at: time.Date(2026, 7, 31, 9, 0, 0, 0, time.UTC)}
	e := NewEditor(repo, gate)
	e.now = c.now
	return e, c
}

func commitSet(t *testing.T, e *Editor, repo *fakeRepo, actor Actor, origin Origin, path, value string) Revision {
	t.Helper()
	_, rev, err := e.Commit(context.Background(), uuid.New(), 1, Change{
		Actor:  actor,
		Origin: origin,
		Ops:    []Op{{Kind: OpSet, Path: mustParse(t, path), Value: value}},
	})
	if err != nil {
		t.Fatalf("Commit(%s): %v", path, err)
	}
	return rev
}

func TestCommitChangesTheDocumentAndRecordsIt(t *testing.T) {
	repo := newFakeRepo()
	e, _ := newEditor(repo, nil)

	rev := commitSet(t, e, repo, ActorCandidate, OriginEditor, "summary", "Distributed systems")

	if repo.state.Summary != "Distributed systems" {
		t.Fatalf("document not changed: %q", repo.state.Summary)
	}
	if len(repo.revisions) != 1 {
		t.Fatalf("got %d revisions, want 1", len(repo.revisions))
	}
	if rev.Actor != ActorCandidate || rev.Origin != OriginEditor {
		t.Fatalf("revision does not record who made it: %+v", rev)
	}
	if len(rev.Inverse) != 1 || rev.Inverse[0].Value != "Ten years of Go" {
		t.Fatalf("revision does not carry the undo: %+v", rev.Inverse)
	}
	if rev.Title == "" {
		t.Fatal("revision has no description")
	}
}

func TestCommitRefusesAnEmptyChange(t *testing.T) {
	repo := newFakeRepo()
	e, _ := newEditor(repo, nil)

	if _, _, err := e.Commit(context.Background(), uuid.New(), 1, Change{Actor: ActorCandidate, Origin: OriginEditor}); err == nil {
		t.Fatal("an empty change committed, want a refusal")
	}
	if len(repo.revisions) != 0 || repo.saves != 0 {
		t.Fatal("an empty change wrote something")
	}
}

func TestCommitLeavesNothingBehindWhenTheBatchIsRefused(t *testing.T) {
	repo := newFakeRepo()
	e, _ := newEditor(repo, nil)

	_, _, err := e.Commit(context.Background(), uuid.New(), 1, Change{
		Actor:  ActorCandidate,
		Origin: OriginEditor,
		Ops: []Op{
			{Kind: OpSet, Path: mustParse(t, "summary"), Value: "Distributed systems"},
			{Kind: OpSet, Path: mustParse(t, "experience[9].role"), Value: "Never happened"},
		},
	})
	if err == nil {
		t.Fatal("Commit succeeded, want a refusal")
	}
	if repo.saves != 0 || len(repo.revisions) != 0 {
		t.Fatal("a refused batch wrote the document or the feed")
	}
	if repo.state.Summary != "Ten years of Go" {
		t.Fatalf("a refused batch changed the document: %q", repo.state.Summary)
	}
}

func TestTypingIntoOnePlaceCoalescesIntoOneRevision(t *testing.T) {
	repo := newFakeRepo()
	e, c := newEditor(repo, nil)

	commitSet(t, e, repo, ActorCandidate, OriginEditor, "summary", "Distributed")
	c.at = c.at.Add(2 * time.Second)
	commitSet(t, e, repo, ActorCandidate, OriginEditor, "summary", "Distributed systems")
	c.at = c.at.Add(2 * time.Second)
	commitSet(t, e, repo, ActorCandidate, OriginEditor, "summary", "Distributed systems at scale")

	if len(repo.revisions) != 1 {
		t.Fatalf("got %d revisions, want one for the burst", len(repo.revisions))
	}
	// The inverse still leads back to where the burst started, not to the previous keystroke.
	if got := repo.revisions[0].Inverse[0].Value; got != "Ten years of Go" {
		t.Fatalf("inverse = %v, want the text from before the first save", got)
	}
}

func TestEditingADifferentPlaceStartsANewRevision(t *testing.T) {
	repo := newFakeRepo()
	e, c := newEditor(repo, nil)

	commitSet(t, e, repo, ActorCandidate, OriginEditor, "summary", "Distributed systems")
	c.at = c.at.Add(time.Second)
	commitSet(t, e, repo, ActorCandidate, OriginEditor, "title", "Staff Engineer")

	if len(repo.revisions) != 2 {
		t.Fatalf("got %d revisions, want one per place", len(repo.revisions))
	}
}

func TestAPauseStartsANewRevision(t *testing.T) {
	repo := newFakeRepo()
	e, c := newEditor(repo, nil)

	commitSet(t, e, repo, ActorCandidate, OriginEditor, "summary", "Distributed")
	c.at = c.at.Add(5 * time.Minute)
	commitSet(t, e, repo, ActorCandidate, OriginEditor, "summary", "Distributed systems")

	if len(repo.revisions) != 2 {
		t.Fatalf("got %d revisions, want the pause to close the first", len(repo.revisions))
	}
}

func TestTheAgentDoesNotCoalesceIntoTheCandidatesRevision(t *testing.T) {
	repo := newFakeRepo()
	e, _ := newEditor(repo, nil)

	commitSet(t, e, repo, ActorCandidate, OriginEditor, "summary", "Distributed")
	commitSet(t, e, repo, ActorAgent, OriginTailorAgent, "summary", "Distributed systems")

	if len(repo.revisions) != 2 {
		t.Fatalf("got %d revisions, want the hands kept apart", len(repo.revisions))
	}
}

func TestCommitDocumentDerivesTheOperations(t *testing.T) {
	repo := newFakeRepo()
	e, _ := newEditor(repo, nil)

	next := sample()
	next.Summary = "Distributed systems"
	next.Experience[0].Bullets[1] = "Twice over"

	_, rev, err := e.CommitDocument(context.Background(), uuid.New(), 1, ActorCandidate, OriginEditor, next)
	if err != nil {
		t.Fatalf("CommitDocument: %v", err)
	}
	if len(rev.Ops) != 2 {
		t.Fatalf("got %d operations, want one per changed place: %+v", len(rev.Ops), rev.Ops)
	}
	if repo.state.Summary != "Distributed systems" || repo.state.Experience[0].Bullets[1] != "Twice over" {
		t.Fatalf("document not changed: %+v", repo.state)
	}
}

func TestCommitDocumentWithNothingChangedRecordsNothing(t *testing.T) {
	repo := newFakeRepo()
	e, _ := newEditor(repo, nil)

	if _, _, err := e.CommitDocument(context.Background(), uuid.New(), 1, ActorCandidate, OriginEditor, sample()); err != nil {
		t.Fatalf("CommitDocument: %v", err)
	}
	if len(repo.revisions) != 0 || repo.saves != 0 {
		t.Fatal("an unchanged save wrote something")
	}
}

func TestTheFeedIsTrimmed(t *testing.T) {
	repo := newFakeRepo()
	e, c := newEditor(repo, nil)

	for i := range feedLimit + 10 {
		c.at = c.at.Add(2 * time.Minute) // outside the window, so each stands alone
		commitSet(t, e, repo, ActorCandidate, OriginEditor, "summary", "Revision "+string(rune('a'+i%26)))
	}

	if len(repo.revisions) != feedLimit {
		t.Fatalf("feed holds %d revisions, want it trimmed to %d", len(repo.revisions), feedLimit)
	}
}

func TestAnOperationThatChangesNothingRecordsNothing(t *testing.T) {
	repo := newFakeRepo()
	e, _ := newEditor(repo, nil)

	// Picking the template already in use is a click the gallery allows.
	commitSet(t, e, repo, ActorCandidate, OriginTemplate, "template_id", repo.state.TemplateID)

	if len(repo.revisions) != 0 {
		t.Fatalf("got %d revisions for a change that changed nothing", len(repo.revisions))
	}
	if repo.saves != 0 {
		t.Fatal("a change that changed nothing wrote the document")
	}
}
