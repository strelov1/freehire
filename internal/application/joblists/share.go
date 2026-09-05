package joblists

import (
	"context"
	"errors"

	"github.com/strelov1/freehire/internal/dict/slugmint"
)

// Share publishes a job list as a public, read-only page, owner-scoped. It reads the
// list (a missing or non-owned id → ErrNotFound) and keeps an existing public slug on
// re-share or mints a readable one from the name otherwise. A slug collision is
// retried with a fresh suffix. Returns the updated row (with its public slug).
func (s *Service) Share(ctx context.Context, userID, id int64) (JobList, error) {
	list, err := s.repo.Get(ctx, id, userID)
	if err != nil {
		return JobList{}, err
	}

	// Re-share keeps the existing slug so a previously shared link stays valid.
	if list.PublicSlug != "" {
		return s.repo.SetPublicSlug(ctx, id, userID, list.PublicSlug)
	}

	for attempt := 0; attempt < maxShareAttempts; attempt++ {
		slug, err := slugmint.New(list.Name, slugFallbackBase)
		if err != nil {
			return JobList{}, err
		}
		row, err := s.repo.SetPublicSlug(ctx, id, userID, slug)
		if errors.Is(err, ErrSlugTaken) {
			continue // fresh suffix on the next attempt
		}
		if err != nil {
			return JobList{}, err
		}
		return row, nil
	}
	return JobList{}, ErrSlugTaken
}

// Unshare makes a shared list private again, owner-scoped. It is an idempotent no-op
// when the list is already private; a missing or non-owned id → ErrNotFound.
func (s *Service) Unshare(ctx context.Context, userID, id int64) error {
	return s.repo.ClearPublicSlug(ctx, id, userID)
}
