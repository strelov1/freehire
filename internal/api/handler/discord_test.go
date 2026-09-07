package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/engage/discordlink"
	"github.com/strelov1/freehire/internal/identity/auth"
)

// The package's shared test signing secret, defined in gmail_callback_test.go. Reused
// rather than given a sibling: .gitleaks.toml allowlists that exact literal, so a second
// one of the same shape is a finding — and a growing list of "secrets we promise are fake"
// is the wrong direction for that file.
const testDiscordSecret = testGmailSecret

// fakeDiscordLinker stands in for the service. What these tests are about is the HTTP
// contract — which status, which redirect, which marker — not the linking itself, which is
// covered in internal/engage/discordlink.
type fakeDiscordLinker struct {
	link      discordlink.Link
	linkErr   error
	statusErr error
	unlinked  bool
	linkCalls int
}

func (f *fakeDiscordLinker) Link(context.Context, int64, string, string) (discordlink.Link, error) {
	f.linkCalls++
	if f.linkErr != nil {
		return discordlink.Link{}, f.linkErr
	}
	return f.link, nil
}

func (f *fakeDiscordLinker) Unlink(context.Context, int64) error {
	f.unlinked = true
	return nil
}

func (f *fakeDiscordLinker) Status(context.Context, int64) (discordlink.Link, error) {
	if f.statusErr != nil {
		return discordlink.Link{}, f.statusErr
	}
	return f.link, nil
}

// newDiscordApp mounts the real route table, so the middleware each route is actually
// served behind is what these tests exercise.
func newDiscordApp(t *testing.T, svc DiscordLinker) (*fiber.App, string) {
	t.Helper()
	const frontend = "https://freehire.me"
	h := newDiscordHandlers(svc, "app-1", frontend, true)

	iss := auth.NewIssuer(testDiscordSecret, time.Hour)
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	h.register(app.Group("/api/v1"), middleware{
		cookie:         auth.RequireAuth(iss, testVersions),
		optionalCookie: auth.OptionalCookieAuth(iss, testVersions),
		key:            auth.RequireAuthOrKey(iss, testVersions, nil),
	})
	return app, frontend
}

func discordSession(t *testing.T) string {
	t.Helper()
	token, err := auth.NewIssuer(testDiscordSecret, time.Hour).Issue(1, testTokenVersion)
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	return token
}

func TestDiscordConnectRedirectsToConsent(t *testing.T) {
	app, _ := newDiscordApp(t, &fakeDiscordLinker{})

	r := httptest.NewRequestWithContext(context.Background(), "GET", "/api/v1/me/discord/connect", nil)
	r.Header.Set("Cookie", auth.CookieName+"="+discordSession(t))
	resp, err := app.Test(r, -1)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != fiber.StatusFound {
		t.Fatalf("status = %d, want a redirect", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	for _, want := range []string{"discord.com", "client_id=app-1", "guilds.join", "state="} {
		if !strings.Contains(loc, want) {
			t.Errorf("consent URL %q is missing %q", loc, want)
		}
	}
	// The state must be stored for the callback to compare against, or the CSRF check has
	// nothing to check.
	if len(resp.Cookies()) == 0 {
		t.Error("no state cookie was set")
	}
}

// An API key cannot complete a browser redirect, so the connect route is cookie-only. This
// is the same rule key management and password change follow: a link changes what an
// account IS, not what it may read.
func TestDiscordConnectRefusesAnUnauthenticatedCaller(t *testing.T) {
	app, _ := newDiscordApp(t, &fakeDiscordLinker{})

	resp, err := app.Test(httptest.NewRequestWithContext(context.Background(), "GET",
		"/api/v1/me/discord/connect", nil), -1)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
}

// The callback is a browser returning from Discord, not an XHR. Every refusal has to land
// back on Integrations with a marker; a JSON error would be rendered into the address bar
// and strand the user with no way forward. This is the trap the Gmail callback already fell
// into once — see its own test.
func TestDiscordCallbackRefusalsRedirectInsteadOfJSON(t *testing.T) {
	for _, tc := range []struct {
		name    string
		session bool
		state   string
		query   string
		linkErr error
		want    string
	}{
		{name: "no session", state: "abc", query: "state=abc&code=xyz", want: "discord_error=auth"},
		{name: "state mismatch", session: true, state: "abc", query: "state=nope&code=xyz", want: "discord_error=state"},
		{name: "no state cookie", session: true, query: "state=abc&code=xyz", want: "discord_error=state"},
		{name: "consent declined", session: true, state: "abc", query: "state=abc&error=access_denied", want: "discord_error=denied"},
		{name: "no code", session: true, state: "abc", query: "state=abc", want: "discord_error=exchange"},
		{
			name: "already linked elsewhere", session: true, state: "abc", query: "state=abc&code=xyz",
			linkErr: discordlink.ErrAlreadyLinkedElsewhere, want: "discord_error=taken",
		},
		{
			name: "discord is down", session: true, state: "abc", query: "state=abc&code=xyz",
			linkErr: errors.New("boom"), want: "discord_error=exchange",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc := &fakeDiscordLinker{linkErr: tc.linkErr}
			app, frontend := newDiscordApp(t, svc)

			r := httptest.NewRequestWithContext(context.Background(), "GET",
				"/api/v1/me/discord/callback?"+tc.query, nil)
			cookie := ""
			if tc.session {
				cookie = auth.CookieName + "=" + discordSession(t)
			}
			if tc.state != "" {
				if cookie != "" {
					cookie += "; "
				}
				cookie += discordStateCookieName + "=" + tc.state
			}
			if cookie != "" {
				r.Header.Set("Cookie", cookie)
			}

			resp, err := app.Test(r, -1)
			if err != nil {
				t.Fatalf("callback: %v", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != fiber.StatusFound {
				t.Fatalf("status = %d, want a redirect", resp.StatusCode)
			}
			if got, want := resp.Header.Get("Location"), frontend+"/my/integrations?"+tc.want; got != want {
				t.Errorf("Location = %q, want %q", got, want)
			}
		})
	}
}

// A refused state must not reach the service at all: the whole point of the check is that
// the code in the URL may have been planted by somebody else.
func TestDiscordCallbackDoesNotLinkOnABadState(t *testing.T) {
	svc := &fakeDiscordLinker{}
	app, _ := newDiscordApp(t, svc)

	r := httptest.NewRequestWithContext(context.Background(), "GET",
		"/api/v1/me/discord/callback?state=nope&code=xyz", nil)
	r.Header.Set("Cookie", auth.CookieName+"="+discordSession(t)+"; "+discordStateCookieName+"=abc")
	resp, err := app.Test(r, -1)
	if err != nil {
		t.Fatalf("callback: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if svc.linkCalls != 0 {
		t.Errorf("the service was called %d times on a mismatched state", svc.linkCalls)
	}
}

func TestDiscordCallbackSuccessRedirectsToIntegrations(t *testing.T) {
	svc := &fakeDiscordLinker{link: discordlink.Link{UserID: 1, DiscordUserID: "1", RoleGranted: true}}
	app, frontend := newDiscordApp(t, svc)

	r := httptest.NewRequestWithContext(context.Background(), "GET",
		"/api/v1/me/discord/callback?state=abc&code=xyz", nil)
	r.Header.Set("Cookie", auth.CookieName+"="+discordSession(t)+"; "+discordStateCookieName+"=abc")
	resp, err := app.Test(r, -1)
	if err != nil {
		t.Fatalf("callback: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if got, want := resp.Header.Get("Location"), frontend+"/my/integrations?discord=connected"; got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}
}

func TestDiscordStatus(t *testing.T) {
	for _, tc := range []struct {
		name       string
		svc        *fakeDiscordLinker
		wantLinked bool
	}{
		{"linked", &fakeDiscordLinker{link: discordlink.Link{DiscordUserID: "1", RoleGranted: true}}, true},
		{"not linked", &fakeDiscordLinker{statusErr: discordlink.ErrNotLinked}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, _ := newDiscordApp(t, tc.svc)

			r := httptest.NewRequestWithContext(context.Background(), "GET", "/api/v1/me/discord", nil)
			r.Header.Set("Cookie", auth.CookieName+"="+discordSession(t))
			resp, err := app.Test(r, -1)
			if err != nil {
				t.Fatalf("status: %v", err)
			}
			defer func() { _ = resp.Body.Close() }()

			var body struct {
				Data struct {
					Enabled bool `json:"enabled"`
					Linked  bool `json:"linked"`
				} `json:"data"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if !body.Data.Enabled {
				t.Error("a mounted route must report the feature as enabled")
			}
			if body.Data.Linked != tc.wantLinked {
				t.Errorf("linked = %v, want %v", body.Data.Linked, tc.wantLinked)
			}
		})
	}
}

func TestDiscordUnlink(t *testing.T) {
	svc := &fakeDiscordLinker{link: discordlink.Link{DiscordUserID: "1"}}
	app, _ := newDiscordApp(t, svc)

	r := httptest.NewRequestWithContext(context.Background(), "DELETE", "/api/v1/me/discord", nil)
	r.Header.Set("Cookie", auth.CookieName+"="+discordSession(t))
	resp, err := app.Test(r, -1)
	if err != nil {
		t.Fatalf("unlink: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != fiber.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if !svc.unlinked {
		t.Error("the service was not asked to unlink")
	}
}

// Without credentials the routes are not mounted at all, so they 404 rather than reporting
// a feature that half exists. This is what lets the change deploy before the Discord
// application is created, and what rolling it back looks like.
func TestDiscordRoutesAreAbsentWhenUnconfigured(t *testing.T) {
	iss := auth.NewIssuer(testDiscordSecret, time.Hour)
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	// A nil service is how the composition root expresses "not configured".
	newDiscordHandlers(nil, "", "https://freehire.me", true).register(app.Group("/api/v1"), middleware{
		cookie:         auth.RequireAuth(iss, testVersions),
		optionalCookie: auth.OptionalCookieAuth(iss, testVersions),
		key:            auth.RequireAuthOrKey(iss, testVersions, nil),
	})

	for _, path := range []string{"/api/v1/me/discord", "/api/v1/me/discord/connect", "/api/v1/me/discord/callback"} {
		r := httptest.NewRequestWithContext(context.Background(), "GET", path, nil)
		r.Header.Set("Cookie", auth.CookieName+"="+discordSession(t))
		resp, err := app.Test(r, -1)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if resp.StatusCode != fiber.StatusNotFound {
			t.Errorf("%s status = %d, want 404", path, resp.StatusCode)
		}
		_ = resp.Body.Close()
	}
}
