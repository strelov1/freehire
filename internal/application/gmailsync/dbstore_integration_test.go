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

func TestCooldownSurvivesStoreRecreationAndSkipsGmailQueries(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	var id int64
	if err := pool.QueryRow(ctx, "INSERT INTO users(email) VALUES ('cooldown@example.test') RETURNING id").Scan(&id); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO gmail_connections(user_id,email,refresh_token_enc,status) VALUES ($1,'cooldown@example.test','token','connected')", id); err != nil {
		t.Fatal(err)
	}
	store := NewDBStore(db.New(pool))
	until, err := store.CooldownUntil(ctx, id)
	if err != nil || !until.IsZero() {
		t.Fatalf("initial cooldown=%v err=%v", until, err)
	}
	var before time.Time
	if err := pool.QueryRow(ctx, "SELECT now()").Scan(&before); err != nil {
		t.Fatal(err)
	}
	if err := store.SetCooldown(ctx, id); err != nil {
		t.Fatal(err)
	}
	fresh := NewDBStore(db.New(pool))
	until, err = fresh.CooldownUntil(ctx, id)
	if err != nil || until.Before(before.Add(59*time.Minute)) || until.After(before.Add(61*time.Minute)) {
		t.Fatalf("persisted cooldown=%v err=%v", until, err)
	}
	w := NewWorker(fresh, testCipher(t), func(context.Context, string, []string) GmailReader {
		t.Fatal("queried Gmail during persisted cooldown")
		return nil
	})
	stats, err := w.RunOnce(ctx)
	if err != nil || stats.Throttled != 1 {
		t.Fatalf("stats=%+v err=%v", stats, err)
	}
	if _, err := pool.Exec(ctx, "UPDATE gmail_connections SET gmail_retry_after=now()-interval '1 second' WHERE user_id=$1", id); err != nil {
		t.Fatal(err)
	}
	rows, err := fresh.ListConnected(ctx)
	if err != nil || len(rows) != 1 || rows[0].UserID != id {
		t.Fatalf("eligible connections=%v err=%v", rows, err)
	}
}
