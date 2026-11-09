package main

import (
	"context"
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

// facetdMirror copies applied waves to facetd after Meilisearch accepted them. It can never
// fail or slow the Meilisearch path: a write is bounded by timeout, and on any failure the
// wave's job ids go to the retry queue. A nil mirror (FACETD_URL unset) does nothing.
type facetdMirror struct {
	client   facetdWriter
	queue    facetdQueue
	timeout  time.Duration
	failures atomic.Int64 // failed writes this run, published as a metric
}

// Apply sends w to facetd, or queues its job ids when that fails.
func (m *facetdMirror) Apply(ctx context.Context, w search.FacetdWrite) {
	if m == nil || w.Empty() {
		return
	}
	// Detached from the wave's context: a deadline already spent on Meilisearch must not
	// turn a healthy facetd write (or the enqueue after a failed one) into a failure.
	ctx = context.WithoutCancel(ctx)
	wctx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()
	err := m.client.Write(wctx, w)
	if err == nil {
		return
	}
	m.failures.Add(1)
	ids := w.IDs()
	log.Printf("search-drain: facetd write failed, queueing %d jobs for retry: %v", len(ids), err)
	if qerr := m.queue.Enqueue(ctx, ids); qerr != nil {
		log.Printf("search-drain: facetd retry enqueue failed, facetd now misses %d jobs until a rebuild: %v", len(ids), qerr)
	}
}

// dbFacetdQueue is facetdQueue over facetd_outbox.
type dbFacetdQueue struct{ q *db.Queries }

func (d dbFacetdQueue) Enqueue(ctx context.Context, jobIDs []int64) error {
	return d.q.EnqueueFacetdOutbox(ctx, jobIDs)
}
