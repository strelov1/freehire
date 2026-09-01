//go:build integration

// Integration tests for the plan and usage query semantics — the consumption-idempotency
// index, what a release frees, how a tailoring session's ceilings are read off its refs
// alone, and the day key that makes a rollover need no reset. All of it is SQL behaviour
// and can only be verified against a real Postgres. Run with:
// go test -tags=integration ./internal/platform/db/
package db

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

func seedPlanUser(t *testing.T, pool *pgxpool.Pool, email string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO users (email) VALUES ($1) RETURNING id`, email).Scan(&id); err != nil {
		t.Fatalf("seed user %s: %v", email, err)
	}
	return id
}

func planDay(t time.Time) pgtype.Date {
	return pgtype.Date{Time: t, Valid: true}
}

// TestProUntilIsTheWholePlan covers the one column the metered path reads: absent by
// default, movable, and clearable.
func TestProUntilIsTheWholePlan(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := seedPlanUser(t, pool, "plan-default@example.test")

	got, err := q.GetProUntil(ctx, user)
	if err != nil {
		t.Fatalf("GetProUntil: %v", err)
	}
	if got.Valid {
		t.Fatalf("a fresh account reads as pro (%v); every existing account is free, and NULL is how that is said", got.Time)
	}

	until := time.Now().UTC().Add(30 * 24 * time.Hour).Truncate(time.Second)
	if err := q.SetProUntil(ctx, SetProUntilParams{
		ID: user, ProUntil: pgtype.Timestamptz{Time: until, Valid: true},
	}); err != nil {
		t.Fatalf("SetProUntil: %v", err)
	}
	got, err = q.GetProUntil(ctx, user)
	if err != nil {
		t.Fatalf("GetProUntil after set: %v", err)
	}
	if !got.Valid || !got.Time.UTC().Equal(until) {
		t.Fatalf("GetProUntil = %v (valid=%v), want %v", got.Time, got.Valid, until)
	}

	if err := q.SetProUntil(ctx, SetProUntilParams{ID: user}); err != nil {
		t.Fatalf("SetProUntil(NULL): %v", err)
	}
	if got, err = q.GetProUntil(ctx, user); err != nil || got.Valid {
		t.Fatalf("clearing pro_until left %v (valid=%v), err %v", got.Time, got.Valid, err)
	}
}

// TestConsumptionIsIdempotentByRef is the property the whole metering path rests on: the
// same (user, feature, ref) can be charged once, and a second attempt is refused by the
// index rather than by the caller remembering to check.
func TestConsumptionIsIdempotentByRef(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := seedPlanUser(t, pool, "plan-idempotent@example.test")
	day := planDay(time.Now().UTC())

	exists, err := q.ConsumptionExists(ctx, ConsumptionExistsParams{UserID: user, Feature: "match", Ref: "job-1"})
	if err != nil {
		t.Fatalf("ConsumptionExists: %v", err)
	}
	if exists {
		t.Fatal("an untouched (user, feature, ref) reports as already charged")
	}

	insert := InsertConsumptionParams{UserID: user, Feature: "match", Day: day, Delta: 1, Ref: "job-1"}
	if err := q.InsertConsumption(ctx, insert); err != nil {
		t.Fatalf("InsertConsumption: %v", err)
	}
	if err := q.InsertConsumption(ctx, insert); err == nil {
		t.Fatal("the same ref was charged twice; usage_ledger_consume_ref_uniq did not hold, which is what a retried request would do")
	}

	if exists, err = q.ConsumptionExists(ctx, ConsumptionExistsParams{UserID: user, Feature: "match", Ref: "job-1"}); err != nil || !exists {
		t.Fatalf("ConsumptionExists after charge = %v, err %v; want true", exists, err)
	}

	// A different feature sharing the ref is a different charge: a job id and a CV id
	// that happen to be the same number must not collide.
	if err := q.InsertConsumption(ctx, InsertConsumptionParams{
		UserID: user, Feature: "tailor", Day: day, Delta: 1, Ref: "job-1",
	}); err != nil {
		t.Fatalf("the same ref under another feature was refused: %v", err)
	}
}

// TestReleaseFreesTheRefForARetry covers why a release RESTAMPS rather than compensating
// or deleting: a compensating row would leave the ref spent forever and the retry would
// find it charged, while deleting would erase the fact that a reservation was ever taken.
func TestReleaseFreesTheRefForARetry(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := seedPlanUser(t, pool, "plan-release@example.test")
	day := planDay(time.Now().UTC())
	charge := InsertConsumptionParams{UserID: user, Feature: "match", Day: day, Delta: 1, Ref: "job-7"}

	if err := q.InsertConsumption(ctx, charge); err != nil {
		t.Fatalf("InsertConsumption: %v", err)
	}
	released, err := q.ReleaseConsumption(ctx, ReleaseConsumptionParams{UserID: user, Feature: "match", Ref: "job-7"})
	if err != nil {
		t.Fatalf("ReleaseConsumption: %v", err)
	}
	if released != 1 {
		t.Fatalf("ReleaseConsumption restamped %d rows, want 1", released)
	}

	// Releasing again matches nothing and reports so — this is what lets every failure
	// path call it without first establishing whether it owes one.
	if released, err = q.ReleaseConsumption(ctx, ReleaseConsumptionParams{UserID: user, Feature: "match", Ref: "job-7"}); err != nil {
		t.Fatalf("second ReleaseConsumption: %v", err)
	}
	if released != 0 {
		t.Fatalf("a second release restamped %d rows, want 0", released)
	}

	if err := q.InsertConsumption(ctx, charge); err != nil {
		t.Fatalf("the released ref could not be charged again: %v — a retry would be free forever", err)
	}

	// The released entry is still there, restamped rather than deleted: a ledger that
	// forgets a reservation was taken cannot be reconstructed, which is the whole reason
	// it is append-only. There are now two rows for this reference — one released, one
	// live — and only the live one counts as a consumption.
	var releases, consumes int
	if err := pool.QueryRow(ctx,
		`SELECT count(*) FILTER (WHERE kind='release'), count(*) FILTER (WHERE kind='consume')
		 FROM usage_ledger WHERE user_id=$1 AND feature='match' AND ref='job-7'`,
		user).Scan(&releases, &consumes); err != nil {
		t.Fatalf("count ledger entries: %v", err)
	}
	if releases != 1 || consumes != 1 {
		t.Errorf("ledger holds %d released and %d live entries, want 1 and 1", releases, consumes)
	}
}

// TestSessionCeilingsAreReadFromRefs covers the decision that the tailoring turn ceiling
// needs no column: the ceilings a session holds are read off its '<session>#n' charges, and
// a session id that prefixes another must not borrow its charges.
func TestSessionCeilingsAreReadFromRefs(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := seedPlanUser(t, pool, "plan-ceiling@example.test")
	day := planDay(time.Now().UTC())

	for _, ref := range []string{"sess-1#1", "sess-1#2", "sess-12#1"} {
		if err := q.InsertConsumption(ctx, InsertConsumptionParams{
			UserID: user, Feature: "tailor", Day: day, Delta: 1, Ref: ref,
		}); err != nil {
			t.Fatalf("InsertConsumption %s: %v", ref, err)
		}
	}

	refs, err := q.ListConsumptionRefsByPrefix(ctx, ListConsumptionRefsByPrefixParams{
		UserID: user, Feature: "tailor", RefPrefix: "sess-1#",
	})
	if err != nil {
		t.Fatalf("ListConsumptionRefsByPrefix: %v", err)
	}
	if got := refTexts(refs); !slices.Equal(got, []string{"sess-1#1", "sess-1#2"}) {
		t.Fatalf("sess-1 holds %v, want [sess-1#1 sess-1#2] — sess-12's charge leaked in through the prefix", got)
	}

	// A release must not shrink what the session holds: the slot was sold, and reading the
	// live rows only is what keeps the ceiling from moving backwards under it.
	if _, err := q.ReleaseConsumption(ctx, ReleaseConsumptionParams{
		UserID: user, Feature: "tailor", Ref: "sess-1#1",
	}); err != nil {
		t.Fatalf("ReleaseConsumption: %v", err)
	}
	if refs, err = q.ListConsumptionRefsByPrefix(ctx, ListConsumptionRefsByPrefixParams{
		UserID: user, Feature: "tailor", RefPrefix: "sess-1#",
	}); err != nil {
		t.Fatalf("ListConsumptionRefsByPrefix after release: %v", err)
	}
	if got := refTexts(refs); !slices.Equal(got, []string{"sess-1#2"}) {
		t.Fatalf("after releasing #1 sess-1 holds %v, want [sess-1#2] — the highest slot is what the ceiling follows", got)
	}

	if refs, err = q.ListConsumptionRefsByPrefix(ctx, ListConsumptionRefsByPrefixParams{
		UserID: user, Feature: "tailor", RefPrefix: "sess-12#",
	}); err != nil || !slices.Equal(refTexts(refs), []string{"sess-12#1"}) {
		t.Fatalf("sess-12 holds %v (err %v), want [sess-12#1]", refTexts(refs), err)
	}
}

// refTexts flattens the nullable ref column for comparison. Every row this query returns was
// written with a reference, so an absent one would itself be the finding.
func refTexts(refs []pgtype.Text) []string {
	out := make([]string, 0, len(refs))
	for _, r := range refs {
		out = append(out, r.String)
	}
	slices.Sort(out)
	return out
}

// TestUsageDayIsKeyedByDay covers the lazy rollover: yesterday's counter is a different
// row, so a new day needs nothing reset and no scheduled job can forget to run.
func TestUsageDayIsKeyedByDay(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := seedPlanUser(t, pool, "plan-day@example.test")
	now := time.Now().UTC()
	today, yesterday := planDay(now), planDay(now.AddDate(0, 0, -1))

	for _, d := range []pgtype.Date{yesterday, today} {
		if err := q.EnsureUsageDay(ctx, EnsureUsageDayParams{UserID: user, Feature: "match", Day: d}); err != nil {
			t.Fatalf("EnsureUsageDay: %v", err)
		}
	}
	// Seeding again must not reset a counter that is already there.
	if err := q.SetUsageDay(ctx, SetUsageDayParams{UserID: user, Feature: "match", Day: yesterday, Used: 3}); err != nil {
		t.Fatalf("SetUsageDay: %v", err)
	}
	if err := q.EnsureUsageDay(ctx, EnsureUsageDayParams{UserID: user, Feature: "match", Day: yesterday}); err != nil {
		t.Fatalf("re-EnsureUsageDay: %v", err)
	}

	used, err := q.GetUsageDayForUpdate(ctx, GetUsageDayForUpdateParams{UserID: user, Feature: "match", Day: yesterday})
	if err != nil {
		t.Fatalf("GetUsageDayForUpdate: %v", err)
	}
	if used != 3 {
		t.Fatalf("yesterday reads %d after a re-seed, want 3 — EnsureUsageDay overwrote a live counter", used)
	}
	if used, err = q.GetUsageDayForUpdate(ctx, GetUsageDayForUpdateParams{UserID: user, Feature: "match", Day: today}); err != nil || used != 0 {
		t.Fatalf("today reads %d (err %v), want 0 — yesterday's consumption carried into a new day", used, err)
	}

	rows, err := q.ListUsageForDay(ctx, ListUsageForDayParams{UserID: user, Day: yesterday})
	if err != nil {
		t.Fatalf("ListUsageForDay: %v", err)
	}
	if len(rows) != 1 || rows[0].Feature != "match" || rows[0].Used != 3 {
		t.Fatalf("ListUsageForDay = %+v, want one match row of 3", rows)
	}
}

// TestDeletingAUserErasesTheirUsage covers what account deletion promises. The foreign
// keys cascade, but deletion states what it erases rather than trusting a constraint to
// mean it — and a new account on the same address must not inherit a spent allowance.
func TestDeletingAUserErasesTheirUsage(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := seedPlanUser(t, pool, "plan-delete@example.test")
	day := planDay(time.Now().UTC())

	if err := q.EnsureUsageDay(ctx, EnsureUsageDayParams{UserID: user, Feature: "match", Day: day}); err != nil {
		t.Fatalf("EnsureUsageDay: %v", err)
	}
	if err := q.InsertConsumption(ctx, InsertConsumptionParams{
		UserID: user, Feature: "match", Day: day, Delta: 1, Ref: "job-9",
	}); err != nil {
		t.Fatalf("InsertConsumption: %v", err)
	}

	if err := q.DeleteUsageForUser(ctx, user); err != nil {
		t.Fatalf("DeleteUsageForUser: %v", err)
	}
	if err := q.DeleteUsageDailyForUser(ctx, user); err != nil {
		t.Fatalf("DeleteUsageDailyForUser: %v", err)
	}

	entries, err := q.ListUsageLedger(ctx, ListUsageLedgerParams{UserID: user, Limit: 10})
	if err != nil {
		t.Fatalf("ListUsageLedger: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("%d ledger entries survived deletion", len(entries))
	}
	rows, err := q.ListUsageForDay(ctx, ListUsageForDayParams{UserID: user, Day: day})
	if err != nil {
		t.Fatalf("ListUsageForDay: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("%d daily counters survived deletion", len(rows))
	}
}
