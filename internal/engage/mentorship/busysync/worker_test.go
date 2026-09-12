package busysync

import (
	"context"
	"net/url"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"github.com/strelov1/freehire/internal/application/gmailsync"
	"github.com/strelov1/freehire/internal/platform/tokencrypt"
)

// fakeStore records what the worker asked it to persist, so the tests assert on stored
// state rather than on log output.
type fakeStore struct {
	connections []Connection
	tokenErr    error

	replaced  []replaceCall
	reconsent []int64
}

type replaceCall struct {
	mentorID  int64
	windowEnd time.Time
	periods   []BusyPeriod
}

func (s *fakeStore) ListConnections(context.Context) ([]Connection, error) {
	return s.connections, nil
}

func (s *fakeStore) RefreshToken(_ context.Context, userID int64) (string, error) {
	if s.tokenErr != nil {
		return "", s.tokenErr
	}
	return encryptedFor(userID), nil
}

func (s *fakeStore) ReplaceBusyWindow(_ context.Context, mentorID int64, windowEnd time.Time, periods []BusyPeriod) error {
	s.replaced = append(s.replaced, replaceCall{mentorID: mentorID, windowEnd: windowEnd, periods: periods})
	return nil
}

func (s *fakeStore) SetNeedsReconsent(_ context.Context, userID int64) error {
	s.reconsent = append(s.reconsent, userID)
	return nil
}

// fakeReader returns a fixed set of busy periods, or an error standing in for a revoked
// grant.
type fakeReader struct {
	periods []BusyPeriod
	err     error
}

func (r fakeReader) ListBusy(context.Context, time.Time, time.Time) ([]BusyPeriod, error) {
	return r.periods, r.err
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

func newTestWorker(store *fakeStore, reader FreeBusyReader) *Worker {
	w := NewWorker(store, testCipher, func(context.Context, string) FreeBusyReader { return reader })
	w.now = func() time.Time { return at(12, 0) }
	return w
}

// The ordinary path: a connected mentor's currently-reported busy periods replace
// whatever was stored, over the fixed forward window bounded at busyWindowDays.
func TestRunOnceReplacesTheBusyWindow(t *testing.T) {
	store := &fakeStore{connections: []Connection{{MentorID: 3, UserID: 7}}}
	periods := []BusyPeriod{
		{Start: at(13, 9), End: at(13, 10)},
		{Start: at(14, 9), End: at(14, 11)},
	}

	if err := newTestWorker(store, fakeReader{periods: periods}).RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}

	if len(store.replaced) != 1 {
		t.Fatalf("replaced %d windows, want 1", len(store.replaced))
	}
	got := store.replaced[0]
	if got.mentorID != 3 {
		t.Errorf("mentorID = %d, want 3", got.mentorID)
	}
	wantWindowEnd := at(12, 0).AddDate(0, 0, busyWindowDays)
	if !got.windowEnd.Equal(wantWindowEnd) {
		t.Errorf("windowEnd = %v, want %v", got.windowEnd, wantWindowEnd)
	}
	if len(got.periods) != 2 {
		t.Errorf("periods = %v, want the 2 the reader returned", got.periods)
	}
}

// One mentor's revoked grant is not the fleet's problem: mark it, carry on, and let the
// exit code say the run was not wholly clean. The spec's own scenario for this.
func TestRunOnceMarksAFailingGrantAndKeepsGoing(t *testing.T) {
	store := &fakeStore{connections: []Connection{{MentorID: 3, UserID: 7}, {MentorID: 4, UserID: 8}}}
	w := newTestWorker(store, fakeReader{periods: []BusyPeriod{{Start: at(13, 9), End: at(13, 10)}}})
	// Mentor 7's reader refuses; mentor 8's works.
	w.newReader = func(_ context.Context, _ string) FreeBusyReader {
		if len(store.reconsent) == 0 {
			return fakeReader{err: &gmailsync.APIError{
				Op: "calendar: freeBusy", StatusCode: 401, Status: "401 Unauthorized",
			}}
		}
		return fakeReader{periods: []BusyPeriod{{Start: at(13, 9), End: at(13, 10)}}}
	}

	err := w.RunOnce(context.Background())

	if err == nil {
		t.Error("RunOnce reported success although a grant failed; the exit code is how cron learns")
	}
	if len(store.reconsent) != 1 || store.reconsent[0] != 7 {
		t.Errorf("marked %v for re-consent, want just user 7", store.reconsent)
	}
	if len(store.replaced) != 1 || store.replaced[0].mentorID != 4 {
		t.Errorf("replaced %v, want just the healthy mentor's window — a failing peer must not stop the run", store.replaced)
	}
}

// A revocation-shaped failure must not also try to reconcile the window: there is
// nothing current to replace it with, and doing so would clear a mentor's busy set on
// the same run that just lost the ability to read it.
func TestRunOnceDoesNotReplaceTheWindowOnARevokedGrant(t *testing.T) {
	store := &fakeStore{connections: []Connection{{MentorID: 3, UserID: 7}}}
	w := newTestWorker(store, fakeReader{err: &gmailsync.APIError{
		Op: "calendar: freeBusy", StatusCode: 401, Status: "401 Unauthorized",
	}})

	if err := w.RunOnce(context.Background()); err == nil {
		t.Error("RunOnce reported success although the grant was revoked")
	}
	if len(store.replaced) != 0 {
		t.Errorf("replaced %v, want no reconcile on a revoked grant", store.replaced)
	}
}

// Google having a bad day is not a mentor revoking a grant. The flag is shared with
// every other Google feature this account may use, so treating a 500 as a revocation
// would disconnect it during one provider incident.
func TestRunOnceDoesNotRevokeAGrantOverAProviderFailure(t *testing.T) {
	store := &fakeStore{connections: []Connection{{MentorID: 3, UserID: 7}}}
	w := newTestWorker(store, fakeReader{err: &gmailsync.APIError{
		Op: "calendar: freeBusy", StatusCode: 500, Status: "500 Internal Server Error",
	}})

	err := w.RunOnce(context.Background())

	if err == nil {
		t.Error("RunOnce reported success although the provider failed")
	}
	if len(store.reconsent) != 0 {
		t.Errorf("marked %v for re-consent over a provider failure", store.reconsent)
	}
}

// The revocation this sync actually sees. The client is built from a stored refresh
// token, so a mentor who withdrew consent at Google is refused at the TOKEN endpoint
// and the freeBusy API is never reached — no 401, no 403, nothing for a status-only
// rule to catch.
func TestRunOnceMarksReconsentWhenTheTokenEndpointRefusesTheRefreshToken(t *testing.T) {
	store := &fakeStore{connections: []Connection{{MentorID: 3, UserID: 7}}}
	w := newTestWorker(store, fakeReader{err: &url.Error{
		Op:  "Post",
		URL: "https://www.googleapis.com/calendar/v3/freeBusy",
		Err: &oauth2.RetrieveError{ErrorCode: "invalid_grant"},
	}})

	if err := w.RunOnce(context.Background()); err == nil {
		t.Error("RunOnce reported success although a grant was refused")
	}
	if len(store.reconsent) != 1 || store.reconsent[0] != 7 {
		t.Errorf("marked %v for re-consent, want user 7 — invalid_grant IS the revocation", store.reconsent)
	}
}
