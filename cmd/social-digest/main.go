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
	"log"
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
	var requested time.Time
	if *dayArg != "" {
		d, err := time.Parse("2006-01-02", *dayArg)
		if err != nil {
			log.Printf("social-digest: -day must be YYYY-MM-DD: %v", err)
			return 1
		}
		requested = d
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

	svc := socialdigest.New(socialdigest.NewPostgresRepository(db.New(pool)), time.Now)

	digest, err := svc.Build(ctx, requested)
	if err != nil {
		log.Printf("social-digest: %v", err)
		return 1
	}
	if digest.Empty() {
		log.Printf("social-digest: %s had nothing above the floor of %d page views; publishing nothing",
			digest.Day.Format("2006-01-02"), socialdigest.MinPageUniques)
		return 0
	}

	if *dryRun {
		return renderOnly(digest, publishers)
	}

	log.Printf("social-digest: publishing %d postings for %s to %d channel(s)",
		len(digest.Items), digest.Day.Format("2006-01-02"), len(publishers))
	if err := svc.Dispatch(ctx, digest, publishers); err != nil {
		log.Printf("social-digest: %v", err)
		return 1
	}
	return 0
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
