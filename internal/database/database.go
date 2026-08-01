package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// defaultMaxConns caps each process's pool when the DSN does not set
// pool_max_conns. pgxpool otherwise defaults to max(4, NumCPU) connections per
// pool; with the API server and a dozen cron workers each opening a pool on a
// many-core host, that sum can exhaust Postgres's non-superuser connection slots
// (FATAL ... reserved for roles with the SUPERUSER attribute, SQLSTATE 53300). A
// modest per-process cap keeps the fleet's total bounded, and ops can still raise
// an individual process by adding pool_max_conns to its DATABASE_URL.
const defaultMaxConns = 10

// reachableWindow bounds how long Connect keeps retrying a database that is not answering yet.
//
// It exists because "not answering yet" is the normal case at two moments, and a single ping
// loses to both. A Postgres container running its init scripts serves a TEMPORARY server on a
// unix socket only, so every TCP connect is refused until init finishes and the real server
// starts — the window grows with each migration added, and CI's smoke run began losing that race
// (freehire#1461: the app died one second before "database system is ready"). The same shape
// happens in production whenever the database restarts under a worker.
//
// Thirty seconds is chosen against the failure it must not cause: a cron worker whose DSN is
// simply wrong should still fail its slot promptly rather than hold it. The caller's context
// wins if it is shorter, so a SIGTERM during startup cancels immediately.
const reachableWindow = 30 * time.Second

// Connect creates a Postgres connection pool and waits for it to become reachable.
func Connect(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	config, err := poolConfig(dsn)
	if err != nil {
		return nil, err
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	if err := waitReachable(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

// waitReachable pings until the database answers, the window closes, or the caller's context
// ends — whichever comes first. The backoff doubles to a two-second ceiling: the common case is
// a database that becomes ready within a second or two, and a tight loop against a container
// still unpacking its data directory is just noise in the log.
func waitReachable(ctx context.Context, pool *pgxpool.Pool) error {
	deadline := time.Now().Add(reachableWindow)
	backoff := 100 * time.Millisecond

	for attempt := 1; ; attempt++ {
		pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err := pool.Ping(pingCtx)
		cancel()
		if err == nil {
			return nil
		}
		// The caller's context ending is not a database fault, and must not be retried:
		// a worker cancelled by SIGTERM mid-startup stops now, not in thirty seconds.
		if ctx.Err() != nil {
			return fmt.Errorf("ping: %w", ctx.Err())
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("ping: database unreachable after %s (%d attempts): %w",
				reachableWindow, attempt, err)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("ping: %w", ctx.Err())
		case <-time.After(backoff):
		}
		if backoff < 2*time.Second {
			backoff *= 2
		}
	}
}

// poolConfig parses the DSN and applies defaultMaxConns unless the DSN sets its
// own pool_max_conns, so an explicit override always wins.
func poolConfig(dsn string) (*pgxpool.Config, error) {
	// pgxpool.ParseConfig CONSUMES pool_max_conns, so detect the override on a
	// pgx-level parse first (unknown settings land in RuntimeParams). A substring
	// match on the raw DSN could false-positive on a password containing the text.
	connConfig, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse pool config: %w", err)
	}
	_, maxConnsSet := connConfig.RuntimeParams["pool_max_conns"]

	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse pool config: %w", err)
	}
	if !maxConnsSet {
		config.MaxConns = defaultMaxConns
	}
	return config, nil
}
