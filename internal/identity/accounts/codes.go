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

// Purposes a mailed code can serve. One store is keyed by (user, purpose), so pending
// codes of different purposes do not overwrite each other.
const (
	PurposeVerifyEmail   = "verify_email"
	PurposeResetPassword = "password_reset"
	// PurposeVerifyWorkEmail is the employer package's work-email claim: never the account's
	// own email, no MarkEmailVerified on confirm (see ConfirmCode).
	PurposeVerifyWorkEmail = "verify_work_email"
	// PurposeChangeEmail moves the account to a new address. Its code goes to an address not
	// yet held, so the row records it (StoredCode.Target) and confirm applies exactly it.
	PurposeChangeEmail = "change_email"
)

const (
	// codeTTL bounds how long a mailed code is usable.
	codeTTL = 15 * time.Minute

	// maxCodeAttempts is how many wrong guesses a six-digit code survives. It bounds
	// guessing only together with resendCooldown, which is why a burnt code keeps its row
	// (see consumeCodeTx).
	maxCodeAttempts = 5

	// resendCooldown stops the endpoint flooding an address with mail.
	resendCooldown = 60 * time.Second
)

var (
	// ErrNoCode means nothing is outstanding for that purpose.
	ErrNoCode = errors.New("accounts: no outstanding code")

	// ErrInvalidCode covers a wrong, consumed or burnt code, deliberately without saying which.
	ErrInvalidCode = errors.New("accounts: invalid code")

	// ErrCodeExpired is separate from ErrInvalidCode because the remedy is a new code.
	ErrCodeExpired = errors.New("accounts: code expired")

	// ErrResendTooSoon reports a resend inside the cooldown window.
	ErrResendTooSoon = errors.New("accounts: code was just sent")

	// ErrMailUnavailable reports no mail transport; surfaced as 503, never a silent success.
	ErrMailUnavailable = errors.New("accounts: email delivery is not configured")

	// ErrSameEmail reports an email change to the address the account already holds.
	ErrSameEmail = errors.New("accounts: that is already the account's email")

	// ErrAlreadyVerified reports a verification request for an already verified account.
	ErrAlreadyVerified = errors.New("accounts: email already verified")
)

// StoredCode is one outstanding code: the hash (never the code), expiry, wrong-guess
// count, and issue time (read by the resend cooldown).
type StoredCode struct {
	Hash      string
	ExpiresAt time.Time
	Attempts  int32
	IssuedAt  time.Time
	// Target is the address a PurposeChangeEmail code was mailed to; empty otherwise.
	Target string
}

// CodeStore persists at most one outstanding code per (user, purpose).
//
// Begin must return a real transaction: every code path reads the code FOR UPDATE and
// writes under that lock, which bounds guessing and makes "spend the code and reset the
// password" one act.
type CodeStore interface {
	UpsertCode(ctx context.Context, userID int64, purpose, codeHash, target string, expiresAt time.Time) error
	Code(ctx context.Context, userID int64, purpose string) (StoredCode, error)
	GetEmailCodeForUpdate(ctx context.Context, userID int64, purpose string) (StoredCode, error)
	BumpAttempts(ctx context.Context, userID int64, purpose string) (int32, error)
	DeleteCode(ctx context.Context, userID int64, purpose string) error
	WithTx(tx pgx.Tx) CodeStore
	Begin(ctx context.Context) (pgx.Tx, error)
}

// CodeMailer delivers the transactional account mails.
type CodeMailer interface {
	SendVerificationCode(ctx context.Context, email, code string) error
	SendPasswordResetCode(ctx context.Context, email, code string) error
	// SendEmailChangeCode mails the code that proves control of a NEW address.
	SendEmailChangeCode(ctx context.Context, email, code string) error
	// SendEmailChangedNotice tells the old address the account moved away; the one signal a
	// hijacked member would still get. It never names the new address.
	SendEmailChangedNotice(ctx context.Context, oldEmail string) error
}

// WithCodes enables the code-backed flows (verification, reset, email change).
func (s *Service) WithCodes(codes CodeStore, mailer CodeMailer) {
	s.codes = codes
	s.mailer = mailer
}

// codesEnabled reports whether both the store and a transport are wired.
func (s *Service) codesEnabled() bool { return s.codes != nil && s.mailer != nil }

// newCode returns a six-digit code from a secure source; safe only because codes are
// short-lived and attempt-limited.
func newCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

// issueCode mints and stores a fresh code, refusing a resend inside the cooldown; the
// check and upsert are one transaction. Only a hash is stored. target is set only for
// PurposeChangeEmail.
func (s *Service) issueCode(ctx context.Context, userID int64, purpose, target string) (string, error) {
	tx, err := s.codes.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()
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
	if err := store.UpsertCode(ctx, userID, purpose, hash, target, s.now().Add(codeTTL)); err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return code, nil
}

// consumeCodeTx checks a presented code against the outstanding one and returns it
// (for StoredCode.Target). A wrong guess bumps attempts; the caller must commit that.
func (s *Service) consumeCodeTx(ctx context.Context, store CodeStore, userID int64, purpose, presented string) (StoredCode, error) {
	stored, err := store.GetEmailCodeForUpdate(ctx, userID, purpose)
	if errors.Is(err, ErrNoCode) {
		return StoredCode{}, ErrInvalidCode
	}
	if err != nil {
		return StoredCode{}, err
	}
	if stored.Attempts >= maxCodeAttempts {
		return StoredCode{}, ErrInvalidCode
	}
	if s.now().After(stored.ExpiresAt) {
		return StoredCode{}, ErrCodeExpired
	}
	if s.hasher.Check(stored.Hash, presented) != nil {
		if _, err := store.BumpAttempts(ctx, userID, purpose); err != nil {
			return StoredCode{}, err
		}
		return StoredCode{}, ErrInvalidCode
	}
	return stored, store.DeleteCode(ctx, userID, purpose)
}

// IssueVerificationCode mails a fresh verification code. Reports ErrMailUnavailable
// without a transport and ErrResendTooSoon inside the cooldown.
func (s *Service) IssueVerificationCode(ctx context.Context, userID int64, email string) error {
	if !s.codesEnabled() {
		return ErrMailUnavailable
	}
	return s.IssueCode(ctx, userID, PurposeVerifyEmail, email, s.mailer.SendVerificationCode)
}

// IssueCode mints, stores and delivers a code for an arbitrary purpose through the
// caller's send func, so outside callers share the rate-limited, attempt-bounded store.
//
// Minting and sending are not one transaction: a send failure leaves the code stored, so
// a retry inside the cooldown is refused rather than minting a second code.
func (s *Service) IssueCode(ctx context.Context, userID int64, purpose, email string, send func(ctx context.Context, email, code string) error) error {
	if s.codes == nil {
		return ErrMailUnavailable
	}
	code, err := s.issueCode(ctx, userID, purpose, "")
	if err != nil {
		return err
	}
	return send(ctx, email, code)
}

// ConfirmCode checks and consumes the code outstanding for (userID, purpose), with
// ConfirmVerification's attempt and expiry rules but no side effect: what a confirmed
// code means is the caller's to decide.
func (s *Service) ConfirmCode(ctx context.Context, userID int64, purpose, code string) error {
	if s.codes == nil {
		return ErrMailUnavailable
	}
	tx, err := s.codes.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	txStore := s.codes.WithTx(tx)

	_, consumeErr := s.consumeCodeTx(ctx, txStore, userID, purpose, code)
	if consumeErr != nil {
		if errors.Is(consumeErr, ErrInvalidCode) {
			// Commit the attempt bump even though the confirmation is refused.
			if err := tx.Commit(ctx); err != nil {
				return fmt.Errorf("commit the failed attempt: %w", err)
			}
		}
		return consumeErr
	}
	return tx.Commit(ctx)
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
	defer func() { _ = tx.Rollback(ctx) }()
	txStore := s.codes.WithTx(tx)
	txRepo := s.repo.WithTx(tx)

	_, consumeErr := s.consumeCodeTx(ctx, txStore, userID, PurposeVerifyEmail, code)
	if consumeErr != nil {
		if errors.Is(consumeErr, ErrInvalidCode) {
			// Commit the attempt bump: rolling it back would make the attempt limit unreachable.
			// A failed commit must surface as our error; answering "wrong code" would give the
			// guesser a free attempt.
			if err := tx.Commit(ctx); err != nil {
				return fmt.Errorf("commit the failed attempt: %w", err)
			}
		}
		return consumeErr
	}

	if err := txRepo.MarkEmailVerified(ctx, userID); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// sendVerificationOnRegister mails the first code, best-effort: a mail failure is logged
// and must not undo the registration.
func (s *Service) sendVerificationOnRegister(ctx context.Context, userID int64, email string) {
	if !s.codesEnabled() {
		return
	}
	if err := s.IssueVerificationCode(ctx, userID, email); err != nil {
		log.Printf("accounts: verification mail for user=%d: %v", userID, err)
	}
}
