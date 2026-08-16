// Command broadcast sends one campaign to the whole audience: one letter, once,
// on a date someone picked.
//
//	./broadcast -campaign ph-heads-up -dry-run   # how many would receive it
//	./broadcast -campaign ph-heads-up            # send one capped batch
//	./broadcast -campaign ph-heads-up -to me@example.com   # send only to me
//
// It is not a timer worker in the usual sense. It is scheduled once, for the day of
// the announcement, and then it is done — which is also why it refuses to guess:
// without -campaign it lists what exists and exits non-zero rather than picking one.
//
// -dry-run exists because a campaign is irreversible and reaches everyone. Seeing
// the number first is cheap; unsending is impossible.
package main

import (
	"context"
	"flag"
	"log"
	"strings"

	"github.com/strelov1/freehire/internal/broadcast"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/emailnotify"
	"github.com/strelov1/freehire/internal/worker"
)

var (
	campaignName = flag.String("campaign", "", "which campaign to send (required)")
	dryRun       = flag.Bool("dry-run", false, "report the audience size and send nothing")
	only         = flag.String("to", "", "send only to this address, and do not touch the ledger")
	maxPerRun    = flag.Int("max", broadcast.DefaultMaxPerRun, "cap on one run")
)

func main() {
	flag.Parse()
	worker.Main(run)
}

func run() int {
	ctx, cfg, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	if *campaignName == "" {
		log.Printf("broadcast: -campaign is required; known campaigns: %s",
			strings.Join(broadcast.Names(), ", "))
		return 1
	}
	campaign, ok := broadcast.Lookup(*campaignName)
	if !ok {
		log.Printf("broadcast: unknown campaign %q; known: %s",
			*campaignName, strings.Join(broadcast.Names(), ", "))
		return 1
	}

	if cfg.AWSRegion == "" || cfg.NotifyEmailFrom == "" || cfg.OnboardingReplyTo == "" {
		log.Print("broadcast: AWS_REGION / NOTIFY_EMAIL_FROM / ONBOARDING_REPLY_TO must all be set")
		return 1
	}

	ses, err := emailnotify.NewClient(ctx, cfg.AWSRegion)
	if err != nil {
		log.Printf("broadcast: ses: %v", err)
		return 1
	}
	mailer := broadcast.NewMailer(ses, cfg.NotifyEmailFrom, cfg.OnboardingReplyTo, cfg.FrontendOrigin)
	runner := broadcast.New(db.New(pool), mailer, int32(*maxPerRun))

	// A single address, for looking at the letter in a real client. The ledger is
	// left alone: inspecting a campaign must not consume anybody's one delivery.
	if *only != "" {
		if err := mailer.Send(ctx, campaign, *only); err != nil {
			log.Printf("broadcast: %v", err)
			return 1
		}
		log.Printf("broadcast: sent %s to %s (ledger untouched)", campaign.Name, *only)
		return 0
	}

	if *dryRun {
		pending, err := runner.Pending(ctx, campaign)
		if err != nil {
			log.Printf("broadcast: %v", err)
			return 1
		}
		log.Printf("broadcast: %s would go to %d people (cap %d per run)", campaign.Name, pending, *maxPerRun)
		return 0
	}

	stats, err := runner.Run(ctx, campaign)
	if err != nil {
		log.Printf("broadcast: %v", err)
		return 1
	}
	log.Printf("broadcast: %s sent=%d failed=%d remaining=%d",
		campaign.Name, stats.Sent, stats.Failed, stats.Remaining)
	if stats.Failed > 0 {
		return 1
	}
	return 0
}
