// Package apptimeline reads the application-event ledger as a dated series: one caller's
// events over a range, oldest first.
//
// It is the ledger's first dated reader. Until now the table was written by every path
// that moves an application and read only by the per-company aggregate in insights.sql,
// which groups by employer and never asks when.
//
// It is not a method on internal/jobtracking. That package is organised around mutations
// of one (user, job) pair; this is a range read across every application a caller has,
// joining two tables jobtracking never touches, with no mutation behind it.
//
// The rules live here rather than in the Fiber handler for the reason internal/inbox
// states: the in-app assistant calls services directly with the session owner's id and
// issues no HTTP request, so a rule enforced in a handler is a rule that reader never
// meets. "What happened last month" is an obvious tool for it.
package apptimeline

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/appevent"
	"github.com/strelov1/freehire/internal/db"
)

// Queries is the slice of the store this package reaches.
//
// One statement, and deliberately no way to read a message. The subject arrives joined
// onto the event; anything more would mean GET /me/emails/:id, which marks mail read —
// and read_at means "a human saw this", not "a calendar was scrolled past this".
type Queries interface {
	ListApplicationEventsInRange(ctx context.Context, arg db.ListApplicationEventsInRangeParams) ([]db.ListApplicationEventsInRangeRow, error)
	ListApplicationInterviewsInRange(ctx context.Context, arg db.ListApplicationInterviewsInRangeParams) ([]db.ListApplicationInterviewsInRangeRow, error)
	ListApplicationEvents(ctx context.Context, arg db.ListApplicationEventsParams) ([]db.ListApplicationEventsRow, error)
}

// MaxRangeDays caps one request at a year and a day — enough for any calendar span a
// reader paints at once, including a leap year.
//
// The index (user_id, occurred_at) makes the scan cheap and per-user volumes are small,
// so this is hygiene rather than a fix: an unbounded range over an append-only table is
// the kind of thing that costs nothing now and something later.
const MaxRangeDays = 366

// MaxApplicationEvents caps one application's history. An application accrues a handful of
// events — an apply, some mail, a stage change or two — so this is the same hygiene
// MaxRangeDays is rather than a limit anyone meets: an unbounded read of an append-only
// table costs nothing now and something later.
const MaxApplicationEvents = 100

// ErrInvalidRange is a from/to pair the reader cannot answer for. Callers wrap it with
// what was wrong; both readers need only to recognise it — the handler renders a 400,
// the in-process caller a sentence.
var ErrInvalidRange = errors.New("invalid range")

// Event is one thing that happened to one application.
//
// Kind and Signal are values from the appevent and mailclassify vocabularies, carried as
// they are rather than mapped onto a closed set here. application_events splits the two
// precisely so a growing vocabulary is not a change to a table that must not change, and
// a reader that enumerated today's kinds would put that cost back.
//
// The optional fields are zero when absent: an application whose posting cmd/prune has
// removed has no JobSlug, an event the candidate recorded has no message, and a message
// the candidate deleted lends neither EmailID nor EmailSubject while the event stands.
type Event struct {
	ID     int64
	Kind   string
	Signal string
	Source string
	// Observed is appevent's verdict on Source, resolved here so no reader has to hold a
	// second copy of the trust rule.
	Observed      bool
	OccurredAt    time.Time
	CompanySlug   string
	RoleTitle     string
	ApplicationID int64
	JobSlug       string
	EmailID       int64
	EmailSubject  string
}

// Service is the ledger's dated read.
type Service struct{ q Queries }

// New builds the service.
func New(q Queries) *Service { return &Service{q: q} }

// validRange is the one bound rule, shared by both reads. Two copies would drift, and the
// calendar asks them for the same window — a range one accepted and the other refused
// would paint half a month with no way to tell why.
// ForApplication reads one application's events, newest first — the history the application
// panel renders, where Range paints a month for the calendar.
//
// Same rows, same rules, one order each: the panel's reader wants what just happened, the
// calendar's wants a month laid out. The mapping is shared with Range for the reason the two
// queries share their joins — an event that meant one thing on the calendar and another in the
// panel would be exactly the drift the ledger exists to remove.
func (s *Service) ForApplication(ctx context.Context, userID, jobID int64) ([]Event, error) {
	rows, err := s.q.ListApplicationEvents(ctx, db.ListApplicationEventsParams{
		UserID: userID,
		JobID:  pgtype.Int8{Int64: jobID, Valid: true},
		Limit:  MaxApplicationEvents,
		// Which sources carry an emails.id in source_ref — see Range.
		SrcGmail:    appevent.SourceMailGmail,
		SrcHosted:   appevent.SourceMailHosted,
		SrcExternal: appevent.SourceMailExternal,
	})
	if err != nil {
		return nil, err
	}

	events := make([]Event, 0, len(rows))
	for _, r := range rows {
		events = append(events, Event{
			ID:            r.ID,
			Kind:          r.Kind,
			Signal:        r.Signal,
			Source:        r.Source,
			Observed:      appevent.TrustedForDayMath(r.Source),
			OccurredAt:    r.OccurredAt.Time,
			CompanySlug:   r.CompanySlug,
			RoleTitle:     r.RoleTitle.String,
			ApplicationID: r.ApplicationID.Int64,
			JobSlug:       r.JobSlug.String,
			EmailID:       r.EmailID.Int64,
			EmailSubject:  r.EmailSubject.String,
		})
	}
	return events, nil
}

func validRange(from, to time.Time) error {
	if from.IsZero() || to.IsZero() {
		return fmt.Errorf("%w: both bounds are required", ErrInvalidRange)
	}
	if to.Before(from) {
		return fmt.Errorf("%w: the upper bound precedes the lower one", ErrInvalidRange)
	}
	if to.Sub(from) > MaxRangeDays*24*time.Hour {
		return fmt.Errorf("%w: a range may span at most %d days", ErrInvalidRange, MaxRangeDays)
	}
	return nil
}

// Range returns the caller's live events between from and to inclusive, oldest first.
func (s *Service) Range(ctx context.Context, userID int64, from, to time.Time) ([]Event, error) {
	if err := validRange(from, to); err != nil {
		return nil, err
	}

	rows, err := s.q.ListApplicationEventsInRange(ctx, db.ListApplicationEventsInRangeParams{
		UserID: userID,
		FromAt: pgtype.Timestamptz{Time: from, Valid: true},
		ToAt:   pgtype.Timestamptz{Time: to, Valid: true},
		// Which sources carry an emails.id in source_ref. Passed rather than written into
		// the statement so the vocabulary stays here, beside the trust rule that reads it.
		SrcGmail:    appevent.SourceMailGmail,
		SrcHosted:   appevent.SourceMailHosted,
		SrcExternal: appevent.SourceMailExternal,
	})
	if err != nil {
		return nil, err
	}

	events := make([]Event, 0, len(rows))
	for _, r := range rows {
		events = append(events, Event{
			ID:            r.ID,
			Kind:          r.Kind,
			Signal:        r.Signal,
			Source:        r.Source,
			Observed:      appevent.TrustedForDayMath(r.Source),
			OccurredAt:    r.OccurredAt.Time,
			CompanySlug:   r.CompanySlug,
			RoleTitle:     r.RoleTitle.String,
			ApplicationID: r.ApplicationID.Int64,
			JobSlug:       r.JobSlug.String,
			EmailID:       r.EmailID.Int64,
			EmailSubject:  r.EmailSubject.String,
		})
	}
	return events, nil
}

// Interview is one meeting arranged for an application: what is going to happen, as
// against the Events above, which are what already has.
//
// It is served separately from the ledger for the reason the schema separates them. A
// meeting moves and is called off, so it cannot live in an append-only record; and its
// time is in the future, so it cannot be an occurred_at without making that column mean
// two different things.
type Interview struct {
	ID            int64
	ApplicationID int64
	StartsAt      time.Time
	EndsAt        time.Time
	Title         string
	JoinURL       string
	// Status is suggested | confirmed | cancelled. A cancelled meeting is served rather
	// than filtered: an interview that simply vanished from a Thursday cannot be told
	// apart from a calendar that failed to load.
	Status      string
	CompanySlug string
	RoleTitle   string
	JobSlug     string
}

// Interviews returns the caller's meetings starting inside the range, oldest first.
//
// Same bounds and the same validation as Range, so the calendar asks both reads for the
// same window and a mistyped bound is refused by both in the same way.
func (s *Service) Interviews(ctx context.Context, userID int64, from, to time.Time) ([]Interview, error) {
	if err := validRange(from, to); err != nil {
		return nil, err
	}
	rows, err := s.q.ListApplicationInterviewsInRange(ctx, db.ListApplicationInterviewsInRangeParams{
		UserID: userID,
		FromAt: pgtype.Timestamptz{Time: from, Valid: true},
		ToAt:   pgtype.Timestamptz{Time: to, Valid: true},
	})
	if err != nil {
		return nil, err
	}
	out := make([]Interview, 0, len(rows))
	for _, r := range rows {
		out = append(out, Interview{
			ID:            r.ID,
			ApplicationID: r.ApplicationID,
			StartsAt:      r.StartsAt.Time,
			EndsAt:        r.EndsAt.Time,
			Title:         r.Title,
			JoinURL:       r.JoinUrl,
			Status:        r.Status,
			CompanySlug:   r.CompanySlug,
			RoleTitle:     r.RoleTitle,
			JobSlug:       r.JobSlug.String,
		})
	}
	return out, nil
}
