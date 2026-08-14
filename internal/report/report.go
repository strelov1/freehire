// Package report is the job-report moderation queue: any authenticated user can flag a
// live vacancy with a reason and details, and a moderator resolves the report (optionally
// soft-closing the job) or dismisses it. It owns reason validation and the review state
// machine; the Repository owns persistence. A report never creates a job — unlike the
// submission queue there is no Minter; resolving may close the reported job through the
// narrow JobCloser seam, so the close concern stays explicit and testable.
package report

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
)

// Sentinel errors mapped to HTTP statuses by the handler.
var (
	// ErrReportNotFound is the missing review target (mapped to 404).
	ErrReportNotFound = errors.New("report: not found")
	// ErrDuplicateOpen is a second open report of the same job by the same user (the
	// partial unique index; mapped to 409).
	ErrDuplicateOpen = errors.New("report: an open report for this job already exists")
	// ErrAlreadyDecided is a resolve/dismiss of a report that is no longer pending
	// (mapped to 409).
	ErrAlreadyDecided = errors.New("report: already decided")
	// ErrInvalid is a content-validation failure (mapped to 400); its message is
	// user-facing.
	ErrInvalid = errors.New("report: invalid")
	// ErrRateLimited is a filing past the daily cap (mapped to 429).
	ErrRateLimited = errors.New("report: too many reports today")
)

// maxDetailsLen bounds the free-text explanation so a single report cannot carry an
// unbounded payload (the content-field convention).
const maxDetailsLen = 5000

// DailyCap bounds how many reports one account may file per rolling 24h window
// (ghostreport.DailyCap's counterpart for this queue). A real job seeker does not have more
// than a handful of postings to flag in a day, so the cap costs them nothing while bounding
// what one account can do to the moderation queue.
const DailyCap = 20

// validReasons is the closed reason vocabulary, mirrored by the migration's CHECK and the
// SPA. The service re-validates against it so an out-of-vocabulary value is a clean 400
// rather than a raw constraint error.
var validReasons = map[string]bool{
	"no_response":  true,
	"not_relevant": true,
	"spam":         true,
	"fraud":        true,
	"other":        true,
}

// FileInput is the user-supplied content of a report. The job is identified separately (by
// the slug the handler resolves), so it is not part of this shape.
type FileInput struct {
	Reason          string
	Details         string
	ContactTelegram string
}

// validate normalizes and checks the input, returning a trimmed copy ready to persist. A
// reason outside the vocabulary or over-long details is ErrInvalid.
//
// Details are OPTIONAL, and blank is stored as absent rather than refused. The reason
// vocabulary already states what is wrong, so the free text is elaboration — and a
// mandatory elaboration mostly collects whatever a reporter types to get past the field,
// which is worse than an empty one because it reaches a moderator looking like evidence.
func (in FileInput) validate() (FileInput, error) {
	if !validReasons[in.Reason] {
		return FileInput{}, fmt.Errorf("%w: unknown reason %q", ErrInvalid, in.Reason)
	}
	details := strings.TrimSpace(in.Details)
	if len(details) > maxDetailsLen {
		return FileInput{}, fmt.Errorf("%w: details too long", ErrInvalid)
	}
	return FileInput{
		Reason:          in.Reason,
		Details:         details,
		ContactTelegram: strings.TrimSpace(in.ContactTelegram),
	}, nil
}

// Report is a stored moderation-queue report: the package domain type, decoupled from the
// generated db row. The internal ownership columns (reported_by, reviewed_by) are dropped —
// they are never on the wire — while created_at/reviewed_at are kept as *time.Time because
// the handler serializes them.
type Report struct {
	ID              int64
	JobID           int64
	Reason          string
	Details         string
	ContactTelegram string
	Status          string
	ReviewReason    string
	ReviewedAt      *time.Time
	CreatedAt       *time.Time
}

// ReportDetail is a Report plus the joined columns a human needs to make sense of it — the
// reporter's email and the reported job's slug and title. The moderator queue renders them;
// the decision path mails them.
type ReportDetail struct {
	Report
	ReporterEmail string
	JobSlug       string
	JobTitle      string
}

// Repository is the persistence contract for the report queue. Implementations map the
// generated db rows to Report/ReportDetail, so the use case never sees db.*.
type Repository interface {
	Create(ctx context.Context, reportedBy, jobID int64, reason, details, contactTelegram string) (Report, error)
	Get(ctx context.Context, id int64) (ReportDetail, error)
	ListPending(ctx context.Context) ([]ReportDetail, error)
	MarkResolved(ctx context.Context, id, reviewedBy int64, note string) (Report, error)
	MarkDismissed(ctx context.Context, id, reviewedBy int64, reviewReason string) (Report, error)
	// CountFiledSince backs the daily cap: how many reports reportedBy has filed since the
	// given cutoff.
	CountFiledSince(ctx context.Context, reportedBy int64, since time.Time) (int, error)
}

// JobCloser soft-closes one job. The QueriesRepository satisfies it (over CloseJobByID);
// the seam keeps Resolve testable without a database and keeps the close path explicit.
type JobCloser interface {
	Close(ctx context.Context, jobID int64) error
}

// The outcomes a decision notice describes. A dismissal and a resolution read differently to
// the recipient, and a closed vacancy differently again (Decision.JobClosed).
const (
	OutcomeResolved  = "resolved"
	OutcomeDismissed = "dismissed"
)

// Decision is what the reporter is told: who to reach, what they reported, and what the
// moderator did about it. Note carries the moderator's words — the resolve note or the
// dismissal reason — and may be empty.
type Decision struct {
	Email     string
	JobTitle  string
	JobSlug   string
	Details   string
	Note      string
	Outcome   string
	JobClosed bool
}

// ReporterNotifier tells the person who filed a report what a moderator decided. MailNotifier
// is the production implementation; the seam keeps the use cases testable without a
// transport, and keeps the decision path independent of how the notice travels.
type ReporterNotifier interface {
	NotifyDecision(ctx context.Context, d Decision) error
}

// Service implements the report use cases.
type Service struct {
	repo     Repository
	closer   JobCloser
	notifier ReporterNotifier
}

// New creates a Service backed by the given Repository and JobCloser. It notifies no one
// until WithNotifier is called, which is the correct behavior wherever no mail transport is
// configured.
func New(repo Repository, closer JobCloser) *Service {
	return &Service{repo: repo, closer: closer}
}

// WithNotifier attaches the channel a decision is announced over and returns the Service, so
// the wiring reads as one expression. A Service without one still decides reports; it just
// tells no one, the same soft skip an unconfigured notification channel makes elsewhere.
func (s *Service) WithNotifier(n ReporterNotifier) *Service {
	s.notifier = n
	return s
}

// File validates the content and stores a pending report owned by reportedBy against
// jobID. A second open report of the same job by the same user surfaces ErrDuplicateOpen
// (the repository maps the unique violation); a reportedBy already at DailyCap for the
// rolling 24h window surfaces ErrRateLimited.
func (s *Service) File(ctx context.Context, reportedBy, jobID int64, in FileInput) (Report, error) {
	v, err := in.validate()
	if err != nil {
		return Report{}, err
	}
	filed, err := s.repo.CountFiledSince(ctx, reportedBy, time.Now().Add(-24*time.Hour))
	if err != nil {
		return Report{}, err
	}
	if filed >= DailyCap {
		return Report{}, ErrRateLimited
	}
	return s.repo.Create(ctx, reportedBy, jobID, v.Reason, v.Details, v.ContactTelegram)
}

// ListPending returns the moderator review queue (with reporter email and job slug/title),
// newest first.
func (s *Service) ListPending(ctx context.Context) ([]ReportDetail, error) {
	return s.repo.ListPending(ctx)
}

// ResolveInput is a moderator's resolve decision: whether to soft-close the reported job,
// the note explaining what was done, and whether to tell the reporter. The note is stored as
// the review reason and quoted in the notice, so it is written for the reporter, not as an
// internal annotation.
type ResolveInput struct {
	CloseJob       bool
	NotifyReporter bool
	Note           string
}

// DismissInput is a moderator's dismissal: the reason the report changed nothing, and
// whether to tell the reporter. Like ResolveInput.Note, the reason reaches the reporter.
type DismissInput struct {
	NotifyReporter bool
	Reason         string
}

// Review is the outcome of a moderator decision: the updated report, and whether the
// reporter was actually reached. Notified is false when the moderator did not ask, when no
// notifier is configured, and when delivery failed — the decision stands either way, so the
// moderator can follow up by hand instead of assuming a reply was sent.
type Review struct {
	Report   Report
	Notified bool
}

// Resolve marks a pending report resolved, recording the reviewing moderator and their note.
// When in.CloseJob is set, the reported job is soft-closed first; a close failure aborts
// before the mark, so the report stays pending and the action is safe to retry. A missing
// report is ErrReportNotFound; one no longer pending is ErrAlreadyDecided.
func (s *Service) Resolve(ctx context.Context, reviewerID, id int64, in ResolveInput) (Review, error) {
	rep, err := s.pending(ctx, id)
	if err != nil {
		return Review{}, err
	}
	if in.CloseJob {
		if err := s.closer.Close(ctx, rep.JobID); err != nil {
			return Review{}, err
		}
	}
	decided, err := s.repo.MarkResolved(ctx, id, reviewerID, in.Note)
	if err != nil {
		return Review{}, err
	}
	notified := s.notify(ctx, in.NotifyReporter, rep, Decision{
		Note:      in.Note,
		Outcome:   OutcomeResolved,
		JobClosed: in.CloseJob,
	})
	return Review{Report: decided, Notified: notified}, nil
}

// Dismiss marks a pending report dismissed with an optional reason, recording the reviewing
// moderator. The reported job is not touched. A missing report is ErrReportNotFound; one no
// longer pending is ErrAlreadyDecided.
func (s *Service) Dismiss(ctx context.Context, reviewerID, id int64, in DismissInput) (Review, error) {
	rep, err := s.pending(ctx, id)
	if err != nil {
		return Review{}, err
	}
	decided, err := s.repo.MarkDismissed(ctx, id, reviewerID, in.Reason)
	if err != nil {
		return Review{}, err
	}
	notified := s.notify(ctx, in.NotifyReporter, rep, Decision{
		Note:    in.Reason,
		Outcome: OutcomeDismissed,
	})
	return Review{Report: decided, Notified: notified}, nil
}

// pending loads the report a decision targets, rejecting one that is no longer awaiting
// review. Both decisions start here, so neither can forget the guard.
func (s *Service) pending(ctx context.Context, id int64) (ReportDetail, error) {
	rep, err := s.repo.Get(ctx, id)
	if err != nil {
		return ReportDetail{}, err
	}
	if rep.Status != statusPending {
		return ReportDetail{}, ErrAlreadyDecided
	}
	return rep, nil
}

// notify announces a decision that has already been recorded, reporting whether the reporter
// was reached. It is deliberately total: a moderator who did not ask, a Service with no
// notifier, and a transport that failed all mean "not notified" rather than an error,
// because a decision already written must not be undone by the announcement of it. A failure
// is logged so an outage is visible in the server log as well as in the response.
func (s *Service) notify(ctx context.Context, asked bool, rep ReportDetail, d Decision) bool {
	if !asked || s.notifier == nil {
		return false
	}
	d.Email, d.JobTitle, d.JobSlug, d.Details = rep.ReporterEmail, rep.JobTitle, rep.JobSlug, rep.Details
	if err := s.notifier.NotifyDecision(ctx, d); err != nil {
		log.Printf("report: notifying the reporter of report %d failed: %v", rep.ID, err)
		return false
	}
	return true
}

// statusPending is the only status that can be resolved or dismissed; the closed vocabulary
// lives in the migration's CHECK.
const statusPending = "pending"
