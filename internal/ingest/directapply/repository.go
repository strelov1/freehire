package directapply

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/application/jobtracking"
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
		ViewedAt:       pgconv.TimePtr(row.ViewedAt),
		EmployerNotes:  row.EmployerNotes,
		HasUnreadReply: row.HasUnreadReply,
	}, nil
}

// List reads one page of the employer's delivered applications.
func (r *QueriesRepository) List(ctx context.Context, employerID int64, f InboxFilter) ([]InboxApplication, error) {
	return r.list(ctx, db.ListEmployerApplicationsParams{
		EmployerID: employerID,
		JobSlug:    pgconv.Text(f.JobSlug),
		Stage:      pgconv.Text(string(f.Stage)),
		Search:     pgconv.Text(f.Search),
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
func (r *QueriesRepository) Counts(ctx context.Context, employerID int64, f InboxFilter) (map[Stage]int64, int64, error) {
	rows, err := r.q.CountEmployerApplicationsByStage(ctx, db.CountEmployerApplicationsByStageParams{
		EmployerID: employerID,
		JobSlug:    pgconv.Text(f.JobSlug),
		Search:     pgconv.Text(f.Search),
	})
	if err != nil {
		return nil, 0, err
	}
	out := make(map[Stage]int64, len(rows))
	var total int64
	for _, row := range rows {
		stage := Stage(row.Stage)
		out[stage] = row.Applications
		if f.Stage == "" || f.Stage == stage {
			total += row.Matches
		}
	}
	return out, total, nil
}

func (r *QueriesRepository) UpdateWorkspace(ctx context.Context, employerID, id int64, markViewed bool, notes *string) error {
	text := pgconv.TextFromPtr(notes)
	_, err := r.q.UpdateEmployerApplicationWorkspace(ctx, db.UpdateEmployerApplicationWorkspaceParams{
		ID: id, EmployerID: employerID, MarkViewed: markViewed, EmployerNotes: text,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
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

// ApplicationMoves runs each move of an application on one pooled connection that first
// takes the application's session advisory lock (LockDirectApplication), so moves of one
// application are serialised. On that connection the move's database work — the stage, the
// candidate's tracking — runs in one transaction, so a move never holds one connection while
// waiting for another (which, with enough concurrent moves, would exhaust the pool); the
// mirror runs in a savepoint so its failure leaves the stage standing. The transaction
// commits BEFORE the move's after-commit step (the candidate's letter) runs, still inside the
// lock: a letter is only ever about a committed decision, and letters keep the order of the
// moves. The lock is released at the end; if releasing fails the connection is closed, which
// releases it too.
type ApplicationMoves struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

// NewApplicationMoves builds ApplicationMoves over the pool.
func NewApplicationMoves(q *db.Queries, pool *pgxpool.Pool) *ApplicationMoves {
	return &ApplicationMoves{pool: pool, q: q}
}

// RunMove runs fn as one move of application id; see ApplicationMoves. fn returns the step
// to run after the commit, or nil.
func (m *ApplicationMoves) RunMove(ctx context.Context, id int64, fn func(context.Context, MoveScope) (func(context.Context), error)) error {
	conn, err := m.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	qc := db.New(conn)
	if err := qc.LockDirectApplication(ctx, id); err != nil {
		return err
	}
	defer func() {
		// Released even when ctx is done: a lock left on a pooled connection would block
		// every later move of this application.
		if ok, err := qc.UnlockDirectApplication(context.WithoutCancel(ctx), id); err != nil || !ok {
			_ = conn.Hijack().Close(context.WithoutCancel(ctx))
		}
	}()

	after, err := m.inTx(ctx, conn, fn)
	if err != nil {
		return err
	}
	if after != nil {
		after(ctx)
	}
	return nil
}

func (m *ApplicationMoves) inTx(ctx context.Context, conn *pgxpool.Conn, fn func(context.Context, MoveScope) (func(context.Context), error)) (func(context.Context), error) {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	mirror := savepointMirror{tx: tx, build: func(d db.DBTX) StageMirror { return jobtracking.NewEmployerStages(db.New(d)) }}
	after, err := fn(ctx, MoveScope{Repo: NewQueriesRepository(m.q.WithTx(tx)), Mirror: mirror})
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return after, nil
}

// savepointMirror runs each mirror inside a savepoint of the move's transaction. The mirror
// is best effort, but a statement that fails in Postgres aborts the whole transaction —
// without the savepoint, a failed mirror would roll back the employer's stage too.
type savepointMirror struct {
	tx    pgx.Tx
	build func(db.DBTX) StageMirror
}

func (m savepointMirror) Mirror(ctx context.Context, userID, jobID int64, stage string) (bool, error) {
	sp, err := m.tx.Begin(ctx) // a savepoint: pgx nests a transaction begun on a transaction
	if err != nil {
		return false, err
	}
	moved, err := m.build(sp).Mirror(ctx, userID, jobID, stage)
	if err != nil {
		_ = sp.Rollback(ctx)
		return false, err
	}
	return moved, sp.Commit(ctx)
}
