// Package jobtracking contains the per-user job-tracking use cases: view, apply,
// save, unsave, and track. It is decoupled from Fiber and pgx — the caller
// supplies a Repository that maps the domain types; the HTTP handler is
// responsible for translating between the wire format and these domain types.
package jobtracking

import (
	"context"
	"errors"
	"time"

	"github.com/strelov1/freehire/internal/jobview"
	"github.com/strelov1/freehire/internal/userjob"
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
	// ID addresses the row. The board has always been a list of applications and only
	// borrowed the posting's slug because one was always at hand; an application whose
	// posting was pruned has none to borrow, so the row carries its own.
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
	// ReminderFireAt is the pending saved-job reminder's deadline, or nil when the
	// job has no pending reminder — the saved list renders its "remind in N days"
	// chip from it.
	ReminderFireAt *time.Time
	// LastActivityAt and HasPendingSuggestion are the raw silence inputs the
	// repository supplies; Silence turns them into a verdict. They are kept
	// separate so the adapter carries facts and the domain does the judging.
	LastActivityAt       *time.Time
	HasPendingSuggestion bool
	// FollowedUpAt is when the candidate last recorded chasing this application, or
	// nil for one never chased. It rides beside the silence rather than inside it:
	// Silence must not read it, because a chase is not a reply (see 0059's column
	// comment). The board renders the pair — still silent, and chased N days ago.
	FollowedUpAt *time.Time
	// CVOpenedAt is when a CV of the caller's tied to this job was last opened by a countable
	// visitor — not automated traffic, and not the candidate checking their own PDF. Nil when they
	// have no traced CV for it, or nobody has opened one.
	//
	// The board shows it BESIDE the silence state, never instead of it: "they have not answered in
	// 24 days" and "somebody opened the CV yesterday" are two readings of the same application, and
	// the second does not soften the first. It is kept out of SilenceStateFor's inputs for the same
	// reason FollowedUpAt is — see internal/userjob.
	CVOpenedAt *time.Time
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
	// caller holds. It carries no distinction between "no such row" and "somebody
	// else's row" — the handler renders both as one 404, and the repository must not
	// hand it the material to do otherwise.
	ErrApplicationNotFound = errors.New("jobtracking: application not found")
	// ErrNoInteraction is returned by Repository.UnsaveJob when there is no
	// interaction row to clear. The Service converts it into a zero-interaction
	// success; it is never surfaced to the caller.
	ErrNoInteraction = errors.New("jobtracking: no interaction row")
)

// Repository is the narrow persistence contract required by the Service. The
// real adapter maps db.UserJob → Interaction; the test fake returns canned
// values.
type Repository interface {
	// JobIDBySlug returns the internal job id for the given public slug, or
	// ErrJobNotFound when no job matches.
	JobIDBySlug(ctx context.Context, slug string) (int64, error)

	RecordView(ctx context.Context, userID, jobID int64) (Interaction, error)

	// MarkApplied records an application. `source` is the appevent source of the
	// recording — who observed it, not who applied — and is carried by the caller
	// rather than assumed here: the same method serves the SPA, an API key, and the
	// in-app assistant, and a ledger that guessed would flatten the three.
	MarkApplied(ctx context.Context, userID, jobID int64, source string) (Interaction, error)

	// MarkAppliedAt records an application dated by `at` instead of now(), for an
	// application reconstructed from employer mail. It keeps an existing
	// applied_at rather than rewriting it: a later recording of the same
	// application is not a later application.
	MarkAppliedAt(ctx context.Context, userID, jobID int64, at time.Time, source string) (Interaction, error)

	// MarkAppliedOn records an application on a day the candidate states, and
	// overwrites a date already recorded. It is the counterpart of MarkAppliedAt and
	// differs from it in exactly that: mail supplies an upper bound, while a person
	// supplies the answer, so theirs wins. The `applied` ledger event moves with the
	// column, because every aggregate dates the application from the event.
	MarkAppliedOn(ctx context.Context, userID, jobID int64, at time.Time, source string) (Interaction, error)

	SaveJob(ctx context.Context, userID, jobID int64) (Interaction, error)

	// UnsaveJob clears the saved mark. It returns ErrNoInteraction when no row
	// exists at all (the Service turns that into a zero-interaction success).
	UnsaveJob(ctx context.Context, userID, jobID int64) (Interaction, error)

	// DismissJob sets the dismissed mark, idempotently, keeping the job out of the
	// swipe deck without affecting the public list/search.
	DismissJob(ctx context.Context, userID, jobID int64) (Interaction, error)

	// UndismissJob clears the dismissed mark. It returns ErrNoInteraction when no
	// row exists at all (the Service turns that into a zero-interaction success).
	UndismissJob(ctx context.Context, userID, jobID int64) (Interaction, error)

	// TrackJob upserts the stage and/or notes for the interaction. A nil pointer
	// means "leave unchanged". `source` is the appevent source of the recording,
	// carried for the same reason MarkApplied carries it.
	TrackJob(ctx context.Context, userID, jobID int64, stage, notes *string, source string) (Interaction, error)

	// ClearJobProgress drops stage and applied_at, keeping saved_at/viewed_at/notes.
	ClearJobProgress(ctx context.Context, userID, jobID int64) (Interaction, error)

	// UntrackJob removes a job from the board entirely: clears saved_at, applied_at,
	// stage, and notes, keeping viewed_at so the job stays in view history.
	UntrackJob(ctx context.Context, userID, jobID int64) (Interaction, error)

	// The three below address an application by its own id rather than through a
	// posting. They exist because the posting is the part that can disappear: once
	// cmd/prune removes it the slug-addressed writes above have nothing to resolve,
	// and the card on the board becomes unmovable. Each returns
	// ErrApplicationNotFound when the id names nothing the caller holds.

	// TrackApplication sets stage and/or notes on the application. A nil pointer
	// means "leave unchanged", and `source` is carried as TrackJob carries it.
	TrackApplication(ctx context.Context, userID, appID int64, stage, notes *string, source string) (Interaction, error)

	// ClearApplicationProgress drops stage and applied_at, keeping the record and its
	// notes — the pair is what puts an application on the board.
	ClearApplicationProgress(ctx context.Context, userID, appID int64) (Interaction, error)

	// UntrackApplication removes the application outright, as UntrackJob does for a
	// posting-backed one.
	UntrackApplication(ctx context.Context, userID, appID int64) (Interaction, error)

	// ListInteractions returns the caller's interactions joined with the jobs,
	// narrowed by an already-validated filter, most recently touched first.
	ListInteractions(ctx context.Context, userID int64, filter Filter, limit, offset int32) ([]TrackedJob, error)

	// CountInteractions returns the per-filter counts for the caller in one pass.
	CountInteractions(ctx context.Context, userID int64) (Counts, error)

	// PipelineCounts returns the caller's per-stage application counts (saved-only
	// rows excluded; an applied row with no stage carries an empty Stage).
	PipelineCounts(ctx context.Context, userID int64) ([]userjob.StageCount, error)

	// ViewedSlugs returns every public job slug the caller has interacted with.
	ViewedSlugs(ctx context.Context, userID int64) ([]string, error)

	// SavedSlugs returns every public job slug the caller has saved (bookmarked).
	SavedSlugs(ctx context.Context, userID int64) ([]string, error)

	// DismissedSlugs returns every public job slug the caller has hidden (dismissed).
	DismissedSlugs(ctx context.Context, userID int64) ([]string, error)

	// ExcludedJobIDs returns up to limit job ids the caller has already interacted
	// with (viewed, saved, applied, or dismissed), most-recently-touched first.
	ExcludedJobIDs(ctx context.Context, userID int64, limit int32) ([]int64, error)
}

// excludedJobsCap bounds the swipe deck's exclusion set so the search
// `id NOT IN (...)` filter stays small; a heavy triager past this cap may
// occasionally re-see a long-ago-seen job, which is acceptable.
const excludedJobsCap int32 = 1000

// Service implements the per-user job-tracking use cases.
type Service struct {
	repo Repository
}

// New creates a Service backed by the given Repository.
func New(repo Repository) *Service {
	return &Service{repo: repo}
}

// ListTracked validates the filter (the vocabulary and its default live in
// ParseFilter, not the HTTP layer), then reads the caller's interactions and the
// per-filter counts. The returned Listing knows its active filter, so Total picks
// the matching count.
func (s *Service) ListTracked(ctx context.Context, userID int64, filter string, limit, offset int32) (Listing, error) {
	f, err := ParseFilter(filter)
	if err != nil {
		return Listing{}, err
	}
	items, err := s.repo.ListInteractions(ctx, userID, f, limit, offset)
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

// Pipeline returns the caller's application-pipeline snapshot: the count at each stage of the
// vocabulary, plus the application total. Grouping those stages into the four the board and the
// funnel draw is the reader's job, applied from userjob.Groups — the server does not pick a
// second vocabulary on their behalf.
func (s *Service) Pipeline(ctx context.Context, userID int64) (userjob.Pipeline, error) {
	counts, err := s.repo.PipelineCounts(ctx, userID)
	if err != nil {
		return userjob.Pipeline{}, err
	}
	return userjob.CountByStage(counts), nil
}

// RecordView resolves slug → jobID then delegates to the repository.
func (s *Service) RecordView(ctx context.Context, userID int64, slug string) (Interaction, error) {
	jobID, err := s.repo.JobIDBySlug(ctx, slug)
	if err != nil {
		return Interaction{}, err
	}
	return s.repo.RecordView(ctx, userID, jobID)
}

// MarkApplied resolves slug → jobID then delegates to the repository. `source` is
// the appevent source of the recording, supplied by the caller.
func (s *Service) MarkApplied(ctx context.Context, userID int64, slug, source string) (Interaction, error) {
	jobID, err := s.repo.JobIDBySlug(ctx, slug)
	if err != nil {
		return Interaction{}, err
	}
	return s.repo.MarkApplied(ctx, userID, jobID, source)
}

// MarkAppliedAt resolves slug → jobID then records an application dated by `at`
// — the mail-reconstruction path (see Repository.MarkAppliedAt).
func (s *Service) MarkAppliedAt(ctx context.Context, userID int64, slug string, at time.Time, source string) (Interaction, error) {
	jobID, err := s.repo.JobIDBySlug(ctx, slug)
	if err != nil {
		return Interaction{}, err
	}
	return s.repo.MarkAppliedAt(ctx, userID, jobID, at, source)
}

// MarkAppliedOn records an application on the day the candidate states, overwriting a date
// already held. `now` is supplied rather than read here so the bound is testable and so the
// caller's clock, not this package's, decides what "the future" means.
//
// The window is enforced at this level rather than at the HTTP door so that it holds for every
// caller of the service, not only the one that arrives over HTTP: the in-app assistant calls
// jobtracking directly and never passes through Fiber. Its apply tool takes no date today, so
// this is a guard against the next caller rather than a fix for a current one.
// `day` is the calendar day stated, and the window is checked against it — never against the
// instant it is stored at. Those differ by the storage hour, and bounding the instant refuses
// "today" for the whole UTC morning.
func (s *Service) MarkAppliedOn(ctx context.Context, userID int64, slug string, day, now time.Time, source string) (Interaction, error) {
	if err := userjob.ValidateAppliedOn(day, now); err != nil {
		return Interaction{}, err
	}
	jobID, err := s.repo.JobIDBySlug(ctx, slug)
	if err != nil {
		return Interaction{}, err
	}
	return s.repo.MarkAppliedOn(ctx, userID, jobID, userjob.AppliedOnInstant(day), source)
}

// SaveJob resolves slug → jobID then delegates to the repository.
func (s *Service) SaveJob(ctx context.Context, userID int64, slug string) (Interaction, error) {
	jobID, err := s.repo.JobIDBySlug(ctx, slug)
	if err != nil {
		return Interaction{}, err
	}
	return s.repo.SaveJob(ctx, userID, jobID)
}

// Unsave resolves slug → jobID then clears the saved mark. If the repository
// returns ErrNoInteraction (no row to clear), the method returns a zero
// Interaction with only JobID set — unsaving is idempotent.
func (s *Service) Unsave(ctx context.Context, userID int64, slug string) (Interaction, error) {
	jobID, err := s.repo.JobIDBySlug(ctx, slug)
	if err != nil {
		return Interaction{}, err
	}
	row, err := s.repo.UnsaveJob(ctx, userID, jobID)
	if errors.Is(err, ErrNoInteraction) {
		return Interaction{JobID: jobID}, nil
	}
	return row, err
}

// Dismiss resolves slug → jobID then delegates to the repository, marking the
// job dismissed in the swipe deck.
func (s *Service) Dismiss(ctx context.Context, userID int64, slug string) (Interaction, error) {
	jobID, err := s.repo.JobIDBySlug(ctx, slug)
	if err != nil {
		return Interaction{}, err
	}
	return s.repo.DismissJob(ctx, userID, jobID)
}

// Undismiss resolves slug → jobID then clears the dismissed mark. If the
// repository returns ErrNoInteraction (no row to clear), the method returns a
// zero Interaction with only JobID set — undismissing is idempotent.
func (s *Service) Undismiss(ctx context.Context, userID int64, slug string) (Interaction, error) {
	jobID, err := s.repo.JobIDBySlug(ctx, slug)
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
	jobID, err := s.repo.JobIDBySlug(ctx, slug)
	if err != nil {
		return Interaction{}, err
	}
	return s.repo.ClearJobProgress(ctx, userID, jobID)
}

// Untrack resolves slug → jobID then removes the job from the board by clearing
// saved_at, applied_at, stage, and notes while keeping viewed_at.
func (s *Service) Untrack(ctx context.Context, userID int64, slug string) (Interaction, error) {
	jobID, err := s.repo.JobIDBySlug(ctx, slug)
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
	jobID, err := s.repo.JobIDBySlug(ctx, slug)
	if err != nil {
		return Interaction{}, err
	}
	return s.repo.TrackJob(ctx, userID, jobID, stage, notes, source)
}

// TrackApplication is Track for a caller holding the application's own id. The same
// validation runs first and for the same reason: a bad body is a bad request whichever
// way the row was named, and rejecting it here keeps it off the database.
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
