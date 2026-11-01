package worker

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/strelov1/freehire/internal/platform/observability"
	otellog "go.opentelemetry.io/otel/log"

	"github.com/strelov1/freehire/internal/platform/config"
)

// panicFlushTimeout bounds how long capturePanic waits to deliver a fatal panic
// to Sentry before re-raising. It runs on the crash path, so it stays short.
const panicFlushTimeout = 2 * time.Second

// Main runs a worker and exits only after its final outcome has been reported.
// Reporting lives outside run so Bootstrap's deferred cleanup cannot shut it
// down before the worker's terminal error or panic is known.
func Main(run func() int) {
	os.Exit(runReported(run))
}

func runReported(run func() int) int {
	shutdown, err := observability.InitLogs("freehire-"+workerJob(), config.Load().SentryEnvironment)
	if err != nil {
		log.Printf("exception reporting: %v", err)
		return 1
	}
	defer shutdown()
	defer capturePanic()
	code := runOrPause(run)
	if code != 0 {
		observability.CaptureError(context.Background(), fmt.Errorf("worker %s exited with status %d", workerJob(), code), otellog.String("worker.name", workerJob()), otellog.Int("process.exit.code", code))
	}
	return code
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
		observability.CapturePanic(context.Background(), r, otellog.String("worker.name", workerJob()))
		sentry.CurrentHub().Recover(r)
		sentry.Flush(panicFlushTimeout)
		panic(r)
	}
}
