package moderation

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/enrich"
	"github.com/strelov1/freehire/internal/job"
	"github.com/strelov1/freehire/internal/pgconv"
)

// Compile-time proof that QueriesRepository satisfies Repository.
var _ Repository = (*QueriesRepository)(nil)

// QueriesRepository adapts *db.Queries + a pool to the Repository. targetVersion is the
// enrichment schema version a newly created job is enqueued at (enrich.Version), so a
// manual job flows into enrichment like every other source.
type QueriesRepository struct {
	q             *db.Queries
	pool          *pgxpool.Pool
	targetVersion int32
}

// NewQueriesRepository constructs a QueriesRepository.
func NewQueriesRepository(q *db.Queries, pool *pgxpool.Pool, targetVersion int32) *QueriesRepository {
	return &QueriesRepository{q: q, pool: pool, targetVersion: targetVersion}
}

// Create runs the manual-job upsert and the gated enrichment enqueue in one transaction,
// so a newly created job is queued for enrichment atomically with its write (the same
// transactional-outbox property as the ingest write path).
func (r *QueriesRepository) Create(ctx context.Context, f job.Fields, actorID int64) (job.Job, job.Extras, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return job.Job{}, job.Extras{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := r.q.WithTx(tx)
	row, err := qtx.UpsertManualJob(ctx, db.UpsertManualJobParams{
		Source:      f.Source,
		ExternalID:  f.ExternalID,
		URL:         f.URL,
		Title:       f.Title,
		Company:     f.Company,
		CompanySlug: f.CompanySlug,
		Location:    f.Location,
		Remote:      f.Remote,
		Description: f.Description,
		PostedAt:    pgconv.Timestamptz(f.PostedAt),
		PublicSlug:  f.PublicSlug,
		Countries:   f.Countries,
		Regions:     f.Regions,
		Cities:      f.Cities,
		WorkMode:    f.WorkMode,
		Skills:      f.Skills,
		Seniority:   f.Seniority,
		Category:    f.Category,

		PostingLanguage:    f.PostingLanguage,
		EmploymentType:     f.EmploymentType,
		EducationLevel:     f.EducationLevel,
		EnglishLevel:       f.EnglishLevel,
		ExperienceYearsMin: pgconv.Int4(f.ExperienceYearsMin),

		CreatedBy: actorID,
		UpdatedBy: actorID,
	})
	if err != nil {
		return job.Job{}, job.Extras{}, fmt.Errorf("upsert manual job: %w", err)
	}
	if _, err := qtx.EnqueueJobEnrichment(ctx, db.EnqueueJobEnrichmentParams{
		TargetVersion:     r.targetVersion,
		JobID:             row.ID,
		ExcludeCategories: enrich.NonTechCategories,
	}); err != nil {
		return job.Job{}, job.Extras{}, fmt.Errorf("enqueue enrichment: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return job.Job{}, job.Extras{}, err
	}
	return job.FromRow(row)
}

// BySlug loads a job by its public slug, returning ErrJobNotFound when no job matches or
// the matched job was not moderator-authored (created_by IS NULL) — so the edit path can
// never touch an automated-source (ATS/telegram) vacancy, whatever its declared source.
func (r *QueriesRepository) BySlug(ctx context.Context, slug string) (job.Job, job.Extras, error) {
	row, err := r.q.GetJobBySlug(ctx, slug)
	if errors.Is(err, pgx.ErrNoRows) {
		return job.Job{}, job.Extras{}, ErrJobNotFound
	}
	if err != nil {
		return job.Job{}, job.Extras{}, err
	}
	if !row.CreatedBy.Valid {
		return job.Job{}, job.Extras{}, ErrJobNotFound
	}
	return job.FromRow(row)
}

// Update writes the full resulting row for a moderator-authored job. The query's
// created_by scope means a missing or non-moderator-created slug affects no row
// (ErrNoRows → ErrJobNotFound).
func (r *QueriesRepository) Update(ctx context.Context, slug string, f job.Fields, actorID int64) (job.Job, job.Extras, error) {
	row, err := r.q.UpdateManualJob(ctx, db.UpdateManualJobParams{
		PublicSlug:  slug,
		Title:       f.Title,
		Company:     f.Company,
		CompanySlug: f.CompanySlug,
		Location:    f.Location,
		Remote:      f.Remote,
		Description: f.Description,
		PostedAt:    pgconv.Timestamptz(f.PostedAt),
		Countries:   f.Countries,
		Regions:     f.Regions,
		Cities:      f.Cities,
		WorkMode:    f.WorkMode,
		Skills:      f.Skills,
		Seniority:   f.Seniority,
		Category:    f.Category,

		PostingLanguage:    f.PostingLanguage,
		EmploymentType:     f.EmploymentType,
		EducationLevel:     f.EducationLevel,
		EnglishLevel:       f.EnglishLevel,
		ExperienceYearsMin: pgconv.Int4(f.ExperienceYearsMin),

		UpdatedBy: actorID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return job.Job{}, job.Extras{}, ErrJobNotFound
	}
	if err != nil {
		return job.Job{}, job.Extras{}, err
	}
	return job.FromRow(row)
}
