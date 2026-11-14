//go:build integration

package db

import (
	"context"
	"testing"
)

func TestInboxDigestDefaultsOff(t *testing.T) {
	pool := startPostgres(t)
	ctx := context.Background()
	uid := insertUser(t, pool, "digest-default@example.test")
	var enabled bool
	err := pool.QueryRow(ctx, "SELECT coalesce((SELECT inbox_digest_enabled FROM notification_settings WHERE user_id=$1),false)", uid).Scan(&enabled)
	if err != nil {
		t.Fatal(err)
	}
	if enabled {
		t.Fatal("inbox digest must require explicit consent")
	}
}
