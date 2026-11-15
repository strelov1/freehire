// Command social-digest publishes the day's ten most-viewed postings, rotating through themed
// tops by weekday, to the social channels that are configured. One run builds one day's list and
// exits; schedule it once a day, after cmd/rollup-views has produced the day it will read.
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

	"github.com/strelov1/freehire/internal/engage/linkedinauth"
	"github.com/strelov1/freehire/internal/engage/socialdigest"
	"github.com/strelov1/freehire/internal/platform/config"
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
	// Parsed before the pool is opened so a typo in -day fails immediately.
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

	// The LinkedIn channel reads its credential from the database on every publish, so a renewal
	// is picked up without a restart.
	publishers := configuredPublishers(cfg, linkedinauth.NewPostgresStore(db.New(pool)))
	if len(publishers) == 0 {
		// Not a failure: no channel configured means this is not turned on, and it should not look
		// like a broken worker every night.
		log.Printf("social-digest: no channel configured, nothing to do")
		return 0
	}

	// FRONTEND_ORIGIN defaults to localhost, and every link in the post is rooted at it. This
	// worker's output is read by strangers, so an unset origin means dead links posted in public;
	// a dry run is exempt.
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
		// A day nothing clears the floor on never gets here: Build reports it as an error.
		log.Printf("social-digest: nothing to publish for %s: every posting above the floor of %d page views was published within the last %d days",
			digest.Day.Format(socialdigest.DayLayout), socialdigest.MinPageUniques, socialdigest.QuarantineDays)
		return 0
	}

	if *dryRun {
		return renderOnly(digest, publishers)
	}

	if err := svc.Dispatch(ctx, digest, publishers); err != nil {
		log.Printf("social-digest: %v", err)
		return 1
	}
	return 0
}

// parseDay reads the -day flag. An empty value yields the zero time, which the service reads
// as "discover the freshest day with data". It rejects rather than falls back, so a mistyped
// day is not mistaken for a day with nothing to publish.
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

// configuredPublishers builds one publisher per configured channel; an unconfigured channel is
// simply absent. Discord is configured by its static webhook URL. LinkedIn's access token
// EXPIRES and lives in the database, so configuration means the application and company page,
// never the token: requiring a stored token would silently drop the channel from the run that
// should report it needs signing in.
func configuredPublishers(cfg config.Settings, tokens socialdigest.TokenSource) []socialdigest.Publisher {
	var out []socialdigest.Publisher
	if cfg.DiscordDigestWebhookURL != "" {
		out = append(out, socialdigest.NewDiscordPublisher(cfg.DiscordDigestWebhookURL, cfg.FrontendOrigin))
	}
	if cfg.LinkedInDigestConfigured() && tokens != nil {
		out = append(out, socialdigest.NewLinkedInPublisher(tokens, cfg.LinkedInOrganizationURN(), cfg.FrontendOrigin))
	}
	return out
}

// renderOnly writes what each channel would send and touches nothing. It prints the real
// payload, since a dry run exists to catch a list that reads badly.
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
