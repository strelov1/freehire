// Package catalogstats owns the public catalogue-scale figures: how many open postings
// the catalogue holds, from how many companies, across how many platforms and channels.
//
// The figures are computed once by a scheduled worker and published as one Snapshot, so
// every surface that quotes catalogue scale quotes the same numbers. That is the point
// of the package: before it, /about and /open each took their own estimate at their own
// moment, and could disagree.
//
// The exact counts are catalogue-wide, so computing one is a sequential scan. It runs in
// the worker and never on a request path — see Load, which reads the published snapshot
// and degrades to an approximation rather than ever recomputing.
package catalogstats

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/sources"
)

// Snapshot is the catalogue's scale at one instant.
type Snapshot struct {
	// OpenJobs and Companies are exact counts over the set the public listings
	// paginate: open, not duplicate-suppressed, not private.
	OpenJobs  int64 `json:"open_jobs"`
	Companies int64 `json:"companies"`

	// Sources, ATSPlatforms and TelegramChannels describe reach rather than contents:
	// what the crawler can read, whether or not each currently holds an open posting.
	//
	// Sources is every registered adapter; ATSPlatforms is the ATS subset. Both are
	// published because they answer different questions — "how much of the market can
	// you see" and "how many hiring systems do you speak" — and because quoting the
	// wider number under the narrower label is the mistake this replaces.
	Sources          int `json:"sources"`
	ATSPlatforms     int `json:"ats_platforms"`
	TelegramChannels int `json:"telegram_channels"`

	ComputedAt time.Time `json:"computed_at"`
}

// ExactCounter reads the exact catalogue totals. It is the one thing Compute needs from
// the database, named narrowly so the dependency says so: *db.Queries satisfies it, and
// nothing else about the query layer is reachable from here.
type ExactCounter interface {
	CountCatalogueScale(ctx context.Context) (db.CountCatalogueScaleRow, error)
}

// Compute measures the catalogue now.
//
// The exact counts are a full scan, so this belongs in the scheduled worker and nowhere
// near a request. Load is the read path.
//
// telegramChannels is passed in rather than read here: resolving it means reading
// sources/telegram.yml, which is relative to the worker's working directory. Keeping
// that at the edge leaves Compute with no file system of its own.
func Compute(ctx context.Context, counts ExactCounter, telegramChannels int) (Snapshot, error) {
	exact, err := counts.CountCatalogueScale(ctx)
	if err != nil {
		return Snapshot{}, fmt.Errorf("catalogstats: counting catalogue scale: %w", err)
	}

	return Snapshot{
		OpenJobs:         exact.OpenJobs,
		Companies:        exact.Companies,
		Sources:          Sources(),
		ATSPlatforms:     ATSPlatforms(),
		TelegramChannels: telegramChannels,
		ComputedAt:       time.Now().UTC(),
	}, nil
}

// Sources counts every registered source adapter, of every kind — ATS platforms,
// aggregators, and single-company career feeds alike. It is the breadth figure: how many
// distinct places the crawler knows how to read.
func Sources() int { return len(sources.Taxonomy()) }

// ATSPlatforms counts the registered multi-tenant applicant-tracking systems: adapters
// addressed by a board id that are not aggregators.
//
// Both exclusions are the label doing its job. An aggregator republishes many
// companies' postings and is not an ATS; a boardless adapter serves one company's own
// careers feed and is not a platform. The registry holds all three kinds, and counting
// all of them under "ATS platforms" is what the frontend constant this replaces did.
//
// It reads the taxonomy registry, which carries no transport, so this is pure in-process
// work — no network, no credentials, and the same answer on every host.
func ATSPlatforms() int {
	registry := sources.Taxonomy()
	aggregators := sources.AggregatorProviders(registry)

	n := 0
	for _, provider := range sources.BoardKeyedProviders(registry) {
		if !slices.Contains(aggregators, provider) {
			n++
		}
	}
	return n
}
