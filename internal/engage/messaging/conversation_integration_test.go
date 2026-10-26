//go:build integration

// Integration tests for starting, reopening and answering conversations.
// Run with: go test -tags=integration ./internal/engage/messaging/
package messaging_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/engage/messaging"
)

func TestStartCreatesPendingConversationWithBothSidesAndFirstMessage(t *testing.T) {
	pool, svc := setup(t)
	ctx := context.Background()
	a := seedUser(t, pool, "a@example.test")
	b := seedUser(t, pool, "b@example.test")

	c, err := start(ctx, svc, a, b, "  Interviewing for the same role?  ")
	if err != nil {
		t.Fatal(err)
	}
	if c.ID == 0 || c.Kind != "buddy" || c.Status != messaging.StatusPending || c.StatusForMe != messaging.ForMeOutgoing ||
		!c.Starter || c.Counterpart.Persona != "buddy" || c.Counterpart.Detail != b {
		t.Fatalf("start = %+v", c)
	}
	var body string
	var sender int64
	if err := pool.QueryRow(ctx, `SELECT body, sender_id FROM messages WHERE conversation_id = $1`, c.ID).Scan(&body, &sender); err != nil {
		t.Fatal(err)
	}
	if body != "Interviewing for the same role?" || sender != a {
		t.Fatalf("first message = %q from %d", body, sender)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM conversation_participants WHERE conversation_id = $1 AND user_id IN ($2, $3)`, c.ID, a, b); n != 2 {
		t.Fatalf("participants = %d, want 2", n)
	}
	list, err := svc.List(ctx, b)
	if err != nil || len(list) != 1 || list[0].StatusForMe != messaging.ForMeIncoming || list[0].Unread != 1 {
		t.Fatalf("recipient's list = %+v, %v", list, err)
	}
}

func TestStartRefusesBadBodiesAndSelf(t *testing.T) {
	pool, svc := setup(t)
	ctx := context.Background()
	a := seedUser(t, pool, "a@example.test")
	b := seedUser(t, pool, "b@example.test")
	for name, body := range map[string]string{
		"empty": "", "blank": " \t ", "too long": strings.Repeat("щ", messaging.MaxMessageRunes+1),
	} {
		if _, err := start(ctx, svc, a, b, body); !errors.Is(err, messaging.ErrInvalidMessage) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if _, err := start(ctx, svc, a, a, "me"); !errors.Is(err, messaging.ErrUnavailable) {
		t.Fatalf("self: %v", err)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM conversations`); n != 0 {
		t.Fatalf("conversations = %d, want 0", n)
	}
}

func TestPairKeyRefusesSecondConversationInEveryStatusButClosed(t *testing.T) {
	pool, svc := setup(t)
	ctx := context.Background()
	a := seedUser(t, pool, "a@example.test")
	b := seedUser(t, pool, "b@example.test")
	c, err := start(ctx, svc, a, b, "hello")
	if err != nil {
		t.Fatal(err)
	}
	for _, status := range []string{messaging.StatusPending, messaging.StatusOpen, messaging.StatusDeclined} {
		if _, err := pool.Exec(ctx, `UPDATE conversations SET status = $2 WHERE id = $1`, c.ID, status); err != nil {
			t.Fatal(err)
		}
		// Either direction finds the same pair.
		if _, err := start(ctx, svc, b, a, "again"); !errors.Is(err, messaging.ErrExists) {
			t.Fatalf("%s: %v, want ErrExists", status, err)
		}
	}
	if n := countRows(t, pool, `SELECT count(*) FROM conversations`); n != 1 {
		t.Fatalf("conversations = %d, want 1", n)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM messages`); n != 1 {
		t.Fatalf("messages = %d, want 1", n)
	}
}

func TestConcurrentStartsOnOnePairMakeOneConversation(t *testing.T) {
	pool, svc := setup(t)
	ctx := context.Background()
	a := seedUser(t, pool, "a@example.test")
	b := seedUser(t, pool, "b@example.test")

	var wg sync.WaitGroup
	errs := make([]error, 8)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if i%2 == 0 {
				_, errs[i] = start(ctx, svc, a, b, "hi")
			} else {
				_, errs[i] = start(ctx, svc, b, a, "hi")
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
			t.Fatalf("racing start failed with %v, want nil or ErrExists", err)
		}
	}
	if created != 1 {
		t.Fatalf("%d starts succeeded, want 1", created)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM messages`); n != 1 {
		t.Fatalf("messages = %d, want 1", n)
	}
}

func TestStartReopensAConversationClosedWithoutABlock(t *testing.T) {
	pool, svc, a, b, id := openChat(t)
	ctx := context.Background()
	if _, err := svc.Send(ctx, b, id, "reply"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE conversations SET status = 'closed', closed_at = now(), created_at = now() - interval '48 hours' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}

	// The recipient of the first round starts the second.
	c, err := start(ctx, svc, b, a, "back again?")
	if err != nil {
		t.Fatal(err)
	}
	if c.ID != id || c.Status != messaging.StatusPending || !c.Starter || c.AcceptedAt != nil {
		t.Fatalf("reopened = %+v, want conversation %d pending, started by B", c, id)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM conversations`); n != 1 {
		t.Fatalf("conversations = %d, want 1", n)
	}
	msgs, err := svc.Messages(ctx, a, id, 0, 50)
	if err != nil || len(msgs) != 3 || msgs[2].Body != "back again?" {
		t.Fatalf("messages = %+v, %v, want the history plus the new one", msgs, err)
	}
	for user, want := range map[int64]int{a: 1, b: 0} {
		list, err := svc.List(ctx, user)
		if err != nil || len(list) != 1 || list[0].Unread != want {
			t.Fatalf("list for %d = %+v, %v, want unread %d", user, list, err, want)
		}
	}
	// It counts against the starter's window again.
	if n, _, err := svc.StartedSince(ctx, "buddy", b, time.Now().Add(-time.Minute)); err != nil || n != 1 {
		t.Fatalf("started since = %d, %v, want 1", n, err)
	}
	if _, err := svc.Accept(ctx, a, id); err != nil {
		t.Fatalf("accepting the reopened request: %v", err)
	}
}

func TestStartAcrossABlockIsRefusedWithoutSayingSo(t *testing.T) {
	pool, svc := setup(t)
	ctx := context.Background()
	a := seedUser(t, pool, "a@example.test")
	b := seedUser(t, pool, "b@example.test")
	if _, err := pool.Exec(ctx, `INSERT INTO user_blocks (blocker_id, blocked_id) VALUES ($1, $2)`, b, a); err != nil {
		t.Fatal(err)
	}
	for _, from := range []int64{a, b} {
		to := a + b - from
		if _, err := start(ctx, svc, from, to, "hi"); !errors.Is(err, messaging.ErrUnavailable) {
			t.Fatalf("start from %d: %v, want ErrUnavailable", from, err)
		}
	}
	if n := countRows(t, pool, `SELECT count(*) FROM conversations`); n != 0 {
		t.Fatalf("conversations = %d, want 0", n)
	}
}

func TestABlockKeepsAClosedConversationClosed(t *testing.T) {
	pool, svc, a, b, id := openChat(t)
	ctx := context.Background()
	if err := svc.Block(ctx, b, id); err != nil {
		t.Fatal(err)
	}
	if _, err := start(ctx, svc, a, b, "hi"); !errors.Is(err, messaging.ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM conversations WHERE id = $1 AND status = 'closed'`, id); n != 1 {
		t.Fatal("the blocked conversation reopened")
	}
}

func TestAcceptOpensAndDeclineIsSilent(t *testing.T) {
	pool, svc := setup(t)
	ctx := context.Background()
	a := seedUser(t, pool, "a@example.test")
	b := seedUser(t, pool, "b@example.test")
	c := seedUser(t, pool, "c@example.test")
	toB, _ := start(ctx, svc, a, b, "hi")
	toC, _ := start(ctx, svc, a, c, "hi")

	opened, err := svc.Accept(ctx, b, toB.ID)
	if err != nil || opened.Status != messaging.StatusOpen || opened.AcceptedAt == nil || opened.Counterpart.Detail != a {
		t.Fatalf("accept = %+v, %v", opened, err)
	}
	if _, err := svc.Send(ctx, a, toB.ID, "great"); err != nil {
		t.Fatalf("send after accept: %v", err)
	}
	if _, err := svc.Decline(ctx, c, toC.ID); err != nil {
		t.Fatal(err)
	}
	list, err := svc.List(ctx, a)
	if err != nil {
		t.Fatal(err)
	}
	for _, cv := range list {
		if cv.ID == toC.ID && cv.StatusForMe != messaging.ForMeOutgoing {
			t.Fatalf("the starter sees the declined request as %q", cv.StatusForMe)
		}
	}
}

func TestOnlyTheRecipientAnswersAndOnlyOnce(t *testing.T) {
	pool, svc := setup(t)
	ctx := context.Background()
	a := seedUser(t, pool, "a@example.test")
	b := seedUser(t, pool, "b@example.test")
	c, err := start(ctx, svc, a, b, "hi")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Accept(ctx, a, c.ID); !errors.Is(err, messaging.ErrState) {
		t.Fatalf("starter accepts: %v", err)
	}
	if _, err := svc.Decline(ctx, a, c.ID); !errors.Is(err, messaging.ErrState) {
		t.Fatalf("starter declines: %v", err)
	}
	if _, err := svc.Accept(ctx, b, c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Decline(ctx, b, c.ID); !errors.Is(err, messaging.ErrState) {
		t.Fatalf("answering twice: %v", err)
	}
}

// A block committing between the recipient's read and their accept must still stop it:
// the answer write checks the block table itself. Written directly so the conversation
// stays pending, as it would in the race.
func TestAnswerWriteRefusesABlockedPair(t *testing.T) {
	pool, svc := setup(t)
	ctx := context.Background()
	a := seedUser(t, pool, "a@example.test")
	b := seedUser(t, pool, "b@example.test")
	c, err := start(ctx, svc, a, b, "hi")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO user_blocks (blocker_id, blocked_id) VALUES ($1, $2)`, a, b); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Accept(ctx, b, c.ID); !errors.Is(err, messaging.ErrState) {
		t.Fatalf("accept across a block = %v, want ErrState", err)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM conversations WHERE status = 'pending'`); n != 1 {
		t.Fatal("the conversation moved")
	}
}

func TestStartedSinceCountsTheKindsStartsInTheWindow(t *testing.T) {
	pool, svc := setup(t)
	ctx := context.Background()
	a := seedUser(t, pool, "a@example.test")
	var ids []int64
	for _, email := range []string{"b@example.test", "c@example.test", "d@example.test"} {
		c, err := start(ctx, svc, a, seedUser(t, pool, email), "hi")
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, c.ID)
	}
	oldest := time.Now().Add(-20 * time.Hour).Truncate(time.Microsecond)
	if _, err := pool.Exec(ctx, `UPDATE conversations SET created_at = $2 WHERE id = $1`, ids[0], oldest); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE conversations SET created_at = now() - interval '30 hours' WHERE id = $1`, ids[1]); err != nil {
		t.Fatal(err)
	}
	n, first, err := svc.StartedSince(ctx, "buddy", a, time.Now().Add(-24*time.Hour))
	if err != nil || n != 2 || !first.Equal(oldest) {
		t.Fatalf("started since = %d, %v, %v; want 2 starting at %v", n, first, err, oldest)
	}
}
