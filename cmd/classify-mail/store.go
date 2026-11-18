package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/application/appevent"
	"github.com/strelov1/freehire/internal/application/gmailsync"
	"github.com/strelov1/freehire/internal/application/inbox"
	"github.com/strelov1/freehire/internal/application/maillink"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
)

// domainLearner adapts the gmailsync self-learning cache to maillink.Learner: a
// confidently-classified application email teaches its sender domain toward the
// sync allowlist (free-mail and already-known senders are skipped inside).
type domainLearner struct{ store *gmailsync.DBStore }

func newDomainLearner(pool *pgxpool.Pool) domainLearner {
	return domainLearner{store: gmailsync.NewDBStore(db.New(pool))}
}

func (l domainLearner) Learn(ctx context.Context, fromAddr string) error {
	return gmailsync.RecordJobMail(ctx, l.store, fromAddr)
}

func (l domainLearner) Miss(ctx context.Context, fromAddr string) error {
	return gmailsync.RecordNonJobMail(ctx, l.store, fromAddr)
}

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
		out[i] = claimedFrom(r)
	}
	return out, nil
}

// claimedFrom copies one claim row into the port's shape. A named function so store_test.go
// can hold it to copying every field.
func claimedFrom(r db.ClaimEmailClassificationBatchRow) maillink.Claimed {
	return maillink.Claimed{
		OutboxID: r.ID,
		EmailID:  r.EmailID,
		UserID:   r.UserID,
		Source:   r.Source,
		ThreadID: r.ThreadID,
		FromAddr: r.FromAddr,
		FromName: r.FromName,
		Subject:  r.Subject,
		Body:     r.BodyText,
		BodyHTML: r.BodyHtml,
	}
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
		LinkSource:      pgconv.Text(r.LinkSource),
		MatchConfidence: pgtype.Float4{Float32: float32(r.Confidence), Valid: matched},
		StatusSignal:    pgconv.Text(string(r.Signal)),
		Model:           pgconv.TextValid(model),
		ID:              r.EmailID,
		UserID:          userID,
	}); err != nil {
		return fmt.Errorf("set classification: %w", err)
	}
	if r.AdvanceStageTo != "" {
		// The move records a stage_set event dated by the message's received_at and attributed to
		// the carrying mailbox, the same provenance ReconcileMailEvent derives below.
		source, err := appevent.SourceForMail(r.MailSource)
		if err != nil {
			return fmt.Errorf("advance stage: %w", err)
		}
		if err := qtx.AdvanceUserJobStage(ctx, db.AdvanceUserJobStageParams{
			UserID:      userID,
			JobID:       r.JobID,
			Stage:       r.AdvanceStageTo,
			EmailID:     r.EmailID,
			EventSource: source,
		}); err != nil {
			return fmt.Errorf("advance stage: %w", err)
		}
	}
	// Reconcile the ledger in the same transaction that persisted the link, so an employer_reply
	// event cannot exist for a rolled-back classification or be missing for a committed one.
	if err := inbox.ReconcileMailEvent(ctx, qtx, userID, r.EmailID, r.MailSource); err != nil {
		return err
	}
	if err := qtx.DeleteEmailClassificationOutbox(ctx, outboxID); err != nil {
		return fmt.Errorf("delete outbox entry: %w", err)
	}
	return tx.Commit(ctx)
}

// Fail records the attempt and reports whether the entry dead-lettered (the statement stamps
// failed_at). Which of the policy's bounds decides is the statement's business.
func (s *dbStore) Fail(ctx context.Context, outboxID int64, cause string, policy maillink.FailurePolicy) (bool, error) {
	row, err := s.q.FailEmailClassification(ctx, db.FailEmailClassificationParams{
		LastError:         cause,
		MessageAtFault:    policy.MessageAtFault,
		MaxAttempts:       int32(policy.MaxAttempts),
		UpstreamGraceDays: int32(policy.UpstreamGraceDays),
		ID:                outboxID,
	})
	if err != nil {
		return false, err
	}
	return row.FailedAt.Valid, nil
}

func int8OrNull(v int64) pgtype.Int8 {
	return pgtype.Int8{Int64: v, Valid: v != 0}
}
