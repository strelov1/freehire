package worker

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/getsentry/sentry-go"

	"github.com/strelov1/freehire/internal/config"
)

// panicFlushTimeout bounds how long capturePanic waits to deliver a fatal panic
// to Sentry before re-raising. It runs on the crash path, so it stays short.
const panicFlushTimeout = 2 * time.Second

// Main is the entry wrapper every cron worker uses in place of a bare
// os.Exit(run()). On the normal path it times the run, writes it to
// PROM_TEXTFILE_DIR (see writeRunMetrics — a no-op when unset), and exits with
// run's status; Sentry was already flushed by Bootstrap's cleanup, which run
// defers. Before any of that it consults the pause switch (see runOrPause), so a
// held fleet exits zero without run being called at all.
//
// If run panics, the deferred capturePanic reports the panic to Sentry,
// flushes it, and re-panics so the process still crashes with the original stack
// and a non-zero exit code — the metrics write is skipped on that path, since
// Main never regains control to reach it.
//
// Sentry is initialized inside Bootstrap (run's first call), so a panic after
// bootstrap is captured; a panic before it (e.g. bad config) is not — acceptable,
// as those paths already log and exit non-zero.
func Main(run func() int) {
	defer capturePanic()
	os.Exit(runOrPause(run))
}

// runOrPause consults the pause switch and, unless it is held, times the run and
// publishes its outcome. It returns the process's exit status.
//
// The gate sits here rather than in Bootstrap because Bootstrap reports failure
// by returning an error and every worker answers an error with a non-zero exit —
// a pause is an operator's decision, not a failure, and must not page anyone.
// Refusing here also means a held run costs one Redis GET: Sentry is never
// initialized and the database pool is never opened.
func runOrPause(run func() int) int {
	if Paused(context.Background(), config.Load().RedisURL, workerJob()) {
		log.Printf("worker: %s is paused, skipping this run", workerJob())
		writePausedMetrics()
		return 0
	}
	start := time.Now()
	code := run()
	writeRunMetrics(time.Since(start), code)
	return code
}

// capturePanic, deferred by Main, reports an in-flight panic to Sentry, flushes,
// then re-panics to preserve the crash. It is a no-op when nothing panicked, and
// harmless when Sentry is unconfigured (the global hub's capture is a no-op).
func capturePanic() {
	if r := recover(); r != nil {
		sentry.CurrentHub().Recover(r)
		sentry.Flush(panicFlushTimeout)
		panic(r)
	}
}
