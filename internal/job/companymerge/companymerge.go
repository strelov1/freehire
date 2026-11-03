// Package companymerge is the write side of retiring one company slug into another. Both
// writers share it: cmd/merge-companies (the rule's waves and the curated list) and a
// moderator's approval (internal/job/companyrelation). They cannot drift, which matters
// because the procedure has to survive being interrupted. See
// docs/agents/company-identity.md for why each step is ordered as it is.
package companymerge

import (
	"context"
	"fmt"

	"github.com/strelov1/freehire/internal/platform/db"
)

// Chunk is how many job rows one re-key statement moves. One company can carry twenty
// thousand jobs, and each statement is a transaction against the hottest table in the schema.
const Chunk = 500

// Retire records that alias now redirects to canonical, and carries the alias's subsidiaries
// over to the canon. It must run BEFORE the jobs move. A run killed in between then leaves a
// slug that redirects to a company whose count is short. The other order leaves a slug with
// no jobs and no record of where they went.
//
// The alias insert is ON CONFLICT DO NOTHING (the canon freezes at first merge). foldedKey is
// the ALIAS's own fold of its name, which is the key ingest will look the spelling up by.
func Retire(ctx context.Context, q *db.Queries, alias, canonical, foldedKey, reason string) error {
	if _, err := q.InsertCompanySlugAlias(ctx, db.InsertCompanySlugAliasParams{
		AliasSlug:     alias,
		CanonicalSlug: canonical,
		FoldedKey:     foldedKey,
		Reason:        reason,
	}); err != nil {
		return err
	}
	// The alias's subsidiaries move to the canon. The alias's own parent link, and a link
	// that would now point the canon at itself, go: one level is the rule, and the canon
	// may already have a parent or be one.
	if err := q.RepointCompanyParentsOnMerge(ctx, db.RepointCompanyParentsOnMergeParams{
		CanonicalSlug: canonical,
		AliasSlug:     alias,
	}); err != nil {
		return err
	}
	return q.DeleteCompanyParentLinksOf(ctx, alias)
}

// RekeyFunc moves up to chunk of alias's jobs onto canonical and reports how many it moved.
type RekeyFunc func(ctx context.Context, alias, canonical string, chunk int) (int64, error)

// Rekey is the RekeyFunc over the real queries.
func Rekey(q *db.Queries) RekeyFunc {
	return func(ctx context.Context, alias, canonical string, chunk int) (int64, error) {
		return q.RekeyCompanySlugChunk(ctx, db.RekeyCompanySlugChunkParams{
			CanonicalSlug: canonical,
			AliasSlug:     alias,
			ChunkSize:     int32(chunk),
		})
	}
}

// MoveJobs drains alias's jobs onto canonical, a chunk at a time. The chunk statement
// selects rows still carrying the old slug, so a moved row leaves the set. The loop ends on
// its own, a re-run moves nothing, and a stopped run resumes. It stops between chunks when
// ctx ends: what moved is committed.
func MoveJobs(ctx context.Context, rekey RekeyFunc, alias, canonical string, chunk int) (int64, error) {
	var moved int64
	for {
		n, err := rekey(ctx, alias, canonical, chunk)
		if err != nil {
			return moved, fmt.Errorf("re-key %s -> %s: %w", alias, canonical, err)
		}
		if n == 0 {
			return moved, nil
		}
		moved += n
		if err := ctx.Err(); err != nil {
			return moved, err
		}
	}
}

// Reconcile gives each canonical slug that holds jobs a companies row, then removes the row
// of each retired slug no job carries. Until that row is gone, GET /companies/<retired>
// still finds a company with no jobs and answers 200 instead of the 301. Both statements are
// scoped to the alias registry, so a run killed before this step is healed by the next one.
func Reconcile(ctx context.Context, q *db.Queries) (created, removed int64, err error) {
	if created, err = q.InsertMissingCanonicalCompanies(ctx); err != nil {
		return 0, 0, fmt.Errorf("insert canonical companies: %w", err)
	}
	if removed, err = q.DeleteOrphanAliasCompanies(ctx); err != nil {
		return created, 0, fmt.Errorf("delete retired companies: %w", err)
	}
	return created, removed, nil
}
