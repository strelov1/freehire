package embedprogress

import (
	"context"
	"fmt"
	"time"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/search/search"
)

// CoverageCounter runs the coverage scan and reads the queue depth beside it.
// *db.Queries satisfies it.
type CoverageCounter interface {
	DepthReader
	SemanticCoverageCounts(ctx context.Context, targetModel string) (db.SemanticCoverageCountsRow, error)
}

// Measure counts coverage against the model the catalogue is embedded with now, the
// same target cmd/embed enqueues against, so "covered" means "the queue will not take
// it again". It records the queue depth with the counts so the live figure can move by
// how much the queue has drained since; the caller runs both in one snapshot
// transaction, so the two describe the same moment.
func Measure(ctx context.Context, counter CoverageCounter, now func() time.Time) (Snapshot, error) {
	depth, err := counter.SemanticOutboxClaimableDepth(ctx)
	if err != nil {
		return Snapshot{}, fmt.Errorf("embedprogress: reading the queue depth: %w", err)
	}
	row, err := counter.SemanticCoverageCounts(ctx, search.CurrentEmbedderModel())
	if err != nil {
		return Snapshot{}, fmt.Errorf("embedprogress: counting coverage: %w", err)
	}
	return Snapshot{Eligible: row.Eligible, Covered: row.Covered, QueueDepth: &depth, MeasuredAt: now().UTC()}, nil
}
