package sponsored

import (
	"context"

	"github.com/strelov1/freehire/internal/platform/db"
)

// EndClosedJobs ends every live campaign whose job has closed or whose owner's account is gone
// and returns how many it ended. closed_at is set by many statements, so this periodic sweep
// (cmd/billing-sync) stands in for a hook. A closed job needs no reindex (search filters it out
// whatever its document says); an owner-less campaign's open job is requeued.
func (s *Service) EndClosedJobs(ctx context.Context) (int64, error) {
	q := db.New(s.pool)
	n, err := q.EndSponsoredCampaignsOfClosedJobs(ctx)
	if err != nil {
		return n, err
	}
	err = s.inTx(ctx, func(q *db.Queries) error {
		jobs, err := q.EndSponsoredCampaignsOfDeletedOwners(ctx)
		if err != nil {
			return err
		}
		n += int64(len(jobs))
		for _, id := range jobs {
			if err := q.RequeueSearchOutbox(ctx, id); err != nil {
				return err
			}
		}
		return nil
	})
	return n, err
}

// RequeueDailyCapJobs queues for re-indexing the open jobs of every active campaign that has
// a daily cap. Run just after UTC midnight it brings back the jobs whose document went
// sponsored=false when yesterday's cap was reached; nothing else writes them then.
func (s *Service) RequeueDailyCapJobs(ctx context.Context) (int64, error) {
	return db.New(s.pool).RequeueSponsoredDailyCapJobs(ctx)
}
