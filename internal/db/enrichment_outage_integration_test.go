//go:build integration

// The regression this change exists to prevent, reproduced end to end at small scale:
// a gateway outage must not permanently remove a posting from the catalogue's
// searchable set.
//
// On 17 and 24 July 2026 the LiteLLM gateway returned 502s and 500s for days. With the
// attempt ceiling as the only bound, an entry at the head of the queue spent all three
// attempts within about fifteen minutes and was dead-lettered. 172,875 postings were
// buried that way. A dead-lettered entry is excluded from the claim query forever, its
// job never receives a category, and search.CategoryUnresolved keeps a job without a
// category out of the index — so the posting stayed in the catalogue, listed by
// GET /api/v1/jobs, and unreachable by search.
// Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"testing"
)

func TestGatewayOutageLeavesNoPostingBehind(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	// The production error text, verbatim: 128,744 entries died carrying it.
	const gateway502 = "enrich: llm: generate: API returned unexpected status code: 502"
	const (
		maxAttempts = 3
		graceDays   = 14
	)

	id := queueEntry(t, pool, q, "outage")

	// Two days of outage at roughly twelve attempts an hour — the rate an entry at the
	// head of the queue accrues once its 300-second lease keeps expiring. Far past the
	// attempt ceiling, and the point is that the ceiling is not what governs here.
	const attemptsDuringOutage = 24 * 2 * 12
	for i := 1; i <= attemptsDuringOutage; i++ {
		row, err := q.RecordEnrichmentFailure(ctx, RecordEnrichmentFailureParams{
			LastError:         gateway502,
			PostingAtFault:    false,
			MaxAttempts:       maxAttempts,
			UpstreamGraceDays: graceDays,
			ID:                id,
		})
		if err != nil {
			t.Fatalf("attempt %d: %v", i, err)
		}
		if row.FailedAt.Valid {
			t.Fatalf("dead-lettered after %d gateway failures (ceiling is %d) — this is the "+
				"July 2026 regression: the posting is enrichable and would never be indexed",
				i, maxAttempts)
		}
	}

	// The gateway recovers. The entry must still be there to enrich.
	claimed, err := q.ClaimEnrichmentBatch(ctx, ClaimEnrichmentBatchParams{LeaseSeconds: 0, BatchSize: 10})
	if err != nil {
		t.Fatalf("claim after recovery: %v", err)
	}
	if len(claimed) != 1 {
		t.Fatalf("claimable entries after the outage = %d, want 1 — the posting survived "+
			"the outage only if it can still be picked up", len(claimed))
	}
	if claimed[0].ID != id {
		t.Errorf("claimed entry = %d, want the one that failed throughout (%d)", claimed[0].ID, id)
	}

	// The same outage under the old policy, which blamed the posting for every failure.
	// Without this the test above proves only that something passes — it would pass
	// just as well against a statement that never dead-letters anything.
	t.Run("the old policy buries the same entry", func(t *testing.T) {
		old := queueEntry(t, pool, q, "outage-old-policy")

		for i := 1; i <= maxAttempts; i++ {
			row, err := q.RecordEnrichmentFailure(ctx, RecordEnrichmentFailureParams{
				LastError:      gateway502,
				PostingAtFault: true, // what the old code did for every cause
				MaxAttempts:    maxAttempts,
				// Irrelevant on this branch, which is the asymmetry being fixed.
				UpstreamGraceDays: graceDays,
				ID:                old,
			})
			if err != nil {
				t.Fatalf("attempt %d: %v", i, err)
			}
			if want := i >= maxAttempts; row.FailedAt.Valid != want {
				t.Fatalf("attempt %d: dead=%v, want %v", i, row.FailedAt.Valid, want)
			}
		}

		claimed, err := q.ClaimEnrichmentBatch(ctx, ClaimEnrichmentBatchParams{LeaseSeconds: 0, BatchSize: 10})
		if err != nil {
			t.Fatal(err)
		}
		if len(claimed) != 0 {
			t.Errorf("claimable = %d, want 0 — this branch is meant to demonstrate the loss", len(claimed))
		}
	})
}
