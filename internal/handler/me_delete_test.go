package handler

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/accountdelete"
	"github.com/strelov1/freehire/internal/auth"
)

// fakeEraser stands in for the account-deletion service.
type fakeEraser struct {
	deleted []int64
	err     error
}

func (f *fakeEraser) Delete(_ context.Context, userID int64) error {
	if f.err != nil {
		return f.err
	}
	f.deleted = append(f.deleted, userID)
	return nil
}

// fakeEmailLookup resolves the caller's own email, which is what the confirmation is
// checked against.
type fakeEmailLookup struct {
	email string
	err   error
}

func (f fakeEmailLookup) UserEmail(context.Context, int64) (string, error) {
	return f.email, f.err
}

func deleteAccountApp(t *testing.T, eraser *fakeEraser, email string) (*fiber.App, string) {
	t.Helper()
	iss := auth.NewIssuer("test-secret", time.Hour)
	token, err := iss.Issue(1, testTokenVersion)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	h := &authHandlers{issuer: iss, accountDelete: eraser, accountEmails: fakeEmailLookup{email: email}}
	app := fiber.New(fiber.Config{ErrorHandler: RenderError})
	app.Delete("/me", auth.RequireAuth(iss, testVersions), h.DeleteAccount)
	return app, token
}

func doDelete(t *testing.T, app *fiber.App, body, token string) *http.Response {
	t.Helper()
	r := httptest.NewRequestWithContext(context.Background(), fiber.MethodDelete, "/me", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if token != "" {
		r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	}
	resp, err := app.Test(r)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	return resp
}

func TestDeleteAccount_ConfirmedByOwnEmail(t *testing.T) {
	eraser := &fakeEraser{}
	app, token := deleteAccountApp(t, eraser, "member@example.test")

	resp := doDelete(t, app, `{"email":"member@example.test"}`, token)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusNoContent {
		t.Fatalf("status = %d, want 204", resp.StatusCode)
	}
	if len(eraser.deleted) != 1 || eraser.deleted[0] != 1 {
		t.Errorf("deleted = %v, want the caller's own id", eraser.deleted)
	}
	// The session must not outlive the account it names. ClearTokenCookie expires it
	// by writing an empty value with a past Expires.
	var cleared bool
	for _, c := range resp.Cookies() {
		if c.Name == auth.CookieName && c.Value == "" && c.Expires.Before(time.Now()) {
			cleared = true
		}
	}
	if !cleared {
		t.Errorf("response did not expire the session cookie (cookies: %v)", resp.Cookies())
	}
}

// Case is not identity: accounts are looked up case-insensitively at login, so the
// confirmation must accept the same address typed differently.
func TestDeleteAccount_ConfirmationIsCaseInsensitive(t *testing.T) {
	eraser := &fakeEraser{}
	app, token := deleteAccountApp(t, eraser, "Member@Example.test")

	resp := doDelete(t, app, `{"email":"member@example.TEST"}`, token)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusNoContent {
		t.Errorf("status = %d, want 204", resp.StatusCode)
	}
	if len(eraser.deleted) != 1 {
		t.Errorf("deleted = %v, want the account erased", eraser.deleted)
	}
}

func TestDeleteAccount_RejectsWrongOrMissingConfirmation(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{"someone else's address", `{"email":"someone@example.test"}`},
		{"empty", `{"email":""}`},
		{"no field", `{}`},
		{"not json", `garbage`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			eraser := &fakeEraser{}
			app, token := deleteAccountApp(t, eraser, "member@example.test")

			resp := doDelete(t, app, tc.body, token)
			defer resp.Body.Close()
			if resp.StatusCode != fiber.StatusBadRequest {
				t.Errorf("status = %d, want 400", resp.StatusCode)
			}
			if len(eraser.deleted) != 0 {
				t.Error("account was erased without a matching confirmation")
			}
		})
	}
}

func TestDeleteAccount_RejectsAnonymous(t *testing.T) {
	eraser := &fakeEraser{}
	app, _ := deleteAccountApp(t, eraser, "member@example.test")

	resp := doDelete(t, app, `{"email":"member@example.test"}`, "")
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
	if len(eraser.deleted) != 0 {
		t.Error("an anonymous request erased an account")
	}
}

// A leaked API key must not be able to destroy the account that issued it, so the
// route is mounted cookie-only and a Bearer credential is simply not authentication.
func TestDeleteAccount_RejectsAPIKey(t *testing.T) {
	eraser := &fakeEraser{}
	app, _ := deleteAccountApp(t, eraser, "member@example.test")

	r := httptest.NewRequestWithContext(context.Background(), fiber.MethodDelete, "/me", strings.NewReader(`{"email":"member@example.test"}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Authorization", "Bearer fh_livekey")
	resp, err := app.Test(r)
	if err != nil {
		t.Fatalf("Test: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Errorf("status = %d, want 401", resp.StatusCode)
	}
	if len(eraser.deleted) != 0 {
		t.Error("an API key erased an account")
	}
}

// Storage being unreachable means nothing was erased. That is a retry, not a
// half-deleted account, and the caller is told so.
func TestDeleteAccount_StorageFailureIsRetryable(t *testing.T) {
	eraser := &fakeEraser{err: fmt.Errorf("erase objects: %w", accountdelete.ErrStorageUnavailable)}
	app, token := deleteAccountApp(t, eraser, "member@example.test")

	resp := doDelete(t, app, `{"email":"member@example.test"}`, token)
	defer resp.Body.Close()
	if resp.StatusCode != fiber.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", resp.StatusCode)
	}
}
