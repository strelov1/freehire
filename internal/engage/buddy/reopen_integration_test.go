//go:build integration

// Integration tests for re-inviting after a conversation closed by leaving.
// Run with: go test -tags=integration ./internal/engage/buddy/
package buddy_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/engage/buddy"
)

// chatThenLeaveAndRejoin leaves A and B with an open conversation holding two messages,
// then has B leave and rejoin, so the conversation stands closed.
func chatThenLeaveAndRejoin(t *testing.T) (pool *pgxpool.Pool, svc *buddy.Service, a, b int64, bHandle string, convID int64) {
	t.Helper()
	pool, svc = setup(t)
	ctx := context.Background()
	a, _ = member(t, pool, svc, "a@example.test")
	b, bHandle = member(t, pool, svc, "b@example.test")
	c, err := svc.Invite(ctx, a, bHandle, "first")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Accept(ctx, b, c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Send(ctx, b, c.ID, "reply"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Leave(ctx, b); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Join(ctx, b); err != nil {
		t.Fatal(err)
	}
	return pool, svc, a, b, bHandle, c.ID
}

func TestInviteReopensConversationClosedByLeaving(t *testing.T) {
	pool, svc, a, b, bHandle, convID := chatThenLeaveAndRejoin(t)
	ctx := context.Background()

	c, err := svc.Invite(ctx, a, bHandle, "back again?")
	if err != nil {
		t.Fatal(err)
	}
	if c.ID != convID || c.Status != buddy.StatusPending || !c.Initiator || c.AcceptedAt != nil {
		t.Fatalf("reopened = %+v, want conversation %d pending, initiated by A", c, convID)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM buddy_conversations`); n != 1 {
		t.Fatalf("conversations = %d, want 1", n)
	}
	msgs, err := svc.Messages(ctx, b, convID, 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 3 || msgs[2].Body != "back again?" {
		t.Fatalf("messages = %+v, want the history plus the new one", msgs)
	}
	list, err := svc.ListConversations(ctx, b)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Unread != 1 {
		t.Fatalf("B's conversations = %+v, want only the new message unread", list)
	}
	listA, err := svc.ListConversations(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	if len(listA) != 1 || listA[0].Unread != 0 {
		t.Fatalf("A's conversations = %+v, want nothing unread", listA)
	}
	if _, err := svc.Accept(ctx, b, convID); err != nil {
		t.Fatalf("accepting the reopened invitation: %v", err)
	}
}

func TestInviteReopenedConversationCountsAgainstTheCap(t *testing.T) {
	pool, svc, a, _, bHandle, convID := chatThenLeaveAndRejoin(t)
	ctx := context.Background()
	// Age the original invitation out of the window; only the reopen can put it back.
	if _, err := pool.Exec(ctx, `UPDATE buddy_conversations SET created_at = now() - interval '48 hours' WHERE id = $1`, convID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Invite(ctx, a, bHandle, "again"); err != nil {
		t.Fatal(err)
	}
	if got := countRows(t, pool, `SELECT count(*) FROM buddy_conversations WHERE initiator_id = $1 AND created_at > now() - interval '1 minute'`, a); got != 1 {
		t.Fatalf("recent invitations by A = %d, want 1", got)
	}
}

func TestInviteBlockKeepsClosedConversationClosed(t *testing.T) {
	pool, svc, a, b, bHandle, convID := chatThenLeaveAndRejoin(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO buddy_blocks (blocker_id, blocked_id) VALUES ($1, $2)`, b, a); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Invite(ctx, a, bHandle, "hi"); !errors.Is(err, buddy.ErrRecipientUnavailable) {
		t.Fatalf("err = %v, want ErrRecipientUnavailable", err)
	}
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM buddy_conversations WHERE id = $1`, convID).Scan(&status); err != nil || status != buddy.StatusClosed {
		t.Fatalf("status = %q, %v", status, err)
	}
}

func TestInviteReopenRefusesOpenAndDeclined(t *testing.T) {
	pool, svc := setup(t)
	ctx := context.Background()
	a, _ := member(t, pool, svc, "a@example.test")
	b, bHandle := member(t, pool, svc, "b@example.test")
	c, err := svc.Invite(ctx, a, bHandle, "hi")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Accept(ctx, b, c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Invite(ctx, a, bHandle, "again"); !errors.Is(err, buddy.ErrConversationExists) {
		t.Fatalf("open: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE buddy_conversations SET status = 'declined' WHERE id = $1`, c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Invite(ctx, a, bHandle, "again"); !errors.Is(err, buddy.ErrConversationExists) {
		t.Fatalf("declined: %v", err)
	}
}
