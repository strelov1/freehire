//go:build integration

// Integration test for get_plan over the signed-in MCP server
// (openspec/changes/mcp-plan-standing): what an agent reads agrees with what GET /me/plan
// answers on the wire, for a paid account with usage and for a free one.
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/identity/auth"
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
	rest := planEndpointReader(t, plans, queries, userID)

	got, want := decodeMCPPlan(t, mcpc), rest()
	if got.Plan != "pro" || got.Plan != want.Plan || got.PaidUntil == nil || want.ProUntil == nil ||
		!got.PaidUntil.Equal(*want.ProUntil) || !got.ResetsAt.Equal(want.ResetsAt) {
		t.Fatalf("get_plan = %+v, want GET /me/plan's %+v", got, want)
	}
	if len(got.Allowances) != len(want.Allowances) {
		t.Fatalf("allowances = %+v, want %+v", got.Allowances, want.Allowances)
	}
	for i, a := range got.Allowances {
		w := want.Allowances[i]
		if a.Feature != w.Feature || a.Used != w.Used || a.Unlimited != w.Unlimited || a.Enforced != w.Enforced ||
			!sameInt(a.Limit, w.Limit) {
			t.Errorf("allowance %d = %+v, want %+v", i, a, w)
		}
		if a.Feature == string(plan.FeatureAutoApply) &&
			(a.Used != 2 || a.Limit == nil || a.Remaining == nil || *a.Remaining != *a.Limit-2) {
			t.Errorf("auto-apply = %+v, want 2 used of its limit with the rest remaining", a)
		}
	}

	// A free account: nothing remains on any enforced limit, and the agent has the page to
	// send them to.
	endPro(t, pool, userID)
	free := decodeMCPPlan(t, mcpc)
	if free.Plan != "free" || free.Plan != rest().Plan || free.PaidUntil != nil || free.PlanURL != testMCPOrigin+"/my/plan" {
		t.Fatalf("free get_plan = %+v", free)
	}
	for _, a := range free.Allowances {
		if a.Enforced && (a.Remaining == nil || *a.Remaining != 0) {
			t.Errorf("free allowance %+v, want nothing remaining", a)
		}
	}
}

// planWire is the part of GET /me/plan's body get_plan restates.
type planWire struct {
	Plan       string     `json:"plan"`
	ResetsAt   time.Time  `json:"resets_at"`
	ProUntil   *time.Time `json:"pro_until"`
	Allowances []struct {
		Feature   string `json:"feature"`
		Used      int    `json:"used"`
		Limit     *int   `json:"limit"`
		Unlimited bool   `json:"unlimited"`
		Enforced  bool   `json:"enforced"`
	} `json:"allowances"`
}

// planEndpointReader serves GET /me/plan through its real route and contract and returns a
// reader of the caller's answer.
func planEndpointReader(t *testing.T, plans *plan.Store, queries *db.Queries, userID int64) func() planWire {
	t.Helper()
	iss := auth.NewIssuer("test-secret", time.Hour)
	planH := newPlanHandlers(plans, queries)
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	planH.register(app.Group("/api/v1"), middleware{key: auth.RequireAuth(iss, testVersions)},
		newContract(contractHandlers{planHandlers: planH}))
	token, err := iss.Issue(userID, testTokenVersion)
	if err != nil {
		t.Fatal(err)
	}
	return func() planWire {
		t.Helper()
		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/me/plan", nil)
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		resp, err := app.Test(req, 15000)
		if err != nil {
			t.Fatalf("GET /me/plan: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET /me/plan = %d", resp.StatusCode)
		}
		var out struct {
			Data planWire `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			t.Fatalf("decode GET /me/plan: %v", err)
		}
		return out.Data
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
