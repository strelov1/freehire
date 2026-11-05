//go:build integration

package worker

import (
	"context"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/platform/testdb"
)

func TestAnalyticsReaderIsReadOnlyWithoutChangingWriter(t *testing.T) {
	t.Setenv("ANALYTICS_DATABASE_URL", "")
	ctx := context.Background()
	primary := testdb.Pool(t)
	reader, err := AnalyticsPool(ctx, primary)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if _, err := reader.Exec(ctx, `INSERT INTO job_daily_stats(day,added,removed) VALUES ('2026-01-01',1,0)`); err == nil {
		t.Fatal("analytics connection permitted a write")
	}
	if _, err := primary.Exec(ctx, `INSERT INTO job_daily_stats(day,added,removed) VALUES ('2026-01-01',1,0)`); err != nil {
		t.Fatalf("writer became read-only: %v", err)
	}
}

func TestAnalyticsRejectsPrimaryWhenReplicaConfigured(t *testing.T) {
	primary := testdb.Pool(t)
	t.Setenv("ANALYTICS_DATABASE_URL", primary.Config().ConnConfig.ConnString())
	reader, err := AnalyticsPool(context.Background(), primary)
	if reader != nil {
		reader.Close()
		t.Fatal("configured primary accepted as replica")
	}
	if err == nil || !strings.Contains(err.Error(), "standby") {
		t.Fatalf("error=%v, want standby refusal", err)
	}
}
