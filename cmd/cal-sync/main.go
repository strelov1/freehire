// Command cal-sync reads the interviews out of every connected candidate's calendar and
// attaches them to the applications they belong to — a run-once-and-exit cron worker
// beside gmail-sync and classify-mail.
//
// It stores only a meeting it could attach. The rest of the window — the medical
// appointments, the family, the current employer's meetings — is read into memory,
// matched, and discarded, and the schema refuses to hold it even if this changed.
//
// Gated on config: without the Google OAuth client and GMAIL_TOKEN_KEY it exits cleanly,
// and with them it still does nothing until a candidate grants the calendar scope. Best
// effort per candidate — a revoked grant flags that connection for re-consent, the run
// continues, and the exit code says the run was not wholly clean.
package main

import (
	"context"
	"log"

	"github.com/strelov1/freehire/internal/calsync"
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
		log.Print("cal-sync: not configured (Google OAuth client / GMAIL_TOKEN_KEY) — nothing to do")
		return 0
	}
	cipher, err := tokencrypt.New(cfg.GmailTokenKey)
	if err != nil {
		log.Printf("cal-sync: token key: %v", err)
		return 1
	}

	connector := gmailsync.NewConnector(g.ClientID, g.ClientSecret, cfg.FrontendOrigin)
	store := calsync.NewDBStore(db.New(pool))

	if err := calsync.NewWorker(store, cipher, calsync.ReaderFactoryFor(connector)).RunOnce(ctx); err != nil {
		// Not a fatal error in the sense of "nothing worked" — RunOnce is best-effort per
		// candidate and reports how many failed. Exiting non-zero is how cron learns:
		// a run that swallowed a revoked grant looks exactly like a run with nothing to do.
		log.Printf("cal-sync: %v", err)
		return 1
	}
	return 0
}
