// Package credits implements the unified per-user AI-points balance that the
// metered AI features (résumé match, CV tailoring) draw from. Points are granted
// once per calendar month (use-it-or-lose-it, reset lazily on first access) and
// debited per action; the credit_ledger table is the append-only source of truth
// and credit_balances a materialized cache read on the hot debit path. See the
// add-ai-credits change.
package credits

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/platform/db"
)

// ErrInsufficient is returned by Debit when the caller's remaining points are below
// the action cost. Handlers map it to HTTP 402.
var ErrInsufficient = errors.New("credits: insufficient points")

// Feature identifies a metered AI action. The value is persisted in the ledger and
// matched by the debit-idempotency index, so it must stay stable.
type Feature string

const (
	FeatureMatch  Feature = "match"
	FeatureTailor Feature = "tailor"
)

// Config carries the points economics: the monthly grant, per-action costs, and the
// reward granted for an accepted board contribution.
type Config struct {
	MonthlyGrant       int
	CostMatch          int
	CostTailor         int
	ContributionReward int
}

// cost returns the points a given feature debits.
func (c Config) cost(f Feature) int {
	if f == FeatureTailor {
		return c.CostTailor
	}
	return c.CostMatch
}

// Balance is a point-in-time view of a user's credits: the points left in the
// current period and when that period resets.
type Balance struct {
	Remaining int       `json:"remaining"`
	ResetsAt  time.Time `json:"resets_at"`
}

// Store reads and mutates user points balances. It holds a *db.Queries for the
// read path and a *pgxpool.Pool for the atomic debit transaction.
type Store struct {
	q    *db.Queries
	pool *pgxpool.Pool
	cfg  Config
}

// NewStore constructs a Store.
func NewStore(q *db.Queries, pool *pgxpool.Pool, cfg Config) *Store {
	return &Store{q: q, pool: pool, cfg: cfg}
}

// Cost returns the points a feature debits, so callers can pre-check a balance.
func (s *Store) Cost(f Feature) int { return s.cfg.cost(f) }

// applicableRemaining resolves the balance to work from given the stored row's period.
// Same period: the stored remaining. A rolled-over period: floor at the monthly grant but
// preserve any banked surplus above it — the monthly grant never lifts a balance above
// MonthlyGrant, so anything higher is earned (reward) or bought and must not expire.
func (s *Store) applicableRemaining(period, cur string, remaining int32) int32 {
	if period == cur {
		return remaining
	}
	if grant := int32(s.cfg.MonthlyGrant); remaining < grant {
		return grant
	}
	return remaining
}

// Balance returns the caller's current points without consuming any. A user with no
// stored row, or whose stored balance is from an earlier period, is reported at the
// full monthly grant (the lazy reset applied in-memory for display); the persisted
// reset happens on the next Debit.
func (s *Store) Balance(ctx context.Context, userID int64) (Balance, error) {
	now := time.Now().UTC()
	remaining := s.cfg.MonthlyGrant
	row, err := s.q.GetBalance(ctx, userID)
	switch {
	case err == nil:
		remaining = int(s.applicableRemaining(row.Period, periodKey(now), row.Remaining))
	case errors.Is(err, pgx.ErrNoRows):
		// fresh user — full grant remaining
	default:
		return Balance{}, err
	}
	return Balance{Remaining: remaining, ResetsAt: resetsAt(now)}, nil
}

// Debit charges the feature's cost against the caller's balance, atomically and
// idempotently by (feature, ref). It applies any pending monthly reset, records the
// grant for the period, and either debits (returning the new balance), no-ops when
// the ref was already charged, or returns ErrInsufficient with the unchanged balance.
func (s *Store) Debit(ctx context.Context, userID int64, feature Feature, ref string) (Balance, error) {
	now := time.Now().UTC()
	cur := periodKey(now)
	grant := int32(s.cfg.MonthlyGrant)
	cost := int32(s.cfg.cost(feature))

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Balance{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)

	// Seed a row for a brand-new user so the row lock below has something to hold —
	// this is what serializes concurrent first-ever debits. Existing rows are untouched.
	if err := q.EnsureBalance(ctx, db.EnsureBalanceParams{UserID: userID, Period: cur, Remaining: grant}); err != nil {
		return Balance{}, err
	}
	row, err := q.GetBalanceForUpdate(ctx, userID)
	if err != nil {
		return Balance{}, err
	}
	// Record the monthly grant for this period, idempotent via the grant-period index.
	if err := q.InsertGrant(ctx, db.InsertGrantParams{UserID: userID, Period: cur, Delta: grant}); err != nil {
		return Balance{}, err
	}

	// Period rolled over: floor at the grant but keep any banked surplus (rewards/purchases).
	remaining := s.applicableRemaining(row.Period, cur, row.Remaining)

	already, err := q.DebitExists(ctx, db.DebitExistsParams{UserID: userID, Feature: string(feature), Ref: ref})
	if err != nil {
		return Balance{}, err
	}

	var debitErr error
	switch {
	case already:
		// recompute/resume of an already-charged ref — no debit
	case remaining < cost:
		debitErr = ErrInsufficient
	default:
		remaining -= cost
		if err := q.InsertDebit(ctx, db.InsertDebitParams{
			UserID: userID, Period: cur, Feature: string(feature), Delta: -cost, Ref: ref,
		}); err != nil {
			return Balance{}, err
		}
	}

	// Persist the current period + remaining (writes back any lazy reset and/or debit).
	if err := q.UpdateBalance(ctx, db.UpdateBalanceParams{UserID: userID, Period: cur, Remaining: remaining}); err != nil {
		return Balance{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Balance{}, err
	}
	return Balance{Remaining: int(remaining), ResetsAt: resetsAt(now)}, debitErr
}

// Release voids a debit taken as a reservation for work that produced nothing, atomically and
// idempotently by (feature, ref): the points go back and the ref becomes chargeable again, so
// a candidate whose analysis failed can retry and be charged once, not twice and not never.
//
// It takes the same row lock as Debit, so a release cannot interleave with a concurrent charge
// for the same user and leave the materialized balance disagreeing with the ledger.
//
// Releasing something that was never charged — a free recompute, a caller that did not reserve,
// a second release of the same reservation — removes nothing and reports the balance unchanged.
// That is what lets every failure path call it without first working out whether it owes one.
func (s *Store) Release(ctx context.Context, userID int64, feature Feature, ref string) (Balance, error) {
	now := time.Now().UTC()
	cur := periodKey(now)
	cost := int32(s.cfg.cost(feature))

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Balance{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)

	row, err := q.GetBalanceForUpdate(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		// No balance row means nothing was ever charged here, so there is nothing to give
		// back — and the caller is reported at the full monthly grant, the same reading
		// Balance gives a fresh user. Answering zero would say "out of credits" about
		// somebody who has spent nothing.
		return Balance{Remaining: s.cfg.MonthlyGrant, ResetsAt: resetsAt(now)}, nil
	}
	if err != nil {
		return Balance{}, err
	}

	removed, err := q.DeleteDebit(ctx, db.DeleteDebitParams{
		UserID: userID, Feature: string(feature), Ref: ref,
	})
	if err != nil {
		return Balance{}, err
	}

	// The cost goes back onto the STORED balance and the period reset is applied after, not
	// the other way round. Reversed, a release that crosses a month boundary would floor the
	// balance at the fresh grant and then add the cost on top — a grant of 20 with 19 left
	// would come back as 21, so a failed analysis would hand out a credit.
	restored := row.Remaining
	if removed > 0 {
		restored += cost
	}
	remaining := s.applicableRemaining(row.Period, cur, restored)
	if err := q.UpdateBalance(ctx, db.UpdateBalanceParams{UserID: userID, Period: cur, Remaining: remaining}); err != nil {
		return Balance{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Balance{}, err
	}
	return Balance{Remaining: int(remaining), ResetsAt: resetsAt(now)}, nil
}

// Reward credits the configured contribution reward to a user, atomically and idempotently
// by ref (e.g. the accepted contribution id). The reward banks above the monthly grant and
// survives the period reset (applicableRemaining preserves any surplus). A non-positive
// configured reward is a no-op that just reports the current balance.
func (s *Store) Reward(ctx context.Context, userID int64, ref string) (Balance, error) {
	if s.cfg.ContributionReward <= 0 {
		return s.Balance(ctx, userID)
	}
	now := time.Now().UTC()
	cur := periodKey(now)
	grant := int32(s.cfg.MonthlyGrant)
	amount := int32(s.cfg.ContributionReward)

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Balance{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)

	if err := q.EnsureBalance(ctx, db.EnsureBalanceParams{UserID: userID, Period: cur, Remaining: grant}); err != nil {
		return Balance{}, err
	}
	row, err := q.GetBalanceForUpdate(ctx, userID)
	if err != nil {
		return Balance{}, err
	}
	if err := q.InsertGrant(ctx, db.InsertGrantParams{UserID: userID, Period: cur, Delta: grant}); err != nil {
		return Balance{}, err
	}
	remaining := s.applicableRemaining(row.Period, cur, row.Remaining)

	already, err := q.RewardExists(ctx, db.RewardExistsParams{UserID: userID, Ref: ref})
	if err != nil {
		return Balance{}, err
	}
	if !already {
		remaining += amount
		if err := q.InsertReward(ctx, db.InsertRewardParams{UserID: userID, Period: cur, Delta: amount, Ref: ref}); err != nil {
			return Balance{}, err
		}
	}
	if err := q.UpdateBalance(ctx, db.UpdateBalanceParams{UserID: userID, Period: cur, Remaining: remaining}); err != nil {
		return Balance{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Balance{}, err
	}
	return Balance{Remaining: int(remaining), ResetsAt: resetsAt(now)}, nil
}

// PeriodKey is the calendar-month key (UTC) a grant and its debits share.
//
// Exported so anything else that reports "this period" reports the SAME period. Points
// and gateway usage are different instruments over one calendar, and two calendars would
// put a user's balance and their usage on different months without either being wrong.
func PeriodKey(t time.Time) string {
	return t.Format("2006-01")
}

// PeriodStart is midnight UTC on the first of the month t falls in.
func PeriodStart(t time.Time) time.Time {
	y, m, _ := t.Date()
	return time.Date(y, m, 1, 0, 0, 0, 0, time.UTC)
}

// ResetsAt is midnight UTC on the first of the month after t — when the current
// period's grant lapses and a fresh grant is issued.
func ResetsAt(t time.Time) time.Time {
	y, m, _ := t.Date()
	return time.Date(y, m+1, 1, 0, 0, 0, 0, time.UTC)
}

func periodKey(t time.Time) string   { return PeriodKey(t) }
func resetsAt(t time.Time) time.Time { return ResetsAt(t) }
