// Command rollup-stats computes the daily-activity and Trends & Insights rollups
// on the analytics reader and publishes their results atomically on the primary.
//
// job_daily_stats holds one row per UTC calendar day with that day's `added` (jobs
// created) and `removed` (jobs whose current closed_at falls on it). The insights_*
// tables hold role demand, skill demand, salary bands, and faceted hiring velocity
// that back GET /api/v1/insights/*. Every rollup is a pure function of current jobs
// state — role/skill growth compares open-now against open-as-of a fixed window
// (growthWindowDays), and salary bands are per (currency, period) with a minimum
// sample floor (minSalarySample).
//
// It is a run-once-and-exit worker (cron-scheduled intra-day, ~every few hours, so
// the current day's data stays fresh without a live-overlay query). Computation
// precedes one primary publication transaction, so readers keep the previous snapshot
// and orphaned rows (e.g. a reopened job) vanish in the same step. Re-running is
// safe. It exits non-zero if the rebuild transaction fails, so cron can alert.
package main

import (
	"context"
	"log"
	"time"

	"github.com/strelov1/freehire/internal/ingest/catalogstats"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
	"github.com/strelov1/freehire/internal/search/search"
)

const (
	// growthWindowDays is how far back the role/skill "prior window" open-count looks,
	// so growth = open_now - open_as_of(now - growthWindowDays). A posting is open as of
	// a date from LEAST(posted_at, created_at), not first-seen — see insights.sql's header.
	growthWindowDays = 30
	// minSalarySample is the smallest number of disclosed salaries a (currency, period)
	// band needs to be published — below it the band is suppressed as unreliable and
	// potentially individually identifying.
	minSalarySample = 5
	// minSalaryMonthSample is the smallest sample one month of the salary history needs
	// to be stored. Far above minSalarySample: a band is one figure, while the history
	// is drawn as a line, and a line through five postings a month reads as a trend.
	minSalaryMonthSample = 30
	// skillHistoryRetentionWeeks bounds insights_skill_history: snapshots older than
	// this are pruned every run, so the table stays roughly this many weeks deep.
	skillHistoryRetentionWeeks = 26
	// defaultMinRoleSkillSample is the smallest number of a role's postings a skill
	// must appear in to be published in that role's distribution. Unlike the floors
	// above it is overridable per run (ROLE_SKILL_MIN_SAMPLE), because its correct
	// value is not yet known: the rollup is new, and the intended way to set it is to
	// read the first production run's output. A floor guessed in advance has already
	// gone wrong once here — cmd/social-digest shipped at 10, which selected exactly
	// one posting in a day and had to be lowered to 3 — and there it took a deploy to
	// correct. Fold it back into a constant once a value has proven itself.
	defaultMinRoleSkillSample = 5
)

func main() {
	worker.Main(run)
}

func run() int {
	ctx, cfg, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	analytics, err := worker.AnalyticsPool(ctx, pool)
	if err != nil {
		log.Printf("analytics database: %v", err)
		return 1
	}
	defer analytics.Close()
	// A failed rollup still fails the run, but does not take the snapshots below with
	// it: they read other data, and skipping them lets the public figures expire.
	exit := 0
	if err := rebuildRollups(ctx, analytics, pool); err != nil {
		log.Printf("rebuild rollups: %v", err)
		exit = 1
	}

	// The catalogue-scale snapshot is a separate concern with its own failure mode, so
	// it neither joins the rollups' transaction nor changes this run's exit code.
	done := worker.Stage("catalogue snapshot")
	publish(ctx, cfg.RedisURL, cfg.MeiliURL, cfg.MeiliKey, db.New(analytics), db.New(pool))
	done()

	// So does the per-source snapshot behind the public /sources page, for the same
	// reason and on the same terms.
	done = worker.Stage("source stats")
	rebuildSourceStats(ctx, analytics, pool, cfg.MeiliURL, cfg.MeiliKey)
	done()

	// And the embedding-coverage snapshot behind /embeddings: a scan over every open
	// posting, which is why it is here and never on a request path.
	done = worker.Stage("embed progress")
	rebuildEmbedProgress(ctx, analytics, cfg.RedisURL)
	done()

	return exit
}

// publish measures the catalogue and stores the snapshot every public surface reads.
// Every failure here is logged and swallowed — see the call site.
func publish(ctx context.Context, redisURL, meiliURL, meiliKey string, q, writer *db.Queries) {
	c, closeCache, err := snapshotCache(redisURL)
	if err != nil {
		log.Printf("rollup-stats: catalogue snapshot not published: %v", err)
		return
	}
	defer closeCache()

	// An unreadable channel list costs one stat, not the snapshot. The counts are why
	// this exists; publishing them with a zero channel count beats publishing nothing
	// and leaving every surface on the estimate.
	channels, err := configuredTelegramChannels(ctx, q)
	if err != nil {
		log.Printf("rollup-stats: telegram channel count unavailable, publishing without it: %v", err)
	}

	// Same tolerance as the channel count: no MEILI_MASTER_KEY, or an unreachable
	// Meilisearch, costs this one figure rather than the snapshot. Unlike
	// cmd/rollup-facets, Meilisearch is not this worker's job — the rollups above are —
	// so a missing key is not fatal here. Unlike the channel count, though, a failure
	// here keeps the PREVIOUSLY published figure (resolveUniqueOpenJobs) rather than
	// falling back to zero: a bare zero would misrepresent a transient outage as "no
	// unique jobs," which is a worse-labelled number than a slightly stale real one.
	previous := catalogstats.Load(ctx, c, q).UniqueOpenJobs
	unique := previous
	if meiliKey == "" {
		log.Print("rollup-stats: MEILI_MASTER_KEY not set, keeping the last published unique-jobs count")
	} else {
		// Bounded so a stalled Meilisearch response cannot hold this run open
		// indefinitely — the SDK's own HTTP client has no overall request timeout.
		mctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		fresh, ferr := uniqueOpenJobs(mctx, search.NewClient(meiliURL, meiliKey))
		cancel()
		if ferr != nil {
			log.Printf("rollup-stats: unique-jobs count unavailable, keeping the last published figure: %v", ferr)
		}
		unique = resolveUniqueOpenJobs(previous, fresh, ferr)
	}

	if err := publishSnapshot(ctx, q, c, writer, channels, unique); err != nil {
		log.Printf("rollup-stats: catalogue snapshot not published: %v", err)
		return
	}

	log.Printf("rollup-stats: published the catalogue-scale snapshot")
}
