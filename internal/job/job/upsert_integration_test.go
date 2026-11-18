//go:build integration

package job_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/job/job"
	"github.com/strelov1/freehire/internal/job/jobderive"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

func upsertParams(t *testing.T, externalID string) db.UpsertJobParams {
	t.Helper()
	j, err := job.New(job.Draft{
		Input: jobderive.Input{
			Source: "himalayas", ExternalID: externalID, Title: "Backend Engineer", Company: "Acme",
			Location: "Berlin, Germany", Description: "<p>Build services.</p>",
		},
		URL: "https://himalayas.app/acme/" + externalID,
	})
	if err != nil {
		t.Fatal(err)
	}
	return j.Fields().UpsertParams()
}

// upsertCommitted runs job.Upsert in its own transaction and commits it.
func upsertCommitted(t *testing.T, pool *pgxpool.Pool, p db.UpsertJobParams) db.UpsertJobRow {
	t.Helper()
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	row, err := job.Upsert(ctx, tx, p)
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return row
}

type activityRow struct {
	seenAge      time.Duration
	hydrated     *time.Time
	strikes      int32
	closed       bool
	stateVersion int64
}

func readActivity(t *testing.T, pool *pgxpool.Pool, id int64) activityRow {
	t.Helper()
	var seen time.Time
	var r activityRow
	if err := pool.QueryRow(context.Background(),
		`SELECT a.last_seen_at, a.hydrated_at, a.liveness_strikes, j.closed_at IS NOT NULL, a.state_version
		 FROM job_activity a JOIN jobs j ON j.id = a.job_id WHERE a.job_id = $1`, id).Scan(&seen, &r.hydrated, &r.strikes, &r.closed, &r.stateVersion); err != nil {
		t.Fatalf("read activity %d: %v", id, err)
	}
	r.seenAge = time.Since(seen)
	return r
}

func TestUpsert_NewRowIsObservedAndHydrated(t *testing.T) {
	pool := testdb.Pool(t)
	row := upsertCommitted(t, pool, upsertParams(t, "up:new"))
	a := readActivity(t, pool, row.ID)
	if a.seenAge > time.Minute || a.hydrated == nil || time.Since(*a.hydrated) > time.Minute {
		t.Errorf("new row: seen %s ago, hydrated %v; want both stamped now", a.seenAge, a.hydrated)
	}
}

// The manual, private and raw insert paths never call Upsert: their hydration stays NULL, which
// readers take as "never checked".
func TestRawInsertKeepsNullHydration(t *testing.T) {
	pool := testdb.Pool(t)
	var id int64
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO jobs (source, external_id, url, title, description, public_slug)
		 VALUES ('manual', 'raw:1', 'u', 't', 'b', 'slug-raw-1') RETURNING id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if a := readActivity(t, pool, id); a.hydrated != nil {
		t.Errorf("raw insert hydrated_at = %v, want NULL", a.hydrated)
	}
}

func TestUpsert_ExistingRowHydratesOnlyWhenTheBodyWasRead(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	p := upsertParams(t, "up:existing")
	row := upsertCommitted(t, pool, p)
	age := func() {
		if _, err := pool.Exec(ctx, `UPDATE job_activity SET last_seen_at = now() - interval '10 days', hydrated_at = now() - interval '10 days' WHERE job_id = $1`, row.ID); err != nil {
			t.Fatal(err)
		}
	}

	age()
	p.BodyFromCatalogue = true // the stored body stood in for a failed fetch: nothing was read
	upsertCommitted(t, pool, p)
	a := readActivity(t, pool, row.ID)
	if a.seenAge > time.Minute {
		t.Errorf("last_seen_at is %s old: the listing was not observed", a.seenAge)
	}
	if a.hydrated == nil || time.Since(*a.hydrated) < 9*24*time.Hour {
		t.Errorf("hydrated_at = %v, want the old stamp kept for a body that was not read", a.hydrated)
	}

	age()
	p.BodyFromCatalogue = false
	upsertCommitted(t, pool, p)
	if a := readActivity(t, pool, row.ID); a.hydrated == nil || time.Since(*a.hydrated) > time.Minute {
		t.Errorf("hydrated_at = %v, want stamped for a body the crawl read", a.hydrated)
	}
}

// A reopen clears the strikes through the state trigger, with no help from Upsert.
func TestUpsert_ReopensAClosedRowAndClearsItsStrikes(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	p := upsertParams(t, "up:reopen")
	row := upsertCommitted(t, pool, p)
	if _, err := pool.Exec(ctx, `UPDATE jobs SET closed_at = now(), closed_reason = 'unseen' WHERE id = $1`, row.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE job_activity SET liveness_strikes = 3 WHERE job_id = $1`, row.ID); err != nil {
		t.Fatal(err)
	}

	upsertCommitted(t, pool, p)

	if a := readActivity(t, pool, row.ID); a.closed || a.strikes != 0 {
		t.Errorf("closed=%t strikes=%d, want reopened with strikes cleared", a.closed, a.strikes)
	}
}

// An OPEN row keeps its strikes through a re-listing, and a closed row whose deadline has passed
// stays closed yet has its strikes cleared: the old single statement reset them whenever the
// stored row was closed, reopened or not.
func TestUpsert_StrikeParity(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	strikesOf := func(id int64) int32 { return readActivity(t, pool, id).strikes }

	open := upsertParams(t, "up:strikes-open")
	row := upsertCommitted(t, pool, open)
	if _, err := pool.Exec(ctx, `UPDATE job_activity SET liveness_strikes = 4 WHERE job_id = $1`, row.ID); err != nil {
		t.Fatal(err)
	}
	upsertCommitted(t, pool, open)
	if got := strikesOf(row.ID); got != 4 {
		t.Errorf("open row strikes = %d after a re-listing, want 4 kept", got)
	}

	expired := upsertParams(t, "up:strikes-expired")
	expired.ApplicationDeadline = pgtype.Date{Time: time.Now().UTC().AddDate(0, 0, -3), Valid: true}
	row = upsertCommitted(t, pool, expired)
	if a := readActivity(t, pool, row.ID); !a.closed {
		t.Fatal("precondition: a posting past its deadline is stored closed")
	}
	if _, err := pool.Exec(ctx, `UPDATE job_activity SET liveness_strikes = 3 WHERE job_id = $1`, row.ID); err != nil {
		t.Fatal(err)
	}
	upsertCommitted(t, pool, expired)
	if a := readActivity(t, pool, row.ID); !a.closed || a.strikes != 0 {
		t.Errorf("expired row: closed=%t strikes=%d, want still closed with strikes cleared", a.closed, a.strikes)
	}
}

// The posting and its observation commit or roll back together.
func TestUpsert_RollbackLeavesNothing(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := job.Upsert(ctx, tx, upsertParams(t, "up:rollback")); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM jobs) + (SELECT count(*) FROM job_activity)`).Scan(&n); err != nil || n != 0 {
		t.Errorf("rows after rollback = %d, %v; want 0", n, err)
	}
}

// Observe refuses to report success for a job with no activity row.
func TestObserve_RequiresExactlyOneActivityRow(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := job.Observe(ctx, tx, 987654321, true, false); err == nil {
		t.Error("Observe of a job without an activity row returned nil, want an error")
	}
}
