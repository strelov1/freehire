//go:build integration

// Integration tests for blocks, reports and account deletion.
// Run with: go test -tags=integration ./internal/engage/messaging/
package messaging_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/engage/messaging"
)

func TestBlockClosesEveryConversationBetweenThePair(t *testing.T) {
	pool, svc, a, b, id := openChat(t)
	ctx := context.Background()
	// A second conversation between the same two, under no pair key, and one with a third
	// account that the block must not touch.
	second, err := svc.Start(ctx, messaging.StartRequest{
		Kind: "buddy", StarterID: b, StarterPersona: "buddy", RecipientID: a, RecipientPersona: "buddy", Body: "other thread",
	})
	if err != nil {
		t.Fatal(err)
	}
	c := seedUser(t, pool, "c@example.test")
	other, err := start(ctx, svc, a, c, "hi")
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.Block(ctx, a, id); err != nil {
		t.Fatal(err)
	}
	if err := svc.Block(ctx, a, id); err != nil {
		t.Fatalf("second block: %v", err)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM user_blocks WHERE blocker_id = $1 AND blocked_id = $2`, a, b); n != 1 {
		t.Fatalf("block rows = %d", n)
	}
	for _, cid := range []int64{id, second.ID} {
		if n := countRows(t, pool, `SELECT count(*) FROM conversations WHERE id = $1 AND status = 'closed' AND closed_at IS NOT NULL`, cid); n != 1 {
			t.Fatalf("conversation %d not closed", cid)
		}
		for _, u := range []int64{a, b} {
			if _, err := svc.Send(ctx, u, cid, "hi"); !errors.Is(err, messaging.ErrState) {
				t.Fatalf("send by %d into %d: %v", u, cid, err)
			}
		}
	}
	if n := countRows(t, pool, `SELECT count(*) FROM conversations WHERE id = $1 AND status = 'pending'`, other.ID); n != 1 {
		t.Fatal("the block closed a conversation with someone else")
	}
	if blocked, err := svc.Blocked(ctx, b, a); err != nil || !blocked {
		t.Fatalf("Blocked(b, a) = %v, %v", blocked, err)
	}
	// The history stays readable.
	if msgs, err := svc.Messages(ctx, b, id, 0, 0); err != nil || len(msgs) != 1 {
		t.Fatalf("history = %+v, %v", msgs, err)
	}
}

func TestBlockClosesAPendingRequest(t *testing.T) {
	pool, svc := setup(t)
	ctx := context.Background()
	a := seedUser(t, pool, "a@example.test")
	b := seedUser(t, pool, "b@example.test")
	c, err := start(ctx, svc, a, b, "hello")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Report(ctx, b, c.ID, "spam", "unsolicited"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Block(ctx, b, c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Accept(ctx, b, c.ID); !errors.Is(err, messaging.ErrState) {
		t.Fatalf("accept after block: %v", err)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM conversation_reports`); n != 1 {
		t.Fatalf("reports = %d, want 1", n)
	}
}

func TestReportValidatesAndIsOncePerReporter(t *testing.T) {
	pool, svc, a, b, id := openChat(t)
	ctx := context.Background()
	for name, c := range map[string]struct{ reason, note string }{
		"unknown reason": {"rude", ""},
		"empty reason":   {"", ""},
		"long note":      {"spam", strings.Repeat("щ", messaging.MaxReportNoteRunes+1)},
	} {
		if err := svc.Report(ctx, a, id, c.reason, c.note); !errors.Is(err, messaging.ErrInvalidReport) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if err := svc.Report(ctx, a, id, "spam", strings.Repeat("щ", messaging.MaxReportNoteRunes)); err != nil {
		t.Fatalf("1000-rune note: %v", err)
	}
	if err := svc.Report(ctx, a, id, "scam", "again"); !errors.Is(err, messaging.ErrAlreadyReported) {
		t.Fatalf("duplicate: %v", err)
	}
	// The other side may report it too, and a report neither blocks nor closes.
	if err := svc.Report(ctx, b, id, "harassment", "  rude  "); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM conversation_reports WHERE conversation_id = $1`, id); n != 2 {
		t.Fatalf("reports = %d", n)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM conversation_reports WHERE note = 'rude'`); n != 1 {
		t.Fatal("note not trimmed")
	}
	if n := countRows(t, pool, `SELECT count(*) FROM user_blocks`); n != 0 {
		t.Fatal("a report blocked")
	}
	if _, err := svc.Send(ctx, a, id, "still open"); err != nil {
		t.Fatalf("a report closed the chat: %v", err)
	}
}

func TestModeratorListsReportsInThePersonasNamesAndResolves(t *testing.T) {
	pool, svc, a, b, id := openChat(t)
	ctx := context.Background()
	moderator := seedUser(t, pool, "mod@example.test")
	if _, err := svc.Send(ctx, b, id, "send me money"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Report(ctx, a, id, "scam", "asked for money"); err != nil {
		t.Fatal(err)
	}

	reports, err := svc.ListReports(ctx, 0, 0, false)
	if err != nil || len(reports) != 1 {
		t.Fatalf("reports = %+v, %v", reports, err)
	}
	r := reports[0]
	aName, bName := fmt.Sprintf("user-%d", a), fmt.Sprintf("user-%d", b)
	if r.Kind != "buddy" || r.Reason != "scam" || r.Note != "asked for money" || r.Reporter != aName || r.Reported != bName ||
		r.ConversationID != id || r.ResolvedAt != nil || r.CreatedAt.IsZero() {
		t.Fatalf("report = %+v", r)
	}
	if len(r.Messages) != 2 || r.Messages[0].Sender != aName || r.Messages[0].Body != "hello" ||
		r.Messages[1].Sender != bName || r.Messages[1].Body != "send me money" {
		t.Fatalf("messages = %+v", r.Messages)
	}

	if err := svc.ResolveReport(ctx, moderator, r.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.ResolveReport(ctx, moderator, r.ID); err != nil {
		t.Fatalf("second resolve: %v", err)
	}
	if err := svc.ResolveReport(ctx, moderator, r.ID+1000); !errors.Is(err, messaging.ErrNotFound) {
		t.Fatalf("unknown report: %v", err)
	}
	if open, err := svc.ListReports(ctx, 0, 0, false); err != nil || len(open) != 0 {
		t.Fatalf("open after resolve = %+v, %v", open, err)
	}
	if all, err := svc.ListReports(ctx, 0, 0, true); err != nil || len(all) != 1 || all[0].ResolvedAt == nil {
		t.Fatalf("all = %+v, %v", all, err)
	}
}

// Deleting either account deletes every conversation it took part in, with all its
// messages and reports, and every block it is a side of — whichever side started it.
func TestAccountDeletionRemovesItsConversations(t *testing.T) {
	pool, svc, a, b, id := openChat(t)
	ctx := context.Background()
	c := seedUser(t, pool, "c@example.test")
	started, err := start(ctx, svc, c, b, "hi") // B is the recipient here
	if err != nil {
		t.Fatal(err)
	}
	untouched, err := start(ctx, svc, a, c, "hi")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Report(ctx, a, id, "spam", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO user_blocks (blocker_id, blocked_id) VALUES ($1, $2), ($2, $3)`, a, b, c); err != nil {
		t.Fatal(err)
	}

	if _, err := pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, b); err != nil {
		t.Fatal(err)
	}
	for _, cid := range []int64{id, started.ID} {
		if n := countRows(t, pool, `SELECT count(*) FROM conversations WHERE id = $1`, cid); n != 0 {
			t.Fatalf("conversation %d survived its participant's deletion", cid)
		}
		if n := countRows(t, pool, `SELECT count(*) FROM messages WHERE conversation_id = $1`, cid); n != 0 {
			t.Fatalf("messages of %d survived", cid)
		}
	}
	if n := countRows(t, pool, `SELECT count(*) FROM conversation_reports`); n != 0 {
		t.Fatalf("reports = %d, want 0", n)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM user_blocks WHERE $1 IN (blocker_id, blocked_id)`, b); n != 0 {
		t.Fatalf("blocks = %d, want 0", n)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM conversations WHERE id = $1`, untouched.ID); n != 1 {
		t.Fatal("a conversation between two other accounts was deleted")
	}
	if list, err := svc.List(ctx, a); err != nil || len(list) != 1 || list[0].ID != untouched.ID {
		t.Fatalf("A's list = %+v, %v", list, err)
	}
}
