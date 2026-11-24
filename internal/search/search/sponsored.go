package search

import (
	"context"
	"time"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
)

// ServingReader reads which jobs a paid campaign serves today. *db.Queries satisfies it.
type ServingReader interface {
	ListServingSponsoredCampaigns(ctx context.Context, arg db.ListServingSponsoredCampaignsParams) ([]db.ListServingSponsoredCampaignsRow, error)
}

// UTCDay is the day a daily cap is counted in: caps reset at UTC midnight.
func UTCDay(now time.Time) time.Time {
	u := now.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}

// ServingCampaigns returns, per job id, the campaign serving it today (the highest cpc one).
// A nil reader reads nothing, so a writer without a database leaves every document unmarked.
func ServingCampaigns(ctx context.Context, r ServingReader, jobIDs []int64, now time.Time) (map[int64]db.ListServingSponsoredCampaignsRow, error) {
	if r == nil || len(jobIDs) == 0 {
		return nil, nil
	}
	today := UTCDay(now)
	rows, err := r.ListServingSponsoredCampaigns(ctx, db.ListServingSponsoredCampaignsParams{
		JobIds: jobIDs, Today: pgconv.Date(&today),
	})
	if err != nil {
		return nil, err
	}
	out := make(map[int64]db.ListServingSponsoredCampaignsRow, len(rows))
	for _, row := range rows {
		out[row.JobID] = row
	}
	return out, nil
}

// AttachSponsored marks the document sponsored when its job is in serving (the result of
// ServingCampaigns for the batch); any other job is left unmarked.
func (d *JobDocument) AttachSponsored(serving map[int64]db.ListServingSponsoredCampaignsRow) {
	if row, ok := serving[d.ID]; ok && row.CpcCents > 0 {
		d.Sponsored, d.SponsoredCPC = true, row.CpcCents
	}
}
