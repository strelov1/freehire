//go:build integration

// Integration tests for the in-app chat.
// Run with: go test -tags=integration ./internal/engage/buddy/
package buddy_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/engage/buddy"
)

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
	if _, err := svc.Accept(ctx, b, c.ID); err != nil {
		t.Fatal(err)
	}
	return pool, svc, a, b, c.ID
}

func TestSendIsDeliveredAndCountsAsUnread(t *testing.T) {
	_, svc, a, b, id := openChat(t)
	ctx := context.Background()

	m, err := svc.Send(ctx, a, id, "  see you at the screen  ")
	if err != nil {
		t.Fatal(err)
	}
	if m.Body != "see you at the screen" || !m.Mine || m.ID == 0 {
		t.Fatalf("sent = %+v", m)
	}
	// The invitation's own text is the first message.
	msgs, err := svc.Messages(ctx, b, id, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 || msgs[0].Body != "hello" || msgs[1].Body != "see you at the screen" || msgs[0].Mine || msgs[1].Mine {
		t.Fatalf("B reads %+v", msgs)
	}
	list, err := svc.ListConversations(ctx, b)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Unread != 2 {
		t.Fatalf("B's list = %+v, want unread 2", list)
	}
	// The sender never sees their own message as unread.
	mine, err := svc.ListConversations(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	if len(mine) != 1 || mine[0].Unread != 0 {
		t.Fatalf("A's list = %+v, want unread 0", mine)
	}
}

func TestSendRefusesAPendingDeclinedOrForeignConversation(t *testing.T) {
	pool, svc := setup(t)
	ctx := context.Background()
	a, _ := member(t, pool, svc, "a@example.test")
	b, bHandle := member(t, pool, svc, "b@example.test")
	third, _ := member(t, pool, svc, "c@example.test")
	c, err := svc.Invite(ctx, a, bHandle, "hello")
	if err != nil {
		t.Fatal(err)
	}
	// Pending: refused for the inviter and for the recipient alike.
	for name, u := range map[string]int64{"inviter": a, "recipient": b} {
		if _, err := svc.Send(ctx, u, c.ID, "more"); !errors.Is(err, buddy.ErrConversationState) {
			t.Fatalf("pending, %s: %v", name, err)
		}
	}
	if _, err := svc.Send(ctx, third, c.ID, "hi"); !errors.Is(err, buddy.ErrNotFound) {
		t.Fatalf("outsider: %v", err)
	}
	if _, err := svc.Send(ctx, a, c.ID+1000, "hi"); !errors.Is(err, buddy.ErrNotFound) {
		t.Fatalf("unknown: %v", err)
	}
	if _, err := svc.Decline(ctx, b, c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Send(ctx, b, c.ID, "hi"); !errors.Is(err, buddy.ErrConversationState) {
		t.Fatalf("declined: %v", err)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM buddy_messages`); n != 1 {
		t.Fatalf("messages = %d, want 1", n)
	}
}

func TestSendRefusesBadBodiesAndClosedChats(t *testing.T) {
	pool, svc, a, b, id := openChat(t)
	ctx := context.Background()
	for name, body := range map[string]string{
		"empty": "", "blank": "  \n", "too long": strings.Repeat("x", buddy.MaxMessageRunes+1),
		"too long in runes": strings.Repeat("щ", buddy.MaxMessageRunes+1),
	} {
		if _, err := svc.Send(ctx, a, id, body); !errors.Is(err, buddy.ErrInvalidMessage) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if _, err := svc.Send(ctx, a, id, strings.Repeat("щ", buddy.MaxMessageRunes)); err != nil {
		t.Fatalf("4000 runes: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE buddy_conversations SET status = 'closed' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Send(ctx, b, id, "hi"); !errors.Is(err, buddy.ErrConversationState) {
		t.Fatalf("closed: %v", err)
	}
	// A closed chat's history stays readable to both parties.
	msgs, err := svc.Messages(ctx, b, id, 0, 0)
	if err != nil || len(msgs) != 2 {
		t.Fatalf("closed history = %+v, %v", msgs, err)
	}
	outsider := seedUser(t, pool, "x@example.test")
	if _, err := svc.Send(ctx, outsider, id, "hi"); !errors.Is(err, buddy.ErrNotMember) {
		t.Fatalf("non-member: %v", err)
	}
}

// A send that read the conversation as open and then waits behind a close must not
// slip its message in once the close commits: the insert guards the status itself.
func TestConcurrentCloseNeverLetsAMessageIn(t *testing.T) {
	pool, svc, a, _, id := openChat(t)
	ctx := context.Background()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck // rollback after commit is a no-op
	if _, err := tx.Exec(ctx, `UPDATE buddy_conversations SET status = 'closed' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := svc.Send(ctx, a, id, "too late")
		done <- err
	}()
	// Let the send reach the row lock before the close commits.
	deadline := time.Now().Add(5 * time.Second)
	for countRows(t, pool, `SELECT count(*) FROM pg_stat_activity WHERE wait_event_type = 'Lock' AND query LIKE '%buddy_messages%'`) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the send never blocked behind the close")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, buddy.ErrConversationState) {
		t.Fatalf("send behind a close: %v, want ErrConversationState", err)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM buddy_messages WHERE conversation_id = $1`, id); n != 1 {
		t.Fatalf("messages = %d, want only the invitation", n)
	}
}

func TestOutsiderCannotReadOrMarkRead(t *testing.T) {
	pool, svc, _, _, id := openChat(t)
	ctx := context.Background()
	third, _ := member(t, pool, svc, "c@example.test")
	if _, err := svc.Messages(ctx, third, id, 0, 0); !errors.Is(err, buddy.ErrNotFound) {
		t.Fatalf("Messages: %v", err)
	}
	if err := svc.MarkRead(ctx, third, id, 1); !errors.Is(err, buddy.ErrNotFound) {
		t.Fatalf("MarkRead: %v", err)
	}
	if _, err := svc.Messages(ctx, third, id+1000, 0, 0); !errors.Is(err, buddy.ErrNotFound) {
		t.Fatalf("unknown: %v", err)
	}
}

func TestMessagesAfterCursorOldestFirstAndLimit(t *testing.T) {
	_, svc, a, b, id := openChat(t)
	ctx := context.Background()
	var ids []int64
	for _, step := range []struct {
		sender int64
		body   string
	}{{a, "one"}, {b, "two"}, {a, "three"}} {
		m, err := svc.Send(ctx, step.sender, id, step.body)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, m.ID)
	}
	got, err := svc.Messages(ctx, b, id, ids[0], 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Body != "two" || got[1].Body != "three" || !got[0].Mine || got[1].Mine {
		t.Fatalf("after first = %+v", got)
	}
	got, err = svc.Messages(ctx, b, id, 0, 2)
	if err != nil || len(got) != 2 || got[0].Body != "hello" {
		t.Fatalf("limit 2 = %+v, %v", got, err)
	}
	got, err = svc.Messages(ctx, b, id, ids[2], 0)
	if err != nil || len(got) != 0 {
		t.Fatalf("nothing newer = %+v, %v", got, err)
	}
	// Reading alone does not mark anything read. B's own "two" moved B's cursor past the
	// invitation and "one", so only A's "three" is left unread.
	list, err := svc.ListConversations(ctx, b)
	if err != nil || list[0].Unread != 1 {
		t.Fatalf("unread after a bare read = %+v, %v", list, err)
	}
}

func TestMarkReadZeroesUnreadAndIsMonotonic(t *testing.T) {
	_, svc, a, b, id := openChat(t)
	ctx := context.Background()
	m1, _ := svc.Send(ctx, a, id, "one")
	m2, _ := svc.Send(ctx, a, id, "two")
	m3, _ := svc.Send(ctx, a, id, "three")

	unread := func() int {
		list, err := svc.ListConversations(ctx, b)
		if err != nil || len(list) != 1 {
			t.Fatalf("list = %+v, %v", list, err)
		}
		return list[0].Unread
	}
	if err := svc.MarkRead(ctx, b, id, m2.ID); err != nil {
		t.Fatal(err)
	}
	if got := unread(); got != 1 {
		t.Fatalf("after reading through m2 = %d, want 1", got)
	}
	// An older id never moves the cursor back.
	if err := svc.MarkRead(ctx, b, id, m1.ID); err != nil {
		t.Fatal(err)
	}
	if got := unread(); got != 1 {
		t.Fatalf("after an older mark = %d, want 1", got)
	}
	// An id beyond the newest message is clamped, so a later message stays unread.
	if err := svc.MarkRead(ctx, b, id, m3.ID+1_000_000); err != nil {
		t.Fatal(err)
	}
	if got := unread(); got != 0 {
		t.Fatalf("after reading everything = %d, want 0", got)
	}
	if _, err := svc.Send(ctx, a, id, "four"); err != nil {
		t.Fatal(err)
	}
	if got := unread(); got != 1 {
		t.Fatalf("new message after a clamped mark = %d, want 1", got)
	}
}

func TestListConversationsStatusesOrderAndHiddenDecline(t *testing.T) {
	pool, svc := setup(t)
	ctx := context.Background()
	me, myHandle := member(t, pool, svc, "me@example.test")
	incoming, _ := member(t, pool, svc, "in@example.test")
	_, outgoingH := member(t, pool, svc, "out@example.test")
	open, openH := member(t, pool, svc, "open@example.test")
	closedU, closedH := member(t, pool, svc, "closed@example.test")
	iDeclined, iDeclinedH := member(t, pool, svc, "d1@example.test")
	theyDeclined, theyDeclinedH := member(t, pool, svc, "d2@example.test")

	invite := func(from int64, toHandle string) int64 {
		t.Helper()
		c, err := svc.Invite(ctx, from, toHandle, "hi")
		if err != nil {
			t.Fatal(err)
		}
		return c.ID
	}
	invite(incoming, myHandle)
	invite(me, outgoingH)
	openID := invite(me, openH)
	closedID := invite(me, closedH)
	iDeclinedID := invite(iDeclined, myHandle)
	theyDeclinedID := invite(me, theyDeclinedH)
	if _, err := svc.Decline(ctx, me, iDeclinedID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Decline(ctx, theyDeclined, theyDeclinedID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Accept(ctx, open, openID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Accept(ctx, closedU, closedID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE buddy_conversations SET status = 'closed' WHERE id = $1`, closedID); err != nil {
		t.Fatal(err)
	}
	// The open chat gets the newest message, so it leads the list.
	if _, err := svc.Send(ctx, open, openID, "latest"); err != nil {
		t.Fatal(err)
	}

	list, err := svc.ListConversations(ctx, me)
	if err != nil {
		t.Fatal(err)
	}
	byHandle := map[string]buddy.Conversation{}
	for _, c := range list {
		byHandle[c.OtherHandle] = c
	}
	var incomingH string
	if err := pool.QueryRow(ctx, `SELECT handle FROM buddy_members WHERE user_id = $1`, incoming).Scan(&incomingH); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		incomingH: buddy.ForMeIncoming, outgoingH: buddy.ForMeOutgoing, openH: buddy.ForMeOpen,
		closedH: buddy.ForMeClosed, theyDeclinedH: buddy.ForMeOutgoing,
	}
	if len(list) != len(want) {
		t.Fatalf("list has %d entries, want %d: %+v", len(list), len(want), list)
	}
	if _, ok := byHandle[iDeclinedH]; ok {
		t.Fatal("the invitation I declined is listed")
	}
	for h, s := range want {
		if byHandle[h].StatusForMe != s {
			t.Fatalf("%s: status_for_me = %q, want %q", h, byHandle[h].StatusForMe, s)
		}
	}
	if list[0].OtherHandle != openH || list[0].Unread != 1 || list[0].LastMessageAt.IsZero() {
		t.Fatalf("first = %+v, want the open chat with the newest message", list[0])
	}
	if byHandle[incomingH].Unread != 1 {
		t.Fatalf("incoming unread = %d, want 1", byHandle[incomingH].Unread)
	}
	// The member whose invitation was declined still sees it, as unanswered.
	got, err := svc.ListConversations(ctx, iDeclined)
	if err != nil || len(got) != 1 || got[0].StatusForMe != buddy.ForMeOutgoing {
		t.Fatalf("initiator of the declined invitation sees %+v, %v", got, err)
	}
}

func TestListConversationsCarriesTheOtherCard(t *testing.T) {
	pool, svc, a, b, _ := openChat(t)
	seedCV(t, pool, a)
	list, err := svc.ListConversations(context.Background(), b)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Card.Category == "" {
		t.Fatalf("card = %+v", list)
	}
}
