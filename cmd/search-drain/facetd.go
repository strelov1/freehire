package main

import (
	"context"
	"errors"
	"log"
	"sync/atomic"
	"time"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/search/search"
)

// facetdWriter is the facetd client's write side.
type facetdWriter interface {
	Write(ctx context.Context, w search.FacetdWrite) error
}

// facetdQueue holds the ids of writes facetd did not take until the retry pass re-sends them.
type facetdQueue interface {
	Enqueue(ctx context.Context, jobIDs []int64) error
}

// facetdMirror copies applied waves to facetd after Meilisearch accepted them. It never fails
// the Meilisearch path and slows it by at most one timeout per run: a write is bounded by
// timeout, and after the first failure that is facetd's (down, slow, not ready) the mirror
// stops calling it for the rest of the run and queues every later wave's ids straight away.
// A rejected batch (400) does not trip it. A nil mirror (FACETD_URL unset) does nothing.
type facetdMirror struct {
	client   facetdWriter
	queue    facetdQueue
	timeout  time.Duration
	failures atomic.Int64 // failed writes this run, published as a metric
	down     atomic.Bool  // facetd failed this run: skip the call, queue directly
}

// Apply sends w to facetd, or queues its job ids when that fails.
func (m *facetdMirror) Apply(ctx context.Context, w search.FacetdWrite) {
	if m == nil || w.Empty() {
		return
	}
	// Detached from the wave's context: a deadline already spent on Meilisearch must not
	// turn a healthy facetd write (or the enqueue after a failed one) into a failure.
	ctx = context.WithoutCancel(ctx)
	ids := w.IDs()
	if m.down.Load() {
		m.enqueue(ctx, ids)
		return
	}
	wctx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()
	err := m.client.Write(wctx, w)
	if err == nil {
		return
	}
	m.failures.Add(1)
	if !errors.Is(err, search.ErrFacetdRejected) && !m.down.Swap(true) {
		log.Printf("search-drain: facetd unavailable, queueing the rest of this run's waves without calling it")
	}
	log.Printf("search-drain: facetd write failed, queueing %d jobs for retry: %v", len(ids), err)
	m.enqueue(ctx, ids)
}

func (m *facetdMirror) enqueue(ctx context.Context, ids []int64) {
	if qerr := m.queue.Enqueue(ctx, ids); qerr != nil {
		log.Printf("search-drain: facetd retry enqueue failed, facetd now misses %d jobs until a rebuild: %v", len(ids), qerr)
	}
}

// dbFacetdQueue is facetdQueue over facetd_outbox.
type dbFacetdQueue struct{ q *db.Queries }

func (d dbFacetdQueue) Enqueue(ctx context.Context, jobIDs []int64) error {
	return d.q.EnqueueFacetdOutbox(ctx, jobIDs)
}
