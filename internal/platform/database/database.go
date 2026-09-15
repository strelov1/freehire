package database

import (
	"context"
	"fmt"
	"strconv"
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

// Option adjusts a pool before it is opened.
type Option func(*pgxpool.Config)

// WithStatementTimeout makes Postgres cancel any query on this pool that runs longer than d.
//
// It is the backstop for the endpoint whose cost nobody anticipated, and it is opt-in because
// the right answer differs by caller rather than by deployment. On 2026-09-14 a deep-offset
// crawl pinned all ten of the API server's connections in one query for minutes each; every
// other request queued behind them and nginx answered 504 for 54 minutes. Nothing cut those
// queries: the schema sets statement_timeout to 0, pgx sets none, and Fiber's WriteTimeout
// closes the client socket while the handler goroutine and its in-flight query keep the
// connection.
//
// Only cmd/server asks for it. The cron workers share this package and some legitimately run
// for hours — backfill-derive walks the whole catalogue — so a package-wide default would
// turn a correct batch pass into a nightly failure.
//
// It bounds the QUERY, not the wait for a connection. A caller that also needs the ACQUIRE
// bounded must carry a context deadline; the two failures are different and this covers the
// one that holds a connection hostage.
func WithStatementTimeout(d time.Duration) Option {
	return func(c *pgxpool.Config) {
		// Milliseconds, as a bare integer, is what Postgres takes for a unitless value.
		c.ConnConfig.RuntimeParams["statement_timeout"] = strconv.FormatInt(d.Milliseconds(), 10)
	}
}

// Connect creates a Postgres connection pool and waits for it to become reachable.
func Connect(ctx context.Context, dsn string, opts ...Option) (*pgxpool.Pool, error) {
	config, err := poolConfig(dsn)
	if err != nil {
		return nil, err
	}
	for _, opt := range opts {
		opt(config)
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
