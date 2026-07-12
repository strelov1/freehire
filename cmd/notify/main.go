// Command notify is the standalone filter-subscription notification worker. One
// run does a single MATCH→DELIVER pass: it re-runs each distinct saved-search
// query against the search index, records new matches in the dedup ledger, then
// delivers each subscription's pending matches as one digest over the channel it
// was subscribed on (Telegram and/or email). Run it on a schedule (e.g. cron); it
// processes a bounded batch and exits. It exits non-zero when the run had delivery
// failures so cron can alert.
//
// The feature is optional: with the search backend unconfigured, or with NO
// delivery channel configured (neither the Telegram bot nor SES email), the worker
// logs that it is disabled and exits 0 (nothing to do), so scheduling it before the
// feature is set up does not raise false alarms. A subscription on a channel that
// is not configured this run is soft-skipped, so one channel can run without the
// other.
package main

import (
	"context"
	"log"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/emailnotify"
	"github.com/strelov1/freehire/internal/notify"
	"github.com/strelov1/freehire/internal/search"
	"github.com/strelov1/freehire/internal/telegramnotify"
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

	// Matching needs the search index. Treat absence as "feature disabled" (exit 0),
	// not an error, so an unprovisioned cron stays quiet.
	if cfg.MeiliKey == "" {
		log.Printf("notify: search not configured (MEILI_MASTER_KEY); nothing to do")
		return 0
	}

	// Register every configured delivery channel; the Router dispatches each
	// subscription to its channel and soft-skips one that is not configured.
	router := notify.Router{}
	if cfg.TelegramBotToken != "" {
		router[notify.ChannelTelegram] = telegramnotify.NewNotifier(telegramnotify.NewClient(cfg.TelegramBotToken), cfg.FrontendOrigin)
	}
	if cfg.AWSRegion != "" && cfg.NotifyEmailFrom != "" {
		// A failure to build the SES client disables only the email channel — the
		// email subscriptions soft-skip and retry next pass, while any other
		// configured channel still delivers this run (channels are independent).
		if ses, err := emailnotify.NewClient(ctx, cfg.AWSRegion); err != nil {
			log.Printf("notify: email channel disabled: %v", err)
		} else {
			router[notify.ChannelEmail] = emailnotify.NewNotifier(ses, cfg.NotifyEmailFrom, cfg.FrontendOrigin)
		}
	}
	if len(router) == 0 {
		log.Printf("notify: no delivery channel configured (TELEGRAM_BOT_TOKEN or AWS_REGION+NOTIFY_EMAIL_FROM); nothing to deliver")
		return 0
	}

	searcher := search.NewClient(cfg.MeiliURL, cfg.MeiliKey)
	runner := notify.New(db.New(pool), searcher, router, notify.DefaultConfig())

	stats, err := runner.Run(ctx)
	if err != nil {
		log.Printf("notify: %v", err)
		return 1
	}
	return worker.ExitCode(stats.Failed, 0)
}
