package main

import (
	"context"
	"fmt"
)

// writer is the write side of a merge, narrowed to the two statements an apply run makes so
// the loop below can be exercised without a database.
type writer interface {
	// InsertAlias records the retirement. ON CONFLICT DO NOTHING at the SQL layer, so a
	// re-run neither errors nor moves a canon that is already frozen.
	InsertAlias(ctx context.Context, a alias, canonical, foldedKey string) error
	// RekeyChunk moves up to chunk of the alias's jobs onto the canonical slug and reports
	// how many it moved. 0 means the slug is drained.
	RekeyChunk(ctx context.Context, aliasSlug, canonical string, chunk int) (int64, error)
}

// applyMerges performs a planned wave and returns how many job rows moved.
//
// The alias row is written BEFORE its jobs move, and that order is the durability story: a
// run killed between the two leaves a slug that still holds its jobs and already redirects —
// a redirect to a company whose count is short. The other order leaves a slug with no jobs
// and no record of where they went, which is a 404 and an unanswerable support question.
//
// Jobs move in chunks so no single statement holds a long transaction over a table where one
// company can carry twenty thousand rows. The chunk statement selects rows still carrying the
// retired slug, so an updated row leaves the set: the loop terminates on its own, a re-run
// moves nothing, and a wave stopped half-way simply resumes.
func applyMerges(ctx context.Context, w writer, plan []merge, chunk int) (int64, error) {
	var moved int64
	for _, m := range plan {
		for _, a := range m.Aliases {
			if err := w.InsertAlias(ctx, a, m.Canonical, m.FoldedKey); err != nil {
				return moved, fmt.Errorf("record alias %s -> %s: %w", a.Slug, m.Canonical, err)
			}
			for {
				n, err := w.RekeyChunk(ctx, a.Slug, m.Canonical, chunk)
				if err != nil {
					return moved, fmt.Errorf("re-key %s -> %s: %w", a.Slug, m.Canonical, err)
				}
				if n == 0 {
					break
				}
				moved += n
				if err := ctx.Err(); err != nil {
					// A cron timeout or redeploy: stop where we are rather than fighting
					// the signal. What moved is committed, and the next run resumes.
					return moved, err
				}
			}
		}
	}
	return moved, nil
}
