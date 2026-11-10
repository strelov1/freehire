package accounts

import (
	"context"
	"errors"
	"testing"
)

// A banned account is refused at sign-in only after the password proves out, and is
// neither mailed a reset code nor able to spend one.
func TestBannedAccount(t *testing.T) {
	banned := userByEmailResult{user: User{ID: 7, Email: "b@example.test", Banned: true}, passwordHash: "hashed:password123", hasPassword: true}
	ctx := context.Background()

	t.Run("login with the right password is suspended", func(t *testing.T) {
		svc := New(&fakeRepo{userByEmailResults: []userByEmailResult{banned}}, &fakeHasher{})
		if _, err := svc.Login(ctx, "b@example.test", "password123"); !errors.Is(err, ErrAccountSuspended) {
			t.Errorf("err = %v, want ErrAccountSuspended", err)
		}
	})
	t.Run("login with a wrong password stays a generic failure", func(t *testing.T) {
		svc := New(&fakeRepo{userByEmailResults: []userByEmailResult{banned}}, &fakeHasher{})
		if _, err := svc.Login(ctx, "b@example.test", "wrong-password"); !errors.Is(err, ErrInvalidCredentials) {
			t.Errorf("err = %v, want ErrInvalidCredentials", err)
		}
	})
	t.Run("no reset code is mailed", func(t *testing.T) {
		mailer := &fakeMailer{}
		s := recoveryService(&fakeRepo{userByEmailResults: []userByEmailResult{banned}}, newFakeCodes(), mailer)
		if err := s.RequestPasswordReset(ctx, "b@example.test"); err != nil {
			t.Fatalf("err = %v, want nil so the caller cannot tell", err)
		}
		if len(mailer.reset) != 0 {
			t.Errorf("mailed %d reset codes to a banned account", len(mailer.reset))
		}
	})
	t.Run("a reset is refused as an invalid code", func(t *testing.T) {
		s := recoveryService(&fakeRepo{userByEmailResults: []userByEmailResult{banned}}, newFakeCodes(), &fakeMailer{})
		if err := s.ResetPassword(ctx, "b@example.test", "123456", "new-password-1"); !errors.Is(err, ErrInvalidCode) {
			t.Errorf("err = %v, want ErrInvalidCode", err)
		}
	})
}
