// Command nudge is the standalone lifecycle-notification worker. One run re-scans tracked
// applications for two conditions (gone silent past the stage's threshold, or a stage_set moved
// one into `interview`), records new candidates, then delivers each pending nudge over the
// channels the account's shared notification rule configures (Telegram, email, push). Run it on
// a schedule; it processes a bounded batch and exits non-zero on delivery failures.
//
// Telegram and email are registered only when TELEGRAM_BOT_TOKEN and AWS_REGION +
// NOTIFY_EMAIL_FROM are set; push needs no credential and is always registered. A nudge whose
// channel is not configured this run is soft-skipped and retried next pass.
package main

import (
	"context"
	"log"

	"github.com/strelov1/freehire/internal/engage/emailnotify"
	"github.com/strelov1/freehire/internal/engage/emailprefs"
	"github.com/strelov1/freehire/internal/engage/notify"
	"github.com/strelov1/freehire/internal/engage/nudge"
	"github.com/strelov1/freehire/internal/engage/pushnotify"
	"github.com/strelov1/freehire/internal/engage/telegramnotify"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
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

	// Register every configured delivery channel; the Router soft-skips an unconfigured one.
	router := nudge.Router{}
	if cfg.TelegramBotToken != "" {
		router[notify.ChannelTelegram] = nudge.NewTelegramNotifier(telegramnotify.NewClient(cfg.TelegramBotToken), cfg.FrontendOrigin)
	}
	if emailnotify.EmailHeld(ctx, cfg.RedisURL, "nudge") {
		// A class hold withholds email only: items stay pending, no attempt counted.
		log.Printf("nudge: email channel held by a mail pause key, skipping it this run")
	} else if cfg.AWSRegion != "" && cfg.NotifyEmailFrom != "" {
		// A failure to build the SES client disables only the email channel; nudges retry next pass.
		if ses, err := emailnotify.NewClient(ctx, cfg.AWSRegion); err != nil {
			log.Printf("nudge: email channel disabled: %v", err)
		} else {
			ses = ses.WithSendingBreaker()
			router[notify.ChannelEmail] = nudge.NewEmailNotifier(ses, cfg.NotifyEmailFrom, cfg.FrontendOrigin, emailprefs.NewLinks(cfg.JWTSecret, cfg.FrontendOrigin))
		}
	}
	// Push needs no server-held credential, so it is always registered.
	pushStore := pushnotify.NewQueriesStore(queries)
	router[notify.ChannelPush] = nudge.NewPushNotifier(queries, pushnotify.NewExpoNotifier(pushStore, pushStore, pushStore))

	runner := nudge.New(queries, router, nudge.DefaultConfig())

	stats, err := runner.Run(ctx)
	if err != nil {
		log.Printf("nudge: %v", err)
		return 1
	}
	return worker.ExitCode(stats.Failed, 0)
}
