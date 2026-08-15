package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
)

// fakeQueries answers each query from a canned value, so collect's assembly and error
// handling are testable without a database. A non-nil err field fails that one query.
type fakeQueries struct {
	search    db.SearchOutboxMetricsRow
	enrich    db.EnrichmentOutboxMetricsRow
	semantic  db.SemanticOutboxMetricsRow
	boards    db.BoardHealthMetricsRow
	newest    pgtype.Timestamptz
	newestErr error
	searchErr error
}

func (f fakeQueries) SearchOutboxMetrics(context.Context) (db.SearchOutboxMetricsRow, error) {
	return f.search, f.searchErr
}

func (f fakeQueries) EnrichmentOutboxMetrics(context.Context) (db.EnrichmentOutboxMetricsRow, error) {
	return f.enrich, nil
}

func (f fakeQueries) SemanticOutboxMetrics(context.Context) (db.SemanticOutboxMetricsRow, error) {
	return f.semantic, nil
}

func (f fakeQueries) BoardHealthMetrics(context.Context) (db.BoardHealthMetricsRow, error) {
	return f.boards, nil
}

func (f fakeQueries) NewestOpenJobCreatedAt(context.Context) (pgtype.Timestamptz, error) {
	return f.newest, f.newestErr
}

func populatedQueries() fakeQueries {
	return fakeQueries{
		search:   db.SearchOutboxMetricsRow{Depth: 3, DeadLetters: 2, OldestAgeSeconds: 21600.5},
		enrich:   db.EnrichmentOutboxMetricsRow{Depth: 1049297, DeadLetters: 41, OldestAgeSeconds: 5529600},
		semantic: db.SemanticOutboxMetricsRow{Depth: 0, DeadLetters: 0, OldestAgeSeconds: 0},
		boards:   db.BoardHealthMetricsRow{Healthy: 74894, Failing: 7002, Cooled: 1882},
		newest:   pgtype.Timestamptz{Time: time.Unix(1786821346, 0), Valid: true},
	}
}

func TestCollectAssemblesEveryQueueInOrder(t *testing.T) {
	got, err := collect(context.Background(), populatedQueries())
	if err != nil {
		t.Fatalf("collect: %v", err)
	}

	want := []queueMetrics{
		{name: "search_outbox", depth: 3, deadLetters: 2, oldestAgeSeconds: 21600.5},
		{name: "enrichment_outbox", depth: 1049297, deadLetters: 41, oldestAgeSeconds: 5529600},
		{name: "semantic_outbox", depth: 0, deadLetters: 0, oldestAgeSeconds: 0},
	}
	if len(got.queues) != len(want) {
		t.Fatalf("collected %d queues, want %d", len(got.queues), len(want))
	}
	for i := range want {
		if got.queues[i] != want[i] {
			t.Errorf("queue %d = %+v, want %+v", i, got.queues[i], want[i])
		}
	}
	if got.healthyBoards != 74894 || got.failingBoards != 7002 || got.cooledBoards != 1882 {
		t.Errorf("boards = %d/%d/%d, want 74894/7002/1882", got.healthyBoards, got.failingBoards, got.cooledBoards)
	}
	if !got.newestJob.Equal(time.Unix(1786821346, 0)) {
		t.Errorf("newestJob = %v, want %v", got.newestJob, time.Unix(1786821346, 0))
	}
}

func TestCollectTreatsAnEmptyCatalogueAsAbsentNotAsAFailure(t *testing.T) {
	q := populatedQueries()
	q.newest = pgtype.Timestamptz{}
	q.newestErr = pgx.ErrNoRows

	got, err := collect(context.Background(), q)

	// An empty catalogue is a fresh-install state, not an incident: the run must
	// still publish every other family rather than failing outright.
	if err != nil {
		t.Fatalf("collect on an empty catalogue: %v", err)
	}
	if !got.newestJob.IsZero() {
		t.Errorf("newestJob = %v, want the zero time so render omits the sample", got.newestJob)
	}
	if len(got.queues) != 3 {
		t.Errorf("collected %d queues, want all 3 despite the empty catalogue", len(got.queues))
	}
}

func TestCollectPropagatesAQueryFailure(t *testing.T) {
	q := populatedQueries()
	q.searchErr = errors.New("connection reset")

	_, err := collect(context.Background(), q)

	if err == nil {
		t.Fatal("collect succeeded, want the query error propagated so the run exits non-zero")
	}
	if !errors.Is(err, q.searchErr) {
		t.Errorf("collect error = %v, want it to wrap the underlying query error", err)
	}
}
