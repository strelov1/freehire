// Command limit-nudge-mail nudges every free-tier account that ran into a daily plan
// ceiling with a one-time, personal email pointing at Pro — see
// internal/engage/limitnudge for why this touchpoint exists. One run does a single
// pass and exits; run it hourly, independent of cmd/billing-sync's own cadence — this
// is not a billing reconciliation.
//
// It exits non-zero when any send failed, so the timer's failure handling surfaces a
// broken sender rather than the pass quietly nudging nobody.
//
// Needs:
//
//	AWS_REGION + NOTIFY_EMAIL_FROM   the SES transport, as for every other mail. Missing
//	                                  either is a no-op that never opens the pool — the
//	                                  gate runs BEFORE worker.Bootstrap, same as
//	                                  cmd/pro-welcome-mail.
//	ONBOARDING_REPLY_TO               the same human inbox the founder's other letters
//	                                  already answer from. Missing this is a hard
//	                                  failure (exit 1): this letter asks for a reply.
//	FRONTEND_ORIGIN, JWT_SECRET       where the portrait resolves and the unsubscribe
//	                                  link is signed
//
// LIMIT_NUDGE_WINDOW_DAYS (default 3) is how many days back a hit still counts.
// LIMIT_NUDGE_MAX_PER_RUN (default 200) bounds one pass.
package main

import (
	"context"
	"log"

	"github.com/strelov1/freehire/internal/engage/emailnotify"
	"github.com/strelov1/freehire/internal/engage/emailprefs"
	"github.com/strelov1/freehire/internal/engage/limitnudge"
	"github.com/strelov1/freehire/internal/platform/config"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
)

func main() {
	worker.Main(run)
}

func run() int {
	// Gate before Bootstrap, not after — the same reason cmd/pro-welcome-mail does:
	// with no mail transport there is nothing to send, and an unconfigured deployment
	// must run without touching the database at all.
	cfg := config.Load()
	if cfg.AWSRegion == "" || cfg.NotifyEmailFrom == "" {
		log.Print("limit-nudge-mail: email transport not configured (AWS_REGION / NOTIFY_EMAIL_FROM) — nothing to do")
		return 0
	}
	if cfg.OnboardingReplyTo == "" {
		log.Print("limit-nudge-mail: ONBOARDING_REPLY_TO is unset — refusing to send a letter that asks for a reply")
		return 1
	}

	windowDays, err := worker.EnvInt32("LIMIT_NUDGE_WINDOW_DAYS", 3)
	if err != nil {
		log.Printf("limit-nudge-mail: %v", err)
		return 1
	}
	maxRows, err := worker.EnvInt32("LIMIT_NUDGE_MAX_PER_RUN", 200)
	if err != nil {
		log.Printf("limit-nudge-mail: %v", err)
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
		log.Printf("limit-nudge-mail: ses: %v", err)
		return 1
	}

	mailer := limitnudge.NewMailer(ses, cfg.NotifyEmailFrom, cfg.OnboardingReplyTo, cfg.FrontendOrigin,
		emailprefs.NewLinks(cfg.JWTSecret, cfg.FrontendOrigin))
	runner := limitnudge.New(db.New(pool), mailer, windowDays, maxRows)

	stats, err := runner.Run(ctx)
	if err != nil {
		log.Printf("limit-nudge-mail: %v", err)
		return 1
	}

	if stats.Sent > 0 {
		log.Printf("limit-nudge-mail: nudged %d free-tier account(s)", stats.Sent)
	}
	if stats.Failed > 0 {
		log.Printf("limit-nudge-mail: %d nudge send(s) failed, will retry next run", stats.Failed)
		return 1
	}
	return 0
}
