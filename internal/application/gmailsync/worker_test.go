package gmailsync

import (
	"context"
	"errors"
	"net/url"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"github.com/strelov1/freehire/internal/platform/tokencrypt"
)

func testCipher(t *testing.T) *tokencrypt.Cipher {
	t.Helper()
	key := make([]byte, 32)
	c, err := tokencrypt.New(key)
	if err != nil {
		t.Fatalf("cipher: %v", err)
	}
	return c
}

type fakeStore struct {
	conns          []Connection
	encToken       string
	upserted       []StoredEmail
	upsertErrIDs   map[string]bool // message ids on which UpsertEmail fails
	syncedCursor   int64
	syncedCalled   bool
	reconsentUsers []int64
	reconsentErr   error
}

func (f *fakeStore) ListConnected(context.Context) ([]Connection, error) { return f.conns, nil }
func (f *fakeStore) RefreshToken(context.Context, int64) (string, error) { return f.encToken, nil }
func (f *fakeStore) UpsertEmail(_ context.Context, e StoredEmail) error {
	if f.upsertErrIDs[e.Message.ID] {
		return errors.New("store: transient failure")
	}
	f.upserted = append(f.upserted, e)
	return nil
}
func (f *fakeStore) SetSynced(_ context.Context, _, cursor int64) error {
	f.syncedCalled = true
	f.syncedCursor = cursor
	return nil
}
func (f *fakeStore) SetNeedsReconsent(_ context.Context, userID int64) error {
	if f.reconsentErr != nil {
		return f.reconsentErr
	}
	f.reconsentUsers = append(f.reconsentUsers, userID)
	return nil
}

type fakeReader struct {
	ids     []string
	byID    map[string]Message
	threads map[string][]string // threadID -> message ids
	listErr error
}

func (f *fakeReader) ListATSMessageIDs(context.Context, string, int64) ([]string, error) {
	return f.ids, f.listErr
}
func (f *fakeReader) ListThreadMessageIDs(_ context.Context, threadID string) ([]string, error) {
	return f.threads[threadID], nil
}
func (f *fakeReader) GetMessage(_ context.Context, id string) (Message, error) {
	return f.byID[id], nil
}

func TestRunOnceSyncsUser(t *testing.T) {
	c := testCipher(t)
	enc, _ := c.Encrypt("refresh-token")
	store := &fakeStore{
		conns:    []Connection{{UserID: 7, Email: "u@gmail.com", Cursor: 0}},
		encToken: enc,
	}
	t1 := time.Unix(1_700_000_100, 0)
	t2 := time.Unix(1_700_000_500, 0) // newest
	reader := &fakeReader{
		ids: []string{"m1", "m2"},
		byID: map[string]Message{
			"m1": {ID: "m1", Subject: "Thank you for applying to Acme", ReceivedAt: t1},
			"m2": {ID: "m2", Subject: "Re: Thank you for applying to Acme", ReceivedAt: t2},
		},
	}
	w := NewWorker(store, c, func(context.Context, string, []string) GmailReader { return reader })

	stats, err := w.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if len(store.upserted) != 2 {
		t.Fatalf("upserted = %d, want 2", len(store.upserted))
	}
	if !store.syncedCalled || store.syncedCursor != t2.Unix() {
		t.Errorf("cursor = %d (called=%v), want %d", store.syncedCursor, store.syncedCalled, t2.Unix())
	}
	if (stats != Stats{Connections: 1, Synced: 1}) {
		t.Errorf("stats = %+v, want one connection cleanly synced", stats)
	}
}

// TestRunOnceExpandsThread locks in thread-expansion: a matched ATS message
// pulls in every sibling of its thread, so a recruiter's reply from a personal
// domain (which no allowlist/phrase would match) is ingested too. The user's own
// in-thread reply is skipped — inbound mail only.
func TestRunOnceExpandsThread(t *testing.T) {
	c := testCipher(t)
	enc, _ := c.Encrypt("refresh-token")
	store := &fakeStore{
		conns:    []Connection{{UserID: 7, Email: "me@gmail.com", Cursor: 0}},
		encToken: enc,
	}
	reader := &fakeReader{
		ids: []string{"m1"}, // only the ATS anchor matches the search
		byID: map[string]Message{
			"m1": {ID: "m1", ThreadID: "t1", FromAddr: "no-reply@ashbyhq.com", Subject: "Thanks for applying", ReceivedAt: time.Unix(1_700_000_100, 0)},
			"m3": {ID: "m3", ThreadID: "t1", FromAddr: "recruiter@acme-personal.com", Subject: "Re: a few questions", ReceivedAt: time.Unix(1_700_000_300, 0)},
			"m9": {ID: "m9", ThreadID: "t1", FromAddr: "me@gmail.com", Subject: "Re: a few questions", ReceivedAt: time.Unix(1_700_000_400, 0)},
		},
		threads: map[string][]string{"t1": {"m1", "m3", "m9"}},
	}
	w := NewWorker(store, c, func(context.Context, string, []string) GmailReader { return reader })

	if _, err := w.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	got := map[string]bool{}
	for _, e := range store.upserted {
		got[e.Message.ID] = true
	}
	if !got["m1"] || !got["m3"] {
		t.Errorf("want anchor m1 and sibling m3 upserted, got %v", got)
	}
	if got["m9"] {
		t.Error("user's own in-thread reply m9 should not be ingested")
	}
	if len(store.upserted) != 2 {
		t.Errorf("upserted = %d, want 2 (m1, m3; no dupes)", len(store.upserted))
	}
}

// TestRunOnceFreezesWatermarkOnFailure locks in that a transient failure on one
// message in a wave stops the watermark from advancing past it, even when a
// later-processed message in the same wave is newer and stores fine — otherwise
// the failed message's timestamp falls at-or-before the next run's cursor filter
// and is never retried. In this specific ordering (the failure is processed
// before any success) newest never left u.Cursor in the first place, so it does
// not by itself distinguish "freeze in place" from "rewind to u.Cursor" — see
// TestRunOnceRewindsWatermarkWhenASuccessPrecedesAFailure for that.
func TestRunOnceFreezesWatermarkOnFailure(t *testing.T) {
	c := testCipher(t)
	enc, _ := c.Encrypt("refresh-token")
	store := &fakeStore{
		conns:        []Connection{{UserID: 7, Email: "u@gmail.com", Cursor: 0}},
		encToken:     enc,
		upsertErrIDs: map[string]bool{"m1": true},
	}
	t1 := time.Unix(1_700_000_100, 0) // older, fails to store
	t2 := time.Unix(1_700_000_500, 0) // newer, stores fine
	reader := &fakeReader{
		ids: []string{"m1", "m2"},
		byID: map[string]Message{
			"m1": {ID: "m1", Subject: "Thank you for applying to Acme", ReceivedAt: t1},
			"m2": {ID: "m2", Subject: "Thank you for applying to Widget Co", ReceivedAt: t2},
		},
	}
	w := NewWorker(store, c, func(context.Context, string, []string) GmailReader { return reader })

	stats, err := w.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if len(store.upserted) != 1 || store.upserted[0].Message.ID != "m2" {
		t.Fatalf("upserted = %v, want only m2 (m1 fails)", store.upserted)
	}
	if !store.syncedCalled {
		t.Fatal("SetSynced not called")
	}
	if store.syncedCursor >= t1.Unix() {
		t.Errorf("cursor = %d, want < %d (m1's timestamp) so the failed message is retried next run", store.syncedCursor, t1.Unix())
	}
	if stats.Failed != 1 || stats.Synced != 0 {
		t.Errorf("stats = %+v, want the connection counted as failed — a partial sync is what "+
			"the exit code exists to report", stats)
	}
}

// TestRunOnceRewindsWatermarkWhenASuccessPrecedesAFailure covers the ordering
// ListATSMessageIDs actually returns in production (newest first): a newer
// message stores fine and advances newest, then an older sibling in the same
// wave fails. A watermark that was merely frozen at whatever it last reached
// would still persist the newer message's timestamp — past the failed one —
// so the fix must rewind newest to u.Cursor, not just stop advancing it.
func TestRunOnceRewindsWatermarkWhenASuccessPrecedesAFailure(t *testing.T) {
	c := testCipher(t)
	enc, _ := c.Encrypt("refresh-token")
	store := &fakeStore{
		conns:        []Connection{{UserID: 7, Email: "u@gmail.com", Cursor: 0}},
		encToken:     enc,
		upsertErrIDs: map[string]bool{"m1": true},
	}
	t1 := time.Unix(1_700_000_100, 0) // older, fails to store, processed SECOND
	t2 := time.Unix(1_700_000_500, 0) // newer, stores fine, processed FIRST
	reader := &fakeReader{
		ids: []string{"m2", "m1"}, // newest-first, as the real Gmail list API returns
		byID: map[string]Message{
			"m1": {ID: "m1", Subject: "Thank you for applying to Acme", ReceivedAt: t1},
			"m2": {ID: "m2", Subject: "Thank you for applying to Widget Co", ReceivedAt: t2},
		},
	}
	w := NewWorker(store, c, func(context.Context, string, []string) GmailReader { return reader })

	if _, err := w.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if len(store.upserted) != 1 || store.upserted[0].Message.ID != "m2" {
		t.Fatalf("upserted = %v, want only m2 (m1 fails)", store.upserted)
	}
	if store.syncedCursor != 0 {
		t.Errorf("cursor = %d, want 0 (u.Cursor): a success before the failure must not leave the watermark past it", store.syncedCursor)
	}
}

// TestRunOnceRewindsWatermarkOnAFailedThreadSibling covers a failure reached
// only through thread expansion, after the main wave already advanced newest.
func TestRunOnceRewindsWatermarkOnAFailedThreadSibling(t *testing.T) {
	c := testCipher(t)
	enc, _ := c.Encrypt("refresh-token")
	store := &fakeStore{
		conns:        []Connection{{UserID: 7, Email: "me@gmail.com", Cursor: 0}},
		encToken:     enc,
		upsertErrIDs: map[string]bool{"m3": true},
	}
	reader := &fakeReader{
		ids: []string{"m1"},
		byID: map[string]Message{
			"m1": {ID: "m1", ThreadID: "t1", FromAddr: "no-reply@ashbyhq.com", Subject: "Thanks for applying", ReceivedAt: time.Unix(1_700_000_100, 0)},
			"m3": {ID: "m3", ThreadID: "t1", FromAddr: "recruiter@acme-personal.com", Subject: "Re: a few questions", ReceivedAt: time.Unix(1_700_000_300, 0)},
		},
		threads: map[string][]string{"t1": {"m1", "m3"}},
	}
	w := NewWorker(store, c, func(context.Context, string, []string) GmailReader { return reader })

	if _, err := w.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if len(store.upserted) != 1 || store.upserted[0].Message.ID != "m1" {
		t.Fatalf("upserted = %v, want only m1 (thread sibling m3 fails)", store.upserted)
	}
	if store.syncedCursor != 0 {
		t.Errorf("cursor = %d, want 0 (u.Cursor): a thread-sibling failure must still rewind the watermark", store.syncedCursor)
	}
}

// A 401 from the Gmail API is the grant saying no, and the only kind of failure that may
// cost the candidate their connection.
func TestRunOnceRevokedTokenMarksReconsent(t *testing.T) {
	store, stats := runWithListError(t, &APIError{
		Op: "gmail: list", StatusCode: 401, Status: "401 Unauthorized",
	})

	if len(store.reconsentUsers) != 1 || store.reconsentUsers[0] != 9 {
		t.Errorf("reconsent = %v, want [9]", store.reconsentUsers)
	}
	if store.syncedCalled {
		t.Error("should not advance cursor when listing failed")
	}
	if stats.Reconsent != 1 || stats.Synced != 0 {
		t.Errorf("stats = %+v, want one connection flagged for re-consent", stats)
	}
}

// The revocation that actually happens in production never reaches the Gmail API at all:
// the reader's client is built from a stored refresh token, so Google refuses it at the
// TOKEN endpoint and oauth2 hands back a *url.Error wrapping *oauth2.RetrieveError.
func TestRunOnceMarksReconsentWhenTheTokenEndpointRefusesTheRefreshToken(t *testing.T) {
	store, stats := runWithListError(t, &url.Error{
		Op:  "Get",
		URL: "https://gmail.googleapis.com/gmail/v1/users/me/messages",
		Err: &oauth2.RetrieveError{ErrorCode: "invalid_grant", ErrorDescription: "Token has been expired or revoked."},
	})

	if len(store.reconsentUsers) != 1 || store.reconsentUsers[0] != 9 {
		t.Errorf("reconsent = %v, want [9] — invalid_grant at the token endpoint IS the revocation", store.reconsentUsers)
	}
	if stats.Reconsent != 1 {
		t.Errorf("stats = %+v, want one connection flagged for re-consent", stats)
	}
}

// Counting a re-consent the store refused to record would report a transition that did not
// happen. The mailbox is still `connected`, so the next run meets the same revoked grant and
// tries again — and nothing else notices in the meantime: the candidate is supposed to be
// told to reconnect by a status that was never written. That belongs in the failed count,
// which is what the exit code reads.
func TestRunOnceCountsAnUnwritableReconsentAsAFailure(t *testing.T) {
	c := testCipher(t)
	enc, _ := c.Encrypt("refresh-token")
	store := &fakeStore{
		conns:        []Connection{{UserID: 9, Cursor: 0}},
		encToken:     enc,
		reconsentErr: errors.New("connection reset by peer"),
	}
	reader := &fakeReader{listErr: &APIError{
		Op: "gmail: list", StatusCode: 401, Status: "401 Unauthorized",
	}}
	w := NewWorker(store, c, func(context.Context, string, []string) GmailReader { return reader })

	stats, err := w.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if stats.Reconsent != 0 || stats.Failed != 1 {
		t.Errorf("stats = %+v, want the connection counted as failed — the status never moved", stats)
	}
}

// Everything else is Google having a bad day, or this process being stopped. The status
// flag is SHARED with the calendar and only a browser consent clears it, so one incident
// read as a revocation disconnects every mailbox we hold at once — and the on-demand sync
// behind the inbox's Sync button runs this same path, so a click at a bad moment did it
// to one candidate on its own.
func TestRunOnceDoesNotDisconnectOverATransientFailure(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"rate limit", &APIError{Op: "gmail: list", StatusCode: 429, Status: "429 Too Many Requests"}},
		{"provider fault", &APIError{Op: "gmail: list", StatusCode: 503, Status: "503 Service Unavailable"}},
		{"connection reset", errors.New("read tcp: connection reset by peer")},
		{"run cancelled", context.Canceled},
		{"our own client credential is wrong", &url.Error{
			Op:  "Post",
			URL: "https://oauth2.googleapis.com/token",
			Err: &oauth2.RetrieveError{ErrorCode: "invalid_client"},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, stats := runWithListError(t, tc.err)

			if len(store.reconsentUsers) != 0 {
				t.Errorf("marked %v for re-consent over %v", store.reconsentUsers, tc.err)
			}
			if store.syncedCalled {
				t.Error("should not advance cursor when listing failed")
			}
			if stats.Failed != 1 || stats.Reconsent != 0 {
				t.Errorf("stats = %+v, want the connection counted as failed and retried next run", stats)
			}
		})
	}
}

// runWithListError syncs one connection whose listing fails with err, returning what the
// store saw and what the run reported.
func runWithListError(t *testing.T, listErr error) (*fakeStore, Stats) {
	t.Helper()
	c := testCipher(t)
	enc, _ := c.Encrypt("refresh-token")
	store := &fakeStore{conns: []Connection{{UserID: 9, Cursor: 0}}, encToken: enc}
	reader := &fakeReader{listErr: listErr}
	w := NewWorker(store, c, func(context.Context, string, []string) GmailReader { return reader })

	stats, err := w.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if stats.Connections != 1 {
		t.Fatalf("connections = %d, want 1", stats.Connections)
	}
	return store, stats
}
