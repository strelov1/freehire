package worker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"
)

// The lock exists because a schedule pinned to clock times cannot stay correct while the
// catalogue grows. Measured 2026-09-15: a full rebuild started 03:15 and finished 07:47, and
// the next one started 15:15 and was still running at 18:57 — 4.5 hours against a schedule laid
// out when it took about three. The 06:45 suggestions build and the 16:30 and 19:30 dedup
// passes all land INSIDE that window, every single day. On 2026-09-15 the overlap took the
// site down: load average 701, 51 processes waiting on disk, the front page timing out.
func TestHoldHeavyIndexLock_ReportsWhenAnotherHolderHasIt(t *testing.T) {
	taken := &stubLocker{locked: false}

	release, ok, err := holdHeavyIndexLock(context.Background(), taken)

	if err != nil {
		t.Fatalf("a lock already held is an ordinary answer, not an error: %v", err)
	}
	if ok {
		t.Error("ok = true while another holder has the lock")
	}
	if release != nil {
		t.Error("release is non-nil for a lock we never took; calling it would unlock someone else's work")
	}
}

func TestHoldHeavyIndexLock_HandsBackAReleaseWhenItWins(t *testing.T) {
	free := &stubLocker{locked: true}

	release, ok, err := holdHeavyIndexLock(context.Background(), free)

	if err != nil || !ok {
		t.Fatalf("holdHeavyIndexLock = (_, %v, %v), want it to win a free lock", ok, err)
	}
	if release == nil {
		t.Fatal("won the lock with no way to release it")
	}
	release()
	if !free.released {
		t.Error("release did not unlock; the next run would find the lock held by a dead session")
	}
}

// A database that cannot answer is not a free lock. Reporting "go ahead" there is how two
// rebuilds end up running at once — the failure this whole thing exists to prevent.
func TestHoldHeavyIndexLock_DoesNotTreatAFailureAsPermission(t *testing.T) {
	broken := &stubLocker{err: errors.New("connection refused")}

	_, ok, err := holdHeavyIndexLock(context.Background(), broken)

	if err == nil {
		t.Error("a failed lock attempt was reported as success")
	}
	if ok {
		t.Error("ok = true after the lock could not be read at all")
	}
}

// The 2026-09-15 production crash: HoldHeavyIndexLock's own `return func() { release();
// conn.Release() }, true, nil` captured its named-return variable `release` and then
// reassigned that same variable to the closure that captured it, so calling the returned
// function called itself forever — a fatal, unrecoverable stack overflow in every caller
// that ever released the lock (cmd/reindex, cmd/build-suggestions, the dedup passes). This
// pins the fix's shape: composeRelease closes over two plain parameters that are never
// reassigned, so the returned function cannot alias itself.
func TestComposeRelease_CallsInnerThenCleanupExactlyOnce(t *testing.T) {
	var innerCalls, cleanupCalls int
	release := composeRelease(func() { innerCalls++ }, func() { cleanupCalls++ })

	release()

	if innerCalls != 1 {
		t.Errorf("inner called %d times, want 1", innerCalls)
	}
	if cleanupCalls != 1 {
		t.Errorf("cleanup called %d times, want 1", cleanupCalls)
	}
}

type stubLocker struct {
	locked   bool
	released bool
	err      error
}

func (s *stubLocker) tryLock(context.Context, int64) (bool, error) {
	if s.err != nil {
		return false, s.err
	}
	return s.locked, nil
}

func (s *stubLocker) unlock(context.Context, int64) {
	s.released = true
}

// The key has to be unique among the project's advisory-lock users, and migrate.go's comment
// is the list everyone is told to check before picking one. A key that collides silently
// serializes two unrelated things — or, worse, lets one unlock the other.
func TestHeavyIndexLockKey_IsRegisteredAndUnique(t *testing.T) {
	src, err := os.ReadFile("../migrate/migrate.go")
	if err != nil {
		t.Fatal(err)
	}
	registry := string(src)

	if !strings.Contains(registry, fmt.Sprintf("%#x", HeavyIndexLockKey)) {
		t.Errorf("%#x is not in migrate.go's list of advisory-lock keys, which is where the next person looks", HeavyIndexLockKey)
	}
	others := regexp.MustCompile(`0x[0-9a-f]{8}`).FindAllString(registry, -1)
	seen := 0
	for _, o := range others {
		if o == fmt.Sprintf("%#x", HeavyIndexLockKey) {
			seen++
		}
	}
	if seen != 1 {
		t.Errorf("key %#x appears %d times in the registry, want exactly 1", HeavyIndexLockKey, seen)
	}
}
