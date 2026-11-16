//go:build integration

package linkimport

import (
	"context"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/db"
)

func TestWriteNativeReportsExistingOpenPostingAndPreservesReopening(t *testing.T) {
	pool, ctx := startPostgres(t), context.Background()
	q := db.New(pool)
	im := New(pool, q, nil, nil, sources.Taxonomy(), nil)
	native := NativeDestination{
		Entry: sources.CompanyEntry{Provider: "hrsmart", Board: "gistworld.mua.hrdepartment.com", Company: "Gist Limited"},
		Job:   sources.Job{ExternalID: "gistworld.mua.hrdepartment.com:3735", URL: "https://gistworld.mua.hrdepartment.com/hrsmart/ats/Posting/view/3735", Title: "Java Engineer", Company: "Gist Limited", Description: "Build Java services."},
	}
	first, err := im.WriteNative(ctx, nil, native)
	if err != nil || first.Deduped {
		t.Fatalf("new posting=%+v error=%v", first, err)
	}
	before, err := q.GetJobBySlug(ctx, first.PublicSlug)
	if err != nil {
		t.Fatal(err)
	}
	second, err := im.WriteNative(ctx, nil, native)
	if err != nil || !second.Deduped || second.PublicSlug != first.PublicSlug {
		t.Fatalf("already-carried posting=%+v error=%v", second, err)
	}
	if _, err := pool.Exec(ctx, "UPDATE jobs SET closed_at=now(),closed_reason='unseen' WHERE id=$1", before.ID); err != nil {
		t.Fatal(err)
	}
	third, err := im.WriteNative(ctx, nil, native)
	if err != nil || third.Deduped || third.PublicSlug != first.PublicSlug {
		t.Fatalf("rediscovered closed posting=%+v error=%v", third, err)
	}
	after, err := q.GetJobBySlug(ctx, first.PublicSlug)
	if err != nil || after.ID != before.ID || after.ClosedAt.Valid {
		t.Fatalf("reopening changed stable identity: id=%d->%d closed=%t error=%v", before.ID, after.ID, after.ClosedAt.Valid, err)
	}
}
