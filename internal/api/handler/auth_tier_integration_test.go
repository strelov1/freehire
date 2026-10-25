//go:build integration

// Integration test for the caller's tier riding along on GET /api/v1/auth/me — see the
// welcome-pro-subscribers change: the header badge reads this field rather than a second
// request, so it must be present and correct on the same response every other /me consumer
// already gets. Run with: go test -tags=integration ./internal/api/handler/
package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/identity/auth/oauth2server"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
)

// meResponse decodes only the fields this test cares about; toUserResponse's full contract
// (no password hash, every other field present) is TestUserResponse_OmitsPasswordHash's job.
type meResponse struct {
	Data struct {
		ID             int64  `json:"id"`
		Tier           string `json:"tier"`
		AgentConnected bool   `json:"agent_connected"`
	} `json:"data"`
}

func fetchMe(t *testing.T, app *fiber.App, cookie string) meResponse {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/auth/me", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
	resp, err := app.Test(req, 10_000)
	if err != nil {
		t.Fatalf("GET /api/v1/auth/me: %v", err)
	}
	var out meResponse
	if status := decodeJSON(t, resp, &out); status != fiber.StatusOK {
		t.Fatalf("GET /api/v1/auth/me status = %d, want 200", status)
	}
	return out
}

func TestMe_ReportsTier(t *testing.T) {
	app, _, queries, _ := recoveryApp(t)

	// A free account: registered, never granted anything.
	freeResp := postAuthJSON(t, app, "/api/v1/auth/register",
		`{"email":"free-tier@example.test","password":"password123"}`, "")
	defer freeResp.Body.Close()
	freeCookie := sessionCookie(t, freeResp)

	if got := fetchMe(t, app, freeCookie); got.Data.Tier != "free" {
		t.Errorf("free account tier = %q, want %q", got.Data.Tier, "free")
	}

	// A pro account: granted, not sold — this test is about the field riding along, not
	// about how a subscription is bought (see makePro's own comment in
	// auto_apply_enqueue_integration_test.go for why granted is the right source here).
	proResp := postAuthJSON(t, app, "/api/v1/auth/register",
		`{"email":"pro-tier@example.test","password":"password123"}`, "")
	defer proResp.Body.Close()
	proCookie := sessionCookie(t, proResp)
	proCreated := fetchMe(t, app, proCookie)

	if err := queries.SetProUntilGranted(context.Background(), db.SetProUntilGrantedParams{
		ID:    proCreated.Data.ID,
		Until: pgtype.Timestamptz{Time: time.Now().Add(24 * time.Hour), Valid: true},
	}); err != nil {
		t.Fatalf("grant pro: %v", err)
	}

	if got := fetchMe(t, app, proCookie); got.Data.Tier != "pro" {
		t.Errorf("pro account tier = %q, want %q", got.Data.Tier, "pro")
	}
}

// The "connect your agent" strip stops asking once agent_connected is true, so the field
// must turn on for exactly the credentials an agent connects with: a key of any scope but
// `embed`, or an OAuth grant the account's sessions have not revoked.
func TestMe_ReportsAgentConnected(t *testing.T) {
	app, _, queries, _ := recoveryApp(t)
	ctx := context.Background()

	resp := postAuthJSON(t, app, "/api/v1/auth/register",
		`{"email":"agent-connected@example.test","password":"password123"}`, "")
	defer resp.Body.Close()
	cookie := sessionCookie(t, resp)
	me := fetchMe(t, app, cookie)
	if me.Data.AgentConnected {
		t.Fatal("a fresh account reports agent_connected")
	}
	userID := me.Data.ID
	// CreateAPIKey mints only for a verified address.
	if err := queries.SetUserEmailVerified(ctx, userID); err != nil {
		t.Fatalf("SetUserEmailVerified: %v", err)
	}

	createKey := func(hash, scope string) {
		t.Helper()
		if _, err := queries.CreateAPIKey(ctx, db.CreateAPIKeyParams{
			UserID: userID, Name: scope, TokenHash: hash, TokenPrefix: "fh_test", Scope: scope,
		}); err != nil {
			t.Fatalf("CreateAPIKey(%s): %v", scope, err)
		}
	}

	// An embed key grants nothing on this API, so it is not a connected agent.
	createKey("hash-embed", auth.ScopeEmbed)
	if fetchMe(t, app, cookie).Data.AgentConnected {
		t.Error("an embed-only key reports agent_connected")
	}

	createKey("hash-full", auth.ScopeFull)
	if !fetchMe(t, app, cookie).Data.AgentConnected {
		t.Error("a full-scope key does not report agent_connected")
	}

	// A second account connected through MCP OAuth instead of a pasted key.
	grantResp := postAuthJSON(t, app, "/api/v1/auth/register",
		`{"email":"agent-grant@example.test","password":"password123"}`, "")
	defer grantResp.Body.Close()
	grantCookie := sessionCookie(t, grantResp)
	grantUser := fetchMe(t, app, grantCookie).Data.ID

	seedOAuthTestClient(t, queries, "client-agent-connected", []string{"http://127.0.0.1:9999/cb"})
	version, err := queries.GetUserTokenVersion(ctx, grantUser)
	if err != nil {
		t.Fatalf("GetUserTokenVersion: %v", err)
	}
	expiresAt := time.Now().Add(30 * 24 * time.Hour)
	if _, err := queries.CreateOAuthGrant(ctx, db.CreateOAuthGrantParams{
		UserID: grantUser, ClientID: "client-agent-connected",
		AccessTokenHash:    oauth2server.HashToken("fhm_agent-connected"),
		IssuedTokenVersion: version, ExpiresAt: pgconv.Timestamptz(&expiresAt),
	}); err != nil {
		t.Fatalf("CreateOAuthGrant: %v", err)
	}
	if !fetchMe(t, app, grantCookie).Data.AgentConnected {
		t.Error("a live OAuth grant does not report agent_connected")
	}

	// Signing out everywhere revokes the grant, so the strip may ask again. Read through
	// the query: the bump also ends the session cookie fetchMe would present.
	if _, err := queries.BumpUserTokenVersion(ctx, grantUser); err != nil {
		t.Fatalf("BumpUserTokenVersion: %v", err)
	}
	connected, err := queries.UserHasAgentCredential(ctx, grantUser)
	if err != nil {
		t.Fatalf("UserHasAgentCredential: %v", err)
	}
	if connected {
		t.Error("a grant revoked by a token-version bump still reports agent_connected")
	}
}
