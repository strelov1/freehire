//go:build integration

// Integration tests for OAuth account resolution: identity-first lookup,
// linking by verified email, and passwordless account creation can only be
// exercised against real Postgres constraints. Run with:
// go test -tags=integration ./internal/handler/
package handler

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/accounts"
	"github.com/strelov1/freehire/internal/db"
)

func oauthHandler(t *testing.T) *authHandlers {
	t.Helper()
	pool := startPostgres(t)
	queries := db.New(pool)
	return &authHandlers{queries: queries, accounts: accounts.New(accounts.NewQueriesRepository(queries, pool), authHasher{})}
}

func TestResolveOAuthUser_CreatesPasswordlessUser(t *testing.T) {
	h := oauthHandler(t)
	ctx := context.Background()

	id, err := h.accounts.ResolveOAuthAccount(ctx, "google", "g-1", "New@Example.com", true)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}

	user, err := h.queries.GetUserByEmail(ctx, "new@example.com")
	if err != nil {
		t.Fatalf("lookup created user: %v", err)
	}
	if user.ID != id {
		t.Errorf("resolved id %d != created user %d", id, user.ID)
	}
	if user.PasswordHash.Valid {
		t.Error("OAuth-created user has a password hash; want NULL")
	}
}

func TestResolveOAuthUser_ReturningIdentityResolvesSameUser(t *testing.T) {
	h := oauthHandler(t)
	ctx := context.Background()
	first, err := h.accounts.ResolveOAuthAccount(ctx, "google", "g-2", "ret@example.com", true)
	if err != nil {
		t.Fatalf("first resolve: %v", err)
	}
	// Even if the provider email changed since, the identity wins.
	second, err := h.accounts.ResolveOAuthAccount(ctx, "google", "g-2", "changed@example.com", true)
	if err != nil {
		t.Fatalf("second resolve: %v", err)
	}
	if first != second {
		t.Errorf("returning identity resolved to %d, want %d", second, first)
	}
	if _, err := h.queries.GetUserByEmail(ctx, "changed@example.com"); err == nil {
		t.Error("changed provider email created an account; want identity-first resolution")
	}
}

func TestResolveOAuthUser_LinksExistingPasswordAccountByEmail(t *testing.T) {
	h := oauthHandler(t)
	ctx := context.Background()

	existing, err := h.queries.CreateUser(ctx, db.CreateUserParams{
		Email:        "linked@example.com",
		PasswordHash: pgtype.Text{String: "$2a$10$fakehash", Valid: true},
	})
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}

	id, err := h.accounts.ResolveOAuthAccount(ctx, "github", "gh-1", "Linked@Example.com", true)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if id != existing.ID {
		t.Errorf("resolved id %d, want existing account %d", id, existing.ID)
	}
	// The password must survive the link.
	user, err := h.queries.GetUserByEmail(ctx, "linked@example.com")
	if err != nil || !user.PasswordHash.Valid {
		t.Errorf("password hash lost on link (err=%v valid=%v)", err, user.PasswordHash.Valid)
	}
}

func TestResolveOAuthUser_RejectsUnverifiedEmail(t *testing.T) {
	h := oauthHandler(t)
	ctx := context.Background()

	if _, err := h.accounts.ResolveOAuthAccount(ctx, "github", "gh-2", "victim@example.com", false); err == nil {
		t.Fatal("want error for unverified email")
	}
	if _, err := h.queries.GetUserByEmail(ctx, "victim@example.com"); err == nil {
		t.Error("unverified email created an account")
	}
}

func TestResolveOAuthUser_SameEmailDifferentProvidersShareAccount(t *testing.T) {
	h := oauthHandler(t)
	ctx := context.Background()

	a, err := h.accounts.ResolveOAuthAccount(ctx, "google", "g-3", "multi@example.com", true)
	if err != nil {
		t.Fatalf("google resolve: %v", err)
	}
	b, err := h.accounts.ResolveOAuthAccount(ctx, "github", "gh-3", "multi@example.com", true)
	if err != nil {
		t.Fatalf("github resolve: %v", err)
	}
	if a != b {
		t.Errorf("same email resolved to different accounts: %d vs %d", a, b)
	}
}
