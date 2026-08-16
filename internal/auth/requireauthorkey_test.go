package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
)

// fakeKeyAuth authenticates exactly one token hash to a full-scope key for a user id;
// any other hash returns pgx.ErrNoRows, standing in for an unknown, revoked, or expired
// key — mirroring the real db layer, which reports all of those as ErrNoRows. Scope
// behaviour has its own tests in keyscope_test.go.
type fakeKeyAuth struct {
	validHash string
	userID    int64
}

func (f fakeKeyAuth) AuthenticateAPIKey(_ context.Context, tokenHash string) (APIKeyIdentity, error) {
	if tokenHash == f.validHash {
		return APIKeyIdentity{UserID: f.userID, Scope: ScopeFull}, nil
	}
	return APIKeyIdentity{}, pgx.ErrNoRows
}

// errKeyAuth fails every lookup with a generic error, standing in for a database
// outage — which the middleware must NOT mask as a 401/anonymous.
type errKeyAuth struct{ err error }

func (f errKeyAuth) AuthenticateAPIKey(_ context.Context, _ string) (APIKeyIdentity, error) {
	return APIKeyIdentity{}, f.err
}

// dualAuthApp mounts a route behind RequireAuthOrKey that echoes the resolved user
// id, so tests assert both access control and that identity propagates into the
// handler via the shared c.Locals.
func dualAuthApp(iss *Issuer, keys APIKeyAuthenticator) *fiber.App {
	app := fiber.New()
	app.Get("/me", RequireAuthOrKey(iss, anyVersion{1}, keys), func(c *fiber.Ctx) error {
		id, ok := UserID(c)
		if !ok {
			return fiber.NewError(fiber.StatusInternalServerError, "user id missing from context")
		}
		return c.JSON(fiber.Map{"id": id, "via_key": ViaAPIKey(c), "via_cookie": ViaCookie(c), "is_extension": IsExtensionBearer(c)})
	})
	return app
}

func decodeID(t *testing.T, resp *http.Response) int64 {
	t.Helper()
	var body struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return body.ID
}

func TestRequireAuthOrKey_ValidKeyAuthenticates(t *testing.T) {
	iss := NewIssuer("secret", time.Hour)
	const token = "fhk_test-key"
	keys := fakeKeyAuth{validHash: HashAPIKey(token), userID: 9}

	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := dualAuthApp(iss, keys).Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if id := decodeID(t, resp); id != 9 {
		t.Errorf("handler saw user id %d, want 9", id)
	}
}

func decodeViaKey(t *testing.T, resp *http.Response) bool {
	t.Helper()
	var body struct {
		ViaKey bool `json:"via_key"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return body.ViaKey
}

func TestRequireAuthOrKey_FlagsKeyAuth(t *testing.T) {
	iss := NewIssuer("secret", time.Hour)
	const token = "fhk_test-key"
	keys := fakeKeyAuth{validHash: HashAPIKey(token), userID: 9}

	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := dualAuthApp(iss, keys).Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	if !decodeViaKey(t, resp) {
		t.Error("ViaAPIKey should be true for key auth")
	}
}

func TestRequireAuthOrKey_CookieIsNotViaKey(t *testing.T) {
	iss := NewIssuer("secret", time.Hour)
	token, _ := iss.Issue(7, 1)

	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/me", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: token})
	resp, err := dualAuthApp(iss, fakeKeyAuth{}).Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	if decodeViaKey(t, resp) {
		t.Error("ViaAPIKey should be false for cookie auth")
	}
}

func TestRequireAuthOrKey_ValidCookieAuthenticates(t *testing.T) {
	iss := NewIssuer("secret", time.Hour)
	keys := fakeKeyAuth{} // no valid key; the cookie must carry the identity
	token, _ := iss.Issue(7, 1)

	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/me", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: token})

	resp, err := dualAuthApp(iss, keys).Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if id := decodeID(t, resp); id != 7 {
		t.Errorf("handler saw user id %d, want 7", id)
	}
}

func decodeViaCookie(t *testing.T, resp *http.Response) bool {
	t.Helper()
	var body struct {
		ViaCookie bool `json:"via_cookie"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return body.ViaCookie
}

// ViaCookie is the signal `read_current_page`'s registration gates on (see the
// confine-browse-preset-to-extension change): the website authenticates by cookie,
// so a browse session reached from there must not read as the extension's own
// Bearer-JWT carrier.
func TestRequireAuthOrKey_FlagsCookieAuth(t *testing.T) {
	iss := NewIssuer("secret", time.Hour)
	token, _ := iss.Issue(7, 1)

	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/me", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: token})

	resp, err := dualAuthApp(iss, fakeKeyAuth{}).Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	if !decodeViaCookie(t, resp) {
		t.Error("ViaCookie should be true for cookie auth")
	}
}

func TestRequireAuthOrKey_JWTBearerIsNotViaCookie(t *testing.T) {
	iss := NewIssuer("secret", time.Hour)
	token, _ := iss.Issue(42, 1) // a session JWT presented as a Bearer header, as the extension does
	keys := fakeKeyAuth{}        // no valid API key — the JWT must carry the identity

	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := dualAuthApp(iss, keys).Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	if decodeViaCookie(t, resp) {
		t.Error("ViaCookie should be false for a Bearer session JWT — the extension's own carrier")
	}
}

func TestRequireAuthOrKey_APIKeyIsNotViaCookie(t *testing.T) {
	iss := NewIssuer("secret", time.Hour)
	const token = "fhk_test-key"
	keys := fakeKeyAuth{validHash: HashAPIKey(token), userID: 9}

	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := dualAuthApp(iss, keys).Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	if decodeViaCookie(t, resp) {
		t.Error("ViaCookie should be false for an API key")
	}
}

func decodeIsExtension(t *testing.T, resp *http.Response) bool {
	t.Helper()
	var body struct {
		IsExtension bool `json:"is_extension"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return body.IsExtension
}

// IsExtensionBearer is the single primitive every caller that must confine a
// capability to the extension's own connection shares (the assistant's browse
// preset, agent-driven autofill) — rather than each re-deriving "not cookie, not
// API key" inline, which is easy to phrase differently at each call site and drift.
// See the confine-browse-preset-to-extension change.
func TestIsExtensionBearer_TrueOnlyForTheSessionJWT(t *testing.T) {
	iss := NewIssuer("secret", time.Hour)
	const apiKey = "fhk_test-key"
	keys := fakeKeyAuth{validHash: HashAPIKey(apiKey), userID: 9}
	cookieToken, _ := iss.Issue(7, 1)
	jwtBearerToken, _ := iss.Issue(42, 1)

	cases := []struct {
		name   string
		cookie string // "" = no cookie
		bearer string // "" = no Authorization header
		want   bool
	}{
		{"cookie", cookieToken, "", false},
		{"API key", "", apiKey, false},
		{"Bearer session JWT — the extension's own carrier", "", jwtBearerToken, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/me", nil)
			if tc.cookie != "" {
				req.AddCookie(&http.Cookie{Name: CookieName, Value: tc.cookie})
			}
			if tc.bearer != "" {
				req.Header.Set("Authorization", "Bearer "+tc.bearer)
			}
			resp, err := dualAuthApp(iss, keys).Test(req)
			if err != nil {
				t.Fatalf("Test: %v", err)
			}
			defer resp.Body.Close()
			if got := decodeIsExtension(t, resp); got != tc.want {
				t.Errorf("IsExtensionBearer = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRequireAuthOrKey_CookieTakesPrecedenceOverKey(t *testing.T) {
	iss := NewIssuer("secret", time.Hour)
	const token = "fhk_test-key"
	keys := fakeKeyAuth{validHash: HashAPIKey(token), userID: 9}
	cookie, _ := iss.Issue(7, 1)

	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/me", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: cookie})
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := dualAuthApp(iss, keys).Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	if id := decodeID(t, resp); id != 7 {
		t.Errorf("handler saw user id %d, want 7 (a valid cookie should win)", id)
	}
}

func TestRequireAuthOrKey_InvalidCookieFallsThroughToKey(t *testing.T) {
	iss := NewIssuer("secret", time.Hour)
	const token = "fhk_test-key"
	keys := fakeKeyAuth{validHash: HashAPIKey(token), userID: 9}

	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/me", nil)
	req.AddCookie(&http.Cookie{Name: CookieName, Value: "not-a-jwt"})
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := dualAuthApp(iss, keys).Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if id := decodeID(t, resp); id != 9 {
		t.Errorf("handler saw user id %d, want 9 (key authenticates after a bad cookie)", id)
	}
}

func TestRequireAuthOrKey_JWTBearerAuthenticatesAsSession(t *testing.T) {
	iss := NewIssuer("secret", time.Hour)
	token, _ := iss.Issue(42, 1) // a session JWT presented as a Bearer header
	keys := fakeKeyAuth{}        // no valid API key — the JWT must carry the identity

	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/me", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := dualAuthApp(iss, keys).Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	var body struct {
		ID     int64 `json:"id"`
		ViaKey bool  `json:"via_key"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.ID != 42 {
		t.Errorf("handler saw user id %d, want 42", body.ID)
	}
	if body.ViaKey {
		t.Error("a JWT bearer is a session credential, not via-key")
	}
}

func TestRequireAuthOrKey_KeyLookupErrorIsNot401(t *testing.T) {
	iss := NewIssuer("secret", time.Hour)
	keys := errKeyAuth{err: errors.New("connection refused")}

	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/me", nil)
	req.Header.Set("Authorization", "Bearer fhk_whatever")

	resp, err := dualAuthApp(iss, keys).Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503 (a lookup outage must surface as 503)", resp.StatusCode)
	}
}

func TestOptionalAuth_KeyLookupErrorPropagates(t *testing.T) {
	iss := NewIssuer("secret", time.Hour)

	newApp := func(keys APIKeyAuthenticator) *fiber.App {
		app := fiber.New()
		app.Get("/job", OptionalAuth(iss, anyVersion{1}, keys), func(c *fiber.Ctx) error {
			_, ok := UserID(c)
			return c.JSON(fiber.Map{"authed": ok})
		})
		return app
	}
	newReq := func() *http.Request {
		req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/job", nil)
		req.Header.Set("Authorization", "Bearer fhk_whatever")
		return req
	}

	// Unknown key (ErrNoRows) passes through anonymously.
	resp, err := newApp(fakeKeyAuth{}).Test(newReq())
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("unknown key: status = %d, want 200", resp.StatusCode)
	}

	// A lookup outage is surfaced as 503, not silently degraded to anonymous.
	resp, err = newApp(errKeyAuth{err: errors.New("connection refused")}).Test(newReq())
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusServiceUnavailable {
		t.Errorf("outage: status = %d, want 503", resp.StatusCode)
	}
}

func TestRequireAuthOrKey_RejectsUnauthorized(t *testing.T) {
	iss := NewIssuer("secret", time.Hour)
	const token = "fhk_valid"
	keys := fakeKeyAuth{validHash: HashAPIKey(token), userID: 9}

	cases := []struct {
		name   string
		cookie string // empty = no cookie
		bearer string // empty = no Authorization header
	}{
		{"no credentials", "", ""},
		{"unknown key", "", "fhk_unknown"},
		{"garbage bearer", "", "not-even-prefixed"},
		{"malformed cookie only", "not-a-jwt", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/me", nil)
			if tc.cookie != "" {
				req.AddCookie(&http.Cookie{Name: CookieName, Value: tc.cookie})
			}
			if tc.bearer != "" {
				req.Header.Set("Authorization", "Bearer "+tc.bearer)
			}
			resp, err := dualAuthApp(iss, keys).Test(req)
			if err != nil {
				t.Fatalf("Test: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != fiber.StatusUnauthorized {
				t.Errorf("status = %d, want 401", resp.StatusCode)
			}
		})
	}
}
