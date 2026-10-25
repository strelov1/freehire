//go:build integration

// Integration tests for auto-apply over the signed-in MCP server and its HTTP parity routes
// (openspec/changes/auto-apply-over-mcp): the account switch, the full start → review →
// block → answer → retry → cancel flow through real tool calls, and the uncertain-submission
// refusal.
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
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/api/mcpapp"
	"github.com/strelov1/freehire/internal/candidate/answerbank"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/ingest/screeninganswers"
	"github.com/strelov1/freehire/internal/platform/db"
)

// mcpToolCaller runs the account server's tools for one user over in-memory transports.
type mcpToolCaller struct {
	t       *testing.T
	session *mcp.ClientSession
}

func newMCPToolCaller(t *testing.T, h *assistantHandlers, userID int64) *mcpToolCaller {
	t.Helper()
	server := mcpapp.NewAccountServer(mcpAccountTools(h), userID, testMCPOrigin)
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	go func() { _ = server.Run(context.Background(), serverTransport) }()
	session, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil).
		Connect(context.Background(), clientTransport, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return &mcpToolCaller{t: t, session: session}
}

// call runs a tool and returns its text and whether it failed.
func (c *mcpToolCaller) call(name string, args map[string]any) (string, bool) {
	c.t.Helper()
	res, err := c.session.CallTool(context.Background(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		c.t.Fatalf("%s: %v", name, err)
	}
	return res.Content[0].(*mcp.TextContent).Text, res.IsError
}

// ok runs a tool that must succeed and decodes its result.
func (c *mcpToolCaller) ok(name string, args map[string]any) map[string]any {
	c.t.Helper()
	text, failed := c.call(name, args)
	if failed {
		c.t.Fatalf("%s failed: %s", name, text)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		c.t.Fatalf("%s: decode %s: %v", name, text, err)
	}
	return out
}

func historyEvents(view map[string]any) []string {
	var out []string
	for _, e := range view["history"].([]any) {
		out = append(out, e.(map[string]any)["event"].(string))
	}
	return out
}

func newAutoApplyMCPFixture(t *testing.T) (*pgxpool.Pool, *assistantHandlers, *db.Queries, int64, int64) {
	t.Helper()
	pool := startPostgres(t)
	truncateAutoApplyEnqueueTables(t, pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	_, h, _ := newAutoApplyTailorAppFull(pool, iss, &turnModel{}, plan.DefaultConfig(), "", nil)
	queries := db.New(pool)
	h.answerBank = answerbank.NewStore(answerbank.NewQueriesRepository(queries))
	h.screeningAnswers = screeninganswers.New(screeninganswers.NewQueriesRepository(queries, pool))

	userID, _ := autoApplyTailorUser(t, pool, iss, "mcp-auto-apply@example.test")
	insertBaseCV(t, pool, userID)
	jobID := insertAutoApplyJob(t, pool, "mcp-gh-job")
	return pool, h, queries, userID, jobID
}

func TestAutoApplyOverMCP_FullFlow(t *testing.T) {
	pool, h, queries, userID, jobID := newAutoApplyMCPFixture(t)
	ctx := context.Background()
	mcpc := newMCPToolCaller(t, h, userID)
	slug := map[string]any{"job_slug": "mcp-gh-job"}

	// The switch is off by default: acting is refused, reading is not.
	if text, failed := mcpc.call("auto_apply_start", slug); !failed || !strings.Contains(text, "turned off") {
		t.Fatalf("start with the switch off = %q failed=%v, want the switch refusal", text, failed)
	}
	mcpc.ok("auto_apply_list", map[string]any{})

	if err := queries.SetAgentsMayApply(ctx, db.SetAgentsMayApplyParams{ID: userID, AgentsMayApply: true}); err != nil {
		t.Fatal(err)
	}
	if got := mcpc.ok("auto_apply_start", slug)["status"]; got != "tailoring" {
		t.Fatalf("status after start = %v, want tailoring", got)
	}

	// The worker tailors and previews; one question has no answer yet.
	entry, err := queries.GetAutoApplyQueueEntryForJob(ctx, db.GetAutoApplyQueueEntryForJobParams{UserID: userID, JobID: jobID})
	if err != nil {
		t.Fatal(err)
	}
	var cv uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO cvs (user_id, title, data) VALUES ($1, 'Tailored', '{}') RETURNING id`, userID).Scan(&cv); err != nil {
		t.Fatal(err)
	}
	if _, err := queries.SetAutoApplyTailoredCV(ctx, db.SetAutoApplyTailoredCVParams{ID: entry.ID, TailoredCvID: &cv}); err != nil {
		t.Fatal(err)
	}
	preview := []byte(`{"fields":[{"label":"Email","value":"a@b.c"}],"pending":[{"label":"Why us?","will_draft_at_submission":false}]}`)
	if _, err := queries.SetAutoApplyResolvedPreview(ctx, db.SetAutoApplyResolvedPreviewParams{ID: entry.ID, ResolvedPreview: preview}); err != nil {
		t.Fatal(err)
	}
	view := mcpc.ok("auto_apply_get", slug)
	if view["status"] != "pending_review" || !strings.Contains(jsonText(t, view["pending_answers"]), "Why us?") {
		t.Fatalf("view = %v, want pending_review listing the pending question", view)
	}

	// The agent relays the person's answer; it is theirs, so it is sendable.
	mcpc.ok("answer_bank_save", map[string]any{"question": "Why us?", "answer": "I like the product."})
	sendable, err := h.answerBank.Sendable(ctx, userID)
	if err != nil || len(sendable) != 1 {
		t.Fatalf("sendable = %v err=%v, want the saved answer", sendable, err)
	}

	// Refresh drops the stale preview for the preview pass to recompute.
	if got := mcpc.ok("auto_apply_retry", slug)["status"]; got != "tailoring" {
		t.Fatalf("status after refresh = %v, want tailoring (preview dropped)", got)
	}
	if _, err := queries.SetAutoApplyResolvedPreview(ctx, db.SetAutoApplyResolvedPreviewParams{ID: entry.ID, ResolvedPreview: []byte(`{"fields":[]}`)}); err != nil {
		t.Fatal(err)
	}
	if got := mcpc.ok("auto_apply_review", map[string]any{"job_slug": "mcp-gh-job", "decision": "approved"})["status"]; got != "approved" {
		t.Fatalf("status after approve = %v, want approved", got)
	}

	// The submission parks on a question; answering and retrying re-arms it.
	unmapped := []byte(`[{"id":"q2","label":"Start date?","required":true,"reason":"no known answer"}]`)
	if err := queries.MarkAutoApplyBlocked(ctx, db.MarkAutoApplyBlockedParams{ID: entry.ID, LastError: "internal", Unmapped: unmapped}); err != nil {
		t.Fatal(err)
	}
	view = mcpc.ok("auto_apply_get", slug)
	if view["status"] != "blocked" || !strings.Contains(jsonText(t, view["open_questions"]), "Start date?") {
		t.Fatalf("view = %v, want blocked with the open question", view)
	}
	if strings.Contains(jsonText(t, view), "internal") {
		t.Errorf("the view leaks last_error: %v", view)
	}
	if got := mcpc.ok("auto_apply_retry", slug)["status"]; got != "approved" {
		t.Fatalf("status after retry = %v, want approved", got)
	}

	list := mcpc.ok("auto_apply_list", map[string]any{})
	if !strings.Contains(jsonText(t, list), "mcp-gh-job") {
		t.Fatalf("list = %v, want the attempt", list)
	}

	mcpc.ok("auto_apply_cancel", slug)
	view = mcpc.ok("auto_apply_get", slug)
	want := []string{"queued", "tailored", "ready_for_review", "retried", "ready_for_review", "approved", "blocked", "retried", "cancelled"}
	if view["status"] != "cancelled" || strings.Join(historyEvents(view), ",") != strings.Join(want, ",") {
		t.Fatalf("after cancel status=%v history=%v, want cancelled with %v", view["status"], historyEvents(view), want)
	}
}

func TestAutoApplyOverMCP_UncertainSubmissionIsNeverRetried(t *testing.T) {
	_, h, queries, userID, jobID := newAutoApplyMCPFixture(t)
	ctx := context.Background()
	if err := queries.SetAgentsMayApply(ctx, db.SetAgentsMayApplyParams{ID: userID, AgentsMayApply: true}); err != nil {
		t.Fatal(err)
	}
	mcpc := newMCPToolCaller(t, h, userID)
	slug := map[string]any{"job_slug": "mcp-gh-job"}
	mcpc.ok("auto_apply_start", slug)
	entry, err := queries.GetAutoApplyQueueEntryForJob(ctx, db.GetAutoApplyQueueEntryForJobParams{UserID: userID, JobID: jobID})
	if err != nil {
		t.Fatal(err)
	}
	if err := queries.MarkAutoApplySubmitUncertain(ctx, db.MarkAutoApplySubmitUncertainParams{ID: entry.ID, LastError: "unconfirmed"}); err != nil {
		t.Fatal(err)
	}

	view := mcpc.ok("auto_apply_get", slug)
	if view["status"] != "failed" || view["may_have_reached_employer"] != true {
		t.Fatalf("view = %v, want failed and may_have_reached_employer", view)
	}
	for _, tool := range []string{"auto_apply_retry", "auto_apply_cancel"} {
		if text, failed := mcpc.call(tool, slug); !failed || !strings.Contains(text, "may already have reached the employer") {
			t.Fatalf("%s = %q failed=%v, want the uncertain refusal", tool, text, failed)
		}
	}
}

func TestAgentPermissions_OnlyTheBrowserMayTurnItOn(t *testing.T) {
	pool := startPostgres(t)
	truncateAutoApplyEnqueueTables(t, pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	_, _, mw := newAutoApplyTailorAppFull(pool, iss, &turnModel{}, plan.DefaultConfig(), "", nil)
	queries := db.New(pool)
	userID, cookie := autoApplyTailorUser(t, pool, iss, "perm@example.test")

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	(&agentPermissionHandlers{queries: queries}).register(app.Group("/api/v1"), mw)

	put := func(withCookie bool, bearer string) int {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodPut, "/api/v1/me/agent-permissions",
			strings.NewReader(`{"agents_may_apply":true}`))
		req.Header.Set("Content-Type", "application/json")
		if withCookie {
			req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
		}
		if bearer != "" {
			req.Header.Set("Authorization", "Bearer "+bearer)
		}
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}

	if got := put(false, "fhk_whatever"); got != fiber.StatusUnauthorized {
		t.Fatalf("PUT with an API key = %d, want 401", got)
	}
	if got := put(true, ""); got != fiber.StatusOK {
		t.Fatalf("PUT with the cookie = %d, want 200", got)
	}
	if allowed, err := queries.GetAgentsMayApply(context.Background(), userID); err != nil || !allowed {
		t.Fatalf("agents_may_apply = %v err=%v, want true", allowed, err)
	}
}

func TestJobAutoApplyRetryAndCancelRoutes(t *testing.T) {
	pool := startPostgres(t)
	truncateAutoApplyEnqueueTables(t, pool)
	iss := auth.NewIssuer("test-secret", time.Hour)
	_, h, mw := newAutoApplyTailorAppFull(pool, iss, &turnModel{}, plan.DefaultConfig(), "", nil)
	queries := db.New(pool)
	userID, cookie := autoApplyTailorUser(t, pool, iss, "routes@example.test")
	jobID := insertAutoApplyJob(t, pool, "routes-job")
	queueID := insertAutoApplyQueueRow(t, pool, userID, jobID)

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	api := app.Group("/api/v1")
	api.Post("/jobs/:slug/auto-apply/retry", mw.cookie, h.PostJobAutoApplyRetry)
	api.Delete("/jobs/:slug/auto-apply", mw.cookie, h.DeleteJobAutoApply)
	do := func(method, path string) int {
		req := httptest.NewRequestWithContext(context.Background(), method, path, nil)
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}

	if got := do(http.MethodPost, "/api/v1/jobs/routes-job/auto-apply/retry"); got != fiber.StatusConflict {
		t.Fatalf("retry of a healthy attempt = %d, want 409", got)
	}
	if err := queries.MarkAutoApplyBlocked(context.Background(), db.MarkAutoApplyBlockedParams{ID: queueID, LastError: "p", Unmapped: []byte("[]")}); err != nil {
		t.Fatal(err)
	}
	if got := do(http.MethodPost, "/api/v1/jobs/routes-job/auto-apply/retry"); got != fiber.StatusOK {
		t.Fatalf("retry of a blocked attempt = %d, want 200", got)
	}
	if got := do(http.MethodDelete, "/api/v1/jobs/routes-job/auto-apply"); got != fiber.StatusOK {
		t.Fatalf("cancel = %d, want 200", got)
	}
	if got := do(http.MethodDelete, "/api/v1/jobs/routes-job/auto-apply"); got != fiber.StatusNotFound {
		t.Fatalf("cancel with no attempt = %d, want 404", got)
	}
}

func jsonText(t *testing.T, v any) string { return string(mustJSON(t, v)) }
