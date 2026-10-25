// Command buddy-notify tells interview-buddies members about messages they have left
// unread. Schedule it every 5 minutes.
//
// A conversation side is due once the oldest message the other member wrote since its read
// cursor is five minutes old, and is not due again until it has read and a newer message
// ages the same way. The run CLAIMS each notice before sending it and sends only the ones
// it won; when every channel fails the claim is released so the next run retries.
//
// The notice carries no message text and no account detail — only that a message or
// invitation is waiting, with a link to the chat.
//
// Needs DATABASE_URL, AWS_REGION + NOTIFY_EMAIL_FROM (the SES transport), FRONTEND_ORIGIN
// and JWT_SECRET (the unsubscribe link). TELEGRAM_BOT_TOKEN is optional. Without the mail
// transport it is a no-op that never opens the pool — it must not claim notices it cannot
// deliver. BUDDY_NOTIFY_MAX_PER_RUN (default 500) bounds one run.
package main

import (
	"context"
	"log"
	"time"

	"github.com/strelov1/freehire/internal/engage/buddy"
	"github.com/strelov1/freehire/internal/engage/emailnotify"
	"github.com/strelov1/freehire/internal/engage/emailprefs"
	"github.com/strelov1/freehire/internal/engage/telegramnotify"
	"github.com/strelov1/freehire/internal/platform/config"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
)

func main() { worker.Main(run) }

func run() int {
	cfg := config.Load()
	if cfg.AWSRegion == "" || cfg.NotifyEmailFrom == "" {
		log.Print("buddy-notify: email transport not configured (AWS_REGION / NOTIFY_EMAIL_FROM) — nothing to do")
		return 0
	}
	maxRows, err := worker.EnvInt32("BUDDY_NOTIFY_MAX_PER_RUN", buddy.DefaultNotifyMaxPerRun)
	if err != nil {
		log.Printf("buddy-notify: %v", err)
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
		log.Printf("buddy-notify: ses: %v", err)
		return 1
	}
	queries := db.New(pool)
	nc := buddy.NotifierConfig{
		Email:     ses,
		From:      cfg.NotifyEmailFrom,
		Links:     emailprefs.NewLinks(cfg.JWTSecret, cfg.FrontendOrigin),
		Silencer:  emailprefs.NewService(queries, cfg.JWTSecret),
		Origin:    cfg.FrontendOrigin,
		MaxPerRun: maxRows,
	}
	if cfg.TelegramBotToken != "" {
		nc.Telegram = telegramnotify.NewClient(cfg.TelegramBotToken)
	}

	res, err := buddy.NewNotifier(queries, nc).NotifyUnread(ctx, time.Now())
	if err != nil {
		log.Printf("buddy-notify: %v", err)
		return 1
	}
	log.Printf("buddy-notify: notified=%d failed=%d", res.Notified, res.Failed)
	if res.Failed > 0 {
		return 1
	}
	return 0
}
