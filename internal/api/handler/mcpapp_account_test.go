package handler

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ai/assistant"
)

const testMCPOrigin = "https://freehire.example"

func mcpAccountApp(assistants *assistantHandlers) *fiber.App {
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	// A nil throttler fails open (ratelimit.Middleware's documented behavior),
	// so the zero-value middleware needs no fake here.
	mw := middleware{}
	api := app.Group("/api/v1")
	mountMCPAccount(api, mw, assistants, testMCPOrigin)
	return app
}

func TestMCPAccountMount_RejectsMissingBearer(t *testing.T) {
	app := mcpAccountApp(&assistantHandlers{})
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, "/api/v1/mcp/account", nil)
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
	const wantMetadata = `resource_metadata="` + testMCPOrigin + `/.well-known/oauth-protected-resource"`
	cases := map[string]struct {
		authorization string
		wantError     bool
	}{
		"no bearer at all": {authorization: ""},
		"not a bearer":     {authorization: "Basic Zm9vOmJhcg=="},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			app := mcpAccountApp(&assistantHandlers{})
			req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, "/api/v1/mcp/account", nil)
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
// client skips the confirmation prompt on a read. Walks the set mountMCPAccount actually
// serves, so a tool added to it must be classified here to pass — which is the point: the
// decision is one a person makes once, looking at what the tool does.
func TestMCPAccountTools_DeclareTheirEffect(t *testing.T) {
	want := map[string]assistant.Effect{
		"get_profile": assistant.EffectReads,
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
	}
	for _, tool := range mcpAccountTools(&assistantHandlers{}) {
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

// present_jobs renders cards only inside freehire's own chat and forbids the model from
// writing job links; served to Claude or ChatGPT it would leave the person seeing no
// vacancies at all.
func TestMCPAccountTools_LeavesOutPresentJobs(t *testing.T) {
	tools := mcpAccountTools(&assistantHandlers{})
	for _, tool := range tools {
		if tool.Name == presentJobsToolName {
			t.Fatalf("%s is served on the MCP account server, which has no card renderer", presentJobsToolName)
		}
	}
	if len(tools) == 0 {
		t.Fatal("mcpAccountTools served nothing")
	}
}
