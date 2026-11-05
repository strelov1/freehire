package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/job/jobreality"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/search/search"
)

// timeCrossingsPass names this pass's row in marker_pass_cursors. Its value is the RFC 3339
// instant the last run covered up to, not a company: see migration 0241.
const timeCrossingsPass = "search-time-crossings"

// firstTimeCrossingsBand is how far back the very first run looks, when no cursor exists.
// Until this pass shipped, the daily full rebuild re-derived every document, so a day is
// what that rebuild last left uncovered.
const firstTimeCrossingsBand = 24 * time.Hour

type timeCrossingsStore interface {
	GetMarkerPassCursor(ctx context.Context, pass string) (string, error)
	SetMarkerPassCursor(ctx context.Context, arg db.SetMarkerPassCursorParams) error
	EnqueueSearchTimeCrossings(ctx context.Context, arg db.EnqueueSearchTimeCrossingsParams) (int64, error)
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
		if since, err = time.Parse(time.RFC3339Nano, stored); err != nil {
			return fmt.Errorf("parse cursor %q: %w", stored, err)
		}
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

	n, err := q.EnqueueSearchTimeCrossings(ctx, db.EnqueueSearchTimeCrossingsParams{
		Sources:       names,
		UnseenSeconds: secs,
		AgeDays:       jobreality.AgeCrossingDays(),
		PostedAgeDays: jobreality.PostedAgeCrossingDays(),
		Since:         pgtype.Timestamptz{Time: since, Valid: true},
		Until:         pgtype.Timestamptz{Time: now, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("enqueue: %w", err)
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
