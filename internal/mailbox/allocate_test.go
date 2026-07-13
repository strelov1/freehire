package mailbox

import (
	"context"
	"testing"
)

// fakeStore is an in-memory mailbox.Store for the allocator tests.
type fakeStore struct {
	byUser map[int64]string
	taken  map[string]int64 // address -> owner
}

func newFakeStore() *fakeStore {
	return &fakeStore{byUser: map[int64]string{}, taken: map[string]int64{}}
}

func (f *fakeStore) AddressByUser(_ context.Context, userID int64) (string, bool, error) {
	a, ok := f.byUser[userID]
	return a, ok, nil
}

func (f *fakeStore) Insert(_ context.Context, userID int64, address string) error {
	if _, ok := f.taken[address]; ok {
		return ErrTaken
	}
	if _, ok := f.byUser[userID]; ok {
		return ErrTaken // user already has a mailbox (user_id unique)
	}
	f.taken[address] = userID
	f.byUser[userID] = address
	return nil
}

func TestGetOrCreate_FreshUser(t *testing.T) {
	s := newFakeStore()
	addr, err := GetOrCreate(context.Background(), s, 1, "ivan@gmail.com", "inbox.freehire.dev")
	if err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	if addr != "ivan@inbox.freehire.dev" {
		t.Errorf("addr = %q", addr)
	}
}

func TestGetOrCreate_Collision(t *testing.T) {
	s := newFakeStore()
	s.taken["ivan@inbox.freehire.dev"] = 999 // someone else already holds the bare handle
	addr, err := GetOrCreate(context.Background(), s, 1, "ivan@gmail.com", "inbox.freehire.dev")
	if err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	if addr != "ivan-2@inbox.freehire.dev" {
		t.Errorf("addr = %q, want suffixed", addr)
	}
}

func TestGetOrCreate_Idempotent(t *testing.T) {
	s := newFakeStore()
	first, _ := GetOrCreate(context.Background(), s, 1, "ivan@gmail.com", "inbox.freehire.dev")
	second, err := GetOrCreate(context.Background(), s, 1, "ivan@gmail.com", "inbox.freehire.dev")
	if err != nil {
		t.Fatalf("GetOrCreate second: %v", err)
	}
	if first != second {
		t.Errorf("not idempotent: %q != %q", first, second)
	}
	if len(s.taken) != 1 {
		t.Errorf("allocated %d addresses, want 1", len(s.taken))
	}
}
