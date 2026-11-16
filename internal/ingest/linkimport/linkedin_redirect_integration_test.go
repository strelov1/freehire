//go:build integration

package linkimport

import (
	"context"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/linksource"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/db"
)

func TestWriteRefusesGenericLinkedInJobWithoutPublishing(t *testing.T) {
	pool, ctx := startPostgres(t), context.Background()
	im := New(pool, db.New(pool), nil, pageClient{body: jobPostingPage}, nil, nil)
	_, ok, err := im.Write(ctx, linksource.Resolved{
		Source: linksource.GenericSource,
		Job:    sources.Job{ExternalID: "https://www.linkedin.com/jobs/view/4490679811/", URL: "https://www.linkedin.com/jobs/view/4490679811/", Title: "Java Engineer", Company: "Acme", Description: "Build Java services."},
	})
	if err == nil || ok {
		t.Fatalf("generic LinkedIn job bypassed application verification: ok=%t err=%v", ok, err)
	}
	var count int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM jobs").Scan(&count); err != nil || count != 0 {
		t.Fatalf("unverified LinkedIn jobs=%d error=%v", count, err)
	}
}
