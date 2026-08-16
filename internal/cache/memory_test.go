package cache

import (
	"context"
	"testing"
	"time"
)

// fixedClock drives Memory's expiry deterministically. Sleeping past a TTL would make
// these tests slow and flaky for no gain — expiry is arithmetic on a timestamp, and the
// timestamp is worth injecting.
type fixedClock struct{ now time.Time }

func (c *fixedClock) Now() time.Time          { return c.now }
func (c *fixedClock) advance(d time.Duration) { c.now = c.now.Add(d) }

func newTestMemory(t *testing.T) (*Memory, *fixedClock) {
	t.Helper()
	clock := &fixedClock{now: time.Unix(1_700_000_000, 0)}
	m := NewMemory()
	m.now = clock.Now
	return m, clock
}

func TestMemoryRoundTrip(t *testing.T) {
	m, _ := newTestMemory(t)
	ctx := context.Background()

	if err := m.Set(ctx, "k", []byte("v"), time.Minute); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got, found, err := m.Get(ctx, "k")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !found {
		t.Fatal("Get: found = false, want true for a key just set")
	}
	if string(got) != "v" {
		t.Errorf("Get = %q, want %q", got, "v")
	}
}

func TestMemoryMissingKeyIsNotAnError(t *testing.T) {
	m, _ := newTestMemory(t)

	got, found, err := m.Get(context.Background(), "absent")
	if err != nil {
		t.Fatalf("Get on an absent key returned an error: %v — a miss is a normal outcome, not a failure", err)
	}
	if found {
		t.Error("Get: found = true, want false for a key never set")
	}
	if got != nil {
		t.Errorf("Get = %q, want nil on a miss", got)
	}
}

func TestMemoryEntryExpires(t *testing.T) {
	m, clock := newTestMemory(t)
	ctx := context.Background()

	if err := m.Set(ctx, "k", []byte("v"), time.Minute); err != nil {
		t.Fatalf("Set: %v", err)
	}

	clock.advance(59 * time.Second)
	if _, found, _ := m.Get(ctx, "k"); !found {
		t.Error("entry expired early: found = false one second before its TTL")
	}

	clock.advance(2 * time.Second)
	got, found, err := m.Get(ctx, "k")
	if err != nil {
		t.Fatalf("Get past TTL returned an error: %v — an expired entry is a miss", err)
	}
	if found {
		t.Errorf("entry survived its TTL: found = true, value %q", got)
	}
}

func TestMemoryOverwrite(t *testing.T) {
	m, _ := newTestMemory(t)
	ctx := context.Background()

	if err := m.Set(ctx, "k", []byte("first"), time.Minute); err != nil {
		t.Fatalf("Set first: %v", err)
	}
	if err := m.Set(ctx, "k", []byte("second"), time.Minute); err != nil {
		t.Fatalf("Set second: %v", err)
	}

	got, _, err := m.Get(ctx, "k")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != "second" {
		t.Errorf("Get = %q, want %q — a later Set must replace the entry", got, "second")
	}
}

// Memory backs a snapshot read on the hottest public path, so concurrent use is the
// normal case, not an edge one. Run with -race.
func TestMemoryConcurrentUse(t *testing.T) {
	m, _ := newTestMemory(t)
	ctx := context.Background()

	done := make(chan struct{})
	for i := range 8 {
		go func() {
			defer func() { done <- struct{}{} }()
			for range 50 {
				_ = m.Set(ctx, "k", []byte{byte(i)}, time.Minute)
				_, _, _ = m.Get(ctx, "k")
			}
		}()
	}
	for range 8 {
		<-done
	}
}
