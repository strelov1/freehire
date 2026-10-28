//go:build integration

// Integration tests for the account-recovery HTTP surface: email verification by code,
// forgotten-password reset by code, and password change. All three touch the users table
// and the code store, so they run against a real Postgres. Run with:
// go test -tags=integration ./internal/api/handler/
package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/identity/accounts"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
)

// captureMailer records the codes instead of sending them, so a test can read what the
// user would have received.
type captureMailer struct {
	verification []string
	reset        []string
	// change holds the email-change codes, changeTo the address each went to, and
	// changedNotice the old addresses told the account moved.
	change        []string
	changeTo      []string
	changedNotice []string
}

func (m *captureMailer) SendEmailChangeCode(_ context.Context, email, code string) error {
	m.change = append(m.change, code)
	m.changeTo = append(m.changeTo, email)
	return nil
}

func (m *captureMailer) SendEmailChangedNotice(_ context.Context, email string) error {
	m.changedNotice = append(m.changedNotice, email)
	return nil
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
	svc.WithCodes(accounts.NewQueriesCodeStore(queries, pool), mailer)

	iss := auth.NewIssuer("test-secret", time.Hour)
	h := &authHandlers{queries: queries, issuer: iss, accounts: svc}

	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	cookieAuth := auth.RequireAuth(iss, queries)
	app.Post("/api/v1/auth/register", h.Register)
	app.Post("/api/v1/auth/login", h.Login)
	app.Post("/api/v1/auth/verify/request", cookieAuth, h.RequestEmailVerification)
	app.Post("/api/v1/auth/verify/confirm", cookieAuth, h.ConfirmEmailVerification)
	app.Post("/api/v1/auth/password/forgot", h.ForgotPassword)
	app.Post("/api/v1/auth/password/reset", h.ResetPassword)
	app.Post("/api/v1/me/password", cookieAuth, h.ChangePassword)
	app.Post("/api/v1/me/email", cookieAuth, h.RequestEmailChange)
	app.Post("/api/v1/me/email/confirm", cookieAuth, h.ConfirmEmailChange)
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

// TestResetPasswordDeletesOAuthGrants is a regression/spec test for
// freehire#3114's own wording ("A password reset deletes them"): a reset must
// delete the account's MCP OAuth grant rows outright, not merely bump
// users.token_version and leave the row behind unable to authenticate.
func TestResetPasswordDeletesOAuthGrants(t *testing.T) {
	app, mailer, queries, _ := recoveryApp(t)

	reg := postAuthJSON(t, app, "/api/v1/auth/register",
		`{"email":"grant-holder@example.test","password":"original-pw"}`, "")
	defer reg.Body.Close()
	var created struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(reg.Body).Decode(&created); err != nil {
		t.Fatalf("decode register: %v", err)
	}
	userID := created.Data.ID

	if _, err := queries.RegisterOAuthClient(context.Background(), db.RegisterOAuthClientParams{
		ClientID: "client-reset-test", ClientName: "Test Client",
		RedirectUris: []string{"http://127.0.0.1:9999/cb"},
	}); err != nil {
		t.Fatalf("RegisterOAuthClient: %v", err)
	}
	version, err := queries.GetUserTokenVersion(context.Background(), userID)
	if err != nil {
		t.Fatalf("GetUserTokenVersion: %v", err)
	}
	expiresAt := time.Now().Add(30 * 24 * time.Hour)
	if _, err := queries.CreateOAuthGrant(context.Background(), db.CreateOAuthGrantParams{
		UserID: userID, ClientID: "client-reset-test", AccessTokenHash: "grant-hash-reset-test",
		IssuedTokenVersion: version, ExpiresAt: pgconv.Timestamptz(&expiresAt),
	}); err != nil {
		t.Fatalf("CreateOAuthGrant: %v", err)
	}

	forgot := postAuthJSON(t, app, "/api/v1/auth/password/forgot",
		`{"email":"grant-holder@example.test"}`, "")
	defer forgot.Body.Close()
	deadline := time.Now().Add(5 * time.Second)
	for len(mailer.reset) == 0 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if len(mailer.reset) != 1 {
		t.Fatalf("mailed %d reset codes, want 1", len(mailer.reset))
	}

	reset := postAuthJSON(t, app, "/api/v1/auth/password/reset",
		`{"email":"grant-holder@example.test","code":"`+mailer.reset[0]+`","password":"replacement-pw"}`, "")
	defer reset.Body.Close()
	if reset.StatusCode != fiber.StatusOK {
		t.Fatalf("reset status = %d, want 200", reset.StatusCode)
	}

	rows, err := queries.ListOAuthGrantsByUser(context.Background(), userID)
	if err != nil {
		t.Fatalf("ListOAuthGrantsByUser: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("oauth_grants rows for user = %d, want 0 — a password reset must delete the row, not just revoke it", len(rows))
	}
}

// TestResetPassword_ConcurrentRequestsConsumeTheCodeOnce drives the real ResetPassword
// path — through the HTTP handler, the real QueriesCodeStore, and a real Postgres
// transaction — with the same code presented concurrently. Only one request may consume
// it; this is the guarantee the FOR UPDATE lock exists to make, and it must hold against
// the actual production wiring, not just an in-memory fake.
func TestResetPassword_ConcurrentRequestsConsumeTheCodeOnce(t *testing.T) {
	app, mailer, _, _ := recoveryApp(t)

	reg := postAuthJSON(t, app, "/api/v1/auth/register",
		`{"email":"racer@example.test","password":"original-pw"}`, "")
	defer reg.Body.Close()

	forgot := postAuthJSON(t, app, "/api/v1/auth/password/forgot", `{"email":"racer@example.test"}`, "")
	defer forgot.Body.Close()
	if forgot.StatusCode != fiber.StatusAccepted {
		t.Fatalf("forgot status = %d, want 202", forgot.StatusCode)
	}

	deadline := time.Now().Add(5 * time.Second)
	for len(mailer.reset) == 0 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if len(mailer.reset) != 1 {
		t.Fatalf("mailed %d reset codes, want 1", len(mailer.reset))
	}
	code := mailer.reset[0]

	const attempts = 10
	var successes atomic.Int32
	var wg sync.WaitGroup
	wg.Add(attempts)
	for i := 0; i < attempts; i++ {
		go func(idx int) {
			defer wg.Done()
			body := fmt.Sprintf(`{"email":"racer@example.test","code":"%s","password":"replacement-pw-%d"}`, code, idx)
			resp := postAuthJSON(t, app, "/api/v1/auth/password/reset", body, "")
			defer resp.Body.Close()
			if resp.StatusCode == fiber.StatusOK {
				successes.Add(1)
			}
		}(i)
	}
	wg.Wait()

	if got := successes.Load(); got != 1 {
		t.Errorf("concurrent resets succeeded = %d, want exactly 1", got)
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

// The SPA needs to know whether the account even has a password, so the security page can
// offer the change form to password accounts and explain itself to OAuth-only ones. It
// cannot infer this from being signed in.
func TestMeReportsWhetherTheAccountHasAPassword(t *testing.T) {
	app, _, queries, iss := recoveryApp(t)
	ctx := context.Background()

	reg := postAuthJSON(t, app, "/api/v1/auth/register",
		`{"email":"withpw@example.test","password":"original-pw"}`, "")
	defer reg.Body.Close()
	if got := decodeHasPassword(t, reg); !got {
		t.Error("a password registration must report has_password true")
	}

	// A provider-created account is passwordless.
	oauthUser, err := queries.CreateUser(ctx, db.CreateUserParams{
		Email: "oauthonly@example.test", EmailVerified: true,
	})
	if err != nil {
		t.Fatalf("seed OAuth account: %v", err)
	}
	cookie, _ := iss.Issue(oauthUser.ID, 1)

	req := httptest.NewRequest(fiber.MethodGet, "/api/v1/auth/me", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("me: %v", err)
	}
	defer resp.Body.Close()
	if got := decodeHasPassword(t, resp); got {
		t.Error("a passwordless OAuth account must report has_password false")
	}
}

func decodeHasPassword(t *testing.T, resp *http.Response) bool {
	t.Helper()
	var body struct {
		Data struct {
			HasPassword bool `json:"has_password"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return body.Data.HasPassword
}

// An email change mails a code to the NEW address and moves the account only when that
// code comes back; it then behaves like a credential change (every other session revoked,
// the caller kept signed in) and leaves no code in the OLD mailbox that could reopen the
// account.
func TestChangeEmailMovesTheAccountToTheProvenAddress(t *testing.T) {
	app, mailer, _, iss := recoveryApp(t)

	other := postAuthJSON(t, app, "/api/v1/auth/register",
		`{"email":"taken@example.test","password":"original-pw"}`, "")
	defer other.Body.Close()

	reg := postAuthJSON(t, app, "/api/v1/auth/register",
		`{"email":"mover@example.test","password":"original-pw"}`, "")
	defer reg.Body.Close()
	var created struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(reg.Body).Decode(&created); err != nil {
		t.Fatalf("decode register: %v", err)
	}
	cookie := sessionCookie(t, reg)
	// Prove the current address, so the move has an owner to tell.
	verified := postAuthJSON(t, app, "/api/v1/auth/verify/confirm",
		`{"code":"`+mailer.verification[len(mailer.verification)-1]+`"}`, cookie)
	defer verified.Body.Close()
	if verified.StatusCode != fiber.StatusOK {
		t.Fatalf("verify status = %d, want 200", verified.StatusCode)
	}
	otherDevice, _ := iss.Issue(created.Data.ID, 1)

	for _, tc := range []struct {
		body string
		want int
	}{
		{`{"email":"not-an-address"}`, fiber.StatusBadRequest},
		{`{"email":"MOVER@example.test"}`, fiber.StatusBadRequest},
		{`{"email":"taken@example.test"}`, fiber.StatusConflict},
	} {
		resp := postAuthJSON(t, app, "/api/v1/me/email", tc.body, cookie)
		resp.Body.Close()
		if resp.StatusCode != tc.want {
			t.Errorf("request %s = %d, want %d", tc.body, resp.StatusCode, tc.want)
		}
	}
	if len(mailer.change) != 0 {
		t.Fatalf("a refused request mailed %d codes", len(mailer.change))
	}

	// A reset code waiting in the OLD mailbox, issued before the move.
	forgot := postAuthJSON(t, app, "/api/v1/auth/password/forgot", `{"email":"mover@example.test"}`, "")
	forgot.Body.Close()
	deadline := time.Now().Add(5 * time.Second)
	for len(mailer.reset) == 0 && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
	}
	if len(mailer.reset) != 1 {
		t.Fatalf("mailed %d reset codes, want 1", len(mailer.reset))
	}

	req := postAuthJSON(t, app, "/api/v1/me/email", `{"email":"Moved@Example.test"}`, cookie)
	req.Body.Close()
	if req.StatusCode != fiber.StatusAccepted {
		t.Fatalf("request status = %d, want 202", req.StatusCode)
	}
	if len(mailer.change) != 1 || mailer.changeTo[0] != "moved@example.test" {
		t.Fatalf("change mails = %v, want one to moved@example.test", mailer.changeTo)
	}

	wrong := postAuthJSON(t, app, "/api/v1/me/email/confirm", `{"code":"000000"}`, cookie)
	wrong.Body.Close()
	if wrong.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("wrong code = %d, want 400", wrong.StatusCode)
	}

	confirmed := postAuthJSON(t, app, "/api/v1/me/email/confirm", `{"code":"`+mailer.change[0]+`"}`, cookie)
	defer confirmed.Body.Close()
	if confirmed.StatusCode != fiber.StatusOK {
		t.Fatalf("confirm status = %d, want 200", confirmed.StatusCode)
	}
	var moved struct {
		Data struct {
			Email         string `json:"email"`
			EmailVerified bool   `json:"email_verified"`
		} `json:"data"`
	}
	if err := json.NewDecoder(confirmed.Body).Decode(&moved); err != nil {
		t.Fatalf("decode confirm: %v", err)
	}
	if moved.Data.Email != "moved@example.test" || !moved.Data.EmailVerified {
		t.Errorf("after confirm = %+v, want moved@example.test, verified", moved.Data)
	}
	if len(mailer.changedNotice) != 1 || mailer.changedNotice[0] != "mover@example.test" {
		t.Errorf("old-address notices = %v, want one to mover@example.test", mailer.changedNotice)
	}

	// The caller stays signed in on the re-issued cookie; every other session is gone.
	fresh := sessionCookie(t, confirmed)
	still := postAuthJSON(t, app, "/api/v1/me/email", `{"email":"moved@example.test"}`, fresh)
	still.Body.Close()
	if still.StatusCode == fiber.StatusUnauthorized {
		t.Error("the caller was signed out by their own email change")
	}
	gone := postAuthJSON(t, app, "/api/v1/me/email", `{"email":"x@example.test"}`, otherDevice)
	gone.Body.Close()
	if gone.StatusCode != fiber.StatusUnauthorized {
		t.Errorf("other device status = %d, want 401 after an email change", gone.StatusCode)
	}

	// The reset code in the old mailbox no longer opens the account at its new address.
	reset := postAuthJSON(t, app, "/api/v1/auth/password/reset",
		`{"email":"moved@example.test","code":"`+mailer.reset[0]+`","password":"hijacked-pw"}`, "")
	reset.Body.Close()
	if reset.StatusCode != fiber.StatusBadRequest {
		t.Errorf("old-mailbox reset code = %d, want 400", reset.StatusCode)
	}

	// The account answers to the new address only.
	newLogin := postAuthJSON(t, app, "/api/v1/auth/login", `{"email":"moved@example.test","password":"original-pw"}`, "")
	newLogin.Body.Close()
	if newLogin.StatusCode != fiber.StatusOK {
		t.Errorf("login at the new address = %d, want 200", newLogin.StatusCode)
	}
	oldLogin := postAuthJSON(t, app, "/api/v1/auth/login", `{"email":"mover@example.test","password":"original-pw"}`, "")
	oldLogin.Body.Close()
	if oldLogin.StatusCode != fiber.StatusUnauthorized {
		t.Errorf("login at the old address = %d, want 401", oldLogin.StatusCode)
	}
}
