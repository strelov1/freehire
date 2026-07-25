//go:build integration

// Integration tests for the account-recovery HTTP surface: email verification by code,
// forgotten-password reset by code, and password change. All three touch the users table
// and the code store, so they run against a real Postgres. Run with:
// go test -tags=integration ./internal/handler/
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/accounts"
	"github.com/strelov1/freehire/internal/auth"
	"github.com/strelov1/freehire/internal/db"
)

// captureMailer records the codes instead of sending them, so a test can read what the
// user would have received.
type captureMailer struct {
	verification []string
	reset        []string
}

func (m *captureMailer) SendVerificationCode(_ context.Context, _, code string) error {
	m.verification = append(m.verification, code)
	return nil
}

func (m *captureMailer) SendPasswordResetCode(_ context.Context, _, code string) error {
	m.reset = append(m.reset, code)
	return nil
}

// recoveryApp wires the real accounts service (bcrypt, real repository, real code store)
// onto the recovery routes.
func recoveryApp(t *testing.T) (*fiber.App, *captureMailer, *db.Queries, *auth.Issuer) {
	t.Helper()
	pool := startPostgres(t)
	queries := db.New(pool)
	mailer := &captureMailer{}

	svc := accounts.New(accounts.NewQueriesRepository(queries, pool), authHasher{})
	svc.WithCodes(accounts.NewQueriesCodeStore(queries), mailer)

	iss := auth.NewIssuer("test-secret", time.Hour)
	h := &API{pool: pool, queries: queries, issuer: iss, accounts: svc}

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	cookieAuth := auth.RequireAuth(iss, queries)
	app.Post("/api/v1/auth/register", h.Register)
	app.Post("/api/v1/auth/login", h.Login)
	app.Post("/api/v1/auth/verify/request", cookieAuth, h.RequestEmailVerification)
	app.Post("/api/v1/auth/verify/confirm", cookieAuth, h.ConfirmEmailVerification)
	app.Post("/api/v1/auth/password/forgot", h.ForgotPassword)
	app.Post("/api/v1/auth/password/reset", h.ResetPassword)
	app.Post("/api/v1/me/password", cookieAuth, h.ChangePassword)
	app.Get("/api/v1/auth/me", cookieAuth, h.Me)
	return app, mailer, queries, iss
}

// postAuthJSON sends a JSON body, optionally with a session cookie, and returns the response.
func postAuthJSON(t *testing.T, app *fiber.App, path, body, cookie string) *http.Response {
	t.Helper()
	req := httptest.NewRequest(fiber.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
	}
	resp, err := app.Test(req, 10_000)
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return resp
}

// sessionCookie pulls the auth cookie out of a Set-Cookie response.
func sessionCookie(t *testing.T, resp *http.Response) string {
	t.Helper()
	for _, c := range resp.Cookies() {
		if c.Name == auth.CookieName && c.Value != "" {
			return c.Value
		}
	}
	t.Fatal("response carried no session cookie")
	return ""
}

func TestRegisterThenVerifyEmail(t *testing.T) {
	app, mailer, queries, _ := recoveryApp(t)

	resp := postAuthJSON(t, app, "/api/v1/auth/register",
		`{"email":"newbie@example.test","password":"password123"}`, "")
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusCreated {
		t.Fatalf("register status = %d, want 201", resp.StatusCode)
	}
	cookie := sessionCookie(t, resp)

	var created struct {
		Data struct {
			ID            int64 `json:"id"`
			EmailVerified bool  `json:"email_verified"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode register: %v", err)
	}
	if created.Data.EmailVerified {
		t.Error("a fresh registration must report the address as unverified")
	}
	if len(mailer.verification) != 1 {
		t.Fatalf("mailed %d verification codes, want 1", len(mailer.verification))
	}

	// A wrong code changes nothing.
	wrong := postAuthJSON(t, app, "/api/v1/auth/verify/confirm", `{"code":"000000"}`, cookie)
	defer wrong.Body.Close()
	if wrong.StatusCode != fiber.StatusBadRequest {
		t.Errorf("wrong-code status = %d, want 400", wrong.StatusCode)
	}

	confirm := postAuthJSON(t, app, "/api/v1/auth/verify/confirm",
		`{"code":"`+mailer.verification[0]+`"}`, cookie)
	defer confirm.Body.Close()
	if confirm.StatusCode != fiber.StatusOK {
		t.Fatalf("confirm status = %d, want 200", confirm.StatusCode)
	}

	user, err := queries.GetUserByID(context.Background(), created.Data.ID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if !user.EmailVerified {
		t.Error("the account is still unverified after a correct code")
	}
}

func TestForgotPasswordIsSilentAboutUnknownAddresses(t *testing.T) {
	app, mailer, _, _ := recoveryApp(t)

	resp := postAuthJSON(t, app, "/api/v1/auth/password/forgot", `{"email":"ghost@example.test"}`, "")
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusAccepted {
		t.Errorf("status = %d, want 202 for an unknown address", resp.StatusCode)
	}
	// The send runs in the background; give it a moment before asserting nothing was sent.
	time.Sleep(200 * time.Millisecond)
	if len(mailer.reset) != 0 {
		t.Error("a reset code was mailed for an address with no account")
	}
}

func TestForgotThenResetPassword(t *testing.T) {
	app, mailer, _, _ := recoveryApp(t)

	reg := postAuthJSON(t, app, "/api/v1/auth/register",
		`{"email":"forgetful@example.test","password":"original-pw"}`, "")
	defer reg.Body.Close()
	oldCookie := sessionCookie(t, reg)

	forgot := postAuthJSON(t, app, "/api/v1/auth/password/forgot",
		`{"email":"forgetful@example.test"}`, "")
	defer forgot.Body.Close()
	if forgot.StatusCode != fiber.StatusAccepted {
		t.Fatalf("forgot status = %d, want 202", forgot.StatusCode)
	}

	// The mail is sent off the response path, so wait for it.
	deadline := time.Now().Add(5 * time.Second)
	for len(mailer.reset) == 0 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if len(mailer.reset) != 1 {
		t.Fatalf("mailed %d reset codes, want 1", len(mailer.reset))
	}

	reset := postAuthJSON(t, app, "/api/v1/auth/password/reset",
		`{"email":"forgetful@example.test","code":"`+mailer.reset[0]+`","password":"replacement-pw"}`, "")
	defer reset.Body.Close()
	if reset.StatusCode != fiber.StatusOK {
		t.Fatalf("reset status = %d, want 200", reset.StatusCode)
	}

	// The old session is gone: a reset is exactly when a user suspects an intruder.
	me := postAuthJSON(t, app, "/api/v1/auth/verify/request", `{}`, oldCookie)
	defer me.Body.Close()
	if me.StatusCode != fiber.StatusUnauthorized {
		t.Errorf("pre-reset session status = %d, want 401 — the reset must revoke sessions", me.StatusCode)
	}

	// The new password works and the old one does not.
	ok := postAuthJSON(t, app, "/api/v1/auth/login",
		`{"email":"forgetful@example.test","password":"replacement-pw"}`, "")
	defer ok.Body.Close()
	if ok.StatusCode != fiber.StatusOK {
		t.Errorf("login with the new password = %d, want 200", ok.StatusCode)
	}
	stale := postAuthJSON(t, app, "/api/v1/auth/login",
		`{"email":"forgetful@example.test","password":"original-pw"}`, "")
	defer stale.Body.Close()
	if stale.StatusCode != fiber.StatusUnauthorized {
		t.Errorf("login with the old password = %d, want 401", stale.StatusCode)
	}
}

func TestChangePasswordKeepsTheCallerSignedInAndRevokesOthers(t *testing.T) {
	app, _, _, iss := recoveryApp(t)

	reg := postAuthJSON(t, app, "/api/v1/auth/register",
		`{"email":"changer@example.test","password":"original-pw"}`, "")
	defer reg.Body.Close()
	var created struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(reg.Body).Decode(&created); err != nil {
		t.Fatalf("decode register: %v", err)
	}
	callerCookie := sessionCookie(t, reg)
	otherDevice, _ := iss.Issue(created.Data.ID, 1)

	wrong := postAuthJSON(t, app, "/api/v1/me/password",
		`{"current_password":"not-it","password":"replacement-pw"}`, callerCookie)
	defer wrong.Body.Close()
	if wrong.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("wrong current password = %d, want 401", wrong.StatusCode)
	}

	changed := postAuthJSON(t, app, "/api/v1/me/password",
		`{"current_password":"original-pw","password":"replacement-pw"}`, callerCookie)
	defer changed.Body.Close()
	if changed.StatusCode != fiber.StatusOK {
		t.Fatalf("change status = %d, want 200", changed.StatusCode)
	}
	fresh := sessionCookie(t, changed)

	// The caller keeps working on the re-issued cookie...
	still := postAuthJSON(t, app, "/api/v1/auth/verify/request", `{}`, fresh)
	defer still.Body.Close()
	if still.StatusCode == fiber.StatusUnauthorized {
		t.Error("the caller was signed out by their own password change")
	}
	// ...while the other device is not.
	other := postAuthJSON(t, app, "/api/v1/auth/verify/request", `{}`, otherDevice)
	defer other.Body.Close()
	if other.StatusCode != fiber.StatusUnauthorized {
		t.Errorf("other device status = %d, want 401 after a password change", other.StatusCode)
	}
}
