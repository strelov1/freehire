//go:build integration

// The API product end to end against a real Postgres: an API subscription writes its own
// column and no plan column, and a lapse clears it on the next sync.
package billing

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/platform/db"
)

func apiUntilsOf(t *testing.T, pool *pgxpool.Pool, userID int64) (api, data pgtype.Timestamptz) {
	t.Helper()
	err := pool.QueryRow(context.Background(),
		`SELECT api_until_stripe, api_data_until_stripe FROM users WHERE id = $1`, userID).Scan(&api, &data)
	if err != nil {
		t.Fatalf("reading the API columns: %v", err)
	}
	return api, data
}

func TestAnAPIPriceConfersTheAPIProductAndNoPlan(t *testing.T) {
	until := time.Now().UTC().Add(30 * 24 * time.Hour).Truncate(time.Second)
	base, pool := newService(t, subscriptionsFor(apiPrice, until))
	t.Setenv("STRIPE_API_PRICE_IDS", apiPrice)
	s := NewWithBase(ConfigFromEnv(), db.New(pool), base.client.baseURL)
	ctx := context.Background()

	userID := insertUser(t, pool, "api-buyer@example.com")
	bindCustomer(t, pool, userID, "cus_api")
	if err := s.SyncCaller(ctx, userID); err != nil {
		t.Fatalf("SyncCaller: %v", err)
	}

	api, data := apiUntilsOf(t, pool, userID)
	if !api.Valid || !api.Time.UTC().Equal(until) || data.Valid {
		t.Fatalf("api_until_stripe = %v (valid=%v), api_data_until_stripe valid=%v; want api = %v, data NULL",
			api.Time, api.Valid, data.Valid, until)
	}
	pro, ultra := untilsOf(t, pool, userID)
	if pro.Valid || ultra.Valid {
		t.Fatalf("pro_until valid=%v ultra_until valid=%v — the API product must confer no plan", pro.Valid, ultra.Valid)
	}

	// Credential introspection sees the same entitlement.
	row, err := db.New(pool).GetUserAPIEntitlement(ctx, userID)
	if err != nil || !row.ApiUntilStripe.Time.UTC().Equal(until) {
		t.Fatalf("GetUserAPIEntitlement = %+v, %v", row, err)
	}
}

// TestResolveAPIKeyIdentityReadsTheOwnersEntitlement covers credential introspection: a live key resolves to its owner's API columns, an expired one to nothing.
func TestResolveAPIKeyIdentityReadsTheOwnersEntitlement(t *testing.T) {
	_, pool := newService(t, subscriptionsFor(apiPrice, time.Now()))
	ctx := context.Background()
	q := db.New(pool)
	userID := insertUser(t, pool, "key-owner@example.com")
	until := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	if _, err := pool.Exec(ctx, `UPDATE users SET api_until_stripe = $1 WHERE id = $2`, until, userID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO api_keys (user_id, name, token_hash, token_prefix, scope)
		VALUES ($1, 'live', 'hash-live', 'fhk_live', 'full'),
		       ($1, 'gone', 'hash-gone', 'fhk_gone', 'full')`, userID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE api_keys SET expires_at = now() - interval '1 minute' WHERE token_hash = 'hash-gone'`); err != nil {
		t.Fatal(err)
	}

	row, err := q.ResolveAPIKeyIdentity(ctx, "hash-live")
	if err != nil || row.UserID != userID || row.Scope != "full" || !row.ApiUntilStripe.Time.UTC().Equal(until) {
		t.Fatalf("live key = %+v, %v", row, err)
	}
	if _, err := q.ResolveAPIKeyIdentity(ctx, "hash-gone"); err == nil {
		t.Fatal("an expired key resolved")
	}
}
