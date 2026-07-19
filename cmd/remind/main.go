// Command remind is the standalone saved-job reminder worker. One run does a
// single firing pass: it leases the due, pending reminders and delivers each as a
// one-shot nudge over the channels it was scheduled on (Telegram and/or email),
// re-checking that the job is still open and still saved-but-unapplied immediately
// before sending. Run it on a schedule (e.g. cron, ~every 15 min); it processes a
// bounded batch and exits. It exits non-zero when the run had delivery failures so
// cron can alert.
//
// The feature is optional: with NO delivery channel configured (neither the
// Telegram bot nor SES email), the worker logs that it is disabled and exits 0
// (nothing to deliver), so scheduling it before the feature is set up raises no
// false alarms. A reminder whose channel is not configured this run is soft-skipped
// and retried next pass, so one channel can run without the other.
package main

import (
	"context"
	"log"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/emailnotify"
	"github.com/strelov1/freehire/internal/notify"
	"github.com/strelov1/freehire/internal/reminder"
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
	// reminder to its channel and soft-skips one that is not configured. Channel
	// names are shared with subscriptions via the notify vocabulary.
	router := reminder.Router{}
	if cfg.TelegramBotToken != "" {
		router[notify.ChannelTelegram] = reminder.NewTelegramNotifier(telegramnotify.NewClient(cfg.TelegramBotToken), cfg.FrontendOrigin)
	}
	if cfg.AWSRegion != "" && cfg.NotifyEmailFrom != "" {
		// A failure to build the SES client disables only the email channel — email
		// reminders soft-skip and retry next pass while any other configured channel
		// still delivers this run (channels are independent).
		if ses, err := emailnotify.NewClient(ctx, cfg.AWSRegion); err != nil {
			log.Printf("remind: email channel disabled: %v", err)
		} else {
			router[notify.ChannelEmail] = reminder.NewEmailNotifier(ses, cfg.NotifyEmailFrom, cfg.FrontendOrigin)
		}
	}
	if len(router) == 0 {
		log.Printf("remind: no delivery channel configured (TELEGRAM_BOT_TOKEN or AWS_REGION+NOTIFY_EMAIL_FROM); nothing to deliver")
		return 0
	}

	runner := reminder.NewRunner(db.New(pool), router, reminder.DefaultConfig())

	stats, err := runner.Run(ctx)
	if err != nil {
		log.Printf("remind: %v", err)
		return 1
	}
	return worker.ExitCode(stats.Failed, 0)
}
