package handler

import (
	"context"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/job/outboundurl"
	"github.com/strelov1/freehire/internal/platform/db"
)

// defaultCopiesLimit / maxCopiesLimit bound the "openings across cities" list a
// collapsed job exposes, capped so a client cannot pull an unbounded cluster at once.
const (
	defaultCopiesLimit = 50
	maxCopiesLimit     = 200
)

// JobCopies lists the open postings represented by the job addressed by :slug — the per-city
// openings folded under one canonical card by the content-dedup collapse. Public
// (unauthenticated) like the other job reads.
//
// Membership is the duplicate closure, the same one the search document's geography union is
// built from, so a city a candidate can filter to is a city they can reach. The addressed job
// may itself be a suppressed posting (those stay readable by slug); the query resolves it to
// its owner and lists that owner's whole group rather than a fragment. Response:
// {"data": [copy...]}.
func (h *jobsHandlers) JobCopies(ctx context.Context, params apigen.JobCopiesParams) (*apigen.JobCopiesPage, error) {
	// Before the slug lookup: a refused page must cost no database work at all. See
	// GetCompany, which orders its own two reads the same way.
	limit, offset, err := contractPageWindowed(ctx, defaultCopiesLimit, maxCopiesLimit)
	if err != nil {
		return nil, err
	}

	job, err := h.queries.GetJobBySlug(ctx, params.Slug)
	if err != nil {
		return nil, err
	}
	if err := jobVisibleToCaller(job, 0, false); err != nil {
		return nil, err
	}

	rows, err := h.queries.ListJobCopies(ctx, db.ListJobCopiesParams{
		JobID:     job.ID,
		RowLimit:  int32(limit),
		RowOffset: int32(offset),
	})
	if err != nil {
		return nil, err
	}

	copies := make([]apigen.JobCopy, len(rows))
	for i, r := range rows {
		cp := apigen.JobCopy{PublicSlug: r.PublicSlug, Location: r.Location, ApplyURL: outboundurl.Tag(r.URL), PostedAt: apigen.NilDateTime{Null: true}}
		if r.PostedAt.Valid {
			cp.PostedAt = apigen.NewNilDateTime(r.PostedAt.Time)
		}
		copies[i] = cp
	}

	// total is the whole closure's open size (COUNT(*) OVER, pre-LIMIT), so the client's
	// "N openings" header stays accurate even when the list is a capped page.
	var total int64
	if len(rows) > 0 {
		total = rows[0].Total
	}
	return &apigen.JobCopiesPage{Data: copies, Meta: apigen.JobCopiesMeta{Total: total}}, nil
}
