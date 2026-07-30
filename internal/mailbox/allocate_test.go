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
	addr, err := GetOrCreate(context.Background(), s, 1, "ivan@gmail.com", "inbox.freehire.me")
	if err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	if addr != "ivan@inbox.freehire.me" {
		t.Errorf("addr = %q", addr)
	}
}

func TestGetOrCreate_Collision(t *testing.T) {
	s := newFakeStore()
	s.taken["ivan@inbox.freehire.me"] = 999 // someone else already holds the bare handle
	addr, err := GetOrCreate(context.Background(), s, 1, "ivan@gmail.com", "inbox.freehire.me")
	if err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	if addr != "ivan-2@inbox.freehire.me" {
		t.Errorf("addr = %q, want suffixed", addr)
	}
}

// A user whose own local-part happens to be an operational name must not be handed the
// bare form of it on the receiving domain: those addresses are where abuse reports land and
// what a CA will mail to prove control of the domain.
func TestGetOrCreate_SkipsReservedHandles(t *testing.T) {
	for _, email := range []string{"postmaster@somewhere.io", "abuse@somewhere.io", "admin@somewhere.io"} {
		s := newFakeStore()
		addr, err := GetOrCreate(context.Background(), s, 1, email, "inbox.freehire.me")
		if err != nil {
			t.Fatalf("GetOrCreate(%q): %v", email, err)
		}
		base := Handle(email)
		if addr == base+"@inbox.freehire.me" {
			t.Errorf("GetOrCreate(%q) allocated the reserved address %q", email, addr)
		}
		if want := base + "-2@inbox.freehire.me"; addr != want {
			t.Errorf("GetOrCreate(%q) = %q, want the first non-reserved candidate %q", email, addr, want)
		}
	}
}

// A suffixed form of a reserved name is not itself an operational address, so nothing
// stops a user from holding it.
func TestGetOrCreate_ReservationIsExactLocalPartOnly(t *testing.T) {
	s := newFakeStore()
	addr, err := GetOrCreate(context.Background(), s, 1, "administrator.uk@somewhere.io", "inbox.freehire.me")
	if err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	if addr != "administrator.uk@inbox.freehire.me" {
		t.Errorf("addr = %q, want the bare handle (not a reserved name)", addr)
	}
}

func TestGetOrCreate_Idempotent(t *testing.T) {
	s := newFakeStore()
	first, _ := GetOrCreate(context.Background(), s, 1, "ivan@gmail.com", "inbox.freehire.me")
	second, err := GetOrCreate(context.Background(), s, 1, "ivan@gmail.com", "inbox.freehire.me")
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
