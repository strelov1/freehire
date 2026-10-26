//go:build integration

// Integration tests for what a block and a report mean for buddies: the block is the
// messaging core's, but it must hide the pair on every buddy surface.
// Run with: go test -tags=integration ./internal/engage/buddy/
package buddy_test

import (
	"context"
	"errors"
	"testing"

	"github.com/strelov1/freehire/internal/engage/buddy"
	"github.com/strelov1/freehire/internal/engage/messaging"
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

	if err := chat(pool).Block(ctx, a, id); err != nil {
		t.Fatal(err)
	}
	// Idempotent.
	if err := chat(pool).Block(ctx, a, id); err != nil {
		t.Fatalf("second block: %v", err)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM user_blocks WHERE blocker_id = $1 AND blocked_id = $2`, a, b); n != 1 {
		t.Fatalf("block rows = %d", n)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM conversations WHERE id = $1 AND status = 'closed' AND closed_at IS NOT NULL`, id); n != 1 {
		t.Fatalf("conversation not closed")
	}

	// Neither side can send.
	for _, u := range []int64{a, b} {
		if _, err := chat(pool).Send(ctx, u, id, "hi"); !errors.Is(err, messaging.ErrState) {
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
	if msgs, err := chat(pool).Messages(ctx, b, id, 0, 0); err != nil || len(msgs) != 1 {
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
	if err := chat(pool).Block(ctx, b, id); err != nil {
		t.Fatal(err)
	}
	for _, u := range []int64{a, b} {
		page, err := svc.Feed(ctx, u, buddy.FeedAll, "", 50)
		if err != nil || len(page.Entries) != 0 {
			t.Fatalf("feed for %d after block = %+v, %v", u, page, err)
		}
	}
}

// A moderator reads a buddy conversation by the handles its members saw, never by account.
func TestModeratorReadsBuddyReportsByHandle(t *testing.T) {
	pool, svc, a, b, id := openChat(t)
	ctx := context.Background()
	moderator := seedUser(t, pool, "mod@example.test")
	aHandle, bHandle := handleOf(t, svc, a), handleOf(t, svc, b)
	if _, err := chat(pool).Send(ctx, b, id, "send me money"); err != nil {
		t.Fatal(err)
	}
	if err := chat(pool).Report(ctx, a, id, "scam", "asked for money"); err != nil {
		t.Fatal(err)
	}

	reports, err := chat(pool).ListReports(ctx, 0, 0, false)
	if err != nil || len(reports) != 1 {
		t.Fatalf("reports = %+v, %v", reports, err)
	}
	r := reports[0]
	if r.Reason != "scam" || r.Note != "asked for money" || r.Reporter != aHandle || r.Reported != bHandle ||
		r.ConversationID != id || r.ResolvedAt != nil || r.CreatedAt.IsZero() {
		t.Fatalf("report = %+v", r)
	}
	if len(r.Messages) != 2 || r.Messages[0].Sender != aHandle || r.Messages[0].Body != "hello" ||
		r.Messages[1].Sender != bHandle || r.Messages[1].Body != "send me money" {
		t.Fatalf("messages = %+v", r.Messages)
	}

	if err := chat(pool).ResolveReport(ctx, moderator, r.ID); err != nil {
		t.Fatal(err)
	}
	if err := chat(pool).ResolveReport(ctx, moderator, r.ID); err != nil {
		t.Fatalf("second resolve: %v", err)
	}
	if err := chat(pool).ResolveReport(ctx, moderator, r.ID+1000); !errors.Is(err, messaging.ErrNotFound) {
		t.Fatalf("unknown report: %v", err)
	}
	open, err := chat(pool).ListReports(ctx, 0, 0, false)
	if err != nil || len(open) != 0 {
		t.Fatalf("open after resolve = %+v, %v", open, err)
	}
	all, err := chat(pool).ListReports(ctx, 0, 0, true)
	if err != nil || len(all) != 1 || all[0].ResolvedAt == nil {
		t.Fatalf("all = %+v, %v", all, err)
	}
}
