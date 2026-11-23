package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/application/appevent"
	"github.com/strelov1/freehire/internal/application/autoapply"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
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

var (
	_ autoapply.Store        = (*dbStore)(nil)
	_ autoapply.PreviewStore = (*dbStore)(nil)
)

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
		// Never nil in practice (ClaimAutoApplyBatch requires tailored_cv_id IS NOT NULL), but a
		// missing id degrades to zero rather than a nil dereference.
		if r.TailoredCvID != nil {
			c.TailoredCVID = *r.TailoredCvID
		}
		out = append(out, c)
	}
	return out, nil
}

// Submit records the application and retires the queue entry in ONE transaction. Split, a
// recorded application with a live queue entry is reclaimed and submitted again (the "never
// twice" requirement), and a retired entry with no application loses the fact that a
// submission happened.
//
// LockJobForApply then MarkJobApplied is the same locked shape as
// jobtracking.QueriesRepository.MarkApplied, reused so the applied_count/ledger guarantee has
// one implementation. EventSource is appevent.SourceAutoApply, not SourceSystem, so
// internal/engage/nudge can query a successful auto-apply submission as a stated fact.
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
		EventSource: appevent.SourceAutoApply,
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

// DeadLetterUncertain dead-letters an attempt the employer may already hold and marks it so,
// which is what RetryAutoApplyEntry refuses on (migration 0196).
func (s *dbStore) DeadLetterUncertain(ctx context.Context, queueID int64, reason string) error {
	return s.q.MarkAutoApplySubmitUncertain(ctx, db.MarkAutoApplySubmitUncertainParams{ID: queueID, LastError: reason})
}

// FailCaptcha is Fail's counterpart for the one refusal safe to retry, on captcha_attempts
// rather than attempts. It shares failed_at: running out of asks is still the entry giving up.
// The separate counter stops the captcha's generous budget from spending down the one a
// transient error depends on.
func (s *dbStore) FailCaptcha(ctx context.Context, queueID int64, errMsg string, maxAttempts int) (bool, error) {
	row, err := s.q.RecordAutoApplyCaptchaRefusal(ctx, db.RecordAutoApplyCaptchaRefusalParams{
		ID:          queueID,
		LastError:   errMsg,
		MaxAttempts: int32(maxAttempts),
	})
	if err != nil {
		return false, err
	}
	return row.FailedAt.Valid, nil
}

// FailPreview is Fail's counterpart for the preview pass, on
// preview_attempts/preview_failed_at, so a transient preview error cannot dead-letter a row
// before a submission was attempted.
func (s *dbStore) FailPreview(ctx context.Context, queueID int64, errMsg string, maxAttempts int) (bool, error) {
	row, err := s.q.RecordAutoApplyPreviewFailure(ctx, db.RecordAutoApplyPreviewFailureParams{
		ID:          queueID,
		LastError:   errMsg,
		MaxAttempts: int32(maxAttempts),
	})
	if err != nil {
		return false, err
	}
	return row.PreviewFailedAt.Valid, nil
}

func (s *dbStore) ClaimForPreview(ctx context.Context, batch, leaseSeconds int) ([]autoapply.Claimed, error) {
	rows, err := s.q.ClaimAutoApplyPreviewBatch(ctx, db.ClaimAutoApplyPreviewBatchParams{
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
		// Never nil in practice, as in Claim.
		if r.TailoredCvID != nil {
			c.TailoredCVID = *r.TailoredCvID
		}
		out = append(out, c)
	}
	return out, nil
}

// SetPreview persists the resolved answer preview, approving the entry in the same write when
// it asks the candidate nothing (unless they confirm each one), and otherwise records the
// ready-for-review notification (sent here, not at tailor-completion, so the
// candidate never lands on an empty screen). pgx.ErrNoRows means a stale or retried pass lost
// the race to an already-decided entry; see SetAutoApplyResolvedPreview's guard.
func (s *dbStore) SetPreview(ctx context.Context, queueID int64, preview autoapply.ResolvedPreview) error {
	payload, err := json.Marshal(preview)
	if err != nil {
		return fmt.Errorf("encode resolved preview: %w", err)
	}
	row, err := s.q.SetAutoApplyResolvedPreview(ctx, db.SetAutoApplyResolvedPreviewParams{
		ID: queueID, ResolvedPreview: payload, NoQuestions: !preview.AsksCandidate(),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return fmt.Errorf("record resolved preview for queue entry %d: %w", queueID, err)
	}
	if row.AutoApproved {
		return nil // the submit pass sends it; the submitted nudge tells the candidate
	}
	if _, err := s.q.RecordNotification(ctx, db.RecordNotificationParams{
		UserID:     row.UserID,
		Kind:       "auto_apply_ready_for_review",
		Title:      "Your application is ready to review",
		Body:       fmt.Sprintf("We tailored your CV for %s at %s — take a look before it goes out.", row.Title, row.Company),
		PublicSlug: pgconv.TextValid(row.PublicSlug),
		// A re-tailor clears and recomputes the preview; the candidate hears about each entry once.
		DedupKey: pgconv.TextValid(fmt.Sprintf("auto_apply_ready:%d", queueID)),
	}); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		// Best-effort: a failure here must never fail the preview write.
		log.Printf("auto-apply: recording ready-for-review notification for queue entry %d: %v", queueID, err)
	}
	return nil
}
