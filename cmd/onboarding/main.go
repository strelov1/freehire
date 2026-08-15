// Command onboarding is the founder signup-sequence worker. One run does a single
// pass over the three steps — greet, ask about the missing alert, talk about the
// project — sending each eligible mail once and recording it. Run it on a schedule
// (hourly is plenty; the first step is the only time-sensitive one) and it exits.
//
// It exits non-zero when any send failed, so the timer's failure handling surfaces
// a broken sender rather than the sequence quietly stopping.
//
// The sequence needs three things configured, and refuses to run without them
// rather than sending something worse than nothing:
//
//	AWS_REGION + NOTIFY_EMAIL_FROM   the SES transport, as for every other mail
//	ONBOARDING_REPLY_TO              the human inbox that answers these letters
//	FRONTEND_ORIGIN                  where the logo, portrait and links resolve
package main

import (
	"context"
	"log"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/emailnotify"
	"github.com/strelov1/freehire/internal/onboarding"
	"github.com/strelov1/freehire/internal/worker"
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

	if cfg.AWSRegion == "" || cfg.NotifyEmailFrom == "" {
		log.Print("onboarding: email transport not configured (AWS_REGION / NOTIFY_EMAIL_FROM) — nothing to do")
		return 0
	}
	// A missing reply address is a misconfiguration, not an absent feature: these
	// mails ask the reader a direct question. Sending them with replies going
	// nowhere is worse than not sending them, so this is a hard stop.
	if cfg.OnboardingReplyTo == "" {
		log.Print("onboarding: ONBOARDING_REPLY_TO is unset — refusing to send mails that ask for a reply")
		return 1
	}

	ses, err := emailnotify.NewClient(ctx, cfg.AWSRegion)
	if err != nil {
		log.Printf("onboarding: ses: %v", err)
		return 1
	}

	mailer := onboarding.NewMailer(ses, cfg.NotifyEmailFrom, cfg.OnboardingReplyTo, cfg.FrontendOrigin)
	runner := onboarding.New(db.New(pool), mailer, onboarding.DefaultConfig())

	stats, err := runner.Run(ctx)
	if err != nil {
		log.Printf("onboarding: %v", err)
		return 1
	}

	var failed int
	for step, n := range stats.Sent {
		if n > 0 {
			log.Printf("onboarding: sent %d %s", n, step)
		}
	}
	for step, n := range stats.Failed {
		if n > 0 {
			log.Printf("onboarding: %d %s sends failed", n, step)
			failed += n
		}
	}
	if failed > 0 {
		return 1
	}
	return 0
}
