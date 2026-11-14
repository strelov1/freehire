package jobtracking

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/application/appevent"
	"github.com/strelov1/freehire/internal/application/userjob"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
)

// EmployerStages mirrors an employer's pipeline moves onto the candidate's own record of
// the application (internal/ingest/directapply). It is its own type, not a Service method,
// because it answers to the employer's action rather than the candidate's and needs none of
// the Service's collaborators.
type EmployerStages struct {
	repo employerStageRepo
}

// employerStageRepo is the persistence EmployerStages needs.
type employerStageRepo interface {
	// CandidateStage is the candidate's stage for jobID and whether the employer set it;
	// found is false when the candidate has no record of the application.
	CandidateStage(ctx context.Context, userID, jobID int64) (stage string, byEmployer, found bool, err error)
	// SetIfUnchanged sets the stage only while it still equals expected, recording the
	// ledger event; it reports whether it moved.
	SetIfUnchanged(ctx context.Context, userID, jobID int64, expected, stage string) (bool, error)
}

// NewEmployerStages builds EmployerStages over the generated queries.
func NewEmployerStages(q *db.Queries) *EmployerStages {
	return &EmployerStages{repo: employerStageQueries{q: q}}
}

// Mirror sets the candidate's stage for jobID to `stage` (a userjob stage) when
// userjob.EmployerMay allows it, and reports whether it did. A candidate with no record of
// the application — they took it off their board — is left without one. The write is a
// compare-and-set against the stage just read, so a change the candidate makes in between
// wins rather than being overwritten.
func (m *EmployerStages) Mirror(ctx context.Context, userID, jobID int64, stage string) (bool, error) {
	current, byEmployer, found, err := m.repo.CandidateStage(ctx, userID, jobID)
	if err != nil || !found {
		return false, err
	}
	if !userjob.EmployerMay(current, byEmployer, stage) {
		return false, nil
	}
	return m.repo.SetIfUnchanged(ctx, userID, jobID, current, stage)
}

type employerStageQueries struct{ q *db.Queries }

func (r employerStageQueries) CandidateStage(ctx context.Context, userID, jobID int64) (string, bool, bool, error) {
	row, err := r.q.GetApplicationStageForMirror(ctx, db.GetApplicationStageForMirrorParams{UserID: userID, JobID: jobID})
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, false, nil
	}
	if err != nil {
		return "", false, false, err
	}
	return row.Stage.String, row.StageSource == appevent.SourceEmployer, true, nil
}

func (r employerStageQueries) SetIfUnchanged(ctx context.Context, userID, jobID int64, expected, stage string) (bool, error) {
	return r.q.MirrorEmployerStage(ctx, db.MirrorEmployerStageParams{
		UserID:   userID,
		JobID:    jobID,
		Stage:    stage,
		Expected: pgconv.Text(expected),
	})
}
