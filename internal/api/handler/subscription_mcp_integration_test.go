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

func TestSubscriptionMCP_SubscribeListUpdateUnsubscribe(t *testing.T) {
	pool, h := newSubscriptionMCPFixture(t)
	ctx := context.Background()
	alice := seedOAuthUser(t, pool, "mcp-subs-alice@example.test")
	bob := seedOAuthUser(t, pool, "mcp-subs-bob@example.test")
	a := newMCPToolCaller(t, h, alice)
	b := newMCPToolCaller(t, h, bob)

	args := map[string]any{
		"filters": map[string]any{
			"work_mode": []string{"remote"},
			"skills":    []string{"go", "golangg"},
		},
		"salary_min": 80000,
	}
	got := a.ok("subscribe", args)
	outcomes := got["channels_outcome"].([]any)
	if len(outcomes) != 1 || outcomes[0].(map[string]any)["channel"] != "email" || outcomes[0].(map[string]any)["status"] != "created" {
		t.Fatalf("outcomes = %v, want one created email delivery", outcomes)
	}
	if ig := got["ignored_params"].([]any); len(ig) != 1 || ig[0] != "skills=golangg" {
		t.Fatalf("ignored_params = %v, want [skills=golangg]", ig)
	}
	alert := got["alert"].(map[string]any)
	alertID := alert["id"]
	subID := int64(alert["channels"].([]any)[0].(map[string]any)["subscription_id"].(float64))

	// The stored query is the website's form, so the website sees the same set.
	var query string
	if err := pool.QueryRow(ctx, `SELECT query FROM saved_searches WHERE user_id = $1`, alice).Scan(&query); err != nil {
		t.Fatalf("read saved search: %v", err)
	}
	if query != "salary_min=80000&skills=go&work_mode=remote" {
		t.Fatalf("stored query = %q", query)
	}

	// Subscribing again reuses the saved search and reports the existing delivery.
	again := a.ok("subscribe", args)
	if again["alert"].(map[string]any)["id"] != alertID {
		t.Fatalf("second subscribe alert = %v, want reuse of %v", again["alert"], alertID)
	}
	if st := again["channels_outcome"].([]any)[0].(map[string]any)["status"]; st != "already_subscribed" {
		t.Fatalf("second subscribe status = %v, want already_subscribed", st)
	}

	list := a.ok("list_subscriptions", map[string]any{})
	alerts := list["alerts"].([]any)
	if len(alerts) != 1 {
		t.Fatalf("alerts = %v, want one", alerts)
	}
	item := alerts[0].(map[string]any)
	if item["filter"] != "salary_min: 80000 · skills: go · work_mode: remote" || item["paused"] != false {
		t.Fatalf("alert = %v", item)
	}
	if s := item["search"].(map[string]any); s["salary_min"] != float64(80000) || s["filters"].(map[string]any)["skills"].([]any)[0] != "go" {
		t.Fatalf("structured search = %v", s)
	}
	if sched := list["delivery_schedule"].(map[string]any); sched["digest_frequency"] != "instant" {
		t.Fatalf("delivery_schedule = %v", sched)
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
	updates := a.ok("subscription_updates", map[string]any{"id": alertID})["postings"].([]any)
	if len(updates) != 1 {
		t.Fatalf("updates = %v, want only the open posting", updates)
	}
	if u := updates[0].(map[string]any); u["url"] != testMCPOrigin+"/jobs/mcp-subs-open-job" || u["delivered"] != false {
		t.Fatalf("update = %v", u)
	}

	// Update: an unfiltered result is refused and changes nothing.
	if text, failed := a.call("update_subscription", map[string]any{
		"id": alertID, "filters": map[string]any{}, "salary_min": 0,
	}); !failed || !strings.Contains(text, "nothing was changed") {
		t.Fatalf("emptying update = %q failed=%v, want refusal", text, failed)
	}
	// A channel the account has not connected is refused before anything is written.
	if text, failed := a.call("update_subscription", map[string]any{
		"id": alertID, "name": "Renamed", "channels": []string{"email", "telegram"},
	}); !failed || !strings.Contains(text, "connected: email") {
		t.Fatalf("telegram update = %q failed=%v, want refusal", text, failed)
	}
	if n := countSubscriptionRows(t, pool, `SELECT count(*) FROM saved_searches WHERE user_id = $1 AND name = 'Renamed'`, alice); n != 0 {
		t.Fatal("a refused update renamed the alert")
	}

	// Once Telegram is linked: change a filter, rename, move to Telegram and pause, in one call.
	if _, err := pool.Exec(ctx, `INSERT INTO telegram_links (user_id, chat_id) VALUES ($1, 4242)`, alice); err != nil {
		t.Fatalf("link telegram: %v", err)
	}
	upd := a.ok("update_subscription", map[string]any{
		"id": alertID, "name": "Senior remote Go",
		"filters":  map[string]any{"work_mode": []string{"remote"}, "skills": []string{"go"}, "seniority": []string{"senior"}},
		"channels": []string{"telegram"}, "paused": true,
	})["alert"].(map[string]any)
	if upd["name"] != "Senior remote Go" || upd["filter"] != "salary_min: 80000 · seniority: senior · skills: go · work_mode: remote" {
		t.Fatalf("updated alert = %v", upd)
	}
	chans := upd["channels"].([]any)
	if len(chans) != 1 || chans[0].(map[string]any)["channel"] != "telegram" || chans[0].(map[string]any)["active"] != false || upd["paused"] != true {
		t.Fatalf("updated channels = %v paused=%v, want telegram only, paused", chans, upd["paused"])
	}
	// Resume.
	if res := a.ok("update_subscription", map[string]any{"id": alertID, "paused": false})["alert"].(map[string]any); res["paused"] != false {
		t.Fatalf("resumed alert = %v", res)
	}

	// Bob can neither read, change nor remove Alice's alert.
	for _, call := range []struct {
		tool string
		args map[string]any
	}{
		{"unsubscribe", map[string]any{"id": alertID}},
		{"update_subscription", map[string]any{"id": alertID, "paused": true}},
		{"subscription_updates", map[string]any{"id": alertID}},
	} {
		if text, failed := b.call(call.tool, call.args); !failed || !strings.Contains(text, "no job alert") {
			t.Fatalf("bob %s = %q failed=%v, want not found", call.tool, text, failed)
		}
	}
	if n := countSubscriptionRows(t, pool, `SELECT count(*) FROM subscriptions WHERE user_id = $1 AND active`, alice); n != 1 {
		t.Fatal("bob's calls changed alice's alert")
	}

	a.ok("unsubscribe", map[string]any{"id": alertID})
	if alerts := a.ok("list_subscriptions", map[string]any{})["alerts"].([]any); len(alerts) != 0 {
		t.Fatalf("alerts after unsubscribe = %v", alerts)
	}
}
