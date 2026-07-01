package handler

import (
	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/jobview"
)

// ListJobs returns a page of jobs using limit/offset pagination. Jobs are
// served in the shared jobview wire shape (public_slug, no internal id) — the
// same shape the detail and search endpoints use. The page rides the partial
// index jobs_open_created_idx (no full-table sort) and meta.total is an
// approximate planner estimate (EstimateOpenJobs), so neither query scans the
// whole open-job set at catalogue scale.
func (a *API) ListJobs(c *fiber.Ctx) error {
	limit, offset := pageParams(c)

	jobs, err := a.queries.ListJobs(c.Context(), db.ListJobsParams{
		Limit:  int32(limit),
		Offset: int32(offset),
	})
	if err != nil {
		return err
	}

	total, err := a.queries.EstimateOpenJobs(c.Context())
	if err != nil {
		return err
	}

	views, err := jobview.FromRows(jobs)
	if err != nil {
		return err
	}

	return listResponse(c, views, total, limit, offset)
}

// GetJob returns a single job addressed by its public slug.
func (a *API) GetJob(c *fiber.Ctx) error {
	job, err := a.queries.GetJobBySlug(c.Context(), c.Params("slug"))
	if err != nil {
		// RenderError maps pgx.ErrNoRows to 404, anything else to 500.
		return err
	}

	view, err := jobview.FromRow(job)
	if err != nil {
		return err
	}

	return c.JSON(fiber.Map{"data": view})
}
