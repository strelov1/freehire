package worker

import (
	"os"
	"time"

	"github.com/getsentry/sentry-go"
)

// panicFlushTimeout bounds how long capturePanic waits to deliver a fatal panic
// to Sentry before re-raising. It runs on the crash path, so it stays short.
const panicFlushTimeout = 2 * time.Second

// Main is the entry wrapper every cron worker uses in place of a bare
// os.Exit(run()). On the normal path it times the run, writes it to
// PROM_TEXTFILE_DIR (see writeRunMetrics — a no-op when unset), and exits with
// run's status; Sentry was already flushed by Bootstrap's cleanup, which run
// defers. If run panics, the deferred capturePanic reports the panic to Sentry,
// flushes it, and re-panics so the process still crashes with the original stack
// and a non-zero exit code — the metrics write is skipped on that path, since
// Main never regains control to reach it.
//
// Sentry is initialized inside Bootstrap (run's first call), so a panic after
// bootstrap is captured; a panic before it (e.g. bad config) is not — acceptable,
// as those paths already log and exit non-zero.
func Main(run func() int) {
	defer capturePanic()
	start := time.Now()
	code := run()
	writeRunMetrics(time.Since(start), code)
	os.Exit(code)
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
