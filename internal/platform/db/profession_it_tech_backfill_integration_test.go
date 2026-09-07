//go:build integration

// Integration test for the cmd/backfill-profession-it-tech query (issue #2601). Like
// the clearance backfill, idempotency rests on the UPDATE's IS DISTINCT FROM guard — a
// SQL behaviour, so only a real Postgres can confirm it.
// Run with: go test -tags=integration ./internal/platform/db/
package db

import (
	"context"
	"testing"
)

// TestBackfillProfessionITBoardTech pins that the backfill sets is_tech only on
// Profession itdev/itops rows, leaves everything else untouched, and is a no-op on a
// second run.
func TestBackfillProfessionITBoardTech(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	truncate(t, pool)

	itdev := ingestParams("itdev:1", "Windows rendszermérnök")
	itdev.Source = "profession"
	itdev.Category = ""
	itops := ingestParams("itops:1", "Junior rendszergazda")
	itops.Source = "profession"
	itops.Category = ""
	// A Profession posting from a general-population board (never crawled today, but
	// the predicate must not assume it never will be) should not be touched.
	otherBoard := ingestParams("hr:1", "HR Assistant")
	otherBoard.Source = "profession"
	otherBoard.Category = ""
	// Another source whose external_id happens to start with "itdev:" must not match —
	// the predicate is scoped by source, not by prefix alone.
	otherSource := ingestParams("itdev:1", "Some Role")
	otherSource.Source = "greenhouse"
	otherSource.Category = ""
	otherSource.PublicSlug = "pslug-other-source-itdev-1"

	for _, p := range []UpsertJobParams{itdev, itops, otherBoard, otherSource} {
		if _, err := ingestUpsert(ctx, q, p); err != nil {
			t.Fatalf("upsert %s/%s: %v", p.Source, p.ExternalID, err)
		}
	}

	n, err := q.BackfillProfessionITBoardTech(ctx)
	if err != nil {
		t.Fatalf("first backfill: %v", err)
	}
	if n != 2 {
		t.Fatalf("first backfill affected %d rows, want 2 (itdev + itops)", n)
	}

	for _, want := range []struct {
		source, externalID string
		wantTech           bool
	}{
		{"profession", "itdev:1", true},
		{"profession", "itops:1", true},
		{"profession", "hr:1", false},
		{"greenhouse", "itdev:1", false},
	} {
		job, err := q.GetJobBySourceExternalID(ctx, GetJobBySourceExternalIDParams{Source: want.source, ExternalID: want.externalID})
		if err != nil {
			t.Fatalf("read back %s/%s: %v", want.source, want.externalID, err)
		}
		if got := job.IsTech.Valid && job.IsTech.Bool; got != want.wantTech {
			t.Errorf("%s/%s is_tech = %+v, want valid=true bool=%v", want.source, want.externalID, job.IsTech, want.wantTech)
		}
	}

	n, err = q.BackfillProfessionITBoardTech(ctx)
	if err != nil {
		t.Fatalf("second backfill: %v", err)
	}
	if n != 0 {
		t.Fatalf("second backfill affected %d rows, want 0 — the guard did not hold", n)
	}
}
