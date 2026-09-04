// Command backfill-username-from-mailbox fills users.username for every account that
// already has a hosted mailbox but predates the column, then exits.
//
// See the add-username-claim change's design.md (Migration Plan, step 2) for why this
// is a Go worker rather than a plain SQL UPDATE: mailboxes.address is a full
// "<handle>@<domain>" address, and the legacy mailbox.Handle charset/length rules
// ([a-z0-9.-], no minimum length) are looser than the new, stricter username format
// (no dots, 3-30 chars) — a raw copy could write a value the new users.username_check
// CHECK constraint rejects, or collide two different legacy handles into the same
// sanitized value with no resolution. This worker instead reuses the exact allocation
// path a fresh claim goes through (internal/identity/accounts.EnsureUsernameFromBase,
// which itself uses internal/identity/username's Sanitize/Candidate/IsReserved), so a
// backfilled username obeys the same rules a freshly claimed one does.
//
// Idempotent per row (only ever considers accounts with username still NULL) and safe
// to stop and re-run. Needs only DATABASE_URL.
package main

import (
	"context"
	"log"
	"strings"

	"github.com/strelov1/freehire/internal/identity/accounts"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
)

// localPart returns the portion of a mailbox address before '@' — the legacy handle
// mailbox.Handle allocated. EnsureUsernameFromBase sanitizes it into a valid
// username base internally, so the raw local-part is passed through as-is.
func localPart(address string) string {
	if at := strings.IndexByte(address, '@'); at >= 0 {
		return address[:at]
	}
	return address
}

// noopHasher satisfies accounts.PasswordHasher without ever being invoked: this
// worker only calls EnsureUsernameFromBase, which never touches passwords.
type noopHasher struct{}

func (noopHasher) Hash(string) (string, error) {
	panic("backfill-username-from-mailbox: password hashing is never used")
}
func (noopHasher) Check(string, string) error {
	panic("backfill-username-from-mailbox: password checking is never used")
}

func main() { worker.Main(run) }

func run() int {
	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	q := db.New(pool)
	rows, err := q.ListMailboxesWithoutBackfilledUsername(ctx)
	if err != nil {
		log.Printf("backfill-username-from-mailbox: list: %v", err)
		return 1
	}
	if len(rows) == 0 {
		log.Print("backfill-username-from-mailbox: nothing to do")
		return 0
	}
	log.Printf("backfill-username-from-mailbox: %d mailboxes to backfill", len(rows))

	svc := accounts.New(accounts.NewQueriesRepository(q, pool), noopHasher{})
	var filled int
	for _, row := range rows {
		if !row.Address.Valid {
			// Cannot happen in practice: a NULL address only arises on a mailbox row
			// created after this worker's own deploy, by which point EnsureUsername has
			// already run at claim time, so the row would already have a username and
			// never match this query's WHERE clause. Skip rather than derive from nothing.
			log.Printf("backfill-username-from-mailbox: user %d has no address, skipping", row.UserID)
			continue
		}
		base := localPart(row.Address.String)
		name, err := svc.EnsureUsernameFromBase(ctx, row.UserID, base)
		if err != nil {
			// Report what was already committed: each account is its own statement, so
			// the work done so far survives and a re-run resumes from it.
			log.Printf("backfill-username-from-mailbox: user %d after %d filled: %v", row.UserID, filled, err)
			return 1
		}
		filled++
		if name != base {
			log.Printf("backfill-username-from-mailbox: user %d: %q -> %q (sanitized or suffixed)", row.UserID, base, name)
		}
	}
	log.Printf("backfill-username-from-mailbox: done, filled=%d", filled)
	return 0
}
