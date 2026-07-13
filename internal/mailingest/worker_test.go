package mailingest

import (
	"context"
	"errors"
	"testing"
)

// fakeSource yields one batch then empties, recording acks.
type fakeSource struct {
	batch []Inbound
	acked []string
}

func (f *fakeSource) Receive(context.Context) ([]Inbound, error) {
	b := f.batch
	f.batch = nil
	return b, nil
}

func (f *fakeSource) Ack(_ context.Context, handle string) error {
	f.acked = append(f.acked, handle)
	return nil
}

// fakeStore resolves known addresses and captures stored messages.
type fakeStore struct {
	byAddr    map[string]int64
	stored    []HostedMessage
	insertErr error
}

func (s *fakeStore) MailboxByAddress(_ context.Context, address string) (int64, bool, error) {
	id, ok := s.byAddr[address]
	return id, ok, nil
}

func (s *fakeStore) InsertMessage(_ context.Context, m HostedMessage) error {
	if s.insertErr != nil {
		return s.insertErr
	}
	s.stored = append(s.stored, m)
	return nil
}

func inbound(handle, recipient, raw string) Inbound {
	return Inbound{Raw: []byte(raw), Recipients: []string{recipient}, S3Key: "obj/" + handle, AckHandle: handle}
}

func TestRunOnce_KnownRecipientStored(t *testing.T) {
	src := &fakeSource{batch: []Inbound{inbound("m1", "ivan@inbox.freehire.dev", sampleMIME)}}
	store := &fakeStore{byAddr: map[string]int64{"ivan@inbox.freehire.dev": 42}}
	w := NewWorker(src, store, "inbox.freehire.dev")

	if err := w.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if len(store.stored) != 1 {
		t.Fatalf("stored %d messages, want 1", len(store.stored))
	}
	got := store.stored[0]
	if got.UserID != 42 {
		t.Errorf("UserID = %d, want 42", got.UserID)
	}
	if got.ExternalID != "abc123@acme.com" {
		t.Errorf("ExternalID = %q", got.ExternalID)
	}
	if len(src.acked) != 1 || src.acked[0] != "m1" {
		t.Errorf("acked = %v, want [m1]", src.acked)
	}
}

func TestRunOnce_MixedCaseRecipientResolves(t *testing.T) {
	// SES may hand us the envelope recipient in a different case than the stored
	// (always-lowercase) address; the worker must still resolve it.
	src := &fakeSource{batch: []Inbound{inbound("m1", "Ivan@Inbox.Freehire.Dev", sampleMIME)}}
	store := &fakeStore{byAddr: map[string]int64{"ivan@inbox.freehire.dev": 42}}
	w := NewWorker(src, store, "inbox.freehire.dev")

	if err := w.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if len(store.stored) != 1 || store.stored[0].UserID != 42 {
		t.Fatalf("mixed-case recipient not resolved: stored=%v", store.stored)
	}
}

func TestRunOnce_UnknownRecipientDropped(t *testing.T) {
	src := &fakeSource{batch: []Inbound{inbound("m1", "nobody@inbox.freehire.dev", sampleMIME)}}
	store := &fakeStore{byAddr: map[string]int64{}}
	w := NewWorker(src, store, "inbox.freehire.dev")

	if err := w.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if len(store.stored) != 0 {
		t.Errorf("stored %d, want 0 (dropped)", len(store.stored))
	}
	if len(src.acked) != 1 {
		t.Errorf("unknown recipient should be acked (dropped), acked = %v", src.acked)
	}
}

func TestRunOnce_MissingMessageIDUsesS3Key(t *testing.T) {
	raw := "From: solo@x.io\r\nSubject: hi\r\n\r\njust text\r\n" // no Message-ID
	src := &fakeSource{batch: []Inbound{inbound("m9", "ivan@inbox.freehire.dev", raw)}}
	store := &fakeStore{byAddr: map[string]int64{"ivan@inbox.freehire.dev": 7}}
	w := NewWorker(src, store, "inbox.freehire.dev")

	if err := w.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if len(store.stored) != 1 || store.stored[0].ExternalID != "s3:obj/m9" {
		t.Errorf("ExternalID = %v, want s3:obj/m9", store.stored)
	}
	if store.stored[0].ReceivedAt.IsZero() {
		t.Error("ReceivedAt should fall back to now, got zero")
	}
}

func TestRunOnce_StoreErrorNotAcked(t *testing.T) {
	src := &fakeSource{batch: []Inbound{inbound("m1", "ivan@inbox.freehire.dev", sampleMIME)}}
	store := &fakeStore{byAddr: map[string]int64{"ivan@inbox.freehire.dev": 42}, insertErr: errors.New("db down")}
	w := NewWorker(src, store, "inbox.freehire.dev")

	if err := w.RunOnce(context.Background()); err != nil {
		t.Fatalf("RunOnce should swallow per-message store error: %v", err)
	}
	if len(src.acked) != 0 {
		t.Errorf("store error must NOT ack (redelivery), acked = %v", src.acked)
	}
}
