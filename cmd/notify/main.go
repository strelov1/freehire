// Command notify is the standalone filter-subscription notification worker. One run first
// delivers opted-in inbox digests, independently of search, then does a match-then-deliver
// pass: it re-runs each distinct saved-search query against the search index, records new
// matches in the dedup ledger, and delivers each subscription's pending matches as one digest
// over its channel (Telegram, email, push, webhook). Run it on a schedule; it processes a
// bounded batch and exits non-zero on delivery failures so cron can alert.
//
// With the search backend unconfigured, matching is disabled and inbox digests still run.
// Telegram and email are registered only when their server credential is set (bot token /
// SES region and from address); push and webhook need none and are always registered. A
// subscription on a channel not configured this run (or a push user with no device, or a
// webhook account with no enabled destination) is soft-skipped.
package main

import (
	"context"
	"log"
	"time"

	"github.com/strelov1/freehire/internal/engage/emailnotify"
	"github.com/strelov1/freehire/internal/engage/emailprefs"
	"github.com/strelov1/freehire/internal/engage/inboxdigest"
	"github.com/strelov1/freehire/internal/engage/notify"
	"github.com/strelov1/freehire/internal/engage/pushnotify"
	"github.com/strelov1/freehire/internal/engage/telegramnotify"
	"github.com/strelov1/freehire/internal/engage/webhooknotify"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
	"github.com/strelov1/freehire/internal/search/search"
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

	queries := db.New(pool)
	var inboxSender emailnotify.Sender

	// Register every configured delivery channel; the Router soft-skips an unconfigured one.
	// Push and webhook need no server credential, so they are always registered; their
	// per-recipient soft-skips are handled inside recipient().
	pushStore := pushnotify.NewQueriesStore(queries)
	router := notify.Router{
		notify.ChannelPush:    notify.NewPushNotifier(queries, pushnotify.NewExpoNotifier(pushStore, pushStore, pushStore)),
		notify.ChannelWebhook: webhooknotify.NewNotifier(),
	}
	if cfg.TelegramBotToken != "" {
		router[notify.ChannelTelegram] = telegramnotify.NewNotifier(telegramnotify.NewClient(cfg.TelegramBotToken), cfg.FrontendOrigin)
	}
	if emailnotify.EmailHeld(ctx, cfg.RedisURL, "notify") {
		// A class hold withholds email only: items stay pending, no attempt counted.
		log.Printf("notify: email channel held by a mail pause key, skipping it this run")
	} else if cfg.AWSRegion != "" && cfg.NotifyEmailFrom != "" {
		// A failure to build the SES client disables only the email channel; its subscriptions
		// soft-skip and retry next pass.
		if ses, err := emailnotify.NewClient(ctx, cfg.AWSRegion); err != nil {
			log.Printf("notify: email channel disabled: %v", err)
		} else {
			ses = ses.WithSendingBreaker()
			inboxSender = ses
			// A broken set costs the measurement, never the digest.
			tracked := ses.WithEngagementTracking(cfg.SESEngagementConfigurationSet)
			if err := tracked.CheckEngagementTracking(ctx); err != nil {
				log.Printf("notify: sending untracked: %v", err)
				tracked = ses
			}
			router[notify.ChannelEmail] = emailnotify.NewNotifier(tracked, cfg.NotifyEmailFrom, cfg.FrontendOrigin, emailprefs.NewLinks(cfg.JWTSecret, cfg.FrontendOrigin))
		}
	}
	inboxFailures := 0
	if inboxSender != nil {
		inboxStats, err := inboxdigest.New(inboxdigest.NewDBStore(queries), inboxdigest.NewMailer(cfg.NotifyEmailFrom, cfg.FrontendOrigin, cfg.JWTSecret), inboxSender).Run(ctx, time.Now())
		inboxFailures = inboxStats.Failed
		log.Printf("inbox-digest: sent=%d failed=%d skipped=%d", inboxStats.Sent, inboxStats.Failed, inboxStats.Skipped)
		if err != nil {
			log.Printf("inbox-digest: %v", err)
			inboxFailures++
		}
	}
	// Inbox mail does not depend on the job search index.
	if cfg.MeiliKey == "" {
		log.Printf("notify: search not configured (MEILI_MASTER_KEY); skipping saved-search matching")
		return worker.ExitCode(inboxFailures, 0)
	}
	searcher := search.NewClient(cfg.MeiliURL, cfg.MeiliKey)
	runner := notify.New(queries, searcher, router, notify.DefaultConfig())

	stats, err := runner.Run(ctx)
	if err != nil {
		log.Printf("notify: %v", err)
		return 1
	}
	// One failed saved-search query is isolated and must not fail the pass; every query
	// failing means the index or its credential is gone.
	if stats.MatchingCollapsed() {
		log.Printf("notify: all %d saved-search queries failed — nothing was matched this pass", stats.Queries)
		return 1
	}
	return worker.ExitCode(stats.Failed+inboxFailures, 0)
}
