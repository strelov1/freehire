// Command message-notify tells people about conversation messages they have left unread,
// for every messaging kind (today: interview buddies). Schedule it every 5 minutes.
//
// A participant is due once the oldest message another wrote since its read cursor is five
// minutes old, and is not due again until it has read and a newer message ages the same way. The run CLAIMS each notice before sending it and sends only the ones
// it won; when every channel fails the claim is released so the next run retries.
//
// The notice carries no message text and no account detail — only that a message or
// request is waiting, in the kind's words, with a link to the conversation.
//
// Needs DATABASE_URL, AWS_REGION + NOTIFY_EMAIL_FROM (the SES transport), FRONTEND_ORIGIN
// and JWT_SECRET (the unsubscribe link). TELEGRAM_BOT_TOKEN is optional. Without the mail
// transport it is a no-op that never opens the pool — it must not claim notices it cannot
// deliver. MESSAGE_NOTIFY_MAX_PER_RUN (default 500) bounds one run.
package main

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/engage/buddy"
	"github.com/strelov1/freehire/internal/engage/emailnotify"
	"github.com/strelov1/freehire/internal/engage/emailprefs"
	"github.com/strelov1/freehire/internal/engage/messaging"
	"github.com/strelov1/freehire/internal/engage/telegramnotify"
	"github.com/strelov1/freehire/internal/platform/config"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
)

func main() { worker.Main(run) }

func run() int {
	cfg := config.Load()
	if cfg.AWSRegion == "" || cfg.NotifyEmailFrom == "" {
		log.Print("message-notify: email transport not configured (AWS_REGION / NOTIFY_EMAIL_FROM) — nothing to do")
		return 0
	}
	if len(cfg.JWTSecret) < 32 {
		log.Print("message-notify: JWT_SECRET must be at least 32 bytes — the unsubscribe link would not verify; refusing")
		return 1
	}
	if !strings.HasPrefix(cfg.FrontendOrigin, "https://") {
		log.Printf("message-notify: FRONTEND_ORIGIN is %q, which would send links nobody can open; refusing", cfg.FrontendOrigin)
		return 1
	}
	maxRows, err := worker.EnvInt32("MESSAGE_NOTIFY_MAX_PER_RUN", messaging.DefaultNotifyMaxPerRun)
	if err != nil {
		log.Printf("message-notify: %v", err)
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
		log.Printf("message-notify: ses: %v", err)
		return 1
	}
	queries := db.New(pool)
	nc := messaging.NotifierConfig{
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

	res, err := messaging.NewNotifier(queries, nc, buddy.Kind{}).NotifyUnread(ctx, time.Now())
	if err != nil {
		log.Printf("message-notify: %v", err)
		return 1
	}
	log.Printf("message-notify: notified=%d failed=%d", res.Notified, res.Failed)
	if res.Failed > 0 {
		return 1
	}
	return 0
}
