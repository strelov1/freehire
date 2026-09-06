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
// than testing each member in its own function on purpose: a further closing query added
// later is a one-line addition here, and leaving it out is the exact mistake that would put
// jobs in the index forever with nothing to notice it. Keep this list in step with the
// queries that set closed_at in internal/platform/db/queries/jobs.sql.
//
// It is a hand-written list, so it can only be as complete as its last reader: cmd/liveness'
// two closes (the age rule and the probe's own) were both absent from it until 2026-09-06,
// and both were absent from the queue for the same length of time.
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
		// The age rule (cmd/liveness). The cutoff is in the FUTURE because the fixture's
		// posted_at is NULL and its created_at is now — COALESCE(posted_at, created_at)
		// then reads as "just posted", and this statement's question is only whether the
		// row is older than what the caller passes.
		{"CloseStaleUnsignalledJobs", func(ctx context.Context, q *Queries, _ int64) error {
			_, err := q.CloseStaleUnsignalledJobs(ctx, CloseStaleUnsignalledJobsParams{
				Sources: []string{"greenhouse"},
				Cutoff:  pgTimestamptz(time.Now().Add(time.Hour)),
			})
			return err
		}},
		{"CloseStaleUnseenUnprobeableJobs", func(ctx context.Context, q *Queries, _ int64) error {
			_, err := q.CloseStaleUnseenUnprobeableJobs(ctx, CloseStaleUnseenUnprobeableJobsParams{
				Sources:    []string{"greenhouse"},
				Cutoff:     pgTimestamptz(time.Now().Add(time.Hour)),
				SeenCutoff: pgTimestamptz(time.Now().Add(-48 * time.Hour)),
			})
			return err
		}},
		// The probe's own close. Threshold 1 makes this strike the closing one; the
		// strike-only call is a separate case below, since it must queue NOTHING.
		{"MarkLivenessExpired", func(ctx context.Context, q *Queries, jobID int64) error {
			_, err := q.MarkLivenessExpired(ctx, MarkLivenessExpiredParams{ID: jobID, Threshold: 1})
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

// MarkLivenessExpired is called on EVERY expired probe, and most of those only advance a
// strike. Queuing a removal for a job that is still open would delete a live posting's
// document from the index, which no later close would put back.
func TestMarkLivenessExpiredQueuesNothingForAStrikeThatDidNotClose(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	job, err := ingestUpsert(ctx, q, ingestParams("acme:struck", "Engineer"))
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	res, err := q.MarkLivenessExpired(ctx, MarkLivenessExpiredParams{ID: job.ID, Threshold: 2})
	if err != nil {
		t.Fatalf("mark expired: %v", err)
	}
	if res.ClosedAt.Valid {
		t.Fatal("the first of two strikes closed the job, so this case proves nothing")
	}
	if got := countDeletionQueue(t, pool); got != 0 {
		t.Errorf("a strike that left the job open queued %d removals, want 0 — "+
			"that would drop a live posting out of the index", got)
	}
}

// The age rule is the only close that rests on a guess, and for a source the crawl still
// re-lists it is a guess against evidence. whatjobs leaves posted_at unset, so the 45-day
// clock runs from first ingest and every posting older than that qualifies — including the
// ones the crawl listed an hour ago. The second clock is what keeps the guess from
// contradicting the crawl; without it the posting is closed and the next crawl reopens it,
// twice a day, telling everyone who applied that it closed each time.
func TestTheAgeRuleWillNotCloseAPostingTheCrawlStillSees(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	job, err := ingestUpsert(ctx, q, ingestParams("acme:still-listed", "Engineer"))
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	// Old enough to be presumed filled, and seen by the crawl minutes ago.
	agePosting(t, pool, job.ID, 60*24*time.Hour)
	ageJob(t, pool, job.ID, 5*time.Minute)

	closed, err := q.CloseStaleUnseenUnprobeableJobs(ctx, CloseStaleUnseenUnprobeableJobsParams{
		Sources:    []string{"greenhouse"},
		Cutoff:     pgTimestamptz(time.Now().Add(-45 * 24 * time.Hour)),
		SeenCutoff: pgTimestamptz(time.Now().Add(-14 * 24 * time.Hour)),
	})
	if err != nil {
		t.Fatalf("expire: %v", err)
	}
	if closed != 0 {
		t.Errorf("closed %d postings the crawl listed 5 minutes ago, want 0", closed)
	}

	// The age-only statement — the one this path used to call — closes it, which is the
	// flap. Asserting it here keeps the two statements' difference from reading as an
	// accident of the fixture.
	ageOnly, err := q.CloseStaleUnsignalledJobs(ctx, CloseStaleUnsignalledJobsParams{
		Sources: []string{"greenhouse"},
		Cutoff:  pgTimestamptz(time.Now().Add(-45 * 24 * time.Hour)),
	})
	if err != nil {
		t.Fatalf("age-only expire: %v", err)
	}
	if ageOnly != 1 {
		t.Fatalf("the age-only rule closed %d postings, want 1 — the fixture no longer "+
			"reproduces what the second clock exists to prevent", ageOnly)
	}
}

// The other half of the same rule: a posting the crawl has genuinely stopped listing IS
// closed. Without this the test above would pass on a statement that closes nothing.
func TestTheAgeRuleClosesAPostingTheCrawlStoppedSeeing(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	job, err := ingestUpsert(ctx, q, ingestParams("acme:dropped", "Engineer"))
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	agePosting(t, pool, job.ID, 60*24*time.Hour)
	ageJob(t, pool, job.ID, 30*24*time.Hour)

	closed, err := q.CloseStaleUnseenUnprobeableJobs(ctx, CloseStaleUnseenUnprobeableJobsParams{
		Sources:    []string{"greenhouse"},
		Cutoff:     pgTimestamptz(time.Now().Add(-45 * 24 * time.Hour)),
		SeenCutoff: pgTimestamptz(time.Now().Add(-14 * 24 * time.Hour)),
	})
	if err != nil {
		t.Fatalf("expire: %v", err)
	}
	if closed != 1 {
		t.Fatalf("closed %d postings unseen for 30 days, want 1", closed)
	}
	if got := countDeletionQueue(t, pool); got != 1 {
		t.Errorf("queued %d removals for 1 closed posting, want 1", got)
	}
}

// agePosting backdates the clock the age rule reads (created_at, since these fixtures carry
// no posted_at), which ageJob deliberately leaves alone — the two clocks are the whole
// subject of the tests above.
func agePosting(t *testing.T, pool *pgxpool.Pool, id int64, ago time.Duration) {
	t.Helper()
	_, err := pool.Exec(context.Background(),
		"UPDATE jobs SET created_at = now() - $2::interval WHERE id = $1", id, ago.String())
	if err != nil {
		t.Fatalf("backdate created_at: %v", err)
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
