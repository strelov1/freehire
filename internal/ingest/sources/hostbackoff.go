package sources

import (
	"context"
	"sync"
	"time"
)

// Host-wide 429 backoff defaults. The base is the old per-request retry delay doubled; the cap
// is long enough to outlast the per-minute windows the metered platforms use and short enough
// that a run whose host has recovered loses at most a minute to finding out.
const (
	hostBackoffBase = time.Second
	hostBackoffMax  = time.Minute
)

// hostBackoff holds back every request to a host that has just refused one with a 429.
//
// A Retry-After honoured per REQUEST is not enough once a crawl runs several boards at once: the
// request that drew the 429 waits, and the others keep firing into the same refusal, so one
// tripped budget fails a mass of boards (it is also how join's retries went out on top of its
// pacer). The hold is per host, shared by every request this Client makes, so the whole crawl
// slows the moment the platform says so, and boards the run then never reaches keep their last
// state instead of recording a failure each.
//
// A refusal after a hold has expired doubles the next hold, up to max; the refusals of one burst
// count once. A served response resets the escalation (it does not shorten a hold already set,
// which other workers are already waiting out).
type hostBackoff struct {
	base, max time.Duration
	now       func() time.Time

	mu     sync.Mutex
	until  map[string]time.Time
	streak map[string]int
}

func newHostBackoff(base, max time.Duration) *hostBackoff {
	return &hostBackoff{
		base:   base,
		max:    max,
		now:    time.Now,
		until:  map[string]time.Time{},
		streak: map[string]int{},
	}
}

// wait blocks until host is no longer held, or ctx ends. A nil receiver never waits, so a
// Client built without one (tests, struct literals) keeps its old behaviour.
func (b *hostBackoff) wait(ctx context.Context, host string) error {
	if b == nil {
		return nil
	}
	for {
		b.mu.Lock()
		d := b.until[host].Sub(b.now())
		if d <= 0 {
			// Forget an expired hold: the Client in cmd/server lives for days and meets
			// many hosts, and the map must not keep every one it ever held.
			delete(b.until, host)
		}
		b.mu.Unlock()
		if d <= 0 {
			return nil
		}
		// Loop rather than return after one sleep: another worker's refusal may have pushed
		// the hold further out while this one slept.
		t := time.NewTimer(d)
		select {
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		case <-t.C:
		}
	}
}

// refused records a 429 from host. hint is the server's Retry-After (0 when absent); the hold
// is the longer of it and the escalation, and never shortens an existing hold.
//
// Only a refusal arriving after the current hold has EXPIRED escalates. The refusals that land
// while a hold is running are one burst — every worker with a request in flight draws its 429
// at the same moment — and counting each would jump the first hold straight to the cap, then
// release every worker together when it ran out. Such a refusal may still lengthen the hold
// with its own Retry-After.
func (b *hostBackoff) refused(host string, hint time.Duration) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	now := b.now()
	if b.until[host].After(now) {
		if until := now.Add(hint); until.After(b.until[host]) {
			b.until[host] = until
		}
		return
	}
	d := b.base << b.streak[host]
	if d > b.max || d <= 0 { // d<=0 guards the shift overflowing on a long streak
		d = b.max
	} else {
		b.streak[host]++
	}
	if hint > d {
		d = hint
	}
	b.until[host] = now.Add(d)
}

// served records a successful response from host, resetting its escalation.
func (b *hostBackoff) served(host string) {
	if b == nil {
		return
	}
	b.mu.Lock()
	delete(b.streak, host)
	b.mu.Unlock()
}
