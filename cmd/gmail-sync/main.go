// Command gmail-sync is the standalone Gmail ATS-inbox sync worker. For every
// user who connected their Gmail, it reads their ATS mail via the Gmail API
// (scoped to the curated ATS sender domains), stores full messages, and advances
// a per-user watermark — a run-once-and-exit cron worker beside enrich/liveness.
//
// It is gated on config: without the Google OAuth client and GMAIL_TOKEN_KEY it
// exits cleanly (nothing to sync). Best-effort per user — a revoked grant flags
// that connection for re-consent, the run continues, and the exit code says the
// run was not wholly clean.
package main

import (
	"context"
	"log"

	"github.com/strelov1/freehire/internal/application/gmailsync"
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
		log.Print("gmail-sync: not configured (Google OAuth client / GMAIL_TOKEN_KEY) — nothing to do")
		return 0
	}
	cipher, err := tokencrypt.New(cfg.GmailTokenKey)
	if err != nil {
		log.Printf("gmail-sync: token key: %v", err)
		return 1
	}
	connector := gmailsync.NewConnector(g.ClientID, g.ClientSecret, cfg.FrontendOrigin)
	store := gmailsync.NewDBStore(db.New(pool))

	stats, err := gmailsync.NewWorker(store, cipher, connector.ReaderFactory()).WithLearnedDomains(store).RunOnce(ctx)
	if err != nil {
		log.Printf("gmail-sync: %v", err)
		return 1
	}
	log.Printf("gmail-sync done: connections=%d synced=%d failed=%d needs_reconsent=%d",
		stats.Connections, stats.Synced, stats.Failed, stats.Reconsent)
	// A flagged grant is this worker's dead letter: no later run retries it, because only a
	// browser consent clears the flag. Both counts must reach the exit code — a run in which
	// every mailbox failed used to exit 0 and publish a successful last-run metric.
	return worker.ExitCode(stats.Failed, stats.Reconsent)
}
