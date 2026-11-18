package worker

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// HeavyIndexLockKey is the advisory lock every catalogue-wide index job takes before it
// starts: the full Meilisearch rebuild, the duplicate-marker passes, and the suggestion
// dictionary build. One at a time, whichever way a run was started.
//
// It is registered in internal/platform/migrate's list of the project's advisory-lock keys,
// which is where a new user is meant to look before picking one. "fhix" — freehire index.
const HeavyIndexLockKey int64 = 0x66686978

// Advisory-lock keys held through TryLock, next to HeavyIndexLockKey. The full list of the
// project's keys, including the ones taken elsewhere, is in internal/platform/migrate.
const (
	// GhostCrosscheckLockKey serializes cmd/ghost-crosscheck: concurrent runs would double the
	// read load and interleave reports. "fhgc".
	GhostCrosscheckLockKey int64 = 0x66686763
	// SchedulerLockKey serializes cmd/ingest-scheduler ticks; it makes the fleet's concurrency
	// cap atomic, which a check-then-act pair is not. "fhsc".
	SchedulerLockKey int64 = 0x66687363
	// RewardLockKey serializes cmd/billing-sync's referral pass across processes, so the
	// per-referrer reward ceiling is a bound. "fhrw".
	RewardLockKey int64 = 0x66687277
)

// HoldHeavyIndexLock takes HeavyIndexLockKey for as long as the returned release is not
// called, or reports that somebody else is already doing catalogue-wide index work.
//
// Schedule times cannot keep these apart once a rebuild outgrows its slot, and an overlap
// takes the site down. A lock, unlike a systemd condition, also holds for a run started by
// hand.
//
// The caller decides what to do when it loses. Every current one SKIPS rather than waits:
// each of these jobs is idempotent and scheduled again soon, so a skipped pass costs one
// cycle, while a queued one holds a Type=oneshot unit open for hours and looks like a hang.
func HoldHeavyIndexLock(ctx context.Context, pool *pgxpool.Pool) (release func(), ok bool, err error) {
	return TryLock(ctx, pool, HeavyIndexLockKey)
}

// TryLock takes the session advisory lock `key` without waiting, for as long as the returned
// release is not called. ok is false (and release nil) when another session holds it. Pick the
// key from the list in internal/platform/migrate, and add the new one there.
//
// A non-blocking lock is the right shape for a Type=oneshot worker: a queued run holds its unit
// open and systemd shows it as a hang, while a skipped one is simply the next tick's work. An
// error is NOT "somebody else holds it": a database that cannot answer says nothing about
// whether a run is in flight, and assuming it is free lets two run at once.
//
// release unlocks on a context that outlives ctx, so a cancelled run still lets go, then
// returns the connection to the pool.
func TryLock(ctx context.Context, pool *pgxpool.Pool, key int64) (release func(), ok bool, err error) {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("acquire lock connection: %w", err)
	}
	// The lock lives on this ONE connection for its whole life — a session advisory lock is
	// released the moment its session ends, so handing the connection back to the pool
	// mid-run would silently drop it.
	l := &poolLocker{conn: conn}
	innerRelease, ok, err := holdLock(ctx, l, key)
	if !ok || err != nil {
		conn.Release()
		return nil, ok, err
	}
	return composeRelease(innerRelease, conn.Release), true, nil
}

// composeRelease returns a release function that calls inner then cleanup exactly once.
// Do not inline it as `return func() { release(); conn.Release() }, true, nil`: the closure
// captures the named return `release` by reference, and the return statement assigns the
// closure back into it, so it calls itself forever. Here it closes over plain parameters.
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

func holdLock(ctx context.Context, l locker, key int64) (func(), bool, error) {
	got, err := l.tryLock(ctx, key)
	if err != nil {
		return nil, false, fmt.Errorf("advisory lock %#x: %w", key, err)
	}
	if !got {
		return nil, false, nil
	}
	return func() {
		// The caller's ctx may be cancelled by the time the run ends; the unlock must still go
		// out, but bounded, so a stalled connection cannot hold shutdown open.
		uctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), unlockTimeout)
		defer cancel()
		l.unlock(uctx, key)
	}, true, nil
}

// unlockTimeout bounds the unlock statement; the session's end drops the lock anyway.
const unlockTimeout = 5 * time.Second

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
		log.Printf("advisory lock %#x: release: %v", key, err)
	}
}
