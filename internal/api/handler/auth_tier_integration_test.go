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
	"github.com/strelov1/freehire/internal/platform/db"
)

// meResponse decodes only the fields this test cares about; toUserResponse's full contract
// (no password hash, every other field present) is TestUserResponse_OmitsPasswordHash's job.
type meResponse struct {
	Data struct {
		ID   int64  `json:"id"`
		Tier string `json:"tier"`
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
