package accounts

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log"
	"math/big"
	"time"

	"github.com/jackc/pgx/v5"
)

// Purposes a mailed code can serve. They share one store keyed by (user, purpose), so a
// pending verification and a pending reset never overwrite each other.
const (
	PurposeVerifyEmail   = "verify_email"
	PurposeResetPassword = "password_reset"
)

const (
	// codeTTL bounds how long a mailed code is usable. Long enough to switch to a mail
	// client and back, short enough that a code sitting in an inbox is not a standing key.
	codeTTL = 15 * time.Minute

	// maxCodeAttempts is how many wrong guesses a code survives. Six digits is a million
	// possibilities; five tries makes online guessing hopeless without frustrating a human
	// who mistyped. It bounds the guessing rate only together with resendCooldown, which is
	// why a burnt code keeps its row (see consumeCodeTx).
	maxCodeAttempts = 5

	// resendCooldown keeps the endpoint from being used to flood an address with mail.
	resendCooldown = 60 * time.Second
)

var (
	// ErrNoCode is returned by a CodeStore when nothing is outstanding for that purpose.
	ErrNoCode = errors.New("accounts: no outstanding code")

	// ErrInvalidCode covers a wrong, already-consumed, or burnt code. It deliberately does
	// not say which: a caller learns only that this code does not work.
	ErrInvalidCode = errors.New("accounts: invalid code")

	// ErrCodeExpired is separate from ErrInvalidCode because the remedy differs — the
	// caller should request a new code rather than re-read the mail.
	ErrCodeExpired = errors.New("accounts: code expired")

	// ErrResendTooSoon reports a resend inside the cooldown window.
	ErrResendTooSoon = errors.New("accounts: code was just sent")

	// ErrMailUnavailable reports that no outbound mail transport is configured, so no code
	// can be delivered. Surfaced as 503 — never as a silent success.
	ErrMailUnavailable = errors.New("accounts: email delivery is not configured")

	// ErrAlreadyVerified reports a verification request for an account that is already
	// verified, so the client can stop prompting.
	ErrAlreadyVerified = errors.New("accounts: email already verified")
)

// StoredCode is one outstanding code as the store holds it: the hash (never the code),
// when it dies, how many wrong guesses it has taken, and when it was issued (which is
// what the resend cooldown reads).
type StoredCode struct {
	Hash      string
	ExpiresAt time.Time
	Attempts  int32
	IssuedAt  time.Time
}

// CodeStore persists at most one outstanding code per (user, purpose).
type CodeStore interface {
	UpsertCode(ctx context.Context, userID int64, purpose, codeHash string, expiresAt time.Time) error
	Code(ctx context.Context, userID int64, purpose string) (StoredCode, error)
	GetEmailCodeForUpdate(ctx context.Context, userID int64, purpose string) (StoredCode, error)
	BumpAttempts(ctx context.Context, userID int64, purpose string) (int32, error)
	DeleteCode(ctx context.Context, userID int64, purpose string) error
	WithTx(tx pgx.Tx) CodeStore
	Begin(ctx context.Context) (pgx.Tx, error)
}

// CodeMailer delivers the two transactional mails. It is a port so accounts stays free of
// the AWS/SES dependency graph.
type CodeMailer interface {
	SendVerificationCode(ctx context.Context, email, code string) error
	SendPasswordResetCode(ctx context.Context, email, code string) error
}

// WithCodes enables the code-backed flows (email verification, password reset). Without
// it the Service still registers and authenticates; the code endpoints report the feature
// unavailable rather than pretending a mail was sent.
func (s *Service) WithCodes(codes CodeStore, mailer CodeMailer) {
	s.codes = codes
	s.mailer = mailer
}

// codesEnabled reports whether both the store and a transport are wired.
func (s *Service) codesEnabled() bool { return s.codes != nil && s.mailer != nil }

// newCode returns a six-digit code from a cryptographically secure source. Six digits is
// safe here only because the code is short-lived and attempt-limited — both enforced below.
func newCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// issueCode mints, stores, and returns a fresh code for one purpose, refusing a resend
// inside the cooldown. The code is stored only as a hash — a stolen snapshot must not
// yield live codes. It checks resend cooldown and upserts code atomically inside a transaction.
func (s *Service) issueCode(ctx context.Context, userID int64, purpose string) (string, error) {
	tx, err := s.codes.Begin(ctx)
	if err != nil {
		return "", err
	}
	if tx != nil {
		defer func() { _ = tx.Rollback(ctx) }()
	}
	store := s.codes.WithTx(tx)

	existing, err := store.GetEmailCodeForUpdate(ctx, userID, purpose)
	switch {
	case err == nil:
		if s.now().Sub(existing.IssuedAt) < resendCooldown {
			return "", ErrResendTooSoon
		}
	case !errors.Is(err, ErrNoCode):
		return "", err
	}

	code, err := newCode()
	if err != nil {
		return "", err
	}
	hash, err := s.hasher.Hash(code)
	if err != nil {
		return "", err
	}
	if err := store.UpsertCode(ctx, userID, purpose, hash, s.now().Add(codeTTL)); err != nil {
		return "", err
	}
	if tx != nil {
		if err := tx.Commit(ctx); err != nil {
			return "", err
		}
	}
	return code, nil
}

// consumeCodeTx checks a presented code against the outstanding one using GetEmailCodeForUpdate
// inside a transaction.
func (s *Service) consumeCodeTx(ctx context.Context, store CodeStore, userID int64, purpose, presented string) error {
	stored, err := store.GetEmailCodeForUpdate(ctx, userID, purpose)
	if errors.Is(err, ErrNoCode) {
		return ErrInvalidCode
	}
	if err != nil {
		return err
	}
	if stored.Attempts >= maxCodeAttempts {
		return ErrInvalidCode
	}
	if s.now().After(stored.ExpiresAt) {
		return ErrCodeExpired
	}
	if s.hasher.Check(stored.Hash, presented) != nil {
		if _, err := store.BumpAttempts(ctx, userID, purpose); err != nil {
			return err
		}
		return ErrInvalidCode
	}
	return store.DeleteCode(ctx, userID, purpose)
}

// IssueVerificationCode mails a fresh email-verification code to the account's address.
// Reports ErrMailUnavailable when no transport is configured and ErrResendTooSoon inside
// the cooldown.
func (s *Service) IssueVerificationCode(ctx context.Context, userID int64, email string) error {
	if !s.codesEnabled() {
		return ErrMailUnavailable
	}
	code, err := s.issueCode(ctx, userID, PurposeVerifyEmail)
	if err != nil {
		return err
	}
	return s.mailer.SendVerificationCode(ctx, email, code)
}

// ConfirmVerification marks the account verified when the presented code matches the
// outstanding one.
func (s *Service) ConfirmVerification(ctx context.Context, userID int64, code string) error {
	if !s.codesEnabled() {
		return ErrMailUnavailable
	}
	tx, err := s.codes.Begin(ctx)
	if err != nil {
		return err
	}
	if tx != nil {
		defer func() { _ = tx.Rollback(ctx) }()
	}
	txStore := s.codes.WithTx(tx)
	txRepo := s.repo.WithTx(tx)

	consumeErr := s.consumeCodeTx(ctx, txStore, userID, PurposeVerifyEmail, code)
	if consumeErr != nil {
		if tx != nil && errors.Is(consumeErr, ErrInvalidCode) {
			_ = tx.Commit(ctx)
		}
		return consumeErr
	}

	if err := txRepo.MarkEmailVerified(ctx, userID); err != nil {
		return err
	}

	if tx != nil {
		if err := tx.Commit(ctx); err != nil {
			return err
		}
	}
	return nil
}

// sendVerificationOnRegister mails the new account's first code, best-effort: a mail
// failure must not undo a registration that already succeeded, so it is logged and the
// user is left to ask for a resend.
func (s *Service) sendVerificationOnRegister(ctx context.Context, userID int64, email string) {
	if !s.codesEnabled() {
		return
	}
	if err := s.IssueVerificationCode(ctx, userID, email); err != nil {
		log.Printf("accounts: verification mail for user=%d: %v", userID, err)
	}
}
