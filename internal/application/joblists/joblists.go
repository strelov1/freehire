// Package joblists is the per-user job-list use case: a signed-in user names a set of
// specific jobs (independent of the single-flag "save"), manages its membership, and
// can optionally publish it read-only by minting a public slug. It owns validation
// (name/description bounds, the per-user cap); the Repository owns persistence and
// maps the relevant Postgres conditions (unique violation, no row) onto the package
// sentinels.
package joblists

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/strelov1/freehire/internal/job/jobview"
)

// Sentinel errors mapped to HTTP statuses by the handler.
var (
	// ErrInvalidName is a blank or over-long name (mapped to 400).
	ErrInvalidName = errors.New("joblists: name must be 1-100 characters")
	// ErrDuplicateName is a name the user already uses (the UNIQUE (user_id, name)
	// constraint; mapped to 409).
	ErrDuplicateName = errors.New("joblists: a job list with this name already exists")
	// ErrCapExceeded is a create past the per-user limit (mapped to 409).
	ErrCapExceeded = errors.New("joblists: job-list limit reached")
	// ErrNotFound is a missing or non-owned target (mapped to 404).
	ErrNotFound = errors.New("joblists: not found")
	// ErrInvalidDescription is an over-long description (mapped to 400).
	ErrInvalidDescription = errors.New("joblists: description must be at most 2000 characters")
	// ErrSlugTaken is a public-slug UNIQUE collision on share. It is an internal retry
	// signal (Share regenerates the suffix and tries again), not a client-facing status.
	ErrSlugTaken = errors.New("joblists: public slug already taken")
	// ErrJobNotFound is an add-to-list for a job id that does not exist (mapped to 404).
	ErrJobNotFound = errors.New("joblists: job not found")
	// ErrListFull is an add-to-list past the per-list job cap (mapped to 409).
	ErrListFull = errors.New("joblists: job-list is at its job limit")
)

const (
	// maxNameLen bounds a job-list name; the migration's CHECK is the backstop.
	maxNameLen = 100
	// maxPerUser caps how many job lists a single user may keep.
	maxPerUser = 50
	// maxDescriptionLen bounds the stored description; the migration's CHECK is the backstop.
	maxDescriptionLen = 2000
	// maxJobsPerList caps how many jobs a single list may hold. A curated shortlist
	// has no legitimate need for more; the bound also keeps the public, unauthenticated
	// read (GetPublicList, which renders every job's blurb) at a predictable cost per
	// request rather than scaling with however many jobs one list accumulated.
	maxJobsPerList = 200
	// slugFallbackBase is used when a name transliterates to nothing (e.g. all symbols),
	// so a list always gets a usable slug.
	slugFallbackBase = "list"
	// maxShareAttempts bounds the retry loop when a minted slug collides with an
	// existing list; each attempt draws a fresh suffix.
	maxShareAttempts = 5
)

// JobList is a stored named set of jobs: the package domain type, decoupled from the
// generated db row. The internal owner column (user_id) is dropped — it is never on
// the wire and scoping is enforced in SQL — while created_at/updated_at are kept as
// *time.Time because the handler serializes them. JobCount is populated by List only
// (a per-item count is not meaningful outside a listing). PublicSlug is a plain
// string, empty when the list is private (a shared list always carries a non-empty
// slug, so an empty PublicSlug is an unambiguous "not shared").
type JobList struct {
	ID          int64
	Name        string
	Description string
	PublicSlug  string
	JobCount    int64
	CreatedAt   *time.Time
	UpdatedAt   *time.Time
}

// PublicJobList is the public read of a shared list: its display fields and jobs (no
// owner columns).
type PublicJobList struct {
	Name        string
	Description string
	Jobs        []jobview.Card
}

// ListMembership is one of a user's lists, flagged with whether a given job already
// belongs to it — what the job card's "Add to list" control renders as a toggle.
type ListMembership struct {
	ID     int64
	Name   string
	InList bool
}

// Repository is the persistence contract for job lists. Every method except
// GetPublicList is user-scoped. Create maps a unique violation to ErrDuplicateName;
// Update maps a unique violation to ErrDuplicateName and a missing owner-scoped row
// to ErrNotFound; Delete maps "no row affected" to ErrNotFound. Implementations map
// the generated db rows to JobList/PublicJobList, so the use case never sees db.*.
type Repository interface {
	List(ctx context.Context, userID int64) ([]JobList, error)
	Count(ctx context.Context, userID int64) (int64, error)
	Create(ctx context.Context, userID int64, name, description string) (JobList, error)
	// Update overwrites the name and/or description (a nil field is left unchanged),
	// owner-scoped.
	Update(ctx context.Context, id, userID int64, name, description *string) (JobList, error)
	Delete(ctx context.Context, id, userID int64) error
	// Get reads one of a user's lists, owner-scoped; no row → ErrNotFound.
	Get(ctx context.Context, id, userID int64) (JobList, error)
	// CountItems returns how many jobs a list holds (the per-list cap check input).
	CountItems(ctx context.Context, id int64) (int64, error)
	// HasItem reports whether a job already belongs to a list — an existing member is
	// exempt from the per-list cap on re-add.
	HasItem(ctx context.Context, id, jobID int64) (bool, error)
	// AddJob adds a job to a list the caller already confirmed ownership of.
	// Idempotent: an already-present job changes nothing.
	AddJob(ctx context.Context, id, jobID int64) error
	// RemoveJob removes a job from a list the caller already confirmed ownership of.
	// Idempotent: an absent job changes nothing.
	RemoveJob(ctx context.Context, id, jobID int64) error
	// JobIDBySlug returns the internal job id for the given public slug, or
	// ErrJobNotFound when no job matches. Jobs are addressed by slug at the wire
	// boundary everywhere in this app (save/unsave, tracking, ...); this resolves
	// that boundary the same way internal/application/jobtracking does.
	JobIDBySlug(ctx context.Context, slug string) (int64, error)
	// SetPublicSlug publishes a list (owner-scoped); a slug UNIQUE collision →
	// ErrSlugTaken (the service retries), no owner-scoped row → ErrNotFound.
	SetPublicSlug(ctx context.Context, id, userID int64, publicSlug string) (JobList, error)
	// ClearPublicSlug unpublishes a list (owner-scoped); no owner-scoped row → ErrNotFound.
	ClearPublicSlug(ctx context.Context, id, userID int64) error
	// GetPublicList reads a shared list by slug (no auth, no owner-scoping); no row → ErrNotFound.
	GetPublicList(ctx context.Context, slug string) (PublicJobList, error)
	// MembershipForJob returns every one of the user's lists, flagged with whether
	// jobID belongs to it.
	MembershipForJob(ctx context.Context, userID, jobID int64) ([]ListMembership, error)
}

// Service implements the job-list use cases.
type Service struct {
	repo Repository
}

// New creates a Service backed by the given Repository.
func New(repo Repository) *Service {
	return &Service{repo: repo}
}

// List returns the user's job lists, most recently updated first.
func (s *Service) List(ctx context.Context, userID int64) ([]JobList, error) {
	return s.repo.List(ctx, userID)
}

// Create validates and stores a job list for the user. The name is trimmed and
// bounded; the description is trimmed and bounded (empty is valid); the per-user cap
// is checked before the insert; a duplicate name surfaces as ErrDuplicateName (mapped
// by the repository).
func (s *Service) Create(ctx context.Context, userID int64, name, description string) (JobList, error) {
	name, err := validName(name)
	if err != nil {
		return JobList{}, err
	}
	description, err = validDescription(description)
	if err != nil {
		return JobList{}, err
	}
	count, err := s.repo.Count(ctx, userID)
	if err != nil {
		return JobList{}, err
	}
	if count >= maxPerUser {
		return JobList{}, ErrCapExceeded
	}
	return s.repo.Create(ctx, userID, name, description)
}

// Update overwrites a job list's name and/or description, scoped to its owner. A nil
// field is left unchanged (partial update). A provided name/description is validated.
// A missing or non-owned row surfaces as ErrNotFound (mapped by the repository).
func (s *Service) Update(ctx context.Context, userID, id int64, name, description *string) (JobList, error) {
	if name != nil {
		valid, err := validName(*name)
		if err != nil {
			return JobList{}, err
		}
		name = &valid
	}
	if description != nil {
		valid, err := validDescription(*description)
		if err != nil {
			return JobList{}, err
		}
		description = &valid
	}
	return s.repo.Update(ctx, id, userID, name, description)
}

// Delete removes one of the user's job lists. A missing or non-owned row surfaces as
// ErrNotFound (mapped by the repository).
func (s *Service) Delete(ctx context.Context, userID, id int64) error {
	return s.repo.Delete(ctx, id, userID)
}

// AddJob adds a job (addressed by its public slug, the wire identifier every job
// carries) to one of the user's lists. Ownership is confirmed first (a missing or
// non-owned list → ErrNotFound); an unknown slug → ErrJobNotFound. Adding an
// already-present job is idempotent (mapped by the repository) and exempt from the
// per-list cap; adding a new job once the list already holds maxJobsPerList → ErrListFull.
func (s *Service) AddJob(ctx context.Context, userID, id int64, jobSlug string) error {
	if _, err := s.repo.Get(ctx, id, userID); err != nil {
		return err
	}
	jobID, err := s.repo.JobIDBySlug(ctx, jobSlug)
	if err != nil {
		return err
	}
	has, err := s.repo.HasItem(ctx, id, jobID)
	if err != nil {
		return err
	}
	if !has {
		count, err := s.repo.CountItems(ctx, id)
		if err != nil {
			return err
		}
		if count >= maxJobsPerList {
			return ErrListFull
		}
	}
	return s.repo.AddJob(ctx, id, jobID)
}

// RemoveJob removes a job (addressed by its public slug) from one of the user's
// lists. Ownership is confirmed first (a missing or non-owned list → ErrNotFound). A
// slug that does not resolve to any job, or one that resolves but is not in the
// list, is idempotent — the goal state (the job is not in the list) already holds.
func (s *Service) RemoveJob(ctx context.Context, userID, id int64, jobSlug string) error {
	if _, err := s.repo.Get(ctx, id, userID); err != nil {
		return err
	}
	jobID, err := s.repo.JobIDBySlug(ctx, jobSlug)
	if errors.Is(err, ErrJobNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return s.repo.RemoveJob(ctx, id, jobID)
}

// GetPublicList reads a shared list by its public slug. An unknown or unshared slug
// surfaces as ErrNotFound (mapped by the repository).
func (s *Service) GetPublicList(ctx context.Context, slug string) (PublicJobList, error) {
	return s.repo.GetPublicList(ctx, slug)
}

// ListMembership resolves the given job slug and returns every one of the user's
// lists flagged with whether that job belongs to it — the job card's "Add to list"
// control reads this to render its toggle state. An unknown slug surfaces as
// ErrJobNotFound.
func (s *Service) ListMembership(ctx context.Context, userID int64, jobSlug string) ([]ListMembership, error) {
	jobID, err := s.repo.JobIDBySlug(ctx, jobSlug)
	if err != nil {
		return nil, err
	}
	return s.repo.MembershipForJob(ctx, userID, jobID)
}

// validName trims the name and enforces the 1..maxNameLen bound (counted in runes, to
// match the DB CHECK's character semantics — names are often Cyrillic), returning the
// trimmed value or ErrInvalidName.
func validName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > maxNameLen {
		return "", ErrInvalidName
	}
	return name, nil
}

// validDescription trims the description and enforces the maxDescriptionLen bound
// (counted in runes, matching validName and the DB CHECK's character semantics). An
// empty description is valid — it means "no description".
func validDescription(description string) (string, error) {
	description = strings.TrimSpace(description)
	if utf8.RuneCountInString(description) > maxDescriptionLen {
		return "", ErrInvalidDescription
	}
	return description, nil
}
