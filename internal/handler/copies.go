package handler

import (
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/db"
)

// defaultCopiesLimit / maxCopiesLimit bound the "openings across cities" list a
// collapsed job exposes, capped so a client cannot pull an unbounded cluster at once.
const (
	defaultCopiesLimit = 50
	maxCopiesLimit     = 200
)

// jobCopy is one posting in a role cluster — a single city's opening under a collapsed
// role. Each keeps its own location and apply URL so a seeker picks their city.
type jobCopy struct {
	PublicSlug string     `json:"public_slug"`
	Location   string     `json:"location"`
	ApplyURL   string     `json:"apply_url"`
	PostedAt   *time.Time `json:"posted_at"`
}

// JobCopies lists the open postings sharing the role cluster of the job addressed by
// :slug — the per-city openings folded under one canonical card by the content-dedup
// collapse. Public (unauthenticated) like the other job reads; the anchor itself is
// included (it is one of the openings). Response: {"data": [copy...]}.
func (a *API) JobCopies(c *fiber.Ctx) error {
	id, err := a.queries.GetJobIDBySlug(c.Context(), c.Params("slug"))
	if err != nil {
		// RenderError maps pgx.ErrNoRows to 404, anything else to 500.
		return err
	}

	limit := min(max(c.QueryInt("limit", defaultCopiesLimit), 1), maxCopiesLimit)
	offset := max(c.QueryInt("offset", 0), 0)
	rows, err := a.queries.ListRoleClusterCopies(c.Context(), db.ListRoleClusterCopiesParams{
		JobID:     id,
		RowLimit:  int32(limit),
		RowOffset: int32(offset),
	})
	if err != nil {
		return err
	}

	copies := make([]jobCopy, len(rows))
	for i, r := range rows {
		cp := jobCopy{PublicSlug: r.PublicSlug, Location: r.Location, ApplyURL: r.URL}
		if r.PostedAt.Valid {
			t := r.PostedAt.Time
			cp.PostedAt = &t
		}
		copies[i] = cp
	}

	// total is the whole cluster's open size (COUNT(*) OVER, pre-LIMIT), so the client's
	// "N openings" header stays accurate even when the list is a capped page.
	var total int64
	if len(rows) > 0 {
		total = rows[0].Total
	}
	return c.JSON(fiber.Map{"data": copies, "meta": fiber.Map{"total": total}})
}
