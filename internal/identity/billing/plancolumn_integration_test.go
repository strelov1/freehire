//go:build integration

// Integration tests for the shape of the plan column itself, against a real Postgres.
//
// They exercise the SCHEMA rather than any Go code, and that is the point: the guarantee
// under test is that users.pro_until is DERIVED from its three sources and cannot be
// assigned, and a guarantee enforced by the schema has to be tested against the schema. A
// test that went through the billing service would prove only that the service happens not
// to write it today.
//
// Run with: go test -tags=integration ./internal/identity/billing/
package billing

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/platform/testdb"
)

// The migration and its rollback, addressed from this package's directory — which is where
// `go test` runs. The tests below execute the operator's own files rather than a copy of
// their statements, so a change that breaks either one fails here rather than on prod.
var (
	forwardMigration = filepath.Join("..", "..", "..", "migrations", "0135_pro_until_sources.sql")
	rollbackScript   = filepath.Join("..", "..", "..", "deploy", "rollback", "0135_pro_until_sources.down.sql")
	reapplyScript    = filepath.Join("..", "..", "..", "deploy", "rollback", "0135_pro_until_sources.reapply.sql")
)

// setSource writes one source column. It interpolates the column NAME, which is safe here
// and nowhere else: the value is one of three constants in this file, never anything a
// caller supplies.
func setSource(t *testing.T, pool *pgxpool.Pool, userID int64, column string, at *time.Time) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`UPDATE users SET `+column+` = $1 WHERE id = $2`, at, userID); err != nil {
		t.Fatalf("set %s: %v", column, err)
	}
}

func TestProUntilIsNullWhenNoSourceConfersAnything(t *testing.T) {
	pool := testdb.Pool(t)
	id := insertUser(t, pool, "no-source@example.com")

	if got := proUntil(t, pool, id); got != nil {
		t.Fatalf("pro_until = %v, want NULL for an account with no source", got)
	}
}

func TestProUntilTakesTheOnlySourceThatConfers(t *testing.T) {
	pool := testdb.Pool(t)
	want := time.Now().Add(30 * 24 * time.Hour).UTC().Truncate(time.Microsecond)

	for _, column := range []string{"pro_until_stripe", "pro_until_revenuecat", "pro_until_granted"} {
		t.Run(column, func(t *testing.T) {
			id := insertUser(t, pool, column+"-only@example.com")
			setSource(t, pool, id, column, &want)

			got := proUntil(t, pool, id)
			if got == nil || !got.Equal(want) {
				t.Fatalf("pro_until = %v, want %v from %s", got, want, column)
			}
		})
	}
}

// The whole reason the column is derived rather than written: two sources must compose, and
// the one that reaches furthest is the one the user paid for.
func TestProUntilTakesTheFurthestReach(t *testing.T) {
	pool := testdb.Pool(t)
	near := time.Now().Add(24 * time.Hour).UTC().Truncate(time.Microsecond)
	far := time.Now().Add(90 * 24 * time.Hour).UTC().Truncate(time.Microsecond)

	id := insertUser(t, pool, "two-sources@example.com")
	setSource(t, pool, id, "pro_until_stripe", &near)
	setSource(t, pool, id, "pro_until_revenuecat", &far)

	got := proUntil(t, pool, id)
	if got == nil || !got.Equal(far) {
		t.Fatalf("pro_until = %v, want the further reach %v", got, far)
	}
}

// A provider clearing its own column must not shorten a plan another source still confers.
// This is the App Store purchase that a Stripe sync would otherwise revoke.
func TestClearingOneSourceLeavesTheOtherStanding(t *testing.T) {
	pool := testdb.Pool(t)
	until := time.Now().Add(30 * 24 * time.Hour).UTC().Truncate(time.Microsecond)

	id := insertUser(t, pool, "one-cleared@example.com")
	setSource(t, pool, id, "pro_until_revenuecat", &until)
	setSource(t, pool, id, "pro_until_stripe", nil)

	got := proUntil(t, pool, id)
	if got == nil || !got.Equal(until) {
		t.Fatalf("pro_until = %v, want %v — clearing Stripe must not revoke a store purchase", got, until)
	}
}

// The guarantee that makes every other test here redundant in production: the statement
// that would erase another source's grant does not run at all.
func TestProUntilCannotBeAssigned(t *testing.T) {
	pool := testdb.Pool(t)
	id := insertUser(t, pool, "unassignable@example.com")
	at := time.Now().Add(time.Hour)

	_, err := pool.Exec(context.Background(), `UPDATE users SET pro_until = $1 WHERE id = $2`, at, id)
	if err == nil {
		t.Fatal("assigning pro_until succeeded; the column must be generated, not writable")
	}

	// 428C9 is ERRCODE_GENERATED_ALWAYS. Asserting the code rather than the message keeps
	// the test from breaking on a Postgres release that rewords the sentence, and keeps it
	// from passing on an unrelated error that merely happens to be an error.
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "428C9" {
		t.Fatalf("assigning pro_until failed with %v, want SQLSTATE 428C9 (generated column)", err)
	}
}

// The rollback is executed here rather than described in a runbook, because a rollback that
// has never been run is a paragraph, not a plan. Each test gets its own database cloned from
// the template, so undoing the migration inside one costs the rest of the suite nothing.
func TestTheRollbackRestoresAWritableColumnWithoutMovingAnybodysPlan(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()

	near := time.Now().Add(24 * time.Hour).UTC().Truncate(time.Microsecond)
	far := time.Now().Add(90 * 24 * time.Hour).UTC().Truncate(time.Microsecond)

	subscriber := insertUser(t, pool, "rollback-two-sources@example.com")
	setSource(t, pool, subscriber, "pro_until_stripe", &near)
	setSource(t, pool, subscriber, "pro_until_revenuecat", &far)

	free := insertUser(t, pool, "rollback-free@example.com")

	runSQLFile(t, pool, rollbackScript)

	if got := proUntil(t, pool, subscriber); got == nil || !got.Equal(far) {
		t.Fatalf("after rollback pro_until = %v, want the furthest reach %v — nobody's plan may move", got, far)
	}
	if got := proUntil(t, pool, free); got != nil {
		t.Fatalf("after rollback a free account has pro_until = %v, want NULL", got)
	}

	// The point of rolling back: the older binary's SetProUntil has to work again.
	if _, err := pool.Exec(ctx, `UPDATE users SET pro_until = $1 WHERE id = $2`, near, subscriber); err != nil {
		t.Fatalf("after rollback pro_until is still not writable: %v", err)
	}

	// The sources are kept rather than dropped, because which origin conferred a plan is not
	// reconstructible afterwards. Losing them is what makes a re-application re-split every
	// account by stripe_customer_id — turning a store subscription into an unrevocable grant,
	// and a grant into something the next Stripe sync can shorten.
	if got := readSource(t, pool, subscriber, "pro_until_revenuecat"); got == nil || !got.Equal(far) {
		t.Fatalf("the rollback lost pro_until_revenuecat (%v); the origin of a plan must survive it", got)
	}
	if got := readSource(t, pool, subscriber, "pro_until_stripe"); got == nil || !got.Equal(near) {
		t.Fatalf("the rollback lost pro_until_stripe (%v)", got)
	}

	// And rolling forward again restores the derived column WITHOUT re-splitting anything.
	runSQLFile(t, pool, reapplyScript)

	if got := readSource(t, pool, subscriber, "pro_until_revenuecat"); got == nil || !got.Equal(far) {
		t.Fatalf("after rolling forward pro_until_revenuecat = %v, want %v — no account may change origin", got, far)
	}
	if got := readSource(t, pool, subscriber, "pro_until_granted"); got != nil {
		t.Fatalf("after rolling forward pro_until_granted = %v, want NULL — a store subscription must not become an unrevocable grant", got)
	}
	if got := proUntil(t, pool, subscriber); got == nil || !got.Equal(far) {
		t.Fatalf("after rolling forward pro_until = %v, want %v", got, far)
	}
}

// The backfill is the half of the migration that a fresh install never exercises: initdb
// runs 0135 against an empty users table, so the two UPDATEs touch nothing and a mistake in
// them would ship unnoticed. The rollback restores exactly the pre-0135 shape, which is what
// makes it possible to seed that shape here and run the real forward file over it.
//
// Getting this wrong is silent in both directions: everything into _stripe lets the next
// sync revoke support's manual grants, and everything into _granted makes today's
// subscribers permanently Pro, cancellation included.
func TestTheMigrationSeparatesExistingPlansByTheirOrigin(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	until := time.Now().Add(30 * 24 * time.Hour).UTC().Truncate(time.Microsecond)

	// The rollback restores a writable pro_until but deliberately KEEPS the source columns,
	// so it lands one step short of the shape 0135 was written against. Dropping them here
	// completes the reconstruction; doing it in the test rather than in the rollback file is
	// the point of issue: an operator must never lose those columns.
	runSQLFile(t, pool, rollbackScript)
	if _, err := pool.Exec(ctx, `ALTER TABLE users
		DROP COLUMN pro_until_stripe, DROP COLUMN pro_until_revenuecat, DROP COLUMN pro_until_granted`); err != nil {
		t.Fatalf("reconstruct the pre-0135 shape: %v", err)
	}

	// The three shapes an account can be in before 0135.
	bought := insertUser(t, pool, "split-stripe@example.com")
	granted := insertUser(t, pool, "split-granted@example.com")
	free := insertUser(t, pool, "split-free@example.com")

	if _, err := pool.Exec(ctx,
		`UPDATE users SET pro_until = $1, stripe_customer_id = 'cus_split' WHERE id = $2`, until, bought); err != nil {
		t.Fatalf("seed the Stripe subscriber: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE users SET pro_until = $1 WHERE id = $2`, until, granted); err != nil {
		t.Fatalf("seed the hand-granted account: %v", err)
	}

	runSQLFile(t, pool, forwardMigration)

	for _, c := range []struct {
		name    string
		userID  int64
		sources map[string]*time.Time
	}{
		{
			// A customer of the provider: the value must stay where a cancellation can revoke it.
			name:   "a Stripe subscriber's plan stays revocable",
			userID: bought,
			sources: map[string]*time.Time{
				"pro_until_stripe": &until, "pro_until_revenuecat": nil, "pro_until_granted": nil,
			},
		},
		{
			// No customer means nobody sold it, so a provider must not be able to take it away.
			name:   "a hand-set plan stops being revocable by a provider",
			userID: granted,
			sources: map[string]*time.Time{
				"pro_until_stripe": nil, "pro_until_revenuecat": nil, "pro_until_granted": &until,
			},
		},
		{
			name:   "an account with no plan gains none",
			userID: free,
			sources: map[string]*time.Time{
				"pro_until_stripe": nil, "pro_until_revenuecat": nil, "pro_until_granted": nil,
			},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			for column, want := range c.sources {
				got := readSource(t, pool, c.userID, column)
				switch {
				case want == nil && got != nil:
					t.Errorf("%s = %v, want NULL", column, got)
				case want != nil && (got == nil || !got.Equal(*want)):
					t.Errorf("%s = %v, want %v", column, got, *want)
				}
			}
		})
	}

	// The migration reshapes where a plan is recorded. It must not move when one ends.
	if got := proUntil(t, pool, bought); got == nil || !got.Equal(until) {
		t.Fatalf("the Stripe subscriber's pro_until = %v, want %v unchanged by the migration", got, until)
	}
	if got := proUntil(t, pool, granted); got == nil || !got.Equal(until) {
		t.Fatalf("the granted account's pro_until = %v, want %v unchanged by the migration", got, until)
	}
	if got := proUntil(t, pool, free); got != nil {
		t.Fatalf("the free account's pro_until = %v, want NULL", got)
	}
}

// runSQLFile executes a migration or rollback file as the operator would: whole, and from
// disk.
func runSQLFile(t *testing.T, pool *pgxpool.Pool, path string) {
	t.Helper()
	sql, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if _, err := pool.Exec(context.Background(), string(sql)); err != nil {
		t.Fatalf("run %s: %v", path, err)
	}
}

func readSource(t *testing.T, pool *pgxpool.Pool, userID int64, column string) *time.Time {
	t.Helper()
	var out *time.Time
	if err := pool.QueryRow(context.Background(),
		`SELECT `+column+` FROM users WHERE id = $1`, userID).Scan(&out); err != nil {
		t.Fatalf("read %s: %v", column, err)
	}
	return out
}
