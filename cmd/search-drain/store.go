package main

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/searchdrain"
)

// dbStore adapts the generated queries + pool to searchdrain.Store.
type dbStore struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

func newDBStore(pool *pgxpool.Pool) *dbStore {
	return &dbStore{pool: pool, q: db.New(pool)}
}

func (s *dbStore) Claim(ctx context.Context, batch, leaseSeconds int) ([]searchdrain.Claimed, error) {
	rows, err := s.q.ClaimSearchOutboxBatch(ctx, db.ClaimSearchOutboxBatchParams{
		LeaseSeconds: int32(leaseSeconds),
		BatchSize:    int32(batch),
	})
	if err != nil {
		return nil, err
	}
	out := make([]searchdrain.Claimed, len(rows))
	for i, r := range rows {
		out[i] = searchdrain.Claimed{OutboxID: r.ID, JobID: r.JobID}
	}
	return out, nil
}

func (s *dbStore) Jobs(ctx context.Context, ids []int64) ([]db.Job, error) {
	return s.q.GetJobsByIDs(ctx, ids)
}

func (s *dbStore) Complete(ctx context.Context, entries []searchdrain.Claimed) error {
	ids := make([]int64, len(entries))
	for i, e := range entries {
		ids[i] = e.OutboxID
	}
	return s.q.DeleteSearchOutboxEntries(ctx, ids)
}

func (s *dbStore) Reap(ctx context.Context, maxRows int) (int64, error) {
	return s.q.DeleteIneligibleSearchOutbox(ctx, int32(maxRows))
}

func (s *dbStore) Fail(ctx context.Context, outboxID int64, errMsg string, maxAttempts int) (bool, error) {
	row, err := s.q.RecordSearchOutboxFailure(ctx, db.RecordSearchOutboxFailureParams{
		LastError:   errMsg,
		MaxAttempts: int32(maxAttempts),
		ID:          outboxID,
	})
	if err != nil {
		return false, err
	}
	return row.FailedAt.Valid, nil
}
