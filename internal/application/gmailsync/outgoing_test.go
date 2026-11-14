package gmailsync

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestParseSentAlias(t *testing.T) {
	msg, err := parseMessage([]byte(`{"id":"reply","threadId":"hiring","labelIds":["SENT"],"payload":{"headers":[{"name":"From","value":"Alias <alias@example.test>"},{"name":"To","value":"Recruiter <hr@example.test>"},{"name":"Cc","value":"team@example.test"}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if msg.Direction != DirectionOutgoing || msg.ToAddrs != "Recruiter <hr@example.test>" || msg.CcAddrs != "team@example.test" {
		t.Fatalf("sent message: %+v", msg)
	}
}

func TestSentReplyInOldThread(t *testing.T) {
	for _, first := range []bool{true, false} {
		t.Run(map[bool]string{true: "backfill", false: "incremental"}[first], func(t *testing.T) {
			c := testCipher(t)
			enc, _ := c.Encrypt("refresh")
			cursor := int64(1_700_000_100)
			sentCursor := cursor
			if first {
				sentCursor = 0
			}
			store := &fakeStore{conns: []Connection{{UserID: 7, Cursor: cursor, SentCursor: sentCursor}}, encToken: enc, knownThreads: []string{"old"}}
			reader := &fakeReader{
				byID:      map[string]Message{"sent": {ID: "sent", ThreadID: "old", Direction: DirectionOutgoing, FromAddr: "alias@example.test", ReceivedAt: time.Unix(cursor+100, 0)}},
				threads:   map[string][]string{"old": {"sent"}},
				sent:      []MessageRef{{ID: "sent", ThreadID: "old"}, {ID: "personal", ThreadID: "private"}},
				getErrIDs: map[string]error{"personal": errors.New("unrelated body must never be fetched")},
			}
			w := NewWorker(store, c, func(context.Context, string, []string) GmailReader { return reader })
			stats, err := w.RunOnce(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if stats.Synced != 1 || len(store.upserted) != 1 || store.upserted[0].Message.ID != "sent" {
				t.Fatalf("stats=%+v stored=%+v", stats, store.upserted)
			}
			if store.syncedCursor != cursor {
				t.Fatalf("sent reply moved incoming cursor: %d", store.syncedCursor)
			}
			if store.syncedSent <= sentCursor {
				t.Fatalf("sent cursor did not advance: %d", store.syncedSent)
			}
		})
	}
}

func TestSentFailurePreservesCursors(t *testing.T) {
	c := testCipher(t)
	enc, _ := c.Encrypt("refresh")
	store := &fakeStore{conns: []Connection{{UserID: 7, Cursor: 50, SentCursor: 60}}, encToken: enc}
	reader := &fakeReader{sentErr: errors.New("temporary outage")}
	w := NewWorker(store, c, func(context.Context, string, []string) GmailReader { return reader })
	stats, err := w.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.Failed != 1 || store.syncedCursor != 50 || store.syncedSent != 60 {
		t.Fatalf("stats=%+v cursors=%d/%d", stats, store.syncedCursor, store.syncedSent)
	}
}

func TestDraftInHiringThreadIsNotStored(t *testing.T) {
	c := testCipher(t)
	enc, _ := c.Encrypt("refresh")
	store := &fakeStore{conns: []Connection{{UserID: 7, Cursor: 100}}, encToken: enc, knownThreads: []string{"old"}}
	reader := &fakeReader{threads: map[string][]string{"old": {"draft"}}, byID: map[string]Message{"draft": {ID: "draft", ThreadID: "old", Draft: true, ReceivedAt: time.Unix(200, 0)}}}
	w := NewWorker(store, c, func(context.Context, string, []string) GmailReader { return reader })
	stats, err := w.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.Synced != 1 || len(store.upserted) != 0 || store.syncedCursor != 100 {
		t.Fatalf("draft stored or moved incoming cursor: stats=%+v stored=%+v cursor=%d", stats, store.upserted, store.syncedCursor)
	}
}

func TestDeletedOldThreadDoesNotStallBackfill(t *testing.T) {
	c := testCipher(t)
	enc, _ := c.Encrypt("refresh")
	store := &fakeStore{conns: []Connection{{UserID: 7, Cursor: 100}}, encToken: enc, knownThreads: []string{"deleted", "remaining"}}
	reader := &fakeReader{threads: map[string][]string{"remaining": {"gone"}}, threadErrIDs: map[string]error{"deleted": &APIError{StatusCode: 404}}, getErrIDs: map[string]error{"gone": &APIError{StatusCode: 404}}}
	w := NewWorker(store, c, func(context.Context, string, []string) GmailReader { return reader })
	stats, err := w.RunOnce(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.Synced != 1 || store.syncedSent == 0 {
		t.Fatalf("permanently deleted mail stalled backfill: %+v sent_cursor=%d", stats, store.syncedSent)
	}
}

func TestIncomingSearchExcludesSentAndDrafts(t *testing.T) {
	query := BuildQueryFor("me@example.test", 100, nil)
	for _, term := range []string{"-in:sent", "-in:drafts"} {
		if !strings.Contains(query, term) {
			t.Fatalf("incoming discovery includes candidate content: %s", query)
		}
	}
}

func TestOwnInboxDigestDoesNotEnterHiringMail(t *testing.T) {
	c := testCipher(t)
	enc, _ := c.Encrypt("refresh")
	msg, err := parseMessage([]byte(`{"id":"digest","internalDate":"1700000000000","payload":{"headers":[{"name":"Subject","value":"Your daily hiring inbox"},{"name":"Auto-Submitted","value":"auto-generated"},{"name":"X-Freehire-Notification","value":"inbox-digest"}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	store := &fakeStore{conns: []Connection{{UserID: 7}}, encToken: enc}
	reader := &fakeReader{ids: []string{"digest"}, byID: map[string]Message{"digest": msg}}
	w := NewWorker(store, c, func(context.Context, string, []string) GmailReader { return reader })
	if _, err := w.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.upserted) != 0 {
		t.Fatal("generated digest entered the hiring inbox")
	}
}
