package main

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/embed"
	"github.com/strelov1/freehire/internal/enrich"
)

// dbStore adapts the generated queries + pool to embed.Store. It is the only place the
// runner's domain operations meet the DB layer; each success path (stamp/clear + delete
// outbox for a whole batch) runs in one transaction here.
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

func (s *dbStore) Jobs(ctx context.Context, ids []int64) ([]db.Job, error) {
	return s.q.GetJobsByIDs(ctx, ids)
}

func (s *dbStore) CompleteOpen(ctx context.Context, entries []embed.Claimed, model string, vectors map[int64][]float32) error {
	jobIDs, outboxIDs := splitIDs(entries)
	return s.tx(ctx, func(qtx *db.Queries) error {
		if err := qtx.StampSemanticEmbeddedBatch(ctx, db.StampSemanticEmbeddedBatchParams{
			Model: model, Ids: jobIDs,
		}); err != nil {
			return fmt.Errorf("stamp: %w", err)
		}
		// Persist each job's vector in the same transaction as the stamp, so a job is
		// never marked embedded without its vector reaching Postgres. A missing vector
		// for an entry is skipped (leaves the column untouched) rather than nulling it.
		for _, e := range entries {
			v, ok := vectors[e.JobID]
			if !ok {
				continue
			}
			if err := qtx.SetSemanticEmbedding(ctx, db.SetSemanticEmbeddingParams{
				ID: e.JobID, Embedding: v,
			}); err != nil {
				return fmt.Errorf("set embedding (job %d): %w", e.JobID, err)
			}
		}
		return qtx.DeleteSemanticEntriesBatch(ctx, outboxIDs)
	})
}

func (s *dbStore) CompleteClosed(ctx context.Context, entries []embed.Claimed) error {
	jobIDs, outboxIDs := splitIDs(entries)
	return s.tx(ctx, func(qtx *db.Queries) error {
		if err := qtx.ClearSemanticEmbeddedBatch(ctx, jobIDs); err != nil {
			return fmt.Errorf("clear: %w", err)
		}
		return qtx.DeleteSemanticEntriesBatch(ctx, outboxIDs)
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

// splitIDs pulls the parallel job-id and outbox-id slices a batch completion needs.
func splitIDs(entries []embed.Claimed) (jobIDs, outboxIDs []int64) {
	jobIDs = make([]int64, len(entries))
	outboxIDs = make([]int64, len(entries))
	for i, e := range entries {
		jobIDs[i] = e.JobID
		outboxIDs[i] = e.OutboxID
	}
	return jobIDs, outboxIDs
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
