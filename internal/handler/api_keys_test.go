package handler

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/auth"
)

// apiKeysApp mounts the key-management routes behind RequireAuth (cookie-only) on a
// handler with no DB. The cookie-gate cases below reject before any query runs, so
// the nil queries is never dereferenced. The DB-backed paths (persisting the hash,
// listing, owner-scoped delete, and authenticating a per-user endpoint by key) are
// covered by the api_keys integration tests.
func apiKeysApp() *fiber.App {
	iss := auth.NewIssuer("test-secret", time.Hour)
	h := &authHandlers{}
	app := fiber.New()
	app.Post("/api/v1/me/api-keys", auth.RequireAuth(iss, testVersions), h.CreateAPIKey)
	app.Get("/api/v1/me/api-keys", auth.RequireAuth(iss, testVersions), h.ListAPIKeys)
	app.Delete("/api/v1/me/api-keys/:id", auth.RequireAuth(iss, testVersions), h.RevokeAPIKey)
	return app
}

// Key management is cookie-only: a request with an API key (or nothing) but no
// session cookie must be rejected, so a leaked key cannot mint or revoke keys.
func TestAPIKeysManagement_IsCookieOnly(t *testing.T) {
	app := apiKeysApp()
	cases := []struct {
		name, method, path string
		bearer             bool
	}{
		{"create, no credential", fiber.MethodPost, "/api/v1/me/api-keys", false},
		{"create, bearer only", fiber.MethodPost, "/api/v1/me/api-keys", true},
		{"list, bearer only", fiber.MethodGet, "/api/v1/me/api-keys", true},
		{"revoke, bearer only", fiber.MethodDelete, "/api/v1/me/api-keys/1", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(context.Background(), tc.method, tc.path, nil)
			if tc.bearer {
				req.Header.Set("Authorization", "Bearer fhk_whatever")
			}
			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("Test: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != fiber.StatusUnauthorized {
				t.Errorf("status = %d, want 401 (key management must be cookie-only)", resp.StatusCode)
			}
		})
	}
}

// The created-key response carries the plaintext token exactly once, alongside the
// display metadata — and never the stored hash.
func TestCreatedAPIKeyResponse_IncludesTokenNotHash(t *testing.T) {
	fields := marshalToFields(t, createdAPIKeyResponse{
		apiKeyResponse: apiKeyResponse{ID: 1, Name: "ci", TokenPrefix: "fhk_ab12cd"},
		Token:          "fhk_the-one-time-secret",
	})
	for _, want := range []string{"id", "name", "token_prefix", "created_at", "last_used_at", "expires_at", "token"} {
		if _, ok := fields[want]; !ok {
			t.Errorf("created-key response missing %q", want)
		}
	}
	if _, leaked := fields["token_hash"]; leaked {
		t.Error("created-key response must not include token_hash")
	}
}

// The list/metadata response never exposes the plaintext token or its hash.
func TestAPIKeyResponse_OmitsSecret(t *testing.T) {
	fields := marshalToFields(t, apiKeyResponse{ID: 1, Name: "ci", TokenPrefix: "fhk_ab12cd"})
	for _, leaked := range []string{"token", "token_hash"} {
		if _, ok := fields[leaked]; ok {
			t.Errorf("list response must not include %q", leaked)
		}
	}
	for _, want := range []string{"id", "name", "token_prefix", "created_at", "last_used_at", "expires_at"} {
		if _, ok := fields[want]; !ok {
			t.Errorf("list response missing %q", want)
		}
	}
}

func marshalToFields(t *testing.T, v any) map[string]json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return fields
}
