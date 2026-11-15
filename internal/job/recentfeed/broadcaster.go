package recentfeed

import "sync"

// subscriberBuffer bounds how many entries a subscriber's channel holds before Publish
// drops for it rather than blocking every other client on one slow reader. The feed is
// cosmetic, so a dropped entry costs nothing.
const subscriberBuffer = 32

// Broadcaster fans newly produced feed Entry values out to every connected SSE
// client and replays a short backlog to a client that has just connected, so a new
// connection never starts on an empty feed. It is in-memory only: losing state on a
// restart or blue/green flip is an accepted, cosmetic gap.
type Broadcaster struct {
	mu   sync.Mutex
	cap  int
	ring []Entry
	subs map[chan Entry]struct{}
}

// NewBroadcaster creates a Broadcaster whose backlog holds at most capacity of the
// most recently published entries.
func NewBroadcaster(capacity int) *Broadcaster {
	return &Broadcaster{
		cap:  capacity,
		subs: make(map[chan Entry]struct{}),
	}
}

// Publish appends e to the backlog (trimming the oldest entry past capacity) and
// delivers it to every currently subscribed channel. Delivery to a subscriber whose
// channel is full is dropped rather than blocking — see subscriberBuffer.
func (b *Broadcaster) Publish(e Entry) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.ring = append(b.ring, e)
	if over := len(b.ring) - b.cap; over > 0 {
		b.ring = b.ring[over:]
	}

	for ch := range b.subs {
		select {
		case ch <- e:
		default:
		}
	}
}

// Subscribe registers a new subscriber and returns the current backlog (oldest
// first), a channel that receives every entry published after this call, and a
// cancel function that must be called when the caller is done listening. The
// returned channel is never closed by a later Publish; only cancel closes it.
func (b *Broadcaster) Subscribe() (backlog []Entry, entries <-chan Entry, cancel func()) {
	b.mu.Lock()
	defer b.mu.Unlock()

	backlog = make([]Entry, len(b.ring))
	copy(backlog, b.ring)

	ch := make(chan Entry, subscriberBuffer)
	b.subs[ch] = struct{}{}

	cancel = func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if _, ok := b.subs[ch]; ok {
			delete(b.subs, ch)
			close(ch)
		}
	}
	return backlog, ch, cancel
}
