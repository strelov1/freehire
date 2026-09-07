package recentfeed

import (
	"testing"
	"time"
)

func entryN(n int) Entry {
	return Entry{Kind: KindSingle, Title: "Role", CompanyName: "Company", JobSlug: string(rune('a' + n))}
}

func TestBroadcaster_NewSubscriptionReceivesBacklog(t *testing.T) {
	b := NewBroadcaster(3)
	b.Publish(entryN(0))
	b.Publish(entryN(1))

	backlog, _, cancel := b.Subscribe()
	defer cancel()

	if len(backlog) != 2 {
		t.Fatalf("backlog = %d entries, want 2: %+v", len(backlog), backlog)
	}
	if backlog[0] != entryN(0) || backlog[1] != entryN(1) {
		t.Errorf("backlog = %+v, want entries in publish order", backlog)
	}
}

func TestBroadcaster_RingBufferTrimsToCapacity(t *testing.T) {
	b := NewBroadcaster(2)
	b.Publish(entryN(0))
	b.Publish(entryN(1))
	b.Publish(entryN(2))

	backlog, _, cancel := b.Subscribe()
	defer cancel()

	if len(backlog) != 2 {
		t.Fatalf("backlog = %d entries, want capacity-bounded 2: %+v", len(backlog), backlog)
	}
	if backlog[0] != entryN(1) || backlog[1] != entryN(2) {
		t.Errorf("backlog = %+v, want the 2 most recently published entries", backlog)
	}
}

func TestBroadcaster_SubscriberReceivesLivePublish(t *testing.T) {
	b := NewBroadcaster(5)
	_, ch, cancel := b.Subscribe()
	defer cancel()

	b.Publish(entryN(0))

	select {
	case got := <-ch:
		if got != entryN(0) {
			t.Errorf("received %+v, want %+v", got, entryN(0))
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for published entry")
	}
}

func TestBroadcaster_MultipleSubscribersEachReceive(t *testing.T) {
	b := NewBroadcaster(5)
	_, ch1, cancel1 := b.Subscribe()
	defer cancel1()
	_, ch2, cancel2 := b.Subscribe()
	defer cancel2()

	b.Publish(entryN(0))

	for i, ch := range []<-chan Entry{ch1, ch2} {
		select {
		case got := <-ch:
			if got != entryN(0) {
				t.Errorf("subscriber %d received %+v, want %+v", i, got, entryN(0))
			}
		case <-time.After(time.Second):
			t.Fatalf("subscriber %d timed out waiting for published entry", i)
		}
	}
}

func TestBroadcaster_CancelStopsDelivery(t *testing.T) {
	b := NewBroadcaster(5)
	_, ch, cancel := b.Subscribe()
	cancel()

	// Publishing after cancel must not block or panic even though the
	// subscriber's channel is never drained again.
	done := make(chan struct{})
	go func() {
		b.Publish(entryN(0))
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Publish blocked after subscriber cancelled")
	}

	select {
	case _, ok := <-ch:
		if ok {
			t.Error("received a value on a cancelled subscription's channel, want it closed or empty")
		}
	default:
	}
}

func TestBroadcaster_SlowSubscriberDoesNotBlockPublish(t *testing.T) {
	b := NewBroadcaster(5)
	_, ch, cancel := b.Subscribe()
	defer cancel()
	_ = ch // never drained, simulating a slow/stalled subscriber

	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			b.Publish(entryN(i))
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Publish blocked on a slow subscriber's full channel")
	}
}
