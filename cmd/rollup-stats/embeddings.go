package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/platform/cache"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/search/embedprogress"
)

// coverageStatementTimeout bounds the coverage scan. It walks every open posting —
// over a minute on production on 2026-10-19 — and nothing waits on it, but an unbounded
// one on a bad night would hold this worker, and its unit, open with it.
const coverageStatementTimeout = "10min"

// rebuildEmbedProgress measures embedding coverage and publishes it for the public
// /embeddings page. Best-effort, like the catalogue and source snapshots before it:
// the rollups are this worker's job and are already committed, so a failure here is
// logged and changes nothing about the exit code.
func rebuildEmbedProgress(ctx context.Context, pool *pgxpool.Pool, redisURL string) {
	c, closeCache, err := snapshotCache(redisURL)
	if err != nil {
		log.Printf("rollup-stats: embedding progress not published: %v", err)
		return
	}
	defer closeCache()

	// A read-only transaction so SET LOCAL has a scope — the timeout must not outlive
	// this measurement on a pooled connection — and REPEATABLE READ so the queue depth
	// and the coverage counts read one snapshot: the live figure moves from that depth,
	// and a drain between the two reads would be counted twice.
	err = pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SET LOCAL statement_timeout = '"+coverageStatementTimeout+"'"); err != nil {
			return fmt.Errorf("set statement_timeout: %w", err)
		}
		return publishCoverage(ctx, db.New(tx), c, time.Now)
	})
	if err != nil {
		log.Printf("rollup-stats: embedding progress not published, keeping the previous snapshot: %v", err)
		return
	}
	log.Printf("rollup-stats: published the embedding-progress snapshot")
}

// publishCoverage measures and stores. On a failed measurement it stores nothing, so
// the page keeps the previous snapshot and its time rather than losing the figure.
func publishCoverage(ctx context.Context, counter embedprogress.CoverageCounter, c cache.Cache, now func() time.Time) error {
	snapshot, err := embedprogress.Measure(ctx, counter, now)
	if err != nil {
		return err
	}
	return embedprogress.Store(ctx, c, snapshot)
}
