// Package jobtracking contains the per-user job-tracking use cases: view, apply,
// save, unsave, and track. It is decoupled from Fiber and pgx — the caller
// supplies a Repository that maps the domain types; the HTTP handler is
// responsible for translating between the wire format and these domain types.
package jobtracking

import (
	"context"
	"errors"
	"time"

	"github.com/strelov1/freehire/internal/userjob"
)

// Interaction is the storage-agnostic result of a per-user job interaction.
type Interaction struct {
	JobID     int64
	ViewedAt  *time.Time
	SavedAt   *time.Time
	AppliedAt *time.Time
	Stage     *string
	Notes     *string
}

// Sentinel errors returned by the Service and Repository.
var (
	ErrJobNotFound  = errors.New("jobtracking: job not found")
	ErrInvalidStage = errors.New("jobtracking: invalid stage")
	ErrEmptyTrack   = errors.New("jobtracking: provide stage and/or notes")
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
	MarkApplied(ctx context.Context, userID, jobID int64) (Interaction, error)
	SaveJob(ctx context.Context, userID, jobID int64) (Interaction, error)

	// UnsaveJob clears the saved mark. It returns ErrNoInteraction when no row
	// exists at all (the Service turns that into a zero-interaction success).
	UnsaveJob(ctx context.Context, userID, jobID int64) (Interaction, error)

	// TrackJob upserts the stage and/or notes for the interaction. A nil pointer
	// means "leave unchanged".
	TrackJob(ctx context.Context, userID, jobID int64, stage, notes *string) (Interaction, error)

	// ClearJobProgress drops stage and applied_at, keeping saved_at/viewed_at/notes.
	ClearJobProgress(ctx context.Context, userID, jobID int64) (Interaction, error)

	// UntrackJob removes a job from the board entirely: clears saved_at, applied_at,
	// stage, and notes, keeping viewed_at so the job stays in view history.
	UntrackJob(ctx context.Context, userID, jobID int64) (Interaction, error)
}

// Service implements the per-user job-tracking use cases.
type Service struct {
	repo Repository
}

// New creates a Service backed by the given Repository.
func New(repo Repository) *Service {
	return &Service{repo: repo}
}

// RecordView resolves slug → jobID then delegates to the repository.
func (s *Service) RecordView(ctx context.Context, userID int64, slug string) (Interaction, error) {
	jobID, err := s.repo.JobIDBySlug(ctx, slug)
	if err != nil {
		return Interaction{}, err
	}
	return s.repo.RecordView(ctx, userID, jobID)
}

// MarkApplied resolves slug → jobID then delegates to the repository.
func (s *Service) MarkApplied(ctx context.Context, userID int64, slug string) (Interaction, error) {
	jobID, err := s.repo.JobIDBySlug(ctx, slug)
	if err != nil {
		return Interaction{}, err
	}
	return s.repo.MarkApplied(ctx, userID, jobID)
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
func (s *Service) Track(ctx context.Context, userID int64, slug string, stage, notes *string) (Interaction, error) {
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
	return s.repo.TrackJob(ctx, userID, jobID, stage, notes)
}
