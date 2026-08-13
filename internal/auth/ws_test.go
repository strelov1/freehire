package auth

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
)

// wsApp mounts a route behind RequireAuthWS that reports the resolved user id,
// standing in for the websocket handler that reads the same local.
func wsApp(iss *Issuer, keys ...APIKeyAuthenticator) *fiber.App {
	var authenticator APIKeyAuthenticator = fakeKeyAuth{}
	if len(keys) > 0 {
		authenticator = keys[0]
	}
	app := fiber.New()
	app.Get("/tools/ws", RequireAuthWS(iss, anyVersion{1}, authenticator), func(c *fiber.Ctx) error {
		id, _ := UserID(c)
		return c.JSON(fiber.Map{"id": id})
	})
	return app
}

// A long-running harness is not a browser: it holds an API key (the credential
// the CLI uses), not a short-lived session JWT it has no way to re-mint.
func TestRequireAuthWS_AcceptsAnAPIKey(t *testing.T) {
	const key = "fh_live_abc123"
	keys := fakeKeyAuth{validHash: HashAPIKey(key), userID: 9}

	for _, carrier := range []struct{ name, header, value string }{
		{"bearer", fiber.HeaderAuthorization, "Bearer " + key},
		{"subprotocol", "Sec-WebSocket-Protocol", WSSubprotocolMarker + ", " + key},
	} {
		t.Run(carrier.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/tools/ws", nil)
			req.Header.Set(carrier.header, carrier.value)

			resp, err := wsApp(NewIssuer("secret", time.Hour), keys).Test(req)
			if err != nil {
				t.Fatalf("Test: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != fiber.StatusOK {
				t.Fatalf("status = %d, want 200", resp.StatusCode)
			}
			var body struct {
				ID int64 `json:"id"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if body.ID != 9 {
				t.Fatalf("handler saw user %d, want the key's owner", body.ID)
			}
		})
	}
}

func TestRequireAuthWS_RefusesAnUnknownKey(t *testing.T) {
	keys := fakeKeyAuth{validHash: HashAPIKey("fh_live_good"), userID: 9}
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/tools/ws", nil)
	req.Header.Set(fiber.HeaderAuthorization, "Bearer fh_live_revoked")

	resp, err := wsApp(NewIssuer("secret", time.Hour), keys).Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestSubprotocolToken(t *testing.T) {
	tests := []struct {
		name, header, want string
	}{
		{"marker and token", "freehire-jwt, abc.def.ghi", "abc.def.ghi"},
		{"no space after comma", "freehire-jwt,abc", "abc"},
		{"marker only", "freehire-jwt", ""},
		{"wrong marker", "roy-jwt, abc", ""},
		{"empty", "", ""},
		{"extra values", "freehire-jwt, abc, def", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SubprotocolToken(tt.header); got != tt.want {
				t.Errorf("SubprotocolToken(%q) = %q, want %q", tt.header, got, tt.want)
			}
		})
	}
}

func TestRequireAuthWS_AcceptsTheJWTFromEitherCarrier(t *testing.T) {
	iss := NewIssuer("secret", time.Hour)
	token, err := iss.Issue(7, 1)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	for _, carrier := range []struct {
		name, header, value string
	}{
		{"browser subprotocol", "Sec-WebSocket-Protocol", WSSubprotocolMarker + ", " + token},
		{"harness bearer", fiber.HeaderAuthorization, "Bearer " + token},
	} {
		t.Run(carrier.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/tools/ws", nil)
			req.Header.Set(carrier.header, carrier.value)

			resp, err := wsApp(iss).Test(req)
			if err != nil {
				t.Fatalf("Test: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != fiber.StatusOK {
				t.Fatalf("status = %d, want 200", resp.StatusCode)
			}
		})
	}
}

func TestRequireAuthWS_RefusesAnUnauthenticatedHandshake(t *testing.T) {
	iss := NewIssuer("secret", time.Hour)
	expired := NewIssuer("secret", -time.Minute)
	expiredToken, _ := expired.Issue(7, 1)
	otherSecret, _ := NewIssuer("other", time.Hour).Issue(7, 1)

	for _, tt := range []struct{ name, header, value string }{
		{"no credential at all", "", ""},
		{"unmarked subprotocol", "Sec-WebSocket-Protocol", "just-a-token"},
		{"expired token", fiber.HeaderAuthorization, "Bearer " + expiredToken},
		{"token from another secret", "Sec-WebSocket-Protocol", WSSubprotocolMarker + ", " + otherSecret},
	} {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/tools/ws", nil)
			if tt.header != "" {
				req.Header.Set(tt.header, tt.value)
			}

			resp, err := wsApp(iss).Test(req)
			if err != nil {
				t.Fatalf("Test: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != fiber.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", resp.StatusCode)
			}
		})
	}
}
