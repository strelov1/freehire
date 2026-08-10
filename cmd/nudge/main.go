// Command nudge is the standalone lifecycle-notification worker. One run does a
// single MATCH→DELIVER pass: it re-scans tracked applications for two conditions —
// gone silent past the stage's tolerated threshold, or a stage_set moved one into
// `interview` — records new candidates, then delivers each pending nudge over the
// channels the account's shared notification rule configures (Telegram and/or
// email). Run it on a schedule (e.g. cron, ~every 30-60 min); it processes a
// bounded batch and exits. It exits non-zero when the run had delivery failures so
// cron can alert.
//
// The feature is optional: with NO delivery channel configured (neither the
// Telegram bot nor SES email), the worker logs that it is disabled and exits 0
// (nothing to deliver). A nudge whose channel is not configured this run is
// soft-skipped and retried next pass, so one channel can run without the other.
package main

import (
	"context"
	"log"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/emailnotify"
	"github.com/strelov1/freehire/internal/notify"
	"github.com/strelov1/freehire/internal/nudge"
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

	// Register every configured delivery channel; the Router dispatches each
	// nudge to its channel and soft-skips one that is not configured. Channel
	// names are shared with reminders and subscriptions via the notify vocabulary.
	router := nudge.Router{}
	if cfg.TelegramBotToken != "" {
		router[notify.ChannelTelegram] = nudge.NewTelegramNotifier(telegramnotify.NewClient(cfg.TelegramBotToken), cfg.FrontendOrigin)
	}
	if cfg.AWSRegion != "" && cfg.NotifyEmailFrom != "" {
		// A failure to build the SES client disables only the email channel — email
		// nudges soft-skip and retry next pass while any other configured channel
		// still delivers this run (channels are independent).
		if ses, err := emailnotify.NewClient(ctx, cfg.AWSRegion); err != nil {
			log.Printf("nudge: email channel disabled: %v", err)
		} else {
			router[notify.ChannelEmail] = nudge.NewEmailNotifier(ses, cfg.NotifyEmailFrom, cfg.FrontendOrigin)
		}
	}
	if len(router) == 0 {
		log.Printf("nudge: no delivery channel configured (TELEGRAM_BOT_TOKEN or AWS_REGION+NOTIFY_EMAIL_FROM); nothing to deliver")
		return 0
	}

	runner := nudge.New(db.New(pool), router, nudge.DefaultConfig())

	stats, err := runner.Run(ctx)
	if err != nil {
		log.Printf("nudge: %v", err)
		return 1
	}
	return worker.ExitCode(stats.Failed, 0)
}
