package handler

import (
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/job/outboundurl"
	"github.com/strelov1/freehire/internal/platform/db"
)

// defaultCopiesLimit / maxCopiesLimit bound the "openings across cities" list a
// collapsed job exposes, capped so a client cannot pull an unbounded cluster at once.
const (
	defaultCopiesLimit = 50
	maxCopiesLimit     = 200
)

// jobCopy is one posting a collapsed job represents — a single city's opening. Each keeps
// its own location and apply URL so a seeker picks their city.
type jobCopy struct {
	PublicSlug string     `json:"public_slug"`
	Location   string     `json:"location"`
	ApplyURL   string     `json:"apply_url"`
	PostedAt   *time.Time `json:"posted_at"`
}

// JobCopies lists the open postings represented by the job addressed by :slug — the per-city
// openings folded under one canonical card by the content-dedup collapse. Public
// (unauthenticated) like the other job reads.
//
// Membership is the duplicate closure, the same one the search document's geography union is
// built from, so a city a candidate can filter to is a city they can reach. The addressed job
// may itself be a suppressed posting (those stay readable by slug); the query resolves it to
// its owner and lists that owner's whole group rather than a fragment. Response:
// {"data": [copy...]}.
func (h *jobsHandlers) JobCopies(c *fiber.Ctx) error {
	id, err := h.queries.GetJobIDBySlug(c.Context(), c.Params("slug"))
	if err != nil {
		// RenderError maps pgx.ErrNoRows to 404, anything else to 500.
		return err
	}

	limit, offset := pageParamsBounded(c, defaultCopiesLimit, maxCopiesLimit)
	rows, err := h.queries.ListJobCopies(c.Context(), db.ListJobCopiesParams{
		JobID:     id,
		RowLimit:  int32(limit),
		RowOffset: int32(offset),
	})
	if err != nil {
		return err
	}

	copies := make([]jobCopy, len(rows))
	for i, r := range rows {
		cp := jobCopy{PublicSlug: r.PublicSlug, Location: r.Location, ApplyURL: outboundurl.Tag(r.URL)}
		if r.PostedAt.Valid {
			t := r.PostedAt.Time
			cp.PostedAt = &t
		}
		copies[i] = cp
	}

	// total is the whole closure's open size (COUNT(*) OVER, pre-LIMIT), so the client's
	// "N openings" header stays accurate even when the list is a capped page.
	var total int64
	if len(rows) > 0 {
		total = rows[0].Total
	}
	return c.JSON(fiber.Map{"data": copies, "meta": fiber.Map{"total": total}})
}
