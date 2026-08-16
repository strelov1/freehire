package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/auth"
)

// stubValidKey authenticates exactly one full-scope key; any other hash is unknown,
// mirroring the real db layer's ErrNoRows. Mirrors internal/auth's own fakeKeyAuth.
type stubValidKey struct {
	hash   string
	userID int64
}

func (s stubValidKey) AuthenticateAPIKey(_ context.Context, hash string) (auth.APIKeyIdentity, error) {
	if hash == s.hash {
		return auth.APIKeyIdentity{UserID: s.userID, Scope: auth.ScopeFull}, nil
	}
	return auth.APIKeyIdentity{}, pgx.ErrNoRows
}

func TestProfileFields_IncludesScreeningAnswers(t *testing.T) {
	p := autofillProfile{
		FullName:      "Ilya Strelov",
		NoticePeriod:  "30 days",
		DesiredSalary: "120000 USD/year",
	}
	got := profileFields(p)

	if got["notice_period"] != "30 days" {
		t.Errorf(`profileFields["notice_period"] = %q, want "30 days"`, got["notice_period"])
	}
	if got["desired_salary"] != "120000 USD/year" {
		t.Errorf(`profileFields["desired_salary"] = %q, want "120000 USD/year"`, got["desired_salary"])
	}
	// The existing identity fields must not regress.
	if got["full_name"] != "Ilya Strelov" {
		t.Errorf(`profileFields["full_name"] = %q, want "Ilya Strelov"`, got["full_name"])
	}
}

// autofillRunApp mounts /me/autofill/run behind the same RequireAuthOrKey mw.key uses in
// production, on a handler with no DB. The refusal cases below reject before any query
// runs, so the nil autofillHandlers fields are never dereferenced.
func autofillRunApp(iss *auth.Issuer, keys auth.APIKeyAuthenticator) *fiber.App {
	app := fiber.New()
	h := &autofillHandlers{}
	app.Post("/api/v1/me/autofill/run", auth.RequireAuthOrKey(iss, testVersions, keys), h.RunAgentAutofill)
	return app
}

// RunAgentAutofill attaches to the caller's browsertools.Hub channel unconditionally —
// the same channel read_current_page reaches, but this one WRITES into whatever form the
// caller's extension currently has open. Hub is keyed by user id, not session id, so
// without this guard a website user (cookie) or a full-scope API key could trigger it
// against a page they never opened on this surface. See the
// confine-browse-preset-to-extension change.
func TestRunAgentAutofillRefusesNonExtensionAuth(t *testing.T) {
	iss := auth.NewIssuer("test-secret", time.Hour)
	token, err := iss.Issue(7, testTokenVersion)
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	const apiKey = "fhk_test-key"
	keys := stubValidKey{hash: auth.HashAPIKey(apiKey), userID: 7}

	cases := []struct {
		name   string
		cookie bool
		bearer string // "" = no Authorization header
	}{
		{"website cookie", true, ""},
		{"API key", false, apiKey},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, "/api/v1/me/autofill/run", nil)
			if tc.cookie {
				req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
			}
			if tc.bearer != "" {
				req.Header.Set("Authorization", "Bearer "+tc.bearer)
			}
			resp, err := autofillRunApp(iss, keys).Test(req)
			if err != nil {
				t.Fatalf("Test: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != fiber.StatusForbidden {
				t.Errorf("status = %d, want 403 (autofill must run only from the extension's own connection)", resp.StatusCode)
			}
		})
	}
}
