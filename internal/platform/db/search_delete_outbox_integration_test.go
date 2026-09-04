//go:build integration

// Integration tests for search_delete_outbox, the queue that removes a closed job's
// document from the facet index.
//
// The load-bearing property here is the one search_outbox does NOT have: an entry must
// outlive the job row it names. cmd/prune hard-deletes jobs by id list with no closed_at
// condition, so mirroring search_outbox's ON DELETE CASCADE would delete a pending removal
// exactly when the job disappeared — stranding that document in the index with nothing left
// in the database that knows it should not be there. Verifiable only against real Postgres.
// Run with: go test -tags=integration ./internal/platform/db/
package db

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/platform/externalid"
)

// enqueueDeletion queues a removal directly, standing in for the closing statements until
// they carry the CTE themselves.
func enqueueDeletion(t *testing.T, pool *pgxpool.Pool, jobID int64) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		`INSERT INTO search_delete_outbox (job_id) VALUES ($1) ON CONFLICT (job_id) DO NOTHING`, jobID)
	if err != nil {
		t.Fatalf("enqueue deletion for %d: %v", jobID, err)
	}
}

func countDeletionQueue(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM search_delete_outbox`).Scan(&n); err != nil {
		t.Fatalf("count deletion queue: %v", err)
	}
	return n
}

// The regression this whole table's shape exists to prevent.
func TestSearchDeleteOutboxSurvivesItsJobBeingDeleted(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	job, err := ingestUpsert(ctx, q, ingestParams("acme:pruned", "Pruned"))
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	enqueueDeletion(t, pool, job.ID)

	if _, err := pool.Exec(ctx, `DELETE FROM jobs WHERE id = $1`, job.ID); err != nil {
		t.Fatalf("hard-delete the job: %v", err)
	}

	if got := countDeletionQueue(t, pool); got != 1 {
		t.Fatalf("deletion queue holds %d rows after the job was deleted, want 1 — "+
			"a foreign key with ON DELETE CASCADE would strand this document in the index", got)
	}
}

// The sweep closes every posting a crawl no longer saw in ONE statement, so the enqueue has
// to ride that statement rather than being a call per row.
func TestCloseUnseenJobsQueuesEveryJobItClosed(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	const want = 3
	for i := 0; i < want; i++ {
		job, err := ingestUpsert(ctx, q, ingestParams(fmt.Sprintf("acme:sweep-%d", i), "Engineer"))
		if err != nil {
			t.Fatalf("upsert %d: %v", i, err)
		}
		ageJob(t, pool, job.ID, 72*time.Hour)
	}

	closed, err := q.CloseUnseenJobs(ctx, CloseUnseenJobsParams{
		Source:       "greenhouse",
		Cutoff:       pgTimestamptz(time.Now().Add(-48 * time.Hour)),
		CompanySlugs: []string{"acme"},
	})
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if closed != want {
		t.Fatalf("sweep closed %d jobs, want %d", closed, want)
	}
	if got := countDeletionQueue(t, pool); got != want {
		t.Errorf("sweep queued %d removals for %d closed jobs — every closed job must be queued", got, want)
	}
}

// A job the sweep did not close must not be queued: the enqueue rides the UPDATE's RETURNING,
// so it can only ever see rows that actually closed.
func TestCloseUnseenJobsQueuesNothingForJobsItLeftOpen(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	if _, err := ingestUpsert(ctx, q, ingestParams("acme:fresh", "Engineer")); err != nil {
		t.Fatalf("upsert: %v", err)
	}

	closed, err := q.CloseUnseenJobs(ctx, CloseUnseenJobsParams{
		Source:       "greenhouse",
		Cutoff:       pgTimestamptz(time.Now().Add(-48 * time.Hour)),
		CompanySlugs: []string{"acme"},
	})
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if closed != 0 {
		t.Fatalf("sweep closed %d jobs, want 0", closed)
	}
	if got := countDeletionQueue(t, pool); got != 0 {
		t.Errorf("sweep queued %d removals having closed nothing, want 0", got)
	}
}

// Every way a job can be closed must queue its removal. This enumerates the family rather
// than testing each member in its own function on purpose: a sixth closing query added later
// is a one-line addition here, and leaving it out is the exact mistake that would put jobs in
// the index forever with nothing to notice it. Keep this list in step with the queries that
// set closed_at in internal/platform/db/queries/jobs.sql.
func TestEveryClosingQueryQueuesTheRemoval(t *testing.T) {
	closers := []struct {
		name  string
		close func(ctx context.Context, q *Queries, jobID int64) error
	}{
		{"CloseUnseenJobs", func(ctx context.Context, q *Queries, _ int64) error {
			_, err := q.CloseUnseenJobs(ctx, CloseUnseenJobsParams{
				Source:       "greenhouse",
				Cutoff:       pgTimestamptz(time.Now().Add(-48 * time.Hour)),
				CompanySlugs: []string{"acme"},
			})
			return err
		}},
		{"CloseUnseenJobsBySource", func(ctx context.Context, q *Queries, _ int64) error {
			_, err := q.CloseUnseenJobsBySource(ctx, CloseUnseenJobsBySourceParams{
				Source: "greenhouse",
				Cutoff: pgTimestamptz(time.Now().Add(-48 * time.Hour)),
			})
			return err
		}},
		{"CloseUnseenJobsForBoard", func(ctx context.Context, q *Queries, _ int64) error {
			_, err := q.CloseUnseenJobsForBoard(ctx, CloseUnseenJobsForBoardParams{
				Source:       "greenhouse",
				Cutoff:       pgTimestamptz(time.Now().Add(-48 * time.Hour)),
				BoardPattern: externalid.BoardPattern("acme"),
			})
			return err
		}},
		{"CloseUnseenJobByID", func(ctx context.Context, q *Queries, jobID int64) error {
			_, err := q.CloseUnseenJobByID(ctx, jobID)
			return err
		}},
		{"CloseJobByID", func(ctx context.Context, q *Queries, jobID int64) error {
			_, err := q.CloseJobByID(ctx, jobID)
			return err
		}},
		{"CloseJobBySourceExternalID", func(ctx context.Context, q *Queries, _ int64) error {
			_, err := q.CloseJobBySourceExternalID(ctx, CloseJobBySourceExternalIDParams{
				Source:     "greenhouse",
				ExternalID: "acme:closer",
			})
			return err
		}},
	}

	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	for _, c := range closers {
		t.Run(c.name, func(t *testing.T) {
			truncate(t, pool)

			job, err := ingestUpsert(ctx, q, ingestParams("acme:closer", "Engineer"))
			if err != nil {
				t.Fatalf("upsert: %v", err)
			}
			ageJob(t, pool, job.ID, 72*time.Hour)

			if err := c.close(ctx, q, job.ID); err != nil {
				t.Fatalf("close: %v", err)
			}

			var closed bool
			if err := pool.QueryRow(ctx,
				`SELECT closed_at IS NOT NULL FROM jobs WHERE id = $1`, job.ID).Scan(&closed); err != nil {
				t.Fatalf("read closed_at: %v", err)
			}
			if !closed {
				t.Fatalf("%s did not close the job, so this case proves nothing", c.name)
			}
			if got := countDeletionQueue(t, pool); got != 1 {
				t.Errorf("%s closed a job and queued %d removals, want 1 — "+
					"a closing path that skips the queue leaves that document in the index forever", c.name, got)
			}
		})
	}
}

// The enqueue is a CTE inside the closing statement, so it cannot outlive a transaction the
// close did not survive. This pins that: the whole point of not using a watermark scan is
// that the queue and the close commit together or not at all.
func TestARolledBackCloseQueuesNothing(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	truncate(t, pool)

	job, err := ingestUpsert(ctx, New(pool), ingestParams("acme:rolled-back", "Engineer"))
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, err := New(tx).CloseJobByID(ctx, job.ID); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("close in transaction: %v", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatalf("rollback: %v", err)
	}

	var closed bool
	if err := pool.QueryRow(ctx,
		`SELECT closed_at IS NOT NULL FROM jobs WHERE id = $1`, job.ID).Scan(&closed); err != nil {
		t.Fatalf("read closed_at: %v", err)
	}
	if closed {
		t.Fatal("the job stayed closed after a rollback, so this case proves nothing")
	}
	if got := countDeletionQueue(t, pool); got != 0 {
		t.Errorf("a rolled-back close left %d queued removals, want 0", got)
	}
}

// cmd/prune is the only hard-delete path, and it deletes by id list with no closed_at
// condition — so it can remove an OPEN, indexed job outright. That document has to leave the
// index too, and the queue row has to survive the row it names disappearing.
func TestPruneJobsQueuesTheRemovalAndOutlivesTheJob(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	job, err := ingestUpsert(ctx, q, ingestParams("acme:pruned-open", "Engineer"))
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	pruned, err := q.PruneJobs(ctx, PruneJobsParams{
		Ids:   []int64{job.ID},
		Rules: []string{"test"},
	})
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if len(pruned) != 1 {
		t.Fatalf("pruned %d jobs, want 1", len(pruned))
	}

	var stillThere bool
	if err := pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM jobs WHERE id = $1)`, job.ID).Scan(&stillThere); err != nil {
		t.Fatalf("check the job row: %v", err)
	}
	if stillThere {
		t.Fatal("prune left the job row behind, so this case proves nothing")
	}

	if got := countDeletionQueue(t, pool); got != 1 {
		t.Errorf("prune queued %d removals for a hard-deleted job, want 1 — "+
			"its document would otherwise stay in the index with nothing left to notice", got)
	}
}

func TestClaimSearchDeleteOutboxBatchReturnsQueuedJobIDs(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	first, err := ingestUpsert(ctx, q, ingestParams("acme:one", "One"))
	if err != nil {
		t.Fatalf("upsert one: %v", err)
	}
	second, err := ingestUpsert(ctx, q, ingestParams("acme:two", "Two"))
	if err != nil {
		t.Fatalf("upsert two: %v", err)
	}
	enqueueDeletion(t, pool, first.ID)
	enqueueDeletion(t, pool, second.ID)

	claimed, err := q.ClaimSearchDeleteOutboxBatch(ctx, ClaimSearchDeleteOutboxBatchParams{
		LeaseSeconds: 300,
		BatchSize:    10,
	})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if len(claimed) != 2 {
		t.Fatalf("claimed %d entries, want 2", len(claimed))
	}
}

// A claimed entry is leased, so a second worker running concurrently takes nothing.
func TestClaimSearchDeleteOutboxBatchLeasesWhatItTakes(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	job, err := ingestUpsert(ctx, q, ingestParams("acme:leased", "Leased"))
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	enqueueDeletion(t, pool, job.ID)

	args := ClaimSearchDeleteOutboxBatchParams{LeaseSeconds: 300, BatchSize: 10}
	if _, err := q.ClaimSearchDeleteOutboxBatch(ctx, args); err != nil {
		t.Fatalf("first claim: %v", err)
	}
	again, err := q.ClaimSearchDeleteOutboxBatch(ctx, args)
	if err != nil {
		t.Fatalf("second claim: %v", err)
	}
	if len(again) != 0 {
		t.Errorf("second claim took %d leased entries, want 0", len(again))
	}
}

func TestCompleteSearchDeleteOutboxRemovesTheEntries(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	job, err := ingestUpsert(ctx, q, ingestParams("acme:done", "Done"))
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	enqueueDeletion(t, pool, job.ID)

	claimed, err := q.ClaimSearchDeleteOutboxBatch(ctx, ClaimSearchDeleteOutboxBatchParams{
		LeaseSeconds: 300,
		BatchSize:    10,
	})
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	ids := make([]int64, 0, len(claimed))
	for _, c := range claimed {
		ids = append(ids, c.ID)
	}
	if err := q.CompleteSearchDeleteOutbox(ctx, ids); err != nil {
		t.Fatalf("complete: %v", err)
	}

	if got := countDeletionQueue(t, pool); got != 0 {
		t.Errorf("deletion queue holds %d rows after completion, want 0", got)
	}
}
