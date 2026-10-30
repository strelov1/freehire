package embedprogress

import (
	"context"
	"log"
	"time"

	"github.com/strelov1/freehire/internal/platform/cache"
)

// Snapshot is the coverage measurement cmd/rollup-stats publishes. QueueDepth is the
// claimable queue depth read in the same transaction as the counts — what the live
// figure moves from. It is nil in a snapshot published before it was recorded.
type Snapshot struct {
	Eligible   int64     `json:"eligible"`
	Covered    int64     `json:"covered"`
	QueueDepth *int64    `json:"queue_depth,omitempty"`
	MeasuredAt time.Time `json:"measured_at"`
}

const snapshotKey = "embedprogress:snapshot"

// snapshotTTL outlives cmd/rollup-stats' three-hour schedule by a wide margin, as
// catalogstats' does: a skipped run should leave the page stale, with its time shown,
// rather than empty.
const snapshotTTL = 24 * time.Hour

// readTimeout bounds one cache read, so an unreachable cache costs the figure and not
// the sample.
const readTimeout = 100 * time.Millisecond

// Store publishes a snapshot for every reader.
func Store(ctx context.Context, c cache.Cache, s Snapshot) error {
	return cache.SetJSON(ctx, c, snapshotKey, s, snapshotTTL)
}

// Load reads the published snapshot. A cold cache, no cache, an unreachable one and an
// undecodable payload all answer "no snapshot" — never a zero-valued one, which would
// reach the page as a measurement of an empty catalogue.
func Load(ctx context.Context, c cache.Cache) (Snapshot, bool) {
	if c == nil {
		return Snapshot{}, false
	}
	readCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()
	s, found, err := cache.GetJSON[Snapshot](readCtx, c, snapshotKey)
	if err != nil {
		log.Printf("embedprogress: reading the published snapshot: %v", err)
		return Snapshot{}, false
	}
	return s, found
}
