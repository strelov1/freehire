package accounts

import (
	"context"
	"errors"
	"fmt"
)

// RequestPasswordReset mails a reset code if the address has an account. It returns nil
// for every well-formed address so the endpoint cannot enumerate accounts; the caller
// answers 202 either way.
//
// An OAuth-only account is served too: the code proves the same control of the address
// the provider does. Errors are only mail unconfigured and the resend cooldown, neither
// of which leaks whether the account exists.
func (s *Service) RequestPasswordReset(ctx context.Context, email string) error {
	if !s.codesEnabled() {
		return ErrMailUnavailable
	}
	addr, err := normalizeEmail(email)
	if err != nil {
		return nil // a malformed address has no account either; stay silent
	}
	user, _, _, err := s.repo.UserByEmail(ctx, addr)
	if errors.Is(err, ErrUserNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if user.Banned {
		return nil // a banned account gets no code, and the caller cannot tell
	}

	code, err := s.issueCode(ctx, user.ID, PurposeResetPassword, "")
	if err != nil {
		return err
	}
	return s.mailer.SendPasswordResetCode(ctx, user.Email, code)
}

// ResetPassword sets a password when the code matches the outstanding one for the
// address. It also marks the account verified and revokes every session.
func (s *Service) ResetPassword(ctx context.Context, email, code, newPassword string) error {
	if !s.codesEnabled() {
		return ErrMailUnavailable
	}
	addr, err := normalizeEmail(email)
	if err != nil {
		return ErrInvalidCode // no account, so no code can match
	}
	user, _, _, err := s.repo.UserByEmail(ctx, addr)
	if errors.Is(err, ErrUserNotFound) {
		return ErrInvalidCode
	}
	if err != nil {
		return err
	}
	if user.Banned {
		return ErrInvalidCode // no code was sent, so none can match
	}
	if err := validatePassword(newPassword); err != nil {
		// Checked before the code is consumed so a rejected password does not burn it.
		return err
	}

	// Hash before opening the transaction so bcrypt does not hold DB locks.
	hash, err := s.hasher.Hash(newPassword)
	if err != nil {
		return err
	}

	tx, err := s.codes.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	txStore := s.codes.WithTx(tx)
	txRepo := s.repo.WithTx(tx)

	_, consumeErr := s.consumeCodeTx(ctx, txStore, user.ID, PurposeResetPassword, code)
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

	if _, err := txRepo.ResetPassword(ctx, user.ID, hash); err != nil {
		return err
	}

	// A reset is the moment of suspicion: delete every MCP OAuth grant too, since the
	// session-generation bump alone does not revoke them.
	if err := txRepo.DeleteOAuthGrantsByUser(ctx, user.ID); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// ChangePassword replaces a known password and returns the new session generation so the
// caller can re-issue its own cookie (every session is revoked). A passwordless account
// has nothing to verify against and must use the reset flow.
func (s *Service) ChangePassword(ctx context.Context, userID int64, current, next string) (int32, error) {
	hash, hasPassword, err := s.repo.PasswordHash(ctx, userID)
	if err != nil {
		return 0, err
	}
	if !hasPassword || s.hasher.Check(hash, current) != nil {
		return 0, ErrInvalidCredentials
	}
	if err := validatePassword(next); err != nil {
		return 0, err
	}
	newHash, err := s.hasher.Hash(next)
	if err != nil {
		return 0, err
	}
	return s.repo.SetPassword(ctx, userID, newHash)
}

// validatePassword is the one rule set shared by registration, reset and change.
func validatePassword(password string) error {
	switch {
	case len(password) < minPasswordLen:
		return ErrPasswordTooShort
	case len(password) > maxPasswordLen:
		return ErrPasswordTooLong
	}
	return nil
}
