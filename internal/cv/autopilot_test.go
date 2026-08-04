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

// cv_edit and tailor_report write two different columns through two different tool calls,
// with nothing to keep them in step — a model that edits the document without a follow-up
// tailor_report call leaves a requirement marked open when the CV already closes it.
// MergeAutopilotEntry is what lets cv_edit self-report the one requirement it just closed,
// in the same call, without depending on the model to remember a second one.
//
// Merging into an entry the report already holds must replace it, not duplicate it — the
// report is one entry per requirement, and a second row for the same requirement would
// leave the panel showing both an old "open" and a new "closed_bank" for the same line.
func TestMergeAutopilotEntryReplacesAMatchingRequirement(t *testing.T) {
	repo := newFakeRepo()
	s := NewStore(repo)
	ctx := context.Background()

	meta, err := s.Create(ctx, 1, "Tailored", "classic-ats", Document{Summary: "mine"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := s.SetAutopilotReport(ctx, meta.ID, 1, []AutopilotEntry{
		{Requirement: "PostgreSQL experience", Status: AutopilotOpen},
		{Requirement: "Team leadership", Status: AutopilotClosedBank},
	}); err != nil {
		t.Fatalf("seed report: %v", err)
	}

	if err := s.MergeAutopilotEntry(ctx, meta.ID, 1, AutopilotEntry{
		Requirement: "  PostgreSQL experience  ", // matched case/whitespace-insensitively
		Status:      AutopilotClosedBank,
		Note:        "Added the RDS PostgreSQL bullet.",
	}); err != nil {
		t.Fatalf("merge: %v", err)
	}

	rec, err := s.Get(ctx, meta.ID, 1)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(rec.AutopilotReport) != 2 {
		t.Fatalf("report has %d entries, want 2 (merge must replace, not duplicate)", len(rec.AutopilotReport))
	}
	got := rec.AutopilotReport[0]
	if got.Status != AutopilotClosedBank || got.Note != "Added the RDS PostgreSQL bullet." {
		t.Errorf("merged entry = %+v, want status closed_bank with the new note", got)
	}
	if rec.AutopilotReport[1].Requirement != "Team leadership" || rec.AutopilotReport[1].Status != AutopilotClosedBank {
		t.Errorf("unrelated entry was disturbed: %+v", rec.AutopilotReport[1])
	}
}

// A requirement cv_edit closes in the same turn it was first read — before any
// tailor_report call ever ran — has no matching entry to replace. It must be appended
// rather than silently dropped, or the very first requirement closed in a session would
// never show up in the report.
func TestMergeAutopilotEntryAppendsWhenNoReportExistsYet(t *testing.T) {
	repo := newFakeRepo()
	s := NewStore(repo)
	ctx := context.Background()

	meta, err := s.Create(ctx, 1, "Tailored", "classic-ats", Document{Summary: "mine"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := s.MergeAutopilotEntry(ctx, meta.ID, 1, AutopilotEntry{
		Requirement: "PostgreSQL experience",
		Status:      AutopilotClosedCandidate,
	}); err != nil {
		t.Fatalf("merge: %v", err)
	}

	rec, err := s.Get(ctx, meta.ID, 1)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(rec.AutopilotReport) != 1 || rec.AutopilotReport[0].Requirement != "PostgreSQL experience" {
		t.Fatalf("report = %+v, want one appended entry", rec.AutopilotReport)
	}
}

// The merge is owner-scoped the same way every other CV write is: a foreign id must not
// let one candidate's tool call touch another's report.
func TestMergeAutopilotEntryIsOwnerScoped(t *testing.T) {
	repo := newFakeRepo()
	s := NewStore(repo)
	ctx := context.Background()

	meta, err := s.Create(ctx, 3, "Tailored", "classic-ats", Document{Summary: "mine"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := s.MergeAutopilotEntry(ctx, meta.ID, 4, AutopilotEntry{
		Requirement: "Kafka", Status: AutopilotClosedBank,
	}); !errors.Is(err, ErrNotFound) {
		t.Errorf("foreign merge = %v, want ErrNotFound", err)
	}
}

// The Store-level behaviour: a report is written whole, and reverting takes the report with
// the document — a log describing edits that no longer exist misdescribes the CV.
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
}
