package sponsored

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/strelov1/freehire/internal/platform/db"
)

// Upserter is the one write the impression counter needs. *db.Queries satisfies it.
type Upserter interface {
	UpsertSponsoredDaily(ctx context.Context, arg db.UpsertSponsoredDailyParams) error
}

type impressionKey struct {
	campaignID int64
	day        time.Time
}

// Impressions counts sponsored slots served ("served", not "seen": cached answers and
// off-screen cards count too). Counts sit in memory and are added to sponsored_daily.impressions
// by Flush, so the request path never waits on the database. A failed flush keeps its counts;
// a crash loses at most one interval. The API flushes on a ticker (Run) and once at shutdown;
// cmd/notify flushes once at the end of a run. Same shape as apiusage.Counter.
type Impressions struct {
	store Upserter
	mu    sync.Mutex
	buf   map[impressionKey]int64
}

func NewImpressions(store Upserter) *Impressions {
	return &Impressions{store: store, buf: map[impressionKey]int64{}}
}

// Add counts one served slot for the campaign at now. A nil counter ignores it.
func (b *Impressions) Add(campaignID int64, now time.Time) {
	if b == nil {
		return
	}
	k := impressionKey{campaignID, day(now).Time}
	b.mu.Lock()
	b.buf[k]++
	b.mu.Unlock()
}

// Flush writes everything buffered, one upsert per campaign and day. A row that fails goes
// back into the buffer for the next flush.
func (b *Impressions) Flush(ctx context.Context) error {
	b.mu.Lock()
	batch := b.buf
	b.buf = map[impressionKey]int64{}
	b.mu.Unlock()
	var firstErr error
	for k, n := range batch {
		err := b.store.UpsertSponsoredDaily(ctx, db.UpsertSponsoredDailyParams{
			CampaignID: k.campaignID, Day: day(k.day), Impressions: n,
		})
		if err != nil {
			b.mu.Lock()
			b.buf[k] += n
			b.mu.Unlock()
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// Run flushes every interval until ctx ends; the caller flushes once more after.
func (b *Impressions) Run(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			if err := b.Flush(ctx); err != nil {
				log.Printf("sponsored: flush impressions: %v", err)
			}
		case <-ctx.Done():
			return
		}
	}
}
