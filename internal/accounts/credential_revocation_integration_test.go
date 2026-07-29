//go:build integration

// Integration tests for the OTHER half of revoking an account's credentials: its API
// keys. Advancing token_version strands every session, but an API key authenticates by
// its own hash against api_keys and never consults users, so a session revocation alone
// leaves it live. These tests pin the two events where the account is understood to have
// changed hands — a seizure and a mailed-code password reset — to also destroying the
// keys. They need a real Postgres because the guarantee lives in the SQL statement.
// Run with: go test -tags=integration ./internal/accounts/
package accounts_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// seedAPIKey stores a never-expiring, full-scope key for the user, as POST /me/api-keys
// mints one, and returns its hash.
func seedAPIKey(t *testing.T, pool *pgxpool.Pool, userID int64, hash string) string {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO api_keys (user_id, name, token_hash, token_prefix, scope, expires_at)
		 VALUES ($1, 'squatter cli', $2, 'fh_test', 'full', NULL)`,
		userID, hash); err != nil {
		t.Fatalf("seed api key: %v", err)
	}
	return hash
}

// liveKeys counts the keys that would still authenticate for the user.
func liveKeys(t *testing.T, pool *pgxpool.Pool, userID int64) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM api_keys WHERE user_id = $1`, userID).Scan(&n); err != nil {
		t.Fatalf("count api keys: %v", err)
	}
	return n
}

func TestSeizureDestroysTheSquattersAPIKeys(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	repo := oauthRepo(pool)

	// The squatter registered the victim's address, never proved it, and — because
	// key creation is only guarded by a session — minted a bearer credential that
	// outlives any cookie.
	const email = "keyed-squatter@example.test"
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (email, password_hash, email_verified) VALUES ($1, 'squatter-hash', false)`,
		email); err != nil {
		t.Fatalf("seed squatted account: %v", err)
	}
	user := readAccount(t, pool, email)
	seedAPIKey(t, pool, user.id, "squatter-key-hash")

	// The real owner arrives with a provider-verified identity.
	if _, err := repo.LinkOrCreateByEmail(ctx, "google", "google-keyed-victim", email); err != nil {
		t.Fatalf("LinkOrCreateByEmail: %v", err)
	}

	if n := liveKeys(t, pool, user.id); n != 0 {
		t.Errorf("%d API key(s) survived the seizure: the squatter keeps bearer access to "+
			"the account they just lost the password and sessions to", n)
	}
}

func TestSeizureLeavesOtherAccountsKeysAlone(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	repo := oauthRepo(pool)

	const seized = "narrow-seizure@example.test"
	const bystander = "bystander@example.test"
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (email, password_hash, email_verified) VALUES ($1, 'hash', false), ($2, 'hash', true)`,
		seized, bystander); err != nil {
		t.Fatalf("seed accounts: %v", err)
	}
	seizedUser := readAccount(t, pool, seized)
	otherUser := readAccount(t, pool, bystander)
	seedAPIKey(t, pool, seizedUser.id, "doomed-key-hash")
	seedAPIKey(t, pool, otherUser.id, "innocent-key-hash")

	if _, err := repo.LinkOrCreateByEmail(ctx, "google", "google-narrow", seized); err != nil {
		t.Fatalf("LinkOrCreateByEmail: %v", err)
	}

	if n := liveKeys(t, pool, otherUser.id); n != 1 {
		t.Errorf("a bystander account lost %d key(s): the delete is not scoped to the seized user", 1-n)
	}
}

func TestPasswordResetDestroysTheAccountsAPIKeys(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	repo := oauthRepo(pool)

	// A reset by mailed code is how an owner takes an account back. Whoever knew the
	// old password may have minted a key from it; leaving that key live makes the
	// recovery cosmetic.
	const email = "recovering@example.test"
	if _, err := pool.Exec(ctx,
		`INSERT INTO users (email, password_hash, email_verified) VALUES ($1, 'stolen-hash', true)`,
		email); err != nil {
		t.Fatalf("seed account: %v", err)
	}
	user := readAccount(t, pool, email)
	seedAPIKey(t, pool, user.id, "attacker-key-hash")

	if _, err := repo.ResetPassword(ctx, user.id, "fresh-hash"); err != nil {
		t.Fatalf("ResetPassword: %v", err)
	}

	if n := liveKeys(t, pool, user.id); n != 0 {
		t.Errorf("%d API key(s) survived the password reset: an attacker who held the old "+
			"password keeps programmatic access after the owner recovers", n)
	}
}
