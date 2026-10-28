package accounts

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
)

// RequestEmailChange mails a code to newEmail that, once presented to ConfirmEmailChange,
// moves the account there. Nothing about the account changes here: an address is a
// sign-in credential, so it is switched only after its new holder proves they read it.
//
// The caller has already established that the session belongs to the account's owner
// (the recent-auth gate on the route) — this method proves only the NEW address.
//
// Refuses an address another account holds with ErrEmailTaken. That tells a signed-in,
// freshly re-authenticated caller whether an address is registered, which registration
// already answers to anyone; the alternative — mailing a code that can never be applied —
// would leave the member waiting on a change that is impossible.
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

// ConfirmEmailChange moves the account to the address the outstanding change code was
// mailed to, and returns the account's new session generation so the caller can re-issue
// its own cookie: the change revokes every session, like a password change.
//
// The address applied is the one stored with the code, never one the caller supplies, so
// a code proves exactly the address it reached.
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
			// Same reasoning as ConfirmVerification: the failed guess must be kept.
			if err := tx.Commit(ctx); err != nil {
				return 0, fmt.Errorf("commit the failed attempt: %w", err)
			}
		}
		return 0, consumeErr
	}
	if stored.Target == "" {
		// A change code is only ever issued with its address; one without is not ours to
		// guess at.
		return 0, ErrInvalidCode
	}

	version, err := s.repo.WithTx(tx).ChangeEmail(ctx, userID, stored.Target)
	if err != nil {
		// ErrEmailTaken here means another account claimed the address after the code was
		// mailed. The rollback keeps the code, but it can never apply; the member asks for
		// a code to a different address.
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}

	// Best-effort: the change already happened, and a mail outage must not report it as
	// failed. Only an address that was ever proven is told — an unverified one may belong
	// to somebody who never had this account.
	if before.EmailVerified {
		if err := s.mailer.SendEmailChangedNotice(ctx, before.Email); err != nil {
			log.Printf("accounts: email-changed notice for user=%d: %v", userID, err)
		}
	}
	return version, nil
}
