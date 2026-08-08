package accounts

import (
	"context"
	"errors"
)

// RequestPasswordReset mails a reset code for the address, if it has an account. It
// reports nil for every well-formed address — known or not — so the endpoint cannot be
// used to discover which addresses have accounts. The caller answers 202 either way.
//
// A passwordless (OAuth-only) account is served too, so its owner can set a password and
// gain a second way in. Receiving the code proves control of the address, which is the
// same proof the provider offers, so refusing would strand the user without buying safety.
//
// It reports an error only for conditions the caller itself must act on: mail being
// unconfigured, or the resend cooldown. Neither leaks whether the account exists, because
// the cooldown is only reachable by someone who already asked for this address.
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

	code, err := s.issueCode(ctx, user.ID, PurposeResetPassword)
	if err != nil {
		return err
	}
	return s.mailer.SendPasswordResetCode(ctx, user.Email, code)
}

// ResetPassword sets a password when the presented code matches the outstanding one for
// the address — replacing an existing one, or giving an OAuth-only account its first. A completed reset proves control of the address, so it also marks the
// account verified, and it revokes every existing session — a reset is exactly the moment
// a user suspects someone else is signed in.
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
	if err := validatePassword(newPassword); err != nil {
		// Checked before the code is consumed: a valid code must not be burnt because
		// the user typed a password the rules reject.
		return err
	}

	// Hash the new password BEFORE opening the transaction so CPU-heavy bcrypt work
	// does not hold DB locks.
	hash, err := s.hasher.Hash(newPassword)
	if err != nil {
		return err
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

	consumeErr := s.consumeCodeTx(ctx, txStore, user.ID, PurposeResetPassword, code)
	if consumeErr != nil {
		if tx != nil && errors.Is(consumeErr, ErrInvalidCode) {
			_ = tx.Commit(ctx)
		}
		return consumeErr
	}

	if _, err := txRepo.ResetPassword(ctx, user.ID, hash); err != nil {
		return err
	}

	if tx != nil {
		if err := tx.Commit(ctx); err != nil {
			return err
		}
	}
	return nil
}

// ChangePassword replaces a known password. It returns the account's new session
// generation so the caller can re-issue its own cookie: the change revokes every session,
// and the user who just typed their current password should not be signed out by it.
//
// An account with no password cannot use this — there is nothing to verify against. Those
// users go through the reset flow, which proves the address instead.
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

// validatePassword enforces the one rule set every password path shares, so registration,
// reset, and change cannot drift apart.
func validatePassword(password string) error {
	switch {
	case len(password) < minPasswordLen:
		return ErrPasswordTooShort
	case len(password) > maxPasswordLen:
		return ErrPasswordTooLong
	}
	return nil
}
