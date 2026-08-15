package worker

import (
	"os"
	"path/filepath"
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
