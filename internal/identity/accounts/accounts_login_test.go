package accounts

import (
	"context"
	"errors"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

// Login when UserByEmail returns ErrUserNotFound → ErrInvalidCredentials.
func TestLogin_UserNotFound(t *testing.T) {
	repo := &fakeRepo{
		userByEmailResults: []userByEmailResult{{err: ErrUserNotFound}},
	}
	svc := New(repo, &fakeHasher{})

	_, err := svc.Login(context.Background(), "user@example.com", "password123")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("want ErrInvalidCredentials, got %v", err)
	}
}

// Login when hasPassword=false → ErrInvalidCredentials, but a dummy Check still
// runs so the passwordless path costs one bcrypt like a real account (timing guard
// against account enumeration).
func TestLogin_NoPassword(t *testing.T) {
	wantUser := User{ID: 5, Email: "user@example.com"}
	repo := &fakeRepo{
		userByEmailResults: []userByEmailResult{{user: wantUser, hasPassword: false}},
	}
	hasher := &fakeHasher{}
	svc := New(repo, hasher)

	_, err := svc.Login(context.Background(), "user@example.com", "password123")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("want ErrInvalidCredentials, got %v", err)
	}
	if hasher.checkCalls != 1 {
		t.Errorf("dummy Check must run exactly once on the passwordless path, got %d calls", hasher.checkCalls)
	}
}

// Login for an unknown email spends a dummy Check too, so an unknown account is
// timing-indistinguishable from a real one.
func TestLogin_UserNotFound_SpendsDummyCheck(t *testing.T) {
	repo := &fakeRepo{
		userByEmailResults: []userByEmailResult{{err: ErrUserNotFound}},
	}
	hasher := &fakeHasher{}
	svc := New(repo, hasher)

	if _, err := svc.Login(context.Background(), "ghost@example.com", "password123"); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("want ErrInvalidCredentials, got %v", err)
	}
	if hasher.checkCalls != 1 {
		t.Errorf("dummy Check must run exactly once for an unknown email, got %d calls", hasher.checkCalls)
	}
}

// dummyPasswordHash must be a well-formed bcrypt hash — a malformed one would make
// CompareHashAndPassword return immediately without spending bcrypt work, silently
// re-opening the timing side-channel.
func TestDummyPasswordHashIsWellFormed(t *testing.T) {
	if _, err := bcrypt.Cost([]byte(dummyPasswordHash)); err != nil {
		t.Fatalf("dummyPasswordHash is not a valid bcrypt hash: %v", err)
	}
}

// Login when Check fails → ErrInvalidCredentials.
func TestLogin_WrongPassword(t *testing.T) {
	wantUser := User{ID: 5, Email: "user@example.com"}
	repo := &fakeRepo{
		userByEmailResults: []userByEmailResult{{user: wantUser, passwordHash: "hashed:correct", hasPassword: true}},
	}
	svc := New(repo, &fakeHasher{})

	_, err := svc.Login(context.Background(), "user@example.com", "wrongpassword")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("want ErrInvalidCredentials, got %v", err)
	}
}

// Login success → returns user, flagged as password-backed.
func TestLogin_Happy(t *testing.T) {
	stored := User{ID: 5, Email: "user@example.com"}
	// The lookup row does not carry HasPassword; Login sets it, because authenticating
	// by password is itself the proof that one exists.
	wantUser := stored
	wantUser.HasPassword = true
	repo := &fakeRepo{
		userByEmailResults: []userByEmailResult{{user: stored, passwordHash: "hashed:correct", hasPassword: true}},
	}
	svc := New(repo, &fakeHasher{})

	got, err := svc.Login(context.Background(), "USER@Example.COM", "correct")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != wantUser {
		t.Errorf("want %+v, got %+v", wantUser, got)
	}
}

// UserByID found → returns user.
func TestUserByID_Found(t *testing.T) {
	wantUser := User{ID: 10, Email: "someone@example.com"}
	repo := &fakeRepo{
		userByIDResults: []userByIDResult{{user: wantUser}},
	}
	svc := New(repo, &fakeHasher{})

	got, err := svc.UserByID(context.Background(), 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != wantUser {
		t.Errorf("want %+v, got %+v", wantUser, got)
	}
}

// UserByID when repo returns ErrUserNotFound → propagated.
func TestUserByID_NotFound(t *testing.T) {
	repo := &fakeRepo{
		userByIDResults: []userByIDResult{{err: ErrUserNotFound}},
	}
	svc := New(repo, &fakeHasher{})

	_, err := svc.UserByID(context.Background(), 99)
	if !errors.Is(err, ErrUserNotFound) {
		t.Errorf("want ErrUserNotFound, got %v", err)
	}
}

// Login answers with the same wire shape as /auth/me, so a client that renders the
// password UI from it must not be told the account is passwordless when it is not.
func TestLoginReportsThatTheAccountHasAPassword(t *testing.T) {
	repo := &fakeRepo{userByEmailResults: []userByEmailResult{
		{user: User{ID: 7, Email: "user@example.test"}, passwordHash: "hashed:pw", hasPassword: true},
	}}
	s := New(repo, &fakeHasher{})

	user, err := s.Login(context.Background(), "user@example.test", "pw")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}
	if !user.HasPassword {
		t.Error("login returned HasPassword false for an account that just authenticated by password")
	}
}
