package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/platform/db"
)

// cursorStore reads and writes a per-company marker pass's resume point (marker_pass_cursors).
type cursorStore interface {
	GetMarkerPassCursor(ctx context.Context, pass string) (string, error)
	SetMarkerPassCursor(ctx context.Context, arg db.SetMarkerPassCursorParams) error
}

// passCursor is one marker pass's resume point. Every per-company pass walks its companies in sorted
// order and saves the last one it finished as it goes, so a run its deadline ends is picked up by the
// next run after that company, and a walk that reaches the end clears the point so the next starts
// from the top. Without it a pass longer than the run never finishes: every run redoes the same head
// of the list until it is cut off, and the tail is never reached.
type passCursor struct {
	store cursorStore
	pass  string
	err   error // the outcome of the latest save
}

// resume sorts companies and drops the ones at or before the saved point.
func (c *passCursor) resume(ctx context.Context, companies []string) ([]string, error) {
	after, err := c.store.GetMarkerPassCursor(ctx, c.pass)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("read resume point: %w", err)
	}
	// Sorted here rather than by the query: the comparison with the saved point is Go's, so the
	// order has to be Go's too — Postgres's collation would order the same slugs differently.
	slices.Sort(companies)
	if after == "" {
		return companies, nil
	}
	start, found := slices.BinarySearch(companies, after)
	if found {
		start++
	}
	log.Printf("reindex: %s pass resuming after %q: %d of %d companies left", c.pass, after, len(companies)-start, len(companies))
	return companies[start:], nil
}

// save records point as the last finished company. A failed save does not stop the walk, but if the
// LAST one failed, finish reports it: the stored point is stale, so the next run re-covers ground.
func (c *passCursor) save(ctx context.Context, point string) {
	// The run context may already be cancelled (deadline or SIGTERM), so the write gets its own short one.
	sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	c.err = c.store.SetMarkerPassCursor(sctx, db.SetMarkerPassCursorParams{Pass: c.pass, AfterCompany: point})
	if c.err != nil {
		c.err = fmt.Errorf("save %s resume point %q: %w", c.pass, point, c.err)
		log.Printf("reindex: %v", c.err) // now, not hours later: a killed run never returns it
	}
}

// finish clears the point when the walk reached the end (the run context is still live) and returns
// the latest save's error.
func (c *passCursor) finish(ctx context.Context) error {
	if ctx.Err() == nil {
		c.save(ctx, "")
	}
	return c.err
}
