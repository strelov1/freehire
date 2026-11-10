package handler

import (
	"encoding/json"
	"fmt"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ingest/applyform"
)

// JobApplyForm serves the application form captured for the job addressed by :slug —
// the questions a candidate will have to answer, shaped for reading. Public
// (unauthenticated) like the other job reads. Response: {"data": {...}}.
//
// Two queries rather than one join, mirroring JobCopies on the same resource: an
// unknown slug fails at the first exactly as /copies does, while a posting that exists
// but has no captured form fails at the second with its own message. Both are 404s, and
// keeping them distinguishable matters — "this employer asks nothing" and "we cannot
// read this platform" are different statements, and only the second is usually true.
//
// A form is captured for a minority of postings (the platforms whose forms can be read
// at all are roughly a sixth of technical ones), so the 404 is the ordinary case rather
// than an error worth logging.
func (h *jobsHandlers) JobApplyForm(c *fiber.Ctx) error {
	job, err := h.queries.GetJobBySlug(c.Context(), c.Params("slug"))
	if err != nil {
		return err
	}
	if err := jobVisibleToCaller(job, 0, false); err != nil {
		return err
	}

	row, err := h.queries.GetApplyFormByJobID(c.Context(), job.ID)
	if err != nil {
		return err
	}

	var form applyform.Form
	if err := json.Unmarshal(row.Payload, &form); err != nil {
		return fmt.Errorf("decode apply form for job %d: %w", job.ID, err)
	}
	// The provider is authoritative on the row rather than inside the payload: the row
	// is what a re-capture rewrites and what a per-provider purge would select on.
	form.Provider = row.Provider

	return c.JSON(fiber.Map{"data": form.ForDisplay()})
}
