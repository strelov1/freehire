package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/application/viewlog"
	"github.com/strelov1/freehire/internal/platform/cache"
	"github.com/strelov1/freehire/internal/platform/db"
)

// viewTotalsStatementTimeout bounds the page-view sum: one full pass over job_daily_views
// (22.6s cold on the replica, 2026-11-21). Nothing waits on it, but an unbounded one would
// hold this worker and its unit open.
const viewTotalsStatementTimeout = "10min"

// rebuildViewTotals measures the site-wide page-view totals and publishes them for the
// public engagement endpoint, which used to sum the rollup on every cache miss and timed
// out. Best-effort like the other snapshots: a failure keeps the previous one.
func rebuildViewTotals(ctx context.Context, pool *pgxpool.Pool, redisURL string) {
	c, closeCache, err := snapshotCache(redisURL)
	if err != nil {
		log.Printf("rollup-stats: view totals not published: %v", err)
		return
	}
	defer closeCache()

	// A read-only transaction so SET LOCAL has a scope and the timeout does not outlive
	// this measurement on a pooled connection.
	err = pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{AccessMode: pgx.ReadOnly}, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SET LOCAL statement_timeout = '"+viewTotalsStatementTimeout+"'"); err != nil {
			return fmt.Errorf("set statement_timeout: %w", err)
		}
		return publishViewTotals(ctx, db.New(tx), c, time.Now)
	})
	if err != nil {
		log.Printf("rollup-stats: view totals not published, keeping the previous snapshot: %v", err)
		return
	}
	log.Printf("rollup-stats: published the page-view totals")
}

// viewTotalsCounter is the one query publishViewTotals needs.
type viewTotalsCounter interface {
	GetPageViewTotals(ctx context.Context) (db.GetPageViewTotalsRow, error)
}

// publishViewTotals measures and stores. A failed measurement stores nothing.
func publishViewTotals(ctx context.Context, q viewTotalsCounter, c cache.Cache, now func() time.Time) error {
	row, err := q.GetPageViewTotals(ctx)
	if err != nil {
		return err
	}
	totals := viewlog.Totals{Viewed: row.Viewed, ComputedAt: now().UTC()}
	if row.ViewedSince.Valid {
		since := row.ViewedSince.Time
		totals.ViewedSince = &since
	}
	return viewlog.StoreTotals(ctx, c, totals)
}
