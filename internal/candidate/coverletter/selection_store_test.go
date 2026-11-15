package coverletter

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/strelov1/freehire/internal/candidate/experience"
)

type selectionRepo struct {
	fakeRepo
	selection *Selection
}

func (r *selectionRepo) GetSelection(_ context.Context, userID, jobID int64) (Selection, error) {
	r.gotUser, r.gotJob = userID, jobID
	if r.selection == nil {
		return Selection{}, pgx.ErrNoRows
	}
	return *r.selection, nil
}
func (r *selectionRepo) SaveSelection(_ context.Context, userID, jobID, revision int64, ids []uuid.UUID) (Selection, error) {
	r.gotUser, r.gotJob = userID, jobID
	if r.selection == nil && revision != 0 || r.selection != nil && r.selection.Revision != revision {
		return Selection{}, pgx.ErrNoRows
	}
	r.selection = &Selection{Revision: revision + 1, AtomIDs: ids}
	return *r.selection, nil
}
func TestSelectionStorePreservesEmptyAndDetectsConflicts(t *testing.T) {
	ctx := context.Background()
	r := &selectionRepo{}
	s := NewStore(r)
	got, err := s.GetSelection(ctx, 42, 7)
	if err != nil || got != nil {
		t.Fatalf("absent: %v %v", got, err)
	}
	empty, err := s.SaveSelection(ctx, 42, 7, 0, []uuid.UUID{}, nil)
	if err != nil || empty.Revision != 1 || len(empty.AtomIDs) != 0 {
		t.Fatalf("empty: %v %v", empty, err)
	}
	if r.gotUser != 42 || r.gotJob != 7 {
		t.Fatal("ownership dropped")
	}
	if _, err = s.SaveSelection(ctx, 42, 7, 0, nil, nil); !errors.Is(err, ErrSelectionConflict) {
		t.Fatalf("first-save conflict: %v", err)
	}
	a := experience.Atom{ID: uuid.New(), Claim: "Did work", Provenance: experience.ProvenanceManual}
	if _, err = s.SaveSelection(ctx, 42, 7, 1, []uuid.UUID{a.ID}, nil); !errors.Is(err, ErrInvalidSelection) {
		t.Fatalf("foreign evidence: %v", err)
	}
	if _, err = s.SaveSelection(ctx, 42, 7, 1, []uuid.UUID{a.ID}, []experience.Atom{a}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.SaveSelection(ctx, 42, 7, 1, nil, nil); !errors.Is(err, ErrSelectionConflict) {
		t.Fatalf("update conflict: %v", err)
	}
}
