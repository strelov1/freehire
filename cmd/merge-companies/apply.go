package main

import (
	"context"
	"fmt"

	"github.com/strelov1/freehire/internal/job/companymerge"
)

// writer is the write side of a merge, narrowed to the two statements an apply run makes so
// the loop below can be exercised without a database.
type writer interface {
	// InsertAlias records the retirement. ON CONFLICT DO NOTHING at the SQL layer, so a
	// re-run neither errors nor moves a canon that is already frozen. The folded key written
	// is the ALIAS's own (a.FoldedKey), which for a folded group is the group's key and for a
	// curated one is not — see the field's comment.
	InsertAlias(ctx context.Context, a alias, canonical string) error
	// RekeyChunk moves up to chunk of the alias's jobs onto the canonical slug and reports
	// how many it moved. 0 means the slug is drained.
	RekeyChunk(ctx context.Context, aliasSlug, canonical string, chunk int) (int64, error)
}

// applyMerges performs a planned wave and returns how many job rows moved.
//
// The alias row is written BEFORE its jobs move: a run killed between the two leaves a slug
// that still holds its jobs and already redirects. The other order leaves a slug with no
// jobs and no record of where they went (a 404).
//
// Jobs move in chunks so no statement holds a long transaction on a company with many rows.
// The chunk statement selects rows still carrying the retired slug, so the loop terminates,
// a re-run moves nothing, and a half-finished wave resumes.
func applyMerges(ctx context.Context, w writer, plan []merge, chunk int) (int64, error) {
	var moved int64
	for _, m := range plan {
		for _, a := range m.Aliases {
			if err := w.InsertAlias(ctx, a, m.Canonical); err != nil {
				return moved, fmt.Errorf("record alias %s -> %s: %w", a.Slug, m.Canonical, err)
			}
			// A cron timeout or redeploy stops this between chunks. What moved is
			// committed, and the next run resumes.
			n, err := companymerge.MoveJobs(ctx, w.RekeyChunk, a.Slug, m.Canonical, chunk)
			moved += n
			if err != nil {
				return moved, err
			}
		}
	}
	return moved, nil
}
