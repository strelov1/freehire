//go:build integration

// Integration tests for the orphan-job liveness SQL contract (openspec change
// probe-orphan-job-liveness): the two-strike close, strike reset on a healthy
// probe, and candidate selection that targets only open non-board (orphan) jobs.
// Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"testing"
	"time"
)

// orphanParams builds an UpsertJob for a non-board source (no ingest sweep ever
// re-crawls it), the population the liveness worker owns.
func orphanParams(externalID, title string) UpsertJobParams {
	p := ingestParams(externalID, title)
	p.Source = "telegram"
	return p
}

func TestLivenessClosesAfterTwoConsecutiveExpiredProbes(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	job, err := ingestUpsert(ctx, q, orphanParams("tg:1", "Orphan"))
	if err != nil {
		t.Fatalf("upsert orphan: %v", err)
	}

	// First expired probe: a strike is recorded but the job stays open.
	first, err := q.MarkLivenessExpired(ctx, MarkLivenessExpiredParams{ID: job.ID, Threshold: 2})
	if err != nil {
		t.Fatalf("first expired: %v", err)
	}
	if first.LivenessStrikes != 1 {
		t.Fatalf("first strike count = %d, want 1", first.LivenessStrikes)
	}
	if first.ClosedAt.Valid {
		t.Fatal("must not close on the first expired probe")
	}

	// Second consecutive expired probe: reaches the threshold and closes.
	second, err := q.MarkLivenessExpired(ctx, MarkLivenessExpiredParams{ID: job.ID, Threshold: 2})
	if err != nil {
		t.Fatalf("second expired: %v", err)
	}
	if second.LivenessStrikes != 2 {
		t.Fatalf("second strike count = %d, want 2", second.LivenessStrikes)
	}
	if !second.ClosedAt.Valid {
		t.Fatal("must close on the second consecutive expired probe")
	}
}

func TestLivenessHealthyProbeResetsStrikes(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	job, err := ingestUpsert(ctx, q, orphanParams("tg:1", "Orphan"))
	if err != nil {
		t.Fatalf("upsert orphan: %v", err)
	}
	if _, err := q.MarkLivenessExpired(ctx, MarkLivenessExpiredParams{ID: job.ID, Threshold: 2}); err != nil {
		t.Fatalf("expired probe: %v", err)
	}

	if err := q.ResetLivenessStrikes(ctx, job.ID); err != nil {
		t.Fatalf("reset strikes: %v", err)
	}

	got, err := q.GetJob(ctx, job.ID)
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if got.LivenessStrikes != 0 {
		t.Fatalf("strike count after reset = %d, want 0", got.LivenessStrikes)
	}
	if got.ClosedAt.Valid {
		t.Fatal("a healthy probe must leave the job open")
	}
}

// A reopened orphan job must start its liveness strike count fresh. After two
// strikes close it, re-ingesting the same posting sets closed_at = NULL and must
// also reset liveness_strikes, so a single later expired probe records strike 1 and
// leaves it open instead of immediately re-closing it — the two-strike grace
// survives a reopen.
func TestLivenessReopenResetsStrikes(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	job, err := ingestUpsert(ctx, q, orphanParams("tg:1", "Orphan"))
	if err != nil {
		t.Fatalf("upsert orphan: %v", err)
	}
	if _, err := q.MarkLivenessExpired(ctx, MarkLivenessExpiredParams{ID: job.ID, Threshold: 2}); err != nil {
		t.Fatalf("first expired: %v", err)
	}
	closed, err := q.MarkLivenessExpired(ctx, MarkLivenessExpiredParams{ID: job.ID, Threshold: 2})
	if err != nil {
		t.Fatalf("second expired: %v", err)
	}
	if !closed.ClosedAt.Valid || closed.LivenessStrikes != 2 {
		t.Fatalf("setup: want closed with 2 strikes, got closed=%v strikes=%d", closed.ClosedAt.Valid, closed.LivenessStrikes)
	}

	// Re-ingest the same posting: it reopens (closed_at = NULL) and strikes reset.
	reopened, err := ingestUpsert(ctx, q, orphanParams("tg:1", "Orphan"))
	if err != nil {
		t.Fatalf("re-ingest: %v", err)
	}
	if reopened.ClosedAt.Valid {
		t.Fatal("re-ingest must reopen the job")
	}
	if reopened.LivenessStrikes != 0 {
		t.Fatalf("reopen must reset liveness_strikes, got %d", reopened.LivenessStrikes)
	}

	// One expired probe now only records strike 1 and leaves the job open.
	after, err := q.MarkLivenessExpired(ctx, MarkLivenessExpiredParams{ID: job.ID, Threshold: 2})
	if err != nil {
		t.Fatalf("post-reopen expired: %v", err)
	}
	if after.LivenessStrikes != 1 || after.ClosedAt.Valid {
		t.Fatalf("after reopen, one probe must not re-close: strikes=%d closed=%v", after.LivenessStrikes, after.ClosedAt.Valid)
	}
}

func TestSelectOrphanLivenessCandidatesExcludesBoardAndClosed(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	orphan, err := ingestUpsert(ctx, q, orphanParams("tg:1", "Orphan"))
	if err != nil {
		t.Fatalf("upsert orphan: %v", err)
	}
	// A board (ATS) job: same shape but source = greenhouse, which the ingest sweep
	// already owns — it must not be a liveness candidate.
	if _, err := ingestUpsert(ctx, q, ingestParams("gh:1", "Board")); err != nil {
		t.Fatalf("upsert board: %v", err)
	}
	// A closed orphan: already closed, so it is not re-probed.
	closedOrphan, err := ingestUpsert(ctx, q, orphanParams("tg:2", "ClosedOrphan"))
	if err != nil {
		t.Fatalf("upsert closed orphan: %v", err)
	}
	if _, err := pool.Exec(ctx, "UPDATE jobs SET closed_at = now() WHERE id = $1", closedOrphan.ID); err != nil {
		t.Fatalf("close orphan: %v", err)
	}

	cands, err := q.SelectOrphanLivenessCandidates(ctx, []string{"greenhouse", "lever", "ashby"})
	if err != nil {
		t.Fatalf("select candidates: %v", err)
	}
	if len(cands) != 1 || cands[0].ID != orphan.ID {
		t.Fatalf("candidates must be exactly the open orphan job, got %d rows", len(cands))
	}
}

// himalayasParams builds an UpsertJob for himalayas, a registered but recency-budgeted
// aggregator whose ingest sweep can age a company out of its crawled-slug scope (see
// job-lifecycle) — the population SelectStaleRegisteredCandidates backstops.
func himalayasParams(externalID, title string) UpsertJobParams {
	p := ingestParams(externalID, title)
	p.Source = "himalayas"
	return p
}

func TestSelectStaleRegisteredCandidatesOnlyPastCutoff(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	stale, err := ingestUpsert(ctx, q, himalayasParams("hm:1", "Stale"))
	if err != nil {
		t.Fatalf("upsert stale: %v", err)
	}
	ageJob(t, pool, stale.ID, 72*time.Hour)

	// A fresh (recently seen) himalayas job must not be a candidate — only the sweep's
	// own 48h staleness window makes a job eligible for this backstop.
	if _, err := ingestUpsert(ctx, q, himalayasParams("hm:2", "Fresh")); err != nil {
		t.Fatalf("upsert fresh: %v", err)
	}

	// A stale job from a DIFFERENT source must not leak in — the sweep for other
	// providers is not the scope leak this candidate set exists to backstop.
	otherSource, err := ingestUpsert(ctx, q, ingestParams("gh:1", "OtherSource"))
	if err != nil {
		t.Fatalf("upsert other source: %v", err)
	}
	ageJob(t, pool, otherSource.ID, 72*time.Hour)

	// A stale but already-closed himalayas job must not be re-probed.
	closedStale, err := ingestUpsert(ctx, q, himalayasParams("hm:3", "ClosedStale"))
	if err != nil {
		t.Fatalf("upsert closed stale: %v", err)
	}
	ageJob(t, pool, closedStale.ID, 72*time.Hour)
	if _, err := pool.Exec(ctx, "UPDATE jobs SET closed_at = now() WHERE id = $1", closedStale.ID); err != nil {
		t.Fatalf("close job: %v", err)
	}

	cands, err := q.SelectStaleRegisteredCandidates(ctx, SelectStaleRegisteredCandidatesParams{
		Sources: []string{"himalayas"},
		Cutoff:  pgTimestamptz(time.Now().Add(-48 * time.Hour)),
	})
	if err != nil {
		t.Fatalf("select stale registered candidates: %v", err)
	}
	if len(cands) != 1 || cands[0].ID != stale.ID {
		t.Fatalf("candidates must be exactly the stale open himalayas job, got %d rows", len(cands))
	}
}
