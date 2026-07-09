package job_test

import (
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/job"
)

func openJob(t *testing.T) job.Job {
	t.Helper()
	j, err := job.New(job.Draft{Source: "manual", ExternalID: "1", Title: "Engineer", Company: "Acme", Description: "We use Golang."})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return j
}

// Close soft-closes an open job without destroying its identity or facets, and is
// idempotent: closing an already-closed job preserves the original closed_at.
func TestClose_IdempotentAndNonDestructive(t *testing.T) {
	j := openJob(t)
	slug := j.Fields().PublicSlug
	skills := j.Fields().Skills

	t0 := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	j.Close(t0)
	if j.IsOpen() {
		t.Fatal("job should be closed after Close")
	}
	f := j.Fields()
	if f.ClosedAt == nil || !f.ClosedAt.Equal(t0) {
		t.Errorf("ClosedAt = %v, want %v", f.ClosedAt, t0)
	}
	if f.PublicSlug != slug {
		t.Errorf("Close changed PublicSlug: %q != %q", f.PublicSlug, slug)
	}
	if len(f.Skills) != len(skills) {
		t.Errorf("Close changed Skills: %v", f.Skills)
	}

	// Second close with a later time is a no-op — the original timestamp stands.
	j.Close(t0.Add(48 * time.Hour))
	if got := j.Fields().ClosedAt; got == nil || !got.Equal(t0) {
		t.Errorf("idempotent Close moved ClosedAt to %v, want %v", got, t0)
	}
}

// Reopen clears the closed state so the job serves on list/search again.
func TestReopen_ClearsClosedState(t *testing.T) {
	j := openJob(t)
	j.Close(time.Now())
	j.Reopen()
	if !j.IsOpen() {
		t.Error("job should be open after Reopen")
	}
	if j.Fields().ClosedAt != nil {
		t.Errorf("Reopen left ClosedAt = %v", j.Fields().ClosedAt)
	}
}

// ShouldEnrich mirrors the enrichment queue's SQL guard: open AND below the target
// schema version.
func TestShouldEnrich(t *testing.T) {
	const target int32 = 1

	fresh := openJob(t) // version 0, open
	if !fresh.ShouldEnrich(target) {
		t.Error("open, below-version job should be enrichment-eligible")
	}

	closed := openJob(t)
	closed.Close(time.Now())
	if closed.ShouldEnrich(target) {
		t.Error("closed job must not be enrichment-eligible")
	}
}
