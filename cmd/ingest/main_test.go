package main

import (
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/ingest/pipeline"
	sweeppolicy "github.com/strelov1/freehire/internal/ingest/sweep"
)

// HYDRATION_RETRY_DAYS widens the window during which a body-less row is re-offered for
// detail hydration, so an operator can repair a backlog the ordinary two-week window has
// already aged past (freehire#1866). Unset means the default; a value that is not a positive
// number is a config error, not a silent fallback — a typo on a repair run would otherwise
// look like a run that repaired nothing.
func TestHydrationRetryWindowFor(t *testing.T) {
	cases := []struct {
		name    string
		env     string
		want    time.Duration
		wantErr bool
	}{
		{"unset keeps the default", "", pipeline.HydrationRetryWindow, false},
		{"widened for a repair run", "365", 365 * 24 * time.Hour, false},
		{"narrowed", "1", 24 * time.Hour, false},
		{"not a number", "two weeks", 0, true},
		{"zero", "0", 0, true},
		{"negative", "-5", 0, true},
	}
	for _, tc := range cases {
		got, err := hydrationRetryWindowFor(tc.env)
		if (err != nil) != tc.wantErr {
			t.Errorf("%s: hydrationRetryWindowFor(%q) err = %v, wantErr %v", tc.name, tc.env, err, tc.wantErr)
			continue
		}
		if err == nil && got != tc.want {
			t.Errorf("%s: hydrationRetryWindowFor(%q) = %v, want %v", tc.name, tc.env, got, tc.want)
		}
	}
}

// INGEST_REFETCH_ALL empties the seen-set so a crawl re-writes the provider's stored rows —
// the only way an adapter fix reaches postings ingested before it, since a re-listed posting
// otherwise takes the content-less refresh path. Anything but the two accepted spellings is a
// config error rather than a silent false, for the same reason HYDRATION_RETRY_DAYS is.
func TestRefetchAllFor(t *testing.T) {
	cases := []struct {
		name    string
		env     string
		want    bool
		wantErr bool
	}{
		{"unset is an ordinary crawl", "", false, false},
		{"one", "1", true, false},
		{"true", "true", true, false},
		{"zero is not a spelling of off", "0", false, true},
		{"typo", "yes", false, true},
	}
	for _, tc := range cases {
		got, err := refetchAllFor(tc.env)
		if (err != nil) != tc.wantErr {
			t.Errorf("%s: refetchAllFor(%q) err = %v, wantErr %v", tc.name, tc.env, err, tc.wantErr)
			continue
		}
		if err == nil && got != tc.want {
			t.Errorf("%s: refetchAllFor(%q) = %v, want %v", tc.name, tc.env, got, tc.want)
		}
	}
}

// defaultSeenPolicy is the seen-set policy a deployment with no knobs set gets: retry a
// body-less row inside the default window, re-fetch nothing else. It is what the store
// integration tests construct, so a test asserting a WRITE never accidentally asserts a
// re-fetch policy as well.
func defaultSeenPolicy() seenPolicy {
	return seenPolicy{
		hydrationWindow: pipeline.HydrationRetryWindow,
		bodies:          sweeppolicy.BodyRefresh{Slices: 1, Slot: sweeppolicy.BodyRefreshDisabledSlot},
	}
}
