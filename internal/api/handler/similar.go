package handler

import (
	"context"
	"slices"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/search/search"
)

// unseenWindows is search's unseen-posting rule, resolved once from the adapter taxonomy:
// a recommendation must not send anyone to a posting search itself withholds because its
// source stopped listing it.
var unseenWindows = search.UnseenWindows(sources.UnseenWindows(sources.Taxonomy(), search.UnseenWindow))

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
// neighbour may have closed since, or (rarely) been hard-deleted,
// leaving a dangling id the FK reference no longer covers because the id sits in
// another job's array column, not in a row the delete itself touches. Both cases
// are tolerated silently — the id is just dropped, not treated as an error — and
// the nearest-first order is preserved across the drop by re-sorting the fetched
// rows to the original id order rather than trusting GetJobsByIDs' own order,
// which is undefined for a WHERE id = ANY($1) fetch.
//
// A private posting is dropped the same way. NearestJobsToJob now refuses to name
// one (semantic.sql), but that only governs lists computed from here on: a
// user-pasted job description from the JD-tailor intake is a technical, open,
// embedded row, so it was a legitimate neighbour of public postings and the
// stored lists still hold it. This endpoint is unauthenticated and answers with
// the whole jobview, so publishing one would publish the unguessable slug its
// privacy rests on.
func (h *searchHandlers) SimilarJobs(c *fiber.Ctx) error {
	id, err := h.queries.GetJobIDBySlug(c.Context(), c.Params("slug"))
	if err != nil {
		// RenderError maps pgx.ErrNoRows to 404, anything else to 500.
		return err
	}

	limit := min(max(c.QueryInt("limit", defaultSimilarLimit), 1), maxSimilarLimit)

	ordered, err := similarViews(c.Context(), h.queries, id, limit, func(row db.Job) bool {
		return !row.ClosedAt.Valid && !row.IsPrivate
	})
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": ordered})
}

// similarReader is the two reads a similar-jobs answer needs. *db.Queries satisfies it.
type similarReader interface {
	GetSimilarJobIDs(ctx context.Context, id int64) (db.GetSimilarJobIDsRow, error)
	GetJobsByIDs(ctx context.Context, ids []int64) ([]db.Job, error)
}

// similarViews returns up to limit of the job's precomputed neighbours that show admits,
// nearest first. It is shared by GET /jobs/:slug/similar and the ChatGPT app's
// similar_jobs; each passes its own visibility rule, and the app's is stricter (it also
// drops duplicates, as every one of its tools does).
//
// Whatever show admits, a neighbour its source has stopped listing is dropped too
// (search.UnseenWindows): it is withheld from search, and a recommendation is a search
// result by another name. The posting's own page still serves it.
//
// It fetches the whole stored (nearest-first, capped at cmd/similar-backfill's -similar
// count) list, not just the first `limit` ids: some may turn out closed or dangling, and
// truncating before the filter would silently starve a limit=maxSimilarLimit request even
// though enough open neighbours exist further down the stored list. The nearest-first order
// is restored from the stored ids rather than trusted from GetJobsByIDs, whose order is
// undefined for a WHERE id = ANY($1) fetch.
//
// Neighbours sharing a country with the source come first, each group keeping that
// nearest-first order (freehire#3874). The stored list is computed by meaning alone, and
// measured on prod 2026-10-30 only 2.9 of the 6 shown shared the source's country while
// 46% of postings had six such neighbours further down their own list. Reordering here
// rather than filtering in NearestJobsToJob is deliberate: that query runs ~8s per job on
// a disk-bound host, so a geographic predicate there means re-computing ~1.2M lists.
// A source with no country orders as before.
func similarViews(ctx context.Context, q similarReader, id int64, limit int, show func(db.Job) bool) ([]jobview.Job, error) {
	source, err := q.GetSimilarJobIDs(ctx, id)
	if err != nil {
		return nil, err
	}
	similarIDs := source.SimilarJobIds
	if len(similarIDs) == 0 {
		return []jobview.Job{}, nil
	}

	rows, err := q.GetJobsByIDs(ctx, similarIDs)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	byID := make(map[int64]int, len(rows))
	for i, row := range rows {
		if show(row) && !unseenWindows.Unseen(row, now) {
			byID[row.ID] = i
		}
	}

	local := make([]db.Job, 0, len(byID))
	var elsewhere []db.Job
	for _, sid := range similarIDs {
		i, ok := byID[sid]
		if !ok {
			continue // not shown, or a dangling id (hard-deleted, stale backfill entry).
		}
		if sharesCountry(rows[i].Countries, source.Countries) {
			local = append(local, rows[i])
		} else {
			elsewhere = append(elsewhere, rows[i])
		}
	}

	picked := append(local, elsewhere...)
	picked = picked[:min(len(picked), limit)]
	ordered := make([]jobview.Job, 0, len(picked))
	for _, row := range picked {
		view, err := jobview.FromRow(row)
		if err != nil {
			return nil, err
		}
		ordered = append(ordered, view)
	}
	return ordered, nil
}

// sharesCountry reports whether two country-code lists overlap.
func sharesCountry(a, b []string) bool {
	for _, x := range a {
		if slices.Contains(b, x) {
			return true
		}
	}
	return false
}
