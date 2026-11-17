package gmailsync

import (
	"context"
	"errors"
	"net/url"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

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

// A 403 on an individual message fetch is the grant saying no exactly as much as a 403 on
// the list call is — RevokedGrant reads the status code, not which endpoint sent it. Before
// this test the fetch loop treated every GetMessage error alike (logged, counted as
// sawFailure, retried next run), so a mailbox whose grant lost message-read access relisted
// and re-403'd the SAME messages every run forever: never flagged for re-consent, because
// nothing on this path asked RevokedGrant. Production measured this at ~450k failed message
// fetches in 24h for one connection.
func TestRunOnceRevokedTokenOnMessageFetchMarksReconsent(t *testing.T) {
	c := testCipher(t)
	enc, _ := c.Encrypt("refresh-token")
	store := &fakeStore{conns: []Connection{{UserID: 9, Cursor: 1_700_000_000}}, encToken: enc}
	reader := &fakeReader{
		ids: []string{"m1", "m2"},
		byID: map[string]Message{
			"m2": {ID: "m2", Subject: "Thank you for applying to Acme", ReceivedAt: time.Unix(1_700_000_500, 0)},
		},
		getErrIDs: map[string]error{
			"m1": &APIError{Op: "gmail: get message m1", StatusCode: 403, Status: "403 Forbidden"},
		},
	}
	w := NewWorker(store, c, func(context.Context, string, []string) GmailReader { return reader })

	stats, err := w.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if len(store.reconsentUsers) != 1 || store.reconsentUsers[0] != 9 {
		t.Errorf("reconsent = %v, want [9]", store.reconsentUsers)
	}
	if store.syncedCalled {
		t.Error("should not advance the cursor when a message fetch revealed a revoked grant")
	}
	if stats.Reconsent != 1 || stats.Synced != 0 || stats.Failed != 0 {
		t.Errorf("stats = %+v, want one connection flagged for re-consent", stats)
	}
}

// A 500, a reset connection, or a message that would not store on a live grant says nothing
// about the grant — the existing per-message failure path, unchanged by the fix above.
func TestRunOnceDoesNotDisconnectOverATransientMessageFetchFailure(t *testing.T) {
	c := testCipher(t)
	enc, _ := c.Encrypt("refresh-token")
	store := &fakeStore{conns: []Connection{{UserID: 9, Cursor: 0}}, encToken: enc}
	reader := &fakeReader{
		ids:       []string{"m1"},
		byID:      map[string]Message{},
		getErrIDs: map[string]error{"m1": &APIError{Op: "gmail: get message m1", StatusCode: 503, Status: "503 Service Unavailable"}},
	}
	w := NewWorker(store, c, func(context.Context, string, []string) GmailReader { return reader })

	stats, err := w.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if len(store.reconsentUsers) != 0 {
		t.Errorf("marked %v for re-consent over a transient fetch failure", store.reconsentUsers)
	}
	if stats.Failed != 1 || stats.Reconsent != 0 {
		t.Errorf("stats = %+v, want the connection counted as failed and retried next run", stats)
	}
}

// A third Gmail API call — thread expansion's ListThreadMessageIDs — can reveal a revoked
// grant exactly as the list call and a message fetch can, and was the one call site this
// PR's own review found still unchecked: its error path only logged and continued,
// leaving a connection that lost thread-read access mid-run un-flagged and retried by cron
// forever, the same bug class this whole PR exists to close.
func TestRunOnceRevokedTokenOnThreadListingMarksReconsent(t *testing.T) {
	c := testCipher(t)
	enc, _ := c.Encrypt("refresh-token")
	store := &fakeStore{conns: []Connection{{UserID: 9, Cursor: 1_700_000_000}}, encToken: enc}
	reader := &fakeReader{
		ids: []string{"m1"},
		byID: map[string]Message{
			"m1": {ID: "m1", ThreadID: "t1", FromAddr: "no-reply@ashbyhq.com", Subject: "Thanks for applying", ReceivedAt: time.Unix(1_700_000_100, 0)},
		},
		threadErrIDs: map[string]error{
			"t1": &APIError{Op: "gmail: thread t1", StatusCode: 403, Status: "403 Forbidden"},
		},
	}
	w := NewWorker(store, c, func(context.Context, string, []string) GmailReader { return reader })

	stats, err := w.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if len(store.reconsentUsers) != 1 || store.reconsentUsers[0] != 9 {
		t.Errorf("reconsent = %v, want [9]", store.reconsentUsers)
	}
	if store.syncedCalled {
		t.Error("should not advance the cursor when a thread listing revealed a revoked grant")
	}
	if stats.Reconsent != 1 || stats.Synced != 0 || stats.Failed != 0 {
		t.Errorf("stats = %+v, want one connection flagged for re-consent", stats)
	}
}

// A transient thread failure retains both cursors so its siblings can be retried.
func TestRunOnceDoesNotDisconnectOverATransientThreadListingFailure(t *testing.T) {
	c := testCipher(t)
	enc, _ := c.Encrypt("refresh-token")
	store := &fakeStore{conns: []Connection{{UserID: 9, Cursor: 0}}, encToken: enc}
	reader := &fakeReader{
		ids: []string{"m1"},
		byID: map[string]Message{
			"m1": {ID: "m1", ThreadID: "t1", FromAddr: "no-reply@ashbyhq.com", Subject: "Thanks for applying", ReceivedAt: time.Unix(1_700_000_100, 0)},
		},
		threadErrIDs: map[string]error{
			"t1": &APIError{Op: "gmail: thread t1", StatusCode: 503, Status: "503 Service Unavailable"},
		},
	}
	w := NewWorker(store, c, func(context.Context, string, []string) GmailReader { return reader })

	stats, err := w.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if len(store.reconsentUsers) != 0 {
		t.Errorf("marked %v for re-consent over a transient thread listing failure", store.reconsentUsers)
	}
	if len(store.upserted) != 1 || store.upserted[0].Message.ID != "m1" {
		t.Fatalf("upserted = %v, want only the anchor m1 (thread expansion failed, not the anchor)", store.upserted)
	}
	if stats.Failed != 1 || stats.Synced != 0 || stats.Reconsent != 0 {
		t.Errorf("stats = %+v, want one failed connection with retry coverage", stats)
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
