//go:build integration

// The two-session races of release B. An activity-only write (the cheap refresh, the ordinary
// touch, the covered refresh) waits for the activity row when a writer holds it, then runs on the
// LATEST version of that row but against the SNAPSHOT of jobs, so only job_activity.state_version
// can tell it that the posting changed meanwhile. Each test stages the writer in an open
// transaction, starts the statement, waits until it is blocked on the lock, commits the writer and
// reads the outcome. Nothing here sleeps: the wait is on pg_stat_activity, as in the release A tests.
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
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/platform/externalid"
)

// raceWith holds the locks hold takes in an open transaction, runs stmt on another connection until
// it is blocked on a lock, commits the writer and returns stmt's error.
func raceWith(t *testing.T, pool *pgxpool.Pool, hold func(t *testing.T, ctx context.Context, tx pgx.Tx), stmt func(ctx context.Context, q *Queries) error) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	writer, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = writer.Rollback(context.Background()) }()
	hold(t, ctx, writer)
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	done := make(chan error, 1)
	go func() { done <- stmt(ctx, New(conn)) }()
	obsWaitBlocked(t, ctx, pool, conn.Conn().PgConn().PID(), done)
	if err := writer.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return <-done
}

// seedHashed stores the stale, hashed, open posting the cheap path matches and ages its
// observation, so a refresh that DID write is visible and a withheld requeue would be queued.
func seedForRace(t *testing.T, pool *pgxpool.Pool) int64 {
	t.Helper()
	id := obsSeedStaleHashed(t, pool)
	if _, err := pool.Exec(context.Background(), `UPDATE job_activity SET last_seen_at = now() - interval '30 days' WHERE job_id = $1`, id); err != nil {
		t.Fatal(err)
	}
	return id
}

func requireUnwritten(t *testing.T, pool *pgxpool.Pool, id int64, what string) {
	t.Helper()
	if age := obsSeenAge(t, pool, id); age < 29*24*time.Hour {
		t.Errorf("%s wrote the activity row (last_seen is only %s old) although the fence rejected it", what, age)
	}
	var queued int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM search_outbox WHERE job_id = $1`, id).Scan(&queued); err != nil || queued != 0 {
		t.Errorf("%s queued %d search_outbox rows (%v) although the fence rejected it", what, queued, err)
	}
}

// What a writer does to a posting while an activity-only write waits: each is a change the fence
// must see, with the SQL the real writers use.
func fencedWriters() []struct {
	name string
	hold func(t *testing.T, ctx context.Context, tx pgx.Tx)
} {
	return []struct {
		name string
		hold func(t *testing.T, ctx context.Context, tx pgx.Tx)
	}{
		{"content edit", func(t *testing.T, ctx context.Context, tx pgx.Tx) {
			if _, err := tx.Exec(ctx, `UPDATE jobs SET content_hash = 'h2', title = 'Edited' WHERE external_id = 'acme:1'`); err != nil {
				t.Fatal(err)
			}
		}},
		{"close", func(t *testing.T, ctx context.Context, tx pgx.Tx) {
			if _, err := tx.Exec(ctx, `UPDATE jobs SET closed_at = now(), closed_reason = 'unseen' WHERE external_id = 'acme:1'`); err != nil {
				t.Fatal(err)
			}
		}},
		{"deadline", func(t *testing.T, ctx context.Context, tx pgx.Tx) {
			if _, err := tx.Exec(ctx, `UPDATE jobs SET application_deadline = current_date + 3 WHERE external_id = 'acme:1'`); err != nil {
				t.Fatal(err)
			}
		}},
		{"duplicate marker", func(t *testing.T, ctx context.Context, tx pgx.Tx) {
			if _, err := tx.Exec(ctx, `UPDATE jobs SET duplicate_of_role = id WHERE external_id = 'acme:1'`); err != nil {
				t.Fatal(err)
			}
		}},
		{"company rekey", func(t *testing.T, ctx context.Context, tx pgx.Tx) {
			if _, err := New(tx).RekeyCompanySlugChunk(ctx, RekeyCompanySlugChunkParams{AliasSlug: "acme", CanonicalSlug: "acme-inc", ChunkSize: 10}); err != nil {
				t.Fatal(err)
			}
		}},
	}
}

func TestCheapRefreshIsFencedByEveryWriterThatChangesWhatItMatched(t *testing.T) {
	for _, w := range fencedWriters() {
		t.Run(w.name, func(t *testing.T) {
			pool := startPostgres(t)
			id := seedForRace(t, pool)
			var got RefreshUnchangedJobRow
			err := raceWith(t, pool, w.hold, func(ctx context.Context, q *Queries) error {
				var err error
				got, err = q.RefreshUnchangedJob(ctx, obsRefreshParams())
				return err
			})
			if err != nil || !got.Fenced || got.ID != id {
				t.Fatalf("refresh after a %s = %+v, %v; want a fenced marker for job %d", w.name, got, err, id)
			}
			requireUnwritten(t, pool, id, "the fenced cheap refresh")
		})
	}
}

func TestOrdinaryTouchIsFencedByEveryWriterThatChangesTheLifecycle(t *testing.T) {
	cutoff := pgtype.Timestamptz{Time: time.Now().Add(-14 * 24 * time.Hour), Valid: true}
	for _, w := range fencedWriters() {
		t.Run(w.name, func(t *testing.T) {
			pool := startPostgres(t)
			id := seedForRace(t, pool)
			var got TouchOpenJobRow
			err := raceWith(t, pool, w.hold, func(ctx context.Context, q *Queries) error {
				var err error
				got, err = q.TouchOpenJob(ctx, TouchOpenJobParams{Source: "greenhouse", ExternalID: "acme:1", WithheldIfSeenBefore: cutoff})
				return err
			})
			if err != nil || !got.Fenced || got.ID != id {
				t.Fatalf("touch after a %s = %+v, %v; want a fenced marker for job %d", w.name, got, err, id)
			}
			requireUnwritten(t, pool, id, "the fenced touch")
		})
	}
}

// A pure observation, an enrichment write or a title edit are not changes of what the match key
// reads: a refresh that waited for them must still refresh.
func TestCheapRefreshIsNotFencedByWritersThatChangeNothingItMatched(t *testing.T) {
	for _, w := range []struct {
		name string
		hold func(t *testing.T, ctx context.Context, tx pgx.Tx)
	}{
		{"another observation", func(t *testing.T, ctx context.Context, tx pgx.Tx) {
			if _, err := tx.Exec(ctx, `UPDATE job_activity SET last_seen_at = now() - interval '29 days'`); err != nil {
				t.Fatal(err)
			}
		}},
		{"title and updated_at", func(t *testing.T, ctx context.Context, tx pgx.Tx) {
			if _, err := tx.Exec(ctx, `UPDATE jobs SET title = 'Retitled', updated_at = now() WHERE external_id = 'acme:1'`); err != nil {
				t.Fatal(err)
			}
			if _, err := tx.Exec(ctx, `SELECT 1 FROM job_activity WHERE job_id = (SELECT id FROM jobs WHERE external_id = 'acme:1') FOR NO KEY UPDATE`); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(w.name, func(t *testing.T) {
			pool := startPostgres(t)
			id := seedForRace(t, pool)
			var got RefreshUnchangedJobRow
			err := raceWith(t, pool, w.hold, func(ctx context.Context, q *Queries) error {
				var err error
				got, err = q.RefreshUnchangedJob(ctx, obsRefreshParams())
				return err
			})
			if err != nil || got.Fenced || got.ID != id {
				t.Fatalf("refresh = %+v, %v; want it refreshed, not fenced", got, err)
			}
			if age := obsSeenAge(t, pool, id); age > time.Minute {
				t.Errorf("last_seen is %s old, want refreshed", age)
			}
		})
	}
}

// After a fenced marker the transaction is rolled back and the statement retried from a fresh one:
// a content edit then no longer matches and falls through (pgx.ErrNoRows) to the full upsert; a
// metadata-only change still matches and the retry refreshes against the new state.
func TestFencedRefreshRetriesFromAFreshSnapshot(t *testing.T) {
	t.Run("content edit falls through to the full upsert", func(t *testing.T) {
		pool := startPostgres(t)
		seedForRace(t, pool)
		hold := fencedWriters()[0].hold
		var got RefreshUnchangedJobRow
		if err := raceWith(t, pool, hold, func(ctx context.Context, q *Queries) error {
			var err error
			got, err = q.RefreshUnchangedJob(ctx, obsRefreshParams())
			return err
		}); err != nil || !got.Fenced {
			t.Fatalf("first attempt = %+v, %v; want fenced", got, err)
		}
		if _, err := New(pool).RefreshUnchangedJob(context.Background(), obsRefreshParams()); !errors.Is(err, pgx.ErrNoRows) {
			t.Fatalf("retry after a content edit: err = %v, want pgx.ErrNoRows (the key no longer matches)", err)
		}
	})
	t.Run("a change the key does not read refreshes on the retry", func(t *testing.T) {
		pool := startPostgres(t)
		id := seedForRace(t, pool)
		hold := fencedWriters()[2].hold
		var got RefreshUnchangedJobRow
		if err := raceWith(t, pool, hold, func(ctx context.Context, q *Queries) error {
			var err error
			got, err = q.RefreshUnchangedJob(ctx, obsRefreshParams())
			return err
		}); err != nil || !got.Fenced {
			t.Fatalf("first attempt = %+v, %v; want fenced", got, err)
		}
		// The caller states no deadline, so the key still matches the moved one: the retry refreshes.
		row, err := New(pool).RefreshUnchangedJob(context.Background(), obsRefreshParams())
		if err != nil || row.Fenced {
			t.Fatalf("retry = %+v, %v; want it refreshed against the new state", row, err)
		}
		if age := obsSeenAge(t, pool, id); age > time.Minute {
			t.Errorf("last_seen is %s old after the retry, want refreshed", age)
		}
	})
}

// The fresh observation wins over a closer's stale verdict, whichever the order: observed first,
// the closer re-checks on the locked row and closes nothing; closed first, the refresh is fenced.
func TestFreshObservationBeatsAClose(t *testing.T) {
	cutoff := pgtype.Timestamptz{Time: time.Now().Add(-48 * time.Hour), Valid: true}
	t.Run("observation commits first", func(t *testing.T) {
		pool := startPostgres(t)
		id := obsSeedStaleHashed(t, pool)
		err := raceWith(t, pool, func(t *testing.T, ctx context.Context, tx pgx.Tx) {
			row, err := New(tx).RefreshUnchangedJob(ctx, obsRefreshParams())
			if err != nil || row.Fenced {
				t.Fatalf("observation = %+v, %v; want it written", row, err)
			}
		}, func(ctx context.Context, q *Queries) error {
			_, err := q.CloseUnseenJobs(ctx, CloseUnseenJobsParams{Source: "greenhouse", Cutoff: cutoff, CompanySlugs: []string{"acme"}})
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		if got := obsClosedExternalIDs(t, pool); len(got) != 0 {
			t.Fatalf("closed = %v, want nothing: the row was seen after the closer's snapshot", got)
		}
		_ = id
	})
	t.Run("close commits first", func(t *testing.T) {
		pool := startPostgres(t)
		obsSeedStaleHashed(t, pool)
		var got RefreshUnchangedJobRow
		err := raceWith(t, pool, func(t *testing.T, ctx context.Context, tx pgx.Tx) {
			if _, err := New(tx).CloseUnseenJobsForBoards(ctx, CloseUnseenJobsForBoardsParams{
				Source: "greenhouse", Cutoff: cutoff, BoardPatterns: []string{externalid.BoardPattern("acme")},
			}); err != nil {
				t.Fatal(err)
			}
		}, func(ctx context.Context, q *Queries) error {
			var err error
			got, err = q.RefreshUnchangedJob(ctx, obsRefreshParams())
			return err
		})
		if err != nil || !got.Fenced {
			t.Fatalf("refresh = %+v, %v; want fenced by the close", got, err)
		}
	})
}
