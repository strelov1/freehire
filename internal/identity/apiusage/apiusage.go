// Package apiusage counts the API requests each account makes per UTC day, for the
// account's own API page.
//
// Requests are counted in memory and added to api_usage_daily in one statement per
// flush, so the request path never waits on the database. A failed flush keeps its counts
// for the next one; a crash loses at most one interval.
package apiusage

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
)

// Store is the one write the counter needs.
type Store interface {
	AddAPIUsage(ctx context.Context, arg db.AddAPIUsageParams) error
}

type key struct {
	userID int64
	day    time.Time
}

type count struct{ requests, errors int64 }

// Counter buffers per-account daily counts until Flush.
type Counter struct {
	store Store
	mu    sync.Mutex
	buf   map[key]count
}

// New returns a counter that flushes into store.
func New(store Store) *Counter {
	return &Counter{store: store, buf: map[key]count{}}
}

// Record counts one request by userID made at now; failed marks a 4xx/5xx answer.
func (c *Counter) Record(userID int64, failed bool, now time.Time) {
	k := key{userID, now.UTC().Truncate(24 * time.Hour)}
	c.mu.Lock()
	n := c.buf[k]
	n.requests++
	if failed {
		n.errors++
	}
	c.buf[k] = n
	c.mu.Unlock()
}

// Flush writes everything buffered. On failure the counts go back into the buffer, so
// the next flush retries them.
func (c *Counter) Flush(ctx context.Context) error {
	c.mu.Lock()
	batch := c.buf
	c.buf = map[key]count{}
	c.mu.Unlock()
	if len(batch) == 0 {
		return nil
	}
	arg := db.AddAPIUsageParams{}
	for k, n := range batch {
		arg.UserIds = append(arg.UserIds, k.userID)
		arg.Days = append(arg.Days, pgtype.Date{Time: k.day, Valid: true})
		arg.Requests = append(arg.Requests, n.requests)
		arg.Errors = append(arg.Errors, n.errors)
	}
	if err := c.store.AddAPIUsage(ctx, arg); err != nil {
		c.mu.Lock()
		for k, n := range batch {
			m := c.buf[k]
			c.buf[k] = count{m.requests + n.requests, m.errors + n.errors}
		}
		c.mu.Unlock()
		return err
	}
	return nil
}

// Run flushes every interval until ctx ends. The caller flushes once more after the
// server stops serving, so the last interval is kept.
func (c *Counter) Run(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			if err := c.Flush(ctx); err != nil {
				log.Printf("apiusage: flush: %v", err)
			}
		case <-ctx.Done():
			return
		}
	}
}
