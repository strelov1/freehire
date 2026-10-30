package embedprogress

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/platform/cache"
)

type fakeDepth struct {
	depths []int64
	err    error
}

func (f *fakeDepth) SemanticOutboxClaimableDepth(ctx context.Context) (int64, error) {
	if f.err != nil {
		return 0, f.err
	}
	d := f.depths[0]
	if len(f.depths) > 1 {
		f.depths = f.depths[1:]
	}
	return d, nil
}

func TestSampler_PublishesEachSample(t *testing.T) {
	ctx := context.Background()
	c := cache.NewMemory()
	depthAtSnapshot := int64(50)
	if err := Store(ctx, c, Snapshot{Eligible: 100, Covered: 50, QueueDepth: &depthAtSnapshot, MeasuredAt: t0}); err != nil {
		t.Fatal(err)
	}
	now := t0
	s := NewSampler(&fakeDepth{depths: []int64{40, 30}}, c, func() time.Time { return now })

	current, events, cancel := s.Subscribe()
	defer cancel()
	if current.Queued != nil {
		t.Errorf("before any sample queued = %v, want null", *current.Queued)
	}

	s.sample(ctx)
	got := <-events
	if *got.Queued != 40 || *got.Covered != 60 {
		t.Errorf("first event = queued %d covered %d, want 40 and 60", *got.Queued, *got.Covered)
	}

	now = now.Add(15 * time.Second)
	s.sample(ctx)
	if got := <-events; *got.Queued != 30 || *got.Covered != 70 {
		t.Errorf("second event = queued %d covered %d, want 30 and 70", *got.Queued, *got.Covered)
	}
	if cur := s.Current(); *cur.Queued != 30 {
		t.Errorf("Current queued = %d, want 30", *cur.Queued)
	}
}

func TestSampler_AFailedReadKeepsTheLastDocument(t *testing.T) {
	ctx := context.Background()
	depth := &fakeDepth{depths: []int64{40}}
	s := NewSampler(depth, nil, func() time.Time { return t0 })
	s.sample(ctx)

	depth.err = errors.New("db down")
	s.sample(ctx)

	if cur := s.Current(); cur.Queued == nil || *cur.Queued != 40 {
		t.Errorf("Current after a failed read = %v, want the last good 40", cur.Queued)
	}
}

// A subscriber that stops reading must not hold up the sampler or anyone else.
func TestSampler_ASlowSubscriberDoesNotBlock(t *testing.T) {
	ctx := context.Background()
	s := NewSampler(&fakeDepth{depths: []int64{1}}, nil, func() time.Time { return t0 })
	_, _, cancel := s.Subscribe()
	defer cancel()

	done := make(chan struct{})
	go func() {
		for i := 0; i < subscriberBuffer+10; i++ {
			s.sample(ctx)
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("sampling blocked on a subscriber that never reads")
	}
}
