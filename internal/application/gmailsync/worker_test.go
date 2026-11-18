package gmailsync

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

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
	cooldown          time.Time
	cooldownReadErr   error
	cooldownWriteErr  error
	conns             []Connection
	encToken          string
	upserted          []StoredEmail
	upsertErrIDs      map[string]bool // message ids on which UpsertEmail fails
	syncedSent        int64
	knownThreads      []string
	knownThreadsErr   error
	knownThreadsCalls int
	syncedCursor      int64
	syncedCalled      bool
	reconsentUsers    []int64
	reconsentErr      error
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
func (f *fakeStore) SetSynced(_ context.Context, _, cursor, sentCursor int64) error {
	f.syncedCalled = true
	f.syncedCursor = cursor
	f.syncedSent = sentCursor
	return nil
}
func (f *fakeStore) SetNeedsReconsent(_ context.Context, userID int64) error {
	if f.reconsentErr != nil {
		return f.reconsentErr
	}
	f.reconsentUsers = append(f.reconsentUsers, userID)
	return nil
}

func (f *fakeStore) ListThreadIDs(context.Context, int64) ([]string, error) {
	f.knownThreadsCalls++
	return f.knownThreads, f.knownThreadsErr
}

// StoredMessages answers from what this fake has upserted, leaving out rows with no recipients
// (legacy rows a fetch repairs), as the real store does.
func (f *fakeStore) StoredMessages(_ context.Context, _ int64, ids []string) (map[string]StoredRef, error) {
	want := make(map[string]bool, len(ids))
	for _, id := range ids {
		want[id] = true
	}
	out := make(map[string]StoredRef)
	for _, e := range f.upserted {
		if want[e.Message.ID] && e.Message.ToAddrs != "" {
			out[e.Message.ID] = StoredRef{ThreadID: e.Message.ThreadID, ReceivedAt: e.Message.ReceivedAt, Incoming: e.Message.Direction != DirectionOutgoing}
		}
	}
	return out, nil
}

type fakeReader struct {
	sent         []MessageRef
	sentErr      error
	ids          []string
	byID         map[string]Message
	threads      map[string][]string // threadID -> message ids
	listErr      error
	getErrIDs    map[string]error // message ids on which GetMessage fails
	gets         []string         // message ids GetMessage was asked for, in order
	threadErrIDs map[string]error // thread ids on which ListThreadMessageIDs fails
}

func (f *fakeReader) ListSentMessages(context.Context, int64) ([]MessageRef, error) {
	return f.sent, f.sentErr
}

func (f *fakeReader) ListATSMessageIDs(context.Context, string, int64) ([]string, error) {
	return f.ids, f.listErr
}
func (f *fakeReader) ListThreadMessageIDs(_ context.Context, threadID string) ([]string, error) {
	if err := f.threadErrIDs[threadID]; err != nil {
		return nil, err
	}
	return f.threads[threadID], nil
}
func (f *fakeReader) GetMessage(_ context.Context, id string) (Message, error) {
	f.gets = append(f.gets, id)
	if err := f.getErrIDs[id]; err != nil {
		return Message{}, err
	}
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
// in-thread reply is retained as conversation history.
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
	if !got["m9"] {
		t.Error("user reply m9 must be retained")
	}
	if len(store.upserted) != 3 {
		t.Errorf("upserted = %d, want 3 (no dupes)", len(store.upserted))
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

// A first sync too big for one run's rate limit must still finish: the run that hits the
// limit keeps its cursor, so the next run lists the same messages and must skip the ones
// already stored rather than re-fetch them and stop at the same limit again.
func TestRunOnceResumesAThrottledSyncWithoutRefetchingStoredMessages(t *testing.T) {
	c := testCipher(t)
	enc, _ := c.Encrypt("refresh-token")
	store := &fakeStore{conns: []Connection{{UserID: 7, Email: "u@gmail.com"}}, encToken: enc}
	t1, t2, t3 := time.Unix(1_700_000_100, 0), time.Unix(1_700_000_200, 0), time.Unix(1_700_000_300, 0)
	throttle := &APIError{StatusCode: http.StatusForbidden, Reason: "rateLimitExceeded"}
	reader := &fakeReader{
		ids: []string{"m3", "m2", "m1"},
		byID: map[string]Message{
			"m1": {ID: "m1", ThreadID: "t1", ToAddrs: "u@gmail.com", Subject: "Thanks for applying", ReceivedAt: t1},
			"m2": {ID: "m2", ThreadID: "t2", ToAddrs: "u@gmail.com", Subject: "Thanks for applying", ReceivedAt: t2},
			"m3": {ID: "m3", ThreadID: "t3", ToAddrs: "u@gmail.com", Subject: "Thanks for applying", ReceivedAt: t3},
		},
		getErrIDs: map[string]error{"m2": throttle},
	}
	w := NewWorker(store, c, func(context.Context, string, []string) GmailReader { return reader })

	if stats, _ := w.RunOnce(context.Background()); stats.Throttled != 1 {
		t.Fatalf("first run stats = %+v, want the connection counted as throttled", stats)
	}
	if store.syncedCursor != 0 {
		t.Fatalf("cursor after the throttled run = %d, want 0", store.syncedCursor)
	}

	reader.gets = nil
	if stats, _ := w.RunOnce(context.Background()); stats.Throttled != 1 || len(reader.gets) != 0 {
		t.Fatalf("cooldown made API calls: %+v %v", stats, reader.gets)
	}
	store.cooldown = time.Time{}
	delete(reader.getErrIDs, "m2")
	reader.gets = nil
	stats, err := w.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if stats.Synced != 1 {
		t.Fatalf("second run stats = %+v, want synced", stats)
	}
	for _, id := range reader.gets {
		if id == "m3" {
			t.Errorf("second run fetched m3 again (gets = %v); it was stored by the first run", reader.gets)
		}
	}
	if store.syncedCursor != t3.Unix() {
		t.Errorf("cursor = %d, want %d: a skipped stored message still counts toward the watermark", store.syncedCursor, t3.Unix())
	}
}

// A row stored before recipients were recorded may be a sent reply saved as incoming; the
// fetch is what repairs it, so a legacy row is fetched again rather than skipped.
func TestRunOnceRefetchesALegacyStoredMessage(t *testing.T) {
	c := testCipher(t)
	enc, _ := c.Encrypt("refresh-token")
	legacy := Message{ID: "m1", ThreadID: "t1", FromAddr: "u@gmail.com", ReceivedAt: time.Unix(1_700_000_100, 0)}
	store := &fakeStore{
		conns:    []Connection{{UserID: 7, Email: "u@gmail.com"}},
		encToken: enc,
		upserted: []StoredEmail{{UserID: 7, Message: legacy}},
	}
	repaired := legacy
	repaired.ToAddrs, repaired.Direction = "hr@acme.test", DirectionOutgoing
	reader := &fakeReader{ids: []string{"m1"}, byID: map[string]Message{"m1": repaired}}
	w := NewWorker(store, c, func(context.Context, string, []string) GmailReader { return reader })

	if _, err := w.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if len(reader.gets) != 1 || reader.gets[0] != "m1" {
		t.Fatalf("gets = %v, want the legacy m1 fetched again", reader.gets)
	}
	if last := store.upserted[len(store.upserted)-1].Message; last.Direction != DirectionOutgoing {
		t.Errorf("re-stored m1 direction = %q, want outgoing", last.Direction)
	}
}

func (f *fakeStore) CooldownUntil(context.Context, int64) (time.Time, error) {
	return f.cooldown, f.cooldownReadErr
}
func (f *fakeStore) SetCooldown(context.Context, int64) error {
	if f.cooldownWriteErr != nil {
		return f.cooldownWriteErr
	}
	f.cooldown = time.Now().Add(time.Hour)
	return nil
}

func TestThrottlePersistsCooldownBeforeThreadDiscovery(t *testing.T) {
	c := testCipher(t)
	enc, err := c.Encrypt("refresh-token")
	if err != nil {
		t.Fatal(err)
	}
	store := &fakeStore{conns: []Connection{{UserID: 7}}, encToken: enc, knownThreadsErr: errors.New("thread storage unavailable")}
	reader := &fakeReader{ids: []string{"limited"}, getErrIDs: map[string]error{"limited": &APIError{StatusCode: http.StatusForbidden, Reason: "rateLimitExceeded"}}}
	w := NewWorker(store, c, func(context.Context, string, []string) GmailReader { return reader })
	stats, err := w.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.Throttled != 1 || store.cooldown.IsZero() || store.knownThreadsCalls != 0 || store.syncedCalled {
		t.Fatalf("stats=%+v cooldown=%v threadCalls=%d synced=%v", stats, store.cooldown, store.knownThreadsCalls, store.syncedCalled)
	}
}

func TestCooldownFailuresRemainVisible(t *testing.T) {
	for _, stage := range []string{"read", "write"} {
		t.Run(stage, func(t *testing.T) {
			c := testCipher(t)
			enc, _ := c.Encrypt("refresh")
			store := &fakeStore{conns: []Connection{{UserID: 7}}, encToken: enc}
			failure := errors.New("cooldown storage unavailable")
			if stage == "read" {
				store.cooldownReadErr = failure
			} else {
				store.cooldownWriteErr = failure
			}
			reader := &fakeReader{listErr: &APIError{StatusCode: 429}}
			calls := 0
			w := NewWorker(store, c, func(context.Context, string, []string) GmailReader { calls++; return reader })
			stats, err := w.RunOnce(context.Background())
			if err != nil || stats.Failed != 1 || stats.Throttled != 0 || store.syncedCalled {
				t.Fatalf("stats=%+v err=%v", stats, err)
			}
			if stage == "read" && calls != 0 {
				t.Fatal("queried Gmail despite unreadable cooldown")
			}
		})
	}
}
