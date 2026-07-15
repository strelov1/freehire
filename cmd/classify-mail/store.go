package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/maillink"
)

// dbStore adapts the generated queries + connection pool to maillink.Store. The
// success path (SetEmailClassification + optional stage advance + delete outbox
// row) runs in one transaction here.
type dbStore struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

func newDBStore(pool *pgxpool.Pool) *dbStore {
	return &dbStore{pool: pool, q: db.New(pool)}
}

func (s *dbStore) EnqueuePending(ctx context.Context) (int64, error) {
	return s.q.EnqueuePendingEmailClassification(ctx)
}

func (s *dbStore) ClaimBatch(ctx context.Context, leaseSeconds, batchSize int) ([]maillink.Claimed, error) {
	rows, err := s.q.ClaimEmailClassificationBatch(ctx, db.ClaimEmailClassificationBatchParams{
		LeaseSeconds: int32(leaseSeconds),
		BatchSize:    int32(batchSize),
	})
	if err != nil {
		return nil, err
	}
	out := make([]maillink.Claimed, len(rows))
	for i, r := range rows {
		out[i] = maillink.Claimed{
			OutboxID: r.ID,
			EmailID:  r.EmailID,
			UserID:   r.UserID,
			ThreadID: r.ThreadID,
			FromName: r.FromName,
			Subject:  r.Subject,
			Body:     r.BodyText,
			BodyHTML: r.BodyHtml,
		}
	}
	return out, nil
}

func (s *dbStore) Applications(ctx context.Context, userID int64) ([]maillink.Application, error) {
	rows, err := s.q.ListUserApplicationsForMatch(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]maillink.Application, len(rows))
	for i, r := range rows {
		out[i] = maillink.Application{JobID: r.ID, Company: r.Company}
	}
	return out, nil
}

func (s *dbStore) ThreadLinks(ctx context.Context, userID int64) (map[string]int64, error) {
	rows, err := s.q.ListUserEmailThreadLinks(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make(map[string]int64, len(rows))
	for _, r := range rows {
		if r.JobID.Valid {
			out[r.ThreadID] = r.JobID.Int64
		}
	}
	return out, nil
}

func (s *dbStore) CurrentStage(ctx context.Context, userID, jobID int64) (string, error) {
	stage, err := s.q.GetUserJobStage(ctx, db.GetUserJobStageParams{UserID: userID, JobID: jobID})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return stage, err
}

func (s *dbStore) Save(ctx context.Context, outboxID, userID int64, r maillink.Result, model string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.q.WithTx(tx)
	matched := r.JobID != 0 || r.SuggestedJobID != 0
	if err := qtx.SetEmailClassification(ctx, db.SetEmailClassificationParams{
		JobID:           int8OrNull(r.JobID),
		SuggestedJobID:  int8OrNull(r.SuggestedJobID),
		LinkSource:      textOrNull(r.LinkSource),
		MatchConfidence: pgtype.Float4{Float32: float32(r.Confidence), Valid: matched},
		StatusSignal:    textOrNull(string(r.Signal)),
		Model:           pgtype.Text{String: model, Valid: true},
		ID:              r.EmailID,
		UserID:          userID,
	}); err != nil {
		return fmt.Errorf("set classification: %w", err)
	}
	if r.AdvanceStageTo != "" {
		if err := qtx.AdvanceUserJobStage(ctx, db.AdvanceUserJobStageParams{
			UserID: userID,
			JobID:  r.JobID,
			Stage:  pgtype.Text{String: r.AdvanceStageTo, Valid: true},
		}); err != nil {
			return fmt.Errorf("advance stage: %w", err)
		}
	}
	if err := qtx.DeleteEmailClassificationOutbox(ctx, outboxID); err != nil {
		return fmt.Errorf("delete outbox entry: %w", err)
	}
	return tx.Commit(ctx)
}

func (s *dbStore) Fail(ctx context.Context, outboxID int64, cause string, maxAttempts int) error {
	return s.q.FailEmailClassification(ctx, db.FailEmailClassificationParams{
		LastError:   cause,
		MaxAttempts: int32(maxAttempts),
		ID:          outboxID,
	})
}

func int8OrNull(v int64) pgtype.Int8 {
	return pgtype.Int8{Int64: v, Valid: v != 0}
}

func textOrNull(s string) pgtype.Text {
	return pgtype.Text{String: s, Valid: s != ""}
}
