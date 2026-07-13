package gmailsync

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/tokencrypt"
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
	syncedCursor   int64
	syncedCalled   bool
	reconsentUsers []int64
}

func (f *fakeStore) ListConnected(context.Context) ([]Connection, error) { return f.conns, nil }
func (f *fakeStore) RefreshToken(context.Context, int64) (string, error) { return f.encToken, nil }
func (f *fakeStore) UpsertEmail(_ context.Context, e StoredEmail) error {
	f.upserted = append(f.upserted, e)
	return nil
}
func (f *fakeStore) SetSynced(_ context.Context, _, cursor int64) error {
	f.syncedCalled = true
	f.syncedCursor = cursor
	return nil
}
func (f *fakeStore) SetNeedsReconsent(_ context.Context, userID int64) error {
	f.reconsentUsers = append(f.reconsentUsers, userID)
	return nil
}

type fakeReader struct {
	ids     []string
	byID    map[string]Message
	listErr error
}

func (f *fakeReader) ListATSMessageIDs(context.Context, int64) ([]string, error) {
	return f.ids, f.listErr
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
	w := NewWorker(store, c, func(context.Context, string) GmailReader { return reader })

	if err := w.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if len(store.upserted) != 2 {
		t.Fatalf("upserted = %d, want 2", len(store.upserted))
	}
	// Both messages normalize to the same grouping key.
	for _, e := range store.upserted {
		if e.SubjectNorm != "thank you for applying to acme" {
			t.Errorf("subject_norm = %q", e.SubjectNorm)
		}
	}
	if !store.syncedCalled || store.syncedCursor != t2.Unix() {
		t.Errorf("cursor = %d (called=%v), want %d", store.syncedCursor, store.syncedCalled, t2.Unix())
	}
}

func TestRunOnceRevokedTokenMarksReconsent(t *testing.T) {
	c := testCipher(t)
	enc, _ := c.Encrypt("refresh-token")
	store := &fakeStore{conns: []Connection{{UserID: 9, Cursor: 0}}, encToken: enc}
	reader := &fakeReader{listErr: errors.New("401 invalid_grant")}
	w := NewWorker(store, c, func(context.Context, string) GmailReader { return reader })

	if err := w.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if len(store.reconsentUsers) != 1 || store.reconsentUsers[0] != 9 {
		t.Errorf("reconsent = %v, want [9]", store.reconsentUsers)
	}
	if store.syncedCalled {
		t.Error("should not advance cursor when listing failed")
	}
}
