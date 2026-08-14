package accounts

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// fakeCodes is an in-memory CodeStore: one live code per (user, purpose), exactly like
// the composite-primary-key table it stands in for.
type fakeCodes struct {
	mu       sync.Mutex
	txMu     sync.Mutex
	codes    map[string]StoredCode
	upserts  int
	deleted  []string
	failNext error
}

type fakeTx struct {
	pgx.Tx
	f    *fakeCodes
	done bool
}

func (tx *fakeTx) Commit(ctx context.Context) error {
	if !tx.done {
		tx.done = true
		tx.f.txMu.Unlock()
	}
	return nil
}

func (tx *fakeTx) Rollback(ctx context.Context) error {
	if !tx.done {
		tx.done = true
		tx.f.txMu.Unlock()
	}
	return nil
}

func newFakeCodes() *fakeCodes { return &fakeCodes{codes: map[string]StoredCode{}} }

func codeKey(userID int64, purpose string) string { return fmt.Sprintf("%s:%d", purpose, userID) }

func (f *fakeCodes) WithTx(tx pgx.Tx) CodeStore {
	return f
}

func (f *fakeCodes) Begin(ctx context.Context) (pgx.Tx, error) {
	f.txMu.Lock()
	return &fakeTx{f: f}, nil
}

func (f *fakeCodes) UpsertCode(_ context.Context, userID int64, purpose, codeHash string, expiresAt time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
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
	f.mu.Lock()
	defer f.mu.Unlock()
	c, ok := f.codes[codeKey(userID, purpose)]
	if !ok {
		return StoredCode{}, ErrNoCode
	}
	return c, nil
}

func (f *fakeCodes) GetEmailCodeForUpdate(ctx context.Context, userID int64, purpose string) (StoredCode, error) {
	return f.Code(ctx, userID, purpose)
}

func (f *fakeCodes) BumpAttempts(_ context.Context, userID int64, purpose string) (int32, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
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
	f.mu.Lock()
	defer f.mu.Unlock()
	f.deleted = append(f.deleted, purpose)
	delete(f.codes, codeKey(userID, purpose))
	return nil
}

// put installs a code directly, so a test can age it or pre-load attempts without
// going through the issuing path.
func (f *fakeCodes) put(userID int64, purpose string, c StoredCode) {
	f.mu.Lock()
	defer f.mu.Unlock()
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

	if _, err := s.Register(context.Background(), "new@example.test", "password123", nil); err != nil {
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

	user, err := s.Register(context.Background(), "new@example.test", "password123", nil)
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

	if _, err := s.Register(context.Background(), "new@example.test", "password123", nil); err != nil {
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
	// Even the right code no longer works: the guesser must request a new one.
	if err := s.ConfirmVerification(context.Background(), 5, "123456"); !errors.Is(err, ErrInvalidCode) {
		t.Errorf("err = %v, want ErrInvalidCode after the code was burnt", err)
	}
	// The row survives on purpose — it carries the resend cooldown, so burning the code is
	// not a way to skip it. Only a successful consume deletes.
	c, err := codes.Code(context.Background(), 5, PurposeVerifyEmail)
	if err != nil {
		t.Fatalf("a burnt code must keep its row: %v", err)
	}
	if c.Attempts < maxCodeAttempts {
		t.Errorf("attempts = %d, want the ceiling recorded on the row", c.Attempts)
	}
	if len(codes.deleted) != 0 {
		t.Errorf("deleted %v, want nothing deleted by a burn", codes.deleted)
	}
}

// The attempt ceiling is only a real bound if it cannot be reset on demand. Burning a code
// must therefore leave the cooldown standing: otherwise the ceiling costs a guesser one
// round-trip instead of a minute, and five guesses per code becomes an unbounded rate
// against a six-digit secret.
func TestBurningACodeDoesNotLiftTheResendCooldown(t *testing.T) {
	codes := newFakeCodes()
	now := time.Now()
	codes.put(5, PurposeVerifyEmail, StoredCode{
		Hash: "hashed:123456", ExpiresAt: now.Add(codeTTL), IssuedAt: now, Attempts: maxCodeAttempts - 1,
	})
	mailer := &fakeMailer{}
	s := verifyService(&fakeRepo{}, codes, mailer, now)

	// Spend the last attempt; the code is dead from here on.
	if err := s.ConfirmVerification(context.Background(), 5, "999999"); !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("err = %v, want ErrInvalidCode", err)
	}
	if err := s.IssueVerificationCode(context.Background(), 5, "new@example.test"); !errors.Is(err, ErrResendTooSoon) {
		t.Errorf("err = %v, want ErrResendTooSoon — burning a code must not reset the cooldown", err)
	}
	if len(mailer.verification) != 0 {
		t.Errorf("mailed %d codes inside the cooldown, want 0", len(mailer.verification))
	}
}

// Once the cooldown has passed, a fresh code is issued normally and starts with a clean
// attempt budget — the burnt row must not outlive its own cooldown.
func TestABurntCodeIsReplacedAfterTheCooldown(t *testing.T) {
	codes := newFakeCodes()
	now := time.Now()
	codes.put(5, PurposeVerifyEmail, StoredCode{
		Hash:      "hashed:123456",
		ExpiresAt: now.Add(codeTTL),
		IssuedAt:  now.Add(-2 * resendCooldown),
		Attempts:  maxCodeAttempts,
	})
	mailer := &fakeMailer{}
	s := verifyService(&fakeRepo{}, codes, mailer, now)

	if err := s.IssueVerificationCode(context.Background(), 5, "new@example.test"); err != nil {
		t.Fatalf("IssueVerificationCode: %v", err)
	}
	if len(mailer.verification) != 1 {
		t.Fatalf("mailed %d codes, want 1", len(mailer.verification))
	}
	c, err := codes.Code(context.Background(), 5, PurposeVerifyEmail)
	if err != nil {
		t.Fatalf("Code: %v", err)
	}
	if c.Attempts != 0 {
		t.Errorf("attempts = %d, want 0 — a re-issued code starts with a fresh budget", c.Attempts)
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

	if _, err := s.Register(context.Background(), "new@example.test", "password123", nil); err != nil {
		t.Errorf("Register must work with mail unconfigured: %v", err)
	}
}

func TestCodeConsume_ConcurrentRace(t *testing.T) {
	repo := &fakeRepo{}
	codes := newFakeCodes()
	now := time.Now()
	codes.put(5, PurposeVerifyEmail, StoredCode{
		Hash:      "hashed:123456",
		ExpiresAt: now.Add(time.Minute),
		IssuedAt:  now,
	})
	s := verifyService(repo, codes, &fakeMailer{}, now)

	const goroutines = 10
	var wg sync.WaitGroup
	var successes int32
	var failures int32

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			err := s.ConfirmVerification(context.Background(), 5, "123456")
			if err == nil {
				atomic.AddInt32(&successes, 1)
			} else if errors.Is(err, ErrInvalidCode) {
				atomic.AddInt32(&failures, 1)
			} else {
				t.Errorf("unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()

	if successes != 1 {
		t.Errorf("successes = %d, want 1", successes)
	}
	if failures != goroutines-1 {
		t.Errorf("failures = %d, want %d", failures, goroutines-1)
	}
}

func TestCodeConsume_ConcurrentGuessRace(t *testing.T) {
	repo := &fakeRepo{}
	codes := newFakeCodes()
	now := time.Now()
	codes.put(5, PurposeVerifyEmail, StoredCode{
		Hash:      "hashed:123456",
		ExpiresAt: now.Add(time.Minute),
		IssuedAt:  now,
	})
	s := verifyService(repo, codes, &fakeMailer{}, now)

	const goroutines = 10
	var wg sync.WaitGroup
	var failures int32

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(id int) {
			defer wg.Done()
			wrongCode := fmt.Sprintf("%06d", id+900000)
			err := s.ConfirmVerification(context.Background(), 5, wrongCode)
			if errors.Is(err, ErrInvalidCode) {
				atomic.AddInt32(&failures, 1)
			} else {
				t.Errorf("goroutine %d: unexpected error %v", id, err)
			}
		}(i)
	}
	wg.Wait()

	if failures != goroutines {
		t.Errorf("failures = %d, want %d", failures, goroutines)
	}

	c, err := codes.Code(context.Background(), 5, PurposeVerifyEmail)
	if err != nil {
		t.Fatalf("Code: %v", err)
	}
	if c.Attempts < maxCodeAttempts {
		t.Errorf("attempts = %d, want at least %d", c.Attempts, maxCodeAttempts)
	}

	// Attempting with the correct code must now fail as the code is burnt.
	if err := s.ConfirmVerification(context.Background(), 5, "123456"); !errors.Is(err, ErrInvalidCode) {
		t.Errorf("err = %v, want ErrInvalidCode for burnt code", err)
	}
}
