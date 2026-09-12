// Command mentor-busy-sync reads every connected mentor's Google Calendar free/busy state
// and reconciles mentor_busy_intervals with it — a run-once-and-exit cron worker beside
// cal-sync and gmail-sync.
//
// It stores only bounds: no title, no attendee, no identifier. The slot engine
// (internal/engage/mentorship) already reads this table when it builds a mentor's
// offerable slots — it has since migration 0145 — and this worker is the sync that table
// was always waiting on.
//
// Gated on config: without the Google OAuth client and GMAIL_TOKEN_KEY it exits cleanly,
// and with them it still does nothing until a mentor grants busy-sync's own consent. Best
// effort per mentor — a revoked grant flags that connection for re-consent, the run
// continues, and the exit code says the run was not wholly clean.
package main

import (
	"context"
	"log"

	"github.com/strelov1/freehire/internal/application/gmailsync"
	"github.com/strelov1/freehire/internal/engage/mentorship/busysync"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/tokencrypt"
	"github.com/strelov1/freehire/internal/platform/worker"
)

func main() { worker.Main(run) }

func run() int {
	ctx, cfg, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	g := cfg.OAuth["google"]
	if g.ClientID == "" || g.ClientSecret == "" || len(cfg.GmailTokenKey) != 32 {
		log.Print("mentor-busy-sync: not configured (Google OAuth client / GMAIL_TOKEN_KEY) — nothing to do")
		return 0
	}
	cipher, err := tokencrypt.New(cfg.GmailTokenKey)
	if err != nil {
		log.Printf("mentor-busy-sync: token key: %v", err)
		return 1
	}

	connector := gmailsync.NewConnector(g.ClientID, g.ClientSecret, cfg.FrontendOrigin)
	store := busysync.NewDBStore(db.New(pool), pool)

	if err := busysync.NewWorker(store, cipher, busysync.ReaderFactoryFor(connector)).RunOnce(ctx); err != nil {
		// Not a fatal error in the sense of "nothing worked" — RunOnce is best-effort per
		// mentor and reports how many failed. Exiting non-zero is how cron learns: a run
		// that swallowed a revoked grant looks exactly like a run with nothing to do.
		log.Printf("mentor-busy-sync: %v", err)
		return 1
	}
	return 0
}
