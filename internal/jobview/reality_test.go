package jobview

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
)

func TestClassifyReality_MapsConvergentSignalsToVerdict(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	j := db.Job{
		CreatedAt:   pgtype.Timestamptz{Time: now.Add(-240 * 24 * time.Hour), Valid: true},
		PostedAt:    pgtype.Timestamptz{Time: now.Add(-1 * 24 * time.Hour), Valid: true},
		Description: "We are always hiring — join our talent community.",
	}
	// Reposted 6× historically; not mass-posted concurrently.
	r := ClassifyReality(j, now, 6, 1)

	if r.Class != "likely-evergreen" {
		t.Errorf("class = %q, want likely-evergreen", r.Class)
	}
	if r.AgeDays != 240 {
		t.Errorf("ageDays = %d, want 240", r.AgeDays)
	}
	if r.RepostCount != 6 {
		t.Errorf("repostCount = %d, want 6", r.RepostCount)
	}
	if !r.FakeFreshness {
		t.Error("expected FakeFreshness (posted 1d over first-seen 240d)")
	}
}

func TestClassifyReality_FreshJobHasNoEvidence(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	j := db.Job{
		CreatedAt:   pgtype.Timestamptz{Time: now.Add(-2 * 24 * time.Hour), Valid: true},
		PostedAt:    pgtype.Timestamptz{Time: now.Add(-2 * 24 * time.Hour), Valid: true},
		Description: "Own the checkout service. Apply by Friday.",
	}
	r := ClassifyReality(j, now, 1, 1)
	if r.Class != "fresh" {
		t.Errorf("class = %q, want fresh", r.Class)
	}
	if r.FakeFreshness {
		t.Error("did not expect FakeFreshness on a genuinely new job")
	}
}
