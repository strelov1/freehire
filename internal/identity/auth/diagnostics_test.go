package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/strelov1/freehire/internal/platform/observability"
)

func TestAuthFailureDiagnosticsExcludeCredentials(t *testing.T) {
	for _, tc := range []struct {
		name, token, reason string
		keys                APIKeyAuthenticator
		status              int
	}{
		{name: "scope", token: "fhk_private-token", reason: "insufficient_scope", keys: fakeScopedKeys{validHash: HashAPIKey("fhk_private-token"), userID: 4, scope: ScopeEmbed, keyID: 73}, status: 403},
		{name: "unknown", token: "private-unknown-token", reason: "invalid_or_expired_credential", keys: fakeScopedKeys{}, status: 401},
		{name: "absent", reason: "missing_credential", keys: fakeScopedKeys{}, status: 401},
		{name: "database", token: "private-token", reason: "credential_lookup_failed", keys: errKeyAuth{err: errors.New("private-database-password")}, status: 503},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var logs bytes.Buffer
			previous := slog.Default()
			output, flags := log.Writer(), log.Flags()
			slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
			t.Cleanup(func() {
				slog.SetDefault(previous)
				log.SetOutput(output)
				log.SetFlags(flags)
			})
			app := fiber.New()
			app.Use(observability.RequestIDs())
			app.Get("/probe", RequireAuthOrKey(NewIssuer("secret", time.Hour), anyVersion{1}, tc.keys), func(c *fiber.Ctx) error { return c.SendStatus(200) })
			req := httptest.NewRequestWithContext(context.Background(), "GET", "/probe", nil)
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
				t.Fatalf("missing diagnostic: %s (%v)", &logs, err)
			}
			if record["reason"] != tc.reason || record["request_id"] != resp.Header.Get("X-Request-ID") || record["request_id"] == "" {
				t.Errorf("incorrect diagnostic: %v", record)
			}
			if tc.name == "scope" && (record["user_id"] != float64(4) || record["key_id"] != float64(73) || record["scope"] != "embed" || record["credential_type"] != "api_key") {
				t.Errorf("missing identity: %v", record)
			}
			for _, secret := range []string{tc.token, "private-database-password", HashAPIKey(tc.token)} {
				if secret != "" && strings.Contains(logs.String(), secret) {
					t.Fatalf("secret leaked: %s", &logs)
				}
			}
		})
	}
}
