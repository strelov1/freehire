//go:build integration

// Integration tests for the job-alert tools of the signed-in MCP server: subscribe through
// the website's own use cases, the Narrows refusal, the channel rule, listing, reading the
// match ledger, and the owner scope of unsubscribe.
package handler

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/platform/db"
)

func newSubscriptionMCPFixture(t *testing.T) (*pgxpool.Pool, *assistantHandlers) {
	t.Helper()
	pool := startPostgres(t)
	return pool, &assistantHandlers{queries: db.New(pool)}
}

func countSubscriptionRows(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), sql, args...).Scan(&n); err != nil {
		t.Fatalf("count: %v", err)
	}
	return n
}

func TestSubscriptionMCP_RefusesASearchThatNarrowsNothing(t *testing.T) {
	pool, h := newSubscriptionMCPFixture(t)
	userID := seedOAuthUser(t, pool, "mcp-subs-narrow@example.test")
	mcpc := newMCPToolCaller(t, h, userID)

	if text, failed := mcpc.call("subscribe", map[string]any{}); !failed || !strings.Contains(text, "add at least one filter") {
		t.Fatalf("empty subscribe = %q failed=%v, want the unfiltered refusal", text, failed)
	}
	// A value outside the skills vocabulary is dropped, which leaves nothing — refused, and
	// the dropped value is named so the agent can fix it.
	text, failed := mcpc.call("subscribe", map[string]any{"filters": map[string]any{"skills": []string{"golangg"}}})
	if !failed || !strings.Contains(text, "add at least one filter") || !strings.Contains(text, "skills=golangg") {
		t.Fatalf("unknown-only subscribe = %q failed=%v, want the refusal naming skills=golangg", text, failed)
	}
	if n := countSubscriptionRows(t, pool, `SELECT count(*) FROM saved_searches WHERE user_id = $1`, userID); n != 0 {
		t.Fatalf("a refused subscribe left %d saved searches behind", n)
	}
}

func TestSubscriptionMCP_NeedsADeliverableChannel(t *testing.T) {
	pool, h := newSubscriptionMCPFixture(t)
	userID := seedOAuthUser(t, pool, "mcp-subs-channel@example.test")
	mcpc := newMCPToolCaller(t, h, userID)
	search := map[string]any{"filters": map[string]any{"skills": []string{"go"}}}

	// Telegram is not linked: asking for it names what IS connected.
	text, failed := mcpc.call("subscribe", map[string]any{"filters": search["filters"], "channels": []string{"telegram"}})
	if !failed || !strings.Contains(text, "connected: email") {
		t.Fatalf("telegram subscribe = %q failed=%v, want a refusal naming email", text, failed)
	}

	// Alert mail off and nothing else connected: nothing is created, and the person is
	// pointed at the settings.
	if err := h.queries.SetEmailGroupSwitches(context.Background(), db.SetEmailGroupSwitchesParams{
		UserID: userID, AlertsEmailEnabled: false, NewsEmailEnabled: true,
	}); err != nil {
		t.Fatalf("turn alert mail off: %v", err)
	}
	text, failed = mcpc.call("subscribe", search)
	if !failed || !strings.Contains(text, "/my/integrations") || !strings.Contains(text, "/my/notifications/settings") {
		t.Fatalf("no-channel subscribe = %q failed=%v, want a refusal with the settings links", text, failed)
	}
	if n := countSubscriptionRows(t, pool, `SELECT count(*) FROM subscriptions WHERE user_id = $1`, userID); n != 0 {
		t.Fatalf("a subscription nobody can receive was created (%d)", n)
	}
}

func TestSubscriptionMCP_SubscribeListUpdatesUnsubscribe(t *testing.T) {
	pool, h := newSubscriptionMCPFixture(t)
	ctx := context.Background()
	alice := seedOAuthUser(t, pool, "mcp-subs-alice@example.test")
	bob := seedOAuthUser(t, pool, "mcp-subs-bob@example.test")
	a := newMCPToolCaller(t, h, alice)
	b := newMCPToolCaller(t, h, bob)

	args := map[string]any{"filters": map[string]any{
		"company_slug": []string{"stripe"},
		"skills":       []string{"go", "golangg"},
	}}
	got := a.ok("subscribe", args)
	subs := got["subscriptions"].([]any)
	if len(subs) != 1 || subs[0].(map[string]any)["channel"] != "email" || subs[0].(map[string]any)["status"] != "created" {
		t.Fatalf("subscriptions = %v, want one created email subscription", subs)
	}
	if ig := got["ignored_params"].([]any); len(ig) != 1 || ig[0] != "skills=golangg" {
		t.Fatalf("ignored_params = %v, want [skills=golangg]", ig)
	}
	subID := int64(subs[0].(map[string]any)["id"].(float64))
	savedID := got["saved_search_id"]

	// The stored query is the website's form, so the website sees the same set.
	var query string
	if err := pool.QueryRow(ctx, `SELECT query FROM saved_searches WHERE user_id = $1`, alice).Scan(&query); err != nil {
		t.Fatalf("read saved search: %v", err)
	}
	if query != "company_slug=stripe&skills=go" {
		t.Fatalf("stored query = %q", query)
	}

	// Subscribing again reuses the saved search and reports the existing subscription.
	again := a.ok("subscribe", args)
	if again["saved_search_id"] != savedID {
		t.Fatalf("second subscribe saved search = %v, want reuse of %v", again["saved_search_id"], savedID)
	}
	if st := again["subscriptions"].([]any)[0].(map[string]any)["status"]; st != "already_subscribed" {
		t.Fatalf("second subscribe status = %v, want already_subscribed", st)
	}

	list := a.ok("list_subscriptions", map[string]any{})["subscriptions"].([]any)
	if len(list) != 1 {
		t.Fatalf("list = %v, want one", list)
	}
	item := list[0].(map[string]any)
	if item["filter"] != "company_slug: stripe · skills: go" || item["channel"] != "email" {
		t.Fatalf("list item = %v", item)
	}

	// The ledger: one open posting matched, one that has since closed.
	open := seedJobSlug(t, pool, "mcp-subs-open-job")
	closed := seedJobSlug(t, pool, "mcp-subs-closed-job")
	if _, err := pool.Exec(ctx, `UPDATE jobs SET closed_at = now() WHERE id = $1`, closed); err != nil {
		t.Fatalf("close job: %v", err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO subscription_matches (subscription_id, job_id) VALUES ($1, $2), ($1, $3)`,
		subID, open, closed); err != nil {
		t.Fatalf("seed matches: %v", err)
	}
	updates := a.ok("subscription_updates", map[string]any{})["postings"].([]any)
	if len(updates) != 1 {
		t.Fatalf("updates = %v, want only the open posting", updates)
	}
	if u := updates[0].(map[string]any); u["url"] != testMCPOrigin+"/jobs/mcp-subs-open-job" || u["delivered"] != false {
		t.Fatalf("update = %v", u)
	}
	if pending := a.ok("list_subscriptions", map[string]any{})["subscriptions"].([]any)[0].(map[string]any)["pending"]; pending != float64(2) {
		t.Fatalf("pending = %v, want 2", pending)
	}

	// Bob can neither read nor remove Alice's subscription.
	if text, failed := b.call("unsubscribe", map[string]any{"id": subID}); !failed || !strings.Contains(text, "no subscription") {
		t.Fatalf("bob unsubscribe = %q failed=%v, want not found", text, failed)
	}
	if text, failed := b.call("subscription_updates", map[string]any{"subscription_id": subID}); !failed || !strings.Contains(text, "no subscription") {
		t.Fatalf("bob updates = %q failed=%v, want not found", text, failed)
	}
	if n := countSubscriptionRows(t, pool, `SELECT count(*) FROM subscriptions WHERE id = $1`, subID); n != 1 {
		t.Fatal("bob's unsubscribe removed alice's subscription")
	}

	a.ok("unsubscribe", map[string]any{"id": subID})
	if list := a.ok("list_subscriptions", map[string]any{})["subscriptions"].([]any); len(list) != 0 {
		t.Fatalf("list after unsubscribe = %v", list)
	}
}
