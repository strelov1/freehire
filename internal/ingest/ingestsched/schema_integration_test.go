//go:build integration

// Integration tests for the scheduling schema itself, against a real Postgres. What is
// asserted here is deliberately not asserted in Go: these are the rules a hand-written
// UPDATE in psql must also obey, so they belong to the table, not to the one writer that
// happens to exist today.
package ingestsched

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/strelov1/freehire/internal/platform/testdb"
)

// Postgres SQLSTATEs these tests name rather than accept any error at all. A test that
// asserts only "some error" passes just as happily when the table is missing, which is
// exactly how a schema test comes to prove nothing.
const (
	checkViolation  = "23514"
	uniqueViolation = "23505"
)

func requireSQLState(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("accepted; want refused with SQLSTATE %s", want)
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("error is not a Postgres error: %v", err)
	}
	if pgErr.Code != want {
		t.Fatalf("SQLSTATE %s (%s); want %s", pgErr.Code, pgErr.Message, want)
	}
}

// The column defaults must equal the Go defaults, because they are two places holding ONE
// fact that apply in different circumstances: the constants to a provider with NO row, the
// columns the moment a row exists. `schedule-board --manage` creates a row, so a drift
// between them silently changes the cadence of every provider handed to the scheduler --
// which is exactly what happened on 2026-09-16, when DefaultCadence moved to 2h and this
// column did not. Asserted against the CONSTANTS, not against literals, so the next move
// cannot pass by editing one side.
func TestScheduleRowDefaultsMatchTheGoDefaults(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()

	if _, err := pool.Exec(ctx,
		`INSERT INTO ingest_schedule (provider) VALUES ($1)`, "greenhouse"); err != nil {
		t.Fatalf("insert bare row: %v", err)
	}

	var (
		shards     int
		cadenceSec int
		timeoutSec int
		enabled    bool
		managed    bool
	)
	err := pool.QueryRow(ctx,
		`SELECT shards, cadence_sec, timeout_sec, enabled, managed
		   FROM ingest_schedule WHERE provider = $1`, "greenhouse").
		Scan(&shards, &cadenceSec, &timeoutSec, &enabled, &managed)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}

	if shards != DefaultShards {
		t.Errorf("shards = %d, want %d (DefaultShards)", shards, DefaultShards)
	}
	if want := int(DefaultCadence.Seconds()); cadenceSec != want {
		t.Errorf("cadence_sec = %d, want %d (DefaultCadence)", cadenceSec, want)
	}
	if want := int(DefaultRunTimeout.Seconds()); timeoutSec != want {
		t.Errorf("timeout_sec = %d, want %d (DefaultRunTimeout)", timeoutSec, want)
	}
	if !enabled {
		t.Error("enabled = false, want true — a row existing is not a decision to stop crawling")
	}
	// managed is the rollout gate: a freshly seeded override must NOT start driving the
	// provider until an operator flips it, or seeding the table would cut every provider
	// over at once.
	if managed {
		t.Error("managed = true, want false — the rollout gate must default to off")
	}
}

// A disabled provider without a stated reason is the failure this change exists to
// prevent: "nobody configured it" and "we decided not to crawl it" must not be the same
// state. The rule lives in the schema because psql is also a writer.
func TestDisablingAProviderRequiresAReason(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()

	const disable = `INSERT INTO ingest_schedule (provider, enabled, disabled_reason)
	                 VALUES ($1, false, $2)`

	for _, tc := range []struct {
		name   string
		reason any
	}{
		{"null-reason", nil},
		{"empty-reason", ""},
		{"whitespace-reason", "   "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := pool.Exec(ctx, disable, tc.name, tc.reason)
			requireSQLState(t, err, checkViolation)
		})
	}

	_, err := pool.Exec(ctx, disable, "bayt",
		"fingerprint client has no proxy support; hard-403s the prod IP")
	if err != nil {
		t.Fatalf("disabling with a reason should be accepted: %v", err)
	}
}

// An enabled row may carry a reason from a previous disable — clearing the text on every
// re-enable would destroy the record of why it was ever off.
func TestAnEnabledRowMayKeepAStaleReason(t *testing.T) {
	pool := testdb.Pool(t)
	_, err := pool.Exec(context.Background(),
		`INSERT INTO ingest_schedule (provider, enabled, disabled_reason) VALUES ($1, true, $2)`,
		"gulftalent", "was off while the proxy burned its exit IP")
	if err != nil {
		t.Fatalf("enabled row with a reason: %v", err)
	}
}

func TestScheduleRefusesNonPositiveNumbers(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()

	for _, tc := range []struct {
		column string
		value  int
	}{
		{"shards", 0},
		{"shards", -1},
		// Bounded ABOVE too: reconcile materialises one row per shard through
		// generate_series, so an unbounded count turns a typo into a statement that
		// outlives the scheduler's start timeout — and every following tick repeats it
		// first, stopping the whole fleet.
		{"shards", 65},
		{"shards", 100000},
		{"cadence_sec", 0},
		{"cadence_sec", -3600},
		{"timeout_sec", 0},
		{"timeout_sec", -30},
	} {
		name := tc.column + "=" + strconv.Itoa(tc.value)
		t.Run(name, func(t *testing.T) {
			// The column name is a test-local literal, never input.
			_, err := pool.Exec(ctx,
				`INSERT INTO ingest_schedule (provider, `+tc.column+`) VALUES ($1, $2)`,
				name, tc.value)
			requireSQLState(t, err, checkViolation)
		})
	}
}

// Run state is per shard, not per provider: shard 3 of paylocity is due independently of
// shard 4, and the primary key is what makes that true.
func TestRunStateIsKeyedByProviderAndShard(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()

	const insert = `INSERT INTO ingest_run_state (provider, shard, next_due_at)
	                VALUES ($1, $2, now())`

	for _, shard := range []int{1, 2} {
		if _, err := pool.Exec(ctx, insert, "paylocity", shard); err != nil {
			t.Fatalf("insert shard %d: %v", shard, err)
		}
	}

	_, err := pool.Exec(ctx, insert, "paylocity", 1)
	requireSQLState(t, err, uniqueViolation)
}

func TestRunStateRefusesANonPositiveShard(t *testing.T) {
	pool := testdb.Pool(t)
	_, err := pool.Exec(context.Background(),
		`INSERT INTO ingest_run_state (provider, shard, next_due_at) VALUES ($1, $2, now())`,
		"paylocity", 0)
	requireSQLState(t, err, checkViolation)
}
