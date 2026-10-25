//go:build integration

// Integration test for the interview-buddies schema's account-deletion cascade and the
// membership queries. Run with: go test -tags=integration ./internal/platform/db/
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

	low, high := alice, bob
	if low > high {
		low, high = high, low
	}
	var conv int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO buddy_conversations (user_low, user_high, initiator_id) VALUES ($1, $2, $1) RETURNING id`,
		low, high).Scan(&conv); err != nil {
		t.Fatalf("insert conversation: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO buddy_messages (conversation_id, sender_id, body) VALUES ($1, $2, 'hello')`, conv, low); err != nil {
		t.Fatalf("insert message: %v", err)
	}

	for _, stmt := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO buddy_blocks (blocker_id, blocked_id) VALUES ($1, $2)`, []any{alice, bob}},
		{`INSERT INTO buddy_hidden_applications (user_id, application_id) VALUES ($1, 42)`, []any{alice}},
		{`INSERT INTO buddy_reports (conversation_id, reporter_id, reason) VALUES ($1, $2, 'spam')`, []any{conv, bob}},
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
		{"conversations", `SELECT count(*) FROM buddy_conversations`, nil, 0},
		{"messages", `SELECT count(*) FROM buddy_messages`, nil, 0},
		{"blocks", `SELECT count(*) FROM buddy_blocks`, nil, 0},
		{"hidden applications", `SELECT count(*) FROM buddy_hidden_applications`, nil, 0},
		{"reports", `SELECT count(*) FROM buddy_reports`, nil, 0},
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

	low, high := user, other
	if low > high {
		low, high = high, low
	}
	var conv int64
	if err := pool.QueryRow(ctx,
		`INSERT INTO buddy_conversations (user_low, user_high, initiator_id, status) VALUES ($1, $2, $1, 'open') RETURNING id`,
		low, high).Scan(&conv); err != nil {
		t.Fatalf("insert conversation: %v", err)
	}

	left, err := q.LeaveBuddyMember(ctx, user)
	if err != nil {
		t.Fatalf("leave: %v", err)
	}
	if !left.LeftAt.Valid {
		t.Error("leave did not set left_at")
	}
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM buddy_conversations WHERE id = $1`, conv).Scan(&status); err != nil {
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
