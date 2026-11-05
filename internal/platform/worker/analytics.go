package worker

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/strelov1/freehire/internal/platform/database"
)

// AnalyticsPool opens the statistics reader. A configured replica never falls back
// to the primary. Without a replica URL, local development uses a separate read-only
// connection to the primary without changing the writer's session settings.
func AnalyticsPool(ctx context.Context, primary *pgxpool.Pool) (*pgxpool.Pool, error) {
	dsn := os.Getenv("ANALYTICS_DATABASE_URL")
	configured := dsn != ""
	if !configured {
		dsn = primary.Config().ConnConfig.ConnString()
	}
	reader, err := database.Connect(ctx, dsn, func(c *pgxpool.Config) {
		c.MaxConns = 2
		c.MinConns = 0
		c.ConnConfig.RuntimeParams["application_name"] = "freehire-" + workerJob() + "-analytics"
		c.ConnConfig.RuntimeParams["default_transaction_read_only"] = "on"
		c.ConnConfig.RuntimeParams["work_mem"] = "256MB"
		c.ConnConfig.RuntimeParams["statement_timeout"] = "1800000"
		c.ConnConfig.RuntimeParams["idle_in_transaction_session_timeout"] = "60000"
	})
	if err != nil {
		return nil, err
	}
	if !configured {
		return reader, nil
	}
	readyCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	var standby bool
	if err := reader.QueryRow(readyCtx, "SELECT pg_is_in_recovery()").Scan(&standby); err != nil {
		reader.Close()
		return nil, err
	}
	if !standby {
		reader.Close()
		return nil, fmt.Errorf("analytics URL must point to a PostgreSQL standby")
	}
	var lsn string
	if err := primary.QueryRow(readyCtx, "SELECT pg_current_wal_lsn()::text").Scan(&lsn); err != nil {
		reader.Close()
		return nil, err
	}
	for {
		var caughtUp bool
		err = reader.QueryRow(readyCtx, "SELECT NOT pg_is_wal_replay_paused() AND pg_last_wal_replay_lsn() >= $1::pg_lsn", lsn).Scan(&caughtUp)
		if err != nil {
			reader.Close()
			return nil, fmt.Errorf("analytics catch-up: %w", err)
		}
		if caughtUp {
			return reader, nil
		}
		timer := time.NewTimer(time.Second)
		select {
		case <-readyCtx.Done():
			timer.Stop()
			reader.Close()
			return nil, fmt.Errorf("analytics catch-up: %w", readyCtx.Err())
		case <-timer.C:
		}
	}
}
