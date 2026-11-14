//go:build integration

// Release B writers: what ObserveJobActivity, the ordinary touch and the covered refresh write,
// what they must leave alone, and the invariants the old statements held that moved with them
// (hydration of a new row, a body the pipeline substituted, strike resets, the expiry and reopen
// gates). Footprint tests live in job_activity_footprint_integration_test.go and the two-session
// races in job_activity_fence_integration_test.go.
//
// Run with: go test -tags=integration ./internal/platform/db/
package db

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type activityRow struct {
	LastSeen time.Time
	Hydrated pgtype.Timestamptz
	Strikes  int32
	StateVer int64
}

func readActivity(t *testing.T, pool interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, id int64) activityRow {
	t.Helper()
	var a activityRow
	if err := pool.QueryRow(context.Background(),
		`SELECT last_seen_at, hydrated_at, liveness_strikes, state_version FROM job_activity WHERE job_id = $1`, id).
		Scan(&a.LastSeen, &a.Hydrated, &a.Strikes, &a.StateVer); err != nil {
		t.Fatalf("read activity of %d: %v", id, err)
	}
	return a
}

func TestObserveJobActivity(t *testing.T) {
	ctx := context.Background()
	pool := startPostgres(t)
	q := New(pool)
	truncate(t, pool)
	job, err := ingestUpsert(ctx, q, ingestParams("acme:obs", "Observed"))
	if err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-10 * 24 * time.Hour)
	reset := func() {
		t.Helper()
		if _, err := pool.Exec(ctx, `UPDATE job_activity SET last_seen_at = $2, hydrated_at = $2, liveness_strikes = 3 WHERE job_id = $1`, job.ID, old); err != nil {
			t.Fatal(err)
		}
	}
	jobsBefore := jobsFootprint(t, pool, job.ID)

	t.Run("body read advances hydration, a plain sighting does not", func(t *testing.T) {
		reset()
		before := readActivity(t, pool, job.ID)
		if n, err := q.ObserveJobActivity(ctx, ObserveJobActivityParams{JobID: job.ID}); err != nil || n != 1 {
			t.Fatalf("observe = %d, %v; want 1 row", n, err)
		}
		got := readActivity(t, pool, job.ID)
		if !got.LastSeen.After(before.LastSeen) || !got.Hydrated.Time.Equal(before.Hydrated.Time) || got.Strikes != 3 {
			t.Errorf("plain sighting: %+v from %+v; want last_seen advanced, hydration and strikes unchanged", got, before)
		}
		if _, err := q.ObserveJobActivity(ctx, ObserveJobActivityParams{JobID: job.ID, BodyRead: true}); err != nil {
			t.Fatal(err)
		}
		if got := readActivity(t, pool, job.ID); !got.Hydrated.Time.After(before.Hydrated.Time) || got.Strikes != 3 {
			t.Errorf("body read: %+v; want hydration advanced and strikes kept", got)
		}
	})

	t.Run("strikes are cleared only when asked", func(t *testing.T) {
		reset()
		if _, err := q.ObserveJobActivity(ctx, ObserveJobActivityParams{JobID: job.ID, BodyRead: true}); err != nil {
			t.Fatal(err)
		}
		if got := readActivity(t, pool, job.ID); got.Strikes != 3 {
			t.Fatalf("strikes = %d after an observation that did not ask, want 3", got.Strikes)
		}
		if _, err := q.ObserveJobActivity(ctx, ObserveJobActivityParams{JobID: job.ID, ResetStrikes: true}); err != nil {
			t.Fatal(err)
		}
		if got := readActivity(t, pool, job.ID); got.Strikes != 0 {
			t.Fatalf("strikes = %d after reset_strikes, want 0", got.Strikes)
		}
	})

	t.Run("a NULL hydration stays NULL and last_seen never moves backwards", func(t *testing.T) {
		future := time.Now().Add(time.Hour)
		if _, err := pool.Exec(ctx, `UPDATE job_activity SET last_seen_at = $2, hydrated_at = NULL WHERE job_id = $1`, job.ID, future); err != nil {
			t.Fatal(err)
		}
		if _, err := q.ObserveJobActivity(ctx, ObserveJobActivityParams{JobID: job.ID}); err != nil {
			t.Fatal(err)
		}
		got := readActivity(t, pool, job.ID)
		if got.Hydrated.Valid || got.LastSeen.Before(future.Add(-time.Second)) {
			t.Errorf("got %+v; want hydration still NULL and last_seen kept at %v", got, future)
		}
	})

	t.Run("never bumps the fence and never touches jobs", func(t *testing.T) {
		reset()
		before := readActivity(t, pool, job.ID)
		if _, err := q.ObserveJobActivity(ctx, ObserveJobActivityParams{JobID: job.ID, BodyRead: true, ResetStrikes: true}); err != nil {
			t.Fatal(err)
		}
		if got := readActivity(t, pool, job.ID); got.StateVer != before.StateVer {
			t.Errorf("state_version %d -> %d: an observation must not fence another observation", before.StateVer, got.StateVer)
		}
		requireJobsTupleUntouched(t, pool, job.ID, jobsBefore, "ObserveJobActivity")
	})

	t.Run("a job without an activity row reports zero rows", func(t *testing.T) {
		if n, err := q.ObserveJobActivity(ctx, ObserveJobActivityParams{JobID: 987654321}); err != nil || n != 0 {
			t.Fatalf("observe of an unknown job = %d, %v; want 0 rows", n, err)
		}
	})

	t.Run("a rolled-back observation leaves nothing", func(t *testing.T) {
		reset()
		before := readActivity(t, pool, job.ID)
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := New(tx).ObserveJobActivity(ctx, ObserveJobActivityParams{JobID: job.ID, BodyRead: true, ResetStrikes: true}); err != nil {
			t.Fatal(err)
		}
		if err := tx.Rollback(ctx); err != nil {
			t.Fatal(err)
		}
		if got := readActivity(t, pool, job.ID); got != before {
			t.Errorf("after rollback %+v, want %+v", got, before)
		}
	})
}

// A new row's hydration: the insert trigger copies the legacy default (NULL), the full write then
// stamps it as the second command; a raw, manual or private insert never does and keeps NULL (the
// 0144 "never checked" reading); a body the pipeline filled in from the stored row is not a read.
func TestFullWriteHydration(t *testing.T) {
	ctx := context.Background()
	pool := startPostgres(t)
	q := New(pool)
	truncate(t, pool)

	t.Run("insert: trigger-created row is hydrated by the observation", func(t *testing.T) {
		row, err := q.UpsertJob(ctx, ingestParams("acme:new", "New"))
		if err != nil {
			t.Fatal(err)
		}
		if !row.Inserted.Bool {
			t.Fatal("not an insert")
		}
		if got := readActivity(t, pool, row.ID); got.Hydrated.Valid {
			t.Fatalf("hydration right after the insert = %v, want NULL (the trigger copies the legacy NULL)", got.Hydrated)
		}
		if _, err := q.ObserveJobActivity(ctx, ObserveJobActivityParams{JobID: row.ID, BodyRead: true}); err != nil {
			t.Fatal(err)
		}
		if got := readActivity(t, pool, row.ID); !got.Hydrated.Valid || time.Since(got.Hydrated.Time) > time.Minute {
			t.Fatalf("hydration after the observation = %v, want ~now", got.Hydrated)
		}
	})

	t.Run("an upsert that only re-sees a row leaves hydration to the observation's flag", func(t *testing.T) {
		first, err := upsertObserved(ctx, q, ingestParams("acme:re", "Re"))
		if err != nil {
			t.Fatal(err)
		}
		stale := time.Now().Add(-40 * 24 * time.Hour)
		if _, err := pool.Exec(ctx, `UPDATE job_activity SET hydrated_at = $2 WHERE job_id = $1`, first.ID, stale); err != nil {
			t.Fatal(err)
		}
		p := ingestParams("acme:re", "Re v2")
		p.BodyFromCatalogue = true
		if _, err := upsertObserved(ctx, q, p); err != nil {
			t.Fatal(err)
		}
		if got := readActivity(t, pool, first.ID); !got.Hydrated.Time.Equal(stale.Truncate(time.Microsecond)) {
			t.Errorf("hydration = %v after a substituted body, want it kept at %v", got.Hydrated.Time, stale)
		}
		if _, err := upsertObserved(ctx, q, ingestParams("acme:re", "Re v3")); err != nil {
			t.Fatal(err)
		}
		if got := readActivity(t, pool, first.ID); time.Since(got.Hydrated.Time) > time.Minute {
			t.Errorf("hydration = %v after a body that was read, want ~now", got.Hydrated.Time)
		}
	})

	t.Run("a raw or manual insert keeps NULL hydration", func(t *testing.T) {
		var id int64
		if err := pool.QueryRow(ctx, `INSERT INTO jobs (source, external_id, url, title, public_slug) VALUES ('manual','m:1','u','t','m-1') RETURNING id`).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if got := readActivity(t, pool, id); got.Hydrated.Valid {
			t.Errorf("raw insert hydration = %v, want NULL", got.Hydrated)
		}
	})
}

// The gates that decide which statement may touch a posting, and what each one writes.
func TestTouchOpenJobGates(t *testing.T) {
	ctx := context.Background()
	pool := startPostgres(t)
	q := New(pool)
	truncate(t, pool)
	cutoff := pgtype.Timestamptz{Time: time.Now().Add(-14 * 24 * time.Hour), Valid: true}
	touch := func(ext string) (TouchOpenJobRow, error) {
		return q.TouchOpenJob(ctx, TouchOpenJobParams{Source: "justjoin", ExternalID: ext, WithheldIfSeenBefore: cutoff})
	}
	seed := func(ext, set string) int64 {
		t.Helper()
		p := ingestParams(ext, "Job")
		p.Source = "justjoin"
		job, err := ingestUpsert(ctx, q, p)
		if err != nil {
			t.Fatal(err)
		}
		if set != "" {
			if _, err := pool.Exec(ctx, `UPDATE jobs SET `+set+` WHERE id = $1`, job.ID); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := pool.Exec(ctx, `UPDATE job_activity SET last_seen_at = now() - interval '2 days' WHERE job_id = $1`, job.ID); err != nil {
			t.Fatal(err)
		}
		return job.ID
	}

	open := seed("jj:open", "")
	closed := seed("jj:closed", "closed_at = now(), closed_reason = 'unseen'")
	expired := seed("jj:expired", "application_deadline = current_date - 1")
	future := seed("jj:future", "application_deadline = current_date + 5")

	for _, tc := range []struct {
		ext     string
		id      int64
		touched bool
	}{{"jj:open", open, true}, {"jj:future", future, true}, {"jj:closed", closed, false}, {"jj:expired", expired, false}} {
		before := readActivity(t, pool, tc.id)
		row, err := touch(tc.ext)
		switch {
		case tc.touched && (err != nil || row.Fenced || row.ID != tc.id || row.CompanySlug != "acme"):
			t.Errorf("%s: %+v, %v; want the open posting touched, company acme", tc.ext, row, err)
		case !tc.touched && !errors.Is(err, pgx.ErrNoRows):
			t.Errorf("%s: err = %v, want pgx.ErrNoRows so the full TouchJob decides (closed or expired)", tc.ext, err)
		}
		advanced := readActivity(t, pool, tc.id).LastSeen.After(before.LastSeen)
		if advanced != tc.touched {
			t.Errorf("%s: last_seen advanced = %v, want %v", tc.ext, advanced, tc.touched)
		}
	}
	if _, err := touch("jj:absent"); !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("absent posting: err = %v, want pgx.ErrNoRows", err)
	}

	t.Run("a posting search withheld for going unseen is queued back, others are not", func(t *testing.T) {
		dup := seed("jj:dup", "")
		priv := seed("jj:private", "is_private = true")
		recent := seed("jj:recent", "")
		withheld := seed("jj:withheld", "")
		if _, err := pool.Exec(ctx, `UPDATE jobs SET duplicate_of_role = $2 WHERE id = $1`, dup, open); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `UPDATE job_activity SET last_seen_at = now() - interval '30 days' WHERE job_id = ANY($1)`, []int64{dup, priv, withheld}); err != nil {
			t.Fatal(err)
		}
		// Marking a duplicate queues its owner (a trigger): clear the fixture's own entries.
		if _, err := pool.Exec(ctx, `TRUNCATE search_outbox`); err != nil {
			t.Fatal(err)
		}
		for _, ext := range []string{"jj:dup", "jj:private", "jj:recent", "jj:withheld"} {
			if row, err := touch(ext); err != nil || row.Fenced {
				t.Fatalf("%s: %+v, %v", ext, row, err)
			}
		}
		var queued []int64
		rows, err := pool.Query(ctx, `SELECT job_id FROM search_outbox ORDER BY job_id`)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				t.Fatal(err)
			}
			queued = append(queued, id)
		}
		if len(queued) != 1 || queued[0] != withheld {
			t.Errorf("search_outbox = %v, want only the canonical public posting that was withheld (%d); recent=%d dup=%d private=%d",
				queued, withheld, recent, dup, priv)
		}
	})
	_ = open
}

// The full touch keeps the old exceptions: an expired deadline closes the posting and it is NOT
// reopened by a later touch; ResetStrikes reports the old row was closed so the observation can
// clear strikes even when the touch leaves it closed; a deferred reopen writes nothing.
func TestFullTouchStrikeAndExpiryParity(t *testing.T) {
	ctx := context.Background()
	pool := startPostgres(t)
	q := New(pool)
	truncate(t, pool)
	cutoff := pgtype.Timestamptz{Time: time.Now().Add(-14 * 24 * time.Hour), Valid: true}
	seed := func(ext, set string) int64 {
		t.Helper()
		p := ingestParams(ext, "Job")
		p.Source = "justjoin"
		job, err := ingestUpsert(ctx, q, p)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `UPDATE jobs SET `+set+` WHERE id = $1`, job.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `UPDATE job_activity SET liveness_strikes = 2, last_seen_at = now() - interval '2 days' WHERE job_id = $1`, job.ID); err != nil {
			t.Fatal(err)
		}
		return job.ID
	}

	t.Run("an expired closed posting stays closed and its strikes are cleared", func(t *testing.T) {
		id := seed("jj:exp", "closed_at = now() - interval '1 day', closed_reason = 'expired', application_deadline = current_date - 1")
		row, err := touchObserved(ctx, q, TouchJobParams{Source: "justjoin", ExternalID: "jj:exp", AllowReopen: true, WithheldIfSeenBefore: cutoff})
		if err != nil {
			t.Fatal(err)
		}
		if row.Reopened || !row.ResetStrikes || row.Deferred {
			t.Errorf("row = %+v; want not reopened, strikes reset, not deferred", row)
		}
		var closed bool
		if err := pool.QueryRow(ctx, `SELECT closed_at IS NOT NULL FROM jobs WHERE id = $1`, id).Scan(&closed); err != nil || !closed {
			t.Errorf("closed = %v, %v; want the expired posting left closed", closed, err)
		}
		if got := readActivity(t, pool, id); got.Strikes != 0 || time.Since(got.LastSeen) > time.Minute {
			t.Errorf("activity = %+v; want strikes cleared and last_seen refreshed", got)
		}
	})

	t.Run("an open posting keeps its strikes", func(t *testing.T) {
		id := seed("jj:keep", "updated_at = updated_at")
		row, err := touchObserved(ctx, q, TouchJobParams{Source: "justjoin", ExternalID: "jj:keep", AllowReopen: true, WithheldIfSeenBefore: cutoff})
		if err != nil || row.ResetStrikes {
			t.Fatalf("row = %+v, %v; want no strike reset for an open row", row, err)
		}
		if got := readActivity(t, pool, id); got.Strikes != 2 {
			t.Errorf("strikes = %d, want 2 kept", got.Strikes)
		}
	})

	t.Run("a reopen is cleared by the state trigger", func(t *testing.T) {
		id := seed("jj:reopen", "closed_at = now(), closed_reason = 'unseen'")
		row, err := touchObserved(ctx, q, TouchJobParams{Source: "justjoin", ExternalID: "jj:reopen", AllowReopen: true, WithheldIfSeenBefore: cutoff})
		if err != nil || !row.Reopened {
			t.Fatalf("row = %+v, %v; want reopened", row, err)
		}
		if got := readActivity(t, pool, id); got.Strikes != 0 {
			t.Errorf("strikes = %d after a reopen, want 0", got.Strikes)
		}
	})

	t.Run("a deferred reopen writes nothing", func(t *testing.T) {
		id := seed("jj:defer", "closed_at = now(), closed_reason = 'unseen'")
		before := readActivity(t, pool, id)
		row, err := touchObserved(ctx, q, TouchJobParams{Source: "justjoin", ExternalID: "jj:defer", WithheldIfSeenBefore: cutoff})
		if err != nil || !row.Deferred {
			t.Fatalf("row = %+v, %v; want deferred", row, err)
		}
		if got := readActivity(t, pool, id); got != before {
			t.Errorf("activity = %+v, want untouched %+v", got, before)
		}
	})
}

// The full upsert keeps the old strike rules without a second query: the row it overwrote was
// closed (RETURNING old) tells the observation to clear the strikes, which covers the expired
// posting that stays closed as well as a reopen; an open row keeps them; an insert has none.
func TestFullUpsertStrikeParity(t *testing.T) {
	ctx := context.Background()
	pool := startPostgres(t)
	q := New(pool)

	setup := func(t *testing.T, set string) int64 {
		t.Helper()
		truncate(t, pool)
		job, err := ingestUpsert(ctx, q, ingestParams("acme:s", "Struck"))
		if err != nil {
			t.Fatal(err)
		}
		if set != "" {
			if _, err := pool.Exec(ctx, `UPDATE jobs SET `+set+` WHERE id = $1`, job.ID); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := pool.Exec(ctx, `UPDATE job_activity SET liveness_strikes = 4 WHERE job_id = $1`, job.ID); err != nil {
			t.Fatal(err)
		}
		return job.ID
	}

	t.Run("an open row keeps its strikes", func(t *testing.T) {
		id := setup(t, "")
		row, err := upsertObserved(ctx, q, ingestParams("acme:s", "Struck again"))
		if err != nil || row.ResetStrikes {
			t.Fatalf("row = %+v, %v; want no reset for an open row", row, err)
		}
		if got := readActivity(t, pool, id); got.Strikes != 4 {
			t.Errorf("strikes = %d, want 4", got.Strikes)
		}
	})
	t.Run("a reopen clears them", func(t *testing.T) {
		id := setup(t, "closed_at = now(), closed_reason = 'unseen'")
		row, err := upsertObserved(ctx, q, ingestParams("acme:s", "Back"))
		if err != nil || !row.ResetStrikes || row.ClosedAt.Valid {
			t.Fatalf("row = %+v, %v; want reopened with the reset flag", row, err)
		}
		if got := readActivity(t, pool, id); got.Strikes != 0 {
			t.Errorf("strikes = %d, want 0", got.Strikes)
		}
	})
	t.Run("an expired posting that stays closed has them cleared too", func(t *testing.T) {
		id := setup(t, "closed_at = now(), closed_reason = 'expired'")
		p := ingestParams("acme:s", "Still expired")
		p.ApplicationDeadline = pgtype.Date{Time: time.Now().UTC().AddDate(0, 0, -2), Valid: true}
		row, err := upsertObserved(ctx, q, p)
		if err != nil || !row.ResetStrikes || !row.ClosedAt.Valid {
			t.Fatalf("row = %+v, %v; want still closed with the reset flag", row, err)
		}
		if got := readActivity(t, pool, id); got.Strikes != 0 {
			t.Errorf("strikes = %d, want 0 (the old statement cleared them on any overwrite of a closed row)", got.Strikes)
		}
	})
	t.Run("an insert reports no reset", func(t *testing.T) {
		truncate(t, pool)
		row, err := q.UpsertJob(ctx, ingestParams("acme:fresh", "Fresh"))
		if err != nil || row.ResetStrikes || !row.Inserted.Bool {
			t.Fatalf("row = %+v, %v", row, err)
		}
	})
}
