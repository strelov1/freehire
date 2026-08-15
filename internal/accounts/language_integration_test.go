//go:build integration

// Integration test for a regression CodeRabbit flagged on PR #1944:
// GetUserByEmail (the Login lookup) did not select users.language, so a
// password login silently answered the zero value instead of the account's
// actual preference. Run with: go test -tags=integration ./internal/accounts/
package accounts_test

import (
	"context"
	"testing"

	"github.com/strelov1/freehire/internal/accounts"
)

func TestLogin_ReturnsLanguage(t *testing.T) {
	pool := startPostgres(t)
	repo := oauthRepo(pool)
	svc := accounts.New(repo, plainHasher{})

	user, err := svc.Register(context.Background(), "lang-login@example.test", "password123", nil)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if _, err := svc.UpdateLanguage(context.Background(), user.ID, "ru"); err != nil {
		t.Fatalf("UpdateLanguage: %v", err)
	}

	got, err := svc.Login(context.Background(), "lang-login@example.test", "password123")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if got.Language != "ru" {
		t.Errorf("Login Language = %q, want %q", got.Language, "ru")
	}
}
