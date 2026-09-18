package accounts

import (
	"context"
	"errors"
	"testing"
	"time"
)

const testPurpose = "verify_work_email"

// TestIssueCodeMailsViaTheSuppliedSendFunc pins the whole point of the generic method: it
// mints and stores a code exactly like IssueVerificationCode, but delivers it through
// whatever send func the caller supplies rather than a hardcoded CodeMailer method — so a
// caller outside this package's own purposes can reuse the same rate-limited, attempt-bounded
// storage without this package knowing anything about its mail copy.
func TestIssueCodeMailsViaTheSuppliedSendFunc(t *testing.T) {
	codes := newFakeCodes()
	s := verifyService(&fakeRepo{}, codes, &fakeMailer{}, time.Now())

	var sentEmail, sentCode string
	send := func(_ context.Context, email, code string) error {
		sentEmail, sentCode = email, code
		return nil
	}

	if err := s.IssueCode(context.Background(), 7, testPurpose, "hr@acme.test", send); err != nil {
		t.Fatalf("IssueCode: %v", err)
	}
	if sentEmail != "hr@acme.test" {
		t.Errorf("sent to %q, want hr@acme.test", sentEmail)
	}
	if len(sentCode) != 6 {
		t.Errorf("code = %q, want six digits", sentCode)
	}
	if codes.upserts != 1 {
		t.Errorf("stored %d codes, want 1", codes.upserts)
	}
}

// A purpose is only useful as a namespace if the resend cooldown is enforced per purpose,
// the same way it already is for the two existing purposes.
func TestIssueCodeAppliesTheResendCooldown(t *testing.T) {
	codes := newFakeCodes()
	now := time.Now()
	codes.put(7, testPurpose, StoredCode{
		Hash: "hashed:123456", ExpiresAt: now.Add(time.Minute), IssuedAt: now.Add(-10 * time.Second),
	})
	s := verifyService(&fakeRepo{}, codes, &fakeMailer{}, now)

	sent := false
	send := func(_ context.Context, _, _ string) error { sent = true; return nil }

	if err := s.IssueCode(context.Background(), 7, testPurpose, "hr@acme.test", send); !errors.Is(err, ErrResendTooSoon) {
		t.Fatalf("err = %v, want ErrResendTooSoon", err)
	}
	if sent {
		t.Error("a throttled resend must mail nothing")
	}
}

// If the send func itself fails (a transport outage), the minted code must not be silently
// lost: the caller gets the error back and can retry, but the stored code is still there
// since issuing the code and sending it are not one guarded transaction.
func TestIssueCodeSurfacesASendFailure(t *testing.T) {
	codes := newFakeCodes()
	s := verifyService(&fakeRepo{}, codes, &fakeMailer{}, time.Now())
	wantErr := errors.New("smtp down")
	send := func(_ context.Context, _, _ string) error { return wantErr }

	if err := s.IssueCode(context.Background(), 7, testPurpose, "hr@acme.test", send); !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want %v", err, wantErr)
	}
}

func TestIssueCodeNeedsTheCodeStoreConfigured(t *testing.T) {
	s := New(&fakeRepo{}, &fakeHasher{}) // no WithCodes at all
	send := func(_ context.Context, _, _ string) error { return nil }

	if err := s.IssueCode(context.Background(), 7, testPurpose, "hr@acme.test", send); !errors.Is(err, ErrMailUnavailable) {
		t.Errorf("err = %v, want ErrMailUnavailable", err)
	}
}

// ConfirmCode is the side-effect-free sibling of ConfirmVerification: it must consume a
// correct code exactly like ConfirmVerification does, but must NOT touch the repository —
// unlike ConfirmVerification's MarkEmailVerified, "what verified means" belongs to the
// caller's own purpose, not to this package.
func TestConfirmCodeAcceptsTheIssuedCodeWithNoRepositorySideEffect(t *testing.T) {
	repo := &fakeRepo{}
	codes, mailer := newFakeCodes(), &fakeMailer{}
	now := time.Now()
	s := verifyService(repo, codes, mailer, now)

	var sentCode string
	send := func(_ context.Context, _, code string) error { sentCode = code; return nil }
	if err := s.IssueCode(context.Background(), 7, testPurpose, "hr@acme.test", send); err != nil {
		t.Fatalf("IssueCode: %v", err)
	}

	if err := s.ConfirmCode(context.Background(), 7, testPurpose, sentCode); err != nil {
		t.Fatalf("ConfirmCode: %v", err)
	}
	if repo.markedVerified {
		t.Error("ConfirmCode must not mark the account's own email verified — that is ConfirmVerification's job, not this generic primitive's")
	}
	if _, err := codes.Code(context.Background(), 7, testPurpose); !errors.Is(err, ErrNoCode) {
		t.Error("a consumed code must be gone, so it cannot be replayed")
	}
}

func TestConfirmCodeRejectsAWrongCode(t *testing.T) {
	codes := newFakeCodes()
	now := time.Now()
	codes.put(7, testPurpose, StoredCode{
		Hash: "hashed:123456", ExpiresAt: now.Add(time.Minute), IssuedAt: now,
	})
	s := verifyService(&fakeRepo{}, codes, &fakeMailer{}, now)

	if err := s.ConfirmCode(context.Background(), 7, testPurpose, "999999"); !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("err = %v, want ErrInvalidCode", err)
	}
	c, err := codes.Code(context.Background(), 7, testPurpose)
	if err != nil {
		t.Fatalf("Code: %v", err)
	}
	if c.Attempts != 1 {
		t.Errorf("attempts = %d, want 1 — a wrong guess must still count against the limit", c.Attempts)
	}
}

func TestConfirmCodeNeedsTheCodeStoreConfigured(t *testing.T) {
	s := New(&fakeRepo{}, &fakeHasher{}) // no WithCodes at all

	if err := s.ConfirmCode(context.Background(), 7, testPurpose, "123456"); !errors.Is(err, ErrMailUnavailable) {
		t.Errorf("err = %v, want ErrMailUnavailable", err)
	}
}

// Two different purposes for the same user must not collide — the same guarantee
// PurposeVerifyEmail/PurposeResetPassword already have, extended to an arbitrary
// caller-chosen purpose (accounts.PurposeVerifyWorkEmail, used by internal/ingest/employer).
func TestPurposeVerifyWorkEmailDoesNotCollideWithAccountVerification(t *testing.T) {
	repo := &fakeRepo{createUserResults: []createUserResult{{user: User{ID: 7, Email: "hr@acme.test"}}}}
	codes, mailer := newFakeCodes(), &fakeMailer{}
	now := time.Now()
	s := verifyService(repo, codes, mailer, now)

	if _, err := s.Register(context.Background(), "hr@acme.test", "password123", nil); err != nil {
		t.Fatalf("Register: %v", err)
	}
	accountCode := mailer.verification[0]

	var workEmailCode string
	send := func(_ context.Context, _, code string) error { workEmailCode = code; return nil }
	if err := s.IssueCode(context.Background(), 7, PurposeVerifyWorkEmail, "founder@acme.test", send); err != nil {
		t.Fatalf("IssueCode: %v", err)
	}

	// Confirming the work-email code must not consume or otherwise disturb the still-pending
	// account-verification code.
	if err := s.ConfirmCode(context.Background(), 7, PurposeVerifyWorkEmail, workEmailCode); err != nil {
		t.Fatalf("ConfirmCode: %v", err)
	}
	if err := s.ConfirmVerification(context.Background(), 7, accountCode); err != nil {
		t.Fatalf("ConfirmVerification: %v — the account-verification code must still be live", err)
	}
}
