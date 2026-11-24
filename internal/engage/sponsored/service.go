package sponsored

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
)

// ErrBadTransition is returned when the lifecycle does not allow the requested move.
var ErrBadTransition = errors.New("sponsored: transition not allowed")

type Service struct {
	pool     *pgxpool.Pool
	refunder Refunder
}

func New(pool *pgxpool.Pool) *Service { return &Service{pool: pool} }

// inTx runs fn in one transaction with a Queries bound to it.
func (s *Service) inTx(ctx context.Context, fn func(q *db.Queries) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(db.New(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func day(t time.Time) pgtype.Date {
	u := t.UTC()
	return pgtype.Date{Time: time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC), Valid: true}
}

// Charge records one click on a campaign and charges it when valid, in one transaction
// that holds the campaign row lock. The click row is written whether or not it is valid.
func (s *Service) Charge(ctx context.Context, campaignID int64, click Click, now time.Time) (Decision, error) {
	var d Decision
	err := s.inTx(ctx, func(q *db.Queries) error {
		c, err := q.GetSponsoredCampaignForUpdate(ctx, campaignID)
		if err != nil {
			return err
		}
		dup, err := q.HasRecentValidSponsoredClick(ctx, db.HasRecentValidSponsoredClickParams{
			CampaignID: campaignID, VisitorHash: click.VisitorHash,
			Since: pgconv.Instant(now.Add(-DuplicateWindow)),
		})
		if err != nil {
			return err
		}
		today := day(now)
		dailySpent, err := q.GetSponsoredDailySpent(ctx, db.GetSponsoredDailySpentParams{CampaignID: campaignID, Day: today})
		if err != nil {
			return err
		}
		st := State{Status: Status(c.Status), CPCCents: c.CpcCents, BudgetCents: c.BudgetCents, SpentCents: c.SpentCents}
		if c.DailyCapCents.Valid {
			st.DailyCapCents = &c.DailyCapCents.Int64
		}
		d = Decide(st, click, dup, dailySpent)

		if d.Valid {
			if _, err := q.ChargeSponsoredCampaign(ctx, db.ChargeSponsoredCampaignParams{ID: campaignID, CostCents: d.CostCents}); err != nil {
				return fmt.Errorf("charge: %w", err)
			}
		}
		if err := q.InsertSponsoredClick(ctx, db.InsertSponsoredClickParams{
			CampaignID: campaignID, Ts: pgconv.Instant(now),
			Placement: click.Placement, VisitorHash: click.VisitorHash,
			Valid: d.Valid, Reason: string(d.Reason), CostCents: d.CostCents,
		}); err != nil {
			return err
		}
		daily := db.UpsertSponsoredDailyParams{CampaignID: campaignID, Day: today, SpentCents: d.CostCents}
		if d.Valid {
			daily.ValidClicks = 1
		} else {
			daily.InvalidClicks = 1
		}
		if err := q.UpsertSponsoredDaily(ctx, daily); err != nil {
			return err
		}
		if d.Reindex {
			return q.RequeueSearchOutbox(ctx, c.JobID)
		}
		return nil
	})
	return d, err
}

// Transition moves a campaign to a new status if the lifecycle allows it, and queues its
// job for re-indexing.
func (s *Service) Transition(ctx context.Context, campaignID int64, to Status) error {
	return s.inTx(ctx, func(q *db.Queries) error {
		c, err := q.GetSponsoredCampaignForUpdate(ctx, campaignID)
		if err != nil {
			return err
		}
		if !CanTransition(Status(c.Status), to) {
			return fmt.Errorf("%w: %s -> %s", ErrBadTransition, c.Status, to)
		}
		if err := q.SetSponsoredCampaignStatus(ctx, db.SetSponsoredCampaignStatusParams{ID: campaignID, Status: string(to)}); err != nil {
			return err
		}
		return q.RequeueSearchOutbox(ctx, c.JobID)
	})
}

// EndForJob ends every live campaign of a job (call when the job closes) and returns how
// many it ended. Unused budget stays recorded.
func (s *Service) EndForJob(ctx context.Context, jobID int64) (int, error) {
	var n int
	err := s.inTx(ctx, func(q *db.Queries) error {
		ids, err := q.EndSponsoredCampaignsForJob(ctx, jobID)
		if err != nil {
			return err
		}
		n = len(ids)
		if n == 0 {
			return nil
		}
		return q.RequeueSearchOutbox(ctx, jobID)
	})
	return n, err
}
