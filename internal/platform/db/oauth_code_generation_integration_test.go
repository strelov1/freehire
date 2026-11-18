//go:build integration

// Integration tests for the authorization code's generation latch
// (oauth_authorization_codes.issued_token_version, 0233): a code minted under
// an account generation becomes unredeemable the moment that generation stops
// being current — the same rule oauth_grants.issued_token_version already
// enforces for grants. The row is deleted only when the latch holds, so a
// stale code stays standing and dead. SQL behavior — real Postgres only. Run
// with: go test -tags=integration ./internal/platform/db/
package db

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// seedOAuthCode inserts one pending authorization code. A negative issuedVersion
// seeds the unknown-generation sentinel (0): the fail-closed shape the 0233
// backfill stamps onto pre-0233 rows, dead for exactly the same reason. issued=0
// cannot be used for the sentinel directly — 0 is also the zero value of the
// param struct's pgtype.Int4.Int32 — so the sentinel form is what the negative
// case builds explicitly.
func seedOAuthCode(t *testing.T, q *Queries, codeHash string, userID int64, issuedVersion int32) {
	t.Helper()
	params := CreateOAuthAuthorizationCodeParams{
		CodeHash:      codeHash,
		ClientID:      "client-gen-1",
		UserID:        userID,
		RedirectUri:   "http://127.0.0.1:9999/callback",
		CodeChallenge: "challenge-gen",
		ExpiresAt:     pgtype.Timestamptz{Time: time.Now().Add(10 * time.Minute), Valid: true},
	}
	if issuedVersion >= 0 {
		params.IssuedTokenVersion = pgtype.Int4{Int32: issuedVersion, Valid: true}
	} else {
		params.IssuedTokenVersion = pgtype.Int4{Int32: 0, Valid: true}
	}
	if err := q.CreateOAuthAuthorizationCode(context.Background(), params); err != nil {
		t.Fatalf("CreateOAuthAuthorizationCode: %v", err)
	}
}

func TestOAuthCodeGenerationLatch(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	alice := seedAPIKeyUser(t, pool, "oauth-gen-alice@example.test")
	if _, err := q.RegisterOAuthClient(ctx, RegisterOAuthClientParams{
		ClientID:     "client-gen-1",
		ClientName:   "Test MCP Client",
		RedirectUris: []string{"http://127.0.0.1:9999/callback"},
	}); err != nil {
		t.Fatalf("RegisterOAuthClient: %v", err)
	}
	version, err := q.GetUserTokenVersion(ctx, alice)
	if err != nil {
		t.Fatalf("GetUserTokenVersion: %v", err)
	}

	t.Run("a code of the current generation is redeemable", func(t *testing.T) {
		seedOAuthCode(t, q, "gen-current", alice, version)
		row, err := q.ConsumeOAuthAuthorizationCode(ctx, "gen-current")
		if err != nil {
			t.Fatalf("ConsumeOAuthAuthorizationCode: %v", err)
		}
		if row.IssuedTokenVersion.Int32 != version {
			t.Errorf("consumed generation = %d, want %d", row.IssuedTokenVersion.Int32, version)
		}
		var n int
		if err := pool.QueryRow(ctx,
			`SELECT count(*) FROM oauth_authorization_codes WHERE code_hash = 'gen-current'`).Scan(&n); err != nil {
			t.Fatalf("count: %v", err)
		}
		if n != 0 {
			t.Errorf("redeemed code row remains")
		}
	})

	t.Run("a code left pending over a generation bump is refused and left dead, not deleted", func(t *testing.T) {
		seedOAuthCode(t, q, "gen-stale", alice, version)
		bumped, err := q.BumpUserTokenVersion(ctx, alice)
		if err != nil {
			t.Fatalf("BumpUserTokenVersion: %v", err)
		}
		if bumped != version+1 {
			t.Fatalf("bumped to %d, want %d", bumped, version+1)
		}
		if _, err := q.ConsumeOAuthAuthorizationCode(ctx, "gen-stale"); !errors.Is(err, pgx.ErrNoRows) {
			t.Errorf("stale-generation code err = %v, want pgx.ErrNoRows", err)
		}
		var n int
		if err := pool.QueryRow(ctx,
			`SELECT count(*) FROM oauth_authorization_codes WHERE code_hash = 'gen-stale'`).Scan(&n); err != nil {
			t.Fatalf("count: %v", err)
		}
		if n != 1 {
			t.Errorf("stale code was deleted; it must stand dead so no second exchange can revive it")
		}
		// The refusal is permanent under the monotonic-generation assumption every
		// revocation path obeys (token_version only ever increments): the row's
		// generation can never become current again, so a retry re-derives the same
		// refusal. (Nothing lowers a generation; if one ever did, equality would of
		// course admit the code again.)
		if _, err := q.ConsumeOAuthAuthorizationCode(ctx, "gen-stale"); !errors.Is(err, pgx.ErrNoRows) {
			t.Errorf("retry of dead code err = %v, want pgx.ErrNoRows", err)
		}
	})

	t.Run("the unknown-generation sentinel is dead on arrival", func(t *testing.T) {
		seedOAuthCode(t, q, "gen-unknown", alice, -1)
		if _, err := q.ConsumeOAuthAuthorizationCode(ctx, "gen-unknown"); !errors.Is(err, pgx.ErrNoRows) {
			t.Errorf("sentinel-generation code err = %v, want pgx.ErrNoRows", err)
		}
		var n int
		if err := pool.QueryRow(ctx,
			`SELECT count(*) FROM oauth_authorization_codes WHERE code_hash = 'gen-unknown'`).Scan(&n); err != nil {
			t.Fatalf("count: %v", err)
		}
		if n != 1 {
			t.Errorf("sentinel code was deleted; it must stand dead like any stale code")
		}
	})

	// Expiry is independent of the generation latch: a LIVE-generation code that
	// has passed its expires_at is refused on its own clock.
	t.Run("a live code expires on its own clock regardless of generation", func(t *testing.T) {
		// The earlier subtests bumped the account; read the CURRENT generation so
		// this subtest fails on expiry alone, never on a stale generation.
		current, err := q.GetUserTokenVersion(ctx, alice)
		if err != nil {
			t.Fatalf("GetUserTokenVersion: %v", err)
		}
		params := CreateOAuthAuthorizationCodeParams{
			CodeHash:           "gen-expired",
			ClientID:           "client-gen-1",
			UserID:             alice,
			RedirectUri:        "http://127.0.0.1:9999/callback",
			CodeChallenge:      "challenge-gen",
			IssuedTokenVersion: pgtype.Int4{Int32: current, Valid: true},
			ExpiresAt:          pgtype.Timestamptz{Time: time.Now().Add(-time.Minute), Valid: true},
		}
		if err := q.CreateOAuthAuthorizationCode(ctx, params); err != nil {
			t.Fatalf("CreateOAuthAuthorizationCode: %v", err)
		}
		if _, err := q.ConsumeOAuthAuthorizationCode(ctx, "gen-expired"); !errors.Is(err, pgx.ErrNoRows) {
			t.Errorf("expired code err = %v, want pgx.ErrNoRows", err)
		}
	})

	// A NULL issued_token_version is what the OLD binary writes during rollout
	// (its INSERT omits the new column) — distinct from the 0 sentinel the
	// migration backfills. NULL never satisfies the equality latch, so those
	// codes are dead too: fail closed in both rollout directions.
	t.Run("a NULL-generation code from the old binary is dead", func(t *testing.T) {
		if _, err := pool.Exec(ctx,
			`INSERT INTO oauth_authorization_codes
			     (code_hash, client_id, user_id, redirect_uri, code_challenge, expires_at)
			 VALUES ('gen-null', 'client-gen-1', $1, 'http://127.0.0.1:9999/callback', 'challenge-gen', now() + interval '10 minutes')`, alice); err != nil {
			t.Fatalf("seed NULL code: %v", err)
		}
		if _, err := q.ConsumeOAuthAuthorizationCode(ctx, "gen-null"); !errors.Is(err, pgx.ErrNoRows) {
			t.Errorf("NULL-generation code err = %v, want pgx.ErrNoRows", err)
		}
		var n int
		if err := pool.QueryRow(ctx,
			`SELECT count(*) FROM oauth_authorization_codes WHERE code_hash = 'gen-null'`).Scan(&n); err != nil {
			t.Fatalf("count: %v", err)
		}
		if n != 1 {
			t.Errorf("NULL code was deleted; it must stand dead like any stale code")
		}
	})
}
