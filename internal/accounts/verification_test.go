package accounts

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeCodes is an in-memory CodeStore: one live code per (user, purpose), exactly like
// the composite-primary-key table it stands in for.
type fakeCodes struct {
	codes    map[string]StoredCode
	upserts  int
	deleted  []string
	failNext error
}

func newFakeCodes() *fakeCodes { return &fakeCodes{codes: map[string]StoredCode{}} }

func codeKey(userID int64, purpose string) string { return purpose + ":" + string(rune(userID)) }

func (f *fakeCodes) UpsertCode(_ context.Context, userID int64, purpose, codeHash string, expiresAt time.Time) error {
	if f.failNext != nil {
		return f.failNext
	}
	f.upserts++
	f.codes[codeKey(userID, purpose)] = StoredCode{
		Hash: codeHash, ExpiresAt: expiresAt, Attempts: 0, IssuedAt: time.Now(),
	}
	return nil
}

func (f *fakeCodes) Code(_ context.Context, userID int64, purpose string) (StoredCode, error) {
	c, ok := f.codes[codeKey(userID, purpose)]
	if !ok {
		return StoredCode{}, ErrNoCode
	}
	return c, nil
}

func (f *fakeCodes) BumpAttempts(_ context.Context, userID int64, purpose string) (int32, error) {
	k := codeKey(userID, purpose)
	c, ok := f.codes[k]
	if !ok {
		return 0, ErrNoCode
	}
	c.Attempts++
	f.codes[k] = c
	return c.Attempts, nil
}

func (f *fakeCodes) DeleteCode(_ context.Context, userID int64, purpose string) error {
	f.deleted = append(f.deleted, purpose)
	delete(f.codes, codeKey(userID, purpose))
	return nil
}

// put installs a code directly, so a test can age it or pre-load attempts without
// going through the issuing path.
func (f *fakeCodes) put(userID int64, purpose string, c StoredCode) {
	f.codes[codeKey(userID, purpose)] = c
}

// fakeMailer records what was mailed. A non-nil err makes every send fail, standing in
// for an SES outage.
type fakeMailer struct {
	verification []string
	reset        []string
	err          error
}

func (m *fakeMailer) SendVerificationCode(_ context.Context, _, code string) error {
	if m.err != nil {
		return m.err
	}
	m.verification = append(m.verification, code)
	return nil
}

func (m *fakeMailer) SendPasswordResetCode(_ context.Context, _, code string) error {
	if m.err != nil {
		return m.err
	}
	m.reset = append(m.reset, code)
	return nil
}

// verifyService builds a Service wired to the given doubles, with a fixed clock so
// expiry and cooldown are deterministic.
func verifyService(repo Repository, codes CodeStore, mailer CodeMailer, now time.Time) *Service {
	s := New(repo, &fakeHasher{})
	s.WithCodes(codes, mailer)
	s.now = func() time.Time { return now }
	return s
}

func TestRegisterIssuesAndMailsAVerificationCode(t *testing.T) {
	repo := &fakeRepo{createUserResults: []createUserResult{{user: User{ID: 5, Email: "new@example.test"}}}}
	codes, mailer := newFakeCodes(), &fakeMailer{}
	s := verifyService(repo, codes, mailer, time.Now())

	if _, err := s.Register(context.Background(), "new@example.test", "password123"); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if len(mailer.verification) != 1 {
		t.Fatalf("mailed %d verification codes, want 1", len(mailer.verification))
	}
	if got := mailer.verification[0]; len(got) != 6 {
		t.Errorf("code = %q, want six digits", got)
	}
	if codes.upserts != 1 {
		t.Errorf("stored %d codes, want 1", codes.upserts)
	}
	if repo.createUserCalls[0].emailVerified {
		t.Error("a password registration must create the account unverified")
	}
}

func TestRegisterSurvivesAMailFailure(t *testing.T) {
	repo := &fakeRepo{createUserResults: []createUserResult{{user: User{ID: 5, Email: "new@example.test"}}}}
	s := verifyService(repo, newFakeCodes(), &fakeMailer{err: errors.New("ses down")}, time.Now())

	user, err := s.Register(context.Background(), "new@example.test", "password123")
	if err != nil {
		t.Fatalf("Register must not fail when the mail cannot be sent: %v", err)
	}
	if user.ID != 5 {
		t.Errorf("user id = %d, want the created account", user.ID)
	}
}

func TestConfirmVerificationAcceptsTheMailedCode(t *testing.T) {
	repo := &fakeRepo{createUserResults: []createUserResult{{user: User{ID: 5, Email: "new@example.test"}}}}
	codes, mailer := newFakeCodes(), &fakeMailer{}
	now := time.Now()
	s := verifyService(repo, codes, mailer, now)

	if _, err := s.Register(context.Background(), "new@example.test", "password123"); err != nil {
		t.Fatalf("Register: %v", err)
	}

	if err := s.ConfirmVerification(context.Background(), 5, mailer.verification[0]); err != nil {
		t.Fatalf("ConfirmVerification: %v", err)
	}
	if !repo.markedVerified {
		t.Error("the account was not marked verified")
	}
	if _, err := codes.Code(context.Background(), 5, PurposeVerifyEmail); !errors.Is(err, ErrNoCode) {
		t.Error("a consumed code must be gone, so it cannot be replayed")
	}
}

func TestConfirmVerificationRejectsAWrongCode(t *testing.T) {
	repo := &fakeRepo{}
	codes := newFakeCodes()
	now := time.Now()
	codes.put(5, PurposeVerifyEmail, StoredCode{
		Hash: "hashed:123456", ExpiresAt: now.Add(time.Minute), IssuedAt: now,
	})
	s := verifyService(repo, codes, &fakeMailer{}, now)

	if err := s.ConfirmVerification(context.Background(), 5, "999999"); !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("err = %v, want ErrInvalidCode", err)
	}
	if repo.markedVerified {
		t.Error("a wrong code must not verify the account")
	}
	c, err := codes.Code(context.Background(), 5, PurposeVerifyEmail)
	if err != nil {
		t.Fatalf("Code: %v", err)
	}
	if c.Attempts != 1 {
		t.Errorf("attempts = %d, want 1 — a wrong guess must count", c.Attempts)
	}
}

func TestConfirmVerificationBurnsTheCodeAfterTooManyAttempts(t *testing.T) {
	codes := newFakeCodes()
	now := time.Now()
	codes.put(5, PurposeVerifyEmail, StoredCode{
		Hash: "hashed:123456", ExpiresAt: now.Add(time.Minute), IssuedAt: now, Attempts: maxCodeAttempts - 1,
	})
	s := verifyService(&fakeRepo{}, codes, &fakeMailer{}, now)

	if err := s.ConfirmVerification(context.Background(), 5, "999999"); !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("err = %v, want ErrInvalidCode", err)
	}
	if _, err := codes.Code(context.Background(), 5, PurposeVerifyEmail); !errors.Is(err, ErrNoCode) {
		t.Error("the code must be burnt once the attempt ceiling is reached")
	}
	// Even the right code no longer works: the guesser must request a new one.
	if err := s.ConfirmVerification(context.Background(), 5, "123456"); !errors.Is(err, ErrInvalidCode) {
		t.Errorf("err = %v, want ErrInvalidCode after the code was burnt", err)
	}
}

func TestConfirmVerificationRejectsAnExpiredCode(t *testing.T) {
	codes := newFakeCodes()
	now := time.Now()
	codes.put(5, PurposeVerifyEmail, StoredCode{
		Hash: "hashed:123456", ExpiresAt: now.Add(-time.Second), IssuedAt: now.Add(-time.Hour),
	})
	s := verifyService(&fakeRepo{}, codes, &fakeMailer{}, now)

	if err := s.ConfirmVerification(context.Background(), 5, "123456"); !errors.Is(err, ErrCodeExpired) {
		t.Errorf("err = %v, want ErrCodeExpired so the client knows to ask for a new one", err)
	}
}

func TestResendIsThrottled(t *testing.T) {
	codes := newFakeCodes()
	now := time.Now()
	codes.put(5, PurposeVerifyEmail, StoredCode{
		Hash: "hashed:123456", ExpiresAt: now.Add(time.Minute), IssuedAt: now.Add(-10 * time.Second),
	})
	mailer := &fakeMailer{}
	s := verifyService(&fakeRepo{}, codes, mailer, now)

	if err := s.IssueVerificationCode(context.Background(), 5, "new@example.test"); !errors.Is(err, ErrResendTooSoon) {
		t.Fatalf("err = %v, want ErrResendTooSoon", err)
	}
	if len(mailer.verification) != 0 {
		t.Error("a throttled resend must mail nothing")
	}
}

func TestResendAfterTheCooldownIssuesAFreshCode(t *testing.T) {
	codes := newFakeCodes()
	now := time.Now()
	codes.put(5, PurposeVerifyEmail, StoredCode{
		Hash: "hashed:123456", ExpiresAt: now.Add(time.Minute), IssuedAt: now.Add(-2 * resendCooldown),
	})
	mailer := &fakeMailer{}
	s := verifyService(&fakeRepo{}, codes, mailer, now)

	if err := s.IssueVerificationCode(context.Background(), 5, "new@example.test"); err != nil {
		t.Fatalf("IssueVerificationCode: %v", err)
	}
	if len(mailer.verification) != 1 {
		t.Fatalf("mailed %d codes, want 1", len(mailer.verification))
	}
	if mailer.verification[0] == "123456" {
		t.Error("a resend must issue a new code, not re-send the old one")
	}
}

func TestVerificationNeedsAMailer(t *testing.T) {
	s := New(&fakeRepo{}, &fakeHasher{}) // no WithCodes: mail is unconfigured

	if err := s.IssueVerificationCode(context.Background(), 5, "new@example.test"); !errors.Is(err, ErrMailUnavailable) {
		t.Errorf("err = %v, want ErrMailUnavailable when no transport is configured", err)
	}
}

func TestRegisterWithoutAMailerStillCreatesTheAccount(t *testing.T) {
	repo := &fakeRepo{createUserResults: []createUserResult{{user: User{ID: 5}}}}
	s := New(repo, &fakeHasher{})

	if _, err := s.Register(context.Background(), "new@example.test", "password123"); err != nil {
		t.Errorf("Register must work with mail unconfigured: %v", err)
	}
}
