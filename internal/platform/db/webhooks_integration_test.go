//go:build integration

// Integration tests for the webhook_configs auto-disable threshold — the
// RETURNING ... CASE WHEN in RecordWebhookDeliveryFailure can only be verified
// against a real Postgres. Run with: go test -tags=integration ./internal/platform/db/
// Requires Docker (testcontainers spins up a throwaway Postgres with the migrations).
package db

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
)

// A destination that fails exactly maxFailures times in a row gets disabled on
// that last call — not before, not after — the same boundary MaxAttempts draws
// for subscription_matches, just counted per destination instead of per match.
func TestRecordWebhookDeliveryFailure_DisablesAtThreshold(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, "TRUNCATE webhook_configs, users RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	uid := insertUser(t, pool, "webhook-threshold@example.test")
	if _, err := q.UpsertWebhookConfig(ctx, UpsertWebhookConfigParams{UserID: uid, URL: "https://example.test/hook", Secret: "whsec_test"}); err != nil {
		t.Fatalf("seed webhook config: %v", err)
	}

	const maxFailures = 3
	for i := int64(1); i <= maxFailures; i++ {
		row, err := q.RecordWebhookDeliveryFailure(ctx, RecordWebhookDeliveryFailureParams{UserID: uid, MaxFailures: maxFailures})
		if err != nil {
			t.Fatalf("record failure %d: %v", i, err)
		}
		if row.ConsecutiveFailures != i {
			t.Errorf("failure %d: ConsecutiveFailures = %d, want %d", i, row.ConsecutiveFailures, i)
		}
		wantEnabled := i < maxFailures
		if row.Enabled != wantEnabled {
			t.Errorf("failure %d: Enabled = %v, want %v", i, row.Enabled, wantEnabled)
		}
	}

	got, err := q.GetWebhookConfig(ctx, uid)
	if err != nil {
		t.Fatalf("get webhook config: %v", err)
	}
	if got.Enabled {
		t.Error("webhook config enabled after reaching the failure threshold, want disabled")
	}
	if !got.DisabledAt.Valid {
		t.Error("disabled_at not stamped after reaching the failure threshold")
	}
}

// A success between failures resets the streak, so an occasional blip never
// adds up toward disabling an otherwise-healthy destination.
func TestRecordWebhookDeliveryFailure_SuccessResetsStreak(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, "TRUNCATE webhook_configs, users RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	uid := insertUser(t, pool, "webhook-reset@example.test")
	if _, err := q.UpsertWebhookConfig(ctx, UpsertWebhookConfigParams{UserID: uid, URL: "https://example.test/hook", Secret: "whsec_test"}); err != nil {
		t.Fatalf("seed webhook config: %v", err)
	}

	const maxFailures = 3
	for i := 0; i < maxFailures-1; i++ {
		if _, err := q.RecordWebhookDeliveryFailure(ctx, RecordWebhookDeliveryFailureParams{UserID: uid, MaxFailures: maxFailures}); err != nil {
			t.Fatalf("record failure: %v", err)
		}
	}
	if err := q.RecordWebhookDeliverySuccess(ctx, uid); err != nil {
		t.Fatalf("record success: %v", err)
	}

	row, err := q.RecordWebhookDeliveryFailure(ctx, RecordWebhookDeliveryFailureParams{UserID: uid, MaxFailures: maxFailures})
	if err != nil {
		t.Fatalf("record failure after reset: %v", err)
	}
	if row.ConsecutiveFailures != 1 {
		t.Errorf("ConsecutiveFailures after a success = %d, want 1 (reset, not %d)", row.ConsecutiveFailures, maxFailures)
	}
	if !row.Enabled {
		t.Error("Enabled = false right after a reset, want true — the streak restarted, not continued")
	}
}

// An update lands only on the row whose secret signed the caller's probe: a save that probed
// with another secret (a concurrent create or delete-and-recreate) gets no row back and changes
// nothing, while a save with the stored secret updates the URL and keeps the secret.
func TestUpsertWebhookConfig_UpdatesOnlyWithTheStoredSecret(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	if _, err := pool.Exec(ctx, "TRUNCATE webhook_configs, users RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}
	uid := insertUser(t, pool, "webhook-secret@example.test")
	if _, err := q.UpsertWebhookConfig(ctx, UpsertWebhookConfigParams{UserID: uid, URL: "https://example.test/a", Secret: "whsec_first"}); err != nil {
		t.Fatalf("create: %v", err)
	}

	_, err := q.UpsertWebhookConfig(ctx, UpsertWebhookConfigParams{UserID: uid, URL: "https://example.test/b", Secret: "whsec_other"})
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("upsert with another secret: err = %v, want pgx.ErrNoRows", err)
	}
	if row, _ := q.GetWebhookConfig(ctx, uid); row.URL != "https://example.test/a" {
		t.Errorf("a refused upsert changed the url to %q", row.URL)
	}

	row, err := q.UpsertWebhookConfig(ctx, UpsertWebhookConfigParams{UserID: uid, URL: "https://example.test/c", Secret: "whsec_first"})
	if err != nil {
		t.Fatalf("upsert with the stored secret: %v", err)
	}
	if row.URL != "https://example.test/c" || row.Secret != "whsec_first" {
		t.Errorf("after update: url %q secret %q, want the new url and the first secret", row.URL, row.Secret)
	}
}
