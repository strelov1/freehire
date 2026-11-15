package directapply

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/ingest/applyform"
	"github.com/strelov1/freehire/internal/job/job"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
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
		JobID:         rec.JobID,
		UserID:        rec.UserID,
		CvID:          pgtype.UUID{Bytes: rec.CV.CVID, Valid: !rec.CV.Resume},
		CvKind:        rec.CV.kind(),
		Note:          rec.Note,
		Answers:       answers,
		DailyLimit:    dailyLimit,
		CvFilename:    rec.CVFile.Filename,
		CvContentType: rec.CVFile.ContentType,
		CvContent:     rec.CVFile.Content,
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

var _ InboxRepository = (*QueriesRepository)(nil)

// inboxApplication maps one ListEmployerApplications row.
func inboxApplication(row db.ListEmployerApplicationsRow) (InboxApplication, error) {
	var answers []Answer
	if err := json.Unmarshal(row.Answers, &answers); err != nil {
		return InboxApplication{}, fmt.Errorf("decode answers of application %d: %w", row.ID, err)
	}
	return InboxApplication{
		ID:             row.ID,
		JobSlug:        row.JobSlug,
		JobTitle:       row.JobTitle,
		CandidateEmail: row.CandidateEmail,
		CVKind:         row.CvKind,
		HasCV:          row.HasCv,
		Note:           row.Note,
		Answers:        answers,
		Stage:          Stage(row.Stage),
		StageChangedAt: pgconv.TimePtr(row.StageChangedAt),
		SentAt:         row.SentAt.Time,
	}, nil
}

// List reads one page of the employer's delivered applications.
func (r *QueriesRepository) List(ctx context.Context, employerID int64, f InboxFilter) ([]InboxApplication, error) {
	return r.list(ctx, db.ListEmployerApplicationsParams{
		EmployerID: employerID,
		JobSlug:    pgconv.Text(f.JobSlug),
		Stage:      pgconv.Text(string(f.Stage)),
		RowLimit:   f.Limit,
		RowOffset:  f.Offset,
	})
}

func (r *QueriesRepository) list(ctx context.Context, p db.ListEmployerApplicationsParams) ([]InboxApplication, error) {
	rows, err := r.q.ListEmployerApplications(ctx, p)
	if err != nil {
		return nil, err
	}
	out := make([]InboxApplication, len(rows))
	for i, row := range rows {
		if out[i], err = inboxApplication(row); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// Counts reads how many delivered applications sit at each stage.
func (r *QueriesRepository) Counts(ctx context.Context, employerID int64, jobSlug string) (map[Stage]int64, error) {
	rows, err := r.q.CountEmployerApplicationsByStage(ctx, db.CountEmployerApplicationsByStageParams{
		EmployerID: employerID,
		JobSlug:    pgconv.Text(jobSlug),
	})
	if err != nil {
		return nil, err
	}
	out := make(map[Stage]int64, len(rows))
	for _, row := range rows {
		out[Stage(row.Stage)] = row.Applications
	}
	return out, nil
}

// Get reads one delivered application of the employer's.
func (r *QueriesRepository) Get(ctx context.Context, employerID, id int64) (InboxApplication, error) {
	apps, err := r.list(ctx, db.ListEmployerApplicationsParams{
		EmployerID: employerID,
		ID:         pgtype.Int8{Int64: id, Valid: true},
		RowLimit:   1,
	})
	if err != nil {
		return InboxApplication{}, err
	}
	if len(apps) == 0 {
		return InboxApplication{}, ErrNotFound
	}
	return apps[0], nil
}

// CV reads the CV file a delivered application of the employer's sent.
func (r *QueriesRepository) CV(ctx context.Context, employerID, id int64) (Attachment, error) {
	row, err := r.q.GetEmployerApplicationCV(ctx, db.GetEmployerApplicationCVParams{ID: id, EmployerID: employerID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Attachment{}, ErrNotFound
	}
	if err != nil {
		return Attachment{}, err
	}
	return Attachment{Filename: row.Filename, ContentType: row.ContentType, Content: row.Content}, nil
}

// SetStage moves a delivered application of the employer's to st.
func (r *QueriesRepository) SetStage(ctx context.Context, employerID, id int64, st Stage) (StageMove, error) {
	row, err := r.q.SetDirectApplicationStage(ctx, db.SetDirectApplicationStageParams{ID: id, EmployerID: employerID, Stage: string(st)})
	if errors.Is(err, pgx.ErrNoRows) {
		return StageMove{}, ErrNotFound
	}
	if err != nil {
		return StageMove{}, err
	}
	return StageMove{
		Changed:        row.Changed,
		CandidateID:    row.CandidateID,
		CandidateEmail: row.CandidateEmail,
		JobID:          row.JobID,
		JobSlug:        row.JobSlug,
		JobTitle:       row.JobTitle,
		Company:        row.Company,
	}, nil
}

// ApplicationLocks serialises work per application with a transaction-scoped advisory
// lock (LockDirectApplication). Rolling the transaction back releases it however fn ends.
type ApplicationLocks struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

// NewApplicationLocks builds ApplicationLocks over the pool.
func NewApplicationLocks(q *db.Queries, pool *pgxpool.Pool) *ApplicationLocks {
	return &ApplicationLocks{pool: pool, q: q}
}

// Serialize runs fn holding the application's lock. fn does its own work on its own
// connections; the lock lives on the transaction this method holds open meanwhile.
func (l *ApplicationLocks) Serialize(ctx context.Context, id int64, fn func(context.Context) error) error {
	tx, err := l.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := l.q.WithTx(tx).LockDirectApplication(ctx, id); err != nil {
		return err
	}
	return fn(ctx)
}
