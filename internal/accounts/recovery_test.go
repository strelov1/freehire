package accounts

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// recoveryService wires a Service with the code store, mailer, and a frozen clock.
func recoveryService(repo Repository, codes CodeStore, mailer CodeMailer) *Service {
	return verifyService(repo, codes, mailer, time.Now())
}

func TestRequestPasswordResetMailsACodeToAKnownAccount(t *testing.T) {
	repo := &fakeRepo{userByEmailResults: []userByEmailResult{
		{user: User{ID: 7, Email: "user@example.test"}, passwordHash: "hashed:old", hasPassword: true},
	}}
	codes, mailer := newFakeCodes(), &fakeMailer{}
	s := recoveryService(repo, codes, mailer)

	if err := s.RequestPasswordReset(context.Background(), "user@example.test"); err != nil {
		t.Fatalf("RequestPasswordReset: %v", err)
	}
	if len(mailer.reset) != 1 {
		t.Fatalf("mailed %d reset codes, want 1", len(mailer.reset))
	}
	if len(mailer.reset[0]) != 6 {
		t.Errorf("code = %q, want six digits", mailer.reset[0])
	}
}

// The endpoint must not be an account-enumeration oracle: an unknown address gets the
// same nil error as a known one, and nothing is mailed.
func TestRequestPasswordResetIsSilentAboutUnknownAddresses(t *testing.T) {
	repo := &fakeRepo{userByEmailResults: []userByEmailResult{{err: ErrUserNotFound}}}
	mailer := &fakeMailer{}
	s := recoveryService(repo, newFakeCodes(), mailer)

	if err := s.RequestPasswordReset(context.Background(), "nobody@example.test"); err != nil {
		t.Errorf("err = %v, want nil so the caller cannot tell the address is unknown", err)
	}
	if len(mailer.reset) != 0 {
		t.Error("a code was mailed for an address with no account")
	}
}

// An OAuth-only account can set a password through this flow. Receiving the mailed code
// proves control of the address — the same proof the provider offers — so refusing would
// only strand a user who wants a second way in, without buying any safety.
func TestRequestPasswordResetServesPasswordlessAccounts(t *testing.T) {
	repo := &fakeRepo{userByEmailResults: []userByEmailResult{
		{user: User{ID: 7, Email: "oauth@example.test"}, hasPassword: false},
	}}
	mailer := &fakeMailer{}
	s := recoveryService(repo, newFakeCodes(), mailer)

	if err := s.RequestPasswordReset(context.Background(), "oauth@example.test"); err != nil {
		t.Fatalf("RequestPasswordReset: %v", err)
	}
	if len(mailer.reset) != 1 {
		t.Fatalf("mailed %d codes, want 1 — a passwordless account may set a password", len(mailer.reset))
	}
}

// Setting the first password on an OAuth-only account works the same way as replacing
// an existing one.
func TestResetPasswordSetsAFirstPassword(t *testing.T) {
	repo := &fakeRepo{userByEmailResults: []userByEmailResult{
		{user: User{ID: 7, Email: "oauth@example.test"}, hasPassword: false},
		{user: User{ID: 7, Email: "oauth@example.test"}, hasPassword: false},
	}}
	codes, mailer := newFakeCodes(), &fakeMailer{}
	s := recoveryService(repo, codes, mailer)

	if err := s.RequestPasswordReset(context.Background(), "oauth@example.test"); err != nil {
		t.Fatalf("RequestPasswordReset: %v", err)
	}
	if err := s.ResetPassword(context.Background(), "oauth@example.test", mailer.reset[0], "first-password"); err != nil {
		t.Fatalf("ResetPassword: %v", err)
	}
	if repo.resetHash != "hashed:first-password" {
		t.Errorf("stored hash = %q, want the new password's hash", repo.resetHash)
	}
}

func TestResetPasswordSetsTheHashAndRevokesSessions(t *testing.T) {
	repo := &fakeRepo{userByEmailResults: []userByEmailResult{
		{user: User{ID: 7, Email: "user@example.test"}, passwordHash: "hashed:old", hasPassword: true},
		{user: User{ID: 7, Email: "user@example.test"}, passwordHash: "hashed:old", hasPassword: true},
	}}
	codes, mailer := newFakeCodes(), &fakeMailer{}
	s := recoveryService(repo, codes, mailer)

	if err := s.RequestPasswordReset(context.Background(), "user@example.test"); err != nil {
		t.Fatalf("RequestPasswordReset: %v", err)
	}
	if err := s.ResetPassword(context.Background(), "user@example.test", mailer.reset[0], "brand-new-pw"); err != nil {
		t.Fatalf("ResetPassword: %v", err)
	}
	if repo.resetHash != "hashed:brand-new-pw" {
		t.Errorf("stored hash = %q, want the new password's hash", repo.resetHash)
	}
	if _, err := codes.Code(context.Background(), 7, PurposeResetPassword); !errors.Is(err, ErrNoCode) {
		t.Error("the reset code must be consumed, so it cannot be replayed")
	}
}

func TestResetPasswordRejectsAWrongCode(t *testing.T) {
	repo := &fakeRepo{userByEmailResults: []userByEmailResult{
		{user: User{ID: 7}, passwordHash: "hashed:old", hasPassword: true},
	}}
	codes := newFakeCodes()
	now := time.Now()
	codes.put(7, PurposeResetPassword, StoredCode{
		Hash: "hashed:123456", ExpiresAt: now.Add(time.Minute), IssuedAt: now,
	})
	s := verifyService(repo, codes, &fakeMailer{}, now)

	if err := s.ResetPassword(context.Background(), "user@example.test", "000000", "brand-new-pw"); !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("err = %v, want ErrInvalidCode", err)
	}
	if repo.resetHash != "" {
		t.Error("a wrong code must not change the stored password")
	}
}

func TestResetPasswordEnforcesPasswordRules(t *testing.T) {
	repo := &fakeRepo{userByEmailResults: []userByEmailResult{
		{user: User{ID: 7}, passwordHash: "hashed:old", hasPassword: true},
	}}
	codes := newFakeCodes()
	now := time.Now()
	codes.put(7, PurposeResetPassword, StoredCode{
		Hash: "hashed:123456", ExpiresAt: now.Add(time.Minute), IssuedAt: now,
	})
	s := verifyService(repo, codes, &fakeMailer{}, now)

	if err := s.ResetPassword(context.Background(), "user@example.test", "123456", "short"); !errors.Is(err, ErrPasswordTooShort) {
		t.Errorf("err = %v, want ErrPasswordTooShort", err)
	}
	if repo.resetHash != "" {
		t.Error("a rejected password must not be stored")
	}
}

func TestChangePasswordRequiresTheCurrentOne(t *testing.T) {
	repo := &fakeRepo{userByEmailResults: []userByEmailResult{
		{passwordHash: "hashed:current", hasPassword: true},
	}}
	s := recoveryService(repo, newFakeCodes(), &fakeMailer{})

	if _, err := s.ChangePassword(context.Background(), 7, "not-the-current-one", "brand-new-pw"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("err = %v, want ErrInvalidCredentials", err)
	}
	if repo.setHash != "" {
		t.Error("a wrong current password must not change anything")
	}
}

func TestChangePasswordStoresTheNewHashAndReturnsTheNewGeneration(t *testing.T) {
	repo := &fakeRepo{
		userByEmailResults: []userByEmailResult{
			{passwordHash: "hashed:current", hasPassword: true},
		},
		setPasswordVersion: 9,
	}
	s := recoveryService(repo, newFakeCodes(), &fakeMailer{})

	version, err := s.ChangePassword(context.Background(), 7, "current", "brand-new-pw")
	if err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}
	if repo.setHash != "hashed:brand-new-pw" {
		t.Errorf("stored hash = %q, want the new password's hash", repo.setHash)
	}
	if version != 9 {
		t.Errorf("returned generation = %d, want the repository's new value so the caller can re-issue its own cookie", version)
	}
}

// An account with no password must set one through the reset flow, which proves the
// address — not here, where there is no current password to check.
func TestChangePasswordRefusesAPasswordlessAccount(t *testing.T) {
	repo := &fakeRepo{userByEmailResults: []userByEmailResult{{hasPassword: false}}}
	s := recoveryService(repo, newFakeCodes(), &fakeMailer{})

	if _, err := s.ChangePassword(context.Background(), 7, "anything", "brand-new-pw"); !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("err = %v, want ErrInvalidCredentials", err)
	}
}

func TestResetPassword_ConcurrentRace(t *testing.T) {
	repo := &fakeRepo{
		userByEmailResults: []userByEmailResult{
			{user: User{ID: 7, Email: "user@example.test"}, passwordHash: "hashed:old", hasPassword: true},
		},
	}
	codes, mailer := newFakeCodes(), &fakeMailer{}
	s := recoveryService(repo, codes, mailer)

	if err := s.RequestPasswordReset(context.Background(), "user@example.test"); err != nil {
		t.Fatalf("RequestPasswordReset: %v", err)
	}

	if len(mailer.reset) != 1 {
		t.Fatalf("len(mailer.reset) = %d, want 1", len(mailer.reset))
	}
	code := mailer.reset[0]
	var successes atomic.Int32
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			newPassword := fmt.Sprintf("new-password-%d", idx)
			if err := s.ResetPassword(context.Background(), "user@example.test", code, newPassword); err == nil {
				successes.Add(1)
			}
		}(i)
	}
	wg.Wait()

	if successes.Load() != 1 {
		t.Errorf("successes = %d, want exactly 1 successful code consumption under concurrent calls", successes.Load())
	}
}
