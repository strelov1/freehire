package worker

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestRunInstance(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"no args", nil, ""},
		{"bare board file", []string{"eightfold.yml"}, "eightfold"},
		{"path-like board file", []string{"/opt/freehire/src/hire-current/sources/eightfold.yml"}, "eightfold"},
		{"trailing flag after board file", []string{"sources/eightfold.yml", "--shard=1/6"}, "eightfold"},
		{"flag only", []string{"--posted-within", "168h"}, ""},
		{"end-of-options marker before board file", []string{"--", "sources/eightfold.yml"}, "eightfold"},
		{"end-of-options marker alone", []string{"--"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := runInstance(tt.args); got != tt.want {
				t.Errorf("runInstance(%v) = %q, want %q", tt.args, got, tt.want)
			}
		})
	}
}

func TestWriteTextfileWritesBodyAndLeavesNoTemp(t *testing.T) {
	dir := t.TempDir()

	if err := WriteTextfile(dir, "queue-metrics.prom", "freehire_queue_depth{queue=\"search_outbox\"} 3\n"); err != nil {
		t.Fatalf("WriteTextfile: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "queue-metrics.prom"))
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	if got, want := string(data), "freehire_queue_depth{queue=\"search_outbox\"} 3\n"; got != want {
		t.Errorf("file body = %q, want %q", got, want)
	}
	if _, err := os.Stat(filepath.Join(dir, "queue-metrics.prom.tmp")); !os.IsNotExist(err) {
		t.Errorf("expected the .tmp file to be gone after rename, stat err: %v", err)
	}
}

func TestWriteTextfileReplacesPreviousContentWholesale(t *testing.T) {
	dir := t.TempDir()

	if err := WriteTextfile(dir, "queue-metrics.prom", "old body, considerably longer\n"); err != nil {
		t.Fatalf("first WriteTextfile: %v", err)
	}
	if err := WriteTextfile(dir, "queue-metrics.prom", "new\n"); err != nil {
		t.Fatalf("second WriteTextfile: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "queue-metrics.prom"))
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	// The published file must hold the new body and nothing else. (Atomicity itself
	// is not observable from here — a single-threaded read after the call cannot
	// distinguish a rename from a truncating overwrite; what this pins is that a
	// republish fully replaces rather than appends.)
	if got := string(data); got != "new\n" {
		t.Errorf("file body = %q, want %q", got, "new\n")
	}
}

func TestWriteTextfileKeepsPreviousFileWhenWriteFails(t *testing.T) {
	dir := t.TempDir()

	if err := WriteTextfile(dir, "queue-metrics.prom", "good body\n"); err != nil {
		t.Fatalf("seed WriteTextfile: %v", err)
	}
	// Occupy the temp path with a directory so writing it cannot succeed. This is
	// the collector's worst case: a failed refresh must leave the last good file
	// readable rather than truncating it.
	if err := os.Mkdir(filepath.Join(dir, "queue-metrics.prom.tmp"), 0o755); err != nil {
		t.Fatalf("occupy temp path: %v", err)
	}

	if err := WriteTextfile(dir, "queue-metrics.prom", "body that cannot land\n"); err == nil {
		t.Fatal("WriteTextfile succeeded, want an error when the temp path is unwritable")
	}

	data, err := os.ReadFile(filepath.Join(dir, "queue-metrics.prom"))
	if err != nil {
		t.Fatalf("read previous file: %v", err)
	}
	if got := string(data); got != "good body\n" {
		t.Errorf("previous file body = %q, want it left intact", got)
	}
}

func TestWriteRunMetricsDisabledWithoutEnv(t *testing.T) {
	// Explicitly empty (equivalent to unset for the dir == "" check): the
	// write must be a no-op and, critically, must not touch os.Args[0]'s
	// actual directory or panic trying to resolve one.
	t.Setenv(PromTextfileDirEnv, "")
	writeRunMetrics(time.Second, 0)
}

func TestWriteRunMetricsWritesExpectedSeries(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(PromTextfileDirEnv, dir)

	// os.Args[0] drives the job label; restore it after the test.
	origArgs := os.Args
	defer func() { os.Args = origArgs }()
	os.Args = []string{"/opt/freehire/src/hire-current/ingest", "/opt/freehire/src/hire-current/sources/eightfold.yml"}

	writeRunMetrics(2500*time.Millisecond, 1)

	data, err := os.ReadFile(filepath.Join(dir, "ingest_eightfold.prom"))
	if err != nil {
		t.Fatalf("read metrics file: %v", err)
	}
	got := string(data)

	for _, want := range []string{
		`freehire_worker_last_run_duration_seconds{job="ingest",instance="eightfold"} 2.500000`,
		`freehire_worker_last_run_success{job="ingest",instance="eightfold"} 0`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("metrics file missing %q; got:\n%s", want, got)
		}
	}

	// No leftover .tmp file from the write-then-rename.
	if _, err := os.Stat(filepath.Join(dir, "ingest_eightfold.prom.tmp")); !os.IsNotExist(err) {
		t.Errorf("expected the .tmp file to be gone after rename, stat err: %v", err)
	}
}

// A completed run clears the paused gauge rather than omitting it, so the series
// is a live signal an alert can read as "not held" instead of one that merely
// stops being written.
func TestWriteRunMetricsClearsThePausedGauge(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(PromTextfileDirEnv, dir)

	origArgs := os.Args
	defer func() { os.Args = origArgs }()
	os.Args = []string{"/opt/freehire/src/hire-current/ingest", "sources/eightfold.yml"}

	writeRunMetrics(time.Second, 0)

	data, err := os.ReadFile(filepath.Join(dir, "ingest_eightfold.prom"))
	if err != nil {
		t.Fatalf("read metrics file: %v", err)
	}
	if want := `freehire_worker_paused{job="ingest",instance="eightfold"} 0`; !strings.Contains(string(data), want) {
		t.Errorf("metrics file missing %q; got:\n%s", want, string(data))
	}
}

// A refused run must not refresh the last-run series. Leaving them to age is what
// keeps an existing staleness alert firing for a switch nobody lifted; stamping a
// refused run as a success is the failure mode that kept reindexw's skipped cycles
// invisible for days.
func TestWritePausedMetricsPublishesOnlyThePausedGauge(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(PromTextfileDirEnv, dir)

	origArgs := os.Args
	defer func() { os.Args = origArgs }()
	os.Args = []string{"/opt/freehire/src/hire-current/ingest", "sources/eightfold.yml"}

	writePausedMetrics()

	data, err := os.ReadFile(filepath.Join(dir, "ingest_eightfold.prom"))
	if err != nil {
		t.Fatalf("read metrics file: %v", err)
	}
	got := string(data)

	if want := `freehire_worker_paused{job="ingest",instance="eightfold"} 1`; !strings.Contains(got, want) {
		t.Errorf("metrics file missing %q; got:\n%s", want, got)
	}
	if strings.Contains(got, "freehire_worker_last_run") {
		t.Errorf("a refused run refreshed the last-run series; got:\n%s", got)
	}
}

func TestWritePausedMetricsDisabledWithoutEnv(t *testing.T) {
	t.Setenv(PromTextfileDirEnv, "")
	writePausedMetrics()
}

// The exact text below is the contract the freehire-ops dashboard and alert rules
// bind to. Renaming a metric or a label here silently breaks queries in another
// repository that cannot be compiled against this one, so both outputs are pinned
// in full rather than spot-checked. The run timestamp is the one value that cannot
// be pinned, so it is normalised before comparison.
const (
	wantCompletedRunRender = `# HELP freehire_worker_last_run_timestamp_seconds Unix time the worker last finished a run.
# TYPE freehire_worker_last_run_timestamp_seconds gauge
freehire_worker_last_run_timestamp_seconds{job="ingest",instance="eightfold"} STAMP
# HELP freehire_worker_last_run_duration_seconds How long the worker's last run took, in seconds.
# TYPE freehire_worker_last_run_duration_seconds gauge
freehire_worker_last_run_duration_seconds{job="ingest",instance="eightfold"} 2.500000
# HELP freehire_worker_last_run_success Whether the worker's last run exited zero (1) or not (0).
# TYPE freehire_worker_last_run_success gauge
freehire_worker_last_run_success{job="ingest",instance="eightfold"} 1
# HELP freehire_worker_paused Whether the pause switch held this worker's run back (1) or not (0).
# TYPE freehire_worker_paused gauge
freehire_worker_paused{job="ingest",instance="eightfold"} 0
`

	wantPausedRunRender = `# HELP freehire_worker_paused Whether the pause switch held this worker's run back (1) or not (0).
# TYPE freehire_worker_paused gauge
freehire_worker_paused{job="ingest",instance="eightfold"} 1
`
)

var runStamp = regexp.MustCompile(`(freehire_worker_last_run_timestamp_seconds\{[^}]*\} )\d+`)

func readRunFile(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "ingest_eightfold.prom"))
	if err != nil {
		t.Fatalf("read metrics file: %v", err)
	}
	return runStamp.ReplaceAllString(string(data), "${1}STAMP")
}

func TestMetricsRenderMatchesThePublishedContract(t *testing.T) {
	origArgs := os.Args
	defer func() { os.Args = origArgs }()
	os.Args = []string{"/opt/freehire/src/hire-current/ingest", "sources/eightfold.yml"}

	t.Run("completed run", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv(PromTextfileDirEnv, dir)

		writeRunMetrics(2500*time.Millisecond, 0)

		if got := readRunFile(t, dir); got != wantCompletedRunRender {
			t.Errorf("completed-run render mismatch:\ngot:\n%s\nwant:\n%s", got, wantCompletedRunRender)
		}
	})

	t.Run("refused run", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv(PromTextfileDirEnv, dir)

		writePausedMetrics()

		if got := readRunFile(t, dir); got != wantPausedRunRender {
			t.Errorf("refused-run render mismatch:\ngot:\n%s\nwant:\n%s", got, wantPausedRunRender)
		}
	})
}

func TestWriteRunMetricsWithoutInstance(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(PromTextfileDirEnv, dir)

	origArgs := os.Args
	defer func() { os.Args = origArgs }()
	os.Args = []string{"/opt/freehire/src/hire-current/embed"}

	writeRunMetrics(time.Millisecond, 0)

	data, err := os.ReadFile(filepath.Join(dir, "embed.prom"))
	if err != nil {
		t.Fatalf("read metrics file: %v", err)
	}
	if !strings.Contains(string(data), `freehire_worker_last_run_success{job="embed"} 1`) {
		t.Errorf("metrics file missing job-only series; got:\n%s", string(data))
	}
}
