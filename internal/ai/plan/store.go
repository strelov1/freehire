package plan

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/google/uuid"
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

// Tier resolves the caller's plan from one row with no network call, so a slow billing
// provider can never delay a metered action. An invalid column is a zero time, which TierOf
// reads as reaching nothing.
func (s *Store) Tier(ctx context.Context, userID int64) (Tier, error) {
	untils, err := s.q.GetPlanUntils(ctx, userID)
	if err != nil {
		return "", err
	}
	return TierOf(untils.ProUntil.Time, untils.UltraUntil.Time, s.now()), nil
}

// Consume reserves one unit of a feature's daily allowance for this reference, atomically
// and idempotently by (user, feature, ref).
//
// It returns the Decision even on refusal, which also returns ErrRefused. The reservation is
// taken BEFORE the work, so a disconnect mid-answer cannot yield unbilled work; Release undoes
// it when nothing was delivered.
func (s *Store) Consume(ctx context.Context, userID int64, feature Feature, ref string) (Decision, error) {
	return s.consume(ctx, userID, feature, ref, s.cfg.decide)
}

// consume is the transaction around one decision. The rule is a parameter because the paid
// features and the free support allowance answer different questions over the same ledger.
func (s *Store) consume(
	ctx context.Context, userID int64, feature Feature, ref string,
	decide func(tier Tier, f Feature, used int, alreadyCharged bool, now time.Time) Decision,
) (Decision, error) {
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

	// Seed today's counter so the lock below has a row to hold; this serialises two simultaneous
	// first-ever consumptions so an allowance cannot be oversold.
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

	d := decide(tier, feature, int(used), already, now)
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
			// Firing means an automated caller, a runaway retry loop or a guard set too low; operators
			// need to see it.
			log.Printf("plan: fair-use guard refused user %d on %s at %d/day", userID, feature, d.Used)
		} else if feature != FeatureSupport {
			// A real plan-ceiling refusal: the signal cmd/limit-nudge-mail reads. Best-effort and outside
			// the committed transaction, so the refusal reaches the caller whether or not the write lands.
			s.recordLimitHit(ctx, userID, feature, day)
		}
		return d, ErrRefused
	}
	return d, nil
}

// recordLimitHit best-effort records that a plan ceiling (not the fair-use guard) just refused
// this user today. A failure is logged and dropped, never changing the refusal.
func (s *Store) recordLimitHit(ctx context.Context, userID int64, feature Feature, day time.Time) {
	if err := s.q.RecordPlanLimitHit(ctx, db.RecordPlanLimitHitParams{
		UserID: userID, Feature: string(feature), Day: pgDay(day),
	}); err != nil {
		log.Printf("plan: record limit hit for user %d on %s: %v", userID, feature, err)
	}
}

// PaywallJob names the vacancy a refused action concerned, by whichever identifier the route
// carries; the zero value means none. At most one field is set. An unknown or foreign
// identifier records no job rather than failing.
type PaywallJob struct {
	// Slug is a job's public slug.
	Slug string
	// CVID is a CV of the caller, resolved through the vacancy it was tailored for.
	CVID uuid.UUID
	// SessionID is an assistant session of the caller, resolved through its vacancy.
	SessionID uuid.UUID
}

// RecordPaywallHit records that the paid-AI gate refused this user on this feature today, once
// per (user, feature, job, day). feature is a string because routes without an allowance use a
// fixed name. The gate drops a failure, since the 402 must go out regardless.
func (s *Store) RecordPaywallHit(ctx context.Context, userID int64, feature string, job PaywallJob) error {
	return s.q.RecordPaywallHit(ctx, db.RecordPaywallHitParams{
		UserID:    userID,
		Feature:   feature,
		JobSlug:   pgtype.Text{String: job.Slug, Valid: job.Slug != ""},
		CvID:      pgUUID(job.CVID),
		SessionID: pgUUID(job.SessionID),
		Day:       pgDay(s.now()),
	})
}

func pgUUID(id uuid.UUID) pgtype.UUID { return pgtype.UUID{Bytes: id, Valid: id != uuid.Nil} }

// Release gives back a reservation for work that produced nothing, so the reference is
// chargeable again and a retry is charged exactly once. Releasing something never charged is a
// no-op without error, so every failure path can call it blindly.
func (s *Store) Release(ctx context.Context, userID int64, feature Feature, ref string) error {
	// Read the charge's day before any lock: a reservation taken at 23:59 and released at 00:01
	// must decrement yesterday's counter.
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

	// usage_daily before usage_ledger, the same order as Consume, so the two cannot deadlock.
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
	released, err := q.ReleaseConsumption(ctx, db.ReleaseConsumptionParams{
		UserID: userID, Feature: string(feature), Ref: ref,
	})
	if err != nil {
		return err
	}
	if released > 0 {
		if used -= released; used < 0 {
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
	Enforced  bool
}

// Standing is where one user stands on one feature today: plan, usage, allowance and reset
// time. Unlike Usage it reads one feature, so a feature about to run does not query them all.
type Standing struct {
	Tier      Tier
	Feature   Feature
	Used      int
	Limit     int
	Unlimited bool
	ResetsAt  time.Time

	// Enforced says whether this feature's ceiling turns anybody away yet. It travels with the
	// standing so a surface cannot refuse on Exhausted alone.
	Enforced bool
}

// Exhausted reports whether this feature has nothing left today. An unlimited standing is
// never exhausted; the fair-use guard refuses at the point of use. It ignores enforcement, so
// it is for deciding what to say; use Refuses to decide whether to refuse.
func (s Standing) Exhausted() bool { return !s.Unlimited && s.Used >= s.Limit }

// Refuses reports whether an action would actually be turned away now: the allowance is spent
// AND enforcement is on. Refusing on Exhausted alone would hard-refuse features still in shadow
// mode. It is a method on Standing so every holder of one, including the SPA, asks the same
// expression.
func (s Standing) Refuses() bool { return s.Exhausted() && s.Enforced }

// Standing reports where the caller stands on one feature today. It consumes nothing and makes
// no model call, so a surface may ask before offering an action.
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
		Enforced:  s.cfg.Enforced(f),
	}, nil
}

// Usage reports the caller's plan and standing on every metered feature today. Untouched
// features report as unused rather than being absent.
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
			Enforced:  s.cfg.Enforced(f),
		})
	}
	return tier, out, ResetsAt(now), nil
}

func pgDay(d time.Time) pgtype.Date { return pgtype.Date{Time: d, Valid: true} }
