// Command onboarding is the founder signup-sequence worker. One run does a single pass over
// the steps (greet, filter panel, missing alert, extension, the project), sending each eligible
// mail once and recording it. Run it hourly; it exits non-zero when any send failed.
//
// It refuses to run without three settings rather than send something worse than nothing:
//
//	AWS_REGION + NOTIFY_EMAIL_FROM   the SES transport, as for every other mail
//	ONBOARDING_REPLY_TO              the human inbox that answers these letters
//	FRONTEND_ORIGIN                  where the logo, portrait and links resolve
package main

import (
	"context"
	"flag"
	"log"

	"github.com/strelov1/freehire/internal/engage/emailnotify"
	"github.com/strelov1/freehire/internal/engage/emailprefs"
	"github.com/strelov1/freehire/internal/engage/onboarding"
	"github.com/strelov1/freehire/internal/platform/config"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
)

// The -to flag sends one mail to one address and exits, touching neither the candidate queries
// nor the ledger, so a copy change can be seen in a real inbox.
//
//	./onboarding -to you@example.com            # the welcome mail
//	./onboarding -to you@example.com -step open_source
var (
	sendTo   = flag.String("to", "", "send one mail to this address and exit (no database, no ledger)")
	sendUser = flag.Int64("to-user", 0, "the account id -to belongs to; required with -to, because the mail carries that account's unsubscribe link")
	sendStep = flag.String("step", string(onboarding.StepWelcome), "which step -to sends: welcome | advanced_search | no_alert | extension | open_source | tailor | buddies")
)

func main() {
	flag.Parse()
	if *sendTo != "" {
		worker.Main(sendOne)
		return
	}
	worker.Main(run, emailnotify.HoldKeys("onboarding")...)
}

// sendOne delivers a single mail for inspection, sharing the transport and refusal checks with
// the real pass so the inbox shows what the sequence would send.
func sendOne() int {
	ctx := context.Background()
	cfg := config.Load()
	if cfg.AWSRegion == "" || cfg.NotifyEmailFrom == "" || cfg.OnboardingReplyTo == "" {
		log.Print("onboarding: AWS_REGION / NOTIFY_EMAIL_FROM / ONBOARDING_REPLY_TO must all be set")
		return 1
	}

	ses, err := emailnotify.NewClient(ctx, cfg.AWSRegion)
	if err != nil {
		log.Printf("onboarding: ses: %v", err)
		return 1
	}

	mailer := onboarding.NewMailer(ses, cfg.NotifyEmailFrom, cfg.OnboardingReplyTo, cfg.FrontendOrigin, emailprefs.NewLinks(cfg.JWTSecret, cfg.FrontendOrigin))
	// The mail carries a real unsubscribe link, so -to needs the account.
	if *sendUser <= 0 {
		log.Print("onboarding: -to also needs -to-user, the account id the address belongs to")
		return 2
	}
	if err := mailer.Send(ctx, onboarding.Step(*sendStep), *sendUser, *sendTo); err != nil {
		log.Printf("onboarding: %v", err)
		return 1
	}
	log.Printf("onboarding: sent %s to %s", *sendStep, *sendTo)
	return 0
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
	// A missing reply address is a misconfiguration: these mails ask a direct question, so
	// sending them with replies going nowhere is a hard stop.
	if cfg.OnboardingReplyTo == "" {
		log.Print("onboarding: ONBOARDING_REPLY_TO is unset — refusing to send mails that ask for a reply")
		return 1
	}

	ses, err := emailnotify.NewClient(ctx, cfg.AWSRegion)
	if err != nil {
		log.Printf("onboarding: ses: %v", err)
		return 1
	}
	ses = ses.WithSendingBreaker()

	mailer := onboarding.NewMailer(ses, cfg.NotifyEmailFrom, cfg.OnboardingReplyTo, cfg.FrontendOrigin, emailprefs.NewLinks(cfg.JWTSecret, cfg.FrontendOrigin))
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
