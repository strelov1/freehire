//go:build integration

// Integration tests for the OAuth link-or-create transaction — the account-pre-hijacking
// fix. Whether a provider identity may silently join an existing account depends on that
// account's verified state, and the seizure that follows an unverified one is a
// multi-statement transaction, so this is only meaningful against a real Postgres. It
// lives in the external test package because it needs both accounts and db, which
// accounts itself imports. Run with: go test -tags=integration ./internal/accounts/
package accounts_test

import (
	"context"
	"path/filepath"
	"sort"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/strelov1/freehire/internal/accounts"
	"github.com/strelov1/freehire/internal/db"
)

func startPostgres(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	// Apply every migration, in name order — the same way Postgres initdb runs
	// the mounted migrations/ dir — so a new migration is never silently missing
	// from the test schema.
	migrationsDir, err := filepath.Abs(filepath.Join("..", "..", "migrations"))
	if err != nil {
		t.Fatalf("resolve migrations dir: %v", err)
	}
	scripts, err := filepath.Glob(filepath.Join(migrationsDir, "*.sql"))
	if err != nil || len(scripts) == 0 {
		t.Fatalf("list migrations: %v (found %d)", err, len(scripts))
	}
	sort.Strings(scripts)

	pg, err := postgres.Run(ctx, "postgres:18-alpine",
		postgres.WithDatabase("hire"),
		postgres.WithUsername("hire"),
		postgres.WithPassword("hire"),
		postgres.WithInitScripts(scripts...),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	t.Cleanup(func() { _ = pg.Terminate(ctx) })

	dsn, err := pg.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		t.Fatalf("connection string: %v", err)
	}
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("pool: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// accountRow is the state assertions read back after a link.
type accountRow struct {
	id           int64
	hasPassword  bool
	verified     bool
	tokenVersion int32
}

func readAccount(t *testing.T, pool *pgxpool.Pool, email string) accountRow {
	t.Helper()
	var got accountRow
	var hash *string
	if err := pool.QueryRow(context.Background(),
		`SELECT id, password_hash, email_verified, token_version FROM users WHERE lower(email) = lower($1)`,
		email).Scan(&got.id, &hash, &got.verified, &got.tokenVersion); err != nil {
		t.Fatalf("read account %s: %v", email, err)
	}
	got.hasPassword = hash != nil
	return got
}

// oauthRepo builds the real repository over the test pool.
func oauthRepo(pool *pgxpool.Pool) *accounts.QueriesRepository {
	return accounts.NewQueriesRepository(db.New(pool), pool)
}

func TestLinkSeizesAnUnverifiedPasswordAccount(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	repo := oauthRepo(pool)

	// The squatter registered the victim's address first and never proved it.
	const email = "victim@example.test"
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (email, password_hash, email_verified) VALUES ($1, 'squatter-hash', false)`,
		email); err != nil {
		t.Fatalf("seed squatted account: %v", err)
	}
	before := readAccount(t, pool, email)

	// The victim signs in with a provider that vouches for the address.
	id, err := repo.LinkOrCreateByEmail(ctx, "google", "google-victim", email)
	if err != nil {
		t.Fatalf("LinkOrCreateByEmail: %v", err)
	}
	if id != before.id {
		t.Fatalf("linked to user %d, want the existing account %d", id, before.id)
	}

	after := readAccount(t, pool, email)
	if after.hasPassword {
		t.Error("the squatter's password survived the link — they keep a way in")
	}
	if !after.verified {
		t.Error("the account should be verified: the provider proved the address")
	}
	if after.tokenVersion <= before.tokenVersion {
		t.Errorf("token_version %d \u2192 %d: the squatter's sessions were not revoked",
			before.tokenVersion, after.tokenVersion)
	}
}

func TestLinkLeavesAVerifiedAccountIntact(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	repo := oauthRepo(pool)

	const email = "owner@example.test"
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (email, password_hash, email_verified) VALUES ($1, 'owner-hash', true)`,
		email); err != nil {
		t.Fatalf("seed verified account: %v", err)
	}
	before := readAccount(t, pool, email)

	if _, err := repo.LinkOrCreateByEmail(ctx, "google", "google-owner", email); err != nil {
		t.Fatalf("LinkOrCreateByEmail: %v", err)
	}

	after := readAccount(t, pool, email)
	if !after.hasPassword {
		t.Error("a verified account must keep its password when a provider is linked")
	}
	if after.tokenVersion != before.tokenVersion {
		t.Error("linking a provider to a verified account must not sign the owner out")
	}
}

func TestLinkLeavesAnUnverifiedPasswordlessAccountAlone(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	repo := oauthRepo(pool)

	// An account with no password cannot have been squatted by password: there is
	// nothing to seize, so the link must not churn the session generation.
	const email = "passwordless@example.test"
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (email, email_verified) VALUES ($1, false)`, email); err != nil {
		t.Fatalf("seed passwordless account: %v", err)
	}
	before := readAccount(t, pool, email)

	if _, err := repo.LinkOrCreateByEmail(ctx, "github", "github-user", email); err != nil {
		t.Fatalf("LinkOrCreateByEmail: %v", err)
	}

	after := readAccount(t, pool, email)
	if after.tokenVersion != before.tokenVersion {
		t.Error("nothing to seize, so no session should have been revoked")
	}
	if !after.verified {
		t.Error("the provider proved the address; the account should end up verified")
	}
}

func TestLinkCreatesNewAccountsVerified(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	repo := oauthRepo(pool)

	const email = "fresh-oauth@example.test"
	if _, err := repo.LinkOrCreateByEmail(ctx, "google", "google-fresh", email); err != nil {
		t.Fatalf("LinkOrCreateByEmail: %v", err)
	}

	got := readAccount(t, pool, email)
	if !got.verified {
		t.Error("an account born from a provider-verified sign-in must be verified")
	}
	if got.hasPassword {
		t.Error("an OAuth-created account must be passwordless")
	}
}
