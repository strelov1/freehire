package coverletter

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/strelov1/freehire/internal/candidate/experience"
)

func (s *Store) GetSelection(ctx context.Context, userID, jobID int64) (*Selection, error) {
	row, err := s.repo.GetSelection(ctx, userID, jobID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *Store) SaveSelection(ctx context.Context, userID, jobID, revision int64, ids []uuid.UUID, bank []experience.Atom) (Selection, error) {
	if revision < 0 {
		return Selection{}, ErrInvalidSelection
	}
	if len(ids) > 0 {
		if _, err := ResolveSelection(ids, bank); err != nil {
			return Selection{}, err
		}
	}
	row, err := s.repo.SaveSelection(ctx, userID, jobID, revision, ids)
	if errors.Is(err, pgx.ErrNoRows) {
		return Selection{}, ErrSelectionConflict
	}
	return row, err
}

func (s Stored) EvidenceChanged(selection *Selection, bank []experience.Atom) bool {
	if s.EvidenceFingerprint == "" {
		return false
	}
	if selection == nil {
		return s.EvidenceRevision != 0
	}
	if selection.Revision != s.EvidenceRevision {
		return true
	}
	atoms, err := ResolveSelection(selection.AtomIDs, bank)
	return err != nil || EvidenceFingerprint(atoms) != s.EvidenceFingerprint
}
