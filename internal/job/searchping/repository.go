package searchping

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
)

// PostgresRepository is the Repository backed by the generated queries. It holds no
// policy: which postings are eligible lives in the SQL, and how much of a budget is
// left lives in the Runner above it.
type PostgresRepository struct {
	q *db.Queries
}

// NewPostgresRepository wraps the generated query set.
func NewPostgresRepository(q *db.Queries) *PostgresRepository {
	return &PostgresRepository{q: q}
}

func (r *PostgresRepository) JobsToPing(ctx context.Context, engine string, limit int32) ([]Candidate, error) {
	rows, err := r.q.ListJobsToPing(ctx, db.ListJobsToPingParams{Engine: engine, BatchSize: limit})
	if err != nil {
		return nil, err
	}
	candidates := make([]Candidate, 0, len(rows))
	for _, row := range rows {
		candidates = append(candidates, Candidate{JobID: row.ID, Slug: row.PublicSlug})
	}
	return candidates, nil
}

func (r *PostgresRepository) ClosedJobsToPing(ctx context.Context, engine string, limit int32) ([]Candidate, error) {
	rows, err := r.q.ListClosedJobsToPing(ctx, db.ListClosedJobsToPingParams{Engine: engine, BatchSize: limit})
	if err != nil {
		return nil, err
	}
	candidates := make([]Candidate, 0, len(rows))
	for _, row := range rows {
		candidates = append(candidates, Candidate{JobID: row.ID, Slug: row.PublicSlug})
	}
	return candidates, nil
}

func (r *PostgresRepository) CompaniesToPing(ctx context.Context, engine string, limit int32) ([]Candidate, error) {
	slugs, err := r.q.ListCompaniesToPing(ctx, db.ListCompaniesToPingParams{Engine: engine, BatchSize: limit})
	if err != nil {
		return nil, err
	}
	candidates := make([]Candidate, 0, len(slugs))
	for _, slug := range slugs {
		candidates = append(candidates, Candidate{Company: slug, Slug: slug})
	}
	return candidates, nil
}

func (r *PostgresRepository) RecordPing(ctx context.Context, c Candidate, engine string, kind Kind) error {
	if kind.isCompany() {
		return r.q.RecordCompanySearchPing(ctx, db.RecordCompanySearchPingParams{
			CompanySlug: c.Company,
			Engine:      engine,
		})
	}
	return r.q.RecordJobSearchPing(ctx, db.RecordJobSearchPingParams{
		JobID:  c.JobID,
		Engine: engine,
		Kind:   string(kind),
	})
}

// PingsSince sums BOTH ledgers. The budget belongs to the engine and a company page
// costs it exactly what a posting does, so counting only one would let the day's
// allowance be spent close to twice over the moment an engine takes both.
func (r *PostgresRepository) PingsSince(ctx context.Context, engine string, since time.Time) (int64, error) {
	at := pgtype.Timestamptz{Time: since, Valid: true}

	jobs, err := r.q.CountJobSearchPingsSince(ctx, db.CountJobSearchPingsSinceParams{Engine: engine, Since: at})
	if err != nil {
		return 0, err
	}
	companies, err := r.q.CountCompanySearchPingsSince(ctx, db.CountCompanySearchPingsSinceParams{Engine: engine, Since: at})
	if err != nil {
		return 0, err
	}
	return jobs + companies, nil
}
