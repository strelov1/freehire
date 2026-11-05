package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/platform/db"
)

type crossingsFake struct {
	cursor   string
	getErr   error
	enqErr   error
	enqueued []db.EnqueueSearchTimeCrossingsParams
	savedTo  string
}

func (f *crossingsFake) GetMarkerPassCursor(context.Context, string) (string, error) {
	return f.cursor, f.getErr
}

func (f *crossingsFake) SetMarkerPassCursor(_ context.Context, arg db.SetMarkerPassCursorParams) error {
	f.savedTo = arg.AfterCompany
	return nil
}

func (f *crossingsFake) EnqueueSearchTimeCrossings(_ context.Context, arg db.EnqueueSearchTimeCrossingsParams) (int64, error) {
	f.enqueued = append(f.enqueued, arg)
	return 3, f.enqErr
}

func TestEnqueueTimeCrossings(t *testing.T) {
	now := time.Date(2026, 11, 5, 20, 0, 0, 0, time.UTC)
	ctx := context.Background()

	t.Run("first run looks back one day and saves now", func(t *testing.T) {
		f := &crossingsFake{getErr: pgx.ErrNoRows}
		if err := enqueueTimeCrossings(ctx, f, now); err != nil {
			t.Fatal(err)
		}
		if got := f.enqueued[0].Since.Time; !got.Equal(now.Add(-24 * time.Hour)) {
			t.Errorf("since = %v", got)
		}
		if f.savedTo != now.Format(time.RFC3339Nano) {
			t.Errorf("cursor saved as %q", f.savedTo)
		}
		if len(f.enqueued[0].Sources) == 0 || len(f.enqueued[0].Sources) != len(f.enqueued[0].UnseenSeconds) {
			t.Errorf("unseen windows not passed pairwise: %d sources, %d windows",
				len(f.enqueued[0].Sources), len(f.enqueued[0].UnseenSeconds))
		}
	})

	t.Run("next run starts where the last one ended", func(t *testing.T) {
		last := now.Add(-8 * time.Hour)
		f := &crossingsFake{cursor: last.Format(time.RFC3339Nano)}
		if err := enqueueTimeCrossings(ctx, f, now); err != nil {
			t.Fatal(err)
		}
		if got := f.enqueued[0].Since.Time; !got.Equal(last) {
			t.Errorf("since = %v, want %v", got, last)
		}
	})

	t.Run("a failed enqueue keeps the cursor", func(t *testing.T) {
		f := &crossingsFake{cursor: now.Add(-time.Hour).Format(time.RFC3339Nano), enqErr: errors.New("boom")}
		if err := enqueueTimeCrossings(ctx, f, now); err == nil {
			t.Fatal("want error")
		}
		if f.savedTo != "" {
			t.Errorf("cursor moved to %q after a failure", f.savedTo)
		}
	})

	t.Run("an unreadable cursor fails rather than guessing", func(t *testing.T) {
		f := &crossingsFake{cursor: "not-a-time"}
		if err := enqueueTimeCrossings(ctx, f, now); err == nil || len(f.enqueued) != 0 {
			t.Fatal("want an error and no enqueue")
		}
	})
}
