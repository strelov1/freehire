//go:build integration

package testdb

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/platform/modroot"
)

// InstallLegacyJobActivity restores release A's observation columns and triggers
// in a disposable test database. Historical backfill/bridge/DDL-order regressions
// must keep exercising the real legacy writers after release C removes them.
func InstallLegacyJobActivity(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	root, err := modroot.Find()
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(root, "migrations", "0258_job_activity.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sql := string(body)
	start := strings.Index(sql, "CREATE FUNCTION job_activity_insert()")
	end := strings.Index(sql, "-- Target-row version")
	if start < 0 || end <= start {
		t.Fatal("historical activity trigger boundaries changed")
	}
	sql = strings.Replace(sql[start:end], "CREATE FUNCTION job_activity_insert()", "CREATE OR REPLACE FUNCTION job_activity_insert()", 1)
	_, err = pool.Exec(context.Background(), `
		ALTER TABLE jobs ADD COLUMN IF NOT EXISTS last_seen_at timestamptz NOT NULL DEFAULT now(),
		    ADD COLUMN IF NOT EXISTS hydrated_at timestamptz,
		    ADD COLUMN IF NOT EXISTS liveness_strikes integer NOT NULL DEFAULT 0;
		DROP TRIGGER IF EXISTS job_activity_insert ON jobs;
		DROP TRIGGER IF EXISTS job_activity_legacy_bridge ON jobs;
		DROP FUNCTION IF EXISTS job_activity_legacy_bridge();
	`+sql)
	if err != nil {
		t.Fatalf("restore historical release A observation schema: %v", err)
	}
}
