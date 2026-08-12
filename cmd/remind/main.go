// Command remind is the standalone saved-job reminder worker. One run does a
// single firing pass: it leases the due, pending reminders and delivers each as a
// one-shot nudge over the channels it was scheduled on (Telegram, email, and/or
// push), re-checking that the job is still open and still saved-but-unapplied
// immediately before sending. Run it on a schedule (e.g. cron, ~every 15 min); it
// processes a bounded batch and exits. It exits non-zero when the run had delivery
// failures so cron can alert.
//
// Telegram and email register only when their credential is configured
// (TELEGRAM_BOT_TOKEN, or AWS_REGION+NOTIFY_EMAIL_FROM); push registers
// unconditionally, since Expo's relay needs no server-held credential (see
// add-push-notification-channel design decision #6). A reminder whose channel is
// not configured, or whose recipient has no usable destination on that channel
// (unlinked Telegram, no registered push device), is soft-skipped and retried next
// pass, so one channel's absence never blocks another.
package main

import (
	"context"
	"log"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/emailnotify"
	"github.com/strelov1/freehire/internal/notify"
	"github.com/strelov1/freehire/internal/pushnotify"
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

	queries := db.New(pool)

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
	// Push needs no server-held credential (Expo's relay holds the APNs/FCM
	// credential), so it registers unconditionally — no "feature globally off"
	// state to represent for it.
	pushStore := pushnotify.NewQueriesStore(queries)
	router[notify.ChannelPush] = reminder.NewPushNotifier(queries, pushnotify.NewExpoNotifier(pushStore, pushStore, pushStore))

	runner := reminder.NewRunner(queries, router, reminder.DefaultConfig())

	stats, err := runner.Run(ctx)
	if err != nil {
		log.Printf("remind: %v", err)
		return 1
	}
	return worker.ExitCode(stats.Failed, 0)
}
