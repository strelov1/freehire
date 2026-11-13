//go:build integration

package handler

import (
	"bytes"
	"encoding/json"
	"log"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/observability"
)

func TestMCPAuthDiagnosticsResolveKeyWithoutLoggingToken(t *testing.T) {
	pool := startPostgres(t)
	queries := db.New(pool)
	userID := seedOAuthUser(t, pool, "mcp-diagnostics@example.test")
	if err := queries.SetUserEmailVerified(t.Context(), userID); err != nil {
		t.Fatal(err)
	}
	keyIDs := map[string]int64{}
	for _, scope := range []string{auth.ScopeFull, auth.ScopeEmbed} {
		key, err := queries.CreateAPIKey(t.Context(), db.CreateAPIKeyParams{UserID: userID, Name: "diagnostic", TokenHash: auth.HashAPIKey("fhk_private-" + scope), TokenPrefix: "fhk_private", Scope: scope})
		if err != nil {
			t.Fatal(err)
		}
		keyIDs[scope] = key.ID
	}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Use(observability.RequestIDs())
	app.Post("/probe", mcpAuthenticate(&assistantHandlers{queries: queries}, testMCPOrigin), func(c *fiber.Ctx) error {
		observability.RequestLogger(c.Context()).Info("authenticated")
		return c.SendStatus(200)
	})
	for _, tc := range []struct {
		scope, token, reason string
		status               int
	}{
		{scope: "full", token: "fhk_private-full", status: 200},
		{scope: "embed", token: "fhk_private-embed", reason: "insufficient_scope", status: 403},
		{token: "fhk_private-unknown", reason: "invalid_or_expired_credential", status: 401},
		{reason: "missing_bearer_token", status: 401},
	} {
		t.Run(tc.scope+tc.reason, func(t *testing.T) {
			var logs bytes.Buffer
			previous := slog.Default()
			output, flags := log.Writer(), log.Flags()
			slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
			t.Cleanup(func() {
				slog.SetDefault(previous)
				log.SetOutput(output)
				log.SetFlags(flags)
			})
			req := httptest.NewRequestWithContext(t.Context(), "POST", "/probe", nil)
			if tc.token != "" {
				req.Header.Set("Authorization", "Bearer "+tc.token)
			}
			resp, err := app.Test(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tc.status {
				t.Fatalf("status = %d", resp.StatusCode)
			}
			var record map[string]any
			if err := json.Unmarshal(bytes.TrimSpace(logs.Bytes()), &record); err != nil {
				t.Fatalf("missing diagnostic: %s: %v", &logs, err)
			}
			if record["request_id"] != resp.Header.Get("X-Request-ID") || record["request_id"] == "" {
				t.Errorf("lost correlation: %v", record)
			}
			if tc.reason != "" && record["reason"] != tc.reason {
				t.Errorf("incorrect cause: %v", record)
			}
			if tc.scope != "" && (record["key_id"] != float64(keyIDs[tc.scope]) || record["scope"] != tc.scope || record["credential_type"] != "api_key" || record["user_id"] != float64(userID)) {
				t.Errorf("wrong resolved identity: %v", record)
			}
			if strings.Contains(logs.String(), "fhk_private") || strings.Contains(logs.String(), auth.HashAPIKey(tc.token)) {
				t.Fatalf("credential leaked: %s", &logs)
			}
		})
	}
}
