package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/strelov1/freehire/internal/identity/accounts"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
)

// The request-origin gate on session-issuing routes. The scenario the gate closes:
// an attacker page submits a top-level cross-site HTML form POST to /auth/login with
// the attacker's own credentials; the browser attaches an Origin header, the server
// must refuse the request BEFORE verifying anything, so no session cookie is written
// and the victim's browser never stores an attacker principal. Non-browser clients
// (curl, the CLI, a mobile SDK) send no Origin or Referer at all and must keep
// working — the gate refuses nothing that presents no browser metadata.

const testFrontendOrigin = "https://freehire.me"

// sameOriginApp mounts the login route behind h.requireSameOrigin over a repo whose
// Login succeeds (the credentials themselves are correct — the point is what the gate
// does with an otherwise-valid request).
func sameOriginApp(t *testing.T, servedHosts ...string) *fiber.App {
	t.Helper()
	queries := db.New(fakeDBTX{ScanAnswer: func(dest ...any) error {
		// GetUserTokenVersion scans one int32: the account's session generation.
		if len(dest) == 1 {
			if p, ok := dest[0].(*int32); ok {
				*p = 3
				return nil
			}
		}
		return errors.New("fakeDBTX: unexpected Scan shape")
	}})
	h := &authHandlers{
		issuer: auth.NewIssuer("test-secret", time.Hour),
		accounts: accounts.New(fakeRepoWithUser{
			UserByEmailUser: accounts.User{ID: 7, Email: "attacker@example.com"},
			StoredHash:      "$2a$04$8FiWKw7BxY5jF3jWY8mnb.VHkbEJbBEgDrBXCSDS6sByrdlKBaOb.", // "password"
			HasPassword:     true,
		}, authHasher{}),
		queries:        queries,
		browserOrigins: auth.BrowserOrigins(testFrontendOrigin, nil, servedHosts...),
	}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Post("/api/v1/auth/login", h.requireSameOrigin(), h.Login)
	return app
}

// fakeDBTX is a minimal db.DBTX for the one generated query the success path needs
// (GetUserTokenVersion, a QueryRow returning the account's session generation). It
// lets a unit test exercise the full session-issuing path with no database and no
// production fallback: ScanAnswer is what that one query scans.
type fakeDBTX struct {
	ScanAnswer func(dest ...any) error
}

func (f fakeDBTX) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, errors.New("fakeDBTX: Exec not expected")
}
func (f fakeDBTX) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, errors.New("fakeDBTX: Query not expected")
}
func (f fakeDBTX) QueryRow(context.Context, string, ...any) pgx.Row {
	return fakeRow{answer: f.ScanAnswer}
}
func (f fakeDBTX) CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error) {
	return 0, errors.New("fakeDBTX: CopyFrom not expected")
}
func (f fakeDBTX) SendBatch(context.Context, *pgx.Batch) pgx.BatchResults {
	return nil
}

// fakeRow answers a single Scan — exactly what GetUserTokenVersion's :one query
// performs on the row QueryRow returns.
type fakeRow struct {
	answer func(dest ...any) error
}

func (r fakeRow) Scan(dest ...any) error { return r.answer(dest...) }

// doLogin sends the login request shape described and returns the raw response, so a
// test can assert on BOTH the status and the Set-Cookie headers.
func doLogin(t *testing.T, app *fiber.App, headers map[string]string) *http.Response {
	t.Helper()
	body := `email=attacker%40example.com&password=password`
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationForm)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	return resp
}

func TestLogin_CrossSiteFormPostIsRefusedWithoutSettingSession(t *testing.T) {
	app := sameOriginApp(t)

	// The exact request a top-level cross-site form navigation produces: an Origin
	// header naming the attacker's page. The credentials are VALID — the refusal must
	// come from the origin gate, not from authentication.
	resp := doLogin(t, app, map[string]string{
		fiber.HeaderOrigin: "https://evil.example",
	})
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("status = %d, want 403 for a cross-site Origin", resp.StatusCode)
	}
	if cookies := resp.Cookies(); len(cookies) > 0 {
		t.Errorf("a refused login must set no cookie, got %v", cookies)
	}
	// The refusal must carry the repo's coded-error envelope, keyed on the gate's own
	// code — this is what isCrossSiteRefusal matches, and what separates the gate's
	// answer from an unrelated future 403.
	var body struct {
		Error string `json:"error"`
		Code  string `json:"code"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode refusal body: %v", err)
	}
	if body.Code != codeCrossSiteRefusal {
		t.Errorf("refusal code = %q, want %q", body.Code, codeCrossSiteRefusal)
	}
	if body.Error == "" {
		t.Error("refusal must carry a readable error message")
	}
}

// A foreign Referer decides exactly like a foreign Origin: present metadata must
// name a served origin. The Fetch Metadata header is irrelevant to this branch —
// foreign/malformed Referer refuses with or without it.
func TestLogin_ForeignRefererIsRefused(t *testing.T) {
	for _, tc := range []struct {
		name, referer string
	}{
		// Privacy-tool shape: Origin stripped, Referer left, attacker page.
		{"foreign referer, no fetch metadata", "https://evil.example/login-form.html"},
		{"foreign referer with corroborating metadata", "https://evil.example/login-form.html"},
		// A Referer that parses to nothing usable is metadata that names no served
		// origin and refuses like any other.
		{"malformed referer", "::::not-a-url"},
		{"lookalike referer host", testFrontendOrigin + ".evil.example/form.html"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := sameOriginApp(t)
			headers := map[string]string{fiber.HeaderReferer: tc.referer}
			if strings.Contains(tc.name, "corroborating") {
				headers["Sec-Fetch-Site"] = "cross-site"
			}
			resp := doLogin(t, app, headers)
			defer resp.Body.Close()

			if resp.StatusCode != fiber.StatusForbidden {
				t.Fatalf("status = %d, want 403 for %s", resp.StatusCode, tc.name)
			}
			if cookies := resp.Cookies(); len(cookies) > 0 {
				t.Errorf("a refused login must set no cookie, got %v", cookies)
			}
		})
	}
}

// The other same shape: a privacy tool stripped Origin but left Referer, and the
// referring page is the attacker's. Referer alone is site information too — and on
// the fleet that sends Fetch Metadata, the browser itself says the request was
// initiated cross-site, which is what the gate refuses on.
func TestLogin_CrossSiteRefererWithFetchMetadataIsRefused(t *testing.T) {
	app := sameOriginApp(t)

	resp := doLogin(t, app, map[string]string{
		fiber.HeaderReferer: "https://evil.example/login-form.html",
		"Sec-Fetch-Site":    "cross-site",
	})
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("status = %d, want 403 for a cross-site Referer with Sec-Fetch-Site: cross-site", resp.StatusCode)
	}
	if cookies := resp.Cookies(); len(cookies) > 0 {
		t.Errorf("a refused login must set no cookie, got %v", cookies)
	}
}

// The boundary the headerless shape rests on, stated precisely: a request whose
// metadata is absent must actually be absent. A browser that sends Fetch Metadata
// never gets classified as a headerless CLI call — even with Origin and Referer
// stripped, its own Sec-Fetch-Site: cross-site refuses it.
func TestLogin_FetchMetadataCrossSiteIsRefusedWithoutAnyOtherHeader(t *testing.T) {
	app := sameOriginApp(t)

	resp := doLogin(t, app, map[string]string{
		"Sec-Fetch-Site": "cross-site",
	})
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("status = %d, want 403 for Sec-Fetch-Site: cross-site with Origin and Referer stripped", resp.StatusCode)
	}
	if cookies := resp.Cookies(); len(cookies) > 0 {
		t.Errorf("a refused login must set no cookie, got %v", cookies)
	}
}

// The value is matched case-insensitively and never on a substring.
func TestLogin_FetchMetadataCaseInsensitive(t *testing.T) {
	app := sameOriginApp(t)

	resp := doLogin(t, app, map[string]string{"Sec-Fetch-Site": "Cross-Site"})
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("status = %d, want 403 for a non-lowercase cross-site value", resp.StatusCode)
	}

	app2 := sameOriginApp(t)
	resp2 := doLogin(t, app2, map[string]string{"Sec-Fetch-Site": "cross-site-ish"})
	defer resp2.Body.Close()
	if resp2.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200 — an unknown value is absent-metadata, not cross-site", resp2.StatusCode)
	}
}

// The Fetch Metadata values a browser sends for requests the site itself initiated
// (same-origin fetch, same-site navigation from another freehire subdomain, and the
// address-bar "none") never refuse.
func TestLogin_FetchMetadataSameSiteValuesAccepted(t *testing.T) {
	for _, v := range []string{"same-origin", "same-site", "none"} {
		t.Run(v, func(t *testing.T) {
			app := sameOriginApp(t)
			resp := doLogin(t, app, map[string]string{"Sec-Fetch-Site": v})
			defer resp.Body.Close()
			if resp.StatusCode != fiber.StatusOK {
				t.Fatalf("status = %d, want 200 for Sec-Fetch-Site: %s", resp.StatusCode, v)
			}
		})
	}
}

// The genuinely headerless shape — no Origin, no Referer, no Fetch Metadata — is
// the non-browser compatibility case and is accepted. A present-but-foreign
// Referer is NOT this shape (see TestLogin_ForeignRefererIsRefused).
func TestLogin_GenuinelyHeaderlessShapeAccepted(t *testing.T) {
	app := sameOriginApp(t)

	resp := doLogin(t, app, nil)
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200 for the genuinely headerless shape", resp.StatusCode)
	}
}

// A form post from a foreign page whose URL merely CONTAINS the allowed origin as a
// substring — https://freehire.me.evil.example — must not pass the Referer prefix
// match. (Covered without Fetch Metadata by TestLogin_ForeignRefererIsRefused's
// lookalike case; this one adds the metadata-corroborated shape.)
func TestLogin_RefererPrefixLookalikeIsRefused(t *testing.T) {
	app := sameOriginApp(t)

	resp := doLogin(t, app, map[string]string{
		fiber.HeaderReferer: testFrontendOrigin + ".evil.example/form.html",
		"Sec-Fetch-Site":    "cross-site",
	})
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("status = %d, want 403 for a lookalike Referer host", resp.StatusCode)
	}
}

// The legitimate case the gate must not break: the SPA's own same-origin fetch
// (Origin names the frontend origin), and the no-metadata programmatic client
// (curl / the documented JSON API / the CLI). Both must reach Login and, with valid
// credentials, receive the session cookie.
func TestLogin_SameOriginAndHeaderlessClientsStillSignIn(t *testing.T) {
	app := sameOriginApp(t)

	for name, headers := range map[string]map[string]string{
		"same-origin fetch (Origin: the frontend origin)": {
			fiber.HeaderOrigin: testFrontendOrigin,
		},
		"programmatic client (no Origin, no Referer)": {},
		// The documented curl shape: JSON content type, no browser metadata.
		"programmatic JSON client": {},
	} {
		t.Run(name, func(t *testing.T) {
			contentType := fiber.MIMEApplicationForm
			body := `email=attacker%40example.com&password=password`
			if strings.Contains(name, "JSON") {
				contentType = fiber.MIMEApplicationJSON
				body = `{"email":"attacker@example.com","password":"password"}`
			}
			req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
			req.Header.Set(fiber.HeaderContentType, contentType)
			for k, v := range headers {
				req.Header.Set(k, v)
			}
			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("app.Test: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != fiber.StatusOK {
				t.Fatalf("status = %d, want 200", resp.StatusCode)
			}
			found := false
			for _, c := range resp.Cookies() {
				if c.Name == auth.CookieName && c.Value != "" {
					found = true
				}
			}
			if !found {
				t.Error("a same-origin (or metadata-less) login must set the session cookie")
			}
		})
	}
}

// Some privacy tools strip Origin but leave Referer. The request then carries its
// site information in Referer alone, and a same-origin Referer must be accepted.
func TestLogin_SameOriginRefererAcceptedWhenOriginStripped(t *testing.T) {
	app := sameOriginApp(t)

	resp := doLogin(t, app, map[string]string{
		fiber.HeaderReferer: testFrontendOrigin + "/login",
	})
	defer resp.Body.Close()

	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200 for a same-origin Referer with Origin stripped", resp.StatusCode)
	}
}

// The security property the route wiring pins: login, register, logout and
// logout-all are mounted behind the gate. A future refactor that drops the
// middleware from any of them fails here, because a cross-site Origin must then
// pass through the gate and reach the handler (the named gate answers 200).
// RenderError is wired as the error handler so the codedError envelope renders —
// matching production, where fiber.New uses it.
func TestAuthRoutes_SessionRoutesRequireSameOrigin(t *testing.T) {
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	api := app.Group("/api/v1")
	h := &authHandlers{browserOrigins: []string{testFrontendOrigin}}
	h.register(api, middleware{
		cookie: namedGate("cookie"),
		cvKey:  namedGate("cvKey"),
	}, newContract(contractHandlers{authHandlers: h}))

	for _, path := range []string{
		"/api/v1/auth/login",
		"/api/v1/auth/register",
		"/api/v1/auth/logout",
		"/api/v1/auth/logout-all",
	} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, path, nil)
			req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationForm)
			req.Header.Set(fiber.HeaderOrigin, "https://evil.example")
			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("Test: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != fiber.StatusForbidden {
				t.Errorf("status = %d, want 403 — %s must sit behind the same-origin gate", resp.StatusCode, path)
			}
		})
	}
}

// The gate is bound to the browser origins this deployment serves (the CORS
// allowlist's same pair). A deployment that serves another apex must admit that
// apex's origin, and the frontend origin's https scheme is never assumed for a
// domain configured bare — the caller's helper decides.
func TestBrowserOrigins_FrontendPlusCookieDomainApexes(t *testing.T) {
	got := auth.BrowserOrigins("https://freehire.me", []string{".freehire.me", "example.com"})
	want := []string{"https://freehire.me", "https://freehire.me", "https://example.com"}
	if len(got) != len(want) {
		t.Fatalf("BrowserOrigins = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("BrowserOrigins[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestLoginServedHostOrigin(t *testing.T) {
	for _, tc := range []struct {
		name, origin string
		status       int
	}{
		{"configured subdomain", "https://apply.freehire.me", fiber.StatusOK},
		{"foreign lookalike", "https://apply.freehire.me.evil.example", fiber.StatusForbidden},
		{"unconfigured subdomain", "https://other.freehire.me", fiber.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resp := doLogin(t, sameOriginApp(t, "apply.freehire.me"), map[string]string{fiber.HeaderOrigin: tc.origin})
			defer resp.Body.Close()
			if resp.StatusCode != tc.status {
				t.Fatalf("status = %d, want %d", resp.StatusCode, tc.status)
			}
			if tc.status == fiber.StatusForbidden && len(resp.Cookies()) != 0 {
				t.Fatal("refused login set a cookie")
			}
		})
	}
}

func TestAuthHandlerIncludesConfiguredServedOrigins(t *testing.T) {
	h := newAuthHandlers(nil, nil, auth.NewIssuer("test-secret", time.Hour), false,
		[]string{".freehire.me"}, nil, testFrontendOrigin, nil, []string{"apply.freehire.me"})
	if !originAllowed("https://apply.freehire.me", h.browserOrigins) {
		t.Fatal("configured served origin omitted from handler")
	}
	if originAllowed("https://other.freehire.me", h.browserOrigins) {
		t.Fatal("unconfigured subdomain accepted")
	}
}
