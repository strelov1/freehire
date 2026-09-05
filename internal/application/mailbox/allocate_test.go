package mailbox

import (
	"context"
	"errors"
	"testing"
)

// fakeStore is an in-memory mailbox.Store for the allocator tests. trace, when
// non-nil, records call order across both fakeStore and fakeEnsurer.
type fakeStore struct {
	enrolled map[int64]bool
	trace    *[]string
}

func newFakeStore() *fakeStore {
	return &fakeStore{enrolled: map[int64]bool{}}
}

func (f *fakeStore) EnsureRow(_ context.Context, userID int64) error {
	if f.trace != nil {
		*f.trace = append(*f.trace, "EnsureRow")
	}
	f.enrolled[userID] = true
	return nil
}

// fakeEnsurer is an in-memory UsernameEnsurer: each userID gets a canned
// username or error, set up per test.
type fakeEnsurer struct {
	usernames map[int64]string
	errs      map[int64]error
	calls     []int64
	trace     *[]string
}

func (f *fakeEnsurer) EnsureUsername(_ context.Context, userID int64, _ string) (string, error) {
	if f.trace != nil {
		*f.trace = append(*f.trace, "EnsureUsername")
	}
	f.calls = append(f.calls, userID)
	if err, ok := f.errs[userID]; ok {
		return "", err
	}
	return f.usernames[userID], nil
}

func TestGetOrCreate_ComposesAddressFromUsername(t *testing.T) {
	s := newFakeStore()
	e := &fakeEnsurer{usernames: map[int64]string{1: "ivan"}}

	addr, err := GetOrCreate(context.Background(), s, e, 1, "ivan@gmail.com", "inbox.freehire.me")
	if err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	if addr != "ivan@inbox.freehire.me" {
		t.Errorf("addr = %q, want %q", addr, "ivan@inbox.freehire.me")
	}
}

func TestGetOrCreate_EnrollsTheUser(t *testing.T) {
	s := newFakeStore()
	e := &fakeEnsurer{usernames: map[int64]string{1: "ivan"}}

	if _, err := GetOrCreate(context.Background(), s, e, 1, "ivan@gmail.com", "inbox.freehire.me"); err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	if !s.enrolled[1] {
		t.Error("GetOrCreate did not enroll the user in the Store")
	}
}

func TestGetOrCreate_IsIdempotent(t *testing.T) {
	s := newFakeStore()
	e := &fakeEnsurer{usernames: map[int64]string{1: "ivan"}}
	ctx := context.Background()

	first, err := GetOrCreate(ctx, s, e, 1, "ivan@gmail.com", "inbox.freehire.me")
	if err != nil {
		t.Fatalf("GetOrCreate (first): %v", err)
	}
	second, err := GetOrCreate(ctx, s, e, 1, "ivan@gmail.com", "inbox.freehire.me")
	if err != nil {
		t.Fatalf("GetOrCreate (second): %v", err)
	}
	if first != second {
		t.Errorf("not idempotent: %q != %q", first, second)
	}
}

func TestGetOrCreate_ResolvesUsernameBeforeEnrolling(t *testing.T) {
	var trace []string
	s := newFakeStore()
	s.trace = &trace
	e := &fakeEnsurer{usernames: map[int64]string{1: "ivan"}, trace: &trace}

	if _, err := GetOrCreate(context.Background(), s, e, 1, "ivan@gmail.com", "inbox.freehire.me"); err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	want := []string{"EnsureUsername", "EnsureRow"}
	if len(trace) != len(want) || trace[0] != want[0] || trace[1] != want[1] {
		t.Errorf("call order = %v, want %v (username first — enrolling first would leave a mailbox row with no username on a partial failure)", trace, want)
	}
}

func TestGetOrCreate_DoesNotEnrollWhenEnsureUsernameFails(t *testing.T) {
	s := newFakeStore()
	e := &fakeEnsurer{errs: map[int64]error{1: errors.New("boom")}}

	if _, err := GetOrCreate(context.Background(), s, e, 1, "ivan@gmail.com", "inbox.freehire.me"); err == nil {
		t.Fatal("GetOrCreate: want an error")
	}
	if s.enrolled[1] {
		t.Error("GetOrCreate enrolled the user despite EnsureUsername failing")
	}
}

func TestGetOrCreate_PropagatesEnsureUsernameError(t *testing.T) {
	s := newFakeStore()
	wantErr := errors.New("boom")
	e := &fakeEnsurer{errs: map[int64]error{1: wantErr}}

	if _, err := GetOrCreate(context.Background(), s, e, 1, "ivan@gmail.com", "inbox.freehire.me"); !errors.Is(err, wantErr) {
		t.Errorf("GetOrCreate error = %v, want %v", err, wantErr)
	}
}
