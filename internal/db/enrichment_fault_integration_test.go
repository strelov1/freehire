//go:build integration

// Integration tests for the split dead-letter policy: an entry is buried on the attempt
// counter only when the posting itself caused the failure. Every other failure — a
// gateway error, a timeout, an unreachable database — is bounded by how long the entry
// has been queued instead, because an attempt counter does not measure outage duration.
//
// This is the regression behind 172,875 permanently unsearchable postings: two LiteLLM
// outages in July 2026 spent the three-attempt budget of every entry at the head of the
// queue within about fifteen minutes each.
// Run with: go test -tags=integration ./internal/db/
package db

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// queueEntry seeds one claimable outbox entry and returns its id.
func queueEntry(t *testing.T, pool *pgxpool.Pool, q *Queries, externalID string) int64 {
	t.Helper()
	ctx := context.Background()
	truncate(t, pool)
	insertJob(t, pool, externalID)
	if _, err := q.EnqueuePendingJobs(ctx, targetVersion); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	claimed, err := q.ClaimEnrichmentBatch(ctx, ClaimEnrichmentBatchParams{LeaseSeconds: 3600, BatchSize: 10})
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim: rows=%d err=%v, want 1", len(claimed), err)
	}
	return claimed[0].ID
}

// ageEntry backdates an entry's queue age so the grace window can be crossed without
// waiting two weeks.
func ageEntry(t *testing.T, pool *pgxpool.Pool, id int64, days int) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`UPDATE enrichment_outbox SET created_at = now() - make_interval(days => $2) WHERE id = $1`,
		id, days); err != nil {
		t.Fatalf("age entry: %v", err)
	}
}

func TestDeadLetterPolicySplitsByFault(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	const graceDays = 14

	t.Run("the posting's own fault dead-letters on the attempt ceiling", func(t *testing.T) {
		id := queueEntry(t, pool, q, "unparseable")

		for i := 1; i <= 3; i++ {
			row, err := q.RecordEnrichmentFailure(ctx, RecordEnrichmentFailureParams{
				LastError:         "enrich: unparseable model response: json: cannot unmarshal bool",
				MaxAttempts:       3,
				PostingAtFault:    true,
				UpstreamGraceDays: graceDays,
				ID:                id,
			})
			if err != nil {
				t.Fatalf("failure %d: %v", i, err)
			}
			if want := i >= 3; row.FailedAt.Valid != want {
				t.Errorf("after attempt %d: dead=%v, want %v", i, row.FailedAt.Valid, want)
			}
		}
	})

	// The regression. Before this policy, three of these buried the entry.
	t.Run("a gateway outage does not dead-letter, however many attempts", func(t *testing.T) {
		id := queueEntry(t, pool, q, "gateway")

		for i := 1; i <= 10; i++ {
			row, err := q.RecordEnrichmentFailure(ctx, RecordEnrichmentFailureParams{
				LastError:         "enrich: llm: generate: API returned unexpected status code: 502",
				MaxAttempts:       3,
				PostingAtFault:    false,
				UpstreamGraceDays: graceDays,
				ID:                id,
			})
			if err != nil {
				t.Fatalf("failure %d: %v", i, err)
			}
			if row.FailedAt.Valid {
				t.Fatalf("dead-lettered after %d gateway failures — an outage must cost nothing "+
					"permanently while the entry is inside its grace window", i)
			}
		}

		// Still claimable, which is the point: the entry survives to be enriched
		// once the gateway recovers.
		claimed, err := q.ClaimEnrichmentBatch(ctx, ClaimEnrichmentBatchParams{LeaseSeconds: 0, BatchSize: 10})
		if err != nil || len(claimed) != 1 {
			t.Errorf("claim after the outage: rows=%d err=%v, want 1", len(claimed), err)
		}
	})

	t.Run("past the grace window our own fault does dead-letter", func(t *testing.T) {
		id := queueEntry(t, pool, q, "forever")
		ageEntry(t, pool, id, graceDays+1)

		row, err := q.RecordEnrichmentFailure(ctx, RecordEnrichmentFailureParams{
			LastError:         "enrich: llm: generate: API returned unexpected status code: 502",
			MaxAttempts:       3,
			PostingAtFault:    false,
			UpstreamGraceDays: graceDays,
			ID:                id,
		})
		if err != nil {
			t.Fatal(err)
		}
		if !row.FailedAt.Valid {
			t.Error("not dead-lettered past the grace window — an entry nothing can serve " +
				"must stop eventually")
		}
	})

	t.Run("inside the grace window age alone does not bury an entry", func(t *testing.T) {
		id := queueEntry(t, pool, q, "young")
		ageEntry(t, pool, id, graceDays-1)

		row, err := q.RecordEnrichmentFailure(ctx, RecordEnrichmentFailureParams{
			LastError:         "enrich: llm: generate: API returned unexpected status code: 500",
			MaxAttempts:       3,
			PostingAtFault:    false,
			UpstreamGraceDays: graceDays,
			ID:                id,
		})
		if err != nil {
			t.Fatal(err)
		}
		if row.FailedAt.Valid {
			t.Errorf("dead-lettered at %d days, one day inside the %d-day window", graceDays-1, graceDays)
		}
	})

	// The zero value must be the safe one. Read as arithmetic, a window of 0 makes
	// `created_at < now() - 0` true for every row, so a caller that forgot to set it
	// would bury every entry on its first failure — the exact bug this policy exists to
	// fix, reintroduced through a default.
	t.Run("a non-positive grace window never buries on age", func(t *testing.T) {
		id := queueEntry(t, pool, q, "unconfigured")
		ageEntry(t, pool, id, 400)

		for _, grace := range []int32{0, -1} {
			row, err := q.RecordEnrichmentFailure(ctx, RecordEnrichmentFailureParams{
				LastError:         "enrich: llm: generate: API returned unexpected status code: 502",
				MaxAttempts:       3,
				PostingAtFault:    false,
				UpstreamGraceDays: grace,
				ID:                id,
			})
			if err != nil {
				t.Fatalf("grace=%d: %v", grace, err)
			}
			if row.FailedAt.Valid {
				t.Errorf("grace=%d dead-lettered a 400-day-old entry — a misconfigured window "+
					"must cost retries, not postings", grace)
			}
		}
	})

	// A posting's own fault is bounded by attempts alone: an old entry that has failed
	// once must not be buried for its age.
	t.Run("age does not shortcut the attempt ceiling for the posting's fault", func(t *testing.T) {
		id := queueEntry(t, pool, q, "old-but-fresh-failure")
		ageEntry(t, pool, id, graceDays*3)

		row, err := q.RecordEnrichmentFailure(ctx, RecordEnrichmentFailureParams{
			LastError:         "enrich: unparseable model response: json: cannot unmarshal bool",
			MaxAttempts:       3,
			PostingAtFault:    true,
			UpstreamGraceDays: graceDays,
			ID:                id,
		})
		if err != nil {
			t.Fatal(err)
		}
		if row.FailedAt.Valid {
			t.Error("dead-lettered on its first attempt because the entry is old — the two " +
				"bounds are separate, and age is not one of the posting's attempts")
		}
	})
}
