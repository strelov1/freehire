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
	labels := metricLabels()

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
`, labels, time.Now().Unix(), runDuration.Seconds(), success) + pausedGauge(labels, 0)

	publishRunFile(dir, content)
}

// writePausedMetrics records that the pause switch refused this run, if
// PROM_TEXTFILE_DIR is set. It publishes the paused gauge ALONE: the last-run
// series are deliberately left to age, so an existing staleness rule still fires
// for a switch nobody lifted, while this gauge beside it identifies the silence
// as deliberate rather than broken.
func writePausedMetrics() {
	dir := os.Getenv(PromTextfileDirEnv)
	if dir == "" {
		return
	}
	publishRunFile(dir, pausedGauge(metricLabels(), 1))
}

// metricLabels renders the label set both writers share: the running binary's
// name, plus the board instance when the invocation carries one.
func metricLabels() string {
	job := filepath.Base(os.Args[0])
	if instance := runInstance(os.Args[1:]); instance != "" {
		return fmt.Sprintf(`job=%q,instance=%q`, job, instance)
	}
	return fmt.Sprintf(`job=%q`, job)
}

// publishRunFile writes this process's metrics file. Errors are logged, not
// fatal — a metrics file failing to write must not fail the worker's actual job.
func publishRunFile(dir, content string) {
	if err := WriteTextfile(dir, RunMetricsFilename(), content); err != nil {
		log.Printf("worker: %v", err)
	}
}

// pausedGauge renders the freehire_worker_paused series, which reports whether
// the pause switch held this run back. It is published by both writers so the
// gauge is a live 0/1 signal rather than one that merely stops being written.
func pausedGauge(labels string, held int) string {
	return fmt.Sprintf(`# HELP freehire_worker_paused Whether the pause switch held this worker's run back (1) or not (0).
# TYPE freehire_worker_paused gauge
freehire_worker_paused{%s} %d
`, labels, held)
}

// RunMetricsFilename reports the textfile-collector file this process's run
// outcome lands in: the binary's name, plus any board instance, plus ".prom".
//
// Exported because Main writes that file AFTER run() returns (main.go:29-30), so
// a worker that ALSO publishes a textfile of its own would have its payload
// silently overwritten on every single run if it picked the same name. A worker
// in that position must own a distinct filename and assert it — see
// cmd/queue-metrics.
func RunMetricsFilename() string {
	name := filepath.Base(os.Args[0])
	if instance := runInstance(os.Args[1:]); instance != "" {
		name += "_" + instance
	}
	return name + ".prom"
}

// WriteTextfile publishes body as the textfile-collector file dir/name, the
// mechanism a run-once worker uses to expose Prometheus metrics it has no
// listener to serve (see PromTextfileDirEnv). Callers own the decision to
// publish at all — this writes unconditionally, so a worker gated on an unset
// PROM_TEXTFILE_DIR must return before calling it.
//
// Write-then-rename: the textfile collector polls the directory on its own
// schedule and skips a file it fails to fully parse, but a half-written file
// (the worker killed mid-write) could still be picked up mid-write and read as
// garbage. Renaming into place is atomic on the same filesystem, so the
// collector only ever sees the old file or the complete new one — and a failed
// write leaves the last good file untouched rather than truncating it.
func WriteTextfile(dir, name, body string) error {
	path := filepath.Join(dir, name)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(body), 0o644); err != nil {
		return fmt.Errorf("write metrics file %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename metrics file %s: %w", tmp, err)
	}
	return nil
}

// runInstance derives a metric instance label from a worker's command-line
// arguments, when one looks like a path (cmd/ingest's `sources/<board>.yml`).
// It scans for the first non-flag argument rather than assuming the last one,
// since a flag can trail the path (`ingest sources/eightfold.yml --shard=1/6`).
// A bare (non-"=") flag's space-separated value (`reindex --posted-within
// 168h`) is skipped along with the flag itself, so it isn't mistaken for a
// board path. Returns "" when no non-flag argument is present (embed,
// search-drain, reindex, or reindex --posted-within 168h), which then reports
// under job alone.
func runInstance(args []string) string {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			// The POSIX end-of-options marker: everything after it is positional,
			// so it must not be treated as a bare flag whose "value" (the board
			// path itself) gets skipped by the generic rule below.
			if i+1 < len(args) {
				base := filepath.Base(args[i+1])
				return strings.TrimSuffix(base, filepath.Ext(base))
			}
			return ""
		}
		if !strings.HasPrefix(arg, "-") {
			base := filepath.Base(arg)
			return strings.TrimSuffix(base, filepath.Ext(base))
		}
		if !strings.Contains(arg, "=") && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
			i++
		}
	}
	return ""
}
