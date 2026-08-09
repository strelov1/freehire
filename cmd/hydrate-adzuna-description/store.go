package main

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/adzunadesc"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/jobhash"
)

// dbStore adapts the generated queries + pool to adzunadesc.Store.
type dbStore struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

func newDBStore(pool *pgxpool.Pool) *dbStore {
	return &dbStore{pool: pool, q: db.New(pool)}
}

var _ adzunadesc.Store = (*dbStore)(nil)

func (s *dbStore) Claim(ctx context.Context, batch, leaseSeconds int) ([]adzunadesc.Claimed, error) {
	rows, err := s.q.ClaimAdzunaDescriptionBatch(ctx, db.ClaimAdzunaDescriptionBatchParams{
		LeaseSeconds: int32(leaseSeconds),
		BatchSize:    int32(batch),
	})
	if err != nil {
		return nil, err
	}
	out := make([]adzunadesc.Claimed, 0, len(rows))
	for _, r := range rows {
		out = append(out, adzunadesc.Claimed{OutboxID: r.ID, JobID: r.JobID, URL: r.URL})
	}
	return out, nil
}

// Save rewrites the job's description, recomputing content_hash from the row's OTHER
// indexed fields so the row re-indexes correctly (mirrors cmd/backfill-echojobs), queues it
// for the live search index so the fuller text reaches search without a manual `make
// reindex`, marks the job hydrated (closing the ingest enqueue gate for good), and retires
// the queue entry — all in one transaction, so a capture is never stored without also being
// retired and marked, nor the reverse.
func (s *dbStore) Save(ctx context.Context, outboxID, jobID int64, description string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.q.WithTx(tx)

	j, err := qtx.GetJob(ctx, jobID)
	if err != nil {
		return fmt.Errorf("load job %d: %w", jobID, err)
	}
	hash := jobhash.OfRow(j, description)

	if _, err := qtx.UpdateJobDescription(ctx, db.UpdateJobDescriptionParams{
		ID:          jobID,
		Description: description,
		ContentHash: pgtype.Text{String: hash, Valid: true},
	}); err != nil {
		return fmt.Errorf("update description for job %d: %w", jobID, err)
	}
	if err := qtx.EnqueueSearchOutbox(ctx, jobID); err != nil {
		return fmt.Errorf("enqueue search outbox for job %d: %w", jobID, err)
	}
	if err := qtx.MarkAdzunaDescriptionHydrated(ctx, jobID); err != nil {
		return fmt.Errorf("mark job %d hydrated: %w", jobID, err)
	}
	if err := qtx.DeleteAdzunaDescriptionEntry(ctx, outboxID); err != nil {
		return fmt.Errorf("retire capture %d: %w", outboxID, err)
	}
	return tx.Commit(ctx)
}

// Discard retires a capture whose URL drifted to the ad-network redirect since it was
// enqueued. No description is stored and the job is NOT marked hydrated, so a later crawl
// that finds it eligible again (Adzuna's own hosted page) is free to re-enqueue it.
func (s *dbStore) Discard(ctx context.Context, outboxID int64) error {
	return s.q.DeleteAdzunaDescriptionEntry(ctx, outboxID)
}

func (s *dbStore) Fail(ctx context.Context, outboxID int64, errMsg string, maxAttempts int) (bool, error) {
	row, err := s.q.RecordAdzunaDescriptionFailure(ctx, db.RecordAdzunaDescriptionFailureParams{
		ID:          outboxID,
		LastError:   errMsg,
		MaxAttempts: int32(maxAttempts),
	})
	if err != nil {
		return false, err
	}
	return row.FailedAt.Valid, nil
}
