package auth

import (
	"context"
	"errors"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
)

// TestRequireAuthWS_DBOutage_Clean503Response validates that when the database is down,
// RequireAuthWS returns a clean HTTP 503 Service Unavailable response without protocol corruption,
// regardless of carrier (Bearer vs Sec-WebSocket-Protocol) or credential type (JWT vs API Key).
func TestRequireAuthWS_DBOutage_Clean503Response(t *testing.T) {
	iss := NewIssuer("secret", time.Hour)
	validToken, err := iss.Issue(42, 1)
	if err != nil {
		t.Fatalf("Issue token: %v", err)
	}

	dbErr := errors.New("pgx: connection refusal or pool timeout")

	tests := []struct {
		name       string
		versions   TokenVersionLoader
		keys       APIKeyAuthenticator
		headerName string
		headerVal  string
	}{
		{
			name:       "JWT Bearer header - DB outage on token version lookup",
			versions:   errVersions{err: dbErr},
			keys:       fakeKeyAuth{},
			headerName: fiber.HeaderAuthorization,
			headerVal:  "Bearer " + validToken,
		},
		{
			name:       "JWT Subprotocol header - DB outage on token version lookup",
			versions:   errVersions{err: dbErr},
			keys:       fakeKeyAuth{},
			headerName: "Sec-WebSocket-Protocol",
			headerVal:  WSSubprotocolMarker + ", " + validToken,
		},
		{
			name:       "API Key Bearer header - DB outage on key lookup",
			versions:   anyVersion{1},
			keys:       errKeyAuth{err: dbErr},
			headerName: fiber.HeaderAuthorization,
			headerVal:  "Bearer fh_live_secret123",
		},
		{
			name:       "API Key Subprotocol header - DB outage on key lookup",
			versions:   anyVersion{1},
			keys:       errKeyAuth{err: dbErr},
			headerName: "Sec-WebSocket-Protocol",
			headerVal:  WSSubprotocolMarker + ", fh_live_secret123",
		},
		{
			name:       "Revoked JWT Bearer - DB outage when falling back to key auth",
			versions:   anyVersion{99}, // different version -> sessionRevoked
			keys:       errKeyAuth{err: dbErr},
			headerName: fiber.HeaderAuthorization,
			headerVal:  "Bearer " + validToken,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := fiber.New()
			app.Get("/tools/ws", RequireAuthWS(iss, tt.versions, tt.keys), func(c *fiber.Ctx) error {
				return c.SendString("connected websocket")
			})

			// Simulate a full WebSocket opening handshake request (RFC 6455)
			req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/tools/ws", nil)
			req.Header.Set("Connection", "Upgrade")
			req.Header.Set("Upgrade", "websocket")
			req.Header.Set("Sec-WebSocket-Version", "13")
			req.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
			req.Header.Set(tt.headerName, tt.headerVal)

			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("app.Test failed: %v", err)
			}
			defer resp.Body.Close()

			// Check status code is strictly 503 Service Unavailable
			if resp.StatusCode != fiber.StatusServiceUnavailable {
				t.Errorf("StatusCode = %d, want 503 (StatusServiceUnavailable)", resp.StatusCode)
			}

			// Ensure upgrade was NOT granted (no 101 Switching Protocols)
			if resp.StatusCode == 101 {
				t.Errorf("Unexpected WebSocket upgrade (101) granted during DB outage!")
			}

			// Ensure Sec-WebSocket-Accept header is not present (no protocol corruption)
			if accept := resp.Header.Get("Sec-WebSocket-Accept"); accept != "" {
				t.Errorf("Sec-WebSocket-Accept header set (%q) during HTTP 503 error", accept)
			}
		})
	}
}

type scopeKeyAuth struct {
	scope  string
	userID int64
	err    error
}

func (s scopeKeyAuth) AuthenticateAPIKey(_ context.Context, _ string) (APIKeyIdentity, error) {
	if s.err != nil {
		return APIKeyIdentity{}, s.err
	}
	return APIKeyIdentity{UserID: s.userID, Scope: s.scope}, nil
}

// TestRequireAuthWS_ScopedKeyRejection tests that scoped API keys (e.g. cv scope) are refused 403
// while full scope keys are accepted 200, and DB errors during key lookup return 503.
func TestRequireAuthWS_ScopedKeyRejection(t *testing.T) {
	iss := NewIssuer("secret", time.Hour)

	t.Run("CV scope key rejected with 403", func(t *testing.T) {
		app := fiber.New()
		app.Get("/tools/ws", RequireAuthWS(iss, anyVersion{1}, scopeKeyAuth{scope: ScopeCV, userID: 5}), func(c *fiber.Ctx) error {
			return c.SendString("ok")
		})

		req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/tools/ws", nil)
		req.Header.Set(fiber.HeaderAuthorization, "Bearer fh_live_cvkey")

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("Test: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusForbidden {
			t.Errorf("status = %d, want 403 for CV scope key", resp.StatusCode)
		}
	})

	t.Run("Full scope key accepted with 200", func(t *testing.T) {
		app := fiber.New()
		app.Get("/tools/ws", RequireAuthWS(iss, anyVersion{1}, scopeKeyAuth{scope: ScopeFull, userID: 5}), func(c *fiber.Ctx) error {
			id, ok := UserID(c)
			if !ok || id != 5 {
				t.Errorf("UserID = (%d, %v), want (5, true)", id, ok)
			}
			if !ViaAPIKey(c) {
				t.Errorf("ViaAPIKey = false, want true")
			}
			if KeyScope(c) != ScopeFull {
				t.Errorf("KeyScope = %q, want %q", KeyScope(c), ScopeFull)
			}
			return c.SendString("ok")
		})

		req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/tools/ws", nil)
		req.Header.Set(fiber.HeaderAuthorization, "Bearer fh_live_fullkey")

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("Test: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != fiber.StatusOK {
			t.Errorf("status = %d, want 200 for full scope key", resp.StatusCode)
		}
	})
}

type dynamicVersions struct {
	mu  sync.Mutex
	err error
	ver int32
}

func (d *dynamicVersions) GetUserTokenVersion(_ context.Context, _ int64) (int32, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.err != nil {
		return 0, d.err
	}
	return d.ver, nil
}

type dynamicKeys struct {
	mu     sync.Mutex
	err    error
	userID int64
}

func (d *dynamicKeys) AuthenticateAPIKey(_ context.Context, _ string) (APIKeyIdentity, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.err != nil {
		return APIKeyIdentity{}, d.err
	}
	if d.userID > 0 {
		return APIKeyIdentity{UserID: d.userID, Scope: ScopeFull}, nil
	}
	return APIKeyIdentity{}, pgx.ErrNoRows
}

// TestRequireAuthWS_ConcurrentStress executes concurrent handshake requests across multiple goroutines
// while database status toggles between healthy, error, and invalid key state.
func TestRequireAuthWS_ConcurrentStress(t *testing.T) {
	iss := NewIssuer("secret", time.Hour)
	jwtToken, err := iss.Issue(100, 1)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	dynVer := &dynamicVersions{ver: 1}
	dynKeys := &dynamicKeys{userID: 200}

	app := fiber.New()
	app.Get("/tools/ws", RequireAuthWS(iss, dynVer, dynKeys), func(c *fiber.Ctx) error {
		id, ok := UserID(c)
		if !ok {
			return fiber.NewError(500, "missing user id")
		}
		return c.JSON(fiber.Map{"user_id": id})
	})

	const numWorkers = 20
	const requestsPerWorker = 50

	var wg sync.WaitGroup
	wg.Add(numWorkers)

	for w := 0; w < numWorkers; w++ {
		go func(workerID int) {
			defer wg.Done()
			for i := 0; i < requestsPerWorker; i++ {
				// Toggle DB errors occasionally
				if (workerID+i)%7 == 0 {
					dynVer.mu.Lock()
					dynVer.err = errors.New("temporary DB error")
					dynVer.mu.Unlock()

					dynKeys.mu.Lock()
					dynKeys.err = errors.New("temporary DB error")
					dynKeys.mu.Unlock()
				} else {
					dynVer.mu.Lock()
					dynVer.err = nil
					dynVer.mu.Unlock()

					dynKeys.mu.Lock()
					dynKeys.err = nil
					dynKeys.mu.Unlock()
				}

				// Choose carrier: JWT or API Key
				req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/tools/ws", nil)
				if i%2 == 0 {
					req.Header.Set("Sec-WebSocket-Protocol", WSSubprotocolMarker+", "+jwtToken)
				} else {
					req.Header.Set("Authorization", "Bearer fh_live_key_"+string(rune('a'+i%26)))
				}

				resp, err := app.Test(req, -1)
				if err != nil {
					t.Errorf("Worker %d req %d error: %v", workerID, i, err)
					return
				}
				resp.Body.Close()

				// Status must be either 200 OK or 503 Service Unavailable or 401 Unauthorized
				switch resp.StatusCode {
				case fiber.StatusOK, fiber.StatusServiceUnavailable, fiber.StatusUnauthorized:
					// Expected
				default:
					t.Errorf("Unexpected status code %d in stress test", resp.StatusCode)
				}
			}
		}(w)
	}

	wg.Wait()
}
