// Command pro-welcome-mail welcomes every account newly entitled to a paying tier (Pro or
// Ultra) with a one-time, personal email. One run does a single pass and exits non-zero when
// any send failed; run it every 10 minutes, independent of cmd/billing-sync's hourly cadence.
//
// It is its own worker, not a pass inside cmd/billing-sync, for layering: identity/billing must
// not import engage, which this mails through.
//
// Needs:
//
//	AWS_REGION + NOTIFY_EMAIL_FROM   the SES transport. Missing either is a no-op that never
//	                                  opens the pool (the gate runs before worker.Bootstrap).
//	ONBOARDING_REPLY_TO              the founder sequence's reply inbox. Missing it is a hard
//	                                  failure (exit 1): this letter asks for a reply.
//	FRONTEND_ORIGIN, JWT_SECRET       where the logo/portrait resolve and the
//	                                  unsubscribe link is signed
//
// PRO_WELCOME_MAX_PER_RUN (default 200) bounds one pass.
package main

import (
	"context"
	"log"

	"github.com/strelov1/freehire/internal/engage/emailnotify"
	"github.com/strelov1/freehire/internal/engage/emailprefs"
	"github.com/strelov1/freehire/internal/engage/prowelcome"
	"github.com/strelov1/freehire/internal/platform/config"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
)

func main() {
	worker.Main(run, emailnotify.HoldKeys("pro-welcome-mail")...)
}

func run() int {
	// Gate before Bootstrap: an unconfigured deployment must not touch the database.
	cfg := config.Load()
	if cfg.AWSRegion == "" || cfg.NotifyEmailFrom == "" {
		log.Print("pro-welcome-mail: email transport not configured (AWS_REGION / NOTIFY_EMAIL_FROM) — nothing to do")
		return worker.Skip("AWS_REGION+NOTIFY_EMAIL_FROM")
	}
	// A missing reply address is a hard stop, as in cmd/onboarding: this letter asks for a reply.
	if cfg.OnboardingReplyTo == "" {
		log.Print("pro-welcome-mail: ONBOARDING_REPLY_TO is unset — refusing to send a letter that asks for a reply")
		return 1
	}

	maxRows, err := worker.EnvInt32("PRO_WELCOME_MAX_PER_RUN", 200)
	if err != nil {
		log.Printf("pro-welcome-mail: %v", err)
		return 1
	}

	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	ses, err := emailnotify.NewClient(ctx, cfg.AWSRegion)
	if err != nil {
		log.Printf("pro-welcome-mail: ses: %v", err)
		return 1
	}
	ses = ses.WithSendingBreaker()

	mailer := prowelcome.NewMailer(ses, cfg.NotifyEmailFrom, cfg.OnboardingReplyTo, cfg.FrontendOrigin,
		emailprefs.NewLinks(cfg.JWTSecret, cfg.FrontendOrigin))
	runner := prowelcome.New(db.New(pool), mailer, maxRows)

	stats, err := runner.Run(ctx)
	if err != nil {
		log.Printf("pro-welcome-mail: %v", err)
		return 1
	}

	if stats.Sent > 0 {
		log.Printf("pro-welcome-mail: welcomed %d newly-paying account(s)", stats.Sent)
	}
	if stats.Failed > 0 {
		log.Printf("pro-welcome-mail: %d welcome send(s) failed, will retry next run", stats.Failed)
		return 1
	}
	return 0
}
