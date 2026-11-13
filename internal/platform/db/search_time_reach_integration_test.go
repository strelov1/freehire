//go:build integration

// Integration tests for the writes that used to reach the search index only at the next
// full rebuild: a duplicate's change re-queuing its canonical owner (migration 0240), a
// withheld-as-unseen posting re-queued when its source lists it again, the time-crossings
// pass, and the view-count rollup. Reuses startPostgres/truncate and insertChunkTestJob.
package db

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// searchOutboxCount counts a job's search_outbox rows.
func searchOutboxCount(t *testing.T, pool *pgxpool.Pool, jobID int64) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		"SELECT count(*) FROM search_outbox WHERE job_id = $1", jobID).Scan(&n); err != nil {
		t.Fatalf("count search_outbox for job %d: %v", jobID, err)
	}
	return n
}

func TestSearchReachWithoutRebuild(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	reset := func(t *testing.T) {
		t.Helper()
		truncate(t, pool)
		clearOutbox(t, pool)
	}
	exec := func(t *testing.T, sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatalf("%s: %v", sql, err)
		}
	}
	ts := func(d time.Duration) pgtype.Timestamptz {
		return pgtype.Timestamptz{Time: time.Now().Add(d), Valid: true}
	}

	t.Run("a duplicate's change queues its canonical owner", func(t *testing.T) {
		for _, tc := range []struct {
			name, change string
			want         int
		}{
			{"closes", "UPDATE jobs SET closed_at = now() WHERE id = $1", 1},
			{"geography moves", "UPDATE jobs SET countries = '{DE}' WHERE id = $1", 1},
			{"leaves the cluster", "UPDATE jobs SET duplicate_of_role = NULL WHERE id = $1", 1},
			{"unrelated write", "UPDATE jobs SET title = 'Renamed' WHERE id = $1", 0},
		} {
			t.Run(tc.name, func(t *testing.T) {
				reset(t)
				owner := insertChunkTestJob(t, pool, "owner", "acme")
				dup := insertChunkTestJob(t, pool, "dup", "acme")
				exec(t, "UPDATE jobs SET duplicate_of_role = $1 WHERE id = $2", owner, dup)
				clearOutbox(t, pool)

				exec(t, tc.change, dup)
				if got := searchOutboxCount(t, pool, owner); got != tc.want {
					t.Errorf("owner queued %d times, want %d", got, tc.want)
				}
			})
		}
	})

	t.Run("a chain queues the canonical root, not the middle", func(t *testing.T) {
		reset(t)
		root := insertChunkTestJob(t, pool, "root", "acme")
		mid := insertChunkTestJob(t, pool, "mid", "acme")
		leaf := insertChunkTestJob(t, pool, "leaf", "acme")
		exec(t, "UPDATE jobs SET duplicate_of_role = $1 WHERE id = $2", root, mid)
		exec(t, "UPDATE jobs SET duplicate_of_role = $1 WHERE id = $2", mid, leaf)
		clearOutbox(t, pool)

		exec(t, "UPDATE jobs SET closed_at = now() WHERE id = $1", leaf)
		if got := searchOutboxCount(t, pool, root); got != 1 {
			t.Errorf("root queued %d times, want 1", got)
		}
		if got := searchOutboxCount(t, pool, mid); got != 0 {
			t.Errorf("non-canonical middle queued %d times, want 0", got)
		}
	})

	t.Run("a posting re-seen after going unseen is queued back", func(t *testing.T) {
		for _, tc := range []struct {
			name     string
			lastSeen time.Duration
			want     int
		}{
			{"was withheld", -20 * 24 * time.Hour, 1},
			{"was still shown", -24 * time.Hour, 0},
		} {
			t.Run(tc.name, func(t *testing.T) {
				reset(t)
				id := insertChunkTestJob(t, pool, "seen", "acme")
				exec(t, "UPDATE jobs SET last_seen_at = $1 WHERE id = $2", ts(tc.lastSeen), id)
				// job_activity is what readers compare and the bridge never lowers it, so age it directly.
				exec(t, "UPDATE job_activity SET last_seen_at = $1 WHERE job_id = $2", ts(tc.lastSeen), id)
				clearOutbox(t, pool)

				if _, err := q.TouchJob(ctx, TouchJobParams{
					Source: "test", ExternalID: "seen", WithheldIfSeenBefore: ts(-14 * 24 * time.Hour),
				}); err != nil {
					t.Fatalf("touch: %v", err)
				}
				if got := searchOutboxCount(t, pool, id); got != tc.want {
					t.Errorf("TouchJob queued %d, want %d", got, tc.want)
				}

				// The cheap re-crawl write: same rule. The match key is the row's own state.
				exec(t, "UPDATE jobs SET last_seen_at = $1, content_hash = 'h' WHERE id = $2", ts(tc.lastSeen), id)
				exec(t, "UPDATE job_activity SET last_seen_at = $1 WHERE job_id = $2", ts(tc.lastSeen), id)
				clearOutbox(t, pool)
				if _, err := q.RefreshUnchangedJob(ctx, RefreshUnchangedJobParams{
					Source: "test", ExternalID: "seen", ContentHash: pgtype.Text{String: "h", Valid: true},
					WithheldIfSeenBefore: ts(-14 * 24 * time.Hour),
				}); err != nil {
					t.Fatalf("refresh: %v", err)
				}
				if got := searchOutboxCount(t, pool, id); got != tc.want {
					t.Errorf("RefreshUnchangedJob queued %d, want %d", got, tc.want)
				}
			})
		}
	})

	// The drain completes a wave by deleting its claimed entries BY ID. A re-seen posting whose
	// entry a wave already holds must survive that, or the wave that is removing its document
	// leaves nothing behind to restore it.
	t.Run("a re-seen posting survives the completion of a wave holding its entry", func(t *testing.T) {
		reset(t)
		id := insertChunkTestJob(t, pool, "held", "acme")
		exec(t, "UPDATE jobs SET last_seen_at = $1 WHERE id = $2", ts(-20*24*time.Hour), id)
		exec(t, "UPDATE job_activity SET last_seen_at = $1 WHERE job_id = $2", ts(-20*24*time.Hour), id)
		clearOutbox(t, pool)
		exec(t, "INSERT INTO search_outbox (job_id, job_posted_at, claimed_at) VALUES ($1, now(), now())", id)
		var heldEntry int64
		if err := pool.QueryRow(ctx, "SELECT id FROM search_outbox WHERE job_id = $1", id).Scan(&heldEntry); err != nil {
			t.Fatal(err)
		}

		if _, err := q.TouchJob(ctx, TouchJobParams{
			Source: "test", ExternalID: "held", WithheldIfSeenBefore: ts(-14 * 24 * time.Hour),
		}); err != nil {
			t.Fatalf("touch: %v", err)
		}
		if err := q.DeleteSearchOutboxEntries(ctx, []int64{heldEntry}); err != nil {
			t.Fatalf("complete wave: %v", err)
		}
		if got := searchOutboxCount(t, pool, id); got != 1 {
			t.Errorf("entries left after the wave completed = %d, want 1", got)
		}
	})

	t.Run("the time-crossings pass queues exactly the postings that crossed", func(t *testing.T) {
		reset(t)
		crossedAge := insertChunkTestJob(t, pool, "aged", "acme")
		young := insertChunkTestJob(t, pool, "young", "acme")
		wentUnseen := insertChunkTestJob(t, pool, "unseen", "acme")
		otherSource := insertChunkTestJob(t, pool, "other", "acme")
		// Re-posted long after it was first seen: fake freshness ends 15 days after posted_at.
		repostedFresh := insertChunkTestJob(t, pool, "reposted", "acme")
		exec(t, "UPDATE jobs SET created_at = $1, last_seen_at = now() WHERE id = $2", ts(-15*24*time.Hour-time.Hour), crossedAge)
		exec(t, "UPDATE jobs SET created_at = $1, last_seen_at = now() WHERE id = $2", ts(-10*24*time.Hour), young)
		exec(t, "UPDATE jobs SET created_at = $1, last_seen_at = $1 WHERE id = $2", ts(-14*24*time.Hour-time.Hour), wentUnseen)
		exec(t, "UPDATE job_activity SET last_seen_at = $1 WHERE job_id = $2", ts(-14*24*time.Hour-time.Hour), wentUnseen)
		exec(t, "UPDATE jobs SET source = 'nowindow', created_at = $1, last_seen_at = $1 WHERE id = $2", ts(-14*24*time.Hour-time.Hour), otherSource)
		exec(t, "UPDATE job_activity SET last_seen_at = $1 WHERE job_id = $2", ts(-14*24*time.Hour-time.Hour), otherSource)
		exec(t, "UPDATE jobs SET created_at = $1, posted_at = $2, last_seen_at = now() WHERE id = $3",
			ts(-100*24*time.Hour), ts(-15*24*time.Hour-time.Hour), repostedFresh)
		clearOutbox(t, pool)

		n, err := q.EnqueueSearchTimeCrossings(ctx, EnqueueSearchTimeCrossingsParams{
			Sources: []string{"test"}, UnseenSeconds: []int64{14 * 24 * 3600},
			AgeDays: []int32{15, 90}, PostedAgeDays: []int32{15},
			Since: ts(-2 * time.Hour), Until: ts(0),
		})
		if err != nil {
			t.Fatalf("enqueue: %v", err)
		}
		if n != 3 {
			t.Errorf("queued %d postings, want 3", n)
		}
		for id, want := range map[int64]int{crossedAge: 1, young: 0, wentUnseen: 1, otherSource: 0, repostedFresh: 1} {
			if got := searchOutboxCount(t, pool, id); got != want {
				t.Errorf("job %d queued %d, want %d", id, got, want)
			}
		}
	})

	// What cmd/reindex's overlap exists for: a crossing skipped because a drain wave held the
	// job's entry is queued by the next run, whose window reaches back over it.
	t.Run("a crossing skipped under an in-flight wave is caught by the overlapping next run", func(t *testing.T) {
		reset(t)
		id := insertChunkTestJob(t, pool, "held-aged", "acme")
		exec(t, "UPDATE jobs SET created_at = $1, last_seen_at = now() WHERE id = $2", ts(-15*24*time.Hour-10*time.Minute), id)
		clearOutbox(t, pool)
		exec(t, "INSERT INTO search_outbox (job_id, job_posted_at, claimed_at) VALUES ($1, now(), now())", id)
		var held int64
		if err := pool.QueryRow(ctx, "SELECT id FROM search_outbox WHERE job_id = $1", id).Scan(&held); err != nil {
			t.Fatal(err)
		}
		run := func(since, until time.Duration) {
			t.Helper()
			if _, err := q.EnqueueSearchTimeCrossings(ctx, EnqueueSearchTimeCrossingsParams{
				Sources: []string{"test"}, UnseenSeconds: []int64{14 * 24 * 3600},
				AgeDays: []int32{15, 90}, PostedAgeDays: []int32{15},
				Since: ts(since), Until: ts(until),
			}); err != nil {
				t.Fatalf("enqueue: %v", err)
			}
		}
		run(-time.Hour, -time.Minute) // covers the crossing, meets the held entry
		if err := q.DeleteSearchOutboxEntries(ctx, []int64{held}); err != nil {
			t.Fatal(err)
		}
		if got := searchOutboxCount(t, pool, id); got != 0 {
			t.Fatalf("entries left once the wave completed = %d, want 0: the first run must not have re-stamped the held entry", got)
		}
		run(-time.Minute-time.Hour, 0) // the next run: its cursor minus the overlap
		if got := searchOutboxCount(t, pool, id); got != 1 {
			t.Errorf("crossing queued %d times after the overlapping run, want 1", got)
		}
	})

	t.Run("a view-count rollup queues the job it counted", func(t *testing.T) {
		reset(t)
		id := insertChunkTestJob(t, pool, "viewed", "acme")
		clearOutbox(t, pool)

		day := pgtype.Date{Time: time.Now(), Valid: true}
		q.ApplyDailyView(ctx, []ApplyDailyViewParams{{Day: day, JobID: id, TotalDelta: 2, PageDelta: 1}}).Exec(func(_ int, err error) {
			if err != nil {
				t.Fatalf("apply view: %v", err)
			}
		})
		if got := searchOutboxCount(t, pool, id); got != 1 {
			t.Errorf("job queued %d after a view, want 1", got)
		}

		private := insertChunkTestJob(t, pool, "private", "acme")
		exec(t, "UPDATE jobs SET is_private = true WHERE id = $1", private)
		q.ApplyDailyAgentView(ctx, []ApplyDailyAgentViewParams{{Day: day, JobID: private, Delta: 1}}).Exec(func(_ int, err error) {
			if err != nil {
				t.Fatalf("apply agent view: %v", err)
			}
		})
		if got := searchOutboxCount(t, pool, private); got != 0 {
			t.Errorf("private job queued %d after a view, want 0", got)
		}
	})
}

func clearOutbox(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), "TRUNCATE search_outbox"); err != nil {
		t.Fatalf("truncate search_outbox: %v", err)
	}
}
