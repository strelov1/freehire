package enrich

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"
)

func TestUpstreamBackoffGrowsCapsAndResets(t *testing.T) {
	var b upstreamBackoff
	var got []time.Duration
	up := int64(0)
	// Seven refused waves in a row: doubles from backoffMin, then holds at backoffMax.
	for range 7 {
		up++
		got = append(got, b.next(0, up))
	}
	want := []time.Duration{30 * time.Second, time.Minute, 2 * time.Minute, 4 * time.Minute, 8 * time.Minute, backoffMax, backoffMax}
	if !slices.Equal(got, want) {
		t.Fatalf("refused waves: got %v, want %v", got, want)
	}

	// One write resets it, even in a wave that also had upstream failures.
	up++
	if d := b.next(1, up); d != 0 {
		t.Fatalf("wave with a write: got %v, want 0", d)
	}
	up++
	if d := b.next(1, up); d != backoffMin {
		t.Fatalf("first refused wave after a reset: got %v, want %v", d, backoffMin)
	}

	// A wave that only failed by the posting's fault says nothing about the upstream.
	if d := b.next(1, up); d != 0 {
		t.Fatalf("wave without upstream failures: got %v, want 0", d)
	}
}

// The drain must pause between waves the upstream refused entirely, and must not pause
// for failures the posting caused (those are not a sign the upstream is down).
func TestRunPausesOnlyWhileUpstreamRefuses(t *testing.T) {
	for _, tc := range []struct {
		name        string
		providerErr error
		want        []time.Duration
	}{
		{
			name:        "quota exhausted",
			providerErr: errors.New("API returned unexpected status code: 429"),
			want:        []time.Duration{backoffMin, 2 * backoffMin, 4 * backoffMin},
		},
		{
			name:        "posting at fault",
			providerErr: errUnparseableResponse,
			want:        nil,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakeStore{
				claims: [][]Claimed{
					{{OutboxID: 1, JobID: 100, TargetVersion: Version}},
					{{OutboxID: 2, JobID: 100, TargetVersion: Version}},
					{{OutboxID: 3, JobID: 100, TargetVersion: Version}},
				},
				jobs: map[int64]JobInput{100: {Title: "Go dev"}},
			}
			prov := &funcProvider{fn: func(JobInput) (Enrichment, error) { return Enrichment{}, tc.providerErr }}
			var waits []time.Duration
			r := Runner{Provider: prov, Store: store, wait: func(_ context.Context, d time.Duration) { waits = append(waits, d) }}

			if _, err := r.Run(context.Background(), opts()); err != nil {
				t.Fatalf("Run: %v", err)
			}
			if !slices.Equal(waits, tc.want) {
				t.Fatalf("pauses: got %v, want %v", waits, tc.want)
			}
		})
	}
}

func TestSleepCtxReturnsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan struct{})
	go func() { sleepCtx(ctx, time.Hour); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("sleepCtx ignored a cancelled context")
	}
}

// noWait skips the between-wave pause in tests that are not about it.
func noWait(context.Context, time.Duration) {}
