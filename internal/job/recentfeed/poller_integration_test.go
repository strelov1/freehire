//go:build integration

// Integration test for Poller.Poll against real Postgres: claiming a batch must
// actually remove the rows it claims (recent_feed_outbox is a pure transit queue,
// see migrations/0144_recent_feed_outbox.sql), and an empty outbox must publish
// nothing. Run with: go test -tags=integration ./internal/job/recentfeed/
// Requires Docker (testcontainers spins up a throwaway Postgres with the migrations).
package recentfeed

import (
	"context"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

var pollerJobSeq int

func insertPollerTestJob(t *testing.T, pool *pgxpool.Pool, title, company string) int64 {
	t.Helper()
	pollerJobSeq++
	externalID := fmt.Sprintf("recentfeed-test:%d", pollerJobSeq)
	publicSlug := fmt.Sprintf("recentfeed-test-%d", pollerJobSeq)

	var id int64
	err := pool.QueryRow(context.Background(),
		`INSERT INTO jobs (source, external_id, url, title, company, public_slug)
		 VALUES ('test', $1, 'https://example.com/job', $2, $3, $4)
		 RETURNING id`,
		externalID, title, company, publicSlug).Scan(&id)
	if err != nil {
		t.Fatalf("insert test job: %v", err)
	}
	return id
}

func outboxRowCount(t *testing.T, pool *pgxpool.Pool, jobID int64) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM recent_feed_outbox WHERE job_id = $1`, jobID).Scan(&n); err != nil {
		t.Fatalf("count outbox rows: %v", err)
	}
	return n
}

func TestPoller_PollDrainsOutboxAndPublishesEntries(t *testing.T) {
	pool := testdb.Pool(t)
	q := db.New(pool)
	ctx := context.Background()

	jobID := insertPollerTestJob(t, pool, "Senior Backend Engineer", "Acme")
	if err := q.EnqueueRecentFeedOutbox(ctx, jobID); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	b := NewBroadcaster(10)
	p := NewPoller(q, b, 100)
	if err := p.Poll(ctx); err != nil {
		t.Fatalf("Poll: %v", err)
	}

	backlog, _, cancel := b.Subscribe()
	defer cancel()
	if len(backlog) != 1 {
		t.Fatalf("backlog = %d entries, want 1: %+v", len(backlog), backlog)
	}
	if backlog[0].Title != "Senior Backend Engineer" || backlog[0].CompanyName != "Acme" {
		t.Errorf("unexpected entry: %+v", backlog[0])
	}
	if backlog[0].Kind != KindSingle {
		t.Errorf("Kind = %q, want %q for a single claimed job", backlog[0].Kind, KindSingle)
	}

	if n := outboxRowCount(t, pool, jobID); n != 0 {
		t.Errorf("outbox still holds %d row(s) for job %d after Poll, want 0 (claim must delete)", n, jobID)
	}
}

func TestPoller_PollOnEmptyOutboxPublishesNothing(t *testing.T) {
	pool := testdb.Pool(t)
	q := db.New(pool)
	ctx := context.Background()

	b := NewBroadcaster(10)
	p := NewPoller(q, b, 100)
	if err := p.Poll(ctx); err != nil {
		t.Fatalf("Poll: %v", err)
	}

	backlog, _, cancel := b.Subscribe()
	defer cancel()
	if len(backlog) != 0 {
		t.Errorf("backlog = %d entries on an empty outbox, want 0: %+v", len(backlog), backlog)
	}
}

func TestPoller_PollGroupsABurstAcrossCompanies(t *testing.T) {
	pool := testdb.Pool(t)
	q := db.New(pool)
	ctx := context.Background()

	for i := 0; i < AggregationThreshold; i++ {
		id := insertPollerTestJob(t, pool, "Senior Backend Engineer", fmt.Sprintf("Company %d", i))
		if err := q.EnqueueRecentFeedOutbox(ctx, id); err != nil {
			t.Fatalf("enqueue: %v", err)
		}
	}

	b := NewBroadcaster(10)
	p := NewPoller(q, b, 100)
	if err := p.Poll(ctx); err != nil {
		t.Fatalf("Poll: %v", err)
	}

	backlog, _, cancel := b.Subscribe()
	defer cancel()
	if len(backlog) != 1 {
		t.Fatalf("backlog = %d entries, want 1 aggregated entry: %+v", len(backlog), backlog)
	}
	if backlog[0].Kind != KindAggregate || backlog[0].Count != AggregationThreshold {
		t.Errorf("unexpected entry: %+v", backlog[0])
	}
}
