//go:build integration

// Integration tests for the board-scoped half of the unseen sweep: closing a board's stale
// postings because the board itself was crawled, rather than because the run happened to write
// something for their company. Every property here is a property of the SQL — the LIKE scope,
// the terminator that keeps a prefix-sharing board out of it, and the search_delete_outbox
// enqueue riding the same statement — so none can be checked without a real Postgres.
// Run with: go test -tags=integration ./internal/platform/db/
package db

import (
	"context"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/platform/externalid"
)

// boardJob builds an upsert for one posting of one board, with its own company. The board is
// carried the only way the catalogue carries it: as the external_id's namespace.
func boardJob(board, id, companySlug string) UpsertJobParams {
	p := ingestParams(externalid.Namespace(board, id), "Engineer "+id)
	p.Company = companySlug
	p.CompanySlug = companySlug
	return p
}

func TestCloseUnseenJobsForBoard(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	// Two postings of the board under test, belonging to DIFFERENT companies. The second is
	// the case the company scope cannot reach: its company has no other posting, so a run that
	// re-listed the board without it never writes that slug and never sweeps it.
	stale, err := ingestUpsert(ctx, q, boardJob("acme", "1", "acme"))
	if err != nil {
		t.Fatalf("upsert stale: %v", err)
	}
	orphan, err := ingestUpsert(ctx, q, boardJob("acme", "2", "acme-subsidiary"))
	if err != nil {
		t.Fatalf("upsert orphan: %v", err)
	}
	fresh, err := ingestUpsert(ctx, q, boardJob("acme", "3", "acme"))
	if err != nil {
		t.Fatalf("upsert fresh: %v", err)
	}
	// A different board of the same provider, equally stale — it was not crawled, so it must
	// survive. And a board whose id has the swept board's as a PREFIX: "acme:%" must not reach
	// "acmecorp:1", which is what the ":" terminator in BoardPattern is for.
	otherBoard, err := ingestUpsert(ctx, q, boardJob("globex", "1", "globex"))
	if err != nil {
		t.Fatalf("upsert other board: %v", err)
	}
	prefixBoard, err := ingestUpsert(ctx, q, boardJob("acmecorp", "1", "acmecorp"))
	if err != nil {
		t.Fatalf("upsert prefix board: %v", err)
	}

	for _, id := range []int64{stale.ID, orphan.ID, otherBoard.ID, prefixBoard.ID} {
		ageJob(t, pool, id, 49*time.Hour)
	}
	ageJob(t, pool, fresh.ID, 6*time.Hour)

	closed, err := q.CloseUnseenJobsForBoard(ctx, CloseUnseenJobsForBoardParams{
		Source:       "greenhouse",
		BoardPattern: externalid.BoardPattern("acme"),
		Cutoff:       pgTimestamptz(time.Now().Add(-48 * time.Hour)),
	})
	if err != nil {
		t.Fatalf("board sweep: %v", err)
	}
	if closed != 2 {
		t.Fatalf("closed %d jobs, want 2 (the board's two stale postings)", closed)
	}

	for _, tc := range []struct {
		name   string
		id     int64
		closed bool
	}{
		{"a stale posting of the crawled board", stale.ID, true},
		{"a stale posting whose company the run never wrote", orphan.ID, true},
		{"a recently seen posting of the same board", fresh.ID, false},
		{"a stale posting of a board the run did not crawl", otherBoard.ID, false},
		{"a stale posting of a board whose id merely shares a prefix", prefixBoard.ID, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			job, err := q.GetJob(ctx, tc.id)
			if err != nil {
				t.Fatalf("get: %v", err)
			}
			if job.ClosedAt.Valid != tc.closed {
				t.Errorf("closed = %v, want %v", job.ClosedAt.Valid, tc.closed)
			}
			if tc.closed && job.ClosedReason != "unseen" {
				t.Errorf("closed_reason = %q, want \"unseen\" — the board scope is the same "+
					"mechanism reaching further, not a new one", job.ClosedReason)
			}
		})
	}

	t.Run("every closed row is queued for search deletion", func(t *testing.T) {
		// The enqueue rides the UPDATE's own RETURNING so it is atomic with the close. Omitting
		// it would close the rows in Postgres and leave every one of them in the search index
		// until the next full rebuild.
		var queued int
		err := pool.QueryRow(ctx,
			`SELECT count(*) FROM search_delete_outbox WHERE job_id = ANY($1::bigint[])`,
			[]int64{stale.ID, orphan.ID}).Scan(&queued)
		if err != nil {
			t.Fatalf("count queued: %v", err)
		}
		if queued != 2 {
			t.Errorf("queued %d of the 2 closed jobs for search deletion, want 2", queued)
		}
	})

	t.Run("a second sweep closes nothing", func(t *testing.T) {
		again, err := q.CloseUnseenJobsForBoard(ctx, CloseUnseenJobsForBoardParams{
			Source:       "greenhouse",
			BoardPattern: externalid.BoardPattern("acme"),
			Cutoff:       pgTimestamptz(time.Now().Add(-48 * time.Hour)),
		})
		if err != nil {
			t.Fatalf("second sweep: %v", err)
		}
		if again != 0 {
			t.Errorf("second sweep closed %d, want 0 — the close is idempotent", again)
		}
	})
}
