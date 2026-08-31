package plan

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/platform/db"
)

// ErrRefused is returned by Consume when the action may not proceed. The caller renders
// it as HTTP 402 with the Decision beside it, which carries whether the refusal was a
// plan ceiling or the fair-use guard.
var ErrRefused = errors.New("plan: allowance exhausted")

// Store reads a user's plan and meters their consumption of it. It holds a *db.Queries
// for the read paths and a *pgxpool.Pool for the transactions that must be atomic.
type Store struct {
	q    *db.Queries
	pool *pgxpool.Pool
	cfg  Config

	// now is injectable so a test can stand at a day boundary without waiting for one.
	now func() time.Time
}

// NewStore constructs a Store.
func NewStore(q *db.Queries, pool *pgxpool.Pool, cfg Config) *Store {
	return &Store{q: q, pool: pool, cfg: cfg, now: func() time.Time { return time.Now().UTC() }}
}

// Config exposes the configuration this store meters against, for callers that need to
// state a limit before attempting an action — the tailoring prompt says what a session
// will cost before the user commits to one.
func (s *Store) Config() Config { return s.cfg }

// Tier resolves the caller's plan. It reads one column and makes no network call, so a
// billing provider being slow or unreachable can never delay a metered action.
func (s *Store) Tier(ctx context.Context, userID int64) (Tier, error) {
	proUntil, err := s.q.GetProUntil(ctx, userID)
	if err != nil {
		return "", err
	}
	if !proUntil.Valid {
		return TierFree, nil
	}
	return TierOf(proUntil.Time, s.now()), nil
}

// Consume reserves one unit of a feature's daily allowance for this reference, atomically
// and idempotently by (user, feature, ref).
//
// It returns the Decision in every case, so a refusal can be rendered with the same
// numbers a success reports. A refusal also returns ErrRefused, so a caller that only
// wants to know whether to continue can check the error.
//
// The reservation is taken BEFORE the work it pays for. That is deliberate and it is what
// Release exists to undo: charging afterwards would let a user who disconnects mid-answer
// receive work nobody was charged for, and charging without a way back would bill them
// for an answer that never arrived.
func (s *Store) Consume(ctx context.Context, userID int64, feature Feature, ref string) (Decision, error) {
	now := s.now()
	day := Day(now)

	tier, err := s.Tier(ctx, userID)
	if err != nil {
		return Decision{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Decision{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)

	// Seed today's counter so the lock below has a row to hold. This is what serialises
	// two simultaneous first-ever consumptions, so an allowance cannot be oversold by a
	// race. An existing row is left alone.
	if err := q.EnsureUsageDay(ctx, db.EnsureUsageDayParams{
		UserID: userID, Feature: string(feature), Day: pgDay(day),
	}); err != nil {
		return Decision{}, err
	}
	used, err := q.GetUsageDayForUpdate(ctx, db.GetUsageDayForUpdateParams{
		UserID: userID, Feature: string(feature), Day: pgDay(day),
	})
	if err != nil {
		return Decision{}, err
	}
	already, err := q.ConsumptionExists(ctx, db.ConsumptionExistsParams{
		UserID: userID, Feature: string(feature), Ref: ref,
	})
	if err != nil {
		return Decision{}, err
	}

	d := s.cfg.decide(tier, feature, int(used), already, now)
	if d.Charge > 0 {
		if err := q.InsertConsumption(ctx, db.InsertConsumptionParams{
			UserID: userID, Feature: string(feature), Day: pgDay(day),
			Delta: int64(d.Charge), Ref: ref,
		}); err != nil {
			return Decision{}, err
		}
		if err := q.SetUsageDay(ctx, db.SetUsageDayParams{
			UserID: userID, Feature: string(feature), Day: pgDay(day), Used: int64(d.Used),
		}); err != nil {
			return Decision{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Decision{}, err
	}
	if !d.Allowed {
		if d.FairUse {
			// The guard sits far above human behaviour, so it firing means an automated
			// caller, a runaway retry loop, or a guard set too low — and all three are
			// operators' business. A guard nobody sees fire is a guard that gets blamed
			// for an outage it did not cause.
			log.Printf("plan: fair-use guard refused user %d on %s at %d/day", userID, feature, d.Used)
		}
		return d, ErrRefused
	}
	return d, nil
}

// Release gives back a reservation for work that produced nothing the user can use, so
// the reference becomes chargeable again and a retry is charged exactly once rather than
// twice or never.
//
// Releasing something that was never charged — a free recompute, a caller that did not
// reserve, a second release of the same reservation — gives back nothing and reports no
// error. That is what lets every failure path call it without first working out whether
// it owes one.
func (s *Store) Release(ctx context.Context, userID int64, feature Feature, ref string) error {
	// Which day the charge landed on, read before any lock. A reservation taken at 23:59
	// and released at 00:01 must decrement yesterday's counter: crediting today's would
	// hand back an allowance the user never spent today and leave the day they really
	// spent still spent.
	charged, err := s.q.GetConsumptionDay(ctx, db.GetConsumptionDayParams{
		UserID: userID, Feature: string(feature), Ref: ref,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := s.q.WithTx(tx)

	// usage_daily before usage_ledger, the same order Consume takes them in — a
	// consumption and a release for one user must not be able to deadlock each other.
	if err := q.EnsureUsageDay(ctx, db.EnsureUsageDayParams{
		UserID: userID, Feature: string(feature), Day: charged,
	}); err != nil {
		return err
	}
	used, err := q.GetUsageDayForUpdate(ctx, db.GetUsageDayForUpdateParams{
		UserID: userID, Feature: string(feature), Day: charged,
	})
	if err != nil {
		return err
	}
	removed, err := q.DeleteConsumption(ctx, db.DeleteConsumptionParams{
		UserID: userID, Feature: string(feature), Ref: ref,
	})
	if err != nil {
		return err
	}
	if removed > 0 {
		if used -= removed; used < 0 {
			used = 0
		}
		if err := q.SetUsageDay(ctx, db.SetUsageDayParams{
			UserID: userID, Feature: string(feature), Day: charged, Used: used,
		}); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// FeatureUsage is one feature's standing for the current day.
type FeatureUsage struct {
	Feature   Feature
	Used      int
	Limit     int
	Unlimited bool
}

// Standing is where one user stands on one feature today, with everything a surface needs
// to say so: their plan, what they have used, what they are allowed, and when it resets.
//
// It exists separately from Usage because the two are asked in different places. A page
// listing the plan wants every feature; a feature about to run wants only its own, and
// making it read all of them would be a query per surface for data nobody displays.
type Standing struct {
	Tier      Tier
	Feature   Feature
	Used      int
	Limit     int
	Unlimited bool
	ResetsAt  time.Time
}

// Exhausted reports whether this feature has nothing left today. An unlimited standing is
// never exhausted — the fair-use guard behind it refuses at the point of use rather than
// being reported as a ceiling somebody is approaching.
func (s Standing) Exhausted() bool { return !s.Unlimited && s.Used >= s.Limit }

// Standing reports where the caller stands on one feature today. It makes no model call
// and consumes nothing, so a surface may ask before offering an action — the tailoring
// prompt says what a session costs and how many remain before the candidate commits.
func (s *Store) Standing(ctx context.Context, userID int64, f Feature) (Standing, error) {
	now := s.now()

	tier, err := s.Tier(ctx, userID)
	if err != nil {
		return Standing{}, err
	}
	used, err := s.q.GetUsageDay(ctx, db.GetUsageDayParams{
		UserID: userID, Feature: string(f), Day: pgDay(Day(now)),
	})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Standing{}, err
	}
	allowance := s.cfg.Allowance(tier, f)
	return Standing{
		Tier:      tier,
		Feature:   f,
		Used:      int(used),
		Limit:     allowance.Limit,
		Unlimited: allowance.Unlimited,
		ResetsAt:  ResetsAt(now),
	}, nil
}

// Usage reports the caller's plan and where they stand on every metered feature today.
// A feature they have not touched reports as untouched rather than being absent, so the
// surface can list the whole plan without knowing which rows happen to exist.
func (s *Store) Usage(ctx context.Context, userID int64) (Tier, []FeatureUsage, time.Time, error) {
	now := s.now()

	tier, err := s.Tier(ctx, userID)
	if err != nil {
		return "", nil, time.Time{}, err
	}
	rows, err := s.q.ListUsageForDay(ctx, db.ListUsageForDayParams{
		UserID: userID, Day: pgDay(Day(now)),
	})
	if err != nil {
		return "", nil, time.Time{}, err
	}
	used := make(map[Feature]int, len(rows))
	for _, r := range rows {
		used[Feature(r.Feature)] = int(r.Used)
	}

	out := make([]FeatureUsage, 0, len(AllFeatures()))
	for _, f := range AllFeatures() {
		allowance := s.cfg.Allowance(tier, f)
		out = append(out, FeatureUsage{
			Feature:   f,
			Used:      used[f],
			Limit:     allowance.Limit,
			Unlimited: allowance.Unlimited,
		})
	}
	return tier, out, ResetsAt(now), nil
}

func pgDay(d time.Time) pgtype.Date { return pgtype.Date{Time: d, Valid: true} }
