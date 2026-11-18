package gmailsync

import (
	"context"
	"testing"
	"time"
)

// A revoked grant answered on the sent-mail listing must flag re-consent once, not re-401 every run.
func TestRunOnceRevokedSentListingMarksReconsent(t *testing.T) {
	c := testCipher(t)
	enc, _ := c.Encrypt("refresh-token")
	store := &fakeStore{conns: []Connection{{UserID: 7, Cursor: 100, SentCursor: 100}}, encToken: enc}
	reader := &fakeReader{sentErr: &APIError{StatusCode: 401, Status: "401 Unauthorized"}}
	w := NewWorker(store, c, func(context.Context, string, []string) GmailReader { return reader })

	stats, err := w.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if len(store.reconsentUsers) != 1 || store.reconsentUsers[0] != 7 || stats.Reconsent != 1 {
		t.Errorf("reconsent=%v stats=%+v, want [7] and Reconsent=1", store.reconsentUsers, stats)
	}
}

// A stored outgoing reply re-listed during thread expansion must not move the incoming watermark.
func TestRunOnceStoredOutgoingSiblingDoesNotMoveIncomingCursor(t *testing.T) {
	c := testCipher(t)
	enc, _ := c.Encrypt("refresh-token")
	out1 := Message{ID: "out1", ThreadID: "t1", Direction: DirectionOutgoing, ToAddrs: "r@acme.com", ReceivedAt: time.Unix(500, 0)}
	store := &fakeStore{
		conns:    []Connection{{UserID: 7, Cursor: 100, SentCursor: 100}},
		encToken: enc,
		upserted: []StoredEmail{{UserID: 7, Message: out1}},
	}
	reader := &fakeReader{
		ids:     []string{"in1"},
		byID:    map[string]Message{"in1": {ID: "in1", ThreadID: "t1", ToAddrs: "me@gmail.com", Subject: "Thanks for applying", ReceivedAt: time.Unix(200, 0)}},
		threads: map[string][]string{"t1": {"in1", "out1"}},
	}
	w := NewWorker(store, c, func(context.Context, string, []string) GmailReader { return reader })

	if _, err := w.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if store.syncedCursor != 200 {
		t.Errorf("incoming cursor = %d, want 200", store.syncedCursor)
	}
}
