//go:build integration

// Integration tests for invitations.
// Run with: go test -tags=integration ./internal/engage/buddy/
package buddy_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/engage/buddy"
	"github.com/strelov1/freehire/internal/engage/messaging"
)

// member seeds a user and joins them to the network.
func member(t *testing.T, pool *pgxpool.Pool, svc *buddy.Service, email string) (int64, string) {
	t.Helper()
	id := seedUser(t, pool, email)
	m, err := svc.Join(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return id, m.Handle
}

// openChat seeds two members and an accepted conversation between them, A having invited B.
func openChat(t *testing.T) (pool *pgxpool.Pool, svc *buddy.Service, a, b, convID int64) {
	t.Helper()
	pool, svc = setup(t)
	ctx := context.Background()
	a, _ = member(t, pool, svc, "a@example.test")
	b, bHandle := member(t, pool, svc, "b@example.test")
	c, err := svc.Invite(ctx, a, bHandle, "hello")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := chat(pool).Accept(ctx, b, c.ID); err != nil {
		t.Fatal(err)
	}
	return pool, svc, a, b, c.ID
}

func countRows(t *testing.T, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestInviteCreatesPendingConversationWithFirstMessage(t *testing.T) {
	pool, svc := setup(t)
	ctx := context.Background()
	a, _ := member(t, pool, svc, "a@example.test")
	_, bHandle := member(t, pool, svc, "b@example.test")

	c, err := svc.Invite(ctx, a, bHandle, "  Interviewing for the same role?  ")
	if err != nil {
		t.Fatal(err)
	}
	if c.Status != messaging.StatusPending || !c.Starter || otherHandle(c) != bHandle || c.ID == 0 {
		t.Fatalf("invite = %+v", c)
	}
	var body string
	var sender int64
	if err := pool.QueryRow(ctx, `SELECT body, sender_id FROM messages WHERE conversation_id = $1`, c.ID).Scan(&body, &sender); err != nil {
		t.Fatal(err)
	}
	if body != "Interviewing for the same role?" || sender != a {
		t.Fatalf("first message = %q from %d", body, sender)
	}
}

func TestInviteIsOrderIndependent(t *testing.T) {
	pool, svc := setup(t)
	ctx := context.Background()
	a, aHandle := member(t, pool, svc, "a@example.test")
	b, bHandle := member(t, pool, svc, "b@example.test")
	// The higher id invites the lower one: the stored pair is still ordered.
	hi, lo, loHandle := max(a, b), min(a, b), aHandle
	if lo == b {
		loHandle = bHandle
	}
	if _, err := svc.Invite(ctx, hi, loHandle, "hi"); err != nil {
		t.Fatal(err)
	}
	// And the reverse direction finds the same pair.
	other := aHandle
	if lo == a {
		other = bHandle
	}
	if _, err := svc.Invite(ctx, lo, other, "hi"); !errors.Is(err, messaging.ErrExists) {
		t.Fatalf("reverse invite err = %v, want ErrConversationExists", err)
	}
}

func TestInviteRefusesSecondConversationForPairInAnyStatus(t *testing.T) {
	pool, svc := setup(t)
	ctx := context.Background()
	a, aHandle := member(t, pool, svc, "a@example.test")
	b, bHandle := member(t, pool, svc, "b@example.test")

	c, err := svc.Invite(ctx, a, bHandle, "hello")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Invite(ctx, a, bHandle, "again"); !errors.Is(err, messaging.ErrExists) {
		t.Fatalf("pending: %v", err)
	}
	if _, err := chat(pool).Decline(ctx, b, c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Invite(ctx, a, bHandle, "again"); !errors.Is(err, messaging.ErrExists) {
		t.Fatalf("declined: %v", err)
	}
	if _, err := svc.Invite(ctx, b, aHandle, "my turn"); !errors.Is(err, messaging.ErrExists) {
		t.Fatalf("declined, other direction: %v", err)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM conversations`); n != 1 {
		t.Fatalf("conversations = %d, want 1", n)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM messages`); n != 1 {
		t.Fatalf("messages = %d, want 1", n)
	}
}

func TestInviteConcurrentDuplicateIsConflictNotError(t *testing.T) {
	pool, svc := setup(t)
	ctx := context.Background()
	a, aHandle := member(t, pool, svc, "a@example.test")
	b, bHandle := member(t, pool, svc, "b@example.test")

	var wg sync.WaitGroup
	errs := make([]error, 8)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Both directions race for the one pair.
			if i%2 == 0 {
				_, errs[i] = svc.Invite(ctx, a, bHandle, "hi")
			} else {
				_, errs[i] = svc.Invite(ctx, b, aHandle, "hi")
			}
		}()
	}
	wg.Wait()
	created := 0
	for _, err := range errs {
		switch {
		case err == nil:
			created++
		case !errors.Is(err, messaging.ErrExists):
			t.Fatalf("racing invite failed with %v, want nil or ErrConversationExists", err)
		}
	}
	if created != 1 {
		t.Fatalf("%d invitations succeeded, want 1", created)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM messages`); n != 1 {
		t.Fatalf("messages = %d, want 1", n)
	}
}

func TestInviteDailyLimitReportsWhenNextIsPossible(t *testing.T) {
	pool, svc := setup(t)
	ctx := context.Background()
	a, _ := member(t, pool, svc, "a@example.test")
	for i := range buddy.DailyInvitationCap {
		_, h := member(t, pool, svc, fmt.Sprintf("t%d@example.test", i))
		if _, err := svc.Invite(ctx, a, h, "hi"); err != nil {
			t.Fatalf("invite %d: %v", i, err)
		}
	}
	// Age the first invitation so the oldest in the window is a known instant.
	oldest := time.Now().Add(-20 * time.Hour).Truncate(time.Microsecond)
	if _, err := pool.Exec(ctx, `UPDATE conversations SET created_at = $2 WHERE id = (SELECT min(id) FROM conversations WHERE starter_id = $1)`, a, oldest); err != nil {
		t.Fatal(err)
	}
	_, extra := member(t, pool, svc, "extra@example.test")
	before := countRows(t, pool, `SELECT count(*) FROM conversations`)

	_, err := svc.Invite(ctx, a, extra, "one more")
	var limit *buddy.InvitationLimitError
	if !errors.As(err, &limit) {
		t.Fatalf("err = %v, want *InvitationLimitError", err)
	}
	if want := oldest.Add(24 * time.Hour); !limit.RetryAt.Equal(want) {
		t.Fatalf("RetryAt = %v, want %v", limit.RetryAt, want)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM conversations`); n != before {
		t.Fatalf("a refused invitation created a conversation")
	}

	// An invitation older than the window no longer counts.
	if _, err := pool.Exec(ctx, `UPDATE conversations SET created_at = now() - interval '25 hours' WHERE starter_id = $1`, a); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Invite(ctx, a, extra, "one more"); err != nil {
		t.Fatalf("after the window rolled: %v", err)
	}
}

func TestInviteRefusesBadMessage(t *testing.T) {
	pool, svc := setup(t)
	ctx := context.Background()
	a, _ := member(t, pool, svc, "a@example.test")
	_, bHandle := member(t, pool, svc, "b@example.test")

	for name, body := range map[string]string{
		"empty": "", "blank": " \n\t ", "too long": strings.Repeat("x", buddy.MaxInvitationRunes+1),
		"too long in runes": strings.Repeat("щ", buddy.MaxInvitationRunes+1),
	} {
		if _, err := svc.Invite(ctx, a, bHandle, body); !errors.Is(err, messaging.ErrInvalidMessage) {
			t.Fatalf("%s: err = %v, want ErrInvalidMessage", name, err)
		}
	}
	// 500 multi-byte characters is within the limit even though it exceeds 500 bytes.
	if _, err := svc.Invite(ctx, a, bHandle, strings.Repeat("щ", buddy.MaxInvitationRunes)); err != nil {
		t.Fatalf("500 runes: %v", err)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM conversations`); n != 1 {
		t.Fatalf("conversations = %d, want 1", n)
	}
}

func TestInviteRefusesUnavailableRecipients(t *testing.T) {
	pool, svc := setup(t)
	ctx := context.Background()
	a, aHandle := member(t, pool, svc, "a@example.test")
	_, bHandle := member(t, pool, svc, "b@example.test")
	left, leftHandle := member(t, pool, svc, "left@example.test")
	blocked, blockedHandle := member(t, pool, svc, "blocked@example.test")
	blocker, blockerHandle := member(t, pool, svc, "blocker@example.test")
	outsider := seedUser(t, pool, "outsider@example.test")
	if err := svc.Leave(ctx, left); err != nil {
		t.Fatal(err)
	}
	for _, q := range [][2]int64{{a, blocked}, {blocker, a}} {
		if _, err := pool.Exec(ctx, `INSERT INTO user_blocks (blocker_id, blocked_id) VALUES ($1, $2)`, q[0], q[1]); err != nil {
			t.Fatal(err)
		}
	}

	cases := map[string]string{
		"unknown handle": "No-Such-Handle", "left member": leftHandle,
		"blocked by me": blockedHandle, "blocks me": blockerHandle, "myself": aHandle,
	}
	for name, h := range cases {
		if _, err := svc.Invite(ctx, a, h, "hi"); !errors.Is(err, buddy.ErrRecipientUnavailable) {
			t.Fatalf("%s: err = %v, want ErrRecipientUnavailable", name, err)
		}
	}
	// A non-member cannot invite, and a left member cannot either.
	if _, err := svc.Invite(ctx, outsider, bHandle, "hi"); !errors.Is(err, buddy.ErrNotMember) {
		t.Fatalf("non-member inviter: %v", err)
	}
	if _, err := svc.Invite(ctx, left, bHandle, "hi"); !errors.Is(err, buddy.ErrNotMember) {
		t.Fatalf("left inviter: %v", err)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM conversations`); n != 0 {
		t.Fatalf("conversations = %d, want 0", n)
	}
}

func TestAcceptOpensTheConversation(t *testing.T) {
	pool, svc := setup(t)
	ctx := context.Background()
	a, aHandle := member(t, pool, svc, "a@example.test")
	b, bHandle := member(t, pool, svc, "b@example.test")
	c, err := svc.Invite(ctx, a, bHandle, "hello")
	if err != nil {
		t.Fatal(err)
	}
	got, err := chat(pool).Accept(ctx, b, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != messaging.StatusOpen || got.Starter || otherHandle(got) != aHandle || got.AcceptedAt == nil {
		t.Fatalf("accept = %+v", got)
	}
	if _, err := chat(pool).Accept(ctx, b, c.ID); !errors.Is(err, messaging.ErrState) {
		t.Fatalf("second accept: %v", err)
	}
	if _, err := chat(pool).Decline(ctx, b, c.ID); !errors.Is(err, messaging.ErrState) {
		t.Fatalf("decline after accept: %v", err)
	}
}

func TestDeclineIsSilentAndFinal(t *testing.T) {
	pool, svc := setup(t)
	ctx := context.Background()
	a, _ := member(t, pool, svc, "a@example.test")
	b, bHandle := member(t, pool, svc, "b@example.test")
	c, err := svc.Invite(ctx, a, bHandle, "hello")
	if err != nil {
		t.Fatal(err)
	}
	got, err := chat(pool).Decline(ctx, b, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != messaging.StatusDeclined || got.AcceptedAt != nil {
		t.Fatalf("decline = %+v", got)
	}
	if _, err := chat(pool).Accept(ctx, b, c.ID); !errors.Is(err, messaging.ErrState) {
		t.Fatalf("accept after decline: %v", err)
	}
}

func TestOnlyTheRecipientAnswers(t *testing.T) {
	pool, svc := setup(t)
	ctx := context.Background()
	a, _ := member(t, pool, svc, "a@example.test")
	_, bHandle := member(t, pool, svc, "b@example.test")
	third, _ := member(t, pool, svc, "c@example.test")
	c, err := svc.Invite(ctx, a, bHandle, "hello")
	if err != nil {
		t.Fatal(err)
	}
	// The inviter cannot answer their own invitation.
	if _, err := chat(pool).Accept(ctx, a, c.ID); !errors.Is(err, messaging.ErrState) {
		t.Fatalf("inviter accept: %v", err)
	}
	if _, err := chat(pool).Decline(ctx, a, c.ID); !errors.Is(err, messaging.ErrState) {
		t.Fatalf("inviter decline: %v", err)
	}
	// An outsider and an unknown id are both "not found".
	if _, err := chat(pool).Accept(ctx, third, c.ID); !errors.Is(err, messaging.ErrNotFound) {
		t.Fatalf("outsider accept: %v", err)
	}
	if _, err := chat(pool).Decline(ctx, third, c.ID+1000); !errors.Is(err, messaging.ErrNotFound) {
		t.Fatalf("unknown id: %v", err)
	}
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM conversations WHERE id = $1`, c.ID).Scan(&status); err != nil || status != messaging.StatusPending {
		t.Fatalf("status = %q, %v", status, err)
	}
}
