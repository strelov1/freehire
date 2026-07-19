package database

import (
	"context"
	"fmt"
	"strings"
	"time"

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

// Connect creates a Postgres connection pool and verifies it is reachable.
func Connect(ctx context.Context, dsn string) (*pgxpool.Pool, error) {
	config, err := poolConfig(dsn)
	if err != nil {
		return nil, err
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}

	return pool, nil
}

// poolConfig parses the DSN and applies defaultMaxConns unless the DSN sets its
// own pool_max_conns, so an explicit override always wins.
func poolConfig(dsn string) (*pgxpool.Config, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse pool config: %w", err)
	}
	if !strings.Contains(dsn, "pool_max_conns") {
		config.MaxConns = defaultMaxConns
	}
	return config, nil
}
