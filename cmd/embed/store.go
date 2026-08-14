package main

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/embed"
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

// Enqueue gates on is_tech = true (see EnqueuePendingSemanticJobs's comment), the same
// gate cmd/enrich applies, so embed budget stays on confirmed-technical roles.
func (s *dbStore) Enqueue(ctx context.Context, targetModel string) (int64, error) {
	return s.q.EnqueuePendingSemanticJobs(ctx, targetModel)
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

func (s *dbStore) CompleteOpen(ctx context.Context, entries []embed.Claimed, model string, chunks map[int64][]embed.ChunkEmbedding) error {
	jobIDs, outboxIDs := splitIDs(entries)
	return s.tx(ctx, func(qtx *db.Queries) error {
		if err := qtx.StampSemanticEmbeddedBatch(ctx, db.StampSemanticEmbeddedBatchParams{
			Model: model, Ids: jobIDs,
		}); err != nil {
			return fmt.Errorf("stamp: %w", err)
		}
		// Replace each job's pgvector-backed chunk rows in the SAME transaction: a job's
		// chunk COUNT can change between embeds (its source text was re-chunked), so
		// "replace" is delete-then-insert, not an upsert — every entry in this batch is
		// replaced, even one with zero new chunks this time (an empty/very short
		// description), so a job never ends up with a mix of stale and fresh rows. The
		// delete is ONE batched call for the whole wave's job ids (mirrors
		// StampSemanticEmbeddedBatch/ClearSimilarComputedAtBatch above, not a per-job
		// round trip — see DeleteJobSemanticChunks' doc comment); only the insert
		// genuinely needs a per-job call, since each job's chunk vectors are their own
		// parallel-array parameter set.
		if err := qtx.DeleteJobSemanticChunks(ctx, jobIDs); err != nil {
			return fmt.Errorf("delete chunks: %w", err)
		}
		// Flatten every entry's chunks into three parallel arrays across the WHOLE
		// wave, then insert them all in ONE round trip — not one INSERT per job. A
		// per-job loop here was exactly what made a prior HF GPU bulk-embed
		// Postgres-bound instead of GPU-bound (see InsertJobSemanticChunks' comment).
		var flatJobIDs []int64
		var flatIndices []int16
		var flatEmbeddings []string
		for _, e := range entries {
			for _, c := range chunks[e.JobID] {
				flatJobIDs = append(flatJobIDs, e.JobID)
				flatIndices = append(flatIndices, c.ChunkIndex)
				// Each vector travels as pgvector literal TEXT (e.g. "[0.1,0.2,...]"),
				// not a native array element — see InsertJobSemanticChunks' comment
				// for why (no OID codec registered for an array of vectors).
				flatEmbeddings = append(flatEmbeddings, pgvector.NewVector(c.Vector).String())
			}
		}
		if len(flatJobIDs) > 0 {
			if err := qtx.InsertJobSemanticChunks(ctx, db.InsertJobSemanticChunksParams{
				JobIds: flatJobIDs, ChunkIndices: flatIndices, Embeddings: flatEmbeddings,
			}); err != nil {
				return fmt.Errorf("insert chunks: %w", err)
			}
		}
		// A re-embed's fresh chunk set makes any precomputed similar_job_ids stale (or
		// changes the job's eligibility for the precompute entirely), so clear the
		// staleness stamp for the whole batch — cmd/similar-backfill's incremental
		// predicate picks the job back up on its next pass.
		if err := qtx.ClearSimilarComputedAtBatch(ctx, jobIDs); err != nil {
			return fmt.Errorf("clear similar_computed_at: %w", err)
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
		// Delete each closed job's pgvector-backed chunk rows in the same transaction,
		// as ONE batched call for the whole batch's job ids — mirrors the existing
		// ClearSemanticEmbeddedBatch clear above, both the semantics AND the round-trip
		// shape. ON DELETE CASCADE from jobs already covers a hard delete (cmd/prune);
		// this is for the soft close path cascade doesn't reach.
		if err := qtx.DeleteJobSemanticChunks(ctx, jobIDs); err != nil {
			return fmt.Errorf("delete chunks: %w", err)
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
