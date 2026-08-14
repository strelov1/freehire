package handler

import (
	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/jobview"
)

// defaultSimilarLimit / maxSimilarLimit bound how many neighbours the similar-jobs
// endpoint returns: a small default sized for a "Similar jobs" row on the detail
// page, capped so a client cannot ask for an unbounded fan-out. maxSimilarLimit also
// matches cmd/similar-backfill's default -similar count, so a limit=maxSimilarLimit
// request is never starved by the precomputed list itself being shorter.
const (
	defaultSimilarLimit = 6
	maxSimilarLimit     = 20
)

// SimilarJobs returns jobs nearest to the one addressed by :slug, read from the
// precomputed jobs.similar_job_ids list (populated offline by cmd/similar-backfill
// from pgvector nearest-neighbour-over-chunks — see
// openspec/changes/drop-hybrid-search-pgvector-similar/design.md Decision 5).
// Public (unauthenticated) like the other job reads. Response: {"data": [job
// view...]} — neighbours carry public_slug and never the internal id.
//
// similar_job_ids is nearest-first but was computed at backfill time: a listed
// neighbour may have closed since, or (rarely) been hard-deleted by cmd/prune,
// leaving a dangling id the FK reference no longer covers because the id sits in
// another job's array column, not in a row cmd/prune itself touches. Both cases
// are tolerated silently — the id is just dropped, not treated as an error — and
// the nearest-first order is preserved across the drop by re-sorting the fetched
// rows to the original id order rather than trusting GetJobsByIDs' own order,
// which is undefined for a WHERE id = ANY($1) fetch.
func (h *searchHandlers) SimilarJobs(c *fiber.Ctx) error {
	id, err := h.queries.GetJobIDBySlug(c.Context(), c.Params("slug"))
	if err != nil {
		// RenderError maps pgx.ErrNoRows to 404, anything else to 500.
		return err
	}

	limit := min(max(c.QueryInt("limit", defaultSimilarLimit), 1), maxSimilarLimit)

	similarIDs, err := h.queries.GetSimilarJobIDs(c.Context(), id)
	if err != nil {
		return err
	}
	if len(similarIDs) == 0 {
		return c.JSON(fiber.Map{"data": []jobview.Job{}})
	}

	// Fetch the whole stored (nearest-first, capped at cmd/similar-backfill's
	// -similar count) list, not just the first `limit` ids: some of those ids may
	// turn out to be closed or dangling, and truncating before the open-status
	// filter would silently starve a limit=maxSimilarLimit request even though
	// enough open neighbours exist further down the stored list.
	rows, err := h.queries.GetJobsByIDs(c.Context(), similarIDs)
	if err != nil {
		return err
	}

	byID := make(map[int64]int, len(rows))
	for i, row := range rows {
		if row.ClosedAt.Valid {
			continue // closed since the list was computed — drop, don't error.
		}
		byID[row.ID] = i
	}

	ordered := make([]jobview.Job, 0, min(len(similarIDs), limit))
	for _, sid := range similarIDs {
		if len(ordered) == limit {
			break
		}
		i, ok := byID[sid]
		if !ok {
			continue // closed, or a dangling id (hard-deleted, stale backfill entry).
		}
		view, err := jobview.FromRow(rows[i])
		if err != nil {
			return err
		}
		ordered = append(ordered, view)
	}

	return c.JSON(fiber.Map{"data": ordered})
}
