package handler

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/go-faster/jx"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/ingest/applyform"
)

// JobApplyForm serves the application form captured for the job addressed by :slug: the
// questions a candidate will have to answer. Public, like the other job reads.
//
// Two queries rather than one join, mirroring JobCopies: an unknown slug fails at the first, a
// posting with no captured form at the second with its own message. Both are 404s but stay
// distinguishable ("this employer asks nothing" vs "we cannot read this platform").
//
// Forms are captured for only a minority of postings, so the 404 is the ordinary case, not an
// error worth logging.
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
	// The provider is authoritative on the row, not inside the payload: a re-capture rewrites the
	// row and a per-provider purge selects on it.
	form.Provider = row.Provider

	raw, err := json.Marshal(form.ForDisplay())
	if err != nil {
		return nil, err
	}
	return &apigen.JobApplyFormOK{Data: jx.Raw(raw)}, nil
}
