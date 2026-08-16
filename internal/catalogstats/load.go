package catalogstats

import (
	"context"
	"log"
	"time"

	"github.com/strelov1/freehire/internal/cache"
)

// snapshotKey is where the published snapshot lives. One key, one value, read by every
// process — that shared read is what makes two surfaces agree.
const snapshotKey = "catalogstats:snapshot"

// snapshotTTL deliberately outlives the worker's schedule by a wide margin. Setting it
// near the cron interval would mean one skipped or slow run drops every surface back to
// the estimate; a long window degrades a missed run to "stale but exact", which is
// strictly better than "fresh but wrong". ComputedAt travels in the snapshot, so the
// staleness is observable rather than hidden.
const snapshotTTL = 24 * time.Hour

// readTimeout bounds a single cache read. Long enough for a healthy same-host round
// trip, short enough that an unreachable backend degrades the figure instead of holding
// up the response — mirroring the bound ratelimit puts on its own Redis call.
const readTimeout = 100 * time.Millisecond

// Estimator supplies the approximate open-job count used when no snapshot is published.
// *db.Queries satisfies it via EstimateOpenJobs.
type Estimator interface {
	EstimateOpenJobs(ctx context.Context) (int64, error)
}

// Result is a snapshot plus whether it is the real thing.
//
// Exact distinguishes a published measurement from the degraded fallback. Consumers may
// render the two differently, and none of them should have to guess which they hold.
type Result struct {
	Snapshot
	Exact bool
}

// Store publishes a snapshot for every reader.
func Store(ctx context.Context, c cache.Cache, s Snapshot) error {
	return cache.SetJSON(ctx, c, snapshotKey, s, snapshotTTL)
}

// Load reads the published snapshot.
//
// It never recomputes — that is enforced by this signature, which has no ExactCounter to
// reach for, not by a runtime check. A cold cache, an unreachable backend, and a payload
// left by an older build all degrade the same way: the approximate open-job count, the
// registry-derived figures (which cost nothing and need no cache), and Exact false.
//
// It returns no error. Every caller is rendering a page or a list, and for all of them a
// missing figure beats a failed response, so there is no decision left to delegate.
// Failures are logged here instead.
func Load(ctx context.Context, c cache.Cache, est Estimator) Result {
	// No cache configured is the same situation as a cache with nothing in it, and it
	// is a real deployment: the API can run without Redis, it just never sees a
	// snapshot. Silently, because unlike a failure there is nothing to report.
	if c == nil {
		return Result{Snapshot: degraded(ctx, est), Exact: false}
	}

	readCtx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()

	snapshot, found, err := cache.GetJSON[Snapshot](readCtx, c, snapshotKey)
	if err != nil {
		// Worth a line: an unreachable cache or an undecodable payload is an
		// operational fact, unlike an ordinary miss before the first worker run.
		log.Printf("catalogstats: reading the published snapshot: %v (falling back to the estimate)", err)
	}
	if found {
		return Result{Snapshot: snapshot, Exact: true}
	}

	return Result{Snapshot: degraded(ctx, est), Exact: false}
}

// degraded assembles the best snapshot obtainable without the published one: an
// approximate open-job count and the registry figures, which are pure in-process work.
// The counts that exist only in the database — companies, configured channels — stay
// zero, and Exact false is how a consumer knows not to show them.
func degraded(ctx context.Context, est Estimator) Snapshot {
	s := Snapshot{Sources: Sources(), ATSPlatforms: ATSPlatforms()}

	openJobs, err := est.EstimateOpenJobs(ctx)
	if err != nil {
		log.Printf("catalogstats: estimating open jobs: %v (serving no figure)", err)
		return s
	}
	s.OpenJobs = openJobs
	return s
}
