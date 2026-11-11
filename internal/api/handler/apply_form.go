package handler

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/go-faster/jx"

	"github.com/strelov1/freehire/internal/api/apigen"
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
func (h *jobsHandlers) JobApplyForm(ctx context.Context, params apigen.JobApplyFormParams) (*apigen.JobApplyFormOK, error) {
	job, err := h.queries.GetJobBySlug(ctx, params.Slug)
	if err != nil {
		return nil, err
	}
	if err := jobVisibleToCaller(job, 0, false); err != nil {
		return nil, err
	}

	row, err := h.queries.GetApplyFormByJobID(ctx, job.ID)
	if err != nil {
		return nil, err
	}

	var form applyform.Form
	if err := json.Unmarshal(row.Payload, &form); err != nil {
		return nil, fmt.Errorf("decode apply form for job %d: %w", job.ID, err)
	}
	// The provider is authoritative on the row rather than inside the payload: the row
	// is what a re-capture rewrites and what a per-provider purge would select on.
	form.Provider = row.Provider

	raw, err := json.Marshal(form.ForDisplay())
	if err != nil {
		return nil, err
	}
	return &apigen.JobApplyFormOK{Data: jx.Raw(raw)}, nil
}
