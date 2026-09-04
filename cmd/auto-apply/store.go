package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/application/appevent"
	"github.com/strelov1/freehire/internal/application/autoapply"
	"github.com/strelov1/freehire/internal/platform/db"
)

// dbStore adapts the generated queries + pool to autoapply.Store. It is the only place the
// runner's domain operations meet the DB layer, mirroring cmd/capture-apply-form's dbStore.
type dbStore struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

func newDBStore(pool *pgxpool.Pool) *dbStore {
	return &dbStore{pool: pool, q: db.New(pool)}
}

var _ autoapply.Store = (*dbStore)(nil)

func (s *dbStore) Claim(ctx context.Context, batch, leaseSeconds int) ([]autoapply.Claimed, error) {
	rows, err := s.q.ClaimAutoApplyBatch(ctx, db.ClaimAutoApplyBatchParams{
		LeaseSeconds: int32(leaseSeconds),
		BatchSize:    int32(batch),
	})
	if err != nil {
		return nil, err
	}
	out := make([]autoapply.Claimed, 0, len(rows))
	for _, r := range rows {
		c := autoapply.Claimed{
			QueueID:    r.ID,
			UserID:     r.UserID,
			JobID:      r.JobID,
			Provider:   r.Source,
			ExternalID: r.ExternalID,
			JobURL:     r.URL,
		}
		// Never nil in practice — ClaimAutoApplyBatch's WHERE requires tailored_cv_id IS
		// NOT NULL — but a claim with no tailored CV degrades to a zero-value id rather
		// than panicking on a nil dereference, consistent with everything else here
		// treating a surprising row as data, not a crash.
		if r.TailoredCvID != nil {
			c.TailoredCVID = *r.TailoredCvID
		}
		out = append(out, c)
	}
	return out, nil
}

// Submit records the application and retires the queue entry in ONE transaction — split
// apart, the two halves fail in opposite bad ways: a recorded application with a live queue
// entry gets reclaimed and the sidecar asked to submit again (the spec's "never twice"
// requirement is what this transaction is FOR), and a retired entry with no application
// recorded loses the fact that a real submission happened.
//
// LockJobForApply first, then MarkJobApplied, is the exact same locked-transaction shape
// jobtracking.QueriesRepository.MarkApplied already runs for a candidate's own manual apply
// — reused directly rather than re-derived, so the applied_count/ledger guarantee is the one
// guarantee, not a second implementation of it. EventSource is appevent.SourceSystem: this
// submission was not typed by the candidate or drafted by the assistant, it is the platform
// acting on their behalf — exactly what that source already means for an auto-expired
// application in the other direction.
func (s *dbStore) Submit(ctx context.Context, c autoapply.Claimed) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.q.WithTx(tx)
	if err := qtx.LockJobForApply(ctx, c.JobID); err != nil {
		return fmt.Errorf("lock job %d: %w", c.JobID, err)
	}
	if _, err := qtx.MarkJobApplied(ctx, db.MarkJobAppliedParams{
		UserID:      c.UserID,
		JobID:       c.JobID,
		At:          pgtype.Timestamptz{},
		EventSource: appevent.SourceSystem,
	}); err != nil {
		return fmt.Errorf("mark job %d applied for user %d: %w", c.JobID, c.UserID, err)
	}
	if err := qtx.DeleteAutoApplyEntry(ctx, c.QueueID); err != nil {
		return fmt.Errorf("retire queue entry %d: %w", c.QueueID, err)
	}
	return tx.Commit(ctx)
}

func (s *dbStore) Park(ctx context.Context, queueID int64, unmapped []autoapply.UnmappedField, reason string) error {
	payload, err := json.Marshal(unmapped)
	if err != nil {
		return fmt.Errorf("encode unmapped fields: %w", err)
	}
	return s.q.MarkAutoApplyBlocked(ctx, db.MarkAutoApplyBlockedParams{
		ID:        queueID,
		LastError: reason,
		Unmapped:  payload,
	})
}

func (s *dbStore) Fail(ctx context.Context, queueID int64, errMsg string, maxAttempts int) (bool, error) {
	row, err := s.q.RecordAutoApplyFailure(ctx, db.RecordAutoApplyFailureParams{
		ID:          queueID,
		LastError:   errMsg,
		MaxAttempts: int32(maxAttempts),
	})
	if err != nil {
		return false, err
	}
	return row.FailedAt.Valid, nil
}
