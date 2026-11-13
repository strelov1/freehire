package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/api/mcpapp"
)

const testMCPOrigin = "https://freehire.example"

// stampUser plays the authentication middleware: every request arrives as user 42.
type stampUser struct{ userID string }

func (s stampUser) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set(mcpapp.AccountUserIDHeader, s.userID)
	return http.DefaultTransport.RoundTrip(req)
}

// connectMCPTools serves tools to userID over the MCP server /mcp mounts.
func connectMCPTools(t *testing.T, tools []assistant.Tool, userID string) *mcp.ClientSession {
	t.Helper()
	server := httptest.NewServer(mcpapp.Handler(tools, testMCPOrigin, nil))
	t.Cleanup(server.Close)
	session, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil).Connect(t.Context(),
		&mcp.StreamableClientTransport{Endpoint: server.URL, HTTPClient: &http.Client{Transport: stampUser{userID}}}, nil)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}

func mcpAccountApp(assistants *assistantHandlers) *fiber.App {
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	mountMCP(app.Group("/api/v1"), assistants, &mcpappHandlers{origin: testMCPOrigin})
	return app
}

func TestMCPAccountMount_RejectsMissingBearer(t *testing.T) {
	app := mcpAccountApp(&assistantHandlers{})
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, "/api/v1/mcp", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

// A 401 must say where to sign in. The MCP authorization spec requires the
// resource_metadata pointer, and a client that does not fall back to guessing the
// well-known path has no other way to find our authorization server.
func TestMCPAccountMount_UnauthorizedNamesResourceMetadata(t *testing.T) {
	const wantMetadata = `resource_metadata="` + testMCPOrigin + mcpapp.ResourceMetadataPath + `"`
	cases := map[string]struct {
		authorization string
		wantError     bool
	}{
		// The first request of a connector being added: it must be told to sign in, never
		// served anonymously, or Claude.ai and Grok add the connector without sign-in.
		"no bearer at all": {authorization: ""},
		"not a bearer":     {authorization: "Basic Zm9vOmJhcg=="},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			app := mcpAccountApp(&assistantHandlers{})
			req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, "/api/v1/mcp", nil)
			if tc.authorization != "" {
				req.Header.Set(fiber.HeaderAuthorization, tc.authorization)
			}
			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("app.Test: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != fiber.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", resp.StatusCode)
			}
			got := resp.Header.Get(fiber.HeaderWWWAuthenticate)
			if !strings.HasPrefix(got, "Bearer ") || !strings.Contains(got, wantMetadata) {
				t.Fatalf("WWW-Authenticate = %q, want a Bearer challenge carrying %s", got, wantMetadata)
			}
			if strings.Contains(got, "invalid_token") {
				t.Fatalf("WWW-Authenticate = %q: a request with no token must not be told its token is invalid", got)
			}
		})
	}
}

// The signed-in MCP server announces each tool's effect from the tool's own Effect, and a
// client skips the confirmation prompt on a read. Walks the set mountMCP actually
// serves, so a tool added to it must be classified here to pass — which is the point: the
// decision is one a person makes once, looking at what the tool does.
func TestMCPAccountTools_DeclareTheirEffect(t *testing.T) {
	want := map[string]assistant.Effect{
		"get_profile": assistant.EffectReads,
		"get_plan":    assistant.EffectReads,
		"facets":      assistant.EffectReads,
		"search_jobs": assistant.EffectReads,
		"get_job":     assistant.EffectReads,
		"get_company": assistant.EffectReads,
		"market_fit":  assistant.EffectReads,
		"my_jobs":     assistant.EffectReads,
		"save_job":    assistant.EffectAdds,
		"apply_job":   assistant.EffectAdds,
		"unsave_job":  assistant.EffectOverwrites,
		"track_job":   assistant.EffectOverwrites,
		// Files a moderation report; changes nothing the person already had.
		"report_job":              assistant.EffectAdds,
		"suggest_dictionary_term": assistant.EffectAdds,
		"report_problem":          assistant.EffectAdds,
		// Replaces whichever preference lists the call sends.
		"update_profile": assistant.EffectOverwrites,
		// Stores a first CV and refuses to replace one, so nothing held is lost.
		"add_cv":           assistant.EffectAdds,
		"get_setup_status": assistant.EffectReads,

		"auto_apply_list":       assistant.EffectReads,
		"auto_apply_get":        assistant.EffectReads,
		"answer_bank_list":      assistant.EffectReads,
		"screening_answers_get": assistant.EffectReads,
		"auto_apply_start":      assistant.EffectAdds,
		// Approving sends a real application in the person's name; a client should ask.
		"auto_apply_review":     assistant.EffectOverwrites,
		"auto_apply_retry":      assistant.EffectOverwrites,
		"auto_apply_cancel":     assistant.EffectOverwrites,
		"answer_bank_save":      assistant.EffectOverwrites,
		"answer_bank_delete":    assistant.EffectOverwrites,
		"screening_answers_set": assistant.EffectOverwrites,

		// Lists headers without touching read_at.
		"list_unread_mail": assistant.EffectReads,

		"list_subscriptions":   assistant.EffectReads,
		"subscription_updates": assistant.EffectReads,
		// Adds a saved search (or reuses one) and a subscription; removes nothing.
		"subscribe": assistant.EffectAdds,
		// Deletes the subscription and its match record.
		"unsubscribe": assistant.EffectOverwrites,
		// Replaces the alert's search, name, channels or paused state.
		"update_subscription": assistant.EffectOverwrites,

		"buddy_status":            assistant.EffectReads,
		"buddy_mock_hosts":        assistant.EffectReads,
		"buddy_mock_slots":        assistant.EffectReads,
		"buddy_mock_list":         assistant.EffectReads,
		"buddy_mock_schedule_get": assistant.EffectReads,
		"buddy_join":              assistant.EffectAdds,
		"buddy_mock_book":         assistant.EffectAdds,
		// Adds one dated row; the weekly rules and other overrides stay.
		"buddy_mock_schedule_add_override": assistant.EffectAdds,
		// Closes conversations and cancels every upcoming mock.
		"buddy_leave":                    assistant.EffectOverwrites,
		"buddy_mock_cancel":              assistant.EffectOverwrites,
		"buddy_mock_settings_set":        assistant.EffectOverwrites,
		"buddy_mock_schedule_set_weekly": assistant.EffectOverwrites,
		"buddy_mock_schedule_delete":     assistant.EffectOverwrites,
	}
	for _, tool := range mcpAccountTools(&assistantHandlers{}, testMCPOrigin) {
		effect, known := want[tool.Name]
		if !known {
			t.Errorf("%s: served on the MCP account server but not classified here", tool.Name)
			continue
		}
		if tool.Effect != effect {
			t.Errorf("%s: Effect = %d, want %d", tool.Name, tool.Effect, effect)
		}
		// A client lists an untitled tool by its snake_case name, which is what a
		// directory reviewer sees first.
		if tool.Title == "" {
			t.Errorf("%s: served on the MCP account server without a Title", tool.Name)
		}
	}
}

// get_plan is read-only and GET /me/plan is open to a key over REST, so a full-scope key
// reads it too.
func TestMCPKeyTools_ServeGetPlan(t *testing.T) {
	for _, tool := range mcpKeyTools(&assistantHandlers{}, testMCPOrigin) {
		if tool.Name == "get_plan" {
			return
		}
	}
	t.Fatal("get_plan is not served to a full-scope API key")
}

// present_jobs renders cards only inside freehire's own chat and forbids the model from
// writing job links; served to Claude or ChatGPT it would leave the person seeing no
// vacancies at all.
func TestMCPAccountTools_LeavesOutPresentJobs(t *testing.T) {
	tools := mcpAccountTools(&assistantHandlers{}, testMCPOrigin)
	for _, tool := range tools {
		if tool.Name == presentJobsToolName {
			t.Fatalf("%s is served on the MCP account server, which has no card renderer", presentJobsToolName)
		}
	}
	if len(tools) == 0 {
		t.Fatal("mcpAccountTools served nothing")
	}
}

// Anthropic's directory policy rules out a tool description that instructs the model about
// another tool, and the account server is the one submitted to the directory. The job-alert
// tools keep that rule: their order of use lives in the server instructions. Scoped to the
// alert tools, whose names it reads from the set they register, so the check does not
// depend on the anonymous server's tool list.
func TestMCPAccountAlertTools_NameNoOtherTool(t *testing.T) {
	tools := (&assistantHandlers{}).subscriptionMCPTools(testMCPOrigin)
	names := make([]string, 0, len(tools)+1)
	for _, tool := range tools {
		names = append(names, tool.Name)
	}
	names = append(names, "search_jobs")
	for _, tool := range tools {
		raw, err := json.Marshal(struct {
			Description string
			Schema      any
		}{tool.Description, tool.Schema})
		if err != nil {
			t.Fatalf("%s: %v", tool.Name, err)
		}
		for _, name := range names {
			if name != tool.Name && strings.Contains(string(raw), name) {
				t.Errorf("%s: description or schema names %s", tool.Name, name)
			}
		}
	}
}

// A full-scope API key reaches the account server so the local MCP package can manage job
// alerts; auto-apply, which sends applications in the person's name and has never been open
// to a key over REST, stays with the OAuth grant.
func TestMCPKeyTools_ServeAlertsButNotAutoApply(t *testing.T) {
	h := &assistantHandlers{}
	served := map[string]bool{}
	for _, tool := range mcpKeyTools(h, testMCPOrigin) {
		served[tool.Name] = true
	}
	for _, tool := range h.subscriptionMCPTools(testMCPOrigin) {
		if !served[tool.Name] {
			t.Errorf("%s: a job-alert tool missing from the key's set", tool.Name)
		}
	}
	for _, tool := range h.autoApplyMCPTools() {
		if served[tool.Name] {
			t.Errorf("%s: an auto-apply tool served to an API key", tool.Name)
		}
	}
	if served[presentJobsToolName] {
		t.Errorf("%s served to an API key", presentJobsToolName)
	}
}

// Every buddy route is cookie-only, so a leaked API key cannot book or read a member's mocks;
// the account server keeps that line and serves the buddy tools to OAuth grants only.
func TestMCPKeyTools_ServeNoBuddyTool(t *testing.T) {
	served := map[string]bool{}
	for _, tool := range mcpKeyTools(&assistantHandlers{}, testMCPOrigin) {
		served[tool.Name] = true
	}
	granted := map[string]bool{}
	for _, tool := range mcpAccountTools(&assistantHandlers{}, testMCPOrigin) {
		granted[tool.Name] = true
	}
	buddyTools := (*buddyHandlers)(nil).mcpTools()
	if len(buddyTools) != 13 {
		t.Fatalf("buddy tools = %d, want 13", len(buddyTools))
	}
	for _, tool := range buddyTools {
		if served[tool.Name] {
			t.Errorf("%s: a buddy tool served to an API key", tool.Name)
		}
		if !granted[tool.Name] {
			t.Errorf("%s: a buddy tool missing from the OAuth grant's set", tool.Name)
		}
	}
}
