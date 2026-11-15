// Package mailbox composes a per-user hosted-mailbox address on the freehire
// receiving domain (<username>@<domain>). The address is always the account's own
// username, lazily allocated via UsernameEnsurer. Enrollment (the mailboxes row) is
// separate: releasing a mailbox stops delivery without touching the username.
package mailbox

import "context"

// UsernameEnsurer resolves an account's username, allocating a default if it has
// none. Satisfied by *accounts.Service.
type UsernameEnsurer interface {
	EnsureUsername(ctx context.Context, userID int64, email string) (string, error)
}

// Store is the persistence GetOrCreate needs beyond the account's username,
// kept db-free so it is faked in tests.
type Store interface {
	// EnsureRow marks userID as enrolled in the hosted mailbox. Idempotent — a
	// second call for the same user is a no-op.
	EnsureRow(ctx context.Context, userID int64) error
}

// GetOrCreate returns the user's hosted-mailbox address, enrolling them in the
// feature on first use. Resolves the username BEFORE enrolling: the writes are not
// transactional, and a failure between them must leave a username without a mailbox
// row (an ordinary state), not a mailbox row without a username.
func GetOrCreate(ctx context.Context, s Store, ensurer UsernameEnsurer, userID int64, email, domain string) (string, error) {
	name, err := ensurer.EnsureUsername(ctx, userID, email)
	if err != nil {
		return "", err
	}
	if err := s.EnsureRow(ctx, userID); err != nil {
		return "", err
	}
	return name + "@" + domain, nil
}
