package joblists

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
	"github.com/strelov1/freehire/internal/platform/pgerr"
)

// Compile-time proof that QueriesRepository satisfies Repository.
var _ Repository = (*QueriesRepository)(nil)

// QueriesRepository adapts *db.Queries to the Repository. It maps the relevant
// Postgres conditions onto package sentinels: a unique violation on create/update/
// share → duplicate name / slug taken, no row on an owner-scoped read/update →
// not found, no row affected on delete → not found.
type QueriesRepository struct {
	q *db.Queries
}

// NewQueriesRepository constructs a QueriesRepository.
func NewQueriesRepository(q *db.Queries) *QueriesRepository {
	return &QueriesRepository{q: q}
}

// List returns a user's job lists, most recently updated first, each carrying its
// job count.
func (r *QueriesRepository) List(ctx context.Context, userID int64) ([]JobList, error) {
	rows, err := r.q.ListJobLists(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]JobList, len(rows))
	for i, row := range rows {
		out[i] = JobList{
			ID:          row.ID,
			Name:        row.Name,
			Description: row.Description,
			PublicSlug:  row.PublicSlug.String,
			JobCount:    row.JobCount,
			CreatedAt:   pgconv.TimePtr(row.CreatedAt),
			UpdatedAt:   pgconv.TimePtr(row.UpdatedAt),
		}
	}
	return out, nil
}

// Count returns how many job lists the user has (the cap check input).
func (r *QueriesRepository) Count(ctx context.Context, userID int64) (int64, error) {
	return r.q.CountJobLists(ctx, userID)
}

// Create inserts a job list. The UNIQUE (user_id, name) violation maps to
// ErrDuplicateName.
func (r *QueriesRepository) Create(ctx context.Context, userID int64, name, description string) (JobList, error) {
	row, err := r.q.CreateJobList(ctx, db.CreateJobListParams{
		UserID:      userID,
		Name:        name,
		Description: description,
	})
	if pgerr.IsUniqueViolation(err) {
		return JobList{}, ErrDuplicateName
	}
	if err != nil {
		return JobList{}, err
	}
	return fromRow(row), nil
}

// Update overwrites a job list scoped to its owner. A nil name/description is left
// unchanged (NULL param). No matching row (wrong id or another user's) returns no
// row → ErrNotFound; a name collision → ErrDuplicateName. The returned JobCount
// reflects the list's real membership (the query computes it directly, the same way
// List does) rather than the zero value fromRow would otherwise carry.
func (r *QueriesRepository) Update(ctx context.Context, id, userID int64, name, description *string) (JobList, error) {
	row, err := r.q.UpdateJobList(ctx, db.UpdateJobListParams{
		ID:          id,
		UserID:      userID,
		Name:        textPtr(name),
		Description: textPtr(description),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return JobList{}, ErrNotFound
	}
	if pgerr.IsUniqueViolation(err) {
		return JobList{}, ErrDuplicateName
	}
	if err != nil {
		return JobList{}, err
	}
	return JobList{
		ID:          row.ID,
		Name:        row.Name,
		Description: row.Description,
		PublicSlug:  row.PublicSlug.String,
		JobCount:    row.JobCount,
		CreatedAt:   pgconv.TimePtr(row.CreatedAt),
		UpdatedAt:   pgconv.TimePtr(row.UpdatedAt),
	}, nil
}

// Delete removes a job list scoped to its owner, mapping "no row affected" (missing
// or non-owned) to ErrNotFound.
func (r *QueriesRepository) Delete(ctx context.Context, id, userID int64) error {
	affected, err := r.q.DeleteJobList(ctx, db.DeleteJobListParams{ID: id, UserID: userID})
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

// Get reads one of a user's job lists, owner-scoped, mapping "no row" (missing or
// another user's) to ErrNotFound.
func (r *QueriesRepository) Get(ctx context.Context, id, userID int64) (JobList, error) {
	row, err := r.q.GetJobList(ctx, db.GetJobListParams{ID: id, UserID: userID})
	if errors.Is(err, pgx.ErrNoRows) {
		return JobList{}, ErrNotFound
	}
	if err != nil {
		return JobList{}, err
	}
	return fromRow(row), nil
}

// CountItems returns how many jobs a list holds (the per-list cap check input).
func (r *QueriesRepository) CountItems(ctx context.Context, id int64) (int64, error) {
	return r.q.CountJobListItems(ctx, id)
}

// HasItem reports whether a job already belongs to a list.
func (r *QueriesRepository) HasItem(ctx context.Context, id, jobID int64) (bool, error) {
	return r.q.JobListHasItem(ctx, db.JobListHasItemParams{ListID: id, JobID: jobID})
}

// AddJob adds a job to a list the caller already confirmed ownership of. Idempotent:
// an already-present job changes nothing (ON CONFLICT DO NOTHING).
func (r *QueriesRepository) AddJob(ctx context.Context, id, jobID int64) error {
	return r.q.AddJobListItem(ctx, db.AddJobListItemParams{ListID: id, JobID: jobID})
}

// RemoveJob removes a job from a list the caller already confirmed ownership of.
// Idempotent: an absent job changes nothing.
func (r *QueriesRepository) RemoveJob(ctx context.Context, id, jobID int64) error {
	return r.q.RemoveJobListItem(ctx, db.RemoveJobListItemParams{ListID: id, JobID: jobID})
}

// JobIDBySlug returns the internal job id for the given public slug, or
// ErrJobNotFound when no job matches.
func (r *QueriesRepository) JobIDBySlug(ctx context.Context, slug string) (int64, error) {
	id, err := r.q.GetJobIDBySlug(ctx, slug)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrJobNotFound
	}
	if err != nil {
		return 0, err
	}
	return id, nil
}

// SetPublicSlug publishes a list scoped to its owner, mapping a slug UNIQUE collision
// to ErrSlugTaken (retried by the service) and "no row" (missing or non-owned) to
// ErrNotFound. The returned JobCount reflects the list's real membership (see Update).
func (r *QueriesRepository) SetPublicSlug(ctx context.Context, id, userID int64, publicSlug string) (JobList, error) {
	row, err := r.q.SetJobListPublicSlug(ctx, db.SetJobListPublicSlugParams{
		ID:         id,
		UserID:     userID,
		PublicSlug: pgtype.Text{String: publicSlug, Valid: true},
	})
	if pgerr.IsUniqueViolation(err) {
		return JobList{}, ErrSlugTaken
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return JobList{}, ErrNotFound
	}
	if err != nil {
		return JobList{}, err
	}
	return JobList{
		ID:          row.ID,
		Name:        row.Name,
		Description: row.Description,
		PublicSlug:  row.PublicSlug.String,
		JobCount:    row.JobCount,
		CreatedAt:   pgconv.TimePtr(row.CreatedAt),
		UpdatedAt:   pgconv.TimePtr(row.UpdatedAt),
	}, nil
}

// ClearPublicSlug unpublishes a list scoped to its owner, mapping "no row affected"
// (missing or non-owned) to ErrNotFound. Clearing an already-private owned row still
// matches (row count 1), so unshare is an idempotent no-op.
func (r *QueriesRepository) ClearPublicSlug(ctx context.Context, id, userID int64) error {
	affected, err := r.q.ClearJobListPublicSlug(ctx, db.ClearJobListPublicSlugParams{ID: id, UserID: userID})
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

// GetPublicList reads a shared list by slug (no auth, no owner-scoping), mapping "no
// row" (unknown or unshared slug) to ErrNotFound. Jobs are projected through the
// existing jobview card shape, newest-added first; a closed/expired job stays in the
// result, carrying its ClosedAt status like every other surface.
func (r *QueriesRepository) GetPublicList(ctx context.Context, slug string) (PublicJobList, error) {
	list, err := r.q.GetPublicJobListBySlug(ctx, pgtype.Text{String: slug, Valid: true})
	if errors.Is(err, pgx.ErrNoRows) {
		return PublicJobList{}, ErrNotFound
	}
	if err != nil {
		return PublicJobList{}, err
	}

	rows, err := r.q.ListJobListItemCards(ctx, list.ID)
	if err != nil {
		return PublicJobList{}, err
	}
	jobs := make([]jobview.Card, len(rows))
	for i, row := range rows {
		jobs[i] = jobview.NewCard(jobview.CardInput{
			PublicSlug:     row.PublicSlug,
			Title:          row.Title,
			Company:        row.Company,
			ClosedAt:       pgconv.TimePtr(row.ClosedAt),
			WorkMode:       row.WorkMode,
			Seniority:      row.Seniority,
			EmploymentType: row.EmploymentType,
			DictCountries:  row.Countries,
			DictRegions:    row.Regions,
			LLMCountries:   row.LlmCountries,
			LLMRegions:     row.LlmRegions,
			Skills:         row.Skills,
			Collections:    row.Collections,
			PostedAt:       pgconv.TimePtr(row.PostedAt),
			CreatedAt:      pgconv.TimePtr(row.CreatedAt),
			Blurb:          row.Blurb,
		})
	}
	return PublicJobList{Name: list.Name, Description: list.Description, Jobs: jobs}, nil
}

// MembershipForJob returns every one of the user's lists flagged with whether jobID
// belongs to it.
func (r *QueriesRepository) MembershipForJob(ctx context.Context, userID, jobID int64) ([]ListMembership, error) {
	rows, err := r.q.ListJobListMembershipForJob(ctx, db.ListJobListMembershipForJobParams{
		UserID: userID,
		JobID:  jobID,
	})
	if err != nil {
		return nil, err
	}
	out := make([]ListMembership, len(rows))
	for i, row := range rows {
		out[i] = ListMembership{ID: row.ID, Name: row.Name, InList: row.InList}
	}
	return out, nil
}

// fromRow maps the generated db row to the package domain type, collapsing the
// nullable public_slug column to a plain string (NULL → "") and dropping the owner
// column the use case does not need.
func fromRow(row db.JobList) JobList {
	return JobList{
		ID:          row.ID,
		Name:        row.Name,
		Description: row.Description,
		PublicSlug:  row.PublicSlug.String,
		CreatedAt:   pgconv.TimePtr(row.CreatedAt),
		UpdatedAt:   pgconv.TimePtr(row.UpdatedAt),
	}
}

// textPtr maps an optional string to the pgtype a partial update expects: nil becomes
// the zero (NULL, "leave unchanged") value, a non-nil pointer a valid text (an empty
// string is a real value — "no description" — so it stays valid).
func textPtr(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *s, Valid: true}
}
