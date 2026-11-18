package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/identity/auth"
)

// This file holds the invariant a cross-site navigation must never reach a route that
// mutates state on GET: a SameSite=Lax cookie rides exactly such a navigation by spec, and
// the fit stream spends a paid user's AI credit before its SSE body goes out. The gate is
// Fetch Metadata (Sec-Fetch-Site), which a cross-site top-level traversal stamps
// "cross-site" and an attacker page cannot forge.
//
// The tests mount the REAL register — not a hand-built app — because the bug is a property
// of the mount (which gates stand between the internet and the handler), not of handler
// logic. The mw.key gate runs for real (with stubVersions, the same fixture the other
// handler tests use), the paid-AI stand-in counts its invocations, and the handler body is
// never reached on a denial: reaching it means the gates passed, and the nil dereference
// is the recovered 500 the pass is asserted by (the pagination_window_routes_test.go
// shape).
//
// Mount order the assertions encode: register mounts these routes as
// [mw.key, mw.paidAI, h.StreamMatchAnalysis], and the handler performs the
// Sec-Fetch-Site refusal itself — so a DENIED request has already reached the
// paid-AI gate’s entitlement read (one plan lookup per refused
// request) before the 403 lands. What a denial never reaches is everything after the
// handler's first line: the job read, the reservation, the credential mint, the chain,
// the cache.

type noAPIKeys struct{}

func (noAPIKeys) AuthenticateAPIKey(context.Context, string) (auth.APIKeyIdentity, error) {
	return auth.APIKeyIdentity{}, pgx.ErrNoRows
}

// keyAuthFor builds the real cookie-or-key gate over the shared token-version stub.
func keyAuthFor(iss *auth.Issuer) fiber.Handler {
	return auth.RequireAuthOrKey(iss, testVersions, noAPIKeys{})
}

// countingPaidAI stands in for the paid-AI gate and counts how often it ran.
type countingPaidAI struct {
	calls int
}

func (g *countingPaidAI) handler(feature plan.Feature) fiber.Handler {
	return func(c *fiber.Ctx) error {
		g.calls++
		return markPaidAI(feature)(c)
	}
}

// mountFitStream drives matchHandlers.register for the stream routes with the real keyAuth
// in front of the counting paid-AI stand-in.
func mountFitStream(iss *auth.Issuer, gate *countingPaidAI) *fiber.App {
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Use(recover.New())
	(&matchHandlers{}).register(app.Group("/api/v1"), middleware{
		key:    keyAuthFor(iss),
		paidAI: gate.handler,
	}, newContract(contractHandlers{}))
	return app
}

// streamToken mints a session the shared testVersions stub accepts. The TTL matches the
// sibling fixtures (time.Hour): a zero TTL mints an already-expired token, which the real
// auth gate answers 401 for — proving nothing about the gate under test.
func streamToken(t *testing.T, userID int64) string {
	t.Helper()
	tok, err := auth.NewIssuer("test-secret", time.Hour).Issue(userID, testTokenVersion)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	return tok
}

// getStream drives one cookie-authenticated stream request through the mount with an
// optional Sec-Fetch-Site stamp, and returns the response status.
func getStream(t *testing.T, app *fiber.App, path, tok, secFetchSite string) int {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, path, nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: tok})
	if secFetchSite != "" {
		req.Header.Set(headerSecFetchSite, secFetchSite)
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

// A cross-site top-level navigation carrying a valid session cookie must be refused with
// 403. The mount order (register runs mw.paidAI BEFORE the handler) means the paid-AI
// entitlement read DOES run for a denied request — the assertion records exactly that: the
// gate is consulted once per refused request and nothing else on the paid path is ever
// reached (no reservation, no model, no cache).
func TestCrossSiteNavigationCannotReachTheFitStream(t *testing.T) {
	iss := auth.NewIssuer("test-secret", time.Hour)
	gate := &countingPaidAI{}
	app := mountFitStream(iss, gate)
	tok := streamToken(t, 42)

	paths := []string{
		"/api/v1/jobs/a-job/match-analysis/stream",
		"/api/v1/jobs/a-job/fit/stream",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			if got := getStream(t, app, path, tok, secFetchSiteCrossSite); got != fiber.StatusForbidden {
				t.Fatalf("cross-site navigation = %d, want 403", got)
			}
		})
	}
	if gate.calls != len(paths) {
		t.Errorf("the paid-AI gate ran %d times for %d refused requests, want %d (the mount runs it before the handler; its entitlement read is the cost a denial still pays)", gate.calls, len(paths), len(paths))
	}
}

// A cookie caller whose browser sends no Fetch Metadata at all is denied. The header is
// the only site-intent signal a cookie request carries; its absence proves nothing, and
// "proves nothing" must not spend a credit. As with the cross-site case, the mounted
// paid-AI gate runs before the handler, so the denial still pays its entitlement read —
// asserted here as exactly one consultation.
func TestCookieStreamWithoutFetchMetadataIsRefused(t *testing.T) {
	iss := auth.NewIssuer("test-secret", time.Hour)
	gate := &countingPaidAI{}
	app := mountFitStream(iss, gate)
	tok := streamToken(t, 42)

	if got := getStream(t, app, "/api/v1/jobs/a-job/match-analysis/stream", tok, ""); got != fiber.StatusForbidden {
		t.Fatalf("headerless cookie stream = %d, want 403", got)
	}
	if gate.calls != 1 {
		t.Errorf("the paid-AI gate ran %d times for the refused request, want 1 (the mount runs it before the handler)", gate.calls)
	}
}

// Legitimate cookie entry points survive: the SPA's same-origin EventSource and a
// same-site prefetch. A recovered 500 from the zero-valued handler IS the pass signal —
// the gate must be the thing that stops a hostile request, never the thing that stops a
// legitimate one. The end-to-end behaviour behind this point (events ordered, cache
// written, refund on failure) is asserted by match_analysis_stream_integration_test.go.
func TestSameSiteCallersStillReachTheFitStream(t *testing.T) {
	iss := auth.NewIssuer("test-secret", time.Hour)
	app := mountFitStream(iss, &countingPaidAI{})
	tok := streamToken(t, 42)

	t.Run("same-origin EventSource", func(t *testing.T) {
		if got := getStream(t, app, "/api/v1/jobs/a-job/match-analysis/stream", tok, secFetchSiteSameOrigin); got != fiber.StatusInternalServerError {
			t.Fatalf("same-origin stream = %d, want 500 (reached the zero-valued handler)", got)
		}
	})
	t.Run("same-site prefetch", func(t *testing.T) {
		if got := getStream(t, app, "/api/v1/jobs/a-job/match-analysis/stream", tok, secFetchSiteSameSite); got != fiber.StatusInternalServerError {
			t.Fatalf("same-site stream = %d, want 500 (reached the zero-valued handler)", got)
		}
	})
}

// A valid Bearer session bypasses the cookie-only gate even with a cross-site stamp.
func TestBearerSessionReachesTheFitStream(t *testing.T) {
	iss := auth.NewIssuer("test-secret", time.Hour)
	gate := &countingPaidAI{}
	app := mountFitStream(iss, gate)
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/api/v1/jobs/a-job/match-analysis/stream", nil)
	req.Header.Set(fiber.HeaderAuthorization, "Bearer "+streamToken(t, 42))
	req.Header.Set(headerSecFetchSite, secFetchSiteCrossSite)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusInternalServerError {
		t.Fatalf("Bearer stream = %d, want 500 from the zero-valued handler", resp.StatusCode)
	}
	if gate.calls != 1 {
		t.Errorf("paid gate calls = %d, want 1", gate.calls)
	}
}
