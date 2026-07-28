package handler

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"context"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/accounts"
	"github.com/strelov1/freehire/internal/auth"
)

// fakeRepo is an in-memory accounts.Repository for handler tests. The register
// validation cases below all reject inside the service before any repo method
// runs, so these methods are never reached by those tests.
type fakeRepo struct{}

func (fakeRepo) UserIDByIdentity(context.Context, string, string) (int64, error) {
	return 0, accounts.ErrIdentityNotFound
}
func (fakeRepo) LinkOrCreateByEmail(context.Context, string, string, string) (int64, error) {
	return 0, nil
}
func (fakeRepo) CreateUser(context.Context, string, string, bool) (accounts.User, error) {
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
	req := httptest.NewRequest(fiber.MethodPost, path, strings.NewReader(body))
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationJSON)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
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
