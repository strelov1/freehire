package accounts

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/strelov1/freehire/internal/identity/username"
)

// usernameCooldown bounds how often an account may explicitly change its
// username. It is evaluated only against a non-nil username_updated_at — a
// lazily allocated default never sets that field, so it never starts the
// clock (see the add-username-claim change's design.md, Decision 2).
const usernameCooldown = 30 * 24 * time.Hour

// maxUsernameAttempts bounds EnsureUsername's collision-suffix search so a
// pathological run of taken names fails loudly instead of looping forever.
const maxUsernameAttempts = 100

var (
	// ErrUsernameTaken is returned by ClaimUsername when the desired name is
	// already held by another account, and internally by the repository when a
	// collision (on the username itself, or a concurrent claim for the same
	// account) needs the caller to resolve it.
	ErrUsernameTaken = errors.New("accounts: username taken")

	// ErrUsernameReserved is returned by ClaimUsername when the desired name is
	// on the reserved list.
	ErrUsernameReserved = errors.New("accounts: username reserved")

	// ErrUsernameInvalid is returned by ClaimUsername when the desired name
	// fails format validation.
	ErrUsernameInvalid = errors.New("accounts: invalid username")

	// ErrUsernameCooldown is returned by ClaimUsername when the account changed
	// its username less than usernameCooldown ago.
	ErrUsernameCooldown = errors.New("accounts: username changed too recently")
)

// EnsureUsername returns the account's username, lazily allocating a default
// derived from email on first need. Idempotent: an account that already has a
// username is returned its existing one, never reallocated. The default is
// written without setting username_updated_at, so it never starts the
// cooldown ClaimUsername enforces.
func (s *Service) EnsureUsername(ctx context.Context, userID int64, email string) (string, error) {
	return s.EnsureUsernameFromBase(ctx, userID, username.Suggest(email))
}

// EnsureUsernameFromBase is EnsureUsername's allocator seeded from an
// arbitrary base instead of an email — used by
// cmd/backfill-username-from-mailbox to transform a legacy mailbox handle
// rather than deriving fresh from the account's email, which would not
// necessarily preserve the account's existing address. base is run through
// username.Sanitize internally, so a caller need not pre-sanitize it. Same
// idempotency, reserved-word skip, and collision-suffix behavior as
// EnsureUsername.
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
		// Either this account already has a username (a concurrent EnsureUsername
		// call won), or another account holds cand — re-reading resolves both.
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

// ClaimUsername sets the account's username to desired, replacing whatever it
// currently has. Unlike EnsureUsername, a taken, reserved, or invalid desired
// name is rejected outright — never substituted with a suffixed alternative.
// Rate-limited to one change per usernameCooldown, evaluated only against a
// prior EXPLICIT claim (username_updated_at non-nil); an account's first
// explicit claim is never rate-limited, even if a lazy default already exists.
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

// Username returns the account's username (ok=false if none) and, when set,
// the time of its last explicit change. A thin read-only passthrough to the
// repository, for a caller that wants the current value without allocating or
// changing anything (EnsureUsername and ClaimUsername respectively).
func (s *Service) Username(ctx context.Context, userID int64) (string, *time.Time, bool, error) {
	return s.repo.UsernameByUser(ctx, userID)
}

// UsernameAvailable reports whether desired could be claimed right now: valid
// format, not reserved, and not already held by any account. It performs no
// write, so a caller may probe a name before committing to ClaimUsername.
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
