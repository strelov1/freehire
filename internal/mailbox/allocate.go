package mailbox

import (
	"context"
	"errors"
	"fmt"
)

// maxAllocAttempts bounds the collision-suffix search so a pathological run of
// taken handles fails loudly instead of looping forever.
const maxAllocAttempts = 100

// ErrTaken is returned by Store.Insert when the address (or the user's mailbox)
// already exists — the allocator resolves it by re-reading and, if needed,
// trying the next suffix.
var ErrTaken = errors.New("mailbox: taken")

// Store is the persistence the allocator needs, kept db-free so it is faked in
// tests. A db-backed adapter maps a Postgres unique violation (23505) to ErrTaken.
type Store interface {
	// AddressByUser returns the user's mailbox address, ok=false if none.
	AddressByUser(ctx context.Context, userID int64) (string, bool, error)
	// Insert claims address for userID, or ErrTaken on any unique collision.
	Insert(ctx context.Context, userID int64, address string) error
}

// GetOrCreate returns the user's mailbox address, allocating one on first use.
// The handle derives from the user's email; an address collision tries the next
// numeric suffix. It is safe under a race: a concurrent create that wins the
// unique user_id is resolved by re-reading the user's mailbox.
func GetOrCreate(ctx context.Context, s Store, userID int64, email, domain string) (string, error) {
	if addr, ok, err := s.AddressByUser(ctx, userID); err != nil {
		return "", err
	} else if ok {
		return addr, nil
	}

	base := Handle(email)
	for n := 1; n <= maxAllocAttempts; n++ {
		addr := Address(base, n, domain)
		err := s.Insert(ctx, userID, addr)
		if err == nil {
			return addr, nil
		}
		if !errors.Is(err, ErrTaken) {
			return "", err
		}
		// A collision is either the address (try the next suffix) or the user_id
		// — a concurrent allocation for this same user, whose row we then return.
		if existing, ok, gerr := s.AddressByUser(ctx, userID); gerr == nil && ok {
			return existing, nil
		}
	}
	return "", fmt.Errorf("mailbox: no free address for %q after %d attempts", base, maxAllocAttempts)
}
