// Command notify is the standalone filter-subscription notification worker. One
// run does a single MATCH→DELIVER pass: it re-runs each distinct saved-search
// query against the search index, records new matches in the dedup ledger, then
// delivers each subscription's pending matches as one digest (Telegram today).
// Run it on a schedule (e.g. cron); it processes a bounded batch and exits. It
// exits non-zero when the run had delivery failures so cron can alert.
//
// The feature is optional: with the Telegram bot or search backend unconfigured
// the worker logs that it is disabled and exits 0 (nothing to do), so scheduling
// it before the feature is set up does not raise false alarms.
package main

import (
	"context"
	"log"
	"os"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/notify"
	"github.com/strelov1/freehire/internal/search"
	"github.com/strelov1/freehire/internal/telegramnotify"
	"github.com/strelov1/freehire/internal/worker"
)

func main() {
	os.Exit(run())
}

func run() int {
	ctx, cfg, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	// Both backends are required to do any work. Treat absence as "feature
	// disabled" (exit 0), not an error, so an unprovisioned cron stays quiet.
	if cfg.MeiliKey == "" {
		log.Printf("notify: search not configured (MEILI_MASTER_KEY); nothing to do")
		return 0
	}
	if cfg.TelegramBotToken == "" {
		log.Printf("notify: telegram bot not configured (TELEGRAM_BOT_TOKEN); nothing to deliver")
		return 0
	}

	searcher := search.NewClient(cfg.MeiliURL, cfg.MeiliKey)
	notifier := telegramnotify.NewNotifier(telegramnotify.NewClient(cfg.TelegramBotToken), cfg.FrontendOrigin)
	runner := notify.New(db.New(pool), searcher, notifier, notify.DefaultConfig())

	stats, err := runner.Run(ctx)
	if err != nil {
		log.Printf("notify: %v", err)
		return 1
	}
	return worker.ExitCode(stats.Failed, 0)
}
