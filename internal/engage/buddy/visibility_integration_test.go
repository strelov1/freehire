//go:build integration

// Integration tests for who a conversation still shows, and for the guards that live in
// the writes. Run with: go test -tags=integration ./internal/engage/buddy/
package buddy_test

import (
	"context"
	"errors"
	"testing"

	"github.com/strelov1/freehire/internal/engage/buddy"
	"github.com/strelov1/freehire/internal/platform/db"
)

func assertFormerMember(t *testing.T, svc *buddy.Service, viewer, convID int64) {
	t.Helper()
	list, err := svc.ListConversations(context.Background(), viewer)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != convID {
		t.Fatalf("list = %+v", list)
	}
	got := list[0]
	if !got.FormerMember || got.OtherHandle != "" || got.Card != (buddy.CardSummary{}) {
		t.Fatalf("conversation must be a bare former member, got %+v", got)
	}
}

func TestLeftMemberIsAFormerMemberToTheOtherSide(t *testing.T) {
	pool, svc, a, b, id := openChat(t)
	ctx := context.Background()
	seedCV(t, pool, b)
	bHandle := handleOf(t, svc, b)
	if err := svc.Leave(ctx, b); err != nil {
		t.Fatal(err)
	}
	assertFormerMember(t, svc, a, id)
	if _, err := svc.Profile(ctx, a, bHandle); !errors.Is(err, buddy.ErrNotFound) {
		t.Fatalf("profile of a left member = %v, want ErrNotFound", err)
	}
}

func TestLeftMemberDeclinedInvitationDoesNotLeakToTheInitiator(t *testing.T) {
	pool, svc := setup(t)
	ctx := context.Background()
	a, _ := member(t, pool, svc, "a@example.test")
	b, bHandle := member(t, pool, svc, "b@example.test")
	c, err := svc.Invite(ctx, a, bHandle, "hi")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Decline(ctx, b, c.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.Leave(ctx, b); err != nil {
		t.Fatal(err)
	}
	assertFormerMember(t, svc, a, c.ID)
}

func TestBlockedPairAreFormerMembersToEachOther(t *testing.T) {
	_, svc, a, b, id := openChat(t)
	if err := svc.Block(context.Background(), a, id); err != nil {
		t.Fatal(err)
	}
	assertFormerMember(t, svc, a, id)
	assertFormerMember(t, svc, b, id)
}

// The service pre-checks membership and blocks, but a leave or block can commit between
// that read and the write. These call the repository directly, which is the write alone.
func TestInvitationWriteRefusesAnUnavailablePair(t *testing.T) {
	pool, svc := setup(t)
	ctx := context.Background()
	repo := buddy.NewQueriesRepository(db.New(pool))
	a, _ := member(t, pool, svc, "a@example.test")
	b, _ := member(t, pool, svc, "b@example.test")
	c, _ := member(t, pool, svc, "c@example.test")

	if _, err := pool.Exec(ctx, `UPDATE buddy_members SET left_at = now() WHERE user_id = $1`, b); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateInvitation(ctx, a, b, "hi"); !errors.Is(err, buddy.ErrRecipientUnavailable) {
		t.Fatalf("invite a left recipient = %v", err)
	}
	if _, err := repo.CreateInvitation(ctx, b, a, "hi"); !errors.Is(err, buddy.ErrRecipientUnavailable) {
		t.Fatalf("invite from a left inviter = %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO buddy_blocks (blocker_id, blocked_id) VALUES ($1, $2)`, c, a); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.CreateInvitation(ctx, a, c, "hi"); !errors.Is(err, buddy.ErrRecipientUnavailable) {
		t.Fatalf("invite across a block = %v", err)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM buddy_conversations`); n != 0 {
		t.Fatalf("conversations = %d, want 0", n)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM buddy_messages`); n != 0 {
		t.Fatalf("messages = %d, want 0", n)
	}
}

func TestAnswerWriteRefusesAnUnavailablePair(t *testing.T) {
	pool, svc := setup(t)
	ctx := context.Background()
	repo := buddy.NewQueriesRepository(db.New(pool))
	a, _ := member(t, pool, svc, "a@example.test")
	b, bHandle := member(t, pool, svc, "b@example.test")
	c, cHandle := member(t, pool, svc, "c@example.test")
	left, err := svc.Invite(ctx, a, bHandle, "hi")
	if err != nil {
		t.Fatal(err)
	}
	blocked, err := svc.Invite(ctx, a, cHandle, "hi")
	if err != nil {
		t.Fatal(err)
	}
	// State the service's own Leave and Block would have closed; written directly so the
	// conversations stay pending, as they would in the race.
	if _, err := pool.Exec(ctx, `UPDATE buddy_members SET left_at = now() WHERE user_id = $1`, a); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.AnswerInvitation(ctx, b, left.ID, buddy.StatusOpen); !errors.Is(err, buddy.ErrConversationState) {
		t.Fatalf("accept with a left initiator = %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE buddy_members SET left_at = NULL WHERE user_id = $1`, a); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO buddy_blocks (blocker_id, blocked_id) VALUES ($1, $2)`, a, c); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.AnswerInvitation(ctx, c, blocked.ID, buddy.StatusOpen); !errors.Is(err, buddy.ErrConversationState) {
		t.Fatalf("accept across a block = %v", err)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM buddy_conversations WHERE status = 'pending'`); n != 2 {
		t.Fatalf("pending = %d, want 2", n)
	}
}
