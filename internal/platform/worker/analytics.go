package worker

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/strelov1/freehire/internal/platform/database"
	"github.com/strelov1/freehire/internal/platform/pgerr"
)

// AnalyticsPool opens the statistics reader. A configured replica never falls back
// to the primary. Without a replica URL, local development uses a separate read-only
// connection to the primary without changing the writer's session settings.
func AnalyticsPool(ctx context.Context, primary *pgxpool.Pool) (*pgxpool.Pool, error) {
	dsn := os.Getenv("ANALYTICS_DATABASE_URL")
	configured := dsn != ""
	if !configured {
		log.Print("analytics: ANALYTICS_DATABASE_URL is unset; reading statistics from the primary")
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

// standbyConflictAttempts bounds how often one statement is rerun after the standby
// cancelled it. A conflict is a single replay event (a relation lock or buffer pin the
// primary's vacuum needs), so the rerun usually passes; repeated conflicts still fail.
const standbyConflictAttempts = 3

// standbyConflictPause lets replay move past the record that cancelled the statement.
var standbyConflictPause = 30 * time.Second

// RetryStandbyConflict runs one analytics statement and reruns it when the standby
// cancelled it for a recovery conflict. On a read-only standby that is the only source
// of SQLSTATE 40001 and 40P01, and hot_standby_feedback cannot prevent the lock and
// buffer-pin kinds, so one cancel used to fail the whole multi-hour rebuild.
func RetryStandbyConflict[R any](ctx context.Context, name string, run func() (R, error)) (R, error) {
	for attempt := 1; ; attempt++ {
		result, err := run()
		if err == nil || attempt == standbyConflictAttempts ||
			(!pgerr.IsSerializationFailure(err) && !pgerr.IsDeadlock(err)) {
			return result, err
		}
		log.Printf("analytics: %s cancelled by a recovery conflict (attempt %d of %d), retrying: %v", name, attempt, standbyConflictAttempts, err)
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		case <-time.After(standbyConflictPause):
		}
	}
}
