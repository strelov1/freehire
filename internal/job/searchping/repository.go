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

func (r *PostgresRepository) RecordPing(ctx context.Context, jobID int64, engine string, kind Kind) error {
	return r.q.RecordJobSearchPing(ctx, db.RecordJobSearchPingParams{
		JobID:  jobID,
		Engine: engine,
		Kind:   string(kind),
	})
}

func (r *PostgresRepository) PingsSince(ctx context.Context, engine string, since time.Time) (int64, error) {
	return r.q.CountJobSearchPingsSince(ctx, db.CountJobSearchPingsSinceParams{
		Engine: engine,
		Since:  pgtype.Timestamptz{Time: since, Valid: true},
	})
}
