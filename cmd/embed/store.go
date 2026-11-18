package main

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"

	"github.com/strelov1/freehire/internal/ai/embed"
	"github.com/strelov1/freehire/internal/platform/db"
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

func (s *dbStore) CompleteOpen(ctx context.Context, entries []embed.Claimed, model string, chunks map[int64][]embed.ChunkEmbedding, inputHashes map[int64]string) error {
	jobIDs, outboxIDs := splitIDs(entries)
	return s.tx(ctx, func(qtx *db.Queries) error {
		if err := stamp(ctx, qtx, model, jobIDs, inputHashes); err != nil {
			return err
		}
		// Replace each job's chunk rows in the same transaction: the chunk count can change between
		// embeds, so replace is delete-then-insert, and every entry is replaced even with zero new
		// chunks, so a job never mixes stale and fresh rows. The delete is one batched call for the
		// wave's job ids (see DeleteJobSemanticChunks); only the insert needs a per-job call.
		if err := qtx.DeleteJobSemanticChunks(ctx, jobIDs); err != nil {
			return fmt.Errorf("delete chunks: %w", err)
		}
		// Flatten every entry's chunks into three parallel arrays across the whole wave and insert in
		// one round trip, not one INSERT per job (see InsertJobSemanticChunks).
		var flatJobIDs []int64
		var flatIndices []int16
		var flatEmbeddings []string
		for _, e := range entries {
			for _, c := range chunks[e.JobID] {
				flatJobIDs = append(flatJobIDs, e.JobID)
				flatIndices = append(flatIndices, c.ChunkIndex)
				// Each vector travels as pgvector literal text ("[0.1,0.2,...]"), not a native array element:
				// no OID codec is registered for an array of vectors.
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
		return qtx.DeleteSemanticEntriesBatch(ctx, outboxIDs)
	})
}

// CompleteUnchanged brings the stamp current and drains the entries, nothing else: the
// chunk rows are still the embedding of this text and stay untouched.
func (s *dbStore) CompleteUnchanged(ctx context.Context, entries []embed.Claimed, model string, inputHashes map[int64]string) error {
	jobIDs, outboxIDs := splitIDs(entries)
	return s.tx(ctx, func(qtx *db.Queries) error {
		if err := stamp(ctx, qtx, model, jobIDs, inputHashes); err != nil {
			return err
		}
		return qtx.DeleteSemanticEntriesBatch(ctx, outboxIDs)
	})
}

// stamp records the embed provenance for jobIDs, each paired with its input hash.
func stamp(ctx context.Context, qtx *db.Queries, model string, jobIDs []int64, inputHashes map[int64]string) error {
	hashes := make([]string, len(jobIDs))
	for i, id := range jobIDs {
		hashes[i] = inputHashes[id]
	}
	if err := qtx.StampSemanticEmbeddedBatch(ctx, db.StampSemanticEmbeddedBatchParams{
		Model: model, Ids: jobIDs, InputHashes: hashes,
	}); err != nil {
		return fmt.Errorf("stamp: %w", err)
	}
	return nil
}

func (s *dbStore) CompleteClosed(ctx context.Context, entries []embed.Claimed) error {
	jobIDs, outboxIDs := splitIDs(entries)
	return s.tx(ctx, func(qtx *db.Queries) error {
		if err := qtx.ClearSemanticEmbeddedBatch(ctx, jobIDs); err != nil {
			return fmt.Errorf("clear: %w", err)
		}
		// Delete each closed job's chunk rows in the same transaction, as one batched call.
		// ON DELETE CASCADE covers a hard delete; this covers the soft close the cascade doesn't reach.
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
