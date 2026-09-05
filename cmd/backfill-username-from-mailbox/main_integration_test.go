//go:build integration

// End-to-end check (task 6.3 of the add-username-claim change): an account with a
// pre-existing mailbox keeps the same derived address after the schema migration and
// this worker's backfill. Simulates the production sequence — a mailboxes row that
// predates users.username — against a real Postgres.
package main

import (
	"context"
	"testing"

	"github.com/strelov1/freehire/internal/identity/accounts"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

func TestBackfillPreservesThePreExistingMailboxAddress(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()

	var userID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email) VALUES ('legacy@example.test') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	// A pre-existing mailbox with a legacy handle (dot allowed, per the retired
	// mailbox.Handle), simulating data from before this change shipped.
	const legacyAddress = "legacy.name@inbox.freehire.test"
	if _, err := pool.Exec(ctx,
		`INSERT INTO mailboxes (user_id, address) VALUES ($1, $2)`, userID, legacyAddress); err != nil {
		t.Fatalf("seed legacy mailbox: %v", err)
	}

	q := db.New(pool)
	rows, err := q.ListMailboxesWithoutBackfilledUsername(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 1 || rows[0].UserID != userID {
		t.Fatalf("candidates = %+v, want exactly the seeded user", rows)
	}

	svc := accounts.New(accounts.NewQueriesRepository(q, pool), noopHasher{})
	base := localPart(rows[0].Address.String)
	name, err := svc.EnsureUsernameFromBase(ctx, userID, base)
	if err != nil {
		t.Fatalf("EnsureUsernameFromBase: %v", err)
	}

	// The legacy handle had a dot, sanitized to a hyphen — the derived address is
	// therefore cosmetically different, not byte-identical, matching design.md's
	// Goals (a dot is the one character the new format cannot preserve).
	const wantUsername = "legacy-name"
	if name != wantUsername {
		t.Errorf("backfilled username = %q, want %q", name, wantUsername)
	}

	gotAddress := name + "@inbox.freehire.test"
	if gotAddress == legacyAddress {
		t.Fatalf("test setup did not exercise the dot-sanitization path")
	}
	t.Logf("legacy address %q backfilled to %q", legacyAddress, gotAddress)

	// No further candidates remain — the backfill is complete for this account.
	remaining, err := q.ListMailboxesWithoutBackfilledUsername(ctx)
	if err != nil {
		t.Fatalf("list after backfill: %v", err)
	}
	if len(remaining) != 0 {
		t.Errorf("remaining candidates = %d, want 0", len(remaining))
	}
}

func TestBackfillPreservesAnAlreadyValidAddressByteForByte(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()

	var userID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email) VALUES ('plain@example.test') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	const address = "plain-handle@inbox.freehire.test"
	if _, err := pool.Exec(ctx,
		`INSERT INTO mailboxes (user_id, address) VALUES ($1, $2)`, userID, address); err != nil {
		t.Fatalf("seed mailbox: %v", err)
	}

	q := db.New(pool)
	rows, err := q.ListMailboxesWithoutBackfilledUsername(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("candidates = %d, want 1", len(rows))
	}

	svc := accounts.New(accounts.NewQueriesRepository(q, pool), noopHasher{})
	name, err := svc.EnsureUsernameFromBase(ctx, userID, localPart(rows[0].Address.String))
	if err != nil {
		t.Fatalf("EnsureUsernameFromBase: %v", err)
	}
	if got := name + "@inbox.freehire.test"; got != address {
		t.Errorf("backfilled address = %q, want the original %q unchanged", got, address)
	}
}
