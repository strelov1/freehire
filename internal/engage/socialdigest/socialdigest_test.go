package socialdigest

import (
	"errors"
	"testing"
	"time"
)

func day(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func TestResolveDay(t *testing.T) {
	now := day("2026-09-04").Add(13 * time.Hour) // 13:00 UTC, when the timer fires

	t.Run("freshest available day is used", func(t *testing.T) {
		got, err := ResolveDay(day("2026-09-03"), true, now)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !got.Equal(day("2026-09-03")) {
			t.Errorf("got %s, want 2026-09-03", got.Format("2006-01-02"))
		}
	})

	// The host's logrotate timing is not recorded anywhere, so the freshest complete
	// day can legitimately be the day before yesterday. That is not staleness.
	t.Run("day before yesterday is still fresh enough", func(t *testing.T) {
		got, err := ResolveDay(day("2026-09-02"), true, now)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !got.Equal(day("2026-09-02")) {
			t.Errorf("got %s, want 2026-09-02", got.Format("2006-01-02"))
		}
	})

	t.Run("exactly at the staleness bound is still accepted", func(t *testing.T) {
		if _, err := ResolveDay(day("2026-09-01"), true, now); err != nil {
			t.Fatalf("three days back should be accepted, got %v", err)
		}
	})

	t.Run("past the staleness bound fails", func(t *testing.T) {
		_, err := ResolveDay(day("2026-08-31"), true, now)
		if !errors.Is(err, ErrStaleViewData) {
			t.Errorf("got %v, want ErrStaleViewData", err)
		}
	})

	t.Run("no view data at all fails distinctly", func(t *testing.T) {
		_, err := ResolveDay(time.Time{}, false, now)
		if !errors.Is(err, ErrNoViewData) {
			t.Errorf("got %v, want ErrNoViewData", err)
		}
	})

	// A day carrying a clock time must not shift the answer or the bound.
	t.Run("a timestamped day is truncated", func(t *testing.T) {
		got, err := ResolveDay(day("2026-09-03").Add(22*time.Hour), true, now)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !got.Equal(day("2026-09-03")) {
			t.Errorf("got %s, want midnight of 2026-09-03", got.Format(time.RFC3339))
		}
	})
}

func TestQuarantineSince(t *testing.T) {
	got := QuarantineSince(day("2026-09-03"))
	if !got.Equal(day("2026-08-27")) {
		t.Errorf("got %s, want 2026-08-27", got.Format("2006-01-02"))
	}
}

func TestDigestEmpty(t *testing.T) {
	if !(Digest{Day: day("2026-09-03")}).Empty() {
		t.Error("a digest with no items should be empty")
	}
	if (Digest{Items: []Posting{{JobID: 1}}}).Empty() {
		t.Error("a digest with an item should not be empty")
	}
}
