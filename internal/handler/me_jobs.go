package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/jobview"
)

// myJobResponse is one item of the my-jobs listing: the job in the shared
// jobview wire shape with the caller's interaction timestamps riding alongside
// (not flattened in — the job shape stays identical to every other job surface).
type myJobResponse struct {
	Job       jobview.Job        `json:"job"`
	ViewedAt  pgtype.Timestamptz `json:"viewed_at"`
	SavedAt   pgtype.Timestamptz `json:"saved_at"`
	AppliedAt pgtype.Timestamptz `json:"applied_at"`
	Stage     pgtype.Text        `json:"stage"`
	Notes     pgtype.Text        `json:"notes"`
}

// ListMyJobs returns the authenticated user's job interactions joined with the
// jobs, most recently touched first, narrowed by ?filter=all|viewed|saved|applied|board
// (default all; viewed is the view-only subset — neither saved nor applied;
// board is the Kanban view — jobs with saved_at, applied_at, or stage set).
// meta carries total/limit/offset for the active filter plus the per-filter
// counts for the tab badges — which is also why this writes its own envelope
// instead of listResponse. Closed jobs stay listed: a user's history must not
// shrink when a posting closes.
func (h *Handler) ListMyJobs(c *fiber.Ctx) error {
	userID, ok := auth.UserID(c)
	if !ok {
		return fiber.NewError(fiber.StatusUnauthorized, "unauthorized")
	}

	filter := c.Query("filter", "all")
	if filter != "all" && filter != "viewed" && filter != "saved" && filter != "applied" && filter != "board" {
		return fiber.NewError(fiber.StatusBadRequest, "filter must be one of: all, viewed, saved, applied, board")
	}
	limit, offset := pageParams(c)

	rows, err := h.queries.ListUserJobs(c.Context(), db.ListUserJobsParams{
		UserID: userID,
		Filter: filter,
		Limit:  int32(limit),
		Offset: int32(offset),
	})
	if err != nil {
		return err
	}
	counts, err := h.queries.CountUserJobs(c.Context(), userID)
	if err != nil {
		return err
	}

	items := make([]myJobResponse, 0, len(rows))
	for _, row := range rows {
		view, err := jobview.FromRow(row.Job)
		if err != nil {
			return err
		}
		items = append(items, myJobResponse{
			Job:       view,
			ViewedAt:  row.ViewedAt,
			SavedAt:   row.SavedAt,
			AppliedAt: row.AppliedAt,
			Stage:     row.Stage,
			Notes:     row.Notes,
		})
	}

	total := counts.All
	switch filter {
	case "viewed":
		total = counts.Viewed
	case "saved":
		total = counts.Saved
	case "applied":
		total = counts.Applied
	case "board":
		total = counts.Board
	}

	return c.JSON(fiber.Map{
		"data": items,
		"meta": fiber.Map{
			"total":  total,
			"limit":  limit,
			"offset": offset,
			"counts": fiber.Map{
				"all":     counts.All,
				"viewed":  counts.Viewed,
				"saved":   counts.Saved,
				"applied": counts.Applied,
				"board":   counts.Board,
			},
		},
	})
}
