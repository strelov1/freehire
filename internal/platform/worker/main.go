package worker

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/strelov1/freehire/internal/platform/observability"
	"go.opentelemetry.io/otel/attribute"

	"github.com/strelov1/freehire/internal/platform/config"
)

// Main runs a worker and exits only after its final outcome has been reported.
// Reporting lives outside run so Bootstrap's deferred cleanup cannot shut it
// down before the worker's terminal error or panic is known.
//
// extraPauseKeys are pause keys, beyond the fleet's and the binary's own, that
// also hold this worker. Main knows nothing about what they mean: a block that
// classifies its workers (outbound mail) names the keys, and this package only
// checks them.
func Main(run func() int, extraPauseKeys ...string) {
	os.Exit(runReported(run, extraPauseKeys))
}

func runReported(run func() int, extraPauseKeys []string) int {
	shutdown, err := observability.InitLogs("freehire-" + workerJob())
	if err != nil {
		log.Printf("exception reporting: %v", err)
		return 1
	}
	defer shutdown()
	defer capturePanic()
	code := runOrPause(run, extraPauseKeys)
	if code != 0 {
		observability.CaptureError(context.Background(), fmt.Errorf("worker %s exited with status %d", workerJob(), code), attribute.String("worker.name", workerJob()), attribute.Int("process.exit.code", code))
	}
	return code
}

// runOrPause consults the pause switch and, unless it is held, times the run and
// publishes its outcome. It returns the process's exit status.
//
// The gate sits here rather than in Bootstrap because Bootstrap reports failure
// by returning an error and every worker answers an error with a non-zero exit —
// a pause is an operator's decision, not a failure, and must not page anyone.
// Refusing here also means a held run costs one Redis GET: the database pool is never opened.
func runOrPause(run func() int, extraPauseKeys []string) int {
	if Paused(context.Background(), config.Load().RedisURL, workerJob(), extraPauseKeys...) {
		log.Printf("worker: %s is paused, skipping this run", workerJob())
		writePausedMetrics()
		return 0
	}
	start := time.Now()
	code := run()
	writeRunMetrics(time.Since(start), code)
	return code
}

// capturePanic, deferred by Main, reports an in-flight panic, then re-panics to
// preserve the crash. It is a no-op when nothing panicked.
func capturePanic() {
	if r := recover(); r != nil {
		observability.CapturePanic(context.Background(), r, attribute.String("worker.name", workerJob()))
		panic(r)
	}
}
