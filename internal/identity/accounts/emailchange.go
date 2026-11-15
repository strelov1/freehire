package accounts

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
)

// RequestEmailChange mails a code to newEmail that ConfirmEmailChange applies. Nothing
// changes until the new holder proves they read it, since the address is a sign-in
// credential. The caller has already proven ownership of the session (recent-auth gate);
// this proves only the new address.
//
// An address another account holds is refused with ErrEmailTaken; mailing a code that can
// never apply would leave the member waiting.
func (s *Service) RequestEmailChange(ctx context.Context, userID int64, newEmail string) error {
	if !s.codesEnabled() {
		return ErrMailUnavailable
	}
	addr, err := normalizeEmail(newEmail)
	if err != nil {
		return ErrInvalidEmail
	}
	user, err := s.repo.UserByID(ctx, userID)
	if err != nil {
		return err
	}
	if strings.EqualFold(addr, user.Email) {
		return ErrSameEmail
	}
	switch _, _, _, err := s.repo.UserByEmail(ctx, addr); {
	case err == nil:
		return ErrEmailTaken
	case !errors.Is(err, ErrUserNotFound):
		return err
	}

	code, err := s.issueCode(ctx, userID, PurposeChangeEmail, addr)
	if err != nil {
		return err
	}
	return s.mailer.SendEmailChangeCode(ctx, addr, code)
}

// ConfirmEmailChange moves the account to the address the code was mailed to and returns
// the new session generation (every session is revoked, so the caller re-issues its
// cookie). The applied address is the one stored with the code, never a caller-supplied one.
func (s *Service) ConfirmEmailChange(ctx context.Context, userID int64, code string) (int32, error) {
	if !s.codesEnabled() {
		return 0, ErrMailUnavailable
	}
	before, err := s.repo.UserByID(ctx, userID)
	if err != nil {
		return 0, err
	}

	tx, err := s.codes.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	stored, consumeErr := s.consumeCodeTx(ctx, s.codes.WithTx(tx), userID, PurposeChangeEmail, code)
	if consumeErr != nil {
		if errors.Is(consumeErr, ErrInvalidCode) {
			// Commit the failed attempt, as ConfirmVerification does.
			if err := tx.Commit(ctx); err != nil {
				return 0, fmt.Errorf("commit the failed attempt: %w", err)
			}
		}
		return 0, consumeErr
	}
	if stored.Target == "" {
		// A change code is always issued with its address; one without is not ours.
		return 0, ErrInvalidCode
	}

	version, err := s.repo.WithTx(tx).ChangeEmail(ctx, userID, stored.Target)
	if err != nil {
		// ErrEmailTaken: another account claimed the address after the mail. The code stays
		// but can never apply.
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}

	// Best-effort: a mail outage must not report the change as failed. Only a proven old
	// address is told; an unverified one may belong to someone who never had this account.
	if before.EmailVerified {
		if err := s.mailer.SendEmailChangedNotice(ctx, before.Email); err != nil {
			log.Printf("accounts: email-changed notice for user=%d: %v", userID, err)
		}
	}
	return version, nil
}
