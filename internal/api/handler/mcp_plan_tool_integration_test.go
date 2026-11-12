//go:build integration

// Integration test for get_plan over the signed-in MCP server
// (openspec/changes/mcp-plan-standing): what an agent reads agrees with GET /me/plan's use
// case, for a paid account with usage and for a free one.
package handler

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/platform/db"
)

func TestMCPGetPlan_AgreesWithThePlanEndpoint(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	queries := db.New(pool)
	plans := plan.NewStore(queries, pool, plan.DefaultConfig())
	h := &assistantHandlers{plans: plans, queries: queries}

	var userID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('mcp-plan@example.test') RETURNING id`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	grantPro(t, pool, userID, time.Now().Add(72*time.Hour))
	if _, err := pool.Exec(ctx,
		`INSERT INTO usage_daily (user_id, feature, day, used) VALUES ($1, 'auto-apply', CURRENT_DATE, 2)`, userID); err != nil {
		t.Fatal(err)
	}
	mcpc := newMCPToolCaller(t, h, userID)

	got := decodeMCPPlan(t, mcpc)
	want, err := loadPlan(ctx, plans, queries, userID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Plan != "pro" || got.Plan != want.Plan || got.PaidUntil == nil || !got.PaidUntil.Equal(*want.ProUntil) ||
		!got.ResetsAt.Equal(want.ResetsAt) {
		t.Fatalf("get_plan = %+v, want the plan endpoint's %+v", got, want)
	}
	if len(got.Allowances) != len(want.Allowances) {
		t.Fatalf("allowances = %+v, want %+v", got.Allowances, want.Allowances)
	}
	for i, a := range got.Allowances {
		w := want.Allowances[i]
		if a.Feature != w.Feature || a.Used != w.Used || a.Unlimited != w.Unlimited || a.Enforced != w.Enforced {
			t.Errorf("allowance %d = %+v, want %+v", i, a, w)
		}
		if a.Feature == string(plan.FeatureAutoApply) &&
			(a.Used != 2 || a.Limit == nil || a.Remaining == nil || *a.Remaining != *a.Limit-2) {
			t.Errorf("auto-apply = %+v, want 2 used of its limit with the rest remaining", a)
		}
	}

	// A free account: nothing remains anywhere, and the agent has the page to send them to.
	endPro(t, pool, userID)
	free := decodeMCPPlan(t, mcpc)
	if free.Plan != "free" || free.PaidUntil != nil || free.PlanURL != testMCPOrigin+"/my/plan" {
		t.Fatalf("free get_plan = %+v", free)
	}
	for _, a := range free.Allowances {
		if a.Unlimited || a.Remaining == nil || *a.Remaining != 0 {
			t.Errorf("free allowance %+v, want nothing remaining", a)
		}
	}
}

func decodeMCPPlan(t *testing.T, mcpc *mcpToolCaller) mcpPlan {
	t.Helper()
	text, failed := mcpc.call("get_plan", map[string]any{})
	if failed {
		t.Fatalf("get_plan failed: %s", text)
	}
	if strings.Contains(text, "stripe") || strings.Contains(text, "granted") || strings.Contains(text, "price") {
		t.Errorf("get_plan carries payment details: %s", text)
	}
	var out mcpPlan
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("decode %s: %v", text, err)
	}
	return out
}
