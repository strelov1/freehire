//go:build integration

package gmailsync

import (
	"context"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

func TestStoredMessagesReportsOnlyTheUsersStoredIDs(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	store := NewDBStore(db.New(pool))

	var owner, other int64
	for email, id := range map[string]*int64{"owner@example.test": &owner, "other@example.test": &other} {
		if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ($1) RETURNING id`, email).Scan(id); err != nil {
			t.Fatalf("seed user: %v", err)
		}
	}
	at := time.Unix(1_700_000_000, 0).UTC()
	for _, e := range []StoredEmail{
		{UserID: owner, Message: Message{ID: "in", ThreadID: "t1", ToAddrs: "owner@example.test", ReceivedAt: at, Direction: DirectionIncoming}},
		{UserID: owner, Message: Message{ID: "out", ThreadID: "t2", ToAddrs: "hr@example.test", ReceivedAt: at, Direction: DirectionOutgoing}},
		// Stored before recipients were recorded: a fetch repairs it, so it is not reported.
		{UserID: owner, Message: Message{ID: "legacy", ThreadID: "t4", ReceivedAt: at}},
		{UserID: other, Message: Message{ID: "theirs", ThreadID: "t3", ToAddrs: "other@example.test", ReceivedAt: at}},
	} {
		if err := store.UpsertEmail(ctx, e); err != nil {
			t.Fatalf("UpsertEmail: %v", err)
		}
	}

	got, err := store.StoredMessages(ctx, owner, []string{"in", "out", "legacy", "theirs", "missing"})
	if err != nil {
		t.Fatalf("StoredMessages: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("StoredMessages = %v, want only the owner's in and out", got)
	}
	if ref := got["in"]; ref.ThreadID != "t1" || !ref.Incoming || !ref.ReceivedAt.Equal(at) {
		t.Errorf("in = %+v, want thread t1, incoming, received %v", ref, at)
	}
	if ref := got["out"]; ref.Incoming {
		t.Errorf("out = %+v, want outgoing", ref)
	}
	if empty, err := store.StoredMessages(ctx, owner, nil); err != nil || len(empty) != 0 {
		t.Errorf("StoredMessages(nil) = %v, %v; want nothing", empty, err)
	}
}
