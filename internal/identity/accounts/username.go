package accounts

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/strelov1/freehire/internal/identity/username"
)

// usernameCooldown bounds explicit username changes. It reads only a non-nil
// username_updated_at, so a lazily allocated default never starts the clock.
const usernameCooldown = 30 * 24 * time.Hour

// maxUsernameAttempts bounds EnsureUsername's collision-suffix search.
const maxUsernameAttempts = 100

var (
	// ErrUsernameTaken means another account holds the name, or (from the repository) a
	// concurrent claim collided and the caller must resolve it.
	ErrUsernameTaken = errors.New("accounts: username taken")

	// ErrUsernameReserved means the name is on the reserved list.
	ErrUsernameReserved = errors.New("accounts: username reserved")

	// ErrUsernameInvalid means the name fails format validation.
	ErrUsernameInvalid = errors.New("accounts: invalid username")

	// ErrUsernameCooldown means the account changed its username less than usernameCooldown ago.
	ErrUsernameCooldown = errors.New("accounts: username changed too recently")
)

// EnsureUsername returns the account's username, lazily allocating a default derived from
// email. Idempotent. The default leaves username_updated_at unset, so it never starts
// the ClaimUsername cooldown.
func (s *Service) EnsureUsername(ctx context.Context, userID int64, email string) (string, error) {
	return s.EnsureUsernameFromBase(ctx, userID, username.Suggest(email))
}

// EnsureUsernameFromBase is EnsureUsername seeded from an arbitrary base instead of an
// email (used by cmd/backfill-username-from-mailbox). base is sanitized internally.
func (s *Service) EnsureUsernameFromBase(ctx context.Context, userID int64, base string) (string, error) {
	if name, _, ok, err := s.repo.UsernameByUser(ctx, userID); err != nil {
		return "", err
	} else if ok {
		return name, nil
	}

	base = username.Sanitize(base)
	for n := 1; n <= maxUsernameAttempts; n++ {
		cand := username.Candidate(base, n)
		if username.IsReserved(cand) {
			continue
		}
		err := s.repo.SetUsernameIfAbsent(ctx, userID, cand)
		if err == nil {
			return cand, nil
		}
		if !errors.Is(err, ErrUsernameTaken) {
			return "", err
		}
		// A concurrent call won, or another account holds cand; re-reading resolves both.
		name, _, ok, gerr := s.repo.UsernameByUser(ctx, userID)
		if gerr != nil {
			return "", gerr
		}
		if ok {
			return name, nil
		}
	}
	return "", fmt.Errorf("accounts: no free username for %q after %d attempts", base, maxUsernameAttempts)
}

// ClaimUsername sets the username to desired, replacing the current one. A taken,
// reserved or invalid name is rejected, never suffixed. Rate-limited to one change per
// usernameCooldown against a prior explicit claim only; the first claim is never limited.
func (s *Service) ClaimUsername(ctx context.Context, userID int64, desired string) error {
	if err := username.Valid(desired); err != nil {
		return ErrUsernameInvalid
	}
	if username.IsReserved(desired) {
		return ErrUsernameReserved
	}

	_, updatedAt, _, err := s.repo.UsernameByUser(ctx, userID)
	if err != nil {
		return err
	}
	if updatedAt != nil && s.now().Sub(*updatedAt) < usernameCooldown {
		return ErrUsernameCooldown
	}

	return s.repo.SetUsername(ctx, userID, desired)
}

// Username returns the username (ok=false if none) and the time of its last explicit
// change, without allocating anything.
func (s *Service) Username(ctx context.Context, userID int64) (string, *time.Time, bool, error) {
	return s.repo.UsernameByUser(ctx, userID)
}

// UsernameAvailable reports whether desired is valid, unreserved and unheld. No write.
func (s *Service) UsernameAvailable(ctx context.Context, desired string) (bool, error) {
	if username.Valid(desired) != nil || username.IsReserved(desired) {
		return false, nil
	}
	taken, err := s.repo.UsernameTaken(ctx, desired)
	if err != nil {
		return false, err
	}
	return !taken, nil
}
