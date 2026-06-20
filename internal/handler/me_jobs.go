package handler

import (
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/jobview"
)

// myJobResponse is one item of the my-jobs listing: the job in the shared
// jobview wire shape with the caller's interaction timestamps riding alongside
// (not flattened in — the job shape stays identical to every other job surface).
type myJobResponse struct {
	Job       jobview.Job `json:"job"`
	ViewedAt  *time.Time  `json:"viewed_at"`
	SavedAt   *time.Time  `json:"saved_at"`
	AppliedAt *time.Time  `json:"applied_at"`
	Stage     *string     `json:"stage"`
	Notes     *string     `json:"notes"`
}

// ListMyJobs returns the authenticated user's job interactions joined with the
// jobs, most recently touched first, narrowed by ?filter=all|viewed|saved|applied|board
// (default all; viewed is the view-only subset — neither saved nor applied;
// board is the Kanban view — jobs with saved_at, applied_at, or stage set).
// meta carries total/limit/offset for the active filter plus the per-filter
// counts for the tab badges — which is also why this writes its own envelope
// instead of listResponse. Closed jobs stay listed: a user's history must not
// shrink when a posting closes.
func (a *API) ListMyJobs(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}

	limit, offset := pageParams(c)
	listing, err := a.tracking.ListTracked(c.Context(), userID, c.Query("filter"), int32(limit), int32(offset))
	if err != nil {
		return trackingError(err)
	}

	items := make([]myJobResponse, 0, len(listing.Items))
	for _, it := range listing.Items {
		items = append(items, myJobResponse{
			Job:       it.Job,
			ViewedAt:  it.ViewedAt,
			SavedAt:   it.SavedAt,
			AppliedAt: it.AppliedAt,
			Stage:     it.Stage,
			Notes:     it.Notes,
		})
	}

	return c.JSON(fiber.Map{
		"data": items,
		"meta": fiber.Map{
			"total":  listing.Total(),
			"limit":  limit,
			"offset": offset,
			"counts": fiber.Map{
				"all":     listing.Counts.All,
				"viewed":  listing.Counts.Viewed,
				"saved":   listing.Counts.Saved,
				"applied": listing.Counts.Applied,
				"board":   listing.Counts.Board,
			},
		},
	})
}

// ListViewedSlugs returns the set of public job slugs the authenticated caller
// has interacted with (every user_jobs row counts as viewed). The SPA reads this
// to dim already-seen cards in the browse list and search results without
// authenticating the public job-read path — viewed state is cross-referenced
// client-side, never joined into ListJobs/SearchJobs. The response is a flat
// {"data": [slug, ...]} list scoped to the caller.
func (a *API) ListViewedSlugs(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}

	slugs, err := a.tracking.ViewedSlugs(c.Context(), userID)
	if err != nil {
		return err
	}

	return c.JSON(fiber.Map{"data": slugs})
}
