package main

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/pipeline"
)

// dbStore adapts the generated queries + connection pool to pipeline.Store. Save runs
// the job upsert and the gated enrichment enqueue in one transaction, so a newly
// ingested job is queued for enrichment atomically with its write.
type dbStore struct {
	pool          *pgxpool.Pool
	q             *db.Queries
	targetVersion int32
}

func newDBStore(pool *pgxpool.Pool, targetVersion int) *dbStore {
	return &dbStore{pool: pool, q: db.New(pool), targetVersion: int32(targetVersion)}
}

func (s *dbStore) Save(ctx context.Context, job pipeline.Job) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	qtx := s.q.WithTx(tx)
	saved, err := qtx.UpsertJob(ctx, db.UpsertJobParams{
		Source:      job.Source,
		ExternalID:  job.ExternalID,
		URL:         job.URL,
		Title:       job.Title,
		Company:     job.Company,
		CompanySlug: job.CompanySlug,
		PublicSlug:  job.PublicSlug,
		Location:    job.Location,
		Remote:      job.Remote,
		Description: job.Description,
		PostedAt:    toTimestamptz(job.PostedAt),
		Countries:   job.Countries,
		Regions:     job.Regions,
		WorkMode:    job.WorkMode,
		Skills:      job.Skills,
		Seniority:   job.Seniority,
		Category:    job.Category,

		PostingLanguage:    job.PostingLanguage,
		EmploymentType:     job.EmploymentType,
		EducationLevel:     job.EducationLevel,
		ExperienceYearsMin: toInt4(job.ExperienceYearsMin),
	})
	if err != nil {
		return fmt.Errorf("upsert job: %w", err)
	}

	if _, err := qtx.EnqueueJobEnrichment(ctx, db.EnqueueJobEnrichmentParams{
		TargetVersion: s.targetVersion,
		JobID:         saved.ID,
	}); err != nil {
		return fmt.Errorf("enqueue enrichment: %w", err)
	}

	return tx.Commit(ctx)
}

// toTimestamptz maps an optional posted_at to the pgtype the generated params expect;
// a nil time becomes SQL NULL.
func toTimestamptz(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

// toInt4 maps an optional int (e.g. experience_years_min) to the pgtype the generated
// params expect; a nil pointer becomes SQL NULL.
func toInt4(n *int) pgtype.Int4 {
	if n == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: int32(*n), Valid: true}
}
