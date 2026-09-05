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

	"github.com/strelov1/freehire/internal/identity/accounts"
	"github.com/strelov1/freehire/internal/identity/auth"
)

// fakeRepo is an in-memory accounts.Repository for handler tests. The register
// validation cases below all reject inside the service before any repo method
// runs, so these methods are never reached by those tests.
type fakeRepo struct{}

func (f fakeRepo) WithTx(pgx.Tx) accounts.Repository { return f }

func (fakeRepo) UserIDByIdentity(context.Context, string, string) (int64, error) {
	return 0, accounts.ErrIdentityNotFound
}
func (fakeRepo) LinkOrCreateByEmail(context.Context, string, string, string) (int64, error) {
	return 0, nil
}
func (fakeRepo) CreateUser(context.Context, string, string, bool, *string) (accounts.User, error) {
	return accounts.User{}, nil
}
func (fakeRepo) UpdateTimezone(context.Context, int64, *string) (accounts.User, error) {
	return accounts.User{}, nil
}
func (fakeRepo) UpdateLanguage(context.Context, int64, string) (accounts.User, error) {
	return accounts.User{}, nil
}
func (fakeRepo) MarkEmailVerified(context.Context, int64) error { return nil }
func (fakeRepo) PasswordHash(context.Context, int64) (string, bool, error) {
	return "", false, nil
}
func (fakeRepo) SetPassword(context.Context, int64, string) (int32, error) {
	return 0, nil
}
func (fakeRepo) ResetPassword(context.Context, int64, string) (int32, error) {
	return 0, nil
}
func (fakeRepo) UserByEmail(context.Context, string) (accounts.User, string, bool, error) {
	return accounts.User{}, "", false, accounts.ErrUserNotFound
}
func (fakeRepo) UserByID(context.Context, int64) (accounts.User, error) {
	return accounts.User{}, accounts.ErrUserNotFound
}
func (fakeRepo) UsernameByUser(context.Context, int64) (string, *time.Time, bool, error) {
	return "", nil, false, nil
}
func (fakeRepo) SetUsernameIfAbsent(context.Context, int64, string) error { return nil }
func (fakeRepo) SetUsername(context.Context, int64, string) error         { return nil }
func (fakeRepo) UsernameTaken(context.Context, string) (bool, error)      { return false, nil }

// registerApp mounts only the register route on a handler whose accounts service
// is backed by an in-memory repo. The validation cases below all reject inside
// the service (invalid email / short password) or at body-parse, so the repo's
// write methods are never reached.
func registerApp() *fiber.App {
	app := fiber.New()
	h := &authHandlers{
		issuer:   auth.NewIssuer("test-secret", time.Hour),
		accounts: accounts.New(fakeRepo{}, authHasher{}),
	}
	app.Post("/register", h.Register)
	return app
}

func postJSON(t *testing.T, app *fiber.App, path, body string) int {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPost, path, strings.NewReader(body))
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

func TestRegister_RejectsShortPassword(t *testing.T) {
	if got := postJSON(t, registerApp(), "/register", `{"email":"a@b.com","password":"short"}`); got != fiber.StatusBadRequest {
		t.Errorf("status = %d, want 400", got)
	}
}

func TestRegister_RejectsInvalidEmail(t *testing.T) {
	if got := postJSON(t, registerApp(), "/register", `{"email":"not-an-email","password":"longenough123"}`); got != fiber.StatusBadRequest {
		t.Errorf("status = %d, want 400", got)
	}
}

func TestRegister_RejectsMalformedBody(t *testing.T) {
	if got := postJSON(t, registerApp(), "/register", `{not json`); got != fiber.StatusBadRequest {
		t.Errorf("status = %d, want 400", got)
	}
}

// userResponse is the only user shape that reaches a response. This locks the
// contract that it never carries the password hash.
func TestUserResponse_OmitsPasswordHash(t *testing.T) {
	raw, err := json.Marshal(userResponse{ID: 1, Email: "a@b.com"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, leaked := fields["password_hash"]; leaked {
		t.Error("userResponse must not include password_hash")
	}
	for _, want := range []string{"id", "email", "created_at"} {
		if _, ok := fields[want]; !ok {
			t.Errorf("userResponse missing %q", want)
		}
	}
}

func TestAuthRoutes_NoCacheHeaders(t *testing.T) {
	app := fiber.New()
	api := app.Group("/api/v1")
	h := &authHandlers{
		issuer:   auth.NewIssuer("test-secret", time.Hour),
		accounts: accounts.New(fakeRepo{}, authHasher{}),
	}
	h.register(api, middleware{
		cookie: namedGate("cookie"),
		cvKey:  namedGate("cvKey"),
	})

	routes := []struct {
		method string
		path   string
	}{
		{fiber.MethodPost, "/api/v1/auth/login"},
		{fiber.MethodPost, "/api/v1/auth/register"},
		{fiber.MethodPost, "/api/v1/auth/logout"},
		{fiber.MethodGet, "/api/v1/auth/me"},
		{fiber.MethodGet, "/api/v1/me/api-keys"},
		{fiber.MethodPost, "/api/v1/me/password"},
		{fiber.MethodDelete, "/api/v1/me"},
	}

	for _, r := range routes {
		t.Run(r.method+" "+r.path, func(t *testing.T) {
			req := httptest.NewRequestWithContext(context.Background(), r.method, r.path, nil)
			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("Test: %v", err)
			}
			defer resp.Body.Close()

			if cc := resp.Header.Get("Cache-Control"); cc != "no-store" {
				t.Errorf("Cache-Control = %q, want %q", cc, "no-store")
			}
			if pragma := resp.Header.Get("Pragma"); pragma != "no-cache" {
				t.Errorf("Pragma = %q, want %q", pragma, "no-cache")
			}
		})
	}
}

type fakeRepoWithErr struct {
	fakeRepo
	err error
}

func (f fakeRepoWithErr) CreateUser(context.Context, string, string, bool, *string) (accounts.User, error) {
	return accounts.User{}, f.err
}

func (f fakeRepoWithErr) UserByEmail(context.Context, string) (accounts.User, string, bool, error) {
	return accounts.User{}, "", false, f.err
}

func TestRegister_DuplicateEmail(t *testing.T) {
	app := fiber.New()
	h := &authHandlers{
		issuer:   auth.NewIssuer("test-secret", time.Hour),
		accounts: accounts.New(fakeRepoWithErr{err: accounts.ErrEmailTaken}, authHasher{}),
	}
	app.Post("/register", h.Register)

	if got := postJSON(t, app, "/register", `{"email":"taken@b.com","password":"validpassword123"}`); got != fiber.StatusConflict {
		t.Errorf("status = %d, want 409 Conflict", got)
	}
}

func TestRegister_DBDown(t *testing.T) {
	app := fiber.New()
	h := &authHandlers{
		issuer:   auth.NewIssuer("test-secret", time.Hour),
		accounts: accounts.New(fakeRepoWithErr{err: errors.New("pgx: connection refused timeout")}, authHasher{}),
	}
	app.Post("/register", h.Register)

	if got := postJSON(t, app, "/register", `{"email":"a@b.com","password":"validpassword123"}`); got != fiber.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503 Service Unavailable on DB outage", got)
	}
}

func TestLogin_WrongPassword(t *testing.T) {
	app := fiber.New()
	h := &authHandlers{
		issuer:   auth.NewIssuer("test-secret", time.Hour),
		accounts: accounts.New(fakeRepoWithErr{err: accounts.ErrInvalidCredentials}, authHasher{}),
	}
	app.Post("/login", h.Login)

	if got := postJSON(t, app, "/login", `{"email":"a@b.com","password":"wrongpassword"}`); got != fiber.StatusUnauthorized {
		t.Errorf("status = %d, want 401 Unauthorized", got)
	}
}

func TestLogin_DBDown(t *testing.T) {
	app := fiber.New()
	h := &authHandlers{
		issuer:   auth.NewIssuer("test-secret", time.Hour),
		accounts: accounts.New(fakeRepoWithErr{err: errors.New("pgx: connection refusal or pool timeout")}, authHasher{}),
	}
	app.Post("/login", h.Login)

	if got := postJSON(t, app, "/login", `{"email":"a@b.com","password":"validpassword123"}`); got != fiber.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503 Service Unavailable on DB outage", got)
	}
}

type fakeRepoTimezone struct {
	fakeRepo
	user accounts.User
	err  error
}

func (f fakeRepoTimezone) UpdateTimezone(context.Context, int64, *string) (accounts.User, error) {
	return f.user, f.err
}

func timezoneApp(repo accounts.Repository) (*fiber.App, *auth.Issuer) {
	iss := auth.NewIssuer("test-secret", time.Hour)
	h := &authHandlers{issuer: iss, accounts: accounts.New(repo, authHasher{})}
	app := fiber.New()
	app.Patch("/me/timezone", auth.RequireAuth(iss, testVersions), h.UpdateTimezone)
	return app, iss
}

func patchJSON(t *testing.T, app *fiber.App, path, token, body string) int {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodPatch, path, strings.NewReader(body))
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	if token != "" {
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

func TestUpdateTimezone_RequiresAuth(t *testing.T) {
	app, _ := timezoneApp(fakeRepoTimezone{})
	if got := patchJSON(t, app, "/me/timezone", "", `{"timezone":"Europe/Moscow"}`); got != fiber.StatusUnauthorized {
		t.Errorf("status = %d, want 401", got)
	}
}

func TestUpdateTimezone_Valid(t *testing.T) {
	app, iss := timezoneApp(fakeRepoTimezone{user: accounts.User{ID: 7}})
	token, _ := iss.Issue(7, testTokenVersion)
	if got := patchJSON(t, app, "/me/timezone", token, `{"timezone":"Europe/Moscow"}`); got != fiber.StatusOK {
		t.Errorf("status = %d, want 200", got)
	}
}

func TestUpdateTimezone_RejectsInvalid(t *testing.T) {
	app, iss := timezoneApp(fakeRepoTimezone{err: accounts.ErrInvalidTimezone})
	token, _ := iss.Issue(7, testTokenVersion)
	if got := patchJSON(t, app, "/me/timezone", token, `{"timezone":"Not/AZone"}`); got != fiber.StatusBadRequest {
		t.Errorf("status = %d, want 400", got)
	}
}

type fakeRepoLanguage struct {
	fakeRepo
	user accounts.User
	err  error
}

func (f fakeRepoLanguage) UpdateLanguage(context.Context, int64, string) (accounts.User, error) {
	return f.user, f.err
}

func languageApp(repo accounts.Repository) (*fiber.App, *auth.Issuer) {
	iss := auth.NewIssuer("test-secret", time.Hour)
	h := &authHandlers{issuer: iss, accounts: accounts.New(repo, authHasher{})}
	app := fiber.New()
	app.Patch("/me/language", auth.RequireAuth(iss, testVersions), h.UpdateLanguage)
	return app, iss
}

func TestUpdateLanguage_RequiresAuth(t *testing.T) {
	app, _ := languageApp(fakeRepoLanguage{})
	if got := patchJSON(t, app, "/me/language", "", `{"language":"ru"}`); got != fiber.StatusUnauthorized {
		t.Errorf("status = %d, want 401", got)
	}
}

func TestUpdateLanguage_Valid(t *testing.T) {
	app, iss := languageApp(fakeRepoLanguage{user: accounts.User{ID: 7, Language: "ru"}})
	token, _ := iss.Issue(7, testTokenVersion)
	if got := patchJSON(t, app, "/me/language", token, `{"language":"ru"}`); got != fiber.StatusOK {
		t.Errorf("status = %d, want 200", got)
	}
}

func TestUpdateLanguage_RejectsInvalid(t *testing.T) {
	app, iss := languageApp(fakeRepoLanguage{err: accounts.ErrInvalidLanguage})
	token, _ := iss.Issue(7, testTokenVersion)
	if got := patchJSON(t, app, "/me/language", token, `{"language":"xx"}`); got != fiber.StatusBadRequest {
		t.Errorf("status = %d, want 400", got)
	}
}
