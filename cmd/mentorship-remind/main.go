// Command mentorship-remind sends the reminders a booked mentorship session is due.
// Schedule it every few minutes.
//
// For each offset in mentorship.ReminderOffsets — 24 hours and 1 hour — it takes the
// confirmed sessions starting within that window and not yet reminded at it, CLAIMS each
// reminder, and sends only the ones it won. The order is the design: a worker that sends
// first and records afterwards sends twice whenever the second step fails, and on a
// several-minute schedule "whenever" is often.
//
// A session that has already STARTED is never reminded about. "Your session starts in an
// hour", delivered afterwards, is worse than silence, so a run that arrives late does
// nothing rather than catching up. That predicate lives in the query, not here.
//
// A per-session delivery failure is counted and stepped over — one unreachable mailbox
// must not cost everybody else their reminder. The run exits non-zero only when it could
// not read the work at all.
//
// Needs DATABASE_URL and the mail configuration. Without a mail transport it is a no-op
// that never opens the pool, which is both how this ships before SES is wired and what
// rolling it back looks like — and it must not merely skip SENDING, because claiming a
// reminder it cannot deliver would mark it sent forever.
package main

import (
	"context"
	"log"
	"os"
	"strconv"

	"github.com/strelov1/freehire/internal/engage/emailnotify"
	"github.com/strelov1/freehire/internal/engage/mentorship"
	"github.com/strelov1/freehire/internal/platform/config"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
)

// maxPerRunDefault bounds one run per offset, so a backlog cannot turn a Type=oneshot
// unit into a run that outlives its own timer — systemd will not start a second instance
// while the first is active, so an unbounded run silently skips firings.
//
// Nothing is lost to the bound: what it does not reach this run is still due next run,
// and the offsets are hours wide.
const maxPerRunDefault = 500

func main() { worker.Main(run) }

func run() int {
	// Gate before Bootstrap, not after. With no mail transport there is nothing to send,
	// and an unconfigured deployment must run without touching the database at all —
	// which also means it cannot claim a reminder it could never deliver.
	cfg := config.Load()
	if cfg.NotifyEmailFrom == "" || cfg.AWSRegion == "" {
		log.Print("mentorship-remind: no mail transport configured, nothing to send")
		return 0
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

	svc := mentorship.New(
		mentorship.NewQueriesRepository(db.New(pool), pool),
		mentorship.Config{
			Notifier: mentorship.NewMailNotifier(sender, cfg.NotifyEmailFrom,
				cfg.FrontendOrigin+"/my/sessions"),
		},
	)

	stats, err := svc.SendDueReminders(ctx, maxPerRun())
	if err != nil {
		log.Printf("mentorship-remind: %v", err)
		return 1
	}
	log.Printf("mentorship-remind: sent=%d failed=%d", stats.Sent, stats.Failed)
	return 0
}

// maxPerRun reads the per-run bound.
//
// A set-but-unreadable value falls back and says so rather than failing the run: this runs
// unattended every few minutes, where failing hard would stop reminding everybody for as
// long as nobody reads systemctl — a bad trade for a typo in a batch size. The one-off
// backfill passes make the opposite choice, and for the opposite reason: they run once
// under an operator's eye. cmd/discord-sync and cmd/billing-sync choose as this does.
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
