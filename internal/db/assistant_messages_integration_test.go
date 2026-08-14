//go:build integration

// Integration test for ListRecentAssistantMessages — the bounded counterpart of
// ListAssistantMessages added so Runner.history() (internal/assistant) can rebuild the
// model's context from just the tail of a session's transcript instead of fetching and
// JSON-decoding the whole thing on every turn. Run with:
// go test -tags=integration ./internal/db/
package db

import (
	"context"
	"testing"
)

func TestListRecentAssistantMessagesReturnsNewestFirstLimited(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	owner := seedAssistantUser(t, pool, "recent-transcript@example.test")
	sess, err := q.CreateAssistantSession(ctx, CreateAssistantSessionParams{UserID: owner, Preset: "chat"})
	if err != nil {
		t.Fatalf("CreateAssistantSession: %v", err)
	}

	for i := 0; i < 5; i++ {
		if _, err := q.AppendAssistantMessage(ctx, AppendAssistantMessageParams{
			SessionID: sess.ID, Role: "user", Content: []byte(`{"text":"m"}`),
		}); err != nil {
			t.Fatalf("append message %d: %v", i, err)
		}
	}

	rows, err := q.ListRecentAssistantMessages(ctx, ListRecentAssistantMessagesParams{SessionID: sess.ID, Limit: 3})
	if err != nil {
		t.Fatalf("ListRecentAssistantMessages: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3", len(rows))
	}
	// Newest first: seq 5, 4, 3 — the caller (assistant.Store.RecentTranscript) is what
	// reverses this back to ascending order.
	wantSeqs := []int32{5, 4, 3}
	for i, want := range wantSeqs {
		if rows[i].Seq != want {
			t.Errorf("rows[%d].Seq = %d, want %d", i, rows[i].Seq, want)
		}
	}

	// A limit at or above the whole transcript returns everything, still newest first.
	all, err := q.ListRecentAssistantMessages(ctx, ListRecentAssistantMessagesParams{SessionID: sess.ID, Limit: 100})
	if err != nil {
		t.Fatalf("ListRecentAssistantMessages(100): %v", err)
	}
	if len(all) != 5 {
		t.Fatalf("got %d rows, want all 5 when the limit exceeds the transcript length", len(all))
	}
}
