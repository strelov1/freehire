package prowelcome

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/platform/db"
)

// fakeStore serves a fixed candidate page and records what was claimed, mirroring
// onboarding's own runner_test.go fakeStore.
type fakeStore struct {
	rows      []db.ListNewlyPayingUsersMissingWelcomeEmailRow
	listErr   error
	claimErr  error
	claimed   []int64
	maxRowsIn int32
}

func (s *fakeStore) ListNewlyPayingUsersMissingWelcomeEmail(_ context.Context, maxRows int32) ([]db.ListNewlyPayingUsersMissingWelcomeEmailRow, error) {
	s.maxRowsIn = maxRows
	return s.rows, s.listErr
}

func (s *fakeStore) SetProWelcomeSent(_ context.Context, id int64) (int64, error) {
	if s.claimErr != nil {
		return 0, s.claimErr
	}
	s.claimed = append(s.claimed, id)
	return 1, nil
}

// fakeMailer records what it was asked to send — including the tier it resolved, for the
// resolution tests below and in store_integration_test.go — and can be told to fail one
// user.
type fakeMailer struct {
	sent     []int64
	lastTier plan.Tier
	failFor  int64
}

func (m *fakeMailer) Send(_ context.Context, userID int64, _ string, tier plan.Tier, _ time.Time) error {
	if userID == m.failFor {
		return errors.New("simulated send failure")
	}
	m.sent = append(m.sent, userID)
	m.lastTier = tier
	return nil
}

func proUntilRow(id int64, email string, proDays int) db.ListNewlyPayingUsersMissingWelcomeEmailRow {
	return db.ListNewlyPayingUsersMissingWelcomeEmailRow{
		ID: id, Email: email,
		ProUntil: pgtype.Timestamptz{Time: time.Now().Add(time.Duration(proDays) * 24 * time.Hour), Valid: true},
	}
}

func TestRunner_SendsAndClaimsEveryCandidate(t *testing.T) {
	store := &fakeStore{rows: []db.ListNewlyPayingUsersMissingWelcomeEmailRow{
		proUntilRow(1, "a@example.test", 30),
		proUntilRow(2, "b@example.test", 30),
	}}
	mailer := &fakeMailer{}
	r := New(store, mailer, 500)

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
	if store.maxRowsIn != 500 {
		t.Errorf("maxRows passed to the store = %d, want 500", store.maxRowsIn)
	}
}

func TestRunner_FailedSendLeavesTheClaimUnset(t *testing.T) {
	store := &fakeStore{rows: []db.ListNewlyPayingUsersMissingWelcomeEmailRow{
		proUntilRow(1, "a@example.test", 30),
		proUntilRow(2, "b@example.test", 30),
	}}
	mailer := &fakeMailer{failFor: 1}
	r := New(store, mailer, 500)

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
	r := New(store, &fakeMailer{}, 500)

	if _, err := r.Run(context.Background()); err == nil {
		t.Error("Run() = nil error, want the listing error surfaced")
	}
}

func TestRunner_ResolvesTierFromTheCandidateRow(t *testing.T) {
	// An Ultra row (ultra_until in the future, pro_until further still — an upgrade
	// leaves both live) must be welcomed as Ultra, per plan.TierOf's own rule that
	// Ultra wins even when Pro reaches further.
	row := db.ListNewlyPayingUsersMissingWelcomeEmailRow{
		ID: 9, Email: "u@example.test",
		ProUntil:   pgtype.Timestamptz{Time: time.Now().Add(60 * 24 * time.Hour), Valid: true},
		UltraUntil: pgtype.Timestamptz{Time: time.Now().Add(30 * 24 * time.Hour), Valid: true},
	}
	mailer := &fakeMailer{}
	store := &fakeStore{rows: []db.ListNewlyPayingUsersMissingWelcomeEmailRow{row}}
	r := New(store, mailer, 500)

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if mailer.lastTier != plan.TierUltra {
		t.Errorf("tier = %q, want %q", mailer.lastTier, plan.TierUltra)
	}
}
