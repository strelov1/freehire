package plan

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/platform/db"
)

// FeatureSupport is one message to the support agent. It is deliberately NOT in AllFeatures:
// it is not part of a plan. Every account may use it, paid or not — a person who cannot reach
// support about a payment is the one who most needs it — so it has no tier allowances, no
// place on the plans page and no usage row to sell against, and the "free gets no AI" rule
// that every feature in AllFeatures obeys does not apply to it.
//
// The ledger is the same one, keyed by (user, feature, ref), so a turn is reserved before the
// stream opens and given back with Release when it produced nothing.
const FeatureSupport Feature = "support"

// decideSupport is the rule for one support message: the same shape as decide, minus the
// tier. One flat daily figure for everybody, and a reference already paid for is always
// allowed so a retry costs nothing.
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
// once the account has sent its daily number, whatever its plan, and never records the refusal
// as a plan-limit hit: that signal drives the "Pro removes this" mail, and support has nothing
// for Pro to remove.
func (s *Store) ConsumeSupport(ctx context.Context, userID int64, ref string) (Decision, error) {
	return s.consume(ctx, userID, FeatureSupport, ref, decideSupport(s.cfg.SupportMessagesPerDay))
}

// SupportStanding is where one account stands on today's support messages, read from the
// ledger ConsumeSupport charges. The limit is flat for every tier, so it never asks for one.
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

// SupportGloballyExhausted reports whether every account together has used the day's support
// budget. It is a pre-check read without a lock, so it is a soft ceiling that a burst can
// overshoot by the requests in flight; what it bounds is the day's model spend on a surface
// that is free to every account, not a price.
func (s *Store) SupportGloballyExhausted(ctx context.Context) (bool, error) {
	used, err := s.q.SumUsageDayForFeature(ctx, db.SumUsageDayForFeatureParams{
		Feature: string(FeatureSupport), Day: pgDay(Day(s.now())),
	})
	if err != nil {
		return false, err
	}
	return int(used) >= s.cfg.SupportMessagesGlobalPerDay, nil
}
