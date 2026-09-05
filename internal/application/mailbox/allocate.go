// Package mailbox composes a per-user hosted-mailbox address on the freehire
// receiving domain (<username>@<domain>). It no longer allocates or suffixes
// its own handle — the address is always the account's own username (see the
// add-username-claim change's design.md), lazily allocated on first use via
// UsernameEnsurer. Enrollment (the mailboxes row) is separate from the
// username itself: releasing a mailbox stops mail delivery without touching
// the account's username.
package mailbox

import "context"

// UsernameEnsurer resolves (lazily allocating a default if the caller does
// not already have one) an account's username. Satisfied by
// *accounts.Service.
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
// feature on first use. Resolves the username BEFORE enrolling: the two are
// separate, non-transactional writes, and this order means a failure between
// them leaves an account with a username but no mailbox row — the ordinary
// state of every non-mailbox user — rather than a mailbox row with no
// username, which would be an anomaly every reader of that row has to
// specifically account for.
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
