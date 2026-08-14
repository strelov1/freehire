package calsync

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/calmatch"
	"github.com/strelov1/freehire/internal/tokencrypt"
)

// fakeStore records what the worker asked it to persist, so the tests assert on stored
// state rather than on log output.
type fakeStore struct {
	connections []Connection
	candidates  map[int64][]calmatch.Candidate
	tokenErr    error

	stored    []StoredInterview
	cancelled []string
	reconsent []int64
}

func (s *fakeStore) ListCalendarConnections(context.Context) ([]Connection, error) {
	return s.connections, nil
}

func (s *fakeStore) RefreshToken(_ context.Context, userID int64) (string, error) {
	if s.tokenErr != nil {
		return "", s.tokenErr
	}
	return encryptedFor(userID), nil
}

func (s *fakeStore) Candidates(_ context.Context, userID int64) ([]calmatch.Candidate, error) {
	return s.candidates[userID], nil
}

func (s *fakeStore) UpsertInterview(_ context.Context, in StoredInterview) error {
	s.stored = append(s.stored, in)
	return nil
}

func (s *fakeStore) CancelInterview(_ context.Context, _ int64, uid string) error {
	s.cancelled = append(s.cancelled, uid)
	return nil
}

func (s *fakeStore) SetNeedsReconsent(_ context.Context, userID int64) error {
	s.reconsent = append(s.reconsent, userID)
	return nil
}

// fakeReader returns a fixed window, or an error standing in for a revoked grant.
type fakeReader struct {
	meetings []Meeting
	err      error
}

func (r fakeReader) ListEvents(context.Context, time.Time, time.Time) ([]Meeting, error) {
	return r.meetings, r.err
}

var testCipher = mustCipher()

func mustCipher() *tokencrypt.Cipher {
	c, err := tokencrypt.New([]byte("0123456789abcdef0123456789abcdef"))
	if err != nil {
		panic(err)
	}
	return c
}

func encryptedFor(int64) string {
	enc, err := testCipher.Encrypt("refresh-token")
	if err != nil {
		panic(err)
	}
	return enc
}

func at(day, hour int) time.Time {
	return time.Date(2026, 8, day, hour, 0, 0, 0, time.UTC)
}

// THE privacy invariant, asserted against the store rather than against a log line. A
// calendar holds medical appointments, family, and interviews with employers the
// candidate never told us about; only a meeting attached to one of their own
// applications may be written, and nothing else may leave a trace anywhere.
func TestRunOnceStoresOnlyMeetingsItCouldAttach(t *testing.T) {
	store := &fakeStore{
		connections: []Connection{{UserID: 7}},
		candidates: map[int64][]calmatch.Candidate{
			7: {{ApplicationID: 11, UIDs: []string{"derq@ashbyhq.com"}}},
		},
	}
	reader := fakeReader{meetings: []Meeting{
		{UID: "dentist@personal", Title: "Dentist", StartsAt: at(12, 9)},
		{UID: "standup@currentjob", Title: "Team standup", StartsAt: at(12, 10)},
		{UID: "derq@ashbyhq.com", Title: "Technical screen", StartsAt: at(13, 9), EndsAt: at(13, 10)},
		{UID: "other-employer@greenhouse.io", Title: "Interview with Supabase", StartsAt: at(14, 9)},
	}}

	if err := newTestWorker(store, reader).RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	if len(store.stored) != 1 {
		t.Fatalf("stored %d meetings, want exactly the one that attached — the rest are the candidate's private life", len(store.stored))
	}
	got := store.stored[0]
	if got.UID != "derq@ashbyhq.com" || got.ApplicationID != 11 {
		t.Errorf("stored %+v, want the Derq interview against application 11", got)
	}
	if got.Status != StatusConfirmed {
		t.Errorf("status = %q, want %q — the invitation's own identifier attached it", got.Status, StatusConfirmed)
	}
}

// A meeting no invitation named is not stored at all. There is no weaker tier to fall
// back to: one existed, matched an employer's name in the title, and attached meetings to
// the wrong employer often enough that removing it was the fix.
func TestRunOnceStoresNothingForAMeetingNoInvitationNamed(t *testing.T) {
	store := &fakeStore{
		connections: []Connection{{UserID: 7}},
		candidates:  map[int64][]calmatch.Candidate{7: {{ApplicationID: 12}}},
	}
	reader := fakeReader{meetings: []Meeting{
		{UID: "v-1@google.com", Title: "Vercel <> Ivan", StartsAt: at(13, 9)},
	}}

	if err := newTestWorker(store, reader).RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if len(store.stored) != 0 {
		t.Errorf("stored %+v, want nothing — only the invitation's identifier attaches a meeting", store.stored)
	}
}

// A cancellation is a fact about a meeting we already hold. It marks rather than deletes,
// and it must not try to store the cancelled event as a fresh one.
func TestRunOnceMarksACancelledMeeting(t *testing.T) {
	store := &fakeStore{
		connections: []Connection{{UserID: 7}},
		candidates:  map[int64][]calmatch.Candidate{7: {{ApplicationID: 11, UIDs: []string{"derq@ashbyhq.com"}}}},
	}
	reader := fakeReader{meetings: []Meeting{
		{UID: "derq@ashbyhq.com", Title: "Technical screen", StartsAt: at(13, 9), Cancelled: true},
	}}

	if err := newTestWorker(store, reader).RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if len(store.cancelled) != 1 || store.cancelled[0] != "derq@ashbyhq.com" {
		t.Errorf("cancelled %v, want the one meeting", store.cancelled)
	}
	if len(store.stored) != 0 {
		t.Errorf("a cancelled meeting was also stored as current: %+v", store.stored)
	}
}

// Google only guarantees `id` for a cancelled event or occurrence — the iCalUID Resolve
// keys on may not survive deletion at all. A cancellation like that must still reach
// CancelInterview, keyed on the provider id, without ever going through the match gate.
func TestRunOnceMarksACancelledMeetingWithNoUID(t *testing.T) {
	store := &fakeStore{
		connections: []Connection{{UserID: 7}},
		candidates:  map[int64][]calmatch.Candidate{7: {{ApplicationID: 11, UIDs: []string{"derq@ashbyhq.com"}}}},
	}
	reader := fakeReader{meetings: []Meeting{
		{ProviderID: "evt-minimal", Cancelled: true},
	}}

	if err := newTestWorker(store, reader).RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if len(store.cancelled) != 1 || store.cancelled[0] != "evt-minimal" {
		t.Errorf("cancelled %v, want the provider id alone since Resolve can never key on an empty UID", store.cancelled)
	}
	if len(store.stored) != 0 {
		t.Errorf("a cancelled meeting was also stored as current: %+v", store.stored)
	}
}

// One candidate's revoked grant is not the fleet's problem: mark it, carry on, and let
// the exit code say the run was not wholly clean.
func TestRunOnceMarksAFailingGrantAndKeepsGoing(t *testing.T) {
	store := &fakeStore{
		connections: []Connection{{UserID: 7}, {UserID: 8}},
		candidates:  map[int64][]calmatch.Candidate{8: {{ApplicationID: 11, UIDs: []string{"derq@ashbyhq.com"}}}},
	}
	w := newTestWorker(store, fakeReader{meetings: []Meeting{
		{UID: "derq@ashbyhq.com", Title: "Screen", StartsAt: at(13, 9)},
	}})
	// User 7's reader refuses; user 8's works.
	w.newReader = func(_ context.Context, _ string) CalendarReader {
		if len(store.reconsent) == 0 {
			return fakeReader{err: &AuthError{err: errors.New("401 Unauthorized")}}
		}
		return fakeReader{meetings: []Meeting{{UID: "derq@ashbyhq.com", Title: "Screen", StartsAt: at(13, 9)}}}
	}

	err := w.RunOnce(context.Background())

	if err == nil {
		t.Error("RunOnce reported success although a grant failed; the exit code is how cron learns")
	}
	if len(store.reconsent) != 1 || store.reconsent[0] != 7 {
		t.Errorf("marked %v for re-consent, want just user 7", store.reconsent)
	}
	if len(store.stored) != 1 {
		t.Errorf("stored %d meetings, want the healthy user's one — a failing peer must not stop the run", len(store.stored))
	}
}

func newTestWorker(store *fakeStore, reader CalendarReader) *Worker {
	w := NewWorker(store, testCipher, func(context.Context, string) CalendarReader { return reader })
	w.now = func() time.Time { return at(12, 0) }
	return w
}

// Google having a bad day is not a candidate revoking a grant. The flag is shared with
// mail, so treating a 500 as a revocation would disconnect every mailbox we hold during
// one incident — each needing a restricted-scope consent to restore.
func TestRunOnceDoesNotRevokeAGrantOverAProviderFailure(t *testing.T) {
	store := &fakeStore{connections: []Connection{{UserID: 7}}}
	w := newTestWorker(store, fakeReader{err: errors.New("500 Internal Server Error")})

	err := w.RunOnce(context.Background())

	if err == nil {
		t.Error("RunOnce reported success although the provider failed")
	}
	if len(store.reconsent) != 0 {
		t.Errorf("marked %v for re-consent over a provider failure — that flag is shared with mail", store.reconsent)
	}
}
