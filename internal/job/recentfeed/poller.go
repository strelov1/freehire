package recentfeed

import (
	"context"
	"log"
	"time"

	"github.com/strelov1/freehire/internal/platform/db"
)

// Store is the DB dependency Poller needs. *db.Queries satisfies it; tests may
// supply a fake for anything that does not need real Postgres.
type Store interface {
	ClaimRecentFeedOutboxBatch(ctx context.Context, batchSize int32) ([]db.ClaimRecentFeedOutboxBatchRow, error)
}

// Poller drains recent_feed_outbox on a fixed interval inside the long-lived
// cmd/server process and publishes the grouped result to a Broadcaster. See
// design.md, "Poller runs inside cmd/server, not as a separate cmd/*-drain binary".
type Poller struct {
	store       Store
	broadcaster *Broadcaster
	batchSize   int32
}

// NewPoller builds a Poller that claims up to batchSize outbox rows per Poll call.
func NewPoller(store Store, broadcaster *Broadcaster, batchSize int32) *Poller {
	return &Poller{store: store, broadcaster: broadcaster, batchSize: batchSize}
}

// Run calls Poll every interval until ctx is cancelled. A failed Poll is logged
// and never stops the loop — a homepage feed missing a few seconds of ticks is
// cosmetic, not a reason to bring down a poller inside the API process.
func (p *Poller) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := p.Poll(ctx); err != nil {
				log.Printf("recentfeed: poll: %v", err)
			}
		}
	}
}

// Poll claims one batch, groups it, and publishes the resulting entries. It is
// exported so tests can drive one pass directly instead of waiting on a ticker.
func (p *Poller) Poll(ctx context.Context) error {
	rows, err := p.store.ClaimRecentFeedOutboxBatch(ctx, p.batchSize)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}

	postings := make([]Posting, len(rows))
	for i, r := range rows {
		postings[i] = Posting{Title: r.Title, CompanyName: r.Company, JobSlug: r.PublicSlug}
	}
	for _, e := range Group(postings) {
		p.broadcaster.Publish(e)
	}
	return nil
}
