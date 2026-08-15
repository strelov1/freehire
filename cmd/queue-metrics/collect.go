package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
)

// metricsQueries is the slice of *db.Queries this worker needs, declared here so the
// assembly and its error handling are testable against a fake rather than a container.
// Narrow by intent, matching internal/worker's FullScanQueries.
type metricsQueries interface {
	SearchOutboxMetrics(context.Context) (db.SearchOutboxMetricsRow, error)
	EnrichmentOutboxMetrics(context.Context) (db.EnrichmentOutboxMetricsRow, error)
	SemanticOutboxMetrics(context.Context) (db.SemanticOutboxMetricsRow, error)
	BoardHealthMetrics(context.Context) (db.BoardHealthMetricsRow, error)
	NewestOpenJobCreatedAt(context.Context) (pgtype.Timestamptz, error)
}

// collect runs one measurement pass. Any query failure aborts the pass: a partial
// exposition would publish some families and silently drop others, and a dropped family
// reads downstream as a dead exporter — a louder and more misleading signal than the one
// failed run this returns.
func collect(ctx context.Context, q metricsQueries) (snapshot, error) {
	search, err := q.SearchOutboxMetrics(ctx)
	if err != nil {
		return snapshot{}, fmt.Errorf("search outbox metrics: %w", err)
	}
	enrichment, err := q.EnrichmentOutboxMetrics(ctx)
	if err != nil {
		return snapshot{}, fmt.Errorf("enrichment outbox metrics: %w", err)
	}
	semantic, err := q.SemanticOutboxMetrics(ctx)
	if err != nil {
		return snapshot{}, fmt.Errorf("semantic outbox metrics: %w", err)
	}
	boards, err := q.BoardHealthMetrics(ctx)
	if err != nil {
		return snapshot{}, fmt.Errorf("board health metrics: %w", err)
	}

	newestJob, err := newestJobTime(ctx, q)
	if err != nil {
		return snapshot{}, err
	}

	return snapshot{
		queues: []queueMetrics{
			{name: "search_outbox", depth: search.Depth, deadLetters: search.DeadLetters, oldestAgeSeconds: search.OldestAgeSeconds},
			{name: "enrichment_outbox", depth: enrichment.Depth, deadLetters: enrichment.DeadLetters, oldestAgeSeconds: enrichment.OldestAgeSeconds},
			{name: "semantic_outbox", depth: semantic.Depth, deadLetters: semantic.DeadLetters, oldestAgeSeconds: semantic.OldestAgeSeconds},
		},
		healthyBoards: boards.Healthy,
		failingBoards: boards.Failing,
		cooledBoards:  boards.Cooled,
		newestJob:     newestJob,
	}, nil
}

// newestJobTime reports when the catalogue last gained an open posting, or the zero time
// when it holds none. An empty catalogue answers with no row, which is a fresh-install
// state rather than a failure — every other family still deserves to be published.
func newestJobTime(ctx context.Context, q metricsQueries) (time.Time, error) {
	newest, err := q.NewestOpenJobCreatedAt(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("newest open job: %w", err)
	}
	if !newest.Valid {
		return time.Time{}, nil
	}
	return newest.Time, nil
}
