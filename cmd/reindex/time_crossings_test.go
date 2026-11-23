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
	enqueued []db.EnqueueSearchAgeCrossingsParams
	// unseenLast is the cursor each unseen call returns in turn (0 ends the walk).
	unseenLast []int64
	unseenErr  error
	unseen     []db.EnqueueSearchUnseenCrossingsParams
	savedTo    string
}

func (f *crossingsFake) GetMarkerPassCursor(context.Context, string) (string, error) {
	return f.cursor, f.getErr
}

func (f *crossingsFake) SetMarkerPassCursor(_ context.Context, arg db.SetMarkerPassCursorParams) error {
	f.savedTo = arg.AfterCompany
	return nil
}

func (f *crossingsFake) EnqueueSearchAgeCrossings(_ context.Context, arg db.EnqueueSearchAgeCrossingsParams) (int64, error) {
	f.enqueued = append(f.enqueued, arg)
	return 3, f.enqErr
}

func (f *crossingsFake) EnqueueSearchUnseenCrossings(_ context.Context, arg db.EnqueueSearchUnseenCrossingsParams) (db.EnqueueSearchUnseenCrossingsRow, error) {
	f.unseen = append(f.unseen, arg)
	if f.unseenErr != nil {
		return db.EnqueueSearchUnseenCrossingsRow{}, f.unseenErr
	}
	var last int64
	if len(f.unseenLast) > 0 {
		last, f.unseenLast = f.unseenLast[0], f.unseenLast[1:]
	}
	return db.EnqueueSearchUnseenCrossingsRow{LastID: last, Enqueued: 2}, nil
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
		if len(f.unseen[0].Sources) == 0 || len(f.unseen[0].Sources) != len(f.unseen[0].UnseenSeconds) {
			t.Errorf("unseen windows not passed pairwise: %d sources, %d windows",
				len(f.unseen[0].Sources), len(f.unseen[0].UnseenSeconds))
		}
	})

	t.Run("next run reaches back an overlap into the last one's window", func(t *testing.T) {
		last := now.Add(-8 * time.Hour)
		f := &crossingsFake{cursor: last.Format(time.RFC3339Nano)}
		if err := enqueueTimeCrossings(ctx, f, now); err != nil {
			t.Fatal(err)
		}
		if got, want := f.enqueued[0].Since.Time, last.Add(-timeCrossingsOverlap); !got.Equal(want) {
			t.Errorf("since = %v, want %v", got, want)
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

	t.Run("the unseen half walks job_activity chunk by chunk to the end", func(t *testing.T) {
		f := &crossingsFake{getErr: pgx.ErrNoRows, unseenLast: []int64{40, 95, 0}}
		if err := enqueueTimeCrossings(ctx, f, now); err != nil {
			t.Fatal(err)
		}
		var after []int64
		for _, u := range f.unseen {
			after = append(after, u.AfterID)
			if !u.Since.Time.Equal(f.enqueued[0].Since.Time) || !u.Until.Time.Equal(now) {
				t.Errorf("unseen chunk window (%v, %v] differs from the age pass", u.Since.Time, u.Until.Time)
			}
		}
		if len(after) != 3 || after[0] != 0 || after[1] != 40 || after[2] != 95 {
			t.Errorf("chunks started after %v, want [0 40 95]", after)
		}
		if f.savedTo == "" {
			t.Error("cursor not saved after a complete walk")
		}
	})

	t.Run("a failed unseen chunk keeps the cursor", func(t *testing.T) {
		f := &crossingsFake{cursor: now.Add(-time.Hour).Format(time.RFC3339Nano), unseenErr: errors.New("boom")}
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
