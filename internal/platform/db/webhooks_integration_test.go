//go:build integration

// Integration tests for the webhook_configs auto-disable threshold — the
// RETURNING ... CASE WHEN in RecordWebhookDeliveryFailure can only be verified
// against a real Postgres. Run with: go test -tags=integration ./internal/platform/db/
// Requires Docker (testcontainers spins up a throwaway Postgres with the migrations).
package db

import (
	"context"
	"testing"
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
	if _, err := q.UpsertWebhookConfig(ctx, UpsertWebhookConfigParams{UserID: uid, URL: "https://example.test/hook"}); err != nil {
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
	if _, err := q.UpsertWebhookConfig(ctx, UpsertWebhookConfigParams{UserID: uid, URL: "https://example.test/hook"}); err != nil {
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
