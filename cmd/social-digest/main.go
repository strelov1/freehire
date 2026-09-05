// Command social-digest publishes the day's ten most-viewed postings to the social
// channels that are configured. One run builds one day's list and exits; schedule it
// once a day, after cmd/rollup-views has produced the day it will read.
//
//	./social-digest -dry-run                 # render the post, send nothing
//	./social-digest -day 2026-09-03          # replay one day
//	./social-digest                          # publish
//
// It ranks on job_daily_views.page_uniques, never on uniques: uniques fuses
// bot-filtered page opens with unfiltered API reads, and crawlers are most of this
// host's traffic, so a list built on it would publish what robots fetched as though
// it were what people liked.
//
// The day is DISCOVERED, not computed from the clock — cmd/rollup-views reads the
// rotated access log, so whether its freshest complete day is yesterday or the day
// before depends on when logrotate runs on the host. A run whose freshest day is more
// than three days old treats the view pipeline as broken and publishes nothing.
//
// A channel with no credentials is not configured and is skipped without error, like
// the rest of this fleet. A channel that fails does not stop another; the run exits
// non-zero if any attempted channel failed.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/engage/socialdigest"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
)

var (
	dryRun = flag.Bool("dry-run", false, "render each configured channel's post and send nothing")
	dayArg = flag.String("day", "", "publish this day (YYYY-MM-DD) instead of the freshest one with data")
)

func main() {
	flag.Parse()
	worker.Main(run)
}

func run() int {
	// Parsed before the pool is opened: a typo in -day should cost nothing and say so
	// immediately, not after a connection and a query.
	requested, err := parseDay(*dayArg)
	if err != nil {
		log.Printf("social-digest: %v", err)
		return 1
	}

	ctx, cfg, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	publishers := configuredPublishers(cfg.DiscordDigestWebhookURL, cfg.FrontendOrigin)
	if len(publishers) == 0 {
		// Not a failure. A deployment with no channel configured is a deployment that
		// has not turned this on, and it should look like nothing rather than like a
		// broken worker every night.
		log.Printf("social-digest: no channel configured, nothing to do")
		return 0
	}

	// FRONTEND_ORIGIN defaults to localhost so that a developer's checkout runs, and
	// every link in the published post is rooted at it. This is the one worker in the
	// fleet whose output strangers read, so an unset origin here means ten dead links
	// posted in public rather than a quiet local oddity. A dry run is exempt: rendering
	// localhost links to a log is exactly what a developer's checkout is for.
	if !*dryRun && !strings.HasPrefix(cfg.FrontendOrigin, "https://") {
		log.Printf("social-digest: FRONTEND_ORIGIN is %q, which would publish links nobody can open; refusing", cfg.FrontendOrigin)
		return 1
	}

	svc := socialdigest.New(socialdigest.NewPostgresRepository(db.New(pool)), time.Now)

	digest, err := svc.Build(ctx, requested)
	if err != nil {
		log.Printf("social-digest: %v", err)
		return 1
	}
	if digest.Empty() {
		// Several different things produce an empty list, and naming only one of them
		// would send an operator hunting the wrong cause. The one that will actually
		// happen first is the third: page_uniques is zero for every row written before
		// migration 0138, so the digest is empty until cmd/rollup-views has run once
		// with the split — the whole window between deploying this and the next night.
		log.Printf("social-digest: nothing to publish for %s. Either no posting cleared the floor of %d page views, "+
			"or every candidate was published within the last %d days, or cmd/rollup-views has not yet run with the "+
			"page/API split and page_uniques is still zero for that day",
			digest.Day.Format(socialdigest.DayLayout), socialdigest.MinPageUniques, socialdigest.QuarantineDays)
		return 0
	}

	if *dryRun {
		return renderOnly(digest, publishers)
	}

	log.Printf("social-digest: publishing %d postings for %s to %d channel(s)",
		len(digest.Items), digest.Day.Format(socialdigest.DayLayout), len(publishers))
	if err := svc.Dispatch(ctx, digest, publishers); err != nil {
		log.Printf("social-digest: %v", err)
		return 1
	}
	return 0
}

// parseDay reads the -day flag. An empty value is the ordinary case and yields the
// zero time, which the service reads as "discover the freshest day with data".
//
// It rejects rather than falls back, because a mistyped day and a day with nothing to
// publish would otherwise look identical in the log — and the flag exists precisely to
// replay a day somebody has a reason to care about.
func parseDay(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	d, err := time.Parse(socialdigest.DayLayout, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("-day must be "+socialdigest.DayLayout+": %w", err)
	}
	return d, nil
}

// configuredPublishers builds one publisher per configured channel. A channel whose
// credential is absent is simply not in the list — there is no "disabled" state to
// represent, and no error to report for a feature nobody turned on.
func configuredPublishers(discordWebhook, origin string) []socialdigest.Publisher {
	var out []socialdigest.Publisher
	if discordWebhook != "" {
		out = append(out, socialdigest.NewDiscordPublisher(discordWebhook, origin))
	}
	return out
}

// renderOnly writes what each channel would send and touches nothing. It prints the
// real payload rather than a summary: what a dry run is for is catching a list that
// reads badly, and a summary of a post cannot read badly.
func renderOnly(digest socialdigest.Digest, publishers []socialdigest.Publisher) int {
	failed := 0
	for _, p := range publishers {
		body, err := p.Render(digest)
		if err != nil {
			log.Printf("social-digest: dry-run: %s: %v", p.Name(), err)
			failed++
			continue
		}
		log.Printf("social-digest: dry-run: %s would send:\n%s", p.Name(), body)
	}
	return worker.ExitCode(failed, 0)
}
