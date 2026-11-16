//go:build integration

package linkimport

import (
	"context"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/boardcatalog"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/db"
)

func TestLinkedInReadOnlyResolveThenWriteKeepsNativePollingOwnership(t *testing.T) {
	pool, ctx := startPostgres(t), context.Background()
	q := db.New(pool)
	const target = "https://gistworld.mua.hrdepartment.com/hrsmart/ats/Posting/view/3735"
	posting := sources.Job{ExternalID: "123", Title: "Java Engineer", Company: "Gist Limited", Description: "Build Java services.", ApplyMethod: sources.ApplyExternal, ExternalApplyURL: target, CompanyWebsite: "https://www.gistworld.com"}
	registry := map[string]sources.Source{
		"linkedin": linkedInTestSource{posting: posting},
		"hrsmart":  nativeTestSource{jobs: []sources.Job{{ExternalID: "3735", URL: target, Title: posting.Title, Company: posting.Company, Description: posting.Description}}},
	}
	im := New(pool, q, nil, nil, registry, nativeEmployerProof{})
	resolved, ok, err := im.Resolve(ctx, "https://www.linkedin.com/jobs/view/123/", Board{})
	if err != nil || !ok || resolved.Source != "hrsmart" {
		t.Fatalf("native read-only resolution: %+v ok=%t error=%v", resolved, ok, err)
	}
	if _, found, err := boardcatalog.NewQueriesRepository(q).Entry(ctx, "hrsmart", "gistworld.mua.hrdepartment.com"); err != nil || found {
		t.Fatalf("read-only resolution changed board catalogue: found=%t error=%v", found, err)
	}
	if _, ok, err := im.Write(ctx, resolved); err != nil || !ok {
		t.Fatalf("native write: ok=%t error=%v", ok, err)
	}
	entry, found, err := boardcatalog.NewQueriesRepository(q).Entry(ctx, "hrsmart", "gistworld.mua.hrdepartment.com")
	if err != nil || !found || entry.Company != posting.Company {
		t.Fatalf("native polling lost after resolve/write: %+v found=%t error=%v", entry, found, err)
	}
	cfg, err := q.GetIngestSchedule(ctx, "hrsmart")
	if err != nil || !cfg.Enabled || !cfg.Managed {
		t.Fatalf("native provider is not scheduled: %+v error=%v", cfg, err)
	}
}
