package database

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// defaultMaxConns caps each process's pool when the DSN does not set pool_max_conns. pgxpool's
// default (max(4, NumCPU)) across the API server and many cron workers can exhaust Postgres's
// connection slots (SQLSTATE 53300). Ops can raise one process via pool_max_conns in its DSN.
const defaultMaxConns = 10

// reachableWindow bounds how long Connect keeps retrying a database that is not answering yet.
// A Postgres container running init scripts serves only a temporary unix-socket server, so TCP
// is refused until init ends, and a database restart under a worker looks the same. Kept short so
// a worker with a wrong DSN still fails its slot promptly. A shorter caller context wins.
const reachableWindow = 30 * time.Second

// Option adjusts a pool before it is opened.
type Option func(*pgxpool.Config)

// WithStatementTimeout makes Postgres cancel any query on this pool that runs longer than d.
//
// Opt-in, because the right value differs by caller: only cmd/server asks for it. Cron workers
// share this package and some run for hours (backfill-derive), so a package default would fail
// correct batch passes. Nothing else cuts a runaway query: the schema sets statement_timeout to 0
// and Fiber's WriteTimeout closes the socket while the query keeps its connection.
//
// It bounds the query, not the wait for a connection; bound the acquire with a context deadline.
func WithStatementTimeout(d time.Duration) Option {
	return func(c *pgxpool.Config) {
		// Unitless values are milliseconds.
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
// ends. Backoff doubles to a two-second ceiling.
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
		// The caller's context ending is not a database fault; do not retry (SIGTERM stops now).
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

// poolConfig parses the DSN and applies defaultMaxConns unless the DSN sets pool_max_conns.
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
