//go:build integration

package db

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestCredentialIntrospectionQueries(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	user := seedAPIKeyUser(t, pool, "introspection@example.test")
	until := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	if _, err := pool.Exec(ctx, `UPDATE users SET pro_until_granted = $2 WHERE id = $1`, user, until); err != nil {
		t.Fatal(err)
	}
	key, err := q.CreateAPIKey(ctx, CreateAPIKeyParams{UserID: user, Name: "introspection", TokenHash: "identity-key", TokenPrefix: "fhk_identity", Scope: "cv", ExpiresAt: pgtype.Timestamptz{Time: until, Valid: true}})
	if err != nil {
		t.Fatal(err)
	}
	identity, err := q.ResolveAPIKeyIdentity(ctx, "identity-key")
	if err != nil || identity.UserID != user || identity.Scope != "cv" || !identity.ExpiresAt.Time.Equal(until) || !identity.ProUntil.Time.Equal(until) {
		t.Fatalf("key identity=%+v err=%v", identity, err)
	}
	keys, err := q.ListAPIKeysByUser(ctx, user)
	if err != nil || len(keys) != 1 || keys[0].LastUsedAt.Valid {
		t.Fatalf("introspection changed key usage: %+v err=%v", keys, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE api_keys SET expires_at = now() - interval '1 second' WHERE id = $1`, key.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := q.ResolveAPIKeyIdentity(ctx, "identity-key"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("expired key err=%v", err)
	}
	if _, err := q.RegisterOAuthClient(ctx, RegisterOAuthClientParams{ClientID: "identity-client", ClientName: "Introspection", RedirectUris: []string{"http://127.0.0.1/callback"}}); err != nil {
		t.Fatal(err)
	}
	version, err := q.GetUserTokenVersion(ctx, user)
	if err != nil {
		t.Fatal(err)
	}
	for hash, expiry := range map[string]time.Time{"identity-grant": until, "identity-expired": time.Now().Add(-time.Hour)} {
		if _, err := q.CreateOAuthGrant(ctx, CreateOAuthGrantParams{UserID: user, ClientID: "identity-client", AccessTokenHash: hash, IssuedTokenVersion: version, ExpiresAt: pgtype.Timestamptz{Time: expiry, Valid: true}}); err != nil {
			t.Fatal(err)
		}
	}
	grant, err := q.ResolveOAuthGrantIdentity(ctx, "identity-grant")
	if err != nil || grant.UserID != user || !grant.ExpiresAt.Time.Equal(until) || !grant.ProUntil.Time.Equal(until) {
		t.Fatalf("grant identity=%+v err=%v", grant, err)
	}
	var lastUsed pgtype.Timestamptz
	if err := pool.QueryRow(ctx, `SELECT last_used_at FROM oauth_grants WHERE access_token_hash = 'identity-grant'`).Scan(&lastUsed); err != nil || lastUsed.Valid {
		t.Fatalf("introspection changed grant usage: %+v err=%v", lastUsed, err)
	}
	if _, err := q.ResolveOAuthGrantIdentity(ctx, "identity-expired"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("expired grant err=%v", err)
	}
	if _, err := q.BumpUserTokenVersion(ctx, user); err != nil {
		t.Fatal(err)
	}
	if _, err := q.ResolveOAuthGrantIdentity(ctx, "identity-grant"); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("revoked grant err=%v", err)
	}
}
