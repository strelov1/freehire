//go:build integration

// Integration tests for the in-app chat.
// Run with: go test -tags=integration ./internal/engage/messaging/
package messaging_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/engage/messaging"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

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
	list, err := svc.List(ctx, b)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Unread != 2 {
		t.Fatalf("B's list = %+v, want unread 2", list)
	}
	// The sender never sees their own message as unread.
	mine, err := svc.List(ctx, a)
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
	a := seedUser(t, pool, "a@example.test")
	b := seedUser(t, pool, "b@example.test")
	third := seedUser(t, pool, "c@example.test")
	c, err := start(ctx, svc, a, b, "hello")
	if err != nil {
		t.Fatal(err)
	}
	// Pending: refused for the inviter and for the recipient alike.
	for name, u := range map[string]int64{"inviter": a, "recipient": b} {
		if _, err := svc.Send(ctx, u, c.ID, "more"); !errors.Is(err, messaging.ErrState) {
			t.Fatalf("pending, %s: %v", name, err)
		}
	}
	if _, err := svc.Send(ctx, third, c.ID, "hi"); !errors.Is(err, messaging.ErrNotFound) {
		t.Fatalf("outsider: %v", err)
	}
	if _, err := svc.Send(ctx, a, c.ID+1000, "hi"); !errors.Is(err, messaging.ErrNotFound) {
		t.Fatalf("unknown: %v", err)
	}
	if _, err := svc.Decline(ctx, b, c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Send(ctx, b, c.ID, "hi"); !errors.Is(err, messaging.ErrState) {
		t.Fatalf("declined: %v", err)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM messages`); n != 1 {
		t.Fatalf("messages = %d, want 1", n)
	}
}

func TestSendRefusesBadBodiesAndClosedChats(t *testing.T) {
	pool, svc, a, b, id := openChat(t)
	ctx := context.Background()
	for name, body := range map[string]string{
		"empty": "", "blank": "  \n", "too long": strings.Repeat("x", messaging.MaxMessageRunes+1),
		"too long in runes": strings.Repeat("щ", messaging.MaxMessageRunes+1),
	} {
		if _, err := svc.Send(ctx, a, id, body); !errors.Is(err, messaging.ErrInvalidMessage) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if _, err := svc.Send(ctx, a, id, strings.Repeat("щ", messaging.MaxMessageRunes)); err != nil {
		t.Fatalf("4000 runes: %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE conversations SET status = 'closed' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Send(ctx, b, id, "hi"); !errors.Is(err, messaging.ErrState) {
		t.Fatalf("closed: %v", err)
	}
	// A closed chat's history stays readable to both parties.
	msgs, err := svc.Messages(ctx, b, id, 0, 0)
	if err != nil || len(msgs) != 2 {
		t.Fatalf("closed history = %+v, %v", msgs, err)
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
	if _, err := tx.Exec(ctx, `UPDATE conversations SET status = 'closed' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := svc.Send(ctx, a, id, "too late")
		done <- err
	}()
	// Let the send reach the row lock before the close commits.
	deadline := time.Now().Add(5 * time.Second)
	for countRows(t, pool, `SELECT count(*) FROM pg_stat_activity WHERE wait_event_type = 'Lock' AND query LIKE '%messages%'`) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the send never blocked behind the close")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err := <-done; !errors.Is(err, messaging.ErrState) {
		t.Fatalf("send behind a close: %v, want ErrConversationState", err)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM messages WHERE conversation_id = $1`, id); n != 1 {
		t.Fatalf("messages = %d, want only the invitation", n)
	}
}

// Every operation an outsider can name answers exactly as for a conversation that does not
// exist, and changes nothing.
func TestOutsiderCannotReachAConversation(t *testing.T) {
	pool, svc, _, b, open := openChat(t)
	ctx := context.Background()
	c := seedUser(t, pool, "c@example.test")
	pending, err := start(ctx, svc, b, seedUser(t, pool, "d@example.test"), "hi")
	if err != nil {
		t.Fatal(err)
	}
	ops := map[string]func(id int64) error{
		"Messages": func(id int64) error { _, err := svc.Messages(ctx, c, id, 0, 0); return err },
		"Send":     func(id int64) error { _, err := svc.Send(ctx, c, id, "hi"); return err },
		"MarkRead": func(id int64) error { return svc.MarkRead(ctx, c, id, 1) },
		"Accept":   func(id int64) error { _, err := svc.Accept(ctx, c, id); return err },
		"Decline":  func(id int64) error { _, err := svc.Decline(ctx, c, id); return err },
		"Block":    func(id int64) error { return svc.Block(ctx, c, id) },
		"Report":   func(id int64) error { return svc.Report(ctx, c, id, "spam", "") },
	}
	for name, op := range ops {
		for _, id := range []int64{open, pending.ID, open + 1000} {
			if err := op(id); !errors.Is(err, messaging.ErrNotFound) {
				t.Fatalf("%s on %d: %v, want ErrNotFound", name, id, err)
			}
		}
	}
	if n := countRows(t, pool, `SELECT count(*) FROM user_blocks`) + countRows(t, pool, `SELECT count(*) FROM conversation_reports`); n != 0 {
		t.Fatalf("an outsider wrote %d block/report rows", n)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM messages`); n != 2 {
		t.Fatalf("messages = %d, want the two first messages", n)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM conversations WHERE status = 'pending'`); n != 1 {
		t.Fatal("an outsider answered a request")
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
	list, err := svc.List(ctx, b)
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
		list, err := svc.List(ctx, b)
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
	me := seedUser(t, pool, "me@example.test")
	incoming := seedUser(t, pool, "in@example.test")
	outgoing := seedUser(t, pool, "out@example.test")
	open := seedUser(t, pool, "open@example.test")
	closedU := seedUser(t, pool, "closed@example.test")
	iDeclined := seedUser(t, pool, "d1@example.test")
	theyDeclined := seedUser(t, pool, "d2@example.test")

	invite := func(from, to int64) int64 {
		t.Helper()
		c, err := start(ctx, svc, from, to, "hi")
		if err != nil {
			t.Fatal(err)
		}
		return c.ID
	}
	invite(incoming, me)
	invite(me, outgoing)
	openID := invite(me, open)
	closedID := invite(me, closedU)
	iDeclinedID := invite(iDeclined, me)
	theyDeclinedID := invite(me, theyDeclined)
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
	if _, err := pool.Exec(ctx, `UPDATE conversations SET status = 'closed' WHERE id = $1`, closedID); err != nil {
		t.Fatal(err)
	}
	// The open chat gets the newest message, so it leads the list.
	if _, err := svc.Send(ctx, open, openID, "latest"); err != nil {
		t.Fatal(err)
	}

	list, err := svc.List(ctx, me)
	if err != nil {
		t.Fatal(err)
	}
	// The stub kind draws the other participant as their account id.
	byOther := map[int64]messaging.Conversation{}
	for _, c := range list {
		if c.Kind != "buddy" || c.Counterpart.Persona != "buddy" || !c.Counterpart.Reachable {
			t.Fatalf("entry = %+v", c)
		}
		byOther[c.Counterpart.Detail.(int64)] = c
	}
	want := map[int64]string{
		incoming: messaging.ForMeIncoming, outgoing: messaging.ForMeOutgoing, open: messaging.ForMeOpen,
		closedU: messaging.ForMeClosed, theyDeclined: messaging.ForMeOutgoing,
	}
	if len(list) != len(want) {
		t.Fatalf("list has %d entries, want %d: %+v", len(list), len(want), list)
	}
	if _, ok := byOther[iDeclined]; ok {
		t.Fatal("the request I declined is listed")
	}
	for u, s := range want {
		if byOther[u].StatusForMe != s {
			t.Fatalf("%d: status_for_me = %q, want %q", u, byOther[u].StatusForMe, s)
		}
	}
	if list[0].ID != openID || list[0].Unread != 1 || list[0].LastMessageAt.IsZero() {
		t.Fatalf("first = %+v, want the open chat with the newest message", list[0])
	}
	if byOther[incoming].Unread != 1 {
		t.Fatalf("incoming unread = %d, want 1", byOther[incoming].Unread)
	}
	// The starter of the declined request still sees it, as unanswered.
	got, err := svc.List(ctx, iDeclined)
	if err != nil || len(got) != 1 || got[0].StatusForMe != messaging.ForMeOutgoing {
		t.Fatalf("starter of the declined request sees %+v, %v", got, err)
	}
}

func TestUnreachableCounterpartServesNothing(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	a := seedUser(t, pool, "a@example.test")
	b := seedUser(t, pool, "b@example.test")
	// The persona hides B from everybody, yet tries to hand over a detail anyway.
	svc := messaging.New(messaging.NewQueriesRepository(db.New(pool)), stubPersona{hidden: map[int64]bool{b: true}})
	c, err := start(ctx, svc, a, b, "hi")
	if err != nil {
		t.Fatal(err)
	}
	bare := messaging.Counterpart{Persona: "buddy"}
	if c.Counterpart != bare {
		t.Fatalf("start answered counterpart %+v", c.Counterpart)
	}
	list, err := svc.List(ctx, a)
	if err != nil || len(list) != 1 || list[0].Counterpart != bare {
		t.Fatalf("list = %+v, %v", list, err)
	}
	// A persona the service was not built with still lists, as unreachable.
	unregistered := messaging.New(messaging.NewQueriesRepository(db.New(pool)))
	list, err = unregistered.List(ctx, b)
	if err != nil || len(list) != 1 || list[0].Counterpart != bare {
		t.Fatalf("unregistered persona = %+v, %v", list, err)
	}
}

func TestRecipientReadsAPendingInvitation(t *testing.T) {
	pool, svc := setup(t)
	ctx := context.Background()
	a := seedUser(t, pool, "a@example.test")
	b := seedUser(t, pool, "b@example.test")
	c, err := start(ctx, svc, a, b, "hello there")
	if err != nil {
		t.Fatal(err)
	}
	for name, u := range map[string]int64{"inviter": a, "recipient": b} {
		got, err := svc.Messages(ctx, u, c.ID, 0, 10)
		if err != nil || len(got) != 1 || got[0].Body != "hello there" {
			t.Fatalf("%s: %+v, %v", name, got, err)
		}
	}
}
