package inbox

import (
	"context"
	"errors"

	"github.com/strelov1/freehire/internal/db"
)

// errNoRows stands in for pgx.ErrNoRows: the store's way of saying a row the
// caller named is not theirs, or does not exist.
var errNoRows = errors.New("no rows in result set")

// fakeQueries is an in-memory stand-in for the store. It records what it was asked
// so a test can assert on the call that was NOT made — which is most of what
// matters here (a link left alone, a classification not written).
type fakeQueries struct {
	// reads
	list       []db.ListEmailsRow
	total      int64
	state      []db.CountEmailsByStateRow
	email      db.GetEmailRow
	job        db.Job
	invitation db.GetInterviewInvitationRow

	// failures to inject
	invitationErr error
	jobErr        error
	stageErr      error
	advanceErr    error
	// noRows makes every mutation match nothing, as it would for another user's mail.
	noRows bool

	// observed
	lastList   db.ListEmailsParams
	lastTriage db.AgentTriageEmailParams
	triaged    int
	stage      string
	advancedTo string
	// synced records the ledger reconciles asked for, so a test can assert that every
	// link mutation ends with one.
	synced   []db.RecordEmailApplicationEventParams
	retracts int
	// recordedBeforeRetract catches the ordering bug that CTE-based reconciliation had:
	// recording before retracting leaves the superseded row live, so the insert conflicts
	// with it and the correction silently does nothing.
	recordedBeforeRetract bool
}

// RetractSupersededEmailEvent is step 1 of the reconcile; the fake counts it so a test
// can assert it ran BEFORE the record — the order is the whole correctness of a re-link.
func (f *fakeQueries) RetractSupersededEmailEvent(_ context.Context, _ db.RetractSupersededEmailEventParams) (int64, error) {
	f.retracts++
	return 0, nil
}

// RecordEmailApplicationEvent is step 2.
func (f *fakeQueries) RecordEmailApplicationEvent(_ context.Context, arg db.RecordEmailApplicationEventParams) error {
	if f.retracts == 0 {
		f.recordedBeforeRetract = true
	}
	f.synced = append(f.synced, arg)
	return nil
}

func (f *fakeQueries) rows() int64 {
	if f.noRows {
		return 0
	}
	return 1
}

func (f *fakeQueries) ListEmails(_ context.Context, arg db.ListEmailsParams) ([]db.ListEmailsRow, error) {
	f.lastList = arg
	return f.list, nil
}

func (f *fakeQueries) CountEmails(context.Context, db.CountEmailsParams) (db.CountEmailsRow, error) {
	return db.CountEmailsRow{Total: f.total}, nil
}

func (f *fakeQueries) CountEmailsByState(context.Context, int64) ([]db.CountEmailsByStateRow, error) {
	return f.state, nil
}

func (f *fakeQueries) GetEmail(context.Context, db.GetEmailParams) (db.GetEmailRow, error) {
	return f.email, nil
}

func (f *fakeQueries) GetInterviewInvitation(context.Context, db.GetInterviewInvitationParams) (db.GetInterviewInvitationRow, error) {
	return f.invitation, f.invitationErr
}

func (f *fakeQueries) GetJobBySlug(context.Context, string) (db.Job, error) {
	return f.job, f.jobErr
}

func (f *fakeQueries) AgentTriageEmail(_ context.Context, arg db.AgentTriageEmailParams) (int64, error) {
	f.lastTriage = arg
	f.triaged++
	return f.rows(), nil
}

func (f *fakeQueries) LinkEmailToJob(context.Context, db.LinkEmailToJobParams) (int64, error) {
	return f.rows(), nil
}

func (f *fakeQueries) UnlinkEmail(context.Context, db.UnlinkEmailParams) (int64, error) {
	return f.rows(), nil
}

func (f *fakeQueries) ConfirmEmailLink(context.Context, db.ConfirmEmailLinkParams) (int64, error) {
	return f.rows(), nil
}

func (f *fakeQueries) RejectEmailLink(context.Context, db.RejectEmailLinkParams) (int64, error) {
	return f.rows(), nil
}

func (f *fakeQueries) GetUserJobStage(context.Context, db.GetUserJobStageParams) (string, error) {
	return f.stage, f.stageErr
}

func (f *fakeQueries) AdvanceUserJobStage(_ context.Context, arg db.AdvanceUserJobStageParams) error {
	if f.advanceErr != nil {
		return f.advanceErr
	}
	f.advancedTo = arg.Stage
	return nil
}
