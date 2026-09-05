package socialdigest

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

// fakeRepo is an in-memory Repository. The ledger is a real map keyed the way the
// table is, so the publish-once behaviour is exercised rather than stubbed.
type fakeRepo struct {
	latest     time.Time
	hasLatest  bool
	candidates []Posting
	quarantine map[int64]bool
	published  map[string]bool // "day|channel"
	recorded   map[string][]Posting

	latestErr error
	topErr    error
	recordErr error
}

func newFakeRepo(candidates ...Posting) *fakeRepo {
	return &fakeRepo{
		latest:     day("2026-09-03"),
		hasLatest:  true,
		candidates: candidates,
		published:  map[string]bool{},
		recorded:   map[string][]Posting{},
	}
}

func key(d time.Time, channel string) string { return d.Format(dayLayout) + "|" + channel }

func (f *fakeRepo) LatestViewDay(context.Context) (time.Time, bool, error) {
	return f.latest, f.hasLatest, f.latestErr
}

func (f *fakeRepo) TopPageViewed(_ context.Context, _ time.Time, limit int) ([]Posting, error) {
	if f.topErr != nil {
		return nil, f.topErr
	}
	if len(f.candidates) > limit {
		return f.candidates[:limit], nil
	}
	return f.candidates, nil
}

func (f *fakeRepo) RecentlyDigested(context.Context, time.Time) (map[int64]bool, error) {
	return f.quarantine, nil
}

func (f *fakeRepo) PublishedForChannel(_ context.Context, d time.Time, channel string) (bool, error) {
	return f.published[key(d, channel)], nil
}

func (f *fakeRepo) RecordPublished(_ context.Context, d time.Time, channel string, items []Posting) error {
	if f.recordErr != nil {
		return f.recordErr
	}
	f.published[key(d, channel)] = true
	f.recorded[key(d, channel)] = items
	return nil
}

// fakePublisher records what it was asked to send and can be told to fail.
type fakePublisher struct {
	name  string
	calls int
	fail  error
}

func (p *fakePublisher) Name() string { return p.name }

func (p *fakePublisher) Render(d Digest) (string, error) {
	return p.name + ":" + d.Day.Format(dayLayout), nil
}

func (p *fakePublisher) Publish(context.Context, Digest) error {
	p.calls++
	return p.fail
}

func fixedNow(s string) func() time.Time {
	return func() time.Time { return day(s).Add(13 * time.Hour) }
}

func TestServiceBuild(t *testing.T) {
	ctx := context.Background()

	t.Run("builds for the freshest day when none is requested", func(t *testing.T) {
		repo := newFakeRepo(posting(1, "alpha", 90), posting(2, "beta", 50))
		got, err := New(repo, fixedNow("2026-09-04")).Build(ctx, time.Time{})
		if err != nil {
			t.Fatal(err)
		}
		if !got.Day.Equal(day("2026-09-03")) {
			t.Errorf("day = %s, want 2026-09-03", got.Day.Format(dayLayout))
		}
		assertIDs(t, got.Items, []int64{1, 2})
	})

	t.Run("an explicit day bypasses the freshness lookup entirely", func(t *testing.T) {
		repo := newFakeRepo(posting(1, "alpha", 90))
		repo.latestErr = errors.New("must not be called")
		got, err := New(repo, fixedNow("2026-09-04")).Build(ctx, day("2025-01-01"))
		if err != nil {
			t.Fatal(err)
		}
		if !got.Day.Equal(day("2025-01-01")) {
			t.Errorf("day = %s, want 2025-01-01", got.Day.Format(dayLayout))
		}
	})

	t.Run("stale view data fails rather than publishing", func(t *testing.T) {
		repo := newFakeRepo(posting(1, "alpha", 90))
		repo.latest = day("2026-08-01")
		_, err := New(repo, fixedNow("2026-09-04")).Build(ctx, time.Time{})
		if !errors.Is(err, ErrStaleViewData) {
			t.Errorf("got %v, want ErrStaleViewData", err)
		}
	})

	t.Run("no view data at all fails distinctly", func(t *testing.T) {
		repo := newFakeRepo()
		repo.hasLatest = false
		_, err := New(repo, fixedNow("2026-09-04")).Build(ctx, time.Time{})
		if !errors.Is(err, ErrNoViewData) {
			t.Errorf("got %v, want ErrNoViewData", err)
		}
	})

	t.Run("a quiet day is an empty digest, not an error", func(t *testing.T) {
		repo := newFakeRepo(posting(1, "alpha", MinPageUniques-1))
		got, err := New(repo, fixedNow("2026-09-04")).Build(ctx, time.Time{})
		if err != nil {
			t.Fatalf("a quiet day must not be an error: %v", err)
		}
		if !got.Empty() {
			t.Errorf("got %v, want an empty digest", ids(got.Items))
		}
	})

	t.Run("a candidate query failure surfaces", func(t *testing.T) {
		repo := newFakeRepo()
		repo.topErr = errors.New("boom")
		if _, err := New(repo, fixedNow("2026-09-04")).Build(ctx, time.Time{}); err == nil {
			t.Error("want an error")
		}
	})
}

func TestServiceDispatch(t *testing.T) {
	ctx := context.Background()
	digest := Digest{Day: day("2026-09-03"), Items: []Posting{posting(1, "alpha", 90)}}

	t.Run("publishes to every channel and records each", func(t *testing.T) {
		repo := newFakeRepo()
		a, b := &fakePublisher{name: "discord"}, &fakePublisher{name: "other"}
		if err := New(repo, nil).Dispatch(ctx, digest, []Publisher{a, b}); err != nil {
			t.Fatal(err)
		}
		if a.calls != 1 || b.calls != 1 {
			t.Errorf("calls = %d/%d, want 1/1", a.calls, b.calls)
		}
		if !repo.published[key(digest.Day, "discord")] || !repo.published[key(digest.Day, "other")] {
			t.Error("both channels should be in the ledger")
		}
	})

	t.Run("a second run for the same day publishes nothing", func(t *testing.T) {
		repo := newFakeRepo()
		p := &fakePublisher{name: "discord"}
		svc := New(repo, nil)
		if err := svc.Dispatch(ctx, digest, []Publisher{p}); err != nil {
			t.Fatal(err)
		}
		if err := svc.Dispatch(ctx, digest, []Publisher{p}); err != nil {
			t.Fatal(err)
		}
		if p.calls != 1 {
			t.Errorf("published %d times, want 1", p.calls)
		}
	})

	// The reason the ledger is keyed on the channel and not on the day alone.
	t.Run("a channel that has not published yet still publishes", func(t *testing.T) {
		repo := newFakeRepo()
		repo.published[key(digest.Day, "discord")] = true
		discord, other := &fakePublisher{name: "discord"}, &fakePublisher{name: "other"}
		if err := New(repo, nil).Dispatch(ctx, digest, []Publisher{discord, other}); err != nil {
			t.Fatal(err)
		}
		if discord.calls != 0 {
			t.Error("discord already published this day and should have been skipped")
		}
		if other.calls != 1 {
			t.Errorf("other published %d times, want 1", other.calls)
		}
	})

	t.Run("one channel failing does not stop the other", func(t *testing.T) {
		repo := newFakeRepo()
		broken := &fakePublisher{name: "broken", fail: errors.New("503")}
		ok := &fakePublisher{name: "ok"}
		err := New(repo, nil).Dispatch(ctx, digest, []Publisher{broken, ok})
		if err == nil {
			t.Fatal("a failed channel must fail the run")
		}
		if ok.calls != 1 {
			t.Error("the healthy channel should still have published")
		}
		if repo.published[key(digest.Day, "broken")] {
			t.Error("a failed publish must not be recorded as published")
		}
		if !repo.published[key(digest.Day, "ok")] {
			t.Error("the successful publish should be recorded")
		}
	})

	t.Run("all failures are reported together, not just the first", func(t *testing.T) {
		repo := newFakeRepo()
		err := New(repo, nil).Dispatch(ctx, digest, []Publisher{
			&fakePublisher{name: "first", fail: errors.New("501")},
			&fakePublisher{name: "second", fail: errors.New("502")},
		})
		if err == nil {
			t.Fatal("want an error")
		}
		if !strings.Contains(err.Error(), "first") || !strings.Contains(err.Error(), "second") {
			t.Errorf("both channels should be named, got %v", err)
		}
	})

	// A ledger write that fails after a successful publish is the one case that can
	// cause a duplicate post tomorrow, so it must be loud rather than swallowed.
	t.Run("a ledger failure after a successful publish is reported", func(t *testing.T) {
		repo := newFakeRepo()
		repo.recordErr = errors.New("disk full")
		p := &fakePublisher{name: "discord"}
		err := New(repo, nil).Dispatch(ctx, digest, []Publisher{p})
		if err == nil {
			t.Fatal("want an error")
		}
		if p.calls != 1 {
			t.Error("the publish itself did happen and should not be retried in this run")
		}
	})

	t.Run("an empty digest publishes nowhere", func(t *testing.T) {
		repo := newFakeRepo()
		p := &fakePublisher{name: "discord"}
		if err := New(repo, nil).Dispatch(ctx, Digest{Day: digest.Day}, []Publisher{p}); err != nil {
			t.Fatal(err)
		}
		if p.calls != 0 {
			t.Error("a quiet day should reach no channel")
		}
		if len(repo.published) != 0 {
			t.Error("a quiet day should write no ledger row")
		}
	})
}

func TestJobURL(t *testing.T) {
	got := JobURL("https://freehire.me/", "acme-engineer-123", "discord")
	want := "https://freehire.me/jobs/acme-engineer-123?utm_source=discord"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
