// Package jobtracking contains the per-user job-tracking use cases: view, apply,
// save, unsave, and track. It is decoupled from Fiber and pgx — the caller
// supplies a Repository that maps the domain types; the HTTP handler is
// responsible for translating between the wire format and these domain types.
package jobtracking

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/strelov1/freehire/internal/application/autoapply"
	"github.com/strelov1/freehire/internal/application/userjob"
	"github.com/strelov1/freehire/internal/job/applydate"
	"github.com/strelov1/freehire/internal/job/jobview"
)

// Interaction is the storage-agnostic result of a per-user job interaction.
type Interaction struct {
	JobID       int64
	ViewedAt    *time.Time
	SavedAt     *time.Time
	AppliedAt   *time.Time
	DismissedAt *time.Time
	Stage       *string
	Notes       *string
}

// Filter selects which interactions a listing returns. It is a controlled
// vocabulary owned here (mirroring userjob.Stage): "all" is every interaction,
// "viewed" the passive history (neither saved nor applied), "saved"/"applied"
// the respective subsets, "board" the Kanban set (saved, applied, or staged),
// and "dismissed" the jobs the user hid from the feed.
type Filter string

const (
	FilterAll       Filter = "all"
	FilterViewed    Filter = "viewed"
	FilterSaved     Filter = "saved"
	FilterApplied   Filter = "applied"
	FilterBoard     Filter = "board"
	FilterDismissed Filter = "dismissed"
)

// ParseFilter validates a raw filter string against the vocabulary. An empty
// string defaults to FilterAll, so the whole filter policy (including the default)
// lives here rather than leaking into the HTTP layer. An unknown value is
// ErrInvalidFilter.
func ParseFilter(s string) (Filter, error) {
	switch Filter(s) {
	case "", FilterAll:
		return FilterAll, nil
	case FilterViewed, FilterSaved, FilterApplied, FilterBoard, FilterDismissed:
		return Filter(s), nil
	}
	return "", ErrInvalidFilter
}

// CardDetail says how much of each row's job card a listing carries. The Kanban
// board draws only identity and tags, so it asks for CardCompact and skips the
// description read.
type CardDetail int

const (
	CardFull CardDetail = iota
	CardCompact
)

// Counts are the per-filter interaction totals for the my-jobs tab badges.
type Counts struct {
	All       int64
	Viewed    int64
	Saved     int64
	Applied   int64
	Board     int64
	Dismissed int64
}

// Total returns the count matching the active filter — the value the listing's
// meta.total should report.
func (c Counts) Total(f Filter) int64 {
	switch f {
	case FilterViewed:
		return c.Viewed
	case FilterSaved:
		return c.Saved
	case FilterApplied:
		return c.Applied
	case FilterBoard:
		return c.Board
	case FilterDismissed:
		return c.Dismissed
	default:
		return c.All
	}
}

// TrackedJob pairs a job in its canonical wire shape with the caller's
// interaction marks. The job carries identity via its slug; the embedded
// Interaction's JobID is the internal id, never serialized.
type TrackedJob struct {
	// AddedToBoardAt is the earliest stored save or application-record creation timestamp.
	// It stays independent of passive views and the candidate's editable application date.
	AddedToBoardAt *time.Time
	// ID addresses the row; an application whose posting was pruned has no slug to borrow.
	ID string
	// CompanySlug and RoleTitle come from the application record and are present on
	// every row, so a card can be rendered without reaching into Job.
	CompanySlug string
	RoleTitle   string
	// Job is absent when the catalogue no longer holds the posting.
	Job *jobview.Card
	Interaction
	// EmailCount is the caller's live inbox messages linked to this job — the
	// board's per-card ✉ badge. 0 for users without a connected mailbox.
	EmailCount int
	// LastActivityAt and HasPendingSuggestion are the raw silence inputs the
	// repository supplies; Silence turns them into a verdict.
	LastActivityAt       *time.Time
	HasPendingSuggestion bool
	// FollowedUpAt is when the candidate last recorded chasing this application, or
	// nil. Silence must not read it: a chase is not a reply.
	FollowedUpAt *time.Time
	// CVOpenedAt is when a CV of the caller's tied to this job was last opened by a countable
	// visitor (not automated traffic, not the candidate). Nil when none exists or none was opened.
	//
	// The board shows it BESIDE the silence state, never instead of it, and it stays out of
	// silence.StateFor's inputs like FollowedUpAt.
	CVOpenedAt *time.Time
	// AutoApplyStatus is the status of this job's live auto-apply attempt, nil when it has
	// none. The board's "needs your review" badge reads only this; the answer preview and
	// unmapped questions come from GetTrackedApplication.
	AutoApplyStatus *autoapply.Status
}

// Listing is the result of ListTracked: a page of tracked jobs for the active
// filter plus the per-filter counts for the tab badges.
type Listing struct {
	Filter Filter
	Items  []TrackedJob
	Counts Counts
}

// Total is the count for the active filter — the listing's meta.total.
func (l Listing) Total() int64 { return l.Counts.Total(l.Filter) }

// Sentinel errors returned by the Service and Repository.
var (
	ErrJobNotFound   = errors.New("jobtracking: job not found")
	ErrInvalidStage  = errors.New("jobtracking: invalid stage")
	ErrInvalidFilter = errors.New("jobtracking: invalid filter")
	ErrEmptyTrack    = errors.New("jobtracking: provide stage and/or notes")
	// ErrApplicationNotFound is returned when an application id names nothing the
	// caller holds. It must not distinguish "no such row" from "somebody else's row".
	ErrApplicationNotFound = errors.New("jobtracking: application not found")
	// ErrNoInteraction is returned by Repository.UnsaveJob when there is no
	// interaction row to clear. The Service converts it into a zero-interaction
	// success; it is never surfaced to the caller.
	ErrNoInteraction = errors.New("jobtracking: no interaction row")
)

// excludedJobsCap bounds the swipe deck's exclusion set so the search
// `id NOT IN (...)` filter stays small; a heavy triager past this cap may
// occasionally re-see a long-ago-seen job, which is acceptable.
const excludedJobsCap int32 = 1000

// appliedCompaniesCap bounds how many of a caller's newest applications the applied-
// company set is drawn from. It bounds both the read (one posting lookup per
// application) and the search's `company_slug NOT IN [...]` filter. The overflow is
// a long-ago employer missing its badge, never a wrong one.
const appliedCompaniesCap int32 = 1000

// Reminders is the come-back-and-apply reminder, as this package needs it.
//
// Declared here because internal/engage sits ABOVE internal/application:
// reminder.Service satisfies it structurally and the composition root supplies it.
//
// It hangs off the service so every caller (HTTP handler, assistant, mail
// reconstruction) gets the same side effect: saving creates the intent, applying
// or unsaving ends it.
type Reminders interface {
	ScheduleOnSave(ctx context.Context, userID, jobID int64) error
	Cancel(ctx context.Context, userID, jobID int64) error
}

// Option configures a Service at construction.
type Option func(*Service)

// WithReminders attaches the reminder side effects to save, unsave and apply.
// Without it those calls schedule and cancel nothing, which is what a caller with
// no reminder store wants.
func WithReminders(r Reminders) Option { return func(s *Service) { s.reminders = r } }

// Service implements the per-user job-tracking use cases.
type Service struct {
	repo      Repository
	reminders Reminders
}

// New creates a Service backed by the given Repository.
func New(repo Repository, opts ...Option) *Service {
	s := &Service{repo: repo}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// scheduleReminder and cancelReminder are best-effort: the tracking write already
// succeeded, and the reminder worker re-checks the intent before sending.
func (s *Service) scheduleReminder(ctx context.Context, userID, jobID int64) {
	if s.reminders == nil {
		return
	}
	if err := s.reminders.ScheduleOnSave(ctx, userID, jobID); err != nil {
		log.Printf("jobtracking: schedule reminder user=%d job=%d: %v", userID, jobID, err)
	}
}

func (s *Service) cancelReminder(ctx context.Context, userID, jobID int64) {
	if s.reminders == nil {
		return
	}
	if err := s.reminders.Cancel(ctx, userID, jobID); err != nil {
		log.Printf("jobtracking: cancel reminder user=%d job=%d: %v", userID, jobID, err)
	}
}

// ListTracked validates the filter, then reads the caller's interactions and the
// per-filter counts.
func (s *Service) ListTracked(ctx context.Context, userID int64, filter string, detail CardDetail, limit, offset int32) (Listing, error) {
	f, err := ParseFilter(filter)
	if err != nil {
		return Listing{}, err
	}
	items, err := s.repo.ListInteractions(ctx, userID, f, detail, limit, offset)
	if err != nil {
		return Listing{}, err
	}
	counts, err := s.repo.CountInteractions(ctx, userID)
	if err != nil {
		return Listing{}, err
	}
	return Listing{Filter: f, Items: items, Counts: counts}, nil
}

// ViewedSlugs returns every public job slug the caller has interacted with.
func (s *Service) ViewedSlugs(ctx context.Context, userID int64) ([]string, error) {
	return s.repo.ViewedSlugs(ctx, userID)
}

// SavedSlugs returns every public job slug the caller has saved (bookmarked).
func (s *Service) SavedSlugs(ctx context.Context, userID int64) ([]string, error) {
	return s.repo.SavedSlugs(ctx, userID)
}

// DismissedSlugs returns every public job slug the caller has hidden (dismissed).
func (s *Service) DismissedSlugs(ctx context.Context, userID int64) ([]string, error) {
	return s.repo.DismissedSlugs(ctx, userID)
}

// ExcludedJobIDs returns the job ids the caller has already interacted with
// (viewed, saved, applied, or dismissed) — the swipe deck's exclusion set, so a
// card is shown at most once across sessions, capped at excludedJobsCap.
func (s *Service) ExcludedJobIDs(ctx context.Context, userID int64) ([]int64, error) {
	return s.repo.ExcludedJobIDs(ctx, userID, excludedJobsCap)
}

// AppliedCompanySlugs returns the company slugs of the employers the caller has
// applied to — the set the feed marks "you already applied to this company" by, and
// the one /jobs/search's hide_applied_companies leaves out. Newest first, capped at
// appliedCompaniesCap applications.
func (s *Service) AppliedCompanySlugs(ctx context.Context, userID int64) ([]string, error) {
	return s.repo.AppliedCompanySlugs(ctx, userID, appliedCompaniesCap)
}

// Pipeline returns the caller's per-stage application counts plus the total.
// Grouping stages for the board is the reader's job (userjob.Groups).
func (s *Service) Pipeline(ctx context.Context, userID int64) (userjob.Pipeline, error) {
	counts, err := s.repo.PipelineCounts(ctx, userID)
	if err != nil {
		return userjob.Pipeline{}, err
	}
	pipeline := userjob.CountByStage(counts)

	// Best-effort: a lookup error omits the optional benchmark instead of failing the endpoint.
	you, global, err := s.repo.ReplyRateCounts(ctx, userID)
	if err != nil {
		log.Printf("jobtracking: reply-rate counts for user=%d: %v (omitting the benchmark)", userID, err)
	} else {
		pipeline.ReplyRate = userjob.GateReplyRateBenchmark(you, global)
	}

	return pipeline, nil
}

// RecordView resolves slug → jobID then delegates to the repository.
func (s *Service) RecordView(ctx context.Context, userID int64, slug string) (Interaction, error) {
	jobID, err := s.repo.JobIDBySlug(ctx, userID, slug)
	if err != nil {
		return Interaction{}, err
	}
	return s.repo.RecordView(ctx, userID, jobID)
}

// MarkApplied resolves slug → jobID then delegates to the repository. `source` is
// the appevent source of the recording, supplied by the caller.
func (s *Service) MarkApplied(ctx context.Context, userID int64, slug, source string) (Interaction, error) {
	jobID, err := s.repo.JobIDBySlug(ctx, userID, slug)
	if err != nil {
		return Interaction{}, err
	}
	row, err := s.repo.MarkApplied(ctx, userID, jobID, source)
	if err != nil {
		return row, err
	}
	// Applying ends the "come back and apply" intent.
	s.cancelReminder(ctx, userID, jobID)
	return row, nil
}

// MarkAppliedAt resolves slug → jobID then records an application dated by `at`
// — the mail-reconstruction path (see Repository.MarkAppliedAt).
func (s *Service) MarkAppliedAt(ctx context.Context, userID int64, slug string, at time.Time, source string) (Interaction, error) {
	jobID, err := s.repo.JobIDBySlug(ctx, userID, slug)
	if err != nil {
		return Interaction{}, err
	}
	row, err := s.repo.MarkAppliedAt(ctx, userID, jobID, at, source)
	if err != nil {
		return row, err
	}
	// A mail-reconstructed application ends the intent like a clicked one.
	s.cancelReminder(ctx, userID, jobID)
	return row, nil
}

// MarkAppliedOn records an application on the day the candidate states, overwriting a date
// already held. `now` is supplied so the bound is testable.
//
// The window is enforced here, not at the HTTP door, so it holds for callers that bypass
// Fiber, like the assistant.
// `day` is the calendar day stated and the window is checked against it, never against the
// stored instant, which would refuse "today" for the whole UTC morning.
func (s *Service) MarkAppliedOn(ctx context.Context, userID int64, slug string, day, now time.Time, source string) (Interaction, error) {
	if err := applydate.Validate(day, now); err != nil {
		return Interaction{}, err
	}
	jobID, err := s.repo.JobIDBySlug(ctx, userID, slug)
	if err != nil {
		return Interaction{}, err
	}
	row, err := s.repo.MarkAppliedOn(ctx, userID, jobID, applydate.Instant(day), source)
	if err != nil {
		return row, err
	}
	s.cancelReminder(ctx, userID, jobID)
	return row, nil
}

// SaveJob resolves slug → jobID then delegates to the repository.
func (s *Service) SaveJob(ctx context.Context, userID int64, slug string) (Interaction, error) {
	jobID, err := s.repo.JobIDBySlug(ctx, userID, slug)
	if err != nil {
		return Interaction{}, err
	}
	row, err := s.repo.SaveJob(ctx, userID, jobID)
	if err != nil {
		return row, err
	}
	// Saving is what creates the intent the reminder nudges toward.
	s.scheduleReminder(ctx, userID, jobID)
	return row, nil
}

// Unsave resolves slug → jobID then clears the saved mark. If the repository
// returns ErrNoInteraction (no row to clear), the method returns a zero
// Interaction with only JobID set — unsaving is idempotent.
func (s *Service) Unsave(ctx context.Context, userID int64, slug string) (Interaction, error) {
	jobID, err := s.repo.JobIDBySlug(ctx, userID, slug)
	if err != nil {
		return Interaction{}, err
	}
	row, err := s.repo.UnsaveJob(ctx, userID, jobID)
	if errors.Is(err, ErrNoInteraction) {
		row, err = Interaction{JobID: jobID}, nil
	}
	if err != nil {
		return row, err
	}
	// Unsaving withdraws the intent the reminder was nudging toward.
	s.cancelReminder(ctx, userID, jobID)
	return row, nil
}

// Dismiss resolves slug → jobID then delegates to the repository, marking the
// job dismissed in the swipe deck.
func (s *Service) Dismiss(ctx context.Context, userID int64, slug string) (Interaction, error) {
	jobID, err := s.repo.JobIDBySlug(ctx, userID, slug)
	if err != nil {
		return Interaction{}, err
	}
	return s.repo.DismissJob(ctx, userID, jobID)
}

// Undismiss resolves slug → jobID then clears the dismissed mark. If the
// repository returns ErrNoInteraction (no row to clear), the method returns a
// zero Interaction with only JobID set — undismissing is idempotent.
func (s *Service) Undismiss(ctx context.Context, userID int64, slug string) (Interaction, error) {
	jobID, err := s.repo.JobIDBySlug(ctx, userID, slug)
	if err != nil {
		return Interaction{}, err
	}
	row, err := s.repo.UndismissJob(ctx, userID, jobID)
	if errors.Is(err, ErrNoInteraction) {
		return Interaction{JobID: jobID}, nil
	}
	return row, err
}

// ClearProgress resolves slug → jobID then drops stage and applied state, keeping
// saved_at/viewed_at/notes intact (the "drag back to Saved" Kanban action).
func (s *Service) ClearProgress(ctx context.Context, userID int64, slug string) (Interaction, error) {
	jobID, err := s.repo.JobIDBySlug(ctx, userID, slug)
	if err != nil {
		return Interaction{}, err
	}
	return s.repo.ClearJobProgress(ctx, userID, jobID)
}

// Untrack resolves slug → jobID then removes the job from the board by clearing
// saved_at, applied_at, stage, and notes while keeping viewed_at.
func (s *Service) Untrack(ctx context.Context, userID int64, slug string) (Interaction, error) {
	jobID, err := s.repo.JobIDBySlug(ctx, userID, slug)
	if err != nil {
		return Interaction{}, err
	}
	return s.repo.UntrackJob(ctx, userID, jobID)
}

// Track validates the request first (before any slug lookup), then resolves
// slug → jobID and delegates to the repository.
//
// Validation rules:
//   - Both stage and notes nil → ErrEmptyTrack.
//   - stage set but not a valid userjob.Stage value → ErrInvalidStage.
func (s *Service) Track(ctx context.Context, userID int64, slug string, stage, notes *string, source string) (Interaction, error) {
	if stage == nil && notes == nil {
		return Interaction{}, ErrEmptyTrack
	}
	if stage != nil && !userjob.ValidStage(*stage) {
		return Interaction{}, ErrInvalidStage
	}
	jobID, err := s.repo.JobIDBySlug(ctx, userID, slug)
	if err != nil {
		return Interaction{}, err
	}
	return s.repo.TrackJob(ctx, userID, jobID, stage, notes, source)
}

// TrackApplication is Track for a caller holding the application's own id; the same
// validation runs first, keeping a bad body off the database.
func (s *Service) TrackApplication(ctx context.Context, userID, appID int64, stage, notes *string, source string) (Interaction, error) {
	if stage == nil && notes == nil {
		return Interaction{}, ErrEmptyTrack
	}
	if stage != nil && !userjob.ValidStage(*stage) {
		return Interaction{}, ErrInvalidStage
	}
	return s.repo.TrackApplication(ctx, userID, appID, stage, notes, source)
}

// ClearApplicationProgress is ClearProgress for a caller holding the application's id.
func (s *Service) ClearApplicationProgress(ctx context.Context, userID, appID int64) (Interaction, error) {
	return s.repo.ClearApplicationProgress(ctx, userID, appID)
}

// UntrackApplication is Untrack for a caller holding the application's id.
func (s *Service) UntrackApplication(ctx context.Context, userID, appID int64) (Interaction, error) {
	return s.repo.UntrackApplication(ctx, userID, appID)
}
