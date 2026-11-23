package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/job/jobreality"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
	"github.com/strelov1/freehire/internal/search/search"
)

// timeCrossingsPass names this pass's row in marker_pass_cursors. Its value is the RFC 3339 instant
// the last run covered up to, not a company (migration 0241).
const timeCrossingsPass = "search-time-crossings"

// timeCrossingsOverlap is how far each run reaches back into the window the last one covered.
// The enqueue skips a job whose entry is already queued, and a drain wave holding that entry may have built
// its document just before the crossing, then deleted the entry, losing the crossing. A wave holds an entry
// for at most its lease (SEARCH_DRAIN_LEASE_SECONDS), so an hour of overlap re-queues it; the only cost
// is a second, skipped insert.
const timeCrossingsOverlap = time.Hour

// firstTimeCrossingsBand is how far back the very first run looks, when no cursor exists.
// A day is what the daily full rebuild last left uncovered.
const firstTimeCrossingsBand = 24 * time.Hour

// unseenCrossingsChunk is how many job_activity rows one EnqueueSearchUnseenCrossings call reads.
// Measured on the prod replica (2026-11-23): a million rows took 7-9s cold, so each statement
// stays well under a minute while the whole table takes a few dozen calls.
const unseenCrossingsChunk = 1_000_000

type timeCrossingsStore interface {
	GetMarkerPassCursor(ctx context.Context, pass string) (string, error)
	SetMarkerPassCursor(ctx context.Context, arg db.SetMarkerPassCursorParams) error
	EnqueueSearchAgeCrossings(ctx context.Context, arg db.EnqueueSearchAgeCrossingsParams) (int64, error)
	EnqueueSearchUnseenCrossings(ctx context.Context, arg db.EnqueueSearchUnseenCrossingsParams) (db.EnqueueSearchUnseenCrossingsRow, error)
}

// enqueueTimeCrossings queues the postings whose search document changed since the last run
// through time alone (jobreality.AgeCrossingDays, search.Withheld's unseen rule), then moves
// the cursor. The cursor moves only after the enqueue succeeds, so a failed run is covered
// by the next one's wider band.
func enqueueTimeCrossings(ctx context.Context, q timeCrossingsStore, now time.Time) error {
	since := now.Add(-firstTimeCrossingsBand)
	stored, err := q.GetMarkerPassCursor(ctx, timeCrossingsPass)
	switch {
	case errors.Is(err, pgx.ErrNoRows) || (err == nil && stored == ""):
	case err != nil:
		return fmt.Errorf("read cursor: %w", err)
	default:
		last, err := time.Parse(time.RFC3339Nano, stored)
		if err != nil {
			return fmt.Errorf("parse cursor %q: %w", stored, err)
		}
		since = last.Add(-timeCrossingsOverlap)
	}
	if !since.Before(now) {
		return nil
	}

	windows := sources.UnseenWindows(sources.Taxonomy(), search.UnseenWindow)
	names := make([]string, 0, len(windows))
	secs := make([]int64, 0, len(windows))
	for name, w := range windows {
		names = append(names, name)
		secs = append(secs, int64(w/time.Second))
	}

	n, err := q.EnqueueSearchAgeCrossings(ctx, db.EnqueueSearchAgeCrossingsParams{
		AgeDays:       jobreality.AgeCrossingDays(),
		PostedAgeDays: jobreality.PostedAgeCrossingDays(),
		Since:         pgconv.Instant(since),
		Until:         pgconv.Instant(now),
	})
	if err != nil {
		return fmt.Errorf("enqueue age crossings: %w", err)
	}
	for after := int64(0); ; {
		r, err := q.EnqueueSearchUnseenCrossings(ctx, db.EnqueueSearchUnseenCrossingsParams{
			Sources:       names,
			UnseenSeconds: secs,
			Since:         pgconv.Instant(since),
			Until:         pgconv.Instant(now),
			AfterID:       after,
			ChunkSize:     unseenCrossingsChunk,
		})
		if err != nil {
			return fmt.Errorf("enqueue unseen crossings after job %d: %w", after, err)
		}
		n += r.Enqueued
		if r.LastID == 0 {
			break
		}
		after = r.LastID
	}
	if err := q.SetMarkerPassCursor(ctx, db.SetMarkerPassCursorParams{
		Pass: timeCrossingsPass, AfterCompany: now.UTC().Format(time.RFC3339Nano),
	}); err != nil {
		return fmt.Errorf("save cursor: %w", err)
	}
	log.Printf("reindex: queued %d postings whose search document changed with time since %s",
		n, since.UTC().Format(time.RFC3339))
	return nil
}
