package moderation

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/enrich"
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
func (r *QueriesRepository) Create(ctx context.Context, p db.UpsertManualJobParams) (db.Job, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return db.Job{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := r.q.WithTx(tx)
	job, err := qtx.UpsertManualJob(ctx, p)
	if err != nil {
		return db.Job{}, fmt.Errorf("upsert manual job: %w", err)
	}
	if _, err := qtx.EnqueueJobEnrichment(ctx, db.EnqueueJobEnrichmentParams{
		TargetVersion:     r.targetVersion,
		JobID:             job.ID,
		ExcludeCategories: enrich.NonTechCategories,
	}); err != nil {
		return db.Job{}, fmt.Errorf("enqueue enrichment: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return db.Job{}, err
	}
	return job, nil
}

// BySlug loads a job by its public slug, returning ErrJobNotFound when no job matches or
// the matched job was not moderator-authored (created_by IS NULL) — so the edit path can
// never touch an automated-source (ATS/telegram) vacancy, whatever its declared source.
func (r *QueriesRepository) BySlug(ctx context.Context, slug string) (db.Job, error) {
	job, err := r.q.GetJobBySlug(ctx, slug)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Job{}, ErrJobNotFound
	}
	if err != nil {
		return db.Job{}, err
	}
	if !job.CreatedBy.Valid {
		return db.Job{}, ErrJobNotFound
	}
	return job, nil
}

// Update writes the full resulting row for a moderator-authored job. The query's
// created_by scope means a missing or non-moderator-created slug affects no row
// (ErrNoRows → ErrJobNotFound).
func (r *QueriesRepository) Update(ctx context.Context, p db.UpdateManualJobParams) (db.Job, error) {
	job, err := r.q.UpdateManualJob(ctx, p)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.Job{}, ErrJobNotFound
	}
	if err != nil {
		return db.Job{}, err
	}
	return job, nil
}
