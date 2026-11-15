package embedprogress

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/strelov1/freehire/internal/platform/cache"
)

// SampleInterval is how often cmd/server reads the queue depth.
const SampleInterval = 15 * time.Second

// subscriberBuffer bounds how far a stream subscriber may fall behind before it misses documents.
// Each document is complete, not a delta, so a missed one costs nothing.
const subscriberBuffer = 4

// DepthReader reads the claimable semantic_outbox depth. *db.Queries satisfies it.
type DepthReader interface {
	SemanticOutboxClaimableDepth(ctx context.Context) (int64, error)
}

// Sampler reads the queue depth on a tick, keeps the recent samples, rebuilds the Progress document
// and hands it to every subscriber. Nothing durable: a restart starts an empty ring, so the rate
// reads "not measured" for minRateSpan.
type Sampler struct {
	depth DepthReader
	cache cache.Cache
	now   func() time.Time

	mu      sync.Mutex
	ring    *Ring
	current Progress
	subs    map[chan Progress]struct{}
}

func NewSampler(depth DepthReader, c cache.Cache, now func() time.Time) *Sampler {
	return &Sampler{depth: depth, cache: c, now: now, ring: NewRing(), subs: map[chan Progress]struct{}{}}
}

// Run samples until ctx is cancelled, starting at once so the first request does not wait an interval.
func (s *Sampler) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		s.sample(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Sampler) sample(ctx context.Context) {
	depth, err := s.depth.SemanticOutboxClaimableDepth(ctx)
	if err != nil {
		// The last document stays: a stale figure with its time beats none.
		log.Printf("embedprogress: reading the queue depth: %v", err)
		return
	}
	var snap *Snapshot
	if loaded, ok := Load(ctx, s.cache); ok {
		snap = &loaded
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.ring.Add(Sample{Depth: depth, At: s.now()})
	latest, _ := s.ring.Latest()
	var rate *float64
	if r, ok := s.ring.DrainRatePerMinute(); ok {
		rate = &r
	}
	s.current = Build(snap, &latest, rate)
	for ch := range s.subs {
		select {
		case ch <- s.current:
		default: // a subscriber that is behind misses this one; see subscriberBuffer
		}
	}
}

// Current is the latest document.
func (s *Sampler) Current() Progress {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.current
}

// Subscribe returns the latest document, a channel receiving every later one, and a
// cancel function the caller must call when it stops listening.
func (s *Sampler) Subscribe() (Progress, <-chan Progress, func()) {
	ch := make(chan Progress, subscriberBuffer)
	s.mu.Lock()
	s.subs[ch] = struct{}{}
	current := s.current
	s.mu.Unlock()
	var once sync.Once
	return current, ch, func() {
		once.Do(func() {
			s.mu.Lock()
			delete(s.subs, ch)
			s.mu.Unlock()
		})
	}
}
