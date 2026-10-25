//go:build integration

// Integration tests for blocking and reporting.
// Run with: go test -tags=integration ./internal/engage/buddy/
package buddy_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/engage/buddy"
)

func handleOf(t *testing.T, svc *buddy.Service, userID int64) string {
	t.Helper()
	st, err := svc.Status(context.Background(), userID)
	if err != nil {
		t.Fatal(err)
	}
	return st.Handle
}

func TestBlockClosesTheChatAndHidesThePairEverywhere(t *testing.T) {
	pool, svc, a, b, id := openChat(t)
	ctx := context.Background()
	aHandle, bHandle := handleOf(t, svc, a), handleOf(t, svc, b)
	seedCV(t, pool, a)
	seedCV(t, pool, b)
	for _, u := range []int64{a, b} {
		if err := svc.Touch(ctx, u); err != nil {
			t.Fatal(err)
		}
	}
	if sim, err := svc.Similar(ctx, a, 50, 0); err != nil || len(sim) != 1 {
		t.Fatalf("precondition: similar = %+v, %v", sim, err)
	}

	if err := svc.Block(ctx, a, id); err != nil {
		t.Fatal(err)
	}
	// Idempotent.
	if err := svc.Block(ctx, a, id); err != nil {
		t.Fatalf("second block: %v", err)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM buddy_blocks WHERE blocker_id = $1 AND blocked_id = $2`, a, b); n != 1 {
		t.Fatalf("block rows = %d", n)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM buddy_conversations WHERE id = $1 AND status = 'closed' AND closed_at IS NOT NULL`, id); n != 1 {
		t.Fatalf("conversation not closed")
	}

	// Neither side can send.
	for _, u := range []int64{a, b} {
		if _, err := svc.Send(ctx, u, id, "hi"); !errors.Is(err, buddy.ErrConversationState) {
			t.Fatalf("send by %d: %v", u, err)
		}
	}
	// Neither side can invite the other.
	if _, err := svc.Invite(ctx, a, bHandle, "again"); !errors.Is(err, buddy.ErrRecipientUnavailable) {
		t.Fatalf("blocker invites: %v", err)
	}
	if _, err := svc.Invite(ctx, b, aHandle, "again"); !errors.Is(err, buddy.ErrRecipientUnavailable) {
		t.Fatalf("blocked invites: %v", err)
	}
	// Similar and the profile hide both ways; the profile reads as an unknown handle.
	for _, c := range []struct {
		viewer int64
		target string
	}{{a, bHandle}, {b, aHandle}} {
		sim, err := svc.Similar(ctx, c.viewer, 50, 0)
		if err != nil || len(sim) != 0 {
			t.Fatalf("similar for %d = %+v, %v", c.viewer, sim, err)
		}
		if _, err := svc.Profile(ctx, c.viewer, c.target); !errors.Is(err, buddy.ErrNotFound) {
			t.Fatalf("profile for %d: %v", c.viewer, err)
		}
	}
	// The history stays readable.
	if msgs, err := svc.Messages(ctx, b, id, 0, 0); err != nil || len(msgs) != 1 {
		t.Fatalf("history = %+v, %v", msgs, err)
	}
}

func TestBlockHidesTheFeedBothWays(t *testing.T) {
	pool, svc, a, b, id := openChat(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `INSERT INTO companies (slug, name) VALUES ('acme', 'Acme Corp')`); err != nil {
		t.Fatal(err)
	}
	for _, u := range []int64{a, b} {
		event(t, pool, u, newApp(t, pool, u, "acme"), "acme", "applied", "", 5)
	}
	for _, u := range []int64{a, b} {
		page, err := svc.Feed(ctx, u, buddy.FeedAll, "", 50)
		if err != nil || len(page.Entries) != 1 {
			t.Fatalf("precondition: feed for %d = %+v, %v", u, page, err)
		}
	}
	if err := svc.Block(ctx, b, id); err != nil {
		t.Fatal(err)
	}
	for _, u := range []int64{a, b} {
		page, err := svc.Feed(ctx, u, buddy.FeedAll, "", 50)
		if err != nil || len(page.Entries) != 0 {
			t.Fatalf("feed for %d after block = %+v, %v", u, page, err)
		}
	}
}

func TestBlockIsForPartiesOfAnActiveMemberOnly(t *testing.T) {
	pool, svc, a, _, id := openChat(t)
	ctx := context.Background()
	third, _ := member(t, pool, svc, "c@example.test")
	outsider := seedUser(t, pool, "x@example.test")

	if err := svc.Block(ctx, third, id); !errors.Is(err, buddy.ErrNotFound) {
		t.Fatalf("non-party: %v", err)
	}
	if err := svc.Block(ctx, a, id+1000); !errors.Is(err, buddy.ErrNotFound) {
		t.Fatalf("unknown conversation: %v", err)
	}
	if err := svc.Block(ctx, outsider, id); !errors.Is(err, buddy.ErrNotMember) {
		t.Fatalf("non-member: %v", err)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM buddy_blocks`); n != 0 {
		t.Fatalf("a refused block wrote %d rows", n)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM buddy_conversations WHERE id = $1 AND status = 'open'`, id); n != 1 {
		t.Fatal("a refused block closed the conversation")
	}
}

func TestBlockClosesAPendingInvitation(t *testing.T) {
	pool, svc := setup(t)
	ctx := context.Background()
	a, _ := member(t, pool, svc, "a@example.test")
	b, bHandle := member(t, pool, svc, "b@example.test")
	c, err := svc.Invite(ctx, a, bHandle, "hello")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Block(ctx, b, c.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Accept(ctx, b, c.ID); !errors.Is(err, buddy.ErrConversationState) {
		t.Fatalf("accept after block: %v", err)
	}
}

func TestReportValidatesAndIsOncePerReporter(t *testing.T) {
	pool, svc, a, b, id := openChat(t)
	ctx := context.Background()
	third, _ := member(t, pool, svc, "c@example.test")

	for name, c := range map[string]struct{ reason, note string }{
		"unknown reason": {"rude", ""},
		"empty reason":   {"", ""},
		"long note":      {"spam", strings.Repeat("щ", buddy.MaxReportNoteRunes+1)},
	} {
		if err := svc.Report(ctx, a, id, c.reason, c.note); !errors.Is(err, buddy.ErrInvalidReport) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if err := svc.Report(ctx, third, id, "spam", ""); !errors.Is(err, buddy.ErrNotFound) {
		t.Fatalf("non-party: %v", err)
	}
	if err := svc.Report(ctx, a, id, "spam", strings.Repeat("щ", buddy.MaxReportNoteRunes)); err != nil {
		t.Fatalf("1000-rune note: %v", err)
	}
	if err := svc.Report(ctx, a, id, "scam", "again"); !errors.Is(err, buddy.ErrAlreadyReported) {
		t.Fatalf("duplicate: %v", err)
	}
	// The other party may report the same conversation, and reporting alone neither blocks nor closes.
	if err := svc.Report(ctx, b, id, "harassment", "  rude  "); err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM buddy_reports WHERE conversation_id = $1`, id); n != 2 {
		t.Fatalf("reports = %d", n)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM buddy_reports WHERE note = 'rude'`); n != 1 {
		t.Fatal("note not trimmed")
	}
	if n := countRows(t, pool, `SELECT count(*) FROM buddy_blocks`); n != 0 {
		t.Fatal("a report blocked")
	}
	if _, err := svc.Send(ctx, a, id, "still open"); err != nil {
		t.Fatalf("a report closed the chat: %v", err)
	}
}

func TestModeratorListsReportsWithMessagesAndResolves(t *testing.T) {
	pool, svc, a, b, id := openChat(t)
	ctx := context.Background()
	moderator := seedUser(t, pool, "mod@example.test")
	aHandle, bHandle := handleOf(t, svc, a), handleOf(t, svc, b)
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
	if r.Reason != "scam" || r.Note != "asked for money" || r.ReporterHandle != aHandle || r.ReportedHandle != bHandle ||
		r.ConversationID != id || r.ResolvedAt != nil || r.CreatedAt.IsZero() {
		t.Fatalf("report = %+v", r)
	}
	if len(r.Messages) != 2 || r.Messages[0].SenderHandle != aHandle || r.Messages[0].Body != "hello" ||
		r.Messages[1].SenderHandle != bHandle || r.Messages[1].Body != "send me money" {
		t.Fatalf("messages = %+v", r.Messages)
	}

	if err := svc.ResolveReport(ctx, moderator, r.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.ResolveReport(ctx, moderator, r.ID); err != nil {
		t.Fatalf("second resolve: %v", err)
	}
	if err := svc.ResolveReport(ctx, moderator, r.ID+1000); !errors.Is(err, buddy.ErrNotFound) {
		t.Fatalf("unknown report: %v", err)
	}
	open, err := svc.ListReports(ctx, 0, 0, false)
	if err != nil || len(open) != 0 {
		t.Fatalf("open after resolve = %+v, %v", open, err)
	}
	all, err := svc.ListReports(ctx, 0, 0, true)
	if err != nil || len(all) != 1 || all[0].ResolvedAt == nil {
		t.Fatalf("all = %+v, %v", all, err)
	}
}
