// Command mentorship-remind sends the reminders a booked mentorship session is due.
// Schedule it every few minutes.
//
// For each offset in mentorship.ReminderOffsets it takes the confirmed sessions starting
// within that window and not yet reminded at it, CLAIMS each reminder, and sends only the
// ones it won. Claim before send: sending first sends twice whenever recording fails.
//
// A session that has already started is never reminded about, so a late run does nothing
// rather than catching up. That predicate lives in the query, not here.
//
// A per-session delivery failure is counted and stepped over. The run exits non-zero only
// when it could not read the work at all.
//
// Needs DATABASE_URL and the mail configuration. Without a mail transport it is a no-op that
// never opens the pool: it must not merely skip sending, because claiming a reminder it
// cannot deliver would mark it sent forever.
//
// A second pass handles buddy mock interviews (buddy.SendDueMockReminders) with the same
// offsets, windows and claim order. A failure in one pass does not skip the other, and the
// run exits non-zero if either could not read its work.
package main

import (
	"context"
	"log"
	"os"
	"strconv"

	"github.com/strelov1/freehire/internal/engage/buddy"
	"github.com/strelov1/freehire/internal/engage/emailnotify"
	"github.com/strelov1/freehire/internal/engage/mentorship"
	"github.com/strelov1/freehire/internal/platform/config"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
)

// maxPerRunDefault bounds one run per offset: systemd will not start a second instance while
// the first is active, so an unbounded run silently skips firings. Nothing is lost to the
// bound; what it does not reach is still due next run, and the offsets are hours wide.
const maxPerRunDefault = 500

func main() { worker.Main(run, emailnotify.HoldKeys("mentorship-remind")...) }

func run() int {
	// Gate before Bootstrap: an unconfigured deployment must not touch the database, so it
	// cannot claim a reminder it could never deliver.
	cfg := config.Load()
	if cfg.NotifyEmailFrom == "" || cfg.AWSRegion == "" {
		log.Print("mentorship-remind: no mail transport configured, nothing to send")
		return worker.Skip("AWS_REGION+NOTIFY_EMAIL_FROM")
	}

	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	sender, err := emailnotify.NewClient(ctx, cfg.AWSRegion)
	if err != nil {
		log.Printf("mentorship-remind: mail transport: %v", err)
		return 1
	}
	sender = sender.WithSendingBreaker()

	q := db.New(pool)
	svc := mentorship.New(
		mentorship.NewQueriesRepository(q, pool),
		mentorship.Config{
			Notifier: mentorship.NewMailNotifier(sender, cfg.NotifyEmailFrom,
				cfg.FrontendOrigin+"/my/sessions"),
		},
	)
	// The buddy pass needs no messaging core: reminders never start a conversation.
	buddyRepo := buddy.NewQueriesRepository(q, pool)
	mocks := buddy.NewMockService(buddy.New(buddyRepo, nil), buddyRepo,
		buddy.NewMockMailNotifier(q, sender, cfg.NotifyEmailFrom, cfg.FrontendOrigin))

	exit := 0
	stats, err := svc.SendDueReminders(ctx, maxPerRun())
	if err != nil {
		log.Printf("mentorship-remind: %v", err)
		exit = 1
	} else {
		log.Printf("mentorship-remind: sent=%d failed=%d", stats.Sent, stats.Failed)
	}
	mockStats, err := mocks.SendDueMockReminders(ctx, maxPerRun())
	if err != nil {
		log.Printf("mentorship-remind: buddy mocks: %v", err)
		exit = 1
	} else {
		log.Printf("mentorship-remind: buddy mocks sent=%d failed=%d", mockStats.Sent, mockStats.Failed)
	}
	return exit
}

// maxPerRun reads the per-run bound.
//
// A set-but-unreadable value falls back and says so rather than failing the run: this runs
// unattended, and failing hard would stop all reminders over a batch-size typo. One-off
// backfills fail hard instead because an operator watches them. cmd/discord-sync and
// cmd/billing-sync choose as this does.
func maxPerRun() int32 {
	raw := os.Getenv("MENTORSHIP_REMIND_MAX_PER_RUN")
	if raw == "" {
		return maxPerRunDefault
	}
	n, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || n <= 0 {
		log.Printf("mentorship-remind: MENTORSHIP_REMIND_MAX_PER_RUN=%q is not a positive number that fits a batch size — keeping %d", raw, maxPerRunDefault)
		return maxPerRunDefault
	}
	return int32(n)
}
