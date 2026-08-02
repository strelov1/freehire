package apptimeline

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/appevent"
	"github.com/strelov1/freehire/internal/db"
)

// stubStore answers with a fixed set of rows, so the service's own rules — validation and
// the observed verdict — are tested without a pool.
type stubStore struct {
	rows            []db.ListApplicationEventsInRangeRow
	interviews      []db.ListApplicationInterviewsInRangeRow
	applicationRows []db.ListApplicationEventsRow
}

func (s stubStore) ListApplicationEventsInRange(context.Context, db.ListApplicationEventsInRangeParams) ([]db.ListApplicationEventsInRangeRow, error) {
	return s.rows, nil
}

func row(source string) db.ListApplicationEventsInRangeRow {
	return db.ListApplicationEventsInRangeRow{
		ID:         1,
		Kind:       appevent.KindEmployerReply,
		Source:     source,
		OccurredAt: pgtype.Timestamptz{Time: time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC), Valid: true},
	}
}

// Every case here passes a nil store. Validation that reaches the database first is
// validation the in-process caller pays for on every mistyped bound, and a panic here is
// the proof that it did.
func TestRangeRefusesBoundsItCannotAnswerFor(t *testing.T) {
	day := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name     string
		from, to time.Time
	}{
		{"no bounds at all", time.Time{}, time.Time{}},
		{"no lower bound", time.Time{}, day},
		{"no upper bound", day, time.Time{}},
		{"inverted", day.AddDate(0, 0, 1), day},
		{"longer than the cap", day, day.AddDate(0, 0, MaxRangeDays+1)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := New(nil).Range(context.Background(), 1, tc.from, tc.to)
			if !errors.Is(err, ErrInvalidRange) {
				t.Errorf("Range(%v, %v) error = %v, want one wrapping ErrInvalidRange", tc.from, tc.to, err)
			}
		})
	}
}

// The cap bounds one request, not the calendar: a month with a day of margin either side,
// and a full year, both have to fit through it.
func TestRangeAcceptsTheSpansTheCalendarAsksFor(t *testing.T) {
	day := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	cases := map[string]time.Time{
		"a single instant": day,
		"a padded month":   day.AddDate(0, 1, 2),
		"the cap exactly":  day.AddDate(0, 0, MaxRangeDays),
	}
	for name, to := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := New(stubStore{}).Range(context.Background(), 1, day, to); err != nil {
				t.Errorf("Range over %s returned %v, want no error", name, err)
			}
		})
	}
}

// The verdict must be appevent's, not a copy of it. A second list of trusted sources would
// still call a newly added one observed after appevent had already refused it — and the
// calendar draws that difference as the difference between a date somebody set and a date
// the candidate typed. Iterating the whole vocabulary is what makes a hardcoded copy fail.
func TestObservedIsAppeventsVerdictAndNotACopy(t *testing.T) {
	for _, src := range appevent.Sources {
		t.Run(src, func(t *testing.T) {
			events, err := New(stubStore{rows: []db.ListApplicationEventsInRangeRow{row(src)}}).
				Range(context.Background(), 1, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
					time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC))
			if err != nil {
				t.Fatalf("Range: %v", err)
			}
			if len(events) != 1 {
				t.Fatalf("got %d events, want 1", len(events))
			}
			if got, want := events[0].Observed, appevent.TrustedForDayMath(src); got != want {
				t.Errorf("Observed for source %q = %v, want %v", src, got, want)
			}
		})
	}
}

// An unknown source is untrusted for the same reason appevent gives: unknown provenance
// must never read as an observation. The calendar would otherwise draw it filled.
func TestAnUnknownSourceIsNotObserved(t *testing.T) {
	events, err := New(stubStore{rows: []db.ListApplicationEventsInRangeRow{row("mail_carrier_pigeon")}}).
		Range(context.Background(), 1, time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Range: %v", err)
	}
	if events[0].Observed {
		t.Error("an event from an unrecognised source reported as observed")
	}
}

func (s stubStore) ListApplicationInterviewsInRange(context.Context, db.ListApplicationInterviewsInRangeParams) ([]db.ListApplicationInterviewsInRangeRow, error) {
	return s.interviews, nil
}

func (s stubStore) ListApplicationEvents(context.Context, db.ListApplicationEventsParams) ([]db.ListApplicationEventsRow, error) {
	return s.applicationRows, nil
}

func applicationRow(source, subject string) db.ListApplicationEventsRow {
	return db.ListApplicationEventsRow{
		ID:           7,
		Kind:         appevent.KindEmployerReply,
		Source:       source,
		OccurredAt:   pgtype.Timestamptz{Time: time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC), Valid: true},
		CompanySlug:  "derq",
		EmailSubject: pgtype.Text{String: subject, Valid: subject != ""},
	}
}

// One application's history is the same Event the calendar renders, resolved by the same
// rules — including the observed verdict, which is appevent's to give and this package's to
// ask for. A second mapping would be a second chance to disagree about what an event is.
func TestForApplicationMapsRowsThroughTheSameRules(t *testing.T) {
	svc := New(stubStore{applicationRows: []db.ListApplicationEventsRow{
		applicationRow(appevent.SourceMailGmail, "Invitation to interview"),
	}})

	events, err := svc.ForApplication(context.Background(), 1, 2)
	if err != nil {
		t.Fatalf("ForApplication: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("got %d events, want 1", len(events))
	}
	got := events[0]
	if got.ID != 7 || got.Kind != appevent.KindEmployerReply || got.CompanySlug != "derq" {
		t.Errorf("event = %+v, want the row's own id, kind and employer", got)
	}
	if got.EmailSubject != "Invitation to interview" {
		t.Errorf("subject = %q, want the message's own", got.EmailSubject)
	}
	if !got.Observed {
		t.Error("a gmail-sourced event is not observed; the trust rule is appevent's and this reads it")
	}
}

// Deletion hides content and does not un-happen the reply: the query withholds the subject
// and keeps the event, and the mapping must not turn that into a dropped row.
func TestForApplicationKeepsAnEventWhoseMessageIsGone(t *testing.T) {
	svc := New(stubStore{applicationRows: []db.ListApplicationEventsRow{
		applicationRow(appevent.SourceMailGmail, ""),
	}})

	events, err := svc.ForApplication(context.Background(), 1, 2)
	if err != nil {
		t.Fatalf("ForApplication: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("got %d events, want the event to stand without its message", len(events))
	}
	if events[0].EmailSubject != "" {
		t.Errorf("subject = %q, want none", events[0].EmailSubject)
	}
}
