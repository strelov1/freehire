package cv

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestSanitizeAutopilotReportKeepsAValidRun(t *testing.T) {
	got, err := SanitizeAutopilotReport([]AutopilotEntry{
		{Requirement: " Kafka in production ", Status: AutopilotClosedBank, Note: " Reframed the payments bullet. "},
		{Requirement: "Team leadership", Status: AutopilotOpen},
		{Requirement: "Terraform", Status: AutopilotNotReached, Note: "Ran out of rounds."},
		{Requirement: "Python", Status: AutopilotClosedCandidate, Note: "Their words: migration scripts."},
	})
	if err != nil {
		t.Fatalf("sanitize: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("kept %d entries, want 4", len(got))
	}
	if got[0].Requirement != "Kafka in production" || got[0].Note != "Reframed the payments bullet." {
		t.Errorf("entry not trimmed: %+v", got[0])
	}
}

func TestSanitizeAutopilotReportRefusesAnUnknownStatus(t *testing.T) {
	_, err := SanitizeAutopilotReport([]AutopilotEntry{
		{Requirement: "Kafka", Status: "partially_closed"},
	})
	if err == nil {
		t.Fatal("an out-of-vocabulary status was accepted; it would persist a value nothing can render")
	}
	// The message is the model's only route to correcting itself inside the turn, so it
	// must name what is allowed rather than only what was wrong.
	for _, want := range []string{"partially_closed", string(AutopilotClosedBank), string(AutopilotOpen)} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

func TestSanitizeAutopilotReportRefusesAnEmptyRequirement(t *testing.T) {
	if _, err := SanitizeAutopilotReport([]AutopilotEntry{
		{Requirement: "   ", Status: AutopilotOpen},
	}); err == nil {
		t.Error("an entry naming no requirement was accepted; the panel would render a blank row")
	}
}

func TestSanitizeAutopilotReportBoundsTextAndCount(t *testing.T) {
	long := strings.Repeat("x", 5000)
	got, err := SanitizeAutopilotReport([]AutopilotEntry{
		{Requirement: long, Status: AutopilotOpen, Note: long},
	})
	if err != nil {
		t.Fatalf("sanitize: %v", err)
	}
	if len([]rune(got[0].Requirement)) > maxAutopilotRequirement {
		t.Errorf("requirement kept %d runes, want it bounded to %d", len([]rune(got[0].Requirement)), maxAutopilotRequirement)
	}
	if len([]rune(got[0].Note)) > maxAutopilotNote {
		t.Errorf("note kept %d runes, want it bounded to %d", len([]rune(got[0].Note)), maxAutopilotNote)
	}

	// A report is one entry per requirement of one vacancy. An unbounded list would be
	// replayed into the model's context on every later turn of the session.
	many := make([]AutopilotEntry, maxAutopilotEntries+1)
	for i := range many {
		many[i] = AutopilotEntry{Requirement: "req", Status: AutopilotOpen}
	}
	if _, err := SanitizeAutopilotReport(many); err == nil {
		t.Errorf("a report of %d entries was accepted; want it refused above %d", len(many), maxAutopilotEntries)
	}
}

func TestSanitizeAutopilotReportRefusesAnEmptyReport(t *testing.T) {
	if _, err := SanitizeAutopilotReport(nil); err == nil {
		t.Error("an empty report was accepted; a run that considered nothing has nothing to report")
	}
}

// The Store-level behaviour: a report is written whole, and reverting takes the report with
// the document — a log describing edits that no longer exist misdescribes the CV.
func TestRevertAutopilotRestoresTheDocumentAndClearsTheReport(t *testing.T) {
	repo := newFakeRepo()
	s := NewStore(repo)
	ctx := context.Background()

	before, err := s.Create(ctx, 3, "Tailored", "classic-ats", Document{Summary: "before the run"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := s.SnapshotForAutopilot(ctx, before.ID, 3); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if _, err := s.Update(ctx, before.ID, 3, "Tailored", "classic-ats", Document{Summary: "after the run"}); err != nil {
		t.Fatalf("update: %v", err)
	}
	if err := s.SetAutopilotReport(ctx, before.ID, 3, []AutopilotEntry{
		{Requirement: "Kafka", Status: AutopilotClosedBank, Note: "reframed"},
	}); err != nil {
		t.Fatalf("set report: %v", err)
	}

	rec, err := s.Get(ctx, before.ID, 3)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !rec.AutopilotRevertable || len(rec.AutopilotReport) != 1 {
		t.Fatalf("read back revertable=%v report=%d, want a revertable run with one entry", rec.AutopilotRevertable, len(rec.AutopilotReport))
	}

	if _, err := s.RevertAutopilot(ctx, before.ID, 3); err != nil {
		t.Fatalf("revert: %v", err)
	}
	after, err := s.Get(ctx, before.ID, 3)
	if err != nil {
		t.Fatalf("get after revert: %v", err)
	}
	if after.Document.Summary != "before the run" {
		t.Errorf("summary = %q, want the pre-run document", after.Document.Summary)
	}
	if after.AutopilotRevertable || len(after.AutopilotReport) != 0 {
		t.Errorf("after revert: revertable=%v report=%d, want both cleared", after.AutopilotRevertable, len(after.AutopilotReport))
	}
}

func TestRevertAutopilotWithoutARunIsRefused(t *testing.T) {
	repo := newFakeRepo()
	s := NewStore(repo)
	ctx := context.Background()

	meta, err := s.Create(ctx, 3, "Tailored", "classic-ats", Document{Summary: "untouched"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := s.RevertAutopilot(ctx, meta.ID, 3); !errors.Is(err, ErrNoAutopilotRun) {
		t.Errorf("revert without a run = %v, want ErrNoAutopilotRun", err)
	}
	rec, err := s.Get(ctx, meta.ID, 3)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if rec.Document.Summary != "untouched" {
		t.Errorf("summary = %q, want the document left alone", rec.Document.Summary)
	}
}

// A foreign caller must not be able to write a report onto someone else's CV, and the
// refusal reads as "not found" rather than "forbidden".
func TestAutopilotWritesAreOwnerScoped(t *testing.T) {
	repo := newFakeRepo()
	s := NewStore(repo)
	ctx := context.Background()

	meta, err := s.Create(ctx, 3, "Tailored", "classic-ats", Document{Summary: "mine"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := s.SetAutopilotReport(ctx, meta.ID, 4, []AutopilotEntry{
		{Requirement: "Kafka", Status: AutopilotOpen},
	}); !errors.Is(err, ErrNotFound) {
		t.Errorf("foreign report write = %v, want ErrNotFound", err)
	}
	if err := s.SnapshotForAutopilot(ctx, meta.ID, 4); !errors.Is(err, ErrNotFound) {
		t.Errorf("foreign snapshot = %v, want ErrNotFound", err)
	}
	if _, err := s.RevertAutopilot(ctx, meta.ID, 4); !errors.Is(err, ErrNoAutopilotRun) {
		t.Errorf("foreign revert = %v, want ErrNoAutopilotRun", err)
	}
}
