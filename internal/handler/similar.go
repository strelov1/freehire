package handler

import (
	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/jobview"
)

// defaultSimilarLimit / maxSimilarLimit bound how many neighbours the similar-jobs
// endpoint returns: a small default sized for a "Similar jobs" row on the detail
// page, capped so a client cannot ask for an unbounded fan-out of embedder queries.
const (
	defaultSimilarLimit = 6
	maxSimilarLimit     = 20
)

// SimilarJobs returns jobs semantically nearest to the one addressed by :slug,
// from the semantic index. Public (unauthenticated) like the other job reads.
// Response: {"data": [job view...]} — neighbours carry public_slug and never the
// internal id, and the source job is excluded by the search backend.
func (a *API) SimilarJobs(c *fiber.Ctx) error {
	if a.search == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "search is not available")
	}

	id, err := a.queries.GetJobIDBySlug(c.Context(), c.Params("slug"))
	if err != nil {
		// RenderError maps pgx.ErrNoRows to 404, anything else to 500.
		return err
	}

	limit := min(max(c.QueryInt("limit", defaultSimilarLimit), 1), maxSimilarLimit)
	hits, err := a.search.SimilarJobs(c.Context(), id, limit)
	if err != nil {
		return err
	}

	views := make([]jobview.Job, len(hits))
	for i, hit := range hits {
		views[i] = hit.Job
	}

	return c.JSON(fiber.Map{"data": views})
}
