// Command pro-welcome-mail welcomes every account newly entitled to a paying tier (Pro or
// Ultra) with a one-time, personal email — see openspec/changes/welcome-pro-subscribers.
// One run does a single pass and exits; run it every 10 minutes, independent of
// cmd/billing-sync/cmd/discord-sync's hourly cadence, so a purchase confirmation does not
// read as broken by arriving an hour late.
//
// It exits non-zero when any send failed, so the timer's failure handling surfaces a
// broken sender rather than the pass quietly welcoming nobody.
//
// It is its own worker rather than a pass inside cmd/billing-sync for the same layering
// reason cmd/discord-sync is: internal/identity/billing is layer identity, this mails
// through internal/engage/prowelcome which is layer engage, and identity must not import
// engage.
//
// Needs:
//
//	AWS_REGION + NOTIFY_EMAIL_FROM   the SES transport, as for every other mail. Missing
//	                                  either is a no-op that never opens the pool — the
//	                                  gate runs BEFORE worker.Bootstrap, same as
//	                                  cmd/discord-sync, not after.
//	ONBOARDING_REPLY_TO              the same human inbox the founder signup sequence
//	                                  already answers from — one reply address, not a
//	                                  second one that could disagree with it. Missing this
//	                                  is a hard failure (exit 1), not a soft no-op: this
//	                                  letter asks for a reply, and sending it with nowhere
//	                                  for that reply to land is worse than not sending it.
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
	worker.Main(run)
}

func run() int {
	// Gate before Bootstrap, not after — the same reason cmd/discord-sync does: with no
	// mail transport there is nothing to send, and an unconfigured deployment must run
	// without touching the database at all.
	cfg := config.Load()
	if cfg.AWSRegion == "" || cfg.NotifyEmailFrom == "" {
		log.Print("pro-welcome-mail: email transport not configured (AWS_REGION / NOTIFY_EMAIL_FROM) — nothing to do")
		return 0
	}
	// A missing reply address is a misconfiguration, not an absent feature: this letter
	// asks for a reply. Sending it with replies going nowhere is worse than not sending
	// it — the same hard stop cmd/onboarding takes for the sequence it shares the inbox
	// with.
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
