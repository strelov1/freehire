package plan

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/platform/db"
)

// FeatureSupport is one message to the support agent. It is NOT in AllFeatures: every account
// may use it, paid or not, so it has no tier allowances and the "free gets no AI" rule does not
// apply. It shares the ledger, so a turn is reserved before the stream opens and given back with
// Release when it produced nothing.
const FeatureSupport Feature = "support"

// decideSupport is decide without the tier: one flat daily figure for everybody, and an
// already-charged reference is always allowed.
func decideSupport(limit int) func(Tier, Feature, int, bool, time.Time) Decision {
	return func(tier Tier, f Feature, used int, alreadyCharged bool, now time.Time) Decision {
		d := Decision{
			Tier: tier, Feature: f, Used: used, Limit: limit,
			ResetsAt: ResetsAt(now), Enforced: true,
		}
		if alreadyCharged {
			d.Allowed = true
			return d
		}
		if used < limit {
			d.Allowed, d.Charge, d.Used = true, 1, used+1
		}
		return d
	}
}

// ConsumeSupport reserves one support message for this reference. It refuses with ErrRefused
// once the daily number is sent, whatever the plan, and never records a plan-limit hit: that
// signal drives the "Pro removes this" mail.
func (s *Store) ConsumeSupport(ctx context.Context, userID int64, ref string) (Decision, error) {
	return s.consume(ctx, userID, FeatureSupport, ref, decideSupport(s.cfg.SupportMessagesPerDay))
}

// SupportStanding is where one account stands on today's support messages. The limit is flat
// for every tier.
type SupportStanding struct {
	Used      int
	Limit     int
	Remaining int
	ResetsAt  time.Time
}

// SupportStanding reads the caller's support usage today without charging anything.
func (s *Store) SupportStanding(ctx context.Context, userID int64) (SupportStanding, error) {
	now := s.now()
	used, err := s.q.GetUsageDay(ctx, db.GetUsageDayParams{
		UserID: userID, Feature: string(FeatureSupport), Day: pgDay(Day(now)),
	})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return SupportStanding{}, err
	}
	limit := s.cfg.SupportMessagesPerDay
	return SupportStanding{
		Used:      int(used),
		Limit:     limit,
		Remaining: max(limit-int(used), 0),
		ResetsAt:  ResetsAt(now),
	}, nil
}

// SupportTurnsPerSession is how many messages one support conversation may hold.
func (s *Store) SupportTurnsPerSession() int { return s.cfg.SupportTurnsPerSession }

// SupportGloballyExhausted reports whether all accounts together have used the day's support
// budget. It reads without a lock, so a burst can overshoot by the requests in flight.
func (s *Store) SupportGloballyExhausted(ctx context.Context) (bool, error) {
	used, err := s.q.SumUsageDayForFeature(ctx, db.SumUsageDayForFeatureParams{
		Feature: string(FeatureSupport), Day: pgDay(Day(s.now())),
	})
	if err != nil {
		return false, err
	}
	return int(used) >= s.cfg.SupportMessagesGlobalPerDay, nil
}
