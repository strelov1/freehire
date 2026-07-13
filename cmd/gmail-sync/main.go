// Command gmail-sync is the standalone Gmail ATS-inbox sync worker. For every
// user who connected their Gmail, it reads their ATS mail via the Gmail API
// (scoped to the curated ATS sender domains), stores full messages, and advances
// a per-user watermark — a run-once-and-exit cron worker beside enrich/liveness.
//
// It is gated on config: without the Google OAuth client and GMAIL_TOKEN_KEY it
// exits cleanly (nothing to sync). Best-effort per user — a revoked token flags
// that connection for re-consent and the run continues.
package main

import (
	"context"
	"log"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/gmailsync"
	"github.com/strelov1/freehire/internal/tokencrypt"
	"github.com/strelov1/freehire/internal/worker"
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

	if err := gmailsync.NewWorker(store, cipher, connector.ReaderFactory()).RunOnce(ctx); err != nil {
		log.Printf("gmail-sync: %v", err)
		return 1
	}
	return 0
}
