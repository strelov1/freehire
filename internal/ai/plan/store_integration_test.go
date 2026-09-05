//go:build integration

// Integration tests for the plan Store against a real Postgres: the consumption
// transaction is atomic and idempotent by reference, a day rollover resets nothing and
// needs nothing reset, a release gives back the day it took from, and a concurrent race
// never oversells an allowance. Run with: go test -tags=integration ./internal/ai/plan/
// Requires Docker (testcontainers spins up a throwaway Postgres with the migrations).
package plan

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

func newStore(t *testing.T, cfg Config, now time.Time) (*Store, *pgxpool.Pool) {
	t.Helper()
	pool := testdb.Pool(t)
	s := NewStore(db.New(pool), pool, cfg)
	s.now = func() time.Time { return now }
	return s, pool
}

func insertUser(t *testing.T, pool *pgxpool.Pool, email string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO users (email) VALUES ($1) RETURNING id`, email).Scan(&id); err != nil {
		t.Fatalf("insert user %s: %v", email, err)
	}
	return id
}

// makePro puts an account on the pro plan. It writes pro_until_granted, not pro_until: since
// migration 0135 the plan column is derived from three sources and assigning it fails.
//
// The granted source is the right one here rather than a convenient one. These tests are
// about what a plan ALLOWS, and no payment provider is involved in any of them — granted is
// exactly "pro without a provider". Seeding a provider's column would tie a test about
// allowances to a subscription that does not exist.
func makePro(t *testing.T, pool *pgxpool.Pool, userID int64, until time.Time) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`UPDATE users SET pro_until_granted = $2 WHERE id = $1`, userID, until); err != nil {
		t.Fatalf("make pro: %v", err)
	}
}

// countLedger returns how many consumption rows exist for a (user, feature).
func countLedger(t *testing.T, pool *pgxpool.Pool, userID int64, feature Feature) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM usage_ledger WHERE user_id=$1 AND kind='consume' AND feature=$2`,
		userID, string(feature)).Scan(&n); err != nil {
		t.Fatalf("count ledger: %v", err)
	}
	return n
}

func usedOn(t *testing.T, pool *pgxpool.Pool, userID int64, feature Feature, day time.Time) int {
	t.Helper()
	var n int
	err := pool.QueryRow(context.Background(),
		`SELECT used FROM usage_daily WHERE user_id=$1 AND feature=$2 AND day=$3`,
		userID, string(feature), pgtype.Date{Time: day, Valid: true}).Scan(&n)
	if err != nil {
		t.Fatalf("read counter for %v: %v", day, err)
	}
	return n
}

func TestConsumeSpendsTheDailyAllowanceThenRefuses(t *testing.T) {
	now := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	s, pool := newStore(t, enforcing(), now)
	ctx := context.Background()
	user := insertUser(t, pool, "consume-allowance@example.test")

	limit := s.cfg.FreeDaily(FeatureFit)
	for i := range limit {
		d, err := s.Consume(ctx, user, FeatureFit, jobRef(i))
		if err != nil {
			t.Fatalf("consumption %d of %d was refused: %v", i+1, limit, err)
		}
		if d.Used != i+1 {
			t.Fatalf("after consumption %d, Used = %d", i+1, d.Used)
		}
	}

	d, err := s.Consume(ctx, user, FeatureFit, jobRef(limit))
	if !errors.Is(err, ErrRefused) {
		t.Fatalf("the consumption past the allowance returned %v, want ErrRefused", err)
	}
	if d.Allowed || d.Charge != 0 {
		t.Fatalf("a refusal reported allowed=%v charge=%d", d.Allowed, d.Charge)
	}
	if got := countLedger(t, pool, user, FeatureFit); got != limit {
		t.Fatalf("%d ledger rows after a refusal, want %d — a refusal must write nothing", got, limit)
	}
	if !d.ResetsAt.Equal(time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("the refusal reports ResetsAt %v, want the next UTC midnight", d.ResetsAt)
	}
}

func TestConsumeIsIdempotentByRef(t *testing.T) {
	now := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	s, pool := newStore(t, enforcing(), now)
	ctx := context.Background()
	user := insertUser(t, pool, "consume-idempotent@example.test")

	const first = "the-job-under-test"
	if _, err := s.Consume(ctx, user, FeatureFit, first); err != nil {
		t.Fatalf("first consumption: %v", err)
	}
	// Spend the rest of the allowance on other references, then re-submit the first.
	for i := 1; i < s.cfg.FreeDaily(FeatureFit); i++ {
		if _, err := s.Consume(ctx, user, FeatureFit, jobRef(i)); err != nil {
			t.Fatalf("consumption %d: %v", i, err)
		}
	}

	d, err := s.Consume(ctx, user, FeatureFit, first)
	if err != nil {
		t.Fatalf("re-submitting an already-charged reference with no allowance left was refused: %v", err)
	}
	if d.Charge != 0 {
		t.Errorf("the repeat charged %d, want 0", d.Charge)
	}
	if got, want := countLedger(t, pool, user, FeatureFit), s.cfg.FreeDaily(FeatureFit); got != want {
		t.Errorf("%d ledger rows, want %d — the repeat wrote a second row", got, want)
	}
}

func TestANewDayNeedsNothingReset(t *testing.T) {
	yesterday := time.Date(2026, 9, 15, 23, 30, 0, 0, time.UTC)
	s, pool := newStore(t, enforcing(), yesterday)
	ctx := context.Background()
	user := insertUser(t, pool, "consume-rollover@example.test")

	for i := range s.cfg.FreeDaily(FeatureFit) {
		if _, err := s.Consume(ctx, user, FeatureFit, jobRef(i)); err != nil {
			t.Fatalf("consumption %d: %v", i, err)
		}
	}
	if _, err := s.Consume(ctx, user, FeatureFit, "job-over"); !errors.Is(err, ErrRefused) {
		t.Fatalf("expected the allowance to be spent, got %v", err)
	}

	// Midnight passes. Nothing runs; the store simply asks a different day.
	s.now = func() time.Time { return yesterday.Add(time.Hour) }

	d, err := s.Consume(ctx, user, FeatureFit, "job-next-day")
	if err != nil {
		t.Fatalf("the first consumption of a new day was refused: %v", err)
	}
	if d.Used != 1 {
		t.Errorf("the new day starts at Used = %d, want 1 — yesterday's consumption carried over", d.Used)
	}
	if got := usedOn(t, pool, user, FeatureFit, Day(yesterday)); got != s.cfg.FreeDaily(FeatureFit) {
		t.Errorf("yesterday's counter now reads %d; a rollover must leave history alone, not rewrite it", got)
	}
}

func TestReleaseGivesBackTheDayItTookFrom(t *testing.T) {
	lateYesterday := time.Date(2026, 9, 15, 23, 59, 0, 0, time.UTC)
	s, pool := newStore(t, enforcing(), lateYesterday)
	ctx := context.Background()
	user := insertUser(t, pool, "release-boundary@example.test")

	if _, err := s.Consume(ctx, user, FeatureFit, "job-boundary"); err != nil {
		t.Fatalf("consumption: %v", err)
	}

	// The work fails two minutes later, on the other side of midnight.
	s.now = func() time.Time { return lateYesterday.Add(2 * time.Minute) }
	if err := s.Release(ctx, user, FeatureFit, "job-boundary"); err != nil {
		t.Fatalf("Release: %v", err)
	}

	if got := usedOn(t, pool, user, FeatureFit, Day(lateYesterday)); got != 0 {
		t.Errorf("yesterday's counter reads %d after the release, want 0 — the release credited the wrong day", got)
	}
	var todayRows int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FROM usage_daily WHERE user_id=$1 AND feature=$2 AND day=$3`,
		user, string(FeatureFit), pgtype.Date{Time: Day(lateYesterday.Add(2 * time.Minute)), Valid: true},
	).Scan(&todayRows); err != nil {
		t.Fatalf("count today's counters: %v", err)
	}
	if todayRows != 0 {
		t.Error("the release created a counter for today; it would have handed back an allowance the user never spent today")
	}
}

func TestReleaseIsSafeToCallBlind(t *testing.T) {
	now := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	s, pool := newStore(t, enforcing(), now)
	ctx := context.Background()
	user := insertUser(t, pool, "release-blind@example.test")

	// Nothing was ever charged under this reference.
	if err := s.Release(ctx, user, FeatureFit, "never-charged"); err != nil {
		t.Fatalf("releasing an uncharged reference errored: %v — every failure path must be able to call this blind", err)
	}

	if _, err := s.Consume(ctx, user, FeatureFit, "job-1"); err != nil {
		t.Fatalf("consumption: %v", err)
	}
	for range 2 {
		if err := s.Release(ctx, user, FeatureFit, "job-1"); err != nil {
			t.Fatalf("Release: %v", err)
		}
	}
	if got := usedOn(t, pool, user, FeatureFit, Day(now)); got != 0 {
		t.Errorf("counter reads %d after a double release, want 0 — the second release gave back an allowance nobody took", got)
	}

	// The reference is chargeable again, so the retry is charged exactly once.
	if _, err := s.Consume(ctx, user, FeatureFit, "job-1"); err != nil {
		t.Fatalf("the released reference could not be charged again: %v", err)
	}
	if got := usedOn(t, pool, user, FeatureFit, Day(now)); got != 1 {
		t.Errorf("counter reads %d after the retry, want 1", got)
	}
}

// A release must leave a trace. Deleting the row would free the reference and erase the
// fact that a reservation was ever taken — exactly the hole an append-only ledger exists
// to prevent, and the one thing nobody could reconstruct afterwards.
func TestAReleaseIsRecordedNotErased(t *testing.T) {
	now := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	s, pool := newStore(t, enforcing(), now)
	ctx := context.Background()
	user := insertUser(t, pool, "release-recorded@example.test")

	if _, err := s.Consume(ctx, user, FeatureFit, "job-traced"); err != nil {
		t.Fatalf("consumption: %v", err)
	}
	if err := s.Release(ctx, user, FeatureFit, "job-traced"); err != nil {
		t.Fatalf("Release: %v", err)
	}

	var kind string
	if err := pool.QueryRow(ctx,
		`SELECT kind FROM usage_ledger WHERE user_id=$1 AND feature=$2 AND ref=$3`,
		user, string(FeatureFit), "job-traced").Scan(&kind); err != nil {
		t.Fatalf("the released entry vanished from the ledger: %v", err)
	}
	if kind != "release" {
		t.Errorf("the entry reads %q, want \"release\"", kind)
	}
	// And it no longer counts as a consumption, so the day's counter stays derivable from
	// the ledger by summing only what was actually spent.
	if got := countLedger(t, pool, user, FeatureFit); got != 0 {
		t.Errorf("%d consumptions remain, want 0", got)
	}
}

func TestConcurrentConsumptionsNeverOversell(t *testing.T) {
	now := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	s, pool := newStore(t, enforcing(), now)
	ctx := context.Background()
	user := insertUser(t, pool, "consume-race@example.test")

	limit := s.cfg.FreeDaily(FeatureFit)
	const attempts = 12

	var wg sync.WaitGroup
	allowed := make([]bool, attempts)
	for i := range attempts {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d, err := s.Consume(ctx, user, FeatureFit, jobRef(i))
			allowed[i] = err == nil && d.Allowed
		}()
	}
	wg.Wait()

	granted := 0
	for _, ok := range allowed {
		if ok {
			granted++
		}
	}
	if granted != limit {
		t.Fatalf("%d of %d simultaneous requests were allowed, want exactly %d — the row lock did not serialise them", granted, attempts, limit)
	}
	if got := countLedger(t, pool, user, FeatureFit); got != limit {
		t.Fatalf("%d ledger rows, want %d", got, limit)
	}
	if got := usedOn(t, pool, user, FeatureFit, Day(now)); got != limit {
		t.Fatalf("counter reads %d, want %d — it disagrees with the ledger it is derived from", got, limit)
	}
}

func TestProIsUnlimitedUntilTheFairUseGuard(t *testing.T) {
	now := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	s, pool := newStore(t, enforcing(), now)
	ctx := context.Background()
	user := insertUser(t, pool, "pro-guard@example.test")
	makePro(t, pool, user, now.Add(30*24*time.Hour))

	guard := s.cfg.ProFairUse(FeatureFit)
	for i := range guard {
		d, err := s.Consume(ctx, user, FeatureFit, jobRef(i))
		if err != nil {
			t.Fatalf("pro consumption %d of %d was refused: %v", i+1, guard, err)
		}
		if !d.Unlimited {
			t.Fatalf("a pro decision reported a countable limit of %d", d.Limit)
		}
	}

	d, err := s.Consume(ctx, user, FeatureFit, jobRef(guard))
	if !errors.Is(err, ErrRefused) {
		t.Fatalf("a pro account past its fair-use guard was allowed (err %v)", err)
	}
	if !d.FairUse {
		t.Error("the refusal is not marked as a fair-use one, so the surface would show it as a plan ceiling")
	}
}

func TestALapsedSubscriptionFallsBackToFree(t *testing.T) {
	now := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	s, pool := newStore(t, enforcing(), now)
	ctx := context.Background()
	user := insertUser(t, pool, "pro-lapsed@example.test")
	makePro(t, pool, user, now.Add(-time.Hour))

	tier, err := s.Tier(ctx, user)
	if err != nil {
		t.Fatalf("Tier: %v", err)
	}
	if tier != TierFree {
		t.Fatalf("a lapsed subscription reads as %q; it expires by itself, with nothing to sweep", tier)
	}
}

func TestUsageReportsTheWholePlan(t *testing.T) {
	now := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	s, pool := newStore(t, enforcing(), now)
	ctx := context.Background()
	user := insertUser(t, pool, "usage-report@example.test")

	if _, err := s.Consume(ctx, user, FeatureFit, "job-1"); err != nil {
		t.Fatalf("consumption: %v", err)
	}

	tier, usage, resets, err := s.Usage(ctx, user)
	if err != nil {
		t.Fatalf("Usage: %v", err)
	}
	if tier != TierFree {
		t.Errorf("tier = %q, want free", tier)
	}
	if len(usage) != len(AllFeatures()) {
		t.Fatalf("Usage reported %d features, want %d — an untouched feature must still be listed", len(usage), len(AllFeatures()))
	}
	for _, u := range usage {
		switch u.Feature {
		case FeatureFit:
			if u.Used != 1 || u.Limit != s.cfg.FreeDaily(FeatureFit) {
				t.Errorf("fit usage = %+v", u)
			}
		default:
			if u.Used != 0 {
				t.Errorf("%q reports %d used without ever being touched", u.Feature, u.Used)
			}
		}
	}
	if !resets.Equal(time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("Usage reports ResetsAt %v, want the next UTC midnight", resets)
	}
}

func TestUsageOnProReportsUnlimited(t *testing.T) {
	now := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	s, pool := newStore(t, enforcing(), now)
	ctx := context.Background()
	user := insertUser(t, pool, "usage-pro@example.test")
	makePro(t, pool, user, now.Add(24*time.Hour))

	_, usage, _, err := s.Usage(ctx, user)
	if err != nil {
		t.Fatalf("Usage: %v", err)
	}
	for _, u := range usage {
		if !u.Unlimited {
			t.Errorf("%q reports a countable limit to a pro caller; pro is the same product without the ceilings", u.Feature)
		}
	}
}

func TestShadowModeRecordsWithoutRefusing(t *testing.T) {
	now := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	s, pool := newStore(t, DefaultConfig(), now) // enforcement off, as shipped
	ctx := context.Background()
	user := insertUser(t, pool, "shadow-mode@example.test")

	limit := s.cfg.FreeDaily(FeatureFit)
	over := limit + 3
	shadowed := 0
	for i := range over {
		d, err := s.Consume(ctx, user, FeatureFit, jobRef(i))
		if err != nil {
			t.Fatalf("shadow mode refused consumption %d: %v", i+1, err)
		}
		if d.Shadowed {
			shadowed++
		}
	}
	if shadowed != over-limit {
		t.Errorf("%d consumptions were flagged as would-have-been-refused, want %d — that flag is the measurement the shadow run collects", shadowed, over-limit)
	}
	if got := usedOn(t, pool, user, FeatureFit, Day(now)); got != over {
		t.Errorf("counter reads %d, want %d — shadow mode must still count, or it describes a day that did not happen", got, over)
	}
}

// jobRef makes a distinct reference per attempt, standing in for a job id.
func jobRef(i int) string { return "job-" + strconv.Itoa(i) }
