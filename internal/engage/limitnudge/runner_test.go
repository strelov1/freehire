package limitnudge

import (
	"context"
	"errors"
	"testing"

	"github.com/strelov1/freehire/internal/platform/db"
)

// fakeStore serves a fixed candidate page and records what was claimed, mirroring
// prowelcome's own runner_test.go fakeStore.
type fakeStore struct {
	rows      []db.ListLimitHitUsersMissingNudgeEmailRow
	listErr   error
	claimErr  error
	claimed   []int64
	windowIn  int32
	maxRowsIn int32
}

func (s *fakeStore) ListLimitHitUsersMissingNudgeEmail(_ context.Context, arg db.ListLimitHitUsersMissingNudgeEmailParams) ([]db.ListLimitHitUsersMissingNudgeEmailRow, error) {
	s.windowIn, s.maxRowsIn = arg.WindowDays, arg.MaxRows
	return s.rows, s.listErr
}

func (s *fakeStore) SetLimitNudgeSent(_ context.Context, id int64) (int64, error) {
	if s.claimErr != nil {
		return 0, s.claimErr
	}
	s.claimed = append(s.claimed, id)
	return 1, nil
}

// fakeMailer records who it was asked to mail and can be told to fail one user.
type fakeMailer struct {
	sent    []int64
	failFor int64
}

func (m *fakeMailer) Send(_ context.Context, userID int64, _ string) error {
	if userID == m.failFor {
		return errors.New("simulated send failure")
	}
	m.sent = append(m.sent, userID)
	return nil
}

func row(id int64, email string) db.ListLimitHitUsersMissingNudgeEmailRow {
	return db.ListLimitHitUsersMissingNudgeEmailRow{ID: id, Email: email}
}

func TestRunner_SendsAndClaimsEveryCandidate(t *testing.T) {
	store := &fakeStore{rows: []db.ListLimitHitUsersMissingNudgeEmailRow{
		row(1, "a@example.test"),
		row(2, "b@example.test"),
	}}
	mailer := &fakeMailer{}
	r := New(store, mailer, 3, 500)

	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Sent != 2 || stats.Failed != 0 {
		t.Errorf("stats = %+v, want {Sent:2 Failed:0}", stats)
	}
	if len(store.claimed) != 2 {
		t.Errorf("claimed %v, want both ids stamped", store.claimed)
	}
	if store.windowIn != 3 {
		t.Errorf("windowDays passed to the store = %d, want 3", store.windowIn)
	}
	if store.maxRowsIn != 500 {
		t.Errorf("maxRows passed to the store = %d, want 500", store.maxRowsIn)
	}
}

func TestRunner_FailedSendLeavesTheClaimUnset(t *testing.T) {
	store := &fakeStore{rows: []db.ListLimitHitUsersMissingNudgeEmailRow{
		row(1, "a@example.test"),
		row(2, "b@example.test"),
	}}
	mailer := &fakeMailer{failFor: 1}
	r := New(store, mailer, 3, 500)

	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Sent != 1 || stats.Failed != 1 {
		t.Errorf("stats = %+v, want {Sent:1 Failed:1}", stats)
	}
	// The failed send's account is never claimed, so a later run retries it — the
	// successful one's account IS claimed, so a later run does not re-send it.
	if len(store.claimed) != 1 || store.claimed[0] != 2 {
		t.Errorf("claimed = %v, want only [2]", store.claimed)
	}
}

func TestRunner_ListErrorAbortsThePass(t *testing.T) {
	store := &fakeStore{listErr: errors.New("db down")}
	r := New(store, &fakeMailer{}, 3, 500)

	if _, err := r.Run(context.Background()); err == nil {
		t.Error("Run() = nil error, want the listing error surfaced")
	}
}
