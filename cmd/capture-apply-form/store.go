package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/applyform"
	"github.com/strelov1/freehire/internal/db"
)

// dbStore adapts the generated queries + pool to applyform.Store. It is the only place the
// runner's domain operations meet the DB layer.
type dbStore struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

func newDBStore(pool *pgxpool.Pool) *dbStore {
	return &dbStore{pool: pool, q: db.New(pool)}
}

var _ applyform.Store = (*dbStore)(nil)

func (s *dbStore) Claim(ctx context.Context, batch, leaseSeconds int) ([]applyform.Claimed, error) {
	rows, err := s.q.ClaimApplyFormBatch(ctx, db.ClaimApplyFormBatchParams{
		LeaseSeconds: int32(leaseSeconds),
		BatchSize:    int32(batch),
	})
	if err != nil {
		return nil, err
	}
	out := make([]applyform.Claimed, 0, len(rows))
	for _, r := range rows {
		out = append(out, applyform.Claimed{
			OutboxID:   r.ID,
			JobID:      r.JobID,
			Provider:   r.Source,
			ExternalID: r.ExternalID,
		})
	}
	return out, nil
}

// Save stores the form and retires the queue entry in ONE transaction. Split apart, the
// two halves fail in opposite bad ways: a stored form with a live entry is re-fetched
// forever (the enqueue gate would never open again, but the claim keeps coming back), and
// a retired entry without a stored form loses the capture with nothing left to re-queue it.
func (s *dbStore) Save(ctx context.Context, outboxID, jobID int64, form applyform.Form) error {
	payload, err := json.Marshal(form)
	if err != nil {
		return fmt.Errorf("encode apply form: %w", err)
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	// Rollback after Commit is a no-op; suppress its error (matches the other stores).
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.q.WithTx(tx)
	if err := qtx.UpsertApplyForm(ctx, db.UpsertApplyFormParams{
		JobID:    jobID,
		Provider: form.Provider,
		Payload:  payload,
	}); err != nil {
		return fmt.Errorf("upsert apply form: %w", err)
	}
	if err := qtx.DeleteApplyFormEntry(ctx, outboxID); err != nil {
		return fmt.Errorf("retire capture %d: %w", outboxID, err)
	}
	return tx.Commit(ctx)
}

// Discard retires a capture whose posting the platform no longer has. No form is stored:
// there is nothing to store, and the posting itself will be closed by the unseen sweep.
func (s *dbStore) Discard(ctx context.Context, outboxID int64) error {
	return s.q.DeleteApplyFormEntry(ctx, outboxID)
}

func (s *dbStore) Fail(ctx context.Context, outboxID int64, errMsg string, maxAttempts int) (bool, error) {
	row, err := s.q.RecordApplyFormFailure(ctx, db.RecordApplyFormFailureParams{
		ID:          outboxID,
		LastError:   errMsg,
		MaxAttempts: int32(maxAttempts),
	})
	if err != nil {
		return false, err
	}
	return row.FailedAt.Valid, nil
}
