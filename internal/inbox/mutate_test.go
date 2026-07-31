package inbox

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/mailclassify"
)

// mailclassify.Sanitize coerces an out-of-vocabulary label to `other` because the
// classification worker feeds it raw model output derived from an attacker's email
// body. This path is the opposite case: the label is a judgement the caller asked
// for, and quietly rewriting a typo to `other` records a verdict nobody chose while
// looking like success.
func TestTriageRefusesAnUnknownLabelInsteadOfCoercingIt(t *testing.T) {
	q := &fakeQueries{}

	_, err := New(q, nil).Triage(context.Background(), 7, 812, Verdict{Signal: "ghosted"})

	var invalid *InvalidError
	if !errors.As(err, &invalid) {
		t.Fatalf("Triage with a bogus label = %v, want an *InvalidError", err)
	}
	if q.triaged != 0 {
		t.Error("Triage wrote a classification for a label it refused")
	}
	for _, want := range mailclassify.SignalValues {
		if !contains(invalid.Valid, want) {
			t.Errorf("the error omits the valid label %q; it is the model's only route to self-correction", want)
		}
	}
}

// Omitting a slug means "I am not deciding the link", not "clear it": a
// classify-only pass must not silently detach an application. Clearing stays the
// explicit Unlink.
func TestTriageWithoutASlugLeavesTheLinkAlone(t *testing.T) {
	q := &fakeQueries{}

	if _, err := New(q, nil).Triage(context.Background(), 7, 812, Verdict{Signal: "rejection"}); err != nil {
		t.Fatalf("Triage: %v", err)
	}
	if q.lastTriage.JobID.Valid {
		t.Error("Triage without a slug still named a job; the link must be left untouched")
	}
}

// The slug is resolved before anything is written, so an unknown one changes
// nothing at all.
func TestTriageWithAnUnknownSlugWritesNothing(t *testing.T) {
	q := &fakeQueries{jobErr: errNoRows}

	_, err := New(q, nil).Triage(context.Background(), 7, 812, Verdict{Signal: "rejection", Slug: "nope"})

	if err == nil {
		t.Fatal("Triage with an unknown slug succeeded")
	}
	if q.triaged != 0 {
		t.Error("Triage wrote a classification for a slug it could not resolve")
	}
}

// A linked verdict that implies progress moves the application forward.
func TestTriageAdvancesTheStageOfALinkedApplication(t *testing.T) {
	q := &fakeQueries{job: db.Job{ID: 42}, stage: "applied"}

	if _, err := New(q, nil).Triage(context.Background(), 7, 812, Verdict{Signal: "interview_invitation", Slug: "go-dev-acme"}); err != nil {
		t.Fatalf("Triage: %v", err)
	}
	if q.advancedTo == "" {
		t.Fatal("an interview invitation on a linked application advanced no stage")
	}
	if q.advancedTo == "applied" {
		t.Error("the stage was rewritten to itself")
	}
}

// The verdict is already durable by the time the stage is considered, so a failed
// advance must not fail the triage the caller successfully recorded.
func TestTriageSurvivesAFailedStageAdvance(t *testing.T) {
	q := &fakeQueries{job: db.Job{ID: 42}, stage: "applied", advanceErr: errors.New("deadlock")}

	if _, err := New(q, nil).Triage(context.Background(), 7, 812, Verdict{Signal: "offer", Slug: "go-dev-acme"}); err != nil {
		t.Fatalf("Triage failed because the stage advance did: %v", err)
	}
}

// Mail linked to a job the caller does not track has no stage to advance — that is
// not an error, it is simply nothing to do.
func TestTriageIgnoresAnUntrackedJob(t *testing.T) {
	q := &fakeQueries{job: db.Job{ID: 42}, stageErr: errNoRows}

	if _, err := New(q, nil).Triage(context.Background(), 7, 812, Verdict{Signal: "offer", Slug: "go-dev-acme"}); err != nil {
		t.Fatalf("Triage: %v", err)
	}
}

// A message that is not the caller's matches no row, and is reported as missing
// rather than as forbidden.
func TestMutationsOnAnotherUsersMailAreNotFound(t *testing.T) {
	svc := New(&fakeQueries{noRows: true}, nil)
	ctx := context.Background()

	for name, call := range map[string]func() error{
		"unlink":  func() error { _, err := svc.Unlink(ctx, 7, 812); return err },
		"confirm": func() error { _, err := svc.ResolveSuggestion(ctx, 7, 812, true); return err },
		"reject":  func() error { _, err := svc.ResolveSuggestion(ctx, 7, 812, false); return err },
		"link":    func() error { _, err := svc.Link(ctx, 7, 812, "go-dev-acme"); return err },
	} {
		if err := call(); !errors.Is(err, ErrNotFound) {
			t.Errorf("%s on another user's mail = %v, want ErrNotFound", name, err)
		}
	}
}

// The matcher has already proposed an answer; letting this path overwrite it
// silently would leave the resulting link's provenance ambiguous.
func TestRecordApplicationRefusesMailWithAPendingSuggestion(t *testing.T) {
	q := &fakeQueries{email: db.GetEmailRow{
		ID:             812,
		SuggestedJobID: pgtype.Int8{Int64: 9, Valid: true},
	}}
	apps := &fakeApps{}

	_, err := New(q, apps).RecordApplication(context.Background(), 7, 812, "go-dev-acme")

	if !errors.Is(err, ErrPendingSuggestion) {
		t.Fatalf("RecordApplication over a pending suggestion = %v, want ErrPendingSuggestion", err)
	}
	if apps.calls != 0 {
		t.Error("an application was recorded despite the refusal")
	}
}

// The application demonstrably existed by the time the employer wrote, so the
// message's timestamp is an honest upper bound. now() would over-report elapsed
// silence and tell someone they were ignored when they were not.
func TestRecordApplicationIsDatedByTheMail(t *testing.T) {
	wrote := time.Date(2026, 3, 4, 9, 0, 0, 0, time.UTC)
	q := &fakeQueries{
		email: db.GetEmailRow{ID: 812, ReceivedAt: pgtype.Timestamptz{Time: wrote, Valid: true}},
		job:   db.Job{ID: 42},
	}
	apps := &fakeApps{}

	if _, err := New(q, apps).RecordApplication(context.Background(), 7, 812, "go-dev-acme"); err != nil {
		t.Fatalf("RecordApplication: %v", err)
	}
	if !apps.at.Equal(wrote) {
		t.Errorf("application dated %v, want the message's %v", apps.at, wrote)
	}
}

// The overview is what lets a broad question be answered without reading the
// mailbox, so it must name every label the vocabulary has — including the ones at
// zero. "No interview invitations" and "no such label" are different answers.
func TestOverviewNamesEveryLabelIncludingTheEmptyOnes(t *testing.T) {
	q := &fakeQueries{state: []db.CountEmailsByStateRow{
		{Label: "rejection", N: 12, Unread: 2, Unclassified: 0, Linked: 9, Suggested: 1},
		{Label: "interview_invitation", N: 3, Unread: 3, Linked: 3},
		{Label: "", N: 5, Unread: 5, Unclassified: 5},
	}}

	got, err := New(q, nil).Overview(context.Background(), 7)
	if err != nil {
		t.Fatalf("Overview: %v", err)
	}

	if len(got.Labels) != len(mailclassify.SignalValues) {
		t.Fatalf("Overview reported %d labels, want all %d", len(got.Labels), len(mailclassify.SignalValues))
	}
	byLabel := map[string]int64{}
	for _, l := range got.Labels {
		byLabel[l.Label] = l.Count
	}
	for _, want := range mailclassify.SignalValues {
		if _, ok := byLabel[want]; !ok {
			t.Errorf("Overview omits the label %q", want)
		}
	}
	switch {
	case byLabel["rejection"] != 12:
		t.Errorf("rejection = %d, want 12", byLabel["rejection"])
	case byLabel["offer"] != 0:
		t.Errorf("offer = %d, want 0", byLabel["offer"])
	case got.Total != 20:
		t.Errorf("Total = %d, want 20 (the rows summed)", got.Total)
	case got.Unread != 10:
		t.Errorf("Unread = %d, want 10", got.Unread)
	case got.Unclassified != 5:
		t.Errorf("Unclassified = %d, want 5", got.Unclassified)
	case got.Linked != 12:
		t.Errorf("Linked = %d, want 12", got.Linked)
	case got.Suggested != 1:
		t.Errorf("Suggested = %d, want 1", got.Suggested)
	case got.Unlinked != 7:
		t.Errorf("Unlinked = %d, want 7 (total less linked less suggested)", got.Unlinked)
	}
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

type fakeApps struct {
	calls int
	at    time.Time
}

func (f *fakeApps) MarkAppliedAt(_ context.Context, _ int64, _ string, at time.Time) error {
	f.calls++
	f.at = at
	return nil
}
