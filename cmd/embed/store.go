package main

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/embed"
	"github.com/strelov1/freehire/internal/enrich"
)

// dbStore adapts the generated queries + pool to embed.Store. It is the only place the
// runner's domain operations meet the DB layer; each success path (stamp/clear + delete
// outbox) runs in one transaction here.
type dbStore struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

func newDBStore(pool *pgxpool.Pool) *dbStore {
	return &dbStore{pool: pool, q: db.New(pool)}
}

// Enqueue reuses enrich.NonTechCategories as the exclusion set, so embed budget stays on
// technical roles from day one — the same gate cmd/enrich applies.
func (s *dbStore) Enqueue(ctx context.Context, targetModel string) (int64, error) {
	return s.q.EnqueuePendingSemanticJobs(ctx, db.EnqueuePendingSemanticJobsParams{
		TargetModel:       targetModel,
		ExcludeCategories: enrich.NonTechCategories,
	})
}

func (s *dbStore) Claim(ctx context.Context, batch, leaseSeconds int) ([]embed.Claimed, error) {
	rows, err := s.q.ClaimSemanticBatch(ctx, db.ClaimSemanticBatchParams{
		LeaseSeconds: int32(leaseSeconds),
		BatchSize:    int32(batch),
	})
	if err != nil {
		return nil, err
	}
	out := make([]embed.Claimed, len(rows))
	for i, r := range rows {
		out[i] = embed.Claimed{OutboxID: r.ID, JobID: r.JobID, Closed: r.Closed}
	}
	return out, nil
}

func (s *dbStore) Job(ctx context.Context, id int64) (db.Job, error) {
	return s.q.GetJob(ctx, id)
}

func (s *dbStore) CompleteOpen(ctx context.Context, entry embed.Claimed, model string, hash pgtype.Text) error {
	return s.tx(ctx, func(qtx *db.Queries) error {
		if err := qtx.StampSemanticEmbedded(ctx, db.StampSemanticEmbeddedParams{
			Model: model, Hash: hash, ID: entry.JobID,
		}); err != nil {
			return fmt.Errorf("stamp: %w", err)
		}
		return qtx.DeleteSemanticEntry(ctx, entry.OutboxID)
	})
}

func (s *dbStore) CompleteClosed(ctx context.Context, entry embed.Claimed) error {
	return s.tx(ctx, func(qtx *db.Queries) error {
		if err := qtx.ClearSemanticEmbedded(ctx, entry.JobID); err != nil {
			return fmt.Errorf("clear: %w", err)
		}
		return qtx.DeleteSemanticEntry(ctx, entry.OutboxID)
	})
}

func (s *dbStore) Fail(ctx context.Context, outboxID int64, errMsg string, maxAttempts int) (bool, error) {
	row, err := s.q.RecordSemanticFailure(ctx, db.RecordSemanticFailureParams{
		LastError:   errMsg,
		MaxAttempts: int32(maxAttempts),
		ID:          outboxID,
	})
	if err != nil {
		return false, err
	}
	return row.FailedAt.Valid, nil
}

// tx runs fn against a transaction, committing on success and rolling back otherwise.
func (s *dbStore) tx(ctx context.Context, fn func(*db.Queries) error) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(s.q.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
