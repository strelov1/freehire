//go:build integration

// Integration coverage for DBStore.MailboxByAddress's username-based recipient
// resolution against a real Postgres — see the add-username-claim change for why
// this is no longer a single address lookup.
package mailingest

import (
	"context"
	"testing"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

func TestMailboxByAddress(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	q := db.New(pool)
	store := NewDBStore(q)

	var enrolledID, unenrolledID int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email, username) VALUES ('enrolled@example.test', 'ivan') RETURNING id`).Scan(&enrolledID); err != nil {
		t.Fatalf("seed enrolled user: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO mailboxes (user_id) VALUES ($1)`, enrolledID); err != nil {
		t.Fatalf("seed mailbox: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (email, username) VALUES ('unenrolled@example.test', 'petr') RETURNING id`).Scan(&unenrolledID); err != nil {
		t.Fatalf("seed unenrolled user: %v", err)
	}

	t.Run("known username with a mailbox resolves", func(t *testing.T) {
		userID, ok, err := store.MailboxByAddress(ctx, "ivan@inbox.freehire.test")
		if err != nil {
			t.Fatalf("MailboxByAddress: %v", err)
		}
		if !ok || userID != enrolledID {
			t.Errorf("MailboxByAddress = (%d, %v), want (%d, true)", userID, ok, enrolledID)
		}
	})

	t.Run("known username with no mailbox is not a recipient", func(t *testing.T) {
		_, ok, err := store.MailboxByAddress(ctx, "petr@inbox.freehire.test")
		if err != nil {
			t.Fatalf("MailboxByAddress: %v", err)
		}
		if ok {
			t.Error("MailboxByAddress reported a recipient for a username with no mailbox row")
		}
	})

	t.Run("unknown username is not a recipient", func(t *testing.T) {
		_, ok, err := store.MailboxByAddress(ctx, "nobody@inbox.freehire.test")
		if err != nil {
			t.Fatalf("MailboxByAddress: %v", err)
		}
		if ok {
			t.Error("MailboxByAddress reported a recipient for an unknown username")
		}
	})
}
