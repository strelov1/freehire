package recentfeed

import "sync"

// subscriberBuffer bounds how many published entries a subscriber's channel holds
// before Publish starts dropping for it rather than blocking. The feed is cosmetic
// and entries arrive in small batches (see AggregationThreshold), so this only
// matters for a stalled or unusually slow client — losing an entry it never picked
// up costs nothing correctness-wise, unlike blocking every other connected client
// on one slow reader.
const subscriberBuffer = 32

// Broadcaster fans newly produced feed Entry values out to every connected SSE
// client and replays a short backlog to a client that has just connected, so a new
// connection never starts on an empty feed. It holds no reference to Postgres or
// any other durable store: losing its state on a process restart or a blue/green
// flip is an accepted, purely cosmetic gap (see design.md, "In-process ring buffer
// + fan-out, no persistence").
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
