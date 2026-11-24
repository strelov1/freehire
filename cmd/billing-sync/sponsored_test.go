package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeSweeper struct {
	ended, requeued int
	endErr          error
}

func (f *fakeSweeper) EndClosedJobs(context.Context) (int64, error) {
	f.ended++
	return 0, f.endErr
}

func (f *fakeSweeper) RequeueDailyCapJobs(context.Context) (int64, error) {
	f.requeued++
	return 0, nil
}

func TestSponsoredSweeps(t *testing.T) {
	at := func(h int) time.Time { return time.Date(2026, 11, 25, h, 20, 0, 0, time.UTC) }
	for _, c := range []struct {
		hour     int
		requeued int
	}{{0, 1}, {2, 1}, {3, 0}, {13, 0}} {
		f := &fakeSweeper{}
		if n := sponsoredSweeps(context.Background(), f, at(c.hour)); n != 0 || f.ended != 1 || f.requeued != c.requeued {
			t.Errorf("hour %d: failures=%d ended=%d requeued=%d", c.hour, n, f.ended, f.requeued)
		}
	}
	// A failing sweep is counted and does not stop the other.
	f := &fakeSweeper{endErr: errors.New("db")}
	if n := sponsoredSweeps(context.Background(), f, at(1)); n != 1 || f.requeued != 1 {
		t.Errorf("failures=%d requeued=%d", n, f.requeued)
	}
}
