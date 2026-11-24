package sponsored

import (
	"context"

	"github.com/strelov1/freehire/internal/platform/db"
)

// EndClosedJobs ends every live campaign whose job has closed and returns how many it ended.
// closed_at is set by many statements, so this periodic sweep (cmd/billing-sync) stands in for
// a hook. No reindex: a closed job is filtered out of search whatever its document says.
func (s *Service) EndClosedJobs(ctx context.Context) (int64, error) {
	return db.New(s.pool).EndSponsoredCampaignsOfClosedJobs(ctx)
}

// RequeueDailyCapJobs queues for re-indexing the open jobs of every active campaign that has
// a daily cap. Run just after UTC midnight it brings back the jobs whose document went
// sponsored=false when yesterday's cap was reached; nothing else writes them then.
func (s *Service) RequeueDailyCapJobs(ctx context.Context) (int64, error) {
	return db.New(s.pool).RequeueSponsoredDailyCapJobs(ctx)
}
