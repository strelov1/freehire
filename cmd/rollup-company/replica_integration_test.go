//go:build integration

package main

import (
	"context"
	"testing"

	"github.com/strelov1/freehire/internal/platform/testdb"
)

func TestCompanyRollupReadsAnalyticsAndWritesPrimary(t *testing.T) {
	ctx := context.Background()
	analytics, primary := testdb.Pool(t), testdb.Pool(t)
	if _, err := analytics.Exec(ctx, `INSERT INTO jobs(public_slug,source,external_id,title,url,company,company_slug,created_at) VALUES ('test-company','test','company','Engineer','https://example.com/company','Example','example',now()-interval '60 days')`); err != nil {
		t.Fatal(err)
	}
	if _, err := analytics.Exec(ctx, `ALTER DATABASE "`+analytics.Config().ConnConfig.Database+`" SET default_transaction_read_only=on`); err != nil {
		t.Fatal(err)
	}
	analytics.Reset()
	if err := rebuildCompanyRollups(ctx, analytics, primary); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := primary.QueryRow(ctx, `SELECT open_count FROM insights_company_growth WHERE company_slug='example'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("company count=%d, want 1", count)
	}
	if err := analytics.QueryRow(ctx, `SELECT count(*) FROM insights_company_growth`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("analytics database was written")
	}
}
