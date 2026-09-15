package worker

import (
	"context"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"
)

// HeavyIndexLockKey is the advisory lock every catalogue-wide index job takes before it
// starts: the full Meilisearch rebuild, the duplicate-marker passes, and the suggestion
// dictionary build. One at a time, whichever way a run was started.
//
// It is registered in internal/platform/migrate's list of the project's advisory-lock keys,
// which is where a new user is meant to look before picking one. "fhix" — freehire index.
const HeavyIndexLockKey int64 = 0x66686978

// HoldHeavyIndexLock takes HeavyIndexLockKey for as long as the returned release is not
// called, or reports that somebody else is already doing catalogue-wide index work.
//
// It exists because the schedule could not stay correct on its own. Measured 2026-09-15: the
// full rebuild started 03:15 and finished 07:47, and the next started 15:15 and was still
// running at 18:57 — four and a half hours against a schedule laid out when it took about
// three. The 06:45 suggestion build and the 16:30 and 19:30 dedup passes therefore land
// inside a rebuild EVERY day, not on an unlucky one. That afternoon the overlap took the
// site down: load average 701, 51 processes waiting on disk, the front page timing out
// entirely, and an auto-apply attempt that could not even start a browser.
//
// Moving the clock times would fix today and break again at five hours. A lock fixes the
// thing that is actually true — two of these must not run at once — and, unlike a systemd
// condition, it also holds for a run started by hand.
//
// The caller decides what to do when it loses. Every current one SKIPS rather than waits:
// each of these jobs is idempotent and scheduled again soon, so a skipped pass costs one
// cycle, while a queued one holds a Type=oneshot unit open for hours and looks like a hang.
func HoldHeavyIndexLock(ctx context.Context, pool *pgxpool.Pool) (release func(), ok bool, err error) {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("acquire lock connection: %w", err)
	}
	// The lock lives on this ONE connection for its whole life — a session advisory lock is
	// released the moment its session ends, so handing the connection back to the pool
	// mid-run would silently drop it.
	l := &poolLocker{conn: conn}
	innerRelease, ok, err := holdHeavyIndexLock(ctx, l)
	if !ok || err != nil {
		conn.Release()
		return nil, ok, err
	}
	return composeRelease(innerRelease, conn.Release), true, nil
}

// composeRelease returns a release function that calls inner then cleanup exactly once.
// Kept as its own step, rather than inlined as a closure assigned into HoldHeavyIndexLock's
// named `release` return, because that inlined shape is what caused a fatal stack-overflow
// crash on 2026-09-15: `return func() { release(); conn.Release() }, true, nil` builds a
// closure that captures `release` by reference, and assigning that very closure BACK into
// the named return `release` (which is exactly what a named-return `return` statement does)
// made the closure call itself, forever, the moment anything invoked it. `composeRelease`
// closes over two plain, never-reassigned local parameters instead, so there is no variable
// left for the returned closure to alias onto itself.
func composeRelease(inner, cleanup func()) func() {
	return func() {
		inner()
		cleanup()
	}
}

// locker is the two statements this needs, so the decision around them can be tested without
// a database.
type locker interface {
	tryLock(ctx context.Context, key int64) (bool, error)
	unlock(ctx context.Context, key int64)
}

func holdHeavyIndexLock(ctx context.Context, l locker) (func(), bool, error) {
	got, err := l.tryLock(ctx, HeavyIndexLockKey)
	if err != nil {
		// Not permission. A database that cannot answer says nothing about whether a
		// rebuild is running, and assuming it is free is how two of them end up running
		// at once — the exact failure this prevents.
		return nil, false, fmt.Errorf("heavy index lock: %w", err)
	}
	if !got {
		return nil, false, nil
	}
	return func() { l.unlock(ctx, HeavyIndexLockKey) }, true, nil
}

type poolLocker struct{ conn *pgxpool.Conn }

func (p *poolLocker) tryLock(ctx context.Context, key int64) (bool, error) {
	var got bool
	if err := p.conn.QueryRow(ctx, "SELECT pg_try_advisory_lock($1)", key).Scan(&got); err != nil {
		return false, err
	}
	return got, nil
}

func (p *poolLocker) unlock(ctx context.Context, key int64) {
	// Best-effort: releasing the connection ends the session and drops the lock anyway.
	if _, err := p.conn.Exec(ctx, "SELECT pg_advisory_unlock($1)", key); err != nil {
		log.Printf("heavy index lock: release: %v", err)
	}
}
