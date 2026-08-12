package worker

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// PromTextfileDirEnv names the environment variable that turns on per-run
// Prometheus metrics for cron workers, written as a node_exporter textfile-
// collector file (https://github.com/prometheus/node_exporter#textfile-collector).
// A worker is a run-once-and-exit process — there is nothing for Prometheus to
// scrape between runs — so this is the pull-model equivalent for it, reusing the
// node_exporter already deployed on host-2 rather than standing up a
// Pushgateway. Unset disables it, matching this repo's other optional,
// env-gated integrations (Sentry, S3, search).
const PromTextfileDirEnv = "PROM_TEXTFILE_DIR"

// writeRunMetrics records the outcome of one worker run as a
// freehire_worker_last_run_* gauge set, if PROM_TEXTFILE_DIR is set. The job
// label is the running binary's name (os.Args[0]); when the binary was invoked
// with a trailing path-like argument (cmd/ingest's per-board source file,
// `sources/<board>.yml`), that argument's base name (extension stripped) becomes
// the instance label, so ~140 independently-timered ingest boards land in
// distinct series instead of overwriting one shared file. Errors are logged, not
// fatal — a metrics file failing to write must not fail the worker's actual job.
func writeRunMetrics(runDuration time.Duration, exitCode int) {
	dir := os.Getenv(PromTextfileDirEnv)
	if dir == "" {
		return
	}
	job := filepath.Base(os.Args[0])
	instance := runInstance(os.Args[1:])

	labels := fmt.Sprintf(`job=%q`, job)
	name := job
	if instance != "" {
		labels = fmt.Sprintf(`job=%q,instance=%q`, job, instance)
		name += "_" + instance
	}

	success := 0
	if exitCode == 0 {
		success = 1
	}

	content := fmt.Sprintf(`# HELP freehire_worker_last_run_timestamp_seconds Unix time the worker last finished a run.
# TYPE freehire_worker_last_run_timestamp_seconds gauge
freehire_worker_last_run_timestamp_seconds{%[1]s} %[2]d
# HELP freehire_worker_last_run_duration_seconds How long the worker's last run took, in seconds.
# TYPE freehire_worker_last_run_duration_seconds gauge
freehire_worker_last_run_duration_seconds{%[1]s} %[3]f
# HELP freehire_worker_last_run_success Whether the worker's last run exited zero (1) or not (0).
# TYPE freehire_worker_last_run_success gauge
freehire_worker_last_run_success{%[1]s} %[4]d
`, labels, time.Now().Unix(), runDuration.Seconds(), success)

	// Write-then-rename: the textfile collector polls the directory on its own
	// schedule and skips a file it fails to fully parse, but a half-written file
	// (this worker killed mid-write) could still be picked up mid-write and read
	// as garbage. Renaming into place is atomic on the same filesystem, so it
	// only ever sees the old file or the complete new one.
	path := filepath.Join(dir, name+".prom")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		log.Printf("worker: write metrics file %s: %v", tmp, err)
		return
	}
	if err := os.Rename(tmp, path); err != nil {
		log.Printf("worker: rename metrics file %s: %v", tmp, err)
	}
}

// runInstance derives a metric instance label from a worker's trailing
// command-line argument, when it looks like a path (cmd/ingest's
// `sources/<board>.yml`). Returns "" for a worker invoked with no arguments
// (embed, search-drain, reindex), which then reports under job alone.
func runInstance(args []string) string {
	if len(args) == 0 {
		return ""
	}
	base := filepath.Base(args[len(args)-1])
	return strings.TrimSuffix(base, filepath.Ext(base))
}
