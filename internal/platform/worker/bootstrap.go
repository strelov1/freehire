package worker

import (
	"context"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/platform/config"
	"github.com/strelov1/freehire/internal/platform/database"
	"github.com/strelov1/freehire/internal/platform/observability"
)

// signalContext returns a context that is cancelled when the process receives
// SIGINT or SIGTERM, plus a stop function that releases the signal notification.
// A cron timeout or redeploy delivers SIGTERM, so a worker built on this context
// observes the cancellation and unwinds in-flight work instead of being killed.
func signalContext(parent context.Context) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(parent, syscall.SIGINT, syscall.SIGTERM)
}

// Bootstrap performs the setup every standalone worker shares: it initializes
// error reporting, loads config, derives a signal-cancellable root context, and
// opens the database pool. The returned cleanup releases the reporting owner,
// stops the signal notification, and closes the pool; call it with defer. On a
// misconfigured exporter or a connection failure it returns the error with no usable
// pool (releasing anything already acquired), so the caller fails fast.
func Bootstrap(parent context.Context) (context.Context, config.Settings, *pgxpool.Pool, func(), error) {
	cfg := config.Load()

	flush, err := observability.InitLogs("freehire-" + workerJob())
	if err != nil {
		return nil, cfg, nil, nil, err
	}

	ctx, stop := signalContext(parent)

	pool, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		stop()
		flush()
		return nil, cfg, nil, nil, err
	}

	cleanup := func() {
		stop()
		pool.Close()
		flush()
	}
	return ctx, cfg, pool, cleanup, nil
}
