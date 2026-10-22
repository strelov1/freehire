package directapply

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/ingest/applyform"
	"github.com/strelov1/freehire/internal/job/job"
	"github.com/strelov1/freehire/internal/platform/db"
)

var _ Repository = (*QueriesRepository)(nil)

// QueriesRepository is the Repository over the generated queries.
type QueriesRepository struct{ q *db.Queries }

// NewQueriesRepository builds the Repository.
func NewQueriesRepository(q *db.Queries) *QueriesRepository { return &QueriesRepository{q: q} }

// Vacancy loads the posting at slug if, and only if, it takes applications here. Every
// other posting answers ErrNotFound, exactly as a missing one does.
func (r *QueriesRepository) Vacancy(ctx context.Context, slug string) (Vacancy, error) {
	row, err := r.q.GetJobBySlug(ctx, slug)
	if errors.Is(err, pgx.ErrNoRows) {
		return Vacancy{}, ErrNotFound
	}
	if err != nil {
		return Vacancy{}, err
	}
	if !job.IsDirectApply(row.Source, row.ExternalID) || row.IsPrivate || !row.CreatedBy.Valid {
		return Vacancy{}, ErrNotFound
	}
	return Vacancy{
		ID:         row.ID,
		Slug:       row.PublicSlug,
		Title:      row.Title,
		Company:    row.Company,
		EmployerID: row.CreatedBy.Int64,
		Open:       !row.ClosedAt.Valid,
	}, nil
}

// Form reads the vacancy's questions. No stored row means the employer asks nothing.
func (r *QueriesRepository) Form(ctx context.Context, jobID int64) (applyform.Form, error) {
	row, err := r.q.GetApplyFormByJobID(ctx, jobID)
	if errors.Is(err, pgx.ErrNoRows) {
		return applyform.Form{Provider: applyform.ProviderFreehire}, nil
	}
	if err != nil {
		return applyform.Form{}, err
	}
	var form applyform.Form
	if err := json.Unmarshal(row.Payload, &form); err != nil {
		return applyform.Form{}, fmt.Errorf("decode questions of job %d: %w", jobID, err)
	}
	return form, nil
}

// Candidate reads the applying user's address and its verification.
func (r *QueriesRepository) Candidate(ctx context.Context, userID int64) (Candidate, error) {
	u, err := r.q.GetUserByID(ctx, userID)
	if err != nil {
		return Candidate{}, err
	}
	return Candidate{Email: u.Email, Verified: u.EmailVerified}, nil
}

// Record writes or replaces the application; see UpsertDirectApplication for the two cases
// in which it writes nothing.
func (r *QueriesRepository) Record(ctx context.Context, rec Record, dailyLimit int64) (int64, bool, error) {
	answers, err := json.Marshal(rec.Answers)
	if err != nil {
		return 0, false, fmt.Errorf("encode answers: %w", err)
	}
	if rec.Answers == nil {
		answers = []byte(`[]`)
	}
	id, err := r.q.UpsertDirectApplication(ctx, db.UpsertDirectApplicationParams{
		JobID:      rec.JobID,
		UserID:     rec.UserID,
		CvID:       pgtype.UUID{Bytes: rec.CV.CVID, Valid: !rec.CV.Resume},
		CvKind:     rec.CV.kind(),
		Note:       rec.Note,
		Answers:    answers,
		DailyLimit: dailyLimit,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return id, true, nil
}

// State reads where userID's application to jobID stands.
func (r *QueriesRepository) State(ctx context.Context, jobID, userID int64) (State, error) {
	row, err := r.q.GetDirectApplication(ctx, db.GetDirectApplicationParams{JobID: jobID, UserID: userID})
	if errors.Is(err, pgx.ErrNoRows) {
		return StateNone, nil
	}
	if err != nil {
		return StateNone, err
	}
	switch {
	case row.SentAt.Valid:
		return StateSent, nil
	case row.Sending:
		return StateSending, nil
	}
	return StateUnsent, nil
}

// MarkSent stamps an application delivered.
func (r *QueriesRepository) MarkSent(ctx context.Context, id int64) error {
	return r.q.MarkDirectApplicationSent(ctx, id)
}

// Release drops a delivery claim after a failed send.
func (r *QueriesRepository) Release(ctx context.Context, id int64) error {
	return r.q.ReleaseDirectApplication(ctx, id)
}
