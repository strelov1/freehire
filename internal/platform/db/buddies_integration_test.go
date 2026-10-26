//go:build integration

// Integration test for the interview-buddies and messaging schemas' account-deletion
// cascade and the membership queries. Run with: go test -tags=integration ./internal/platform/db/
package db

import (
	"context"
	"testing"
)

func TestDeletingUserCascadesBuddyData(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	alice := seedAccount(t, pool, "alice-buddy@example.test")
	bob := seedAccount(t, pool, "bob-buddy@example.test")
	for id, handle := range map[int64]string{alice: "calm-otter-11", bob: "brave-lynx-22"} {
		if _, err := q.JoinBuddyMember(ctx, JoinBuddyMemberParams{UserID: id, Handle: handle}); err != nil {
			t.Fatalf("join %d: %v", id, err)
		}
	}

	// Bob starts it, so Alice's deletion reaches the conversation only through her
	// participant row — the trigger's path, not the starter's foreign key.
	started, err := q.StartConversation(ctx, StartConversationParams{
		Kind: "buddy", StarterID: bob, StarterPersona: "buddy", RecipientID: alice, RecipientPersona: "buddy", Body: "hello",
	})
	if err != nil {
		t.Fatalf("start conversation: %v", err)
	}
	conv := started.ID

	for _, stmt := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO user_blocks (blocker_id, blocked_id) VALUES ($1, $2)`, []any{alice, bob}},
		{`INSERT INTO buddy_hidden_applications (user_id, application_id) VALUES ($1, 42)`, []any{alice}},
		{`INSERT INTO conversation_reports (conversation_id, reporter_id, reason) VALUES ($1, $2, 'spam')`, []any{conv, bob}},
	} {
		if _, err := pool.Exec(ctx, stmt.sql, stmt.args...); err != nil {
			t.Fatalf("seed %q: %v", stmt.sql, err)
		}
	}

	if _, err := pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, alice); err != nil {
		t.Fatalf("delete user: %v", err)
	}

	for _, c := range []struct {
		name, query string
		args        []any
		want        int
	}{
		{"deleted member", `SELECT count(*) FROM buddy_members WHERE user_id = $1`, []any{alice}, 0},
		{"other member", `SELECT count(*) FROM buddy_members WHERE user_id = $1`, []any{bob}, 1},
		{"conversations", `SELECT count(*) FROM conversations`, nil, 0},
		{"participants", `SELECT count(*) FROM conversation_participants`, nil, 0},
		{"messages", `SELECT count(*) FROM messages`, nil, 0},
		{"blocks", `SELECT count(*) FROM user_blocks`, nil, 0},
		{"hidden applications", `SELECT count(*) FROM buddy_hidden_applications`, nil, 0},
		{"reports", `SELECT count(*) FROM conversation_reports`, nil, 0},
	} {
		var n int
		if err := pool.QueryRow(ctx, c.query, c.args...).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", c.name, err)
		}
		if n != c.want {
			t.Errorf("%s: %d rows, want %d", c.name, n, c.want)
		}
	}
}

func TestBuddyMembershipQueries(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	user := seedAccount(t, pool, "member-buddy@example.test")
	other := seedAccount(t, pool, "other-buddy@example.test")

	m, err := q.JoinBuddyMember(ctx, JoinBuddyMemberParams{UserID: user, Handle: "calm-otter-11", Countries: []string{"de"}})
	if err != nil {
		t.Fatalf("join: %v", err)
	}
	if m.LeftAt.Valid {
		t.Fatal("fresh member has left_at set")
	}

	started, err := q.StartConversation(ctx, StartConversationParams{
		Kind: "buddy", StarterID: other, StarterPersona: "buddy", RecipientID: user, RecipientPersona: "buddy", Body: "hello",
	})
	if err != nil {
		t.Fatalf("start conversation: %v", err)
	}
	conv := started.ID

	left, err := q.LeaveBuddyMember(ctx, user)
	if err != nil {
		t.Fatalf("leave: %v", err)
	}
	if !left.LeftAt.Valid {
		t.Error("leave did not set left_at")
	}
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM conversations WHERE id = $1`, conv).Scan(&status); err != nil {
		t.Fatalf("read conversation: %v", err)
	}
	if status != "closed" {
		t.Errorf("conversation status %q after leave, want closed", status)
	}

	again, err := q.JoinBuddyMember(ctx, JoinBuddyMemberParams{UserID: user, Handle: "other-handle-99"})
	if err != nil {
		t.Fatalf("rejoin: %v", err)
	}
	if again.Handle != "calm-otter-11" {
		t.Errorf("rejoin handle %q, want the original calm-otter-11", again.Handle)
	}
	if again.LeftAt.Valid {
		t.Error("rejoin did not clear left_at")
	}

	byHandle, err := q.GetBuddyMemberByHandle(ctx, "calm-otter-11")
	if err != nil || byHandle.UserID != user {
		t.Errorf("GetBuddyMemberByHandle = %+v, %v", byHandle, err)
	}
}
